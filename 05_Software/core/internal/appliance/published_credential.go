package appliance

import (
	"context"
	"errors"
)

// PublishedAdministratorUsername and PublishedAdministratorPassword are the
// credential every appliance image ships with, so somebody who has just imaged
// an appliance can sign in and commission it without being given anything and
// without a console step.
//
// They are published, not secret: they appear in the deployment documentation,
// and anyone holding an image holds them. What keeps that acceptable is that
// commissioning refuses to complete while this password is still in place, so
// the exposure cannot outlive first boot. The password is an ordinary password
// in every other respect and is held to the ordinary rule when it is replaced.
const (
	PublishedAdministratorUsername = "admin"
	PublishedAdministratorPassword = "admin_password"
)

// VerifierMatcher answers whether one identity's stored verifier matches a
// password. It is the narrow part of the identity service this adapter needs,
// named here so the appliance package does not depend on the whole of it.
type VerifierMatcher interface {
	PasswordVerifierMatches(ctx context.Context, username string, password []byte) (bool, error)
}

// PublishedCredential reports whether the credential published with the
// appliance image still opens this appliance. It is the adapter between the
// network service, which decides whether commissioning may complete, and the
// identity service, which owns what a stored verifier accepts; neither learns
// about the other.
type PublishedCredential struct {
	matcher VerifierMatcher
}

// NewPublishedCredential returns an adapter over matcher.
func NewPublishedCredential(matcher VerifierMatcher) *PublishedCredential {
	return &PublishedCredential{matcher: matcher}
}

// PublishedVerifierRemains reports whether the appliance still stores a
// verifier matching the published password under the published user name.
//
// It deliberately asks about the stored verifier rather than about who could
// sign in. An identity that is currently disabled still answers true, because
// disabling is reversible: letting a disabled published credential through
// commissioning would leave it to be re-enabled afterwards, on an appliance
// whose first-boot exposure is supposed to be over. How the identity was
// created does not enter into it either - an administrator created at a console
// who chose the published password holds the published credential just as
// surely as a seeded one does.
//
// It answers a single boolean and never the credential itself, so it cannot
// become a way to read the published password out of a running appliance.
//
// A credential it cannot check is an error rather than a false, because the
// commissioning gate refuses on an error and would pass on a false: a gate that
// cannot answer must not silently let the published credential through.
func (credential *PublishedCredential) PublishedVerifierRemains(ctx context.Context) (bool, error) {
	if credential == nil || credential.matcher == nil {
		return false, errors.New("no identity service is configured to check the published credential against")
	}
	return credential.matcher.PasswordVerifierMatches(
		ctx, PublishedAdministratorUsername, []byte(PublishedAdministratorPassword))
}

// ErrPublishedCredentialPresent is a commissioning attempt on an appliance that
// still stores the published verifier, refused by the transaction that would
// otherwise have recorded it commissioned.
//
// It reports itself through a method rather than only through its identity, so
// the network service can recognize it without importing this package and
// without the two packages having to agree on anything but the question.
var ErrPublishedCredentialPresent error = publishedCredentialPresent{}

type publishedCredentialPresent struct{}

func (publishedCredentialPresent) Error() string {
	return "the appliance still stores the published administrator verifier"
}

// PublishedCredentialPresent identifies this refusal to a caller that knows
// nothing else about it.
func (publishedCredentialPresent) PublishedCredentialPresent() bool { return true }
