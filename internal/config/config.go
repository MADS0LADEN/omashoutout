package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// MaxReceiverVolume is the receiver protocol range. Agent hardware tests stay at or below 0.05.
const MaxReceiverVolume = 1.0

type Config struct {
	TargetDelayMS  int     `json:"target_delay_ms"`
	Codec          string  `json:"codec"`
	SegmentMS      int     `json:"segment_ms"`
	Enabled        bool    `json:"enabled"`
	Version        int     `json:"version"`
	DeviceID       string  `json:"device_id"`
	DeviceName     string  `json:"device_name"`
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	ReceiverVolume float64 `json:"receiver_volume"`
	Preset         string  `json:"preset"`
	Bitrate        int     `json:"bitrate_kbps"`
	MediaPort      int     `json:"media_port"`
}

func Default() Config {
	return Config{TargetDelayMS: 100, Codec: "cast-opus", SegmentMS: 500, Enabled: true, Version: 1, Port: 8009, ReceiverVolume: 0.01, Preset: "balanced", Bitrate: 192, MediaPort: 17833}
}

func (c Config) Validate() error {
	if c.Codec != "aac-hls" && c.Codec != "mp3" && c.Codec != "cast-opus" {
		return errors.New("codec must be cast-opus, aac-hls or mp3")
	}
	if c.TargetDelayMS < 40 || c.TargetDelayMS > 1000 {
		return errors.New("target playback delay must be between 40 and 1000 ms")
	}
	if c.SegmentMS < 250 || c.SegmentMS > 2000 {
		return errors.New("live segment length must be between 250 and 2000 ms")
	}
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	if c.Port < 1 || c.Port > 65535 || c.MediaPort < 1 || c.MediaPort > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if math.IsNaN(c.ReceiverVolume) || math.IsInf(c.ReceiverVolume, 0) || c.ReceiverVolume < 0 || c.ReceiverVolume > MaxReceiverVolume {
		return errors.New("receiver volume must be between 0 and 100%")
	}
	switch c.Preset {
	case "low-latency", "balanced", "high-quality", "custom", "interactive", "video", "music":
	default:
		return errors.New("unknown preset")
	}
	if c.Bitrate != 128 && c.Bitrate != 192 && c.Bitrate != 256 && c.Bitrate != 320 {
		return errors.New("encoding bitrate must be 128, 192, 256 or 320 kbps")
	}
	return nil
}

// RestartRequired compares only settings consumed by the active transport.
// Volume and descriptive preset metadata can change in an existing session.
func (c Config) RestartRequired(next Config) bool {
	if c.Enabled != next.Enabled || c.DeviceID != next.DeviceID || c.Host != next.Host || c.Port != next.Port || c.Codec != next.Codec || c.Bitrate != next.Bitrate {
		return true
	}
	switch c.Codec {
	case "cast-opus":
		return c.TargetDelayMS != next.TargetDelayMS
	case "aac-hls":
		return c.SegmentMS != next.SegmentMS || c.MediaPort != next.MediaPort
	default:
		return c.MediaPort != next.MediaPort
	}
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
	switch c.Preset {
	case "interactive", "video", "music":
		// Keep existing transport values; old labels do not match new presets.
		c.Preset = "custom"
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
