// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type Market string

const (
	MarketHK              Market = "HK"
	MarketUS              Market = "US"
	MarketShenzhenConnect Market = "SZ"
	MarketShanghaiConnect Market = "SH"
)

func MarketFromExchange(exchange types.ExchangeType) Market {
	switch exchange {
	case types.ExchangeHK:
		return MarketHK
	case types.ExchangeUS:
		return MarketUS
	case types.ExchangeShenzhenConnect:
		return MarketShenzhenConnect
	case types.ExchangeShanghaiConnect:
		return MarketShanghaiConnect
	default:
		return Market("")
	}
}

type Symbol struct {
	Market      Market
	Code        string
	DataType    types.DataType
	FullCode    string
	DisplayName string
}

func NewSymbol(market Market, code string, dtype types.DataType) Symbol {
	return Symbol{
		Market:   market,
		Code:     code,
		DataType: dtype,
		FullCode: code + "." + string(market),
	}
}

func (s Symbol) IsZero() bool { return s.Code == "" }

type LotSize struct {
	Market   Market
	DataType types.DataType
	Lot      uint64
	TickSize string
}

type MarketSession struct {
	Market   Market
	Open     string
	Close    string
	PreOpen  string
	PreClose string
	Timezone string
}

type TickSchedule struct {
	Market   Market
	DataType types.DataType
	Lot      uint64
	TickSize string
}

func DefaultHKTickSchedule(dtype types.DataType) TickSchedule {
	switch dtype {
	case types.DataTypeHKStock:
		return TickSchedule{Market: MarketHK, DataType: dtype, Lot: 100, TickSize: "0.001"}
	case types.DataTypeHKETF:
		return TickSchedule{Market: MarketHK, DataType: dtype, Lot: 100, TickSize: "0.001"}
	case types.DataTypeHKWarrant, types.DataTypeHKCBBC:
		return TickSchedule{Market: MarketHK, DataType: dtype, Lot: 1, TickSize: "0.001"}
	case types.DataTypeHKBond:
		return TickSchedule{Market: MarketHK, DataType: dtype, Lot: 1, TickSize: "0.0001"}
	default:
		return TickSchedule{}
	}
}
