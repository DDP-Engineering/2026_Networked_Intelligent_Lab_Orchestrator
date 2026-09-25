//go:build linux

package maintenance

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func captureFileMetadata(path string, info fs.FileInfo) (int, int, map[string]string, error) {
	details, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, nil, errors.New("file ownership metadata is unavailable")
	}
	attributes := make(map[string]string)
	size, err := unix.Listxattr(path, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return int(details.Uid), int(details.Gid), attributes, nil
	}
	if err != nil {
		return 0, 0, nil, err
	}
	if size > 0 {
		buffer := make([]byte, size)
		length, listErr := unix.Listxattr(path, buffer)
		if listErr != nil {
			return 0, 0, nil, listErr
		}
		for _, name := range strings.Split(strings.TrimRight(string(buffer[:length]), "\x00"), "\x00") {
			if name == "" || (!strings.HasPrefix(name, "user.") && name != "system.posix_acl_access" && name != "system.posix_acl_default") {
				continue
			}
			valueSize, getErr := unix.Getxattr(path, name, nil)
			if getErr != nil {
				return 0, 0, nil, getErr
			}
			value := make([]byte, valueSize)
			valueLength, getErr := unix.Getxattr(path, name, value)
			if getErr != nil {
				return 0, 0, nil, getErr
			}
			attributes[name] = base64.StdEncoding.EncodeToString(value[:valueLength])
		}
	}
	return int(details.Uid), int(details.Gid), attributes, nil
}

func applyFileMetadata(path string, mode fs.FileMode, uid, gid int, attributes map[string]string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	details, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("file ownership metadata is unavailable")
	}
	if int(details.Uid) != uid || int(details.Gid) != gid {
		if err := os.Chown(path, uid, gid); err != nil {
			return err
		}
	}
	if err := os.Chmod(path, mode.Perm()); err != nil {
		return err
	}
	current, err := captureAllowedXAttrs(path)
	if err != nil {
		return err
	}
	for name := range current {
		if _, retained := attributes[name]; retained {
			continue
		}
		if err := unix.Removexattr(path, name); err != nil && !errors.Is(err, unix.ENODATA) {
			return err
		}
	}
	for name, encoded := range attributes {
		value, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			return decodeErr
		}
		if err := unix.Setxattr(path, name, value, 0); err != nil {
			return err
		}
	}
	return nil
}

func captureAllowedXAttrs(path string) (map[string]struct{}, error) {
	attributes := make(map[string]struct{})
	size, err := unix.Listxattr(path, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return attributes, nil
	}
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return attributes, nil
	}
	buffer := make([]byte, size)
	length, err := unix.Listxattr(path, buffer)
	if err != nil {
		return nil, err
	}
	for _, name := range strings.Split(strings.TrimRight(string(buffer[:length]), "\x00"), "\x00") {
		if strings.HasPrefix(name, "user.") || name == "system.posix_acl_access" || name == "system.posix_acl_default" {
			attributes[name] = struct{}{}
		}
	}
	return attributes, nil
}
