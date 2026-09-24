// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type Order struct {
	Symbol        Symbol
	Side          types.EntrustBS
	OrderType     types.EntrustType
	Quantity      Quantity
	Price         Price
	TimeInForce   string
	SessionType   string
	IcebergQty    Quantity
	ValidDays     int
	CondValue     string
	CondTrackType string
	ClientType    int
	Exchange      string
}

type OrderResult struct {
	EntrustID EntrustID
	OrderID   OrderID
	Status    types.EntrustStatus
}

type Entrust struct {
	Symbol       Symbol
	EntrustID    EntrustID
	OrderID      OrderID
	Side         types.EntrustBS
	OrderType    types.EntrustType
	Price        Price
	Quantity     Quantity
	FilledQty    Quantity
	AvgPrice     Price
	Status       types.EntrustStatus
	StatusDesc   string
	CanCancel    bool
	CanModify    bool
	Date         string
	Time         string
	ExchangeType types.ExchangeType
	RemarkType   string
	Remark       string
	OpponentSeat string
	Fare         *Fare
}

type Fill struct {
	Symbol       Symbol
	EntrustID    EntrustID
	OrderID      OrderID
	Side         types.EntrustBS
	Price        Price
	Quantity     Quantity
	FilledQty    Quantity
	Turnover     Money
	Status       types.EntrustStatus
	StatusDesc   string
	Date         string
	Time         string
	ExchangeType types.ExchangeType
	CounterID    string
	Fare         *Fare
}

type Fare struct {
	Commission        Money
	StampDuty         Money
	TradingFee        Money
	TransactionLevy   Money
	OptionRegFee      Money
	OptionClearingFee Money
	PlatformFee       Money
	AFRCLevy          Money
	SettlementFee     Money
	Total             Money
}

type CondOrder struct {
	CondOrderID   string
	Symbol        Symbol
	Side          types.EntrustBS
	OrderType     types.EntrustType
	Quantity      Quantity
	Status        string
	StatusDesc    string
	CanCancel     bool
	CanModify     bool
	CreateTime    string
	StartTime     string
	EndTime       string
	CondValue     string
	CondPrice     string
	CondTrackType string
	ErrorCode     string
	ErrorMsg      string
	SessionType   string
}

type MaxAvailable struct {
	PositionStatus               string
	Position                     Quantity
	LongOpenAvailable            Quantity
	LongCloseAvailable           Quantity
	CashAvailableToOpen          Quantity
	MarginAvailableToOpen        Quantity
	CashAndMarginAvailableToOpen Quantity
	CashAvailableAmount          Money
	MarginAvailableAmount        Money
	CashAndMarginAvailableAmount Money
	ShortOpenAvailable           Quantity
	ShortCloseAvailable          Quantity
	ShortCloseCashAvailable      Quantity
	ShortOpenPool                Quantity
	ShortBuyPower                Money
	ContractSize                 Quantity
	UnitedBuyPowerStatus         string
	CreditLimit                  Money
	OptionLongMarginAmount       Money
	OptionShortMarginAmount      Money
}

type MarginInfo struct {
	MarginAllow           bool
	MarginInitRatio       Rate
	MarginKeepRatio       Rate
	ShortAllow            bool
	ShortInitMarginRatio  Rate
	ShortInterestRate     Rate
	ShortKeepMarginRatio  Rate
	ShortLastAvailableQty Quantity
	Rates                 []*MarginInterestRate
}

type MarginInterestRate struct {
	CurrencyCode               string
	CurrencyDesc               string
	InterestRateWithinMortgage Rate
}

type SessionSupport struct {
	Symbol     Symbol
	PreMarket  bool
	AfterHours bool
}

func EntrustFromWire(v *EntrustWire, exchangeType types.ExchangeType) *Entrust {
	if v == nil {
		return nil
	}
	symbol := NewSymbol(MarketFromExchange(exchangeType), v.StockCode, 0)
	return &Entrust{
		Symbol:       symbol,
		EntrustID:    EntrustID(v.EntrustID),
		OrderID:      OrderID(v.EntrustID),
		Side:         v.EntrustBS,
		OrderType:    v.EntrustType,
		Price:        MustNewPrice(v.EntrustPrice, "0.001"),
		Quantity:     MustNewQuantity(v.EntrustAmount),
		FilledQty:    MustNewQuantity(v.BusinessAmount),
		AvgPrice:     MustNewPrice(v.BusinessPrice, "0.001"),
		Status:       v.Status,
		StatusDesc:   v.StatusDesc,
		CanCancel:    v.CanBeCanceled == 1,
		CanModify:    v.CanBeUpdated == 1,
		Date:         v.Date,
		Time:         v.EntrustTime,
		ExchangeType: exchangeType,
		RemarkType:   v.RemarkType,
		Remark:       v.Remark,
		OpponentSeat: v.OpponentSeat,
		Fare:         FareFromWire(v.FareVo),
	}
}

func FillFromWire(v *EntrustWire, exchangeType types.ExchangeType) *Fill {
	if v == nil {
		return nil
	}
	symbol := NewSymbol(MarketFromExchange(exchangeType), v.StockCode, 0)
	return &Fill{
		Symbol:       symbol,
		EntrustID:    EntrustID(v.EntrustID),
		OrderID:      OrderID(v.EntrustID),
		Side:         v.EntrustBS,
		Price:        MustNewPrice(v.BusinessPrice, "0.001"),
		Quantity:     MustNewQuantity(v.EntrustAmount),
		FilledQty:    MustNewQuantity(v.BusinessAmount),
		Turnover:     MustNewMoney(v.BusinessBalance, "HKD", 3),
		Status:       v.Status,
		StatusDesc:   v.StatusDesc,
		Date:         v.Date,
		Time:         v.BusinessTime,
		ExchangeType: exchangeType,
		CounterID:    v.OpponentSeat,
		Fare:         FareFromWire(v.FareVo),
	}
}

func FareFromWire(f *FareWire) *Fare {
	if f == nil {
		return nil
	}
	return &Fare{
		Commission:        MustNewMoney(f.Fare0, "HKD", 3),
		StampDuty:         MustNewMoney(f.Fare1, "HKD", 3),
		TradingFee:        MustNewMoney(f.Fare2, "HKD", 3),
		TransactionLevy:   MustNewMoney(f.Fare3, "HKD", 3),
		OptionRegFee:      MustNewMoney(f.Fare4, "HKD", 3),
		OptionClearingFee: MustNewMoney(f.Fare5, "HKD", 3),
		PlatformFee:       MustNewMoney(f.Fare6, "HKD", 3),
		AFRCLevy:          MustNewMoney(f.Fare9, "HKD", 3),
		SettlementFee:     MustNewMoney(f.FareX, "HKD", 3),
		Total:             MustNewMoney(f.FareT, "HKD", 3),
	}
}

func CondOrderFromWire(v *CondOrderWire) *CondOrder {
	if v == nil {
		return nil
	}
	return &CondOrder{
		CondOrderID:   v.CondOrderID,
		Symbol:        NewSymbol(MarketFromExchange(v.ExchangeType), v.StockCode, 0),
		Side:          v.EntrustBS,
		OrderType:     v.EntrustType,
		Quantity:      MustNewQuantity(v.EntrustAmount),
		Status:        v.Status,
		StatusDesc:    condOrderStatusDesc(v.Status),
		CanCancel:     v.CanBeCancel == "1",
		CanModify:     v.CanBeModify == "1",
		CreateTime:    v.CreateTime,
		StartTime:     v.StartTime,
		EndTime:       v.EndTime,
		CondValue:     v.CondValue,
		CondPrice:     v.CondPrice,
		CondTrackType: v.CondTrackType,
		ErrorCode:     v.ErrorCode,
		ErrorMsg:      v.ErrorMsg,
		SessionType:   v.SessionType,
	}
}

func condOrderStatusDesc(status string) string {
	switch status {
	case "1":
		return "pending trigger"
	case "2":
		return "triggered"
	case "3":
		return "paused"
	case "4":
		return "expired"
	case "5":
		return "deleted"
	case "6":
		return "error"
	case "8":
		return "stop invalidated"
	case "9":
		return "ex-rights invalidated"
	default:
		return status
	}
}

func MaxAvailableFromWire(m *MaxAvailableWire) *MaxAvailable {
	if m == nil {
		return nil
	}
	return &MaxAvailable{
		PositionStatus:               m.PositionStatus,
		Position:                     MustNewQuantity(m.Position),
		LongOpenAvailable:            MustNewQuantity(m.LongOpenAvailable),
		LongCloseAvailable:           MustNewQuantity(m.LongCloseAvailable),
		CashAvailableToOpen:          MustNewQuantity(m.CashAvailableToOpen),
		MarginAvailableToOpen:        MustNewQuantity(m.MarginAvailableToOpen),
		CashAndMarginAvailableToOpen: MustNewQuantity(m.CashAndMarginAvailableToOpen),
		CashAvailableAmount:          MustNewMoney(m.CashAvailableAmount, "HKD", 3),
		MarginAvailableAmount:        MustNewMoney(m.MarginAvailableAmount, "HKD", 3),
		CashAndMarginAvailableAmount: MustNewMoney(m.CashAndMarginAvailableAmount, "HKD", 3),
		ShortOpenAvailable:           MustNewQuantity(m.ShortOpenAvailable),
		ShortCloseAvailable:          MustNewQuantity(m.ShortCloseAvailable),
		ShortCloseCashAvailable:      MustNewQuantity(m.ShortCloseCashAvailable),
		ShortOpenPool:                MustNewQuantity(m.ShortOpenPool),
		ShortBuyPower:                MustNewMoney(m.ShortBuyPower, "HKD", 3),
		ContractSize:                 MustNewQuantity(m.ContractSize),
		UnitedBuyPowerStatus:         m.UnitedBuyPowerStatus,
		CreditLimit:                  MustNewMoney(m.CreditLimit, "HKD", 3),
		OptionLongMarginAmount:       MustNewMoney(m.OptionLongMarginAmount, "HKD", 3),
		OptionShortMarginAmount:      MustNewMoney(m.OptionShortMarginAmount, "HKD", 3),
	}
}

func MarginInfoFromWire(m *MarginFullInfoWire) *MarginInfo {
	if m == nil {
		return nil
	}
	info := &MarginInfo{
		MarginAllow:           m.MarginAllow == "1",
		MarginInitRatio:       MustNewRate(m.MarginInitRatio),
		MarginKeepRatio:       MustNewRate(m.MarginKeepRatio),
		ShortAllow:            m.ShortAllow == "1",
		ShortInitMarginRatio:  MustNewRate(m.ShortInitMarginRatio),
		ShortInterestRate:     MustNewRate(m.ShortInterestRate),
		ShortKeepMarginRatio:  MustNewRate(m.ShortKeepMarginRatio),
		ShortLastAvailableQty: MustNewQuantity(m.ShortLastAvailableQty),
		Rates:                 make([]*MarginInterestRate, len(m.Rate)),
	}
	for i, r := range m.Rate {
		info.Rates[i] = &MarginInterestRate{
			CurrencyCode:               r.CurrencyCode,
			CurrencyDesc:               r.CurrencyDesc,
			InterestRateWithinMortgage: MustNewRate(r.InterestRateWithinMortgage),
		}
	}
	return info
}

func SessionSupportFromWire(symbol Symbol, support string) *SessionSupport {
	return &SessionSupport{
		Symbol:     symbol,
		PreMarket:  support == "1",
		AfterHours: support == "1",
	}
}

type EntrustWire struct {
	StockCode        string              `json:"stockCode"`
	StockName        string              `json:"stockName"`
	BusinessPrice    string              `json:"businessPrice"`
	EntrustBS        types.EntrustBS     `json:"entrustBs"`
	EntrustPrice     string              `json:"entrustPrice"`
	BusinessBalance  string              `json:"businessBalance"`
	EntrustAmount    string              `json:"entrustAmount"`
	BusinessAmount   string              `json:"businessAmount"`
	Date             string              `json:"date"`
	BusinessTime     string              `json:"businessTime"`
	EntrustTime      string              `json:"entrustTime"`
	QueryParamStr    string              `json:"queryParamStr"`
	StatusDesc       string              `json:"statusDesc"`
	Status           types.EntrustStatus `json:"status"`
	EntrustID        string              `json:"entrustId"`
	UnBusinessAmount string              `json:"unBusinessAmount"`
	CanBeCanceled    int32               `json:"canBeCanceled"`
	EntrustType      types.EntrustType   `json:"entrustType"`
	OpponentSeat     string              `json:"opponentSeat"`
	EntrustTypeNum   string              `json:"entrustTypeNum"`
	RemarkType       string              `json:"remarkType"`
	Remark           string              `json:"remark"`
	FareVo           *FareWire           `json:"fareVo"`
	ExchangeType     types.ExchangeType  `json:"exchangeType"`
	CanBeUpdated     int32               `json:"canBeUpdated"`
	Exchange         string              `json:"exchange"`
}

type FareWire struct {
	Fare0 string `json:"fare0"`
	Fare1 string `json:"fare1"`
	Fare2 string `json:"fare2"`
	Fare3 string `json:"fare3"`
	Fare4 string `json:"fare4"`
	Fare5 string `json:"fare5"`
	Fare6 string `json:"fare6"`
	Fare7 string `json:"fare7"`
	Fare8 string `json:"fare8"`
	Fare9 string `json:"fare9"`
	FareX string `json:"farex"`
	FareT string `json:"faret"`
}

type CondOrderWire struct {
	CondOrderID   string             `json:"condOrderId"`
	DataType      string             `json:"dataType"`
	StockCode     string             `json:"stockCode"`
	StockName     string             `json:"stockName"`
	StockNameTc   string             `json:"stockNameTc"`
	StockNameEn   string             `json:"stockNameEn"`
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	EntrustType   types.EntrustType  `json:"entrustType"`
	SessionType   string             `json:"sessionType"`
	Status        string             `json:"status"`
	CanBeCancel   string             `json:"canBeCancel"`
	CanBeModify   string             `json:"canBeModify"`
	EntrustBS     types.EntrustBS    `json:"entrustBs"`
	EntrustAmount string             `json:"entrustAmount"`
	CreateTime    string             `json:"createTime"`
	StartTime     string             `json:"startTime"`
	EndTime       string             `json:"endTime"`
	ErrorCode     string             `json:"errorCode"`
	ErrorMsg      string             `json:"errorMsg"`
	CondValue     string             `json:"condValue"`
	CondPrice     string             `json:"condPrice"`
	CondTrackType string             `json:"condTrackType"`
}

type CondOrderPageWire struct {
	Data        []CondOrderWire `json:"data"`
	CurPageNo   int32           `json:"curPageNo"`
	CurPageSize int32           `json:"curPageSize"`
	TotalPages  int64           `json:"totalPages"`
}

type MaxAvailableWire struct {
	PositionStatus               string `json:"positionStatus"`
	Position                     string `json:"position"`
	LongOpenAvailable            string `json:"longOpenAvailable"`
	LongCloseAvailable           string `json:"longCloseAvailable"`
	CashAvailableToOpen          string `json:"cashAvailableToOpen"`
	MarginAvailableToOpen        string `json:"marginAvailableToOpen"`
	CashAndMarginAvailableToOpen string `json:"cashAndMarginAvailableToOpen"`
	CashAvailableAmount          string `json:"cashAvailableAmount"`
	MarginAvailableAmount        string `json:"marginAvailableAmount"`
	CashAndMarginAvailableAmount string `json:"cashAndMarginAvailableAmount"`
	ShortOpenAvailable           string `json:"shortOpenAvailable"`
	ShortCloseAvailable          string `json:"shortCloseAvailable"`
	ShortCloseCashAvailable      string `json:"shortCloseCashAvailable"`
	ShortOpenPool                string `json:"shortOpenPool"`
	ShortBuyPower                string `json:"shortBuyPower"`
	ContractSize                 string `json:"contractSize"`
	UnitedBuyPowerStatus         string `json:"unitedBuyPowerStatus"`
	CreditLimit                  string `json:"creditLimit"`
	OptionLongMarginAmount       string `json:"optionLongMarginAmount"`
	OptionShortMarginAmount      string `json:"optionShortMarginAmount"`
}

type MarginFullInfoWire struct {
	MarginAllow           string     `json:"marginAllow"`
	MarginInitRatio       string     `json:"marginInitRatio"`
	MarginKeepRatio       string     `json:"marginKeepRatio"`
	ShortAllow            string     `json:"shortAllow"`
	ShortInitMarginRatio  string     `json:"shortInitMarginRatio"`
	ShortInterestRate     string     `json:"shortInterestRate"`
	ShortKeepMarginRatio  string     `json:"shortKeepMarginRatio"`
	ShortLastAvailableQty string     `json:"shortLastAvailableQty"`
	Rate                  []RateWire `json:"rate"`
}

type RateWire struct {
	CurrencyCode               string `json:"currencyCode"`
	CurrencyDesc               string `json:"currencyDesc"`
	InterestRateWithinMortgage string `json:"interestRateWithinMortgage"`
}

type OrderListWire struct {
	Data []EntrustWire `json:"data"`
}

type CancelResultWire struct {
	SuccessEntrustID  []string         `json:"successEntrustId"`
	FailCancelEntrust []FailCancelWire `json:"failCancelEntrust"`
}

type FailCancelWire struct {
	FailEntrustID string `json:"failEntrustId"`
	Remark        string `json:"remark"`
}
