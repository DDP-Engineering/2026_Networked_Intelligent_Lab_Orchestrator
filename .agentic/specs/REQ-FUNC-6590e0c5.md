# REQ-FUNC-6590e0c5 — Administer identities and API tokens remotely from the command line

This spec also carries the confirmation contract of REQ-SEC-bfd1cf86, because
the two are one command surface.

## Behavior

`nilo admin user ...` and `nilo admin token ...` run in exactly one of two modes:

| Mode | Selected by | Authority | Actor attribution |
|---|---|---|---|
| local | `--data-dir D` | write access to the data directory | `--actor` names an enabled administrator |
| remote | `--server URL` **and** `--token-file PATH` | the bearer token's owner and scopes, checked by the appliance on every request | the token, resolved by the appliance; `--actor` is refused |

Mode resolution, before anything else happens:

- `--data-dir` together with `--server` or `--token-file`: usage error, exit 2,
  message "use either --data-dir or --server with --token-file, not both".
- `--server` without `--token-file`, or `--token-file` without `--server`:
  usage error, exit 2, message naming the missing option.
- neither: usage error, exit 2, the admin usage text.
- `--actor` in remote mode: usage error, exit 2 (the appliance attributes a
  remote change to the token in use; `--actor` is a local-mode option).
- `--ca-file`, `--json`, `--confirm-delete` in local mode: usage error, exit 2
  (they have no local meaning).
- `nilo admin bootstrap` is local only and unchanged.
- `user delete` and `token delete` are remote only; in local mode they are a
  usage error naming the remote options (RULING-001).

Local mode is otherwise unchanged: same flags, same JSON output, same exit codes.

`--acknowledge` is not an admin flag. It is rejected like any unknown flag
(exit 2) and is never read as consent.

Remote mode never reads a token from a flag or the environment. The token is
read from `--token-file` by `internal/apiclient`, exactly as `nilo network`
does; a missing, empty or unreadable file exits 1 before any request.

## Data / interface

```text
nilo admin user list           --server URL --token-file PATH [--ca-file PATH] [--json]
nilo admin user create         ... --username U --role administrator|operator|viewer   (password on stdin, twice)
nilo admin user set-role       ... --username U --role R
nilo admin user enable         ... --username U
nilo admin user disable        ... --username U
nilo admin user reset-password ... --username U                                        (password on stdin, twice)
nilo admin user delete         ... --username U [--confirm-delete]
nilo admin token list          ...
nilo admin token create        ... --owner U --name N [--scopes a,b] [--expires-in 720h]
nilo admin token revoke        ... --id T
nilo admin token delete        ... --id T [--confirm-delete]
```

Request mapping (all with `Authorization: Bearer <token>` and
`Accept: application/json`):

| Command | Request | Body | Success |
|---|---|---|---|
| user list | `GET /api/v1/users` | - | 200 listing |
| user create | `POST /api/v1/users` | `{"username","role","password"}` | 201 identity |
| user set-role | `PUT /api/v1/users/{u}/role` | `{"role"}` | 204, then re-read `GET /api/v1/users` |
| user enable / disable | `PUT /api/v1/users/{u}/enabled` | `{"enabled":true|false}` | 204, then re-read |
| user reset-password | `PUT /api/v1/users/{u}/password` | `{"password"}` | 204, then re-read |
| user delete | `DELETE /api/v1/users/{u}` | - | 204, then re-read |
| token list | `GET /api/v1/tokens` | - | 200 listing |
| token create | `POST /api/v1/tokens` | `{"owner","name","scopes":[...],"expires_at":RFC3339?}` | 201 token + secret |
| token revoke | `POST /api/v1/tokens/{id}/revoke` | `{}` | 204, then re-read `GET /api/v1/tokens` |
| token delete | `DELETE /api/v1/tokens/{id}` | - | 204, then re-read |

`--expires-in` is converted to an absolute UTC RFC 3339 `expires_at` on the
client, mirroring the local command; the appliance validates that it lies in
the future. `--scopes` is split on commas and trimmed; an empty list is sent as
`[]`, which the appliance reads as "not narrowed".

The password is read exactly as the local command reads it: two lines from
standard input, refused on mismatch (exit 2), never echoed by NILO, never
printed, never placed in a flag, an error message or an audit detail. The
appliance enforces the length rule and answers `invalid_password`.

### Output

- `--json`: the appliance's own document, trimmed, on standard output and
  nothing else on standard output. A 204 answer prints the re-read listing
  document. This is the automation contract.
- default: a readable rendering on standard output.
  - user listing: a header line, then one line per identity with the columns
    `USERNAME ROLE STATE ID` (state `enabled`/`disabled`).
  - token listing: a header line, then one line per token with the columns
    `ID NAME OWNER STATE EXPIRES` (state `revoked`/`expired`/`active`, in that
    precedence; expiry `never` or RFC 3339).
  - created identity: `created user "<username>" (<role>, id <id>)`.
  - created token: the metadata lines followed by `secret: <value>` on
    standard output, and on standard error the warning
    `The token secret is shown once and cannot be retrieved later; store it now.`
    The warning goes to standard error in both modes so `--json` output stays
    pure.
- Everything that is not data (prompts, the deletion description, warnings,
  errors) goes to standard error.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | the appliance applied the request |
| 1 | the appliance refused (`400`, `401`, `403`, `404`, `409`, `429`, `5xx`), or cannot be reached, or the token file cannot be read, or an interactive confirmation was cancelled |
| 2 | usage: unknown flag, mode conflict, missing option, password mismatch, noninteractive deletion without `--confirm-delete` |

A refusal prints the apiclient error, which carries the appliance's stable
error code (`last_administrator`, `user_has_tokens`, `user_not_found`,
`token_not_found`, `forbidden`, `unauthorized`, ...). No message is built from
the response body beyond that code.

### Deletion confirmation (REQ-SEC-bfd1cf86)

1. The command reads the listing (`GET /api/v1/users` or `GET /api/v1/tokens`)
   and prints on standard error what will be removed:
   `This permanently deletes user "operator" (id 0199..., role operator, enabled). Its browser sessions are removed with it.`
   or
   `This permanently deletes API token 0199... (name "ci", owner operator, active, expires never). The record cannot be recovered.`
   When the listing does not contain the subject, it prints
   `No listed <user|API token> matches "<subject>"; the appliance decides.` and
   continues, so an already-deleted or unknown subject reaches the appliance
   and is answered with the existing `404` semantics.
2. Interactive (standard input is a terminal): prompt
   `Type the username to confirm: ` / `Type the token ID to confirm: `. Only
   an exact match proceeds. Anything else prints `deletion cancelled; nothing deleted`
   and exits 1 with no further request.
3. Noninteractive: without `--confirm-delete` print
   `noninteractive deletion requires --confirm-delete; nothing deleted` and exit 2
   with no further request. With it, proceed.
4. Send `DELETE`. On 204 print `deleted user "operator"` (or `deleted API token <id>`)
   on standard error and the re-read listing on standard output. On a refusal
   exit 1 with the appliance's code.

`--confirm-delete` on an interactive terminal skips the typed prompt: it is
the same explicit consent, given up front.

The listing read in step 1 is informational, not a gate. When the appliance
answers it with `403` (a token that holds `users.manage` or
`tokens.manage.all` without the matching read permission), the command says
the subject cannot be described here and continues to the confirmation; any
other failure of that read (unreachable, `401`, `5xx`) stops the command with
exit `1` before anything destructive is attempted.

After an applied change (a `204`, or a `201`), the listing re-read is likewise
informational: if it fails, the command reports on standard error that the
change was applied but the listing could not be re-read, prints nothing on
standard output, and exits `0`. A change that the appliance applied is never
reported as a failure.

Usernames are folded to lower case and trimmed by the command before they are
placed in a path, described or confirmed, because that is what the appliance
does with them (`--username Operator` addresses `operator`); the typed
confirmation is folded the same way. Token identifiers are matched exactly.

## Examples

```bash
$ nilo admin user delete --username operator --server https://nilo.local --token-file /etc/nilo/admin.token
This permanently deletes user "operator" (id 0199..., role operator, enabled). Its browser sessions are removed with it.
Type the username to confirm: operator
deleted user "operator"
USERNAME  ROLE           STATE    ID
admin     administrator  enabled  0199...
```

```bash
$ nilo admin token delete --id 0199... --server https://nilo.local --token-file /etc/nilo/admin.token </dev/null
This permanently deletes API token 0199... (name "ci", owner operator, active, expires 2026-10-02T09:00:00Z). The record cannot be recovered.
noninteractive deletion requires --confirm-delete; nothing deleted
$ echo $?
2
```

```bash
$ nilo admin user delete --username admin --confirm-delete --json --server https://nilo.local --token-file /etc/nilo/admin.token </dev/null
the appliance refused the request: last_administrator (HTTP 409)
$ echo $?
1
```

## Acceptance

Every row of the request mapping is exercised by a recording-stub test; every
row of the mode-resolution list exits 2 without a request; the confirmation
steps 2-4 are exercised for both subjects; the local command tests are
unchanged and still pass.

## Assumptions and open decisions

- Human-readable formats are this command's own and may change; `--json` is
  the stable contract, as for `nilo network`.
- Local-mode deletion is deliberately not offered (RULING-001).
