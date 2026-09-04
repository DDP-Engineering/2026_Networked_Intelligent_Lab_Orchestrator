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
		"nilo serve", "nilo admin", "nilo network", "nilo activity",
		"--data-dir", "--server", "--token-file",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("root usage does not name %q:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "nilo diagnostics") {
		t.Fatalf("root usage still names the removed diagnostics group:\n%s", stderr.String())
	}
}
