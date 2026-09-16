package auth

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	// PasswordPolicyFloorBytes is the shortest minimum length an administrator
	// may configure. It is compiled in and is not itself configurable, because a
	// minimum somebody can set to one character is not a policy. Eight is the
	// fewest characters NIST SP 800-63B accepts for a user-chosen secret.
	PasswordPolicyFloorBytes = 8
	// DefaultMinimumPasswordBytes is the minimum an appliance applies while no
	// policy has been recorded. It sits above the floor deliberately: an
	// appliance nobody configures keeps the rule every NILO has shipped with,
	// and an administrator who goes below it does so on purpose.
	DefaultMinimumPasswordBytes = 12
	// PasswordPolicyMaxCharacterClasses is every class a password can draw from:
	// lowercase, uppercase, digit, and everything else.
	PasswordPolicyMaxCharacterClasses = 4
)

// ErrPasswordPolicyInvalid refuses a policy outside the bounds the appliance
// will accept. ErrPasswordRejected refuses a password against a valid policy.
// They are separate because one is an administrator's mistake about the rule
// and the other is anybody's mistake about a password.
var (
	ErrPasswordPolicyInvalid = errors.New("password policy rejected")
	ErrPasswordRejected      = errors.New("password rejected")
)

// PasswordPolicy is the appliance's one rule for a password being set. It is
// never consulted when a stored verifier is checked: tightening it would
// otherwise lock out every identity that already exists, including the
// appliance's only administrator, with no way back in.
type PasswordPolicy struct {
	// MinimumLength is the fewest bytes a password may be.
	MinimumLength int
	// MinimumCharacterClasses is the fewest distinct classes a password must
	// carry, from lowercase, uppercase, digit and other.
	MinimumCharacterClasses int
}

// DefaultPasswordPolicy is the rule an appliance applies while it has recorded
// no policy of its own.
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinimumLength: DefaultMinimumPasswordBytes}
}

// Validate refuses a policy the appliance will not hold. The minimum may not go
// below the floor, and may not exceed the fixed maximum, which would leave a
// rule no password could satisfy.
func (policy PasswordPolicy) Validate() error {
	if policy.MinimumLength < PasswordPolicyFloorBytes || policy.MinimumLength > MaxPasswordBytes {
		return fmt.Errorf("%w: minimum length must be between %d and %d bytes",
			ErrPasswordPolicyInvalid, PasswordPolicyFloorBytes, MaxPasswordBytes)
	}
	if policy.MinimumCharacterClasses < 0 || policy.MinimumCharacterClasses > PasswordPolicyMaxCharacterClasses {
		return fmt.Errorf("%w: minimum character classes must be between 0 and %d",
			ErrPasswordPolicyInvalid, PasswordPolicyMaxCharacterClasses)
	}
	return nil
}

// Check holds one password to this policy. It names the part that failed and
// never repeats the password, so the refusal can travel into a response, a log
// line or an audit event as it stands.
func (policy PasswordPolicy) Check(password []byte) error {
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrPasswordRejected, MaxPasswordBytes)
	}
	if len(password) < policy.MinimumLength {
		return fmt.Errorf("%w: password must be at least %d bytes", ErrPasswordRejected, policy.MinimumLength)
	}
	if policy.MinimumCharacterClasses > 0 {
		if present := countCharacterClasses(password); present < policy.MinimumCharacterClasses {
			return fmt.Errorf(
				"%w: password must contain at least %d of lowercase, uppercase, digit and other characters",
				ErrPasswordRejected, policy.MinimumCharacterClasses)
		}
	}
	return nil
}

// countCharacterClasses counts how many of the four classes the password draws
// from. A byte sequence that is not valid UTF-8 contributes the "other" class
// rather than an error: the policy counts variety, it does not police encoding.
func countCharacterClasses(password []byte) int {
	var lower, upper, digit, other bool
	for index := 0; index < len(password); {
		character, width := utf8.DecodeRune(password[index:])
		index += width
		switch {
		case character == utf8.RuneError && width == 1:
			other = true
		case unicode.IsLower(character):
			lower = true
		case unicode.IsUpper(character):
			upper = true
		case unicode.IsDigit(character):
			digit = true
		default:
			other = true
		}
	}
	present := 0
	for _, class := range []bool{lower, upper, digit, other} {
		if class {
			present++
		}
	}
	return present
}
