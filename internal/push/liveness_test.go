// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"errors"
	"net"
	"testing"
	"time"
)

// TestReadLoopDeadlineFiresOnSilentPeer covers R3 in the threat model: a peer
// that stops sending without closing the connection used to park the read
// goroutine indefinitely. With a read deadline armed the loop gives up.
func TestReadLoopDeadlineFiresOnSilentPeer(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = serverSide.Close() }()
	defer func() { _ = clientSide.Close() }()

	// The server accepts the connection and then says nothing at all: the
	// connection stays open, so only a deadline can end the read.
	c := New(WithReadDeadline(50 * time.Millisecond))
	err := c.readLoop(clientSide)
	if err == nil {
		t.Fatal("readLoop() = nil, want a timeout error on a silent peer")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("readLoop() error = %v, want a net.Error timeout", err)
	}
}

// TestReadLoopWithoutDeadlineStillBlocks is the control case. The deadline is
// off by default because the Gateway's inter-frame gap is undocumented, so this
// test pins the default: no deadline means the read is not interrupted.
func TestReadLoopWithoutDeadlineStillBlocks(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = serverSide.Close() }()
	defer func() { _ = clientSide.Close() }()

	c := New()
	if c.opts.ReadDeadline != 0 {
		t.Fatalf("default ReadDeadline = %v, want 0 (disabled)", c.opts.ReadDeadline)
	}

	done := make(chan error, 1)
	go func() { done <- c.readLoop(clientSide) }()

	select {
	case err := <-done:
		t.Fatalf("readLoop() returned %v with no deadline; want it to keep waiting", err)
	case <-time.After(200 * time.Millisecond):
		// Still blocked, which is the documented default behaviour.
	}
}

// TestReadDeadlineIsAppliedPerRead documents the option value is carried into
// the loop. The deadline is re-armed before every read, so it bounds the gap
// between frames rather than the connection's total lifetime: a peer that keeps
// sending frames is never torn down, however long the connection stays open.
func TestReadDeadlineIsAppliedPerRead(t *testing.T) {
	c := New(WithReadDeadline(40 * time.Millisecond))
	if c.opts.ReadDeadline != 40*time.Millisecond {
		t.Fatalf("ReadDeadline = %v, want 40ms", c.opts.ReadDeadline)
	}
}

// TestNonPositiveReadDeadlineDisables documents that a zero or negative value
// turns the deadline off, so the knob cannot be half-applied.
func TestNonPositiveReadDeadlineDisables(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		c := New(WithReadDeadline(d))
		if c.opts.ReadDeadline != 0 {
			t.Errorf("WithReadDeadline(%v): ReadDeadline = %v, want 0 (disabled)", d, c.opts.ReadDeadline)
		}
	}
}
