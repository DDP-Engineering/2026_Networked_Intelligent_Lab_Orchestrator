package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const MaxCanonicalParametersBytes = 1 << 20

var operationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type OperationRequestInput struct {
	ID                    RequestID
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
	ID                    RequestID
	Actor                 Actor
	IntegrationID         IntegrationID
	OperationName         string
	DeviceID              DeviceID
	CanonicalParameters   json.RawMessage
	ConfigurationSnapshot SnapshotID
	PolicySnapshot        SnapshotID
	Deadline              time.Time
	CreatedAt             time.Time
}

func NewOperationRequest(input OperationRequestInput, createdAt time.Time) (OperationRequest, error) {
	if input.ID.isZero() || input.Actor.ID.isZero() || input.IntegrationID.isZero() || input.DeviceID.isZero() || input.ConfigurationSnapshot.isZero() || input.PolicySnapshot.isZero() {
		return OperationRequest{}, errors.New("operation request identity is required")
	}
	if !operationNamePattern.MatchString(input.OperationName) {
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
		ID:                    input.ID,
		Actor:                 input.Actor,
		IntegrationID:         input.IntegrationID,
		OperationName:         input.OperationName,
		DeviceID:              input.DeviceID,
		CanonicalParameters:   append(json.RawMessage(nil), input.CanonicalParameters...),
		ConfigurationSnapshot: input.ConfigurationSnapshot,
		PolicySnapshot:        input.PolicySnapshot,
		Deadline:              input.Deadline,
		CreatedAt:             createdAt,
	}, nil
}
