-- The appliance password policy: one minimum length and one minimum count of
-- distinct character classes, applied wherever a password is set.
--
-- No row is seeded. An absent row means the shipped default - a minimum of 12
-- bytes with no character-class requirement - which is the rule every appliance
-- enforced before this table existed. A database carried forward from an
-- earlier release therefore keeps behaving exactly as it did until an
-- administrator records a policy of their own.
--
-- The CHECK constraints mirror the bounds the Go policy validates. They are the
-- second line under that validation, not the primary one: a client is always
-- answered with the Go message naming the bound it missed.
CREATE TABLE password_policy (
    singleton                 INTEGER PRIMARY KEY CHECK (singleton = 1),
    minimum_length            INTEGER NOT NULL CHECK (minimum_length BETWEEN 8 AND 1024),
    minimum_character_classes INTEGER NOT NULL CHECK (minimum_character_classes BETWEEN 0 AND 4),
    updated_at                TEXT    NOT NULL
) STRICT;
