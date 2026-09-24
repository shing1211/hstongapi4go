// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

//go:build otel

package otel

import (
	"context"
	"errors"

	"github.com/shing1211/hstongapi4go/internal/logging"
	"github.com/shing1211/hstongapi4go/internal/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/shing1211/hstongapi4go"
	meterName  = "github.com/shing1211/hstongapi4go"
)

func init() {
	otel.Tracer(tracerName)
	otel.Meter(meterName)
}

func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

func Meter() metric.Meter {
	return otel.Meter(meterName)
}

func Propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}

func SetTracerProvider(tp trace.TracerProvider) {
	otel.SetTracerProvider(tp)
}

func SetMeterProvider(mp metric.MeterProvider) {
	otel.SetMeterProvider(mp)
}

func SetPropagator(p propagation.TextMapPropagator) {
	otel.SetTextMapPropagator(p)
}

func Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	Propagator().Inject(ctx, carrier)
}

func Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return Propagator().Extract(ctx, carrier)
}

func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// SafeError returns err with any credential-shaped assignment in its message
// replaced by logging.Mask. A nil error is returned unchanged. The result is
// for recording only: it does not wrap err, so errors.Is and errors.As do not
// traverse it.
func SafeError(err error) error {
	if err == nil {
		return nil
	}
	safe := logging.RedactText(err.Error())
	if safe == err.Error() {
		return err
	}
	return errors.New(safe)
}

// EndSpan records err on span and ends it. The error text is passed through
// SafeError first, so a secret carried in an error message cannot reach a
// trace, whatever produced the error.
func EndSpan(span trace.Span, err error) {
	if err != nil {
		safe := SafeError(err)
		span.SetStatus(codes.Error, safe.Error())
		span.RecordError(safe)
	}
	span.End()
}

func AttrOp(op string) attribute.KeyValue {
	return attribute.String("hstong.operation", op)
}

func AttrEndpoint(path string) attribute.KeyValue {
	return attribute.String("hstong.endpoint", path)
}

func AttrMarket(market string) attribute.KeyValue {
	return attribute.String("hstong.market", market)
}

func AttrCategory(cat string) attribute.KeyValue {
	return attribute.String("hstong.category", cat)
}

func AttrOutcome(outcome string) attribute.KeyValue {
	return attribute.String("hstong.outcome", outcome)
}

func AttrAddr(addr string) attribute.KeyValue {
	return attribute.String("hstong.addr", addr)
}

func AttrKind(kind string) attribute.KeyValue {
	return attribute.String("hstong.kind", kind)
}

type otelHook struct {
	meter      metric.Meter
	counters   map[string]metric.Int64Counter
	histograms map[string]metric.Float64Histogram
	gauges     map[string]metric.Float64Gauge
}

func newOTelHook() *otelHook {
	return &otelHook{
		meter:      Meter(),
		counters:   make(map[string]metric.Int64Counter),
		histograms: make(map[string]metric.Float64Histogram),
		gauges:     make(map[string]metric.Float64Gauge),
	}
}

func (h *otelHook) Record(name string, kind metrics.Kind, value float64, labels ...string) {
	if h.meter == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, len(labels)/2)
	for i := 0; i < len(labels); i += 2 {
		if i+1 < len(labels) {
			attrs = append(attrs, attribute.String(labels[i], labels[i+1]))
		}
	}
	opts := metric.WithAttributes(attrs...)
	switch kind {
	case metrics.KindCounter:
		h.getCounter(name).Add(context.Background(), int64(value), opts)
	case metrics.KindHistogram:
		h.getHistogram(name).Record(context.Background(), value, opts)
	case metrics.KindGauge:
		h.getGauge(name).Record(context.Background(), value, opts)
	}
}

func (h *otelHook) getCounter(name string) metric.Int64Counter {
	if c, ok := h.counters[name]; ok {
		return c
	}
	c, _ := h.meter.Int64Counter(name)
	h.counters[name] = c
	return c
}

func (h *otelHook) getHistogram(name string) metric.Float64Histogram {
	if hist, ok := h.histograms[name]; ok {
		return hist
	}
	hist, _ := h.meter.Float64Histogram(name)
	h.histograms[name] = hist
	return hist
}

func (h *otelHook) getGauge(name string) metric.Float64Gauge {
	if g, ok := h.gauges[name]; ok {
		return g
	}
	g, _ := h.meter.Float64Gauge(name)
	h.gauges[name] = g
	return g
}

func Hook() metrics.Hook {
	return newOTelHook()
}

type TracerMiddleware struct {
	tracer trace.Tracer
}

func NewTracerMiddleware() *TracerMiddleware {
	return &TracerMiddleware{tracer: Tracer()}
}

func (m *TracerMiddleware) Start(ctx context.Context, op, path string) (context.Context, trace.Span) {
	return m.tracer.Start(ctx, op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			AttrOp(op),
			AttrEndpoint(path),
		))
}

func End(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}
