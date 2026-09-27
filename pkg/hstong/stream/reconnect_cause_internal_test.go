// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests in this file pin the reconnect notice as a wrap, which is what the
// ErrReconnected doc comment promises. The notice used to be
// fmt.Errorf("%w: %v", ErrReconnected, cause), whose single %w bound to the
// sentinel: the cause was readable but unmatchable, and errors.Unwrap returned
// the sentinel rather than the cause. So every assertion here goes through
// errors.Is, errors.As or errors.Unwrap, never through the message, and the
// message is checked only for continuity with what the old format produced.
//
// Nothing here sleeps, dials, or contacts a Gateway. onReconnect is called
// directly and the notice is read from the subscription's buffered channel, so
// the whole file runs off a scripted httptest server or not at all.

// TestReconnectNoticeWrapsCause covers the properties the doc comment promises,
// for a cause the caller can match with the standard library alone — a plain
// io sentinel, which is what a Gateway hangup surfaces as.
func TestReconnectNoticeWrapsCause(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	notice := newReconnectNotice(cause)

	t.Run("matches the sentinel", func(t *testing.T) {
		if !errors.Is(notice, ErrReconnected) {
			t.Errorf("errors.Is(notice, ErrReconnected) = false, want true: %v", notice)
		}
		if errors.Is(notice, cause) && cause == ErrReconnected {
			t.Fatal("the test cause must differ from the sentinel")
		}
	})

	t.Run("unwraps to the cause, not the sentinel", func(t *testing.T) {
		// This is the assertion that distinguishes the notice type from
		// fmt.Errorf("%w: %w", ...): a multi-%w value has Unwrap() []error,
		// which errors.Unwrap does not understand, so it returns nil there and
		// the documented mechanism silently stops working.
		if got := errors.Unwrap(notice); got != cause {
			t.Errorf("errors.Unwrap(notice) = %v, want the cause %v", got, cause)
		}
		if got := errors.Unwrap(notice); got == ErrReconnected {
			t.Error("errors.Unwrap(notice) = ErrReconnected, want the cause")
		}
	})

	t.Run("the cause is matchable", func(t *testing.T) {
		if !errors.Is(notice, cause) {
			t.Errorf("errors.Is(notice, %v) = false, want true", cause)
		}
	})

	t.Run("nothing else matches", func(t *testing.T) {
		// io.EOF is a sibling of the cause, not the cause: a notice that matched
		// it would be collapsing every short read into one branch.
		if errors.Is(notice, io.EOF) {
			t.Errorf("errors.Is(notice, io.EOF) = true, want false for cause %v", cause)
		}
		if errors.Is(notice, errors.New("unrelated")) {
			t.Error("errors.Is(notice, unrelated) = true, want false")
		}
	})

	t.Run("the message is unchanged", func(t *testing.T) {
		// Byte-identical to fmt.Errorf("%w: %v", ErrReconnected, cause), so a
		// caller matching the rendered text and a log line that already contains
		// it both keep working.
		want := ErrReconnected.Error() + ": " + cause.Error()
		if got := notice.Error(); got != want {
			t.Errorf("notice.Error() = %q, want %q", got, want)
		}
		if got := notice.Error(); !strings.Contains(got, cause.Error()) {
			t.Errorf("notice.Error() = %q, want it to carry the cause text %q", got, cause)
		}
	})
}

// TestReconnectNoticeReachesNetError covers the decision a caller actually has
// to make: whether the feed went away because the peer hung up, or because the
// read deadline expired and the data is stale. That difference is only visible
// through the chain, so it is asserted with errors.As and Timeout.
func TestReconnectNoticeReachesNetError(t *testing.T) {
	cause := &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}
	notice := newReconnectNotice(cause)

	if !errors.Is(notice, os.ErrDeadlineExceeded) {
		t.Errorf("errors.Is(notice, os.ErrDeadlineExceeded) = false, want true: %v", notice)
	}
	var netErr *net.OpError
	if !errors.As(notice, &netErr) {
		t.Fatalf("errors.As(notice, &net.OpError) = false, want true: %v", notice)
	}
	if !netErr.Timeout() {
		t.Errorf("netErr.Timeout() = false, want true for cause %v", netErr)
	}
	if !errors.Is(notice, ErrReconnected) {
		t.Errorf("errors.Is(notice, ErrReconnected) = false, want true: %v", notice)
	}
}

// TestReconnectNoticeNilCause covers the guard. push.Client guards against a nil
// error before reporting one but does not promise a cause can never be nil, and
// the %v this replaced printed <nil> rather than panicking, so a notice must not
// be able to turn a missing cause into a panic on a public API.
func TestReconnectNoticeNilCause(t *testing.T) {
	notice := newReconnectNotice(nil)

	if notice != ErrReconnected {
		t.Errorf("newReconnectNotice(nil) = %v, want the bare ErrReconnected sentinel", notice)
	}
	if !errors.Is(notice, ErrReconnected) {
		t.Errorf("errors.Is(notice, ErrReconnected) = false, want true: %v", notice)
	}
	if got := errors.Unwrap(notice); got != nil {
		t.Errorf("errors.Unwrap(notice) = %v, want nil", got)
	}
	if got, want := notice.Error(), ErrReconnected.Error(); got != want {
		t.Errorf("notice.Error() = %q, want %q", got, want)
	}
}

// TestOnReconnectDeliversWrappedCause is the production path: the notice a
// caller reads off Errors() is the one built by onReconnect, so the wrap has to
// survive the trip rather than exist only in newReconnectNotice.
func TestOnReconnectDeliversWrappedCause(t *testing.T) {
	t.Run("with a cause", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		s := newTestClient(t, srv.url, &pipeDialer{})

		ctx := context.Background()
		sub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		cause := newReconnectCause()
		s.onReconnect(cause)
		assertReconnectNotice(t, nextError(t, sub), cause)
	})

	t.Run("with a nil cause", func(t *testing.T) {
		// A bare sentinel is what reaches the caller, not a nil and not a
		// rendered <nil>: sendErr drops a nil error, so a nil-producing
		// constructor would make the channel silently miss the reconnect.
		srv := newHookServer(t, nil, nil)
		s := newTestClient(t, srv.url, &pipeDialer{})

		ctx := context.Background()
		sub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		s.onReconnect(nil)
		notice := nextError(t, sub)
		if !errors.Is(notice, ErrReconnected) {
			t.Errorf("reconnect notice after a nil cause = %v, want ErrReconnected", notice)
		}
		if got, want := notice.Error(), ErrReconnected.Error(); got != want {
			t.Errorf("reconnect notice message = %q, want %q", got, want)
		}
	})
}

// TestOnReconnectResubscribeNamesNumericTopic pins the topic in a resubscribe
// failure as the number the Gateway is addressed with. types.TopicID is an int
// with a String method, so %s rendered the label ("basic-qot") and left the
// caller with a topic they cannot compare against the value they subscribed
// with, and against the value in docs/SPEC.md.
func TestOnReconnectResubscribeNamesNumericTopic(t *testing.T) {
	// The first /hq/Subscribe establishes the subscription; every later one
	// fails, which is the resubscribe.
	srv := newHookServer(t, func(_ string, call int) string {
		if call == 1 {
			return okEnvelope
		}
		return failEnvelope
	}, nil)
	s := newTestClient(t, srv.url, &pipeDialer{})

	ctx := context.Background()
	sub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	s.onReconnect(newReconnectCause())
	_ = nextError(t, sub) // the reconnect notice
	failure := nextError(t, sub)

	if want := "stream: resubscribe 11: "; !strings.HasPrefix(failure.Error(), want) {
		t.Errorf("resubscribe failure = %q, want the topic as the number 11 in %q", failure, want)
	}
	if label := types.TopicBasicQot.String(); strings.Contains(failure.Error(), label) {
		t.Errorf("resubscribe failure = %q, want no %q label in place of the number", failure, label)
	}
	if errors.Is(failure, ErrReconnected) {
		t.Errorf("a resubscribe failure = %v, want it to stay distinct from the reconnect notice", failure)
	}
}
