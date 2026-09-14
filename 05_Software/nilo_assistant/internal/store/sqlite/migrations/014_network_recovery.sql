CREATE TABLE IF NOT EXISTS network_recovery (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    version INTEGER NOT NULL,
    state TEXT NOT NULL,
    armed_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    connected_at_arm_json TEXT NOT NULL,
    adapter_id TEXT NOT NULL,
    interface_name TEXT NOT NULL,
    helper_record_id TEXT NOT NULL,
    hardware_generation INTEGER NOT NULL,
    pending_audits_json TEXT NOT NULL,
    detail TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
