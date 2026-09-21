// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned by callers (for example the client) when the
// circuit breaker refuses to admit a call. It is wrapped with the operation
// name so errors.Is(err, ErrCircuitOpen) keeps working.
var ErrCircuitOpen = errors.New("resilience: circuit breaker is open")

// State is the circuit-breaker state.
type State int

const (
	// StateClosed admits calls and counts failures. It is the initial state.
	StateClosed State = iota
	// StateOpen refuses calls until the cooldown elapses.
	StateOpen
	// StateHalfOpen admits a limited number of probe calls to test recovery.
	StateHalfOpen
)

// String returns the lower-case name of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Defaults applied by NewBreaker for non-positive arguments.
const (
	// DefaultFailureThreshold is the consecutive-failure count that opens the
	// breaker when NewBreaker is given a non-positive threshold.
	DefaultFailureThreshold = 5
	// DefaultCooldown is how long the breaker stays open when NewBreaker is
	// given a non-positive cooldown.
	DefaultCooldown = 30 * time.Second
)

// Breaker is a thread-safe circuit breaker with the classic closed / open /
// half-open states.
//
// While closed it admits every call and counts consecutive failures; reaching
// the failure threshold opens it. While open it refuses calls until the
// cooldown elapses, then moves to half-open. While half-open it admits at most
// HalfOpenProbes calls; a success closes it and resets the failure count, a
// failure reopens it and restarts the cooldown.
//
// A Breaker is safe for concurrent use. It is inert unless a caller installs
// one with client.WithCircuitBreaker.
type Breaker struct {
	mu             sync.Mutex
	state          State
	threshold      int
	cooldown       time.Duration
	halfOpenProbes int
	failures       int
	probes         int
	openedAt       time.Time
	now            func() time.Time
}

// NewBreaker returns a closed breaker with the given consecutive-failure
// threshold and cooldown. A non-positive threshold becomes
// DefaultFailureThreshold and a non-positive cooldown becomes DefaultCooldown.
// The breaker admits a single half-open probe after the cooldown.
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = DefaultFailureThreshold
	}
	if cooldown <= 0 {
		cooldown = DefaultCooldown
	}
	return &Breaker{
		state:          StateClosed,
		threshold:      threshold,
		cooldown:       cooldown,
		halfOpenProbes: 1,
		now:            time.Now,
	}
}

// Allow reports whether a call may proceed now. From the open state it moves to
// half-open once the cooldown has elapsed; from half-open it admits up to the
// configured probe count.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == StateOpen {
		if b.now().Sub(b.openedAt) < b.cooldown {
			return false
		}
		b.state = StateHalfOpen
		b.probes = 0
	}
	if b.state == StateHalfOpen {
		if b.probes >= b.halfOpenProbes {
			return false
		}
		b.probes++
		return true
	}
	return true
}

// OnSuccess records a successful call. It closes the breaker from half-open and
// resets the failure count. A success observed while the breaker is open (an
// in-flight call that started before it opened) is ignored, so a stale call
// cannot close a freshly opened breaker.
func (b *Breaker) OnSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateOpen {
		return
	}
	b.state = StateClosed
	b.failures = 0
	b.probes = 0
}

// OnFailure records a failed call. In the closed state it opens the breaker
// once the failure threshold is reached; in half-open it reopens immediately
// and restarts the cooldown; in the open state it leaves the breaker open.
func (b *Breaker) OnFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateHalfOpen:
		b.state = StateOpen
		b.openedAt = b.now()
		b.probes = 0
		b.failures++
	case StateClosed:
		b.failures++
		if b.failures >= b.threshold {
			b.state = StateOpen
			b.openedAt = b.now()
		}
	case StateOpen:
	}
}

// State returns the current state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Failures returns the current consecutive-failure count. It is reset to zero
// when the breaker closes.
func (b *Breaker) Failures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures
}
