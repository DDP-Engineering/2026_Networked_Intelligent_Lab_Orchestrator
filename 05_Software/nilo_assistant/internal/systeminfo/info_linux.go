//go:build linux

package systeminfo

import (
	"os"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

func platformInfo(dataPath string) Info {
	info := Info{OperatingSystem: "Linux", Architecture: runtime.GOARCH}
	if contents, err := os.ReadFile("/etc/os-release"); err == nil {
		if release := parseOSRelease(contents); release != "" {
			info.OperatingSystem = release
		}
	}

	var name unix.Utsname
	if unix.Uname(&name) == nil {
		var release strings.Builder
		for _, character := range name.Release {
			if character == 0 {
				break
			}
			release.WriteByte(byte(character))
		}
		info.KernelVersion = release.String()
	}

	var filesystem unix.Statfs_t
	if unix.Statfs(dataPath, &filesystem) == nil && filesystem.Bsize > 0 {
		blockSize := uint64(filesystem.Bsize)
		info.StorageFreeBytes = uint64(filesystem.Bavail) * blockSize
		info.StorageTotalBytes = filesystem.Blocks * blockSize
	}
	return info
}
