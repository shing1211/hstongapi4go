// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

// TestClients_NoLeaksUnderConcurrentDo creates many clients, runs concurrent Do
// calls through each, closes them all, and then asserts the package left no
// goroutines behind. The package-level TestMain in client_test.go installs the
// same check at process exit; VerifyNone here pins the scope to this test.
func TestClients_NoLeaksUnderConcurrentDo(t *testing.T) {
	const (
		numClients = 100
		perClient  = 8
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"err":"","data":{"code":"AAPL","price":"1.00","qty":1}}`)
	}))

	clients := make([]*Client, 0, numClients)
	for i := 0; i < numClients; i++ {
		c, err := New(WithBaseURL(srv.URL))
		if err != nil {
			srv.Close()
			t.Fatalf("New #%d: %v", i, err)
		}
		clients = append(clients, c)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, numClients*perClient)
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *Client) {
			defer wg.Done()
			for j := 0; j < perClient; j++ {
				var out map[string]any
				err := c.Do(context.Background(), "hq/BasicQot", RouteHqBasicQot,
					map[string]any{"code": "AAPL"}, c.JSON(), &out)
				if err != nil {
					errCh <- fmt.Errorf("client %d Do %d: %w", i, j, err)
					return
				}
				if out["code"] != "AAPL" {
					errCh <- fmt.Errorf("client %d Do %d: out = %v, want code=AAPL", i, j, out)
					return
				}
			}
		}(i, c)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	for i, c := range clients {
		if err := c.Close(); err != nil {
			t.Errorf("Close #%d: %v", i, err)
		}
	}
	srv.Close()

	goleak.VerifyNone(t)
}

// TestClients_ManyCreateClose cycles through create-then-close many times to
// confirm New and Close pair up without accumulating goroutines.
func TestClients_ManyCreateClose(t *testing.T) {
	for i := 0; i < 100; i++ {
		c, err := New(WithBaseURL("http://127.0.0.1:1"))
		if err != nil {
			t.Fatalf("New #%d: %v", i, err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
	goleak.VerifyNone(t)
}
