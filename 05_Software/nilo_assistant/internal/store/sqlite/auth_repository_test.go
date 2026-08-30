package sqlite

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
)

const authUserID = "018f3d8d-8d2c-7c20-8e0c-000000000007"

func TestAuthRepositoryStoresOnlyVerifierAndRejectsDuplicateVerifier(t *testing.T) {
	ctx := context.Background()
	db := openTemporaryDatabase(t)
	seedAuthUser(t, db)
	repository := NewAuthRepository(db)
	created, verifier := newCreatedToken(t, 0x01)

	if err := repository.CreateToken(ctx, authUserID, created.ID, created.CreatedAt, verifier); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	var verifierBytes []byte
	if err := db.SQL().QueryRowContext(ctx, "SELECT verifier FROM mcp_tokens WHERE id = ?", created.ID).Scan(&verifierBytes); err != nil {
		t.Fatalf("read persisted verifier: %v", err)
	}
	if !bytes.Equal(verifierBytes, verifier[:]) {
		t.Fatal("persisted verifier differs from token verifier")
	}
	var plaintextOccurrences int
	if err := db.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM mcp_tokens WHERE CAST(verifier AS TEXT) = ?", created.Plaintext).Scan(&plaintextOccurrences); err != nil {
		t.Fatalf("check plaintext persistence: %v", err)
	}
	if plaintextOccurrences != 0 {
		t.Fatal("token plaintext was stored in the database")
	}

	duplicate := created
	duplicate.ID = "018f3d8d-8d2c-7c20-8e0c-000000000008"
	if err := repository.CreateToken(ctx, authUserID, duplicate.ID, duplicate.CreatedAt, verifier); err == nil {
		t.Fatal("CreateToken accepted a duplicate token verifier")
	}
}

func TestAuthRepositoryAuthenticatesAfterReloadAndPersistsRevocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seedAuthUser(t, db)
	created, verifier := newCreatedToken(t, 0x02)
	if err := NewAuthRepository(db).CreateToken(ctx, authUserID, created.ID, created.CreatedAt, verifier); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close before reload: %v", err)
	}

	reloaded, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reload database: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	repository := NewAuthRepository(reloaded)
	record, err := repository.VerifyToken(ctx, created.Plaintext)
	if err != nil {
		t.Fatalf("VerifyToken after reload: %v", err)
	}
	if record.ID != created.ID || record.UserID != authUserID || record.RevokedAt != nil {
		t.Fatalf("verified record = %#v, want active persisted token", record)
	}

	revokedAt := time.Date(2026, 8, 30, 15, 0, 0, 0, time.FixedZone("EEST", 3*60*60))
	if err := repository.RevokeToken(ctx, created.ID, revokedAt); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := reloaded.Close(); err != nil {
		t.Fatalf("close after revoke: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	_, err = NewAuthRepository(reopened).VerifyToken(ctx, created.Plaintext)
	if !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("VerifyToken after revocation error = %v, want ErrTokenRevoked", err)
	}
	var persisted string
	if err := reopened.SQL().QueryRowContext(ctx, "SELECT revoked_at FROM mcp_tokens WHERE id = ?", created.ID).Scan(&persisted); err != nil {
		t.Fatalf("read revocation timestamp: %v", err)
	}
	if persisted != revokedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("revoked_at = %q, want %q", persisted, revokedAt.UTC().Format(time.RFC3339Nano))
	}
}

func TestAuthRepositoryReturnsTypedErrorForUnknownToken(t *testing.T) {
	db := openTemporaryDatabase(t)
	_, err := NewAuthRepository(db).VerifyToken(context.Background(), "nilo_unknown")
	if !errors.Is(err, auth.ErrTokenNotFound) {
		t.Fatalf("VerifyToken unknown error = %v, want ErrTokenNotFound", err)
	}
}

func seedAuthUser(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.SQL().ExecContext(context.Background(), "INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", authUserID, "admin", "stored-verifier-only", createdAt, createdAt)
	if err != nil {
		t.Fatalf("seed auth user: %v", err)
	}
}

func newCreatedToken(t *testing.T, fill byte) (auth.CreatedToken, auth.TokenVerifier) {
	t.Helper()
	created, verifier, err := auth.NewToken(bytes.NewReader(bytes.Repeat([]byte{fill}, 32)), time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	return created, verifier
}
