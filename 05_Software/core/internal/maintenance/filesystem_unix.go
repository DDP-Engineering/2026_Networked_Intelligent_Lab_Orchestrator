//go:build !windows

package maintenance

import (
	"errors"
	"os"
	"syscall"
)

func availableBytes(path string) (uint64, error) {
	var details syscall.Statfs_t
	if err := syscall.Statfs(path, &details); err != nil {
		return 0, err
	}
	return details.Bavail * uint64(details.Bsize), nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
