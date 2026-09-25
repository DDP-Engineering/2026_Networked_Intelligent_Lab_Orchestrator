package nethelper

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"nilo.local/nilo/internal/network"
)

// fakeSettings is the settings half of the fake driver: it records what it was
// asked to apply and, for a join, the password it was handed.
type fakeSettings struct {
	applied  []network.InterfaceSettings
	joined   []joinedNetwork
	networks []network.WiFiNetwork
}

type joinedNetwork struct {
	join     network.WiFiJoin
	password string
}

func (d *fakeDriver) ApplySettings(_ context.Context, device Device, settings network.InterfaceSettings) error {
	d.events = append(d.events, "settings:"+device.Interface.ID)
	d.settings.applied = append(d.settings.applied, settings)
	d.device.Interface.IPv4 = []string{settings.Address.String()}
	return nil
}

func (d *fakeDriver) ScanWiFi(_ context.Context, device Device) ([]network.WiFiNetwork, error) {
	if device.Interface.Kind != network.KindWiFi {
		return nil, network.ErrUnsupported
	}
	return d.settings.networks, nil
}

func (d *fakeDriver) JoinWiFi(_ context.Context, device Device, join network.WiFiJoin, password string) error {
	d.events = append(d.events, "join:"+device.Interface.ID)
	d.settings.joined = append(d.settings.joined, joinedNetwork{join: join, password: password})
	return nil
}

// TestSettingsChangeIsCheckpointedAndBoundToTheAdapter covers the helper half
// of REQ-FUNC-5e2b91d7 and REQ-FUNC-3a71c204: the change carries the adapter
// identity, is checkpointed against the device, applied through the driver and
// restored like any interface change.
func TestSettingsChangeIsCheckpointedAndBoundToTheAdapter(t *testing.T) {
	ctx := context.Background()
	e, d, _ := tunnelEngine(t)
	d.device.Interface.AdapterID = "aa:00:00:00:00:01"
	d.device.Interface.IPv4 = []string{"192.0.2.2/24"}
	settings := &network.InterfaceSettings{AdapterID: "aa:00:00:00:00:01", Method: network.MethodStatic, Address: netip.MustParsePrefix("192.0.2.7/24")}
	if _, err := e.Prepare(ctx, "wrong-adapter", network.Change{Action: network.ActionInterfaceSettings, Target: "eth-a",
		Settings: &network.InterfaceSettings{AdapterID: "aa:00:00:00:00:99", Method: network.MethodAutomatic, DNSAutomatic: true}}, network.ConfirmationWindow); err == nil {
		t.Fatal("settings for a different adapter were prepared on this device")
	}
	record, err := e.Prepare(ctx, "settings", network.Change{Action: network.ActionInterfaceSettings, Target: "eth-a", Settings: settings}, network.ConfirmationWindow)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if record.Before.Interface.ID != "eth-a" || len(d.events) != 1 || d.events[0] != "checkpoint" {
		t.Fatalf("record=%+v events=%v", record.Before.Interface, d.events)
	}
	if err := e.Apply(ctx, "settings"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(d.settings.applied) != 1 || d.settings.applied[0].Address.String() != "192.0.2.7/24" {
		t.Fatalf("applied = %+v", d.settings.applied)
	}
	if err := e.Restore(ctx, "settings"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(d.device.Interface.IPv4) != 1 || d.device.Interface.IPv4[0] != "192.0.2.2/24" {
		t.Fatalf("device after restore = %+v", d.device.Interface)
	}
}

// TestWiFiJoinConsumesTheStagedPasswordExactlyOnce covers the helper half of
// REQ-FUNC-7c40aa18: the password reaches the driver from the keystore, and
// the staged copy is gone afterwards.
func TestWiFiJoinConsumesTheStagedPasswordExactlyOnce(t *testing.T) {
	ctx := context.Background()
	e, d, keystore := tunnelEngine(t)
	d.device.Interface.Kind = network.KindWiFi
	d.device.Interface.AdapterID = "aa:00:00:00:00:02"
	d.settings.networks = []network.WiFiNetwork{{SSID: "lab-hotspot", Security: network.WiFiWPA2Personal, Supported: true, Strength: 70}}
	staged := dispatch(ctx, e, Request{Op: "wireguard.secrets.stage", Secret: &SecretPayload{Password: "SENTINELpassword"}})
	if staged.Error != "" {
		t.Fatal(staged.Error)
	}
	scan := dispatch(ctx, e, Request{Op: "wifi.scan", Change: network.Change{Action: network.ActionWiFiJoin, Target: "eth-a"}})
	if scan.Error != "" || len(scan.Networks) != 1 {
		t.Fatalf("scan: %+v", scan)
	}
	join := network.Change{Action: network.ActionWiFiJoin, Target: "eth-a", WiFi: &network.WiFiJoin{SSID: "lab-hotspot", Security: network.WiFiWPA2Personal}, SecretHandle: staged.Handle}
	if _, err := e.Prepare(ctx, "join", join, network.ConfirmationWindow); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := e.Apply(ctx, "join"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(d.settings.joined) != 1 || d.settings.joined[0].password != "SENTINELpassword" || d.settings.joined[0].join.SSID != "lab-hotspot" {
		t.Fatalf("joined = %+v", d.settings.joined)
	}
	if _, err := keystore.ReadStaged(staged.Handle); !errors.Is(err, ErrSecretsNotFound) {
		t.Fatalf("the staged password survived the join: %v", err)
	}
	if err := e.Confirm(ctx, "join"); err != nil {
		t.Fatal(err)
	}
	// An open network needs no handle; a personal one without a handle is
	// refused before anything is touched.
	if _, err := e.Prepare(ctx, "open", network.Change{Action: network.ActionWiFiJoin, Target: "eth-a", WiFi: &network.WiFiJoin{SSID: "lab-hotspot", Security: network.WiFiOpen}}, network.ConfirmationWindow); err != nil {
		t.Fatalf("open join: %v", err)
	}
	if err := e.Restore(ctx, "open"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prepare(ctx, "no-password", network.Change{Action: network.ActionWiFiJoin, Target: "eth-a", WiFi: &network.WiFiJoin{SSID: "lab-hotspot", Security: network.WiFiWPA2Personal}}, network.ConfirmationWindow); err == nil {
		t.Fatal("a personal join without a staged password was prepared")
	}
	d.device.Interface.Kind = network.KindEthernet
	if _, err := e.Prepare(ctx, "wired", join, network.ConfirmationWindow); err == nil {
		t.Fatal("a wired adapter joined a wireless network")
	}
}
