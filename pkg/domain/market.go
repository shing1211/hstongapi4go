// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shopspring/decimal"
)

type Quote struct {
	Symbol        Symbol
	IsSuspended   bool
	OpenPrice     Price
	HighPrice     Price
	LowPrice      Price
	LastPrice     Price
	LastClosePrice Price
	PriceSpread   Price
	Volume        Quantity
	Turnover      Money
	TurnoverRate  decimal.Decimal
	Amplitude     decimal.Decimal
	SecStatus     int32
	ListTime      string
	LotSize       int32
	TradeTime     string
	MarketTime    string
	StockStatus   string
	MktTmType     int32
}

func QuoteFromDTO(b *dto.BasicQot) *Quote {
	if b == nil {
		return nil
	}
	return &Quote{
		Symbol:        SymbolFromSecurity(b.Security),
		IsSuspended:   b.IsSuspended,
		OpenPrice:     floatToPrice(b.OpenPrice),
		HighPrice:     floatToPrice(b.HighPrice),
		LowPrice:      floatToPrice(b.LowPrice),
		LastPrice:     floatToPrice(b.LastPrice),
		LastClosePrice: floatToPrice(b.LastClosePrice),
		PriceSpread:   floatToPrice(b.PriceSpread),
		Volume:        Quantity{dec: decimal.NewFromInt(b.Volume)},
		Turnover:      floatToMoney(b.Turnover),
		TurnoverRate:  decimal.NewFromFloat(b.TurnoverRate),
		Amplitude:     decimal.NewFromFloat(b.Amplitude),
		SecStatus:     b.SecStatus,
		ListTime:      b.ListTime,
		LotSize:       b.LotSize,
		TradeTime:     b.TradeTime,
		MarketTime:    b.MarketTime,
		StockStatus:   b.StockStatus,
		MktTmType:     b.MktTmType,
	}
}

type KLine struct {
	Date           string
	HighPrice      Price
	OpenPrice      Price
	LowPrice       Price
	ClosePrice     Price
	LastClosePrice Price
	Volume         Quantity
	Turnover       Money
	Timestamp      int64
	Time           string
}

func KLineFromDTO(k *dto.KLine) *KLine {
	if k == nil {
		return nil
	}
	return &KLine{
		Date:           k.Date,
		HighPrice:      floatToPrice(k.HighPrice),
		OpenPrice:      floatToPrice(k.OpenPrice),
		LowPrice:       floatToPrice(k.LowPrice),
		ClosePrice:     floatToPrice(k.ClosePrice),
		LastClosePrice: floatToPrice(k.LastClosePrice),
		Volume:         Quantity{dec: decimal.NewFromInt(k.Volume)},
		Turnover:       floatToMoney(k.Turnover),
		Timestamp:      k.Timestamp,
		Time:           k.Time,
	}
}

type TimeSharePoint struct {
	Time           string
	Price          Price
	LastClosePrice Price
	AvgPrice       Price
	Volume         Quantity
	Turnover       Money
}

func TimeSharePointFromDTO(t *dto.TimeShare) *TimeSharePoint {
	if t == nil {
		return nil
	}
	return &TimeSharePoint{
		Time:           t.Time,
		Price:          floatToPrice(t.Price),
		LastClosePrice: floatToPrice(t.LastClosePrice),
		AvgPrice:       floatToPrice(t.AvgPrice),
		Volume:         Quantity{dec: decimal.NewFromInt(t.Volume)},
		Turnover:       floatToMoney(t.Turnover),
	}
}

type TickerTick struct {
	Time      string
	Side      int32
	Price     Price
	Volume    Quantity
	Turnover  Money
	Type      int32
	Timestamp int64
	MktTmType int32
}

func TickerTickFromDTO(t *dto.Ticker) *TickerTick {
	if t == nil {
		return nil
	}
	return &TickerTick{
		Time:      t.Time,
		Side:      t.Side,
		Price:     floatToPrice(t.Price),
		Volume:    Quantity{dec: decimal.NewFromInt(t.Volume)},
		Turnover:  floatToMoney(t.Turnover),
		Type:      t.Type,
		Timestamp: t.Timestamp,
		MktTmType: t.MktTmType,
	}
}

func floatToPrice(v float64) Price {
	return Price{dec: decimal.NewFromFloat(v)}
}

func floatToMoney(v float64) Money {
	return Money{dec: decimal.NewFromFloat(v), currency: "HKD", scale: defaultHKDScale}
}
