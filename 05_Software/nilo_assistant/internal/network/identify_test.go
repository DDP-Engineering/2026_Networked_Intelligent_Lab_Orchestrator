package network

import (
	"context"
	"errors"
	"testing"
	"time"
)

type identifyIndicator struct {
	states []bool
	err    error
}

func (indicator *identifyIndicator) SetIdentifyIndicator(_ context.Context, enabled bool) error {
	indicator.states = append(indicator.states, enabled)
	return indicator.err
}

func TestIdentifyServiceIsBoundedCoalescedAndTypedWhenUnsupported(t *testing.T) {
	unsupported, err := NewIdentifyService(IdentifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := unsupported.Identify(context.Background())
	if err != nil || result.Status != IdentifyUnsupported || result.Duration != 0 {
		t.Fatalf("unsupported result = %+v, %v", result, err)
	}

	indicator := &identifyIndicator{}
	var callbacks []func()
	service, err := NewIdentifyService(IdentifyOptions{
		Indicator: indicator, Duration: 12 * time.Second,
		After: func(_ time.Duration, callback func()) func() {
			callbacks = append(callbacks, callback)
			cancelled := false
			return func() { cancelled = true; _ = cancelled }
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err = service.Identify(context.Background())
		if err != nil || result.Status != IdentifyAccepted || result.Duration != 12*time.Second {
			t.Fatalf("attempt %d result = %+v, %v", attempt, result, err)
		}
	}
	if len(indicator.states) != 2 || !indicator.states[0] {
		t.Fatalf("indicator states = %v", indicator.states)
	}
	callbacks[0]()
	if len(indicator.states) != 2 {
		t.Fatalf("superseded expiry changed indicator: %v", indicator.states)
	}
	callbacks[1]()
	if got := indicator.states[len(indicator.states)-1]; got {
		t.Fatalf("final indicator state = %t", got)
	}

	indicator.err = ErrIdentifyUnsupported
	result, err = service.Identify(context.Background())
	if err != nil || result.Status != IdentifyUnsupported {
		t.Fatalf("runtime unsupported result = %+v, %v", result, err)
	}
	indicator.err = errors.New("gpio sentinel secret")
	if _, err = service.Identify(context.Background()); err == nil || err.Error() != "identify indicator failed" {
		t.Fatalf("unbounded helper failure escaped: %v", err)
	}
}

func TestIdentifyServiceOperationReceiptIsIdempotentUntilAcknowledged(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 4, 0, 0, time.UTC)
	indicator := &identifyIndicator{}
	service, err := NewIdentifyService(IdentifyOptions{Indicator: indicator, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := IdentifyActionRequest{
		OperationID: "identify-operation-1", Source: IdentifySourceInterface, ActiveUntil: now.Add(10 * time.Second),
	}
	first, err := service.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ExecuteIdentifyAction(context.Background(), request)
	if err != nil || second != first || len(indicator.states) != 1 {
		t.Fatalf("first=%+v second=%+v states=%v err=%v", first, second, indicator.states, err)
	}
	if err := service.AcknowledgeIdentifyAction(context.Background(), request.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecuteIdentifyAction(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(indicator.states) != 2 {
		t.Fatalf("acknowledged operation remained cached: %v", indicator.states)
	}
}
