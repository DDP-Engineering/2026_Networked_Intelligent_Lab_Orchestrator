//go:build linux

package nethelper

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestLoginManagerMapsPowerActionsToExactDBusMethods(t *testing.T) {
	var calls []string
	manager := &loginManager{call: func(_ context.Context, method string) error {
		calls = append(calls, method)
		return nil
	}}
	if err := manager.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.PowerOff(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"org.freedesktop.login1.Manager.Reboot",
		"org.freedesktop.login1.Manager.PowerOff",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("login1 methods = %v, want %v", calls, want)
	}
}

func TestPowerClientUsesAuthenticatedHelperSocket(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	controller := &capturingSystemPower{}
	if err = engine.ConfigurePower(controller); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "power.sock")
	listener, err := ListenUnix(path, os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ServeUnix(ctx, listener, engine, uint32(os.Getuid())) }()

	client := NewClient(path, network.Source("test"))
	if err = client.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	if err = client.PowerOff(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"restart", "power_off"}; !reflect.DeepEqual(controller.calls, want) {
		t.Fatalf("socket power calls = %v, want %v", controller.calls, want)
	}
	cancel()
	_ = listener.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
