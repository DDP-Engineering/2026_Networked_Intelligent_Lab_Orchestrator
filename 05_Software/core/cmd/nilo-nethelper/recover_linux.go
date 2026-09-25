//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
	"nilo.local/nilo/internal/nethelper"
)

// recoverOnce is what the root-owned systemd timer runs: one pass over the saved
// recovery record and forwarding inventory, and nothing else. It takes the
// helper's own lock without waiting: while the helper holds it the helper is
// alive and runs its own timer, so this pass has nothing to do and says so.
func recoverOnce(ctx context.Context, state string) error {
	if os.Geteuid() != 0 {
		return errors.New("independent recovery runs as root")
	}
	if err := privateDirectory(state, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(state, "helper.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		fmt.Fprintln(os.Stdout, `{"skipped":"the helper is running and owns recovery"}`)
		return nil
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	store, err := nethelper.NewFileStore(state)
	if err != nil {
		return err
	}
	driver, err := nethelper.NewNetworkManager()
	if err != nil {
		return err
	}
	defer driver.Close()
	pass, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	outcome, err := nethelper.RecoverOnce(pass, driver, store, time.Now)
	encoded, _ := json.Marshal(outcome)
	fmt.Fprintln(os.Stdout, string(encoded))
	return err
}
