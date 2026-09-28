// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"strings"
	"sync"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/auth"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file owns the two trade-session endpoints: /trade/TradeLogin and
// /trade/TradeLogout, SPEC §2.3 rows 12 and 13. The reply payload and the
// session model it maps to live in pkg/domain, which is the split futures.go
// states; the request bodies stay unexported here, because the layer that owns
// a request is the layer that decodes its reply.
//
// It is also the only place in the v-next layer that composes internal/auth.
// Everything else in pkg/services reaches the Gateway through an Executor and
// nothing else, so this file is the seam between the two: an injected login
// function is what turns internal/auth's transport-free authenticator into
// something that can talk to the Gateway, and the composition is one method
// value, s.wireLogin, handed to auth.NewAuthenticator. See NewSessionService.
//
// # Two facts about the protocol that are structural
//
// Neither can be checked by the mock Gateway, which answers by path and never
// inspects a request body, and each is a mistake a reader would otherwise make:
//
//   - **Neither session request carries an account id.** /trade/TradeLogin takes
//     the encrypted trade password and nothing else; /trade/TradeLogout takes no
//     body at all. The accountID argument is a session key and a local
//     precondition, which is the same rule the cash, futures and algo services
//     follow, and it is stated on every method here for the same reason: an
//     unused-looking parameter is the one a future maintainer "fixes" by adding
//     it to the wire struct, and that would leak an account identifier into every
//     session request.
//   - **Neither session request carries the encryption salt.**
//     auth.LoginRequest has an EncryptionSalt field and the service never
//     populates it, because the one source that has the corresponding field -
//     the Java vendor's TradeLoginParam.keyBase64 - explicitly nulls it before
//     building the request, because the handle encrypts the password itself and
//     sends only the ciphertext. The Python vendor sends the same single key,
//     and the released pkg/hstong/session.go sends it too. The service therefore
//     does not expose a salt option either: a caller-supplied salt is a
//     caller-supplied AES key, which is credential-shaped surface this SDK has no
//     reason to accept (ADR 0005: the Gateway owns the credentials, and the only
//     key in this process is the published protocol key inside internal/crypto).
//
// # The password is encrypted once, by internal/auth, and never logged
//
// auth.Authenticator.Login calls internal/auth.EncryptTradePassword before it
// invokes the injected login function, so s.wireLogin receives a ciphertext and
// must not encrypt a second time. The plaintext is a parameter of Login, is
// held for the duration of the call, and reaches nothing else: not the wire, not
// an error, not a returned struct, and not a log - this file contains no logging
// at all. The Gateway's own documented known-answer vector is the wire-level
// assertion that the plaintext is absent and the ciphertext is present, and
// TestSessionPasswordIsNeverOnTheWireInAnyForm is that assertion.
//
// # A session request is not an order mutation, and it retries
//
// /trade/TradeLogin and /trade/TradeLogout are not in
// internal/resilience's mutation set, so the client classifies them as queries
// and a retry policy will re-send them on a retryable rejection. That is
// correct rather than a gap: neither is an order mutation, so ADR 0003 does not
// apply, and both are idempotent in the sense that matters - re-logging in
// re-establishes the same session and re-logging out ends a session that is
// already gone. The service adds no retry of its own. The behaviour is pinned by
// TestSessionRequestsRetryAndOrderMutationsDoNot, with the read control in the
// same test, so the row is a decision rather than an accident.

// Operation labels used in errors raised by SessionService. They are the
// canonical Gateway route paths, with the same convention the released
// pkg/hstong/session.go uses, and they never carry a request payload or a
// credential.
//
// The names match the released ones exactly rather than taking an
// unprefixed spelling, because this package declares opEntrust and
// opCancelEntrust for the cash order endpoints and neither is a session call;
// a shared label would make one error indistinguishable from the other.
const (
	opTradeLogin  = "trade/TradeLogin"
	opTradeLogout = "trade/TradeLogout"
)

// sessionLoginWireRequest is the body of /trade/TradeLogin.
//
// One field, and it is the ciphertext: auth.Authenticator.Login encrypts the
// caller's password before the login function is called, so what lands here is
// already the AES-ECB/PKCS7 Base64 the Gateway expects and encrypting it again
// would produce a password the Gateway cannot decrypt.
//
// The key is password, not tradePassword. Both vendor SDKs and the released
// pkg/hstong/session.go agree on password; docs/protocol.md's curl example says
// tradePassword, which is the one spelling in this repository with no source
// behind it, and matching the three that ship the traffic is what matters.
type sessionLoginWireRequest struct {
	Password string `json:"password"`
}

// /trade/TradeLogout sends no body, and no wire struct is declared for it: an
// empty named struct would encode nothing and give a field somewhere to be added
// by accident. What the method must get right is that the params it passes are
// struct{}{} and not nil, because internal/transport drops a nil params from the
// envelope entirely (request.Params carries omitempty) and the key would be
// absent rather than an empty object. The Java and Python vendors both send an
// empty object, so the two are not the same request.

// loginAttempt is one in-flight /trade/TradeLogin, shared by every caller that
// arrived while it was running.
//
// sess and err are written once, under the gate's mutex, before done is closed,
// and read by a waiter only after done is received, so the channel close is the
// happens-before edge and no waiter needs the mutex to read them.
type loginAttempt struct {
	done chan struct{}
	sess auth.Session
	err  error
	// waiting counts the callers that have taken the join path. It is diagnostic
	// rather than load-bearing - the outcome is delivered over done either way -
	// and it exists because "is one login slow, or is it contended" is a question
	// neither the request log nor the caller can otherwise answer. See
	// loginGate.waiting.
	waiting int
}

// loginGate coalesces concurrent logins for one account into a single HTTP
// request, and it is the only lock on the login path.
//
// # Why this is not a second lock
//
// internal/auth holds three locks, and none of them is this one. Its store
// guards the session map, TokenManager's pending map is keyed by account, and
// session.Machine guards the login state transitions - but Authenticator.Login
// holds none of them across the call to the injected login function, so two
// concurrent logins reach the Gateway as two requests. The primitives that
// would close that gap (TokenManager.IsLoginInProgress, markLoginPending,
// clearLoginPending, and the sessionManager built on session.Machine) are
// unexported or write-only outside internal/auth, so the composition cannot reach
// them from here; internal/auth's own doc.go records that they have no caller
// outside its tests.
//
// This gate is therefore not a duplicate of an existing exclusion, it is the
// exclusion that does not exist yet, and it is scoped as narrowly as the
// problem: it is taken for a map lookup and for publishing a result, never
// across an HTTP call, so a slow Gateway cannot block a login for a different
// account or a Logout. The accounting is per account rather than global so two
// accounts can authenticate in parallel.
//
// A SessionService is safe for concurrent use once constructed and must not be
// copied after first use.
type loginGate struct {
	mu      sync.Mutex
	pending map[domain.AccountID]*loginAttempt
}

// newLoginGate returns a gate with no login in flight.
func newLoginGate() *loginGate {
	return &loginGate{pending: make(map[domain.AccountID]*loginAttempt)}
}

// begin either joins the attempt in flight for accountID or starts the one this
// caller will run. lead reports which: exactly one caller per attempt is a
// leader, however many raced.
//
// The mutex is released before returning in both cases. A leader must not hold
// it across the request it is about to make, and a joiner must not hold it while
// it waits for the outcome.
//
// The two paths are genuinely different outcomes and only one of them is
// guaranteed. A caller that takes the join path is concurrent with the request in
// flight, and receives that request's outcome. A caller that finds the map empty
// starts its own attempt, which is what a caller arriving after the previous
// attempt has already finished must do - the released pkg/hstong/sessionManager
// behaves the same way, because its state machine reads LoggedOut by then and
// begins a fresh login.
func (g *loginGate) begin(accountID domain.AccountID) (a *loginAttempt, lead bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.pending[accountID]; ok {
		existing.waiting++
		return existing, false
	}
	a = &loginAttempt{done: make(chan struct{})}
	g.pending[accountID] = a
	return a, true
}

// waiting reports how many callers are queued behind the login in flight for
// accountID, and zero when there is none.
//
// It is a diagnostic, not a gate: the outcome reaches a joiner over attempt.done
// whether or not anyone counts it. What it answers is the question a request log
// cannot - whether a slow login is one slow Gateway or many callers contending for
// it - and it is the only part of this gate a caller can observe without
// disturbing it, which is also what makes it usable as a test barrier.
func (g *loginGate) waiting(accountID domain.AccountID) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a, ok := g.pending[accountID]; ok {
		return a.waiting
	}
	return 0
}

// publish records the outcome of the attempt and wakes every waiter.
//
// The entry is removed before done is closed, so a caller that arrives after
// this point starts a fresh attempt rather than joining a finished one. That
// caller then re-reads the session - see establish - and finds the one the
// leader stored, so the window opens no second request; on the failure path no
// session was stored and a fresh attempt is exactly what the caller wanted.
func (g *loginGate) publish(accountID domain.AccountID, a *loginAttempt, sess auth.Session, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a.sess, a.err = sess, err
	delete(g.pending, accountID)
	close(a.done)
}

// SessionService is the use-case surface for the two trade-session endpoints.
//
// It holds the trade session, so unlike the other five services in this package
// it does have per-account state: the sessions it established, in the
// SessionStore the constructor was given, and the in-flight logins its gate is
// coalescing. It is safe for concurrent use once constructed.
//
// # accountID is a session key and never crosses the wire
//
// Every method takes a domain.AccountID first, refuses a zero one, and places
// it in no request body - the rule the other five services follow, and here it
// carries more weight than elsewhere. On every other method the account is
// resolved by the Gateway from the session this service established, so
// accountID only says which session the caller means. On Login it is the key the
// new session is *filed under*, and it is therefore the value every later call
// must pass to find it. It is not a claim about which account the Gateway
// authenticated: the Gateway authenticated the password, and a caller that files
// a session under the wrong key gets a Gateway rejection on its next call rather
// than a wrong number.
type SessionService struct {
	client Executor

	// clock measures session expiry and the refresh window. It is the same
	// clock the authenticator's token manager was given, so the service's
	// refresh decision and the timestamps on a stored session cannot disagree
	// about what time it is.
	clock auth.Clock
	// store holds the sessions. The default is internal/auth's in-memory store.
	store auth.SessionStore
	// auth performs the encryption, the store write and the expiry arithmetic.
	auth *auth.Authenticator
	// gate coalesces concurrent logins.
	gate *loginGate
}

// SessionOption configures a SessionService. Options are applied in order on
// top of the client passed to NewSessionService, and the last option that sets
// a field wins.
type SessionOption func(*SessionService)

// WithSessionClient sets the request path a SessionService issues its calls
// through. It is the same option shape the other five services expose, and it
// exists so a caller (or a test) can supply something other than a live client.
func WithSessionClient(c Executor) SessionOption {
	return func(s *SessionService) { s.client = c }
}

// WithSessionClock sets the clock a SessionService measures session expiry and
// the refresh window against, and it is the clock the stored session's own
// timestamps are computed from.
//
// It exists so a caller can substitute a clock of their own - for a test, or
// for a process whose wall clock is known to drift - and a nil value restores
// the real clock. The default is internal/auth's real clock.
//
// It does not exist to change the session's *lifetime*: internal/auth fixes that
// at three hours with a ten-minute refresh window and exposes no option to
// change either, and no such option is added here. A lifetime short enough to be
// worth configuring would be shorter than the refresh window, and then every
// session would be born already due for refresh, so Login would re-authenticate
// on every call. Time is the one knob, and moving it moves the timestamps with it.
func WithSessionClock(c auth.Clock) SessionOption {
	return func(s *SessionService) {
		if c == nil {
			c = auth.RealClock()
		}
		s.clock = c
	}
}

// WithSessionStore replaces the store a SessionService files its sessions in.
// The default is internal/auth's in-memory store, which is per-process and is
// lost when the process exits.
//
// It exists for two callers: a test that needs a store it can inspect, and a
// test that needs one that fails, because a store error is a real arm of both
// methods. A nil value restores the in-memory default.
//
// A store supplied here is shared with the authenticator the constructor builds,
// so replacing it does not leave the service reading from one store and writing
// to another.
func WithSessionStore(store auth.SessionStore) SessionOption {
	return func(s *SessionService) {
		if store == nil {
			store = auth.NewInMemoryStore()
		}
		s.store = store
	}
}

// NewSessionService returns a SessionService over c with opts applied in order.
// c is not owned by the service: closing the underlying client remains the
// caller's responsibility.
//
// Unlike the other five constructors in this package, this one builds a second
// collaborator after the options are resolved, and the order matters. The
// authenticator needs the store and the clock, and the option set has to have
// been applied before either is read, so the authenticator is constructed at the
// end of this function rather than in the struct literal. The three options
// therefore resolve into the service's fields first and the composition happens
// once, here.
//
// # How loginFn is wired
//
// The wiring is one method value. auth.NewAuthenticator takes the login function
// as its second argument - internal/auth has no client, no HTTP and no
// knowledge of the Gateway, which is what makes it testable - and this
// constructor passes s.wireLogin. The result is that the credentials never pass
// through a service-specific adapter of their own:
//
//	EncryptTradePassword  ->  s.wireLogin  ->  Executor.Do(RouteTradeLogin)
//
// and the arrow is the whole path. s.wireLogin is a method rather than a
// closure over the service, so it is bound to the instance the constructor is
// building and cannot be pointed at a different one.
//
// A nil option panics, exactly as it does in NewFuturesService, NewAlgoService
// and the other constructors in this package; the difference is deliberate
// rather than accidental and is the one place where this constructor is not
// total.
//
// The service is not required to be logged in before any other service method
// is called. Nothing here can make it so: the other services in this package
// validate their accountID and send, and the Gateway is what refuses a request
// made without a session. Gating them on a session is C10's PushOrchestration
// decision, not this file's.
func NewSessionService(c Executor, opts ...SessionOption) *SessionService {
	s := &SessionService{client: c}
	for _, opt := range opts {
		opt(s)
	}
	if s.clock == nil {
		s.clock = auth.RealClock()
	}
	if s.store == nil {
		s.store = auth.NewInMemoryStore()
	}
	s.auth = auth.NewAuthenticator(s.store, s.wireLogin, auth.WithClock(s.clock))
	s.gate = newLoginGate()
	return s
}

// Login returns the trade session for accountID, establishing one on
// /trade/TradeLogin when the service does not already hold a usable one.
//
// # What it decides, and when
//
//	usable session held  ->  it is returned; zero HTTP requests
//	no session, or one past its refresh point  ->  exactly one request
//	a request already in flight for this account  ->  it joins that one
//
// The rule is one sentence: **a live session is reused, and only a session that
// is missing or past its refresh point costs a request.** The refresh point is
// read from the session's own RefreshAt through internal/auth's ShouldRefresh,
// which opens ten minutes before the three-hour expiry, so a login renews a
// session while the current one is still usable rather than after it has failed.
//
// The alternative - re-authenticate on every call - would put a credential on
// the wire on every call, and a caller that got there by accident would be
// hammering a login endpoint. The cost of the reuse policy is stated rather than
// hidden: **Login will not re-send a credential for a session it still holds**,
// so a caller that has changed its trade password must obtain a new service
// (whose store is empty) to force a fresh authentication. That is the one case
// where the reuse is wrong, and the released layer avoids it by having Login
// always re-attempt; the divergence is deliberate, and its escape hatch is a
// new SessionService rather than a flag, because a flag is the kind of thing
// that gets left at its default and then surprises a caller.
//
// # Concurrent calls issue one request
//
// N concurrent Login calls for one account produce exactly one
// /trade/TradeLogin. One caller runs the request; every other joins it and
// receives the leader's outcome - the same session on success, the same error on
// failure. A joiner whose own context is cancelled returns that cancellation and
// nothing else, and the leader is unaffected: it is the one whose context drives
// the request, which is also how the released layer's single-flight behaves.
//
// A failed login is not retried, by this method or by any other in this package.
// The next call starts a fresh attempt, and a caller that wants the Gateway to
// be asked again must call again.
//
// # What is never conflated
//
// Three outcomes look alike from outside and are kept apart, because each asks
// the caller for something different:
//
//   - **A locally refused request.** A zero accountID or a blank password is a
//     types.StatusInvalidParam *errs.Error in errs.CategoryAPI, and it costs
//     zero HTTP requests: the Gateway never saw a request it could have acted
//     on.
//   - **A rejected credential.** The Gateway answered ok with a body that is not
//     an acknowledgement - a {"success": false}, or a body with neither a
//     success flag nor a data string. The error carries no Gateway status code,
//     so errs.Retryable reports false and errs.ReLoginRequired reports false:
//     asking again unchanged will fail the same way, and the caller's next move
//     is a different password or a different account, not a retry. A session the
//     service held for this account is discarded, because a Gateway that refuses
//     a login has told us the session is not the one it wants.
//   - **A transient failure.** A rate limit, a timeout or a dropped connection
//     arrives already typed by the transport, in errs.CategoryRateLimit,
//     errs.CategoryTimeout or errs.CategoryConnection, and errs.Retryable
//     reports true for the first and third. It says nothing about the credential,
//     the stored session is left alone, and it is still not retried here: the
//     service issues one attempt and returns.
//
// The last two are separated by category, not by message, so a reworded message
// cannot merge them. See TestSessionSeparatesARefusedCredentialFromARetryableRejection
// and TestSessionDiscardsASessionTheGatewayRefused.
//
// # The result
//
// A non-nil *domain.TradeSession, whose Token is the zero SessionToken when the
// Gateway's acknowledgement carried no string - which is the case on the
// {"success": true} shape the released layer and the mock Gateway both send.
// See domain.TradeSession for why that is the honest value and not a placeholder.
// The value is never nil alongside a nil error.
func (s *SessionService) Login(ctx context.Context, accountID domain.AccountID, tradePassword string) (*domain.TradeSession, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opTradeLogin, "accountID must not be empty")
	}
	if strings.TrimSpace(tradePassword) == "" {
		return nil, errs.New(types.StatusInvalidParam, opTradeLogin, "trade password must not be empty")
	}

	if held, ok, err := s.liveSession(ctx, accountID); err != nil {
		return nil, err
	} else if ok {
		return held, nil
	}

	sess, err := s.establish(ctx, accountID, tradePassword)
	if err != nil {
		return nil, err
	}
	// internal/auth's Authenticator.Login discards the error from the read it
	// performs after saving, so a store that fails there yields a zero session
	// and a nil error. A zero AccountID is the tell: every session the store
	// holds was saved under the non-zero key this method validated, so a zero
	// one can only mean the read failed. Handing it back would give the caller a
	// session for the empty account, and each of this layer's other forty-nine
	// methods would then fail with a Gateway rejection that says nothing about the
	// real cause. One check turns that into a clear error.
	//
	// This is a report, not a fix: the discarded error is in internal/auth, which
	// this layer composes rather than modifies.
	if sess.AccountID.IsZero() {
		return nil, errs.New("", opTradeLogin,
			"the session store did not return the session that was just established")
	}
	return tradeSessionFrom(sess), nil
}

// Logout ends the trade session for accountID on /trade/TradeLogout, and clears
// the local session when the Gateway agrees.
//
// It takes no request body and sends struct{}{} - not nil, because a nil params
// is dropped from the envelope entirely and the key would be absent rather than
// an empty object. accountID is the local session key and appears in no request
// body.
//
// # Idempotence, and which kind
//
// With no session held for accountID, Logout returns nil and sends no request,
// which is the released pkg/hstong/session.go's behaviour and the right one: a
// caller that logs out defensively, in a defer, on every path including the ones
// where no login happened, must not put a request on the wire. A refused logout
// is therefore observable only when a session was actually held.
//
// # A failure leaves the session alone
//
// A transport error or a body that is not an acknowledgement is returned and the
// local session is *kept*, so the caller may reconcile - the released layer makes
// the same choice for the same reason. Two consequences follow and both are
// deliberate. A caller cannot use a failed Logout to force a fresh login, because
// the session it still holds is one Login will reuse; a caller that needs a fresh
// authentication constructs a new SessionService. And a Gateway that has already
// forgotten the session, so that Logout fails, leaves the service holding a
// session no request will succeed with - which the next call's rejection, not this
// one, is what reports.
//
// # It is not a mutation and it is not gated
//
// /trade/TradeLogout is not in internal/resilience's mutation set, so a retry
// policy re-sends it on a retryable rejection; re-logging out a session that is
// already gone is the same no-op twice. The service adds no retry of its own.
func (s *SessionService) Logout(ctx context.Context, accountID domain.AccountID) error {
	if accountID.IsZero() {
		return errs.New(types.StatusInvalidParam, opTradeLogout, "accountID must not be empty")
	}

	_, ok, err := s.auth.GetSession(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	var out domain.TradeSessionWire
	if err := s.client.Do(ctx, opTradeLogout, client.RouteTradeLogout,
		struct{}{}, s.client.JSON(), &out); err != nil {
		return err
	}
	if !domain.TradeSessionAcceptedFromDTO(&out) {
		return errs.New("", opTradeLogout, "trade logout was rejected")
	}

	// The wire call succeeded and was acknowledged, so the session is over. This
	// is internal/auth's local clear and nothing more - no second request - which
	// is why the two Logout shapes on this type, this method and
	// auth.Authenticator.Logout, are not interchangeable and are called out
	// separately wherever each appears.
	return s.auth.Logout(ctx, accountID)
}

// wireLogin is the login function internal/auth calls, and the only place this
// service issues /trade/TradeLogin.
//
// The LoginRequest it receives has already been through
// internal/auth.EncryptTradePassword: req.TradePassword is the AES-ECB/PKCS7
// Base64 ciphertext, and encrypting it again would produce a password the Gateway
// cannot decrypt. The token it returns is the reply's data string, which is empty
// on the {"success": true} shape - see domain.TradeSession.
//
// req.EncryptionSalt is deliberately not sent and is not read: see the file
// header. Nothing here logs, and no error this function returns can carry the
// password, because the only password it ever sees is already ciphertext.
func (s *SessionService) wireLogin(ctx context.Context, req auth.LoginRequest) (string, error) {
	var out domain.TradeSessionWire
	if err := s.client.Do(ctx, opTradeLogin, client.RouteTradeLogin,
		sessionLoginWireRequest{Password: req.TradePassword}, s.client.JSON(), &out); err != nil {
		return "", err
	}
	if !domain.TradeSessionAcceptedFromDTO(&out) {
		return "", errs.New("", opTradeLogin, "trade login was rejected")
	}
	return out.Data, nil
}

// liveSession returns the session the service already holds for accountID when
// it is still usable, and reports ok false when there is none, when there is
// one past its refresh point, or when the store could not be read.
//
// The refresh decision is the whole policy in one predicate: ShouldRefresh is
// true from ten minutes before the expiry onwards, so a session that is already
// expired is also already due for refresh, and a caller that asked only IsExpired
// here would keep a session that is still nominally live but about to be refused
// by the Gateway.
//
// An error from the store is returned rather than swallowed. Treating an
// unreadable store as "no session" would turn a local failure into a credential
// on the wire, and the caller would have no way to tell the two apart.
func (s *SessionService) liveSession(ctx context.Context, accountID domain.AccountID) (*domain.TradeSession, bool, error) {
	sess, ok, err := s.auth.GetSession(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	if sess.ShouldRefresh(s.clock.Now()) {
		return nil, false, nil
	}
	return tradeSessionFrom(sess), true, nil
}

// establish performs or joins the one /trade/TradeLogin for accountID.
//
// The leader is decided by the gate, not by a check-then-act on the store, so
// two callers cannot both read "no session" and both proceed. The leader's own
// context drives the request; a joiner waits for the outcome or for its own
// context, whichever comes first.
//
// # Why the leader re-reads the session before requesting
//
// Login reads the store before it reaches the gate, so a caller that read "no
// session" and then lost the race to a leader that has since finished would
// become a second leader and re-send a credential the service already has. The
// window is small and it is real: the gate's entry is removed when a leader
// publishes, and a caller sitting in begin at that moment becomes a leader.
//
// So a leader re-reads before it spends a request, and the re-read is what makes
// "N concurrent calls produce one request" true for every interleaving rather
// than for the one a test happened to produce. Without it the property would
// hold only when no caller was between its store read and the gate, which is
// exactly the kind of assertion that passes on a quiet machine and fails on a
// loaded one.
//
// On a leader failure the session the service held for this account is discarded
// when - and only when - the Gateway said the session is not usable. See
// sessionUnusable for why that distinction decides whether a later call is
// allowed to reuse a dead session.
func (s *SessionService) establish(ctx context.Context, accountID domain.AccountID, tradePassword string) (auth.Session, error) {
	attempt, lead := s.gate.begin(accountID)
	if !lead {
		select {
		case <-attempt.done:
			return attempt.sess, attempt.err
		case <-ctx.Done():
			// The caller's own cancellation, not a Gateway failure, so it is
			// returned as the context's own error and stays matchable with
			// errors.Is. The leader is unaffected and still publishes its
			// outcome to the other joiners.
			return auth.Session{}, ctx.Err()
		}
	}

	sess, err := s.lead(ctx, accountID, tradePassword)
	if err != nil {
		s.discardUnusable(ctx, accountID, err)
	}

	s.gate.publish(accountID, attempt, sess, err)
	return sess, err
}

// lead runs the login for the caller that won the gate, unless the session it
// lost the race to is already good enough.
//
// The re-read's error is deliberately not propagated: the request below it would
// report the same store failure, and a caller that only wants a session is better
// served by the attempt than by a local bookkeeping error. The caller is already
// a leader, so a stored session here is strictly better than nothing.
func (s *SessionService) lead(ctx context.Context, accountID domain.AccountID, tradePassword string) (auth.Session, error) {
	if held, ok, err := s.liveSession(ctx, accountID); err == nil && ok {
		return auth.Session{
			Token:     held.Token,
			AccountID: held.AccountID,
			Expiry:    held.ExpiresAt,
			RefreshAt: held.RefreshAt,
		}, nil
	}
	return s.auth.Login(ctx, auth.LoginRequest{
		AccountID:     accountID,
		TradePassword: tradePassword,
	})
}

// discardUnusable drops the session stored for accountID when cause says the
// Gateway no longer accepts it.
//
// This is the difference between a retryable expiry and a working session, and
// getting it wrong in either direction is bad in a way a test has to pin. If a
// rejected session were kept, the next Login would find it in liveSession, find
// it inside its TTL, and return it without ever sending a request - a service
// that could not log in again until its three-hour TTL ran out, with no error
// and no request to explain it. If a session were dropped on every failure, a
// transient timeout on one login would silently end a session the Gateway is
// perfectly happy with.
//
// The clear is internal/auth's local one: it removes the stored session and
// sends nothing, because the endpoint that would tell the Gateway about it is
// Logout, and calling it here would turn a failed login into a second request.
func (s *SessionService) discardUnusable(ctx context.Context, accountID domain.AccountID, cause error) {
	if !sessionUnusable(cause) {
		return
	}
	// The store's own error is dropped: the login has already failed and this
	// clear is a repair of local state on the failure path, so reporting it
	// would replace the Gateway's reason with a bookkeeping one.
	_ = s.auth.Logout(ctx, accountID)
}

// sessionUnusable reports whether cause says the session the Gateway holds for
// this account is not the one it will accept, so the stored one must go.
//
// The vocabulary is internal/errs', and nothing here matches on a rendered
// message. CategoryAccount is the set the brief's ADR 0006 note names - 1006
// (user not yet authorized), 1012 (not logged in), 1013 (session displaced) and
// 1014 (login timeout), plus 20033 (futures trade login timeout) - and
// errs.ReLoginRequired covers the four of those that mean a re-login rather than
// an authorization state. CategoryAccount is used because it is the superset,
// and because a 1006 is a real answer to "may I keep this session".
//
// Everything else is deliberately excluded, and the two exclusions are the
// point. A rate limit (1011) says the Gateway was busy, and a timeout or a
// dropped connection says the SDK never learned the answer; neither is evidence
// about the session, so neither may destroy one. A refused credential carries no
// status code at all, and a caller acting on that error changes the password
// rather than retrying - there is nothing to discard, because no session was
// established, and the previous one, if any, is exactly what the caller wanted
// to re-establish.
func sessionUnusable(cause error) bool {
	if cause == nil {
		return false
	}
	return errs.CategoryOf(cause) == errs.CategoryAccount
}

// tradeSessionFrom maps the session internal/auth stored into the domain value
// callers receive.
//
// The mapping is here rather than in pkg/domain because internal/auth is an
// internal package and pkg/domain must not import anything under internal/ -
// the layering rule that makes this file the seam rather than a consumer of one.
// The two timestamps come across unchanged, and the token becomes the zero
// SessionToken when internal/auth stored an empty one, which is what happens
// whenever the Gateway's acknowledgement carried no string.
func tradeSessionFrom(sess auth.Session) *domain.TradeSession {
	return &domain.TradeSession{
		AccountID: sess.AccountID,
		Token:     sess.Token,
		ExpiresAt: sess.Expiry,
		RefreshAt: sess.RefreshAt,
	}
}
