package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceSnapshotAndRestoreVerifyApplicationState(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", dataDir)
	database := filepath.Join(dataDir, "nilo.db")
	configuration := filepath.Join(dataDir, "configuration", "network.json")
	if err := os.MkdirAll(filepath.Dir(configuration), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configuration, []byte("original-configuration"), 0o640); err != nil {
		t.Fatal(err)
	}
	originalDatabase, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}

	created := mustRunAdmin(t, binary, "", "maintenance", "snapshot", "--data-dir", dataDir, "--transaction", "upgrade-001", "--target-version", "0.8.0")
	var receipt struct {
		Snapshot string `json:"snapshot"`
		Files    int    `json:"files"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &receipt); err != nil {
		t.Fatalf("decode snapshot receipt: %v", err)
	}
	if receipt.Files < 2 || receipt.Snapshot != filepath.Join(dataDir, "snapshots", "upgrade-001") {
		t.Fatalf("snapshot receipt = %#v", receipt)
	}
	if err := os.WriteFile(database, []byte("changed"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(configuration); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(dataDir, "post-snapshot.tmp")
	if err := os.WriteFile(extra, []byte("must be removed"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrongTarget := runAdmin(t, binary, "", "maintenance", "restore", "--data-dir", dataDir, "--snapshot", receipt.Snapshot, "--failed-target-version", "0.8.1")
	if wrongTarget.exitCode == 0 || !strings.Contains(wrongTarget.combined, "failed target version") {
		t.Fatalf("wrong-target restore result = %#v", wrongTarget)
	}
	mustRunAdmin(t, binary, "", "maintenance", "restore", "--data-dir", dataDir, "--snapshot", receipt.Snapshot, "--failed-target-version", "0.8.0")
	if got, _ := os.ReadFile(database); string(got) != string(originalDatabase) {
		t.Fatalf("restored database digest changed")
	}
	if got, _ := os.ReadFile(configuration); string(got) != "original-configuration" {
		t.Fatalf("restored configuration = %q", got)
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatalf("restore retained post-snapshot state: %v", err)
	}

	manifest := filepath.Join(receipt.Snapshot, "snapshot.json")
	contents, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		SourceVersion string `json:"source_version"`
		TargetVersion string `json:"target_version"`
	}
	if err := json.Unmarshal(contents, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.SourceVersion == "" || metadata.TargetVersion != "0.8.0" {
		t.Fatalf("snapshot version metadata = %#v", metadata)
	}
	if err := os.WriteFile(manifest, append(contents, byte('x')), 0o640); err != nil {
		t.Fatal(err)
	}
	refused := runAdmin(t, binary, "", "maintenance", "restore", "--data-dir", dataDir, "--snapshot", receipt.Snapshot, "--failed-target-version", "0.8.0")
	if refused.exitCode == 0 || !strings.Contains(refused.combined, "snapshot manifest") {
		t.Fatalf("tampered restore result = %#v", refused)
	}
}

func TestMaintenanceSnapshotRefusesANonNILODatabase(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "nilo.db"), []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "maintenance", "snapshot", "--data-dir", dataDir, "--transaction", "unsafe")
	if result.exitCode == 0 || !strings.Contains(result.combined, "not a NILO SQLite database") {
		t.Fatalf("non-NILO snapshot result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "snapshots", "unsafe")); !os.IsNotExist(err) {
		t.Fatalf("invalid database produced a snapshot: %v", err)
	}
}

func TestMaintenanceInventoryRemoveAndConfirmedPurge(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", dataDir)
	inventory := mustRunAdmin(t, binary, "", "maintenance", "inventory", "--data-dir", dataDir)
	if !strings.Contains(inventory.stdout, `"path":"nilo.db"`) {
		t.Fatalf("inventory = %q", inventory.stdout)
	}
	preserved := mustRunAdmin(t, binary, "", "maintenance", "remove", "--data-dir", dataDir)
	if !strings.Contains(preserved.stdout, `"state":"preserved"`) {
		t.Fatalf("remove result = %q", preserved.stdout)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "nilo.db")); err != nil {
		t.Fatalf("remove did not preserve state: %v", err)
	}
	refused := runAdmin(t, binary, "", "maintenance", "purge", "--data-dir", dataDir)
	if refused.exitCode == 0 || !strings.Contains(refused.combined, "--confirm-local") {
		t.Fatalf("unconfirmed purge = %#v", refused)
	}
	mustRunAdmin(t, binary, "", "maintenance", "purge", "--data-dir", dataDir, "--confirm-local")
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("purged directory contains %v", entries)
	}
}

func TestMaintenancePurgeRefusesAnUnownedDirectory(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	foreign := filepath.Join(dataDir, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("not NILO state"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "maintenance", "purge", "--data-dir", dataDir, "--confirm-local")
	if result.exitCode == 0 || !strings.Contains(result.combined, "NILO-owned") {
		t.Fatalf("unowned purge result = %#v", result)
	}
	if contents, err := os.ReadFile(foreign); err != nil || string(contents) != "not NILO state" {
		t.Fatalf("unowned content changed: %q, %v", contents, err)
	}
}

func TestMaintenancePurgeRefusesAClonedOwnershipMarkerAtAnotherPath(t *testing.T) {
	binary := buildNilo(t)
	original := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", original)
	clone := filepath.Join(t.TempDir(), "clone")
	if err := os.CopyFS(clone, os.DirFS(original)); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "maintenance", "purge", "--data-dir", clone, "--confirm-local")
	if result.exitCode == 0 || !strings.Contains(result.combined, "ownership marker") {
		t.Fatalf("cloned ownership purge result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(clone, "nilo.db")); err != nil {
		t.Fatalf("refused cloned purge changed the database: %v", err)
	}
}
