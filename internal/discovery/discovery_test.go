package discovery

import (
	"golang.org/x/net/dns/dnsmessage"
	"testing"
)

func TestRecordsAcrossPackets(t *testing.T) {
	r := records{txt: map[string]map[string]string{}, srv: map[string]dnsmessage.SRVResource{}, ips: map[string]string{}}
	instance := dnsmessage.MustNewName("Room._googlecast._tcp.local.")
	host := dnsmessage.MustNewName("Speaker.local.")
	packets := [][]dnsmessage.Resource{
		{{Header: dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeTXT, Class: dnsmessage.ClassINET}, Body: &dnsmessage.TXTResource{TXT: []string{"fn=Room", "id=unique", "md=Audio"}}}},
		{{Header: dnsmessage.ResourceHeader{Name: instance, Type: dnsmessage.TypeSRV, Class: dnsmessage.ClassINET}, Body: &dnsmessage.SRVResource{Port: 8009, Target: host}}, {Header: dnsmessage.ResourceHeader{Name: host, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}, Body: &dnsmessage.AResource{A: [4]byte{192, 0, 2, 1}}}},
	}
	for _, answers := range packets {
		b, err := (&dnsmessage.Message{Answers: answers}).Pack()
		if err != nil {
			t.Fatal(err)
		}
		r.ingest(b)
	}
	if r.txt["room._googlecast._tcp.local."]["fn"] != "Room" || r.srv["room._googlecast._tcp.local."].Port != 8009 || r.ips["speaker.local."] != "192.0.2.1" {
		t.Fatal("record aggregation failed")
	}
	r.ingest([]byte{1, 2, 3})
}
