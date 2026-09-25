# Compact field interface

## Routing

Select this page for the shared small-screen layout, type scale, target sizes,
contextual help and record-action patterns. Source paths below are relative to
`05_Software/frontend/`. Start at `src/styles/shell.css`,
`src/components/RowActions.tsx` and `src/layout.test.ts`. Page-specific
Network behavior belongs to [Network GUI](network-gui.md); the shell and API
boundary belong to [Browser GUI foundation](gui-foundation.md).

## Purpose and boundaries

The same interface serves a seven-inch field panel and a desktop display.
Shared layout rules keep controls reachable and readable without changing
action semantics or hiding essential record values.

## Compact field interface and contextual help

The interface is sized for a seven-inch appliance display (`1024x600` primary,
`800x480` fallback) as well as a desktop monitor, and no backend, contract,
permission or confirmation behaviour changed with it.

- `REQ-UX-c8c16d33` - one semantic type scale in `styles/tokens.css`
  (`--text-title` 22px, `--text-section` 18px, `--text-body` 16px,
  `--text-dense` 15px, `--text-label` 14px with their line heights); every
  `font-size` in `styles/shell.css` names a token, nothing operator-facing is
  below 14px, and no heading is set in capitals with widened tracking.
- `REQ-UX-c5f7c0cd` - `--control-height` and `--target-min` are 44px; one base
  `button, .button` rule, 44px inputs and selects, 22px checkboxes inside
  44px labels, and a tab close control the height of the row.
- `REQ-UX-2687f591` - `shell/useViewportClass` reads the same two media queries
  the stylesheet uses and returns `wide | compact | constrained`. Compact
  narrows the rail to `--nav-width-compact` and keeps every label; constrained
  hides the rail, shows the `Menu` control and opens `shell/NavDrawer` (a
  `Modal` with `variant="drawer"`), and `TopRow` folds the appearance and
  language menus into the account menu while keeping the tabs, the network
  summary and the one `SIM`/`LIVE` indicator.
- `REQ-UX-2de87216` - `Modal` is a head (title and `Close`), a scrolling
  `.modal-body`, and a `.modal-actions` bar stuck to its bottom; below 56rem or
  640px it is a full-screen sheet. `components/layers.ts` keeps the stack of
  dismissible layers so Escape closes only the top one: a help popover over an
  editor closes the popover.
- `REQ-UX-2b0b6d59` - `help/HelpPopover` opens on click, tap, Enter, hover and
  focus, exposes `aria-expanded` and `aria-controls`, closes on Escape, outside
  activation and its own control with focus returned, closes any other help
  through one document event, and positions itself from the trigger's rectangle
  so it stays in the viewport. No explanation is left in a `title` attribute.
- `REQ-UX-4570c081` - `help/Field` renders label, help, control, persistent
  guidance and the validation message in reading order, tied to the control by
  `aria-describedby`. The confirmation window, countdown, rollback failure,
  refusal and route prerequisites stay in notices and dialogs.
- `REQ-UX-1860069d` - `help/WorkflowHelp` opens "How to configure" over `Modal`
  with six catalogue sections (`help.dn.*`, `help.if.*`): goal, values needed,
  addressing example, what NILO changes, what must exist elsewhere, verify and
  recover.
- `REQ-UX-cde0b9e2` - `network/NetworkOverview` states the uplink and its
  address, the device side, the active profile or None, and what needs
  attention, computed from the status, the device networks, the WireGuard status
  and the running operation. `network/addressing.ts` holds `uplinkOf`,
  `isAddress`, `isPrefix` and `subnetOf`.
- `REQ-UX-ff989933` - one `table.data-table.record-table` per list with
  `data-label` on each cell; below 56rem the stylesheet turns rows into labelled
  cards. Secondary values live in an explicit details row announced as a region.
- `REQ-UX-61eb69f2` - `components/RowActions.tsx` gives each row one Actions
  control that opens a compact `Modal` attached to the document body. It identifies the object type
  and status, gives every choice a full-width target, keeps disabled entries and
  their reasons visible, separates destructive actions, and cannot be clipped
  by the table scroll compartment.
- `REQ-UX-7d9cf7a1` - record rows have no activation handler; technical
  addresses, prefixes and ports remain plain selectable text beside explicit
  Details and Actions controls.
- `REQ-UX-8859ce12` - the device-network editor is Identity, Device side, Access
  path, Addressing, Services, a collapsed Advanced and a Review that names the
  adapter and address, the authorized sources and target, whether WireGuard is
  involved, whether source NAT or preservation is used and whether the
  management path is affected. Format validation (address, prefix, source list,
  port range) runs before any preview and appears after the field; ports stay
  editable with 502 as an example.
- `REQ-UX-f30891b4` - Bench, Activity, Authentication and About use the same
  classes; Activity keeps time, outcome, action, subject and actor in the row
  and moves category, function, actor identity and the raw record into its
  details.
- `REQ-NFR-aa1a4734` - the English, German and Romanian catalogues cover the
  shared field interface.
- `REQ-TEST-5da12a09` - `e2e/field-display.spec.ts` drives 1920x1080, 1366x768,
  1024x600 and 800x480 in both themes and asserts readable text, 44-pixel field
  targets, no document overflow, no `title`-only explanation, the drawer, the
  sheet, help by pointer and keyboard, the editors' final actions, validation
  and a pending confirmation. It is CSS-pixel proof; the physical seven-inch
  panel is still unverified.
