// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package session holds the I/O-free trading-session state machine used by the
// public session manager. It contains no network or client code so the
// transition rules can be unit-tested and reasoned about in isolation.
package session

import "sync"

// State is the lifecycle phase of a trading session.
type State int

const (
	// LoggedOut is the initial and resting state. No valid trading token is
	// believed to exist. The zero value of State is LoggedOut, so a Machine's
	// zero value is ready to use without construction.
	LoggedOut State = iota
	// LoggingIn means exactly one login attempt is in flight. While in this
	// state BeginLogin returns false so no second attempt is started.
	LoggingIn
	// LoggedIn means the SDK believes a valid trading token is held.
	LoggedIn
)

// String returns a stable, lowercase label for s, suitable for logs and tests.
// It returns "unknown" for a value outside the defined range.
func (s State) String() string {
	switch s {
	case LoggedOut:
		return "logged_out"
	case LoggingIn:
		return "logging_in"
	case LoggedIn:
		return "logged_in"
	default:
		return "unknown"
	}
}

// Machine is a concurrency-safe trading-session state machine. It performs no
// I/O and has no knowledge of the Gateway; callers drive it as network calls
// complete.
//
// Invariants, all enforced under the internal mutex:
//
//   - The zero value is LoggedOut and ready to use.
//   - BeginLogin returns true if and only if the state was LoggedOut, and on
//     success moves the state to LoggingIn. Concurrent callers therefore race
//     for exactly one winner.
//   - LoginSucceeded only has an effect in LoggingIn and moves to LoggedIn; it
//     is a no-op in any other state.
//   - LoginFailed only has an effect in LoggingIn and moves back to
//     LoggedOut; it is a no-op in any other state.
//   - Logout moves to LoggedOut from any state and is idempotent.
//
// A Machine is safe for concurrent use and must not be copied after first use.
type Machine struct {
	mu    sync.Mutex
	state State
}

// State returns the current state. The zero value is LoggedOut.
func (m *Machine) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// BeginLogin attempts to start a login attempt. It returns true, after moving
// the state to LoggingIn, when the state was LoggedOut. It returns false
// without changing the state when a login is already in flight (LoggingIn) or
// the session is already established (LoggedIn). When several goroutines call
// BeginLogin concurrently from LoggedOut, exactly one receives true.
func (m *Machine) BeginLogin() (ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != LoggedOut {
		return false
	}
	m.state = LoggingIn
	return true
}

// LoginSucceeded records a successful login, moving LoggingIn to LoggedIn. It
// is a no-op when the state is not LoggingIn.
func (m *Machine) LoginSucceeded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == LoggingIn {
		m.state = LoggedIn
	}
}

// LoginFailed records a failed login, moving LoggingIn back to LoggedOut so a
// later attempt may be made. It is a no-op when the state is not LoggingIn.
func (m *Machine) LoginFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == LoggingIn {
		m.state = LoggedOut
	}
}

// Logout moves the state to LoggedOut from any state. It is idempotent and
// never blocks.
func (m *Machine) Logout() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = LoggedOut
}

// String returns the current state's label. It is a convenience for logging.
func (m *Machine) String() string {
	return m.State().String()
}
