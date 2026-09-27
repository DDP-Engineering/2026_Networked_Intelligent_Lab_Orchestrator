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
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/apiclient"
)

const serialUsage = `usage: nilo serial <command> --server URL --token-file PATH [--ca-file PATH] [--json]
  status --device ID
  connect --device ID --revision N [--baud-rate 115200 --data-bits 8 --parity none --stop-bits 1 --flow-control none --rts=true --dtr=true --key KEY]
  read --device ID --connection ID [--after N --wait-ms 10000 --follow]
  send --device ID --connection ID --data-file PATH|- [--encoding ascii|utf8|hex --ending none|cr|lf|crlf --key KEY]
  disconnect --device ID --connection ID
  signals --device ID --connection ID [--rts=true|false --dtr=true|false]
  logging --device ID --connection ID --action start|stop
  logs --device ID [--offset N]
  log-read --id LOG [--after N]
  export --id LOG --out NEW_FILE [--format jsonl|text|rx|tx]
  delete --id LOG --confirm LOG
  retention --device ID [--revision N --file LIMITS_JSON]

Connect queues a session approval; inspect its transaction with nilo bench get.
Send reads the payload only from a file or standard input. Reuse a key only for
the identical request. Unknown delivery is never retried automatically.
Read emits bounded JSON batches; Ctrl+C stops following without disconnecting.`

func runSerialCommand(ctx context.Context, args []string, streams networkIO) int {
	if len(args) == 0 {
		fmt.Fprintln(streams.stderr, serialUsage)
		return 2
	}
	action := args[0]
	var device, connection, id, key, dataFile, encoding, ending, parity, flow, loggingAction, out, format, limitsFile, confirm string
	var revision, after int64
	var baud, bits, waitMS, offset, timeout int
	var stopBits float64
	var rts, dtr, follow bool
	var lineFlags *flag.FlagSet
	options, rest, ok := parseNetworkFlags("serial "+action, args[1:], streams, func(f *flag.FlagSet) {
		lineFlags = f
		f.StringVar(&device, "device", "", "managed serial device ID")
		f.StringVar(&connection, "connection", "", "exact approved connection ID")
		f.StringVar(&id, "id", "", "saved log ID")
		f.StringVar(&key, "key", "", "idempotency key for one immutable request")
		f.Int64Var(&revision, "revision", 0, "observed device or retention revision")
		f.Int64Var(&after, "after", 0, "last received event sequence")
		f.IntVar(&baud, "baud-rate", 115200, "baud rate")
		f.IntVar(&bits, "data-bits", 8, "data bits")
		f.StringVar(&parity, "parity", "none", "none, odd, even, mark or space")
		f.Float64Var(&stopBits, "stop-bits", 1, "1, 1.5 or 2")
		f.StringVar(&flow, "flow-control", "none", "none, rtscts or xonxoff")
		f.BoolVar(&rts, "rts", true, "initial or manual RTS state")
		f.BoolVar(&dtr, "dtr", true, "initial or manual DTR state")
		f.IntVar(&waitMS, "wait-ms", 10000, "read wait, 0..10000 milliseconds")
		f.IntVar(&offset, "offset", 0, "log list offset")
		f.IntVar(&timeout, "timeout-seconds", 60, "connect transaction deadline")
		f.BoolVar(&follow, "follow", false, "keep reading bounded cursor batches")
		f.StringVar(&dataFile, "data-file", "-", "payload file, or - for stdin")
		f.StringVar(&encoding, "encoding", "utf8", "ascii, utf8 or hex")
		f.StringVar(&ending, "ending", "none", "none, cr, lf or crlf")
		f.StringVar(&loggingAction, "action", "", "start or stop logging")
		f.StringVar(&out, "out", "", "new export file")
		f.StringVar(&format, "format", "jsonl", "jsonl, text, rx or tx")
		f.StringVar(&limitsFile, "file", "", "JSON retention limits")
		f.StringVar(&confirm, "confirm", "", "exact log ID to delete")
	})
	if !ok || len(rest) > 0 {
		return 2
	}
	invalid := func(message string) int { fmt.Fprintln(streams.stderr, message); return 2 }
	if after < 0 || offset < 0 || waitMS < 0 || waitMS > 10000 {
		return invalid("cursor/offset must be nonnegative; --wait-ms must be 0..10000")
	}
	switch action {
	case "export", "delete", "log-read":
		if id == "" {
			return invalid("--id is required")
		}
	default:
		if device == "" {
			return invalid("--device is required")
		}
	}
	base := "/api/v1/serial-devices/" + url.PathEscape(device)
	method, path := http.MethodGet, ""
	var body any
	switch action {
	case "status":
		path = base + "/workspace"
	case "connect":
		if revision < 1 {
			return invalid("--revision must be the observed device revision")
		}
		if key == "" {
			key = uuid.NewString()
			fmt.Fprintln(streams.stderr, "Idempotency key:", key)
		}
		method, path = http.MethodPost, base+"/connect"
		body = map[string]any{"revision": revision, "idempotency_key": key, "timeout_seconds": timeout, "settings": map[string]any{"baud_rate": baud, "data_bits": bits, "parity": parity, "stop_bits": stopBits, "flow_control": flow, "rts": rts, "dtr": dtr}}
	case "read":
		if connection == "" {
			return invalid("--connection is required")
		}
		return readSerialCLI(ctx, options, streams, base, connection, after, waitMS, follow)
	case "send", "signals", "disconnect", "logging":
		if connection == "" {
			return invalid("--connection is required")
		}
		values := map[string]any{"connection_id": connection}
		switch action {
		case "send":
			data, err := serialPayload(dataFile, streams.stdin)
			if err != nil {
				return invalid(err.Error())
			}
			if key == "" {
				key = uuid.NewString()
				fmt.Fprintln(streams.stderr, "Idempotency key:", key)
			}
			values["data"], values["encoding"], values["ending"], values["idempotency_key"] = data, encoding, ending, key
		case "signals":
			lineFlags.Visit(func(f *flag.Flag) {
				if f.Name == "rts" {
					values["rts"] = rts
				}
				if f.Name == "dtr" {
					values["dtr"] = dtr
				}
			})
			if len(values) == 1 {
				return invalid("at least one of --rts or --dtr is required")
			}
		case "logging":
			if loggingAction != "start" && loggingAction != "stop" {
				return invalid("--action must be start or stop")
			}
			values["action"] = loggingAction
		}
		method, path, body = http.MethodPost, base+"/"+action, values
	case "logs":
		path = base + "/logs?offset=" + strconv.Itoa(offset)
	case "log-read":
		path = "/api/v1/serial-logs/" + url.PathEscape(id) + "/events?after=" + strconv.FormatInt(after, 10)
	case "delete":
		if confirm != id {
			return invalid("--confirm must equal --id")
		}
		method, path = http.MethodDelete, "/api/v1/serial-logs/"+url.PathEscape(id)
	case "export":
		if out == "" {
			return invalid("--out is required")
		}
		if format != "jsonl" && format != "text" && format != "rx" && format != "tx" {
			return invalid("--format must be jsonl, text, rx or tx")
		}
		return downloadBenchArtifact(ctx, options, streams, "/api/v1/serial-logs/"+url.PathEscape(id)+"/content?format="+format, out, 8<<30)
	case "retention":
		path = base + "/retention"
		if limitsFile != "" {
			if revision < 1 {
				return invalid("--revision is required when saving limits")
			}
			limits, err := readBenchJSONObject(limitsFile)
			if err != nil {
				return invalid(err.Error())
			}
			method, body = http.MethodPut, map[string]any{"revision": revision, "limits": limits}
		}
	default:
		return invalid(serialUsage)
	}
	return networkRequest(ctx, options, streams, method, path, body)
}

func serialPayload(path string, stdin io.Reader) (string, error) {
	input := stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("cannot open payload file")
		}
		defer file.Close()
		input = file
	}
	if input == nil {
		return "", fmt.Errorf("payload stdin is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(input, 16385))
	if err != nil || len(data) > 16384 || !utf8.Valid(data) {
		return "", fmt.Errorf("payload must be UTF-8 text or hex, at most 16384 source bytes; encoded send limit is 4096 bytes")
	}
	return string(data), nil
}

func readSerialCLI(ctx context.Context, options networkOptions, streams networkIO, base, connection string, after int64, waitMS int, follow bool) int {
	if options.server == "" || options.tokenFile == "" {
		fmt.Fprintln(streams.stderr, "--server and --token-file are both required")
		return 2
	}
	server, err := resolveServerForRemoteCommand(ctx, options.server)
	if err != nil {
		fmt.Fprintln(streams.stderr, discoveryErrorCode(err))
		return 1
	}
	client, err := apiclient.New(apiclient.Options{BaseURL: server, TokenFile: options.tokenFile, CAFile: options.caFile, Timeout: 15 * time.Second})
	if err != nil {
		fmt.Fprintln(streams.stderr, err)
		return 1
	}
	for {
		query := url.Values{"connection_id": {connection}, "after": {strconv.FormatInt(after, 10)}, "wait_ms": {strconv.Itoa(waitMS)}}
		document, err := client.Do(ctx, http.MethodGet, base+"/read?"+query.Encode(), nil)
		if ctx.Err() != nil {
			return 0
		}
		if err != nil {
			fmt.Fprintln(streams.stderr, err)
			return 1
		}
		var batch struct {
			Next int64 `json:"next_cursor"`
		}
		if json.Unmarshal(document, &batch) != nil || batch.Next < after {
			fmt.Fprintln(streams.stderr, "invalid serial read cursor")
			return 1
		}
		if code := printNetworkDocument(options, streams, document); code != 0 {
			return code
		}
		if !follow {
			return 0
		}
		after = batch.Next
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(100 * time.Millisecond):
		}
	}
}
