package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
)

type runtimeRecoveryDriver struct {
	helper     recoveryLinkHelper
	management *network.ManagementAccessService
	announcer  *network.MDNSAnnouncer
	interfaces func(context.Context) ([]string, error)
}

type recoveryLinkHelper interface {
	ActivateRecoveryLink(context.Context, string, string, time.Time) (string, error)
	ExtendRecoveryLink(context.Context, string, time.Time) error
	RestoreRecoveryLink(context.Context, string) error
}

func (driver *runtimeRecoveryDriver) ActivateRecoveryLink(
	ctx context.Context, adapterID, interfaceName string, expiresAt time.Time,
) (string, error) {
	recordID, err := driver.helper.ActivateRecoveryLink(ctx, adapterID, interfaceName, expiresAt)
	if err != nil {
		return recordID, err
	}
	if err := driver.management.SetRecoveryInterface(ctx, interfaceName); err != nil {
		restoreErr := driver.helper.RestoreRecoveryLink(context.WithoutCancel(ctx), recordID)
		return recordID, errors.Join(err, restoreErr)
	}
	driver.announcer.SetRecoveryInterface(interfaceName)
	interfaces, err := driver.interfaces(ctx)
	if err == nil {
		err = driver.announcer.Announce(ctx, interfaces)
	}
	if err != nil {
		driver.announcer.SetRecoveryInterface("")
		restoreCtx := context.WithoutCancel(ctx)
		managementErr := driver.management.SetRecoveryInterface(restoreCtx, "")
		restoreErr := driver.helper.RestoreRecoveryLink(restoreCtx, recordID)
		return recordID, errors.Join(err, managementErr, restoreErr)
	}
	return recordID, nil
}

func (driver *runtimeRecoveryDriver) ExtendRecoveryLink(ctx context.Context, recordID string, expiresAt time.Time) error {
	return driver.helper.ExtendRecoveryLink(ctx, recordID, expiresAt)
}

func (driver *runtimeRecoveryDriver) RestoreRecoveryLink(ctx context.Context, recordID string) error {
	var cleanupErrs []error
	driver.announcer.SetRecoveryInterface("")
	interfaces, err := driver.interfaces(ctx)
	if err == nil {
		err = driver.announcer.Announce(ctx, interfaces)
	}
	if err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if err := driver.management.SetRecoveryInterface(ctx, ""); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if err := driver.helper.RestoreRecoveryLink(ctx, recordID); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	return errors.Join(cleanupErrs...)
}

func (driver *runtimeRecoveryDriver) Adopt(ctx context.Context, interfaceName string) error {
	if interfaceName == "" {
		return nil
	}
	if err := driver.management.SetRecoveryInterface(ctx, interfaceName); err != nil {
		return err
	}
	driver.announcer.SetRecoveryInterface(interfaceName)
	interfaces, err := driver.interfaces(ctx)
	if err != nil {
		return err
	}
	return driver.announcer.Announce(ctx, interfaces)
}

type hardwareRecoveryRuntime struct {
	helper         hardwareRecoveryHelper
	service        *network.HardwareRecoveryService
	driver         *runtimeRecoveryDriver
	commission     networkCommissioningResetter
	management     *network.ManagementAccessService
	audit          recoveryAuditWriter
	identifyOps    identifyOperationStore
	applianceID    string
	identify       *network.IdentifyService
	identifyAction network.IdentifyActionExecutor
	indicator      *runtimeIdentifyIndicator
	reannounce     func(context.Context) error
}

type runtimeIdentifyIndicator struct {
	mu       sync.Mutex
	helper   hardwareRecoveryHelper
	recovery nethelper.HardwareIndicatorState
	identify bool
}

func newRuntimeIndicatorArbiter(helper hardwareRecoveryHelper) *runtimeIdentifyIndicator {
	return &runtimeIdentifyIndicator{helper: helper, recovery: nethelper.HardwareIndicatorIdle}
}

func (indicator *runtimeIdentifyIndicator) SetIdentifyIndicator(ctx context.Context, enabled bool) error {
	indicator.mu.Lock()
	defer indicator.mu.Unlock()
	indicator.identify = enabled
	err := indicator.apply(ctx)
	if errors.Is(err, nethelper.ErrDirectRecoveryUnsupported) {
		return network.ErrIdentifyUnsupported
	}
	return err
}

func (indicator *runtimeIdentifyIndicator) SetRecoveryIndicator(ctx context.Context, state nethelper.HardwareIndicatorState) error {
	indicator.mu.Lock()
	defer indicator.mu.Unlock()
	indicator.recovery = state
	return indicator.apply(ctx)
}

func (indicator *runtimeIdentifyIndicator) apply(ctx context.Context) error {
	state := indicator.recovery
	if state == nethelper.HardwareIndicatorIdle && indicator.identify {
		state = nethelper.HardwareIndicatorIdentify
	}
	return indicator.helper.SetRecoveryIndicator(ctx, state)
}

type hardwareRecoveryHelper interface {
	ReadRecoveryEvent(context.Context) (nethelper.HardwareRecoveryEvent, bool, error)
	AcknowledgeRecoveryEvent(context.Context, string) error
	SetRecoveryIndicator(context.Context, nethelper.HardwareIndicatorState) error
}

type networkCommissioningResetter interface {
	ResetNetworkCommissioningAudited(context.Context, network.ManagementAccessRecord, audit.Event, uint64) error
}

type recoveryAuditWriter interface {
	Compose(audit.Record) (audit.Event, error)
	Record(context.Context, audit.Record) error
}

type identifyOperationStore interface {
	Begin(context.Context, string, string, time.Time, time.Time) (bool, error)
	State(context.Context, string) (string, error)
	Reserve(context.Context, string, audit.Event, time.Time) error
	RecordOutcome(context.Context, string, bool, string, string, audit.Outcome, time.Time) error
	MarkActed(context.Context, string, time.Time) error
	Flush(context.Context, string) error
}

func (runtime *hardwareRecoveryRuntime) Adopt(ctx context.Context) error {
	record, err := runtime.service.Cancel(ctx)
	if err != nil {
		return err
	}
	return runtime.setIndicator(ctx, record.State)
}

func (runtime *hardwareRecoveryRuntime) Reconcile(ctx context.Context) error {
	before, err := runtime.service.Reconcile(ctx)
	if err != nil {
		_ = runtime.setHardwareIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
		return err
	}
	if before.State != network.RecoveryIdle {
		if err := runtime.setIndicator(ctx, before.State); err != nil {
			return err
		}
	}
	event, present, err := runtime.helper.ReadRecoveryEvent(ctx)
	if err != nil && !errors.Is(err, nethelper.ErrDirectRecoveryUnsupported) {
		return err
	}
	if !present {
		return runtime.setIndicator(ctx, before.State)
	}
	if present {
		if runtime.service.HardwareEventHandled(event.Generation) {
			if err := runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID); err != nil {
				return err
			}
			return runtime.setIndicator(ctx, before.State)
		}
		if event.Action == nethelper.HardwareRecoveryIdentify {
			reserveOperation := true
			if runtime.identifyOps != nil {
				created, err := runtime.identifyOps.Begin(ctx, event.ID, "button", event.ReleasedAt.UTC().Add(10*time.Second), event.ReleasedAt)
				if err != nil {
					return err
				}
				if !created {
					state, err := runtime.identifyOps.State(ctx, event.ID)
					if err != nil {
						return err
					}
					if state == "performed" {
						completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						if runtime.identifyAction != nil {
							if err := runtime.identifyAction.AcknowledgeIdentifyAction(completionContext, event.ID); err != nil {
								return err
							}
						}
						if err := runtime.identifyOps.MarkActed(completionContext, event.ID, event.ReleasedAt); err != nil {
							return err
						}
						if err := runtime.identifyOps.Flush(completionContext, event.ID); err != nil {
							return err
						}
						return runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID)
					}
					if state == "acted" || state == "settled" {
						if state == "acted" && runtime.identifyAction != nil {
							if err := runtime.identifyAction.AcknowledgeIdentifyAction(ctx, event.ID); err != nil {
								return err
							}
						}
						if err := runtime.identifyOps.Flush(ctx, event.ID); err != nil {
							return err
						}
						return runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID)
					}
					reserveOperation = state != "reserved"
				}
			}
			if runtime.audit != nil && runtime.identifyOps != nil && reserveOperation {
				record := runtime.auditRecord("appliance.identify", audit.OutcomeRequested, map[string]any{
					"source": "button", "status": "requested", "reannouncement": "not_requested",
				})
				record.OperationID = event.ID
				reserved, err := runtime.audit.Compose(record)
				if err != nil {
					return err
				}
				completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := runtime.identifyOps.Reserve(completionContext, event.ID, reserved, event.ReleasedAt); err != nil {
					return err
				}
			}
			status := network.IdentifyUnsupported
			outcome := audit.OutcomeApplied
			reannouncement := "not_configured"
			supported := false
			if runtime.identifyAction != nil {
				result, identifyErr := runtime.identifyAction.ExecuteIdentifyAction(ctx, network.IdentifyActionRequest{
					OperationID: event.ID, Source: network.IdentifySourceButton,
					ActiveUntil: event.ReleasedAt.UTC().Add(10 * time.Second),
				})
				if identifyErr != nil {
					return identifyErr
				} else {
					supported = result.Supported
					status = result.Status
					if result.Status == network.IdentifyFailed {
						outcome = audit.OutcomeFailed
					}
					reannouncement = mapActionReannouncement(result.Reannouncement)
				}
			} else if runtime.identify != nil {
				result, identifyErr := runtime.identify.Identify(ctx)
				if identifyErr != nil {
					outcome = audit.OutcomeFailed
				} else {
					status = result.Status
					supported = result.Status == network.IdentifyAccepted
				}
			}
			if runtime.identifyAction == nil && runtime.reannounce != nil {
				if err := runtime.reannounce(ctx); err != nil {
					reannouncement = "failed"
				} else {
					reannouncement = "applied"
				}
			}
			if runtime.audit != nil && runtime.identifyOps != nil {
				completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				storedStatus := string(status)
				if outcome == audit.OutcomeFailed {
					storedStatus = "failed"
				}
				if err := markButtonIdentifyActed(completionContext, runtime.identifyAction, runtime.identifyOps, event.ID,
					supported, mapReannouncement(reannouncement), storedStatus, outcome, event.ReleasedAt); err != nil {
					return err
				}
				if err := runtime.identifyOps.Flush(completionContext, event.ID); err != nil {
					return err
				}
			} else if runtime.audit != nil {
				if err := runtime.record(ctx, "appliance.identify", outcome, map[string]any{
					"source": "button", "status": status, "reannouncement": reannouncement,
				}); err != nil {
					return err
				}
			}
			if err := runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID); err != nil {
				return err
			}
			return nil
		}
		if runtime.management.Provisioning() {
			if _, err := runtime.service.RefuseHardwareEvent(ctx, event.Generation, "appliance_is_provisioning"); err != nil {
				return err
			}
			if err := runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID); err != nil {
				return err
			}
			return runtime.setIndicator(ctx, network.RecoveryIdle)
		}
		switch event.Action {
		case nethelper.HardwareRecoveryArm:
			if _, err := runtime.service.ArmHardwareEvent(ctx, network.RecoveryArmEvent{
				Generation: event.Generation, ReleasedAt: event.ReleasedAt,
				ConnectedAtRelease: event.ConnectedAdapterIDs, SnapshotValid: event.SnapshotValid,
			}); err != nil {
				return err
			}
		case nethelper.HardwareRecoveryResetCommissioning:
			if err := runtime.setHardwareIndicator(ctx, nethelper.HardwareIndicatorResetPending); err != nil {
				return err
			}
			if _, err := runtime.service.Cancel(ctx); err != nil {
				_ = runtime.setHardwareIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
				return err
			}
			preparation, err := runtime.management.PrepareNetworkCommissioningReset(ctx)
			if err != nil {
				_ = runtime.record(context.WithoutCancel(ctx), "network.commissioning_reset", audit.OutcomeFailed, map[string]any{"reason": "admission_preparation_failed"})
				_ = runtime.setHardwareIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
				return err
			}
			eventRecord, err := runtime.audit.Compose(audit.Record{
				Category: audit.CategoryAudit, Component: "network.recovery", Action: "network.commissioning_reset",
				SubjectType: "appliance", SubjectID: runtime.applianceID, ActorKind: audit.ActorKindSystem,
				Outcome: audit.OutcomeApplied, Details: map[string]any{"scope": "network_access_only"},
			})
			if err != nil {
				_ = preparation.Rollback(context.WithoutCancel(ctx))
				return err
			}
			if err := runtime.commission.ResetNetworkCommissioningAudited(ctx, preparation.Record(), eventRecord, event.Generation); err != nil {
				rollbackErr := preparation.Rollback(context.WithoutCancel(ctx))
				_ = runtime.record(context.WithoutCancel(ctx), "network.commissioning_reset", audit.OutcomeFailed, map[string]any{"reason": "persistence_failed"})
				_ = runtime.setHardwareIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
				return errors.Join(err, rollbackErr)
			}
			preparation.Commit()
			runtime.service.AdoptHardwareGeneration(event.Generation)
		default:
			return errors.New("unknown hardware recovery action")
		}
		if err := runtime.helper.AcknowledgeRecoveryEvent(ctx, event.ID); err != nil {
			return err
		}
	}
	after, err := runtime.service.Reconcile(ctx)
	if err != nil {
		_ = runtime.setHardwareIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
		return err
	}
	return runtime.setIndicator(ctx, after.State)
}

func markButtonIdentifyActed(
	ctx context.Context, executor network.IdentifyActionExecutor, operations identifyOperationStore, operationID string, supported bool,
	reannouncement, status string, outcome audit.Outcome, now time.Time,
) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = operations.RecordOutcome(ctx, operationID, supported, reannouncement, status, outcome, now)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if executor != nil {
		if err := executor.AcknowledgeIdentifyAction(ctx, operationID); err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = operations.MarkActed(ctx, operationID, now)
		if err == nil || ctx.Err() != nil {
			return err
		}
	}
	return err
}

func mapReannouncement(value string) string {
	switch value {
	case "applied":
		return "complete"
	case "failed":
		return "partial"
	default:
		return "unavailable"
	}
}

func mapActionReannouncement(value string) string {
	switch value {
	case network.IdentifyReannouncementComplete:
		return "applied"
	case network.IdentifyReannouncementPartial:
		return "failed"
	default:
		return "not_configured"
	}
}

func (runtime *hardwareRecoveryRuntime) setIndicator(ctx context.Context, state network.RecoveryState) error {
	indicator := nethelper.HardwareIndicatorIdle
	switch state {
	case network.RecoveryArmed:
		indicator = nethelper.HardwareIndicatorArmed
	case network.RecoveryActive:
		indicator = nethelper.HardwareIndicatorActive
	case network.RecoveryFailed, network.RecoveryRestoring:
		indicator = nethelper.HardwareIndicatorFailed
	}
	err := runtime.setHardwareIndicator(ctx, indicator)
	if errors.Is(err, nethelper.ErrDirectRecoveryUnsupported) {
		return nil
	}
	return err
}

func (runtime *hardwareRecoveryRuntime) setHardwareIndicator(ctx context.Context, state nethelper.HardwareIndicatorState) error {
	if runtime.indicator != nil {
		return runtime.indicator.SetRecoveryIndicator(ctx, state)
	}
	return runtime.helper.SetRecoveryIndicator(ctx, state)
}

func (runtime *hardwareRecoveryRuntime) record(ctx context.Context, action string, outcome audit.Outcome, details any) error {
	return runtime.audit.Record(ctx, runtime.auditRecord(action, outcome, details))
}

func (runtime *hardwareRecoveryRuntime) auditRecord(action string, outcome audit.Outcome, details any) audit.Record {
	return audit.Record{
		Category: audit.CategoryAudit, Component: "network.recovery", Action: action,
		SubjectType: "appliance", SubjectID: runtime.applianceID,
		ActorKind: audit.ActorKindSystem, Outcome: outcome, Details: details,
	}
}
