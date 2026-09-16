package auth_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
)

func TestDefaultPasswordPolicyIsTheShippedRule(t *testing.T) {
	policy := auth.DefaultPasswordPolicy()
	if policy.MinimumLength != 12 {
		t.Fatalf("default minimum length = %d, want 12", policy.MinimumLength)
	}
	if policy.MinimumCharacterClasses != 0 {
		t.Fatalf("default class requirement = %d, want 0", policy.MinimumCharacterClasses)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("the shipped default must be a valid policy: %v", err)
	}
}

func TestPasswordPolicyValidateRefusesValuesOutsideItsBounds(t *testing.T) {
	cases := []struct {
		name   string
		policy auth.PasswordPolicy
		names  string
	}{
		{"below the floor", auth.PasswordPolicy{MinimumLength: 7}, "8"},
		{"above the maximum", auth.PasswordPolicy{MinimumLength: 1025}, "1024"},
		{"too many classes", auth.PasswordPolicy{MinimumLength: 12, MinimumCharacterClasses: 5}, "4"},
		{"negative classes", auth.PasswordPolicy{MinimumLength: 12, MinimumCharacterClasses: -1}, "0"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.policy.Validate()
			if err == nil {
				t.Fatal("Validate accepted a policy outside its bounds")
			}
			if !errors.Is(err, auth.ErrPasswordPolicyInvalid) {
				t.Fatalf("error %v is not ErrPasswordPolicyInvalid", err)
			}
			if !strings.Contains(err.Error(), testCase.names) {
				t.Fatalf("message %q does not name the bound %s", err.Error(), testCase.names)
			}
		})
	}
}

func TestPasswordPolicyValidateAcceptsTheFloorItself(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: auth.PasswordPolicyFloorBytes, MinimumCharacterClasses: 4}
	if err := policy.Validate(); err != nil {
		t.Fatalf("a policy at the floor with every class must be valid: %v", err)
	}
}

func TestPasswordPolicyCheckHoldsAPasswordToTheConfiguredLength(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 16}
	short := bytes.Repeat([]byte{'a'}, 15)
	err := policy.Check(short)
	if err == nil {
		t.Fatal("Check accepted a password one byte short of the minimum")
	}
	if !errors.Is(err, auth.ErrPasswordRejected) {
		t.Fatalf("error %v is not ErrPasswordRejected", err)
	}
	if !strings.Contains(err.Error(), "16") {
		t.Fatalf("message %q does not name the configured minimum", err.Error())
	}
	if err := policy.Check(bytes.Repeat([]byte{'a'}, 16)); err != nil {
		t.Fatalf("Check refused a password of exactly the minimum: %v", err)
	}
}

func TestPasswordPolicyCheckCountsDistinctCharacterClasses(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 12, MinimumCharacterClasses: 3}
	cases := []struct {
		name     string
		password string
		accepted bool
	}{
		{"one class", "passwordpassword", false},
		{"two classes", "Passwordpassword", false},
		{"three classes", "Password1234", true},
		{"four classes", "Password-1234", true},
		{"symbols count as other", "password-1234", true},
		{"a non-letter rune outside ASCII counts as other", "passwordwort€1", true},
		{"a lowercase rune outside ASCII stays lowercase", "passwordwortß1", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := policy.Check([]byte(testCase.password))
			if testCase.accepted && err != nil {
				t.Fatalf("Check refused %q: %v", testCase.password, err)
			}
			if !testCase.accepted {
				if err == nil {
					t.Fatalf("Check accepted %q with too few classes", testCase.password)
				}
				if !errors.Is(err, auth.ErrPasswordRejected) {
					t.Fatalf("error %v is not ErrPasswordRejected", err)
				}
				if !strings.Contains(err.Error(), "3") {
					t.Fatalf("message %q does not name the required class count", err.Error())
				}
			}
		})
	}
}

// The policy counts variety; it does not police encoding. A byte sequence that
// is not valid UTF-8 contributes the "other" class rather than an error, so a
// password pasted from an unknown encoding is refused for its length or its
// variety and never for being unreadable.
func TestPasswordPolicyCheckTreatsInvalidUTF8AsOtherRatherThanAnError(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 12, MinimumCharacterClasses: 2}
	password := append([]byte("passwordpass"), 0xff, 0xfe)
	if err := policy.Check(password); err != nil {
		t.Fatalf("Check refused a password carrying invalid UTF-8: %v", err)
	}
}

func TestPasswordPolicyCheckKeepsTheMaximumWhateverThePolicySays(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: auth.PasswordPolicyFloorBytes}
	err := policy.Check(bytes.Repeat([]byte{'a'}, auth.MaxPasswordBytes+1))
	if err == nil {
		t.Fatal("Check accepted a password longer than the fixed maximum")
	}
	if !strings.Contains(err.Error(), "1024") {
		t.Fatalf("message %q does not name the maximum", err.Error())
	}
}

// The policy never reports the password back, whatever it refuses.
func TestPasswordPolicyCheckNeverRepeatsThePassword(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 24, MinimumCharacterClasses: 4}
	password := "supersecret-value"
	err := policy.Check([]byte(password))
	if err == nil {
		t.Fatal("Check accepted a password that meets neither part")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("message %q repeats the password", err.Error())
	}
}
