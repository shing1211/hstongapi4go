// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/internal/logging"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

const (
	// DefaultBaseURL is the local HStong Gateway HTTP endpoint used when no
	// WithBaseURL option is supplied.
	DefaultBaseURL = "http://127.0.0.1:11111"
	// DefaultTimeout is the request timeout applied when no WithDefaultTimeout
	// option is supplied. It is used both as the *http.Client timeout and as
	// the envelope's timeout_sec value.
	DefaultTimeout = 10 * time.Second
	// DefaultMaxResponseBytes is the largest response body Transport will read
	// when no WithMaxResponseBytes option is supplied. Gateway responses are
	// JSON documents over loopback; the largest documented endpoint (a day's
	// ticker or time-share series) is orders of magnitude below this, so the cap
	// bounds a malformed or hostile body without constraining real traffic.
	DefaultMaxResponseBytes int64 = 8 << 20 // 8 MiB
)

// nullLiteral is the JSON null payload, which some endpoints return in place of
// an object. A caller that supplied a non-nil out leaves it untouched in that
// case.
var nullLiteral = []byte("null")

// request is the Gateway HTTP request envelope. Params is the codec-encoded
// endpoint body and is omitted entirely when there are no parameters.
type request struct {
	// TimeoutSec is the Gateway-side call budget in whole seconds.
	TimeoutSec int `json:"timeout_sec"`
	// Params is the endpoint-specific, codec-encoded request body.
	Params json.RawMessage `json:"params,omitempty"`
}

// response is the Gateway HTTP response envelope. Data is decoded by the
// endpoint's codec only after OK has been checked.
type response struct {
	// OK reports whether the Gateway processed the call successfully.
	OK bool `json:"ok"`
	// Err carries the Gateway's error text (often a status code, or a status
	// code followed by a message) when OK is false.
	Err string `json:"err"`
	// Data is the endpoint-specific, codec-encoded response body.
	Data json.RawMessage `json:"data"`
}

// Config holds the resolved settings for a Transport. It is populated by the
// With... options and consumed by New; callers do not construct it directly.
type Config struct {
	// BaseURL is the Gateway root, for example "http://127.0.0.1:11111".
	BaseURL string
	// HTTPClient is the client used to issue requests. A nil value yields a
	// client with the resolved default timeout.
	HTTPClient *http.Client
	// DefaultTimeout is used both as the client timeout and as the envelope
	// timeout_sec, unless the caller supplied a client with an explicit timeout.
	DefaultTimeout time.Duration
	// MaxResponseBytes is the largest response body that will be read. A body
	// above the cap is rejected with a typed error instead of being buffered.
	MaxResponseBytes int64
}

// Option mutates a Config. Options are applied in order on top of the defaults
// in New.
type Option func(*Config)

// WithBaseURL sets the Gateway root URL. The default is DefaultBaseURL. A
// trailing slash is trimmed. An empty value restores the default.
func WithBaseURL(baseURL string) Option {
	return func(c *Config) { c.BaseURL = baseURL }
}

// WithHTTPClient sets the *http.Client used for requests. The default is a
// client with DefaultTimeout. When the supplied client has no Timeout set, a
// copy is given the resolved default timeout; a client with an explicit timeout
// is used unchanged.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) { c.HTTPClient = client }
}

// WithDefaultTimeout sets the request timeout used both as the *http.Client
// timeout and as the envelope's timeout_sec. The default is DefaultTimeout.
// Non-positive values restore the default.
func WithDefaultTimeout(timeout time.Duration) Option {
	return func(c *Config) { c.DefaultTimeout = timeout }
}

// WithMaxResponseBytes sets the largest response body that will be read. The
// default is DefaultMaxResponseBytes. A non-positive value restores the default
// rather than disabling the cap: an unbounded read is never selected by
// accident, and a caller that genuinely needs a larger ceiling can set it
// explicitly.
func WithMaxResponseBytes(n int64) Option {
	return func(c *Config) { c.MaxResponseBytes = n }
}

// Transport executes Gateway HTTP requests. It is immutable after New and safe
// for concurrent use; it holds no per-request state. Each Do call issues
// exactly one HTTP attempt and never retries (docs/adr/0003-no-auto-retry-orders.md).
type Transport struct {
	baseURL          string
	client           *http.Client
	defaultTimeout   time.Duration
	maxResponseBytes int64
}

// New builds a Transport from opts, applying them in order on top of
// DefaultBaseURL and DefaultTimeout. It never returns nil and never panics.
// The returned Transport is safe for concurrent use.
func New(opts ...Option) *Transport {
	cfg := Config{
		BaseURL:          DefaultBaseURL,
		DefaultTimeout:   DefaultTimeout,
		MaxResponseBytes: DefaultMaxResponseBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = DefaultTimeout
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = DefaultMaxResponseBytes
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	if client.Timeout == 0 {
		cloned := *client
		cloned.Timeout = cfg.DefaultTimeout
		client = &cloned
	}

	return &Transport{
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		client:           client,
		defaultTimeout:   cfg.DefaultTimeout,
		maxResponseBytes: cfg.MaxResponseBytes,
	}
}

// Do performs one POST to baseURL+path and decodes the response into out.
//
// params is encoded with codec (JSONCodec when codec is nil) and sent as the
// envelope's params; a nil params is omitted. The envelope's timeout_sec is the
// transport default, reduced to the whole seconds remaining on ctx's deadline
// when that deadline is sooner, and never below 1.
//
// A non-2xx HTTP status, a malformed envelope, or an ok:false response is
// returned as a typed *errs.Error tagged with op. When out is non-nil and the
// response data is absent or JSON null, out is left untouched and Do returns
// nil. Do issues exactly one HTTP request; it never retries.
func (t *Transport) Do(ctx context.Context, op, path string, params any, codec Codec, out any) error {
	if codec == nil {
		codec = JSONCodec{}
	}

	var rawParams json.RawMessage
	if params != nil {
		encoded, err := codec.Marshal(params)
		if err != nil {
			return errs.Wrap(err, "", op, "encode params")
		}
		rawParams = encoded
	}

	envelope, err := json.Marshal(request{
		TimeoutSec: t.timeoutSeconds(ctx),
		Params:     rawParams,
	})
	if err != nil {
		return errs.Wrap(err, "", op, "encode request envelope")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(envelope))
	if err != nil {
		return errs.Wrap(err, "", op, "build request")
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(req)
	if err != nil {
		return transportError(op, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	// Bound the read so a malformed or hostile Gateway response cannot force an
	// unbounded allocation. MaxBytesReader reports *http.MaxBytesError, which is
	// translated below into a typed error rather than a generic read failure.
	body, err := io.ReadAll(http.MaxBytesReader(nil, httpResp.Body, t.maxResponseBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errs.New(types.StatusCode(""), op, fmt.Sprintf(
				"gateway response exceeds the %d byte cap", t.maxResponseBytes))
		}
		return readError(op, err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return errs.New(types.StatusCode(""), op, fmt.Sprintf(
			"gateway returned HTTP %d %s%s",
			httpResp.StatusCode, http.StatusText(httpResp.StatusCode), bodySnippet(body)))
	}

	var env response
	if err := json.Unmarshal(body, &env); err != nil {
		return errs.Wrap(err, "", op, "decode response envelope")
	}

	if !env.OK {
		return failure(op, env.Err)
	}

	if out != nil {
		data := bytes.TrimSpace(env.Data)
		if len(data) == 0 || bytes.Equal(data, nullLiteral) {
			return nil
		}
		if err := codec.Unmarshal(data, out); err != nil {
			return errs.Wrap(err, "", op, "decode response data")
		}
	}
	return nil
}

// timeoutSeconds returns the envelope timeout_sec: the transport default,
// shortened to the whole seconds remaining on ctx's deadline when that is
// sooner, and never below 1.
func (t *Transport) timeoutSeconds(ctx context.Context) int {
	secs := int(t.defaultTimeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := int(time.Until(deadline) / time.Second)
		if remaining < 1 {
			remaining = 1
		}
		if remaining < secs {
			secs = remaining
		}
	}
	return secs
}

// transportError maps an error from *http.Client.Do to a typed error. A
// deadline produces a Timeout (CategoryTimeout); every other client failure is
// a Connection failure. Neither is retried here.
func transportError(op string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errs.Timeout(op, err)
	}
	return errs.Connection(op, err)
}

// readError maps a body-read failure to a typed error, classifying a deadline
// as a timeout and any other read failure as a connection error.
func readError(op string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errs.Timeout(op, err)
	}
	return errs.Connection(op, fmt.Errorf("read response body: %w", err))
}

// failure converts an ok:false envelope into a typed error. The status code is
// recovered from errText when it is (or starts with) a documented code; the
// message is the code's documented text, any trailing Gateway text, or a
// generic message when errText is empty.
func failure(op, errText string) *errs.Error {
	code, message := classifyFailure(errText)
	if message == "" {
		message = "gateway reported failure with no error message"
	}
	return errs.New(code, op, message)
}

// classifyFailure extracts a documented status code from the envelope's err
// field. The Gateway may report a bare code ("1012"), a code plus text
// ("1012 not logged in"), or free text with no code. It returns the empty code
// and the trimmed text when no documented code is found.
func classifyFailure(errText string) (types.StatusCode, string) {
	trimmed := strings.TrimSpace(errText)
	if trimmed == "" {
		return "", ""
	}
	if code := types.StatusCode(trimmed); errs.KnownCode(code) {
		return code, errs.MessageForCode(code)
	}
	end := 0
	for end < len(trimmed) && trimmed[end] >= '0' && trimmed[end] <= '9' {
		end++
	}
	if end >= 4 && end <= 5 {
		code := types.StatusCode(trimmed[:end])
		if errs.KnownCode(code) {
			rest := strings.TrimSpace(strings.TrimLeft(trimmed[end:], " :-ï¼š"))
			if rest == "" {
				rest = errs.MessageForCode(code)
			}
			return code, rest
		}
	}
	return "", trimmed
}

// bodySnippet returns a short, single-line excerpt of an error response body
// for a non-2xx error message. It is empty for an empty body. Values assigned
// to a sensitive key are masked with logging.Mask before the excerpt is
// returned, so an echoing Gateway or proxy cannot inject a password or token
// into the returned error and, through it, into a recorded span.
func bodySnippet(body []byte) string {
	const maxSnippet = 256
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	text = logging.RedactText(text)
	if len(text) > maxSnippet {
		text = text[:maxSnippet] + "..."
	}
	return ": " + strings.ReplaceAll(text, "\n", " ")
}
