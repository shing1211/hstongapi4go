// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/transport"
)

// TestCodec_Alias pins the public alias: the accessor returns client.Codec,
// which is the transport codec interface, so no internal type is named by the
// public API (docs/DESIGN.md §6).
func TestCodec_Alias(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	var jsonCodec = c.JSON()
	if jsonCodec == nil {
		t.Fatal("JSON() returned nil")
	}
	if got := reflect.TypeOf(jsonCodec); got != reflect.TypeOf(transport.JSONCodec{}) {
		t.Fatalf("JSON() type = %v, want transport.JSONCodec", got)
	}
}

// TestNew_HTTPClientNoTimeout covers the two copies resolveHTTPClient makes for
// a supplied client with no explicit timeout: one for a nil transport, which
// gains a fresh cloned transport, and one for a concrete transport, which is
// reused unchanged.
func TestNew_HTTPClientNoTimeout(t *testing.T) {
	t.Run("nil transport gains a clone", func(t *testing.T) {
		custom := &http.Client{}
		c, err := New(WithHTTPClient(custom), WithTimeout(3*time.Second))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer c.Close()

		got := c.HTTPClient()
		if got == custom {
			t.Fatal("client with nil transport was used unchanged, want a copy")
		}
		if got.Transport == nil {
			t.Fatal("copied client transport = nil, want a cloned transport")
		}
		if got.Transport == http.DefaultTransport {
			t.Fatal("copied client shares http.DefaultTransport, want an independent clone")
		}
		if got.Timeout != 3*time.Second {
			t.Fatalf("timeout = %v, want 3s", got.Timeout)
		}
	})

	t.Run("concrete transport is reused", func(t *testing.T) {
		base := &http.Transport{}
		custom := &http.Client{Transport: base}
		c, err := New(WithHTTPClient(custom), WithTimeout(4*time.Second))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer c.Close()

		got := c.HTTPClient()
		if got == custom {
			t.Fatal("client with a concrete transport was used unchanged, want a copy")
		}
		if got.Transport != base {
			t.Fatal("copied client did not preserve the supplied transport")
		}
		if got.Timeout != 4*time.Second {
			t.Fatalf("timeout = %v, want 4s", got.Timeout)
		}
	})
}

// nonTransport is a RoundTripper that is deliberately not an *http.Transport so
// cloneDefaultTransport takes its fallback arm.
type nonTransport struct{}

func (nonTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrNotSupported
}

func TestCloneDefaultTransport_Fallback(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = nonTransport{}
	defer func() { http.DefaultTransport = original }()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	if c.HTTPClient().Transport == nil {
		t.Fatal("fallback transport = nil, want a fresh *http.Transport")
	}
	if _, isNonTransport := c.HTTPClient().Transport.(nonTransport); isNonTransport {
		t.Fatal("fallback reused the non-transport DefaultTransport")
	}
}

// TestNew_MissingPushHost covers the missing-host arm of validatePushAddr.
func TestNew_MissingPushHost(t *testing.T) {
	c, err := New(WithPushAddr(":11112"))
	if err == nil {
		c.Close()
		t.Fatal("New with a host-less push address = nil error, want error")
	}
}
