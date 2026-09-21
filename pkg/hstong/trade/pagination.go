// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package trade

import "context"

// Page-size limits for the cursor list endpoints (queryParamStr). The vendor
// documents a default of 20 and a hard limit of "less than 100".
const (
	// DefaultPageSize is the page size applied when the requested size is
	// missing or non-positive.
	DefaultPageSize = 20
	// MaxPageSize is the largest accepted page size. The documented limit is
	// strictly less than 100, so the cap is 99.
	MaxPageSize = 99
)

// Page is one page of a cursor query. Cursor is the value to pass to the next
// call; it is normally the last row's queryParamStr and may be empty when the
// Gateway reports no further page.
type Page[T any] struct {
	// Items holds the rows of this page.
	Items []T
	// Cursor is the cursor for the next page; empty means no further page.
	Cursor string
}

// PageFunc fetches one page of at most pageSize rows starting at cursor. cursor
// is "0" for the first call. The returned Cursor is fed back on the next call.
type PageFunc[T any] func(ctx context.Context, pageSize int, cursor string) (Page[T], error)

// ClampPageSize returns pageSize bounded to [1, MaxPageSize] with a non-positive
// value becoming DefaultPageSize. The list methods use it so a request always
// carries a size the Gateway accepts.
func ClampPageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

// Paginate walks a queryParamStr cursor list with fetch and returns every row in
// order. pageSize is normalized by ClampPageSize and starts the walk at cursor
// "0".
//
// Iteration stops at the vendor-documented end of data — an empty page or a
// page shorter than the resolved page size — and, defensively, when the cursor
// does not advance or becomes empty, so a Gateway that ignores queryParamStr
// cannot cause an unbounded loop. Cancelling ctx stops the walk and returns the
// rows gathered so far together with ctx's error.
//
// Paginate issues exactly one fetch per page and never retries; a fetch error
// is returned as-is.
func Paginate[T any](ctx context.Context, pageSize int, fetch PageFunc[T]) ([]T, error) {
	if fetch == nil {
		return nil, nil
	}

	size := ClampPageSize(pageSize)
	cursor := "0"
	var all []T

	for {
		if err := ctx.Err(); err != nil {
			return all, err
		}

		page, err := fetch(ctx, size, cursor)
		if err != nil {
			return all, err
		}
		all = append(all, page.Items...)

		if len(page.Items) == 0 || len(page.Items) < size {
			return all, nil
		}
		if page.Cursor == "" || page.Cursor == cursor {
			return all, nil
		}
		cursor = page.Cursor
	}
}
