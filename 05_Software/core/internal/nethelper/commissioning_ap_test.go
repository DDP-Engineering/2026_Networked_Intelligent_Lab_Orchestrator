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

func TestCommissioningAccessPointProtocolAcceptsBoundedRetentionPlan(t *testing.T) {
	fakeCommissioningAPPlans = nil
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	retained := network.CommissioningAccessPoint{
		RetainOwned: true, EligibleAdapterIDs: []string{"02:00:00:00:00:03"},
		SerialName: "nilo-p2rjmzdkf1", Address: "192.168.247.1/24", Port: 8443,
	}
	response := dispatch(context.Background(), engine, Request{Op: "commissioning.ap.reconcile", CommissioningAccessPoint: &retained})
	if response.Error != "" || len(fakeCommissioningAPPlans) != 1 || !fakeCommissioningAPPlans[0].RetainOwned {
		t.Fatalf("retained response=%+v plans=%+v", response, fakeCommissioningAPPlans)
	}
	fallback := retained
	fallback.Enabled = true
	fallback.AdapterID = "02:00:00:00:00:03"
	fallback.Interface = "wlan1"
	response = dispatch(context.Background(), engine, Request{Op: "commissioning.ap.reconcile", CommissioningAccessPoint: &fallback})
	if response.Error != "" || len(fakeCommissioningAPPlans) != 2 || !fakeCommissioningAPPlans[1].Enabled {
		t.Fatalf("fallback response=%+v plans=%+v", response, fakeCommissioningAPPlans)
	}
	for _, eligible := range [][]string{
		{"not-a-mac"},
		{"02:00:00:00:00:03", "02:00:00:00:00:03"},
	} {
		invalid := retained
		invalid.EligibleAdapterIDs = eligible
		response = dispatch(context.Background(), engine, Request{Op: "commissioning.ap.reconcile", CommissioningAccessPoint: &invalid})
		if response.Error == "" || len(fakeCommissioningAPPlans) != 2 {
			t.Fatalf("invalid retention response=%+v plans=%+v", response, fakeCommissioningAPPlans)
		}
	}
}
