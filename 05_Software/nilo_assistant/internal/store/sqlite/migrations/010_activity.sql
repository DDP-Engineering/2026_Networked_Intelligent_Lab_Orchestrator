-- Activity policy and retention watermarks.
--
-- The policy is one administrator-owned row: retention targets and quotas per
-- category. A watermark records what retention removed, so a query that asks
-- for a time before it can be told about the gap rather than shown nothing.

CREATE TABLE IF NOT EXISTS activity_policy (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    policy_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS retention_watermarks (
    category      TEXT PRIMARY KEY CHECK (category IN ('audit', 'event')),
    purged_before TEXT NOT NULL,
    reason        TEXT NOT NULL CHECK (reason IN ('age', 'quota')),
    updated_at    TEXT NOT NULL
) STRICT;
