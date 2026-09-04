# Claude Code entry prompt: compact field interface

Copy the following prompt into Claude Code opened in `F:\2026_Networked_Intelligent_Lab_Orchestrator`:

```text
Implement the compact field interface and contextual help plan in:
00_Planning/PLAN_COMPACT_FIELD_UI.md

Read AGENTS.md, CLAUDE.md, .agentic/AGENTS.md,
01_Documentation/Code_Style.md, and
06_SoftwareDocu/Development_and_Simulation_Flow.md before editing. Stay on the
current branch. Never stage, commit, push, stash, reset, merge, rebase, revert,
cherry-pick, tag, or create a branch/worktree. Preserve every existing tracked
and untracked change. Do not change the project version.

This is an implementation task, not a request to restate the plan. Follow the
repository AEGIS workflow. Inspect the existing task journals first. If the
current Network task is already at delivery, create or resume the appropriate
focused GUI follow-on task and link its requirement to the Network work; do not
rewrite completed journal history.

Begin by running the documented development application and capturing the
current shell and Network page at 1920x1080, 1366x768, 1024x600, and 800x480.
Inventory tiny text, overflow, clipped actions, inaccessible help, and dialogs
that do not fit. Reconcile those observed facts with the plan before production
edits.

Treat 1024x600 landscape as the primary seven-inch acceptance viewport and
800x480 as the constrained fallback. Keep ordinary body and control text at
16px, dense values at least 15px, and every operator-facing label/help/status at
least 14px. Do not make tables fit by shrinking text. Use responsive columns,
explicit expandable details, compact records/cards, action menus, and
full-screen sheets where the plan calls for them. Field-layout touch targets
must be at least 44x44px.

Implement a progressive help system: persistent guidance for information needed
to make a safe choice, an accessible click/tap/focus help popover for concise
definitions and examples, and “How to configure” help for complex Network
workflows. Essential safety, validation, confirmation, rollback, and route
requirements must remain visible outside a tooltip. Help must work without
hover, close with Escape, return focus, fit the viewport, and be localized in
English, German, and Romanian.

Apply the complete responsive information architecture first to Interfaces,
WireGuard profiles, and Device networks. Keep status, IP/prefix, gateway or
access path, and actions clear. Put secondary data in explicit details. Use the
generic term Device networks. Modbus TCP port 502 may be an example, but ports
remain editable because a real service can use another port such as 504.

Preserve current backend/API/network behavior, authorization, operation
confirmation, themes, and the single SIM/LIVE indicator. Do not add external
fonts, remote assets, or a UI dependency for functionality that can be small and
local. Do not deploy to a Raspberry Pi or change live networking as part of this
task.

Work phase by phase through P0-P8. Add meaningful component and Playwright tests,
inspect light/dark screenshots at every required viewport, and run format check,
lint, unit tests, build, existing end-to-end tests, and the final AEGIS checks.
Do not weaken tests to make the candidate pass. Record deviations immediately
as required by AEGIS and distinguish automated viewport proof from acceptance
on an actual seven-inch display.

Update relevant implementation/user documentation, CHANGELOG.md under
Unreleased, and the task journal from observed results. Deliver a reviewable,
uncommitted working tree with exact test evidence, before/after screenshots, and
remaining physical-display limitations. Do not stop after an audit or plan;
continue through implementation and validation.
```

\n
