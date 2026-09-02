# Changelog

All notable changes to NILO are documented in this file. The repository carries
one project-level Semantic Version, declared in the root `VERSION` file.

## [Unreleased]

### Added

- One-command Windows development start, `02_Tools/scripts/nilo-dev.ps1
  start|status`, which brings up Bench Core under WSL2 and the browser
  interface on Windows, each in its own window, and reports whether they
  answer. It supervises nothing and never signals a process.
- Integrated browser test, `npm run test:e2e` in `05_Software/nilo_gui`, which
  drives a real browser against the Windows interface and a real Bench Core
  under WSL2 on the simulation profile, covering login, status, simulation
  marking, both inventories, and backend-unavailable recovery. On Windows the
  repository gate now runs it, so the browser-to-WSL2 path is gated evidence.
- Repository-local browser runtime, installed below
  `02_Tools/runtime/windows-x86_64/playwright/` by
  `02_Tools/bootstrap/bootstrap-browser.ps1`, so no test run downloads a browser
  and no per-user cache is written.

## [0.1.0] - 2026-09-01

### Added

- Portable Go Bench Core: configuration paths, operation and session domain,
  transactional SQLite foundation, authentication, permission and approval
  policy, and durable audit and artifact stores.
- Integration boundary: package manifests, wire protocol, Python integration
  SDK, and supervised external integration runners.
- Repository-local toolchain and Windows/WSL2 development bootstrap, with the
  Increment 1 acceptance gate.
- Authenticated simulation status server, deterministic simulation inventory,
  and the repository-local GUI foundation.
