//go:build !windows

package main

import (
	"os"
	"syscall"
)

func seedPasswordOwnerAllowed(info os.FileInfo) bool {
	metadata, ok := info.Sys().(*syscall.Stat_t)
	return ok && (metadata.Uid == 0 || metadata.Uid == uint32(os.Geteuid()))
}

func syncSeedPasswordDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
