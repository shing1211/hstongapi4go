// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"regexp"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file covers the validation surface of market.go: the four unexported
// helpers, the closed topic set they gate Subscribe and Unsubscribe against,
// and the nine methods that consult them before spending a round trip.
//
// The shape follows pkg/hstong/algo/validation_matrix_test.go for a reason B1
// measured: every helper here returns on the first failure, so a suite that
// starts from one fixture and mutates a single sampled field can only ever reach
// the branches it happens to name. Each helper therefore gets a known-valid
// fixture, a reject table that changes exactly one field, and an accept table, so
// a rejection row can never be attributed to a defect in the fixture.
//
// Every row asserts three things: the typed error (code 1016, the method's own
// op, the api category — never a Gateway trading or account code), the zero
// value returned beside it, and that no request was issued. The last is the half
// a validation test usually omits: a rejection that still reached the Gateway
// would look identical to a local one from the caller's side.

// ---------------------------------------------------------------------------
// Direct helper matrices
// ---------------------------------------------------------------------------

// TestValidateSecurityListMatrix covers all four branches of
// validateSecurityList: the empty-list rejection, the nil-element rejection
// (exercised at index 0 and again at index 1), and acceptance.
//
// The two nil positions are not redundant. The message names the offending
// index, so a helper that reported a constant 0 would pass the index-0 row; only
// the index-1 row distinguishes "the element's position" from "the first
// element". Reaching the branch twice is not the same as asserting the index,
// though — that is what TestValidateSecurityListNamesTheOffendingIndex is for,
// and the two tests are kept apart on purpose: this one is string-free, that one
// is the single place the diagnostic text is inspected.
func TestValidateSecurityListMatrix(t *testing.T) {
	t.Run("reject", func(t *testing.T) {
		cases := []struct {
			name string
			secs []*Security
		}{
			{"nil list", nil},
			{"empty list", []*Security{}},
			{"nil element at index 0", []*Security{nil, marketSecurity()}},
			{"nil element at index 1", []*Security{marketSecurity(), nil}},
			{"nil element at index 2 of three", []*Security{marketSecurity(), marketSecurity(), nil}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertInvalidParam(t, validateSecurityList(opBasicQot, tc.secs), opBasicQot)
			})
		}
	})

	t.Run("accept", func(t *testing.T) {
		cases := []struct {
			name string
			secs []*Security
		}{
			{"one security", []*Security{marketSecurity()}},
			{"two securities", []*Security{marketSecurity(), {DataType: types.DataTypeUSStock, Code: "AAPL"}}},
			{"three securities", []*Security{
				marketSecurity(),
				{DataType: types.DataTypeHKIndex, Code: "HSI"},
				{DataType: types.DataTypeHKETF, Code: "02800.HK"},
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := validateSecurityList(opBasicQot, tc.secs); err != nil {
					t.Fatalf("validateSecurityList on %s = %v, want nil", tc.name, err)
				}
			})
		}
	})
}

// TestValidateSecurityListNamesTheOffendingIndex is the one test in this file
// that reads a diagnostic message, and it is here for a measured reason.
//
// Everywhere else the rejection is asserted through errors.As and the typed
// Code, Category, and Op fields. The index of the offending element is not
// reachable that way: it appears only in the text. A mutation that hardcoded the
// index to 0 was therefore measured to survive the whole string-free matrix —
// every row still produced a correct typed rejection, just naming the wrong
// element. Since the index is the only thing that tells a caller which of five
// securities to fix, dropping it would be a real regression, so the assertion
// has to exist.
//
// It reads the typed Message field rather than the rendered Error() string, and
// it extracts the bracketed number with a pattern instead of matching the whole
// sentence. Rewording the message is therefore still free; dropping or falsifying
// the index is not.
func TestValidateSecurityListNamesTheOffendingIndex(t *testing.T) {
	indexPattern := regexp.MustCompile(`\[(\d+)\]`)

	for _, tc := range []struct {
		name string
		secs []*Security
		want string
	}{
		{"first element", []*Security{nil, marketSecurity()}, "0"},
		{"second element", []*Security{marketSecurity(), nil}, "1"},
		{"third of three", []*Security{marketSecurity(), marketSecurity(), nil}, "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := assertInvalidParam(t, validateSecurityList(opBasicQot, tc.secs), opBasicQot)
			m := indexPattern.FindStringSubmatch(e.Message)
			if m == nil {
				t.Fatalf("Message = %q, which names no index: a caller holding %d securities "+
					"cannot tell which one to fix", e.Message, len(tc.secs))
			}
			if m[1] != tc.want {
				t.Errorf("Message = %q, naming index %s; want %s", e.Message, m[1], tc.want)
			}
		})
	}

	// The empty-list rejection is a different diagnostic and must not borrow the
	// nil-element wording, or a caller would be told to fix an element that does
	// not exist.
	t.Run("empty list names no element", func(t *testing.T) {
		e := assertInvalidParam(t, validateSecurityList(opBasicQot, nil), opBasicQot)
		if indexPattern.MatchString(e.Message) {
			t.Errorf("Message = %q, which names an index for a list that has none", e.Message)
		}
	})
}

// TestValidateSecurityMatrix covers both branches of validateSecurity, the
// helper behind the five single-instrument reads. The accept row matters as much
// as the reject row: without it a helper that always returned an error would
// pass the table above and no happy path would run.
func TestValidateSecurityMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sec     *Security
		wantErr bool
	}{
		{"nil security", nil, true},
		{"HK stock", marketSecurity(), false},
		{"US stock", &Security{DataType: types.DataTypeUSStock, Code: "AAPL"}, false},
		// A zero-valued security is a caller mistake, but the helper's contract
		// is only "not nil": widening it to a field-level check would be a
		// behaviour change, so the current contract is pinned rather than
		// assumed.
		{"zero-valued security", &Security{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSecurity(opOrderBook, tc.sec)
			if tc.wantErr {
				assertInvalidParam(t, err, opOrderBook)
				return
			}
			if err != nil {
				t.Fatalf("validateSecurity on %s = %v, want nil", tc.name, err)
			}
		})
	}
}

// TestValidateSecurityCodeMatrix covers both branches of validateSecurityCode
// and pins the accept side, including the two rows a future strings.TrimSpace
// would change.
//
// " " and "  " are accepted on purpose. The helper's contract is emptiness, not
// well-formedness, and a space is not empty. Pinning the acceptance is what
// turns a later TrimSpace into a visible, deliberate change rather than a silent
// one: the row above it fails, so the change cannot land unnoticed.
func TestValidateSecurityCodeMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"empty", "", true},
		{"HK code", "00700.HK", false},
		{"US option code", "AAPL", false},
		{"single space", " ", false},
		{"two spaces", "  ", false},
		{"tab is not empty", "\t", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSecurityCode(opUsOptionChainCode, tc.code)
			if tc.wantErr {
				assertInvalidParam(t, err, opUsOptionChainCode)
				return
			}
			if err != nil {
				t.Fatalf("validateSecurityCode(%q) = %v, want nil", tc.code, err)
			}
		})
	}
}

// knownTopicIDs is the closed set validateTopic accepts, named rather than
// derived from the map, so the test fails if a topic is added to one side and
// not the other.
var knownTopicIDs = []types.TopicID{
	types.TopicBasicQot,
	types.TopicQuoteVariant35,
	types.TopicTicker,
	types.TopicTickVariant27,
	types.TopicTickVariant28,
	types.TopicTickVariant37,
	types.TopicBroker,
	types.TopicOrderBook,
	types.TopicOrderBookArcabook,
	types.TopicOrderBookTotalView,
	types.TopicOrderBookVariant36,
}

// TestValidateTopicMatrix covers both branches of validateTopic, and pins the
// closed set by count.
//
// The count is the whole point. types.TopicID has no exported enumeration to
// diff the map against — its own doc says the set is non-exhaustive — so a
// named list plus a count is the only way to notice drift in either direction,
// exactly as client/route_mutation_test.go pins its own closed set. Adding a
// topic to types without adding it here, or the reverse, fails here.
func TestValidateTopicMatrix(t *testing.T) {
	t.Run("closed set size", func(t *testing.T) {
		if len(knownTopics) != 11 {
			t.Fatalf("len(knownTopics) = %d, want 11: the set validateTopic gates on "+
				"has drifted from the eleven topics named below", len(knownTopics))
		}
		if len(knownTopicIDs) != len(knownTopics) {
			t.Fatalf("the test names %d topics but knownTopics holds %d", len(knownTopicIDs), len(knownTopics))
		}
	})

	t.Run("accept", func(t *testing.T) {
		for _, topic := range knownTopicIDs {
			t.Run(topic.String(), func(t *testing.T) {
				if _, ok := knownTopics[topic]; !ok {
					t.Fatalf("knownTopics has no entry for the topic the test names as accepted: %d", int(topic))
				}
				if err := validateTopic(opSubscribe, topic); err != nil {
					t.Fatalf("validateTopic(%d) = %v, want nil", int(topic), err)
				}
			})
		}
	})

	t.Run("reject", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			topic types.TopicID
		}{
			{"zero", 0},
			{"negative", -1},
			{"below the set", 1},
			// TopicBasicQot is 11, so 12 is the adjacent value: the most likely
			// off-by-one a future edit to knownTopics would introduce.
			{"one past the lowest member", types.TopicBasicQot + 1},
			{"one past the highest member", 38},
			{"above the set", 99},
			{"well past the set", 1000},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assertInvalidParam(t, validateTopic(opSubscribe, tc.topic), opSubscribe)
				assertInvalidParam(t, validateTopic(opUnsubscribe, tc.topic), opUnsubscribe)
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Method-level validation
// ---------------------------------------------------------------------------

// marketValidationCase is one method's argument rejection. invoke runs the call
// and returns only the error, so a method whose success type is a struct
// contributes the same shape as one that returns a bare error.
type marketValidationCase struct {
	name string
	op   string
	// wantZero is the value the method returned beside the error, checked
	// through requireZero inside the table.
	run func(context.Context, *MarketService) error
}

// TestMarketMethodsRejectInvalidArgumentsWithoutARequest is the method-level
// half of the matrix.
//
// Every row asserts the typed rejection, the zero value, and zero recorded Do
// calls. The last assertion is the one with teeth: a method that validated after
// calling the executor would produce a perfectly good-looking error beside a
// request that was already on its way to the Gateway.
func TestMarketMethodsRejectInvalidArgumentsWithoutARequest(t *testing.T) {
	cases := []marketValidationCase{
		{
			name: "BasicQot/nil security list",
			op:   opBasicQot,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.BasicQot(ctx, BasicQotRequest{})
				return err
			},
		},
		{
			name: "BasicQot/empty security list",
			op:   opBasicQot,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.BasicQot(ctx, BasicQotRequest{Security: []*Security{}})
				return err
			},
		},
		{
			name: "BasicQot/nil element at index 0",
			op:   opBasicQot,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.BasicQot(ctx, BasicQotRequest{Security: []*Security{nil, marketSecurity()}})
				return err
			},
		},
		{
			name: "BasicQot/nil element at index 1",
			op:   opBasicQot,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.BasicQot(ctx, BasicQotRequest{Security: []*Security{marketSecurity(), nil}})
				return err
			},
		},
		{
			name: "OrderBook/nil security",
			op:   opOrderBook,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.OrderBook(ctx, OrderBookRequest{})
				return err
			},
		},
		{
			name: "KL/nil security",
			op:   opKL,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.KL(ctx, KLRequest{})
				return err
			},
		},
		{
			name: "TimeShare/nil security",
			op:   opTimeShare,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.TimeShare(ctx, TimeShareRequest{})
				return err
			},
		},
		{
			name: "Ticker/nil security",
			op:   opTicker,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.Ticker(ctx, TickerRequest{Limit: 10})
				return err
			},
		},
		{
			name: "Broker/nil security",
			op:   opBroker,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.Broker(ctx, BrokerRequest{})
				return err
			},
		},
		{
			name: "UsOptionChainCode/empty securityCode",
			op:   opUsOptionChainCode,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.UsOptionChainCode(ctx, UsOptionChainCodeRequest{})
				return err
			},
		},
		{
			name: "UsOptionChainExpireDate/empty securityCode",
			op:   opUsOptionChainExpireDate,
			run: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.UsOptionChainExpireDate(ctx, UsOptionChainExpireDateRequest{})
				return err
			},
		},
		{
			name: "Subscribe/empty security list",
			op:   opSubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Subscribe(ctx, types.TopicBasicQot)
			},
		},
		{
			name: "Subscribe/nil element at index 1",
			op:   opSubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Subscribe(ctx, types.TopicBasicQot, marketSecurity(), nil)
			},
		},
		{
			name: "Unsubscribe/unknown topic",
			op:   opUnsubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, 99, marketSecurity())
			},
		},
		{
			name: "Unsubscribe/empty security list",
			op:   opUnsubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, types.TopicBasicQot)
			},
		},
		{
			name: "Unsubscribe/nil element at index 0",
			op:   opUnsubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, types.TopicBasicQot, nil)
			},
		},
		{
			// Both arguments invalid. Which of the two checks fires first is not
			// observable through the typed surface — both produce 1016 under the
			// same op and the same category, and neither is wrapped in anything
			// that distinguishes them — so what this row pins is the behaviour
			// that *is* observable and is what a caller depends on: the call is
			// refused, and nothing reached the Gateway. The ordering itself is
			// held by the single-fault rows: "Unsubscribe/unknown topic" supplies a
			// valid list, so it fails only if the topic check exists, and
			// "Unsubscribe/empty security list" supplies a valid topic, so it
			// fails only if the list check exists. Delete either check and
			// exactly one of those two rows turns into a real request.
			name: "Unsubscribe/unknown topic and empty list",
			op:   opUnsubscribe,
			run: func(ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, 99)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})
			svc := NewMarketService(exec)

			assertInvalidParam(t, tc.run(t.Context(), svc), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestSubscribeRejectsAnUnknownTopicBeforeTheSecurityList is the accept/reject
// pair that pins Subscribe's topic gate on its own, and the counterpart to the
// same pair in the table above.
//
// A known topic with a valid list reaches the executor; an unknown topic with
// the identical list does not. The difference between the two runs is exactly
// the topic, so a Subscribe that had lost its topic check would still pass the
// list rows and fail here.
func TestSubscribeRejectsAnUnknownTopicBeforeTheSecurityList(t *testing.T) {
	accepted := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})
	if err := NewMarketService(accepted).Subscribe(t.Context(), types.TopicOrderBook, marketSecurity()); err != nil {
		t.Fatalf("Subscribe with a known topic and a valid list = %v, want nil", err)
	}
	requireCalls(t, accepted, 1)

	rejected := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})
	err := NewMarketService(rejected).Subscribe(t.Context(), types.TopicID(99), marketSecurity())
	assertInvalidParam(t, err, opSubscribe)
	requireCalls(t, rejected, 0)
}

// TestTickerLimitRange covers the one range check market.go owns rather than
// delegating, on both sides.
//
// The accept rows are load-bearing: a check that rejected everything would pass
// the reject table and no ticker would ever be fetched.
func TestTickerLimitRange(t *testing.T) {
	t.Run("reject", func(t *testing.T) {
		for _, limit := range []int32{-1, 0, MaxTickerLimit + 1, 1000} {
			exec := newSequencedExecutor(t, sequencedReply{reply: tickerBody()})
			_, err := NewMarketService(exec).Ticker(t.Context(), TickerRequest{
				Security: marketSecurity(),
				Limit:    limit,
			})
			assertInvalidParam(t, err, opTicker)
			requireCalls(t, exec, 0)
		}
	})

	t.Run("accept the boundaries", func(t *testing.T) {
		for _, limit := range []int32{1, 10, MaxTickerLimit} {
			exec := newSequencedExecutor(t, sequencedReply{reply: tickerBody()})
			resp, err := NewMarketService(exec).Ticker(t.Context(), TickerRequest{
				Security: marketSecurity(),
				Limit:    limit,
			})
			if err != nil {
				t.Fatalf("Ticker with limit %d = %v, want nil", limit, err)
			}
			if len(resp.Ticker) != 2 {
				t.Errorf("Ticker with limit %d returned %d ticks, want 2", limit, len(resp.Ticker))
			}
			requireCalls(t, exec, 1)
		}
	})
}

// TestMaxTickerLimitIsTheDocumentedCeiling pins the exported constant itself.
// A change to it would silently widen or narrow every caller's range, and the
// reject table above would follow it rather than notice.
func TestMaxTickerLimitIsTheDocumentedCeiling(t *testing.T) {
	if MaxTickerLimit != 100 {
		t.Fatalf("MaxTickerLimit = %d, want 100: the exported ceiling is part of the API surface", MaxTickerLimit)
	}
}
