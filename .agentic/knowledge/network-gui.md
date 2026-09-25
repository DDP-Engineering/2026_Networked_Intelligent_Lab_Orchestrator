# Network GUI

## Routing

Select this page for first-boot commissioning, Network navigation, adapter
roles, device-network presentation and Network field workflows. Source paths
below are relative to `05_Software/frontend/`. Begin at
`src/components/pages/NetworkPage.tsx` and its focused tests. Shared shell
behavior belongs to [Browser GUI foundation](gui-foundation.md); appliance
rules and helper effects belong to [Appliance Network operations and
commissioning](appliance-network.md).

## Purpose and boundaries

The Network UI presents observed state and explicit proposals, confirmations
and recovery actions. It does not apply host networking directly. Details and
Actions remain separate controls so technical values can be copied safely.

## First-boot commissioning

The management-access payload carries `provisioning`, and the browser parser
reads it as `body.provisioning === true` so an appliance that omits it is
treated as commissioned - the safe direction, because it shows the ordinary
allowlist editor rather than offering to commission an appliance that already
is. `NetworkPage` renders `CommissioningPanel` when that flag is set and
suppresses the allowlist editor in exactly the same condition; the two are
mutually exclusive because while provisioning the appliance refuses every change
the editor could make.

The panel opens with a standing alert naming the exposure as temporary, then
lists each adapter that may carry management access with its name, stable
adapter identity, type, connection state and current addresses, marking the one
this browser session arrived on. An interface that cannot carry management
access is listed with the appliance's own reason and offers no control at all.
The URL section shows the usable appliance URLs together with the facts that the
local name is answered only on the local network and that addresses are not
permanent identifiers.

Completing commissioning sends the chosen adapters. An empty choice is refused
in the browser before anything is sent. Giving up the adapter the session
arrived on raises a second, explicit button naming it, so the consequence is
read before it is accepted. A refusal from the appliance is shown with the
selection kept, so the administrator corrects rather than restarts. The control
is withheld from an identity without the manage permission, and
`completeCommissioning` is withheld entirely under the simulation profile, which
has no eligible physical interface to commission.

## Compact Network overview

The authenticated shell uses one SIM/LIVE badge. Network lists show addresses
and gateways, plus WireGuard configured addresses and peer endpoints. The API
parser retains only public profile metadata. DNS, routes, handshake and
readiness diagnostics are collapsed; Import profile opens the form. Completed
notices are dismissible; pending confirmation, rollback failures and stale data
remain explicit.

## Device networks, adapter configuration and Activity

- `src/api/devicenetworks.ts`, `src/api/activity.ts`, `src/api/settings.ts`
  parse the device-network, activity and adapter-settings resources;
  `contract.ts` defines `adapter_id`, `ssid` and `settings` on an interface, the
  operations listing, and the `activity.read` and `activity.export` permissions;
  `wireguard.ts` defines the `rename` change.
  The client exposes one method per route and `exportActivity` returns the
  export as the text the appliance wrote.
- `src/api/mock.ts` answers these routes through the same acknowledged
  lifecycle, keeps device networks, sessions, the policy and an activity
  history in memory, and gives device-network and Wi-Fi changes a confirmation
  window as the appliance does.
- `components/network/DeviceNetworksPanel.tsx`: table (intent and observation
  are separate columns), editor dialog, enable/disable/delete through previews,
  check and instructions dialogs, activity link narrowed to the network.
  `InterfaceSettingsDialog.tsx`: addressing form and, on a wireless adapter,
  scan and join with a password that is typed once and never displayed.
  `WireGuardPanel.tsx`: rename in the profile details.
- `pages/ActivityPage.tsx`: scope, filters and the More filters disclosure on
  one wrapping line, server paging by cursor, the recording conditions that
  qualify the listing, JSONL/CSV export from the page header, and the
  administrator retention policy above the filters. `shell/pages.ts` places the `activity` core page below
  Authentication, shown to an identity holding `activity.read`; `AppShell` keeps the filter a Network
  page link chose and remounts the page with it.
- `pages/NetworkPage.tsx` reads `GET /api/v1/network/operations` on mount and
  picks up a change still applying, awaiting confirmation, rolling back or
  blocked in rollback failure; it keeps polling the failure until verified
  recovery. An uncertain acknowledgement reacquires the matching authoritative
  operation and replaces any older terminal notice, while a refused preview is
  shown with the appliance's reason rather than as an outage.
- The English, German and Romanian catalogues share the Network keys checked
  by `messages.test.ts`.

## Focused Network navigation

Network is one collapsible System group whose root opens Overview and whose
children open Device access, VPN profiles and Diagnostics. Each destination owns
one workspace tab and one concern: Overview contains the decision summary,
management access and interface configuration; Device access contains forwarding
definitions; VPN profiles contains WireGuard lifecycle and import; and Diagnostics
contains WireGuard readiness plus traffic history. The existing `network` page
id remains Overview; the children use internal
`network:*` ids. Collapsing navigation never closes a tab, Activity remains a
peer page, and common operation refusal, confirmation and recovery notices stay
available on every Network view.

`TrafficChart` labels received (RX) and sent (TX) traffic, scales bytes per
measured minute against a visible time axis, and positions bars by timestamp so
an unmeasured minute remains a gap. One field-sized listbox maps pointer
coordinates to the nearest measured minute, avoiding overlapping targets in
dense one-hour and one-day histories; its options expose exact values, a visible
marker preserves measured zeroes, and arrow/Home/End keys move the active
selection. Hover, touch or keyboard selection shows the minute's time, RX, TX
and total while only the chart itself enters the tab order. Expanded WireGuard
readiness renders as a compact grid of left-label/right-value facts, and the
Statistics title shares one responsive header with the interface and period
controls. This keeps the initial Diagnostics chart in the workspace without
vertical scrolling at 1024x600 and larger; 800x480 may scroll vertically.

## Adapter role selection

`management-access.ts` strictly parses stable adapter identity, the fixed
`management`/`device` role vocabulary and validated 409 conflict details.
`NetworkPage` owns the one management-access observation shared by its focused
views. The management checkbox and device-side option stay visible but disabled
for an opposite claim, with a localized persistent explanation. Device-side
assignment also fails closed while management inventory is reading or failed;
defaults exclude known management claims. The same adapter remains available as
local ingress. `WorkflowHelp` exposes the three confirmed repurposing steps from
both selectors. The development mock and browser fixture return the same
role-aware contract. Management transition notices render only fixed typed
outcomes through the locale catalog, never backend diagnostic prose. A valid
conflict notice raised during preview, acknowledgement, or management update
survives unchanged polling and is superseded only when the corresponding
observed role claim disappears; malformed structured conflicts use a fixed
localized fallback and never retain backend wording.
