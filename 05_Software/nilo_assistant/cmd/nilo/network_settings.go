package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// networkSettings implements `nilo network interface settings ID ...`: one
// previewed, acknowledged configuration change through the same route the
// browser uses.
func networkSettings(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) < 1 {
		fmt.Fprintln(streams.stderr, "settings requires an interface identifier")
		return 2
	}
	var method, address, gateway, dns string
	var dnsAutomatic, protect, neverDefault bool
	options, remaining, ok := parseNetworkFlags("interface settings", arguments[1:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&method, "method", "", "static or automatic")
		flags.StringVar(&address, "address", "", "static IPv4 address with prefix, for example 192.168.0.1/24")
		flags.StringVar(&gateway, "gateway", "", "IPv4 gateway inside the subnet")
		flags.StringVar(&dns, "dns", "", "comma-separated DNS servers")
		flags.BoolVar(&dnsAutomatic, "dns-automatic", false, "take DNS servers from the network")
		flags.BoolVar(&protect, "protect", false, "mark the adapter as a management path")
		flags.BoolVar(&neverDefault, "never-default", false, "never take the default route over this adapter")
	})
	if !ok {
		return 2
	}
	if len(remaining) != 0 || (method != "static" && method != "automatic") {
		fmt.Fprintln(streams.stderr, "settings requires --method static|automatic")
		return 2
	}
	body := map[string]any{
		"method": method, "dns_automatic": dnsAutomatic, "management_protected": protect, "never_default": neverDefault,
		"dns": []string{},
	}
	if address != "" {
		body["address"] = address
	}
	if gateway != "" {
		body["gateway"] = gateway
	}
	if dns != "" {
		servers := make([]string, 0)
		for _, server := range strings.Split(dns, ",") {
			if trimmed := strings.TrimSpace(server); trimmed != "" {
				servers = append(servers, trimmed)
			}
		}
		body["dns"] = servers
	}
	return networkMutate(ctx, options, streams,
		"/api/v1/network/interfaces/"+url.PathEscape(arguments[0])+"/settings/preview", body)
}

// networkWiFi implements `nilo network wifi scan|join ID ...`. The password
// comes from a file and never from a flag, for the same reason the token does.
func networkWiFi(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) < 2 {
		fmt.Fprintln(streams.stderr, "wifi requires scan or join and an interface identifier")
		return 2
	}
	action, interfaceID := arguments[0], arguments[1]
	switch action {
	case "scan":
		return networkRead(ctx, arguments[2:], streams, "wifi scan",
			"/api/v1/network/interfaces/"+url.PathEscape(interfaceID)+"/wifi", nil)
	case "join":
		var ssid, security, passwordFile string
		var startup bool
		options, remaining, ok := parseNetworkFlags("wifi join", arguments[2:], streams, func(flags *flag.FlagSet) {
			flags.StringVar(&ssid, "ssid", "", "network name")
			flags.StringVar(&security, "security", "", "open, wpa2-personal or wpa3-personal")
			flags.StringVar(&passwordFile, "password-file", "", "file holding the network password")
			flags.BoolVar(&startup, "startup", false, "connect to this network when the appliance starts")
		})
		if !ok {
			return 2
		}
		if len(remaining) != 0 || ssid == "" || security == "" {
			fmt.Fprintln(streams.stderr, "join requires --ssid and --security")
			return 2
		}
		body := map[string]any{"ssid": ssid, "security": security, "connect_at_startup": startup}
		if passwordFile != "" {
			input, err := os.Open(passwordFile)
			if err != nil {
				fmt.Fprintln(streams.stderr, "cannot open the password file")
				return 1
			}
			data, err := io.ReadAll(io.LimitReader(input, 4097))
			_ = input.Close()
			if err != nil || len(data) > 4096 {
				fmt.Fprintln(streams.stderr, "the password file must be readable and at most 4 KiB")
				return 1
			}
			body["password"] = strings.TrimRight(string(data), "\r\n")
		}
		return networkMutate(ctx, options, streams,
			"/api/v1/network/interfaces/"+url.PathEscape(interfaceID)+"/wifi/preview", body)
	default:
		fmt.Fprintf(streams.stderr, "unknown wifi action %q\n", action)
		return 2
	}
}

var _ = http.MethodGet
