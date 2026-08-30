package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/domain"
)

var (
	ErrInvalidAppMode          = errors.New("invalid app mode")
	ErrInvalidOperationPolicy  = errors.New("invalid operation policy")
	ErrInvalidDecisionKind     = errors.New("invalid decision kind")
	ErrInvalidDecisionReason   = errors.New("invalid decision reason")
	ErrInvalidApprovalDecision = errors.New("invalid approval decision")
	ErrInvalidResolutionInput  = errors.New("invalid resolution input")
	ErrInvalidSnapshot         = errors.New("invalid policy snapshot")
	ErrInvalidCanonicalRequest = errors.New("invalid canonical request")
	ErrInvalidApprovalRecord   = errors.New("invalid approval record")
	ErrSnapshotAlreadyStored   = errors.New("policy snapshot already stored")
	ErrApprovalAlreadyRecorded = errors.New("approval decision already recorded")
	ErrApprovalNotFound        = errors.New("approval not found")
	ErrApprovalDigestMismatch  = errors.New("approval request digest mismatch")
	ErrApprovalExpired         = errors.New("approval expired")
	ErrApprovalDenied          = errors.New("request was denied")
)

var canonicalJSONNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`)

type AppMode string

const (
	Ask  AppMode = "ask"
	Auto AppMode = "auto"
	Full AppMode = "full"
)

func ParseAppMode(value string) (AppMode, error) {
	mode := AppMode(value)
	if err := mode.Validate(); err != nil {
		return "", err
	}
	return mode, nil
}

func (mode AppMode) Validate() error {
	switch mode {
	case Ask, Auto, Full:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidAppMode, mode)
	}
}

type OperationPolicy string

const (
	AutoEligible OperationPolicy = "auto_eligible"
	FullOnly     OperationPolicy = "full_only"
	AlwaysPrompt OperationPolicy = "always_prompt"
	Denied       OperationPolicy = "denied"
)

func ParseOperationPolicy(value string) (OperationPolicy, error) {
	operationPolicy := OperationPolicy(value)
	if err := operationPolicy.Validate(); err != nil {
		return "", err
	}
	return operationPolicy, nil
}

func (operationPolicy OperationPolicy) Validate() error {
	switch operationPolicy {
	case AutoEligible, FullOnly, AlwaysPrompt, Denied:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidOperationPolicy, operationPolicy)
	}
}

type DecisionKind string

const (
	Allow  DecisionKind = "allow"
	Prompt DecisionKind = "prompt"
	Deny   DecisionKind = "deny"
)

func ParseDecisionKind(value string) (DecisionKind, error) {
	decision := DecisionKind(value)
	if err := decision.Validate(); err != nil {
		return "", err
	}
	return decision, nil
}

func (decision DecisionKind) Validate() error {
	switch decision {
	case Allow, Prompt, Deny:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidDecisionKind, decision)
	}
}

type DecisionReason string

const (
	DeniedPolicyForcedDeny DecisionReason = "denied_policy_forced_deny"
	UnknownAppForcedAsk    DecisionReason = "unknown_app_forced_ask"
	UnknownDeviceForcedAsk DecisionReason = "unknown_device_forced_ask"
	DevicePolicyLowered    DecisionReason = "device_policy_lowered"
	PolicyMatrixApplied    DecisionReason = "policy_matrix_applied"
)

func ParseDecisionReason(value string) (DecisionReason, error) {
	reason := DecisionReason(value)
	if err := reason.Validate(); err != nil {
		return "", err
	}
	return reason, nil
}

type ApprovalDecision string

const (
	ApprovalGranted  ApprovalDecision = "approved"
	ApprovalRejected ApprovalDecision = "denied"
)

func ParseApprovalDecision(value string) (ApprovalDecision, error) {
	decision := ApprovalDecision(value)
	if err := decision.Validate(); err != nil {
		return "", err
	}
	return decision, nil
}

func (decision ApprovalDecision) Validate() error {
	switch decision {
	case ApprovalGranted, ApprovalRejected:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidApprovalDecision, decision)
	}
}

func (reason DecisionReason) Validate() error {
	switch reason {
	case DeniedPolicyForcedDeny, UnknownAppForcedAsk, UnknownDeviceForcedAsk, DevicePolicyLowered, PolicyMatrixApplied:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidDecisionReason, reason)
	}
}

type ResolutionInput struct {
	SnapshotID        domain.SnapshotID
	IntegrationID     domain.IntegrationID
	OperationName     string
	DeviceID          domain.DeviceID
	AppKnown          bool
	AppMode           AppMode
	ManifestPolicy    OperationPolicy
	OperationOverride *OperationPolicy
	DeviceKnown       bool
	DeviceMode        AppMode
	ResolvedAt        time.Time
}

func (input ResolutionInput) Validate() error {
	if isZeroID(input.SnapshotID.String()) || isZeroID(input.IntegrationID.String()) || isZeroID(input.DeviceID.String()) {
		return fmt.Errorf("%w: snapshot, integration, and device identity are required", ErrInvalidResolutionInput)
	}
	if matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*$`, input.OperationName); len(input.OperationName) > domain.MaxOperationNameBytes || !matched {
		return fmt.Errorf("%w: operation name must be a stable descriptor identifier", ErrInvalidResolutionInput)
	}
	if err := input.AppMode.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResolutionInput, err)
	}
	if err := input.ManifestPolicy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResolutionInput, err)
	}
	if input.OperationOverride != nil {
		if err := input.OperationOverride.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidResolutionInput, err)
		}
	}
	if input.DeviceKnown {
		if err := input.DeviceMode.Validate(); err != nil {
			return fmt.Errorf("%w: known device: %v", ErrInvalidResolutionInput, err)
		}
	} else if input.DeviceMode != "" {
		return fmt.Errorf("%w: unknown device cannot carry a configured mode", ErrInvalidResolutionInput)
	}
	if input.ResolvedAt.IsZero() {
		return fmt.Errorf("%w: resolution time is required", ErrInvalidResolutionInput)
	}
	return nil
}

type Snapshot struct {
	ID                domain.SnapshotID
	IntegrationID     domain.IntegrationID
	OperationName     string
	DeviceID          domain.DeviceID
	AppKnown          bool
	AppMode           AppMode
	ManifestPolicy    OperationPolicy
	OperationOverride *OperationPolicy
	DeviceKnown       bool
	DeviceMode        AppMode
	EffectiveMode     AppMode
	EffectivePolicy   OperationPolicy
	Decision          DecisionKind
	Reason            DecisionReason
	ResolvedAt        time.Time
}

// ApprovalRecord binds one administrator decision to exactly one immutable
// operation request and expires at that request's deadline.
type ApprovalRecord struct {
	ID            string
	RequestID     domain.RequestID
	RequestDigest [sha256.Size]byte
	Decision      ApprovalDecision
	ActorID       domain.ActorID
	ExpiresAt     time.Time
	RecordedAt    time.Time
}

func (record ApprovalRecord) Validate() error {
	id, err := uuid.Parse(record.ID)
	if err != nil || id == uuid.Nil {
		return fmt.Errorf("%w: approval ID must be a UUID", ErrInvalidApprovalRecord)
	}
	if isZeroID(record.RequestID.String()) || isZeroID(record.ActorID.String()) {
		return fmt.Errorf("%w: request and actor identity are required", ErrInvalidApprovalRecord)
	}
	if err := record.Decision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidApprovalRecord, err)
	}
	if record.RecordedAt.IsZero() || record.ExpiresAt.IsZero() || !record.ExpiresAt.After(record.RecordedAt) {
		return fmt.Errorf("%w: expiry must be after the decision time", ErrInvalidApprovalRecord)
	}
	return nil
}

// BindApproval creates a record whose digest and expiry are derived from the
// immutable request rather than accepted independently from a caller.
func BindApproval(id string, request domain.OperationRequest, decision ApprovalDecision, actorID domain.ActorID, recordedAt time.Time) (ApprovalRecord, error) {
	digest, err := CanonicalRequestDigest(request)
	if err != nil {
		return ApprovalRecord{}, err
	}
	record := ApprovalRecord{
		ID: id, RequestID: request.ID(), RequestDigest: digest,
		Decision: decision, ActorID: actorID,
		ExpiresAt: request.Deadline().UTC(), RecordedAt: recordedAt.UTC(),
	}
	if err := record.Validate(); err != nil {
		return ApprovalRecord{}, err
	}
	return record, nil
}

type canonicalOperationRequest struct {
	Schema                  string          `json:"schema"`
	RequestID               string          `json:"request_id"`
	ActorID                 string          `json:"actor_id"`
	IntegrationID           string          `json:"integration_id"`
	Operation               string          `json:"operation"`
	DeviceID                string          `json:"device_id"`
	Parameters              json.RawMessage `json:"parameters"`
	ConfigurationSnapshotID string          `json:"configuration_snapshot_id"`
	PolicySnapshotID        string          `json:"policy_snapshot_id"`
	Deadline                string          `json:"deadline"`
	CreatedAt               string          `json:"created_at"`
}

// CanonicalRequestJSON produces a versioned, deterministic JSON encoding of
// every authoritative field in an immutable operation request.
func CanonicalRequestJSON(request domain.OperationRequest) ([]byte, error) {
	if isZeroID(request.ID().String()) || isZeroID(request.Actor().ID.String()) ||
		isZeroID(request.IntegrationID().String()) || isZeroID(request.DeviceID().String()) ||
		isZeroID(request.ConfigurationSnapshot().String()) || isZeroID(request.PolicySnapshot().String()) ||
		request.OperationName() == "" || request.Deadline().IsZero() || request.CreatedAt().IsZero() {
		return nil, fmt.Errorf("%w: immutable request is incomplete", ErrInvalidCanonicalRequest)
	}
	parameters, err := canonicalizeJSON(request.CanonicalParameters())
	if err != nil {
		return nil, fmt.Errorf("%w: parameters: %v", ErrInvalidCanonicalRequest, err)
	}
	canonical, err := json.Marshal(canonicalOperationRequest{
		Schema: "nilo.operation_request/1", RequestID: request.ID().String(),
		ActorID: request.Actor().ID.String(), IntegrationID: request.IntegrationID().String(),
		Operation: request.OperationName(), DeviceID: request.DeviceID().String(), Parameters: parameters,
		ConfigurationSnapshotID: request.ConfigurationSnapshot().String(), PolicySnapshotID: request.PolicySnapshot().String(),
		Deadline: request.Deadline().UTC().Format(time.RFC3339Nano), CreatedAt: request.CreatedAt().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: encode: %v", ErrInvalidCanonicalRequest, err)
	}
	return canonical, nil
}

func CanonicalRequestDigest(request domain.OperationRequest) ([sha256.Size]byte, error) {
	canonical, err := CanonicalRequestJSON(request)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(canonical), nil
}

func canonicalizeJSON(input []byte) (json.RawMessage, error) {
	if err := rejectDuplicateObjectKeys(input); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func rejectDuplicateObjectKeys(input []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func walkJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		if number, isNumber := token.(json.Number); isNumber {
			spelling := number.String()
			if spelling == "-0" || !canonicalJSONNumberPattern.MatchString(spelling) {
				return fmt.Errorf("noncanonical JSON number %q", spelling)
			}
		}
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func (snapshot Snapshot) Validate() error {
	input := ResolutionInput{
		SnapshotID: snapshot.ID, IntegrationID: snapshot.IntegrationID,
		OperationName: snapshot.OperationName, DeviceID: snapshot.DeviceID,
		AppKnown: snapshot.AppKnown, AppMode: snapshot.AppMode,
		ManifestPolicy: snapshot.ManifestPolicy, OperationOverride: snapshot.OperationOverride,
		DeviceKnown: snapshot.DeviceKnown, DeviceMode: snapshot.DeviceMode,
		ResolvedAt: snapshot.ResolvedAt,
	}
	if err := input.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := snapshot.EffectiveMode.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := snapshot.EffectivePolicy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := snapshot.Decision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := snapshot.Reason.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if snapshot.ResolvedAt.Location() != time.UTC {
		return fmt.Errorf("%w: resolution time must be UTC", ErrInvalidSnapshot)
	}
	expected, err := Resolve(input)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if snapshot.EffectiveMode != expected.EffectiveMode ||
		snapshot.EffectivePolicy != expected.EffectivePolicy ||
		snapshot.Decision != expected.Decision || snapshot.Reason != expected.Reason {
		return fmt.Errorf("%w: effective result does not match resolution inputs", ErrInvalidSnapshot)
	}
	return nil
}

func isZeroID(value string) bool {
	return value == "00000000-0000-0000-0000-000000000000"
}
