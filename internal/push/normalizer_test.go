// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func TestNormalizeQuoteEvent(t *testing.T) {
	payload, err := anypb.New(&hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		BasicQot: &dto.BasicQot{
			LastPrice:      388.0,
			OpenPrice:      385.0,
			HighPrice:      390.0,
			LowPrice:       384.0,
			LastClosePrice: 383.0,
			Volume:         1000000,
			Turnover:       388000000.0,
		},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_BasicQotNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	quoteEvent, ok := (*event).(domain.QuoteEvent)
	if !ok {
		t.Fatalf("expected QuoteEvent, got %T", *event)
	}

	if quoteEvent.Symbol.Code != "0700.HK" {
		t.Errorf("Symbol.Code = %q, want %q", quoteEvent.Symbol.Code, "0700.HK")
	}
	if quoteEvent.LastPrice.String() != "388" {
		t.Errorf("LastPrice = %q, want %q", quoteEvent.LastPrice.String(), "388")
	}
}

func TestNormalizeTickerEvent(t *testing.T) {
	payload, err := anypb.New(&hqnotify.TickerNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		Ticker: &dto.Ticker{
			Price:    388.5,
			Volume:   1000,
			Turnover: 385000.0,
			Side:     1,
		},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_TickerNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	tickerEvent, ok := (*event).(domain.TickerEvent)
	if !ok {
		t.Fatalf("expected TickerEvent, got %T", *event)
	}

	if tickerEvent.Symbol.Code != "0700.HK" {
		t.Errorf("Symbol.Code = %q, want %q", tickerEvent.Symbol.Code, "0700.HK")
	}
	if tickerEvent.Price.String() != "388.5" {
		t.Errorf("Price = %q, want %q", tickerEvent.Price.String(), "388.5")
	}
	if tickerEvent.Side != types.EntrustBuy {
		t.Errorf("Side = %q, want %q", tickerEvent.Side, types.EntrustBuy)
	}
}

func TestNormalizeOrderBookEvent(t *testing.T) {
	payload, err := anypb.New(&hqnotify.OrderBookFullNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		Side:     0,
		OrderBookList: []*dto.OrderBook{
			{Level: 1, Price: 388.0, Volume: 1000},
			{Level: 2, Price: 387.5, Volume: 2000},
		},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_OrderBookNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	obEvent, ok := (*event).(domain.OrderBookEvent)
	if !ok {
		t.Fatalf("expected OrderBookEvent, got %T", *event)
	}

	if len(obEvent.Bids) != 2 {
		t.Errorf("len(Bids) = %d, want %d", len(obEvent.Bids), 2)
	}
	if obEvent.Depth != 2 {
		t.Errorf("Depth = %d, want %d", obEvent.Depth, 2)
	}
}

func TestNormalizeBrokerEvent(t *testing.T) {
	payload, err := anypb.New(&hqnotify.BrokerNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		Side:     0,
		BrokerList: []*dto.Broker{
			{Level: 1, Name: "BrokerA"},
			{Level: 2, Name: "BrokerB"},
		},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_BrokerQueueNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	brokerEvent, ok := (*event).(domain.BrokerEvent)
	if !ok {
		t.Fatalf("expected BrokerEvent, got %T", *event)
	}

	if len(brokerEvent.Buyers) != 2 {
		t.Errorf("len(Buyers) = %d, want %d", len(brokerEvent.Buyers), 2)
	}
}

func TestNormalizeTradeEvent(t *testing.T) {
	payload, err := anypb.New(&tradenotify.TradeStockDeliverNotify{
		StockCode:      "0700.HK",
		StockName:      "Tencent",
		EntrustBs:      "1",
		BusinessPrice:  "388.5",
		BusinessAmount: "1000",
		MatchNo:        "M12345",
		EntrustStatus:  "8",
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_TradeStockDeliverMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	tradeEvent, ok := (*event).(domain.TradeEvent)
	if !ok {
		t.Fatalf("expected TradeEvent, got %T", *event)
	}

	if tradeEvent.Price.String() != "388.5" {
		t.Errorf("Price = %q, want %q", tradeEvent.Price.String(), "388.5")
	}
	if tradeEvent.Side != types.EntrustBuy {
		t.Errorf("Side = %q, want %q", tradeEvent.Side, types.EntrustBuy)
	}
}

func TestNormalizeFuturesTradeEvent(t *testing.T) {
	payload, err := anypb.New(&tradenotify.TradeStockDeliverNotify{
		StockCode:      "HSI2501",
		StockName:      "HSI Future",
		EntrustBs:      "2",
		BusinessPrice:  "20000.0",
		BusinessAmount: "1",
		MatchNo:        "M67890",
		EntrustStatus:  "8",
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_FuturesTradeStockDeliverMsgType,
		NotifyId:      "HSI2501",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	tradeEvent, ok := (*event).(domain.TradeEvent)
	if !ok {
		t.Fatalf("expected TradeEvent, got %T", *event)
	}

	if tradeEvent.Side != types.EntrustSell {
		t.Errorf("Side = %q, want %q", tradeEvent.Side, types.EntrustSell)
	}
}

func TestNormalizeUnknownType(t *testing.T) {
	unknownPayload, err := anypb.New(&tradenotify.TradeStockDeliverNotify{})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	unknownPayload.TypeUrl = "type.googleapis.com/does.not.Exist"

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(99999),
		NotifyId:      "TEST",
		NotifyTime:    1700000000000,
		Payload:       unknownPayload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	sysEvent, ok := (*event).(domain.SystemEvent)
	if !ok {
		t.Fatalf("expected SystemEvent for unknown type, got %T", *event)
	}

	if sysEvent.Code != "type.googleapis.com/does.not.Exist" {
		t.Errorf("Code = %q, want %q", sysEvent.Code, "type.googleapis.com/does.not.Exist")
	}
	if sysEvent.Level != "warn" {
		t.Errorf("Level = %q, want %q", sysEvent.Level, "warn")
	}
}

func TestNormalizeUnknownNilPayload(t *testing.T) {
	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(99999),
		NotifyId:      "TEST",
		NotifyTime:    1700000000000,
		Payload:       nil,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	sysEvent, ok := (*event).(domain.SystemEvent)
	if !ok {
		t.Fatalf("expected SystemEvent for nil payload, got %T", *event)
	}

	if sysEvent.Code != "UNKNOWN" {
		t.Errorf("Code = %q, want %q", sysEvent.Code, "UNKNOWN")
	}
}

func TestNormalizeNilPBNotify(t *testing.T) {
	_, err := Normalize(nil)
	if err == nil {
		t.Error("Normalize(nil) should return error")
	}
}

func TestDecodeAny(t *testing.T) {
	inner := &hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		BasicQot: &dto.BasicQot{LastPrice: 388.0},
	}
	payload, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}

	msg, err := DecodeAny(payload)
	if err != nil {
		t.Fatalf("DecodeAny: %v", err)
	}

	result, ok := msg.(*hqnotify.BasicQotNotify)
	if !ok {
		t.Fatalf("expected *hqnotify.BasicQotNotify, got %T", msg)
	}
	if result.GetSecurity().GetCode() != "0700.HK" {
		t.Errorf("Security.Code = %q, want %q", result.GetSecurity().GetCode(), "0700.HK")
	}
}

func TestDecodeAnyNil(t *testing.T) {
	_, err := DecodeAny(nil)
	if err == nil {
		t.Error("DecodeAny(nil) should return error")
	}
}

func TestNormalizeVersionedTypeUrl(t *testing.T) {
	inner := &hqnotify.BasicQotNotify{
		Security: &dto.Security{DataType: 10000, Code: "0700.HK"},
		BasicQot: &dto.BasicQot{LastPrice: 388.0},
	}
	payload, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	payload.TypeUrl = "type.googleapis.com/my.company.com/hq.notify.BasicQotNotify"

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(99998),
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event == nil {
		t.Fatal("Normalize returned nil event")
	}

	sysEvent, ok := (*event).(domain.SystemEvent)
	if !ok {
		t.Fatalf("expected SystemEvent for versioned unknown type, got %T", *event)
	}

	if sysEvent.Code != "type.googleapis.com/my.company.com/hq.notify.BasicQotNotify" {
		t.Errorf("Code = %q, want %q", sysEvent.Code, "type.googleapis.com/my.company.com/hq.notify.BasicQotNotify")
	}
}

func TestNormalizeEmptyPayload(t *testing.T) {
	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_BasicQotNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       &anypb.Any{Value: []byte{}},
	}

	event, err := Normalize(pb)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if event != nil {
		t.Error("Normalize should return nil event for empty payload")
	}
}

func TestNormalizeUnmarshalError(t *testing.T) {
	payload := &anypb.Any{
		TypeUrl: "type.googleapis.com/hq.notify.BasicQotNotify",
		Value:   []byte{0xFF, 0xFF, 0xFF},
	}

	pb := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType_BasicQotNotifyMsgType,
		NotifyId:      "0700.HK",
		NotifyTime:    1700000000000,
		Payload:       payload,
	}

	_, err := Normalize(pb)
	if err == nil {
		t.Error("Normalize should return error for corrupted payload")
	}
}
