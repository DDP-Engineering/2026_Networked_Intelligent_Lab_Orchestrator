# REQ-SEC-de8b193c — Seed the first administrator from a protected file

## Behavior

The local command reads one password from a protected regular file, uses the
existing password policy and identity transaction, and invalidates the source
only after the administrator commit is durable. It never logs or echoes the
password. It succeeds idempotently only when the requested enabled
administrator already exists; any different populated identity state is a
conflict.

## Data / interface

```text
nilo seed-admin --data-dir <absolute> --username <name>
                --password-file <absolute>
```

The file must be an absolute, non-linked regular file, owned by root or the
current effective user, with no group/other permission bits. The command reads
at most the password-policy maximum plus one line ending, clears its byte
buffer, and unlinks the source after success followed by parent-directory
fsync. Password bytes never appear in arguments, output, audit detail or errors.

## Examples

An empty appliance store plus a mode-0600 file creates one Administrator and
removes the file. A symlink, mode-0644 file, weak password or store containing
another user is refused. Repeating after a successful unlink is unnecessary;
recreating the same protected input and rerunning reports the requested state.

## Acceptance

Command and service tests cover success, idempotence, conflicts, policy error,
transaction failure, concurrent calls, file replacement and cleanup failure.
No development credential constant is used.

## Assumptions and open decisions

Existing-host installers require an operator-supplied file. Pi image builds
require an explicit protected build input; no repository or universal default
credential exists.
