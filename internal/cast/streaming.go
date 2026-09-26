package cast

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// StreamingSession holds the negotiated parameters for one encrypted audio stream.
// Key material is transient and must never be included in status or logs.
type StreamingSession struct {
	SenderSSRC, ReceiverSSRC uint32
	Port                     int
	Key, IV                  [16]byte
	DelayMS                  int
}

func (c *Client) NegotiateAudio(ctx context.Context, app Application, bitrate, delay int) (StreamingSession, error) {
	s := StreamingSession{DelayMS: delay}
	var random [36]byte
	if _, err := rand.Read(random[:]); err != nil {
		return s, err
	}
	// Older receivers reject SSRC values above the signed 32-bit range.
	s.SenderSSRC = (binary.BigEndian.Uint32(random[:4]) & 0x7fffffff) | 1
	copy(s.Key[:], random[4:20])
	copy(s.IV[:], random[20:])
	stream := map[string]any{"index": 0, "type": "audio_source", "codecName": "opus", "codecParameter": "", "sampleRate": 48000, "rtpProfile": "cast", "rtpPayloadType": 127, "ssrc": s.SenderSSRC, "targetDelay": delay, "channels": 2, "bitRate": bitrate * 1000, "timeBase": "1/48000", "aesKey": hex.EncodeToString(s.Key[:]), "aesIvMask": hex.EncodeToString(s.IV[:]), "receiverRtcpEventLog": false}
	r, err := c.request(ctx, app.TransportID, streamingNS, map[string]any{"type": "OFFER", "offer": map[string]any{"castMode": "mirroring", "supportedStreams": []any{stream}}})
	if err != nil {
		return s, err
	}
	var reply struct {
		Type, Result string
		Error        struct {
			Code        int
			Description string
		}
		Answer struct {
			UDPPort     int      `json:"udpPort"`
			SendIndexes []int    `json:"sendIndexes"`
			SSRCs       []uint32 `json:"ssrcs"`
			Constraints struct {
				Audio struct{ MaxSampleRate, MaxChannels, MaxBitRate, MaxDelay int }
			}
		}
	}
	if err = json.Unmarshal(r.Raw, &reply); err != nil {
		return s, err
	}
	if reply.Type != "ANSWER" || reply.Result != "ok" {
		return s, fmt.Errorf("audio negotiation rejected: %s (%d)", reply.Error.Description, reply.Error.Code)
	}
	a := reply.Answer
	if a.UDPPort < 1 || a.UDPPort > 65535 || len(a.SendIndexes) != 1 || a.SendIndexes[0] != 0 || len(a.SSRCs) != 1 || a.SSRCs[0] == s.SenderSSRC {
		return s, fmt.Errorf("invalid negotiated audio endpoint")
	}
	limits := a.Constraints.Audio
	if (limits.MaxSampleRate > 0 && limits.MaxSampleRate < 48000) || (limits.MaxChannels > 0 && limits.MaxChannels < 2) || (limits.MaxBitRate > 0 && limits.MaxBitRate < bitrate*1000) || (limits.MaxDelay > 0 && limits.MaxDelay < delay) {
		return s, fmt.Errorf("receiver cannot support requested audio settings")
	}
	s.Port = a.UDPPort
	s.ReceiverSSRC = a.SSRCs[0]
	return s, nil
}

// FrameDurationMS selects short frames for tighter playback budgets, with a
// minimum of 5 ms. Receiver acceptance does not guarantee meeting the target.
func (s StreamingSession) FrameDurationMS() int {
	if s.DelayMS < 40 {
		return 5
	}
	if s.DelayMS < 80 {
		return 10
	}
	return 20
}
