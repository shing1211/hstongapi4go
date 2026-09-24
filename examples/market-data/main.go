// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command market-data exercises the nine market-data pull endpoints of the
// HStong OpenAPI Gateway: quote, order book, K-line, time-share, and ticker.
//
// Run the local Gateway (or the in-repo mock Gateway) first, then:
//
//	go run ./examples/market-data
//
// The optional HSTONG_EXAMPLE_SECURITY variable selects the security and
// defaults to 0700.HK.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatalf("client.New: %v", err)
	}
	defer func() { _ = c.Close() }()

	code := envOr("HSTONG_EXAMPLE_SECURITY", "0700.HK")
	sec := &dto.Security{DataType: int32(types.DataTypeHKStock), Code: code}
	m := market.New(c)

	quote, err := m.BasicQot(ctx, market.BasicQotRequest{
		Security:  []*dto.Security{sec},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatalf("BasicQot: %v", err)
	}
	log.Printf("BasicQot: %d quote(s)", len(quote.BasicQot))

	book, err := m.OrderBook(ctx, market.OrderBookRequest{
		Security:  sec,
		MktTmType: 1,
	})
	if err != nil {
		log.Fatalf("OrderBook: %v", err)
	}
	log.Printf("OrderBook: %d ask / %d bid level(s)",
		len(book.OrderBookAskList), len(book.OrderBookBidList))

	start := time.Now().AddDate(0, 0, -30).Format("20060102")
	startDate, err := strconv.ParseInt(start, 10, 64)
	if err != nil {
		log.Fatalf("parse start date: %v", err)
	}
	kl, err := m.KL(ctx, market.KLRequest{
		Security:    sec,
		StartDate:   startDate,
		Direction:   0,
		ExRightFlag: 1,
		CycType:     2,
		Limit:       10,
	})
	if err != nil {
		log.Fatalf("KL: %v", err)
	}
	log.Printf("KL: %d candle(s) since %s", len(kl.Kline), start)

	ticks, err := m.Ticker(ctx, market.TickerRequest{
		Security:  sec,
		Limit:     10,
		MktTmType: 1,
	})
	if err != nil {
		log.Fatalf("Ticker: %v", err)
	}
	log.Printf("Ticker: %d tick(s)", len(ticks.Ticker))

	ts, err := m.TimeShare(ctx, market.TimeShareRequest{
		Security:  sec,
		MktTmType: 1,
	})
	if err != nil {
		log.Fatalf("TimeShare: %v", err)
	}
	log.Printf("TimeShare: %d point(s)", len(ts.TimeShare))
}
