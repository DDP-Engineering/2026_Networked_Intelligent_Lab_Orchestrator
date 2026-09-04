//go:build linux

package nethelper

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWireGuardVersionOutputIsBounded(t *testing.T) {
	out := &probeOutput{}
	_, err := io.Copy(out, io.LimitReader(strings.NewReader(strings.Repeat("x", 8192)), 8192))
	if err == nil || len(out.Bytes()) > 4096 {
		t.Fatal("unbounded command output")
	}
}

func TestKernelReadinessUsesRunningKernelEvidence(t *testing.T) {
	for _, tc := range []struct {
		name               string
		files              fstest.MapFS
		supported, unknown bool
	}{
		{"loaded", fstest.MapFS{"sys/module/wireguard": {Mode: os.ModeDir | 0555}}, true, false},
		{"built in", fstest.MapFS{"boot/config-test": {Data: []byte("CONFIG_WIREGUARD=y\n")}}, true, false},
		{"disabled", fstest.MapFS{"boot/config-test": {Data: []byte("# CONFIG_WIREGUARD is not set\n")}}, false, false},
		{"loadable", fstest.MapFS{
			"lib/modules/test/modules.dep":                                   {Data: []byte("kernel/drivers/net/wireguard/wireguard.ko.zst: kernel/crypto/lib.ko\n")},
			"lib/modules/test/kernel/drivers/net/wireguard/wireguard.ko.zst": {Data: []byte("module")},
		}, true, false},
		{"builtin metadata", fstest.MapFS{"lib/modules/test/modules.builtin": {Data: []byte("kernel/drivers/net/wireguard/wireguard.ko\n")}}, true, false},
		{"missing module", fstest.MapFS{"boot/config-test": {Data: []byte("CONFIG_WIREGUARD=m\n")}}, false, true},
		{"wrong kernel", fstest.MapFS{"boot/config-other": {Data: []byte("CONFIG_WIREGUARD=y\n")}}, false, true},
		{"no evidence", fstest.MapFS{}, false, true},
		{"invalid kernel release", fstest.MapFS{}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := "test"
			if tc.name == "invalid kernel release" {
				release = "../other"
			}
			got, err := kernelWireGuard(tc.files, release)
			if got != tc.supported || (err != nil) != tc.unknown {
				t.Fatalf("support=%v err=%v", got, err)
			}
		})
	}
}

func TestWireGuardVersionRejectsUnexpectedOutput(t *testing.T) {
	for _, output := range []string{"", "private-key=fake", "wireguard-tools v1.0\nextra", "wireguard-tools v1.0; touch /tmp/no"} {
		if _, err := parseWGVersion([]byte(output)); err == nil {
			t.Fatalf("accepted %q", output)
		}
	}
	got, err := parseWGVersion([]byte("wireguard-tools v1.0.20210914 - https://git.zx2c4.com/wireguard-tools/\n"))
	if err != nil || got != "1.0.20210914" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestWireGuardReadinessThroughAuthenticatedSocket(t *testing.T) {
	e, d, _, _ := fixture(t)
	e.driver = readinessDriver{d}
	path := filepath.Join(t.TempDir(), "helper.sock")
	l, err := ListenUnix(path, os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeUnix(ctx, l, e, uint32(os.Getuid())) }()
	defer func() { cancel(); l.Close(); <-done }()
	s, err := NewClient(path, "test-host").WireGuard(ctx)
	if err != nil || !s.Reported || s.MeasuredBy != "test-host" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestWireGuardSystemReadinessDoesNotChangeNetwork(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("disposable Linux fixture only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("container required")
	}
	n, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx := context.Background()
	before, err := n.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The production probe must ignore inherited executable search paths.
	t.Setenv("PATH", t.TempDir())
	s, err := n.WireGuard(ctx)
	if err != nil || !s.NetworkManagerAvailable || s.NetworkManagerVersion == "" || !s.HelperAvailable || s.CanInstall || len(s.Tools) != 1 {
		t.Fatalf("%+v %v", s, err)
	}
	if !s.Tools[0].Installed || s.Tools[0].Version == "" {
		t.Fatal("installed tool lacks version")
	}
	if !s.Reported && s.Detail == "" {
		t.Fatal("unknown kernel needs diagnostic")
	}
	after, err := n.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("readiness changed device inventory")
	}
	for i := range before {
		before[i].Counters = after[i].Counters
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("readiness changed network state")
	}
}
