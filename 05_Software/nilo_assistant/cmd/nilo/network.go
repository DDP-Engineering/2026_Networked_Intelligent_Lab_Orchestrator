package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"nilo.local/nilo/internal/apiclient"
)

const networkUsage = `usage: nilo network <command>

  status                              read the appliance network state
  statistics --interface ID --period 15m|1h|24h
  interface enable|disable ID         preview and acknowledge an interface change
  interface settings ID --method static|automatic [--address A/P] [--gateway G]
                        [--dns a,b] [--dns-automatic] [--protect] [--never-default]
  wifi scan ID
  wifi join ID --ssid NAME --security open|wpa2-personal|wpa3-personal
               [--password-file PATH] [--startup]
  vpn check|install
  vpn profiles list|import --file PATH --name NAME|remove ID|rename ID --name NAME
  vpn enable ID|disable|autostart ID --enabled true|false
  device-networks list|show ID|instructions ID
  device-networks create --file PATH|update ID --file PATH
  device-networks enable ID|disable ID|delete ID
  device-networks check ID [--tcp-port N]
  operation status ID                 read one managed change
  operation acknowledge ID --revision R
  operation confirm ID                keep a change inside its window
  operation rollback ID               put a change back

Every command needs --server URL and --token-file PATH. The token is read from
that file and nowhere else: there is no flag that accepts a token value, because
a command line is readable by every other process and survives in shell history.
Options: --server URL --token-file PATH [--ca-file PATH] [--json] [--acknowledge]
Noninteractive mutations require --acknowledge. Keeping connectivity is always
a later, separate operation confirm command.`

// networkIO lets a test capture what the command wrote.
type networkIO struct {
	stdout      io.Writer
	stderr      io.Writer
	stdin       io.Reader
	interactive bool
}

// networkOptions are the flags every network command shares.
type networkOptions struct {
	server      string
	tokenFile   string
	caFile      string
	asJSON      bool
	acknowledge bool
}

// bind registers the shared options. There is deliberately no token flag here.
func (options *networkOptions) bind(flags *flag.FlagSet) {
	flags.StringVar(&options.server, "server", "", "appliance API address")
	flags.StringVar(&options.tokenFile, "token-file", "", "file holding the API token")
	flags.StringVar(&options.caFile, "ca-file", "", "certificate authority for a private listener")
	flags.BoolVar(&options.asJSON, "json", false, "print the appliance's own JSON document")
	flags.BoolVar(&options.acknowledge, "acknowledge", false, "acknowledge the preview; does not confirm connectivity")
}

// runNetworkCommand implements `nilo network`. Exit codes: 0 success, 1 the
// appliance refused or the change failed, 2 a usage mistake.
func runNetworkCommand(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, networkUsage)
		return 2
	}

	switch arguments[0] {
	case "status":
		return networkRead(ctx, arguments[1:], streams, "status", "/api/v1/network/status", nil)
	case "statistics":
		return networkStatistics(ctx, arguments[1:], streams)
	case "interface":
		if len(arguments) > 1 && arguments[1] == "settings" {
			return networkSettings(ctx, arguments[2:], streams)
		}
		return networkInterface(ctx, arguments[1:], streams)
	case "wifi":
		return networkWiFi(ctx, arguments[1:], streams)
	case "vpn":
		return networkVPN(ctx, arguments[1:], streams)
	case "device-networks":
		return networkDeviceNetworks(ctx, arguments[1:], streams)
	case "operation":
		return networkOperation(ctx, arguments[1:], streams)
	default:
		fmt.Fprintf(streams.stderr, "unknown network command %q\n%s\n", arguments[0], networkUsage)
		return 2
	}
}

// parseNetworkFlags parses the shared options, turning an unknown flag into a
// usage error that names the token-file option — the mistake most likely to be
// made is passing a token directly.
func parseNetworkFlags(
	name string, arguments []string, streams networkIO, extra func(*flag.FlagSet),
) (networkOptions, []string, bool) {
	flags := flag.NewFlagSet("network "+name, flag.ContinueOnError)
	message := new(bytes.Buffer)
	flags.SetOutput(message)

	var options networkOptions
	options.bind(flags)
	if extra != nil {
		extra(flags)
	}
	if err := flags.Parse(arguments); err != nil {
		fmt.Fprint(streams.stderr, message.String())
		fmt.Fprintln(streams.stderr, networkUsage)
		return networkOptions{}, nil, false
	}
	return options, flags.Args(), true
}

// networkRead performs one request and prints its result.
func networkRead(
	ctx context.Context, arguments []string, streams networkIO,
	name, path string, extra func(*flag.FlagSet),
) int {
	options, _, ok := parseNetworkFlags(name, arguments, streams, extra)
	if !ok {
		return 2
	}
	return networkRequest(ctx, options, streams, http.MethodGet, path, nil)
}

func networkStatistics(ctx context.Context, arguments []string, streams networkIO) int {
	var interfaceID, period string
	options, _, ok := parseNetworkFlags("statistics", arguments, streams, func(flags *flag.FlagSet) {
		flags.StringVar(&interfaceID, "interface", "", "interface identifier")
		flags.StringVar(&period, "period", "", "15m, 1h or 24h")
	})
	if !ok {
		return 2
	}
	if interfaceID == "" || period == "" {
		fmt.Fprintln(streams.stderr, "statistics requires --interface and --period")
		return 2
	}
	query := url.Values{"interface": {interfaceID}, "period": {period}}
	return networkRequest(ctx, options, streams, http.MethodGet,
		"/api/v1/network/statistics?"+query.Encode(), nil)
}

func networkInterface(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) < 2 {
		fmt.Fprintln(streams.stderr, "interface requires enable or disable and an interface identifier")
		return 2
	}
	action := arguments[0]
	if action != "enable" && action != "disable" {
		fmt.Fprintf(streams.stderr, "unknown interface action %q\n", action)
		return 2
	}
	var connectionID string
	options, remaining, ok := parseNetworkFlags("interface", arguments[2:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&connectionID, "connection", "", "existing saved connection to enable")
	})
	if !ok {
		return 2
	}
	if len(remaining) != 0 {
		fmt.Fprintln(streams.stderr, "unexpected interface arguments")
		return 2
	}
	return networkMutate(ctx, options, streams,
		"/api/v1/network/interfaces/"+url.PathEscape(arguments[1])+"/preview",
		map[string]string{"action": action, "connection_id": connectionID})
}

func networkOperation(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) < 2 {
		fmt.Fprintln(streams.stderr, "operation requires an action and an operation identifier")
		return 2
	}
	action, id := arguments[0], arguments[1]
	base := "/api/v1/network/operations/" + url.PathEscape(id)

	var revision string
	options, _, ok := parseNetworkFlags("operation", arguments[2:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&revision, "revision", "", "the revision the preview returned")
	})
	if !ok {
		return 2
	}

	switch action {
	case "status":
		return networkRequest(ctx, options, streams, http.MethodGet, base, nil)
	case "acknowledge":
		if revision == "" {
			fmt.Fprintln(streams.stderr, "acknowledge requires --revision, exactly as the preview returned it")
			return 2
		}
		return networkRequest(ctx, options, streams, http.MethodPost,
			base+"/acknowledge", map[string]string{"revision": revision})
	case "confirm", "rollback":
		return networkRequest(ctx, options, streams, http.MethodPost,
			base+"/"+action, map[string]any{})
	default:
		fmt.Fprintf(streams.stderr, "unknown operation action %q\n", action)
		return 2
	}
}

// networkRequest builds the client, performs the call and reports the result.
func networkRequest(
	ctx context.Context, options networkOptions, streams networkIO,
	method, path string, body any,
) int {
	if options.server == "" || options.tokenFile == "" {
		fmt.Fprintln(streams.stderr, "--server and --token-file are both required")
		return 2
	}

	client, err := apiclient.New(apiclient.Options{
		BaseURL: options.server, TokenFile: options.tokenFile,
		CAFile: options.caFile, Timeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}

	document, err := client.Do(ctx, method, path, body)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}

	return printNetworkDocument(options, streams, document)
}

func printNetworkDocument(options networkOptions, streams networkIO, document []byte) int {
	var outcome struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(document, &outcome)
	code := 0
	if outcome.State == "failed" || outcome.State == "rollback_failed" {
		code = 1
	}
	if options.asJSON {
		// The appliance's own document, unaltered, so a script reads exactly
		// what the API said rather than this command's paraphrase of it.
		fmt.Fprintln(streams.stdout, string(bytes.TrimSpace(document)))
		return code
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, bytes.TrimSpace(document), "", "  "); err != nil {
		fmt.Fprintln(streams.stderr, "the appliance returned a document this command cannot read")
		return 1
	}
	fmt.Fprintln(streams.stdout, pretty.String())
	return code
}

func networkTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
