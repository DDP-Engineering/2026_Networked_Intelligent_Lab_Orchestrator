package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManagedDeviceReconcileKeepsSerialAndPortIdentities(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	strong := ManagedDeviceObservation{Kind: "camera", Vendor: "046d", Product: "0825", Serial: "ABC", PortPath: "1-1.2", EndpointID: "video0"}
	blank := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video2"}
	first, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{strong, blank}, now)
	if err != nil || len(first) != 2 {
		t.Fatalf("first reconcile = %+v, %v", first, err)
	}
	if first[0].ID != "cam-1" || first[0].IdentityRule != "serial" || first[1].ID != "cam-2" || first[1].IdentityRule != "port" {
		t.Fatalf("initial identities = %+v", first)
	}
	strong.PortPath = "1-1.8"
	blank.PortPath = "1-1.9"
	second, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{strong, blank}, now.Add(time.Minute))
	if err != nil || len(second) != 3 {
		t.Fatalf("second reconcile = %+v, %v", second, err)
	}
	if second[0].ID != "cam-1" || second[0].PortPath != "1-1.8" || second[0].Presence != "present" {
		t.Fatalf("serial camera did not follow its port: %+v", second[0])
	}
	if second[1].ID != "cam-2" || second[1].Presence != "absent" || second[2].ID != "cam-3" || second[2].Presence != "present" {
		t.Fatalf("port camera did not leave an absent record: %+v", second)
	}
}

func TestManagedDeviceNamesAndStaleRevisionsConflict(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	first := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0"}
	second := first
	second.PortPath, second.EndpointID = "1-1.4", "video2"
	records, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{first, second}, now)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := repository.Configure(ctx, "camera", records[0].ID, records[0].Revision, "Kamera", "top view", []byte(`{}`))
	if err != nil || !accepted.Configured || accepted.Revision != 2 || accepted.Readiness != "initializing" {
		t.Fatalf("accepted camera = %+v, %v", accepted, err)
	}
	if _, err := repository.Configure(ctx, "camera", records[0].ID, records[0].Revision, "other", "", []byte(`{}`)); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("stale write error = %v", err)
	}
	if _, err := repository.Configure(ctx, "camera", records[1].ID, records[1].Revision, "kamera", "", []byte(`{}`)); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("canonically duplicate name error = %v", err)
	}
}

func TestManagedDeviceConfigurationRefusesActiveWork(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	observed := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0"}
	records, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO config_versions(id,configuration_json,digest,created_at) VALUES('camera-config','{}',X'00','2026-09-24T00:00:00Z')`,
		`INSERT INTO operation_requests(id,actor_id,integration_id,operation_name,device_id,canonical_parameters,configuration_snapshot_id,policy_snapshot_id,deadline_at,created_at) VALUES('camera-request','admin','camera','snapshot','cam-1','{}','camera-config','policy','2026-09-24T00:01:00Z','2026-09-24T00:00:00Z')`,
		`INSERT INTO sessions(id,request_id,state,created_at,updated_at) VALUES('camera-session','camera-request','running','2026-09-24T00:00:00Z','2026-09-24T00:00:00Z')`,
	} {
		if _, err := database.SQL().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.Configure(ctx, "camera", records[0].ID, records[0].Revision, "Bench Camera", "", []byte(`{}`)); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("active configuration error = %v", err)
	}
	current, err := repository.List(ctx, "camera")
	if err != nil || current[0].Configured || current[0].Revision != records[0].Revision {
		t.Fatalf("configuration changed during active work: %+v, %v", current, err)
	}
}

func TestManagedDeviceReadinessRequiresCurrentRevision(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	observed := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0"}
	records, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	configured, err := repository.Configure(ctx, "camera", records[0].ID, records[0].Revision, "Bench Camera", "top view", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	ready, err := repository.MarkReady(ctx, "camera", configured.ID, configured.Revision)
	if err != nil || ready.Readiness != "ready" || ready.Revision != configured.Revision+1 {
		t.Fatalf("initialized camera = %+v, %v", ready, err)
	}
	if _, err := repository.MarkReady(ctx, "camera", configured.ID, configured.Revision); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("stale readiness update = %v", err)
	}
	disabled, err := repository.SetEnabled(ctx, "camera", ready.ID, ready.Revision, false)
	if err != nil || disabled.Enabled || disabled.Readiness == "ready" {
		t.Fatalf("disabled camera = %+v, %v", disabled, err)
	}
	enabled, err := repository.SetEnabled(ctx, "camera", disabled.ID, disabled.Revision, true)
	if err != nil || !enabled.Enabled || enabled.Readiness != "initializing" {
		t.Fatalf("enabled camera = %+v, %v", enabled, err)
	}
	if err := repository.MarkFailed(ctx, "camera", enabled.ID, enabled.Revision); err != nil {
		t.Fatal(err)
	}
	current, err := repository.List(ctx, "camera")
	if err != nil || current[0].Readiness != "failed" {
		t.Fatalf("failed camera = %+v, %v", current, err)
	}
	retrying, err := repository.MarkInitializing(ctx, "camera", current[0].ID, current[0].Revision)
	if err != nil || retrying.Readiness != "initializing" {
		t.Fatalf("retry camera = %+v, %v", retrying, err)
	}
}

func TestManagedDeviceReconcileRetainsCapabilities(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	observed := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0", Endpoints: []byte(`[{"id":"video0","formats":["MJPG"]}]`), Capabilities: []byte(`{"formats":["MJPG"]}`)}
	records, err := repository.Reconcile(context.Background(), "camera", []ManagedDeviceObservation{observed}, time.Now())
	if err != nil || len(records) != 1 || string(records[0].Endpoints) != string(observed.Endpoints) || string(records[0].Capabilities) != string(observed.Capabilities) {
		t.Fatalf("retained camera capabilities = %+v, %v", records, err)
	}
}

func TestManagedDeviceDuplicateSerialStaysWeak(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	first := ManagedDeviceObservation{Kind: "camera", Vendor: "1234", Product: "5678", Serial: "DUP", PortPath: "1-1.2", EndpointID: "video0"}
	second := first
	second.PortPath, second.EndpointID = "1-1.3", "video2"
	records, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{first, second}, now)
	if err != nil || len(records) != 2 || !records[0].SerialWeak || !records[1].SerialWeak {
		t.Fatalf("duplicate serial records = %+v, %v", records, err)
	}
	records, err = repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{first}, now.Add(time.Minute))
	if err != nil || records[0].IdentityRule != "port" || !records[0].SerialWeak || records[1].Presence != "absent" {
		t.Fatalf("weak classification after unplug = %+v, %v", records, err)
	}
}

func TestManagedDeviceFailedDiscoveryLeavesUnknown(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	observed := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0"}
	if _, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkUnknown(ctx, "camera"); err != nil {
		t.Fatal(err)
	}
	records, err := repository.List(ctx, "camera")
	if err != nil || len(records) != 1 || records[0].Presence != "unknown" {
		t.Fatalf("after failed discovery = %+v, %v", records, err)
	}
	if records[0].Configured || records[0].Readiness != "unconfigured" {
		t.Fatalf("new camera became ready without administrator acceptance: %+v", records[0])
	}
}

func TestManagedDeviceRebindSerialLessCameraAndForget(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagedDeviceRepository(database)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	observed := ManagedDeviceObservation{Kind: "camera", Vendor: "0458", Product: "708f", PortPath: "1-1.3", EndpointID: "video0"}
	records, err := repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, now)
	if err != nil {
		t.Fatal(err)
	}
	original, err := repository.Configure(ctx, "camera", records[0].ID, records[0].Revision, "Board view", "top", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	observed.PortPath, observed.EndpointID = "1-1.4", "video2"
	records, err = repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, now.Add(time.Minute))
	if err != nil || len(records) != 2 || records[0].Presence != "absent" || records[1].Configured {
		t.Fatalf("moved camera = %+v, %v", records, err)
	}
	if _, err := repository.Rebind(ctx, "camera", original.ID, original.Revision, records[1].ID, records[1].Revision); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("stale rebind = %v", err)
	}
	bound, err := repository.Rebind(ctx, "camera", records[0].ID, records[0].Revision, records[1].ID, records[1].Revision)
	if err != nil || bound.ID != "cam-1" || bound.PortPath != "1-1.4" || bound.EndpointID != "video2" || bound.Readiness != "initializing" {
		t.Fatalf("rebound camera = %+v, %v", bound, err)
	}
	active, err := repository.List(ctx, "camera")
	if err != nil || len(active) != 1 {
		t.Fatalf("active after rebind = %+v, %v", active, err)
	}
	if err := repository.Forget(ctx, "camera", bound.ID, bound.Revision-1); !errors.Is(err, ErrManagedDeviceConflict) {
		t.Fatalf("stale forget = %v", err)
	}
	if err := repository.Forget(ctx, "camera", bound.ID, bound.Revision); err != nil {
		t.Fatal(err)
	}
	active, err = repository.List(ctx, "camera")
	if err != nil || len(active) != 0 {
		t.Fatalf("active after forget = %+v, %v", active, err)
	}
	records, err = repository.Reconcile(ctx, "camera", []ManagedDeviceObservation{observed}, now.Add(2*time.Minute))
	if err != nil || len(records) != 1 || records[0].ID != "cam-3" {
		t.Fatalf("identity reused after forget = %+v, %v", records, err)
	}
}
