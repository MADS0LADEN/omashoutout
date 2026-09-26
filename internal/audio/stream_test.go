package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

func TestAttenuationBounds(t *testing.T) {
	values := []float32{-2, -1, -0.5, 0, 0.5, 1, 2, float32(math.NaN()), float32(math.Inf(1))}
	b := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	peak := attenuate(b, 0.01)
	if math.Abs(peak-0.01) > 1e-8 {
		t.Fatalf("peak %g", peak)
	}
	for i := range values {
		v := math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
		if math.IsNaN(float64(v)) || math.Abs(float64(v)) > 0.010001 {
			t.Fatalf("unbounded sample %g", v)
		}
	}
	attenuate(b, 0)
	for i := range values {
		if binary.LittleEndian.Uint32(b[i*4:])&0x7fffffff != 0 {
			t.Fatal("mute left nonzero sample")
		}
	}
}
func TestSlowSubscriberDisconnected(t *testing.T) {
	s := Stream{subscribers: map[chan []byte]struct{}{}}
	slow := make(chan []byte, 1)
	fast := make(chan []byte, 2)
	s.subscribers[slow] = struct{}{}
	s.subscribers[fast] = struct{}{}
	s.publish([]byte{1})
	s.publish([]byte{2})
	if s.Subscribers() != 1 {
		t.Fatal("slow reader retained")
	}
	<-slow
	if _, ok := <-slow; ok {
		t.Fatal("slow stream still open")
	}
	if len(fast) != 2 {
		t.Fatal("fast reader blocked")
	}
}

type callbackWriter func([]byte) (int, error)

func (w callbackWriter) Write(b []byte) (int, error) { return w(b) }
func TestAttenuationChangesDuringCapture(t *testing.T) {
	s := &Stream{}
	s.SetTrimDB(0)
	s.Allowed.Store(true)
	pcm := make([]byte, 3840*40)
	for i := 0; i < len(pcm); i += 4 {
		binary.LittleEndian.PutUint32(pcm[i:], math.Float32bits(.5))
	}
	count := 0
	err := s.process(bytes.NewReader(pcm), callbackWriter(func(b []byte) (int, error) {
		count++
		if count == 20 {
			s.SetTrimDB(-20)
		}
		if count == 40 && math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))-.05) > 1e-6 {
			t.Fatal("new gain did not reach running capture")
		}
		return len(b), nil
	}))
	if err != io.EOF || count != 40 {
		t.Fatalf("capture ended early: %d %v", count, err)
	}
}
