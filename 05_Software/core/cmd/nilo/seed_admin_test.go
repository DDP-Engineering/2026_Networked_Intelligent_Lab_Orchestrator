package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const seededAdministratorPassword = "file-seeded-administrator-1"

func TestSeedAdministratorConsumesAProtectedPasswordFile(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	passwordFile := filepath.Join(t.TempDir(), "administrator.password")
	if err := os.WriteFile(passwordFile, []byte(seededAdministratorPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := runAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
		"--username", "admin", "--password-file", passwordFile)
	if result.exitCode != 0 {
		t.Fatalf("seed-admin exited %d: %s", result.exitCode, result.combined)
	}
	if strings.Contains(result.combined, seededAdministratorPassword) {
		t.Fatalf("seed-admin disclosed the password: %s", result.combined)
	}
	if _, err := os.Lstat(passwordFile); !os.IsNotExist(err) {
		t.Fatalf("password source remains after durable bootstrap: %v", err)
	}
	users := listUsers(t, dataDir)
	if len(users) != 1 || users[0].Username != "admin" || users[0].Role != "administrator" {
		t.Fatalf("seeded identities = %#v", users)
	}
}

func TestSeedAdministratorRefusesUnsafeOrAlreadyConsumedInputs(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	unsafeFile := filepath.Join(t.TempDir(), "unsafe.password")
	if err := os.WriteFile(unsafeFile, []byte(seededAdministratorPassword+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		result := runAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
			"--username", "admin", "--password-file", unsafeFile)
		if result.exitCode == 0 || !strings.Contains(result.combined, "group or other permissions") {
			t.Fatalf("unsafe password file result = %#v", result)
		}
		if _, err := os.Stat(unsafeFile); err != nil {
			t.Fatalf("refused input was consumed: %v", err)
		}
	}

	protected := filepath.Join(t.TempDir(), "protected.password")
	if err := os.WriteFile(protected, []byte(seededAdministratorPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRunAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
		"--username", "admin", "--password-file", protected)
	second := filepath.Join(t.TempDir(), "second.password")
	if err := os.WriteFile(second, []byte("second-file-administrator-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := runAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
		"--username", "second", "--password-file", second)
	if refused.exitCode == 0 || !strings.Contains(refused.combined, "administrator already exists") {
		t.Fatalf("second seed result = %#v", refused)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("refused second input was consumed: %v", err)
	}
}

func TestSeedAdministratorRefusesASymlinkedPasswordFile(t *testing.T) {
	binary := buildNilo(t)
	target := filepath.Join(t.TempDir(), "target.password")
	if err := os.WriteFile(target, []byte(seededAdministratorPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "password.link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result := runAdmin(t, binary, "", "seed-admin", "--data-dir", t.TempDir(),
		"--username", "admin", "--password-file", link)
	if result.exitCode == 0 || !strings.Contains(result.combined, "regular non-symlink") {
		t.Fatalf("symlink password file result = %#v", result)
	}
}

func TestSeedAdministratorIsIdempotentForTheSameEnabledAdministrator(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	first := filepath.Join(t.TempDir(), "first.password")
	if err := os.WriteFile(first, []byte(seededAdministratorPassword+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRunAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
		"--username", "admin", "--password-file", first)
	second := filepath.Join(t.TempDir(), "second.password")
	if err := os.WriteFile(second, []byte("different-but-unused-password-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "seed-admin", "--data-dir", dataDir,
		"--username", "admin", "--password-file", second)
	if result.exitCode != 0 {
		t.Fatalf("idempotent seed result = %#v", result)
	}
	if _, err := os.Lstat(second); !os.IsNotExist(err) {
		t.Fatalf("idempotent seed did not consume the recreated source: %v", err)
	}
	if users := listUsers(t, dataDir); len(users) != 1 || users[0].Username != "admin" || !users[0].Enabled() {
		t.Fatalf("idempotent identities = %#v", users)
	}
}

func TestSeedAdministratorBoundsTheProtectedFileBeforeBootstrap(t *testing.T) {
	binary := buildNilo(t)
	passwordFile := filepath.Join(t.TempDir(), "oversized.password")
	if err := os.WriteFile(passwordFile, append(make([]byte, 1025), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runAdmin(t, binary, "", "seed-admin", "--data-dir", t.TempDir(),
		"--username", "admin", "--password-file", passwordFile)
	if result.exitCode == 0 || !strings.Contains(result.combined, "at most 1024 bytes") {
		t.Fatalf("oversized seed result = %#v", result)
	}
	if _, err := os.Stat(passwordFile); err != nil {
		t.Fatalf("oversized password source was consumed: %v", err)
	}
}
