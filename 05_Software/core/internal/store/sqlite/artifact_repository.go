package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	artifactstore "nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/domain"
)

type ArtifactRepository struct {
	database *DB
}

func NewArtifactRepository(database *DB) *ArtifactRepository {
	return &ArtifactRepository{database: database}
}

func (repository *ArtifactRepository) Save(ctx context.Context, value domain.Artifact) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin artifact metadata save: %w", err)
	}
	defer transaction.Rollback()
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO artifacts (
			id, request_id, session_id, device_id, integration_id, media_type,
			digest, size_bytes, terminal_state, presentation_hints_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, value.ID.String(), value.RequestID.String(), value.SessionID.String(), value.DeviceID.String(),
		value.IntegrationID.String(), value.MediaType, value.Digest[:], value.SizeBytes,
		string(value.TerminalState), append([]byte(nil), value.PresentationHints...), value.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", artifactstore.ErrArtifactAlreadyRegistered, value.ID.String())
		}
		return fmt.Errorf("insert artifact metadata: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit artifact metadata: %w", err)
	}
	return nil
}

func (repository *ArtifactRepository) Get(ctx context.Context, id domain.ArtifactID) (domain.Artifact, error) {
	if err := repository.validateDatabase(); err != nil {
		return domain.Artifact{}, err
	}
	var value domain.Artifact
	var idValue, requestValue, sessionValue, deviceValue, integrationValue string
	var digest, hints []byte
	var terminalValue, createdValue string
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT id, request_id, session_id, device_id, integration_id, media_type,
		       digest, size_bytes, terminal_state, presentation_hints_json, created_at
		FROM artifacts WHERE id = ?
	`, id.String()).Scan(
		&idValue, &requestValue, &sessionValue, &deviceValue, &integrationValue, &value.MediaType,
		&digest, &value.SizeBytes, &terminalValue, &hints, &createdValue,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Artifact{}, artifactstore.ErrArtifactNotFound
	}
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("read artifact metadata: %w", err)
	}
	if len(digest) != len(value.Digest) {
		return domain.Artifact{}, fmt.Errorf("stored artifact digest length is %d", len(digest))
	}
	copy(value.Digest[:], digest)
	value.PresentationHints = append([]byte(nil), hints...)
	if value.ID, err = domain.ParseArtifactID(idValue); err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact ID: %w", err)
	}
	if value.RequestID, err = domain.ParseRequestID(requestValue); err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact request ID: %w", err)
	}
	if value.SessionID, err = domain.ParseSessionID(sessionValue); err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact session ID: %w", err)
	}
	if value.DeviceID, err = domain.ParseDeviceID(deviceValue); err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact device ID: %w", err)
	}
	if value.IntegrationID, err = domain.ParseIntegrationID(integrationValue); err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact integration ID: %w", err)
	}
	value.TerminalState = domain.SessionState(terminalValue)
	value.CreatedAt, err = parseUTCTimestamp(createdValue)
	if err != nil {
		return domain.Artifact{}, fmt.Errorf("parse stored artifact timestamp: %w", err)
	}
	return value, nil
}

func (repository *ArtifactRepository) validateDatabase() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("artifact database is required")
	}
	return nil
}

var _ artifactstore.Repository = (*ArtifactRepository)(nil)
