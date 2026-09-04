//go:build linux

package nethelper

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSocketAuthenticatesPeerAndRejectsUnknownOperations(t *testing.T) {
	for _, allowed := range []uint32{uint32(os.Getuid()), uint32(os.Getuid() + 1)} {
		t.Run(string(rune('a'+allowed%26)), func(t *testing.T) {
			e, d, _, _ := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "helper.sock")
			listener, err := ListenUnix(path, os.Getgid())
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() { done <- ServeUnix(ctx, listener, e, allowed) }()
			c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = c.Write([]byte("{\"op\":\"shell.run\",\"command\":\"touch forbidden\"}\n"))
			_ = c.CloseWrite()
			var response Response
			err = json.NewDecoder(c).Decode(&response)
			if allowed == uint32(os.Getuid()) {
				if err != nil || response.Error == "" {
					t.Fatalf("accepted unknown request: %+v %v", response, err)
				}
			} else if err == nil {
				t.Fatal("read request from unauthorized peer")
			}
			if len(d.events) != 0 {
				t.Fatal(d.events)
			}
			cancel()
			_ = listener.Close()
			<-done
		})
	}
}

func TestFileStoreRoundTripAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	s, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := Record{Version: 1, ID: "one", Phase: "prepared"}
	if err = s.Save(r); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil || loaded.ID != "one" {
		t.Fatalf("%+v %v", loaded, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "recovery.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	target := filepath.Join(dir, "target")
	_ = os.Rename(filepath.Join(dir, "recovery.json"), target)
	_ = os.Symlink(target, filepath.Join(dir, "recovery.json"))
	if _, err = s.Load(); err == nil {
		t.Fatal("followed recovery symlink")
	}
}

func TestSocketRejectsOversizedFramesAndStopsIdleClients(t *testing.T) {
	e, _, _, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "socket")
	l, err := ListenUnix(path, os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan error, 1)
	go func() { done <- ServeUnix(ctx, l, e, uint32(os.Getuid())) }()
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = c.Write([]byte(`{"op":"interface.list"}` + strings.Repeat(" ", 70<<10)))
	_ = c.CloseWrite()
	var response Response
	if err = json.NewDecoder(c).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error == "" {
		t.Fatal("accepted oversized frame")
	}
	idle, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked by idle peer")
	}
}
