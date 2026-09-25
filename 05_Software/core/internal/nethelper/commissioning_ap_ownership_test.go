package nethelper

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"

	"nilo.local/nilo/internal/network"
)

func TestCommissioningAccessPointRollbackRunsEveryReverseCleanup(t *testing.T) {
	var calls []string
	rollback := commissioningAPRollback{
		stopServices: func(context.Context) error {
			calls = append(calls, "stop services")
			return errors.New("injected stop failure")
		},
		deactivateProfile: func(context.Context) error {
			calls = append(calls, "deactivate profile")
			return errors.New("injected deactivate failure")
		},
		deleteProfile: func(context.Context) error {
			calls = append(calls, "delete profile")
			return errors.New("injected delete failure")
		},
		restoreOwnership: func() error {
			calls = append(calls, "restore ownership")
			return errors.New("injected ownership failure")
		},
	}
	if err := rollback.Run(context.Background()); err == nil {
		t.Fatal("rollback discarded injected cleanup failures")
	}
	want := []string{"stop services", "deactivate profile", "delete profile", "restore ownership"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("rollback order = %v, want %v", calls, want)
	}
}

func TestCommissioningAccessPointOwnershipPersistsExactProfile(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := CommissioningProfileOwnership{
		Version: 1,
		AccessPoint: &CommissioningAccessPointProfileRecord{
			AdapterID: "02:00:00:00:00:03", Interface: "wlan1",
			UUID: "0199a2d0-1234-7abc-8def-0123456789ab", SSID: "nilo-p2rjmzdkf1",
		},
	}
	if err := store.SaveCommissioningProfiles(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadCommissioningProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ownership = %+v, want %+v", got, want)
	}
}

func TestCommissioningAccessPointRestoreUsesOnlyExactDurableOwnership(t *testing.T) {
	ownership := CommissioningProfileOwnership{Version: 1, AccessPoint: &CommissioningAccessPointProfileRecord{
		AdapterID: "02:00:00:00:00:03", Interface: "wlan1",
		UUID: "0199a2d0-1234-7abc-8def-0123456789ab", SSID: "nilo-p2rjmzdkf1",
	}}
	plan, ok := retainedCommissioningAccessPoint(network.CommissioningAccessPoint{
		RetainOwned: true, SerialName: "nilo-p2rjmzdkf1", Address: "192.168.247.1/24", Port: 8443,
		EligibleAdapterIDs: []string{"02:00:00:00:00:03"},
	}, ownership)
	if !ok || !plan.Enabled || plan.AdapterID != ownership.AccessPoint.AdapterID || plan.Interface != ownership.AccessPoint.Interface {
		t.Fatalf("restored plan = %+v, %t", plan, ok)
	}
	if _, ok := retainedCommissioningAccessPoint(network.CommissioningAccessPoint{
		RetainOwned: true, SerialName: "nilo-other00000", Address: "192.168.247.1/24", Port: 8443,
		EligibleAdapterIDs: []string{"02:00:00:00:00:03"},
	}, ownership); ok {
		t.Fatal("mismatched serial adopted durable ownership")
	}
	if _, ok := retainedCommissioningAccessPoint(network.CommissioningAccessPoint{
		RetainOwned: true, SerialName: "nilo-p2rjmzdkf1", Address: "192.168.247.1/24", Port: 8443,
		EligibleAdapterIDs: []string{"02:00:00:00:00:04"},
	}, ownership); ok {
		t.Fatal("ineligible durable AP ownership was retained")
	}
}

func TestCommissioningAccessPointActivationRefusesForeignClientState(t *testing.T) {
	ownedUUID := "0199a2d0-1234-7abc-8def-0123456789ab"
	valid := Device{
		Interface: network.Interface{
			Name: "wlan1", AdapterID: "02:00:00:00:00:03", Kind: network.KindWiFi,
			Physical: true, Controllable: true,
		},
		Connection: ownedUUID, Available: []string{ownedUUID},
	}
	tests := []Device{
		func() Device { candidate := valid; candidate.Connection = "foreign-uuid"; return candidate }(),
		func() Device {
			candidate := valid
			candidate.Available = []string{ownedUUID, "foreign-uuid"}
			return candidate
		}(),
		func() Device { candidate := valid; candidate.Interface.Kind = network.KindEthernet; return candidate }(),
		func() Device { candidate := valid; candidate.Interface.Physical = false; return candidate }(),
		func() Device { candidate := valid; candidate.Interface.Controllable = false; return candidate }(),
	}
	for _, device := range tests {
		if commissioningAPDeviceEligible(device, ownedUUID) {
			t.Fatalf("foreign client state accepted: %+v", device)
		}
	}
	if !commissioningAPDeviceEligible(valid, ownedUUID) {
		t.Fatal("exact owned AP state was refused")
	}
}
