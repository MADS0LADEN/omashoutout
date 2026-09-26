package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/MADS0LADEN/omashoutout/internal/audio"
	"github.com/MADS0LADEN/omashoutout/internal/config"
	"github.com/MADS0LADEN/omashoutout/internal/control"
	"github.com/MADS0LADEN/omashoutout/internal/discovery"
	"github.com/MADS0LADEN/omashoutout/internal/service"
	"github.com/MADS0LADEN/omashoutout/omarchy"
)

const omarchyPluginID = "io.github.MADS0LADEN.omashoutout"

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("omashoutout", "error", err)
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
		fmt.Println("Omashoutout " + version + ` — virtual audio output for Google Cast

Commands:
  devices                 Discover receivers on the local network
  setup --device NAME     Save a destination
        --host ADDRESS    Or use a receiver address directly
  run                     Run the audio output and local settings service
  install                 Install and start a systemd user service
  uninstall               Remove installed service, binary and desktop entry
  configure               Open Omashoutout settings
  status                  Print the running service status
  config                  Print the current configuration
  apply                   Apply JSON configuration read from stdin
  doctor                  Check runtime prerequisites
  version                 Print build version

Use the desktop audio controls for volume and mute. On Omarchy that is the Audio panel.`)
		return nil
	case "version":
		fmt.Println(version)
		return nil
	case "devices":
		r, err := control.Call(control.Request{Method: "status"})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(r.Devices)
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
		fmt.Println("Saved destination:", c.DeviceName, "— run omashoutout install or omashoutout run.")
		return nil
	case "run":
		return daemon(path)
	case "configure":
		if _, err := exec.LookPath("omarchy-shell"); err == nil {
			cmd := exec.Command("omarchy-shell", omarchyPluginID, "open")
			cmd.Env = omarchyShellEnv()
			return cmd.Run()
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		plugin := filepath.Join(home, ".local", "lib", "qt6", "plugins", "plasma", "kcms", "systemsettings_qwidgets", "kcm_omashoutout.so")
		cmd := exec.Command("systemsettings", "kcm_omashoutout")
		// Packaged plugins use Qt's system search path. Extend it only for
		// installations in the user's home directory.
		if _, err = os.Stat(plugin); err == nil {
			cmd.Env = append(os.Environ(), "QT_PLUGIN_PATH="+pluginSearchPath(home))
		}
		return cmd.Run()
	case "status", "config":
		r, err := control.Call(control.Request{Method: "status"})
		if err != nil {
			return err
		}
		if args[0] == "config" {
			return json.NewEncoder(os.Stdout).Encode(r.Config)
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	case "apply":
		var c config.Config
		if err = decodeSingleJSONObject(os.Stdin, &c); err != nil {
			return err
		}
		r, err := control.Call(control.Request{Method: "configure", Config: &c})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(r.Config)
	case "doctor":
		return doctor()
	case "install":
		return install(false)
	case "uninstall":
		return install(true)
	default:
		return fmt.Errorf("unknown command %q; use omashoutout help", args[0])
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
	lock, err := os.OpenFile(filepath.Join(runtime, "omashoutout.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("Omashoutout is already running")
	}
	listener, err := control.Listen()
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := service.New(c, path)
	discoveryDone := make(chan struct{})
	go func() { defer close(discoveryDone); s.Discovery.Run(ctx) }()
	defer func() { cancel(); <-discoveryDone }()
	done := make(chan error, 1)
	go func() { done <- control.Run(ctx, s, listener); cancel() }()
	slog.Info("native control ready")
	err = s.Run(ctx)
	cancel()
	controlErr := <-done
	if err != nil {
		return err
	}
	return controlErr
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
	for _, encoder := range []string{"libmp3lame", " aac ", "libopus"} {
		if err != nil || !strings.Contains(string(b), encoder) {
			bad = true
			fmt.Println("MISSING FFmpeg encoder", strings.TrimSpace(encoder))
		} else {
			fmt.Println("OK encoder", strings.TrimSpace(encoder))
		}
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
	binaryPath := filepath.Join(home, ".local", "bin", "omashoutout")
	unitPath := filepath.Join(conf, "systemd", "user", "omashoutout.service")
	desktopPath := filepath.Join(data, "applications", "omashoutout.desktop")
	environmentPath := filepath.Join(conf, "plasma-workspace", "env", "omashoutout.sh")
	if remove {
		if omarchyCLIAvailable() {
			pluginDir := filepath.Join(conf, "omarchy", "plugins", omarchyPluginID)
			if _, statErr := os.Stat(pluginDir); statErr == nil {
				cmd := exec.Command("omarchy", "plugin", "remove", omarchyPluginID, "--yes")
				cmd.Env = omarchyShellEnv()
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				_ = cmd.Run()
			}
		}
		cmd := exec.Command("systemctl", "--user", "disable", "--now", "omashoutout.service")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
		if err = audio.RemoveSink(context.Background()); err != nil {
			return err
		}
		for _, p := range []string{unitPath, desktopPath, binaryPath, environmentPath, filepath.Join(home, ".local", "lib", "qt6", "plugins", "plasma", "kcms", "systemsettings_qwidgets", "kcm_omashoutout.so")} {
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
	temp, err := os.CreateTemp(filepath.Dir(binaryPath), ".omashoutout-*")
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
	// Install the native module beside the per-user Qt plugin tree when supplied.
	moduleSource := filepath.Join(filepath.Dir(self), "kcm_omashoutout.so")
	if moduleData, readErr := os.ReadFile(moduleSource); readErr == nil {
		moduleDest := filepath.Join(home, ".local", "lib", "qt6", "plugins", "plasma", "kcms", "systemsettings_qwidgets", "kcm_omashoutout.so")
		if err = os.MkdirAll(filepath.Dir(moduleDest), 0755); err != nil {
			return err
		}
		if err = replaceFile(moduleDest, moduleData, 0755); err != nil {
			return err
		}
		// Plasma sources this on login so its standard Settings launcher finds the module.
		if err = os.MkdirAll(filepath.Dir(environmentPath), 0755); err != nil {
			return err
		}
		pluginRoot := filepath.Join(home, ".local", "lib", "qt6", "plugins")
		quotedRoot := "'" + strings.ReplaceAll(pluginRoot, "'", "'\"'\"'") + "'"
		if err = os.WriteFile(environmentPath, []byte("export QT_PLUGIN_PATH="+quotedRoot+"${QT_PLUGIN_PATH:+:$QT_PLUGIN_PATH}\n"), 0644); err != nil {
			return err
		}
	}
	// systemd interprets percent specifiers even in quoted command arguments.
	escaped := strconv.Quote(strings.ReplaceAll(binaryPath, "%", "%%"))
	unit := "[Unit]\nDescription=Omashoutout virtual audio output\nAfter=pipewire-pulse.service\nStartLimitIntervalSec=60\nStartLimitBurst=5\n\n[Service]\nExecStart=" + escaped + " run\nRestart=on-failure\nRestartSec=3\nTimeoutStopSec=10\nNoNewPrivileges=yes\n\n[Install]\nWantedBy=default.target\n"
	if err = os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return err
	}
	desktop := "[Desktop Entry]\nType=Application\nName=Omashoutout\nComment=Configure your virtual audio output\nExec=" + escaped + " configure\nIcon=audio-speakers\nTerminal=false\nCategories=AudioVideo;Audio;\n"
	if err = os.WriteFile(desktopPath, []byte(desktop), 0644); err != nil {
		return err
	}
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "omashoutout.service"}, {"--user", "restart", "omashoutout.service"}} {
		cmd := exec.Command("systemctl", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	if _, err := exec.LookPath("omarchy-shell"); err == nil {
		if err = installOmarchyPlugin(conf); err != nil {
			return err
		}
	}
	fmt.Println("Installed and started. Use the desktop audio controls for volume and mute, and Omashoutout settings for destination and presets.")
	return nil
}

func omarchyShellEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "OMARCHY_SHELL_IPC_TIMEOUT=") {
			continue
		}
		env = append(env, entry)
	}
	// Plugin rescan walks every installed widget and often exceeds the
	// shell helper's 2s default.
	return append(env, "OMARCHY_SHELL_IPC_TIMEOUT=30s")
}

func omarchyCLIAvailable() bool {
	if _, err := exec.LookPath("omarchy-shell"); err == nil {
		return true
	}
	_, err := exec.LookPath("omarchy")
	return err == nil
}

func installOmarchyPlugin(confDir string) error {
	pluginDir := filepath.Join(confDir, "omarchy", "plugins", omarchyPluginID)
	if err := omarchy.InstallDir(pluginDir); err != nil {
		return err
	}
	shellJSON := filepath.Join(confDir, "omarchy", "shell.json")
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		cmd := exec.Command("omarchy-shell", "shell", "rescanPlugins")
		cmd.Env = omarchyShellEnv()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		if omarchyPluginInShellJSON(shellJSON, omarchyPluginID) {
			return nil
		}
		cmd = exec.Command("omarchy", "plugin", "enable", omarchyPluginID, "--section", "right")
		cmd.Env = omarchyShellEnv()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			last = err
			continue
		}
		return nil
	}
	if last != nil {
		return last
	}
	return errors.New("Omarchy did not enable the settings plugin")
}

func omarchyPluginInShellJSON(path, pluginID string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		Bar struct {
			Layout map[string][]json.RawMessage `json:"layout"`
		} `json:"bar"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return false
	}
	for _, section := range doc.Bar.Layout {
		for _, raw := range section {
			var item struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &item) == nil && item.ID == pluginID {
				return true
			}
		}
	}
	return false
}

func decodeSingleJSONObject(r io.Reader, dest any) error {
	d := json.NewDecoder(io.LimitReader(r, 16384))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return err
	}
	rest, err := io.ReadAll(d.Buffered())
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(rest)) != "" {
		return errors.New("unexpected trailing data")
	}
	return nil
}

func pluginSearchPath(home string) string {
	root := filepath.Join(home, ".local", "lib", "qt6", "plugins")
	if existing := os.Getenv("QT_PLUGIN_PATH"); existing != "" {
		return root + ":" + existing
	}
	return root
}

func replaceFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".omashoutout-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
