package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"nilo.local/nilo/internal/apiclient"
)

func networkVPN(ctx context.Context, args []string, streams networkIO) int {
	if len(args) == 0 {
		fmt.Fprintln(streams.stderr, networkUsage)
		return 2
	}
	if args[0] == "check" {
		return networkRead(ctx, args[1:], streams, "vpn check", "/api/v1/network/wireguard", nil)
	}
	if args[0] == "profiles" && len(args) > 1 && args[1] == "list" {
		return networkRead(ctx, args[2:], streams, "vpn profiles list", "/api/v1/network/wireguard/profiles", nil)
	}
	path := "/api/v1/network/wireguard/preview"
	body := map[string]any{}
	rest := args[1:]
	var file, name, enabled, newName string
	switch args[0] {
	case "install":
		path = "/api/v1/network/wireguard/install/preview"
	case "disable":
		body["action"] = "deactivate"
		body["target"] = "active"
	case "enable", "autostart":
		if len(args) < 2 {
			fmt.Fprintln(streams.stderr, "VPN command requires a profile identifier")
			return 2
		}
		body["action"] = "activate"
		if args[0] == "autostart" {
			body["action"] = "autostart"
		}
		body["target"] = args[1]
		rest = args[2:]
	case "profiles":
		if len(args) < 2 {
			fmt.Fprintln(streams.stderr, networkUsage)
			return 2
		}
		switch args[1] {
		case "import":
			body["action"] = "import"
			rest = args[2:]
		case "remove":
			if len(args) < 3 {
				fmt.Fprintln(streams.stderr, "remove requires a profile identifier")
				return 2
			}
			body["action"] = "remove"
			body["target"] = args[2]
			rest = args[3:]
		case "rename":
			if len(args) < 3 {
				fmt.Fprintln(streams.stderr, "rename requires a profile identifier and --name")
				return 2
			}
			body["action"] = "rename"
			body["target"] = args[2]
			rest = args[3:]
		default:
			fmt.Fprintln(streams.stderr, "unknown VPN profile command")
			return 2
		}
	default:
		fmt.Fprintln(streams.stderr, "unknown VPN command")
		return 2
	}
	options, left, ok := parseNetworkFlags("vpn", rest, streams, func(flags *flag.FlagSet) {
		if body["action"] == "import" {
			flags.StringVar(&file, "file", "", "WireGuard configuration file")
			flags.StringVar(&name, "name", "", "profile name")
		}
		if body["action"] == "autostart" {
			flags.StringVar(&enabled, "enabled", "", "true or false")
		}
		if body["action"] == "rename" {
			flags.StringVar(&newName, "name", "", "new profile name")
		}
	})
	if !ok {
		return 2
	}
	if len(left) != 0 {
		fmt.Fprintln(streams.stderr, "unexpected VPN arguments")
		return 2
	}
	if body["action"] == "rename" {
		if newName == "" {
			fmt.Fprintln(streams.stderr, "rename requires --name")
			return 2
		}
		body["name"] = newName
	}
	if body["action"] == "autostart" {
		if enabled != "true" && enabled != "false" {
			fmt.Fprintln(streams.stderr, "autostart requires --enabled true|false")
			return 2
		}
		body["enabled"] = enabled == "true"
	}
	if body["action"] == "import" {
		if file == "" || name == "" {
			fmt.Fprintln(streams.stderr, "import requires --file and --name")
			return 2
		}
		input, err := os.Open(file)
		if err != nil {
			fmt.Fprintln(streams.stderr, "cannot open profile file")
			return 1
		}
		data, err := io.ReadAll(io.LimitReader(input, (48<<10)+1))
		_ = input.Close()
		if err != nil || len(data) > 48<<10 {
			fmt.Fprintln(streams.stderr, "profile file must be readable and at most 48 KiB")
			return 1
		}
		body["profile_text"] = string(data)
		body["target"] = name
	}
	return networkMutate(ctx, options, streams, path, body)
}

func networkMutate(ctx context.Context, options networkOptions, streams networkIO, path string, body any) int {
	if options.server == "" || options.tokenFile == "" {
		fmt.Fprintln(streams.stderr, "--server and --token-file are both required")
		return 2
	}
	client, err := apiclient.New(apiclient.Options{BaseURL: options.server, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 30 * time.Second})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	document, err := client.Do(ctx, http.MethodPost, path, body)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	var preview struct {
		ID       string `json:"id"`
		Revision string `json:"revision"`
		Action   string `json:"action"`
		Target   string `json:"target"`
	}
	if err := json.Unmarshal(document, &preview); err != nil || preview.ID == "" || preview.Revision == "" {
		fmt.Fprintln(streams.stderr, "invalid operation preview")
		return 1
	}
	fmt.Fprintf(streams.stderr, "Proposed change: %s on %s (operation %s).\n", preview.Action, preview.Target, preview.ID)
	if parameters, ok := body.(map[string]any); ok && parameters["action"] == "autostart" {
		fmt.Fprintf(streams.stderr, "Connect at startup: %t\n", parameters["enabled"])
	}
	if !options.acknowledge {
		if !streams.interactive || streams.stdin == nil {
			fmt.Fprintln(streams.stderr, "noninteractive changes require --acknowledge; no change applied")
			return 2
		}
		fmt.Fprint(streams.stderr, "Apply this change? Type yes: ")
		answer, err := bufio.NewReader(streams.stdin).ReadString('\n')
		if err != nil || strings.TrimSpace(answer) != "yes" {
			fmt.Fprintln(streams.stderr, "change cancelled")
			return 2
		}
	}
	document, err = client.Do(ctx, http.MethodPost, "/api/v1/network/operations/"+url.PathEscape(preview.ID)+"/acknowledge", map[string]string{"revision": preview.Revision})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	return printNetworkDocument(options, streams, document)
}
