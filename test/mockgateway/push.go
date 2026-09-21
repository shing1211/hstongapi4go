// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package mockgateway

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// EmitQuote builds a BasicQotNotify and emits it as a quote push
// (BasicQotNotifyMsgType, topic 11). security and qot must be non-nil.
func (s *Server) EmitQuote(security *dto.Security, qot *dto.BasicQot) error {
	return s.Emit(types.TopicBasicQot, types.BasicQotNotifyMsgType, &hqnotify.BasicQotNotify{
		Security: security,
		BasicQot: qot,
	})
}

// EmitTicker builds a TickerNotify and emits it as a tick push
// (TickerNotifyMsgType, topic 14).
func (s *Server) EmitTicker(security *dto.Security, ticker *dto.Ticker) error {
	return s.Emit(types.TopicTicker, types.TickerNotifyMsgType, &hqnotify.TickerNotify{
		Security: security,
		Ticker:   ticker,
	})
}

// EmitOrderBook builds an OrderBookFullNotify and emits it as an order-book
// push (OrderBookNotifyMsgType, topic 17). side is 0 for bid, 1 for ask.
func (s *Server) EmitOrderBook(security *dto.Security, side int32, book []*dto.OrderBook) error {
	return s.Emit(types.TopicOrderBook, types.OrderBookNotifyMsgType, &hqnotify.OrderBookFullNotify{
		Security:      security,
		Side:          side,
		OrderBookList: book,
	})
}

// EmitBroker builds a BrokerNotify and emits it as a broker-queue push
// (BrokerQueueNotifyMsgType, topic 16). side is 0 for bid, 1 for ask.
func (s *Server) EmitBroker(security *dto.Security, side int32, brokers []*dto.Broker) error {
	return s.Emit(types.TopicBroker, types.BrokerQueueNotifyMsgType, &hqnotify.BrokerNotify{
		Security:   security,
		Side:       side,
		BrokerList: brokers,
	})
}

// EmitTradeDeliver emits an order-status push (TradeStockDeliverMsgType, 1)
// carrying msg, the payload reused by the stock and futures trade push.
func (s *Server) EmitTradeDeliver(msg *tradenotify.TradeStockDeliverNotify) error {
	return s.Emit(0, types.TradeStockDeliverMsgType, msg)
}

// EmitFuturesTradeDeliver emits a futures order-status push
// (FuturesTradeStockDeliverMsgType, 2) carrying msg.
func (s *Server) EmitFuturesTradeDeliver(msg *tradenotify.TradeStockDeliverNotify) error {
	return s.Emit(0, types.FuturesTradeStockDeliverMsgType, msg)
}

// encodeNotify marshals one PBNotify body. typeURL, when non-empty, replaces the
// Any's type_url after packing so tests can prove enum-based decoding.
func encodeNotify(msgType types.NotifyMsgType, payload proto.Message, typeURL string) ([]byte, error) {
	var packed *anypb.Any
	if payload != nil {
		a, err := anypb.New(payload)
		if err != nil {
			return nil, fmt.Errorf("mockgateway: pack push payload: %w", err)
		}
		if typeURL != "" {
			a.TypeUrl = typeURL
		}
		packed = a
	}
	notify := &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(msgType),
		NotifyId:      notifyID(payload),
		NotifyTime:    uint64(time.Now().UnixMilli()),
		Payload:       packed,
	}
	body, err := proto.Marshal(notify)
	if err != nil {
		return nil, fmt.Errorf("mockgateway: marshal PBNotify: %w", err)
	}
	return body, nil
}

// notifyID derives the PBNotify notifyId from the payload: the security code
// for a market push, the stock code (or entrust number) for a trade delivery.
func notifyID(payload proto.Message) string {
	switch m := payload.(type) {
	case *hqnotify.BasicQotNotify:
		return m.GetSecurity().GetCode()
	case *hqnotify.TickerNotify:
		return m.GetSecurity().GetCode()
	case *hqnotify.OrderBookFullNotify:
		return m.GetSecurity().GetCode()
	case *hqnotify.BrokerNotify:
		return m.GetSecurity().GetCode()
	case *tradenotify.TradeStockDeliverNotify:
		if code := m.GetStockCode(); code != "" {
			return code
		}
		return m.GetEntrustNo()
	default:
		return ""
	}
}
