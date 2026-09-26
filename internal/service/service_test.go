package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lkarlslund/shoutout/internal/config"
)

func TestNoIndependentMute(t *testing.T) {
	c := config.Default()
	s := New(c, filepath.Join(t.TempDir(), "config.json"))
	if s.Config() != c {
		t.Fatal("service changed user settings on restart")
	}
}
func TestUpdateCancelsCurrentSession(t *testing.T) {
	c := config.Default()
	s := New(c, filepath.Join(t.TempDir(), "config.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sessionCancel = cancel
	c.ReceiverVolume = 1.01
	if s.Update(c) == nil {
		t.Fatal("accepted unsafe configuration")
	}
	if ctx.Err() != nil {
		t.Fatal("invalid update interrupted current session")
	}
	c.ReceiverVolume = 0.02
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("volume update interrupted the current session")
	}
	c.Host = "192.0.2.1"
	if err := s.Update(c); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("destination update did not cancel old session")
	}
	if s.Config() != c {
		t.Fatal("configuration not applied")
	}
}
func TestConcurrentStatusAndUpdates(t *testing.T) {
	s := New(config.Default(), filepath.Join(t.TempDir(), "config.json"))
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				s.Status()
				c := s.Config()
				if err := s.Update(c); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestLiveUpdatesPreserveSession(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*config.Config)
	}{
		{"unchanged", func(c *config.Config) {}},
		{"volume", func(c *config.Config) { c.ReceiverVolume = .25 }},
		{"preset_label", func(c *config.Config) { c.Preset = "custom" }},
		{"inactive_segment_length", func(c *config.Config) { c.SegmentMS = 1000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := config.Default()
			s := New(c, filepath.Join(t.TempDir(), "config.json"))
			s.status.State = "streaming"
			s.status.PlayerState = "PLAYING"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.sessionCancel = cancel
			tc.change(&c)
			if err := s.Update(c); err != nil {
				t.Fatal(err)
			}
			if ctx.Err() != nil || s.Status().State != "streaming" || s.Status().PlayerState != "PLAYING" {
				t.Fatal("live edit disrupted playback")
			}
			saved, err := config.Load(s.path)
			if err != nil || saved != c {
				t.Fatal("live settings not persisted")
			}
		})
	}
}
