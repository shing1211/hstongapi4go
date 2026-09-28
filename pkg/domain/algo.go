// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the algorithm-trading (算法交易) half of the domain model: the four
// dictionaries that belong to the algo surface, the two that belong to the
// strategy bound to a master order, the row payload a master-order query returns,
// the model it maps to, and the value wrappers every decimal on the path is read
// through.
//
// # The four dictionaries, and why each one is its own type
//
// The released pkg/hstong/algo declares four dictionaries that no other surface
// declares, and three of them share a *name* with a type this module already
// exports for a different meaning:
//
//   - AlgoEntrustType — 1 limit, 2 market. types.EntrustType uses 1 for auction,
//     2 for enhanced limit, 3 for limit and 5 for market, so the same wire code
//     means something else on each surface.
//   - AlgoStatus — the master order's lifecycle. types.EntrustStatus uses 0 for
//     未报 and 2 for 已报; the algo set uses 0 for 已报 and 2 for 全部成交. The
//     two sets disagree on the *direction* of the most common codes.
//   - AlgoSessionType — 0 off, 1 on. The trade surface's sessionType carries
//     three further codes (3, 5, 7) for conditional orders that an algo master
//     order cannot use.
//   - AlgoSensitivity — 1 neutral, 2 aggressive, 3 passive. No namesake, and it
//     is here anyway: it is an algo-only vocabulary, and giving it a name is what
//     makes "which set is this" answerable at the call site.
//
// Each is declared as a distinct named type in this file rather than reused from
// pkg/types, and none of them is an alias (`type X = Y`) of a trade type. An
// alias would be the worse mistake of the two, because it compiles: the value
// sets would be interchangeable at every call site and the compiler would say
// nothing. TestAlgoDictionariesAreNotTheirTradeNamesakes in
// pkg/domain/algo_test.go is the deliberate assertion that catches it, since a
// compiler cannot.
//
// # What is *not* declared here
//
// Direction reuses types.EntrustBS and the market reuses types.ExchangeType,
// because on those two the surfaces agree: SPEC §7.3 and §7.4 are flat four-row
// tables, both vendor SDKs spell the codes the same way, and pkg/hstong/future
// already refuses a direction outside {1,2,3,4}. Reusing them is what makes an
// algo order and a cash order comparable to a caller; declaring a fourth copy
// would create a second source of truth for a set that is not in dispute.
//
// The strategy code (AlgoStrategy) is the one dictionary the SDK deliberately
// does *not* bound, and AlgoAction and AlgoStrategyStatus are the two the
// vendors and the released layer agree on. Each of those decisions is stated on
// the type it belongs to.

const (
	// algoZeroTick is the tick every algo Price is built with.
	//
	// "0" is the tick note's rule for a price whose instrument tick schedule is
	// not known: I observed this price, I do not know this product's tick. It is
	// not a claim that the tick is zero — Price.Validate skips its step check
	// when the tick is zero and Price.Round is a no-op, so nothing is checked
	// against a grid this SDK invented and nothing is rounded.
	//
	// The same reasoning and the same value as futuresZeroTick, deliberately
	// repeated rather than shared: an algo price grid is per-security exactly as
	// a futures one is per-product, and one constant serving two schedules would
	// let a change to either silently move the other. A HK stock's cash-layer
	// grid is a fixed thousandth place (domain.DefaultHKTickSchedule), which the
	// tick note records for removal; applying that here would measure every algo
	// price against a cash-layer grid that says nothing about algorithm orders.
	algoZeroTick = "0"
	// algoBaseCurrency is attributed to an algo amount the Gateway does not label
	// with a currency. Only minAmount is one, and it is a minimum *traded amount*
	// in the security's own currency, which the reply never states.
	//
	// It is an assumption, the same one the cash and futures layers make for
	// their equally unlabelled amounts, and it is stated rather than hidden: a
	// caller can read Money.Currency() and check it. The failure direction is the
	// loud one, because Money.Add and Sub panic across currencies — an amount
	// denominated in something else gets a panic on a cross-currency sum instead
	// of a silently wrong number. An algo order trades on four markets and the
	// wire carries no currency field at all, so no better answer exists here.
	algoBaseCurrency = "HKD"
	// algoMoneyScale is the decimal scale recorded on every algo Money. It
	// matches the cash and futures scales and ADR 0008's HKD row. MustNewMoney
	// records the scale without rounding, so no algo digit is discarded by
	// choosing it.
	algoMoneyScale uint8 = 3
)

// AlgoEntrustType is the algorithm order type: 1 for a limit order (限价单) and 2
// for a market order (市价单).
//
// It is a distinct type from types.EntrustType and the two must never be mixed.
// The trade dictionary uses 1 for auction (竞价), 2 for enhanced limit (增强限价
// 盘), 3 for limit and 5 for market, so "1" is a limit order here and an auction
// there, and a caller who reached for types.EntrustTypeLimit ("3") would be
// sending a code the algo surface does not accept at all. The released
// pkg/hstong/algo says the same thing in its own words and is the reason this is
// a separate declaration rather than a reuse.
//
// The set is closed. All three sources in this repository agree on two values: the
// released layer, the Java AlgoEntrustParam field comment (委托类型 [1:限价单
// 2:市价单]) and the Python AlgoEntrustType class (LIMIT "1", MARKET "2"). An
// unlisted value is refused before a request is sent rather than forwarded. If
// the vendor adds a code, this constant group and Valid must be updated together.
type AlgoEntrustType string

const (
	// AlgoEntrustTypeLimit ("1") is a limit order (限价单).
	AlgoEntrustTypeLimit AlgoEntrustType = "1"
	// AlgoEntrustTypeMarket ("2") is a market order (市价单).
	AlgoEntrustTypeMarket AlgoEntrustType = "2"
)

// Valid reports whether t is one of the two documented algorithm order types. It
// is the gate the endpoint method uses before it builds a request, and a code
// outside the set never reaches the Gateway.
func (t AlgoEntrustType) Valid() bool {
	return t == AlgoEntrustTypeLimit || t == AlgoEntrustTypeMarket
}

// AlgoSessionType controls pre-market and after-hours trading on a master order:
// 0 for off (否) and 1 for on (是).
//
// It is a distinct type from the trade surface's sessionType, which the Python
// SessionType class documents as five codes — 0 and 1 for a normal order and 3, 5
// and 7 for a conditional order. **The two vendors disagree about this field and
// the disagreement is recorded rather than resolved**: the Java AlgoEntrustParam
// comment and the Python algo_entrust docstring both list exactly two values for
// the algo field (盘前盘后交易 [0:否 1:是]), while the Python class that docstring
// points at carries five. An algorithm master order is not a conditional order,
// so the three extra codes have no meaning here; this set is the two the algo
// documentation states in both vendors, and a code outside it is refused locally.
// A caller who wants a conditional order is using the trade surface, which
// declares the wider set where it belongs.
type AlgoSessionType string

const (
	// AlgoSessionTypeOff ("0") disables pre-market and after-hours trading.
	AlgoSessionTypeOff AlgoSessionType = "0"
	// AlgoSessionTypeOn ("1") enables pre-market and after-hours trading.
	AlgoSessionTypeOn AlgoSessionType = "1"
)

// Valid reports whether s is one of the two documented algo session codes.
func (s AlgoSessionType) Valid() bool {
	return s == AlgoSessionTypeOff || s == AlgoSessionTypeOn
}

// AlgoSensitivity is the execution aggressiveness a strategy is asked for: 1
// neutral (中性), 2 aggressive (主动), 3 passive (被动).
//
// It is algo-only — no other surface in this module has the field — and it is a
// named type rather than a bare string so that a caller reading a request
// builder can tell which vocabulary a code belongs to without leaving the line.
// The released layer and both vendors agree on exactly these three values (the
// Java AlgoStrategy comment, the Python AlgoStrategySensitivityType class and the
// Python algo_entrust docstring), so the set is closed and Valid is the gate.
type AlgoSensitivity string

const (
	// AlgoSensitivityNeutral ("1") is neutral execution.
	AlgoSensitivityNeutral AlgoSensitivity = "1"
	// AlgoSensitivityAggressive ("2") is aggressive execution.
	AlgoSensitivityAggressive AlgoSensitivity = "2"
	// AlgoSensitivityPassive ("3") is passive execution.
	AlgoSensitivityPassive AlgoSensitivity = "3"
)

// Valid reports whether s is one of the three documented sensitivity codes.
func (s AlgoSensitivity) Valid() bool {
	switch s {
	case AlgoSensitivityNeutral, AlgoSensitivityAggressive, AlgoSensitivityPassive:
		return true
	default:
		return false
	}
}

// AlgoStatus is the lifecycle state of an algorithm master order as reported by
// the master-order query.
//
// It is a distinct type from types.EntrustStatus and the two sets disagree
// about the most common codes, which is the whole reason they cannot be one
// type: trade 0 is 未报 (not yet submitted) and algo 0 is 已报 (accepted); trade 2
// is 已报 and algo 2 is 全部成交 (fully filled). A caller who read a cash order's
// status and applied it to a master order would invert both.
//
// **This set has one source in this repository and no vendor corroboration.** The
// released pkg/hstong/algo enumerates eleven codes citing the algorithm
// documentation (query-algo-master-order.html); the Java AlgoOrder declares
// `status` as a bare String with the comment 状态 and no enumeration, and the
// Python SDK returns the master-order reply as an untyped dict with no algo
// response DTO at all. The eleven codes are therefore carried as published by
// the reference and are *not* validated on the way in, because they arrive on a
// reply — see AlgoMasterOrderFromDTO and the note on not validating inbound data.
//
// Two gaps in the published list are deliberate and match the reference: 7 is
// absent, and so is every code past "E" in the alphabetical tail the reference
// stops at. Nothing here asserts the set is complete.
type AlgoStatus string

const (
	// AlgoStatusRegistered ("0") is 已报, accepted by the platform.
	AlgoStatusRegistered AlgoStatus = "0"
	// AlgoStatusPartFilled ("1") is 部分成交, partially filled.
	AlgoStatusPartFilled AlgoStatus = "1"
	// AlgoStatusFilled ("2") is 全部成交, fully filled.
	AlgoStatusFilled AlgoStatus = "2"
	// AlgoStatusCompleted ("3") is 当日完成, completed for the day.
	AlgoStatusCompleted AlgoStatus = "3"
	// AlgoStatusCancelled ("4") is 已撤, cancelled.
	AlgoStatusCancelled AlgoStatus = "4"
	// AlgoStatusModified ("5") is 已改, modified.
	AlgoStatusModified AlgoStatus = "5"
	// AlgoStatusWaitCancel ("6") is 待撤, waiting to cancel.
	AlgoStatusWaitCancel AlgoStatus = "6"
	// AlgoStatusRejected ("8") is 废单, rejected.
	AlgoStatusRejected AlgoStatus = "8"
	// AlgoStatusPending ("A") is 待报, waiting to be submitted.
	AlgoStatusPending AlgoStatus = "A"
	// AlgoStatusExpired ("C") is 过期, expired.
	AlgoStatusExpired AlgoStatus = "C"
	// AlgoStatusWaitModify ("E") is 待改, waiting to modify.
	AlgoStatusWaitModify AlgoStatus = "E"
)

// AlgoStrategyStatus is the run state of the strategy bound to a master order.
//
// It shares its four codes with AlgoAction, and both vendor SDKs say so: the
// Java AlgoOrder documents strategyStatus as [1:START 2:STOP 3:SUSPEND 4:RESUME],
// using the action vocabulary, and the released layer notes the same. It is still
// a separate type, because the same four numbers answer a different question on
// the two fields — "what was asked" on an action and "what is running" on a
// strategy status — and a caller who could pass one for the other would be able
// to request a resume of something already running.
//
// As with AlgoStatus it arrives on a reply and is never validated on the way in.
type AlgoStrategyStatus string

const (
	// AlgoStrategyStatusStart ("1") is a started strategy.
	AlgoStrategyStatusStart AlgoStrategyStatus = "1"
	// AlgoStrategyStatusStop ("2") is a stopped strategy.
	AlgoStrategyStatusStop AlgoStrategyStatus = "2"
	// AlgoStrategyStatusSuspend ("3") is a suspended strategy.
	AlgoStrategyStatusSuspend AlgoStrategyStatus = "3"
	// AlgoStrategyStatusResume ("4") is a resumed strategy.
	AlgoStrategyStatusResume AlgoStrategyStatus = "4"
)

// AlgoStrategy is the execution algorithm bound to a master order
// (targetStrategy).
//
// **This is the one dictionary this module refuses to bound, and it is the
// released layer's decision carried forward rather than a new one.** The
// released pkg/hstong/algo records the reason: the reference's AddOrder parameter
// table documents '1005' as POV while its request examples and its master-order
// query label the same code INLINE, and '1003' appears only in the examples.
// Both vendor SDKs at v2.3.0 resolve the second half of that — both label 1005
// INLINE and both list 1003 TPOV — and neither resolves the first, because the
// table they were written from is the table that disagrees with itself. A closed
// set here would refuse a code the vendor's own documentation endorses under a
// different name, so only emptiness is checked locally and an unknown code is
// forwarded unchanged.
//
// The constant below keeps the released layer's name for 1005, POV, so a caller
// migrating from v0.x finds the same identifier. **The vendors call that code
// INLINE; the disagreement is recorded and deliberately not resolved here.**
//
// The remaining codes are uncontested: VWAP "1", TWAP "1001" and ICE_BERG "1002"
// carry the same names in all three sources.
type AlgoStrategy string

const (
	// AlgoStrategyVWAP ("1") is the volume-weighted average price algorithm.
	AlgoStrategyVWAP AlgoStrategy = "1"
	// AlgoStrategyTWAP ("1001") is the time-weighted average price algorithm.
	AlgoStrategyTWAP AlgoStrategy = "1001"
	// AlgoStrategyIceberg ("1002") is the ICE_BERG incremental-display algorithm.
	AlgoStrategyIceberg AlgoStrategy = "1002"
	// AlgoStrategyTPOV ("1003") is the TPOV participation-of-volume algorithm.
	// The released layer notes it is absent from the AddOrder parameter table and
	// appears only in the request examples; both vendors list it, so it is carried
	// here.
	AlgoStrategyTPOV AlgoStrategy = "1003"
	// AlgoStrategyPOV ("1005") is the fifth documented strategy. The released
	// layer names it POV from the AddOrder parameter table; both vendor SDKs name
	// the same code INLINE. See the type comment: the set is not bounded and the
	// naming disagreement is not resolved by this module.
	AlgoStrategyPOV AlgoStrategy = "1005"
)

// AlgoAction is an operation submitted to the master-order action endpoint to
// control a live algorithm order: start, stop, suspend or resume
// (算法订单的启动、停止、暂停、恢复).
//
// The set is closed at four. All three sources agree: the released
// pkg/hstong/algo, the Java AlgoActionParam comment (操作 [1:START 2:STOP
// 3:SUSPEND 4:RESUME]) and the Python AlgoActionType class. An unlisted value is
// refused before a request is sent rather than forwarded, because an action the
// SDK does not recognise on a live order is a request it already knows it does
// not mean. If the vendor extends the set, this group and Valid must be updated
// together.
type AlgoAction string

const (
	// AlgoActionStart ("1") starts a stopped or newly created strategy.
	AlgoActionStart AlgoAction = "1"
	// AlgoActionStop ("2") stops the strategy, cancelling its outstanding
	// children.
	AlgoActionStop AlgoAction = "2"
	// AlgoActionSuspend ("3") pauses the strategy without cancelling its
	// children.
	AlgoActionSuspend AlgoAction = "3"
	// AlgoActionResume ("4") resumes a suspended strategy.
	AlgoActionResume AlgoAction = "4"
)

// Valid reports whether a is one of the four documented action codes. It is the
// gate the action endpoint method uses, and an invalid action never reaches the
// Gateway.
func (a AlgoAction) Valid() bool {
	switch a {
	case AlgoActionStart, AlgoActionStop, AlgoActionSuspend, AlgoActionResume:
		return true
	default:
		return false
	}
}

// String returns the documented action name — "START", "STOP", "SUSPEND" or
// "RESUME" — or "AlgoAction(<raw>)" for a value outside the documented set. It
// never panics and never returns an empty string for a non-empty code, so a log
// line built from an unrecognised code says what the code was.
func (a AlgoAction) String() string {
	switch a {
	case AlgoActionStart:
		return "START"
	case AlgoActionStop:
		return "STOP"
	case AlgoActionSuspend:
		return "SUSPEND"
	case AlgoActionResume:
		return "RESUME"
	default:
		return "AlgoAction(" + string(a) + ")"
	}
}

// AlgoStrategyParamWire is the strategyParam object the Gateway nests inside a
// master-order row and accepts on the two order mutations.
//
// It is a row, not an envelope: the object appears inside another payload, so it
// belongs here beside the mappers that read it rather than in pkg/services with
// the envelopes. Every monetary and quantitative member is a quoted string and
// Interval is the only number, which is a whole number of seconds.
type AlgoStrategyParamWire struct {
	OrigStartTime string `json:"origStartTime"`
	OrigEndTime   string `json:"origEndTime"`
	MaxVolume     string `json:"maxVolume"`
	MinAmount     string `json:"minAmount"`
	Sensitivity   string `json:"sensitivity"`
	ShowQty       string `json:"showQty"`
	QtyPercent    string `json:"qtyPercent"`
	Interval      int32  `json:"interval"`
}

// AlgoMasterOrderWire is one master-order row of a /trade/AlgoQueryOrderList
// reply. Every price, quantity and amount is a quoted string; none of them is a
// number on the wire, which is what lets a caller read them back byte for byte.
//
// EntrustBS is a types.EntrustBS and ExchangeType a types.ExchangeType because
// the algo surface and the trade surface agree on both of those dictionaries;
// EntrustType, Status, TargetStrategy and StrategyStatus are algo types because
// they are the three that disagree.
type AlgoMasterOrderWire struct {
	OrderID         string                `json:"orderId"`
	StockCode       string                `json:"stockCode"`
	ExchangeType    types.ExchangeType    `json:"exchangeType"`
	TradeDate       string                `json:"tradeDate"`
	EntrustType     string                `json:"entrustType"`
	EntrustPrice    string                `json:"entrustPrice"`
	EntrustAmount   string                `json:"entrustAmount"`
	CumQty          string                `json:"cumQty"`
	LeavesQty       string                `json:"leavesQty"`
	Status          string                `json:"status"`
	EntrustBS       types.EntrustBS       `json:"entrustBs"`
	TargetStrategy  string                `json:"targetStrategy"`
	StrategyStatus  string                `json:"strategyStatus"`
	StrategyParam   AlgoStrategyParamWire `json:"strategyParam"`
	RoundLot        string                `json:"roundLot"`
	SendingTime     string                `json:"sendingTime"`
	TransactionTime string                `json:"transactionTime"`
	AvgPx           string                `json:"avgPx"`
}

// AlgoStrategyParam is the per-strategy tuning object: the window the strategy
// runs in, the size of each child order it slices the master into, and how
// aggressively it slices.
//
// Every monetary and quantitative member is a decimal type read verbatim from the
// wire; Interval is the only plain integer and is a whole number of seconds.
type AlgoStrategyParam struct {
	// OrigStartTime is the strategy start time in Hong Kong time, HHmmSS. Empty
	// means the exchange open, or the current system time once open.
	OrigStartTime string
	// OrigEndTime is the strategy end time in Hong Kong time, HHmmSS. Empty
	// means the exchange close.
	OrigEndTime string
	// MaxVolume is the largest order each child may place. It is a quantity and
	// not an amount.
	MaxVolume Quantity
	// MinAmount is the smallest traded amount each child may place. It is money
	// in the security's own currency, which the wire never states, so it is
	// attributed the base currency; see the note on algoBaseCurrency and read
	// Money.Currency() before summing it against another market's amount.
	MinAmount Money
	// Sensitivity is the execution aggressiveness the strategy is asked for.
	Sensitivity AlgoSensitivity
	// ShowQty is the displayed quantity of each ICE_BERG child order.
	ShowQty Quantity
	// QtyPercent is the participation percentage the POV and TPOV strategies aim
	// at, carried as the Gateway's own number rather than as a fraction: a Rate
	// holding 10 means ten per cent, exactly as a futures margin level of "2.50"
	// means 250 per cent.
	QtyPercent Rate
	// Interval is the child-order interval in whole seconds. Zero means the
	// documented default, which is 60 in the reference and in the Java SDK's
	// comment. The Python SDK's function signature defaults it to 64 while its
	// own docstring says 60; the disagreement is recorded, not resolved, and it
	// cannot reach a request from here, because zero is omitted from the body
	// and the Gateway then applies its own default.
	Interval int32
}

// AlgoMasterOrder is one algorithm master order as the Gateway reports it: a
// single order the platform slices into child orders (子单) according to the
// bound strategy.
//
// OrderID is the master identifier and is the key every other algo call takes. A
// child is addressed by the pair (OrderID, EntrustID), and cancelling the master
// does not use a child id while cancelling a child does not touch the master's
// remaining schedule.
//
// Inbound dictionaries are **not** validated. AlgoStatus, AlgoStrategyStatus and
// AlgoEntrustType arrive on a reply, and turning a vendor-side surprise into a
// decode error for the caller is a different policy from validating an outbound
// request; a caller is handed the raw code in its named type and can switch on
// what it recognises. An out-of-set value therefore surfaces as an unrecognised
// constant, not as a failed read.
type AlgoMasterOrder struct {
	// OrderID is the master order identifier (母单ID).
	OrderID OrderID
	// StockCode is the security code; a Hong Kong code carries a ".HK" suffix.
	StockCode string
	// ExchangeType is the market the order trades on.
	ExchangeType types.ExchangeType
	// TradeDate is the order's trade date.
	TradeDate string
	// EntrustType is the algorithm order type; see AlgoEntrustType.
	EntrustType AlgoEntrustType
	// EntrustPrice is the limit price, carrying the zero tick; see algoZeroTick.
	EntrustPrice Price
	// EntrustAmount is the total ordered quantity.
	EntrustAmount Quantity
	// CumQty is the cumulative filled quantity (累计成交数量).
	CumQty Quantity
	// LeavesQty is the remaining unfilled quantity (剩余数量).
	LeavesQty Quantity
	// Status is the master's lifecycle state; see AlgoStatus.
	Status AlgoStatus
	// EntrustBS is the direction, as types.EntrustBS — the one dictionary the
	// algo and trade surfaces share.
	EntrustBS types.EntrustBS
	// TargetStrategy is the bound execution algorithm; see AlgoStrategy.
	TargetStrategy AlgoStrategy
	// StrategyStatus is the strategy's run state; see AlgoStrategyStatus.
	StrategyStatus AlgoStrategyStatus
	// StrategyParam is the tuning the master is running with.
	StrategyParam AlgoStrategyParam
	// RoundLot is the security's board lot size, a count rather than an amount.
	RoundLot Quantity
	// SendingTime is the transport timestamp.
	SendingTime string
	// TransactionTime is the order timestamp.
	TransactionTime string
	// AvgPx is the average execution price, carrying the zero tick.
	AvgPx Price
}

// AlgoMasterOrderFromDTO maps one master-order row. A nil row yields a nil order,
// so the mapping is safe to call over a reply whose list is absent.
//
// Every decimal is read through a wrapper that turns an absent field into an
// explicit zero rather than panicking, for the reason futuresMoney states: a
// partial reply is a read, and a read must not crash the caller's process. A
// non-empty but unparseable amount still panics, which is deliberate — a corrupt
// amount is not a zero, and reporting it as one is the silent wrong-number
// failure the decimal types exist to prevent.
func AlgoMasterOrderFromDTO(v *AlgoMasterOrderWire) *AlgoMasterOrder {
	if v == nil {
		return nil
	}
	return &AlgoMasterOrder{
		OrderID:         OrderID(v.OrderID),
		StockCode:       v.StockCode,
		ExchangeType:    v.ExchangeType,
		TradeDate:       v.TradeDate,
		EntrustType:     AlgoEntrustType(v.EntrustType),
		EntrustPrice:    algoPrice(v.EntrustPrice),
		EntrustAmount:   algoQuantity(v.EntrustAmount),
		CumQty:          algoQuantity(v.CumQty),
		LeavesQty:       algoQuantity(v.LeavesQty),
		Status:          AlgoStatus(v.Status),
		EntrustBS:       v.EntrustBS,
		TargetStrategy:  AlgoStrategy(v.TargetStrategy),
		StrategyStatus:  AlgoStrategyStatus(v.StrategyStatus),
		StrategyParam:   AlgoStrategyParamFromDTO(&v.StrategyParam),
		RoundLot:        algoQuantity(v.RoundLot),
		SendingTime:     v.SendingTime,
		TransactionTime: v.TransactionTime,
		AvgPx:           algoPrice(v.AvgPx),
	}
}

// AlgoStrategyParamFromDTO maps the nested strategyParam object.
//
// It takes a pointer and returns a value, which is the opposite of every mapper
// beside it and is deliberate: strategyParam is nested inside a master-order row,
// so there is no nil to propagate and a value keeps the caller's field
// non-nilable. A nil argument yields the zero tuning, whose decimals are all
// zeros rather than panics, on the same terms as the rest of this file.
func AlgoStrategyParamFromDTO(v *AlgoStrategyParamWire) AlgoStrategyParam {
	if v == nil {
		return AlgoStrategyParam{}
	}
	return AlgoStrategyParam{
		OrigStartTime: v.OrigStartTime,
		OrigEndTime:   v.OrigEndTime,
		MaxVolume:     algoQuantity(v.MaxVolume),
		MinAmount:     algoMoney(v.MinAmount),
		Sensitivity:   AlgoSensitivity(v.Sensitivity),
		ShowQty:       algoQuantity(v.ShowQty),
		QtyPercent:    algoRate(v.QtyPercent),
		Interval:      v.Interval,
	}
}

// algoPrice maps a wire price string, treating an absent field as an explicit
// zero and carrying the zero tick.
//
// The zero tick is not a claim about this security's grid, and the comment on
// algoZeroTick explains why the real one is not applied: the algo rows carry no
// tick schedule and inventing one would measure every price against a grid this
// SDK made up. An absent price is a zero price rather than a panic, on the same
// terms as algoMoney.
func algoPrice(value string) Price {
	if value == "" {
		value = "0"
	}
	return MustNewPrice(value, algoZeroTick)
}

// algoQuantity maps a wire quantity string, treating an absent field as an
// explicit zero, for the reason algoMoney states.
func algoQuantity(value string) Quantity {
	if value == "" {
		value = "0"
	}
	return MustNewQuantity(value)
}

// algoMoney maps a wire amount string, treating an absent field as an explicit
// zero and attributing the base currency.
//
// The empty case is the reason this wrapper exists. MustNewMoney panics on a
// string it cannot parse and the empty string is one, so a partial reply — a field
// the Gateway omits, which a partial response of any kind can produce — would
// crash a read in the caller's process rather than report zero.
func algoMoney(value string) Money {
	if value == "" {
		value = "0"
	}
	return MustNewMoney(value, algoBaseCurrency, algoMoneyScale)
}

// algoRate maps a wire ratio string, treating an absent field as an explicit
// zero, for the reason algoMoney states.
func algoRate(value string) Rate {
	if value == "" {
		value = "0"
	}
	return MustNewRate(value)
}
