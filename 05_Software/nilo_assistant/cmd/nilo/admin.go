package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// The local administration commands treat write access to the runtime data
// directory as the authority: anyone who can open the database can already
// change anything in it. `--actor` therefore names an existing enabled
// administrator for audit attribution, not for authentication. Passwords are
// read from standard input so they never appear in an argument list, a process
// listing, or shell history, and no command ever prints one back.
type adminIO struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func runAdminCommand(ctx context.Context, arguments []string, streams adminIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	switch arguments[0] {
	case "bootstrap":
		return runAdminBootstrap(ctx, arguments[1:], streams)
	case "user":
		return runAdminUser(ctx, arguments[1:], streams)
	case "token":
		return runAdminToken(ctx, arguments[1:], streams)
	default:
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
}

const adminUsage = `usage:
  nilo admin bootstrap --data-dir <absolute-path> --username <name>
  nilo admin user list --data-dir <absolute-path>
  nilo admin user create --data-dir <absolute-path> --actor <administrator> --username <name> --role administrator|operator|viewer
  nilo admin user set-role --data-dir <absolute-path> --actor <administrator> --username <name> --role administrator|operator|viewer
  nilo admin user disable --data-dir <absolute-path> --actor <administrator> --username <name>
  nilo admin user enable --data-dir <absolute-path> --actor <administrator> --username <name>
  nilo admin user reset-password --data-dir <absolute-path> --actor <administrator> --username <name>
  nilo admin token create --data-dir <absolute-path> --actor <administrator> --owner <name> --name <label> [--scopes a,b] [--expires-in 720h]
  nilo admin token list --data-dir <absolute-path>
  nilo admin token revoke --data-dir <absolute-path> --actor <administrator> --id <token-id>

Passwords are read from standard input, never from an argument.`

// withIdentityService opens the data directory once and hands the composed
// identity service to one command. It opens the appliance rather than starting
// it: an administration command may well run beside a live server, and the
// startup reconciliation a server performs would declare that server's running
// operations interrupted and release their leases.
func withIdentityService(
	ctx context.Context, dataDir string, streams adminIO, run func(*identity.Service) error,
) int {
	if dataDir == "" {
		fmt.Fprintln(streams.stderr, "--data-dir is required")
		return 2
	}
	application, err := app.Open(ctx, app.Options{
		DataDir: dataDir, BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	runErr := run(application.IdentityService())
	closeErr := application.Close()
	if runErr != nil {
		fmt.Fprintln(streams.stderr, runErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(streams.stderr, closeErr)
		return 1
	}
	return 0
}

func runAdminBootstrap(ctx context.Context, arguments []string, streams adminIO) int {
	flags := flag.NewFlagSet("admin bootstrap", flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	username := flags.String("username", "", "username of the first administrator")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *username == "" {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	password, err := readConfirmedPassword(streams)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 2
	}
	return withIdentityService(ctx, *dataDir, streams, func(service *identity.Service) error {
		created, err := service.Bootstrap(ctx, *username, password)
		if err != nil {
			return err
		}
		return writeAdminJSON(streams.stdout, struct {
			UserID   string    `json:"user_id"`
			Username string    `json:"username"`
			Role     auth.Role `json:"role"`
		}{UserID: created.ID, Username: created.Username, Role: created.Role})
	})
}

func runAdminUser(ctx context.Context, arguments []string, streams adminIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	subcommand := arguments[0]
	flags := flag.NewFlagSet("admin user "+subcommand, flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	actor := flags.String("actor", "", "username of the enabled administrator performing the change")
	username := flags.String("username", "", "username of the affected identity")
	roleValue := flags.String("role", "", "administrator, operator or viewer")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	if subcommand != "list" && (*actor == "" || *username == "") {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}

	var password []byte
	if subcommand == "create" || subcommand == "reset-password" {
		read, err := readConfirmedPassword(streams)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 2
		}
		password = read
	}

	return withIdentityService(ctx, *dataDir, streams, func(service *identity.Service) error {
		if subcommand == "list" {
			return writeUserListing(ctx, service, streams.stdout)
		}
		administrator, err := service.Administrator(ctx, *actor)
		if err != nil {
			return err
		}
		switch subcommand {
		case "create":
			role, err := auth.ParseRole(*roleValue)
			if err != nil {
				return err
			}
			created, err := service.CreateUser(ctx, administrator, *username, role, password)
			if err != nil {
				return err
			}
			return writeAdminJSON(streams.stdout, struct {
				UserID   string    `json:"user_id"`
				Username string    `json:"username"`
				Role     auth.Role `json:"role"`
			}{UserID: created.ID, Username: created.Username, Role: created.Role})
		case "set-role":
			role, err := auth.ParseRole(*roleValue)
			if err != nil {
				return err
			}
			if err := service.SetRole(ctx, administrator, *username, role); err != nil {
				return err
			}
		case "disable":
			if err := service.SetEnabled(ctx, administrator, *username, false); err != nil {
				return err
			}
		case "enable":
			if err := service.SetEnabled(ctx, administrator, *username, true); err != nil {
				return err
			}
		case "reset-password":
			if err := service.ResetPassword(ctx, administrator, *username, password); err != nil {
				return err
			}
		default:
			return errors.New(adminUsage)
		}
		return writeUserListing(ctx, service, streams.stdout)
	})
}

func runAdminToken(ctx context.Context, arguments []string, streams adminIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}
	subcommand := arguments[0]
	flags := flag.NewFlagSet("admin token "+subcommand, flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	actor := flags.String("actor", "", "username of the enabled administrator performing the change")
	owner := flags.String("owner", "", "username the token acts as")
	name := flags.String("name", "", "human-readable token name")
	scopes := flags.String("scopes", "", "comma-separated permissions narrowing the owner's role")
	expiresIn := flags.Duration("expires-in", 0, "token lifetime, for example 720h; omit for no expiry")
	tokenID := flags.String("id", "", "identifier of the token to revoke")
	if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}

	return withIdentityService(ctx, *dataDir, streams, func(service *identity.Service) error {
		if subcommand == "list" {
			views, err := service.ListTokens(ctx)
			if err != nil {
				return err
			}
			return writeAdminJSON(streams.stdout, struct {
				Items []tokenListEntry `json:"items"`
			}{Items: tokenListEntries(views)})
		}
		if *actor == "" {
			return errors.New("--actor is required")
		}
		administrator, err := service.Administrator(ctx, *actor)
		if err != nil {
			return err
		}
		switch subcommand {
		case "create":
			if *owner == "" || *name == "" {
				return errors.New("--owner and --name are required")
			}
			parsedScopes, err := auth.ParsePermissionList(*scopes)
			if err != nil {
				return err
			}
			var expiresAt *time.Time
			if *expiresIn > 0 {
				expiry := time.Now().UTC().Add(*expiresIn)
				expiresAt = &expiry
			}
			issued, err := service.CreateToken(ctx, administrator, *owner, *name, parsedScopes, expiresAt)
			if err != nil {
				return err
			}
			// The only place a token secret is ever printed.
			return writeAdminJSON(streams.stdout, struct {
				TokenID   string            `json:"token_id"`
				Name      string            `json:"name"`
				Owner     string            `json:"owner"`
				Scopes    []auth.Permission `json:"scopes"`
				ExpiresAt *time.Time        `json:"expires_at,omitempty"`
				Token     string            `json:"token"`
			}{
				TokenID: issued.Token.ID, Name: issued.Token.Name, Owner: issued.OwnerUsername,
				Scopes: scopeList(issued.Token.Scopes), ExpiresAt: issued.Token.ExpiresAt, Token: issued.Secret,
			})
		case "revoke":
			if *tokenID == "" {
				return errors.New("--id is required")
			}
			if err := service.RevokeToken(ctx, administrator, *tokenID); err != nil {
				return err
			}
			views, err := service.ListTokens(ctx)
			if err != nil {
				return err
			}
			return writeAdminJSON(streams.stdout, struct {
				Items []tokenListEntry `json:"items"`
			}{Items: tokenListEntries(views)})
		default:
			return errors.New(adminUsage)
		}
	})
}

type tokenListEntry struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Owner     string            `json:"owner"`
	Scopes    []auth.Permission `json:"scopes"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
	Revoked   bool              `json:"revoked"`
	CreatedAt time.Time         `json:"created_at"`
}

func tokenListEntries(views []identity.TokenView) []tokenListEntry {
	entries := make([]tokenListEntry, 0, len(views))
	for _, view := range views {
		entries = append(entries, tokenListEntry{
			ID: view.Token.ID, Name: view.Token.Name, Owner: view.OwnerUsername,
			Scopes: scopeList(view.Token.Scopes), ExpiresAt: view.Token.ExpiresAt,
			Revoked: view.Token.RevokedAt != nil, CreatedAt: view.Token.CreatedAt,
		})
	}
	return entries
}

func scopeList(scopes []auth.Permission) []auth.Permission {
	if scopes == nil {
		return []auth.Permission{}
	}
	return scopes
}

func writeUserListing(ctx context.Context, service *identity.Service, stdout io.Writer) error {
	users, err := service.ListUsers(ctx)
	if err != nil {
		return err
	}
	type entry struct {
		ID        string    `json:"id"`
		Username  string    `json:"username"`
		Role      auth.Role `json:"role"`
		Enabled   bool      `json:"enabled"`
		CreatedAt time.Time `json:"created_at"`
	}
	entries := make([]entry, 0, len(users))
	for _, user := range users {
		entries = append(entries, entry{
			ID: user.ID, Username: user.Username, Role: user.Role,
			Enabled: user.Enabled(), CreatedAt: user.CreatedAt,
		})
	}
	return writeAdminJSON(stdout, struct {
		Items []entry `json:"items"`
	}{Items: entries})
}

func writeAdminJSON(stdout io.Writer, body any) error {
	if err := json.NewEncoder(stdout).Encode(body); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	return nil
}

// readConfirmedPassword reads a password and its confirmation from standard
// input. Terminal echo is not suppressed, so an interactive operator sees what
// they type; the value is never echoed back by NILO itself.
func readConfirmedPassword(streams adminIO) ([]byte, error) {
	reader := bufio.NewReader(streams.stdin)
	fmt.Fprint(streams.stderr, "Password: ")
	first, err := readPasswordLine(reader)
	if err != nil {
		return nil, err
	}
	fmt.Fprint(streams.stderr, "Confirm password: ")
	second, err := readPasswordLine(reader)
	if err != nil {
		return nil, err
	}
	if string(first) != string(second) {
		return nil, errors.New("the two passwords do not match")
	}
	return first, nil
}

func readPasswordLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return nil, fmt.Errorf("read password from standard input: %w", err)
	}
	return []byte(strings.TrimRight(line, "\r\n")), nil
}
