//go:build unix

package config

import (
	"os"
	"path/filepath"
)

func defaultDataDir() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "nilo")
	}
	return "/var/lib/nilo"
}
