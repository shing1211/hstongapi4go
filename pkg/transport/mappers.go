// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func marketFromCode(code string) domain.Market {
	code = strings.ToUpper(code)
	if strings.HasSuffix(code, ".HK") {
		return domain.MarketHK
	}
	if strings.HasSuffix(code, ".US") {
		return domain.MarketUS
	}
	if strings.HasSuffix(code, ".SZ") {
		return domain.MarketShenzhenConnect
	}
	if strings.HasSuffix(code, ".SH") {
		return domain.MarketShanghaiConnect
	}
	return ""
}

func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func MapSecurityToSymbol(code string, dtype int32) domain.Symbol {
	market := marketFromCode(code)
	return domain.NewSymbol(market, code, types.DataType(dtype))
}

func MapBasicQotToQuoteEvent(n *notify.BasicQotNotify) domain.QuoteEvent {
	sec := n.GetSecurity()
	symbol := MapSecurityToSymbol(sec.GetCode(), sec.GetDataType())
	return domain.QuoteEvent{
		Symbol:     symbol,
		LastPrice:  domain.MustNewPrice(floatToString(n.GetBasicQot().GetLastPrice()), "0.001"),
		OpenPrice:  domain.MustNewPrice(floatToString(n.GetBasicQot().GetOpenPrice()), "0.001"),
		HighPrice:  domain.MustNewPrice(floatToString(n.GetBasicQot().GetHighPrice()), "0.001"),
		LowPrice:   domain.MustNewPrice(floatToString(n.GetBasicQot().GetLowPrice()), "0.001"),
		ClosePrice: domain.MustNewPrice(floatToString(n.GetBasicQot().GetLastClosePrice()), "0.001"),
		Volume:     domain.MustNewQuantity(strconv.FormatInt(n.GetBasicQot().GetVolume(), 10)),
		Turnover:   domain.MustNewMoney(floatToString(n.GetBasicQot().GetTurnover()), "HKD", 3),
		Timestamp:  n.GetBasicQot().GetMarketTime(),
	}
}

func MapTickerToTickerEvent(n *dto.Ticker) domain.TickerEvent {
	return domain.TickerEvent{
		Symbol:    domain.Symbol{Code: "", Market: ""},
		Price:     domain.MustNewPrice(floatToString(n.GetPrice()), "0.001"),
		Volume:    domain.MustNewQuantity(strconv.FormatInt(n.GetVolume(), 10)),
		Turnover:  domain.MustNewMoney(floatToString(n.GetTurnover()), "HKD", 3),
		Timestamp: fmt.Sprintf("%d", n.GetTimestamp()),
		Side:      types.EntrustBS(strconv.FormatInt(int64(n.GetSide()), 10)),
	}
}

func MapOrderBookAskLevel(n *dto.OrderBook) domain.OrderBookLevel {
	return domain.OrderBookLevel{
		Price:    domain.MustNewPrice(floatToString(n.GetPrice()), "0.001"),
		Quantity: domain.MustNewQuantity(strconv.FormatInt(n.GetVolume(), 10)),
	}
}

// decimalOrZero normalises a Gateway string field for the decimal-backed domain
// constructors. Protobuf leaves an unset string field empty, and the domain
// constructors panic on an unparseable value, so an empty field would otherwise
// take down the caller's goroutine. Only the empty case is handled: a
// non-numeric value is still a hard error, because silently substituting a
// number for a malformed price or quantity would be worse than failing.
func decimalOrZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func MapTradeDeliveryToTradeEvent(n *tradenotify.TradeStockDeliverNotify) domain.TradeEvent {
	symbol := domain.NewSymbol(
		marketFromCode(n.GetStockCode()),
		n.GetStockCode(),
		0,
	)
	return domain.TradeEvent{
		Symbol:      symbol,
		OrderID:     domain.OrderID(n.GetClientId()),
		EntrustID:   domain.EntrustID(n.GetEntrustNo()),
		Price:       domain.MustNewPrice(decimalOrZero(n.GetBusinessPrice()), "0.001"),
		Quantity:    domain.MustNewQuantity(decimalOrZero(n.GetBusinessAmount())),
		Turnover:    domain.MustNewMoney(decimalOrZero(n.GetSumBusinessBalance()), "HKD", 3),
		Side:        types.EntrustBS(n.GetEntrustBs()),
		Timestamp:   n.GetBusinessDate() + " " + n.GetBusinessTime(),
		CounterID:   n.GetMatchNo(),
		OrderStatus: types.EntrustStatus(n.GetEntrustStatus()),
	}
}
