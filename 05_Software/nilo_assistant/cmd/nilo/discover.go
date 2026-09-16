package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/discovery"
)

type commandIO struct {
	stdout io.Writer
	stderr io.Writer
}

type discoveryRunner func(context.Context, time.Duration) ([]discovery.Appliance, []string, error)

func runDiscoverCommand(ctx context.Context, arguments []string, streams commandIO, discover discoveryRunner) int {
	flags := flag.NewFlagSet("discover", flag.ContinueOnError)
	flags.SetOutput(streams.stderr)
	timeout := flags.Duration("timeout", 5*time.Second, "discovery window (1s-60s)")
	jsonOutput := flags.Bool("json", false, "emit schema-1 JSON")
	labelName := flags.String("label", "", "emit one selected SVG label")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *timeout < time.Second || *timeout > 60*time.Second || *jsonOutput && *labelName != "" {
		fmt.Fprintln(streams.stderr, "usage: nilo discover [--timeout 5s] [--json] [--label NAME]")
		return 2
	}
	started := time.Now()
	appliances, diagnostics, err := discover(ctx, *timeout)
	if err != nil {
		fmt.Fprintln(streams.stderr, "discovery_unavailable")
		return 1
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(streams.stderr, diagnostic)
	}
	if *labelName != "" {
		selected, err := discovery.SelectUnique(appliances, *labelName)
		if err != nil || selected.URL == "" || len(selected.HardwareAddresses) == 0 {
			fmt.Fprintln(streams.stderr, discoveryErrorCode(err))
			return 1
		}
		artifact, err := appliance.RenderLabelSVG(appliance.Label{SerialName: selected.Serial, HardwareAddresses: selected.HardwareAddresses})
		if err != nil {
			fmt.Fprintln(streams.stderr, "label_unavailable")
			return 1
		}
		if _, err := streams.stdout.Write(artifact); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return 0
	}
	if *jsonOutput {
		output := struct {
			Schema      int                   `json:"schema"`
			DurationMS  int64                 `json:"duration_ms"`
			Appliances  []discovery.Appliance `json:"appliances"`
			Diagnostics []string              `json:"diagnostics"`
		}{Schema: 1, DurationMS: time.Since(started).Milliseconds(), Appliances: appliances, Diagnostics: diagnostics}
		if output.Appliances == nil {
			output.Appliances = []discovery.Appliance{}
		}
		if output.Diagnostics == nil {
			output.Diagnostics = []string{}
		}
		if err := json.NewEncoder(streams.stdout).Encode(output); err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		return 0
	}
	writer := tabwriter.NewWriter(streams.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "NAME\tSERIAL\tSTATE\tVERSION\tURL\tADDRESSES")
	for _, item := range appliances {
		_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", item.Name, item.Serial, item.State, item.Version, item.URL, strings.Join(item.Addresses, ","))
	}
	if err := writer.Flush(); err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return 0
}

func runOpenCommand(ctx context.Context, arguments []string, streams commandIO, discover discoveryRunner, launch func(string) error) int {
	if len(arguments) != 1 || strings.TrimSpace(arguments[0]) != arguments[0] || arguments[0] == "" {
		fmt.Fprintln(streams.stderr, "usage: nilo open <serial-or-friendly-name>")
		return 2
	}
	resolved, err := resolveDiscoveredName(ctx, arguments[0], discover)
	if err != nil {
		fmt.Fprintln(streams.stderr, discoveryErrorCode(err))
		return 1
	}
	if launch == nil || launch(resolved) != nil {
		fmt.Fprintln(streams.stderr, "browser_unavailable")
		return 1
	}
	return 0
}

func resolveDiscoveredServer(ctx context.Context, value string, discover discoveryRunner) (string, error) {
	parsed, err := url.Parse(value)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return value, nil
	}
	return resolveDiscoveredName(ctx, value, discover)
}

func resolveDiscoveredName(ctx context.Context, value string, discover discoveryRunner) (string, error) {
	if discover == nil {
		return "", errors.New("discovery unavailable")
	}
	appliances, _, err := discover(ctx, 5*time.Second)
	if err != nil {
		return "", err
	}
	selected, err := discovery.SelectUnique(appliances, value)
	if err != nil {
		return "", err
	}
	if selected.URL == "" {
		return "", errors.New("discovered appliance has no safe URL")
	}
	return selected.URL, nil
}

func resolveServerForRemoteCommand(ctx context.Context, value string) (string, error) {
	return resolveDiscoveredServer(ctx, value, discovery.Discover)
}

func discoveryErrorCode(err error) string {
	switch {
	case errors.Is(err, discovery.ErrNotFound):
		return "appliance_not_found"
	case errors.Is(err, discovery.ErrAmbiguous):
		return "appliance_name_ambiguous"
	default:
		return "discovery_unavailable"
	}
}
