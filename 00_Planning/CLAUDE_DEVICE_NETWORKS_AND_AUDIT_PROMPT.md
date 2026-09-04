# Claude Code entry prompt

Copy the following prompt into Claude Code opened in `F:\2026_Networked_Intelligent_Lab_Orchestrator` when you want implementation to begin:

```text
Implement the networking and audit extension described in:
00_Planning/PLAN_DEVICE_NETWORKS_AND_AUDIT.md

Read AGENTS.md, CLAUDE.md and .agentic/AGENTS.md first. Stay on the current
branch (expected master). Never stage, commit, push or otherwise rewrite Git
state. Preserve all existing tracked and untracked changes. Do not change the
project version. Do not modify governance instructions to avoid their rules.

Resume the existing Network foundation in TASK-943403e2. Inspect its status,
brief, relevant full journal history, requirements, spec and handoff. Verify the
actual code against the plan's starting-point table; do not reimplement working
features or treat older test results as proof for new work.

Follow the repository AEGIS workflow. Begin with P0 scope/requirements/design
reconciliation for the expanded feature, then implement P1-P11 in dependency
order. The user-facing name is Device networks, with generic targets rather
than PLC-only or Cabinet terminology. Support multiple saved WireGuard
profiles and one active NILO-managed tunnel at a time.

Build durable operations and audit before new privileged mutations. Protect a
working management path and implement recovery for firewall/routes as well as
NetworkManager. Keep secrets outside public metadata, SQLite audit payloads,
logs and exports. A disabled debug setting must never disable required audit.

Use the existing Windows GUI + WSL2 simulation workflow, pinned tools under
02_Tools, and isolated Linux topology tests. Implement real appliance behavior
as well as simulation; do not report simulation-only controls as complete.

Run meaningful focused tests while implementing, inspect the GUI, and run the
configured full gate on the final integrated candidate. Add separate Linux,
systemd/VM and hardware evidence where applicable. Keep the task journal and
Network_Service_Implementation_Status.md current after each runnable phase.

This request authorizes repository implementation. It does not authorize
changing a production VPN server, deploying to a live Pi, or downloading an
application to a real device. Build and verify all software and isolated tests
first; identify the exact external prerequisite when live acceptance needs it.
Do not read or copy secrets from the user's DDP.conf into fixtures or output.

If the work spans sessions, leave the exact next phase/test and remaining
limitations in the handoff. Deliver the reviewable changes without staging or
committing. Do not stop after merely repeating the plan.
```

The plan includes the detailed GUI fields, addressing examples, safety and audit contracts, implementation phases, acceptance IDs, file ownership and existing verification commands. External peer credentials and device access are deliberately not embedded in this prompt.
