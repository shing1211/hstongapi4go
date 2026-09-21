// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Command hstong-mock-gateway runs the in-repo mock HStong OpenAPI Gateway as a
// standalone process so the SDK can be exercised without the real Gateway.
//
// It binds the HTTP surface on -http (default 127.0.0.1:11111) and the TCP push
// surface on -push (default 127.0.0.1:11112), prints the bound addresses, and
// serves until interrupted with Ctrl-C (SIGINT) or SIGTERM.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/shing1211/hstongapi4go/test/mockgateway"
)

func main() {
	httpAddr := flag.String("http", "127.0.0.1:11111", "HTTP listen address for the mock Gateway")
	pushAddr := flag.String("push", "127.0.0.1:11112", "TCP push listen address for the mock Gateway")
	flag.Parse()

	srv := mockgateway.New(
		mockgateway.WithHTTPAddr(*httpAddr),
		mockgateway.WithPushAddr(*pushAddr),
	)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "hstong-mock-gateway: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("hstong-mock-gateway: HTTP  %s\n", srv.HTTPBaseURL())
	fmt.Printf("hstong-mock-gateway: push  %s\n", srv.PushAddr())
	fmt.Println("hstong-mock-gateway: serving 51 endpoints; press Ctrl-C to stop")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	_ = srv.Close()
	fmt.Println("hstong-mock-gateway: stopped")
}
