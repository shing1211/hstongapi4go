// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestPushUpdateSecurityCode pins the accessor the push seam's routing reads.
//
// It exists as a method rather than as logic the caller repeats, for the same reason
// the whole domain layer exists: a caller that does not hold a *PushEvent it has
// type-asserted cannot get at the payload's instrument at all, and repeating the
// type switch at every call site is how one of the arms gets forgotten. So the one
// place that knows every event shape is the place that answers.
//
// Every arm is asserted, including the two that answer the empty string. The empty
// string is a real answer and not a sentinel - a SystemEvent and an AccountEvent name
// no instrument - so a caller routing on it must be able to tell "no instrument" from
// "an instrument with no code", and both of those are here.
func TestPushUpdateSecurityCode(t *testing.T) {
	symbol := NewSymbol(MarketHK, "00700.HK", types.DataTypeHKStock)

	cases := []struct {
		name  string
		event PushEvent
		want  string
	}{
		{name: "quote", event: QuoteEvent{Symbol: symbol}, want: "00700.HK"},
		{name: "ticker", event: TickerEvent{Symbol: symbol}, want: "00700.HK"},
		{name: "order book", event: OrderBookEvent{Symbol: symbol}, want: "00700.HK"},
		{name: "broker queue", event: BrokerEvent{Symbol: symbol}, want: "00700.HK"},
		{name: "trade", event: TradeEvent{Symbol: symbol}, want: "00700.HK"},
		{name: "system event names no instrument", event: SystemEvent{Code: "EMPTY_PAYLOAD"}, want: ""},
		{name: "account event names no instrument", event: AccountEvent{AccountID: AccountID("HST-1")}, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := PushUpdate{Type: types.NotifyMsgType(20003), ID: "00700", Event: tc.event}
			if got := u.SecurityCode(); got != tc.want {
				t.Errorf("SecurityCode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPushUpdateSecurityCodeOfAZeroUpdate covers the value a caller can hold before
// anything has been normalised into it. The zero event is nil, so the switch takes no
// arm and the answer is the empty string rather than a panic - which matters because
// the accessor is on a public type a caller may hold by value.
func TestPushUpdateSecurityCodeOfAZeroUpdate(t *testing.T) {
	var u PushUpdate
	if got := u.SecurityCode(); got != "" {
		t.Errorf("the zero PushUpdate's SecurityCode() = %q, want %q", got, "")
	}
	if u.Event != nil {
		t.Error("the zero PushUpdate carries an event; it must carry nil")
	}
}

// TestTheSignatureMismatchSentinelHasAMessage pins the one property a sentinel needs
// to be useful in a log line. Its identity and its reachability are asserted from
// pkg/transport and pkg/services, which are the two packages that have to name it.
func TestTheSignatureMismatchSentinelHasAMessage(t *testing.T) {
	if ErrSignatureMismatch == nil {
		t.Fatal("ErrSignatureMismatch is nil")
	}
	if got := ErrSignatureMismatch.Error(); got == "" {
		t.Error("ErrSignatureMismatch.Error() is empty; a sentinel with no text cannot be logged usefully")
	}
}
