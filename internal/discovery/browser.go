package discovery

import (
	"context"
	"slices"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Browser owns the live device list and publishes snapshots to subscribers.
// Each subscriber receives the current list immediately, then only changes.
type Browser struct {
	mu          sync.Mutex
	devices     []Device
	subscribers map[chan []Device]struct{}
}

func (b *Browser) Snapshot() []Device {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Device{}, b.devices...)
}

func (b *Browser) Subscribe() (<-chan []Device, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[chan []Device]struct{})
	}
	ch := make(chan []Device, 1)
	b.subscribers[ch] = struct{}{}
	ch <- append([]Device{}, b.devices...)
	return ch, func() { b.mu.Lock(); defer b.mu.Unlock(); delete(b.subscribers, ch) }
}

func (b *Browser) publish(devices []Device) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if slices.Equal(b.devices, devices) {
		return
	}
	b.devices = slices.Clone(devices)
	for ch := range b.subscribers {
		select {
		case <-ch:
		default:
		}
		ch <- append([]Device{}, devices...)
	}
}

// Run listens continuously, queries every ten seconds, and expires silent
// records after at most 45 seconds. Reopen sockets periodically for NIC changes.
func (b *Browser) Run(ctx context.Context) {
	r := &records{txt: map[string]map[string]string{}, srv: map[string]dnsmessage.SRVResource{}, ips: map[string]string{}}
	for ctx.Err() == nil {
		scan, cancel := context.WithTimeout(ctx, time.Minute)
		_, err := discover(scan, "", b.publish, r)
		cancel()
		if err != nil {
			b.publish(nil)
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

// Find uses the shared discovery stream, including devices already cached.
func (b *Browser) Find(ctx context.Context, id string) (Device, error) {
	updates, unsubscribe := b.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return Device{}, ctx.Err()
		case devices := <-updates:
			for _, device := range devices {
				if device.ID == id {
					return device, nil
				}
			}
		}
	}
}
