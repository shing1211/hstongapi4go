// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package mockgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// fixturePageSizeDefault mirrors the Gateway's documented pagination default
// (docs/SPEC.md §8): a list query with no explicit page size returns 20 rows.
const fixturePageSizeDefault = 20

// fixturePageSizeMax is the largest page size the mock will honour. The vendor
// documents "less than 100", so the cap is 99 and any larger request is clamped
// rather than rejected, matching ClampPageSize in pkg/hstong/trade.
const fixturePageSizeMax = 99

// FixtureSet is the collection of wire-level JSON response bodies the mock
// serves, keyed by canonical Gateway path. Build one with DefaultFixtures; the
// zero value serves nothing.
//
// Fixtures are deliberately raw JSON strings, never marshalled SDK types, so a
// change to a Go type cannot silently change the fixture (and the docs and the
// SDK can disagree visibly).
type FixtureSet struct {
	static  map[string]json.RawMessage
	cursors map[string]*cursorFixture
}

// cursorFixture is a list endpoint served one page at a time with the
// documented queryParamStr cursor. wrap is the JSON object key holding the rows
// ("data" or "holdsList"); cursorKey is the row field carrying the next cursor.
type cursorFixture struct {
	wrap      string
	cursorKey string
	rows      []json.RawMessage
}

// fixtureBuilder accumulates fixtures and validates them eagerly.
type fixtureBuilder struct {
	static  map[string]json.RawMessage
	cursors map[string]*cursorFixture
}

func newFixtureBuilder() *fixtureBuilder {
	return &fixtureBuilder{
		static:  make(map[string]json.RawMessage),
		cursors: make(map[string]*cursorFixture),
	}
}

// set registers a static response body for path. It panics on invalid JSON,
// which is a programming error in the fixture table.
func (b *fixtureBuilder) set(path, data string) {
	if !json.Valid([]byte(data)) {
		panic(fmt.Sprintf("mockgateway: fixture for %s is not valid JSON: %s", path, data))
	}
	b.static[path] = json.RawMessage(data)
}

// cursor registers a cursor-paginated list endpoint for path.
func (b *fixtureBuilder) cursor(path, wrap, cursorKey string, rows ...string) {
	raw := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		if !json.Valid([]byte(r)) {
			panic(fmt.Sprintf("mockgateway: cursor row for %s is not valid JSON: %s", path, r))
		}
		raw = append(raw, json.RawMessage(r))
	}
	b.cursors[path] = &cursorFixture{wrap: wrap, cursorKey: cursorKey, rows: raw}
}

// build freezes the accumulated fixtures.
func (b *fixtureBuilder) build() *FixtureSet {
	return &FixtureSet{static: b.static, cursors: b.cursors}
}

// Paths returns every registered canonical path, sorted lexicographically. The
// returned slice is a fresh copy.
func (f *FixtureSet) Paths() []string {
	if f == nil {
		return nil
	}
	out := make([]string, 0, len(f.static)+len(f.cursors))
	for p := range f.static {
		out = append(out, p)
	}
	for p := range f.cursors {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// lookup returns the wire JSON for path, applying cursor pagination when the
// endpoint is a list query. The boolean is false for an unregistered path.
func (f *FixtureSet) lookup(path string, params json.RawMessage) (json.RawMessage, bool) {
	if f == nil {
		return nil, false
	}
	if cf, ok := f.cursors[path]; ok {
		return cf.page(params), true
	}
	data, ok := f.static[path]
	return data, ok
}

// page renders one page of the cursor fixture for the request params, applying
// the documented default page size of 20 and the "less than 100" cap.
func (cf *cursorFixture) page(params json.RawMessage) json.RawMessage {
	var p struct {
		QueryCount    int    `json:"queryCount"`
		QueryParamStr string `json:"queryParamStr"`
	}
	_ = json.Unmarshal(params, &p)

	size := p.QueryCount
	if size <= 0 {
		size = fixturePageSizeDefault
	}
	if size > fixturePageSizeMax {
		size = fixturePageSizeMax
	}

	start := 0
	if p.QueryParamStr != "" {
		if n, err := strconv.Atoi(p.QueryParamStr); err == nil && n >= 0 {
			start = n
		}
	}
	if start > len(cf.rows) {
		start = len(cf.rows)
	}
	end := start + size
	if end > len(cf.rows) {
		end = len(cf.rows)
	}
	next := ""
	if end < len(cf.rows) {
		next = strconv.Itoa(end)
	}

	var buf bytes.Buffer
	buf.WriteString(`{"`)
	buf.WriteString(cf.wrap)
	buf.WriteString(`":[`)
	for i, row := range cf.rows[start:end] {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(setCursor(row, cf.cursorKey, next))
	}
	buf.WriteString(`]}`)
	return json.RawMessage(buf.Bytes())
}

// setCursor returns row with its cursor field set to cursor. A malformed row is
// returned unchanged.
func setCursor(row json.RawMessage, key, cursor string) json.RawMessage {
	if key == "" {
		return row
	}
	var obj map[string]any
	if err := json.Unmarshal(row, &obj); err != nil {
		return row
	}
	obj[key] = cursor
	encoded, err := json.Marshal(obj)
	if err != nil {
		return row
	}
	return encoded
}

// DefaultFixtures returns a fresh FixtureSet covering all 51 canonical
// endpoints with realistic, wire-level JSON. Values are shaped from docs/SPEC.md
// and the online reference; money and quantities are strings, matching the
// hand-written SDK types. Order lists are seeded with enough rows to exercise
// the documented cursor pagination.
func DefaultFixtures() *FixtureSet {
	b := newFixtureBuilder()

	// --- Market pull (9) ---------------------------------------------------

	b.set("/hq/BasicQot", `{"basicQot":[{"security":{"dataType":10000,"code":"00700.HK"},"isSuspended":false,"openPrice":380.0,"highPrice":392.0,"lowPrice":378.5,"lastPrice":388.0,"lastClosePrice":381.0,"priceSpread":0.2,"volume":12345678,"turnover":4800000000.0,"turnoverRate":0.55,"amplitude":3.1,"secStatus":0,"listTime":"20040616","lotSize":100,"tradeTime":"2026-09-21 15:59:59","marketTime":"2026-09-21 16:00:00","stockStatus":"NORMAL","volumeStr":"12345678","mktTmType":1}]}`)
	b.set("/hq/OrderBook", `{"security":{"dataType":10000,"code":"00700.HK"},"orderBookAskList":[{"level":1,"price":388.2,"volume":1200,"orederCount":3,"brokeId":"K"},{"level":2,"price":388.4,"volume":800,"orederCount":2,"brokeId":"K"}],"orderBookBidList":[{"level":1,"price":388.0,"volume":1500,"orederCount":4,"brokeId":"K"},{"level":2,"price":387.8,"volume":900,"orederCount":3,"brokeId":"K"}],"spreadLevel":0.2}`)
	b.set("/hq/KL", `{"security":{"dataType":10000,"code":"00700.HK"},"kline":[{"date":"2026-09-19","highPrice":392.0,"openPrice":381.0,"lowPrice":380.0,"closePrice":388.0,"lastClosePrice":379.0,"volume":12345678,"turnover":4800000000.0,"timestamp":1758240000000,"time":"16:00"}]}`)
	b.set("/hq/TimeShare", `{"security":{"dataType":10000,"code":"00700.HK"},"timeShare":[{"time":"09:30","price":381.0,"lastClosePrice":379.0,"avgPrice":381.2,"volume":1000,"turnover":381200.0}]}`)
	b.set("/hq/Ticker", `{"security":{"dataType":10000,"code":"00700.HK"},"ticker":[{"time":"15:59:59","side":1,"price":388.0,"volume":100,"turnover":38800.0,"type":0,"timestamp":1758240000000,"mktTmType":1}]}`)
	b.set("/hq/Broker", `{"security":{"dataType":10000,"code":"00700.HK"},"brokerAskList":[{"level":1,"item":"01111","type":1,"name":"Example Securities"}],"brokerBidList":[{"level":1,"item":"02222","type":0,"name":"Sample Brokerage"}]}`)
	b.set("/hq/UsOptionChainCode", `{"optionCode":["AAPL261016C00150000","AAPL261016P00150000"]}`)
	b.set("/hq/UsOptionChainExpireDate", `{"expireDate":["2026/10/16","2026/11/20"]}`)
	b.set("/hq/UsOverNightTradeCodes", `{"securityCodes":["AAPL","TSLA","NVDA"]}`)

	// --- Market subscription (2) ------------------------------------------

	b.set("/hq/Subscribe", `{}`)
	b.set("/hq/Unsubscribe", `{}`)

	// --- Trade session (2) -------------------------------------------------

	b.set("/trade/TradeLogin", `{"success":true}`)
	b.set("/trade/TradeLogout", `{"success":true}`)

	// --- Trade assets / positions (5) -------------------------------------

	b.set("/trade/TradeQueryMarginFundInfo", `{"holdsBalance":"0.00","assetBalance":"1000000.00","enableBalance":"500000.00","marketValue":"500000.00","cashOnHold":"0.00","creditValue":"0.00","creditLine":"0.00","fetchBalance":"500000.00","frozenBalance":"0.00","accountStatus":"NORMAL","spentRatio":"0.50","currentCreditLimit":"0.00","maxCreditLimit":"0.00","unitCreditLimit":"0.00","unitMaxCreditLimit":"0.00","buyPower":"500000.00","buyPowerCredit":"0.00","buyPowerHk":"500000.00","buyPowerUs":"64100.00","buyPowerCn":"450000.00","unitedBuyPowerHk":"500000.00","unitedBuyPowerUs":"64100.00","unitedBuyPowerCn":"450000.00","thirdBuyPowerHk":"0.00","thirdBuyPowerUs":"0.00","thirdBuyPowerCn":"0.00","buyPowerShortMarket":"0.00","buyPowerMoney":"500000.00"}`)
	b.set("/trade/TradeQueryHoldsList", `{"holdsList":[{"stockName":"Tencent Holdings","enableAmount":"100","currentAmount":"200","stockCode":"00700.HK","costPrice":"350.00","lastPrice":"388.00","incomeBalance":"7600.00","marketValue":"77600.00","marketValueRate":"21.71","incomeRatio":"7.76","dayCostPrice":"351.00","dayInComeAmount":"0.00","dayOutComeAmount":"0.00","keepCostPrice":"350.00","exchangeType":"K"}]}`)
	b.cursor("/trade/TradeQueryRealFundJourList", "data", "queryParamStr",
		`{"businessBalance":"-1000.00","type":"2","typeDesc":"Withdrawal","fundJourParentType":"1","queryParamStr":"1"}`,
		`{"businessBalance":"5000.00","type":"1","typeDesc":"Deposit","fundJourParentType":"1","queryParamStr":"2"}`,
	)
	b.cursor("/trade/TradeQueryHistoryFundJourList", "data", "queryParamStr",
		`{"businessBalance":"-1000.00","type":"2","typeDesc":"Withdrawal","fundJourParentType":"1","time":"2026-09-18 10:00:00","queryParamStr":"1"}`,
		`{"businessBalance":"5000.00","type":"1","typeDesc":"Deposit","fundJourParentType":"1","time":"2026-09-17 10:00:00","queryParamStr":"2"}`,
	)
	b.set("/hs/rate/queryList", `{"HKD":{"USD":"0.1279","CNY":"0.9210"},"USD":{"HKD":"7.8200","CNY":"7.2000"},"CNY":{"HKD":"1.0860","USD":"0.1389"}}`)

	// --- Trade orders (13) -------------------------------------------------

	b.set("/trade/TradeEntrust", `{"data":"E20260921001"}`)
	b.set("/trade/TradeCancelEntrust", `{"data":"E20260921001"}`)
	b.set("/trade/TradeBatchCancelEntrust", `{"successEntrustId":["E20260921001","E20260921002"],"failCancelEntrust":[{"failEntrustId":"E20260921003","remark":"order already filled"}]}`)
	b.set("/trade/TradeChangeEntrust", `{"data":"E20260921001"}`)
	b.set("/trade/TradeQueryMaxAvailableAsset", `{"data":{"positionStatus":"0","position":"0","longOpenAvailable":"2000","longCloseAvailable":"0","cashAvailableToOpen":"2000","marginAvailableToOpen":"4000","cashAndMarginAvailableToOpen":"6000","cashAvailableAmount":"500000.00","marginAvailableAmount":"1000000.00","cashAndMarginAvailableAmount":"1500000.00","shortOpenAvailable":"0","shortCloseAvailable":"0","shortCloseCashAvailable":"0","shortOpenPool":"0","shortBuyPower":"0.00","contractSize":"100","unitedBuyPowerStatus":"1","creditLimit":"0.00","optionLongMarginAmount":"0.00","optionShortMarginAmount":"0.00"}}`)
	b.cursor("/trade/TradeQueryRealEntrustList", "data", "queryParamStr", entrustOrderRows(120)...)
	b.cursor("/trade/TradeQueryRealDeliverList", "data", "queryParamStr", deliverOrderRows(2)...)
	b.set("/trade/TradeQueryRealCondOrderList", `{"data":[`+condOrderRow+`],"curPageNo":1,"curPageSize":1,"totalPages":1}`)
	b.cursor("/trade/TradeQueryHistoryEntrustList", "data", "queryParamStr", entrustOrderRows(2)...)
	b.cursor("/trade/TradeQueryHistoryDeliverList", "data", "queryParamStr", deliverOrderRows(2)...)
	b.set("/trade/TradeQueryHistoryCondOrderList", `{"data":[`+condOrderRow+`],"curPageNo":1,"curPageSize":1,"totalPages":1}`)
	b.set("/trade/TradeQueryMarginFullInfo", `{"marginAllow":"1","marginInitRatio":"0.50","marginKeepRatio":"0.30","shortAllow":"1","shortInitMarginRatio":"0.60","shortInterestRate":"0.08","shortKeepMarginRatio":"0.30","shortLastAvailableQty":"100000","rate":[{"currencyCode":"HKD","currencyDesc":"Hong Kong Dollar","interestRateWithinMortgage":"0.065"}]}`)
	b.set("/trade/TradeQueryBeforeAndAfterSupport", `{"data":"1"}`)

	// --- Trade push subscribe (2) -----------------------------------------

	b.set("/trade/TradeSubscribe", `{}`)
	b.set("/trade/TradeUnsubscribe", `{}`)

	// --- Algo (7) ----------------------------------------------------------

	b.set("/trade/AlgoAddOrder", `{"data":"MA20260921001"}`)
	b.set("/trade/AlgoCancelOrder", `{"data":"MA20260921001"}`)
	b.set("/trade/AlgoCancelEntrust", `{"data":"CH20260921001"}`)
	b.set("/trade/AlgoChangeOrder", `{"data":"MA20260921001"}`)
	b.set("/trade/AlgoActionOrder", `{"data":"MA20260921001"}`)
	b.set("/trade/AlgoQueryOrderList", `{"algoOrderList":[{"orderId":"MA20260921001","stockCode":"00700.HK","exchangeType":"K","tradeDate":"20260921","entrustType":"1","entrustPrice":"388.00","entrustAmount":"1000","cumQty":"200","leavesQty":"800","status":"1","entrustBs":"1","targetStrategy":"1","strategyStatus":"1","strategyParam":{"origStartTime":"093000","origEndTime":"160000","maxVolume":"100","minAmount":"10000","sensitivity":"1","qtyPercent":"10","interval":60},"roundLot":"100","sendingTime":"2026-09-21 09:30:00","transactionTime":"2026-09-21 09:30:01","avgPx":"388.10"}]}`)
	b.set("/trade/AlgoQueryEntrustIdList", `{"entrustId":["CH20260921001","CH20260921002"]}`)

	// --- Futures (11) ------------------------------------------------------

	b.set("/trade/FuturesQueryProductInfo", `{"productInfoVos":[{"prodCode":"HSI","instCode":"FUT","lotSize":50,"decInPrice":0,"contractSize":"50","priceDecimalPoint":"1","expiryDate":"2026-09-29","isSupportT1":0}]}`)
	b.set("/trade/FuturesQueryMaxBuySellAmount", `{"positionStatus":0,"maxBuyAmount":10,"maxSellAmount":5,"initialMargin":"50000.00","ccy":"HKD"}`)
	b.set("/trade/FuturesQueryFundInfo", `{"fundInfo":`+fundInfo+`}`)
	b.set("/trade/FuturesQueryHoldsList", `{"fundInfo":`+fundInfo+`,"holdsList":[`+futuresHoldRow+`]}`)
	b.set("/trade/FuturesEntrust", `{"data":"F20260921001"}`)
	b.set("/trade/FuturesCancelEntrust", `{"data":"F20260921001"}`)
	b.set("/trade/FuturesModifyEntrust", `{"data":"F20260921001"}`)
	b.set("/trade/FuturesQueryRealEntrustList", `{"data":[`+futuresOrderRow+`],"curPageNo":0,"curPageSize":0,"totalPageNo":0,"lastPage":0}`)
	b.set("/trade/FuturesQueryHistoryEntrustList", `{"data":[`+futuresOrderRow+`],"curPageNo":1,"curPageSize":20,"totalPageNo":1,"lastPage":1}`)
	b.set("/trade/FuturesQueryRealDeliverList", `{"data":[`+futuresOrderRow+`],"curPageNo":0,"curPageSize":0,"totalPageNo":0,"lastPage":0}`)
	b.set("/trade/FuturesQueryHistoryDeliverList", `{"data":[`+futuresOrderRow+`],"curPageNo":1,"curPageSize":20,"totalPageNo":1,"lastPage":1}`)

	return b.build()
}

// fundInfo is the futures account-funds snapshot reused by QueryFundInfo and
// QueryHoldsList. Every monetary field is a string.
const fundInfo = `{"assetBalance":"1000000.00","enableBalance":"500000.00","marginCall":"0.00","incomeBalance":"1200.00","cashBal":"300000.00","iMargin":"100000.00","mMargin":"50000.00","marginLevel":"2.50","maxMargin":"200000.00","creditLimit":"0.00","ctrlLevel":"1","marginClass":"1","aeId":"AE001","cashBalHKD":"250000.00","cashBalUSD":"6410.00","cycRateUSDtoHSD":"7.80","marginStatus":"1","statusPercent":"40","closeProfit":"500.00","cashBalCNH":"0.00"}`

// condOrderRow is one conditional order shared by the real and historical
// conditional-order queries.
const condOrderRow = `{"condOrderId":"C20260921001","dataType":"10000","stockCode":"00700.HK","stockName":"Tencent Holdings","stockNameTc":"騰訊控股","stockNameEn":"Tencent Holdings","exchangeType":"K","entrustType":"33","sessionType":"0","status":"1","canBeCancel":"1","canBeModify":"1","entrustBs":"1","entrustAmount":"100","createTime":"2026-09-21 09:00:00","startTime":"2026-09-21 09:30:00","endTime":"2026-09-21 16:00:00","errorCode":"","errorMsg":"","condValue":"380.00","condPrice":"381.00","condTrackType":"3"}`

// futuresHoldRow is one futures position.
const futuresHoldRow = `{"stockName":"Hang Seng Index Futures Sep26","stockCode":"HSI2609.HK","lastDayQty":"1","lastDayPrice":"24800","depQty":"1","dayLongQty":"0","dayLongPrice":"0","dayShortQty":"0","dayShortPrice":"0","dayNetQty":"0","dayNetPrice":"0","currentQty":"1","costPrice":"25000","lastPrice":"25100","preClosePrice":"24900","profitLoss":"500.00","ccyRate":"1.0","contractValue":"50","profitLossBaseCcy":"500.00","ccy":"HKD","dataType":"10010","closeProfit":"0.00","closeProfitHKD":"0.00"}`

// futuresOrderRow is one futures entrust or deliver row.
const futuresOrderRow = `{"stockCode":"HSI2609.HK","stockName":"Hang Seng Index Futures Sep26","businessPrice":"25100","entrustBs":"1","entrustPrice":"25000","entrustAmount":"1","businessAmount":"1","date":"20260921","businessTime":"10:00:00","entrustTime":"09:59:59","queryParamStr":"1","statusDesc":"Filled","status":"8","entrustId":"F20260921001","canBeCanceled":0,"entrustType":"0","entrustTypeNum":"0","isValid":1,"canBeUpdated":0,"validType":"0","validTypeDesc":"Day","orderOptions":0,"validTime":""}`

// entrustOrderRow is one stock entrust row template; n and cursor are the
// zero-based row index for a stable entrustId and cursor. Rendered by
// entrustOrderRows.
func entrustOrderRow(n int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"stockCode":"00700.HK","stockName":"Tencent Holdings","businessPrice":"388.00","entrustBs":"1","entrustPrice":"387.00","businessBalance":"38800.00","entrustAmount":"100","businessAmount":"100","date":"20260921","businessTime":"10:00:00","entrustTime":"09:59:59","queryParamStr":"%d","statusDesc":"Filled","status":"8","entrustId":"E20260921%05d","unBusinessAmount":"0","canBeCanceled":0,"entrustType":"3","opponentSeat":"","entrustTypeNum":"3","remarkType":"0","remark":"","fareVo":{"fare0":"30.00","fare1":"0.00","fare2":"2.00","fare3":"0.50","fare4":"0.00","fare5":"0.00","fare6":"15.00","fare7":"0.00","fare8":"0.00","fare9":"0.01","farex":"0.20","faret":"47.71"},"exchangeType":"K","canBeUpdated":0,"exchange":"SEHK"}`,
		n, n))
}

// entrustOrderRows renders n stock entrust rows.
func entrustOrderRows(n int) []string {
	rows := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, string(entrustOrderRow(i)))
	}
	return rows
}

// deliverOrderRows renders n stock deliver rows.
func deliverOrderRows(n int) []string {
	rows := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, fmt.Sprintf(
			`{"stockCode":"00700.HK","stockName":"Tencent Holdings","businessPrice":"388.00","entrustBs":"1","entrustPrice":"387.00","businessBalance":"38800.00","entrustAmount":"100","businessAmount":"100","date":"20260921","businessTime":"10:00:00","entrustTime":"09:59:59","queryParamStr":"%d","statusDesc":"Filled","status":"8","entrustId":"D20260921%05d","unBusinessAmount":"0","canBeCanceled":0,"entrustType":"3","opponentSeat":"SEHK","entrustTypeNum":"3","remarkType":"0","remark":"","fareVo":{"fare0":"30.00","fare1":"0.00","fare2":"2.00","fare3":"0.50","fare4":"0.00","fare5":"0.00","fare6":"15.00","fare7":"0.00","fare8":"0.00","fare9":"0.01","farex":"0.20","faret":"47.71"},"exchangeType":"K","canBeUpdated":0,"exchange":"SEHK"}`,
			i, i))
	}
	return rows
}
