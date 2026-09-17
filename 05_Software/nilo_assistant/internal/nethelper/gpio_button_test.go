package nethelper

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type memoryHardwareEventStore struct {
	record    HardwareRecoveryEventRecord
	failSaves int
}

func (store *memoryHardwareEventStore) LoadHardwareRecoveryEvent() (HardwareRecoveryEventRecord, error) {
	return store.record, nil
}

func (store *memoryHardwareEventStore) SaveHardwareRecoveryEvent(record HardwareRecoveryEventRecord) error {
	if store.failSaves > 0 {
		store.failSaves--
		return errors.New("injected hardware event save failure")
	}
	store.record = record
	return nil
}

func TestClassifyRecoveryButtonReleaseBoundaries(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		duration time.Duration
		want     HardwareRecoveryAction
	}{
		{name: "bounce", duration: 49 * time.Millisecond, want: HardwareRecoveryNone},
		{name: "short", duration: 2999 * time.Millisecond, want: HardwareRecoveryIdentify},
		{name: "recovery lower boundary", duration: 3 * time.Second, want: HardwareRecoveryArm},
		{name: "recovery upper interior", duration: 9999 * time.Millisecond, want: HardwareRecoveryArm},
		{name: "reset boundary", duration: 10 * time.Second, want: HardwareRecoveryResetCommissioning},
		{name: "long reset", duration: 30 * time.Second, want: HardwareRecoveryResetCommissioning},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyRecoveryButtonRelease(testCase.duration); got != testCase.want {
				t.Fatalf("ClassifyRecoveryButtonRelease(%s) = %q, want %q", testCase.duration, got, testCase.want)
			}
		})
	}
}

func TestRecoveryIndicatorCadence(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		state HardwareIndicatorState
		at    time.Duration
		want  bool
	}{
		{state: HardwareIndicatorIdle, at: 0, want: false},
		{state: HardwareIndicatorIdentify, at: 0, want: true},
		{state: HardwareIndicatorIdentify, at: 200 * time.Millisecond, want: false},
		{state: HardwareIndicatorIdentify, at: 400 * time.Millisecond, want: true},
		{state: HardwareIndicatorIdentify, at: 1200 * time.Millisecond, want: false},
		{state: HardwareIndicatorIdentify, at: 2 * time.Second, want: true},
		{state: HardwareIndicatorArmed, at: 0, want: true},
		{state: HardwareIndicatorArmed, at: 500 * time.Millisecond, want: false},
		{state: HardwareIndicatorActive, at: time.Hour, want: true},
		{state: HardwareIndicatorResetPending, at: 0, want: true},
		{state: HardwareIndicatorResetPending, at: 125 * time.Millisecond, want: false},
		{state: HardwareIndicatorFailed, at: 0, want: true},
		{state: HardwareIndicatorFailed, at: 200 * time.Millisecond, want: false},
		{state: HardwareIndicatorFailed, at: 400 * time.Millisecond, want: true},
		{state: HardwareIndicatorFailed, at: 600 * time.Millisecond, want: false},
		{state: HardwareIndicatorFailed, at: 2 * time.Second, want: true},
	} {
		if got := RecoveryIndicatorOn(testCase.state, testCase.at); got != testCase.want {
			t.Errorf("RecoveryIndicatorOn(%q, %s) = %t, want %t", testCase.state, testCase.at, got, testCase.want)
		}
	}
}

func TestButtonDetectorEmitsOnceOnDebouncedRelease(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	detector := NewRecoveryButtonDetector(50 * time.Millisecond)
	for _, observation := range []struct {
		at      time.Duration
		pressed bool
		want    HardwareRecoveryAction
	}{
		{at: 0, pressed: false},
		{at: time.Millisecond, pressed: true},
		{at: 30 * time.Millisecond, pressed: false},
		{at: 40 * time.Millisecond, pressed: true},
		{at: 91 * time.Millisecond, pressed: true},
		{at: 3200 * time.Millisecond, pressed: false},
		{at: 3260 * time.Millisecond, pressed: false, want: HardwareRecoveryArm},
		{at: 4 * time.Second, pressed: false},
	} {
		if got := detector.Observe(started.Add(observation.at), observation.pressed); got != observation.want {
			t.Fatalf("Observe(%s, %t) = %q, want %q", observation.at, observation.pressed, got, observation.want)
		}
	}
}

func TestButtonDetectorReportsResetPendingOnlyWhileAStableLongPressIsHeld(t *testing.T) {
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	detector := NewRecoveryButtonDetector(50 * time.Millisecond)
	detector.Observe(start, false)
	detector.Observe(start.Add(time.Millisecond), true)
	detector.Observe(start.Add(60*time.Millisecond), true)
	if detector.ResetPending(start.Add(10*time.Second - time.Millisecond)) {
		t.Fatal("reset became pending before ten seconds of stable hold")
	}
	if !detector.ResetPending(start.Add(10*time.Second + 60*time.Millisecond)) {
		t.Fatal("reset did not become pending after ten seconds of stable hold")
	}
	detector.Observe(start.Add(11*time.Second), false)
	detector.Observe(start.Add(11*time.Second+60*time.Millisecond), false)
	if detector.ResetPending(start.Add(12 * time.Second)) {
		t.Fatal("reset remained pending after release")
	}
}

func TestButtonDetectorDiscontinuityCannotTurnAShortPressIntoReset(t *testing.T) {
	start := time.Date(2026, 9, 14, 13, 0, 0, 0, time.UTC)
	detector := NewRecoveryButtonDetector(50 * time.Millisecond)
	detector.Observe(start, false)
	detector.Observe(start.Add(time.Second), true)
	detector.Observe(start.Add(time.Second+50*time.Millisecond), true)
	detector.Discontinue()
	if action := detector.Observe(start.Add(12*time.Second), false); action != HardwareRecoveryNone {
		t.Fatalf("stale gesture after read outage produced %q", action)
	}
	detector.Observe(start.Add(12*time.Second+50*time.Millisecond), false)
	if detector.ResetPending(start.Add(12 * time.Second)) {
		t.Fatal("read outage retained reset-pending state")
	}
	detector.Observe(start.Add(13*time.Second), true)
	detector.Observe(start.Add(13*time.Second+50*time.Millisecond), true)
	detector.Observe(start.Add(16*time.Second+50*time.Millisecond), false)
	if action := detector.Observe(start.Add(16*time.Second+100*time.Millisecond), false); action != HardwareRecoveryArm {
		t.Fatalf("fresh post-outage press produced %q", action)
	}
}

func TestButtonDetectorDiscontinuityRequiresAReleasedBaseline(t *testing.T) {
	start := time.Date(2026, 9, 14, 15, 0, 0, 0, time.UTC)
	detector := NewRecoveryButtonDetector(50 * time.Millisecond)
	detector.Observe(start, false)
	detector.Observe(start.Add(time.Second), true)
	detector.Observe(start.Add(time.Second+50*time.Millisecond), true)
	detector.Discontinue()

	if action := detector.Observe(start.Add(12*time.Second), true); action != HardwareRecoveryNone {
		t.Fatalf("held button produced %q when sampling resumed", action)
	}
	if action := detector.Observe(start.Add(22*time.Second), true); action != HardwareRecoveryNone {
		t.Fatalf("continued hold produced %q after the reset threshold", action)
	}
	detector.Observe(start.Add(23*time.Second), false)
	if action := detector.Observe(start.Add(23*time.Second+50*time.Millisecond), false); action != HardwareRecoveryNone {
		t.Fatalf("release after an unreadable hold produced %q", action)
	}

	detector.Observe(start.Add(24*time.Second), true)
	detector.Observe(start.Add(24*time.Second+50*time.Millisecond), true)
	detector.Observe(start.Add(27*time.Second+50*time.Millisecond), false)
	if action := detector.Observe(start.Add(27*time.Second+100*time.Millisecond), false); action != HardwareRecoveryArm {
		t.Fatalf("fresh press after a released baseline produced %q", action)
	}
}

func TestHardwareEventQueueRetainsUnacknowledgedReleaseAndMonotonicGenerationAcrossRestart(t *testing.T) {
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Publish(HardwareRecoveryArm); err != nil {
		t.Fatal(err)
	}
	first, present, err := queue.Read(context.Background())
	if err != nil || !present {
		t.Fatalf("first event present=%t err=%v", present, err)
	}
	restarted, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	retained, present, err := restarted.Read(context.Background())
	if err != nil || !present || !reflect.DeepEqual(retained, first) {
		t.Fatalf("retained=%+v present=%t err=%v, want %+v", retained, present, err, first)
	}
	if err := restarted.Acknowledge(context.Background(), retained.ID); err != nil {
		t.Fatal(err)
	}
	again, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Publish(HardwareRecoveryResetCommissioning); err != nil {
		t.Fatal(err)
	}
	second, present, err := again.Read(context.Background())
	if err != nil || !present || second.Generation <= first.Generation {
		t.Fatalf("second=%+v present=%t err=%v, want generation after %+v", second, present, err, first)
	}
}

func TestHardwareEventQueueRetainsIdentifyTokenUntilAcknowledged(t *testing.T) {
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().UTC().Add(10 * time.Second)
	actual, created, err := queue.SetIdentifyOperation("identify-operation-1", deadline)
	if err != nil || !created || !actual.Equal(deadline) {
		t.Fatalf("first token actual=%s created=%t err=%v", actual, created, err)
	}
	restarted, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	actual, created, err = restarted.SetIdentifyOperation("identify-operation-1", deadline)
	if err != nil || created || !actual.Equal(deadline) {
		t.Fatalf("replayed token actual=%s created=%t err=%v", actual, created, err)
	}
	if !restarted.ActiveIdentifyDeadline(time.Now()).Equal(deadline) {
		t.Fatalf("active deadline = %s, want %s", restarted.ActiveIdentifyDeadline(time.Now()), deadline)
	}
	acknowledgedAt := deadline.Add(-time.Second)
	if err := restarted.AcknowledgeIdentifyOperation("identify-operation-1", acknowledgedAt); err != nil {
		t.Fatal(err)
	}
	if len(store.record.IdentifyOperations) != 1 || !store.record.IdentifyOperations[0].Acknowledged {
		t.Fatalf("active acknowledged token was not retained through expiry: %+v", store.record.IdentifyOperations)
	}
	restartedAgain, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if !restartedAgain.ActiveIdentifyDeadline(acknowledgedAt).Equal(deadline) {
		t.Fatalf("acknowledged active deadline was lost across restart: %+v", store.record.IdentifyOperations)
	}
	if err := restartedAgain.ExpireIdentifyOperations(deadline.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(store.record.IdentifyOperations) != 0 {
		t.Fatalf("acknowledged expired token retained: %+v", store.record.IdentifyOperations)
	}
}

func TestHardwareEventQueueExpiredRetryPreservesOriginalDeadline(t *testing.T) {
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 9, 17, 8, 0, 10, 0, time.UTC)
	if _, _, err := queue.SetIdentifyOperation("identify-operation-expired", deadline); err != nil {
		t.Fatal(err)
	}
	actual, created, err := queue.SetIdentifyOperation("identify-operation-expired", deadline.Add(10*time.Second))
	if err != nil || created || !actual.Equal(deadline) {
		t.Fatalf("retry actual=%s created=%t err=%v, want original deadline %s", actual, created, err, deadline)
	}
}

func TestHardwareEventQueueRejectsInconsistentOrExhaustedDurableState(t *testing.T) {
	t.Parallel()

	for _, record := range []HardwareRecoveryEventRecord{
		{Version: 1, Generation: 4, Pending: HardwareRecoveryEvent{Generation: 4, Action: HardwareRecoveryArm}},
		{Version: 1, Generation: 4, Pending: HardwareRecoveryEvent{ID: "gpio-00000000000000000004", Generation: 4}},
	} {
		if _, err := newRecoveryEventQueue(&memoryHardwareEventStore{record: record}); err == nil {
			t.Fatalf("accepted inconsistent durable event record: %+v", record)
		}
	}

	store := &memoryHardwareEventStore{record: HardwareRecoveryEventRecord{Version: 1, Generation: math.MaxUint64}}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Publish(HardwareRecoveryArm); err == nil {
		t.Fatal("published an event after exhausting the durable generation")
	}
}

func TestHardwareEventQueueCoalescesRecoveryAndLetsResetSupersede(t *testing.T) {
	t.Parallel()
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	firstAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	if err := queue.PublishEvent(HardwareRecoveryEvent{
		Action: HardwareRecoveryArm, ReleasedAt: firstAt,
		ConnectedAdapterIDs: []string{"adapter-a"}, SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queue.PublishEvent(HardwareRecoveryEvent{
		Action: HardwareRecoveryArm, ReleasedAt: firstAt.Add(time.Minute),
		ConnectedAdapterIDs: []string{"adapter-b"}, SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	extended, present, err := queue.Read(context.Background())
	if err != nil || !present || extended.Generation != 2 || extended.ReleasedAt != firstAt.Add(time.Minute) ||
		len(extended.ConnectedAdapterIDs) != 1 || extended.ConnectedAdapterIDs[0] != "adapter-b" {
		t.Fatalf("coalesced recovery=%+v present=%t err=%v", extended, present, err)
	}
	if err := queue.PublishEvent(HardwareRecoveryEvent{
		Action: HardwareRecoveryResetCommissioning, ReleasedAt: firstAt.Add(2 * time.Minute), SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	reset, present, err := queue.Read(context.Background())
	if err != nil || !present || reset.Generation != 3 || reset.Action != HardwareRecoveryResetCommissioning {
		t.Fatalf("superseding reset=%+v present=%t err=%v", reset, present, err)
	}
	if err := queue.PublishEvent(HardwareRecoveryEvent{
		Action: HardwareRecoveryArm, ReleasedAt: firstAt.Add(3 * time.Minute), SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	retained, _, _ := queue.Read(context.Background())
	if !reflect.DeepEqual(retained, reset) {
		t.Fatalf("recovery replaced pending reset: got=%+v want=%+v", retained, reset)
	}
}

func TestHardwareEventQueueLetsRecoverySupersedeIdentify(t *testing.T) {
	store := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	if err := queue.PublishEvent(HardwareRecoveryEvent{Action: HardwareRecoveryIdentify, ReleasedAt: at, SnapshotValid: true}); err != nil {
		t.Fatal(err)
	}
	if err := queue.PublishEvent(HardwareRecoveryEvent{
		Action: HardwareRecoveryArm, ReleasedAt: at.Add(time.Second),
		ConnectedAdapterIDs: []string{"adapter-a"}, SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	event, present, err := queue.Read(context.Background())
	if err != nil || !present || event.Action != HardwareRecoveryArm {
		t.Fatalf("recovery did not supersede identify: event=%+v present=%t err=%v", event, present, err)
	}
}

func TestRecoveryLinkSnapshotRejectsAnUnidentifiedConnectedEthernet(t *testing.T) {
	t.Parallel()
	connected, valid := recoveryLinkSnapshot([]Device{
		{Interface: network.Interface{Name: "eth0", AdapterID: "adapter-a", Kind: network.KindEthernet, Physical: true, LinkUp: true}},
		{Interface: network.Interface{Name: "eth1", Kind: network.KindEthernet, Physical: true, LinkUp: true}},
		{Interface: network.Interface{Name: "wlan0", AdapterID: "wifi", Kind: network.KindWiFi, LinkUp: true}},
	})
	if valid || len(connected) != 1 || connected[0] != "adapter-a" {
		t.Fatalf("snapshot=%v valid=%t", connected, valid)
	}
}
