//go:build windows

package main

import "os"

func seedPasswordOwnerAllowed(os.FileInfo) bool { return true }

func syncSeedPasswordDirectory(string) error { return nil }
