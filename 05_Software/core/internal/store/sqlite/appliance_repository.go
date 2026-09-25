package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/appliance"
)

// ApplianceRepository stores the one row that says which appliance this is.
//
// There is no create and no replace, only Ensure: an appliance that could be
// handed a new identity could disown the history recorded under the old one.
type ApplianceRepository struct {
	database *DB
}

func NewApplianceRepository(database *DB) *ApplianceRepository {
	return &ApplianceRepository{database: database}
}

func (repository *ApplianceRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("appliance repository database is required")
	}
	return nil
}

// Ensure writes candidate only when the appliance has no identity yet, and
// returns whatever is stored afterwards. Two concurrent starters therefore
// agree, and a restart is a read rather than a re-provisioning.
func (repository *ApplianceRepository) Ensure(
	ctx context.Context, candidate appliance.Identity,
) (appliance.Identity, error) {
	if err := repository.ready(); err != nil {
		return appliance.Identity{}, err
	}
	if err := candidate.Validate(); err != nil {
		return appliance.Identity{}, err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("begin appliance identity transaction: %w", err)
	}
	defer transaction.Rollback()

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO appliance_identity (id, appliance_id, name, location, commissioned_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`, candidate.ID, candidate.Name, candidate.Location,
		candidate.CommissionedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return appliance.Identity{}, fmt.Errorf("store appliance identity: %w", err)
	}
	stored, err := readIdentity(ctx, transaction)
	if err != nil {
		return appliance.Identity{}, err
	}
	if err := transaction.Commit(); err != nil {
		return appliance.Identity{}, fmt.Errorf("commit appliance identity: %w", err)
	}
	return stored, nil
}

// SetLabels changes the two mutable labels. The identity is untouched, because
// a rename is a label change and not a different appliance.
func (repository *ApplianceRepository) SetLabels(
	ctx context.Context, name, location string,
) (appliance.Identity, error) {
	if err := repository.ready(); err != nil {
		return appliance.Identity{}, err
	}
	if err := (appliance.Identity{
		ID: "00000000-0000-4000-8000-000000000001", Name: name, Location: location,
		CommissionedAt: time.Now().UTC(),
	}).Validate(); err != nil {
		return appliance.Identity{}, err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("begin appliance label transaction: %w", err)
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(ctx,
		"UPDATE appliance_identity SET name = ?, location = ? WHERE id = 1", name, location)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("update appliance labels: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("count appliance label update: %w", err)
	}
	if changed == 0 {
		return appliance.Identity{}, errors.New("this appliance has no identity to label yet")
	}
	stored, err := readIdentity(ctx, transaction)
	if err != nil {
		return appliance.Identity{}, err
	}
	if err := transaction.Commit(); err != nil {
		return appliance.Identity{}, fmt.Errorf("commit appliance labels: %w", err)
	}
	return stored, nil
}

// SetFriendlyName changes only the optional local DNS alias.
func (repository *ApplianceRepository) SetFriendlyName(ctx context.Context, friendlyName string) (appliance.Identity, error) {
	if err := repository.ready(); err != nil {
		return appliance.Identity{}, err
	}
	if err := appliance.ValidateFriendlyName(friendlyName); err != nil {
		return appliance.Identity{}, err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("begin friendly-name transaction: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, "UPDATE appliance_identity SET friendly_name = ? WHERE id = 1", friendlyName)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("update friendly appliance name: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("count friendly-name update: %w", err)
	}
	if changed == 0 {
		return appliance.Identity{}, errors.New("this appliance has no identity to name yet")
	}
	stored, err := readIdentity(ctx, transaction)
	if err != nil {
		return appliance.Identity{}, err
	}
	if err := transaction.Commit(); err != nil {
		return appliance.Identity{}, fmt.Errorf("commit friendly appliance name: %w", err)
	}
	return stored, nil
}

func readIdentity(ctx context.Context, execute statements) (appliance.Identity, error) {
	var identity appliance.Identity
	var commissioned string
	err := execute.QueryRowContext(ctx, `
		SELECT appliance_id, name, location, friendly_name, commissioned_at FROM appliance_identity WHERE id = 1
	`).Scan(&identity.ID, &identity.Name, &identity.Location, &identity.FriendlyName, &commissioned)
	if errors.Is(err, sql.ErrNoRows) {
		return appliance.Identity{}, errors.New("this appliance has no stored identity")
	}
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("read appliance identity: %w", err)
	}
	identity.CommissionedAt, err = parseUTCTimestamp(commissioned)
	if err != nil {
		return appliance.Identity{}, fmt.Errorf("parse stored commissioning time: %w", err)
	}
	return identity, nil
}

var _ appliance.Repository = (*ApplianceRepository)(nil)
