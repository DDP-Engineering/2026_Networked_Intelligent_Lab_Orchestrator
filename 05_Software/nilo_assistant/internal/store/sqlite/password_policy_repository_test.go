package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
)

// An appliance that has never recorded a policy applies the shipped default, so
// upgrading a database written by an earlier release changes no behavior.
func TestPasswordPolicyDefaultsWhereNoneIsRecorded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	policy, err := NewIdentityRepository(database).LoadPasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("LoadPasswordPolicy: %v", err)
	}
	if policy != auth.DefaultPasswordPolicy() {
		t.Fatalf("policy = %#v, want the shipped default %#v", policy, auth.DefaultPasswordPolicy())
	}
}

func TestPasswordPolicySurvivesReopeningTheDataDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	recorded := auth.PasswordPolicy{MinimumLength: 24, MinimumCharacterClasses: 3}
	at := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	if err := NewIdentityRepository(database).SavePasswordPolicy(ctx, recorded, at); err != nil {
		t.Fatalf("SavePasswordPolicy: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	policy, err := NewIdentityRepository(reopened).LoadPasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("LoadPasswordPolicy after reopen: %v", err)
	}
	if policy != recorded {
		t.Fatalf("policy after reopen = %#v, want %#v", policy, recorded)
	}
}

// The record is one row. A second save replaces it rather than adding another,
// so there is never a question of which policy the appliance holds.
func TestPasswordPolicyReplacesTheSingleRecordedRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewIdentityRepository(database)
	at := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)

	for _, policy := range []auth.PasswordPolicy{
		{MinimumLength: 16, MinimumCharacterClasses: 2},
		{MinimumLength: 20, MinimumCharacterClasses: 0},
	} {
		if err := repository.SavePasswordPolicy(ctx, policy, at); err != nil {
			t.Fatalf("SavePasswordPolicy(%#v): %v", policy, err)
		}
	}

	var rows int
	if err := database.SQL().QueryRowContext(ctx, "SELECT count(*) FROM password_policy").Scan(&rows); err != nil {
		t.Fatalf("count password_policy rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("password_policy holds %d rows, want exactly 1", rows)
	}
	policy, err := repository.LoadPasswordPolicy(ctx)
	if err != nil {
		t.Fatalf("LoadPasswordPolicy: %v", err)
	}
	if (policy != auth.PasswordPolicy{MinimumLength: 20}) {
		t.Fatalf("policy = %#v, want the second save", policy)
	}
}

// The Go validation is the rule a client is answered with; the table's own
// constraints are the second line under it, so a policy that somehow reached
// the database outside that validation is refused by SQLite rather than stored.
func TestPasswordPolicyTableRefusesValuesOutsideItsBounds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	for _, values := range [][2]int{{7, 0}, {1025, 0}, {12, 5}, {12, -1}} {
		_, err := database.SQL().ExecContext(ctx, `
			INSERT INTO password_policy (singleton, minimum_length, minimum_character_classes, updated_at)
			VALUES (1, ?, ?, '2026-09-15T10:00:00Z')
			ON CONFLICT(singleton) DO UPDATE SET
				minimum_length = excluded.minimum_length,
				minimum_character_classes = excluded.minimum_character_classes
		`, values[0], values[1])
		if err == nil {
			t.Fatalf("the table accepted minimum_length=%d classes=%d", values[0], values[1])
		}
	}
}
