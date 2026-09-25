package domain

import "time"

// Artifact is immutable metadata for content already verified and durably
// placed by the artifact store.
type Artifact struct {
	ID                ArtifactID
	RequestID         RequestID
	SessionID         SessionID
	DeviceID          DeviceID
	IntegrationID     IntegrationID
	MediaType         string
	Digest            [32]byte
	SizeBytes         int64
	TerminalState     SessionState
	PresentationHints []byte
	CreatedAt         time.Time
}
