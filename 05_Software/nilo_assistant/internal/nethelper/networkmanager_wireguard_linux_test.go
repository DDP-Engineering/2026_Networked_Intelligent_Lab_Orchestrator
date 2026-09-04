//go:build linux

package nethelper

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

// disposableKeyPair makes a real Curve25519 key pair for a test peer. The
// keys exist for the duration of one container and identify nothing outside
// it; no key from any real configuration is used anywhere in the suite.
func disposableKeyPair(t *testing.T) (private, public string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
}

func systemTunnelGate(t *testing.T) {
	t.Helper()
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated NetworkManager test container")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil || os.Geteuid() != 0 {
		t.Fatal("system test requires the root disposable container")
	}
}

// TestNetworkManagerWireGuardLifecycleSystem is the P2 exit criterion on real
// NetworkManager: two profiles are created from staged keys, listed inactive,
// activated, switched under a system-wide checkpoint, put back explicitly and
// by the independent timeout, deactivated and deleted, while an operator's own
// tunnel profile is listed as external and never touched.
func TestNetworkManagerWireGuardLifecycleSystem(t *testing.T) {
	systemTunnelGate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	if tool, err := installedWG(ctx); err != nil || !tool.Installed {
		t.Fatalf("the harness must install wg before this test: %+v %v", tool, err)
	}
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	keystore, _ := newKeystore(t)
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureKeystore(keystore); err != nil {
		t.Fatal(err)
	}

	// An operator's own tunnel profile, created outside NILO, is the external
	// case. It is created through the same bus and deleted at the end by the
	// test itself, never by the helper.
	externalPrivate, externalPublic := disposableKeyPair(t)
	externalUUID := uuid.NewString()
	externalSettings := map[string]map[string]dbus.Variant{
		"connection": {"id": variant("operator-wg"), "uuid": variant(externalUUID), "type": variant("wireguard"), "interface-name": variant("wgext"), "autoconnect": variant(false)},
		"wireguard":  {"private-key": variant(externalPrivate), "private-key-flags": variant(uint32(0)), "peers": variant([]map[string]dbus.Variant{{"public-key": variant(externalPublic), "allowed-ips": variant([]string{"10.99.0.0/24"}), "endpoint": variant("192.0.2.99:51820")}})},
		"ipv4":       {"method": variant("manual"), "address-data": variant([]map[string]dbus.Variant{{"address": variant("10.99.0.2"), "prefix": variant(uint32(32))}})},
		"ipv6":       {"method": variant("disabled")},
	}
	var externalPath dbus.ObjectPath
	var result map[string]dbus.Variant
	if err := driver.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.AddConnection2", 0, externalSettings, uint32(addConnectionToDisk), map[string]dbus.Variant{}).Store(&externalPath, &result); err != nil {
		t.Fatalf("create the external profile: %v", err)
	}
	defer func() {
		_ = driver.bus.Object(nm, externalPath).Call(connectionAPI+".Delete", 0).Err
	}()
	externalBefore, err := driver.settingsOf(ctx, externalPath)
	if err != nil {
		t.Fatal(err)
	}

	// Two NILO profiles: a split tunnel and a full tunnel, each with its own
	// disposable peer.
	type disposable struct {
		id, name, ref string
		profile       network.Profile
	}
	profiles := []disposable{
		{id: "profile-split", name: "split", profile: network.Profile{Name: "split", Addresses: []string{"10.66.1.2/32"}, DNS: []string{"10.66.1.1"}, MTU: 1380,
			Peers: []network.ProfilePeer{{AllowedIPs: []string{"10.66.0.0/24", "192.168.77.0/24"}, Endpoint: "192.0.2.10:51820", PersistentKeepalive: 25, HasPresharedKey: true}}}},
		{id: "profile-full", name: "full", profile: network.Profile{Name: "full", Addresses: []string{"10.67.1.2/32", "fd00:67::2/128"}, DNS: []string{"10.67.1.1"},
			Peers: []network.ProfilePeer{{AllowedIPs: []string{"0.0.0.0/0", "::/0"}, Endpoint: "192.0.2.11:51820"}}}},
	}
	for index := range profiles {
		private, _ := disposableKeyPair(t)
		_, peerPublic := disposableKeyPair(t)
		preshared, _ := disposableKeyPair(t)
		profiles[index].profile.Peers[0].PublicKey = peerPublic
		// The parser always stages one preshared entry per peer, empty when
		// the peer has none, so the helper can check the alignment.
		payload := &SecretPayload{PrivateKey: private, PresharedKeys: []string{""}}
		if profiles[index].profile.Peers[0].HasPresharedKey {
			payload.PresharedKeys = []string{preshared}
		}
		staged := dispatch(ctx, engine, Request{Op: "wireguard.secrets.stage", Secret: payload})
		if staged.Error != "" {
			t.Fatal(staged.Error)
		}
		profile := profiles[index].profile
		created := dispatch(ctx, engine, Request{Op: "wireguard.profile.create", Handle: staged.Handle, ProfileID: profiles[index].id,
			Change: network.Change{Action: network.ActionProfileImport, Target: profiles[index].name, Profile: &profile}})
		if created.Error != "" {
			t.Fatalf("create %s: %s", profiles[index].name, created.Error)
		}
		profiles[index].ref = created.ConnectionRef
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		for _, profile := range profiles {
			_ = driver.DeactivateTunnel(cleanup, profile.ref)
			_ = driver.DeleteTunnel(cleanup, profile.ref)
		}
	}()

	// NetworkManager keeps the created key material root-only on disk.
	entries, err := os.ReadDir("/etc/NetworkManager/system-connections")
	if err != nil {
		t.Fatal(err)
	}
	keyfiles := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if len(entry.Name()) > len(niloTunnelPrefix) && entry.Name()[:len(niloTunnelPrefix)] == niloTunnelPrefix {
			keyfiles++
			if info.Mode().Perm()&0o077 != 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
				t.Fatalf("keyfile %s is not root-only: %v", entry.Name(), info.Mode())
			}
		}
	}
	if keyfiles != 2 {
		t.Fatalf("%d NILO keyfiles on disk, want 2", keyfiles)
	}

	// The inventory lists both inactive with their intent intact, and the
	// operator's profile as external.
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]network.TunnelObservation{}
	for _, tunnel := range tunnels {
		byRef[tunnel.ConnectionRef] = tunnel
	}
	for _, profile := range profiles {
		tunnel, ok := byRef[profile.ref]
		if !ok || !tunnel.NILOOwned || tunnel.Active {
			t.Fatalf("%s after creation = %+v, want an inactive NILO-owned tunnel", profile.name, tunnel)
		}
		if !reflect.DeepEqual(tunnel.Profile.Addresses, profile.profile.Addresses) ||
			!reflect.DeepEqual(tunnel.Profile.Peers[0].AllowedIPs, profile.profile.Peers[0].AllowedIPs) ||
			tunnel.Profile.Peers[0].Endpoint != profile.profile.Peers[0].Endpoint ||
			tunnel.Profile.Peers[0].PublicKey != profile.profile.Peers[0].PublicKey ||
			!reflect.DeepEqual(tunnel.Profile.DNS, profile.profile.DNS) || tunnel.Profile.MTU != profile.profile.MTU {
			t.Fatalf("%s lost its imported intent: got %+v want %+v", profile.name, tunnel.Profile, profile.profile)
		}
	}
	external, ok := byRef[externalUUID]
	if !ok || external.NILOOwned || external.Name != "operator-wg" {
		t.Fatalf("external tunnel = %+v", external)
	}
	if _, err := engine.Prepare(ctx, "external", network.Change{Action: network.ActionVPNActivate, Target: "x", ConnectionID: externalUUID}, network.ConfirmationWindow); !errors.Is(err, network.ErrExternalProfile) {
		t.Fatalf("activating the external tunnel: %v", err)
	}
	if err := engine.DeleteProfile(ctx, "operator", externalUUID); !errors.Is(err, network.ErrExternalProfile) {
		t.Fatalf("deleting the external tunnel: %v", err)
	}

	split, full := profiles[0], profiles[1]
	activate := func(id, ref string) Record {
		t.Helper()
		record, err := engine.Prepare(ctx, id, network.Change{Action: network.ActionVPNActivate, Target: id, ConnectionID: ref}, network.ConfirmationWindow)
		if err != nil {
			t.Fatalf("prepare %s: %v", id, err)
		}
		if err := engine.Apply(ctx, id); err != nil {
			t.Fatalf("apply %s: %v", id, err)
		}
		return record
	}
	activeRef := func() string {
		t.Helper()
		tunnels, err := driver.Tunnels(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if active, ok := activeOwnedTunnel(tunnels); ok {
			if active.Interface != niloTunnelInterface {
				t.Fatalf("active tunnel device = %q, want %s", active.Interface, niloTunnelInterface)
			}
			return active.ConnectionRef
		}
		return ""
	}

	first := activate("up-split", split.ref)
	if first.System == nil || first.System.ActiveTunnel != "" {
		t.Fatalf("first activation snapshot = %+v, want no previous tunnel", first.System)
	}
	if activeRef() != split.ref {
		t.Fatal("the split tunnel is not active after activation")
	}
	if err := engine.Confirm(ctx, "up-split"); err != nil {
		t.Fatal(err)
	}

	// The switch replaces the split tunnel with the full tunnel and puts it
	// back on explicit restoration, including every non-tunnel device's state.
	switched := activate("switch", full.ref)
	if switched.System.ActiveTunnel != split.ref {
		t.Fatalf("switch snapshot = %+v, want the split tunnel recorded", switched.System)
	}
	if activeRef() != full.ref {
		t.Fatal("the full tunnel is not the active one after the switch")
	}
	if err := engine.Restore(ctx, "switch"); err != nil {
		t.Fatalf("restore the switch: %v", err)
	}
	if activeRef() != split.ref {
		t.Fatal("the split tunnel did not come back after restoring the switch")
	}
	if restored, _ := engine.Recovery(); restored.Phase != "restored" {
		t.Fatal(restored.Phase)
	}

	// No helper timer runs here. NetworkManager alone restores the unconfirmed
	// switch after the real two-minute timeout, and the engine verifies it.
	timeout := activate("switch-timeout", full.ref)
	if activeRef() != full.ref {
		t.Fatal("the timed switch did not activate the full tunnel")
	}
	wait, stopWait := context.WithTimeout(ctx, 150*time.Second)
	err = driver.waitSystem(wait, func(snapshot SystemSnapshot) bool { return snapshot == *timeout.System })
	stopWait()
	if err != nil {
		t.Fatalf("NetworkManager did not restore the switch on its own: %v", err)
	}
	if time.Until(timeout.Deadline) > 2*time.Second {
		t.Fatal("restored before the independent timeout")
	}
	if err := engine.Expire(ctx); err != nil {
		t.Fatalf("expire after the independent restoration: %v", err)
	}
	if activeRef() != split.ref {
		t.Fatal("the split tunnel is not active after the timeout restoration")
	}

	// Deactivation, then deletion: an active profile is refused, an inactive one
	// disappears with its key material.
	if err := engine.DeleteProfile(ctx, split.id, split.ref); !errors.Is(err, network.ErrProfileActive) {
		t.Fatalf("deleting the active profile: %v", err)
	}
	if _, err := engine.Prepare(ctx, "down", network.Change{Action: network.ActionVPNDeactivate, Target: split.id, ConnectionID: split.ref}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(ctx, "down"); err != nil {
		t.Fatal(err)
	}
	if activeRef() != "" {
		t.Fatal("a tunnel is still active after deactivation")
	}
	if err := engine.Confirm(ctx, "down"); err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		if err := engine.DeleteProfile(ctx, profile.id, profile.ref); err != nil {
			t.Fatalf("delete %s: %v", profile.name, err)
		}
		if _, err := keystore.Read(profile.id); !errors.Is(err, ErrSecretsNotFound) {
			t.Fatalf("key material of %s survived deletion: %v", profile.name, err)
		}
	}
	tunnels, err = driver.Tunnels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tunnel := range tunnels {
		if tunnel.NILOOwned {
			t.Fatalf("NILO tunnel %s survived deletion", tunnel.Name)
		}
	}
	// The operator's profile is byte-for-byte what it was.
	externalAfter, err := driver.settingsOf(ctx, externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeDBus(externalBefore), normalizeDBus(externalAfter)) {
		t.Fatal("the external profile changed during the NILO lifecycle")
	}
}
