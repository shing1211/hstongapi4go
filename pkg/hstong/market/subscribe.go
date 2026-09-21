// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package market

import (
	"context"
	"fmt"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Operation labels for the market subscription endpoints.
const (
	opSubscribe   = "market.Subscribe"
	opUnsubscribe = "market.Unsubscribe"
)

// SubscribeRequest is the body of POST /hq/Subscribe and POST /hq/Unsubscribe.
// TopicID selects the push topic and Security lists the instruments to add to
// or remove from it. At least one security is required.
type SubscribeRequest struct {
	// TopicID is the market push topic (see types.TopicID).
	TopicID types.TopicID `json:"topicId"`
	// Security is the non-empty list of instruments.
	Security []*dto.Security `json:"security"`
}

// knownTopics is the set of market push topic identifiers accepted by
// Subscribe and Unsubscribe (docs/SPEC.md §3). The set is non-exhaustive in the
// vendor documentation but exhaustive here so a typo fails fast.
var knownTopics = map[types.TopicID]struct{}{
	types.TopicBasicQot:           {},
	types.TopicQuoteVariant35:     {},
	types.TopicTicker:             {},
	types.TopicTickVariant27:      {},
	types.TopicTickVariant28:      {},
	types.TopicTickVariant37:      {},
	types.TopicBroker:             {},
	types.TopicOrderBook:          {},
	types.TopicOrderBookArcabook:  {},
	types.TopicOrderBookTotalView: {},
	types.TopicOrderBookVariant36: {},
}

// Subscribe adds securities to a market push topic on the Gateway. It rejects
// an unknown TopicID or an empty (or nil-containing) security list with a typed
// StatusInvalidParam error and sends no request. The Gateway response is
// discarded; a nil error means the subscription was accepted. Notification
// delivery happens on the TCP push channel (see pkg/hstong/stream).
func (m *Manager) Subscribe(ctx context.Context, topicID types.TopicID, securities ...*dto.Security) error {
	if err := validateTopic(opSubscribe, topicID); err != nil {
		return err
	}
	if err := validateSecurityList(opSubscribe, securities); err != nil {
		return err
	}
	req := SubscribeRequest{TopicID: topicID, Security: securities}
	return m.client.Do(ctx, opSubscribe, client.RouteHqSubscribe, req, m.client.JSON(), nil)
}

// Unsubscribe removes securities from a market push topic on the Gateway. It
// validates its arguments exactly like Subscribe and sends no request when they
// are invalid. The Gateway response is discarded.
func (m *Manager) Unsubscribe(ctx context.Context, topicID types.TopicID, securities ...*dto.Security) error {
	if err := validateTopic(opUnsubscribe, topicID); err != nil {
		return err
	}
	if err := validateSecurityList(opUnsubscribe, securities); err != nil {
		return err
	}
	req := SubscribeRequest{TopicID: topicID, Security: securities}
	return m.client.Do(ctx, opUnsubscribe, client.RouteHqUnsubscribe, req, m.client.JSON(), nil)
}

// validateTopic rejects a topic that is not one of the documented market push
// topics as an invalid parameter.
func validateTopic(op string, topic types.TopicID) error {
	if _, ok := knownTopics[topic]; !ok {
		return errs.New(types.StatusInvalidParam, op, fmt.Sprintf("unknown topicId %d", int(topic)))
	}
	return nil
}
