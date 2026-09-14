//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
	"nilo.local/nilo/internal/nethelper"
)

func privateDirectory(path string, mode os.FileMode) error {
	if !filepath.IsAbs(path) {
		return errors.New("helper directories must be absolute")
	}
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return err
	}
	if stat.Uid != 0 || stat.Mode&unix.S_IFMT != unix.S_IFDIR || os.FileMode(stat.Mode).Perm()&^mode != 0 {
		return errors.New("helper directory must be root-owned with restricted permissions")
	}
	return nil
}
func serve(ctx context.Context, o options) error {
	if os.Geteuid() != 0 {
		return errors.New("the explicitly installed helper must run as root")
	}
	if err := privateDirectory(o.state, 0700); err != nil {
		return err
	}
	if err := privateDirectory(filepath.Dir(o.socket), 0750); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(o.state, "helper.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("another helper owns recovery")
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	store, err := nethelper.NewFileStore(o.state)
	if err != nil {
		return err
	}
	driver, err := nethelper.NewNetworkManager()
	if err != nil {
		return err
	}
	defer driver.Close()
	engine, err := nethelper.NewEngine(driver, store, time.Now)
	if err != nil {
		return err
	}
	powerController, err := nethelper.NewLoginManager()
	if err != nil {
		return err
	}
	defer powerController.Close()
	if err = engine.ConfigurePower(powerController); err != nil {
		return err
	}
	if o.raspberryPiRecovery {
		hardware, hardwareErr := nethelper.NewPiRecoveryHardware(ctx, "/dev/gpiochip0", store, driver)
		if hardwareErr != nil {
			return hardwareErr
		}
		defer hardware.Close()
		if err = engine.ConfigureRecoveryHardware(hardware); err != nil {
			return err
		}
	}
	// Key storage is a subdirectory of the helper's own private state, created
	// here rather than by the keystore itself: deciding where privileged storage
	// lives belongs to startup, not to the code that writes into it.
	secretDirectory := filepath.Join(o.state, "secrets")
	if err = os.MkdirAll(secretDirectory, 0700); err != nil {
		return err
	}
	if err = os.Chmod(secretDirectory, 0700); err != nil {
		return err
	}
	if err = privateDirectory(secretDirectory, 0700); err != nil {
		return err
	}
	keystore, err := nethelper.NewKeystore(secretDirectory)
	if err != nil {
		return err
	}
	if err = engine.ConfigureKeystore(keystore); err != nil {
		return err
	}
	// An import interrupted by a crash has nobody left to discard it.
	if swept, err := engine.SweepStagedSecrets(); err != nil {
		return err
	} else if swept > 0 {
		fmt.Fprintf(os.Stderr, "network helper: removed %d unfinished profile import(s)\n", swept)
	}

	recovery, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = engine.Reconcile(recovery)
	cancel()
	if err != nil {
		return fmt.Errorf("startup recovery blocks management: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	installer, err := nethelper.NewAPTInstaller(o.state, executable, driver)
	if err != nil {
		return err
	}
	if err = engine.ConfigureInstaller(installer, store); err != nil {
		return err
	}
	listener, err := nethelper.ListenUnix(o.socket, o.gid)
	if err != nil {
		return err
	}
	defer listener.Close()
	timerCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	installDone := make(chan struct{})
	go func() { defer close(installDone); engine.RunInstaller(timerCtx) }()
	go func() {
		defer close(done)
		engine.RunRecovery(timerCtx, func(err error) { fmt.Fprintln(os.Stderr, "network recovery:", err) })
	}()
	err = nethelper.ServeUnix(ctx, listener, engine, uint32(o.uid))
	stop()
	<-done
	<-installDone
	return err
}
