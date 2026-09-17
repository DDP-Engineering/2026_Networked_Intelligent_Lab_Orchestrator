# Find your NILO

The permanent name printed on the appliance is the primary way to identify it.
It has the form `nilo-xxxxxxxxxx`; the QR code opens the exact advertised
permanent-name HTTPS URL, including a configured non-default management port.
First contact normally shows a certificate trust warning because `.local`
names cannot use a publicly trusted certificate. Check that the name in the
warning is the name printed on the enclosure before continuing; a name mismatch
is not the expected warning.

## How NILO names work

NILO can show several names or addresses, but they do not all mean the same
thing:

| What you see | What it means | Can it change? |
|---|---|---|
| `nilo-xxxxxxxxxx` | The appliance's permanent name. It is printed on the label and is the safest name to use when identifying a particular NILO. | No. Restarting, commissioning, or returning to setup does not replace it. |
| A friendly name, such as `bench-west` | An optional, memorable alias chosen by an administrator. It helps people recognize where or how the appliance is used. | Yes. It can be changed or removed, but it never replaces the permanent name. |
| `nilo.local` | A shared setup and recovery address used while an appliance is waiting to be commissioned. It is not the identity of a particular unit. | It is available only during setup or recovery. |
| An IP address, such as `192.168.1.42` | A current network route to the appliance. One NILO can have several addresses. | Yes. Routers and network configuration can change it. |

When in doubt, compare the permanent name on screen with the name on the
physical label. The QR code, About page, discovery results and command-line
tools all use that same permanent identity.

## How discovery works

NILO announces itself on each local network connection that is allowed to open
its management interface. A computer or phone on the same local network can
then find it without already knowing its IP address. Windows can show it in
Network, macOS and Linux can open its printed `.local` address, and the NILO
command-line tool can list it.

These announcements normally stay inside one local network. They do not
usually cross a router or VLAN. On a routed network, use the router's device or
lease list, or ask the network administrator which approved discovery relay is
available.

## Choose the path that matches your setup

1. **Windows on the same LAN:** open Network in File Explorer and select the
   NILO device. If it has not appeared after 60 seconds, run `nilo discover`
   from a terminal on that LAN; its default scan completes in five seconds.
2. **macOS or Linux on the same LAN:** open the printed `.local` URL, or run
   `nilo discover`; its default scan completes in five seconds. Multicast
   discovery does not normally cross routers or VLANs, so repeat only after the
   intended local interface reports up.
3. **Direct Ethernet:** connect the client and NILO directly and wait until the
   client reports a link-local address, then run `nilo discover`; the scan is
   bounded to five seconds by default. No DHCP server or guessed address is
   required. If the link never becomes ready, check the cable and each link
   indicator rather than assigning a conventional address to NILO.
4. **A fresh appliance with no cable:** join the WPA2 network whose SSID is the
   printed permanent name. Use the published first-boot password
   `admin_password`; a captive-portal probe redirects to the serial HTTPS URL.
   The access point exists only while the appliance is provisioning and only
   when an unused, unconfigured Wi-Fi radio is available. If the SSID is absent
   after refreshing the client Wi-Fi list, use direct Ethernet; NILO does not
   take over a radio that has a saved or active client profile.
5. **A router-managed or routed lab:** inspect the router's DHCP leases for the
   printed name and use the lease address. Lease convergence is router-specific,
   and router DNS is optional policy. Multicast discovery is link-scoped, so do
   not wait for it to cross a router or VLAN; use the router record or an
   approved relay instead.
6. **An attached appliance display:** configure the optional loopback status
   listener and open it locally; there is no network convergence delay. It
   shows the permanent name, safe management addresses, and printable label
   without exposing that page to any network. If the page is unavailable,
   inspect the local service status before changing appliance networking.

## Command-line and operational details

`nilo discover --json` returns deterministic schema-versioned records merged by
appliance UUID from mDNS/DNS-SD and SSDP. `nilo open <name>` opens an exact
permanent name or one unique friendly name. Existing remote commands also
accept a unique discovered name in `--server`; an explicit URL bypasses
discovery. Use `nilo discover --label <name>` to write the selected SVG label to
standard output. The label prints at 62 by 29 mm with a 27 mm QR code; its
browser preview is enlarged for inspection without changing those print
dimensions.

After signing in, About shows the permanent identity beside the title, puts
friendly-name and Identify controls first, and groups the software/API/protocol
versions, Linux/kernel/architecture/storage facts and management details. The
hardware address is shown once, inside the embedded printable label.
Administrators can assign a lowercase
friendly DNS label and run **Identify appliance**. A supported Raspberry Pi blinks for a
bounded interval; generic hardware reports that Identify is unsupported. A
short physical-button release performs the same indication and refreshes the
current mDNS and SSDP presence. Longer button gestures retain their separately
documented recovery and commissioning-reset meanings.

If none of these paths finds the appliance, verify power, carrier and the
client's selected interface first. A short button release is safe for physical
identification and refreshes discovery; do not use the longer recovery or reset
gestures unless their network effects in
[Network operations and development](Network_Operations.md) are intended.

Discovery advertises only on interfaces currently admitted for management.
Device networks, WireGuard links, loopback, and non-allowlisted interfaces are
excluded. The permanent name remains available if the shared provisioning name
or a friendly alias collides. Discovery, labels, and local status contain public
identity and addressing facts only—never passwords, tokens, TLS private keys,
raw hardware identity sources, or integration credentials.

For operational setup and verification commands, see
[Network operations and development](Network_Operations.md) and
`07_Delivery/NILO_Discovery_Verification.md`.
