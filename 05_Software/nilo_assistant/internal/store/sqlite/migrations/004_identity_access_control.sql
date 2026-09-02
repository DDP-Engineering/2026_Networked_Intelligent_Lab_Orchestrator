-- Identity and access control. Forward-only: existing identities and tokens
-- keep their rows, every existing identity becomes a Viewer so that nothing
-- silently gains administrative rights, and every existing token keeps an
-- empty scope list, which means "not narrowed" and follows its owner's role.

ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'viewer'
    CHECK (role IN ('administrator', 'operator', 'viewer'));

ALTER TABLE users
    ADD COLUMN disabled_at TEXT;

ALTER TABLE mcp_tokens
    ADD COLUMN name TEXT NOT NULL DEFAULT '';

ALTER TABLE mcp_tokens
    ADD COLUMN scopes TEXT NOT NULL DEFAULT '';

ALTER TABLE mcp_tokens
    ADD COLUMN expires_at TEXT;

-- Browser sessions. Named user_sessions because `sessions` already holds
-- operation sessions, which are an unrelated concept.
CREATE TABLE IF NOT EXISTS user_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    verifier BLOB NOT NULL UNIQUE,
    csrf_verifier BLOB NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS user_sessions_by_user ON user_sessions(user_id);
