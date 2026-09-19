package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

const seedAdminUsage = "usage: nilo seed-admin --data-dir <absolute-path> --username <name> --password-file <absolute-path>"

// runSeedAdmin performs the unattended first-boot form of administrator
// bootstrap. Its password comes only from a protected regular file, and the
// file is consumed only after the administrator and audit record are durable.
func runSeedAdmin(ctx context.Context, arguments []string, streams adminIO) int {
	flags := flag.NewFlagSet("seed-admin", flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	username := flags.String("username", "", "username of the first administrator")
	passwordFile := flags.String("password-file", "", "protected file containing the password")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *username == "" {
		fmt.Fprintln(streams.stderr, seedAdminUsage)
		return 2
	}
	if !filepath.IsAbs(*dataDir) || !filepath.IsAbs(*passwordFile) {
		fmt.Fprintln(streams.stderr, "--data-dir and --password-file must be absolute paths")
		return 2
	}

	password, passwordInfo, err := readSeedPassword(*passwordFile)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	defer clear(password)

	application, err := app.Open(ctx, app.Options{
		DataDir: *dataDir, BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	created, runErr := application.IdentityService().Bootstrap(ctx, *username, password)
	if errors.Is(runErr, identity.ErrAdministratorExists) {
		users, listErr := application.IdentityService().ListUsers(ctx)
		if listErr != nil {
			runErr = errors.Join(runErr, listErr)
		} else {
			for _, user := range users {
				if user.Username == *username && user.Role == auth.RoleAdministrator && user.Enabled() {
					created = user
					runErr = nil
					break
				}
			}
		}
	}
	closeErr := application.Close()
	if runErr != nil {
		fmt.Fprintln(streams.stderr, runErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(streams.stderr, closeErr)
		return 1
	}
	if err := consumeSeedPassword(*passwordFile, passwordInfo); err != nil {
		fmt.Fprintln(streams.stderr, "administrator was created but the password file could not be removed:", err)
		return 1
	}
	if err := writeAdminJSON(streams.stdout, struct {
		UserID   string    `json:"user_id"`
		Username string    `json:"username"`
		Role     auth.Role `json:"role"`
	}{UserID: created.ID, Username: created.Username, Role: created.Role}); err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return 0
}

func readSeedPassword(path string) ([]byte, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect password file: %w", err)
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("password file must be a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return nil, nil, errors.New("password file must not grant group or other permissions")
	}
	if !seedPasswordOwnerAllowed(before) {
		return nil, nil, errors.New("password file must be owned by root or the current user")
	}
	if before.Size() < 1 || before.Size() > auth.MaxPasswordBytes+2 {
		return nil, nil, fmt.Errorf("password file must contain at most %d bytes plus one line ending", auth.MaxPasswordBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open password file: %w", err)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("inspect opened password file: %w", err)
	}
	if !os.SameFile(before, after) || !after.Mode().IsRegular() {
		return nil, nil, errors.New("password file changed while it was being opened")
	}
	contents := make([]byte, after.Size())
	if _, err := io.ReadFull(file, contents); err != nil {
		clear(contents)
		return nil, nil, fmt.Errorf("read password file: %w", err)
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r")
	clear(contents)
	if password == "" || strings.ContainsAny(password, "\r\n") {
		return nil, nil, errors.New("password file must contain exactly one non-empty line")
	}
	if len(password) > auth.MaxPasswordBytes {
		return nil, nil, fmt.Errorf("password must be at most %d bytes", auth.MaxPasswordBytes)
	}
	return []byte(password), before, nil
}

func consumeSeedPassword(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reinspect password file: %w", err)
	}
	if !os.SameFile(expected, current) || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 {
		return errors.New("password file changed before it could be consumed")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncSeedPasswordDirectory(filepath.Dir(path))
}
