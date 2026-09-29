// Package modbus defines Core-owned transport metadata shared by storage and Bench.
package modbus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid_modbus_definition")

var identifier = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var usbID = regexp.MustCompile(`^[0-9a-f]{4}$`)
var busPath = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var hostLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validName(value string) bool {
	if !utf8.ValidString(value) || value != strings.TrimSpace(value) || utf8.RuneCountInString(value) < 1 || utf8.RuneCountInString(value) > 128 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func canonicalHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Zone() != "" {
			return "", ErrInvalid
		}
		return ip.Unmap().String(), nil
	}
	if len(host) > 253 || host == "" {
		return "", ErrInvalid
	}
	for _, label := range strings.Split(host, ".") {
		if !hostLabel.MatchString(label) {
			return "", ErrInvalid
		}
	}
	return host, nil
}

// USBIdentity is trusted discovery metadata, never a user-supplied OS path.
type USBIdentity struct {
	StableID     string  `json:"stable_id"`
	VendorID     string  `json:"vendor_id"`
	ProductID    string  `json:"product_id"`
	SerialNumber *string `json:"serial_number"`
	BusPath      string  `json:"bus_path"`
}

// Network explicitly describes a configured peer and its permitted unit range.
type Network struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	UnitMin int    `json:"unit_min"`
	UnitMax int    `json:"unit_max"`
}

// Endpoint separates saved configuration from observed presence and readiness.
type Endpoint struct {
	SchemaVersion   int          `json:"schema_version"`
	ID              string       `json:"id"`
	Revision        int64        `json:"revision"`
	Name            string       `json:"name"`
	Transport       string       `json:"transport"`
	ResourceID      string       `json:"resource_id"`
	Enabled         bool         `json:"enabled"`
	Presence        string       `json:"presence"`
	Readiness       string       `json:"readiness"`
	SerialIdentity  *USBIdentity `json:"serial_identity,omitempty"`
	Network         *Network     `json:"network,omitempty"`
	Binding         string       `json:"-"` // Core managed-device ID; stored separately.
	BindingRevision int64        `json:"-"`
}

// USBStableID includes the physical interface/port suffix carried by discovery.
// Weak serials use their port path; stable serials survive USB topology changes.
func USBStableID(h USBIdentity) string {
	basis, kind := h.BusPath, "port"
	if h.SerialNumber != nil {
		basis, kind = *h.SerialNumber, "serial"
	}
	raw, _ := json.Marshal([]string{kind, strings.ToLower(h.VendorID), strings.ToLower(h.ProductID), basis})
	return fmt.Sprintf("usb-%x", sha256.Sum256(raw))
}

// USBResourceKeys reserve both the physical port and any strong serial identity.
// The physical key deliberately excludes vendor, family, instance and tty alias.
func USBResourceKeys(h USBIdentity) []string {
	keys := []string{"usb:port:" + strings.ToLower(h.BusPath)}
	if h.SerialNumber != nil {
		keys = append(keys, "usb:identity:"+USBStableID(h))
	}
	return keys
}

// ResourceKeys joins network bus aliases and prevents relabeling one peer socket.
func (e Endpoint) ResourceKeys() []string {
	if e.SerialIdentity != nil {
		return USBResourceKeys(*e.SerialIdentity)
	}
	if e.Network == nil {
		return nil
	}
	return []string{"network:bus:" + e.ResourceID, "network:peer:" + net.JoinHostPort(e.Network.Host, strconv.Itoa(e.Network.Port))}
}

// Profile is a shared, revisioned transport configuration with a retained creator.
type Profile struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Revision      int64           `json:"revision"`
	OwnerID       string          `json:"owner_id"`
	Name          string          `json:"name"`
	EndpointID    string          `json:"endpoint_id"`
	Settings      json.RawMessage `json:"settings"`
}

// Canonical validates one complete endpoint and returns an independent normalized copy.
func (e Endpoint) Canonical() (Endpoint, error) {
	if e.SchemaVersion != 1 || !identifier.MatchString(e.ID) || !identifier.MatchString(e.ResourceID) || e.Revision < 1 || e.Revision > 2147483647 || !validName(e.Name) {
		return Endpoint{}, ErrInvalid
	}
	switch e.Readiness {
	case "ready", "offline", "unconfigured", "fault":
	default:
		return Endpoint{}, ErrInvalid
	}
	switch e.Transport {
	case "rtu":
		if e.Network != nil || e.SerialIdentity == nil || (e.Presence != "present" && e.Presence != "absent") {
			return Endpoint{}, ErrInvalid
		}
		h := *e.SerialIdentity
		h.VendorID, h.ProductID = strings.ToLower(h.VendorID), strings.ToLower(h.ProductID)
		if !identifier.MatchString(h.StableID) || !usbID.MatchString(h.VendorID) || !usbID.MatchString(h.ProductID) || !busPath.MatchString(h.BusPath) || e.ResourceID != h.StableID {
			return Endpoint{}, ErrInvalid
		}
		if h.SerialNumber != nil {
			serial := *h.SerialNumber
			if !utf8.ValidString(serial) || len(serial) > 128 || serial == "" || strings.ContainsAny(serial, "\x00\r\n") {
				return Endpoint{}, ErrInvalid
			}
			h.SerialNumber = &serial
		}
		e.SerialIdentity = &h
	case "tcp", "rtu_tcp":
		if e.SerialIdentity != nil || e.Network == nil || e.Binding != "" || e.Presence != "configured" {
			return Endpoint{}, ErrInvalid
		}
		n := *e.Network
		host, err := canonicalHost(n.Host)
		if err != nil || n.Port < 1 || n.Port > 65535 || n.UnitMin < 0 || n.UnitMax > 255 || n.UnitMin > n.UnitMax {
			return Endpoint{}, ErrInvalid
		}
		if e.Transport == "rtu_tcp" && (n.UnitMin < 1 || n.UnitMax > 247) {
			return Endpoint{}, ErrInvalid
		}
		n.Host = host
		e.Network = &n
	default:
		return Endpoint{}, ErrInvalid
	}
	return e, nil
}

// Settings holds explicit transport timing and optional serial framing.
type Settings struct {
	ResponseTimeoutMS *int64  `json:"response_timeout_ms"`
	ConnectTimeoutMS  *int64  `json:"connect_timeout_ms"`
	InterRequestMS    *int64  `json:"inter_request_ms"`
	ReadRetries       *int64  `json:"read_retries"`
	BaudRate          *int    `json:"baud_rate,omitempty"`
	DataBits          *int    `json:"data_bits,omitempty"`
	Parity            *string `json:"parity,omitempty"`
	StopBits          *int    `json:"stop_bits,omitempty"`
	LocalEcho         *bool   `json:"local_echo,omitempty"`
}

// Canonical validates a profile against its endpoint without opening a transport.
func (p Profile) Canonical(e Endpoint) (Profile, error) {
	if p.SchemaVersion != 1 || !identifier.MatchString(p.ID) || !identifier.MatchString(p.OwnerID) || p.EndpointID != e.ID || p.Revision < 1 || p.Revision > 2147483647 || !validName(p.Name) || len(p.Settings) > 16384 {
		return Profile{}, ErrInvalid
	}
	var settings Settings
	decoder := json.NewDecoder(bytes.NewReader(p.Settings))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return Profile{}, ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Profile{}, ErrInvalid
	}
	for _, n := range []*int64{settings.ResponseTimeoutMS, settings.ConnectTimeoutMS} {
		if n == nil || *n < 1 || *n > 2147483647 {
			return Profile{}, ErrInvalid
		}
	}
	for _, n := range []*int64{settings.InterRequestMS, settings.ReadRetries} {
		if n == nil || *n < 0 || *n > 2147483647 {
			return Profile{}, ErrInvalid
		}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(p.Settings, &fields) != nil {
		return Profile{}, ErrInvalid
	}
	for name, raw := range fields {
		switch name {
		case "response_timeout_ms", "connect_timeout_ms", "inter_request_ms", "read_retries", "baud_rate", "data_bits", "parity", "stop_bits", "local_echo":
		default:
			return Profile{}, ErrInvalid
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return Profile{}, ErrInvalid
		}
	}
	if e.Transport == "rtu" {
		if settings.BaudRate == nil || *settings.BaudRate < 50 || *settings.BaudRate > 4000000 || settings.DataBits == nil || *settings.DataBits != 8 || settings.Parity == nil || settings.StopBits == nil || settings.LocalEcho == nil {
			return Profile{}, ErrInvalid
		}
		switch *settings.Parity {
		case "even", "odd":
			if *settings.StopBits != 1 {
				return Profile{}, ErrInvalid
			}
		case "none":
			if *settings.StopBits != 1 && *settings.StopBits != 2 {
				return Profile{}, ErrInvalid
			}
		default:
			return Profile{}, ErrInvalid
		}
	} else if settings.BaudRate != nil || settings.DataBits != nil || settings.Parity != nil || settings.StopBits != nil || settings.LocalEcho != nil {
		return Profile{}, ErrInvalid
	}
	p.Settings, _ = json.Marshal(settings)
	return p, nil
}
