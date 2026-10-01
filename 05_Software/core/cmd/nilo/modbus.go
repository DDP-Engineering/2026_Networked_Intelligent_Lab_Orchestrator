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
	"strconv"

	"github.com/google/uuid"
)

const modbusUsage = `usage: nilo modbus <command> --server URL --token-file PATH [--ca-file PATH] [--json]
  endpoints | endpoint-save --endpoint ID --file endpoint.json [--expected-revision N]
  settings | settings-save --file settings.json --expected-revision N
  profiles | profile-save --profile ID --file profile.json [--expected-revision N]
  maps | map-get --map ID | map-save --map ID --file map.json [--expected-revision N]
  map-decode|map-encode --map ID --revision N --file point.json
  map-duplicate --map ID --target NEW_ID | map-export --map ID [--format json|csv]
  map-import-preview|map-import --file PATH [--format json|csv --mode create|replace|merge --target ID --expected-revision N --accept-conflicts]
  workspaces | workspace-get --workspace ID | workspace-save --workspace ID --file workspace.json [--expected-revision N]
  workspace-duplicate --workspace ID --target NEW_ID | workspace-export --workspace ID
  workspace-import-preview|workspace-import --file workspace.json [--mode create|replace --target ID --expected-revision N]
  status --endpoint ID
  connect --endpoint ID --endpoint-revision N --profile ID --profile-revision N [--workspace ID --workspace-revision N --file groups.json --key KEY]
  test --endpoint ID --endpoint-revision N --profile ID --profile-revision N [--key KEY --timeout-seconds N]
  read --endpoint ID --session ID --file request.json
  scan-start --endpoint ID --session ID --file plan.json | scan-status|scan-results|scan-cancel --endpoint ID --session ID
  scan-export --endpoint ID --session ID --format csv|jsonl --out FILE
  diagnostic --endpoint ID --session ID --kind device_information|comm_event_counter|report_server_id --unit N
  write|preview --endpoint ID --session ID --file request.json [--key KEY]
  poll --endpoint ID --session ID --file groups.json [--paused]
  poll-group --endpoint ID --session ID --group ID [--paused]
  display --endpoint ID --session ID [--paused]
  events --endpoint ID --session ID [--after N]
  command --command ID | commands --endpoint ID --session ID [--offset N]
  recording-start --endpoint ID --session ID --mode values|traffic|both --sampling every|interval|change [--interval-ms N --name TEXT]
  recording-stop|recording-status --endpoint ID --session ID
  recordings [--name TEXT --tag TEXT --endpoint ID --started-from RFC3339 --started-through RFC3339 --before UUID]
  recording-get|recording-find|recording-delete --recording UUID
  recording-export --recording UUID --format jsonl|csv|text|bundle --out FILE
  disconnect --endpoint ID --session ID

Connect returns a finite admission job; inspect it with nilo bench get. Ask
approval is human-only. A write's key is tied to one exact request; an unknown
delivery result never sends the request again.`

func runModbusCommand(ctx context.Context, args []string, streams networkIO) int {
	if len(args) == 0 {
		fmt.Fprintln(streams.stderr, modbusUsage)
		return 2
	}
	action := args[0]
	var endpoint, profile, session, key, file, commandID, mapID, workspaceID, targetID, format, mode, groupID string
	var recordingID, sampling, name, tag, startedFrom, startedThrough, out, textQuery, hexQuery, before, kind, outcome, pointID, utcFrom, utcThrough string
	var endpointRevision, profileRevision, expectedRevision, mapRevision, workspaceRevision, after, through int64
	var offset int
	var intervalMS int
	var findUnit, findFunction, findAddress int
	var paused, acceptConflicts bool
	var timeout int
	timeoutDefault := 60
	if action == "test" {
		timeoutDefault = 30
	}
	options, rest, ok := parseNetworkFlags("modbus "+action, args[1:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&endpoint, "endpoint", "", "configured Modbus endpoint ID")
		flags.StringVar(&profile, "profile", "", "configured Modbus profile ID")
		flags.StringVar(&mapID, "map", "", "named register map ID")
		flags.StringVar(&workspaceID, "workspace", "", "saved workspace ID")
		flags.Int64Var(&workspaceRevision, "workspace-revision", 0, "pinned workspace revision for connect")
		flags.StringVar(&targetID, "target", "", "new or import target ID")
		flags.StringVar(&format, "format", "json", "map document format: json or csv")
		flags.StringVar(&mode, "mode", "create", "map import mode")
		flags.BoolVar(&acceptConflicts, "accept-conflicts", false, "replace conflicting point IDs in merge")
		flags.StringVar(&session, "session", "", "live session ID")
		flags.StringVar(&groupID, "group", "", "admitted poll group ID")
		flags.StringVar(&recordingID, "recording", "", "saved Modbus recording ID")
		flags.StringVar(&sampling, "sampling", "every", "recording sample mode")
		flags.IntVar(&intervalMS, "interval-ms", 0, "sample interval in milliseconds")
		flags.StringVar(&name, "name", "", "recording name")
		flags.StringVar(&tag, "tag", "", "recording tag to find")
		flags.StringVar(&startedFrom, "started-from", "", "recording start lower bound (RFC3339)")
		flags.StringVar(&startedThrough, "started-through", "", "recording start upper bound (RFC3339)")
		flags.StringVar(&out, "out", "", "export destination path")
		flags.StringVar(&textQuery, "text", "", "literal Find text")
		flags.StringVar(&hexQuery, "hex", "", "even-length hex Find text")
		flags.StringVar(&kind, "kind", "", "recording event kind")
		flags.StringVar(&outcome, "outcome", "", "recording event outcome")
		flags.StringVar(&pointID, "point", "", "recording point ID")
		flags.StringVar(&utcFrom, "utc-from", "", "event UTC lower bound (RFC3339)")
		flags.StringVar(&utcThrough, "utc-through", "", "event UTC upper bound (RFC3339)")
		flags.IntVar(&findUnit, "unit", 0, "event unit filter")
		flags.IntVar(&findFunction, "function", 0, "event function filter")
		flags.IntVar(&findAddress, "address", -1, "event address filter")
		flags.StringVar(&before, "before", "", "recording metadata cursor")
		flags.StringVar(&key, "key", "", "idempotency key")
		flags.StringVar(&commandID, "command", "", "durable write command ID")
		flags.IntVar(&offset, "offset", 0, "durable command page offset")
		flags.StringVar(&file, "file", "", "typed JSON input file")
		flags.Int64Var(&endpointRevision, "endpoint-revision", 0, "pinned endpoint revision")
		flags.Int64Var(&profileRevision, "profile-revision", 0, "pinned profile revision")
		flags.Int64Var(&expectedRevision, "expected-revision", 0, "revision for definition save")
		flags.Int64Var(&mapRevision, "revision", 0, "pinned map or workspace revision to open")
		flags.Int64Var(&after, "after", 0, "last live event sequence")
		flags.Int64Var(&through, "through", 0, "last sequence to export")
		flags.IntVar(&timeout, "timeout-seconds", timeoutDefault, "finite admission deadline")
		flags.BoolVar(&paused, "paused", false, "pause polling or display")
	})
	if !ok || len(rest) > 0 {
		return 2
	}
	invalid := func(message string) int { fmt.Fprintln(streams.stderr, message); return 2 }
	base := "/api/v1/modbus"
	method, path := http.MethodGet, ""
	var body any
	switch action {
	case "recordings":
		path = base + "/recordings"
		query := url.Values{}
		for key, value := range map[string]string{"before": before, "name": name, "tag": tag, "endpoint_id": endpoint, "started_from": startedFrom, "started_through": startedThrough} {
			if value != "" {
				query.Set(key, value)
			}
		}
		if len(query) != 0 {
			path += "?" + query.Encode()
		}
	case "recording-get", "recording-find", "recording-export", "recording-delete":
		if recordingID == "" {
			return invalid("--recording is required")
		}
		path = base + "/recordings/" + url.PathEscape(recordingID)
		switch action {
		case "recording-find":
			if after < 0 {
				return invalid("--after must be nonnegative")
			}
			query := url.Values{"after": {strconv.FormatInt(after, 10)}}
			if textQuery != "" {
				query.Set("text", textQuery)
			}
			if hexQuery != "" {
				query.Set("hex", hexQuery)
			}
			for key, value := range map[string]string{"kind": kind, "outcome": outcome, "point_id": pointID, "utc_from": utcFrom, "utc_through": utcThrough} {
				if value != "" {
					query.Set(key, value)
				}
			}
			if findUnit != 0 {
				query.Set("unit", strconv.Itoa(findUnit))
			}
			if findFunction != 0 {
				query.Set("function", strconv.Itoa(findFunction))
			}
			if findAddress >= 0 {
				query.Set("address", strconv.Itoa(findAddress))
			}
			path += "/find?" + query.Encode()
		case "recording-export":
			if out == "" || after < 0 || through < 0 {
				return invalid("--out and nonnegative sequence bounds are required")
			}
			if format != "jsonl" && format != "csv" && format != "text" && format != "bundle" {
				return invalid("--format must be jsonl, csv, text, or bundle")
			}
			query := url.Values{"format": {format}, "after": {strconv.FormatInt(after, 10)}, "through": {strconv.FormatInt(through, 10)}}
			return downloadBenchArtifact(ctx, options, streams, path+"/export?"+query.Encode(), out, 16<<30)
		case "recording-delete":
			method = http.MethodDelete
		}
	case "endpoints":
		path = base + "/endpoints"
	case "settings":
		path = base + "/settings"
	case "settings-save":
		if file == "" || expectedRevision < 1 {
			return invalid("--file and a positive --expected-revision are required")
		}
		settings, err := readBenchJSONObject(file)
		if err != nil {
			return invalid(err.Error())
		}
		method, path = http.MethodPut, base+"/settings"
		body = map[string]any{"expected_revision": expectedRevision, "settings": settings}
	case "profiles":
		path = base + "/profiles"
	case "maps", "workspaces":
		path = base + "/" + action
	case "map-decode", "map-encode":
		if mapID == "" || mapRevision < 1 || file == "" {
			return invalid("--map, positive --revision and --file are required")
		}
		point, err := readBenchJSONObject(file)
		if err != nil {
			return invalid(err.Error())
		}
		operation := "decode"
		if action == "map-encode" {
			operation = "encode"
		}
		method = http.MethodPost
		path = base + "/maps/" + url.PathEscape(mapID) + "/revisions/" + strconv.FormatInt(mapRevision, 10) + "/" + operation
		body = point
	case "map-get", "map-export", "map-duplicate", "workspace-get", "workspace-export", "workspace-duplicate":
		kind, id := "maps", mapID
		if action == "workspace-get" || action == "workspace-export" || action == "workspace-duplicate" {
			kind, id = "workspaces", workspaceID
		}
		if id == "" {
			return invalid("--map or --workspace is required")
		}
		path = base + "/" + kind + "/" + url.PathEscape(id)
		if action == "map-get" && mapRevision > 0 {
			path += "/revisions/" + strconv.FormatInt(mapRevision, 10)
		}
		if action == "workspace-get" && mapRevision > 0 {
			path += "/revisions/" + strconv.FormatInt(mapRevision, 10)
		}
		if action == "map-export" {
			path += "/export?format=" + url.QueryEscape(format)
		}
		if action == "workspace-export" {
			path += "/export"
		}
		if action == "map-duplicate" || action == "workspace-duplicate" {
			if targetID == "" {
				return invalid("--target is required")
			}
			method = http.MethodPost
			path += "/duplicate"
			body = map[string]any{"new_id": targetID}
		}
	case "map-save", "workspace-save":
		kind, id, field := "maps", mapID, "map"
		if action == "workspace-save" {
			kind, id, field = "workspaces", workspaceID, "workspace"
		}
		if id == "" || file == "" || expectedRevision < 0 {
			return invalid("ID, --file and nonnegative --expected-revision are required")
		}
		definition, err := readModbusDefinition(file)
		if err != nil {
			return invalid(err.Error())
		}
		method, path = http.MethodPut, base+"/"+kind+"/"+url.PathEscape(id)
		body = map[string]any{"expected_revision": expectedRevision, field: definition}
	case "map-import-preview", "map-import":
		if file == "" || expectedRevision < 0 {
			return invalid("--file and nonnegative --expected-revision are required")
		}
		raw, err := readModbusDocument(file)
		if err != nil {
			return invalid(err.Error())
		}
		method = http.MethodPost
		path = base + "/maps/import"
		if action == "map-import-preview" {
			path = base + "/maps/import-preview"
		}
		body = map[string]any{"format": format, "mode": mode, "target_id": targetID, "expected_revision": expectedRevision, "accept_conflicts": acceptConflicts, "document": string(raw)}
	case "workspace-import-preview", "workspace-import":
		if file == "" || expectedRevision < 0 {
			return invalid("--file and nonnegative --expected-revision are required")
		}
		raw, err := readModbusDocument(file)
		if err != nil {
			return invalid(err.Error())
		}
		method = http.MethodPost
		path = base + "/workspaces/import"
		if action == "workspace-import-preview" {
			path = base + "/workspaces/import-preview"
		}
		body = map[string]any{"mode": mode, "target_id": targetID, "expected_revision": expectedRevision, "document": string(raw)}
	case "command":
		if commandID == "" {
			return invalid("--command is required")
		}
		path = base + "/commands/" + url.PathEscape(commandID)
	case "endpoint-save", "profile-save":
		if file == "" || expectedRevision < 0 {
			return invalid("--file and a nonnegative expected revision are required")
		}
		definition, err := readBenchJSONObject(file)
		if err != nil {
			return invalid(err.Error())
		}
		method = http.MethodPut
		if action == "endpoint-save" {
			if endpoint == "" {
				return invalid("--endpoint is required")
			}
			path, body = base+"/endpoints/"+url.PathEscape(endpoint), map[string]any{"expected_revision": expectedRevision, "endpoint": definition}
		} else {
			if profile == "" {
				return invalid("--profile is required")
			}
			path, body = base+"/profiles/"+url.PathEscape(profile), map[string]any{"expected_revision": expectedRevision, "profile": definition}
		}
	case "status", "connect", "test", "read", "write", "preview", "poll", "poll-group", "display", "events", "commands", "disconnect", "recording-start", "recording-stop", "recording-status", "scan-start", "scan-status", "scan-results", "scan-cancel", "scan-export", "diagnostic":
		if endpoint == "" {
			return invalid("--endpoint is required")
		}
		path = base + "/endpoints/" + url.PathEscape(endpoint)
		switch action {
		case "status":
			path += "/session"
		case "recording-status":
			path += "/session"
		case "recording-start":
			if session == "" {
				return invalid("--session is required")
			}
			if mode == "create" {
				mode = "both"
			}
			method, path = http.MethodPost, path+"/recording"
			body = map[string]any{"session_id": session, "mode": mode, "sampling": sampling, "interval_ms": intervalMS, "name": name}
		case "recording-stop":
			if session == "" {
				return invalid("--session is required")
			}
			method, path = http.MethodDelete, path+"/recording"
			body = map[string]any{"session_id": session}
		case "connect":
			if profile == "" || endpointRevision < 1 || profileRevision < 1 || (workspaceID == "") != (workspaceRevision == 0) {
				return invalid("--profile and both pinned revisions are required")
			}
			if key == "" {
				key = uuid.NewString()
				fmt.Fprintln(streams.stderr, "Idempotency key:", key)
			}
			method, path = http.MethodPost, path+"/connect"
			groups := any([]any{})
			if file != "" {
				plan, err := readBenchJSONObject(file)
				if err != nil {
					return invalid(err.Error())
				}
				groups = plan["groups"]
				if groups == nil {
					return invalid("--file must contain a groups array")
				}
			}
			body = map[string]any{"endpoint_revision": endpointRevision, "profile_id": profile, "profile_revision": profileRevision, "workspace_id": workspaceID, "workspace_revision": workspaceRevision, "groups": groups, "idempotency_key": key, "timeout_seconds": timeout}
		case "test":
			if profile == "" || endpointRevision < 1 || profileRevision < 1 || file != "" || timeout < 1 || timeout > 60 {
				return invalid("--profile, both pinned revisions and a 1-60 second timeout are required; --file is not allowed")
			}
			if key == "" {
				key = uuid.NewString()
				fmt.Fprintln(streams.stderr, "Idempotency key:", key)
			}
			method, path = http.MethodPost, path+"/test"
			body = map[string]any{"endpoint_revision": endpointRevision, "profile_id": profile, "profile_revision": profileRevision, "idempotency_key": key, "timeout_seconds": timeout}
		case "events":
			if session == "" || after < 0 {
				return invalid("--session and a nonnegative --after are required")
			}
			path += "/events?session_id=" + url.QueryEscape(session) + "&after=" + strconv.FormatInt(after, 10)
		case "commands":
			if session == "" || offset < 0 {
				return invalid("--session and a nonnegative --offset are required")
			}
			path += "/commands?session_id=" + url.QueryEscape(session) + "&offset=" + strconv.Itoa(offset)
		case "scan-status", "scan-results", "scan-export":
			if session == "" || offset < 0 || offset > 512 {
				return invalid("--session and an offset from 0 to 512 are required")
			}
			path += "/scans"
			if action == "scan-results" {
				path += "/results"
			}
			query := url.Values{"session_id": {session}}
			if action == "scan-results" {
				query.Set("offset", strconv.Itoa(offset))
			}
			if action == "scan-export" {
				if out == "" || format != "csv" && format != "jsonl" {
					return invalid("--out and --format csv|jsonl are required")
				}
				query.Set("format", format)
				return downloadBenchArtifact(ctx, options, streams, path+"/export?"+query.Encode(), out, 1<<20)
			}
			path += "?" + query.Encode()
		default:
			if session == "" {
				return invalid("--session is required")
			}
			method, path = http.MethodPost, path+"/"+action
			switch action {
			case "scan-start":
				if file == "" {
					return invalid("--file is required")
				}
				plan, err := readBenchJSONObject(file)
				if err != nil {
					return invalid(err.Error())
				}
				plan["session_id"] = session
				method, path, body = http.MethodPost, base+"/endpoints/"+url.PathEscape(endpoint)+"/scans", plan
			case "scan-cancel":
				method, path, body = http.MethodDelete, base+"/endpoints/"+url.PathEscape(endpoint)+"/scans", map[string]any{"session_id": session}
			case "diagnostic":
				if kind != "device_information" && kind != "comm_event_counter" && kind != "report_server_id" || findUnit < 1 || findUnit > 247 {
					return invalid("--kind and --unit 1..247 are required")
				}
				body = map[string]any{"session_id": session, "kind": kind, "unit": findUnit}
			case "read", "write", "preview":
				if file == "" {
					return invalid("--file is required")
				}
				request, err := readBenchJSONObject(file)
				if err != nil {
					return invalid(err.Error())
				}
				values := map[string]any{"session_id": session, "request": request}
				if action == "write" || action == "preview" {
					if key == "" {
						key = uuid.NewString()
						fmt.Fprintln(streams.stderr, "Idempotency key:", key)
					}
					values["idempotency_key"] = key
				}
				body = values
			case "poll":
				if file == "" {
					return invalid("--file is required")
				}
				groups, err := readBenchJSONObject(file)
				if err != nil {
					return invalid(err.Error())
				}
				plan, ok := groups["groups"].([]any)
				if !ok {
					return invalid("--file must contain a groups array")
				}
				body = map[string]any{"session_id": session, "groups": plan, "paused": paused}
			case "poll-group":
				if groupID == "" {
					return invalid("--group is required")
				}
				body = map[string]any{"session_id": session, "group_id": groupID, "paused": paused}
			case "display":
				body = map[string]any{"session_id": session, "paused": paused}
			case "disconnect":
				body = map[string]any{"session_id": session}
			}
		}
	default:
		return invalid(modbusUsage)
	}
	return networkRequest(ctx, options, streams, method, path, body)
}

func readModbusDefinition(path string) (json.RawMessage, error) {
	raw, err := readModbusDocument(path)
	if err != nil || len(raw) > 1<<20 || !json.Valid(raw) || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, fmt.Errorf("invalid or oversized Modbus definition")
	}
	return json.RawMessage(raw), nil
}

func readModbusDocument(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("Modbus document exceeds 1 MiB or cannot be read")
	}
	return raw, nil
}
