package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PluginConfiguration struct {
	PluginID string
	Revision int64
	Settings json.RawMessage
}

type PluginConfigurationRepository struct{ database *DB }

func NewPluginConfigurationRepository(database *DB) *PluginConfigurationRepository {
	return &PluginConfigurationRepository{database: database}
}

func (repository *PluginConfigurationRepository) Get(ctx context.Context, pluginID string) (PluginConfiguration, error) {
	if repository == nil || repository.database == nil || pluginID == "" {
		return PluginConfiguration{}, errors.New("invalid plugin configuration read")
	}
	configuration := PluginConfiguration{PluginID: pluginID}
	var settings string
	err := repository.database.SQL().QueryRowContext(ctx, `SELECT revision,settings_json FROM bench_plugin_configuration WHERE plugin_id=?`, pluginID).Scan(&configuration.Revision, &settings)
	if errors.Is(err, sql.ErrNoRows) {
		configuration.Settings = json.RawMessage(`{}`)
		return configuration, nil
	}
	if err != nil {
		return PluginConfiguration{}, err
	}
	configuration.Settings = json.RawMessage(settings)
	return configuration, nil
}

func (repository *PluginConfigurationRepository) Put(ctx context.Context, pluginID string, revision int64, settings json.RawMessage, now time.Time) (PluginConfiguration, error) {
	if repository == nil || repository.database == nil || pluginID == "" || revision < 0 || len(settings) > 16<<10 {
		return PluginConfiguration{}, errors.New("invalid plugin configuration update")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(settings, &fields) != nil || fields == nil {
		return PluginConfiguration{}, errors.New("plugin configuration must be an object")
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	var result sql.Result
	var err error
	if revision == 0 {
		result, err = repository.database.SQL().ExecContext(ctx, `INSERT INTO bench_plugin_configuration(plugin_id,revision,settings_json,updated_at) VALUES(?,1,?,?) ON CONFLICT(plugin_id) DO NOTHING`, pluginID, settings, stamp)
	} else {
		result, err = repository.database.SQL().ExecContext(ctx, `UPDATE bench_plugin_configuration SET revision=revision+1,settings_json=?,updated_at=? WHERE plugin_id=? AND revision=?`, settings, stamp, pluginID, revision)
	}
	if err != nil {
		return PluginConfiguration{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return PluginConfiguration{}, err
	}
	if changed == 0 {
		return PluginConfiguration{}, ErrManagedDeviceConflict
	}
	return repository.Get(ctx, pluginID)
}
