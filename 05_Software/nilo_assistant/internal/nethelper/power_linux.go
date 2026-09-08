//go:build linux

package nethelper

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const login1 = "org.freedesktop.login1"

var login1Path = dbus.ObjectPath("/org/freedesktop/login1")

type loginManager struct {
	bus  *dbus.Conn
	call func(context.Context, string) error
}

// NewLoginManager connects to systemd-logind on the system bus.
func NewLoginManager() (*loginManager, error) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system D-Bus unavailable: %w", err)
	}
	return &loginManager{bus: bus}, nil
}

func (manager *loginManager) Close() error { return manager.bus.Close() }

func (manager *loginManager) invoke(ctx context.Context, method string) error {
	if manager.call != nil {
		return manager.call(ctx, method)
	}
	if err := manager.bus.Object(login1, login1Path).CallWithContext(ctx, method, 0, false).Err; err != nil {
		return fmt.Errorf("login1 power action: %w", err)
	}
	return nil
}

func (manager *loginManager) Restart(ctx context.Context) error {
	return manager.invoke(ctx, login1+".Manager.Reboot")
}

func (manager *loginManager) PowerOff(ctx context.Context) error {
	return manager.invoke(ctx, login1+".Manager.PowerOff")
}
