package nethelper

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestDirectRecoveryActivationFailureReturnsItsDurableIdentityForCleanup(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	driver := &fakeDriver{
		device: Device{Interface: network.Interface{
			Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
			Physical: true, State: network.StateConnected, Controllable: true,
		}},
		recoveryErr: errors.New("activation failed after profile creation"),
	}
	store := &memoryStore{}
	engine, err := NewEngine(driver, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	response := dispatch(context.Background(), engine, Request{
		Op: "recovery.link.activate", AdapterID: "aa:bb:cc:dd:ee:ff", InterfaceName: "eth1",
		RecoveryExpiresAt: now.Add(10 * time.Minute),
	})
	if response.Error == "" || response.RecoveryRecordID == "" {
		t.Fatalf("partial activation response = %+v, want error and cleanup identity", response)
	}
	if store.directRecovery == nil || store.directRecovery.ID != response.RecoveryRecordID || store.directRecovery.Phase != "activation_failed" {
		t.Fatalf("durable partial activation = %+v, response=%+v", store.directRecovery, response)
	}
}

func TestDirectRecoveryRefusesToOverwriteAnExistingOwnedLink(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	existing := DirectRecoveryRecord{
		Version: 1, ID: "00000000-0000-4000-8000-000000000021", Phase: "active",
		AdapterID: "aa:bb:cc:dd:ee:21", InterfaceName: "eth1", Deadline: now.Add(10 * time.Minute),
		UpdatedAt: now,
	}
	store := &memoryStore{directRecovery: &existing}
	driver := &fakeDriver{device: Device{Interface: network.Interface{
		Name: "eth2", AdapterID: "aa:bb:cc:dd:ee:22", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}}
	engine, err := NewEngine(driver, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	identity, err := engine.ActivateRecoveryLink(context.Background(), "aa:bb:cc:dd:ee:22", "eth2", now.Add(10*time.Minute))
	if err == nil || identity != existing.ID {
		t.Fatalf("second activation identity=%q err=%v, want existing identity and refusal", identity, err)
	}
	if store.directRecovery == nil || *store.directRecovery != existing || len(driver.recovery) != 0 {
		t.Fatalf("existing recovery was changed: record=%+v effects=%v", store.directRecovery, driver.recovery)
	}
}

func TestRecoveryDeviceSafetyMatrix(t *testing.T) {
	t.Parallel()

	safe := Device{Interface: network.Interface{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}
	if err := validateRecoveryDevice(safe, "aa:bb:cc:dd:ee:ff", "eth1"); err != nil {
		t.Fatalf("safe adapter refused: %v", err)
	}
	linkLocalIPv6 := safe
	linkLocalIPv6.Interface.IPv6 = []string{"fe80::1/64"}
	if err := validateRecoveryDevice(linkLocalIPv6, "aa:bb:cc:dd:ee:ff", "eth1"); err != nil {
		t.Fatalf("ordinary IPv6 link-local address refused: %v", err)
	}
	for _, mutate := range []func(*Device){
		func(device *Device) { device.Interface.Kind = network.KindWiFi },
		func(device *Device) { device.Interface.Physical = false },
		func(device *Device) { device.Interface.Controllable = false },
		func(device *Device) { device.Interface.State = network.StateDisconnected },
		func(device *Device) { device.Interface.IPv4 = []string{"169.254.10.20/16"} },
		func(device *Device) { device.Interface.IPv6 = []string{"2001:db8::1/64"} },
		func(device *Device) { device.Interface.Gateway4 = "192.0.2.1" },
		func(device *Device) { device.Connection = "dhcp-or-static-active" },
		func(device *Device) { device.Available = []string{"saved-production-profile"} },
		func(device *Device) { device.Transitioning = true },
	} {
		candidate := safe
		mutate(&candidate)
		if err := validateRecoveryDevice(candidate, "aa:bb:cc:dd:ee:ff", "eth1"); err == nil {
			t.Errorf("unsafe device accepted: %+v", candidate)
		}
	}
	if err := validateRecoveryDevice(safe, "different", "eth1"); err == nil {
		t.Fatal("adapter identity mismatch accepted")
	}
	if err := validateRecoveryDevice(safe, "aa:bb:cc:dd:ee:ff", "eth9"); err == nil {
		t.Fatal("interface name mismatch accepted")
	}
}

func TestDirectRecoveryDeadlineIsDurableAndExtended(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	driver := &fakeDriver{device: Device{Interface: network.Interface{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}}
	store := &memoryStore{}
	engine, err := NewEngine(driver, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	firstDeadline := now.Add(10 * time.Minute)
	recordID, err := engine.ActivateRecoveryLink(context.Background(), "aa:bb:cc:dd:ee:ff", "eth1", firstDeadline)
	if err != nil {
		t.Fatal(err)
	}
	if store.directRecovery == nil || store.directRecovery.ID != recordID || store.directRecovery.Deadline != firstDeadline {
		t.Fatalf("durable record = %+v", store.directRecovery)
	}
	now = now.Add(5 * time.Minute)
	extendedDeadline := now.Add(10 * time.Minute)
	if err := engine.ExtendRecoveryLink(context.Background(), recordID, extendedDeadline); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery.Deadline != extendedDeadline {
		t.Fatalf("extended deadline = %v, want %v", store.directRecovery.Deadline, extendedDeadline)
	}

	restarted, err := NewEngine(driver, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	now = extendedDeadline.Add(time.Second)
	if err := restarted.ExpireDirectRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery != nil || len(driver.recovery) == 0 || driver.recovery[len(driver.recovery)-1] != "restore:"+recordID {
		t.Fatalf("record=%+v effects=%v, want exact restore", store.directRecovery, driver.recovery)
	}
}

func TestDirectRecoveryExpiresByElapsedTimeAcrossWallClockRollback(t *testing.T) {
	wall := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	steady := wall
	driver := &fakeDriver{device: Device{Interface: network.Interface{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}}
	store := &memoryStore{}
	engine, err := newEngineWithClocks(driver, store, func() time.Time { return wall }, func() time.Time { return steady })
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := engine.ActivateRecoveryLink(
		context.Background(), "aa:bb:cc:dd:ee:ff", "eth1", wall.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	steady = steady.Add(2 * time.Minute)
	wall = wall.Add(-time.Hour)
	if err := engine.ExpireDirectRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery != nil || !slices.Contains(driver.recovery, "restore:"+recordID) {
		t.Fatalf("elapsed deadline did not restore through wall rollback: record=%+v effects=%v", store.directRecovery, driver.recovery)
	}
}

func TestDirectRecoveryRestartFailsClosedOnPersistedClockRollback(t *testing.T) {
	recordedAt := time.Date(2026, 9, 14, 21, 0, 0, 0, time.UTC)
	wall := recordedAt.Add(-time.Minute)
	steady := recordedAt
	record := DirectRecoveryRecord{
		Version: 1, ID: "00000000-0000-4000-8000-000000000041", Phase: "active",
		AdapterID: "aa:bb:cc:dd:ee:ff", InterfaceName: "eth1",
		Deadline: recordedAt.Add(10 * time.Minute), UpdatedAt: recordedAt,
	}
	store := &memoryStore{directRecovery: &record}
	driver := &fakeDriver{}
	engine, err := newEngineWithClocks(driver, store, func() time.Time { return wall }, func() time.Time { return steady })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ExpireDirectRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery != nil || !slices.Contains(driver.recovery, "restore:"+record.ID) {
		t.Fatalf("persisted clock rollback did not fail closed: record=%+v effects=%v", store.directRecovery, driver.recovery)
	}
}

func TestDirectRecoveryPersistsItsLastClockObservationForRestart(t *testing.T) {
	wall := time.Date(2026, 9, 14, 21, 30, 0, 0, time.UTC)
	steady := wall
	driver := &fakeDriver{device: Device{Interface: network.Interface{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}}
	store := &memoryStore{}
	engine, err := newEngineWithClocks(driver, store, func() time.Time { return wall }, func() time.Time { return steady })
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := engine.ActivateRecoveryLink(
		context.Background(), "aa:bb:cc:dd:ee:ff", "eth1", wall.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	wall = wall.Add(time.Minute)
	steady = steady.Add(time.Minute)
	if err := engine.ExpireDirectRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery == nil || store.directRecovery.UpdatedAt != wall {
		t.Fatalf("last helper wall observation was not persisted: record=%+v", store.directRecovery)
	}

	wall = wall.Add(-30 * time.Second)
	steady = steady.Add(time.Hour)
	restarted, err := newEngineWithClocks(driver, store, func() time.Time { return wall }, func() time.Time { return steady })
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ExpireDirectRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.directRecovery != nil || !slices.Contains(driver.recovery, "restore:"+recordID) {
		t.Fatalf("helper restart did not fail closed behind its last observation: record=%+v effects=%v", store.directRecovery, driver.recovery)
	}
}
