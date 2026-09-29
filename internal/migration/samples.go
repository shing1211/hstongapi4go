// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package migration holds the code samples published in docs/MIGRATION.md, as
// compiling code rather than as prose in a fenced block.
//
// A migration guide whose samples do not compile is worse than no guide: a reader
// who trusts it loses an afternoon instead of thirty seconds, and nothing in CI
// notices. A fenced block cannot be checked by anything.
//
// Every sample is therefore a real function calling the real API of the layer it
// documents, the package builds as part of `go build ./...`, and
// TestEverySampleInTheGuideIsPublished ties the two together: it reads
// docs/MIGRATION.md, extracts each ```go block, and fails if a block in the guide
// has no counterpart here. Deleting a sample from the guide without deleting the
// code fails, and so does leaving stale code behind.
//
// The functions are never called and never run. They exist to be compiled.
package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/services"
	"github.com/shing1211/hstongapi4go/pkg/transport"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// SampleReleasedBasicQot is the "before" of the REST migration: a manager built
// on *client.Client, a request from pkg/hstong/market, and a response of plain
// strings the caller must parse.
func SampleReleasedBasicQot(ctx context.Context, c *client.Client) error {
	mkt := market.New(c)

	resp, err := mkt.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{{
			DataType: int32(types.DataTypeHKStock),
			Code:     "00700.HK",
		}},
	})
	if err != nil {
		return err
	}

	// A string. Converting it is the caller's problem, and a float64 is where
	// v0.1.x's money defect lived.
	_ = resp
	return nil
}

// SampleVNextBasicQot is the "after": the Stack composes the six v-next services
// over one request path and closes as a unit, and a price arrives as a
// domain.Price that carries no invented grid.
func SampleVNextBasicQot(ctx context.Context, c *client.Client) error {
	stack := services.NewStack(c)
	defer func() { _ = stack.Close() }()

	resp, err := stack.Market.BasicQot(ctx, services.BasicQotRequest{
		Security: []*services.Security{{
			DataType: types.DataTypeHKStock,
			Code:     "00700.HK",
		}},
	})
	if err != nil {
		return err
	}

	// A decimal-backed Price. No ParseFloat, and nothing is rounded on the way
	// through, because a read-path price carries the zero tick.
	for _, q := range resp.BasicQot {
		if q != nil {
			fmt.Println(q.LastPrice.String())
		}
	}
	return nil
}

// SampleReleasedPositions is the "before" of the trade migration.
//
// HoldsVo.MarketValue, LastPrice, IncomeBalance, MarketValueRate and IncomeRatio
// are all marked deprecated in the Gateway's own field documentation; the SDK
// repeats that in GoDoc rather than hiding it. They are not removed in v1.0.
func SampleReleasedPositions(ctx context.Context, c *client.Client) error {
	trd := trade.New(c)

	positions, err := trd.Positions(ctx, trade.PositionsRequest{})
	if err != nil {
		return err
	}

	for _, p := range positions {
		// Deprecated and unreliable. Present, and not removed in v1.0.
		_ = p.MarketValue
		// The two fields that are actually usable.
		_ = p.EnableAmount
		_ = p.CurrentAmount
	}
	return nil
}

// SampleVNextPositions is the "after".
//
// The deprecated fields have no counterpart because the v-next mapper does not
// populate them: a value the Gateway marks unreliable is not reconstructed as
// though it were reliable.
func SampleVNextPositions(ctx context.Context, c *client.Client, accountID domain.AccountID) error {
	stack := services.NewStack(c)
	defer func() { _ = stack.Close() }()

	positions, err := stack.Account.HoldsList(ctx, accountID, services.HoldsFilter{})
	if err != nil {
		return err
	}

	for _, p := range positions {
		if p == nil {
			continue
		}
		// A Quantity is an exact decimal. Note that Quantity has no Add method -
		// Money does, because adding two Mights of different currencies is a
		// category error, while two Quantities of the same instrument are not.
		// Arithmetic goes through Decimal, so it stays exact.
		total := p.EnableAmount.Decimal().Add(p.CurrentAmount.Decimal())
		fmt.Println(total.String())
	}
	return nil
}

// SampleReleasedErrorMatching is the "before", and it is a pattern this
// repository's own conventions forbid in library code.
//
// It compiles, and it is wrong in a way no compiler can see: the message text is
// not part of any contract, and a Gateway that rewords a message breaks it
// silently.
func SampleReleasedErrorMatching(err error) bool {
	return err != nil && strings.Contains(err.Error(), "1012")
}

// SampleVNextErrorMatching is the "after".
//
// errors.As reaches the typed error through any wrapping, which is the property
// the string form can never have.
func SampleVNextErrorMatching(err error) bool {
	var gwErr *errs.Error
	if !errors.As(err, &gwErr) {
		return false
	}
	return gwErr.Code == types.StatusNotLoggedIn
}

// SampleReleasedPush is the "before" of the push migration: a stream.Client
// whose Updates channel carries an Event the caller type-asserts.
func SampleReleasedPush(ctx context.Context, c *client.Client) error {
	s := stream.New(c)
	if err := s.Connect(ctx); err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	sub, err := s.Subscribe(ctx, types.TopicBasicQot, &dto.Security{
		DataType: int32(types.DataTypeHKStock),
		Code:     "00700.HK",
	})
	if err != nil {
		return err
	}

	for ev := range sub.Updates() {
		// A failed assertion is a runtime branch on a push goroutine, and the
		// payload behind it is still a raw protobuf message, so the price is
		// reached through two nested getters.
		if q, ok := ev.BasicQot(); ok {
			fmt.Println(q.GetBasicQot().GetLastPrice())
		}
	}
	return nil
}

// SampleVNextPush is the "after".
//
// Push is opt-in, so a caller who does not want the TCP socket does not get one,
// and every delivered update carries a non-nil Event, so the type switch cannot
// fail.
func SampleVNextPush(ctx context.Context, c *client.Client) error {
	adapter, err := transport.NewPushAdapter()
	if err != nil {
		return err
	}
	stack := services.NewStack(c, services.WithPushTransport(adapter))
	// One Close releases the push connection and the HTTP client together, in
	// that order, and returns the first error while attempting both.
	defer func() { _ = stack.Close() }()

	if err := stack.Push.Connect(ctx); err != nil {
		return err
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	go func() { _ = stack.Push.Run(runCtx) }()

	sub, err := stack.Push.Subscribe(ctx, types.TopicBasicQot, &services.Security{
		DataType: types.DataTypeHKStock,
		Code:     "00700.HK",
	})
	if err != nil {
		return err
	}

	for u := range sub.Updates() {
		switch ev := u.Event.(type) {
		case domain.QuoteEvent:
			// A value type, so there is no failed assertion and no nil check.
			fmt.Println(ev.LastPrice.String())
		case domain.SystemEvent:
			// A frame the SDK could not classify is surfaced, not dropped.
			fmt.Println(ev.Code)
		}
	}
	return nil
}
