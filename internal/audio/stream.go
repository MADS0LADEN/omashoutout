package audio

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MADS0LADEN/omashoutout/internal/cast"
	"github.com/MADS0LADEN/omashoutout/internal/config"
)

// Capture batching and the compatibility stream queue are implementation
// limits. Playback buffering is controlled separately at the receiver.
const captureLatencyMS = 40
const mp3QueueMS = 200

type Stream struct {
	realtime         *cast.Transport
	ContentType      string
	directory        string
	lastRequest      atomic.Int64
	mu               sync.Mutex
	subscribers      map[chan []byte]struct{}
	queue            int
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	server           *http.Server
	capture, encoder *exec.Cmd
	Done             chan error
	URL              string
	Allowed          atomic.Bool
	Bytes            atomic.Uint64
	Requests         atomic.Uint64
	Peak             atomic.Uint64
	closed           bool
}

func NewStream(parent context.Context, c config.Config, localIP, receiverIP string) (*Stream, error) {
	ctx, cancel := context.WithCancel(parent)
	started := false
	s := &Stream{cancel: cancel, subscribers: make(map[chan []byte]struct{}), Done: make(chan error, 4), queue: max(2, c.Bitrate*125*mp3QueueMS/1000/1024)}
	defer func() {
		if !started && s.directory != "" {
			os.RemoveAll(s.directory)
		}
	}()
	listener, err := net.Listen("tcp", net.JoinHostPort(localIP, strconv.Itoa(c.MediaPort)))
	if err != nil {
		cancel()
		return nil, err
	}
	var token [24]byte
	if _, err = rand.Read(token[:]); err != nil {
		listener.Close()
		cancel()
		return nil, err
	}
	path := "/audio/" + hex.EncodeToString(token[:]) + ".mp3"
	s.URL = "http://" + listener.Addr().String() + path
	s.ContentType = "audio/mpeg"
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host != receiverIP {
			http.Error(w, "receiver only", http.StatusForbidden)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == "HEAD" {
			return
		}
		ch := make(chan []byte, s.queue)
		s.mu.Lock()
		if s.closed || len(s.subscribers) >= 3 {
			s.mu.Unlock()
			http.Error(w, "stream busy", 503)
			return
		}
		s.subscribers[ch] = struct{}{}
		s.mu.Unlock()
		defer func() { s.mu.Lock(); delete(s.subscribers, ch); s.mu.Unlock() }()
		s.Requests.Add(1)
		rc := http.NewResponseController(w)
		if err := rc.Flush(); err != nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.Context().Done():
				return
			case data, ok := <-ch:
				if !ok {
					return
				}
				if rc.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil {
					return
				}
				if _, err := w.Write(data); err != nil {
					return
				}
				if rc.Flush() != nil {
					return
				}
			}
		}
	})
	if c.Codec == "aac-hls" {
		s.directory, err = os.MkdirTemp(os.Getenv("XDG_RUNTIME_DIR"), "omashoutout-stream-")
		if err != nil {
			listener.Close()
			cancel()
			return nil, err
		}
		prefix := "/audio/" + hex.EncodeToString(token[:]) + "/"
		s.URL = "http://" + listener.Addr().String() + prefix + "index.m3u8"
		s.ContentType = "application/x-mpegURL"
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if host != receiverIP {
				http.Error(w, "receiver only", 403)
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" {
				w.WriteHeader(405)
				return
			}
			name := strings.TrimPrefix(r.URL.Path, prefix)
			if name != filepath.Base(name) || (name != "index.m3u8" && (!strings.HasPrefix(name, "segment-") || !strings.HasSuffix(name, ".ts"))) {
				http.NotFound(w, r)
				return
			}
			b, err := os.ReadFile(filepath.Join(s.directory, name))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if name == "index.m3u8" {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
				b = []byte(strings.ReplaceAll(string(b), "#EXT-X-TARGETDURATION:0", "#EXT-X-TARGETDURATION:1"))
			} else {
				w.Header().Set("Content-Type", "video/mp2t")
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cache-Control", "no-store")
			http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
			s.Requests.Add(1)
			s.lastRequest.Store(time.Now().UnixNano())
			if r.Method == "GET" {
				n, _ := w.Write(b)
				if name != "index.m3u8" {
					s.Bytes.Add(uint64(n))
				}
			}
		})
	}
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	s.capture = exec.CommandContext(ctx, "parec", "--device="+SinkName+".monitor", "--format=float32le", "--rate=48000", "--channels=2", "--latency-msec="+strconv.Itoa(captureLatencyMS), "--property=application.name=Omashoutout", "--property=node.dont-reconnect=true", "--property=node.virtual=true", "--property=media.role=filter")
	captured, err := s.capture.StdoutPipe()
	if err != nil {
		listener.Close()
		cancel()
		return nil, err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-probesize", "32", "-analyzeduration", "0", "-f", "f32le", "-ar", "48000", "-ac", "2", "-i", "pipe:0"}
	if c.Codec == "aac-hls" {
		args = append(args, "-c:a", "aac", "-b:a", strconv.Itoa(c.Bitrate)+"k", "-flush_packets", "1", "-f", "hls", "-hls_time", strconv.FormatFloat(float64(c.SegmentMS)/1000, 'f', 3, 64), "-hls_list_size", "6", "-hls_delete_threshold", "3", "-hls_flags", "delete_segments+temp_file+omit_endlist", "-hls_segment_filename", filepath.Join(s.directory, "segment-%09d.ts"), filepath.Join(s.directory, "index.m3u8"))
	} else {
		args = append(args, "-c:a", "libmp3lame", "-b:a", strconv.Itoa(c.Bitrate)+"k", "-f", "mp3", "-write_xing", "0", "-id3v2_version", "0", "-flush_packets", "1", "pipe:1")
	}
	s.encoder = exec.CommandContext(ctx, "ffmpeg", args...)

	input, err := s.encoder.StdinPipe()
	if err != nil {
		listener.Close()
		cancel()
		return nil, err
	}
	output, err := s.encoder.StdoutPipe()
	if err != nil {
		input.Close()
		listener.Close()
		cancel()
		return nil, err
	}
	if err = s.encoder.Start(); err != nil {
		input.Close()
		listener.Close()
		cancel()
		return nil, fmt.Errorf("start encoder: %w", err)
	}
	if err = s.capture.Start(); err != nil {
		input.Close()
		listener.Close()
		cancel()
		s.encoder.Wait()
		return nil, fmt.Errorf("start capture: %w", err)
	}
	s.wg.Add(4)
	go func() {
		defer s.wg.Done()
		err := s.server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			s.report(err)
		}
	}()
	go func() { defer s.wg.Done(); defer input.Close(); s.report(s.process(captured, input)) }()
	go func() {
		defer s.wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := output.Read(buf)
			if n > 0 {
				s.publish(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				s.report(err)
				return
			}
		}
	}()
	go func() { defer s.wg.Done(); <-ctx.Done(); s.server.Close() }()
	started = true
	return s, nil
}
func (s *Stream) report(err error) {
	if err != nil {
		select {
		case s.Done <- err:
		default:
		}
	}
}
func (s *Stream) publish(data []byte) {
	s.Bytes.Add(uint64(len(data)))
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- data:
		default:
			close(ch)
			delete(s.subscribers, ch)
		}
	}
}
func boundPCM(b []byte, gain float64) float64 {
	peak := 0.0
	for i := 0; i+4 <= len(b); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:])))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		v = math.Max(-1, math.Min(1, v)) * gain
		peak = math.Max(peak, math.Abs(v))
		binary.LittleEndian.PutUint32(b[i:], math.Float32bits(float32(v)))
	}
	return peak
}

func (s *Stream) process(in io.Reader, out io.Writer) error {
	chunk := 3840
	if s.realtime != nil {
		chunk = 1920
	}
	buf := make([]byte, chunk)
	ramp := 0.0
	for {
		n, err := io.ReadFull(in, buf)
		if err != nil {
			return err
		}
		factor := 0.0
		if s.Allowed.Load() {
			ramp = math.Min(1, ramp+0.05*float64(chunk)/3840)
			factor = ramp
		} else {
			ramp = 0
		}
		peak := boundPCM(buf[:n], factor)
		s.Peak.Store(math.Float64bits(peak))
		if _, err = out.Write(buf[:n]); err != nil {
			return err
		}
	}
}
func (s *Stream) Close() {
	s.Allowed.Store(false)
	s.cancel()
	if s.server != nil {
		s.server.Close()
	}
	s.wg.Wait()
	s.capture.Wait()
	s.encoder.Wait()
	if s.directory != "" {
		os.RemoveAll(s.directory)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
}
func (s *Stream) Subscribers() int {
	if s.realtime != nil {
		if s.realtime.Stats().Feedback > 0 {
			return 1
		}
		return 0
	}
	if s.directory != "" {
		if time.Since(time.Unix(0, s.lastRequest.Load())) < 5*time.Second {
			return 1
		}
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subscribers)
}

// WaitReady ensures live media exists before the receiver is asked to load it.
func (s *Stream) WaitReady(ctx context.Context) error {
	if s.directory == "" {
		return nil
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.Done:
			return err
		case <-timeout.C:
			return errors.New("live encoder did not produce a playlist")
		case <-ticker.C:
			b, err := os.ReadFile(filepath.Join(s.directory, "index.m3u8"))
			if err == nil && strings.Count(string(b), "#EXTINF:") >= 3 {
				return nil
			}
		}
	}
}
