# Pi webcam feasibility record

Observed on 2026-09-24 with the FaceCam 1000X (USB `0458:708f`) attached to the Raspberry Pi 4. `/dev/video0` is the capture endpoint and `/dev/video1` is metadata only. Both pinned candidate bindings captured MJPEG and YUYV, changed and restored a typed control, and passed the bounded cancellation check. The automated cell remains `needs_review`; this record alone does not select a production binding.

`evidence.json` is the unedited schema 2 probe output and `wheels.json` gives the checked local wheel digests. The isolated probe environment installed only from those wheels. `pyrav4l2` wheel version 1.0 was built from Antmicro commit `9c0beeb9a8a6941e8e0dbc6f65830b8d713e0bdf`; `linuxpy` was pinned at 0.24.0 and Pillow at 11.3.0. The wheel files are local under ignored `02_Tools/runtime/camera-probe/wheelhouse/` and were also transferred to the Pi's private probe directory.

The Pi's Debian 13 arm64 profile differs from the probe guide's older Raspberry Pi OS wording, which the probe explicitly recognises. The HDMI dongle and Ubuntu mini PC cells remain `not_run`. Review licence terms and the full packaged runtime closure before approving a binding.
