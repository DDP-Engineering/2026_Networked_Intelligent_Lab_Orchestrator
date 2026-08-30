CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    digest TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS config_versions (
    id TEXT PRIMARY KEY,
    configuration_json TEXT NOT NULL,
    digest BLOB NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mcp_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    verifier BLOB NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS app_policies (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL UNIQUE,
    mode TEXT NOT NULL CHECK (mode IN ('ask', 'auto', 'full')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operation_policy_overrides (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    policy TEXT NOT NULL CHECK (policy IN ('auto_eligible', 'full_only', 'always_prompt', 'denied')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (integration_id, operation_name)
);

CREATE TABLE IF NOT EXISTS device_policies (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL UNIQUE,
    mode TEXT NOT NULL CHECK (mode IN ('ask', 'auto', 'full')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operation_requests (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    integration_id TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    device_id TEXT NOT NULL,
    canonical_parameters BLOB NOT NULL,
    configuration_snapshot_id TEXT NOT NULL REFERENCES config_versions(id),
    policy_snapshot_id TEXT NOT NULL,
    deadline_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approval_records (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES operation_requests(id),
    request_digest BLOB NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('approved', 'denied')),
    actor_id TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    UNIQUE (request_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL UNIQUE REFERENCES operation_requests(id),
    state TEXT NOT NULL CHECK (state IN ('pending_approval', 'queued', 'running', 'completed', 'failed', 'cancelled', 'incomplete')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    terminal_at TEXT
);

CREATE TABLE IF NOT EXISTS resource_leases (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    resource_name TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    released_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS resource_leases_one_active_resource
    ON resource_leases(resource_name)
    WHERE released_at IS NULL;

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    sequence INTEGER NOT NULL UNIQUE,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    payload_json BLOB NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL REFERENCES operation_requests(id),
    session_id TEXT NOT NULL REFERENCES sessions(id),
    device_id TEXT NOT NULL,
    integration_id TEXT NOT NULL,
    media_type TEXT NOT NULL,
    digest BLOB NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    terminal_state TEXT NOT NULL CHECK (terminal_state IN ('completed', 'failed', 'cancelled', 'incomplete')),
    presentation_hints_json BLOB NOT NULL,
    created_at TEXT NOT NULL
);
