//go:build linux

package maintenance

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

func releaseBeforeDirectoryExchange() bool { return false }

func exchangeDirectories(left, right string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, left, unix.AT_FDCWD, right, unix.RENAME_EXCHANGE); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(left))
}
