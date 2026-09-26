package config

import (
	"math"
	"path/filepath"
	"testing"
)

func TestReceiverProtocolRange(t *testing.T) {
	for _, v := range []float64{-0.01, 1.000001, 2, math.NaN(), math.Inf(1)} {
		c := Default()
		c.ReceiverVolume = v
		if c.Validate() == nil {
			t.Errorf("accepted unsafe volume %v", v)
		}
	}
	for _, v := range []float64{0, 0.01, 0.05, 0.5, 1} {
		c := Default()
		c.ReceiverVolume = v
		if err := c.Validate(); err != nil {
			t.Errorf("rejected %v: %v", v, err)
		}
	}
}
func TestPersistenceAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReceiverVolume != 0.01 || c.TrimDB != 0 {
		t.Fatalf("unsafe defaults: %+v", c)
	}
	c.Host = "192.0.2.1"
	if err = Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != c {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	c.ReceiverVolume = 1.01
	if Save(path, c) == nil {
		t.Fatal("saved unsafe config")
	}
	got, _ = Load(path)
	if got.ReceiverVolume != 0.01 {
		t.Fatal("invalid write replaced safe config")
	}
}
