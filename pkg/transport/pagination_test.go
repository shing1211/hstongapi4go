// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
)

func TestPaginationApply(t *testing.T) {
	p := &domain.Pagination{
		Cursor:   "abc123",
		PageSize: 25,
		HasMore:  true,
	}

	params := map[string]any{
		"filter": "active",
	}
	result := ApplyPagination(p, params)

	if len(result) != 3 {
		t.Errorf("result length = %d, want 3", len(result))
	}
	if result["cursor"] != "abc123" {
		t.Errorf("cursor = %v, want abc123", result["cursor"])
	}
	if result["page_size"] != 25 {
		t.Errorf("page_size = %v, want 25", result["page_size"])
	}
	if result["filter"] != "active" {
		t.Errorf("filter = %v, want active", result["filter"])
	}
}

func TestPaginationApplyEmptyCursor(t *testing.T) {
	p := &domain.Pagination{
		PageSize: 0,
		HasMore:  true,
	}

	params := map[string]any{
		"filter": "all",
	}
	result := ApplyPagination(p, params)

	if _, ok := result["cursor"]; ok {
		t.Error("cursor should not be in result when empty")
	}
	if result["page_size"] != 50 {
		t.Errorf("page_size = %v, want 50 (default)", result["page_size"])
	}
}

func TestPaginationApplyNoPageSize(t *testing.T) {
	p := &domain.Pagination{
		Cursor:   "cursor456",
		PageSize: 0,
	}

	params := map[string]any{}
	result := ApplyPagination(p, params)

	if result["cursor"] != "cursor456" {
		t.Errorf("cursor = %v, want cursor456", result["cursor"])
	}
	if result["page_size"] != 50 {
		t.Errorf("page_size = %v, want 50 (default)", result["page_size"])
	}
}

func TestPaginationApplyWithPageSize(t *testing.T) {
	p := &domain.Pagination{
		Cursor:   "",
		PageSize: 100,
	}

	params := map[string]any{}
	result := ApplyPagination(p, params)

	if result["page_size"] != 100 {
		t.Errorf("page_size = %v, want 100", result["page_size"])
	}
	if _, ok := result["cursor"]; ok {
		t.Error("cursor should not be in result when empty")
	}
}

// TestApplyPaginationDoesNotMutateItsInput pins that the caller's map survives,
// which a shared params map would not: the walk closures build one map per
// request and a mutation would leak the previous page's cursor into the next.
func TestApplyPaginationDoesNotMutateItsInput(t *testing.T) {
	params := map[string]any{"filter": "active"}
	ApplyPagination(&domain.Pagination{Cursor: "abc"}, params)

	if len(params) != 1 {
		t.Errorf("the input map gained %d keys, want none", len(params)-1)
	}
	if _, ok := params["cursor"]; ok {
		t.Error("the input map was mutated with a cursor key")
	}
}

// TestApplyPaginationToleratesANilPagination pins the nil case, because a caller
// that has not finished building a request should get the default page size
// rather than a panic.
func TestApplyPaginationToleratesANilPagination(t *testing.T) {
	result := ApplyPagination(nil, map[string]any{"filter": "all"})

	if result["page_size"] != defaultPageSize {
		t.Errorf("page_size = %v, want %d", result["page_size"], defaultPageSize)
	}
	if _, ok := result["cursor"]; ok {
		t.Error("cursor should not be present for a nil pagination")
	}
}
