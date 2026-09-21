// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo

import (
	"context"
	"errors"
	"fmt"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels attached to errors raised by Manager methods. They mirror
// the canonical route and never carry request payloads or credentials.
const (
	opAddOrder           = "trade/AlgoAddOrder"
	opCancelOrder        = "trade/AlgoCancelOrder"
	opCancelEntrust      = "trade/AlgoCancelEntrust"
	opChangeOrder        = "trade/AlgoChangeOrder"
	opActionOrder        = "trade/AlgoActionOrder"
	opQueryOrderList     = "trade/AlgoQueryOrderList"
	opQueryEntrustIDList = "trade/AlgoQueryEntrustIdList"
)

// ErrInvalidParams is the sentinel returned by a Manager method when its
// arguments fail local validation before any HTTP request is sent. It is
// wrapped with the operation and the offending field, so callers can branch
// with errors.Is(err, algo.ErrInvalidParams) and still read the message.
var ErrInvalidParams = errors.New("algo: invalid params")

// Strategy identifies the execution algorithm bound to a master order
// (targetStrategy). The AddOrder table documents '1'-VWAP, '1001'-TWAP,
// '1002'-ICE_BERG, and '1005'-POV; the request-example comments and the
// master-order query additionally label '1003' as TPOV and '1005' as INLINE.
// Because '1005' is labelled POV in the table but INLINE in the examples, the
// set is non-exhaustive and ambiguous: only emptiness is rejected locally and
// unknown codes are forwarded to the Gateway unchanged.
type Strategy string

const (
	// StrategyVWAP ("1") is the volume-weighted average price algorithm.
	StrategyVWAP Strategy = "1"
	// StrategyTWAP ("1001") is the time-weighted average price algorithm.
	StrategyTWAP Strategy = "1001"
	// StrategyIceberg ("1002") is the ICE_BERG incremental-display algorithm.
	StrategyIceberg Strategy = "1002"
	// StrategyTPOV ("1003") is the TPOV participation-of-volume algorithm
	// documented in the request examples; it is absent from the AddOrder table.
	StrategyTPOV Strategy = "1003"
	// StrategyPOV ("1005") is the participation-of-volume algorithm. The
	// reference labels the same code INLINE in the request examples, so callers
	// relying on it should confirm the contract with the vendor.
	StrategyPOV Strategy = "1005"
)

// EntrustType is the algorithm order type. It is distinct from the trade
// surface's types.EntrustType: the algorithm documentation uses '1' for limit
// and '2' for market, whereas the trade dictionary uses other codes for those
// same meanings, so the two must not be mixed.
type EntrustType string

const (
	// EntrustTypeLimit ("1") is a limit order (限价单).
	EntrustTypeLimit EntrustType = "1"
	// EntrustTypeMarket ("2") is a market order (市价单).
	EntrustTypeMarket EntrustType = "2"
)

// valid reports whether t is one of the two documented algorithm order types.
func (t EntrustType) valid() bool {
	return t == EntrustTypeLimit || t == EntrustTypeMarket
}

// SessionType controls pre-market and after-hours trading.
type SessionType string

const (
	// SessionTypeOff ("0") disables pre-market/after-hours trading.
	SessionTypeOff SessionType = "0"
	// SessionTypeOn ("1") enables pre-market/after-hours trading.
	SessionTypeOn SessionType = "1"
)

// valid reports whether s is one of the two documented session-type codes.
func (s SessionType) valid() bool {
	return s == SessionTypeOff || s == SessionTypeOn
}

// Sensitivity is the execution aggressiveness requested from a strategy.
type Sensitivity string

const (
	// SensitivityNeutral ("1") is neutral execution.
	SensitivityNeutral Sensitivity = "1"
	// SensitivityAggressive ("2") is aggressive execution.
	SensitivityAggressive Sensitivity = "2"
	// SensitivityPassive ("3") is passive execution.
	SensitivityPassive Sensitivity = "3"
)

// valid reports whether s is one of the three documented sensitivity codes.
func (s Sensitivity) valid() bool {
	switch s {
	case SensitivityNeutral, SensitivityAggressive, SensitivityPassive:
		return true
	default:
		return false
	}
}

// Status is the lifecycle state of a master order as reported by
// QueryOrderList. The codes are the algorithm-specific set documented on
// query-algo-master-order.html and differ from types.EntrustStatus.
type Status string

const (
	// StatusRegistered ("0") is 已报, accepted by the platform.
	StatusRegistered Status = "0"
	// StatusPartFilled ("1") is 部分成交, partially filled.
	StatusPartFilled Status = "1"
	// StatusFilled ("2") is 全部成交, fully filled.
	StatusFilled Status = "2"
	// StatusCompleted ("3") is 当日完成, completed for the day.
	StatusCompleted Status = "3"
	// StatusCancelled ("4") is 已撤, cancelled.
	StatusCancelled Status = "4"
	// StatusModified ("5") is 已改, modified.
	StatusModified Status = "5"
	// StatusWaitCancel ("6") is 待撤, waiting to cancel.
	StatusWaitCancel Status = "6"
	// StatusRejected ("8") is 废单, rejected.
	StatusRejected Status = "8"
	// StatusPending ("A") is 待报, waiting to be submitted.
	StatusPending Status = "A"
	// StatusExpired ("C") is 过期, expired.
	StatusExpired Status = "C"
	// StatusWaitModify ("E") is 待改, waiting to modify.
	StatusWaitModify Status = "E"
)

// StrategyStatus is the run state of a strategy in a master-order query
// response. It shares its code set with Action.
type StrategyStatus string

const (
	// StrategyStatusStart ("1") is a started strategy.
	StrategyStatusStart StrategyStatus = "1"
	// StrategyStatusStop ("2") is a stopped strategy.
	StrategyStatusStop StrategyStatus = "2"
	// StrategyStatusSuspend ("3") is a suspended strategy.
	StrategyStatusSuspend StrategyStatus = "3"
	// StrategyStatusResume ("4") is a resumed strategy.
	StrategyStatusResume StrategyStatus = "4"
)

// StrategyParam is the per-strategy tuning object accepted by AddOrder and
// ChangeOrder and returned inside a master-order query result. Every monetary
// or quantitative member is a string; Interval is the only numeric field and is
// a whole number of seconds.
type StrategyParam struct {
	// OrigStartTime is the strategy start time in Hong Kong time, HHmmSS. Empty
	// means the exchange open (or the current system time once open).
	OrigStartTime string `json:"origStartTime,omitempty"`
	// OrigEndTime is the strategy end time in Hong Kong time, HHmmSS. Empty
	// means the exchange close.
	OrigEndTime string `json:"origEndTime,omitempty"`
	// MaxVolume is the maximum quantity of each child order. AddOrder requires
	// it; ChangeOrder does not.
	MaxVolume string `json:"maxVolume,omitempty"`
	// MinAmount is the minimum traded amount of each child order.
	MinAmount string `json:"minAmount,omitempty"`
	// Sensitivity is the execution aggressiveness. AddOrder requires it;
	// ChangeOrder does not.
	Sensitivity Sensitivity `json:"sensitivity,omitempty"`
	// ShowQty is the displayed quantity of each ICE_BERG child order.
	ShowQty string `json:"showQty,omitempty"`
	// QtyPercent is the cumulative participation percentage (1-99) used by the
	// POV/INLINE strategies.
	QtyPercent string `json:"qtyPercent,omitempty"`
	// Interval is the child-order interval in seconds used by the POV/INLINE
	// strategies. Zero means the documented default (60).
	Interval int `json:"interval,omitempty"`
}

// AddOrderParams is the body of /trade/AlgoAddOrder. All codes and amounts are
// strings; see StrategyParam for the nested tuning object.
type AddOrderParams struct {
	// StockCode is the security code, for example "700.HK".
	StockCode string `json:"stockCode"`
	// ExchangeType is the market: K (HK), P (US), v (Shenzhen Connect), or t
	// (Shanghai Connect).
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// EntrustType is the algorithm order type (1 limit, 2 market).
	EntrustType EntrustType `json:"entrustType"`
	// EntrustPrice is the limit price as a decimal string.
	EntrustPrice string `json:"entrustPrice"`
	// EntrustAmount is the total quantity as a decimal string.
	EntrustAmount string `json:"entrustAmount"`
	// EntrustBS is the buy/sell direction reused from the trade dictionary
	// (1 buy, 2 sell).
	EntrustBS types.EntrustBS `json:"entrustBs"`
	// TargetStrategy selects the execution algorithm.
	TargetStrategy Strategy `json:"targetStrategy"`
	// SessionType enables pre-market/after-hours trading.
	SessionType SessionType `json:"sessionType"`
	// StrategyParam carries the algorithm-specific tuning.
	StrategyParam StrategyParam `json:"strategyParam"`
}

// validate rejects an AddOrder request that would be malformed at the Gateway.
func (p AddOrderParams) validate(op string) error {
	if p.StockCode == "" {
		return invalid(op, "stockCode is required")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	if p.EntrustType == "" {
		return invalid(op, "entrustType is required")
	}
	if !p.EntrustType.valid() {
		return invalid(op, fmt.Sprintf("entrustType %q is not one of 1 (limit), 2 (market)", p.EntrustType))
	}
	if p.EntrustPrice == "" {
		return invalid(op, "entrustPrice is required")
	}
	if !isPositiveDecimal(p.EntrustPrice) {
		return invalid(op, "entrustPrice must be a positive decimal string")
	}
	if p.EntrustAmount == "" {
		return invalid(op, "entrustAmount is required")
	}
	if !isPositiveDecimal(p.EntrustAmount) {
		return invalid(op, "entrustAmount must be a positive decimal string")
	}
	if p.EntrustBS == "" {
		return invalid(op, "entrustBs is required")
	}
	if p.TargetStrategy == "" {
		return invalid(op, "targetStrategy is required")
	}
	if p.SessionType == "" {
		return invalid(op, "sessionType is required")
	}
	if !p.SessionType.valid() {
		return invalid(op, fmt.Sprintf("sessionType %q is not one of 0 (off), 1 (on)", p.SessionType))
	}
	if p.StrategyParam.MaxVolume == "" {
		return invalid(op, "strategyParam.maxVolume is required")
	}
	if !isPositiveDecimal(p.StrategyParam.MaxVolume) {
		return invalid(op, "strategyParam.maxVolume must be a positive decimal string")
	}
	if p.StrategyParam.Sensitivity == "" {
		return invalid(op, "strategyParam.sensitivity is required")
	}
	if !p.StrategyParam.Sensitivity.valid() {
		return invalid(op, fmt.Sprintf("strategyParam.sensitivity %q is not one of 1 (neutral), 2 (aggressive), 3 (passive)", p.StrategyParam.Sensitivity))
	}
	return nil
}

// CancelOrderParams is the body of /trade/AlgoCancelOrder. It cancels a whole
// master order.
type CancelOrderParams struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// ExchangeType is the market the master trades on.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// validate rejects a CancelOrder request that would be malformed at the
// Gateway.
func (p CancelOrderParams) validate(op string) error {
	if p.OrderID == "" {
		return invalid(op, "orderId is required")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	return nil
}

// CancelEntrustParams is the body of /trade/AlgoCancelEntrust. It cancels one
// child order of a master.
type CancelEntrustParams struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// EntrustID is the child entrust identifier within OrderID.
	EntrustID string `json:"entrustId"`
	// ExchangeType is the market the master trades on.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// validate rejects a CancelEntrust request that would be malformed at the
// Gateway.
func (p CancelEntrustParams) validate(op string) error {
	if p.OrderID == "" {
		return invalid(op, "orderId is required")
	}
	if p.EntrustID == "" {
		return invalid(op, "entrustId is required")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	return nil
}

// ChangeOrderParams is the body of /trade/AlgoChangeOrder. It modifies a live
// master's price, quantity, and strategy tuning.
type ChangeOrderParams struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// StockCode must be the master's stock code; the code cannot be changed.
	StockCode string `json:"stockCode"`
	// ExchangeType is the market the master trades on.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// EntrustPrice is the new limit price as a decimal string.
	EntrustPrice string `json:"entrustPrice"`
	// EntrustAmount is the new total quantity as a decimal string.
	EntrustAmount string `json:"entrustAmount"`
	// StrategyParam carries the new algorithm-specific tuning. Unlike AddOrder,
	// every member is optional.
	StrategyParam StrategyParam `json:"strategyParam"`
}

// validate rejects a ChangeOrder request that would be malformed at the
// Gateway.
func (p ChangeOrderParams) validate(op string) error {
	if p.OrderID == "" {
		return invalid(op, "orderId is required")
	}
	if p.StockCode == "" {
		return invalid(op, "stockCode is required")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	if p.EntrustPrice == "" {
		return invalid(op, "entrustPrice is required")
	}
	if !isPositiveDecimal(p.EntrustPrice) {
		return invalid(op, "entrustPrice must be a positive decimal string")
	}
	if p.EntrustAmount == "" {
		return invalid(op, "entrustAmount is required")
	}
	if !isPositiveDecimal(p.EntrustAmount) {
		return invalid(op, "entrustAmount must be a positive decimal string")
	}
	if p.StrategyParam.Sensitivity != "" && !p.StrategyParam.Sensitivity.valid() {
		return invalid(op, fmt.Sprintf("strategyParam.sensitivity %q is not one of 1 (neutral), 2 (aggressive), 3 (passive)", p.StrategyParam.Sensitivity))
	}
	return nil
}

// ActionOrderParams is the body of /trade/AlgoActionOrder. It starts, stops,
// suspends, or resumes a master order.
type ActionOrderParams struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// Action is the operation; it must be one of the four documented codes.
	Action Action `json:"action"`
	// TargetStrategy is the strategy bound to the master. It is required for
	// every action.
	TargetStrategy Strategy `json:"targetStrategy"`
	// ExchangeType is the market the master trades on. It is required for every
	// action even though the reference request example omits it.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// validate rejects an ActionOrder request that would be malformed at the
// Gateway. Every documented action requires orderId, targetStrategy, and
// exchangeType; the action itself must be in the closed set.
func (p ActionOrderParams) validate(op string) error {
	if p.OrderID == "" {
		return invalid(op, "orderId is required")
	}
	if p.TargetStrategy == "" {
		return invalid(op, "targetStrategy is required")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	if !p.Action.Valid() {
		return invalid(op, fmt.Sprintf("action %q is not one of 1 (START), 2 (STOP), 3 (SUSPEND), 4 (RESUME)", p.Action))
	}
	return nil
}

// QueryOrderListParams is the body of /trade/AlgoQueryOrderList, a paged master
// order query over a date range.
type QueryOrderListParams struct {
	// PageNo is the one-based page number as a string.
	PageNo string `json:"pageNo"`
	// PageSize is the page size as a string.
	PageSize string `json:"pageSize"`
	// StartDate is the inclusive start date, yyyyMMdd.
	StartDate string `json:"startDate"`
	// EndDate is the inclusive end date, yyyyMMdd.
	EndDate string `json:"endDate"`
	// ExchangeType narrows the query to one market. It is required when
	// StockCode is set and optional otherwise.
	ExchangeType types.ExchangeType `json:"exchangeType,omitempty"`
	// StockCode narrows the query to one security.
	StockCode string `json:"stockCode,omitempty"`
}

// validate rejects a QueryOrderList request that would be malformed at the
// Gateway.
func (p QueryOrderListParams) validate(op string) error {
	if p.PageNo == "" {
		return invalid(op, "pageNo is required")
	}
	if !isPositiveInt(p.PageNo) {
		return invalid(op, "pageNo must be a positive integer string")
	}
	if p.PageSize == "" {
		return invalid(op, "pageSize is required")
	}
	if !isPositiveInt(p.PageSize) {
		return invalid(op, "pageSize must be a positive integer string")
	}
	if p.StartDate == "" {
		return invalid(op, "startDate is required")
	}
	if !isDate(p.StartDate) {
		return invalid(op, "startDate must have the form yyyyMMdd")
	}
	if p.EndDate == "" {
		return invalid(op, "endDate is required")
	}
	if !isDate(p.EndDate) {
		return invalid(op, "endDate must have the form yyyyMMdd")
	}
	if p.StockCode != "" && p.ExchangeType == "" {
		return invalid(op, "exchangeType is required when stockCode is set")
	}
	return nil
}

// QueryEntrustIDListParams is the body of /trade/AlgoQueryEntrustIdList. It
// lists the child entrust IDs of one master on one trade date.
type QueryEntrustIDListParams struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// TradeDate is the child orders' trade date, yyyyMMdd.
	TradeDate string `json:"tradeDate"`
	// ExchangeType is the market the master trades on.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// validate rejects a QueryEntrustIDList request that would be malformed at the
// Gateway.
func (p QueryEntrustIDListParams) validate(op string) error {
	if p.OrderID == "" {
		return invalid(op, "orderId is required")
	}
	if p.TradeDate == "" {
		return invalid(op, "tradeDate is required")
	}
	if !isDate(p.TradeDate) {
		return invalid(op, "tradeDate must have the form yyyyMMdd")
	}
	if p.ExchangeType == "" {
		return invalid(op, "exchangeType is required")
	}
	return nil
}

// MasterOrder is one master order in a QueryOrderList result. Every monetary or
// quantitative member is a string.
type MasterOrder struct {
	// OrderID is the master order identifier.
	OrderID string `json:"orderId"`
	// StockCode is the security code; a Hong Kong code carries a ".HK" suffix.
	StockCode string `json:"stockCode"`
	// ExchangeType is the market the order trades on.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// TradeDate is the order's trade date.
	TradeDate string `json:"tradeDate"`
	// EntrustType is the algorithm order type.
	EntrustType EntrustType `json:"entrustType"`
	// EntrustPrice is the limit price.
	EntrustPrice string `json:"entrustPrice"`
	// EntrustAmount is the total ordered quantity.
	EntrustAmount string `json:"entrustAmount"`
	// CumQty is the cumulative filled quantity.
	CumQty string `json:"cumQty"`
	// LeavesQty is the remaining unfilled quantity.
	LeavesQty string `json:"leavesQty"`
	// Status is the master's lifecycle state.
	Status Status `json:"status"`
	// EntrustBS is the buy/sell direction.
	EntrustBS types.EntrustBS `json:"entrustBs"`
	// TargetStrategy is the bound execution algorithm.
	TargetStrategy Strategy `json:"targetStrategy"`
	// StrategyStatus is the strategy's run state.
	StrategyStatus StrategyStatus `json:"strategyStatus"`
	// StrategyParam is the tuning the master is running with.
	StrategyParam StrategyParam `json:"strategyParam"`
	// RoundLot is the security's board lot size.
	RoundLot string `json:"roundLot"`
	// SendingTime is the transport timestamp.
	SendingTime string `json:"sendingTime"`
	// TransactionTime is the order timestamp.
	TransactionTime string `json:"transactionTime"`
	// AvgPx is the average execution price.
	AvgPx string `json:"avgPx"`
}

// orderIDResponse is the shared {"data":"<id>"} body returned by the add,
// cancel, change, and action mutations.
type orderIDResponse struct {
	// Data is the master or child identifier echoed by the Gateway.
	Data string `json:"data"`
}

// masterOrderListResponse is the {"algoOrderList":[...]} body returned by
// /trade/AlgoQueryOrderList.
type masterOrderListResponse struct {
	// AlgoOrderList is the page of master orders.
	AlgoOrderList []MasterOrder `json:"algoOrderList"`
}

// entrustIDListResponse is the {"entrustId":[...]} body returned by
// /trade/AlgoQueryEntrustIdList.
type entrustIDListResponse struct {
	// EntrustID is the child entrust identifiers of one master.
	EntrustID []string `json:"entrustId"`
}

// Option configures a Manager. Options are applied in order on top of the
// defaults inside New; the last option that sets a field wins.
type Option func(*config)

// config holds the resolved Manager settings.
type config struct {
	defaultExchangeType types.ExchangeType
}

// WithDefaultExchangeType sets the market applied when a request leaves its
// ExchangeType empty. Without it, every call whose endpoint requires a market
// must supply one and an empty value is rejected. An empty value removes any
// previously configured default.
func WithDefaultExchangeType(exchange types.ExchangeType) Option {
	return func(c *config) { c.defaultExchangeType = exchange }
}

// Manager wraps a *client.Client with typed methods for the seven
// algorithm-trading endpoints. It holds no per-request state and is safe for
// concurrent use.
//
// Mutations (AddOrder, CancelOrder, CancelEntrust, ChangeOrder, ActionOrder)
// issue exactly one HTTP attempt and are never retried (ADR 0003). Queries
// (QueryOrderList, QueryEntrustIDList) are read-only.
type Manager struct {
	client *client.Client
	cfg    config
}

// New returns a Manager over c with opts applied in order on top of the
// defaults. c must be non-nil and is not owned by the Manager: closing it is
// the caller's responsibility. The returned Manager is safe for concurrent
// use.
func New(c *client.Client, opts ...Option) *Manager {
	cfg := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Manager{client: c, cfg: cfg}
}

// exchangeType returns e when non-empty, otherwise the configured default.
func (m *Manager) exchangeType(e types.ExchangeType) types.ExchangeType {
	if e != "" {
		return e
	}
	return m.cfg.defaultExchangeType
}

// AddOrder places an algorithm master order and returns its order ID.
//
// It validates params before sending anything; an empty required field, a
// malformed price/amount, or an out-of-set code returns an error wrapping
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
//
// AddOrder is a mutation: it issues exactly one attempt and is never retried
// (ADR 0003). An ambiguous failure must be reconciled by the caller before
// resubmitting.
func (m *Manager) AddOrder(ctx context.Context, params AddOrderParams) (string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opAddOrder); err != nil {
		return "", err
	}
	var resp orderIDResponse
	if err := m.client.Do(ctx, opAddOrder, client.RouteTradeAlgoAddOrder, params, m.client.JSON(), &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CancelOrder cancels a whole master order and returns its order ID.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
//
// CancelOrder is a mutation: it issues exactly one attempt and is never
// retried (ADR 0003).
func (m *Manager) CancelOrder(ctx context.Context, params CancelOrderParams) (string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opCancelOrder); err != nil {
		return "", err
	}
	var resp orderIDResponse
	if err := m.client.Do(ctx, opCancelOrder, client.RouteTradeAlgoCancelOrder, params, m.client.JSON(), &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CancelEntrust cancels one child order of a master and returns the child's
// entrust ID.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
//
// CancelEntrust is a mutation: it issues exactly one attempt and is never
// retried (ADR 0003).
func (m *Manager) CancelEntrust(ctx context.Context, params CancelEntrustParams) (string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opCancelEntrust); err != nil {
		return "", err
	}
	var resp orderIDResponse
	if err := m.client.Do(ctx, opCancelEntrust, client.RouteTradeAlgoCancelEntrust, params, m.client.JSON(), &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// ChangeOrder modifies a live master's price, quantity, and strategy tuning and
// returns its order ID.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
//
// ChangeOrder is a mutation: it issues exactly one attempt and is never
// retried (ADR 0003).
func (m *Manager) ChangeOrder(ctx context.Context, params ChangeOrderParams) (string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opChangeOrder); err != nil {
		return "", err
	}
	var resp orderIDResponse
	if err := m.client.Do(ctx, opChangeOrder, client.RouteTradeAlgoChangeOrder, params, m.client.JSON(), &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// ActionOrder starts, stops, suspends, or resumes a master order and returns
// its order ID. The action must be one of the four documented Action codes.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
//
// ActionOrder is a mutation: it issues exactly one attempt and is never
// retried (ADR 0003).
func (m *Manager) ActionOrder(ctx context.Context, params ActionOrderParams) (string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opActionOrder); err != nil {
		return "", err
	}
	var resp orderIDResponse
	if err := m.client.Do(ctx, opActionOrder, client.RouteTradeAlgoActionOrder, params, m.client.JSON(), &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// QueryOrderList returns the page of master orders matching params. It is a
// read-only query and is retryable by the resilience layer.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. Unlike the mutations,
// QueryOrderList does not apply a default exchange type because ExchangeType is
// optional here.
func (m *Manager) QueryOrderList(ctx context.Context, params QueryOrderListParams) ([]MasterOrder, error) {
	if err := params.validate(opQueryOrderList); err != nil {
		return nil, err
	}
	var resp masterOrderListResponse
	if err := m.client.Do(ctx, opQueryOrderList, client.RouteTradeAlgoQueryOrderList, params, m.client.JSON(), &resp); err != nil {
		return nil, err
	}
	return resp.AlgoOrderList, nil
}

// QueryEntrustIDList returns the child entrust IDs of one master on one trade
// date. It is a read-only query and is retryable by the resilience layer.
//
// It validates params before sending anything; a failure wraps
// ErrInvalidParams and no request is made. A configured
// WithDefaultExchangeType fills an empty ExchangeType first.
func (m *Manager) QueryEntrustIDList(ctx context.Context, params QueryEntrustIDListParams) ([]string, error) {
	params.ExchangeType = m.exchangeType(params.ExchangeType)
	if err := params.validate(opQueryEntrustIDList); err != nil {
		return nil, err
	}
	var resp entrustIDListResponse
	if err := m.client.Do(ctx, opQueryEntrustIDList, client.RouteTradeAlgoQueryEntrustIdList, params, m.client.JSON(), &resp); err != nil {
		return nil, err
	}
	return resp.EntrustID, nil
}

// invalid builds the ErrInvalidParams-wrapped validation error for op.
func invalid(op, message string) error {
	return errs.Wrap(ErrInvalidParams, "", op, message)
}

// isPositiveInt reports whether s is a non-empty run of ASCII digits with at
// least one non-zero digit.
func isPositiveInt(s string) bool {
	if s == "" {
		return false
	}
	nonzero := false
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		if s[i] != '0' {
			nonzero = true
		}
	}
	return nonzero
}

// isDate reports whether s is exactly eight ASCII digits (yyyyMMdd). It does
// not validate the calendar value; the Gateway does that.
func isDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isPositiveDecimal reports whether s is a plain decimal number (optionally
// with a fractional part) greater than zero. It uses no floating point, so it
// can never lose precision or accept exponent notation, and it rejects a bare
// ".", "1.", ".5", and "0".
func isPositiveDecimal(s string) bool {
	if s == "" {
		return false
	}
	intDigits := 0
	fracDigits := 0
	dot := false
	nonzero := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			if dot {
				fracDigits++
			} else {
				intDigits++
			}
			if c != '0' {
				nonzero = true
			}
		case c == '.':
			if dot {
				return false
			}
			dot = true
		default:
			return false
		}
	}
	if intDigits == 0 || (dot && fracDigits == 0) {
		return false
	}
	return nonzero
}
