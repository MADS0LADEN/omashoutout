package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lkarlslund/shoutout/internal/config"
)

func TestRestartAlwaysMuted(t *testing.T) {
	c := config.Default()
	c.Muted = false
	s := New(c, filepath.Join(t.TempDir(), "config.json"))
	if !s.Config().Muted {
		t.Fatal("startup did not enforce mute")
	}
}
func TestUpdateCancelsCurrentSession(t *testing.T) {
	c := config.Default()
	s := New(c, filepath.Join(t.TempDir(), "config.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sessionCancel = cancel
	c.ReceiverVolume = 0.06
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
	if ctx.Err() == nil {
		t.Fatal("valid update did not cancel old session")
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
