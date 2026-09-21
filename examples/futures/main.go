// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command futures exercises the futures surface: account funds, positions, an
// optional order placement, and today's futures orders.
//
// Order placement is a mutation and is skipped unless HSTONG_EXAMPLE_PLACE_ORDER
// is a true value ("1", "true").
//
// Run the local Gateway (or the in-repo mock Gateway) first, then:
//
//	HSTONG_TRADE_PASSWORD=... go run ./examples/futures
//
// Optional variables: HSTONG_EXAMPLE_FUTURES_CODE (default HSI2603),
// HSTONG_EXAMPLE_PRICE (default 20000), HSTONG_EXAMPLE_AMOUNT (default 1).
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/future"
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

	m := future.New(c)
	code := envOr("HSTONG_EXAMPLE_FUTURES_CODE", "HSI2603")
	price := envOr("HSTONG_EXAMPLE_PRICE", "20000")
	amount := envOr("HSTONG_EXAMPLE_AMOUNT", "1")

	funds, err := m.QueryFundInfo(ctx)
	if err != nil {
		log.Fatalf("QueryFundInfo: %v", err)
	}
	log.Printf("futures funds: assetBalance=%s enableBalance=%s marginStatus=%s",
		funds.FundInfo.AssetBalance, funds.FundInfo.EnableBalance, funds.FundInfo.MarginStatus)

	holds, err := m.QueryHoldsList(ctx)
	if err != nil {
		log.Fatalf("QueryHoldsList: %v", err)
	}
	log.Printf("futures positions: %d", len(holds.HoldsList))
	for _, h := range holds.HoldsList {
		log.Printf("  %s %s currentQty=%s profitLoss=%s", h.StockCode, h.StockName, h.CurrentQty, h.ProfitLoss)
	}

	if place, _ := strconv.ParseBool(envOr("HSTONG_EXAMPLE_PLACE_ORDER", "false")); place {
		resp, err := m.Entrust(ctx, future.EntrustRequest{
			StockCode:     code,
			EntrustType:   "0",
			EntrustPrice:  price,
			EntrustAmount: amount,
			EntrustBS:     string(types.EntrustBuy),
			ValidTimeType: "0",
		})
		if err != nil {
			log.Fatalf("FuturesEntrust: %v", err)
		}
		log.Printf("placed futures order %s: %s x%s @ %s", resp.Data, code, amount, price)
	} else {
		log.Println("futures order placement skipped (set HSTONG_EXAMPLE_PLACE_ORDER=1 to place one)")
	}

	orders, err := m.QueryRealEntrustList(ctx)
	if err != nil {
		log.Fatalf("QueryRealEntrustList: %v", err)
	}
	log.Printf("today's futures orders: %d", len(orders.Data))
	for _, o := range orders.Data {
		log.Printf("  %s %s %s status=%s", o.EntrustID, o.StockCode, o.EntrustAmount, o.StatusDesc)
	}
}
