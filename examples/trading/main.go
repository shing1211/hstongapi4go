// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command trading exercises the stock trading surface: funds, positions, an
// optional order placement, today's open orders, and an optional cancel.
//
// Order placement and cancellation are mutations. They are skipped unless
// HSTONG_EXAMPLE_PLACE_ORDER is a true value ("1", "true"), so the example is
// safe to run against a funded account by accident-free default.
//
// Run the local Gateway (or the in-repo mock Gateway) first, then:
//
//	HSTONG_TRADE_PASSWORD=... go run ./examples/trading
//
// Optional variables: HSTONG_EXAMPLE_SECURITY (default 0700.HK),
// HSTONG_EXAMPLE_PRICE (default 300), HSTONG_EXAMPLE_AMOUNT (default 100).
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
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
	defer c.Close()

	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login (set HSTONG_TRADE_PASSWORD): %v", err)
	}
	defer func() {
		if err := session.Logout(ctx); err != nil {
			log.Printf("trade logout: %v", err)
		}
	}()

	m := trade.New(c, trade.WithSession(session))
	code := envOr("HSTONG_EXAMPLE_SECURITY", "0700.HK")
	price := envOr("HSTONG_EXAMPLE_PRICE", "300")
	amount := envOr("HSTONG_EXAMPLE_AMOUNT", "100")

	funds, err := m.MarginFundInfo(ctx, trade.MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
	if err != nil {
		log.Fatalf("MarginFundInfo: %v", err)
	}
	log.Printf("funds: assetBalance=%s enableBalance=%s", funds.AssetBalance, funds.EnableBalance)

	positions, err := m.Positions(ctx, trade.PositionsRequest{ExchangeType: types.ExchangeHK})
	if err != nil {
		log.Fatalf("Positions: %v", err)
	}
	log.Printf("positions: %d", len(positions))
	for _, p := range positions {
		log.Printf("  %s %s amount=%s", p.StockCode, p.StockName, p.CurrentAmount)
	}

	var placedID string
	if place, _ := strconv.ParseBool(envOr("HSTONG_EXAMPLE_PLACE_ORDER", "false")); place {
		id, err := m.Entrust(ctx, trade.EntrustRequest{
			ExchangeType:  types.ExchangeHK,
			StockCode:     code,
			EntrustAmount: amount,
			EntrustPrice:  price,
			EntrustBS:     types.EntrustBuy,
			EntrustType:   types.EntrustTypeLimit,
		})
		if err != nil {
			log.Fatalf("Entrust: %v", err)
		}
		placedID = id
		log.Printf("placed order %s: %s x%s @ %s", id, code, amount, price)
	} else {
		log.Println("order placement skipped (set HSTONG_EXAMPLE_PLACE_ORDER=1 to place one)")
	}

	orders, err := m.RealEntrustList(ctx, trade.RealEntrustListRequest{
		ExchangeType: types.ExchangeHK,
		QueryCount:   20,
	})
	if err != nil {
		log.Fatalf("RealEntrustList: %v", err)
	}
	log.Printf("open orders: %d", len(orders))
	for _, o := range orders {
		log.Printf("  %s %s %s status=%s cancelable=%d",
			o.EntrustID, o.StockCode, o.EntrustAmount, o.StatusDesc, o.CanBeCanceled)
	}

	if placedID != "" {
		cancelled, err := m.CancelEntrust(ctx, trade.CancelEntrustRequest{
			ExchangeType:  types.ExchangeHK,
			StockCode:     code,
			EntrustAmount: amount,
			EntrustPrice:  price,
			EntrustID:     placedID,
			EntrustType:   types.EntrustTypeLimit,
		})
		if err != nil {
			log.Printf("CancelEntrust %s: %v", placedID, err)
		} else {
			log.Printf("cancelled order %s (result %s)", placedID, cancelled)
		}
	}
}
