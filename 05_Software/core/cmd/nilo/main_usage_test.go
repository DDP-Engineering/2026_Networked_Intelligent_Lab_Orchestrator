package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// An operator at the console discovers the command groups from the root usage,
// so every group and both administration modes have to be named there.
func TestRootUsageNamesEveryCommandGroup(t *testing.T) {
	binary := buildNilo(t)

	command := exec.Command(binary, "wrong-command")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 2 {
		t.Fatalf("unknown command error = %v, want exit code 2", err)
	}
	for _, want := range []string{
		"nilo version", "nilo discover", "nilo open", "nilo serve",
		"nilo serve-foundation-check", "nilo development", "nilo seed-admin",
		"nilo maintenance", "nilo admin", "nilo network", "nilo plugin",
		"nilo bench", "nilo activity",
		"--data-dir", "--server", "--token-file", "--management-interface", "--management-port", "--management-reconfigure",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("root usage does not name %q:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "nilo diagnostics") {
		t.Fatalf("root usage still names the removed diagnostics group:\n%s", stderr.String())
	}
}

func TestServeManagementOptionsRejectAmbiguousEmptyAndUnsafeConfigurations(t *testing.T) {
	binary := buildNilo(t)
	tests := []struct {
		name      string
		arguments []string
		message   string
	}{
		{
			name:      "legacy bind mixed with allowlist",
			arguments: []string{"serve", "--profile", "appliance", "--data-dir", t.TempDir(), "--bind", "192.168.1.2:8443", "--management-interface", "wlan0"},
			message:   "cannot be combined",
		},
		{
			name:      "simulation wildcard management",
			arguments: []string{"serve", "--profile", "simulation", "--data-dir", t.TempDir(), "--management-interface", "wlan0", "--management-tls-name", "ddp-remote.local"},
			message:   "simulation requires a literal loopback",
		},
		{
			name:      "unsafe interface name",
			arguments: []string{"serve", "--profile", "appliance", "--data-dir", t.TempDir(), "--management-interface", "eth0;flush", "--management-tls-name", "ddp-remote.local"},
			message:   "invalid management interface",
		},
		{
			name:      "missing stable TLS identity",
			arguments: []string{"serve", "--profile", "appliance", "--data-dir", t.TempDir(), "--management-interface", "wlan0"},
			message:   "requires at least one --management-tls-name",
		},
		{
			name:      "reconfigure without an interface list",
			arguments: []string{"serve", "--profile", "appliance", "--data-dir", t.TempDir(), "--management-reconfigure"},
			message:   "--management-reconfigure requires",
		},
		{
			name:      "TLS identity without an interface list",
			arguments: []string{"serve", "--profile", "appliance", "--data-dir", t.TempDir(), "--management-tls-name", "ddp-remote.local"},
			message:   "--management-tls-name requires",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(binary, test.arguments...)
			output, err := command.CombinedOutput()
			exitError, ok := err.(*exec.ExitError)
			if !ok || exitError.ExitCode() != 2 {
				t.Fatalf("error=%v, want exit 2; output=%s", err, output)
			}
			if !strings.Contains(string(output), test.message) {
				t.Fatalf("output=%q, want %q", output, test.message)
			}
		})
	}
}
