// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package logging provides the SDK's structured-logging helpers: a never-nil
// *slog.Logger factory, per-subsystem child loggers, and a redacting handler
// that masks secrets before they reach a handler.
//
// The SDK never logs secrets. Callers that pass their own values to a logger
// should wrap the handler with Redacting (or use Redact / RedactAttrs) so that
// trade passwords, tokens, and encrypted keys are masked even when a helper
// forwards arbitrary attributes.
package logging
