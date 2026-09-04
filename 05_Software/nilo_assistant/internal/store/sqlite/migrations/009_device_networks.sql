-- Saved device networks: which adapter faces the devices, what the appliance
-- is on it, what is exposed, from where, and whether the operator wants it
-- enabled. Applied state is never stored here; it is observed.

CREATE TABLE IF NOT EXISTS device_networks (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL CHECK (name <> ''),
    location           TEXT NOT NULL,
    -- The device-side adapter's permanent hardware identity.
    device_interface   TEXT NOT NULL CHECK (device_interface <> ''),
    appliance_address  TEXT NOT NULL,
    scope              TEXT NOT NULL CHECK (scope IN ('host', 'subnet')),
    target             TEXT NOT NULL,
    addressing         TEXT NOT NULL CHECK (addressing IN ('original', 'translated')),
    exposed            TEXT NOT NULL,
    access             TEXT NOT NULL CHECK (access IN ('local', 'wireguard', 'both')),
    local_interface    TEXT NOT NULL,
    local_sources_json TEXT NOT NULL,
    vpn_profile_id     TEXT NOT NULL,
    vpn_sources_json   TEXT NOT NULL,
    services_json      TEXT NOT NULL,
    enabled            INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    revision           TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
) STRICT;
