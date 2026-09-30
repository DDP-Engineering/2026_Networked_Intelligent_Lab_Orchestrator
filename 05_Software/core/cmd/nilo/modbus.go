package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

const modbusUsage = `usage: nilo modbus <command> --server URL --token-file PATH [--ca-file PATH] [--json]
  endpoints | endpoint-save --endpoint ID --file endpoint.json [--expected-revision N]
  profiles | profile-save --profile ID --file profile.json [--expected-revision N]
  status --endpoint ID
  connect --endpoint ID --endpoint-revision N --profile ID --profile-revision N [--file groups.json --key KEY]
  read --endpoint ID --session ID --file request.json
  write|preview --endpoint ID --session ID --file request.json [--key KEY]
  poll --endpoint ID --session ID --file groups.json [--paused]
  display --endpoint ID --session ID [--paused]
  events --endpoint ID --session ID [--after N]
  command --command ID | commands --endpoint ID --session ID [--offset N]
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
	var endpoint, profile, session, key, file, commandID string
	var endpointRevision, profileRevision, expectedRevision, after int64
	var offset int
	var paused bool
	var timeout int
	options, rest, ok := parseNetworkFlags("modbus "+action, args[1:], streams, func(flags *flag.FlagSet) {
		flags.StringVar(&endpoint, "endpoint", "", "configured Modbus endpoint ID")
		flags.StringVar(&profile, "profile", "", "configured Modbus profile ID")
		flags.StringVar(&session, "session", "", "live session ID")
		flags.StringVar(&key, "key", "", "idempotency key")
		flags.StringVar(&commandID, "command", "", "durable write command ID")
		flags.IntVar(&offset, "offset", 0, "durable command page offset")
		flags.StringVar(&file, "file", "", "typed JSON input file")
		flags.Int64Var(&endpointRevision, "endpoint-revision", 0, "pinned endpoint revision")
		flags.Int64Var(&profileRevision, "profile-revision", 0, "pinned profile revision")
		flags.Int64Var(&expectedRevision, "expected-revision", 0, "revision for definition save")
		flags.Int64Var(&after, "after", 0, "last live event sequence")
		flags.IntVar(&timeout, "timeout-seconds", 60, "finite admission deadline")
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
	case "endpoints":
		path = base + "/endpoints"
	case "profiles":
		path = base + "/profiles"
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
	case "status", "connect", "read", "write", "preview", "poll", "display", "events", "commands", "disconnect":
		if endpoint == "" {
			return invalid("--endpoint is required")
		}
		path = base + "/endpoints/" + url.PathEscape(endpoint)
		switch action {
		case "status":
			path += "/session"
		case "connect":
			if profile == "" || endpointRevision < 1 || profileRevision < 1 {
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
			body = map[string]any{"endpoint_revision": endpointRevision, "profile_id": profile, "profile_revision": profileRevision, "groups": groups, "idempotency_key": key, "timeout_seconds": timeout}
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
		default:
			if session == "" {
				return invalid("--session is required")
			}
			method, path = http.MethodPost, path+"/"+action
			switch action {
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
