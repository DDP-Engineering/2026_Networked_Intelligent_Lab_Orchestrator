package identity_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// A refused deletion is as accountable as an applied one: who tried, what, and
// why it was refused. The record is written after the refusal, outside the
// transaction that rolled back, so it never depends on the change.
func TestRefusedDeletionsAreRecordedWithTheirReason(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	administrator := f.bootstrap(t)
	ctx := context.Background()
	operator, err := f.service.CreateUser(ctx, administrator, "operator", auth.RoleOperator, []byte(operatorPassword))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	issued, err := f.service.CreateToken(ctx, administrator, "operator", "owned", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	for _, refusal := range []struct {
		name    string
		action  string
		label   string
		reason  string
		actor   auth.User
		subject map[string]string
		attempt func() error
	}{
		{
			name: "last administrator", action: "identity.user.delete_refused", label: "admin",
			reason: "last_administrator", actor: administrator, subject: map[string]string{"username": "admin"},
			attempt: func() error { return f.service.DeleteUser(ctx, administrator, "admin") },
		},
		{
			name: "owned tokens", action: "identity.user.delete_refused", label: "operator",
			reason: "user_has_tokens", actor: administrator, subject: map[string]string{"username": "operator"},
			attempt: func() error { return f.service.DeleteUser(ctx, administrator, "operator") },
		},
		{
			name: "unknown username", action: "identity.user.delete_refused", label: "nobody",
			reason: "user_not_found", actor: administrator, subject: map[string]string{"username": "nobody"},
			attempt: func() error { return f.service.DeleteUser(ctx, administrator, "nobody") },
		},
		{
			name: "non-administrator actor", action: "identity.user.delete_refused", label: "admin",
			reason: "not_administrator", actor: operator, subject: map[string]string{"username": "admin"},
			attempt: func() error { return f.service.DeleteUserAs(ctx, identity.UserActor(operator), "admin") },
		},
		{
			name: "unknown token", action: "identity.token.delete_refused", label: "missing-token",
			reason: "token_not_found", actor: administrator, subject: map[string]string{"token_id": "missing-token"},
			attempt: func() error { return f.service.DeleteToken(ctx, administrator, "missing-token") },
		},
		{
			name: "non-administrator token deletion", action: "identity.token.delete_refused", label: issued.Token.ID,
			reason: "not_administrator", actor: operator, subject: map[string]string{"token_id": issued.Token.ID},
			attempt: func() error {
				return f.service.DeleteTokenAs(ctx, identity.UserActor(operator), issued.Token.ID)
			},
		},
	} {
		if err := refusal.attempt(); err == nil {
			t.Fatalf("%s: the deletion was applied", refusal.name)
		}
		event := lastEvent(t, f, refusal.action)
		if event.Category != audit.CategoryAudit || event.Component != "identity" ||
			event.Outcome != audit.OutcomeDenied || event.Severity != audit.SeverityWarning ||
			event.ActorKind != audit.ActorKindUser || event.ActorID.String() != refusal.actor.ID {
			t.Fatalf("%s: refusal record has wrong common fields: %#v", refusal.name, event)
		}
		if event.TargetLabel != refusal.label {
			t.Fatalf("%s: target label = %q, want %q", refusal.name, event.TargetLabel, refusal.label)
		}
		want := map[string]string{"reason": refusal.reason, "actor_username": refusal.actor.Username}
		for key, value := range refusal.subject {
			want[key] = value
		}
		assertDetails(t, event, want)
	}

	// Six attempts, six records, and not one deletion.
	for action, count := range map[string]int{
		"identity.user.delete_refused": 4, "identity.token.delete_refused": 2,
		"identity.user.deleted": 0, "identity.token.deleted": 0,
	} {
		events, err := f.audit.List(ctx, audit.Filter{Action: action})
		if err != nil {
			t.Fatalf("list %q: %v", action, err)
		}
		if len(events) != count {
			t.Fatalf("%q recorded %d events, want %d", action, len(events), count)
		}
		for _, event := range events {
			for _, canary := range []string{issued.Secret, administratorPassword, operatorPassword, "verifier", "$argon2id$"} {
				if strings.Contains(string(event.Payload), canary) {
					t.Fatalf("%q leaked %q", action, canary)
				}
			}
		}
	}
	users, err := f.service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("identities after the refusals = %d, want 2", len(users))
	}
	tokens, err := f.service.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens after the refusals = %d, want 1", len(tokens))
	}
}

// Two administrators, one removing the other while the other disables the one:
// the write lock serializes them, exactly one is applied, one enabled
// administrator remains, and a deletion that lost says why.
func TestConcurrentDeleteAndDisableLeaveOneEnabledAdministrator(t *testing.T) {
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
			results[0] = first.service.DeleteUser(context.Background(), administrator, "second-admin")
		}()
		go func() {
			defer running.Done()
			<-start
			results[1] = other.service.SetEnabled(context.Background(), second, "admin", false)
		}()
		close(start)
		running.Wait()

		applied := 0
		for index, err := range results {
			switch {
			case err == nil:
				applied++
			case errors.Is(err, identity.ErrLastAdministrator), errors.Is(err, identity.ErrNotAdministrator):
			default:
				t.Fatalf("concurrent change %d failed for an unrelated reason: %v", index, err)
			}
		}
		if applied != 1 {
			t.Fatalf("%d of the two concurrent changes were applied, want 1: %v", applied, results)
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
			t.Fatalf("enabled administrators after the concurrent changes = %d, want 1", administrators)
		}
		refusals, err := first.audit.List(context.Background(), audit.Filter{Action: "identity.user.delete_refused"})
		if err != nil {
			t.Fatalf("list refusals: %v", err)
		}
		if results[0] != nil && len(refusals) != 1 {
			t.Fatalf("the losing deletion recorded %d refusals, want 1", len(refusals))
		}
		if results[0] == nil && len(refusals) != 0 {
			t.Fatalf("the applied deletion recorded %d refusals, want 0", len(refusals))
		}
	}
}

func lastEvent(t *testing.T, f fixture, action string) audit.Event {
	t.Helper()
	events, err := f.audit.List(context.Background(), audit.Filter{Action: action})
	if err != nil {
		t.Fatalf("list %q: %v", action, err)
	}
	if len(events) == 0 {
		t.Fatalf("%q recorded no event", action)
	}
	return events[len(events)-1]
}
