package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"nilo.local/nilo/internal/apiclient"
)

const activityUsage = `usage: nilo activity <command>

  list    [--category audit|event] [--component F] [--target T] [--actor ID]
          [--severity S] [--outcome O] [--operation ID] [--action A]
          [--from RFC3339] [--to RFC3339] [--cursor C] [--limit N]
  export  --format jsonl|csv [same filters] [--out FILE]
  retention [--retention audit=90,event=14]
            [--quota audit=268435456,event=134217728]

Shared options: --server URL --token-file PATH [--ca-file PATH] [--json]`

var activityFilterNames = []string{"category", "component", "target", "actor", "severity", "outcome", "operation", "action", "from", "to", "cursor", "limit"}

// collectFilters parses the shared options and the history filters into a
// query string. Every filter is passed through as given; the appliance is
// the one that validates them.
func collectFilters(name string, arguments []string, streams networkIO, extra func(*flag.FlagSet)) (networkOptions, url.Values, bool) {
	fields := make(map[string]*string, len(activityFilterNames))
	options, remaining, ok := parseNetworkFlags(name, arguments, streams, func(flags *flag.FlagSet) {
		for _, filter := range activityFilterNames {
			fields[filter] = flags.String(filter, "", "filter by "+filter)
		}
		if extra != nil {
			extra(flags)
		}
	})
	if !ok {
		return options, nil, false
	}
	if len(remaining) != 0 {
		fmt.Fprintf(streams.stderr, "%s takes flags only\n", name)
		return options, nil, false
	}
	values := url.Values{}
	for _, filter := range activityFilterNames {
		if value := *fields[filter]; value != "" {
			values.Set(filter, value)
		}
	}
	if limit := values.Get("limit"); limit != "" {
		if _, err := strconv.Atoi(limit); err != nil {
			fmt.Fprintln(streams.stderr, "--limit must be a number")
			return options, nil, false
		}
	}
	return options, values, true
}

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

// runActivityCommand implements `nilo activity`.
func runActivityCommand(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) == 0 {
		fmt.Fprintln(streams.stderr, activityUsage)
		return 2
	}
	switch arguments[0] {
	case "list":
		options, values, ok := collectFilters("activity list", arguments[1:], streams, nil)
		if !ok {
			return 2
		}
		path := "/api/v1/activity"
		if encoded := values.Encode(); encoded != "" {
			path += "?" + encoded
		}
		return networkRequest(ctx, options, streams, http.MethodGet, path, nil)
	case "export":
		var format, out string
		options, values, ok := collectFilters("activity export", arguments[1:], streams, func(flags *flag.FlagSet) {
			flags.StringVar(&format, "format", "jsonl", "jsonl or csv")
			flags.StringVar(&out, "out", "", "write to this file instead of standard output")
		})
		if !ok {
			return 2
		}
		if format != "jsonl" && format != "csv" {
			fmt.Fprintln(streams.stderr, "--format must be jsonl or csv")
			return 2
		}
		values.Set("format", format)
		return activityDownload(ctx, options, streams, "/api/v1/activity/export?"+values.Encode(), out)
	case "retention":
		var retention, quota string
		options, remaining, ok := parseNetworkFlags("activity retention", arguments[1:], streams, func(flags *flag.FlagSet) {
			flags.StringVar(&retention, "retention", "", "retention days per category, for example audit=90,event=14")
			flags.StringVar(&quota, "quota", "", "quota bytes per category")
		})
		if !ok || len(remaining) != 0 {
			return 2
		}
		if retention == "" && quota == "" {
			return networkRequest(ctx, options, streams, http.MethodGet, "/api/v1/activity/retention", nil)
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
		return networkRequest(ctx, options, streams, http.MethodPut, "/api/v1/activity/retention", map[string]any{"retention_days": days, "quota_bytes": bytes})
	default:
		fmt.Fprintf(streams.stderr, "unknown activity command %q\n%s\n", arguments[0], activityUsage)
		return 2
	}
}

// activityDownload streams an export to a file or standard output, untouched:
// what the appliance wrote is what the operator keeps.
func activityDownload(ctx context.Context, options networkOptions, streams networkIO, path, out string) int {
	if options.server == "" || options.tokenFile == "" {
		fmt.Fprintln(streams.stderr, "--server and --token-file are both required")
		return 2
	}
	client, err := apiclient.New(apiclient.Options{
		BaseURL: options.server, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 5 * time.Minute,
	})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	body, err := client.Stream(ctx, path)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	defer body.Close()
	var destination io.Writer = streams.stdout
	if out != "" {
		file, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		defer file.Close()
		destination = file
	}
	if _, err := io.Copy(destination, body); err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return 0
}
