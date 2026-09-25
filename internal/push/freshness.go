// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrNotStale is the sentinel wrapped by every Check error, so a caller can tell
// "this topic has no record" and "this topic has gone quiet" apart with
// errors.Is without matching on message text.
var ErrNotStale = errors.New("push: not stale")

// FreshnessEntry is the last recorded activity for one topic.
type FreshnessEntry struct {
	// LastSeq is the highest sequence number seen, or 0 if the Gateway has never
	// supplied one. The push envelope carries no per-event sequence, so this
	// stays 0 in practice; it is retained so a future Gateway that does supply
	// one needs no API change.
	LastSeq uint64
	// LastSeen is when Record was last called for this topic.
	LastSeen time.Time
	// TopicID is the topic this entry describes.
	TopicID int
}

// FreshnessMonitor tracks per-topic liveness so a caller can detect a topic that
// has gone silent. It is time-based rather than sequence-based, which is what
// makes it usable at all: the Gateway push envelope carries no per-event
// sequence number, so sequence-based gap detection is impossible (see the R4
// entry in docs/threat-model.md).
type FreshnessMonitor struct {
	entries map[int]FreshnessEntry
	mu      sync.RWMutex
	tol     time.Duration
}

// NewFreshnessMonitor returns a monitor that reports a topic as stale after tol
// of silence. A non-positive tol restores the 5-minute default rather than
// accepting zero, which would mark every topic stale on the first Check.
func NewFreshnessMonitor(tol time.Duration) *FreshnessMonitor {
	if tol <= 0 {
		tol = 5 * time.Minute
	}
	return &FreshnessMonitor{entries: make(map[int]FreshnessEntry), tol: tol}
}

// Record notes activity on topicID. LastSeen is always updated, so time-based
// staleness detection works even when no sequence number is available. LastSeq
// only advances when a non-zero seq is supplied.
func (m *FreshnessMonitor) Record(topicID int, seq uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[topicID]
	if seq > entry.LastSeq {
		entry.LastSeq = seq
	}
	entry.LastSeen = time.Now()
	entry.TopicID = topicID
	m.entries[topicID] = entry
}

// Check reports the last activity for topicID. It returns an error wrapping
// ErrNotStale both when the topic has never been recorded and when it has been
// silent for longer than the tolerance; the returned entry is still populated in
// the stale case, so a caller can report how long the topic has been quiet.
func (m *FreshnessMonitor) Check(topicID int) (FreshnessEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[topicID]
	if !ok {
		return FreshnessEntry{}, fmt.Errorf("push: no freshness record for topic %d: %w", topicID, ErrNotStale)
	}
	if time.Since(entry.LastSeen) > m.tol {
		return entry, fmt.Errorf("push: topic %d is stale (last seen %v ago): %w",
			topicID, time.Since(entry.LastSeen), ErrNotStale)
	}
	return entry, nil
}

// Clear discards every recorded topic.
func (m *FreshnessMonitor) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[int]FreshnessEntry)
}
