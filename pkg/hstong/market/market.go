// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package market

import (
	"context"
	"fmt"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels used in the typed errors raised by Manager. They mirror the
// package-qualified method names and never carry request payloads.
const (
	opBasicQot                = "market.BasicQot"
	opOrderBook               = "market.OrderBook"
	opKL                      = "market.KL"
	opTimeShare               = "market.TimeShare"
	opTicker                  = "market.Ticker"
	opBroker                  = "market.Broker"
	opUsOptionChainCode       = "market.UsOptionChainCode"
	opUsOptionChainExpireDate = "market.UsOptionChainExpireDate"
	opUsOverNightTradeCodes   = "market.UsOverNightTradeCodes"
)

// MaxTickerLimit is the largest tick count Ticker accepts. The reference
// documentation caps a single Ticker response at 100 records; a larger request
// is rejected with a typed StatusInvalidParam error rather than being clamped.
const MaxTickerLimit = 100

// Manager issues the nine market-data pull calls against a shared
// *client.Client. It holds no mutable state and is safe for concurrent use. A
// Manager must not be copied after first use.
type Manager struct {
	client *client.Client
}

// New returns a Manager over c. c must be non-nil and is not owned by the
// Manager: the caller remains responsible for calling c.Close.
func New(c *client.Client) *Manager {
	return &Manager{client: c}
}

// BasicQotRequest is the body of POST /hq/BasicQot. Security is required and
// must be non-empty; NeedDelayFlag and MktTmType are optional. MktTmType is
// always sent, so an explicit 0 (the Hong Kong session) is preserved.
type BasicQotRequest struct {
	// Security lists the instruments to quote. At least one is required.
	Security []*dto.Security `json:"security"`
	// NeedDelayFlag requests delayed data ("0" no, "1" yes). Empty omits it.
	NeedDelayFlag string `json:"needDelayFlag,omitempty"`
	// MktTmType selects the session: -1 pre-market, 1 intraday, -2
	// post-market, -3 overnight, 0 Hong Kong.
	MktTmType int32 `json:"mktTmType"`
}

// BasicQotResponse is the "data" body of POST /hq/BasicQot.
type BasicQotResponse struct {
	// BasicQot holds one quote per requested security, in request order.
	BasicQot []*dto.BasicQot `json:"basicQot"`
}

// BasicQot returns real-time quotes for the requested securities. It rejects
// an empty Security list with a typed StatusInvalidParam error and sends no
// request.
func (m *Manager) BasicQot(ctx context.Context, req BasicQotRequest) (BasicQotResponse, error) {
	if err := validateSecurityList(opBasicQot, req.Security); err != nil {
		return BasicQotResponse{}, err
	}
	var resp BasicQotResponse
	if err := m.client.Do(ctx, opBasicQot, client.RouteHqBasicQot, req, m.client.JSON(), &resp); err != nil {
		return BasicQotResponse{}, err
	}
	return resp, nil
}

// OrderBookRequest is the body of POST /hq/OrderBook. Security is required;
// MktTmType and DepthBookType are optional.
type OrderBookRequest struct {
	// Security is the instrument whose book is requested. It is required.
	Security *dto.Security `json:"security"`
	// MktTmType selects the session: -1 pre-market, 1 intraday, -2
	// post-market, -3 overnight, 0 Hong Kong.
	MktTmType int32 `json:"mktTmType"`
	// DepthBookType selects a depth book: 2 totalview, 3 arcabook. Zero
	// omits the field.
	DepthBookType int32 `json:"depthBookType,omitempty"`
}

// OrderBookResponse is the "data" body of POST /hq/OrderBook. The ask and bid
// lists are ordered from level 1 outward.
type OrderBookResponse struct {
	// Security echoes the requested instrument.
	Security *dto.Security `json:"security"`
	// OrderBookAskList is the ask (sell) side.
	OrderBookAskList []*dto.OrderBook `json:"orderBookAskList"`
	// OrderBookBidList is the bid (buy) side.
	OrderBookBidList []*dto.OrderBook `json:"orderBookBidList"`
	// TickSize is the minimum price step ("spreadLevel" on the wire). It is a
	// price tick, so it stays a float64.
	TickSize float64 `json:"spreadLevel"`
}

// OrderBook returns the real-time order book for a single security. A nil
// Security is rejected with a typed StatusInvalidParam error and sends no
// request.
func (m *Manager) OrderBook(ctx context.Context, req OrderBookRequest) (OrderBookResponse, error) {
	if err := validateSecurity(opOrderBook, req.Security); err != nil {
		return OrderBookResponse{}, err
	}
	var resp OrderBookResponse
	if err := m.client.Do(ctx, opOrderBook, client.RouteHqOrderBook, req, m.client.JSON(), &resp); err != nil {
		return OrderBookResponse{}, err
	}
	return resp, nil
}

// KLRequest is the body of POST /hq/KL. All fields are required.
type KLRequest struct {
	// Security is the instrument whose candles are requested.
	Security *dto.Security `json:"security"`
	// StartDate is the first day to query, formatted yyyyMMdd.
	StartDate int64 `json:"startDate"`
	// Direction selects the query direction (see the reference dictionary).
	Direction int32 `json:"direction"`
	// ExRightFlag selects the price-adjustment mode (see the dictionary).
	ExRightFlag int32 `json:"exRightFlag"`
	// CycType is the candle period (for example 1, 3, 5, 15, 30, 60, 120,
	// 240 minutes or a daily-and-above period); see the dictionary.
	CycType int32 `json:"cycType"`
	// Limit is the number of days (intraday periods) or candles (daily and
	// above) to return.
	Limit int32 `json:"limit"`
}

// KLResponse is the "data" body of POST /hq/KL.
type KLResponse struct {
	// Security echoes the requested instrument.
	Security *dto.Security `json:"security"`
	// Kline holds the candles, oldest first.
	Kline []*dto.KLine `json:"kline"`
}

// KL returns real-time candlesticks for a single security. A nil Security is
// rejected with a typed StatusInvalidParam error and sends no request.
func (m *Manager) KL(ctx context.Context, req KLRequest) (KLResponse, error) {
	if err := validateSecurity(opKL, req.Security); err != nil {
		return KLResponse{}, err
	}
	var resp KLResponse
	if err := m.client.Do(ctx, opKL, client.RouteHqKL, req, m.client.JSON(), &resp); err != nil {
		return KLResponse{}, err
	}
	return resp, nil
}

// TimeShareRequest is the body of POST /hq/TimeShare. Security is required;
// MktTmType is optional and always sent.
type TimeShareRequest struct {
	// Security is the instrument whose time-and-sales series is requested.
	Security *dto.Security `json:"security"`
	// MktTmType selects the session: -1 pre-market, 1 intraday, -2
	// post-market, -3 overnight, 0 Hong Kong.
	MktTmType int32 `json:"mktTmType"`
}

// TimeShareResponse is the "data" body of POST /hq/TimeShare.
type TimeShareResponse struct {
	// Security echoes the requested instrument.
	Security *dto.Security `json:"security"`
	// TimeShare is the intraday price/volume series.
	TimeShare []*dto.TimeShare `json:"timeShare"`
}

// TimeShare returns the real-time time-and-sales series for a single security.
// A nil Security is rejected with a typed StatusInvalidParam error and sends no
// request.
func (m *Manager) TimeShare(ctx context.Context, req TimeShareRequest) (TimeShareResponse, error) {
	if err := validateSecurity(opTimeShare, req.Security); err != nil {
		return TimeShareResponse{}, err
	}
	var resp TimeShareResponse
	if err := m.client.Do(ctx, opTimeShare, client.RouteHqTimeShare, req, m.client.JSON(), &resp); err != nil {
		return TimeShareResponse{}, err
	}
	return resp, nil
}

// TickerRequest is the body of POST /hq/Ticker. Security and Limit are
// required; Limit must be within 1..MaxTickerLimit. MktTmType is optional and
// always sent.
type TickerRequest struct {
	// Security is the instrument whose ticks are requested.
	Security *dto.Security `json:"security"`
	// Limit is the maximum number of ticks to return, within
	// 1..MaxTickerLimit.
	Limit int32 `json:"limit"`
	// MktTmType selects the session: -1 pre-market, 1 intraday, -2
	// post-market, -3 overnight, 0 Hong Kong.
	MktTmType int32 `json:"mktTmType"`
}

// TickerResponse is the "data" body of POST /hq/Ticker.
type TickerResponse struct {
	// Security echoes the requested instrument.
	Security *dto.Security `json:"security"`
	// Ticker holds the most recent ticks, newest first.
	Ticker []*dto.Ticker `json:"ticker"`
}

// Ticker returns recent tick-by-tick trades for a single security. It rejects a
// nil Security and a Limit outside 1..MaxTickerLimit with a typed
// StatusInvalidParam error and sends no request.
func (m *Manager) Ticker(ctx context.Context, req TickerRequest) (TickerResponse, error) {
	if err := validateSecurity(opTicker, req.Security); err != nil {
		return TickerResponse{}, err
	}
	if req.Limit < 1 || req.Limit > MaxTickerLimit {
		return TickerResponse{}, errs.New(types.StatusInvalidParam, opTicker,
			fmt.Sprintf("ticker limit %d out of range 1..%d", req.Limit, MaxTickerLimit))
	}
	var resp TickerResponse
	if err := m.client.Do(ctx, opTicker, client.RouteHqTicker, req, m.client.JSON(), &resp); err != nil {
		return TickerResponse{}, err
	}
	return resp, nil
}

// BrokerRequest is the body of POST /hq/Broker. Security is required. The
// endpoint supports Hong Kong instruments only.
type BrokerRequest struct {
	// Security is the Hong Kong instrument whose broker queue is requested.
	Security *dto.Security `json:"security"`
}

// BrokerResponse is the "data" body of POST /hq/Broker.
type BrokerResponse struct {
	// Security echoes the requested instrument.
	Security *dto.Security `json:"security"`
	// BrokerAskList is the ask-side broker queue.
	BrokerAskList []*dto.Broker `json:"brokerAskList"`
	// BrokerBidList is the bid-side broker queue.
	BrokerBidList []*dto.Broker `json:"brokerBidList"`
}

// Broker returns the real-time broker queue for a single Hong Kong security. A
// nil Security is rejected with a typed StatusInvalidParam error and sends no
// request.
func (m *Manager) Broker(ctx context.Context, req BrokerRequest) (BrokerResponse, error) {
	if err := validateSecurity(opBroker, req.Security); err != nil {
		return BrokerResponse{}, err
	}
	var resp BrokerResponse
	if err := m.client.Do(ctx, opBroker, client.RouteHqBroker, req, m.client.JSON(), &resp); err != nil {
		return BrokerResponse{}, err
	}
	return resp, nil
}

// UsOptionChainCodeRequest is the body of POST /hq/UsOptionChainCode.
// SecurityCode and ExpireDate are required; FlagInOut and OptionType are
// optional.
type UsOptionChainCodeRequest struct {
	// SecurityCode is the underlying stock code, for example "AAPL".
	SecurityCode string `json:"securityCode"`
	// ExpireDate is the option expiry, formatted yyyy/MM/dd.
	ExpireDate string `json:"expireDate"`
	// FlagInOut selects in/out of the money: 1 in, 2 out. Zero omits it.
	FlagInOut int32 `json:"flagInOut,omitempty"`
	// OptionType selects call ("C") or put ("P"). Empty omits it.
	OptionType string `json:"optionType,omitempty"`
}

// UsOptionChainCodeResponse is the "data" body of POST /hq/UsOptionChainCode.
type UsOptionChainCodeResponse struct {
	// OptionCode is the list of option contract codes matching the query.
	OptionCode []string `json:"optionCode"`
}

// UsOptionChainCode returns the option-chain code list for one underlying and
// expiry. An empty SecurityCode is rejected with a typed StatusInvalidParam
// error and sends no request.
func (m *Manager) UsOptionChainCode(ctx context.Context, req UsOptionChainCodeRequest) (UsOptionChainCodeResponse, error) {
	if err := validateSecurityCode(opUsOptionChainCode, req.SecurityCode); err != nil {
		return UsOptionChainCodeResponse{}, err
	}
	var resp UsOptionChainCodeResponse
	if err := m.client.Do(ctx, opUsOptionChainCode, client.RouteHqUsOptionChainCode, req, m.client.JSON(), &resp); err != nil {
		return UsOptionChainCodeResponse{}, err
	}
	return resp, nil
}

// UsOptionChainExpireDateRequest is the body of POST
// /hq/UsOptionChainExpireDate. SecurityCode is required.
type UsOptionChainExpireDateRequest struct {
	// SecurityCode is the underlying stock code, for example "AAPL".
	SecurityCode string `json:"securityCode"`
}

// UsOptionChainExpireDateResponse is the "data" body of POST
// /hq/UsOptionChainExpireDate.
type UsOptionChainExpireDateResponse struct {
	// ExpireDate is the list of option expiries, formatted yyyy/MM/dd.
	ExpireDate []string `json:"expireDate"`
}

// UsOptionChainExpireDate returns the option-chain expiry list for one
// underlying. An empty SecurityCode is rejected with a typed
// StatusInvalidParam error and sends no request.
func (m *Manager) UsOptionChainExpireDate(ctx context.Context, req UsOptionChainExpireDateRequest) (UsOptionChainExpireDateResponse, error) {
	if err := validateSecurityCode(opUsOptionChainExpireDate, req.SecurityCode); err != nil {
		return UsOptionChainExpireDateResponse{}, err
	}
	var resp UsOptionChainExpireDateResponse
	if err := m.client.Do(ctx, opUsOptionChainExpireDate, client.RouteHqUsOptionChainExpireDate, req, m.client.JSON(), &resp); err != nil {
		return UsOptionChainExpireDateResponse{}, err
	}
	return resp, nil
}

// UsOverNightTradeCodesRequest is the body of POST
// /hq/UsOverNightTradeCodes. The endpoint takes no parameters; the empty struct
// serialises to the documented empty params object.
type UsOverNightTradeCodesRequest struct{}

// UsOverNightTradeCodesResponse is the "data" body of POST
// /hq/UsOverNightTradeCodes.
type UsOverNightTradeCodesResponse struct {
	// SecurityCodes is the list of US codes tradable overnight.
	SecurityCodes []string `json:"securityCodes"`
}

// UsOverNightTradeCodes returns the US overnight-tradable code list. The
// request carries no parameters and is never rejected locally.
func (m *Manager) UsOverNightTradeCodes(ctx context.Context, req UsOverNightTradeCodesRequest) (UsOverNightTradeCodesResponse, error) {
	var resp UsOverNightTradeCodesResponse
	if err := m.client.Do(ctx, opUsOverNightTradeCodes, client.RouteHqUsOverNightTradeCodes, req, m.client.JSON(), &resp); err != nil {
		return UsOverNightTradeCodesResponse{}, err
	}
	return resp, nil
}

// validateSecurityList rejects a nil or empty security list as an invalid
// parameter.
func validateSecurityList(op string, secs []*dto.Security) error {
	if len(secs) == 0 {
		return errs.New(types.StatusInvalidParam, op, "security list must not be empty")
	}
	for i, s := range secs {
		if s == nil {
			return errs.New(types.StatusInvalidParam, op, fmt.Sprintf("security[%d] must not be nil", i))
		}
	}
	return nil
}

// validateSecurity rejects a nil security as an invalid parameter.
func validateSecurity(op string, s *dto.Security) error {
	if s == nil {
		return errs.New(types.StatusInvalidParam, op, "security must not be nil")
	}
	return nil
}

// validateSecurityCode rejects an empty underlying code as an invalid
// parameter.
func validateSecurityCode(op, code string) error {
	if code == "" {
		return errs.New(types.StatusInvalidParam, op, "securityCode must not be empty")
	}
	return nil
}
