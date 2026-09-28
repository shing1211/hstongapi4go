// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"fmt"
	"strconv"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func decodeBasicQotEvent(notify *hqnotify.BasicQotNotify, notifyTime uint64) domain.QuoteEvent {
	event := domain.QuoteEvent{
		Symbol: pushSymbolFromSecurity(notify.GetSecurity()),
	}
	if q := notify.GetBasicQot(); q != nil {
		event.LastPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastPrice(), 'f', -1, 64), wireTick)
		event.OpenPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetOpenPrice(), 'f', -1, 64), wireTick)
		event.HighPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetHighPrice(), 'f', -1, 64), wireTick)
		event.LowPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLowPrice(), 'f', -1, 64), wireTick)
		event.ClosePrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastClosePrice(), 'f', -1, 64), wireTick)
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(q.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(q.GetTurnover(), 'f', -1, 64), "HKD", 3)
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeTickerEvent(notify *hqnotify.TickerNotify, notifyTime uint64) domain.TickerEvent {
	event := domain.TickerEvent{
		Symbol: pushSymbolFromSecurity(notify.GetSecurity()),
	}
	if t := notify.GetTicker(); t != nil {
		event.Price = domain.MustNewPrice(strconv.FormatFloat(t.GetPrice(), 'f', -1, 64), wireTick)
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(t.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(t.GetTurnover(), 'f', -1, 64), "HKD", 3)
		event.Side = entrustBSFromInt32(t.GetSide())
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeOrderBookEvent(notify *hqnotify.OrderBookFullNotify, notifyTime uint64) domain.OrderBookEvent {
	event := domain.OrderBookEvent{
		Symbol:    pushSymbolFromSecurity(notify.GetSecurity()),
		Bids:      make([]domain.OrderBookLevel, 0),
		Asks:      make([]domain.OrderBookLevel, 0),
		Depth:     0,
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, ob := range notify.GetOrderBookList() {
		level := domain.OrderBookLevel{
			Level:    ob.GetLevel(),
			Price:    domain.MustNewPrice(strconv.FormatFloat(ob.GetPrice(), 'f', -1, 64), wireTick),
			Quantity: domain.MustNewQuantity(strconv.FormatInt(ob.GetVolume(), 10)),
		}
		if notify.GetSide() == 0 {
			event.Bids = append(event.Bids, level)
		} else {
			event.Asks = append(event.Asks, level)
		}
	}
	event.Depth = len(event.Bids) + len(event.Asks)
	return event
}

// decodeBrokerEvent maps a pushed broker queue.
//
// The push broker frame carries level, item, type, and name and nothing else. It
// used to publish a domain.BrokerLevel{BrokerID, Quantity: "0"} and discard the
// other three: a hardcoded zero in a field named like the order-book decoder's
// Quantity, which reads as an observed quantity to a caller and to a reviewer
// comparing the two decoders side by side. domain.BrokerLevel no longer has the
// field to fill, so the mistake is unrepresentable rather than merely removed;
// see that type's own doc.
func decodeBrokerEvent(notify *hqnotify.BrokerNotify, notifyTime uint64) domain.BrokerEvent {
	event := domain.BrokerEvent{
		Symbol:    pushSymbolFromSecurity(notify.GetSecurity()),
		Buyers:    make([]domain.BrokerLevel, 0),
		Sellers:   make([]domain.BrokerLevel, 0),
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, b := range notify.GetBrokerList() {
		bl := domain.BrokerLevel{
			Level: b.GetLevel(),
			Item:  b.GetItem(),
			Type:  b.GetType(),
			Name:  b.GetName(),
		}
		if notify.GetSide() == 0 {
			event.Buyers = append(event.Buyers, bl)
		} else {
			event.Sellers = append(event.Sellers, bl)
		}
	}
	return event
}

func decodeTradeEvent(notify *tradenotify.TradeStockDeliverNotify, notifyTime uint64) domain.TradeEvent {
	event := domain.TradeEvent{
		Symbol:      pushSymbol(notify.GetStockCode(), 0),
		Price:       domain.MustNewPrice(pushDecimal(notify.GetBusinessPrice()), wireTick),
		Quantity:    domain.MustNewQuantity(pushDecimal(notify.GetBusinessAmount())),
		Turnover:    domain.MustNewMoney(pushDecimal(notify.GetSumBusinessBalance()), "HKD", 3),
		OrderID:     domain.OrderID(notify.GetClientId()),
		EntrustID:   domain.EntrustID(notify.GetEntrustNo()),
		CounterID:   notify.GetMatchNo(),
		Timestamp:   fmt.Sprintf("%d", notifyTime),
		Side:        types.EntrustBS(notify.GetEntrustBs()),
		OrderStatus: types.EntrustStatus(notify.GetEntrustStatus()),
	}
	return event
}

// wireTick is the tick every price decoded off a push frame carries.
//
// It is "0" and not the invented three-decimal tick these call sites used to
// hardcode. These prices are the most wire-derived numbers in the SDK: the
// Gateway sent the digits, and this package has no instrument master and so no
// instrument's tick schedule. A non-zero tick here states as fact something the
// SDK invented, and it does so in the type rather than in a comment, so
// Price.Validate would reject a legitimate price and Price.Round would round it
// to a different one (design-tick-model.md 2.1). "0" says "I observed this; I do
// not know this instrument's tick".
//
// The strconv.FormatFloat(..., 'f', -1, 64) around these values is a different
// question and is left alone. The wire field is a protobuf double, so 'f' with
// precision -1 already renders the shortest decimal that round-trips that
// binary64 exactly; the limit on fidelity is the wire type, and no local
// formatting choice can improve on it.
const wireTick = "0"

// pushDecimal normalises a Gateway string field for the decimal-backed domain
// constructors, which panic on an unparseable value.
//
// Protobuf leaves an unset string field empty, so an absent businessPrice or
// businessAmount on a delivery frame would otherwise panic inside a push
// dispatcher goroutine, which no caller can recover from. Only the empty case is
// handled: a malformed number is still a hard error at decode time rather than
// silently becoming a number, because substituting one would be the same
// fabricated-value defect this package was corrected for elsewhere.
func pushDecimal(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// pushSymbolFromSecurity maps a pushed frame's security onto the domain Symbol,
// tolerating an absent security by yielding the zero Symbol.
func pushSymbolFromSecurity(sec *dto.Security) domain.Symbol {
	if sec == nil {
		return domain.Symbol{}
	}
	return pushSymbol(sec.GetCode(), sec.GetDataType())
}

// pushSymbol builds the domain Symbol for a Gateway instrument code.
//
// It deliberately does not call domain.SymbolFromSecurity or domain.NewSymbol.
// Those derive a market from the code and then append it again, so a code the
// Gateway already suffixed yields FullCode "00700.HK.HK". Push routing does not
// read FullCode, but FullCode is on a public type now and a doubled suffix is a
// string a caller would read as an instrument. Code is kept verbatim - it is the
// routing key - and FullCode carries exactly one suffix, which is also what an
// unsuffixed code has always produced.
func pushSymbol(code string, dataType int32) domain.Symbol {
	market := domain.MarketFromCode(code)
	bare := code
	if market != "" {
		bare = code[:len(code)-len(string(market))-1]
	}
	full := bare
	if market != "" {
		full = bare + "." + string(market)
	}
	return domain.Symbol{
		Market:   market,
		Code:     code,
		DataType: types.DataType(dataType),
		FullCode: full,
	}
}

func entrustBSFromInt32(side int32) types.EntrustBS {
	switch side {
	case 1:
		return types.EntrustBuy
	case 2:
		return types.EntrustSell
	case 3:
		return types.EntrustCloseShort
	case 4:
		return types.EntrustOpenShort
	default:
		return types.EntrustBS(strconv.FormatInt(int64(side), 10))
	}
}
