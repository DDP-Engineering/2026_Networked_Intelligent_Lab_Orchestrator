-- The password policy becomes four named requirements instead of a count.
--
-- `015_password_policy` stored `minimum_character_classes`: how many distinct
-- classes a password had to draw from. An administrator could not act on that
-- number, because it never said which classes. It is replaced by one flag per
-- class, which is what the interface now offers as four checkboxes.
--
-- 015 is not edited to say this. An appliance that has already applied it
-- recorded that file's digest, and every open compares the two, so rewriting
-- 015 would stop such an appliance starting rather than upgrading it. The
-- forward step is this migration.
--
-- A configured minimum length is carried across. The retired count is not
-- converted: "three of four classes" names no particular three, so there is no
-- honest mapping onto named requirements. An appliance that had configured one
-- keeps its length and starts again from no class required, which is the rule
-- the appliance ships with. A database with no policy row stays empty, and an
-- absent row still means the shipped default.
CREATE TABLE password_policy_next (
    singleton         INTEGER PRIMARY KEY CHECK (singleton = 1),
    minimum_length    INTEGER NOT NULL CHECK (minimum_length BETWEEN 8 AND 1024),
    require_lowercase INTEGER NOT NULL CHECK (require_lowercase IN (0, 1)),
    require_uppercase INTEGER NOT NULL CHECK (require_uppercase IN (0, 1)),
    require_digit     INTEGER NOT NULL CHECK (require_digit IN (0, 1)),
    require_special   INTEGER NOT NULL CHECK (require_special IN (0, 1)),
    updated_at        TEXT    NOT NULL
) STRICT;

INSERT INTO password_policy_next (
    singleton, minimum_length,
    require_lowercase, require_uppercase, require_digit, require_special,
    updated_at
)
SELECT singleton, minimum_length, 0, 0, 0, 0, updated_at FROM password_policy;

DROP TABLE password_policy;

ALTER TABLE password_policy_next RENAME TO password_policy;
