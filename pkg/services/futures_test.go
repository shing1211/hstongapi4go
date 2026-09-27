// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
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

// futuresCapacityBody is the /trade/FuturesQueryMaxBuySellAmount payload, which is
// the data object itself rather than an envelope.
//
// The two counts sit either side of the binary64 ceiling deliberately: 2^53 is
// 9007199254740992, and 9007199254740993 is the first integer above it that
// binary64 cannot represent, so a float anywhere in this path renders it as the
// neighbouring even number. They are also different from each other, so the two
// fields being swapped — the one mistake a mapper of this shape makes — is
// visible rather than symmetric.
const futuresCapacityBody = `{"positionStatus":1,` +
	`"maxBuyAmount":9007199254740993,` +
	`"maxSellAmount":9007199254740992,` +
	`"initialMargin":"123456.789",` +
	`"ccy":"HKD"}`

// futuresRealListEmptyBody is what a real (unpaginated) list reply looks like with
// nothing in it: an empty row array and four zero page counters. It is the reason
// the two real methods return a slice and the two page methods return a page.
const futuresRealListEmptyBody = `{"data":[],"curPageNo":0,"curPageSize":0,"totalPageNo":0,"lastPage":0}`

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
	var reply futuresOrderListWireResponse
	if err := json.Unmarshal([]byte(futuresRealListEmptyBody), &reply); err != nil {
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

// ===========================================================================
// C4 — the eight read endpoints
//
// Everything above this line tests declarations: a struct, a tag, a default, a
// mapper. Everything below it drives the eight methods, and the difference
// matters because of what A2 found one layer down. pkg/hstong/trade had twenty
// endpoints, a fully covered happy path, and not one test that made a Gateway
// call *fail* — its entire error-propagation surface was dark while every
// assertion passed. So the tables below are not a happy-path supplement: the
// error arms, the local rejections and the money hosts are the point.
//
// Four levels, each reaching something the level above cannot:
//
//   - Local rejection: a zero accountID or a bad input is refused before the
//     executor is consulted, so the assertion is *zero* recorded calls. That is
//     what distinguishes a local rejection from a Gateway rejection wearing the
//     same code.
//   - Level 1 is the sequencedExecutor returning a sentinel: the error reaches
//     the caller unchanged, the zero value comes back beside it, one call is
//     recorded.
//   - Level 2 is a real *client.Client over a server answering a real ok:false
//     envelope, asserting the typed *errs.Error the transport actually builds. A
//     fake never builds a typed error, so a service laundering a Gateway
//     rejection into an opaque one passes every Level 1 row and fails here.
//   - The control is a live retry policy: a single HTTP request is also what a
//     client with no policy installed produces, so without the control the
//     one-request assertions would prove nothing. It also proves the eight reads
//     are classified as queries, which is what makes ADR 0003 the C5
//     constraint rather than a C4 one.
//
// No row sleeps: where a retry count is the assertion, the policy installs
// MaxAttempts with a zero backoff, so the count is a property of the retry
// classification rather than of elapsed time. No row recovers: a futures mapper
// that panicked on a partial reply would take the process down, and the
// tolerance tests below assert the opposite.
// ===========================================================================

// futuresFixtureAccount is the account every futures fixture names. The futures
// wire never carries it, which is exactly the property the zero-accountID tests
// below assert.
func futuresFixtureAccount() domain.AccountID { return domain.AccountID("ACC-C4-FUT") }

// futuresFixtureSymbol is the contract the single-contract reads name.
func futuresFixtureSymbol() domain.Symbol {
	return domain.Symbol{Code: "HSI2609.HK", Market: domain.MarketHK}
}

// futuresFixtureCodes is the batch the product query names.
func futuresFixtureCodes() []string { return []string{"HSI2609.HK", "01810.HK"} }

// futuresFixturePage is the page the two paged reads ask for. It is fully
// populated so a row cannot pass by reaching the executor and being rejected
// there, and so a defaulted field is distinguishable from an absent one.
func futuresFixturePage() PageRequest {
	return PageRequest{PageNo: 2, PageSize: 50, StartDate: "20260901", EndDate: "20260921"}
}

// futuresReadCase is one row of every per-method table below: one row per read
// endpoint, carrying the op and route it must reach, the request it must build,
// the reply that satisfies it, and the call itself.
//
// A new read endpoint cannot be added without a row here, and therefore cannot
// be added without a row in the error arms, the local-rejection table and the
// money hosts below. That coupling is the point: A2's twenty dark endpoints were
// dark precisely because the happy path and the failure path had no shared table
// forcing them to be written together.
type futuresReadCase struct {
	name string
	op   string
	// route is the endpoint the method must reach.
	route client.Route
	// wantParams is the request body the method must hand the executor. It is
	// compared with reflect.DeepEqual, so a field added by mistake and a field
	// dropped by mistake both fail.
	wantParams any
	// reply is the data object the sequencedExecutor answers with. It is a
	// json.RawMessage rather than a string because the executor marshals it as a
	// document; a []byte would be marshalled as a base64 JSON string and every
	// row would fail on a decode error.
	reply json.RawMessage
	// noParams marks the four reads that take no request body and must therefore
	// send struct{}{}. It is a field rather than a name match so the test that
	// asserts the empty params object cannot drift from the table, and so the
	// count of four is itself assertable.
	noParams bool
	// invoke performs the call and returns its error. When the call failed it
	// also asserts the zero value beside the error, against the concrete result
	// type, because that is the only way a generic zero check means anything
	// here: a caller must never be handed a partially decoded payload alongside a
	// failure. The check is conditional so the same closure serves the happy-path
	// and wire-shape tables below, where there is no error to accompany.
	invoke func(t *testing.T, ctx context.Context, svc *FuturesService) error
}

// futuresReadCases is the eight reads, in SPEC order.
func futuresReadCases() []futuresReadCase {
	page := futuresFixturePage()
	pageBody := futuresPageQueryParams(page)
	id := futuresFixtureAccount()
	sym := futuresFixtureSymbol()
	codes := futuresFixtureCodes()

	return []futuresReadCase{
		{
			name: "QueryProductInfo", op: opFuturesQueryProductInfo,
			route:      client.RouteTradeFuturesQueryProductInfo,
			wantParams: futuresProductInfoWireRequest{StockCodes: codes},
			reply:      json.RawMessage(futuresProductInfoResponseBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				products, err := svc.QueryProductInfo(ctx, id, codes)
				if err != nil {
					requireZero(t, products)
				}
				return err
			},
		},
		{
			name: "QueryMaxBuySellAmount", op: opFuturesQueryMaxBuySellAmount,
			route:      client.RouteTradeFuturesQueryMaxBuySellAmount,
			wantParams: futuresMaxBuySellAmountWireRequest{StockCode: sym.Code},
			reply:      json.RawMessage(futuresCapacityBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				capacity, err := svc.QueryMaxBuySellAmount(ctx, id, sym)
				if err != nil {
					requireZero(t, capacity)
				}
				return err
			},
		},
		{
			name: "QueryFundInfo", op: opFuturesQueryFundInfo,
			route: client.RouteTradeFuturesQueryFundInfo,
			// struct{}{}, never nil: internal/transport drops a nil params from
			// the envelope entirely, so the key would be absent rather than {}.
			wantParams: struct{}{},
			reply:      json.RawMessage(futuresFundInfoWrappedBody),
			noParams:   true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				account, err := svc.QueryFundInfo(ctx, id)
				if err != nil {
					requireZero(t, account)
				}
				return err
			},
		},
		{
			name: "QueryHoldsList", op: opFuturesQueryHoldsList,
			route:      client.RouteTradeFuturesQueryHoldsList,
			wantParams: struct{}{},
			reply:      json.RawMessage(futuresHoldBody),
			noParams:   true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				result, err := svc.QueryHoldsList(ctx, id)
				if err != nil {
					requireZero(t, result)
				}
				return err
			},
		},
		{
			name: "QueryRealEntrustList", op: opFuturesQueryRealEntrustList,
			route:      client.RouteTradeFuturesQueryRealEntrustList,
			wantParams: struct{}{},
			reply:      json.RawMessage(futuresOrderBody),
			noParams:   true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				orders, err := svc.QueryRealEntrustList(ctx, id)
				if err != nil {
					requireZero(t, orders)
				}
				return err
			},
		},
		{
			// The method is named "Real" and the op and route are the history
			// pair. TestFuturesPageMethodsUseTheHistoryRoutes states why.
			name: "QueryHistoryEntrustPage", op: opFuturesQueryHistoryEntrust,
			route:      client.RouteTradeFuturesQueryHistoryEntrustList,
			wantParams: pageBody,
			reply:      json.RawMessage(futuresOrderBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				result, err := svc.QueryHistoryEntrustPage(ctx, id, page)
				if err != nil {
					requireZero(t, result)
				}
				return err
			},
		},
		{
			name: "QueryRealDeliverList", op: opFuturesQueryRealDeliverList,
			route:      client.RouteTradeFuturesQueryRealDeliverList,
			wantParams: struct{}{},
			reply:      json.RawMessage(futuresOrderBody),
			noParams:   true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				fills, err := svc.QueryRealDeliverList(ctx, id)
				if err != nil {
					requireZero(t, fills)
				}
				return err
			},
		},
		{
			name: "QueryHistoryDeliverPage", op: opFuturesQueryHistoryDeliver,
			route:      client.RouteTradeFuturesQueryHistoryDeliverList,
			wantParams: pageBody,
			reply:      json.RawMessage(futuresOrderBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				result, err := svc.QueryHistoryDeliverPage(ctx, id, page)
				if err != nil {
					requireZero(t, result)
				}
				return err
			},
		},
	}
}

// futuresExpectCall asserts the recorded call reached wantOp and wantRoute.
func futuresExpectCall(t *testing.T, exec *sequencedExecutor, wantOp string, wantRoute client.Route) {
	t.Helper()
	got := exec.lastCall(t)
	if got.op != wantOp {
		t.Errorf("op = %q, want %q", got.op, wantOp)
	}
	if got.route != wantRoute {
		t.Errorf("route = %q, want %q", got.route, wantRoute)
	}
}

// futuresRecordedParams decodes the params member of the request envelope the
// recorder saw for path. It decodes rather than substring-matches because
// "absent" and "spelled differently" are indistinguishable to strings.Contains,
// and the absent direction is the one that matters for an omitempty.
func futuresRecordedParams(t *testing.T, rec *wireRecorder, path string) map[string]json.RawMessage {
	t.Helper()
	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	body := rec.lastBody(t, path)
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("the recorded request body is not the expected envelope: %v", err)
	}
	if len(envelope.Params) == 0 {
		t.Fatalf("the request envelope carries no params member at all: %s", body)
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("the envelope's params member is not a JSON object (%s): %v", envelope.Params, err)
	}
	return params
}

// futuresReplaceRowField rewrites one string field of the first row of a named
// array member of a reply — holdsList[0], data[0], productInfoVos[0] — so a
// money test can target any field of a position, order or product row without a
// second hand-written fixture.
func futuresReplaceRowField(t *testing.T, raw json.RawMessage, arrayKey, key, value string) json.RawMessage {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("test bug: the fixture is not a JSON object: %v", err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(env[arrayKey], &rows); err != nil {
		t.Fatalf("test bug: the %q member is not a JSON array: %v", arrayKey, err)
	}
	if len(rows) == 0 {
		t.Fatalf("test bug: the %q array is empty, so there is no first row to rewrite", arrayKey)
	}
	if _, ok := rows[0][key]; !ok {
		t.Fatalf("test bug: %s[0] has no %q key; it has %v",
			arrayKey, key, futuresBodyKeys(rows[0]))
	}
	rows[0][key] = json.RawMessage(strconv.Quote(value))
	return futuresReassemble(t, env, arrayKey, rows)
}

// futuresReassemble puts a rewritten member back into its envelope.
func futuresReassemble(t *testing.T, env map[string]json.RawMessage, key string, member any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(member)
	if err != nil {
		t.Fatalf("test bug: re-marshalling the rewritten %q member: %v", key, err)
	}
	env[key] = encoded
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("test bug: re-marshalling the rewritten fixture: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Routing, and the positive control for the accountID check
// ---------------------------------------------------------------------------

// TestFuturesReadMethodsReachTheirRouteWithTheirOp is the positive path for all
// eight: the op label and the route are the ones the method names, the request
// body is the one the call built, and exactly one call was recorded.
//
// It is also the control for TestFuturesReadMethodsRejectAZeroAccountID. A
// validator that refused every accountID, or one whose check was accidentally
// inverted, would satisfy the rejection table while making the eight methods
// unusable; the recorded call here is the evidence that validation was really
// skipped rather than merely outvoted.
func TestFuturesReadMethodsReachTheirRouteWithTheirOp(t *testing.T) {
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tc.reply})
			svc := NewFuturesService(exec)

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s with a valid request = %v, want nil", tc.name, err)
			}
			futuresExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)

			if got := exec.lastParams(t); !reflect.DeepEqual(got, tc.wantParams) {
				t.Errorf("the request body = %#v, want %#v", got, tc.wantParams)
			}
		})
	}
}

// TestFuturesReadMethodsRejectAZeroAccountIDWithoutARequest is the fail-closed
// row for all eight.
//
// Zero recorded calls is the assertion that matters: it distinguishes a local
// rejection from a Gateway rejection wearing the same code, and a caller
// reconciling state must be able to trust that a request it never sent produced
// no side effect. accountID is a session key and never crosses the wire, so
// there is no other way for the check to be visible from the outside — the
// request body of a zero-accountID call is identical to a valid one, and only
// the absence of the call distinguishes them.
func TestFuturesReadMethodsRejectAZeroAccountIDWithoutARequest(t *testing.T) {
	zero := domain.AccountID("")
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			svc := NewFuturesService(exec)

			var err error
			switch tc.name {
			case "QueryProductInfo":
				_, err = svc.QueryProductInfo(t.Context(), zero, futuresFixtureCodes())
			case "QueryMaxBuySellAmount":
				_, err = svc.QueryMaxBuySellAmount(t.Context(), zero, futuresFixtureSymbol())
			case "QueryFundInfo":
				_, err = svc.QueryFundInfo(t.Context(), zero)
			case "QueryHoldsList":
				_, err = svc.QueryHoldsList(t.Context(), zero)
			case "QueryRealEntrustList":
				_, err = svc.QueryRealEntrustList(t.Context(), zero)
			case "QueryHistoryEntrustPage":
				_, err = svc.QueryHistoryEntrustPage(t.Context(), zero, futuresFixturePage())
			case "QueryRealDeliverList":
				_, err = svc.QueryRealDeliverList(t.Context(), zero)
			case "QueryHistoryDeliverPage":
				_, err = svc.QueryHistoryDeliverPage(t.Context(), zero, futuresFixturePage())
			default:
				t.Fatalf("no zero-accountID row for %q; futuresReadCases has drifted", tc.name)
			}

			assertInvalidParam(t, err, tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestFuturesAccountIDNeverReachesTheWire is the other direction of the same
// rule, and it is the one that survives a rename: the op label in a local
// rejection is the only place the account appears, and the request body must
// carry no trace of it.
//
// C2a §7.1 calls this out as the mistake a future maintainer makes — adding
// accountID to a futures wire struct "because the parameter is unused" — and
// names the consequence: an account identifier in every futures request, invisible
// on the mock, which answers by path. So the assertion is that the decoded params
// of every read contain no account-shaped key at all.
func TestFuturesAccountIDNeverReachesTheWire(t *testing.T) {
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			exec := newWireExecutor(t, rec)
			svc := NewFuturesService(exec)

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			params := futuresRecordedParams(t, rec, string(tc.route))
			for key, raw := range params {
				lower := strings.ToLower(key)
				for _, forbidden := range []string{"account", "accid", "userid", "clientid", "subaccount"} {
					if strings.Contains(lower, forbidden) {
						t.Errorf("%s sent the key %q (%s): a futures request carries no account "+
							"id, because the account and the book both come from the session "+
							"(design-futures-requests §7.1)", tc.name, key, raw)
					}
				}
			}
			body := rec.lastBody(t, string(tc.route))
			if strings.Contains(body, string(futuresFixtureAccount())) {
				t.Errorf("%s put the account id on the wire: %s", tc.name, body)
			}
		})
	}
}

// TestFuturesPageMethodsUseTheHistoryRoutes pins the op/route pairing of the two
// paged methods, whose *names* disagree with both.
//
// design-futures-requests.md §7.2 point 3 says the page-returning methods are the
// history queries, and §7.2's own signature block then names both of them
// "Real". The code follows the point and not the names, because only the history
// route populates the page counters a page reports — futuresRealListEmptyBody
// above is a real-list reply with all four at zero, and a page built from it would
// be a fiction. This test exists so the naming debt is greppable rather than only
// prose: if a future editor routes one of these at the real list "to match the
// name", this fails, and the comment above the method says the rename — not the
// route — is the correct fix.
func TestFuturesPageMethodsUseTheHistoryRoutes(t *testing.T) {
	for _, tc := range []struct {
		method string
		op     string
		route  client.Route
		realOp string
		real   client.Route
	}{
		{
			method: "QueryHistoryEntrustPage",
			op:     opFuturesQueryHistoryEntrust,
			route:  client.RouteTradeFuturesQueryHistoryEntrustList,
			realOp: opFuturesQueryRealEntrustList,
			real:   client.RouteTradeFuturesQueryRealEntrustList,
		},
		{
			method: "QueryHistoryDeliverPage",
			op:     opFuturesQueryHistoryDeliver,
			route:  client.RouteTradeFuturesQueryHistoryDeliverList,
			realOp: opFuturesQueryRealDeliverList,
			real:   client.RouteTradeFuturesQueryRealDeliverList,
		},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if tc.op == tc.realOp || tc.route == tc.real {
				t.Fatal("test bug: the history and real pairs are indistinguishable")
			}
			if got := string(tc.route); !strings.Contains(got, "History") {
				t.Fatalf("test bug: %q does not look like a history route", got)
			}
			// The two op labels and the two routes must differ, or every typed
			// error from a history page would be attributed to the real list.
			if opFuturesQueryHistoryEntrust == opFuturesQueryHistoryDeliver {
				t.Error("the two history ops are the same string, so an error from one " +
					"would be attributed to the other")
			}
			if client.RouteTradeFuturesQueryHistoryEntrustList == client.RouteTradeFuturesQueryHistoryDeliverList {
				t.Error("the two history routes are the same path, so one reply would be " +
					"decoded as the other")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local rejections that are not about the account
// ---------------------------------------------------------------------------

// futuresLocalCase is one row of the input-validation table. Each asserts a typed
// rejection under the method's own op *and* zero recorded calls, so a row cannot
// pass by reaching the executor and being rejected there.
type futuresLocalCase struct {
	name string
	op   string
	// run performs the call and returns the error.
	run func(t *testing.T, ctx context.Context, svc *FuturesService) error
}

// futuresLocalCases covers the validation a method body owns and C3 deliberately
// left out: the contract codes and the page bound and the dates.
//
// The four no-param reads are absent, and that absence is a statement rather than
// an omission: they have no input beyond accountID, so a row here would pin a
// check the code does not have. accountValidatedCases in account_errors_test.go
// says the same about RateQueryList.
func futuresLocalCases() []futuresLocalCase {
	id := futuresFixtureAccount()

	pageCases := func(op string, paged func(*testing.T, context.Context, *FuturesService, domain.AccountID, PageRequest) error) []futuresLocalCase {
		return []futuresLocalCase{
			{
				name: "pageSize at the exclusive bound",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{PageSize: futuresMaxPageSizeExclusive})
				},
			},
			{
				name: "pageSize far above the bound",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{PageSize: 5000})
				},
			},
			{
				name: "startDate is not eight digits",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{StartDate: "2026-09-01"})
				},
			},
			{
				name: "startDate is eight characters but not all digits",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{StartDate: "2026090X"})
				},
			},
			{
				name: "startDate is shaped but not a calendar date",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{StartDate: "20260230"})
				},
			},
			{
				name: "endDate is not eight digits",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{EndDate: "2026092"})
				},
			},
			{
				name: "endDate is shaped but not a calendar date",
				op:   op,
				run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
					t.Helper()
					return paged(t, ctx, svc, id, PageRequest{EndDate: "20261301"})
				},
			},
		}
	}

	var out []futuresLocalCase

	out = append(out,
		futuresLocalCase{
			name: "QueryProductInfo with no codes", op: opFuturesQueryProductInfo,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				products, err := svc.QueryProductInfo(ctx, id, nil)
				requireZero(t, products)
				return err
			},
		},
		futuresLocalCase{
			name: "QueryProductInfo with an empty code list", op: opFuturesQueryProductInfo,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				products, err := svc.QueryProductInfo(ctx, id, []string{})
				requireZero(t, products)
				return err
			},
		},
		futuresLocalCase{
			name: "QueryProductInfo with a blank code", op: opFuturesQueryProductInfo,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				products, err := svc.QueryProductInfo(ctx, id, []string{"HSI2609.HK", "   "})
				requireZero(t, products)
				return err
			},
		},
		futuresLocalCase{
			name: "QueryProductInfo with an interior-whitespace code", op: opFuturesQueryProductInfo,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				products, err := svc.QueryProductInfo(ctx, id, []string{"HSI 2609"})
				requireZero(t, products)
				return err
			},
		},
		futuresLocalCase{
			name: "QueryMaxBuySellAmount with an empty code", op: opFuturesQueryMaxBuySellAmount,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				capacity, err := svc.QueryMaxBuySellAmount(ctx, id, domain.Symbol{})
				requireZero(t, capacity)
				return err
			},
		},
		futuresLocalCase{
			name: "QueryMaxBuySellAmount with an interior-whitespace code", op: opFuturesQueryMaxBuySellAmount,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				capacity, err := svc.QueryMaxBuySellAmount(ctx, id, domain.Symbol{Code: "HSI\t2609"})
				requireZero(t, capacity)
				return err
			},
		},
	)

	out = append(out, pageCases(opFuturesQueryHistoryEntrust,
		func(t *testing.T, ctx context.Context, svc *FuturesService, id domain.AccountID, p PageRequest) error {
			t.Helper()
			page, err := svc.QueryHistoryEntrustPage(ctx, id, p)
			requireZero(t, page)
			return err
		})...)
	out = append(out, pageCases(opFuturesQueryHistoryDeliver,
		func(t *testing.T, ctx context.Context, svc *FuturesService, id domain.AccountID, p PageRequest) error {
			t.Helper()
			page, err := svc.QueryHistoryDeliverPage(ctx, id, p)
			requireZero(t, page)
			return err
		})...)

	return out
}

// TestFuturesReadRejectsBadInputWithoutARequest is the input-validation arm: a
// bad code, an oversized page, or a date the Gateway will not accept is refused
// locally, with the method's own op, and costs no HTTP request.
//
// The page rows are also the "no pre-flight request" property ADR 0003 relies on
// for the mutations, asserted on a read so C5 inherits a proven mechanism rather
// than a claim.
func TestFuturesReadRejectsBadInputWithoutARequest(t *testing.T) {
	for _, tc := range futuresLocalCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.run(t, t.Context(), NewFuturesService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestFuturesReadAcceptsThePageBoundaries pins the accepting side of the page
// table, because a validator that refused everything would satisfy the table
// above while making the two paged reads unusable.
//
// Three rows matter: 99 is the largest documented page size and must be accepted;
// 0 and a negative size must be *defaulted* rather than rejected, which is the
// zero-value contract PageRequest documents; and 20260230 must be rejected while
// 20240229 — the leap day — is accepted, so the calendar check is a real calendar
// and not a length check.
func TestFuturesReadAcceptsThePageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		page     PageRequest
		wantNo   int
		wantSize int
	}{
		{"largest documented size", PageRequest{PageSize: futuresMaxPageSizeExclusive - 1},
			futuresDefaultPageNo, futuresMaxPageSizeExclusive - 1},
		{"smallest documented size", PageRequest{PageSize: 1}, futuresDefaultPageNo, 1},
		{"zero value is defaulted", PageRequest{}, futuresDefaultPageNo, futuresDefaultPageSize},
		{"negative is defaulted", PageRequest{PageNo: -9, PageSize: -9},
			futuresDefaultPageNo, futuresDefaultPageSize},
		{"leap day is a real date", PageRequest{StartDate: "20240229", EndDate: "20240229"},
			futuresDefaultPageNo, futuresDefaultPageSize},
		{"unbounded range is legitimate", PageRequest{PageNo: 3}, 3, futuresDefaultPageSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, tc2 := range futuresReadCases() {
				if tc2.name != "QueryHistoryEntrustPage" {
					continue
				}
				exec := newSequencedExecutor(t, sequencedReply{reply: tc2.reply})
				page, err := NewFuturesService(exec).QueryHistoryEntrustPage(
					t.Context(), futuresFixtureAccount(), tc.page)
				if err != nil {
					t.Fatalf("QueryHistoryEntrustPage with page %+v = %v, want nil", tc.page, err)
				}
				if page == nil {
					t.Fatal("QueryHistoryEntrustPage returned no page and no error")
				}
				requireCalls(t, exec, 1)
				got, ok := exec.lastParams(t).(futuresPageQueryWireRequest)
				if !ok {
					t.Fatalf("the params are %T, want futuresPageQueryWireRequest", exec.lastParams(t))
				}
				if got.PageNo != tc.wantNo {
					t.Errorf("pageNo on the wire = %d, want %d", got.PageNo, tc.wantNo)
				}
				if got.PageSize != tc.wantSize {
					t.Errorf("pageSize on the wire = %d, want %d", got.PageSize, tc.wantSize)
				}
			}
		})
	}
}

// TestFuturesLeapDayIsRejectedOnANonLeapYear is the negative twin of the row
// above, kept separate because a table that only asserted the accepting side
// would pass a validator whose calendar check never ran.
func TestFuturesLeapDayIsRejectedOnANonLeapYear(t *testing.T) {
	exec := newSequencedExecutor(t)
	page, err := NewFuturesService(exec).QueryHistoryDeliverPage(t.Context(), futuresFixtureAccount(),
		PageRequest{StartDate: "20260229"})
	assertInvalidParam(t, err, opFuturesQueryHistoryDeliver)
	requireZero(t, page)
	requireCalls(t, exec, 0)
}

// ---------------------------------------------------------------------------
// Transport error arms
// ---------------------------------------------------------------------------

// errFuturesExecutorDown is the sentinel the Level 1 rows return. It is compared
// with errors.Is and never on a rendered message.
var errFuturesExecutorDown = errors.New("futures: scripted executor failure")

// TestFuturesReadExecutorFailurePropagates is Level 1: the executor's error
// reaches the caller unchanged, the zero value comes back beside it, and exactly
// one call was recorded.
func TestFuturesReadExecutorFailurePropagates(t *testing.T) {
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: errFuturesExecutorDown})

			err := tc.invoke(t, t.Context(), NewFuturesService(exec))
			if !errors.Is(err, errFuturesExecutorDown) {
				t.Fatalf("%s = %v, want the executor's own error", tc.name, err)
			}
			futuresExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)
		})
	}
}

// TestFuturesReadGatewayRejectionArrivesTyped is Level 2: a real ok:false
// envelope becomes a typed *errs.Error carrying the Gateway's code, its category,
// and the service's own op.
//
// This is the row that proves the reads do not launder a typed Gateway error into
// an opaque one — something a fake cannot show, because a fake never builds a
// typed error at all. A caller branching on Op to decide whether to re-establish a
// futures session is misinformed by a read that filled in the wrong one, and the
// two history ops are exactly where a copy-paste would go unnoticed.
func TestFuturesReadGatewayRejectionArrivesTyped(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the Gateway rejection %q", tc.name, code)
			}
			errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a %s is a trading-state failure the caller "+
					"must reconcile before resubmitting", got, errs.CategoryTrading, code)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1: the read must not have "+
					"reached a route it was not sent to", got)
			}
		})
	}
}

// TestFuturesReadsRetryUnderARetryPolicy is the control for every single-request
// assertion above, and it is the row that makes them mean something.
//
// A single HTTP request is also what a client with no retry policy installed
// produces, so without this control a service that silently ignored its policy
// would pass all of it. It installs MaxAttempts 5 with a zero backoff and a
// retryable rejection ("1011 service busy", which errs.Retryable reports as
// retryable) and shows all eight futures reads taking all five attempts.
//
// It therefore proves two things at once. That the policy is live, so the one
// request in Level 2 is a decision. And that none of the eight routes is in
// internal/resilience's closed mutation set — which is the opposite of what the
// three C5 futures mutations must do, and is stated here so C5 inherits a proven
// boundary rather than an assumption about it.
func TestFuturesReadsRetryUnderARetryPolicy(t *testing.T) {
	const wantAttempts = 5

	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
			})
			svc := NewFuturesService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: wantAttempts})))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			}
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			if got := rec.total(); got != wantAttempts {
				t.Fatalf("total requests = %d, want %d: this control exists to show the retry "+
					"policy really is applied, and that a futures read is classified as a query",
					got, wantAttempts)
			}
		})
	}
}

// TestFuturesReadCallIsCancellable pins that a cancelled context is reported as a
// cancellation and never as a success, matched through errors.Is on
// context.Canceled rather than on a rendered message.
//
// All eight are single-shot, so each costs exactly one recorded call: each hands
// the context to the executor and lets the request path report the cancellation.
// That is a promise the count holds rather than an accident of the assertion, and
// it is why no futures read here can be as cheap to abort as a cursor walk, which
// checks ctx.Err() before spending a request.
func TestFuturesReadCallIsCancellable(t *testing.T) {
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: context.Canceled})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(t, ctx, NewFuturesService(exec))
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
				t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestFuturesReadMalformedReplyIsReportedAndNotDressedAsAGatewayCode covers the
// third way a request can fail: the Gateway says ok:true and then sends a data
// member the endpoint cannot decode.
//
// A read that ignored the decode failure would return an empty result and no
// error, and the caller would read that as a successful empty book — which for
// QueryFundInfo means a zero balance and for QueryHoldsList means no positions.
// The zero value is asserted, and so is the absence of a status code: a local
// transport failure must not be dressed as a Gateway code, or a caller retrying on
// a code would chase a fault the Gateway never reported. The op is still filled
// in, because that is what a caller branches on to decide whether to re-authenticate.
func TestFuturesReadMalformedReplyIsReportedAndNotDressedAsAGatewayCode(t *testing.T) {
	for _, tc := range futuresReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(`42`),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s on a data member of 42 = nil error; the value would be silently empty",
					tc.name)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Errorf("CodeOf = (%q, true), want (false): a decode failure is a local "+
					"transport error and must not carry a Gateway status code", code)
			}
			var typed *errs.Error
			if !errors.As(err, &typed) {
				t.Fatalf("errors.As(%T) yielded no *errs.Error, so the op is not attributable", err)
			}
			if typed.Op != tc.op {
				t.Errorf("Op = %q, want %q: the op still identifies which method failed", typed.Op, tc.op)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Wire shape
// ---------------------------------------------------------------------------

// TestFuturesNoParamReadsSendAnEmptyParamsObject asserts the `{}` the four
// account-scoped reads are required to send, on the wire rather than through a
// struct comparison.
//
// The check is a decoded params member, not a substring, because the failure it
// guards against is a *missing* key: internal/transport drops a nil params from
// the envelope entirely, and an absent "params" is a different request from an
// empty one. A `strings.Contains(body, "{}")` assertion would pass on an absent
// key, which is why this decodes.
func TestFuturesNoParamReadsSendAnEmptyParamsObject(t *testing.T) {
	rows := 0
	for _, tc := range futuresReadCases() {
		if !tc.noParams {
			continue
		}
		rows++
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			params := futuresRecordedParams(t, rec, string(tc.route))
			if len(params) != 0 {
				t.Errorf("%s sent %v, want an empty params object: a futures request carries "+
					"nothing on the four account-scoped reads", tc.name, futuresBodyKeys(params))
			}
		})
	}
	// The count is asserted so a new read cannot land in neither bucket and pass
	// this test by never being visited: there are four account-scoped reads and
	// four that carry a body.
	if rows != 4 {
		t.Errorf("the table marks %d reads as taking no request body, want the documented 4", rows)
	}
}

// TestFuturesPageRequestsAlwaysCarryTheirPageCounters is the omitempty row that is
// reachable through a public method, and it asserts the *present* direction.
//
// design-futures-requests §6.1 rules that pageNo and pageSize carry no omitempty,
// which is the one place the v-next body differs from the released
// pkg/hstong/future body that tags all four fields omitempty. The bytes are
// identical for a well-formed request, so this test is the only thing that would
// catch a future editor "fixing" the tags back: with omitempty, a caller-supplied
// zero would vanish from the key set, and this asserts the two keys are always
// there.
//
// The counterpart assertion — startDate and endDate *are* dropped when unbounded —
// is the other half and is in the same table, because an omitempty rule tested in
// one direction only is half a rule.
func TestFuturesPageRequestsAlwaysCarryTheirPageCounters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		page  PageRequest
		keys  []string
		no    int
		size  int
		dated bool
	}{
		{
			name: "a fully zero page still sends both counters",
			page: PageRequest{},
			keys: []string{"pageNo", "pageSize"},
			no:   futuresDefaultPageNo, size: futuresDefaultPageSize,
		},
		{
			name: "an unbounded page drops both dates",
			page: PageRequest{PageNo: 4, PageSize: 7},
			keys: []string{"pageNo", "pageSize"},
			no:   4, size: 7,
		},
		{
			name: "a bounded page carries all four",
			page: PageRequest{PageNo: 1, PageSize: 99, StartDate: "20260901", EndDate: "20260921"},
			keys: []string{"endDate", "pageNo", "pageSize", "startDate"},
			no:   1, size: 99, dated: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, read := range []string{"QueryHistoryEntrustPage", "QueryHistoryDeliverPage"} {
				var caseRow futuresReadCase
				for _, c := range futuresReadCases() {
					if c.name == read {
						caseRow = c
					}
				}
				rec := newWireRecorder(map[string]string{
					string(caseRow.route): gatewaySuccess(string(caseRow.reply)),
				})
				svc := NewFuturesService(newWireExecutor(t, rec))

				var err error
				if read == "QueryHistoryEntrustPage" {
					_, err = svc.QueryHistoryEntrustPage(t.Context(), futuresFixtureAccount(), tc.page)
				} else {
					_, err = svc.QueryHistoryDeliverPage(t.Context(), futuresFixtureAccount(), tc.page)
				}
				if err != nil {
					t.Fatalf("%s = %v, want nil", read, err)
				}

				params := futuresRecordedParams(t, rec, string(caseRow.route))
				futuresRequireKeys(t, read+" body", params, tc.keys...)
				var pageNo, pageSize int
				if err := json.Unmarshal(params["pageNo"], &pageNo); err != nil {
					t.Fatalf("pageNo is not a JSON number (%s): the vendor types it as a boxed "+
						"Integer, and it is a page counter rather than money", params["pageNo"])
				}
				if err := json.Unmarshal(params["pageSize"], &pageSize); err != nil {
					t.Fatalf("pageSize is not a JSON number (%s)", params["pageSize"])
				}
				if pageNo != tc.no || pageSize != tc.size {
					t.Errorf("pageNo/pageSize on the wire = %d/%d, want %d/%d",
						pageNo, pageSize, tc.no, tc.size)
				}
				if !tc.dated {
					for _, key := range []string{"startDate", "endDate"} {
						if _, ok := params[key]; ok {
							t.Errorf("the %q key is present on an unbounded history query: an "+
								"absent range is a legitimate wire state and must be omitted", key)
						}
					}
				}
			}
		})
	}
}

// TestFuturesProductAndCapacityRequestsCarryNoMarket is the endpoint-level
// counterpart to TestFuturesRequestsCarryNoMarketOrCursorField: the struct-level
// rule restated where it is actually reachable, with the bodies on the wire.
//
// A market field would be a wire change on a guess and invisible on the mock,
// which answers all eleven futures routes by path. Both bodies must therefore be
// exactly the keys the vendor sends: one stockCode, singular tag for the batch
// because that is the vendor's spelling.
func TestFuturesProductAndCapacityRequestsCarryNoMarket(t *testing.T) {
	t.Run("QueryProductInfo", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeFuturesQueryProductInfo): gatewaySuccess(futuresProductInfoResponseBody),
		})
		svc := NewFuturesService(newWireExecutor(t, rec))
		if _, err := svc.QueryProductInfo(t.Context(), futuresFixtureAccount(), futuresFixtureCodes()); err != nil {
			t.Fatalf("QueryProductInfo = %v, want nil", err)
		}
		params := futuresRecordedParams(t, rec, string(client.RouteTradeFuturesQueryProductInfo))
		futuresRequireKeys(t, "QueryProductInfo body", params, "stockCode")
		var codes []string
		if err := json.Unmarshal(params["stockCode"], &codes); err != nil {
			t.Fatalf("stockCode is not a JSON string array (%s): the Gateway's own key is the "+
				"singular spelling for a list", params["stockCode"])
		}
		if !reflect.DeepEqual(codes, futuresFixtureCodes()) {
			t.Errorf("stockCode = %v, want the caller's codes verbatim and in order", codes)
		}
	})

	t.Run("QueryMaxBuySellAmount", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeFuturesQueryMaxBuySellAmount): gatewaySuccess(futuresCapacityBody),
		})
		svc := NewFuturesService(newWireExecutor(t, rec))
		symbol := domain.Symbol{Code: "HSI2609.HK", Market: domain.MarketUS}
		if _, err := svc.QueryMaxBuySellAmount(t.Context(), futuresFixtureAccount(), symbol); err != nil {
			t.Fatalf("QueryMaxBuySellAmount = %v, want nil", err)
		}
		params := futuresRecordedParams(t, rec, string(client.RouteTradeFuturesQueryMaxBuySellAmount))
		futuresRequireKeys(t, "QueryMaxBuySellAmount body", params, "stockCode")
		futuresRequireString(t, params, "stockCode", "HSI2609.HK")
	})
}

// ---------------------------------------------------------------------------
// Money
// ---------------------------------------------------------------------------

// futuresMoneySet is one of the frozen value sets, with the guard that states
// which failure mode it is chosen to detect.
type futuresMoneySet struct {
	label string
	cases []moneyCase
	guard func(*testing.T, string)
}

// futuresMoneySets returns both sets, applied to every host below rather than to
// whichever one a given host happened to get.
//
// A value that discriminates a float64 is often blind to a fixed-scale
// formatter, and vice versa, so applying only one set to a host would leave half
// the regression net unbuilt there.
func futuresMoneySets() []futuresMoneySet {
	return []futuresMoneySet{
		{"float64-hostile", float64HostilePrices, requireFloat64Hostile},
		{"scale-hostile", scaleHostilePrices, requireScaleHostile},
	}
}

// TestFuturesMoneyValueSetsAreStillHostile is the guard on the guard: each row
// asserts the property that makes it worth having, so a future edit that softened
// a literal into a benign one deletes the regression net below without any test
// noticing. market_money_test.go and account_money_test.go assert the same rows for
// their own hosts; it is repeated so this file's hosts are not silently disarmed
// by an edit to either of those.
func TestFuturesMoneyValueSetsAreStillHostile(t *testing.T) {
	if len(float64HostilePrices) == 0 || len(float64HostileQuantities) == 0 || len(scaleHostilePrices) == 0 {
		t.Fatal("a money value set is empty, so the round-trip tests in this file would pass " +
			"vacuously")
	}
	for _, set := range futuresMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					set.guard(t, tc.in)
					if tc.want == "" {
						t.Fatalf("row %q has no want value", tc.name)
					}
				})
			}
		})
	}
	t.Run("float64-hostile-quantities", func(t *testing.T) {
		for _, tc := range float64HostileQuantities {
			t.Run(tc.name, func(t *testing.T) {
				requireFloat64Hostile(t, tc.in)
			})
		}
	})
}

// futuresAssertVerbatim is the central money assertion: the domain value reports
// the exact digits the Gateway sent, and the failure message names the two
// conversions that would have destroyed them.
func futuresAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal the Gateway sent).\n"+
		"  input literal:          %s\n"+
		"  a float64 round trip:   %s\n"+
		"  the removed %%.3f form: %s",
		field, got, tc.want, tc.in,
		strconv.FormatFloat(mustFloat(t, tc.in), 'f', -1, 64),
		legacyRounded(mustFloat(t, tc.in)))
}

// futuresAssertQuotedOnWire is the real-stack half. A field retyped from string to
// float64 would still decode and might still produce a plausible number, so this
// is the assertion that catches it: the recorded reply carries the value as a
// quoted JSON string, which a float64 field cannot produce.
//
// It looks for tc.in, not tc.want, because the two differ — the Gateway sends
// 1e-330 and the domain constructor expands it to the positional form — and
// asserting the expanded form here would be asserting the wrong layer.
func futuresAssertQuotedOnWire(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	quoted := `"` + field + `":"` + tc.in + `"`
	if !strings.Contains(body, quoted) {
		t.Errorf("the recorded reply does not contain %s.\n"+
			"  body:             %s\n"+
			"  wanted substring: %s",
			quoted, body, quoted)
		return
	}
	if unquoted := `"` + field + `":` + tc.in; strings.Contains(body, unquoted) {
		t.Errorf("the unquoted form %s is also present, so the field is a JSON number on the "+
			"wire: a float64 in the response path would render this way", unquoted)
	}
}

// futuresMoneyHost is one money-bearing field on one read endpoint.
//
// The body builder and the reader are functions rather than names so the same row
// drives both the sequenced fake and the real HTTP stack without either reaching
// into the other's fixture. read performs the call itself and returns the mapped
// value's digits, so a row cannot assert a field the method does not return.
type futuresMoneyHost struct {
	name  string
	op    string
	route client.Route
	// body builds a reply carrying the given literal in the targeted field.
	body func(t *testing.T, literal string) json.RawMessage
	// read drives the endpoint over exec and returns the mapped field.
	read func(t *testing.T, ctx context.Context, exec Executor) string
	// quantity is true for a field the 27-digit integer set is aimed at. The
	// price sets are not fed to a quantity: 1e-330 is a price-shaped value, and
	// a quantity host with a price set would be asserting something the field
	// never had to do.
	quantity bool
}

// futuresMoneyHosts is the table. It covers all four value types the futures mappers
// produce — Money, Price, Quantity, Rate — and all eight reads, so no endpoint is
// money-tested only by proxy through another.
func futuresMoneyHosts() []futuresMoneyHost {
	id := futuresFixtureAccount()
	sym := futuresFixtureSymbol()

	return []futuresMoneyHost{
		{
			name: "QueryProductInfo/contractSize", op: opFuturesQueryProductInfo,
			route: client.RouteTradeFuturesQueryProductInfo,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresProductInfoResponseBody),
					"productInfoVos", "contractSize", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				products, err := NewFuturesService(exec).QueryProductInfo(ctx, id, futuresFixtureCodes())
				if err != nil {
					t.Fatalf("QueryProductInfo: %v", err)
				}
				if len(products) != 1 {
					t.Fatalf("products = %d, want 1", len(products))
				}
				return products[0].ContractSize.String()
			},
			quantity: true,
		},
		{
			name: "QueryMaxBuySellAmount/initialMargin", op: opFuturesQueryMaxBuySellAmount,
			route: client.RouteTradeFuturesQueryMaxBuySellAmount,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				var fields map[string]json.RawMessage
				if err := json.Unmarshal([]byte(futuresCapacityBody), &fields); err != nil {
					t.Fatalf("test bug: the capacity fixture is not a JSON object: %v", err)
				}
				fields["initialMargin"] = json.RawMessage(strconv.Quote(literal))
				out, err := json.Marshal(fields)
				if err != nil {
					t.Fatalf("test bug: re-marshalling the capacity fixture: %v", err)
				}
				return out
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				capacity, err := NewFuturesService(exec).QueryMaxBuySellAmount(ctx, id, sym)
				if err != nil {
					t.Fatalf("QueryMaxBuySellAmount: %v", err)
				}
				return capacity.InitialMargin.String()
			},
		},
		{
			name: "QueryFundInfo/assetBalance", op: opFuturesQueryFundInfo,
			route: client.RouteTradeFuturesQueryFundInfo,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresFundInfoReplyWith(t, "assetBalance", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				account, err := NewFuturesService(exec).QueryFundInfo(ctx, id)
				if err != nil {
					t.Fatalf("QueryFundInfo: %v", err)
				}
				return account.AssetBalance.String()
			},
		},
		{
			name: "QueryFundInfo/marginLevel", op: opFuturesQueryFundInfo,
			route: client.RouteTradeFuturesQueryFundInfo,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresFundInfoReplyWith(t, "marginLevel", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				account, err := NewFuturesService(exec).QueryFundInfo(ctx, id)
				if err != nil {
					t.Fatalf("QueryFundInfo: %v", err)
				}
				return account.MarginLevel.String()
			},
		},
		{
			name: "QueryFundInfo/cycRateUSDtoHSD", op: opFuturesQueryFundInfo,
			route: client.RouteTradeFuturesQueryFundInfo,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresFundInfoReplyWith(t, "cycRateUSDtoHSD", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				account, err := NewFuturesService(exec).QueryFundInfo(ctx, id)
				if err != nil {
					t.Fatalf("QueryFundInfo: %v", err)
				}
				return account.CycRateUSDtoHSD.String()
			},
		},
		{
			name: "QueryHoldsList/profitLoss", op: opFuturesQueryHoldsList,
			route: client.RouteTradeFuturesQueryHoldsList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresHoldBody),
					"holdsList", "profitLoss", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				result, err := NewFuturesService(exec).QueryHoldsList(ctx, id)
				if err != nil {
					t.Fatalf("QueryHoldsList: %v", err)
				}
				if len(result.Positions) != 1 {
					t.Fatalf("positions = %d, want 1", len(result.Positions))
				}
				return result.Positions[0].ProfitLoss.String()
			},
		},
		{
			name: "QueryHoldsList/costPrice", op: opFuturesQueryHoldsList,
			route: client.RouteTradeFuturesQueryHoldsList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresHoldBody),
					"holdsList", "costPrice", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				result, err := NewFuturesService(exec).QueryHoldsList(ctx, id)
				if err != nil {
					t.Fatalf("QueryHoldsList: %v", err)
				}
				if len(result.Positions) != 1 {
					t.Fatalf("positions = %d, want 1", len(result.Positions))
				}
				return result.Positions[0].CostPrice.String()
			},
		},
		{
			name: "QueryHoldsList/contractValue", op: opFuturesQueryHoldsList,
			route: client.RouteTradeFuturesQueryHoldsList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresHoldBody),
					"holdsList", "contractValue", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				result, err := NewFuturesService(exec).QueryHoldsList(ctx, id)
				if err != nil {
					t.Fatalf("QueryHoldsList: %v", err)
				}
				if len(result.Positions) != 1 {
					t.Fatalf("positions = %d, want 1", len(result.Positions))
				}
				return result.Positions[0].ContractValue.String()
			},
			quantity: true,
		},
		{
			name: "QueryHoldsList/ccyRate", op: opFuturesQueryHoldsList,
			route: client.RouteTradeFuturesQueryHoldsList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresHoldBody),
					"holdsList", "ccyRate", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				result, err := NewFuturesService(exec).QueryHoldsList(ctx, id)
				if err != nil {
					t.Fatalf("QueryHoldsList: %v", err)
				}
				if len(result.Positions) != 1 {
					t.Fatalf("positions = %d, want 1", len(result.Positions))
				}
				return result.Positions[0].CcyRate.String()
			},
		},
		{
			name: "QueryRealEntrustList/entrustPrice", op: opFuturesQueryRealEntrustList,
			route: client.RouteTradeFuturesQueryRealEntrustList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresOrderBody),
					"data", "entrustPrice", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				orders, err := NewFuturesService(exec).QueryRealEntrustList(ctx, id)
				if err != nil {
					t.Fatalf("QueryRealEntrustList: %v", err)
				}
				if len(orders) != 1 {
					t.Fatalf("orders = %d, want 1", len(orders))
				}
				return orders[0].OrderPrice.String()
			},
		},
		{
			name: "QueryRealEntrustList/entrustAmount", op: opFuturesQueryRealEntrustList,
			route: client.RouteTradeFuturesQueryRealEntrustList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresOrderBody),
					"data", "entrustAmount", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				orders, err := NewFuturesService(exec).QueryRealEntrustList(ctx, id)
				if err != nil {
					t.Fatalf("QueryRealEntrustList: %v", err)
				}
				if len(orders) != 1 {
					t.Fatalf("orders = %d, want 1", len(orders))
				}
				return orders[0].Quantity.String()
			},
			quantity: true,
		},
		{
			name: "QueryHistoryEntrustPage/businessPrice", op: opFuturesQueryHistoryEntrust,
			route: client.RouteTradeFuturesQueryHistoryEntrustList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresOrderBody),
					"data", "businessPrice", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				page, err := NewFuturesService(exec).QueryHistoryEntrustPage(ctx, id, futuresFixturePage())
				if err != nil {
					t.Fatalf("QueryHistoryEntrustPage: %v", err)
				}
				if len(page.Orders) != 1 {
					t.Fatalf("orders = %d, want 1", len(page.Orders))
				}
				return page.Orders[0].FillPrice.String()
			},
		},
		{
			name: "QueryRealDeliverList/businessPrice", op: opFuturesQueryRealDeliverList,
			route: client.RouteTradeFuturesQueryRealDeliverList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresOrderBody),
					"data", "businessPrice", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				fills, err := NewFuturesService(exec).QueryRealDeliverList(ctx, id)
				if err != nil {
					t.Fatalf("QueryRealDeliverList: %v", err)
				}
				if len(fills) != 1 {
					t.Fatalf("fills = %d, want 1", len(fills))
				}
				return fills[0].Price.String()
			},
		},
		{
			name: "QueryHistoryDeliverPage/businessAmount", op: opFuturesQueryHistoryDeliver,
			route: client.RouteTradeFuturesQueryHistoryDeliverList,
			body: func(t *testing.T, literal string) json.RawMessage {
				t.Helper()
				return futuresReplaceRowField(t, json.RawMessage(futuresOrderBody),
					"data", "businessAmount", literal)
			},
			read: func(t *testing.T, ctx context.Context, exec Executor) string {
				t.Helper()
				page, err := NewFuturesService(exec).QueryHistoryDeliverPage(ctx, id, futuresFixturePage())
				if err != nil {
					t.Fatalf("QueryHistoryDeliverPage: %v", err)
				}
				if len(page.Fills) != 1 {
					t.Fatalf("fills = %d, want 1", len(page.Fills))
				}
				return page.Fills[0].FilledQty.String()
			},
			quantity: true,
		},
	}
}

// futuresFundInfoReplyWith rewrites one key of the fundInfo block and puts the
// whole object back inside the envelope the two endpoints that carry it decode —
// so one builder serves QueryFundInfo and QueryHoldsList.
//
// It targets any of the twenty-odd fund-info keys, which is the point: a money
// test aimed at one field must not need a second hand-written twenty-key fixture,
// because a second fixture drifts and the drift is invisible.
func futuresFundInfoReplyWith(t *testing.T, key, value string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(futuresFundInfoBody), &fields); err != nil {
		t.Fatalf("test bug: the fund info fixture is not a JSON object: %v", err)
	}
	if _, ok := fields[key]; !ok {
		t.Fatalf("test bug: the fund info fixture has no %q key; it has %v", key, futuresBodyKeys(fields))
	}
	fields[key] = json.RawMessage(strconv.Quote(value))
	inner, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("test bug: re-marshalling the fund info fixture: %v", err)
	}
	return json.RawMessage(`{"fundInfo":` + string(inner) + `}`)
}

// futuresMoneyCasesFor returns the value sets a host is driven with.
func futuresMoneyCasesFor(host futuresMoneyHost) []futuresMoneySet {
	if host.quantity {
		return []futuresMoneySet{
			{"float64-hostile-quantity", float64HostileQuantities, requireFloat64Hostile},
		}
	}
	return futuresMoneySets()
}

// TestFuturesReadMoneyCrossesVerbatim drives every money host over the
// sequencedExecutor with every value set that host is aimed at.
//
// The quantity hosts get the 27-digit integer rather than the price sets, because
// 1e-330 is a price-shaped value and a quantity is not: the 27-digit integer is
// the quantity analogue of the underflowing price — a legal decimal that no
// binary64 can represent.
func TestFuturesReadMoneyCrossesVerbatim(t *testing.T) {
	for _, host := range futuresMoneyHosts() {
		t.Run(host.name, func(t *testing.T) {
			if host.op == "" || host.route == "" {
				t.Fatal("test bug: the host has no op or route, so the row proves nothing")
			}
			for _, set := range futuresMoneyCasesFor(host) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							exec := newSequencedExecutor(t, sequencedReply{
								reply: host.body(t, tc.in),
							})
							futuresAssertVerbatim(t, host.name, host.read(t, t.Context(), exec), tc)
							requireCalls(t, exec, 1)
						})
					}
				})
			}
		})
	}
}

// TestFuturesReadHostileValuesSurviveTheRealStack is the same guarantee proved end
// to end: a real *client.Client, a real HTTP round trip, a real JSON decode, and
// a value that a float64 anywhere in that path would destroy.
//
// It is the row that says "reached the wire" and "came off the wire" literally.
// The quoted-on-wire assertion is included, so a field retyped from string to
// float64 fails on the quoting even if the decoded number happened to look right
// — which is the regression a byte-for-byte value assertion alone would miss,
// because a float64 that happens to render the same digits passes it.
func TestFuturesReadHostileValuesSurviveTheRealStack(t *testing.T) {
	for _, host := range futuresMoneyHosts() {
		t.Run(host.name, func(t *testing.T) {
			for _, set := range futuresMoneyCasesFor(host) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							reply := string(host.body(t, tc.in))
							rec := newWireRecorder(map[string]string{
								string(host.route): gatewaySuccess(reply),
							})
							exec := newWireExecutor(t, rec)

							futuresAssertVerbatim(t, host.name, host.read(t, t.Context(), exec), tc)
							field := host.name[strings.LastIndex(host.name, "/")+1:]
							futuresAssertQuotedOnWire(t, reply, field, tc)
							if got := rec.count(string(host.route)); got != 1 {
								t.Errorf("requests to %s = %d, want exactly 1", host.route, got)
							}
						})
					}
				})
			}
		})
	}
}

// TestFuturesMaxBuySellAmountCountsStayExactInt64 is the one value set the
// quantity table cannot express, because these two fields are not quoted strings:
// they are the int64 contract counts.
//
// Three properties are asserted and each is load-bearing. That both counts are
// exact — 2^53+1 has no float64 representation, so a float in the path renders
// the buy count as the even number below it. That the two are not swapped — a
// mapper that copied MaxSellAmount into MaxBuyAmount would otherwise be
// symmetric and invisible. And that a null count, the Gateway's way of saying
// "you can buy nothing", becomes an explicit zero rather than an absent value —
// which is the reason domain.FuturesCapacityFromDTO formats an integer instead
// of treating a zero as missing.
func TestFuturesMaxBuySellAmountCountsStayExactInt64(t *testing.T) {
	const (
		buy  = int64(9007199254740993) // 2^53 + 1: the first integer binary64 cannot hold
		sell = int64(9007199254740992) // 2^53 exactly
	)
	// Prove the case is hostile before relying on it: a float64 renders the buy
	// count as the sell count, so the two rows below are only distinguishable if
	// this holds.
	if got := strconv.FormatFloat(float64(buy), 'f', -1, 64); got != strconv.FormatInt(sell, 10) {
		t.Fatalf("test bug: float64(%d) renders as %q, which is not %d, so this case no longer "+
			"discriminates a float in the path", buy, got, sell)
	}

	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(futuresCapacityBody)})
	capacity, err := NewFuturesService(exec).QueryMaxBuySellAmount(t.Context(),
		futuresFixtureAccount(), futuresFixtureSymbol())
	if err != nil {
		t.Fatalf("QueryMaxBuySellAmount = %v, want nil", err)
	}
	if got := capacity.MaxBuyAmount.String(); got != "9007199254740993" {
		t.Errorf("MaxBuyAmount = %s, want 9007199254740993: a float64 in this path renders it "+
			"as its even neighbour", got)
	}
	if got := capacity.MaxSellAmount.String(); got != "9007199254740992" {
		t.Errorf("MaxSellAmount = %s, want 9007199254740992", got)
	}
	if capacity.PositionStatus != 1 {
		t.Errorf("positionStatus = %d, want 1", capacity.PositionStatus)
	}
	futuresRequireMoney(t, "initialMargin", capacity.InitialMargin, "123456.789", "HKD")
	if capacity.Ccy != "HKD" {
		t.Errorf("ccy = %q, want HKD", capacity.Ccy)
	}

	t.Run("a null count is an explicit zero", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(
			`{"positionStatus":0,"maxBuyAmount":null,"maxSellAmount":null,"initialMargin":"0","ccy":"HKD"}`)})
		capacity, err := NewFuturesService(exec).QueryMaxBuySellAmount(t.Context(),
			futuresFixtureAccount(), futuresFixtureSymbol())
		if err != nil {
			t.Fatalf("QueryMaxBuySellAmount with null counts = %v, want nil", err)
		}
		if got := capacity.MaxBuyAmount.String(); got != "0" {
			t.Errorf("MaxBuyAmount = %s, want an explicit 0: \"you can buy nothing\" is a "+
				"legitimate answer, not an absent one", got)
		}
		if got := capacity.MaxSellAmount.String(); got != "0" {
			t.Errorf("MaxSellAmount = %s, want an explicit 0", got)
		}
	})
}

// TestFuturesReadDomainTypesCarryNoFloat is the static half of hard rule 3 for the
// nine futures models, on the same reasoning as TestAccountDomainTypesCarryNoFloat:
// scripts/check_money.py guards by field name and by wire key, which is the right
// heuristic for the generated DTOs and blind to a decimal field given an innocuous
// name. This walks the types the eight reads return and asserts no field is a
// float at all, whatever it is called.
func TestFuturesReadDomainTypesCarryNoFloat(t *testing.T) {
	for _, v := range []any{
		domain.FuturesProduct{},
		domain.FuturesCapacity{},
		domain.FuturesAccount{},
		domain.FuturesPosition{},
		domain.FuturesOrder{},
		domain.FuturesFill{},
	} {
		rt := reflect.TypeOf(v)
		t.Run(rt.Name(), func(t *testing.T) {
			fields := 0
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				fields++
				switch f.Type.Kind() {
				case reflect.Float32, reflect.Float64:
					t.Errorf("%s.%s is a %s: money, prices, quantities, and rates are decimal "+
						"values in this layer (docs/DESIGN.md §7, hard rule 3)", rt.Name(), f.Name, f.Type)
				}
			}
			if fields == 0 {
				t.Errorf("%s has no fields, so this test would pass vacuously", rt.Name())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Partial replies, and the loop over rows
// ---------------------------------------------------------------------------

// TestFuturesReadToleratesAPartialReply is the absent-field contract, and it is
// the one respect in which the futures mappers deliberately diverge from the cash
// ones.
//
// The cash domain.*FromDTO mappers hand the wire string straight to a MustNew…
// constructor, and the empty string is one that constructor panics on. A cash
// mapper therefore crashes the caller's process on a partial reply — finding F2 —
// and B1 explicitly refused to write a recover()-based test for it, because such a
// test would pass while pinning the defect. The futures mappers do not have that
// shape: they map "" to an explicit zero, because a partial reply is a *read* and
// a read must not crash a process.
//
// So these rows drive the emptiest replies the four envelopes can carry and assert
// a zero-valued result with no error and no panic. It is the test that would have
// been wrong to write with recover() and is right without it.
func TestFuturesReadToleratesAPartialReply(t *testing.T) {
	t.Run("QueryProductInfo with an empty product row", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"productInfoVos":[{}]}`)})
		products, err := NewFuturesService(exec).QueryProductInfo(t.Context(),
			futuresFixtureAccount(), futuresFixtureCodes())
		if err != nil {
			t.Fatalf("QueryProductInfo on a product row of {} = %v, want nil", err)
		}
		if len(products) != 1 || products[0] == nil {
			t.Fatalf("products = %+v, want one mapped product", products)
		}
		futuresRequireQuantity(t, "contractSize", products[0].ContractSize, "0")
	})

	t.Run("QueryMaxBuySellAmount with a bare capacity payload", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{}`)})
		capacity, err := NewFuturesService(exec).QueryMaxBuySellAmount(t.Context(),
			futuresFixtureAccount(), futuresFixtureSymbol())
		if err != nil {
			t.Fatalf("QueryMaxBuySellAmount on {} = %v, want nil", err)
		}
		if got := capacity.MaxBuyAmount.String(); got != "0" {
			t.Errorf("MaxBuyAmount = %s, want 0 for an absent count", got)
		}
		// An absent ccy must not produce a Money that panics on comparison with
		// a real one; it falls back to the base currency.
		futuresRequireMoney(t, "initialMargin", capacity.InitialMargin, "0", "HKD")
	})

	t.Run("QueryHoldsList with an empty position row and no funds block", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"holdsList":[{}]}`)})
		result, err := NewFuturesService(exec).QueryHoldsList(t.Context(), futuresFixtureAccount())
		if err != nil {
			t.Fatalf("QueryHoldsList on a position row of {} = %v, want nil", err)
		}
		if result.Account == nil {
			t.Fatal("account = nil; the funds snapshot is not optional, and a reader that " +
				"reconciles against it must be able to see the zero it was given")
		}
		futuresRequireMoney(t, "account.assetBalance", result.Account.AssetBalance, "0", "HKD")
		if len(result.Positions) != 1 {
			t.Fatalf("positions = %d, want 1", len(result.Positions))
		}
		pos := result.Positions[0]
		if pos == nil {
			t.Fatal("a present position row mapped to nil")
		}
		futuresRequireQuantity(t, "currentQty", pos.CurrentQty, "0")
		futuresRequirePrice(t, "costPrice", pos.CostPrice, "0")
		futuresRequireMoney(t, "profitLoss", pos.ProfitLoss, "0", "HKD")
		// The currency-labelled amounts keep their own currency even at zero, so
		// a caller summing the snapshot does not get a panic on a cross-currency
		// sum it did not ask for.
		futuresRequireMoney(t, "closeProfitHKD", pos.CloseProfitHKD, "0", "HKD")
	})

	t.Run("QueryRealEntrustList with an empty order row", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":[{}]}`)})
		orders, err := NewFuturesService(exec).QueryRealEntrustList(t.Context(), futuresFixtureAccount())
		if err != nil {
			t.Fatalf("QueryRealEntrustList on an order row of {} = %v, want nil", err)
		}
		if len(orders) != 1 || orders[0] == nil {
			t.Fatalf("orders = %+v, want one mapped order", orders)
		}
		futuresRequirePrice(t, "orderPrice", orders[0].OrderPrice, "0")
		futuresRequireQuantity(t, "quantity", orders[0].Quantity, "0")
		// A zero enum is a real answer here, not a defaulted one: the futures
		// status dictionary is the cash one, and the field carries no
		// "unset" spelling.
		if orders[0].IsValid {
			t.Error("isValid = true for an absent isValid; the mapper reads true only for the " +
				"wire's 1, so a partial row must not claim the row is valid")
		}
	})

	t.Run("an absent list is the non-nil empty slice on every list-returning read", func(t *testing.T) {
		exec := newSequencedExecutor(t,
			sequencedReply{reply: json.RawMessage(`{"productInfoVos":[]}`)},
			sequencedReply{reply: json.RawMessage(futuresRealListEmptyBody)},
			sequencedReply{reply: json.RawMessage(futuresRealListEmptyBody)},
		)
		svc := NewFuturesService(exec)
		id := futuresFixtureAccount()

		products, err := svc.QueryProductInfo(t.Context(), id, futuresFixtureCodes())
		if err != nil {
			t.Fatalf("QueryProductInfo: %v", err)
		}
		if products == nil {
			t.Error("products = nil for an empty list, want the non-nil empty slice")
		}
		orders, err := svc.QueryRealEntrustList(t.Context(), id)
		if err != nil {
			t.Fatalf("QueryRealEntrustList: %v", err)
		}
		if orders == nil {
			t.Error("orders = nil for an empty list, want the non-nil empty slice")
		}
		fills, err := svc.QueryRealDeliverList(t.Context(), id)
		if err != nil {
			t.Fatalf("QueryRealDeliverList: %v", err)
		}
		if fills == nil {
			t.Error("fills = nil for an empty list, want the non-nil empty slice")
		}
	})
}

// TestFuturesReadMethodsMapEveryRowDistinctly pins that the loops index rather
// than repeat: a body with two rows that differ must produce two domain values
// that differ, and a mapper that read row zero for every index would return two
// copies of the first.
//
// It uses the sequencedExecutor rather than the wire, because the property under
// test is in the method's own loop and not in the transport.
func TestFuturesReadMethodsMapEveryRowDistinctly(t *testing.T) {
	const twoOrders = `{"data":[` +
		`{"stockCode":"HSI2609.HK","entrustId":"F-1","entrustPrice":"25000","entrustAmount":"1","businessPrice":"25000","businessAmount":"1","status":"8","isValid":1,"canBeCanceled":1,"canBeUpdated":0},` +
		`{"stockCode":"HHI2612.HK","entrustId":"F-2","entrustPrice":"31000.5","entrustAmount":"2","businessPrice":"31000.25","businessAmount":"2","status":"4","isValid":0,"canBeCanceled":0,"canBeUpdated":1}` +
		`],"curPageNo":2,"curPageSize":2,"totalPageNo":5,"lastPage":0}`

	t.Run("QueryRealEntrustList", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(twoOrders)})
		orders, err := NewFuturesService(exec).QueryRealEntrustList(t.Context(), futuresFixtureAccount())
		if err != nil {
			t.Fatalf("QueryRealEntrustList = %v, want nil", err)
		}
		if len(orders) != 2 {
			t.Fatalf("orders = %d, want 2", len(orders))
		}
		if orders[0].StockCode == orders[1].StockCode {
			t.Fatalf("both rows mapped to %q, so the loop is not indexing", orders[0].StockCode)
		}
		if orders[0].EntrustID != "F-1" || orders[1].EntrustID != "F-2" {
			t.Errorf("entrustId = %q/%q, want F-1/F-2", orders[0].EntrustID, orders[1].EntrustID)
		}
		futuresRequirePrice(t, "orders[1].orderPrice", orders[1].OrderPrice, "31000.5")
		futuresRequirePrice(t, "orders[1].fillPrice", orders[1].FillPrice, "31000.25")
		futuresRequireQuantity(t, "orders[1].filledQty", orders[1].FilledQty, "2")
		if orders[0].CanBeCanceled != true || orders[1].CanBeCanceled != false {
			t.Errorf("canBeCanceled = %v/%v, want true/false: the flag is the field that tells a "+
				"live order from a spent one, so it is read from the wire's 1 and nothing else",
				orders[0].CanBeCanceled, orders[1].CanBeCanceled)
		}
		if orders[1].CanBeUpdated != true {
			t.Error("canBeUpdated = false for the wire's 1")
		}
	})

	t.Run("QueryRealDeliverList", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(twoOrders)})
		fills, err := NewFuturesService(exec).QueryRealDeliverList(t.Context(), futuresFixtureAccount())
		if err != nil {
			t.Fatalf("QueryRealDeliverList = %v, want nil", err)
		}
		if len(fills) != 2 {
			t.Fatalf("fills = %d, want 2", len(fills))
		}
		if fills[0].StockCode == fills[1].StockCode {
			t.Fatalf("both rows mapped to %q, so the loop is not indexing", fills[0].StockCode)
		}
		// A fill's Price is the business price and its OrderPrice the entrust
		// price. Swapping them would be symmetric only if both rows were equal,
		// which is why the two rows differ.
		futuresRequirePrice(t, "fills[1].price", fills[1].Price, "31000.25")
		futuresRequirePrice(t, "fills[1].orderPrice", fills[1].OrderPrice, "31000.5")
	})

	t.Run("QueryHistoryEntrustPage", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(twoOrders)})
		page, err := NewFuturesService(exec).QueryHistoryEntrustPage(t.Context(),
			futuresFixtureAccount(), futuresFixturePage())
		if err != nil {
			t.Fatalf("QueryHistoryEntrustPage = %v, want nil", err)
		}
		if len(page.Orders) != 2 {
			t.Fatalf("orders = %d, want 2", len(page.Orders))
		}
		if page.CurPageNo != 2 || page.CurPageSize != 2 || page.TotalPageNo != 5 {
			t.Errorf("page state = %d/%d/%d, want 2/2/5",
				page.CurPageNo, page.CurPageSize, page.TotalPageNo)
		}
		if page.LastPage {
			t.Error("lastPage = true for the wire's 0: this is page 2 of 5, and reporting the " +
				"last page would end a walk one page early")
		}
	})

	t.Run("QueryHoldsList with two positions", func(t *testing.T) {
		const twoHolds = `{"fundInfo":` + futuresFundInfoBody + `,"holdsList":[` +
			`{"stockCode":"HSI2609.HK","currentQty":"1","dataType":"10010","ccy":"HKD"},` +
			`{"stockCode":"HHI2612.HK","currentQty":"-2","dataType":"10020","ccy":"USD"}` +
			`]}`
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(twoHolds)})
		result, err := NewFuturesService(exec).QueryHoldsList(t.Context(), futuresFixtureAccount())
		if err != nil {
			t.Fatalf("QueryHoldsList = %v, want nil", err)
		}
		if len(result.Positions) != 2 {
			t.Fatalf("positions = %d, want 2", len(result.Positions))
		}
		if result.Positions[0].StockCode == result.Positions[1].StockCode {
			t.Fatalf("both positions mapped to %q, so the loop is not indexing",
				result.Positions[0].StockCode)
		}
		// dataType is the futures-only field that says which book a position is
		// in, and it is the reason no request on this path needs a market.
		if result.Positions[0].DataType != "10010" || result.Positions[1].DataType != "10020" {
			t.Errorf("dataType = %q/%q, want 10010/10020",
				result.Positions[0].DataType, result.Positions[1].DataType)
		}
		// A negative position quantity is real — a short position — and must
		// survive the round trip rather than being normalised to a magnitude.
		futuresRequireQuantity(t, "positions[1].currentQty", result.Positions[1].CurrentQty, "-2")
		// The funds snapshot travels with the positions and is not derived from
		// them: two positions and one snapshot.
		futuresRequireMoney(t, "account.assetBalance", result.Account.AssetBalance, "1000000", "HKD")
	})
}

// TestFuturesHoldResultKeepsBothHalves is the reason QueryHoldsList returns a
// struct, asserted from the caller's side.
//
// C2a §7.2 point 4 chose the struct over a position slice so that flattening two
// things into one is impossible. A caller holding only positions would see this
// as a successful empty read; a caller holding only the snapshot would see no
// positions at all. Both are the silent loss of a fact the caller asked for, and
// neither is an error, which is what makes it worth a test rather than a comment.
func TestFuturesHoldResultKeepsBothHalves(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(futuresHoldBody)})
	result, err := NewFuturesService(exec).QueryHoldsList(t.Context(), futuresFixtureAccount())
	if err != nil {
		t.Fatalf("QueryHoldsList = %v, want nil", err)
	}
	if result.Account == nil {
		t.Fatal("Account = nil: the funds snapshot is half of one atomic read and is not optional")
	}
	futuresRequireMoney(t, "account.cashBalUSD", result.Account.CashBalUSD, "6410", "USD")
	futuresRequireMoney(t, "account.cashBalHKD", result.Account.CashBalHKD, "250000", "HKD")
	futuresRequireMoney(t, "account.cashBalCNH", result.Account.CashBalCNH, "0", "CNH")
	// An unlabelled aggregate is attributed the base currency, which is an
	// assumption the domain layer states; this asserts it reached the caller
	// rather than leaving it implicit.
	futuresRequireMoney(t, "account.enableBalance", result.Account.EnableBalance, "500000", "HKD")
	if result.Account.MarginStatus != "1" || result.Account.AEID != "AE001" {
		t.Errorf("marginStatus/aeId = %q/%q, want 1/AE001",
			result.Account.MarginStatus, result.Account.AEID)
	}
	if len(result.Positions) != 1 {
		t.Fatalf("positions = %d, want 1", len(result.Positions))
	}
	// The snapshot is not derived from the position: the fixture's asset balance
	// and the position's profit differ, and both must be present.
	futuresRequireMoney(t, "positions[0].profitLoss", result.Positions[0].ProfitLoss, "500", "HKD")
}

// TestFuturesReadMethodsAreSafeForConcurrentUse is the concurrency half of the
// FuturesService contract, and it is cheap to state because the service holds no
// per-request state.
//
// It is worth having at all because the futures reads share one sequencedExecutor
// shape: the assertion is that N concurrent calls each get their own reply and
// their own result, so a future edit that cached a decoded envelope on the
// service would be caught rather than assumed away. The recorder's request count
// is the evidence that all N really went out.
func TestFuturesReadMethodsAreSafeForConcurrentUse(t *testing.T) {
	const workers = 8

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeFuturesQueryFundInfo): gatewaySuccess(futuresFundInfoWrappedBody),
	})
	exec := newWireExecutor(t, rec)
	svc := NewFuturesService(exec)

	type result struct {
		balance string
		err     error
	}
	results := make([]result, workers)
	start := make(chan struct{})
	done := make(chan struct{}, workers)
	for i := range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			<-start
			account, err := svc.QueryFundInfo(t.Context(), futuresFixtureAccount())
			if err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{balance: account.AssetBalance.String()}
		}()
	}
	close(start)
	for range workers {
		<-done
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("worker %d = %v, want nil", i, r.err)
			continue
		}
		if r.balance != "1000000" {
			t.Errorf("worker %d read %s, want 1000000: two callers shared one decoded reply", i, r.balance)
		}
	}
	if got := rec.count(string(client.RouteTradeFuturesQueryFundInfo)); got != workers {
		t.Errorf("requests = %d, want %d: a call that returned a result must have gone out", got, workers)
	}
}
