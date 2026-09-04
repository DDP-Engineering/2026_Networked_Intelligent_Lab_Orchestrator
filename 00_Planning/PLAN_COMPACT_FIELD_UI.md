# Compact field interface and contextual help: implementation plan

Date: 2026-09-04. Status: implementation handoff. This document describes planned GUI work; it does not claim that the compact layouts or help system are already implemented.

Workspace: `F:\2026_Networked_Intelligent_Lab_Orchestrator`. Stay on the current branch and preserve the existing dirty working tree.

## 1. Outcome

Make the NILO interface readable and efficient on a seven-inch Raspberry Pi display and on a field laptop. Operators must be able to identify status, inspect addresses, and complete Network configuration without tiny text, horizontal page scrolling, or a large desktop monitor.

Add contextual help for settings, especially on the Network page. Help must explain what a value controls, when it is required, and the effect of changing it. A tooltip alone must not contain instructions that are necessary to complete or safely apply a workflow.

The interface should remain restrained. It should show the facts needed for the current decision, move secondary facts into expandable details, and keep advanced settings out of the primary path until requested.

## 2. Working rules

1. Read `AGENTS.md`, `CLAUDE.md`, `.agentic/AGENTS.md`, `01_Documentation/Code_Style.md`, and `06_SoftwareDocu/Development_and_Simulation_Flow.md` before editing.
2. Follow AEGIS for implementation. Inspect the existing task state and open or resume the appropriate GUI task. If the Network task is already at delivery, do not rewrite its history; record this as a focused follow-on requirement and link the related Network requirement.
3. Never stage, commit, push, stash, reset, merge, rebase, revert, cherry-pick, tag, or create a branch/worktree. Leave a reviewable working tree for the human.
4. Preserve all current tracked and untracked changes. Reconcile the plan with the actual components before editing them.
5. Do not change backend networking behavior, HTTP contracts, permissions, persistence, or appliance policy for this UI milestone unless a demonstrated UI defect cannot be fixed without a compatible contract correction.
6. Do not change `VERSION` or its mirrors. Add a concise `CHANGELOG.md` entry under Unreleased when implementation is complete.
7. Keep the application self-contained and usable without Internet access. Do not add a web font, CDN asset, analytics service, or remote help service.
8. Prefer existing React, CSS, icons, modal behavior, themes, and test tools. Do not add a UI dependency merely to implement a tooltip or responsive table.
9. Preserve English, German, and Romanian catalogues. New visible text and accessible labels must exist in all three.
10. Preserve one clear SIM/LIVE indicator. Do not add repeated simulation notices or explanatory banners.

## 3. Target displays and input methods

Use CSS pixels and the usable browser viewport for acceptance. Test at 100% browser zoom first.

| Class | Required viewport | Expected behavior |
|---|---:|---|
| Seven-inch field display | `1024 x 600` landscape | Primary required layout. All workflows are complete and readable; no page-level horizontal scrolling. |
| Constrained seven-inch display | `800 x 480` landscape | Supported fallback. Content may stack and scroll vertically; dialogs become full-screen sheets; all actions remain reachable. |
| Field laptop | `1366 x 768` | Full labels and efficient tables without excessive empty space. |
| Desktop regression | `1920 x 1080` | Current desktop capability remains complete; content does not stretch into hard-to-scan lines. |
| Portrait or very narrow browser | `600 x 800` and `320` CSS px reflow check | Graceful single-column operation. It is a compatibility check, not the primary appliance orientation. |

Support mouse, keyboard, and coarse touch input. Never require hover to reveal an action or essential instruction.

## 4. Readability contract

### 4.1 Typography tokens

Create a small semantic type scale in `src/styles/tokens.css` and replace scattered component-specific micro sizes with those tokens.

Recommended starting values, to be verified in screenshots:

| Token/use | Target size | Target line height |
|---|---:|---:|
| Page title | `22px` | `1.25` |
| Section title | `18px` | `1.3` |
| Body, controls, important table values | `16px` | `1.45` |
| Dense table values and navigation labels | `15px` | `1.4` |
| Field labels, secondary status, help text | `14px` | `1.4` |

Do not render operator-facing text below `14px`. Do not shrink technical values such as IP addresses, CIDRs, ports, interface names, or error details. Monospace text may use a different face but must use the same readable size tier as the surrounding value.

Use normal sentence casing. Do not compensate for small text with wide letter spacing or all-uppercase copy. Preserve adequate contrast in all themes and make muted text readable outdoors on an ordinary laptop display.

### 4.2 Controls and spacing

- Primary controls, icon buttons, row menus, checkboxes with labels, and touch targets are at least `44 x 44px` on field layouts.
- Inputs and selects are at least `44px` high on field layouts. Their values and placeholders use the body size.
- A dense desktop row may be slightly shorter only when every interactive target remains at least `40px`; the seven-inch layouts keep the `44px` target.
- Use a consistent spacing scale. Reduce unused margins and decorative whitespace before reducing type or control size.
- Focus rings remain visible in light and dark themes. Disabled controls remain distinguishable without relying only on opacity.
- Long names, addresses, and translated ranges wrap or truncate with an accessible way to read the full value. They must not force page overflow.

## 5. Responsive shell

The current narrow layout hides many labels and was designed mainly by width. Replace it with explicit field behavior that also accounts for short height.

### 5.1 Navigation and top row

- At `1024 x 600`, retain visible navigation labels in a compact rail when the page still has enough useful width. Reduce the rail from its desktop width rather than hiding every label.
- At `800 x 480` and similarly constrained widths, use a menu button that opens a labeled navigation drawer or sheet. Do not make an unlabeled icon rail the only navigation.
- The currently selected page is always stated in text in the page header.
- Keep the tab strip horizontally scrollable within its own region. It must not expand the document width.
- Keep the most important appliance state and the SIM/LIVE indicator visible. Move lower-priority account, language, theme, or system actions into an existing menu when space is insufficient.
- Do not wrap the shell into several permanently occupied header rows on a `600px`-high display.

### 5.2 Page and overlay behavior

- Use one predictable page scroll area. Avoid nested vertical scroll regions except inside an intentionally bounded table or overlay.
- Set a readable maximum content width on large displays while using available width on field displays.
- Modals must fit inside `100dvh`. On short or narrow displays, convert large configuration dialogs to a full-screen sheet with a scrollable body and visible title, close control, and sticky action bar.
- Opening a drawer, popover, or modal must not move focus behind it. Escape closes the top dismissible layer and focus returns to its trigger.
- Confirmation countdowns, rollback warnings, failed operations, and lost-connection instructions remain visible above ordinary help or informational content.

## 6. Progressive help model

Use three levels of help. Each level has a distinct purpose.

### 6.1 Persistent field guidance

Place a short sentence below a field when the operator needs it to choose a valid value. Examples include the required relationship between NILO's device-side address and the target subnet, or the consequence of preserving the remote source address.

Persistent guidance must not disappear on touch devices. Keep it to one or two short sentences. Put validation and correction messages next to the affected field, after the field in reading order.

### 6.2 Help popover

Add a reusable `HelpPopover` or equivalent component for concise definitions and examples. A small help button sits next to the setting label, with a localized accessible name such as “Help for Gateway”.

The popover must:

- open on click/tap and keyboard activation;
- provide useful hover behavior for mouse users without making hover mandatory;
- associate the trigger and content with suitable ARIA attributes;
- remain open while its content is used;
- close with Escape, outside activation, or the close control;
- return focus to its trigger;
- stay within the viewport and avoid covering the active input when possible;
- support wrapped content at `800 x 480` and at 200% text zoom;
- contain text and simple examples, never an actionable control whose discovery depends on the popover;
- avoid the native `title` attribute as its only implementation.

Use a true short tooltip only for brief names of familiar icon-only controls. Use the popover for settings help.

### 6.3 Workflow help

Provide a “How to configure” action in complex Network editors. It opens a localized help panel or modal with:

1. the goal of the workflow;
2. the values the operator needs before starting;
3. a small addressing example;
4. what NILO will change;
5. what must already be configured on another computer, router, VPN hub, or target device;
6. how to verify the result and recover if communication fails.

This help is documentation, not a wizard that silently chooses network policy. It must use generic “device” terminology. PLC and Modbus may appear as examples.

## 7. Network page information architecture

The Network page is the first complete application of this work. Keep its major sections: **Interfaces**, **WireGuard profiles**, and **Device networks**. Place Statistics/traffic and detailed diagnostics after the configuration sections or behind an explicit details action.

### 7.1 Compact overview

At the top, show one concise overview containing only:

- management/uplink interface and current address;
- device-side interface and address, when configured;
- active WireGuard profile, or “None”;
- pending operation, warning, or recovery failure.

Do not repeat status already obvious in the first rows. Do not repeat the simulation explanation inside each section.

### 7.2 Responsive lists

Do not solve narrow tables by reducing fonts. Each section uses the same responsive pattern:

- On desktop and field laptop, use the existing table style with readable values and a bounded set of columns.
- At `1024 x 600`, keep essential columns and place secondary values in an expandable row detail.
- At `800 x 480` or narrow reflow widths, render each item as a compact record/card with label-value pairs.
- Use one `Actions` menu per row/card when several actions would otherwise form a button cluster. Keep a directly visible primary action only when it is safe and unambiguous.
- The row/card itself is not a hidden activation target. Details and actions have explicit controls.

Essential information by section:

| Section | Always visible | Expandable detail |
|---|---|---|
| Interfaces | Name/type, role, state, IP/prefix, primary action | Gateway, DNS, speed, RX/TX, stable adapter identity, management protection, ownership |
| WireGuard profiles | Name, state, tunnel address, startup selection, primary action | Endpoint, last handshake, peer details, DNS/routes, dependency and error information |
| Device networks | Name, device interface, target/device subnet, access method, exposed address, state | Source ranges, selected VPN/local ingress, service policy, NAT/source behavior, startup intent, diagnostics |

Use status text plus color/icon; do not encode state by color alone. Preserve exact IP addresses, gateways, CIDRs, and ports as selectable text.

### 7.3 Editors

Group long configuration forms into a clear sequence:

1. **Identity**: name and optional location.
2. **Device side**: interface, NILO address/prefix, target host or subnet.
3. **Access path**: local, WireGuard, or both; ingress/profile and source ranges.
4. **Addressing**: original or translated address/range.
5. **Services**: protocols and ports.
6. **Advanced**: source-address behavior, startup and other expert settings.
7. **Review**: concise before/after summary and connectivity risk before apply.

On a short display, this may be a stepper, accordions, or a single form with sticky section navigation. Choose the simplest pattern that preserves entered values, validation context, and keyboard order. Do not open every advanced group by default.

The final review must state, in plain language, which interface receives which address, which sources may reach which target, whether WireGuard is involved, whether translation/SNAT is used, and whether the management connection could be affected.

### 7.4 Network help catalogue

Add help copy for at least these concepts. Keep each popover short; place longer consequences in persistent or workflow help.

| Concept | Help must explain |
|---|---|
| Device-side interface | The physical Ethernet/USB adapter connected toward the managed device network; adapter identity may persist even if its displayed name changes. |
| DHCP / Static | Where the address comes from and why a device-side gateway normally uses a static address. |
| NILO address and prefix | NILO's address on the device subnet; it must be unused and in the same subnet as the target. It does not replace the target address. |
| Gateway | The next hop for traffic leaving an interface. A device-side interface normally has no default gateway; the uplink keeps the default route. |
| Management protection | Why disabling or readdressing the last management path is blocked or requires recovery confirmation. |
| Target IP / device subnet | One existing device address versus an explicitly exposed subnet. |
| Local ingress | The management-side interface from which local clients may enter. |
| WireGuard profile | The saved tunnel used for remote access; activating a profile is separate from authorizing a device network. |
| Allowed source range | Which client addresses may initiate access. It is an access restriction, not the target range. |
| Original / Translated address | When the client uses the real device address and when an alternate exposed range is needed for duplicate or conflicting networks. |
| Source NAT | Default behavior that makes replies return through NILO without changing the target gateway. |
| Preserve source | The target can see the real client, but it needs a valid return route; this is an advanced choice. |
| Protocol and port | The application service to allow. Modbus TCP commonly uses `502`, but the actual device may use another configured port such as `504`; the value must remain editable. |
| Startup | Whether NILO should try to restore this profile or device-network intent after restart. |
| Apply and confirmation | Temporary application, confirmation deadline, and automatic rollback if connectivity is not confirmed. |

Use a non-secret example such as:

- management computer: `192.168.1.13`;
- NILO uplink: an address in `192.168.1.0/24`;
- NILO device side: `192.168.0.1/24`;
- device: `192.168.0.130`;
- application check: TCP port selected by the operator.

Make clear that another computer/router or VPN hub may still need a route. Do not imply that a local target probe proves end-to-end remote access.

## 8. Other pages

After the Network page works, apply the type scale, control targets, shell behavior, and overflow rules to Authentication, Bench, Activity, Administration, Settings, and simple placeholder pages.

Do not redesign their product workflows during this milestone. Fix only issues needed for consistent readability, responsive navigation, dialogs, tables/cards, and help primitives.

Activity is a useful secondary acceptance page because it contains dense records. At field widths, keep time, outcome/severity, action, subject, and actor readable; move raw detail into an explicit expansion.

## 9. Implementation phases

### P0 — Measure and reconcile

- Start the current development application using the documented script and inspect the real Network page.
- Capture baseline screenshots at `1920x1080`, `1366x768`, `1024x600`, and `800x480` in light and dark themes.
- Inventory every rendered font size below `14px`, page-level horizontal overflow, clipped action, oversized dialog, icon-only control, and hover-only explanation.
- Map current components and tests. Record the final scope and acceptance IDs in AEGIS before production edits.

### P1 — Type and metric tokens

- Add semantic typography, control-height, touch-target, spacing, and readable-line-length tokens.
- Normalize base text, labels, buttons, fields, tables, status text, menus, and monospaced values.
- Remove local font-size overrides that violate the readability contract.
- Verify both themes before changing layout.

### P2 — Responsive shell

- Implement the compact labeled rail and constrained drawer behavior.
- Bound tab-strip and top-row overflow.
- Make the page scroll region and overlay sizing reliable for short viewports.
- Preserve focus, menus, selected-page context, and SIM/LIVE state.

### P3 — Help primitives

- Implement and test the help button/popover, persistent field guidance, and workflow-help container.
- Add localized accessible labels and interaction behavior.
- Document a short usage contract so future pages do not put critical instructions in hover-only tooltips.

### P4 — Network responsive lists

- Add the concise overview.
- Adapt Interfaces, WireGuard profiles, and Device networks to table/detail/card layouts.
- Consolidate row action clusters and preserve explicit status and exact technical values.
- Keep Statistics and diagnostics reachable without letting them dominate the page.

### P5 — Network editors and help copy

- Group settings by operator task and collapse advanced options.
- Add field guidance, popovers, examples, validation proximity, review summaries, and workflow help.
- Verify that help describes current behavior and does not promise unavailable routing or VPN automation.

### P6 — Cross-page consistency and localization

- Apply the shell/readability rules to other pages without expanding their scope.
- Complete English, German, and Romanian catalogues.
- Check long translated labels at all target viewports.

### P7 — Automated and visual validation

- Run focused component tests during each phase.
- Add Playwright field-display coverage and inspect screenshots.
- Run GUI format check, lint, unit tests, build, and the existing end-to-end suite.
- Run the repository's configured final AEGIS checks. Report any unrelated baseline failure as a deviation; do not weaken a gate.

### P8 — Documentation and handoff

- Update the GUI/Network implementation status, relevant user documentation, changelog, and AEGIS journal with observed evidence.
- Record any remaining hardware-only visual acceptance, browser/OS limitation, or untranslated technical term.
- Leave the result uncommitted and ready for human inspection.

## 10. Tests and acceptance criteria

### 10.1 Component tests

- Help trigger opens by mouse click, keyboard, and touch-equivalent pointer input.
- Escape/outside activation closes the popover; focus returns to the trigger.
- Only one settings help popover is open at a time.
- Content has an accessible name/description relationship and no help exists only in `title`.
- Responsive row details expose every field removed from the compact table.
- Row action menus retain every permitted action and their disabled reasons.
- Long IP/CIDR/profile values do not cause document overflow.
- Dialog action controls remain available while the body scrolls.
- All new catalogue keys exist in English, German, and Romanian.

### 10.2 Playwright matrix

For each required viewport, sign in and verify the shell plus Network page. Cover at least:

1. navigation to Network;
2. inspection of interface, WireGuard, and device-network details;
3. opening and closing field help with keyboard and pointer;
4. opening each major editor and reaching its final action controls;
5. validation of an invalid address/port;
6. viewing a pending confirmation or equivalent simulated operation state;
7. light and dark theme screenshot review;
8. absence of page-level horizontal overflow.

At `800x480`, also verify the navigation drawer, full-screen editor, sticky actions, and software keyboard-safe scroll behavior where automation permits.

### 10.3 Acceptance IDs

| ID | Acceptance condition |
|---|---|
| CFUI-01 | No operator-facing text is rendered below `14px`; ordinary body/control text is `16px` and dense values are at least `15px`. |
| CFUI-02 | At `1024x600`, every application page is usable without document-level horizontal scrolling. |
| CFUI-03 | At `800x480`, every primary workflow remains complete through vertical scrolling, stacking, drawers, or full-screen sheets. |
| CFUI-04 | At field sizes, interactive touch targets are at least `44x44px`, focus is visible, and controls are not hover-dependent. |
| CFUI-05 | Network shows Interfaces, WireGuard profiles, and Device networks with their essential status, IP/prefix, gateway/access, and actions in readable form. |
| CFUI-06 | Secondary Network data remains available through explicit row/card details; responsive layout never silently discards it. |
| CFUI-07 | Complex Network settings have concise field help and a “How to configure” explanation with an addressing example and external-route dependencies. |
| CFUI-08 | Settings help works with mouse, keyboard, and touch; Escape dismissal and focus return are verified. |
| CFUI-09 | Critical safety, validation, pending-confirmation, and rollback information is persistent and is never available only in a tooltip/popover. |
| CFUI-10 | Large dialogs fit the usable viewport; title, close control, and final actions remain reachable at `800x480`. |
| CFUI-11 | English, German, and Romanian layouts retain meaning and do not clip required actions or labels. |
| CFUI-12 | Light and dark themes meet existing contrast expectations and preserve readable muted/status text. |
| CFUI-13 | Existing Network behavior, authorization, operation confirmation, and API contracts continue to pass their regression tests. |
| CFUI-14 | Field-display Playwright coverage runs at `1366x768`, `1024x600`, and `800x480`, with desktop regression at `1920x1080`. |
| CFUI-15 | The final handoff includes before/after screenshots and records any check that was not run on an actual seven-inch display. |

## 11. Likely code areas

Reconfirm paths before editing; this is an ownership map, not a mandate to create every suggested file.

- `05_Software/nilo_gui/src/styles/tokens.css`
- `05_Software/nilo_gui/src/styles/shell.css`
- `05_Software/nilo_gui/src/components/shell/AppShell.tsx`
- `05_Software/nilo_gui/src/components/shell/SideNav.tsx`
- `05_Software/nilo_gui/src/components/shell/TabStrip.tsx`
- `05_Software/nilo_gui/src/components/shell/TopRow.tsx`
- `05_Software/nilo_gui/src/components/Modal.tsx`
- a small shared help component under `05_Software/nilo_gui/src/components/`
- `05_Software/nilo_gui/src/components/pages/NetworkPage.tsx`
- `05_Software/nilo_gui/src/components/network/InterfaceSettingsDialog.tsx`
- `05_Software/nilo_gui/src/components/network/WireGuardPanel.tsx`
- `05_Software/nilo_gui/src/components/network/DeviceNetworksPanel.tsx`
- `05_Software/nilo_gui/src/i18n/{en,de,ro}.ts`
- existing component tests and `05_Software/nilo_gui/e2e/`
- relevant GUI and Network user/implementation documentation

Prefer a shared responsive record pattern and one help primitive. Avoid parallel desktop/mobile component trees that can drift in behavior.

## 12. Delivery evidence

The implementation handoff must include:

- the AEGIS summary and requirement/acceptance mapping;
- a concise list of changed behavior;
- exact validation commands and results;
- screenshots for every required viewport in both themes, or a clearly recorded deviation for any missing physical-device check;
- known limitations, especially behavior that still needs an actual Raspberry Pi seven-inch display;
- confirmation that no network backend semantics, credentials, versions, Git staging, or commits were introduced.

\n
