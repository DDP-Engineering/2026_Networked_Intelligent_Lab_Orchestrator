# Camera V4L2 feasibility probe

This is the CAM-00 non-production probe. It compares `linuxpy` and `pyrav4l2`
on one selected physical USB camera at a time. It collects sysfs grouping,
candidate endpoint and format observations, typed control set/read-back with
restoration, MJPEG/YUYV captures, Pillow decoding, warm-up, cancellation timing
and a supplied offline wheel inventory. It never chooses a production binding.

## Local checks without hardware

```sh
python -m unittest discover -s 02_Tools/tests/camera -p test_probe.py -v
```

The fixture test creates schema 2 `evidence.json` and `summary.txt` in a temporary
directory. Fixture evidence always has `mode: fixture`, `target_eligible: false`
and four `not_run` acceptance cells. Windows or WSL fixture success is not
Raspberry Pi or mini PC acceptance.
WSL is never recognised as the Ubuntu appliance target, including when a USB
camera is forwarded into it.

## Target runs

Run in an isolated, preinstalled Python environment on Raspberry Pi OS based
on Debian 13 arm64 on a Raspberry Pi 4, then Ubuntu 24.04 amd64. Install both
candidate bindings and Pillow from an independently prepared, pinned offline
wheelhouse. Do not install them system-wide or resolve packages during a probe
run. Provide a lock file with exactly the wheel filenames and SHA-256 digests
in that wheelhouse:

```json
{"wheels":{"example-1.0-py3-none-any.whl":"<64 lowercase hex digits>"}}
```

Connect a UVC webcam and an HDMI-to-USB capture device in turn. Use the video
capture node for each selected physical USB device. Check the physical device
class yourself; `--kind` records the operator's classification.

```sh
python3 02_Tools/tests/camera/probe.py \
  --device /dev/video0 --kind webcam \
  --wheelhouse /path/to/pinned-wheelhouse --lock /path/to/wheels.json \
  --output /path/to/private-evidence/pi-webcam
```

Repeat with `--kind hdmi` and the HDMI dongle on the Pi, then both devices on
the Ubuntu mini PC. The probe accesses only the USB parent of the selected
node, but its control check changes one reversible value and restores the
original in `finally`. If restoration fails, mark that candidate failed and
restore the device manually before any other test. It does not need root when
the account already has the intended video-device permission.

One run can mark only its target/device cell `incomplete` or `needs_review`;
the other three remain `not_run`. `target_eligible: true` means the automated
checks and supplied wheel hashes warrant manual review. It is not hardware
acceptance. Review the raw record against `evidence.schema.json`, check the
actual USB identities, inspect unsupported formats and controls, verify the
binding and Pillow versions and licences, and prove the complete offline/native
runtime closure before selecting a binding. Preserve all four records and the
`probe_sources` SHA-256 digests with the task's acceptance evidence; require the
same digests across all four runs. No real target machine is
connected in this workspace, so the four cells currently remain unobserved.

The command returns 0 for a fixture or a hardware record ready for manual
review, 3 for an incomplete hardware record (after writing its evidence), and
2 for invalid input or a report-write failure.

The candidate APIs were implemented against the maintainers'
[linuxpy video guide](https://tiagocoutinho.github.io/linuxpy/user_guide/video/)
and [pyrav4l2 source](https://github.com/antmicro/pyrav4l2). Their behavior on
the selected target hardware still requires the runs above. `linuxpy` advertises
a GPL-3.0-or-later licence on [PyPI](https://pypi.org/project/linuxpy/); its
redistribution decision is open.
