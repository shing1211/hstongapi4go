// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"sync"
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies the package leaves no goroutines behind. The state machine
// spawns none; the check is cheap insurance for later additions.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestState_String pins the log labels, including the out-of-range fallback.
func TestState_String(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  string
	}{
		{"logged out", LoggedOut, "logged_out"},
		{"logging in", LoggingIn, "logging_in"},
		{"logged in", LoggedIn, "logged_in"},
		{"out of range", State(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// startLoggedIn returns a Machine already in LoggedIn.
func startLoggedIn() *Machine {
	m := &Machine{}
	m.BeginLogin()
	m.LoginSucceeded()
	return m
}

// startLoggingIn returns a Machine already in LoggingIn.
func startLoggingIn() *Machine {
	m := &Machine{}
	m.BeginLogin()
	return m
}

// TestMachine_Transitions drives the legal and illegal transitions from every
// starting state. The zero-value Machine must behave as LoggedOut.
func TestMachine_Transitions(t *testing.T) {
	tests := []struct {
		name      string
		start     func() *Machine
		run       func(t *testing.T, m *Machine)
		wantState State
	}{
		{
			name:      "zero value is logged out",
			start:     func() *Machine { return &Machine{} },
			run:       func(t *testing.T, m *Machine) {},
			wantState: LoggedOut,
		},
		{
			name:  "begin login from logged out wins",
			start: func() *Machine { return &Machine{} },
			run: func(t *testing.T, m *Machine) {
				if !m.BeginLogin() {
					t.Fatal("BeginLogin() = false, want true")
				}
			},
			wantState: LoggingIn,
		},
		{
			name:  "begin login while logging in loses",
			start: startLoggingIn,
			run: func(t *testing.T, m *Machine) {
				if m.BeginLogin() {
					t.Fatal("BeginLogin() = true, want false")
				}
			},
			wantState: LoggingIn,
		},
		{
			name:  "begin login while logged in loses",
			start: startLoggedIn,
			run: func(t *testing.T, m *Machine) {
				if m.BeginLogin() {
					t.Fatal("BeginLogin() = true, want false")
				}
			},
			wantState: LoggedIn,
		},
		{
			name:  "login succeeded from logging in",
			start: startLoggingIn,
			run: func(t *testing.T, m *Machine) {
				m.LoginSucceeded()
			},
			wantState: LoggedIn,
		},
		{
			name:  "login succeeded from logged out is a no-op",
			start: func() *Machine { return &Machine{} },
			run: func(t *testing.T, m *Machine) {
				m.LoginSucceeded()
			},
			wantState: LoggedOut,
		},
		{
			name:  "login succeeded from logged in stays logged in",
			start: startLoggedIn,
			run: func(t *testing.T, m *Machine) {
				m.LoginSucceeded()
			},
			wantState: LoggedIn,
		},
		{
			name:  "login failed from logging in returns to logged out",
			start: startLoggingIn,
			run: func(t *testing.T, m *Machine) {
				m.LoginFailed()
			},
			wantState: LoggedOut,
		},
		{
			name:  "login failed from logged out is a no-op",
			start: func() *Machine { return &Machine{} },
			run: func(t *testing.T, m *Machine) {
				m.LoginFailed()
			},
			wantState: LoggedOut,
		},
		{
			name:  "logout from logged in",
			start: startLoggedIn,
			run: func(t *testing.T, m *Machine) {
				m.Logout()
			},
			wantState: LoggedOut,
		},
		{
			name:  "logout from logging in",
			start: startLoggingIn,
			run: func(t *testing.T, m *Machine) {
				m.Logout()
			},
			wantState: LoggedOut,
		},
		{
			name:  "logout is idempotent",
			start: func() *Machine { return &Machine{} },
			run: func(t *testing.T, m *Machine) {
				m.Logout()
				m.Logout()
			},
			wantState: LoggedOut,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.start()
			tt.run(t, m)
			if got := m.State(); got != tt.wantState {
				t.Fatalf("state after action = %v, want %v", got, tt.wantState)
			}
		})
	}
}

// TestMachine_BeginLoginSingleWinner races many goroutines on a fresh Machine
// and asserts exactly one BeginLogin returns true while the state ends in
// LoggingIn. Run with -race, this also exercises the mutex.
func TestMachine_BeginLoginSingleWinner(t *testing.T) {
	const goroutines = 128

	m := &Machine{}
	start := make(chan struct{})
	wins := make(chan struct{}, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if m.BeginLogin() {
				wins <- struct{}{}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(wins)

	var winners int
	for range wins {
		winners++
	}
	if winners != 1 {
		t.Fatalf("BeginLogin winners = %d, want exactly 1", winners)
	}
	if got := m.State(); got != LoggingIn {
		t.Fatalf("state = %v, want %v", got, LoggingIn)
	}

	// Once the winner finishes, a later BeginLogin from LoggedOut succeeds.
	m.LoginFailed()
	if !m.BeginLogin() {
		t.Fatal("BeginLogin after LoginFailed = false, want true")
	}
}
