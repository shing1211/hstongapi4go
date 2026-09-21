// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUnknownRoute is returned by Route.Validate when a Route is not one of the
// registered canonical Gateway endpoints. It is wrapped with the offending path
// so errors.Is(err, ErrUnknownRoute) keeps working.
var ErrUnknownRoute = errors.New("client: unknown route")

// Route is a canonical Gateway endpoint path, for example "/hq/BasicQot". The
// Gateway additionally accepts the two aliases "<path>Request" and
// "<path>RequestMsgType" for every route; those are mapped back to the
// canonical Route by NormalizePath.
type Route string

// The 51 canonical Gateway endpoints (plan.md §Appendix A). The constant names
// are mechanical: "Route" followed by the capitalised path segments, so
// "/trade/FuturesEntrust" is RouteTradeFuturesEntrust and "/hs/rate/queryList"
// is RouteHsRateQueryList.
const (
	// Market pull (9).
	RouteHqBasicQot                Route = "/hq/BasicQot"
	RouteHqOrderBook               Route = "/hq/OrderBook"
	RouteHqKL                      Route = "/hq/KL"
	RouteHqTimeShare               Route = "/hq/TimeShare"
	RouteHqTicker                  Route = "/hq/Ticker"
	RouteHqBroker                  Route = "/hq/Broker"
	RouteHqUsOptionChainCode       Route = "/hq/UsOptionChainCode"
	RouteHqUsOptionChainExpireDate Route = "/hq/UsOptionChainExpireDate"
	RouteHqUsOverNightTradeCodes   Route = "/hq/UsOverNightTradeCodes"

	// Market subscription (2).
	RouteHqSubscribe   Route = "/hq/Subscribe"
	RouteHqUnsubscribe Route = "/hq/Unsubscribe"

	// Trade session (2).
	RouteTradeLogin  Route = "/trade/TradeLogin"
	RouteTradeLogout Route = "/trade/TradeLogout"

	// Trade assets/positions (5).
	RouteTradeQueryMarginFundInfo      Route = "/trade/TradeQueryMarginFundInfo"
	RouteTradeQueryHoldsList           Route = "/trade/TradeQueryHoldsList"
	RouteTradeQueryRealFundJourList    Route = "/trade/TradeQueryRealFundJourList"
	RouteTradeQueryHistoryFundJourList Route = "/trade/TradeQueryHistoryFundJourList"
	RouteHsRateQueryList               Route = "/hs/rate/queryList"

	// Trade orders (13).
	RouteTradeEntrust                    Route = "/trade/TradeEntrust"
	RouteTradeCancelEntrust              Route = "/trade/TradeCancelEntrust"
	RouteTradeBatchCancelEntrust         Route = "/trade/TradeBatchCancelEntrust"
	RouteTradeChangeEntrust              Route = "/trade/TradeChangeEntrust"
	RouteTradeQueryMaxAvailableAsset     Route = "/trade/TradeQueryMaxAvailableAsset"
	RouteTradeQueryRealEntrustList       Route = "/trade/TradeQueryRealEntrustList"
	RouteTradeQueryRealDeliverList       Route = "/trade/TradeQueryRealDeliverList"
	RouteTradeQueryRealCondOrderList     Route = "/trade/TradeQueryRealCondOrderList"
	RouteTradeQueryHistoryEntrustList    Route = "/trade/TradeQueryHistoryEntrustList"
	RouteTradeQueryHistoryDeliverList    Route = "/trade/TradeQueryHistoryDeliverList"
	RouteTradeQueryHistoryCondOrderList  Route = "/trade/TradeQueryHistoryCondOrderList"
	RouteTradeQueryMarginFullInfo        Route = "/trade/TradeQueryMarginFullInfo"
	RouteTradeQueryBeforeAndAfterSupport Route = "/trade/TradeQueryBeforeAndAfterSupport"

	// Trade push (2).
	RouteTradeSubscribe   Route = "/trade/TradeSubscribe"
	RouteTradeUnsubscribe Route = "/trade/TradeUnsubscribe"

	// Algo (7).
	RouteTradeAlgoAddOrder           Route = "/trade/AlgoAddOrder"
	RouteTradeAlgoCancelOrder        Route = "/trade/AlgoCancelOrder"
	RouteTradeAlgoCancelEntrust      Route = "/trade/AlgoCancelEntrust"
	RouteTradeAlgoChangeOrder        Route = "/trade/AlgoChangeOrder"
	RouteTradeAlgoActionOrder        Route = "/trade/AlgoActionOrder"
	RouteTradeAlgoQueryOrderList     Route = "/trade/AlgoQueryOrderList"
	RouteTradeAlgoQueryEntrustIdList Route = "/trade/AlgoQueryEntrustIdList"

	// Futures (11).
	RouteTradeFuturesQueryProductInfo        Route = "/trade/FuturesQueryProductInfo"
	RouteTradeFuturesQueryMaxBuySellAmount   Route = "/trade/FuturesQueryMaxBuySellAmount"
	RouteTradeFuturesQueryFundInfo           Route = "/trade/FuturesQueryFundInfo"
	RouteTradeFuturesQueryHoldsList          Route = "/trade/FuturesQueryHoldsList"
	RouteTradeFuturesEntrust                 Route = "/trade/FuturesEntrust"
	RouteTradeFuturesCancelEntrust           Route = "/trade/FuturesCancelEntrust"
	RouteTradeFuturesModifyEntrust           Route = "/trade/FuturesModifyEntrust"
	RouteTradeFuturesQueryRealEntrustList    Route = "/trade/FuturesQueryRealEntrustList"
	RouteTradeFuturesQueryHistoryEntrustList Route = "/trade/FuturesQueryHistoryEntrustList"
	RouteTradeFuturesQueryRealDeliverList    Route = "/trade/FuturesQueryRealDeliverList"
	RouteTradeFuturesQueryHistoryDeliverList Route = "/trade/FuturesQueryHistoryDeliverList"
)

// canonicalRoutes is the registry of every valid Route. Validate consults it;
// NormalizePath consults it so an alias is only rewritten when its canonical
// form is a known endpoint.
var canonicalRoutes = map[Route]struct{}{
	RouteHqBasicQot:                {},
	RouteHqOrderBook:               {},
	RouteHqKL:                      {},
	RouteHqTimeShare:               {},
	RouteHqTicker:                  {},
	RouteHqBroker:                  {},
	RouteHqUsOptionChainCode:       {},
	RouteHqUsOptionChainExpireDate: {},
	RouteHqUsOverNightTradeCodes:   {},

	RouteHqSubscribe:   {},
	RouteHqUnsubscribe: {},

	RouteTradeLogin:  {},
	RouteTradeLogout: {},

	RouteTradeQueryMarginFundInfo:      {},
	RouteTradeQueryHoldsList:           {},
	RouteTradeQueryRealFundJourList:    {},
	RouteTradeQueryHistoryFundJourList: {},
	RouteHsRateQueryList:               {},

	RouteTradeEntrust:                    {},
	RouteTradeCancelEntrust:              {},
	RouteTradeBatchCancelEntrust:         {},
	RouteTradeChangeEntrust:              {},
	RouteTradeQueryMaxAvailableAsset:     {},
	RouteTradeQueryRealEntrustList:       {},
	RouteTradeQueryRealDeliverList:       {},
	RouteTradeQueryRealCondOrderList:     {},
	RouteTradeQueryHistoryEntrustList:    {},
	RouteTradeQueryHistoryDeliverList:    {},
	RouteTradeQueryHistoryCondOrderList:  {},
	RouteTradeQueryMarginFullInfo:        {},
	RouteTradeQueryBeforeAndAfterSupport: {},

	RouteTradeSubscribe:   {},
	RouteTradeUnsubscribe: {},

	RouteTradeAlgoAddOrder:           {},
	RouteTradeAlgoCancelOrder:        {},
	RouteTradeAlgoCancelEntrust:      {},
	RouteTradeAlgoChangeOrder:        {},
	RouteTradeAlgoActionOrder:        {},
	RouteTradeAlgoQueryOrderList:     {},
	RouteTradeAlgoQueryEntrustIdList: {},

	RouteTradeFuturesQueryProductInfo:        {},
	RouteTradeFuturesQueryMaxBuySellAmount:   {},
	RouteTradeFuturesQueryFundInfo:           {},
	RouteTradeFuturesQueryHoldsList:          {},
	RouteTradeFuturesEntrust:                 {},
	RouteTradeFuturesCancelEntrust:           {},
	RouteTradeFuturesModifyEntrust:           {},
	RouteTradeFuturesQueryRealEntrustList:    {},
	RouteTradeFuturesQueryHistoryEntrustList: {},
	RouteTradeFuturesQueryRealDeliverList:    {},
	RouteTradeFuturesQueryHistoryDeliverList: {},
}

// aliasSuffixMsgType is the trailing alias suffix that carries the notification
// message-type form of a route, stripped before aliasSuffixRequest.
const (
	aliasSuffixMsgType = "RequestMsgType"
	aliasSuffixRequest = "Request"
)

// Path returns the canonical request path of the route, suitable for the
// transport's Do. The empty Route returns the empty string.
func (r Route) Path() string {
	return string(r)
}

// String returns the route's canonical path. It makes Route usable directly in
// log and error formatting.
func (r Route) String() string {
	return string(r)
}

// Validate reports whether r is one of the registered canonical Gateway
// endpoints. It returns an error wrapping ErrUnknownRoute (with the offending
// path) when it is not, so callers can test with errors.Is(err, ErrUnknownRoute).
func (r Route) Validate() error {
	if _, ok := canonicalRoutes[r]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownRoute, string(r))
	}
	return nil
}

// Routes returns every registered canonical Route as a new slice, sorted
// lexicographically by canonical path. Callers may modify the returned slice
// without affecting the registry.
func Routes() []Route {
	out := make([]Route, 0, len(canonicalRoutes))
	for r := range canonicalRoutes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NormalizePath maps any of the three Gateway alias forms of a route to its
// canonical "surface/Name" path:
//
//	/hq/BasicQotRequestMsgType -> /hq/BasicQot
//	/hq/BasicQotRequest        -> /hq/BasicQot
//	/hq/BasicQot               -> /hq/BasicQot
//
// A trailing "RequestMsgType" is stripped before a trailing "Request". A path
// is returned unchanged when it is already canonical or when the stripped form
// is not a registered route, so an unknown path is never invented and a
// canonical path is never mangled. The empty string is returned unchanged.
// NormalizePath is idempotent: applying it to its own output is a no-op.
func NormalizePath(p string) string {
	if _, ok := canonicalRoutes[Route(p)]; ok {
		return p
	}
	if stripped := strings.TrimSuffix(p, aliasSuffixMsgType); stripped != p {
		if _, ok := canonicalRoutes[Route(stripped)]; ok {
			return stripped
		}
	}
	if stripped := strings.TrimSuffix(p, aliasSuffixRequest); stripped != p {
		if _, ok := canonicalRoutes[Route(stripped)]; ok {
			return stripped
		}
	}
	return p
}
