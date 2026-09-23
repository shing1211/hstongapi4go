// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import "fmt"

type AccountID string

func (a AccountID) String() string { return string(a) }
func (a AccountID) IsZero() bool   { return a == "" }

type OrderID string

func (o OrderID) String() string { return string(o) }
func (o OrderID) IsZero() bool   { return o == "" }

type EntrustID string

func (e EntrustID) String() string { return string(e) }
func (e EntrustID) IsZero() bool   { return e == "" }

type ContractID string

func (c ContractID) String() string { return string(c) }
func (c ContractID) IsZero() bool   { return c == "" }

type SessionToken string

func (s SessionToken) String() string { return string(s) }
func (s SessionToken) IsZero() bool   { return s == "" }

func ParseAccountID(s string) (AccountID, error) {
	if s == "" {
		return "", fmt.Errorf("domain: AccountID: empty")
	}
	return AccountID(s), nil
}
