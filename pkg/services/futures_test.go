// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/test/mockgateway"
)

// This file is the regression net for the futures service layer: the operation
// labels, the constructor, the seven request bodies, the request-boundary
// defaults, and the four reply envelopes with the three helpers that assemble
// an envelope into one domain value.
//
// The futures domain layer — the row payloads, the models, the mappers and the
// value wrappers — is tested in pkg/domain/futures_test.go, beside the code it
// covers. What stays here is what this layer owns: a request is built here, an
// envelope is decoded here, and an envelope cannot be decoded into a domain
// value without pairing its parts, so those three assembly helpers are here too.
//
// It has no endpoint method to drive, because the eleven methods arrive with the
// read and mutation work that follows, so every assertion here is made against
// the type or against a marshalled body rather than against a request.
//
// That is not a weaker test than it looks, it is a different one. The mock
// Gateway answers all eleven futures routes by path and never inspects a
// request body, so a call-level futures test would pass with every field name
// misspelled. A struct-level assertion on the marshalled body is the only
// check in this repository that can fail when a JSON key is wrong, and these
// tests are that check for the 24 request keys.
//
// The fixtures are the mock Gateway's own futures payloads, copied verbatim from
// test/mockgateway/fixtures.go. Copying rather than importing them is
// deliberate: importing would make the SDK's wire types and the mock's payloads
// move together, so a rename of a key in one would silently pass. Here a
// mismatch between the two is the failure.
//
// pkg/domain/futures_test.go carries its own copy of the bare row payloads,
// because a row and the envelope that wraps it are declared in different
// packages and neither may import the other's test file. A drift between the two
// copies is caught by both sides: each copy's own test asserts the decoded
// values.

// ---------------------------------------------------------------------------
// Fixtures - the mock Gateway's futures payloads
// ---------------------------------------------------------------------------

const futuresProductInfoBody = `{"prodCode":"HSI","instCode":"FUT","lotSize":50,"decInPrice":0,"contractSize":"50","priceDecimalPoint":"1","expiryDate":"2026-09-29","isSupportT1":0}`

const futuresProductInfoResponseBody = `{"productInfoVos":[` + futuresProductInfoBody + `]}`

const futuresFundInfoBody = `{"assetBalance":"1000000.00","enableBalance":"500000.00","marginCall":"0.00","incomeBalance":"1200.00","cashBal":"300000.00","iMargin":"100000.00","mMargin":"50000.00","marginLevel":"2.50","maxMargin":"200000.00","creditLimit":"0.00","ctrlLevel":"1","marginClass":"1","aeId":"AE001","cashBalHKD":"250000.00","cashBalUSD":"6410.00","cycRateUSDtoHSD":"7.80","marginStatus":"1","statusPercent":"40","closeProfit":"500.00","cashBalCNH":"0.00"}`

// futuresFundInfoWrappedBody is the same snapshot inside the envelope
// /trade/FuturesQueryFundInfo returns it in.
const futuresFundInfoWrappedBody = `{"fundInfo":` + futuresFundInfoBody + `}`

const futuresHoldBody = `{"fundInfo":` + futuresFundInfoBody + `,"holdsList":[{"stockName":"Hang Seng Index Futures Sep26","stockCode":"HSI2609.HK","lastDayQty":"1","lastDayPrice":"24800","depQty":"1","dayLongQty":"0","dayLongPrice":"0","dayShortQty":"0","dayShortPrice":"0","dayNetQty":"0","dayNetPrice":"0","currentQty":"1","costPrice":"25000","lastPrice":"25100","preClosePrice":"24900","profitLoss":"500.00","ccyRate":"1.0","contractValue":"50","profitLossBaseCcy":"500.00","ccy":"HKD","dataType":"10010","closeProfit":"0.00","closeProfitHKD":"0.00"}]}`

const futuresOrderBody = `{"data":[{"stockCode":"HSI2609.HK","stockName":"Hang Seng Index Futures Sep26","businessPrice":"25100","entrustBs":"1","entrustPrice":"25000","entrustAmount":"1","businessAmount":"1","date":"20260921","businessTime":"10:00:00","entrustTime":"09:59:59","queryParamStr":"1","statusDesc":"Filled","status":"8","entrustId":"F20260921001","canBeCanceled":0,"entrustType":"0","entrustTypeNum":"0","isValid":1,"canBeUpdated":0,"validType":"0","validTypeDesc":"Day","orderOptions":0,"validTime":""}],"curPageNo":1,"curPageSize":20,"totalPageNo":1,"lastPage":1}`

const futuresMutationBody = `{"data":"F20260921001"}`

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// futuresWireKeys marshals a wire request and returns its decoded body, so a
// test can assert on the key set rather than on a substring. "absent" and
// "spelled differently" are indistinguishable to strings.Contains, and the
// absent direction is the one that matters for an omitempty.
func futuresWireKeys(t *testing.T, body any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("test bug: marshalling %T: %v", body, err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("test bug: the marshalled %T is not a JSON object: %v", body, err)
	}
	return out
}

// futuresRequireKeys asserts the body carries exactly want, so a key that is
// added by mistake fails here as loudly as one that is dropped.
func futuresRequireKeys(t *testing.T, label string, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	gotKeys := make([]string, 0, len(got))
	for k := range got {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := append([]string(nil), want...)
	sort.Strings(wantKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("%s carries %v, want exactly %v", label, gotKeys, wantKeys)
	}
}

// futuresRequireString asserts a decoded wire value, quoted or not.
func futuresRequireString(t *testing.T, params map[string]json.RawMessage, key, want string) {
	t.Helper()
	raw, ok := params[key]
	if !ok {
		t.Fatalf("the body carries no %q key; it carries %v", key, futuresBodyKeys(params))
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatalf("the %q value is neither a string nor a number: %s", key, raw)
		}
		s = n.String()
	}
	if s != want {
		t.Errorf("%s on the wire = %q, want %q", key, s, want)
	}
}

func futuresBodyKeys(params map[string]json.RawMessage) []string {
	out := make([]string, 0, len(params))
	for k := range params {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// futuresRequireMoney, futuresRequireQuantity, futuresRequirePrice and
// futuresRequireRate compare a mapped value against the literal the mock sent.
// The currency is asserted alongside the amount, because a futures amount
// attributed to the wrong currency is a wrong amount that still adds up.
//
// The same four helpers are declared in pkg/domain/futures_test.go. They cannot
// be shared: a helper in either package's _test.go file is invisible to the
// other, and exporting them from the non-test source to share them would put
// test scaffolding on the public surface.
func futuresRequireMoney(t *testing.T, label string, got domain.Money, want, wantCurrency string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
	if got.Currency() != wantCurrency {
		t.Errorf("%s currency = %q, want %q", label, got.Currency(), wantCurrency)
	}
}

func futuresRequireQuantity(t *testing.T, label string, got domain.Quantity, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func futuresRequirePrice(t *testing.T, label string, got domain.Price, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func futuresRequireRate(t *testing.T, label string, got domain.Rate, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// TestFuturesOpConstantsNameGatewayPaths is the cross-check on the eleven
// operation labels, in both directions.
//
// An op label is the string that appears in every typed error the service
// raises, so a typo in one does not fail loudly — it misattributes every error
// to a route the caller never called. The two ends of this assertion come from
// different sources on purpose: the labels come from the vendor SDK request
// literals and the released pkg/hstong/future, while the path list is the mock
// Gateway's fixture table, which was written from docs/SPEC.md §2.8. Neither
// side can be wrong alone without this failing.
//
// The reverse direction is the one that would otherwise go unnoticed: a
// twelfth futures route appearing in the Gateway with no label here.
func TestFuturesOpConstantsNameGatewayPaths(t *testing.T) {
	ops := []string{
		opFuturesQueryProductInfo,
		opFuturesQueryMaxBuySellAmount,
		opFuturesQueryFundInfo,
		opFuturesQueryHoldsList,
		opFuturesQueryRealEntrustList,
		opFuturesQueryHistoryEntrust,
		opFuturesQueryRealDeliverList,
		opFuturesQueryHistoryDeliver,
		opFuturesEntrust,
		opFuturesCancelEntrust,
		opFuturesModifyEntrust,
	}
	if len(ops) != 11 {
		t.Fatalf("the service declares %d operation labels, want one per futures endpoint (11)", len(ops))
	}

	paths := make(map[string]struct{})
	for _, p := range mockgateway.DefaultFixtures().Paths() {
		paths[p] = struct{}{}
	}

	labelled := make(map[string]string, len(ops))
	for _, op := range ops {
		if !strings.HasPrefix(op, "trade/Futures") {
			t.Errorf("op label %q is not a futures route path", op)
		}
		if prev, dup := labelled[op]; dup {
			t.Errorf("op label %q is declared for two endpoints (%s and another)", op, prev)
		}
		labelled[op] = op
		if _, ok := paths["/"+op]; !ok {
			t.Errorf("op label %q names /%s, which the Gateway does not answer", op, op)
		}
	}
	for p := range paths {
		if !strings.Contains(p, "/trade/Futures") {
			continue
		}
		if _, ok := labelled[strings.TrimPrefix(p, "/")]; !ok {
			t.Errorf("the Gateway answers %s but no op label names it", p)
		}
	}
}

// TestNewFuturesService covers the three constructor states the other three
// services share: the client passed positionally, an option that replaces it,
// and two options where the last wins.
func TestNewFuturesService(t *testing.T) {
	first := &sequencedExecutor{}
	second := &sequencedExecutor{}

	svc := NewFuturesService(first)
	if svc.client != Executor(first) {
		t.Errorf("client = %v, want the client passed to the constructor", svc.client)
	}

	svc = NewFuturesService(first, WithFuturesClient(second))
	if svc.client != Executor(second) {
		t.Error("WithFuturesClient did not replace the constructor's client")
	}

	svc = NewFuturesService(nil, WithFuturesClient(first), WithFuturesClient(second))
	if svc.client != Executor(second) {
		t.Error("two options were applied, but the last one did not win")
	}
}

// ---------------------------------------------------------------------------
// Request wire types
// ---------------------------------------------------------------------------

// TestFuturesRequestWireKeys pins the 24 request keys against the two vendor
// SDKs' request literals, which is the only evidence in this repository that
// can fail when one of them is wrong. Every key is asserted, and so is the
// absence of every key not listed, because an invented key is as wrong as a
// misspelled one: the Gateway drops what it does not recognise, so a stray field
// is a silent no-op rather than a rejection.
func TestFuturesRequestWireKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  any
		keys  []string
		check func(t *testing.T, params map[string]json.RawMessage)
	}{
		{
			name: "QueryProductInfo",
			body: futuresProductInfoWireRequest{StockCodes: []string{"HSI2609.HK", "01810.HK"}},
			keys: []string{"stockCode"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				var codes []string
				if err := json.Unmarshal(params["stockCode"], &codes); err != nil {
					t.Fatalf("stockCode is not a string array: %v", err)
				}
				if !reflect.DeepEqual(codes, []string{"HSI2609.HK", "01810.HK"}) {
					t.Errorf("stockCode = %v, want the two codes in order", codes)
				}
			},
		},
		{
			name: "QueryMaxBuySellAmount",
			body: futuresMaxBuySellAmountWireRequest{StockCode: "HSI2609.HK"},
			keys: []string{"stockCode"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				futuresRequireString(t, params, "stockCode", "HSI2609.HK")
			},
		},
		{
			name: "history page",
			body: futuresPageQueryWireRequest{
				PageNo:    2,
				PageSize:  50,
				StartDate: "20260901",
				EndDate:   "20260921",
			},
			keys: []string{"endDate", "pageNo", "pageSize", "startDate"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				var pageNo, pageSize int
				if err := json.Unmarshal(params["pageNo"], &pageNo); err != nil {
					t.Fatalf("pageNo is not a number: %v", err)
				}
				if err := json.Unmarshal(params["pageSize"], &pageSize); err != nil {
					t.Fatalf("pageSize is not a number: %v", err)
				}
				// Both are unquoted numbers: the vendor types them as boxed
				// Integer, and they are page counters, not money.
				if pageNo != 2 || pageSize != 50 {
					t.Errorf("pageNo/pageSize = %d/%d, want 2/50", pageNo, pageSize)
				}
				futuresRequireString(t, params, "startDate", "20260901")
				futuresRequireString(t, params, "endDate", "20260921")
			},
		},
		{
			name: "Entrust",
			body: futuresEntrustWireRequest{
				StockCode:     "HSI2609.HK",
				EntrustType:   "0",
				EntrustPrice:  "25000",
				EntrustAmount: "1",
				EntrustBS:     "1",
				ValidTimeType: "0",
				OrderOptions:  "0",
				ValidTime:     "20260930",
			},
			keys: []string{
				"entrustAmount", "entrustBs", "entrustPrice", "entrustType",
				"orderOptions", "stockCode", "validTime", "validTimeType",
			},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				for key, want := range map[string]string{
					"stockCode":     "HSI2609.HK",
					"entrustType":   "0",
					"entrustPrice":  "25000",
					"entrustAmount": "1",
					"entrustBs":     "1",
					"validTimeType": "0",
					"orderOptions":  "0",
					"validTime":     "20260930",
				} {
					futuresRequireString(t, params, key, want)
				}
			},
		},
		{
			name: "CancelEntrust",
			body: futuresCancelEntrustWireRequest{EntrustID: "F20260921001", StockCode: "HSI2609.HK"},
			keys: []string{"entrustId", "stockCode"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				futuresRequireString(t, params, "entrustId", "F20260921001")
				futuresRequireString(t, params, "stockCode", "HSI2609.HK")
			},
		},
		{
			name: "ModifyEntrust",
			body: futuresModifyEntrustWireRequest{
				EntrustID:     "F20260921001",
				StockCode:     "HSI2609.HK",
				EntrustPrice:  "25100",
				EntrustAmount: "2",
				EntrustBS:     "1",
				ValidTimeType: "0",
				OrderOptions:  "0",
				ValidTime:     "20260930",
			},
			keys: []string{
				"entrustAmount", "entrustBs", "entrustId", "entrustPrice",
				"orderOptions", "stockCode", "validTime", "validTimeType",
			},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				futuresRequireString(t, params, "entrustId", "F20260921001")
				futuresRequireString(t, params, "stockCode", "HSI2609.HK")
				futuresRequireString(t, params, "entrustPrice", "25100")
				futuresRequireString(t, params, "entrustAmount", "2")
				futuresRequireString(t, params, "entrustBs", "1")
				futuresRequireString(t, params, "validTimeType", "0")
				futuresRequireString(t, params, "orderOptions", "0")
				futuresRequireString(t, params, "validTime", "20260930")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := futuresWireKeys(t, tc.body)
			futuresRequireKeys(t, tc.name+" body", params, tc.keys...)
			tc.check(t, params)
		})
	}
}

// TestFuturesModifyRequestHasNoEntrustType is the mirror-image row of the key
// table: the field the body must NOT have.
//
// Both vendors omit entrustType from a modify and include it on an entrust, and
// a symmetry argument that mirrored the entrust body would add a field the
// Gateway does not read — on the operation whose entire argument is that it
// retypes nothing.
func TestFuturesModifyRequestHasNoEntrustType(t *testing.T) {
	modify := futuresWireKeys(t, futuresModifyEntrustWireRequest{
		EntrustID:     "F20260921001",
		StockCode:     "HSI2609.HK",
		EntrustPrice:  "25100",
		EntrustAmount: "2",
		EntrustBS:     "1",
		ValidTimeType: "0",
		OrderOptions:  "0",
	})
	if _, ok := modify["entrustType"]; ok {
		t.Error("the modify body carries an entrustType key; the Gateway's modify " +
			"body has no such field and a modify must not retype the order")
	}
	entrust := futuresWireKeys(t, futuresEntrustWireRequest{EntrustType: "0"})
	if _, ok := entrust["entrustType"]; !ok {
		t.Error("the entrust body has no entrustType key, so the mirror-image " +
			"omission the modify body asserts cannot be the real difference")
	}
}

// TestFuturesRequestOmitemptyInBothDirections proves the omitempty rule from
// design-futures-requests.md §6.1 on the wire, in the direction that matters: a
// field that is merely empty is still sent, and only a field whose absence is a
// legitimate wire state is dropped.
//
// The present-but-empty rows are also the observable difference from the
// released pkg/hstong/future bodies, which tag entrustType, entrustPrice,
// validTimeType, orderOptions, pageNo and pageSize omitempty. The bytes are
// identical for a well-formed request, so a future editor who "fixes" this back
// to match the released layer would remove only this coverage.
func TestFuturesRequestOmitemptyInBothDirections(t *testing.T) {
	t.Run("validTime is dropped when the time-in-force is not a specified date", func(t *testing.T) {
		params := futuresWireKeys(t, futuresEntrustWireRequest{
			StockCode:    "HSI2609.HK",
			EntrustType:  "2",
			EntrustPrice: "0",
			OrderOptions: "0",
		})
		if _, ok := params["validTime"]; ok {
			t.Error("the validTime key is present with no date; it is the one " +
				"conditional field on the entrust body and must be omitted")
		}
		futuresRequireKeys(t, "entrust body", params,
			"entrustAmount", "entrustBs", "entrustPrice", "entrustType",
			"orderOptions", "stockCode", "validTimeType")
	})

	t.Run("history dates are dropped when unbounded", func(t *testing.T) {
		params := futuresWireKeys(t, futuresPageQueryWireRequest{PageNo: 1, PageSize: 20})
		if _, ok := params["startDate"]; ok {
			t.Error("the startDate key is present on an unbounded history query; " +
				"an absent range is a legitimate state the released layer sends")
		}
		if _, ok := params["endDate"]; ok {
			t.Error("the endDate key is present on an unbounded history query")
		}
		futuresRequireKeys(t, "history body", params, "pageNo", "pageSize")
	})

	t.Run("an empty required field is sent empty, not dropped", func(t *testing.T) {
		params := futuresWireKeys(t, futuresEntrustWireRequest{})
		for _, key := range []string{
			"stockCode", "entrustType", "entrustPrice", "entrustAmount",
			"entrustBs", "validTimeType", "orderOptions",
		} {
			raw, ok := params[key]
			if !ok {
				t.Errorf("the %q key is absent from an all-zero entrust body; it is "+
					"required, so it must be present and empty rather than omitted", key)
				continue
			}
			if string(raw) != `""` {
				t.Errorf("the %q key = %s, want an empty string", key, raw)
			}
		}
	})

	t.Run("a zero page counter is sent zero, not dropped", func(t *testing.T) {
		// The mapping boundary substitutes 1 and 20 before the marshal, so a
		// zero only reaches the body if the substitution was skipped. Dropping
		// the key instead would make the request depend on a pre-substitution
		// value, which is the reason this struct has no omitempty at all.
		params := futuresWireKeys(t, futuresPageQueryWireRequest{})
		for _, key := range []string{"pageNo", "pageSize"} {
			raw, ok := params[key]
			if !ok {
				t.Errorf("the %q key is absent; a defaulted page counter is still a "+
					"required field and must be present", key)
				continue
			}
			if string(raw) != "0" {
				t.Errorf("the %q key = %s, want the unquoted number 0", key, raw)
			}
		}
	})
}

// TestFuturesRequestsCarryNoMarketOrCursorField is the structural guard for the
// two rules that are most often broken by a well-meaning "consistency" edit: no
// futures request carries a market, and no futures request carries a cursor.
//
// A market field would be a wire change on a guess, invisible to the mock; a
// cursor field would be copied from the cash layer, which pages by cursor and
// futures does not.
func TestFuturesRequestsCarryNoMarketOrCursorField(t *testing.T) {
	forbidden := map[string]string{
		"exchangeType": "a futures request carries no market; the contract code is unique " +
			"across the HK and US books and the reply reads the book back in dataType",
		"dataType": "the same rule as exchangeType, and the field that would be added by " +
			"copying the cash body",
		"exchange":   "no futures body has a venue field",
		"queryCount": "the cash layer pages with queryCount; futures pages with pageNo/pageSize",
		"queryParamStr": "futures pagination is page-numbered and has no cursor; the only " +
			"queryParamStr in this file is a response field",
		"cursor":    "the futures Gateway takes no cursor",
		"page_size": "the futures wire keys are pageNo and pageSize",
	}
	for _, body := range []any{
		futuresProductInfoWireRequest{},
		futuresMaxBuySellAmountWireRequest{},
		futuresPageQueryWireRequest{},
		futuresEntrustWireRequest{},
		futuresCancelEntrustWireRequest{},
		futuresModifyEntrustWireRequest{},
	} {
		rt := reflect.TypeOf(body)
		for i := range rt.NumField() {
			key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
			if why, bad := forbidden[key]; bad {
				t.Errorf("%s.%s carries the key %q: %s",
					rt.Name(), rt.Field(i).Name, key, why)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Mapper defaults at the request boundary
// ---------------------------------------------------------------------------

// TestFuturesPageQueryParamsDefaults covers the two substitutions that belong at
// the mapping boundary: a non-positive page number becomes 1 and a non-positive
// page size becomes 20. Both are the Gateway's documented defaults, and both are
// the released pkg/hstong/future defaults, so a caller migrating from that
// surface sees the same first page.
func TestFuturesPageQueryParamsDefaults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		page     PageRequest
		wantNo   int
		wantSize int
	}{
		{"zero value", PageRequest{}, futuresDefaultPageNo, futuresDefaultPageSize},
		{"negative", PageRequest{PageNo: -3, PageSize: -1}, futuresDefaultPageNo, futuresDefaultPageSize},
		{"explicit", PageRequest{PageNo: 4, PageSize: 99}, 4, 99},
		{"page only", PageRequest{PageNo: 7}, 7, futuresDefaultPageSize},
		{"size only", PageRequest{PageSize: 5}, futuresDefaultPageNo, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := futuresPageQueryParams(tc.page)
			if got.PageNo != tc.wantNo {
				t.Errorf("pageNo = %d, want %d", got.PageNo, tc.wantNo)
			}
			if got.PageSize != tc.wantSize {
				t.Errorf("pageSize = %d, want %d", got.PageSize, tc.wantSize)
			}
		})
	}
}

// TestFuturesPageQueryParamsCarriesDatesVerbatim checks the optional half of the
// body: the dates cross unchanged, in the request's yyyyMMdd form, and an
// unbounded query simply leaves them empty for the omitempty to drop.
func TestFuturesPageQueryParamsCarriesDatesVerbatim(t *testing.T) {
	got := futuresPageQueryParams(PageRequest{StartDate: "20260901", EndDate: "20260921"})
	if got.StartDate != "20260901" || got.EndDate != "20260921" {
		t.Errorf("dates = %q/%q, want 20260901/20260921", got.StartDate, got.EndDate)
	}
	if empty := futuresPageQueryParams(PageRequest{}); empty.StartDate != "" || empty.EndDate != "" {
		t.Errorf("an unbounded page carries %q/%q, want both empty", empty.StartDate, empty.EndDate)
	}
}

// TestFuturesOrderOptionsParamDefault is the one caller-supplied default on a
// mutation: the field has no omitempty, so an unsupplied order option would put
// "" on the wire, which is a worse default than the documented "0".
func TestFuturesOrderOptionsParamDefault(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", futuresDefaultOrderOptions},
		{"0", "0"},
		{"1", "1"},
	} {
		if got := futuresOrderOptionsParam(tc.in); got != tc.want {
			t.Errorf("futuresOrderOptionsParam(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Reply envelopes
// ---------------------------------------------------------------------------

// TestFuturesReplyEnvelopesDecodeTheMockPayloads is the decode check for the
// four envelopes, and it is a service-layer check because the envelope is a
// service-layer declaration: the row types it holds are exercised in
// pkg/domain/futures_test.go, and what can go wrong here is a key on the
// wrapper rather than a key on the row.
//
// The row counts are asserted because each is the one the fixture carries and
// a wrapper that decoded the list under a different key would otherwise decode
// an empty reply and pass every per-row assertion with nothing to assert.
func TestFuturesReplyEnvelopesDecodeTheMockPayloads(t *testing.T) {
	t.Run("QueryProductInfo", func(t *testing.T) {
		var reply futuresProductInfoWireResponse
		if err := json.Unmarshal([]byte(futuresProductInfoResponseBody), &reply); err != nil {
			t.Fatalf("the mock's product reply does not decode: %v", err)
		}
		if len(reply.ProductInfoVos) != 1 {
			t.Fatalf("the fixture carries %d products, want 1", len(reply.ProductInfoVos))
		}
		if reply.ProductInfoVos[0].ProdCode != "HSI" {
			t.Errorf("the row did not survive the wrapper: prodCode = %q, want HSI",
				reply.ProductInfoVos[0].ProdCode)
		}
	})

	t.Run("QueryFundInfo", func(t *testing.T) {
		var reply futuresFundInfoWireResponse
		if err := json.Unmarshal([]byte(futuresFundInfoWrappedBody), &reply); err != nil {
			t.Fatalf("the mock's fund info does not decode: %v", err)
		}
		if reply.FundInfo.AssetBalance != "1000000.00" {
			t.Errorf("the row did not survive the wrapper: assetBalance = %q, want 1000000.00",
				reply.FundInfo.AssetBalance)
		}
		if reply.FundInfo.AEID != "AE001" {
			t.Errorf("the row did not survive the wrapper: aeId = %q, want AE001", reply.FundInfo.AEID)
		}
	})

	t.Run("QueryHoldsList", func(t *testing.T) {
		var reply futuresHoldsListWireResponse
		if err := json.Unmarshal([]byte(futuresHoldBody), &reply); err != nil {
			t.Fatalf("the mock's holds reply does not decode: %v", err)
		}
		if reply.FundInfo.AEID != "AE001" {
			t.Errorf("fundInfo did not decode: aeId = %q, want AE001", reply.FundInfo.AEID)
		}
		if len(reply.HoldsList) != 1 {
			t.Fatalf("the fixture carries %d positions, want 1", len(reply.HoldsList))
		}
		if reply.HoldsList[0].StockCode != "HSI2609.HK" {
			t.Errorf("the row did not survive the wrapper: stockCode = %q, want HSI2609.HK",
				reply.HoldsList[0].StockCode)
		}
	})

	t.Run("the four list routes", func(t *testing.T) {
		var reply futuresOrderListWireResponse
		if err := json.Unmarshal([]byte(futuresOrderBody), &reply); err != nil {
			t.Fatalf("the mock's order reply does not decode: %v", err)
		}
		if len(reply.Data) != 1 {
			t.Fatalf("the fixture carries %d rows, want 1", len(reply.Data))
		}
		if reply.Data[0].EntrustID != "F20260921001" {
			t.Errorf("the row did not survive the wrapper: entrustId = %q, want F20260921001",
				reply.Data[0].EntrustID)
		}
		if reply.CurPageNo != 1 || reply.CurPageSize != 20 || reply.TotalPageNo != 1 || reply.LastPage != 1 {
			t.Errorf("page state did not decode: %d/%d/%d/%d, want 1/20/1/1",
				reply.CurPageNo, reply.CurPageSize, reply.TotalPageNo, reply.LastPage)
		}
	})
}

// TestFuturesHoldResultFromDTOTheMockFixture maps the two-part position reply:
// the funds snapshot and the position, neither derived from the other.
//
// The field-by-field mapping of each part is asserted in
// pkg/domain/futures_test.go, where the mappers live. What is asserted here is
// what only the assembly can be wrong about: that both parts survive the
// envelope, that neither is derived from the other, and that the account is not
// silently dropped.
func TestFuturesHoldResultFromDTOTheMockFixture(t *testing.T) {
	var reply futuresHoldsListWireResponse
	if err := json.Unmarshal([]byte(futuresHoldBody), &reply); err != nil {
		t.Fatalf("the mock's holds reply does not decode: %v", err)
	}

	got := futuresHoldResultFromDTO(&reply)
	if got == nil {
		t.Fatal("futuresHoldResultFromDTO returned nil for a present reply")
	}
	if got.Account == nil {
		t.Fatal("the funds snapshot was dropped; the reply is one atomic read")
	}
	futuresRequireMoney(t, "account.assetBalance", got.Account.AssetBalance, "1000000", "HKD")

	if len(got.Positions) != 1 {
		t.Fatalf("positions = %d, want 1", len(got.Positions))
	}
	pos := got.Positions[0]
	if pos == nil {
		t.Fatal("a present position row mapped to nil")
	}
	if pos.StockCode != "HSI2609.HK" || pos.StockName != "Hang Seng Index Futures Sep26" {
		t.Errorf("stockCode/stockName = %q/%q", pos.StockCode, pos.StockName)
	}
	if pos.DataType != "10010" {
		t.Errorf("dataType = %q, want 10010 (the field that says the book without a request field)", pos.DataType)
	}
	if pos.Ccy != "HKD" {
		t.Errorf("ccy = %q, want HKD", pos.Ccy)
	}
	for _, c := range []struct {
		label string
		got   domain.Quantity
		want  string
	}{
		{"lastDayQty", pos.LastDayQty, "1"},
		{"depQty", pos.DepQty, "1"},
		{"dayLongQty", pos.DayLongQty, "0"},
		{"dayShortQty", pos.DayShortQty, "0"},
		{"dayNetQty", pos.DayNetQty, "0"},
		{"currentQty", pos.CurrentQty, "1"},
		{"contractValue", pos.ContractValue, "50"},
	} {
		futuresRequireQuantity(t, c.label, c.got, c.want)
	}
	for _, c := range []struct {
		label string
		got   domain.Price
		want  string
	}{
		{"lastDayPrice", pos.LastDayPrice, "24800"},
		{"dayLongPrice", pos.DayLongPrice, "0"},
		{"dayShortPrice", pos.DayShortPrice, "0"},
		{"dayNetPrice", pos.DayNetPrice, "0"},
		{"costPrice", pos.CostPrice, "25000"},
		{"lastPrice", pos.LastPrice, "25100"},
		{"preClosePrice", pos.PreClosePrice, "24900"},
	} {
		futuresRequirePrice(t, c.label, c.got, c.want)
	}
	futuresRequireRate(t, "ccyRate", pos.CcyRate, "1")
	futuresRequireMoney(t, "profitLoss", pos.ProfitLoss, "500", "HKD")
	futuresRequireMoney(t, "profitLossBaseCcy", pos.ProfitLossBaseCcy, "500", "HKD")
	futuresRequireMoney(t, "closeProfit", pos.CloseProfit, "0", "HKD")
	futuresRequireMoney(t, "closeProfitHKD", pos.CloseProfitHKD, "0", "HKD")
}

// TestFuturesOrderAndFillPages checks the page state, which is the only reason
// the two history methods return a page and the two real methods return a slice.
//
// This is the case that is worth reading twice: the page counters and the rows
// arrive in the same envelope, so the page is assembled here and not by a
// mapper in pkg/domain. The two mappers it calls take one row each, which is
// why the layering holds.
func TestFuturesOrderAndFillPages(t *testing.T) {
	var reply futuresOrderListWireResponse
	if err := json.Unmarshal([]byte(futuresOrderBody), &reply); err != nil {
		t.Fatalf("the mock's order reply does not decode: %v", err)
	}

	page := futuresOrderPageFromDTO(&reply)
	if page == nil {
		t.Fatal("futuresOrderPageFromDTO returned nil for a present reply")
	}
	if len(page.Orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(page.Orders))
	}
	if page.CurPageNo != 1 || page.CurPageSize != 20 || page.TotalPageNo != 1 {
		t.Errorf("page state = %d/%d/%d, want 1/20/1",
			page.CurPageNo, page.CurPageSize, page.TotalPageNo)
	}
	if !page.LastPage {
		t.Error("lastPage = false, want true for the wire's 1")
	}

	fills := futuresFillPageFromDTO(&reply)
	if fills == nil {
		t.Fatal("futuresFillPageFromDTO returned nil for a present reply")
	}
	if len(fills.Fills) != 1 {
		t.Fatalf("fills = %d, want 1", len(fills.Fills))
	}
	if fills.CurPageNo != page.CurPageNo || !fills.LastPage {
		t.Errorf("the fill page state = %d/%v, want the same as the order page",
			fills.CurPageNo, fills.LastPage)
	}
}

// TestFuturesRealListPagesAreEmpty is the boundary the two real-list methods
// rely on: a real (unpaginated) query reports its page fields as zero, so a
// page built from it must say it is on no page rather than inventing one.
func TestFuturesRealListPagesAreEmpty(t *testing.T) {
	const realList = `{"data":[],"curPageNo":0,"curPageSize":0,"totalPageNo":0,"lastPage":0}`
	var reply futuresOrderListWireResponse
	if err := json.Unmarshal([]byte(realList), &reply); err != nil {
		t.Fatalf("the real-list reply does not decode: %v", err)
	}
	page := futuresOrderPageFromDTO(&reply)
	if page.Orders == nil {
		t.Error("orders = nil, want the non-nil empty slice so a caller ranging " +
			"over it behaves the same as for a populated page")
	}
	if len(page.Orders) != 0 {
		t.Errorf("orders = %d, want 0", len(page.Orders))
	}
	if page.LastPage {
		t.Error("lastPage = true, want false for the wire's 0")
	}
}

// TestFuturesMutationResponseDecodes covers the shared mutation body. The field
// is the resulting order number and the Gateway may omit it, so the type is an
// alias of the cash commonStringResponse rather than a second identical struct.
func TestFuturesMutationResponseDecodes(t *testing.T) {
	var out futuresMutationWireResponse
	if err := json.Unmarshal([]byte(futuresMutationBody), &out); err != nil {
		t.Fatalf("the mutation reply does not decode: %v", err)
	}
	if out.Data != "F20260921001" {
		t.Errorf("data = %q, want F20260921001", out.Data)
	}
	if err := json.Unmarshal([]byte(`{"data":""}`), &out); err != nil {
		t.Fatalf("an omitted order number does not decode: %v", err)
	}
	if out.Data != "" {
		t.Errorf("data = %q, want the empty string the Gateway may send", out.Data)
	}
}

// ---------------------------------------------------------------------------
// Nil and empty replies
// ---------------------------------------------------------------------------

// TestFuturesAssemblyHelpersRejectNil is the nil guard the three envelope
// assembly helpers carry. The endpoint methods hand each helper a decoded reply
// unconditionally, so a nil must be reachable and safe.
//
// The nil guard on the six per-row mappers is asserted in
// pkg/domain/futures_test.go, beside them.
func TestFuturesAssemblyHelpersRejectNil(t *testing.T) {
	if got := futuresHoldResultFromDTO(nil); got != nil {
		t.Errorf("futuresHoldResultFromDTO(nil) = %v, want nil", got)
	}
	if got := futuresOrderPageFromDTO(nil); got != nil {
		t.Errorf("futuresOrderPageFromDTO(nil) = %v, want nil", got)
	}
	if got := futuresFillPageFromDTO(nil); got != nil {
		t.Errorf("futuresFillPageFromDTO(nil) = %v, want nil", got)
	}
}

// TestFuturesAssemblyTolerAnAbsentList is the absent-list half of the envelope
// contract: a reply whose lists are absent maps to the non-nil empty slices the
// endpoint methods return, and a reply with no funds block still yields an
// account, because the funds snapshot is not optional.
func TestFuturesAssemblyTolerAnAbsentList(t *testing.T) {
	result := futuresHoldResultFromDTO(&futuresHoldsListWireResponse{})
	if result.Positions == nil {
		t.Error("positions = nil for an absent list, want the non-nil empty slice")
	}
	if result.Account == nil {
		t.Error("account = nil for an absent snapshot; the funds block is not optional")
	}
	if page := futuresOrderPageFromDTO(&futuresOrderListWireResponse{}); page.Orders == nil {
		t.Error("a page built from an absent list has a nil slice")
	}
	if page := futuresFillPageFromDTO(&futuresOrderListWireResponse{}); page.Fills == nil {
		t.Error("a fill page built from an absent list has a nil slice")
	}
}
