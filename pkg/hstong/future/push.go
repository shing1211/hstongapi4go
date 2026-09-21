// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package future

import (
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// NotifyType is the PBNotify message discriminator that carries a futures trade
// delivery (成交). It is types.FuturesTradeStockDeliverMsgType (value 2).
// P04/P06 select this decoder when a push frame's notifyMsgType equals it.
const NotifyType = types.FuturesTradeStockDeliverMsgType

// DeliverNotification is the typed, documented view of a futures trade-delivery
// push payload. It mirrors the TradeStockDeliverNotify protobuf message that
// the Gateway reuses for the FuturesTradeStockDeliverMsgType notify.
//
// Every field is a string on the wire. Monetary fields (BusinessPrice,
// SumBusinessBalance) and quantities (BusinessAmount, OriginalAmount,
// LeftAmount, EntrustAmount, SumBusinessAmount) are therefore preserved exactly,
// with no float conversion.
//
// Two fields carry special semantics, documented on the original message:
//
//   - MatchNo is the fill serial number (成交流水号) and is populated only when
//     the notification reports a fill; it is empty for a pure order-state
//     change such as a placement, cancel, or modify acknowledgement. Use
//     HasFill to test this without comparing strings.
//   - RecordNo is the order record number (订单号) and equals EntrustNo.
//
// A DeliverNotification is a value type and is safe to copy.
type DeliverNotification struct {
	// FundAccount is the funding account (资金账号).
	FundAccount string
	// StockCode is the futures contract code (股票代码).
	StockCode string
	// StockName is the contract name (股票名称).
	StockName string
	// EntrustBS is the order direction (委托方向).
	EntrustBS string
	// BusinessPrice is the fill price (成交价格).
	BusinessPrice string
	// BusinessAmount is the filled quantity (成交数量).
	BusinessAmount string
	// BusinessDate is the fill date (成交日期).
	BusinessDate string
	// BusinessTime is the fill time (成交时间).
	BusinessTime string
	// ExchangeType is the trading market (交易类型).
	ExchangeType string
	// EntrustStatus is the order status (委托状态).
	EntrustStatus string
	// ClientID is the client number (客户号).
	ClientID string
	// EntrustNo is the order number (委托编号).
	EntrustNo string
	// SumBusinessAmount is the total filled quantity (总数量).
	SumBusinessAmount string
	// SumBusinessBalance is the total filled amount (总金额).
	SumBusinessBalance string
	// OriginalAmount is the original order quantity (原始委托数量).
	OriginalAmount string
	// OriginalPrice is the original order price (原始委托价格).
	OriginalPrice string
	// LeftAmount is the remaining open quantity (剩余挂单数量).
	LeftAmount string
	// EntrustPrice is the price after a modify (改后委托价格).
	EntrustPrice string
	// EntrustAmount is the quantity after a modify (改后委托数量).
	EntrustAmount string
	// RecordNo is the order record number (订单号) and equals EntrustNo.
	RecordNo string
	// Remark is a free-text note (备注).
	Remark string
	// MatchNo is the fill serial number (成交流水号); it is populated only when
	// the notification reports a fill.
	MatchNo string
}

// HasFill reports whether the notification carries a fill, that is whether the
// fill-only MatchNo field is non-empty.
func (n DeliverNotification) HasFill() bool {
	return n.MatchNo != ""
}

// FromDeliverNotify maps a generated TradeStockDeliverNotify payload into its
// typed DeliverNotification view. A nil message yields the zero value, so the
// mapping is safe to call on an exhausted push. MatchNo remains empty unless the
// Gateway reported a fill, and RecordNo mirrors EntrustNo.
func FromDeliverNotify(msg *tradenotify.TradeStockDeliverNotify) DeliverNotification {
	if msg == nil {
		return DeliverNotification{}
	}
	return DeliverNotification{
		FundAccount:        msg.GetFundAccount(),
		StockCode:          msg.GetStockCode(),
		StockName:          msg.GetStockName(),
		EntrustBS:          msg.GetEntrustBs(),
		BusinessPrice:      msg.GetBusinessPrice(),
		BusinessAmount:     msg.GetBusinessAmount(),
		BusinessDate:       msg.GetBusinessDate(),
		BusinessTime:       msg.GetBusinessTime(),
		ExchangeType:       msg.GetExchangeType(),
		EntrustStatus:      msg.GetEntrustStatus(),
		ClientID:           msg.GetClientId(),
		EntrustNo:          msg.GetEntrustNo(),
		SumBusinessAmount:  msg.GetSumBusinessAmount(),
		SumBusinessBalance: msg.GetSumBusinessBalance(),
		OriginalAmount:     msg.GetOriginalAmount(),
		OriginalPrice:      msg.GetOriginalPrice(),
		LeftAmount:         msg.GetLeftAmount(),
		EntrustPrice:       msg.GetEntrustPrice(),
		EntrustAmount:      msg.GetEntrustAmount(),
		RecordNo:           msg.GetRecordNo(),
		Remark:             msg.GetRemark(),
		MatchNo:            msg.GetMatchNo(),
	}
}
