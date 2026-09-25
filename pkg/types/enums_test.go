// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// notifyMsgTypes is every defined NotifyMsgType constant. Each entry is
// (constant, protobuf enum name), which is what String must return.
var notifyMsgTypes = []struct {
	id   NotifyMsgType
	want string
}{
	{TrsStockDeliverMsgType, "TrsStockDeliverMsgType"},
	{TradeStockDeliverMsgType, "TradeStockDeliverMsgType"},
	{FuturesTradeStockDeliverMsgType, "FuturesTradeStockDeliverMsgType"},
	{OrderBookNotifyMsgType, "OrderBookNotifyMsgType"},
	{BrokerQueueNotifyMsgType, "BrokerQueueNotifyMsgType"},
	{BasicQotNotifyMsgType, "BasicQotNotifyMsgType"},
	{TickerNotifyMsgType, "TickerNotifyMsgType"},
}

func TestNotifyMsgTypeString(t *testing.T) {
	for _, tt := range notifyMsgTypes {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("NotifyMsgType(%d).String() = %q, want %q", tt.id, got, tt.want)
		}
	}
}

// TestNotifyMsgTypeStringFallback pins the documented behaviour for a value that
// is not one of the defined constants.
func TestNotifyMsgTypeStringFallback(t *testing.T) {
	for _, id := range []NotifyMsgType{999, -1, 30000} {
		want := "NotifyMsgType(" + itoa(int64(id)) + ")"
		if got := id.String(); got != want {
			t.Errorf("NotifyMsgType(%d).String() = %q, want %q", id, got, want)
		}
	}
}

// topicIDs is every defined TopicID constant and its label.
var topicIDs = []struct {
	id   TopicID
	want string
}{
	{TopicBasicQot, "basic-qot"},
	{TopicQuoteVariant35, "quote-variant-35"},
	{TopicTicker, "ticker"},
	{TopicTickVariant27, "tick-variant-27"},
	{TopicTickVariant28, "tick-variant-28"},
	{TopicTickVariant37, "tick-variant-37"},
	{TopicBroker, "broker"},
	{TopicOrderBook, "order-book"},
	{TopicOrderBookArcabook, "order-book-arcabook"},
	{TopicOrderBookTotalView, "order-book-totalview"},
	{TopicOrderBookVariant36, "order-book-variant-36"},
}

func TestTopicIDString(t *testing.T) {
	for _, tt := range topicIDs {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("TopicID(%d).String() = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestTopicIDStringFallback(t *testing.T) {
	for _, id := range []TopicID{0, 1, 999, -7} {
		want := "TopicID(" + itoa(int64(id)) + ")"
		if got := id.String(); got != want {
			t.Errorf("TopicID(%d).String() = %q, want %q", id, got, want)
		}
	}
}

// dataTypes is every defined DataType constant and its label.
var dataTypes = []struct {
	id   DataType
	want string
}{
	{DataTypeHKStock, "HK stock"},
	{DataTypeHKIndex, "HK index"},
	{DataTypeHKETF, "HK ETF"},
	{DataTypeHKWarrant, "HK warrant"},
	{DataTypeHKCBBC, "HK CBBC"},
	{DataTypeHKBond, "HK bond"},
	{DataTypeHKSector, "HK sector"},
	{DataTypeHKConcept, "HK concept"},
	{DataTypeDerivativesFutures, "derivatives future"},
	{DataTypeHKIndexFutures, "HK index future"},
	{DataTypeHKSingleStockFutures, "HK single-stock future"},
	{DataTypeHKHSIDividendFutures, "HSI dividend future"},
	{DataTypeHKCNYFutures, "HKD/CNY future"},
	{DataTypeHKCESFutures, "CES future"},
	{DataTypeHKVolatilityFutures, "HSI volatility future"},
	{DataTypeInlineWarrant, "inline warrant"},
	{DataTypeUSStock, "US stock"},
	{DataTypeUSIndex, "US index"},
	{DataTypeUSETF, "US ETF"},
	{DataTypeUSOption, "US option"},
	{DataTypeUSSector, "US sector"},
	{DataTypeUSConcept, "US concept"},
	{DataTypeUSOTCStock, "US OTC stock"},
	{DataTypeAShareStock, "A-share stock"},
	{DataTypeAShareIndex, "A-share index"},
	{DataTypeAShareETF, "A-share ETF"},
	{DataTypeAShareSector, "A-share sector"},
	{DataTypeAShareConcept, "A-share concept"},
	{DataTypeAShareSTAR, "A-share STAR"},
}

func TestDataTypeString(t *testing.T) {
	for _, tt := range dataTypes {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("DataType(%d).String() = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestDataTypeStringFallback(t *testing.T) {
	for _, id := range []DataType{0, -1, 99999} {
		want := "DataType(" + itoa(int64(id)) + ")"
		if got := id.String(); got != want {
			t.Errorf("DataType(%d).String() = %q, want %q", id, got, want)
		}
	}
}

// TestEnumLabelsAreDistinct guards against a copy-paste defect that the literal
// assertions above would tolerate: two constants resolving to the same label
// because one case body was duplicated. Distinct labels also matter to users,
// who select on them when filtering or logging.
func TestEnumLabelsAreDistinct(t *testing.T) {
	t.Run("NotifyMsgType", func(t *testing.T) {
		seen := make(map[string]NotifyMsgType, len(notifyMsgTypes))
		for _, tt := range notifyMsgTypes {
			if prev, dup := seen[tt.want]; dup {
				t.Errorf("NotifyMsgType(%d) and NotifyMsgType(%d) both render as %q", prev, tt.id, tt.want)
			}
			seen[tt.want] = tt.id
		}
	})

	t.Run("TopicID", func(t *testing.T) {
		seen := make(map[string]TopicID, len(topicIDs))
		for _, tt := range topicIDs {
			if prev, dup := seen[tt.want]; dup {
				t.Errorf("TopicID(%d) and TopicID(%d) both render as %q", prev, tt.id, tt.want)
			}
			seen[tt.want] = tt.id
		}
	})

	t.Run("DataType", func(t *testing.T) {
		seen := make(map[string]DataType, len(dataTypes))
		for _, tt := range dataTypes {
			if prev, dup := seen[tt.want]; dup {
				t.Errorf("DataType(%d) and DataType(%d) both render as %q", prev, tt.id, tt.want)
			}
			seen[tt.want] = tt.id
		}
	})
}

// TestEnumLabelsAreNonEmpty rejects an empty label, which would render as a
// blank field in a log line and look like a missing value rather than a defect.
func TestEnumLabelsAreNonEmpty(t *testing.T) {
	for _, tt := range notifyMsgTypes {
		if tt.id.String() == "" {
			t.Errorf("NotifyMsgType(%d).String() is empty", tt.id)
		}
	}
	for _, tt := range topicIDs {
		if tt.id.String() == "" {
			t.Errorf("TopicID(%d).String() is empty", tt.id)
		}
	}
	for _, tt := range dataTypes {
		if tt.id.String() == "" {
			t.Errorf("DataType(%d).String() is empty", tt.id)
		}
	}
}

// statusCodes is every defined StatusCode and its description.
var statusCodes = []struct {
	id   StatusCode
	want string
}{
	{StatusOK, "success"},
	{StatusUnknownError, "unknown system error"},
	{StatusSignatureError, "signature error"},
	{StatusEncryptionError, "data encryption error"},
	{StatusSocketNotInitialized, "socket not initialized"},
	{StatusEndpointDeprecated, "endpoint deprecated"},
	{StatusUserNotAuthorized, "user not authorized"},
	{StatusDuplicateSubmit, "duplicate submission"},
	{StatusCallFailed, "call failed"},
	{StatusEndpointNotFound, "endpoint not found"},
	{StatusIllegalRequest, "illegal request"},
	{StatusServiceBusy, "service busy, retry later"},
	{StatusNotLoggedIn, "not logged in"},
	{StatusKickedOffline, "session displaced"},
	{StatusLoginTimeout, "login timeout"},
	{StatusCallTimeout, "call timeout"},
	{StatusInvalidParam, "invalid parameter"},
	{StatusConnectFailed, "long-connection establishment failed"},
	{StatusReconnecting, "reconnecting, retry later"},
	{StatusFuturesLoginTimeout, "futures trade login timeout"},
	{StatusQueryProductInfoFailed, "query product info failed"},
	{StatusQueryContractInfoFailed, "query contract info failed"},
}

func TestStatusCodeString(t *testing.T) {
	for _, tt := range statusCodes {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("StatusCode(%q).String() = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestStatusCodeStringFallback(t *testing.T) {
	// An undocumented code renders as itself so an operator sees what the
	// Gateway actually sent instead of a placeholder.
	for _, id := range []StatusCode{"9999", "", "abc"} {
		want := string(id)
		if got := id.String(); got != want {
			t.Errorf("StatusCode(%q).String() = %q, want the raw code %q", id, got, want)
		}
	}
}

// TestStatusCodeIsSuccess pins the only success code. Every other documented
// code, including the retryable ones, must be false: treating 1011 or 1018 as
// success would silently drop a real failure.
func TestStatusCodeIsSuccess(t *testing.T) {
	if !StatusOK.IsSuccess() {
		t.Error("StatusOK.IsSuccess() = false, want true")
	}
	for _, tt := range statusCodes {
		if tt.id == StatusOK {
			continue
		}
		if tt.id.IsSuccess() {
			t.Errorf("StatusCode(%q).IsSuccess() = true, want false", tt.id)
		}
	}
	if StatusCode("9999").IsSuccess() {
		t.Error("an undocumented status code must not report success")
	}
	if StatusCode("").IsSuccess() {
		t.Error("the empty status code must not report success")
	}
}

// itoa is a test-local helper so the fallback assertions read clearly without
// importing strconv alongside the package's own use of it.
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
