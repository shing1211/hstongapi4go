// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestClampPageSize asserts the documented default and hard cap.
func TestClampPageSize(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
	}{
		{-5, DefaultPageSize},
		{0, DefaultPageSize},
		{1, 1},
		{20, 20},
		{99, 99},
		{100, MaxPageSize},
		{1000, MaxPageSize},
	} {
		if got := ClampPageSize(tc.in); got != tc.want {
			t.Errorf("ClampPageSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestPaginate_NilFetch asserts a nil fetch is a no-op.
func TestPaginate_NilFetch(t *testing.T) {
	got, err := Paginate[int](context.Background(), DefaultPageSize, nil)
	if err != nil {
		t.Fatalf("Paginate(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("Paginate(nil) = %v, want nil", got)
	}
}

// TestPaginate_StopsOnShortPage asserts a page shorter than the page size ends
// the walk after a single request.
func TestPaginate_StopsOnShortPage(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, _ int, _ string) (Page[int], error) {
		calls++
		return Page[int]{Items: []int{1, 2, 3}}, nil
	}

	got, err := Paginate(context.Background(), DefaultPageSize, fetch)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

// TestPaginate_DoesNotLoopOnStalledCursor asserts a full page whose cursor does
// not advance terminates instead of looping forever.
func TestPaginate_DoesNotLoopOnStalledCursor(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, _ int, cursor string) (Page[int], error) {
		calls++
		return Page[int]{Items: []int{1, 2}, Cursor: cursor}, nil
	}

	if _, err := Paginate(context.Background(), 2, fetch); err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1 (cursor did not advance)", calls)
	}
}

// fundPageHandler serves a real fund-journey list keyed by the queryParamStr
// cursor: "0" and "cur1" return full pages, anything else returns an empty page.
type fundPageHandler struct {
	mu    sync.Mutex
	calls int
}

// ServeHTTP echoes two full pages then an empty one.
func (h *fundPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	h.mu.Lock()
	h.calls++
	h.mu.Unlock()

	var env struct {
		Params struct {
			QueryParamStr string `json:"queryParamStr"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &env)

	var resp string
	switch env.Params.QueryParamStr {
	case "0":
		resp = fundPage("cur1", DefaultPageSize)
	case "cur1":
		resp = fundPage("cur2", DefaultPageSize)
	default:
		resp = `{"ok":true,"err":"","data":{"data":[]}}`
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, resp)
}

// fundPage builds a success envelope with n fund-journey rows whose last row
// carries cursor.
func fundPage(cursor string, n int) string {
	rows := make([]FundJourVo, n)
	for i := range rows {
		rows[i] = FundJourVo{BusinessBalance: "1.00", Type: "1", QueryParamStr: fmt.Sprintf("row-%d", i)}
	}
	rows[n-1].QueryParamStr = cursor
	payload, _ := json.Marshal(map[string]any{"data": rows})
	return `{"ok":true,"err":"","data":` + string(payload) + `}`
}

// TestPaginate_WalksAllPages drives Paginate through the real manager against a
// handler that returns two full pages then an empty page, and asserts exactly
// three requests and all rows concatenated.
func TestPaginate_WalksAllPages(t *testing.T) {
	handler := &fundPageHandler{}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := client.New(client.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	m := New(c)

	fetch := func(ctx context.Context, size int, cursor string) (Page[FundJourVo], error) {
		rows, err := m.RealFundJourList(ctx, FundJourListRequest{
			ExchangeType:  types.ExchangeHK,
			QueryCount:    size,
			QueryParamStr: cursor,
		})
		if err != nil {
			return Page[FundJourVo]{}, err
		}
		next := ""
		if len(rows) > 0 {
			next = rows[len(rows)-1].QueryParamStr
		}
		return Page[FundJourVo]{Items: rows, Cursor: next}, nil
	}

	all, err := Paginate(context.Background(), DefaultPageSize, fetch)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}

	handler.mu.Lock()
	calls := handler.calls
	handler.mu.Unlock()

	if calls != 3 {
		t.Fatalf("HTTP requests = %d, want exactly 3", calls)
	}
	if len(all) != 2*DefaultPageSize {
		t.Fatalf("rows = %d, want %d", len(all), 2*DefaultPageSize)
	}
	for i, row := range all {
		if row.BusinessBalance != "1.00" {
			t.Fatalf("row %d BusinessBalance = %q", i, row.BusinessBalance)
		}
	}
}

// TestPaginate_ContextCancelled asserts a cancelled context stops the walk and
// returns the error.
func TestPaginate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetch := func(_ context.Context, _ int, _ string) (Page[int], error) {
		t.Fatal("fetch called after cancellation")
		return Page[int]{}, nil
	}
	if _, err := Paginate(ctx, DefaultPageSize, fetch); err == nil {
		t.Fatal("Paginate = nil error, want context error")
	}
}
