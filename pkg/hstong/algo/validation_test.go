// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo

import "testing"

// validChangeOrder is a ChangeOrderParams that passes every validation branch.
func validChangeOrder() ChangeOrderParams {
	return ChangeOrderParams{
		OrderID:       "ORDER-1",
		StockCode:     "00700.HK",
		ExchangeType:  "K",
		EntrustPrice:  "100.5",
		EntrustAmount: "100",
	}
}

// TestChangeOrderParamsValidateBranches walks each required-field and format
// branch. The happy path alone left most of validate uncovered, so a regression
// that dropped one check would not have been caught.
func TestChangeOrderParamsValidateBranches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ChangeOrderParams)
	}{
		{"orderId required", func(p *ChangeOrderParams) { p.OrderID = "" }},
		{"stockCode required", func(p *ChangeOrderParams) { p.StockCode = "" }},
		{"exchangeType required", func(p *ChangeOrderParams) { p.ExchangeType = "" }},
		{"entrustPrice required", func(p *ChangeOrderParams) { p.EntrustPrice = "" }},
		{"entrustPrice must be a positive decimal", func(p *ChangeOrderParams) { p.EntrustPrice = "abc" }},
		{"entrustAmount required", func(p *ChangeOrderParams) { p.EntrustAmount = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validChangeOrder()
			tt.mutate(&p)
			if err := p.validate(opChangeOrder); err == nil {
				t.Errorf("validate() = nil, want an invalid-parameter error for %s", tt.name)
			}
		})
	}

	if err := validChangeOrder().validate(opChangeOrder); err != nil {
		t.Errorf("validate() on a well-formed request = %v, want nil", err)
	}
}

// validQueryOrderList is a QueryOrderListParams that passes validation.
func validQueryOrderList() QueryOrderListParams {
	return QueryOrderListParams{
		PageNo:    "1",
		PageSize:  "20",
		StartDate: "20260101",
		EndDate:   "20260131",
	}
}

// TestQueryOrderListParamsValidateBranches covers the paging and date branches,
// which are the guards against a malformed cursor walk.
func TestQueryOrderListParamsValidateBranches(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*QueryOrderListParams)
	}{
		{"pageNo required", func(p *QueryOrderListParams) { p.PageNo = "" }},
		{"pageNo must be a positive integer", func(p *QueryOrderListParams) { p.PageNo = "0" }},
		{"pageSize required", func(p *QueryOrderListParams) { p.PageSize = "" }},
		{"pageSize must be a positive integer", func(p *QueryOrderListParams) { p.PageSize = "x" }},
		{"startDate required", func(p *QueryOrderListParams) { p.StartDate = "" }},
		{"startDate must be yyyyMMdd", func(p *QueryOrderListParams) { p.StartDate = "2026-01-01" }},
		{"endDate required", func(p *QueryOrderListParams) { p.EndDate = "" }},
		{"endDate must be yyyyMMdd", func(p *QueryOrderListParams) { p.EndDate = "2026010" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validQueryOrderList()
			tt.mutate(&p)
			if err := p.validate(opQueryOrderList); err == nil {
				t.Errorf("validate() = nil, want an invalid-parameter error for %s", tt.name)
			}
		})
	}

	if err := validQueryOrderList().validate(opQueryOrderList); err != nil {
		t.Errorf("validate() on a well-formed request = %v, want nil", err)
	}
}

// TestIsDateIsShapeOnly documents the current contract rather than an ideal one.
// isDate checks that the value is exactly eight digits; it does not parse a
// calendar, so an impossible date such as month 13 is accepted here and rejected
// later by the Gateway. Tightening this would start rejecting requests the SDK
// currently forwards, which is a released-behaviour change and therefore out of
// scope for a test-only pass. If the Gateway turns out to accept an impossible
// date, this is the place to tighten.
func TestIsDateIsShapeOnly(t *testing.T) {
	accepted := []string{"20260101", "20261301", "20260000", "00000000"}
	for _, s := range accepted {
		if !isDate(s) {
			t.Errorf("isDate(%q) = false, want true (shape-only check)", s)
		}
	}

	rejected := []string{"", "2026010", "202601011", "2026-01-01", "2026010a", "abcdefgh"}
	for _, s := range rejected {
		if isDate(s) {
			t.Errorf("isDate(%q) = true, want false", s)
		}
	}
}
