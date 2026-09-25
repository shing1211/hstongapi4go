// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestNotifyTypeForTopic covers the topic-to-message-type mapping in full. A
// wrong entry here is silent: the subscription is created, events arrive, and
// they are handed to the caller under the wrong type, so every accessor on the
// Event would report false.
func TestNotifyTypeForTopic(t *testing.T) {
	tests := []struct {
		topic types.TopicID
		want  types.NotifyMsgType
	}{
		{types.TopicBasicQot, types.BasicQotNotifyMsgType},
		{types.TopicQuoteVariant35, types.BasicQotNotifyMsgType},
		{types.TopicTicker, types.TickerNotifyMsgType},
		{types.TopicTickVariant27, types.TickerNotifyMsgType},
		{types.TopicTickVariant28, types.TickerNotifyMsgType},
		{types.TopicTickVariant37, types.TickerNotifyMsgType},
		{types.TopicBroker, types.BrokerQueueNotifyMsgType},
		{types.TopicOrderBook, types.OrderBookNotifyMsgType},
		{types.TopicOrderBookArcabook, types.OrderBookNotifyMsgType},
		{types.TopicOrderBookTotalView, types.OrderBookNotifyMsgType},
		{types.TopicOrderBookVariant36, types.OrderBookNotifyMsgType},
	}
	for _, tt := range tests {
		got, ok := notifyTypeForTopic(tt.topic)
		if !ok {
			t.Errorf("notifyTypeForTopic(%v) ok = false, want true", tt.topic)
			continue
		}
		if got != tt.want {
			t.Errorf("notifyTypeForTopic(%v) = %v, want %v", tt.topic, got, tt.want)
		}
	}
}

// TestNotifyTypeForTopicUnknown pins the failure mode: an unregistered topic
// must report false rather than defaulting to a valid message type, so the
// caller can reject it.
func TestNotifyTypeForTopicUnknown(t *testing.T) {
	for _, topic := range []types.TopicID{0, 999, types.TopicID(-1)} {
		if got, ok := notifyTypeForTopic(topic); ok {
			t.Errorf("notifyTypeForTopic(%v) = %v, true; want false for an unregistered topic", topic, got)
		}
	}
}

// TestSendUpdateDeliversWhenRoom covers the normal path.
func TestSendUpdateDeliversWhenRoom(t *testing.T) {
	sub := &Subscription{updates: make(chan Event, 2), errs: make(chan error, 1)}
	ev := Event{Type: types.BasicQotNotifyMsgType, ID: "00700.HK"}
	sub.sendUpdate(ev)

	select {
	case got := <-sub.updates:
		if got.ID != "00700.HK" {
			t.Errorf("delivered ID = %q, want 00700.HK", got.ID)
		}
	default:
		t.Fatal("sendUpdate did not deliver into a channel with room")
	}
}

// TestSendUpdateDropsOldestWhenFull covers the documented drop-oldest
// backpressure: a slow consumer loses the stale event rather than the fresh one
// or blocking the dispatcher.
func TestSendUpdateDropsOldestWhenFull(t *testing.T) {
	sub := &Subscription{updates: make(chan Event, 1), errs: make(chan error, 1)}

	stale := Event{Type: types.TickerNotifyMsgType, ID: "STALE"}
	fresh := Event{Type: types.TickerNotifyMsgType, ID: "FRESH"}

	sub.updates <- stale
	sub.sendUpdate(fresh)

	got := <-sub.updates
	if got.ID != "FRESH" {
		t.Errorf("after overflow the channel holds %q, want FRESH (drop-oldest)", got.ID)
	}
	select {
	case extra := <-sub.updates:
		t.Errorf("channel holds a second event %q, want exactly one", extra.ID)
	default:
	}
}

// TestSendUpdateOnClosedSubscriptionIsDropped covers the shutdown race: once
// Cancel or Close has run, a late event must be discarded rather than sent on a
// closed channel.
func TestSendUpdateOnClosedSubscriptionIsDropped(t *testing.T) {
	sub := &Subscription{updates: make(chan Event, 1), errs: make(chan error, 1), closed: true}
	sub.sendUpdate(Event{ID: "LATE"})

	select {
	case got := <-sub.updates:
		t.Errorf("closed subscription delivered %q, want the event dropped", got.ID)
	default:
	}
}

// TestSendErr covers the error path, including the nil guard and the same
// drop-oldest behaviour as updates.
func TestSendErr(t *testing.T) {
	sub := &Subscription{updates: make(chan Event, 1), errs: make(chan error, 1)}

	// A nil error is ignored rather than being delivered as a non-nil-looking
	// value on the channel.
	sub.sendErr(nil)
	select {
	case err := <-sub.errs:
		t.Fatalf("sendErr(nil) delivered %v, want nothing", err)
	default:
	}

	first := errors.New("first")
	sub.sendErr(first)
	select {
	case got := <-sub.errs:
		if !errors.Is(got, first) {
			t.Errorf("delivered %v, want %v", got, first)
		}
	default:
		t.Fatal("sendErr did not deliver into a channel with room")
	}

	// Overflow drops the oldest error.
	sub.sendErr(errors.New("stale"))
	sub.sendErr(errors.New("fresh"))
	select {
	case got := <-sub.errs:
		if got.Error() != "fresh" {
			t.Errorf("after overflow the channel holds %q, want the fresh error", got)
		}
	default:
		t.Error("expected the fresh error to be delivered")
	}
}

// TestSendErrOnClosedSubscriptionIsDropped mirrors the update path.
func TestSendErrOnClosedSubscriptionIsDropped(t *testing.T) {
	sub := &Subscription{updates: make(chan Event, 1), errs: make(chan error, 1), closed: true}
	sub.sendErr(errors.New("late"))
	select {
	case err := <-sub.errs:
		t.Errorf("closed subscription delivered error %v, want it dropped", err)
	default:
	}
}
