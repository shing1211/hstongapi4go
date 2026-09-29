// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

// Pagination is a cursor-based pagination request, as a caller supplies it.
//
// It lived in pkg/transport until D3, which is where the wire knowledge of a
// request parameter type does not belong: the Gateway key names and the default
// page size are transport concerns (ADR 0010 rules 2 and 4), while this is a
// value the caller builds and the services read. That the wire half has been
// split off is what lets pkg/services stop importing pkg/transport, which is the
// edge D3's new layering rule forbids.
//
// HasMore is part of the caller's intent and is never written by the services;
// the services read Cursor and PageSize and derive the rest from the reply.
type Pagination struct {
	// Cursor is an opaque cursor; empty for the first page.
	Cursor string
	// PageSize is the max items per page; 0 means the Gateway default.
	PageSize int
	// HasMore is true if more pages exist.
	HasMore bool
}

// NextPage advances the cursor to the next page's value. It returns false when
// cursor is empty, which is how a walk learns it has reached the end: an empty
// cursor is not a page to fetch, so the caller stops rather than re-requesting
// the first page forever.
func (p *Pagination) NextPage(cursor string) bool {
	if cursor == "" {
		return false
	}
	p.Cursor = cursor
	return true
}
