package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
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

// ResetNetworkCommissioning atomically returns only network access to first
// boot. Appliance identity, hostname, TLS, users, credentials, audit history,
// integrations, interface settings, and device networks are not touched.
// CommissionUnlessPasswordMatches records the appliance as commissioned, but
// only if no identity under username still stores a verifier matching password.
//
// The two halves are one transaction, begun with `BEGIN IMMEDIATE` so that it
// holds the database's write lock before it reads anything - the same way an
// administrative identity change does. Checking the credential and recording the
// lifecycle state as separate statements would leave a window in which a
// password reset, which commits on its own transaction and may come from
// another process holding the same data directory, can restore the published
// verifier after the check has passed and before the appliance is recorded as
// commissioned. Taking the write lock up front means such a reset lands either
// entirely before this transaction, and is seen, or entirely after it, by which
// time the appliance is already commissioned and the published password is an
// ordinary password like any other.
func (repository *CommissioningRepository) CommissionUnlessPasswordMatches(
	ctx context.Context, username string, password []byte,
) error {
	if err := repository.ready(); err != nil {
		return err
	}
	connection, err := repository.database.SQL().Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve the commissioning connection: %w", err)
	}
	defer func() { _ = connection.Close() }()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin the commissioning transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// The rollback runs even for a cancelled context, because this
		// connection returns to the pool either way and must never carry an
		// open transaction back to it.
		_, _ = connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
	}()

	var verifier string
	switch scanErr := connection.QueryRowContext(ctx,
		"SELECT password_hash FROM users WHERE username = ?", username,
	).Scan(&verifier); {
	case scanErr == nil:
		matches, verifyErr := auth.VerifyPassword(verifier, password)
		if verifyErr == nil && matches {
			return appliance.ErrPublishedCredentialPresent
		}
	case errors.Is(scanErr, sql.ErrNoRows):
		// No identity under that name, so no published verifier to survive.
	default:
		return fmt.Errorf("read the published administrator verifier: %w", scanErr)
	}

	result, err := connection.ExecContext(ctx,
		"UPDATE commissioning SET state = ? WHERE singleton = 1",
		string(appliance.CommissioningCommissioned))
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
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit the commissioning transaction: %w", err)
	}
	committed = true
	return nil
}

func (repository *CommissioningRepository) ResetNetworkCommissioning(ctx context.Context, access network.ManagementAccessRecord) (err error) {
	return repository.resetNetworkCommissioning(ctx, access, nil, 0)
}

// ResetNetworkCommissioningAudited joins the network-only reset and its
// already composed audit event in one SQLite transaction.
func (repository *CommissioningRepository) ResetNetworkCommissioningAudited(
	ctx context.Context, access network.ManagementAccessRecord, event audit.Event, hardwareGeneration uint64,
) error {
	return repository.resetNetworkCommissioning(ctx, access, &event, hardwareGeneration)
}

func (repository *CommissioningRepository) resetNetworkCommissioning(
	ctx context.Context, access network.ManagementAccessRecord, event *audit.Event, hardwareGeneration uint64,
) (err error) {
	if err := repository.ready(); err != nil {
		return err
	}
	if err := access.Configuration.ValidateFirewall(); err != nil {
		return fmt.Errorf("invalid derived management access: %w", err)
	}
	if err := access.LastKnownGood.ValidateFirewall(); err != nil {
		return fmt.Errorf("invalid derived last known-good access: %w", err)
	}
	if access.State != network.ManagementAccessApplied || access.UpdatedAt.IsZero() {
		return errors.New("derived management access must be applied and timestamped")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin network commissioning reset: %w", err)
	}
	defer func() {
		if err != nil {
			_ = transaction.Rollback()
		}
	}()
	result, err := transaction.ExecContext(ctx,
		"UPDATE commissioning SET state = ? WHERE singleton = 1 AND state = ?",
		string(appliance.CommissioningProvisioning), string(appliance.CommissioningCommissioned))
	if err != nil {
		return fmt.Errorf("reset commissioning lifecycle: %w", err)
	}
	if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
		if rowsErr != nil {
			return fmt.Errorf("confirm commissioning reset: %w", rowsErr)
		}
		return errors.New("network commissioning reset requires a commissioned appliance")
	}
	interfaces, err := json.Marshal(bindingsForStorage(access.Configuration))
	if err != nil {
		return err
	}
	tlsNames, err := json.Marshal(access.Configuration.TLSNames)
	if err != nil {
		return err
	}
	goodInterfaces, err := json.Marshal(bindingsForStorage(access.LastKnownGood))
	if err != nil {
		return err
	}
	goodTLSNames, err := json.Marshal(access.LastKnownGood.TLSNames)
	if err != nil {
		return err
	}
	detail, err := encodeManagementAccessDetail(access)
	if err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, `
		INSERT INTO management_access (
			singleton, interfaces_json, port, tls_names_json,
			last_good_interfaces_json, last_good_port, last_good_tls_names_json,
			state, confirm_by, detail, updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			interfaces_json=excluded.interfaces_json, port=excluded.port, tls_names_json=excluded.tls_names_json,
			last_good_interfaces_json=excluded.last_good_interfaces_json,
			last_good_port=excluded.last_good_port, last_good_tls_names_json=excluded.last_good_tls_names_json,
			state=excluded.state, confirm_by='', detail=excluded.detail, updated_at=excluded.updated_at
	`, string(interfaces), access.Configuration.Port, string(tlsNames), string(goodInterfaces),
		access.LastKnownGood.Port, string(goodTLSNames), string(access.State), detail,
		access.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("replace explicit management selection with derived access: %w", err)
	}
	if _, err = transaction.ExecContext(ctx, `
		INSERT INTO network_recovery (
			singleton, version, state, armed_at, expires_at, connected_at_arm_json,
			adapter_id, interface_name, helper_record_id, hardware_generation, pending_audits_json, detail, updated_at
		) VALUES (1, 1, ?, '', '', '[]', '', '', '', ?, '[]', '', ?)
		ON CONFLICT(singleton) DO UPDATE SET
			version=1, state=excluded.state, armed_at='', expires_at='', connected_at_arm_json='[]',
			adapter_id='', interface_name='', helper_record_id='',
			hardware_generation=MAX(network_recovery.hardware_generation, excluded.hardware_generation),
			pending_audits_json='[]', detail='', updated_at=excluded.updated_at
	`, string(network.RecoveryIdle), hardwareGeneration, access.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("clear temporary network recovery: %w", err)
	}
	if event != nil {
		if err = NewAuditRepository(repository.database).AppendInTransaction(ctx, transaction, *event); err != nil {
			return fmt.Errorf("record network commissioning reset: %w", err)
		}
	}
	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("commit network commissioning reset: %w", err)
	}
	return nil
}

var _ appliance.CommissioningStore = (*CommissioningRepository)(nil)
var _ appliance.CommissioningWriter = (*CommissioningRepository)(nil)
