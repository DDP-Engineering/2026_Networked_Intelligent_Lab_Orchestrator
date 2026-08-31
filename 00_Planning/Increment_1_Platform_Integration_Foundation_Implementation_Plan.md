# NILO Increment 1 Platform and Integration Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Governance:** Execute this plan inside the repository's AEGIS lifecycle. AEGIS owns scope, requirements, design traceability, journals, validation evidence, delivery approval, and Git authority; Superpowers owns task decomposition, TDD execution, task-level review, and verification discipline during the AEGIS build and validate stages.

**Goal:** Build the portable, trusted NILO foundation and prove it with a supervised fake Python integration completing the entire policy, approval, execution, evidence, cancellation, crash, and restart lifecycle.

**Architecture:** A Go Bench Core owns configuration, authentication primitives, policy, immutable requests, sessions, leases, evidence, and process supervision. Python integrations run out of process and communicate through a versioned, language-neutral NDJSON protocol on standard input/output; SQLite is the transactional store. Linux ARM64 is release-blocking, while the portable core must also compile for Linux x86_64 and Windows x86_64 from the first increment.

**Tech Stack:** Go 1.26+, `modernc.org/sqlite` v1.57.0, `github.com/google/uuid` v1.6.0, `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3, `golang.org/x/crypto` v0.55.0; Python 3.11+, Pydantic 2.13.5, pytest 9.1.1, pytest-asyncio 1.4.0, Ruff 0.16.5; JSON Schema 2020-12; SQLite; PowerShell and POSIX verification scripts.

**Spec:** `00_Planning/Remote_Bench_MVP_PRD.md`

## Global Constraints

- This plan implements only PRD Increment 1. It does not implement sigrok, MCP/HTTP routes, browser GUI, WireGuard, packaging, or hardware-in-the-loop tests.
- Implementation is governed by `.agentic/AGENTS.md`. Start or resume one AEGIS task for Increment 1, complete its frame/specify/design obligations, and execute this Superpowers plan only when that task reaches build.
- Use the pinned repository-local toolchain under `02_Tools`; bootstrap Go, uv, and managed Python without a system-wide installation before running implementation checks.
- Raspberry Pi 4 Linux ARM64 is the release-blocking platform. Every portable Go package must also compile for Linux x86_64 and Windows 11 x86_64. Platform-specific behavior belongs behind narrow adapters and build-tagged files.
- Bench Core never accepts arbitrary shell commands, unbounded messages, or unrestricted filesystem paths. Integration input, output, manifests, configuration, and artifacts are typed, size-bounded, and schema-validated.
- Python integrations run as independently supervised processes. Protocol output uses stdout exclusively; diagnostics use stderr. Dependencies remain isolated from Bench Core.
- Use test-driven development: add one failing behavior test, observe the expected failure, add the smallest implementation, then rerun focused and broader checks.
- The user authorized necessary commits on `ddp/increment-1-foundation` on 2026-08-30. Each Superpowers task may stage and commit only its reviewed files so task-scoped review ranges and recovery remain reliable; pushing, merging, PR publication, and release actions remain reserved for explicit AEGIS deliver approval.
- Preserve unrelated and pre-existing worktree changes. `.superpowers/` and the PRD may remain untracked during implementation unless the user directs otherwise.

## Combined AEGIS and Superpowers Execution Model

1. **AEGIS frame:** Open or resume the Increment 1 task journal, confirm scope and Git limits, choose the full path, and stop at the normal frame checkpoint.
2. **AEGIS specify:** Convert the approved PRD requirements used by Increment 1 into traceable AEGIS requirement files and link this implementation plan as the detailed execution specification.
3. **AEGIS design:** Record the Go-core/Python-runner boundary, persistence model, protocol boundary, platform adapters, and threat-relevant trust boundaries. Reconcile any design change back into the PRD and this plan before code.
4. **AEGIS build:** Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to execute Tasks 1-15 in order. For every task, follow the red-green TDD steps, review the diff, and append exact commands/results to the AEGIS journal.
5. **AEGIS validate:** Use `superpowers:verification-before-completion` for fresh proof while completing the AEGIS review, knowledge, full-suite, strict-check, and platform-evidence obligations. A task-level green result does not replace Increment 1 validation.
6. **AEGIS deliver:** Present the validated branch and AEGIS report at the mandatory approval stop. Push, merge, create a PR, or publish only to the extent explicitly authorized by the user.

When the two workflows phrase the same control differently, AEGIS governs lifecycle state, journal evidence, approval stops, and Git actions. Superpowers governs how the current implementation task is decomposed, tested, reviewed, and verified within that state.

## Target Repository Layout

```text
05_Software/
  nilo_assistant/
    go.mod
    go.sum
    cmd/nilo/main.go
    internal/
      app/
      artifact/
      audit/
      auth/
      config/
      domain/
      integration/
        manifest/
        protocol/
        supervisor/
      operation/
      policy/
      session/
      store/sqlite/
  schemas/integration/v1/
  integration_sdk/python/
    pyproject.toml
    src/nilo_integration/
    tests/
  integrations/fake_python/
    manifest.json
    schemas/
    fake_integration.py
    tests/
  tests/contract/
  scripts/
```

## Stable Increment 1 Contracts

The task sequence below may add implementations, but these boundaries should not drift without updating the PRD and this plan first.

```go
// internal/policy/service.go
type Resolver interface {
    Resolve(ctx context.Context, input ResolveInput) (Decision, error)
}

// internal/session/service.go
type Service interface {
    Create(ctx context.Context, request domain.OperationRequest, decision policy.Decision) (domain.Session, error)
    Transition(ctx context.Context, sessionID domain.SessionID, from, to domain.SessionState) (domain.Session, error)
}

// internal/integration/supervisor/supervisor.go
type Supervisor interface {
    Describe(ctx context.Context, packageID string) (protocol.DescribeResult, error)
    Execute(ctx context.Context, req protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error)
    Cancel(ctx context.Context, packageID string, requestID domain.RequestID) error
}

// internal/operation/service.go
type Service interface {
    Submit(ctx context.Context, cmd SubmitCommand) (domain.Session, error)
    Approve(ctx context.Context, sessionID domain.SessionID, actor domain.Actor) (domain.Session, error)
    Deny(ctx context.Context, sessionID domain.SessionID, actor domain.Actor, reason string) error
    Cancel(ctx context.Context, sessionID domain.SessionID, actor domain.Actor) error
    Get(ctx context.Context, sessionID domain.SessionID) (domain.Session, error)
}
```

---

## Task 1: Establish the portable Go core and truthful repository documentation

**Files:**

- Modify: `README.md`
- Modify: `05_Software/README.md`
- Modify: `05_Software/nilo_assistant/README.md`
- Modify: `05_Software/tests/README.md`
- Create: `05_Software/nilo_assistant/go.mod`
- Create: `05_Software/nilo_assistant/cmd/nilo/main.go`
- Create: `05_Software/nilo_assistant/internal/buildinfo/buildinfo.go`
- Create: `05_Software/nilo_assistant/internal/buildinfo/buildinfo_test.go`
- Modify: `.agentic/aegis.toml`
- Create: `05_Software/scripts/aegis_test.py`
- Create: `05_Software/scripts/tests/test_aegis_test.py`
- Create: `05_Software/scripts/verify-portable-core.ps1`
- Create: `05_Software/scripts/verify-portable-core.sh`

**Consumes:** The approved Go-core/Python-plugin decision and target platform list from the PRD.

**Produces:** A buildable Go module named `nilo.local/nilo`, a version-reporting executable, cross-target compile checks, and documentation that no longer describes NILO as Python-first.

- [ ] Verify prerequisites from the repository root:

```powershell
go version
python --version
uv --version
```

Expected: Go 1.26 or newer, Python 3.11 or newer, and a working `uv`. Stop Task 1 if Go is unavailable.

- [ ] Initialize `go.mod` with the exact module and baseline dependencies:

```go
module nilo.local/nilo

go 1.26.0

require (
    github.com/google/uuid v1.6.0
    github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
    golang.org/x/crypto v0.55.0
    modernc.org/sqlite v1.57.0
)
```

- [ ] Write the failing build information test:

```go
func TestCurrentHasProtocolAndVersion(t *testing.T) {
    got := Current()
    if got.Name != "nilo" || got.Version == "" || got.IntegrationProtocol != "nilo.integration/1" {
        t.Fatalf("unexpected build info: %#v", got)
    }
}
```

- [ ] Run the focused test from `05_Software/nilo_assistant`:

```powershell
go test ./internal/buildinfo
```

Expected: FAIL because `Current` and its result type do not exist.

- [ ] Add the minimal implementation:

```go
package buildinfo

type Info struct {
    Name                string `json:"name"`
    Version             string `json:"version"`
    IntegrationProtocol string `json:"integration_protocol"`
}

var Version = "0.1.0-dev"

func Current() Info {
    return Info{Name: "nilo", Version: Version, IntegrationProtocol: "nilo.integration/1"}
}
```

- [ ] Make `cmd/nilo/main.go` encode `buildinfo.Current()` as JSON for `nilo version`, return exit code 2 for unknown arguments, and contain no service behavior yet.

- [x] Update all four README files to state: Go Bench Core, external Python integrations, Linux ARM64 first, Linux x86_64 later, Windows x86_64 compile-only, and Increment 1 scope.

- [ ] Add both verification scripts. Each must run `go test ./...`, build the host executable, then cross-compile with `CGO_ENABLED=0` for `linux/arm64`, `linux/amd64`, and `windows/amd64` into a temporary directory that is removed on success or failure.

- [ ] Write a failing standard-library Python test for a portable AEGIS test router that discovers the Go module and any Python integration projects beneath `05_Software`, then add `scripts/aegis_test.py` with `full`, `impacted`, and `lint` modes. `impacted` may conservatively run every discovered suite until selector routing is proven; it must never omit a suite silently.

- [ ] Update `.agentic/aegis.toml` commands to run `python scripts/aegis_test.py full`, `python scripts/aegis_test.py impacted {tests}`, and `python scripts/aegis_test.py lint` from `05_Software`. Run the router test directly before relying on the new AEGIS commands.

- [ ] Run focused, full, and cross-target checks:

```powershell
go test ./internal/buildinfo
go test ./...
..\scripts\verify-portable-core.ps1
```

Expected: all commands succeed and no binary is left in the repository.

- [ ] Record Task 1 changed files and red/green/cross-target evidence in the AEGIS journal, then commit only Task 1 files with message `build: establish portable Go core` for task review; do not push.

---

## Task 2: Add validated configuration and platform path adapters

**Files:**

- Create: `05_Software/nilo_assistant/internal/config/config.go`
- Create: `05_Software/nilo_assistant/internal/config/config_test.go`
- Create: `05_Software/nilo_assistant/internal/config/paths.go`
- Create: `05_Software/nilo_assistant/internal/config/paths_unix.go`
- Create: `05_Software/nilo_assistant/internal/config/paths_windows.go`
- Create: `05_Software/nilo_assistant/internal/config/paths_test.go`

**Consumes:** An OS environment and an optional explicit data-directory override.

**Produces:** Validated immutable runtime configuration and platform-neutral absolute paths for database, artifacts, integrations, and diagnostics.

- [ ] Write table-driven failing tests for defaults, an invalid relative data directory, a missing bind address, and artifact paths escaping the data root:

```go
func TestLoadRejectsRelativeDataDir(t *testing.T) {
    _, err := Load(Inputs{DataDir: "relative/data", BindAddress: "127.0.0.1:8080"})
    if !errors.Is(err, ErrDataDirNotAbsolute) {
        t.Fatalf("expected ErrDataDirNotAbsolute, got %v", err)
    }
}
```

- [ ] Run:

```powershell
go test ./internal/config
```

Expected: FAIL because `Load`, `Inputs`, and `ErrDataDirNotAbsolute` do not exist.

- [ ] Implement these exact public values:

```go
type Inputs struct {
    DataDir     string
    BindAddress string
}

type Config struct {
    DataDir        string
    DatabasePath   string
    ArtifactRoot   string
    IntegrationDir string
    BindAddress    string
}

func Load(in Inputs) (Config, error)
func ResolveWithin(root, candidate string) (string, error)
```

`ResolveWithin` must use cleaned absolute paths plus `filepath.Rel`; reject `..`, absolute escapes, empty candidates, and equality when a file rather than a directory is required. Do not use string-prefix path checks.

- [ ] Put only default directory discovery behind `defaultDataDir()` in build-tagged files. Unix uses `$XDG_STATE_HOME/nilo` or `/var/lib/nilo`; Windows uses `%ProgramData%\NILO`. Tests supply overrides and must not write to either default.

- [ ] Run:

```powershell
go test ./internal/config
go test ./...
..\scripts\verify-portable-core.ps1
```

Expected: validation tests and all portable builds pass.

- [ ] Record Task 2 evidence, then commit only Task 2 files with message `feat: add portable configuration paths` for task review; do not push.

---

## Task 3: Define immutable domain identities, requests, and session transitions

**Files:**

- Create: `05_Software/nilo_assistant/go.sum`
- Create: `05_Software/nilo_assistant/internal/domain/ids.go`
- Create: `05_Software/nilo_assistant/internal/domain/operation.go`
- Create: `05_Software/nilo_assistant/internal/domain/session.go`
- Create: `05_Software/nilo_assistant/internal/domain/session_test.go`

**Consumes:** Validated identifiers and canonical JSON parameters.

**Produces:** Strong ID types, an immutable operation request value, the approved session state set, and one authoritative transition function.

- [ ] Write failing transition tests covering every allowed edge and representative forbidden edges:

```go
func TestSessionTransitions(t *testing.T) {
    allowed := [][2]SessionState{
        {PendingApproval, Queued}, {Queued, Running},
        {Running, Completed}, {Running, Failed},
        {Running, Cancelled}, {Running, Incomplete},
    }
    for _, edge := range allowed {
        if err := ValidateTransition(edge[0], edge[1]); err != nil {
            t.Errorf("%s -> %s: %v", edge[0], edge[1], err)
        }
    }
    if err := ValidateTransition(Completed, Running); !errors.Is(err, ErrInvalidTransition) {
        t.Fatalf("expected terminal transition rejection, got %v", err)
    }
}
```

- [ ] Run `go test ./internal/domain`; expect compile failure.

- [ ] Implement typed IDs for request, session, actor, device, integration, artifact, policy snapshot, and configuration snapshot. Constructors use UUID v7; parsers reject invalid UUIDs. Operation descriptor IDs remain stable validated strings such as `capture` or `read_registers`, not runtime UUIDs.

- [ ] Implement the immutable request as a value with copied canonical parameter bytes:

```go
type OperationRequest struct {
    id                    RequestID
    actor                 Actor
    integrationID         IntegrationID
    operationName         string
    deviceID              DeviceID
    canonicalParameters   json.RawMessage
    configurationSnapshot SnapshotID
    policySnapshot        SnapshotID
    deadline              time.Time
    createdAt             time.Time
}
```

Do not expose fields or setters. Construct through `NewOperationRequest`, which rejects empty identity, invalid canonical JSON, non-future deadline, and parameter payloads over 1 MiB. Expose value-returning getters for every field; `CanonicalParameters()` must return a fresh defensive copy on every call so neither constructor input nor accessor output can mutate the accepted request.

- [ ] Define exact states: `pending_approval`, `queued`, `running`, `completed`, `failed`, `cancelled`, and `incomplete`. Rejected or expired approvals remain approval records and do not create a new runner state.

- [ ] Run `go test ./internal/domain`, `go test ./...`, and the portable-core script; expect all pass.

- [ ] Record Task 3 evidence, then commit only Task 3 files with message `feat: define operation and session domain` for task review; do not push.

---

## Task 4: Create the transactional SQLite repository and migrations

**Files:**

- Modify: `05_Software/nilo_assistant/go.mod`
- Modify: `05_Software/nilo_assistant/go.sum`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/db.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/migrations.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/migrations/001_initial.sql`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/db_test.go`

**Consumes:** An absolute database path from `config.Config`.

**Produces:** A pure-Go SQLite connection with enforced durability settings and all Increment 1 tables created transactionally.

- [ ] Write a failing temporary-database test that opens the store, verifies `PRAGMA foreign_keys=1`, `journal_mode=wal`, a nonzero busy timeout, and the required tables:

```go
required := []string{
    "schema_migrations", "config_versions", "users", "mcp_tokens",
    "app_policies", "operation_policy_overrides", "device_policies",
    "operation_requests", "approval_records", "sessions", "resource_leases", "audit_events", "artifacts",
}
```

- [ ] Run `go test ./internal/store/sqlite`; expect failure because `Open` is absent.

- [ ] Implement:

```go
type DB struct { sql *sql.DB }

func Open(ctx context.Context, path string) (*DB, error)
func (db *DB) SQL() *sql.DB
func (db *DB) Close() error
```

Use driver `modernc.org/sqlite`. On every connection enforce `foreign_keys=ON`, `journal_mode=WAL`, `busy_timeout=5000`, and `synchronous=FULL`. Limit the pool to one writer connection for Increment 1.

Update `go.mod` only with the indirect runtime requirements needed by `modernc.org/sqlite v1.57.0`; preserve the four planned direct requirements even when later tasks have not imported all of them yet. Do not use `go mod tidy` to remove those future-task dependencies.

- [ ] Make migration 001 create the required tables with UUID text primary keys, UTC RFC3339-nanosecond timestamps, foreign keys, a partial unique index on resource name where `released_at IS NULL`, and CHECK constraints for known policy/session values. `audit_events` must have no update/delete repository method.

- [ ] Apply migrations under `BEGIN IMMEDIATE`; record the SHA-256 digest of each migration and fail startup if an already-applied migration's digest changes.

- [ ] Add tests for idempotent reopen, tampered migration digest, foreign-key rejection, and a second conflicting active lease.

- [ ] Run `go test ./internal/store/sqlite`, `go test ./...`, and cross-target compilation; expect all pass without CGO.

- [ ] Record Task 4 evidence, then commit only Task 4 files with message `feat: add transactional SQLite foundation` for task review; do not push.

---

## Task 5: Implement administrator password and MCP token primitives

**Files:**

- Create: `05_Software/nilo_assistant/internal/auth/password.go`
- Create: `05_Software/nilo_assistant/internal/auth/password_test.go`
- Create: `05_Software/nilo_assistant/internal/auth/token.go`
- Create: `05_Software/nilo_assistant/internal/auth/token_test.go`
- Create: `05_Software/nilo_assistant/internal/auth/repository.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/auth_repository.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/auth_repository_test.go`

**Consumes:** Administrator credentials and token creation/revocation commands. Network authentication routes are deferred to Increment 3.

**Produces:** Argon2id password verifiers, one-time plaintext MCP token creation, SHA-256 token lookup, and durable revocation.

- [ ] Write failing tests that assert password round-trip, wrong-password rejection, random salts, token length of 32 random bytes before encoding, plaintext returned only by creation, and revoked-token rejection.

- [ ] Run `go test ./internal/auth ./internal/store/sqlite`; expect compile failures.

- [ ] Implement password hashes in PHC form using Argon2id with explicit versioned parameters:

```go
type Argon2Params struct {
    MemoryKiB uint32 // 65536
    Time      uint32 // 3
    Threads   uint8  // 2
    SaltBytes uint32 // 16
    KeyBytes  uint32 // 32
}

func HashPassword(password []byte, random io.Reader) (string, error)
func VerifyPassword(encoded string, password []byte) (bool, error)
```

Reject passwords shorter than 12 bytes or longer than 1 KiB; clear the caller-owned password buffer at the application boundary after use.

- [ ] Implement tokens:

```go
type CreatedToken struct { ID string; Plaintext string; CreatedAt time.Time }
type TokenVerifier [32]byte

func NewToken(random io.Reader, now time.Time) (CreatedToken, TokenVerifier, error)
func VerifyToken(ctx context.Context, plaintext string) (TokenRecord, error)
func RevokeToken(ctx context.Context, id string, at time.Time) error
```

Encode plaintext as `nilo_` plus unpadded base64url. Store only SHA-256 verifier bytes and constant-time compare candidates. Never log or return plaintext after creation.

- [ ] Add SQLite repository tests for duplicate verifier rejection, revocation timestamp persistence, and unknown token behavior.

- [ ] Run focused tests, `go test ./...`, and portable compilation.

- [ ] Record Task 5 evidence, then commit only Task 5 files with message `feat: add authentication primitives` for task review; do not push.

---

## Task 6: Implement the permission, override, and approval engine

**Files:**

- Create: `05_Software/nilo_assistant/internal/policy/model.go`
- Create: `05_Software/nilo_assistant/internal/policy/resolver.go`
- Create: `05_Software/nilo_assistant/internal/policy/resolver_test.go`
- Create: `05_Software/nilo_assistant/internal/policy/repository.go`
- Modify: `05_Software/nilo_assistant/internal/domain/operation.go`
- Modify: `05_Software/nilo_assistant/internal/domain/session_test.go`
- Modify: `05_Software/nilo_assistant/internal/store/sqlite/migrations.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/migrations/002_policy_snapshots.sql`
- Modify: `05_Software/nilo_assistant/internal/store/sqlite/db_test.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/policy_repository.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/policy_repository_test.go`

**Consumes:** App mode, operation default policy, optional operation override, optional device policy, immutable request digest, and administrator decision.

**Produces:** `allow`, `prompt`, or `deny`; persistent policy snapshots; single-request approval/denial records.

- [ ] First encode the approved 4x3 matrix as the failing table test:

```go
cases := []struct{ policy OperationPolicy; mode AppMode; want DecisionKind }{
    {AutoEligible, Ask, Prompt}, {AutoEligible, Auto, Allow}, {AutoEligible, Full, Allow},
    {FullOnly, Ask, Prompt}, {FullOnly, Auto, Prompt}, {FullOnly, Full, Allow},
    {AlwaysPrompt, Ask, Prompt}, {AlwaysPrompt, Auto, Prompt}, {AlwaysPrompt, Full, Prompt},
    {Denied, Ask, Deny}, {Denied, Auto, Deny}, {Denied, Full, Deny},
}
```

- [ ] Run `go test ./internal/policy`; expect compile failure.

- [ ] Implement exact enums and resolver precedence:

1. A `Denied` effective operation policy always denies.
2. An operation override replaces the manifest default.
3. A new/unknown app or device forces Ask mode.
4. A known device may lower effective mode; it cannot silently raise the app mode.
5. Apply the approved matrix.
6. Return a snapshot containing every input and the decision reason.

- [ ] Canonicalize the immutable request and calculate `SHA-256(request canonical JSON)`. Approval records bind to this digest and expire at their stored deadline. A changed parameter set necessarily has a different request ID and digest.

- [ ] The public request-creation boundary must allocate a fresh UUID-v7 request ID itself; any distinct accepted request therefore has a distinct identity. Keep an explicit, separately named validated rehydration path only for durable stored requests. Canonical parameter JSON must either normalize equivalent numeric spellings to one JSON number representation or reject noncanonical spellings before a digest is computed.

- [ ] Add migration 002 for a dedicated `policy_snapshots` table. Do not reuse `config_versions`: configuration and policy snapshots are distinct immutable audit domains. Persist the snapshot ID, integration, operation, device, known/unknown inputs, nullable override and device mode, effective mode and policy, decision, bounded reason, and UTC resolution time with enum/boolean/nullability checks. Update the exact non-internal table-set regression to include `policy_snapshots`; migration digest verification and idempotent reopen remain mandatory.

- [ ] Add repository methods:

```go
SaveSnapshot(ctx context.Context, snapshot Snapshot) error
RecordApproval(ctx context.Context, record ApprovalRecord) error
FindValidApproval(ctx context.Context, requestID domain.RequestID, digest [32]byte, now time.Time) (ApprovalRecord, error)
```

- [ ] Test new app/device Ask behavior, persistent mode reload, override precedence, digest mismatch, expiry, duplicate decision rejection, and Full Access not bypassing `Denied` or `AlwaysPrompt`.

- [ ] Run focused tests, `go test ./...`, and portable compilation.

- [ ] Record Task 6 evidence, then commit only Task 6 files with message `feat: enforce permission and approval policy` for task review; do not push.

---

## Task 7: Add append-only audit and confined artifact stores

**Files:**

- Create: `05_Software/nilo_assistant/internal/audit/audit.go`
- Create: `05_Software/nilo_assistant/internal/audit/audit_test.go`
- Create: `05_Software/nilo_assistant/internal/domain/artifact.go`
- Create: `05_Software/nilo_assistant/internal/artifact/store.go`
- Create: `05_Software/nilo_assistant/internal/artifact/store_test.go`
- Create: `05_Software/nilo_assistant/internal/artifact/store_unix.go`
- Create: `05_Software/nilo_assistant/internal/artifact/store_windows.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/audit_repository.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/artifact_repository.go`

**Consumes:** Structured actor/action/subject events and runner-produced files staged inside an operation-specific incoming directory.

**Produces:** Append-only audit rows and content-verified artifacts stored beneath the configured root.

The `domain.Artifact` value contains its UUID-v7 artifact ID; producing request, session, device, and integration IDs; media type; fixed SHA-256 digest; verified size; terminal `domain.SessionState`; copied presentation hints; and UTC creation time. The artifact store owns construction and validates the terminal-state subset.

- [x] Write failing tests for audit sequence order, missing actor/action rejection, artifact traversal rejection, wrong declared size/digest, successful atomic move, and metadata persistence.

- [x] Run `go test ./internal/audit ./internal/artifact`; expect compile failures.

- [ ] Implement the interfaces:

```go
type audit.Repository interface {
    Append(ctx context.Context, event Event) error
    List(ctx context.Context, filter Filter) ([]Event, error)
}

type Store interface {
    Register(ctx context.Context, input RegisterInput) (domain.Artifact, error)
    Open(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.Artifact, error)
}
```

There is intentionally no normal audit mutation/deletion method.

- [x] `Register` must open the staged file without following a caller-provided destination, stream at most the configured maximum, calculate SHA-256 and size, compare declarations, fsync the completed file, atomically place it under `<root>/<session-id>/<artifact-id>`, then insert metadata transactionally. On metadata failure, retain the verified unregistered destination for reconciliation when ownership cannot be atomically proved.

- [x] Placement must be atomic no-replace: a colliding destination returns a typed already-registered error and cannot overwrite bytes. After placement, durably synchronize the containing directory on platforms that support directory sync before metadata insertion. A portable pathname compare-and-delete is not atomic; therefore post-placement metadata failure must never delete a final artifact pathname unless an OS-specific primitive proves it still identifies the created file. In its absence, preserve the verified unregistered file for later reconciliation rather than risk deleting replacement evidence. Compare staging and artifact roots by physical identity and reject aliases that collapse the separation. Open must validate the opened handle's regular-file identity, not only a prior pathname stat. Keep Unix/Windows filesystem-specific primitives behind build-tagged files.

- [x] Store media type, digest, size, producing request/session/device/integration, created timestamp, terminal-state label, and presentation hints. Reject symlinks and paths escaping the operation staging directory.

- [x] Run focused tests, `go test ./...`, and cross-target compilation.

- [x] Record Task 7 evidence, then commit only Task 7 files with message `feat: add durable audit and artifact stores` for task review; do not push.

---

## Task 8: Implement sessions, exclusive leases, and startup reconciliation

**Files:**

- Modify: `05_Software/nilo_assistant/internal/domain/session.go`
- Create: `05_Software/nilo_assistant/internal/session/repository.go`
- Create: `05_Software/nilo_assistant/internal/session/service.go`
- Create: `05_Software/nilo_assistant/internal/session/service_test.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/session_repository.go`
- Create: `05_Software/nilo_assistant/internal/store/sqlite/session_repository_test.go`

**Consumes:** Immutable operation requests, policy decisions, resource claim strings, transitions, and startup time.

**Produces:** Durable sessions, compare-and-swap state changes, exclusive resource leases, and deterministic restart reconciliation.

Extend the existing domain package with the `domain.Session` value required by the repository contract. It carries a UUID-v7 session ID, owning request ID, state, UTC creation and update timestamps, and optional UTC terminal timestamp; construction and rehydration validate identifiers, timestamps, and terminal-state consistency without weakening the existing transition invariant.

- [x] Write failing tests for creating a pending/queued session, CAS transition conflict, double lease conflict, terminal lease release, and restart conversion of `running` to `incomplete`.

- [x] Run `go test ./internal/session ./internal/store/sqlite`; expect compile failures.

- [x] Implement the repository contract:

```go
type Repository interface {
    Create(ctx context.Context, session domain.Session) error
    Get(ctx context.Context, id domain.SessionID) (domain.Session, error)
    Transition(ctx context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error)
    AcquireLeases(ctx context.Context, id domain.SessionID, resources []string, at time.Time) error
    ReleaseLeases(ctx context.Context, id domain.SessionID, at time.Time) error
    ListInterrupted(ctx context.Context) ([]domain.Session, error)
}
```

Normalize resource claims as `<integration-id>/<resource-kind>/<stable-device-id>`; sort and deduplicate before one transaction acquires all claims. A conflict returns a typed `ResourceBusyError` with the owning session ID but no secret data.

- [x] Implement `ReconcileStartup`: list only sessions persisted as `running`; in one transaction mark each `incomplete`, append an audit event, and release leases owned by that session. Never delete a lease merely because it is old.

- [x] Test process restart by closing and reopening a temporary SQLite DB. Verify completed sessions remain unchanged and queued sessions retain their state.

- [x] Run focused tests, `go test ./...`, and portable compilation.

- [x] Record Task 8 evidence, then commit only Task 8 files with message `feat: add sessions leases and reconciliation` for task review; do not push.

---

## Task 9: Define and validate integration manifests and operation schemas

**Files:**

- Modify: `05_Software/nilo_assistant/go.mod`
- Modify: `05_Software/nilo_assistant/go.sum`
- Create: `05_Software/schemas/integration/v1/manifest.schema.json`
- Create: `05_Software/schemas/integration/v1/operation.schema.json`
- Create: `05_Software/schemas/integration/v1/event.schema.json`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/model.go`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/loader.go`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/loader_test.go`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/schemas/manifest.schema.json`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/schemas/operation.schema.json`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/schemas/event.schema.json`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/testdata/valid/manifest.json`
- Create: `05_Software/nilo_assistant/internal/integration/manifest/testdata/invalid-command/manifest.json`

**Consumes:** A package directory with `manifest.json` and referenced local schemas.

**Produces:** A validated manifest model with resolved files confined to the package root.

- [x] Write failing loader tests for a valid package, unsupported protocol, duplicate operation ID, missing policy, unsupported platform, schema path escape, absolute runner path, and a runner represented as a raw shell command.

- [x] Run `go test ./internal/integration/manifest`; expect compile failure.

- [x] Define `manifest.schema.json` with `additionalProperties: false` and these required fields: package ID/version/publisher/integrity, protocol range, OS/architecture matrix, runner executable plus argument array, app configuration schema, operation descriptors, resource claims, timeout bounds, cancellation/safe-shutdown declarations, event/artifact declarations, health checks, and contract fixtures.

- [ ] Model the runner as executable plus argument array, never a concatenated command:

```go
type Runner struct {
    Executable string   `json:"executable"`
    Arguments  []string `json:"arguments"`
}
```

- [x] Use `jsonschema/v6` with JSON Schema 2020-12. Limit manifest and each loaded schema to 1 MiB, reject remote `$ref`, resolve referenced files with `config.ResolveWithin`, and check `nilo.integration/1` compatibility.

- [ ] Map each operation to the Task 6 policy enum and require a nonempty input schema, result schema, timeout, cancellability flag, state-change flag, and normalized resource claims.

- [x] Run focused tests, `go test ./...`, schema fixture checks, and portable compilation.

- [x] Record Task 9 evidence, then commit only Task 9 files with message `feat: define integration package manifests` for task review; do not push.

---

## Task 10: Implement the bounded versioned runner wire protocol

**Files:**

- Create: `05_Software/nilo_assistant/internal/integration/protocol/message.go`
- Create: `05_Software/nilo_assistant/internal/integration/protocol/codec.go`
- Create: `05_Software/nilo_assistant/internal/integration/protocol/codec_test.go`
- Create: `05_Software/nilo_assistant/internal/integration/protocol/types.go`
- Create: `05_Software/tests/contract/fixtures/protocol-v1/hello.ndjson`

**Consumes:** One newline-delimited JSON message at a time.

**Produces:** Strictly decoded requests, responses, and events for protocol `nilo.integration/1`.

- [ ] Write failing codec tests for a valid hello exchange, unknown fields, unknown message kind, duplicate request message ID, malformed JSON, missing newline at EOF, and a line larger than 1 MiB.

- [ ] Run `go test ./internal/integration/protocol`; expect compile failure.

- [ ] Define the envelope:

```go
type Envelope struct {
    Protocol  string          `json:"protocol"`
    Kind      Kind            `json:"kind"` // request, response, event
    MessageID string          `json:"message_id"`
    RequestID string          `json:"request_id"`
    Method    string          `json:"method"`
    Timestamp time.Time       `json:"timestamp"`
    Payload   json.RawMessage `json:"payload"`
}
```

Each message has a unique `message_id`; responses and events copy the initiating request's `message_id` into `request_id`. Use methods `hello`, `describe`, `discover`, `health`, `configure`, `execute`, `cancel`, and `safe_shutdown`. An execute operation emits zero or more events and exactly one terminal response.

- [ ] Implement `Decoder` with a hard 1 MiB line limit before allocation growth, `json.Decoder.DisallowUnknownFields`, protocol equality checks, and per-connection uniqueness for request `message_id` values. Implement `Encoder` with a mutex so one JSON object plus newline is written atomically.

- [ ] Define typed progress, measurement, domain event, warning, log reference, artifact declaration, result, and error payloads. Progress is in `[0,1]`; raw logs are not embedded without a bound; timestamps are UTC.

- [ ] Add golden round-trip fixtures shared later with Python under `05_Software/tests/contract/fixtures/protocol-v1/`.

- [ ] Run focused tests, `go test ./...`, and portable compilation.

- [ ] Record Task 10 evidence, then commit only Task 10 files with message `feat: add integration wire protocol` for task review; do not push.

---

## Task 11: Build the first-class Python integration SDK

**Files:**

- Create: `05_Software/integration_sdk/python/pyproject.toml`
- Create: `05_Software/integration_sdk/python/src/nilo_integration/__init__.py`
- Create: `05_Software/integration_sdk/python/src/nilo_integration/models.py`
- Create: `05_Software/integration_sdk/python/src/nilo_integration/api.py`
- Create: `05_Software/integration_sdk/python/src/nilo_integration/server.py`
- Create: `05_Software/integration_sdk/python/tests/test_models.py`
- Create: `05_Software/integration_sdk/python/tests/test_server.py`
- Create: `05_Software/integration_sdk/python/README.md`
- Modify: `05_Software/scripts/aegis_test.py`
- Modify: `05_Software/scripts/tests/test_aegis_test.py`

**Consumes:** Protocol-v1 NDJSON on stdin and an integration implementation supplied by a package.

**Produces:** Validated Pydantic models, an async integration interface, and a protocol server that reserves stdout for messages.

- [x] Pin the Python project:

```toml
[project]
name = "nilo-integration-sdk"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["pydantic==2.13.5"]

[dependency-groups]
dev = ["pytest==9.1.1", "pytest-asyncio==1.4.0", "ruff==0.16.5"]

[build-system]
requires = ["uv_build==0.11.25"]
build-backend = "uv_build"
```

- [x] Write failing async tests that feed a hello and describe request through in-memory streams, reject extra fields and oversized input, verify one terminal response, and ensure an integration `print()` cannot corrupt protocol stdout.

- [x] Run from `05_Software/integration_sdk/python`:

```powershell
uv sync --all-groups
uv run pytest -q
```

Expected: FAIL because package models/server do not exist.

- [x] Define the integration interface:

```python
class Integration(Protocol):
    async def describe(self) -> DescribeResult: ...
    async def discover(self) -> DiscoverResult: ...
    async def health(self) -> HealthResult: ...
    async def configure(self, request: ConfigureRequest) -> ConfigureResult: ...
    async def execute(self, request: ExecuteRequest, events: EventSink) -> ExecuteResult: ...
    async def cancel(self, request_id: UUID) -> None: ...
    async def safe_shutdown(self) -> None: ...
```

- [x] Make all Pydantic models `extra="forbid"`, validate protocol equality, enforce the 1 MiB line bound, and serialize one compact JSON object per newline. `server.py` captures the original stdout buffer for protocol writes, redirects `sys.stdout` to stderr for integration diagnostics, and serializes writes through one `asyncio.Lock`.

- [x] Ensure malformed messages return typed protocol errors when an ID is recoverable; otherwise write a bounded diagnostic to stderr and terminate nonzero. Never treat EOF during a running operation as success.

- [x] Run:

```powershell
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
```

Then run the Go protocol golden tests against the same fixture directory.

- [x] Record Task 11 evidence, then commit only Task 11 files with message `feat: add Python integration SDK` for task review; do not push.

Scope correction: the generic AEGIS suite router now detects UV-managed integration projects and executes them through `uv run pytest -q`, preserving their pinned dependencies rather than attempting host-Python `unittest` discovery.

---

## Task 12: Supervise integrations as isolated external processes

**Files:**

- Create: `05_Software/nilo_assistant/internal/integration/supervisor/process.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/process_unix.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/process_windows.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/ring.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/supervisor.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/supervisor_test.go`
- Create: `05_Software/nilo_assistant/internal/integration/supervisor/testhelper/main.go`
- Modify: `05_Software/nilo_assistant/internal/integration/protocol/types.go`

**Consumes:** A validated manifest, package root, context deadline, protocol request, and event sink.

**Produces:** A handshaken runner process, validated events/results, cancellation, bounded diagnostics, and typed crash/protocol/timeout errors.

- [x] Write failing tests using the Go test-helper process for: successful handshake/execute, wrong protocol version, malformed stdout, 1 MiB overflow, stderr truncation to the last 64 KiB, timeout, cancellation, exit-before-result, and two concurrent writes.

- [x] Run `go test ./internal/integration/supervisor`; expect compile failure.

- [x] Implement the process boundary with argument arrays and `exec.CommandContext`; set working directory to the validated package root; construct an allowlisted environment rather than inheriting secrets indiscriminately.

- [x] Implement:

```go
type Supervisor interface {
    Describe(context.Context, string) (protocol.DescribeResult, error)
    Execute(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error)
    Cancel(context.Context, string, domain.RequestID) error
    Shutdown(context.Context) error
}
```

Require a successful `hello` before any other method. Validate every response/event, reject mismatched IDs, and accept exactly one terminal result.

- [x] Unix cancellation sends a protocol `cancel`, waits the configured grace period, attempts `safe_shutdown`, then terminates the process group. Windows uses the same protocol sequence and a platform adapter that kills the direct process in Increment 1; Windows Job Object ownership is a later delivery requirement and must be recorded in the adapter documentation.

- [x] Capture stderr in a concurrency-safe 64 KiB ring buffer; include only bounded sanitized text in typed errors. A runner crash never panics Bench Core.

- [x] Run focused tests with `-race` on the host where supported, `go test ./...`, and all three target builds.

- [x] Record Task 12 evidence, then commit only Task 12 files with message `feat: supervise external integration runners` for task review; do not push.

---

## Task 13: Create the deterministic fake Python integration and contract kit

**Files:**

- Create: `05_Software/integrations/fake_python/manifest.json`
- Create: `05_Software/integrations/fake_python/pyproject.toml`
- Create: `05_Software/integrations/fake_python/src/nilo_fake_integration/__init__.py`
- Create: `05_Software/integrations/fake_python/src/nilo_fake_integration/__main__.py`
- Create: `05_Software/integrations/fake_python/src/nilo_fake_integration/plugin.py`
- Create: `05_Software/integrations/fake_python/schemas/echo.input.schema.json`
- Create: `05_Software/integrations/fake_python/schemas/echo.result.schema.json`
- Create: `05_Software/integrations/fake_python/schemas/sleep.input.schema.json`
- Create: `05_Software/integrations/fake_python/tests/test_fake_integration.py`
- Create: `05_Software/tests/contract/test_integration_contract.py`
- Create: `05_Software/tests/contract/README.md`

**Consumes:** The Python SDK and generic contract fixtures.

**Produces:** An install-shaped package proving semantic operations without sigrok or hardware.

- [x] Write failing tests for manifest validation and these operations:

  - `echo`: emit two progress events and return canonical input;
  - `sleep`: run until duration or cancellation, then report cancelled;
  - `fail`: return a selected typed error without crashing;
  - `crash`: terminate after one progress event for supervisor recovery testing;
  - `artifact`: write deterministic bytes only inside the supplied staging directory and declare digest/size/media type.

- [x] Run the SDK and fake-package tests; expect failure because the fake integration does not exist.

- [x] Implement `FakeIntegration` against the Task 11 interface. Use injected clock/sleeper and deterministic UUID/bytes fixtures; do not use shell execution, hardware, network, or repository paths outside the test temporary directory.

- [x] Give the fake package an isolated `pyproject.toml` using `uv_build==0.11.25`, a `nilo-fake-integration` console entry point, and a repository-test `tool.uv.sources` path dependency on `../../integration_sdk/python`; the manifest declares executable and argument array separately.

- [x] Give operations meaningful policies: `echo` and `artifact` are `auto_eligible`; `sleep` is `full_only`; `fail` and `crash` are `always_prompt` test operations. Mark all resources as `fake/device/<stable-id>`.

- [x] Make the contract kit launch any package command supplied as executable plus argument array and verify hello, describe/manifest agreement, strict schemas, health, event ordering, one terminal response, cancel, safe shutdown, malformed-request behavior, and stderr/stdout separation.

- [x] Run:

```powershell
Set-Location 05_Software/integration_sdk/python
uv run pytest -q
uv run ruff check .
uv run ruff format --check .
Set-Location ../../nilo_assistant
go test ./internal/integration/...
```

Expected: all SDK, contract, manifest, protocol, and supervisor tests pass.

- [x] Record Task 13 evidence, then commit only Task 13 files with message `test: add fake integration contract package` for task review; do not push.

---

## Task 14: Orchestrate the complete operation lifecycle

**Files:**

- Create: `05_Software/nilo_assistant/internal/operation/service.go`
- Create: `05_Software/nilo_assistant/internal/operation/service_test.go`
- Create: `05_Software/nilo_assistant/internal/app/app.go`
- Create: `05_Software/nilo_assistant/internal/app/app_test.go`
- Modify: `05_Software/nilo_assistant/cmd/nilo/main.go`

**Consumes:** Validated submit/approve/deny/cancel commands and the policy, session, audit, artifact, manifest, and supervisor interfaces.

**Produces:** One coherent vertical slice from immutable request through terminal durable evidence and resource release.

- [x] Write failing service tests with fakes for these exact scenarios:

1. Ask creates `pending_approval` and does not invoke the runner.
2. Approval of the same request queues, leases, runs, persists evidence, and completes.
3. Auto-eligible under Auto runs without approval.
4. Full-only under Auto remains pending; under Full it runs.
5. Denied never creates a runnable session.
6. Cancellation sends cancel, records the actual terminal state, and releases leases.
7. Runner crash records `failed` when a typed failure is known, otherwise `incomplete`.
8. Artifact persistence failure prevents `completed`.
9. A second conflicting request returns typed busy and never starts its runner.

- [x] Run `go test ./internal/operation`; expect compile failure.

- [x] Implement the stable `operation.Service` contract declared at the top of this plan. Submit order is fixed:

```text
authenticate actor (already resolved at this layer)
-> validate manifest/operation/parameters/deadline
-> create immutable request and policy snapshot
-> resolve policy
-> append decision audit
-> create pending or queued session
-> if queued: acquire all leases atomically
-> transition running
-> execute supervised runner
-> validate and persist terminal result and required artifacts
-> append terminal audit durably
-> transition to the truthful terminal state
-> release leases
```

- [x] Ensure approval revalidates the exact request digest, expiry, current package compatibility, and device presence before queueing. It does not silently update parameters or reuse an older approval.

- [x] Ensure cancellation is idempotent. Cancelling a terminal session returns its existing state and cancelling running work invokes the supervisor. A request still in `pending_approval` is resolved with `Deny`, producing a denial record and audit event without dispatching to the runner; it is not forced through an invalid session transition.

- [x] Add `app.New` to open configuration/database/artifact services and invoke `ReconcileStartup` before accepting operations. `cmd/nilo` gains `serve-foundation-check --data-dir <temp>` only as a local smoke command; it opens, reconciles, reports JSON health, and exits without network listeners.

- [x] Run focused tests, `go test ./...`, Python tests/lint, and portable compilation.

- [x] Record Task 14 evidence, then commit only Task 14 files with message `feat: orchestrate operation lifecycle` for task review; do not push.

---

## Task 15: Prove the Increment 1 acceptance gate and document the handoff

**Files:**

- Create: `05_Software/nilo_assistant/internal/acceptance/increment1_e2e_test.go`
- Create: `05_Software/scripts/test-increment-1.ps1`
- Create: `05_Software/scripts/test-increment-1.sh`
- Create: `05_Software/INCREMENT_1_ACCEPTANCE.md`
- Modify: `05_Software/nilo_assistant/README.md`
- Modify: `05_Software/integration_sdk/python/README.md`
- Modify: `00_Planning/Remote_Bench_MVP_PRD.md` only if implementation reveals an approved requirement change

**Consumes:** A clean temporary data directory, the built Go core, the fake Python package, and all Increment 1 test suites.

**Produces:** Reproducible evidence for the PRD Increment 1 gate and a clean starting boundary for the sigrok package.

- [x] Write the failing black-box test before adding the harness. It must launch the real fake Python runner through the real supervisor and demonstrate:

  - new integration and device default to Ask;
  - immutable approval then successful `echo` with ordered progress;
  - Auto-mode `artifact` with verified bytes and digest;
  - running `sleep` cancellation with no active lease;
  - `crash` preserving diagnostic evidence and truthful terminal state;
  - Bench Core close/reopen converting a deliberately interrupted running session to `incomplete` and releasing only its leases;
  - full audit correlation by request/session/actor/integration/device.

- [x] Run the new package; expect failure until the black-box fixture and launch path are wired.

- [x] Implement both test scripts in this order:

```text
verify tool versions
sync the locked Python environment
run Python tests and Ruff checks
run Go focused/full tests
run Go race tests where supported
run the black-box fake integration test in a clean temporary directory
cross-compile Linux ARM64, Linux x86_64, and Windows x86_64
verify no test artifact or binary remains in the repository
```

- [ ] Run on Windows:

```powershell
05_Software\scripts\test-increment-1.ps1
git diff --check
git status --short
```

Run the POSIX equivalent on Linux ARM64 before declaring the increment release-ready:

```sh
./05_Software/scripts/test-increment-1.sh
git diff --check
git status --short
```

Expected: every check exits zero. Linux ARM64 execution evidence is mandatory for release readiness; Windows cross-compilation alone is not a substitute.

- [x] Write `INCREMENT_1_ACCEPTANCE.md` with prerequisites, exact commands, observed platform evidence, protocol/package authoring entry points, known later-platform adapter gaps, and an explicit statement that sigrok/MCP/GUI/WireGuard remain outside Increment 1.

- [x] Review against every Increment 1 PRD deliverable and gate. Record any unmet item honestly; do not relabel a partial vertical slice as accepted.

- [ ] Finish the AEGIS validate stage with the complete review, knowledge update, full-suite evidence, strict check, and refreshed stage report. Necessary reviewed task and fix commits are already authorized on the task branch; then enter AEGIS deliver and present the validated branch at its mandatory approval stop before any push, merge, PR publication, or release action.

## Final Review Checklist

- [x] Every operation parameter is schema-validated before the runner sees it.
- [x] Every policy matrix cell has a test, including `Denied` and `Always prompt` under Full Access.
- [x] Approval is bound to one immutable request digest.
- [x] A runner cannot write protocol messages through diagnostics accidentally.
- [x] stdout, stderr, manifests, schemas, events, logs, and artifacts are bounded.
- [x] Resource acquisition is atomic and conflicting work cannot race.
- [x] Success is reported only after required evidence and audit data are durable.
- [x] Cancellation, timeout, crash, malformed output, and restart have truthful terminal states and no stale proven-orphan lease.
- [x] The Python fake integration passes the same generic contract intended for sigrok.
- [x] Go tests pass and portable core builds for Linux ARM64, Linux x86_64, and Windows x86_64.
- [ ] The full test script runs natively on Raspberry Pi 4 Linux ARM64 before Increment 1 is called release-ready.
- [x] No sigrok-specific behavior leaked into Bench Core.
- [x] No MCP/HTTP, GUI, WireGuard, or appliance packaging scope was pulled into Increment 1.
- [x] No Git mutation occurred without explicit user authority.
