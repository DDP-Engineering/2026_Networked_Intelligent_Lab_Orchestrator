package systeminfo

import "testing"

func TestParseOSReleaseUsesPrettyNameWithoutTrustingShellSyntax(t *testing.T) {
	t.Parallel()

	contents := []byte("NAME=Debian\nVERSION_ID=\"13\"\nPRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\nUNRELATED=$(uname -a)\n")
	if got := parseOSRelease(contents); got != "Debian GNU/Linux 13 (trixie)" {
		t.Fatalf("parseOSRelease() = %q", got)
	}
}

func TestParseOSReleaseFallsBackToNameAndVersion(t *testing.T) {
	t.Parallel()

	contents := []byte("NAME=Alpine Linux\nVERSION_ID=3.22\n")
	if got := parseOSRelease(contents); got != "Alpine Linux 3.22" {
		t.Fatalf("parseOSRelease() = %q", got)
	}
}
