// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"fmt"

	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// emptyPayloadCode is the SystemEvent code substituted for a frame that carried
// no payload body, in place of Normalize's (nil, nil).
//
// It is a code and not a message because it is the machine-readable half: a
// caller branchs on it with errors-free type assertion and a string compare,
// and because the five substitution sites are identical, naming the condition
// once is what keeps the five arms from drifting into five different codes.
const emptyPayloadCode = "EMPTY_PAYLOAD"

// NormalizeNotification maps an already-decoded Notification onto the domain
// push event, closing the gap between this package's two decoders.
//
// decodeNotification (see client.go) consumes the PBNotify envelope into a
// *Notification, holding the payload as the generated pointer Decode selected.
// Normalize takes a *pbmsg.PBNotify and cannot be given one: the envelope is
// already gone, and Notification has no way back to it without a
// serialise/deserialise round trip on every frame. So this is the bridge, and
// it reuses the same five decoders Normalize does rather than becoming a second
// copy of the wire-to-domain mapping.
//
// # It never returns (nil, nil)
//
// Normalize returns (nil, nil) at each of its five empty-payload arms, which is
// a documented contract for a function that reads an envelope directly: there, a
// frame with no payload body and a frame the caller must skip are the same
// thing. A *Notification has already crossed into a caller-visible API whose
// contract is that a delivered update always carries an event, so this function
// substitutes a domain.SystemEvent{Code: emptyPayloadCode, Level: "warn"} at
// each of the five arms and never hands back a nil event with a nil error. The
// substitution is a decision the transport layer makes once, on the way in,
// rather than a nil check every handler would otherwise have to write.
//
// Normalize itself is unchanged and remains the right function for a caller
// holding a *pbmsg.PBNotify.
//
// # It covers one type Normalize does not
//
// Decode accepts types.TrsStockDeliverMsgType and returns a
// *tradenotify.TradeStockDeliverNotify for it, so a Notification can carry that
// type; Normalize would classify it as an unknown type and hand back an UNKNOWN
// SystemEvent. The trade arm here covers all three delivery types, because the
// push transport registers a handler for each of them.
//
// An error is returned only for a Notification that could not have come off the
// wire: a nil Notification, or a Payload that is not the generated message its
// Type selects. Both are programming or wiring errors rather than observed
// conditions, and reporting them is better than substituting an event that would
// read as a successful delivery of something else.
func NormalizeNotification(n *Notification) (*domain.PushEvent, error) {
	if n == nil {
		return nil, fmt.Errorf("push: NormalizeNotification: nil Notification")
	}

	switch n.Type {
	case types.BasicQotNotifyMsgType:
		if n.Payload == nil {
			return emptyPayloadEvent(), nil
		}
		msg, ok := n.Payload.(*hqnotify.BasicQotNotify)
		if !ok {
			return nil, notifPayloadError(n)
		}
		if msg == nil {
			return emptyPayloadEvent(), nil
		}
		event := decodeBasicQotEvent(msg, n.Time)
		return pushEvent(event), nil

	case types.TickerNotifyMsgType:
		if n.Payload == nil {
			return emptyPayloadEvent(), nil
		}
		msg, ok := n.Payload.(*hqnotify.TickerNotify)
		if !ok {
			return nil, notifPayloadError(n)
		}
		if msg == nil {
			return emptyPayloadEvent(), nil
		}
		event := decodeTickerEvent(msg, n.Time)
		return pushEvent(event), nil

	case types.OrderBookNotifyMsgType:
		if n.Payload == nil {
			return emptyPayloadEvent(), nil
		}
		msg, ok := n.Payload.(*hqnotify.OrderBookFullNotify)
		if !ok {
			return nil, notifPayloadError(n)
		}
		if msg == nil {
			return emptyPayloadEvent(), nil
		}
		event := decodeOrderBookEvent(msg, n.Time)
		return pushEvent(event), nil

	case types.BrokerQueueNotifyMsgType:
		if n.Payload == nil {
			return emptyPayloadEvent(), nil
		}
		msg, ok := n.Payload.(*hqnotify.BrokerNotify)
		if !ok {
			return nil, notifPayloadError(n)
		}
		if msg == nil {
			return emptyPayloadEvent(), nil
		}
		event := decodeBrokerEvent(msg, n.Time)
		return pushEvent(event), nil

	case types.TrsStockDeliverMsgType, types.TradeStockDeliverMsgType, types.FuturesTradeStockDeliverMsgType:
		if n.Payload == nil {
			return emptyPayloadEvent(), nil
		}
		msg, ok := n.Payload.(*tradenotify.TradeStockDeliverNotify)
		if !ok {
			return nil, notifPayloadError(n)
		}
		if msg == nil {
			return emptyPayloadEvent(), nil
		}
		event := decodeTradeEvent(msg, n.Time)
		return pushEvent(event), nil

	default:
		// An unrecognised discriminator. normalizeUnknownEvent classifies it
		// exactly as Normalize's own default arm does, and it never returns nil.
		return normalizeUnknownEvent(nil)
	}
}

// notifPayloadError names both halves of a Notification whose Payload is not the
// generated message its Type selects.
//
// decodeNotification cannot produce one - Decode is driven by the same Type -
// so this is the report for a Notification a caller assembled by hand or a
// Notification whose payload a future Decode widened. Both halves are named
// because either alone leaves the reader guessing which one moved.
func notifPayloadError(n *Notification) error {
	return fmt.Errorf("push: NormalizeNotification: payload %T does not match notifyMsgType %s (%d)",
		n.Payload, n.Type.String(), int32(n.Type))
}

// emptyPayloadEvent is the substitution made at each of the five
// empty-payload arms, and at each of the five empty-payload arms Normalize
// returns (nil, nil) for.
//
// The Message is fixed rather than derived from the type because the five sites
// are deliberately identical: a caller that gets one of these learns that a
// frame arrived with no body, which is the whole of the fact, and the Type on
// the enclosing envelope already says which push it belonged to.
func emptyPayloadEvent() *domain.PushEvent {
	return pushEvent(domain.SystemEvent{
		Code:    emptyPayloadCode,
		Message: "push frame carried no payload body",
		Level:   "warn",
	})
}

// pushEvent boxes a concrete event into the PushEvent interface pointer the
// transport layer hands to a handler. It exists so the five arms above do not
// each open a `e := domain.PushEvent(x); return &e, nil` block, which is the
// shape Normalize writes out five times and the reason a sixth arm would be easy
// to add wrongly.
func pushEvent[T domain.PushEvent](e T) *domain.PushEvent {
	boxed := domain.PushEvent(e)
	return &boxed
}
