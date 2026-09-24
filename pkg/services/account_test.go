// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

func entry() *domain.FundJournalEntry { return &domain.FundJournalEntry{} }

func TestClampPageSize(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, DefaultPageSize},
		{-5, DefaultPageSize},
		{1, 1},
		{50, 50},
		{99, 99},
		{100, MaxPageSize},
		{100000, MaxPageSize},
	}
	for _, tt := range tests {
		if got := clampPageSize(tt.in); got != tt.want {
			t.Errorf("clampPageSize(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestWalkFundJourPagesStopsOnStalledCursor(t *testing.T) {
	calls := 0
	got, err := walkFundJourPages(context.Background(), 10, "0",
		func(_ context.Context, _ int, cursor string) ([]*domain.FundJournalEntry, string, error) {
			calls++
			if calls > 50 {
				t.Fatalf("walk did not terminate on a stalled cursor: %d calls", calls)
			}
			return []*domain.FundJournalEntry{entry()}, cursor, nil
		})
	if err != nil {
		t.Fatalf("walkFundJourPages err = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: a cursor that does not advance must end the walk", calls)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
}

func TestWalkFundJourPagesStopsOnEmptyCursorAndEmptyPage(t *testing.T) {
	t.Run("empty cursor", func(t *testing.T) {
		calls := 0
		_, err := walkFundJourPages(context.Background(), 10, "0",
			func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error) {
				calls++
				return []*domain.FundJournalEntry{entry()}, "", nil
			})
		if err != nil || calls != 1 {
			t.Fatalf("err=%v calls=%d, want nil/1", err, calls)
		}
	})
	t.Run("empty page", func(t *testing.T) {
		calls := 0
		_, err := walkFundJourPages(context.Background(), 10, "0",
			func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error) {
				calls++
				return nil, "next", nil
			})
		if err != nil || calls != 1 {
			t.Fatalf("err=%v calls=%d, want nil/1", err, calls)
		}
	})
}

func TestWalkFundJourPagesAccumulatesPages(t *testing.T) {
	pages := [][]*domain.FundJournalEntry{
		{entry(), entry()},
		{entry()},
	}
	idx := 0
	got, err := walkFundJourPages(context.Background(), 10, "0",
		func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error) {
			if idx >= len(pages) {
				return nil, "", nil
			}
			items := pages[idx]
			idx++
			return items, "cursor", nil
		})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Fatalf("entries = %d, want 3", len(got))
	}
}

func TestWalkFundJourPagesReturnsPartialOnError(t *testing.T) {
	sentinel := errors.New("gateway unavailable")
	calls := 0
	got, err := walkFundJourPages(context.Background(), 10, "0",
		func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error) {
			calls++
			if calls == 1 {
				return []*domain.FundJournalEntry{entry()}, "cursor", nil
			}
			return nil, "", sentinel
		})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %d, want the 1 completed page to survive a partial outage", len(got))
	}
}

func TestWalkFundJourPagesStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := walkFundJourPages(ctx, 10, "0",
		func(context.Context, int, string) ([]*domain.FundJournalEntry, string, error) {
			calls++
			cancel()
			return []*domain.FundJournalEntry{entry()}, "cursor", nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestWalkFundJourPagesBoundsPageCount(t *testing.T) {
	calls := 0
	got, err := walkFundJourPages(context.Background(), 10, "0",
		func(_ context.Context, _ int, cursor string) ([]*domain.FundJournalEntry, string, error) {
			calls++
			return []*domain.FundJournalEntry{entry()}, cursor + "x", nil
		})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if calls != maxPages {
		t.Fatalf("calls = %d, want the walk bounded to maxPages=%d", calls, maxPages)
	}
	if len(got) != maxPages {
		t.Fatalf("entries = %d, want %d", len(got), maxPages)
	}
}
