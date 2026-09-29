// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import "testing"

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
