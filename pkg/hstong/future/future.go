// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package future

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels used in errors raised by Manager. They match the canonical
// Gateway route path and never carry request payloads or credentials.
const (
	opQueryProductInfo      = "trade/FuturesQueryProductInfo"
	opQueryMaxBuySellAmount = "trade/FuturesQueryMaxBuySellAmount"
	opQueryFundInfo         = "trade/FuturesQueryFundInfo"
	opQueryHoldsList        = "trade/FuturesQueryHoldsList"
	opEntrust               = "trade/FuturesEntrust"
	opCancelEntrust         = "trade/FuturesCancelEntrust"
	opModifyEntrust         = "trade/FuturesModifyEntrust"
	opQueryRealEntrustList  = "trade/FuturesQueryRealEntrustList"
	opQueryHistoryEntrust   = "trade/FuturesQueryHistoryEntrustList"
	opQueryRealDeliverList  = "trade/FuturesQueryRealDeliverList"
	opQueryHistoryDeliver   = "trade/FuturesQueryHistoryDeliverList"
)

// DefaultPageSize is the page size applied to history queries when the caller
// leaves PageSize unset. The Gateway documents a default of 20 and a maximum
// below 100 (docs/SPEC.md §8).
const DefaultPageSize = 20

// defaultPageNo is the first page number applied to history queries when the
// caller leaves PageNo unset.
const defaultPageNo int32 = 1

// entrustTypeMarket is the documented futures order type for a market order.
// A market order does not require an explicit price.
const entrustTypeMarket = "2"

// maxPageSizeExclusive is the exclusive upper bound the Gateway documents for a
// paginated history query: the accepted page size is strictly less than 100.
const maxPageSizeExclusive = 100

// decimalPattern matches a non-negative decimal number with no sign, exponent,
// or thousands separators: "0", "12", "3.5", and "0.01" are accepted; "-1",
// "1e3", and "" are not. It is used to validate string money and quantities
// without ever converting them to a binary float.
var decimalPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// datePattern matches the Gateway's yyyyMMdd date form used by history queries.
var datePattern = regexp.MustCompile(`^[0-9]{8}$`)

// Option configures a Manager. Options are applied in order on top of the
// defaults inside New; the last option that sets a field wins.
type Option func(*config)

// config holds the resolved Manager settings.
type config struct {
	// defaultPageSize is applied to a history query whose PageSize is unset.
	defaultPageSize int
}

// defaultConfig returns the configuration applied before any option.
func defaultConfig() config {
	return config{defaultPageSize: DefaultPageSize}
}

// WithDefaultPageSize sets the page size applied to a history query whose
// PageSize is left at zero. The default is DefaultPageSize (20). A value that
// is non-positive or not below the documented maximum of 100 restores the
// default; an oversized page is therefore never sent on the wire.
func WithDefaultPageSize(size int) Option {
	return func(c *config) {
		if size <= 0 || size >= maxPageSizeExclusive {
			c.defaultPageSize = DefaultPageSize
			return
		}
		c.defaultPageSize = size
	}
}

// Manager exposes the eleven futures trading endpoints over a shared
// *client.Client. It holds no per-request state and is safe for concurrent use.
type Manager struct {
	client          *client.Client
	defaultPageSize int
}

// New returns a Manager over c with opts applied in order on top of the
// defaults. c must be non-nil and is not owned by the manager: closing it is
// the caller's responsibility. Trading requires a live session; log in with
// hstong.SessionManager before calling any method here.
func New(c *client.Client, opts ...Option) *Manager {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Manager{client: c, defaultPageSize: cfg.defaultPageSize}
}

// invalidParam builds a typed "1016 invalid parameter" error for op. It never
// includes a secret and is the single way this package rejects bad input.
func invalidParam(op, message string) *errs.Error {
	return errs.New(types.StatusInvalidParam, op, message)
}

// validateStockCode reports whether code is a usable futures contract code. A
// code is usable when it is non-empty and contains no internal whitespace. The
// check is deliberately permissive about characters so that venue-specific
// forms (for example "HSI2603" or "01810.HK") pass unchanged.
func validateStockCode(op, code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return invalidParam(op, "stockCode must not be empty")
	}
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return invalidParam(op, "stockCode must not contain whitespace")
	}
	return nil
}

// validateQuantity reports whether q is a strictly positive decimal quantity.
// Zero and negative values are rejected; the value is never parsed as a float.
func validateQuantity(op, field, q string) error {
	if !decimalPattern.MatchString(q) {
		return invalidParam(op, field+" must be a non-negative decimal number")
	}
	if isZeroDecimal(q) {
		return invalidParam(op, field+" must be greater than zero")
	}
	return nil
}

// validatePrice reports whether price is a non-negative decimal price. When
// required is false an empty price is accepted (a market order carries no
// price); a supplied price must still be a well-formed decimal.
func validatePrice(op, field, price string, required bool) error {
	if price == "" {
		if required {
			return invalidParam(op, field+" is required")
		}
		return nil
	}
	if !decimalPattern.MatchString(price) {
		return invalidParam(op, field+" must be a non-negative decimal number")
	}
	return nil
}

// validateEntrustBS reports whether bs is one of the four documented futures
// buy/sell directions (1 open long, 2 close long, 3 close short, 4 open short).
func validateEntrustBS(op, bs string) error {
	switch types.EntrustBS(bs) {
	case types.EntrustBuy, types.EntrustSell, types.EntrustCloseShort, types.EntrustOpenShort:
		return nil
	default:
		return invalidParam(op, "entrustBs must be one of 1, 2, 3, 4")
	}
}

// validateDate reports whether date is a real yyyyMMdd calendar date.
func validateDate(op, field, date string) error {
	if !datePattern.MatchString(date) {
		return invalidParam(op, field+" must be yyyyMMdd")
	}
	if _, err := time.Parse("20060102", date); err != nil {
		return invalidParam(op, field+" must be a valid calendar date")
	}
	return nil
}

// isZeroDecimal reports whether a decimal string that decimalPattern accepts
// represents zero. It works on the digit characters only, so it never converts
// the value to a float.
func isZeroDecimal(s string) bool {
	for _, r := range s {
		if r != '0' && r != '.' {
			return false
		}
	}
	return true
}

// QueryProductInfoRequest is the body of /trade/FuturesQueryProductInfo. The
// Gateway expects the contract codes under the single "stockCode" array key.
type QueryProductInfoRequest struct {
	// StockCodes lists the futures contract codes to look up. It must be
	// non-empty and every entry must be a usable contract code.
	StockCodes []string `json:"stockCode"`
}

// ProductInfo describes one futures product/contract series.
type ProductInfo struct {
	// ProdCode is the product code (产品代码).
	ProdCode string `json:"prodCode"`
	// InstCode is the contract series type (合约系列类型).
	InstCode string `json:"instCode"`
	// LotSize is the number of units per lot (每手数量).
	LotSize int32 `json:"lotSize"`
	// DecInPrice is the price decimal power: 0 means 1, 1 means 0.1, 2 means
	// 0.01 (产品价格小数位).
	DecInPrice int32 `json:"decInPrice"`
	// ContractSize is the contract value (合约值).
	ContractSize string `json:"contractSize"`
	// PriceDecimalPoint is the price decimal point matching DecInPrice.
	PriceDecimalPoint string `json:"priceDecimalPoint"`
	// ExpiryDate is the product expiry date in yyyy-MM-dd form.
	ExpiryDate string `json:"expiryDate"`
	// IsSupportT1 reports T+1 support: 0 no, 1 yes.
	IsSupportT1 int32 `json:"isSupportT1"`
}

// QueryProductInfoResponse is the data object of /trade/FuturesQueryProductInfo.
type QueryProductInfoResponse struct {
	// ProductInfoVos is the matched product list.
	ProductInfoVos []ProductInfo `json:"productInfoVos"`
}

// QueryProductInfo returns product information for the requested contract
// codes. An empty StockCodes slice, or an entry that is empty or contains
// whitespace, is rejected with a typed "1016" error before any request is sent.
func (m *Manager) QueryProductInfo(ctx context.Context, req QueryProductInfoRequest) (*QueryProductInfoResponse, error) {
	if len(req.StockCodes) == 0 {
		return nil, invalidParam(opQueryProductInfo, "stockCode must contain at least one contract code")
	}
	for _, code := range req.StockCodes {
		if err := validateStockCode(opQueryProductInfo, code); err != nil {
			return nil, err
		}
	}
	out := &QueryProductInfoResponse{}
	if err := m.client.Do(ctx, opQueryProductInfo, client.RouteTradeFuturesQueryProductInfo, req, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// QueryMaxBuySellAmountRequest is the body of
// /trade/FuturesQueryMaxBuySellAmount.
type QueryMaxBuySellAmountRequest struct {
	// StockCode is the futures contract code to query.
	StockCode string `json:"stockCode"`
}

// QueryMaxBuySellAmountResponse is the data object of
// /trade/FuturesQueryMaxBuySellAmount.
type QueryMaxBuySellAmountResponse struct {
	// PositionStatus is the position state: 0 flat, 1 long, 2 short.
	PositionStatus int32 `json:"positionStatus"`
	// MaxBuyAmount is the maximum buyable quantity. It is an integer count as
	// documented, not a monetary value.
	MaxBuyAmount int64 `json:"maxBuyAmount"`
	// MaxSellAmount is the maximum sellable quantity. It is an integer count as
	// documented, not a monetary value.
	MaxSellAmount int64 `json:"maxSellAmount"`
	// InitialMargin is the opening margin (开仓按金).
	InitialMargin string `json:"initialMargin"`
	// Ccy is the currency of the amounts.
	Ccy string `json:"ccy"`
}

// QueryMaxBuySellAmount returns the maximum buyable and sellable quantity for
// one contract under the futures account. An empty or whitespace-containing
// StockCode is rejected with a typed "1016" error before any request is sent.
func (m *Manager) QueryMaxBuySellAmount(ctx context.Context, req QueryMaxBuySellAmountRequest) (*QueryMaxBuySellAmountResponse, error) {
	if err := validateStockCode(opQueryMaxBuySellAmount, req.StockCode); err != nil {
		return nil, err
	}
	out := &QueryMaxBuySellAmountResponse{}
	if err := m.client.Do(ctx, opQueryMaxBuySellAmount, client.RouteTradeFuturesQueryMaxBuySellAmount, req, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// FundInfo is the futures account funds snapshot. Every monetary field is a
// string so the Gateway's precision is preserved exactly.
type FundInfo struct {
	// AssetBalance is the net asset value (资产净值).
	AssetBalance string `json:"assetBalance"`
	// EnableBalance is the available funds / buying power (可用金额).
	EnableBalance string `json:"enableBalance"`
	// MarginCall is the margin call amount (追缴保证金).
	MarginCall string `json:"marginCall"`
	// IncomeBalance is the open-position profit/loss (持仓盈亏).
	IncomeBalance string `json:"incomeBalance"`
	// CashBal is the cash balance (现金结余).
	CashBal string `json:"cashBal"`
	// IMargin is the initial margin (基本保证金).
	IMargin string `json:"iMargin"`
	// MMargin is the maintenance margin (维持保证金).
	MMargin string `json:"mMargin"`
	// MarginLevel is the margin level (保证金水平).
	MarginLevel string `json:"marginLevel"`
	// MaxMargin is the maximum margin (最高保证金).
	MaxMargin string `json:"maxMargin"`
	// CreditLimit is the credit limit (信贷限额).
	CreditLimit string `json:"creditLimit"`
	// CtrlLevel is the control level (控制级数).
	CtrlLevel string `json:"ctrlLevel"`
	// MarginClass is the margin type (保证金类型).
	MarginClass string `json:"marginClass"`
	// AeID is the broker (经纪).
	AeID string `json:"aeId"`
	// CashBalHKD is the HKD cash balance (港币现金结余).
	CashBalHKD string `json:"cashBalHKD"`
	// CashBalUSD is the USD cash balance (美元现金结余).
	CashBalUSD string `json:"cashBalUSD"`
	// CycRateUSDtoHSD is the USD/HKD exchange rate (美元兑港币汇率).
	CycRateUSDtoHSD string `json:"cycRateUSDtoHSD"`
	// MarginStatus is the risk state: 1 safe, 2 warning, 3 danger, 4 liquidation.
	MarginStatus string `json:"marginStatus"`
	// StatusPercent is the risk gauge percentage (风险状态画图百分比).
	StatusPercent string `json:"statusPercent"`
	// CloseProfit is the realised profit/loss (已实现盈亏).
	CloseProfit string `json:"closeProfit"`
	// CashBalCNH is the CNH cash balance (人民币现金结余).
	CashBalCNH string `json:"cashBalCNH"`
}

// QueryFundInfoResponse is the data object of /trade/FuturesQueryFundInfo.
type QueryFundInfoResponse struct {
	// FundInfo is the futures account funds snapshot.
	FundInfo FundInfo `json:"fundInfo"`
}

// QueryFundInfo returns the futures account's net asset value, cash, buying
// power, and margin data. It takes no parameters and sends an empty params
// object.
func (m *Manager) QueryFundInfo(ctx context.Context) (*QueryFundInfoResponse, error) {
	out := &QueryFundInfoResponse{}
	if err := m.client.Do(ctx, opQueryFundInfo, client.RouteTradeFuturesQueryFundInfo, struct{}{}, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// Hold describes one futures position row. Every quantity and money field is a
// string.
type Hold struct {
	// StockName is the contract name (股票名称).
	StockName string `json:"stockName"`
	// StockCode is the contract code; Hong Kong codes carry a ".HK" suffix.
	StockCode string `json:"stockCode"`
	// LastDayQty is the previous day's position quantity (上日持仓数量).
	LastDayQty string `json:"lastDayQty"`
	// LastDayPrice is the previous day's holding cost (上日持仓成本).
	LastDayPrice string `json:"lastDayPrice"`
	// DepQty is the stored position (存储仓位).
	DepQty string `json:"depQty"`
	// DayLongQty is today's long quantity (今日长仓数量).
	DayLongQty string `json:"dayLongQty"`
	// DayLongPrice is today's long average price (今日长仓均价).
	DayLongPrice string `json:"dayLongPrice"`
	// DayShortQty is today's short quantity (今日短仓数量).
	DayShortQty string `json:"dayShortQty"`
	// DayShortPrice is today's short average price (今日短仓均价).
	DayShortPrice string `json:"dayShortPrice"`
	// DayNetQty is today's net quantity (今日净仓数量).
	DayNetQty string `json:"dayNetQty"`
	// DayNetPrice is today's net average price (今日净仓均价).
	DayNetPrice string `json:"dayNetPrice"`
	// CurrentQty is the current position quantity (持仓数量).
	CurrentQty string `json:"currentQty"`
	// CostPrice is the cost price (成本价).
	CostPrice string `json:"costPrice"`
	// LastPrice is the current price (现价). It is documented as unreliable;
	// prefer a market-data quote for valuation.
	LastPrice string `json:"lastPrice"`
	// PreClosePrice is the previous close (昨收价).
	PreClosePrice string `json:"preClosePrice"`
	// ProfitLoss is the unrealised profit/loss (盈亏).
	ProfitLoss string `json:"profitLoss"`
	// CcyRate is the reference conversion rate (参考兑换率).
	CcyRate string `json:"ccyRate"`
	// ContractValue is the contract value (合约值).
	ContractValue string `json:"contractValue"`
	// ProfitLossBaseCcy is the profit/loss in the base currency (盈亏基本货币).
	ProfitLossBaseCcy string `json:"profitLossBaseCcy"`
	// Ccy is the product series trading currency.
	Ccy string `json:"ccy"`
	// DataType is returned for futures only, to distinguish HK from US futures.
	DataType string `json:"dataType"`
	// CloseProfit is the realised profit/loss (已实现盈亏).
	CloseProfit string `json:"closeProfit"`
	// CloseProfitHKD is the realised profit/loss in HKD (已实现盈亏HKD).
	CloseProfitHKD string `json:"closeProfitHKD"`
}

// QueryHoldsListResponse is the data object of /trade/FuturesQueryHoldsList.
type QueryHoldsListResponse struct {
	// FundInfo is the account funds snapshot returned alongside the positions.
	FundInfo FundInfo `json:"fundInfo"`
	// HoldsList is the futures position list.
	HoldsList []Hold `json:"holdsList"`
}

// QueryHoldsList returns the futures account's positions together with the
// account funds snapshot. It takes no parameters and sends an empty params
// object.
func (m *Manager) QueryHoldsList(ctx context.Context) (*QueryHoldsListResponse, error) {
	out := &QueryHoldsListResponse{}
	if err := m.client.Do(ctx, opQueryHoldsList, client.RouteTradeFuturesQueryHoldsList, struct{}{}, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// EntrustRequest is the body of /trade/FuturesEntrust.
type EntrustRequest struct {
	// StockCode is the futures contract code.
	StockCode string `json:"stockCode"`
	// EntrustType is the order type: 0 limit, 1 auction, 2 market.
	EntrustType string `json:"entrustType,omitempty"`
	// EntrustPrice is the order price. It is optional for a market order
	// (EntrustType "2") and required otherwise.
	EntrustPrice string `json:"entrustPrice,omitempty"`
	// EntrustAmount is the order quantity; it must be a positive decimal.
	EntrustAmount string `json:"entrustAmount"`
	// EntrustBS is the direction: 1 open long, 2 close long, 3 close short,
	// 4 open short.
	EntrustBS string `json:"entrustBs"`
	// ValidTimeType is the time-in-force: 0 day, 1 immediate-or-cancel,
	// 2 fill-or-kill, 3 good-till-date, 4 good-till-specified-date.
	ValidTimeType string `json:"validTimeType,omitempty"`
	// ValidTime is the yyyyMMdd expiry and is required when ValidTimeType is 4.
	ValidTime string `json:"validTime,omitempty"`
	// OrderOptions is 0 default or 1 T+1.
	OrderOptions string `json:"orderOptions,omitempty"`
}

// ModifyEntrustRequest is the body of /trade/FuturesModifyEntrust.
type ModifyEntrustRequest struct {
	// EntrustID is the order to modify.
	EntrustID string `json:"entrustId"`
	// StockCode is the futures contract code.
	StockCode string `json:"stockCode"`
	// EntrustPrice is the new price; it must be a non-negative decimal.
	EntrustPrice string `json:"entrustPrice"`
	// EntrustAmount is the new quantity; it must be a positive decimal.
	EntrustAmount string `json:"entrustAmount"`
	// EntrustBS is the direction: 1 open long, 2 close long, 3 close short,
	// 4 open short.
	EntrustBS string `json:"entrustBs"`
	// ValidTimeType is the time-in-force, as in EntrustRequest.
	ValidTimeType string `json:"validTimeType,omitempty"`
	// ValidTime is the yyyyMMdd expiry and is required when ValidTimeType is 4.
	ValidTime string `json:"validTime,omitempty"`
	// OrderOptions is 0 default or 1 T+1.
	OrderOptions string `json:"orderOptions,omitempty"`
}

// CancelEntrustRequest is the body of /trade/FuturesCancelEntrust.
type CancelEntrustRequest struct {
	// EntrustID is the order to cancel.
	EntrustID string `json:"entrustId"`
	// StockCode is the futures contract code.
	StockCode string `json:"stockCode"`
}

// EntrustResponse is the data object of a successful futures order mutation.
// It is returned by Entrust, CancelEntrust, and ModifyEntrust. Data carries the
// resulting 委托编号; the Gateway does not guarantee that it is present.
type EntrustResponse struct {
	// Data is the order number, or the empty string when the Gateway omitted it.
	Data string `json:"data"`
}

// Entrust places a futures order. It is a mutation and issues exactly one
// attempt: the caller must reconcile by querying the real/history entrust list
// after an ambiguous failure rather than resubmitting
// (docs/adr/0003-no-auto-retry-orders.md).
//
// The request is rejected with a typed "1016" error before any request is sent
// when StockCode is empty, EntrustBS is not 1-4, EntrustAmount is not a positive
// decimal, the price is required but missing or malformed, or ValidTimeType is 4
// without a valid yyyyMMdd ValidTime.
func (m *Manager) Entrust(ctx context.Context, req EntrustRequest) (*EntrustResponse, error) {
	if err := validateStockCode(opEntrust, req.StockCode); err != nil {
		return nil, err
	}
	if err := validateEntrustBS(opEntrust, req.EntrustBS); err != nil {
		return nil, err
	}
	if err := validateQuantity(opEntrust, "entrustAmount", req.EntrustAmount); err != nil {
		return nil, err
	}
	if err := validatePrice(opEntrust, "entrustPrice", req.EntrustPrice, req.EntrustType != entrustTypeMarket); err != nil {
		return nil, err
	}
	if err := validateValidTime(opEntrust, req.ValidTimeType, req.ValidTime); err != nil {
		return nil, err
	}
	out := &EntrustResponse{}
	if err := m.client.Do(ctx, opEntrust, client.RouteTradeFuturesEntrust, req, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// CancelEntrust cancels a futures order, or the unfilled remainder of a
// partially filled order. It is a mutation and issues exactly one attempt: the
// caller must reconcile rather than resubmit after an ambiguous failure
// (docs/adr/0003-no-auto-retry-orders.md).
//
// An empty EntrustID or an empty/whitespace StockCode is rejected with a typed
// "1016" error before any request is sent.
func (m *Manager) CancelEntrust(ctx context.Context, req CancelEntrustRequest) (*EntrustResponse, error) {
	if strings.TrimSpace(req.EntrustID) == "" {
		return nil, invalidParam(opCancelEntrust, "entrustId must not be empty")
	}
	if err := validateStockCode(opCancelEntrust, req.StockCode); err != nil {
		return nil, err
	}
	out := &EntrustResponse{}
	if err := m.client.Do(ctx, opCancelEntrust, client.RouteTradeFuturesCancelEntrust, req, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// ModifyEntrust changes the price and quantity of an open futures order. It is
// a mutation and issues exactly one attempt: the caller must reconcile rather
// than resubmit after an ambiguous failure
// (docs/adr/0003-no-auto-retry-orders.md).
//
// An empty EntrustID, empty/whitespace StockCode, non-positive EntrustAmount,
// malformed EntrustPrice, or EntrustBS outside 1-4 is rejected with a typed
// "1016" error before any request is sent.
func (m *Manager) ModifyEntrust(ctx context.Context, req ModifyEntrustRequest) (*EntrustResponse, error) {
	if strings.TrimSpace(req.EntrustID) == "" {
		return nil, invalidParam(opModifyEntrust, "entrustId must not be empty")
	}
	if err := validateStockCode(opModifyEntrust, req.StockCode); err != nil {
		return nil, err
	}
	if err := validateQuantity(opModifyEntrust, "entrustAmount", req.EntrustAmount); err != nil {
		return nil, err
	}
	if err := validatePrice(opModifyEntrust, "entrustPrice", req.EntrustPrice, true); err != nil {
		return nil, err
	}
	if err := validateEntrustBS(opModifyEntrust, req.EntrustBS); err != nil {
		return nil, err
	}
	if err := validateValidTime(opModifyEntrust, req.ValidTimeType, req.ValidTime); err != nil {
		return nil, err
	}
	out := &EntrustResponse{}
	if err := m.client.Do(ctx, opModifyEntrust, client.RouteTradeFuturesModifyEntrust, req, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// validateValidTime enforces the documented ValidTimeType/ValidTime coupling: a
// specified-date order (type "4") requires a real yyyyMMdd ValidTime; any other
// non-empty type must not be "4" with an empty date, and a supplied date is
// always validated.
func validateValidTime(op, validTimeType, validTime string) error {
	if validTime == "" {
		if validTimeType == "4" {
			return invalidParam(op, "validTime is required when validTimeType is 4")
		}
		return nil
	}
	return validateDate(op, "validTime", validTime)
}

// EntrustOrder is one row of a futures entrust or deliver query. The Gateway
// returns the same object shape for both, so DeliverOrder aliases this type.
// Quantities, prices, and amounts are strings.
type EntrustOrder struct {
	// StockCode is the contract code; Hong Kong codes carry a ".HK" suffix.
	StockCode string `json:"stockCode"`
	// StockName is the contract name.
	StockName string `json:"stockName"`
	// BusinessPrice is the fill price (成交价格).
	BusinessPrice string `json:"businessPrice"`
	// EntrustBS is the direction: 1 buy, 2 sell.
	EntrustBS string `json:"entrustBs"`
	// EntrustPrice is the order price (委托价格).
	EntrustPrice string `json:"entrustPrice"`
	// EntrustAmount is the order quantity (委托数量).
	EntrustAmount string `json:"entrustAmount"`
	// BusinessAmount is the filled quantity (成交数量).
	BusinessAmount string `json:"businessAmount"`
	// Date is the fill date for a deliver query, or the order date otherwise.
	Date string `json:"date"`
	// BusinessTime is the fill time (成交时间).
	BusinessTime string `json:"businessTime"`
	// EntrustTime is the order time (委托时间).
	EntrustTime string `json:"entrustTime"`
	// QueryParamStr is the record cursor used for pagination.
	QueryParamStr string `json:"queryParamStr"`
	// StatusDesc is the order status description (委托状态中文描述).
	StatusDesc string `json:"statusDesc"`
	// Status is the order status code (委托状态).
	Status string `json:"status"`
	// EntrustID is the order number (委托编号).
	EntrustID string `json:"entrustId"`
	// CanBeCanceled is 1 when the order can be cancelled, else 0.
	CanBeCanceled int32 `json:"canBeCanceled"`
	// EntrustType is the order type.
	EntrustType string `json:"entrustType"`
	// EntrustTypeNum is the numeric order type.
	EntrustTypeNum string `json:"entrustTypeNum"`
	// IsValid is 1 when the row is valid, else 0.
	IsValid int32 `json:"isValid"`
	// CanBeUpdated is 1 when the order can be modified, else 0.
	CanBeUpdated int32 `json:"canBeUpdated"`
	// ValidType is the time-in-force code.
	ValidType string `json:"validType"`
	// ValidTypeDesc describes ValidType; for a specified date it holds the date.
	ValidTypeDesc string `json:"validTypeDesc"`
	// OrderOptions is 0 default or 1 T+1.
	OrderOptions int32 `json:"orderOptions"`
	// ValidTime is the expiry in yyyy/MM/dd form.
	ValidTime string `json:"validTime"`
}

// DeliverOrder is one futures fill row. It is an alias of EntrustOrder because
// the Gateway documents the same fields for both entrust and deliver lists.
type DeliverOrder = EntrustOrder

// EntrustListResponse is the data object of both the real and history futures
// entrust queries. The pagination fields are populated only by a history query.
type EntrustListResponse struct {
	// Data is the entrust rows.
	Data []EntrustOrder `json:"data"`
	// CurPageNo is the current page number (history only).
	CurPageNo int32 `json:"curPageNo"`
	// CurPageSize is the current page size (history only).
	CurPageSize int32 `json:"curPageSize"`
	// TotalPageNo is the total page count and is documented as obsolete.
	TotalPageNo int32 `json:"totalPageNo"`
	// LastPage is 1 on the last page, else 0 (history only).
	LastPage int32 `json:"lastPage"`
}

// DeliverListResponse is the data object of both the real and history futures
// deliver (fill) queries. The pagination fields are populated only by a history
// query.
type DeliverListResponse struct {
	// Data is the fill rows.
	Data []DeliverOrder `json:"data"`
	// CurPageNo is the current page number (history only).
	CurPageNo int32 `json:"curPageNo"`
	// CurPageSize is the current page size (history only).
	CurPageSize int32 `json:"curPageSize"`
	// TotalPageNo is the total page count and is documented as obsolete.
	TotalPageNo int32 `json:"totalPageNo"`
	// LastPage is 1 on the last page, else 0 (history only).
	LastPage int32 `json:"lastPage"`
}

// QueryRealEntrustList returns today's futures orders for the account. The
// Gateway returns at most 500 rows. It takes no parameters and sends an empty
// params object.
func (m *Manager) QueryRealEntrustList(ctx context.Context) (*EntrustListResponse, error) {
	out := &EntrustListResponse{}
	if err := m.client.Do(ctx, opQueryRealEntrustList, client.RouteTradeFuturesQueryRealEntrustList, struct{}{}, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// HistoryQueryRequest is the body of the two futures history queries. PageNo
// and PageSize default to 1 and the manager's default page size (20) when left
// unset; StartDate and EndDate are optional yyyyMMdd dates.
type HistoryQueryRequest struct {
	// PageNo is the 1-based page number.
	PageNo int32 `json:"pageNo,omitempty"`
	// PageSize is the rows per page; it must be positive and below 100.
	PageSize int32 `json:"pageSize,omitempty"`
	// StartDate is the query start date in yyyyMMdd form.
	StartDate string `json:"startDate,omitempty"`
	// EndDate is the query end date in yyyyMMdd form.
	EndDate string `json:"endDate,omitempty"`
}

// historyParams resolves the effective history parameters: it applies the
// manager's defaults for PageNo and PageSize and validates the page size and
// any supplied dates before the request is sent.
func (m *Manager) historyParams(op string, req HistoryQueryRequest) (HistoryQueryRequest, error) {
	if req.PageNo <= 0 {
		req.PageNo = defaultPageNo
	}
	if req.PageSize <= 0 {
		req.PageSize = int32(m.defaultPageSize)
	}
	if req.PageSize >= maxPageSizeExclusive {
		return HistoryQueryRequest{}, invalidParam(op, "pageSize must be below 100")
	}
	if req.StartDate != "" {
		if err := validateDate(op, "startDate", req.StartDate); err != nil {
			return HistoryQueryRequest{}, err
		}
	}
	if req.EndDate != "" {
		if err := validateDate(op, "endDate", req.EndDate); err != nil {
			return HistoryQueryRequest{}, err
		}
	}
	return req, nil
}

// QueryHistoryEntrustList returns historical futures orders for the account,
// page by page. The list is the last page when it is empty or shorter than
// PageSize. An oversized PageSize or a malformed StartDate/EndDate is rejected
// with a typed "1016" error before any request is sent.
func (m *Manager) QueryHistoryEntrustList(ctx context.Context, req HistoryQueryRequest) (*EntrustListResponse, error) {
	params, err := m.historyParams(opQueryHistoryEntrust, req)
	if err != nil {
		return nil, err
	}
	out := &EntrustListResponse{}
	if err := m.client.Do(ctx, opQueryHistoryEntrust, client.RouteTradeFuturesQueryHistoryEntrustList, params, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// QueryRealDeliverList returns today's futures fills for the account. The
// Gateway returns at most 200 rows. It takes no parameters and sends an empty
// params object.
func (m *Manager) QueryRealDeliverList(ctx context.Context) (*DeliverListResponse, error) {
	out := &DeliverListResponse{}
	if err := m.client.Do(ctx, opQueryRealDeliverList, client.RouteTradeFuturesQueryRealDeliverList, struct{}{}, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// QueryHistoryDeliverList returns historical futures fills for the account,
// page by page. An oversized PageSize or a malformed StartDate/EndDate is
// rejected with a typed "1016" error before any request is sent.
func (m *Manager) QueryHistoryDeliverList(ctx context.Context, req HistoryQueryRequest) (*DeliverListResponse, error) {
	params, err := m.historyParams(opQueryHistoryDeliver, req)
	if err != nil {
		return nil, err
	}
	out := &DeliverListResponse{}
	if err := m.client.Do(ctx, opQueryHistoryDeliver, client.RouteTradeFuturesQueryHistoryDeliverList, params, m.client.JSON(), out); err != nil {
		return nil, err
	}
	return out, nil
}
