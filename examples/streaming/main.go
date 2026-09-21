// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command streaming connects the Gateway TCP push channel, subscribes one
// market topic over HTTP, and consumes decoded push events until the context is
// cancelled (Ctrl-C).
//
// Run the local Gateway (or the in-repo mock Gateway) first, then:
//
//	go run ./examples/streaming
//
// The optional HSTONG_EXAMPLE_SECURITY variable selects the security and
// defaults to 0700.HK.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong/stream"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatalf("client.New: %v", err)
	}
	defer c.Close()

	s := stream.New(c)
	if err := s.Connect(ctx); err != nil {
		log.Fatalf("stream.Connect: %v", err)
	}
	defer s.Close()

	code := envOr("HSTONG_EXAMPLE_SECURITY", "0700.HK")
	sub, err := s.Subscribe(ctx, types.TopicBasicQot,
		&dto.Security{DataType: int32(types.DataTypeHKStock), Code: code})
	if err != nil {
		log.Fatalf("stream.Subscribe: %v", err)
	}
	defer func() {
		if err := sub.Cancel(context.Background()); err != nil {
			log.Printf("subscription cancel: %v", err)
		}
	}()
	fmt.Printf("subscribed to %s; press Ctrl-C to stop\n", code)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("stopping")
			return
		case ev, ok := <-sub.Updates():
			if !ok {
				return
			}
			if q, ok := ev.BasicQot(); ok {
				bq := q.GetBasicQot()
				fmt.Printf("%s %s last=%v volume=%d\n",
					ev.Time.Format("15:04:05"), ev.ID, bq.GetLastPrice(), bq.GetVolume())
				continue
			}
			fmt.Printf("%s event type=%s id=%s\n", ev.Time.Format("15:04:05"), ev.Type, ev.ID)
		case err := <-sub.Errors():
			if err != nil {
				log.Printf("stream error: %v", err)
			}
		}
	}
}
