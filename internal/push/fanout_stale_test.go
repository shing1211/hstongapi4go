// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// TestFreshnessMonitor_RecordWithoutSequence asserts that time-based freshness
// is recorded even when the caller has no sequence number. Previously Record
// returned early for seq == 0, which meant the fanout — where seqOf always
// yields 0 — never populated freshness at all and every staleness check
// reported "no freshness record".
func TestFreshnessMonitor_RecordWithoutSequence(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(time.Minute)

	m.Record(1, 0)

	entry, err := m.Check(1)
	if err != nil {
		t.Fatalf("Check after Record(topic, 0): %v, want a freshness record", err)
	}
	if entry.LastSeen.IsZero() {
		t.Error("LastSeen is zero, want the activity timestamp recorded")
	}
	if entry.TopicID != 1 {
		t.Errorf("TopicID = %d, want 1", entry.TopicID)
	}
}

func TestFreshnessMonitor_ZeroSequenceDoesNotAdvanceLastSeq(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(time.Minute)

	m.Record(1, 10)
	m.Record(1, 0)

	entry, err := m.Check(1)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if entry.LastSeq != 10 {
		t.Errorf("LastSeq = %d, want 10: a zero sequence must not advance it", entry.LastSeq)
	}
}

// TestFanout_DispatchPopulatesFreshness asserts the real dispatch path records
// freshness, so a consumer watching a quiet topic can be told it has gone stale.
func TestFanout_DispatchPopulatesFreshness(t *testing.T) {
	defer goleak.VerifyNone(t)
	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()

	if _, err := f.Freshness(1); err == nil {
		t.Fatal("Freshness(1) succeeded before any event, want no record")
	}

	src.send(&domain.QuoteEvent{})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := f.Freshness(1); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	_, err := f.Freshness(1)
	t.Fatalf("Freshness(1) after a dispatch = %v, want a freshness record", err)
}

// TestFanout_DedupInertWithoutSequence pins a known limitation rather than
// asserting a fix. seqOf returns a constant 0 because the Gateway push
// envelope carries no per-event sequence, so the dedup guard in dispatch never
// engages and no gap detection is possible. Removing this test, or flipping it
// to require suppression, would be the signal that a real sequence source now
// exists.
func TestFanout_DedupInertWithoutSequence(t *testing.T) {
	defer goleak.VerifyNone(t)
	if got := seqOf(&domain.QuoteEvent{}); got != 0 {
		t.Fatalf("seqOf = %d, want 0: the Gateway supplies no per-event sequence", got)
	}

	cache := NewDedupCache(8)
	if cache.CheckAndInsert(7) {
		t.Fatal("DedupCache.CheckAndInsert(7) = true, want false (newly inserted, not a duplicate)")
	}
	if !cache.CheckAndInsert(7) {
		t.Fatal("DedupCache.CheckAndInsert(7) repeat = false, want true (duplicate)")
	}

	src := newMockSource()
	f := NewFanout(src)
	defer f.Close()
	sub, _ := f.Subscribe(t.Context(), 1)

	// Two identical events both reach the subscriber because dedup is inactive.
	src.send(&domain.QuoteEvent{})
	src.send(&domain.QuoteEvent{})

	received := 0
	for received < 2 {
		select {
		case <-sub.Updates():
			received++
		case <-time.After(time.Second):
			t.Fatalf("received %d events, want 2: dedup is inert without a sequence", received)
		}
	}
}
