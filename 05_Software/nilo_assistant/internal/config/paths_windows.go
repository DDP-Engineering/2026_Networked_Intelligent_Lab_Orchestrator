//go:build windows

package config

import (
	"os"
	"path/filepath"
)

func defaultDataDir() string {
	return filepath.Join(os.Getenv("ProgramData"), "NILO")
}
