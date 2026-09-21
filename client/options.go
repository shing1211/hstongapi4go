// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/shing1211/hstongapi4go/internal/transport"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Default values applied by New when no option overrides them. They mirror the
// local HStong Gateway's documented addresses and the transport defaults.
const (
	// DefaultBaseURL is the local Gateway HTTP endpoint,
	// "http://127.0.0.1:11111".
	DefaultBaseURL = transport.DefaultBaseURL
	// DefaultPushAddr is the local Gateway TCP push address,
	// "127.0.0.1:11112".
	DefaultPushAddr = "127.0.0.1:11112"
	// DefaultTimeout is the per-request timeout, 10s. It is used both as the
	// HTTP client timeout and as the envelope's timeout_sec.
	DefaultTimeout = transport.DefaultTimeout
)

// Config holds the resolved settings for a Client. It is populated by the
// With... options and consumed by New; callers normally do not construct it
// directly. Every field has a documented default (see defaultConfig).
type Config struct {
	// BaseURL is the Gateway HTTP root, for example
	// "http://127.0.0.1:11111". It must be an absolute http or https URL.
	BaseURL string
	// PushAddr is the Gateway TCP push address in "host:port" form, for
	// example "127.0.0.1:11112". Later phases dial it; it is validated here.
	PushAddr string
	// HTTPClient is the client used for HTTP requests. It is never nil on a
	// Client built by New.
	HTTPClient *http.Client
	// Timeout is the per-request timeout. It is always positive on a Client
	// built by New.
	Timeout time.Duration
	// TradePassword is the plaintext trade password sent, encrypted, to
	// TradeLogin. It is sensitive: the SDK never logs it or includes it in an
	// error, and callers should prefer WithEnv or secret storage over
	// hardcoding it. The empty string means "not configured".
	TradePassword string
	// PlatformPublicKey is the base64 SPKI platform RSA public key used when
	// push-signature verification is enabled. It is public reference data, not
	// a secret.
	PlatformPublicKey string
	// VerifyPush enables verification of the push frame's SHA1WithRSA
	// signature. It is off by default. Later phases consume it.
	VerifyPush bool

	// Logger, when non-nil, receives structured SDK diagnostics. It is inert
	// by default: with no logger the SDK emits nothing. The SDK never logs
	// request payloads or secrets; a caller-supplied logger should still use
	// internal/logging.Redacting when forwarding arbitrary attributes.
	Logger *slog.Logger
	// Metrics, when non-nil, receives SDK measurements (counters, gauges, and
	// histograms). It is inert by default: with no recorder the SDK records
	// nothing. The SDK never measures payloads or secrets.
	Metrics Recorder
	// RetryPolicy, when non-nil, retries read-only query calls that fail with a
	// transient error. It is nil by default (exactly one attempt). Order,
	// futures, and algo mutations are never retried regardless of the policy
	// (docs/adr/0003-no-auto-retry-orders.md).
	RetryPolicy *RetryPolicy
	// RateLimiter, when non-nil, gates every call through its global and
	// per-endpoint token buckets. It is nil by default (no rate limit).
	RateLimiter *RateLimiter
	// CircuitBreaker, when non-nil, gates all calls and records their outcomes.
	// It is nil by default (no breaker). Deprecated: use QueryBreaker or
	// MutationBreaker for separate breakers, or WithCircuitBreaker which sets
	// both for backward compatibility.
	CircuitBreaker *CircuitBreaker
	// QueryBreaker, when non-nil, gates ClassQuery calls and records their
	// outcomes. It is nil by default (no breaker for queries).
	QueryBreaker *CircuitBreaker
	// MutationBreaker, when non-nil, gates ClassMutation calls and records
	// their outcomes. It is nil by default (no breaker for mutations).
	MutationBreaker *CircuitBreaker

	// err records the first failure produced by an option that can fail, such
	// as WithEnv. New surfaces it instead of the client.
	err error
}

// Option mutates a Config. Options are applied in order on top of defaultConfig
// inside New, so the last option that sets a field wins. Because WithEnv is
// itself an option, an explicit option placed after WithEnv overrides the
// environment value, while one placed before it does not; see WithEnv.
type Option func(*Config)

// defaultConfig returns the configuration applied before any option. It is the
// single source of truth for the SDK defaults.
func defaultConfig() Config {
	return Config{
		BaseURL:           DefaultBaseURL,
		PushAddr:          DefaultPushAddr,
		Timeout:           DefaultTimeout,
		PlatformPublicKey: types.PlatformPublicKeyTest,
	}
}

// WithBaseURL sets the Gateway HTTP root, for example
// "http://127.0.0.1:11111". The default is DefaultBaseURL; an empty value
// restores the default. A non-empty value is validated by New. A trailing
// slash is trimmed by the transport.
func WithBaseURL(baseURL string) Option {
	return func(c *Config) {
		if baseURL == "" {
			c.BaseURL = DefaultBaseURL
			return
		}
		c.BaseURL = baseURL
	}
}

// WithPushAddr sets the Gateway TCP push address in "host:port" form, for
// example "127.0.0.1:11112". The default is DefaultPushAddr; an empty value
// restores the default. A non-empty value is validated by New.
func WithPushAddr(addr string) Option {
	return func(c *Config) {
		if addr == "" {
			c.PushAddr = DefaultPushAddr
			return
		}
		c.PushAddr = addr
	}
}

// WithHTTPClient sets the *http.Client used for requests. The default is a
// client with its own cloned *http.Transport and the resolved Timeout. A client
// with no Timeout set is copied and given the resolved timeout; a client with
// an explicit timeout is used unchanged. When the supplied client has a nil
// Transport it uses http.DefaultTransport and Close cannot close its idle
// connections; supply a dedicated client if that matters.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) { c.HTTPClient = client }
}

// WithTimeout sets the per-request timeout used both as the HTTP client timeout
// and as the envelope's timeout_sec. The default is DefaultTimeout. A
// non-positive value restores the default.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		if timeout <= 0 {
			c.Timeout = DefaultTimeout
			return
		}
		c.Timeout = timeout
	}
}

// WithTradePassword sets the plaintext trade password used by TradeLogin. It is
// sensitive: the SDK never logs it or embeds it in an error, and the value is
// held only in memory for the lifetime of the Client. Prefer WithEnv or
// platform secret storage over hardcoding it. An empty value leaves the
// password unset.
func WithTradePassword(password string) Option {
	return func(c *Config) { c.TradePassword = password }
}

// WithPlatformPublicKey overrides the base64 SPKI platform RSA public key used
// for opt-in push-frame verification, for rotation or for an environment whose
// key is not bundled. The keys are public reference data, not secrets. An empty
// value restores the bundled test key (types.PlatformPublicKeyTest).
func WithPlatformPublicKey(key string) Option {
	return func(c *Config) {
		if key == "" {
			c.PlatformPublicKey = types.PlatformPublicKeyTest
			return
		}
		c.PlatformPublicKey = key
	}
}

// WithEnv reads the HSTONG_* environment variables documented in env.go and
// applies them to the configuration. Because it is an option, it participates
// in the normal in-order application: New(WithEnv(), WithBaseURL(u)) lets the
// explicit WithBaseURL win, while New(WithBaseURL(u), WithEnv()) lets the
// environment win. An unset or empty variable leaves the corresponding default
// or earlier value in place.
//
// WithEnv can fail: an unparsable duration or boolean, a malformed URL, or a
// malformed push address records an error that New returns instead of a
// Client. See env.go for the exact variables and formats.
func WithEnv() Option {
	return func(c *Config) { applyEnv(c) }
}

// WithLogger installs a structured logger for SDK diagnostics. The default is
// nil, which is inert: the SDK logs nothing. The logger receives operation and
// category labels only, never request payloads or secrets. A nil logger
// restores the inert default.
func WithLogger(l *slog.Logger) Option {
	return func(c *Config) { c.Logger = l }
}

// WithMetrics installs a metrics recorder for SDK measurements. The default is
// nil, which is inert: the SDK records nothing. The recorder receives
// operation, category, outcome, state, address, and error-kind labels only,
// never payloads or secrets. A nil recorder restores the inert default.
func WithMetrics(rec Recorder) Option {
	return func(c *Config) { c.Metrics = rec }
}

// WithRetryPolicy installs a retry policy for read-only query calls. The
// default is nil, which issues exactly one attempt. Order, futures, and algo
// mutations are never retried regardless of the policy, because the client
// derives the retry class from the endpoint path via the closed mutation set
// (docs/adr/0003-no-auto-retry-orders.md). Passing the zero RetryPolicy also
// results in exactly one attempt.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Config) { c.RetryPolicy = &p }
}

// WithRateLimiter installs a token-bucket rate limiter. The default is nil (no
// rate limit). A nil limiter restores the default.
func WithRateLimiter(l *RateLimiter) Option {
	return func(c *Config) { c.RateLimiter = l }
}

// WithCircuitBreaker installs a circuit breaker. The default is nil (no
// breaker). A nil breaker restores the default.
//
// Deprecated: this sets both QueryBreaker and MutationBreaker to b for backward
// compatibility. Use WithQueryBreaker or WithMutationBreaker to set only one.
func WithCircuitBreaker(b *CircuitBreaker) Option {
	return func(c *Config) {
		c.CircuitBreaker = b
		c.QueryBreaker = b
		c.MutationBreaker = b
	}
}

// WithQueryBreaker installs a circuit breaker for ClassQuery calls. The default
// is nil (no breaker for queries).
func WithQueryBreaker(b *CircuitBreaker) Option {
	return func(c *Config) { c.QueryBreaker = b }
}

// WithMutationBreaker installs a circuit breaker for ClassMutation calls. The
// default is nil (no breaker for mutations).
func WithMutationBreaker(b *CircuitBreaker) Option {
	return func(c *Config) { c.MutationBreaker = b }
}
