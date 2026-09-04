package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"nilo.local/nilo/internal/apiclient"
)

// Remote administration is the local command set spoken to the appliance
// instead of to its data directory. It is an API client like the network
// commands: the appliance resolves the bearer token, checks the route's
// permission, applies every identity rule and records every outcome. Nothing
// here decides who may do what, and no flag ever carries a credential.

const (
	adminModeLocal  = "local"
	adminModeRemote = "remote"
)

// adminOptions are the mode-selecting options every user and token command
// shares. There is deliberately no token flag and no password flag.
type adminOptions struct {
	dataDir       string
	server        string
	tokenFile     string
	caFile        string
	asJSON        bool
	confirmDelete bool
}

func (options *adminOptions) bind(flags *flag.FlagSet) {
	flags.StringVar(&options.dataDir, "data-dir", "", "local mode: absolute runtime data directory")
	flags.StringVar(&options.server, "server", "", "remote mode: appliance API address")
	flags.StringVar(&options.tokenFile, "token-file", "", "remote mode: file holding the API token")
	flags.StringVar(&options.caFile, "ca-file", "", "remote mode: certificate authority for a private listener")
	flags.BoolVar(&options.asJSON, "json", false, "remote mode: print the appliance's own JSON document")
	flags.BoolVar(&options.confirmDelete, "confirm-delete", false, "remote mode: consent to a permanent deletion without a prompt")
}

// mode decides which of the two modes the options select, and refuses a
// combination that is ambiguous or incomplete before anything is opened or
// contacted. The actor is a local-mode notion: a remote change is attributed
// by the appliance to the token in use.
func (options adminOptions) mode(actor string) (string, error) {
	switch {
	case options.dataDir != "" && (options.server != "" || options.tokenFile != ""):
		return "", errors.New("use either --data-dir or --server with --token-file, not both")
	case options.dataDir != "":
		for _, remoteOnly := range []struct {
			name string
			set  bool
		}{
			{"--json", options.asJSON}, {"--confirm-delete", options.confirmDelete}, {"--ca-file", options.caFile != ""},
		} {
			if remoteOnly.set {
				return "", fmt.Errorf("%s is a remote-mode option and has no meaning with --data-dir", remoteOnly.name)
			}
		}
		return adminModeLocal, nil
	case options.server != "" && options.tokenFile != "":
		if actor != "" {
			return "", errors.New("--actor is a local-mode option: the appliance attributes a remote change to the token in use")
		}
		return adminModeRemote, nil
	case options.server != "":
		return "", errors.New("--server needs --token-file: the token is read from that file and from nowhere else")
	case options.tokenFile != "":
		return "", errors.New("--token-file needs --server")
	}
	return "", errors.New("--data-dir (local mode) or --server with --token-file (remote mode) is required")
}

// remoteAdmin performs the administration requests and prints their results.
type remoteAdmin struct {
	client  *apiclient.Client
	options adminOptions
	streams adminIO
}

func newRemoteAdmin(options adminOptions, streams adminIO) (*remoteAdmin, error) {
	client, err := apiclient.New(apiclient.Options{
		BaseURL: options.server, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &remoteAdmin{client: client, options: options, streams: streams}, nil
}

// renderer prints the readable form of one appliance document.
type renderer func(stdout io.Writer, document []byte) error

// show prints a document: the appliance's own bytes with --json, so a script
// reads exactly what the API said, and a readable rendering otherwise.
func (remote *remoteAdmin) show(document []byte, render renderer) int {
	if remote.options.asJSON {
		fmt.Fprintln(remote.streams.stdout, string(bytes.TrimSpace(document)))
		return 0
	}
	if err := render(remote.streams.stdout, document); err != nil {
		fmt.Fprintln(remote.streams.stderr, "the appliance returned a document this command cannot read")
		return 1
	}
	return 0
}

func (remote *remoteAdmin) read(ctx context.Context, path string, render renderer) int {
	document, err := remote.client.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		fmt.Fprintln(remote.streams.stderr, err)
		return 1
	}
	return remote.show(document, render)
}

// apply sends one change and then re-reads the listing, so the operator sees
// the state the appliance now holds rather than an empty 204, exactly as the
// local commands print the listing after a change.
func (remote *remoteAdmin) apply(
	ctx context.Context, method, path string, body any, listing string, render renderer,
) int {
	if _, err := remote.client.Do(ctx, method, path, body); err != nil {
		fmt.Fprintln(remote.streams.stderr, err)
		return 1
	}
	fmt.Fprintln(remote.streams.stderr, "change applied")
	return remote.readAfterChange(ctx, listing, render)
}

// readAfterChange re-reads the listing once a change has been applied. The
// change is already the appliance's state, so a listing that cannot be read
// or rendered is reported as exactly that and never turns the applied change
// into a reported failure: the exit code stays 0.
func (remote *remoteAdmin) readAfterChange(ctx context.Context, listing string, render renderer) int {
	document, err := remote.client.Do(ctx, http.MethodGet, listing, nil)
	if err != nil {
		fmt.Fprintf(remote.streams.stderr, "the change was applied, but the listing could not be re-read: %v\n", err)
		return 0
	}
	if remote.options.asJSON {
		fmt.Fprintln(remote.streams.stdout, string(bytes.TrimSpace(document)))
		return 0
	}
	if err := render(remote.streams.stdout, document); err != nil {
		fmt.Fprintln(remote.streams.stderr, "the change was applied, but the listing the appliance returned cannot be rendered")
	}
	return 0
}

// isForbidden reports a usable credential that lacks the route's permission.
func isForbidden(err error) bool {
	var refusal *apiclient.Error
	return errors.As(err, &refusal) && refusal.Status == http.StatusForbidden
}

// runRemoteAdminUser implements the remote `nilo admin user` subcommands over
// the identity administration routes.
func runRemoteAdminUser(
	ctx context.Context, subcommand string, options adminOptions, username, role string, streams adminIO,
) int {
	username = foldUsername(username)
	switch subcommand {
	case "list":
	case "create", "set-role":
		if username == "" || role == "" {
			fmt.Fprintln(streams.stderr, adminUsage)
			return 2
		}
	case "enable", "disable", "reset-password", "delete":
		if username == "" {
			fmt.Fprintln(streams.stderr, adminUsage)
			return 2
		}
	default:
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}

	// The password is read before the appliance is contacted, exactly as the
	// local command reads it, and goes from here into the request body only.
	var password []byte
	if subcommand == "create" || subcommand == "reset-password" {
		read, err := readConfirmedPassword(streams)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 2
		}
		password = read
	}

	remote, err := newRemoteAdmin(options, streams)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	const listing = "/api/v1/users"
	subject := listing + "/" + url.PathEscape(username)
	switch subcommand {
	case "list":
		return remote.read(ctx, listing, renderUserListing)
	case "create":
		document, err := remote.client.Do(ctx, http.MethodPost, listing, map[string]any{
			"username": username, "role": role, "password": string(password),
		})
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return remote.show(document, renderCreatedUser)
	case "set-role":
		return remote.apply(ctx, http.MethodPut, subject+"/role", map[string]any{"role": role}, listing, renderUserListing)
	case "enable":
		return remote.apply(ctx, http.MethodPut, subject+"/enabled", map[string]any{"enabled": true}, listing, renderUserListing)
	case "disable":
		return remote.apply(ctx, http.MethodPut, subject+"/enabled", map[string]any{"enabled": false}, listing, renderUserListing)
	case "reset-password":
		return remote.apply(ctx, http.MethodPut, subject+"/password", map[string]any{"password": string(password)}, listing, renderUserListing)
	default:
		return remote.deleteSubject(ctx, deletionSubject{
			kind: "user", prompt: "username", id: username, fold: true,
			listing: listing, path: subject, describe: describeUser, render: renderUserListing,
		})
	}
}

// runRemoteAdminToken implements the remote `nilo admin token` subcommands.
func runRemoteAdminToken(
	ctx context.Context, subcommand string, options adminOptions,
	owner, name, scopes string, expiresIn time.Duration, tokenID string, streams adminIO,
) int {
	switch subcommand {
	case "list":
	case "create":
		if owner == "" || name == "" {
			fmt.Fprintln(streams.stderr, "token create requires --owner and --name")
			return 2
		}
	case "revoke", "delete":
		if tokenID == "" {
			fmt.Fprintln(streams.stderr, "token "+subcommand+" requires --id")
			return 2
		}
	default:
		fmt.Fprintln(streams.stderr, adminUsage)
		return 2
	}

	remote, err := newRemoteAdmin(options, streams)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	const listing = "/api/v1/tokens"
	switch subcommand {
	case "list":
		return remote.read(ctx, listing, renderTokenListing)
	case "create":
		body := map[string]any{"owner": owner, "name": name, "scopes": splitScopes(scopes)}
		if expiresIn > 0 {
			body["expires_at"] = time.Now().UTC().Add(expiresIn).Format(time.RFC3339)
		}
		document, err := remote.client.Do(ctx, http.MethodPost, listing, body)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		// The one moment the secret exists outside the appliance. The warning
		// goes to standard error in both modes so --json output stays pure.
		fmt.Fprintln(streams.stderr, "The token secret is shown once and cannot be retrieved later; store it now.")
		return remote.show(document, renderIssuedToken)
	case "revoke":
		return remote.apply(ctx, http.MethodPost, listing+"/"+url.PathEscape(tokenID)+"/revoke",
			map[string]any{}, listing, renderTokenListing)
	default:
		return remote.deleteSubject(ctx, deletionSubject{
			kind: "API token", prompt: "token ID", id: tokenID,
			listing: listing, path: listing + "/" + url.PathEscape(tokenID),
			describe: describeToken, render: renderTokenListing,
		})
	}
}

// deletionSubject is what a permanent deletion needs to describe and remove
// one identity or token.
type deletionSubject struct {
	kind     string
	prompt   string
	id       string
	listing  string
	path     string
	describe func(document []byte, id string) (string, bool)
	render   renderer
	// fold is true for a username, which the appliance folds to lower case.
	fold bool
}

// deleteSubject is the destructive path. It states exactly what would be
// removed, then sends the deletion only with explicit consent: the subject
// typed back on a terminal, or --confirm-delete given up front. Nothing but
// that flag counts, and a run that is refused or cancelled has made only the
// listing read.
func (remote *remoteAdmin) deleteSubject(ctx context.Context, subject deletionSubject) int {
	listing, err := remote.client.Do(ctx, http.MethodGet, subject.listing, nil)
	switch {
	case err == nil:
		if description, listed := subject.describe(listing, subject.id); listed {
			fmt.Fprintln(remote.streams.stderr, description)
		} else {
			// An unlisted subject is still sent: an already-deleted or mistyped
			// name gets the appliance's own answer rather than this command's guess.
			fmt.Fprintf(remote.streams.stderr, "No listed %s matches %q; the appliance decides.\n", subject.kind, subject.id)
		}
	case isForbidden(err):
		// The listing is a courtesy, not a gate. A token narrowed to the manage
		// permission alone may delete without being allowed to list, and the
		// appliance, not this command, decides that.
		fmt.Fprintf(remote.streams.stderr,
			"This token may not read the %s listing (forbidden), so the subject cannot be described here; the appliance decides on %s %q.\n",
			subject.kind, subject.kind, subject.id)
	default:
		// Anything else, unreachable or unusable, is a stop: nothing destructive
		// is attempted on the strength of a prompt alone.
		fmt.Fprintln(remote.streams.stderr, err)
		return 1
	}
	if !remote.options.confirmDelete {
		if !remote.streams.interactive || remote.streams.stdin == nil {
			fmt.Fprintln(remote.streams.stderr, "noninteractive deletion requires --confirm-delete; nothing deleted")
			return 2
		}
		fmt.Fprintf(remote.streams.stderr, "Type the %s to confirm: ", subject.prompt)
		answer, _ := bufio.NewReader(remote.streams.stdin).ReadString('\n')
		answer = strings.TrimRight(answer, "\r\n")
		if subject.fold {
			answer = foldUsername(answer)
		}
		if answer != subject.id {
			fmt.Fprintln(remote.streams.stderr, "deletion cancelled; nothing deleted")
			return 1
		}
	}
	if _, err := remote.client.Do(ctx, http.MethodDelete, subject.path, nil); err != nil {
		fmt.Fprintln(remote.streams.stderr, err)
		return 1
	}
	fmt.Fprintf(remote.streams.stderr, "deleted %s %q\n", subject.kind, subject.id)
	return remote.readAfterChange(ctx, subject.listing, subject.render)
}

// foldUsername folds a username the way the appliance folds it, so the
// command describes, addresses and confirms the identity the appliance would
// actually act on. Validation stays with the appliance.
func foldUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// splitScopes turns "a, b" into ["a","b"]; the appliance validates each one.
// The result is never nil, so an empty list is sent as [] and read as "not
// narrowed".
func splitScopes(text string) []string {
	scopes := make([]string, 0)
	for _, scope := range strings.Split(text, ",") {
		if trimmed := strings.TrimSpace(scope); trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	return scopes
}

// The readable renderings. --json is the stable contract; these may change.

type remoteUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
}

func (user remoteUser) state() string {
	if user.Enabled {
		return "enabled"
	}
	return "disabled"
}

type remoteToken struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Owner     string     `json:"owner"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
	Revoked   bool       `json:"revoked"`
}

func (token remoteToken) state() string {
	switch {
	case token.Revoked:
		return "revoked"
	case token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now()):
		return "expired"
	}
	return "active"
}

func (token remoteToken) expiry() string {
	if token.ExpiresAt == nil {
		return "never"
	}
	return token.ExpiresAt.UTC().Format(time.RFC3339)
}

func decodeUsers(document []byte) ([]remoteUser, error) {
	var listing struct {
		Items []remoteUser `json:"items"`
	}
	if err := json.Unmarshal(document, &listing); err != nil {
		return nil, err
	}
	return listing.Items, nil
}

func decodeTokens(document []byte) ([]remoteToken, error) {
	var listing struct {
		Items []remoteToken `json:"items"`
	}
	if err := json.Unmarshal(document, &listing); err != nil {
		return nil, err
	}
	return listing.Items, nil
}

func renderUserListing(stdout io.Writer, document []byte) error {
	users, err := decodeUsers(document)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "USERNAME\tROLE\tSTATE\tID")
	for _, user := range users {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", user.Username, user.Role, user.state(), user.ID)
	}
	return table.Flush()
}

func renderCreatedUser(stdout io.Writer, document []byte) error {
	var user remoteUser
	if err := json.Unmarshal(document, &user); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "created user %q (%s, id %s)\n", user.Username, user.Role, user.ID)
	return err
}

func renderTokenListing(stdout io.Writer, document []byte) error {
	tokens, err := decodeTokens(document)
	if err != nil {
		return err
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tOWNER\tSTATE\tEXPIRES")
	for _, token := range tokens {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", token.ID, token.Name, token.Owner, token.state(), token.expiry())
	}
	return table.Flush()
}

// renderIssuedToken prints the new token's metadata and, last, its secret:
// the only place any admin command ever prints one.
func renderIssuedToken(stdout io.Writer, document []byte) error {
	var issued struct {
		Token  remoteToken `json:"token"`
		Secret string      `json:"secret"`
	}
	if err := json.Unmarshal(document, &issued); err != nil {
		return err
	}
	scopes := "none (the owner's full role)"
	if len(issued.Token.Scopes) > 0 {
		scopes = strings.Join(issued.Token.Scopes, ",")
	}
	_, err := fmt.Fprintf(stdout, "token:   %s\nname:    %s\nowner:   %s\nscopes:  %s\nexpires: %s\nsecret:  %s\n",
		issued.Token.ID, issued.Token.Name, issued.Token.Owner, scopes, issued.Token.expiry(), issued.Secret)
	return err
}

// describeUser states exactly which identity a deletion would remove.
func describeUser(document []byte, username string) (string, bool) {
	users, err := decodeUsers(document)
	if err != nil {
		return "", false
	}
	for _, user := range users {
		if user.Username == username {
			return fmt.Sprintf("This permanently deletes user %q (id %s, role %s, %s). Its browser sessions are removed with it.",
				user.Username, user.ID, user.Role, user.state()), true
		}
	}
	return "", false
}

// describeToken states exactly which token record a deletion would remove.
func describeToken(document []byte, id string) (string, bool) {
	tokens, err := decodeTokens(document)
	if err != nil {
		return "", false
	}
	for _, token := range tokens {
		if token.ID == id {
			return fmt.Sprintf("This permanently deletes API token %s (name %q, owner %s, %s, expires %s). The record cannot be recovered.",
				token.ID, token.Name, token.Owner, token.state(), token.expiry()), true
		}
	}
	return "", false
}
