-- Saved adapter configuration, keyed by the adapter's permanent hardware
-- identity rather than its kernel name, so a rename keeps it and a replaced
-- adapter does not silently inherit it.
--
-- No password is here and none may ever be added. A wireless password lives in
-- the privileged helper's storage and in NetworkManager's root-only keyfile;
-- this table holds an opaque reference to it.

CREATE TABLE IF NOT EXISTS network_interface_settings (
    adapter_id           TEXT PRIMARY KEY CHECK (adapter_id <> ''),
    connection_ref       TEXT NOT NULL,
    method               TEXT NOT NULL CHECK (method IN ('automatic', 'static')),
    -- Empty for an automatic configuration.
    address              TEXT NOT NULL,
    gateway              TEXT NOT NULL,
    dns_json             TEXT NOT NULL,
    dns_automatic        INTEGER NOT NULL CHECK (dns_automatic IN (0, 1)),
    management_protected INTEGER NOT NULL CHECK (management_protected IN (0, 1)),
    never_default        INTEGER NOT NULL CHECK (never_default IN (0, 1)),
    -- Wireless intent; empty SSID means a wired configuration.
    wifi_ssid            TEXT NOT NULL,
    wifi_security        TEXT NOT NULL,
    wifi_startup         INTEGER NOT NULL CHECK (wifi_startup IN (0, 1)),
    wifi_secret_ref      TEXT NOT NULL,
    revision             TEXT NOT NULL,
    updated_at           TEXT NOT NULL
) STRICT;
