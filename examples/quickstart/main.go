// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command quickstart is the smallest end-to-end hstongapi4go program. It reads
// the HSTONG_* environment, builds a client, logs in to the trade session,
// requests one market quote, and lists today's real orders.
//
// Start the local HStong Gateway (or the in-repo mock Gateway) first, then run:
//
//	HSTONG_TRADE_PASSWORD=... go run ./examples/quickstart
//
// Configuration comes from the same HSTONG_* variables as the client package.
// The optional HSTONG_EXAMPLE_SECURITY variable selects the quoted security and
// defaults to 0700.HK.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
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
	defer func() { _ = c.Close() }()

	fmt.Printf("gateway http: %s\n", c.BaseURL())
	fmt.Printf("gateway push: %s\n", c.PushAddr())

	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Printf("trade login failed (set HSTONG_TRADE_PASSWORD to log in): %v", err)
	} else {
		fmt.Println("trade login: ok")
	}

	code := envOr("HSTONG_EXAMPLE_SECURITY", "0700.HK")
	sec := &dto.Security{DataType: int32(types.DataTypeHKStock), Code: code}

	marketMgr := market.New(c)
	quote, err := marketMgr.BasicQot(ctx, market.BasicQotRequest{
		Security:  []*dto.Security{sec},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatalf("market.BasicQot: %v", err)
	}
	for _, q := range quote.BasicQot {
		fmt.Printf("quote %s: last=%v volume=%d\n",
			q.GetSecurity().GetCode(), q.GetLastPrice(), q.GetVolume())
	}

	tradeMgr := trade.New(c, trade.WithSession(session))
	orders, err := tradeMgr.RealEntrustList(ctx, trade.RealEntrustListRequest{
		ExchangeType: types.ExchangeHK,
		QueryCount:   20,
	})
	if err != nil {
		log.Fatalf("trade.RealEntrustList: %v", err)
	}
	fmt.Printf("today's real orders: %d\n", len(orders))
	for _, o := range orders {
		fmt.Printf("  %s %s %s status=%s\n",
			o.EntrustID, o.StockCode, o.EntrustAmount, o.StatusDesc)
	}
}
