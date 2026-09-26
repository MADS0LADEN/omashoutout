package cast

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"net"
	"testing"
	"time"
)

func TestUnsafeVolumeNeverWrites(t *testing.T) {
	c := &Client{}
	for _, v := range []float64{-1, 1.000001, 2, math.NaN(), math.Inf(1)} {
		if c.SetVolume(context.Background(), v, false) == nil {
			t.Fatalf("accepted volume %v", v)
		}
	}
}
func TestVolumeRequiresReceiverConfirmation(t *testing.T) {
	for _, reported := range []float64{0.01, 0.9} {
		t.Run(string(rune('a'+int(reported*10))), func(t *testing.T) {
			left, right := net.Pipe()
			c := &Client{conn: left, pending: make(map[int]chan response), done: make(chan struct{})}
			c.workers.Add(1)
			go c.readLoop()
			defer c.Close()
			go func() {
				defer right.Close()
				for i := 0; i < 4; i++ {
					e, err := readEnvelope(right)
					if err != nil {
						return
					}
					var p map[string]any
					if json.Unmarshal(e.Payload, &p) != nil {
						return
					}
					payload, _ := json.Marshal(map[string]any{"type": "RECEIVER_STATUS", "requestId": p["requestId"], "status": ReceiverStatus{Volume: Volume{Level: reported, Muted: true}}})
					b := encode(envelope{Source: receiver, Destination: sender, Namespace: receiverNS, Payload: payload})
					right.SetWriteDeadline(time.Now().Add(time.Second))
					right.Write(append(binary.BigEndian.AppendUint32(nil, uint32(len(b))), b...))
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := c.SetVolume(ctx, 0.01, true)
			if reported <= 0.01 && err != nil {
				t.Fatal(err)
			}
			if reported > 0.05 && err == nil {
				t.Fatal("unsafe receiver accepted")
			}
		})
	}
}
func TestFraming(t *testing.T) {
	want := envelope{Source: "sender", Destination: "receiver", Namespace: "media", Payload: []byte(`{"type":"PING"}`)}
	b := encode(want)
	got, err := decode(b)
	if err != nil || got.Source != want.Source || !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	for _, bad := range [][]byte{{}, {0, 0, 0, 0}, {0xff, 0xff, 0xff, 0xff}, {0, 0, 0, 20, 8}} {
		if _, err = readEnvelope(bytes.NewReader(bad)); err == nil {
			t.Fatal("accepted malformed frame")
		}
	}
}
func FuzzDecode(f *testing.F) {
	f.Add(encode(envelope{Source: "a", Destination: "b", Namespace: "c", Payload: []byte("{}")}))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxFrame {
			t.Skip()
		}
		decode(b)
	})
}

func TestLiveVolumeDoesNotRestartOrMute(t *testing.T) {
	left, right := net.Pipe()
	c := &Client{conn: left, pending: make(map[int]chan response), done: make(chan struct{})}
	c.workers.Add(1)
	go c.readLoop()
	defer c.Close()
	valid := make(chan bool, 1)
	go func() {
		defer right.Close()
		ok := true
		for i := 0; i < 2; i++ {
			e, err := readEnvelope(right)
			if err != nil {
				valid <- false
				return
			}
			var p map[string]any
			if json.Unmarshal(e.Payload, &p) != nil {
				valid <- false
				return
			}
			if i == 0 {
				v, good := p["volume"].(map[string]any)
				ok = ok && good && p["type"] == "SET_VOLUME" && len(v) == 1 && v["level"] != nil
			} else {
				ok = ok && p["type"] == "GET_STATUS"
			}
			payload, _ := json.Marshal(map[string]any{"type": "RECEIVER_STATUS", "requestId": p["requestId"], "status": ReceiverStatus{Volume: Volume{Level: .02, Muted: false}}})
			b := encode(envelope{Source: receiver, Destination: sender, Namespace: receiverNS, Payload: payload})
			right.SetWriteDeadline(time.Now().Add(time.Second))
			right.Write(append(binary.BigEndian.AppendUint32(nil, uint32(len(b))), b...))
		}
		valid <- ok
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.SetLiveVolume(ctx, .02, false); err != nil {
		t.Fatal(err)
	}
	if !<-valid {
		t.Fatal("live volume emitted disruptive commands")
	}
}
