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
// authenticated.
//
// On retries: the call issues exactly one HTTP request per attempt, and it is
// never retried when no retry policy is installed - which is the default
// configuration, where every route is sent once. An earlier version of this
// comment said "is never retried" as an absolute, and that was wrong: neither
// route is in internal/resilience.mutationPaths, so ClassForPath returns query
// class for both and an installed retry policy re-sends them. The code is
// unchanged and is the thing that was measured: see
// TestManager_MutationRoutesIssueExactlyOneAttemptUnderARetryPolicy in this
// package, whose "trade push subscribe routes retry under the same policy"
// subtest drives both routes through one MaxAttempts-5 policy and a retryable
// rejection, recording 5 requests each against 1 for an order mutation.
func (m *Manager) SubscribeOrders(ctx context.Context) error {
	return m.call(ctx, opTradeSubscribe, client.RouteTradeSubscribe, struct{}{}, nil)
}

// UnsubscribeOrders cancels the session-wide order-status push subscription
// (POST /trade/TradeUnsubscribe). It takes the same empty params and trade
// session as SubscribeOrders and discards the Gateway response. Its retry
// behaviour is SubscribeOrders' in every respect, stated there and not repeated
// here so the two cannot drift.
func (m *Manager) UnsubscribeOrders(ctx context.Context) error {
	return m.call(ctx, opTradeUnsubscribe, client.RouteTradeUnsubscribe, struct{}{}, nil)
}
