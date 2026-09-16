package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/auth"
)

// LoadPasswordPolicy reads the appliance's one password policy. An appliance
// that has recorded none applies the shipped default rather than erroring: the
// absent row is a valid state, and it is the state every database carried
// forward from a release before this table starts in.
func (repository *IdentityRepository) LoadPasswordPolicy(ctx context.Context) (auth.PasswordPolicy, error) {
	if err := repository.validate(); err != nil {
		return auth.PasswordPolicy{}, err
	}
	var policy auth.PasswordPolicy
	err := repository.execute().QueryRowContext(ctx,
		"SELECT minimum_length, minimum_character_classes FROM password_policy WHERE singleton = 1",
	).Scan(&policy.MinimumLength, &policy.MinimumCharacterClasses)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.DefaultPasswordPolicy(), nil
	}
	if err != nil {
		return auth.PasswordPolicy{}, fmt.Errorf("read the appliance password policy: %w", err)
	}
	return policy, nil
}

// SavePasswordPolicy records the policy, replacing whatever was there. It runs
// on the repository's own statements, so a save handed a repository bound to an
// administrative transaction joins that transaction and commits with the audit
// event that describes it.
func (repository *IdentityRepository) SavePasswordPolicy(
	ctx context.Context, policy auth.PasswordPolicy, at time.Time,
) error {
	if err := repository.validate(); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	if _, err := repository.execute().ExecContext(ctx, `
		INSERT INTO password_policy (singleton, minimum_length, minimum_character_classes, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			minimum_length = excluded.minimum_length,
			minimum_character_classes = excluded.minimum_character_classes,
			updated_at = excluded.updated_at
	`, policy.MinimumLength, policy.MinimumCharacterClasses, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record the appliance password policy: %w", err)
	}
	return nil
}
