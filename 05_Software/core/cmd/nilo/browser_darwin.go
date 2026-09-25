//go:build darwin

package main

import "os/exec"

func defaultBrowserLauncher(target string) error {
	return exec.Command("open", target).Start()
}
