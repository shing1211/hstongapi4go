// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the shared test support for pkg/services. It is frozen after
// B2 lands it: B3 (trading.go) and B4 (account.go) build on these helpers rather
// than re-declaring them, because a duplicate definition is a compile error that
// takes every other run in the package down with it. A task that needs something
// extra adds it to its own file under its own prefix rather than editing this one.
//
// It lives in package services, not services_test, because the tests need the
// unexported op constants and knownTopics, exactly as executor_test.go and
// market_test.go already do.
//
// Nothing here touches production code, and nothing here redeclares a helper
// another file already provides: orderBookBody and legacyRounded come from
// market_test.go and entry() from account_test.go, and the four services files'
// own tests use them rather than re-deriving them.

// ---------------------------------------------------------------------------
// 6.1 Assertions
// ---------------------------------------------------------------------------

// requireZero fails when v is not the zero value of T. Every service method
// here must hand back the zero value beside an error, so a caller cannot mistake
// a partially decoded payload for a successful response.
func requireZero[T any](t *testing.T, v T) {
	t.Helper()
	var zero T
	if !reflect.DeepEqual(v, zero) {
		t.Fatalf("value returned alongside the error = %+v, want the zero value", v)
	}
}

// errRejects asserts that err is the typed *errs.Error for wantCode under
// wantOp and returns it. It matches only through errors.As and the typed
// fields, never on a rendered message, so rewording a message cannot change the
// outcome.
func errRejects(t *testing.T, err error, wantCode types.StatusCode, wantOp string) *errs.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want the Gateway rejection %q", wantCode)
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(%T) yielded no *errs.Error", err)
	}
	if e.Code != wantCode {
		t.Errorf("Code = %q, want %q", e.Code, wantCode)
	}
	if e.Op != wantOp {
		t.Errorf("Op = %q, want %q", e.Op, wantOp)
	}
	return e
}

// assertInvalidParam is the fail-closed assertion for a request this package
// rejected before sending it. Unlike pkg/hstong/algo, whose ErrInvalidParams is
// a bare sentinel carrying no code, a local rejection here is a real
// *errs.Error, so the fail-closed property is that the code is 1016 and the
// category is api: a caller must never be handed a Gateway trading or account
// code for a request the Gateway never saw.
func assertInvalidParam(t *testing.T, err error, wantOp string) *errs.Error {
	t.Helper()
	e := errRejects(t, err, types.StatusInvalidParam, wantOp)
	if e.Category != errs.CategoryAPI {
		t.Errorf("Category = %q, want %q for a local rejection", e.Category, errs.CategoryAPI)
	}
	if got, ok := errs.CodeOf(err); !ok || got != types.StatusInvalidParam {
		t.Errorf("errs.CodeOf = (%q, %v), want (%q, true)", got, ok, types.StatusInvalidParam)
	}
	if got := errs.CategoryOf(err); got != errs.CategoryAPI {
		t.Errorf("errs.CategoryOf = %q, want %q", got, errs.CategoryAPI)
	}
	return e
}

// requireCalls asserts the exact number of Do calls a sequencedExecutor
// recorded. It is the "exactly one attempt" half of the error-arm rows, and the
// reason a zero-argument rejection can be told apart from a silent success.
func requireCalls(t *testing.T, f *sequencedExecutor, want int) {
	t.Helper()
	if got := f.callCount(); got != want {
		t.Fatalf("recorded Do calls = %d, want %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// 6.2 The sequenced fake
// ---------------------------------------------------------------------------

// sequencedReply is one scripted outcome: the body to decode into the caller's
// out value, or the error to return instead. A reply with a nil body and a nil
// error leaves out untouched, which is what the two out==nil methods (Subscribe,
// Unsubscribe) need.
type sequencedReply struct {
	reply any
	err   error
}

// sequencedCall is one recorded request.
type sequencedCall struct {
	op     string
	route  client.Route
	params any
}

// sequencedExecutor is an Executor that answers a scripted sequence of replies
// and records what it was asked for.
//
// fakeExecutor replies identically to every call, which is fine for a
// single-request method and useless for walkFundJourPages, which must see page 1
// and then page 2. This is the fix; fakeExecutor itself is pre-existing and
// shared, so it is not modified.
//
// Every field is guarded: pkg/hstong/stream learned the hard way that a fake
// which is not race-safe turns go test -race into a coin flip.
type sequencedExecutor struct {
	mu      sync.Mutex
	t       *testing.T
	replies []sequencedReply
	calls   []sequencedCall
}

// newSequencedExecutor builds a sequencedExecutor over replies, which are
// consumed in order: the nth Do gets the nth reply.
//
// The testing.T is the one deviation from the shape sketched in the plan, and it
// is deliberate. An exhausted reply list calls t.Fatal rather than silently
// reusing the last reply, so a walk that fetches more pages than the fixture
// provides fails where the bug is instead of quietly passing on stale data. All
// Do calls in this package happen on the test's own goroutine, so t.Fatal is
// legal here; a fixture consumed from another goroutine must not use this.
func newSequencedExecutor(t *testing.T, replies ...sequencedReply) *sequencedExecutor {
	t.Helper()
	return &sequencedExecutor{t: t, replies: replies}
}

// JSON implements Executor. A nil codec is what fakeExecutor returns too, and
// it is deliberate: the services only pass it through, so returning a real
// codec here would add a code path no assertion depends on.
func (s *sequencedExecutor) JSON() client.Codec { return nil }

// Do records the call and answers with the next scripted reply.
func (s *sequencedExecutor) Do(_ context.Context, op string, route client.Route, params any, _ client.Codec, out any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, sequencedCall{op: op, route: route, params: params})

	if len(s.replies) == 0 {
		s.t.Fatalf("%s: the executor was called with no scripted reply left; "+
			"the fixture is shorter than the code under test", op)
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]

	if reply.err != nil {
		return reply.err
	}
	if out == nil || reply.reply == nil {
		return nil
	}
	raw, err := json.Marshal(reply.reply)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// callCount returns how many Do calls have been recorded.
func (s *sequencedExecutor) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// callAt returns the ith recorded call and fails the test if there is no such
// call. It is the accessor the op, route, and request-shape assertions read.
func (s *sequencedExecutor) callAt(t *testing.T, i int) sequencedCall {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < 0 || i >= len(s.calls) {
		t.Fatalf("recorded call %d requested, but only %d call(s) were recorded", i, len(s.calls))
	}
	return s.calls[i]
}

// lastCall returns the most recent recorded call.
func (s *sequencedExecutor) lastCall(t *testing.T) sequencedCall {
	t.Helper()
	return s.callAt(t, s.callCount()-1)
}

// lastParams returns the params of the most recent recorded call.
func (s *sequencedExecutor) lastParams(t *testing.T) any {
	t.Helper()
	return s.lastCall(t).params
}

// paramsAt returns the params of the ith recorded call.
func (s *sequencedExecutor) paramsAt(t *testing.T, i int) any {
	t.Helper()
	return s.callAt(t, i).params
}

// ---------------------------------------------------------------------------
// 6.3 The wire recorder and a real client
// ---------------------------------------------------------------------------

// recordedRequest is one HTTP request the recorder saw.
type recordedRequest struct {
	method string
	path   string
	body   string
}

// wireRecorder is an http.Handler that records every request and answers from a
// canned response table. It is the only way to reach the behaviour a fake
// cannot show: what internal/transport builds out of an ok:false envelope, and
// how many HTTP requests a route actually produces under a retry policy
// (ADR 0003).
type wireRecorder struct {
	mu        sync.Mutex
	requests  []recordedRequest
	responses map[string]string
}

// newWireRecorder builds a recorder whose reply for each listed path is the
// given whole envelope body. An unlisted path answers
// {"ok":true,"err":"","data":{}}, so a test that asks for an endpoint it did not
// set up gets a decodable empty reply rather than a 404, and the failure shows
// up as a missing field rather than as an opaque HTTP error.
func newWireRecorder(responses map[string]string) *wireRecorder {
	return &wireRecorder{responses: responses}
}

// ServeHTTP records the request and writes the canned reply for its path.
func (r *wireRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)

	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{
		method: req.Method,
		path:   req.URL.Path,
		body:   string(body),
	})
	reply, ok := r.responses[req.URL.Path]
	r.mu.Unlock()

	if !ok {
		reply = `{"ok":true,"err":"","data":{}}`
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(reply))
}

// count returns how many requests reached path.
func (r *wireRecorder) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, req := range r.requests {
		if req.path == path {
			n++
		}
	}
	return n
}

// total returns how many requests reached the server at all.
func (r *wireRecorder) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// lastBody returns the body of the most recent request to path, and fails the
// test when path was never requested. It is the Level 2 money assertion: the
// recorded request is the real envelope, so a field whose wire type changed
// stops matching a quoted substring.
func (r *wireRecorder) lastBody(t *testing.T, path string) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.requests) - 1; i >= 0; i-- {
		if r.requests[i].path == path {
			return r.requests[i].body
		}
	}
	t.Fatalf("no request reached %s; the server saw %d request(s)", path, len(r.requests))
	return ""
}

// gatewayFailure renders an ok:false envelope in the "<code> : <text>" form the
// Gateway sends, which is what internal/transport.classifyFailure parses back
// into a typed *errs.Error.
func gatewayFailure(code types.StatusCode, text string) string {
	return fmt.Sprintf(`{"ok":false,"err":%q}`, string(code)+" : "+text)
}

// gatewaySuccess renders an ok:true envelope whose data is the given JSON
// document, so a test can put a real reply on the wire without hand-writing the
// envelope around it.
func gatewaySuccess(data string) string {
	return fmt.Sprintf(`{"ok":true,"err":"","data":%s}`, data)
}

// newWireExecutor starts a recorder-backed test server and returns a real
// *client.Client over it as an Executor — the dependency inversion of ADR 0010
// now used against the production implementation rather than a fake. copts are
// applied on top of the base URL, which is how a retry policy or a timeout gets
// installed.
//
// The server and the client are both closed through t.Cleanup, so no test has to
// remember to. This never points at 127.0.0.1:11111: a services test has no
// business talking to a live Gateway.
func newWireExecutor(t *testing.T, rec *wireRecorder, copts ...client.Option) Executor {
	t.Helper()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	all := make([]client.Option, 0, len(copts)+1)
	all = append(all, client.WithBaseURL(srv.URL))
	all = append(all, copts...)

	c, err := client.New(all...)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// ---------------------------------------------------------------------------
// 6.4 Money value sets
// ---------------------------------------------------------------------------

// moneyCase is one hostile value: in is the literal a test feeds in, and want is
// the exact string that must reach the wire (or reach the domain value, on the
// response side).
type moneyCase struct {
	name string
	in   string
	want string
}

// float64HostilePrices are values a float64 round-trip changes: they carry more
// significant digits than binary64 can hold, or they underflow it to zero. A
// float64 wire field or a parse-then-format conversion cannot pass them; only a
// verbatim decimal field can.
//
// The last row is the interesting one: 1e-330 is a positive decimal that is
// zero to every float64. domain.MustNewPrice expands it to the full positional
// form, so the value survives, while a float64 in the path would silently turn
// a real price into nothing.
var float64HostilePrices = []moneyCase{
	{
		name: "19 significant digits",
		in:   "123.4567890123456789",
		want: "123.4567890123456789",
	},
	{
		name: "25 significant digits",
		in:   "0.1234567890123456789012345",
		want: "0.1234567890123456789012345",
	},
	{
		name: "underflows binary64",
		in:   "1e-330",
		// The positional expansion, which is what a decimal String() yields: a
		// leading zero, a point, 329 zeros, and the significant 1. Written as a
		// construction because the literal is 332 characters long and a
		// transcription error in it would be invisible in review.
		want: underflowPositional,
	},
}

// float64HostileQuantities is the quantity counterpart: a 27-digit integer that
// a float64 renders as a different number, and that is nevertheless a legal
// order quantity (an integer, and a multiple of the HK stock lot of 100).
var float64HostileQuantities = []moneyCase{
	{
		name: "27-digit integer",
		in:   "999999999999999999999999900",
		want: "999999999999999999999999900",
	},
}

// scaleHostilePrices are float64-stable values that a fixed-scale formatter
// still destroys. This is the set the shipped bug lived in: fmt.Sprintf("%.3f",
// …) turned a 0.0005 tick into 0.001, a different and wrong price, with no
// error anywhere. All four are float64-stable, which is exactly why they
// discriminate a re-introduced %.3f and would not have shown up in the table
// above.
var scaleHostilePrices = []moneyCase{
	{
		name: "the shipped bug",
		in:   "0.0005",
		want: "0.0005",
	},
	{
		name: "sub-micro",
		in:   "0.0000001",
		want: "0.0000001",
	},
	{
		name: "trailing zeros",
		in:   "0.1",
		want: "0.1",
	},
	{
		name: "one third",
		in:   "0.3333333333333333",
		want: "0.3333333333333333",
	},
}

// underflowPositional is the positional decimal form of 1e-330, the
// representation every domain constructor here renders. It is computed rather
// than typed out for the reason given on the row above.
var underflowPositional = "0." + strings.Repeat("0", 329) + "1"

// requireFloat64Hostile asserts that in is genuinely hostile to a float64
// round-trip: parsing it and reformatting the shortest way changes the digits.
// The guard is the point — a future edit that softens a literal into a benign
// one fails here instead of quietly removing the regression net.
func requireFloat64Hostile(t *testing.T, in string) {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: strconv.ParseFloat(%q) failed: %v", in, err)
	}
	if got := strconv.FormatFloat(f, 'f', -1, 64); got == in {
		t.Fatalf("test bug: %q survives a float64 round-trip (%q), so it is no longer "+
			"float64-hostile and proves nothing about a float64 in the path", in, got)
	}
}

// requireScaleHostile asserts that in is destroyed by a three-decimal
// formatter, which is the specific conversion the bug used. It deliberately does
// not require the value to be float64-stable: what matters here is only that
// %.3f changes it, because a scale-hostile value that is also float64-hostile
// would be indistinguishable from the table above.
func requireScaleHostile(t *testing.T, in string) {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: strconv.ParseFloat(%q) failed: %v", in, err)
	}
	if got := legacyRounded(f); got == in {
		t.Fatalf("test bug: %%.3f renders %q as %q, so it is no longer scale-hostile "+
			"and would not catch a re-introduced conversion", in, got)
	}
}

// ---------------------------------------------------------------------------
// 6.5 Wire fixtures for market.go
// ---------------------------------------------------------------------------

// marketSecurity is the instrument every market fixture asks for and every
// market reply echoes. It is shared so a fixture and its request cannot drift
// apart: the echoed values are asserted against this same function.
func marketSecurity() *Security {
	return &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
}

// marketSecurityEcho is the wire spelling of marketSecurity, as it appears
// inside a reply.
const marketSecurityEcho = `"security":{"dataType":10000,"code":"00700.HK"}`

// basicQotBody is a BasicQot reply carrying two quotes and the security echo.
// Every float field is populated: the generated DTO types are float64, so an
// absent field decodes to 0 rather than failing, but a zero price is not a
// realistic reply and a test that asserted against it would be asserting against
// the fixture's gaps rather than against the code.
func basicQotBody() json.RawMessage {
	return json.RawMessage(`{` + marketSecurityEcho + `,"basicQot":[` +
		`{"security":{"dataType":10000,"code":"00700.HK"},"isSuspended":false,` +
		`"openPrice":380.5,"highPrice":388.2,"lowPrice":379.1,"lastPrice":387.05,` +
		`"lastClosePrice":380.0,"priceSpread":0.05,"volume":1200,"turnover":464460.0,` +
		`"turnoverRate":0.00005,"amplitude":0.0024,"secStatus":1,"listTime":"20011206",` +
		`"lotSize":100,"tradeTime":"09:30:01","marketTime":"20260126 09:30:01",` +
		`"stockStatus":"0","volumeStr":"1200","mktTmType":1},` +
		`{"security":{"dataType":10000,"code":"00700.HK"},"isSuspended":true,` +
		`"openPrice":381.0,"highPrice":381.5,"lowPrice":380.5,"lastPrice":381.25,` +
		`"lastClosePrice":381.0,"priceSpread":0.01,"volume":400,"turnover":152500.0,` +
		`"turnoverRate":0.00002,"amplitude":0.0003,"secStatus":0,"listTime":"20011206",` +
		`"lotSize":100,"tradeTime":"09:30:02","marketTime":"20260126 09:30:02",` +
		`"stockStatus":"1","volumeStr":"400","mktTmType":1}]}`)
}

// klBody is a KL reply carrying two candles and the security echo.
func klBody() json.RawMessage {
	return json.RawMessage(`{` + marketSecurityEcho + `,"kline":[` +
		`{"date":"20260123","highPrice":389.2,"openPrice":380.5,"lowPrice":379.1,` +
		`"closePrice":387.05,"lastClosePrice":380.0,"volume":12000,` +
		`"turnover":4644600.0,"timestamp":1769126400,"time":"16:00"},` +
		`{"date":"20260126","highPrice":388.2,"openPrice":387.05,"lowPrice":381.0,` +
		`"closePrice":382.5,"lastClosePrice":387.05,"volume":15000,` +
		`"turnover":5737500.0,"timestamp":1769212800,"time":"16:00"}]}`)
}

// timeShareBody is a TimeShare reply carrying two points and the security echo.
func timeShareBody() json.RawMessage {
	return json.RawMessage(`{` + marketSecurityEcho + `,"timeShare":[` +
		`{"time":"09:30","price":380.5,"lastClosePrice":380.0,"avgPrice":380.5,` +
		`"volume":1000,"turnover":380500.0},` +
		`{"time":"09:31","price":381.25,"lastClosePrice":380.0,"avgPrice":380.875,` +
		`"volume":2000,"turnover":762500.0}]}`)
}

// tickerBody is a Ticker reply carrying two ticks and the security echo.
func tickerBody() json.RawMessage {
	return json.RawMessage(`{` + marketSecurityEcho + `,"ticker":[` +
		`{"time":"09:30:01","side":1,"price":387.05,"volume":100,` +
		`"turnover":38705.0,"type":1,"timestamp":1769391001,"mktTmType":1},` +
		`{"time":"09:30:02","side":2,"price":387.0,"volume":200,` +
		`"turnover":77400.0,"type":2,"timestamp":1769391002,"mktTmType":1}]}`)
}

// brokerBody is a Broker reply carrying two ask and two bid queue entries plus
// the security echo.
func brokerBody() json.RawMessage {
	return json.RawMessage(`{` + marketSecurityEcho + `,"brokerAskList":[` +
		`{"level":1,"item":"1","type":0,"name":"broker-a"},` +
		`{"level":2,"item":"2","type":0,"name":"broker-b"}],` +
		`"brokerBidList":[` +
		`{"level":1,"item":"3","type":1,"name":"broker-c"},` +
		`{"level":2,"item":"4","type":1,"name":"broker-d"}]}`)
}

// usOptionChainBody is a UsOptionChainCode reply.
func usOptionChainBody() json.RawMessage {
	return json.RawMessage(`{"optionCode":["AAPL260619C00150000","AAPL260619C00200000"]}`)
}

// usOptionChainExpireBody is a UsOptionChainExpireDate reply.
func usOptionChainExpireBody() json.RawMessage {
	return json.RawMessage(`{"expireDate":["20260619","20260918"]}`)
}

// usOverNightBody is a UsOverNightTradeCodes reply.
func usOverNightBody() json.RawMessage {
	return json.RawMessage(`{"securityCodes":["AAPL","MSFT","TSLA"]}`)
}
