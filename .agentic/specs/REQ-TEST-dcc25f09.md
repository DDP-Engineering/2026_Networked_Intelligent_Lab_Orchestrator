# REQ-TEST-dcc25f09 — Prove camera binding on target architectures

## Behavior
One bounded, non-production probe is run on Raspberry Pi OS/Debian 13 arm64 and Ubuntu 24.04 amd64 with a UVC webcam and an HDMI-to-USB capture device. Its records compare candidate Python V4L2 bindings and Pillow integration. A dependency is selected only after all four target/device combinations demonstrate physical-node grouping, MJPEG and YUYV capture where advertised, typed control enumeration/set/read-back, cancellation, warm-up and offline dependency closure. Unsupported hardware capabilities are recorded with an observed reason; they are not treated as failed mandatory features unless the package would need them.
## Data / interface
Each record identifies target OS, architecture, kernel, device vendor/product/serial/port, endpoint candidates, selected endpoint, advertised/attempted formats, controls and typed failures, per-step duration, cancellation outcome, frame quality, Python/binding/Pillow versions, native closure and locked wheel hashes. The probe returns a nonzero exit for a mandatory failure and writes raw machine-readable evidence plus a human summary. It never asserts target success from WSL or simulation.
## Examples
A camera with a metadata node and a capture node records both and selects the capture node. If a candidate binding cannot cancel a blocked frame read within the operation deadline, that candidate is rejected.
## Acceptance
Manual: run the same versioned probe and acceptance matrix on both target machines; compare raw records and wheel closure before selecting the binding.
## Assumptions and open decisions
No target hardware is connected in this workspace. Candidate shortlist, final binding, observed unsupported capabilities and dependency closure remain open until the target runs. This requirement remains draft until then.
