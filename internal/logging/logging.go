// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Mask is the replacement text written in place of a redacted value. It is a
// fixed string, so it never reveals the length or content of the secret.
const Mask = "***"

// Subsystem names attached by Sub. They are stable, dot-separated prefixes so
// consumers can filter and route by component.
const (
	// SubsystemHTTP labels HTTP transport and client activity.
	SubsystemHTTP = "hstong.http"
	// SubsystemSession labels trade-session and re-login activity.
	SubsystemSession = "hstong.session"
	// SubsystemPush labels the TCP push channel.
	SubsystemPush = "hstong.push"
	// SubsystemStream labels the public stream API.
	SubsystemStream = "hstong.stream"
	// SubsystemRateLimit labels rate-limiter decisions.
	SubsystemRateLimit = "hstong.ratelimit"
	// SubsystemBreaker labels circuit-breaker state changes.
	SubsystemBreaker = "hstong.breaker"
	// SubsystemError labels error-path logging.
	SubsystemError = "hstong.error"
	// SubsystemConfig labels configuration resolution.
	SubsystemConfig = "hstong.config"
)

// sensitiveKeys is the normalized key set that triggers redaction. Keys are
// compared after lower-casing and removing '_' and '-', so "trade_password",
// "tradePassword", and "Trade-Password" all match.
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"tradepassword": {},
	"token":         {},
	"accesstoken":   {},
	"refreshtoken":  {},
	"encryptedkey":  {},
	"secret":        {},
	"apikey":        {},
	"privatekey":    {},
	"authorization": {},
	"signature":     {},
	"cookie":        {},
	"sessionid":     {},
	"credential":    {},
	"credentials":   {},
}

// normalizeKey lower-cases key and strips separators for sensitive matching.
func normalizeKey(key string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
}

// Discard returns a non-nil logger that writes nothing. It is used wherever the
// SDK needs a logger but the caller supplied none.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// New returns l when it is non-nil and a discarding logger otherwise. It never
// returns nil, so callers may use the result without a nil check.
func New(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return Discard()
}

// Sub returns a child of l tagged with the given subsystem name. A nil l is
// treated as Discard, so the result is never nil.
func Sub(l *slog.Logger, subsystem string) *slog.Logger {
	return New(l).With("subsystem", subsystem)
}

// IsSensitiveKey reports whether key names a secret that must be redacted. The
// comparison is case-insensitive and ignores '_' and '-' separators.
func IsSensitiveKey(key string) bool {
	_, ok := sensitiveKeys[normalizeKey(key)]
	return ok
}

// RedactValue returns the masked form of a secret. An empty value returns the
// empty string; every other value returns Mask, so neither the content nor the
// length of the secret is exposed.
func RedactValue(value string) string {
	if value == "" {
		return ""
	}
	return Mask
}

// Redact returns the attribute to log for key. A sensitive key is logged as a
// string with its value masked; every other key is logged unchanged.
func Redact(key string, value any) slog.Attr {
	if !IsSensitiveKey(key) {
		return slog.Any(key, value)
	}
	if value == nil {
		return slog.String(key, "")
	}
	return slog.String(key, RedactValue(fmt.Sprint(value)))
}

// RedactAttrs returns a copy of attrs with every sensitive value masked. Group
// attributes are recursed into.
func RedactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return out
}

// redactAttr masks a, recursing into groups.
func redactAttr(a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, RedactValue(a.Value.String()))
	}
	if a.Value.Kind() == slog.KindGroup {
		a.Value = slog.GroupValue(RedactAttrs(a.Value.Group())...)
	}
	return a
}

// Redacting wraps h so that any attribute whose key is sensitive is masked
// before it reaches h. It applies to values passed in the record, to attributes
// attached with WithAttrs, and to nested groups. It is the recommended wrapper
// when a caller forwards externally supplied attributes to the SDK logger.
func Redacting(h slog.Handler) slog.Handler {
	if h == nil {
		h = slog.NewTextHandler(io.Discard, nil)
	}
	return redactingHandler{Handler: h}
}

// redactingHandler is the slog.Handler created by Redacting.
type redactingHandler struct {
	slog.Handler
}

// Handle redacts the record's attributes and forwards it.
func (h redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redactAttr(a))
		return true
	})
	return h.Handler.Handle(ctx, nr)
}

// WithAttrs redacts the attached attributes before forwarding.
func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactingHandler{Handler: h.Handler.WithAttrs(RedactAttrs(attrs))}
}

// WithGroup forwards the group unchanged; attributes added inside it are
// redacted by Handle or by the derived WithAttrs.
func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{Handler: h.Handler.WithGroup(name)}
}
