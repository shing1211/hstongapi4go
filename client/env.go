// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Environment variable names read by WithEnv. Unset and empty variables are
// treated identically: the corresponding setting keeps its default (or its
// value from an earlier option).
const (
	// EnvGatewayURL is the Gateway HTTP root, parsed as an absolute http or
	// https URL. Default: DefaultBaseURL ("http://127.0.0.1:11111").
	EnvGatewayURL = "HSTONG_GATEWAY_URL"
	// EnvPushAddr is the Gateway TCP push address in "host:port" form.
	// Default: DefaultPushAddr ("127.0.0.1:11112").
	EnvPushAddr = "HSTONG_PUSH_ADDR"
	// EnvTimeout is the per-request timeout as a Go duration string, for
	// example "10s" or "1500ms". Default: DefaultTimeout ("10s").
	EnvTimeout = "HSTONG_TIMEOUT"
	// EnvTradePassword is the plaintext trade password. It is sensitive and is
	// never logged. Default: unset.
	EnvTradePassword = "HSTONG_TRADE_PASSWORD"
	// EnvVerifyPush enables push-frame signature verification and is parsed
	// with strconv.ParseBool ("1", "t", "true", "0", "f", "false"). Default:
	// false.
	EnvVerifyPush = "HSTONG_VERIFY_PUSH"
)

// applyEnv reads the documented environment variables and applies the ones
// that are set and non-empty. The first parse failure is recorded on c.err (New
// returns it); later variables are still parsed so that a single New call
// reports one clear error. A successfully parsed variable is applied even when
// a later one fails.
func applyEnv(c *Config) {
	if raw, ok := os.LookupEnv(EnvGatewayURL); ok && raw != "" {
		if err := validateBaseURL(raw); err != nil {
			recordErr(c, err)
		} else {
			c.BaseURL = raw
		}
	}
	if raw, ok := os.LookupEnv(EnvPushAddr); ok && raw != "" {
		if err := validatePushAddr(raw); err != nil {
			recordErr(c, err)
		} else {
			c.PushAddr = raw
		}
	}
	if raw, ok := os.LookupEnv(EnvTimeout); ok && raw != "" {
		d, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			recordErr(c, fmt.Errorf("client: %s: invalid duration %q: %w", EnvTimeout, raw, err))
		case d <= 0:
			recordErr(c, fmt.Errorf("client: %s: duration %q must be positive", EnvTimeout, raw))
		default:
			c.Timeout = d
		}
	}
	if raw, ok := os.LookupEnv(EnvTradePassword); ok && raw != "" {
		c.TradePassword = raw
	}
	if raw, ok := os.LookupEnv(EnvVerifyPush); ok && raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			recordErr(c, fmt.Errorf("client: %s: invalid boolean %q: %w", EnvVerifyPush, raw, err))
		} else {
			c.VerifyPush = v
		}
	}
}

// recordErr keeps the first error encountered while applying a configuration
// option, so the earliest and usually most specific failure is reported.
func recordErr(c *Config, err error) {
	if c.err == nil {
		c.err = err
	}
}

// validateBaseURL reports whether raw is an absolute http or https URL with a
// host. It is used by both WithEnv and New so explicit and environment-supplied
// values are checked identically.
func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("client: invalid %s %q: %w", EnvGatewayURL, raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("client: invalid %s %q: scheme must be http or https", EnvGatewayURL, raw)
	}
	if u.Host == "" {
		return fmt.Errorf("client: invalid %s %q: missing host", EnvGatewayURL, raw)
	}
	return nil
}

// validatePushAddr reports whether raw is a "host:port" TCP address with a
// non-empty host and port.
func validatePushAddr(raw string) error {
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return fmt.Errorf("client: invalid %s %q: %w", EnvPushAddr, raw, err)
	}
	if host == "" {
		return fmt.Errorf("client: invalid %s %q: missing host", EnvPushAddr, raw)
	}
	if port == "" {
		return fmt.Errorf("client: invalid %s %q: missing port", EnvPushAddr, raw)
	}
	return nil
}
