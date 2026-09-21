// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import (
	"context"

	"github.com/shing1211/hstongapi4go/client"
)

// Operation labels for the trade-push subscription endpoints. Each matches the
// Gateway path so a log line or error points at the failing endpoint.
const (
	opTradeSubscribe   = "trade/TradeSubscribe"
	opTradeUnsubscribe = "trade/TradeUnsubscribe"
)

// SubscribeOrders subscribes the logged-in trading session to order-status push
// on the Gateway (POST /trade/TradeSubscribe).
//
// The request params are empty: order push is a single, session-wide
// subscription rather than a per-security one, unlike the market push topics.
// The Gateway response is discarded; a nil error means the subscription was
// accepted, and notifications then arrive on the TCP push channel as
// TradeStockDeliverNotify (see pkg/hstong/stream.SubscribeTrade).
//
// A trade session is required. The call is routed through Manager.call, which
// first ensures the attached Session is logged in; with no Session attached the
// request is still sent, and the Gateway rejects it when the connection is not
// authenticated. The call issues exactly one HTTP request and is never retried.
func (m *Manager) SubscribeOrders(ctx context.Context) error {
	return m.call(ctx, opTradeSubscribe, client.RouteTradeSubscribe, struct{}{}, nil)
}

// UnsubscribeOrders cancels the session-wide order-status push subscription
// (POST /trade/TradeUnsubscribe). It takes the same empty params and trade
// session as SubscribeOrders, discards the Gateway response, issues exactly one
// HTTP request, and is never retried.
func (m *Manager) UnsubscribeOrders(ctx context.Context) error {
	return m.call(ctx, opTradeUnsubscribe, client.RouteTradeUnsubscribe, struct{}{}, nil)
}
