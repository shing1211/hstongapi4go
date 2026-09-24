// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

// Pagination holds cursor-based pagination state.
type Pagination struct {
	// Cursor is an opaque cursor; empty for the first page.
	Cursor string
	// PageSize is the max items per page; 0 means default (50).
	PageSize int
	// HasMore is true if more pages exist.
	HasMore bool
}

// NextPage advances the cursor for the next page. Returns false if no
// more pages exist.
func (p *Pagination) NextPage(cursor string) bool {
	if cursor == "" {
		return false
	}
	p.Cursor = cursor
	return true
}

// Apply adds pagination query parameters to the params map.
// Returns a new map with cursor/page_size keys added.
func (p *Pagination) Apply(params map[string]any) map[string]any {
	out := make(map[string]any, len(params)+2)
	for k, v := range params {
		out[k] = v
	}
	if p.Cursor != "" {
		out["cursor"] = p.Cursor
	}
	if p.PageSize > 0 {
		out["page_size"] = p.PageSize
	} else {
		out["page_size"] = 50
	}
	return out
}
