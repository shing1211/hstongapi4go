// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestEntrustBSFromInt32 covers every branch of the wire-side mapping. This one
// matters more than its size suggests: it decides whether a trade delivery is
// labelled buy or sell, and a wrong mapping silently mislabels a fill rather
// than failing. Only one of the five cases had any coverage before this.
//
// The default branch deliberately preserves an unrecognised value as its decimal
// string instead of collapsing it to a known side, so an unmapped code is
// visible to the caller rather than silently reported as a buy.
func TestEntrustBSFromInt32(t *testing.T) {
	tests := []struct {
		name string
		in   int32
		want types.EntrustBS
	}{
		{"buy", 1, types.EntrustBuy},
		{"sell", 2, types.EntrustSell},
		{"close short", 3, types.EntrustCloseShort},
		{"open short", 4, types.EntrustOpenShort},
		{"zero is unmapped", 0, types.EntrustBS("0")},
		{"five is unmapped", 5, types.EntrustBS("5")},
		{"negative is unmapped", -1, types.EntrustBS("-1")},
		{"large is unmapped", 9999, types.EntrustBS("9999")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entrustBSFromInt32(tt.in)
			if got != tt.want {
				t.Errorf("entrustBSFromInt32(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestEntrustBSMappingIsDistinct guards against two wire codes collapsing to the
// same side, which a copy-paste in the switch would cause.
func TestEntrustBSMappingIsDistinct(t *testing.T) {
	seen := make(map[types.EntrustBS]int32, 4)
	for _, side := range []int32{1, 2, 3, 4} {
		got := entrustBSFromInt32(side)
		if prev, dup := seen[got]; dup {
			t.Errorf("wire codes %d and %d both map to %q", prev, side, got)
			continue
		}
		seen[got] = side
	}
	if len(seen) != 4 {
		t.Errorf("got %d distinct sides from 4 wire codes, want 4", len(seen))
	}
}

// TestClientAddr pins the accessor, which callers use to confirm which Gateway
// endpoint a Client is bound to.
func TestClientAddr(t *testing.T) {
	c := New(WithAddr("127.0.0.1:19999"))
	t.Cleanup(func() { _ = c.Close() })

	if got := c.Addr(); got != "127.0.0.1:19999" {
		t.Errorf("Addr() = %q, want 127.0.0.1:19999", got)
	}
}

// TestClientErrorsChannel asserts the error channel exists and that reporting an
// error with no reader attached does not block. A nil or unbuffered channel
// would turn every report into a silent no-op or a stalled read loop.
func TestClientErrorsChannel(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	if c.Errors() == nil {
		t.Fatal("Errors() = nil, want a usable channel")
	}

	// report must return promptly even though nothing is reading. The bound is
	// generous because the failure it guards against is a permanent block, not a
	// slow one.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 4 {
			c.report(errors.New("probe"))
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Error("report blocked with no reader on the error channel")
	}
}

// TestClientExitCause covers the three states the run loop distinguishes: a
// deliberate Close, a cancelled context, and a live context. Getting these
// apart wrongly would make a normal shutdown look like a transport failure.
func TestClientExitCause(t *testing.T) {
	t.Run("closed reports ErrClosed", func(t *testing.T) {
		c := New()
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := c.exitCause(context.Background()); !errors.Is(err, ErrClosed) {
			t.Errorf("exitCause after Close = %v, want ErrClosed", err)
		}
	})

	t.Run("cancelled context reports the context error", func(t *testing.T) {
		c := New()
		t.Cleanup(func() { _ = c.Close() })

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := c.exitCause(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("exitCause on a cancelled ctx = %v, want context.Canceled", err)
		}
	})

	t.Run("live context reports no error", func(t *testing.T) {
		c := New()
		t.Cleanup(func() { _ = c.Close() })

		if err := c.exitCause(context.Background()); err != nil {
			t.Errorf("exitCause on a live ctx = %v, want nil", err)
		}
	})
}

// TestClientUnsubscribeTypes asserts the handler is actually removed, not merely
// shadowed. If it were only shadowed, a later dispatch would still reach the old
// handler goroutine and keep it alive after the caller cancelled.
func TestClientUnsubscribeTypes(t *testing.T) {
	c := New()
	t.Cleanup(func() { _ = c.Close() })

	const t1 = types.TickerNotifyMsgType
	c.SubscribeTypes(t1, func(*Notification) {})
	if len(c.handlers) != 1 {
		t.Fatalf("after SubscribeTypes: %d handlers, want 1", len(c.handlers))
	}

	c.UnsubscribeTypes(t1)
	if len(c.handlers) != 0 {
		t.Errorf("after UnsubscribeTypes: %d handlers, want 0", len(c.handlers))
	}

	// Dispatching to the removed type must be a no-op rather than a panic.
	c.dispatch(&Notification{Type: t1})

	// Unsubscribing something never subscribed must also be safe.
	c.UnsubscribeTypes(types.BrokerQueueNotifyMsgType)
}

// TestNormalizeUnknown covers the catch-all path for a payload whose message
// type is not one this SDK maps. The event must carry the type URL and byte
// count so an operator can identify what arrived, rather than being dropped.
func TestNormalizeUnknown(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		ev, err := NormalizeUnknown(nil)
		if err != nil {
			t.Fatalf("NormalizeUnknown(nil) = %v, want nil", err)
		}
		if ev == nil {
			t.Fatal("NormalizeUnknown(nil) returned a nil event pointer")
		}
		sys, ok := (*ev).(domain.SystemEvent)
		if !ok {
			t.Fatalf("event is %T, want a SystemEvent", *ev)
		}
		if sys.Code != "UNKNOWN" {
			t.Errorf("Code = %q, want UNKNOWN", sys.Code)
		}
		if sys.Level != "warn" {
			t.Errorf("Level = %q, want warn", sys.Level)
		}
	})

	t.Run("unmapped type url", func(t *testing.T) {
		a := &anypb.Any{TypeUrl: "example.com/unmapped", Value: []byte{1, 2, 3}}
		ev, err := NormalizeUnknown(a)
		if err != nil {
			t.Fatalf("NormalizeUnknown = %v, want nil", err)
		}
		if ev == nil {
			t.Fatal("NormalizeUnknown returned a nil event pointer")
		}
		sys, ok := (*ev).(domain.SystemEvent)
		if !ok {
			t.Fatalf("event is %T, want a SystemEvent", *ev)
		}
		if sys.Code != "example.com/unmapped" {
			t.Errorf("Code = %q, want the type URL preserved", sys.Code)
		}
		if want := "unknown push event type: 3 bytes"; sys.Message != want {
			t.Errorf("Message = %q, want %q", sys.Message, want)
		}
	})
}

// TestNormalizeUnknownOnZeroValueAny pins that a zero-value Any is handled like
// any other unmapped payload rather than producing an empty code.
func TestNormalizeUnknownOnZeroValueAny(t *testing.T) {
	ev, err := NormalizeUnknown(&anypb.Any{})
	if err != nil {
		t.Fatalf("NormalizeUnknown(&anypb.Any{}) = %v, want nil", err)
	}
	sys, ok := (*ev).(domain.SystemEvent)
	if !ok {
		t.Fatalf("event is %T, want a SystemEvent", *ev)
	}
	if want := "unknown push event type: 0 bytes"; sys.Message != want {
		t.Errorf("Message = %q, want %q", sys.Message, want)
	}
}

// TestClientRunRespectsCancelledContext covers the Run entry point on a context
// that is already done, so the loop exits without dialing. A Run that ignored
// ctx here would hang a caller who passed an expired deadline.
//
// The context is cancelled before Run is called rather than given a very short
// timeout and slept on, so the test does not depend on wall-clock timing. The
// bound is generous because the failure it guards against is a hang.
func TestClientRunRespectsCancelledContext(t *testing.T) {
	c := New(WithAddr("127.0.0.1:1"))
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Run on a cancelled ctx = nil, want a context error")
		}
	case <-time.After(30 * time.Second):
		t.Error("Run did not return on an already-cancelled context")
	}
}
