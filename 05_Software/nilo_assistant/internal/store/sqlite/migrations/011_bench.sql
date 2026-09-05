CREATE TABLE bench_plugins (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL,
    installed INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 0,
    mode TEXT NOT NULL DEFAULT 'ask',
    digest TEXT NOT NULL DEFAULT '',
    devices_json BLOB NOT NULL DEFAULT '[]',
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE bench_jobs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id),
    plugin_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    credential_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    submission_json BLOB NOT NULL,
    deadline TEXT NOT NULL,
    created_at TEXT NOT NULL,
    result_json BLOB NOT NULL DEFAULT '{}',
    events_json BLOB NOT NULL DEFAULT '[]',
    error TEXT NOT NULL DEFAULT '',
    artifact_id TEXT NOT NULL DEFAULT '',
    UNIQUE(owner_id, idempotency_key)
);
CREATE INDEX bench_jobs_created ON bench_jobs(created_at);
