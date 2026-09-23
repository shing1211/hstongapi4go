// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

type OrderState struct {
	EntrustID   EntrustID
	OrderID     OrderID
	Status      types.EntrustStatus
	Side        types.EntrustBS
	Symbol      Symbol
	Price       Price
	Quantity    Quantity
	FilledQty   Quantity
	AvgPrice    Price
	Market      Market
	Channel     string
	OrderType   types.EntrustType
	TimeInForce string
	CreatedAt   string
	UpdatedAt   string
}

type EntrustEvent string

const (
	EventCreated            EntrustEvent = "created"
	EventAccepted           EntrustEvent = "accepted"
	EventPartiallyFilled    EntrustEvent = "partially_filled"
	EventFilled             EntrustEvent = "filled"
	EventCancelled          EntrustEvent = "cancelled"
	EventPartiallyCancelled EntrustEvent = "partially_cancelled"
	EventRejected           EntrustEvent = "rejected"
	EventModifySubmitted    EntrustEvent = "modify_submitted"
	EventModifyAccepted     EntrustEvent = "modify_accepted"
	EventCancelSubmitted    EntrustEvent = "cancel_submitted"
	EventCancelAccepted     EntrustEvent = "cancel_accepted"
)

func NextOrderState(current types.EntrustStatus, event EntrustEvent) types.EntrustStatus {
	transitions := map[struct {
		from  types.EntrustStatus
		event EntrustEvent
	}]types.EntrustStatus{
		{types.EntrustStatusNoRegister, EventCreated}:                  types.EntrustStatusWaitToRegister,
		{types.EntrustStatusWaitToRegister, EventAccepted}:             types.EntrustStatusRegistered,
		{types.EntrustStatusRegistered, EventPartiallyFilled}:          types.EntrustStatusPartFilled,
		{types.EntrustStatusRegistered, EventFilled}:                   types.EntrustStatusFilled,
		{types.EntrustStatusPartFilled, EventFilled}:                   types.EntrustStatusFilled,
		{types.EntrustStatusRegistered, EventCancelled}:                types.EntrustStatusWaitCancel,
		{types.EntrustStatusPartFilled, EventCancelled}:                types.EntrustStatusPartFilledWaitCancel,
		{types.EntrustStatusWaitCancel, EventCancelAccepted}:           types.EntrustStatusCancelled,
		{types.EntrustStatusPartFilledWaitCancel, EventCancelAccepted}: types.EntrustStatusPartCancelled,
		{types.EntrustStatusRegistered, EventRejected}:                 types.EntrustStatusHostReject,
		{types.EntrustStatusRegistered, EventModifySubmitted}:          types.EntrustStatusWaitModifyRegistered,
		{types.EntrustStatusWaitModifyRegistered, EventModifyAccepted}: types.EntrustStatusRegistered,
		{types.EntrustStatusPartFilled, EventModifySubmitted}:          types.EntrustStatusWaitModifyPartFilled,
		{types.EntrustStatusWaitModifyPartFilled, EventModifyAccepted}: types.EntrustStatusPartFilled,
	}
	if next, ok := transitions[struct {
		from  types.EntrustStatus
		event EntrustEvent
	}{current, event}]; ok {
		return next
	}
	return current
}
