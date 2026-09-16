//go:build linux

package main

import "os/exec"

func defaultBrowserLauncher(target string) error {
	return exec.Command("xdg-open", target).Start()
}
