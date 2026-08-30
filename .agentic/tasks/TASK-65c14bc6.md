<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt in the .agentic folder for full license details.
SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

<!--
Task journal. One file per task, at .agentic/tasks/TASK-<id>.md, kept
after delivery as the task's historical record.

The header is the block of "key: value" lines directly under the title.
Keep it exactly in that form — one key per line, no blank lines inside
the block, no comments between the lines — so it stays machine-readable.

  stage         frame | specify | design | build | validate | deliver | done
                (adoption journals: survey | components | close | done)
  type          feature | fix | change | adopt
  path          full | quick
  requirements  comma-separated requirement IDs, empty until specify
  branch        the branch this task is delivered on
  base          the commit this task started from, filled in at creation;
                the delivery report and `check --task` measure the diff
                against it, so leave it alone once it is written. Empty
                when the repository had no history yet.
  opened        YYYY-MM-DD

Header lines are edited in place; log lines are only ever appended.
Keep each log line to one sentence.

Stage report is one replaceable latest snapshot, not an evidence store.
Use plain one-line fields with no fences or bullets inside that section.
stage is the reported stage; current must match the journal header after any
transition. result is in_progress | completed | paused | blocked | skipped.
Keep all observations and decisions in the append-only Log as well.
Leave unknown fields blank; summary prints not recorded, never an assumed pass.
Refresh after work or decisions even if current has not changed.
-->

# TASK-65c14bc6 — Increment 1 platform and integration foundation

stage: build
type: feature
path: full
requirements: REQ-FUNC-880eac8d, REQ-FUNC-1d02a85d, REQ-FUNC-b0dd990f, REQ-FUNC-39397f99, REQ-DATA-b2e24314, REQ-DATA-4357c6f2, REQ-DATA-f9b0b55d, REQ-DATA-9608cfd8, REQ-REL-32b81ca8, REQ-SEC-0ecda0e7, REQ-SEC-f9fd49ff, REQ-SEC-035a8f94, REQ-SEC-50599a7e, REQ-SEC-ae30d292, REQ-NFR-6b5de8e9, REQ-NFR-c4c70f53, REQ-NFR-1ad0cee7, REQ-PLAT-93e1b194, REQ-REL-31cc1e94, REQ-REL-e223f4b5, REQ-TEST-2bbda8a9, REQ-TEST-b52051b8
branch: ddp/increment-1-foundation
base: 67df8be9401082b6a8ba2e255c35ce58ec11272b
opened: 2026-08-30

## Done checklist (current stage)

- [ ] each automated requirement has a pre-implementation red record with exact selector or command, non-zero exit, and expected failure reason; each manual requirement has a post-implementation observation
- [ ] repository anchor recorded after red and evolution reviewed after implementation
- [ ] implementation makes those tests pass
- [ ] `test --impacted` green
- [ ] `affected_paths` and `linked_tests` updated on every requirement
- [ ] `status = "active"` on every requirement this task delivers - or `"retired"`, with the reason in its prose, where this task deliberately removed the behavior
- [ ] `check` clean, or each remaining warning explained in the log
- [ ] `stage: validate`

## Log

- 2026-08-30 frame: Scope is PRD Increment 1 only: portable Go Bench Core, configuration and SQLite persistence, authentication primitives, policy and approvals, sessions and leases, audit and artifacts, integration package contracts, supervised runner protocol, Python SDK, fake integration, contract tests, and the complete foundation acceptance slice.
- 2026-08-30 frame: Exclusions are sigrok and Hantek behavior, MCP or HTTP routes, browser GUI, WireGuard and other networking, appliance packaging, hardware-in-the-loop testing, and any commit or push without explicit deliver-stage approval.
- 2026-08-30 frame: The full path is required because Increment 1 introduces externally observable behavior, new data and wire formats, new dependencies, authentication and authorization controls, durable evidence, and security-sensitive process supervision.
- 2026-08-30 frame: AEGIS governs the lifecycle and journal while Superpowers subagent-driven development executes and reviews the fifteen implementation tasks during build.
- 2026-08-30 frame: Work is isolated at F:\NILO-worktrees\increment-1-foundation on branch ddp/increment-1-foundation, while the original master checkout and its untracked files remain untouched.
- 2026-08-30 frame: Proposed REQ-FUNC covers immutable operation lifecycle, approvals, cancellation, and terminal outcomes.
- 2026-08-30 frame: Proposed REQ-FUNC covers runtime-installable integration manifests, semantic operations, and the versioned runner contract.
- 2026-08-30 frame: Proposed REQ-SEC covers administrator credentials, revocable MCP token primitives, policy resolution, request-bound approvals, and Full Access boundaries.
- 2026-08-30 frame: Proposed REQ-DATA covers transactional configuration, requests, sessions, leases, audit history, artifact integrity, and restart reconciliation.
- 2026-08-30 frame: Proposed REQ-REL covers runner isolation, bounded diagnostics, timeouts, cancellation, safe shutdown, crashes, and orphan-lease recovery.
- 2026-08-30 frame: Proposed REQ-PLAT covers Linux ARM64 as release-blocking plus portable compilation for Linux x86_64 and Windows x86_64.
- 2026-08-30 frame: Proposed REQ-TEST covers the generic contract kit, deterministic fake Python integration, TDD cycles, cross-target checks, and Increment 1 black-box acceptance.
- 2026-08-30 frame: Proposed REQ-NFR covers schema validation, message and artifact bounds, path confinement, language-neutral extension boundaries, and no arbitrary command execution.
- 2026-08-30 frame: No product test baseline exists yet and Go is not installed on this host, so the Go 1.26 or newer prerequisite must be resolved before Superpowers Task 1 can execute.
- 2026-08-30 frame: Git authority currently covers the isolated worktree and task branch only; staging, committing, pushing, merging, and publishing remain unauthorized.
- 2026-08-30 frame: The transferred PRD and implementation plan match their source content after line-ending normalization, while byte hashes differ because the isolated copies use LF rather than CRLF.
- 2026-08-30 frame: DEVIATION `aegis check --task TASK-65c14bc6` reported repo-evolution note `no anchor recorded for task`, which is expected before the build-stage red test and anchor step but remains recorded for later resolution.
- 2026-08-30 frame: User approved the recorded scope, exclusions, full path, and proposed requirement areas with `continue`, so the task advanced to specify.
- 2026-08-30 specify: Created twenty-two draft requirements so every foundation behavior has one independently verifiable contract across functional, data, security, reliability, platform, non-functional, and test categories.
- 2026-08-30 specify: Added focused specifications for the immutable operation lifecycle, runtime package manifest, integration wire protocol, and permission matrix because those formats and algorithms were not independently buildable from one-line requirements.
- 2026-08-30 specify: DEVIATION AEGIS check reported sixty stale-path warnings because the draft requirements intentionally name files and directories that build will create, so the paths remain exact and the warnings are accepted until those files exist.
- 2026-08-30 specify: Structural verification found twenty-two unique draft requirements with exactly one shall clause each, four reachable specification files, no template placeholders, and no whitespace errors.
- 2026-08-30 design: No component knowledge pages exist yet, so design owes no prospective architecture-diagram update and validate will create knowledge from the implemented shape.
- 2026-08-30 design: DEVIATION the first design-ID coverage command included the requirement-template example REQ-FUNC-00000000 and exited non-zero, so the command was corrected to read actual id fields and then verified all twenty-two IDs.
- 2026-08-30 design: The thirteen-line design cites all twenty-two actual requirements, names every test seam, covers interface and storage decisions, records the rejected architecture, and owes no knowledge-page diagram because none exists yet.
- 2026-08-30 design: DEVIATION AEGIS check retained only the sixty previously accepted future-path warnings and the expected missing pre-build anchor note, with no new warning class or format error.
- 2026-08-30 build: User authorized commits when necessary, so task-local staging and commits on ddp/increment-1-foundation are permitted for Superpowers review ranges while push, merge, PR publication, and release remain unauthorized.
- 2026-08-30 build: DEVIATION necessary governance and per-task commits will occur before AEGIS deliver because the selected Superpowers workflow requires durable task review ranges and the user explicitly authorized them.
- 2026-08-30 build: DEVIATION AEGIS reports scope drift because the governance baseline contains only planning and AEGIS artifacts before Task 1 creates linked product files, so the warning is accepted for the baseline and must be re-evaluated after Task 1 begins.
- 2026-08-30 build: The planning, requirement, specification, and journal baseline will be committed on the isolated task branch to establish the first durable Superpowers review boundary.
- 2026-08-30 build: DEVIATION the first baseline requirement-count command used custom PowerShell objects that constrained language rejected while still returning zero, so its apparent result is discarded and replaced by a constrained-compatible check.
- 2026-08-30 build: DEVIATION the first placeholder scan exited non-zero because it matched documented template examples and the journal's historical example rather than unresolved content, so a value-focused scan replaces it.
- 2026-08-30 build: DEVIATION the replacement requirement check exited non-zero because its word-boundary regex was over-escaped, so the result is discarded and a direct case-insensitive token count replaces it.
- 2026-08-30 build: DEVIATION the replacement placeholder scan exited non-zero because PowerShell removed quoting inside the regex and made it invalid, so a single-quoted expression replaces it.
- 2026-08-30 build: DEVIATION the first compact AEGIS JSON parser inspected nonexistent top-level finding arrays and falsely printed zero findings, so its evidence is discarded and the actual JSON structure will be inspected before summarizing it.
- 2026-08-30 build: DEVIATION the first portable Go setup command exited non-zero after verifying the official archive because constrained PowerShell blocked Expand-Archive, so native tar extraction replaces it without changing system configuration.
- 2026-08-30 build: DEVIATION the native extraction run emitted a constrained-mode path-canonicalization error while still extracting successfully and returning Go 1.27.0, so a clean Resolve-Path and version check replaces the mixed evidence.
- 2026-08-30 build: The clean prerequisite check confirmed Go 1.27.0 windows/amd64, Python 3.12.10, and uv 0.11.25 with no system installation or persistent PATH change.
- 2026-08-30 build: DEVIATION the first read-only Superpowers reviewer-template lookup exited non-zero because the assumed filename did not exist, so the installed skill directory is searched for the authoritative template before review.
- 2026-08-30 build: Task 1 RED command `go test ./internal/buildinfo` from `05_Software/nilo_assistant` exited 1 because `Current` was undefined, exactly proving the missing build-information contract before implementation.
- 2026-08-30 build: Task 1 RED command `python -m unittest discover -s scripts/tests -p test_aegis_test.py -v` from `05_Software` exited 1 because `scripts/aegis_test.py` was absent, exactly proving the missing polyglot suite-discovery router before implementation.
- 2026-08-30 build: REQ-PLAT-93e1b194 RED command `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/nilo` from `05_Software/nilo_assistant` exited 1 because `cmd/nilo` did not exist, exactly proving the portable target was absent before implementation.
- 2026-08-30 build: AEGIS anchor `2997e73e9a5a363a` captured 21 files at governance baseline commit `421438da83649d3ca1dfc5895d373f6666a635d7` after the Task 1 red tests and before implementation.
- 2026-08-30 build: DEVIATION the first Task 1 Python GREEN attempt exited 1 because the test called nonexistent `importlib.util.module_from_specification`, so the test-only typo was corrected to `module_from_spec` and the same focused command then passed.
- 2026-08-30 build: DEVIATION the first PowerShell portable-core verification exited 1 because constrained mode blocked `System.IO.Path.GetTempPath`, so the script switched to `New-TemporaryFile` and the complete host plus three-target check then passed.
- 2026-08-30 build: Task 1 final verification passed the focused Go and Python tests, `go test ./...`, AEGIS full, impacted, and lint routes, CLI version output, PowerShell host build, CGO-disabled linux/arm64, linux/amd64, and windows/amd64 builds, Bash syntax, and repository artifact absence.
- 2026-08-30 build: Task 1 implementation commit `13fb80857fc58be498443af2682584558f4b4371` contains only the thirteen planned implementation and documentation files and was not pushed.
- 2026-08-30 build: DEVIATION the host policy rejected removal of two verified in-worktree Python cache directories before execution, so they remain generated, untracked, and excluded from commits and review ranges.
- 2026-08-30 test: test --impacted (2 test ref(s)) exit 0 (0.8s)
- 2026-08-30 build: AEGIS evolution for anchor `2997e73e9a5a363a` reported 5 added and 5 modified expected Task 1 paths with no removals or unexpected architecture change.
- 2026-08-30 build: The first post-Task 1 check exposed five outside-layout warnings for the new scripts and software README, so `.agentic/aegis.toml` now classifies scripts as test infrastructure and the README as documentation in commit `056f3295ae01f632c9cd0db6b23cdc93ce2e5bb0`.
- 2026-08-30 build: DEVIATION the corrected AEGIS check reports 58 stale-path warnings only because later Increment 1 tasks have not yet created their declared paths, so these known warnings remain accepted during staged implementation.
- 2026-08-30 build: Independent Task 1 review approved specification compliance and code quality with no critical or important findings and one minor missing CLI regression test.
- 2026-08-30 build: DEVIATION the reviewer-requested CLI regression test could not demonstrate a pre-implementation red because the required behavior already existed, so it was added as post-implementation coverage without changing production behavior.
- 2026-08-30 build: Commit `9d64ab1becc3e67bf8560398c986a7c6ea989f81` adds a portable subprocess test for exact version JSON and invalid-invocation exit code 2, with focused, full, and cross-target checks passing.
- 2026-08-30 build: The same reviewer re-reviewed the fix-only range and found the sole finding addressed with no new breakage or out-of-scope observation.
- 2026-08-30 build: Final Task 1 evolution reports 6 added and 5 modified expected paths with no removals, and the final impacted run passes the router, CLI, and build-information suites.
- 2026-08-30 build: Task 2 RED command `go test ./internal/config` exited 1 because `Load`, `Inputs`, `Config`, `ErrDataDirNotAbsolute`, and `ResolveWithin` were undefined, proving the REQ-NFR-6b5de8e9 configuration-validation and REQ-NFR-c4c70f53 path-confinement slices were absent before implementation.
- 2026-08-30 build: Task 2 commit `d05ddc88434817f61685a2ece8cd4ad77b8486d6` adds the six planned configuration and platform-path files, and focused, full, portable-core, plus direct linux/arm64, linux/amd64, and windows/amd64 config test-package compilations passed.
- 2026-08-30 build: DEVIATION the combined Task 2 cross-compile cleanup command was rejected by host policy, so three known test binaries remain only in the system temporary directory and no repository artifact was created or committed.
- 2026-08-30 build: Controller self-review found a forbidden string-prefix containment check and a prefix-confusion fixture that exercised only absolute-path rejection, so commit `af2747728a7bb6595c3b723947b7f033446acf2d` replaced the check with a leading relative-path component decision and made the fixture a sibling escape.
- 2026-08-30 build: Task 2 links its configuration and path tests to REQ-NFR-6b5de8e9 and REQ-NFR-c4c70f53 while both requirements remain draft because later schema, manifest, supervisor, and artifact boundary slices are not yet implemented.
- 2026-08-30 build: Task 2 impacted routing passed all seven linked references and the complete discovered Python and Go suites.
- 2026-08-30 build: AEGIS evolution reports 12 added and 5 modified expected cumulative paths with no removals after Task 2.
- 2026-08-30 build: DEVIATION the post-Task 2 AEGIS check reports 55 stale-path warnings only for later Increment 1 paths, reduced from 58 with no error, note, or new warning class, so the known staged-build warnings remain accepted.
- 2026-08-30 build: Independent Task 2 review approved specification compliance and code quality with no critical, important, or minor finding.
- 2026-08-30 build: Controller confirmed REQ-NFR-6b5de8e9 and REQ-NFR-c4c70f53 remain draft while retaining all five Task 2 linked test selectors, exactly matching the staged-delivery ruling.
- 2026-08-30 build: DEVIATION the first Task 3 red test contained an inconsistent oversized-fixture variable name, so the test-only typo was corrected before evidence was accepted and no production implementation had begun.
- 2026-08-30 build: Task 3 RED command `go test ./internal/domain` exited 1 solely because the session state, transition, typed identity, and operation-request API were undefined, proving the REQ-FUNC-880eac8d domain slice and REQ-NFR-6b5de8e9 request-validation slice were absent before implementation.
- 2026-08-30 build: DEVIATION Task 3 is the first task to import the Go dependencies declared in Task 1, so the approved plan now explicitly adds the required `go.sum` checksum file that its original file list omitted.
- 2026-08-30 build: Task 3 commit `e4158040beeff9cb8d13e09ed26ded8b899141e8` adds the checksum file and four planned domain files, and focused, full, portable-core, plus direct linux/arm64, linux/amd64, and windows/amd64 domain test-package compilations passed.
- 2026-08-30 build: DEVIATION the Task 3 compile-only cleanup wrapper was rejected by host policy, so three known domain test binaries remain only in the system temporary directory and no repository artifact was created or committed.
- 2026-08-30 build: Task 3 links its identity, transition, stable-operation-name, immutable-byte-copy, and invalid-request tests to REQ-FUNC-880eac8d and REQ-NFR-6b5de8e9 while both requirements remain draft for later orchestration and parser-boundary slices.
- 2026-08-30 build: Task 3 impacted routing passed all thirteen linked references and the complete discovered Python and Go suites.
- 2026-08-30 build: AEGIS evolution reports 17 added and 6 modified expected cumulative paths with no removals after Task 3, including the deliberate implementation-plan checksum-file correction.
- 2026-08-30 build: DEVIATION the post-Task 3 AEGIS check reports 54 stale-path warnings only for later Increment 1 paths, reduced from 55 with no error, note, or new warning class, so the known staged-build warnings remain accepted.
- 2026-08-30 build: DEVIATION the first read-only Task 3 plan check exited non-zero because PowerShell interpreted Markdown backticks in the search expression, so a literal fixed-string check replaces it before staging.
- 2026-08-30 build: Independent Task 3 review found one important plan-mandated immutability conflict because exported request fields allowed post-validation mutation, while all identity, validation, state, test, and portability work otherwise matched the task.
- 2026-08-30 build: DEVIATION the Task 3 exported-field sketch contradicted the approved immutable-request invariant, so the plan and REQ-FUNC-880eac8d specification now require private authoritative fields, value getters, and a defensive copy from every canonical-parameter accessor.
- 2026-08-30 build: DEVIATION the first controller inspection of the Task 3 immutability red used the module directory for a repository-relative report path and emitted an unrelated missing-path error, so that mixed output is discarded and a focused test-only rerun replaces it.
- 2026-08-30 build: Task 3 immutability-fix RED command `go test ./internal/domain` exited 1 because the newly specified value getters and defensive canonical-parameter accessor did not exist while public fields still did, exactly proving the reviewer finding before the fix.
- 2026-08-30 build: Task 3 fix commit `44066472f127f3c5200fdba5fb0f561295cb9e23` makes every accepted-request field private, exposes value getters, returns a fresh canonical-parameter copy per call, and passes focused, full, portable-core, plus direct three-target domain compilation.
- 2026-08-30 build: DEVIATION the fix-round compile-only artifacts use three additional system-temporary filenames after cleanup remained policy-blocked, so nine Task 2/3 test binaries now remain outside the repository and every commit.
- 2026-08-30 build: The immutable-request fix impacted run passed all fourteen linked references and the complete discovered Python and Go suites.
- 2026-08-30 build: Refreshed AEGIS evolution retains 17 added and 6 modified expected cumulative paths with no removals, and check retains only the 54 previously accepted future-path warnings.
- 2026-08-30 build: The same Task 3 reviewer found the immutable-request finding addressed by private state, value getters, defensive-copy access, and mutation-isolation tests with no new breakage or out-of-scope observation.
- 2026-08-30 build: DEVIATION Task 4 is the first task to import the declared modernc SQLite driver, so the implementation plan now explicitly includes the required `go.sum` checksum modification that its original file list omitted.
- 2026-08-30 build: Task 4 RED command `CGO_ENABLED=0 go test ./internal/store/sqlite` exited 1 solely because `Open` and `DB` were undefined, proving the transactional schema, exact PRAGMAs, migration integrity, foreign keys, and active-lease uniqueness foundation was absent before implementation.
- 2026-08-30 build: DEVIATION the first Task 4 GREEN attempt exited 1 before tests because Go 1.27 required module-graph updates that the then-current brief forbade, so implementation paused without a commit for root-cause analysis.
- 2026-08-30 build: Read-only `go mod tidy -diff` exited 1 with the exact unapplied delta while identical before/after `go.mod` hashes proved no mutation; it identified eight SQLite runtime indirect requirements and showed that tidying would wrongly remove two planned future direct dependencies.
- 2026-08-30 build: DEVIATION Task 4 now explicitly permits the necessary `go.mod` indirect additions while preserving the four Task 1 direct requirements, correcting the plan-level dependency conflict without authorizing a blind tidy.
- 2026-08-30 build: DEVIATION an independent Task 4 inspection used a repository-relative test-file path from the module directory and produced a non-terminating `Get-Content` path error; the verification commands in the same shell passed and the file inspection was immediately repeated from its owning directory.
- 2026-08-30 build: Task 4 commit `c259a669462e546aaec609660a4022ee69e37589` adds the pure-Go single-connection SQLite store, exact durable PRAGMAs, `BEGIN IMMEDIATE` digest-verified migration execution, all thirteen foundation tables, foreign keys, and the partial unique active-resource lease index.
- 2026-08-30 build: Independent focused and full Go suites plus portable-core verification passed with `CGO_ENABLED=0`; the implementer also compiled the SQLite test package for linux/arm64, linux/amd64, and windows/amd64 without creating repository artifacts.
- 2026-08-30 build: REQ-DATA-b2e24314 and REQ-DATA-4357c6f2 now link the Task 4 persistence-foundation and active-lease conflict tests but remain draft until the later repository behaviors complete their acceptance clauses.
- 2026-08-30 build: Task 4 impacted routing passed all seventeen linked references and the complete discovered Python and Go suites.
- 2026-08-30 build: AEGIS evolution reports 21 added and 7 modified expected cumulative paths with no removals after Task 4.
- 2026-08-30 build: DEVIATION the post-Task 4 AEGIS check reports 53 stale-path warnings only for later Increment 1 paths, reduced from 54 with no error, note, or new warning class, so the known staged-build warnings remain accepted.
- 2026-08-30 build: DEVIATION the first Task 4 reviewer-template read exited 1 because it used the obsolete `code-reviewer.md` filename; repository search immediately resolved the installed template as `task-reviewer-prompt.md` before reviewer dispatch.
- 2026-08-30 build: DEVIATION the first review-package command passed the task number as an obsolete positional argument and exited 1 with `bad BASE: 4`; its usage output confirmed the current three-argument contract before regeneration.
- 2026-08-30 build: The independent Task 4 reviewer found one Important test gap: the table test proved every required table existed but did not reject extra non-internal tables, so the explicitly bound exact-set claim remains open pending a focused fix and scoped re-review.
- 2026-08-30 build: Task 4 fix round 1 commit `bf5348c80284cea18c0015b626b1da729d4f6850` replaced the subset assertion with a complete ordered table query; focused, full, and portable-core checks passed with CGO disabled.
- 2026-08-30 build: The scoped re-review kept the finding open because SQL `LIKE 'sqlite_%'` treats `_` as a wildcard and could hide a valid non-internal table such as `sqliteXunexpected`; fix round 2 must use a literal `sqlite_` prefix exclusion and prove the distinction.
- 2026-08-30 build: Task 4 fix round 2 commit `38b9e95` adds the discriminating `sqliteXunexpected` fixture and changes the helper to literal-prefix `NOT GLOB 'sqlite_*'`; focused, full, and portable-core checks passed with CGO disabled.
- 2026-08-30 build: The same reviewer found the exact-table-set finding addressed and reported no new Critical, Important, Minor, or out-of-scope issue in the fix diff.
- 2026-08-30 build: Final controller focused, full, portable-core, and seventeen-reference AEGIS impacted checks passed; refreshed evolution remains 21 added and 7 modified expected paths with no removals, and AEGIS check remains errors=0 warnings=53 known future paths notes=0.
- 2026-08-30 build: Task 5 RED command `CGO_ENABLED=0 go test ./internal/auth ./internal/store/sqlite` exited 1 because the password, token, and repository production APIs were absent; a second test-first API-boundary red exited 1 because persistence still accepted `CreatedToken` containing plaintext.
- 2026-08-30 build: Task 5 commit `aeecc76` adds Argon2id PHC password verifiers, bounded password validation, UUID-v7 one-time MCP tokens, SHA-256 verifier-only persistence, constant-time candidate comparison, typed token outcomes, and durable SQLite revocation.
- 2026-08-30 build: Independent focused and full Go suites plus portable-core verification passed with `CGO_ENABLED=0`; implementer compile checks also passed for both affected packages on linux/arm64, linux/amd64, and windows/amd64.
- 2026-08-30 build: DEVIATION the implementer's first temporary-output cross-target compilation attempt was rejected by host command policy before execution, so it used Go's no-op executor to compile both test packages for all three targets without running foreign binaries or creating repository artifacts.
- 2026-08-30 build: REQ-SEC-0ecda0e7 and REQ-SEC-f9fd49ff are active with Task 5 linked tests because their complete password-verifier and token create/authenticate/reload/revoke behaviors are implemented and passing; network routes remain outside these requirements and deferred to Increment 3.
- 2026-08-30 build: Task 5 impacted routing passed all twenty-seven linked references and the complete discovered Python and Go suites.
- 2026-08-30 build: AEGIS evolution reports 28 added and 7 modified expected cumulative paths with no removals after Task 5.
- 2026-08-30 build: DEVIATION the post-Task 5 AEGIS check reports 47 stale-path warnings only for later Increment 1 paths plus one `secret-in-diff` warning for the deterministic public password phrase in `password_test.go`; the fixture is not a credential or secret, so both warning classes are accepted with errors=0 and notes=0.
- 2026-08-30 build: The independent Task 5 reviewer found one accepted Important product defect: PHC parsing can split and decode attacker-sized input before rejecting its decoded length, so total and component encoded sizes require pre-allocation bounds and regression tests.
- 2026-08-30 build: Ruling: Task 5's two requirement files, task journal, and evolution snapshot are controller-owned AEGIS evidence explicitly required by the combined workflow, not an implementer violation of the seven-file product boundary; they remain as separate governance commits.
- 2026-08-30 build: Task 5 reviewer minors for early malformed-token rejection and symmetric VerifyPassword buffer-immutability coverage are deferred to the final branch review ledger because current production behavior is correct and neither blocks the Task 5 contract.
- 2026-08-30 build: Task 5 fix commit `627085a` bounds the complete PHC shape before splitting and exact salt/hash encoded sizes before decoding; focused, full, and portable-core checks passed with CGO disabled.
- 2026-08-30 build: The same reviewer found the unbounded-parser defect addressed with no new Critical, Important, or out-of-scope issue in the fix diff.
- 2026-08-30 build: Final controller focused, full, portable-core, and AEGIS impacted checks passed; refreshed evolution remains 28 added and 7 modified paths with no removals and check remains errors=0 warnings=48 accepted staged-path/test-fixture warnings notes=0.
- 2026-08-30 build: DEVIATION the Task 6 preflight attempted to read a nonexistent optional `REQ-SEC-50599a7e.md` spec and exited 1 for that file while successfully reading the authoritative requirement and existing policy spec; repository search confirmed no missing referenced spec contract.
- 2026-08-30 build: Task 6 RED `CGO_ENABLED=0 go test ./internal/policy` exited 1 with only expected missing policy enums and resolver symbols after the approved matrix test was added.
- 2026-08-30 build: DEVIATION Task 7 brief required `domain.Artifact` while the existing domain defined only `ArtifactID`; before product edits, scope was corrected to add the missing domain value, preserving the specified Store interface rather than weakening it to an artifact-package type.
- 2026-08-30 build: DEVIATION Task 8 brief requires `domain.Session` while the existing domain currently defines only `SessionState`; before product edits, scope is corrected to modify `internal/domain/session.go` with the validated session value rather than introducing a competing session-package type.
- 2026-08-30 build: Task 8 commits `2a722be` and `af0e2e0` add sessions, exclusive leases, atomic terminal lease release, and restart reconciliation. Initial review found three Important state/lease/timestamp/UUID invariant defects; the focused fix and re-review cleared all three with no new issue.
- 2026-08-30 build: DEVIATION Task 9 imports the already-pinned JSON Schema library but normal Go commands require its missing module-graph metadata; scope now includes the mechanically generated `go.mod` and `go.sum` update because accepting a disposable-copy-only verification would leave the repository non-buildable by its declared command.
- 2026-08-30 build: DEVIATION Task 9 review found that source-tree-relative schemas cannot support relocated or `-trimpath` binaries; scope now packages embedded schema copies under the manifest package and adds a focused hardening round for physical-path confinement, compiler URI loading, timeout bounds, integrity length, and normalized-claim duplicates.
- 2026-08-30 build: Task 9 commits `c6b6814`, `9b1599f`, and `434cef6` provide strict runtime package manifests, bounded physically confined schema loading, embedded portable schemas, and canonical package-root handling. Two review rounds cleared seven Important confinement, portability, and validation findings with no new issue.
- 2026-08-30 build: REQ-NFR-6b5de8e9 and REQ-NFR-c4c70f53 retain Task 9 manifest-boundary links but remain draft until later protocol, SDK, supervisor, and artifact slices complete their full acceptance clauses.
- 2026-08-30 build: Task 10 commits `68ef057`, `6ef2c98`, and `a3ae7f6` add the bounded strict wire protocol. Initial review found lifecycle correlation, typed payload, outbound-bound, canonical-key, and concurrency/fixture gaps; the hardening range is independently approved. REQ-FUNC-b0dd990f remains draft pending the Task 11 Python implementation, but now links the Go protocol regressions.
- 2026-08-30 build: REQ-DATA-4357c6f2 is active with complete lease tests. REQ-REL-32b81ca8 keeps Task 8 reconciliation tests but remains draft until a later application-composition task invokes reconciliation during actual Bench Core startup.
- 2026-08-30 build: Controller focused, full, vet, portable-core, and 54-reference AEGIS impacted checks passed with CGO disabled; evolution is 49 added and 7 modified with no removals; check errors=0 warnings=28 (27 future paths plus deterministic password-fixture scanner) notes=0.
- 2026-08-30 build: DEVIATION Task 11's first local checkpoint included generated SDK bytecode because no SDK-local ignore rule existed; the unpushed commit was immediately amended to exclude bytecode and add the scoped ignore rule.
- 2026-08-30 build: Task 11 commits `e248757`, `019b3c5`, `70bca6f`, `9e83177`, and `bae5229` provide the pinned Python SDK, strict protocol models, async lifecycle interface, shared-fixture validation, fail-closed NDJSON server, and 31 passing SDK regressions; four independent review rounds closed lifecycle, frame-bound, strict-JSON, fatal-output, cancellation-cleanup, and hello-shape gaps with no remaining finding.
- 2026-08-30 build: REQ-FUNC-b0dd990f and REQ-FUNC-39397f99 are active with linked Go and Python protocol/SDK tests; unmanaged plugin child tasks are an explicit authoring constraint until Task 13 supplies the process entry-point enforcement boundary.
- 2026-08-30 build: DEVIATION the first Task 11 AEGIS impacted run bypassed the repository-local Go wrapper and then exposed that the generic Python router used host `unittest` without the SDK's pinned dependencies; the same route was rerun with the wrapper, the router was corrected test-first to use `uv run pytest -q` for UV-managed projects, and the final 70-reference impacted run passed.
- 2026-08-30 build: Task 11 router correction commit `287f9a4` is independently approved and preserves non-UV suite routing while returning bounded status 127 diagnostics for a missing runner.
- 2026-08-31 build: Task 12 commits `467fd9b`, `9dce0ee`, `793fb27`, `068f6d7`, and `ea769b0` provide the isolated runner supervisor and reviewed hardening for protocol identity, bounded controls, runner admission, shutdown cleanup, diagnostics, and typed failures; REQ-NFR-1ad0cee7 is active while REQ-REL-31cc1e94 remains draft pending operation-layer lease release and native Linux runtime acceptance.
- 2026-08-31 build: DEVIATION focused Go race verification remains unavailable because CGO is disabled and forcing CGO requires absent gcc; native Linux process-group runtime evidence remains an explicit Raspberry Pi acceptance task, while focused/full/vet/portable builds passed.
- 2026-08-30 build: Task 7 independent review found five Important artifact-store defects: destination overwrite race, missing post-rename directory durability, pathname-only cleanup ownership, lexical root disjointness, and Open handle race. The plan now adds Unix/Windows filesystem primitives and a focused fix round.
- 2026-08-30 build: Ruling: portable Unix/Windows APIs lack atomic pathname compare-and-delete, so Task 7 post-placement metadata failure preserves a verified unregistered artifact for later reconciliation rather than risking deletion of a concurrent replacement. Cost if wrong: an orphan consumes artifact-root space until reconciliation, but unrelated evidence is never destructively removed.
- 2026-08-30 build: Task 7 implementation commits `354214b`, `ecc4513`, and `db47219` provide append-only audit persistence plus bounded, digest-verified artifact registration with atomic no-replace placement, directory durability where supported, physical-root confinement, and opened-handle identity validation.
- 2026-08-30 build: Task 7 independent review initially found five Important artifact-store defects; fix round one addressed four and fix round two removed the remaining cleanup ownership race. The final verdict approves `db47219` with no new Critical, Important, Minor, or out-of-scope issue.
- 2026-08-30 build: DEVIATION the physical-root alias regression is skipped on this Windows host when directory-symlink privilege is unavailable; Unix and Windows implementations compile, but that Windows alias behavior remains runtime-unverified on this host.
- 2026-08-30 build: REQ-DATA-f9b0b55d and REQ-DATA-9608cfd8 are active with complete Task 7 linked tests. Controller focused, full, portable-core, and 49-reference AEGIS impacted checks passed with CGO disabled; evolution is 44 added and 7 modified with no removals; check has errors=0 warnings=34 (33 planned later-path warnings and one deterministic password-fixture scanner warning) and notes=0.
- 2026-08-30 build: DEVIATION Task 6 exposed that the Task 4 foundation schema omitted a dedicated `policy_snapshots` table even though operation requests carry policy snapshot IDs; persistence paused before misusing `config_versions`, and the plan now adds digest-verified migration 002 plus the exact table-set update.
- 2026-08-30 build: Task 6 independent review found four Important gaps: numeric canonicalization preserved equivalent spelling differences, caller-controlled request IDs broke the changed-request identity guarantee, the migration accepted malformed `Z` timestamps, and model/schema operation-name limits differed. The plan is expanded with domain creation/rehydration correction; the implementer resumes for one focused fix round.
- 2026-08-30 build: Task 6 fix `0456a8c` rejects noncanonical equivalent number spellings, allocates request IDs only at public creation with separate rehydration, enforces canonical policy-snapshot UTC timestamps, and aligns the operation-name boundary at 128 bytes; focused, full, portable, cross-target, and vet checks passed.
- 2026-08-30 build: The scoped Task 6 re-review found all four Important findings addressed with no new Critical, Important, or out-of-scope issue.
- 2026-08-30 build: REQ-SEC-035a8f94, REQ-SEC-50599a7e, and REQ-SEC-ae30d292 are active with complete Task 6 linked tests; 38-reference AEGIS impacted passed with CGO disabled.
- 2026-08-30 build: AEGIS evolution reports 35 added and 7 modified expected cumulative paths with no removals after Task 6; check has errors=0 warnings=42 (41 planned later-path warnings plus the known deterministic password-fixture scanner warning) and notes=0.
- 2026-08-30 test: test --impacted (2 test ref(s)) exit 0 (0.7s)
- 2026-08-30 test: test --impacted (7 test ref(s)) exit 0 (0.7s)
- 2026-08-30 test: test --impacted (13 test ref(s)) exit 0 (0.8s)
- 2026-08-30 test: test --impacted (14 test ref(s)) exit 0 (0.8s)
- 2026-08-30 test: test --impacted (17 test ref(s)) exit 0 (0.9s)
- 2026-08-30 test: test --impacted (17 test ref(s)) exit 0 (1.0s)
- 2026-08-30 test: test --impacted (27 test ref(s)) exit 0 (1.3s)
- 2026-08-30 test: test --impacted (27 test ref(s)) exit 0 (1.4s)
- 2026-08-30 test: test --impacted (29 test ref(s)) exit 0 (1.2s)
- 2026-08-30 test: test --impacted (38 test ref(s)) exit 0 (0.6s)
- 2026-08-30 test: test --impacted (49 test ref(s)) exit 0 (0.9s)
- 2026-08-30 test: test --impacted (54 test ref(s)) exit 0 (0.8s)

## Design

- Components: a Go command and composition root own config, domain, SQLite repositories, auth, policy, sessions, audit, artifacts, operation orchestration, manifest validation, protocol, and supervision; a separate Python SDK and fake package exercise the language-neutral boundary (REQ-FUNC-880eac8d, REQ-FUNC-1d02a85d, REQ-FUNC-39397f99).
- Interface: `operation.Service` exposes Submit, Approve, Deny, Cancel, and Get; `policy.Resolver` returns Allow, Prompt, or Deny; `supervisor.Supervisor` exposes Describe, Execute, Cancel, and Shutdown; NDJSON `nilo.integration/1` carries typed lifecycle messages (REQ-FUNC-880eac8d, REQ-SEC-035a8f94, REQ-FUNC-b0dd990f).
- Data: immutable canonical requests, configuration and policy snapshots, sessions, active leases, approvals, audit events, token verifiers, and artifact metadata live in transactional SQLite; artifact bytes live below one configured root and are registered only after digest and durability checks (REQ-DATA-b2e24314, REQ-DATA-4357c6f2, REQ-DATA-f9b0b55d, REQ-DATA-9608cfd8, REQ-SEC-50599a7e).
- Trust boundary: manifests, schemas, paths, runner messages, and diagnostics are untrusted and bounded; runners use executable-plus-argument arrays in external processes with platform adapters (REQ-NFR-6b5de8e9, REQ-NFR-c4c70f53, REQ-NFR-1ad0cee7).
- Platform: portable packages avoid CGO and OS-specific behavior is isolated behind build-tagged path and process files for Linux ARM64, Linux x86_64, and Windows x86_64 (REQ-PLAT-93e1b194).
- Rejected alternative: a Python core with in-process or sigrok-specific plugins was rejected because it weakens fault isolation, couples the extension ABI, and makes later hardware families core changes (REQ-FUNC-1d02a85d, REQ-NFR-1ad0cee7).
- Test seams: domain transition and orchestration fakes cover immutable lifecycle and truthful completion (REQ-FUNC-880eac8d, REQ-REL-e223f4b5).
- Test seams: temporary SQLite reopen and concurrent repository tests cover snapshots, leases, audit, artifacts, and restart reconciliation (REQ-DATA-b2e24314, REQ-DATA-4357c6f2, REQ-DATA-f9b0b55d, REQ-DATA-9608cfd8, REQ-REL-32b81ca8).
- Test seams: credential, token, table-driven policy, approval-digest, and new-identity tests cover the security model (REQ-SEC-0ecda0e7, REQ-SEC-f9fd49ff, REQ-SEC-035a8f94, REQ-SEC-50599a7e, REQ-SEC-ae30d292).
- Test seams: manifest fixtures, shared codec goldens, Python in-memory streams, and malicious boundary fixtures cover packages, protocol, SDK, bounds, and confinement (REQ-FUNC-1d02a85d, REQ-FUNC-b0dd990f, REQ-FUNC-39397f99, REQ-NFR-6b5de8e9, REQ-NFR-c4c70f53).
- Test seams: helper processes cover crash isolation, cancellation, safe shutdown, timeouts, and bounded stderr (REQ-NFR-1ad0cee7, REQ-REL-31cc1e94).
- Test seams: cross-target scripts, the common package contract, and the real fake-runner black-box slice cover platform and Increment 1 gates (REQ-PLAT-93e1b194, REQ-TEST-2bbda8a9, REQ-TEST-b52051b8).
- Expected files: `05_Software/nilo_assistant/{cmd,internal}`, `05_Software/schemas/integration/v1`, `05_Software/integration_sdk/python`, `05_Software/integrations/fake_python`, `05_Software/tests/contract`, and `05_Software/scripts` as enumerated task-by-task in the approved implementation plan.

## Stage report

stage: build
result: in_progress
current: build
changed: Tasks 1 through 11 are review-approved; Task 11 adds a pinned Python integration SDK with strict NDJSON protocol validation and supervised-lifecycle-safe server behavior
evidence: Task 11 RED recorded missing Python models/server, then 31 SDK tests, Ruff check/format, focused Go protocol tests, portable core host plus linux/arm64, linux/amd64, and windows/amd64 builds, and the final 70-reference AEGIS impacted route through the repository-local Go wrapper passed; review cleared all protocol lifecycle and stream-integrity findings
issues: two generated Python cache directories and twelve compile-only Task 2/3/4 binaries remain outside commits after policy-blocked cleanup; later requirement slices and affected paths remain pending; push, merge, PR publication, and release remain unauthorized
next: generate the Task 12 brief for external integration supervision
needs_user: no - approved implementation continues continuously in build and stops only for a named safety or authority boundary
