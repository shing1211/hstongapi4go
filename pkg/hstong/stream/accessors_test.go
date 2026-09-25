// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestWithPushAddr covers a public option that had no test.
func TestWithPushAddr(t *testing.T) {
	o := options{}
	WithPushAddr("127.0.0.1:19999")(&o)
	if o.pushAddr != "127.0.0.1:19999" {
		t.Errorf("pushAddr = %q, want 127.0.0.1:19999", o.pushAddr)
	}
}

// TestWithDialer covers a public option that had no test, including the nil case
// that must fall back to the default dialer.
func TestWithDialer(t *testing.T) {
	o := options{}
	if o.dial != nil {
		t.Fatal("zero-value options should have a nil dialer")
	}

	var gotNetwork, gotAddress string
	sentinel := errors.New("sentinel")
	dial := push.DialFunc(func(_ context.Context, network, address string) (net.Conn, error) {
		gotNetwork, gotAddress = network, address
		return nil, sentinel
	})
	WithDialer(dial)(&o)
	if o.dial == nil {
		t.Fatal("WithDialer did not install the supplied dialer")
	}

	// The installed dialer must be the one the stream layer will actually call,
	// with its arguments passed through untouched.
	if _, err := o.dial(context.Background(), "tcp", "127.0.0.1:11112"); !errors.Is(err, sentinel) {
		t.Errorf("installed dialer returned %v, want the sentinel error", err)
	}
	if gotNetwork != "tcp" || gotAddress != "127.0.0.1:11112" {
		t.Errorf("dialer got (%q, %q), want (tcp, 127.0.0.1:11112)", gotNetwork, gotAddress)
	}

	WithDialer(nil)(&o)
	if o.dial != nil {
		t.Error("WithDialer(nil) should leave the default dialer in place")
	}
}

// TestWithBuffer covers a public option that had no test, and pins that a
// non-positive size leaves the default rather than producing an unusable
// zero-capacity channel.
func TestWithBuffer(t *testing.T) {
	o := options{buffer: defaultBuffer}
	WithBuffer(8)(&o)
	if o.buffer != 8 {
		t.Errorf("buffer = %d, want 8", o.buffer)
	}

	for _, bad := range []int{0, -1} {
		WithBuffer(bad)(&o)
		if o.buffer != 8 {
			t.Errorf("WithBuffer(%d) = %d, want the previous value 8 to be kept", bad, o.buffer)
		}
	}
}

// TestSubscriptionAccessors covers Subscription.TopicID and Subscription.Securities,
// both public and both previously untested.
func TestSubscriptionAccessors(t *testing.T) {
	securities := []*dto.Security{
		{Code: "00700.HK", DataType: 10000},
		{Code: "AAPL.US", DataType: 20000},
	}
	sub := &Subscription{
		topicID:    types.TopicBasicQot,
		securities: securities,
	}

	if got := sub.TopicID(); got != types.TopicBasicQot {
		t.Errorf("TopicID() = %v, want %v", got, types.TopicBasicQot)
	}

	got := sub.Securities()
	if len(got) != 2 {
		t.Fatalf("Securities() len = %d, want 2", len(got))
	}
	if got[0].GetCode() != "00700.HK" || got[1].GetCode() != "AAPL.US" {
		t.Errorf("Securities() codes = %q, %q", got[0].GetCode(), got[1].GetCode())
	}

	// The accessor must hand back a copy: mutating it cannot corrupt the
	// subscription's own view, which is what the doc comment promises.
	got[0] = &dto.Security{Code: "MUTATED"}
	if sub.Securities()[0].GetCode() != "00700.HK" {
		t.Error("mutating the returned slice changed the subscription's securities")
	}
}

// TestSubscriptionSecuritiesNil covers the case where a subscription carries no
// instruments, which is what a trade subscription looks like. The topic id is
// irrelevant to the assertion, so any registered value will do.
func TestSubscriptionSecuritiesNil(t *testing.T) {
	sub := &Subscription{topicID: types.TopicTicker}
	if got := sub.Securities(); len(got) != 0 {
		t.Errorf("Securities() len = %d, want 0 for a subscription with no instruments", len(got))
	}
}

// TestEventPayloadAccessors covers the typed accessors consumers actually call.
// Each must return the payload when the type matches and report false when it
// does not, rather than panicking or silently returning a nil body.
func TestEventPayloadAccessors(t *testing.T) {
	tickerEv := Event{Type: types.TickerNotifyMsgType, Payload: &hqnotify.TickerNotify{}}
	if m, ok := tickerEv.Ticker(); !ok || m == nil {
		t.Errorf("Ticker() = %v, %v; want the payload and true", m, ok)
	}
	if _, ok := tickerEv.OrderBook(); ok {
		t.Error("OrderBook() on a ticker event = true, want false")
	}
	if _, ok := tickerEv.Broker(); ok {
		t.Error("Broker() on a ticker event = true, want false")
	}

	bookEv := Event{Type: types.OrderBookNotifyMsgType, Payload: &hqnotify.OrderBookFullNotify{}}
	if m, ok := bookEv.OrderBook(); !ok || m == nil {
		t.Errorf("OrderBook() = %v, %v; want the payload and true", m, ok)
	}
	if _, ok := bookEv.Ticker(); ok {
		t.Error("Ticker() on an order-book event = true, want false")
	}

	brokerEv := Event{Type: types.BrokerQueueNotifyMsgType, Payload: &hqnotify.BrokerNotify{}}
	if m, ok := brokerEv.Broker(); !ok || m == nil {
		t.Errorf("Broker() = %v, %v; want the payload and true", m, ok)
	}

	// An event with no decoded payload must report false everywhere rather than
	// panicking on a nil type assertion.
	empty := Event{}
	if _, ok := empty.Ticker(); ok {
		t.Error("Ticker() on an empty event = true, want false")
	}
	if _, ok := empty.OrderBook(); ok {
		t.Error("OrderBook() on an empty event = true, want false")
	}
	if _, ok := empty.Broker(); ok {
		t.Error("Broker() on an empty event = true, want false")
	}
}
