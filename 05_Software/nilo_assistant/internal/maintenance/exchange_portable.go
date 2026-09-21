package maintenance

import (
	"errors"
	"io/fs"
	"os"
)

type renameDirectory func(string, string) error

// exchangeDirectoriesPortable emulates an exchange and restores both names if
// the final rename fails. Callers must hold the sibling exclusion marker.
func exchangeDirectoriesPortable(left, right string, rename renameDirectory) error {
	backup := right + ".exchange-backup"
	if _, err := os.Lstat(backup); err == nil {
		return errors.New("portable exchange backup already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := rename(left, backup); err != nil {
		return err
	}
	if err := rename(right, left); err != nil {
		return errors.Join(err, rename(backup, left))
	}
	if err := rename(backup, right); err != nil {
		return errors.Join(err, rename(left, right), rename(backup, left))
	}
	return nil
}
