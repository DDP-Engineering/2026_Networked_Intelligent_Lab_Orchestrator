package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/store/sqlite"
)

// The stub proves what the command asks for. This test proves what a real
// appliance answers: the permission model, the scope narrowing and the
// credential state are the server's, and the command adds nothing to them.

const remoteLivePassword = "remote-created-0001"

type issuedLocalToken struct {
	id     string
	secret string
}

func issueLocalToken(t *testing.T, binary, dataDir, owner, name, scopes, expiresIn string) issuedLocalToken {
	t.Helper()
	arguments := []string{
		"admin", "token", "create", "--data-dir", dataDir, "--actor", "admin", "--owner", owner, "--name", name,
	}
	if scopes != "" {
		arguments = append(arguments, "--scopes", scopes)
	}
	if expiresIn != "" {
		arguments = append(arguments, "--expires-in", expiresIn)
	}
	created := mustRunAdmin(t, binary, "", arguments...)
	var issued struct {
		TokenID string `json:"token_id"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &issued); err != nil {
		t.Fatalf("decode token creation %q: %v", created.stdout, err)
	}
	return issuedLocalToken{id: issued.TokenID, secret: issued.Token}
}

// expireToken moves a token's expiry into the past directly in the store, the
// way time would.
func expireToken(t *testing.T, dataDir, id string) {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := database.SQL().ExecContext(context.Background(),
		"UPDATE mcp_tokens SET expires_at = ? WHERE id = ?", past, id); err != nil {
		t.Fatalf("expire token: %v", err)
	}
}

func listLiveAuditEvents(t *testing.T, dataDir, action string) []audit.Event {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()
	events, err := sqlite.NewAuditRepository(database).List(context.Background(), audit.Filter{Action: action})
	if err != nil {
		t.Fatalf("list %q: %v", action, err)
	}
	return events
}

// startNiloServer serves the data directory on a loopback address and returns
// the address once the status route answers the given token.
func startNiloServer(t *testing.T, binary, dataDir, readyToken string) (string, func()) {
	t.Helper()
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	address := reservation.Addr().String()
	if err := reservation.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}
	server := exec.Command(binary, "serve", "--profile", "simulation", "--data-dir", dataDir, "--bind", address)
	var output bytes.Buffer
	server.Stdout = &output
	server.Stderr = &output
	if err := server.Start(); err != nil {
		t.Fatalf("start nilo serve: %v", err)
	}
	returned := make(chan error, 1)
	go func() { returned <- server.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = server.Process.Kill()
		<-returned
	}
	t.Cleanup(stop)

	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	for {
		request, err := http.NewRequest(http.MethodGet, "http://"+address+"/api/v1/status", nil)
		if err != nil {
			t.Fatalf("create readiness request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+readyToken)
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return address, stop
			}
		}
		select {
		case err := <-returned:
			t.Fatalf("nilo serve exited before listening: %v; output=%s", err, output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server did not become ready: %v; output=%s", requestErr, output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRemoteAdministrationAgainstARealApplianceHonoursRolesScopesAndCredentialState(t *testing.T) {
	binary := buildNilo(t)
	dataDir := t.TempDir()
	mustRunAdmin(t, binary, password(bootstrapPassword),
		"admin", "bootstrap", "--data-dir", dataDir, "--username", "admin")
	mustRunAdmin(t, binary, password(operatorPassword),
		"admin", "user", "create", "--data-dir", dataDir, "--actor", "admin", "--username", "viewer", "--role", "viewer")

	full := issueLocalToken(t, binary, dataDir, "admin", "full", "", "")
	viewer := issueLocalToken(t, binary, dataDir, "viewer", "viewer", "", "")
	narrowed := issueLocalToken(t, binary, dataDir, "admin", "narrowed", "status.read", "")
	revoked := issueLocalToken(t, binary, dataDir, "admin", "revoked", "", "")
	mustRunAdmin(t, binary, "", "admin", "token", "revoke", "--data-dir", dataDir, "--actor", "admin", "--id", revoked.id)
	expired := issueLocalToken(t, binary, dataDir, "admin", "expired", "", "1h")
	expireToken(t, dataDir, expired.id)

	address, stop := startNiloServer(t, binary, dataDir, full.secret)
	server := "http://" + address
	files := map[string]string{}
	for name, token := range map[string]issuedLocalToken{
		"full": full, "viewer": viewer, "narrowed": narrowed, "revoked": revoked, "expired": expired,
	} {
		files[name] = tokenFile(t, token.secret)
	}
	remote := func(name string, extra ...string) []string {
		return append(extra, "--server", server, "--token-file", files[name])
	}
	secrets := []string{full.secret, viewer.secret, narrowed.secret, revoked.secret, expired.secret,
		bootstrapPassword, operatorPassword, remoteLivePassword}
	var outputs []string

	listed := mustRunAdmin(t, binary, "", remote("full", "admin", "user", "list", "--json")...)
	outputs = append(outputs, listed.combined)
	var listing struct {
		Items []struct {
			Username string `json:"username"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listed.stdout), &listing); err != nil || len(listing.Items) != 2 {
		t.Fatalf("remote listing = %q (%v), want two identities", listed.stdout, err)
	}

	// The role, the scope narrowing and the credential state are decided by
	// the appliance on every request. The command reports the refusal it gets.
	for name, want := range map[string]string{
		"viewer": "forbidden", "narrowed": "forbidden", "revoked": "unauthorized", "expired": "unauthorized",
	} {
		refused := runAdmin(t, binary, "", remote(name, "admin", "user", "list")...)
		outputs = append(outputs, refused.combined)
		if refused.exitCode != 1 {
			t.Fatalf("%s token: exit = %d, want 1: %s", name, refused.exitCode, refused.combined)
		}
		if !strings.Contains(refused.combined, want) {
			t.Fatalf("%s token: refusal does not carry %q: %s", name, want, refused.combined)
		}
		if refused.stdout != "" {
			t.Fatalf("%s token: a refusal printed on standard output: %s", name, refused.stdout)
		}
	}
	if narrowedCanRead := runAdmin(t, binary, "", remote("narrowed", "network", "status")...); narrowedCanRead.exitCode != 1 {
		// status.read narrows the administrator to the status route only.
		t.Fatalf("the narrowed token reached the network route: %s", narrowedCanRead.combined)
	}

	created := mustRunAdmin(t, binary, password(remoteLivePassword),
		remote("full", "admin", "user", "create", "--username", "newcomer", "--role", "operator")...)
	outputs = append(outputs, created.combined)
	if !strings.Contains(created.stdout, "newcomer") {
		t.Fatalf("remote creation output = %s", created.combined)
	}
	if user := findUser(t, dataDir, "newcomer"); !user.Enabled() {
		t.Fatal("the remotely created identity is not enabled")
	}

	refused := runAdmin(t, binary, "", remote("full", "admin", "user", "delete", "--username", "admin", "--confirm-delete")...)
	outputs = append(outputs, refused.combined)
	if refused.exitCode != 1 || !strings.Contains(refused.combined, "last_administrator") {
		t.Fatalf("deleting the last administrator: exit = %d, output = %s", refused.exitCode, refused.combined)
	}
	unconfirmed := runAdmin(t, binary, "", remote("full", "admin", "user", "delete", "--username", "newcomer")...)
	if unconfirmed.exitCode != 2 {
		t.Fatalf("an unconfirmed noninteractive deletion: exit = %d, want 2: %s", unconfirmed.exitCode, unconfirmed.combined)
	}
	deleted := mustRunAdmin(t, binary, "", remote("full", "admin", "user", "delete", "--username", "newcomer", "--confirm-delete")...)
	outputs = append(outputs, deleted.combined)
	for _, user := range listUsers(t, dataDir) {
		if user.Username == "newcomer" {
			t.Fatal("the remotely deleted identity still exists")
		}
	}

	operations := mustRunAdmin(t, binary, "", remote("full", "network", "operation", "list", "--json")...)
	if !strings.Contains(operations.stdout, `"items"`) {
		t.Fatalf("operation list = %s", operations.combined)
	}

	stop()

	// Every attempt is on record, applied or refused, attributed to the token
	// that made it and readable without the identity row.
	refusals := listLiveAuditEvents(t, dataDir, "identity.user.delete_refused")
	if len(refusals) != 1 || refusals[0].Outcome != audit.OutcomeDenied || refusals[0].ActorKind != audit.ActorKindToken ||
		refusals[0].TargetLabel != "admin" || !strings.Contains(string(refusals[0].Payload), `"reason":"last_administrator"`) {
		t.Fatalf("refused deletion records = %#v", refusals)
	}
	applied := listLiveAuditEvents(t, dataDir, "identity.user.deleted")
	if len(applied) != 1 || applied[0].ActorKind != audit.ActorKindToken || applied[0].TargetLabel != "newcomer" {
		t.Fatalf("applied deletion records = %#v", applied)
	}
	denied := listLiveAuditEvents(t, dataDir, "identity.access.denied")
	if len(denied) < 2 {
		t.Fatalf("the forbidden listings were not recorded: %d access denials", len(denied))
	}

	for _, output := range outputs {
		for _, secret := range secrets {
			if strings.Contains(output, secret) {
				t.Fatalf("a command printed a secret: %s", output)
			}
		}
	}
	for _, events := range [][]audit.Event{refusals, applied, denied} {
		for _, event := range events {
			for _, secret := range secrets {
				if strings.Contains(string(event.Payload), secret) {
					t.Fatalf("audit event %q carries a secret", event.Action)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "nilo.db")); err != nil {
		t.Fatalf("the data directory is gone: %v", err)
	}
}
