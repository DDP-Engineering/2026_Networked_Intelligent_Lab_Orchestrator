-- Appliance-wide Modbus admission and future recording limits.
CREATE TABLE modbus_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL CHECK (revision > 0),
    max_sessions INTEGER NOT NULL,
    session_lease_seconds INTEGER NOT NULL,
    idle_seconds INTEGER NOT NULL,
    retention_count INTEGER NOT NULL,
    retention_bytes INTEGER NOT NULL,
    part_bytes INTEGER NOT NULL
);

INSERT INTO modbus_settings VALUES (1, 1, 4, 3600, 300, 100, 2147483648, 104857600);
