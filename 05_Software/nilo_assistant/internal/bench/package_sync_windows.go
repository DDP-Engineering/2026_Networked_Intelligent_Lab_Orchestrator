//go:build windows

package bench

import (
	"errors"
	"os"
)

func syncPackageDirectory(string) error { return nil }

func syncPackageFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
