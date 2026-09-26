CREATE TABLE camera_workspace_settings (
    camera_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL DEFAULT 0,
    settings_json TEXT NOT NULL DEFAULT '{}',
    retention_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE camera_media (
    id TEXT PRIMARY KEY,
    camera_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    state TEXT NOT NULL,
    reserved_bytes INTEGER NOT NULL DEFAULT 0,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
CREATE INDEX camera_media_camera_created ON camera_media(camera_id, created_at, id);
CREATE TABLE camera_legacy_deletions (
    job_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    complete INTEGER NOT NULL DEFAULT 0
);
