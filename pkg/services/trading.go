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

type TradingService struct {
	client Executor
}

type TradingOption func(*TradingService)

func WithTradingClient(c Executor) TradingOption {
	return func(s *TradingService) { s.client = c }
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
	if err := validateOrderForHK(order); err != nil {
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

	if err := validatePriceForHK(newPrice, types.DataTypeHKStock); err != nil {
		return err
	}
	if err := validateQuantityForHK(newQty, types.DataTypeHKStock); err != nil {
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

func validateOrderForHK(order domain.Order) error {
	if order.Symbol.Market != domain.MarketHK {
		return nil
	}

	if err := validateQuantityForHK(order.Quantity, order.Symbol.DataType); err != nil {
		return err
	}

	if !isPriceOptional(order.OrderType) {
		if err := validatePriceForHK(order.Price, order.Symbol.DataType); err != nil {
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

func validateQuantityForHK(qty domain.Quantity, dtype types.DataType) error {
	schedule := domain.DefaultHKTickSchedule(dtype)
	if schedule.Lot == 0 {
		return nil
	}
	if err := qty.ValidateInteger(); err != nil {
		return errs.New(types.StatusInvalidParam, opEntrust, err.Error())
	}
	if err := qty.ValidateLot(schedule.Lot); err != nil {
		return errs.New(types.StatusInvalidParam, opEntrust, err.Error())
	}
	return nil
}

func validatePriceForHK(price domain.Price, dtype types.DataType) error {
	schedule := domain.DefaultHKTickSchedule(dtype)
	if schedule.TickSize == "" {
		return nil
	}
	if err := price.Validate(true); err != nil {
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

	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Hong_Kong")
	now = now.In(loc)

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
