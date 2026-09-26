package cast

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const packetPayload = 1100

type TransportStats struct {
	Frames, Packets, Retransmits uint64
	Feedback                     uint64
	Acknowledged                 int64
	ReceiverDelayMS              int
}
type sentFrame struct {
	packets         [][]byte
	sent, lastRetry time.Time
}

// Transport sends paced, independently encrypted Opus frames. It only accepts
// feedback from the negotiated peer and retains a bounded acknowledgement window.
type Transport struct {
	conn       *net.UDPConn
	session    StreamingSession
	cipher     cipher.Block
	mu         sync.Mutex
	epoch      time.Time
	next       uint32
	sequence   uint16
	octets     uint32
	cache      map[uint32]*sentFrame
	stats      TransportStats
	lastReport time.Time
}

func NewTransport(localIP, remoteIP string, s StreamingSession) (*Transport, error) {
	block, err := aes.NewCipher(s.Key[:])
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", &net.UDPAddr{IP: net.ParseIP(localIP)}, &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: s.Port})
	if err != nil {
		return nil, err
	}
	return &Transport{conn: conn, session: s, cipher: block, cache: make(map[uint32]*sentFrame), stats: TransportStats{Acknowledged: -1}}, nil
}
func (t *Transport) Close() error          { return t.conn.Close() }
func (t *Transport) Stats() TransportStats { t.mu.Lock(); defer t.mu.Unlock(); return t.stats }

func frameCipher(block cipher.Block, mask [16]byte, id uint32, plain []byte) []byte {
	var nonce [16]byte
	binary.BigEndian.PutUint32(nonce[8:12], id)
	for i := range nonce {
		nonce[i] ^= mask[i]
	}
	result := make([]byte, len(plain))
	cipher.NewCTR(block, nonce[:]).XORKeyStream(result, plain)
	return result
}

func audioPackets(ssrc, frame uint32, sequence *uint16, samples uint32, encrypted []byte) [][]byte {
	count := max(1, (len(encrypted)+packetPayload-1)/packetPayload)
	packets := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		n := min(packetPayload, len(encrypted)-i*packetPayload)
		p := make([]byte, 19+n)
		p[0] = 0x80
		p[1] = 127
		if i == count-1 {
			p[1] |= 0x80
		}
		binary.BigEndian.PutUint16(p[2:4], *sequence)
		*sequence++
		binary.BigEndian.PutUint32(p[4:8], frame*samples)
		binary.BigEndian.PutUint32(p[8:12], ssrc)
		p[12] = 0xc0
		p[13] = byte(frame)
		binary.BigEndian.PutUint16(p[14:16], uint16(i))
		binary.BigEndian.PutUint16(p[16:18], uint16(count-1))
		p[18] = byte(frame)
		copy(p[19:], encrypted[i*packetPayload:i*packetPayload+n])
		packets = append(packets, p)
	}
	return packets
}

// SendFrame is called by one encoder reader. Receiver feedback runs concurrently.
func (t *Transport) SendFrame(ctx context.Context, opus []byte) error {
	if len(opus) == 0 || len(opus) > 8192 {
		return errors.New("invalid Opus frame length")
	}
	frameDuration := time.Duration(t.session.FrameDurationMS()) * time.Millisecond
	t.mu.Lock()
	if t.epoch.IsZero() {
		t.epoch = time.Now().Add(-frameDuration)
	}
	due := t.epoch.Add(time.Duration(t.next+1) * frameDuration)
	t.mu.Unlock()
	if wait := time.Until(due); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if time.Since(due) > 250*time.Millisecond {
		return errors.New("audio encoder fell behind; discard stale audio")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if int64(t.next)-t.stats.Acknowledged > int64(min(120, max(25, t.session.DelayMS/t.session.FrameDurationMS()+25))) {
		return errors.New("receiver acknowledgement stalled; discard stale audio")
	}
	if t.next == ^uint32(0) {
		return errors.New("audio encryption frame counter exhausted")
	}
	now := time.Now()
	if now.Sub(t.lastReport) >= 250*time.Millisecond {
		if err := t.sendReport(now); err != nil {
			return err
		}
	}
	encrypted := frameCipher(t.cipher, t.session.IV, t.next, opus)
	packets := audioPackets(t.session.SenderSSRC, t.next, &t.sequence, uint32(t.session.FrameDurationMS()*48), encrypted)
	for _, p := range packets {
		if err := t.writePacket(p, false); err != nil {
			return err
		}
	}
	t.cache[t.next] = &sentFrame{packets: packets, sent: now}
	t.next++
	t.stats.Frames++
	return nil
}
func (t *Transport) writePacket(p []byte, retry bool) error {
	if err := t.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return err
	}
	if _, err := t.conn.Write(p); err != nil {
		return err
	}
	t.stats.Packets++
	t.octets += uint32(len(p) - 19)
	if retry {
		t.stats.Retransmits++
	}
	return nil
}
func senderReport(ssrc uint32, now, epoch time.Time, packets, octets uint32) []byte {
	p := make([]byte, 28)
	p[0] = 0x80
	p[1] = 200
	binary.BigEndian.PutUint16(p[2:4], 6)
	binary.BigEndian.PutUint32(p[4:8], ssrc)
	binary.BigEndian.PutUint32(p[8:12], uint32(now.Unix()+2208988800))
	binary.BigEndian.PutUint32(p[12:16], uint32((uint64(now.Nanosecond())<<32)/1000000000))
	binary.BigEndian.PutUint32(p[16:20], uint32(uint64(now.Sub(epoch)/time.Second)*48000+uint64(now.Sub(epoch)%time.Second)*48000/1000000000))
	binary.BigEndian.PutUint32(p[20:24], packets)
	binary.BigEndian.PutUint32(p[24:28], octets)
	return p
}
func (t *Transport) sendReport(now time.Time) error {
	p := senderReport(t.session.SenderSSRC, now, t.epoch, uint32(t.stats.Packets), t.octets)
	if err := t.conn.SetWriteDeadline(now.Add(100 * time.Millisecond)); err != nil {
		return err
	}
	if _, err := t.conn.Write(p); err != nil {
		return err
	}
	t.lastReport = now
	return nil
}

type packetLoss struct {
	frame     byte
	packet    uint16
	following byte
}
type feedback struct {
	checkpoint byte
	delay      int
	loss       []packetLoss
}

func parseFeedback(b []byte, senderSSRC, receiverSSRC uint32) ([]feedback, error) {
	var out []feedback
	for len(b) > 0 {
		if len(b) < 4 || b[0]>>6 != 2 {
			return nil, errors.New("invalid RTCP header")
		}
		size := (int(binary.BigEndian.Uint16(b[2:4])) + 1) * 4
		if size > len(b) || size < 4 {
			return nil, errors.New("truncated RTCP")
		}
		p := b[:size]
		b = b[size:]
		if p[0]&0x20 != 0 {
			padding := int(p[len(p)-1])
			if padding == 0 || padding > len(p)-4 {
				return nil, errors.New("invalid RTCP padding")
			}
			p = p[:len(p)-padding]
		}
		if p[1] != 206 || p[0]&31 != 15 {
			continue
		}
		if len(p) < 20 {
			return nil, errors.New("short Cast feedback")
		}
		if binary.BigEndian.Uint32(p[4:8]) != receiverSSRC || binary.BigEndian.Uint32(p[8:12]) != senderSSRC || string(p[12:16]) != "CAST" {
			continue
		}
		count := int(p[17])
		if len(p) < 20+count*4 {
			return nil, errors.New("truncated packet loss list")
		}
		f := feedback{checkpoint: p[16], delay: int(binary.BigEndian.Uint16(p[18:20]))}
		for i := 0; i < count; i++ {
			offset := 20 + i*4
			f.loss = append(f.loss, packetLoss{p[offset], binary.BigEndian.Uint16(p[offset+1 : offset+3]), p[offset+3]})
		}
		out = append(out, f)
	}
	return out, nil
}

func (t *Transport) Receive(ctx context.Context) error {
	buf := make([]byte, 65536)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return err
		}
		n, err := t.conn.Read(buf)
		if err != nil {
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				continue
			}
			return err
		}
		reports, err := parseFeedback(buf[:n], t.session.SenderSSRC, t.session.ReceiverSSRC)
		if err != nil {
			continue
		}
		for _, f := range reports {
			if err := t.receiveFeedback(f); err != nil {
				return err
			}
		}
	}
}
func (t *Transport) receiveFeedback(f feedback) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.next == 0 {
		return nil
	}
	// Expand the truncated ID relative to the most recently sent frame. The
	// in-flight limit is below half the ID space, keeping wraparound unambiguous.
	latest := int64(t.next) - 1
	ack := latest - int64(byte(byte(latest)-f.checkpoint))
	if ack < t.stats.Acknowledged {
		return nil
	}
	if latest-ack > 127 {
		return nil
	}
	t.stats.Acknowledged = ack
	t.stats.Feedback++
	t.stats.ReceiverDelayMS = f.delay
	for id := range t.cache {
		if int64(id) <= ack {
			delete(t.cache, id)
		}
	}
	now := time.Now()
	retried := make(map[uint32]bool)
	for _, loss := range f.loss {
		id := ack + int64(uint8(loss.frame-byte(ack)))
		if id <= ack || id > latest {
			continue
		}
		frame := t.cache[uint32(id)]
		if frame == nil || now.Sub(frame.sent) > time.Duration(t.session.DelayMS+200)*time.Millisecond {
			continue
		}
		if !retried[uint32(id)] && now.Sub(frame.lastRetry) < 15*time.Millisecond {
			continue
		}
		retry := func(index int) error {
			if index < 0 || index >= len(frame.packets) {
				return nil
			}
			p := append([]byte(nil), frame.packets[index]...)
			binary.BigEndian.PutUint16(p[2:4], t.sequence)
			t.sequence++
			if err := t.writePacket(p, true); err != nil {
				return fmt.Errorf("retransmit audio: %w", err)
			}
			return nil
		}
		if loss.packet == 65535 {
			for i := range frame.packets {
				if err := retry(i); err != nil {
					return err
				}
			}
		} else {
			if err := retry(int(loss.packet)); err != nil {
				return err
			}
			for bit := 0; bit < 8; bit++ {
				if loss.following&(1<<bit) != 0 {
					if err := retry(int(loss.packet) + bit + 1); err != nil {
						return err
					}
				}
			}
		}
		frame.lastRetry = now
		retried[uint32(id)] = true
	}
	return nil
}
