-- Saved VPN profiles, as public metadata.
--
-- No key material is here and none may ever be added. A private key, a
-- preshared key and a Wi-Fi password live in the privileged helper's own
-- storage; this table holds an opaque reference to them. That separation is
-- what makes a database backup, an export and a support bundle safe to hand
-- over, so it is a schema decision rather than a convention.

CREATE TABLE IF NOT EXISTS vpn_profiles (
    id       TEXT PRIMARY KEY,
    -- The operator-visible name. Unique because two profiles an operator cannot
    -- tell apart are worse than a refused import.
    name     TEXT NOT NULL UNIQUE CHECK (name <> ''),
    -- The provider boundary this milestone keeps small on purpose: WireGuard
    -- only, with room for a second value rather than a plugin framework.
    provider TEXT NOT NULL CHECK (provider = 'wireguard'),

    -- The parsed, safe configuration: addresses, DNS, MTU, listen port and the
    -- peers with their public keys, allowed IPs and endpoints.
    public_config_json TEXT NOT NULL,
    -- An opaque identity the privileged keystore understands. It is not a
    -- secret and cannot be used to derive one.
    secret_ref     TEXT NOT NULL,
    -- The NetworkManager connection NILO created for this profile, empty until
    -- one exists.
    connection_ref TEXT NOT NULL,

    revision   TEXT NOT NULL,
    -- Zero or one profile is connected at startup. The partial unique index
    -- below makes that structural: clearing the previous selection is not
    -- something a writer can forget to do.
    start_selected INTEGER NOT NULL CHECK (start_selected IN (0, 1)),

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS vpn_profiles_one_startup_selection
    ON vpn_profiles (start_selected)
    WHERE start_selected = 1;
