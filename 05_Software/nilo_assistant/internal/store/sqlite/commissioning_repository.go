package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"nilo.local/nilo/internal/appliance"
)

// CommissioningRepository reads the single durable record of where this
// appliance stands in its lifecycle. The record is seeded by the schema
// migration that creates it; nothing in this milestone writes it afterwards.
type CommissioningRepository struct {
	database *DB
}

func NewCommissioningRepository(database *DB) *CommissioningRepository {
	return &CommissioningRepository{database: database}
}

func (repository *CommissioningRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("commissioning repository database is required")
	}
	return nil
}

// LoadCommissioning reads the recorded lifecycle state. A missing row is an
// error rather than a default, because the safe value differs in each direction
// and neither may be assumed.
func (repository *CommissioningRepository) LoadCommissioning(ctx context.Context) (appliance.CommissioningState, error) {
	if err := repository.ready(); err != nil {
		return "", err
	}
	var state string
	err := repository.database.SQL().QueryRowContext(ctx,
		"SELECT state FROM commissioning WHERE singleton = 1").Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("no appliance commissioning state is recorded")
	}
	if err != nil {
		return "", fmt.Errorf("read appliance commissioning state: %w", err)
	}
	return appliance.CommissioningState(state), nil
}

// SaveCommissioning records the lifecycle state, refusing one that is not a
// recognized value so an unusable state can never be written.
func (repository *CommissioningRepository) SaveCommissioning(ctx context.Context, state appliance.CommissioningState) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	result, err := repository.database.SQL().ExecContext(ctx,
		"UPDATE commissioning SET state = ? WHERE singleton = 1", string(state))
	if err != nil {
		return fmt.Errorf("record appliance commissioning state: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm appliance commissioning state: %w", err)
	}
	if changed != 1 {
		return errors.New("no appliance commissioning state is recorded")
	}
	return nil
}

var _ appliance.CommissioningStore = (*CommissioningRepository)(nil)
var _ appliance.CommissioningWriter = (*CommissioningRepository)(nil)
