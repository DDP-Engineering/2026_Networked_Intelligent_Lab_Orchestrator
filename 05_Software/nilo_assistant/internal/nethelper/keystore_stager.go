package nethelper

import (
	"context"
	"time"

	"nilo.local/nilo/internal/network"
)

// KeystoreStager lets the operation lifecycle stage an import's key material
// without being able to read it back.
//
// The asymmetry is the point: this type exposes Stage, Promote and Discard and
// deliberately does not expose Read. The unprivileged core can put key material
// beyond its own reach and can bind it to a profile, and that is all it can do
// with it. Reading is the privileged side's, and only in order to hand the value
// to NetworkManager's own secret storage.
type KeystoreStager struct {
	keystore *Keystore
	now      func() time.Time
}

// NewKeystoreStager binds a stager to an open keystore.
func NewKeystoreStager(keystore *Keystore, now func() time.Time) *KeystoreStager {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &KeystoreStager{keystore: keystore, now: now}
}

func (stager *KeystoreStager) StageSecrets(_ context.Context, secrets network.Secrets) (string, error) {
	return stager.keystore.Stage(secrets, stager.now().UTC())
}

func (stager *KeystoreStager) BindSecrets(_ context.Context, handle, profileID string) error {
	return stager.keystore.Promote(handle, profileID)
}

func (stager *KeystoreStager) DropSecrets(_ context.Context, handle string) error {
	return stager.keystore.Discard(handle)
}

var _ network.SecretStager = (*KeystoreStager)(nil)
