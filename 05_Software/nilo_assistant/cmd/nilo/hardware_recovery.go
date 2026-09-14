package main

import (
	"context"
	"errors"
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
	helper      hardwareRecoveryHelper
	service     *network.HardwareRecoveryService
	driver      *runtimeRecoveryDriver
	commission  networkCommissioningResetter
	management  *network.ManagementAccessService
	audit       recoveryAuditWriter
	applianceID string
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
		_ = runtime.helper.SetRecoveryIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
		return err
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
			if err := runtime.helper.SetRecoveryIndicator(ctx, nethelper.HardwareIndicatorResetPending); err != nil {
				return err
			}
			if _, err := runtime.service.Cancel(ctx); err != nil {
				_ = runtime.helper.SetRecoveryIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
				return err
			}
			preparation, err := runtime.management.PrepareNetworkCommissioningReset(ctx)
			if err != nil {
				_ = runtime.record(context.WithoutCancel(ctx), "network.commissioning_reset", audit.OutcomeFailed, map[string]any{"reason": "admission_preparation_failed"})
				_ = runtime.helper.SetRecoveryIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
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
				_ = runtime.helper.SetRecoveryIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
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
		_ = runtime.helper.SetRecoveryIndicator(context.WithoutCancel(ctx), nethelper.HardwareIndicatorFailed)
		return err
	}
	return runtime.setIndicator(ctx, after.State)
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
	err := runtime.helper.SetRecoveryIndicator(ctx, indicator)
	if errors.Is(err, nethelper.ErrDirectRecoveryUnsupported) {
		return nil
	}
	return err
}

func (runtime *hardwareRecoveryRuntime) record(ctx context.Context, action string, outcome audit.Outcome, details any) error {
	return runtime.audit.Record(ctx, audit.Record{
		Category: audit.CategoryAudit, Component: "network.recovery", Action: action,
		SubjectType: "appliance", SubjectID: runtime.applianceID,
		ActorKind: audit.ActorKindSystem, Outcome: outcome, Details: details,
	})
}
