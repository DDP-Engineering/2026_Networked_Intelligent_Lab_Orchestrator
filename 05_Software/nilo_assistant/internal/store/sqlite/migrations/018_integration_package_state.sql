ALTER TABLE bench_plugins ADD COLUMN state_schema INTEGER NOT NULL DEFAULT 1 CHECK (state_schema > 0);
