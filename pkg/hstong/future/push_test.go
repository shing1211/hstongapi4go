// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package future

import (
	"testing"

	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestNotifyType pins the push discriminator to the documented
// FuturesTradeStockDeliverMsgType value.
func TestNotifyType(t *testing.T) {
	if NotifyType != types.FuturesTradeStockDeliverMsgType {
		t.Fatalf("NotifyType = %v, want %v", NotifyType, types.FuturesTradeStockDeliverMsgType)
	}
	if got := int32(NotifyType); got != 2 {
		t.Fatalf("NotifyType value = %d, want 2", got)
	}
}

// TestFromDeliverNotify asserts the generated payload maps field-for-field into
// the typed view, including the fill-only MatchNo and the RecordNo/EntrustNo
// mirror.
func TestFromDeliverNotify(t *testing.T) {
	msg := &tradenotify.TradeStockDeliverNotify{
		FundAccount:        "FUND1",
		StockCode:          "HSI2603",
		StockName:          "HSI MAR 26",
		EntrustBs:          "1",
		BusinessPrice:      "20000.5",
		BusinessAmount:     "1",
		BusinessDate:       "20260102",
		BusinessTime:       "09:31:00",
		ExchangeType:       "K",
		EntrustStatus:      "7",
		ClientId:           "C1",
		EntrustNo:          "E1",
		SumBusinessAmount:  "1",
		SumBusinessBalance: "20000.5",
		OriginalAmount:     "2",
		OriginalPrice:      "19999",
		LeftAmount:         "1",
		EntrustPrice:       "20000",
		EntrustAmount:      "2",
		RecordNo:           "E1",
		Remark:             "note",
		MatchNo:            "M9",
	}

	got := FromDeliverNotify(msg)

	want := DeliverNotification{
		FundAccount:        "FUND1",
		StockCode:          "HSI2603",
		StockName:          "HSI MAR 26",
		EntrustBS:          "1",
		BusinessPrice:      "20000.5",
		BusinessAmount:     "1",
		BusinessDate:       "20260102",
		BusinessTime:       "09:31:00",
		ExchangeType:       "K",
		EntrustStatus:      "7",
		ClientID:           "C1",
		EntrustNo:          "E1",
		SumBusinessAmount:  "1",
		SumBusinessBalance: "20000.5",
		OriginalAmount:     "2",
		OriginalPrice:      "19999",
		LeftAmount:         "1",
		EntrustPrice:       "20000",
		EntrustAmount:      "2",
		RecordNo:           "E1",
		Remark:             "note",
		MatchNo:            "M9",
	}
	if got != want {
		t.Fatalf("FromDeliverNotify() = %+v, want %+v", got, want)
	}
	if got.RecordNo != got.EntrustNo {
		t.Fatalf("RecordNo = %q, want it to mirror EntrustNo %q", got.RecordNo, got.EntrustNo)
	}
	if !got.HasFill() {
		t.Fatal("HasFill() = false, want true when MatchNo is set")
	}
}

// TestFromDeliverNotifyFillOnly asserts MatchNo is empty, and HasFill false, for
// an order-state notification that reports no fill.
func TestFromDeliverNotifyFillOnly(t *testing.T) {
	msg := &tradenotify.TradeStockDeliverNotify{
		StockCode:     "HSI2603",
		EntrustBs:     "2",
		EntrustStatus: "2",
		EntrustNo:     "E2",
		RecordNo:      "E2",
		// MatchNo intentionally unset: a cancel/placement acknowledgement.
	}

	got := FromDeliverNotify(msg)
	if got.HasFill() {
		t.Fatalf("HasFill() = true, want false when MatchNo is empty (got=%+v)", got)
	}
	if got.MatchNo != "" {
		t.Fatalf("MatchNo = %q, want empty", got.MatchNo)
	}
	if got.EntrustNo != "E2" || got.RecordNo != "E2" {
		t.Fatalf("EntrustNo/RecordNo = %q/%q, want E2/E2", got.EntrustNo, got.RecordNo)
	}
}

// TestFromDeliverNotifyNil asserts a nil message maps to the zero value rather
// than panicking.
func TestFromDeliverNotifyNil(t *testing.T) {
	got := FromDeliverNotify(nil)
	if got != (DeliverNotification{}) {
		t.Fatalf("FromDeliverNotify(nil) = %+v, want zero value", got)
	}
	if got.HasFill() {
		t.Fatal("HasFill() = true for nil message, want false")
	}
}
