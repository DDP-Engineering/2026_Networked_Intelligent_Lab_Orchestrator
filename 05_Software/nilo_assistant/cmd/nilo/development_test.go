package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/app"
)

func openApplication(t *testing.T) *app.App {
	t.Helper()
	application, err := app.New(context.Background(), app.Options{
		DataDir: t.TempDir(), BindAddress: "127.0.0.1:0", Now: nowUTC,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return application
}

// The development administrator exists so a developer can start the appliance
// and sign in, with no token to issue and no bootstrap to run by hand.
func TestTheDevelopmentAdministratorSignsInWithTheFixedPassword(t *testing.T) {
	t.Parallel()

	application := openApplication(t)
	created, err := ensureDevelopmentAdministrator(context.Background(), application, "simulation")
	if err != nil {
		t.Fatalf("ensureDevelopmentAdministrator: %v", err)
	}
	if !created {
		t.Fatal("the first call did not report creating the administrator")
	}

	result, err := application.IdentityService().Login(
		context.Background(), DevelopmentAdministratorUsername,
		[]byte(DevelopmentAdministratorPassword))
	if err != nil {
		t.Fatalf("the development administrator cannot sign in: %v", err)
	}
	if result.User.Role != "administrator" {
		t.Fatalf("role = %q, want administrator", result.User.Role)
	}
}

// A developer restarts the appliance constantly. Every restart must leave one
// administrator and a working credential, never a growing pile of identities.
func TestRestartingLeavesOneAdministratorAndAWorkingCredential(t *testing.T) {
	t.Parallel()

	application := openApplication(t)
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := ensureDevelopmentAdministrator(
			context.Background(), application, "simulation"); err != nil {
			t.Fatalf("start %d: %v", attempt+1, err)
		}
	}

	users, err := application.IdentityService().ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	administrators := 0
	for _, user := range users {
		if user.Role == "administrator" {
			administrators++
		}
	}
	if administrators != 1 {
		t.Fatalf("after three starts there are %d administrators, want 1", administrators)
	}
	if _, err := application.IdentityService().Login(
		context.Background(), DevelopmentAdministratorUsername,
		[]byte(DevelopmentAdministratorPassword)); err != nil {
		t.Fatalf("the credential stopped working after restarts: %v", err)
	}
}

// The whole point of a fixed password is that it is a development convenience.
// It must be impossible to carry into any other profile.
func TestTheDevelopmentAdministratorIsRefusedOutsideSimulation(t *testing.T) {
	t.Parallel()

	application := openApplication(t)
	for _, profile := range []string{"appliance", "production", ""} {
		_, err := ensureDevelopmentAdministrator(context.Background(), application, profile)
		if !errors.Is(err, errDevelopmentAdministratorProfile) {
			t.Fatalf("profile %q returned %v, want errDevelopmentAdministratorProfile", profile, err)
		}
		// Nothing may have been created on the way to refusing.
		if _, err := application.IdentityService().Login(
			context.Background(), DevelopmentAdministratorUsername,
			[]byte(DevelopmentAdministratorPassword)); err == nil {
			t.Fatalf("profile %q created the development administrator anyway", profile)
		}
	}
}

// A data directory left over from an earlier run already has this same
// development identity, with whatever password it was last given. The flag has
// to make the published credential work again, or it silently stops being a
// convenience at all — which is exactly what happened on the bench.
func TestTheDevelopmentAdministratorPasswordIsResetWhenItAlreadyExists(t *testing.T) {
	t.Parallel()

	application := openApplication(t)
	if _, err := application.IdentityService().Bootstrap(
		context.Background(), DevelopmentAdministratorUsername,
		[]byte("a-completely-different-password")); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	created, err := ensureDevelopmentAdministrator(context.Background(), application, "simulation")
	if err != nil {
		t.Fatalf("ensureDevelopmentAdministrator: %v", err)
	}
	if !created {
		t.Fatal("the existing development administrator was left unusable")
	}
	if _, err := application.IdentityService().Login(
		context.Background(), DevelopmentAdministratorUsername,
		[]byte(DevelopmentAdministratorPassword)); err != nil {
		t.Fatalf("the published credential still does not work: %v", err)
	}
}

// Someone else's administrator is not this one. It keeps its own password.
func TestAnExistingAdministratorIsNeverReplaced(t *testing.T) {
	t.Parallel()

	application := openApplication(t)
	const realPassword = "a-real-administrator-password"
	if _, err := application.IdentityService().Bootstrap(
		context.Background(), "operator-in-charge", []byte(realPassword)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	created, err := ensureDevelopmentAdministrator(context.Background(), application, "simulation")
	if err != nil {
		t.Fatalf("ensureDevelopmentAdministrator: %v", err)
	}
	if created {
		t.Fatal("an administrator already existed and a second was created")
	}
	if _, err := application.IdentityService().Login(
		context.Background(), "operator-in-charge", []byte(realPassword)); err != nil {
		t.Fatalf("the existing administrator was disturbed: %v", err)
	}
}

// The credentials are printed, because a fixed password nobody can see is worse
// than useless, and the notice has to say plainly what it is.
func TestTheDevelopmentAdministratorNoticeNamesTheCredentialsAndTheRisk(t *testing.T) {
	t.Parallel()

	notice := developmentAdministratorNotice(true)
	for _, expected := range []string{
		DevelopmentAdministratorUsername,
		DevelopmentAdministratorPassword,
		"development",
	} {
		if !strings.Contains(notice, expected) {
			t.Fatalf("the notice does not mention %q: %s", expected, notice)
		}
	}
	if strings.Contains(developmentAdministratorNotice(false), "created") {
		t.Fatal("the notice claims to have created an administrator when it did not")
	}
}

func nowUTC() time.Time { return time.Now().UTC() }
