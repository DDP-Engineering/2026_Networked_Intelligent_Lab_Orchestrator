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
	if policy.RequireLowercase || policy.RequireUppercase || policy.RequireDigit || policy.RequireSpecial {
		t.Fatalf("the shipped default requires a character class: %#v", policy)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("the shipped default must be a valid policy: %v", err)
	}
}

func TestPasswordPolicyValidateRefusesALengthOutsideItsBounds(t *testing.T) {
	cases := []struct {
		name   string
		length int
		names  string
	}{
		{"below the floor", 7, "8"},
		{"above the maximum", 1025, "1024"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := auth.PasswordPolicy{MinimumLength: testCase.length}.Validate()
			if err == nil {
				t.Fatal("Validate accepted a length outside its bounds")
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

// Every combination of requirements is a valid policy: a requirement is on or
// off, so there is no range for Validate to police.
func TestPasswordPolicyValidateAcceptsEveryCombinationOfRequirements(t *testing.T) {
	for _, policy := range []auth.PasswordPolicy{
		{MinimumLength: auth.PasswordPolicyFloorBytes},
		{MinimumLength: 12, RequireLowercase: true},
		{MinimumLength: 12, RequireLowercase: true, RequireUppercase: true, RequireDigit: true, RequireSpecial: true},
	} {
		if err := policy.Validate(); err != nil {
			t.Fatalf("Validate refused %#v: %v", policy, err)
		}
	}
}

func TestPasswordPolicyCheckHoldsAPasswordToTheConfiguredLength(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 16}
	err := policy.Check(bytes.Repeat([]byte{'a'}, 15))
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

// A requirement means what it says: this class must be present. It is not a
// count, so requiring two named classes is not satisfied by any other two.
func TestPasswordPolicyCheckRequiresEachTickedClass(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 12, RequireUppercase: true, RequireDigit: true}
	cases := []struct {
		name     string
		password string
		accepted bool
	}{
		{"neither required class", "passwordpassword", false},
		{"uppercase only", "Passwordpassword", false},
		{"digit only", "password12345678", false},
		{"both required classes", "Password1234", true},
		{"both, plus classes not required", "Password-1234", true},
		{"two classes, but not the required two", "password-1234", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := policy.Check([]byte(testCase.password))
			if testCase.accepted {
				if err != nil {
					t.Fatalf("Check refused %q: %v", testCase.password, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Check accepted %q", testCase.password)
			}
			if !errors.Is(err, auth.ErrPasswordRejected) {
				t.Fatalf("error %v is not ErrPasswordRejected", err)
			}
		})
	}
}

// The refusal names every requirement the password misses, and only those, so
// an administrator fixes the password once instead of one rule per attempt.
func TestPasswordPolicyCheckNamesEveryUnmetRequirementAndNoOther(t *testing.T) {
	policy := auth.PasswordPolicy{
		MinimumLength: 12, RequireLowercase: true, RequireUppercase: true,
		RequireDigit: true, RequireSpecial: true,
	}
	err := policy.Check([]byte("passwordpassword"))
	if err == nil {
		t.Fatal("Check accepted a password missing three required classes")
	}
	message := err.Error()
	for _, named := range []string{"uppercase", "digit"} {
		if !strings.Contains(message, named) {
			t.Fatalf("message %q does not name the missing %s", message, named)
		}
	}
	if !strings.Contains(message, "special") {
		t.Fatalf("message %q does not name the missing special character", message)
	}
	if strings.Contains(message, "lowercase") {
		t.Fatalf("message %q names lowercase, which the password carries", message)
	}
}

func TestPasswordPolicyCheckClassifiesEveryRune(t *testing.T) {
	cases := []struct {
		name     string
		policy   auth.PasswordPolicy
		password string
	}{
		{"a symbol is special", auth.PasswordPolicy{MinimumLength: 12, RequireSpecial: true}, "passwordpass-"},
		{"a space is special", auth.PasswordPolicy{MinimumLength: 12, RequireSpecial: true}, "password pass"},
		{"a non-ASCII symbol is special", auth.PasswordPolicy{MinimumLength: 12, RequireSpecial: true}, "passwordpass€"},
		{"a non-ASCII lowercase letter is lowercase", auth.PasswordPolicy{MinimumLength: 12, RequireLowercase: true}, "ßaaaaaaaaaaaa"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := testCase.policy.Check([]byte(testCase.password)); err != nil {
				t.Fatalf("Check refused %q: %v", testCase.password, err)
			}
		})
	}
}

// The policy describes composition; it does not police encoding.
func TestPasswordPolicyCheckTreatsInvalidUTF8AsSpecialRatherThanAnError(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 12, RequireSpecial: true}
	if err := policy.Check(append([]byte("passwordpass"), 0xff, 0xfe)); err != nil {
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

func TestPasswordPolicyCheckNeverRepeatsThePassword(t *testing.T) {
	policy := auth.PasswordPolicy{MinimumLength: 24, RequireDigit: true}
	password := "supersecret-value"
	err := policy.Check([]byte(password))
	if err == nil {
		t.Fatal("Check accepted a password that meets neither part")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("message %q repeats the password", err.Error())
	}
}
