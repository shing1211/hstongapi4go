// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"testing"
)

// wantRouteCount is the number of canonical endpoints in plan.md §Appendix A.
const wantRouteCount = 51

func TestRoutes_Count(t *testing.T) {
	if got := len(Routes()); got != wantRouteCount {
		t.Fatalf("len(Routes()) = %d, want %d", got, wantRouteCount)
	}
}

// TestRoute_ValidateAndPath checks every registered route: it validates, its
// Path matches its constant, and the registry contains no duplicate constants.
func TestRoute_ValidateAndPath(t *testing.T) {
	seen := make(map[Route]bool, wantRouteCount)
	for _, route := range Routes() {
		if seen[route] {
			t.Fatalf("duplicate route %q", route)
		}
		seen[route] = true

		if err := route.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", route, err)
		}
		if got := route.Path(); got != string(route) {
			t.Errorf("Path() of %q = %q, want %q", route, got, string(route))
		}
		if got := route.String(); got != string(route) {
			t.Errorf("String() of %q = %q, want %q", route, got, string(route))
		}
	}
}

// TestNormalizePath_AliasForms checks that all three Gateway alias forms of
// every one of the 51 canonical routes normalize to the same canonical path.
func TestNormalizePath_AliasForms(t *testing.T) {
	for _, route := range Routes() {
		canonical := string(route)
		forms := []struct {
			name string
			in   string
		}{
			{"canonical", canonical},
			{"Request", canonical + "Request"},
			{"RequestMsgType", canonical + "RequestMsgType"},
		}
		for _, form := range forms {
			if got := NormalizePath(form.in); got != canonical {
				t.Errorf("NormalizePath(%q) [%s] = %q, want %q", form.in, form.name, got, canonical)
			}
		}
	}
}

// TestNormalizePath_Idempotent checks that NormalizePath is a no-op on its own
// output for every canonical route and alias form.
func TestNormalizePath_Idempotent(t *testing.T) {
	for _, route := range Routes() {
		canonical := string(route)
		for _, in := range []string{canonical, canonical + "Request", canonical + "RequestMsgType"} {
			once := NormalizePath(in)
			twice := NormalizePath(once)
			if twice != once {
				t.Errorf("NormalizePath(NormalizePath(%q)) = %q, want %q", in, twice, once)
			}
			if once != canonical {
				t.Errorf("NormalizePath(%q) = %q, want %q", in, once, canonical)
			}
		}
	}
}

func TestNormalizePath_UnknownUnchanged(t *testing.T) {
	tests := []string{
		"",
		"/",
		"/hq/Unknown",
		"/trade/TradeLoginRequestX",
		"/unknown/FooRequest",
		"/unknown/FooRequestMsgType",
	}
	for _, in := range tests {
		if got := NormalizePath(in); got != in {
			t.Errorf("NormalizePath(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestRoute_ValidateUnknown(t *testing.T) {
	tests := []Route{
		"",
		"/",
		"/hq/Unknown",
		"/trade/TradeLoginRequest",
		"/trade/TradeLoginRequestMsgType",
		"/trade/tradelogin",
	}
	for _, route := range tests {
		err := route.Validate()
		if err == nil {
			t.Errorf("Validate(%q) = nil, want error", route)
			continue
		}
		if !errors.Is(err, ErrUnknownRoute) {
			t.Errorf("Validate(%q) error = %v, want errors.Is ErrUnknownRoute", route, err)
		}
	}
}
