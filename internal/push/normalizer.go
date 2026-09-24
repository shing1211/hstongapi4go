// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func Normalize(pb *pbmsg.PBNotify) (*domain.PushEvent, error) {
	if pb == nil {
		return nil, fmt.Errorf("push: Normalize: nil PBNotify")
	}

	t := types.NotifyMsgType(pb.GetNotifyMsgType())
	payload := pb.GetPayload()

	if payload == nil {
		return normalizeUnknownEvent(nil)
	}

	switch t {
	case types.BasicQotNotifyMsgType:
		if len(payload.GetValue()) == 0 {
			return nil, nil
		}
		notify := &hqnotify.BasicQotNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, fmt.Errorf("push: Normalize: unmarshal BasicQotNotify: %w", err)
		}
		event := decodeBasicQotEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.TickerNotifyMsgType:
		if len(payload.GetValue()) == 0 {
			return nil, nil
		}
		notify := &hqnotify.TickerNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, fmt.Errorf("push: Normalize: unmarshal TickerNotify: %w", err)
		}
		event := decodeTickerEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.OrderBookNotifyMsgType:
		if len(payload.GetValue()) == 0 {
			return nil, nil
		}
		notify := &hqnotify.OrderBookFullNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, fmt.Errorf("push: Normalize: unmarshal OrderBookFullNotify: %w", err)
		}
		event := decodeOrderBookEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.BrokerQueueNotifyMsgType:
		if len(payload.GetValue()) == 0 {
			return nil, nil
		}
		notify := &hqnotify.BrokerNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, fmt.Errorf("push: Normalize: unmarshal BrokerNotify: %w", err)
		}
		event := decodeBrokerEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	case types.TradeStockDeliverMsgType, types.FuturesTradeStockDeliverMsgType:
		if len(payload.GetValue()) == 0 {
			return nil, nil
		}
		notify := &tradenotify.TradeStockDeliverNotify{}
		if err := proto.Unmarshal(payload.GetValue(), notify); err != nil {
			return nil, fmt.Errorf("push: Normalize: unmarshal TradeStockDeliverNotify: %w", err)
		}
		event := decodeTradeEvent(notify, pb.GetNotifyTime())
		e := domain.PushEvent(event)
		return &e, nil

	default:
		return normalizeUnknownEvent(payload)
	}
}

func NormalizeUnknown(a *anypb.Any) (*domain.PushEvent, error) {
	return normalizeUnknownEvent(a)
}

func DecodeAny(a *anypb.Any) (proto.Message, error) {
	if a == nil {
		return nil, fmt.Errorf("push: DecodeAny: nil Any")
	}

	msg, err := a.UnmarshalNew()
	if err != nil {
		return nil, fmt.Errorf("push: DecodeAny: UnmarshalNew: %w", err)
	}

	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("push: DecodeAny: result does not implement proto.Message")
	}

	return protoMsg, nil
}

func normalizeUnknownEvent(a *anypb.Any) (*domain.PushEvent, error) {
	if a == nil {
		ev := domain.SystemEvent{
			Code:    "UNKNOWN",
			Message: "nil payload",
			Level:   "warn",
		}
		e := domain.PushEvent(ev)
		return &e, nil
	}

	typeURL := a.GetTypeUrl()
	valueLen := len(a.GetValue())

	ev := domain.SystemEvent{
		Code:    typeURL,
		Message: fmt.Sprintf("unknown push event type: %d bytes", valueLen),
		Level:   "warn",
	}
	e := domain.PushEvent(ev)
	return &e, nil
}
