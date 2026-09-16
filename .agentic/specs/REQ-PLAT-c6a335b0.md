# REQ-PLAT-c6a335b0 — Classify Raspberry Pi recovery button presses

## Behavior

Raspberry Pi hardware recovery is an explicit install-time feature. Its
reference circuit connects BCM GPIO17 to ground through a normally-open
momentary button and uses the SoC internal pull-up. BCM GPIO27 drives a status
LED active-high through a current-limiting resistor. GPIO line access uses the
Linux character-device interface and does not require CGO.

Input transitions are debounced for 50 milliseconds. Duration is measured from
the first stable pressed state to the stable released state. Action occurs once
on release: a stable press below 3 seconds is `identify`; 3 seconds inclusive
to below 10 seconds is `recovery`; 10 seconds inclusive and above is
`commissioning_reset`. Crossing 10 seconds while still held selects the
reset-pending indication but does not execute reset before release.

The LED patterns are:

- idle: off;
- identify: 200 ms on/200 ms off three times, then 800 ms off, repeated for a
  total ten-second window;
- recovery armed: 500 ms on, 500 ms off;
- recovery active: continuously on;
- reset pending: 125 ms on, 125 ms off;
- failure requiring recovery retry: two 200 ms flashes followed by 1.4 seconds
  off.

The durable core recovery state selects the normal pattern after restart. A
currently held >=10-second button temporarily overrides it with reset-pending.
If the helper loses GPIO access, networking stays unchanged and the failure is
reported; it does not emulate a button event.

On every recovery-duration release, the helper captures the UTC release time and a
sorted set of stable identities for connected physical Ethernet adapters. An
unidentified connected adapter marks the snapshot invalid. The event is stored
before core can observe it; a reset supersedes a pending recovery request and a
pending reset is never weakened by a later recovery-duration press. A GPIO read
failure invalidates the gesture in progress; sampling recovery establishes a
new baseline only after a stable released sample. A continuously held input
after the read failure cannot become a fresh press; only a later debounced
press may emit an action.

## Data / interface

Installation enables the feature only with a complete validated configuration:
GPIO chip, button line 17 active-low with pull-up, LED line 27 active-high, 50 ms
debounce, 3 s recovery threshold, 10 s reset threshold, and 10 minute recovery
window. Partial configuration, identical input/output lines, thresholds out of
order, or unsupported bias/output capabilities are refused.

Generic Linux packages and simulation omit the configuration and open no GPIO
device. The helper's typed event/status interface is described by the recovery
coordinator specification; no GPIO device path is accepted from an unprivileged
request.

## Examples

A contact shorter than the debounce window emits no action. A stable
2.999-second press emits identify. A stable 3.000-second press emits recovery.
A stable 10.000-second press shows reset-pending while held and emits
commissioning reset only after release. Contact bounce inside the 50 ms window
emits nothing extra.

## Acceptance

- Boundary and bounce tests use an injected clock and fake GPIO lines.
- Linux ARM64 and x86_64 builds remain CGO-free; x86_64 starts without GPIO.
- Each identify/recovery state produces the exact pattern above with
  reset-pending and failure taking precedence over Identify.
- Installation documentation includes numbering, polarity, pull, resistor,
  permissions, and a warning to power down before wiring.
- Available wired Pi testing records observations without presenting the
  personal bench as certified acceptance.

## Operational boundary

Physical duration and visible-pattern acceptance requires a powered-down wired
button/LED circuit. The current DDP Pi capability observation proves GPIO17 and
GPIO27 acquisition and release only; it is not certified electrical acceptance.
