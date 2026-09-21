// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package metrics provides a dependency-free metrics layer for the HStong
// (华盛) SDK. It defines counters, gauges, and histograms, a Recorder interface
// the SDK writes through, and a Hook interface an OpenTelemetry, Prometheus, or
// in-memory adapter can implement.
//
// The SDK records no payloads and no secrets. Labels are operation names,
// categories, and outcome strings only. Recording is inert until a caller
// installs a Recorder with client.WithMetrics; the default is a no-op.
package metrics
