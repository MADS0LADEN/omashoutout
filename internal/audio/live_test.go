package audio

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lkarlslund/shoutout/internal/config"
)

// Synthetic silence bypasses the desktop audio server and never contacts a receiver.
func TestLiveStream(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg required for encoder integration test")
	}
	dir := t.TempDir()
	capture := "#!/bin/sh\nexec '" + strings.ReplaceAll(ffmpeg, "'", "'\"'\"'") + "' -hide_banner -loglevel error -re -f lavfi -i anullsrc=r=48000:cl=stereo -f f32le pipe:1\n"
	if err := os.WriteFile(filepath.Join(dir, "parec"), []byte(capture), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_RUNTIME_DIR", dir)
	c := config.Default()
	c.Codec = "aac-hls"
	c.MediaPort = 0
	c.SegmentMS = 250
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := NewStream(ctx, c, "127.0.0.1", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	request := func(url, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", url, nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, r)
		return w
	}
	w := request(s.URL, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("manifest HTTP %d", w.Code)
	}
	manifest, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(manifest), "#EXT-X-TARGETDURATION:1") {
		t.Fatalf("invalid target duration: %s", manifest)
	}
	var segment string
	for _, line := range strings.Split(string(manifest), "\n") {
		if strings.HasPrefix(line, "segment-") {
			segment = line
			break
		}
	}
	base := strings.TrimSuffix(s.URL, "index.m3u8")
	if w = request(base+segment, "127.0.0.1:1234"); w.Code != 200 || w.Body.Len() == 0 {
		t.Fatal("missing live segment")
	}
	if w = request(s.URL, "127.0.0.2:1234"); w.Code != 403 {
		t.Fatal("non-receiver allowed")
	}
	if w = request(base+"secret.txt", "127.0.0.1:1234"); w.Code != 404 {
		t.Fatal("unexpected file exposed")
	}
	if s.Bytes.Load() == 0 || s.Subscribers() != 1 {
		t.Fatal("missing transfer status")
	}
}
