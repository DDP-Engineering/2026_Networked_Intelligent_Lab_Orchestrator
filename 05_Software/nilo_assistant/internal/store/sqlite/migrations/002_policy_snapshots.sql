CREATE TABLE IF NOT EXISTS policy_snapshots (
    id TEXT PRIMARY KEY,
    integration_id TEXT NOT NULL,
    operation_name TEXT NOT NULL CHECK (
        length(operation_name) BETWEEN 1 AND 128
        AND operation_name NOT GLOB '*[^a-z0-9_]*'
        AND substr(operation_name, 1, 1) GLOB '[a-z]'
    ),
    device_id TEXT NOT NULL,
    app_known INTEGER NOT NULL CHECK (app_known IN (0, 1)),
    app_mode TEXT NOT NULL CHECK (app_mode IN ('ask', 'auto', 'full')),
    manifest_policy TEXT NOT NULL CHECK (manifest_policy IN ('auto_eligible', 'full_only', 'always_prompt', 'denied')),
    operation_override TEXT CHECK (
        operation_override IS NULL
        OR operation_override IN ('auto_eligible', 'full_only', 'always_prompt', 'denied')
    ),
    device_known INTEGER NOT NULL CHECK (device_known IN (0, 1)),
    device_mode TEXT CHECK (
        device_mode IS NULL
        OR device_mode IN ('ask', 'auto', 'full')
    ),
    effective_mode TEXT NOT NULL CHECK (effective_mode IN ('ask', 'auto', 'full')),
    effective_policy TEXT NOT NULL CHECK (effective_policy IN ('auto_eligible', 'full_only', 'always_prompt', 'denied')),
    decision TEXT NOT NULL CHECK (decision IN ('allow', 'prompt', 'deny')),
    reason TEXT NOT NULL CHECK (
        length(reason) BETWEEN 1 AND 64
        AND reason IN (
            'denied_policy_forced_deny',
            'unknown_app_forced_ask',
            'unknown_device_forced_ask',
            'device_policy_lowered',
            'policy_matrix_applied'
        )
    ),
    resolved_at TEXT NOT NULL CHECK (
        length(resolved_at) BETWEEN 20 AND 30
        AND substr(resolved_at, -1, 1) = 'Z'
        AND substr(resolved_at, 1, 19) GLOB
            '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]'
        AND strftime('%Y-%m-%dT%H:%M:%SZ', resolved_at) IS NOT NULL
        AND strftime('%Y-%m-%dT%H:%M:%SZ', resolved_at) =
            substr(resolved_at, 1, 19) || 'Z'
        AND (
            length(resolved_at) = 20
            OR (
                length(resolved_at) BETWEEN 22 AND 30
                AND substr(resolved_at, 20, 1) = '.'
                AND substr(resolved_at, 21, length(resolved_at) - 21)
                    NOT GLOB '*[^0-9]*'
                AND substr(resolved_at, length(resolved_at) - 1, 1)
                    BETWEEN '1' AND '9'
            )
        )
    ),
    CHECK (
        (device_known = 1 AND device_mode IS NOT NULL)
        OR (device_known = 0 AND device_mode IS NULL)
    )
);
