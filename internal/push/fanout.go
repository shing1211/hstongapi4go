// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

type EventSource interface {
	Updates() <-chan *domain.PushEvent
	Errors() <-chan error
	Done() <-chan struct{}
}

type FreshnessEntry struct {
	LastSeq  uint64
	LastSeen time.Time
	TopicID  int
}

type DedupCache struct {
	entries map[uint64]time.Time
	mu      sync.Mutex
	cap     int
}

func NewDedupCache(cap int) *DedupCache {
	if cap <= 0 {
		cap = 1024
	}
	return &DedupCache{entries: make(map[uint64]time.Time), cap: cap}
}

func (c *DedupCache) CheckAndInsert(seq uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[seq]; ok {
		return true
	}
	if len(c.entries) >= c.cap {
		var oldest uint64
		var oldestTime time.Time
		for s, t := range c.entries {
			if oldestTime.IsZero() || t.Before(oldestTime) {
				oldest, oldestTime = s, t
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[seq] = time.Now()
	return false
}

func (c *DedupCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[uint64]time.Time)
}

var ErrNotStale = errors.New("push: not stale")

type FreshnessMonitor struct {
	entries map[int]FreshnessEntry
	mu      sync.RWMutex
	tol     time.Duration
}

func NewFreshnessMonitor(tol time.Duration) *FreshnessMonitor {
	if tol <= 0 {
		tol = 5 * time.Minute
	}
	return &FreshnessMonitor{entries: make(map[int]FreshnessEntry), tol: tol}
}

func (m *FreshnessMonitor) Record(topicID int, seq uint64) {
	if seq == 0 {
		return
	}
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

func (m *FreshnessMonitor) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[int]FreshnessEntry)
}

type BackpressurePolicy int

const (
	BackpressureDropOldest BackpressurePolicy = iota
	BackpressureDropNewest
	BackpressureBlock
)

type SubscriberOption func(*Subscriber)

func WithBufferSize(n int) SubscriberOption {
	return func(s *Subscriber) {
		if n > 0 {
			s.ch = make(chan domain.PushEvent, n)
		}
	}
}

func WithBackpressurePolicy(p BackpressurePolicy) SubscriberOption {
	return func(s *Subscriber) {
		s.bp = p
	}
}

type Subscriber struct {
	ID      int64
	TopicID int
	ch      chan domain.PushEvent
	Errors  chan error
	closed  atomic.Bool
	bp      BackpressurePolicy
}

func (s *Subscriber) Updates() <-chan domain.PushEvent { return s.ch }
func (s *Subscriber) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	close(s.ch)
	close(s.Errors)
}

var subIDSeed int64
var subIDMu sync.Mutex

func nextSubID() int64 {
	subIDMu.Lock()
	id := subIDSeed
	subIDSeed++
	subIDMu.Unlock()
	return id
}

func newSubscriber(topicID int, opts ...SubscriberOption) *Subscriber {
	s := &Subscriber{
		ID:      nextSubID(),
		TopicID: topicID,
		ch:      make(chan domain.PushEvent, 64),
		Errors:  make(chan error, 8),
		bp:      BackpressureDropOldest,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Fanout struct {
	src     EventSource
	subs    map[int64]*Subscriber
	byTopic map[int][]int64
	mu      sync.RWMutex
	dedup   *DedupCache
	fresh   *FreshnessMonitor
	closed  atomic.Bool
	wg      sync.WaitGroup
	stop    chan struct{}
}

type FanoutOption func(*Fanout)

func WithDedupCacheSize(n int) FanoutOption {
	return func(f *Fanout) {
		f.dedup = NewDedupCache(n)
	}
}

func WithFreshnessTolerance(d time.Duration) FanoutOption {
	return func(f *Fanout) {
		f.fresh = NewFreshnessMonitor(d)
	}
}

func NewFanout(src EventSource, opts ...FanoutOption) *Fanout {
	f := &Fanout{
		src:     src,
		subs:    make(map[int64]*Subscriber),
		byTopic: make(map[int][]int64),
		dedup:   NewDedupCache(1024),
		fresh:   NewFreshnessMonitor(5 * time.Minute),
		stop:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(f)
	}
	f.wg.Add(1)
	go f.run()
	return f
}

func (f *Fanout) run() {
	defer f.wg.Done()
	for {
		select {
		case <-f.stop:
			return
		case <-f.src.Done():
			f.broadcastErr(errors.New("push: source closed"))
			return
		case err, ok := <-f.src.Errors():
			if !ok {
				return
			}
			if err != nil {
				f.broadcastErr(err)
			}
		case ev, ok := <-f.src.Updates():
			if !ok {
				return
			}
			if ev != nil {
				f.dispatch(*ev)
			}
		}
	}
}

func (f *Fanout) broadcastErr(err error) {
	if err == nil {
		return
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, s := range f.subs {
		select {
		case s.Errors <- err:
		default:
		}
	}
}

func (f *Fanout) dispatch(ev domain.PushEvent) {
	if ev == nil {
		return
	}
	topicID := topicOf(ev)
	seq := seqOf(ev)

	if seq != 0 && f.dedup.CheckAndInsert(seq) {
		return
	}
	f.fresh.Record(topicID, seq)

	f.mu.RLock()
	for _, id := range f.byTopic[topicID] {
		s := f.subs[id]
		if s == nil || s.closed.Load() {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			switch s.bp {
			case BackpressureDropOldest:
				select {
				case <-s.ch:
				default:
				}
				select {
				case s.ch <- ev:
				default:
				}
			case BackpressureDropNewest:
			case BackpressureBlock:
				select {
				case s.ch <- ev:
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	}
	f.mu.RUnlock()
}

func (f *Fanout) Subscribe(_ context.Context, topicID int, opts ...SubscriberOption) (*Subscriber, error) {
	if f.closed.Load() {
		return nil, errors.New("push: fanout closed")
	}
	s := newSubscriber(topicID, opts...)
	f.mu.Lock()
	f.subs[s.ID] = s
	f.byTopic[topicID] = append(f.byTopic[topicID], s.ID)
	f.mu.Unlock()
	return s, nil
}

func (f *Fanout) Unsubscribe(_ context.Context, s *Subscriber) error {
	if f.closed.Load() {
		return errors.New("push: fanout closed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.subs[s.ID]; !ok {
		return errors.New("push: subscriber not found")
	}
	delete(f.subs, s.ID)
	ids := f.byTopic[s.TopicID]
	for i, id := range ids {
		if id == s.ID {
			f.byTopic[s.TopicID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	s.Close()
	return nil
}

func (f *Fanout) Close() error {
	if !f.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(f.stop)
	f.wg.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		s.Close()
	}
	return nil
}

func (f *Fanout) Freshness(topicID int) (FreshnessEntry, error) {
	return f.fresh.Check(topicID)
}

func (f *Fanout) ClearDedup() {
	f.dedup.Clear()
}

func topicOf(ev domain.PushEvent) int {
	switch ev.(type) {
	case *domain.QuoteEvent:
		return 1
	case *domain.TickerEvent:
		return 2
	case *domain.OrderBookEvent:
		return 3
	case *domain.BrokerEvent:
		return 4
	case *domain.TradeEvent:
		return 5
	default:
		return 0
	}
}

func seqOf(ev domain.PushEvent) uint64 {
	return 0
}
