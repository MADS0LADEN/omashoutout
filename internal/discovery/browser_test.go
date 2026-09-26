package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestBrowserSubscriptions(t *testing.T) {
	var b Browser
	first, stopFirst := b.Subscribe()
	second, stopSecond := b.Subscribe()
	defer stopSecond()
	if len(<-first) != 0 || len(<-second) != 0 {
		t.Fatal("initial list not empty")
	}
	d := Device{ID: "one", Name: "Room", Host: "192.0.2.1", Port: 8009}
	b.publish([]Device{d})
	if got := <-first; len(got) != 1 || got[0] != d {
		t.Fatalf("arrival: %v", got)
	}
	got := <-second
	got[0].Name = "modified"
	if b.Snapshot()[0].Name != "Room" {
		t.Fatal("subscriber mutated shared state")
	}
	b.publish([]Device{d})
	select {
	case <-first:
		t.Fatal("published unchanged devices")
	default:
	}
	stopFirst()
	b.publish(nil)
	if got := <-second; len(got) != 0 {
		t.Fatal("departure not delivered")
	}
	select {
	case <-first:
		t.Fatal("unsubscribe failed")
	default:
	}
	// A stalled subscriber must see the latest state and never block discovery.
	b.publish([]Device{d})
	b.publish(nil)
	if len(<-second) != 0 {
		t.Fatal("stalled subscriber got stale state")
	}
}

func TestDiscoveryExpiryAndRefresh(t *testing.T) {
	r := records{txt: map[string]map[string]string{}, srv: map[string]dnsmessage.SRVResource{}, ips: map[string]string{}}
	instance := dnsmessage.MustNewName("Room._googlecast._tcp.local.")
	host := dnsmessage.MustNewName("Room.local.")
	packet := func(ttl uint32) []byte {
		resources := []dnsmessage.Resource{
			{Header: dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET, TTL: ttl}, Body: &dnsmessage.TXTResource{TXT: []string{"fn=Room", "id=one"}}},
			{Header: dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET, TTL: ttl}, Body: &dnsmessage.SRVResource{Target: host, Port: 8009}},
			{Header: dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: ttl}, Body: &dnsmessage.AResource{A: [4]byte{192, 0, 2, 1}}},
		}
		data, err := (&dnsmessage.Message{Answers: resources}).Pack()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	r.ingest(packet(120))
	now := time.Now()
	r.expire(now.Add(44 * time.Second))
	if len(r.devices()) != 1 {
		t.Fatal("expired before silence deadline")
	}
	r.expire(now.Add(46 * time.Second))
	if len(r.devices()) != 0 {
		t.Fatal("silent device retained")
	}
	r.ingest(packet(1))
	r.ingest(packet(120))
	r.expire(time.Now().Add(2 * time.Second))
	if len(r.devices()) != 1 {
		t.Fatal("new response did not renew expiry")
	}
	r.ingest(packet(0))
	r.expire(time.Now())
	if len(r.devices()) != 0 {
		t.Fatal("goodbye did not remove device")
	}
	r.ingest(packet(1))
	r.expire(time.Now().Add(2 * time.Second))
	if len(r.devices()) != 0 {
		t.Fatal("ignored shorter advertised TTL")
	}
}

func TestFindUsesSharedCache(t *testing.T) {
	var b Browser
	d := Device{ID: "one", Name: "Room"}
	b.publish([]Device{d})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if got, err := b.Find(ctx, "one"); err != nil || got != d {
		t.Fatalf("cached lookup: %v %v", got, err)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := b.Find(canceled, "absent"); !errors.Is(err, context.Canceled) {
		t.Fatalf("missing device cancellation: %v", err)
	}
}
