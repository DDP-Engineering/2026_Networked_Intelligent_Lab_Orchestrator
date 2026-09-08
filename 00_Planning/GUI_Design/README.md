# GUI design references

Preferred direction recorded on 2026-09-02: the second and third examples
from the revised design exploration. These are complementary visual references,
not two theme-locked products or screenshots of implemented functionality.

## Examples

- [Instrument workspace, dark theme](02_Instrument_Workbench_Dark.png): compact
  network summary opened over the active instrument.
- [Network page, light theme](03_Network_Overview_Light.png): interface details
  and theme selection, corrected to put Network in the lower sidebar cluster.

## Agreed layout

- NILO and open page/instrument tabs share one compact top row. Do not add a
  separate tab strip or duplicate page-title row above an instrument.
- Keep the central workspace as large as possible for plugin/instrument views.
  Navigation is clear, labelled, and on the left; avoid decorative backgrounds
  and large connection-status panels.
- The upper navigation area contains the working/instrument entries. The lower
  core-management cluster is **Plugins, Network, Authentication, Activity, About**.
  This supersedes the order agreed on 2026-09-02, Network, Plugins,
  Authentication, Settings, About: Activity joined the cluster on 2026-09-04,
  and on 2026-09-05 the order was changed and the unimplemented Settings entry
  removed (TASK-17d71acd, REQ-UX-30821068).
  Network is a built-in NILO capability, not an integration. Plugins here means
  plugin management, not an individual instrument.
- Provide a compact network indicator/summary and a dedicated Network page.
  Show the NILO appliance's VPN, LAN, and Wi-Fi state, interface IP addresses,
  link speed, and current traffic where available. Distinguish link capacity
  from measured RX/TX traffic and VPN state from remote endpoint reachability.
- Support **Light, Dark, and System** themes in the same layout. System follows
  the user's OS/browser preference. Keep language selection in the top row.
- Prefer one-monitor layouts with bounded internal scrolling when needed,
  rather than document-level scrolling or clipped controls.

## Applied

The direction above was implemented in the browser interface on 2026-09-02
(TASK-a1b5a675). Two decisions in the images were deliberately not reproduced,
because the written decisions and the repository's own requirements override
incidental image detail:

- every address, link speed and RX/TX figure is demo data, and Bench Core
  reports no network telemetry at all, so the Network page and the compact
  summary state that the information is not yet available rather than showing
  an illustrative value as measured;
- neither image carries the physical-equipment boundary statement that
  `REQ-UX-188a4860` requires on every screen, so the shell keeps it in a slim
  strip under the workspace.

The illustrated Logic Analyzer, USB Camera and Flash Tools instruments were not
created. The instrument cluster lists the integrations the backend actually
reports, which today is the simulated fake Python integration.

## Status and provenance

Saved design references only; no application implementation, live telemetry,
hardware connection, plugin embedding capability, or production acceptance is
implied. All illustrated instruments, addresses and traffic values are demo data.
Image details do not override the decisions above.

Both concepts were produced with the built-in image-generation tool. The dark
reference is preserved from the second displayed revised example. The light
reference is an image edit of the third displayed revised example, with only
the sidebar Network placement requested to change. No final preference between
the two visual treatments was required to save these references.

### Light-reference revision prompt

Edit the attached NILO Network UI mockup with ONE precise layout change. Target dimensions: retain the attached image dimensions and aspect ratio, desktop app screenshot. Move ONLY the selected Network navigation entry in the LEFT SIDEBAR from its current position directly below Flash Tools into the LOWER navigation cluster, immediately ABOVE Plugins. Upper cluster remains Bench, Instruments, USB Camera, Flash Tools. Lower cluster must be a compact contiguous group ordered Network (selected), Plugins, Authentication, Settings, About, anchored near the bottom, with a LARGE flexible empty gap between Flash Tools and Network. Network is a core NILO function, not an integration. Remove the old Network row entirely so it appears only once in the sidebar. Keep all other visual content unchanged: NILO wordmark and tabs on the same top row; selected Network top tab; header controls; open Appearance menu with Light, Dark, System; EN language selector; central interface table, IP addresses, traffic graph, labels, spacing, colors and typography. No new pages, controls, labels, captions or decorations. Produce one revised image, not a collage.
