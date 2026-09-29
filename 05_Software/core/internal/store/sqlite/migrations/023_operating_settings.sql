-- One appliance-wide record; all values are whole units and replaced together.
CREATE TABLE operating_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL CHECK (revision > 0),
    executing_jobs INTEGER NOT NULL,
    outstanding_jobs INTEGER NOT NULL,
    serial_connections INTEGER NOT NULL,
    camera_streams INTEGER NOT NULL,
    default_job_timeout_seconds INTEGER NOT NULL,
    max_job_timeout_seconds INTEGER NOT NULL,
    serial_lifetime_minutes INTEGER NOT NULL,
    serial_idle_minutes INTEGER NOT NULL,
    camera_lifetime_minutes INTEGER NOT NULL
);

INSERT INTO operating_settings VALUES (1, 1, 2, 32, 4, 2, 60, 300, 60, 1, 55);
