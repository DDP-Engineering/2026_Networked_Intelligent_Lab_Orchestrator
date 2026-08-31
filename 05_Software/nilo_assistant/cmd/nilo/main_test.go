package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNiloCLIContract(t *testing.T) {
	binary := buildNilo(t)

	version := exec.Command(binary, "version")
	output, err := version.Output()
	if err != nil {
		t.Fatalf("nilo version: %v", err)
	}

	var info struct {
		Name                string `json:"name"`
		Version             string `json:"version"`
		IntegrationProtocol string `json:"integration_protocol"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		t.Fatalf("decode nilo version output: %v", err)
	}
	if info.Name != "nilo" || info.Version != "0.1.0-dev" || info.IntegrationProtocol != "nilo.integration/1" {
		t.Fatalf("unexpected build information: %#v", info)
	}

	foundation := exec.Command(binary, "serve-foundation-check", "--data-dir", t.TempDir())
	foundationOutput, err := foundation.Output()
	if err != nil {
		t.Fatalf("nilo serve-foundation-check: %v", err)
	}
	var health struct {
		Status     string `json:"status"`
		Reconciled bool   `json:"reconciled"`
	}
	if err := json.Unmarshal(foundationOutput, &health); err != nil {
		t.Fatalf("decode foundation health: %v", err)
	}
	if health.Status != "ok" || !health.Reconciled {
		t.Fatalf("unexpected foundation health: %#v", health)
	}

	invalid := exec.Command(binary, "invalid")
	if err := invalid.Run(); err == nil {
		t.Fatal("invalid invocation succeeded")
	} else if exitError, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("invalid invocation did not return an exit error: %v", err)
	} else if exitError.ExitCode() != 2 {
		t.Fatalf("invalid invocation exit code = %d, want 2", exitError.ExitCode())
	}
}

func buildNilo(t *testing.T) string {
	t.Helper()
	name := "nilo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nilo: %v\n%s", err, output)
	}
	return binary
}
