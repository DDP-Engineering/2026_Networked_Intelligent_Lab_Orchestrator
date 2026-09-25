package sqlite

import (
	"context"
	"slices"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestHardwareRecoveryRecordSurvivesRestartWithoutNetworkPayload(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewHardwareRecoveryRepository(database)
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	want := network.RecoveryRecord{
		Version: 1, State: network.RecoveryActive, ArmedAt: now,
		ExpiresAt: now.Add(10 * time.Minute), ConnectedAtArm: []string{"adapter-a", "adapter-b"},
		AdapterID: "adapter-c", Interface: "eth1", HelperRecordID: "record-1",
		PendingAudits: []string{"activated"}, UpdatedAt: now,
	}
	if err := repository.SaveHardwareRecovery(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := repository.LoadHardwareRecovery(ctx)
	if err != nil || !found {
		t.Fatalf("LoadHardwareRecovery = %+v/%t/%v", got, found, err)
	}
	if got.State != want.State || got.AdapterID != want.AdapterID || got.Interface != want.Interface ||
		got.HelperRecordID != want.HelperRecordID || got.ArmedAt != want.ArmedAt || got.ExpiresAt != want.ExpiresAt ||
		!slices.Equal(got.ConnectedAtArm, want.ConnectedAtArm) || !slices.Equal(got.PendingAudits, want.PendingAudits) {
		t.Fatalf("loaded record = %+v, want %+v", got, want)
	}
}
