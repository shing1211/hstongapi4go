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
	"strconv"
	"sync"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
)

// marketProtoRoute is the one route exercised with the ProtoJSON codec so the
// end-to-end routing test covers the proto-backed market path as well as the
// hand-written JSON path. Every other route uses JSON.
const marketProtoRoute = RouteHqBasicQot

// TestRoutesE2E_AllCanonicalRoutes drives client.New(...).Do for every
// registered route against one httptest server and asserts the canonical POST
// envelope contract on each request. The JSON codec is used for the trade-ish
// surfaces and ProtoJSON for the market route, so both codecs cross the public
// API.
func TestRoutesE2E_AllCanonicalRoutes(t *testing.T) {
	type routeCase struct {
		name   string
		route  Route
		proto  bool
		params any
		data   string
	}

	cases := make([]routeCase, 0, len(Routes()))
	dataByPath := make(map[string]string, len(Routes()))
	for _, route := range Routes() {
		c := routeCase{name: route.Path(), route: route}
		if route == marketProtoRoute {
			c.proto = true
			c.params = &dto.Security{DataType: 10000, Code: "00700"}
			c.data = `{"dataType":10000,"code":"00700"}`
		} else {
			c.params = map[string]any{"code": "AAPL"}
			c.data = `{"code":"AAPL","price":"1.00","qty":1}`
		}
		cases = append(cases, c)
		dataByPath[route.Path()] = c.data
	}
	if len(cases) != 51 {
		t.Fatalf("registered routes = %d, want 51 (docs/SPEC.md)", len(cases))
	}

	var (
		mu     sync.Mutex
		seen   = make(map[string]int, len(cases))
		errMu  sync.Mutex
		firstE error
	)
	record := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstE == nil {
			firstE = err
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			record(fmt.Errorf("method = %s, want POST", r.Method))
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			record(fmt.Errorf("Content-Type = %q, want application/json", ct))
			return
		}

		path := r.URL.Path
		route := Route(path)
		if err := route.Validate(); err != nil {
			record(fmt.Errorf("path %q is not a canonical route: %w", path, err))
			return
		}
		if path != route.Path() {
			record(fmt.Errorf("path = %q, want its canonical form %q", path, route.Path()))
			return
		}
		if _, ok := dataByPath[path]; !ok {
			record(fmt.Errorf("path %q is not in the exercised route table", path))
			return
		}

		var env map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			record(fmt.Errorf("decoding envelope for %s: %v", path, err))
			return
		}
		ts, ok := env["timeout_sec"]
		if !ok {
			record(fmt.Errorf("envelope for %s is missing timeout_sec", path))
			return
		}
		if _, err := strconv.Atoi(string(ts)); err != nil {
			record(fmt.Errorf("timeout_sec for %s = %s, want a numeric integer", path, ts))
			return
		}
		params, ok := env["params"]
		if !ok {
			record(fmt.Errorf("envelope for %s is missing params", path))
			return
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(params, &obj); err != nil {
			record(fmt.Errorf("params for %s is not a JSON object: %v", path, err))
			return
		}

		mu.Lock()
		seen[path]++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"ok":true,"err":"","data":%s}`, dataByPath[path]); err != nil {
			record(fmt.Errorf("writing response for %s: %v", path, err))
		}
	}))
	defer srv.Close()

	cli, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cli.Close()

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			if c.proto {
				var out dto.Security
				if err := cli.Do(ctx, c.name, c.route, c.params, cli.ProtoJSON(), &out); err != nil {
					t.Fatalf("Do(%s): %v", c.route.Path(), err)
				}
				if out.GetDataType() != 10000 || out.GetCode() != "00700" {
					t.Fatalf("proto data = %+v, want dataType=10000 code=00700", &out)
				}
				return
			}

			out := map[string]any{}
			if err := cli.Do(ctx, c.name, c.route, c.params, cli.JSON(), &out); err != nil {
				t.Fatalf("Do(%s): %v", c.route.Path(), err)
			}
			if out["code"] != "AAPL" {
				t.Fatalf("json data = %+v, want code=AAPL", out)
			}
		})
	}

	errMu.Lock()
	handlerErr := firstE
	errMu.Unlock()
	if handlerErr != nil {
		t.Fatalf("server handler rejected a request: %v", handlerErr)
	}

	if len(seen) != len(cases) {
		missing := make([]string, 0, len(cases)-len(seen))
		for _, c := range cases {
			if seen[c.route.Path()] == 0 {
				missing = append(missing, c.route.Path())
			}
		}
		t.Fatalf("exercised %d paths, want %d; missing: %v", len(seen), len(cases), missing)
	}
	for _, c := range cases {
		if seen[c.route.Path()] != 1 {
			t.Errorf("path %s requested %d times, want exactly 1", c.route.Path(), seen[c.route.Path()])
		}
	}
}

// TestRoutesE2E_InvalidRoutesSendNothing asserts that unknown and alias routes
// are rejected before any request reaches the server.
func TestRoutesE2E_InvalidRoutesSendNothing(t *testing.T) {
	var count int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
	}))
	defer srv.Close()

	cli, err := New(WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cli.Close()

	invalid := []Route{
		"",
		"/hq/Nope",
		"/hq/basicqot",
		"/hq/BasicQot/",
		"/hq/BasicQotRequest",
		"/hq/BasicQotRequestMsgType",
		"/trade/TradeEntrust ",
		"hq/BasicQot",
	}
	for _, route := range invalid {
		t.Run(string(route), func(t *testing.T) {
			if err := cli.Do(context.Background(), "bad", route, nil, cli.JSON(), nil); !errors.Is(err, ErrUnknownRoute) {
				t.Fatalf("Do(%q) error = %v, want ErrUnknownRoute", route, err)
			}
		})
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 0 {
		t.Fatalf("requests = %d, want 0 for invalid routes", got)
	}
}
