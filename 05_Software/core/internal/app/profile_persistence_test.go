package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/network"
)

const importedKey = "SENTINELimportedAAAAAAAAAAAAAAAAAAAAAAAAAAA="

// auditEverything selects every stored event, which is what a leak check needs.
var auditEverything = audit.Filter{}

func profileText(t *testing.T, peerKey string) string {
	t.Helper()
	return "[Interface]\nPrivateKey = " + importedKey + "\nAddress = 10.60.0.2/32\nDNS = 10.60.0.1\n" +
		"[Peer]\nPublicKey = " + peerKey + "\nAllowedIPs = 10.60.0.0/16\nEndpoint = hub.example:51820\n"
}

// TestImportedProfilesSurviveARestartInactive is the behaviour a user asked for
// in plain terms: several saved WireGuard profiles, still there after the
// appliance restarts, none of them silently connected.
func TestImportedProfilesSurviveARestartInactive(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	actor := uuid.NewString()
	now := time.Date(2026, 9, 3, 17, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	first := openApplication(t, ctx, dataDir, clock)
	simulated := network.NewSimulation(clock)
	if err := simulated.UseProfileStore(ctx, first.VPNProfileRepository()); err != nil {
		t.Fatalf("attach the profile store: %v", err)
	}
	operations := newOperations(t, first, simulated, clock)

	imports := map[string]string{
		"office": "SENTINELpeeroneAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"site-b": "SENTINELpeertwoAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	}
	for name, peerKey := range imports {
		previewed, err := operations.PreviewChange(ctx, actor, network.Change{
			Action: network.ActionProfileImport, Target: name, ProfileText: profileText(t, peerKey),
		})
		if err != nil {
			t.Fatalf("preview import %s: %v", name, err)
		}
		if _, err := operations.Acknowledge(ctx, actor, previewed.ID, previewed.Revision); err != nil {
			t.Fatalf("acknowledge import %s: %v", name, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A second run over the same data directory, with a fresh in-memory
	// simulation, is what a restart is.
	second := openApplication(t, ctx, dataDir, clock)
	t.Cleanup(func() { _ = second.Close() })
	restarted := network.NewSimulation(clock)
	if err := restarted.UseProfileStore(ctx, second.VPNProfileRepository()); err != nil {
		t.Fatalf("attach the profile store after the restart: %v", err)
	}

	status, err := restarted.WireGuard(ctx)
	if err != nil {
		t.Fatalf("read WireGuard status: %v", err)
	}
	if len(status.Profiles) != 2 {
		t.Fatalf("%d profiles after the restart, want 2", len(status.Profiles))
	}
	byName := map[string]network.ProfileStatus{}
	for _, profile := range status.Profiles {
		byName[profile.Name] = profile
		if profile.Active {
			t.Fatalf("%s came back connected, which nobody observed", profile.Name)
		}
		if profile.AutoConnect {
			t.Fatalf("%s came back selected for startup, which nobody chose", profile.Name)
		}
	}
	for name := range imports {
		profile, present := byName[name]
		if !present {
			t.Fatalf("%s did not survive the restart", name)
		}
		if len(profile.Profile.Peers) != 1 || profile.Profile.Peers[0].Endpoint != "hub.example:51820" {
			t.Fatalf("%s lost its public configuration: %+v", name, profile.Profile)
		}
		if len(profile.Profile.Peers[0].AllowedIPs) != 1 || profile.Profile.Peers[0].AllowedIPs[0] != "10.60.0.0/16" {
			t.Fatalf("%s lost its routing intent: %v", name, profile.Profile.Peers[0].AllowedIPs)
		}
	}

	// The simulation holds no key material and the database must not either.
	events, err := second.AuditRepository().List(ctx, auditEverything)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), importedKey) {
			t.Fatalf("audit event %s carries the imported private key", event.ID)
		}
	}
	operationsAfter, err := second.NetworkOperationRepository().Recent(ctx, 10)
	if err != nil {
		t.Fatalf("read operations: %v", err)
	}
	if len(operationsAfter) != 2 {
		t.Fatalf("%d recorded imports, want 2", len(operationsAfter))
	}
	for _, record := range operationsAfter {
		if strings.Contains(string(record.Parameters)+record.Target+record.Revision, importedKey) {
			t.Fatalf("operation %s carries the imported private key", record.ID)
		}
	}
}

// TestSelectingAStartupProfileSurvivesARestart covers the other half of the
// saved inventory: the startup choice is configuration, so it persists, and
// only one profile can hold it.
func TestSelectingAStartupProfileSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	actor := uuid.NewString()
	now := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	first := openApplication(t, ctx, dataDir, clock)
	simulated := network.NewSimulation(clock)
	if err := simulated.UseProfileStore(ctx, first.VPNProfileRepository()); err != nil {
		t.Fatalf("attach: %v", err)
	}
	operations := newOperations(t, first, simulated, clock)

	for name, peerKey := range map[string]string{
		"office": "SENTINELpeeroneAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"site-b": "SENTINELpeertwoAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
	} {
		previewed, err := operations.PreviewChange(ctx, actor, network.Change{
			Action: network.ActionProfileImport, Target: name, ProfileText: profileText(t, peerKey),
		})
		if err != nil {
			t.Fatalf("preview %s: %v", name, err)
		}
		if _, err := operations.Acknowledge(ctx, actor, previewed.ID, previewed.Revision); err != nil {
			t.Fatalf("acknowledge %s: %v", name, err)
		}
	}

	for _, chosen := range []string{"sim-office", "sim-site-b"} {
		previewed, err := operations.PreviewChange(ctx, actor, network.Change{
			Action: network.ActionAutostartSelect, Target: chosen, Enabled: true,
		})
		if err != nil {
			t.Fatalf("preview startup %s: %v", chosen, err)
		}
		if _, err := operations.Acknowledge(ctx, actor, previewed.ID, previewed.Revision); err != nil {
			t.Fatalf("acknowledge startup %s: %v", chosen, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	second := openApplication(t, ctx, dataDir, clock)
	t.Cleanup(func() { _ = second.Close() })
	restarted := network.NewSimulation(clock)
	if err := restarted.UseProfileStore(ctx, second.VPNProfileRepository()); err != nil {
		t.Fatalf("attach after restart: %v", err)
	}
	status, err := restarted.WireGuard(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	selected := make([]string, 0, 1)
	for _, profile := range status.Profiles {
		if profile.AutoConnect {
			selected = append(selected, profile.Name)
		}
	}
	if len(selected) != 1 || selected[0] != "site-b" {
		t.Fatalf("startup selection after the restart = %v, want only site-b", selected)
	}
}
