# Find your NILO

The permanent name printed on the appliance is the primary way to identify it.
It has the form `nilo-xxxxxxxxxx`; the QR code opens
`https://<permanent-name>.local/`. First contact normally shows a certificate
trust warning because `.local` names cannot use a publicly trusted certificate.
Check that the name in the warning is the name printed on the enclosure before
continuing.

## Choose the path that matches your setup

1. **Windows on the same LAN:** open Network in File Explorer and select the
   NILO device. If it has not appeared after 60 seconds, run `nilo discover`
   from a terminal on that LAN.
2. **macOS or Linux on the same LAN:** open the printed `.local` URL, or run
   `nilo discover`. Multicast discovery does not normally cross routers or
   VLANs.
3. **A router-managed network:** inspect the router's DHCP leases for the
   printed name. Router DNS is optional router policy, so use the reported
   address if the name is not resolvable.
4. **Direct Ethernet:** connect the client and NILO directly, wait for
   link-local addressing, then run `nilo discover`. No DHCP server or guessed
   address is required.
5. **A fresh appliance with Wi-Fi:** join the WPA2 network whose SSID is the
   printed permanent name. Use the published first-boot password
   `admin_password`; a captive-portal probe redirects to the serial HTTPS URL.
   The access point exists only while the appliance is provisioning and only
   when an unused, unconfigured Wi-Fi radio is available.
6. **An attached appliance display:** configure the optional loopback status
   listener and open it locally. It shows the permanent name, safe management
   addresses, and printable label without exposing that page to any network.

`nilo discover --json` returns deterministic schema-versioned records merged by
appliance UUID from mDNS/DNS-SD and SSDP. `nilo open <name>` opens an exact
permanent name or one unique friendly name. Existing remote commands also
accept a unique discovered name in `--server`; an explicit URL bypasses
discovery. Use `nilo discover --label <name>` to write the selected SVG label to
standard output.

After signing in, About shows the permanent and optional friendly names,
management addresses and label. Administrators can assign a lowercase friendly
DNS label and run **Identify appliance**. A supported Raspberry Pi blinks for a
bounded interval; generic hardware reports that Identify is unsupported. A
short physical-button release performs the same indication and refreshes the
current mDNS and SSDP presence. Longer button gestures retain their separately
documented recovery and commissioning-reset meanings.

Discovery advertises only on interfaces currently admitted for management.
Device networks, WireGuard links, loopback, and non-allowlisted interfaces are
excluded. The permanent name remains available if the shared provisioning name
or a friendly alias collides. Discovery, labels, and local status contain public
identity and addressing facts only—never passwords, tokens, TLS private keys,
raw hardware identity sources, or integration credentials.

For operational setup and verification commands, see
[Network operations and development](Network_Operations.md) and
`07_Delivery/NILO_Discovery_Verification.md`.
