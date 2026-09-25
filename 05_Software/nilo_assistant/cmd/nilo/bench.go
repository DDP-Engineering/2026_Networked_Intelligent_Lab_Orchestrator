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

const benchUsage = `usage: nilo plugin list|install|enable|disable|restart|uninstall|discover|devices [--plugin camera]
       nilo plugin configure --plugin camera --revision REV --file settings.json
       nilo plugin device configure|enable|disable|retry|rebind|forget --plugin camera --device ID --revision REV
       nilo bench jobs|get|cancel|download|submit [--id TRANSACTION] [--out FILE]
       nilo bench capture --device ID [--channels D0,D1] [--samplerate 1000000] [--samples 1024]
       nilo bench decode --capture TRANSACTION [--channel D0] [--baudrate 115200]
       nilo camera snapshot --device ID [--width 640 --height 480 --zoom 1]
       nilo camera controls get|set|reset --device ID [--controls-file JSON]

Shared: --server URL --token-file PATH [--ca-file PATH] [--json]
Submit: [--key IDEMPOTENCY_KEY] [--timeout-seconds 60]
Submission returns immediately; get retrieves delayed results. Human approval is available in the GUI.`

func runBenchCommand(ctx context.Context, group string, args []string, streams networkIO) int {
	if len(args) == 0 {
		fmt.Fprintln(streams.stderr, benchUsage)
		return 2
	}
	action := args[0]
	if group == "camera" && action == "controls" && len(args) > 1 {
		action = "controls-" + args[1]
		args = append([]string{action}, args[2:]...)
	}
	if group == "plugin" && action == "device" && len(args) > 1 {
		action = "device-" + args[1]
		args = append([]string{action}, args[2:]...)
	}
	var plugin, mode, id, out, device, channels, capture, channel, key, operation, parametersFile, controlsFile, pixelFormat, configFile, provisionalID, confirmation string
	var samplerate, samples, baudrate, timeout, width, height, rotation, waitSeconds, maximumDimension, revision, provisionalRevision int
	var zoom float64
	var grayscale, invert, factory bool
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
		flags.StringVar(&operation, "operation", "", "operation for generic bench submit")
		flags.StringVar(&parametersFile, "parameters-file", "", "JSON object for generic bench submit")
		flags.StringVar(&controlsFile, "controls-file", "", "JSON control values for camera set")
		flags.IntVar(&width, "width", 640, "camera capture width")
		flags.IntVar(&height, "height", 480, "camera capture height")
		flags.IntVar(&maximumDimension, "maximum-dimension", 0, "optional maximum retained image dimension")
		flags.StringVar(&pixelFormat, "format", "MJPG", "camera format MJPG or YUYV")
		flags.Float64Var(&zoom, "zoom", 1, "camera digital zoom factor")
		flags.IntVar(&rotation, "rotation", 0, "camera rotation in degrees")
		flags.BoolVar(&grayscale, "grayscale", false, "retain a grayscale image")
		flags.BoolVar(&invert, "invert", false, "invert image colours")
		flags.BoolVar(&factory, "factory", false, "reset hardware controls to factory defaults")
		flags.IntVar(&waitSeconds, "wait-seconds", 0, "bounded wait for a camera snapshot")
		flags.IntVar(&revision, "revision", 0, "last observed configuration or device revision")
		flags.IntVar(&provisionalRevision, "provisional-revision", 0, "last observed provisional device revision")
		flags.StringVar(&provisionalID, "provisional", "", "newly discovered provisional camera ID")
		flags.StringVar(&confirmation, "confirm", "", "exact camera record ID confirmation")
		flags.StringVar(&configFile, "file", "", "JSON configuration file")
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
		case "devices":
			path = "/api/v1/plugins/" + url.PathEscape(plugin) + "/devices"
		case "configure":
			if configFile == "" || revision < 0 {
				fmt.Fprintln(streams.stderr, "--file and --revision are required")
				return 2
			}
			settings, err := readBenchJSONObject(configFile)
			if err != nil {
				fmt.Fprintln(streams.stderr, err)
				return 2
			}
			path, method, body = "/api/v1/plugins/"+url.PathEscape(plugin)+"/configuration", http.MethodPut, map[string]any{"revision": revision, "settings": settings}
		case "device-configure", "device-enable", "device-disable", "device-retry", "device-rebind", "device-forget":
			if device == "" || revision < 1 {
				fmt.Fprintln(streams.stderr, "--device and --revision are required")
				return 2
			}
			base := "/api/v1/plugins/" + url.PathEscape(plugin) + "/devices/" + url.PathEscape(device)
			switch action {
			case "device-configure":
				if configFile == "" {
					fmt.Fprintln(streams.stderr, "--file is required")
					return 2
				}
				values, err := readBenchJSONObject(configFile)
				if err != nil {
					fmt.Fprintln(streams.stderr, err)
					return 2
				}
				values["revision"] = revision
				path, method, body = base, http.MethodPut, values
			case "device-rebind":
				if confirmation != device || provisionalID == "" || provisionalRevision < 1 {
					fmt.Fprintln(streams.stderr, "--confirm must equal --device; --provisional and --provisional-revision are required")
					return 2
				}
				path, method, body = base+"/rebind", http.MethodPost, map[string]any{"revision": revision, "provisional_id": provisionalID, "provisional_revision": provisionalRevision, "confirm_device_id": confirmation}
			case "device-forget":
				if confirmation != device {
					fmt.Fprintln(streams.stderr, "--confirm must equal --device")
					return 2
				}
				path, method, body = base, http.MethodDelete, map[string]any{"revision": revision, "confirm_device_id": confirmation}
			default:
				path, method, body = base+"/"+strings.TrimPrefix(action, "device-"), http.MethodPost, map[string]any{"revision": revision}
			}
		case "install", "upgrade", "enable", "disable", "restart", "uninstall", "discover":
			path = "/api/v1/plugins/" + url.PathEscape(plugin) + "/" + action
			method = http.MethodPost
			body = map[string]string{}
			if action == "enable" && mode != "" {
				body = map[string]string{"mode": mode}
			}
		}
	} else if group == "camera" {
		if device == "" {
			fmt.Fprintln(streams.stderr, "--device is required")
			return 2
		}
		parameters := map[string]any{}
		switch action {
		case "snapshot":
			parameters = map[string]any{"width": width, "height": height, "format": pixelFormat,
				"processing": map[string]any{"zoom": zoom, "rotation": rotation, "grayscale": grayscale, "invert": invert}}
			if maximumDimension != 0 {
				parameters["maximum_dimension"] = maximumDimension
			}
		case "controls-get":
			action = "get_controls"
		case "controls-set":
			if controlsFile == "" {
				fmt.Fprintln(streams.stderr, "--controls-file is required")
				return 2
			}
			values, err := readBenchJSONObject(controlsFile)
			if err != nil {
				fmt.Fprintln(streams.stderr, err)
				return 2
			}
			parameters["controls"] = values
			action = "set_controls"
		case "controls-reset":
			parameters["factory"] = factory
			action = "reset_controls"
		default:
			fmt.Fprintln(streams.stderr, benchUsage)
			return 2
		}
		if key == "" {
			key = uuid.NewString()
			fmt.Fprintln(streams.stderr, "Idempotency key:", key)
		}
		input := bench.Submission{PluginID: "camera", Operation: action, DeviceID: device, IdempotencyKey: key, TimeoutSeconds: timeout}
		input.Parameters, _ = json.Marshal(parameters)
		body = input
		method, path = http.MethodPost, "/api/v1/bench/jobs"
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
		case "capture", "decode", "submit":
			if action == "submit" && (operation == "" || parametersFile == "") {
				fmt.Fprintln(streams.stderr, "--operation and --parameters-file are required")
				return 2
			}
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
			if action == "submit" {
				values, err := readBenchJSONObject(parametersFile)
				if err != nil {
					fmt.Fprintln(streams.stderr, err)
					return 2
				}
				input.Operation = operation
				input.Parameters, _ = json.Marshal(values)
				parameters = nil
			}
			if parameters != nil {
				input.Parameters, _ = json.Marshal(parameters)
			}
			body = input
			method = http.MethodPost
			path = "/api/v1/bench/jobs"
		}
	}
	if path == "" {
		fmt.Fprintln(streams.stderr, benchUsage)
		return 2
	}
	if group == "camera" && action == "snapshot" && (out != "" || waitSeconds > 0) {
		if waitSeconds == 0 {
			waitSeconds = 20
		}
		if waitSeconds < 1 || waitSeconds > 30 {
			fmt.Fprintln(streams.stderr, "--wait-seconds must be 1..30")
			return 2
		}
		return cameraSnapshotWait(ctx, options, streams, body, out, waitSeconds)
	}
	return networkRequest(ctx, options, streams, method, path, body)
}

func cameraSnapshotWait(ctx context.Context, options networkOptions, streams networkIO, body any, out string, waitSeconds int) int {
	if options.server == "" || options.tokenFile == "" {
		fmt.Fprintln(streams.stderr, "--server and --token-file are both required")
		return 2
	}
	server, err := resolveServerForRemoteCommand(ctx, options.server)
	if err != nil {
		fmt.Fprintln(streams.stderr, discoveryErrorCode(err))
		return 1
	}
	client, err := apiclient.New(apiclient.Options{BaseURL: server, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 30 * time.Second})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	document, err := client.Do(ctx, http.MethodPost, "/api/v1/bench/jobs", body)
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	var job struct {
		ID    string `json:"transaction_id"`
		State string `json:"state"`
	}
	if json.Unmarshal(document, &job) != nil || job.ID == "" {
		fmt.Fprintln(streams.stderr, "invalid snapshot transaction")
		return 1
	}
	deadline := time.NewTimer(time.Duration(waitSeconds) * time.Second)
	defer deadline.Stop()
	for job.State != "completed" && !benchJobTerminal(job.State) {
		select {
		case <-ctx.Done():
			fmt.Fprintln(streams.stderr, ctx.Err())
			return 1
		case <-deadline.C:
			fmt.Fprintf(streams.stdout, "snapshot pending: %s (%s)\n", job.ID, job.State)
			return 0
		case <-time.After(250 * time.Millisecond):
			document, err = client.Do(ctx, http.MethodGet, "/api/v1/bench/jobs/"+url.PathEscape(job.ID), nil)
			if err != nil {
				fmt.Fprintln(streams.stderr, err)
				return 1
			}
			if json.Unmarshal(document, &job) != nil {
				fmt.Fprintln(streams.stderr, "invalid snapshot transaction")
				return 1
			}
		}
	}
	if job.State != "completed" {
		return printNetworkDocument(options, streams, document)
	}
	if out != "" {
		return downloadBenchArtifact(ctx, options, streams, "/api/v1/bench/jobs/"+url.PathEscape(job.ID)+"/artifact", out)
	}
	return printNetworkDocument(options, streams, document)
}

func benchJobTerminal(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled" || state == "incomplete"
}

func readBenchJSONObject(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil || len(content) > 16<<10 {
		return nil, bench.ErrInvalid
	}
	var result map[string]any
	if json.Unmarshal(content, &result) != nil || result == nil {
		return nil, bench.ErrInvalid
	}
	return result, nil
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
