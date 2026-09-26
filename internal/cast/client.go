package cast

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/lkarlslund/shoutout/internal/config"
)

const (
	receiverNS   = "urn:x-cast:com.google.cast.receiver"
	connectionNS = "urn:x-cast:com.google.cast.tp.connection"
	heartbeatNS  = "urn:x-cast:com.google.cast.tp.heartbeat"
	mediaNS      = "urn:x-cast:com.google.cast.media"
	receiver     = "receiver-0"
	sender       = "shoutout-0"
	appID        = "CC1AD845"
)

type Volume struct {
	Level float64 `json:"level"`
	Muted bool    `json:"muted"`
}
type Application struct {
	AppID       string `json:"appId"`
	TransportID string `json:"transportId"`
	SessionID   string `json:"sessionId"`
	DisplayName string `json:"displayName"`
}
type ReceiverStatus struct {
	Volume       Volume        `json:"volume"`
	Applications []Application `json:"applications"`
}
type MediaStatus struct {
	Media struct {
		ContentID string `json:"contentId"`
	} `json:"media"`
	PlayerState    string  `json:"playerState"`
	MediaSessionID int     `json:"mediaSessionId"`
	CurrentTime    float64 `json:"currentTime"`
	IdleReason     string  `json:"idleReason"`
}
type response struct {
	Type      string          `json:"type"`
	RequestID int             `json:"requestId"`
	Status    json.RawMessage `json:"status"`
	Reason    string          `json:"reason"`
}

type Client struct {
	conn      net.Conn
	writeMu   sync.Mutex
	mu        sync.Mutex
	next      int
	pending   map[int]chan response
	done      chan struct{}
	closeOnce sync.Once
	readErr   error
	workers   sync.WaitGroup
}

func Dial(ctx context.Context, address string) (*Client, error) {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{
		// Cast devices present device certificates outside the public Web PKI.
		// This connection is only to the explicitly selected local-network receiver.
		InsecureSkipVerify: true, MinVersion: tls.VersionTLS12,
	}}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, pending: make(map[int]chan response), done: make(chan struct{})}
	c.workers.Add(2)
	go c.readLoop()
	go c.heartbeat()
	if err = c.send(receiver, connectionNS, map[string]any{"type": "CONNECT", "origin": map[string]any{}}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
func (c *Client) LocalIP() string {
	h, _, _ := net.SplitHostPort(c.conn.LocalAddr().String())
	return h
}
func (c *Client) RemoteIP() string {
	h, _, _ := net.SplitHostPort(c.conn.RemoteAddr().String())
	return h
}
func (c *Client) Close() { c.shutdown(errors.New("Cast connection closed")); c.workers.Wait() }
func (c *Client) shutdown(err error) {
	c.closeOnce.Do(func() { c.mu.Lock(); c.readErr = err; c.mu.Unlock(); close(c.done); c.conn.Close() })
}
func (c *Client) send(destination, namespace string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame := encode(envelope{Source: sender, Destination: destination, Namespace: namespace, Payload: b})
	data := binary.BigEndian.AppendUint32(nil, uint32(len(frame)))
	data = append(data, frame...)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	for len(data) > 0 {
		n, err := c.conn.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("empty Cast write")
		}
		data = data[n:]
	}
	return nil
}
func (c *Client) readLoop() {
	defer c.workers.Done()
	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(25 * time.Second)); err != nil {
			c.shutdown(err)
			return
		}
		e, err := readEnvelope(c.conn)
		if err != nil {
			c.shutdown(err)
			return
		}
		var r response
		if json.Unmarshal(e.Payload, &r) != nil {
			continue
		}
		if e.Namespace == heartbeatNS && r.Type == "PING" {
			if err = c.send(e.Source, heartbeatNS, map[string]any{"type": "PONG"}); err != nil {
				c.shutdown(err)
				return
			}
		}
		c.mu.Lock()
		ch := c.pending[r.RequestID]
		c.mu.Unlock()
		if ch != nil {
			select {
			case ch <- r:
			default:
			}
		}
	}
}
func (c *Client) heartbeat() {
	defer c.workers.Done()
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			if err := c.send(receiver, heartbeatNS, map[string]any{"type": "PING"}); err != nil {
				c.shutdown(err)
				return
			}
		}
	}
}
func (c *Client) request(ctx context.Context, destination, namespace string, p map[string]any) (response, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	p["requestId"] = id
	if err := c.send(destination, namespace, p); err != nil {
		return response{}, err
	}
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case r := <-ch:
		switch r.Type {
		case "INVALID_REQUEST", "LOAD_FAILED", "LAUNCH_ERROR", "LOAD_CANCELLED":
			return r, fmt.Errorf("receiver %s: %s", r.Type, r.Reason)
		}
		return r, nil
	case <-ctx.Done():
		return response{}, ctx.Err()
	case <-c.done:
		c.mu.Lock()
		err := c.readErr
		c.mu.Unlock()
		return response{}, err
	case <-timer.C:
		return response{}, errors.New("Cast response timed out")
	}
}
func (c *Client) Status(ctx context.Context) (ReceiverStatus, error) {
	r, err := c.request(ctx, receiver, receiverNS, map[string]any{"type": "GET_STATUS"})
	if err != nil {
		return ReceiverStatus{}, err
	}
	var s ReceiverStatus
	err = json.Unmarshal(r.Status, &s)
	return s, err
}
func (c *Client) SetVolume(ctx context.Context, level float64, muted bool) error {
	if !(level >= 0 && level <= config.MaxReceiverVolume) {
		return errors.New("receiver volume must be between 0 and 100%")
	}
	// Quantize down, never up, because receivers commonly store float32 levels.
	// Do not exceed the requested receiver level when converting precision.
	quantized := float32(level)
	if float64(quantized) > level {
		quantized = math.Nextafter32(quantized, 0)
	}
	level = float64(quantized)
	// Some receivers apply only one volume field per request. Mute first,
	// then set the level and reassert mute: changing level can clear mute.
	// Verify both fields before any intentional request to unmute.
	for _, volume := range []map[string]any{{"muted": true}, {"level": level}, {"muted": true}} {
		if _, err := c.request(ctx, receiver, receiverNS, map[string]any{"type": "SET_VOLUME", "volume": volume}); err != nil {
			return err
		}
	}
	check := func(wantMuted bool) error {
		s, err := c.Status(ctx)
		if err != nil {
			return err
		}
		if s.Volume.Level > config.MaxReceiverVolume || s.Volume.Level > level+0.00001 || s.Volume.Muted != wantMuted {
			return fmt.Errorf("receiver did not confirm safe volume/mute state: level %.6f, muted %t; requested %.6f, muted %t", s.Volume.Level, s.Volume.Muted, level, wantMuted)
		}
		return nil
	}
	if err := check(true); err != nil {
		return err
	}
	if !muted {
		if _, err := c.request(ctx, receiver, receiverNS, map[string]any{"type": "SET_VOLUME", "volume": map[string]any{"muted": false}}); err != nil {
			return err
		}
		if err := check(false); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) Launch(ctx context.Context) (Application, error) {
	r, err := c.request(ctx, receiver, receiverNS, map[string]any{"type": "LAUNCH", "appId": appID})
	if err != nil {
		return Application{}, err
	}
	var s ReceiverStatus
	if err = json.Unmarshal(r.Status, &s); err != nil {
		return Application{}, err
	}
	for _, app := range s.Applications {
		if app.AppID == appID && app.TransportID != "" {
			err = c.send(app.TransportID, connectionNS, map[string]any{"type": "CONNECT", "origin": map[string]any{}})
			return app, err
		}
	}
	return Application{}, errors.New("receiver did not launch audio application")
}
func (c *Client) Load(ctx context.Context, app Application, url, contentType string) error {
	_, err := c.request(ctx, app.TransportID, mediaNS, map[string]any{"type": "LOAD", "autoplay": true, "currentTime": 0, "media": map[string]any{"contentId": url, "contentType": contentType, "streamType": "LIVE", "metadata": map[string]any{"metadataType": 3, "title": "Shoutout", "artist": "Desktop audio"}}})
	return err
}
func (c *Client) Media(ctx context.Context, app Application) (MediaStatus, error) {
	r, err := c.request(ctx, app.TransportID, mediaNS, map[string]any{"type": "GET_STATUS"})
	if err != nil {
		return MediaStatus{}, err
	}
	var states []MediaStatus
	if err = json.Unmarshal(r.Status, &states); err != nil {
		return MediaStatus{}, err
	}
	if len(states) == 0 {
		return MediaStatus{}, errors.New("receiver has no media session")
	}
	return states[0], nil
}
func (c *Client) Stop(ctx context.Context, app Application) error {
	s, err := c.Status(ctx)
	if err != nil {
		return err
	}
	for _, a := range s.Applications {
		if a.SessionID == app.SessionID {
			_, err = c.request(ctx, receiver, receiverNS, map[string]any{"type": "STOP", "sessionId": app.SessionID})
			return err
		}
	}
	return nil
}
