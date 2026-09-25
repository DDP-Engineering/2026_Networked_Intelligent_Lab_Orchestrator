package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

const storedSentinelKey = "SENTINELstoredKeyAAAAAAAAAAAAAAAAAAAAAAAAAA="

func profileRecord(t *testing.T, identity, name string) network.ProfileRecord {
	t.Helper()
	profile, secrets, err := network.ParseProfile(name, []byte(
		"[Interface]\nPrivateKey = "+storedSentinelKey+"\nAddress = 10.60.0.2/32\nDNS = 10.60.0.1\n"+
			"[Peer]\nPublicKey = SENTINELpeerkeyAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"+
			"AllowedIPs = 10.60.0.0/16, 192.168.7.0/24\nEndpoint = hub.example:51820\n"))
	if err != nil {
		t.Fatalf("parse fixture profile: %v", err)
	}
	if secrets.PrivateKey != storedSentinelKey {
		t.Fatalf("the fixture lost its key, so this test would prove nothing")
	}
	at := time.Date(2026, 9, 3, 14, 0, 0, 0, time.UTC)
	return network.ProfileRecord{
		ID: identity, Name: name, Provider: network.ProviderWireGuard,
		Profile: profile, SecretRef: identity, ConnectionRef: "nm-" + identity,
		Revision: "rev-" + identity, CreatedAt: at, UpdatedAt: at,
	}
}

// TestTwoProfilesSurviveARestartAndStayInactive covers REQ-FUNC-0f6ac82d's
// persistence half: several imports are separate records, they come back after
// a restart, and importing is not connecting.
func TestTwoProfilesSurviveARestartAndStayInactive(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")

	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repository := NewVPNProfileRepository(first)
	for _, name := range []string{"office", "site-b"} {
		if err := repository.Save(ctx, profileRecord(t, "profile-"+name, name)); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := NewVPNProfileRepository(reopened)

	records, err := restarted.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("listed %d profiles after the restart, want 2", len(records))
	}
	for _, record := range records {
		if record.StartSelected {
			t.Fatalf("%s came back selected for startup, which nobody chose", record.Name)
		}
		if record.SecretRef == "" {
			t.Fatalf("%s lost the reference to its key material", record.Name)
		}
		if len(record.Profile.Peers) != 1 || record.Profile.Peers[0].Endpoint != "hub.example:51820" {
			t.Fatalf("%s lost its public configuration: %+v", record.Name, record.Profile)
		}
		if len(record.Profile.Peers[0].AllowedIPs) != 2 {
			t.Fatalf("%s lost its routing intent: %v", record.Name, record.Profile.Peers[0].AllowedIPs)
		}
	}
}

// TestAStoredProfileCarriesNoKeyMaterial covers REQ-SEC-15dcc526 at the
// application database, which is the store an export and a backup both read.
func TestAStoredProfileCarriesNoKeyMaterial(t *testing.T) {
	ctx := context.Background()
	db := openTemporaryDatabase(t)
	if err := NewVPNProfileRepository(db).Save(ctx, profileRecord(t, "profile-office", "office")); err != nil {
		t.Fatalf("save: %v", err)
	}
	rows, err := db.SQL().QueryContext(ctx, "SELECT id, name, public_config_json, secret_ref FROM vpn_profiles")
	if err != nil {
		t.Fatalf("read raw rows: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var identity, name, config, secretRef string
		if err := rows.Scan(&identity, &name, &config, &secretRef); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		for field, value := range map[string]string{
			"public configuration": config, "secret reference": secretRef,
			"identity": identity, "name": name,
		} {
			if strings.Contains(value, storedSentinelKey) {
				t.Fatalf("the stored %s carries the private key: %s", field, value)
			}
			if strings.Contains(strings.ToLower(value), "privatekey") {
				t.Fatalf("the stored %s names a private key field: %s", field, value)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("stored %d rows, want 1", seen)
	}

	// The guard has to still bite. It was loosened once because it matched
	// has_preshared_key, the boolean that exists so the key does not, so this
	// case exists to prove loosening it did not turn it off.
	smuggled := profileRecord(t, "profile-smuggled", "smuggled")
	smuggled.Profile.Addresses = append(smuggled.Profile.Addresses, "PrivateKey = "+storedSentinelKey)
	if err := NewVPNProfileRepository(db).Save(ctx, smuggled); err == nil {
		t.Fatal("a profile carrying raw key material in its public configuration was stored")
	}
	viaField := profileRecord(t, "profile-field", "field")
	viaField.Profile.DNS = append(viaField.Profile.DNS, `{"private_key": "x"}`)
	if err := NewVPNProfileRepository(db).Save(ctx, viaField); err == nil {
		t.Fatal("a profile carrying a private-key field in its public configuration was stored")
	}
}

// TestOnlyOneProfileIsSelectedForStartup covers the startup half of
// REQ-FUNC-16e918cb and REQ-FUNC-0f6ac82d: zero or one, enforced by the schema
// rather than by whoever remembers to clear the previous one.
func TestOnlyOneProfileIsSelectedForStartup(t *testing.T) {
	ctx := context.Background()
	repository := NewVPNProfileRepository(openTemporaryDatabase(t))
	at := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	for _, name := range []string{"office", "site-b"} {
		if err := repository.Save(ctx, profileRecord(t, "profile-"+name, name)); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	if err := repository.SelectStartup(ctx, "profile-office", at); err != nil {
		t.Fatalf("select office: %v", err)
	}
	if err := repository.SelectStartup(ctx, "profile-site-b", at.Add(time.Minute)); err != nil {
		t.Fatalf("select site-b: %v", err)
	}
	records, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	selected := make([]string, 0, 1)
	for _, record := range records {
		if record.StartSelected {
			selected = append(selected, record.Name)
		}
	}
	if len(selected) != 1 || selected[0] != "site-b" {
		t.Fatalf("startup selection = %v, want only site-b", selected)
	}

	if err := repository.ClearStartup(ctx, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("clear: %v", err)
	}
	records, _ = repository.List(ctx)
	for _, record := range records {
		if record.StartSelected {
			t.Fatalf("%s is still selected after the selection was cleared", record.Name)
		}
	}
}

// TestRenamingKeepsTheIdentityAndTheSecretReference covers REQ-FUNC-72e0dc95.
func TestRenamingKeepsTheIdentityAndTheSecretReference(t *testing.T) {
	ctx := context.Background()
	repository := NewVPNProfileRepository(openTemporaryDatabase(t))
	original := profileRecord(t, "profile-office", "office")
	if err := repository.Save(ctx, original); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repository.Save(ctx, profileRecord(t, "profile-site-b", "site-b")); err != nil {
		t.Fatalf("save second: %v", err)
	}

	renamed, err := repository.Rename(ctx, "profile-office", "hall B gateway", time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.ID != original.ID || renamed.SecretRef != original.SecretRef {
		t.Fatalf("rename changed the identity or the secret reference: %+v", renamed)
	}
	if renamed.ConnectionRef != original.ConnectionRef {
		t.Fatalf("rename changed the NetworkManager reference: %q", renamed.ConnectionRef)
	}
	if renamed.Name != "hall B gateway" {
		t.Fatalf("name = %q", renamed.Name)
	}
	if renamed.Revision == original.Revision {
		t.Fatal("an accepted edit did not change the revision")
	}

	// Two profiles cannot share a name, or an operator cannot tell them apart.
	if _, err := repository.Rename(ctx, "profile-site-b", "hall B gateway", time.Now().UTC()); err == nil {
		t.Fatal("a duplicate profile name was accepted")
	}
	if err := repository.Save(ctx, profileRecord(t, "profile-third", "hall B gateway")); err == nil {
		t.Fatal("a duplicate profile name was accepted on import")
	}
}

// TestDeletingAProfileRemovesOnlyThatProfile keeps removal from touching a
// sibling, which is what REQ-FUNC-090be3ae is about at the storage level.
func TestDeletingAProfileRemovesOnlyThatProfile(t *testing.T) {
	ctx := context.Background()
	repository := NewVPNProfileRepository(openTemporaryDatabase(t))
	for _, name := range []string{"office", "site-b"} {
		if err := repository.Save(ctx, profileRecord(t, "profile-"+name, name)); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	if err := repository.Delete(ctx, "profile-office"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	records, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 || records[0].Name != "site-b" {
		t.Fatalf("remaining profiles = %+v, want only site-b", records)
	}
	if err := repository.Delete(ctx, "profile-office"); err == nil {
		t.Fatal("deleting a profile that is not there was reported as success")
	}
}
