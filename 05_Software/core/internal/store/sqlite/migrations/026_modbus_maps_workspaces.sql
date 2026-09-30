CREATE TABLE modbus_profile_revisions (
    id TEXT NOT NULL REFERENCES modbus_profiles(id),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    document TEXT NOT NULL CHECK (json_valid(document)),
    PRIMARY KEY(id, revision)
);

INSERT INTO modbus_profile_revisions(id,revision,document)
SELECT id,revision,document FROM modbus_profiles;

CREATE TABLE modbus_maps (
    id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    owner_id TEXT NOT NULL,
    document TEXT NOT NULL CHECK (json_valid(document))
);

CREATE TABLE modbus_map_revisions (
    id TEXT NOT NULL REFERENCES modbus_maps(id),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    document TEXT NOT NULL CHECK (json_valid(document)),
    PRIMARY KEY(id, revision)
);

CREATE TABLE modbus_workspaces (
    id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    owner_id TEXT NOT NULL,
    endpoint_id TEXT NOT NULL REFERENCES modbus_endpoints(id),
    document TEXT NOT NULL CHECK (json_valid(document))
);

CREATE TABLE modbus_workspace_revisions (
    id TEXT NOT NULL REFERENCES modbus_workspaces(id),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 2147483647),
    document TEXT NOT NULL CHECK (json_valid(document)),
    PRIMARY KEY(id, revision)
);

CREATE INDEX modbus_workspaces_endpoint ON modbus_workspaces(endpoint_id);
