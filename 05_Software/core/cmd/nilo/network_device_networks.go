package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
)

// networkDeviceNetworks implements `nilo network device-networks ...`. Reads
// go straight to the appliance; every change is one previewed, acknowledged
// operation through the same route the browser uses. A create or update takes
// its definition from a JSON file, because a device network has too many
// fields to spell out as flags without inventing a second schema.
func networkDeviceNetworks(ctx context.Context, arguments []string, streams networkIO) int {
	if len(arguments) < 1 {
		fmt.Fprintln(streams.stderr, "device-networks requires list, show, create, update, enable, disable, delete, check or instructions")
		return 2
	}
	base := "/api/v1/network/device-networks"
	action := arguments[0]
	switch action {
	case "list":
		return networkRead(ctx, arguments[1:], streams, "device-networks list", base, nil)
	case "create", "update":
		var file string
		rest := arguments[1:]
		target := ""
		if action == "update" {
			if len(rest) < 1 {
				fmt.Fprintln(streams.stderr, "update requires a device network identifier")
				return 2
			}
			target, rest = rest[0], rest[1:]
		}
		options, remaining, ok := parseNetworkFlags("device-networks "+action, rest, streams, func(flags *flag.FlagSet) {
			flags.StringVar(&file, "file", "", "JSON file holding the device network definition")
		})
		if !ok {
			return 2
		}
		if len(remaining) != 0 || file == "" {
			fmt.Fprintf(streams.stderr, "%s requires --file PATH\n", action)
			return 2
		}
		definition, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 2
		}
		var deviceNetwork json.RawMessage
		if err := json.Unmarshal(definition, &deviceNetwork); err != nil {
			fmt.Fprintf(streams.stderr, "%s: not a JSON document: %v\n", file, err)
			return 2
		}
		body := map[string]any{"action": action, "device_network": deviceNetwork}
		if target != "" {
			body["target"] = target
		}
		return networkMutate(ctx, options, streams, base+"/preview", body)
	case "enable", "disable", "delete":
		if len(arguments) < 2 {
			fmt.Fprintf(streams.stderr, "%s requires a device network identifier\n", action)
			return 2
		}
		options, remaining, ok := parseNetworkFlags("device-networks "+action, arguments[2:], streams, nil)
		if !ok || len(remaining) != 0 {
			return 2
		}
		return networkMutate(ctx, options, streams, base+"/preview", map[string]any{"action": action, "target": arguments[1]})
	case "show", "instructions":
		if len(arguments) < 2 {
			fmt.Fprintf(streams.stderr, "%s requires a device network identifier\n", action)
			return 2
		}
		path := base + "/" + url.PathEscape(arguments[1])
		if action == "instructions" {
			path += "/access-instructions"
		}
		return networkRead(ctx, arguments[2:], streams, "device-networks "+action, path, nil)
	case "check":
		if len(arguments) < 2 {
			fmt.Fprintln(streams.stderr, "check requires a device network identifier")
			return 2
		}
		var tcpPort int
		options, remaining, ok := parseNetworkFlags("device-networks check", arguments[2:], streams, func(flags *flag.FlagSet) {
			flags.IntVar(&tcpPort, "tcp-port", 0, "TCP port to probe on the configured target")
		})
		if !ok || len(remaining) != 0 {
			return 2
		}
		return networkRequest(ctx, options, streams, http.MethodPost,
			base+"/"+url.PathEscape(arguments[1])+"/checks", map[string]any{"tcp_port": tcpPort})
	default:
		fmt.Fprintf(streams.stderr, "unknown device-networks command %q\n", action)
		return 2
	}
}
