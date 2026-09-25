package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/store/sqlite"
)

const (
	bootstrapPassword   = "administrator-secret-1"
	operatorPassword    = "operator-secret-0001"
	replacementPassword = "replacement-secret-1"
)

type adminResult struct {
	stdout   string
	combined string
	exitCode int
}

func runAdmin(t *testing.T, binary, stdin string, arguments ...string) adminResult {
	t.Helper()
	command := exec.Command(binary, arguments...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := adminResult{stdout: stdout.String(), combined: stdout.String() + stderr.String()}
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", arguments, err)
		}
		result.exitCode = exitError.ExitCode()
	}
	return result
}

func mustRunAdmin(t *testing.T, binary, stdin string, arguments ...string) adminResult {
	t.Helper()
	result := runAdmin(t, binary, stdin, arguments...)
	if result.exitCode != 0 {
		t.Fatalf("%v exited %d: %s", arguments, result.exitCode, result.combined)
	}
	return result
}

func password(value string) string { return value + "\n" + value + "\n" }

func listUsers(t *testing.T, dataDir string) []auth.User {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open administered database: %v", err)
	}
	defer func() { _ = database.Close() }()
	users, err := sqlite.NewIdentityRepository(database).ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list administered identities: %v", err)
	}
	return users
}

func findUser(t *testing.T, dataDir, username string) auth.User {
	t.Helper()
	for _, user := range listUsers(t, dataDir) {
		if user.Username == username {
			return user
		}
	}
	t.Fatalf("identity %q not found in %s", username, dataDir)
	return auth.User{}
}

func TestAdminBootstrapCreatesTheFirstAdministratorAndThenRefuses(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()

	created := mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	var bootstrapped struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &bootstrapped); err != nil {
		t.Fatalf("decode bootstrap output %q: %v", created.stdout, err)
	}
	if bootstrapped.Username != "admin" || bootstrapped.Role != "administrator" || bootstrapped.UserID == "" {
		t.Fatalf("bootstrap output = %#v", bootstrapped)
	}
	if strings.Contains(created.combined, bootstrapPassword) {
		t.Fatalf("bootstrap printed the password: %s", created.combined)
	}

	refused := runAdmin(t, binary, password("second-administrator-1"),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "second")
	if refused.exitCode == 0 {
		t.Fatal("a second bootstrap succeeded")
	}
	if !strings.Contains(refused.combined, "administrator already exists") {
		t.Fatalf("second bootstrap message = %q", refused.combined)
	}
	if users := listUsers(t, dataDir); len(users) != 1 {
		t.Fatalf("the refused bootstrap left %d identities, want 1", len(users))
	}
}

func TestAdminBootstrapRefusesMismatchedOrWeakPasswords(t *testing.T) {
	binary := buildNilo(t)

	mismatched := runAdmin(t, binary, "first-password-value\nsecond-password-value\n",
		"admin", "bootstrap", "--data-dir", t.TempDir(), "--username", "admin")
	if mismatched.exitCode == 0 {
		t.Fatal("bootstrap accepted two different passwords")
	}
	weak := runAdmin(t, binary, password("short"),
		"admin", "bootstrap", "--data-dir", t.TempDir(), "--username", "admin")
	if weak.exitCode == 0 {
		t.Fatal("bootstrap accepted a password below the minimum length")
	}
}

func TestAdminUserCommandsManageIdentitiesWithoutPrintingPasswords(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")

	create := mustRunAdmin(t, binary, password(operatorPassword),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "operator", "--role", "operator")
	if strings.Contains(create.combined, operatorPassword) {
		t.Fatalf("user create printed the password: %s", create.combined)
	}
	if user := findUser(t, dataDir, "operator"); user.Role != auth.RoleOperator || !user.Enabled() {
		t.Fatalf("created identity = %#v", user)
	}

	listed := mustRunAdmin(t, binary, "", "admin", "user", "list", "--data-dir", dataDir)
	var listing struct {
		Items []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
			Enabled  bool   `json:"enabled"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listed.stdout), &listing); err != nil {
		t.Fatalf("decode user list %q: %v", listed.stdout, err)
	}
	if len(listing.Items) != 2 {
		t.Fatalf("user list = %#v, want two identities", listing.Items)
	}
	for _, secret := range []string{bootstrapPassword, operatorPassword} {
		if strings.Contains(listed.combined, secret) {
			t.Fatalf("user list printed a password: %s", listed.combined)
		}
	}

	mustRunAdmin(t, binary, "", "admin", "user", "set-role", "--data-dir", dataDir,
		"--actor", "admin", "--username", "operator", "--role", "viewer")
	if user := findUser(t, dataDir, "operator"); user.Role != auth.RoleViewer {
		t.Fatalf("role after set-role = %q, want viewer", user.Role)
	}

	mustRunAdmin(t, binary, "", "admin", "user", "disable", "--data-dir", dataDir,
		"--actor", "admin", "--username", "operator")
	if user := findUser(t, dataDir, "operator"); user.Enabled() {
		t.Fatal("disable left the identity enabled")
	}

	mustRunAdmin(t, binary, "", "admin", "user", "enable", "--data-dir", dataDir,
		"--actor", "admin", "--username", "operator")
	if user := findUser(t, dataDir, "operator"); !user.Enabled() {
		t.Fatal("enable left the identity disabled")
	}

	before := findUser(t, dataDir, "operator").PasswordVerifier
	reset := mustRunAdmin(t, binary, password(replacementPassword),
		"admin", "user", "reset-password", "--data-dir", dataDir, "--actor", "admin", "--username", "operator")
	if strings.Contains(reset.combined, replacementPassword) {
		t.Fatalf("reset-password printed the password: %s", reset.combined)
	}
	if after := findUser(t, dataDir, "operator").PasswordVerifier; after == before {
		t.Fatal("reset-password did not replace the stored verifier")
	}
}

func TestAdminUserCommandsRefuseANonAdministratorActorAndTheLastAdministrator(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	mustRunAdmin(t, binary, password(operatorPassword),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "operator", "--role", "operator")

	impostor := runAdmin(t, binary, password("another-secret-value-1"),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "operator",
		"--username", "sneaky", "--role", "administrator")
	if impostor.exitCode == 0 {
		t.Fatal("a non-administrator actor created an identity")
	}
	if len(listUsers(t, dataDir)) != 2 {
		t.Fatal("the refused creation still wrote an identity")
	}

	disabled := runAdmin(t, binary, "", "admin", "user", "disable", "--data-dir", dataDir,
		"--actor", "admin", "--username", "admin")
	if disabled.exitCode == 0 {
		t.Fatal("the last enabled administrator was disabled")
	}
	demoted := runAdmin(t, binary, "", "admin", "user", "set-role", "--data-dir", dataDir,
		"--actor", "admin", "--username", "admin", "--role", "viewer")
	if demoted.exitCode == 0 {
		t.Fatal("the last enabled administrator was demoted")
	}
	if administrator := findUser(t, dataDir, "admin"); administrator.Role != auth.RoleAdministrator || !administrator.Enabled() {
		t.Fatalf("the last administrator changed anyway: %#v", administrator)
	}
}

// TestAdminUserListLeavesRunningSessionsAndTheirLeasesUntouched proves that a
// local administration command is not a server start: it opens the same data
// directory a running server owns, so declaring that server's work interrupted
// would destroy live operations.
func TestAdminUserListLeavesRunningSessionsAndTheirLeasesUntouched(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	running, resource := seedRunningSession(t, dataDir)

	mustRunAdmin(t, binary, "", "admin", "user", "list", "--data-dir", dataDir)

	state, leases := readSessionState(t, dataDir, running)
	if state != string(domain.Running) {
		t.Fatalf("session state after the listing = %q, want %q", state, domain.Running)
	}
	if leases != 1 {
		t.Fatalf("active leases on %q after the listing = %d, want 1", resource, leases)
	}
}

// seedRunningSession writes one running session holding one active lease, the
// state a server that is currently executing an operation keeps on disk.
func seedRunningSession(t *testing.T, dataDir string) (domain.SessionID, string) {
	t.Helper()
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open seeded database: %v", err)
	}
	defer func() { _ = database.Close() }()

	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	requestID, err := domain.NewRequestID()
	if err != nil {
		t.Fatalf("NewRequestID: %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, 'echo', ?, '{}', ?, ?, ?, ?)
	`, requestID.String(), "018f3d8d-8d2c-7c20-8e0c-000000000302", "018f3d8d-8d2c-7c20-8e0c-000000000303",
		"018f3d8d-8d2c-7c20-8e0c-000000000304", "018f3d8d-8d2c-7c20-8e0c-000000000305",
		"018f3d8d-8d2c-7c20-8e0c-000000000306", now.Add(time.Hour).Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed operation request: %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	repository := sqlite.NewSessionRepository(database)
	queued, err := domain.NewSession(requestID, domain.Queued, now)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := repository.Create(ctx, queued); err != nil {
		t.Fatalf("create seeded session: %v", err)
	}
	const resource = "018f3d8d-8d2c-7c20-8e0c-000000000303/device/stable-001"
	if err := repository.AcquireLeases(ctx, queued.ID, []string{resource}, now); err != nil {
		t.Fatalf("acquire seeded lease: %v", err)
	}
	if _, err := repository.Transition(ctx, queued.ID, domain.Queued, domain.Running, now); err != nil {
		t.Fatalf("run seeded session: %v", err)
	}
	return queued.ID, resource
}

func readSessionState(t *testing.T, dataDir string, id domain.SessionID) (string, int) {
	t.Helper()
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("reopen administered database: %v", err)
	}
	defer func() { _ = database.Close() }()

	var state string
	if err := database.SQL().QueryRowContext(
		ctx, "SELECT state FROM sessions WHERE id = ?", id.String(),
	).Scan(&state); err != nil {
		t.Fatalf("read seeded session state: %v", err)
	}
	var leases int
	if err := database.SQL().QueryRowContext(
		ctx, "SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL", id.String(),
	).Scan(&leases); err != nil {
		t.Fatalf("count seeded active leases: %v", err)
	}
	return state, leases
}

func TestAdminTokenCommandsDiscloseTheSecretOnceAndNeverAgain(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	mustRunAdmin(t, binary, password(operatorPassword),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "operator", "--role", "operator")

	created := mustRunAdmin(t, binary, "", "admin", "token", "create", "--data-dir", dataDir,
		"--actor", "admin", "--owner", "operator", "--name", "continuous-integration",
		"--scopes", "status.read,inventory.read", "--expires-in", "720h")
	var issued struct {
		TokenID   string   `json:"token_id"`
		Name      string   `json:"name"`
		Owner     string   `json:"owner"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at"`
		Token     string   `json:"token"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &issued); err != nil {
		t.Fatalf("decode token creation %q: %v", created.stdout, err)
	}
	if issued.Token == "" || issued.TokenID == "" || issued.Owner != "operator" || issued.ExpiresAt == "" {
		t.Fatalf("token creation output = %#v", issued)
	}
	if len(issued.Scopes) != 2 {
		t.Fatalf("token scopes = %v, want the two requested scopes", issued.Scopes)
	}

	second := mustRunAdmin(t, binary, "", "admin", "token", "create", "--data-dir", dataDir,
		"--actor", "admin", "--owner", "admin", "--name", "second")
	var other struct {
		TokenID string `json:"token_id"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal([]byte(second.stdout), &other); err != nil {
		t.Fatalf("decode second token creation: %v", err)
	}

	listed := mustRunAdmin(t, binary, "", "admin", "token", "list", "--data-dir", dataDir)
	for _, secret := range []string{issued.Token, other.Token} {
		if strings.Contains(listed.combined, secret) {
			t.Fatalf("token list exposed a secret: %s", listed.combined)
		}
	}
	var tokens struct {
		Items []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Owner   string   `json:"owner"`
			Scopes  []string `json:"scopes"`
			Revoked bool     `json:"revoked"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listed.stdout), &tokens); err != nil {
		t.Fatalf("decode token list %q: %v", listed.stdout, err)
	}
	if len(tokens.Items) != 2 {
		t.Fatalf("token list = %#v, want two tokens", tokens.Items)
	}

	mustRunAdmin(t, binary, "", "admin", "token", "revoke", "--data-dir", dataDir,
		"--actor", "admin", "--id", issued.TokenID)
	afterRevocation := mustRunAdmin(t, binary, "", "admin", "token", "list", "--data-dir", dataDir)
	if err := json.Unmarshal([]byte(afterRevocation.stdout), &tokens); err != nil {
		t.Fatalf("decode token list after revocation: %v", err)
	}
	for _, item := range tokens.Items {
		if item.ID == issued.TokenID && !item.Revoked {
			t.Fatal("the revoked token is not marked revoked")
		}
		if item.ID == other.TokenID && item.Revoked {
			t.Fatal("revoking one token revoked another")
		}
	}
}

func TestAdminTokenCreateRejectsUnknownScopesAndOwners(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")

	unknownScope := runAdmin(t, binary, "", "admin", "token", "create", "--data-dir", dataDir,
		"--actor", "admin", "--owner", "admin", "--name", "bad", "--scopes", "hardware.flash")
	if unknownScope.exitCode == 0 {
		t.Fatal("token create accepted a permission the model does not define")
	}
	unknownOwner := runAdmin(t, binary, "", "admin", "token", "create", "--data-dir", dataDir,
		"--actor", "admin", "--owner", "nobody", "--name", "bad")
	if unknownOwner.exitCode == 0 {
		t.Fatal("token create accepted an unknown owner")
	}
}

func TestDevelopmentTokenIdentityIsNotAdministrative(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, "", "development", "issue-token", "--data-dir", dataDir)

	users := listUsers(t, dataDir)
	if len(users) != 1 {
		t.Fatalf("development token issuance created %d identities, want 1", len(users))
	}
	if users[0].Role != auth.RoleViewer {
		t.Fatalf("development identity role = %q, want the least-privileged viewer", users[0].Role)
	}
}
