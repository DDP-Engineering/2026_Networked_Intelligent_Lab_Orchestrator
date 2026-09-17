-- User-owned discovery presentation state and durable Identify audit outbox.

ALTER TABLE appliance_identity ADD COLUMN friendly_name TEXT NOT NULL DEFAULT '';

CREATE TABLE identify_operations (
    id                 TEXT PRIMARY KEY,
    source             TEXT NOT NULL CHECK (source IN ('interface', 'button')),
    state              TEXT NOT NULL CHECK (state IN ('pending', 'acted', 'settled')),
    supported          INTEGER NOT NULL CHECK (supported IN (0, 1)),
    reannouncement     TEXT NOT NULL CHECK (reannouncement IN ('not_requested', 'complete', 'partial', 'unavailable')),
    pending_audit_json TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
) STRICT;
