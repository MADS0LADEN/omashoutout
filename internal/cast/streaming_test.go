package cast

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestAudioNegotiation(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		valid        bool
	}{
		{"accepted", `{"type":"ANSWER","result":"ok","answer":{"udpPort":12345,"sendIndexes":[0],"ssrcs":[42]}}`, true},
		{"wrong_stream", `{"type":"ANSWER","result":"ok","answer":{"udpPort":12345,"sendIndexes":[1],"ssrcs":[42]}}`, false},
		{"bad_port", `{"type":"ANSWER","result":"ok","answer":{"udpPort":0,"sendIndexes":[0],"ssrcs":[42]}}`, false},
		{"unsupported_rate", `{"type":"ANSWER","result":"ok","answer":{"udpPort":12345,"sendIndexes":[0],"ssrcs":[42],"constraints":{"audio":{"maxSampleRate":24000}}}}`, false},
		{"rejected", `{"type":"ANSWER","result":"error","error":{"code":-2,"description":"unsupported audio"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := net.Pipe()
			c := &Client{conn: left, pending: make(map[int]chan response), done: make(chan struct{})}
			c.workers.Add(1)
			go c.readLoop()
			defer c.Close()
			checked := make(chan bool, 1)
			go func() {
				defer right.Close()
				e, err := readEnvelope(right)
				if err != nil {
					checked <- false
					return
				}
				var p map[string]any
				if json.Unmarshal(e.Payload, &p) != nil {
					checked <- false
					return
				}
				offer, ok := p["offer"].(map[string]any)
				if !ok {
					checked <- false
					return
				}
				streams, ok := offer["supportedStreams"].([]any)
				if !ok || len(streams) != 1 {
					checked <- false
					return
				}
				stream := streams[0].(map[string]any)
				valid := e.Namespace == streamingNS && p["seqNum"] != nil && stream["codecName"] == "opus" && stream["sampleRate"] == float64(48000) && stream["targetDelay"] == float64(400) && stream["ssrc"].(float64) <= 2147483647 && len(stream["aesKey"].(string)) == 32 && len(stream["aesIvMask"].(string)) == 32
				checked <- valid
				var answer map[string]any
				json.Unmarshal([]byte(tc.answer), &answer)
				answer["seqNum"] = p["seqNum"]
				payload, _ := json.Marshal(answer)
				b := encode(envelope{Source: "app", Destination: sender, Namespace: streamingNS, Payload: payload})
				right.SetWriteDeadline(time.Now().Add(time.Second))
				right.Write(append(binary.BigEndian.AppendUint32(nil, uint32(len(b))), b...))
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			s, err := c.NegotiateAudio(ctx, Application{TransportID: "app"}, 128, 400)
			if (err == nil) != tc.valid {
				t.Fatalf("success=%t, err=%v", tc.valid, err)
			}
			if tc.valid && (s.Port != 12345 || s.ReceiverSSRC != 42) {
				t.Fatal("wrong negotiated endpoint")
			}
			if !<-checked {
				t.Fatal("offer missing required audio parameters")
			}
		})
	}
}
