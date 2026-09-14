package network

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
)

// RecoveryWindow is the default bounded direct-connect recovery interval.
const RecoveryWindow = 10 * time.Minute

// RecoveryState is the durable coordinator phase.
type RecoveryState string

const (
	RecoveryIdle      RecoveryState = "idle"
	RecoveryArmed     RecoveryState = "armed"
	RecoveryActive    RecoveryState = "active"
	RecoveryRestoring RecoveryState = "restoring"
	RecoveryFailed    RecoveryState = "failed"

	pendingActivationAudit = "activated"
	pendingExpiryAudit     = "expired"
	pendingRestoreAudit    = "restore_requested"
	pendingRestoredAudit   = "restored"
	pendingCancelAudit     = "cancelled"
	pendingActivateFailed  = "activation_failed"
	pendingExtendFailed    = "extension_failed"
	pendingRestoreFailed   = "restoration_failed"
)

// RecoveryRecord contains only stable identities and bounded recovery state.
// It deliberately carries no addresses, routes, profile bodies, or secrets.
type RecoveryRecord struct {
	Version            int
	State              RecoveryState
	ArmedAt            time.Time
	ExpiresAt          time.Time
	ConnectedAtArm     []string
	AdapterID          string
	Interface          string
	HelperRecordID     string
	HardwareGeneration uint64
	PendingAudits      []string
	Detail             string
	UpdatedAt          time.Time
}

// RecoveryArmEvent is the durable root-helper observation made when the
// recovery-duration button press is released. Core must not resample either
// time or already-connected links later while polling the event.
type RecoveryArmEvent struct {
	Generation         uint64
	ReleasedAt         time.Time
	ConnectedAtRelease []string
	SnapshotValid      bool
}

// HardwareRecoveryDriver owns the privileged, exactly reversible link effect.
type HardwareRecoveryDriver interface {
	ActivateRecoveryLink(context.Context, string, string, time.Time) (string, error)
	ExtendRecoveryLink(context.Context, string, time.Time) error
	RestoreRecoveryLink(context.Context, string) error
}

// HardwareRecoveryStore persists the singleton coordinator record.
type HardwareRecoveryStore interface {
	LoadHardwareRecovery(context.Context) (RecoveryRecord, bool, error)
	SaveHardwareRecovery(context.Context, RecoveryRecord) error
}

// HardwareRecoveryOptions composes the policy coordinator.
type HardwareRecoveryOptions struct {
	Provider    Provider
	Driver      HardwareRecoveryDriver
	Store       HardwareRecoveryStore
	Coordinator *AdapterRoleCoordinator
	Now         func() time.Time
	SteadyNow   func() time.Time
	Window      time.Duration
	Audit       AuditJournal
	SubjectID   string
}

// HardwareRecoveryService serializes button arming, safe link selection, and
// expiry. The helper repeats the safety checks before changing the host.
type HardwareRecoveryService struct {
	mu          sync.Mutex
	provider    Provider
	driver      HardwareRecoveryDriver
	store       HardwareRecoveryStore
	coordinator *AdapterRoleCoordinator
	now         func() time.Time
	steadyNow   func() time.Time
	steadyUntil time.Time
	steadySeen  time.Time
	wallSeen    time.Time
	window      time.Duration
	record      RecoveryRecord
	audit       AuditJournal
	subjectID   string
}

// Current returns a copy of the durable recovery phase.
func (service *HardwareRecoveryService) Current() RecoveryRecord {
	service.mu.Lock()
	defer service.mu.Unlock()
	record := service.record
	record.ConnectedAtArm = append([]string(nil), record.ConnectedAtArm...)
	record.PendingAudits = append([]string(nil), record.PendingAudits...)
	return record
}

// HardwareEventHandled reports whether this durable generation has already
// changed core state. It lets the runtime acknowledge a replay without
// repeating a reset after an earlier acknowledgement failure.
func (service *HardwareRecoveryService) HardwareEventHandled(generation uint64) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	return generation != 0 && generation <= service.record.HardwareGeneration
}

// AdoptHardwareGeneration updates the in-memory view after another repository
// transaction has atomically persisted a reset tombstone for this generation.
func (service *HardwareRecoveryService) AdoptHardwareGeneration(generation uint64) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if generation > service.record.HardwareGeneration {
		service.record.HardwareGeneration = generation
	}
}

// RefuseHardwareEvent durably consumes a physical event that cannot act in the
// current lifecycle. Persistence precedes helper acknowledgement so a replay
// cannot become actionable after commissioning changes.
func (service *HardwareRecoveryService) RefuseHardwareEvent(ctx context.Context, generation uint64, reason string) (RecoveryRecord, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if generation == 0 {
		return service.record, errors.New("hardware recovery generation is required")
	}
	if generation <= service.record.HardwareGeneration {
		return service.record, nil
	}
	if service.record.State != RecoveryIdle {
		return service.record, errors.New("hardware recovery event cannot be refused while recovery is non-idle")
	}
	return service.refuseHardwareEventLocked(ctx, generation, reason)
}

// Cancel removes any temporary recovery effect before a network-only return to
// commissioning. It never changes the commissioning lifecycle itself.
func (service *HardwareRecoveryService) Cancel(ctx context.Context) (RecoveryRecord, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	pendingAuditErr := service.flushPendingAuditsLocked(ctx)
	if pendingAuditErr != nil && hasPendingAudit(service.record, pendingRestoredAudit) {
		return service.record, pendingAuditErr
	}
	now := service.now().UTC()
	switch service.record.State {
	case RecoveryIdle:
		return service.record, pendingAuditErr
	case RecoveryArmed:
		next := idleRecoveryRecord(now, service.record.HardwareGeneration)
		appendPendingAudit(&next, pendingCancelAudit)
		if err := service.saveRecord(ctx, next); err != nil {
			return service.record, err
		}
		service.record = next
		cancelAuditErr := service.flushPendingAuditsLocked(ctx)
		return service.record, errors.Join(pendingAuditErr, cancelAuditErr)
	default:
		record, restoreErr := service.restore(ctx, now)
		return record, errors.Join(pendingAuditErr, restoreErr)
	}
}

// NewHardwareRecoveryService loads the recovery interlock without changing a
// link. Reconcile performs any needed restore before selecting new work.
func NewHardwareRecoveryService(options HardwareRecoveryOptions) (*HardwareRecoveryService, error) {
	if options.Provider == nil || options.Driver == nil {
		return nil, errors.New("hardware recovery requires a provider and driver")
	}
	if options.Coordinator == nil {
		options.Coordinator = NewAdapterRoleCoordinator()
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.SteadyNow == nil {
		options.SteadyNow = time.Now
	}
	if options.Window <= 0 {
		options.Window = RecoveryWindow
	}
	record := RecoveryRecord{Version: 1, State: RecoveryIdle}
	if options.Store != nil {
		loaded, found, err := options.Store.LoadHardwareRecovery(context.Background())
		if err != nil {
			return nil, fmt.Errorf("load hardware recovery: %w", err)
		}
		if found {
			record = loaded
		}
	}
	service := &HardwareRecoveryService{
		provider: options.Provider, driver: options.Driver, store: options.Store,
		coordinator: options.Coordinator, now: options.Now, steadyNow: options.SteadyNow,
		window: options.Window, record: record,
		audit: options.Audit, subjectID: options.SubjectID,
	}
	if record.State == RecoveryArmed || record.State == RecoveryActive {
		service.trackDeadline(options.Now().UTC(), record.ExpiresAt)
	}
	if record.AdapterID != "" && record.State != RecoveryIdle {
		if err := service.reserve(record.AdapterID, record.Interface); err != nil {
			return nil, fmt.Errorf("restore hardware recovery interlock: %w", err)
		}
	}
	return service, nil
}

// Arm starts or extends the recovery window. Extending never resamples link
// state and never switches an active interface.
func (service *HardwareRecoveryService) Arm(ctx context.Context) (RecoveryRecord, error) {
	return service.arm(ctx, RecoveryArmEvent{})
}

// ArmEvent makes a durable hardware generation idempotent across a core crash
// between applying the event and acknowledging it to the helper.
func (service *HardwareRecoveryService) ArmEvent(ctx context.Context, generation uint64) (RecoveryRecord, error) {
	if generation == 0 {
		return service.Current(), errors.New("hardware recovery generation is required")
	}
	return service.arm(ctx, RecoveryArmEvent{Generation: generation})
}

// ArmHardwareEvent uses the release-time deadline and link snapshot captured
// by the privileged helper. A future release after clock rollback is consumed
// and refused rather than allowed to create a window longer than ten minutes.
func (service *HardwareRecoveryService) ArmHardwareEvent(ctx context.Context, event RecoveryArmEvent) (RecoveryRecord, error) {
	if event.Generation == 0 || event.ReleasedAt.IsZero() {
		return service.Current(), errors.New("complete hardware recovery event is required")
	}
	previous := ""
	for index, adapterID := range event.ConnectedAtRelease {
		canonical := canonicalAdapterID(adapterID)
		if canonical == "" || canonical != adapterID || index > 0 && canonical <= previous {
			return service.Current(), errors.New("invalid hardware recovery link snapshot")
		}
		previous = canonical
	}
	return service.arm(ctx, event)
}

func (service *HardwareRecoveryService) arm(ctx context.Context, event RecoveryArmEvent) (RecoveryRecord, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	generation := event.Generation
	if generation != 0 && generation <= service.record.HardwareGeneration {
		return service.record, nil
	}
	now := service.now().UTC()
	armedAt := now
	expiresAt := now.Add(service.window)
	connected := []string(nil)
	connectedCandidates := make([]Interface, 0)
	if !event.ReleasedAt.IsZero() {
		armedAt = event.ReleasedAt.UTC()
		if armedAt.After(now) {
			return service.refuseHardwareEventLocked(ctx, generation, "release_time_in_future")
		}
		expiresAt = armedAt.Add(service.window)
		if !event.SnapshotValid {
			if err := service.journal(ctx, "network.recovery.refused", audit.OutcomeDenied, "link_snapshot_unavailable", nil); err != nil {
				return service.record, err
			}
			next := service.record
			next.HardwareGeneration = generation
			next.UpdatedAt = now
			if err := service.saveRecord(ctx, next); err != nil {
				return service.record, err
			}
			service.record = next
			return service.record, nil
		}
		connected = append([]string(nil), event.ConnectedAtRelease...)
		for _, adapterID := range connected {
			connectedCandidates = append(connectedCandidates, Interface{AdapterID: adapterID})
		}
	}
	if !expiresAt.After(now) {
		if err := service.journal(ctx, "network.recovery.expired", audit.OutcomeDenied, "event_delivery_expired", nil); err != nil {
			return service.record, err
		}
		next := service.record
		next.HardwareGeneration = generation
		next.UpdatedAt = now
		if err := service.saveRecord(ctx, next); err != nil {
			return service.record, err
		}
		service.record = next
		return service.record, nil
	}
	if service.record.State == RecoveryActive || service.record.State == RecoveryArmed {
		if service.deadlineElapsed(now) {
			return service.record, errors.New("hardware recovery deadline elapsed; reconcile restoration first")
		}
		if err := service.journal(ctx, "network.recovery.extended", audit.OutcomeRequested, "window_extended", nil); err != nil {
			return service.record, err
		}
		if service.record.State == RecoveryActive {
			if err := service.driver.ExtendRecoveryLink(ctx, service.record.HelperRecordID, expiresAt); err != nil {
				auditErr := service.recordOrQueueAuditLocked(
					context.WithoutCancel(ctx), pendingExtendFailed, "network.recovery.failed",
					audit.OutcomeFailed, "helper_extension_failed", nil,
				)
				return service.record, errors.Join(errors.New("extend recovery link failed"), auditErr)
			}
		}
		next := service.record
		next.ExpiresAt = expiresAt
		if generation != 0 {
			next.HardwareGeneration = generation
		}
		next.UpdatedAt = now
		if err := service.saveRecord(ctx, next); err != nil {
			return service.record, err
		}
		service.record = next
		service.trackDeadline(now, expiresAt)
		return service.record, nil
	}
	if service.record.State == RecoveryFailed || service.record.State == RecoveryRestoring {
		return service.record, errors.New("hardware recovery restoration is incomplete")
	}
	if event.ReleasedAt.IsZero() {
		interfaces, err := service.provider.Interfaces(ctx)
		if err != nil {
			return service.record, fmt.Errorf("observe interfaces while arming recovery: %w", err)
		}
		for _, candidate := range interfaces {
			if recoveryLinkUp(candidate) && canonicalAdapterID(candidate.AdapterID) != "" {
				connected = append(connected, canonicalAdapterID(candidate.AdapterID))
				connectedCandidates = append(connectedCandidates, candidate)
			}
		}
	}
	sort.Strings(connected)
	sort.Slice(connectedCandidates, func(first, second int) bool {
		return canonicalAdapterID(connectedCandidates[first].AdapterID) < canonicalAdapterID(connectedCandidates[second].AdapterID)
	})
	if err := service.journal(ctx, "network.recovery.armed", audit.OutcomeRequested, "hardware_request", nil); err != nil {
		return service.record, err
	}
	for index := range connectedCandidates {
		if err := service.journal(ctx, "network.recovery.refused", audit.OutcomeDenied, "already_connected_when_armed", &connectedCandidates[index]); err != nil {
			return service.record, err
		}
	}
	next := RecoveryRecord{
		Version: 1, State: RecoveryArmed, ArmedAt: armedAt, ExpiresAt: expiresAt,
		ConnectedAtArm: connected, HardwareGeneration: generation, UpdatedAt: now,
	}
	if err := service.saveRecord(ctx, next); err != nil {
		return service.record, err
	}
	service.record = next
	service.trackDeadline(now, expiresAt)
	return service.record, nil
}

// Reconcile expires owned effects first, then permits the first deterministic
// eligible adapter which transitioned to connected after arming.
func (service *HardwareRecoveryService) Reconcile(ctx context.Context) (RecoveryRecord, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.now().UTC()
	deadlineElapsed := service.deadlineElapsed(now)
	pendingAuditErr := service.flushPendingAuditsLocked(ctx)
	if pendingAuditErr != nil {
		if hasPendingAudit(service.record, pendingRestoredAudit) ||
			service.record.State == RecoveryIdle || service.record.State == RecoveryArmed ||
			service.record.State == RecoveryActive && !deadlineElapsed {
			return service.record, pendingAuditErr
		}
	}
	if !deadlineElapsed && (service.record.State == RecoveryArmed || service.record.State == RecoveryActive) &&
		now.After(service.record.UpdatedAt) {
		next := service.record
		next.UpdatedAt = now
		if err := service.saveRecord(ctx, next); err != nil {
			return service.record, err
		}
		service.record = next
	}
	if service.record.State == RecoveryFailed || service.record.State == RecoveryRestoring ||
		(service.record.State == RecoveryActive && deadlineElapsed) {
		var expiryAuditErr error
		if service.record.State == RecoveryActive {
			expiryAuditErr = service.recordOrQueueAuditLocked(
				ctx, pendingExpiryAudit, "network.recovery.expired", audit.OutcomeRequested, "deadline_elapsed", nil,
			)
		}
		record, restoreErr := service.restore(ctx, now)
		return record, errors.Join(pendingAuditErr, expiryAuditErr, restoreErr)
	}
	if service.record.State != RecoveryArmed {
		return service.record, pendingAuditErr
	}
	if deadlineElapsed {
		if err := service.journal(ctx, "network.recovery.expired", audit.OutcomeRequested, "armed_window_elapsed", nil); err != nil {
			return service.record, err
		}
		service.record = idleRecoveryRecord(now, service.record.HardwareGeneration)
		return service.record, service.save(ctx)
	}
	interfaces, err := service.provider.Interfaces(ctx)
	if err != nil {
		return service.record, fmt.Errorf("observe recovery links: %w", err)
	}
	sort.Slice(interfaces, func(first, second int) bool {
		return canonicalAdapterID(interfaces[first].AdapterID) < canonicalAdapterID(interfaces[second].AdapterID)
	})
	connectedAtArm := make(map[string]bool, len(service.record.ConnectedAtArm))
	for _, adapterID := range service.record.ConnectedAtArm {
		connectedAtArm[canonicalAdapterID(adapterID)] = true
	}
	for _, candidate := range interfaces {
		adapterID := canonicalAdapterID(candidate.AdapterID)
		if !recoveryLinkUp(candidate) || adapterID == "" || connectedAtArm[adapterID] {
			continue
		}
		reason, eligibilityErr := recoveryEligibility(candidate, service.coordinator)
		if eligibilityErr != nil {
			if err := service.journal(ctx, "network.recovery.refused", audit.OutcomeDenied, reason, &candidate); err != nil {
				return service.record, err
			}
			service.record.ConnectedAtArm = append(service.record.ConnectedAtArm, adapterID)
			sort.Strings(service.record.ConnectedAtArm)
			service.record.UpdatedAt = now
			if err := service.save(ctx); err != nil {
				return service.record, err
			}
			continue
		}
		if err := service.reserve(adapterID, candidate.Name); err != nil {
			if auditErr := service.journal(ctx, "network.recovery.refused", audit.OutcomeDenied, "role_conflict", &candidate); auditErr != nil {
				return service.record, auditErr
			}
			service.record.ConnectedAtArm = append(service.record.ConnectedAtArm, adapterID)
			sort.Strings(service.record.ConnectedAtArm)
			service.record.UpdatedAt = now
			if saveErr := service.save(ctx); saveErr != nil {
				return service.record, saveErr
			}
			continue
		}
		if err := service.journal(ctx, "network.recovery.activation_requested", audit.OutcomeRequested, "eligible_link_transition", &candidate); err != nil {
			service.coordinator.Release("hardware-recovery")
			return service.record, err
		}
		recordID, err := service.driver.ActivateRecoveryLink(ctx, adapterID, candidate.Name, service.record.ExpiresAt)
		if err != nil {
			service.record.State = RecoveryFailed
			service.record.AdapterID = adapterID
			service.record.Interface = candidate.Name
			service.record.HelperRecordID = recordID
			service.record.Detail = "activation failed; recovery interlock retained"
			service.record.UpdatedAt = now
			appendPendingAudit(&service.record, pendingActivateFailed)
			saveErr := service.save(context.WithoutCancel(ctx))
			auditErr := service.flushPendingAuditsLocked(context.WithoutCancel(ctx))
			return service.record, errors.Join(errors.New("activate recovery link failed"), saveErr, auditErr)
		}
		service.record.State = RecoveryActive
		service.record.AdapterID = adapterID
		service.record.Interface = candidate.Name
		service.record.HelperRecordID = recordID
		appendPendingAudit(&service.record, pendingActivationAudit)
		service.record.Detail = ""
		service.record.UpdatedAt = now
		if err := service.save(ctx); err != nil {
			record, restoreErr := service.restore(context.WithoutCancel(ctx), now)
			return record, errors.Join(err, restoreErr)
		}
		return service.record, service.flushPendingAuditsLocked(ctx)
	}
	return service.record, nil
}

// RecoveryInterfaceEligible applies the core-side refusal matrix. The helper
// must additionally refuse active/available foreign profiles and DHCP leases.
func RecoveryInterfaceEligible(candidate Interface, coordinator *AdapterRoleCoordinator) error {
	_, err := recoveryEligibility(candidate, coordinator)
	return err
}

func recoveryEligibility(candidate Interface, coordinator *AdapterRoleCoordinator) (string, error) {
	if candidate.Kind != KindEthernet || !candidate.Physical || canonicalAdapterID(candidate.AdapterID) == "" {
		return "not_identified_physical_ethernet", errors.New("recovery requires an identified physical Ethernet adapter")
	}
	if !recoveryLinkUp(candidate) {
		return "link_not_connected", errors.New("recovery requires a newly connected link")
	}
	if candidate.Settings != nil && candidate.Settings.Saved {
		return "saved_configuration", errors.New("adapter has saved production configuration")
	}
	if len(candidate.IPv4) > 0 || hasNonLinkLocalPrefixes(candidate.IPv6) || candidate.Gateway4 != "" || candidate.Gateway6 != "" {
		return "existing_network_configuration", errors.New("adapter already has network configuration")
	}
	if coordinator != nil {
		if err := coordinator.Check([]AdapterRoleClaim{{
			AdapterID: candidate.AdapterID, Interface: candidate.Name, Role: AdapterRoleRecovery,
		}}); err != nil {
			return "protected_role", err
		}
	}
	return "", nil
}

func recoveryLinkUp(candidate Interface) bool {
	return candidate.LinkUp || candidate.State == StateConnected
}

func (service *HardwareRecoveryService) restore(ctx context.Context, now time.Time) (RecoveryRecord, error) {
	auditErr := service.recordOrQueueAuditLocked(
		ctx, pendingRestoreAudit, "network.recovery.restore_requested", audit.OutcomeRequested, "restore_temporary_link", nil,
	)
	service.record.State = RecoveryRestoring
	service.record.UpdatedAt = now
	if err := service.save(context.WithoutCancel(ctx)); err != nil {
		return service.record, err
	}
	if service.record.HelperRecordID != "" {
		if err := service.driver.RestoreRecoveryLink(ctx, service.record.HelperRecordID); err != nil {
			service.record.State = RecoveryFailed
			service.record.Detail = "restoration failed; recovery interlock retained"
			service.record.UpdatedAt = now
			appendPendingAudit(&service.record, pendingRestoreFailed)
			saveErr := service.save(context.WithoutCancel(ctx))
			failureAuditErr := service.flushPendingAuditsLocked(context.WithoutCancel(ctx))
			return service.record, errors.Join(errors.New("restore recovery link failed"), saveErr, failureAuditErr)
		}
	}
	appendPendingAudit(&service.record, pendingRestoredAudit)
	service.record.Detail = ""
	service.record.UpdatedAt = now
	if err := service.save(ctx); err != nil {
		return service.record, err
	}
	pendingAuditErr := service.flushPendingAuditsLocked(ctx)
	return service.record, errors.Join(auditErr, pendingAuditErr)
}

func (service *HardwareRecoveryService) refuseHardwareEventLocked(
	ctx context.Context, generation uint64, reason string,
) (RecoveryRecord, error) {
	if err := service.journal(ctx, "network.recovery.refused", audit.OutcomeDenied, reason, nil); err != nil {
		return service.record, err
	}
	next := service.record
	next.HardwareGeneration = generation
	next.UpdatedAt = service.now().UTC()
	if service.store != nil {
		if err := service.store.SaveHardwareRecovery(ctx, next); err != nil {
			return service.record, fmt.Errorf("save hardware recovery: %w", err)
		}
	}
	service.record = next
	return service.record, nil
}

func (service *HardwareRecoveryService) flushPendingAuditsLocked(ctx context.Context) error {
	for len(service.record.PendingAudits) > 0 {
		pending := service.record.PendingAudits[0]
		action, outcome, reason := "", audit.Outcome(""), ""
		switch pending {
		case pendingActivationAudit:
			action, outcome, reason = "network.recovery.activated", audit.OutcomeApplied, "temporary_link_active"
		case pendingExpiryAudit:
			action, outcome, reason = "network.recovery.expired", audit.OutcomeRequested, "deadline_elapsed"
		case pendingRestoreAudit:
			action, outcome, reason = "network.recovery.restore_requested", audit.OutcomeRequested, "restore_temporary_link"
		case pendingRestoredAudit:
			action, outcome, reason = "network.recovery.restored", audit.OutcomeRecovered, "temporary_link_removed"
		case pendingCancelAudit:
			action, outcome, reason = "network.recovery.restored", audit.OutcomeRecovered, "commissioning_reset_cancelled_window"
		case pendingActivateFailed:
			action, outcome, reason = "network.recovery.failed", audit.OutcomeFailed, "helper_activation_failed"
		case pendingExtendFailed:
			action, outcome, reason = "network.recovery.failed", audit.OutcomeFailed, "helper_extension_failed"
		case pendingRestoreFailed:
			action, outcome, reason = "network.recovery.failed", audit.OutcomeFailed, "helper_restoration_failed"
		default:
			return errors.New("unknown pending recovery audit")
		}
		if err := service.journal(ctx, action, outcome, reason, nil); err != nil {
			return err
		}
		next := service.record
		next.PendingAudits = append([]string(nil), service.record.PendingAudits[1:]...)
		next.UpdatedAt = service.now().UTC()
		if pending == pendingRestoredAudit {
			next = idleRecoveryRecord(next.UpdatedAt, service.record.HardwareGeneration)
		}
		if service.store != nil {
			if err := service.store.SaveHardwareRecovery(ctx, next); err != nil {
				return fmt.Errorf("save hardware recovery: %w", err)
			}
		}
		if pending == pendingRestoredAudit {
			service.coordinator.Release("hardware-recovery")
		}
		service.record = next
	}
	return nil
}

func (service *HardwareRecoveryService) recordOrQueueAuditLocked(
	ctx context.Context, pending, action string, outcome audit.Outcome, reason string, candidate *Interface,
) error {
	auditErr := service.journal(ctx, action, outcome, reason, candidate)
	if auditErr == nil {
		return nil
	}
	appendPendingAudit(&service.record, pending)
	observedAt := service.now().UTC()
	if observedAt.After(service.record.UpdatedAt) {
		service.record.UpdatedAt = observedAt
	}
	return errors.Join(auditErr, service.save(context.WithoutCancel(ctx)))
}

func appendPendingAudit(record *RecoveryRecord, pending string) {
	if !hasPendingAudit(*record, pending) {
		record.PendingAudits = append(record.PendingAudits, pending)
	}
}

func hasPendingAudit(record RecoveryRecord, pending string) bool {
	for _, candidate := range record.PendingAudits {
		if candidate == pending {
			return true
		}
	}
	return false
}

func idleRecoveryRecord(now time.Time, generation uint64) RecoveryRecord {
	return RecoveryRecord{Version: 1, State: RecoveryIdle, HardwareGeneration: generation, UpdatedAt: now}
}

func (service *HardwareRecoveryService) reserve(adapterID, interfaceName string) error {
	return service.coordinator.Reserve("hardware-recovery", []AdapterRoleClaim{{
		AdapterID: adapterID, Interface: interfaceName, Role: AdapterRoleRecovery,
		ResourceID: "hardware-recovery", ResourceName: "temporary network recovery",
	}})
}

func (service *HardwareRecoveryService) trackDeadline(wallNow, expiresAt time.Time) {
	steadyNow := service.steadyNow()
	remaining := expiresAt.Sub(wallNow)
	if !service.record.UpdatedAt.IsZero() && wallNow.Before(service.record.UpdatedAt) {
		remaining = 0
	}
	if remaining < 0 {
		remaining = 0
	}
	service.steadyUntil = steadyNow.Add(remaining)
	service.steadySeen = steadyNow
	service.wallSeen = wallNow
	if service.record.UpdatedAt.After(service.wallSeen) {
		service.wallSeen = service.record.UpdatedAt
	}
}

func (service *HardwareRecoveryService) deadlineElapsed(wallNow time.Time) bool {
	if service.record.State != RecoveryArmed && service.record.State != RecoveryActive {
		return false
	}
	steadyNow := service.steadyNow()
	rollback := !service.wallSeen.IsZero() && wallNow.Before(service.wallSeen)
	steadyRollback := !service.steadySeen.IsZero() && steadyNow.Before(service.steadySeen)
	if wallNow.After(service.wallSeen) {
		service.wallSeen = wallNow
	}
	if steadyNow.After(service.steadySeen) {
		service.steadySeen = steadyNow
	}
	return rollback || steadyRollback || !wallNow.Before(service.record.ExpiresAt) ||
		service.steadyUntil.IsZero() || !steadyNow.Before(service.steadyUntil)
}

func (service *HardwareRecoveryService) save(ctx context.Context) error {
	return service.saveRecord(ctx, service.record)
}

func (service *HardwareRecoveryService) saveRecord(ctx context.Context, record RecoveryRecord) error {
	if service.store == nil {
		return nil
	}
	if err := service.store.SaveHardwareRecovery(ctx, record); err != nil {
		return fmt.Errorf("save hardware recovery: %w", err)
	}
	return nil
}

func (service *HardwareRecoveryService) journal(
	ctx context.Context, action string, outcome audit.Outcome, reason string, candidate *Interface,
) error {
	if service.audit == nil {
		return nil
	}
	details := map[string]string{"reason": reason}
	if candidate != nil {
		details["adapter_id"] = canonicalAdapterID(candidate.AdapterID)
		if candidate.Name != "" {
			details["interface"] = candidate.Name
		}
	} else if service.record.AdapterID != "" {
		details["adapter_id"] = canonicalAdapterID(service.record.AdapterID)
		details["interface"] = service.record.Interface
	}
	severity := audit.SeverityInfo
	if outcome == audit.OutcomeDenied {
		severity = audit.SeverityWarning
	}
	if outcome == audit.OutcomeFailed {
		severity = audit.SeverityError
	}
	return service.audit.Record(ctx, audit.Record{
		Category: audit.CategoryAudit, Component: "network.recovery", Action: action,
		SubjectType: "appliance", SubjectID: service.subjectID, ActorKind: audit.ActorKindSystem,
		Outcome: outcome, Severity: severity, Details: details,
	})
}
