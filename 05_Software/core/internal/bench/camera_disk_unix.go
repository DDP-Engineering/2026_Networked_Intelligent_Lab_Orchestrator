//go:build !windows

package bench

import "golang.org/x/sys/unix"

func cameraDiskSpace(path string) (int64, int64, error) {
	var value unix.Statfs_t
	if err := unix.Statfs(path, &value); err != nil {
		return 0, 0, err
	}
	return int64(value.Blocks) * int64(value.Bsize), int64(value.Bavail) * int64(value.Bsize), nil
}
