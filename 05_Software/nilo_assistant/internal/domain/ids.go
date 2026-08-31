package domain

import "github.com/google/uuid"

type RequestID uuid.UUID

func NewRequestID() (RequestID, error) {
	id, err := newUUIDv7()
	return RequestID(id), err
}

func ParseRequestID(value string) (RequestID, error) {
	id, err := uuid.Parse(value)
	return RequestID(id), err
}

func (id RequestID) String() string { return uuid.UUID(id).String() }

func (id RequestID) isZero() bool { return uuid.UUID(id) == uuid.Nil }

type SessionID uuid.UUID

func NewSessionID() (SessionID, error) {
	id, err := newUUIDv7()
	return SessionID(id), err
}

func ParseSessionID(value string) (SessionID, error) {
	id, err := uuid.Parse(value)
	return SessionID(id), err
}

func (id SessionID) String() string { return uuid.UUID(id).String() }

type ActorID uuid.UUID

func NewActorID() (ActorID, error) {
	id, err := newUUIDv7()
	return ActorID(id), err
}

func ParseActorID(value string) (ActorID, error) {
	id, err := uuid.Parse(value)
	return ActorID(id), err
}

func (id ActorID) String() string { return uuid.UUID(id).String() }

func (id ActorID) isZero() bool { return uuid.UUID(id) == uuid.Nil }

type Actor struct {
	ID ActorID
}

type DeviceID uuid.UUID

func NewDeviceID() (DeviceID, error) {
	id, err := newUUIDv7()
	return DeviceID(id), err
}

func ParseDeviceID(value string) (DeviceID, error) {
	id, err := uuid.Parse(value)
	return DeviceID(id), err
}

func (id DeviceID) String() string { return uuid.UUID(id).String() }

func (id DeviceID) isZero() bool { return uuid.UUID(id) == uuid.Nil }

type IntegrationID uuid.UUID

func NewIntegrationID() (IntegrationID, error) {
	id, err := newUUIDv7()
	return IntegrationID(id), err
}

func ParseIntegrationID(value string) (IntegrationID, error) {
	id, err := uuid.Parse(value)
	return IntegrationID(id), err
}

func (id IntegrationID) String() string { return uuid.UUID(id).String() }

func (id IntegrationID) isZero() bool { return uuid.UUID(id) == uuid.Nil }

type ArtifactID uuid.UUID

func NewArtifactID() (ArtifactID, error) {
	id, err := newUUIDv7()
	return ArtifactID(id), err
}

func ParseArtifactID(value string) (ArtifactID, error) {
	id, err := uuid.Parse(value)
	return ArtifactID(id), err
}

func (id ArtifactID) String() string { return uuid.UUID(id).String() }

type SnapshotID uuid.UUID

func NewSnapshotID() (SnapshotID, error) {
	id, err := newUUIDv7()
	return SnapshotID(id), err
}

func ParseSnapshotID(value string) (SnapshotID, error) {
	id, err := uuid.Parse(value)
	return SnapshotID(id), err
}

func (id SnapshotID) String() string { return uuid.UUID(id).String() }

func (id SnapshotID) isZero() bool { return uuid.UUID(id) == uuid.Nil }

func newUUIDv7() (uuid.UUID, error) {
	return uuid.NewV7()
}
