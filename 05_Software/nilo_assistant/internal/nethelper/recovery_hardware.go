package nethelper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/network"
)

// HardwareRecoveryEvent is one button release retained until acknowledged.
type HardwareRecoveryEvent struct {
	ID                  string                 `json:"id"`
	Generation          uint64                 `json:"generation"`
	Action              HardwareRecoveryAction `json:"action"`
	ReleasedAt          time.Time              `json:"released_at"`
	ConnectedAdapterIDs []string               `json:"connected_adapter_ids"`
	SnapshotValid       bool                   `json:"snapshot_valid"`
}

// HardwareRecoveryEventRecord retains the last assigned generation and at most
// one unacknowledged physical action in the helper's root-owned state.
type HardwareRecoveryEventRecord struct {
	Version            int                         `json:"version"`
	Generation         uint64                      `json:"generation"`
	Pending            HardwareRecoveryEvent       `json:"pending"`
	IdentifyOperations []HardwareIdentifyOperation `json:"identify_operations,omitempty"`
}

// HardwareIdentifyOperation is the durable operation token and absolute
// indicator deadline owned by the Raspberry Pi hardware boundary.
type HardwareIdentifyOperation struct {
	ID           string    `json:"id"`
	ActiveUntil  time.Time `json:"active_until"`
	Acknowledged bool      `json:"acknowledged,omitempty"`
}

type HardwareRecoveryEventStore interface {
	LoadHardwareRecoveryEvent() (HardwareRecoveryEventRecord, error)
	SaveHardwareRecoveryEvent(HardwareRecoveryEventRecord) error
}

type recoveryEventQueue struct {
	mu     sync.Mutex
	store  HardwareRecoveryEventStore
	record HardwareRecoveryEventRecord
}

func newRecoveryEventQueue(store HardwareRecoveryEventStore) (*recoveryEventQueue, error) {
	if store == nil {
		return nil, errors.New("hardware recovery event store is required")
	}
	record, err := store.LoadHardwareRecoveryEvent()
	if err != nil {
		return nil, fmt.Errorf("load hardware recovery event: %w", err)
	}
	if record.Version == 0 {
		record.Version = 1
	}
	pending := record.Pending
	pendingEmpty := pending.ID == "" && pending.Generation == 0 && pending.Action == "" &&
		pending.ReleasedAt.IsZero() && len(pending.ConnectedAdapterIDs) == 0 && !pending.SnapshotValid
	pendingValid := pending.ID != "" && pending.Generation != 0 && pending.Generation == record.Generation &&
		!pending.ReleasedAt.IsZero() && validHardwareSnapshot(pending) &&
		(pending.Action == HardwareRecoveryIdentify || pending.Action == HardwareRecoveryArm || pending.Action == HardwareRecoveryResetCommissioning)
	identifyValid := len(record.IdentifyOperations) <= identifyActionLimit
	seenIdentify := make(map[string]bool, len(record.IdentifyOperations))
	for _, operation := range record.IdentifyOperations {
		if !validID.MatchString(operation.ID) || operation.ActiveUntil.IsZero() || seenIdentify[operation.ID] {
			identifyValid = false
			break
		}
		seenIdentify[operation.ID] = true
	}
	if record.Version != 1 || !pendingEmpty && !pendingValid || !identifyValid {
		return nil, errors.New("invalid hardware recovery event record")
	}
	return &recoveryEventQueue{store: store, record: record}, nil
}

func (queue *recoveryEventQueue) SetIdentifyOperation(
	operationID string, activeUntil time.Time,
) (time.Time, bool, error) {
	if !validID.MatchString(operationID) || activeUntil.IsZero() {
		return time.Time{}, false, errors.New("invalid identify indicator operation")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for _, operation := range queue.record.IdentifyOperations {
		if operation.ID != operationID {
			continue
		}
		// The durable token proves this effect already started. Its original
		// absolute deadline is authoritative even when a delayed caller offers
		// a refreshed deadline while recovering the receipt.
		return operation.ActiveUntil, false, nil
	}
	if len(queue.record.IdentifyOperations) >= identifyActionLimit {
		return time.Time{}, false, errors.New("identify indicator operation ledger is full")
	}
	next := queue.record
	next.IdentifyOperations = append(append([]HardwareIdentifyOperation(nil), next.IdentifyOperations...), HardwareIdentifyOperation{
		ID: operationID, ActiveUntil: activeUntil.UTC(),
	})
	if err := queue.store.SaveHardwareRecoveryEvent(next); err != nil {
		return time.Time{}, false, fmt.Errorf("save identify indicator operation: %w", err)
	}
	queue.record = next
	return activeUntil.UTC(), true, nil
}

func (queue *recoveryEventQueue) AcknowledgeIdentifyOperation(operationID string, now time.Time) error {
	if !validID.MatchString(operationID) || now.IsZero() {
		return errors.New("invalid identify indicator acknowledgement")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	index := -1
	for candidate := range queue.record.IdentifyOperations {
		if queue.record.IdentifyOperations[candidate].ID == operationID {
			index = candidate
			break
		}
	}
	if index == -1 {
		return nil
	}
	next := queue.record
	next.IdentifyOperations = append([]HardwareIdentifyOperation(nil), next.IdentifyOperations...)
	if next.IdentifyOperations[index].ActiveUntil.After(now) {
		if next.IdentifyOperations[index].Acknowledged {
			return nil
		}
		next.IdentifyOperations[index].Acknowledged = true
	} else {
		next.IdentifyOperations = append(next.IdentifyOperations[:index], next.IdentifyOperations[index+1:]...)
	}
	if err := queue.store.SaveHardwareRecoveryEvent(next); err != nil {
		return fmt.Errorf("acknowledge identify indicator operation: %w", err)
	}
	queue.record = next
	return nil
}

// ExpireIdentifyOperations removes only acknowledged tokens whose physical
// interval has ended. Unacknowledged tokens remain as idempotency evidence for
// a Core receipt retry, but no expired token keeps the indicator active.
func (queue *recoveryEventQueue) ExpireIdentifyOperations(now time.Time) error {
	if now.IsZero() {
		return errors.New("identify indicator expiry time is required")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	next := queue.record
	next.IdentifyOperations = make([]HardwareIdentifyOperation, 0, len(queue.record.IdentifyOperations))
	for _, operation := range queue.record.IdentifyOperations {
		if operation.Acknowledged && !operation.ActiveUntil.After(now) {
			continue
		}
		next.IdentifyOperations = append(next.IdentifyOperations, operation)
	}
	if len(next.IdentifyOperations) == len(queue.record.IdentifyOperations) {
		return nil
	}
	if err := queue.store.SaveHardwareRecoveryEvent(next); err != nil {
		return fmt.Errorf("expire identify indicator operation: %w", err)
	}
	queue.record = next
	return nil
}

func (queue *recoveryEventQueue) ActiveIdentifyDeadline(now time.Time) time.Time {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	var latest time.Time
	for _, operation := range queue.record.IdentifyOperations {
		if operation.ActiveUntil.After(now) && operation.ActiveUntil.After(latest) {
			latest = operation.ActiveUntil
		}
	}
	return latest
}

func (queue *recoveryEventQueue) Publish(action HardwareRecoveryAction) error {
	return queue.PublishEvent(HardwareRecoveryEvent{Action: action, ReleasedAt: time.Now().UTC(), SnapshotValid: true})
}

// PublishEvent durably coalesces repeated recovery requests and lets a reset
// supersede recovery. A pending reset is never weakened by a later short press.
func (queue *recoveryEventQueue) PublishEvent(event HardwareRecoveryEvent) error {
	action := event.Action
	if action != HardwareRecoveryIdentify && action != HardwareRecoveryArm && action != HardwareRecoveryResetCommissioning {
		return errors.New("invalid hardware recovery action")
	}
	if event.ReleasedAt.IsZero() || !validHardwareSnapshot(event) {
		return errors.New("invalid hardware recovery event")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if hardwareActionPriority(action) < hardwareActionPriority(queue.record.Pending.Action) {
		return nil
	}
	if queue.record.Generation >= math.MaxInt64 {
		return errors.New("hardware recovery event generation is exhausted")
	}
	next := queue.record
	next.Generation++
	event.ID = fmt.Sprintf("gpio-%020d", next.Generation)
	event.Generation = next.Generation
	event.ReleasedAt = event.ReleasedAt.UTC()
	event.ConnectedAdapterIDs = append([]string(nil), event.ConnectedAdapterIDs...)
	next.Pending = event
	if err := queue.store.SaveHardwareRecoveryEvent(next); err != nil {
		return fmt.Errorf("save hardware recovery event: %w", err)
	}
	queue.record = next
	return nil
}

func hardwareActionPriority(action HardwareRecoveryAction) int {
	switch action {
	case HardwareRecoveryIdentify:
		return 1
	case HardwareRecoveryArm:
		return 2
	case HardwareRecoveryResetCommissioning:
		return 3
	default:
		return 0
	}
}

func (queue *recoveryEventQueue) Read(context.Context) (HardwareRecoveryEvent, bool, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	event := queue.record.Pending
	event.ConnectedAdapterIDs = append([]string(nil), event.ConnectedAdapterIDs...)
	return event, event.ID != "", nil
}

func validHardwareSnapshot(event HardwareRecoveryEvent) bool {
	if event.Action == HardwareRecoveryResetCommissioning && (!event.SnapshotValid || len(event.ConnectedAdapterIDs) != 0) {
		return false
	}
	previous := ""
	for _, adapterID := range event.ConnectedAdapterIDs {
		if adapterID == "" || len(adapterID) > 128 || adapterID <= previous || strings.TrimSpace(adapterID) != adapterID {
			return false
		}
		previous = adapterID
	}
	return true
}

func recoveryLinkSnapshot(devices []Device) ([]string, bool) {
	connected := make([]string, 0)
	valid := true
	for _, device := range devices {
		observed := device.Interface
		if observed.Kind != network.KindEthernet || !observed.Physical ||
			!observed.LinkUp && observed.State != network.StateConnected {
			continue
		}
		adapterID := strings.ToLower(strings.TrimSpace(observed.AdapterID))
		if adapterID == "" {
			valid = false
			continue
		}
		connected = append(connected, adapterID)
	}
	sort.Strings(connected)
	connected = slicesCompact(connected)
	return connected, valid
}

func slicesCompact(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func (queue *recoveryEventQueue) Acknowledge(_ context.Context, eventID string) error {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.record.Pending.ID == "" || queue.record.Pending.ID != eventID {
		return errors.New("hardware recovery event is no longer pending")
	}
	next := queue.record
	next.Pending = HardwareRecoveryEvent{}
	if err := queue.store.SaveHardwareRecoveryEvent(next); err != nil {
		return fmt.Errorf("acknowledge hardware recovery event: %w", err)
	}
	queue.record = next
	return nil
}

// RecoveryHardware is the optional Raspberry Pi-only hardware boundary.
type RecoveryHardware interface {
	ReadRecoveryEvent(context.Context) (HardwareRecoveryEvent, bool, error)
	AcknowledgeRecoveryEvent(context.Context, string) error
	SetRecoveryIndicator(context.Context, HardwareIndicatorState) error
}

// ConfigureRecoveryHardware enables the explicitly installed Pi hardware path.
func (e *Engine) ConfigureRecoveryHardware(hardware RecoveryHardware) error {
	if hardware == nil {
		return errors.New("recovery hardware is required")
	}
	e.mu.Lock()
	e.hardware = hardware
	e.mu.Unlock()
	return nil
}

func (e *Engine) ReadRecoveryEvent(ctx context.Context) (HardwareRecoveryEvent, bool, error) {
	e.mu.Lock()
	hardware := e.hardware
	e.mu.Unlock()
	if hardware == nil {
		return HardwareRecoveryEvent{}, false, ErrDirectRecoveryUnsupported
	}
	return hardware.ReadRecoveryEvent(ctx)
}

func (e *Engine) AcknowledgeRecoveryEvent(ctx context.Context, eventID string) error {
	e.mu.Lock()
	hardware := e.hardware
	e.mu.Unlock()
	if hardware == nil {
		return ErrDirectRecoveryUnsupported
	}
	return hardware.AcknowledgeRecoveryEvent(ctx, eventID)
}

func (e *Engine) SetRecoveryIndicator(ctx context.Context, state HardwareIndicatorState) error {
	if !state.valid() {
		return errors.New("invalid recovery indicator state")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.recoveryIndicator = state
	return e.applyIndicatorLocked(ctx)
}
