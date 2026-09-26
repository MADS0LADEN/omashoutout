package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/lkarlslund/shoutout/internal/config"
	"github.com/lkarlslund/shoutout/internal/service"
)

func TestPrivateSocketConfiguration(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	l, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	path, _ := Path()
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions: %v %v", st, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := service.New(config.Default(), filepath.Join(runtime, "config.json"))
	done := make(chan error, 1)
	go func() { done <- Run(ctx, s, l) }()
	for _, level := range []float64{0.01, 0.5, 1} {
		c := config.Default()
		c.ReceiverVolume = level
		r, err := Call(Request{Method: "configure", Config: &c})
		if err != nil || r.Config.ReceiverVolume != level {
			t.Fatalf("configured %v: %+v %v", level, r, err)
		}
	}
	c := config.Default()
	c.ReceiverVolume = 1.01
	if _, err = Call(Request{Method: "configure", Config: &c}); err == nil {
		t.Fatal("invalid protocol volume accepted")
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	json.NewEncoder(conn).Encode(map[string]any{"method": "unknown"})
	var response Response
	json.NewDecoder(conn).Decode(&response)
	conn.Close()
	if response.Error == "" {
		t.Fatal("unknown method accepted")
	}
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
