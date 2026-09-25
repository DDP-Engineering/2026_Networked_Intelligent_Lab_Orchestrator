//go:build linux

package nethelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// ListenUnix refuses an existing path. Startup must not unlink another service's
// socket, a symlink, or a file supplied by a different user.
func ListenUnix(path string, gid int) (*net.UnixListener, error) {
	if _, err := os.Lstat(path); err == nil {
		return nil, errors.New("helper socket path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chown(path, os.Geteuid(), gid); err == nil {
		err = os.Chmod(path, 0660)
	}
	if err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}
func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credential *unix.Ucred
	var inner error
	err = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if err != nil {
		return 0, err
	}
	if inner != nil {
		return 0, inner
	}
	return credential.Uid, nil
}

// ServeUnix checks peer identity before reading, bounds frames and concurrency,
// and joins all accepted clients before returning at shutdown.
func ServeUnix(ctx context.Context, listener *net.UnixListener, e *Engine, allowedUID uint32) error {
	var clients sync.WaitGroup
	defer clients.Wait()
	slots := make(chan struct{}, 8)
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	for {
		c, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		uid, err := peerUID(c)
		if err != nil || uid != allowedUID && uid != 0 {
			c.Close()
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			c.Close()
			continue
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer func() { <-slots }()
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(35 * time.Second))
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			stopRead := context.AfterFunc(ctx, func() { _ = c.Close() })
			defer stopRead()
			data, err := io.ReadAll(io.LimitReader(c, (64<<10)+1))
			if err != nil || len(data) > 64<<10 {
				_ = json.NewEncoder(c).Encode(Response{Error: "helper request too large or unreadable", Code: "invalid_request"})
				return
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			var q Request
			if err := decoder.Decode(&q); err != nil {
				_ = json.NewEncoder(c).Encode(Response{Error: "invalid helper request", Code: "invalid_request"})
				return
			}
			if decoder.Decode(new(any)) != io.EOF {
				_ = json.NewEncoder(c).Encode(Response{Error: "trailing helper request", Code: "invalid_request"})
				return
			}
			_ = json.NewEncoder(c).Encode(dispatch(requestCtx, e, q))
		}()
	}
}
