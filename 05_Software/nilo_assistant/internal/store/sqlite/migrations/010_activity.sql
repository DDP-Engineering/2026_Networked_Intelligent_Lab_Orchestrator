-- Activity policy, retention watermarks and timed diagnostic sessions.
--
-- The policy is one administrator-owned row: retention targets and quotas per
-- category. A watermark records what retention removed, so a query that asks
-- for a time before it can be told about the gap rather than shown nothing.
-- A diagnostic session is bounded by its own expiry and by the boot it
-- started in; a session from an earlier boot is never active.

CREATE TABLE IF NOT EXISTS activity_policy (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    policy_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS retention_watermarks (
    category      TEXT PRIMARY KEY CHECK (category IN ('audit', 'event', 'debug')),
    purged_before TEXT NOT NULL,
    reason        TEXT NOT NULL CHECK (reason IN ('age', 'quota')),
    updated_at    TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS diagnostic_sessions (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL CHECK (scope <> ''),
    target     TEXT NOT NULL,
    actor_id   TEXT NOT NULL,
    minutes    INTEGER NOT NULL CHECK (minutes IN (5, 15, 60)),
    started_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    boot_id    TEXT NOT NULL,
    state      TEXT NOT NULL CHECK (state IN ('active', 'stopped', 'expired', 'replaced', 'interrupted')),
    ended_at   TEXT,
    end_reason TEXT NOT NULL
) STRICT;

CREATE INDEX IF NOT EXISTS diagnostic_sessions_state ON diagnostic_sessions (state, started_at);
