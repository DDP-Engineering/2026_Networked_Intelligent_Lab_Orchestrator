package nethelper

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// importingChanger is a provider that accepts an import and assigns it an
// identity, which is the half of the lifecycle the keystore binds to.
type importingChanger struct {
	assigned string
	applyErr error
	seen     []network.Change
	// bind is what a real importer does with the staged handle: the privileged
	// side promotes it to the identity it assigns, atomically with creating
	// the profile.
	bind func(handle, profileID string) error
}

func (changer *importingChanger) Inspect(_ context.Context, change network.Change) (map[string]string, error) {
	return map[string]string{"target": change.Target}, nil
}

func (changer *importingChanger) Observe(
	ctx context.Context, action network.Action, target string,
) (map[string]string, error) {
	return changer.Inspect(ctx, network.Change{Action: action, Target: target})
}

func (changer *importingChanger) ApplyChange(_ context.Context, change network.Change) error {
	changer.seen = append(changer.seen, change)
	return changer.applyErr
}

func (changer *importingChanger) ImportProfile(ctx context.Context, change network.Change) (string, error) {
	if err := changer.ApplyChange(ctx, change); err != nil {
		return "", err
	}
	if changer.bind != nil {
		if err := changer.bind(change.SecretHandle, changer.assigned); err != nil {
			return "", err
		}
	}
	return changer.assigned, nil
}

func (changer *importingChanger) ArmRecovery(context.Context, string, time.Duration) (network.Checkpoint, error) {
	return network.Checkpoint{}, nil
}

func (changer *importingChanger) Apply(context.Context, network.Action, string) error { return nil }

func (changer *importingChanger) Restore(context.Context, network.Checkpoint) (bool, error) {
	return true, nil
}

func (changer *importingChanger) Discard(context.Context, network.Checkpoint) error { return nil }

func newImportHarness(t *testing.T, changer *importingChanger) (*network.OperationManager, *Keystore) {
	t.Helper()
	keystore, _ := newKeystore(t)
	changer.bind = keystore.Promote
	counter := 0
	manager, err := network.NewOperationManager(network.OperationManagerOptions{
		Changer:  changer,
		Observer: changer,
		Now:      func() time.Time { return time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC) },
		NewID: func() string {
			counter++
			return "import-operation"
		},
		Stager: NewKeystoreStager(keystore, func() time.Time {
			return time.Date(2026, 9, 3, 19, 0, 0, 0, time.UTC)
		}),
	})
	if err != nil {
		t.Fatalf("build operation manager: %v", err)
	}
	return manager, keystore
}

func importText(t *testing.T) string {
	t.Helper()
	return "[Interface]\nPrivateKey = " + sentinelPrivate + "\nAddress = 10.60.0.2/32\n" +
		"[Peer]\nPublicKey = " + sentinelPublic + "\nPresharedKey = " + sentinelPreshared + "\n" +
		"AllowedIPs = 10.60.0.0/16\nEndpoint = hub.example:51820\n"
}

// TestAnImportPutsItsKeysInTheKeystoreAndNowhereElse is the whole point of the
// staging seam: the uploaded file is consumed by the preview, its key material
// ends up only in root-owned storage, and the operation carries a handle that
// is not a secret.
func TestAnImportPutsItsKeysInTheKeystoreAndNowhereElse(t *testing.T) {
	ctx := context.Background()
	changer := &importingChanger{assigned: "profile-office"}
	manager, keystore := newImportHarness(t, changer)

	previewed, err := manager.PreviewChange(ctx, "actor-1", network.Change{
		Action: network.ActionProfileImport, Target: "office", ProfileText: importText(t),
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// The preview is what a caller gets back, and what a stored operation is
	// built from. Neither may carry the upload or the key.
	encoded, err := json.Marshal(previewed)
	if err != nil {
		t.Fatalf("encode preview: %v", err)
	}
	for _, sentinel := range []string{sentinelPrivate, sentinelPreshared, "PrivateKey"} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("the preview carries %q: %s", sentinel, encoded)
		}
	}

	if _, err := manager.Acknowledge(ctx, "actor-1", previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if len(changer.seen) != 1 {
		t.Fatalf("the provider saw %d changes, want 1", len(changer.seen))
	}
	applied := changer.seen[0]
	if applied.ProfileText != "" {
		t.Fatal("the uploaded profile text reached the provider")
	}
	if applied.Profile == nil || len(applied.Profile.Peers) != 1 {
		t.Fatalf("the provider did not receive the parsed public configuration: %+v", applied.Profile)
	}
	if !applied.Profile.Peers[0].HasPresharedKey {
		t.Fatal("the public configuration lost the fact that a preshared key was supplied")
	}
	if applied.SecretHandle == "" {
		t.Fatal("the provider received no handle, so nothing could be bound")
	}

	// The key material is now the profile's, readable only through the keystore.
	stored, err := keystore.Read("profile-office")
	if err != nil {
		t.Fatalf("read promoted secrets: %v", err)
	}
	if stored.PrivateKey != sentinelPrivate {
		t.Fatal("the promoted private key is not the imported one")
	}
	if len(stored.PresharedKeys) != 1 || stored.PresharedKeys[0] != sentinelPreshared {
		t.Fatalf("the preshared key lost its peer alignment: %v", len(stored.PresharedKeys))
	}

	// The handle is spent. Replaying it must not bind the same material again.
	if err := keystore.Promote(applied.SecretHandle, "profile-elsewhere"); err == nil {
		t.Fatal("a spent handle was promoted a second time")
	}
}

// TestAFailedImportLeavesNoStagedKeyMaterial covers the other side: an import
// that did not happen must not leave a key waiting on disk.
func TestAFailedImportLeavesNoStagedKeyMaterial(t *testing.T) {
	ctx := context.Background()
	changer := &importingChanger{assigned: "profile-office", applyErr: errors.New("the tunnel could not be created")}
	manager, keystore := newImportHarness(t, changer)

	previewed, err := manager.PreviewChange(ctx, "actor-1", network.Change{
		Action: network.ActionProfileImport, Target: "office", ProfileText: importText(t),
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	staged := stagedFiles(t, keystore)
	if len(staged) != 1 {
		t.Fatalf("%d staged records after the preview, want 1", len(staged))
	}

	if _, err := manager.Acknowledge(ctx, "actor-1", previewed.ID, previewed.Revision); err == nil {
		t.Fatal("a failing import was reported as successful")
	}
	if remaining := stagedFiles(t, keystore); len(remaining) != 0 {
		t.Fatalf("%d staged records survived a failed import, want none", len(remaining))
	}
	if _, err := keystore.Read("profile-office"); err == nil {
		t.Fatal("a failed import still bound key material to a profile")
	}
}

func stagedFiles(t *testing.T, keystore *Keystore) []string {
	t.Helper()
	entries, err := os.ReadDir(keystore.directory)
	if err != nil {
		t.Fatalf("read keystore: %v", err)
	}
	staged := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagedPrefix) {
			staged = append(staged, entry.Name())
		}
	}
	return staged
}
