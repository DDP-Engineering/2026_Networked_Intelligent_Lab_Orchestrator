package network

import (
	"context"
	"testing"
	"time"
)

func TestMaintenancePersistsTrafficExpiresChangesAndStops(t *testing.T) {
	h := newOperationHarness(t)
	counters := &countersStub{readings: steady(1000, 100, 5)}
	sampler := newSamplerHarness(t, counters)
	p, _ := h.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	_, _ = h.manager.Acknowledge(context.Background(), firstActor, p.ID, p.Revision)
	ctx, cancel := context.WithCancel(context.Background())
	samples, expiry := make(chan time.Time), make(chan time.Time)
	done := make(chan struct{})
	observed := make(chan error, 3)
	go func() {
		runMaintenance(ctx, sampler.sampler, h.manager, samples, expiry, func(err error) { observed <- err })
		close(done)
	}()
	// Each tick is acknowledged through the callback, avoiding races with the
	// deterministic clocks while proving the same loop serve launches.
	<-observed
	sampler.now = sampler.now.Add(SampleInterval)
	sampler.elapsed += SampleInterval
	samples <- time.Now()
	<-observed
	sampler.now = sampler.now.Add(time.Minute)
	sampler.elapsed += time.Minute
	samples <- time.Now()
	<-observed
	if len(sampler.store.written) != 1 {
		t.Fatal("maintenance did not persist history")
	}
	h.now = h.now.Add(ConfirmationWindow)
	expiry <- time.Now()
	<-observed
	op, _ := h.manager.Get(p.ID)
	if op.State != StateRolledBack {
		t.Fatalf("maintenance did not expire change: %s", op.State)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance leaked after shutdown")
	}
}
