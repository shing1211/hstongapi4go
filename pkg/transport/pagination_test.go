// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"testing"
)

func TestPaginationApply(t *testing.T) {
	p := &Pagination{
		Cursor:   "abc123",
		PageSize: 25,
		HasMore:  true,
	}

	params := map[string]any{
		"filter": "active",
	}
	result := p.Apply(params)

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
	p := &Pagination{
		PageSize: 0,
		HasMore:  true,
	}

	params := map[string]any{
		"filter": "all",
	}
	result := p.Apply(params)

	if _, ok := result["cursor"]; ok {
		t.Error("cursor should not be in result when empty")
	}
	if result["page_size"] != 50 {
		t.Errorf("page_size = %v, want 50 (default)", result["page_size"])
	}
}

func TestPaginationApplyNoPageSize(t *testing.T) {
	p := &Pagination{
		Cursor:   "cursor456",
		PageSize: 0,
	}

	params := map[string]any{}
	result := p.Apply(params)

	if result["cursor"] != "cursor456" {
		t.Errorf("cursor = %v, want cursor456", result["cursor"])
	}
	if result["page_size"] != 50 {
		t.Errorf("page_size = %v, want 50 (default)", result["page_size"])
	}
}

func TestPaginationApplyWithPageSize(t *testing.T) {
	p := &Pagination{
		Cursor:   "",
		PageSize: 100,
	}

	params := map[string]any{}
	result := p.Apply(params)

	if result["page_size"] != 100 {
		t.Errorf("page_size = %v, want 100", result["page_size"])
	}
	if _, ok := result["cursor"]; ok {
		t.Error("cursor should not be in result when empty")
	}
}

func TestNextPage(t *testing.T) {
	p := &Pagination{}

	if p.NextPage("") {
		t.Error("NextPage with empty cursor should return false")
	}
	if p.Cursor != "" {
		t.Errorf("cursor = %q, want empty", p.Cursor)
	}

	if !p.NextPage("newcursor") {
		t.Error("NextPage with non-empty cursor should return true")
	}
	if p.Cursor != "newcursor" {
		t.Errorf("cursor = %q, want newcursor", p.Cursor)
	}
}

func TestNextPageReturnsFalseForEmptyCursor(t *testing.T) {
	p := &Pagination{}
	if p.NextPage("") {
		t.Error("NextPage with empty cursor should return false")
	}
}

func TestNextPageSetsCursor(t *testing.T) {
	p := &Pagination{}
	if !p.NextPage("newcursor") {
		t.Error("NextPage with non-empty cursor should return true")
	}
	if p.Cursor != "newcursor" {
		t.Errorf("cursor = %q, want newcursor", p.Cursor)
	}
}
