package nethelper

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

const (
	applianceIdentityFileName = "appliance-identity.json"
	piDeviceTreeSerialPath    = "/sys/firmware/devicetree/base/serial-number"
	piProcSerialPath          = "/proc/device-tree/serial-number"
	dmiProductSerialPath      = "/sys/class/dmi/id/product_serial"
	applianceNameAlphabet     = "0123456789abcdefghjkmnpqrstvwxyz"
)

// ApplianceIdentity is the public, non-secret part of the helper-owned
// appliance identity. SourceKind describes which class of evidence was used;
// the source value itself never crosses the helper boundary.
type ApplianceIdentity struct {
	Schema     int    `json:"schema"`
	SerialName string `json:"serial_name"`
	SourceKind string `json:"source_kind"`
}

type applianceIdentityRecord struct {
	Schema       int    `json:"schema"`
	SerialName   string `json:"serial_name"`
	SourceKind   string `json:"source_kind"`
	FallbackSeed string `json:"fallback_seed,omitempty"`
}

// ApplianceIdentitySources makes hardware reads and entropy injectable while
// keeping their paths fixed by the helper.
type ApplianceIdentitySources struct {
	ReadFile func(string) ([]byte, error)
	Random   io.Reader
}

// EnsureApplianceIdentity returns the authoritative protected record, creating
// and durably saving it once when the helper has no record yet.
func EnsureApplianceIdentity(store *FileStore, sources ApplianceIdentitySources) (ApplianceIdentity, error) {
	if store == nil {
		return ApplianceIdentity{}, errors.New("appliance identity store is required")
	}
	record, found, err := store.loadApplianceIdentity()
	if err != nil {
		return ApplianceIdentity{}, fmt.Errorf("load appliance identity: %w", err)
	}
	if found {
		if err := validateApplianceIdentityRecord(record); err != nil {
			return ApplianceIdentity{}, fmt.Errorf("invalid appliance identity: %w", err)
		}
		return record.public(), nil
	}

	readFile := sources.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	for _, candidate := range []struct {
		kind string
		path string
	}{
		{kind: "rpi-board", path: piDeviceTreeSerialPath},
		{kind: "rpi-board", path: piProcSerialPath},
		{kind: "dmi-product", path: dmiProductSerialPath},
	} {
		value, readErr := readFile(candidate.path)
		if readErr != nil {
			continue
		}
		normalized := normalizeIdentitySource(value)
		if !usableIdentitySource(normalized) {
			continue
		}
		record = applianceIdentityRecord{
			Schema:     1,
			SerialName: deriveApplianceName(candidate.kind, normalized),
			SourceKind: candidate.kind,
		}
		if err := store.saveApplianceIdentity(record); err != nil {
			return ApplianceIdentity{}, fmt.Errorf("persist appliance identity: %w", err)
		}
		return record.public(), nil
	}

	random := sources.Random
	if random == nil {
		random = rand.Reader
	}
	seed := make([]byte, 16)
	if _, err := io.ReadFull(random, seed); err != nil {
		return ApplianceIdentity{}, fmt.Errorf("generate appliance identity fallback: %w", err)
	}
	encoded := hex.EncodeToString(seed)
	record = applianceIdentityRecord{
		Schema:       1,
		SerialName:   deriveApplianceName("fallback", encoded),
		SourceKind:   "fallback",
		FallbackSeed: encoded,
	}
	if err := store.saveApplianceIdentity(record); err != nil {
		return ApplianceIdentity{}, fmt.Errorf("persist appliance identity: %w", err)
	}
	return record.public(), nil
}

func (record applianceIdentityRecord) public() ApplianceIdentity {
	return ApplianceIdentity{
		Schema:     record.Schema,
		SerialName: record.SerialName,
		SourceKind: record.SourceKind,
	}
}

func normalizeIdentitySource(value []byte) string {
	return strings.ToLower(strings.TrimFunc(string(value), func(character rune) bool {
		return character == 0 || unicode.IsSpace(character)
	}))
}

func usableIdentitySource(value string) bool {
	if value == "" {
		return false
	}
	switch value {
	case "none", "unknown", "default string", "system serial number", "to be filled by o.e.m.":
		return false
	}
	allZero := true
	allF := true
	for _, character := range value {
		allZero = allZero && character == '0'
		allF = allF && character == 'f'
	}
	return !allZero && !allF
}

func deriveApplianceName(kind, value string) string {
	digest := sha256.Sum256([]byte("nilo-appliance-name\x00v1\x00" + kind + "\x00" + value))
	bits := uint64(digest[0])<<48 |
		uint64(digest[1])<<40 |
		uint64(digest[2])<<32 |
		uint64(digest[3])<<24 |
		uint64(digest[4])<<16 |
		uint64(digest[5])<<8 |
		uint64(digest[6])
	bits >>= 6
	encoded := make([]byte, 10)
	for index := len(encoded) - 1; index >= 0; index-- {
		encoded[index] = applianceNameAlphabet[bits&31]
		bits >>= 5
	}
	return "nilo-" + string(encoded)
}

func validPermanentSerialName(value string) bool {
	if len(value) != len("nilo-")+10 || !strings.HasPrefix(value, "nilo-") {
		return false
	}
	for _, character := range value[len("nilo-"):] {
		if !strings.ContainsRune(applianceNameAlphabet, character) {
			return false
		}
	}
	return true
}

func validateApplianceIdentityRecord(record applianceIdentityRecord) error {
	if record.Schema != 1 || !validPermanentSerialName(record.SerialName) {
		return errors.New("unsupported schema or serial name")
	}
	switch record.SourceKind {
	case "rpi-board", "dmi-product":
		if record.FallbackSeed != "" {
			return errors.New("hardware identity contains fallback state")
		}
	case "fallback":
		seed, err := hex.DecodeString(record.FallbackSeed)
		if err != nil || len(seed) != 16 || record.FallbackSeed != strings.ToLower(record.FallbackSeed) {
			return errors.New("invalid fallback seed")
		}
		if record.SerialName != deriveApplianceName("fallback", record.FallbackSeed) {
			return errors.New("fallback name does not match seed")
		}
	default:
		return errors.New("unknown source kind")
	}
	return nil
}
