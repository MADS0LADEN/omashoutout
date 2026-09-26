package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lkarlslund/shoutout/internal/config"
	"github.com/lkarlslund/shoutout/internal/discovery"
	"github.com/lkarlslund/shoutout/internal/service"
	"github.com/lkarlslund/shoutout/internal/settings"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("shoutout", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		args = []string{"help"}
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Println("Shoutout " + version + ` — virtual audio output for Google Cast

Commands:
  devices                 Discover receivers on the local network
  setup --device NAME     Save a destination (always starts muted)
        --host ADDRESS    Or use a receiver address directly
  run                     Run the audio output and local settings service
  install                 Install and start a systemd user service
  uninstall               Remove installed service, binary and desktop entry
  configure               Open settings in the default browser
  status                  Print the running service status
  doctor                  Check runtime prerequisites
  version                 Print build version

Settings: http://127.0.0.1:17832
Receiver volume has an absolute maximum of 5%.`)
		return nil
	case "version":
		fmt.Println(version)
		return nil
	case "devices":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		d, err := discovery.Discover(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(d)
	case "setup":
		f := flag.NewFlagSet("setup", flag.ContinueOnError)
		name := f.String("device", "", "receiver friendly name")
		host := f.String("host", "", "receiver host")
		port := f.Int("port", 8009, "receiver port")
		if err = f.Parse(args[1:]); err != nil {
			return err
		}
		c, err := config.Load(path)
		if err != nil {
			return err
		}
		c.Muted = true
		if *name != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			d, err := discovery.Discover(ctx)
			if err != nil {
				return err
			}
			matches := 0
			for _, dev := range d {
				if strings.EqualFold(dev.Name, *name) {
					c.DeviceID = dev.ID
					c.DeviceName = dev.Name
					c.Host = dev.Host
					c.Port = dev.Port
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("found %d devices named %q; use --host for an unambiguous destination", matches, *name)
			}
		} else if *host != "" {
			c.Host = *host
			c.Port = *port
			c.DeviceName = *host
			c.DeviceID = ""
		} else {
			return errors.New("setup requires --device or --host")
		}
		if err = config.Save(path, c); err != nil {
			return err
		}
		fmt.Println("Saved muted destination:", c.DeviceName, "— run shoutout install or shoutout run.")
		return nil
	case "run":
		return daemon(path)
	case "configure":
		return exec.Command("xdg-open", "http://"+settings.Address).Run()
	case "status":
		client := http.Client{Timeout: 3 * time.Second}
		r, err := client.Get("http://" + settings.Address + "/api/status")
		if err != nil {
			return err
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("status HTTP %d", r.StatusCode)
		}
		_, err = io.Copy(os.Stdout, r.Body)
		return err
	case "doctor":
		return doctor()
	case "install":
		return install(false)
	case "uninstall":
		return install(true)
	default:
		return fmt.Errorf("unknown command %q; use shoutout help", args[0])
	}
}
func daemon(path string) error {
	c, err := config.Load(path)
	if err != nil {
		return err
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return errors.New("XDG_RUNTIME_DIR is required; run inside a user desktop session")
	}
	lock, err := os.OpenFile(filepath.Join(runtime, "shoutout.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("Shoutout is already running")
	}
	listener, err := net.Listen("tcp", settings.Address)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := service.New(c, path)
	done := make(chan error, 1)
	go func() { done <- settings.Run(ctx, s, listener); cancel() }()
	slog.Info("settings available", "url", "http://"+settings.Address)
	err = s.Run(ctx)
	cancel()
	httpErr := <-done
	if err != nil {
		return err
	}
	return httpErr
}
func doctor() error {
	bad := false
	for _, name := range []string{"pactl", "parec", "ffmpeg", "systemctl"} {
		path, err := exec.LookPath(name)
		if err != nil {
			fmt.Println("MISSING", name)
			bad = true
		} else {
			fmt.Println("OK", name, path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "pactl", "info").CombinedOutput()
	if err != nil {
		bad = true
		fmt.Println("FAIL audio server:", strings.TrimSpace(string(b)))
	} else {
		fmt.Println("OK audio server reachable")
	}
	b, err = exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	if err != nil || !strings.Contains(string(b), "libmp3lame") {
		bad = true
		fmt.Println("MISSING FFmpeg MP3 encoder")
	} else {
		fmt.Println("OK MP3 encoder")
	}
	fmt.Println("Receiver must reach this machine on TCP port 17833; discovery uses UDP 5353.")
	if bad {
		return errors.New("runtime prerequisites missing")
	}
	return nil
}
func install(remove bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	conf, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	binaryPath := filepath.Join(home, ".local", "bin", "shoutout")
	unitPath := filepath.Join(conf, "systemd", "user", "shoutout.service")
	desktopPath := filepath.Join(data, "applications", "shoutout.desktop")
	if remove {
		cmd := exec.Command("systemctl", "--user", "disable", "--now", "shoutout.service")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
		for _, p := range []string{unitPath, desktopPath, binaryPath} {
			if err = os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err = exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
			return err
		}
		fmt.Println("Uninstalled. Personal settings were retained.")
		return nil
	}
	if err = doctor(); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	for _, p := range []string{binaryPath, unitPath, desktopPath} {
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(filepath.Dir(binaryPath), ".shoutout-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0755); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(b); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), binaryPath); err != nil {
		return err
	}
	// systemd interprets percent specifiers even in quoted command arguments.
	escaped := strconv.Quote(strings.ReplaceAll(binaryPath, "%", "%%"))
	unit := "[Unit]\nDescription=Shoutout virtual audio output\nAfter=pipewire-pulse.service\nStartLimitIntervalSec=60\nStartLimitBurst=5\n\n[Service]\nExecStart=" + escaped + " run\nRestart=on-failure\nRestartSec=3\nTimeoutStopSec=10\nNoNewPrivileges=yes\n\n[Install]\nWantedBy=default.target\n"
	if err = os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	desktop := "[Desktop Entry]\nType=Application\nName=Shoutout\nComment=Configure your virtual audio output\nExec=" + escaped + " configure\nIcon=audio-speakers\nTerminal=false\nCategories=AudioVideo;Audio;\n"
	if err = os.WriteFile(desktopPath, []byte(desktop), 0644); err != nil {
		return err
	}
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "shoutout.service"}, {"--user", "restart", "shoutout.service"}} {
		cmd := exec.Command("systemctl", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	fmt.Println("Installed and started. Open Shoutout from the application menu, or visit http://" + settings.Address)
	return nil
}
