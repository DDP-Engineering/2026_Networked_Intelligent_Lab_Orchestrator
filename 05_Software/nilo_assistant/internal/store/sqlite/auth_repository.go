package sqlite

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/auth"
)

// AuthRepository persists and authenticates non-secret MCP token records.
type AuthRepository struct {
	database *DB
}

// NewAuthRepository constructs a repository with its database dependency.
func NewAuthRepository(database *DB) *AuthRepository {
	return &AuthRepository{database: database}
}

// CreateUserAndToken atomically persists a user and only the token verifier.
func (repository *AuthRepository) CreateUserAndToken(
	ctx context.Context,
	user auth.UserRecord,
	token auth.CreatedToken,
	verifier auth.TokenVerifier,
) (err error) {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("authentication database is required")
	}
	if user.ID == "" || user.Username == "" || user.PasswordVerifier == "" || token.ID == "" {
		return errors.New("user and token fields are required")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user and token transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = transaction.Rollback()
		}
	}()
	timestamp := user.CreatedAt.UTC().Format(time.RFC3339Nano)
	if _, err = transaction.ExecContext(
		ctx,
		"INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		user.ID,
		user.Username,
		user.PasswordVerifier,
		timestamp,
		timestamp,
	); err != nil {
		return fmt.Errorf("store development user: %w", err)
	}
	if _, err = transaction.ExecContext(
		ctx,
		"INSERT INTO mcp_tokens (id, user_id, verifier, created_at) VALUES (?, ?, ?, ?)",
		token.ID,
		user.ID,
		verifier[:],
		token.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("store development token verifier: %w", err)
	}
	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("commit development user and token: %w", err)
	}
	return nil
}

// CreateToken persists only the SHA-256 verifier for a newly-created token.
func (repository *AuthRepository) CreateToken(ctx context.Context, userID, id string, createdAt time.Time, verifier auth.TokenVerifier) error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("authentication database is required")
	}
	if userID == "" || id == "" {
		return errors.New("token user and ID are required")
	}
	_, err := repository.database.SQL().ExecContext(
		ctx,
		"INSERT INTO mcp_tokens (id, user_id, verifier, created_at) VALUES (?, ?, ?, ?)",
		id,
		userID,
		verifier[:],
		createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("store token verifier: %w", err)
	}
	return nil
}

// VerifyToken authenticates plaintext using a constant-time verifier comparison.
func (repository *AuthRepository) VerifyToken(ctx context.Context, plaintext string) (auth.TokenRecord, error) {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return auth.TokenRecord{}, errors.New("authentication database is required")
	}
	candidate := auth.TokenVerifier(sha256.Sum256([]byte(plaintext)))
	rows, err := repository.database.SQL().QueryContext(ctx, "SELECT id, user_id, verifier, created_at, revoked_at FROM mcp_tokens")
	if err != nil {
		return auth.TokenRecord{}, fmt.Errorf("read token verifiers: %w", err)
	}
	defer rows.Close()

	var matched auth.TokenRecord
	found := false
	for rows.Next() {
		var verifier []byte
		var createdAt string
		var revokedAt sql.NullString
		var record auth.TokenRecord
		if err := rows.Scan(&record.ID, &record.UserID, &verifier, &createdAt, &revokedAt); err != nil {
			return auth.TokenRecord{}, fmt.Errorf("scan token verifier: %w", err)
		}
		if subtle.ConstantTimeCompare(candidate[:], verifier) != 1 {
			continue
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return auth.TokenRecord{}, fmt.Errorf("parse token creation time: %w", err)
		}
		record.CreatedAt = parsedCreatedAt.UTC()
		if revokedAt.Valid {
			parsedRevokedAt, err := time.Parse(time.RFC3339Nano, revokedAt.String)
			if err != nil {
				return auth.TokenRecord{}, fmt.Errorf("parse token revocation time: %w", err)
			}
			utcRevokedAt := parsedRevokedAt.UTC()
			record.RevokedAt = &utcRevokedAt
		}
		matched = record
		found = true
	}
	if err := rows.Err(); err != nil {
		return auth.TokenRecord{}, fmt.Errorf("iterate token verifiers: %w", err)
	}
	if !found {
		return auth.TokenRecord{}, auth.ErrTokenNotFound
	}
	if matched.RevokedAt != nil {
		return auth.TokenRecord{}, auth.ErrTokenRevoked
	}
	return matched, nil
}

// RevokeToken records the UTC revocation time for an active token.
func (repository *AuthRepository) RevokeToken(ctx context.Context, id string, at time.Time) error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("authentication database is required")
	}
	result, err := repository.database.SQL().ExecContext(
		ctx,
		"UPDATE mcp_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL",
		at.UTC().Format(time.RFC3339Nano),
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read token revocation result: %w", err)
	}
	if changed == 0 {
		return auth.ErrTokenNotFound
	}
	return nil
}

var _ auth.TokenRepository = (*AuthRepository)(nil)
var _ auth.DevelopmentTokenRepository = (*AuthRepository)(nil)
