package main

import (
	"context"
	"errors"
	"nilo.local/nilo/internal/network"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRuntimeSelectionNeverConnectsSimulationToHelper(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "absent.sock")
	p, e := newNetworkProvider(context.Background(), "simulation", socket)
	if e != nil {
		t.Fatal(e)
	}
	if p.Source() != network.SourceSimulation {
		t.Fatal("simulation reached live provider")
	}
	items, e := p.Interfaces(context.Background())
	if e != nil || len(items) == 0 {
		t.Fatalf("simulation missing: %v", e)
	}
	if _, e = newNetworkProvider(context.Background(), "appliance", socket); e == nil {
		t.Fatal("live provider accepted absent helper")
	}
	if _, e = newNetworkProvider(context.Background(), "invalid", socket); e == nil {
		t.Fatal("unknown runtime accepted")
	}
}

func TestRuntimeReconciliationStopsBeforeUnsafeDownstreamWork(t *testing.T) {
	t.Parallel()
	failure := errors.New("reconciliation failed")
	for _, test := range []struct {
		name          string
		operationErr  error
		managementErr error
		wantCalls     []string
	}{
		{name: "operation recovery failed", operationErr: failure, wantCalls: []string{"operations"}},
		{name: "management recovery failed", managementErr: failure, wantCalls: []string{"operations", "management"}},
		{name: "safe state reaches forwarding", wantCalls: []string{"operations", "management", "device"}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := make([]string, 0, 3)
			reconciler := func(name string, result error) func(context.Context) error {
				return func(context.Context) error {
					calls = append(calls, name)
					return result
				}
			}
			err := reconcileNetworkRuntime(
				context.Background(),
				reconciler("operations", test.operationErr),
				reconciler("management", test.managementErr),
				reconciler("device", nil),
			)
			if !reflect.DeepEqual(calls, test.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, test.wantCalls)
			}
			if (test.operationErr != nil || test.managementErr != nil) && !errors.Is(err, failure) {
				t.Fatalf("error = %v, want injected failure", err)
			}
			if test.operationErr == nil && test.managementErr == nil && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
		})
	}
}

func TestApplianceRuntimeRestoresDiscoveryBeforePendingIdentify(t *testing.T) {
	t.Parallel()
	calls := make([]string, 0, 6)
	step := func(name string) func(context.Context) error {
		return func(context.Context) error {
			calls = append(calls, name)
			return nil
		}
	}

	err := reconcileApplianceManagementRuntime(
		context.Background(),
		step("access-point"), step("commissioning-links"), step("management"),
		step("metadata"), step("mdns"), step("ssdp"), step("pending-identify"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"access-point", "commissioning-links", "management", "metadata", "mdns", "ssdp", "pending-identify"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want helper-restart publication before pending Identify recovery %v", calls, want)
	}
}
