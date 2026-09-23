// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type PushEvent interface {
	isPushEvent()
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

type BrokerLevel struct {
	BrokerID int
	Quantity Quantity
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
