// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// defaultPageSize is the page size sent when the caller supplies none.
//
// The number is a Gateway convention, which is why it lives here and not on
// domain.Pagination: the value type carries the caller's intent, this carries
// what the wire expects.
const defaultPageSize = 50

// ApplyPagination adds the pagination query parameters for p to params, returning
// a new map. The input map is not modified.
//
// It was a method on transport.Pagination until D3 moved the value type to
// pkg/domain, so the Gateway key names and the default page size stayed on this
// side of the boundary where they belong (ADR 0010 rule 2).
//
// The reply is discarded by every current caller: the services walk cursors
// through a per-row fetch closure rather than reading a paged envelope, so this
// function has no production caller and is exercised only by its tests. It is
// kept because it is the documented encoding of the cursor parameters, and
// dropping it would leave the key names with nowhere to live at all.
func ApplyPagination(p *domain.Pagination, params map[string]any) map[string]any {
	out := make(map[string]any, len(params)+2)
	for k, v := range params {
		out[k] = v
	}
	if p == nil {
		// A caller that has not finished building its request still gets the
		// default page size rather than a panic; the zero Pagination is exactly
		// "first page, server default".
		out["page_size"] = defaultPageSize
		return out
	}
	if p.Cursor != "" {
		out["cursor"] = p.Cursor
	}
	if p.PageSize > 0 {
		out["page_size"] = p.PageSize
	} else {
		out["page_size"] = defaultPageSize
	}
	return out
}
