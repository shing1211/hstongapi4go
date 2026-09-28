// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"errors"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// ErrSignatureMismatch reports a push frame whose SHA1WithRSA bodySHA1
// signature did not verify against the configured platform public key.
//
// It lives here rather than beside the verifier because it is a failure a caller
// is expected to branch on with errors.Is, and the sentinel that
// internal/push raises for it cannot be named outside this module: `internal/`
// is unimportable from another repository, so an exported API may not put one of
// its types in a signature a third party has to spell. pkg/domain is the one
// package both pkg/services (which declares the push seam) and pkg/transport
// (which implements it) already import, so it is the only home both sides can
// name. pkg/services re-exports it as services.ErrSignatureMismatch.
//
// ADR 0005 keeps verification off by default; this sentinel exists for the
// callers that turn it on.
var ErrSignatureMismatch = errors.New("domain: push frame signature verification failed")

type PushEvent interface {
	isPushEvent()
}

// PushUpdate is one delivered push notification: the PBNotify envelope facts
// plus the normalised event.
//
// The two halves are not derivable from each other, which is why they travel
// together. ID is the notifyId the Gateway put on the frame, which is one of the
// two keys a push subscription is routed by and which no PushEvent carries. Time
// is the wire notifyTime, which is what a monotonic out-of-order check compares;
// the only trace of it inside an event is a formatted string in the payload's own
// Timestamp field, which is a different clock.
//
// Event is never nil for a delivered update. A frame whose payload cannot be
// classified, or carried none, is delivered as a domain.SystemEvent, so a caller
// type-switches with a SystemEvent default arm and never has to nil-check.
//
// The type is declared here, beside the PushEvent interface it wraps, because
// pkg/services and pkg/transport must both be able to name it: the first
// declares the method that delivers it, the second implements that method.
type PushUpdate struct {
	// Type is the PBNotify notifyMsgType discriminator, and the key the push
	// transport registers a handler under.
	Type types.NotifyMsgType
	// ID is the PBNotify notifyId. For market pushes the Gateway sends the
	// security code here; the format is not documented and is recorded as an open
	// question, which is why routing matches on this and on SecurityCode rather
	// than on this alone.
	ID string
	// Time is the PBNotify notifyTime in UTC. It is the zero time when the
	// Gateway sent none, rather than 1970-01-01.
	Time time.Time
	// Event is the normalised payload. It is never nil.
	Event PushEvent
}

// SecurityCode returns the instrument code carried in the update's payload, or
// the empty string when the event names no instrument.
//
// A SystemEvent has none, so the empty string is a real answer here and not a
// sentinel: a caller routing on it must not treat "" as an instrument. It is
// spelled out because the alternative - reading a Symbol out of a payload the
// caller has not type-asserted yet - is the released layer's Payload any defect
// one layer up.
func (u PushUpdate) SecurityCode() string {
	switch e := u.Event.(type) {
	case QuoteEvent:
		return e.Symbol.Code
	case TickerEvent:
		return e.Symbol.Code
	case OrderBookEvent:
		return e.Symbol.Code
	case BrokerEvent:
		return e.Symbol.Code
	case TradeEvent:
		return e.Symbol.Code
	default:
		return ""
	}
}

type QuoteEvent struct {
	Symbol     Symbol
	LastPrice  Price
	OpenPrice  Price
	HighPrice  Price
	LowPrice   Price
	ClosePrice Price
	Volume     Quantity
	Turnover   Money
	BidPrice   Price
	AskPrice   Price
	BidQty     Quantity
	AskQty     Quantity
	Timestamp  string
}

func (QuoteEvent) isPushEvent() {}

type TickerEvent struct {
	Symbol    Symbol
	Price     Price
	Volume    Quantity
	Turnover  Money
	Timestamp string
	Side      types.EntrustBS
}

func (TickerEvent) isPushEvent() {}

type OrderBookEvent struct {
	Symbol    Symbol
	Bids      []OrderBookLevel
	Asks      []OrderBookLevel
	Depth     int
	Timestamp string
}

type OrderBookLevel struct {
	Level    int32
	Price    Price
	Quantity Quantity
}

func (OrderBookEvent) isPushEvent() {}

type BrokerEvent struct {
	Symbol    Symbol
	Buyers    []BrokerLevel
	Sellers   []BrokerLevel
	Timestamp string
}

// BrokerLevel is one broker-queue entry as the Gateway pushes it.
//
// It carries the four fields the wire carries (gen/hq/dto/Broker: level, item,
// type, name) and nothing else. There is deliberately no Quantity and no Price
// field: the push broker frame carries neither, and a fabricated zero in a field
// named like the sibling order-book decoder's field is indistinguishable from an
// observation to a reader and to a caller. The shape is the pull path's
// BrokerQueueEntry, which is what a caller already handles for GET /hq/Broker,
// so "no quantity on the wire" now reads as the absence of a field rather than as
// a number.
//
// This is the type-level half of that fix: a push broker level that carries no
// quantity now has nothing a caller could mistake for one.
type BrokerLevel struct {
	// Level is the broker's queue position, 1-based on the wire.
	Level int32
	// Item is the broker's seat identifier.
	Item string
	// Type is the broker's seat kind as the Gateway numbers it; the data
	// dictionary does not enumerate the codes.
	Type int32
	// Name is the broker's display name.
	Name string
}

func (BrokerEvent) isPushEvent() {}

type TradeEvent struct {
	Symbol      Symbol
	OrderID     OrderID
	EntrustID   EntrustID
	Price       Price
	Quantity    Quantity
	Turnover    Money
	Side        types.EntrustBS
	Timestamp   string
	CounterID   string
	OrderStatus types.EntrustStatus
}

func (TradeEvent) isPushEvent() {}

type AccountEvent struct {
	AccountID    AccountID
	TotalAssets  Money
	Cash         Money
	MarketValues map[Market]Money
	Frozen       Money
	Timestamp    string
}

func (AccountEvent) isPushEvent() {}

type SystemEvent struct {
	Code    string
	Message string
	Level   string
}

func (SystemEvent) isPushEvent() {}
