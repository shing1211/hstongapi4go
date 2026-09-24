// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

//go:build otel

package client

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *Config) {
		otel.SetTracerProvider(tp)
	}
}

func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *Config) {
		otel.SetMeterProvider(mp)
	}
}

func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(c *Config) {
		otel.SetTextMapPropagator(p)
	}
}
