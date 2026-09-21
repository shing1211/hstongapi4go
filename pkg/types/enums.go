// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package types

import "strconv"

// NotifyMsgType is the push-message discriminator carried in the PBNotify
// envelope. It matches the protobuf enum in
// common/constant/NotifyMsgType.proto (v2.2.0).
type NotifyMsgType int32

const (
	// TrsStockDeliverMsgType reports a 柜台 (counter) stock delivery.
	TrsStockDeliverMsgType NotifyMsgType = 0
	// TradeStockDeliverMsgType reports a stock trade delivery (成交).
	TradeStockDeliverMsgType NotifyMsgType = 1
	// FuturesTradeStockDeliverMsgType reports a futures trade delivery.
	FuturesTradeStockDeliverMsgType NotifyMsgType = 2
	// OrderBookNotifyMsgType is the order-book (买卖档) push.
	OrderBookNotifyMsgType NotifyMsgType = 20001
	// BrokerQueueNotifyMsgType is the broker-queue (买卖经纪) push.
	BrokerQueueNotifyMsgType NotifyMsgType = 20002
	// BasicQotNotifyMsgType is the basic-quote (基础报价) push.
	BasicQotNotifyMsgType NotifyMsgType = 20003
	// TickerNotifyMsgType is the tick-by-tick (逐笔) push.
	TickerNotifyMsgType NotifyMsgType = 20004
)

// String returns the protobuf enum name of m, or "NotifyMsgType(<n>)" when the
// value is not one of the defined constants.
func (m NotifyMsgType) String() string {
	switch m {
	case TrsStockDeliverMsgType:
		return "TrsStockDeliverMsgType"
	case TradeStockDeliverMsgType:
		return "TradeStockDeliverMsgType"
	case FuturesTradeStockDeliverMsgType:
		return "FuturesTradeStockDeliverMsgType"
	case OrderBookNotifyMsgType:
		return "OrderBookNotifyMsgType"
	case BrokerQueueNotifyMsgType:
		return "BrokerQueueNotifyMsgType"
	case BasicQotNotifyMsgType:
		return "BasicQotNotifyMsgType"
	case TickerNotifyMsgType:
		return "TickerNotifyMsgType"
	default:
		return "NotifyMsgType(" + strconv.FormatInt(int64(m), 10) + ")"
	}
}

// TopicID identifies a market-data push topic passed to Subscribe and
// Unsubscribe. The reference documentation labels topics 11, 14, 16, 17, 25,
// and 26 explicitly; the remaining values are the current Gateway's variant
// topics grouped by kind in docs/SPEC.md §3. The set is non-exhaustive.
type TopicID int

const (
	// TopicBasicQot (11) is the basic-quote push.
	TopicBasicQot TopicID = 11
	// TopicQuoteVariant35 (35) is a quote-group push variant; the reference
	// does not enumerate its exact semantics.
	TopicQuoteVariant35 TopicID = 35
	// TopicTicker (14) is the tick-by-tick (逐笔) push.
	TopicTicker TopicID = 14
	// TopicTickVariant27 (27) is a tick-group push variant.
	TopicTickVariant27 TopicID = 27
	// TopicTickVariant28 (28) is a tick-group push variant.
	TopicTickVariant28 TopicID = 28
	// TopicTickVariant37 (37) is a tick-group push variant.
	TopicTickVariant37 TopicID = 37
	// TopicBroker (16) is the broker-queue (买卖经纪) push.
	TopicBroker TopicID = 16
	// TopicOrderBook (17) is the order-book (买卖档) push.
	TopicOrderBook TopicID = 17
	// TopicOrderBookArcabook (25) is the ARCABOOK depth order-book push.
	TopicOrderBookArcabook TopicID = 25
	// TopicOrderBookTotalView (26) is the TOTALVIEW depth order-book push.
	TopicOrderBookTotalView TopicID = 26
	// TopicOrderBookVariant36 (36) is an order-book push variant.
	TopicOrderBookVariant36 TopicID = 36
)

// String returns a short human-readable label for the topic, or
// "TopicID(<n>)" when the value is not one of the defined constants.
func (t TopicID) String() string {
	switch t {
	case TopicBasicQot:
		return "basic-qot"
	case TopicQuoteVariant35:
		return "quote-variant-35"
	case TopicTicker:
		return "ticker"
	case TopicTickVariant27:
		return "tick-variant-27"
	case TopicTickVariant28:
		return "tick-variant-28"
	case TopicTickVariant37:
		return "tick-variant-37"
	case TopicBroker:
		return "broker"
	case TopicOrderBook:
		return "order-book"
	case TopicOrderBookArcabook:
		return "order-book-arcabook"
	case TopicOrderBookTotalView:
		return "order-book-totalview"
	case TopicOrderBookVariant36:
		return "order-book-variant-36"
	default:
		return "TopicID(" + strconv.Itoa(int(t)) + ")"
	}
}

// ExchangeType is a trading market identifier. The values are case-sensitive:
// Hong Kong and US are uppercase, while the two Stock Connect markets are
// lowercase.
type ExchangeType string

const (
	// ExchangeHK is the Hong Kong market ('K').
	ExchangeHK ExchangeType = "K"
	// ExchangeUS is the US market ('P').
	ExchangeUS ExchangeType = "P"
	// ExchangeShenzhenConnect is Shenzhen Stock Connect ('v').
	ExchangeShenzhenConnect ExchangeType = "v"
	// ExchangeShanghaiConnect is Shanghai Stock Connect ('t').
	ExchangeShanghaiConnect ExchangeType = "t"
)

// EntrustBS is the buy/sell direction of an order. Values 1 and 2 are the
// plain long open/close directions; 3 and 4 are used when the server must be
// told explicitly to close or open a short position.
type EntrustBS string

const (
	// EntrustBuy opens a long position (买入).
	EntrustBuy EntrustBS = "1"
	// EntrustSell closes a long position (卖出).
	EntrustSell EntrustBS = "2"
	// EntrustCloseShort closes a short position (空头平仓).
	EntrustCloseShort EntrustBS = "3"
	// EntrustOpenShort opens a short position (空头开仓).
	EntrustOpenShort EntrustBS = "4"
)

// EntrustType is the order type. The same code can carry a different meaning
// per market (for example "3" is limit everywhere), so the constants are named
// by meaning rather than by market. The set is non-exhaustive and the vendor
// may add codes; unknown values are forwarded unchanged.
type EntrustType string

const (
	// EntrustTypeAuctionLimit (0, HK) is auction limit (竞价限价).
	EntrustTypeAuctionLimit EntrustType = "0"
	// EntrustTypeAuction (1, HK) is auction (竞价).
	EntrustTypeAuction EntrustType = "1"
	// EntrustTypeEnhancedLimit (2, HK) is enhanced limit (增强限价盘).
	EntrustTypeEnhancedLimit EntrustType = "2"
	// EntrustTypeLimit (3, HK/US/A-share) is limit (限价盘).
	EntrustTypeLimit EntrustType = "3"
	// EntrustTypeSpecialLimit (4, HK) is special limit (特别限价盘).
	EntrustTypeSpecialLimit EntrustType = "4"
	// EntrustTypeMarket (5, US) is market (市价盘).
	EntrustTypeMarket EntrustType = "5"
	// EntrustTypeDarkPool (6, HK) is dark pool (暗盘).
	EntrustTypeDarkPool EntrustType = "6"
	// EntrustTypeOddLot (7, HK) is odd lot (碎股).
	EntrustTypeOddLot EntrustType = "7"
	// EntrustTypeIcebergMarket (8, US) is iceberg market (冰山市价).
	EntrustTypeIcebergMarket EntrustType = "8"
	// EntrustTypeIcebergLimit (9, US) is iceberg limit (冰山限价).
	EntrustTypeIcebergLimit EntrustType = "9"
	// EntrustTypeHiddenMarket (10, US) is hidden market (隐藏市价).
	EntrustTypeHiddenMarket EntrustType = "10"
	// EntrustTypeHiddenLimit (11, US) is hidden limit (隐藏限价).
	EntrustTypeHiddenLimit EntrustType = "11"
	// EntrustTypeStopProfitLimit (31) is a conditional stop-profit limit order.
	EntrustTypeStopProfitLimit EntrustType = "31"
	// EntrustTypeStopProfitMarket (32, US) is a conditional stop-profit market order.
	EntrustTypeStopProfitMarket EntrustType = "32"
	// EntrustTypeStopLossLimit (33) is a conditional stop-loss limit order.
	EntrustTypeStopLossLimit EntrustType = "33"
	// EntrustTypeStopLossMarket (34, US) is a conditional stop-loss market order.
	EntrustTypeStopLossMarket EntrustType = "34"
	// EntrustTypeTrailingStopLimit (35) is a conditional trailing stop-loss limit order.
	EntrustTypeTrailingStopLimit EntrustType = "35"
	// EntrustTypeTrailingStopMarket (36, US) is a conditional trailing stop-loss market order.
	EntrustTypeTrailingStopMarket EntrustType = "36"
)

// EntrustStatus is an order lifecycle status as published in the reference
// data dictionary. Several codes are documented as unused ("无用") but are
// retained so that responses can be decoded faithfully.
type EntrustStatus string

const (
	// EntrustStatusNoRegister (0) is 未报, not yet submitted.
	EntrustStatusNoRegister EntrustStatus = "0"
	// EntrustStatusWaitToRegister (1) is 待报, waiting to submit.
	EntrustStatusWaitToRegister EntrustStatus = "1"
	// EntrustStatusRegistered (2) is 已报, accepted by the host.
	EntrustStatusRegistered EntrustStatus = "2"
	// EntrustStatusWaitCancel (3) is 已报待撤, accepted and queued for cancel.
	EntrustStatusWaitCancel EntrustStatus = "3"
	// EntrustStatusPartFilledWaitCancel (4) is 部成待撤, partially filled and queued for cancel.
	EntrustStatusPartFilledWaitCancel EntrustStatus = "4"
	// EntrustStatusPartCancelled (5) is 部撤, partially cancelled.
	EntrustStatusPartCancelled EntrustStatus = "5"
	// EntrustStatusCancelled (6) is 已撤, cancelled.
	EntrustStatusCancelled EntrustStatus = "6"
	// EntrustStatusPartFilled (7) is 部成, partially filled.
	EntrustStatusPartFilled EntrustStatus = "7"
	// EntrustStatusFilled (8) is 已成, fully filled.
	EntrustStatusFilled EntrustStatus = "8"
	// EntrustStatusHostReject (9) is 废单, rejected by the host.
	EntrustStatusHostReject EntrustStatus = "9"
	// EntrustStatusWaitModifyRegistered (A) is 已报待改, accepted and queued for modify.
	EntrustStatusWaitModifyRegistered EntrustStatus = "A"
	// EntrustStatusUnregisteredUnused (B) is documented as unused.
	EntrustStatusUnregisteredUnused EntrustStatus = "B"
	// EntrustStatusRegisteringUnused (C) is documented as unused.
	EntrustStatusRegisteringUnused EntrustStatus = "C"
	// EntrustStatusRevokeCancelUnused (D) is documented as unused.
	EntrustStatusRevokeCancelUnused EntrustStatus = "D"
	// EntrustStatusWaitConfirming (W) is 待确认, waiting for confirmation.
	EntrustStatusWaitConfirming EntrustStatus = "W"
	// EntrustStatusPreFilledUnused (X) is documented as unused.
	EntrustStatusPreFilledUnused EntrustStatus = "X"
	// EntrustStatusWaitModifyPartFilled (E) is 部成待改, partially filled and queued for modify.
	EntrustStatusWaitModifyPartFilled EntrustStatus = "E"
	// EntrustStatusRejectPreOrder (F) is 预埋单检查废单, rejected pre-order check.
	EntrustStatusRejectPreOrder EntrustStatus = "F"
	// EntrustStatusCancelledPreOrder (G) is 预埋单已撤, pre-order cancelled.
	EntrustStatusCancelledPreOrder EntrustStatus = "G"
	// EntrustStatusWaitReview (H) is 待审核, waiting for review.
	EntrustStatusWaitReview EntrustStatus = "H"
	// EntrustStatusReviewFail (J) is 审核失败, review failed.
	EntrustStatusReviewFail EntrustStatus = "J"
)

// DataType is the instrument type carried in Security. The numbering is
// grouped by market (10000s Hong Kong, 20000s US, 30000s A-share) and is a
// closed set as documented; numbering gaps are intentional.
type DataType int32

const (
	// DataTypeHKStock (10000) is a Hong Kong stock.
	DataTypeHKStock DataType = 10000
	// DataTypeHKIndex (10001) is a Hong Kong index.
	DataTypeHKIndex DataType = 10001
	// DataTypeHKETF (10002) is a Hong Kong ETF.
	DataTypeHKETF DataType = 10002
	// DataTypeHKWarrant (10003) is a Hong Kong warrant (窝轮).
	DataTypeHKWarrant DataType = 10003
	// DataTypeHKCBBC (10004) is a Hong Kong callable bull/bear contract (牛熊证).
	DataTypeHKCBBC DataType = 10004
	// DataTypeHKBond (10005) is a Hong Kong bond.
	DataTypeHKBond DataType = 10005
	// DataTypeHKSector (10006) is a Hong Kong industry sector.
	DataTypeHKSector DataType = 10006
	// DataTypeHKConcept (10007) is a Hong Kong concept sector.
	DataTypeHKConcept DataType = 10007
	// DataTypeDerivativesFutures (10009) is a derivatives future.
	DataTypeDerivativesFutures DataType = 10009
	// DataTypeHKIndexFutures (10010) is a Hong Kong index future.
	DataTypeHKIndexFutures DataType = 10010
	// DataTypeHKSingleStockFutures (10011) is a Hong Kong single-stock future.
	DataTypeHKSingleStockFutures DataType = 10011
	// DataTypeHKHSIDividendFutures (10012) is an HSI dividend future.
	DataTypeHKHSIDividendFutures DataType = 10012
	// DataTypeHKCNYFutures (10013) is an HKD/CNY currency future.
	DataTypeHKCNYFutures DataType = 10013
	// DataTypeHKCESFutures (10014) is a CES (中华交易服务) future.
	DataTypeHKCESFutures DataType = 10014
	// DataTypeHKVolatilityFutures (10015) is an HSI volatility index future.
	DataTypeHKVolatilityFutures DataType = 10015
	// DataTypeInlineWarrant (10016) is an inline warrant (界内证).
	DataTypeInlineWarrant DataType = 10016
	// DataTypeUSStock (20000) is a US stock.
	DataTypeUSStock DataType = 20000
	// DataTypeUSIndex (20001) is a US index.
	DataTypeUSIndex DataType = 20001
	// DataTypeUSETF (20002) is a US ETF.
	DataTypeUSETF DataType = 20002
	// DataTypeUSOption (20003) is a US option.
	DataTypeUSOption DataType = 20003
	// DataTypeUSSector (20006) is a US industry sector.
	DataTypeUSSector DataType = 20006
	// DataTypeUSConcept (20007) is a US concept sector.
	DataTypeUSConcept DataType = 20007
	// DataTypeUSOTCStock (20009) is a US OTC stock.
	DataTypeUSOTCStock DataType = 20009
	// DataTypeAShareStock (30000) is an A-share stock.
	DataTypeAShareStock DataType = 30000
	// DataTypeAShareIndex (30001) is an A-share index.
	DataTypeAShareIndex DataType = 30001
	// DataTypeAShareETF (30002) is an A-share ETF.
	DataTypeAShareETF DataType = 30002
	// DataTypeAShareSector (30006) is an A-share industry sector.
	DataTypeAShareSector DataType = 30006
	// DataTypeAShareConcept (30007) is an A-share concept sector.
	DataTypeAShareConcept DataType = 30007
	// DataTypeAShareSTAR (30008) is an A-share STAR Market (科创板) instrument.
	DataTypeAShareSTAR DataType = 30008
)

// String returns a short human-readable label for the instrument type, or
// "DataType(<n>)" when the value is not one of the defined constants.
func (d DataType) String() string {
	switch d {
	case DataTypeHKStock:
		return "HK stock"
	case DataTypeHKIndex:
		return "HK index"
	case DataTypeHKETF:
		return "HK ETF"
	case DataTypeHKWarrant:
		return "HK warrant"
	case DataTypeHKCBBC:
		return "HK CBBC"
	case DataTypeHKBond:
		return "HK bond"
	case DataTypeHKSector:
		return "HK sector"
	case DataTypeHKConcept:
		return "HK concept"
	case DataTypeDerivativesFutures:
		return "derivatives future"
	case DataTypeHKIndexFutures:
		return "HK index future"
	case DataTypeHKSingleStockFutures:
		return "HK single-stock future"
	case DataTypeHKHSIDividendFutures:
		return "HSI dividend future"
	case DataTypeHKCNYFutures:
		return "HKD/CNY future"
	case DataTypeHKCESFutures:
		return "CES future"
	case DataTypeHKVolatilityFutures:
		return "HSI volatility future"
	case DataTypeInlineWarrant:
		return "inline warrant"
	case DataTypeUSStock:
		return "US stock"
	case DataTypeUSIndex:
		return "US index"
	case DataTypeUSETF:
		return "US ETF"
	case DataTypeUSOption:
		return "US option"
	case DataTypeUSSector:
		return "US sector"
	case DataTypeUSConcept:
		return "US concept"
	case DataTypeUSOTCStock:
		return "US OTC stock"
	case DataTypeAShareStock:
		return "A-share stock"
	case DataTypeAShareIndex:
		return "A-share index"
	case DataTypeAShareETF:
		return "A-share ETF"
	case DataTypeAShareSector:
		return "A-share sector"
	case DataTypeAShareConcept:
		return "A-share concept"
	case DataTypeAShareSTAR:
		return "A-share STAR"
	default:
		return "DataType(" + strconv.FormatInt(int64(d), 10) + ")"
	}
}
