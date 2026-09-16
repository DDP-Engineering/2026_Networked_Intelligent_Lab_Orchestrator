package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/store/sqlite"
)

// The command is an API client like every other remote administration command:
// the appliance authorizes the token, applies the rule and answers. Nothing
// here decides what a policy may be.

func stubPasswordPolicy(minimumLength int, required ...string) map[string]any {
	policy := map[string]any{
		"minimum_length":    minimumLength,
		"require_lowercase": false,
		"require_uppercase": false,
		"require_digit":     false,
		"require_special":   false,
	}
	for _, field := range required {
		policy[field] = true
	}
	return policy
}

func TestRemotePasswordPolicyReadsWithNoValuesGiven(t *testing.T) {
	stub := newAdminStub(t)
	stub.answer("GET /api/v1/password-policy", http.StatusOK, stubPasswordPolicy(12))

	code, stdout, stderr := runAdminInProcess(
		stub.remote(t, "password-policy"), "", false)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	assertRequests(t, stub.requests(), "GET /api/v1/password-policy")
	for _, expected := range []string{"12", "none"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("output %q does not report %q", stdout, expected)
		}
	}
}

func TestRemotePasswordPolicySubmitsBothValues(t *testing.T) {
	stub := newAdminStub(t)
	stub.answer("PUT /api/v1/password-policy", http.StatusOK, stubPasswordPolicy(16, "require_uppercase", "require_digit"))

	code, stdout, stderr := runAdminInProcess(
		stub.remote(t, "password-policy", "--minimum-length", "16", "--require", "upper,digit"), "", false)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	requests := stub.requests()
	assertRequests(t, requests, "PUT /api/v1/password-policy")
	for _, expected := range []string{
		`"minimum_length":16`, `"require_uppercase":true`, `"require_digit":true`,
		`"require_lowercase":false`, `"require_special":false`,
	} {
		if !strings.Contains(strings.ReplaceAll(requests[0].body, " ", ""), expected) {
			t.Fatalf("request body %q does not carry %q", requests[0].body, expected)
		}
	}
	if !strings.Contains(stdout, "16") {
		t.Fatalf("output %q does not report what the appliance stored", stdout)
	}
}

// A partial change is refused before anything is contacted, for the reason the
// route refuses one: half a policy would silently relax the half not sent.
func TestRemotePasswordPolicyRefusesExactlyOneValue(t *testing.T) {
	for _, partial := range [][]string{
		{"--minimum-length", "16"},
		{"--require", "digit"},
	} {
		stub := newAdminStub(t)
		code, _, stderr := runAdminInProcess(
			stub.remote(t, append([]string{"password-policy"}, partial...)...), "", false)
		if code != 2 {
			t.Fatalf("exit = %d for %v, want 2", code, partial)
		}
		if len(stub.requests()) != 0 {
			t.Fatalf("a refused command contacted the appliance: %v", describeRequests(stub.requests()))
		}
		if stderr == "" {
			t.Fatal("the refusal said nothing")
		}
	}
}

// The appliance owns the rule, so its refusal is what the operator reads.
func TestRemotePasswordPolicyReportsTheAppliancesRefusal(t *testing.T) {
	stub := newAdminStub(t)
	stub.answer("PUT /api/v1/password-policy", http.StatusBadRequest, map[string]any{
		"error":   "invalid_password_policy",
		"message": "minimum length must be between 8 and 1024 bytes",
	})

	code, _, stderr := runAdminInProcess(
		stub.remote(t, "password-policy", "--minimum-length", "7", "--require", "none"), "", false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "8") {
		t.Fatalf("stderr %q does not carry the appliance's message naming the floor", stderr)
	}
}

// The policy is the appliance's, and the appliance authorizes the token on
// every request, so there is no local mode to run this against a data
// directory.
// A misspelt class would otherwise travel as "not required" and quietly weaken
// the policy the operator meant, so it is refused before anything is contacted.
func TestRemotePasswordPolicyRefusesAnUnknownClassName(t *testing.T) {
	stub := newAdminStub(t)
	code, _, stderr := runAdminInProcess(
		stub.remote(t, "password-policy", "--minimum-length", "16", "--require", "symbols"), "", false)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if len(stub.requests()) != 0 {
		t.Fatalf("a refused command contacted the appliance: %v", describeRequests(stub.requests()))
	}
	if !strings.Contains(stderr, "symbols") {
		t.Fatalf("stderr %q does not name the unknown class", stderr)
	}
}

// "none" is how an operator clears every requirement, rather than achieving it
// by leaving the option out.
func TestRemotePasswordPolicyClearsEveryRequirementWithNone(t *testing.T) {
	stub := newAdminStub(t)
	stub.answer("PUT /api/v1/password-policy", http.StatusOK, stubPasswordPolicy(16))
	code, _, stderr := runAdminInProcess(
		stub.remote(t, "password-policy", "--minimum-length", "16", "--require", "none"), "", false)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	body := strings.ReplaceAll(stub.requests()[0].body, " ", "")
	for _, field := range []string{"require_lowercase", "require_uppercase", "require_digit", "require_special"} {
		if !strings.Contains(body, `"`+field+`":false`) {
			t.Fatalf("body %q does not clear %s", body, field)
		}
	}
}

func TestPasswordPolicyIsARemoteModeCommand(t *testing.T) {
	code, _, stderr := runAdminInProcess(
		[]string{"password-policy", "--data-dir", t.TempDir()}, "", false)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--server") {
		t.Fatalf("stderr %q does not point at remote mode", stderr)
	}
}

func TestAdminUsageNamesThePasswordPolicyCommand(t *testing.T) {
	if !strings.Contains(adminUsage, "password-policy") {
		t.Fatal("the admin usage does not name the password-policy command")
	}
}

// Local mode opens the appliance directly, so it is held to the same stored
// policy as the HTTP routes without the command knowing the rule exists.
func TestLocalUserCommandsAreHeldToTheStoredPasswordPolicy(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")

	recordPasswordPolicy(t, dataDir, auth.PasswordPolicy{MinimumLength: 28, RequireUppercase: true, RequireDigit: true})

	refused := runAdmin(t, binary, password("Passw0rd-not-long-enough"),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "newcomer", "--role", "operator")
	if refused.exitCode == 0 {
		t.Fatal("user create accepted a password the stored policy refuses")
	}
	if !strings.Contains(refused.combined, "28") {
		t.Fatalf("the refusal does not name the configured minimum: %s", refused.combined)
	}
	for _, user := range listUsers(t, dataDir) {
		if user.Username == "newcomer" {
			t.Fatal("a refused create left an identity behind")
		}
	}

	// Long enough, and still refused: the class half of the policy reaches the
	// command line exactly as the length half does.
	tooFewClasses := runAdmin(t, binary, password("passwordpasswordpasswordpassword"),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "newcomer", "--role", "operator")
	if tooFewClasses.exitCode == 0 {
		t.Fatal("user create accepted a long password of one character class")
	}
	for _, named := range []string{"uppercase", "digit"} {
		if !strings.Contains(tooFewClasses.combined, named) {
			t.Fatalf("the refusal does not name the missing %s: %s", named, tooFewClasses.combined)
		}
	}

	mustRunAdmin(t, binary, password("Correct-Horse-Battery-Staple1"),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin",
		"--username", "newcomer", "--role", "operator")
	if user := findUser(t, dataDir, "newcomer"); user.Role != auth.RoleOperator {
		t.Fatalf("created identity = %#v", user)
	}
}

// The administrator bootstrapped before the policy was tightened still signs
// in, which is what the local commands prove by continuing to accept it as the
// acting actor.
func TestATightenedPolicyLeavesTheBootstrappedAdministratorUsable(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	recordPasswordPolicy(t, dataDir, auth.PasswordPolicy{MinimumLength: 64, RequireLowercase: true, RequireUppercase: true, RequireDigit: true, RequireSpecial: true})

	listed := mustRunAdmin(t, binary, "", "admin", "user", "list", "--data-dir", dataDir)
	if !strings.Contains(listed.stdout, "admin") {
		t.Fatalf("the bootstrapped administrator is gone from %s", listed.stdout)
	}
}

func recordPasswordPolicy(t *testing.T, dataDir string, policy auth.PasswordPolicy) {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open the appliance database: %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := sqlite.NewIdentityRepository(database).SavePasswordPolicy(
		context.Background(), policy, time.Now().UTC(),
	); err != nil {
		t.Fatalf("record the password policy: %v", err)
	}
}
