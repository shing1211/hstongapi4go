// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file owns the futures request and the futures reply envelopes; the rows
// inside those envelopes and the domain models they map to live in pkg/domain,
// which is the split account.go and account.go's service half establish for the
// cash layer. The rule is one sentence: a payload the Gateway sends as a row
// becomes an exported …Wire type in pkg/domain, while an envelope that holds
// rows — and any request body — stays unexported here.
//
// That is also why the three assembly helpers at the foot of this file are
// here rather than beside the mappers. They take an envelope, which is this
// layer's to decode, and pair its parts into one domain value; the mappers they
// call take a single row. AccountService.HoldsList does the same thing with no
// helper at all, inlining the loop above domain.PositionFromDTO; the helper
// exists only because the futures endpoint methods that would inline it arrive
// with the read work, and inlining at that point is a one-function change.

// Operation labels used in errors raised by FuturesService. They are the
// canonical Gateway route paths, with the same convention the released
// pkg/hstong/future Manager uses, and they never carry a request payload or a
// credential.
//
// The names carry a Futures prefix because this package already declares
// opEntrust and opCancelEntrust for the cash endpoints: the two surfaces are
// different routes with different bodies, and a shared label would make one
// error indistinguishable from the other.
const (
	opFuturesQueryProductInfo      = "trade/FuturesQueryProductInfo"
	opFuturesQueryMaxBuySellAmount = "trade/FuturesQueryMaxBuySellAmount"
	opFuturesQueryFundInfo         = "trade/FuturesQueryFundInfo"
	opFuturesQueryHoldsList        = "trade/FuturesQueryHoldsList"
	opFuturesQueryRealEntrustList  = "trade/FuturesQueryRealEntrustList"
	opFuturesQueryHistoryEntrust   = "trade/FuturesQueryHistoryEntrustList"
	opFuturesQueryRealDeliverList  = "trade/FuturesQueryRealDeliverList"
	opFuturesQueryHistoryDeliver   = "trade/FuturesQueryHistoryDeliverList"
	opFuturesEntrust               = "trade/FuturesEntrust"
	opFuturesCancelEntrust         = "trade/FuturesCancelEntrust"
	opFuturesModifyEntrust         = "trade/FuturesModifyEntrust"
)

const (
	// futuresDefaultPageNo is the first page a history query asks for when the
	// caller leaves PageRequest.PageNo at zero. It is the Gateway's documented
	// default (page 1).
	futuresDefaultPageNo = 1
	// futuresDefaultPageSize is the page size a history query asks for when the
	// caller leaves PageRequest.PageSize at zero. It is the Gateway's
	// documented default (20) and it is the released pkg/hstong/future
	// DefaultPageSize, which is what a caller migrating from that surface
	// already sees.
	//
	// It is deliberately not a configurable option. The released
	// future.WithDefaultPageSize is a pkg/hstong surface, outside ADR 0011's
	// protected surface and outside this one, and reproducing the option here
	// would give the same default two places to change.
	futuresDefaultPageSize = 20
	// futuresDefaultOrderOptions is the order-option code substituted for a
	// caller who leaves FuturesOrderRequest.OrderOptions empty. Both vendor
	// SDKs send orderOptions on every entrust and both default it to "0", and
	// the field carries no omitempty, so an unset caller value would otherwise
	// put "" on the wire — a worse default than the documented one.
	futuresDefaultOrderOptions = "0"
)

// FuturesService is the use-case surface for the eleven futures endpoints.
//
// It holds no per-request state, is safe for concurrent use once constructed,
// and takes its request path from an Executor the caller supplies — the same
// inversion MarketService, AccountService and TradingService use
// (docs/adr/0010-vnext-layered-architecture.md). The endpoint methods arrive in
// the follow-on work; this type currently carries the wire and mapper scaffold,
// so nothing routes through it yet.
//
// Four facts about the futures protocol are structural and are stated here
// because each is a mistake a reader would otherwise make, and none of them can
// be checked by the mock Gateway (it answers by path and never inspects a
// request body):
//
//   - **No futures request carries a market.** There is no exchangeType and no
//     dataType on any of the eleven bodies, in either vendor SDK or in the
//     released pkg/hstong/future. A futures contract code is unique across the
//     Hong Kong and US books, and the response reads the book back
//     (domain.FuturesPosition.DataType). Adding a market for symmetry with the
//     cash layer would be a wire change on a guess, and an invisible one.
//   - **No futures request carries an account id.** The futures account and
//     book are both resolved from the authenticated session. The accountID
//     argument the endpoint methods take is therefore a local precondition — it
//     is validated and never placed in a wire struct. It must not be added to
//     one; that would leak an account identifier into every futures request.
//   - **Futures pagination is page-numbered, not cursor-based.** The two
//     history queries take pageNo/pageSize and the Gateway has no futures
//     cursor, which is why this package uses PageRequest rather than
//     transport.Pagination. The four unpaginated reads take no body at all.
//   - **Every futures price and quantity crosses the wire as a quoted string**,
//     with the single exception of the two int64 contract counts in
//     domain.FuturesCapacity. Nothing on the futures path is a float, and the
//     two constructors that could panic on a malformed reply are wrapped in
//     pkg/domain so an empty field maps to an explicit zero rather than
//     crashing a read.
type FuturesService struct {
	client Executor
}

// FuturesOption configures a FuturesService. Options are applied in order on
// top of the client passed to NewFuturesService, and the last option that sets
// the request path wins.
type FuturesOption func(*FuturesService)

// WithFuturesClient sets the request path a FuturesService issues its calls
// through. It is the same option shape the other three services expose, and it
// exists so a caller (or a test) can supply something other than a live client.
func WithFuturesClient(c Executor) FuturesOption {
	return func(s *FuturesService) { s.client = c }
}

// NewFuturesService returns a FuturesService over c with opts applied in order.
// c is not owned by the service: closing the underlying client remains the
// caller's responsibility. Every futures endpoint requires a live trade session,
// so a request issued before one is established is rejected by the Gateway
// rather than by this constructor.
//
// A nil option panics, exactly as it does in NewAccountService and
// NewTradingService; the difference is deliberate rather than accidental and is
// the one place where this constructor is not total.
func NewFuturesService(c Executor, opts ...FuturesOption) *FuturesService {
	s := &FuturesService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// PageRequest is the page-number pagination the two futures history queries
// take. It is deliberately not transport.Pagination: that type is cursor-shaped
// and its Apply writes cursor/page_size keys the futures Gateway does not
// accept, so a shared struct would carry two incompatible meanings.
//
// The zero value is a valid first page: PageNo and PageSize are substituted
// with futuresDefaultPageNo (1) and futuresDefaultPageSize (20) at the mapping
// boundary, so a caller who fills in only the dates gets the Gateway's
// documented defaults rather than a malformed page.
//
// The bounds are not enforced by this type. A PageSize must end up positive and
// below 100 and a date must be yyyyMMdd, but both are validated by the
// endpoint method before a request is sent, which is where the rejection has to
// happen to keep a bad page off the wire.
//
// There is no stockCode filter: neither vendor SDK offers one, so a futures
// history query cannot be narrowed to a single contract. That is a vendor
// limitation rather than an SDK choice, and it is a capability gap a caller
// will notice.
type PageRequest struct {
	// PageNo is the 1-based page number. Zero or negative is replaced with 1.
	PageNo int
	// PageSize is the number of rows per page. Zero or negative is replaced
	// with 20; a value of 100 or more is rejected by the endpoint method,
	// because the Gateway documents the accepted page size as strictly below
	// 100.
	PageSize int
	// StartDate is the query start date in yyyyMMdd form. Empty means
	// unbounded, which the released pkg/hstong/future also allows: both vendor
	// SDKs reject a blank date client-side, but a blank range is refused by the
	// Gateway with a clear message and no side effect, so refusing it locally
	// would break a caller migrating from the released surface for no gain.
	StartDate string
	// EndDate is the query end date in yyyyMMdd form, with the same optionality
	// as StartDate.
	EndDate string
}

// FuturesOrderRequest is the caller-facing body of a futures entrust.
//
// Its money is already domain-typed: Price and Quantity are decimal values, so
// a caller cannot lose a digit between composing the order and this struct
// receiving it. The four string fields are the ones the Gateway documents as
// codes rather than amounts, and two of them have no bounded set this repository
// can establish.
type FuturesOrderRequest struct {
	// Symbol identifies the contract. Only Code crosses the wire: futures
	// requests carry no market, so Symbol.Market is not sent and does not
	// select a book.
	Symbol domain.Symbol
	// Side is the direction, as types.EntrustBS: 1 opens a long, 2 closes a
	// long, 3 closes a short, 4 opens a short. All four are accepted, which is
	// the released surface's set and deliberately wider than the vendor's
	// two-value futures enum; narrowing it would be a caller-visible change on
	// a money-moving call made on incomplete evidence.
	Side types.EntrustBS
	// OrderType is the futures order type, as the Gateway's own code: 0 limit,
	// 1 auction, 2 market, and a fourth option code the Java vendor SDK
	// declares and the Python one does not. It is a string and is not validated
	// against a set, because the two vendor SDKs contradict each other and a
	// local closed set would reject a code one of them documents. The endpoint
	// method checks only that it is a non-empty run of decimal digits.
	OrderType string
	// Price is the order price. A market order still carries one: both vendors
	// send entrustPrice unconditionally and require it non-blank, so a
	// market-order caller passes the conventional "0" through
	// domain.MustNewPrice rather than relying on a Gateway tolerance for an
	// empty price.
	Price domain.Price
	// Quantity is the order quantity and must be strictly positive.
	Quantity domain.Quantity
	// ValidTimeType is the time-in-force code: 0 today, 1 deal-and-cancel,
	// 2 full-or-cancel, 3 good-till-expiry-date, 4 good-till-specified-date.
	//
	// It keeps the wire's own name rather than reusing domain.Order's
	// TimeInForce because the two value domains are disjoint: the cash field
	// holds "DAY"/"IOC"/"FOK"/"GTD" and this one holds "0" to "4".
	ValidTimeType string
	// ValidTime is the yyyyMMdd expiry. It is required if and only if
	// ValidTimeType is "4", and it is the only genuinely optional field on the
	// entrust body: the wire omits the key entirely otherwise. Note that the
	// *response* carries its expiry in yyyy/MM/dd form, so the two directions
	// use different formats and the response form must not be sent back.
	ValidTime string
	// OrderOptions is "0" for the default or "1" for T+1. Empty is replaced
	// with "0" at the mapping boundary, because the field has no omitempty and
	// an empty string is a worse default than the documented one.
	OrderOptions string
}

// FuturesModifyRequest is the caller-facing body of a futures modify.
//
// It is not FuturesOrderRequest with one field removed: a modify is a distinct
// operation with a bounded failure mode, and the two differences are load
// bearing. The entrust id names the order to change, and OrderType is absent
// because the Gateway's modify body has no such field — the order keeps the
// type it was placed with. A modify is also not a cancel followed by an
// entrust: it is one request instead of two, the order never leaves the book,
// and its priority survives, which under the no-auto-retry rule is the
// difference between one unreconciled mutation and two.
type FuturesModifyRequest struct {
	// EntrustID is the order to modify. It is required, and it is required
	// together with Symbol even though the id alone would identify the order:
	// the Gateway's body carries both.
	EntrustID domain.EntrustID
	// Symbol is the contract the order sits on. Only Code crosses the wire.
	Symbol domain.Symbol
	// Price is the new price and must be a well-formed non-negative decimal.
	Price domain.Price
	// Quantity is the new quantity and must be strictly positive.
	Quantity domain.Quantity
	// Side is the direction, with the same four accepted values as
	// FuturesOrderRequest.Side.
	Side types.EntrustBS
	// ValidTimeType is the time-in-force code, as
	// FuturesOrderRequest.ValidTimeType.
	ValidTimeType string
	// ValidTime is the yyyyMMdd expiry, required if and only if ValidTimeType
	// is "4".
	ValidTime string
	// OrderOptions is "0" or "1", defaulted to "0" when empty.
	OrderOptions string
}

// futuresProductInfoWireRequest is the body of /trade/FuturesQueryProductInfo.
// The singular key holding a list is the vendor's spelling and is not a typo;
// it is what both vendor SDKs send.
type futuresProductInfoWireRequest struct {
	StockCodes []string `json:"stockCode"`
}

// futuresMaxBuySellAmountWireRequest is the body of
// /trade/FuturesQueryMaxBuySellAmount. One contract: the reply has no list, so
// there is no plural variant of this key.
type futuresMaxBuySellAmountWireRequest struct {
	StockCode string `json:"stockCode"`
}

// futuresPageQueryWireRequest is the body of both futures history queries.
//
// One struct for both routes, because the vendor shares one parameter class and
// one request shape between them. The cash layer declares two identical structs
// for the same reason (historyEntrustListWireRequest and
// historyDeliverListWireRequest in trading.go); that duplication has no vendor
// justification and is not reproduced here.
//
// PageNo and PageSize carry no omitempty even though they are defaulted. A
// defaulted field is still a required one: the substitution happens before the
// marshal, so the emitted bytes are the same with or without the tag, and
// omitting it would make the wire body depend on a pre-substitution value — a
// refactor that moved the defaulting after the marshal would silently change
// the request. This is the one place where the v-next body differs from the
// released pkg/hstong/future body, which tags all four fields omitempty. The
// difference is observationally nil for every valid request.
type futuresPageQueryWireRequest struct {
	PageNo    int    `json:"pageNo"`
	PageSize  int    `json:"pageSize"`
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
}

// futuresEntrustWireRequest is the body of /trade/FuturesEntrust.
//
// The field order follows the vendor's request literal — stockCode, entrustType,
// entrustPrice, entrustAmount, entrustBs, validTimeType, orderOptions, and
// validTime last — which is cosmetic, since a JSON object is unordered, but it
// costs nothing and makes the two bodies diffable against the vendor source.
//
// entrustType, entrustPrice, validTimeType and orderOptions carry no omitempty
// where the released body does. All four are required by the vendor and always
// sent by it, and a required field that vanished from the body would be a
// request the Gateway resolves against its own default — on a mutation, with no
// retry to reveal the mistake.
type futuresEntrustWireRequest struct {
	StockCode     string `json:"stockCode"`
	EntrustType   string `json:"entrustType"`
	EntrustPrice  string `json:"entrustPrice"`
	EntrustAmount string `json:"entrustAmount"`
	EntrustBS     string `json:"entrustBs"`
	ValidTimeType string `json:"validTimeType"`
	OrderOptions  string `json:"orderOptions"`
	// ValidTime is the only conditional field on any futures request: it is
	// present if and only if validTimeType is "4", and it is omitted rather
	// than sent empty otherwise.
	ValidTime string `json:"validTime,omitempty"`
}

// futuresCancelEntrustWireRequest is the body of
// /trade/FuturesCancelEntrust. Two required fields, no omitempty, and nothing
// else exists on the wire to require. It is the smallest request body in the
// SDK and the only one whose shape is not a judgement call.
type futuresCancelEntrustWireRequest struct {
	EntrustID string `json:"entrustId"`
	StockCode string `json:"stockCode"`
}

// futuresModifyEntrustWireRequest is the body of
// /trade/FuturesModifyEntrust.
//
// It deliberately has no entrustType. Both vendors omit it — the request
// literal has no such key and the parameter class has no such field, while the
// entrust class has both — so a symmetry argument that mirrored the entrust
// body would add a field the Gateway does not read, and a change that retyped an
// order is a more powerful operation than this endpoint offers.
type futuresModifyEntrustWireRequest struct {
	EntrustID     string `json:"entrustId"`
	StockCode     string `json:"stockCode"`
	EntrustPrice  string `json:"entrustPrice"`
	EntrustAmount string `json:"entrustAmount"`
	EntrustBS     string `json:"entrustBs"`
	ValidTimeType string `json:"validTimeType"`
	OrderOptions  string `json:"orderOptions"`
	ValidTime     string `json:"validTime,omitempty"`
}

// The four account-scoped futures reads — /trade/FuturesQueryFundInfo,
// /trade/FuturesQueryHoldsList, /trade/FuturesQueryRealEntrustList and
// /trade/FuturesQueryRealDeliverList — send an empty params object and
// therefore have no wire struct here. Both vendors pass an empty map; the
// released layer passes struct{}{}, which marshals to the same {} byte for
// byte. The one thing the endpoint methods must get right is that it is
// struct{}{} and not nil: marshalling nil would send "params":null, which is a
// different body. An empty named struct is not declared for them on purpose —
// it would encode nothing and give a field somewhere to be added by accident.

// futuresProductInfoWireResponse is the data object of
// /trade/FuturesQueryProductInfo. The rows it holds are domain payloads
// (domain.FuturesProductInfoWire); only the wrapper is this layer's.
type futuresProductInfoWireResponse struct {
	ProductInfoVos []domain.FuturesProductInfoWire `json:"productInfoVos"`
}

// futuresFundInfoWireResponse is the data object of
// /trade/FuturesQueryFundInfo.
type futuresFundInfoWireResponse struct {
	FundInfo domain.FuturesFundInfoWire `json:"fundInfo"`
}

// futuresHoldsListWireResponse is the data object of
// /trade/FuturesQueryHoldsList.
//
// The funds snapshot and the position list are one atomic read and neither is
// derived from the other, so the reply keeps both rather than discarding one.
// Pairing them into one domain.FuturesHoldResult is the assembly helper's job
// below, not a mapper's.
type futuresHoldsListWireResponse struct {
	FundInfo  domain.FuturesFundInfoWire `json:"fundInfo"`
	HoldsList []domain.FuturesHoldWire   `json:"holdsList"`
}

// futuresOrderListWireResponse is the data object of all four futures list
// routes: the real and history entrust queries and the real and history deliver
// queries. The pagination fields are populated only by a history query; a real
// query reports them as zero.
type futuresOrderListWireResponse struct {
	Data        []domain.FuturesOrderWire `json:"data"`
	CurPageNo   int32                     `json:"curPageNo"`
	CurPageSize int32                     `json:"curPageSize"`
	TotalPageNo int32                     `json:"totalPageNo"`
	LastPage    int32                     `json:"lastPage"`
}

// futuresMutationWireResponse is the data object of all three futures
// mutations. It is the cash commonStringResponse by alias rather than a second
// identical struct: SPEC.md types all three as a single `data` string, and the
// cash layer already shares one type across its own mutations.
//
// Data is the resulting order number, and the Gateway does not guarantee that it
// is populated — it is documented as the empty string when omitted. A caller
// must therefore reconcile by querying the order list rather than by trusting
// this field.
type futuresMutationWireResponse = commonStringResponse

// futuresPageQueryParams builds the history page body from the caller's page.
//
// It performs the two substitutions that belong at the mapping boundary rather
// than in the caller's struct: a non-positive PageNo becomes 1 and a
// non-positive PageSize becomes 20. Both are the Gateway's documented defaults.
//
// It deliberately performs no validation. The bounds — a page size below 100, a
// yyyyMMdd start and end date — are the endpoint method's to enforce, because
// only there can a rejection be a typed error returned before a request is sent.
func futuresPageQueryParams(page PageRequest) futuresPageQueryWireRequest {
	pageNo, pageSize := page.PageNo, page.PageSize
	if pageNo <= 0 {
		pageNo = futuresDefaultPageNo
	}
	if pageSize <= 0 {
		pageSize = futuresDefaultPageSize
	}
	return futuresPageQueryWireRequest{
		PageNo:    pageNo,
		PageSize:  pageSize,
		StartDate: page.StartDate,
		EndDate:   page.EndDate,
	}
}

// futuresOrderOptionsParam substitutes the Gateway's default order option for
// an empty one.
//
// The field carries no omitempty and both vendors always send it, so an
// unsupplied value would reach the wire as "" — a worse default than the
// documented "0", and a rejection rather than a benign omission.
func futuresOrderOptionsParam(options string) string {
	if options == "" {
		return futuresDefaultOrderOptions
	}
	return options
}

// futuresHoldResultFromDTO maps the position query's two-part reply. A nil
// reply yields a nil result; an absent list yields a non-nil empty slice.
//
// It is an assembly helper and not a mapper, and the difference is structural
// rather than stylistic: it takes this layer's envelope, so it cannot live
// beside domain.FuturesPositionFromDTO. Each part is mapped by the per-row
// mappers, exactly as AccountService.HoldsList maps each
// domain.HoldsVoWire with domain.PositionFromDTO.
func futuresHoldResultFromDTO(v *futuresHoldsListWireResponse) *domain.FuturesHoldResult {
	if v == nil {
		return nil
	}
	positions := make([]*domain.FuturesPosition, len(v.HoldsList))
	for i := range v.HoldsList {
		positions[i] = domain.FuturesPositionFromDTO(&v.HoldsList[i])
	}
	return &domain.FuturesHoldResult{
		Account:   domain.FuturesAccountFromDTO(&v.FundInfo),
		Positions: positions,
	}
}

// futuresOrderPageFromDTO maps a paged order reply. A nil reply yields a nil
// page; an absent list yields a non-nil empty slice.
//
// An assembly helper for the reason futuresHoldResultFromDTO states: the page
// counters live in the envelope this layer decodes, so the page cannot be built
// by a mapper that sees only rows.
func futuresOrderPageFromDTO(v *futuresOrderListWireResponse) *domain.FuturesOrderPage {
	if v == nil {
		return nil
	}
	orders := make([]*domain.FuturesOrder, len(v.Data))
	for i := range v.Data {
		orders[i] = domain.FuturesOrderFromDTO(&v.Data[i])
	}
	return &domain.FuturesOrderPage{
		Orders:      orders,
		CurPageNo:   v.CurPageNo,
		CurPageSize: v.CurPageSize,
		TotalPageNo: v.TotalPageNo,
		LastPage:    v.LastPage == 1,
	}
}

// futuresFillPageFromDTO maps a paged fill reply. A nil reply yields a nil
// page; an absent list yields a non-nil empty slice. It is an assembly helper
// for the reason futuresHoldResultFromDTO states, and it reads the same envelope
// as futuresOrderPageFromDTO — the Gateway documents one object for both lists.
func futuresFillPageFromDTO(v *futuresOrderListWireResponse) *domain.FuturesFillPage {
	if v == nil {
		return nil
	}
	fills := make([]*domain.FuturesFill, len(v.Data))
	for i := range v.Data {
		fills[i] = domain.FuturesFillFromDTO(&v.Data[i])
	}
	return &domain.FuturesFillPage{
		Fills:       fills,
		CurPageNo:   v.CurPageNo,
		CurPageSize: v.CurPageSize,
		TotalPageNo: v.TotalPageNo,
		LastPage:    v.LastPage == 1,
	}
}
