package cast

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/binary"
	"encoding/hex"
	"net"
	"testing"
	"time"
)

func hexBytes(s string) []byte { b, _ := hex.DecodeString(s); return b }
func TestFrameEncryption(t *testing.T) {
	block, _ := aes.NewCipher(hexBytes("2b7e151628aed2a6abf7158809cf4f3c"))
	var iv [16]byte
	copy(iv[:], hexBytes("f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff"))
	plain := hexBytes("6bc1bee22e409f96e93d7e117393172a")
	if got := frameCipher(block, iv, 0, plain); !bytes.Equal(got, hexBytes("874d6191b620e3261bef6864990db6ce")) {
		t.Fatalf("AES-CTR vector: %x", got)
	}
	if bytes.Equal(frameCipher(block, iv, 1, plain), frameCipher(block, iv, 257, plain)) {
		t.Fatal("nonce repeated after wire frame-ID wrap")
	}
}
func TestPacketFragmentation(t *testing.T) {
	data := bytes.Repeat([]byte{17}, 2500)
	seq := uint16(65535)
	packets := audioPackets(42, 257, &seq, 960, data)
	if len(packets) != 3 || seq != 2 {
		t.Fatal("fragment count or sequence wrap")
	}
	var joined []byte
	for i, p := range packets {
		if p[0] != 0x80 || p[1]&127 != 127 || p[12] != 0xc0 || p[13] != 1 || p[18] != 1 || binary.BigEndian.Uint16(p[14:16]) != uint16(i) || binary.BigEndian.Uint16(p[16:18]) != 2 {
			t.Fatalf("bad packet header: %x", p[:19])
		}
		if (p[1]&128 != 0) != (i == 2) {
			t.Fatal("bad last-packet marker")
		}
		if binary.BigEndian.Uint32(p[4:8]) != 257*960 {
			t.Fatal("wrong audio timestamp")
		}
		joined = append(joined, p[19:]...)
	}
	if !bytes.Equal(joined, data) {
		t.Fatal("fragmentation changed encrypted bytes")
	}
}
func feedbackPacket(ack byte) []byte {
	p := make([]byte, 20)
	p[0] = 0x8f
	p[1] = 206
	binary.BigEndian.PutUint16(p[2:4], 4)
	binary.BigEndian.PutUint32(p[4:8], 43)
	binary.BigEndian.PutUint32(p[8:12], 42)
	copy(p[12:], "CAST")
	p[16] = ack
	binary.BigEndian.PutUint16(p[18:20], 400)
	return p
}
func TestFeedbackParsing(t *testing.T) {
	p := feedbackPacket(255)
	p = append(p, 0, 0xff, 0xff, 0)
	p[17] = 1
	binary.BigEndian.PutUint16(p[2:4], 5)
	f, err := parseFeedback(p, 42, 43)
	if err != nil || len(f) != 1 || f[0].checkpoint != 255 || f[0].delay != 400 || len(f[0].loss) != 1 || f[0].loss[0].packet != 65535 {
		t.Fatalf("feedback: %+v %v", f, err)
	}
	if f, err = parseFeedback(p, 42, 44); err != nil || len(f) != 0 {
		t.Fatal("wrong receiver accepted")
	}
	for i := 1; i < len(p); i++ {
		if _, err = parseFeedback(p[:i], 42, 43); err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
}
func TestAcknowledgementWrap(t *testing.T) {
	tr := &Transport{next: 259, stats: TransportStats{Acknowledged: 253}, cache: map[uint32]*sentFrame{254: {}, 255: {}, 256: {}, 257: {}, 258: {}}}
	if err := tr.receiveFeedback(feedback{checkpoint: 1, delay: 100}); err != nil {
		t.Fatal(err)
	}
	if tr.stats.Acknowledged != 257 || len(tr.cache) != 1 {
		t.Fatalf("wrap handling: %+v", tr.stats)
	}
	tr.receiveFeedback(feedback{checkpoint: 254, delay: 400})
	if tr.stats.Acknowledged != 257 || tr.stats.ReceiverDelayMS != 100 {
		t.Fatal("old feedback rolled state backward")
	}
}
func TestSenderReportClock(t *testing.T) {
	epoch := time.Unix(1700000000, 0)
	now := epoch.Add(1500 * time.Millisecond)
	p := senderReport(42, now, epoch, 75, 24000)
	if len(p) != 28 || p[1] != 200 || binary.BigEndian.Uint32(p[16:20]) != 72000 || binary.BigEndian.Uint32(p[12:16]) != 0x80000000 {
		t.Fatalf("invalid sender clock mapping: %x", p)
	}
}
func FuzzFeedback(f *testing.F) {
	f.Add(feedbackPacket(1))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 65536 {
			t.Skip()
		}
		parseFeedback(b, 42, 43)
	})
}

func TestLossRetransmission(t *testing.T) {
	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	session := StreamingSession{SenderSSRC: 42, ReceiverSSRC: 43, Port: receiver.LocalAddr().(*net.UDPAddr).Port, DelayMS: 400}
	tr, err := NewTransport("127.0.0.1", "127.0.0.1", session)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	readFrame := func() []byte {
		t.Helper()
		receiver.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 1500)
		for {
			n, _, err := receiver.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			if buf[1] == 200 {
				continue
			}
			return append([]byte(nil), buf[:n]...)
		}
	}
	for i := 0; i < 2; i++ {
		if err = tr.SendFrame(context.Background(), []byte{0xf8, 1, 2}); err != nil {
			t.Fatal(err)
		}
		readFrame()
	}
	if err = tr.receiveFeedback(feedback{checkpoint: 0, delay: 400, loss: []packetLoss{{frame: 1, packet: 65535}}}); err != nil {
		t.Fatal(err)
	}
	retry := readFrame()
	if retry[13] != 1 || binary.BigEndian.Uint16(retry[2:4]) != 2 || tr.Stats().Retransmits != 1 {
		t.Fatal("lost frame not retransmitted with fresh sequence")
	}
	tr.receiveFeedback(feedback{checkpoint: 1, delay: 400})
	if len(tr.cache) != 0 || tr.Stats().Acknowledged != 1 {
		t.Fatal("acknowledged frame retained")
	}
}
