-- The appliance commissioning lifecycle.
--
-- A clean installation begins in first-boot provisioning; an appliance that is
-- already deployed must never be pulled back into it by an upgrade, because
-- provisioning temporarily offers management access on every eligible adapter.
--
-- The one fact that separates the two is whether this database already holds an
-- appliance identity at the moment this migration runs. Migrations are applied
-- inside sqlite.Open, before anything provisions an identity, so a genuinely new
-- database has no identity row here and a database carried forward from a
-- previous release always does. That covers the interface-managed and the legacy
-- single-listener deployment alike, without detecting either separately.
--
-- The seed is expressed here rather than in Go because this is the only code
-- that runs before the first identity row is written.
CREATE TABLE commissioning (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    state     TEXT    NOT NULL CHECK (state IN ('provisioning', 'commissioned'))
) STRICT;

INSERT INTO commissioning (singleton, state)
SELECT 1, CASE
    WHEN EXISTS (SELECT 1 FROM appliance_identity) THEN 'commissioned'
    ELSE 'provisioning'
END;
