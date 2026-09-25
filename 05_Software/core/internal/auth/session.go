package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const (
	// SessionCookieName carries the browser session secret. It is HttpOnly, so
	// page scripts cannot read it and automation cannot borrow it.
	SessionCookieName = "nilo_session"
	// CSRFCookieName carries the readable half of the browser session
	// credential; the browser attaches it, but only page code can echo it back.
	CSRFCookieName = "nilo_csrf"
	// CSRFHeaderName is where a state-changing request repeats the CSRF token.
	CSRFHeaderName = "X-NILO-CSRF"

	sessionRandomBytes = 32
)

var (
	// ErrSessionNotFound reports that no session matches the presented secret.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionRevoked reports that the session was invalidated server-side.
	ErrSessionRevoked = errors.New("session revoked")
	// ErrCredentialSuperseded reports that the password a login verified was
	// replaced, or its identity disabled, before the session could be stored.
	ErrCredentialSuperseded = errors.New("credential superseded before its session was established")
)

// SessionVerifier is the SHA-256 digest persisted instead of a session or CSRF
// secret, so a stolen database yields no usable browser credential.
type SessionVerifier [sha256.Size]byte

// CreatedSession is the one-time result of establishing a browser session.
// Secret and CSRFToken are never persisted and never returned again.
type CreatedSession struct {
	ID        string
	Secret    string
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionRecord is the non-secret representation of a stored browser session.
type SessionRecord struct {
	ID           string
	UserID       string
	CSRFVerifier SessionVerifier
	CreatedAt    time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

// Expired reports whether the session has reached its absolute expiry.
func (session SessionRecord) Expired(now time.Time) bool {
	return !now.Before(session.ExpiresAt)
}

// ComputeSessionVerifier derives the stored verifier for a session or CSRF secret.
func ComputeSessionVerifier(plaintext string) SessionVerifier {
	return SessionVerifier(sha256.Sum256([]byte(plaintext)))
}

// NewSessionSecrets mints one browser session: an independent session secret
// and CSRF token, and the absolute expiry the server enforces on every request.
func NewSessionSecrets(random io.Reader, now time.Time, lifetime time.Duration) (CreatedSession, SessionVerifier, SessionVerifier, error) {
	if random == nil {
		return CreatedSession{}, SessionVerifier{}, SessionVerifier{}, errors.New("session random source is required")
	}
	if lifetime <= 0 {
		return CreatedSession{}, SessionVerifier{}, SessionVerifier{}, errors.New("positive session lifetime is required")
	}
	secret, err := readSessionSecret(random)
	if err != nil {
		return CreatedSession{}, SessionVerifier{}, SessionVerifier{}, err
	}
	csrfToken, err := readSessionSecret(random)
	if err != nil {
		return CreatedSession{}, SessionVerifier{}, SessionVerifier{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return CreatedSession{}, SessionVerifier{}, SessionVerifier{}, fmt.Errorf("create session UUID v7: %w", err)
	}
	created := CreatedSession{
		ID:        id.String(),
		Secret:    secret,
		CSRFToken: csrfToken,
		CreatedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(lifetime),
	}
	return created, ComputeSessionVerifier(secret), ComputeSessionVerifier(csrfToken), nil
}

func readSessionSecret(random io.Reader) (string, error) {
	buffer := make([]byte, sessionRandomBytes)
	if _, err := io.ReadFull(random, buffer); err != nil {
		return "", fmt.Errorf("read session randomness: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
