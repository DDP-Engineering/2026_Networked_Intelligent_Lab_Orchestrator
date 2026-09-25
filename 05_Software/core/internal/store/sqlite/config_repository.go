package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const maxConfigurationBytes = 1 << 20

var (
	ErrInvalidConfigurationSnapshot = errors.New("invalid configuration snapshot")
	ErrNoActiveConfiguration        = errors.New("no active configuration")
)

// ConfigSnapshot is one immutable canonical configuration version.
type ConfigSnapshot struct {
	ID                uuid.UUID
	ConfigurationJSON []byte
	Digest            [sha256.Size]byte
	CreatedAt         time.Time
}

// ConfigRepository atomically versions and selects validated configuration.
type ConfigRepository struct {
	database *DB
}

func NewConfigRepository(database *DB) *ConfigRepository {
	return &ConfigRepository{database: database}
}

// ActivateValidated persists canonical bytes and makes that version the sole
// active configuration. Semantic validation belongs to the configuration
// boundary; this method still rejects malformed, non-object, and oversized JSON.
func (repository *ConfigRepository) ActivateValidated(ctx context.Context, configuration []byte) (snapshot ConfigSnapshot, err error) {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return ConfigSnapshot{}, errors.New("configuration repository database is required")
	}
	canonical, err := canonicalConfiguration(configuration)
	if err != nil {
		return ConfigSnapshot{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ConfigSnapshot{}, fmt.Errorf("create configuration version identity: %w", err)
	}
	snapshot = ConfigSnapshot{
		ID:                id,
		ConfigurationJSON: canonical,
		Digest:            sha256.Sum256(canonical),
		CreatedAt:         time.Now().UTC(),
	}

	connection, err := repository.database.SQL().Conn(ctx)
	if err != nil {
		return ConfigSnapshot{}, fmt.Errorf("reserve configuration connection: %w", err)
	}
	defer func() {
		closeErr := connection.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("release configuration connection: %w", closeErr)
		}
	}()
	if _, err = connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return ConfigSnapshot{}, fmt.Errorf("begin configuration activation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err = connection.ExecContext(ctx, "UPDATE config_versions SET active = 0 WHERE active = 1"); err != nil {
		return ConfigSnapshot{}, fmt.Errorf("clear active configuration: %w", err)
	}
	if _, err = connection.ExecContext(ctx, `
		INSERT INTO config_versions (id, configuration_json, digest, created_at, active)
		VALUES (?, ?, ?, ?, 1)
	`, snapshot.ID.String(), string(snapshot.ConfigurationJSON), snapshot.Digest[:], snapshot.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return ConfigSnapshot{}, fmt.Errorf("store active configuration: %w", err)
	}
	if _, err = connection.ExecContext(ctx, "COMMIT"); err != nil {
		return ConfigSnapshot{}, fmt.Errorf("commit configuration activation: %w", err)
	}
	committed = true
	snapshot.ConfigurationJSON = bytes.Clone(snapshot.ConfigurationJSON)
	return snapshot, nil
}

// Active reloads the selected immutable configuration version.
func (repository *ConfigRepository) Active(ctx context.Context) (ConfigSnapshot, error) {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return ConfigSnapshot{}, errors.New("configuration repository database is required")
	}
	var idText, configurationText, createdAtText string
	var digestBytes []byte
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT id, configuration_json, digest, created_at
		FROM config_versions
		WHERE active = 1
	`).Scan(&idText, &configurationText, &digestBytes, &createdAtText)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigSnapshot{}, ErrNoActiveConfiguration
	}
	if err != nil {
		return ConfigSnapshot{}, fmt.Errorf("load active configuration: %w", err)
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return ConfigSnapshot{}, fmt.Errorf("parse active configuration identity: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtText)
	if err != nil {
		return ConfigSnapshot{}, fmt.Errorf("parse active configuration timestamp: %w", err)
	}
	if createdAt.Location() != time.UTC {
		return ConfigSnapshot{}, errors.New("active configuration timestamp is not UTC")
	}
	if len(digestBytes) != sha256.Size {
		return ConfigSnapshot{}, fmt.Errorf("active configuration digest length is %d", len(digestBytes))
	}
	var digest [sha256.Size]byte
	copy(digest[:], digestBytes)
	configuration := []byte(configurationText)
	if sha256.Sum256(configuration) != digest {
		return ConfigSnapshot{}, errors.New("active configuration digest mismatch")
	}
	return ConfigSnapshot{
		ID:                id,
		ConfigurationJSON: bytes.Clone(configuration),
		Digest:            digest,
		CreatedAt:         createdAt,
	}, nil
}

func canonicalConfiguration(configuration []byte) ([]byte, error) {
	if len(configuration) == 0 || len(configuration) > maxConfigurationBytes {
		return nil, fmt.Errorf("%w: byte length %d", ErrInvalidConfigurationSnapshot, len(configuration))
	}
	decoder := json.NewDecoder(bytes.NewReader(configuration))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfigurationSnapshot, err)
	}
	if value == nil {
		return nil, fmt.Errorf("%w: top-level value must be an object", ErrInvalidConfigurationSnapshot)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: canonicalize: %v", ErrInvalidConfigurationSnapshot, err)
	}
	return canonical, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%w: multiple JSON values", ErrInvalidConfigurationSnapshot)
	}
	return fmt.Errorf("%w: trailing data: %v", ErrInvalidConfigurationSnapshot, err)
}
