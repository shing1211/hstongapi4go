// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

const (
	opBasicQot                = "market.BasicQot"
	opOrderBook               = "market.OrderBook"
	opKL                      = "market.KL"
	opTimeShare               = "market.TimeShare"
	opTicker                  = "market.Ticker"
	opBroker                  = "market.Broker"
	opUsOptionChainCode       = "market.UsOptionChainCode"
	opUsOptionChainExpireDate = "market.UsOptionChainExpireDate"
	opUsOverNightTradeCodes   = "market.UsOverNightTradeCodes"
	opSubscribe               = "market.Subscribe"
	opUnsubscribe             = "market.Unsubscribe"
)

const MaxTickerLimit = 100

type MarketService struct {
	client Executor
}

type MarketOption func(*MarketService)

func WithMarketClient(c Executor) MarketOption {
	return func(s *MarketService) { s.client = c }
}

func NewMarketService(c Executor, opts ...MarketOption) *MarketService {
	s := &MarketService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type Security struct {
	DataType types.DataType
	Code     string
}

type basicQotRequest struct {
	Security      []*dto.Security `json:"security"`
	NeedDelayFlag string          `json:"needDelayFlag,omitempty"`
	MktTmType     int32           `json:"mktTmType"`
}

type BasicQotRequest struct {
	Security      []*Security `json:"security"`
	NeedDelayFlag string      `json:"needDelayFlag,omitempty"`
	MktTmType     int32       `json:"mktTmType"`
}

type BasicQotResponse struct {
	BasicQot []*domain.Quote `json:"basicQot"`
}

type basicQotWireResponse struct {
	BasicQot []*dto.BasicQot `json:"basicQot"`
}

func (s *MarketService) BasicQot(ctx context.Context, req BasicQotRequest) (BasicQotResponse, error) {
	if err := validateSecurityList(opBasicQot, req.Security); err != nil {
		return BasicQotResponse{}, err
	}
	dtoSecs := make([]*dto.Security, len(req.Security))
	for i, sec := range req.Security {
		dtoSecs[i] = &dto.Security{DataType: int32(sec.DataType), Code: sec.Code}
	}
	var wireResp basicQotWireResponse
	if err := s.client.Do(ctx, opBasicQot, client.RouteHqBasicQot, basicQotRequest{
		Security:      dtoSecs,
		NeedDelayFlag: req.NeedDelayFlag,
		MktTmType:     req.MktTmType,
	}, s.client.JSON(), &wireResp); err != nil {
		return BasicQotResponse{}, err
	}
	out := BasicQotResponse{BasicQot: make([]*domain.Quote, len(wireResp.BasicQot))}
	for i, q := range wireResp.BasicQot {
		out.BasicQot[i] = convertToQuote(q)
	}
	return out, nil
}

type orderBookRequest struct {
	Security      *dto.Security `json:"security"`
	MktTmType     int32         `json:"mktTmType"`
	DepthBookType int32         `json:"depthBookType,omitempty"`
}

type OrderBookRequest struct {
	Security      *Security `json:"security"`
	MktTmType     int32     `json:"mktTmType"`
	DepthBookType int32     `json:"depthBookType,omitempty"`
}

type OrderBookResponse struct {
	Security *Security               `json:"security"`
	Ask      []domain.OrderBookLevel `json:"orderBookAskList"`
	Bid      []domain.OrderBookLevel `json:"orderBookBidList"`
	TickSize domain.Price            `json:"spreadLevel"`
}

type orderBookWireResponse struct {
	Security         *dto.Security    `json:"security"`
	OrderBookAskList []*dto.OrderBook `json:"orderBookAskList"`
	OrderBookBidList []*dto.OrderBook `json:"orderBookBidList"`
	// TickSize is json.Number, not float64 and not plain string. The Gateway
	// sends the HQ market numerics as unquoted JSON numbers, which encoding/json
	// refuses to decode into a string field ("cannot unmarshal number into Go
	// value of type string") and only preserves verbatim for json.Number; it also
	// accepts the quoted form. Either way the digits the Gateway sent reach
	// domain.MustNewPrice unchanged, so a tick of 0.0005 stays 0.0005.
	TickSize json.Number `json:"spreadLevel"`
}

func (s *MarketService) OrderBook(ctx context.Context, req OrderBookRequest) (OrderBookResponse, error) {
	if err := validateSecurity(opOrderBook, req.Security); err != nil {
		return OrderBookResponse{}, err
	}
	var wireResp orderBookWireResponse
	if err := s.client.Do(ctx, opOrderBook, client.RouteHqOrderBook, orderBookRequest{
		Security:      &dto.Security{DataType: int32(req.Security.DataType), Code: req.Security.Code},
		MktTmType:     req.MktTmType,
		DepthBookType: req.DepthBookType,
	}, s.client.JSON(), &wireResp); err != nil {
		return OrderBookResponse{}, err
	}
	return OrderBookResponse{
		Security: &Security{DataType: types.DataType(wireResp.Security.DataType), Code: wireResp.Security.Code},
		Ask:      convertOrderBookLevels(wireResp.OrderBookAskList),
		Bid:      convertOrderBookLevels(wireResp.OrderBookBidList),
		TickSize: domain.MustNewPrice(decimalOrZero(string(wireResp.TickSize)), "0.001"),
	}, nil
}

type klRequest struct {
	Security    *dto.Security `json:"security"`
	StartDate   int64         `json:"startDate"`
	Direction   int32         `json:"direction"`
	ExRightFlag int32         `json:"exRightFlag"`
	CycType     int32         `json:"cycType"`
	Limit       int32         `json:"limit"`
}

type KLRequest struct {
	Security    *Security `json:"security"`
	StartDate   int64     `json:"startDate"`
	Direction   int32     `json:"direction"`
	ExRightFlag int32     `json:"exRightFlag"`
	CycType     int32     `json:"cycType"`
	Limit       int32     `json:"limit"`
}

type KLResponse struct {
	Security *Security       `json:"security"`
	Kline    []*domain.KLine `json:"kline"`
}

type klineWireResponse struct {
	Security *dto.Security `json:"security"`
	Kline    []*dto.KLine  `json:"kline"`
}

func (s *MarketService) KL(ctx context.Context, req KLRequest) (KLResponse, error) {
	if err := validateSecurity(opKL, req.Security); err != nil {
		return KLResponse{}, err
	}
	var wireResp klineWireResponse
	if err := s.client.Do(ctx, opKL, client.RouteHqKL, klRequest{
		Security:    &dto.Security{DataType: int32(req.Security.DataType), Code: req.Security.Code},
		StartDate:   req.StartDate,
		Direction:   req.Direction,
		ExRightFlag: req.ExRightFlag,
		CycType:     req.CycType,
		Limit:       req.Limit,
	}, s.client.JSON(), &wireResp); err != nil {
		return KLResponse{}, err
	}
	out := KLResponse{Security: &Security{DataType: types.DataType(wireResp.Security.DataType), Code: wireResp.Security.Code}}
	out.Kline = make([]*domain.KLine, len(wireResp.Kline))
	for i, k := range wireResp.Kline {
		out.Kline[i] = convertToKLine(k)
	}
	return out, nil
}

type timeShareRequest struct {
	Security  *dto.Security `json:"security"`
	MktTmType int32         `json:"mktTmType"`
}

type TimeShareRequest struct {
	Security  *Security `json:"security"`
	MktTmType int32     `json:"mktTmType"`
}

type TimeShareResponse struct {
	Security  *Security                `json:"security"`
	TimeShare []*domain.TimeSharePoint `json:"timeShare"`
}

type timeShareWireResponse struct {
	Security  *dto.Security    `json:"security"`
	TimeShare []*dto.TimeShare `json:"timeShare"`
}

func (s *MarketService) TimeShare(ctx context.Context, req TimeShareRequest) (TimeShareResponse, error) {
	if err := validateSecurity(opTimeShare, req.Security); err != nil {
		return TimeShareResponse{}, err
	}
	var wireResp timeShareWireResponse
	if err := s.client.Do(ctx, opTimeShare, client.RouteHqTimeShare, timeShareRequest{
		Security:  &dto.Security{DataType: int32(req.Security.DataType), Code: req.Security.Code},
		MktTmType: req.MktTmType,
	}, s.client.JSON(), &wireResp); err != nil {
		return TimeShareResponse{}, err
	}
	out := TimeShareResponse{Security: &Security{DataType: types.DataType(wireResp.Security.DataType), Code: wireResp.Security.Code}}
	out.TimeShare = make([]*domain.TimeSharePoint, len(wireResp.TimeShare))
	for i, ts := range wireResp.TimeShare {
		out.TimeShare[i] = convertToTimeSharePoint(ts)
	}
	return out, nil
}

type tickerRequest struct {
	Security  *dto.Security `json:"security"`
	Limit     int32         `json:"limit"`
	MktTmType int32         `json:"mktTmType"`
}

type TickerRequest struct {
	Security  *Security `json:"security"`
	Limit     int32     `json:"limit"`
	MktTmType int32     `json:"mktTmType"`
}

type TickerResponse struct {
	Security *Security            `json:"security"`
	Ticker   []*domain.TickerTick `json:"ticker"`
}

type tickerWireResponse struct {
	Security *dto.Security `json:"security"`
	Ticker   []*dto.Ticker `json:"ticker"`
}

func (s *MarketService) Ticker(ctx context.Context, req TickerRequest) (TickerResponse, error) {
	if err := validateSecurity(opTicker, req.Security); err != nil {
		return TickerResponse{}, err
	}
	if req.Limit < 1 || req.Limit > MaxTickerLimit {
		return TickerResponse{}, errs.New(types.StatusInvalidParam, opTicker,
			fmt.Sprintf("ticker limit %d out of range 1..%d", req.Limit, MaxTickerLimit))
	}
	var wireResp tickerWireResponse
	if err := s.client.Do(ctx, opTicker, client.RouteHqTicker, tickerRequest{
		Security:  &dto.Security{DataType: int32(req.Security.DataType), Code: req.Security.Code},
		Limit:     req.Limit,
		MktTmType: req.MktTmType,
	}, s.client.JSON(), &wireResp); err != nil {
		return TickerResponse{}, err
	}
	out := TickerResponse{Security: &Security{DataType: types.DataType(wireResp.Security.DataType), Code: wireResp.Security.Code}}
	out.Ticker = make([]*domain.TickerTick, len(wireResp.Ticker))
	for i, t := range wireResp.Ticker {
		out.Ticker[i] = convertToTickerTick(t)
	}
	return out, nil
}

type brokerRequest struct {
	Security *dto.Security `json:"security"`
}

type BrokerRequest struct {
	Security *Security `json:"security"`
}

type BrokerResponse struct {
	Security *Security                 `json:"security"`
	Ask      []domain.BrokerQueueEntry `json:"brokerAskList"`
	Bid      []domain.BrokerQueueEntry `json:"brokerBidList"`
}

type brokerWireResponse struct {
	Security      *dto.Security `json:"security"`
	BrokerAskList []*dto.Broker `json:"brokerAskList"`
	BrokerBidList []*dto.Broker `json:"brokerBidList"`
}

func (s *MarketService) Broker(ctx context.Context, req BrokerRequest) (BrokerResponse, error) {
	if err := validateSecurity(opBroker, req.Security); err != nil {
		return BrokerResponse{}, err
	}
	var wireResp brokerWireResponse
	if err := s.client.Do(ctx, opBroker, client.RouteHqBroker, brokerRequest{
		Security: &dto.Security{DataType: int32(req.Security.DataType), Code: req.Security.Code},
	}, s.client.JSON(), &wireResp); err != nil {
		return BrokerResponse{}, err
	}
	return BrokerResponse{
		Security: &Security{DataType: types.DataType(wireResp.Security.DataType), Code: wireResp.Security.Code},
		Ask:      convertBrokerEntries(wireResp.BrokerAskList),
		Bid:      convertBrokerEntries(wireResp.BrokerBidList),
	}, nil
}

type usOptionChainCodeRequest struct {
	SecurityCode string `json:"securityCode"`
	ExpireDate   string `json:"expireDate"`
	FlagInOut    int32  `json:"flagInOut,omitempty"`
	OptionType   string `json:"optionType,omitempty"`
}

type UsOptionChainCodeRequest struct {
	SecurityCode string `json:"securityCode"`
	ExpireDate   string `json:"expireDate"`
	FlagInOut    int32  `json:"flagInOut,omitempty"`
	OptionType   string `json:"optionType,omitempty"`
}

type UsOptionChainCodeResponse struct {
	OptionCode []string `json:"optionCode"`
}

func (s *MarketService) UsOptionChainCode(ctx context.Context, req UsOptionChainCodeRequest) (UsOptionChainCodeResponse, error) {
	if err := validateSecurityCode(opUsOptionChainCode, req.SecurityCode); err != nil {
		return UsOptionChainCodeResponse{}, err
	}
	var resp struct {
		OptionCode []string `json:"optionCode"`
	}
	if err := s.client.Do(ctx, opUsOptionChainCode, client.RouteHqUsOptionChainCode, usOptionChainCodeRequest(req), s.client.JSON(), &resp); err != nil {
		return UsOptionChainCodeResponse{}, err
	}
	return UsOptionChainCodeResponse{OptionCode: resp.OptionCode}, nil
}

type usOptionChainExpireDateRequest struct {
	SecurityCode string `json:"securityCode"`
}

type UsOptionChainExpireDateRequest struct {
	SecurityCode string `json:"securityCode"`
}

type UsOptionChainExpireDateResponse struct {
	ExpireDate []string `json:"expireDate"`
}

func (s *MarketService) UsOptionChainExpireDate(ctx context.Context, req UsOptionChainExpireDateRequest) (UsOptionChainExpireDateResponse, error) {
	if err := validateSecurityCode(opUsOptionChainExpireDate, req.SecurityCode); err != nil {
		return UsOptionChainExpireDateResponse{}, err
	}
	var resp struct {
		ExpireDate []string `json:"expireDate"`
	}
	if err := s.client.Do(ctx, opUsOptionChainExpireDate, client.RouteHqUsOptionChainExpireDate, usOptionChainExpireDateRequest(req), s.client.JSON(), &resp); err != nil {
		return UsOptionChainExpireDateResponse{}, err
	}
	return UsOptionChainExpireDateResponse{ExpireDate: resp.ExpireDate}, nil
}

type UsOverNightTradeCodesRequest struct{}

type UsOverNightTradeCodesResponse struct {
	SecurityCodes []string `json:"securityCodes"`
}

func (s *MarketService) UsOverNightTradeCodes(ctx context.Context, req UsOverNightTradeCodesRequest) (UsOverNightTradeCodesResponse, error) {
	var resp struct {
		SecurityCodes []string `json:"securityCodes"`
	}
	if err := s.client.Do(ctx, opUsOverNightTradeCodes, client.RouteHqUsOverNightTradeCodes, req, s.client.JSON(), &resp); err != nil {
		return UsOverNightTradeCodesResponse{}, err
	}
	return UsOverNightTradeCodesResponse{SecurityCodes: resp.SecurityCodes}, nil
}

type subscribeRequest struct {
	TopicID  types.TopicID   `json:"topicId"`
	Security []*dto.Security `json:"security"`
}

type SubscribeRequest struct {
	TopicID  types.TopicID `json:"topicId"`
	Security []*Security   `json:"security"`
}

func (s *MarketService) Subscribe(ctx context.Context, topicID types.TopicID, securities ...*Security) error {
	if err := validateTopic(opSubscribe, topicID); err != nil {
		return err
	}
	if err := validateSecurityList(opSubscribe, securities); err != nil {
		return err
	}
	dtoSecs := make([]*dto.Security, len(securities))
	for i, sec := range securities {
		dtoSecs[i] = &dto.Security{DataType: int32(sec.DataType), Code: sec.Code}
	}
	return s.client.Do(ctx, opSubscribe, client.RouteHqSubscribe, subscribeRequest{
		TopicID:  topicID,
		Security: dtoSecs,
	}, s.client.JSON(), nil)
}

func (s *MarketService) Unsubscribe(ctx context.Context, topicID types.TopicID, securities ...*Security) error {
	if err := validateTopic(opUnsubscribe, topicID); err != nil {
		return err
	}
	if err := validateSecurityList(opUnsubscribe, securities); err != nil {
		return err
	}
	dtoSecs := make([]*dto.Security, len(securities))
	for i, sec := range securities {
		dtoSecs[i] = &dto.Security{DataType: int32(sec.DataType), Code: sec.Code}
	}
	return s.client.Do(ctx, opUnsubscribe, client.RouteHqUnsubscribe, subscribeRequest{
		TopicID:  topicID,
		Security: dtoSecs,
	}, s.client.JSON(), nil)
}

// floatToString renders a generated DTO float as the shortest decimal string that
// parses back to the same float64, so the value reaches the domain decimal
// constructors without being rounded on the way.
//
// It replaces the fmt.Sprintf("%.Nf", x) conversions this file used to make. Those
// hardcoded a scale the Gateway never promised: %.3f turned a 0.0005 tick into
// 0.001, a wrong price, and %.4f truncated the rates the same way. 'f' with
// precision -1 also keeps the exponent out ("0.0000001", never "1e-07"), which
// matters because a bare "1e-07" panics in domain.MustNewPrice.
//
// This is deliberately a package-local copy of the twin in pkg/transport
// (mappers.go, floatToString). Exporting that one, or introducing a third package
// to hold the shared copy, would each be a public API addition and so a new
// architecture decision under AGENTS.md rule 8; three lines of stdlib duplicated
// across two internal call sites is the cheaper trade. The two must stay in step:
// if either changes, change both.
func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// decimalOrZero normalises a Gateway string field for the decimal-backed domain
// constructors, which panic on an unparseable value and would otherwise take down
// the caller's goroutine when the Gateway omits the field. A JSON null or an absent
// key leaves the string empty, which is the only case handled here: a malformed
// price still fails at decode rather than being silently replaced by a number.
//
// The twin is pkg/transport (mappers.go, decimalOrZero); see floatToString above
// for why it is copied rather than shared.
func decimalOrZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func convertToQuote(q *dto.BasicQot) *domain.Quote {
	if q == nil {
		return nil
	}
	return &domain.Quote{
		IsSuspended:    q.IsSuspended,
		OpenPrice:      domain.MustNewPrice(floatToString(q.OpenPrice), "0.001"),
		HighPrice:      domain.MustNewPrice(floatToString(q.HighPrice), "0.001"),
		LowPrice:       domain.MustNewPrice(floatToString(q.LowPrice), "0.001"),
		LastPrice:      domain.MustNewPrice(floatToString(q.LastPrice), "0.001"),
		LastClosePrice: domain.MustNewPrice(floatToString(q.LastClosePrice), "0.001"),
		PriceSpread:    domain.MustNewPrice(floatToString(q.PriceSpread), "0.001"),
		Volume:         domain.MustNewQuantity(fmt.Sprintf("%d", q.Volume)),
		Turnover:       domain.MustNewMoney(floatToString(q.Turnover), "HKD", 3),
		TurnoverRate:   domain.MustNewRate(floatToString(q.TurnoverRate)),
		Amplitude:      domain.MustNewRate(floatToString(q.Amplitude)),
		SecStatus:      q.SecStatus,
		ListTime:       q.ListTime,
		LotSize:        q.LotSize,
		TradeTime:      q.TradeTime,
		MarketTime:     q.MarketTime,
		StockStatus:    q.StockStatus,
		VolumeStr:      q.VolumeStr,
		MktTmType:      q.MktTmType,
	}
}

func convertOrderBookLevels(levels []*dto.OrderBook) []domain.OrderBookLevel {
	out := make([]domain.OrderBookLevel, len(levels))
	for i, l := range levels {
		out[i] = domain.OrderBookLevel{
			Level:    l.Level,
			Price:    domain.MustNewPrice(floatToString(l.Price), "0.001"),
			Quantity: domain.MustNewQuantity(fmt.Sprintf("%d", l.Volume)),
		}
	}
	return out
}

func convertToKLine(k *dto.KLine) *domain.KLine {
	if k == nil {
		return nil
	}
	return &domain.KLine{
		Date:           k.Date,
		HighPrice:      domain.MustNewPrice(floatToString(k.HighPrice), "0.001"),
		OpenPrice:      domain.MustNewPrice(floatToString(k.OpenPrice), "0.001"),
		LowPrice:       domain.MustNewPrice(floatToString(k.LowPrice), "0.001"),
		ClosePrice:     domain.MustNewPrice(floatToString(k.ClosePrice), "0.001"),
		LastClosePrice: domain.MustNewPrice(floatToString(k.LastClosePrice), "0.001"),
		Volume:         domain.MustNewQuantity(fmt.Sprintf("%d", k.Volume)),
		Turnover:       domain.MustNewMoney(floatToString(k.Turnover), "HKD", 3),
		Timestamp:      k.Timestamp,
		Time:           k.Time,
	}
}

func convertToTimeSharePoint(ts *dto.TimeShare) *domain.TimeSharePoint {
	if ts == nil {
		return nil
	}
	return &domain.TimeSharePoint{
		Time:           ts.Time,
		Price:          domain.MustNewPrice(floatToString(ts.Price), "0.001"),
		LastClosePrice: domain.MustNewPrice(floatToString(ts.LastClosePrice), "0.001"),
		AvgPrice:       domain.MustNewPrice(floatToString(ts.AvgPrice), "0.001"),
		Volume:         domain.MustNewQuantity(fmt.Sprintf("%d", ts.Volume)),
		Turnover:       domain.MustNewMoney(floatToString(ts.Turnover), "HKD", 3),
	}
}

func convertToTickerTick(t *dto.Ticker) *domain.TickerTick {
	if t == nil {
		return nil
	}
	return &domain.TickerTick{
		Time:      t.Time,
		Side:      t.Side,
		Price:     domain.MustNewPrice(floatToString(t.Price), "0.001"),
		Volume:    domain.MustNewQuantity(fmt.Sprintf("%d", t.Volume)),
		Turnover:  domain.MustNewMoney(floatToString(t.Turnover), "HKD", 3),
		Type:      t.Type,
		Timestamp: t.Timestamp,
		MktTmType: t.MktTmType,
	}
}

func convertBrokerEntries(entries []*dto.Broker) []domain.BrokerQueueEntry {
	out := make([]domain.BrokerQueueEntry, len(entries))
	for i, e := range entries {
		out[i] = domain.BrokerQueueEntry{
			Level: e.Level,
			Item:  e.Item,
			Type:  e.Type,
			Name:  e.Name,
		}
	}
	return out
}

func validateSecurityList(op string, secs []*Security) error {
	if len(secs) == 0 {
		return errs.New(types.StatusInvalidParam, op, "security list must not be empty")
	}
	for i, s := range secs {
		if s == nil {
			return errs.New(types.StatusInvalidParam, op, fmt.Sprintf("security[%d] must not be nil", i))
		}
	}
	return nil
}

func validateSecurity(op string, s *Security) error {
	if s == nil {
		return errs.New(types.StatusInvalidParam, op, "security must not be nil")
	}
	return nil
}

func validateSecurityCode(op, code string) error {
	if code == "" {
		return errs.New(types.StatusInvalidParam, op, "securityCode must not be empty")
	}
	return nil
}

var knownTopics = map[types.TopicID]struct{}{
	types.TopicBasicQot:           {},
	types.TopicQuoteVariant35:     {},
	types.TopicTicker:             {},
	types.TopicTickVariant27:      {},
	types.TopicTickVariant28:      {},
	types.TopicTickVariant37:      {},
	types.TopicBroker:             {},
	types.TopicOrderBook:          {},
	types.TopicOrderBookArcabook:  {},
	types.TopicOrderBookTotalView: {},
	types.TopicOrderBookVariant36: {},
}

func validateTopic(op string, topic types.TopicID) error {
	if _, ok := knownTopics[topic]; !ok {
		return errs.New(types.StatusInvalidParam, op, fmt.Sprintf("unknown topicId %d", int(topic)))
	}
	return nil
}
