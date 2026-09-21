// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/internal/metrics"
	"github.com/shing1211/hstongapi4go/internal/resilience"
	"github.com/shing1211/hstongapi4go/internal/transport"
)

// ErrClientClosed is returned by Client.Do after Client.Close. It is wrapped
// with the operation name so errors.Is(err, ErrClientClosed) keeps working.
var ErrClientClosed = errors.New("client: client is closed")

// Client is the SDK entry point. It holds the resolved configuration and the
// HTTP transport, dispatches calls to canonical Gateway routes with the
// endpoint's codec, and closes cleanly. A Client is immutable after New and
// safe for concurrent use by multiple goroutines.
//
// A Client issues exactly one HTTP request per Do call; it never retries
// (docs/adr/0003-no-auto-retry-orders.md). Retry and re-login policy for query
// endpoints is applied above the client by later layers, never inside it.
type Client struct {
	cfg         Config
	transport   *transport.Transport
	instruments *metrics.Instruments

	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// New builds a Client from opts, applying them in order on top of the defaults
// returned by defaultConfig. An explicit option placed after WithEnv overrides
// the corresponding environment value; one placed before it does not.
//
// New returns an error, never panics, when the resolved configuration is
// invalid: an unparsable environment value recorded by WithEnv, a non-http(s)
// or host-less base URL, or a malformed push address. The returned *Client is
// nil on error.
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if err := validatePushAddr(cfg.PushAddr); err != nil {
		return nil, err
	}

	cfg.HTTPClient = resolveHTTPClient(cfg.HTTPClient, cfg.Timeout)

	c := &Client{
		cfg: cfg,
		transport: transport.New(
			transport.WithBaseURL(cfg.BaseURL),
			transport.WithHTTPClient(cfg.HTTPClient),
			transport.WithDefaultTimeout(cfg.Timeout),
		),
	}
	if cfg.Metrics != nil {
		c.instruments = metrics.NewInstruments(cfg.Metrics)
	}
	return c, nil
}

// resolveHTTPClient returns the *http.Client the Client will use. A nil client
// becomes a fresh client over a cloned *http.Transport with the resolved
// timeout and no shared global state. A client with no explicit timeout but a
// concrete transport is copied and given the resolved timeout. A client with an
// explicit timeout is used unchanged.
func resolveHTTPClient(client *http.Client, timeout time.Duration) *http.Client {
	if client == nil {
		return &http.Client{Transport: cloneDefaultTransport(), Timeout: timeout}
	}
	if client.Timeout == 0 && client.Transport == nil {
		cloned := *client
		cloned.Transport = cloneDefaultTransport()
		cloned.Timeout = timeout
		return &cloned
	}
	if client.Timeout == 0 {
		cloned := *client
		cloned.Timeout = timeout
		return &cloned
	}
	return client
}

// cloneDefaultTransport returns an independent copy of the process default
// HTTP transport. Owning the transport lets Close release idle connections
// without disturbing http.DefaultTransport, which other code shares.
func cloneDefaultTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		return base.Clone()
	}
	return &http.Transport{}
}

// Do issues one POST to route and decodes the response data into out.
//
// The route is validated first; an unregistered route returns an error wrapping
// ErrUnknownRoute before any request is sent. op is a human-readable label for
// the operation (for example "trade/TradeEntrust") used in error messages and
// never sent on the wire. params is encoded with codec and sent as the
// envelope's params; a nil codec defaults to the transport's JSON codec. The
// codec parameter is the public client.Codec alias, so callers pass the value
// returned by JSON without naming an internal type. out may be nil to discard
// the response data.
//
// Do delegates to the transport, which issues exactly one HTTP attempt unless a
// caller installed a RetryPolicy (and the route is a read-only query), a
// RateLimiter, or a CircuitBreaker with the corresponding option. With the
// defaults it never retries, never rate-limits, and never trips a breaker. A
// closed Client returns an error wrapping ErrClientClosed.
func (c *Client) Do(ctx context.Context, op string, route Route, params any, codec Codec, out any) error {
	if c.closed.Load() {
		return fmt.Errorf("%w: %s", ErrClientClosed, op)
	}
	if err := route.Validate(); err != nil {
		return err
	}

	start := time.Now()
	if c.cfg.Logger != nil {
		c.cfg.Logger.Debug("http request", "op", op, "route", route.Path())
	}
	err := c.execute(ctx, op, route, params, codec, out)
	if c.instruments != nil {
		c.instruments.HTTPRequest(op)
		c.instruments.HTTPLatency(op, time.Since(start), err)
		if err != nil {
			c.instruments.HTTPError(op, string(errs.CategoryOf(err)))
		}
		if resilience.IsMutation(route.Path()) {
			c.instruments.OrderOutcome(op, outcomeLabel(err))
		}
	}
	if err != nil && c.cfg.Logger != nil {
		c.cfg.Logger.Debug("http error", "op", op, "category", string(errs.CategoryOf(err)))
	}
	return err
}

// execute runs the optional rate limiter, circuit breaker, and retry policy
// around one transport call. With no options installed it issues exactly one
// attempt, matching the SDK's default behaviour. The retry class is derived
// from the endpoint path, so a mutation is never retried.
func (c *Client) execute(ctx context.Context, op string, route Route, params any, codec Codec, out any) error {
	path := route.Path()

	// Route to the operation-specific breaker if set, otherwise fall back to the
	// legacy combined CircuitBreaker for backward compatibility.
	var cb *CircuitBreaker
	if resilience.IsMutation(path) {
		cb = c.cfg.MutationBreaker
		if cb == nil {
			cb = c.cfg.CircuitBreaker
		}
	} else {
		cb = c.cfg.QueryBreaker
		if cb == nil {
			cb = c.cfg.CircuitBreaker
		}
	}
	if cb != nil && !cb.Allow() {
		return fmt.Errorf("%w: %s", resilience.ErrCircuitOpen, op)
	}

	attempt := func(ctx context.Context) error {
		if c.cfg.RateLimiter != nil {
			limitStart := time.Now()
			if werr := c.cfg.RateLimiter.WaitEndpoint(ctx, path); werr != nil {
				return werr
			}
			if c.instruments != nil {
				c.instruments.RateLimitWait(op, time.Since(limitStart))
			}
		}
		return c.transport.Do(ctx, op, path, params, codec, out)
	}

	var err error
	if c.cfg.RetryPolicy != nil {
		_, err = c.cfg.RetryPolicy.DoRoute(ctx, route.Path(), attempt)
	} else {
		err = attempt(ctx)
	}

	if cb != nil {
		if err != nil {
			cb.OnFailure()
		} else {
			cb.OnSuccess()
		}
	}
	return err
}

// outcomeLabel maps an error to the "ok"/"error" outcome label.
func outcomeLabel(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}

// JSON returns the Codec for hand-written JSON request and response bodies
// (trade, futures, algo, assets, and session surfaces). The returned value is a
// stateless singleton and is safe for concurrent use.
func (c *Client) JSON() Codec {
	return transport.JSONCodec{}
}

// Close releases the client's idle HTTP connections and marks it closed. It is
// idempotent and safe to call concurrently: the first call performs the release
// and every call returns the same result. After Close, Do returns an error
// wrapping ErrClientClosed. Close does not close an *http.Client supplied via
// WithHTTPClient; it only releases that client's idle connections when its
// transport supports it.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.cfg.HTTPClient != nil {
			if closer, ok := c.cfg.HTTPClient.Transport.(interface{ CloseIdleConnections() }); ok {
				closer.CloseIdleConnections()
			}
		}
	})
	return c.closeErr
}

// BaseURL returns the resolved Gateway HTTP root.
func (c *Client) BaseURL() string { return c.cfg.BaseURL }

// PushAddr returns the resolved Gateway TCP push address in "host:port" form.
func (c *Client) PushAddr() string { return c.cfg.PushAddr }

// Timeout returns the resolved per-request timeout, used both as the HTTP
// client timeout and as the envelope's timeout_sec.
func (c *Client) Timeout() time.Duration { return c.cfg.Timeout }

// TradePassword returns the configured plaintext trade password. It is
// sensitive: do not log it, print it, or include it in error messages or
// telemetry. It returns the empty string when unset.
func (c *Client) TradePassword() string { return c.cfg.TradePassword }

// PlatformPublicKey returns the configured base64 SPKI platform RSA public key
// used for opt-in push verification. It is public reference data, not a secret.
func (c *Client) PlatformPublicKey() string { return c.cfg.PlatformPublicKey }

// VerifyPush reports whether push-frame signature verification is enabled. It
// is off unless WithEnv read HSTONG_VERIFY_PUSH or a later phase enables it.
func (c *Client) VerifyPush() bool { return c.cfg.VerifyPush }

// HTTPClient returns the *http.Client used for requests. It is never nil on a
// Client built by New and is safe for concurrent use.
func (c *Client) HTTPClient() *http.Client { return c.cfg.HTTPClient }
