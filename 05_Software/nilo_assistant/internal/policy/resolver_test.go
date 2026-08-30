package policy

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
)

func TestResolveApprovedPolicyMatrix(t *testing.T) {
	cases := []struct {
		policy OperationPolicy
		mode   AppMode
		want   DecisionKind
	}{
		{AutoEligible, Ask, Prompt}, {AutoEligible, Auto, Allow}, {AutoEligible, Full, Allow},
		{FullOnly, Ask, Prompt}, {FullOnly, Auto, Prompt}, {FullOnly, Full, Allow},
		{AlwaysPrompt, Ask, Prompt}, {AlwaysPrompt, Auto, Prompt}, {AlwaysPrompt, Full, Prompt},
		{Denied, Ask, Deny}, {Denied, Auto, Deny}, {Denied, Full, Deny},
	}

	for _, test := range cases {
		t.Run(string(test.policy)+"_"+string(test.mode), func(t *testing.T) {
			input := validResolutionInput(t)
			input.AppMode = test.mode
			input.ManifestPolicy = test.policy
			input.DeviceMode = test.mode
			got, err := Resolve(input)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Decision != test.want {
				t.Fatalf("Resolve() decision = %q, want %q", got.Decision, test.want)
			}
		})
	}
}

func TestResolveUnknownIdentityAndDeviceAuthorityPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*ResolutionInput)
		wantMode   AppMode
		want       DecisionKind
		wantReason DecisionReason
	}{
		{
			name: "unknown app forces ask",
			mutate: func(input *ResolutionInput) {
				input.AppKnown = false
				input.AppMode = Full
				input.DeviceMode = Full
			},
			wantMode: Ask, want: Prompt, wantReason: UnknownAppForcedAsk,
		},
		{
			name: "unknown device forces ask",
			mutate: func(input *ResolutionInput) {
				input.AppMode = Full
				input.DeviceKnown = false
				input.DeviceMode = ""
			},
			wantMode: Ask, want: Prompt, wantReason: UnknownDeviceForcedAsk,
		},
		{
			name: "known device lowers authority",
			mutate: func(input *ResolutionInput) {
				input.AppMode = Full
				input.DeviceMode = Ask
			},
			wantMode: Ask, want: Prompt, wantReason: DevicePolicyLowered,
		},
		{
			name: "known device cannot raise authority",
			mutate: func(input *ResolutionInput) {
				input.AppMode = Auto
				input.DeviceMode = Full
			},
			wantMode: Auto, want: Allow, wantReason: PolicyMatrixApplied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validResolutionInput(t)
			test.mutate(&input)

			got, err := Resolve(input)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.EffectiveMode != test.wantMode || got.Decision != test.want || got.Reason != test.wantReason {
				t.Fatalf("Resolve() = mode %q, decision %q, reason %q; want %q, %q, %q", got.EffectiveMode, got.Decision, got.Reason, test.wantMode, test.want, test.wantReason)
			}
		})
	}
}

func TestResolveOperationOverrideReplacesDefaultWithoutBypassingBindingPolicies(t *testing.T) {
	tests := []struct {
		name     string
		manifest OperationPolicy
		override OperationPolicy
		mode     AppMode
		want     DecisionKind
	}{
		{name: "override replaces denied default", manifest: Denied, override: AutoEligible, mode: Auto, want: Allow},
		{name: "denied override remains binding in full", manifest: AutoEligible, override: Denied, mode: Full, want: Deny},
		{name: "always prompt override remains binding in full", manifest: AutoEligible, override: AlwaysPrompt, mode: Full, want: Prompt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validResolutionInput(t)
			input.ManifestPolicy = test.manifest
			input.OperationOverride = &test.override
			input.AppMode = test.mode
			input.DeviceMode = test.mode

			got, err := Resolve(input)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.EffectivePolicy != test.override || got.Decision != test.want {
				t.Fatalf("Resolve() = policy %q, decision %q; want %q, %q", got.EffectivePolicy, got.Decision, test.override, test.want)
			}
		})
	}
}

func TestResolveSnapshotRecordsAllInputsAndCopiesOverride(t *testing.T) {
	input := validResolutionInput(t)
	override := FullOnly
	input.OperationOverride = &override
	input.AppMode = Full
	input.DeviceMode = Auto

	got, err := Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	override = Denied

	if got.ID != input.SnapshotID || got.IntegrationID != input.IntegrationID || got.OperationName != input.OperationName || got.DeviceID != input.DeviceID {
		t.Fatalf("snapshot identity = %#v, want input identity", got)
	}
	if got.AppKnown != input.AppKnown || got.AppMode != Full || got.ManifestPolicy != AutoEligible || got.OperationOverride == nil || *got.OperationOverride != FullOnly {
		t.Fatalf("snapshot app inputs = %#v, want original values", got)
	}
	if got.DeviceKnown != input.DeviceKnown || got.DeviceMode != Auto || got.EffectiveMode != Auto || got.EffectivePolicy != FullOnly {
		t.Fatalf("snapshot effective values = %#v, want device-lowered FullOnly values", got)
	}
	if got.Decision != Prompt || got.Reason != DevicePolicyLowered || !got.ResolvedAt.Equal(input.ResolvedAt.UTC()) {
		t.Fatalf("snapshot result = %#v, want prompt/device-lowered at %s", got, input.ResolvedAt.UTC())
	}
}

func TestSnapshotValidateRejectsAResultThatDoesNotMatchItsInputs(t *testing.T) {
	input := validResolutionInput(t)
	input.AppMode = Ask
	input.DeviceMode = Ask
	snapshot, err := Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	snapshot.Decision = Allow
	if err := snapshot.Validate(); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("Snapshot.Validate() error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestResolveRejectsOpenEndedOrIncompleteInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResolutionInput)
	}{
		{name: "unknown app mode", mutate: func(input *ResolutionInput) { input.AppMode = "operator" }},
		{name: "unknown manifest policy", mutate: func(input *ResolutionInput) { input.ManifestPolicy = "permit" }},
		{name: "unknown override policy", mutate: func(input *ResolutionInput) { invalid := OperationPolicy("permit"); input.OperationOverride = &invalid }},
		{name: "known device without mode", mutate: func(input *ResolutionInput) { input.DeviceMode = "" }},
		{name: "unknown device with configured mode", mutate: func(input *ResolutionInput) { input.DeviceKnown = false }},
		{name: "missing snapshot ID", mutate: func(input *ResolutionInput) { input.SnapshotID = domain.SnapshotID{} }},
		{name: "missing integration ID", mutate: func(input *ResolutionInput) { input.IntegrationID = domain.IntegrationID{} }},
		{name: "invalid operation name", mutate: func(input *ResolutionInput) { input.OperationName = "Capture-1" }},
		{name: "missing device ID", mutate: func(input *ResolutionInput) { input.DeviceID = domain.DeviceID{} }},
		{name: "missing resolution time", mutate: func(input *ResolutionInput) { input.ResolvedAt = time.Time{} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validResolutionInput(t)
			test.mutate(&input)
			_, err := Resolve(input)
			if !errors.Is(err, ErrInvalidResolutionInput) {
				t.Fatalf("Resolve() error = %v, want ErrInvalidResolutionInput", err)
			}
		})
	}
}

func TestResolveEnforcesPersistedOperationNameBound(t *testing.T) {
	input := validResolutionInput(t)
	input.OperationName = strings.Repeat("a", 128)
	if _, err := Resolve(input); err != nil {
		t.Fatalf("Resolve(128-character name) error = %v", err)
	}
	input.OperationName = strings.Repeat("a", 129)
	if _, err := Resolve(input); !errors.Is(err, ErrInvalidResolutionInput) {
		t.Fatalf("Resolve(129-character name) error = %v, want ErrInvalidResolutionInput", err)
	}
}

func TestPolicyStringTypesRejectUnknownValues(t *testing.T) {
	if _, err := ParseAppMode("operator"); !errors.Is(err, ErrInvalidAppMode) {
		t.Fatalf("ParseAppMode() error = %v, want ErrInvalidAppMode", err)
	}
	if _, err := ParseOperationPolicy("permit"); !errors.Is(err, ErrInvalidOperationPolicy) {
		t.Fatalf("ParseOperationPolicy() error = %v, want ErrInvalidOperationPolicy", err)
	}
	if _, err := ParseDecisionKind("maybe"); !errors.Is(err, ErrInvalidDecisionKind) {
		t.Fatalf("ParseDecisionKind() error = %v, want ErrInvalidDecisionKind", err)
	}
	if _, err := ParseDecisionReason("because"); !errors.Is(err, ErrInvalidDecisionReason) {
		t.Fatalf("ParseDecisionReason() error = %v, want ErrInvalidDecisionReason", err)
	}
	if _, err := ParseApprovalDecision("maybe"); !errors.Is(err, ErrInvalidApprovalDecision) {
		t.Fatalf("ParseApprovalDecision() error = %v, want ErrInvalidApprovalDecision", err)
	}
}

func TestCanonicalRequestJSONIsDeterministicAndUnambiguous(t *testing.T) {
	request := operationRequestWithParameters(t, `{"channel":"A","amplitude":2}`)
	reordered := operationRequestWithParameters(t, " { \"amplitude\" : 2, \"channel\" : \"A\" } ")

	canonical, err := CanonicalRequestJSON(request)
	if err != nil {
		t.Fatalf("CanonicalRequestJSON() error = %v", err)
	}
	reorderedCanonical, err := CanonicalRequestJSON(reordered)
	if err != nil {
		t.Fatalf("CanonicalRequestJSON(reordered) error = %v", err)
	}
	if !bytes.Equal(canonical, reorderedCanonical) {
		t.Fatalf("canonical request changed with parameter whitespace/order:\nfirst  = %s\nsecond = %s", canonical, reorderedCanonical)
	}

	want := `{"schema":"nilo.operation_request/1","request_id":"018f3d8d-8d2c-7c20-8e0c-000000000101","actor_id":"018f3d8d-8d2c-7c20-8e0c-000000000102","integration_id":"018f3d8d-8d2c-7c20-8e0c-000000000103","operation":"capture","device_id":"018f3d8d-8d2c-7c20-8e0c-000000000104","parameters":{"amplitude":2,"channel":"A"},"configuration_snapshot_id":"018f3d8d-8d2c-7c20-8e0c-000000000105","policy_snapshot_id":"018f3d8d-8d2c-7c20-8e0c-000000000106","deadline":"2026-08-30T13:00:00Z","created_at":"2026-08-30T12:00:00Z"}`
	if string(canonical) != want {
		t.Fatalf("CanonicalRequestJSON() = %s, want %s", canonical, want)
	}
}

func TestCanonicalRequestDigestChangesWithRequestParameters(t *testing.T) {
	original := operationRequestWithParameters(t, `{"amplitude":2,"channel":"A"}`)
	changed := operationRequestWithParameters(t, `{"amplitude":3,"channel":"A"}`)

	originalDigest, err := CanonicalRequestDigest(original)
	if err != nil {
		t.Fatalf("CanonicalRequestDigest(original) error = %v", err)
	}
	changedDigest, err := CanonicalRequestDigest(changed)
	if err != nil {
		t.Fatalf("CanonicalRequestDigest(changed) error = %v", err)
	}
	if originalDigest == changedDigest {
		t.Fatal("changed request parameters produced the same canonical request digest")
	}
}

func TestCanonicalRequestJSONRejectsDuplicateParameterKeys(t *testing.T) {
	request := operationRequestWithParameters(t, `{"channel":"A","channel":"B"}`)
	if _, err := CanonicalRequestJSON(request); !errors.Is(err, ErrInvalidCanonicalRequest) {
		t.Fatalf("CanonicalRequestJSON() error = %v, want ErrInvalidCanonicalRequest", err)
	}
}

func TestCanonicalRequestJSONRejectsNoncanonicalEquivalentNumberSpellings(t *testing.T) {
	canonical := operationRequestWithParameters(t, `{"amplitude":2}`)
	if _, err := CanonicalRequestJSON(canonical); err != nil {
		t.Fatalf("CanonicalRequestJSON(canonical number) error = %v", err)
	}

	for _, parameters := range []string{`{"amplitude":2.0}`, `{"amplitude":2e0}`} {
		t.Run(parameters, func(t *testing.T) {
			request := operationRequestWithParameters(t, parameters)
			if _, err := CanonicalRequestJSON(request); !errors.Is(err, ErrInvalidCanonicalRequest) {
				t.Fatalf("CanonicalRequestJSON(%s) error = %v, want ErrInvalidCanonicalRequest", parameters, err)
			}
		})
	}
}

func TestBindApprovalUsesImmutableRequestDigestAndDeadline(t *testing.T) {
	request := operationRequestWithParameters(t, `{"amplitude":2,"channel":"A"}`)
	actorID, err := domain.ParseActorID("018f3d8d-8d2c-7c20-8e0c-000000000107")
	if err != nil {
		t.Fatalf("ParseActorID() error = %v", err)
	}
	recordedAt := time.Date(2026, 8, 30, 12, 5, 0, 0, time.FixedZone("EEST", 3*60*60))

	record, err := BindApproval("018f3d8d-8d2c-7c20-8e0c-000000000108", request, ApprovalGranted, actorID, recordedAt)
	if err != nil {
		t.Fatalf("BindApproval() error = %v", err)
	}
	wantDigest, err := CanonicalRequestDigest(request)
	if err != nil {
		t.Fatalf("CanonicalRequestDigest() error = %v", err)
	}
	if record.RequestID != request.ID() || record.RequestDigest != wantDigest || record.Decision != ApprovalGranted || record.ActorID != actorID {
		t.Fatalf("BindApproval() record = %#v, want request-bound approval", record)
	}
	if !record.ExpiresAt.Equal(request.Deadline().UTC()) || !record.RecordedAt.Equal(recordedAt.UTC()) {
		t.Fatalf("BindApproval() times = expires %s, recorded %s; want %s and %s", record.ExpiresAt, record.RecordedAt, request.Deadline().UTC(), recordedAt.UTC())
	}
}

func validResolutionInput(t *testing.T) ResolutionInput {
	t.Helper()
	snapshotID, err := domain.NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	integrationID, err := domain.NewIntegrationID()
	if err != nil {
		t.Fatalf("NewIntegrationID() error = %v", err)
	}
	deviceID, err := domain.NewDeviceID()
	if err != nil {
		t.Fatalf("NewDeviceID() error = %v", err)
	}
	return ResolutionInput{
		SnapshotID:     snapshotID,
		IntegrationID:  integrationID,
		OperationName:  "capture",
		DeviceID:       deviceID,
		AppKnown:       true,
		AppMode:        Auto,
		ManifestPolicy: AutoEligible,
		DeviceKnown:    true,
		DeviceMode:     Auto,
		ResolvedAt:     time.Date(2026, 8, 30, 15, 4, 5, 123456789, time.FixedZone("EEST", 3*60*60)),
	}
}

func operationRequestWithParameters(t *testing.T, parameters string) domain.OperationRequest {
	t.Helper()
	requestID, err := domain.ParseRequestID("018f3d8d-8d2c-7c20-8e0c-000000000101")
	if err != nil {
		t.Fatalf("ParseRequestID() error = %v", err)
	}
	actorID, err := domain.ParseActorID("018f3d8d-8d2c-7c20-8e0c-000000000102")
	if err != nil {
		t.Fatalf("ParseActorID() error = %v", err)
	}
	integrationID, err := domain.ParseIntegrationID("018f3d8d-8d2c-7c20-8e0c-000000000103")
	if err != nil {
		t.Fatalf("ParseIntegrationID() error = %v", err)
	}
	deviceID, err := domain.ParseDeviceID("018f3d8d-8d2c-7c20-8e0c-000000000104")
	if err != nil {
		t.Fatalf("ParseDeviceID() error = %v", err)
	}
	configurationSnapshot, err := domain.ParseSnapshotID("018f3d8d-8d2c-7c20-8e0c-000000000105")
	if err != nil {
		t.Fatalf("ParseSnapshotID(configuration) error = %v", err)
	}
	policySnapshot, err := domain.ParseSnapshotID("018f3d8d-8d2c-7c20-8e0c-000000000106")
	if err != nil {
		t.Fatalf("ParseSnapshotID(policy) error = %v", err)
	}
	createdAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	request, err := domain.RehydrateOperationRequest(requestID, domain.OperationRequestInput{
		Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID,
		OperationName: "capture", DeviceID: deviceID,
		CanonicalParameters: []byte(parameters), ConfigurationSnapshot: configurationSnapshot,
		PolicySnapshot: policySnapshot, Deadline: createdAt.Add(time.Hour),
	}, createdAt)
	if err != nil {
		t.Fatalf("RehydrateOperationRequest() error = %v", err)
	}
	return request
}
