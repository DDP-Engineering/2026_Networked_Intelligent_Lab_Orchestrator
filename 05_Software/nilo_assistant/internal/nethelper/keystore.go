package nethelper

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nilo.local/nilo/internal/network"
)

// The keystore is the only place WireGuard key material is written, and it
// lives on the privileged side of the socket. Nothing it holds is ever put in a
// Response, a log line, the application database or an export: the unprivileged
// core knows profiles by identity and never by content.
//
// An import happens in two steps because an acknowledgement happens in two
// steps. The uploaded file is parsed transiently, its key material is staged
// under an opaque handle, and only a successful acknowledgement promotes that
// handle to a profile. A handle is not a secret, is single use, and expires, so
// an import nobody finished does not leave key material on disk.

const (
	// secretsFormat versions the on-disk encoding. Key material is written field
	// by field rather than through JSON, because network.Secrets deliberately
	// redacts itself when marshalled and a round trip through it would store the
	// word "redacted".
	secretsFormat = "nilo-wireguard-secrets/1"
	// stagedPrefix and profilePrefix keep the two lifetimes apart on disk, so
	// expiry can never reach a promoted profile.
	stagedPrefix  = "staged-"
	profilePrefix = "profile-"
	secretsSuffix = ".secret"
	// maximumSecretsBytes bounds one stored record. A profile's keys are a few
	// hundred bytes; anything larger is a mistake or an attempt.
	maximumSecretsBytes = 64 << 10
	// emptyValue stands in for a peer with no preshared key, so the stored list
	// keeps its alignment with the profile's peers.
	emptyValue = "-"
	// stagedImportLifetime bounds how long an unfinished import keeps its key
	// material on disk. It is generous enough for an operator to read a preview
	// and decide, and short enough that an abandoned one does not persist.
	stagedImportLifetime = 30 * time.Minute
)

// ErrSecretsNotFound means no key material is stored under that identity.
var ErrSecretsNotFound = errors.New("no stored key material for this identity")

// Keystore owns a private helper directory of key material.
type Keystore struct{ directory string }

// NewKeystore opens an existing private directory. It never creates one: the
// directory and its ownership are established by installation, which is where
// a decision about privilege belongs.
func NewKeystore(directory string) (*Keystore, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("keystore directory must be absolute")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("keystore directory must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("keystore directory must have mode 0700")
	}
	return &Keystore{directory: directory}, nil
}

// Stage writes key material under a fresh opaque handle. The handle stands in
// for the secret in a preview and an acknowledgement, so neither ever carries
// the value itself.
func (keystore *Keystore) Stage(secrets network.Secrets, now time.Time) (string, error) {
	if keystore == nil {
		return "", errors.New("keystore is required")
	}
	if strings.TrimSpace(secrets.PrivateKey) == "" && secrets.Password == "" {
		return "", errors.New("nothing to stage: neither a private key nor a password")
	}
	if strings.TrimSpace(secrets.PrivateKey) != "" && secrets.Password != "" {
		return "", errors.New("a staged record carries either tunnel keys or a password")
	}
	if strings.ContainsAny(secrets.Password, "\r\n\x00") {
		return "", errors.New("a password cannot contain line breaks")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("create import handle: %w", err)
	}
	handle := hex.EncodeToString(raw)
	if err := keystore.write(keystore.path(stagedPrefix, handle), secrets, now); err != nil {
		return "", err
	}
	return handle, nil
}

// Promote binds staged key material to a profile identity. It is a rename, so
// the material is never held in two places and the handle stops working.
func (keystore *Keystore) Promote(handle, profileID string) error {
	if err := validateHandle(handle); err != nil {
		return err
	}
	if err := validateIdentity(profileID); err != nil {
		return err
	}
	staged := keystore.path(stagedPrefix, handle)
	if _, err := os.Lstat(staged); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: this import is no longer staged", ErrSecretsNotFound)
		}
		return err
	}
	if err := os.Rename(staged, keystore.path(profilePrefix, profileID)); err != nil {
		return fmt.Errorf("promote staged key material: %w", err)
	}
	return keystore.syncDirectory()
}

// Discard removes staged key material an import no longer needs.
func (keystore *Keystore) Discard(handle string) error {
	if err := validateHandle(handle); err != nil {
		return err
	}
	return keystore.remove(keystore.path(stagedPrefix, handle))
}

// Remove deletes a profile's key material. It is called when the profile is
// deleted, so removing a profile removes its secret rather than orphaning it.
func (keystore *Keystore) Remove(profileID string) error {
	if err := validateIdentity(profileID); err != nil {
		return err
	}
	return keystore.remove(keystore.path(profilePrefix, profileID))
}

// Read returns a profile's key material. Only the privileged side calls it, and
// only to hand the value to NetworkManager's own secret storage.
func (keystore *Keystore) Read(profileID string) (network.Secrets, error) {
	if err := validateIdentity(profileID); err != nil {
		return network.Secrets{}, err
	}
	return keystore.read(keystore.path(profilePrefix, profileID))
}

// ExpireStaged removes every staged import older than the cutoff and reports
// how many it removed, so an abandoned import cannot outlive the session that
// created it.
func (keystore *Keystore) ExpireStaged(before time.Time) (int, error) {
	if keystore == nil {
		return 0, errors.New("keystore is required")
	}
	entries, err := os.ReadDir(keystore.directory)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), stagedPrefix) || !strings.HasSuffix(entry.Name(), secretsSuffix) {
			continue
		}
		path := filepath.Join(keystore.directory, entry.Name())
		staged, err := keystore.stagedAt(path)
		if err != nil {
			// A staged record this helper cannot read is a record it cannot
			// promote either, so removing it is the safe reading.
			if removeErr := keystore.remove(path); removeErr != nil {
				return removed, removeErr
			}
			removed++
			continue
		}
		if staged.Before(before) {
			if err := keystore.remove(path); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

func (keystore *Keystore) path(prefix, identity string) string {
	return filepath.Join(keystore.directory, prefix+identity+secretsSuffix)
}

func (keystore *Keystore) remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return keystore.syncDirectory()
}

// write stores one record atomically with private permissions.
func (keystore *Keystore) write(path string, secrets network.Secrets, now time.Time) error {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "%s\n", secretsFormat)
	fmt.Fprintf(&buffer, "staged %s\n", now.UTC().Format(time.RFC3339Nano))
	if secrets.Password != "" {
		fmt.Fprintf(&buffer, "password %s\n", secrets.Password)
	} else {
		fmt.Fprintf(&buffer, "private %s\n", secrets.PrivateKey)
	}
	for _, preshared := range secrets.PresharedKeys {
		if preshared == "" {
			preshared = emptyValue
		}
		fmt.Fprintf(&buffer, "preshared %s\n", preshared)
	}
	if buffer.Len() > maximumSecretsBytes {
		return errors.New("key material exceeds the stored bound")
	}

	file, err := os.CreateTemp(keystore.directory, ".secret-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(buffer.Bytes()); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return keystore.syncDirectory()
}

func (keystore *Keystore) open(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrSecretsNotFound, filepath.Base(path))
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("stored key material must be a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("stored key material must be private")
	}
	if info.Size() > maximumSecretsBytes {
		return nil, errors.New("stored key material exceeds the bound")
	}
	return os.ReadFile(path)
}

func (keystore *Keystore) read(path string) (network.Secrets, error) {
	data, err := keystore.open(path)
	if err != nil {
		return network.Secrets{}, err
	}
	secrets, _, err := decodeSecrets(data)
	return secrets, err
}

func (keystore *Keystore) stagedAt(path string) (time.Time, error) {
	data, err := keystore.open(path)
	if err != nil {
		return time.Time{}, err
	}
	_, staged, err := decodeSecrets(data)
	return staged, err
}

func decodeSecrets(data []byte) (network.Secrets, time.Time, error) {
	var (
		secrets network.Secrets
		staged  time.Time
	)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	if !scanner.Scan() || scanner.Text() != secretsFormat {
		return network.Secrets{}, time.Time{}, errors.New("stored key material is not in a format this helper wrote")
	}
	for scanner.Scan() {
		field, value, found := strings.Cut(scanner.Text(), " ")
		if !found {
			return network.Secrets{}, time.Time{}, errors.New("stored key material is malformed")
		}
		switch field {
		case "staged":
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return network.Secrets{}, time.Time{}, errors.New("stored key material has no usable staging time")
			}
			staged = parsed.UTC()
		case "private":
			secrets.PrivateKey = value
		case "password":
			secrets.Password = value
		case "preshared":
			if value == emptyValue {
				value = ""
			}
			secrets.PresharedKeys = append(secrets.PresharedKeys, value)
		default:
			return network.Secrets{}, time.Time{}, fmt.Errorf("unknown stored key field %q", field)
		}
	}
	if err := scanner.Err(); err != nil {
		return network.Secrets{}, time.Time{}, err
	}
	if secrets.PrivateKey == "" && secrets.Password == "" {
		return network.Secrets{}, time.Time{}, errors.New("stored key material has neither a private key nor a password")
	}
	return secrets, staged, nil
}

// ReadStaged returns staged material by its handle. Only the privileged side
// calls it, at the moment the material is consumed - a wireless password is
// handed to NetworkManager and the staged copy discarded in the same step.
func (keystore *Keystore) ReadStaged(handle string) (network.Secrets, error) {
	if err := validateHandle(handle); err != nil {
		return network.Secrets{}, err
	}
	return keystore.read(keystore.path(stagedPrefix, handle))
}

// validateHandle refuses anything that is not one of our own handles, which is
// what keeps a caller from naming a path instead.
func validateHandle(handle string) error {
	if len(handle) != 64 {
		return errors.New("import handle is not one this helper issued")
	}
	if _, err := hex.DecodeString(handle); err != nil {
		return errors.New("import handle is not one this helper issued")
	}
	return nil
}

// validateIdentity refuses a profile identity that could reach outside the
// keystore directory or collide with the staging namespace.
func validateIdentity(profileID string) error {
	if profileID == "" || len(profileID) > 128 {
		return errors.New("profile identity must be between 1 and 128 characters")
	}
	if strings.ContainsAny(profileID, `/\:*?"<>|`) || strings.Contains(profileID, "..") {
		return errors.New("profile identity must not contain a path")
	}
	for _, character := range profileID {
		if character < 0x20 || character == 0x7f {
			return errors.New("profile identity must not contain control characters")
		}
	}
	return nil
}

func (keystore *Keystore) syncDirectory() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(keystore.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
