package network

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAdapterRoleCoordinatorRejectsTheOppositePhysicalRole(t *testing.T) {
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{
			AdapterID:    "AA:BB:CC:DD:EE:FF",
			Interface:    "enx-device",
			Role:         AdapterRoleDevice,
			ResourceID:   "device-network-1",
			ResourceName: "PLC network",
		}}
	})

	err := coordinator.CheckAndRun([]AdapterRoleClaim{{
		AdapterID: "aa:bb:cc:dd:ee:ff",
		Interface: "renamed-device",
		Role:      AdapterRoleManagement,
	}}, func() error {
		t.Fatal("a conflicting mutation must not run")
		return nil
	})

	var conflict *AdapterRoleConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected adapter role conflict, got %v", err)
	}
	if conflict.AdapterID != "aa:bb:cc:dd:ee:ff" || conflict.Interface != "renamed-device" {
		t.Fatalf("unexpected adapter identity: %+v", conflict)
	}
	if conflict.ExistingRole != AdapterRoleDevice || conflict.RequestedRole != AdapterRoleManagement {
		t.Fatalf("unexpected roles: %+v", conflict)
	}
	if conflict.ConflictingResourceID != "device-network-1" || conflict.ConflictingResourceName != "PLC network" {
		t.Fatalf("unexpected resource: %+v", conflict)
	}
}

func TestAdapterRoleCoordinatorAllowsSameRoleAndVirtualClaims(t *testing.T) {
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{
			{AdapterID: "adapter-a", Interface: "eth0", Role: AdapterRoleManagement},
			{Interface: "nilo0", Role: AdapterRoleManagement},
		}
	})
	runs := 0
	if err := coordinator.CheckAndRun([]AdapterRoleClaim{{AdapterID: "ADAPTER-A", Role: AdapterRoleManagement}}, func() error {
		runs++
		return nil
	}); err != nil {
		t.Fatalf("same-role claim was refused: %v", err)
	}
	if err := coordinator.CheckAndRun([]AdapterRoleClaim{{Interface: "nilo0", Role: AdapterRoleDevice}}, func() error {
		runs++
		return nil
	}); err != nil {
		t.Fatalf("virtual claim was refused: %v", err)
	}
	if runs != 2 {
		t.Fatalf("mutation ran %d times, want 2", runs)
	}
}

func TestAdapterRoleCoordinatorReservationHasOneOppositeRoleWinner(t *testing.T) {
	coordinator := NewAdapterRoleCoordinator()
	start := make(chan struct{})
	result := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	reserve := func(owner string, role AdapterRole) {
		ready.Done()
		<-start
		result <- coordinator.Reserve(owner, []AdapterRoleClaim{{AdapterID: "adapter-a", Interface: "eth0", Role: role}})
	}
	go reserve("management-change", AdapterRoleManagement)
	go reserve("device-change", AdapterRoleDevice)
	ready.Wait()
	close(start)

	first, second := <-result, <-result
	if (first == nil) == (second == nil) {
		t.Fatalf("expected exactly one reservation winner, got %v and %v", first, second)
	}
	coordinator.Release("management-change")
	coordinator.Release("device-change")
	if err := coordinator.Reserve("later-device-change", []AdapterRoleClaim{{AdapterID: "adapter-a", Role: AdapterRoleDevice}}); err != nil {
		t.Fatalf("released adapter could not be reserved again: %v", err)
	}
}

func TestAdapterRoleCoordinatorRejectsUnknownRoles(t *testing.T) {
	coordinator := NewAdapterRoleCoordinator()
	if err := coordinator.Check([]AdapterRoleClaim{{
		AdapterID: "adapter-a", Role: AdapterRole("uplink"),
	}}); err == nil {
		t.Fatal("an unknown requested role was accepted")
	}
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{AdapterID: "adapter-b", Role: AdapterRole("uplink")}}
	})
	if err := coordinator.Check([]AdapterRoleClaim{{
		AdapterID: "adapter-c", Role: AdapterRoleManagement,
	}}); err == nil {
		t.Fatal("an unknown persisted role was accepted")
	}
}

func TestSimulationReservesTheProposedDeviceAdapterUntilRecovery(t *testing.T) {
	ctx := context.Background()
	simulation := NewSimulation(time.Now)
	store := newMemoryDeviceNetworkStore()
	record := *proposedNetwork()
	record.ID = "dn-line-3"
	if _, err := store.Save(ctx, record, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := simulation.UseDeviceNetworkStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	proposed := record
	proposed.DeviceInterface = "02:00:00:00:00:01"
	checkpoint, err := simulation.ArmChange(ctx, "operation-1", Change{
		Action: ActionDeviceNetworkUpdate, Target: record.ID, DeviceNetwork: &proposed,
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{
		AdapterID: proposed.DeviceInterface, Role: AdapterRoleManagement,
	}}); err == nil {
		t.Fatal("the proposed device adapter was not reserved")
	}
	if _, err := simulation.Restore(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := simulation.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{
		AdapterID: proposed.DeviceInterface, Role: AdapterRoleManagement,
	}}); err != nil {
		t.Fatalf("recovery retained the proposed adapter reservation: %v", err)
	}
}

func TestSimulationRollbackRestoresDurableDeviceRoleClaims(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	store := newMemoryDeviceNetworkStore()
	record := *proposedNetwork()
	record.ID = "dn-line-3"
	var err error
	record, err = store.Save(ctx, record, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.UseDeviceNetworkStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	proposed := record
	proposed.DeviceInterface = "02:00:00:00:00:03"
	change := Change{Action: ActionDeviceNetworkUpdate, Target: record.ID, DeviceNetwork: &proposed}
	checkpoint, err := simulation.ArmChange(ctx, "operation-1", change, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.ApplyChange(ctx, change); err != nil {
		t.Fatal(err)
	}
	if _, err := simulation.Restore(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}

	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), record.DeviceInterface, true)
	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), proposed.DeviceInterface, false)
}

func TestSimulationExpiryRestoresDurableDeviceRoleClaims(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	store := newMemoryDeviceNetworkStore()
	record := *proposedNetwork()
	record.ID = "dn-line-3"
	var err error
	record, err = store.Save(ctx, record, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.UseDeviceNetworkStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	proposed := record
	proposed.DeviceInterface = "02:00:00:00:00:03"
	change := Change{Action: ActionDeviceNetworkUpdate, Target: record.ID, DeviceNetwork: &proposed}
	if _, err := simulation.ArmChange(ctx, "operation-1", change, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := simulation.ApplyChange(ctx, change); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := simulation.DeviceNetworks(ctx); err != nil {
		t.Fatal(err)
	}

	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), record.DeviceInterface, true)
	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), proposed.DeviceInterface, false)
}

type restoreFailingDeviceNetworkStore struct {
	*memoryDeviceNetworkStore
	fail bool
}

func (store *restoreFailingDeviceNetworkStore) Save(ctx context.Context, record DeviceNetwork, at time.Time) (DeviceNetwork, error) {
	if store.fail {
		return DeviceNetwork{}, errors.New("simulated snapshot restore failure")
	}
	return store.memoryDeviceNetworkStore.Save(ctx, record, at)
}

func TestSimulationRollbackFailureRetainsBothDeviceRoleReservations(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	store := &restoreFailingDeviceNetworkStore{memoryDeviceNetworkStore: newMemoryDeviceNetworkStore()}
	record := *proposedNetwork()
	record.ID = "dn-line-3"
	var err error
	record, err = store.Save(ctx, record, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.UseDeviceNetworkStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	proposed := record
	proposed.DeviceInterface = "02:00:00:00:00:03"
	change := Change{Action: ActionDeviceNetworkUpdate, Target: record.ID, DeviceNetwork: &proposed}
	checkpoint, err := simulation.ArmChange(ctx, "operation-1", change, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulation.ApplyChange(ctx, change); err != nil {
		t.Fatal(err)
	}
	store.fail = true
	if _, err := simulation.Restore(ctx, checkpoint); err == nil {
		t.Fatal("rollback succeeded despite the simulated store failure")
	}
	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), record.DeviceInterface, true)
	assertDeviceRoleClaim(t, simulation.AdapterRoleCoordinator(), proposed.DeviceInterface, true)
}

func assertDeviceRoleClaim(t *testing.T, coordinator *AdapterRoleCoordinator, adapterID string, claimed bool) {
	t.Helper()
	err := coordinator.Check([]AdapterRoleClaim{{AdapterID: adapterID, Role: AdapterRoleManagement}})
	if claimed && err == nil {
		t.Fatalf("adapter %s is no longer claimed by the restored device network", adapterID)
	}
	if !claimed && err != nil {
		t.Fatalf("adapter %s retained a stale device role claim: %v", adapterID, err)
	}
}
