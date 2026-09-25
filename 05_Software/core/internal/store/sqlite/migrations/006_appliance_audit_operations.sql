-- Appliance identity, the audit event envelope, its sequence allocator, and
-- durable network operations.
--
-- Every column added to audit_events is nullable with no default value, so a
-- row written by the previous writer reads back as "this writer recorded
-- nothing here" rather than as an asserted value. Rewriting the historical
-- meaning of a stored audit row is the one thing an append-only log may not do.

-- One row, ever. The CHECK on id is what makes that structural rather than a
-- convention a later writer could forget.
CREATE TABLE IF NOT EXISTS appliance_identity (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    appliance_id   TEXT    NOT NULL CHECK (appliance_id <> ''),
    name           TEXT    NOT NULL,
    location       TEXT    NOT NULL,
    commissioned_at TEXT   NOT NULL
) STRICT;

-- The audit ordering allocator.
--
-- MAX(sequence) + 1 is correct only while rows are never deleted. Retention
-- deletes them, and a cursor that restarts at one after an empty sweep would
-- hand out numbers that have already been used, which would silently reorder
-- history and make an export ambiguous. The allocator therefore lives in its
-- own row that retention never touches.
CREATE TABLE IF NOT EXISTS audit_sequence (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    next INTEGER NOT NULL CHECK (next > 0)
) STRICT;

INSERT INTO audit_sequence (id, next)
SELECT 1, COALESCE((SELECT MAX(sequence) FROM audit_events), 0) + 1
WHERE NOT EXISTS (SELECT 1 FROM audit_sequence WHERE id = 1);

ALTER TABLE audit_events ADD COLUMN envelope_version INTEGER;
ALTER TABLE audit_events ADD COLUMN appliance_id TEXT;
ALTER TABLE audit_events ADD COLUMN boot_id TEXT;
ALTER TABLE audit_events ADD COLUMN category TEXT;
ALTER TABLE audit_events ADD COLUMN component TEXT;
ALTER TABLE audit_events ADD COLUMN actor_kind TEXT;
ALTER TABLE audit_events ADD COLUMN outcome TEXT;
ALTER TABLE audit_events ADD COLUMN severity TEXT;
ALTER TABLE audit_events ADD COLUMN operation_id TEXT;
ALTER TABLE audit_events ADD COLUMN target_label TEXT;
ALTER TABLE audit_events ADD COLUMN time_source TEXT;

-- Activity reads are filtered by category and time and paged by sequence.
CREATE INDEX IF NOT EXISTS audit_events_by_category_sequence
    ON audit_events (category, sequence);
CREATE INDEX IF NOT EXISTS audit_events_by_component_sequence
    ON audit_events (component, sequence);
CREATE INDEX IF NOT EXISTS audit_events_by_operation
    ON audit_events (operation_id);

-- Network operations, so a change that is in flight survives a page reload, a
-- core restart and a reboot.
--
-- parameters_json holds only safe normalized parameters. Imported profile text,
-- keys and passwords never reach this table: they are held by the privileged
-- helper and referenced, and the repository refuses a record that names one.
CREATE TABLE IF NOT EXISTS network_operations (
    id               TEXT PRIMARY KEY,
    action           TEXT NOT NULL CHECK (action <> ''),
    target           TEXT NOT NULL,
    actor_id         TEXT NOT NULL CHECK (actor_id <> ''),
    state            TEXT NOT NULL CHECK (state <> ''),
    revision         TEXT NOT NULL,
    parameters_json  TEXT NOT NULL,
    checkpoint_id    TEXT NOT NULL,
    failure          TEXT NOT NULL,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    -- NULL for an action that never opens a confirmation window, so a caller
    -- never sees a countdown that does not apply.
    confirm_deadline TEXT
) STRICT;

CREATE INDEX IF NOT EXISTS network_operations_by_recency
    ON network_operations (created_at DESC);
CREATE INDEX IF NOT EXISTS network_operations_by_state
    ON network_operations (state);
