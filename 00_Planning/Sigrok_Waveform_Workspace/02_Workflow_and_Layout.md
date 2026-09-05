# NILO workflow and layout

Proposed product behavior. The appliance owns plugin enablement; individual users may navigate and inspect captures independently within their permissions.

## Navigation and plugin administration

```text
Bench
  Sigrok                 ← present while enabled on this appliance
  <other enabled plugin>

System
  Plugins
    Sigrok               ← present while installed, including when disabled
    <other installed plugin>
  Network
  Authentication
  Activity
```

System → Plugins remains an installed-package overview and installation entry point for administrators. Selecting its Sigrok child opens a compact administration page: short name, description, version, enabled/disabled state, runtime health, dependency versions, Enable/Disable, Restart, Uninstall and Activity. Do not combine this page with the acquisition form or waveform history. Remove the redundant large “Plugins” heading; the existing tab and navigation already locate the user.

Enable/disable changes the appliance configuration and is reflected in every connected client's navigation. Runtime failure does not silently disable the plugin: its Bench entry stays visible with a diagnostic state. Restart preserves enablement and configuration. A disabled plugin can expose a runtime-unavailable reason without pretending it is actively running.

Uninstall requires administrator confirmation naming the package and explaining data retention. Default proposal: remove the installed plugin and its navigation entry, preserve saved captures and audit records. Data purge is a separate explicit operation. Bundled delivery must not silently reinstall an intentionally uninstalled plugin during ordinary appliance startup; package upgrade policy must reconcile that choice. A saved waveform library should remain reachable from Bench even if its analysis plugin is unavailable, with raw download and dependency information.

For a busy plugin, default to a clear busy response listing accessible affected operations. Do not interrupt another user's acquisition through an ordinary restart/disable/uninstall. An administrator may choose a separately confirmed stop-and-proceed flow. It closes admission first, settles affected operations and releases hardware before the lifecycle change. State transitions and navigation changes are broadcast to clients.

## The Sigrok workspace

The primary screen is the waveform, with compact controls and contextual panels. This is a layout proposal, not an implemented mockup.

```text
┌ Existing NILO application tabs / connection / theme / language / user ┐
│ Capture name*  Open ▾  Save  Export ▾     Operations (1)  Help         │
│ Device ▾  Discover  Settings  Rate ▾  Length ▾  Trigger  Run / Stop   │
├───────────────┬─────────────────────────────────────────────────────┤
│ Channel rail  │ Time ruler       marker A      trigger      cursor B │
│ SCLK  [T]     │ ─────┐ ┌────┐ ┌────┐ ┌─────────────────────────────  │
│ MOSI          │ ──┐ └─┘    └─┘    └─┘                               │
│ SPI           │ [Address 0x03] [Data 0x7F]                           │
│ Analog        │      /\       /\                                    │
├───────────────┴─────────────────────────────────────────────────────┤
│ Optional drawer: Decoders | Search | Measurements | Saved captures  │
├─────────────────────────────────────────────────────────────────────┤
│ Ready • device/owner • captured length • cursor Δt • fit/follow      │
└─────────────────────────────────────────────────────────────────────┘
```

The channel rail and waveform share vertical scroll position. Keep the ruler and acquisition controls visible. Large channel lists scroll inside the trace area; large tables are paginated/virtualized inside their drawer. Prefer no whole-page scrolling at common desktop sizes, but preserve usable controls at 200% zoom and on small screens rather than forcing everything into an unreadable single screen.

Use one contextual side panel for detailed device/trigger/channel settings and one optional bottom panel for analysis results. Opening a panel should not discard zoom, cursor position or decoder selection. A resizable split view can compare two regions of the same capture later in the implementation sequence.

### Acquisition flow

1. Select or discover an analyzer. Discovery itself can take time: show an operation state, cancel control and results as available. Manual connection accepts driver-specific typed connection settings.
2. Read capabilities and device ownership. Show common controls in the toolbar and less-used settings in a panel, with descriptions, units and current values.
3. Choose channels, sampling, capture limits and supported triggers. Estimate capture duration/storage where enough information is known; show estimates as estimates.
4. Run submits a snapshot of the configuration. Accepted submission returns an operation ID immediately. Show Queued, Waiting for approval, Starting, Waiting for trigger, Acquiring and Finishing only when the service has evidence for those phases.
5. While acquiring, stream progress and bounded waveform previews. Inspection must not stop capture. Stop requests orderly finalization; Cancel follows the separately defined abort contract.
6. Completion opens the durable capture result. An interrupted result is marked partial with its reason and valid extent. Never display an incomplete capture as complete because the browser request succeeded.

Run remains visibly Run or Start capture. If the device is occupied, offer Queue capture with the owner/busy explanation allowed by RBAC. Never silently change the device's settings while another transaction owns it. A viewer can inspect existing captures but cannot configure hardware or start capture.

### Channel and analysis interactions

- Click a channel label to edit its name, color, height and visibility; offer move/group actions with keyboard equivalents. Acquisition enablement is separately labeled “Include in next capture”.
- Trigger badges open supported trigger choices. The detailed panel explains whether thresholds apply to one input or a shared hardware group.
- Add decoder opens a searchable catalog with description, input/output protocols, required/optional channel mappings and typed options. Label-based mapping is a suggestion requiring visible review.
- Decoder annotation tracks align with raw channels. Clicking an annotation opens its complete details and the corresponding table row; selecting a row navigates to that interval.
- Cursors and named markers are directly manipulable and editable numerically. Measurements identify range, source and method.
- Search returns a bounded result list and Previous/Next navigation. Distinct modes cover transitions/pulses/patterns, annotation text/classes and decoder binary payloads. The waveform remains visible while navigating results.
- Filters explain their effect: “Hide unchanged channels” changes presentation; a decoder class filter changes displayed annotations; a derived signal transform creates new data with provenance.

## Saved waveforms and operations are different

**Saved waveform** is the object the user names, for example “SPI memory read — board rev B”. It has a stable capture ID, display name, optional notes/labels, creator, acquisition time, device/configuration snapshot, size, completeness, raw artifacts and one or more analysis documents. Duplicate names are permitted because identity is not the name. Save, rename and delete must use clear user-facing actions.

Proposed retention: completed captures are initially retained as explicitly labeled temporary captures. Save pins them against automatic cleanup and stores a meaningful name; autosaving edits can apply after that. Temporary retention duration and byte quota are appliance settings. Show expiry and allow saving before expiry. Saved captures are not silently deleted to make room; refuse a capture reservation or ask for cleanup when the quota cannot be satisfied.

The library offers name/date/device/label search, sort, pagination, open, rename, export and confirmed delete. Deletion reports related analysis outputs and active readers/jobs. Active dependencies block destructive removal or require orderly cancellation; never leave a decoder reading removed raw data. Saved waveform data, operation history and security audit each have separate retention policies.

**Operation** is asynchronous work: discover, capture, decode, import, export or lifecycle action. It may fail without producing a saved waveform. Show recent operations in a compact drawer with descriptive action, requested name, state, elapsed time and result link. Keep the UUID in details with a copy button. Paginate history and apply a documented retention limit; it must not grow forever.

The current “Deadline” column represents the latest time an operation may remain unfinished, including queue/approval waiting under the current model. It is not a waveform timestamp or an estimate of capture completion. Replace it in the normal view with useful state/elapsed information. Details should explain the overall timeout and, while active, remaining time or expiry. Terminal rows should show outcome and completion time rather than an old deadline.

## RBAC proposal

Backend permission enforcement is authoritative for HTTP, CLI, MCP, events, downloads and GUI. Hiding buttons is only presentation. Preserve the existing identity/token scope intersection and any existing approval policy.

| Action | Viewer | Operator | Administrator |
|---|---|---|---|
| Read installed/enabled plugin metadata | Yes | Yes | Yes |
| Install, enable, disable, uninstall | No | No | Yes |
| Restart, subject to busy checks | No | Yes, new narrow permission | Yes |
| Read authorized saved captures, inspect locally, export allowed data | Yes | Yes | Yes |
| Discover/connect hardware, apply settings, acquire | No | Yes | Yes |
| Submit decode/search/measurement/derived-data jobs | Read existing results; new jobs require execution permission | Yes | Yes |
| Persist capture names, analysis edits and markers | No by default | Own/manageable objects | All authorized objects |
| Delete captures or saved analysis | No | Own/manageable objects, confirmation | All authorized objects, confirmation |
| Cancel operations | No | Own operations | Any authorized operation |
| View security audit Activity | Existing audit permission only | Existing audit permission only | Existing audit permission |

The inspected role model currently gives `plugins.manage` to administrators and `bench.execute` to operators; plugin restart currently shares the administrator-only management permission. Introduce a narrow restart permission instead of granting operators full management. Do not grant broad audit access merely to make an Activity button usable. If an existing role cannot read audit, omit/disable that entry with an explanation and expose only permitted operational status.

Capture sharing/ownership needs an explicit policy before schema implementation. Safe initial proposal: preserve existing owner-scoped visibility with administrator oversight; add explicit sharing rather than making every capture globally readable. **This data-access policy is independent of the appliance-wide plugin configuration.** A viewer may inspect only captures made readable to that identity under the selected policy.

## Consistency and accessibility

Use NILO's existing components, theme tokens and EN/DE/RO localization. Plugin short name is “Sigrok”; its description explains signal capture and protocol analysis. The NILO project version remains canonical under repository versioning rules. Show dependency/build versions separately; do not invent an independent component release scheme.

Every icon action has an accessible name and tooltip. Explain unfamiliar units and scope through short help text; do not require hover for essential information. Status includes text/icons as well as color. Channel colors remain distinguishable in light and dark modes and may be customized without losing label contrast. Canvas tracks need keyboard navigation, focused-channel information and accessible numeric/tabular alternatives. Provide visible errors linked to the affected setting and preserve edits after validation failures.
