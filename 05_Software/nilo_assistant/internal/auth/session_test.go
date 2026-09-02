package auth

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func TestNewSessionSecretsProducesDistinctSecretsAndAnAbsoluteExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	created, sessionVerifier, csrfVerifier, err := NewSessionSecrets(rand.Reader, now, 12*time.Hour)
	if err != nil {
		t.Fatalf("NewSessionSecrets: %v", err)
	}
	if created.Secret == "" || created.CSRFToken == "" || created.Secret == created.CSRFToken {
		t.Fatalf("session and CSRF secrets are missing or identical: %#v", created)
	}
	if !created.ExpiresAt.Equal(now.Add(12 * time.Hour)) {
		t.Fatalf("ExpiresAt = %s, want %s", created.ExpiresAt, now.Add(12*time.Hour))
	}
	if want := ComputeSessionVerifier(created.Secret); sessionVerifier != want {
		t.Fatal("session verifier is not the digest of the session secret")
	}
	if want := ComputeSessionVerifier(created.CSRFToken); csrfVerifier != want {
		t.Fatal("CSRF verifier is not the digest of the CSRF token")
	}
	if bytes.Contains([]byte(created.Secret), []byte(created.CSRFToken)) {
		t.Fatal("the CSRF token is derivable from the session secret")
	}

	other, _, _, err := NewSessionSecrets(rand.Reader, now, time.Hour)
	if err != nil {
		t.Fatalf("NewSessionSecrets second call: %v", err)
	}
	if other.Secret == created.Secret || other.ID == created.ID {
		t.Fatal("two sessions shared a secret or an identity")
	}
}

func TestNewSessionSecretsRejectsAnUnusableLifetimeOrRandomSource(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	if _, _, _, err := NewSessionSecrets(nil, now, time.Hour); err == nil {
		t.Fatal("NewSessionSecrets accepted a missing random source")
	}
	if _, _, _, err := NewSessionSecrets(rand.Reader, now, 0); err == nil {
		t.Fatal("NewSessionSecrets accepted a non-positive lifetime")
	}
}

func TestNormalizeUsernameFoldsCaseAndRejectsUnusableNames(t *testing.T) {
	t.Parallel()

	if got, err := NormalizeUsername("  Bench.Admin_1  "); err != nil || got != "bench.admin_1" {
		t.Fatalf("NormalizeUsername = %q, %v", got, err)
	}
	for _, name := range []string{"", "  ", "a", "has space", "ta\tb", "sym$bol", strings.Repeat("x", 65)} {
		if _, err := NormalizeUsername(name); err == nil {
			t.Fatalf("NormalizeUsername(%q) accepted an unusable name", name)
		}
	}
}
