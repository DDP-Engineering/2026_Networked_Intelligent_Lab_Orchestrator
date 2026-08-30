package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const tokenRandomBytes = 32

// CreatedToken is the one-time plaintext result of token creation.
type CreatedToken struct {
	ID        string
	Plaintext string
	CreatedAt time.Time
}

// TokenVerifier is the SHA-256 verifier persisted instead of token plaintext.
type TokenVerifier [sha256.Size]byte

// NewToken creates a UUID-v7 identified token from exactly 32 random bytes.
func NewToken(random io.Reader, now time.Time) (CreatedToken, TokenVerifier, error) {
	if random == nil {
		return CreatedToken{}, TokenVerifier{}, fmt.Errorf("token random source is required")
	}
	randomBytes := make([]byte, tokenRandomBytes)
	if _, err := io.ReadFull(random, randomBytes); err != nil {
		return CreatedToken{}, TokenVerifier{}, fmt.Errorf("read token randomness: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CreatedToken{}, TokenVerifier{}, fmt.Errorf("create token UUID v7: %w", err)
	}
	plaintext := "nilo_" + base64.RawURLEncoding.EncodeToString(randomBytes)
	verifier := TokenVerifier(sha256.Sum256([]byte(plaintext)))
	return CreatedToken{ID: id.String(), Plaintext: plaintext, CreatedAt: now.UTC()}, verifier, nil
}
