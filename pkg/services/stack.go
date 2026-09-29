// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"sync"
)

// StackExecutor is the request path NewStack composes over, plus the lifecycle it
// may own.
//
// It exists because Executor alone is not enough. Executor is exactly Do and JSON
// (see executor.go) and has no Close, so a Stack built on it would own nothing it
// could release: the caller would still have to close the client separately, and a
// Stack that advertises one Close while owning none is a promise the compiler
// cannot keep.
//
// *client.Client satisfies it, and it is the intended value. A test fake needs one
// extra method, which sequencedExecutor in testsupport_test.go carries.
type StackExecutor interface {
	Executor
	// Close releases the request path's resources. It must be idempotent, as
	// *client.Client.Close is: Stack.Close may be called any number of times and
	// will not call it more than once itself, but a caller that also closes the
	// client must not be punished for it.
	Close() error
}

// Stack is the v-next service set over one request path, closed as a unit.
//
// Every field is non-nil after construction except Push, which is nil unless a
// push transport was supplied. The six services are the ones this package already
// ships; the Stack adds no behaviour of its own beyond ownership and a single
// Close. It names no client.Route, so it is invisible to the parity guard, which
// credits a route to the method that references it.
//
// A Stack is safe for concurrent use once constructed. Construction itself must
// not race with use: the services it returns share the one request path.
type Stack struct {
	Market  *MarketService
	Account *AccountService
	Trading *TradingService
	Algo    *AlgoService
	Futures *FuturesService
	Session *SessionService

	// Push is nil unless WithPushTransport was given. A nil Push is not a failure
	// state: most callers never want the TCP push connection, and constructing one
	// by default would make every caller own a socket it did not ask for.
	Push *PushOrchestration

	exec StackExecutor

	// pushTransport is retained unexported rather than read back out of Push,
	// because the push options are applied before Push exists and the two halves
	// are set independently. Reading Push would couple construction order to the
	// field's value.
	pushTransport PushTransport

	closeOnce sync.Once
	closeErr  error
}

// StackOption configures a Stack. Options are applied in order over the defaults,
// and a nil option is skipped rather than panicking, so a conditionally-built
// option slice is safe to pass through.
type StackOption func(*Stack)

// WithPushTransport opts the Stack into push.
//
// tr is the TCP half. It is not dialled here and no request is issued: construct
// the Stack, then call Connect and Run on the PushOrchestration it produces.
//
// A nil tr is the documented way to say "no push", and is identical to omitting
// the option.
func WithPushTransport(tr PushTransport) StackOption {
	return func(s *Stack) { s.pushTransport = tr }
}

// WithStackExecutor replaces the request path after the services are built.
//
// It exists so a test can substitute a fake without rebuilding the composition,
// and it accepts the same shape as the constructor argument, so it cannot install
// a value the constructor would have rejected. The six services keep the executor
// they were constructed with, so an option applied after them does not rewire the
// services - it changes only what Stack.Close releases. Callers that want the
// services themselves to use the new executor should pass it to NewStack.
func WithStackExecutor(c StackExecutor) StackOption {
	return func(s *Stack) { s.exec = c }
}

// NewStack returns the v-next services over c, configured by opts.
//
// It never dials and never issues a request. The returned Stack is the opt-in
// entry point to this layer: everything in it is reachable only because a caller
// called this function.
//
// The two push halves are the Stack's own Market and Trading services, because
// they already satisfy TopicSubscriber and OrderPushSubscriber structurally
// (see the var _ assertions in push.go). The trade half arrives as the
// WithTradeSubscriber *option* rather than as a parameter, which is not obvious
// from NewPushOrchestration's signature and is easy to get wrong: passing it
// positionally does not compile, and were the arity to line up it would compile
// while leaving SubscribeOrders with no route to /trade/TradeSubscribe.
func NewStack(c StackExecutor, opts ...StackOption) *Stack {
	s := &Stack{
		Market:  NewMarketService(c),
		Account: NewAccountService(c),
		Trading: NewTradingService(c),
		Algo:    NewAlgoService(c),
		Futures: NewFuturesService(c),
		Session: NewSessionService(c),
		exec:    c,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.pushTransport != nil {
		s.Push = NewPushOrchestration(s.Market, s.pushTransport,
			WithTradeSubscriber(s.Trading))
	}
	return s
}

// Close releases the Stack: push first, then the request path, returning the first
// error while still attempting both.
//
// Push goes first because it is the resource whose failure mode is a real socket:
// PushOrchestration.Close closes the PushTransport it owns, so closing the adapter
// separately would double-close a component that is already idempotent, and would
// imply an ownership the orchestrator actually has. Closing the request path first
// would leave a live TCP connection pointed at a closed HTTP client.
//
// The first error wins, and both closes are always attempted: stopping after a
// failed push close would trade a socket leak for an HTTP-connection leak.
// PushOrchestration.Close returns nil today, so in practice the request path's
// error is the one that surfaces; the rule is kept because that is a property of
// the orchestrator, not a guarantee Stack can rely on.
//
// Close is idempotent: the second and later calls return the first call's result
// without touching anything. It never panics, including on a nil *Stack, on a
// Stack with no push, and on one built over a nil executor - all reachable states.
func (s *Stack) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.Push != nil {
			s.closeErr = s.Push.Close()
		}
		if s.exec == nil {
			return
		}
		err := s.exec.Close()
		if s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}
