// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

const (
	DefaultHeartbeatInterval = 30 * time.Second
	DefaultMaxRetries        = 10
	DefaultMinBackoff        = 1 * time.Second
	DefaultMaxBackoff        = 60 * time.Second
)

var ErrManagerClosed = errors.New("push: manager is closed")

type ManagerOption func(*Manager)

func WithHeartbeatInterval(d time.Duration) ManagerOption {
	return func(m *Manager) {
		if d > 0 {
			m.heartbeat = d
		}
	}
}

func WithReconnectMaxRetries(n int) ManagerOption {
	return func(m *Manager) {
		if n >= 0 {
			m.maxRetries = n
		}
	}
}

func WithReconnectBackoff(min, max time.Duration) ManagerOption {
	return func(m *Manager) {
		if min > 0 {
			m.minBackoff = min
		}
		if max > 0 {
			m.maxBackoff = max
		}
	}
}

func WithManagerDialFunc(dial func(ctx context.Context, addr string) (net.Conn, error)) ManagerOption {
	return func(m *Manager) {
		if dial != nil {
			m.dialFunc = dial
		}
	}
}

type Manager struct {
	conn    net.Conn
	subs    map[int][]*dto.Security
	subsMu  sync.RWMutex
	updates chan *domain.PushEvent
	errors  chan error
	closed  atomic.Bool

	heartbeat  time.Duration
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration
	dialFunc   func(ctx context.Context, addr string) (net.Conn, error)

	done   chan struct{}
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewManager(_ interface{}, opts ...ManagerOption) *Manager {
	m := &Manager{
		subs:       make(map[int][]*dto.Security),
		updates:    make(chan *domain.PushEvent, 64),
		errors:     make(chan error, 16),
		heartbeat:  DefaultHeartbeatInterval,
		maxRetries: DefaultMaxRetries,
		minBackoff: DefaultMinBackoff,
		maxBackoff: DefaultMaxBackoff,
		dialFunc:   defaultManagerDialFunc,
		done:       make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

func defaultManagerDialFunc(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{}
	return d.DialContext(ctx, "tcp", addr)
}

func (m *Manager) Updates() <-chan *domain.PushEvent {
	return m.updates
}

func (m *Manager) Errors() <-chan error {
	return m.errors
}

func (m *Manager) Dial(ctx context.Context, addr string) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	conn, err := m.dialFunc(ctx, addr)
	if err != nil {
		return fmt.Errorf("push: dial: %w", err)
	}

	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()

	m.wg.Add(1)
	go m.readLoop()

	if m.heartbeat > 0 {
		m.wg.Add(1)
		go m.heartbeatLoop()
	}

	return nil
}

func (m *Manager) Subscribe(ctx context.Context, topicID int, securities []*dto.Security) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	m.subsMu.Lock()
	m.subs[topicID] = securities
	m.subsMu.Unlock()

	return m.sendTopicRequest(ctx, topicID, securities)
}

func (m *Manager) Unsubscribe(ctx context.Context, topicID int, securities []*dto.Security) error {
	if m.closed.Load() {
		return ErrManagerClosed
	}

	m.subsMu.Lock()
	delete(m.subs, topicID)
	m.subsMu.Unlock()

	return m.sendTopicRequest(ctx, topicID, securities)
}

func (m *Manager) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(m.done)

	if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	conn := m.conn
	m.conn = nil
	m.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	m.wg.Wait()

	close(m.updates)
	close(m.errors)

	return nil
}

func (m *Manager) readLoop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.done:
			return
		default:
		}

		m.mu.Lock()
		conn := m.conn
		m.mu.Unlock()

		if conn == nil {
			if !m.reconnectLoop() {
				return
			}
			continue
		}

		h, body, err := ReadFrame(conn)
		if err != nil {
			m.reportError(fmt.Errorf("push: read frame: %w", err))
			m.mu.Lock()
			m.closeConnLocked()
			m.mu.Unlock()
			if !m.reconnectLoop() {
				return
			}
			continue
		}

		switch h.MsgType {
		case MsgHeartbeat:
			continue
		case MsgPush:
			event, err := m.decodePushEvent(body)
			if err != nil {
				m.reportError(fmt.Errorf("push: decode push event: %w", err))
				continue
			}
			if event != nil {
				select {
				case m.updates <- event:
				default:
				}
			}
		default:
			continue
		}
	}
}

func (m *Manager) reconnectLoop() bool {
	backoff := m.minBackoff
	retries := 0

	for {
		select {
		case <-m.done:
			return false
		default:
		}

		if m.maxRetries > 0 && retries >= m.maxRetries {
			m.reportError(fmt.Errorf("push: max reconnect retries (%d) exceeded", m.maxRetries))
			return false
		}

		if m.ctx == nil || m.ctx.Err() != nil {
			return false
		}

		m.mu.Lock()
		addr := ""
		if m.conn != nil {
			addr = m.conn.RemoteAddr().String()
		}
		m.mu.Unlock()

		conn, err := m.dialFunc(m.ctx, addr)
		if err != nil {
			retries++
			m.reportError(fmt.Errorf("push: reconnect dial: %w", err))
			sleepCtx(m.ctx, backoff)
			backoff = nextBackoff(backoff, m.maxBackoff)
			continue
		}

		m.mu.Lock()
		m.conn = conn
		m.mu.Unlock()

		m.resubscribeAll()

		return true
	}
}

func (m *Manager) resubscribeAll() {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()

	for topicID, securities := range m.subs {
		_ = m.sendTopicRequest(context.Background(), topicID, securities)
	}
}

func (m *Manager) heartbeatLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.mu.Lock()
			conn := m.conn
			m.mu.Unlock()

			if conn == nil {
				continue
			}

			_, err := conn.Write(marshalHeartbeat())
			if err != nil {
				m.reportError(fmt.Errorf("push: heartbeat write: %w", err))
			}
		}
	}
}

func marshalHeartbeat() []byte {
	var buf [151]byte
	buf[0] = 'H'
	buf[1] = 'S'
	binary.LittleEndian.PutUint16(buf[2:4], uint16(MsgHeartbeat))
	return buf[:]
}

func (m *Manager) sendTopicRequest(ctx context.Context, topicID int, securities []*dto.Security) error {
	data := m.buildTopicRequest(topicID, securities)

	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()

	if conn == nil {
		return errors.New("push: not connected")
	}

	_, err := conn.Write(data)
	if err != nil {
		return fmt.Errorf("push: topic request write: %w", err)
	}

	return nil
}

func (m *Manager) buildTopicRequest(topicID int, securities []*dto.Security) []byte {
	var bodyLen int = 4
	for _, s := range securities {
		if s != nil {
			bodyLen += 8
		}
	}

	totalLen := 151 + bodyLen
	buf := make([]byte, totalLen)

	buf[0] = 'H'
	buf[1] = 'S'
	binary.LittleEndian.PutUint16(buf[2:4], uint16(MsgRequest))
	binary.LittleEndian.PutUint32(buf[6:10], uint32(topicID))
	binary.LittleEndian.PutUint32(buf[10:14], uint32(bodyLen))

	offset := 151
	for i, s := range securities {
		if s != nil {
			binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(i))
			offset += 4
		}
	}

	return buf
}

func (m *Manager) closeConnLocked() {
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
}

func (m *Manager) reportError(err error) {
	if err == nil {
		return
	}
	select {
	case m.errors <- err:
	default:
	}
}

func (m *Manager) decodePushEvent(body []byte) (*domain.PushEvent, error) {
	var pb pbmsg.PBNotify
	if err := proto.Unmarshal(body, &pb); err != nil {
		return nil, fmt.Errorf("push: unmarshal PBNotify: %w", err)
	}

	t := types.NotifyMsgType(pb.GetNotifyMsgType())
	payload := pb.GetPayload()

	if payload == nil || len(payload.GetValue()) == 0 {
		return nil, nil
	}

	switch t {
	case types.BasicQotNotifyMsgType:
		notify := &hqnotify.BasicQotNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, err
		}
		event := decodeBasicQotEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.TickerNotifyMsgType:
		notify := &hqnotify.TickerNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, err
		}
		event := decodeTickerEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.OrderBookNotifyMsgType:
		notify := &hqnotify.OrderBookFullNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, err
		}
		event := decodeOrderBookEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.BrokerQueueNotifyMsgType:
		notify := &hqnotify.BrokerNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, err
		}
		event := decodeBrokerEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.TradeStockDeliverMsgType, types.FuturesTradeStockDeliverMsgType:
		notify := &tradenotify.TradeStockDeliverNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, err
		}
		event := decodeTradeEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	default:
		return nil, nil
	}
}

func decodeBasicQotEvent(notify *hqnotify.BasicQotNotify, notifyTime uint64) *domain.QuoteEvent {
	event := &domain.QuoteEvent{
		Symbol: domain.SymbolFromSecurity(notify.GetSecurity()),
	}
	if q := notify.GetBasicQot(); q != nil {
		event.LastPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastPrice(), 'f', -1, 64), "0.001")
		event.OpenPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetOpenPrice(), 'f', -1, 64), "0.001")
		event.HighPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetHighPrice(), 'f', -1, 64), "0.001")
		event.LowPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLowPrice(), 'f', -1, 64), "0.001")
		event.ClosePrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastClosePrice(), 'f', -1, 64), "0.001")
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(q.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(q.GetTurnover(), 'f', -1, 64), "HKD", 3)
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeTickerEvent(notify *hqnotify.TickerNotify, notifyTime uint64) *domain.TickerEvent {
	event := &domain.TickerEvent{
		Symbol: domain.SymbolFromSecurity(notify.GetSecurity()),
	}
	if t := notify.GetTicker(); t != nil {
		event.Price = domain.MustNewPrice(strconv.FormatFloat(t.GetPrice(), 'f', -1, 64), "0.001")
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(t.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(t.GetTurnover(), 'f', -1, 64), "HKD", 3)
		event.Side = entrustBSFromInt32(t.GetSide())
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeOrderBookEvent(notify *hqnotify.OrderBookFullNotify, notifyTime uint64) *domain.OrderBookEvent {
	event := &domain.OrderBookEvent{
		Symbol:    domain.SymbolFromSecurity(notify.GetSecurity()),
		Bids:      make([]domain.OrderBookLevel, 0),
		Asks:      make([]domain.OrderBookLevel, 0),
		Depth:     0,
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, ob := range notify.GetOrderBookList() {
		level := domain.OrderBookLevel{
			Level:    ob.GetLevel(),
			Price:    domain.MustNewPrice(strconv.FormatFloat(ob.GetPrice(), 'f', -1, 64), "0.001"),
			Quantity: domain.MustNewQuantity(strconv.FormatInt(ob.GetVolume(), 10)),
		}
		if notify.GetSide() == 0 {
			event.Bids = append(event.Bids, level)
		} else {
			event.Asks = append(event.Asks, level)
		}
	}
	event.Depth = len(event.Bids) + len(event.Asks)
	return event
}

func decodeBrokerEvent(notify *hqnotify.BrokerNotify, notifyTime uint64) *domain.BrokerEvent {
	event := &domain.BrokerEvent{
		Symbol:    domain.SymbolFromSecurity(notify.GetSecurity()),
		Buyers:    make([]domain.BrokerLevel, 0),
		Sellers:   make([]domain.BrokerLevel, 0),
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, b := range notify.GetBrokerList() {
		bl := domain.BrokerLevel{
			BrokerID: int(b.GetLevel()),
			Quantity: domain.MustNewQuantity("0"),
		}
		if notify.GetSide() == 0 {
			event.Buyers = append(event.Buyers, bl)
		} else {
			event.Sellers = append(event.Sellers, bl)
		}
	}
	return event
}

func decodeTradeEvent(notify *tradenotify.TradeStockDeliverNotify, notifyTime uint64) *domain.TradeEvent {
	event := &domain.TradeEvent{
		Symbol:      domain.NewSymbol(domain.MarketFromCode(notify.GetStockCode()), notify.GetStockCode(), 0),
		Price:       domain.MustNewPrice(notify.GetBusinessPrice(), "0.001"),
		Quantity:    domain.MustNewQuantity(notify.GetBusinessAmount()),
		Turnover:    domain.MustNewMoney(notify.GetBusinessAmount(), "HKD", 3),
		CounterID:   notify.GetMatchNo(),
		Timestamp:   fmt.Sprintf("%d", notifyTime),
		Side:        types.EntrustBS(notify.GetEntrustBs()),
		OrderStatus: types.EntrustStatus(notify.GetEntrustStatus()),
	}
	return event
}

func entrustBSFromInt32(side int32) types.EntrustBS {
	switch side {
	case 1:
		return types.EntrustBuy
	case 2:
		return types.EntrustSell
	case 3:
		return types.EntrustCloseShort
	case 4:
		return types.EntrustOpenShort
	default:
		return types.EntrustBS(strconv.FormatInt(int64(side), 10))
	}
}
