// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/hstong/algo"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestActionValid asserts Action.Valid accepts exactly the four documented
// codes and rejects everything else, including the empty string and the
// human-readable names.
func TestActionValid(t *testing.T) {
	cases := []struct {
		name string
		in   algo.Action
		want bool
	}{
		{"start", algo.ActionStart, true},
		{"stop", algo.ActionStop, true},
		{"suspend", algo.ActionSuspend, true},
		{"resume", algo.ActionResume, true},
		{"empty", "", false},
		{"zero", "0", false},
		{"out of range", "5", false},
		{"name instead of code", "START", false},
		{"lowercase name", "start", false},
		{"whitespace", " 1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Valid(); got != tc.want {
				t.Fatalf("Action(%q).Valid() = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestActionString asserts the documented names and the non-panicking fallback.
func TestActionString(t *testing.T) {
	cases := []struct {
		in   algo.Action
		want string
	}{
		{algo.ActionStart, "START"},
		{algo.ActionStop, "STOP"},
		{algo.ActionSuspend, "SUSPEND"},
		{algo.ActionResume, "RESUME"},
		{"9", "Action(9)"},
		{"", "Action()"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Fatalf("Action(%q).String() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestActionOrder_PerActionRequest asserts every documented action produces
// exactly one request carrying that action code and its required parameters.
func TestActionOrder_PerActionRequest(t *testing.T) {
	actions := []algo.Action{
		algo.ActionStart,
		algo.ActionStop,
		algo.ActionSuspend,
		algo.ActionResume,
	}

	for _, action := range actions {
		t.Run(action.String(), func(t *testing.T) {
			m, rec := newManager(t, map[string]string{
				pathActionOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`,
			})

			params := validActionOrder()
			params.Action = action

			if _, err := m.ActionOrder(context.Background(), params); err != nil {
				t.Fatalf("ActionOrder(%q): %v", action, err)
			}
			if got := rec.count(pathActionOrder); got != 1 {
				t.Fatalf("requests = %d, want exactly 1", got)
			}
			assertRequest(t, rec.last(pathActionOrder), pathActionOrder, `{
				"orderId": "MASTER-1",
				"action": "`+string(action)+`",
				"targetStrategy": "1",
				"exchangeType": "K"
			}`)
		})
	}
}

// TestActionOrder_RejectsInvalidAction asserts an action outside the documented
// set is rejected locally, wrapping ErrInvalidParams, with no request sent.
func TestActionOrder_RejectsInvalidAction(t *testing.T) {
	cases := []struct {
		name   string
		action algo.Action
	}{
		{"empty", ""},
		{"zero", "0"},
		{"out of range", "5"},
		{"name", "START"},
		{"lowercase", "start"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newManager(t, nil)

			params := validActionOrder()
			params.Action = tc.action

			_, err := m.ActionOrder(context.Background(), params)
			if err == nil {
				t.Fatal("ActionOrder = nil error, want validation error")
			}
			if !errors.Is(err, algo.ErrInvalidParams) {
				t.Fatalf("errors.Is(err, ErrInvalidParams) = false, err = %v", err)
			}
			if got := rec.total(); got != 0 {
				t.Fatalf("requests = %d, want 0", got)
			}
		})
	}
}

// TestActionOrder_RequiredParamsPerAction asserts each documented action
// requires orderId, targetStrategy, and exchangeType (the reference request
// example omits exchangeType, but the parameter table marks it required).
func TestActionOrder_RequiredParamsPerAction(t *testing.T) {
	actions := []algo.Action{
		algo.ActionStart,
		algo.ActionStop,
		algo.ActionSuspend,
		algo.ActionResume,
	}

	for _, action := range actions {
		t.Run(action.String(), func(t *testing.T) {
			base := validActionOrder()
			base.Action = action

			missingOrder := base
			missingOrder.OrderID = ""

			missingStrategy := base
			missingStrategy.TargetStrategy = ""

			missingExchange := base
			missingExchange.ExchangeType = ""

			cases := []struct {
				name   string
				params algo.ActionOrderParams
			}{
				{"missing orderId", missingOrder},
				{"missing targetStrategy", missingStrategy},
				{"missing exchangeType", missingExchange},
			}

			for _, tc := range cases {
				m, rec := newManager(t, nil)
				_, err := m.ActionOrder(context.Background(), tc.params)
				if err == nil {
					t.Fatalf("%s: error = nil, want validation error", tc.name)
				}
				if !errors.Is(err, algo.ErrInvalidParams) {
					t.Fatalf("%s: errors.Is(err, ErrInvalidParams) = false, err = %v", tc.name, err)
				}
				if got := rec.total(); got != 0 {
					t.Fatalf("%s: requests = %d, want 0", tc.name, got)
				}
			}
		})
	}
}

// TestActionOrder_DefaultExchangeType asserts a configured default satisfies
// the exchangeType requirement for an action call.
func TestActionOrder_DefaultExchangeType(t *testing.T) {
	m, rec := newManager(t,
		map[string]string{pathActionOrder: `{"ok":true,"err":"","data":{"data":"MASTER-1"}}`},
		algo.WithDefaultExchangeType(types.ExchangeUS),
	)

	params := validActionOrder()
	params.ExchangeType = ""

	if _, err := m.ActionOrder(context.Background(), params); err != nil {
		t.Fatalf("ActionOrder: %v", err)
	}
	assertRequest(t, rec.last(pathActionOrder), pathActionOrder, `{
		"orderId": "MASTER-1",
		"action": "1",
		"targetStrategy": "1",
		"exchangeType": "P"
	}`)
}
