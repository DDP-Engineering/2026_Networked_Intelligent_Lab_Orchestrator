package nethelper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/network"
)

const (
	IdentifyActionPending   = "pending"
	IdentifyActionCompleted = "completed"
	identifyEffectStarted   = "started"
	identifyActionLimit     = 128
)

type IdentifyActionRecord struct {
	Version       int                           `json:"version"`
	Request       network.IdentifyActionRequest `json:"request"`
	State         string                        `json:"state"`
	IndicatorDone bool                          `json:"indicator_done"`
	MDNSResult    string                        `json:"mdns_result,omitempty"`
	SSDPResult    string                        `json:"ssdp_result,omitempty"`
	Result        network.IdentifyActionResult  `json:"result"`
	UpdatedAt     time.Time                     `json:"updated_at"`
}

type IdentifyActionLedger struct {
	Version    int                    `json:"version"`
	Operations []IdentifyActionRecord `json:"operations"`
}

type IdentifyActionStore interface {
	LoadIdentifyActions() (IdentifyActionLedger, error)
	SaveIdentifyActions(IdentifyActionLedger) error
}

type identifyIndicatorHardware interface {
	SetIdentifyIndicator(context.Context, string, time.Time, HardwareIndicatorState) (time.Time, error)
	AcknowledgeIdentifyIndicator(context.Context, string) error
}

type identifyMDNSDriver interface {
	ReannounceMDNS(context.Context, string, network.MDNSAdvertisement) error
	AcknowledgeMDNSReannouncement(string)
}

type identifySSDPResponder interface {
	Reannounce(context.Context, string, network.SSDPPresence) error
	AcknowledgeReannouncement(string)
}

func validIdentifyActionRequest(request network.IdentifyActionRequest) bool {
	return validID.MatchString(request.OperationID) && !request.ActiveUntil.IsZero() &&
		(request.Source == network.IdentifySourceInterface || request.Source == network.IdentifySourceButton)
}

func validIdentifyActionResult(result network.IdentifyActionResult) bool {
	if result.Status != network.IdentifyAccepted && result.Status != network.IdentifyUnsupported && result.Status != network.IdentifyFailed {
		return false
	}
	if result.Supported != (result.Status == network.IdentifyAccepted) {
		return false
	}
	if (result.Status == network.IdentifyAccepted) != !result.ActiveUntil.IsZero() {
		return false
	}
	switch result.Reannouncement {
	case network.IdentifyReannouncementNotRequested, network.IdentifyReannouncementComplete,
		network.IdentifyReannouncementPartial, network.IdentifyReannouncementUnavailable:
		return true
	default:
		return false
	}
}

func validIdentifyEffectResult(value string) bool {
	return value == "" || value == identifyEffectStarted || value == network.IdentifyReannouncementComplete ||
		value == network.IdentifyReannouncementPartial || value == network.IdentifyReannouncementUnavailable
}

func validateIdentifyActionLedger(ledger IdentifyActionLedger) error {
	if ledger.Version == 0 && len(ledger.Operations) == 0 {
		return nil
	}
	if ledger.Version != 1 || len(ledger.Operations) > identifyActionLimit {
		return errors.New("invalid identify action ledger")
	}
	seen := make(map[string]bool, len(ledger.Operations))
	for _, operation := range ledger.Operations {
		if operation.Version != 1 || !validIdentifyActionRequest(operation.Request) || seen[operation.Request.OperationID] ||
			(operation.State != IdentifyActionPending && operation.State != IdentifyActionCompleted) || operation.UpdatedAt.IsZero() ||
			!validIdentifyEffectResult(operation.MDNSResult) || !validIdentifyEffectResult(operation.SSDPResult) {
			return errors.New("invalid identify action record")
		}
		if operation.IndicatorDone && !validIdentifyActionResult(operation.Result) {
			return errors.New("invalid identify indicator result")
		}
		if operation.State == IdentifyActionCompleted && !validIdentifyActionResult(operation.Result) {
			return errors.New("invalid identify action result")
		}
		if operation.State == IdentifyActionCompleted && (!operation.IndicatorDone ||
			operation.Request.Source == network.IdentifySourceButton && (operation.MDNSResult == "" || operation.SSDPResult == "")) {
			return errors.New("incomplete identify action result")
		}
		if operation.State == IdentifyActionCompleted &&
			(operation.MDNSResult == identifyEffectStarted || operation.SSDPResult == identifyEffectStarted) {
			return errors.New("in-progress identify action result")
		}
		seen[operation.Request.OperationID] = true
	}
	return nil
}

func sameIdentifyActionRequest(left, right network.IdentifyActionRequest) bool {
	return left.OperationID == right.OperationID && left.Source == right.Source && left.ActiveUntil.Equal(right.ActiveUntil)
}

func copyIdentifyActionLedger(ledger IdentifyActionLedger) IdentifyActionLedger {
	ledger.Operations = append([]IdentifyActionRecord(nil), ledger.Operations...)
	return ledger
}

// ExecuteIdentifyAction owns the external action receipt. A completed result
// is saved in the helper's private store before it crosses the RPC boundary,
// so a Core audit retry obtains the exact receipt without touching hardware or
// discovery again.
func (e *Engine) ExecuteIdentifyAction(
	ctx context.Context, request network.IdentifyActionRequest,
) (network.IdentifyActionResult, error) {
	if !validIdentifyActionRequest(request) {
		return network.IdentifyActionResult{}, errors.New("invalid identify action request")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if request.ActiveUntil.After(e.now().UTC().Add(network.IdentifyActionMaxDuration)) {
		return network.IdentifyActionResult{}, errors.New("identify action deadline exceeds the bounded interval")
	}
	if e.identifyStore == nil {
		return network.IdentifyActionResult{}, errors.New("identify action store is unavailable")
	}
	for _, operation := range e.identifyActions.Operations {
		if operation.Request.OperationID != request.OperationID {
			continue
		}
		if !sameIdentifyActionRequest(operation.Request, request) {
			return network.IdentifyActionResult{}, errors.New("identify operation identity conflicts with its durable request")
		}
		if operation.State == IdentifyActionCompleted {
			return operation.Result, nil
		}
		return e.executePendingIdentifyActionLocked(ctx, request)
	}
	next := IdentifyActionRecord{
		Version: 1, Request: request, State: IdentifyActionPending, UpdatedAt: e.now().UTC(),
	}
	ledger := copyIdentifyActionLedger(e.identifyActions)
	ledger.Version = 1
	if len(ledger.Operations) >= identifyActionLimit {
		return network.IdentifyActionResult{}, errors.New("identify action ledger is full")
	}
	ledger.Operations = append(ledger.Operations, next)
	if err := e.identifyStore.SaveIdentifyActions(ledger); err != nil {
		return network.IdentifyActionResult{}, fmt.Errorf("save identify action intent: %w", err)
	}
	e.identifyActions = ledger
	return e.executePendingIdentifyActionLocked(ctx, request)
}

func (e *Engine) executePendingIdentifyActionLocked(
	ctx context.Context, request network.IdentifyActionRequest,
) (network.IdentifyActionResult, error) {
	record, ok := e.identifyActionRecordLocked(request.OperationID)
	if !ok {
		return network.IdentifyActionResult{}, errors.New("identify action intent is unavailable")
	}
	if !record.IndicatorDone {
		result := network.IdentifyActionResult{
			Status: network.IdentifyUnsupported, Reannouncement: network.IdentifyReannouncementNotRequested,
		}
		indicator, supported := e.hardware.(identifyIndicatorHardware)
		if e.hardware != nil && !supported {
			result.Status = network.IdentifyFailed
		} else if supported {
			activeUntil := request.ActiveUntil
			if !activeUntil.After(e.now()) {
				activeUntil = e.now().UTC().Add(10 * time.Second)
			}
			actualUntil, err := indicator.SetIdentifyIndicator(ctx, request.OperationID, activeUntil, e.recoveryIndicator)
			if err != nil {
				if !errors.Is(err, ErrDirectRecoveryUnsupported) {
					result.Status = network.IdentifyFailed
				}
			} else if actualUntil.IsZero() {
				result.Status = network.IdentifyFailed
			} else {
				result.Supported = true
				result.Status = network.IdentifyAccepted
				result.ActiveUntil = actualUntil.UTC()
				if actualUntil.After(e.identifyUntil) {
					e.identifyUntil = actualUntil.UTC()
					e.scheduleIdentifyExpiryLocked(e.identifyUntil)
				}
			}
		}
		record.IndicatorDone = true
		record.Result = result
		if request.Source == network.IdentifySourceInterface {
			record.State = IdentifyActionCompleted
		}
		if err := e.saveIdentifyActionRecordLocked(record); err != nil {
			return network.IdentifyActionResult{}, fmt.Errorf("save identify indicator result: %w", err)
		}
	}
	if request.Source == network.IdentifySourceInterface {
		return record.Result, nil
	}
	record, _ = e.identifyActionRecordLocked(request.OperationID)
	if record.MDNSResult == "" {
		record.MDNSResult = identifyEffectStarted
		if err := e.saveIdentifyActionRecordLocked(record); err != nil {
			return network.IdentifyActionResult{}, fmt.Errorf("save identify mDNS intent: %w", err)
		}
		e.identifyMDNSAttempts[request.OperationID] = true
	}
	record, _ = e.identifyActionRecordLocked(request.OperationID)
	if record.MDNSResult == identifyEffectStarted {
		if e.identifyMDNSAttempts[request.OperationID] {
			record.MDNSResult = e.reannounceMDNSLocked(ctx, request.OperationID+"-mdns")
		} else if e.mdnsPublished {
			// Normal helper restart publication fulfills an interrupted
			// reannouncement intent without issuing the logical action twice.
			record.MDNSResult = network.IdentifyReannouncementComplete
		} else {
			record.MDNSResult = network.IdentifyReannouncementUnavailable
		}
		if err := e.saveIdentifyActionRecordLocked(record); err != nil {
			return network.IdentifyActionResult{}, fmt.Errorf("save identify mDNS result: %w", err)
		}
	}
	record, _ = e.identifyActionRecordLocked(request.OperationID)
	if record.SSDPResult == "" {
		record.SSDPResult = identifyEffectStarted
		if err := e.saveIdentifyActionRecordLocked(record); err != nil {
			return network.IdentifyActionResult{}, fmt.Errorf("save identify SSDP intent: %w", err)
		}
		e.identifySSDPAttempts[request.OperationID] = true
	}
	record, _ = e.identifyActionRecordLocked(request.OperationID)
	if record.SSDPResult == identifyEffectStarted {
		if e.identifySSDPAttempts[request.OperationID] {
			record.SSDPResult = e.reannounceSSDPLocked(ctx, request.OperationID+"-ssdp")
		} else if e.ssdpPublished {
			// SSDP startup publication is the idempotent completion of an
			// interrupted reannouncement intent.
			record.SSDPResult = network.IdentifyReannouncementComplete
		} else {
			record.SSDPResult = network.IdentifyReannouncementUnavailable
		}
		record.Result.Reannouncement = combinedIdentifyReannouncement(record.MDNSResult, record.SSDPResult)
		record.State = IdentifyActionCompleted
		if err := e.saveIdentifyActionRecordLocked(record); err != nil {
			return network.IdentifyActionResult{}, fmt.Errorf("save identify SSDP result: %w", err)
		}
	}
	record, _ = e.identifyActionRecordLocked(request.OperationID)
	return record.Result, nil
}

func (e *Engine) identifyActionRecordLocked(operationID string) (IdentifyActionRecord, bool) {
	for _, operation := range e.identifyActions.Operations {
		if operation.Request.OperationID == operationID {
			return operation, true
		}
	}
	return IdentifyActionRecord{}, false
}

func (e *Engine) saveIdentifyActionRecordLocked(record IdentifyActionRecord) error {
	ledger := copyIdentifyActionLedger(e.identifyActions)
	for index := range ledger.Operations {
		if ledger.Operations[index].Request.OperationID == record.Request.OperationID {
			record.UpdatedAt = e.now().UTC()
			ledger.Operations[index] = record
			if err := e.identifyStore.SaveIdentifyActions(ledger); err != nil {
				return err
			}
			e.identifyActions = ledger
			return nil
		}
	}
	return errors.New("identify action intent is unavailable")
}

func (e *Engine) reannounceMDNSLocked(ctx context.Context, operationID string) string {
	if !e.mdnsPublished {
		return network.IdentifyReannouncementUnavailable
	}
	driver, ok := e.driver.(identifyMDNSDriver)
	if !ok || driver.ReannounceMDNS(ctx, operationID, e.mdnsAdvertisement) != nil {
		return network.IdentifyReannouncementPartial
	}
	return network.IdentifyReannouncementComplete
}

func (e *Engine) reannounceSSDPLocked(ctx context.Context, operationID string) string {
	if !e.ssdpPublished {
		return network.IdentifyReannouncementUnavailable
	}
	responder, ok := e.ssdp.(identifySSDPResponder)
	if !ok || responder.Reannounce(ctx, operationID, e.ssdpPresence) != nil {
		return network.IdentifyReannouncementPartial
	}
	return network.IdentifyReannouncementComplete
}

func combinedIdentifyReannouncement(mdns, ssdp string) string {
	if mdns == network.IdentifyReannouncementPartial || ssdp == network.IdentifyReannouncementPartial {
		return network.IdentifyReannouncementPartial
	}
	if mdns == network.IdentifyReannouncementUnavailable && ssdp == network.IdentifyReannouncementUnavailable {
		return network.IdentifyReannouncementUnavailable
	}
	return network.IdentifyReannouncementComplete
}

// AcknowledgeIdentifyAction removes a completed receipt only after Core has
// durably stored the exact outcome. Repeating the acknowledgement is safe.
func (e *Engine) AcknowledgeIdentifyAction(ctx context.Context, operationID string) error {
	if !validID.MatchString(operationID) {
		return errors.New("invalid identify action identity")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	index := -1
	for candidate := range e.identifyActions.Operations {
		if e.identifyActions.Operations[candidate].Request.OperationID == operationID {
			index = candidate
			break
		}
	}
	if index == -1 {
		return nil
	}
	if e.identifyActions.Operations[index].State != IdentifyActionCompleted {
		return errors.New("identify action result is not complete")
	}
	if hardware, ok := e.hardware.(identifyIndicatorHardware); ok {
		if err := hardware.AcknowledgeIdentifyIndicator(ctx, operationID); err != nil {
			return err
		}
	}
	if driver, ok := e.driver.(identifyMDNSDriver); ok {
		driver.AcknowledgeMDNSReannouncement(operationID + "-mdns")
	}
	if responder, ok := e.ssdp.(identifySSDPResponder); ok {
		responder.AcknowledgeReannouncement(operationID + "-ssdp")
	}
	delete(e.identifyMDNSAttempts, operationID)
	delete(e.identifySSDPAttempts, operationID)
	ledger := copyIdentifyActionLedger(e.identifyActions)
	ledger.Operations = append(ledger.Operations[:index], ledger.Operations[index+1:]...)
	if err := e.identifyStore.SaveIdentifyActions(ledger); err != nil {
		return fmt.Errorf("acknowledge identify action: %w", err)
	}
	e.identifyActions = ledger
	return nil
}

func (e *Engine) applyIndicatorLocked(ctx context.Context) error {
	if e.hardware == nil {
		return ErrDirectRecoveryUnsupported
	}
	state := e.recoveryIndicator
	if state == "" {
		state = HardwareIndicatorIdle
	}
	if state == HardwareIndicatorIdle && e.identifyUntil.After(e.now()) {
		state = HardwareIndicatorIdentify
	}
	return e.hardware.SetRecoveryIndicator(ctx, state)
}

func (e *Engine) scheduleIdentifyExpiryLocked(activeUntil time.Time) {
	if e.identifyTimer != nil {
		e.identifyTimer.Stop()
	}
	delay := time.Until(activeUntil)
	if delay < 0 {
		delay = 0
	}
	e.identifyTimer = time.AfterFunc(delay, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if !e.identifyUntil.Equal(activeUntil) {
			return
		}
		e.identifyUntil = time.Time{}
		_ = e.applyIndicatorLocked(context.Background())
	})
}
