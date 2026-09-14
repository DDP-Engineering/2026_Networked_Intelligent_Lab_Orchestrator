package appliance_test

import (
	"context"
	"errors"
	"testing"

	"nilo.local/nilo/internal/appliance"
)

// stubVerifierMatcher stands in for the identity service: it reports what one
// identity's stored verifier matches, saying nothing about who could sign in.
type stubVerifierMatcher struct {
	username string
	password string
	fail     error
	calls    int
}

func (matcher *stubVerifierMatcher) PasswordVerifierMatches(_ context.Context, username string, password []byte) (bool, error) {
	matcher.calls++
	if matcher.fail != nil {
		return false, matcher.fail
	}
	return username == matcher.username && string(password) == matcher.password, nil
}

// REQ-SEC-c9e034de: the adapter answers whether the credential published with
// the image still opens this appliance, and nothing else about it.
func TestPublishedCredentialReportsWhetherThePublishedVerifierRemains(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for _, testCase := range []struct {
		name    string
		matcher *stubVerifierMatcher
		want    bool
	}{
		{
			name:    "the published password is still in place",
			matcher: &stubVerifierMatcher{username: appliance.PublishedAdministratorUsername, password: appliance.PublishedAdministratorPassword},
			want:    true,
		},
		{
			name:    "the published password has been changed",
			matcher: &stubVerifierMatcher{username: appliance.PublishedAdministratorUsername, password: "something-else-entirely"},
			want:    false,
		},
		{
			name:    "the administrator was created under another name",
			matcher: &stubVerifierMatcher{username: "bench-lead", password: appliance.PublishedAdministratorPassword},
			want:    false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			credential := appliance.NewPublishedCredential(testCase.matcher)
			stillOpens, err := credential.PublishedVerifierRemains(ctx)
			if err != nil {
				t.Fatalf("PublishedVerifierRemains: %v", err)
			}
			if stillOpens != testCase.want {
				t.Fatalf("PublishedVerifierRemains = %t, want %t", stillOpens, testCase.want)
			}
		})
	}
}

// REQ-SEC-c9e034de: a checker that cannot answer reports an error, so the
// commissioning gate refuses rather than passing on a silent false.
func TestPublishedCredentialReportsWhenItCannotAnswer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	unavailable := errors.New("identity store unavailable")

	if _, err := appliance.NewPublishedCredential(&stubVerifierMatcher{fail: unavailable}).PublishedVerifierRemains(ctx); !errors.Is(err, unavailable) {
		t.Fatalf("PublishedVerifierRemains with a failing holder = %v, want the holder's error", err)
	}
	if _, err := appliance.NewPublishedCredential(nil).PublishedVerifierRemains(ctx); err == nil {
		t.Fatal("PublishedVerifierRemains without a holder answered instead of reporting that it cannot")
	}
}

// REQ-SEC-c9e034de: the published password is a published fact rather than a
// secret, but it still has to satisfy the ordinary password rule, because the
// appliance never holds a credential the rule would refuse to set.
func TestPublishedAdministratorPasswordSatisfiesTheOrdinaryPasswordRule(t *testing.T) {
	t.Parallel()

	if appliance.PublishedAdministratorUsername == "" {
		t.Fatal("the published administrator user name is empty")
	}
	if got := len(appliance.PublishedAdministratorPassword); got < 12 {
		t.Fatalf("published administrator password is %d bytes, below the ordinary minimum", got)
	}
}
