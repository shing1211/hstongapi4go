// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo

import (
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The seven fixtures below each describe a request that passes every branch of
// its validate method. The matrices then mutate exactly one field at a time, so
// because validate returns on the first failure the rejection is attributable
// to that field and no other. Pinned this way, deleting any single check from a
// validate method turns its row into a nil error and fails the test.

// fullAddOrder returns an AddOrderParams that satisfies every AddOrder branch.
func fullAddOrder() AddOrderParams {
	return AddOrderParams{
		StockCode:      "700.HK",
		ExchangeType:   types.ExchangeHK,
		EntrustType:    EntrustTypeLimit,
		EntrustPrice:   "350.5",
		EntrustAmount:  "1000",
		EntrustBS:      types.EntrustBuy,
		TargetStrategy: StrategyVWAP,
		SessionType:    SessionTypeOff,
		StrategyParam: StrategyParam{
			MaxVolume:   "100",
			Sensitivity: SensitivityNeutral,
		},
	}
}

// fullCancelOrder returns a CancelOrderParams that satisfies every branch.
func fullCancelOrder() CancelOrderParams {
	return CancelOrderParams{OrderID: "MASTER-1", ExchangeType: types.ExchangeHK}
}

// fullCancelEntrust returns a CancelEntrustParams that satisfies every branch.
func fullCancelEntrust() CancelEntrustParams {
	return CancelEntrustParams{OrderID: "MASTER-1", EntrustID: "CHILD-9", ExchangeType: types.ExchangeHK}
}

// fullChangeOrder returns a ChangeOrderParams that satisfies every branch.
func fullChangeOrder() ChangeOrderParams {
	return ChangeOrderParams{
		OrderID:       "MASTER-1",
		StockCode:     "700.HK",
		ExchangeType:  types.ExchangeHK,
		EntrustPrice:  "360",
		EntrustAmount: "800",
		StrategyParam: StrategyParam{Sensitivity: SensitivityAggressive},
	}
}

// fullActionOrder returns an ActionOrderParams that satisfies every branch.
func fullActionOrder() ActionOrderParams {
	return ActionOrderParams{
		OrderID:        "MASTER-1",
		Action:         ActionStart,
		TargetStrategy: StrategyVWAP,
		ExchangeType:   types.ExchangeHK,
	}
}

// fullQueryOrderList returns a QueryOrderListParams that satisfies every branch.
func fullQueryOrderList() QueryOrderListParams {
	return QueryOrderListParams{
		PageNo:       "1",
		PageSize:     "30",
		StartDate:    "20260101",
		EndDate:      "20260131",
		ExchangeType: types.ExchangeHK,
		StockCode:    "700.HK",
	}
}

// fullQueryEntrustIDList returns a QueryEntrustIDListParams that satisfies every
// branch.
func fullQueryEntrustIDList() QueryEntrustIDListParams {
	return QueryEntrustIDListParams{
		OrderID:      "MASTER-1",
		TradeDate:    "20260105",
		ExchangeType: types.ExchangeHK,
	}
}

// rejectCase is one required-field or format branch of a validate method: op is
// the operation label the error must carry, and run invokes validate on a copy
// of a known-valid fixture with exactly one field changed.
type rejectCase struct {
	name string
	op   string
	run  func() error
}

// assertRejected asserts err is a local validation failure: it wraps
// ErrInvalidParams, carries the expected operation label, and carries no
// Gateway status code. The last part is the fail-closed contract — a request
// rejected before it is sent has no code to report, so errors.Is against
// ErrInvalidParams is the only way a caller can tell a local rejection from a
// Gateway rejection, and errs.CodeOf must not fabricate one.
func assertRejected(t *testing.T, name string, err error, wantOp string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: validate() = nil, want a local rejection", name)
	}
	if !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("%s: errors.Is(err, ErrInvalidParams) = false", name)
	}
	if code, ok := errs.CodeOf(err); ok {
		t.Fatalf("%s: errs.CodeOf(err) = (%q, true), want no code for a request that was never sent", name, code)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("%s: errors.As(err, *errs.Error) = false", name)
	}
	if e.Op != wantOp {
		t.Fatalf("%s: op = %q, want %q", name, e.Op, wantOp)
	}
	if e.Category != errs.CategoryUnknown {
		t.Fatalf("%s: category = %q, want %q for a local rejection", name, e.Category, errs.CategoryUnknown)
	}
}

// TestValidateRejectsEveryField walks every required-field and closed-set branch
// of all seven validate methods. The happy path alone covered only the first
// rejection of each method, because validate returns on the first failure: a
// branch after a field the suite never blanked was unreachable.
func TestValidateRejectsEveryField(t *testing.T) {
	cases := []rejectCase{
		// AddOrderParams.validate: every field is required, and three of them
		// additionally have a closed code set or a decimal format.
		{"add/stockCode required", opAddOrder, func() error {
			p := fullAddOrder()
			p.StockCode = ""
			return p.validate(opAddOrder)
		}},
		{"add/exchangeType required", opAddOrder, func() error {
			p := fullAddOrder()
			p.ExchangeType = ""
			return p.validate(opAddOrder)
		}},
		{"add/entrustType required", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustType = ""
			return p.validate(opAddOrder)
		}},
		{"add/entrustType not in set", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustType = "9"
			return p.validate(opAddOrder)
		}},
		{"add/entrustPrice required", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustPrice = ""
			return p.validate(opAddOrder)
		}},
		{"add/entrustPrice not a positive decimal", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustPrice = "0"
			return p.validate(opAddOrder)
		}},
		{"add/entrustAmount required", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustAmount = ""
			return p.validate(opAddOrder)
		}},
		{"add/entrustAmount not a positive decimal", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustAmount = "-1"
			return p.validate(opAddOrder)
		}},
		{"add/entrustBs required", opAddOrder, func() error {
			p := fullAddOrder()
			p.EntrustBS = ""
			return p.validate(opAddOrder)
		}},
		{"add/targetStrategy required", opAddOrder, func() error {
			p := fullAddOrder()
			p.TargetStrategy = ""
			return p.validate(opAddOrder)
		}},
		{"add/sessionType required", opAddOrder, func() error {
			p := fullAddOrder()
			p.SessionType = ""
			return p.validate(opAddOrder)
		}},
		{"add/sessionType not in set", opAddOrder, func() error {
			p := fullAddOrder()
			p.SessionType = "2"
			return p.validate(opAddOrder)
		}},
		{"add/maxVolume required", opAddOrder, func() error {
			p := fullAddOrder()
			p.StrategyParam.MaxVolume = ""
			return p.validate(opAddOrder)
		}},
		{"add/maxVolume not a positive decimal", opAddOrder, func() error {
			p := fullAddOrder()
			p.StrategyParam.MaxVolume = "0.000"
			return p.validate(opAddOrder)
		}},
		{"add/sensitivity required", opAddOrder, func() error {
			p := fullAddOrder()
			p.StrategyParam.Sensitivity = ""
			return p.validate(opAddOrder)
		}},
		{"add/sensitivity not in set", opAddOrder, func() error {
			p := fullAddOrder()
			p.StrategyParam.Sensitivity = "7"
			return p.validate(opAddOrder)
		}},

		// CancelOrderParams.validate.
		{"cancel/orderId required", opCancelOrder, func() error {
			p := fullCancelOrder()
			p.OrderID = ""
			return p.validate(opCancelOrder)
		}},
		{"cancel/exchangeType required", opCancelOrder, func() error {
			p := fullCancelOrder()
			p.ExchangeType = ""
			return p.validate(opCancelOrder)
		}},

		// CancelEntrustParams.validate.
		{"cancelEntrust/orderId required", opCancelEntrust, func() error {
			p := fullCancelEntrust()
			p.OrderID = ""
			return p.validate(opCancelEntrust)
		}},
		{"cancelEntrust/entrustId required", opCancelEntrust, func() error {
			p := fullCancelEntrust()
			p.EntrustID = ""
			return p.validate(opCancelEntrust)
		}},
		{"cancelEntrust/exchangeType required", opCancelEntrust, func() error {
			p := fullCancelEntrust()
			p.ExchangeType = ""
			return p.validate(opCancelEntrust)
		}},

		// ChangeOrderParams.validate. Every StrategyParam member is optional
		// here, so sensitivity is only checked when it is present.
		{"change/orderId required", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.OrderID = ""
			return p.validate(opChangeOrder)
		}},
		{"change/stockCode required", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.StockCode = ""
			return p.validate(opChangeOrder)
		}},
		{"change/exchangeType required", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.ExchangeType = ""
			return p.validate(opChangeOrder)
		}},
		{"change/entrustPrice required", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.EntrustPrice = ""
			return p.validate(opChangeOrder)
		}},
		{"change/entrustPrice not a positive decimal", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.EntrustPrice = "1e5"
			return p.validate(opChangeOrder)
		}},
		{"change/entrustAmount required", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.EntrustAmount = ""
			return p.validate(opChangeOrder)
		}},
		{"change/entrustAmount not a positive decimal", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.EntrustAmount = "0"
			return p.validate(opChangeOrder)
		}},
		{"change/sensitivity not in set", opChangeOrder, func() error {
			p := fullChangeOrder()
			p.StrategyParam.Sensitivity = "9"
			return p.validate(opChangeOrder)
		}},

		// ActionOrderParams.validate.
		{"action/orderId required", opActionOrder, func() error {
			p := fullActionOrder()
			p.OrderID = ""
			return p.validate(opActionOrder)
		}},
		{"action/targetStrategy required", opActionOrder, func() error {
			p := fullActionOrder()
			p.TargetStrategy = ""
			return p.validate(opActionOrder)
		}},
		{"action/exchangeType required", opActionOrder, func() error {
			p := fullActionOrder()
			p.ExchangeType = ""
			return p.validate(opActionOrder)
		}},
		{"action/action not in set", opActionOrder, func() error {
			p := fullActionOrder()
			p.Action = "5"
			return p.validate(opActionOrder)
		}},

		// QueryOrderListParams.validate. exchangeType is optional only while
		// stockCode is absent.
		{"query/pageNo required", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.PageNo = ""
			return p.validate(opQueryOrderList)
		}},
		{"query/pageNo not a positive integer", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.PageNo = "1.0"
			return p.validate(opQueryOrderList)
		}},
		{"query/pageSize required", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.PageSize = ""
			return p.validate(opQueryOrderList)
		}},
		{"query/pageSize not a positive integer", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.PageSize = "0"
			return p.validate(opQueryOrderList)
		}},
		{"query/startDate required", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.StartDate = ""
			return p.validate(opQueryOrderList)
		}},
		{"query/startDate not yyyyMMdd", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.StartDate = "2026010"
			return p.validate(opQueryOrderList)
		}},
		{"query/endDate required", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.EndDate = ""
			return p.validate(opQueryOrderList)
		}},
		{"query/endDate not yyyyMMdd", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.EndDate = "202601011"
			return p.validate(opQueryOrderList)
		}},
		{"query/stockCode without exchangeType", opQueryOrderList, func() error {
			p := fullQueryOrderList()
			p.ExchangeType = ""
			return p.validate(opQueryOrderList)
		}},

		// QueryEntrustIDListParams.validate.
		{"entrustQuery/orderId required", opQueryEntrustIDList, func() error {
			p := fullQueryEntrustIDList()
			p.OrderID = ""
			return p.validate(opQueryEntrustIDList)
		}},
		{"entrustQuery/tradeDate required", opQueryEntrustIDList, func() error {
			p := fullQueryEntrustIDList()
			p.TradeDate = ""
			return p.validate(opQueryEntrustIDList)
		}},
		{"entrustQuery/tradeDate not yyyyMMdd", opQueryEntrustIDList, func() error {
			p := fullQueryEntrustIDList()
			p.TradeDate = "2026010a"
			return p.validate(opQueryEntrustIDList)
		}},
		{"entrustQuery/exchangeType required", opQueryEntrustIDList, func() error {
			p := fullQueryEntrustIDList()
			p.ExchangeType = ""
			return p.validate(opQueryEntrustIDList)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRejected(t, tc.name, tc.run(), tc.op)
		})
	}
}

// TestValidateAcceptsWellFormedRequests is the other half of the matrix: every
// fixture above must pass its own validate, otherwise the rejection rows could
// be attributed to a fixture defect rather than to the branch under test.
func TestValidateAcceptsWellFormedRequests(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"add", func() error { return fullAddOrder().validate(opAddOrder) }},
		{"cancel", func() error { return fullCancelOrder().validate(opCancelOrder) }},
		{"cancelEntrust", func() error { return fullCancelEntrust().validate(opCancelEntrust) }},
		{"change", func() error { return fullChangeOrder().validate(opChangeOrder) }},
		{"action", func() error { return fullActionOrder().validate(opActionOrder) }},
		{"query", func() error { return fullQueryOrderList().validate(opQueryOrderList) }},
		{"entrustQuery", func() error {
			return fullQueryEntrustIDList().validate(opQueryEntrustIDList)
		}},
		// ChangeOrder treats every StrategyParam member as optional, so an
		// absent sensitivity is accepted while a present wrong one is not.
		{"change/empty strategyParam", func() error {
			p := fullChangeOrder()
			p.StrategyParam = StrategyParam{}
			return p.validate(opChangeOrder)
		}},
		// QueryOrderList drops exchangeType along with stockCode: the pair is
		// what makes the market ambiguous, not either field alone.
		{"query/no exchangeType and no stockCode", func() error {
			p := fullQueryOrderList()
			p.ExchangeType = ""
			p.StockCode = ""
			return p.validate(opQueryOrderList)
		}},
		// An add request may omit every optional StrategyParam member other
		// than maxVolume and sensitivity, which AddOrder does require.
		{"add/optional strategyParam members absent", func() error {
			p := fullAddOrder()
			p.StrategyParam = StrategyParam{MaxVolume: "100", Sensitivity: SensitivityPassive}
			return p.validate(opAddOrder)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("validate() on a well-formed request = %v, want nil", err)
			}
		})
	}
}

// TestIsPositiveInt exercises the predicate directly. Its empty-string guard is
// unreachable through validate, which checks for "" first, so only a direct
// call can reach it.
func TestIsPositiveInt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1", true},
		{"30", true},
		{"007", true},
		{"99999999999999999999", true},
		{"", false},
		{"0", false},
		{"00", false},
		{"-1", false},
		{"+1", false},
		{"1.0", false},
		{"1 2", false},
		{" 1", false},
		{"1\n", false},
		{"abc", false},
		// Multi-byte digits: the predicate is byte-wise, so a rune that is a
		// digit in some script is still rejected here rather than silently
		// truncated into a different number.
		{"١٢٣", false},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isPositiveInt(tc.in); got != tc.want {
				t.Fatalf("isPositiveInt(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestIsPositiveDecimal exercises the predicate directly, covering the shapes
// its callers never construct: validate rejects "" before calling it, so both
// the empty guard and the degenerate ".5" / "1." / "1.2.3" forms are only
// reachable from here.
func TestIsPositiveDecimal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"integer", "1", true},
		{"fraction", "1.5", true},
		{"many fraction digits", "0.1234567890123456789", true},
		{"leading zeros", "000.5", true},
		{"trailing fraction zeros", "1.500", true},
		{"large integer part", "99999999999999999999.1", true},
		{"empty", "", false},
		{"zero", "0", false},
		{"zero with fraction", "0.000", false},
		{"negative", "-1", false},
		{"explicit plus", "+1", false},
		{"dot only", ".", false},
		{"no integer digits", ".5", false},
		{"no fraction digits", "1.", false},
		{"two dots", "1.2.3", false},
		{"many dots", "1.2.3.4", false},
		{"exponent notation", "1e5", false},
		{"uppercase exponent", "1E5", false},
		{"comma decimal separator", "1,5", false},
		{"thousands separator", "1,000", false},
		{"leading space", " 1", false},
		{"trailing space", "1 ", false},
		{"hex", "0x10", false},
		{"whitespace inside", "1 .5", false},
		{"words", "abc", false},
		{"non-ASCII digits", "١٢٣", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPositiveDecimal(tc.in); got != tc.want {
				t.Fatalf("isPositiveDecimal(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
