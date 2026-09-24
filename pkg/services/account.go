// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/transport"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

const (
	opMarginFundInfo      = "trade/TradeQueryMarginFundInfo"
	opHoldsList           = "trade/TradeQueryHoldsList"
	opRealFundJourList    = "trade/TradeQueryRealFundJourList"
	opHistoryFundJourList = "trade/TradeQueryHistoryFundJourList"
	opRateQueryList       = "hs/rate/queryList"
)

const DefaultPageSize = 50

type AccountService struct {
	client *client.Client
}

type AccountOption func(*AccountService)

func WithAccountClient(c *client.Client) AccountOption {
	return func(s *AccountService) { s.client = c }
}

func NewAccountService(c *client.Client, opts ...AccountOption) *AccountService {
	s := &AccountService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type MarginFundInfoRequest struct {
	ExchangeType types.ExchangeType
}

type marginFundInfoWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType"`
}

type MarginFundInfoResponse struct {
	Balance *domain.AccountBalance
}

func (s *AccountService) MarginFundInfo(ctx context.Context, accountID domain.AccountID, req MarginFundInfoRequest) (*domain.AccountBalance, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opMarginFundInfo, "accountID must not be empty")
	}
	var out domain.MarginFundInfoWire
	if err := s.client.Do(ctx, opMarginFundInfo, client.RouteTradeQueryMarginFundInfo, marginFundInfoWireRequest{
		ExchangeType: req.ExchangeType,
	}, s.client.JSON(), &out); err != nil {
		return nil, err
	}
	return domain.AccountBalanceFromDTO(&out), nil
}

type HoldsFilter struct {
	ExchangeType types.ExchangeType
}

type HoldsListRequest struct {
	AccountID domain.AccountID
	Filter    HoldsFilter
}

type holdsListWireRequest struct {
	ExchangeType types.ExchangeType `json:"exchangeType,omitempty"`
}

type holdsListWireResponse struct {
	HoldsList []domain.HoldsVoWire `json:"holdsList"`
}

func (s *AccountService) HoldsList(ctx context.Context, accountID domain.AccountID, filter HoldsFilter) ([]*domain.Position, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opHoldsList, "accountID must not be empty")
	}

	var out holdsListWireResponse
	if err := s.client.Do(ctx, opHoldsList, client.RouteTradeQueryHoldsList, holdsListWireRequest{
		ExchangeType: filter.ExchangeType,
	}, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	positions := make([]*domain.Position, len(out.HoldsList))
	for i := range out.HoldsList {
		positions[i] = domain.PositionFromDTO(&out.HoldsList[i])
	}
	return positions, nil
}

type FundJourFilter struct {
	ExchangeType types.ExchangeType
	StartDate    string
	EndDate      string
}

type RealFundJourListRequest struct {
	AccountID  domain.AccountID
	Filter     FundJourFilter
	Pagination transport.Pagination
}

type realFundJourListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
}

type fundJourListWireResponse struct {
	Data []domain.FundJourVoWire `json:"data"`
}

type RealFundJourListResponse struct {
	Entries []*domain.FundJournalEntry
	HasMore bool
	Cursor  string
}

func (s *AccountService) RealFundJourList(ctx context.Context, accountID domain.AccountID, filter FundJourFilter, pagination transport.Pagination) ([]*domain.FundJournalEntry, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opRealFundJourList, "accountID must not be empty")
	}

	var allEntries []*domain.FundJournalEntry
	cursor := pagination.Cursor
	pageSize := pagination.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	for {
		var out fundJourListWireResponse
		if err := s.client.Do(ctx, opRealFundJourList, client.RouteTradeQueryRealFundJourList, realFundJourListWireRequest{
			ExchangeType:  filter.ExchangeType,
			QueryCount:    pageSize,
			QueryParamStr: cursor,
		}, s.client.JSON(), &out); err != nil {
			return nil, err
		}

		for i := range out.Data {
			allEntries = append(allEntries, domain.FundJournalEntryFromDTO(&out.Data[i]))
		}

		if len(out.Data) == 0 {
			break
		}
		nextCursor := out.Data[len(out.Data)-1].QueryParamStr
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allEntries, nil
}

type HistoryFundJourListRequest struct {
	AccountID  domain.AccountID
	Filter     FundJourFilter
	Pagination transport.Pagination
}

type historyFundJourListWireRequest struct {
	ExchangeType  types.ExchangeType `json:"exchangeType"`
	QueryCount    int                `json:"queryCount"`
	QueryParamStr string             `json:"queryParamStr"`
	StartDate     string             `json:"startDate"`
	EndDate       string             `json:"endDate"`
}

type HistoryFundJourListResponse struct {
	Entries []*domain.FundJournalEntry
	HasMore bool
	Cursor  string
}

func (s *AccountService) HistoryFundJourList(ctx context.Context, accountID domain.AccountID, filter FundJourFilter, pagination transport.Pagination) ([]*domain.FundJournalEntry, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opHistoryFundJourList, "accountID must not be empty")
	}

	var allEntries []*domain.FundJournalEntry
	cursor := pagination.Cursor
	pageSize := pagination.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	for {
		var out fundJourListWireResponse
		if err := s.client.Do(ctx, opHistoryFundJourList, client.RouteTradeQueryHistoryFundJourList, historyFundJourListWireRequest{
			ExchangeType:  filter.ExchangeType,
			QueryCount:    pageSize,
			QueryParamStr: cursor,
			StartDate:     filter.StartDate,
			EndDate:       filter.EndDate,
		}, s.client.JSON(), &out); err != nil {
			return nil, err
		}

		for i := range out.Data {
			allEntries = append(allEntries, domain.FundJournalEntryFromDTO(&out.Data[i]))
		}

		if len(out.Data) == 0 {
			break
		}
		nextCursor := out.Data[len(out.Data)-1].QueryParamStr
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return allEntries, nil
}

type RateQueryListRequest struct {
	RateType string
}

type rateQueryListWireRequest struct {
	RateType string `json:"rateType,omitempty"`
}

type rateQueryListWireResponse map[string]map[string]string

type RateQueryListResponse struct {
	Rates []*domain.InterestRate
}

func (s *AccountService) RateQueryList(ctx context.Context) ([]*domain.InterestRate, error) {
	var out rateQueryListWireResponse
	if err := s.client.Do(ctx, opRateQueryList, client.RouteHsRateQueryList, rateQueryListWireRequest{}, s.client.JSON(), &out); err != nil {
		return nil, err
	}

	var rates []*domain.InterestRate
	for source, targets := range out {
		for target, rate := range targets {
			rates = append(rates, domain.InterestRateFromDTO(source, target, rate))
		}
	}

	return rates, nil
}
