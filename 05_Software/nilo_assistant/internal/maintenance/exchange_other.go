//go:build !linux

package maintenance

import (
	"os"
	"path/filepath"
)

// Non-Linux hosts cannot rename a directory containing the open lock file.
// Production restore targets are Linux; portable development hosts release
// the cooperative lock immediately before the best-effort rename sequence.
func releaseBeforeDirectoryExchange() bool { return true }

func exchangeDirectories(left, right string) error {
	if err := exchangeDirectoriesPortable(left, right, os.Rename); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(left))
}
