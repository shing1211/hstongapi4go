// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package stream provides the public, channel-based real-time push API for the
// HStong (华盛) Quant OpenAPI Gateway.
//
// A stream.Client ties the Gateway's HTTP subscription endpoints to its TCP
// push channel: Subscribe performs the HTTP /hq/Subscribe call and returns a
// Subscription whose Updates and Errors channels deliver decoded push events.
// On a TCP reconnect the client automatically re-issues /hq/Subscribe for every
// active subscription and pushes an ErrReconnected notice on each Errors
// channel, so subscription state is restored rather than silently dropped.
//
// # Usage
//
//	c, err := client.New(client.WithBaseURL("http://127.0.0.1:11111"))
//	if err != nil {
//		return err
//	}
//	defer c.Close()
//
//	s := stream.New(c)
//	if err := s.Connect(ctx); err != nil {
//		return err
//	}
//	defer s.Close()
//
//	sub, err := s.Subscribe(ctx, types.TopicBasicQot,
//		&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"})
//	if err != nil {
//		return err
//	}
//	defer sub.Cancel(ctx)
//
//	for {
//		select {
//		case ev, ok := <-sub.Updates():
//			if !ok {
//				return nil
//			}
//			if q, ok := ev.BasicQot(); ok {
//				_ = q
//			}
//		case err := <-sub.Errors():
//			return err
//		}
//	}
//
// # Lifecycle
//
// Connect establishes the shared TCP push connection and starts the read loop;
// the connection stays open until Close or until the context passed to Connect
// is cancelled. Subscribe and Subscription.Cancel take their own context for
// the HTTP call. Close is idempotent and stops every goroutine the package
// starts. The underlying *client.Client is owned by the caller, not by the
// stream client.
//
// # Trade push
//
// SubscribeTrade subscribes to order-status notifications for the trading and
// futures surfaces (notify types 0, 1, and 2, all carrying
// TradeStockDeliverNotify). Because the order-push subscription is session-wide
// rather than per-security, it does not take a topic or a security list. Attach
// a trade manager with WithTradeManager (for example a *hstong/trade.Manager) to
// have SubscribeTrade issue the HTTP /trade/TradeSubscribe call and Cancel issue
// /trade/TradeUnsubscribe; the narrow TradePusher interface keeps this package
// independent of the trade package. With no manager attached the subscription is
// local only (push-only), which suits a caller that manages the Gateway
// subscription itself.
//
// The legacy direct protocol auto-subscribed to order push when the trading
// connection was initialized; the Gateway exposes explicit subscribe and
// unsubscribe endpoints instead, so the SDK requires an explicit SubscribeTrade
// call.
//
// # Push signature verification
//
// When the underlying *client.Client is configured with
// client.WithEnv or a verify option that sets VerifyPush, and it carries a
// platform public key (client.WithPlatformPublicKey, defaulting to the bundled
// test key), the stream client enables SHA1WithRSA bodySHA1 verification on the
// push channel. A frame whose signature does not verify is dropped and reported
// on the subscription Errors channel. Verification is off by default: the
// Gateway is on loopback and is the trusted transport
// (docs/adr/0005-key-model-and-push-verification.md).
//
// # Backpressure
//
// Updates and Errors are buffered channels (64 by default, configurable with
// WithBuffer). They use a drop-oldest policy: when a buffer is full the oldest
// queued item is discarded to make room for the newest, so a slow consumer
// never blocks the shared push read loop.
package stream
