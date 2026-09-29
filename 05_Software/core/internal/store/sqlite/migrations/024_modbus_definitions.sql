CREATE TABLE modbus_endpoints (
    id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    managed_device_id TEXT,
    managed_revision INTEGER,
    document TEXT NOT NULL CHECK (json_valid(document))
);

CREATE UNIQUE INDEX modbus_usb_binding ON modbus_endpoints(managed_device_id)
    WHERE managed_device_id IS NOT NULL;

CREATE TABLE modbus_profiles (
    id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    endpoint_id TEXT NOT NULL REFERENCES modbus_endpoints(id),
    owner_id TEXT NOT NULL,
    document TEXT NOT NULL CHECK (json_valid(document))
);

CREATE INDEX modbus_profiles_endpoint ON modbus_profiles(endpoint_id);
