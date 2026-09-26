package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// MaxReceiverVolume is an absolute ceiling, including during calibration.
const MaxReceiverVolume = 0.05

type Config struct {
	Enabled        bool    `json:"enabled"`
	Version        int     `json:"version"`
	DeviceID       string  `json:"device_id"`
	DeviceName     string  `json:"device_name"`
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	TrimDB         float64 `json:"trim_db"`
	ReceiverVolume float64 `json:"receiver_volume"`
	Muted          bool    `json:"muted"`
	Preset         string  `json:"preset"`
	Bitrate        int     `json:"bitrate_kbps"`
	BufferMS       int     `json:"buffer_ms"`
	MediaPort      int     `json:"media_port"`
}

func Default() Config {
	return Config{Enabled: true, Version: 1, Port: 8009, TrimDB: -40, ReceiverVolume: 0.01, Muted: true, Preset: "video", Bitrate: 192, BufferMS: 200, MediaPort: 17833}
}

func (c Config) Validate() error {
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	if c.Port < 1 || c.Port > 65535 || c.MediaPort < 1 || c.MediaPort > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if math.IsNaN(c.TrimDB) || math.IsInf(c.TrimDB, 0) || c.TrimDB > 0 || c.TrimDB < -80 {
		return errors.New("trim must be between -80 and 0 dB")
	}
	if math.IsNaN(c.ReceiverVolume) || math.IsInf(c.ReceiverVolume, 0) || c.ReceiverVolume < 0 || c.ReceiverVolume > MaxReceiverVolume {
		return errors.New("receiver volume must be between 0 and 5%; 5% is the absolute maximum")
	}
	switch c.Preset {
	case "interactive", "video", "music", "custom":
	default:
		return errors.New("unknown preset")
	}
	if c.Bitrate != 128 && c.Bitrate != 192 && c.Bitrate != 256 && c.Bitrate != 320 {
		return errors.New("MP3 bitrate must be 128, 192, 256 or 320 kbps")
	}
	if c.BufferMS < 40 || c.BufferMS > 2000 {
		return errors.New("buffer must be between 40 and 2000 ms")
	}
	return nil
}

func Path() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "shoutout", "config.json"), nil
}
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("read settings: %w", err)
	}
	return c, c.Validate()
}
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
