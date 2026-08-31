package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenRevoked  = errors.New("token revoked")
)

// TokenRecord is the non-secret representation returned after authentication.
type TokenRecord struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// TokenRepository makes token storage and authentication dependencies explicit.
type TokenRepository interface {
	CreateToken(ctx context.Context, userID, id string, createdAt time.Time, verifier TokenVerifier) error
	VerifyToken(ctx context.Context, plaintext string) (TokenRecord, error)
	RevokeToken(ctx context.Context, id string, at time.Time) error
}
