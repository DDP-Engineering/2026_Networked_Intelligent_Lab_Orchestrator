package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// applyLegacyMigrations recreates the schema as it stood before identity and
// access control existed, so the forward migration can be proven against data
// that was written by the earlier code.
func applyLegacyMigrations(ctx context.Context, t *testing.T, path string) {
	t.Helper()
	database, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Fatalf("close legacy database: %v", err)
		}
	}()
	for _, migration := range migrations[:3] {
		if _, err := database.ExecContext(ctx, migration.sql); err != nil {
			t.Fatalf("apply legacy migration %s: %v", migration.version, err)
		}
		if _, err := database.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, digest, applied_at) VALUES (?, ?, ?)",
			migration.version,
			migrationDigest(migration.sql),
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			t.Fatalf("record legacy migration %s: %v", migration.version, err)
		}
	}
	if _, err := database.ExecContext(
		ctx,
		"INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"0199a3b0-0000-7000-8000-000000000001",
		"development-legacy",
		auth.DisabledDevelopmentPasswordVerifier,
		"2026-08-31T09:00:00Z",
		"2026-08-31T09:00:00Z",
	); err != nil {
		t.Fatalf("insert legacy identity: %v", err)
	}
	verifier := sha256.Sum256([]byte("nilo_legacy-token"))
	if _, err := database.ExecContext(
		ctx,
		"INSERT INTO mcp_tokens (id, user_id, verifier, created_at) VALUES (?, ?, ?, ?)",
		"0199a3b0-0000-7000-8000-000000000002",
		"0199a3b0-0000-7000-8000-000000000001",
		verifier[:],
		"2026-08-31T09:00:00Z",
	); err != nil {
		t.Fatalf("insert legacy token: %v", err)
	}
}

func TestForwardMigrationPreservesLegacyIdentitiesAsLeastPrivileged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	applyLegacyMigrations(ctx, t, path)

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewIdentityRepository(database)

	users, err := repository.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("migration kept %d identities, want 1", len(users))
	}
	if users[0].Username != "development-legacy" {
		t.Fatalf("migrated username = %q", users[0].Username)
	}
	if users[0].Role != auth.RoleViewer {
		t.Fatalf("migrated role = %q, want the least-privileged %q", users[0].Role, auth.RoleViewer)
	}
	if !users[0].Enabled() {
		t.Fatal("the migration disabled an existing identity")
	}

	token, err := repository.APITokenBySecret(ctx, "nilo_legacy-token")
	if err != nil {
		t.Fatalf("legacy token stopped authenticating: %v", err)
	}
	if token.UserID != users[0].ID || len(token.Scopes) != 0 || token.ExpiresAt != nil {
		t.Fatalf("migrated token = %#v, want an unscoped non-expiring token owned by the legacy identity", token)
	}
}

func TestIdentityRepositoryRoundTripsUsersTokensAndSessionsAcrossReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	repository := NewIdentityRepository(database)
	created := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	expiry := created.Add(24 * time.Hour)

	user := auth.User{
		ID:               "0199a3b0-0000-7000-8000-00000000000a",
		Username:         "operator",
		Role:             auth.RoleOperator,
		PasswordVerifier: "$argon2id$v=19$m=65536,t=3,p=2$placeholder$placeholder",
		CreatedAt:        created,
		UpdatedAt:        created,
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := repository.CreateUser(ctx, user); !errors.Is(err, auth.ErrUsernameTaken) {
		t.Fatalf("duplicate CreateUser = %v, want ErrUsernameTaken", err)
	}

	tokenVerifier := auth.TokenVerifier(sha256.Sum256([]byte("nilo_round-trip")))
	token := auth.APIToken{
		ID:        "0199a3b0-0000-7000-8000-00000000000b",
		UserID:    user.ID,
		Name:      "round-trip",
		Scopes:    []auth.Permission{auth.PermissionStatusRead, auth.PermissionInventoryRead},
		CreatedAt: created,
		ExpiresAt: &expiry,
	}
	if err := repository.CreateAPIToken(ctx, token, tokenVerifier); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	session := auth.SessionRecord{
		ID:           "0199a3b0-0000-7000-8000-00000000000c",
		UserID:       user.ID,
		CSRFVerifier: auth.ComputeSessionVerifier("csrf-value"),
		CreatedAt:    created,
		ExpiresAt:    expiry,
	}
	if err := repository.CreateSession(
		ctx, session, auth.ComputeSessionVerifier("session-value"), user.PasswordVerifier,
	); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	repository = NewIdentityRepository(reopened)

	storedUser, err := repository.UserByUsername(ctx, "operator")
	if err != nil {
		t.Fatalf("UserByUsername after reopen: %v", err)
	}
	if storedUser.Role != auth.RoleOperator || !storedUser.CreatedAt.Equal(created) {
		t.Fatalf("stored user = %#v", storedUser)
	}
	storedToken, err := repository.APITokenBySecret(ctx, "nilo_round-trip")
	if err != nil {
		t.Fatalf("APITokenBySecret after reopen: %v", err)
	}
	if storedToken.Name != "round-trip" || len(storedToken.Scopes) != 2 || storedToken.ExpiresAt == nil || !storedToken.ExpiresAt.Equal(expiry) {
		t.Fatalf("stored token = %#v", storedToken)
	}
	storedSession, err := repository.SessionBySecret(ctx, "session-value")
	if err != nil {
		t.Fatalf("SessionBySecret after reopen: %v", err)
	}
	if storedSession.UserID != user.ID || !storedSession.ExpiresAt.Equal(expiry) || storedSession.RevokedAt != nil {
		t.Fatalf("stored session = %#v", storedSession)
	}
	if storedSession.CSRFVerifier != auth.ComputeSessionVerifier("csrf-value") {
		t.Fatal("the stored CSRF verifier did not survive the reopen")
	}

	if err := repository.RevokeUserSessions(ctx, user.ID, expiry); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	if _, err := repository.SessionBySecret(ctx, "session-value"); !errors.Is(err, auth.ErrSessionRevoked) {
		t.Fatalf("SessionBySecret after revocation = %v, want ErrSessionRevoked", err)
	}
}

func TestIdentityRepositoryStoresNoTokenOrSessionPlaintext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewIdentityRepository(database)
	created := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)

	user := auth.User{
		ID: "0199a3b0-0000-7000-8000-00000000001a", Username: "secretless", Role: auth.RoleViewer,
		PasswordVerifier: "$argon2id$v=19$m=65536,t=3,p=2$placeholder$placeholder",
		CreatedAt:        created, UpdatedAt: created,
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	const tokenSecret = "nilo_never-stored"
	if err := repository.CreateAPIToken(ctx, auth.APIToken{
		ID: "0199a3b0-0000-7000-8000-00000000001b", UserID: user.ID, Name: "n", CreatedAt: created,
	}, auth.TokenVerifier(sha256.Sum256([]byte(tokenSecret)))); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	const sessionSecret = "session-never-stored"
	if err := repository.CreateSession(ctx, auth.SessionRecord{
		ID: "0199a3b0-0000-7000-8000-00000000001c", UserID: user.ID,
		CSRFVerifier: auth.ComputeSessionVerifier("csrf"), CreatedAt: created, ExpiresAt: created.Add(time.Hour),
	}, auth.ComputeSessionVerifier(sessionSecret), user.PasswordVerifier); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	for _, test := range []struct {
		query  string
		secret string
	}{
		{query: "SELECT COUNT(*) FROM mcp_tokens WHERE CAST(verifier AS TEXT) = ?", secret: tokenSecret},
		{query: "SELECT COUNT(*) FROM user_sessions WHERE CAST(verifier AS TEXT) = ?", secret: sessionSecret},
	} {
		var count int
		if err := database.SQL().QueryRowContext(ctx, test.query, test.secret).Scan(&count); err != nil {
			t.Fatalf("count stored plaintext: %v", err)
		}
		if count != 0 {
			t.Fatalf("a secret was stored in plaintext: %s", test.secret)
		}
	}
}

// TestAdministrativeTransactionsSerializeAcrossConnections proves the guard an
// administrative change relies on cannot read a database another connection is
// already changing: the second transaction waits for the first and then sees
// what it committed, which is what makes the guard hold between processes.
func TestAdministrativeTransactionsSerializeAcrossConnections(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	holder, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open holding connection: %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	waiter, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open waiting connection: %v", err)
	}
	t.Cleanup(func() { _ = waiter.Close() })

	created := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	administrator := auth.User{
		ID: "0199a3b0-0000-7000-8000-00000000002a", Username: "admin", Role: auth.RoleAdministrator,
		PasswordVerifier: "$argon2id$v=19$m=65536,t=3,p=2$placeholder$placeholder",
		CreatedAt:        created, UpdatedAt: created,
	}
	if err := NewIdentityRepository(holder).CreateUser(ctx, administrator); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	entered := make(chan struct{})
	proceed := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- NewIdentityRepository(holder).WithinAdministrativeTransaction(
			ctx, func(repository identity.Repository, _ audit.Repository) error {
				close(entered)
				<-proceed
				return repository.UpdateUserRole(ctx, administrator.ID, auth.RoleOperator, created)
			},
		)
	}()
	<-entered

	observed := make(chan int, 1)
	waiting := make(chan struct{})
	go func() {
		close(waiting)
		count := -1
		if err := NewIdentityRepository(waiter).WithinAdministrativeTransaction(
			ctx, func(repository identity.Repository, _ audit.Repository) error {
				value, err := repository.CountEnabledAdministrators(ctx)
				count = value
				return err
			},
		); err != nil {
			t.Errorf("waiting administrative transaction: %v", err)
		}
		observed <- count
	}()
	<-waiting
	// The waiting transaction has been started; give it its chance to reach the
	// database before the holder commits, so that observing the committed count
	// can only mean it waited.
	time.Sleep(100 * time.Millisecond)
	close(proceed)

	if err := <-held; err != nil {
		t.Fatalf("holding administrative transaction: %v", err)
	}
	if count := <-observed; count != 0 {
		t.Fatalf("the waiting transaction counted %d enabled administrators, want the committed 0", count)
	}
}

// TestCreateSessionRefusesASupersededPasswordVerifier proves a browser session
// is written only while the identity still carries the password the login
// checked and is still enabled.
func TestCreateSessionRefusesASupersededPasswordVerifier(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewIdentityRepository(database)

	created := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	const firstVerifier = "$argon2id$v=19$m=65536,t=3,p=2$first$first"
	const secondVerifier = "$argon2id$v=19$m=65536,t=3,p=2$second$second"
	user := auth.User{
		ID: "0199a3b0-0000-7000-8000-00000000003a", Username: "operator", Role: auth.RoleOperator,
		PasswordVerifier: firstVerifier, CreatedAt: created, UpdatedAt: created,
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := repository.UpdateUserPassword(ctx, user.ID, secondVerifier, created); err != nil {
		t.Fatalf("UpdateUserPassword: %v", err)
	}

	superseded := auth.SessionRecord{
		ID: "0199a3b0-0000-7000-8000-00000000003b", UserID: user.ID,
		CSRFVerifier: auth.ComputeSessionVerifier("csrf"), CreatedAt: created, ExpiresAt: created.Add(time.Hour),
	}
	if err := repository.CreateSession(
		ctx, superseded, auth.ComputeSessionVerifier("superseded"), firstVerifier,
	); !errors.Is(err, auth.ErrCredentialSuperseded) {
		t.Fatalf("CreateSession on the replaced password = %v, want ErrCredentialSuperseded", err)
	}
	if _, err := repository.SessionBySecret(ctx, "superseded"); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("a session was stored for the replaced password: %v", err)
	}

	current := superseded
	current.ID = "0199a3b0-0000-7000-8000-00000000003c"
	if err := repository.CreateSession(
		ctx, current, auth.ComputeSessionVerifier("current"), secondVerifier,
	); err != nil {
		t.Fatalf("CreateSession on the current password: %v", err)
	}

	disabledAt := created.Add(time.Minute)
	if err := repository.SetUserDisabled(ctx, user.ID, &disabledAt, disabledAt); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}
	disabled := superseded
	disabled.ID = "0199a3b0-0000-7000-8000-00000000003d"
	if err := repository.CreateSession(
		ctx, disabled, auth.ComputeSessionVerifier("disabled"), secondVerifier,
	); !errors.Is(err, auth.ErrCredentialSuperseded) {
		t.Fatalf("CreateSession for a disabled identity = %v, want ErrCredentialSuperseded", err)
	}
}

var _ identity.Repository = (*IdentityRepository)(nil)

// TestIdentityRepositoryRemovesRowsRatherThanMarkingThem states what
// permanent deletion means at the only layer that can answer it: the row is
// gone from the file, a session belonging to a removed identity is gone with
// it, and the foreign key still refuses to strand an owned token even if a
// caller above ever forgot to check.
func TestIdentityRepositoryRemovesRowsRatherThanMarkingThem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewIdentityRepository(database)
	created := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	expiry := created.Add(24 * time.Hour)

	user := auth.User{
		ID:               "0199a3b0-0000-7000-8000-00000000010a",
		Username:         "leaving",
		Role:             auth.RoleOperator,
		PasswordVerifier: "$argon2id$v=19$m=65536,t=3,p=2$placeholder$placeholder",
		CreatedAt:        created,
		UpdatedAt:        created,
	}
	if err := repository.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token := auth.APIToken{
		ID: "0199a3b0-0000-7000-8000-00000000010b", UserID: user.ID, Name: "owned",
		Scopes: []auth.Permission{auth.PermissionStatusRead}, CreatedAt: created, ExpiresAt: &expiry,
	}
	if err := repository.CreateAPIToken(
		ctx, token, auth.TokenVerifier(sha256.Sum256([]byte("nilo_owned"))),
	); err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	session := auth.SessionRecord{
		ID: "0199a3b0-0000-7000-8000-00000000010c", UserID: user.ID,
		CSRFVerifier: auth.ComputeSessionVerifier("csrf-value"),
		CreatedAt:    created, ExpiresAt: expiry,
	}
	if err := repository.CreateSession(
		ctx, session, auth.ComputeSessionVerifier("session-value"), user.PasswordVerifier,
	); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	owned, err := repository.CountAPITokensForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("CountAPITokensForUser: %v", err)
	}
	if owned != 1 {
		t.Fatalf("owned tokens = %d, want 1", owned)
	}
	if err := repository.RevokeAPIToken(ctx, token.ID, created.Add(time.Hour)); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if owned, err = repository.CountAPITokensForUser(ctx, user.ID); err != nil || owned != 1 {
		t.Fatalf("owned tokens after revocation = %d (%v), want 1: revoking is not removing", owned, err)
	}
	// The identity still owns a token, so the key refuses to remove it. This is
	// the last line of defence behind the service's own refusal.
	if err := repository.DeleteUser(ctx, user.ID); err == nil {
		t.Fatal("an identity that still owns a token was deleted")
	}

	stored, err := repository.APITokenByID(ctx, token.ID)
	if err != nil {
		t.Fatalf("APITokenByID: %v", err)
	}
	if stored.Name != "owned" || stored.UserID != user.ID || stored.RevokedAt == nil ||
		stored.ExpiresAt == nil || !stored.ExpiresAt.Equal(expiry) || len(stored.Scopes) != 1 {
		t.Fatalf("token read by identifier = %#v", stored)
	}
	if err := repository.DeleteAPIToken(ctx, token.ID); err != nil {
		t.Fatalf("DeleteAPIToken: %v", err)
	}
	if _, err := repository.APITokenByID(ctx, token.ID); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Fatalf("APITokenByID after deletion = %v, want ErrTokenNotFound", err)
	}
	if err := repository.DeleteAPIToken(ctx, token.ID); !errors.Is(err, auth.ErrTokenNotFound) {
		t.Fatalf("DeleteAPIToken twice = %v, want ErrTokenNotFound", err)
	}

	if err := repository.DeleteUserSessions(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUserSessions: %v", err)
	}
	if err := repository.DeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if err := repository.DeleteUser(ctx, user.ID); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("DeleteUser twice = %v, want ErrUserNotFound", err)
	}
	if _, err := repository.UserByUsername(ctx, "leaving"); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("UserByUsername after deletion = %v, want ErrUserNotFound", err)
	}
	if _, err := repository.SessionBySecret(ctx, "session-value"); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("SessionBySecret after deletion = %v, want ErrSessionNotFound", err)
	}

	// And the rows themselves, read straight out of the file the appliance
	// keeps: nothing marked, nothing left.
	for _, remaining := range []struct {
		what  string
		query string
	}{
		{"identities", "SELECT COUNT(*) FROM users WHERE id = ?"},
		{"browser sessions", "SELECT COUNT(*) FROM user_sessions WHERE user_id = ?"},
		{"API tokens", "SELECT COUNT(*) FROM mcp_tokens WHERE user_id = ?"},
	} {
		var count int
		if err := database.SQL().QueryRowContext(ctx, remaining.query, user.ID).Scan(&count); err != nil {
			t.Fatalf("count remaining %s: %v", remaining.what, err)
		}
		if count != 0 {
			t.Fatalf("%d %s rows survived the deletion", count, remaining.what)
		}
	}
}
