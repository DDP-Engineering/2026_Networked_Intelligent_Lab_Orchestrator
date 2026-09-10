CREATE TABLE management_access (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    interfaces_json TEXT NOT NULL,
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    tls_names_json TEXT NOT NULL,
    last_good_interfaces_json TEXT NOT NULL,
    last_good_port INTEGER NOT NULL CHECK (last_good_port BETWEEN 1 AND 65535),
    last_good_tls_names_json TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('applied', 'pending', 'failed', 'recovered')),
    confirm_by TEXT NOT NULL,
    detail TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
