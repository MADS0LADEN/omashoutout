package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/lkarlslund/shoutout/internal/cast"
	"github.com/lkarlslund/shoutout/internal/config"
)

const realtimeCaptureLatencyMS = 10

// NewRealtime starts capture only after the receiver has negotiated its UDP
// endpoint. PCM stays gated until the service verifies the receiver volume.
func NewRealtime(parent context.Context, c config.Config, localIP, remoteIP string, session cast.StreamingSession) (*Stream, error) {
	ctx, cancel := context.WithCancel(parent)
	transport, err := cast.NewTransport(localIP, remoteIP, session)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &Stream{cancel: cancel, Done: make(chan error, 4), realtime: transport}
	started := false
	defer func() {
		if !started {
			cancel()
			transport.Close()
			if s.capture != nil && s.capture.Process != nil {
				s.capture.Wait()
			}
			if s.encoder != nil && s.encoder.Process != nil {
				s.encoder.Wait()
			}
		}
	}()
	s.capture = exec.CommandContext(ctx, "parec", "--device="+SinkName+".monitor", "--format=float32le", "--rate=48000", "--channels=2", "--latency-msec="+strconv.Itoa(realtimeCaptureLatencyMS), "--property=application.name=ShoutOut", "--property=node.dont-reconnect=true", "--property=node.virtual=true", "--property=media.role=filter")
	captured, err := s.capture.StdoutPipe()
	if err != nil {
		return nil, err
	}
	s.encoder = exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-probesize", "32", "-analyzeduration", "0", "-f", "wav", "-max_size", "3840", "-ignore_length", "1", "-i", "pipe:0", "-c:a", "libopus", "-application", "lowdelay", "-frame_duration", "20", "-b:a", strconv.Itoa(c.Bitrate)+"k", "-vbr", "off", "-f", "ogg", "-page_duration", "20000", "-flush_packets", "1", "pipe:1")
	input, err := s.encoder.StdinPipe()
	if err != nil {
		captured.Close()
		return nil, err
	}
	output, err := s.encoder.StdoutPipe()
	if err != nil {
		captured.Close()
		input.Close()
		return nil, err
	}
	if err = s.encoder.Start(); err != nil {
		captured.Close()
		input.Close()
		output.Close()
		return nil, fmt.Errorf("start Opus encoder: %w", err)
	}
	// Bound the demuxer's input packets to 10 ms. Raw PCM input can collect
	// much larger batches even when the encoder emits short Opus frames.
	if err = writeFloatPCMHeader(input); err != nil {
		captured.Close()
		input.Close()
		return nil, fmt.Errorf("write encoder input header: %w", err)
	}
	if err = s.capture.Start(); err != nil {
		captured.Close()
		input.Close()
		return nil, fmt.Errorf("start capture: %w", err)
	}
	s.wg.Add(4)
	go func() { defer s.wg.Done(); defer input.Close(); s.report(s.process(captured, input)) }()
	go func() {
		defer s.wg.Done()
		s.report(readOpus(output, func(packet []byte) error {
			if err := transport.SendFrame(ctx, packet); err != nil {
				return err
			}
			s.Bytes.Add(uint64(len(packet)))
			return nil
		}))
	}()
	go func() { defer s.wg.Done(); s.report(transport.Receive(ctx)) }()
	go func() { defer s.wg.Done(); <-ctx.Done(); transport.Close() }()
	started = true
	return s, nil
}

func (s *Stream) RealtimeStats() cast.TransportStats {
	if s.realtime == nil {
		return cast.TransportStats{Acknowledged: -1}
	}
	return s.realtime.Stats()
}

// readOpus extracts complete packets from the encoder's Ogg stream, including
// packets split across pages. No container headers are sent to the receiver.
func readOpus(r io.Reader, consume func([]byte) error) error {
	var pending []byte
	var serial, sequence uint32
	headers := 0
	first := true
	for {
		var header [27]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return err
		}
		if string(header[:4]) != "OggS" || header[4] != 0 {
			return errors.New("invalid Ogg page")
		}
		pageSerial := binary.LittleEndian.Uint32(header[14:18])
		pageSequence := binary.LittleEndian.Uint32(header[18:22])
		if !first && (pageSerial != serial || pageSequence != sequence+1) {
			return errors.New("discontinuous Ogg stream")
		}
		first = false
		serial = pageSerial
		sequence = pageSequence
		if (header[5]&1 != 0) != (len(pending) > 0) {
			return errors.New("invalid Ogg continuation")
		}
		sizes := make([]byte, int(header[26]))
		if _, err := io.ReadFull(r, sizes); err != nil {
			return err
		}
		for _, size := range sizes {
			if len(pending)+int(size) > 16384 {
				return errors.New("oversized Opus packet")
			}
			start := len(pending)
			pending = append(pending, make([]byte, int(size))...)
			if _, err := io.ReadFull(r, pending[start:]); err != nil {
				return err
			}
			if size == 255 {
				continue
			}
			switch headers {
			case 0:
				if len(pending) < 19 || string(pending[:8]) != "OpusHead" || pending[9] != 2 {
					return errors.New("expected stereo Opus header")
				}
				headers++
			case 1:
				if len(pending) < 8 || string(pending[:8]) != "OpusTags" {
					return errors.New("expected Opus metadata")
				}
				headers++
			default:
				if opusSamples(pending) != 960 {
					return errors.New("encoder must produce 20 ms Opus frames")
				}
				if err := consume(pending); err != nil {
					return err
				}
			}
			pending = pending[:0]
		}
	}
}
func opusSamples(p []byte) int {
	if len(p) == 0 {
		return 0
	}
	config := p[0] >> 3
	samples := 0
	switch {
	case config < 12:
		samples = []int{480, 960, 1920, 2880}[config&3]
	case config < 16:
		samples = []int{480, 960}[config&1]
	default:
		samples = []int{120, 240, 480, 960}[config&3]
	}
	count := 1
	switch p[0] & 3 {
	case 1, 2:
		count = 2
	case 3:
		if len(p) < 2 {
			return 0
		}
		count = int(p[1] & 63)
	}
	return samples * count
}

// writeFloatPCMHeader describes an indefinite stereo float32/48 kHz WAV stream.
// The encoder's ignore_length option allows playback beyond the RIFF size limit.
func writeFloatPCMHeader(w io.Writer) error {
	var h [44]byte
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], ^uint32(0))
	copy(h[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16)
	binary.LittleEndian.PutUint16(h[20:22], 3)
	binary.LittleEndian.PutUint16(h[22:24], 2)
	binary.LittleEndian.PutUint32(h[24:28], 48000)
	binary.LittleEndian.PutUint32(h[28:32], 48000*2*4)
	binary.LittleEndian.PutUint16(h[32:34], 2*4)
	binary.LittleEndian.PutUint16(h[34:36], 32)
	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], ^uint32(0))
	_, err := w.Write(h[:])
	return err
}
