package policy

func Resolve(input ResolutionInput) (Snapshot, error) {
	if err := input.Validate(); err != nil {
		return Snapshot{}, err
	}

	effectivePolicy := input.ManifestPolicy
	var operationOverride *OperationPolicy
	if input.OperationOverride != nil {
		override := *input.OperationOverride
		operationOverride = &override
		effectivePolicy = override
	}

	effectiveMode := input.AppMode
	reason := PolicyMatrixApplied
	if !input.AppKnown {
		effectiveMode = Ask
		reason = UnknownAppForcedAsk
	} else if !input.DeviceKnown {
		effectiveMode = Ask
		reason = UnknownDeviceForcedAsk
	} else if modeAuthority(input.DeviceMode) < modeAuthority(effectiveMode) {
		effectiveMode = input.DeviceMode
		reason = DevicePolicyLowered
	}

	decision := matrixDecision(effectivePolicy, effectiveMode)
	if effectivePolicy == Denied {
		decision = Deny
		reason = DeniedPolicyForcedDeny
	}

	return Snapshot{
		ID: input.SnapshotID, IntegrationID: input.IntegrationID,
		OperationName: input.OperationName, DeviceID: input.DeviceID,
		AppKnown: input.AppKnown, AppMode: input.AppMode,
		ManifestPolicy: input.ManifestPolicy, OperationOverride: operationOverride,
		DeviceKnown: input.DeviceKnown, DeviceMode: input.DeviceMode,
		EffectiveMode: effectiveMode, EffectivePolicy: effectivePolicy,
		Decision: decision, Reason: reason, ResolvedAt: input.ResolvedAt.UTC(),
	}, nil
}

func matrixDecision(operationPolicy OperationPolicy, mode AppMode) DecisionKind {
	switch operationPolicy {
	case AutoEligible:
		if mode == Auto || mode == Full {
			return Allow
		}
		return Prompt
	case FullOnly:
		if mode == Full {
			return Allow
		}
		return Prompt
	case AlwaysPrompt:
		return Prompt
	case Denied:
		return Deny
	default:
		return Deny
	}
}

func modeAuthority(mode AppMode) int {
	switch mode {
	case Ask:
		return 0
	case Auto:
		return 1
	case Full:
		return 2
	default:
		return -1
	}
}
