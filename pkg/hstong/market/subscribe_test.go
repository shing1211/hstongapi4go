// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package market

import (
	"context"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func TestManager_SubscribeWire(t *testing.T) {
	m, rec := newTestManager(t, map[string]string{
		"/hq/Subscribe": `{"ok":true,"err":"","data":null}`,
	})

	err := m.Subscribe(context.Background(), types.TopicBasicQot,
		&dto.Security{DataType: 10000, Code: "0700.HK"},
		&dto.Security{DataType: 10000, Code: "0005.HK"},
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	want := `{"topicId":11,"security":[{"dataType":10000,"code":"0700.HK"},{"dataType":10000,"code":"0005.HK"}]}`
	assertWire(t, rec, "/hq/Subscribe", want)
}

func TestManager_UnsubscribeWire(t *testing.T) {
	m, rec := newTestManager(t, map[string]string{
		"/hq/Unsubscribe": `{"ok":true,"err":"","data":null}`,
	})

	err := m.Unsubscribe(context.Background(), types.TopicOrderBook,
		&dto.Security{DataType: 10000, Code: "0700.HK"},
	)
	if err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	want := `{"topicId":17,"security":[{"dataType":10000,"code":"0700.HK"}]}`
	assertWire(t, rec, "/hq/Unsubscribe", want)
}

func TestManager_SubscribeValidation(t *testing.T) {
	known := &dto.Security{DataType: 10000, Code: "0700.HK"}
	cases := []struct {
		name  string
		topic types.TopicID
		secs  []*dto.Security
	}{
		{"empty list", types.TopicBasicQot, nil},
		{"nil security", types.TopicBasicQot, []*dto.Security{nil}},
		{"unknown topic", types.TopicID(99), []*dto.Security{known}},
		{"unknown topic and empty list", types.TopicID(0), nil},
	}
	ops := map[string]func(*Manager, context.Context, types.TopicID, ...*dto.Security) error{
		"Subscribe":   (*Manager).Subscribe,
		"Unsubscribe": (*Manager).Unsubscribe,
	}
	for opName, op := range ops {
		for _, tt := range cases {
			t.Run(opName+"/"+tt.name, func(t *testing.T) {
				m, rec := newTestManager(t, nil)

				err := op(m, context.Background(), tt.topic, tt.secs...)
				if err == nil {
					t.Fatal("expected an error")
				}
				if code, ok := errs.CodeOf(err); !ok || code != types.StatusInvalidParam {
					t.Fatalf("err = %v (code=%q ok=%v), want StatusInvalidParam", err, code, ok)
				}
				if got := len(rec.requests()); got != 0 {
					t.Fatalf("sent %d requests for invalid input, want 0", got)
				}
			})
		}
	}
}
