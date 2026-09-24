// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

type AccountBalance struct {
	HoldsBalance        Money
	AssetBalance        Money
	EnableBalance       Money
	MarketValue         Money
	CashOnHold          Money
	CreditValue         Money
	CreditLine          Money
	FetchBalance        Money
	FrozenBalance       Money
	AccountStatus       string
	SpentRatio          Rate
	CurrentCreditLimit  Money
	MaxCreditLimit      Money
	UnitCreditLimit     Money
	UnitMaxCreditLimit  Money
	BuyPower            Money
	BuyPowerCredit      Money
	BuyPowerHk          Money
	BuyPowerUs          Money
	BuyPowerCn          Money
	UnitedBuyPowerHk    Money
	UnitedBuyPowerUs    Money
	UnitedBuyPowerCn    Money
	ThirdBuyPowerHk     Money
	ThirdBuyPowerUs     Money
	ThirdBuyPowerCn     Money
	BuyPowerShortMarket Money
	BuyPowerMoney       Money
}

func AccountBalanceFromDTO(m *MarginFundInfoWire) *AccountBalance {
	if m == nil {
		return nil
	}
	return &AccountBalance{
		HoldsBalance:        MustNewMoney(m.HoldsBalance, "HKD", 3),
		AssetBalance:        MustNewMoney(m.AssetBalance, "HKD", 3),
		EnableBalance:       MustNewMoney(m.EnableBalance, "HKD", 3),
		MarketValue:         MustNewMoney(m.MarketValue, "HKD", 3),
		CashOnHold:          MustNewMoney(m.CashOnHold, "HKD", 3),
		CreditValue:         MustNewMoney(m.CreditValue, "HKD", 3),
		CreditLine:          MustNewMoney(m.CreditLine, "HKD", 3),
		FetchBalance:        MustNewMoney(m.FetchBalance, "HKD", 3),
		FrozenBalance:       MustNewMoney(m.FrozenBalance, "HKD", 3),
		AccountStatus:       m.AccountStatus,
		SpentRatio:          MustNewRate(m.SpentRatio),
		CurrentCreditLimit:  MustNewMoney(m.CurrentCreditLimit, "HKD", 3),
		MaxCreditLimit:      MustNewMoney(m.MaxCreditLimit, "HKD", 3),
		UnitCreditLimit:     MustNewMoney(m.UnitCreditLimit, "HKD", 3),
		UnitMaxCreditLimit:  MustNewMoney(m.UnitMaxCreditLimit, "HKD", 3),
		BuyPower:            MustNewMoney(m.BuyPower, "HKD", 3),
		BuyPowerCredit:      MustNewMoney(m.BuyPowerCredit, "HKD", 3),
		BuyPowerHk:          MustNewMoney(m.BuyPowerHk, "HKD", 3),
		BuyPowerUs:          MustNewMoney(m.BuyPowerUs, "USD", 3),
		BuyPowerCn:          MustNewMoney(m.BuyPowerCn, "CNY", 3),
		UnitedBuyPowerHk:    MustNewMoney(m.UnitedBuyPowerHk, "HKD", 3),
		UnitedBuyPowerUs:    MustNewMoney(m.UnitedBuyPowerUs, "USD", 3),
		UnitedBuyPowerCn:    MustNewMoney(m.UnitedBuyPowerCn, "CNY", 3),
		ThirdBuyPowerHk:     MustNewMoney(m.ThirdBuyPowerHk, "HKD", 3),
		ThirdBuyPowerUs:     MustNewMoney(m.ThirdBuyPowerUs, "USD", 3),
		ThirdBuyPowerCn:     MustNewMoney(m.ThirdBuyPowerCn, "CNY", 3),
		BuyPowerShortMarket: MustNewMoney(m.BuyPowerShortMarket, "HKD", 3),
		BuyPowerMoney:       MustNewMoney(m.BuyPowerMoney, "HKD", 3),
	}
}

type Position struct {
	StockName        string
	EnableAmount     Quantity
	CurrentAmount    Quantity
	StockCode        string
	CostPrice        Price
	LastPrice        Price
	IncomeBalance    Money
	MarketValue      Money
	MarketValueRate  Rate
	IncomeRatio      Rate
	DayCostPrice     Price
	DayInComeAmount  Money
	DayOutComeAmount Money
	KeepCostPrice    Price
	ExchangeType     string
}

func PositionFromDTO(h *HoldsVoWire) *Position {
	if h == nil {
		return nil
	}
	return &Position{
		StockName:        h.StockName,
		EnableAmount:     MustNewQuantity(h.EnableAmount),
		CurrentAmount:    MustNewQuantity(h.CurrentAmount),
		StockCode:        h.StockCode,
		CostPrice:        MustNewPrice(h.CostPrice, "0.001"),
		LastPrice:        MustNewPrice(h.LastPrice, "0.001"),
		IncomeBalance:    MustNewMoney(h.IncomeBalance, "HKD", 3),
		MarketValue:      MustNewMoney(h.MarketValue, "HKD", 3),
		MarketValueRate:  MustNewRate(h.MarketValueRate),
		IncomeRatio:      MustNewRate(h.IncomeRatio),
		DayCostPrice:     MustNewPrice(h.DayCostPrice, "0.001"),
		DayInComeAmount:  MustNewMoney(h.DayInComeAmount, "HKD", 3),
		DayOutComeAmount: MustNewMoney(h.DayOutComeAmount, "HKD", 3),
		KeepCostPrice:    MustNewPrice(h.KeepCostPrice, "0.001"),
		ExchangeType:     h.ExchangeType,
	}
}

type FundJournalEntry struct {
	BusinessBalance    Money
	Type               string
	TypeDesc           string
	FundJourParentType string
	Time               string
	QueryParamStr      string
}

func FundJournalEntryFromDTO(f *FundJourVoWire) *FundJournalEntry {
	if f == nil {
		return nil
	}
	return &FundJournalEntry{
		BusinessBalance:    MustNewMoney(f.BusinessBalance, "HKD", 3),
		Type:               f.Type,
		TypeDesc:           f.TypeDesc,
		FundJourParentType: f.FundJourParentType,
		Time:               f.Time,
		QueryParamStr:      f.QueryParamStr,
	}
}

type InterestRate struct {
	SourceCurrency string
	TargetCurrency string
	Rate           Rate
}

func InterestRateFromDTO(source, target, rate string) *InterestRate {
	return &InterestRate{
		SourceCurrency: source,
		TargetCurrency: target,
		Rate:           MustNewRate(rate),
	}
}

type MarginFundInfoWire struct {
	HoldsBalance        string `json:"holdsBalance"`
	AssetBalance        string `json:"assetBalance"`
	EnableBalance       string `json:"enableBalance"`
	MarketValue         string `json:"marketValue"`
	CashOnHold          string `json:"cashOnHold"`
	CreditValue         string `json:"creditValue"`
	CreditLine          string `json:"creditLine"`
	FetchBalance        string `json:"fetchBalance"`
	FrozenBalance       string `json:"frozenBalance"`
	AccountStatus       string `json:"accountStatus"`
	SpentRatio          string `json:"spentRatio"`
	CurrentCreditLimit  string `json:"currentCreditLimit"`
	MaxCreditLimit      string `json:"maxCreditLimit"`
	UnitCreditLimit     string `json:"unitCreditLimit"`
	UnitMaxCreditLimit  string `json:"unitMaxCreditLimit"`
	BuyPower            string `json:"buyPower"`
	BuyPowerCredit      string `json:"buyPowerCredit"`
	BuyPowerHk          string `json:"buyPowerHk"`
	BuyPowerUs          string `json:"buyPowerUs"`
	BuyPowerCn          string `json:"buyPowerCn"`
	UnitedBuyPowerHk    string `json:"unitedBuyPowerHk"`
	UnitedBuyPowerUs    string `json:"unitedBuyPowerUs"`
	UnitedBuyPowerCn    string `json:"unitedBuyPowerCn"`
	ThirdBuyPowerHk     string `json:"thirdBuyPowerHk"`
	ThirdBuyPowerUs     string `json:"thirdBuyPowerUs"`
	ThirdBuyPowerCn     string `json:"thirdBuyPowerCn"`
	BuyPowerShortMarket string `json:"buyPowerShortMarket"`
	BuyPowerMoney       string `json:"buyPowerMoney"`
}

type HoldsVoWire struct {
	StockName        string `json:"stockName"`
	EnableAmount     string `json:"enableAmount"`
	CurrentAmount    string `json:"currentAmount"`
	StockCode        string `json:"stockCode"`
	CostPrice        string `json:"costPrice"`
	LastPrice        string `json:"lastPrice"`
	IncomeBalance    string `json:"incomeBalance"`
	MarketValue      string `json:"marketValue"`
	MarketValueRate  string `json:"marketValueRate"`
	IncomeRatio      string `json:"incomeRatio"`
	DayCostPrice     string `json:"dayCostPrice"`
	DayInComeAmount  string `json:"dayInComeAmount"`
	DayOutComeAmount string `json:"dayOutComeAmount"`
	KeepCostPrice    string `json:"keepCostPrice"`
	ExchangeType     string `json:"exchangeType"`
}

type FundJourVoWire struct {
	BusinessBalance    string `json:"businessBalance"`
	Type               string `json:"type"`
	TypeDesc           string `json:"typeDesc"`
	FundJourParentType string `json:"fundJourParentType"`
	Time               string `json:"time"`
	QueryParamStr      string `json:"queryParamStr"`
}
