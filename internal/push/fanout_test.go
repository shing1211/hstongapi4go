// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

type mockEventSource struct {
	mu      sync.Mutex
	updates chan *domain.PushEvent
	errors  chan error
	done    chan struct{}
	closed  bool
}

func newMockSource() *mockEventSource {
	return &mockEventSource{
		updates: make(chan *domain.PushEvent, 64),
		errors:  make(chan error, 64),
		done:    make(chan struct{}),
	}
}

func (m *mockEventSource) Updates() <-chan *domain.PushEvent { return m.updates }
func (m *mockEventSource) Errors() <-chan error              { return m.errors }
func (m *mockEventSource) Done() <-chan struct{}             { return m.done }

func (m *mockEventSource) send(ev domain.PushEvent) {
	select {
	case m.updates <- &ev:
	default:
	}
}

func (m *mockEventSource) close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.done)
	m.mu.Unlock()
}

func (m *mockEventSource) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func TestFanout_Subscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub, err := f.Subscribe(context.Background(), 1)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if sub == nil {
		t.Fatal("sub is nil")
	}
	if sub.TopicID != 1 {
		t.Errorf("TopicID = %d, want 1", sub.TopicID)
	}

	src.send(&domain.QuoteEvent{})
	select {
	case ev := <-sub.Updates():
		if ev == nil {
			t.Error("received nil event")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestFanout_Unsubscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub, _ := f.Subscribe(context.Background(), 1)

	err := f.Unsubscribe(context.Background(), sub)
	if err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}

	src.send(&domain.QuoteEvent{})
	waitFor(t, 200*time.Millisecond, func() bool {
		select {
		case <-sub.Updates():
			return true
		default:
			return false
		}
	})
}

func TestFanout_MultiSubscriber(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub1, _ := f.Subscribe(context.Background(), 1)
	sub2, _ := f.Subscribe(context.Background(), 1)

	src.send(&domain.QuoteEvent{})

	for i, sub := range []*Subscriber{sub1, sub2} {
		select {
		case <-sub.Updates():
		case <-time.After(time.Second):
			t.Errorf("sub%d: timeout", i+1)
		}
	}
}

func TestFanout_SourceCloseBroadcastsError(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub, _ := f.Subscribe(context.Background(), 1)

	src.close()

	select {
	case err := <-sub.Errors:
		if err == nil {
			t.Error("expected non-nil error")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for error")
	}
}

func TestFanout_BackpressureDropOldest(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub, _ := f.Subscribe(context.Background(), 1, WithBufferSize(2), WithBackpressurePolicy(BackpressureDropOldest))

	for i := 0; i < 10; i++ {
		src.send(&domain.QuoteEvent{})
	}

	time.Sleep(100 * time.Millisecond)

	count := len(sub.ch)
	if count > 2 {
		t.Errorf("buffer size = %d, want <= 2", count)
	}
}

func TestFanout_CloseIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)

	f.Close()
	f.Close()

	select {
	case <-f.stop:
	default:
		t.Error("stop channel should be closed after first Close")
	}
}

func TestFanout_CloseBeforeSubscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	f.Close()

	_, err := f.Subscribe(context.Background(), 1)
	if err == nil {
		t.Error("Subscribe after Close should fail")
	}
}

func TestFanout_TopicRouting(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	sub1, _ := f.Subscribe(context.Background(), 1)
	sub2, _ := f.Subscribe(context.Background(), 2)

	src.send(&domain.QuoteEvent{})

	select {
	case <-sub1.Updates():
	case <-time.After(time.Second):
		t.Error("sub1 timeout")
	}

	src.send(&domain.TickerEvent{})

	select {
	case <-sub2.Updates():
	case <-time.After(time.Second):
		t.Error("sub2 timeout")
	}

	select {
	case <-sub1.Updates():
		t.Error("sub1 should not receive TickerEvent")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestFreshnessMonitor_Record(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(time.Minute)

	m.Record(1, 10)
	m.Record(1, 5)
	m.Record(1, 20)

	entry, err := m.Check(1)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if entry.LastSeq != 20 {
		t.Errorf("LastSeq = %d, want 20", entry.LastSeq)
	}
}

func TestFreshnessMonitor_Stale(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(10 * time.Millisecond)

	m.Record(1, 1)
	time.Sleep(20 * time.Millisecond)

	_, err := m.Check(1)
	if err == nil {
		t.Error("expected stale error")
	}
}

func TestDedupCache_Dedup(t *testing.T) {
	defer goleak.VerifyNone(t)
	c := NewDedupCache(100)

	if c.CheckAndInsert(1) {
		t.Error("first insert should return false")
	}
	if !c.CheckAndInsert(1) {
		t.Error("duplicate should return true")
	}
	if !c.CheckAndInsert(1) {
		t.Error("duplicate should return true")
	}
}

func waitFor(t *testing.T, d time.Duration, fn func() bool) {
	deadline := time.After(d)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for condition")
		default:
			if fn() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type mockSourceWithCounter struct {
	*mockEventSource
	closeCount atomic.Int32
}

func (m *mockSourceWithCounter) close() {
	m.closeCount.Add(1)
	m.mockEventSource.close()
}
