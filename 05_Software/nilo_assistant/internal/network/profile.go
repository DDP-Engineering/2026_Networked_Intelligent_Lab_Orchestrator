package network

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

// ErrProfileRejected is returned for any configuration NILO will not import.
// Every rejection names the directive responsible, so an operator is told what
// to remove rather than that the file was simply bad.
var ErrProfileRejected = errors.New("WireGuard profile rejected")

const maxProfileNameLength = 64

// executableDirectives are the wg-quick hooks that run a shell command.
//
// This is the reason NILO reads the file itself instead of handing it to
// wg-quick: wg-quick is a shell script that executes these, so importing a
// supplied profile through it would mean running whatever the supplier wrote.
var executableDirectives = map[string]bool{
	"preup": true, "postup": true, "predown": true, "postdown": true,
}

// unsupportedDirectives are understood, deliberately not implemented, and
// refused by name rather than silently dropped — an operator must never be told
// a profile was imported as written when part of it was discarded.
var unsupportedDirectives = map[string]string{
	"saveconfig": "a NILO-owned profile is not rewritten by the tunnel itself",
	"table":      "routing-table selection is not managed in this milestone",
	"fwmark":     "firewall marking is not managed in this milestone",
}

// ProfilePeer is one remote end of a tunnel. It carries no preshared key: that
// is key material and leaves the parser in a Secrets value instead.
type ProfilePeer struct {
	PublicKey           string   `json:"public_key"`
	AllowedIPs          []string `json:"allowed_ips"`
	Endpoint            string   `json:"endpoint"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
	// HasPresharedKey says whether one was supplied, without carrying it.
	HasPresharedKey bool `json:"has_preshared_key"`
}

// Profile is everything about an imported tunnel that is safe to show.
//
// There is no private-key field and no preshared-key field, anywhere in this
// type or below it. That is deliberate: a status response, a listing, an audit
// record and a log line are all built from a Profile, so key material cannot
// reach them through a field that does not exist.
type Profile struct {
	Name       string   `json:"name"`
	Addresses  []string `json:"addresses"`
	DNS        []string `json:"dns"`
	MTU        int      `json:"mtu"`
	ListenPort int      `json:"listen_port"`

	Peers []ProfilePeer `json:"peers"`
}

// Secrets is the key material an imported profile carries. It goes straight to
// root-owned storage and nowhere else.
//
// It redacts itself when printed or serialised, because the accident of a value
// reaching a log line or a response body is exactly how key material escapes.
type Secrets struct {
	PrivateKey    string
	PresharedKeys []string
	// Password is a wireless network password, staged the same way as key
	// material. A Secrets carries either tunnel keys or a password.
	Password string
}

// String renders a Secrets without its contents. It is defined on the value,
// not the pointer, so both %v and %+v on either form go through it.
func (secrets Secrets) String() string {
	return fmt.Sprintf("network.Secrets{private key redacted, %d preshared key(s) redacted, password redacted}",
		len(secrets.PresharedKeys))
}

// MarshalJSON keeps key material out of anything that serialises a Secrets by
// accident. There is no round trip: this type is written to protected storage
// field by field, never through JSON.
func (secrets Secrets) MarshalJSON() ([]byte, error) {
	return []byte(`{"private_key":"redacted","preshared_keys":"redacted","password":"redacted"}`), nil
}

// Connection is a NILO-owned NetworkManager connection built from a profile.
type Connection struct {
	Profile Profile

	// NILOOwned separates connections NILO created from tunnels that were
	// already on the host. An externally managed tunnel is read-only.
	NILOOwned bool
	// Active and AutoConnect are both false for a freshly imported profile:
	// importing is not connecting, and connecting at startup is a separate,
	// explicit choice.
	Active      bool
	AutoConnect bool
}

// NewImportedConnection wraps a parsed profile as it is first created:
// NILO-owned, inactive, and not connecting on its own.
func NewImportedConnection(profile Profile) Connection {
	return Connection{Profile: profile, NILOOwned: true, Active: false, AutoConnect: false}
}

// ParseProfile reads a standard WireGuard configuration file, returning the
// safe profile and its key material separately.
//
// It is a reader, not an interpreter: nothing in the file is ever executed, and
// a directive that would execute something is refused outright.
func ParseProfile(name string, data []byte) (Profile, Secrets, error) {
	if len(data) > 48<<10 {
		return Profile{}, Secrets{}, fmt.Errorf("%w: file exceeds 48 KiB", ErrProfileRejected)
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" || len(trimmedName) > maxProfileNameLength ||
		strings.ContainsAny(trimmedName, `/\:*?"<>|`) || strings.Contains(trimmedName, "..") || strings.IndexFunc(trimmedName, unicode.IsControl) >= 0 {
		return Profile{}, Secrets{}, fmt.Errorf("%w: %q is not a usable profile name", ErrProfileRejected, name)
	}

	var (
		profile   Profile
		secrets   Secrets
		section   string
		sawIface  bool
		peerIndex = -1
	)
	seen := map[string]bool{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; scanner.Scan(); line++ {
		text, _, _ := strings.Cut(scanner.Text(), "#")
		text = strings.TrimSpace(text)
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, ";") {
			continue
		}

		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			seen = map[string]bool{}
			switch strings.ToLower(strings.Trim(text, "[]")) {
			case "interface":
				if sawIface {
					return Profile{}, Secrets{}, fmt.Errorf(
						"%w: line %d declares a second [Interface] section", ErrProfileRejected, line)
				}
				section, sawIface = "interface", true
			case "peer":
				if !sawIface || len(profile.Peers) >= 32 {
					return Profile{}, Secrets{}, fmt.Errorf("%w: invalid peer section order or too many peers", ErrProfileRejected)
				}
				section = "peer"
				profile.Peers = append(profile.Peers, ProfilePeer{})
				secrets.PresharedKeys = append(secrets.PresharedKeys, "")
				peerIndex++
			default:
				return Profile{}, Secrets{}, fmt.Errorf(
					"%w: line %d declares the unsupported section %s", ErrProfileRejected, line, text)
			}
			continue
		}

		key, value, found := strings.Cut(text, "=")
		if !found {
			return Profile{}, Secrets{}, fmt.Errorf(
				"%w: line %d is not a directive", ErrProfileRejected, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		lowered := strings.ToLower(key)
		if seen[lowered] && lowered != "address" && lowered != "dns" && lowered != "allowedips" {
			return Profile{}, Secrets{}, fmt.Errorf("%w: duplicate directive on line %d", ErrProfileRejected, line)
		}
		seen[lowered] = true

		if executableDirectives[lowered] {
			return Profile{}, Secrets{}, fmt.Errorf(
				"%w: %s on line %d would execute a command, which NILO never does when importing a profile",
				ErrProfileRejected, key, line)
		}
		if reason, unsupported := unsupportedDirectives[lowered]; unsupported {
			return Profile{}, Secrets{}, fmt.Errorf(
				"%w: %s on line %d is not supported because %s", ErrProfileRejected, key, line, reason)
		}
		if section == "" {
			return Profile{}, Secrets{}, fmt.Errorf(
				"%w: %s on line %d appears before any section", ErrProfileRejected, key, line)
		}

		var err error
		if section == "interface" {
			err = applyInterfaceDirective(&profile, &secrets, key, lowered, value, line)
		} else {
			err = applyPeerDirective(&profile.Peers[peerIndex], &secrets, key, lowered, value, line)
		}
		if err != nil {
			return Profile{}, Secrets{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return Profile{}, Secrets{}, fmt.Errorf("%w: %v", ErrProfileRejected, err)
	}

	profile.Name = trimmedName
	if err := validateProfile(profile, secrets); err != nil {
		return Profile{}, Secrets{}, err
	}
	return profile, secrets, nil
}

func applyInterfaceDirective(profile *Profile, secrets *Secrets, key, lowered, value string, line int) error {
	switch lowered {
	case "privatekey":
		secrets.PrivateKey = value
	case "address":
		profile.Addresses = append(profile.Addresses, splitList(value)...)
	case "dns":
		profile.DNS = append(profile.DNS, splitList(value)...)
	case "mtu":
		return assignPositiveInt(&profile.MTU, key, value, line)
	case "listenport":
		return assignPositiveInt(&profile.ListenPort, key, value, line)
	default:
		return fmt.Errorf("%w: %s on line %d is not a directive NILO understands",
			ErrProfileRejected, key, line)
	}
	return nil
}

func applyPeerDirective(peer *ProfilePeer, secrets *Secrets, key, lowered, value string, line int) error {
	switch lowered {
	case "publickey":
		peer.PublicKey = value
	case "presharedkey":
		secrets.PresharedKeys[len(secrets.PresharedKeys)-1] = value
		peer.HasPresharedKey = true
	case "allowedips":
		peer.AllowedIPs = append(peer.AllowedIPs, splitList(value)...)
	case "endpoint":
		peer.Endpoint = value
	case "persistentkeepalive":
		return assignPositiveInt(&peer.PersistentKeepalive, key, value, line)
	default:
		return fmt.Errorf("%w: %s on line %d is not a directive NILO understands",
			ErrProfileRejected, key, line)
	}
	return nil
}

func validateProfile(profile Profile, secrets Secrets) error {
	validKey := func(key string) bool {
		decoded, err := base64.StdEncoding.Strict().DecodeString(key)
		return err == nil && len(decoded) == 32
	}
	if !validKey(secrets.PrivateKey) {
		return fmt.Errorf("%w: invalid PrivateKey encoding", ErrProfileRejected)
	}
	if profile.MTU != 0 && (profile.MTU < 576 || profile.MTU > 65535) || profile.ListenPort > 65535 {
		return fmt.Errorf("%w: MTU or ListenPort out of range", ErrProfileRejected)
	}
	for _, address := range profile.Addresses {
		if _, err := netip.ParsePrefix(address); err != nil {
			return fmt.Errorf("%w: invalid Address", ErrProfileRejected)
		}
	}
	for _, dns := range profile.DNS {
		if _, err := netip.ParseAddr(dns); err != nil && !validDNSName(dns) {
			return fmt.Errorf("%w: invalid DNS server or search domain", ErrProfileRejected)
		}
	}
	switch {
	case secrets.PrivateKey == "":
		return fmt.Errorf("%w: the [Interface] section carries no PrivateKey", ErrProfileRejected)
	case len(profile.Addresses) == 0:
		return fmt.Errorf("%w: the [Interface] section carries no Address", ErrProfileRejected)
	case len(profile.Peers) == 0:
		return fmt.Errorf("%w: the profile declares no [Peer]", ErrProfileRejected)
	}
	for index, peer := range profile.Peers {
		if !validKey(peer.PublicKey) || peer.HasPresharedKey && !validKey(secrets.PresharedKeys[index]) {
			return fmt.Errorf("%w: peer %d has invalid key encoding", ErrProfileRejected, index+1)
		}
		if peer.PersistentKeepalive > 65535 {
			return fmt.Errorf("%w: peer keepalive out of range", ErrProfileRejected)
		}
		for _, address := range peer.AllowedIPs {
			if _, err := netip.ParsePrefix(address); err != nil {
				return fmt.Errorf("%w: invalid AllowedIPs", ErrProfileRejected)
			}
		}
		if peer.Endpoint != "" {
			host, port, err := net.SplitHostPort(peer.Endpoint)
			n, numberErr := strconv.Atoi(port)
			_, ipErr := netip.ParseAddr(host)
			if err != nil || numberErr != nil || n < 1 || n > 65535 || ipErr != nil && !validDNSName(host) {
				return fmt.Errorf("%w: invalid Endpoint", ErrProfileRejected)
			}
		}
		if peer.PublicKey == "" {
			return fmt.Errorf("%w: peer %d carries no PublicKey", ErrProfileRejected, index+1)
		}
		if len(peer.AllowedIPs) == 0 {
			return fmt.Errorf("%w: peer %d carries no AllowedIPs", ErrProfileRejected, index+1)
		}
	}
	return nil
}

func assignPositiveInt(target *int, key, value string, line int) error {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 || parsed == 0 && strings.EqualFold(key, "MTU") {
		return fmt.Errorf("%w: %s on line %d is not a positive number", ErrProfileRejected, key, line)
	}
	*target = parsed
	return nil
}

func validDNSName(name string) bool {
	name = strings.TrimPrefix(name, "~")
	if name == "." {
		return true
	}
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}

// splitList reads a comma-separated directive value, dropping empty entries so
// a trailing comma is not turned into a blank address.
func splitList(value string) []string {
	var entries []string
	for _, field := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}
	return entries
}
