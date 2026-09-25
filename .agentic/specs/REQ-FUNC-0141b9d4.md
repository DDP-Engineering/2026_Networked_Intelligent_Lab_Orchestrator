# REQ-FUNC-0141b9d4 — Describe bounded camera capabilities and operations

## Behavior
Discovery returns one record per physical camera, with every candidate video endpoint, one selected capture endpoint or a reason none is usable, supported formats and typed control descriptors. Metadata-only endpoints remain visible but cannot be selected for capture. Integer, Boolean and menu controls may be written when the descriptor permits; other valid types remain read-only with a reason. Snapshot, get_controls, set_controls and reset_controls are the only phase-1 device operations.
## Data / interface
Each control descriptor includes stable id/name, display name, V4L2 type, current/default value, range or menu, step, unit when known, read-only, inactive, volatile and automatic/manual dependency flags. A snapshot request may choose a discovered format, size and frame interval, typed control overrides, processing, and JPEG/PNG encoding. Processing permits zoom 1..8 with normalised centre, quarter-turn rotation, two flips, grayscale, invert and at most 32 overlays of crosshair, grid, line, rectangle, text or scale bar. Text is at most 120 characters. Plugin limits cap width, height and decoded pixels. The runner rejects undiscovered formats, out-of-range controls and unbounded images.
Rendering order: capture; invariant mounting correction; crop/zoom; request rotation/flips; grayscale/invert; output resize; overlay projection; encoding. Overlay coordinates are normalised after mounting correction and before zoom. Manual calibration applies to scale bars after crop and output resize. Control changes persist until a configured or factory reset; automatic controls are ordered before dependent manual controls. Warm-up discards a bounded count or waits a bounded settling interval within the operation timeout, then reads actual control values back.
## Examples
A metadata node plus one MJPEG capture node yields one camera with one usable endpoint. A request for a menu choice absent from the discovered descriptor returns control_out_of_range.
## Acceptance
Isolated V4L2 test doubles cover endpoint grouping, transforms, bounds, controls and warm-up. Pi hardware evidence covers discovery and a real capture. Other target behavior remains an acceptance check.
## Assumptions and open decisions
Exact endpoint capability representation and binding-specific unsupported format reasons await hardware evidence.
