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

const (
	// DefaultPageSize is the page size applied when the requested size is
	// missing or non-positive.
	DefaultPageSize = 50
	// MaxPageSize is the largest page size the Gateway accepts. The documented
	// limit is strictly less than 100, so the cap is 99.
	MaxPageSize = 99
	// maxPages bounds a cursor walk so a Gateway that keeps issuing fresh
	// cursors cannot make the walk unbounded.
	maxPages = 1000
)

// clampPageSize bounds a requested page size to [1, MaxPageSize], substituting
// DefaultPageSize for a non-positive value.
func clampPageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

// walkFundJourPages walks a queryParamStr cursor list and returns every row
// gathered, in order. It issues exactly one fetch per page and never retries.
//
// The walk stops when a page is empty, when the Gateway reports no further
// cursor, when the cursor fails to advance, or after maxPages, so a Gateway
// that ignores queryParamStr cannot cause an unbounded loop. A fetch or context
// failure returns the rows gathered so far together with the error, so a partial
// outage is visible to the caller without discarding completed pages.
func walkFundJourPages(
	ctx context.Context,
	pageSize int,
	cursor string,
	fetch func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error),
) ([]*domain.FundJournalEntry, error) {
	size := clampPageSize(pageSize)
	var all []*domain.FundJournalEntry
	for page := 0; page < maxPages; page++ {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		entries, next, err := fetch(ctx, size, cursor)
		if err != nil {
			return all, err
		}
		all = append(all, entries...)
		if len(entries) == 0 || next == "" || next == cursor {
			return all, nil
		}
		cursor = next
	}
	return all, nil
}

type AccountService struct {
	client Executor
}

type AccountOption func(*AccountService)

func WithAccountClient(c Executor) AccountOption {
	return func(s *AccountService) { s.client = c }
}

func NewAccountService(c Executor, opts ...AccountOption) *AccountService {
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
	if err := s.client.Do(ctx, opMarginFundInfo, client.RouteTradeQueryMarginFundInfo, marginFundInfoWireRequest(req), s.client.JSON(), &out); err != nil {
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
	if err := s.client.Do(ctx, opHoldsList, client.RouteTradeQueryHoldsList, holdsListWireRequest(filter), s.client.JSON(), &out); err != nil {
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

	return walkFundJourPages(ctx, pagination.PageSize, pagination.Cursor,
		func(ctx context.Context, pageSize int, cursor string) ([]*domain.FundJournalEntry, string, error) {
			var out fundJourListWireResponse
			if err := s.client.Do(ctx, opRealFundJourList, client.RouteTradeQueryRealFundJourList, realFundJourListWireRequest{
				ExchangeType:  filter.ExchangeType,
				QueryCount:    pageSize,
				QueryParamStr: cursor,
			}, s.client.JSON(), &out); err != nil {
				return nil, "", err
			}
			entries := make([]*domain.FundJournalEntry, 0, len(out.Data))
			for i := range out.Data {
				entries = append(entries, domain.FundJournalEntryFromDTO(&out.Data[i]))
			}
			next := ""
			if len(out.Data) > 0 {
				next = out.Data[len(out.Data)-1].QueryParamStr
			}
			return entries, next, nil
		})
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

	return walkFundJourPages(ctx, pagination.PageSize, pagination.Cursor,
		func(ctx context.Context, pageSize int, cursor string) ([]*domain.FundJournalEntry, string, error) {
			var out fundJourListWireResponse
			if err := s.client.Do(ctx, opHistoryFundJourList, client.RouteTradeQueryHistoryFundJourList, historyFundJourListWireRequest{
				ExchangeType:  filter.ExchangeType,
				QueryCount:    pageSize,
				QueryParamStr: cursor,
				StartDate:     filter.StartDate,
				EndDate:       filter.EndDate,
			}, s.client.JSON(), &out); err != nil {
				return nil, "", err
			}
			entries := make([]*domain.FundJournalEntry, 0, len(out.Data))
			for i := range out.Data {
				entries = append(entries, domain.FundJournalEntryFromDTO(&out.Data[i]))
			}
			next := ""
			if len(out.Data) > 0 {
				next = out.Data[len(out.Data)-1].QueryParamStr
			}
			return entries, next, nil
		})
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
