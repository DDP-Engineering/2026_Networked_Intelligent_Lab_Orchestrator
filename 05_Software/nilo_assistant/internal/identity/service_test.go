package identity_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

const (
	administratorPassword = "administrator-secret-1"
	operatorPassword      = "operator-secret-1"
	replacementPassword   = "replacement-secret-1"
)

type fixture struct {
	service  *identity.Service
	audit    audit.Repository
	clock    *testClock
	database *sqlite.DB
}

type testClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *testClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *testClock) Advance(delta time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(delta)
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return newFixtureAt(t, filepath.Join(t.TempDir(), "nilo.db"))
}

// newFixtureAt composes one service over one database file. A second fixture on
// the same path is a second connection to the same appliance, which is how a
// concurrent administrator in another process is represented here.
func newFixtureAt(t *testing.T, path string) fixture {
	t.Helper()
	database, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open identity database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	clock := &testClock{now: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)}
	auditRepository := sqlite.NewAuditRepository(database)
	auditWriter, err := audit.NewWriter(audit.WriterOptions{
		Events: auditRepository, ApplianceID: "test-appliance", BootID: "test-boot", Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	service, err := identity.NewService(identity.Options{
		Repository:         sqlite.NewIdentityRepository(database),
		Now:                clock.Now,
		Random:             rand.Reader,
		SessionLifetime:    12 * time.Hour,
		LoginFailureLimit:  3,
		LoginFailureWindow: 15 * time.Minute,
		Audit:              auditWriter,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return fixture{service: service, audit: auditRepository, clock: clock, database: database}
}

// rejectAuditInsertions makes the real database refuse every audit insertion,
// which provokes an unattributable administrative change without replacing any
// repository with a fake.
func (f fixture) rejectAuditInsertions(t *testing.T) {
	t.Helper()
	if _, err := f.database.SQL().ExecContext(context.Background(), `
		CREATE TRIGGER reject_audit_events BEFORE INSERT ON audit_events
		BEGIN SELECT RAISE(ABORT, 'audit insertion rejected'); END
	`); err != nil {
		t.Fatalf("install the audit rejection: %v", err)
	}
}

func (f fixture) bootstrap(t *testing.T) auth.User {
	t.Helper()
	administrator, err := f.service.Bootstrap(context.Background(), "admin", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return administrator
}

func TestBootstrapCreatesTheFirstAdministratorAndThenRefuses(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if administrator.Role != auth.RoleAdministrator || !administrator.Enabled() {
		t.Fatalf("bootstrapped identity = %#v", administrator)
	}

	_, err := f.service.Bootstrap(context.Background(), "second", []byte(administratorPassword))
	if !errors.Is(err, identity.ErrAdministratorExists) {
		t.Fatalf("second Bootstrap error = %v, want ErrAdministratorExists", err)
	}
	users, err := f.service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("ListUsers returned %d identities, want 1", len(users))
	}
}

func TestAnAdministratorMayBeDisabledWhileAnotherRemainsEnabled(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	second, err := f.service.CreateUser(
		context.Background(), administrator, "second-admin", auth.RoleAdministrator, []byte(operatorPassword),
	)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), second, "admin", false); err != nil {
		t.Fatalf("disable one of two administrators: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), second, "second-admin", false); !errors.Is(err, identity.ErrLastAdministrator) {
		t.Fatalf("disabling the last enabled administrator = %v, want ErrLastAdministrator", err)
	}
}

func TestLastEnabledAdministratorCannotBeDisabledOrDemoted(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)

	if err := f.service.SetEnabled(context.Background(), administrator, "admin", false); !errors.Is(err, identity.ErrLastAdministrator) {
		t.Fatalf("disable error = %v, want ErrLastAdministrator", err)
	}
	if err := f.service.SetRole(context.Background(), administrator, "admin", auth.RoleOperator); !errors.Is(err, identity.ErrLastAdministrator) {
		t.Fatalf("demote error = %v, want ErrLastAdministrator", err)
	}
	stored, err := f.service.UserByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if stored.Role != auth.RoleAdministrator || !stored.Enabled() {
		t.Fatalf("rejected changes still altered the identity: %#v", stored)
	}
}

func TestLoginAcceptsCorrectCredentialsAndRefusesEverythingElse(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	result, err := f.service.Login(context.Background(), "Operator", []byte(operatorPassword))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.User.Username != "operator" || result.Session.Secret == "" || result.Session.CSRFToken == "" {
		t.Fatalf("login result = %#v", result)
	}
	if !result.Session.ExpiresAt.Equal(f.clock.Now().Add(12 * time.Hour)) {
		t.Fatalf("session expiry = %s, want %s", result.Session.ExpiresAt, f.clock.Now().Add(12*time.Hour))
	}

	for _, test := range []struct {
		name     string
		username string
		password string
	}{
		{name: "wrong password", username: "operator", password: "definitely-wrong-1"},
		{name: "unknown user", username: "nobody", password: operatorPassword},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.service.Login(context.Background(), test.username, []byte(test.password)); !errors.Is(err, identity.ErrInvalidCredentials) {
				t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

func TestLoginIsThrottledAfterRepeatedFailuresUntilTheWindowElapses(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.bootstrap(t)

	for attempt := 0; attempt < 3; attempt++ {
		if _, err := f.service.Login(context.Background(), "admin", []byte("wrong-password-000")); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("attempt %d error = %v, want ErrInvalidCredentials", attempt, err)
		}
	}
	if _, err := f.service.Login(context.Background(), "admin", []byte(administratorPassword)); !errors.Is(err, identity.ErrThrottled) {
		t.Fatalf("throttled login error = %v, want ErrThrottled", err)
	}

	f.clock.Advance(15 * time.Minute)
	if _, err := f.service.Login(context.Background(), "admin", []byte(administratorPassword)); err != nil {
		t.Fatalf("login after the throttle window: %v", err)
	}
}

func TestSessionStopsAuthenticatingAfterItsExpiry(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.bootstrap(t)
	result, err := f.service.Login(context.Background(), "admin", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := f.service.AuthenticateSession(context.Background(), result.Session.Secret); err != nil {
		t.Fatalf("AuthenticateSession before expiry: %v", err)
	}

	f.clock.Advance(12 * time.Hour)

	if _, err := f.service.AuthenticateSession(context.Background(), result.Session.Secret); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("AuthenticateSession after expiry = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogoutInvalidatesOnlyThatSession(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.bootstrap(t)
	first, err := f.service.Login(context.Background(), "admin", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("first Login: %v", err)
	}
	second, err := f.service.Login(context.Background(), "admin", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("second Login: %v", err)
	}

	if err := f.service.Logout(context.Background(), first.Session.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := f.service.AuthenticateSession(context.Background(), first.Session.Secret); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("logged-out session still authenticates: %v", err)
	}
	if _, err := f.service.AuthenticateSession(context.Background(), second.Session.Secret); err != nil {
		t.Fatalf("logout invalidated an unrelated session: %v", err)
	}
}

func TestDisablingOrResettingAnIdentityInvalidatesItsSessions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		apply func(*testing.T, fixture, auth.User)
	}{
		{
			name: "disable",
			apply: func(t *testing.T, f fixture, actor auth.User) {
				if err := f.service.SetEnabled(context.Background(), actor, "operator", false); err != nil {
					t.Fatalf("SetEnabled: %v", err)
				}
			},
		},
		{
			name: "password reset",
			apply: func(t *testing.T, f fixture, actor auth.User) {
				if err := f.service.ResetPassword(context.Background(), actor, "operator", []byte(replacementPassword)); err != nil {
					t.Fatalf("ResetPassword: %v", err)
				}
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			administrator := f.bootstrap(t)
			if _, err := f.service.CreateUser(
				context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
			); err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			result, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword))
			if err != nil {
				t.Fatalf("Login: %v", err)
			}

			test.apply(t, f, administrator)

			if _, err := f.service.AuthenticateSession(context.Background(), result.Session.Secret); !errors.Is(err, identity.ErrInvalidCredentials) {
				t.Fatalf("session survived %s: %v", test.name, err)
			}
		})
	}
}

func TestDisabledIdentityCannotLogInAndReEnablingRestoresLogin(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if _, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword)); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("disabled identity logged in: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	if _, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword)); err != nil {
		t.Fatalf("re-enabled identity could not log in: %v", err)
	}
}

func TestResetPasswordReplacesTheOldPassword(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := f.service.ResetPassword(context.Background(), administrator, "operator", []byte(replacementPassword)); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword)); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatal("the replaced password still logs in")
	}
	if _, err := f.service.Login(context.Background(), "operator", []byte(replacementPassword)); err != nil {
		t.Fatalf("the new password does not log in: %v", err)
	}
}

func TestRoleChangeAppliesToTheNextAuthorizationCheck(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	result, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	actor, err := f.service.AuthenticateSession(context.Background(), result.Session.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if !actor.Has(auth.PermissionTokensManageSelf) {
		t.Fatal("Operator session lacks its own token permission")
	}

	if err := f.service.SetRole(context.Background(), administrator, "operator", auth.RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}

	actor, err = f.service.AuthenticateSession(context.Background(), result.Session.Secret)
	if err != nil {
		t.Fatalf("AuthenticateSession after the role change: %v", err)
	}
	if actor.Has(auth.PermissionTokensManageSelf) {
		t.Fatal("the demoted session kept a permission the new role does not carry")
	}
	if !actor.Has(auth.PermissionStatusRead) {
		t.Fatal("the demoted session lost a permission the new role does carry")
	}
}

func TestTokenSecretIsDisclosedOnceAndNeverStored(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	issued, err := f.service.CreateToken(
		context.Background(), administrator, "admin", "continuous-integration", nil, nil,
	)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if issued.Secret == "" {
		t.Fatal("CreateToken disclosed no secret")
	}

	views, err := f.service.ListTokens(context.Background())
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(views) != 1 || views[0].Token.Name != "continuous-integration" || views[0].OwnerUsername != "admin" {
		t.Fatalf("ListTokens = %#v", views)
	}
	rendered, err := json.Marshal(views)
	if err != nil {
		t.Fatalf("marshal token views: %v", err)
	}
	if strings.Contains(string(rendered), issued.Secret) {
		t.Fatal("the token listing exposed the token secret")
	}
	if _, err := f.service.AuthenticateToken(context.Background(), issued.Secret); err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
}

func TestTokenPermissionsAreScopesIntersectedWithTheOwnerRole(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	escalating, err := f.service.CreateToken(
		context.Background(), administrator, "operator", "escalating",
		[]auth.Permission{auth.PermissionUsersManage, auth.PermissionStatusRead}, nil,
	)
	if err != nil {
		t.Fatalf("CreateToken escalating: %v", err)
	}
	actor, err := f.service.AuthenticateToken(context.Background(), escalating.Secret)
	if err != nil {
		t.Fatalf("AuthenticateToken escalating: %v", err)
	}
	if actor.Has(auth.PermissionUsersManage) {
		t.Fatal("a scope granted a permission the owner's role does not carry")
	}
	if !actor.Has(auth.PermissionStatusRead) {
		t.Fatal("a scope inside the owner's role was not granted")
	}

	unnarrowed, err := f.service.CreateToken(context.Background(), administrator, "operator", "unnarrowed", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken unnarrowed: %v", err)
	}
	actor, err = f.service.AuthenticateToken(context.Background(), unnarrowed.Secret)
	if err != nil {
		t.Fatalf("AuthenticateToken unnarrowed: %v", err)
	}
	if !actor.Has(auth.PermissionTokensReadSelf) {
		t.Fatal("an unscoped token did not inherit the owner's role")
	}

	if err := f.service.SetRole(context.Background(), administrator, "operator", auth.RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	actor, err = f.service.AuthenticateToken(context.Background(), unnarrowed.Secret)
	if err != nil {
		t.Fatalf("AuthenticateToken after demotion: %v", err)
	}
	if actor.Has(auth.PermissionTokensReadSelf) {
		t.Fatal("demoting the owner did not narrow the token")
	}
}

func TestUnusableTokensAreRejected(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	expiry := f.clock.Now().Add(time.Hour)
	expiring, err := f.service.CreateToken(context.Background(), administrator, "admin", "expiring", nil, &expiry)
	if err != nil {
		t.Fatalf("CreateToken expiring: %v", err)
	}
	revoked, err := f.service.CreateToken(context.Background(), administrator, "admin", "revoked", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken revoked: %v", err)
	}
	orphaned, err := f.service.CreateToken(context.Background(), administrator, "operator", "orphaned", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken orphaned: %v", err)
	}
	surviving, err := f.service.CreateToken(context.Background(), administrator, "admin", "surviving", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken surviving: %v", err)
	}

	if err := f.service.RevokeToken(context.Background(), administrator, revoked.Token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	f.clock.Advance(time.Hour)

	for _, test := range []struct {
		name   string
		secret string
	}{
		{name: "expired", secret: expiring.Secret},
		{name: "revoked", secret: revoked.Secret},
		{name: "disabled owner", secret: orphaned.Secret},
		{name: "unknown", secret: "nilo_not-a-real-token"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.service.AuthenticateToken(context.Background(), test.secret); !errors.Is(err, identity.ErrInvalidCredentials) {
				t.Fatalf("AuthenticateToken(%s) = %v, want ErrInvalidCredentials", test.name, err)
			}
		})
	}
	if _, err := f.service.AuthenticateToken(context.Background(), surviving.Secret); err != nil {
		t.Fatalf("an unaffected token stopped working: %v", err)
	}
}

func TestIdentityAndTokenChangesAreAuditedWithoutSecrets(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := f.service.ListUsers(context.Background()); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if err := f.service.SetRole(context.Background(), administrator, "operator", auth.RoleOperator); err != nil {
		t.Fatalf("SetRole no-op: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", true); err != nil {
		t.Fatalf("SetEnabled no-op: %v", err)
	}
	if _, err := f.service.ListTokens(context.Background()); err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if err := f.service.SetRole(context.Background(), administrator, "operator", auth.RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	if err := f.service.ResetPassword(context.Background(), administrator, "operator", []byte(replacementPassword)); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	issued, err := f.service.CreateToken(context.Background(), administrator, "operator", "audited", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := f.service.RevokeToken(context.Background(), administrator, issued.Token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	events, err := f.audit.List(context.Background(), audit.Filter{
		Category:  audit.CategoryAudit,
		Component: "identity",
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	want := []string{
		"identity.user.created",
		"identity.user.created",
		"identity.user.role_changed",
		"identity.user.disabled",
		"identity.user.enabled",
		"identity.user.password_reset",
		"identity.token.created",
		"identity.token.revoked",
	}
	if len(events) != len(want) {
		t.Fatalf("audit history has %d events, want %d: %#v", len(events), len(want), events)
	}
	for index, action := range want {
		if events[index].Action != action {
			t.Fatalf("audit event %d action = %q, want %q", index, events[index].Action, action)
		}
		if events[index].EnvelopeVersion != audit.EnvelopeVersion ||
			events[index].ApplianceID != "test-appliance" || events[index].BootID != "test-boot" {
			t.Fatalf("audit event %d has incomplete envelope: %#v", index, events[index])
		}
		if events[index].Category != audit.CategoryAudit || events[index].Component != "identity" ||
			events[index].ActorKind != audit.ActorKindUser || events[index].Outcome != audit.OutcomeApplied ||
			events[index].Severity != audit.SeverityInfo {
			t.Fatalf("audit event %d has wrong common fields: %#v", index, events[index])
		}
		if events[index].ActorID.String() != administrator.ID {
			t.Fatalf("audit event %d actor = %s, want %s", index, events[index].ActorID, administrator.ID)
		}
		payload := string(events[index].Payload)
		for _, secret := range []string{administratorPassword, operatorPassword, replacementPassword, issued.Secret} {
			if strings.Contains(payload, secret) {
				t.Fatalf("audit event %d leaked a secret: %s", index, payload)
			}
		}
	}
}

// TestConcurrentSelfDemotionsLeaveOneEnabledAdministrator proves the last
// administrator is an invariant rather than a check: two administrators demote
// themselves at the same time through separate connections to the same
// database, so a count read outside the change it guards lets both through.
func TestConcurrentSelfDemotionsLeaveOneEnabledAdministrator(t *testing.T) {
	t.Parallel()

	for range 4 {
		path := filepath.Join(t.TempDir(), "nilo.db")
		first := newFixtureAt(t, path)
		administrator := first.bootstrap(t)
		second, err := first.service.CreateUser(
			context.Background(), administrator, "second-admin", auth.RoleAdministrator, []byte(operatorPassword),
		)
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		other := newFixtureAt(t, path)

		start := make(chan struct{})
		results := make([]error, 2)
		var running sync.WaitGroup
		running.Add(2)
		go func() {
			defer running.Done()
			<-start
			results[0] = first.service.SetRole(context.Background(), administrator, "admin", auth.RoleOperator)
		}()
		go func() {
			defer running.Done()
			<-start
			results[1] = other.service.SetRole(context.Background(), second, "second-admin", auth.RoleOperator)
		}()
		close(start)
		running.Wait()

		applied := 0
		for index, err := range results {
			switch {
			case err == nil:
				applied++
			case errors.Is(err, identity.ErrLastAdministrator):
			default:
				t.Fatalf("concurrent demotion %d failed for an unrelated reason: %v", index, err)
			}
		}
		if applied != 1 {
			t.Fatalf("%d of two concurrent self-demotions were applied, want 1: %v", applied, results)
		}
		users, err := first.service.ListUsers(context.Background())
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		administrators := 0
		for _, user := range users {
			if user.Role == auth.RoleAdministrator && user.Enabled() {
				administrators++
			}
		}
		if administrators != 1 {
			t.Fatalf("enabled administrators after the concurrent demotions = %d, want 1", administrators)
		}
	}
}

// TestARejectedAuditRecordRollsBackTheAdministrativeChange proves that an
// administrative change and the record that attributes it are one unit: a
// change nobody can be held to account for must not survive.
func TestARejectedAuditRecordRollsBackTheAdministrativeChange(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	operator, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	issued, err := f.service.CreateToken(context.Background(), administrator, "operator", "kept", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	f.rejectAuditInsertions(t)

	if err := f.service.SetRole(context.Background(), administrator, "operator", auth.RoleAdministrator); err == nil {
		t.Fatal("SetRole reported success while its audit record was rejected")
	}
	if err := f.service.SetEnabled(context.Background(), administrator, "operator", false); err == nil {
		t.Fatal("SetEnabled reported success while its audit record was rejected")
	}
	if err := f.service.ResetPassword(
		context.Background(), administrator, "operator", []byte(replacementPassword),
	); err == nil {
		t.Fatal("ResetPassword reported success while its audit record was rejected")
	}
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "unaudited", auth.RoleViewer, []byte(operatorPassword),
	); err == nil {
		t.Fatal("CreateUser reported success while its audit record was rejected")
	}
	if _, err := f.service.CreateToken(
		context.Background(), administrator, "operator", "unaudited", nil, nil,
	); err == nil {
		t.Fatal("CreateToken reported success while its audit record was rejected")
	}
	if err := f.service.RevokeToken(context.Background(), administrator, issued.Token.ID); err == nil {
		t.Fatal("RevokeToken reported success while its audit record was rejected")
	}

	stored, err := f.service.UserByUsername(context.Background(), "operator")
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if stored.ID != operator.ID || stored.Role != auth.RoleOperator || !stored.Enabled() {
		t.Fatalf("the unaudited changes were persisted anyway: %#v", stored)
	}
	if _, err := f.service.Login(context.Background(), "operator", []byte(operatorPassword)); err != nil {
		t.Fatalf("the unaudited password reset was persisted anyway: %v", err)
	}
	users, err := f.service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("identities = %d, want the two created before audit was rejected", len(users))
	}
	if _, err := f.service.AuthenticateToken(context.Background(), issued.Secret); err != nil {
		t.Fatalf("the unaudited revocation was persisted anyway: %v", err)
	}
	tokens, err := f.service.ListTokens(context.Background())
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens = %d, want only the one created before audit was rejected", len(tokens))
	}
}

func TestAdministratorLookupRefusesNonAdministratorsAndDisabledIdentities(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	if _, err := f.service.CreateUser(
		context.Background(), administrator, "operator", auth.RoleOperator, []byte(operatorPassword),
	); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := f.service.Administrator(context.Background(), "operator"); !errors.Is(err, identity.ErrNotAdministrator) {
		t.Fatalf("Administrator(operator) = %v, want ErrNotAdministrator", err)
	}
	if _, err := f.service.Administrator(context.Background(), "nobody"); err == nil {
		t.Fatal("Administrator accepted an unknown identity")
	}
	if resolved, err := f.service.Administrator(context.Background(), "admin"); err != nil || resolved.ID != administrator.ID {
		t.Fatalf("Administrator(admin) = %#v, %v", resolved, err)
	}
}
