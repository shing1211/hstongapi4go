// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package hstong

import (
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/client"
)

// TestWithKeepAliveRoute covers a public option that had no test at all. It is
// the endpoint the keep-alive loop polls to prove the trading token is still
// valid, so a wrong default would turn every poll into an error.
func TestWithKeepAliveRoute(t *testing.T) {
	cfg := defaultSessionConfig()
	if cfg.keepAliveRoute != client.RouteTradeQueryMarginFundInfo {
		t.Fatalf("default keepAliveRoute = %q, want %q", cfg.keepAliveRoute, client.RouteTradeQueryMarginFundInfo)
	}

	WithKeepAliveRoute(client.RouteHqBasicQot)(&cfg)
	if cfg.keepAliveRoute != client.RouteHqBasicQot {
		t.Errorf("keepAliveRoute = %q, want %q", cfg.keepAliveRoute, client.RouteHqBasicQot)
	}

	WithKeepAliveRoute("")(&cfg)
	if cfg.keepAliveRoute != client.RouteTradeQueryMarginFundInfo {
		t.Errorf("empty route: keepAliveRoute = %q, want the default restored", cfg.keepAliveRoute)
	}
}

// TestWithKeepAliveInterval pins both branches: a positive value is taken, and
// a non-positive one restores the default rather than setting a zero interval
// that would spin the keep-alive loop.
func TestWithKeepAliveInterval(t *testing.T) {
	cfg := defaultSessionConfig()

	WithKeepAliveInterval(90 * time.Second)(&cfg)
	if cfg.keepAliveInterval != 90*time.Second {
		t.Errorf("keepAliveInterval = %v, want 90s", cfg.keepAliveInterval)
	}

	for _, bad := range []time.Duration{0, -time.Second} {
		WithKeepAliveInterval(bad)(&cfg)
		if cfg.keepAliveInterval != DefaultKeepAliveInterval {
			t.Errorf("WithKeepAliveInterval(%v) = %v, want the default %v",
				bad, cfg.keepAliveInterval, DefaultKeepAliveInterval)
		}
	}
}

// TestDefaultSessionConfig pins the documented defaults, which the options above
// only ever restore rather than define.
func TestDefaultSessionConfig(t *testing.T) {
	cfg := defaultSessionConfig()
	if cfg.keepAliveInterval != DefaultKeepAliveInterval {
		t.Errorf("default keepAliveInterval = %v, want %v", cfg.keepAliveInterval, DefaultKeepAliveInterval)
	}
	if cfg.keepAliveRoute != client.RouteTradeQueryMarginFundInfo {
		t.Errorf("default keepAliveRoute = %q, want %q", cfg.keepAliveRoute, client.RouteTradeQueryMarginFundInfo)
	}
}
