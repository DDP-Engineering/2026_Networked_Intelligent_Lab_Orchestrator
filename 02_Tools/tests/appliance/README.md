# Appliance verification stage

Runs Bench Core in the configuration an appliance ships and proves it against a
real network.

```sh
./02_Tools/tests/appliance/check.sh
```

Linux, natively or inside the WSL2 distribution a Windows checkout prepares.
A container engine is required. A Windows-native invocation, a missing engine,
and an engine whose daemon does not answer are each named and refused with
exit 2; they are never reported as a pass or a skip.

## What it stands up

Inside one disposable container, on an engine network this script creates and
removes:

- `dbus-daemon` and a real `NetworkManager`, with this container's own `eth0`
  configured at `192.0.2.2/24` and `fd00:10::2/64`;
- the real `nilo-nethelper` as root, on a socket in the run's temporary root;
- Bench Core as an unprivileged service account, started with
  `--profile appliance`, TLS material minted for the run, a built interface
  directory and that helper socket;
- Chromium in a dedicated `nilo-browser` network namespace, connected through
  a veth pair to the appliance management interface so browser traffic crosses
  the production listener and nftables admission boundary;
- WireGuard userspace tools and read-only evidence for the running kernel's
  modules, allowing the production profile lifecycle to be exercised.

Every adapter the stage reports is one NetworkManager detected in that
namespace. Nothing is simulated, and no certificate or credential outlives the
container.

## What it proves

`05_Software/nilo_assistant/cmd/nilo/appliance_stage_linux_test.go`:

- the pairing starts and Bench Core reports the HTTPS address it listens on,
  refusing an unauthenticated request and answering an authenticated one;
- Bench Core's adapter list is a subset of what `nmcli` reports, the configured
  adapter carries the address NetworkManager assigned it, and no reading is
  attributed to a simulated source.

The stage also builds the GUI and runs all Playwright journeys. Those journeys
bootstrap a real administrator over the CLI standard input, sign in through the
appliance HTTPS origin, exercise authentication, shell and field layouts,
perform real NetworkManager changes and rollback, query real network samples,
and import, enable, disable and remove a real WireGuard profile. The fresh
appliance correctly begins with empty integration and device inventories.

The browser suite may not be run directly with `npm run test:e2e`: its
prerequisite check requires this stage's appliance binaries, browser namespace,
management interface and built GUI. It keeps no trace, video or screenshot by
default and removes its `/tmp/nilo-e2e-*` state on success and failure.

The host's own interface list and default route are recorded before the run and
compared after it, on every exit path. A run that changed them fails even when
the stage itself passed.

## Relationship to the other container runs

| Run | Image | Purpose |
|---|---|---|
| `02_Tools/tests/network/check.sh` | `network-system` target | privileged helper and clean WireGuard-install system tests |
| `02_Tools/tests/appliance/check.sh` | `appliance` target | this stage, including the browser suite |
| `02_Tools/env/with-build-environment.sh test` | locked build image | build, lint and unit gate, `--network none --cap-drop all` |

The first two share one multi-stage image definition
(`02_Tools/tests/network/Dockerfile`), one NetworkManager bring-up
(`02_Tools/tests/network/network-up.sh`) and one Go cache volume, because they
build the same module graph from the same checkout. The `network-system` target
deliberately has no WireGuard tools so the install test begins clean; the
`appliance` target adds WireGuard, Node.js, locked GUI dependencies and Chromium.
They need `NET_ADMIN` and `SYS_ADMIN`, which is why they are separate from the
locked build image and its dropped-capability posture.

Bench hardware acceptance on a Raspberry Pi or mini PC remains the higher
verification level; this stage does not replace it.

## Image contents

`02_Tools/tests/appliance/inventory.json` lists what the image must carry and
how to probe each component. The Go toolchain is deliberately absent from that
list: it comes from the repository's pinned Linux runtime through the read-only
mount, via `02_Tools/env/with-go.sh`.
