// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestMain verifies that the package leaves no goroutines behind once every
// test, including Close, has run.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type quoteRequest struct {
	Code string `json:"code"`
}

type quoteData struct {
	Code  string      `json:"code"`
	Price string      `json:"price"`
	Qty   json.Number `json:"qty"`
}

func TestNew_Defaults(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.BaseURL() != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), DefaultBaseURL)
	}
	if c.PushAddr() != DefaultPushAddr {
		t.Errorf("PushAddr = %q, want %q", c.PushAddr(), DefaultPushAddr)
	}
	if c.Timeout() != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.Timeout(), DefaultTimeout)
	}
	if c.TradePassword() != "" {
		t.Errorf("TradePassword = %q, want empty", c.TradePassword())
	}
	if c.VerifyPush() {
		t.Error("VerifyPush = true, want false")
	}
	if c.PlatformPublicKey() != types.PlatformPublicKeyTest {
		t.Errorf("PlatformPublicKey = %q, want bundled test key", c.PlatformPublicKey())
	}
	if c.HTTPClient() == nil {
		t.Error("HTTPClient = nil, want non-nil")
	}
	if c.JSON() == nil || c.ProtoJSON() == nil {
		t.Error("codec accessors returned nil")
	}
}

func TestNew_Options(t *testing.T) {
	custom := &http.Client{Timeout: 3 * time.Second}
	c, err := New(
		WithBaseURL("http://example.test:9/"),
		WithPushAddr("example.test:1234"),
		WithTimeout(2*time.Second),
		WithTradePassword("s3cret"),
		WithPlatformPublicKey("override-key"),
		WithHTTPClient(custom),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.BaseURL() != "http://example.test:9/" {
		t.Errorf("BaseURL = %q", c.BaseURL())
	}
	if c.PushAddr() != "example.test:1234" {
		t.Errorf("PushAddr = %q", c.PushAddr())
	}
	if c.Timeout() != 2*time.Second {
		t.Errorf("Timeout = %v", c.Timeout())
	}
	if c.TradePassword() != "s3cret" {
		t.Errorf("TradePassword = %q", c.TradePassword())
	}
	if c.PlatformPublicKey() != "override-key" {
		t.Errorf("PlatformPublicKey = %q", c.PlatformPublicKey())
	}
	if c.HTTPClient() != custom {
		t.Error("HTTPClient is not the supplied client")
	}
}

func TestNew_EmptyOptionsRestoreDefaults(t *testing.T) {
	c, err := New(WithBaseURL(""), WithPushAddr(""), WithTimeout(0), WithPlatformPublicKey(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.BaseURL() != DefaultBaseURL || c.PushAddr() != DefaultPushAddr || c.Timeout() != DefaultTimeout {
		t.Fatalf("empty options restored %q / %q / %v", c.BaseURL(), c.PushAddr(), c.Timeout())
	}
	if c.PlatformPublicKey() != types.PlatformPublicKeyTest {
		t.Fatalf("PlatformPublicKey = %q, want test key", c.PlatformPublicKey())
	}
}

func TestNew_NilHTTPClientGetsTimeout(t *testing.T) {
	c, err := New(WithTimeout(4 * time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
	if got := c.HTTPClient().Timeout; got != 4*time.Second {
		t.Fatalf("client timeout = %v, want 4s", got)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
	}{
		{"missing scheme", []Option{WithBaseURL("127.0.0.1:11111")}},
		{"bad scheme", []Option{WithBaseURL("ftp://127.0.0.1:11111")}},
		{"missing host", []Option{WithBaseURL("http://")}},
		{"bad push addr", []Option{WithPushAddr("not-an-addr")}},
		{"missing push port", []Option{WithPushAddr("127.0.0.1:")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.opts...)
			if err == nil {
				c.Close()
				t.Fatalf("New(%v) = nil error, want error", tt.name)
			}
			if c != nil {
				t.Fatalf("New returned non-nil client on error")
			}
		})
	}
}

func TestNew_InvalidEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		val  string
	}{
		{"timeout", EnvTimeout, "not-a-duration"},
		{"non-positive timeout", EnvTimeout, "-3s"},
		{"verify bool", EnvVerifyPush, "maybe"},
		{"gateway url", EnvGatewayURL, "://nope"},
		{"push addr", EnvPushAddr, "no-port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.val)
			c, err := New(WithEnv())
			if err == nil {
				c.Close()
				t.Fatalf("New(WithEnv()) with %s=%q = nil error, want error", tt.env, tt.val)
			}
			if !strings.Contains(err.Error(), tt.env) {
				t.Fatalf("error = %q, want to mention %s", err.Error(), tt.env)
			}
		})
	}
}

func TestWithEnv_ReadsAll(t *testing.T) {
	t.Setenv(EnvGatewayURL, "http://127.0.0.1:22222")
	t.Setenv(EnvPushAddr, "127.0.0.1:22223")
	t.Setenv(EnvTimeout, "2500ms")
	t.Setenv(EnvTradePassword, "env-secret")
	t.Setenv(EnvVerifyPush, "true")

	c, err := New(WithEnv())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.BaseURL() != "http://127.0.0.1:22222" {
		t.Errorf("BaseURL = %q", c.BaseURL())
	}
	if c.PushAddr() != "127.0.0.1:22223" {
		t.Errorf("PushAddr = %q", c.PushAddr())
	}
	if c.Timeout() != 2500*time.Millisecond {
		t.Errorf("Timeout = %v", c.Timeout())
	}
	if c.TradePassword() != "env-secret" {
		t.Errorf("TradePassword = %q", c.TradePassword())
	}
	if !c.VerifyPush() {
		t.Error("VerifyPush = false, want true")
	}
}

// TestWithEnv_Precedence pins the documented rule: options applied after
// WithEnv win, options applied before it do not.
func TestWithEnv_Precedence(t *testing.T) {
	t.Setenv(EnvGatewayURL, "http://127.0.0.1:22222")
	t.Setenv(EnvTimeout, "1s")

	after, err := New(WithEnv(), WithBaseURL("http://127.0.0.1:33333"), WithTimeout(9*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer after.Close()
	if after.BaseURL() != "http://127.0.0.1:33333" || after.Timeout() != 9*time.Second {
		t.Fatalf("explicit options after WithEnv did not win: %q / %v", after.BaseURL(), after.Timeout())
	}

	before, err := New(WithBaseURL("http://127.0.0.1:33333"), WithEnv())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer before.Close()
	if before.BaseURL() != "http://127.0.0.1:22222" || before.Timeout() != time.Second {
		t.Fatalf("WithEnv after explicit options did not win: %q / %v", before.BaseURL(), before.Timeout())
	}
}

func TestDo_JSON(t *testing.T) {
	var seenPath atomic.Value
	seenPath.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath.Store(r.URL.Path)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var env struct {
			TimeoutSec int             `json:"timeout_sec"`
			Params     json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decoding envelope: %v", err)
			return
		}
		var params quoteRequest
		if err := json.Unmarshal(env.Params, &params); err != nil {
			t.Errorf("decoding params: %v", err)
			return
		}
		if params.Code != "AAPL" {
			t.Errorf("params.code = %q, want AAPL", params.Code)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"err":"","data":{"code":"AAPL","price":"123.45","qty":100}}`)
	}))
	defer srv.Close()

	c, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	var out quoteData
	if err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, quoteRequest{Code: "AAPL"}, c.JSON(), &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.Code != "AAPL" || out.Price != "123.45" || out.Qty != json.Number("100") {
		t.Fatalf("out = %+v, want decoded data", out)
	}
	if got := seenPath.Load().(string); got != "/hq/BasicQot" {
		t.Fatalf("request path = %q, want canonical /hq/BasicQot", got)
	}
}

func TestDo_ProtoJSON(t *testing.T) {
	var seenPath atomic.Value
	seenPath.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath.Store(r.URL.Path)
		var env struct {
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decoding envelope: %v", err)
			return
		}
		var security dto.Security
		if err := protojson.Unmarshal(env.Params, &security); err != nil {
			t.Errorf("decoding params with protojson: %v", err)
			return
		}
		if security.GetDataType() != 10000 || security.GetCode() != "00700" {
			t.Errorf("params = %+v, want dataType=10000 code=00700", &security)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"err":"","data":%s}`, env.Params)
	}))
	defer srv.Close()

	c, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	var out dto.Security
	err = c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot,
		&dto.Security{DataType: 10000, Code: "00700"}, c.ProtoJSON(), &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.GetDataType() != 10000 || out.GetCode() != "00700" {
		t.Fatalf("out = %+v, want dataType=10000 code=00700", &out)
	}
	if got := seenPath.Load().(string); got != "/hq/BasicQot" {
		t.Fatalf("request path = %q, want canonical /hq/BasicQot", got)
	}
}

func TestDo_UnknownRouteSendsNothing(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
	}))
	defer srv.Close()

	c, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	err = c.Do(context.Background(), "bad", Route("/hq/Nope"), nil, nil, nil)
	if !errors.Is(err, ErrUnknownRoute) {
		t.Fatalf("Do error = %v, want ErrUnknownRoute", err)
	}
	if got := atomic.LoadInt32(&count); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestClose_Idempotent(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Close(); err != nil {
				t.Errorf("concurrent Close: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestDo_AfterClose(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot, nil, c.JSON(), nil)
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("Do after Close = %v, want ErrClientClosed", err)
	}
	if !strings.Contains(err.Error(), "hq/BasicQot") {
		t.Fatalf("Do after Close = %q, want operation context", err.Error())
	}
}
