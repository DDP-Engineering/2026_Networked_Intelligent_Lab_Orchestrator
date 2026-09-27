-- Serial connections are ephemeral; credential bindings and TX reservations survive crashes.
ALTER TABLE bench_jobs ADD COLUMN browser_session_id TEXT NOT NULL DEFAULT '';
CREATE TABLE serial_connections (
 id TEXT PRIMARY KEY, device_id TEXT NOT NULL, owner_id TEXT NOT NULL,
 credential_kind TEXT NOT NULL, credential_id TEXT NOT NULL,
 revision INTEGER NOT NULL, settings_json TEXT NOT NULL,
 started_at TEXT NOT NULL, expires_at TEXT NOT NULL, state TEXT NOT NULL
);
CREATE TABLE serial_sends (
 connection_id TEXT NOT NULL REFERENCES serial_connections(id), request_key TEXT NOT NULL,
 digest TEXT NOT NULL, result_json TEXT NOT NULL,
 PRIMARY KEY(connection_id,request_key)
);
CREATE TABLE serial_retention (
 device_id TEXT PRIMARY KEY, revision INTEGER NOT NULL DEFAULT 1,
 limits_json TEXT NOT NULL DEFAULT '{"files":100,"total_bytes":2147483648,"file_bytes":104857600}'
);
CREATE TABLE serial_logs (
 id TEXT PRIMARY KEY, device_id TEXT NOT NULL, connection_id TEXT NOT NULL,
 owner_id TEXT NOT NULL, started_at TEXT NOT NULL, ended_at TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('active','completed','incomplete')),
 rx_bytes INTEGER NOT NULL DEFAULT 0, tx_bytes INTEGER NOT NULL DEFAULT 0,
 size_bytes INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX serial_logs_device ON serial_logs(device_id,started_at,id);
