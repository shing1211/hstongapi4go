// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// fakeExecutor is an Executor that records what it was asked for and replies
// with a canned body. Its existence is the point of the Executor interface: the
// services can be exercised without a Gateway, which is the dependency inversion
// ADR 0010 asks for.
type fakeExecutor struct {
	calls []fakeCall
	// reply is marshalled into the caller's out value. A non-nil err is returned
	// instead, so a test can drive the error path.
	reply any
	err   error
}

type fakeCall struct {
	op     string
	route  client.Route
	params any
}

func (f *fakeExecutor) JSON() client.Codec { return nil }

func (f *fakeExecutor) Do(_ context.Context, op string, route client.Route, params any, _ client.Codec, out any) error {
	f.calls = append(f.calls, fakeCall{op: op, route: route, params: params})
	if f.err != nil {
		return f.err
	}
	if out == nil || f.reply == nil {
		return nil
	}
	raw, err := json.Marshal(f.reply)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// last returns the most recent recorded call.
func (f *fakeExecutor) last(t *testing.T) fakeCall {
	t.Helper()
	if len(f.calls) == 0 {
		t.Fatal("no call was recorded")
	}
	return f.calls[len(f.calls)-1]
}

// TestServicesAcceptAnyExecutor is the inversion test proper: a type that has
// nothing to do with client.Client satisfies the constructors, which is only
// possible because they depend on the interface rather than the concrete type.
func TestServicesAcceptAnyExecutor(t *testing.T) {
	f := &fakeExecutor{}
	if NewAccountService(f) == nil {
		t.Error("NewAccountService returned nil for a valid Executor")
	}
	if NewMarketService(f) == nil {
		t.Error("NewMarketService returned nil for a valid Executor")
	}
	if NewTradingService(f) == nil {
		t.Error("NewTradingService returned nil for a valid Executor")
	}
}

// TestServiceUsesTheInjectedExecutor checks a service really routes
// through the injected executor rather than reaching for a global or a package
// variable, which is the failure the interface is meant to make impossible.
//
// MarketService.Subscribe is used because it passes a nil out to the executor, so
// the assertion is about routing and nothing else. Decoding into a domain value
// would drag the decimal constructors into the test, and those panic by design
// on an unparseable field.
func TestServiceUsesTheInjectedExecutor(t *testing.T) {
	f := &fakeExecutor{reply: map[string]any{}}
	svc := NewMarketService(f)

	if err := svc.Subscribe(t.Context(), types.TopicBasicQot, &Security{
		DataType: 10000, Code: "00700.HK",
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	got := f.last(t)
	if got.op != opSubscribe {
		t.Errorf("op = %q, want %q", got.op, opSubscribe)
	}
	if got.route != client.RouteHqSubscribe {
		t.Errorf("route = %q, want %q", got.route, client.RouteHqSubscribe)
	}
}

// TestServicePropagatesExecutorError asserts the service surfaces the
// executor's error rather than swallowing it and returning a zero response,
// which would look like a successful empty result to the caller. Subscribe is
// used because it passes a nil out, so the assertion is purely about error
// propagation.
func TestServicePropagatesExecutorError(t *testing.T) {
	sentinel := errors.New("gateway unavailable")
	svc := NewMarketService(&fakeExecutor{err: sentinel})

	err := svc.Subscribe(t.Context(), types.TopicBasicQot, &Security{
		DataType: 10000, Code: "00700.HK",
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Subscribe error = %v, want the executor's error", err)
	}
}

// TestExecutorValidationRunsBeforeTheExecutor pins the ordering: argument
// validation must reject a bad request without spending a round trip. If the
// executor were consulted first, an empty security list would reach the Gateway.
func TestExecutorValidationRunsBeforeTheExecutor(t *testing.T) {
	f := &fakeExecutor{}
	svc := NewMarketService(f)

	if err := svc.Subscribe(t.Context(), types.TopicBasicQot); err == nil {
		t.Fatal("Subscribe with an empty security list = nil, want a validation error")
	}
	if len(f.calls) != 0 {
		t.Errorf("the executor was called %d time(s) for an invalid request, want 0", len(f.calls))
	}
}

// TestWithClientOptionsOverrideTheInjectedExecutor covers the three option
// constructors, which exist to swap the executor after construction. If an option
// silently did nothing, a caller would keep talking to the wrong request path.
// The assertion is on the field rather than through a service method, so it is
// independent of each method's own argument validation.
func TestWithClientOptionsOverrideTheInjectedExecutor(t *testing.T) {
	first := &fakeExecutor{}
	second := &fakeExecutor{}

	if got := NewAccountService(first).client; got != Executor(first) {
		t.Error("NewAccountService did not use the constructor argument")
	}
	if got := NewAccountService(first, WithAccountClient(second)).client; got != Executor(second) {
		t.Error("WithAccountClient did not replace the executor")
	}
	if got := NewMarketService(first).client; got != Executor(first) {
		t.Error("NewMarketService did not use the constructor argument")
	}
	if got := NewMarketService(first, WithMarketClient(second)).client; got != Executor(second) {
		t.Error("WithMarketClient did not replace the executor")
	}
	if got := NewTradingService(first).client; got != Executor(first) {
		t.Error("NewTradingService did not use the constructor argument")
	}
	if got := NewTradingService(first, WithTradingClient(second)).client; got != Executor(second) {
		t.Error("WithTradingClient did not replace the executor")
	}
}

// TestReleasedClientSatisfiesExecutor is the compile-time guarantee restated as
// a runtime one, so a future signature change in client.Client fails here with a
// readable message rather than only at the var _ assertion in executor.go.
func TestReleasedClientSatisfiesExecutor(t *testing.T) {
	c, err := client.New(client.WithEnv())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defer func() { _ = c.Close() }()

	var ex Executor = c

	// A service built from the real client must accept it, which is the
	// production wiring the whole interface exists to support.
	if NewMarketService(ex) == nil {
		t.Fatal("NewMarketService returned nil for the released client")
	}
	if NewAccountService(ex) == nil {
		t.Fatal("NewAccountService returned nil for the released client")
	}
	if NewTradingService(ex) == nil {
		t.Fatal("NewTradingService returned nil for the released client")
	}

	// JSON must be usable through the interface, since every call site passes
	// s.client.JSON() as the codec.
	if ex.JSON() == nil {
		t.Error("JSON() through the interface returned nil")
	}
}
