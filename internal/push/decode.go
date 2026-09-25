// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"fmt"
	"strconv"

	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func decodeBasicQotEvent(notify *hqnotify.BasicQotNotify, notifyTime uint64) domain.QuoteEvent {
	event := domain.QuoteEvent{
		Symbol: domain.SymbolFromSecurity(notify.GetSecurity()),
	}
	if q := notify.GetBasicQot(); q != nil {
		event.LastPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastPrice(), 'f', -1, 64), "0.001")
		event.OpenPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetOpenPrice(), 'f', -1, 64), "0.001")
		event.HighPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetHighPrice(), 'f', -1, 64), "0.001")
		event.LowPrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLowPrice(), 'f', -1, 64), "0.001")
		event.ClosePrice = domain.MustNewPrice(strconv.FormatFloat(q.GetLastClosePrice(), 'f', -1, 64), "0.001")
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(q.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(q.GetTurnover(), 'f', -1, 64), "HKD", 3)
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeTickerEvent(notify *hqnotify.TickerNotify, notifyTime uint64) domain.TickerEvent {
	event := domain.TickerEvent{
		Symbol: domain.SymbolFromSecurity(notify.GetSecurity()),
	}
	if t := notify.GetTicker(); t != nil {
		event.Price = domain.MustNewPrice(strconv.FormatFloat(t.GetPrice(), 'f', -1, 64), "0.001")
		event.Volume = domain.MustNewQuantity(strconv.FormatInt(t.GetVolume(), 10))
		event.Turnover = domain.MustNewMoney(strconv.FormatFloat(t.GetTurnover(), 'f', -1, 64), "HKD", 3)
		event.Side = entrustBSFromInt32(t.GetSide())
	}
	event.Timestamp = fmt.Sprintf("%d", notifyTime)
	return event
}

func decodeOrderBookEvent(notify *hqnotify.OrderBookFullNotify, notifyTime uint64) domain.OrderBookEvent {
	event := domain.OrderBookEvent{
		Symbol:    domain.SymbolFromSecurity(notify.GetSecurity()),
		Bids:      make([]domain.OrderBookLevel, 0),
		Asks:      make([]domain.OrderBookLevel, 0),
		Depth:     0,
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, ob := range notify.GetOrderBookList() {
		level := domain.OrderBookLevel{
			Level:    ob.GetLevel(),
			Price:    domain.MustNewPrice(strconv.FormatFloat(ob.GetPrice(), 'f', -1, 64), "0.001"),
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

func decodeBrokerEvent(notify *hqnotify.BrokerNotify, notifyTime uint64) domain.BrokerEvent {
	event := domain.BrokerEvent{
		Symbol:    domain.SymbolFromSecurity(notify.GetSecurity()),
		Buyers:    make([]domain.BrokerLevel, 0),
		Sellers:   make([]domain.BrokerLevel, 0),
		Timestamp: fmt.Sprintf("%d", notifyTime),
	}
	for _, b := range notify.GetBrokerList() {
		bl := domain.BrokerLevel{
			BrokerID: int(b.GetLevel()),
			Quantity: domain.MustNewQuantity("0"),
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
		Symbol:      domain.NewSymbol(domain.MarketFromCode(notify.GetStockCode()), notify.GetStockCode(), 0),
		Price:       domain.MustNewPrice(notify.GetBusinessPrice(), "0.001"),
		Quantity:    domain.MustNewQuantity(notify.GetBusinessAmount()),
		Turnover:    domain.MustNewMoney(notify.GetBusinessAmount(), "HKD", 3),
		CounterID:   notify.GetMatchNo(),
		Timestamp:   fmt.Sprintf("%d", notifyTime),
		Side:        types.EntrustBS(notify.GetEntrustBs()),
		OrderStatus: types.EntrustStatus(notify.GetEntrustStatus()),
	}
	return event
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
