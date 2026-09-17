package nethelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureApplianceIdentityDerivesAndPersistsHardwareName(t *testing.T) {
	directory := privateIdentityDirectory(t)
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		piDeviceTreeSerialPath: []byte("unknown\x00\n"),
		dmiProductSerialPath:   []byte(" ABC-123 \n"),
	}
	readFile := func(path string) ([]byte, error) {
		value, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return append([]byte(nil), value...), nil
	}

	identity, err := EnsureApplianceIdentity(store, ApplianceIdentitySources{
		ReadFile: readFile,
		Random:   bytes.NewReader(bytes.Repeat([]byte{0x44}, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if identity.Schema != 1 || identity.SerialName != "nilo-n3dvtyjqjs" || identity.SourceKind != "dmi-product" {
		t.Fatalf("unexpected identity: %+v", identity)
	}

	files[dmiProductSerialPath] = []byte("different-hardware")
	again, err := EnsureApplianceIdentity(store, ApplianceIdentitySources{
		ReadFile: readFile,
		Random:   bytes.NewReader(bytes.Repeat([]byte{0x99}, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if again != identity {
		t.Fatalf("persisted identity changed: first=%+v again=%+v", identity, again)
	}

	path := filepath.Join(directory, applianceIdentityFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "abc-123") {
		t.Fatalf("identity record exposed raw hardware serial: %s", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("identity record mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestEnsureApplianceIdentityUsesOnePersistedFallback(t *testing.T) {
	directory := privateIdentityDirectory(t)
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	readMissing := func(string) ([]byte, error) { return nil, os.ErrNotExist }
	seed := bytes.Repeat([]byte{0x2a}, 16)

	identity, err := EnsureApplianceIdentity(store, ApplianceIdentitySources{
		ReadFile: readMissing,
		Random:   bytes.NewReader(seed),
	})
	if err != nil {
		t.Fatal(err)
	}
	if identity.SourceKind != "fallback" || !validPermanentSerialName(identity.SerialName) {
		t.Fatalf("unexpected fallback identity: %+v", identity)
	}

	again, err := EnsureApplianceIdentity(store, ApplianceIdentitySources{
		ReadFile: readMissing,
		Random:   bytes.NewReader(bytes.Repeat([]byte{0xff}, 16)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if again != identity {
		t.Fatalf("fallback was regenerated: first=%+v again=%+v", identity, again)
	}
}

func TestEnsureApplianceIdentityRefusesCorruptRecord(t *testing.T) {
	directory := privateIdentityDirectory(t)
	if err := os.WriteFile(filepath.Join(directory, applianceIdentityFileName), []byte(`{"schema":1,"serial_name":"nilo-bad","source_kind":"fallback","fallback_seed":"00"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = EnsureApplianceIdentity(store, ApplianceIdentitySources{
		ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist },
		Random:   bytes.NewReader(bytes.Repeat([]byte{0x11}, 16)),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid appliance identity") {
		t.Fatalf("corrupt record error = %v", err)
	}
}

func TestDeriveApplianceNameGoldenVectors(t *testing.T) {
	tests := []struct {
		kind, value, want string
	}{
		{"rpi-board", "00000000a1b2c3d4", "nilo-p2rjmzdkf1"},
		{"dmi-product", "abc-123", "nilo-n3dvtyjqjs"},
	}
	for _, test := range tests {
		if got := deriveApplianceName(test.kind, test.value); got != test.want {
			t.Errorf("deriveApplianceName(%q, %q) = %q, want %q", test.kind, test.value, got, test.want)
		}
	}
}

func TestApplianceIdentityProtocolReturnsOnlyPublicRecord(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	identity := ApplianceIdentity{Schema: 1, SerialName: "nilo-p2rjmzdkf1", SourceKind: "rpi-board"}
	if err := engine.ConfigureApplianceIdentity(identity); err != nil {
		t.Fatal(err)
	}

	response := dispatch(context.Background(), engine, Request{Op: "appliance.identity.read"})
	if response.Error != "" || response.ApplianceIdentity == nil || *response.ApplianceIdentity != identity {
		t.Fatalf("unexpected identity response: %+v", response)
	}
	refused := dispatch(context.Background(), engine, Request{Op: "appliance.identity.read", ID: "unexpected"})
	if refused.Code != "invalid_request" || refused.ApplianceIdentity != nil {
		t.Fatalf("identity request accepted extra fields: %+v", refused)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fallback_seed", "00000000a1b2c3d4"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("public identity response contains %q: %s", secret, encoded)
		}
	}
}

func privateIdentityDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil && !errors.Is(err, os.ErrPermission) {
			t.Fatal(err)
		}
	}
	return directory
}
