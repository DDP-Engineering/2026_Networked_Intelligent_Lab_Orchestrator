package nethelper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// The sentinels are disposable values that exist only to be searched for. They
// are not real keys and are not derived from any.
//
// They are computed rather than written out, because the profile parser checks
// the key encoding: a hand-padded literal that is one character short is
// refused, and a test that never gets past the parser proves nothing about
// where secrets end up.
var (
	sentinelPrivate   = sentinelKey("SENTINELprivate")
	sentinelPreshared = sentinelKey("SENTINELpreshared")
	sentinelPublic    = sentinelKey("SENTINELpublic")
)

// sentinelKey builds a well-formed 32-byte base64 key carrying a readable
// label. Padding with 'A' keeps the final character canonical.
func sentinelKey(label string) string {
	const encodedLength = 43
	return label + strings.Repeat("A", encodedLength-len(label)) + "="
}

func newKeystore(t *testing.T) (*Keystore, string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatalf("tighten the keystore directory: %v", err)
	}
	keystore, err := NewKeystore(directory)
	if err != nil {
		t.Fatalf("open keystore: %v", err)
	}
	return keystore, directory
}

func sentinelSecrets() network.Secrets {
	return network.Secrets{
		PrivateKey:    sentinelPrivate,
		PresharedKeys: []string{sentinelPreshared, ""},
	}
}

// TestProfileKeystoreStagesThenPromotes covers REQ-SEC-15dcc526's storage half:
// key material is written once, to root-owned storage, under an opaque handle
// that is not itself a secret, and is bound to a profile only afterwards.
func TestProfileKeystoreStagesThenPromotes(t *testing.T) {
	keystore, directory := newKeystore(t)
	now := time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC)

	handle, err := keystore.Stage(sentinelSecrets(), now)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if handle == "" {
		t.Fatal("staging returned no handle")
	}
	if strings.Contains(handle, sentinelPrivate) || strings.Contains(handle, sentinelPreshared) {
		t.Fatal("the handle carries the key material it is supposed to stand in for")
	}
	if len(handle) < 32 {
		t.Fatalf("handle %q is short enough to guess", handle)
	}

	if err := keystore.Promote(handle, "profile-1"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	// A handle is single use. Replaying it must not rebind the same material to
	// a second profile.
	if err := keystore.Promote(handle, "profile-2"); err == nil {
		t.Fatal("a handle was promoted twice")
	}

	read, err := keystore.Read("profile-1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.PrivateKey != sentinelPrivate {
		t.Fatalf("private key did not round trip")
	}
	if len(read.PresharedKeys) != 2 || read.PresharedKeys[0] != sentinelPreshared || read.PresharedKeys[1] != "" {
		t.Fatalf("preshared keys lost their peer alignment: %d entries", len(read.PresharedKeys))
	}

	if _, err := keystore.Read("profile-2"); err == nil {
		t.Fatal("a profile that was never promoted has secrets")
	}
	if err := keystore.Remove("profile-1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := keystore.Read("profile-1"); err == nil {
		t.Fatal("removing a profile left its key material behind")
	}
	assertNoLooseSecrets(t, directory)
}

// TestProfileKeystoreKeepsFilesPrivate covers the permission half of
// REQ-SEC-15dcc526.
func TestProfileKeystoreKeepsFilesPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are the subject of this test")
	}
	keystore, directory := newKeystore(t)
	handle, err := keystore.Stage(sentinelSecrets(), time.Now().UTC())
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := keystore.Promote(handle, "profile-1"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the keystore wrote nothing")
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat %s: %v", entry.Name(), err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s has mode %v, want no group or other access", entry.Name(), info.Mode().Perm())
		}
	}
}

// TestProfileKeystoreRefusesAnUnsafeLocationOrIdentity keeps a caller from
// pointing the keystore somewhere else or escaping it through a profile name.
func TestProfileKeystoreRefusesAnUnsafeLocationOrIdentity(t *testing.T) {
	if _, err := NewKeystore("relative/path"); err == nil {
		t.Fatal("a relative keystore directory was accepted")
	}
	if _, err := NewKeystore(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing keystore directory was accepted")
	}
	if runtime.GOOS != "windows" {
		loose := t.TempDir()
		if err := os.Chmod(loose, 0o755); err != nil {
			t.Fatalf("loosen: %v", err)
		}
		if _, err := NewKeystore(loose); err == nil {
			t.Fatal("a world-readable keystore directory was accepted")
		}
	}

	keystore, _ := newKeystore(t)
	handle, err := keystore.Stage(sentinelSecrets(), time.Now().UTC())
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	for _, identity := range []string{"", "../escape", "a/b", `a\b`, strings.Repeat("x", 200)} {
		if err := keystore.Promote(handle, identity); err == nil {
			t.Fatalf("profile identity %q was accepted", identity)
		}
	}
	for _, bad := range []string{"", "../escape", "not-a-handle"} {
		if err := keystore.Promote(bad, "profile-1"); err == nil {
			t.Fatalf("handle %q was accepted", bad)
		}
	}
}

// TestProfileKeystoreExpiresAbandonedImports covers the abandoned-import half
// of the plan's secret lifecycle: an import nobody acknowledged does not leave
// key material on disk forever.
func TestProfileKeystoreExpiresAbandonedImports(t *testing.T) {
	keystore, directory := newKeystore(t)
	staged := time.Date(2026, 9, 3, 13, 0, 0, 0, time.UTC)

	abandoned, err := keystore.Stage(sentinelSecrets(), staged)
	if err != nil {
		t.Fatalf("stage abandoned: %v", err)
	}
	kept, err := keystore.Stage(sentinelSecrets(), staged.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("stage kept: %v", err)
	}

	removed, err := keystore.ExpireStaged(staged.Add(15 * time.Minute))
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expired %d staged imports, want 1", removed)
	}
	if err := keystore.Promote(abandoned, "profile-1"); err == nil {
		t.Fatal("an expired import was still promotable")
	}
	if err := keystore.Promote(kept, "profile-2"); err != nil {
		t.Fatalf("a staged import inside its window was refused: %v", err)
	}
	assertNoLooseSecrets(t, directory)
}

// TestSecretsNeverReachAHelperResponse covers REQ-SEC-15dcc526 at the boundary
// that matters most: the socket. The keystore lives on the privileged side and
// nothing it holds may cross.
func TestSecretsNeverReachAHelperResponse(t *testing.T) {
	keystore, _ := newKeystore(t)
	handle, err := keystore.Stage(sentinelSecrets(), time.Now().UTC())
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := keystore.Promote(handle, "profile-1"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	secrets, err := keystore.Read("profile-1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A response built around the profile the secrets belong to.
	profile, _, err := network.ParseProfile("office", []byte(
		"[Interface]\nPrivateKey = "+secrets.PrivateKey+"\nAddress = 10.60.0.2/32\n"+
			"[Peer]\nPublicKey = "+sentinelPublic+"\n"+
			"AllowedIPs = 10.60.0.0/16\nEndpoint = hub.example:51820\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	response := Response{WireGuard: &network.WireGuardStatus{
		Reported: true,
		Profiles: []network.ProfileStatus{{ID: "profile-1", Name: "office", NILOOwned: true, Profile: profile}},
	}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	for _, sentinel := range []string{sentinelPrivate, sentinelPreshared, handle} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("a helper response carries %q", sentinel)
		}
	}
	// The same has to hold of the formatted forms a log line would use.
	for _, rendered := range []string{secrets.String(), profile.Name} {
		if strings.Contains(rendered, sentinelPrivate) {
			t.Fatalf("a rendered value carries the private key: %s", rendered)
		}
	}
}

// assertNoLooseSecrets fails if any sentinel is left in a file the keystore was
// supposed to have removed.
func assertNoLooseSecrets(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), sentinelPrivate) && !strings.HasPrefix(entry.Name(), "profile-") {
			t.Fatalf("%s still holds key material", entry.Name())
		}
	}
}
