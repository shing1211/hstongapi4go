// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/types"
	"github.com/shopspring/decimal"
)

func TestIDTypes(t *testing.T) {
	if got := AccountID("A1").String(); got != "A1" {
		t.Errorf("AccountID.String() = %q, want %q", got, "A1")
	}
	if AccountID("").IsZero() != true || AccountID("A1").IsZero() != false {
		t.Error("AccountID.IsZero failed")
	}
	if got := OrderID("O1").String(); got != "O1" {
		t.Errorf("OrderID.String() = %q", got)
	}
	if OrderID("").IsZero() != true || OrderID("O1").IsZero() != false {
		t.Error("OrderID.IsZero failed")
	}
	if got := EntrustID("E1").String(); got != "E1" {
		t.Errorf("EntrustID.String() = %q", got)
	}
	if EntrustID("").IsZero() != true || EntrustID("E1").IsZero() != false {
		t.Error("EntrustID.IsZero failed")
	}
	if got := ContractID("C1").String(); got != "C1" {
		t.Errorf("ContractID.String() = %q", got)
	}
	if ContractID("").IsZero() != true || ContractID("C1").IsZero() != false {
		t.Error("ContractID.IsZero failed")
	}
	if got := SessionToken("T1").String(); got != "T1" {
		t.Errorf("SessionToken.String() = %q", got)
	}
	if SessionToken("").IsZero() != true || SessionToken("T1").IsZero() != false {
		t.Error("SessionToken.IsZero failed")
	}
}

func TestParseAccountID(t *testing.T) {
	id, err := ParseAccountID("ACC1")
	if err != nil {
		t.Fatalf("ParseAccountID error = %v", err)
	}
	if id != AccountID("ACC1") {
		t.Errorf("ParseAccountID = %q, want %q", id, "ACC1")
	}
	if _, err := ParseAccountID(""); err == nil {
		t.Error("ParseAccountID(empty) expected error")
	}
}

func TestMoneyOperations(t *testing.T) {
	m := MustNewMoneyWithScale("10.5555", "HKD", 2)
	if got := m.String(); got != "10.56" {
		t.Errorf("MustNewMoneyWithScale round = %q, want %q", got, "10.56")
	}
	if got := m.Currency(); got != "HKD" {
		t.Errorf("Currency() = %q, want HKD", got)
	}
	if got := m.Scale(); got != 2 {
		t.Errorf("Scale() = %d, want 2", got)
	}
	if got := m.Decimal(); !got.Equal(decimal.RequireFromString("10.56")) {
		t.Errorf("Decimal() = %v", got)
	}

	a := MustNewMoney("10", "HKD", 3)
	b := MustNewMoney("3", "HKD", 3)
	if got := a.Sub(b).String(); got != "7" {
		t.Errorf("Sub = %q, want 7", got)
	}
	if got := a.Mul(decimal.NewFromInt(2)).String(); got != "20" {
		t.Errorf("Mul = %q, want 20", got)
	}
}

func TestMoneyUnmarshalJSON(t *testing.T) {
	var m Money
	if err := json.Unmarshal([]byte(`"123.456"`), &m); err != nil {
		t.Fatalf("UnmarshalJSON error = %v", err)
	}
	if got := m.String(); got != "123.456" {
		t.Errorf("UnmarshalJSON = %q, want 123.456", got)
	}
	if err := json.Unmarshal([]byte(`"not-a-number"`), &m); err == nil {
		t.Error("UnmarshalJSON invalid expected error")
	}
}

func TestPriceAccessors(t *testing.T) {
	p := MustNewPrice("100.5", "0.001")
	if !p.Decimal().Equal(decimal.RequireFromString("100.5")) {
		t.Errorf("Decimal() = %v", p.Decimal())
	}
	if !p.Tick().Equal(decimal.RequireFromString("0.001")) {
		t.Errorf("Tick() = %v", p.Tick())
	}
	if p.IsZero() {
		t.Error("IsZero() = true, want false")
	}
	if p.IsNegative() {
		t.Error("IsNegative() = true, want false")
	}
	if !MustNewPrice("0", "0.001").IsZero() {
		t.Error("zero price IsZero = false")
	}
	if !MustNewPrice("-1", "0.001").IsNegative() {
		t.Error("negative price IsNegative = false")
	}
}

func TestPriceRound(t *testing.T) {
	p := MustNewPrice("100.567", "0.01")
	if got := p.Round().String(); got != "100.56" {
		t.Errorf("Round() = %q, want 100.56", got)
	}
	noTick := MustNewPrice("100.567", "0")
	if got := noTick.Round().String(); got != "100.567" {
		t.Errorf("Round() with zero tick = %q, want unchanged", got)
	}
}

func TestPriceJSON(t *testing.T) {
	p := MustNewPrice("100.5", "0.001")
	b, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error = %v", err)
	}
	if string(b) != `"100.5"` {
		t.Errorf("MarshalJSON = %s, want \"100.5\"", b)
	}
	var q Price
	if err := json.Unmarshal([]byte(`"99.9"`), &q); err != nil {
		t.Fatalf("UnmarshalJSON error = %v", err)
	}
	if got := q.String(); got != "99.9" {
		t.Errorf("UnmarshalJSON = %q, want 99.9", got)
	}
	if err := json.Unmarshal([]byte(`"bad"`), &q); err == nil {
		t.Error("UnmarshalJSON invalid expected error")
	}
}

func TestQuantityAccessors(t *testing.T) {
	q := MustNewQuantity("100")
	if !q.Decimal().Equal(decimal.NewFromInt(100)) {
		t.Errorf("Decimal() = %v", q.Decimal())
	}
	if q.IsZero() {
		t.Error("IsZero() = true, want false")
	}
	if !q.IsPositive() {
		t.Error("IsPositive() = false, want true")
	}
	if q.IsNegative() {
		t.Error("IsNegative() = true, want false")
	}
	if !MustNewQuantity("0").IsZero() {
		t.Error("zero quantity IsZero = false")
	}
	if !MustNewQuantity("-5").IsNegative() {
		t.Error("negative quantity IsNegative = false")
	}
}

func TestQuantityJSON(t *testing.T) {
	q := MustNewQuantity("100")
	b, err := q.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error = %v", err)
	}
	if string(b) != `"100"` {
		t.Errorf("MarshalJSON = %s, want \"100\"", b)
	}
	var out Quantity
	if err := json.Unmarshal([]byte(`"250"`), &out); err != nil {
		t.Fatalf("UnmarshalJSON error = %v", err)
	}
	if got := out.String(); got != "250" {
		t.Errorf("UnmarshalJSON = %q, want 250", got)
	}
	if err := json.Unmarshal([]byte(`"z"`), &out); err == nil {
		t.Error("UnmarshalJSON invalid expected error")
	}
}

func TestRate(t *testing.T) {
	r := MustNewRate("1.25")
	if !r.Decimal().Equal(decimal.RequireFromString("1.25")) {
		t.Errorf("Decimal() = %v", r.Decimal())
	}
	if r.IsZero() {
		t.Error("IsZero() = true, want false")
	}
	if !r.IsPositive() {
		t.Error("IsPositive() = false, want true")
	}
	if r.IsNegative() {
		t.Error("IsNegative() = true, want false")
	}
	if !MustNewRate("0").IsZero() {
		t.Error("zero rate IsZero = false")
	}
	if !MustNewRate("-1").IsNegative() {
		t.Error("negative rate IsNegative = false")
	}

	b, err := r.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error = %v", err)
	}
	if string(b) != `"1.25"` {
		t.Errorf("MarshalJSON = %s", b)
	}
	var out Rate
	if err := json.Unmarshal([]byte(`"2.5"`), &out); err != nil {
		t.Fatalf("UnmarshalJSON error = %v", err)
	}
	if got := out.String(); got != "2.5" {
		t.Errorf("UnmarshalJSON = %q, want 2.5", got)
	}
	if err := json.Unmarshal([]byte(`"x"`), &out); err == nil {
		t.Error("UnmarshalJSON invalid expected error")
	}
}

func TestQuoteFromDTO(t *testing.T) {
	if got := QuoteFromDTO(nil); got != nil {
		t.Errorf("QuoteFromDTO(nil) = %v, want nil", got)
	}
	q := QuoteFromDTO(&dto.BasicQot{
		Security:       &dto.Security{DataType: 10000, Code: "00700.HK"},
		IsSuspended:    true,
		OpenPrice:      100,
		HighPrice:      105,
		LowPrice:       99,
		LastPrice:      104,
		LastClosePrice: 100,
		PriceSpread:    0.1,
		Volume:         1000,
		VolumeStr:      "1000",
		Turnover:       104000,
		TurnoverRate:   0.0123,
		Amplitude:      0.0567,
		SecStatus:      1,
		ListTime:       "20040101",
		LotSize:        100,
		TradeTime:      "12:00",
		MarketTime:     "12:00",
		StockStatus:    "N",
		MktTmType:      1,
	})
	if q == nil {
		t.Fatal("QuoteFromDTO returned nil")
	}
	if q.Symbol.Code != "00700.HK" || q.Symbol.Market != MarketHK {
		t.Errorf("Symbol = %+v", q.Symbol)
	}
	if !q.IsSuspended {
		t.Error("IsSuspended = false, want true")
	}
	if q.OpenPrice.String() != "100" {
		t.Errorf("OpenPrice = %q", q.OpenPrice)
	}
	if q.Volume.String() != "1000" {
		t.Errorf("Volume = %q", q.Volume)
	}
	if q.Turnover.Currency() != "HKD" {
		t.Errorf("Turnover currency = %q", q.Turnover.Currency())
	}
	if q.VolumeStr != "1000" || q.SecStatus != 1 || q.LotSize != 100 {
		t.Errorf("scalar fields not mapped: %+v", q)
	}
}

func TestKLineFromDTO(t *testing.T) {
	if got := KLineFromDTO(nil); got != nil {
		t.Errorf("KLineFromDTO(nil) = %v, want nil", got)
	}
	k := KLineFromDTO(&dto.KLine{
		Date:           "2026-01-02",
		HighPrice:      10,
		OpenPrice:      9,
		LowPrice:       8,
		ClosePrice:     9.5,
		LastClosePrice: 8.5,
		Volume:         500,
		Turnover:       4750,
		Timestamp:      1767225600,
		Time:           "09:30",
	})
	if k == nil {
		t.Fatal("KLineFromDTO returned nil")
	}
	if k.Date != "2026-01-02" || k.HighPrice.String() != "10" {
		t.Errorf("KLine = %+v", k)
	}
	if k.Volume.String() != "500" || k.Timestamp != 1767225600 {
		t.Errorf("KLine volume/timestamp = %+v", k)
	}
}

func TestTimeSharePointFromDTO(t *testing.T) {
	if got := TimeSharePointFromDTO(nil); got != nil {
		t.Errorf("TimeSharePointFromDTO(nil) = %v, want nil", got)
	}
	p := TimeSharePointFromDTO(&dto.TimeShare{
		Time:           "09:31",
		Price:          9.5,
		LastClosePrice: 9,
		AvgPrice:       9.4,
		Volume:         100,
		Turnover:       950,
	})
	if p == nil {
		t.Fatal("TimeSharePointFromDTO returned nil")
	}
	if p.Time != "09:31" || p.Price.String() != "9.5" || p.Volume.String() != "100" {
		t.Errorf("TimeSharePoint = %+v", p)
	}
}

func TestTickerTickFromDTO(t *testing.T) {
	if got := TickerTickFromDTO(nil); got != nil {
		t.Errorf("TickerTickFromDTO(nil) = %v, want nil", got)
	}
	tk := TickerTickFromDTO(&dto.Ticker{
		Time:      "09:32",
		Side:      1,
		Price:     9.6,
		Volume:    200,
		Turnover:  1920,
		Type:      2,
		Timestamp: 100,
		MktTmType: 1,
	})
	if tk == nil {
		t.Fatal("TickerTickFromDTO returned nil")
	}
	if tk.Side != 1 || tk.Price.String() != "9.6" || tk.Volume.String() != "200" {
		t.Errorf("TickerTick = %+v", tk)
	}
}

func TestBrokerQueueEntryFromDTO(t *testing.T) {
	if got := BrokerQueueEntryFromDTO(nil); got != nil {
		t.Errorf("BrokerQueueEntryFromDTO(nil) = %v, want nil", got)
	}
	b := BrokerQueueEntryFromDTO(&dto.Broker{Level: 1, Item: "B", Type: 0, Name: "Broker"})
	if b == nil {
		t.Fatal("BrokerQueueEntryFromDTO returned nil")
	}
	if b.Level != 1 || b.Item != "B" || b.Type != 0 || b.Name != "Broker" {
		t.Errorf("BrokerQueueEntry = %+v", b)
	}
}

func TestFloatHelpers(t *testing.T) {
	if got := floatToPrice(1.5).String(); got != "1.5" {
		t.Errorf("floatToPrice = %q", got)
	}
	m := floatToMoney(2.5)
	if m.String() != "2.5" || m.Currency() != "HKD" || m.Scale() != defaultHKDScale {
		t.Errorf("floatToMoney = %+v", m)
	}
}

func TestAccountBalanceFromDTO(t *testing.T) {
	if got := AccountBalanceFromDTO(nil); got != nil {
		t.Errorf("AccountBalanceFromDTO(nil) = %v, want nil", got)
	}
	m := &MarginFundInfoWire{
		HoldsBalance:        "1",
		AssetBalance:        "2",
		EnableBalance:       "3",
		MarketValue:         "4",
		CashOnHold:          "5",
		CreditValue:         "6",
		CreditLine:          "7",
		FetchBalance:        "8",
		FrozenBalance:       "9",
		AccountStatus:       "ok",
		SpentRatio:          "0.5",
		CurrentCreditLimit:  "10",
		MaxCreditLimit:      "11",
		UnitCreditLimit:     "12",
		UnitMaxCreditLimit:  "13",
		BuyPower:            "14",
		BuyPowerCredit:      "15",
		BuyPowerHk:          "16",
		BuyPowerUs:          "17",
		BuyPowerCn:          "18",
		UnitedBuyPowerHk:    "19",
		UnitedBuyPowerUs:    "20",
		UnitedBuyPowerCn:    "21",
		ThirdBuyPowerHk:     "22",
		ThirdBuyPowerUs:     "23",
		ThirdBuyPowerCn:     "24",
		BuyPowerShortMarket: "25",
		BuyPowerMoney:       "26",
	}
	b := AccountBalanceFromDTO(m)
	if b == nil {
		t.Fatal("AccountBalanceFromDTO returned nil")
	}
	if b.HoldsBalance.String() != "1" || b.AccountStatus != "ok" {
		t.Errorf("AccountBalance = %+v", b)
	}
	if b.BuyPowerUs.Currency() != "USD" || b.BuyPowerCn.Currency() != "CNY" {
		t.Errorf("currency mapping wrong: %+v", b)
	}
}

func TestPositionFromDTO(t *testing.T) {
	if got := PositionFromDTO(nil); got != nil {
		t.Errorf("PositionFromDTO(nil) = %v, want nil", got)
	}
	p := PositionFromDTO(&HoldsVoWire{
		StockName:        "Tencent",
		EnableAmount:     "100",
		CurrentAmount:    "200",
		StockCode:        "00700.HK",
		CostPrice:        "10",
		LastPrice:        "11",
		IncomeBalance:    "100",
		MarketValue:      "2200",
		MarketValueRate:  "0.1",
		IncomeRatio:      "0.2",
		DayCostPrice:     "10.5",
		DayInComeAmount:  "50",
		DayOutComeAmount: "25",
		KeepCostPrice:    "9",
		ExchangeType:     "K",
	})
	if p == nil {
		t.Fatal("PositionFromDTO returned nil")
	}
	if p.StockName != "Tencent" || p.CurrentAmount.String() != "200" {
		t.Errorf("Position = %+v", p)
	}
	if p.CostPrice.String() != "10" || p.ExchangeType != "K" {
		t.Errorf("Position prices = %+v", p)
	}
}

func TestFundJournalEntryFromDTO(t *testing.T) {
	if got := FundJournalEntryFromDTO(nil); got != nil {
		t.Errorf("FundJournalEntryFromDTO(nil) = %v, want nil", got)
	}
	e := FundJournalEntryFromDTO(&FundJourVoWire{
		BusinessBalance:    "100",
		Type:               "1",
		TypeDesc:           "deposit",
		FundJourParentType: "1",
		Time:               "2026-01-02",
		QueryParamStr:      "q",
	})
	if e == nil {
		t.Fatal("FundJournalEntryFromDTO returned nil")
	}
	if e.BusinessBalance.String() != "100" || e.TypeDesc != "deposit" {
		t.Errorf("FundJournalEntry = %+v", e)
	}
}

func TestInterestRateFromDTO(t *testing.T) {
	r := InterestRateFromDTO("HKD", "USD", "0.01")
	if r.SourceCurrency != "HKD" || r.TargetCurrency != "USD" || r.Rate.String() != "0.01" {
		t.Errorf("InterestRate = %+v", r)
	}
}

func TestEntrustFromWire(t *testing.T) {
	if got := EntrustFromWire(nil, types.ExchangeHK); got != nil {
		t.Errorf("EntrustFromWire(nil) = %v, want nil", got)
	}
	e := EntrustFromWire(&EntrustWire{
		StockCode:       "00700.HK",
		BusinessPrice:   "10.5",
		EntrustBS:       types.EntrustBuy,
		EntrustPrice:    "10",
		BusinessBalance: "1050",
		EntrustAmount:   "100",
		BusinessAmount:  "50",
		Date:            "2026-01-02",
		BusinessTime:    "10:00",
		EntrustTime:     "09:59",
		StatusDesc:      "accepted",
		Status:          types.EntrustStatusRegistered,
		EntrustID:       "E1",
		CanBeCanceled:   1,
		CanBeUpdated:    1,
		RemarkType:      "r",
		Remark:          "note",
		OpponentSeat:    "S1",
	}, types.ExchangeHK)
	if e == nil {
		t.Fatal("EntrustFromWire returned nil")
	}
	if e.Symbol.Market != MarketHK || e.EntrustID != EntrustID("E1") {
		t.Errorf("Entrust = %+v", e)
	}
	if !e.CanCancel || !e.CanModify {
		t.Errorf("CanCancel/CanModify = %v/%v", e.CanCancel, e.CanModify)
	}
	if e.Quantity.String() != "100" || e.FilledQty.String() != "50" {
		t.Errorf("quantities = %+v", e)
	}
}

func TestFillFromWire(t *testing.T) {
	if got := FillFromWire(nil, types.ExchangeUS); got != nil {
		t.Errorf("FillFromWire(nil) = %v, want nil", got)
	}
	f := FillFromWire(&EntrustWire{
		StockCode:       "AAPL.US",
		BusinessPrice:   "20",
		EntrustBS:       types.EntrustSell,
		EntrustPrice:    "19",
		BusinessBalance: "2000",
		EntrustAmount:   "100",
		BusinessAmount:  "100",
		Date:            "2026-01-02",
		BusinessTime:    "11:00",
		Status:          types.EntrustStatusFilled,
		EntrustID:       "E2",
		OpponentSeat:    "S2",
	}, types.ExchangeUS)
	if f == nil {
		t.Fatal("FillFromWire returned nil")
	}
	if f.Symbol.Market != MarketUS || f.CounterID != "S2" {
		t.Errorf("Fill = %+v", f)
	}
	if f.Turnover.Currency() != "HKD" || f.Quantity.String() != "100" {
		t.Errorf("Fill money/qty = %+v", f)
	}
}

func TestFareFromWire(t *testing.T) {
	if got := FareFromWire(nil); got != nil {
		t.Errorf("FareFromWire(nil) = %v, want nil", got)
	}
	f := FareFromWire(&FareWire{
		Fare0: "1",
		Fare1: "2",
		Fare2: "3",
		Fare3: "4",
		Fare4: "5",
		Fare5: "6",
		Fare6: "7",
		Fare9: "8",
		FareX: "9",
		FareT: "45",
	})
	if f == nil {
		t.Fatal("FareFromWire returned nil")
	}
	if f.Commission.String() != "1" || f.StampDuty.String() != "2" || f.Total.String() != "45" {
		t.Errorf("Fare = %+v", f)
	}
	if f.AFRCLevy.String() != "8" || f.SettlementFee.String() != "9" {
		t.Errorf("Fare extra = %+v", f)
	}
}

func TestCondOrderFromWire(t *testing.T) {
	if got := CondOrderFromWire(nil); got != nil {
		t.Errorf("CondOrderFromWire(nil) = %v, want nil", got)
	}
	c := CondOrderFromWire(&CondOrderWire{
		CondOrderID:   "C1",
		StockCode:     "00700.HK",
		EntrustBS:     types.EntrustBuy,
		EntrustType:   types.EntrustTypeStopLossLimit,
		EntrustAmount: "100",
		Status:        "1",
		CanBeCancel:   "1",
		CanBeModify:   "1",
		CreateTime:    "2026-01-02",
		StartTime:     "2026-01-02",
		EndTime:       "2026-01-03",
		CondValue:     "9",
		CondPrice:     "9.5",
		CondTrackType: "0",
		ErrorCode:     "0",
		ErrorMsg:      "",
		SessionType:   "0",
		ExchangeType:  types.ExchangeHK,
	})
	if c == nil {
		t.Fatal("CondOrderFromWire returned nil")
	}
	if c.CondOrderID != "C1" || !c.CanCancel || !c.CanModify {
		t.Errorf("CondOrder = %+v", c)
	}
	if c.StatusDesc != "pending trigger" {
		t.Errorf("StatusDesc = %q, want pending trigger", c.StatusDesc)
	}
}

func TestCondOrderStatusDesc(t *testing.T) {
	tests := map[string]string{
		"1": "pending trigger",
		"2": "triggered",
		"3": "paused",
		"4": "expired",
		"5": "deleted",
		"6": "error",
		"8": "stop invalidated",
		"9": "ex-rights invalidated",
		"7": "7",
	}
	for status, want := range tests {
		if got := condOrderStatusDesc(status); got != want {
			t.Errorf("condOrderStatusDesc(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestMaxAvailableFromWire(t *testing.T) {
	if got := MaxAvailableFromWire(nil); got != nil {
		t.Errorf("MaxAvailableFromWire(nil) = %v, want nil", got)
	}
	m := MaxAvailableFromWire(&MaxAvailableWire{
		PositionStatus:               "normal",
		Position:                     "100",
		LongOpenAvailable:            "100",
		LongCloseAvailable:           "50",
		CashAvailableToOpen:          "10",
		MarginAvailableToOpen:        "10",
		CashAndMarginAvailableToOpen: "20",
		CashAvailableAmount:          "1000",
		MarginAvailableAmount:        "1000",
		CashAndMarginAvailableAmount: "2000",
		ShortOpenAvailable:           "0",
		ShortCloseAvailable:          "0",
		ShortCloseCashAvailable:      "0",
		ShortOpenPool:                "0",
		ShortBuyPower:                "0",
		ContractSize:                 "1",
		UnitedBuyPowerStatus:         "ok",
		CreditLimit:                  "0",
		OptionLongMarginAmount:       "0",
		OptionShortMarginAmount:      "0",
	})
	if m == nil {
		t.Fatal("MaxAvailableFromWire returned nil")
	}
	if m.PositionStatus != "normal" || m.Position.String() != "100" {
		t.Errorf("MaxAvailable = %+v", m)
	}
	if m.CashAndMarginAvailableAmount.Currency() != "HKD" {
		t.Errorf("MaxAvailable money currency = %+v", m.CashAndMarginAvailableAmount)
	}
}

func TestMarginInfoFromWire(t *testing.T) {
	if got := MarginInfoFromWire(nil); got != nil {
		t.Errorf("MarginInfoFromWire(nil) = %v, want nil", got)
	}
	mi := MarginInfoFromWire(&MarginFullInfoWire{
		MarginAllow:           "1",
		MarginInitRatio:       "0.5",
		MarginKeepRatio:       "0.3",
		ShortAllow:            "1",
		ShortInitMarginRatio:  "0.6",
		ShortInterestRate:     "0.1",
		ShortKeepMarginRatio:  "0.4",
		ShortLastAvailableQty: "100",
		Rate: []RateWire{
			{CurrencyCode: "HKD", CurrencyDesc: "HKD", InterestRateWithinMortgage: "0.02"},
		},
	})
	if mi == nil {
		t.Fatal("MarginInfoFromWire returned nil")
	}
	if !mi.MarginAllow || !mi.ShortAllow {
		t.Errorf("flags = %+v", mi)
	}
	if len(mi.Rates) != 1 || mi.Rates[0].CurrencyCode != "HKD" {
		t.Errorf("Rates = %+v", mi.Rates)
	}
}

func TestSessionSupportFromWire(t *testing.T) {
	sym := NewSymbol(MarketHK, "00700.HK", types.DataTypeHKStock)
	s := SessionSupportFromWire(sym, "1")
	if !s.PreMarket || !s.AfterHours {
		t.Errorf("support flags = %+v", s)
	}
	if s.Symbol.Code != "00700.HK" {
		t.Errorf("Symbol = %+v", s.Symbol)
	}
}

func TestSymbolIsZeroAndFromSecurity(t *testing.T) {
	if !NewSymbol("", "", 0).IsZero() {
		t.Error("empty symbol IsZero = false")
	}
	if NewSymbol(MarketHK, "00700.HK", types.DataTypeHKStock).IsZero() {
		t.Error("non-empty symbol IsZero = true")
	}
	if got := SymbolFromSecurity(nil); !got.IsZero() {
		t.Errorf("SymbolFromSecurity(nil) = %+v, want zero", got)
	}
	s := SymbolFromSecurity(&dto.Security{DataType: 10000, Code: "00700.HK"})
	if s.Market != MarketHK || s.DataType != types.DataTypeHKStock {
		t.Errorf("SymbolFromSecurity = %+v", s)
	}
}

func TestMarketFromCode(t *testing.T) {
	tests := map[string]Market{
		"00700.HK":  MarketHK,
		"AAPL.US":   MarketUS,
		"000001.SZ": MarketShenzhenConnect,
		"600000.SH": MarketShanghaiConnect,
		"XXX":       "",
		"":          "",
	}
	for code, want := range tests {
		if got := MarketFromCode(code); got != want {
			t.Errorf("MarketFromCode(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestPushEventImplementations(t *testing.T) {
	events := []PushEvent{
		QuoteEvent{},
		TickerEvent{},
		OrderBookEvent{},
		BrokerEvent{},
		TradeEvent{},
		AccountEvent{},
		SystemEvent{},
	}
	if len(events) != 7 {
		t.Errorf("expected 7 push event implementations, got %d", len(events))
	}
}
