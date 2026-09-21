//go:build !linux

package maintenance

import (
	"io/fs"
	"os"
)

func captureFileMetadata(string, fs.FileInfo) (int, int, map[string]string, error) {
	return -1, -1, map[string]string{}, nil
}

func applyFileMetadata(path string, mode fs.FileMode, _ int, _ int, _ map[string]string) error {
	return os.Chmod(path, mode.Perm())
}
