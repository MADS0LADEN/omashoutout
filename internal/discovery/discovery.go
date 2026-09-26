package discovery

import (
	"context"
	"errors"
	"fmt"
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
	mu      sync.Mutex
	txt     map[string]map[string]string
	srv     map[string]dnsmessage.SRVResource
	ips     map[string]string
	expires map[string]time.Time
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
		if r.expires == nil {
			r.expires = make(map[string]time.Time)
		}
		ttl := min(time.Duration(a.Header.TTL)*time.Second, 45*time.Second)
		r.expires[fmt.Sprintf("%d:%s", a.Header.Type, name)] = time.Now().Add(ttl)
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
	return discover(ctx, "", nil, nil)
}

// Find returns as soon as all address records for the selected receiver arrive.
func Find(ctx context.Context, id string) (Device, error) {
	if id == "" {
		return Device{}, errors.New("receiver ID is empty")
	}
	devices, err := discover(ctx, id, nil, nil)
	if err != nil {
		return Device{}, err
	}
	for _, d := range devices {
		if d.ID == id {
			return d, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return Device{}, err
	}
	return Device{}, errors.New("selected receiver not found")
}

func discover(ctx context.Context, id string, publish func([]Device), r *records) ([]Device, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	if r == nil {
		r = &records{txt: map[string]map[string]string{}, srv: map[string]dnsmessage.SRVResource{}, ips: map[string]string{}}
	}
	name, _ := dnsmessage.NewName("_googlecast._tcp.local.")
	query, err := (&dnsmessage.Message{Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}}}).Pack()
	if err != nil {
		return nil, err
	}
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	updated := make(chan struct{}, 1)
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
				select {
				case updated <- struct{}{}:
				default:
				}
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
	ticks := 0
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-updated:
			if publish != nil {
				r.expire(time.Now())
				publish(r.devices())
			}
			if id != "" {
				for _, d := range r.devices() {
					if d.ID == id {
						return []Device{d}, nil
					}
				}
			}
		case <-timer.C:
			if publish != nil {
				r.expire(time.Now())
				publish(r.devices())
			}
			ticks++
			if publish != nil && ticks%10 != 0 {
				continue
			}
			for _, c := range connections {
				_, _ = c.WriteToUDP(query, group)
			}
		}
	}
	return r.devices(), nil
}

func (r *records) devices() []Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	var devices []Device
	for instance, srv := range r.srv {
		if !strings.HasSuffix(instance, "._googlecast._tcp.local.") {
			continue
		}
		p := r.txt[instance]
		host := r.ips[strings.ToLower(srv.Target.String())]
		if host == "" || p["fn"] == "" || p["id"] == "" {
			continue
		}
		devices = append(devices, Device{ID: p["id"], Name: p["fn"], Model: p["md"], Host: host, Port: int(srv.Port)})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices
}

func (r *records) expire(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, until := range r.expires {
		if now.Before(until) {
			continue
		}
		kind, name, _ := strings.Cut(key, ":")
		switch kind {
		case "16":
			delete(r.txt, name)
		case "33":
			delete(r.srv, name)
		case "1":
			delete(r.ips, name)
		}
		delete(r.expires, key)
	}
}
