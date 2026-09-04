package nethelper

import (
	"errors"

	"nilo.local/nilo/internal/network"
)

// ErrKeystoreUnavailable means this helper was started without key storage, so
// it refuses to take custody of key material rather than accepting it and
// dropping it.
var ErrKeystoreUnavailable = errors.New("this helper has no key storage configured")

// ConfigureKeystore attaches root-owned key storage. It is called once during
// helper startup, from local configuration, never over the socket.
func (e *Engine) ConfigureKeystore(keystore *Keystore) error {
	if keystore == nil {
		return errors.New("key storage is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.keystore != nil {
		return errors.New("key storage is already configured")
	}
	e.keystore = keystore
	return nil
}

func (e *Engine) keys() (*Keystore, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.keysLocked()
}

// keysLocked is keys for a caller that already holds the engine lock, such as
// Prepare and Apply, which must not take it twice.
func (e *Engine) keysLocked() (*Keystore, error) {
	if e.keystore == nil {
		return nil, ErrKeystoreUnavailable
	}
	return e.keystore, nil
}

// StageSecrets takes custody of an import's key material and returns the opaque
// handle that stands in for it everywhere else.
func (e *Engine) StageSecrets(payload SecretPayload) (string, error) {
	keystore, err := e.keys()
	if err != nil {
		return "", err
	}
	return keystore.Stage(network.Secrets{
		PrivateKey:    payload.PrivateKey,
		PresharedKeys: payload.PresharedKeys,
		Password:      payload.Password,
	}, e.now().UTC())
}

// PromoteSecrets binds staged key material to a profile identity.
func (e *Engine) PromoteSecrets(handle, profileID string) error {
	keystore, err := e.keys()
	if err != nil {
		return err
	}
	return keystore.Promote(handle, profileID)
}

// DiscardSecrets throws staged key material away.
func (e *Engine) DiscardSecrets(handle string) error {
	keystore, err := e.keys()
	if err != nil {
		return err
	}
	return keystore.Discard(handle)
}

// SweepStagedSecrets removes imports nobody finished. The helper runs it at
// startup, because an import interrupted by a crash has no one left to discard
// it.
func (e *Engine) SweepStagedSecrets() (int, error) {
	keystore, err := e.keys()
	if errors.Is(err, ErrKeystoreUnavailable) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return keystore.ExpireStaged(e.now().UTC().Add(-stagedImportLifetime))
}
