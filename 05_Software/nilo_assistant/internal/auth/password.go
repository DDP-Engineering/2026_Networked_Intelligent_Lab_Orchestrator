package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// MinPasswordBytes and MaxPasswordBytes bound an acceptable password. They
	// are exported so an interface can refuse a password with a useful message
	// before the slow hash runs, without inventing its own limits.
	MinPasswordBytes = 12
	MaxPasswordBytes = 1024

	argon2Version = 19

	argon2SaltBytes        = 16
	argon2KeyBytes         = 32
	argon2SaltEncodedChars = (argon2SaltBytes*8 + 5) / 6
	argon2KeyEncodedChars  = (argon2KeyBytes*8 + 5) / 6
	argon2PHCPrefixLength  = len("$argon2id$v=19$m=65536,t=3,p=2$")
	// The maximum is the fixed PHC prefix, 16-byte raw-base64 salt, separator,
	// and 32-byte raw-base64 key. Reject it before splitting attacker input.
	argon2PHCMaxLength = argon2PHCPrefixLength + argon2SaltEncodedChars + 1 + argon2KeyEncodedChars
)

// Argon2Params are the explicit parameters recorded in password PHC strings.
type Argon2Params struct {
	MemoryKiB uint32
	Time      uint32
	Threads   uint8
	SaltBytes uint32
	KeyBytes  uint32
}

var defaultArgon2Params = Argon2Params{
	MemoryKiB: 65536,
	Time:      3,
	Threads:   2,
	SaltBytes: argon2SaltBytes,
	KeyBytes:  argon2KeyBytes,
}

// HashPassword returns a versioned Argon2id PHC verifier for password.
func HashPassword(password []byte, random io.Reader) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	if random == nil {
		return "", errors.New("password random source is required")
	}

	salt := make([]byte, defaultArgon2Params.SaltBytes)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}
	hash := derivePasswordKey(password, salt, defaultArgon2Params)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		defaultArgon2Params.MemoryKiB,
		defaultArgon2Params.Time,
		defaultArgon2Params.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword checks password against a supported Argon2id PHC verifier.
func VerifyPassword(encoded string, password []byte) (bool, error) {
	if err := validatePassword(password); err != nil {
		return false, err
	}
	params, salt, expected, err := parsePasswordPHC(encoded)
	if err != nil {
		return false, err
	}
	actual := derivePasswordKey(password, salt, params)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func validatePassword(password []byte) error {
	if len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return fmt.Errorf("password must be between %d and %d bytes", MinPasswordBytes, MaxPasswordBytes)
	}
	return nil
}

func derivePasswordKey(password, salt []byte, params Argon2Params) []byte {
	return argon2.IDKey(password, salt, params.Time, params.MemoryKiB, params.Threads, params.KeyBytes)
}

func parsePasswordPHC(encoded string) (Argon2Params, []byte, []byte, error) {
	if len(encoded) > argon2PHCMaxLength {
		return Argon2Params{}, nil, nil, errors.New("Argon2id PHC exceeds maximum length")
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, errors.New("invalid Argon2id PHC encoding")
	}
	if parts[2] != "v=19" {
		return Argon2Params{}, nil, nil, errors.New("unsupported Argon2id PHC version")
	}

	params, err := parseArgon2Params(parts[3])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}
	if params != defaultArgon2Params {
		return Argon2Params{}, nil, nil, errors.New("unsupported Argon2id PHC parameters")
	}
	if len(parts[4]) != argon2SaltEncodedChars {
		return Argon2Params{}, nil, nil, errors.New("invalid Argon2id PHC salt encoded length")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != int(params.SaltBytes) {
		return Argon2Params{}, nil, nil, errors.New("invalid Argon2id PHC salt")
	}
	if len(parts[5]) != argon2KeyEncodedChars {
		return Argon2Params{}, nil, nil, errors.New("invalid Argon2id PHC hash encoded length")
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(hash) != int(params.KeyBytes) {
		return Argon2Params{}, nil, nil, errors.New("invalid Argon2id PHC hash")
	}
	return params, salt, hash, nil
}

func parseArgon2Params(encoded string) (Argon2Params, error) {
	values := make(map[string]string, 3)
	for _, part := range strings.Split(encoded, ",") {
		name, value, found := strings.Cut(part, "=")
		if !found || name == "" || value == "" {
			return Argon2Params{}, errors.New("invalid Argon2id PHC parameters")
		}
		if _, duplicate := values[name]; duplicate {
			return Argon2Params{}, errors.New("duplicate Argon2id PHC parameter")
		}
		values[name] = value
	}
	if len(values) != 3 {
		return Argon2Params{}, errors.New("invalid Argon2id PHC parameters")
	}
	memory, err := parseUint32(values, "m")
	if err != nil {
		return Argon2Params{}, err
	}
	timeCost, err := parseUint32(values, "t")
	if err != nil {
		return Argon2Params{}, err
	}
	threads, err := parseUint8(values, "p")
	if err != nil {
		return Argon2Params{}, err
	}
	return Argon2Params{MemoryKiB: memory, Time: timeCost, Threads: threads, SaltBytes: defaultArgon2Params.SaltBytes, KeyBytes: defaultArgon2Params.KeyBytes}, nil
}

func parseUint32(values map[string]string, name string) (uint32, error) {
	value, found := values[name]
	if !found {
		return 0, fmt.Errorf("missing Argon2id PHC parameter %q", name)
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("invalid Argon2id PHC parameter %q", name)
	}
	return uint32(parsed), nil
}

func parseUint8(values map[string]string, name string) (uint8, error) {
	value, found := values[name]
	if !found {
		return 0, fmt.Errorf("missing Argon2id PHC parameter %q", name)
	}
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("invalid Argon2id PHC parameter %q", name)
	}
	return uint8(parsed), nil
}
