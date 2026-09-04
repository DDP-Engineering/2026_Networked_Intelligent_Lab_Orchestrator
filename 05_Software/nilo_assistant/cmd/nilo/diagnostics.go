package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const diagnosticsUsage = `usage: nilo diagnostics <command>

  sessions                          list timed debug sessions, scopes and durations
  start --scope S [--target T] --minutes 5|15|60
  show ID
  stop ID
  settings                          read the retention and quota policy
  settings --retention audit=90,event=14,debug=1 --quota audit=268435456,event=134217728,debug=67108864

Shared options: --server URL --token-file PATH [--ca-file PATH] [--json]`

// parseCategoryValues turns "audit=90,event=14" into a JSON-ready map.
func parseCategoryValues(text string) (map[string]int64, error) {
	values := map[string]int64{}
	for _, field := range strings.Split(text, ",") {
		name, value, found := strings.Cut(strings.TrimSpace(field), "=")
		if !found {
			return nil, fmt.Errorf("%q is not category=value", field)
		}
		number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", value)
		}
		values[strings.TrimSpace(name)] = number
	}
	return values, nil
}

// runDiagnosticsCommand implements `nilo diagnostics`. A session is started
// and stopped through the same authenticated route the browser uses.
func runDiagnosticsCommand(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, diagnosticsUsage)
		return 2
	}
	base := "/api/v1/diagnostics"
	switch arguments[0] {
	case "sessions":
		return networkRead(ctx, arguments[1:], streams, "diagnostics sessions", base+"/sessions", nil)
	case "start":
		var scope, target string
		var minutes int
		options, remaining, ok := parseNetworkFlags("diagnostics start", arguments[1:], streams, func(flags *flag.FlagSet) {
			flags.StringVar(&scope, "scope", "", "function scope, for example network.wireguard")
			flags.StringVar(&target, "target", "", "optional target within the scope")
			flags.IntVar(&minutes, "minutes", 0, "5, 15 or 60")
		})
		if !ok {
			return 2
		}
		if len(remaining) != 0 || scope == "" || minutes == 0 {
			fmt.Fprintln(streams.stderr, "start requires --scope and --minutes")
			return 2
		}
		return networkRequest(ctx, options, streams, http.MethodPost, base+"/sessions", map[string]any{"scope": scope, "target": target, "minutes": minutes})
	case "show", "stop":
		if len(arguments) < 2 {
			fmt.Fprintf(streams.stderr, "%s requires a session identifier\n", arguments[0])
			return 2
		}
		options, remaining, ok := parseNetworkFlags("diagnostics "+arguments[0], arguments[2:], streams, nil)
		if !ok || len(remaining) != 0 {
			return 2
		}
		path := base + "/sessions/" + url.PathEscape(arguments[1])
		if arguments[0] == "show" {
			return networkRequest(ctx, options, streams, http.MethodGet, path, nil)
		}
		return networkRequest(ctx, options, streams, http.MethodPost, path+"/stop", map[string]any{})
	case "settings":
		var retention, quota string
		options, remaining, ok := parseNetworkFlags("diagnostics settings", arguments[1:], streams, func(flags *flag.FlagSet) {
			flags.StringVar(&retention, "retention", "", "retention days per category, for example audit=90,event=14,debug=1")
			flags.StringVar(&quota, "quota", "", "quota bytes per category")
		})
		if !ok || len(remaining) != 0 {
			return 2
		}
		if retention == "" && quota == "" {
			return networkRequest(ctx, options, streams, http.MethodGet, base+"/settings", nil)
		}
		if retention == "" || quota == "" {
			fmt.Fprintln(streams.stderr, "a policy change needs both --retention and --quota")
			return 2
		}
		days, err := parseCategoryValues(retention)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 2
		}
		bytes, err := parseCategoryValues(quota)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 2
		}
		return networkRequest(ctx, options, streams, http.MethodPut, base+"/settings", map[string]any{"retention_days": days, "quota_bytes": bytes})
	default:
		fmt.Fprintf(streams.stderr, "unknown diagnostics command %q\n%s\n", arguments[0], diagnosticsUsage)
		return 2
	}
}
