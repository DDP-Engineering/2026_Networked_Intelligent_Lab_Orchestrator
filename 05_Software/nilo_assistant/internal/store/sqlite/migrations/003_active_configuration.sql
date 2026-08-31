ALTER TABLE config_versions
    ADD COLUMN active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1));

CREATE UNIQUE INDEX IF NOT EXISTS one_active_config_version
    ON config_versions(active)
    WHERE active = 1;
