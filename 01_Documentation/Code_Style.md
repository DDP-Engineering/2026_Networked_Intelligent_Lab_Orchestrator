<!--
Copyright (c) 2026 DDP Engineering
This software is licensed under the PolyForm Noncommercial License 1.0.0.
Commercial use requires a separate commercial license.
See the LICENSE.txt file for full license details. SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
-->

# Code Style

## Purpose

This document defines the code style, documentation style, and implementation
readability rules for NILO production code, tests, and repository tooling.
The general rules apply to every language; the language-specific sections
adapt them without imposing one language's syntax or documentation conventions
on another.

## General Code Style

- Code must be clear, simple, and easily maintainable by a human developer.
- Prefer small, composable modules.
- Avoid unnecessary abstraction, wrapper classes, and speculative architecture.
- Keep implementation sufficiently documented so intent, contracts, and
  non-obvious logic remain understandable during maintenance.
- Keep responsibilities separated across parsing, processing, orchestration,
  export, and UI.
- Avoid host-specific assumptions in reusable code.
- Prefer deterministic local configuration and storage paths.
- Preserve public interfaces unless change is explicitly requested.

## Go Code and Documentation Style

Use idiomatic Go rather than translating the Python conventions below.

Required formatting and organization:

- Format every Go source and test file with `gofmt`.
- Let `gofmt` organize imports; do not maintain decorative alignment manually.
- Keep package names short, lowercase, and descriptive without stuttering.
- Keep exported APIs small and introduce interfaces at the consuming boundary
  when substitution is required, not speculatively.
- Prefer explicit constructors for values that must be validated or preserve
  invariants.
- Put platform-specific behavior in narrowly scoped build-tagged files while
  keeping reusable contracts platform-neutral.

Documentation rules:

- Add a package comment when the package's purpose, contract, or safety
  boundary is not obvious from its name and files.
- Document exported identifiers when they form a public or cross-package
  contract. Start the comment with the identifier's name where Go conventions
  require it.
- Document unexported functions and types when they implement non-obvious
  invariants, concurrency ownership, persistence guarantees, protocol rules,
  or safety behavior.
- Explain why a constraint exists rather than narrating straightforward code.
- Use normal Go comments and Go documentation syntax. Do not add Python
  shebangs, encoding declarations, Doxygen tags, decorative section markers,
  or end-of-file markers to Go files.

Implementation rules:

- Pass `context.Context` explicitly as the first parameter of operations that
  may block, perform I/O, or be cancelled; do not store it in long-lived
  structs.
- Wrap errors with useful operation context using `%w` when callers need
  `errors.Is` or `errors.As`; do not discard typed failure meaning.
- Make goroutine ownership and shutdown explicit, and do not leave background
  work without a bounded termination path.
- Prefer table-driven tests when several cases prove the same contract, and
  use focused fixtures or helpers rather than duplicating setup.
- Test public behavior, boundary conditions, failure paths, and concurrency or
  persistence invariants in proportion to their risk.

## Python Documentation Style

For Python source and test files, use a consistent engineering-style header and
Doxygen-like documentation pattern.

Required file prologue:

1. shebang: `#!/usr/bin/env python3`
2. encoding line: `# -*- coding: utf-8 -*-`
3. copyright/license banner using dashed separator lines
4. module docstring with aligned tags. The following are required:
   - `@file`
   - `@brief`
   - `@details`
   Optional only when relevant:
   - `@example`

Docstrings carry no authorship or version tags (no `@author`, `@date`, or
`@version`) at module, function, method, class, or nested-helper level. The
file-top copyright/license banner is the sole in-file attribution. Versioning
is project-level only, tracked in the AEGIS payload version and the existing
changelog.

Required section markers where meaningful:

- `IMPORTS`
- `CONSTANTS`
- `DATA MODELS / TYPEDEFINITIONS`
- `HELPER FUNCTIONS` or `LOCAL HELPERS`
- `PUBLIC API` or `EXPORTED FUNCTIONS / PUBLIC API`
- `END OF FILE`

Function, method, class, and nested-helper docstrings should use Doxygen-like
tags whenever the callable or type is non-trivial, part of the public API,
implements important logic, or benefits from an explicit contract.

Required when applicable:

- `@brief`
- `@param[in]`
- `@param[out]`
- `@param[in,out]`
- `@return`

Use when applicable:

- `@exception`
- `@note`
- `@todo`
- `@retval`
- `@example`

Guidance:

- trivial one-line helpers may omit docstrings if the intent is already obvious
- public API functions should normally have docstrings
- classes should have a class-level docstring
- nested wrappers and decorator helpers should be documented when they carry
  non-obvious behavior

Keep these comments concise and engineering-focused. Avoid placeholder prose or
casual commentary.
