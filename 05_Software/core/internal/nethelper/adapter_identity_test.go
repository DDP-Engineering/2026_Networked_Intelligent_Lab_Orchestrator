package nethelper

import (
	"os"
	"path/filepath"
	"testing"
)

// A container interface and a built-in adapter reach NetworkManager through the
// same wired device API, so neither signal decides alone: a permanent hardware
// address without a bus device, or a bus device without a permanent address, is
// not a physical adapter.
func TestPhysicalAdapterRequiresBothHardwareSignals(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		permanent string
		busDevice bool
		physical  bool
	}{
		{
			name: "permanent address and bus device", permanent: "02:00:00:00:00:01",
			busDevice: true, physical: true,
		},
		{name: "permanent address without a bus device", permanent: "02:00:00:00:00:01"},
		{name: "bus device without a permanent address", busDevice: true},
		{name: "neither signal"},
		{name: "blank permanent address with a bus device", permanent: "   ", busDevice: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if physical := physicalAdapter(testCase.permanent, testCase.busDevice); physical != testCase.physical {
				t.Fatalf("physicalAdapter(%q, %t) = %t, want %t",
					testCase.permanent, testCase.busDevice, physical, testCase.physical)
			}
		})
	}
}

// The probe reads the one kernel fact that separates an adapter on a real bus
// from a virtual interface, and refuses a name that could walk out of the root.
func TestBusDevicePresentReadsTheInterfaceDeviceEntry(t *testing.T) {
	// The root sits one level down and its parent carries a "device" directory
	// of its own, so a name that escapes the root would find something and the
	// refusal is proven rather than assumed.
	outside := t.TempDir()
	root := filepath.Join(outside, "net")
	for _, directory := range []string{
		filepath.Join(outside, "device"),
		filepath.Join(root, "enp1s0", "device"),
		filepath.Join(root, "veth9f2a41"),
		filepath.Join(root, "tun0"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("prepare %s: %v", directory, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tun0", "device"), []byte("not a bus device"), 0o644); err != nil {
		t.Fatalf("prepare regular device entry: %v", err)
	}

	previous := busDeviceRoot
	busDeviceRoot = root
	t.Cleanup(func() { busDeviceRoot = previous })

	for _, testCase := range []struct {
		name    string
		device  string
		present bool
	}{
		{name: "adapter on a real bus", device: "enp1s0", present: true},
		{name: "virtual interface", device: "veth9f2a41"},
		{name: "device entry that is not a bus directory", device: "tun0"},
		{name: "interface that is not present", device: "enp9s0"},
		{name: "empty name", device: ""},
		{name: "name containing a path separator", device: "../enp1s0"},
		{name: "parent directory", device: ".."},
		{name: "current directory", device: "."},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if present := busDevicePresent(testCase.device); present != testCase.present {
				t.Fatalf("busDevicePresent(%q) = %t, want %t",
					testCase.device, present, testCase.present)
			}
		})
	}
}
