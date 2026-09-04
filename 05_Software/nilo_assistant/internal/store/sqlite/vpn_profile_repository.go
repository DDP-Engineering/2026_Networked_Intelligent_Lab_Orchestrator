package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"nilo.local/nilo/internal/network"
)

// VPNProfileRepository stores saved VPN profiles as public metadata.
//
// It refuses a record whose public configuration names key material rather than
// redacting it. Redaction is a promise that has to be kept in every writer;
// refusing at the one place that writes the table is a promise kept once.
type VPNProfileRepository struct {
	database *DB
}

func NewVPNProfileRepository(database *DB) *VPNProfileRepository {
	return &VPNProfileRepository{database: database}
}

// secretFieldNames matches a JSON field that would hold key material. It is
// anchored to the whole field name on purpose: `has_preshared_key` is the
// boolean that exists precisely so the key itself does not, and a guard that
// refused it would be refusing the safe design.
// The optional backslashes catch a field name that reached the blob inside a
// string value, where JSON encoding escapes its quotes.
var secretFieldNames = regexp.MustCompile(`(?i)\\?"(private_?key|preshared_?key|psk|password|passphrase)\\?"\s*:`)

// secretAssignments catches raw WireGuard configuration text that reached the
// public blob as a string value, where there are no JSON field names to anchor
// to.
var secretAssignments = regexp.MustCompile(`(?i)\b(privatekey|presharedkey)\s*=`)

func (repository *VPNProfileRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("VPN profile repository database is required")
	}
	return nil
}

// Save stores one profile, replacing the previous version of the same profile.
func (repository *VPNProfileRepository) Save(ctx context.Context, record network.ProfileRecord) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}
	configuration, err := json.Marshal(record.Profile)
	if err != nil {
		return fmt.Errorf("encode public profile configuration: %w", err)
	}
	if secretFieldNames.Match(configuration) || secretAssignments.Match(configuration) {
		return errors.New("the public profile configuration names key material and was not stored")
	}
	startSelected := 0
	if record.StartSelected {
		startSelected = 1
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO vpn_profiles (
			id, name, provider, public_config_json, secret_ref, connection_ref,
			revision, start_selected, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			public_config_json = excluded.public_config_json,
			connection_ref = excluded.connection_ref,
			revision = excluded.revision,
			updated_at = excluded.updated_at
	`,
		record.ID, record.Name, record.Provider, string(configuration),
		record.SecretRef, record.ConnectionRef, record.Revision, startSelected,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", network.ErrProfileNameTaken, record.Name)
		}
		return fmt.Errorf("store VPN profile: %w", err)
	}
	return nil
}

// List returns every saved profile, by name, so the inventory reads the same
// way twice.
func (repository *VPNProfileRepository) List(ctx context.Context) (records []network.ProfileRecord, err error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, name, provider, public_config_json, secret_ref, connection_ref,
		       revision, start_selected, created_at, updated_at
		FROM vpn_profiles ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("read VPN profiles: %w", err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close VPN profile rows: %w", closeErr)
		}
	}()

	records = make([]network.ProfileRecord, 0)
	for rows.Next() {
		var record network.ProfileRecord
		var configuration, created, updated string
		var startSelected int
		if err := rows.Scan(
			&record.ID, &record.Name, &record.Provider, &configuration,
			&record.SecretRef, &record.ConnectionRef, &record.Revision,
			&startSelected, &created, &updated,
		); err != nil {
			return nil, fmt.Errorf("scan VPN profile: %w", err)
		}
		if err := json.Unmarshal([]byte(configuration), &record.Profile); err != nil {
			return nil, fmt.Errorf("decode stored profile configuration: %w", err)
		}
		record.StartSelected = startSelected == 1
		if record.CreatedAt, err = parseUTCTimestamp(created); err != nil {
			return nil, fmt.Errorf("parse stored profile creation time: %w", err)
		}
		if record.UpdatedAt, err = parseUTCTimestamp(updated); err != nil {
			return nil, fmt.Errorf("parse stored profile update time: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate VPN profiles: %w", err)
	}
	return records, nil
}

// Rename changes the operator-visible label. The identity, the key-material
// reference and the NetworkManager connection are untouched, so everything that
// points at this profile keeps pointing at it.
func (repository *VPNProfileRepository) Rename(
	ctx context.Context, profileID, name string, at time.Time,
) (network.ProfileRecord, error) {
	if err := repository.ready(); err != nil {
		return network.ProfileRecord{}, err
	}
	if name == "" {
		return network.ProfileRecord{}, errors.New("a profile name cannot be empty")
	}
	result, err := repository.database.SQL().ExecContext(ctx, `
		UPDATE vpn_profiles SET name = ?, revision = ?, updated_at = ? WHERE id = ?
	`, name, revisionFor(profileID, name, at), at.UTC().Format(time.RFC3339Nano), profileID)
	if err != nil {
		if isUniqueConstraint(err) {
			return network.ProfileRecord{}, fmt.Errorf("%w: %s", network.ErrProfileNameTaken, name)
		}
		return network.ProfileRecord{}, fmt.Errorf("rename VPN profile: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return network.ProfileRecord{}, fmt.Errorf("count renamed VPN profiles: %w", err)
	}
	if changed == 0 {
		return network.ProfileRecord{}, fmt.Errorf("%w: %s", network.ErrProfileNotFound, profileID)
	}
	return repository.read(ctx, profileID)
}

// SelectStartup makes one profile the startup selection. Clearing the previous
// one is part of the same statement pair, because the schema permits only one
// and a caller must not be able to leave two behind.
func (repository *VPNProfileRepository) SelectStartup(ctx context.Context, profileID string, at time.Time) error {
	if err := repository.ready(); err != nil {
		return err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin startup selection: %w", err)
	}
	defer transaction.Rollback()

	stamp := at.UTC().Format(time.RFC3339Nano)
	if _, err := transaction.ExecContext(ctx,
		"UPDATE vpn_profiles SET start_selected = 0, updated_at = ? WHERE start_selected = 1", stamp); err != nil {
		return fmt.Errorf("clear the previous startup selection: %w", err)
	}
	result, err := transaction.ExecContext(ctx,
		"UPDATE vpn_profiles SET start_selected = 1, updated_at = ? WHERE id = ?", stamp, profileID)
	if err != nil {
		return fmt.Errorf("select the startup profile: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count startup selection: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("%w: %s", network.ErrProfileNotFound, profileID)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit startup selection: %w", err)
	}
	return nil
}

// ClearStartup leaves no profile selected, which is the other permitted state.
func (repository *VPNProfileRepository) ClearStartup(ctx context.Context, at time.Time) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if _, err := repository.database.SQL().ExecContext(ctx,
		"UPDATE vpn_profiles SET start_selected = 0, updated_at = ? WHERE start_selected = 1",
		at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("clear the startup selection: %w", err)
	}
	return nil
}

// Delete removes one profile and reports when there was nothing to remove,
// because a caller that deleted nothing needs to know it deleted nothing.
func (repository *VPNProfileRepository) Delete(ctx context.Context, profileID string) error {
	if err := repository.ready(); err != nil {
		return err
	}
	result, err := repository.database.SQL().ExecContext(ctx, "DELETE FROM vpn_profiles WHERE id = ?", profileID)
	if err != nil {
		return fmt.Errorf("delete VPN profile: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted VPN profiles: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf("%w: %s", network.ErrProfileNotFound, profileID)
	}
	return nil
}

func (repository *VPNProfileRepository) read(ctx context.Context, profileID string) (network.ProfileRecord, error) {
	records, err := repository.List(ctx)
	if err != nil {
		return network.ProfileRecord{}, err
	}
	for _, record := range records {
		if record.ID == profileID {
			return record, nil
		}
	}
	return network.ProfileRecord{}, fmt.Errorf("%w: %s", network.ErrProfileNotFound, profileID)
}

// revisionFor changes whenever an accepted edit changes what is stored, so a
// caller holding an older revision can be told its view is stale.
func revisionFor(parts ...any) string {
	digest := sha256.Sum256([]byte(fmt.Sprint(parts...)))
	return hex.EncodeToString(digest[:8])
}

var _ network.ProfileStore = (*VPNProfileRepository)(nil)
