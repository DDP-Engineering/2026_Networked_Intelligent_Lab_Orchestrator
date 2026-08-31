package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const (
	MaxCanonicalParametersBytes = 1 << 20
	MaxOperationNameBytes       = 128
)

var operationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type OperationRequestInput struct {
	Actor                 Actor
	IntegrationID         IntegrationID
	OperationName         string
	DeviceID              DeviceID
	CanonicalParameters   json.RawMessage
	ConfigurationSnapshot SnapshotID
	PolicySnapshot        SnapshotID
	Deadline              time.Time
}

type OperationRequest struct {
	id                    RequestID
	actor                 Actor
	integrationID         IntegrationID
	operationName         string
	deviceID              DeviceID
	canonicalParameters   json.RawMessage
	configurationSnapshot SnapshotID
	policySnapshot        SnapshotID
	deadline              time.Time
	createdAt             time.Time
}

func NewOperationRequest(input OperationRequestInput, createdAt time.Time) (OperationRequest, error) {
	id, err := NewRequestID()
	if err != nil {
		return OperationRequest{}, err
	}
	return buildOperationRequest(id, input, createdAt)
}

// RehydrateOperationRequest reconstructs a previously accepted durable
// request using its stored identity. New requests must use NewOperationRequest.
func RehydrateOperationRequest(id RequestID, input OperationRequestInput, createdAt time.Time) (OperationRequest, error) {
	return buildOperationRequest(id, input, createdAt)
}

func buildOperationRequest(id RequestID, input OperationRequestInput, createdAt time.Time) (OperationRequest, error) {
	if id.isZero() || input.Actor.ID.isZero() || input.IntegrationID.isZero() || input.DeviceID.isZero() || input.ConfigurationSnapshot.isZero() || input.PolicySnapshot.isZero() {
		return OperationRequest{}, errors.New("operation request identity is required")
	}
	if len(input.OperationName) > MaxOperationNameBytes || !operationNamePattern.MatchString(input.OperationName) {
		return OperationRequest{}, errors.New("operation name must be a stable descriptor identifier")
	}
	if len(input.CanonicalParameters) > MaxCanonicalParametersBytes {
		return OperationRequest{}, errors.New("canonical parameters exceed one MiB")
	}
	if !json.Valid(input.CanonicalParameters) {
		return OperationRequest{}, errors.New("canonical parameters must be valid JSON")
	}
	if !input.Deadline.After(createdAt) {
		return OperationRequest{}, errors.New("deadline must be after creation time")
	}

	return OperationRequest{
		id:                    id,
		actor:                 input.Actor,
		integrationID:         input.IntegrationID,
		operationName:         input.OperationName,
		deviceID:              input.DeviceID,
		canonicalParameters:   append(json.RawMessage(nil), input.CanonicalParameters...),
		configurationSnapshot: input.ConfigurationSnapshot,
		policySnapshot:        input.PolicySnapshot,
		deadline:              input.Deadline,
		createdAt:             createdAt,
	}, nil
}

func (request OperationRequest) ID() RequestID { return request.id }

func (request OperationRequest) Actor() Actor { return request.actor }

func (request OperationRequest) IntegrationID() IntegrationID { return request.integrationID }

func (request OperationRequest) OperationName() string { return request.operationName }

func (request OperationRequest) DeviceID() DeviceID { return request.deviceID }

func (request OperationRequest) CanonicalParameters() json.RawMessage {
	return append(json.RawMessage(nil), request.canonicalParameters...)
}

func (request OperationRequest) ConfigurationSnapshot() SnapshotID {
	return request.configurationSnapshot
}

func (request OperationRequest) PolicySnapshot() SnapshotID { return request.policySnapshot }

func (request OperationRequest) Deadline() time.Time { return request.deadline }

func (request OperationRequest) CreatedAt() time.Time { return request.createdAt }
