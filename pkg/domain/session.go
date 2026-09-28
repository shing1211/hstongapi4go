// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

// This file is the session half of the domain model: the reply both
// /trade/TradeLogin and /trade/TradeLogout return, the model that reply maps
// to, and the mapper between them. It follows the split futures.go states - a
// payload the Gateway sends becomes an exported …Wire type here, an envelope
// that holds rows and every request body stay unexported in pkg/services - and
// the mapper takes one payload, never an envelope. There is no session envelope
// to keep unexported, because neither session endpoint holds rows: each returns
// a single object, so the payload is the whole reply body and the two endpoints
// share its shape.
//
// # Why TradeSession carries no credential
//
// A trade session is established by a password, and nothing on this type is one.
// The password never reaches this package: it is encrypted by internal/auth
// before it is marshalled, and the ciphertext is a transport concern that dies
// with the request. A caller that wants to know whether a session is still good
// asks IsExpired or ShouldRefresh; a caller that wants to end it calls
// Logout. There is deliberately no field here that a future edit could quietly
// turn into a place a secret is kept, and no method that returns one.
//
// # Where the mapping actually runs, and why it is split
//
// There is one reply and one model, and the code that goes from the first to the
// second is in two places rather than one, which is worth stating because it is
// not how the other mappers in this package are arranged.
//
// internal/auth's Authenticator takes its login function as an argument, so the
// reply is consumed *inside* that function, before the authenticator stores
// anything. By the time pkg/services holds a value there is a stored session and
// no reply: the expiry arithmetic has already run, and it is internal/auth's,
// because the Gateway sends no expiry. So:
//
//   - TradeSessionAcceptedFromDTO maps the reply, and runs in pkg/services
//     SessionService.wireLogin. It answers the only question the reply carries -
//     did the Gateway accept - and a refusal becomes a typed error there.
//   - pkg/services' tradeSessionFrom maps the stored session onto TradeSession.
//     It is in pkg/services rather than here because internal/auth is an
//     internal package and this one must not import anything under internal/.
//
// A single mapper from TradeSessionWire to *TradeSession, taking the two
// timestamps as arguments because the reply does not carry them, is therefore
// deliberately absent. It would have exactly one call site in its own package's
// terms and none at all, and an uncalled exported mapper in this package is the
// defect TestDomainDeclaresNoEnvelope exists to keep out: this is the layer
// whose rule is that a type or a function here has a user, not a comment
// describing one.
//
// # The token may legitimately be empty
//
// The Gateway binds a trade session to the local Gateway process rather than to
// a bearer credential this SDK can present, so the acknowledgement it sends on
// /trade/TradeLogin need not carry a token at all - and on the shape the
// released layer and the mock Gateway both send, it carries none. Token is
// therefore the zero SessionToken for a login the Gateway accepted without
// issuing one, and that is the honest value rather than a placeholder: a
// fabricated identifier would read as a credential the SDK does not hold.
// internal/auth still needs a string to store, so it stores this one, and the
// expiry arithmetic - which is what ShouldRefresh and IsExpired read - does not
// depend on it.

// TradeSessionWire is the reply body of /trade/TradeLogin and
// /trade/TradeLogout. One object, two fields, and no rows.
//
// # The two fields are not redundant, and the disagreement is documented
//
// The two sources this repository can check disagree about the shape:
//
//   - The released pkg/hstong/session.go and the mock Gateway both send
//     {"success": true} for the two endpoints. That is the parity anchor, and it
//     is what Success decodes.
//   - The Java vendor (HSQuantOpenApiHandle.tradeLogin) deserializes the login
//     reply as CommonStringVo, which is {"data": "..."}, and the Python vendor's
//     demo prints data for both endpoints. Java's CommonBoolVo, which carries
//     Success, is the type it uses for the logout.
//
// So the login reply is a string body on one reading and a boolean on the other,
// and this SDK decodes both rather than choosing. The reason is the failure
// direction, which is the argument C1b and C2a §9.2 make: a build that read
// only Success would report a *successful* Java-dialect login as a rejection,
// which is a wrong, near-undiagnosable answer, while a build that reads both
// reports the two shapes accurately and still refuses an explicit
// {"success": false}. Nothing is papered over.
//
// Success is a *bool rather than a bool for one reason: encoding/json decodes an
// absent bool field to false, so a plain bool cannot tell {"success": false} -
// an explicit refusal - from a reply that carries no success field at all. Only
// the pointer distinguishes them, and the distinction decides whether a
// data-only reply is an acceptance. See TradeSessionAcceptedFromDTO.
type TradeSessionWire struct {
	// Data is the reply's string body. On the Java vendor's reading of the login
	// reply it is the value the Gateway returns, which is the only place a token
	// could come from. Empty when the Gateway sent no string.
	Data string `json:"data"`
	// Success is the reply's boolean body, and nil when the reply carried no
	// success field. A nil Success is not a refusal: it is an absence, and
	// TradeSessionAcceptedFromDTO decides what an absence means.
	Success *bool `json:"success"`
}

// TradeSession is a live trade session: the account it is filed under, the token
// the Gateway issued (which, as the file header records, it may not have issued
// at all), and the two instants that govern its life as Unix seconds.
//
// ExpiresAt is the instant the session stops being usable and RefreshAt is the
// instant it becomes due to be re-established. RefreshAt is strictly before
// ExpiresAt for every session internal/auth issues, so ShouldRefresh subsumes
// IsExpired: a session that is past its expiry is always also past its refresh
// point. A caller that wants to be conservative asks ShouldRefresh; a caller
// that wants to know whether the session is dead asks IsExpired.
//
// The lifetime is the SDK's, not the protocol's. The Gateway sends no expiry,
// so these two numbers come from the token TTL internal/auth applies and from
// the clock it was given; a caller that needs the real answer asks the Gateway.
type TradeSession struct {
	// AccountID is the local session key the session is filed under. It never
	// crosses the wire: /trade/TradeLogin and /trade/TradeLogout carry no
	// account field, and the Gateway resolves the account from the credential.
	// It is the value every other method in this layer takes as its first
	// argument, which is the whole reason Login has to take one.
	AccountID AccountID
	// Token is the value the Gateway's reply carried, or the zero SessionToken
	// when it carried none. It is not a bearer credential this SDK can present
	// on a later request; see the file header.
	Token SessionToken
	// ExpiresAt is the Unix second at which the session stops being usable. A
	// session is expired at this instant, not one second later.
	ExpiresAt int64
	// RefreshAt is the Unix second from which the session is due to be
	// re-established, which is before ExpiresAt. A session is due for refresh
	// at this instant, not one second later.
	RefreshAt int64
}

// IsExpired reports whether now is at or past the session's expiry.
//
// The comparison is a pure integer test against the supplied instant, with no
// monotonic component, so a wall clock that moves backwards makes a session
// that had expired look live again. That is a known property of the model and is
// pinned in internal/auth's own security tests, not introduced here; the predicate
// is duplicated rather than shared only because internal/auth is an internal
// package and this value is what a caller outside the module receives.
// TestSessionPredicatesAgreeWithInternalAuth runs the two against each other and
// is the control that keeps them from drifting.
func (s TradeSession) IsExpired(now int64) bool {
	return now >= s.ExpiresAt
}

// ShouldRefresh reports whether now is at or past the point at which the
// session should be re-established, which is before it expires.
//
// It is the predicate that makes a refresh a *decision* rather than a reaction:
// asking it at any point inside the session's life costs nothing, and acting on
// a true answer re-authenticates while the current session is still usable. The
// same duplication and the same control as IsExpired apply.
func (s TradeSession) ShouldRefresh(now int64) bool {
	return now >= s.RefreshAt
}

// TradeSessionAcceptedFromDTO reports whether the Gateway accepted the session
// request carried by v.
//
// Four replies are distinguishable, and each is decided rather than defaulted:
//
//   - nil, or a reply carrying neither field: not accepted. A reply that says
//     nothing is not an acknowledgement, and the released layer already reads
//     such a reply as a refusal.
//   - {"success": true}: accepted.
//   - {"success": false}: not accepted. An explicit refusal is never read as an
//     acceptance, which is the row that matters: a build that treated any
//     absence as acceptance would log a caller in on a refusal.
//   - {"data": "..."} with no success field: accepted, on the Java vendor's
//     reading of the login reply. This is the one row the released layer's
//     {"success": true} shape does not reach, and it is a real reply shape
//     rather than a hypothetical; see TradeSessionWire.
func TradeSessionAcceptedFromDTO(v *TradeSessionWire) bool {
	if v == nil {
		return false
	}
	if v.Success != nil {
		return *v.Success
	}
	return v.Data != ""
}
