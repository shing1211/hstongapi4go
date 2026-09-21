// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package metrics

import "time"

// Metric names emitted by the SDK. They are stable, dot-separated identifiers.
const (
	// MetricHTTPRequests counts HTTP calls that reached the transport.
	MetricHTTPRequests = "hstong.http.requests"
	// MetricHTTPErrors counts failed HTTP calls.
	MetricHTTPErrors = "hstong.http.errors"
	// MetricHTTPLatency is the end-to-end HTTP call duration in milliseconds.
	MetricHTTPLatency = "hstong.http.latency_ms"
	// MetricOrderOutcomes counts order-mutation outcomes (ok / error).
	MetricOrderOutcomes = "hstong.order.outcomes"
	// MetricRateLimitWaits counts calls that had to wait for a rate-limit
	// token.
	MetricRateLimitWaits = "hstong.ratelimit.waits"
	// MetricRateLimitWaitDuration is the rate-limit wait duration in
	// milliseconds.
	MetricRateLimitWaitDuration = "hstong.ratelimit.wait_ms"
	// MetricBreakerState reports the circuit-breaker state as a gauge: 0
	// closed, 1 half-open, 2 open.
	MetricBreakerState = "hstong.breaker.state"
	// MetricPushConnects counts successful push connections.
	MetricPushConnects = "hstong.push.connects"
	// MetricPushReconnects counts push reconnections.
	MetricPushReconnects = "hstong.push.reconnects"
	// MetricPushErrors counts push transport and decode errors.
	MetricPushErrors = "hstong.push.errors"
)

// Label keys used with the SDK's metric names.
const (
	// LabelOp is the operation or endpoint label.
	LabelOp = "op"
	// LabelCategory is the errs.Category label.
	LabelCategory = "category"
	// LabelOutcome is the success/failure outcome label.
	LabelOutcome = "outcome"
	// LabelState is the circuit-breaker state label.
	LabelState = "state"
	// LabelAddr is the peer address label.
	LabelAddr = "addr"
	// LabelKind is the error-kind label.
	LabelKind = "kind"
)

// Kind identifies a metric instrument type.
type Kind int

const (
	// KindCounter is a monotonically increasing sum.
	KindCounter Kind = iota
	// KindGauge is a value that goes up or down.
	KindGauge
	// KindHistogram is a distribution of observed values.
	KindHistogram
)

// String returns the lower-case name of the kind.
func (k Kind) String() string {
	switch k {
	case KindCounter:
		return "counter"
	case KindGauge:
		return "gauge"
	case KindHistogram:
		return "histogram"
	default:
		return "unknown"
	}
}

// Recorder is the write-side interface the SDK uses. Implementations must be
// safe for concurrent use. Labels are alternating key/value strings, for
// example ("op", "/hq/BasicQot", "category", "timeout").
type Recorder interface {
	// Count adds value to the counter named name.
	Count(name string, value int64, labels ...string)
	// Observe records value in the histogram named name.
	Observe(name string, value float64, labels ...string)
	// Gauge sets the gauge named name to value.
	Gauge(name string, value float64, labels ...string)
}

// Nop is a Recorder that discards every measurement. It is the default.
type Nop struct{}

// Count implements Recorder and does nothing.
func (Nop) Count(string, int64, ...string) {}

// Observe implements Recorder and does nothing.
func (Nop) Observe(string, float64, ...string) {}

// Gauge implements Recorder and does nothing.
func (Nop) Gauge(string, float64, ...string) {}

// Hook is the single-entry point an adapter can implement to receive every
// measurement. An OpenTelemetry adapter records a counter for KindCounter, a
// histogram for KindHistogram, and a gauge for KindGauge. Labels are the same
// alternating key/value pairs passed to the Recorder.
type Hook interface {
	// Record receives one measurement. kind selects the instrument type.
	Record(name string, kind Kind, value float64, labels ...string)
}

// HookRecorder adapts a Hook to the Recorder interface. A nil hook yields a
// Nop recorder.
func HookRecorder(h Hook) Recorder {
	if h == nil {
		return Nop{}
	}
	return hookRecorder{hook: h}
}

// hookRecorder implements Recorder over a Hook.
type hookRecorder struct {
	hook Hook
}

// Count implements Recorder.
func (r hookRecorder) Count(name string, value int64, labels ...string) {
	r.hook.Record(name, KindCounter, float64(value), labels...)
}

// Observe implements Recorder.
func (r hookRecorder) Observe(name string, value float64, labels ...string) {
	r.hook.Record(name, KindHistogram, value, labels...)
}

// Gauge implements Recorder.
func (r hookRecorder) Gauge(name string, value float64, labels ...string) {
	r.hook.Record(name, KindGauge, value, labels...)
}

// Instruments records the SDK's standard measurements through a Recorder. The
// methods take only non-sensitive labels: operation names, categories,
// outcomes, states, addresses, and error kinds. A nil Recorder is treated as
// Nop, so an Instruments value is always usable.
type Instruments struct {
	rec Recorder
}

// NewInstruments wraps rec. A nil rec becomes Nop, so the result is never nil
// and every method is safe to call.
func NewInstruments(rec Recorder) *Instruments {
	if rec == nil {
		rec = Nop{}
	}
	return &Instruments{rec: rec}
}

// Recorder returns the wrapped recorder, which is never nil.
func (i *Instruments) Recorder() Recorder {
	if i == nil || i.rec == nil {
		return Nop{}
	}
	return i.rec
}

// HTTPRequest records one HTTP call for op.
func (i *Instruments) HTTPRequest(op string) {
	i.Recorder().Count(MetricHTTPRequests, 1, LabelOp, op)
}

// HTTPError records one failed HTTP call for op under category.
func (i *Instruments) HTTPError(op, category string) {
	i.Recorder().Count(MetricHTTPErrors, 1, LabelOp, op, LabelCategory, category)
}

// HTTPLatency records the duration of one HTTP call for op, tagged with the
// outcome (ok or error).
func (i *Instruments) HTTPLatency(op string, d time.Duration, err error) {
	i.Recorder().Observe(MetricHTTPLatency, float64(d)/float64(time.Millisecond),
		LabelOp, op, LabelOutcome, outcome(err))
}

// OrderOutcome records the outcome of an order mutation for op.
func (i *Instruments) OrderOutcome(op, outcome string) {
	i.Recorder().Count(MetricOrderOutcomes, 1, LabelOp, op, LabelOutcome, outcome)
}

// RateLimitWait records that op waited d for a rate-limit token. A zero
// duration is not recorded, so a call admitted immediately produces no noise.
func (i *Instruments) RateLimitWait(op string, d time.Duration) {
	if d <= 0 {
		return
	}
	rec := i.Recorder()
	rec.Count(MetricRateLimitWaits, 1, LabelOp, op)
	rec.Observe(MetricRateLimitWaitDuration, float64(d)/float64(time.Millisecond), LabelOp, op)
}

// BreakerState records the circuit-breaker state as a gauge: 0 closed, 1
// half-open, 2 open.
func (i *Instruments) BreakerState(name, state string) {
	i.Recorder().Gauge(MetricBreakerState, breakerValue(state), LabelState, state)
}

// PushConnect records one successful push connection to addr.
func (i *Instruments) PushConnect(addr string) {
	i.Recorder().Count(MetricPushConnects, 1, LabelAddr, addr)
}

// PushReconnect records one push reconnection to addr.
func (i *Instruments) PushReconnect(addr string) {
	i.Recorder().Count(MetricPushReconnects, 1, LabelAddr, addr)
}

// PushError records one push error of kind for addr.
func (i *Instruments) PushError(addr, kind string) {
	i.Recorder().Count(MetricPushErrors, 1, LabelAddr, addr, LabelKind, kind)
}

// outcome returns the standard outcome label for err.
func outcome(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}

// breakerValue maps a breaker state name to its gauge value.
func breakerValue(state string) float64 {
	switch state {
	case "open":
		return 2
	case "half-open":
		return 1
	default:
		return 0
	}
}
