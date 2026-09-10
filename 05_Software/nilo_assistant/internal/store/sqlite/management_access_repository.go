package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/network"
)

// ManagementAccessRepository stores the single active listener allowlist and
// its last known-good recovery target in NILO's existing SQLite database.
type ManagementAccessRepository struct {
	database *DB
}

func NewManagementAccessRepository(database *DB) *ManagementAccessRepository {
	return &ManagementAccessRepository{database: database}
}

func (repository *ManagementAccessRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("management access repository database is required")
	}
	return nil
}

// LoadManagementAccess reads the durable listener decision, if configured.
func (repository *ManagementAccessRepository) LoadManagementAccess(ctx context.Context) (network.ManagementAccessRecord, bool, error) {
	if err := repository.ready(); err != nil {
		return network.ManagementAccessRecord{}, false, err
	}
	var interfacesJSON, tlsNamesJSON, goodInterfacesJSON, goodTLSNamesJSON string
	var state, confirmBy, detail, updatedAt string
	var port, goodPort int
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT interfaces_json, port, tls_names_json,
		       last_good_interfaces_json, last_good_port, last_good_tls_names_json,
		       state, confirm_by, detail, updated_at
		FROM management_access WHERE singleton = 1
	`).Scan(&interfacesJSON, &port, &tlsNamesJSON, &goodInterfacesJSON, &goodPort, &goodTLSNamesJSON, &state, &confirmBy, &detail, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return network.ManagementAccessRecord{}, false, nil
	}
	if err != nil {
		return network.ManagementAccessRecord{}, false, fmt.Errorf("read management access: %w", err)
	}
	record := network.ManagementAccessRecord{
		Configuration: network.ManagementAccessConfiguration{Port: port},
		LastKnownGood: network.ManagementAccessConfiguration{Port: goodPort},
		State:         network.ManagementAccessState(state), Detail: detail,
	}
	values := []struct {
		raw    string
		target *[]string
	}{
		{interfacesJSON, &record.Configuration.Interfaces},
		{tlsNamesJSON, &record.Configuration.TLSNames},
		{goodInterfacesJSON, &record.LastKnownGood.Interfaces},
		{goodTLSNamesJSON, &record.LastKnownGood.TLSNames},
	}
	for _, value := range values {
		if err := json.Unmarshal([]byte(value.raw), value.target); err != nil {
			return network.ManagementAccessRecord{}, false, fmt.Errorf("decode management access: %w", err)
		}
		if *value.target == nil {
			*value.target = []string{}
		}
	}
	if confirmBy != "" {
		if record.ConfirmBy, err = time.Parse(time.RFC3339Nano, confirmBy); err != nil {
			return network.ManagementAccessRecord{}, false, fmt.Errorf("parse management confirmation deadline: %w", err)
		}
	}
	if record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return network.ManagementAccessRecord{}, false, fmt.Errorf("parse management update time: %w", err)
	}
	return record, true, nil
}

// SaveManagementAccess atomically replaces the singleton decision.
func (repository *ManagementAccessRepository) SaveManagementAccess(ctx context.Context, record network.ManagementAccessRecord) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if err := record.Configuration.Validate(); err != nil {
		return err
	}
	if err := record.LastKnownGood.Validate(); err != nil {
		return fmt.Errorf("invalid last known-good management access: %w", err)
	}
	if record.State != network.ManagementAccessApplied && record.State != network.ManagementAccessPending &&
		record.State != network.ManagementAccessFailed && record.State != network.ManagementAccessRecovered {
		return errors.New("invalid management access state")
	}
	if record.UpdatedAt.IsZero() {
		return errors.New("management access update time is required")
	}
	interfaces, err := json.Marshal(record.Configuration.Interfaces)
	if err != nil {
		return err
	}
	tlsNames, err := json.Marshal(record.Configuration.TLSNames)
	if err != nil {
		return err
	}
	goodInterfaces, err := json.Marshal(record.LastKnownGood.Interfaces)
	if err != nil {
		return err
	}
	goodTLSNames, err := json.Marshal(record.LastKnownGood.TLSNames)
	if err != nil {
		return err
	}
	confirmBy := ""
	if !record.ConfirmBy.IsZero() {
		confirmBy = record.ConfirmBy.UTC().Format(time.RFC3339Nano)
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO management_access (
			singleton, interfaces_json, port, tls_names_json,
			last_good_interfaces_json, last_good_port, last_good_tls_names_json,
			state, confirm_by, detail, updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (singleton) DO UPDATE SET
			interfaces_json = excluded.interfaces_json,
			port = excluded.port,
			tls_names_json = excluded.tls_names_json,
			last_good_interfaces_json = excluded.last_good_interfaces_json,
			last_good_port = excluded.last_good_port,
			last_good_tls_names_json = excluded.last_good_tls_names_json,
			state = excluded.state,
			confirm_by = excluded.confirm_by,
			detail = excluded.detail,
			updated_at = excluded.updated_at
	`, string(interfaces), record.Configuration.Port, string(tlsNames), string(goodInterfaces),
		record.LastKnownGood.Port, string(goodTLSNames), string(record.State), confirmBy,
		record.Detail, record.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store management access: %w", err)
	}
	return nil
}

var _ network.ManagementAccessStore = (*ManagementAccessRepository)(nil)
