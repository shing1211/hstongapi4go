// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestFreshnessMonitor_Record asserts LastSeq tracks the highest sequence seen,
// not the most recent one: a reordered or replayed lower sequence must not walk
// the recorded position backwards.
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

// TestFreshnessMonitor_RecordWithoutSequence asserts that time-based freshness
// is recorded even when the caller has no sequence number. The Gateway push
// envelope carries none, so this is the normal case rather than an edge case;
// Record previously returned early for seq == 0, which meant freshness was
// never populated at all and every staleness check reported "no freshness
// record".
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

// TestFreshnessMonitor_Stale asserts a topic silent for longer than the
// tolerance is reported stale, and that the error wraps the ErrNotStale
// sentinel so a caller can classify it with errors.Is rather than by message.
func TestFreshnessMonitor_Stale(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(10 * time.Millisecond)

	m.Record(1, 1)
	time.Sleep(20 * time.Millisecond)

	entry, err := m.Check(1)
	if err == nil {
		t.Fatal("Check on a silent topic = nil, want a stale error")
	}
	if !errors.Is(err, ErrNotStale) {
		t.Errorf("Check error %v does not wrap ErrNotStale", err)
	}
	// The entry is still returned in the stale case so a caller can report how
	// long the topic has been quiet.
	if entry.TopicID != 1 {
		t.Errorf("stale entry TopicID = %d, want 1", entry.TopicID)
	}
}

// TestFreshnessMonitor_UnrecordedTopic pins the other ErrNotStale case: a topic
// never seen is not the same condition as a topic that went quiet, and the
// returned entry must be zero.
func TestFreshnessMonitor_UnrecordedTopic(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(time.Minute)

	entry, err := m.Check(99)
	if err == nil {
		t.Fatal("Check on an unrecorded topic = nil, want an error")
	}
	if !errors.Is(err, ErrNotStale) {
		t.Errorf("Check error %v does not wrap ErrNotStale", err)
	}
	if !entry.LastSeen.IsZero() || entry.LastSeq != 0 || entry.TopicID != 0 {
		t.Errorf("entry = %+v, want the zero value for an unrecorded topic", entry)
	}
}

// TestFreshnessMonitor_NonPositiveToleranceRestoresDefault guards the zero case:
// a zero tolerance would mark every topic stale on the first Check, so a
// misconfigured value must fall back to the documented default instead.
func TestFreshnessMonitor_NonPositiveToleranceRestoresDefault(t *testing.T) {
	defer goleak.VerifyNone(t)
	for _, tol := range []time.Duration{0, -time.Second} {
		m := NewFreshnessMonitor(tol)
		if m.tol != 5*time.Minute {
			t.Errorf("NewFreshnessMonitor(%v) tolerance = %v, want the 5m default", tol, m.tol)
		}
		m.Record(1, 0)
		if _, err := m.Check(1); err != nil {
			t.Errorf("Check immediately after Record with tol=%v = %v, want fresh", tol, err)
		}
	}
}

// TestFreshnessMonitor_Clear asserts Clear drops every record, so a caller can
// reset state after a reconnect without leaking a stale window into the new
// connection.
func TestFreshnessMonitor_Clear(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(time.Minute)
	m.Record(1, 0)
	m.Record(2, 0)

	m.Clear()

	for _, topic := range []int{1, 2} {
		if _, err := m.Check(topic); !errors.Is(err, ErrNotStale) {
			t.Errorf("Check(%d) after Clear = %v, want ErrNotStale", topic, err)
		}
	}
}

// TestFreshnessMonitor_TopicsAreIndependent asserts the monitor keeps a separate
// record per topic, so activity on a busy topic cannot mask a silent one.
func TestFreshnessMonitor_TopicsAreIndependent(t *testing.T) {
	defer goleak.VerifyNone(t)
	m := NewFreshnessMonitor(10 * time.Millisecond)

	m.Record(1, 0)
	m.Record(2, 0)
	time.Sleep(20 * time.Millisecond)

	// Re-record only topic 1; topic 2 must still read as stale.
	m.Record(1, 0)

	if _, err := m.Check(1); err != nil {
		t.Errorf("Check(1) after re-record = %v, want fresh", err)
	}
	if _, err := m.Check(2); !errors.Is(err, ErrNotStale) {
		t.Errorf("Check(2) = %v, want it still stale: topic 1's activity must not mask it", err)
	}
}
