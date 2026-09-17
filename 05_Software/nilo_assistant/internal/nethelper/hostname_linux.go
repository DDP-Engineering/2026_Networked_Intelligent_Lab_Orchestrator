//go:build linux

package nethelper

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// PersonalizeSystemHostname applies the checked exact-match decision through
// the kernel API, never a shell or caller-supplied command.
func PersonalizeSystemHostname(imageDefaults []string, serial string) (bool, error) {
	current, err := os.Hostname()
	if err != nil {
		return false, fmt.Errorf("read system hostname: %w", err)
	}
	changed, err := PersonalizeHostname(current, imageDefaults, serial, func(value string) error {
		return unix.Sethostname([]byte(value))
	})
	if err != nil {
		return false, fmt.Errorf("personalize system hostname: %w", err)
	}
	return changed, nil
}
