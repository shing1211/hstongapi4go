// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels used in errors raised by Manager. Each matches the Gateway
// path so a log line or error points at the failing endpoint. They never carry
// request payloads or credentials.
const (
	opMarginFundInfo      = "trade/TradeQueryMarginFundInfo"
	opHoldsList           = "trade/TradeQueryHoldsList"
	opRealFundJourList    = "trade/TradeQueryRealFundJourList"
	opHistoryFundJourList = "trade/TradeQueryHistoryFundJourList"
	opRateQueryList       = "hs/rate/queryList"
)

// Session is the subset of hstong.SessionManager that a trade Manager uses. A
// nil Session, the default, makes the Manager send every call without touching
// session state. *hstong.SessionManager satisfies Session.
type Session interface {
	// EnsureLoggedIn returns nil when the session is live and otherwise performs
	// a single-flight login.
	EnsureLoggedIn(ctx context.Context) error
	// ReLogin forces a fresh login when cause indicates the session is invalid,
	// reporting whether it handled the cause.
	ReLogin(ctx context.Context, cause error) (handled bool, err error)
}

// Option configures a Manager. Options are applied in order on top of the
// defaults inside New; the last option that sets a field wins.
type Option func(*config)

// config holds the resolved Manager settings.
type config struct {
	session Session
}

// WithSession attaches a Session to the Manager. When s is nil the Manager has
// no session and sends each call directly. A non-nil session is consulted
// before every call and on a re-login failure.
func WithSession(s Session) Option {
	return func(c *config) { c.session = s }
}

// Manager is the trading-surface client for one *client.Client. It is created
// once with New and is safe for concurrent use. Every method takes a context and
// honors its deadline. A Manager is immutable after New except for the state it
// delegates to the client and session.
type Manager struct {
	client  *client.Client
	session Session
}

// New returns a Manager over c with opts applied in order. c must be non-nil and
// is not owned by the Manager: closing it is the caller's responsibility. A
// Manager without a Session works and simply relies on the caller to have
// logged in already.
func New(c *client.Client, opts ...Option) *Manager {
	cfg := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Manager{client: c, session: cfg.session}
}

// call performs exactly one Gateway request for op with the JSON codec. When a
// Session is attached it first ensures the session is logged in, and if the
// call fails with a re-login condition it asks the session to re-login before
// returning the original error. call never retries the endpoint request, so an
// order mutation that failed ambiguously is returned for the caller to
// reconcile (docs/adr/0003-no-auto-retry-orders.md).
func (m *Manager) call(ctx context.Context, op string, route client.Route, params, out any) error {
	if m.session != nil {
		if err := m.session.EnsureLoggedIn(ctx); err != nil {
			return err
		}
	}

	err := m.client.Do(ctx, op, route, params, m.client.JSON(), out)
	if err != nil && m.session != nil && errs.ReLoginRequired(err) {
		// Best-effort: the login outcome is secondary to the endpoint error the
		// caller must see. The endpoint call itself is never repeated.
		_, _ = m.session.ReLogin(ctx, err)
	}
	return err
}

// invalid returns a typed error for a locally detected request problem. It uses
// the documented "1016" invalid-parameter status code so callers can match it
// with errors.Is / errs.CodeOf like any Gateway rejection.
func invalid(op, message string) error {
	return errs.New(types.StatusInvalidParam, op, message)
}

// MarginFundInfoRequest is the body of /trade/TradeQueryMarginFundInfo.
type MarginFundInfoRequest struct {
	// ExchangeType selects the market: K (Hong Kong), P (US), v (Shenzhen
	// Connect), or t (Shanghai Connect). It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// MarginFundInfo is an account-funds snapshot. Every amount is the exact
// decimal string the Gateway sent.
//
// HoldsBalance and MarketValue are deprecated by the vendor as unreliable and
// should not be relied upon.
type MarginFundInfo struct {
	// HoldsBalance is the position profit/loss. Deprecated: unreliable, not
	// recommended.
	HoldsBalance string `json:"holdsBalance"`
	// AssetBalance is the total assets.
	AssetBalance string `json:"assetBalance"`
	// EnableBalance is the available cash.
	EnableBalance string `json:"enableBalance"`
	// MarketValue is the securities market value. Deprecated: unreliable, not
	// recommended.
	MarketValue string `json:"marketValue"`
	// CashOnHold is the frozen trading amount.
	CashOnHold string `json:"cashOnHold"`
	// CreditValue is the used credit.
	CreditValue string `json:"creditValue"`
	// CreditLine is the credit limit.
	CreditLine string `json:"creditLine"`
	// FetchBalance is the withdrawable cash.
	FetchBalance string `json:"fetchBalance"`
	// FrozenBalance is the total frozen amount.
	FrozenBalance string `json:"frozenBalance"`
	// AccountStatus is the account status.
	AccountStatus string `json:"accountStatus"`
	// SpentRatio is the fund-usage ratio.
	SpentRatio string `json:"spentRatio"`
	// CurrentCreditLimit is the current credit limit.
	CurrentCreditLimit string `json:"currentCreditLimit"`
	// MaxCreditLimit is the raised maximum credit limit.
	MaxCreditLimit string `json:"maxCreditLimit"`
	// UnitCreditLimit is the unified credit limit.
	UnitCreditLimit string `json:"unitCreditLimit"`
	// UnitMaxCreditLimit is the maximum unified credit limit.
	UnitMaxCreditLimit string `json:"unitMaxCreditLimit"`
	// BuyPower is the purchasing power for the primary currency of the unified
	// or single market.
	BuyPower string `json:"buyPower"`
	// BuyPowerCredit is the purchasing power after a credit-limit increase.
	BuyPowerCredit string `json:"buyPowerCredit"`
	// BuyPowerHk is the Hong Kong purchasing power.
	BuyPowerHk string `json:"buyPowerHk"`
	// BuyPowerUs is the US purchasing power.
	BuyPowerUs string `json:"buyPowerUs"`
	// BuyPowerCn is the Stock Connect purchasing power.
	BuyPowerCn string `json:"buyPowerCn"`
	// UnitedBuyPowerHk is the unified Hong Kong purchasing power.
	UnitedBuyPowerHk string `json:"unitedBuyPowerHk"`
	// UnitedBuyPowerUs is the unified US purchasing power.
	UnitedBuyPowerUs string `json:"unitedBuyPowerUs"`
	// UnitedBuyPowerCn is the unified Stock Connect purchasing power.
	UnitedBuyPowerCn string `json:"unitedBuyPowerCn"`
	// ThirdBuyPowerHk is the third-party Hong Kong purchasing power.
	ThirdBuyPowerHk string `json:"thirdBuyPowerHk"`
	// ThirdBuyPowerUs is the third-party US purchasing power.
	ThirdBuyPowerUs string `json:"thirdBuyPowerUs"`
	// ThirdBuyPowerCn is the third-party Stock Connect purchasing power.
	ThirdBuyPowerCn string `json:"thirdBuyPowerCn"`
	// BuyPowerShortMarket is the short-selling purchasing power.
	BuyPowerShortMarket string `json:"buyPowerShortMarket"`
	// BuyPowerMoney is the cash purchasing power.
	BuyPowerMoney string `json:"buyPowerMoney"`
}

// MarginFundInfo returns the account-funds snapshot for the requested market.
func (m *Manager) MarginFundInfo(ctx context.Context, req MarginFundInfoRequest) (*MarginFundInfo, error) {
	var out MarginFundInfo
	if err := m.call(ctx, opMarginFundInfo, client.RouteTradeQueryMarginFundInfo, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PositionsRequest is the body of /trade/TradeQueryHoldsList. ExchangeType is
// optional; when empty the Gateway returns positions for every market.
type PositionsRequest struct {
	// ExchangeType selects the market. It is optional.
	ExchangeType types.ExchangeType `json:"exchangeType,omitempty"`
}

// HoldsVo is one position.
//
// LastPrice, IncomeBalance, MarketValue, MarketValueRate, and IncomeRatio are
// deprecated by the vendor as unreliable and should not be relied upon; they
// are decoded only so a caller can inspect what the Gateway sent.
type HoldsVo struct {
	// StockName is the security name.
	StockName string `json:"stockName"`
	// EnableAmount is the sellable quantity.
	EnableAmount string `json:"enableAmount"`
	// CurrentAmount is the held quantity.
	CurrentAmount string `json:"currentAmount"`
	// StockCode is the security code.
	StockCode string `json:"stockCode"`
	// CostPrice is the cost price.
	CostPrice string `json:"costPrice"`
	// LastPrice is the latest price. Deprecated: unreliable, not recommended.
	LastPrice string `json:"lastPrice"`
	// IncomeBalance is the floating profit/loss. Deprecated: unreliable, not
	// recommended.
	IncomeBalance string `json:"incomeBalance"`
	// MarketValue is the security market value. Deprecated: unreliable, not
	// recommended.
	MarketValue string `json:"marketValue"`
	// MarketValueRate is the profit/loss ratio. Deprecated: unreliable, not
	// recommended.
	MarketValueRate string `json:"marketValueRate"`
	// IncomeRatio is the market-value proportion. Deprecated: unreliable, not
	// recommended.
	IncomeRatio string `json:"incomeRatio"`
	// DayCostPrice is the intraday cost price.
	DayCostPrice string `json:"dayCostPrice"`
	// DayInComeAmount is the effective intraday deposit.
	DayInComeAmount string `json:"dayInComeAmount"`
	// DayOutComeAmount is the effective intraday withdrawal.
	DayOutComeAmount string `json:"dayOutComeAmount"`
	// KeepCostPrice is the break-even price.
	KeepCostPrice string `json:"keepCostPrice"`
	// ExchangeType is the market of the position.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// HoldsListResponse is the decoded body of /trade/TradeQueryHoldsList. The
// legacy wire definition (StockQueryHoldsListResponse) wraps the rows in a
// holdsList array, while the current reference page shows a single row inline.
// UnmarshalJSON accepts either form so both decode faithfully.
type HoldsListResponse struct {
	// HoldsList is every position row in the response.
	HoldsList []HoldsVo `json:"holdsList"`
}

// UnmarshalJSON decodes either {"holdsList":[...]}, a bare array, or a single
// inline position object.
func (r *HoldsListResponse) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []HoldsVo
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		r.HoldsList = list
		return nil
	}

	var wrapped struct {
		HoldsList []HoldsVo `json:"holdsList"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return err
	}
	if wrapped.HoldsList != nil {
		r.HoldsList = wrapped.HoldsList
		return nil
	}

	var single HoldsVo
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return err
	}
	if single.StockCode != "" || single.StockName != "" {
		r.HoldsList = []HoldsVo{single}
	}
	return nil
}

// Positions returns the account's positions for the requested market. An empty
// ExchangeType asks the Gateway for every market.
func (m *Manager) Positions(ctx context.Context, req PositionsRequest) ([]HoldsVo, error) {
	var out HoldsListResponse
	if err := m.call(ctx, opHoldsList, client.RouteTradeQueryHoldsList, req, &out); err != nil {
		return nil, err
	}
	return out.HoldsList, nil
}

// FundJourListRequest is the body of /trade/TradeQueryRealFundJourList.
type FundJourListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// QueryCount is the page size. Paginate clamps it to [1, MaxPageSize]; the
	// documented default is DefaultPageSize.
	QueryCount int `json:"queryCount"`
	// QueryParamStr is the cursor. Start at "0" and pass the last row's
	// QueryParamStr to fetch the next page.
	QueryParamStr string `json:"queryParamStr"`
}

// HistoryFundJourListRequest is the body of
// /trade/TradeQueryHistoryFundJourList.
type HistoryFundJourListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// QueryCount is the page size.
	QueryCount int `json:"queryCount"`
	// QueryParamStr is the cursor.
	QueryParamStr string `json:"queryParamStr"`
	// StartDate is the inclusive start date in yyyyMMdd form.
	StartDate string `json:"startDate"`
	// EndDate is the inclusive end date in yyyyMMdd form.
	EndDate string `json:"endDate"`
}

// FundJourVo is one fund-journey (资金流水) row.
type FundJourVo struct {
	// BusinessBalance is the occurred amount.
	BusinessBalance string `json:"businessBalance"`
	// Type is the numeric flow type.
	Type string `json:"type"`
	// TypeDesc is the flow type description.
	TypeDesc string `json:"typeDesc"`
	// FundJourParentType is the parent flow type.
	FundJourParentType string `json:"fundJourParentType"`
	// Time is the settlement date. Present for historical rows only.
	Time string `json:"time"`
	// QueryParamStr is the cursor for the next page.
	QueryParamStr string `json:"queryParamStr"`
}

// FundJourListResponse is the decoded body shared by the real and historical
// fund-journey queries: a data array of rows.
type FundJourListResponse struct {
	// Data is every row in the response.
	Data []FundJourVo `json:"data"`
}

// RealFundJourList returns one page of the day's fund journeys. Use Paginate to
// walk every page.
func (m *Manager) RealFundJourList(ctx context.Context, req FundJourListRequest) ([]FundJourVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out FundJourListResponse
	if err := m.call(ctx, opRealFundJourList, client.RouteTradeQueryRealFundJourList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// HistoryFundJourList returns one page of the historical fund journeys between
// StartDate and EndDate. Use Paginate to walk every page.
func (m *Manager) HistoryFundJourList(ctx context.Context, req HistoryFundJourListRequest) ([]FundJourVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out FundJourListResponse
	if err := m.call(ctx, opHistoryFundJourList, client.RouteTradeQueryHistoryFundJourList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// ExchangeRateRequest is the body of /hs/rate/queryList. RateType is optional.
type ExchangeRateRequest struct {
	// RateType is "0" for the exchange rate or "1" for the spot rate. An empty
	// value asks the Gateway for the exchange rate.
	RateType string `json:"rateType,omitempty"`
}

// ExchangeRates is the /hs/rate/queryList response: the outer key is the source
// currency, the inner key the target currency, and the value the decimal rate
// string (for example rates["HKD"]["USD"]).
type ExchangeRates map[string]map[string]string

// ExchangeRate returns the current CNY/HKD/USD exchange rates.
func (m *Manager) ExchangeRate(ctx context.Context, req ExchangeRateRequest) (ExchangeRates, error) {
	out := ExchangeRates{}
	if err := m.call(ctx, opRateQueryList, client.RouteHsRateQueryList, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}
