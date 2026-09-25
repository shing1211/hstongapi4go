// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"encoding/json"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// validCancel is a CancelEntrustRequest that passes validation.
func validCancel() CancelEntrustRequest {
	return CancelEntrustRequest{
		ExchangeType: types.ExchangeHK,
		StockCode:    "01810.HK",
		EntrustID:    "ENTRUST-1",
	}
}

// TestCancelEntrustRequestValidateBranches covers each required field.
func TestCancelEntrustRequestValidateBranches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CancelEntrustRequest)
	}{
		{"exchangeType required", func(r *CancelEntrustRequest) { r.ExchangeType = "" }},
		{"stockCode required", func(r *CancelEntrustRequest) { r.StockCode = "" }},
		{"entrustId required", func(r *CancelEntrustRequest) { r.EntrustID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validCancel()
			tt.mutate(&r)
			assertInvalid(t, r.validate())
		})
	}

	if err := validCancel().validate(); err != nil {
		t.Errorf("validate() on a well-formed request = %v, want nil", err)
	}
}

// validChange is a ChangeEntrustRequest that passes validation.
func validChange() ChangeEntrustRequest {
	return ChangeEntrustRequest{
		ExchangeType:  types.ExchangeHK,
		StockCode:     "01810.HK",
		EntrustAmount: "100",
		EntrustID:     "ENTRUST-1",
	}
}

// TestChangeEntrustRequestValidateBranches covers the plain branches plus the
// conditional-order branches, which only run for entrust types 31-36.
func TestChangeEntrustRequestValidateBranches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ChangeEntrustRequest)
	}{
		{"exchangeType required", func(r *ChangeEntrustRequest) { r.ExchangeType = "" }},
		{"stockCode required", func(r *ChangeEntrustRequest) { r.StockCode = "" }},
		{"entrustAmount must be positive", func(r *ChangeEntrustRequest) { r.EntrustAmount = "0" }},
		{"entrustAmount must be numeric", func(r *ChangeEntrustRequest) { r.EntrustAmount = "lots" }},
		{"entrustId required", func(r *ChangeEntrustRequest) { r.EntrustID = "" }},
		{"conditional order requires condValue", func(r *ChangeEntrustRequest) {
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = ""
		}},
		{"conditional order requires validDays", func(r *ChangeEntrustRequest) {
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = "170"
			r.ValidDays = ""
		}},
		{"conditional order validDays out of range", func(r *ChangeEntrustRequest) {
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = "170"
			r.ValidDays = "101"
		}},
		{"conditional order validDays not an integer", func(r *ChangeEntrustRequest) {
			r.EntrustType = types.EntrustTypeStopLossLimit
			r.CondValue = "170"
			r.ValidDays = "many"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validChange()
			tt.mutate(&r)
			assertInvalid(t, r.validate())
		})
	}

	// The conditional happy path: type set, condValue and a 1..100 validDays all
	// present, so neither conditional guard fires.
	ok := validChange()
	ok.EntrustType = types.EntrustTypeStopLossLimit
	ok.CondValue = "170"
	ok.ValidDays = "5"
	if err := ok.validate(); err != nil {
		t.Errorf("validate() on a well-formed conditional request = %v, want nil", err)
	}
}

// validMaxAvailable is a MaxAvailableAssetRequest that passes validation.
func validMaxAvailable() MaxAvailableAssetRequest {
	return MaxAvailableAssetRequest{
		ExchangeType: types.ExchangeHK,
		StockCode:    "00700.HK",
		EntrustPrice: "100.5",
		EntrustType:  types.EntrustTypeLimit,
	}
}

// TestMaxAvailableAssetRequestValidateBranches covers each required field.
func TestMaxAvailableAssetRequestValidateBranches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MaxAvailableAssetRequest)
	}{
		{"exchangeType required", func(r *MaxAvailableAssetRequest) { r.ExchangeType = "" }},
		{"stockCode required", func(r *MaxAvailableAssetRequest) { r.StockCode = "" }},
		{"entrustPrice required", func(r *MaxAvailableAssetRequest) { r.EntrustPrice = "" }},
		{"entrustType required", func(r *MaxAvailableAssetRequest) { r.EntrustType = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validMaxAvailable()
			tt.mutate(&r)
			assertInvalid(t, r.validate())
		})
	}

	if err := validMaxAvailable().validate(); err != nil {
		t.Errorf("validate() on a well-formed request = %v, want nil", err)
	}
}

// TestHoldsListResponseUnmarshalShapes covers all three wire shapes the Gateway
// uses for a position list. The bare-array and single-object shapes arrive when
// a market holds exactly one position, which is why they need explicit tests.
func TestHoldsListResponseUnmarshalShapes(t *testing.T) {
	const one = `{"stockCode":"00700.HK","stockName":"Tencent","currentAmount":"100","enableAmount":"100","costPrice":"300.5","marketValue":"35000"}`

	tests := []struct {
		name string
		body string
		want int
	}{
		{"object with holdsList", `{"holdsList":[` + one + `]}`, 1},
		{"bare array", `[` + one + `]`, 1},
		{"single inline position", one, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got HoldsListResponse
			if err := json.Unmarshal([]byte(tt.body), &got); err != nil {
				t.Fatalf("Unmarshal(%s) = %v, want nil", tt.body, err)
			}
			if len(got.HoldsList) != tt.want {
				t.Fatalf("HoldsList len = %d, want %d", len(got.HoldsList), tt.want)
			}
			if got.HoldsList[0].StockCode != "00700.HK" {
				t.Errorf("stockCode = %q, want 00700.HK", got.HoldsList[0].StockCode)
			}
			if got.HoldsList[0].CurrentAmount != "100" {
				t.Errorf("currentAmount = %q, want 100", got.HoldsList[0].CurrentAmount)
			}
		})
	}

	// Two entries in the bare-array shape, to prove the list is accumulated
	// rather than overwritten per element.
	var two HoldsListResponse
	if err := json.Unmarshal([]byte("["+one+","+one+"]"), &two); err != nil {
		t.Fatalf("Unmarshal(bare array of two) = %v, want nil", err)
	}
	if len(two.HoldsList) != 2 {
		t.Errorf("HoldsList len = %d, want 2", len(two.HoldsList))
	}
}

// TestHoldsListResponseUnmarshalEmpty covers the empty document, which must
// decode to an empty list rather than a nil-pointer or an error.
func TestHoldsListResponseUnmarshalEmpty(t *testing.T) {
	for _, body := range []string{`{}`, `[]`, `null`} {
		var got HoldsListResponse
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Errorf("Unmarshal(%s) = %v, want nil", body, err)
		}
		if len(got.HoldsList) != 0 {
			t.Errorf("Unmarshal(%s) gave %d positions, want 0", body, len(got.HoldsList))
		}
	}
}

// TestHoldsListResponseUnmarshalMalformed covers the error path: a malformed
// element must surface as a decode error rather than a silently empty list.
func TestHoldsListResponseUnmarshalMalformed(t *testing.T) {
	for _, body := range []string{`{"holdsList":"not-a-list"}`, `["not-an-object"]`, `{"holdsList":[{"currentAmount":1}]}`} {
		var got HoldsListResponse
		if err := json.Unmarshal([]byte(body), &got); err == nil {
			t.Errorf("Unmarshal(%s) = nil, want a decode error", body)
		}
	}
}
