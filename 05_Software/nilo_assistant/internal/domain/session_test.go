package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSessionTransitionsAllowOnlyApprovedEdges(t *testing.T) {
	allowed := [][2]SessionState{
		{PendingApproval, Queued},
		{Queued, Running},
		{Running, Completed},
		{Running, Failed},
		{Running, Cancelled},
		{Running, Incomplete},
	}

	for _, edge := range allowed {
		t.Run(string(edge[0])+" to "+string(edge[1]), func(t *testing.T) {
			if err := ValidateTransition(edge[0], edge[1]); err != nil {
				t.Fatalf("ValidateTransition(%q, %q) error = %v", edge[0], edge[1], err)
			}
		})
	}
}

func TestSessionTransitionsRejectUndefinedEdges(t *testing.T) {
	for _, edge := range [][2]SessionState{
		{PendingApproval, PendingApproval},
		{Queued, Queued},
		{Running, Running},
		{PendingApproval, Running},
		{Queued, Completed},
		{Running, PendingApproval},
		{Completed, Running},
		{Failed, Queued},
		{Cancelled, Running},
		{Incomplete, Running},
	} {
		t.Run(string(edge[0])+" to "+string(edge[1]), func(t *testing.T) {
			err := ValidateTransition(edge[0], edge[1])
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("ValidateTransition(%q, %q) error = %v, want ErrInvalidTransition", edge[0], edge[1], err)
			}
		})
	}
}

func TestRuntimeIdentityConstructorsGenerateVersionSevenUUIDsAndParsersRejectInvalidUUIDs(t *testing.T) {
	tests := []struct {
		name  string
		new   func() (string, error)
		parse func(string) error
	}{
		{
			name: "request",
			new: func() (string, error) {
				id, err := NewRequestID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseRequestID(value)
				return err
			},
		},
		{
			name: "session",
			new: func() (string, error) {
				id, err := NewSessionID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseSessionID(value)
				return err
			},
		},
		{
			name: "actor",
			new: func() (string, error) {
				id, err := NewActorID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseActorID(value)
				return err
			},
		},
		{
			name: "device",
			new: func() (string, error) {
				id, err := NewDeviceID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseDeviceID(value)
				return err
			},
		},
		{
			name: "integration",
			new: func() (string, error) {
				id, err := NewIntegrationID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseIntegrationID(value)
				return err
			},
		},
		{
			name: "artifact",
			new: func() (string, error) {
				id, err := NewArtifactID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseArtifactID(value)
				return err
			},
		},
		{
			name: "snapshot",
			new: func() (string, error) {
				id, err := NewSnapshotID()
				return id.String(), err
			},
			parse: func(value string) error {
				_, err := ParseSnapshotID(value)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := test.new()
			if err != nil {
				t.Fatalf("new %s ID error = %v", test.name, err)
			}

			if !isUUIDv7(value) {
				t.Fatalf("new %s ID %q is not a UUID v7", test.name, value)
			}
			if err := test.parse(value); err != nil {
				t.Fatalf("Parse%sID(%q) error = %v", test.name, value, err)
			}
			if err := test.parse("not-a-uuid"); err == nil {
				t.Fatalf("Parse%sID accepted an invalid UUID", test.name)
			}
		})
	}
}

func isUUIDv7(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[14] != '7' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for _, position := range []int{0, 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 15, 16, 17, 19, 20, 21, 22, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35} {
		isHex := '0' <= value[position] && value[position] <= '9' || 'a' <= value[position] && value[position] <= 'f' || 'A' <= value[position] && value[position] <= 'F'
		if !isHex {
			return false
		}
	}
	return true
}

func TestNewOperationRequestKeepsStableValidatedOperationNames(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)

	for _, name := range []string{"capture", "read_registers"} {
		t.Run(name, func(t *testing.T) {
			input := validOperationRequestInput(t, now)
			input.OperationName = name

			request, err := NewOperationRequest(input, now)
			if err != nil {
				t.Fatalf("NewOperationRequest(%q) error = %v", name, err)
			}
			if request.OperationName() != name {
				t.Fatalf("OperationName() = %q, want %q", request.OperationName(), name)
			}
		})
	}
}

func TestNewOperationRequestExposesValueGetters(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	input := validOperationRequestInput(t, now)

	request, err := NewOperationRequest(input, now)
	if err != nil {
		t.Fatalf("NewOperationRequest() error = %v", err)
	}

	if got, want := request.ID(), input.ID; got != want {
		t.Fatalf("ID() = %v, want %v", got, want)
	}
	if got, want := request.Actor(), input.Actor; got != want {
		t.Fatalf("Actor() = %v, want %v", got, want)
	}
	if got, want := request.IntegrationID(), input.IntegrationID; got != want {
		t.Fatalf("IntegrationID() = %v, want %v", got, want)
	}
	if got, want := request.OperationName(), input.OperationName; got != want {
		t.Fatalf("OperationName() = %q, want %q", got, want)
	}
	if got, want := request.DeviceID(), input.DeviceID; got != want {
		t.Fatalf("DeviceID() = %v, want %v", got, want)
	}
	if got, want := string(request.CanonicalParameters()), string(input.CanonicalParameters); got != want {
		t.Fatalf("CanonicalParameters() = %q, want %q", got, want)
	}
	if got, want := request.ConfigurationSnapshot(), input.ConfigurationSnapshot; got != want {
		t.Fatalf("ConfigurationSnapshot() = %v, want %v", got, want)
	}
	if got, want := request.PolicySnapshot(), input.PolicySnapshot; got != want {
		t.Fatalf("PolicySnapshot() = %v, want %v", got, want)
	}
	if got, want := request.Deadline(), input.Deadline; !got.Equal(want) {
		t.Fatalf("Deadline() = %s, want %s", got, want)
	}
	if got := request.CreatedAt(); !got.Equal(now) {
		t.Fatalf("CreatedAt() = %s, want %s", got, now)
	}
}

func TestNewOperationRequestRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	oversizedJSON := append([]byte{'"'}, bytes.Repeat([]byte("a"), 1<<20)...)
	oversizedJSON = append(oversizedJSON, '"')

	tests := []struct {
		name   string
		mutate func(*OperationRequestInput)
	}{
		{name: "empty request ID", mutate: func(in *OperationRequestInput) { in.ID = RequestID{} }},
		{name: "empty actor", mutate: func(in *OperationRequestInput) { in.Actor = Actor{} }},
		{name: "empty integration ID", mutate: func(in *OperationRequestInput) { in.IntegrationID = IntegrationID{} }},
		{name: "empty operation name", mutate: func(in *OperationRequestInput) { in.OperationName = "" }},
		{name: "invalid operation name", mutate: func(in *OperationRequestInput) { in.OperationName = "Capture-1" }},
		{name: "empty device ID", mutate: func(in *OperationRequestInput) { in.DeviceID = DeviceID{} }},
		{name: "empty configuration snapshot", mutate: func(in *OperationRequestInput) { in.ConfigurationSnapshot = SnapshotID{} }},
		{name: "empty policy snapshot", mutate: func(in *OperationRequestInput) { in.PolicySnapshot = SnapshotID{} }},
		{name: "invalid canonical JSON", mutate: func(in *OperationRequestInput) { in.CanonicalParameters = json.RawMessage(`{"broken":`) }},
		{name: "deadline at creation time", mutate: func(in *OperationRequestInput) { in.Deadline = now }},
		{name: "deadline before creation time", mutate: func(in *OperationRequestInput) { in.Deadline = now.Add(-time.Nanosecond) }},
		{name: "parameters over one MiB", mutate: func(in *OperationRequestInput) { in.CanonicalParameters = oversizedJSON }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validOperationRequestInput(t, now)
			test.mutate(&input)

			if _, err := NewOperationRequest(input, now); err == nil {
				t.Fatal("NewOperationRequest() succeeded, want rejection")
			}
		})
	}
}

func TestNewOperationRequestCopiesCanonicalParameters(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	input := validOperationRequestInput(t, now)
	input.CanonicalParameters = json.RawMessage(`{"mode":"fast"}`)

	request, err := NewOperationRequest(input, now)
	if err != nil {
		t.Fatalf("NewOperationRequest() error = %v", err)
	}

	input.CanonicalParameters[2] = 'X'
	firstRead := request.CanonicalParameters()
	if got, want := string(firstRead), `{"mode":"fast"}`; got != want {
		t.Fatalf("CanonicalParameters() = %q after caller mutation, want %q", got, want)
	}

	firstRead[2] = 'Y'
	if got, want := string(request.CanonicalParameters()), `{"mode":"fast"}`; got != want {
		t.Fatalf("CanonicalParameters() = %q after accessor result mutation, want %q", got, want)
	}
	if !request.CreatedAt().Equal(now) {
		t.Fatalf("CreatedAt() = %s, want %s", request.CreatedAt(), now)
	}
}

func validOperationRequestInput(t *testing.T, now time.Time) OperationRequestInput {
	t.Helper()

	requestID, err := NewRequestID()
	if err != nil {
		t.Fatalf("NewRequestID() error = %v", err)
	}
	actorID, err := NewActorID()
	if err != nil {
		t.Fatalf("NewActorID() error = %v", err)
	}
	integrationID, err := NewIntegrationID()
	if err != nil {
		t.Fatalf("NewIntegrationID() error = %v", err)
	}
	deviceID, err := NewDeviceID()
	if err != nil {
		t.Fatalf("NewDeviceID() error = %v", err)
	}
	configurationSnapshot, err := NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	policySnapshot, err := NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}

	return OperationRequestInput{
		ID:                    requestID,
		Actor:                 Actor{ID: actorID},
		IntegrationID:         integrationID,
		OperationName:         "capture",
		DeviceID:              deviceID,
		CanonicalParameters:   json.RawMessage(`{"mode":"fast"}`),
		ConfigurationSnapshot: configurationSnapshot,
		PolicySnapshot:        policySnapshot,
		Deadline:              now.Add(time.Minute),
	}
}
