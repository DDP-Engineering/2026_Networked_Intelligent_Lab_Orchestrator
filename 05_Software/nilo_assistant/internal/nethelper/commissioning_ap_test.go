package nethelper

import (
	"context"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

var fakeCommissioningAPPlans []network.CommissioningAccessPoint

func (*fakeDriver) ReconcileCommissioningAccessPoint(_ context.Context, plan network.CommissioningAccessPoint) error {
	fakeCommissioningAPPlans = append(fakeCommissioningAPPlans, plan)
	return nil
}

func TestCommissioningAccessPointProtocolAcceptsOnlyTypedPlan(t *testing.T) {
	fakeCommissioningAPPlans = nil
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	plan := network.CommissioningAccessPoint{
		Enabled: true, SerialName: "nilo-p2rjmzdkf1", AdapterID: "02:00:00:00:00:03",
		Interface: "wlan1", Address: "192.168.247.1/24",
		Port: 8443,
	}
	response := dispatch(context.Background(), engine, Request{Op: "commissioning.ap.reconcile", CommissioningAccessPoint: &plan})
	if response.Error != "" || len(fakeCommissioningAPPlans) != 1 || !fakeCommissioningAPPlans[0].Enabled {
		t.Fatalf("response=%+v plans=%+v", response, fakeCommissioningAPPlans)
	}
	response = dispatch(context.Background(), engine, Request{Op: "interface.list", CommissioningAccessPoint: &plan})
	if response.Code != "invalid_request" {
		t.Fatalf("smuggled AP plan response=%+v", response)
	}
}
