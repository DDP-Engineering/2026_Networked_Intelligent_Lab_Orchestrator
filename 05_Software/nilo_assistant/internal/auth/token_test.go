package auth_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
)

func TestNewTokenCreatesOneTimePlaintextAndVerifier(t *testing.T) {
	randomBytes := make([]byte, 32)
	for i := range randomBytes {
		randomBytes[i] = byte(i)
	}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.FixedZone("EEST", 3*60*60))

	created, verifier, err := auth.NewToken(bytes.NewReader(randomBytes), now)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if created.Plaintext != "nilo_AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8" {
		t.Fatalf("plaintext = %q, want prefix plus unpadded base64url of 32 bytes", created.Plaintext)
	}
	if strings.Contains(created.Plaintext, "=") {
		t.Fatalf("plaintext = %q, must not be padded", created.Plaintext)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(created.Plaintext, "nilo_"))
	if err != nil {
		t.Fatalf("decode plaintext token: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("random token byte length = %d, want 32", len(decoded))
	}
	if got, err := uuid.Parse(created.ID); err != nil || got.Version() != 7 {
		t.Fatalf("token ID %q is not a UUID v7: %v", created.ID, err)
	}
	if !created.CreatedAt.Equal(now.UTC()) || created.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt = %v, want UTC %v", created.CreatedAt, now.UTC())
	}
	wantVerifier := sha256.Sum256([]byte(created.Plaintext))
	if verifier != auth.TokenVerifier(wantVerifier) {
		t.Fatal("token verifier does not equal SHA-256 of plaintext")
	}
}

func TestNewTokenRejectsShortRandomRead(t *testing.T) {
	_, _, err := auth.NewToken(bytes.NewReader(make([]byte, 31)), time.Now())
	if err == nil {
		t.Fatal("NewToken accepted fewer than 32 random bytes")
	}
}
