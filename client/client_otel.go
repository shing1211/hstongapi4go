// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

//go:build otel

package client

import (
	"context"

	"github.com/shing1211/hstongapi4go/internal/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	spanFactory = otelSpanFactory
}

func otelSpanFactory(ctx context.Context, op string) (context.Context, func(error)) {
	tracer := otel.Tracer()
	ctx, span := tracer.Start(ctx, op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			otel.AttrOp(op),
		))
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}
