// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"strconv"
	"strings"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the futures half of the domain model: the payloads the Gateway
// sends as rows, the models those rows map to, and the mappers between them. It
// follows the split account.go establishes for the cash layer, and the rule it
// applies is the same one: a row becomes an exported …Wire type here, an
// envelope that holds rows stays unexported in pkg/services.
//
// Every mapper here takes one row and never an envelope. The Gateway wraps its
// futures rows — productInfoVos, fundInfo, holdsList, data — and those wrappers
// belong to the layer that owns the request, which is the layer that decodes the
// reply. A mapper here that took a pkg/services envelope would invert the
// dependency ADR 0010 draws, and it would be a cycle besides: pkg/services
// imports this package. The cautionary shape is the one this package used to
// carry: trading.go's CondOrderPageWire and OrderListWire were exported envelope
// types here that no service ever decoded into, and they were deleted rather than
// commented on, so the mistake now has no example left in the tree to be copied
// from. An envelope in this package is a type with no decoder in its own package.
//
// The one respect in which these mappers deliberately diverge from the cash
// ones is load-bearing and is stated on futuresMoney: a futures row is read
// through a wrapper that turns an absent field into an explicit zero, where a
// cash mapper hands the wire string straight to a MustNew… constructor and would
// panic on "". A partial futures reply is a read, not a corrupt input, so a read
// must not crash the caller's process.

const (
	// futuresZeroTick is the tick every futures Price is built with.
	//
	// "0" is the tick note's rule for a price whose instrument tick schedule is
	// not known: I observed this price, I do not know this product's tick. It is
	// not a claim that the tick is zero — Price.Validate skips its step check
	// when the tick is zero and Price.Round is a no-op, so nothing is checked
	// against a grid this SDK invented and nothing is rounded.
	//
	// The per-product tick does exist and is carried: FuturesProduct.DecInPrice
	// is the product's price decimal power, and futuresTickString renders it.
	// What is missing is the type that would hold a tick schedule, which the tick
	// model records as its own follow-on (docs/runs/2026-09-26-vnext-parity-wire/
	// design-tick-model.md §2.5). Inventing it here for futures alone would
	// create that type twice, so the value travels and the tick does not.
	//
	// Note this is not a fixed thousandth-place grid, which is what the sites
	// elsewhere in this repository still carry and which the same note records
	// for removal. A futures price grid is per-product, so a constant would be
	// wrong here more often than not.
	futuresZeroTick = "0"
	// futuresCurrencyHKD, futuresCurrencyUSD and futuresCurrencyCNH are the
	// currencies the Gateway names in a futures field name: cashBalHKD,
	// cashBalUSD, cashBalCNH, closeProfitHKD.
	futuresCurrencyHKD = "HKD"
	futuresCurrencyUSD = "USD"
	futuresCurrencyCNH = "CNH"
	// futuresBaseCurrency is attributed to a futures amount the Gateway does not
	// label with a currency at all — every aggregate on the account snapshot,
	// and the profit/loss columns on a position row, none of which carry a
	// currency field.
	//
	// It is an assumption, and it is the same one the cash layer already makes
	// for its equally unlabelled amounts (account.go in this same package uses
	// "HKD" throughout). The Gateway sends no futures base-currency field, so
	// nothing here can do better, and the assumption is stated rather than
	// hidden: a caller can read Money.Currency() and check it. The failure
	// direction is the loud one, because Money.Add and Sub panic across
	// currencies — an account denominated in something else gets a panic on a
	// cross-currency sum instead of a silently wrong number.
	futuresBaseCurrency = futuresCurrencyHKD
	// futuresMoneyScale is the decimal scale recorded on every futures Money.
	// It matches the cash layer's scale and ADR 0008's HKD row. MustNewMoney
	// records the scale without rounding, so no futures digit is discarded by
	// choosing it.
	futuresMoneyScale uint8 = 3
)

// FuturesProductInfoWire is one product row of a
// /trade/FuturesQueryProductInfo reply.
//
// DecInPrice is the product's price decimal power and PriceDecimalPoint is the
// Gateway's string rendering of it. Both are decoded because both are on the
// wire, and only DecInPrice is authoritative: see FuturesProduct.
type FuturesProductInfoWire struct {
	ProdCode          string `json:"prodCode"`
	InstCode          string `json:"instCode"`
	LotSize           int32  `json:"lotSize"`
	DecInPrice        int32  `json:"decInPrice"`
	ContractSize      string `json:"contractSize"`
	PriceDecimalPoint string `json:"priceDecimalPoint"`
	ExpiryDate        string `json:"expiryDate"`
	IsSupportT1       int32  `json:"isSupportT1"`
}

// FuturesMaxBuySellAmountWire is the /trade/FuturesQueryMaxBuySellAmount
// payload, and it is a payload rather than an envelope: the transport layer has
// already unwrapped the reply's data object, and the mapper below consumes this
// type directly.
//
// MaxBuyAmount and MaxSellAmount are the only numerics in any futures reply.
// They are contract counts, the vendor types them as a signed 64-bit long, and
// they are deliberately not strings: a string would accept a non-numeric value
// the Gateway never sends and could not be range-checked, and a float could not
// represent every integer above 2^53. They are equally not money, so they never
// become Money.
type FuturesMaxBuySellAmountWire struct {
	PositionStatus int32  `json:"positionStatus"`
	MaxBuyAmount   int64  `json:"maxBuyAmount"`
	MaxSellAmount  int64  `json:"maxSellAmount"`
	InitialMargin  string `json:"initialMargin"`
	Ccy            string `json:"ccy"`
}

// FuturesFundInfoWire is the futures account funds snapshot row, read by both
// /trade/FuturesQueryFundInfo and, alongside the positions, by
// /trade/FuturesQueryHoldsList. Every monetary field is a quoted string.
type FuturesFundInfoWire struct {
	AssetBalance    string `json:"assetBalance"`
	EnableBalance   string `json:"enableBalance"`
	MarginCall      string `json:"marginCall"`
	IncomeBalance   string `json:"incomeBalance"`
	CashBal         string `json:"cashBal"`
	IMargin         string `json:"iMargin"`
	MMargin         string `json:"mMargin"`
	MarginLevel     string `json:"marginLevel"`
	MaxMargin       string `json:"maxMargin"`
	CreditLimit     string `json:"creditLimit"`
	CtrlLevel       string `json:"ctrlLevel"`
	MarginClass     string `json:"marginClass"`
	AEID            string `json:"aeId"`
	CashBalHKD      string `json:"cashBalHKD"`
	CashBalUSD      string `json:"cashBalUSD"`
	CycRateUSDtoHSD string `json:"cycRateUSDtoHSD"`
	MarginStatus    string `json:"marginStatus"`
	StatusPercent   string `json:"statusPercent"`
	CloseProfit     string `json:"closeProfit"`
	CashBalCNH      string `json:"cashBalCNH"`
}

// FuturesHoldWire is one position row of a /trade/FuturesQueryHoldsList reply.
// Every quantity, price and amount is a quoted string.
type FuturesHoldWire struct {
	StockName         string `json:"stockName"`
	StockCode         string `json:"stockCode"`
	LastDayQty        string `json:"lastDayQty"`
	LastDayPrice      string `json:"lastDayPrice"`
	DepQty            string `json:"depQty"`
	DayLongQty        string `json:"dayLongQty"`
	DayLongPrice      string `json:"dayLongPrice"`
	DayShortQty       string `json:"dayShortQty"`
	DayShortPrice     string `json:"dayShortPrice"`
	DayNetQty         string `json:"dayNetQty"`
	DayNetPrice       string `json:"dayNetPrice"`
	CurrentQty        string `json:"currentQty"`
	CostPrice         string `json:"costPrice"`
	LastPrice         string `json:"lastPrice"`
	PreClosePrice     string `json:"preClosePrice"`
	ProfitLoss        string `json:"profitLoss"`
	CcyRate           string `json:"ccyRate"`
	ContractValue     string `json:"contractValue"`
	ProfitLossBaseCcy string `json:"profitLossBaseCcy"`
	Ccy               string `json:"ccy"`
	DataType          string `json:"dataType"`
	CloseProfit       string `json:"closeProfit"`
	CloseProfitHKD    string `json:"closeProfitHKD"`
}

// FuturesOrderWire is one row of a futures entrust or deliver reply. The
// Gateway documents the same object for both lists — SPEC.md aliases
// DeliverOrder to EntrustOrder, and the released layer declares the same
// alias — so one wire type decodes all four list routes.
type FuturesOrderWire struct {
	StockCode      string `json:"stockCode"`
	StockName      string `json:"stockName"`
	BusinessPrice  string `json:"businessPrice"`
	EntrustBS      string `json:"entrustBs"`
	EntrustPrice   string `json:"entrustPrice"`
	EntrustAmount  string `json:"entrustAmount"`
	BusinessAmount string `json:"businessAmount"`
	Date           string `json:"date"`
	BusinessTime   string `json:"businessTime"`
	EntrustTime    string `json:"entrustTime"`
	QueryParamStr  string `json:"queryParamStr"`
	StatusDesc     string `json:"statusDesc"`
	Status         string `json:"status"`
	EntrustID      string `json:"entrustId"`
	CanBeCanceled  int32  `json:"canBeCanceled"`
	EntrustType    string `json:"entrustType"`
	EntrustTypeNum string `json:"entrustTypeNum"`
	IsValid        int32  `json:"isValid"`
	CanBeUpdated   int32  `json:"canBeUpdated"`
	ValidType      string `json:"validType"`
	ValidTypeDesc  string `json:"validTypeDesc"`
	OrderOptions   int32  `json:"orderOptions"`
	ValidTime      string `json:"validTime"`
}

// FuturesProduct is one futures contract series.
//
// DecInPrice is the product's price decimal power and is the futures tick
// source: a power of 3 denotes a three-decimal-place grid, a power of 2 a
// two-decimal-place one. A value of 0 is a legitimate whole-unit grid — the
// mock's HSI product has decInPrice 0 and prices like 25000 — and must not be
// read as unset.
//
// PriceDecimalPoint is the Gateway's own string rendering of the same value. It
// is carried because it is on the wire, and it is never the source: a string
// invites a float parse, which is the defect this repository removed from 23
// sites, and a derived value must come from the integer power. futuresTickString
// is the derivation this package uses; a TickSchedule keyed by contract code,
// which is where it belongs, is the tick model's follow-on work.
type FuturesProduct struct {
	// ProdCode is the product code (产品代码).
	ProdCode string
	// InstCode is the contract series type (合约系列类型).
	InstCode string
	// LotSize is the number of units per lot (每手数量).
	LotSize int32
	// DecInPrice is the price decimal power: 0 means 1, 1 means 0.1, 2 means
	// 0.01. See the type comment.
	DecInPrice int32
	// ContractSize is the contract value (合约值), a count rather than an
	// amount, so it is a Quantity and not a Money.
	ContractSize Quantity
	// PriceDecimalPoint is the Gateway's rendering of DecInPrice. Informational:
	// a disagreement with the derived tick is the Gateway's to explain, and the
	// integer power is what this SDK derives from.
	PriceDecimalPoint string
	// ExpiryDate is the product expiry in yyyy-MM-dd form.
	ExpiryDate string
	// IsSupportT1 reports T+1 support, true for the wire's 1 and false for 0.
	IsSupportT1 bool
}

// FuturesCapacity is the buying and selling power for one futures contract.
//
// The reply names no contract, so a caller correlates this with the symbol it
// asked about; nothing here can recover it. A null MaxBuyAmount or
// MaxSellAmount is a legitimate answer meaning "you can buy nothing", which is
// why the two are mapped through an integer formatter rather than treated as
// absent.
type FuturesCapacity struct {
	// PositionStatus is the position state: 0 flat, 1 long, 2 short. It is an
	// int rather than a named type because no closed set for the futures
	// vocabulary is established in this repository, and inventing one would be
	// the kind of set a future narrowing could break.
	PositionStatus int32
	// MaxBuyAmount is the maximum buyable quantity, as a contract count.
	MaxBuyAmount Quantity
	// MaxSellAmount is the maximum sellable quantity, as a contract count.
	MaxSellAmount Quantity
	// InitialMargin is the opening margin (开仓按金), in Ccy. It is money, so
	// unlike the two counts above it is quoted on the wire and read verbatim.
	InitialMargin Money
	// Ccy is the currency the Gateway attributes the amounts to. A reply that
	// omits it is attributed the base currency, as every other unlabelled
	// futures amount is.
	Ccy string
}

// FuturesAccount is the futures account's funds snapshot.
//
// The currency rule is the one stated on futuresBaseCurrency: an amount whose
// field name states a currency — CashBalHKD, CashBalUSD, CashBalCNH — carries
// that currency, and every other amount on the snapshot is attributed the base
// currency because the Gateway sends no futures base-currency field. Read
// Money.Currency() before summing across accounts.
//
// MarginLevel, StatusPercent and CycRateUSDtoHSD are ratios, not amounts, and
// are typed as Rate so an addition cannot mistake a margin level for a balance.
type FuturesAccount struct {
	// AssetBalance is the net asset value (资产净值).
	AssetBalance Money
	// EnableBalance is the available funds, or buying power (可用金额).
	EnableBalance Money
	// MarginCall is the margin call amount (追缴保证金).
	MarginCall Money
	// IncomeBalance is the open-position profit or loss (持仓盈亏).
	IncomeBalance Money
	// CashBal is the cash balance (现金结余).
	CashBal Money
	// IMargin is the initial margin (基本保证金).
	IMargin Money
	// MMargin is the maintenance margin (维持保证金).
	MMargin Money
	// MaxMargin is the maximum margin (最高保证金).
	MaxMargin Money
	// CreditLimit is the credit limit (信贷限额).
	CreditLimit Money
	// CloseProfit is the realised profit or loss (已实现盈亏).
	CloseProfit Money
	// CashBalHKD is the Hong Kong dollar cash balance (港币现金结余).
	CashBalHKD Money
	// CashBalUSD is the US dollar cash balance (美元现金结余).
	CashBalUSD Money
	// CashBalCNH is the offshore renminbi cash balance (人民币现金结余). It is
	// CNH, not CNY, because that is what the field is named on the wire.
	CashBalCNH Money
	// MarginLevel is the margin level (保证金水平) as a ratio.
	MarginLevel Rate
	// StatusPercent is the risk gauge (风险状态画图百分比) as a ratio.
	StatusPercent Rate
	// CycRateUSDtoHSD is the US dollar to Hong Kong dollar rate (美元兑港币汇率).
	CycRateUSDtoHSD Rate
	// AEID is the broker (经纪).
	AEID string
	// CtrlLevel is the control level (控制级数).
	CtrlLevel string
	// MarginClass is the margin type (保证金类型).
	MarginClass string
	// MarginStatus is the risk state: 1 safe, 2 warning, 3 danger,
	// 4 liquidation.
	MarginStatus string
}

// FuturesPosition is one futures position row.
//
// DataType is the futures-only field that distinguishes a Hong Kong contract
// from a US one, and it is the reason no futures request needs a market: the
// request identifies the contract and the reply reads the book back.
//
// Every price here carries the zero tick, because the product's tick schedule
// is not available on a position row; see futuresZeroTick.
type FuturesPosition struct {
	// StockName is the contract name (股票名称).
	StockName string
	// StockCode is the contract code; a Hong Kong code carries a ".HK" suffix.
	StockCode string
	// DataType is the futures instrument type, distinguishing HK from US
	// futures (产品类型).
	DataType string
	// Ccy is the product series trading currency (交易货币).
	Ccy string
	// CcyRate is the reference conversion rate (参考兑换率).
	CcyRate Rate
	// ContractValue is the contract value (合约值), a count rather than an
	// amount.
	ContractValue Quantity
	// LastDayQty is the previous day's position quantity (上日持仓数量).
	LastDayQty Quantity
	// DepQty is the stored position (存储仓位).
	DepQty Quantity
	// DayLongQty is today's long quantity (今日长仓数量).
	DayLongQty Quantity
	// DayShortQty is today's short quantity (今日短仓数量).
	DayShortQty Quantity
	// DayNetQty is today's net quantity (今日净仓数量).
	DayNetQty Quantity
	// CurrentQty is the current position quantity (持仓数量).
	CurrentQty Quantity
	// LastDayPrice is the previous day's holding cost (上日持仓成本).
	LastDayPrice Price
	// DayLongPrice is today's long average price (今日长仓均价).
	DayLongPrice Price
	// DayShortPrice is today's short average price (今日短仓均价).
	DayShortPrice Price
	// DayNetPrice is today's net average price (今日净仓均价).
	DayNetPrice Price
	// CostPrice is the cost price (成本价).
	CostPrice Price
	// LastPrice is the current price (现价). The Gateway documents it as
	// unreliable; prefer a market-data quote to value the position.
	LastPrice Price
	// PreClosePrice is the previous close (昨收价).
	PreClosePrice Price
	// ProfitLoss is the unrealised profit or loss (盈亏).
	ProfitLoss Money
	// ProfitLossBaseCcy is the unrealised profit or loss in the base currency
	// (盈亏基本货币).
	ProfitLossBaseCcy Money
	// CloseProfit is the realised profit or loss (已实现盈亏).
	CloseProfit Money
	// CloseProfitHKD is the realised profit or loss in Hong Kong dollars
	// (已实现盈亏HKD).
	CloseProfitHKD Money
}

// FuturesHoldResult is the atomic result of the futures position query: the
// account funds snapshot and the positions, exactly as the Gateway returns
// them.
//
// It is a struct rather than a position slice because the reply is one read
// carrying both, and returning the positions alone would silently discard a
// funds snapshot the caller asked for.
//
// The two halves arrive in one envelope, which belongs to pkg/services, so the
// pairing is assembled there — this package maps the snapshot and each position
// row and nothing else.
type FuturesHoldResult struct {
	// Account is the funds snapshot the Gateway returned with the positions.
	Account *FuturesAccount
	// Positions is the position list, in the order the Gateway sent it. It is
	// the non-nil empty slice for an empty list, so a caller ranging over it
	// behaves the same either way.
	Positions []*FuturesPosition
}

// FuturesOrder is one row of a futures order list: a resting order, whether
// today's or from the history query.
//
// OrderType is a string and is not validated against a set, because the two
// vendor SDKs disagree on whether a fourth code exists. Status is a
// types.EntrustStatus: the futures rows use the same lifecycle dictionary as
// the cash rows, so the named type is the same set stated once.
//
// Every price carries the zero tick; see futuresZeroTick.
type FuturesOrder struct {
	// StockCode is the contract code; a Hong Kong code carries a ".HK" suffix.
	StockCode string
	// StockName is the contract name.
	StockName string
	// EntrustID is the order number (委托编号).
	EntrustID EntrustID
	// Side is the direction, as types.EntrustBS.
	Side types.EntrustBS
	// OrderType is the order type code, unvalidated by design.
	OrderType string
	// OrderTypeNum is the numeric order type the Gateway echoes alongside it.
	OrderTypeNum string
	// OrderPrice is the order price (委托价格).
	OrderPrice Price
	// FillPrice is the average fill price (成交价格).
	FillPrice Price
	// Quantity is the order quantity (委托数量).
	Quantity Quantity
	// FilledQty is the filled quantity (成交数量).
	FilledQty Quantity
	// Status is the order status code (委托状态).
	Status types.EntrustStatus
	// StatusDesc is the order status description (委托状态中文描述).
	StatusDesc string
	// ValidType is the time-in-force code the order was placed with.
	ValidType string
	// ValidTypeDesc describes ValidType, and holds the date itself for a
	// specified-date order.
	ValidTypeDesc string
	// ValidTime is the expiry as the Gateway renders it in a reply, in
	// yyyy/MM/dd form. The request's validTime is yyyyMMdd: the two directions
	// use different formats and this one must not be sent back.
	ValidTime string
	// OrderOptions is 0 for the default or 1 for T+1.
	OrderOptions int32
	// Date is the order date.
	Date string
	// EntrustTime is the order time (委托时间).
	EntrustTime string
	// BusinessTime is the fill time (成交时间), empty for an order that has not
	// traded.
	BusinessTime string
	// QueryParamStr is the Gateway's record cursor. Futures pagination is
	// page-numbered and has no cursor, so this is informational: it is not a
	// paging token for any method in the SDK.
	QueryParamStr string
	// IsValid reports whether the row is valid, true for the wire's 1.
	IsValid bool
	// CanBeCanceled reports whether the order can still be cancelled, true for
	// the wire's 1. It is the field to read to tell a live order from one that
	// has already gone.
	CanBeCanceled bool
	// CanBeUpdated reports whether the order can be modified, true for the
	// wire's 1. A pending modify leaves it false until the Gateway applies one.
	CanBeUpdated bool
}

// FuturesOrderPage is one page of a futures order history query.
//
// The real (unpaginated) order list returns a slice instead: it has no page
// state to report, and a page with four zero-valued page fields would be a
// fiction.
//
// The page counters arrive in the same envelope as the rows and are assembled by
// pkg/services, which owns the envelope; this package owns the rows and the
// model they map to.
type FuturesOrderPage struct {
	// Orders is the page's order rows, in the order the Gateway sent them.
	Orders []*FuturesOrder
	// CurPageNo is the current page number (当前页码).
	CurPageNo int32
	// CurPageSize is the number of rows in this page (当前页大小).
	CurPageSize int32
	// TotalPageNo is the total page count, documented by the Gateway as
	// obsolete. It is carried for parity and should not be relied on.
	TotalPageNo int32
	// LastPage reports whether this is the last page, true for the wire's 1.
	LastPage bool
}

// FuturesFill is one row of a futures fill (deliver) list.
//
// It is a distinct type from FuturesOrder even though the Gateway documents the
// same object for both lists, because a fill and a resting order are different
// facts: a caller reconciling a submit reads FuturesOrder, and a caller
// reconciling a fill reads this. Every price carries the zero tick; see
// futuresZeroTick.
type FuturesFill struct {
	// StockCode is the contract code; a Hong Kong code carries a ".HK" suffix.
	StockCode string
	// StockName is the contract name.
	StockName string
	// EntrustID is the order number this fill belongs to (委托编号).
	EntrustID EntrustID
	// Side is the direction, as types.EntrustBS.
	Side types.EntrustBS
	// OrderType is the order type code, unvalidated by design.
	OrderType string
	// OrderTypeNum is the numeric order type the Gateway echoes alongside it.
	OrderTypeNum string
	// Price is the fill price (成交价格).
	Price Price
	// OrderPrice is the order price the fill executed against (委托价格).
	OrderPrice Price
	// Quantity is the order quantity (委托数量), which may exceed FilledQty on
	// a partially filled order.
	Quantity Quantity
	// FilledQty is the filled quantity (成交数量).
	FilledQty Quantity
	// Status is the order status code (委托状态).
	Status types.EntrustStatus
	// StatusDesc is the order status description (委托状态中文描述).
	StatusDesc string
	// ValidType is the time-in-force code the order was placed with.
	ValidType string
	// ValidTypeDesc describes ValidType, and holds the date itself for a
	// specified-date order.
	ValidTypeDesc string
	// ValidTime is the expiry as the Gateway renders it in a reply, in
	// yyyy/MM/dd form.
	ValidTime string
	// OrderOptions is 0 for the default or 1 for T+1.
	OrderOptions int32
	// Date is the fill date (成交日期).
	Date string
	// BusinessTime is the fill time (成交时间).
	BusinessTime string
	// EntrustTime is the order time (委托时间).
	EntrustTime string
	// QueryParamStr is the Gateway's record cursor, informational only: futures
	// pagination is page-numbered.
	QueryParamStr string
	// IsValid reports whether the row is valid, true for the wire's 1.
	IsValid bool
	// CanBeCanceled reports whether the parent order can still be cancelled,
	// true for the wire's 1.
	CanBeCanceled bool
	// CanBeUpdated reports whether the parent order can be modified, true for
	// the wire's 1.
	CanBeUpdated bool
}

// FuturesFillPage is one page of a futures fill history query, the fill
// counterpart of FuturesOrderPage.
type FuturesFillPage struct {
	// Fills is the page's fill rows, in the order the Gateway sent them.
	Fills []*FuturesFill
	// CurPageNo is the current page number (当前页码).
	CurPageNo int32
	// CurPageSize is the number of rows in this page (当前页大小).
	CurPageSize int32
	// TotalPageNo is the total page count, documented by the Gateway as
	// obsolete. It is carried for parity and should not be relied on.
	TotalPageNo int32
	// LastPage reports whether this is the last page, true for the wire's 1.
	LastPage bool
}

// FuturesProductFromDTO maps one product row. A nil row yields a nil product,
// so the mapping is safe to call over a reply whose list is absent.
func FuturesProductFromDTO(v *FuturesProductInfoWire) *FuturesProduct {
	if v == nil {
		return nil
	}
	return &FuturesProduct{
		ProdCode:          v.ProdCode,
		InstCode:          v.InstCode,
		LotSize:           v.LotSize,
		DecInPrice:        v.DecInPrice,
		ContractSize:      futuresQuantity(v.ContractSize),
		PriceDecimalPoint: v.PriceDecimalPoint,
		ExpiryDate:        v.ExpiryDate,
		IsSupportT1:       v.IsSupportT1 == 1,
	}
}

// FuturesCapacityFromDTO maps a max buy/sell payload.
//
// The two counts come off the wire as int64 and are rendered with
// strconv.FormatInt before they become a Quantity. That call cannot panic:
// FormatInt of an int64 always yields a decimal string, which is exactly what
// the domain constructor accepts. It is also the only honest treatment of a
// count that can exceed 2^53 — a float would silently round one, and a string
// wire field would accept a non-numeric value the Gateway never sends.
func FuturesCapacityFromDTO(v *FuturesMaxBuySellAmountWire) *FuturesCapacity {
	if v == nil {
		return nil
	}
	return &FuturesCapacity{
		PositionStatus: v.PositionStatus,
		MaxBuyAmount:   MustNewQuantity(strconv.FormatInt(v.MaxBuyAmount, 10)),
		MaxSellAmount:  MustNewQuantity(strconv.FormatInt(v.MaxSellAmount, 10)),
		InitialMargin:  futuresMoney(v.InitialMargin, v.Ccy),
		Ccy:            v.Ccy,
	}
}

// FuturesAccountFromDTO maps the account funds snapshot row. A nil snapshot
// yields a nil account.
func FuturesAccountFromDTO(v *FuturesFundInfoWire) *FuturesAccount {
	if v == nil {
		return nil
	}
	return &FuturesAccount{
		AssetBalance:    futuresMoney(v.AssetBalance, futuresBaseCurrency),
		EnableBalance:   futuresMoney(v.EnableBalance, futuresBaseCurrency),
		MarginCall:      futuresMoney(v.MarginCall, futuresBaseCurrency),
		IncomeBalance:   futuresMoney(v.IncomeBalance, futuresBaseCurrency),
		CashBal:         futuresMoney(v.CashBal, futuresBaseCurrency),
		IMargin:         futuresMoney(v.IMargin, futuresBaseCurrency),
		MMargin:         futuresMoney(v.MMargin, futuresBaseCurrency),
		MaxMargin:       futuresMoney(v.MaxMargin, futuresBaseCurrency),
		CreditLimit:     futuresMoney(v.CreditLimit, futuresBaseCurrency),
		CloseProfit:     futuresMoney(v.CloseProfit, futuresBaseCurrency),
		CashBalHKD:      futuresMoney(v.CashBalHKD, futuresCurrencyHKD),
		CashBalUSD:      futuresMoney(v.CashBalUSD, futuresCurrencyUSD),
		CashBalCNH:      futuresMoney(v.CashBalCNH, futuresCurrencyCNH),
		MarginLevel:     futuresRate(v.MarginLevel),
		StatusPercent:   futuresRate(v.StatusPercent),
		CycRateUSDtoHSD: futuresRate(v.CycRateUSDtoHSD),
		AEID:            v.AEID,
		CtrlLevel:       v.CtrlLevel,
		MarginClass:     v.MarginClass,
		MarginStatus:    v.MarginStatus,
	}
}

// FuturesPositionFromDTO maps one position row. A nil row yields a nil
// position.
func FuturesPositionFromDTO(v *FuturesHoldWire) *FuturesPosition {
	if v == nil {
		return nil
	}
	return &FuturesPosition{
		StockName:         v.StockName,
		StockCode:         v.StockCode,
		DataType:          v.DataType,
		Ccy:               v.Ccy,
		CcyRate:           futuresRate(v.CcyRate),
		ContractValue:     futuresQuantity(v.ContractValue),
		LastDayQty:        futuresQuantity(v.LastDayQty),
		DepQty:            futuresQuantity(v.DepQty),
		DayLongQty:        futuresQuantity(v.DayLongQty),
		DayShortQty:       futuresQuantity(v.DayShortQty),
		DayNetQty:         futuresQuantity(v.DayNetQty),
		CurrentQty:        futuresQuantity(v.CurrentQty),
		LastDayPrice:      futuresPrice(v.LastDayPrice),
		DayLongPrice:      futuresPrice(v.DayLongPrice),
		DayShortPrice:     futuresPrice(v.DayShortPrice),
		DayNetPrice:       futuresPrice(v.DayNetPrice),
		CostPrice:         futuresPrice(v.CostPrice),
		LastPrice:         futuresPrice(v.LastPrice),
		PreClosePrice:     futuresPrice(v.PreClosePrice),
		ProfitLoss:        futuresMoney(v.ProfitLoss, futuresBaseCurrency),
		ProfitLossBaseCcy: futuresMoney(v.ProfitLossBaseCcy, futuresBaseCurrency),
		CloseProfit:       futuresMoney(v.CloseProfit, futuresBaseCurrency),
		CloseProfitHKD:    futuresMoney(v.CloseProfitHKD, futuresCurrencyHKD),
	}
}

// FuturesOrderFromDTO maps one order row. A nil row yields a nil order.
func FuturesOrderFromDTO(v *FuturesOrderWire) *FuturesOrder {
	if v == nil {
		return nil
	}
	return &FuturesOrder{
		StockCode:     v.StockCode,
		StockName:     v.StockName,
		EntrustID:     EntrustID(v.EntrustID),
		Side:          types.EntrustBS(v.EntrustBS),
		OrderType:     v.EntrustType,
		OrderTypeNum:  v.EntrustTypeNum,
		OrderPrice:    futuresPrice(v.EntrustPrice),
		FillPrice:     futuresPrice(v.BusinessPrice),
		Quantity:      futuresQuantity(v.EntrustAmount),
		FilledQty:     futuresQuantity(v.BusinessAmount),
		Status:        types.EntrustStatus(v.Status),
		StatusDesc:    v.StatusDesc,
		ValidType:     v.ValidType,
		ValidTypeDesc: v.ValidTypeDesc,
		ValidTime:     v.ValidTime,
		OrderOptions:  v.OrderOptions,
		Date:          v.Date,
		EntrustTime:   v.EntrustTime,
		BusinessTime:  v.BusinessTime,
		QueryParamStr: v.QueryParamStr,
		IsValid:       v.IsValid == 1,
		CanBeCanceled: v.CanBeCanceled == 1,
		CanBeUpdated:  v.CanBeUpdated == 1,
	}
}

// FuturesFillFromDTO maps one fill row. A nil row yields a nil fill.
func FuturesFillFromDTO(v *FuturesOrderWire) *FuturesFill {
	if v == nil {
		return nil
	}
	return &FuturesFill{
		StockCode:     v.StockCode,
		StockName:     v.StockName,
		EntrustID:     EntrustID(v.EntrustID),
		Side:          types.EntrustBS(v.EntrustBS),
		OrderType:     v.EntrustType,
		OrderTypeNum:  v.EntrustTypeNum,
		Price:         futuresPrice(v.BusinessPrice),
		OrderPrice:    futuresPrice(v.EntrustPrice),
		Quantity:      futuresQuantity(v.EntrustAmount),
		FilledQty:     futuresQuantity(v.BusinessAmount),
		Status:        types.EntrustStatus(v.Status),
		StatusDesc:    v.StatusDesc,
		ValidType:     v.ValidType,
		ValidTypeDesc: v.ValidTypeDesc,
		ValidTime:     v.ValidTime,
		OrderOptions:  v.OrderOptions,
		Date:          v.Date,
		BusinessTime:  v.BusinessTime,
		EntrustTime:   v.EntrustTime,
		QueryParamStr: v.QueryParamStr,
		IsValid:       v.IsValid == 1,
		CanBeCanceled: v.CanBeCanceled == 1,
		CanBeUpdated:  v.CanBeUpdated == 1,
	}
}

// futuresMoney maps a wire money string, treating an absent field as an
// explicit zero.
//
// The empty case is the reason this wrapper exists. MustNewMoney panics on a
// string it cannot parse and the empty string is one, so a partial reply — a
// field the Gateway omits, which a partial *response* of any kind can produce
// — would crash a read in the caller's process rather than report zero. Every
// futures money field is optional in that sense, because none of them is
// required by the Gateway, so the wrapper is applied at every call site rather
// than at the four that matter most.
//
// A non-empty but unparseable value still panics, and that is deliberate: a
// corrupt amount is not a zero, and reporting it as one would be the silent
// wrong-number failure the decimal types exist to prevent.
//
// An empty currency falls back to futuresBaseCurrency, so a reply that omits
// ccy is attributed the same currency as every other unlabelled amount instead
// of producing a Money that panics on any comparison with a real one.
func futuresMoney(value, currency string) Money {
	if currency == "" {
		currency = futuresBaseCurrency
	}
	if value == "" {
		value = "0"
	}
	return MustNewMoney(value, currency, futuresMoneyScale)
}

// futuresQuantity maps a wire quantity string, treating an absent field as an
// explicit zero, for the reason futuresMoney states.
func futuresQuantity(value string) Quantity {
	if value == "" {
		value = "0"
	}
	return MustNewQuantity(value)
}

// futuresPrice maps a wire price string, treating an absent field as an
// explicit zero and carrying the zero tick.
//
// The zero tick is not a claim about this product's grid, and the comment on
// futuresZeroTick explains why the real one is not applied here. An absent
// price is a zero price rather than a panic, on the same terms as futuresMoney.
func futuresPrice(value string) Price {
	if value == "" {
		value = "0"
	}
	return MustNewPrice(value, futuresZeroTick)
}

// futuresRate maps a wire ratio string, treating an absent field as an
// explicit zero, for the reason futuresMoney states.
func futuresRate(value string) Rate {
	if value == "" {
		value = "0"
	}
	return MustNewRate(value)
}

// futuresTickString renders a product's price decimal power as the tick string
// it denotes, deriving the value from the integer and never from the Gateway's
// own string rendering of it.
//
// decInPrice is the source: 0 denotes a whole-unit grid, 1 denotes 0.1, 2
// denotes 0.01. PriceDecimalPoint is the redundant rendering of the same
// number, and a string is exactly the kind of value a caller reaches for
// strconv.ParseFloat on — the conversion this repository removed from 23 sites
// because it cannot represent every decimal it is handed. So the derivation
// walks digits and never a float.
//
// A negative power is not documented on either side. It yields the zero tick
// rather than a number, which makes the result "unknown" — no step check, no
// rounding — instead of a confidently wrong grid, and it also keeps the digit
// loop's bound total.
//
// A power of 0 is a legitimate whole-unit grid, not an unset value, and yields
// "1"; conflating the two would reject every whole-unit product's price against
// an invented grid.
//
// No upper bound is enforced. A very large power produces a very long string,
// which is the Gateway's problem to have sent; bounding it is the tick
// schedule's decision, not this function's.
func futuresTickString(decInPrice int32) string {
	if decInPrice < 0 {
		return futuresZeroTick
	}
	if decInPrice == 0 {
		return "1"
	}
	var b strings.Builder
	b.WriteString("0.")
	// decInPrice counts the decimal places, and the last one is the significant
	// digit: a power of 3 is three places, so two zeros then a 1.
	for i := int32(1); i < decInPrice; i++ {
		b.WriteByte('0')
	}
	b.WriteByte('1')
	return b.String()
}
