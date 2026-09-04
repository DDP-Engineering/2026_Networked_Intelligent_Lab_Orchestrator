//go:build linux

package nethelper

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// TestIndependentRecoverySystem covers REQ-REL-8fa3d260 with the helper gone:
// a change is left applied with its record on disk, no helper process holds
// the lock, and the installed binary's `recover` pass - what the systemd timer
// runs - puts the interface back on real NetworkManager and reports it.
func TestIndependentRecoverySystem(t *testing.T) {
	systemTunnelGate(t)
	binary := os.Getenv("NILO_NETHELPER_BINARY")
	if binary == "" {
		t.Skip("the harness sets NILO_NETHELPER_BINARY to the built helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	state := t.TempDir()
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(state)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := driver.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target Device
	for _, device := range devices {
		if device.Interface.Name == "eth0" {
			target = device
		}
	}
	if target.Interface.ID == "" || target.Interface.State != network.StateConnected {
		t.Fatalf("eth0 is not ready: %+v", target.Interface)
	}
	if _, err := engine.Prepare(ctx, "orphaned", network.Change{Action: network.ActionInterfaceDisable, Target: target.Interface.ID}, network.ConfirmationWindow); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := engine.Apply(ctx, "orphaned"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// The process that made the change is gone; only its record remains.
	engine = nil

	pass := exec.CommandContext(ctx, binary, "recover", "--state-dir", state)
	output, err := pass.CombinedOutput()
	if err != nil {
		t.Fatalf("recover: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"restored":"orphaned"`) || !strings.Contains(string(output), `"phase":"restored"`) {
		t.Fatalf("recover reported:\n%s", output)
	}
	restored, err := driver.find(ctx, target.Interface.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameState(restored, target) {
		t.Fatalf("eth0 after recovery = %+v, before = %+v", restored.Interface, target.Interface)
	}
	// A second pass finds nothing to restore and changes nothing.
	again, err := exec.CommandContext(ctx, binary, "recover", "--state-dir", state).CombinedOutput()
	if err != nil || strings.Contains(string(again), `"restored":"`) {
		t.Fatalf("quiet pass: %v\n%s", err, again)
	}
}
