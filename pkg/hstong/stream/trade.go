// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"

	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// tradeNotifyTypes are the push message types that carry an order-status change
// for the trading and futures surfaces. All three reuse the same
// TradeStockDeliverNotify payload (docs/SPEC.md §4).
var tradeNotifyTypes = []types.NotifyMsgType{
	types.TrsStockDeliverMsgType,
	types.TradeStockDeliverMsgType,
	types.FuturesTradeStockDeliverMsgType,
}

// TradePusher is the subset of the trade manager that the stream client uses to
// manage the Gateway's session-wide order-push subscription. *trade.Manager
// satisfies it. Depending on this narrow interface rather than the trade
// package keeps the stream package free of a compile-time dependency on it.
type TradePusher interface {
	// SubscribeOrders enables order-status push for the logged-in trade
	// session (POST /trade/TradeSubscribe).
	SubscribeOrders(ctx context.Context) error
	// UnsubscribeOrders disables order-status push
	// (POST /trade/TradeUnsubscribe).
	UnsubscribeOrders(ctx context.Context) error
}

// WithTradeManager attaches a trade-push manager to the stream client. When set,
// SubscribeTrade performs the HTTP /trade/TradeSubscribe call and Cancel
// performs /trade/TradeUnsubscribe. When unset, SubscribeTrade registers the
// local push subscription only and Cancel sends no HTTP call, which is useful
// for a caller that manages the Gateway subscription itself.
func WithTradeManager(t TradePusher) Option {
	return func(o *options) { o.trade = t }
}

// SubscribeTrade registers interest in the Gateway's order-status push
// notifications and returns a Subscription whose Updates channel delivers them
// as Events. It covers the stock delivery types TrsStockDeliverMsgType (0) and
// TradeStockDeliverMsgType (1), and the futures type
// FuturesTradeStockDeliverMsgType (2); decode with Event.TradeDeliver or
// Event.FuturesTradeDeliver.
//
// When a trade manager was attached with WithTradeManager, SubscribeTrade first
// performs the HTTP POST /trade/TradeSubscribe call over the attached trade
// session. When none is attached it registers the local push subscription only
// and sends no HTTP request, leaving the Gateway subscription to the caller.
//
// Order push is session-wide rather than per-security, unlike the market
// topics, so the HTTP subscribe is issued once for the first active trade
// subscription and the matching unsubscribe once for the last one cancelled.
// The returned Subscription is owned by the caller and must be released with
// Cancel. SubscribeTrade returns ErrNoClient when the stream client has no
// underlying *client.Client and ErrClosed after Close.
//
// Note: the legacy direct protocol auto-subscribed to order push when the
// trading connection was initialized. The Gateway exposes explicit
// subscribe/unsubscribe endpoints instead, so the SDK requires an explicit
// SubscribeTrade call.
func (s *Client) SubscribeTrade(ctx context.Context) (*Subscription, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	first := s.tradeSubs == 0
	trade := s.trade
	s.mu.Unlock()

	if first && trade != nil {
		if err := trade.SubscribeOrders(ctx); err != nil {
			return nil, err
		}
	}

	sub := &Subscription{
		client:  s,
		trade:   true,
		accepts: append([]types.NotifyMsgType(nil), tradeNotifyTypes...),
		updates: make(chan Event, s.opts.buffer),
		errs:    make(chan error, s.opts.buffer),
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	s.subs[sub] = struct{}{}
	s.tradeSubs++
	s.mu.Unlock()
	return sub, nil
}

// FuturesTradeDeliver returns the TradeStockDeliverNotify payload of a futures
// trade-delivery event (notify type FuturesTradeStockDeliverMsgType, 2), and
// reports whether the event carried one. The futures payload is a documented
// subset of the stock one: absent fields stay at their zero value, and matchNo
// is populated only when the notification reports a fill.
func (e Event) FuturesTradeDeliver() (*tradenotify.TradeStockDeliverNotify, bool) {
	if e.Type != types.FuturesTradeStockDeliverMsgType {
		return nil, false
	}
	m, ok := e.Payload.(*tradenotify.TradeStockDeliverNotify)
	return m, ok
}
