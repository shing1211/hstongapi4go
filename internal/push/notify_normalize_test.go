// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbconstant "github.com/shing1211/hstongapi4go/gen/common/constant"
	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// readOwnSource reads a file from this package's own directory. Tests run with the
// package directory as the working directory, so this needs no path derivation and
// cannot be pointed somewhere else.
func readOwnSource(t *testing.T, name string) (string, error) {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// pbNotifyOf builds the *pbmsg.PBNotify envelope Normalize reads, so the agreement
// test can feed both decoders from one body without going near a socket.
func pbNotifyOf(t *testing.T, msgType types.NotifyMsgType, body proto.Message) *pbmsg.PBNotify {
	t.Helper()
	any, err := anypb.New(body)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &pbmsg.PBNotify{
		NotifyMsgType: pbconstant.NotifyMsgType(msgType),
		NotifyId:      "00700.HK",
		NotifyTime:    1700000000000,
		Payload:       any,
	}
}

// The tests here pin the bridge between this package's two decoders.
//
// Normalize takes a *pbmsg.PBNotify and returns (nil, nil) at its five
// empty-payload arms. NormalizeNotification takes a *Notification - the envelope
// decodeNotification already consumed - and must never do that, because its result
// crosses into a public API documented to carry an event. The five arms are the
// whole difference and they are asserted one at a time, because "five arms" is a
// count a reader can verify and "it substitutes something" is not.
//
// Nothing here marshals a frame or opens a socket: NormalizeNotification is a
// pure function over an already-decoded Notification, so the whole file runs
// offline and instantly.

// notificationPayload builds the payload a Notification of the given type would
// carry for a Hong Kong stock, by decoding it through the same Decode the read
// loop uses. Going through Decode rather than handing over a hand-built pointer
// keeps the test honest about the pairing the read loop actually produces.
func notificationPayload(t *testing.T, msgType types.NotifyMsgType, body []byte) any {
	t.Helper()
	decoded, err := Decode(msgType, body)
	if err != nil {
		t.Fatalf("Decode(%s): %v", msgType.String(), err)
	}
	return decoded
}

// mustMarshal is proto.Marshal with a test-fatal on failure.
func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal(%T): %v", m, err)
	}
	return b
}

// TestNormalizeNotificationMapsEveryDeliveredType covers the positive path for
// each type the push transport registers a handler for.
//
// The trade arm covers all three delivery types, and TrsStockDeliverMsgType is
// the one that differentiates this function from Normalize: Decode accepts type 0
// and returns a delivery message for it, so a Notification can carry it, and
// Normalize would classify it as an unknown type and hand back an UNKNOWN
// SystemEvent instead of the fill.
func TestNormalizeNotificationMapsEveryDeliveredType(t *testing.T) {
	sec := &dto.Security{DataType: 10000, Code: "00700.HK"}

	cases := []struct {
		name    string
		msgType types.NotifyMsgType
		body    proto.Message
		assert  func(*testing.T, *domain.PushEvent)
	}{
		{
			name:    "basic quote",
			msgType: types.BasicQotNotifyMsgType,
			body: &hqnotify.BasicQotNotify{
				Security: sec,
				BasicQot: &dto.BasicQot{LastPrice: 387.05, Volume: 1200, Turnover: 464460},
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				q, ok := (*ev).(domain.QuoteEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.QuoteEvent", *ev)
				}
				if got, want := q.LastPrice.String(), "387.05"; got != want {
					t.Errorf("LastPrice = %q, want %q", got, want)
				}
			},
		},
		{
			name:    "ticker",
			msgType: types.TickerNotifyMsgType,
			body: &hqnotify.TickerNotify{
				Security: sec,
				Ticker:   &dto.Ticker{Price: 387, Volume: 100, Side: 1},
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				tk, ok := (*ev).(domain.TickerEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.TickerEvent", *ev)
				}
				if got, want := tk.Price.String(), "387"; got != want {
					t.Errorf("Price = %q, want %q", got, want)
				}
				if tk.Side != types.EntrustBuy {
					t.Errorf("Side = %q, want %q", tk.Side, types.EntrustBuy)
				}
			},
		},
		{
			name:    "order book",
			msgType: types.OrderBookNotifyMsgType,
			body: &hqnotify.OrderBookFullNotify{
				Security: sec,
				Side:     0,
				OrderBookList: []*dto.OrderBook{
					{Level: 1, Price: 386.9, Volume: 100},
				},
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				ob, ok := (*ev).(domain.OrderBookEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.OrderBookEvent", *ev)
				}
				if len(ob.Bids) != 1 || ob.Depth != 1 {
					t.Errorf("bids=%d depth=%d, want 1/1", len(ob.Bids), ob.Depth)
				}
			},
		},
		{
			name:    "broker queue",
			msgType: types.BrokerQueueNotifyMsgType,
			body: &hqnotify.BrokerNotify{
				Security:   sec,
				Side:       0,
				BrokerList: []*dto.Broker{{Level: 1, Item: "3", Type: 1, Name: "Broker A"}},
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				be, ok := (*ev).(domain.BrokerEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.BrokerEvent", *ev)
				}
				if len(be.Buyers) != 1 {
					t.Fatalf("buyers = %d, want 1", len(be.Buyers))
				}
				// The wire's four fields and nothing else: a broker level that
				// carries no quantity on the wire has no field to read one from.
				want := domain.BrokerLevel{Level: 1, Item: "3", Type: 1, Name: "Broker A"}
				if be.Buyers[0] != want {
					t.Errorf("Buyers[0] = %+v, want %+v", be.Buyers[0], want)
				}
			},
		},
		{
			name:    "stock delivery",
			msgType: types.TradeStockDeliverMsgType,
			body: &tradenotify.TradeStockDeliverNotify{
				StockCode:          "00700.HK",
				EntrustBs:          "1",
				BusinessPrice:      "388.5",
				BusinessAmount:     "1000",
				SumBusinessBalance: "388500",
				MatchNo:            "M1",
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				te, ok := (*ev).(domain.TradeEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.TradeEvent", *ev)
				}
				if got, want := te.Price.String(), "388.5"; got != want {
					t.Errorf("Price = %q, want %q", got, want)
				}
				if got, want := te.Turnover.String(), "388500"; got != want {
					t.Errorf("Turnover = %q, want %q (sumBusinessBalance, not the quantity)", got, want)
				}
			},
		},
		{
			name:    "futures delivery",
			msgType: types.FuturesTradeStockDeliverMsgType,
			body: &tradenotify.TradeStockDeliverNotify{
				StockCode:          "HSI2501",
				EntrustBs:          "2",
				BusinessPrice:      "20000",
				BusinessAmount:     "1",
				SumBusinessBalance: "20000",
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				te, ok := (*ev).(domain.TradeEvent)
				if !ok {
					t.Fatalf("event = %T, want domain.TradeEvent", *ev)
				}
				if te.Side != types.EntrustSell {
					t.Errorf("Side = %q, want %q", te.Side, types.EntrustSell)
				}
			},
		},
		{
			name:    "counter delivery, the type Normalize does not cover",
			msgType: types.TrsStockDeliverMsgType,
			body: &tradenotify.TradeStockDeliverNotify{
				StockCode:          "00700.HK",
				BusinessPrice:      "388.5",
				BusinessAmount:     "100",
				SumBusinessBalance: "38850",
			},
			assert: func(t *testing.T, ev *domain.PushEvent) {
				t.Helper()
				if _, ok := (*ev).(domain.TradeEvent); !ok {
					t.Fatalf("event = %T, want domain.TradeEvent", *ev)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := &Notification{
				Type:    tc.msgType,
				ID:      "00700.HK",
				Time:    1700000000000,
				Payload: notificationPayload(t, tc.msgType, mustMarshal(t, tc.body)),
			}
			ev, err := NormalizeNotification(n)
			if err != nil {
				t.Fatalf("NormalizeNotification: %v", err)
			}
			if ev == nil {
				t.Fatal("NormalizeNotification returned a nil event with a nil error, which its contract forbids")
			}
			tc.assert(t, ev)
		})
	}
}

// TestNormalizeNotificationSubstitutesTheFiveEmptyPayloadArms is the central
// property: for each of the five payload types Normalize returns (nil, nil) for,
// this bridge delivers a domain.SystemEvent carrying EMPTY_PAYLOAD.
//
// The table drives one Notification per type with a nil Payload, which is exactly
// the shape decodeNotification produces for a frame with no Any payload. Each row
// asserts the type, the code, and the level, so a substitution that silently
// became an UNKNOWN event - normalizeUnknownEvent's own answer - would fail rather
// than pass as "some event was returned".
func TestNormalizeNotificationSubstitutesTheFiveEmptyPayloadArms(t *testing.T) {
	types5 := []types.NotifyMsgType{
		types.BasicQotNotifyMsgType,
		types.TickerNotifyMsgType,
		types.OrderBookNotifyMsgType,
		types.BrokerQueueNotifyMsgType,
		types.TradeStockDeliverMsgType,
	}
	if len(types5) != 5 {
		t.Fatalf("the fixture lists %d arms, want the 5 Normalize returns (nil, nil) for", len(types5))
	}

	for _, msgType := range types5 {
		t.Run(msgType.String(), func(t *testing.T) {
			ev, err := NormalizeNotification(&Notification{Type: msgType, ID: "00700.HK"})
			if err != nil {
				t.Fatalf("NormalizeNotification: %v", err)
			}
			assertEmptyPayloadEvent(t, ev)
		})
	}
}

// assertEmptyPayloadEvent checks the substitution: a SystemEvent whose code is
// EMPTY_PAYLOAD, at warn level, with a message.
func assertEmptyPayloadEvent(t *testing.T, ev *domain.PushEvent) {
	t.Helper()
	if ev == nil {
		t.Fatal("event = nil: the bridge must never return (nil, nil)")
	}
	sys, ok := (*ev).(domain.SystemEvent)
	if !ok {
		t.Fatalf("event = %T, want domain.SystemEvent", *ev)
	}
	if sys.Code != emptyPayloadCode {
		t.Errorf("Code = %q, want %q", sys.Code, emptyPayloadCode)
	}
	if sys.Level != "warn" {
		t.Errorf("Level = %q, want %q", sys.Level, "warn")
	}
	if sys.Message == "" {
		t.Error("Message is empty; the substitution has to say what happened")
	}
}

// TestNormalizeNotificationNeverReturnsNilNil sweeps every declared
// NotifyMsgType plus a set of values no Gateway sends, and asserts the contract
// on all of them at once.
//
// The sweep rather than a table is the point: the property is "for every input,
// never (nil, nil)", and the way to check a universal is to try the whole
// vocabulary plus the neighbourhood of it, not one row per arm. Any future type
// added to pkg/types is therefore covered the day it lands.
func TestNormalizeNotificationNeverReturnsNilNil(t *testing.T) {
	inputs := make([]*Notification, 0, 32)
	for _, msgType := range declaredNotifyMsgTypes() {
		// Both payload shapes: absent, and present but of the wrong concrete type.
		inputs = append(inputs,
			&Notification{Type: msgType, ID: "00700.HK", Time: 1700000000000},
			&Notification{Type: msgType, ID: "00700.HK", Time: 1700000000000, Payload: &hqnotify.TickerNotify{}},
		)
	}
	inputs = append(inputs,
		&Notification{Type: types.NotifyMsgType(-1), ID: "00700.HK"},
		&Notification{Type: types.NotifyMsgType(0), ID: "00700.HK", Payload: "not a message"},
		&Notification{Type: types.NotifyMsgType(99999), ID: "00700.HK"},
	)

	for i, n := range inputs {
		ev, err := NormalizeNotification(n)
		if err != nil {
			// The only permitted failure is a reported mismatch, and it must not
			// hand back a nil event with it either: a caller that only checks the
			// error still must not find a nil it might dereference.
			if ev != nil {
				t.Errorf("input %d: event = %T alongside the error %v, want nil", i, *ev, err)
			}
			continue
		}
		if ev == nil {
			t.Errorf("input %d (type %s, payload %T) = (nil, nil), which the contract forbids",
				i, n.Type.String(), n.Payload)
		}
	}
}

// declaredNotifyMsgTypes lists the values the read loop can decode: the six
// ordinary types plus the zero the wire can carry, which is a valid
// TrsStockDeliverMsgType and not a stand-in for "unset".
func declaredNotifyMsgTypes() []types.NotifyMsgType {
	return []types.NotifyMsgType{
		types.TrsStockDeliverMsgType,
		types.TradeStockDeliverMsgType,
		types.FuturesTradeStockDeliverMsgType,
		types.OrderBookNotifyMsgType,
		types.BrokerQueueNotifyMsgType,
		types.BasicQotNotifyMsgType,
		types.TickerNotifyMsgType,
	}
}

// TestNormalizeNotificationSubstitutesForATypedNilPayload covers the second half of
// the never-(nil, nil) contract.
//
// An interface holding a typed nil is not nil, so `n.Payload == nil` does not catch it
// and the type assertion succeeds with a nil message behind it. That is a shape a
// hand-built Notification can take and decodeNotification cannot produce - it either
// leaves Payload nil or hands over a pointer Decode allocated - so it is exercised
// directly. Each of the five arms has to catch it, or a typed nil reaches a decoder
// that dereferences it.
func TestNormalizeNotificationSubstitutesForATypedNilPayload(t *testing.T) {
	typedNils := []struct {
		name    string
		msgType types.NotifyMsgType
		payload any
	}{
		{name: "basic quote", msgType: types.BasicQotNotifyMsgType, payload: (*hqnotify.BasicQotNotify)(nil)},
		{name: "ticker", msgType: types.TickerNotifyMsgType, payload: (*hqnotify.TickerNotify)(nil)},
		{name: "order book", msgType: types.OrderBookNotifyMsgType, payload: (*hqnotify.OrderBookFullNotify)(nil)},
		{name: "broker queue", msgType: types.BrokerQueueNotifyMsgType, payload: (*hqnotify.BrokerNotify)(nil)},
		{name: "trade", msgType: types.TradeStockDeliverMsgType, payload: (*tradenotify.TradeStockDeliverNotify)(nil)},
	}

	for _, tc := range typedNils {
		t.Run(tc.name, func(t *testing.T) {
			if tc.payload == nil {
				t.Fatal("the fixture's typed nil is a nil interface, so it exercises the wrong arm")
			}
			ev, err := NormalizeNotification(&Notification{Type: tc.msgType, ID: "00700.HK", Payload: tc.payload})
			if err != nil {
				t.Fatalf("NormalizeNotification: %v", err)
			}
			assertEmptyPayloadEvent(t, ev)
		})
	}
}

// TestNormalizeNotificationReportsEveryMismatchedPayload covers the mismatch arm for
// all five payload types, with a payload that is wrong for each of them.
//
// The wrong payload is a real message rather than a string, so the failure is a type
// assertion failing rather than something a nil check would have caught - which is
// the shape decodeNotification cannot produce and a caller can.
func TestNormalizeNotificationReportsEveryMismatchedPayload(t *testing.T) {
	cases := []struct {
		name    string
		msgType types.NotifyMsgType
		payload any
	}{
		{name: "basic quote announced as a ticker", msgType: types.BasicQotNotifyMsgType, payload: &hqnotify.TickerNotify{}},
		{name: "ticker announced as a basic quote", msgType: types.TickerNotifyMsgType, payload: &hqnotify.BasicQotNotify{}},
		{name: "order book announced as a ticker", msgType: types.OrderBookNotifyMsgType, payload: &hqnotify.TickerNotify{}},
		{name: "broker queue announced as a ticker", msgType: types.BrokerQueueNotifyMsgType, payload: &hqnotify.TickerNotify{}},
		{name: "trade announced as a basic quote", msgType: types.TradeStockDeliverMsgType, payload: &hqnotify.BasicQotNotify{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := NormalizeNotification(&Notification{Type: tc.msgType, ID: "00700.HK", Payload: tc.payload})
			if err == nil {
				t.Fatal("a mismatched payload = nil error, want one")
			}
			if ev != nil {
				t.Errorf("event = %T alongside the error, want nil", *ev)
			}
		})
	}
}

// TestNormalizeNotificationRejectsUnclassifiableInputs covers the two failures
// that are wiring errors rather than observed conditions.
//
// Both must be reported rather than substituted. A Notification whose Payload is
// not the message its Type selects cannot have come off the wire - decodeNotification
// drives Decode from the same Type - so handing back a SystemEvent would dress a
// bug as a diagnosis, and a nil Notification is a nil dereference one line later.
func TestNormalizeNotificationRejectsUnclassifiableInputs(t *testing.T) {
	t.Run("nil notification", func(t *testing.T) {
		ev, err := NormalizeNotification(nil)
		if err == nil {
			t.Fatal("NormalizeNotification(nil) = nil error, want one")
		}
		if ev != nil {
			t.Errorf("event = %T alongside the error, want nil", *ev)
		}
		if !strings.Contains(err.Error(), "nil Notification") {
			t.Errorf("error = %q, want it to name the nil Notification", err)
		}
	})

	t.Run("payload does not match the type", func(t *testing.T) {
		// A ticker payload announced as a basic quote. Named on both sides,
		// because either half alone leaves a reader guessing which one moved.
		n := &Notification{
			Type:    types.BasicQotNotifyMsgType,
			ID:      "00700.HK",
			Payload: &hqnotify.TickerNotify{},
		}
		ev, err := NormalizeNotification(n)
		if err == nil {
			t.Fatal("a mismatched payload = nil error, want one")
		}
		if ev != nil {
			t.Errorf("event = %T alongside the error, want nil", *ev)
		}
		for _, want := range []string{"*notify.TickerNotify", "BasicQotNotifyMsgType"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to name %q", err, want)
			}
		}
	})
}

// TestNormalizeNotificationUnknownTypeMatchesNormalize pins the default arm
// against Normalize's own default, so the two decoders cannot answer differently
// for the same unrecognised discriminator.
func TestNormalizeNotificationUnknownTypeMatchesNormalize(t *testing.T) {
	ev, err := NormalizeNotification(&Notification{Type: types.NotifyMsgType(99999), ID: "00700.HK"})
	if err != nil {
		t.Fatalf("NormalizeNotification: %v", err)
	}
	if ev == nil {
		t.Fatal("event = nil, want normalizeUnknownEvent's answer")
	}
	sys, ok := (*ev).(domain.SystemEvent)
	if !ok {
		t.Fatalf("event = %T, want domain.SystemEvent", *ev)
	}
	if sys.Code != "UNKNOWN" {
		t.Errorf("Code = %q, want %q (what normalizeUnknownEvent yields for a nil payload)", sys.Code, "UNKNOWN")
	}
}

// TestNormalizeNotificationAndNormalizeAgreeOnTheDeliveredTypes runs both
// decoders over the same frames and requires the same event out of both.
//
// The two differ by construction on the nil payload - Normalize returns (nil, nil)
// and this bridge substitutes - so the comparison is over the non-empty payloads,
// where they must not differ. That is what makes this a reuse of Normalize's five
// decoders rather than a second implementation of them.
func TestNormalizeNotificationAndNormalizeAgreeOnTheDeliveredTypes(t *testing.T) {
	sec := &dto.Security{DataType: 10000, Code: "00700.HK"}
	bodies := map[types.NotifyMsgType]proto.Message{
		types.BasicQotNotifyMsgType: &hqnotify.BasicQotNotify{
			Security: sec,
			BasicQot: &dto.BasicQot{LastPrice: 1.5, OpenPrice: 2, HighPrice: 3, LowPrice: 0.5, LastClosePrice: 1},
		},
		types.TickerNotifyMsgType: &hqnotify.TickerNotify{
			Security: sec,
			Ticker:   &dto.Ticker{Price: 9.25, Volume: 7, Turnover: 64.75, Side: 3},
		},
		types.OrderBookNotifyMsgType: &hqnotify.OrderBookFullNotify{
			Security:      sec,
			Side:          1,
			OrderBookList: []*dto.OrderBook{{Level: 2, Price: 4.5, Volume: 11}},
		},
		types.BrokerQueueNotifyMsgType: &hqnotify.BrokerNotify{
			Security:   sec,
			Side:       1,
			BrokerList: []*dto.Broker{{Level: 3, Item: "9", Type: 2, Name: "N"}},
		},
		types.TradeStockDeliverMsgType: &tradenotify.TradeStockDeliverNotify{
			StockCode:          "00700.HK",
			EntrustBs:          "1",
			BusinessPrice:      "388.5",
			BusinessAmount:     "100",
			SumBusinessBalance: "38850",
			MatchNo:            "M7",
			EntrustStatus:      "8",
		},
	}

	for msgType, body := range bodies {
		t.Run(msgType.String(), func(t *testing.T) {
			raw := mustMarshal(t, body)

			want, err := Normalize(pbNotifyOf(t, msgType, body))
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if want == nil {
				t.Fatal("Normalize = (nil, nil) over a non-empty payload, which would make this comparison vacuous")
			}

			got, err := NormalizeNotification(&Notification{
				Type:    msgType,
				ID:      "00700.HK",
				Time:    1700000000000,
				Payload: notificationPayload(t, msgType, raw),
			})
			if err != nil {
				t.Fatalf("NormalizeNotification: %v", err)
			}
			if got == nil {
				t.Fatal("NormalizeNotification = nil event")
			}
			if !reflect.DeepEqual(*got, *want) {
				t.Errorf("NormalizeNotification produced %+v, Normalize produced %+v; the bridge must reuse the decoders, not reimplement them",
					*got, *want)
			}
		})
	}
}

// TestNormalizeNotificationSurvivesAnEmptyDeliveryFrame is the regression guard
// for the panic the raw decoders would have taken on the path this bridge makes
// reachable.
//
// A protobuf string field the Gateway leaves unset decodes to "", and
// domain.MustNewPrice panics on an unparseable value - so a delivery frame with no
// businessPrice would have panicked inside a push dispatcher goroutine, which no
// caller can recover from. The substitution is a documented zero rather than a
// panic, and it is a zero only for the empty case: a malformed number is still
// rejected rather than turned into one.
func TestNormalizeNotificationSurvivesAnEmptyDeliveryFrame(t *testing.T) {
	ev, err := NormalizeNotification(&Notification{
		Type: types.TradeStockDeliverMsgType,
		ID:   "00700.HK",
		Payload: notificationPayload(t, types.TradeStockDeliverMsgType,
			mustMarshal(t, &tradenotify.TradeStockDeliverNotify{StockCode: "00700.HK"})),
	})
	if err != nil {
		t.Fatalf("NormalizeNotification: %v", err)
	}
	te, ok := (*ev).(domain.TradeEvent)
	if !ok {
		t.Fatalf("event = %T, want domain.TradeEvent", *ev)
	}
	if !te.Price.IsZero() || !te.Quantity.IsZero() || !te.Turnover.IsZero() {
		t.Errorf("price=%v quantity=%v turnover=%v, want zero substitutes for the three absent fields",
			te.Price, te.Quantity, te.Turnover)
	}
	// The market and the code still come through: an absent decimal field is not
	// a reason to blank the rest of the event.
	if te.Symbol.Market != domain.MarketHK {
		t.Errorf("Symbol.Market = %q, want %q", te.Symbol.Market, domain.MarketHK)
	}
}

// TestPushSymbolDoesNotDoubleTheMarketSuffix pins the symbol fix.
//
// domain.NewSymbol appends a market to the code it is given, and
// domain.SymbolFromSecurity derives that market from the code - so for a Gateway
// code that already carries its suffix the pair yields FullCode "00700.HK.HK".
// Push routing does not read FullCode, but FullCode is on a public type now, and a
// doubled suffix is a string a caller would read as an instrument.
func TestPushSymbolDoesNotDoubleTheMarketSuffix(t *testing.T) {
	cases := []struct {
		code         string
		wantMarket   domain.Market
		wantFullCode string
	}{
		{code: "00700.HK", wantMarket: domain.MarketHK, wantFullCode: "00700.HK"},
		{code: "AAPL.US", wantMarket: domain.MarketUS, wantFullCode: "AAPL.US"},
		{code: "000001.SZ", wantMarket: domain.MarketShenzhenConnect, wantFullCode: "000001.SZ"},
		{code: "600000.SH", wantMarket: domain.MarketShanghaiConnect, wantFullCode: "600000.SH"},
		{code: "AAPL", wantMarket: "", wantFullCode: "AAPL"},
		{code: "", wantMarket: "", wantFullCode: ""},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := pushSymbol(tc.code, 10000)
			if got.Market != tc.wantMarket {
				t.Errorf("Market = %q, want %q", got.Market, tc.wantMarket)
			}
			if got.FullCode != tc.wantFullCode {
				t.Errorf("FullCode = %q, want %q", got.FullCode, tc.wantFullCode)
			}
			// The routing key is the wire code, verbatim: the routing index
			// normalises it rather than trusting a rebuilt one.
			if got.Code != tc.code {
				t.Errorf("Code = %q, want the wire code %q verbatim", got.Code, tc.code)
			}
		})
	}
}

// TestPushSymbolFromSecurityToleratesAnAbsentSecurity covers the nil case the
// pull path also tolerates: a frame with no security yields the zero Symbol rather
// than a panic.
func TestPushSymbolFromSecurityToleratesAnAbsentSecurity(t *testing.T) {
	if got := pushSymbolFromSecurity(nil); !got.IsZero() {
		t.Errorf("pushSymbolFromSecurity(nil) = %+v, want the zero Symbol", got)
	}
}

// TestDecodedPricesCarryNoTick is the static half of the tick precondition.
//
// It reads decode.go as text and rejects any Price built with a non-zero tick.
// Reading the source rather than asserting on a value is what makes this a guard
// rather than a snapshot: a decoder that reintroduced a hardcoded tick would produce
// a Price whose Round or Validate happens to match the fixture, and only the
// literal itself betrays it.
//
// It also pins the other half of the precondition, so neither half can be "fixed"
// by reverting the other: the wire-derived double is still rendered with
// FormatFloat's shortest round-tripping form, which is a property of the protobuf
// field and not something this package may change.
func TestDecodedPricesCarryNoTick(t *testing.T) {
	source, err := readOwnSource(t, "decode.go")
	if err != nil {
		t.Fatalf("reading decode.go: %v", err)
	}
	checked := 0
	for i, line := range strings.Split(source, "\n") {
		if !strings.Contains(line, "MustNewPrice(") {
			continue
		}
		checked++
		if !strings.Contains(line, "wireTick") {
			t.Errorf("decode.go:%d builds a Price with a literal tick: %s\n"+
				"A push price is the most wire-derived number in the SDK; the Gateway sent the "+
				"digits and this package has no instrument master, so the tick must be %q "+
				"(design-tick-model.md 2.1)", i+1, strings.TrimSpace(line), wireTick)
		}
	}
	if wireTick != "0" {
		t.Errorf("wireTick = %q, want %q: a non-zero tick states as fact something the SDK invented", wireTick, "0")
	}
	if checked < 8 {
		t.Errorf("found %d MustNewPrice call sites in decode.go, want the 8 the precondition names; "+
			"the fixture is not looking at the code it thinks it is", checked)
	}
	if !strings.Contains(source, "strconv.FormatFloat(") {
		t.Error("decode.go no longer renders wire doubles with strconv.FormatFloat; the shortest " +
			"round-tripping decimal for a protobuf double is a property of the wire type and " +
			"must not be replaced by a fixed scale")
	}
}

// TestDecodedTickCarriesNoTick is the value half of the same precondition.
//
// A tick of zero is not merely a smaller number: Price.Validate skips its modulus
// check and Price.Round returns the value unchanged, which is exactly what a
// wire-derived price wants and exactly what an invented 0.001 would break - a
// legitimate 0.0005 would be rejected by Validate and rounded to 0.000 by Round.
func TestDecodedTickCarriesNoTick(t *testing.T) {
	ev, err := NormalizeNotification(&Notification{
		Type: types.TickerNotifyMsgType,
		ID:   "00700.HK",
		Payload: notificationPayload(t, types.TickerNotifyMsgType, mustMarshal(t, &hqnotify.TickerNotify{
			Security: &dto.Security{DataType: 10000, Code: "00700.HK"},
			Ticker:   &dto.Ticker{Price: 0.0005},
		})),
	})
	if err != nil {
		t.Fatalf("NormalizeNotification: %v", err)
	}
	tk, ok := (*ev).(domain.TickerEvent)
	if !ok {
		t.Fatalf("event = %T, want domain.TickerEvent", *ev)
	}
	if !tk.Price.Tick().IsZero() {
		t.Errorf("Tick = %s, want zero: the SDK has no instrument master for this instrument",
			tk.Price.Tick())
	}
	if got, want := tk.Price.String(), "0.0005"; got != want {
		t.Errorf("Price = %q, want the wire value %q verbatim", got, want)
	}
	if err := tk.Price.Validate(true); err != nil {
		t.Errorf("Validate(true) on a wire price = %v, want nil: with a zero tick the modulus check "+
			"is skipped, which is what keeps a legitimate 0.0005 from being rejected", err)
	}
	if got, want := tk.Price.Round().String(), "0.0005"; got != want {
		t.Errorf("Round() = %q, want %q unchanged with a zero tick", got, want)
	}
}

// TestNormalizeNotificationIsReachableOnlyThroughTheBridge records the reachability
// change this bridge is: before it, Normalize had no production caller, so the tick
// literals and the fabricated broker quantity in this package were unreachable and
// therefore harmless.
func TestNormalizeNotificationIsReachableOnlyThroughTheBridge(t *testing.T) {
	// The substitution code is a single constant, so "five arms" is a count a
	// reader can verify and the constant cannot drift into five spellings.
	if emptyPayloadCode != "EMPTY_PAYLOAD" {
		t.Errorf("emptyPayloadCode = %q, want %q", emptyPayloadCode, "EMPTY_PAYLOAD")
	}
}
