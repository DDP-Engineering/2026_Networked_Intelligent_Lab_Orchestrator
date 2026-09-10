//go:build linux

package nethelper

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestManagementFirewallAllowsOnlyNamedIngressInterfaces(t *testing.T) {
	script := renderManagementAccess(network.ManagementAccessConfiguration{
		Interfaces: []string{"wlan0", "nilo0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
	})
	for _, wanted := range []string{
		`table inet nilo_management {}`,
		`comment "managed by nilo-nethelper for management access"`,
		`iifname { "wlan0", "nilo0" } tcp dport 8443 accept`,
		`tcp dport 8443 drop`,
	} {
		if !strings.Contains(script, wanted) {
			t.Errorf("missing %q in:\n%s", wanted, script)
		}
	}
	if strings.Contains(script, `"eth0"`) || strings.Contains(script, "0.0.0.0") {
		t.Errorf("management firewall exposed an unlisted path:\n%s", script)
	}
	if strings.Index(script, `iifname { "wlan0", "nilo0" }`) > strings.LastIndex(script, "tcp dport 8443 drop") {
		t.Fatal("allow rules must precede the port-wide deny")
	}
}

func TestManagementFirewallSystem(t *testing.T) {
	systemTunnelGate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	t.Cleanup(func() {
		_, _ = runNft(context.Background(), "", "delete", "table", "inet", "nilo_management")
	})
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"wlan0", "nilo0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
	}
	if err := engine.ApplyManagementAccess(ctx, configuration); err != nil {
		t.Fatalf("apply management access: %v", err)
	}
	if state, err := driver.ManagementAccessTable(ctx); err != nil || state != TableOwned {
		t.Fatalf("management table state=%q err=%v", state, err)
	}
	output, err := runNft(ctx, "", "list", "table", "inet", "nilo_management")
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{`iifname { "wlan0", "nilo0" } tcp dport 8443 accept`, `tcp dport 8443 drop`} {
		if !strings.Contains(output, wanted) {
			t.Fatalf("installed table is missing %q:\n%s", wanted, output)
		}
	}

	if _, err := runNft(ctx, "", "delete", "table", "inet", "nilo_management"); err != nil {
		t.Fatal(err)
	}
	if _, err := runNft(ctx, "", "add", "table", "inet", "nilo_management"); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyManagementAccess(ctx, configuration); !errors.Is(err, ErrManagementAccessForeign) {
		t.Fatalf("foreign table error=%v", err)
	}
}
