package network

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSimulationChangesReportedStateAndRestoresItsCheckpoint(t *testing.T) {
	s := NewSimulation(time.Now)
	cp, err := s.ArmRecovery(context.Background(), "one", ConfirmationWindow)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(context.Background(), ActionInterfaceDisable, "sim-enp1s0"); err != nil {
		t.Fatal(err)
	}
	items, _ := s.Interfaces(context.Background())
	if items[0].State != StateDisconnected {
		t.Fatal("disable did not change simulated status")
	}
	if ok, err := s.Restore(context.Background(), cp); err != nil || !ok {
		t.Fatalf("restore: %v %v", ok, err)
	}
	items, _ = s.Interfaces(context.Background())
	if items[0].State != StateConnected {
		t.Fatal("restore did not restore status")
	}
	if err := s.Apply(context.Background(), ActionInterfaceDisable, "sim-lo"); !errors.Is(err, ErrNotControllable) {
		t.Fatalf("loopback control: %v", err)
	}
}

func TestSimulationWireGuardLifecycleAndSingleStartupSelection(t *testing.T) {
	s := NewSimulation(time.Now)
	ctx := context.Background()
	for _, name := range []string{"office", "backup"} {
		change := Change{Action: ActionProfileImport, Target: name, ProfileText: fullTunnel}
		if _, err := s.Inspect(ctx, change); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyChange(ctx, change); err != nil {
			t.Fatal(err)
		}
	}
	status, _ := s.WireGuard(ctx)
	if len(status.Profiles) != 2 || status.Profiles[0].Active || status.Profiles[0].AutoConnect {
		t.Fatalf("import activated profile: %+v", status)
	}
	first, second := status.Profiles[0].ID, status.Profiles[1].ID
	for _, id := range []string{first, second} {
		if err := s.Apply(ctx, ActionVPNActivate, id); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyChange(ctx, Change{Action: ActionAutostartSelect, Target: id, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Apply(ctx, ActionProfileRemove, second); !errors.Is(err, ErrProfileActive) {
		t.Fatalf("removed active profile: %v", err)
	}
	if err := s.Apply(ctx, ActionVPNDeactivate, second); err != nil {
		t.Fatal(err)
	}
	status, _ = s.WireGuard(ctx)
	active, startup := 0, 0
	for _, p := range status.Profiles {
		if p.Active {
			active++
		}
		if p.AutoConnect {
			startup++
		}
	}
	if active != 0 || startup != 1 {
		t.Fatalf("runtime disable lost startup selection: %+v", status)
	}
}

func TestSimulationRejectsMissingAndExternalProfiles(t *testing.T) {
	s := NewSimulation(time.Now)
	for _, action := range []Action{ActionVPNActivate, ActionVPNDeactivate, ActionProfileRemove, ActionAutostartSelect} {
		if _, err := s.Inspect(context.Background(), Change{Action: action, Target: "wg0"}); err == nil {
			t.Fatalf("accepted unmanaged target for %s", action)
		}
	}
}
