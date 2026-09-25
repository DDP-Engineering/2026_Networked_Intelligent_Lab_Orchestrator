//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
	"nilo.local/nilo/internal/maintenance"
)

func TestMaintenanceSnapshotRestorePreservesLinuxFileMetadata(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", dataDir)
	configuration := filepath.Join(dataDir, "configuration", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(configuration), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configuration, []byte("metadata fixture"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(configuration, "user.nilo-test", []byte("preserved"), 0); err != nil {
		t.Fatalf("set fixture xattr: %v", err)
	}
	before, err := os.Stat(configuration)
	if err != nil {
		t.Fatal(err)
	}
	beforeOwner := before.Sys().(*syscall.Stat_t)

	created := mustRunAdmin(t, binary, "", "maintenance", "snapshot", "--data-dir", dataDir, "--transaction", "metadata-001", "--target-version", "0.8.0")
	var receipt struct {
		Snapshot string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &receipt); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(receipt.Snapshot, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Files []struct {
			GID    int               `json:"gid"`
			Path   string            `json:"path"`
			UID    int               `json:"uid"`
			XAttrs map[string]string `json:"xattrs"`
		} `json:"files"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatal(err)
	}
	var recorded bool
	for _, file := range document.Files {
		if file.Path == "configuration/metadata.json" {
			recorded = file.UID == int(beforeOwner.Uid) && file.GID == int(beforeOwner.Gid) && file.XAttrs["user.nilo-test"] != ""
		}
	}
	if !recorded {
		t.Fatalf("snapshot manifest did not retain file ownership and xattrs: %s", manifest)
	}

	if err := os.Chmod(configuration, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(configuration, "user.nilo-test", []byte("changed"), 0); err != nil {
		t.Fatal(err)
	}
	mustRunAdmin(t, binary, "", "maintenance", "restore", "--data-dir", dataDir, "--snapshot", receipt.Snapshot, "--failed-target-version", "0.8.0")
	after, err := os.Stat(configuration)
	if err != nil {
		t.Fatal(err)
	}
	afterOwner := after.Sys().(*syscall.Stat_t)
	if after.Mode().Perm() != 0o640 || afterOwner.Uid != beforeOwner.Uid || afterOwner.Gid != beforeOwner.Gid {
		t.Fatalf("restored metadata = mode %o uid %d gid %d", after.Mode().Perm(), afterOwner.Uid, afterOwner.Gid)
	}
	value := make([]byte, 64)
	length, err := unix.Getxattr(configuration, "user.nilo-test", value)
	if err != nil || string(value[:length]) != "preserved" {
		t.Fatalf("restored xattr = %q, %v", value[:max(length, 0)], err)
	}
}

func TestMaintenancePurgeBackupReuseRejectsMetadataOnlyDrift(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", dataDir)
	configuration := filepath.Join(dataDir, "configuration", "network.json")
	if err := os.MkdirAll(filepath.Dir(configuration), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configuration, []byte("unchanged bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := maintenance.CreatePurgeBackup(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(configuration), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := maintenance.CreatePurgeBackup(dataDir); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("metadata-only drift reused stale backup: %v", err)
	}
}

func TestMaintenanceRestoreRejectsSnapshotMetadataDamage(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "serve-foundation-check", "--data-dir", dataDir)
	configuration := filepath.Join(dataDir, "configuration", "metadata.json")
	if err := os.MkdirAll(filepath.Dir(configuration), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configuration, []byte("metadata fixture"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(configuration, "user.nilo-test", []byte("preserved"), 0); err != nil {
		t.Fatal(err)
	}
	created := mustRunAdmin(t, binary, "", "maintenance", "snapshot", "--data-dir", dataDir, "--transaction", "metadata-damage", "--target-version", "0.8.0")
	var receipt struct {
		Snapshot string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &receipt); err != nil {
		t.Fatal(err)
	}
	snapshotFile := filepath.Join(receipt.Snapshot, "files", "configuration", "metadata.json")
	if err := unix.Setxattr(snapshotFile, "user.nilo-test", []byte("tampered"), 0); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "maintenance", "restore", "--data-dir", dataDir, "--snapshot", receipt.Snapshot, "--failed-target-version", "0.8.0")
	if result.exitCode == 0 || !strings.Contains(result.combined, "metadata mismatch") {
		t.Fatalf("metadata-damaged restore result = %#v", result)
	}
}
