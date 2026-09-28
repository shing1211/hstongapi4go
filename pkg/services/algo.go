// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"strings"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file owns the seven algorithm-trading endpoints: five mutations and two
// reads. The rows inside the reply envelopes and the domain models they map to
// live in pkg/domain, which is the split account.go established and futures.go
// followed; an envelope that holds rows, and every request body, stays unexported
// here.
//
// # Every request carries a market, and none carries an account id
//
// Those two facts are the opposite of the futures layer's, and both are load
// bearing. Unlike a futures contract, an algo security code is not unique across
// the books — the same equity code exists in the Hong Kong and the US book — and
// both vendor SDKs put exchangeType on every one of the seven bodies
// (ExchangeTypeParam is the base of the Java parameter hierarchy; every Python
// literal has the key). An algo request that omitted the market would be a
// request the Gateway has to guess at, and exchangeType on a cancel names the
// book the master is resolved against.
//
// The account is the other way round. Neither vendor puts an account identifier
// on an algo body and the released pkg/hstong/algo takes no account argument, so
// the accountID these methods take is a local precondition: it is validated and
// never placed in a wire struct. It must not be added to one; that would leak an
// account identifier into every algo request.

// Operation labels used in errors raised by AlgoService. They are the canonical
// Gateway route paths, with the same convention the released pkg/hstong/algo
// Manager uses, and they never carry a request payload or a credential.
//
// The names carry an Algo prefix because this package already declares
// opEntrust and opCancelEntrust for the cash endpoints, and algo's cancel is a
// different route with a different body: an algo CancelEntrust names a *child* of
// a master, while the cash one names a standalone order.
const (
	opAlgoAddOrder           = "trade/AlgoAddOrder"
	opAlgoCancelOrder        = "trade/AlgoCancelOrder"
	opAlgoCancelEntrust      = "trade/AlgoCancelEntrust"
	opAlgoChangeOrder        = "trade/AlgoChangeOrder"
	opAlgoActionOrder        = "trade/AlgoActionOrder"
	opAlgoQueryOrderList     = "trade/AlgoQueryOrderList"
	opAlgoQueryEntrustIDList = "trade/AlgoQueryEntrustIdList"
)

const (
	// algoDefaultPageNo is the first page a master-order query asks for when the
	// caller leaves PageRequest.PageNo at zero.
	algoDefaultPageNo = 1
	// algoDefaultPageSize is the page size a master-order query asks for when the
	// caller leaves PageRequest.PageSize at zero.
	//
	// It is 30, not the futures layer's 20, and it is not a shared constant
	// because the two are different documented defaults: the Java
	// AlgoQueryPageParam comment reads 每页条数 默认30 and the Python
	// algo_query_order_list signature defaults page_size to 30. A caller
	// migrating from the released pkg/hstong/future would see 20 there, and
	// sharing one identifier would let a change to either silently move the
	// other.
	//
	// Unlike the futures page there is deliberately no upper bound. Nothing in
	// the reference, the released layer, or either vendor SDK documents a
	// maximum algo page size, and the futures bound of "strictly below 100" is
	// the futures endpoint's own documented limit. Inventing one here would
	// refuse a page the Gateway accepts, which is the failure direction A6
	// argues against; a caller who wants a bound has a vendor to ask.
	algoDefaultPageSize = 30
	// algoDateLayout is the Go reference-time layout for the yyyyMMdd dates every
	// algo body takes.
	algoDateLayout = "20060102"
	// algoTimeLayout is the layout for the HHmmSS strategy window. It exists to
	// validate a shape, and a shape is all the vendors document.
	algoTimeLayout = "150405"
)

// algoQtyPercentMaxDigits is the number of decimal digits a whole percentage in
// the documented 1-99 range can have. It is what makes the range check possible
// without a float and without importing the decimal library into this package:
// a whole number in 1-99 renders as one or two ASCII digits and nothing else, so
// "at most two digits" and "at least one digit that is not a zero" between them
// bound the value at 99 from above and at 1 from below.
//
// Deriving the bound from the digit count rather than comparing to "99" is what
// keeps it exact. A float comparison of 99.0 would be fine here, but the point of
// the check is that this package never routes a financial value through a float,
// and a bounds check is a financial value like any other.
const algoQtyPercentMaxDigits = 2

// AlgoService is the use-case surface for the seven algorithm-trading endpoints.
//
// It holds no per-request state, is safe for concurrent use once constructed,
// and takes its request path from an Executor the caller supplies — the same
// inversion MarketService, AccountService, TradingService and FuturesService use
// (docs/adr/0010-vnext-layered-architecture.md). All seven endpoints are
// implemented here: the two reads, and the five mutations AddOrder, CancelOrder,
// CancelEntrust, ChangeOrder and ActionOrder.
//
// Every method takes a domain.AccountID first, validates it, and never places it
// in a request body — see the account rule at the head of this file.
//
// The five mutations are never retried, at any configuration, by any option. The
// guarantee is structural: client.Client.execute derives the retry class from the
// route path and internal/resilience.IsMutation answers for the five algo paths,
// so it is AlgoService's job to *prove* the guarantee and not to implement it;
// see TestAlgoMutationsAreSentExactlyOnceUnderARetryPolicy and
// TestAlgoReadsRetryUnderARetryPolicy, the control that makes the single request
// in the first a decision rather than a default. Nothing in this package may add
// a retry, a backoff loop, or a "try once more on timeout" of its own
// (docs/adr/0003-no-auto-retry-orders.md).
//
// Four facts about the algo protocol are structural and are stated here because
// each is a mistake a reader would otherwise make, and none of them can be
// checked by the mock Gateway (it answers by path and never inspects a request
// body):
//
//   - **A master order and a child entrust are different things.** AddOrder creates
//     a master and returns its order ID; the platform then slices it into child
//     entrusts. CancelOrder cancels the whole master. CancelEntrust cancels one
//     child and returns the *child's* entrust ID, which is the one place among the
//     five mutations where the identifier in the reply is not a master order ID.
//     A child is addressed by the pair (OrderID, EntrustID).
//   - **A modify is not a cancel followed by a new order.** AlgoChangeOrder is one
//     request; the order never leaves the book and its priority survives. Under
//     ADR 0003 that is the decisive row: a cancel-then-add pair is two
//     unreconciled mutations, and if the cancel succeeded while the add timed out
//     the SDK cannot tell the caller which.
//   - **Four dictionaries on this surface mean something other than their trade
//     namesakes.** domain.AlgoEntrustType, domain.AlgoStatus, domain.AlgoSessionType
//     and domain.AlgoSensitivity are separate types with separate value sets; see
//     pkg/domain/algo.go for the code-by-code comparison. Direction
//     (types.EntrustBS) and market (types.ExchangeType) *are* shared, because the
//     surfaces agree on those two.
//   - **Every algo price and quantity crosses the wire as a quoted string**, and
//     every one of them is read back through a wrapper that turns an absent field
//     into an explicit zero rather than a panic, so a partial reply cannot crash
//     a read in the caller's process. The price tick is the zero tick for the
//     reason on domain.algoZeroTick: no algo row carries a tick schedule.
type AlgoService struct {
	client Executor
}

// AlgoOption configures an AlgoService. Options are applied in order on top of the
// client passed to NewAlgoService, and the last option that sets the request path
// wins.
type AlgoOption func(*AlgoService)

// WithAlgoClient sets the request path an AlgoService issues its calls through.
// It is the same option shape the other four services expose, and it exists so a
// caller (or a test) can supply something other than a live client.
func WithAlgoClient(c Executor) AlgoOption {
	return func(s *AlgoService) { s.client = c }
}

// NewAlgoService returns an AlgoService over c with opts applied in order. c is
// not owned by the service: closing the underlying client remains the caller's
// responsibility. Every algo endpoint requires a live trade session, so a request
// issued before one is established is rejected by the Gateway rather than by this
// constructor.
//
// A nil option panics, exactly as it does in NewFuturesService and
// NewTradingService; the difference is deliberate rather than accidental and is
// the one place where this constructor is not total.
func NewAlgoService(c Executor, opts ...AlgoOption) *AlgoService {
	s := &AlgoService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AlgoStrategyParam is the caller-facing per-strategy tuning object, accepted by
// AddOrder and ChangeOrder.
//
// The three decimal members are domain types, so a caller cannot lose a digit
// between composing a strategy and this struct receiving it. AddOrder requires
// MaxVolume and Sensitivity; ChangeOrder requires neither and treats every member
// as optional, which is why the two methods document their own requirements
// rather than this type carrying them.
//
// The times are HHmmSS in Hong Kong time. Interval is a whole number of seconds
// and zero means the Gateway's own default, so the key is omitted rather than sent
// as a zero the Gateway would have to recognise.
type AlgoStrategyParam struct {
	// OrigStartTime is the strategy start time in Hong Kong time, HHmmSS. Empty
	// means the exchange open, or the current system time once open.
	OrigStartTime string
	// OrigEndTime is the strategy end time in Hong Kong time, HHmmSS. Empty
	// means the exchange close.
	OrigEndTime string
	// MaxVolume is the largest order each child may place. AddOrder requires it
	// and it must be strictly positive; ChangeOrder leaves it optional and, when
	// supplied, still requires it to be positive.
	MaxVolume domain.Quantity
	// MinAmount is the smallest traded amount each child may place, in the
	// security's own currency. The wire field carries a bare decimal and no
	// currency, so the currency is local metadata that is never transmitted: a
	// caller should pass the order's own currency so that reading the value back
	// says what it meant, and must not expect the Gateway to be told.
	MinAmount domain.Money
	// Sensitivity is the execution aggressiveness, as domain.AlgoSensitivity.
	// AddOrder requires it; ChangeOrder validates it when supplied.
	Sensitivity domain.AlgoSensitivity
	// ShowQty is the displayed quantity of each ICE_BERG child order. Empty
	// means the strategy's own default.
	ShowQty domain.Quantity
	// QtyPercent is the participation percentage the POV and TPOV strategies aim
	// at, as the Gateway's own number: 10 means ten per cent, not ten percent of
	// one. The reference documents a range of 1-99, and both vendors repeat it,
	// so a supplied value outside that range is refused. The range is checked
	// rather than assumed because it is stated identically in three places and no
	// fourth source contradicts it.
	QtyPercent domain.Rate
	// Interval is the child-order interval in whole seconds. Zero omits the key
	// and lets the Gateway apply its documented default of 60.
	Interval int32
}

// AlgoAddOrderRequest is the caller-facing body of an algo add: it places one
// algorithm master order and the platform slices it into children.
//
// Its money is already domain-typed: Price and Quantity are decimal values, so a
// caller cannot lose a digit between composing the order and this struct receiving
// it. The four code fields are Gateway codes, and each is validated against the
// set its own surface documents — which for three of them is *not* the trade
// dictionary.
type AlgoAddOrderRequest struct {
	// StockCode is the security code, for example "00700.HK".
	StockCode string
	// ExchangeType is the market: K (Hong Kong), P (US), v (Shenzhen Connect) or
	// t (Shanghai Connect). It is required, and unlike a futures request it is
	// sent.
	ExchangeType types.ExchangeType
	// OrderType is the algorithm order type, as domain.AlgoEntrustType: 1 limit,
	// 2 market. **This is not types.EntrustType**, where 1 is an auction and 3 is
	// a limit order.
	OrderType domain.AlgoEntrustType
	// Price is the order price. A market order still carries one: both vendors
	// send entrustPrice unconditionally, so a market caller passes the
	// conventional "0" through domain.MustNewPrice rather than relying on a
	// Gateway tolerance for an empty price.
	Price domain.Price
	// Quantity is the total order quantity and must be strictly positive.
	Quantity domain.Quantity
	// Side is the direction, as types.EntrustBS: 1 opens a long position, 2
	// closes a long one, 3 closes a short one, 4 opens a short one. All four are
	// accepted; see the GoDoc on AddOrder for why the two vendors' narrower
	// documentation does not narrow this set.
	Side types.EntrustBS
	// TargetStrategy is the execution algorithm, as domain.AlgoStrategy. It is
	// required but deliberately **not** value-bounded, because the reference
	// documents a code under two names; see domain.AlgoStrategy.
	TargetStrategy domain.AlgoStrategy
	// SessionType enables pre-market and after-hours trading, as
	// domain.AlgoSessionType: 0 off, 1 on. The trade surface's wider sessionType
	// is a different dictionary and its extra codes are refused here.
	SessionType domain.AlgoSessionType
	// StrategyParam carries the algorithm-specific tuning. MaxVolume and
	// Sensitivity are required; the rest are optional.
	StrategyParam AlgoStrategyParam
}

// AlgoChangeOrderRequest is the caller-facing body of an algo change: it reprices
// and re-sizes a live master in place.
//
// It is not AlgoAddOrderRequest with fields removed. The order ID names the
// master to change, the security code must be the master's own because the code
// cannot be changed, and the body carries no order type, direction, session type
// or strategy: a modify is a repricing, and retyping an order or reversing its
// direction is a more powerful operation than this endpoint offers. Both vendors
// agree on exactly this field set (Java AlgoChangeEntrustParam adds only orderId
// to AlgoCommonParam; the Python algo_modify_order literal likewise), so the
// omissions are the vendors' and not a choice made here.
//
// Unlike AddOrder, every member of StrategyParam is optional, so a caller can
// change the price alone.
type AlgoChangeOrderRequest struct {
	// OrderID is the master order identifier. It is required.
	OrderID domain.OrderID
	// StockCode must be the master's own stock code; it is required and it is not
	// a filter.
	StockCode string
	// ExchangeType is the market the master trades on. It is required.
	ExchangeType types.ExchangeType
	// Price is the new price and must be a non-negative decimal.
	Price domain.Price
	// Quantity is the new total quantity and must be strictly positive.
	Quantity domain.Quantity
	// StrategyParam carries the new tuning. Every member is optional, but a
	// supplied one is still validated.
	StrategyParam AlgoStrategyParam
}

// AlgoCancelOrderRequest is the caller-facing body of an algo master cancel: it
// cancels a whole master order and every child the platform has already sliced
// out of it.
type AlgoCancelOrderRequest struct {
	// OrderID is the master order identifier. It is required.
	OrderID domain.OrderID
	// ExchangeType is the market the master trades on. It is required, and it is
	// not optional here even though the master order ID is unique: exchangeType
	// names the book the Gateway resolves the master against, so an
	// unrecognised market is a request the SDK already knows it does not mean.
	ExchangeType types.ExchangeType
}

// AlgoCancelEntrustRequest is the caller-facing body of an algo child cancel: it
// cancels one child entrust of a master, leaving the master's remaining schedule
// alone.
type AlgoCancelEntrustRequest struct {
	// OrderID is the master order identifier the child belongs to. It is
	// required: a child is addressed by the pair, and the Gateway's body carries
	// both.
	OrderID domain.OrderID
	// EntrustID is the child entrust identifier within OrderID. It is required.
	EntrustID domain.EntrustID
	// ExchangeType is the market the master trades on. It is required.
	ExchangeType types.ExchangeType
}

// AlgoActionRequest is the caller-facing body of an algo operate: it starts,
// stops, suspends or resumes a live master.
type AlgoActionRequest struct {
	// OrderID is the master order identifier. It is required.
	OrderID domain.OrderID
	// Action is the operation, as domain.AlgoAction. It is required and it is
	// validated against the four documented codes.
	Action domain.AlgoAction
	// TargetStrategy names the strategy bound to the master. It is required for
	// every action, including a stop: the Gateway addresses the strategy, not the
	// master, and the released pkg/hstong/algo carries the same requirement even
	// though the reference's own request example omits the field.
	TargetStrategy domain.AlgoStrategy
	// ExchangeType is the market the master trades on. It is required for every
	// action, for the reason AlgoCancelOrderRequest.ExchangeType states.
	ExchangeType types.ExchangeType
}

// AlgoOrderQuery is the caller-facing body of a master-order query: a paged,
// date-bounded filter over the account's algorithm orders.
//
// ExchangeType and StockCode narrow the result and are independently optional,
// with one coupling carried from the released layer: a StockCode without an
// ExchangeType is refused, because a security code is not unique across the books
// and a code with no market is a filter the SDK cannot check.
type AlgoOrderQuery struct {
	// Page is the page-number pagination. It is not transport.Pagination: that
	// type is cursor-shaped and its Apply writes cursor/page_size keys the algo
	// Gateway does not accept.
	Page PageRequest
	// ExchangeType narrows the query to one market. It is required when StockCode
	// is set and optional otherwise; when supplied it must name a documented
	// market.
	ExchangeType types.ExchangeType
	// StockCode narrows the query to one security. It requires ExchangeType.
	StockCode string
}

// AlgoEntrustIDQuery is the caller-facing body of a child-ID query: it lists the
// child entrust identifiers of one master on one trade date.
//
// It is the reconciliation primitive for a master: after an ambiguous AddOrder or
// ActionOrder, this is how a caller learns whether the platform sliced children
// at all.
type AlgoEntrustIDQuery struct {
	// OrderID is the master order identifier. It is required.
	OrderID domain.OrderID
	// TradeDate is the child orders' trade date, yyyyMMdd. It is required, and it
	// must be a real calendar date.
	TradeDate string
	// ExchangeType is the market the master trades on. It is required.
	ExchangeType types.ExchangeType
}

// algoAddOrderWireRequest is the body of /trade/AlgoAddOrder.
//
// The field order follows the Python request literal — exchangeType, stockCode,
// entrustType, entrustBs, entrustAmount, entrustPrice, targetStrategy,
// sessionType, strategyParam — which is cosmetic, since a JSON object is
// unordered, but it costs nothing and makes the body diffable against the vendor
// source.
//
// strategyParam carries no omitempty. The Java parameter class always declares it
// and the Python literal always builds it, so an absent object would be a request
// shape neither vendor produces; a *change* whose tuning is entirely empty
// therefore still sends `"strategyParam":{}`, which is the vendors' own output for
// that case. The members inside it are omitempty, because a null or an empty
// string where the vendor sends nothing is a body the Gateway must guess at.
type algoAddOrderWireRequest struct {
	ExchangeType   string                `json:"exchangeType"`
	StockCode      string                `json:"stockCode"`
	EntrustType    string                `json:"entrustType"`
	EntrustBS      string                `json:"entrustBs"`
	EntrustAmount  string                `json:"entrustAmount"`
	EntrustPrice   string                `json:"entrustPrice"`
	TargetStrategy string                `json:"targetStrategy"`
	SessionType    string                `json:"sessionType"`
	StrategyParam  algoStrategyParamWire `json:"strategyParam"`
}

// algoChangeOrderWireRequest is the body of /trade/AlgoChangeOrder.
//
// It deliberately has no entrustType, entrustBs, sessionType or targetStrategy.
// Both vendors omit them — the Java change class adds only orderId to the common
// parameter base, and the Python literal likewise — so a symmetry argument that
// mirrored the add body would add four fields the Gateway does not read, and three
// of them would be a more powerful operation than this endpoint offers.
type algoChangeOrderWireRequest struct {
	OrderID       string                `json:"orderId"`
	ExchangeType  string                `json:"exchangeType"`
	StockCode     string                `json:"stockCode"`
	EntrustAmount string                `json:"entrustAmount"`
	EntrustPrice  string                `json:"entrustPrice"`
	StrategyParam algoStrategyParamWire `json:"strategyParam"`
}

// algoStrategyParamWire is the strategyParam object on both order mutations.
//
// Every member carries omitempty, including Interval, whose zero is the Gateway's
// documented default rather than a request for a zero-second interval. A single
// struct serves both bodies for the reason futuresPageQueryWireRequest does: the
// vendor shares one parameter class between them, and two identical structs here
// would have no vendor justification behind the duplication.
type algoStrategyParamWire struct {
	OrigStartTime string `json:"origStartTime,omitempty"`
	OrigEndTime   string `json:"origEndTime,omitempty"`
	MaxVolume     string `json:"maxVolume,omitempty"`
	MinAmount     string `json:"minAmount,omitempty"`
	Sensitivity   string `json:"sensitivity,omitempty"`
	ShowQty       string `json:"showQty,omitempty"`
	QtyPercent    string `json:"qtyPercent,omitempty"`
	Interval      int32  `json:"interval,omitempty"`
}

// algoCancelOrderWireRequest is the body of /trade/AlgoCancelOrder. Two required
// fields, no omitempty, and nothing else exists on the wire to require: the
// security code is *not* on it, because a master cancel is addressed by its
// order ID and its market alone. Both vendors agree — the Java class adds nothing
// to orderId and exchangeType, and the Python literal likewise.
type algoCancelOrderWireRequest struct {
	ExchangeType string `json:"exchangeType"`
	OrderID      string `json:"orderId"`
}

// algoCancelEntrustWireRequest is the body of /trade/AlgoCancelEntrust. It is
// the cancel body plus the child identifier, and it is the only algo body that
// names a child.
type algoCancelEntrustWireRequest struct {
	ExchangeType string `json:"exchangeType"`
	OrderID      string `json:"orderId"`
	EntrustID    string `json:"entrustId"`
}

// algoActionOrderWireRequest is the body of /trade/AlgoActionOrder.
//
// It is structurally unlike the four order mutations: no price, no quantity, no
// strategyParam, and a `action` key none of the others has. That is why the
// request-shape assertions in the test suite are per body rather than per family —
// a rule proved on AddOrder says nothing about this one, and a change that dropped
// `action` from here would satisfy every assertion written against the others.
type algoActionOrderWireRequest struct {
	OrderID        string `json:"orderId"`
	ExchangeType   string `json:"exchangeType"`
	Action         string `json:"action"`
	TargetStrategy string `json:"targetStrategy"`
}

// algoOrderQueryWireRequest is the body of /trade/AlgoQueryOrderList.
//
// PageNo and PageSize carry no omitempty even though they are defaulted, for the
// reason futuresPageQueryWireRequest gives: the substitution happens before the
// marshal, so the emitted bytes are the same either way, and omitting the tag
// would make the body depend on a pre-substitution value.
//
// The two filter keys are omitempty because the Gateway's own filter is optional:
// a query with neither market nor security code is a legitimate request for the
// whole day, and sending empty strings would ask the Gateway to match a market
// named "" rather than to apply no filter. The released pkg/hstong/algo omits both,
// and the Python literal passes None for both.
type algoOrderQueryWireRequest struct {
	PageNo       int    `json:"pageNo"`
	PageSize     int    `json:"pageSize"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
	ExchangeType string `json:"exchangeType,omitempty"`
	StockCode    string `json:"stockCode,omitempty"`
}

// algoEntrustIDQueryWireRequest is the body of /trade/AlgoQueryEntrustIdList. All
// three keys are required and none is omitempty, matching both vendors.
type algoEntrustIDQueryWireRequest struct {
	OrderID      string `json:"orderId"`
	TradeDate    string `json:"tradeDate"`
	ExchangeType string `json:"exchangeType"`
}

// algoOrderListWireResponse is the data object of /trade/AlgoQueryOrderList. The
// rows it holds are domain payloads (domain.AlgoMasterOrderWire); only the
// wrapper is this layer's.
type algoOrderListWireResponse struct {
	AlgoOrderList []domain.AlgoMasterOrderWire `json:"algoOrderList"`
}

// algoEntrustIDListWireResponse is the data object of
// /trade/AlgoQueryEntrustIdList. The rows are bare child identifiers, not
// objects, so there is no row type in pkg/domain to declare.
type algoEntrustIDListWireResponse struct {
	EntrustID []string `json:"entrustId"`
}

// algoMutationWireResponse is the data object of all five algo mutations. It is
// the cash commonStringResponse by alias rather than a second identical struct:
// SPEC.md types all five as a single `data` string, and the cash layer already
// shares one type across its own mutations.
//
// Data is the resulting identifier — a master order ID for four of the five, and
// the child's entrust ID for AlgoCancelEntrust — and the Gateway does not
// guarantee that it is populated: it is documented as the empty string when
// omitted. A caller must therefore reconcile by querying rather than by trusting
// this field.
type algoMutationWireResponse = commonStringResponse

// algoStrategyParamWireFrom maps a caller-facing tuning object to the wire form.
//
// It is the one place the algo money crosses from a domain decimal to a quoted
// string, and it does nothing but that: no scaling, no rounding, no defaulting.
// The omitempty tags then decide which keys appear, so a caller who leaves a
// member at its zero value sends nothing for it rather than sending "0" — which
// matters for the two members where zero is not the same request as absent. The
// money strings are the decimal's own rendering, so a hostile value reaches the
// wire byte for byte; that is asserted per field rather than in aggregate,
// because an aggregate assertion would pass with two fields swapped.
func algoStrategyParamWireFrom(p AlgoStrategyParam) algoStrategyParamWire {
	out := algoStrategyParamWire{
		OrigStartTime: p.OrigStartTime,
		OrigEndTime:   p.OrigEndTime,
		Interval:      p.Interval,
	}
	if !p.MaxVolume.IsZero() {
		out.MaxVolume = p.MaxVolume.String()
	}
	if !p.MinAmount.IsZero() {
		out.MinAmount = p.MinAmount.String()
	}
	if p.Sensitivity != "" {
		out.Sensitivity = string(p.Sensitivity)
	}
	if !p.ShowQty.IsZero() {
		out.ShowQty = p.ShowQty.String()
	}
	if !p.QtyPercent.IsZero() {
		out.QtyPercent = p.QtyPercent.String()
	}
	return out
}

// ---------------------------------------------------------------------------
// Mutation input validation
//
// Every predicate below runs before the request is built, so a bad mutation
// costs zero HTTP requests. That is not only cheaper: it is the property that
// makes a local rejection safe on an order. A request the Gateway saw and refused
// may have had a side effect; a request this SDK never sent cannot have had one.
// It is also why the rejects are typed at types.StatusInvalidParam under the
// method's own op — a caller must be able to tell "I built this wrong" from "the
// exchange said no", and only the former is free of consequence.
// ---------------------------------------------------------------------------

// algoValidateStockCode reports whether code is a usable security code:
// non-empty once trimmed, and carrying no internal whitespace.
//
// The check is deliberately permissive about characters, because no algo code
// pattern is documented in this repository and the same code format is not
// recorded for the Hong Kong and US books. The released pkg/hstong/algo predicate
// is reproduced exactly, including its one quirk: a code with *leading or
// trailing* whitespace passes, because only interior whitespace is tested, and the
// caller's spelling is what goes on the wire. That is parity rather than an
// oversight, and the failure direction is the loud one — a padded code is refused
// by the Gateway with a message and no side effect, and a place where a code is
// sent is a mutation.
func algoValidateStockCode(op, code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return errs.New(types.StatusInvalidParam, op, "stockCode must not be empty")
	}
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return errs.New(types.StatusInvalidParam, op, "stockCode must not contain whitespace")
	}
	return nil
}

// algoValidateExchangeType checks the market against the documented four.
//
// **This is the same check A6 installed on the released algo surface, carried
// forward unchanged: fail closed.** types.ExchangeType and SPEC §7.3 are a flat
// four-row table with no non-exhaustive note, the Java AlgoOrder and the Python
// ExchangeType class both enumerate exactly these four codes, and
// pkg/hstong/future already refuses anything outside the set on the same type. A
// caller who sends "Z" is sending a request the SDK already knows it does not
// mean, and on a cancel that names the book the master is resolved against.
//
// The message names all four codes, and it says which is which, because the field
// is case-sensitive: "v" and "t" are lowercase, so a caller who upper-cases the
// whole string produces a code that looks right and is not.
func algoValidateExchangeType(op string, exchange types.ExchangeType) error {
	switch exchange {
	case types.ExchangeHK, types.ExchangeUS, types.ExchangeShenzhenConnect, types.ExchangeShanghaiConnect:
		return nil
	default:
		return errs.New(types.StatusInvalidParam, op,
			"exchangeType must be K (Hong Kong), P (US), v (Shenzhen Connect) or t (Shanghai Connect)")
	}
}

// algoValidateDirection checks the direction against the four-value cash set.
//
// **The set is disputed and this deliberately keeps the wider one, for the reason
// futuresValidateEntrustBS states.** Both vendors document only two values on the
// algo field itself — the Java AlgoEntrustParam comment reads [1:买入 2:卖出] and
// the Python algo_entrust docstring reads [1:买 2:卖] — while SPEC §7.4, the
// vendors' own shared EntrustBS enum, the released pkg/hstong/algo and
// pkg/hstong/future all carry four. Narrowing here would be a caller-visible
// change on a mutation, made on incomplete evidence, against a field the v0.1.x
// SDK has accepted as 1-4 since v0.1.0. The failure mode of staying permissive is
// the good one: an out-of-set direction is refused by the Gateway with a message
// and no side effect, where a fail-closed rejection here would refuse a request
// the vendor's shared dictionary endorses.
//
// The disagreement is recorded rather than resolved, and it is per-field: the
// futures answer of not bounding an unbounded field does not license skipping the
// next one, and this field is bounded even though the same two vendors
// under-document it there.
//
// Fail closed within the four: a value outside them is refused locally, because
// the Gateway's own rejection of a malformed direction is a worse outcome than
// never sending it.
func algoValidateDirection(op string, side types.EntrustBS) error {
	switch side {
	case types.EntrustBuy, types.EntrustSell, types.EntrustCloseShort, types.EntrustOpenShort:
		return nil
	default:
		return errs.New(types.StatusInvalidParam, op, "entrustBs must be 1, 2, 3 or 4")
	}
}

// algoValidateOrderType checks the algorithm order type against domain's own
// two-value set.
//
// It is deliberately *not* checked against types.EntrustType. That set has
// seventeen codes, uses 1 for an auction and 3 for a limit order, and is
// explicitly documented as non-exhaustive and forward-compatible — a promise this
// field does not make. domain.AlgoEntrustType carries the two codes all three
// sources agree on, and Valid is the gate.
func algoValidateOrderType(op string, orderType domain.AlgoEntrustType) error {
	if !orderType.Valid() {
		return errs.New(types.StatusInvalidParam, op, "entrustType must be 1 (limit) or 2 (market)")
	}
	return nil
}

// algoValidateSessionType checks the pre-market/after-hours code against
// domain.AlgoSessionType's two-value set.
//
// It is not the trade surface's five-code sessionType: the three extra codes are
// for conditional orders, which an algorithm master order is not. See
// domain.AlgoSessionType for the recorded vendor disagreement.
func algoValidateSessionType(op string, session domain.AlgoSessionType) error {
	if !session.Valid() {
		return errs.New(types.StatusInvalidParam, op, "sessionType must be 0 (off) or 1 (on)")
	}
	return nil
}

// algoValidateAction checks the operate code against domain.AlgoAction's four-value
// set, naming all four so a message cannot be read as evidence that only start and
// stop are accepted.
func algoValidateAction(op string, action domain.AlgoAction) error {
	if !action.Valid() {
		return errs.New(types.StatusInvalidParam, op,
			"action must be 1 (START), 2 (STOP), 3 (SUSPEND) or 4 (RESUME)")
	}
	return nil
}

// algoValidateStrategy checks the strategy code for emptiness only.
//
// **This is the one field on the algo surface that is checked for shape and not
// for membership, and the reason is on domain.AlgoStrategy**: the reference
// documents code 1005 under two different names, so a closed set would refuse a
// code the vendor's own documentation endorses. A blank code is refused because
// the Gateway cannot resolve a strategy from an empty string at all; every other
// value is forwarded unchanged, which is what the released layer does and what
// A6's design note endorses for a genuinely unknowable set.
func algoValidateStrategy(op string, strategy domain.AlgoStrategy) error {
	if strategy == "" {
		return errs.New(types.StatusInvalidParam, op, "targetStrategy must not be empty")
	}
	return nil
}

// algoValidateOrderPrice checks the order price on a mutation.
//
// It is required with no market-order waiver. Both vendors send entrustPrice
// unconditionally and neither exempts a market order, so a market caller passes
// the conventional "0" through domain.MustNewPrice and this check is satisfied
// without a special case — the same recommendation the futures layer reached and
// the one place it is deliberately *stricter* than nothing at all.
//
// A domain.Price cannot be malformed — MustNewPrice panics rather than construct
// one — so the only failure left is a negative price, and that is what is checked.
// The tick step is deliberately *not* applied: no algo endpoint carries a tick
// schedule, and Price.Validate skips the step whenever the tick is zero, so a
// caller arriving with a real grid should not have their price measured against a
// grid this SDK invented.
//
// An empty AlgoAddOrderRequest.Price is the zero Price, which reads as "0" and is
// therefore indistinguishable from a market order. That is a property of the type
// rather than a hole in the check, and it is why the field is always sent quoted
// on the wire rather than omitted: an absent price and a market order are the same
// request, so the Gateway must never be asked to infer one.
func algoValidateOrderPrice(op string, price domain.Price) error {
	if err := price.Validate(false); err != nil {
		return errs.New(types.StatusInvalidParam, op,
			"entrustPrice must be a non-negative decimal: "+err.Error())
	}
	return nil
}

// algoValidateOrderQuantity checks that the order quantity is strictly positive.
// Zero and negative are both refused: a zero-quantity order is not an order, and a
// negative one is a direction, which is what entrustBs is for.
func algoValidateOrderQuantity(op string, qty domain.Quantity) error {
	if !qty.IsPositive() {
		return errs.New(types.StatusInvalidParam, op, "entrustAmount must be a positive decimal")
	}
	return nil
}

// algoValidateMaxVolume checks the per-child order ceiling.
//
// It is required on an add and optional on a change, so the required half lives in
// the add method's GoDoc and this predicate enforces only the sign. A zero
// maxVolume on a change means "leave it alone" and the key is omitted; a negative
// one is not a quantity under any reading.
func algoValidateMaxVolume(op string, maxVolume domain.Quantity) error {
	if maxVolume.IsNegative() {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.maxVolume must not be negative")
	}
	return nil
}

// algoValidateOptionalQuantity checks an optional quantity member of a change's
// strategyParam. Absent is legitimate and is expressed by the zero value, so only
// a negative amount is refused.
func algoValidateOptionalQuantity(op, field string, qty domain.Quantity) error {
	if qty.IsNegative() {
		return errs.New(types.StatusInvalidParam, op, "strategyParam."+field+" must not be negative")
	}
	return nil
}

// algoValidateOptionalMoney checks an optional money member of a change's
// strategyParam on the same terms as algoValidateOptionalQuantity. The currency
// is never transmitted, so there is nothing here about it to check.
func algoValidateOptionalMoney(op, field string, amount domain.Money) error {
	if amount.IsNegative() {
		return errs.New(types.StatusInvalidParam, op, "strategyParam."+field+" must not be negative")
	}
	return nil
}

// algoValidateRequiredMaxVolume checks the per-child ceiling on an add, where the
// reference and both vendors require it. A zero is refused as well as a negative
// one, because on an add "leave it alone" has no meaning: there is no prior
// strategy to leave alone.
func algoValidateRequiredMaxVolume(op string, maxVolume domain.Quantity) error {
	if !maxVolume.IsPositive() {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.maxVolume must be a positive decimal")
	}
	return nil
}

// algoValidateRequiredSensitivity checks the aggressiveness code on an add, where
// the reference and both vendors require it. On a change it is optional, and
// algoValidateOptionalSensitivity covers that case.
func algoValidateRequiredSensitivity(op string, sensitivity domain.AlgoSensitivity) error {
	if !sensitivity.Valid() {
		return errs.New(types.StatusInvalidParam, op,
			"strategyParam.sensitivity must be 1 (neutral), 2 (aggressive) or 3 (passive)")
	}
	return nil
}

// algoValidateOptionalSensitivity checks a change's aggressiveness code when one
// is supplied. Absent is legitimate; an unlisted code is not.
func algoValidateOptionalSensitivity(op string, sensitivity domain.AlgoSensitivity) error {
	if sensitivity != "" && !sensitivity.Valid() {
		return errs.New(types.StatusInvalidParam, op,
			"strategyParam.sensitivity must be 1 (neutral), 2 (aggressive) or 3 (passive)")
	}
	return nil
}

// algoDigitsShaped reports whether code is a non-empty run of ASCII decimal
// digits. It is a digit scan and never a parse, so no value a float would have
// mangled can pass or fail it.
//
// It deliberately says nothing about magnitude. A caller who reached for a bounds
// check here would be inventing one, and a length cap in particular would be an
// invented limit on a code the Gateway, not this SDK, defines.
func algoDigitsShaped(code string) bool {
	if code == "" {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

// algoDateShaped reports whether date is eight decimal digits and nothing else.
//
// It is separate from the calendar check so a caller that passed "2026-09-26" is
// told the format is wrong rather than that the date does not exist, which is the
// distinction the released layer draws and the one that tells a caller which of
// its two mistakes to fix. The check is a digit scan and never a parse, so it
// cannot be defeated by a value a float would have mangled.
func algoDateShaped(date string) bool {
	return len(date) == 8 && algoDigitsShaped(date)
}

// algoValidateDate reports whether date is a real yyyyMMdd calendar date. An
// eight-digit string that is not a date on the calendar ("20260230") is rejected
// as a calendar failure rather than accepted because time.Parse is the only
// authority on what exists.
//
// The dates are required on every algo body that takes one: the Java comments read
// 必须 yyyyMMdd and the Python signatures take them positionally, so unlike the
// futures history queries there is no unbounded variant to preserve.
func algoValidateDate(op, field, date string) error {
	if !algoDateShaped(date) {
		return errs.New(types.StatusInvalidParam, op, field+" must be yyyyMMdd")
	}
	if _, err := time.Parse(algoDateLayout, date); err != nil {
		return errs.New(types.StatusInvalidParam, op, field+" must be a valid calendar date")
	}
	return nil
}

// algoValidateTimeShaped reports whether t is a real HHmmSS wall-clock time.
//
// Only the *shape* is checked, and deliberately so: the vendors document the
// format and nothing else, and a time outside 00:00-23:59 is a value no calendar
// has, so rejecting it is decidable rather than a guess. A caller who means
// "the exchange open" leaves it empty, which is a different state and is not
// checked at all.
func algoValidateTimeShaped(op, field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(algoTimeLayout, value); err != nil {
		return errs.New(types.StatusInvalidParam, op, field+" must be HHmmSS")
	}
	return nil
}

// algoValidateInterval checks the child-order interval. A zero omits the key and
// lets the Gateway apply its documented default; a negative interval is a request
// for an impossible schedule.
func algoValidateInterval(op string, interval int32) error {
	if interval < 0 {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.interval must not be negative")
	}
	return nil
}

// algoValidateQtyPercent checks the participation percentage when one is
// supplied.
//
// The range is 1-99, stated identically by the reference, the Java AlgoStrategy
// comment and the Python algo_entrust docstring, with no fourth source
// contradicting it — which is the condition A6 sets for a fail-closed check. It is
// checked only when supplied, because a caller who leaves it out is asking the
// strategy to choose and that is legitimate.
//
// The check is a digit scan over the decimal's own rendering, so it is exact and
// never touches a float. A negative value is refused first, because a leading '-'
// would otherwise read as a non-digit and be reported as "not a whole number",
// which is the wrong diagnosis for a value that is very much a number.
func algoValidateQtyPercent(op string, percent domain.Rate) error {
	if percent.IsZero() {
		return nil
	}
	if percent.IsNegative() {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.qtyPercent must be between 1 and 99")
	}
	digits := percent.String()
	if !algoDigitsShaped(digits) {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.qtyPercent must be a whole number")
	}
	// One or two digits with a non-zero leading digit is exactly the set 1-99:
	// "0" and "00" are the only two-or-fewer-digit renderings excluded by the
	// non-zero requirement, and nothing three digits long can be in range.
	if len(digits) > algoQtyPercentMaxDigits || strings.Trim(digits, "0") == "" {
		return errs.New(types.StatusInvalidParam, op, "strategyParam.qtyPercent must be between 1 and 99")
	}
	return nil
}

// algoValidateOrderID checks the master order id on the four methods that name
// one. It trims before testing, so an all-whitespace id is refused.
func algoValidateOrderID(op string, orderID domain.OrderID) error {
	if strings.TrimSpace(orderID.String()) == "" {
		return errs.New(types.StatusInvalidParam, op, "orderId must not be empty")
	}
	return nil
}

// algoValidateEntrustID checks the child entrust id on the one method that names
// a child, on the same terms as algoValidateOrderID.
func algoValidateEntrustID(op string, entrustID domain.EntrustID) error {
	if strings.TrimSpace(entrustID.String()) == "" {
		return errs.New(types.StatusInvalidParam, op, "entrustId must not be empty")
	}
	return nil
}

// algoValidateStrategyParamTimes checks the two HHmmSS window members. It is a
// separate predicate from algoValidateStrategyParam because the two are adjacent
// in the released layer's GoDoc and a single combined call would let one bad
// member hide the other's check.
func algoValidateStrategyParamTimes(op string, p AlgoStrategyParam) error {
	if err := algoValidateTimeShaped(op, "strategyParam.origStartTime", p.OrigStartTime); err != nil {
		return err
	}
	return algoValidateTimeShaped(op, "strategyParam.origEndTime", p.OrigEndTime)
}

// ---------------------------------------------------------------------------
// The two algo reads
// ---------------------------------------------------------------------------

// QueryOrderList returns the page of algorithm master orders matching query. It
// is a read-only query and is retryable by the resilience layer.
//
// query.Page's PageNo and PageSize are defaulted to 1 and 30 at the mapping
// boundary when they are zero or negative; 30 is the default both vendors document
// for this endpoint and not the futures layer's 20. Unlike futures there is
// deliberately no page-size ceiling, because no source in this repository
// documents one for algo and an invented bound refuses a page the Gateway
// accepts. Both dates are required and must be real yyyyMMdd calendar dates; the
// vendors document them as 必须 and take them positionally, so there is no
// unbounded variant here as there is for the futures history queries.
//
// query.ExchangeType is optional, but present-and-unrecognised is refused: a
// market the SDK does not recognise is a filter the Gateway would have to guess
// at, and a *read* is the cheap place to be loud about it. query.StockCode
// requires query.ExchangeType, which the released pkg/hstong/algo also enforces.
//
// accountID names the session and appears in no request body — see the account
// rule at the head of this file. The result is the non-nil empty slice when the
// Gateway matched nothing, so a caller ranging over it behaves the same as for a
// populated reply.
//
// Every price, quantity and amount on a row crosses as a quoted string and is read
// verbatim; an absent field becomes an explicit zero rather than a panic, as
// QueryEntrustIDList states. This is one of the two reconciliation queries for an
// ambiguous algo mutation (docs/adr/0003-no-auto-retry-orders.md).
func (s *AlgoService) QueryOrderList(ctx context.Context, accountID domain.AccountID, query AlgoOrderQuery) ([]*domain.AlgoMasterOrder, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opAlgoQueryOrderList, "accountID must not be empty")
	}
	if err := algoValidateDate(opAlgoQueryOrderList, "startDate", query.Page.StartDate); err != nil {
		return nil, err
	}
	if err := algoValidateDate(opAlgoQueryOrderList, "endDate", query.Page.EndDate); err != nil {
		return nil, err
	}
	if query.StockCode != "" && query.ExchangeType == "" {
		return nil, errs.New(types.StatusInvalidParam, opAlgoQueryOrderList,
			"exchangeType is required when stockCode is set")
	}
	if query.ExchangeType != "" {
		if err := algoValidateExchangeType(opAlgoQueryOrderList, query.ExchangeType); err != nil {
			return nil, err
		}
	}
	if query.StockCode != "" {
		if err := algoValidateStockCode(opAlgoQueryOrderList, query.StockCode); err != nil {
			return nil, err
		}
	}

	pageNo, pageSize := query.Page.PageNo, query.Page.PageSize
	if pageNo <= 0 {
		pageNo = algoDefaultPageNo
	}
	if pageSize <= 0 {
		pageSize = algoDefaultPageSize
	}
	params := algoOrderQueryWireRequest{
		PageNo:       pageNo,
		PageSize:     pageSize,
		StartDate:    query.Page.StartDate,
		EndDate:      query.Page.EndDate,
		ExchangeType: string(query.ExchangeType),
		StockCode:    query.StockCode,
	}

	var out algoOrderListWireResponse
	if err := s.client.Do(ctx, opAlgoQueryOrderList, client.RouteTradeAlgoQueryOrderList,
		params, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	orders := make([]*domain.AlgoMasterOrder, len(out.AlgoOrderList))
	for i := range out.AlgoOrderList {
		orders[i] = domain.AlgoMasterOrderFromDTO(&out.AlgoOrderList[i])
	}
	return orders, nil
}

// QueryEntrustIDList returns the child entrust IDs of one master order on one
// trade date. It is a read-only query and is retryable by the resilience layer.
//
// It is the reconciliation primitive for the other six endpoints. After an
// ambiguous AddOrder, a list of child IDs is proof the platform sliced the master
// at all; after an ambiguous ActionOrder, the same list plus a master-order query
// says whether the strategy ran. It takes no money and has no page, so there is
// nothing in it to lose a digit from.
//
// tradeDate is required and must be a real yyyyMMdd calendar date; both vendors
// document it as required. exchangeType is required and is checked against the
// documented four, for the reason the mutations state: it names the book the
// master is resolved against.
//
// accountID names the session and appears in no request body. The result is the
// non-nil empty slice when the Gateway matched nothing, so a caller ranging over
// it behaves the same as for a populated reply — which for a reconciliation query
// is the important case, because "no children" is a real answer and must not read
// as a failed call.
func (s *AlgoService) QueryEntrustIDList(ctx context.Context, accountID domain.AccountID, query AlgoEntrustIDQuery) ([]domain.EntrustID, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opAlgoQueryEntrustIDList, "accountID must not be empty")
	}
	if err := algoValidateOrderID(opAlgoQueryEntrustIDList, query.OrderID); err != nil {
		return nil, err
	}
	if err := algoValidateDate(opAlgoQueryEntrustIDList, "tradeDate", query.TradeDate); err != nil {
		return nil, err
	}
	if err := algoValidateExchangeType(opAlgoQueryEntrustIDList, query.ExchangeType); err != nil {
		return nil, err
	}

	params := algoEntrustIDQueryWireRequest{
		OrderID:      query.OrderID.String(),
		TradeDate:    query.TradeDate,
		ExchangeType: string(query.ExchangeType),
	}

	var out algoEntrustIDListWireResponse
	if err := s.client.Do(ctx, opAlgoQueryEntrustIDList, client.RouteTradeAlgoQueryEntrustIdList,
		params, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	ids := make([]domain.EntrustID, len(out.EntrustID))
	for i := range out.EntrustID {
		ids[i] = domain.EntrustID(out.EntrustID[i])
	}
	return ids, nil
}

// ---------------------------------------------------------------------------
// The five algo mutations
//
// All five are in the closed mutation set internal/resilience keeps, so each
// issues exactly one attempt at every retry-policy configuration. The GoDoc on
// each repeats the reconciliation path in full, because a caller who does not
// know what to do after a failure on a money-moving call is the failure mode ADR
// 0003 is about: not the lost order, but the second order placed while
// reconciling the first.
// ---------------------------------------------------------------------------

// AddOrder places one algorithm master order on /trade/AlgoAddOrder and returns
// its order ID.
//
// # It issues exactly one attempt
//
// Under any configuration, at any layer, with any option. The Gateway's own
// timeout_sec means a call can time out *after* the request reached the platform
// and before the reply was read, and there is no idempotency key to make a
// resubmission safe, so a second attempt is a possible duplicate master order
// rather than a possible success (docs/adr/0003-no-auto-retry-orders.md). Nothing
// in this package retries it, and no option can be made to.
//
// # What to do after a failure
//
// **Reconcile; never resubmit.** The failure is ambiguous, not final:
//
//   - The error is a typed *errs.Error carrying opAlgoAddOrder. A Gateway
//     rejection keeps the Gateway's own code and category — a "1007 duplicate
//     submission" arrives in errs.CategoryTrading, which is the code that exists
//     precisely because of this ambiguity — and a local rejection is
//     types.StatusInvalidParam in errs.CategoryAPI and provably cost no request.
//     Both stay traversable with errors.Is and errors.As, so a caller branching
//     on the category is reading the exchange, not the SDK's opinion of it.
//   - Then call QueryOrderList for the day's masters, narrowed by StockCode and
//     ExchangeType, and read domain.AlgoMasterOrder.Status and OrderID; and
//     QueryEntrustIDList for the master, whose child list is proof the platform
//     sliced it at all.
//
// The returned order ID is what the Gateway echoed, and the Gateway does not
// guarantee that it is populated: it is documented as the empty string when
// omitted. So a non-empty id is a convenience, never a substitute for the
// reconciliation above, and an empty one is not evidence the order was refused.
//
// # Validation, all of it local and all of it free
//
// Every check below runs before the request is built, so each refusal is a
// types.StatusInvalidParam *errs.Error under opAlgoAddOrder that costs zero HTTP
// requests and provably had no side effect:
//
//   - accountID is not zero. It names the session and appears in no request body
//     — see the account rule on AlgoService.
//   - StockCode is non-empty and free of interior whitespace. No algo code
//     grammar is documented in this repository, so the check stays permissive
//     about characters.
//   - ExchangeType names one of K, P, v or t.
//   - OrderType is 1 or 2, checked against domain.AlgoEntrustType and **not**
//     against types.EntrustType, whose 1 is an auction and whose 3 is a limit
//     order.
//   - Price is a non-negative decimal, with no market-order waiver: a market order
//     passes "0" through domain.MustNewPrice.
//   - Quantity is strictly positive.
//   - Side is one of 1, 2, 3 or 4. The two vendors document only 1 and 2 on this
//     field and the set is deliberately kept at four; see algoValidateDirection.
//   - TargetStrategy is not empty but is **not** value-bounded, because the
//     reference documents one of its codes under two names. See
//     algoValidateStrategy.
//   - SessionType is 0 or 1, checked against domain.AlgoSessionType, whose set is
//     two codes and not the trade surface's five.
//   - strategyParam.maxVolume is strictly positive and strategyParam.sensitivity
//     is 1, 2 or 3; the reference and both vendors require both on an add.
//   - strategyParam.origStartTime and origEndTime, when supplied, are HHmmSS.
//   - strategyParam.minAmount and showQty, when supplied, are non-negative.
//   - strategyParam.qtyPercent, when supplied, is a whole number in 1-99.
//   - strategyParam.interval, when supplied, is not negative; zero omits the key.
func (s *AlgoService) AddOrder(ctx context.Context, accountID domain.AccountID, order AlgoAddOrderRequest) (domain.OrderID, error) {
	if accountID.IsZero() {
		return "", errs.New(types.StatusInvalidParam, opAlgoAddOrder, "accountID must not be empty")
	}
	if err := algoValidateStockCode(opAlgoAddOrder, order.StockCode); err != nil {
		return "", err
	}
	if err := algoValidateExchangeType(opAlgoAddOrder, order.ExchangeType); err != nil {
		return "", err
	}
	if err := algoValidateOrderType(opAlgoAddOrder, order.OrderType); err != nil {
		return "", err
	}
	if err := algoValidateOrderPrice(opAlgoAddOrder, order.Price); err != nil {
		return "", err
	}
	if err := algoValidateOrderQuantity(opAlgoAddOrder, order.Quantity); err != nil {
		return "", err
	}
	if err := algoValidateDirection(opAlgoAddOrder, order.Side); err != nil {
		return "", err
	}
	if err := algoValidateStrategy(opAlgoAddOrder, order.TargetStrategy); err != nil {
		return "", err
	}
	if err := algoValidateSessionType(opAlgoAddOrder, order.SessionType); err != nil {
		return "", err
	}
	if err := algoValidateRequiredMaxVolume(opAlgoAddOrder, order.StrategyParam.MaxVolume); err != nil {
		return "", err
	}
	if err := algoValidateRequiredSensitivity(opAlgoAddOrder, order.StrategyParam.Sensitivity); err != nil {
		return "", err
	}
	if err := algoValidateStrategyParamTimes(opAlgoAddOrder, order.StrategyParam); err != nil {
		return "", err
	}
	if err := algoValidateOptionalMoney(opAlgoAddOrder, "minAmount", order.StrategyParam.MinAmount); err != nil {
		return "", err
	}
	if err := algoValidateOptionalQuantity(opAlgoAddOrder, "showQty", order.StrategyParam.ShowQty); err != nil {
		return "", err
	}
	if err := algoValidateQtyPercent(opAlgoAddOrder, order.StrategyParam.QtyPercent); err != nil {
		return "", err
	}
	if err := algoValidateInterval(opAlgoAddOrder, order.StrategyParam.Interval); err != nil {
		return "", err
	}

	req := algoAddOrderWireRequest{
		ExchangeType:   string(order.ExchangeType),
		StockCode:      order.StockCode,
		EntrustType:    string(order.OrderType),
		EntrustBS:      string(order.Side),
		EntrustAmount:  order.Quantity.String(),
		EntrustPrice:   order.Price.String(),
		TargetStrategy: string(order.TargetStrategy),
		SessionType:    string(order.SessionType),
		StrategyParam:  algoStrategyParamWireFrom(order.StrategyParam),
	}

	var out algoMutationWireResponse
	if err := s.client.Do(ctx, opAlgoAddOrder, client.RouteTradeAlgoAddOrder,
		req, s.client.JSON(), &out); err != nil {
		return "", err
	}
	return domain.OrderID(out.Data), nil
}

// CancelOrder cancels a whole algorithm master order, and every child the
// platform has already sliced out of it, on /trade/AlgoCancelOrder.
//
// # It is not the same operation as CancelEntrust
//
// Cancelling the master does not use a child's entrust ID, and cancelling a child
// does not affect the master's remaining schedule. A caller who wants to stop
// further slicing and keep what has already traded wants this; a caller who wants
// one child gone wants CancelEntrust. The two are the only algo endpoints whose
// bodies are near-identical and whose effects are not, which is why both are
// separate methods rather than one with an optional child.
//
// # It issues exactly one attempt
//
// Under any configuration, for the reason AddOrder states. A cancel is not the
// idempotent-looking operation it appears to be: a retried cancel can hit a reused
// identifier or report the wrong terminal state, so it is in the same closed set
// as the submit (docs/adr/0003-no-auto-retry-orders.md).
//
// # What to do after a failure
//
// **Reconcile; never resubmit.** The error is a typed *errs.Error carrying
// opAlgoCancelOrder, with a Gateway rejection keeping its own code — a "1007" in
// errs.CategoryTrading is the ambiguous case and stays traversable with errors.Is
// and errors.As. Then call QueryOrderList for the day's masters, narrowed by
// StockCode and ExchangeType, and read domain.AlgoMasterOrder.Status: a status of
// AlgoStatusCancelled means the cancel took effect, and anything else means the
// order is still live or was never live. QueryEntrustIDList gives the master's
// children, because a cancel that raced a platform slice leaves a master and a
// child list rather than nothing.
//
// # Validation, all of it local and all of it free
//
// Each refusal is a types.StatusInvalidParam *errs.Error under
// opAlgoCancelOrder costing zero HTTP requests:
//
//   - accountID is not zero.
//   - OrderID is not blank, trimmed.
//   - ExchangeType names one of K, P, v or t. It is required even though the
//     master order ID is unique, because exchangeType names the book the Gateway
//     resolves the master against and an unrecognised market is a request the SDK
//     already knows it does not mean.
//
// Nothing else is required, and nothing else exists on the wire to require. Note
// what is *absent* from the checks: there is no security code, because the
// Gateway's cancel body does not carry one and both vendors omit it.
func (s *AlgoService) CancelOrder(ctx context.Context, accountID domain.AccountID, req AlgoCancelOrderRequest) (domain.OrderID, error) {
	if accountID.IsZero() {
		return "", errs.New(types.StatusInvalidParam, opAlgoCancelOrder, "accountID must not be empty")
	}
	if err := algoValidateOrderID(opAlgoCancelOrder, req.OrderID); err != nil {
		return "", err
	}
	if err := algoValidateExchangeType(opAlgoCancelOrder, req.ExchangeType); err != nil {
		return "", err
	}

	body := algoCancelOrderWireRequest{
		ExchangeType: string(req.ExchangeType),
		OrderID:      req.OrderID.String(),
	}

	var out algoMutationWireResponse
	if err := s.client.Do(ctx, opAlgoCancelOrder, client.RouteTradeAlgoCancelOrder,
		body, s.client.JSON(), &out); err != nil {
		return "", err
	}
	return domain.OrderID(out.Data), nil
}

// CancelEntrust cancels one child entrust of an algorithm master order on
// /trade/AlgoCancelEntrust, and returns the child's entrust ID.
//
// **This is the one mutation whose reply identifier is not a master order ID.**
// Four of the five echo a master; this one echoes the child the Gateway cancelled,
// so the returned domain.EntrustID is a child and the domain.OrderID it belongs to
// is req.OrderID. Conflating the two is the mistake a caller makes when reading
// the cash CancelEntrust, whose reply is a standalone order id; the two surfaces
// share a route suffix and nothing else.
//
// # It issues exactly one attempt
//
// Under any configuration, for the reason AddOrder states.
//
// # What to do after a failure
//
// **Reconcile; never resubmit.** The error is a typed *errs.Error carrying
// opAlgoCancelEntrust, with a Gateway rejection keeping its own code. Then call
// QueryEntrustIDList for the master and the trade date: a child still in the list
// means the cancel did not land, and a child that has gone means it did. Because
// only the child is affected, a failed cancel here is a narrower problem than a
// failed CancelOrder — the master's schedule is intact either way — which is why
// reconciling it needs no master-order query.
//
// # Validation, all of it local and all of it free
//
// Each refusal is a types.StatusInvalidParam *errs.Error under
// opAlgoCancelEntrust costing zero HTTP requests:
//
//   - accountID is not zero.
//   - OrderID is not blank, trimmed. It is required even though the child
//     identifier would address the child on its own, because a child is addressed
//     by the pair and the Gateway's body carries both.
//   - EntrustID is not blank, trimmed.
//   - ExchangeType names one of K, P, v or t, for the reason CancelOrder states.
func (s *AlgoService) CancelEntrust(ctx context.Context, accountID domain.AccountID, req AlgoCancelEntrustRequest) (domain.EntrustID, error) {
	if accountID.IsZero() {
		return "", errs.New(types.StatusInvalidParam, opAlgoCancelEntrust, "accountID must not be empty")
	}
	if err := algoValidateOrderID(opAlgoCancelEntrust, req.OrderID); err != nil {
		return "", err
	}
	if err := algoValidateEntrustID(opAlgoCancelEntrust, req.EntrustID); err != nil {
		return "", err
	}
	if err := algoValidateExchangeType(opAlgoCancelEntrust, req.ExchangeType); err != nil {
		return "", err
	}

	body := algoCancelEntrustWireRequest{
		ExchangeType: string(req.ExchangeType),
		OrderID:      req.OrderID.String(),
		EntrustID:    req.EntrustID.String(),
	}

	var out algoMutationWireResponse
	if err := s.client.Do(ctx, opAlgoCancelEntrust, client.RouteTradeAlgoCancelEntrust,
		body, s.client.JSON(), &out); err != nil {
		return "", err
	}
	return domain.EntrustID(out.Data), nil
}

// ChangeOrder changes a live algorithm master order's price, quantity and strategy
// tuning in place, on /trade/AlgoChangeOrder.
//
// # It is not a convenience over cancel-then-add
//
// The two are not interchangeable, and the difference is the whole reason this
// endpoint exists:
//
//   - One request reaches the wire, against two. Between a cancel and a new master
//     the account is flat, and a competing order, a margin call or a limit move
//     fills that window.
//   - The order never leaves the book, so its priority survives. A cancel and a
//     replacement reset it.
//   - Under ADR 0003 that is the decisive row: a cancel-then-add pair is two
//     unreconciled mutations, and if the cancel succeeded while the add timed out
//     the SDK cannot tell the caller which, leaving them with an order state they
//     may not know about. A change is one unreconciled mutation whose failure mode
//     is bounded — worst case the change did not apply and the original order
//     stands.
//
// **A change is not a price increase and a cancel is not a failure.** Choosing
// between them is the caller's decision, and this method says so rather than
// implying that repricing is free.
//
// The body carries no order type, direction, session type or strategy. Both
// vendors omit all four — the Java change class adds only orderId to the shared
// parameter base, and the Python literal likewise — so the order keeps the type
// and direction it was placed with, and a change that could retype or reverse an
// order would be a strictly more powerful operation than this endpoint offers.
//
// # It issues exactly one attempt
//
// Under any configuration, for the reason AddOrder states. A change is a
// live-state mutation with no retry and no second chance.
//
// # What to do after a failure
//
// **Reconcile; never resubmit, and in particular never follow a failed change with
// a cancel.** The error is a typed *errs.Error carrying opAlgoChangeOrder, with a
// Gateway rejection keeping its own code — a "1007" in errs.CategoryTrading is the
// ambiguous case. Then call QueryOrderList narrowed by StockCode and ExchangeType
// and read two fields on the matching domain.AlgoMasterOrder:
//
//   - EntrustPrice, which is the field whose value is in question. A change whose
//     result is ambiguous leaves a master whose current price is unknown, so this
//     is the query that answers it.
//   - Status, which is how a *pending* change is told from a live one:
//     AlgoStatusWaitModify means a change the Gateway has not yet applied is in
//     flight, and AlgoStatusModified means one has been. A master you just tried
//     to change and still report as AlgoStatusWaitModify is a
//     reconcile-in-progress, not a failure to resubmit against.
//
// QueryEntrustIDList gives the master's children, since a change that raced a
// platform slice changes what is left to reprice.
//
// # Validation, all of it local and all of it free
//
// Each refusal is a types.StatusInvalidParam *errs.Error under opAlgoChangeOrder
// costing zero HTTP requests. Every member of StrategyParam is optional here
// where the add required two of them, so a caller can change the price alone; a
// *supplied* member is still validated, on the same terms as the add.
//
//   - accountID is not zero.
//   - OrderID is not blank, trimmed.
//   - StockCode is non-empty and free of interior whitespace. It is required even
//     though the order id identifies the master, because the Gateway's body
//     carries it and it must be the master's own code: the code cannot be changed.
//   - ExchangeType names one of K, P, v or t.
//   - Price is a non-negative decimal, with no market-order waiver.
//   - Quantity is strictly positive.
//   - strategyParam.sensitivity, when supplied, is 1, 2 or 3.
//   - strategyParam.origStartTime and origEndTime, when supplied, are HHmmSS.
//   - strategyParam.maxVolume, minAmount and showQty, when supplied, are
//     non-negative.
//   - strategyParam.qtyPercent, when supplied, is a whole number in 1-99.
//   - strategyParam.interval, when supplied, is not negative.
func (s *AlgoService) ChangeOrder(ctx context.Context, accountID domain.AccountID, change AlgoChangeOrderRequest) (domain.OrderID, error) {
	if accountID.IsZero() {
		return "", errs.New(types.StatusInvalidParam, opAlgoChangeOrder, "accountID must not be empty")
	}
	if err := algoValidateOrderID(opAlgoChangeOrder, change.OrderID); err != nil {
		return "", err
	}
	if err := algoValidateStockCode(opAlgoChangeOrder, change.StockCode); err != nil {
		return "", err
	}
	if err := algoValidateExchangeType(opAlgoChangeOrder, change.ExchangeType); err != nil {
		return "", err
	}
	if err := algoValidateOrderPrice(opAlgoChangeOrder, change.Price); err != nil {
		return "", err
	}
	if err := algoValidateOrderQuantity(opAlgoChangeOrder, change.Quantity); err != nil {
		return "", err
	}
	if err := algoValidateOptionalSensitivity(opAlgoChangeOrder, change.StrategyParam.Sensitivity); err != nil {
		return "", err
	}
	if err := algoValidateStrategyParamTimes(opAlgoChangeOrder, change.StrategyParam); err != nil {
		return "", err
	}
	if err := algoValidateMaxVolume(opAlgoChangeOrder, change.StrategyParam.MaxVolume); err != nil {
		return "", err
	}
	if err := algoValidateOptionalMoney(opAlgoChangeOrder, "minAmount", change.StrategyParam.MinAmount); err != nil {
		return "", err
	}
	if err := algoValidateOptionalQuantity(opAlgoChangeOrder, "showQty", change.StrategyParam.ShowQty); err != nil {
		return "", err
	}
	if err := algoValidateQtyPercent(opAlgoChangeOrder, change.StrategyParam.QtyPercent); err != nil {
		return "", err
	}
	if err := algoValidateInterval(opAlgoChangeOrder, change.StrategyParam.Interval); err != nil {
		return "", err
	}

	req := algoChangeOrderWireRequest{
		OrderID:       change.OrderID.String(),
		ExchangeType:  string(change.ExchangeType),
		StockCode:     change.StockCode,
		EntrustAmount: change.Quantity.String(),
		EntrustPrice:  change.Price.String(),
		StrategyParam: algoStrategyParamWireFrom(change.StrategyParam),
	}

	var out algoMutationWireResponse
	if err := s.client.Do(ctx, opAlgoChangeOrder, client.RouteTradeAlgoChangeOrder,
		req, s.client.JSON(), &out); err != nil {
		return "", err
	}
	return domain.OrderID(out.Data), nil
}

// ActionOrder starts, stops, suspends or resumes a live algorithm master order on
// /trade/AlgoActionOrder.
//
// # The body is not like the other four mutations
//
// It carries no price, no quantity and no strategyParam, and it has an `action`
// key none of the others has. Its four codes are documented identically by the
// reference, the released layer and both vendors, and they are validated against
// that closed set: an unlisted action on a live order is a request the SDK already
// knows it does not mean, so it is refused here rather than sent. The Python SDK
// makes target_strategy optional, and the reference's own request example omits
// it; this method requires it anyway, because the released pkg/hstong/algo has
// required it since v0.1.0 and no working caller can be relying on its absence.
//
// # It issues exactly one attempt
//
// Under any configuration, for the reason AddOrder states. A stop is a
// live-state mutation and the Gateway's own timeout can leave the caller unable to
// say whether the strategy is still running.
//
// # What to do after a failure
//
// **Reconcile; never resubmit, and never follow a failed stop with a cancel.** The
// error is a typed *errs.Error carrying opAlgoActionOrder, with a Gateway
// rejection keeping its own code — a "1007" in errs.CategoryTrading is the
// ambiguous case. Then call QueryOrderList narrowed by StockCode and ExchangeType
// and read two fields on the matching domain.AlgoMasterOrder:
//
//   - StrategyStatus, which is the field whose value is in question. It shares its
//     four codes with the action: AlgoStrategyStatusStop means the strategy is
//     stopped, AlgoStrategyStatusSuspend means paused but still holding children,
//     and AlgoStrategyStatusStart or AlgoStrategyStatusResume means running.
//   - Status, which tells a master that has gone (AlgoStatusCancelled,
//     AlgoStatusCompleted, AlgoStatusRejected) from one that is still live.
//
// A stop that raced a platform slice leaves children behind, so
// QueryEntrustIDList is the second call: a master reported stopped with children
// still listed is a reconcile-in-progress, not a failure to retry against.
//
// # Validation, all of it local and all of it free
//
// Each refusal is a types.StatusInvalidParam *errs.Error under opAlgoActionOrder
// costing zero HTTP requests:
//
//   - accountID is not zero.
//   - OrderID is not blank, trimmed.
//   - TargetStrategy is not empty, and is not value-bounded for the reason
//     algoValidateStrategy states.
//   - ExchangeType names one of K, P, v or t.
//   - Action is one of 1 (START), 2 (STOP), 3 (SUSPEND) or 4 (RESUME).
func (s *AlgoService) ActionOrder(ctx context.Context, accountID domain.AccountID, req AlgoActionRequest) (domain.OrderID, error) {
	if accountID.IsZero() {
		return "", errs.New(types.StatusInvalidParam, opAlgoActionOrder, "accountID must not be empty")
	}
	if err := algoValidateOrderID(opAlgoActionOrder, req.OrderID); err != nil {
		return "", err
	}
	if err := algoValidateStrategy(opAlgoActionOrder, req.TargetStrategy); err != nil {
		return "", err
	}
	if err := algoValidateExchangeType(opAlgoActionOrder, req.ExchangeType); err != nil {
		return "", err
	}
	if err := algoValidateAction(opAlgoActionOrder, req.Action); err != nil {
		return "", err
	}

	body := algoActionOrderWireRequest{
		OrderID:        req.OrderID.String(),
		ExchangeType:   string(req.ExchangeType),
		Action:         string(req.Action),
		TargetStrategy: string(req.TargetStrategy),
	}

	var out algoMutationWireResponse
	if err := s.client.Do(ctx, opAlgoActionOrder, client.RouteTradeAlgoActionOrder,
		body, s.client.JSON(), &out); err != nil {
		return "", err
	}
	return domain.OrderID(out.Data), nil
}
