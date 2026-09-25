# Plugin navigation and appliance administration plan

**Status:** Camera navigation and administration implementation in progress,
2026-09-25. CAM-10/11 live preview remains gated by physical feasibility. This plan covers the navigation and administration request
shown in the supplied Plugins screenshot. The existing
`.agentic/specs/plugin-workflow-plan.md` is deferred with the Sigrok
reimplementation; its earlier one-level navigation proposal does not govern
this Camera-focused work.

## 1. Outcome and boundaries

NILO has one set of installed plugins and managed devices per appliance. System
is where an administrator changes that appliance's plugin and device
configuration. Bench is where authorized users work with enabled plugins and
their configured hardware. This phase implements the structure for Camera and
keeps it suitable for later plugins. Every menu item has a real destination and
is shown only when its capability exists. Camera uses physical devices and
images only; this plan adds no Camera simulation source or data. Sigrok and
PulseView are outside this phase and require a separate reimplementation.

The requested tree is:

```text
Bench
  Camera                         enabled plugin overview
    Board camera                 configured device, user-friendly name
      Snapshot                   existing retained capture workflow
      Settings                   working controls and capture options
      Live View                  only after the physical live-preview gate passes
    Inspection camera
      Snapshot
      Settings
      Live View                  when supported by that device
System
  Plugins                        package catalog and installed status
    Camera                       appliance lifecycle and configuration
  Network
  Authentication
  Activity
  About
```

The plugin row is a family, not a hardware instance. One installed Camera
package can own many `cam-N` records. Stable record IDs are used in routes,
jobs and artifacts. A revisioned, administrator-assigned display name is used
in navigation, tabs and results. A device without a configured name appears in
System configuration, not as a ready Bench instrument.

The existing Sigrok package may remain visible in the package catalog while it
exists. Its analyzer navigation, device model, workflows, naming and acceptance
are excluded here. This plan does not rename it to PulseView or modify its
runner. Its replacement can adopt the shared tree and administration contract
in a separate plan.

## 2. Scrollbar and page layout

The screenshot shows a blank strip to the right of the content scrollbar.
Current `.page-body` in `nilo_gui/src/styles/shell.css` has both
`max-width: var(--content-max-width)` and `overflow-y: auto`; its scrollport
therefore ends before the right edge of a wide workspace.

Make the page scrollport fill the complete `.shell-workspace` width and put any
readability max-width on a child content container. The scrollbar must sit at
the right edge of the workspace, beside the browser edge, with no unused strip
after it. Align fixed page headers and scrolled content without a horizontal
jump when the scrollbar appears. Keep the left navigation's independent scroll
behavior. Verify pages with short and long content, nested tables, browser zoom,
responsive drawer, and browsers with classic and overlay scrollbars. Do not
create horizontal page scrolling to achieve the alignment.

## 3. One authoritative inventory and route tree

Build Camera's System and Bench navigation from the same authenticated plugin
and managed-device inventory. Do not infer lifecycle from the older integration
inventory or from the currently open page. Inventory includes stable plugin
id, installed/enabled/health state, declared views and configured device
records with stable id, name, kind, presence, readiness, enablement and
capabilities. Available catalog packages appear in System > Plugins overview;
Camera gets a System child page when installed. Enabled Camera gets a Bench
child even while unhealthy, so a problem remains visible. Its configured
devices remain listed while absent or disabled, with status and safe history;
capture and live actions are unavailable until ready.

Use stable route/page IDs derived from plugin and device IDs, never display
names. Split Camera's current `PluginsPage` concerns into a catalog page, a
Camera System administration page, a Camera Bench overview, and per-device
working pages.
Keep selection and open tabs attached to IDs when a name changes. Reconcile
tabs after install, enable, disable, uninstall, device rename/rebind/forget, and
changes made by another browser, CLI or MCP client. A failed inventory refresh
shows stale state and a retry action; it must not make a plugin disappear as if
uninstalled.

Replace the current one-level `SideNav` grouping with a recursive tree that
supports the required depth. Each branch has a separate open action and
expand/collapse control, keyboard and screen-reader labels, visible focus,
appropriate indentation, and a bounded scrolling region for many devices.
Remember expansion within the session and expand the ancestors of the active
page. Long user names truncate visually but remain available through the
accessible label and page title. The same tree works in the constrained drawer.

Plugin-specific leaf views come from a reviewed view registry in the GUI,
matched to declared backend capabilities. A future package cannot inject
arbitrary routes, HTML or permission names. A view entry specifies its scope
(plugin or device), stable view id, label/help key, required permission and
capability, and registered page component. The server remains authoritative for
whether an operation is allowed. Unknown view ids are omitted with a visible
plugin status diagnostic in System rather than opening a broken page.

## 4. System > Plugins: appliance configuration

The root page lists installed plugins with status and version and available
catalog packages with an Install action for administrators. Camera's child page
shows Enable/Disable, Restart, Configure, approval mode,
package version/health, and the existing upgrade/uninstall actions where
applicable. Configure edits persistent plugin limits and defaults using the
installed package schema and a revision check. It also contains a device
administration list: discover, name/rename, purpose, persistent defaults,
enable/disable, rebind and forget as supported by that family. A package
without a device configuration schema omits those controls rather than showing
an unusable form.

Use the existing `plugins.manage` permission for installation, lifecycle,
approval mode and persistent plugin/device configuration. The administrator
can perform these actions; operators and viewers see status and allowed help
without mutation controls. Direct HTTP, CLI and MCP calls enforce the same
permission. Token scopes only narrow the actor's role. Do not treat a hidden
button as an authorization check. Restart remains administrator-only under the
current contract; a separate operator permission would be a later explicit
policy decision.

Lifecycle actions report busy/refused/failed states from Bench Core and refresh
authoritative inventory before updating navigation. Disable blocks new work
and follows the current queued/running-job contract. Restart and uninstall
respect active-work refusal. Uninstall and device forget require an explicit
confirmation that names the affected plugin or record and explains what
history remains. Persist names/configuration per appliance, not per user or
browser; record changes in the existing audit stream.

## 5. Bench: Camera and device work

Bench lists only installed, enabled plugin families. The plugin overview shows
its configured devices, presence/readiness and recent authorized results. It
offers discovery to an actor with `bench.execute`, but never offers appliance
configuration. Each device has a working landing page and view leaves. For
Camera, keep retained Snapshot and its browser viewer; `Settings` contains
operation-time capture choices and physical controls. Persistent baseline,
mounting and calibration are edited from System > Plugins > Camera by an
administrator. Viewer may inspect authorized results; operator/admin may
execute device work subject to approval policy and device readiness.

Camera Live View is an actual device stream, not a renamed snapshot or replay.
The existing Camera plan gates it on CAM-10 physical measurements and an
accepted transport, lease, permission and cleanup design. Implement CAM-11
after that gate. Show `Live View` only when the deployed core/package and the
particular device advertise support. This avoids a dead or simulated menu
item during the navigation rollout.

## 6. Delivery sequence

1. **Specify the contracts.** Reconcile this plan with the Camera plan. Fix the
   plugin/device/view inventory shape, System versus Bench settings ownership,
   and lifecycle state rules. Add focused requirements for navigation, RBAC and
   scrollbar placement. Keep Sigrok reimplementation in a separate scope.
2. **Complete Camera inventory foundations.** Use existing `cam-N` managed
   records with stable identities, names, revisions and absent state. Extend
   authoritative plugin/device inventory only where Camera navigation needs it;
   leave Camera's physical-only runner unchanged.
3. **Split pages and build navigation.** Add route descriptors and the recursive
   sidebar; separate catalog, plugin administration, plugin overview and
   device views. Move Camera device administration out of the current all-in-one
   Plugins page. Leave the legacy Sigrok workflow alone. Reconcile Camera tabs
   after external changes.
4. **Apply permissions and settings.** Wire each System action to the existing
   guarded backend routes. Put persistent device settings under System and
   operation-time controls under Bench. Add per-family view declarations only
   for implemented views. Keep EN/DE/RO text, themes and responsive behavior.
5. **Fix the scrollport.** Make it full workspace width, move max-width to an
   inner wrapper, and verify fixed headers, nested tables and sidebar scrolling.
6. **Add real Camera live view.** Run CAM-10 physical live-preview feasibility,
   then CAM-11 implementation if accepted; enable Live View navigation only
   after the real feature passes acceptance.
7. **Validate and deploy.** Run focused navigation/RBAC/layout tests and the
   full Go and GUI suites. Exercise the Pi in Chrome with the physical Camera
   package. Test lifecycle and rename from a second client,
   reconnect/absence, direct forbidden requests, narrow and wide viewports,
   200% zoom, keyboard, EN/DE/RO and light/dark themes. Run the live-preview
   hardware matrix when the required cameras and targets are available.

## 7. Acceptance checks

- At 1920x1080, the Plugins page scrollbar is at the workspace's right edge;
  no blank strip follows it. The same holds at 1366x768, 800x480 and 200% zoom.
- System > Plugins lists the installed Camera package as a child. An
  administrator can enable, disable, restart and configure it; an
  operator/viewer cannot perform those mutations through any API. The legacy
  Sigrok package listing and workflow are not acceptance targets for this phase.
- In this phase, Bench adds an enabled Camera branch. Two configured cameras
  appear as separate, renameable hardware records, with stable pages through rename,
  reconnect and refresh. An absent device stays identifiable without allowing
  capture or live work.
- Camera Snapshot opens the correct physical device and keeps its existing
  approval, artifact and history behavior. Plugin-specific menu
  leaves appear only for working capabilities. Camera Live View passes its
  physical transport and cleanup checks before it appears.
- Package lifecycle and device actions made in another client update the tree
  without a reload, unauthorized controls remain unavailable after a role or
  token-scope change, and a failed refresh does not erase open working state.

## 8. Current implementation seams

- `nilo_gui/src/styles/shell.css`: `.page-body` combines max-width and vertical
  scrolling, producing the screenshot's right-side gap.
- `nilo_gui/src/components/shell/pages.ts` and `SideNav.tsx`: Camera has one
  child level and the tree does not recurse.
- `nilo_gui/src/components/shell/AppShell.tsx` and `PluginsPage.tsx`: the same
  large page handles package lifecycle, Camera work and jobs.
- `nilo_assistant/internal/bench/managed_devices.go`: Camera already uses
  managed records that can drive the new device tree.
- `nilo_assistant/internal/auth/role.go` and `internal/httpapi/bench.go`:
  `plugins.manage` currently protects all lifecycle and configuration writes.

The Camera navigation implementation is tracked in `TASK-751c6c4b`. Git staging,
commit and push require a separate explicit user instruction.
