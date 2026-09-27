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
// (docs/adr/0010-vnext-layered-architecture.md). Eight of the eleven endpoints
// are implemented here; the three mutations arrive with the mutation work, so
// nothing in this package can place, cancel or reprice an order yet.
//
// Every method takes a domain.AccountID first, validates it, and never places it
// in a request body — see the account rule on this type. The three mutations
// that would carry it are the reason it is first and unconditional: a caller
// that named no account is refused before the Gateway sees anything.
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
// struct{}{} and not nil: internal/transport drops a nil params from the
// envelope entirely (request.Params carries omitempty), so the key would be
// absent rather than an empty object — a different body, and one the four
// reads' assertions below would not notice. An empty named struct is not
// declared for them on purpose — it would encode nothing and give a field
// somewhere to be added by accident.

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

// futuresMaxPageSizeExclusive is the exclusive upper bound the Gateway documents
// for a futures history page: an accepted page size is strictly below 100.
//
// It deliberately repeats the value pkg/services' cash MaxPageSize encodes rather
// than reusing that constant. The two agree today by documentation and by the
// released pkg/hstong/future maxPageSizeExclusive, but they are different limits
// on different axes — the cash one caps a cursor page size that clampPageSize
// silently narrows to, while this one is a hard rejection on a page-numbered body
// — so sharing one identifier would let a change to either silently move the
// other.
const futuresMaxPageSizeExclusive = 100

// futuresDateLayout is the Go reference-time layout for the yyyyMMdd date the
// futures history body takes. The response's own expiry field uses yyyy/MM/dd,
// which is a different direction and must not be sent back.
const futuresDateLayout = "20060102"

// futuresValidateStockCode reports whether code is a usable futures contract code:
// non-empty once trimmed, and carrying no internal whitespace.
//
// The check is deliberately permissive about characters, because no futures code
// pattern is documented in this repository: the HK and US code formats differ and
// neither is recorded, so a pattern would reject legitimate contracts. The
// released pkg/hstong/future predicate is reproduced exactly, including its one
// quirk: a code with *leading or trailing* whitespace passes, because only
// interior whitespace is tested, and the caller's spelling is what goes on the
// wire. That is parity rather than an oversight, and the failure direction is the
// loud one — a padded code is refused by the Gateway with a message and no side
// effect, and this is a read.
func futuresValidateStockCode(op, code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return errs.New(types.StatusInvalidParam, op, "stockCode must not be empty")
	}
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return errs.New(types.StatusInvalidParam, op, "stockCode must not contain whitespace")
	}
	return nil
}

// futuresDateShaped reports whether date is eight decimal digits and nothing else.
//
// It is separate from the calendar check so a caller that passed "2026-09-26" is
// told the format is wrong rather than that the date does not exist, which is the
// distinction the released layer draws and the one that tells a caller which of
// its two mistakes to fix. The check is a digit scan and never a parse, so it
// cannot be defeated by a value a float would have mangled.
func futuresDateShaped(date string) bool {
	if len(date) != 8 {
		return false
	}
	for i := 0; i < len(date); i++ {
		if date[i] < '0' || date[i] > '9' {
			return false
		}
	}
	return true
}

// futuresValidateDate reports whether date is a real yyyyMMdd calendar date. An
// eight-digit string that is not a date on the calendar ("20260230") is rejected
// as a calendar failure rather than accepted because time.Parse is the only
// authority on what exists.
func futuresValidateDate(op, field, date string) error {
	if !futuresDateShaped(date) {
		return errs.New(types.StatusInvalidParam, op, field+" must be yyyyMMdd")
	}
	if _, err := time.Parse(futuresDateLayout, date); err != nil {
		return errs.New(types.StatusInvalidParam, op, field+" must be a valid calendar date")
	}
	return nil
}

// futuresValidatePage turns a caller's page into the wire body, rejecting the two
// inputs the Gateway documents it will not accept before a request is sent.
//
// The bound is a rejection rather than a clamp, and the difference matters: a
// clamp would send a page the caller did not ask for and report no error, so a
// caller paging through a large history would silently fetch 99 rows per call
// while believing it fetched 500. Both history methods therefore issue zero HTTP
// requests for a page they will not send, which is the "no pre-flight request"
// property ADR 0003 relies on for the mutations and the same discipline applied
// to a read.
//
// The dates are optional, matching the released layer and design-futures-requests
// §4.1: an empty bound is a legitimate unbounded query, and a *supplied* bound is
// format-checked. The vendor SDKs reject a blank range client-side, but refusing
// one here would break a caller migrating from the released surface for no gain —
// the Gateway refuses an unbounded range with a clear message and no side effect.
func futuresValidatePage(op string, page PageRequest) (futuresPageQueryWireRequest, error) {
	if page.PageSize >= futuresMaxPageSizeExclusive {
		return futuresPageQueryWireRequest{},
			errs.New(types.StatusInvalidParam, op, "pageSize must be below 100")
	}
	if page.StartDate != "" {
		if err := futuresValidateDate(op, "startDate", page.StartDate); err != nil {
			return futuresPageQueryWireRequest{}, err
		}
	}
	if page.EndDate != "" {
		if err := futuresValidateDate(op, "endDate", page.EndDate); err != nil {
			return futuresPageQueryWireRequest{}, err
		}
	}
	return futuresPageQueryParams(page), nil
}

// ---------------------------------------------------------------------------
// The eight futures reads
// ---------------------------------------------------------------------------

// QueryProductInfo returns the product series for the requested contract codes.
//
// codes must hold at least one entry and every entry must be a usable contract
// code — non-empty and free of internal whitespace — or the call is refused with
// a types.StatusInvalidParam *errs.Error carrying opFuturesQueryProductInfo and
// zero HTTP requests. The check is deliberately permissive about characters: no
// futures code pattern is documented in this repository, so "HSI2609.HK" and
// "01810.HK" both pass and neither is checked against an invented grammar.
//
// The result is the non-nil empty slice when the Gateway matched nothing, so a
// caller ranging over it behaves the same as for a populated reply. A code the
// Gateway does not recognise is simply absent from the result rather than an
// error: the reply is a filter, not a contract, and a caller passing a batch of
// codes wants the subset that resolved.
//
// No market is sent and none is needed — a futures contract code is unique across
// the Hong Kong and US books — and accountID names the session rather than the
// request. DecInPrice on each result is the price decimal power and is the
// futures tick source; see domain.FuturesProduct.
func (s *FuturesService) QueryProductInfo(ctx context.Context, accountID domain.AccountID, codes []string) ([]*domain.FuturesProduct, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryProductInfo, "accountID must not be empty")
	}
	if len(codes) == 0 {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryProductInfo,
			"stockCode must contain at least one contract code")
	}
	for _, code := range codes {
		if err := futuresValidateStockCode(opFuturesQueryProductInfo, code); err != nil {
			return nil, err
		}
	}

	var out futuresProductInfoWireResponse
	if err := s.client.Do(ctx, opFuturesQueryProductInfo, client.RouteTradeFuturesQueryProductInfo,
		futuresProductInfoWireRequest{StockCodes: codes}, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	products := make([]*domain.FuturesProduct, len(out.ProductInfoVos))
	for i := range out.ProductInfoVos {
		products[i] = domain.FuturesProductFromDTO(&out.ProductInfoVos[i])
	}
	return products, nil
}

// QueryMaxBuySellAmount returns the buying and selling power for one futures
// contract.
//
// symbol supplies only Code: futures requests carry no market, so symbol.Market is
// neither sent nor used to select a book, and a caller that sets it gains nothing
// and loses the ability to tell that it is not being honoured. A code that is
// empty or carries internal whitespace is refused with a types.StatusInvalidParam
// *errs.Error carrying opFuturesQueryMaxBuySellAmount and zero HTTP requests.
//
// The two counts are the only int64 numerics in any futures reply, and they are
// contract counts rather than money. domain.FuturesCapacityFromDTO converts them
// with strconv.FormatInt, so a count above 2^53 survives exactly where a float64
// would round one silently, and a null count — the Gateway's way of saying "you
// can buy nothing" — becomes an explicit zero rather than an absent value.
//
// The reply names no contract, so nothing here can recover which symbol was asked
// about; a caller correlating several contracts keeps its own key. InitialMargin
// is money in the reply's own currency and crosses verbatim.
func (s *FuturesService) QueryMaxBuySellAmount(ctx context.Context, accountID domain.AccountID, symbol domain.Symbol) (*domain.FuturesCapacity, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryMaxBuySellAmount, "accountID must not be empty")
	}
	if err := futuresValidateStockCode(opFuturesQueryMaxBuySellAmount, symbol.Code); err != nil {
		return nil, err
	}

	var out domain.FuturesMaxBuySellAmountWire
	if err := s.client.Do(ctx, opFuturesQueryMaxBuySellAmount, client.RouteTradeFuturesQueryMaxBuySellAmount,
		futuresMaxBuySellAmountWireRequest{StockCode: symbol.Code}, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.FuturesCapacityFromDTO(&out), nil
}

// QueryFundInfo returns the futures account's funds snapshot: net asset value,
// cash, buying power, margins and the risk state.
//
// It takes no request body, so it sends the empty params object struct{}{} —
// never nil, which internal/transport would drop from the envelope entirely
// rather than send as an empty object. A zero accountID is refused before
// anything is sent, with a types.StatusInvalidParam *errs.Error carrying
// opFuturesQueryFundInfo.
//
// Every monetary field crosses the wire as a quoted string and is read verbatim:
// nothing on this path is a float. An absent field maps to an explicit zero rather
// than panicking, because a partial reply is a read and a read must not crash the
// caller's process — the cash mappers hand "" to a MustNew… constructor and would
// panic, and that is the layer's known F2 defect, not the behaviour to copy.
//
// Read Money.Currency() before summing across accounts: an amount whose field
// name states a currency carries it, and every other amount is attributed the base
// currency because the Gateway sends no futures base-currency field.
func (s *FuturesService) QueryFundInfo(ctx context.Context, accountID domain.AccountID) (*domain.FuturesAccount, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryFundInfo, "accountID must not be empty")
	}

	var out futuresFundInfoWireResponse
	if err := s.client.Do(ctx, opFuturesQueryFundInfo, client.RouteTradeFuturesQueryFundInfo,
		struct{}{}, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.FuturesAccountFromDTO(&out.FundInfo), nil
}

// QueryHoldsList returns the account's futures positions together with the funds
// snapshot the Gateway sends alongside them.
//
// The result is a *domain.FuturesHoldResult struct rather than a position slice
// on purpose: the reply is one atomic read carrying both halves, neither derived
// from the other, and returning the positions alone would silently discard a funds
// snapshot the caller asked for. Account is therefore never nil for a successful
// call, and Positions is the non-nil empty slice when nothing is held.
//
// It takes no request body and sends struct{}{}. A zero accountID is refused
// before anything is sent, with a types.StatusInvalidParam *errs.Error carrying
// opFuturesQueryHoldsList. Every price, quantity and amount on a position crosses
// as a quoted string and is read verbatim; an absent field becomes an explicit zero
// rather than a panic, as QueryFundInfo states. DataType on each position is the
// futures-only field that says which book the contract belongs to, and is the
// reason no request on this path needs a market.
func (s *FuturesService) QueryHoldsList(ctx context.Context, accountID domain.AccountID) (*domain.FuturesHoldResult, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryHoldsList, "accountID must not be empty")
	}

	var out futuresHoldsListWireResponse
	if err := s.client.Do(ctx, opFuturesQueryHoldsList, client.RouteTradeFuturesQueryHoldsList,
		struct{}{}, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return futuresHoldResultFromDTO(&out), nil
}

// QueryRealEntrustList returns today's futures orders for the account.
//
// It is the unpaginated query: it sends struct{}{} and one call returns the whole
// day, so it does not walk pages — futures pagination is page-numbered and has no
// cursor to walk — and it returns a slice rather than a page, because a reply with
// no page state must not be dressed up with four zero-valued page fields. The
// result is the non-nil empty slice when nothing is outstanding.
//
// A zero accountID is refused before anything is sent, with a
// types.StatusInvalidParam *errs.Error carrying opFuturesQueryRealEntrustList.
// CanBeCanceled on each order is the field that distinguishes a live order from
// one that has already gone, and it is also how a caller reconciles an ambiguous
// futures mutation (docs/adr/0003-no-auto-retry-orders.md): this method and
// QueryHistoryEntrustPage are the reconciliation queries for a submit, a cancel or a
// modify, and neither of those retries.//
// Every price and quantity on a row crosses as a quoted string and is read
// verbatim; an absent field becomes an explicit zero rather than a panic.
func (s *FuturesService) QueryRealEntrustList(ctx context.Context, accountID domain.AccountID) ([]*domain.FuturesOrder, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryRealEntrustList, "accountID must not be empty")
	}

	var out futuresOrderListWireResponse
	if err := s.client.Do(ctx, opFuturesQueryRealEntrustList, client.RouteTradeFuturesQueryRealEntrustList,
		struct{}{}, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	orders := make([]*domain.FuturesOrder, len(out.Data))
	for i := range out.Data {
		orders[i] = domain.FuturesOrderFromDTO(&out.Data[i])
	}
	return orders, nil
}

// QueryHistoryEntrustPage returns one page of futures orders for the account.
//
// The method returns a page, and only the history route populates the page counters
// it reports: a real-list reply carries curPageNo, curPageSize, totalPageNo and
// lastPage as zeros, so a page built from it would be a fiction with four zero
// fields. That is why this reads the history route rather than
// FuturesQueryRealEntrustList, and why it is named for what it calls.
// design-futures-requests.md §7.2 point 3 agrees, while the signature block in that
// same section misnamed it QueryRealEntrustPage -- the name is corrected here, at the
// point where the two were still free to change, because pkg/services is not yet
// reachable by a caller and a method named "Real" that reads the history route is a
// trap to freeze into a public API at v1.0.
//
// page.PageNo and page.PageSize are defaulted to 1 and 20 at the mapping boundary
// when they are zero or negative, matching the Gateway's documented defaults and
// the released pkg/hstong/future. A page size of 100 or more is refused, not
// clamped, and both dates must be yyyyMMdd calendar dates when supplied; each
// refusal is a types.StatusInvalidParam *errs.Error carrying
// opFuturesQueryHistoryEntrust and costs zero HTTP requests. An empty date bound
// is legitimate and means unbounded, which is the released layer's behaviour and
// design-futures-requests.md §4.1's deliberate divergence from the vendor SDKs'
// client-side guard.
//
// There is no stockCode filter: neither vendor SDK offers one, so a futures
// history query cannot be narrowed to a single contract. That is a vendor
// limitation a caller will notice, not an SDK choice. Orders is the non-nil empty
// slice for an empty page, and LastPage is true only for the wire's 1.
//
// This is one of the two reconciliation queries for an ambiguous futures
// mutation, alongside QueryRealEntrustList.
func (s *FuturesService) QueryHistoryEntrustPage(ctx context.Context, accountID domain.AccountID, page PageRequest) (*domain.FuturesOrderPage, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryHistoryEntrust, "accountID must not be empty")
	}
	params, err := futuresValidatePage(opFuturesQueryHistoryEntrust, page)
	if err != nil {
		return nil, err
	}

	var out futuresOrderListWireResponse
	if err := s.client.Do(ctx, opFuturesQueryHistoryEntrust, client.RouteTradeFuturesQueryHistoryEntrustList,
		params, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return futuresOrderPageFromDTO(&out), nil
}

// QueryRealDeliverList returns today's futures fills for the account.
//
// It is the unpaginated query, so it sends struct{}{}, one call returns the whole
// day, and it returns a slice rather than a page: a real-list reply carries no
// page state, and QueryHistoryDeliverPage is the method that does. The result is the
// non-nil empty slice when nothing traded.
//
// A zero accountID is refused before anything is sent, with a
// types.StatusInvalidParam *errs.Error carrying opFuturesQueryRealDeliverList.
// This is one of the two reconciliation queries for an ambiguous futures mutation
// (docs/adr/0003-no-auto-retry-orders.md); a caller reconciling a fill reads this
// rather than QueryRealEntrustList, because a fill and a resting order are
// different facts even though the Gateway documents one object for both.
//
// FilledQty is the filled quantity and Quantity the order quantity, which exceeds
// it on a partially filled order. Every price and quantity crosses as a quoted
// string and is read verbatim.
func (s *FuturesService) QueryRealDeliverList(ctx context.Context, accountID domain.AccountID) ([]*domain.FuturesFill, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryRealDeliverList, "accountID must not be empty")
	}

	var out futuresOrderListWireResponse
	if err := s.client.Do(ctx, opFuturesQueryRealDeliverList, client.RouteTradeFuturesQueryRealDeliverList,
		struct{}{}, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	fills := make([]*domain.FuturesFill, len(out.Data))
	for i := range out.Data {
		fills[i] = domain.FuturesFillFromDTO(&out.Data[i])
	}
	return fills, nil
}

// QueryHistoryDeliverPage returns one page of futures fills for the account.
//
// This reads the history route rather than FuturesQueryRealDeliverList, and is named
// for what it calls, for the reason QueryHistoryEntrustPage states: only the history
// route populates the page counters a page reports.
//
// page.PageNo and page.PageSize are defaulted to 1 and 20 when zero or negative;
// a page size of 100 or more is refused rather than clamped, and both dates must
// be yyyyMMdd calendar dates when supplied. Each refusal is a
// types.StatusInvalidParam *errs.Error carrying opFuturesQueryHistoryDeliver and
// costs zero HTTP requests. An empty date bound means unbounded, and there is no
// stockCode filter — a vendor limitation, as QueryHistoryEntrustPage states.
//
// Fills is the non-nil empty slice for an empty page, and LastPage is true only
// for the wire's 1.
func (s *FuturesService) QueryHistoryDeliverPage(ctx context.Context, accountID domain.AccountID, page PageRequest) (*domain.FuturesFillPage, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opFuturesQueryHistoryDeliver, "accountID must not be empty")
	}
	params, err := futuresValidatePage(opFuturesQueryHistoryDeliver, page)
	if err != nil {
		return nil, err
	}

	var out futuresOrderListWireResponse
	if err := s.client.Do(ctx, opFuturesQueryHistoryDeliver, client.RouteTradeFuturesQueryHistoryDeliverList,
		params, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return futuresFillPageFromDTO(&out), nil
}
