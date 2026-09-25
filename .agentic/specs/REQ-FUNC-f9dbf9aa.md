<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# REQ-FUNC-f9dbf9aa — Read and replace the password policy over the authenticated HTTP interface

Status: active. This spec also carries the exact command line of
REQ-FUNC-03c21252 and the shared policy model the security requirements
REQ-SEC-492e15ed, REQ-SEC-1583a012, REQ-SEC-06c320c8, REQ-SEC-fe469bac,
REQ-SEC-13b2e0d5 and REQ-SEC-007c3be6 and the persistence requirement
REQ-DATA-757ca87b are all judged against, because a rule split across seven
behaviors has to be one rule.

## Behavior

The appliance holds exactly one password policy with five fields:

- `minimum_length` — the fewest bytes a password may be.
- four independent requirements, each on or off: `require_lowercase`,
  `require_uppercase`, `require_digit`, `require_special`.

Each requirement is what it says: a password must carry at least one character
of that class. They are not a count. An administrator who ticks three boxes
requires those three classes, not "any three of four" - which is the question
the retired `minimum_character_classes` asked and nobody could answer.

The policy governs setting a password and nothing else. It is consulted at
bootstrap, identity creation and password reset, on every interface. It is
never consulted when a stored verifier is checked, so tightening it locks
nobody out.

The fixed maximum of 1024 bytes is not part of the policy. It stays on the
verification path, where it bounds the Argon2 work an unauthenticated login
attempt can ask this appliance to perform, and it applies on the set path too.

**Reading.** An actor holding `password.policy.manage` reads the current
policy. An appliance that has never recorded one reads the shipped default:
`minimum_length` 12 with all four requirements false. That default is what every
appliance enforces today, so an upgrade changes no behavior until an
administrator changes the policy.

**Replacing.** An actor holding `password.policy.manage` submits a complete
policy — all five values, always. A partial submission is refused rather than
merged, so a client can never silently relax the part it did not send.

**Bounds on the policy itself.** `minimum_length` is accepted from the absolute
floor of 8 up to the fixed maximum of 1024. A value outside that range is
refused with a message naming the bound, and the stored policy is left exactly
as it was. The four requirements need no bounds of their own: each is on or off.

The floor is compiled in and is not itself configurable. It sits below the
shipped default of 12 deliberately: the default keeps an unconfigured appliance
behaving exactly as it does today, while an administrator who deliberately
lowers the minimum may go as low as 8, the fewest characters NIST SP 800-63B
accepts for a user-chosen secret.

**Auditing.** An applied replacement appends exactly one audit event to the
Activity history carrying the actor, the policy before and the policy after. A
refused replacement appends nothing. No event, response, log line or error
message carries a password or a verifier.

## Data / interface

### Policy model

```go
// package auth
type PasswordPolicy struct {
    MinimumLength    int
    RequireLowercase bool
    RequireUppercase bool
    RequireDigit     bool
    RequireSpecial   bool
}

const (
    PasswordPolicyFloorBytes    = 8    // the configured minimum may not go below this
    DefaultMinimumPasswordBytes = 12   // the shipped policy, used when none is recorded
    MaxPasswordBytes            = 1024 // unchanged; the verification-path bound
)

func DefaultPasswordPolicy() PasswordPolicy
func (PasswordPolicy) Validate() error          // the policy's own bounds
func (PasswordPolicy) Check(password []byte) error  // the set path
```

A character class is decided per rune over the UTF-8 decoding of the password:
`unicode.IsLower`, `unicode.IsUpper`, `unicode.IsDigit`, and otherwise special.
The classification stays total - special is "neither a letter nor a digit", so
every rune belongs to exactly one class and any password can be judged. A byte
sequence that is not valid UTF-8 contributes special and is not an error: the
policy describes composition, it does not police encoding. Length stays a byte
count, as the existing bounds are.

`auth.validatePassword` is split into `validatePasswordLength`, which refuses
an empty password and one over the maximum and carries no minimum at all. Both
`HashPassword` and `VerifyPassword` keep it; the policy is applied above them,
by `identity.Service`, which is the only layer that can read the stored policy.

### HTTP

```http
GET /api/v1/password-policy
200 {"minimum_length": 12, "require_lowercase": false, "require_uppercase": false,
     "require_digit": false, "require_special": false}
```

```http
PUT /api/v1/password-policy
{"minimum_length": 16, "require_lowercase": true, "require_uppercase": true,
 "require_digit": true, "require_special": false}
200 (the same five values)
```

All five values are always present in both directions. A body missing any of
them is refused rather than merged, so a client can never silently relax the
part it did not send.

Both routes declare `auth.PermissionPasswordPolicyManage`
(`password.policy.manage`) in the one route table in
`internal/httpapi/server.go`. The Administrator role carries it; Operator and
Viewer do not. An actor without it receives the ordinary authorization refusal
with no policy value in the body.

Refusals from the routes themselves, in the shape the administration routes
already use:

| Condition | Status | Code | Message |
|---|---|---|---|
| body is not the documented object, or omits any of the five values | 400 | `invalid_password_policy` | the request body must carry minimum_length and all four require_ values |
| `minimum_length` below 8 or above 1024 | 400 | `invalid_password_policy` | minimum length must be between 8 and 1024 bytes |

Refusals when a password is set, on the existing `invalid_password` code, with
the message naming only the part that failed:

| Condition | Message |
|---|---|
| shorter than the configured minimum | password must be at least N bytes |
| missing required classes | password must contain a lowercase letter, a digit - only the unmet requirements, in a fixed order |
| longer than the maximum | password must be at most 1024 bytes |

### Persistence

Migration `015_password_policy` created this table with the retired count.
`016_password_policy_classes` reshapes it. **015 is never edited**: an appliance
that has already applied it recorded its digest, and `applyMigrationSet`
compares that digest on every open, so changing the file would stop such an
appliance starting rather than upgrading it.

```sql
CREATE TABLE password_policy_next (
    singleton         INTEGER PRIMARY KEY CHECK (singleton = 1),
    minimum_length    INTEGER NOT NULL CHECK (minimum_length BETWEEN 8 AND 1024),
    require_lowercase INTEGER NOT NULL CHECK (require_lowercase IN (0, 1)),
    require_uppercase INTEGER NOT NULL CHECK (require_uppercase IN (0, 1)),
    require_digit     INTEGER NOT NULL CHECK (require_digit IN (0, 1)),
    require_special   INTEGER NOT NULL CHECK (require_special IN (0, 1)),
    updated_at        TEXT    NOT NULL
) STRICT;

INSERT INTO password_policy_next
SELECT singleton, minimum_length, 0, 0, 0, 0, updated_at FROM password_policy;

DROP TABLE password_policy;
ALTER TABLE password_policy_next RENAME TO password_policy;
```

A configured minimum length is carried across; the retired count is not
converted, because no honest conversion exists - "three of four classes" names
no particular three. An appliance that had set one keeps its length and starts
again from no class required, which is the shipped behaviour. No row is seeded,
so an absent row still means the shipped default.

### Command line

```
nilo admin password-policy --server URL --token-file PATH
    [--minimum-length N] [--require lower,upper,digit,special|none]
    [--ca-file PATH] [--json]
```

With neither value, it prints the current policy. With both, it submits them
and prints what the appliance stored. With exactly one, it exits 2 naming both
options and contacts nothing — the same rule the body has, for the same reason. It is a
remote-mode command only, like `nilo activity retention`: the policy is the
appliance's, and the appliance authorizes the token on every request.

### Browser client contract

`src/api/client.ts` gains `getPasswordPolicy(credential)` and
`setPasswordPolicy(credential, policy)` over the same two routes, with
`src/api/contract.ts` holding the path, the permission constant and the parse, with the mock backend
in `src/api/mock.ts` applying the same bounds so the Authentication page behaves
the same against it. REQ-UX-5e428717 owns what the page shows.

## Examples

- Default appliance, `GET` → minimum 12 with all four requirements false.
- `PUT` minimum 16 with lowercase, uppercase and digit required → 200, echoed
  back. A following `POST /api/v1/users` with `"Passw0rd-1234"` (13 bytes) is
  refused 400 `invalid_password`, "password must be at least 16 bytes".
- `PUT` minimum 7 with nothing required → 400 `invalid_password_policy`,
  "minimum length must be between 8 and 1024 bytes"; a following `GET` still
  returns the previous policy.
- `PUT {"minimum_length": 16}`, omitting the four requirements → 400
  `invalid_password_policy`, the documented shape message; the stored policy is
  unchanged.
- Policy minimum 12 requiring uppercase and a digit: `"passwordpassword"` is
  refused, "password must contain an uppercase letter, a digit";
  `"Password1234"` is accepted. `"password1234"` is refused naming only the
  uppercase letter, because that is the only requirement it misses.
- An identity created with a 12-byte password, then policy raised to 24: that
  identity still signs in; a reset of its password to the same 12 bytes is
  refused.
- `admin` / `admin_password` published with the image is 14 bytes carrying a
  lowercase letter and a special character. It still signs in on an appliance
  whose policy has since been raised to 24 with every class required, because
  sign-in never consults the policy.
- A 2000-byte password at `POST /api/v1/session` is refused before any key is
  derived, under every policy.

## Acceptance

- A read returns the stored policy, and the shipped default where none is
  stored; a complete replacement is stored, echoed and survives reopening the
  same data directory.
- A body omitting any of the five values, and a minimum below 8 or above 1024,
  are each refused with the documented code and leave the stored policy readable
  and unchanged.
- An appliance carrying the 015 schema opens on 016 without losing a configured
  minimum length, and starts from no class required.
- An Operator session, a Viewer session, an unscoped token and an
  unauthenticated caller are each refused both the read and the replacement,
  with no policy value in the body.
- With a tightened policy, a password set through `POST /api/v1/users`,
  `PUT /api/v1/users/{username}/password`, `nilo admin user create` and
  `nilo admin user reset-password` is refused unless it meets the length and
  every required class, and the message names each requirement it missed.
- With the same tightened policy, every identity created beforehand still signs
  in, and a password longer than 1024 bytes is still refused before key
  derivation.
- Each applied replacement appends exactly one audit event carrying the actor
  and the before and after policy and no secret; each refused replacement
  appends none.

## Assumptions and open decisions

- The policy is read from the database on each password set rather than cached,
  so a change takes effect immediately and nothing has to be invalidated. The
  read is one indexed singleton row against a write that is already dominated by
  a deliberately slow Argon2 derivation.
- The policy is loaded before hashing and outside the administrative
  transaction, preserving the existing rule that the slow hash never holds the
  database write lock.
- The construction-time placeholder verifier in `identity.NewService`, which
  exists so an unknown username costs the same time as a known one, is not a
  user password and is not held to the policy. It is hashed before any policy
  can be read.
- The command line reports a refusal with the appliance's stable code and the
  appliance's own message. `internal/apiclient` carries both, so a rule the
  appliance owns is never restated by a client that only reports it refused.
