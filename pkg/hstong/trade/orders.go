// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"math/big"
	"strconv"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels used in errors raised by Manager. Each matches the Gateway
// path so a log line or error points at the failing endpoint.
const (
	opEntrust               = "trade/TradeEntrust"
	opCancelEntrust         = "trade/TradeCancelEntrust"
	opBatchCancelEntrust    = "trade/TradeBatchCancelEntrust"
	opChangeEntrust         = "trade/TradeChangeEntrust"
	opMaxAvailableAsset     = "trade/TradeQueryMaxAvailableAsset"
	opRealEntrustList       = "trade/TradeQueryRealEntrustList"
	opRealDeliverList       = "trade/TradeQueryRealDeliverList"
	opRealCondOrderList     = "trade/TradeQueryRealCondOrderList"
	opHistoryEntrustList    = "trade/TradeQueryHistoryEntrustList"
	opHistoryDeliverList    = "trade/TradeQueryHistoryDeliverList"
	opHistoryCondOrderList  = "trade/TradeQueryHistoryCondOrderList"
	opMarginFullInfo        = "trade/TradeQueryMarginFullInfo"
	opBeforeAndAfterSupport = "trade/TradeQueryBeforeAndAfterSupport"
)

// maxValidDays is the vendor's upper bound for a conditional order's lifetime,
// in natural days.
const maxValidDays = 100

// CommonStringResponse is a Gateway body whose data field is a single string,
// for example an entrust identifier or the before/after support flag.
type CommonStringResponse struct {
	// Data is the response value.
	Data string `json:"data"`
}

// CommonIntResponse is a Gateway body whose data field is a single integer.
type CommonIntResponse struct {
	// Data is the response value.
	Data int `json:"data"`
}

// EntrustRequest is the body of /trade/TradeEntrust.
//
// EntrustPrice may be empty only for a market order or a conditional order; for
// every other type it is required. IceBergDisplaySize is required for an
// iceberg order; ValidDays and CondValue are required for a conditional order.
type EntrustRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode is the security code, for example "01810.HK". It is required.
	StockCode string `json:"stockCode"`
	// EntrustAmount is the order quantity as a decimal string. It must be
	// greater than zero.
	EntrustAmount string `json:"entrustAmount"`
	// EntrustPrice is the order price as a decimal string. It is required
	// unless the order type is a market or conditional type.
	EntrustPrice string `json:"entrustPrice,omitempty"`
	// EntrustBS is the buy/sell direction. It is required.
	EntrustBS types.EntrustBS `json:"entrustBs"`
	// EntrustType is the order type. It is required.
	EntrustType types.EntrustType `json:"entrustType"`
	// ClientType is the client type, for example 0 for internet. It is
	// optional.
	ClientType int `json:"clientType,omitempty"`
	// Exchange is the US direct-access exchange. Deprecated: the field is
	// documented as no longer used.
	Exchange string `json:"exchange,omitempty"`
	// SessionType selects pre-market/post-market trading: 0 or 1 for a normal
	// order, 3, 5, or 7 for a conditional order. It is optional.
	SessionType string `json:"sessionType,omitempty"`
	// IceBergDisplaySize is the disclosed quantity of an iceberg order. It must
	// be greater than zero and no more than EntrustAmount.
	IceBergDisplaySize string `json:"iceBergDisplaySize,omitempty"`
	// ValidDays is the conditional order lifetime in natural days. It must be
	// an integer in [1, 100].
	ValidDays string `json:"validDays,omitempty"`
	// CondValue is the trigger value of a conditional order: a price, a spread,
	// or a percentage.
	CondValue string `json:"condValue,omitempty"`
	// CondTrackType is the tracking type: "1" percentage or "2" spread. It is
	// required for a trailing conditional order.
	CondTrackType string `json:"condTrackType,omitempty"`
}

// Entrust places an order and returns the entrust identifier.
//
// Entrust issues exactly one HTTP request and is never retried: an ambiguous
// failure must be reconciled by querying the real/history entrust and deliver
// lists before resubmitting (docs/adr/0003-no-auto-retry-orders.md).
func (m *Manager) Entrust(ctx context.Context, req EntrustRequest) (string, error) {
	if err := req.validate(); err != nil {
		return "", err
	}
	var out CommonStringResponse
	if err := m.call(ctx, opEntrust, client.RouteTradeEntrust, req, &out); err != nil {
		return "", err
	}
	return out.Data, nil
}

// validate enforces the documented Entrust constraints.
func (r EntrustRequest) validate() error {
	if r.ExchangeType == "" {
		return invalid(opEntrust, "exchangeType is required")
	}
	if r.StockCode == "" {
		return invalid(opEntrust, "stockCode is required")
	}
	amount, ok := parsePositive(r.EntrustAmount)
	if !ok {
		return invalid(opEntrust, "entrustAmount must be a decimal number greater than zero")
	}
	if r.EntrustBS == "" {
		return invalid(opEntrust, "entrustBs is required")
	}
	if r.EntrustType == "" {
		return invalid(opEntrust, "entrustType is required")
	}
	if !priceOptional(r.EntrustType) && r.EntrustPrice == "" {
		return invalid(opEntrust, "entrustPrice is required for this order type")
	}
	if isConditional(r.EntrustType) {
		if r.CondValue == "" {
			return invalid(opEntrust, "condValue is required for a conditional order")
		}
		if err := validateValidDays(opEntrust, r.ValidDays); err != nil {
			return err
		}
	}
	if isIceberg(r.EntrustType) {
		display, ok := parsePositive(r.IceBergDisplaySize)
		if !ok {
			return invalid(opEntrust, "iceBergDisplaySize must be a decimal number greater than zero for an iceberg order")
		}
		if display.Cmp(amount) > 0 {
			return invalid(opEntrust, "iceBergDisplaySize must not exceed entrustAmount")
		}
	}
	return nil
}

// CancelEntrustRequest is the body of /trade/TradeCancelEntrust.
type CancelEntrustRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode is the security code. It is required.
	StockCode string `json:"stockCode"`
	// EntrustAmount is the original order quantity, as a decimal string. It is
	// required by the Gateway to locate the order.
	EntrustAmount string `json:"entrustAmount,omitempty"`
	// EntrustPrice is the original order price. It is optional.
	EntrustPrice string `json:"entrustPrice,omitempty"`
	// EntrustID is the identifier to cancel. It is required.
	EntrustID string `json:"entrustId"`
	// EntrustType is the order type. It is required for a conditional order and
	// must match the original.
	EntrustType types.EntrustType `json:"entrustType,omitempty"`
}

// CancelEntrust cancels one order and returns the entrust identifier.
//
// CancelEntrust issues exactly one HTTP request and is never retried
// (docs/adr/0003-no-auto-retry-orders.md).
func (m *Manager) CancelEntrust(ctx context.Context, req CancelEntrustRequest) (string, error) {
	if err := req.validate(); err != nil {
		return "", err
	}
	var out CommonStringResponse
	if err := m.call(ctx, opCancelEntrust, client.RouteTradeCancelEntrust, req, &out); err != nil {
		return "", err
	}
	return out.Data, nil
}

// validate enforces the documented CancelEntrust constraints.
func (r CancelEntrustRequest) validate() error {
	if r.ExchangeType == "" {
		return invalid(opCancelEntrust, "exchangeType is required")
	}
	if r.StockCode == "" {
		return invalid(opCancelEntrust, "stockCode is required")
	}
	if r.EntrustID == "" {
		return invalid(opCancelEntrust, "entrustId is required")
	}
	return nil
}

// BatchCancelEntrustRequest is the body of /trade/TradeBatchCancelEntrust. When
// EntrustIDs is empty the Gateway cancels every cancellable order in the market.
type BatchCancelEntrustRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// EntrustIDs is the optional list of order identifiers to cancel. It does
	// not include conditional orders. When empty, every order in the market is
	// cancelled.
	EntrustIDs []string `json:"entrustId,omitempty"`
}

// FailCancelEntrustVo is one failed entry of a batch cancel.
type FailCancelEntrustVo struct {
	// FailEntrustID is the identifier that could not be cancelled.
	FailEntrustID string `json:"failEntrustId"`
	// Remark is the failure reason.
	Remark string `json:"remark"`
}

// BatchCancelEntrustResult is the body of /trade/TradeBatchCancelEntrust.
type BatchCancelEntrustResult struct {
	// SuccessEntrustID lists the identifiers that were cancelled.
	SuccessEntrustID []string `json:"successEntrustId"`
	// FailCancelEntrust lists the identifiers that could not be cancelled.
	FailCancelEntrust []FailCancelEntrustVo `json:"failCancelEntrust"`
}

// BatchCancelEntrust cancels a set of orders (or every order in a market) and
// returns the per-order outcome. The documented caveat is that a batch cancel
// can time out when the day's order volume is high.
//
// BatchCancelEntrust issues exactly one HTTP request and is never retried: an
// ambiguous failure must be reconciled with a real entrust query before
// resubmitting (docs/adr/0003-no-auto-retry-orders.md).
func (m *Manager) BatchCancelEntrust(ctx context.Context, req BatchCancelEntrustRequest) (*BatchCancelEntrustResult, error) {
	if req.ExchangeType == "" {
		return nil, invalid(opBatchCancelEntrust, "exchangeType is required")
	}
	var out BatchCancelEntrustResult
	if err := m.call(ctx, opBatchCancelEntrust, client.RouteTradeBatchCancelEntrust, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ChangeEntrustRequest is the body of /trade/TradeChangeEntrust.
type ChangeEntrustRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode is the security code. It is required.
	StockCode string `json:"stockCode"`
	// EntrustAmount is the revised quantity as a decimal string. It must be
	// greater than zero.
	EntrustAmount string `json:"entrustAmount"`
	// EntrustPrice is the revised price. It may be empty only for a conditional
	// order.
	EntrustPrice string `json:"entrustPrice,omitempty"`
	// EntrustID is the original order identifier. It is required.
	EntrustID string `json:"entrustId"`
	// EntrustType is the order type. It is required for a conditional order and
	// must match the original.
	EntrustType types.EntrustType `json:"entrustType,omitempty"`
	// SessionType selects pre-market/post-market trading. It is optional.
	SessionType string `json:"sessionType,omitempty"`
	// ValidDays is the conditional order lifetime in natural days, in [1, 100].
	ValidDays string `json:"validDays,omitempty"`
	// CondValue is the trigger value of a conditional order.
	CondValue string `json:"condValue,omitempty"`
	// CondTrackType is the tracking type: "1" percentage or "2" spread.
	CondTrackType string `json:"condTrackType,omitempty"`
}

// ChangeEntrust modifies an order and returns the entrust identifier.
//
// ChangeEntrust issues exactly one HTTP request and is never retried
// (docs/adr/0003-no-auto-retry-orders.md).
func (m *Manager) ChangeEntrust(ctx context.Context, req ChangeEntrustRequest) (string, error) {
	if err := req.validate(); err != nil {
		return "", err
	}
	var out CommonStringResponse
	if err := m.call(ctx, opChangeEntrust, client.RouteTradeChangeEntrust, req, &out); err != nil {
		return "", err
	}
	return out.Data, nil
}

// validate enforces the documented ChangeEntrust constraints.
func (r ChangeEntrustRequest) validate() error {
	if r.ExchangeType == "" {
		return invalid(opChangeEntrust, "exchangeType is required")
	}
	if r.StockCode == "" {
		return invalid(opChangeEntrust, "stockCode is required")
	}
	if _, ok := parsePositive(r.EntrustAmount); !ok {
		return invalid(opChangeEntrust, "entrustAmount must be a decimal number greater than zero")
	}
	if r.EntrustID == "" {
		return invalid(opChangeEntrust, "entrustId is required")
	}
	if isConditional(r.EntrustType) {
		if r.CondValue == "" {
			return invalid(opChangeEntrust, "condValue is required for a conditional order")
		}
		if err := validateValidDays(opChangeEntrust, r.ValidDays); err != nil {
			return err
		}
	}
	return nil
}

// MaxAvailableAssetRequest is the body of /trade/TradeQueryMaxAvailableAsset.
type MaxAvailableAssetRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode is the security code. It is required.
	StockCode string `json:"stockCode"`
	// EntrustPrice is the order price. It is required.
	EntrustPrice string `json:"entrustPrice"`
	// EntrustType is the order type. It is required.
	EntrustType types.EntrustType `json:"entrustType"`
}

// MaxAvailableAsset is the maximum-buy/sell and option-margin snapshot returned
// by /trade/TradeQueryMaxAvailableAsset. Every numeric value is a decimal
// string.
type MaxAvailableAsset struct {
	// PositionStatus is the position status: 0 flat, 1 long, 2 short.
	PositionStatus string `json:"positionStatus"`
	// Position is the current position quantity.
	Position string `json:"position"`
	// LongOpenAvailable is the long open (buy) quantity available.
	LongOpenAvailable string `json:"longOpenAvailable"`
	// LongCloseAvailable is the long close (sell) quantity available.
	LongCloseAvailable string `json:"longCloseAvailable"`
	// CashAvailableToOpen is the long open quantity available with cash.
	CashAvailableToOpen string `json:"cashAvailableToOpen"`
	// MarginAvailableToOpen is the long open quantity available with margin.
	MarginAvailableToOpen string `json:"marginAvailableToOpen"`
	// CashAndMarginAvailableToOpen is the long open quantity available with
	// cash plus margin.
	CashAndMarginAvailableToOpen string `json:"cashAndMarginAvailableToOpen"`
	// CashAvailableAmount is the cash purchasing power.
	CashAvailableAmount string `json:"cashAvailableAmount"`
	// MarginAvailableAmount is the margin purchasing power.
	MarginAvailableAmount string `json:"marginAvailableAmount"`
	// CashAndMarginAvailableAmount is the combined cash-plus-margin purchasing
	// power.
	CashAndMarginAvailableAmount string `json:"cashAndMarginAvailableAmount"`
	// ShortOpenAvailable is the short open (sell) quantity available.
	ShortOpenAvailable string `json:"shortOpenAvailable"`
	// ShortCloseAvailable is the short close (buy) quantity available.
	ShortCloseAvailable string `json:"shortCloseAvailable"`
	// ShortCloseCashAvailable is the short close (buy) quantity available with
	// cash.
	ShortCloseCashAvailable string `json:"shortCloseCashAvailable"`
	// ShortOpenPool is the short-selling pool.
	ShortOpenPool string `json:"shortOpenPool"`
	// ShortBuyPower is the short-selling purchasing power.
	ShortBuyPower string `json:"shortBuyPower"`
	// ContractSize is the option contract multiplier.
	ContractSize string `json:"contractSize"`
	// UnitedBuyPowerStatus is the unified-purchasing-power status.
	UnitedBuyPowerStatus string `json:"unitedBuyPowerStatus"`
	// CreditLimit is the overdraft limit.
	CreditLimit string `json:"creditLimit"`
	// OptionLongMarginAmount is the option margin for a long position.
	OptionLongMarginAmount string `json:"optionLongMarginAmount"`
	// OptionShortMarginAmount is the option margin for a short position.
	OptionShortMarginAmount string `json:"optionShortMarginAmount"`
}

// maxAvailableAssetResponse wraps the single MaxAvailableAsset payload under the
// envelope's data.data path.
type maxAvailableAssetResponse struct {
	// Data is the maximum-available asset snapshot.
	Data MaxAvailableAsset `json:"data"`
}

// MaxAvailableAsset returns the maximum buy/sell quantity and option margin for
// the requested instrument and order type. This is a read: it may be retried by
// a caller, but Manager itself never retries.
func (m *Manager) MaxAvailableAsset(ctx context.Context, req MaxAvailableAssetRequest) (*MaxAvailableAsset, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var out maxAvailableAssetResponse
	if err := m.call(ctx, opMaxAvailableAsset, client.RouteTradeQueryMaxAvailableAsset, req, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// validate enforces the documented MaxAvailableAsset constraints.
func (r MaxAvailableAssetRequest) validate() error {
	if r.ExchangeType == "" {
		return invalid(opMaxAvailableAsset, "exchangeType is required")
	}
	if r.StockCode == "" {
		return invalid(opMaxAvailableAsset, "stockCode is required")
	}
	if r.EntrustPrice == "" {
		return invalid(opMaxAvailableAsset, "entrustPrice is required")
	}
	if r.EntrustType == "" {
		return invalid(opMaxAvailableAsset, "entrustType is required")
	}
	return nil
}

// RealEntrustListRequest is the body of /trade/TradeQueryRealEntrustList. When
// EntrustIDs is non-empty the pagination fields are ignored by the Gateway.
type RealEntrustListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// QueryCount is the page size.
	QueryCount int `json:"queryCount"`
	// QueryParamStr is the cursor.
	QueryParamStr string `json:"queryParamStr"`
	// EntrustIDs optionally restricts the query to these identifiers (at most
	// 100). It is optional.
	EntrustIDs []string `json:"entrustId,omitempty"`
}

// RealDeliverListRequest is the body of /trade/TradeQueryRealDeliverList.
type RealDeliverListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// QueryCount is the page size.
	QueryCount int `json:"queryCount"`
	// QueryParamStr is the cursor.
	QueryParamStr string `json:"queryParamStr"`
}

// OrderVo is one entrust or deliver row shared by the real and historical order
// queries.
type OrderVo struct {
	// StockCode is the security code.
	StockCode string `json:"stockCode"`
	// StockName is the security name.
	StockName string `json:"stockName"`
	// BusinessPrice is the execution price.
	BusinessPrice string `json:"businessPrice"`
	// EntrustBS is the order direction.
	EntrustBS types.EntrustBS `json:"entrustBs"`
	// EntrustPrice is the order price.
	EntrustPrice string `json:"entrustPrice"`
	// BusinessBalance is the executed amount.
	BusinessBalance string `json:"businessBalance"`
	// EntrustAmount is the order quantity.
	EntrustAmount string `json:"entrustAmount"`
	// BusinessAmount is the executed quantity.
	BusinessAmount string `json:"businessAmount"`
	// Date is the execution date for a deliver row and the order date for an
	// entrust row.
	Date string `json:"date"`
	// BusinessTime is the execution time.
	BusinessTime string `json:"businessTime"`
	// EntrustTime is the order time.
	EntrustTime string `json:"entrustTime"`
	// QueryParamStr is the cursor for the next page.
	QueryParamStr string `json:"queryParamStr"`
	// StatusDesc is the localized order-status description.
	StatusDesc string `json:"statusDesc"`
	// Status is the order-status code.
	Status types.EntrustStatus `json:"status"`
	// EntrustID is the order identifier.
	EntrustID string `json:"entrustId"`
	// UnBusinessAmount is the unfilled quantity.
	UnBusinessAmount string `json:"unBusinessAmount"`
	// CanBeCanceled is 1 when the order can be cancelled, 0 otherwise.
	CanBeCanceled int32 `json:"canBeCanceled"`
	// EntrustType is the order type.
	EntrustType types.EntrustType `json:"entrustType"`
	// OpponentSeat is the counterparty seat; present for intraday delivers only.
	OpponentSeat string `json:"opponentSeat"`
	// EntrustTypeNum is the numeric order type.
	EntrustTypeNum string `json:"entrustTypeNum"`
	// RemarkType is the remark type: 0 none, 1 rejected.
	RemarkType string `json:"remarkType"`
	// Remark is the free-text remark.
	Remark string `json:"remark"`
	// FareVo carries the fees; it may be nil for an intraday deliver that has
	// not been settled yet.
	FareVo *FareVo `json:"fareVo"`
	// ExchangeType is the market.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// CanBeUpdated is 1 when the order can be modified, 0 otherwise.
	CanBeUpdated int32 `json:"canBeUpdated"`
	// Exchange is the exchange.
	Exchange string `json:"exchange"`
}

// FareVo is the itemized fee breakdown of a deliver row.
type FareVo struct {
	// Fare0 is commission.
	Fare0 string `json:"fare0"`
	// Fare1 is, for Hong Kong, stamp duty; for the US, the SEC fee.
	Fare1 string `json:"fare1"`
	// Fare2 is, for Hong Kong, the trading fee; for the US, the trading
	// activity fee.
	Fare2 string `json:"fare2"`
	// Fare3 is the transaction levy.
	Fare3 string `json:"fare3"`
	// Fare4 is the US option regulatory fee.
	Fare4 string `json:"fare4"`
	// Fare5 is the US option clearing fee.
	Fare5 string `json:"fare5"`
	// Fare6 is the platform fee.
	Fare6 string `json:"fare6"`
	// Fare7 is reserved by the vendor.
	Fare7 string `json:"fare7"`
	// Fare8 is the US consolidated audit trail regulatory fee.
	Fare8 string `json:"fare8"`
	// Fare9 is, for Hong Kong, the AFRC transaction levy.
	Fare9 string `json:"fare9"`
	// FareX is, for Hong Kong, the settlement fee; for the US, the settlement
	// fee.
	FareX string `json:"farex"`
	// FareT is the total trading fee.
	FareT string `json:"faret"`
}

// OrderListResponse is the decoded body shared by the real and historical
// entrust and deliver queries: a data array of rows.
type OrderListResponse struct {
	// Data is every row in the response.
	Data []OrderVo `json:"data"`
}

// RealEntrustList returns one page of the day's entrusts. Use Paginate to walk
// every page.
func (m *Manager) RealEntrustList(ctx context.Context, req RealEntrustListRequest) ([]OrderVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out OrderListResponse
	if err := m.call(ctx, opRealEntrustList, client.RouteTradeQueryRealEntrustList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// RealDeliverList returns one page of the day's delivers (成交). Use Paginate to
// walk every page.
func (m *Manager) RealDeliverList(ctx context.Context, req RealDeliverListRequest) ([]OrderVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out OrderListResponse
	if err := m.call(ctx, opRealDeliverList, client.RouteTradeQueryRealDeliverList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// CondOrderListRequest is the body of /trade/TradeQueryRealCondOrderList.
type CondOrderListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode optionally restricts the query to one security.
	StockCode string `json:"stockCode,omitempty"`
	// PageNo is the 1-based page number. A non-positive value becomes 1.
	PageNo int `json:"pageNo"`
	// PageSize is the page size, clamped to [1, MaxPageSize].
	PageSize int `json:"pageSize"`
}

// CondOrderVo is one conditional order.
type CondOrderVo struct {
	// CondOrderID is the condition-order identifier.
	CondOrderID string `json:"condOrderId"`
	// DataType is the numeric instrument type.
	DataType string `json:"dataType"`
	// StockCode is the security code.
	StockCode string `json:"stockCode"`
	// StockName is the security name.
	StockName string `json:"stockName"`
	// StockNameTc is the traditional-Chinese security name.
	StockNameTc string `json:"stockNameTc"`
	// StockNameEn is the English security name.
	StockNameEn string `json:"stockNameEn"`
	// ExchangeType is the market.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// EntrustType is the conditional order type (31 through 36).
	EntrustType types.EntrustType `json:"entrustType"`
	// SessionType selects the trading session.
	SessionType string `json:"sessionType"`
	// Status is the condition-order status. Its values differ from the entrust
	// status codes (1 pending trigger, 2 triggered, 3 paused, 4 expired,
	// 5 deleted, 6 error, 8 stop invalidated, 9 ex-rights invalidated), so it is
	// kept as a string rather than types.EntrustStatus.
	Status string `json:"status"`
	// CanBeCancel is "1" when the order can be cancelled, "0" otherwise.
	CanBeCancel string `json:"canBeCancel"`
	// CanBeModify is "1" when the order can be modified, "0" otherwise.
	CanBeModify string `json:"canBeModify"`
	// EntrustBS is the order direction.
	EntrustBS types.EntrustBS `json:"entrustBs"`
	// EntrustAmount is the order quantity.
	EntrustAmount string `json:"entrustAmount"`
	// CreateTime is the order creation time.
	CreateTime string `json:"createTime"`
	// StartTime is the order activation time.
	StartTime string `json:"startTime"`
	// EndTime is the order expiry time.
	EndTime string `json:"endTime"`
	// ErrorCode is the error code, when the status is error.
	ErrorCode string `json:"errorCode"`
	// ErrorMsg is the error message, when the status is error.
	ErrorMsg string `json:"errorMsg"`
	// CondValue is the trigger value: price, spread, or percentage.
	CondValue string `json:"condValue"`
	// CondPrice is the trigger reference price.
	CondPrice string `json:"condPrice"`
	// CondTrackType is the tracking type: 1 percentage, 2 spread, 3 price.
	CondTrackType string `json:"condTrackType"`
}

// CondOrderPage is the body of the conditional-order queries: one page of rows
// plus the paging metadata.
type CondOrderPage struct {
	// Data is every row in this page.
	Data []CondOrderVo `json:"data"`
	// CurPageNo is the 1-based page number.
	CurPageNo int32 `json:"curPageNo"`
	// CurPageSize is the number of rows in this page.
	CurPageSize int32 `json:"curPageSize"`
	// TotalPages is the total number of pages.
	TotalPages int64 `json:"totalPages"`
}

// RealCondOrderList returns one page of the day's conditional orders.
func (m *Manager) RealCondOrderList(ctx context.Context, req CondOrderListRequest) (*CondOrderPage, error) {
	if req.ExchangeType == "" {
		return nil, invalid(opRealCondOrderList, "exchangeType is required")
	}
	if req.PageNo <= 0 {
		req.PageNo = 1
	}
	req.PageSize = ClampPageSize(req.PageSize)
	var out CondOrderPage
	if err := m.call(ctx, opRealCondOrderList, client.RouteTradeQueryRealCondOrderList, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HistoryEntrustListRequest is the body of /trade/TradeQueryHistoryEntrustList.
type HistoryEntrustListRequest struct {
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

// HistoryDeliverListRequest is the body of /trade/TradeQueryHistoryDeliverList.
type HistoryDeliverListRequest struct {
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

// HistoryCondOrderListRequest is the body of
// /trade/TradeQueryHistoryCondOrderList. Unlike the real conditional-order
// query, the documented wire type of PageNo and PageSize here is string.
type HistoryCondOrderListRequest struct {
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
	// StockCode optionally restricts the query to one security.
	StockCode string `json:"stockCode,omitempty"`
	// PageNo is the 1-based page number as a string. An empty value becomes "1".
	PageNo string `json:"pageNo"`
	// PageSize is the page size as a string. An empty value becomes "20".
	PageSize string `json:"pageSize"`
	// StartTime is the inclusive start time, "yyyy-MM-dd HH:mm:ss".
	StartTime string `json:"startTime"`
	// EndTime is the inclusive end time, "yyyy-MM-dd HH:mm:ss".
	EndTime string `json:"endTime"`
}

// HistoryEntrustList returns one page of the historical entrusts between
// StartDate and EndDate. Use Paginate to walk every page.
func (m *Manager) HistoryEntrustList(ctx context.Context, req HistoryEntrustListRequest) ([]OrderVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out OrderListResponse
	if err := m.call(ctx, opHistoryEntrustList, client.RouteTradeQueryHistoryEntrustList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// HistoryDeliverList returns one page of the historical delivers between
// StartDate and EndDate. Use Paginate to walk every page.
func (m *Manager) HistoryDeliverList(ctx context.Context, req HistoryDeliverListRequest) ([]OrderVo, error) {
	req.QueryCount = ClampPageSize(req.QueryCount)
	var out OrderListResponse
	if err := m.call(ctx, opHistoryDeliverList, client.RouteTradeQueryHistoryDeliverList, req, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// HistoryCondOrderList returns one page of the historical conditional orders
// between StartTime and EndTime.
func (m *Manager) HistoryCondOrderList(ctx context.Context, req HistoryCondOrderListRequest) (*CondOrderPage, error) {
	if req.ExchangeType == "" {
		return nil, invalid(opHistoryCondOrderList, "exchangeType is required")
	}
	if req.PageNo == "" {
		req.PageNo = "1"
	}
	if req.PageSize == "" {
		req.PageSize = strconv.Itoa(DefaultPageSize)
	}
	var out CondOrderPage
	if err := m.call(ctx, opHistoryCondOrderList, client.RouteTradeQueryHistoryCondOrderList, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MarginFullInfoRequest is the body of /trade/TradeQueryMarginFullInfo.
type MarginFullInfoRequest struct {
	// DataType is the numeric instrument type from the data dictionary, as a
	// decimal string, for example "10000" for a Hong Kong stock.
	DataType string `json:"dataType"`
	// StockCode is the security code. It is required.
	StockCode string `json:"stockCode"`
}

// RateVo is one currency's margin interest rate.
type RateVo struct {
	// CurrencyCode is the currency code.
	CurrencyCode string `json:"currencyCode"`
	// CurrencyDesc is the currency description.
	CurrencyDesc string `json:"currencyDesc"`
	// InterestRateWithinMortgage is the margin reference interest rate.
	InterestRateWithinMortgage string `json:"interestRateWithinMortgage"`
}

// MarginFullInfo is the securities margin snapshot returned by
// /trade/TradeQueryMarginFullInfo. Every ratio and quantity is a decimal
// string.
type MarginFullInfo struct {
	// MarginAllow is "1" when margin financing is supported, "0" otherwise.
	MarginAllow string `json:"marginAllow"`
	// MarginInitRatio is the margin initial ratio.
	MarginInitRatio string `json:"marginInitRatio"`
	// MarginKeepRatio is the margin maintenance ratio.
	MarginKeepRatio string `json:"marginKeepRatio"`
	// ShortAllow is "1" when securities lending is supported, "0" otherwise.
	ShortAllow string `json:"shortAllow"`
	// ShortInitMarginRatio is the short initial margin ratio.
	ShortInitMarginRatio string `json:"shortInitMarginRatio"`
	// ShortInterestRate is the short reference interest rate.
	ShortInterestRate string `json:"shortInterestRate"`
	// ShortKeepMarginRatio is the short maintenance margin ratio.
	ShortKeepMarginRatio string `json:"shortKeepMarginRatio"`
	// ShortLastAvailableQty is the remaining short-selling pool.
	ShortLastAvailableQty string `json:"shortLastAvailableQty"`
	// Rate lists the margin interest rate per currency.
	Rate []RateVo `json:"rate"`
}

// MarginFullInfo returns the securities margin data for one instrument.
func (m *Manager) MarginFullInfo(ctx context.Context, req MarginFullInfoRequest) (*MarginFullInfo, error) {
	if req.DataType == "" {
		return nil, invalid(opMarginFullInfo, "dataType is required")
	}
	if req.StockCode == "" {
		return nil, invalid(opMarginFullInfo, "stockCode is required")
	}
	var out MarginFullInfo
	if err := m.call(ctx, opMarginFullInfo, client.RouteTradeQueryMarginFullInfo, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BeforeAndAfterSupportRequest is the body of
// /trade/TradeQueryBeforeAndAfterSupport.
type BeforeAndAfterSupportRequest struct {
	// StockCode is the security code. It is required.
	StockCode string `json:"stockCode"`
	// ExchangeType selects the market. It is required.
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

// BeforeAndAfterSupport reports whether the instrument supports pre-market and
// post-market trading: "1" for yes, "0" for no.
func (m *Manager) BeforeAndAfterSupport(ctx context.Context, req BeforeAndAfterSupportRequest) (string, error) {
	if req.StockCode == "" {
		return "", invalid(opBeforeAndAfterSupport, "stockCode is required")
	}
	if req.ExchangeType == "" {
		return "", invalid(opBeforeAndAfterSupport, "exchangeType is required")
	}
	var out CommonStringResponse
	if err := m.call(ctx, opBeforeAndAfterSupport, client.RouteTradeQueryBeforeAndAfterSupport, req, &out); err != nil {
		return "", err
	}
	return out.Data, nil
}

// parsePositive parses s as an exact decimal and reports whether it is strictly
// greater than zero. It uses big.Rat, never a binary float, so the Gateway's
// decimal text is interpreted without rounding.
func parsePositive(s string) (*big.Rat, bool) {
	if s == "" {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, false
	}
	return r, true
}

// validateValidDays enforces the conditional-order lifetime rule: an integer in
// [1, maxValidDays].
func validateValidDays(op, validDays string) error {
	if validDays == "" {
		return invalid(op, "validDays is required for a conditional order")
	}
	n, err := strconv.Atoi(validDays)
	if err != nil || n < 1 || n > maxValidDays {
		return invalid(op, "validDays must be an integer between 1 and 100")
	}
	return nil
}

// isConditional reports whether t is one of the conditional order types
// (31 through 36).
func isConditional(t types.EntrustType) bool {
	switch t {
	case types.EntrustTypeStopProfitLimit,
		types.EntrustTypeStopProfitMarket,
		types.EntrustTypeStopLossLimit,
		types.EntrustTypeStopLossMarket,
		types.EntrustTypeTrailingStopLimit,
		types.EntrustTypeTrailingStopMarket:
		return true
	default:
		return false
	}
}

// isIceberg reports whether t is one of the US iceberg order types (8 or 9).
func isIceberg(t types.EntrustType) bool {
	switch t {
	case types.EntrustTypeIcebergMarket, types.EntrustTypeIcebergLimit:
		return true
	default:
		return false
	}
}

// priceOptional reports whether the docs allow an empty entrustPrice for t: a
// market order, an iceberg market order, a hidden market order, or any
// conditional order.
func priceOptional(t types.EntrustType) bool {
	switch t {
	case types.EntrustTypeMarket,
		types.EntrustTypeIcebergMarket,
		types.EntrustTypeHiddenMarket:
		return true
	default:
		return isConditional(t)
	}
}
