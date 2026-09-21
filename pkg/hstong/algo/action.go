// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package algo

// Action is an operation submitted to /trade/AlgoActionOrder to control a live
// master order (算法订单的启动、停止、暂停、恢复). The reference documentation
// (operate-algo.html) enumerates exactly four codes, so the set is treated as
// closed for local validation: an unlisted value is rejected before any request
// is sent, not forwarded. If the vendor extends the set, this constant group
// and Action.Valid must be updated together.
type Action string

const (
	// ActionStart ("1") starts a stopped or newly created strategy.
	ActionStart Action = "1"
	// ActionStop ("2") stops the strategy, cancelling its outstanding children.
	ActionStop Action = "2"
	// ActionSuspend ("3") pauses the strategy without cancelling its children.
	ActionSuspend Action = "3"
	// ActionResume ("4") resumes a suspended strategy.
	ActionResume Action = "4"
)

// String returns the documented action name ("START", "STOP", "SUSPEND",
// "RESUME"), or "Action(<raw>)" for a value outside the documented set. It
// never panics and never returns an empty string for a non-empty code.
func (a Action) String() string {
	switch a {
	case ActionStart:
		return "START"
	case ActionStop:
		return "STOP"
	case ActionSuspend:
		return "SUSPEND"
	case ActionResume:
		return "RESUME"
	default:
		return "Action(" + string(a) + ")"
	}
}

// Valid reports whether a is one of the four documented action codes. It is the
// gate used by ActionOrderParams validation and by the test suite; an invalid
// action never reaches the Gateway.
func (a Action) Valid() bool {
	switch a {
	case ActionStart, ActionStop, ActionSuspend, ActionResume:
		return true
	default:
		return false
	}
}
