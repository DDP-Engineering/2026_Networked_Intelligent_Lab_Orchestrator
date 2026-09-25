CREATE TABLE bench_device_sequences (
    plugin_id TEXT PRIMARY KEY,
    next_sequence INTEGER NOT NULL CHECK (next_sequence >= 1)
);

CREATE TABLE bench_weak_serials (
    plugin_id TEXT NOT NULL,
    vendor TEXT NOT NULL,
    product TEXT NOT NULL,
    serial TEXT NOT NULL,
    classified_at TEXT NOT NULL,
    PRIMARY KEY (plugin_id, vendor, product, serial)
);

CREATE TABLE bench_devices (
    plugin_id TEXT NOT NULL,
    record_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 1),
    kind TEXT NOT NULL,
    vendor TEXT NOT NULL DEFAULT '',
    product TEXT NOT NULL DEFAULT '',
    serial TEXT NOT NULL DEFAULT '',
    port_path TEXT NOT NULL DEFAULT '',
    identity_rule TEXT NOT NULL CHECK (identity_rule IN ('serial', 'port')),
    serial_weak INTEGER NOT NULL DEFAULT 0 CHECK (serial_weak IN (0, 1)),
    endpoint_id TEXT NOT NULL DEFAULT '',
    endpoints_json BLOB NOT NULL DEFAULT '[]',
    capabilities_json BLOB NOT NULL DEFAULT '{}',
    display_name TEXT NOT NULL,
    name_key TEXT,
    purpose TEXT NOT NULL DEFAULT '',
    settings_json BLOB NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    configured INTEGER NOT NULL DEFAULT 0 CHECK (configured IN (0, 1)),
    presence TEXT NOT NULL DEFAULT 'unknown' CHECK (presence IN ('unknown', 'present', 'absent')),
    readiness TEXT NOT NULL DEFAULT 'unconfigured' CHECK (readiness IN ('unconfigured', 'initializing', 'ready', 'failed')),
    last_reset_status TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    last_seen_at TEXT,
    created_at TEXT NOT NULL,
    tombstone_target TEXT,
    PRIMARY KEY (plugin_id, record_id),
    UNIQUE (plugin_id, sequence)
);

CREATE UNIQUE INDEX bench_devices_name_key
    ON bench_devices(plugin_id, name_key)
    WHERE configured = 1 AND tombstone_target IS NULL;

CREATE INDEX bench_devices_identity
    ON bench_devices(plugin_id, vendor, product, serial, port_path);
