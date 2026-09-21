// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package mockgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Default addresses the mock binds when no WithHTTPAddr/WithPushAddr is given.
// The HTTP address is empty, which selects an ephemeral 127.0.0.1 port; the
// push address is the Gateway's documented local push port.
const (
	// DefaultPushAddr is the local Gateway TCP push port.
	DefaultPushAddr = "127.0.0.1:11112"
	// maxRequestBody bounds the HTTP request body the mock will read.
	maxRequestBody = 8 << 20
)

// ErrorInjection makes the mock answer a route with an ok:false envelope whose
// err text is "<Code> <Message>", so the SDK's error mapping, retryability, and
// re-login paths can be exercised.
type ErrorInjection struct {
	// Code is the documented Gateway status code placed first in err.
	Code string
	// Message is the human-readable text after Code. It may be empty.
	Message string
	// Times is how many matching requests fail. Zero means every matching
	// request fails until the injection is removed or replaced.
	Times int

	// remaining counts down for a finite injection.
	remaining int
}

// requestEnvelope is the Gateway HTTP request body.
type requestEnvelope struct {
	TimeoutSec int             `json:"timeout_sec"`
	Params     json.RawMessage `json:"params"`
}

// responseEnvelope is the Gateway HTTP response body.
type responseEnvelope struct {
	OK   bool            `json:"ok"`
	Err  string          `json:"err"`
	Data json.RawMessage `json:"data"`
}

// config holds the resolved mock settings.
type config struct {
	httpAddr   string
	pushAddr   string
	fixtures   *FixtureSet
	injections map[string]ErrorInjection
	anyTypeURL string
}

// Option configures a Server. Options are applied in order by New.
type Option func(*config)

// WithHTTPAddr sets the HTTP listen address. The default is empty, which binds
// an ephemeral 127.0.0.1 port chosen by httptest. A fixed address such as
// "127.0.0.1:11111" is useful for the standalone binary.
func WithHTTPAddr(addr string) Option {
	return func(c *config) { c.httpAddr = addr }
}

// WithPushAddr sets the push listen address. The default is DefaultPushAddr. An
// empty value restores the default; "127.0.0.1:0" binds an ephemeral port.
func WithPushAddr(addr string) Option {
	return func(c *config) {
		if addr == "" {
			c.pushAddr = DefaultPushAddr
			return
		}
		c.pushAddr = addr
	}
}

// WithFixtureSet replaces the response fixture table. A nil value keeps
// DefaultFixtures.
func WithFixtureSet(f *FixtureSet) Option {
	return func(c *config) {
		if f != nil {
			c.fixtures = f
		}
	}
}

// WithErrorInjection seeds the per-path error injections. The map is copied.
func WithErrorInjection(m map[string]ErrorInjection) Option {
	return func(c *config) {
		for path, inj := range m {
			inj.remaining = inj.Times
			c.injections[path] = inj
		}
	}
}

// WithPushTypeURL overrides the type_url written into every emitted push Any
// with url. Non-empty values let a test prove the SDK decodes push payloads by
// notifyMsgType enum rather than by type_url.
func WithPushTypeURL(url string) Option {
	return func(c *config) { c.anyTypeURL = url }
}

// Server is an HTTP + TCP-push mock of the HStong OpenAPI Gateway. Build it
// with New, start it with Start or StartT, drive the SDK against HTTPBaseURL and
// PushAddr, and release it with Close.
//
// A Server is safe for concurrent use. Close is idempotent and leak-free.
type Server struct {
	cfg      config
	http     *httptest.Server
	push     *pushServer
	fixtures *FixtureSet

	injMu sync.Mutex
	inj   map[string]ErrorInjection

	subMu    sync.Mutex
	topics   map[types.TopicID]int
	tradeSub bool

	started   atomic.Bool
	closed    atomic.Bool
	closeOnce sync.Once
}

// New returns a Server configured by opts. It does not listen until Start.
func New(opts ...Option) *Server {
	cfg := config{
		pushAddr:   DefaultPushAddr,
		injections: map[string]ErrorInjection{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.fixtures == nil {
		cfg.fixtures = DefaultFixtures()
	}
	if cfg.pushAddr == "" {
		cfg.pushAddr = DefaultPushAddr
	}
	return &Server{
		cfg:      cfg,
		fixtures: cfg.fixtures,
		inj:      cfg.injections,
		topics:   map[types.TopicID]int{},
	}
}

// Start binds the HTTP and push listeners and begins serving. It is idempotent
// while started and returns an error when a listener cannot be bound or the
// server has been closed.
func (s *Server) Start() error {
	if s.closed.Load() {
		return fmt.Errorf("mockgateway: server is closed")
	}
	if s.started.Load() {
		return nil
	}

	handler := http.HandlerFunc(s.handleHTTP)
	if s.cfg.httpAddr != "" {
		ln, err := net.Listen("tcp", s.cfg.httpAddr)
		if err != nil {
			return fmt.Errorf("mockgateway: listen http %s: %w", s.cfg.httpAddr, err)
		}
		srv := httptest.NewUnstartedServer(handler)
		srv.Listener = ln
		srv.Start()
		s.http = srv
	} else {
		s.http = httptest.NewServer(handler)
	}

	s.push = &pushServer{
		typeURL: s.cfg.anyTypeURL,
		conns:   map[*pushConn]struct{}{},
	}
	if err := s.push.start(s.cfg.pushAddr); err != nil {
		s.http.Close()
		s.http = nil
		return err
	}
	s.started.Store(true)
	return nil
}

// StartT starts the server and registers Close as a test cleanup, failing t on
// a bind error. It is the convenience entry point for tests.
func (s *Server) StartT(t testing.TB) {
	t.Helper()
	if err := s.Start(); err != nil {
		t.Fatalf("mockgateway: Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
}

// Close stops the HTTP and push servers, closes every connection, and waits for
// all internal goroutines. It is idempotent and never returns a non-nil error.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		if s.push != nil {
			_ = s.push.close()
		}
		if s.http != nil {
			s.http.Close()
		}
	})
	return nil
}

// HTTPBaseURL returns the HTTP base URL, for example "http://127.0.0.1:54321",
// or the empty string before Start.
func (s *Server) HTTPBaseURL() string {
	if s.http == nil {
		return ""
	}
	return s.http.URL
}

// PushAddr returns the bound push "host:port", or the empty string before
// Start.
func (s *Server) PushAddr() string {
	if s.push == nil || s.push.ln == nil {
		return ""
	}
	return s.push.ln.Addr().String()
}

// ClientCount returns the number of currently connected push clients.
func (s *Server) ClientCount() int {
	if s.push == nil {
		return 0
	}
	return s.push.clientCount()
}

// SubscribedTopics returns the market topics registered through /hq/Subscribe,
// with the current security count per topic. The returned map is a copy.
func (s *Server) SubscribedTopics() map[types.TopicID]int {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	out := make(map[types.TopicID]int, len(s.topics))
	for topic, n := range s.topics {
		out[topic] = n
	}
	return out
}

// TradeSubscribed reports whether /trade/TradeSubscribe has registered the
// session-wide order-push subscription.
func (s *Server) TradeSubscribed() bool {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	return s.tradeSub
}

// Inject installs or replaces the error injection for path. Times counts the
// requests to fail; zero means every request. An injection with Code empty is
// removed.
func (s *Server) Inject(path string, inj ErrorInjection) {
	s.injMu.Lock()
	defer s.injMu.Unlock()
	if inj.Code == "" {
		delete(s.inj, path)
		return
	}
	inj.remaining = inj.Times
	s.inj[path] = inj
}

// ClearInjections removes every error injection.
func (s *Server) ClearInjections() {
	s.injMu.Lock()
	defer s.injMu.Unlock()
	s.inj = map[string]ErrorInjection{}
}

// Emit writes one push frame carrying payload as a PBNotify to every connected
// push client. msgType selects the notify discriminator; topicID labels the
// emission for the helper methods and is not placed on the wire. It returns the
// first write error, if any; a connection on which the write fails is closed.
func (s *Server) Emit(topicID types.TopicID, msgType types.NotifyMsgType, payload proto.Message) error {
	if s.push == nil {
		return fmt.Errorf("mockgateway: server is not started")
	}
	return s.push.emit(topicID, msgType, payload)
}

// takeInjection returns the injection for path and decrements a finite one.
func (s *Server) takeInjection(path string) (ErrorInjection, bool) {
	s.injMu.Lock()
	defer s.injMu.Unlock()
	inj, ok := s.inj[path]
	if !ok {
		return ErrorInjection{}, false
	}
	if inj.Times == 0 {
		return inj, true
	}
	inj.remaining--
	if inj.remaining <= 0 {
		delete(s.inj, path)
	}
	return inj, true
}

// handleHTTP serves one Gateway request.
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "1010", "illegal request: POST only")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "1010", "illegal request: unreadable body")
		return
	}
	env := requestEnvelope{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &env); err != nil {
			writeError(w, http.StatusBadRequest, "1010", "illegal request: bad envelope")
			return
		}
	}

	path := client.NormalizePath(r.URL.Path)

	if inj, ok := s.takeInjection(path); ok {
		writeError(w, http.StatusOK, inj.Code, inj.Message)
		return
	}

	s.recordSubscription(path, env.Params)

	data, ok := s.fixtures.lookup(path, env.Params)
	if !ok {
		writeError(w, http.StatusNotFound, "1009", "endpoint not found")
		return
	}
	writeOK(w, data)
}

// recordSubscription tracks the market and trade push subscriptions registered
// through the HTTP subscribe/unsubscribe routes.
func (s *Server) recordSubscription(path string, params json.RawMessage) {
	switch client.Route(path) {
	case client.RouteHqSubscribe, client.RouteHqUnsubscribe:
		var p struct {
			TopicID  int `json:"topicId"`
			Security []struct {
				Code string `json:"code"`
			} `json:"security"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}
		topic := types.TopicID(p.TopicID)
		s.subMu.Lock()
		defer s.subMu.Unlock()
		if client.Route(path) == client.RouteHqSubscribe {
			s.topics[topic] += len(p.Security)
			return
		}
		if n := s.topics[topic] - len(p.Security); n > 0 {
			s.topics[topic] = n
		} else {
			delete(s.topics, topic)
		}
	case client.RouteTradeSubscribe:
		s.subMu.Lock()
		s.tradeSub = true
		s.subMu.Unlock()
	case client.RouteTradeUnsubscribe:
		s.subMu.Lock()
		s.tradeSub = false
		s.subMu.Unlock()
	}
}

// writeOK writes a successful envelope with data.
func writeOK(w http.ResponseWriter, data json.RawMessage) {
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(responseEnvelope{OK: true, Data: data})
}

// writeError writes an ok:false envelope with an err text beginning with code.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(responseEnvelope{OK: false, Err: strings.TrimSpace(code + " " + message)})
}

// pushServer is the TCP push listener and connection registry.
type pushServer struct {
	ln      net.Listener
	typeURL string
	seq     atomic.Int32

	mu     sync.Mutex
	conns  map[*pushConn]struct{}
	closed bool

	wg sync.WaitGroup
}

// pushConn serialises writes to one push connection.
type pushConn struct {
	net.Conn
	mu sync.Mutex
}

// start binds the listener and starts the accept loop.
func (p *pushServer) start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mockgateway: listen push %s: %w", addr, err)
	}
	p.ln = ln
	p.wg.Add(1)
	go p.acceptLoop()
	return nil
}

// acceptLoop accepts connections until the listener closes.
func (p *pushServer) acceptLoop() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		pc := &pushConn{Conn: c}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			_ = c.Close()
			return
		}
		p.conns[pc] = struct{}{}
		p.mu.Unlock()

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer p.remove(pc)
			p.readLoop(pc)
		}()
	}
}

// readLoop drains frames from the client until the connection closes. The mock
// never acts on client frames; it reads only so a peer close is detected.
func (p *pushServer) readLoop(pc *pushConn) {
	for {
		if _, _, err := push.ReadFrame(pc.Conn); err != nil {
			return
		}
	}
}

// remove drops pc from the registry without closing it.
func (p *pushServer) remove(pc *pushConn) {
	p.mu.Lock()
	delete(p.conns, pc)
	p.mu.Unlock()
}

// clientCount returns the number of registered connections.
func (p *pushServer) clientCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

// emit writes one push frame to every connected client.
func (p *pushServer) emit(topicID types.TopicID, msgType types.NotifyMsgType, payload proto.Message) error {
	body, err := encodeNotify(msgType, payload, p.typeURL)
	if err != nil {
		return err
	}
	header := push.Header{MsgType: push.MsgPush, ProtoFmtType: 0, ProtoVer: 0, SerialNo: p.seq.Add(1)}

	p.mu.Lock()
	conns := make([]*pushConn, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
	}
	p.mu.Unlock()

	var firstErr error
	for _, c := range conns {
		if err := c.writeFrame(header, body); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			p.remove(c)
			_ = c.Close()
		}
	}
	_ = topicID
	return firstErr
}

// writeFrame serialises one frame write on pc.
func (pc *pushConn) writeFrame(h push.Header, body []byte) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return push.WriteFrame(pc.Conn, h, body)
}

// close stops the listener, closes every connection, and waits for goroutines.
func (p *pushServer) close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	if p.ln != nil {
		_ = p.ln.Close()
	}
	conns := make([]*pushConn, 0, len(p.conns))
	for c := range p.conns {
		conns = append(conns, c)
	}
	p.conns = map[*pushConn]struct{}{}
	p.mu.Unlock()

	for _, c := range conns {
		_ = c.Close()
	}
	p.wg.Wait()
	return nil
}
