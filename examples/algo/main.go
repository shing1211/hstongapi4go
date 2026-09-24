// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command algo exercises the algorithm-trading surface: an optional master
// order placement, a master-order query, a child-entrust query, and an optional
// action (start/stop/suspend/resume).
//
// The mutations (AddOrder and ActionOrder) are skipped unless
// HSTONG_EXAMPLE_PLACE_ORDER is a true value ("1", "true"). To query an
// existing master, set HSTONG_EXAMPLE_ALGO_ORDER_ID.
//
// Run the local Gateway (or the in-repo mock Gateway) first, then:
//
//	HSTONG_TRADE_PASSWORD=... go run ./examples/algo
//
// Optional variables: HSTONG_EXAMPLE_SECURITY (default 0700.HK),
// HSTONG_EXAMPLE_PRICE (default 300), HSTONG_EXAMPLE_AMOUNT (default 1000).
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/algo"
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

	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login (set HSTONG_TRADE_PASSWORD): %v", err)
	}
	defer func() {
		if err := session.Logout(ctx); err != nil {
			log.Printf("trade logout: %v", err)
		}
	}()

	m := algo.New(c, algo.WithDefaultExchangeType(types.ExchangeHK))
	code := envOr("HSTONG_EXAMPLE_SECURITY", "0700.HK")
	price := envOr("HSTONG_EXAMPLE_PRICE", "300")
	amount := envOr("HSTONG_EXAMPLE_AMOUNT", "1000")
	orderID := os.Getenv("HSTONG_EXAMPLE_ALGO_ORDER_ID")

	if place, _ := strconv.ParseBool(envOr("HSTONG_EXAMPLE_PLACE_ORDER", "false")); place {
		id, err := m.AddOrder(ctx, algo.AddOrderParams{
			StockCode:      code,
			EntrustType:    algo.EntrustTypeLimit,
			EntrustPrice:   price,
			EntrustAmount:  amount,
			EntrustBS:      types.EntrustBuy,
			TargetStrategy: algo.StrategyVWAP,
			SessionType:    algo.SessionTypeOff,
			StrategyParam: algo.StrategyParam{
				MaxVolume:   "100",
				Sensitivity: algo.SensitivityNeutral,
			},
		})
		if err != nil {
			log.Fatalf("AlgoAddOrder: %v", err)
		}
		orderID = id
		log.Printf("placed algo master order %s", id)
	} else {
		log.Println("algo order placement skipped (set HSTONG_EXAMPLE_PLACE_ORDER=1 to place one)")
	}

	start := time.Now().AddDate(0, 0, -7).Format("20060102")
	end := time.Now().Format("20060102")
	masters, err := m.QueryOrderList(ctx, algo.QueryOrderListParams{
		PageNo:    "1",
		PageSize:  "20",
		StartDate: start,
		EndDate:   end,
	})
	if err != nil {
		log.Fatalf("AlgoQueryOrderList: %v", err)
	}
	log.Printf("master orders: %d", len(masters))
	for _, o := range masters {
		log.Printf("  %s %s %s status=%s strategy=%s",
			o.OrderID, o.StockCode, o.EntrustAmount, o.Status, o.TargetStrategy)
	}

	if orderID == "" {
		log.Println("child-entrust query skipped (set HSTONG_EXAMPLE_ALGO_ORDER_ID or place an order)")
		return
	}

	entrustIDs, err := m.QueryEntrustIDList(ctx, algo.QueryEntrustIDListParams{
		OrderID:   orderID,
		TradeDate: end,
	})
	if err != nil {
		log.Fatalf("AlgoQueryEntrustIdList: %v", err)
	}
	log.Printf("child entrusts of %s: %d", orderID, len(entrustIDs))

	if stop, _ := strconv.ParseBool(envOr("HSTONG_EXAMPLE_PLACE_ORDER", "false")); stop {
		id, err := m.ActionOrder(ctx, algo.ActionOrderParams{
			OrderID:        orderID,
			Action:         algo.ActionStop,
			TargetStrategy: algo.StrategyVWAP,
		})
		if err != nil {
			log.Printf("AlgoActionOrder stop %s: %v", orderID, err)
		} else {
			log.Printf("stopped algo master order %s", id)
		}
	}
}
