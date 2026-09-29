// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels used in errors raised by TradingService. Each is the canonical
// Gateway route path, so a log line or an error points at the failing endpoint,
// and none ever carries a request payload or a credential.
//
// The last two are the trade-push subscription labels. They are declared here
// rather than beside the two trade-session labels in session.go because a
// subscription is not a login — it is a change to the Gateway's push state, not
// to the session that authorises it — and a label shared with a login would make
// the two errors indistinguishable. The spellings are the released layer's,
// from pkg/hstong/trade/push.go:14-17.
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
	opTradeSubscribe        = "trade/TradeSubscribe"
	opTradeUnsubscribe      = "trade/TradeUnsubscribe"
)

const (
	maxValidDays      = 100
	defaultPageSize   = 50
	maxPageSize       = 500
	idempotencyKeyLen = 32
)

var knownHKOrderTypes = map[types.EntrustType]struct{}{
	types.EntrustTypeAuctionLimit:      {},
	types.EntrustTypeAuction:           {},
	types.EntrustTypeEnhancedLimit:     {},
	types.EntrustTypeLimit:             {},
	types.EntrustTypeSpecialLimit:      {},
	types.EntrustTypeDarkPool:          {},
	types.EntrustTypeOddLot:            {},
	types.EntrustTypeStopProfitLimit:   {},
	types.EntrustTypeStopLossLimit:     {},
	types.EntrustTypeTrailingStopLimit: {},
}

var hkSessionWindows = map[string]struct {
	preMarketStart  string
	preMarketEnd    string
	regularStart    string
	regularEnd      string
	afterHoursStart string
	afterHoursEnd   string
}{
	"HK": {
		preMarketStart:  "09:00",
		preMarketEnd:    "09:30",
		regularStart:    "09:30",
		regularEnd:      "12:00",
		afterHoursStart: "12:00",
		afterHoursEnd:   "16:00",
	},
}

// hkZone is the Hong Kong wall clock the session windows in hkSessionWindows are
// expressed in. It is a fixed +08:00 zone rather than a
// time.LoadLocation("Asia/Hong_Kong") lookup, and that is a correctness decision,
// not a shortcut: Hong Kong has observed no daylight saving since 1979, so for
// every instant this SDK can be asked about (time.Now()) the IANA zone resolves
// to a constant +08:00 offset and a FixedZone is exact, not an approximation.
//
// A LoadLocation call is not, by contrast, a total function here. It reads a
// host-provided database (the OS zoneinfo, $ZONEINFO, or $GOROOT/lib/time/zoneinfo.zip)
// and returns (nil, err) when none is available — a scratch container, a
// trimmed Windows install, a statically linked binary. Discarding that error
// left a nil *Location to reach Time.In, which panics on nil, so a missing tz
// database would crash the entrust path rather than reject an order.
//
// Neither degraded alternative is wanted now that the lookup is gone. Falling
// back to UTC would shift every window by eight hours and silently validate an
// order against the wrong clock — the exact outcome a fail-closed check exists to
// prevent. Failing closed would refuse every HK entrust on a host without tz
// data, trading a rare crash for a routine rejection of valid orders. Because a
// correct total answer exists, validateSessionWindow has no error path at all.
//
// Never reassign hkZone: Time.In panics on a nil Location, and the point of the
// fixed zone is that it cannot be nil.
var hkZone = time.FixedZone("HKT", 8*60*60)

type TradingService struct {
	client Executor
	// schedule resolves the instrument's lot size and tick for order validation.
	// Nil means DefaultHKTickSchedule, so a caller that never injects one gets
	// exactly the behaviour it had before the interface existed. See
	// tickSchedule.
	schedule domain.TickScheduleResolver
}

type TradingOption func(*TradingService)

func WithTradingClient(c Executor) TradingOption {
	return func(s *TradingService) { s.client = c }
}

// WithTickSchedule injects the resolver used to validate order prices and
// quantities against the instrument's own grid.
//
// Without it, validation falls back to the default HK table, which is keyed on
// DataType and is therefore a data-type-level approximation rather than a
// per-instrument one. Supplying a real instrument master is what makes the
// price check mean anything: the default table returns a grid, and a grid
// invented by the SDK is the defect this option exists to let a caller replace.
// See docs/runs/2026-09-26-vnext-parity-wire/design-tick-model.md §2.2.
//
// A nil resolver is ignored rather than stored, because a nil schedule must
// mean "use the default" rather than a nil dereference on the first order.
func WithTickSchedule(r domain.TickScheduleResolver) TradingOption {
	return func(s *TradingService) {
		if r != nil {
			s.schedule = r
		}
	}
}

// tickSchedule returns the resolver to use, substituting the default table when
// none was injected.
//
// The nil check is the whole guard. An option that stored an untyped nil would
// leave a non-nil interface value holding no resolver, and the ScheduleFor call
// below would panic on the first order rather than fall back.
func (s *TradingService) tickSchedule() domain.TickScheduleResolver {
	if s.schedule == nil {
		return domain.TickSchedule{}
	}
	return s.schedule
}

func NewTradingService(c Executor, opts ...TradingOption) *TradingService {
	s := &TradingService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type EntrustFilter struct {
	ExchangeType types.ExchangeType
	EntrustIDs   []domain.EntrustID
}

type DeliverFilter struct {
	ExchangeType types.ExchangeType
}

type CondOrderFilter struct {
	ExchangeType types.ExchangeType
	StockCode    string
	PageNo       int
	PageSize     int
}

type HistoryFilter struct {
	ExchangeType types.ExchangeType
	StartDate    string
	EndDate      string
	StockCode    string
	PageNo       int
	PageSize     int
}

func (s *TradingService) Entrust(ctx context.Context, accountID domain.AccountID, order domain.Order) (*domain.OrderResult, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opEntrust, "accountID must not be empty")
	}
	if err := validateOrderForHK(order, s.tickSchedule()); err != nil {
		return nil, err
	}

	exchangeType := types.ExchangeType("")
	switch order.Symbol.Market {
	case domain.MarketHK:
		exchangeType = types.ExchangeHK
	case domain.MarketUS:
		exchangeType = types.ExchangeUS
	}

	req := entrustWireRequest{
		ExchangeType:       exchangeType,
		StockCode:          order.Symbol.Code,
		EntrustAmount:      order.Quantity.String(),
		EntrustPrice:       order.Price.String(),
		EntrustBS:          order.Side,
		EntrustType:        order.OrderType,
		ClientType:         order.ClientType,
		SessionType:        order.SessionType,
		IceBergDisplaySize: order.IcebergQty.String(),
		ValidDays:          strconv.Itoa(order.ValidDays),
		CondValue:          order.CondValue,
		CondTrackType:      order.CondTrackType,
	}

	var out entrustWireResponse
	if err := s.client.Do(ctx, opEntrust, client.RouteTradeEntrust, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return &domain.OrderResult{
		EntrustID: domain.EntrustID(out.EntrustID),
		Status:    types.EntrustStatusWaitToRegister,
	}, nil
}

func (s *TradingService) CancelEntrust(ctx context.Context, accountID domain.AccountID, entrustID domain.EntrustID) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opCancelEntrust, "accountID must not be empty")
	}
	if entrustID.IsZero() {
		return errs.New(types.StatusInvalidParam, opCancelEntrust, "entrustID must not be empty")
	}

	req := cancelEntrustWireRequest{
		EntrustID:    entrustID.String(),
		ExchangeType: types.ExchangeHK,
	}

	var out commonStringResponse
	if err := s.client.Do(ctx, opCancelEntrust, client.RouteTradeCancelEntrust, req, s.client.JSON(), &out); err != nil {
		return err
	}
	return nil
}

func (s *TradingService) BatchCancelEntrust(ctx context.Context, accountID domain.AccountID, entrustIDs []domain.EntrustID) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opBatchCancelEntrust, "accountID must not be empty")
	}

	ids := make([]string, len(entrustIDs))
	for i, id := range entrustIDs {
		ids[i] = id.String()
	}

	req := batchCancelEntrustWireRequest{
		ExchangeType: types.ExchangeHK,
		EntrustIDs:   ids,
	}

	var out domain.CancelResultWire
	if err := s.client.Do(ctx, opBatchCancelEntrust, client.RouteTradeBatchCancelEntrust, req, s.client.JSON(), &out); err != nil {
		return err
	}
	return nil
}

func (s *TradingService) ChangeEntrust(ctx context.Context, accountID domain.AccountID, entrustID domain.EntrustID, newPrice domain.Price, newQty domain.Quantity) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opChangeEntrust, "accountID must not be empty")
	}
	if entrustID.IsZero() {
		return errs.New(types.StatusInvalidParam, opChangeEntrust, "entrustID must not be empty")
	}

	// The symbol below is a deliberate fiction, and the two defects it papers over
	// are recorded rather than fixed here. ChangeEntrust takes no domain.Symbol, so
	// the instrument's real DataType is unobtainable and a DataType-keyed schedule
	// cannot be resolved for it either. Substituting an HK stock means every amend
	// is validated against the HK-stock grid, whatever the order actually is.
	//
	// Fixing it needs a Symbol parameter, which is a public API change on the
	// v-next layer, and it also intersects the second defect: the wire request
	// below never sets StockCode even though docs/SPEC.md documents the field, so
	// the instrument is not sent at all. Whether the Gateway requires stockCode is
	// a question only a live request can answer, which is why it joins G6 rather
	// than being settled by inspection.
	//
	// See next-phase.md and
	// docs/runs/2026-09-26-vnext-parity-wire/todos.md for both findings.
	changeEntrustSymbol := domain.Symbol{
		Market:   domain.MarketHK,
		DataType: types.DataTypeHKStock,
	}
	if err := validatePriceForHK(newPrice, s.tickSchedule(), changeEntrustSymbol); err != nil {
		return err
	}
	if err := validateQuantityForHK(newQty, s.tickSchedule(), changeEntrustSymbol); err != nil {
		return err
	}

	req := changeEntrustWireRequest{
		ExchangeType:  types.ExchangeHK,
		EntrustID:     entrustID.String(),
		EntrustAmount: newQty.String(),
		EntrustPrice:  newPrice.String(),
	}

	var out commonStringResponse
	if err := s.client.Do(ctx, opChangeEntrust, client.RouteTradeChangeEntrust, req, s.client.JSON(), &out); err != nil {
		return err
	}
	return nil
}

func (s *TradingService) MaxAvailableAsset(ctx context.Context, accountID domain.AccountID, symbol domain.Symbol, price domain.Price, orderType types.EntrustType) (*domain.MaxAvailable, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opMaxAvailableAsset, "accountID must not be empty")
	}
	if symbol.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opMaxAvailableAsset, "symbol must not be empty")
	}

	exchangeType := types.ExchangeType("")
	switch symbol.Market {
	case domain.MarketHK:
		exchangeType = types.ExchangeHK
	case domain.MarketUS:
		exchangeType = types.ExchangeUS
	}

	req := maxAvailableAssetWireRequest{
		ExchangeType: exchangeType,
		StockCode:    symbol.Code,
		EntrustPrice: price.String(),
		EntrustType:  orderType,
	}

	var out maxAvailableAssetWireResponse
	if err := s.client.Do(ctx, opMaxAvailableAsset, client.RouteTradeQueryMaxAvailableAsset, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.MaxAvailableFromWire(&out.Data), nil
}

func (s *TradingService) RealEntrustList(ctx context.Context, accountID domain.AccountID, filter EntrustFilter) ([]*domain.Entrust, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opRealEntrustList, "accountID must not be empty")
	}

	ids := make([]string, len(filter.EntrustIDs))
	for i, id := range filter.EntrustIDs {
		ids[i] = id.String()
	}

	req := realEntrustListWireRequest{
		ExchangeType:  filter.ExchangeType,
		QueryCount:    defaultPageSize,
		QueryParamStr: "",
		EntrustIDs:    ids,
	}

	var out orderListWireResponse
	if err := s.client.Do(ctx, opRealEntrustList, client.RouteTradeQueryRealEntrustList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	entruts := make([]*domain.Entrust, len(out.Data))
	for i := range out.Data {
		entruts[i] = domain.EntrustFromWire(&out.Data[i], filter.ExchangeType)
	}
	return entruts, nil
}

func (s *TradingService) RealDeliverList(ctx context.Context, accountID domain.AccountID, filter DeliverFilter) ([]*domain.Fill, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opRealDeliverList, "accountID must not be empty")
	}

	req := realDeliverListWireRequest{
		ExchangeType:  filter.ExchangeType,
		QueryCount:    defaultPageSize,
		QueryParamStr: "",
	}

	var out orderListWireResponse
	if err := s.client.Do(ctx, opRealDeliverList, client.RouteTradeQueryRealDeliverList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	fills := make([]*domain.Fill, len(out.Data))
	for i := range out.Data {
		fills[i] = domain.FillFromWire(&out.Data[i], filter.ExchangeType)
	}
	return fills, nil
}

func (s *TradingService) RealCondOrderList(ctx context.Context, accountID domain.AccountID, filter CondOrderFilter) ([]*domain.CondOrder, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opRealCondOrderList, "accountID must not be empty")
	}

	pageNo := filter.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	req := condOrderListWireRequest{
		ExchangeType: filter.ExchangeType,
		StockCode:    filter.StockCode,
		PageNo:       pageNo,
		PageSize:     pageSize,
	}

	var out condOrderPageWireResponse
	if err := s.client.Do(ctx, opRealCondOrderList, client.RouteTradeQueryRealCondOrderList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	orders := make([]*domain.CondOrder, len(out.Data))
	for i := range out.Data {
		orders[i] = domain.CondOrderFromWire(&out.Data[i])
	}
	return orders, nil
}

func (s *TradingService) HistoryEntrustList(ctx context.Context, accountID domain.AccountID, filter HistoryFilter) ([]*domain.Entrust, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opHistoryEntrustList, "accountID must not be empty")
	}

	req := historyEntrustListWireRequest{
		ExchangeType:  filter.ExchangeType,
		QueryCount:    defaultPageSize,
		QueryParamStr: "",
		StartDate:     filter.StartDate,
		EndDate:       filter.EndDate,
	}

	var out orderListWireResponse
	if err := s.client.Do(ctx, opHistoryEntrustList, client.RouteTradeQueryHistoryEntrustList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	entruts := make([]*domain.Entrust, len(out.Data))
	for i := range out.Data {
		entruts[i] = domain.EntrustFromWire(&out.Data[i], filter.ExchangeType)
	}
	return entruts, nil
}

func (s *TradingService) HistoryDeliverList(ctx context.Context, accountID domain.AccountID, filter HistoryFilter) ([]*domain.Fill, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opHistoryDeliverList, "accountID must not be empty")
	}

	req := historyDeliverListWireRequest{
		ExchangeType:  filter.ExchangeType,
		QueryCount:    defaultPageSize,
		QueryParamStr: "",
		StartDate:     filter.StartDate,
		EndDate:       filter.EndDate,
	}

	var out orderListWireResponse
	if err := s.client.Do(ctx, opHistoryDeliverList, client.RouteTradeQueryHistoryDeliverList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	fills := make([]*domain.Fill, len(out.Data))
	for i := range out.Data {
		fills[i] = domain.FillFromWire(&out.Data[i], filter.ExchangeType)
	}
	return fills, nil
}

func (s *TradingService) HistoryCondOrderList(ctx context.Context, accountID domain.AccountID, filter HistoryFilter) ([]*domain.CondOrder, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opHistoryCondOrderList, "accountID must not be empty")
	}

	pageNo := filter.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	req := historyCondOrderListWireRequest{
		ExchangeType: filter.ExchangeType,
		StockCode:    filter.StockCode,
		PageNo:       strconv.Itoa(pageNo),
		PageSize:     strconv.Itoa(pageSize),
		StartTime:    filter.StartDate + " 00:00:00",
		EndTime:      filter.EndDate + " 23:59:59",
	}

	var out condOrderPageWireResponse
	if err := s.client.Do(ctx, opHistoryCondOrderList, client.RouteTradeQueryHistoryCondOrderList, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	orders := make([]*domain.CondOrder, len(out.Data))
	for i := range out.Data {
		orders[i] = domain.CondOrderFromWire(&out.Data[i])
	}
	return orders, nil
}

func (s *TradingService) MarginFullInfo(ctx context.Context, accountID domain.AccountID) (*domain.MarginInfo, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opMarginFullInfo, "accountID must not be empty")
	}

	req := marginFullInfoWireRequest{
		DataType:  "10000",
		StockCode: "",
	}

	var out domain.MarginFullInfoWire
	if err := s.client.Do(ctx, opMarginFullInfo, client.RouteTradeQueryMarginFullInfo, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.MarginInfoFromWire(&out), nil
}

func (s *TradingService) BeforeAndAfterSupport(ctx context.Context, symbol domain.Symbol) (*domain.SessionSupport, error) {
	if symbol.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opBeforeAndAfterSupport, "symbol must not be empty")
	}

	exchangeType := types.ExchangeType("")
	switch symbol.Market {
	case domain.MarketHK:
		exchangeType = types.ExchangeHK
	case domain.MarketUS:
		exchangeType = types.ExchangeUS
	}

	req := beforeAndAfterSupportWireRequest{
		StockCode:    symbol.Code,
		ExchangeType: exchangeType,
	}

	var out commonStringResponse
	if err := s.client.Do(ctx, opBeforeAndAfterSupport, client.RouteTradeQueryBeforeAndAfterSupport, req, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.SessionSupportFromWire(symbol, out.Data), nil
}

// SubscribeOrders subscribes the trading session to order-status push on the
// Gateway (POST /trade/TradeSubscribe) and reports whether the Gateway
// accepted it.
//
// The request params are empty and the reply is discarded, so a nil error means
// the subscription was accepted and there is nothing else to read out of it. The
// notifications themselves arrive on the TCP push channel as
// TradeStockDeliverNotify; receiving them is C10's PushOrchestration, built on
// internal/push, and is not this method's business. The Gateway requires the
// subscription to be asked for explicitly — the legacy direct protocol
// auto-subscribed on connection, and the Gateway does not.
//
// The params are empty because order push is one session-wide subscription
// rather than a per-security one, unlike the market push topics
// pkg/services/market.go subscribes to over /hq/Subscribe. There is no topic, no
// security list, and no filter, so there is nothing for a caller to get wrong and
// nothing for a future maintainer to add.
//
// # accountID is a session key and never crosses the wire
//
// accountID names the session the caller means and is validated as a local
// precondition — a zero one is refused with types.StatusInvalidParam under
// opTradeSubscribe before any request is spent — and it is then placed in no
// request body. That is the rule every method in this package follows, and
// stating it here is load bearing rather than ceremonial: an account identifier
// in this body would be an identifier the Gateway never asked for, because the
// Gateway resolves the account from the authenticated session. It would also be
// invisible on the mock, which answers by path and never inspects a body.
// TestTradePushAccountIDNeverReachesTheWire is the assertion.
//
// # A trade session is required, and only the Gateway can enforce it
//
// Both push routes are refused without an authenticated session, and nothing in
// pkg/services can check for one: this package reaches the Gateway through an
// Executor and nothing else, holds no session store, and the session lives in
// SessionService and in internal/auth behind it. The released layer can gate,
// because its Manager holds a *SessionManager and calls EnsureLoggedIn first;
// this layer deliberately has no such collaborator, and inventing a local
// pre-check would mean adding the dependency this file's package does not have
// in order to duplicate a check the Gateway already performs and reports with a
// documented code. So the request is sent, and the Gateway refuses it — with
// StatusNotLoggedIn or StatusKickedOffline, both of which errs.ReLoginRequired
// reports, so a caller knows to log in and try again. Gating the whole v-next
// surface on a session is C10's PushOrchestration decision, not this file's.
//
// # It is a query, and a retry policy will re-send it
//
// Neither /trade/TradeSubscribe nor /trade/TradeUnsubscribe is in
// internal/resilience's closed mutation set, so client.IsQuery classifies both as
// query class and a retry policy re-sends them on a retryable rejection. That is
// correct rather than a gap: ADR 0003 is about order mutations — a retry of a
// failed entrust can place an order the caller did not intend — and a
// subscription is not one. Re-subscribing a subscription that was never
// established and unsubscribing a subscription that is already gone are the same
// no-op twice, which is the idempotence a retry needs. The service adds no retry
// of its own, and neither path is added to the mutation set; doing that would be
// a behaviour change to the released layer, which sends both through the same
// client path and therefore classifies both the same way this one does.
//
// # The released layer's GoDoc disagrees with its own code here
//
// pkg/hstong/trade/push.go:31 says this call "issues exactly one HTTP request
// and is never retried", and push.go:39 repeats it for the unsubscribe. The code
// does not do that: the route is not in resilience.mutationPaths, so Manager.call
// hands it to the same client whose execute consults the retry class, and a
// policy re-sends it. The doc is stale rather than the code being wrong — the
// sentence describes the *default* configuration, where no policy is installed
// and every route is sent once, as an absolute. This method therefore follows
// the code, which is the thing that was measured:
// TestTradePushRequestsRetryAndOrderMutationsDoNot drives both routes and an
// order mutation through one MaxAttempts-5 policy and a retryable 1011 and
// records 5, 5, and 1. Correcting the released layer's GoDoc is out of scope
// here and is recorded in the run tracker instead.
func (s *TradingService) SubscribeOrders(ctx context.Context, accountID domain.AccountID) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opTradeSubscribe, "accountID must not be empty")
	}
	return s.client.Do(ctx, opTradeSubscribe, client.RouteTradeSubscribe, struct{}{}, s.client.JSON(), nil)
}

// UnsubscribeOrders cancels the session-wide order-status push subscription on
// the Gateway (POST /trade/TradeUnsubscribe) and reports whether the Gateway
// accepted it.
//
// It is SubscribeOrders' exact inverse and shares all of its properties, so each
// of the four is stated only once on SubscribeOrders and named here rather than
// repeated in a second place that could drift: struct{}{} params rather than nil,
// because internal/transport drops a nil params from the envelope entirely
// (request.Params carries omitempty) and the key would be absent rather than the
// empty object both vendor SDKs send; a discarded reply, so a nil error is the
// whole result; an accountID that is validated and then never placed in a wire
// struct; a trade session that only the Gateway can require; and query retry
// class, so a policy re-sends it — see TestTradePushRequestsRetryAndOrderMutationsDoNot,
// which measures 5 requests for this route against 1 for an order mutation under
// one policy.
//
// The one asymmetry is the failure's meaning. A rejected SubscribeOrders leaves
// the caller with no order push at all, so it is a loss of a facility it was
// trying to acquire. A rejected UnsubscribeOrders may mean the Gateway had
// already forgotten the subscription — a login that displaced it, a restart — in
// which case the caller has the outcome it asked for and will not hear about it
// again. Neither is distinguishable here, and neither is retried by this method,
// so the caller's next move is to reconcile against the push channel rather than
// to call again. That is ADR 0003's reconciliation requirement applied to a
// non-mutation: the answer is never guessed locally.
func (s *TradingService) UnsubscribeOrders(ctx context.Context, accountID domain.AccountID) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opTradeUnsubscribe, "accountID must not be empty")
	}
	return s.client.Do(ctx, opTradeUnsubscribe, client.RouteTradeUnsubscribe, struct{}{}, s.client.JSON(), nil)
}

// validateOrderForHK runs the Hong Kong-specific checks on order.
//
// It takes the resolver as a parameter rather than reading it from the service
// so the whole matrix can be driven against a fake without a TradingService, and
// so the nil-means-default rule is applied in exactly one place.
func validateOrderForHK(order domain.Order, schedule domain.TickScheduleResolver) error {
	if order.Symbol.Market != domain.MarketHK {
		return nil
	}

	if err := validateQuantityForHK(order.Quantity, schedule, order.Symbol); err != nil {
		return err
	}

	if !isPriceOptional(order.OrderType) {
		if err := validatePriceForHK(order.Price, schedule, order.Symbol); err != nil {
			return err
		}
	}

	if err := validateSessionWindow(order.Symbol.Market, order.SessionType); err != nil {
		return err
	}

	if err := validateOrderTypeEligibility(order.OrderType, order.Symbol.Market); err != nil {
		return err
	}

	if err := validateTimeInForce(order.TimeInForce); err != nil {
		return err
	}

	if err := validateShortSellingFlag(order.Side, order.Symbol.DataType); err != nil {
		return err
	}

	if isConditionalOrder(order.OrderType) {
		if order.ValidDays < 1 || order.ValidDays > maxValidDays {
			return errs.New(types.StatusInvalidParam, opEntrust,
				fmt.Sprintf("validDays must be between 1 and %d for conditional orders", maxValidDays))
		}
		if order.CondValue == "" {
			return errs.New(types.StatusInvalidParam, opEntrust, "condValue is required for conditional orders")
		}
	}

	return nil
}

// validateQuantityForHK checks qty against the lot size the resolver reports.
//
// ok false means the resolver has no opinion, and the check is skipped: an
// absent lot size is unknown, not zero, and a zero lot would make ValidateLot's
// remainder test pass for every quantity.
func validateQuantityForHK(qty domain.Quantity, schedule domain.TickScheduleResolver, sym domain.Symbol) error {
	lot, _, ok := schedule.ScheduleFor(sym)
	if !ok {
		return nil
	}
	if err := qty.ValidateInteger(); err != nil {
		return errs.New(types.StatusInvalidParam, opEntrust, err.Error())
	}
	if err := qty.ValidateLot(lot); err != nil {
		return errs.New(types.StatusInvalidParam, opEntrust, err.Error())
	}
	return nil
}

// validatePriceForHK checks price against the instrument's tick, not the one the
// caller attached to it.
//
// The previous form asked the schedule only whether a tick existed and then
// called price.Validate(true), which checks the price against its own tick — a
// number the caller chose. The check could therefore not fail for a reason the
// caller had not already ruled out, which made it a gate rather than a
// validation. ValidateOn takes the instrument's grid, so the resolver's answer
// is what decides.
//
// ok false skips the step check. The design's "reject an order whose tick is
// unknown" is deliberately not implemented here: the default table still answers
// 0.001 for an HK stock, so an unknown tick today means "the price-band table has
// not been sourced yet" rather than "this instrument has no grid", and rejecting
// on that basis would break orders the exchange accepts. The rejection lands
// with the band table. See design-tick-model.md §2.3 and the deferred
// price-band work in next-phase.md.
func validatePriceForHK(price domain.Price, schedule domain.TickScheduleResolver, sym domain.Symbol) error {
	_, tick, ok := schedule.ScheduleFor(sym)
	if !ok {
		return nil
	}
	if err := price.ValidateOn(tick); err != nil {
		return errs.New(types.StatusInvalidParam, opEntrust, err.Error())
	}
	return nil
}

func validateSessionWindow(market domain.Market, sessionType string) error {
	if market != domain.MarketHK {
		return nil
	}
	if sessionType == "" {
		return nil
	}
	window, ok := hkSessionWindows[string(market)]
	if !ok {
		return nil
	}

	now := time.Now().In(hkZone)

	currentTime := now.Format("15:04")

	switch sessionType {
	case "0", "":
		return nil
	case "1":
		if currentTime < window.preMarketStart || currentTime > window.preMarketEnd {
			return errs.New(types.StatusInvalidParam, opEntrust,
				"order time is outside pre-market session window (09:00-09:30)")
		}
	case "2":
		if currentTime < window.regularStart || currentTime > window.regularEnd {
			return errs.New(types.StatusInvalidParam, opEntrust,
				"order time is outside regular trading session window (09:30-12:00, 12:00-16:00)")
		}
	case "3":
		if currentTime < window.afterHoursStart || currentTime > window.afterHoursEnd {
			return errs.New(types.StatusInvalidParam, opEntrust,
				"order time is outside after-hours session window (12:00-16:00)")
		}
	}
	return nil
}

func validateOrderTypeEligibility(orderType types.EntrustType, market domain.Market) error {
	if market != domain.MarketHK {
		return nil
	}
	if _, ok := knownHKOrderTypes[orderType]; !ok {
		return errs.New(types.StatusInvalidParam, opEntrust,
			fmt.Sprintf("order type %s is not eligible for HK market", orderType))
	}
	return nil
}

func validateTimeInForce(tif string) error {
	validTIFs := map[string]struct{}{
		"":    {},
		"DAY": {},
		"IOC": {},
		"FOK": {},
		"GTD": {},
	}
	if _, ok := validTIFs[tif]; !ok {
		return errs.New(types.StatusInvalidParam, opEntrust,
			fmt.Sprintf("time-in-force %s is not valid", tif))
	}
	return nil
}

func validateShortSellingFlag(side types.EntrustBS, dtype types.DataType) error {
	if dtype != types.DataTypeHKStock && dtype != types.DataTypeHKETF {
		return nil
	}
	switch side {
	case types.EntrustBuy, types.EntrustSell, types.EntrustOpenShort, types.EntrustCloseShort:
		return nil
	default:
		return errs.New(types.StatusInvalidParam, opEntrust,
			fmt.Sprintf("side %s is not valid for HK stocks/ETFs", side))
	}
}

func isPriceOptional(t types.EntrustType) bool {
	switch t {
	case types.EntrustTypeMarket,
		types.EntrustTypeIcebergMarket,
		types.EntrustTypeHiddenMarket,
		types.EntrustTypeStopProfitLimit,
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

func isConditionalOrder(t types.EntrustType) bool {
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

type commonStringResponse struct {
	Data string `json:"data"`
}

type entrustWireRequest struct {
	ExchangeType       types.ExchangeType `json:"exchangeType"`
	StockCode          string             `json:"stockCode"`
	EntrustAmount      string             `json:"entrustAmount"`
	EntrustPrice       string             `json:"entrustPrice,omitempty"`
	EntrustBS          types.EntrustBS    `json:"entrustBs"`
	EntrustType        types.EntrustType  `json:"entrustType"`
	ClientType         int                `json:"clientType,omitempty"`
	Exchange           string             `json:"exchange,omitempty"`
	SessionType        string             `json:"sessionType,omitempty"`
	IceBergDisplaySize string             `json:"iceBergDisplaySize,omitempty"`
	ValidDays          string             `json:"validDays,omitempty"`
	CondValue          string             `json:"condValue,omitempty"`
	CondTrackType      string             `json:"condTrackType,omitempty"`
}

type entrustWireResponse struct {
	EntrustID string `json:"entrustId"`
}

type cancelEntrustWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	StockCode     string             `json:"stockCode"`
	EntrustAmount string             `json:"entrustAmount,omitempty"`
	EntrustPrice  string             `json:"entrustPrice,omitempty"`
	EntrustID     string             `json:"entrustId"`
	EntrustType   types.EntrustType  `json:"entrustType,omitempty"`
}

type batchCancelEntrustWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType"`
	EntrustIDs   []string           `json:"entrustId,omitempty"`
}

type changeEntrustWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	StockCode     string             `json:"stockCode"`
	EntrustAmount string             `json:"entrustAmount"`
	EntrustPrice  string             `json:"entrustPrice,omitempty"`
	EntrustID     string             `json:"entrustId"`
	EntrustType   types.EntrustType  `json:"entrustType,omitempty"`
	SessionType   string             `json:"sessionType,omitempty"`
	ValidDays     string             `json:"validDays,omitempty"`
	CondValue     string             `json:"condValue,omitempty"`
	CondTrackType string             `json:"condTrackType,omitempty"`
}

type maxAvailableAssetWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType"`
	StockCode    string             `json:"stockCode"`
	EntrustPrice string             `json:"entrustPrice"`
	EntrustType  types.EntrustType  `json:"entrustType"`
}

type maxAvailableAssetWireResponse struct {
	Data domain.MaxAvailableWire `json:"data"`
}

type realEntrustListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
	EntrustIDs    []string           `json:"entrustId,omitempty"`
}

type realDeliverListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
}

type orderListWireResponse struct {
	Data []domain.EntrustWire `json:"data"`
}

type condOrderListWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType"`
	StockCode    string             `json:"stockCode,omitempty"`
	PageNo       int                `json:"pageNo"`
	PageSize     int                `json:"pageSize"`
}

type condOrderPageWireResponse struct {
	Data        []domain.CondOrderWire `json:"data"`
	CurPageNo   int32                  `json:"curPageNo"`
	CurPageSize int32                  `json:"curPageSize"`
	TotalPages  int64                  `json:"totalPages"`
}

type historyEntrustListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
	StartDate     string             `json:"startDate"`
	EndDate       string             `json:"endDate"`
}

type historyDeliverListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
	StartDate     string             `json:"startDate"`
	EndDate       string             `json:"endDate"`
}

type historyCondOrderListWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType"`
	StockCode    string             `json:"stockCode,omitempty"`
	PageNo       string             `json:"pageNo"`
	PageSize     string             `json:"pageSize"`
	StartTime    string             `json:"startTime"`
	EndTime      string             `json:"endTime"`
}

type marginFullInfoWireRequest struct {
	DataType  string `json:"dataType"`
	StockCode string `json:"stockCode"`
}

type beforeAndAfterSupportWireRequest struct {
	StockCode    string             `json:"stockCode"`
	ExchangeType types.ExchangeType `json:"exchangeType"`
}
