// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package mockgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"

	"github.com/shing1211/hstongapi4go/client"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies the package leaves no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// postEnvelope performs a raw POST of the Gateway envelope and returns the HTTP
// status and decoded response envelope.
func postEnvelope(t *testing.T, base, path, params string) (int, responseEnvelope) {
	t.Helper()
	if params == "" {
		params = "{}"
	}
	body := fmt.Sprintf(`{"timeout_sec":10,"params":%s}`, params)
	resp, err := http.Post(base+path, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var env responseEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s (%s): %v", path, raw, err)
	}
	return resp.StatusCode, env
}

// TestAllRoutesRegisteredAndRespond asserts the mock serves every one of the 51
// canonical routes and that its fixture table covers exactly that set.
func TestAllRoutesRegisteredAndRespond(t *testing.T) {
	routes := client.Routes()
	if len(routes) != 51 {
		t.Fatalf("canonical routes = %d, want 51 (docs/SPEC.md §2)", len(routes))
	}

	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	paths := srv.fixtures.Paths()
	have := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		have[p] = struct{}{}
	}
	for _, r := range routes {
		if _, ok := have[r.Path()]; !ok {
			t.Errorf("no fixture registered for %s", r.Path())
		}
	}
	if len(paths) != 51 {
		t.Errorf("fixture paths = %d, want 51", len(paths))
	}

	for _, r := range routes {
		r := r
		t.Run(r.Path(), func(t *testing.T) {
			status, env := postEnvelope(t, srv.HTTPBaseURL(), r.Path(), "{}")
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (err=%q)", status, env.Err)
			}
			if !env.OK {
				t.Fatalf("ok = false, err = %q", env.Err)
			}
		})
	}
}

// TestUnknownAndAliasRoutes asserts an unknown path is 404/ok:false and that a
// Gateway alias resolves to its canonical fixture.
func TestUnknownAndAliasRoutes(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	status, env := postEnvelope(t, srv.HTTPBaseURL(), "/hq/Nope", "{}")
	if status != http.StatusNotFound || env.OK {
		t.Fatalf("unknown route: status=%d ok=%v, want 404/false", status, env.OK)
	}
	if env.Err == "" {
		t.Fatal("unknown route: empty err")
	}

	for _, alias := range []string{"/hq/BasicQotRequest", "/hq/BasicQotRequestMsgType"} {
		status, env := postEnvelope(t, srv.HTTPBaseURL(), alias, "{}")
		if status != http.StatusOK || !env.OK {
			t.Fatalf("alias %s: status=%d ok=%v err=%q", alias, status, env.OK, env.Err)
		}
	}
}

// TestNonPostRejected asserts the mock accepts POST only.
func TestNonPostRejected(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	resp, err := http.Get(srv.HTTPBaseURL() + "/hq/BasicQot")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", resp.StatusCode)
	}
}

// TestCursorPagination asserts the documented cursor behaviour: default page
// size 20, the "less than 100" cap, and a next-page cursor.
func TestCursorPagination(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	// Default page size is 20.
	_, env := postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeQueryRealEntrustList", `{"exchangeType":"K","queryCount":0,"queryParamStr":"0"}`)
	page := decodeRows(t, env.Data, "data")
	if len(page) != 20 {
		t.Fatalf("default page size = %d rows, want 20", len(page))
	}
	if got := cursorOf(t, page[len(page)-1]); got != "20" {
		t.Fatalf("next cursor = %q, want \"20\"", got)
	}

	// Following the cursor returns the next page.
	_, env = postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeQueryRealEntrustList", `{"exchangeType":"K","queryCount":20,"queryParamStr":"20"}`)
	page = decodeRows(t, env.Data, "data")
	if len(page) != 20 {
		t.Fatalf("second page size = %d rows, want 20", len(page))
	}
	if got := cursorOf(t, page[len(page)-1]); got != "40" {
		t.Fatalf("second next cursor = %q, want \"40\"", got)
	}

	// An oversized page is capped below 100.
	_, env = postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeQueryRealEntrustList", `{"exchangeType":"K","queryCount":500,"queryParamStr":"0"}`)
	page = decodeRows(t, env.Data, "data")
	if len(page) != fixturePageSizeMax {
		t.Fatalf("capped page size = %d rows, want %d", len(page), fixturePageSizeMax)
	}
}

// TestErrorInjection asserts a per-path injection produces ok:false with the
// documented code text and is consumed when finite.
func TestErrorInjection(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)
	srv.Inject("/trade/TradeQueryMarginFundInfo", ErrorInjection{Code: "1014", Message: "login timeout", Times: 1})

	status, env := postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeQueryMarginFundInfo", "{}")
	if status != http.StatusOK || env.OK {
		t.Fatalf("injected call: status=%d ok=%v, want 200/false", status, env.OK)
	}
	if env.Err != "1014 login timeout" {
		t.Fatalf("injected err = %q, want %q", env.Err, "1014 login timeout")
	}

	_, env = postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeQueryMarginFundInfo", "{}")
	if !env.OK {
		t.Fatalf("post-injection call still failed: %q", env.Err)
	}
}

// TestSubscriptionsRecorded asserts the market and trade subscribe routes update
// the mock's subscription state.
func TestSubscriptionsRecorded(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	postEnvelope(t, srv.HTTPBaseURL(), "/hq/Subscribe", `{"topicId":11,"security":[{"dataType":10000,"code":"00700.HK"}]}`)
	postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeSubscribe", "{}")

	if n := srv.SubscribedTopics()[types.TopicBasicQot]; n != 1 {
		t.Fatalf("topic 11 count = %d, want 1", n)
	}
	if !srv.TradeSubscribed() {
		t.Fatal("trade push not recorded")
	}

	postEnvelope(t, srv.HTTPBaseURL(), "/hq/Unsubscribe", `{"topicId":11,"security":[{"dataType":10000,"code":"00700.HK"}]}`)
	postEnvelope(t, srv.HTTPBaseURL(), "/trade/TradeUnsubscribe", "{}")
	if n := srv.SubscribedTopics()[types.TopicBasicQot]; n != 0 {
		t.Fatalf("topic 11 count after unsubscribe = %d, want 0", n)
	}
	if srv.TradeSubscribed() {
		t.Fatal("trade push still recorded after unsubscribe")
	}
}

// TestPushEmit asserts an emitted frame is readable on a raw connection and
// decodes to the concrete payload by notifyMsgType.
func TestPushEmit(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	conn, err := net.Dial("tcp", srv.PushAddr())
	if err != nil {
		t.Fatalf("dial push: %v", err)
	}
	defer conn.Close()

	waitForClients(t, srv, 1)

	if err := srv.EmitQuote(&dto.Security{DataType: 10000, Code: "00700.HK"},
		&dto.BasicQot{LastPrice: 388.0}); err != nil {
		t.Fatalf("EmitQuote: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	h, body, err := push.ReadFrame(conn)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if h.MsgType != push.MsgPush {
		t.Fatalf("msgType = %v, want push", h.MsgType)
	}
	var notify pbmsg.PBNotify
	if err := proto.Unmarshal(body, &notify); err != nil {
		t.Fatalf("unmarshal PBNotify: %v", err)
	}
	if types.NotifyMsgType(notify.GetNotifyMsgType()) != types.BasicQotNotifyMsgType {
		t.Fatalf("notifyMsgType = %d, want %d", notify.GetNotifyMsgType(), types.BasicQotNotifyMsgType)
	}
	if notify.GetNotifyId() != "00700.HK" {
		t.Fatalf("notifyId = %q, want 00700.HK", notify.GetNotifyId())
	}
	var payload hqnotify.BasicQotNotify
	if err := notify.GetPayload().UnmarshalTo(&payload); err != nil {
		t.Fatalf("UnmarshalTo: %v", err)
	}
	if payload.GetBasicQot().GetLastPrice() != 388.0 {
		t.Fatalf("last price = %v, want 388", payload.GetBasicQot().GetLastPrice())
	}
}

// TestCloseIdempotent asserts Close can be called repeatedly.
func TestCloseIdempotent(t *testing.T) {
	srv := New(WithPushAddr("127.0.0.1:0"))
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// waitForClients polls until the mock has at least n connected push clients.
func waitForClients(t *testing.T, srv *Server, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if srv.ClientCount() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("push clients = %d, want >= %d", srv.ClientCount(), n)
}

// decodeRows decodes the rows under key from a fixture data object.
func decodeRows(t *testing.T, data json.RawMessage, key string) []json.RawMessage {
	t.Helper()
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrap); err != nil {
		t.Fatalf("decode data %s: %v", data, err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(wrap[key], &rows); err != nil {
		t.Fatalf("decode rows under %q: %v", key, err)
	}
	return rows
}

// cursorOf extracts the queryParamStr cursor from a row.
func cursorOf(t *testing.T, row json.RawMessage) string {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil {
		t.Fatalf("decode row: %v", err)
	}
	var cursor string
	if err := json.Unmarshal(obj["queryParamStr"], &cursor); err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	return cursor
}
