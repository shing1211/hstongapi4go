// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/shing1211/hstongapi4go/internal/metrics"
)

// TestMain verifies the metrics helpers leave no goroutines behind.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// sample is one recorded measurement.
type sample struct {
	name   string
	kind   metrics.Kind
	value  float64
	labels []string
}

// recorder captures measurements in memory for assertions.
type recorder struct {
	samples []sample
}

func (r *recorder) Count(name string, value int64, labels ...string) {
	r.samples = append(r.samples, sample{name, metrics.KindCounter, float64(value), labels})
}

func (r *recorder) Observe(name string, value float64, labels ...string) {
	r.samples = append(r.samples, sample{name, metrics.KindHistogram, value, labels})
}

func (r *recorder) Gauge(name string, value float64, labels ...string) {
	r.samples = append(r.samples, sample{name, metrics.KindGauge, value, labels})
}

// find returns the first sample with the given name.
func (r *recorder) find(t *testing.T, name string) sample {
	t.Helper()
	for _, s := range r.samples {
		if s.name == name {
			return s
		}
	}
	t.Fatalf("no sample named %q in %+v", name, r.samples)
	return sample{}
}

func TestNopIsSafe(t *testing.T) {
	var n metrics.Nop
	n.Count("x", 1, "k", "v")
	n.Observe("x", 1, "k", "v")
	n.Gauge("x", 1, "k", "v")
}

func TestHookRecorderMapsKinds(t *testing.T) {
	rec := &recorder{}
	h := metrics.HookRecorder(hookFunc(func(name string, kind metrics.Kind, value float64, labels ...string) {
		rec.samples = append(rec.samples, sample{name, kind, value, labels})
	}))
	h.Count("c", 3, "a", "b")
	h.Observe("o", 1.5, "a", "b")
	h.Gauge("g", 2.5, "a", "b")

	if rec.samples[0].kind != metrics.KindCounter || rec.samples[0].value != 3 {
		t.Fatalf("Count sample = %+v", rec.samples[0])
	}
	if rec.samples[1].kind != metrics.KindHistogram || rec.samples[1].value != 1.5 {
		t.Fatalf("Observe sample = %+v", rec.samples[1])
	}
	if rec.samples[2].kind != metrics.KindGauge || rec.samples[2].value != 2.5 {
		t.Fatalf("Gauge sample = %+v", rec.samples[2])
	}
	if strings.Join(rec.samples[0].labels, ",") != "a,b" {
		t.Fatalf("labels = %v, want [a b]", rec.samples[0].labels)
	}
}

// hookFunc adapts a function to the Hook interface.
type hookFunc func(string, metrics.Kind, float64, ...string)

func (f hookFunc) Record(name string, kind metrics.Kind, value float64, labels ...string) {
	f(name, kind, value, labels...)
}

func TestHookRecorderNilIsNop(t *testing.T) {
	rec := metrics.HookRecorder(nil)
	rec.Count("x", 1)
	rec.Observe("x", 1)
	rec.Gauge("x", 1)
}

func TestInstrumentsHTTP(t *testing.T) {
	rec := &recorder{}
	in := metrics.NewInstruments(rec)

	in.HTTPRequest("/hq/BasicQot")
	req := rec.find(t, metrics.MetricHTTPRequests)
	if req.kind != metrics.KindCounter || req.value != 1 {
		t.Fatalf("requests sample = %+v", req)
	}
	if got := strings.Join(req.labels, ","); got != "op,/hq/BasicQot" {
		t.Fatalf("request labels = %q", got)
	}

	in.HTTPError("/hq/BasicQot", "timeout")
	errSample := rec.find(t, metrics.MetricHTTPErrors)
	if got := strings.Join(errSample.labels, ","); got != "op,/hq/BasicQot,category,timeout" {
		t.Fatalf("error labels = %q", got)
	}

	in.HTTPLatency("/hq/BasicQot", 12*time.Millisecond, errors.New("boom"))
	lat := rec.find(t, metrics.MetricHTTPLatency)
	if lat.kind != metrics.KindHistogram || lat.value != 12 {
		t.Fatalf("latency sample = %+v", lat)
	}
	if got := strings.Join(lat.labels, ","); got != "op,/hq/BasicQot,outcome,error" {
		t.Fatalf("latency labels = %q", got)
	}

	in.HTTPLatency("/hq/BasicQot", time.Millisecond, nil)
	if got := strings.Join(rec.samples[len(rec.samples)-1].labels, ","); got != "op,/hq/BasicQot,outcome,ok" {
		t.Fatalf("ok latency labels = %q", got)
	}
}

func TestInstrumentsOrderAndBreakerAndPush(t *testing.T) {
	rec := &recorder{}
	in := metrics.NewInstruments(rec)

	in.OrderOutcome("/trade/TradeEntrust", "ok")
	if s := rec.find(t, metrics.MetricOrderOutcomes); s.value != 1 {
		t.Fatalf("order outcome sample = %+v", s)
	}

	in.BreakerState("/trade", "half-open")
	if s := rec.find(t, metrics.MetricBreakerState); s.kind != metrics.KindGauge || s.value != 1 {
		t.Fatalf("half-open gauge = %+v", s)
	}
	in.BreakerState("/trade", "open")
	if got := rec.samples[len(rec.samples)-1].value; got != 2 {
		t.Fatalf("open gauge = %v, want 2", got)
	}

	in.PushConnect("127.0.0.1:11112")
	in.PushReconnect("127.0.0.1:11112")
	in.PushError("127.0.0.1:11112", "decode")
	for _, name := range []string{metrics.MetricPushConnects, metrics.MetricPushReconnects, metrics.MetricPushErrors} {
		if s := rec.find(t, name); s.value != 1 {
			t.Fatalf("%s sample = %+v", name, s)
		}
	}
}

func TestRateLimitWaitSkipsZero(t *testing.T) {
	rec := &recorder{}
	in := metrics.NewInstruments(rec)
	in.RateLimitWait("/hq/BasicQot", 0)
	if len(rec.samples) != 0 {
		t.Fatalf("zero wait recorded %+v", rec.samples)
	}
	in.RateLimitWait("/hq/BasicQot", 25*time.Millisecond)
	if s := rec.find(t, metrics.MetricRateLimitWaits); s.value != 1 {
		t.Fatalf("wait sample = %+v", s)
	}
	if s := rec.find(t, metrics.MetricRateLimitWaitDuration); s.value != 25 {
		t.Fatalf("wait duration sample = %+v", s)
	}
}

func TestInstrumentsNilRecorderAndNilReceiver(t *testing.T) {
	in := metrics.NewInstruments(nil)
	if in.Recorder() == nil {
		t.Fatal("Recorder() = nil, want Nop")
	}
	in.HTTPRequest("x")

	var nilIn *metrics.Instruments
	nilIn.HTTPRequest("x")
	nilIn.HTTPError("x", "unknown")
	nilIn.BreakerState("x", "closed")
}

func TestKindString(t *testing.T) {
	cases := map[metrics.Kind]string{
		metrics.KindCounter:   "counter",
		metrics.KindGauge:     "gauge",
		metrics.KindHistogram: "histogram",
		metrics.Kind(99):      "unknown",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
