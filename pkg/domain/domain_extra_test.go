// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"os/exec"
	"strconv"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

func runMoneyCheck() (int, error) {
	cmd := exec.Command("python", "scripts/check_money.py")
	cmd.Dir = "../../.."
	_, err := cmd.CombinedOutput()
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func TestSymbol(t *testing.T) {
	tests := []struct {
		code     string
		market   Market
		wantFull string
	}{
		{"00700.HK", MarketHK, "00700.HK.HK"},
		{"AAPL.US", MarketUS, "AAPL.US.US"},
		{"000001.SZ", MarketShenzhenConnect, "000001.SZ.SZ"},
		{"600000.SH", MarketShanghaiConnect, "600000.SH.SH"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			sym := NewSymbol(tt.market, tt.code, 10000)
			if sym.FullCode != tt.wantFull {
				t.Errorf("FullCode = %q, want %q", sym.FullCode, tt.wantFull)
			}
		})
	}
}

func TestMarketFromExchange(t *testing.T) {
	tests := []struct {
		exchange string
		want     Market
	}{
		{"K", MarketHK},
		{"P", MarketUS},
		{"v", MarketShenzhenConnect},
		{"t", MarketShanghaiConnect},
		{"X", ""},
	}

	for _, tt := range tests {
		t.Run(tt.exchange, func(t *testing.T) {
			got := MarketFromExchange(types.ExchangeType(tt.exchange))
			if got != tt.want {
				t.Errorf("MarketFromExchange(%q) = %q, want %q", tt.exchange, got, tt.want)
			}
		})
	}
}

func TestDefaultHKTickSchedule(t *testing.T) {
	tests := []struct {
		dtype     int32
		wantLot   uint64
		wantTick  string
	}{
		{10000, 100, "0.001"},
		{10002, 100, "0.001"},
		{10003, 1, "0.001"},
		{10004, 1, "0.001"},
	}

	for _, tt := range tests {
		t.Run(strconv.FormatInt(int64(tt.dtype), 10), func(t *testing.T) {
			ts := DefaultHKTickSchedule(types.DataType(tt.dtype))
			if ts.Lot != tt.wantLot {
				t.Errorf("Lot = %d, want %d", ts.Lot, tt.wantLot)
			}
			if ts.TickSize != tt.wantTick {
				t.Errorf("TickSize = %q, want %q", ts.TickSize, tt.wantTick)
			}
		})
	}
}

func TestOrderStateNext(t *testing.T) {
	tests := []struct {
		current types.EntrustStatus
		event   EntrustEvent
		want    types.EntrustStatus
	}{
		{types.EntrustStatusNoRegister, EventCreated, types.EntrustStatusWaitToRegister},
		{types.EntrustStatusWaitToRegister, EventAccepted, types.EntrustStatusRegistered},
		{types.EntrustStatusRegistered, EventFilled, types.EntrustStatusFilled},
		{types.EntrustStatusRegistered, EventPartiallyFilled, types.EntrustStatusPartFilled},
		{types.EntrustStatusRegistered, EventCancelled, types.EntrustStatusWaitCancel},
	}

	for _, tt := range tests {
		t.Run(string(tt.current)+"+"+string(tt.event), func(t *testing.T) {
			got := NextOrderState(tt.current, tt.event)
			if got != tt.want {
				t.Errorf("NextOrderState = %q, want %q", got, tt.want)
			}
		})
	}
}
