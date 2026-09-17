//go:build !linux

package systeminfo

import (
	"runtime"
	"strings"
)

func platformInfo(string) Info {
	return Info{
		OperatingSystem: strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:],
		Architecture:    runtime.GOARCH,
	}
}
