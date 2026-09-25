# Plugin administration and virtual bench workflow plan

Status: deferred; this Sigrok workflow proposal is outside the current Camera
navigation work and will be reconsidered during Sigrok reimplementation. No
implementation or deployment is authorized by this document. The current
Camera navigation and appliance administration plan is
`00_Planning/Plugin_Navigation_and_Administration_Plan.md`.

## 1. Intent and evidence

STATED: installed plugins belong under System > Plugins; enabled plugins also
belong under Bench. Each plugin has a short name, description and version.
Administrators enable/disable and uninstall; operators may restart. Uninstall
requires confirmation. Preserve the existing help, languages and themes, add
contextual Activity, and make routine work fit one screen where possible.
Captures need meaningful names and an understandable way to remove saved work.

OBSERVED: the supplied screenshot shows lifecycle controls, capture settings,
transaction history and expanded results stacked on one page. The repeated
Plugins heading and large lifecycle card consume the working area.

OBSERVED in the current repository:

- `05_Software/nilo_gui/src/components/pages/PluginsPage.tsx` combines all four
  concerns, polls every 1.5 seconds, displays UUIDs and absolute deadlines, and
  offers uninstall without a confirmation dialog.
- `05_Software/nilo_gui/src/components/shell/pages.ts` derives instrument pages
  from the older integration inventory, separately from managed plugin state.
- `05_Software/nilo_assistant/internal/bench/packages.go` and
  `internal/httpapi/bench.go` require `plugins.manage` for every lifecycle action,
  including restart. Managed inventory includes available catalog entries as
  well as installed packages; submenu membership must filter `installed`.
- `internal/bench/jobs.go` stores acknowledged operations and returns only the
  newest 100 accessible jobs. It has no pagination, naming or deletion methods.
  Older jobs remain stored. The 32-command limit bounds pending/active work,
  not total history or total disk use.
- `internal/bench/model.go` and `jobs.go` define deadline as submission time plus
  overall timeout; approval, queue and execution consume that allowance.
- `internal/auth/role.go` grants full Activity access only to administrators.
  Job reads are owner-scoped, with administrator access through `plugins.manage`.
- `06_SoftwareDocu/Development_and_Simulation_Flow.md` already specifies compact
  layouts, accessible help, EN/DE/RO, themes, and development contract replacement.

INFERRED / RECOMMENDED below: specific layouts, capture metadata, storage policy,
new permission names and implementation sequence. These are proposals for this
iteration, not claims about existing behavior.

## 2. Navigation and plugin identity

| Destination | Children and purpose |
|---|---|
| Bench | Enabled installed plugins, initially Sigrok; opens the working bench overview |
| Bench > Sigrok | Discover, configure, capture and inspect saved results |
| System > Plugins | Installed/available package overview and administrator installation entry point |
| System > Plugins > Sigrok | Plugin status, version, policy and lifecycle administration |

Remove the separate Instruments cluster for these managed plugins. Derive both
groups from one authoritative managed-plugin inventory, shared by shell and pages.
Keep the existing System order: Plugins, Network, Authentication, Activity, About.

Use stable plugin IDs for routing, separate page IDs for work and administration,
short sidebar label `Sigrok`, and distinguish open tabs as `Sigrok` and
`Plugins / Sigrok`. Add metadata to the curated package descriptor and API:
short display name, localized description with fallback, and project release
version. Do not introduce independent component versioning: display the version
of the installed package from the NILO release it was built with.
Suggested description: "Logic analyzer capture and protocol decoding."

An available catalog package is installable from the Plugins overview but gets
no installed submenu. Successful install adds the System child, initially disabled;
successful enable adds its Bench child. Disable removes only the Bench child;
uninstall removes both children after server success. Keep unhealthy enabled
plugins visible with a clear status so they can be diagnosed.

Reconcile open tabs and selection after lifecycle changes, including changes
from another browser, CLI or MCP. Close obsolete administration/work tabs and
focus the appropriate parent; preserve access to running-job status and saved
results from the Bench overview. Failed actions leave navigation intact.
On inventory failure, indicate stale state and offer retry; do not represent
missing data as a successful uninstall or lose the user's capture form.

STATED / CONFIRMED: plugin configuration and enablement belong to the NILO
appliance. Each appliance has one configured set of installed and active plugins,
shared by all connected users and GUI/CLI/MCP clients. Administrators change that
appliance configuration; RBAC determines which actions each user may perform.
Bench lists the appliance's active plugins, not a personal or project selection.
A project label on a capture is descriptive metadata only. Independent benches
and per-user/project plugin activation are outside this product scope.

## 3. System > Plugins > Sigrok

Use a compact toolbar beneath the existing workspace tab, without another large
Plugins heading. Show version, enabled/disabled/starting/error status, help and
an Activity button. The short description is available through accessible help.

The main controls are Enable/Disable, Restart and Uninstall. Keep operation
approval policy (`Ask` / `Auto`) here, labelled "Approval for operations", with
an explanation. Administrators control it. Show Restart to operators separately
from administrator controls; viewers see read-only status.

Uninstall opens the established confirmation dialog: name/version, affected
plugin, and explicit statement that saved captures and audit history remain.
Cancel is safe and restores focus; confirm disables duplicate submissions and
reports the actual result. Keep reinstall possible from the available catalog.
Do not add capture deletion as a hidden side effect of uninstall.

Restart/uninstall refuse while the plugin has pending or executing work, with a
plain reason and link to that work. Do not give operators a force-restart path.
Disable keeps its existing contract: block new work, cancel queued/approval work,
let executing work finish. Explain that consequence next to the action when busy.

Activity opens the existing Activity page filtered to this plugin, including
its operations. Use persistent plugin/transaction correlation in audit records;
filtering only on the broad `bench` component is insufficient. Apply the same
filter to paging and export. The Activity button follows `activity.read`; do not
grant operators access to the appliance-wide audit simply to show this button.

## 4. Bench > Sigrok: everyday work

Make configuration and results the main content, using the existing design tokens,
controls and help components. No package-management card on this page.

Recommended desktop arrangement:

| Region | Content |
|---|---|
| Compact toolbar | Analyzer selector, Discover analyzers, connection status, Help, permitted Activity |
| Configuration pane | Capture name, optional project/notes, sample rate, sample count, D0-D15, Capture |
| Results pane | Selected capture preview and tabs for Saved captures / Recent operations; detail replaces list content |
| Active-work strip, only when needed | Waiting for approval / Queued / Capturing, remaining time, permitted Approve/Cancel |

Put overall timeout under Advanced as "Maximum wait", with persistent default
summary and help: includes waiting for approval, waiting for the analyzer, and
execution. Show sample duration derived from samples / sample rate next to the
capture settings: it is different from maximum wait.

Use `Capture` as the action label. Immediately acknowledge the accepted request
and show its actual state. In Ask mode say "Waiting for administrator approval";
an operator cannot bypass it. A long approval wait can still time out. Discovery
is explicit and bounded, reports no-device/busy/disconnected/error states, and
does not silently start acquisition. Keep D0-D15 only; PWM stays excluded.

Retain the form while inspecting results or polling. Changing a selection must
not clear the user's name or project. Validate channels/rates when devices change.
UART decoding remains available from an eligible saved capture's detail; generic
project labels such as "SPI memory-read test" do not imply a new SPI decoder.

Active status remains reachable if another administrator disables the plugin.
Closing a tab, navigating or losing the HTTP response does not cancel the job.

## 5. Names, saved results and deletion

Use three distinct concepts:

- Operation: the asynchronous command with its immutable transaction ID, owner,
  parameters, state, timeout and replies. It can fail without producing a capture.
- Saved capture: the successful acquisition artifact and its human metadata.
  Save automatically after successful acquisition; show a clear saved indication.
- Audit activity: the accountable history of actions, with its own retention.

Allow a name before acquisition and Rename afterward. Default to a readable
date/time name when blank; never require a UUID to recognize a capture. Allow
an optional project label and notes, searchable with the name. Proposed limits:
name 120 characters, project label 120, notes 2,000; Unicode, plain text, no HTML.
Duplicate names are allowed; timestamp/device distinguish them. Metadata changes
do not rewrite acquisition parameters, the transaction ID or artifact digest.
Keep submitted identity/idempotency immutable when the display name is edited.

Saved captures lists Name, Created, Analyzer and Actions, with project shown as
secondary text or a filter. Open detail to preview, download, rename, decode or
delete. Put UUID, exact timing, raw replies and technical errors under Details.
Recent operations shows failures and cancellations as well as successes, using
readable names/statuses; avoid calling every entry a saved sample.

Provide search, plugin/project/status/date filters and server-side pagination
(proposed 25 per page, maximum 100). Older records must remain findable; limiting
the response to the newest 100 is not retention. Keep the active queue separate
so paging through history cannot hide pending approvals or running captures.

Provide explicit Delete for an owned completed result, or any result for an
administrator, using a confirmation that names the capture and states whether
the file and derived results will be removed. Never delete active work; offer
Cancel and wait for terminal cleanup. When derived results exist, list the
dependents and require explicit inclusion or refuse deletion; no silent cascade.
Reject deletion while decoding/downloading holds a protected reference.

Recommended first policy: saved captures remain until explicitly deleted;
no automatic age-based deletion of a named electronics measurement. Add visible
storage usage and an administrator-configured storage budget, with admission
backpressure when full, rather than silently deleting evidence. Account for
artifacts, verbose replies/results and metadata, including concurrent reservations.
Determine the default budget during specification from the appliance edition.
Deleting a capture must actually reclaim its payload after safe cleanup, not
merely hide its row. Report deletion/cleanup failures truthfully and recover after
restart without orphan files or an apparently deleted readable artifact.

Keep minimal transaction provenance and idempotency tombstones after payload
deletion, so a late retry cannot recreate a deleted capture or execute twice.
The tombstone response explicitly says the result was deleted. Full audit and
minimal operation records have separate documented retention; do not claim
that deleting a capture erases every trace or that pagination bounds disk use.
Metadata counts toward the storage budget too. An automatic history-retention
policy or a finite retry window must be separately specified before pruning it.

Saved data remains accessible through a Saved captures view within the Bench
overview when its plugin is disabled/uninstalled. Do not keep a disabled plugin
under Bench just to provide access to historical files.

## 6. Deadline explained and proposed presentation

Today, submission at 10:00:00 with a 60-second timeout gives deadline 10:01:00.
If approval/queueing consumes 45 seconds, at most 15 seconds remain for execution
under the overall budget. The backend also has its existing bounded runner wait.
It is not a retention/deletion date and not the electrical sampling duration.

Remove Deadline from the main history table. Show remaining wait time only for
active work and exact timestamps under Details. Completed work shows capture
time and outcome. Translate technical states, including "Interrupted" for
`incomplete` and a clear timeout reason; do not present restarted unfinished jobs
as completed or replay them automatically.

## 7. RBAC and all three interfaces

| Capability | Viewer | Operator | Administrator |
|---|---|---|---|
| Read installed/enabled plugins and help | Yes | Yes | Yes |
| Install, enable/disable, change approval policy | No | No | Yes |
| Restart an idle enabled plugin | No | Yes | Yes |
| Uninstall, after confirmation | No | No | Yes |
| Discover, capture, decode | No | Yes, subject to policy | Yes, subject to policy |
| Read/download results | Own authorized records | Own records | All records |
| Rename/delete terminal results | No | Own records | All records |
| Cancel active work | No | Own work | Any work |
| Approve operations in Ask mode | No | No | Human browser session only |
| Full plugin audit activity/export | No | No | Yes, by existing permissions |
| Configure retention/storage limits | No | No | Yes |

Viewer access to existing owned records matters, for example, after demotion
from operator. No new cross-user sharing or shared project ACL is implied.

Add a narrow `plugins.restart` permission for operators and administrators;
keep `plugins.manage` administrator-only. Add a result-management permission
for operators/admins with server-side owner checks, and a separate storage-policy
permission for admins. Resolve exact names during specification. Enforce these
in the shared bench service, HTTP routes, MCP tools and CLI, not just the GUI.
Token scopes can only narrow the owner's current role. Test revocation and role
changes while work or a confirmation is pending; a token cannot approve a job.

Expose naming/listing/deletion coherently through HTTP/CLI/MCP. GUI uninstall
and result deletion use confirmations; CLI uses an explicit confirmation flag
for noninteractive use. Define an explicit confirmation contract for destructive
MCP calls as well, without requiring a browser for an authorized agent action.
Confirmation supplements RBAC and never grants permission.

## 8. Concurrency and live state

Retain durable acknowledgements, IDs, bounded command/reply queues, deadlines,
device leases, cancellation cleanup and no replay on restart. Capture names are
metadata, not locks or idempotency keys. A pending submission retains the same
request key on retry; a later user-requested capture gets a new key.

Use a shared inventory refresh after acknowledged mutations and bounded polling
for changes from other clients. Prevent late responses from repopulating an
uninstalled plugin or overwriting newer selection/metadata. Use revision checks
for conflicting edits/deletes and idempotent deletion outcomes.
Serialize lifecycle against admission in the backend. A click becoming invalid
before it reaches the server produces a useful conflict response, not an unsafe
restart or a phantom navigation update. Keep slow discovery/lifecycle calls
visibly pending with bounded waits; never report success merely because HTTP
timed out. Reconcile from authoritative state before a retry.

## 9. Consistent compact presentation

Reuse HelpPopover, WorkflowHelp, established confirmation dialogs, existing
Activity navigation and theme tokens. Tooltips/help work with keyboard and touch;
essential status, refusal and safety information stays visible. Do not rely only
on hover or HTML title attributes. Names remain visible when the sidebar narrows.

Use EN/DE/RO translations for all labels, explanations, statuses, confirmations,
empty states and permission reasons. User names/project notes remain verbatim.
Light/Dark/System follow the existing settings without adding a new visual style.

At 1920x1080 and 1366x768, target one viewport for routine configuration and a
selected result, with a bounded list/detail region. At 1024x600 and 800x480,
use configuration/results tabs or a full-screen detail sheet and the existing
navigation drawer. Allow local scrolling where necessary; never clip controls,
shrink text below 14 px or violate the established 44 px field touch targets.
Validate 200% zoom, long German/Romanian labels, keyboard focus and error states.
Remove the duplicate Plugins title row while retaining a programmatic page name
and all necessary toolbar actions. This iteration applies to plugin/bench pages,
not an unsolicited redesign of unrelated pages.

## 10. Implementation sequence and acceptance

1. Specify product behaviors and update linked navigation, plugin lifecycle,
   access, saved-evidence and shared-interface requirements. Keep repository
   tooling out of product requirements. Record the agreed enablement scope,
   metadata ownership, deletion semantics and storage policy.
2. Implement plugin identity metadata and shared inventory, with the narrow
   restart permission across core/HTTP/CLI/MCP. Add the System and Bench child
   routes and reconcile lifecycle changes across open pages.
3. Split the current PluginsPage into package overview, plugin administration
   and Sigrok working views; reuse common controls rather than duplicate them.
   Add confirmation, scoped Activity, help, translations and responsive layout.
4. Implement saved-result metadata, searchable paginated reads, safe deletion,
   storage accounting and preserved correlation. Add shared transport contracts,
   then the named capture/history/detail interface and off-plugin history access.
5. Verify with focused core/contract tests, component tests and real WSL2 browser
   workflows. Cover owner/admin access, direct forbidden calls, operator restart,
   confirmation cancellation, multiple clients, busy lifecycle, delete/decode
   races, timeout, reconnect, idempotent retries, disk limits and interrupted
   cleanup. Check all target viewports, languages, themes and keyboard flows.
6. Complete required repository validation, then perform separately authorized
   Pi deployment and GUI/integration acceptance. Driver/signal correctness stays
   outside this iteration, as previously agreed. Never push.

Development compatibility policy: replace superseded development UI/API contracts
directly where necessary; do not add aliases or migrations solely to preserve an
old development build. A database schema change must preserve unrelated appliance
identity/network data. Confirm whether existing Pi captures need preservation
before any future reset of feature-owned data; no reset is authorized by this plan.

Definition of done: an administrator can install/enable Sigrok and see the two
correct destinations; an operator can restart it when idle and capture named
work; saved results can be found, renamed, downloaded and explicitly deleted;
disabling/uninstalling updates every client without losing saved-data access;
RBAC and asynchronous behavior agree across GUI, CLI and MCP; the ordinary bench
workflow fits the available screen while remaining accessible.
