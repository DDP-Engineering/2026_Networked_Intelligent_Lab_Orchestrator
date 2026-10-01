-- Logical Modbus recordings and seekable evidence parts.
CREATE TABLE modbus_recordings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES modbus_sessions(id),
    endpoint_id TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL DEFAULT '',
    tags_json TEXT NOT NULL DEFAULT '[]',
    mode TEXT NOT NULL CHECK (mode IN ('values', 'traffic', 'both')),
    sampling TEXT NOT NULL CHECK (sampling IN ('every', 'interval', 'change')),
    interval_ms INTEGER NOT NULL DEFAULT 0,
    snapshot_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'incomplete')),
    reason TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    ended_at TEXT NOT NULL DEFAULT '',
    last_sequence INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    part_count INTEGER NOT NULL DEFAULT 0,
    recovery_done INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX modbus_recordings_owner_started ON modbus_recordings(owner_id, started_at DESC, id DESC);
CREATE INDEX modbus_recordings_started ON modbus_recordings(started_at DESC, id DESC);

CREATE TABLE modbus_recording_tags (
    recording_id TEXT NOT NULL REFERENCES modbus_recordings(id) ON DELETE CASCADE,
    tag_key TEXT NOT NULL,
    PRIMARY KEY (recording_id, tag_key)
);
CREATE INDEX modbus_recording_tags_key ON modbus_recording_tags(tag_key, recording_id);

CREATE TABLE modbus_recording_parts (
    recording_id TEXT NOT NULL REFERENCES modbus_recordings(id) ON DELETE CASCADE,
    part_no INTEGER NOT NULL CHECK (part_no > 0),
    size_bytes INTEGER NOT NULL DEFAULT 0,
    first_sequence INTEGER NOT NULL DEFAULT 0,
    last_sequence INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    closed INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (recording_id, part_no)
);

CREATE TABLE modbus_recording_events (
    recording_id TEXT NOT NULL REFERENCES modbus_recordings(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    part_no INTEGER NOT NULL,
    offset_bytes INTEGER NOT NULL,
    length_bytes INTEGER NOT NULL CHECK (length_bytes > 0),
    kind TEXT NOT NULL,
    utc_at TEXT NOT NULL,
    elapsed_ns INTEGER NOT NULL,
    unit INTEGER NOT NULL DEFAULT 0,
    function INTEGER NOT NULL DEFAULT 0,
    address INTEGER NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL DEFAULT '',
    point_id TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (recording_id, part_no) REFERENCES modbus_recording_parts(recording_id, part_no) ON DELETE CASCADE,
    PRIMARY KEY (recording_id, sequence)
);
CREATE UNIQUE INDEX modbus_recording_events_recording_sequence ON modbus_recording_events(recording_id, sequence);
CREATE INDEX modbus_recording_events_unit_function ON modbus_recording_events(recording_id, unit, function, sequence);
CREATE INDEX modbus_recording_events_utc ON modbus_recording_events(recording_id, utc_at, sequence);
CREATE INDEX modbus_recording_events_outcome ON modbus_recording_events(recording_id, outcome, sequence);
CREATE INDEX modbus_recording_events_point_address ON modbus_recording_events(recording_id, point_id, address, sequence);
