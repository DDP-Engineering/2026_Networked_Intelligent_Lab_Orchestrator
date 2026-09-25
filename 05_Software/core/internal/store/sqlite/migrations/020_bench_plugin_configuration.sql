CREATE TABLE bench_plugin_configuration (
    plugin_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    settings_json BLOB NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL
);
