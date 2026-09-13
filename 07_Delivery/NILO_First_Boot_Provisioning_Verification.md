# First-boot management provisioning verification

## Candidate and scope

This report covers the first-boot provisioning candidate for issue #6 in
workspace `F:\NILO_first_boot_provisioning`, branch
`ddp/nilo-first-boot-provisioning`, based on `master` commit
`0c94a56` and building on the exclusive adapter-role work delivered there.

The feature was built as seven governed tasks, each independently validated:

| Work | Task | Requirements |
|---|---|---|
| Eligible-interface classification | `TASK-2873801a` | `REQ-FUNC-7ae21313`, `REQ-PLAT-18084875` |
| Commissioning lifecycle state | `TASK-ac1e9003` | `REQ-DATA-72876d18`, `REQ-REL-d9bfd8ae` |
| First-boot serving | `TASK-20c64f67` | `REQ-FUNC-243c38a6`, `REQ-SEC-a1012bc5`, `REQ-PLAT-b1abf074` |
| Link-local addressing | `TASK-799d62a6` | `REQ-FUNC-9a19ad36` |
| Local-name discovery | `TASK-878ab570` | `REQ-FUNC-554b9bbd`, `REQ-SEC-83deb5c6` |
| Completing commissioning | `TASK-bd56e123` | `REQ-FUNC-b49d7866`, `REQ-REL-fa990acb` |
| First-boot browser workflow | `TASK-38d62913` | `REQ-UX-f9c2846a`, `REQ-UX-5a75d710` |

Nothing was pushed. No production certificate, Wi-Fi credential or WireGuard
credential was modified. No hardware appliance was changed by this candidate.

## Implemented contract

- A clean installation records the lifecycle state `provisioning`; an
  installation carried forward from an earlier release records `commissioned`.
  The seeding rule reads whether the database already held an appliance identity
  when the migration ran, which covers interface-managed and legacy `--bind`
  deployments without detecting either separately. An upgrade never re-enters
  first-boot provisioning. An absent or unrecognized recorded state stops the
  start rather than being guessed.
- While provisioning, the management allowlist is every eligible physical
  interface currently present, recomputed on each reconciliation and persisted.
  An adapter connected during first boot begins serving without a restart; one
  removed stops; a restart resumes exactly what was serving.
- Eligibility is decided from recorded hardware facts and never from the
  interface name. The helper records an adapter as physical only when it reports
  a permanent hardware address and the kernel presents it on a real bus, which is
  what keeps a container veth, a bridge and the loopback device out.
- With no eligible adapter present the appliance persists nothing, installs no
  firewall rule and admits nothing. A configuration with an empty allowlist is
  not expressible, and admitting everything instead would leave the wildcard
  listener unguarded.
- `serve` starts with no listener option while provisioning and refuses that on a
  commissioned appliance, naming the options it needs.
- The appliance answers one local name on exactly the interfaces carrying
  management access, published through the privileged helper. The advertisement
  carries a host label and interface names and no address, so one URL survives a
  DHCP renewal. It is renewed only on a change, and a failed publication is
  reported and retried rather than being fatal or silent.
- The helper manages only responder configuration bearing its own marker and
  refuses to replace any other.
- A wired adapter accepts `link-local` as its IPv4 method, so direct
  commissioning needs no DHCP server and no guessed address. The standard probe
  reaches a different address on each adapter, which is why no conventional
  per-adapter address is assigned.
- Commissioning is administrator-only and one way. It refuses a selection the
  appliance could not be reached through, and requires explicit confirmation
  before giving up the interface the request arrived on. The lifecycle state is
  recorded last, so a transition that does not finish restores the allowlist that
  was serving and leaves the appliance provisioning.
- After commissioning the configured allowlist alone governs; a newly attached
  eligible adapter never joins it.
- The three enforcement layers are unchanged: the helper's `inet nilo_management`
  nftables table, the socket gate before TLS, and the per-request recheck outside
  the application handler. TLS, authentication, authorization, CSRF, cookies and
  same-origin handling are unchanged. The unprivileged process gains no
  network-administration capability. No response, log or export carries a secret.
- Production certificates are read and checked, never generated or replaced. A
  missing advertised local name produces an actionable warning.

## Automated evidence

| Acceptance behavior | Automated evidence |
|---|---|
| 1. Reachable through each eligible interface | derivation test asserting the allowlist, the firewall call and admitted destinations |
| 2. Selection independent of Linux names | table-driven eligibility test including a physical adapter named `docker0` and a virtual device named `enp1s0` |
| 3. Discoverable through the appliance `.local` name | responder render test and announcer publication tests |
| 4. Direct Ethernet without DHCP | link-local validation and rendering tests; see the open deployment decision below |
| 5. Wi-Fi renewal without restart | provider refresh path, unchanged from the dual-path work and re-proven by the derivation tests |
| 6. Adapter absent at boot begins serving | appear-and-leave reconciliation test |
| 7. USB adapter only while provisioning | appear-and-leave test plus the commissioned-freeze test |
| 8. Loopback, container and bridge not exposed | eligibility test and simulated-catalog exclusion test |
| 9. Commissioning needs a confirmed path | refusal tests for empty, absent, ineligible and loopback selections |
| 10. Device role follows the exclusive-role contract | role coordinator from issue #5, unchanged; commissioning frees the adapter |
| 11. Failed transition recovers | recorder-failure test asserting the derived allowlist, the firewall and the admitted destinations are restored |
| 12. Restart preserves state | lifecycle reopen tests and the persisted-allowlist restart test |
| 13. Upgrades do not re-enable first boot | migration test building the previous schema with an identity row, then upgrading |
| 14. New adapter not auto-enabled after commissioning | commissioned-freeze test |
| 15. TLS, auth, CSRF, cookies, firewall, pre-application restriction intact | existing listener, gate and CSRF suites plus the commissioning API test |
| 16. No network or credential secret in API, GUI, tests or logs | response-shape tests and the browser panel secret sweep |
| 17. Relevant backend, helper, GUI and integration tests pass | see below |

Completed on the bootstrapped worktree:

```text
go test ./... -count=1
go vet ./...
GOOS=linux go vet ./...
uv run pytest -q        (integration_sdk, sigrok, fake_python)
npm run test:ci
npm run build
npm run test:e2e
```

The Go suite, the three Python suites, the browser component suite (35 files,
470 tests) and the GUI build all pass. `GOOS=linux go vet` is what compiles the
build-tagged helper code - the NetworkManager device reader, the responder
driver and the NetworkManager settings renderer - on a Windows development host.

## Accepted pre-existing failures

`python .agentic/aegis.py check --strict --run` exits non-zero on this branch for
reasons that are not this candidate. Each was reproduced on committed `master`
without the change, and they were accepted by the human under RULING-003 on
`TASK-2873801a`:

- `05_Software/scripts/tests/test_build_environment.py::BuildEnvironmentTests::test_reconstruction_lock_is_complete_and_consumed_offline`
  fails with `reconstruction input manifest SHA-256 mismatch`. It short-circuits
  the configured runner before the Go, Python and GUI suites execute, which is
  why those were run individually.
- `prettier --check` rejects `src/components/network/DeviceNetworksPanel.tsx`.
  It previously rejected four files; the other three are files this candidate
  edits, and formatting them as part of that work resolved their findings.
- `eslint` rejects `src/components/network/ManagementAccessPanel.tsx:179`
  (`setState` within an effect).
- `e2e/network.spec.ts:115` fails its `toBeDisabled()` assertion. This one was
  isolated directly rather than assumed: `provider_simulated.go` was temporarily
  restored to its committed content and the single spec rerun, where it failed
  with the identical assertion and the identical observed value.

The last three carry uncommitted fixes in the main checkout's in-progress
`TASK-8f18c927` work and may resolve when issue #5 lands. **CI running
`check --strict --run` will fail this branch until they are addressed**, and
that is true of `master` today as well.

## Hardware acceptance still owed

None of the following is claimed by this report. On an explicitly authorized
Mini PC deployment, verify: first access through each built-in eligible adapter;
direct Ethernet access with no DHCP server; Wi-Fi DHCP renewal and reconnection;
USB Ethernet hotplug during and after commissioning; safe transition of adapters
to device-facing roles; loss and recovery of the active management path;
persistence across a full reboot; and no NILO management exposure on a
commissioned device-facing network.

## Open decisions carried forward

- **The standing link-local fallback.** Acceptance 4 asks that direct Ethernet
  commissioning work with nothing configured. This candidate makes link-local
  expressible and proves an adapter holding one is a usable management path, but
  does not make it automatic. Closing it needs either a NetworkManager system
  default shipped with the appliance image - deployment configuration, which
  belongs with the offline-installer work that is not on this branch - or NILO
  applying the method during provisioning, which would be an automatic network
  change with no acknowledgement or rollback window through machinery built
  around both. Recorded in `TASK-799d62a6` rather than chosen.
- **Where the deployment change lands.** The appliance unit file still passes
  legacy `--bind`, and the installer and its responder unit live on an unpushed
  branch in a separate clone. First-boot provisioning needs the unit to stop
  passing `--bind` before it takes effect on a real appliance.
- **The seeded administrator.** RULING-002 records that the image seeds `admin`
  with a fixed password. A published default credential on an appliance that is
  deliberately reachable on every eligible adapter during first boot is worth a
  compensating control; making commissioning refuse to complete while the seeded
  password is unchanged would cost one check.
