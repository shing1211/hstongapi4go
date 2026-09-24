// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

//go:build otel

package client

import (
	"github.com/shing1211/hstongapi4go/internal/metrics"
	"github.com/shing1211/hstongapi4go/internal/otel"
)

func OTelHook() metrics.Hook {
	return otel.Hook()
}
