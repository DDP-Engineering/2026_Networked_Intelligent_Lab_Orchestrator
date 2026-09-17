package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
)

const policyCompliantPassword = "Correct-Horse-Battery-Staple-1"

// A fresh appliance applies the shipped default until an administrator records
// something else.
func TestServiceStartsAtTheShippedPasswordPolicy(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	policy, err := fixture.service.PasswordPolicy(context.Background())
	if err != nil {
		t.Fatalf("PasswordPolicy: %v", err)
	}
	if policy != auth.DefaultPasswordPolicy() {
		t.Fatalf("policy = %#v, want %#v", policy, auth.DefaultPasswordPolicy())
	}
}

func TestSetPasswordPolicyHoldsEveryLaterPasswordSetToIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)

	tightened := auth.PasswordPolicy{MinimumLength: 20, RequireUppercase: true, RequireDigit: true}
	if err := fixture.service.SetPasswordPolicy(ctx, administrator, tightened); err != nil {
		t.Fatalf("SetPasswordPolicy: %v", err)
	}

	_, err := fixture.service.CreateUser(ctx, administrator, "shortpass", auth.RoleOperator, []byte("Passw0rd-1234"))
	if err == nil {
		t.Fatal("CreateUser accepted a password shorter than the configured minimum")
	}
	if !errors.Is(err, auth.ErrPasswordRejected) {
		t.Fatalf("error %v is not ErrPasswordRejected", err)
	}
	if !strings.Contains(err.Error(), "20") {
		t.Fatalf("message %q does not name the configured minimum", err.Error())
	}

	if _, err := fixture.service.CreateUser(
		ctx, administrator, "missingclasses", auth.RoleOperator, []byte("passwordpasswordpassword"),
	); !errors.Is(err, auth.ErrPasswordRejected) {
		t.Fatalf("CreateUser missing both required classes = %v, want ErrPasswordRejected", err)
	}

	if _, err := fixture.service.CreateUser(
		ctx, administrator, "compliant", auth.RoleOperator, []byte(policyCompliantPassword),
	); err != nil {
		t.Fatalf("CreateUser refused a password meeting the policy: %v", err)
	}
}

func TestResetPasswordIsHeldToTheConfiguredPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)
	if _, err := fixture.service.CreateUser(
		ctx, administrator, "operator", auth.RoleOperator, []byte(administratorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	tightened := auth.PasswordPolicy{MinimumLength: 24}
	if err := fixture.service.SetPasswordPolicy(ctx, administrator, tightened); err != nil {
		t.Fatalf("SetPasswordPolicy: %v", err)
	}
	if err := fixture.service.ResetPassword(
		ctx, administrator, "operator", []byte(administratorPassword),
	); !errors.Is(err, auth.ErrPasswordRejected) {
		t.Fatalf("ResetPassword under a tightened policy = %v, want ErrPasswordRejected", err)
	}
	if err := fixture.service.ResetPassword(
		ctx, administrator, "operator", []byte("a-password-of-quite-sufficient-length"),
	); err != nil {
		t.Fatalf("ResetPassword refused a password meeting the policy: %v", err)
	}
}

// Tightening the policy must never cost an existing identity its access. This
// is the behavior the split of the shared bounds check exists for.
func TestTighteningThePolicyLeavesExistingIdentitiesAbleToSignIn(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)
	if _, err := fixture.service.CreateUser(
		ctx, administrator, "operator", auth.RoleOperator, []byte(administratorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := fixture.service.SetPasswordPolicy(
		ctx, administrator, auth.PasswordPolicy{MinimumLength: 64, RequireLowercase: true, RequireUppercase: true, RequireDigit: true, RequireSpecial: true},
	); err != nil {
		t.Fatalf("SetPasswordPolicy: %v", err)
	}

	matched, err := fixture.service.PasswordVerifierMatches(ctx, "operator", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("PasswordVerifierMatches after tightening: %v", err)
	}
	if !matched {
		t.Fatal("an identity created under the earlier policy no longer matches its own password")
	}

	result, err := fixture.service.Login(ctx, "operator", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("Login after tightening the policy: %v", err)
	}
	if result.User.Username != "operator" {
		t.Fatalf("Login established a session for %q", result.User.Username)
	}
}

// The published credential every appliance image ships with is 14 bytes of two
// classes. It has to keep opening an appliance whose policy has since been
// raised past it, or first-boot sign-in breaks on a commissioned appliance.
func TestThePublishedCredentialStillSignsInUnderATightenedPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	published := []byte("admin_password")
	administrator, err := fixture.service.Bootstrap(ctx, "admin", published)
	if err != nil {
		t.Fatalf("Bootstrap with the published credential: %v", err)
	}
	if err := fixture.service.SetPasswordPolicy(
		ctx, administrator, auth.PasswordPolicy{MinimumLength: 32, RequireLowercase: true, RequireUppercase: true, RequireDigit: true, RequireSpecial: true},
	); err != nil {
		t.Fatalf("SetPasswordPolicy: %v", err)
	}
	if _, err := fixture.service.Login(ctx, "admin", published); err != nil {
		t.Fatalf("the published credential stopped signing in: %v", err)
	}
}

func TestSetPasswordPolicyRefusesAMinimumBelowTheFloor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)

	err := fixture.service.SetPasswordPolicy(ctx, administrator, auth.PasswordPolicy{MinimumLength: 7})
	if !errors.Is(err, auth.ErrPasswordPolicyInvalid) {
		t.Fatalf("SetPasswordPolicy below the floor = %v, want ErrPasswordPolicyInvalid", err)
	}
	if !strings.Contains(err.Error(), "8") {
		t.Fatalf("message %q does not name the floor", err.Error())
	}
	policy, err := fixture.service.PasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("PasswordPolicy: %v", err)
	}
	if policy != auth.DefaultPasswordPolicy() {
		t.Fatalf("a refused change left policy = %#v", policy)
	}
}

func TestSetPasswordPolicyRefusesANonAdministrator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)
	operator, err := fixture.service.CreateUser(
		ctx, administrator, "operator", auth.RoleOperator, []byte(policyCompliantPassword),
	)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := fixture.service.SetPasswordPolicy(
		ctx, operator, auth.PasswordPolicy{MinimumLength: 16},
	); err == nil {
		t.Fatal("SetPasswordPolicy accepted an operator")
	}
	policy, err := fixture.service.PasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("PasswordPolicy: %v", err)
	}
	if policy != auth.DefaultPasswordPolicy() {
		t.Fatalf("a refused change left policy = %#v", policy)
	}
}

// The change and the event that records it are one transaction, so an appliance
// whose audit history cannot be written does not quietly change its credential
// rule either.
func TestSetPasswordPolicyRecordsOneAuditEventAndFailsWithIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixture := newFixture(t)
	administrator := fixture.bootstrap(t)
	const countPolicyEvents = "SELECT COUNT(*) FROM audit_events WHERE action = 'identity.password_policy.changed'"
	before := fixture.countRows(t, countPolicyEvents)

	tightened := auth.PasswordPolicy{MinimumLength: 16, RequireLowercase: true, RequireDigit: true}
	if err := fixture.service.SetPasswordPolicy(ctx, administrator, tightened); err != nil {
		t.Fatalf("SetPasswordPolicy: %v", err)
	}
	if after := fixture.countRows(t, countPolicyEvents); after != before+1 {
		t.Fatalf("policy change appended %d events, want exactly 1", after-before)
	}

	fixture.rejectAuditInsertions(t)
	if err := fixture.service.SetPasswordPolicy(
		ctx, administrator, auth.PasswordPolicy{MinimumLength: 32},
	); err == nil {
		t.Fatal("SetPasswordPolicy succeeded while its audit event could not be written")
	}
	policy, err := fixture.service.PasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("PasswordPolicy: %v", err)
	}
	if policy != tightened {
		t.Fatalf("policy = %#v after an unattributable change, want the previous %#v", policy, tightened)
	}
}
