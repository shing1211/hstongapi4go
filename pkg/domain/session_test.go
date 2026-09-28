// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"testing"
)

// sessionBoolPtr is the one helper this file needs, and it is unexported and
// session-prefixed so it cannot collide with a helper another domain test
// file adds later. Every test in this package is in package domain, so a
// duplicate declaration would be a compile error that takes the whole package
// down with it.

// sessionBoolPtr returns a pointer to b, which is how a TradeSessionWire
// distinguishes a reply that carried a success field from one that did not.
func sessionBoolPtr(b bool) *bool { return &b }

// decodeTradeSessionWire decodes a real JSON reply into the wire type, so the
// two dialects below are decoded by encoding/json rather than hand-built. A
// hand-built value would not exercise the *bool decoding rule, which is the one
// thing this file exists to pin.
func decodeTradeSessionWire(t *testing.T, body string) (*TradeSessionWire, error) {
	t.Helper()
	var v TradeSessionWire
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decoding %s: %v", body, err)
	}
	return &v, nil
}

// TestTradeSessionAcceptedFromDTO pins the four replies the mapper decides
// between, and the two of them are the whole reason Success is a *bool.
//
// The rows that matter most are the two that must not be merged: an explicit
// {"success": false} and a reply carrying no success field at all. A plain bool
// cannot tell them apart, which is why the field is a pointer, and a build that
// merged them would report a *successful* login as a rejection - a wrong answer
// that a caller has no way to diagnose from the error.
func TestTradeSessionAcceptedFromDTO(t *testing.T) {
	tests := []struct {
		name string
		wire *TradeSessionWire
		want bool
	}{
		{"nil reply", nil, false},
		{"empty object", &TradeSessionWire{}, false},
		{"success true", &TradeSessionWire{Success: sessionBoolPtr(true)}, true},
		{"success false", &TradeSessionWire{Success: sessionBoolPtr(false)}, false},
		{"explicit refusal beats a data string", &TradeSessionWire{Data: "tok", Success: sessionBoolPtr(false)}, false},
		{"data with no success field", &TradeSessionWire{Data: "tok"}, true},
		{"success false with no data", &TradeSessionWire{Success: sessionBoolPtr(false)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TradeSessionAcceptedFromDTO(tt.wire); got != tt.want {
				t.Errorf("TradeSessionAcceptedFromDTO(%+v) = %v, want %v", tt.wire, got, tt.want)
			}
		})
	}
}

// TestTradeSessionAcceptedFromDTOReadsBothDialects states the two reply dialects
// this repository's sources disagree about, so a future reader does not have to
// re-derive them from the header comment and can see that both are handled.
//
// The evidence, restated as executable claims:
//
//   - The released pkg/hstong/session.go and the mock Gateway's fixture both
//     send {"success": true} for the two session endpoints, so the boolean row
//     is the parity anchor and must be accepted.
//   - The Java vendor deserializes the login reply as CommonStringVo, which is
//     {"data": "..."}, and only its logout uses CommonBoolVo. A build reading
//     only the boolean would report that login as a rejection, which is the
//     failure direction the decision on TradeSessionWire rejects.
func TestTradeSessionAcceptedFromDTOReadsBothDialects(t *testing.T) {
	releasedDialect, _ := decodeTradeSessionWire(t, `{"success":true}`)
	if !TradeSessionAcceptedFromDTO(releasedDialect) {
		t.Error(`{"success":true} - the released layer's and the mock Gateway's shape - ` +
			"was read as a rejection; a working login would report itself as refused")
	}
	vendorDialect, _ := decodeTradeSessionWire(t, `{"data":"session-token-abc"}`)
	if !TradeSessionAcceptedFromDTO(vendorDialect) {
		t.Error(`{"data":"..."} - the Java vendor's CommonStringVo shape - was read as a ` +
			"rejection; the disagreement between the two sources would decide against the SDK")
	}
	if vendorDialect.Data != "session-token-abc" {
		t.Errorf("Data = %q, want the string the reply carried", vendorDialect.Data)
	}
	if vendorDialect.Success != nil {
		t.Errorf("Success = %v, want nil: a reply with no success field must not decode to a "+
			"false that is indistinguishable from an explicit refusal", *vendorDialect.Success)
	}
	refused, _ := decodeTradeSessionWire(t, `{"success":false}`)
	if refused.Success == nil {
		t.Error(`{"success":false} decoded to a nil Success, so it cannot be told apart from ` +
			`{"data":"..."} - which is the whole reason the field is a pointer`)
	}
}

// TestTradeSessionPredicatesAreBoundaryInclusive pins that both predicates
// trigger *at* their instant and not one second later. The >= rather than > is
// the whole contract: a session that is exactly at its expiry is expired, and a
// session that is exactly at its refresh point is due, because the alternative
// is a session that is unusable for one second before the SDK notices.
func TestTradeSessionPredicatesAreBoundaryInclusive(t *testing.T) {
	sess := TradeSession{ExpiresAt: 2_000, RefreshAt: 1_000}
	tests := []struct {
		name        string
		now         int64
		expired     bool
		shouldFresh bool
	}{
		{"before the refresh point", 999, false, false},
		{"exactly at the refresh point", 1_000, false, true},
		{"inside the refresh window", 1_500, false, true},
		{"one second before expiry", 1_999, false, true},
		{"exactly at expiry", 2_000, true, true},
		{"past expiry", 9_999, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sess.IsExpired(tt.now); got != tt.expired {
				t.Errorf("IsExpired(%d) = %v, want %v", tt.now, got, tt.expired)
			}
			if got := sess.ShouldRefresh(tt.now); got != tt.shouldFresh {
				t.Errorf("ShouldRefresh(%d) = %v, want %v", tt.now, got, tt.shouldFresh)
			}
		})
	}
}

// TestShouldRefreshSubsumesIsExpired pins the property Login's refresh policy
// relies on: because RefreshAt is strictly before ExpiresAt for every session,
// asking only ShouldRefresh is enough to catch an expired one as well.
//
// If this ever fails, a caller that checked only ShouldRefresh would keep a
// session the Gateway has already refused, which is the failure the policy is
// built to avoid.
func TestShouldRefreshSubsumesIsExpired(t *testing.T) {
	for _, expiresAt := range []int64{1, 1_000, 2_000} {
		for refreshAt := int64(0); refreshAt <= expiresAt; refreshAt++ {
			sess := TradeSession{ExpiresAt: expiresAt, RefreshAt: refreshAt}
			for _, now := range []int64{0, 1, refreshAt, expiresAt, expiresAt + 1} {
				if sess.IsExpired(now) && !sess.ShouldRefresh(now) {
					t.Fatalf("expiry=%d refreshAt=%d now=%d: the session is expired but not "+
						"due for refresh, so a caller asking only ShouldRefresh would keep a "+
						"session the Gateway has already refused", expiresAt, refreshAt, now)
				}
			}
		}
	}
}
