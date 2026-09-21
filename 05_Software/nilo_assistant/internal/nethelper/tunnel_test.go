package nethelper

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// fakeTunnels is the tunnel half of the fake driver. It lives in this file so
// the interface tests keep their fixture, and it records the secrets it was
// handed so a test can prove they came from the keystore and not the request.
type fakeTunnels struct {
	created   []fakeTunnel
	deleted   []string
	active    string
	createErr error
	deleteErr error
	external  bool
}

type fakeTunnel struct {
	profileID string
	profile   network.Profile
	secrets   network.Secrets
	ref       string
}

func (d *fakeDriver) CreateTunnel(_ context.Context, profileID string, profile network.Profile, secrets network.Secrets) (string, error) {
	if d.tunnels.createErr != nil {
		return "", d.tunnels.createErr
	}
	ref := "uuid-" + profileID
	d.tunnels.created = append(d.tunnels.created, fakeTunnel{profileID: profileID, profile: profile, secrets: secrets, ref: ref})
	return ref, nil
}

func (d *fakeDriver) DeleteTunnel(_ context.Context, connectionRef string) error {
	if d.tunnels.deleteErr != nil {
		return d.tunnels.deleteErr
	}
	d.tunnels.deleted = append(d.tunnels.deleted, connectionRef)
	kept := d.tunnels.created[:0]
	for _, tunnel := range d.tunnels.created {
		if tunnel.ref != connectionRef {
			kept = append(kept, tunnel)
		}
	}
	d.tunnels.created = kept
	return nil
}

func (d *fakeDriver) Tunnels(context.Context) ([]network.TunnelObservation, error) {
	items := make([]network.TunnelObservation, 0, len(d.tunnels.created)+1)
	for _, tunnel := range d.tunnels.created {
		items = append(items, network.TunnelObservation{
			ConnectionRef: tunnel.ref, Name: "nilo-wg-" + tunnel.profileID, NILOOwned: true,
			Active: d.tunnels.active == tunnel.ref, Profile: tunnel.profile,
		})
	}
	if d.tunnels.external {
		items = append(items, network.TunnelObservation{ConnectionRef: "external", Name: "wg0", Interface: "wg0", Active: true})
	}
	return items, nil
}

func (d *fakeDriver) ActivateTunnel(_ context.Context, connectionRef string) error {
	d.events = append(d.events, "activate:"+connectionRef)
	d.tunnels.active = connectionRef
	return nil
}

func (d *fakeDriver) DeactivateTunnel(_ context.Context, connectionRef string) error {
	d.events = append(d.events, "deactivate:"+connectionRef)
	if d.tunnels.active == connectionRef {
		d.tunnels.active = ""
	}
	return nil
}

func tunnelEngine(t *testing.T) (*Engine, *fakeDriver, *Keystore) {
	t.Helper()
	keystore, _ := newKeystore(t)
	d := &fakeDriver{device: Device{Interface: network.Interface{ID: "eth-a", Name: "eth0", Kind: network.KindEthernet, State: network.StateConnected, Controllable: true}, Connection: "connection-a", AutoConnect: true, NetworkState: "digest-a"}}
	e, err := NewEngine(d, &memoryStore{}, func() time.Time { return time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ConfigureKeystore(keystore); err != nil {
		t.Fatal(err)
	}
	return e, d, keystore
}

func stagedHandle(t *testing.T, e *Engine) string {
	t.Helper()
	staged := dispatch(context.Background(), e, Request{
		Op: "wireguard.secrets.stage", Secret: &SecretPayload{PrivateKey: sentinelPrivate, PresharedKeys: []string{sentinelPreshared}},
	})
	if staged.Error != "" || staged.Handle == "" {
		t.Fatalf("stage: %+v", staged)
	}
	return staged.Handle
}

func publicProfile() network.Profile {
	return network.Profile{Name: "office", Addresses: []string{"10.60.0.2/32"}, DNS: []string{"10.60.0.1"},
		Peers: []network.ProfilePeer{{PublicKey: sentinelPublic, AllowedIPs: []string{"10.60.0.0/16"}, Endpoint: "hub.example:51820", HasPresharedKey: true}}}
}

func createProfile(t *testing.T, e *Engine, profileID, name string) Response {
	t.Helper()
	profile := publicProfile()
	response := dispatch(context.Background(), e, Request{
		Op: "wireguard.profile.create", Handle: stagedHandle(t, e), ProfileID: profileID,
		Change: network.Change{Action: network.ActionProfileImport, Target: name, Profile: &profile},
	})
	if response.Error != "" {
		t.Fatalf("create %s: %s", profileID, response.Error)
	}
	return response
}

// TestProfileCreationPromotesTheStagedKeyAndCreatesTheConnection is the helper
// half of REQ-FUNC-faf2f697 and REQ-SEC-15dcc526: the connection is created
// from the promoted key material, and the request that asks for it carries a
// handle and public configuration only.
func TestProfileCreationPromotesTheStagedKeyAndCreatesTheConnection(t *testing.T) {
	ctx := context.Background()
	e, d, keystore := tunnelEngine(t)
	handle := stagedHandle(t, e)
	profile := publicProfile()
	response := dispatch(ctx, e, Request{
		Op: "wireguard.profile.create", Handle: handle, ProfileID: "profile-1",
		Change: network.Change{Action: network.ActionProfileImport, Target: "office", Profile: &profile},
	})
	if response.Error != "" {
		t.Fatalf("create: %s", response.Error)
	}
	if response.ConnectionRef != "uuid-profile-1" {
		t.Fatalf("connection reference = %q", response.ConnectionRef)
	}
	if len(d.tunnels.created) != 1 || d.tunnels.created[0].secrets.PrivateKey != sentinelPrivate || d.tunnels.created[0].secrets.PresharedKeys[0] != sentinelPreshared {
		t.Fatalf("the connection was not created from the staged key material: %+v", d.tunnels.created)
	}
	if _, err := keystore.Read("profile-1"); err != nil {
		t.Fatalf("the key material was not promoted to the profile: %v", err)
	}
	if staged := stagedFiles(t, keystore); len(staged) != 0 {
		t.Fatalf("%d staged records remain after creation", len(staged))
	}
	encoded, _ := json.Marshal(response)
	for _, sentinel := range []string{sentinelPrivate, sentinelPreshared} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("the creation response carries key material: %s", encoded)
		}
	}
	if again := dispatch(ctx, e, Request{Op: "wireguard.profile.create", Handle: handle, ProfileID: "profile-2",
		Change: network.Change{Action: network.ActionProfileImport, Target: "again", Profile: &profile}}); again.Error == "" {
		t.Fatal("a spent handle created a second connection")
	}
}

// TestAFailedConnectionCreationLeavesNoKeyMaterial: when NetworkManager refuses
// the connection, neither the staged nor the promoted copy may survive, so an
// operator sees a failed import and nothing on disk.
func TestAFailedConnectionCreationLeavesNoKeyMaterial(t *testing.T) {
	e, d, keystore := tunnelEngine(t)
	d.tunnels.createErr = errors.New("NetworkManager refused the settings")
	handle := stagedHandle(t, e)
	profile := publicProfile()
	response := dispatch(context.Background(), e, Request{
		Op: "wireguard.profile.create", Handle: handle, ProfileID: "profile-1",
		Change: network.Change{Action: network.ActionProfileImport, Target: "office", Profile: &profile},
	})
	if response.Error == "" || response.ConnectionRef != "" {
		t.Fatalf("a failed creation reported success: %+v", response)
	}
	if _, err := keystore.Read("profile-1"); !errors.Is(err, ErrSecretsNotFound) {
		t.Fatalf("promoted key material survived a failed creation: %v", err)
	}
	if staged := stagedFiles(t, keystore); len(staged) != 0 {
		t.Fatalf("%d staged records survived a failed creation", len(staged))
	}
}

// TestProfileDeletionRemovesTheConnectionAndTheKey covers the helper half of
// REQ-FUNC-090be3ae: deleting a profile deletes its connection and its key
// material, and an external connection is never deleted.
func TestProfileDeletionRemovesTheConnectionAndTheKey(t *testing.T) {
	ctx := context.Background()
	e, d, keystore := tunnelEngine(t)
	created := createProfile(t, e, "profile-1", "office")
	d.tunnels.external = true
	if refused := dispatch(ctx, e, Request{Op: "wireguard.profile.delete", ProfileID: "profile-1", ConnectionRef: "external"}); refused.Error == "" {
		t.Fatal("an external connection was deleted")
	}
	if len(d.tunnels.deleted) != 0 {
		t.Fatal(d.tunnels.deleted)
	}
	if deleted := dispatch(ctx, e, Request{Op: "wireguard.profile.delete", ProfileID: "profile-1", ConnectionRef: created.ConnectionRef}); deleted.Error != "" {
		t.Fatal(deleted.Error)
	}
	if len(d.tunnels.deleted) != 1 || d.tunnels.deleted[0] != created.ConnectionRef {
		t.Fatalf("deleted %v", d.tunnels.deleted)
	}
	if _, err := keystore.Read("profile-1"); !errors.Is(err, ErrSecretsNotFound) {
		t.Fatalf("key material survived deletion: %v", err)
	}
}

// TestInventoryIsReadOnlyAndCarriesNoKey covers the helper half of
// REQ-FUNC-1a417267: the inventory lists external tunnels as such and carries
// no secret.
func TestInventoryIsReadOnlyAndCarriesNoKey(t *testing.T) {
	ctx := context.Background()
	e, d, _ := tunnelEngine(t)
	createProfile(t, e, "profile-1", "office")
	d.tunnels.external = true
	before := len(d.events)
	response := dispatch(ctx, e, Request{Op: "wireguard.inventory"})
	if response.Error != "" || len(response.Tunnels) != 2 {
		t.Fatalf("inventory: %+v", response)
	}
	if len(d.events) != before {
		t.Fatal("reading the inventory changed the host")
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), sentinelPrivate) || strings.Contains(string(encoded), sentinelPreshared) {
		t.Fatalf("the inventory carries key material: %s", encoded)
	}
	owned, external := 0, 0
	for _, tunnel := range response.Tunnels {
		if tunnel.NILOOwned {
			owned++
		} else {
			external++
		}
	}
	if owned != 1 || external != 1 {
		t.Fatalf("owned=%d external=%d", owned, external)
	}
}

// TestTunnelActivationIsCheckpointedAgainstTheWholeSystem covers
// REQ-FUNC-9e05d184 and REQ-FUNC-a1584be7 in the helper: a switch is one
// prepared change with a system-wide checkpoint and a recorded previous tunnel,
// applying it deactivates the previous tunnel, and restoring puts it back.
func TestTunnelActivationIsCheckpointedAgainstTheWholeSystem(t *testing.T) {
	ctx := context.Background()
	e, d, _ := tunnelEngine(t)
	createProfile(t, e, "profile-a", "office")
	createProfile(t, e, "profile-b", "site-b")
	d.tunnels.active = "uuid-profile-a"
	d.events = nil

	record, err := e.Prepare(ctx, "switch", network.Change{Action: network.ActionVPNActivate, Target: "profile-b", ConnectionID: "uuid-profile-b"}, network.ConfirmationWindow)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if record.System == nil || record.System.ActiveTunnel != "uuid-profile-a" {
		t.Fatalf("the record does not carry the previous tunnel: %+v", record.System)
	}
	if len(d.events) != 1 || d.events[0] != "checkpoint-all" {
		t.Fatalf("events before apply = %v, want a system-wide checkpoint", d.events)
	}
	if err := e.Apply(ctx, "switch"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if d.tunnels.active != "uuid-profile-b" {
		t.Fatalf("active after switch = %q", d.tunnels.active)
	}
	if err := e.Restore(ctx, "switch"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if d.tunnels.active != "uuid-profile-a" {
		t.Fatalf("active after restore = %q, want profile-a back", d.tunnels.active)
	}
	restored, _ := e.Recovery()
	if restored.Phase != "restored" {
		t.Fatal(restored.Phase)
	}

	// An external tunnel is not a target, and neither is a connection the helper
	// does not know or a change that names none.
	d.tunnels.external = true
	for _, ref := range []string{"external", "unknown", ""} {
		if _, err := e.Prepare(ctx, "bad", network.Change{Action: network.ActionVPNActivate, Target: "x", ConnectionID: ref}, network.ConfirmationWindow); err == nil {
			t.Fatalf("activation of %q was prepared", ref)
		}
	}
}

// TestStartupActivationNeedsNoWindowButRefusesDuringRecovery covers the
// helper half of REQ-FUNC-16e918cb: the selected profile is connected directly,
// and never while a change is pending.
func TestStartupActivationNeedsNoWindowButRefusesDuringRecovery(t *testing.T) {
	ctx := context.Background()
	e, d, _ := tunnelEngine(t)
	createProfile(t, e, "profile-a", "office")
	if _, err := e.Prepare(ctx, "pending", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if response := dispatch(ctx, e, Request{Op: "wireguard.startup", ConnectionRef: "uuid-profile-a"}); response.Code != "busy" {
		t.Fatalf("startup during a pending change: %+v", response)
	}
	if err := e.Restore(ctx, "pending"); err != nil {
		t.Fatal(err)
	}
	if response := dispatch(ctx, e, Request{Op: "wireguard.startup", ConnectionRef: "uuid-profile-a"}); response.Error != "" {
		t.Fatalf("startup: %+v", response)
	}
	if d.tunnels.active != "uuid-profile-a" {
		t.Fatalf("active = %q", d.tunnels.active)
	}
	d.tunnels.external = true
	if response := dispatch(ctx, e, Request{Op: "wireguard.startup", ConnectionRef: "external"}); response.Error == "" {
		t.Fatal("startup connected an external tunnel")
	}
}
