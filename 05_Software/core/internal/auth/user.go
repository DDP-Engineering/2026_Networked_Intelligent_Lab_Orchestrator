package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrUserNotFound reports that no identity carries the requested name or ID.
	ErrUserNotFound = errors.New("user not found")
	// ErrUsernameTaken reports that the username is already in use.
	ErrUsernameTaken = errors.New("username already exists")
)

const (
	minUsernameLength = 2
	maxUsernameLength = 64
)

// User is one durable human or development identity. Identities are disabled
// rather than deleted so that audit records keep resolving to a real subject.
type User struct {
	ID               string
	Username         string
	Role             Role
	PasswordVerifier string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DisabledAt       *time.Time
}

// Enabled reports whether the identity may authenticate or own a usable token.
func (user User) Enabled() bool { return user.DisabledAt == nil }

// CanPasswordLogin reports whether the identity has a real password verifier.
// Development token identities deliberately carry a marker instead of one.
func (user User) CanPasswordLogin() bool {
	return user.PasswordVerifier != DisabledDevelopmentPasswordVerifier
}

// NormalizeUsername folds a username to its stored form and rejects names that
// would be ambiguous in a listing or a command line.
func NormalizeUsername(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) < minUsernameLength || len(normalized) > maxUsernameLength {
		return "", fmt.Errorf("username must be between %d and %d characters", minUsernameLength, maxUsernameLength)
	}
	for _, character := range normalized {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character == '-' || character == '_' || character == '.':
		default:
			return "", fmt.Errorf("username may contain only letters, digits, %q, %q and %q", "-", "_", ".")
		}
	}
	return normalized, nil
}
