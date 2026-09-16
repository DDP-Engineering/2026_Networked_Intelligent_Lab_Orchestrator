package auth

import (
	"errors"
	"fmt"
	"strings"
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
//
// The four requirements are independent and each says what it means - this
// class must be present. They are deliberately not a count: "at least three of
// four classes" is a question an administrator cannot answer for a particular
// password, because it never says which three.
type PasswordPolicy struct {
	// MinimumLength is the fewest bytes a password may be.
	MinimumLength int
	// RequireLowercase, RequireUppercase, RequireDigit and RequireSpecial each
	// demand at least one character of that class. Special is every rune that
	// is neither a letter nor a digit, which keeps the classification total.
	RequireLowercase bool
	RequireUppercase bool
	RequireDigit     bool
	RequireSpecial   bool
}

// DefaultPasswordPolicy is the rule an appliance applies while it has recorded
// no policy of its own: a length alone, with no composition rule, which is what
// NIST SP 800-63B favours and what every earlier release enforced.
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{MinimumLength: DefaultMinimumPasswordBytes}
}

// Validate refuses a policy the appliance will not hold. Only the length has
// bounds: a requirement is on or off, so every combination of the four is a
// policy somebody may legitimately want.
func (policy PasswordPolicy) Validate() error {
	if policy.MinimumLength < PasswordPolicyFloorBytes || policy.MinimumLength > MaxPasswordBytes {
		return fmt.Errorf("%w: minimum length must be between %d and %d bytes",
			ErrPasswordPolicyInvalid, PasswordPolicyFloorBytes, MaxPasswordBytes)
	}
	return nil
}

// Check holds one password to this policy. It names every requirement the
// password misses and never repeats the password, so the refusal can travel
// into a response, a log line or an audit event as it stands, and so an
// administrator can correct the password once rather than learning one rule per
// attempt.
func (policy PasswordPolicy) Check(password []byte) error {
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrPasswordRejected, MaxPasswordBytes)
	}
	if len(password) < policy.MinimumLength {
		return fmt.Errorf("%w: password must be at least %d bytes", ErrPasswordRejected, policy.MinimumLength)
	}
	if missing := policy.missingClasses(password); len(missing) > 0 {
		return fmt.Errorf("%w: password must contain %s",
			ErrPasswordRejected, strings.Join(missing, ", "))
	}
	return nil
}

// missingClasses names the required classes the password does not carry, in a
// fixed order so one password always produces one message.
func (policy PasswordPolicy) missingClasses(password []byte) []string {
	lower, upper, digit, special := presentClasses(password)
	var missing []string
	for _, requirement := range []struct {
		required bool
		present  bool
		name     string
	}{
		{policy.RequireLowercase, lower, "a lowercase letter"},
		{policy.RequireUppercase, upper, "an uppercase letter"},
		{policy.RequireDigit, digit, "a digit"},
		{policy.RequireSpecial, special, "a special character"},
	} {
		if requirement.required && !requirement.present {
			missing = append(missing, requirement.name)
		}
	}
	return missing
}

// presentClasses reports which of the four classes the password carries. Every
// rune belongs to exactly one of them: a byte sequence that is not valid UTF-8
// counts as special rather than an error, because the policy describes
// composition and does not police encoding.
func presentClasses(password []byte) (lower, upper, digit, special bool) {
	for index := 0; index < len(password); {
		character, width := utf8.DecodeRune(password[index:])
		index += width
		switch {
		case character == utf8.RuneError && width == 1:
			special = true
		case unicode.IsLower(character):
			lower = true
		case unicode.IsUpper(character):
			upper = true
		case unicode.IsDigit(character):
			digit = true
		default:
			special = true
		}
	}
	return lower, upper, digit, special
}
