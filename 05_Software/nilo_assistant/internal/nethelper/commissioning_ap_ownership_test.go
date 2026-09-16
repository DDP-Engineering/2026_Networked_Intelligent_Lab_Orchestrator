package nethelper

import (
	"os"
	"reflect"
	"runtime"
	"testing"

	"nilo.local/nilo/internal/network"
)

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
	}, ownership)
	if !ok || !plan.Enabled || plan.AdapterID != ownership.AccessPoint.AdapterID || plan.Interface != ownership.AccessPoint.Interface {
		t.Fatalf("restored plan = %+v, %t", plan, ok)
	}
	if _, ok := retainedCommissioningAccessPoint(network.CommissioningAccessPoint{
		RetainOwned: true, SerialName: "nilo-other00000", Address: "192.168.247.1/24", Port: 8443,
	}, ownership); ok {
		t.Fatal("mismatched serial adopted durable ownership")
	}
}
