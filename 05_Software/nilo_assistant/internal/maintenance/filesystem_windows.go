//go:build windows

package maintenance

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func availableBytes(path string) (uint64, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, nil, nil); err != nil {
		return 0, err
	}
	return available, nil
}

func syncDirectory(string) error {
	// Windows directory handles cannot be opened through os.Open for flushing.
	// File contents are flushed before each rename; MoveFileEx is used by Go's
	// os.Rename implementation for the metadata transition.
	return nil
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
