package nethelper

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type capturingSystemPower struct {
	calls []string
	err   error
}

func (power *capturingSystemPower) Restart(context.Context) error {
	power.calls = append(power.calls, "restart")
	return power.err
}

func (power *capturingSystemPower) PowerOff(context.Context) error {
	power.calls = append(power.calls, "power_off")
	return power.err
}

func TestPowerOperationsAreExactEmptyAllowlistedEnvelopes(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	controller := &capturingSystemPower{}
	if err = engine.ConfigurePower(controller); err != nil {
		t.Fatal(err)
	}

	for _, operation := range []string{"system.restart", "system.power_off"} {
		if response := dispatch(context.Background(), engine, Request{Op: operation}); response.Error != "" {
			t.Fatalf("%s failed: %+v", operation, response)
		}
	}
	if want := []string{"restart", "power_off"}; !reflect.DeepEqual(controller.calls, want) {
		t.Fatalf("power calls = %v, want %v", controller.calls, want)
	}

	before := len(controller.calls)
	for _, request := range []Request{
		{Op: "system.restart", ID: "later"},
		{Op: "system.power_off", Revision: "1"},
		{Op: "system.restart", ConnectionRef: "reboot.target"},
		{Op: "system.power_off", DeviceNetworkID: "now"},
		{Op: "system.reboot"},
	} {
		response := dispatch(context.Background(), engine, request)
		if response.Code != "invalid_request" {
			t.Fatalf("request %+v = %+v, want invalid_request", request, response)
		}
	}
	if len(controller.calls) != before {
		t.Fatalf("invalid envelopes reached controller: %v", controller.calls[before:])
	}
}

func TestPowerOperationsFailWhenControllerIsUnavailable(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	response := dispatch(context.Background(), engine, Request{Op: "system.restart"})
	if response.Code != "power_unavailable" {
		t.Fatalf("unconfigured power = %+v, want power_unavailable", response)
	}
}
