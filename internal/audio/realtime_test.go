package audio

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lkarlslund/shoutout/internal/cast"
	"github.com/lkarlslund/shoutout/internal/config"
)

func TestRealtimeSyntheticAudio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg not installed")
	}
	encoders, err := exec.Command(ffmpeg, "-hide_banner", "-encoders").CombinedOutput()
	if err != nil || !bytes.Contains(encoders, []byte("libopus")) {
		t.Skip("Opus encoder not installed")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(ffmpeg, "'", "'\"'\"'") + "' -hide_banner -loglevel error -re -f lavfi -i anullsrc=r=48000:cl=stereo -f f32le pipe:1\n"
	if err = os.WriteFile(filepath.Join(dir, "parec"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session := cast.StreamingSession{SenderSSRC: 42, ReceiverSSRC: 43, Port: receiver.LocalAddr().(*net.UDPAddr).Port, DelayMS: 400}
	s, err := NewRealtime(ctx, config.Default(), "127.0.0.1", "127.0.0.1", session)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	receiver.SetReadDeadline(time.Now().Add(4 * time.Second))
	buf := make([]byte, 1500)
	frames := 0
	reports := 0
	block, _ := aes.NewCipher(session.Key[:])
	for frames < 12 {
		n, peer, err := receiver.ReadFromUDP(buf)
		if err != nil {
			select {
			case pipeline := <-s.Done:
				t.Fatalf("pipeline: %v", pipeline)
			default:
				t.Fatal(err)
			}
		}
		p := buf[:n]
		if len(p) == 28 && p[1] == 200 {
			reports++
			continue
		}
		if len(p) < 19 || p[1] != 255 {
			t.Fatalf("invalid audio packet %x", p)
		}
		var iv [16]byte
		binary.BigEndian.PutUint32(iv[8:12], uint32(p[13]))
		plain := make([]byte, n-19)
		cipher.NewCTR(block, iv[:]).XORKeyStream(plain, p[19:])
		if opusSamples(plain) != 960 {
			t.Fatal("invalid encrypted Opus frame")
		}
		ack := make([]byte, 20)
		ack[0] = 0x8f
		ack[1] = 206
		binary.BigEndian.PutUint16(ack[2:4], 4)
		binary.BigEndian.PutUint32(ack[4:8], 43)
		binary.BigEndian.PutUint32(ack[8:12], 42)
		copy(ack[12:], "CAST")
		ack[16] = p[13]
		binary.BigEndian.PutUint16(ack[18:], 400)
		if _, err = receiver.WriteToUDP(ack, peer); err != nil {
			t.Fatal(err)
		}
		frames++
	}
	if reports == 0 || s.RealtimeStats().Feedback < 1 {
		t.Fatal("missing timing or acknowledgement exchange")
	}
}
func TestOpusFrameDurations(t *testing.T) {
	for _, c := range []struct {
		p    []byte
		want int
	}{{nil, 0}, {[]byte{0x98}, 960}, {[]byte{0xf8}, 960}, {[]byte{0xf9}, 1920}, {[]byte{0xfb}, 0}, {[]byte{0xfb, 3}, 2880}} {
		if got := opusSamples(c.p); got != c.want {
			t.Fatalf("%x: got %d want %d", c.p, got, c.want)
		}
	}
}
func FuzzOgg(f *testing.F) {
	f.Add([]byte("OggS"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			t.Skip()
		}
		readOpus(bytes.NewReader(b), func([]byte) error { return io.EOF })
	})
}
