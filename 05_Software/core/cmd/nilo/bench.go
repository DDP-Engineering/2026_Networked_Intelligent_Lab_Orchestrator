package main

import (
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

	"github.com/google/uuid"
	"nilo.local/nilo/internal/apiclient"
	"nilo.local/nilo/internal/bench"
)

const benchUsage = `usage: nilo plugin list|install|enable|disable|restart|uninstall|discover [--plugin sigrok] [--mode ask|auto]
       nilo bench jobs|get|cancel|download [--id TRANSACTION] [--out FILE]
       nilo bench capture --device ID [--channels D0,D1] [--samplerate 1000000] [--samples 1024]
       nilo bench decode --capture TRANSACTION [--channel D0] [--baudrate 115200]

Shared: --server URL --token-file PATH [--ca-file PATH] [--json]
Submit: [--key IDEMPOTENCY_KEY] [--timeout-seconds 60]
Submission returns immediately; get retrieves delayed results. Human approval is available in the GUI.`

func runBenchCommand(ctx context.Context, group string, args []string, streams networkIO) int {
	if len(args) == 0 {
		fmt.Fprintln(streams.stderr, benchUsage)
		return 2
	}
	action := args[0]
	var plugin, mode, id, out, device, channels, capture, channel, key string
	var samplerate, samples, baudrate, timeout int
	options, remaining, ok := parseNetworkFlags(group+" "+action, args[1:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&plugin, "plugin", "sigrok", "curated plugin ID")
		flags.StringVar(&mode, "mode", "", "ask or auto policy")
		flags.StringVar(&id, "id", "", "transaction ID")
		flags.StringVar(&out, "out", "", "new artifact output file")
		flags.StringVar(&device, "device", "", "discovered digital device ID")
		flags.StringVar(&channels, "channels", "D0", "comma-separated digital channels")
		flags.IntVar(&samplerate, "samplerate", 1000000, "sample rate in Hz")
		flags.IntVar(&samples, "samples", 1024, "finite sample count")
		flags.StringVar(&capture, "capture", "", "saved capture transaction ID")
		flags.StringVar(&channel, "channel", "D0", "UART receive channel")
		flags.IntVar(&baudrate, "baudrate", 115200, "UART baud rate")
		flags.StringVar(&key, "key", "", "idempotency key; reuse for an identical retry")
		flags.IntVar(&timeout, "timeout-seconds", 60, "overall queue and execution deadline")
	})
	if !ok || len(remaining) > 0 {
		return 2
	}
	method, path := http.MethodGet, ""
	var body any
	if group == "plugin" {
		switch action {
		case "list":
			path = "/api/v1/plugins"
		case "install", "upgrade", "enable", "disable", "restart", "uninstall", "discover":
			path = "/api/v1/plugins/" + url.PathEscape(plugin) + "/" + action
			method = http.MethodPost
			body = map[string]string{}
			if action == "enable" && mode != "" {
				body = map[string]string{"mode": mode}
			}
		}
	} else {
		switch action {
		case "jobs":
			path = "/api/v1/bench/jobs"
		case "get", "cancel", "download":
			if id == "" {
				fmt.Fprintln(streams.stderr, "--id is required")
				return 2
			}
			path = "/api/v1/bench/jobs/" + url.PathEscape(id)
			if action == "cancel" {
				path += "/cancel"
				method = http.MethodPost
				body = map[string]string{}
			}
			if action == "download" {
				if out == "" {
					fmt.Fprintln(streams.stderr, "--out is required")
					return 2
				}
				return downloadBenchArtifact(ctx, options, streams, path+"/artifact", out)
			}
		case "capture", "decode":
			if key == "" {
				key = uuid.NewString()
				fmt.Fprintln(streams.stderr, "Idempotency key:", key)
			}
			input := bench.Submission{PluginID: plugin, Operation: "capture", DeviceID: device, IdempotencyKey: key, TimeoutSeconds: timeout}
			parameters := map[string]any{"channels": strings.Split(channels, ","), "samplerate": samplerate, "samples": samples}
			if action == "decode" {
				input.Operation = "decode_uart"
				input.DeviceID = ""
				parameters = map[string]any{"capture_id": capture, "channel": channel, "baudrate": baudrate}
			}
			input.Parameters, _ = json.Marshal(parameters)
			body = input
			method = http.MethodPost
			path = "/api/v1/bench/jobs"
		}
	}
	if path == "" {
		fmt.Fprintln(streams.stderr, benchUsage)
		return 2
	}
	return networkRequest(ctx, options, streams, method, path, body)
}

func downloadBenchArtifact(ctx context.Context, options networkOptions, streams networkIO, path, out string) int {
	resolvedServer, err := resolveServerForRemoteCommand(ctx, options.server)
	if err != nil {
		fmt.Fprintln(streams.stderr, discoveryErrorCode(err))
		return 1
	}
	client, err := apiclient.New(apiclient.Options{BaseURL: resolvedServer, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 30 * time.Second})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	content, err := client.Stream(ctx, path)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	defer content.Close()
	file, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	written, copyErr := io.Copy(file, io.LimitReader(content, (8<<20)+1))
	closeErr := file.Close()
	if written > 8<<20 {
		copyErr = fmt.Errorf("artifact exceeds download size limit")
	}
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(out)
		fmt.Fprintln(streams.stderr, "artifact download failed", copyErr, closeErr)
		return 1
	}
	fmt.Fprintln(streams.stdout, out)
	return 0
}
