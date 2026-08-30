package auth_test

import (
	"bytes"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
)

func TestHashPasswordRoundTripsAndDoesNotMutateCallerBuffer(t *testing.T) {
	password := []byte("correct horse battery staple")
	original := append([]byte(nil), password...)

	encoded, err := auth.HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{0x7a}, 16)))
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("PHC hash = %q, want Argon2id defaults", encoded)
	}
	if !bytes.Equal(password, original) {
		t.Fatalf("HashPassword changed caller password buffer: got %q, want %q", password, original)
	}

	verified, err := auth.VerifyPassword(encoded, password)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !verified {
		t.Fatal("VerifyPassword rejected the password that was hashed")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	encoded, err := auth.HashPassword([]byte("correct horse battery staple"), bytes.NewReader(bytes.Repeat([]byte{0x7a}, 16)))
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	verified, err := auth.VerifyPassword(encoded, []byte("incorrect horse battery staple"))
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if verified {
		t.Fatal("VerifyPassword accepted a wrong password")
	}
}

func TestHashPasswordUsesDistinctSalts(t *testing.T) {
	password := []byte("correct horse battery staple")
	first, err := auth.HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{0x01}, 16)))
	if err != nil {
		t.Fatalf("first HashPassword: %v", err)
	}
	second, err := auth.HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{0x02}, 16)))
	if err != nil {
		t.Fatalf("second HashPassword: %v", err)
	}
	if first == second {
		t.Fatal("hashes with distinct random salts are equal")
	}
}

func TestPasswordFunctionsRejectOutOfBoundsByteLengths(t *testing.T) {
	for _, password := range [][]byte{bytes.Repeat([]byte{'a'}, 11), bytes.Repeat([]byte{'a'}, 1025)} {
		if _, err := auth.HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{0x7a}, 16))); err == nil {
			t.Fatalf("HashPassword accepted %d-byte password", len(password))
		}
	}

	encoded, err := auth.HashPassword([]byte("correct horse battery staple"), bytes.NewReader(bytes.Repeat([]byte{0x7a}, 16)))
	if err != nil {
		t.Fatalf("HashPassword fixture: %v", err)
	}
	for _, password := range [][]byte{bytes.Repeat([]byte{'a'}, 11), bytes.Repeat([]byte{'a'}, 1025)} {
		if _, err := auth.VerifyPassword(encoded, password); err == nil {
			t.Fatalf("VerifyPassword accepted %d-byte password", len(password))
		}
	}
}

func TestVerifyPasswordRejectsMalformedAndUnsupportedPHCInputs(t *testing.T) {
	cases := []string{
		"",
		"not-phc",
		"$argon2id$v=19$m=65536,t=3,p=2$short",
		"$argon2i$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=16$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=0$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=abc,t=3,p=2$c2FsdA$aGFzaA",
	}
	for _, encoded := range cases {
		t.Run(encoded, func(t *testing.T) {
			if _, err := auth.VerifyPassword(encoded, []byte("correct horse battery staple")); err == nil {
				t.Fatalf("VerifyPassword accepted malformed or unsupported PHC input %q", encoded)
			}
		})
	}
}

func TestVerifyPasswordRejectsOversizedPHCInputBeforeParsing(t *testing.T) {
	encoded := "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 1<<20) + "$" + strings.Repeat("A", 43)

	_, err := auth.VerifyPassword(encoded, []byte("correct horse battery staple"))
	if err == nil || !strings.Contains(err.Error(), "maximum length") {
		t.Fatalf("VerifyPassword oversized PHC error = %v, want maximum-length rejection", err)
	}
}

func TestVerifyPasswordRejectsOversizedPHCComponentsBeforeDecode(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		want    string
	}{
		{
			name:    "salt",
			encoded: "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 23) + "$" + strings.Repeat("A", 42),
			want:    "salt encoded length",
		},
		{
			name:    "hash",
			encoded: "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 22) + "$" + strings.Repeat("A", 44),
			want:    "maximum length",
		},
		{
			name:    "short hash",
			encoded: "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 22) + "$" + strings.Repeat("A", 42),
			want:    "hash encoded length",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := auth.VerifyPassword(test.encoded, []byte("correct horse battery staple"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyPassword oversized %s error = %v, want %q", test.name, err, test.want)
			}
		})
	}
}
