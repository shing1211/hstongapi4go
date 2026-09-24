// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/internal/logging"
)

// TestMain verifies the logging helpers leave no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNewNeverNil(t *testing.T) {
	if got := logging.New(nil); got == nil {
		t.Fatal("New(nil) returned nil")
	}
	if got := logging.Discard(); got == nil {
		t.Fatal("Discard returned nil")
	}
	l := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if got := logging.New(l); got != l {
		t.Fatal("New(non-nil) did not return the supplied logger")
	}
}

func TestSubAddsSubsystem(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, nil))
	logging.Sub(l, logging.SubsystemPush).Info("connected")
	if !strings.Contains(buf.String(), "hstong.push") {
		t.Fatalf("subsystem missing from %q", buf.String())
	}
	if logging.Sub(nil, logging.SubsystemHTTP) == nil {
		t.Fatal("Sub(nil, ...) returned nil")
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for _, key := range []string{
		"password", "Password", "tradePassword", "trade_password",
		"trade-password", "token", "accessToken", "EncryptedKey",
		"encrypted_key", "apiKey", "privateKey", "authorization",
		"accountId", "account_id", "fundAccount", "fund_account",
	} {
		if !logging.IsSensitiveKey(key) {
			t.Errorf("IsSensitiveKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"user", "code", "route", "op", "category", "stockCode", "account", "passwordHash"} {
		if logging.IsSensitiveKey(key) {
			t.Errorf("IsSensitiveKey(%q) = true, want false", key)
		}
	}
}

func TestRedactValueNeverRevealsLength(t *testing.T) {
	if got := logging.RedactValue(""); got != "" {
		t.Errorf("RedactValue(\"\") = %q, want empty", got)
	}
	for _, secret := range []string{"hunter2", "a", "a-very-long-token-value"} {
		if got := logging.RedactValue(secret); got != logging.Mask {
			t.Errorf("RedactValue(%q) = %q, want %q", secret, got, logging.Mask)
		}
	}
}

func TestRedactAttrs(t *testing.T) {
	attrs := logging.RedactAttrs([]slog.Attr{
		slog.String("tradePassword", "hunter2"),
		slog.String("user", "alice"),
		slog.Group("creds", slog.String("token", "abc123"), slog.String("scope", "read")),
	})
	if attrs[0].Value.String() != logging.Mask {
		t.Errorf("password attr = %q, want %q", attrs[0].Value.String(), logging.Mask)
	}
	if attrs[1].Value.String() != "alice" {
		t.Errorf("user attr = %q, want alice", attrs[1].Value.String())
	}
	group := attrs[2].Value.Group()
	if group[0].Value.String() != logging.Mask {
		t.Errorf("group token = %q, want %q", group[0].Value.String(), logging.Mask)
	}
	if group[1].Value.String() != "read" {
		t.Errorf("group scope = %q, want read", group[1].Value.String())
	}
}

func TestRedactingHandlerMasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logging.Redacting(slog.NewTextHandler(&buf, nil)))
	logger.LogAttrs(nil, slog.LevelInfo, "trade login",
		slog.String("tradePassword", "hunter2"),
		slog.String("accessToken", "tok_secret_123"),
		slog.String("EncryptedKey", "ZW5jcnlwdGVk"),
		slog.String("user", "alice"),
	)
	out := buf.String()
	for _, secret := range []string{"hunter2", "tok_secret_123", "ZW5jcnlwdGVk"} {
		if strings.Contains(out, secret) {
			t.Fatalf("log output leaked %q: %s", secret, out)
		}
	}
	if strings.Count(out, logging.Mask) != 3 {
		t.Fatalf("log output = %s, want three %q masks", out, logging.Mask)
	}
	if !strings.Contains(out, "user=alice") {
		t.Fatalf("non-sensitive attribute missing: %s", out)
	}
}

func TestRedactingHandlerWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logging.Redacting(slog.NewTextHandler(&buf, nil))).
		With("token", "with-secret").
		WithGroup("cfg")
	logger.Info("config", "password", "group-secret", "baseURL", "http://127.0.0.1:11111")
	out := buf.String()
	if strings.Contains(out, "with-secret") || strings.Contains(out, "group-secret") {
		t.Fatalf("log output leaked a secret: %s", out)
	}
	if !strings.Contains(out, "http://127.0.0.1:11111") {
		t.Fatalf("non-sensitive value missing: %s", out)
	}
}

func TestRedactNilValue(t *testing.T) {
	attr := logging.Redact("token", nil)
	if attr.Value.String() != "" {
		t.Fatalf("Redact(token, nil) = %q, want empty", attr.Value.String())
	}
}

func TestRedactTextMasksSecrets(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"query form", "password=hunter2", "password=***"},
		{"colon form", "apiKey: sk-live-1", "apiKey: ***"},
		{"json form", `{"token":"abc.def"}`, `{"token":"***"}`},
		{"single quoted json", `{'secret':'top-secret'}`, `{'secret':'***'}`},
		{"comma terminated", "user=bob,password=pw1,op=x", "user=bob,password=***,op=x"},
		{"two secrets", "token=aaa secret=bbb", "token=*** secret=***"},
		{"bearer credential", "Authorization: Bearer tok-9f8e7d", "Authorization: ***"},
		{"basic credential", "Authorization: Basic dXNlcjpwdw==", "Authorization: ***"},
		{"bearer then field", "Authorization: Bearer abc, op=x", "Authorization: ***, op=x"},
		{"trailing separator", "password=", "password="},
		{"key with no value", "password", "password"},
		{"empty quoted value", `{"token":""}`, `{"token":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logging.RedactText(tt.in); got != tt.want {
				t.Errorf("RedactText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactTextLeavesNonSecrets(t *testing.T) {
	for _, in := range []string{
		`{"user":"bob","stockCode":"00700"}`,
		"code=10000&route=/hq/BasicQot",
		"passwordHash=abc",
		"myTokenCount=5",
		"plain text with no assignments",
	} {
		if got := logging.RedactText(in); got != in {
			t.Errorf("RedactText(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestRedactTextNeverRevealsSecretLength(t *testing.T) {
	short := logging.RedactText("token=a")
	long := logging.RedactText("token=aaaaaaaaaaaaaaaa")
	if short != long {
		t.Errorf("masked output varies with secret length: %q vs %q", short, long)
	}
}
