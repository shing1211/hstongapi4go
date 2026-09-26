// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestNewWithoutClientFallsBackToDefaultPushAddr covers the push-address
// fallback that only a nil *client.Client can reach. With no client there is no
// PushAddr to inherit, so New must fall back to the push package default;
// dialling the empty string instead would fail every Connect with an
// unrecognised-address error.
func TestNewWithoutClientFallsBackToDefaultPushAddr(t *testing.T) {
	s := New(nil)
	t.Cleanup(func() { _ = s.Close() })

	if s.opts.pushAddr != push.DefaultAddr {
		t.Errorf("pushAddr = %q, want %q", s.opts.pushAddr, push.DefaultAddr)
	}
	if s.market != nil {
		t.Error("a market manager was built for a nil client")
	}
}

// TestNewNormalizesBufferAndIgnoresNilOption covers the buffer normalization.
// WithBuffer refuses a non-positive depth, but Option is exported, so a caller
// can install a raw option that sets one; New must still refuse to build
// zero-capacity channels, which would turn every later send into a permanent
// drop.
func TestNewNormalizesBufferAndIgnoresNilOption(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want int
	}{
		{"a raw option sets a negative buffer", []Option{func(o *options) { o.buffer = -1 }}, defaultBuffer},
		{"a raw option clears the buffer", []Option{func(o *options) { o.buffer = 0 }}, defaultBuffer},
		{"WithBuffer refuses zero", []Option{WithBuffer(0)}, defaultBuffer},
		{"WithBuffer refuses a negative depth", []Option{WithBuffer(-8)}, defaultBuffer},
		{"WithBuffer installs a positive depth", []Option{WithBuffer(8)}, 8},
		{"a nil option is skipped", []Option{nil, WithBuffer(8)}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(nil, tt.opts...)
			t.Cleanup(func() { _ = s.Close() })
			if s.opts.buffer != tt.want {
				t.Errorf("buffer = %d, want %d", s.opts.buffer, tt.want)
			}
		})
	}
}

// TestConnectGuards covers the three entry guards of Connect. A second Connect
// must not redial, because a fresh push connection drops a healthy stream and
// silently loses every subscription registered on it; and a caller that has
// closed the client, or never supplied one, must be told rather than handed a
// half-started client.
func TestConnectGuards(t *testing.T) {
	t.Run("after Close", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		d := &pipeDialer{}
		s := newTestClient(t, srv.url, d)

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := s.Connect(context.Background()); !errors.Is(err, ErrClosed) {
			t.Errorf("Connect after Close = %v, want ErrClosed", err)
		}
		if got := d.count(); got != 0 {
			t.Errorf("dialled %d times after Close, want 0", got)
		}
	})

	t.Run("a second Connect reuses the live connection", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		d := &pipeDialer{}
		s := newTestClient(t, srv.url, d)

		ctx := context.Background()
		if err := s.Connect(ctx); err != nil {
			t.Fatalf("first Connect: %v", err)
		}
		firstRun := s.ctx
		if err := s.Connect(ctx); err != nil {
			t.Fatalf("second Connect: %v", err)
		}
		if got := d.count(); got != 1 {
			t.Errorf("dialled %d times, want 1: a repeated Connect must reuse the live connection", got)
		}
		// The second Connect must be a complete no-op. Re-deriving the run
		// context would orphan the first one and leave three extra goroutines
		// reading the connection the first Connect installed.
		if s.ctx != firstRun {
			t.Error("a repeated Connect re-derived the run context instead of returning early")
		}
		if !s.started {
			t.Error("started is false after a successful Connect")
		}
	})

	t.Run("no underlying client", func(t *testing.T) {
		s := New(nil)
		t.Cleanup(func() { _ = s.Close() })

		if err := s.Connect(context.Background()); !errors.Is(err, ErrNoClient) {
			t.Errorf("Connect without a client = %v, want ErrNoClient", err)
		}
	})
}

// TestConnectDialFailureReleasesTheRunContext covers the dial-failure path. The
// dial error must reach the caller, the client must not be marked started so a
// later Connect can dial again, and the derived run context must be cancelled
// rather than left live on a client that never connected.
func TestConnectDialFailureReleasesTheRunContext(t *testing.T) {
	srv := newHookServer(t, nil, nil)
	dialErr := errors.New("push dial refused")
	d := &pipeDialer{before: func(int) error { return dialErr }}
	s := newTestClient(t, srv.url, d)

	ctx := context.Background()
	if err := s.Connect(ctx); !errors.Is(err, dialErr) {
		t.Fatalf("Connect = %v, want the dial error", err)
	}
	if s.started {
		t.Error("started is true after a failed dial")
	}
	if s.ctx == nil {
		t.Fatal("run context is nil; Connect must publish it for the caller")
	}
	if err := s.ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("run context error = %v, want context.Canceled: a failed Connect must release it", err)
	}

	// started stayed false, so a second Connect is allowed to dial again rather
	// than reporting a success it never achieved.
	if err := s.Connect(ctx); !errors.Is(err, dialErr) {
		t.Fatalf("second Connect = %v, want the dial error", err)
	}
	if got := d.count(); got != 2 {
		t.Errorf("dialled %d times, want 2", got)
	}
}

// TestConnectDiscardsConnectionWhenClosedDuringDial covers the window between a
// successful dial and the store of the started flag.
//
// In production that window is opened by shutdown, which sets the closed flag
// and then closes the push client: a Connect landing between the two dials
// successfully and must still refuse to start, or it would park a fresh
// connection and three goroutines on a client the caller has already torn
// down. The test reproduces the intermediate state on the Connect goroutine
// itself by setting the closed flag from inside the dialer and leaving the push
// client alone, so no second goroutine and no sleep is involved.
func TestConnectDiscardsConnectionWhenClosedDuringDial(t *testing.T) {
	srv := newHookServer(t, nil, nil)

	var s *Client
	d := &pipeDialer{before: func(int) error {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		return nil
	}}
	s = newTestClient(t, srv.url, d)

	if err := s.Connect(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Connect = %v, want ErrClosed", err)
	}
	if s.started {
		t.Error("started is true although the client was closed during the dial")
	}

	// The connection the dial produced must be released by Close rather than
	// leaked: the pipe peer reports EOF only once the local end is closed.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := d.peer(0).Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read from the dialled peer = %v, want io.EOF (the connection was released)", err)
	}
}

// TestForwardPushErrorsStopsOnClosedPushChannel covers the loop's exit on a
// closed push error channel.
//
// Receiving from a closed channel succeeds immediately and forever, so without
// the ok guard this loop would spin at full CPU for as long as the stream
// client lives, which is the failure mode the guard exists to prevent. The
// test drives the loop with a context it never cancels, so the closed channel
// is the only way out, and waits on the loop's own exit rather than on a clock.
func TestForwardPushErrorsStopsOnClosedPushChannel(t *testing.T) {
	s := newBareClient(t)
	s.push = push.New()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.forwardPushErrors(context.Background())
	}()

	// While the push error channel is open and the context is live there is
	// nothing to receive and nothing to select on, so the loop cannot have
	// returned. This assertion is therefore safe to make immediately.
	select {
	case <-done:
		t.Fatal("forwardPushErrors returned while the push error channel was still open")
	default:
	}

	if err := s.push.Close(); err != nil {
		t.Fatalf("push Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(waitBound):
		t.Fatal("forwardPushErrors did not return after the push error channel was closed")
	}
}

// TestDeliverIgnoresNilAndEmptyFanout covers both of deliver's guards. A nil
// notification reaches the handler when a decode path yields nothing, and an
// empty subscription set is routine between a Cancel and the next Subscribe.
// Neither may panic, because deliver runs on the push dispatcher's goroutine
// and a panic there would take down the whole read loop.
func TestDeliverIgnoresNilAndEmptyFanout(t *testing.T) {
	t.Run("nil notification with subscriptions registered", func(t *testing.T) {
		s := newBareClient(t)
		s.push = push.New()
		// The subscription is registered so the fan-out is actually reached:
		// deliver reads n.Type and n.ID, so a missing nil guard would dereference
		// a nil notification on the dispatcher goroutine.
		sub := &Subscription{
			updates: make(chan Event, 1),
			errs:    make(chan error, 1),
			accepts: []types.NotifyMsgType{types.BasicQotNotifyMsgType},
		}
		s.subs[sub] = struct{}{}

		s.deliver(nil)

		if got := len(sub.updates); got != 0 {
			t.Errorf("a nil notification queued %d events, want 0", got)
		}
	})

	t.Run("notification with no subscriptions registered", func(t *testing.T) {
		s := newBareClient(t)
		s.push = push.New()

		s.deliver(&push.Notification{
			Type: types.BasicQotNotifyMsgType,
			ID:   "0700.HK",
			Time: 1700000000000,
		})
		if got := subscriptionCount(s); got != 0 {
			t.Errorf("deliver registered %d subscriptions, want 0", got)
		}
	})
}

// TestAcceptsTypeRejectsForeignNotifyTypes pins the fan-out filter. A
// subscription created for one topic must not receive another topic's
// notifications: the Event would reach the caller under a type its accessors do
// not accept, so the payload would be silently dropped after the caller had
// already been told an update arrived.
func TestAcceptsTypeRejectsForeignNotifyTypes(t *testing.T) {
	tests := []struct {
		name    string
		accepts []types.NotifyMsgType
		typ     types.NotifyMsgType
		want    bool
	}{
		{
			name:    "the accepted type",
			accepts: []types.NotifyMsgType{types.TickerNotifyMsgType},
			typ:     types.TickerNotifyMsgType,
			want:    true,
		},
		{
			name:    "a different market topic",
			accepts: []types.NotifyMsgType{types.TickerNotifyMsgType},
			typ:     types.BasicQotNotifyMsgType,
			want:    false,
		},
		{
			name:    "a trade delivery",
			accepts: []types.NotifyMsgType{types.TickerNotifyMsgType},
			typ:     types.TradeStockDeliverMsgType,
			want:    false,
		},
		{
			name:    "an unregistered type",
			accepts: []types.NotifyMsgType{types.TickerNotifyMsgType},
			typ:     types.NotifyMsgType(4242),
			want:    false,
		},
		{
			name:    "an empty accept list",
			accepts: nil,
			typ:     types.TickerNotifyMsgType,
			want:    false,
		},
		{
			name:    "the last of several accepted types",
			accepts: []types.NotifyMsgType{types.OrderBookNotifyMsgType, types.BrokerQueueNotifyMsgType},
			typ:     types.BrokerQueueNotifyMsgType,
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &Subscription{accepts: tt.accepts}
			if got := sub.acceptsType(tt.typ); got != tt.want {
				t.Errorf("acceptsType(%d) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}
