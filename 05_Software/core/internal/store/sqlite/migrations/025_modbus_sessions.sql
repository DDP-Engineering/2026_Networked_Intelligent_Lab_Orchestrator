-- Live Modbus transports never resume after a Core restart; write reservations do.
CREATE TABLE modbus_sessions (
 id TEXT PRIMARY KEY, endpoint_id TEXT NOT NULL, endpoint_revision INTEGER NOT NULL,
 profile_id TEXT NOT NULL, profile_revision INTEGER NOT NULL, owner_id TEXT NOT NULL,
 credential_kind TEXT NOT NULL, credential_id TEXT NOT NULL, started_at TEXT NOT NULL,
 expires_at TEXT NOT NULL, state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX modbus_sessions_endpoint ON modbus_sessions(endpoint_id,started_at,id);
CREATE TABLE modbus_commands (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES modbus_sessions(id),
 request_key TEXT NOT NULL, digest TEXT NOT NULL, request_json TEXT NOT NULL,
 result_json TEXT NOT NULL, created_at TEXT NOT NULL,
 UNIQUE(session_id,request_key)
);
CREATE INDEX modbus_commands_session ON modbus_commands(session_id,created_at,id);
