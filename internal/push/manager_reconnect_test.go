// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/push"
)

const reconnectTestAddr = "127.0.0.1:59999"

// dialRecorder records every address handed to the dial function and fails
// every attempt after the first, so the reconnect walk always runs to its
// bound.
type dialRecorder struct {
	mu    sync.Mutex
	addrs []string
}

func (d *dialRecorder) dial(ctx context.Context, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.addrs = append(d.addrs, addr)
	n := len(d.addrs)
	d.mu.Unlock()
	if n == 1 {
		client, server := net.Pipe()
		// Close the far end so the read loop fails immediately and enters the
		// reconnect path.
		_ = server.Close()
		return client, nil
	}
	return nil, errors.New("dial refused")
}

func (d *dialRecorder) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.addrs...)
}

func waitForDials(t *testing.T, d *dialRecorder, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(d.snapshot()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d dials, got %d", want, len(d.snapshot()))
}

func newReconnectManager(t *testing.T, d *dialRecorder, opts ...push.ManagerOption) *push.Manager {
	t.Helper()
	base := []push.ManagerOption{
		push.WithManagerDialFunc(d.dial),
		push.WithReconnectBackoff(time.Millisecond, time.Millisecond),
		push.WithHeartbeatInterval(0),
	}
	m := push.NewManager(nil, append(base, opts...)...)
	t.Cleanup(func() { _ = m.Close() })
	if err := m.Dial(context.Background(), reconnectTestAddr); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return m
}

func TestManager_ReconnectNeverDialsEmptyAddress(t *testing.T) {
	d := &dialRecorder{}
	newReconnectManager(t, d)
	waitForDials(t, d, 3)

	for i, addr := range d.snapshot() {
		if addr == "" {
			t.Fatalf("dial %d received an empty address; the configured address must be reused after the connection is cleared", i)
		}
	}
	if got := d.snapshot()[0]; got != reconnectTestAddr {
		t.Fatalf("first dial address = %q, want %q", got, reconnectTestAddr)
	}
}

func TestManager_ReconnectRetriesAreBounded(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		wantDials  int
	}{
		{"explicit bound", 3, 1 + 3},
		{"zero is not unlimited", 0, 1 + push.DefaultMaxRetries},
		{"negative is not unlimited", -5, 1 + push.DefaultMaxRetries},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &dialRecorder{}
			newReconnectManager(t, d, push.WithReconnectMaxRetries(tt.maxRetries))
			waitForDials(t, d, tt.wantDials)

			// Allow extra time to prove no further attempt is made.
			time.Sleep(50 * time.Millisecond)
			if got := len(d.snapshot()); got != tt.wantDials {
				t.Fatalf("dials = %d, want %d: the reconnect walk must stop at its bound", got, tt.wantDials)
			}
		})
	}
}

func TestManager_ReconnectSucceedsAfterTransientFailures(t *testing.T) {
	var mu sync.Mutex
	var addrs []string
	attempts := 0

	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		mu.Lock()
		addrs = append(addrs, addr)
		attempts++
		n := attempts
		mu.Unlock()
		switch n {
		case 1:
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		case 2, 3:
			return nil, errors.New("temporarily unavailable")
		default:
			// The walk succeeds here. The peer is closed immediately so the read
			// loop fails again and starts another walk; the test only asserts
			// that this attempt was made and that no dial saw an empty address.
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		}
	}

	m := push.NewManager(nil,
		push.WithManagerDialFunc(dial),
		push.WithReconnectBackoff(time.Millisecond, time.Millisecond),
		push.WithHeartbeatInterval(0),
	)
	t.Cleanup(func() { _ = m.Close() })
	if err := m.Dial(context.Background(), reconnectTestAddr); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := attempts
		mu.Unlock()
		if n >= 4 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts < 4 {
		t.Fatalf("attempts = %d, want at least 4: the walk should recover after transient failures", attempts)
	}
	for i, addr := range addrs {
		if addr == "" {
			t.Fatalf("dial %d received an empty address", i)
		}
	}
}
