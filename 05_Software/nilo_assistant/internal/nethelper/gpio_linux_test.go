//go:build linux

package nethelper

import (
	"context"
	"testing"
	"time"
	"unsafe"
)

func TestGPIOV2RequestMatchesLinuxABI(t *testing.T) {
	var request gpioV2LineRequest
	if got := unsafe.Sizeof(request); got != 592 {
		t.Fatalf("request size = %d, want 592", got)
	}
	if got := unsafe.Offsetof(request.Padding); got != 568 {
		t.Fatalf("padding offset = %d, want 568", got)
	}
	if got := unsafe.Offsetof(request.FD); got != 588 {
		t.Fatalf("fd offset = %d, want 588", got)
	}
}

func TestPiIdentifyIndicatorSurvivesAcknowledgedRestartAndExpires(t *testing.T) {
	now := time.Now().UTC()
	deadline := now.Add(time.Minute)
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = queue.SetIdentifyOperation("identify-pi-restart", deadline); err != nil {
		t.Fatal(err)
	}

	restartedBeforeAck := &PiRecoveryHardware{
		events: queue, indicator: HardwareIndicatorIdle, recoveryState: HardwareIndicatorIdle,
	}
	if err = restartedBeforeAck.refreshIdentifyIndicator(now); err != nil {
		t.Fatal(err)
	}
	if restartedBeforeAck.indicator != HardwareIndicatorIdentify {
		t.Fatalf("pre-ack restart indicator = %q", restartedBeforeAck.indicator)
	}
	if err = restartedBeforeAck.AcknowledgeIdentifyIndicator(context.Background(), "identify-pi-restart"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	restartedAfterAck := &PiRecoveryHardware{
		events: reloaded, indicator: HardwareIndicatorIdle, recoveryState: HardwareIndicatorIdle,
	}
	if err = restartedAfterAck.refreshIdentifyIndicator(now); err != nil {
		t.Fatal(err)
	}
	if restartedAfterAck.indicator != HardwareIndicatorIdentify {
		t.Fatalf("post-ack restart indicator = %q", restartedAfterAck.indicator)
	}
	if err = restartedAfterAck.refreshIdentifyIndicator(deadline.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if restartedAfterAck.indicator != HardwareIndicatorIdle || len(store.record.IdentifyOperations) != 0 {
		t.Fatalf("expired indicator=%q tokens=%+v", restartedAfterAck.indicator, store.record.IdentifyOperations)
	}
}

func TestPiIdentifyExpiryRestoresStateWhenCleanupPersistenceFails(t *testing.T) {
	now := time.Now().UTC()
	deadline := now.Add(time.Minute)
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = queue.SetIdentifyOperation("identify-cleanup-retry", deadline); err != nil {
		t.Fatal(err)
	}
	hardware := &PiRecoveryHardware{
		events: queue, indicator: HardwareIndicatorIdentify, recoveryState: HardwareIndicatorIdle,
		identifyUntil: deadline,
	}
	if err = hardware.AcknowledgeIdentifyIndicator(context.Background(), "identify-cleanup-retry"); err != nil {
		t.Fatal(err)
	}

	store.failSaves = 1
	if err = hardware.refreshIdentifyIndicator(deadline.Add(time.Millisecond)); err == nil {
		t.Fatal("expiry cleanup persistence unexpectedly succeeded")
	}
	if hardware.indicator != HardwareIndicatorIdle {
		t.Fatalf("failed cleanup left indicator at %q", hardware.indicator)
	}
	if len(store.record.IdentifyOperations) != 1 {
		t.Fatalf("failed cleanup discarded durable retry state: %+v", store.record.IdentifyOperations)
	}
	if hardware.ioErr != nil {
		t.Fatalf("transient cleanup poisoned GPIO input: %v", hardware.ioErr)
	}
	if _, _, err = hardware.ReadRecoveryEvent(context.Background()); err != nil {
		t.Fatalf("transient cleanup blocked recovery input: %v", err)
	}
	if err = hardware.SetRecoveryIndicator(context.Background(), HardwareIndicatorArmed); err != nil || hardware.indicator != HardwareIndicatorArmed {
		t.Fatalf("recovery precedence after cleanup failure indicator=%q err=%v", hardware.indicator, err)
	}
	if err = hardware.refreshIdentifyIndicator(deadline.Add(2 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(store.record.IdentifyOperations) != 0 || hardware.indicator != HardwareIndicatorArmed {
		t.Fatalf("cleanup retry tokens=%+v indicator=%q", store.record.IdentifyOperations, hardware.indicator)
	}
}

func TestPiStartupContinuesWhenExpiredTokenCleanupPersistenceFails(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryHardwareEventStore{
		record: HardwareRecoveryEventRecord{Version: 1, IdentifyOperations: []HardwareIdentifyOperation{{
			ID: "identify-expired-at-startup", ActiveUntil: now.Add(-time.Second), Acknowledged: true,
		}}},
		failSaves: 1,
	}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if activeUntil := restorePiIdentifyDeadline(queue, now); !activeUntil.IsZero() {
		t.Fatalf("expired startup token remained physically active until %s", activeUntil)
	}
	if len(store.record.IdentifyOperations) != 1 {
		t.Fatalf("failed startup cleanup discarded retry evidence: %+v", store.record.IdentifyOperations)
	}
	if activeUntil := restorePiIdentifyDeadline(queue, now.Add(time.Millisecond)); !activeUntil.IsZero() {
		t.Fatalf("cleanup retry reactivated expired token until %s", activeUntil)
	}
	if len(store.record.IdentifyOperations) != 0 {
		t.Fatalf("startup cleanup retry retained token: %+v", store.record.IdentifyOperations)
	}
}
