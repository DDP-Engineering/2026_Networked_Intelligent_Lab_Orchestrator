package main

import (
	"context"
	"nilo.local/nilo/internal/network"
	"path/filepath"
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
