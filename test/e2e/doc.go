// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package e2e drives the real public SDK against the in-repo mock Gateway
// (test/mockgateway). It exercises every one of the 51 canonical HTTP endpoints
// through its typed manager method, the trade session login/re-login path, the
// TCP push path with an intentionally unrecognisable Any type_url, and the
// typed error mapping. It is offline and credential-free.
package e2e
