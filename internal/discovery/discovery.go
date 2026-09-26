package discovery

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

type Device struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
}

type records struct {
	mu  sync.Mutex
	txt map[string]map[string]string
	srv map[string]dnsmessage.SRVResource
	ips map[string]string
}

func (r *records) ingest(data []byte) {
	var m dnsmessage.Message
	if m.Unpack(data) != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	all := append(append(m.Answers, m.Authorities...), m.Additionals...)
	for _, a := range all {
		name := strings.ToLower(a.Header.Name.String())
		switch b := a.Body.(type) {
		case *dnsmessage.TXTResource:
			p := map[string]string{}
			for _, v := range b.TXT {
				k, val, ok := strings.Cut(v, "=")
				if ok {
					p[k] = val
				}
			}
			r.txt[name] = p
		case *dnsmessage.SRVResource:
			r.srv[name] = *b
		case *dnsmessage.AResource:
			r.ips[name] = net.IP(b.A[:]).String()
		}
	}
}

// Discover uses IPv4 mDNS directly and does not require a system discovery daemon.
func Discover(ctx context.Context) ([]Device, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	r := records{txt: map[string]map[string]string{}, srv: map[string]dnsmessage.SRVResource{}, ips: map[string]string{}}
	name, _ := dnsmessage.NewName("_googlecast._tcp.local.")
	query, err := (&dnsmessage.Message{Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}}}).Pack()
	if err != nil {
		return nil, err
	}
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	var wg sync.WaitGroup
	var connections []*net.UDPConn
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		conn, err := net.ListenMulticastUDP("udp4", &iface, group)
		if err != nil {
			continue
		}
		pc := ipv4.NewPacketConn(conn)
		if pc.SetMulticastInterface(&iface) != nil {
			conn.Close()
			continue
		}
		if pc.SetMulticastTTL(255) != nil {
			conn.Close()
			continue
		}
		connections = append(connections, conn)
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := make([]byte, 65536)
			for {
				n, _, err := conn.ReadFromUDP(b)
				if err != nil {
					return
				}
				r.ingest(b[:n])
			}
		}()
	}
	if len(connections) == 0 {
		return nil, errors.New("no usable IPv4 multicast interface; configure a receiver address manually")
	}
	defer func() {
		for _, c := range connections {
			c.Close()
		}
		wg.Wait()
	}()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for _, c := range connections {
		_, _ = c.WriteToUDP(query, group)
	}
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-timer.C:
			for _, c := range connections {
				_, _ = c.WriteToUDP(query, group)
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var devices []Device
	for instance, srv := range r.srv {
		if !strings.HasSuffix(instance, "._googlecast._tcp.local.") {
			continue
		}
		p := r.txt[instance]
		host := r.ips[strings.ToLower(srv.Target.String())]
		if host == "" || p["fn"] == "" {
			continue
		}
		devices = append(devices, Device{ID: p["id"], Name: p["fn"], Model: p["md"], Host: host, Port: int(srv.Port)})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices, nil
}
