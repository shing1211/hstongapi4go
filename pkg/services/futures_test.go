// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ===========================================================================
// C5 + C6 - the three futures mutations
//
// C5 implemented them and C6 tests them in one change, deliberately. A2 found
// pkg/hstong/trade with twenty endpoints, a fully covered happy path, and not one
// test that made a Gateway call *fail*: splitting implement-then-test is what
// produces that shape, because the test is written against the code that was just
// finished and the failure surface is the part nobody has an excuse to skip.
//
// So the same discipline as the eight reads applies, with one thing added - a
// mutation's failure is money. The tables below therefore drive, for all three:
//
//   - the happy path, pinning the op, the route and the exact request body, so a
//     field swapped or a key misspelled fails here. The mock Gateway answers by
//     path and would not notice either.
//   - the local rejections, each at *zero* recorded calls, which is the only
//     thing that distinguishes "I built this wrong" from "the exchange said no".
//   - both error levels - the sequenced fake, and a real client over an ok:false
//     envelope - because a fake never builds a typed error.
//   - the ADR 0003 proof, with the read control *in the same test* so that
//     "exactly one request" cannot be a policy that never fired.
//   - the typed reconciliation error a caller branches on.
//   - hostile money, byte for byte, on the request side rather than the reply
//     side, because a mutation's money is a *write*.
//   - omitempty in the direction a public method can actually reach.
//
// No row sleeps: where a retry count is the assertion the policy installs
// MaxAttempts with a zero backoff, so the count is a property of the retry
// classification and not of elapsed time. No row recovers: a futures builder or
// mapper that panicked would take the process down, and the partial-reply rows
// below assert the opposite.
// ===========================================================================

// futuresFixtureEntrustID is the order the two id-bearing mutations name. It is
// the same id the mock's own futures payload carries, so the fixture and the
// order row cannot drift apart.
func futuresFixtureEntrustID() domain.EntrustID { return domain.EntrustID("F20260921001") }

// futuresFixtureOrder is a fully populated entrust.
//
// Every field is filled in, so a row cannot pass by reaching the executor and
// being rejected there, and so the one defaulted field (OrderOptions) is
// distinguishable from an absent one. ValidTime is set with a "0" time-in-force,
// which the coupling permits: a supplied date is format-checked and carried
// whatever the code is, and only a "4" *requires* one.
func futuresFixtureOrder() FuturesOrderRequest {
	return FuturesOrderRequest{
		Symbol:        futuresFixtureSymbol(),
		Side:          types.EntrustBuy,
		OrderType:     "0",
		Price:         domain.MustNewPrice("25000", "0"),
		Quantity:      domain.MustNewQuantity("1"),
		ValidTimeType: "0",
		ValidTime:     "20260930",
		OrderOptions:  "0",
	}
}

// futuresFixtureModify is a fully populated modify, and it exercises the two
// shapes the entrust fixture does not: a "4" time-in-force, so the date is
// *required* rather than merely carried, and a "1" order option.
func futuresFixtureModify() FuturesModifyRequest {
	return FuturesModifyRequest{
		EntrustID:     futuresFixtureEntrustID(),
		Symbol:        futuresFixtureSymbol(),
		Price:         domain.MustNewPrice("25100", "0"),
		Quantity:      domain.MustNewQuantity("2"),
		Side:          types.EntrustBuy,
		ValidTimeType: "4",
		ValidTime:     "20260930",
		OrderOptions:  "1",
	}
}

// futuresMutationCase is one row of every per-method table below. It is the read
// table's shape for the read table's reason: a new mutation cannot be added
// without a row here, and therefore cannot be added without a row in the error
// arms, the local rejections and the money hosts.
type futuresMutationCase struct {
	name string
	op   string
	// route is the endpoint the method must reach.
	route client.Route
	// wantParams is the request body the method must hand the executor, compared
	// with reflect.DeepEqual so a field added or dropped by mistake both fail.
	wantParams any
	// reply is the data object the sequencedExecutor answers with. It is a
	// json.RawMessage because the executor marshals it as a document; a []byte
	// would be marshalled as a base64 JSON string and every row would fail on a
	// decode error.
	reply json.RawMessage
	// invoke performs the call and returns its error, asserting the zero value
	// beside a failure against the concrete result type. That is the only way a
	// generic zero check means anything here: a caller must never be handed a
	// partially decoded payload alongside an error.
	invoke func(t *testing.T, ctx context.Context, svc *FuturesService) error
	// zeroAccount invokes the same method with a zero accountID and an otherwise
	// valid request, so the accountID row cannot pass by failing on some other
	// input first.
	zeroAccount func(t *testing.T, ctx context.Context, svc *FuturesService) error
}

// futuresMutationCases is the three futures mutations.
func futuresMutationCases() []futuresMutationCase {
	return []futuresMutationCase{
		{
			name:  "Entrust",
			op:    opFuturesEntrust,
			route: client.RouteTradeFuturesEntrust,
			wantParams: futuresEntrustWireRequest{
				StockCode:     "HSI2609.HK",
				EntrustType:   "0",
				EntrustPrice:  "25000",
				EntrustAmount: "1",
				EntrustBS:     "1",
				ValidTimeType: "0",
				OrderOptions:  "0",
				ValidTime:     "20260930",
			},
			reply: json.RawMessage(futuresMutationBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				result, err := svc.Entrust(ctx, futuresFixtureAccount(), futuresFixtureOrder())
				if err != nil {
					requireZero(t, result)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				result, err := svc.Entrust(ctx, domain.AccountID(""), futuresFixtureOrder())
				requireZero(t, result)
				return err
			},
		},
		{
			name:  "CancelEntrust",
			op:    opFuturesCancelEntrust,
			route: client.RouteTradeFuturesCancelEntrust,
			wantParams: futuresCancelEntrustWireRequest{
				EntrustID: "F20260921001",
				StockCode: "HSI2609.HK",
			},
			reply: json.RawMessage(futuresMutationBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				return svc.CancelEntrust(ctx, futuresFixtureAccount(), futuresFixtureEntrustID(),
					futuresFixtureSymbol())
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				return svc.CancelEntrust(ctx, domain.AccountID(""), futuresFixtureEntrustID(),
					futuresFixtureSymbol())
			},
		},
		{
			name:  "ModifyEntrust",
			op:    opFuturesModifyEntrust,
			route: client.RouteTradeFuturesModifyEntrust,
			wantParams: futuresModifyEntrustWireRequest{
				EntrustID:     "F20260921001",
				StockCode:     "HSI2609.HK",
				EntrustPrice:  "25100",
				EntrustAmount: "2",
				EntrustBS:     "1",
				ValidTimeType: "4",
				OrderOptions:  "1",
				ValidTime:     "20260930",
			},
			reply: json.RawMessage(futuresMutationBody),
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				return svc.ModifyEntrust(ctx, futuresFixtureAccount(), futuresFixtureModify())
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				return svc.ModifyEntrust(ctx, domain.AccountID(""), futuresFixtureModify())
			},
		},
	}
}

// futuresMutationCaseNamed returns the row for one method, failing the test if
// the name no longer resolves, so a renamed method cannot silently drop out of
// the single-method tables below.
func futuresMutationCaseNamed(t *testing.T, name string) futuresMutationCase {
	t.Helper()
	for _, tc := range futuresMutationCases() {
		if tc.name == name {
			return tc
		}
	}
	t.Fatalf("no mutation row named %q; futuresMutationCases has drifted", name)
	return futuresMutationCase{}
}

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// TestFuturesMutationsReachTheirRouteWithTheirOp is the positive path for all
// three: the op label and the route are the ones the method names, the request
// body is the one the call built, and exactly one call was recorded.
//
// It is also the control for every rejection table below. A validator that
// refused every input, or one whose check was accidentally inverted, would satisfy
// the rejection tables while making the three methods unusable; the recorded call
// here is the evidence that validation was really skipped rather than merely
// outvoted.
func TestFuturesMutationsReachTheirRouteWithTheirOp(t *testing.T) {
	for _, tc := range futuresMutationCases() {
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

// TestFuturesEntrustReturnsTheGatewayOrderNumber checks the one thing a mutation
// reply is for.
//
// The Gateway documents data as the resulting order number *or the empty string
// when it omits it*, so all three rows below are legitimate replies. What is
// pinned is that the empty one is reported as an empty id on a non-nil result
// rather than as nil, so a caller reading the id cannot mistake an omitted field
// for a failed call - and, as the GoDoc on Entrust says, must reconcile either
// way, because an absent order number is not evidence the order was refused.
func TestFuturesEntrustReturnsTheGatewayOrderNumber(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		want  string
	}{
		{"the Gateway supplied one", futuresMutationBody, "F20260921001"},
		{"the Gateway omitted it", `{"data":""}`, ""},
		{"the data member is absent entirely", `{}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(tc.reply)})
			result, err := NewFuturesService(exec).Entrust(t.Context(), futuresFixtureAccount(),
				futuresFixtureOrder())
			if err != nil {
				t.Fatalf("Entrust on %s = %v, want nil: a mutation's reply carries a field the "+
					"Gateway documents as optional, and its absence is not a failure to report", tc.reply, err)
			}
			if result == nil {
				t.Fatal("Entrust returned no result and no error, so a caller cannot tell an " +
					"omitted order number from a failed call")
			}
			if got := result.EntrustID.String(); got != tc.want {
				t.Errorf("EntrustID = %q, want %q", got, tc.want)
			}
			if result.Status != types.EntrustStatusWaitToRegister {
				t.Errorf("Status = %q, want %q: it is the state the host is in when the reply is "+
					"built, never a claim about the order's final state",
					result.Status, types.EntrustStatusWaitToRegister)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestFuturesMutationsRejectAZeroAccountIDWithoutARequest is the fail-closed row
// for all three.
//
// Zero recorded calls is the assertion that matters. accountID is a session key
// and never crosses the wire, so a zero-accountID request body is byte-identical
// to a valid one and only the absence of the call distinguishes them - which is
// why C2a §7.1 warns that adding accountID to a futures wire struct would leak
// an account identifier into every futures request while staying invisible on the
// mock.
func TestFuturesMutationsRejectAZeroAccountIDWithoutARequest(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.zeroAccount(t, t.Context(), NewFuturesService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestFuturesMutationAccountIDNeverReachesTheWire is the other direction of the
// same rule, driven over real HTTP so it is the bytes the Gateway would receive.
//
// C2a §7.1 calls the accountID leak "invisible on the mock", which is why this
// decodes the recorded request: the check is that no account-shaped key is
// present *and* that the account id appears nowhere in the envelope.
func TestFuturesMutationAccountIDNeverReachesTheWire(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			params := futuresRecordedParams(t, rec, string(tc.route))
			for key := range params {
				lower := strings.ToLower(key)
				for _, forbidden := range []string{"account", "accid", "userid", "clientid", "subaccount"} {
					if strings.Contains(lower, forbidden) {
						t.Errorf("%s sent the key %q: a futures request carries no account id, "+
							"because the account and the book both come from the session "+
							"(design-futures-requests §7.1)", tc.name, key)
					}
				}
			}
			if body := rec.lastBody(t, string(tc.route)); strings.Contains(body, string(futuresFixtureAccount())) {
				t.Errorf("%s put the account id on the wire: %s", tc.name, body)
			}
		})
	}
}

// TestFuturesMutationCarriesNoMarketField is the endpoint-level restatement of the
// struct-level rule in TestFuturesRequestsCarryNoMarketOrCursorField.
//
// A futures contract code is unique across the Hong Kong and US books, so no
// mutation sends a market - even though the caller supplied one on
// domain.Symbol.Market and even though the cash layer's mutation bodies both
// carry exchangeType. Adding it would be a wire change on a guess and an
// invisible one, since the mock answers all eleven futures routes by path.
func TestFuturesMutationCarriesNoMarketField(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			params := futuresRecordedParams(t, rec, string(tc.route))
			for key, raw := range params {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "exchange") || strings.Contains(lower, "market") ||
					lower == "datatype" {
					t.Errorf("%s sent %q = %s: no futures request carries a market, in either "+
						"vendor SDK or in the released pkg/hstong/future", tc.name, key, raw)
				}
			}
			// The market the caller did supply must not appear as a value either.
			if body := rec.lastBody(t, string(tc.route)); strings.Contains(body, "HSI2609.HK.HK") {
				t.Errorf("%s joined the market onto the contract code: %s", tc.name, body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local rejections
// ---------------------------------------------------------------------------

// futuresMutationLocalCases is the input-validation table for the three
// mutations.
//
// Every row asserts a typed rejection under the method's own op *and* zero
// recorded calls, and the table is driven with newSequencedExecutor(t) and no
// scripted reply, so a rejection that accidentally reached the executor fails
// through the fake's t.Fatalf naming the op rather than passing vacuously. That
// is C4's pattern for the reads, and it is what makes the zero-call assertion
// mean something rather than merely stated.
func futuresMutationLocalCases() []futuresLocalCase {
	id := futuresFixtureAccount()

	// out is declared before the row builders that append to it, because Go's
	// scoping puts a declaration's identifier in scope only from its own point on.
	var out []futuresLocalCase

	entrust := func(name string, mut func(*FuturesOrderRequest)) {
		out = append(out, futuresLocalCase{
			name: "Entrust: " + name,
			op:   opFuturesEntrust,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				req := futuresFixtureOrder()
				mut(&req)
				result, err := svc.Entrust(ctx, id, req)
				requireZero(t, result)
				return err
			},
		})
	}
	modifyRow := func(name string, mut func(*FuturesModifyRequest)) {
		out = append(out, futuresLocalCase{
			name: "ModifyEntrust: " + name,
			op:   opFuturesModifyEntrust,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				req := futuresFixtureModify()
				mut(&req)
				return svc.ModifyEntrust(ctx, id, req)
			},
		})
	}
	cancelRow := func(name string, mut func(*domain.EntrustID, *domain.Symbol)) {
		out = append(out, futuresLocalCase{
			name: "CancelEntrust: " + name,
			op:   opFuturesCancelEntrust,
			run: func(t *testing.T, ctx context.Context, svc *FuturesService) error {
				t.Helper()
				entrustID := futuresFixtureEntrustID()
				sym := futuresFixtureSymbol()
				mut(&entrustID, &sym)
				return svc.CancelEntrust(ctx, id, entrustID, sym)
			},
		})
	}

	// The contract code, on all three. futuresValidateStockCode is shared with the
	// reads, so these rows pin that the mutations really call it rather than
	// reading Code straight into the body.
	entrust("an empty contract code", func(r *FuturesOrderRequest) { r.Symbol = domain.Symbol{} })
	entrust("an interior-whitespace contract code", func(r *FuturesOrderRequest) {
		r.Symbol = domain.Symbol{Code: "HSI 2609"}
	})
	modifyRow("an empty contract code", func(r *FuturesModifyRequest) { r.Symbol = domain.Symbol{} })
	modifyRow("an interior-whitespace contract code", func(r *FuturesModifyRequest) {
		r.Symbol = domain.Symbol{Code: "HSI\t2609"}
	})
	cancelRow("an empty contract code", func(_ *domain.EntrustID, s *domain.Symbol) { *s = domain.Symbol{} })
	cancelRow("an interior-whitespace contract code", func(_ *domain.EntrustID, s *domain.Symbol) {
		*s = domain.Symbol{Code: "HSI 2609"}
	})

	// The order id on the two methods that name one, trimmed before testing.
	cancelRow("an empty order id", func(e *domain.EntrustID, _ *domain.Symbol) { *e = "" })
	cancelRow("an all-whitespace order id", func(e *domain.EntrustID, _ *domain.Symbol) { *e = "   " })
	cancelRow("a tab-only order id", func(e *domain.EntrustID, _ *domain.Symbol) { *e = "\t" })
	modifyRow("an empty order id", func(r *FuturesModifyRequest) { r.EntrustID = "" })
	modifyRow("an all-whitespace order id", func(r *FuturesModifyRequest) { r.EntrustID = "   " })

	// The order type: required and digit-shaped, and deliberately NOT bounded.
	for _, orderType := range []string{"", "limit", "0 ", " 0", "-1", "0x0"} {
		entrust("orderType "+strconv.Quote(orderType), func(r *FuturesOrderRequest) {
			r.OrderType = orderType
		})
	}

	// The direction, on both methods that carry one.
	for _, side := range []types.EntrustBS{"", "0", "5", "9", "buy", "-1"} {
		entrust("side "+strconv.Quote(string(side)), func(r *FuturesOrderRequest) { r.Side = side })
	}
	for _, side := range []types.EntrustBS{"", "0", "7", "sell"} {
		modifyRow("side "+strconv.Quote(string(side)), func(r *FuturesModifyRequest) { r.Side = side })
	}

	// The price: required, with no market-order waiver, so a negative is the one
	// failure a domain.Price can still express.
	entrust("a negative price", func(r *FuturesOrderRequest) { r.Price = domain.MustNewPrice("-1", "0") })
	entrust("a price below one", func(r *FuturesOrderRequest) {
		r.Price = domain.MustNewPrice("-0.0001", "0")
	})
	modifyRow("a negative price", func(r *FuturesModifyRequest) {
		r.Price = domain.MustNewPrice("-0.5", "0")
	})

	// The quantity: strictly positive, so zero and negative both fail.
	entrust("a zero quantity", func(r *FuturesOrderRequest) { r.Quantity = domain.MustNewQuantity("0") })
	entrust("a negative quantity", func(r *FuturesOrderRequest) {
		r.Quantity = domain.MustNewQuantity("-1")
	})
	modifyRow("a zero quantity", func(r *FuturesModifyRequest) { r.Quantity = domain.MustNewQuantity("0") })
	modifyRow("a negative quantity", func(r *FuturesModifyRequest) {
		r.Quantity = domain.MustNewQuantity("-2")
	})

	// The time-in-force and the date it carries.
	for _, vtt := range []string{"", "5", "6", "-1", "day", " 0"} {
		entrust("validTimeType "+strconv.Quote(vtt), func(r *FuturesOrderRequest) {
			r.ValidTimeType = vtt
		})
		modifyRow("validTimeType "+strconv.Quote(vtt), func(r *FuturesModifyRequest) {
			r.ValidTimeType = vtt
		})
	}
	entrust(`validTimeType "4" with no date`, func(r *FuturesOrderRequest) {
		r.ValidTimeType = "4"
		r.ValidTime = ""
	})
	modifyRow(`validTimeType "4" with no date`, func(r *FuturesModifyRequest) {
		r.ValidTimeType = "4"
		r.ValidTime = ""
	})
	entrust("a validTime that is not eight digits", func(r *FuturesOrderRequest) {
		r.ValidTime = "2026-09-30"
	})
	entrust("a validTime shaped but not a calendar date", func(r *FuturesOrderRequest) {
		r.ValidTime = "20260230"
	})
	entrust("a validTime that is a leap day on a non-leap year", func(r *FuturesOrderRequest) {
		r.ValidTime = "20260229"
	})
	modifyRow("a validTime that is not eight digits", func(r *FuturesModifyRequest) {
		r.ValidTime = "2026093"
	})

	// The order option, checked *after* the mapping boundary substitutes "0", so
	// only a non-empty out-of-set value is a rejection.
	for _, oo := range []string{"2", "-1", "01", "T+1", " 0"} {
		entrust("orderOptions "+strconv.Quote(oo), func(r *FuturesOrderRequest) { r.OrderOptions = oo })
		modifyRow("orderOptions "+strconv.Quote(oo), func(r *FuturesModifyRequest) {
			r.OrderOptions = oo
		})
	}

	return out
}

// TestFuturesMutationsRejectBadInputWithoutARequest is the input-validation arm:
// a bad code, direction, price, quantity, time-in-force or order option is
// refused locally, under the method's own op, at zero HTTP cost.
//
// The zero-call assertion is the load-bearing half and it is only credible
// because the executor is built with no scripted reply: had a rejection ever
// reached the executor, the fake's t.Fatalf would fire naming the op, instead of
// the row quietly passing against a fixture the code never consulted. That is the
// gap A2 found in pkg/hstong/trade, and it is closed here by construction rather
// than by care.
func TestFuturesMutationsRejectBadInputWithoutARequest(t *testing.T) {
	for _, tc := range futuresMutationLocalCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.run(t, t.Context(), NewFuturesService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestFuturesMutationsAcceptTheDocumentedBoundaries is the accepting side of the
// table above, because a validator that refused everything would satisfy the table
// while making the three methods unusable.
//
// Four rows are worth naming. "0" is the recommended market-order price and must
// be accepted even though the released layer waived the price entirely for a
// market order - that is design-futures-requests §4.3 recommendation (a), and
// "0" is a well-formed non-negative decimal needing no special case. A
// specified-date order is accepted when it carries a real date. The leap day is
// accepted on a leap year, so the calendar check is a calendar and not a length
// check. And an empty order option is *accepted* and becomes the documented "0" -
// the one defaulted field on either body, and the only always-sent field whose
// value a caller may legitimately leave unset.
func TestFuturesMutationsAcceptTheDocumentedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*FuturesOrderRequest)
		wantParams futuresEntrustWireRequest
	}{
		{
			name: `a market order carries "0" and no date`,
			mutate: func(r *FuturesOrderRequest) {
				r.OrderType = "2"
				r.Price = domain.MustNewPrice("0", "0")
				r.ValidTimeType = "0"
				r.ValidTime = ""
			},
			wantParams: futuresEntrustWireRequest{
				StockCode: "HSI2609.HK", EntrustType: "2", EntrustPrice: "0",
				EntrustAmount: "1", EntrustBS: "1", ValidTimeType: "0", OrderOptions: "0",
			},
		},
		{
			name: "the leap day is a real date",
			mutate: func(r *FuturesOrderRequest) {
				r.ValidTimeType = "4"
				r.ValidTime = "20240229"
			},
			wantParams: futuresEntrustWireRequest{
				StockCode: "HSI2609.HK", EntrustType: "0", EntrustPrice: "25000",
				EntrustAmount: "1", EntrustBS: "1", ValidTimeType: "4", OrderOptions: "0",
				ValidTime: "20240229",
			},
		},
		{
			name:   "an empty order option becomes the documented 0",
			mutate: func(r *FuturesOrderRequest) { r.OrderOptions = "" },
			wantParams: futuresEntrustWireRequest{
				StockCode: "HSI2609.HK", EntrustType: "0", EntrustPrice: "25000",
				EntrustAmount: "1", EntrustBS: "1", ValidTimeType: "0", OrderOptions: "0",
				ValidTime: "20260930",
			},
		},
		{
			name:   "a short quantity is legal and is not rounded",
			mutate: func(r *FuturesOrderRequest) { r.Quantity = domain.MustNewQuantity("0.5") },
			wantParams: futuresEntrustWireRequest{
				StockCode: "HSI2609.HK", EntrustType: "0", EntrustPrice: "25000",
				EntrustAmount: "0.5", EntrustBS: "1", ValidTimeType: "0", OrderOptions: "0",
				ValidTime: "20260930",
			},
		},
		{
			name:   "a padded code is passed through verbatim, as the released layer does",
			mutate: func(r *FuturesOrderRequest) { r.Symbol = domain.Symbol{Code: " HSI2609.HK "} },
			wantParams: futuresEntrustWireRequest{
				StockCode: " HSI2609.HK ", EntrustType: "0", EntrustPrice: "25000",
				EntrustAmount: "1", EntrustBS: "1", ValidTimeType: "0", OrderOptions: "0",
				ValidTime: "20260930",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(futuresMutationBody)})
			req := futuresFixtureOrder()
			tc.mutate(&req)

			if _, err := NewFuturesService(exec).Entrust(t.Context(), futuresFixtureAccount(), req); err != nil {
				t.Fatalf("Entrust with %+v = %v, want nil", req, err)
			}
			requireCalls(t, exec, 1)
			if got := exec.lastParams(t); !reflect.DeepEqual(got, tc.wantParams) {
				t.Errorf("the request body = %#v, want %#v", got, tc.wantParams)
			}
		})
	}
}

// TestFuturesEntrustBSKeepsTheReleasedFour is the pin design-futures-requests
// §9.3 asks for, in the spirit of A6's TestEveryDocumentedCodeIsAccepted.
//
// The set was disputed: the vendor's *futures* enum has two values and SPEC §7.4
// plus the released pkg/hstong/future had four. §9.3 decided to keep four and NOT
// to narrow, because narrowing is a caller-visible change on a money-moving field
// decided on incomplete evidence, and because being permissive fails as a clear
// Gateway rejection rather than a misroute. A test that pinned {3,4} as accepted
// is what stops a future narrowing attempt succeeding quietly on the strength of a
// plausible reading of the vendor enum.
//
// That pin has now done its job in the other direction. C1b was resolved on
// 2026-09-29 from the vendor's own per-endpoint documentation, which shows the two
// sets never conflicted because they govern *different endpoints*: the cash page
// offers 3 and 4 as explicit input values, the futures page lists only
// "1:买入,2:卖出". So this is renamed and inverted - the futures endpoint is
// {1,2}, fail-closed, and 3/4 are asserted to be *refused* rather than accepted.
// The pin is not deleted; it is pointed the other way, which is the whole reason
// it was written as a test rather than a comment.
func TestFuturesEntrustBSIsTheTwoValueFuturesSet(t *testing.T) {
	for _, side := range []types.EntrustBS{types.EntrustBuy, types.EntrustSell} {
		t.Run(string(side), func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeFuturesEntrust): gatewaySuccess(futuresMutationBody),
			})
			recMod := newWireRecorder(map[string]string{
				string(client.RouteTradeFuturesModifyEntrust): gatewaySuccess(futuresMutationBody),
			})
			req := futuresFixtureOrder()
			req.Side = side
			if _, err := NewFuturesService(newWireExecutor(t, rec)).Entrust(
				t.Context(), futuresFixtureAccount(), req); err != nil {
				t.Errorf("Entrust with side %q = %v, want nil: the vendor documents "+
					"1:买入,2:卖出 for POST /trade/FuturesEntrust", side, err)
			}
			mod := futuresFixtureModify()
			mod.Side = side
			if err := NewFuturesService(newWireExecutor(t, recMod)).ModifyEntrust(
				t.Context(), futuresFixtureAccount(), mod); err != nil {
				t.Errorf("ModifyEntrust with side %q = %v, want nil, for the reason above", side, err)
			}
		})
	}

	// The 3/4 case, which is the half this change exists for. Both must be
	// refused locally, with no request sent, and the message must say why
	// rather than just "invalid".
	for _, side := range []types.EntrustBS{types.EntrustCloseShort, types.EntrustOpenShort, "3", "4"} {
		t.Run("rejects_"+string(side), func(t *testing.T) {
			rec := newWireRecorder(nil)
			req := futuresFixtureOrder()
			req.Side = side
			_, err := NewFuturesService(newWireExecutor(t, rec)).Entrust(
				t.Context(), futuresFixtureAccount(), req)
			if err == nil {
				t.Fatalf("Entrust with side %q was accepted, but the futures endpoint "+
					"documents only 1 and 2; 3 and 4 are cash values (docs/SPEC.md 7.4, C1b)", side)
			}
			if !strings.Contains(err.Error(), "cash-only") {
				t.Errorf("rejection for side %q = %q, want a message naming the cash-only "+
					"values so the caller knows the difference is the endpoint, not the code", side, err)
			}
			if got := rec.total(); got != 0 {
				t.Errorf("a refused direction still sent %d request(s); it must fail closed "+
					"before the wire", got)
			}

			recMod := newWireRecorder(nil)
			mod := futuresFixtureModify()
			mod.Side = side
			if err := NewFuturesService(newWireExecutor(t, recMod)).ModifyEntrust(
				t.Context(), futuresFixtureAccount(), mod); err == nil {
				t.Errorf("ModifyEntrust with side %q was accepted, want refused", side)
			}
		})
	}
}

// TestFuturesEntrustTypeIsNotValueBounded is the mirror pin, for §9.2.
//
// The two vendor SDKs, both v2.3.0 shipped the same day, disagree on whether a
// fourth order-type code exists: Java declares four including OPTION("3") and
// Python declares exactly three. A local closed set would therefore reject a code
// one of them documents, so the SDK validates digit-ness and requiredness and
// stops. This test fails if someone narrows it, and fails if the digit check is
// dropped - the two directions of the same rule.
func TestFuturesEntrustTypeIsNotValueBounded(t *testing.T) {
	// Every digit run is accepted, including the codes no single vendor SDK
	// documents and the ones a caller may already be sending in production.
	for _, orderType := range []string{"0", "1", "2", "3", "7", "42", "007", "99999999999999999999"} {
		t.Run("accepted: "+orderType, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeFuturesEntrust): gatewaySuccess(futuresMutationBody),
			})
			req := futuresFixtureOrder()
			req.OrderType = orderType
			if _, err := NewFuturesService(newWireExecutor(t, rec)).Entrust(
				t.Context(), futuresFixtureAccount(), req); err != nil {
				t.Errorf("Entrust with orderType %q = %v, want nil: the two vendor SDKs disagree "+
					"on the set, so a local closed set would refuse a code one of them documents "+
					"(design-futures-requests §9.2)", orderType, err)
			}
		})
	}
	// And the digit check itself is not decorative: these are all refused.
	exec := newSequencedExecutor(t)
	req := futuresFixtureOrder()
	req.OrderType = "limit"
	_, err := NewFuturesService(exec).Entrust(t.Context(), futuresFixtureAccount(), req)
	assertInvalidParam(t, err, opFuturesEntrust)
	requireCalls(t, exec, 0)
}

// ---------------------------------------------------------------------------
// Transport error arms
// ---------------------------------------------------------------------------

// errFuturesMutationDown is the sentinel the Level 1 mutation rows return. It is
// compared with errors.Is and never on a rendered message.
var errFuturesMutationDown = errors.New("futures: scripted mutation failure")

// TestFuturesMutationExecutorFailurePropagates is Level 1: the executor's error
// reaches the caller unchanged, the zero value comes back beside it, and exactly
// one call was recorded.
//
// It is also the guard against a service growing a retry loop of its own. The
// fake has no retry logic, so "the fake recorded one call" is trivially true
// against it; what this row really rules out is a second Do.
func TestFuturesMutationExecutorFailurePropagates(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t,
				sequencedReply{err: errFuturesMutationDown},
				sequencedReply{err: errFuturesMutationDown},
			)

			err := tc.invoke(t, t.Context(), NewFuturesService(exec))
			if !errors.Is(err, errFuturesMutationDown) {
				t.Fatalf("%s = %v, want the executor's own error", tc.name, err)
			}
			futuresExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)
		})
	}
}

// TestFuturesMutationGatewayRejectionArrivesTyped is Level 2: a real ok:false
// envelope becomes a typed *errs.Error carrying the Gateway's code, its category
// and the service's own op.
//
// This is the row that proves the mutations do not launder a typed Gateway error
// into an opaque one - something a fake cannot show, because a fake never builds a
// typed error at all. It matters more on a mutation than on a read: a caller
// deciding whether to re-establish a futures session branches on Op, and a
// mutation that filled in the wrong one would be attributed to a route it never
// called, on the one call the caller cannot safely retry.
func TestFuturesMutationGatewayRejectionArrivesTyped(t *testing.T) {
	const code = types.StatusServiceBusy
	const text = "service busy, retry later"

	for _, tc := range futuresMutationCases() {
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
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want 1: the mutation must not have issued a "+
					"second request on any other path", got)
			}
		})
	}
}

// TestFuturesMutationReconciliationErrorIsTypedAndRecoverable is the
// reconciliation half of ADR 0003, and it is about the *error* rather than the
// count.
//
// "1007 duplicate submission" is the code that exists precisely because of the
// ambiguity ADR 0003 describes: the request may have reached the platform and
// timed out before the reply was read, so the true state is unknown. ADR 0003
// requires the SDK to return a typed error telling the caller to reconcile, and
// requires it to be recoverable - which is a stronger claim than "some error came
// back". Four properties are asserted per method:
//
//   - the Gateway's own code survives, so a caller can tell a duplicate from a
//     rejection for any other reason;
//   - the category is errs.CategoryTrading, the one the code table assigns to
//     1007, so a caller can branch on the *kind* of failure without matching on
//     a rendered message;
//   - errors.As reaches an *errs.Error and errors.Is still traverses the chain,
//     so a caller that wrapped the error on its way out has not lost the
//     classification;
//   - the Op is the method's own, which is how a caller decides which
//     reconciliation query to run.
//
// The failure direction is also asserted: a service that wrapped this in its own
// opaque type would keep the count at one and lose every one of these.
func TestFuturesMutationReconciliationErrorIsTypedAndRecoverable(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection the caller must reconcile", tc.name, code)
			}

			typed := errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a %s is a trading-state failure the caller "+
					"must reconcile before resubmitting, and it is the one code that exists "+
					"because of the ambiguity (docs/adr/0003-no-auto-retry-orders.md)",
					got, errs.CategoryTrading, code)
			}
			if typed.Category != errs.CategoryTrading {
				t.Errorf("Error.Category = %q, want %q on the error itself and not only on the "+
					"CategoryOf projection", typed.Category, errs.CategoryTrading)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			// errors.As must reach the typed error, and it must be reachable
			// through a wrapper the caller adds - which is the "recoverable" half.
			var viaAs *errs.Error
			if !errors.As(err, &viaAs) {
				t.Errorf("errors.As(%T) yielded no *errs.Error", err)
			}
			wrapped := fmt.Errorf("reconcile after: %w", err)
			if !errors.As(wrapped, &viaAs) {
				t.Error("errors.As yielded no *errs.Error through a caller-supplied wrapper, so " +
					"the classification is lost the moment a caller annotates the error")
			}
			// The negative counterpart: a 1007 is a trading state, not a session
			// failure, so a caller that re-logs-in on the wrong signal must not be
			// misled into doing so and losing the order it is trying to reconcile.
			if errs.ReLoginRequired(wrapped) {
				t.Errorf("ReLoginRequired(%v) = true; a duplicate submission is a trading state, "+
					"not a session failure", wrapped)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1", got)
			}
		})
	}
}

// TestFuturesMutationsAreSentExactlyOnceUnderARetryPolicy is the ADR 0003 proof
// for the three futures mutations, and it carries its own control.
//
// Three things make it non-obvious at this layer, and each is a way the assertion
// could pass for the wrong reason.
//
//  1. The guarantee lives in client, not here. client.Client.execute derives the
//     retry class from the route path and consults resilience.IsMutation, which
//     issues one attempt for that class regardless of the policy. A fake executor
//     has no retry logic at all, so "the fake recorded one call" is trivially true
//     and says nothing about retries.
//
//  2. The proof therefore needs a real *client.Client over an httptest server and
//     a retryable rejection, so the policy has every reason to fire. "1011 service
//     busy" is the code errs.Retryable reports as retryable. BaseBackoff is 0, so
//     the request count is a property of the classification and not of elapsed
//     time - the discipline internal/push needed after its reconnect tests made a
//     coverage figure a coin flip. Nothing here points at 127.0.0.1:11111.
//
//  3. A single request is also what a client with *no* retry policy produces, and
//     also what a policy that was never applied produces. So the same test drives a
//     read-only futures route under the identical client options and the identical
//     rejection and shows it taking all five attempts. Without that control the
//     three mutation rows would pass against a policy that did nothing, and a test
//     that cannot fail is worse than no test because it reads as coverage.
//
// Every invocation uses a valid request, which is load-bearing: Entrust validates
// before it calls the executor, so an invalid fixture would record zero requests -
// and zero would look like "no attempt at all" rather than "exactly one". The
// assertions therefore pair rec.count(route) == 1 with rec.total() == 1, and
// TestFuturesMutationsRejectBadInputWithoutARequest covers the other case.
func TestFuturesMutationsAreSentExactlyOnceUnderARetryPolicy(t *testing.T) {
	const maxAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")

	// Every path answers busy, not just the one under test, so a service that
	// quietly issued a second request somewhere else would show up in total().
	paths := make(map[string]string, len(futuresMutationCases())+1)
	for _, tc := range futuresMutationCases() {
		paths[string(tc.route)] = busy
	}
	// The control route: a read-only futures query, which must be retryable.
	const controlRoute = client.RouteTradeFuturesQueryRealEntrustList
	paths[string(controlRoute)] = busy

	// Prove the case is retryable before relying on it. Otherwise "one attempt"
	// would be true for the boring reason that nothing wanted a second one.
	if !errs.Retryable(errs.New(types.StatusServiceBusy, opFuturesEntrust, "service busy, retry later")) {
		t.Fatalf("test bug: %q is not retryable, so a single request would be expected even "+
			"without ADR 0003", types.StatusServiceBusy)
	}

	for _, tc := range futuresMutationCases() {
		t.Run(tc.name+" issues exactly one attempt", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewFuturesService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			}
			// The typed error is asserted, not just the count: a caller reconciling
			// an ambiguous order needs to know the exchange was busy, not that the
			// SDK gave up.
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}

			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want exactly 1: ADR 0003 permits one attempt for "+
					"an order mutation at any policy configuration, and this one allowed %d",
					tc.route, got, maxAttempts)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1: the mutation must not have issued a "+
					"second request on any other path", got)
			}
			t.Logf("%s: %d request under MaxAttempts %d", tc.route, rec.total(), maxAttempts)
		})
	}

	// The control, in the same test and under the same policy, so the two numbers
	// are directly comparable: five for a read, one for each mutation.
	t.Run("the read control retries under the identical policy", func(t *testing.T) {
		rec := newWireRecorder(paths)
		svc := NewFuturesService(newWireExecutor(t, rec,
			client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

		_, err := svc.QueryRealEntrustList(t.Context(), futuresFixtureAccount())
		if err == nil {
			t.Fatalf("QueryRealEntrustList = nil error, want the %q rejection", types.StatusServiceBusy)
		}
		errRejects(t, err, types.StatusServiceBusy, opFuturesQueryRealEntrustList)

		if got := rec.count(string(controlRoute)); got != maxAttempts {
			t.Fatalf("requests to %s = %d, want %d. This control is what makes the three "+
				"single-request rows above mean something: without it, a client whose policy "+
				"was never applied would produce exactly the same one request.",
				controlRoute, got, maxAttempts)
		}
		if got := rec.total(); got != maxAttempts {
			t.Errorf("total requests = %d, want %d", got, maxAttempts)
		}
		t.Logf("CONTROL %s: %d requests under MaxAttempts %d (mutations: 1 each)",
			controlRoute, rec.total(), maxAttempts)
	})
}

// TestFuturesMutationAttemptCountIsIndependentOfThePolicySize widens the
// guarantee from "one policy" to "every policy": a mutation issues one attempt
// whatever the caller configures, including a budget below one.
//
// resilience.Policy treats a MaxAttempts below one as one, so the smallest row is
// a floor rather than a distinct case, and it is here to make that floor explicit.
func TestFuturesMutationAttemptCountIsIndependentOfThePolicySize(t *testing.T) {
	paths := make(map[string]string)
	for _, tc := range futuresMutationCases() {
		paths[string(tc.route)] = gatewayFailure(types.StatusServiceBusy, "busy")
	}

	for _, maxAttempts := range []int{0, 1, 2, 5, 20} {
		t.Run("MaxAttempts "+strconv.Itoa(maxAttempts), func(t *testing.T) {
			for _, tc := range futuresMutationCases() {
				t.Run(tc.name, func(t *testing.T) {
					rec := newWireRecorder(paths)
					svc := NewFuturesService(newWireExecutor(t, rec,
						client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

					if err := tc.invoke(t, t.Context(), svc); err == nil {
						t.Fatalf("%s = nil error, want the rejection", tc.name)
					}
					if got := rec.total(); got != 1 {
						t.Errorf("%s with MaxAttempts %d produced %d requests, want 1",
							tc.name, maxAttempts, got)
					}
				})
			}
		})
	}
}

// TestFuturesMutationCallIsCancellable pins that a cancelled context is reported
// as a cancellation and never as a success, matched through errors.Is on
// context.Canceled rather than on a rendered message.
//
// All three are single-shot, so each costs at most one recorded request: each
// hands the context to the executor and lets the request path report the
// cancellation. A caller who gives up on the context must not be able to turn one
// attempt into several, and the error it gets back must still be the
// cancellation rather than a Gateway status the SDK made up on the way out.
func TestFuturesMutationCallIsCancellable(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(types.StatusServiceBusy, "busy"),
			})
			svc := NewFuturesService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0})))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(t, ctx, svc)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
				t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
			}
			if got := rec.total(); got > 1 {
				t.Errorf("%s issued %d requests before the cancellation took effect, want at most 1",
					tc.name, got)
			}
		})
	}
}

// TestFuturesMutationMalformedReplyIsReportedAndNotDressedAsAGatewayCode covers the
// third way a mutation can fail: the Gateway says ok:true and then sends a data
// member the endpoint cannot decode.
//
// A mutation that ignored the decode failure would return a nil error and an
// empty result, and the caller would read that as an order placed. The zero value
// is asserted, and so is the absence of a status code - a local transport failure
// must not be dressed as a Gateway code, or a caller retrying on a code would
// chase a fault the Gateway never reported. The Op is still filled in, because
// that is what a caller branches on to decide whether to re-authenticate.
func TestFuturesMutationMalformedReplyIsReportedAndNotDressedAsAGatewayCode(t *testing.T) {
	for _, tc := range futuresMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(`42`),
			})
			svc := NewFuturesService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s on a data member of 42 = nil error; the caller would read the "+
					"empty result as an order placed", tc.name)
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
// omitempty, at the level a public method can reach
// ---------------------------------------------------------------------------

// futuresEntrustAlwaysSentKeys, futuresCancelAlwaysSentKeys and
// futuresModifyAlwaysSentKeys are the always-sent key sets of the three mutation
// bodies, written out here rather than derived from the structs so the assertion
// is against the vendor's request literal and not against the code under test.
//
// Entrust and ModifyEntrust share six always-sent keys; entrustType belongs to the
// entrust alone, because a modify must not retype an order, and entrustId belongs
// to the two id-bearing bodies. validTime is deliberately in none of them: it is
// the one conditional field on any futures request, and its presence is a property
// of the time-in-force code rather than of the body.
var (
	futuresEntrustAlwaysSentKeys = []string{
		"entrustAmount", "entrustBs", "entrustPrice", "entrustType",
		"orderOptions", "stockCode", "validTimeType",
	}
	futuresCancelAlwaysSentKeys = []string{"entrustId", "stockCode"}
	futuresModifyAlwaysSentKeys = []string{
		"entrustAmount", "entrustBs", "entrustId", "entrustPrice",
		"orderOptions", "stockCode", "validTimeType",
	}
)

// TestFuturesMutationAlwaysSendsItsRequiredFields is the method-level half of
// design-futures-requests §6.1's omitempty rule, and it is the half C3's
// struct-level TestFuturesRequestOmitemptyInBothDirections could not reach.
//
// C3 pinned the rule on a marshalled struct, which proves the tag. This proves the
// tag *through a public method over real HTTP*, where a default substitution or a
// validation default could have quietly changed the key set. It asserts three
// directions, and the second is the interesting one:
//
//  1. Every always-sent key is present on a well-formed request, with the value the
//     method was given. This is what no-omitempty buys, and it is the observable
//     difference from the released pkg/hstong/future body, which tags entrustType,
//     entrustPrice, validTimeType and orderOptions omitempty. The bytes are
//     identical for a well-formed request, so a future editor "fixing" the tags
//     back would remove only this coverage.
//  2. An unset orderOptions - the one field a caller may legitimately leave empty -
//     still goes on the wire as the documented "0". **That is the present-but-empty
//     case**, and it is the only one reachable through a public method: the other
//     three cannot be empty, because a request that would send them empty is
//     refused locally at zero cost. That half is asserted separately in
//     TestFuturesMutationEmptyRequiredFieldNeverReachesTheWire.
//  3. validTime is dropped when the time-in-force is not a specified date and
//     present when it is. An omitempty rule asserted in one direction only is half
//     a rule, and this is the other half.
func TestFuturesMutationAlwaysSendsItsRequiredFields(t *testing.T) {
	t.Run("Entrust carries every always-sent key", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeFuturesEntrust): gatewaySuccess(futuresMutationBody),
		})
		req := futuresFixtureOrder()
		if _, err := NewFuturesService(newWireExecutor(t, rec)).Entrust(
			t.Context(), futuresFixtureAccount(), req); err != nil {
			t.Fatalf("Entrust = %v, want nil", err)
		}
		params := futuresRecordedParams(t, rec, string(client.RouteTradeFuturesEntrust))
		futuresRequireKeys(t, "Entrust body", params,
			append(append([]string(nil), futuresEntrustAlwaysSentKeys...), "validTime")...)
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
	})

	t.Run("CancelEntrust carries both of its keys", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeFuturesCancelEntrust): gatewaySuccess(futuresMutationBody),
		})
		if err := NewFuturesService(newWireExecutor(t, rec)).CancelEntrust(
			t.Context(), futuresFixtureAccount(), futuresFixtureEntrustID(),
			futuresFixtureSymbol()); err != nil {
			t.Fatalf("CancelEntrust = %v, want nil", err)
		}
		params := futuresRecordedParams(t, rec, string(client.RouteTradeFuturesCancelEntrust))
		futuresRequireKeys(t, "CancelEntrust body", params, futuresCancelAlwaysSentKeys...)
		futuresRequireString(t, params, "entrustId", "F20260921001")
		futuresRequireString(t, params, "stockCode", "HSI2609.HK")
	})

	t.Run("ModifyEntrust carries every always-sent key and no entrustType", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeFuturesModifyEntrust): gatewaySuccess(futuresMutationBody),
		})
		if err := NewFuturesService(newWireExecutor(t, rec)).ModifyEntrust(
			t.Context(), futuresFixtureAccount(), futuresFixtureModify()); err != nil {
			t.Fatalf("ModifyEntrust = %v, want nil", err)
		}
		params := futuresRecordedParams(t, rec, string(client.RouteTradeFuturesModifyEntrust))
		futuresRequireKeys(t, "ModifyEntrust body", params,
			append(append([]string(nil), futuresModifyAlwaysSentKeys...), "validTime")...)
		for key, want := range map[string]string{
			"entrustId":     "F20260921001",
			"stockCode":     "HSI2609.HK",
			"entrustPrice":  "25100",
			"entrustAmount": "2",
			"entrustBs":     "1",
			"validTimeType": "4",
			"orderOptions":  "1",
			"validTime":     "20260930",
		} {
			futuresRequireString(t, params, key, want)
		}
		// The mirror-image row of the key set, restated at the method level: a
		// modify must not retype an order, and the type of check is presence.
		if _, ok := params["entrustType"]; ok {
			t.Error("the modify body carries an entrustType key on the wire; the Gateway's " +
				"modify body has no such field")
		}
	})

	t.Run("an unset order option is sent, not dropped", func(t *testing.T) {
		for _, name := range []string{"Entrust", "ModifyEntrust"} {
			t.Run(name, func(t *testing.T) {
				tc := futuresMutationCaseNamed(t, name)
				rec := newWireRecorder(map[string]string{
					string(tc.route): gatewaySuccess(string(tc.reply)),
				})
				order := futuresFixtureOrder()
				order.OrderOptions = ""
				modify := futuresFixtureModify()
				modify.OrderOptions = ""

				svc := NewFuturesService(newWireExecutor(t, rec))
				var err error
				if name == "Entrust" {
					_, err = svc.Entrust(t.Context(), futuresFixtureAccount(), order)
				} else {
					err = svc.ModifyEntrust(t.Context(), futuresFixtureAccount(), modify)
				}
				if err != nil {
					t.Fatalf("%s with an empty order option = %v, want nil: an empty order option "+
						"is a caller who wants the Gateway's default, not a malformed request", name, err)
				}
				params := futuresRecordedParams(t, rec, string(tc.route))
				raw, ok := params["orderOptions"]
				if !ok {
					t.Fatalf("the orderOptions key is absent: it carries no omitempty, so the key "+
						"is emitted whatever the caller supplied. body: %s",
						rec.lastBody(t, string(tc.route)))
				}
				if string(raw) != `"0"` {
					t.Errorf("orderOptions on the wire = %s, want the quoted default \"0\"", raw)
				}
			})
		}
	})

	t.Run("validTime is present or absent according to the time-in-force", func(t *testing.T) {
		// Both mutation bodies, and both directions. The mirror-image omission here is
		// real rather than theoretical: a driver that dropped omitempty from the
		// *modify* validTime survived the entrust-only version of this row, because
		// every assertion about a dropped date was made through Entrust. A rule
		// asserted on one body is half a rule even when it is asserted on the right
		// body.
		for _, name := range []string{"Entrust", "ModifyEntrust"} {
			tc := futuresMutationCaseNamed(t, name)
			for _, row := range []struct {
				name          string
				validTimeType string
				validTime     string
				wantPresent   bool
			}{
				{"a day order drops the date", "0", "", false},
				{"a specified-date order carries the date", "4", "20260930", true},
				{"a non-specified code with a date still carries it", "0", "20260930", true},
			} {
				t.Run(name+": "+row.name, func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(tc.route): gatewaySuccess(string(tc.reply)),
					})
					order := futuresFixtureOrder()
					order.ValidTimeType = row.validTimeType
					order.ValidTime = row.validTime
					mod := futuresFixtureModify()
					mod.ValidTimeType = row.validTimeType
					mod.ValidTime = row.validTime

					svc := NewFuturesService(newWireExecutor(t, rec))
					var err error
					if name == "Entrust" {
						_, err = svc.Entrust(t.Context(), futuresFixtureAccount(), order)
					} else {
						err = svc.ModifyEntrust(t.Context(), futuresFixtureAccount(), mod)
					}
					if err != nil {
						t.Fatalf("%s = %v, want nil", name, err)
					}

					params := futuresRecordedParams(t, rec, string(tc.route))
					_, present := params["validTime"]
					if present != row.wantPresent {
						t.Fatalf("validTime present = %v, want %v; body: %s", present, row.wantPresent,
							rec.lastBody(t, string(tc.route)))
					}
					if row.wantPresent {
						futuresRequireString(t, params, "validTime", row.validTime)
						return
					}
					// Everything else is unaffected by the drop: an omitted key must not
					// take a neighbour with it.
					if name == "Entrust" {
						futuresRequireKeys(t, "Entrust body", params, futuresEntrustAlwaysSentKeys...)
					} else {
						futuresRequireKeys(t, "ModifyEntrust body", params, futuresModifyAlwaysSentKeys...)
					}
				})
			}
		}
	})
}

// TestFuturesMutationEmptyRequiredFieldNeverReachesTheWire is the other way to
// state the omitempty rule on a mutation, and it is the stronger of the two.
//
// A present-but-empty required field is only ever *emitted* when a caller builds
// one; through a public method the request is refused locally first, at zero HTTP
// cost. So the reachable property is not "an empty required field is sent empty" -
// it is "a request that would send one is never sent". Each row below is the
// method-level counterpart of a key in futuresEntrustAlwaysSentKeys, and each
// asserts zero requests, which is the property that makes local validation safe on
// an order: a request this SDK refused provably had no side effect.
//
// The other direction is not lost. C3's struct-level
// TestFuturesRequestOmitemptyInBothDirections still marshals an all-zero
// futuresEntrustWireRequest and asserts the seven keys are present and empty,
// which is the tag's behaviour; this is the method's.
func TestFuturesMutationEmptyRequiredFieldNeverReachesTheWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   string
		run  func(t *testing.T, ctx context.Context, svc *FuturesService) error
	}{
		{"an empty stockCode", opFuturesEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureOrder()
			req.Symbol = domain.Symbol{}
			result, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
			requireZero(t, result)
			return err
		}},
		{"an empty entrustType", opFuturesEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureOrder()
			req.OrderType = ""
			result, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
			requireZero(t, result)
			return err
		}},
		{"an empty validTimeType", opFuturesEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureOrder()
			req.ValidTimeType = ""
			result, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
			requireZero(t, result)
			return err
		}},
		{"an empty entrustBs", opFuturesEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureOrder()
			req.Side = ""
			result, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
			requireZero(t, result)
			return err
		}},
		{"a zero entrustAmount", opFuturesEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureOrder()
			req.Quantity = domain.MustNewQuantity("0")
			result, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
			requireZero(t, result)
			return err
		}},
		{"an empty entrustId on a modify", opFuturesModifyEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			req := futuresFixtureModify()
			req.EntrustID = ""
			return svc.ModifyEntrust(ctx, futuresFixtureAccount(), req)
		}},
		{"an empty entrustId on a cancel", opFuturesCancelEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			return svc.CancelEntrust(ctx, futuresFixtureAccount(), "", futuresFixtureSymbol())
		}},
		{"an empty stockCode on a cancel", opFuturesCancelEntrust, func(t *testing.T, ctx context.Context, svc *FuturesService) error {
			t.Helper()
			return svc.CancelEntrust(ctx, futuresFixtureAccount(), futuresFixtureEntrustID(), domain.Symbol{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.run(t, t.Context(), NewFuturesService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// ---------------------------------------------------------------------------
// Money
// ---------------------------------------------------------------------------

// futuresMutationMoneyHost is one money-bearing field on one mutation.
//
// A mutation's money is a *write*, which is what makes the request side the side
// that matters here: the C4 money hosts are all reads, where a lost digit is a
// wrong number on a report. A lost digit on entrustPrice or entrustAmount is a
// wrong order. So every host is driven over the real HTTP stack and the assertion
// is on the recorded request body, with the exact literal required to appear there
// quoted.
type futuresMutationMoneyHost struct {
	name   string
	method string
	field  string
	route  client.Route
	// price is true for a price-shaped field, which is driven with the price sets;
	// the quantity hosts get the 27-digit integer instead, because 1e-330 is a
	// price-shaped value and a contract count is not.
	price bool
	// invoke performs the mutation carrying literal in the targeted field.
	invoke func(t *testing.T, ctx context.Context, svc *FuturesService, literal string) error
}

// futuresMutationMoneyHosts covers every money field on every mutation body: the
// price and the quantity of the entrust, and the price and the quantity of the
// modify. There are no money fields on the cancel body - entrustId and stockCode
// are neither - which is why this table has four hosts and three methods.
func futuresMutationMoneyHosts() []futuresMutationMoneyHost {
	return []futuresMutationMoneyHost{
		{
			name: "Entrust/entrustPrice", method: "Entrust", field: "entrustPrice",
			route: client.RouteTradeFuturesEntrust, price: true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService, literal string) error {
				t.Helper()
				req := futuresFixtureOrder()
				req.Price = domain.MustNewPrice(literal, "0")
				_, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
				return err
			},
		},
		{
			name: "Entrust/entrustAmount", method: "Entrust", field: "entrustAmount",
			route: client.RouteTradeFuturesEntrust,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService, literal string) error {
				t.Helper()
				req := futuresFixtureOrder()
				req.Quantity = domain.MustNewQuantity(literal)
				_, err := svc.Entrust(ctx, futuresFixtureAccount(), req)
				return err
			},
		},
		{
			name: "ModifyEntrust/entrustPrice", method: "ModifyEntrust", field: "entrustPrice",
			route: client.RouteTradeFuturesModifyEntrust, price: true,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService, literal string) error {
				t.Helper()
				req := futuresFixtureModify()
				req.Price = domain.MustNewPrice(literal, "0")
				return svc.ModifyEntrust(ctx, futuresFixtureAccount(), req)
			},
		},
		{
			name: "ModifyEntrust/entrustAmount", method: "ModifyEntrust", field: "entrustAmount",
			route: client.RouteTradeFuturesModifyEntrust,
			invoke: func(t *testing.T, ctx context.Context, svc *FuturesService, literal string) error {
				t.Helper()
				req := futuresFixtureModify()
				req.Quantity = domain.MustNewQuantity(literal)
				return svc.ModifyEntrust(ctx, futuresFixtureAccount(), req)
			},
		},
	}
}

// futuresMutationMoneyCasesFor returns the value sets a host is driven with. A
// quantity host gets the 27-digit integer, which is its float64-hostile analogue;
// a price host gets both price sets, because a value that discriminates a float64
// is often blind to a fixed-scale formatter and vice versa.
func futuresMutationMoneyCasesFor(host futuresMutationMoneyHost) []futuresMoneySet {
	if !host.price {
		return []futuresMoneySet{
			{"float64-hostile-quantity", float64HostileQuantities, requireFloat64Hostile},
		}
	}
	return futuresMoneySets()
}

// futuresAssertQuotedInRequest is the mutation-side counterpart of
// futuresAssertQuotedOnWire: it asserts the recorded *request* envelope carries
// the value as a quoted JSON string and does not also carry it unquoted.
//
// It is the assertion that catches a wire field retyped from string to float64, and
// it is the one that fails if a conversion reappears anywhere between the caller's
// domain.Price and the envelope - a float64 round trip, or the removed
// fmt.Sprintf("%.3f", …) that turned a 0.0005 tick into 0.001 with no error
// anywhere.
func futuresAssertQuotedInRequest(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	quoted := `"` + field + `":"` + tc.want + `"`
	if !strings.Contains(body, quoted) {
		t.Errorf("the recorded request does not contain %s.\n"+
			"  body:              %s\n"+
			"  wanted substring:  %s\n"+
			"  a float64 round trip would render %s\n"+
			"  the removed %%.3f form would render %s",
			quoted, body, quoted,
			strconv.FormatFloat(mustFloat(t, tc.in), 'f', -1, 64),
			legacyRounded(mustFloat(t, tc.in)))
		return
	}
	if unquoted := `"` + field + `":` + tc.want; strings.Contains(body, unquoted) {
		t.Errorf("the unquoted form %s is also present, so the field is a JSON number on the "+
			"wire: a float64 in the request path would render this way, and a mutation's price "+
			"would be a lossy write rather than a quoted decimal", unquoted)
	}
}

// futuresAssertNotRenderedAs is the negative half: the two renderings the two
// historical defects produce must both be absent from the request body.
//
// It is separated from the positive assertion because the positive one could pass
// by accident if a future set were short enough that the two renderings coincided,
// and these two sets are chosen so that they never do.
func futuresAssertNotRenderedAs(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	f := mustFloat(t, tc.in)
	for label, rendered := range map[string]string{
		"a float64 round trip":  strconv.FormatFloat(f, 'f', -1, 64),
		"the removed %.3f form": legacyRounded(f),
	} {
		if rendered == tc.want {
			continue
		}
		if strings.Contains(body, `"`+field+`":"`+rendered+`"`) ||
			strings.Contains(body, `"`+field+`":`+rendered) {
			t.Errorf("the request carries %s = %s, which is %s of the input %q rather than the "+
				"exact decimal; a mutation writes this field, so a lost digit is a wrong order",
				field, rendered, label, tc.in)
		}
	}
}

// TestFuturesMutationMoneyCrossesVerbatim drives every mutation money host over
// every value set it is aimed at and asserts the exact decimal reaches the wire.
//
// The value sets are the frozen ones from testsupport_test.go: three
// float64-hostile prices (19 significant digits, 25 significant digits, and
// 1e-330, which is positive as a decimal and zero to every float64), four
// scale-hostile prices (including 0.0005, the literal the shipped bug turned into
// 0.001), and one float64-hostile quantity (a 27-digit integer binary64 cannot
// represent). The guard on the guards - TestFuturesMoneyValueSetsAreStillHostile,
// inherited from C4 - asserts each is still hostile, so a future edit that softened a
// literal cannot quietly delete this net.
func TestFuturesMutationMoneyCrossesVerbatim(t *testing.T) {
	if got := len(futuresMutationMoneyHosts()); got != 4 {
		t.Fatalf("the money host table has %d hosts, want the four money fields the two "+
			"mutation bodies carry", got)
	}
	for _, host := range futuresMutationMoneyHosts() {
		t.Run(host.name, func(t *testing.T) {
			if host.method == "" || host.route == "" || host.field == "" {
				t.Fatal("test bug: the host has no method, route or field, so the row proves nothing")
			}
			for _, set := range futuresMutationMoneyCasesFor(host) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							// The canonical form the wire must carry is the decimal's own
							// String(), which for these inputs is tc.want. Assert that first,
							// so a test bug in the set is visible before the method runs.
							if host.price {
								if got := domain.MustNewPrice(tc.want, "0").String(); got != tc.want {
									t.Fatalf("test bug: the price constructor renders %q as %q, so the "+
										"expected wire literal is not tc.want", tc.want, got)
								}
							} else if got := domain.MustNewQuantity(tc.want).String(); got != tc.want {
								t.Fatalf("test bug: the quantity constructor renders %q as %q, so "+
									"the expected wire literal is not tc.want", tc.want, got)
							}

							rec := newWireRecorder(map[string]string{
								string(host.route): gatewaySuccess(futuresMutationBody),
							})
							if err := host.invoke(t, t.Context(),
								NewFuturesService(newWireExecutor(t, rec)), tc.in); err != nil {
								t.Fatalf("%s carrying %q = %v, want nil", host.name, tc.in, err)
							}
							if got := rec.count(string(host.route)); got != 1 {
								t.Fatalf("requests to %s = %d, want exactly 1", host.route, got)
							}

							body := rec.lastBody(t, string(host.route))
							futuresAssertQuotedInRequest(t, body, host.field, tc)
							futuresAssertNotRenderedAs(t, body, host.field, tc)
						})
					}
				})
			}
		})
	}
}

// TestFuturesMutationMoneyIsReachedThroughTheTypedPath is the Level 1 counterpart:
// the same literals, asserted on the request struct the method handed the executor
// rather than on the marshalled bytes.
//
// The two levels are not redundant. The struct assertion pins that the mapping
// boundary did not touch the digits, and the wire assertion pins that the
// transport did not touch them either - a float64 wire *type* would fail the
// second and a %.3f in the mapper would fail the first, and only running both says
// which layer a regression landed in.
func TestFuturesMutationMoneyIsReachedThroughTheTypedPath(t *testing.T) {
	for _, host := range futuresMutationMoneyHosts() {
		t.Run(host.name, func(t *testing.T) {
			for _, set := range futuresMutationMoneyCasesFor(host) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							exec := newSequencedExecutor(t, sequencedReply{
								reply: json.RawMessage(futuresMutationBody),
							})
							if err := host.invoke(t, t.Context(),
								NewFuturesService(exec), tc.in); err != nil {
								t.Fatalf("%s carrying %q = %v, want nil", host.name, tc.in, err)
							}
							requireCalls(t, exec, 1)

							futuresAssertVerbatim(t, host.name,
								futuresRecordedField(t, exec.lastParams(t), host.field), tc)
						})
					}
				})
			}
		})
	}
}

// futuresRecordedField reads one wire field out of whichever of the two mutation
// request structs the executor recorded. An unrecognised struct is a test bug and
// fails here rather than returning empty and being asserted against, so a third
// body appearing in the table cannot pass by being skipped.
func futuresRecordedField(t *testing.T, params any, field string) string {
	t.Helper()
	switch body := params.(type) {
	case futuresEntrustWireRequest:
		switch field {
		case "entrustPrice":
			return body.EntrustPrice
		case "entrustAmount":
			return body.EntrustAmount
		}
	case futuresModifyEntrustWireRequest:
		switch field {
		case "entrustPrice":
			return body.EntrustPrice
		case "entrustAmount":
			return body.EntrustAmount
		}
	}
	t.Fatalf("the recorded params are %T, which carries no %q field; the money host table and "+
		"the wire structs have drifted apart", params, field)
	return ""
}

// TestFuturesMutationTypesCarryNoFloat is the static half of hard rule 3 for the
// two caller-facing mutation requests and the three wire bodies, on the same
// reasoning as TestFuturesReadDomainTypesCarryNoFloat.
//
// scripts/check_money.py guards by field name and by wire key, which is the right
// heuristic for the generated DTOs and blind to a decimal field given an innocuous
// name. This walks the structs a caller fills in and asserts no field is a float at
// all, whatever it is called: Price and Quantity are decimal values
// (docs/DESIGN.md §7, hard rule 3), and the string fields on the two request
// types are Gateway *codes*, never amounts.
func TestFuturesMutationTypesCarryNoFloat(t *testing.T) {
	for _, v := range []any{
		FuturesOrderRequest{},
		FuturesModifyRequest{},
		futuresEntrustWireRequest{},
		futuresCancelEntrustWireRequest{},
		futuresModifyEntrustWireRequest{},
	} {
		rt := reflect.TypeOf(v)
		t.Run(rt.Name(), func(t *testing.T) {
			fields := 0
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				fields++
				switch f.Type.Kind() {
				case reflect.Float32, reflect.Float64:
					t.Errorf("%s.%s is a %s: a futures price, quantity or amount crosses the wire "+
						"as a quoted decimal string, and the two request types hold domain.Price "+
						"and domain.Quantity (docs/DESIGN.md §7, hard rule 3)", rt.Name(), f.Name, f.Type)
				}
			}
			if fields == 0 {
				t.Errorf("%s has no fields, so this test would pass vacuously", rt.Name())
			}
		})
	}
}

// TestFuturesMutationsAreSafeForConcurrentUse is the concurrency half of the
// FuturesService contract, extended to the mutation path.
//
// It is worth having because a mutation is where a shared decoded envelope would
// be expensive rather than merely wrong: two callers each believing they placed
// their own order is a duplicate, which is the exact failure ADR 0003 exists to
// prevent. The recorder's request count is the evidence that all N really went
// out, and the per-worker quantities in the bodies are the evidence that the
// requests were not confused with one another.
func TestFuturesMutationsAreSafeForConcurrentUse(t *testing.T) {
	const workers = 8

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeFuturesEntrust): gatewaySuccess(futuresMutationBody),
	})
	svc := NewFuturesService(newWireExecutor(t, rec))

	results := make([]string, workers)
	failures := make([]error, workers)
	start := make(chan struct{})
	done := make(chan struct{}, workers)
	for i := range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			<-start
			req := futuresFixtureOrder()
			// A distinct quantity per worker, so a shared decoded envelope would be
			// visible in the recorded bodies below.
			req.Quantity = domain.MustNewQuantity(strconv.Itoa(i + 1))
			result, err := svc.Entrust(t.Context(), futuresFixtureAccount(), req)
			if err != nil {
				failures[i] = err
				return
			}
			results[i] = result.EntrustID.String()
		}()
	}
	close(start)
	for range workers {
		<-done
	}

	for i := range workers {
		if failures[i] != nil {
			t.Errorf("worker %d = %v, want nil", i, failures[i])
			continue
		}
		if results[i] != "F20260921001" {
			t.Errorf("worker %d read entrustId %q, want the Gateway's own value: two callers "+
				"shared one decoded reply", i, results[i])
		}
	}
	if got := rec.count(string(client.RouteTradeFuturesEntrust)); got != workers {
		t.Errorf("requests = %d, want %d: a call that returned a result must have gone out", got, workers)
	}

	// Every worker's own quantity reached the wire, so none of them was served a
	// reply that belonged to another. The first and the last are checked because a
	// shared body would show up as one of them being missing, not as a count.
	saw := map[string]bool{}
	for i := range rec.requests {
		for _, want := range []string{`"entrustAmount":"1"`, `"entrustAmount":"8"`} {
			if strings.Contains(rec.requests[i].body, want) {
				saw[want] = true
			}
		}
	}
	for _, want := range []string{`"entrustAmount":"1"`, `"entrustAmount":"8"`} {
		if !saw[want] {
			t.Errorf("no request carried %s, so the concurrent calls did not each send their own "+
				"body", want)
		}
	}
}
