package bench

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
)

func TestModbusMapCreatorAndRevision(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	owner := identity.Actor{User: auth.User{ID: uuid.NewString(), Role: auth.RoleOperator}, Permissions: auth.RoleOperator.Permissions()}
	other := identity.Actor{User: auth.User{ID: uuid.NewString(), Role: auth.RoleOperator}, Permissions: auth.RoleOperator.Permissions()}
	item := modbus.Map{SchemaVersion: 1, ID: "map-a", Name: "Motors", Points: []modbus.Point{}}
	saved, err := m.SaveModbusMap(t.Context(), owner, 0, item)
	if err != nil || saved.OwnerID != owner.User.ID || saved.Revision != 1 {
		t.Fatal(saved, err)
	}
	if _, err = m.SaveModbusMap(t.Context(), other, 1, saved); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign edit", err)
	}
	copy, err := m.DuplicateModbusMap(t.Context(), other, "map-a", "map-b")
	if err != nil || copy.OwnerID != other.User.ID || copy.Revision != 1 {
		t.Fatal(copy, err)
	}
	if _, err = m.SaveModbusMap(t.Context(), owner, 0, saved); !errors.Is(err, ErrConflict) {
		t.Fatal("stale", err)
	}
	saved.Name = "New"
	if _, err = m.SaveModbusMap(t.Context(), admin, 1, saved); err != nil {
		t.Fatal("admin edit", err)
	}
	first, err := m.ModbusMapRevision(t.Context(), owner, "map-a", 1)
	if err != nil || first.Name != "Motors" {
		t.Fatal("pinned map read", first, err)
	}
}

func TestModbusWorkspaceReferencesStayInert(t *testing.T) {
	m, admin, runner := testManager(t, "auto")
	endpoint, err := m.SaveModbusEndpoint(t.Context(), admin, 0, networkDefinition("peer", "bus"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := m.SaveModbusProfile(t.Context(), admin, 0, profileDefinition("profile", endpoint.ID))
	if err != nil {
		t.Fatal(err)
	}
	mapSaved, err := m.SaveModbusMap(t.Context(), admin, 0, modbus.Map{SchemaVersion: 1, ID: "map", Name: "Map", Points: []modbus.Point{}})
	if err != nil {
		t.Fatal(err)
	}
	w := modbus.Workspace{SchemaVersion: 1, ID: "desk", Name: "Desk", EndpointID: endpoint.ID, ProfileRef: modbus.RevisionRef{ID: profile.ID, Revision: profile.Revision}, MapRefs: []modbus.RevisionRef{{ID: mapSaved.ID, Revision: mapSaved.Revision}}, PollGroups: []modbus.PollGroup{}, Layout: modbus.Layout{Columns: []string{"name"}, AddressBase: "zero", Series: []string{}}}
	saved, err := m.SaveModbusWorkspace(t.Context(), admin, 0, w)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := m.ModbusWorkspace(t.Context(), admin, saved.ID)
	if err != nil || opened.Revision != 1 {
		t.Fatal(opened, err)
	}
	profile.Name = "Updated profile"
	if _, err = m.SaveModbusProfile(t.Context(), admin, 1, profile); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ModbusWorkspace(t.Context(), admin, saved.ID); err != nil {
		t.Fatal("pinned profile revision lost", err)
	}
	var sessions int
	if err = m.db().QueryRow("SELECT count(*) FROM modbus_sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("workspace created session", sessions, err)
	}
	mapSaved.Name = "Changed"
	if _, err = m.SaveModbusMap(t.Context(), admin, 1, mapSaved); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ModbusWorkspace(t.Context(), admin, saved.ID); err != nil {
		t.Fatal("pinned map revision lost", err)
	}
	saved.MapRefs[0].Revision = 99
	if _, err = m.SaveModbusWorkspace(t.Context(), admin, 1, saved); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing map revision", err)
	}
	saved.MapRefs[0].Revision = 1
	raw, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	importRequest := ModbusWorkspaceImport{Mode: "create", TargetID: "desk-copy", Document: string(raw)}
	preview, err := m.PreviewModbusWorkspaceImport(t.Context(), admin, importRequest)
	if err != nil || !preview.CanApply {
		t.Fatal("workspace preview", preview, err)
	}
	copy, err := m.ImportModbusWorkspace(t.Context(), admin, importRequest)
	if err != nil || copy.ID != "desk-copy" || copy.Revision != 1 {
		t.Fatal("workspace import", copy, err)
	}
	copy.Name = "Revised"
	updated, err := m.SaveModbusWorkspace(t.Context(), admin, 1, copy)
	if err != nil || updated.Revision != 2 {
		t.Fatal(updated, err)
	}
	first, err := m.ModbusWorkspaceRevision(t.Context(), admin, "desk-copy", 1)
	if err != nil || first.Name != "Desk" {
		t.Fatal("historic workspace", first, err)
	}
	if err = m.db().QueryRow("SELECT count(*) FROM modbus_sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("import created session", sessions, err)
	}
	runner.mu.Lock()
	calls := runner.calls
	runner.mu.Unlock()
	if calls != 0 {
		t.Fatal("metadata invoked runner", calls)
	}
}

func TestModbusImportPreviewDoesNotPublish(t *testing.T) {
	m, admin, runner := testManager(t, "auto")
	doc := `{"schema_version":1,"id":"map","revision":1,"owner_id":"source","name":"Imported","points":[]}`
	request := ModbusMapImport{Format: "json", Mode: "create", TargetID: "map", Document: doc}
	preview, err := m.PreviewModbusMapImport(t.Context(), admin, request)
	if err != nil || !preview.CanApply {
		t.Fatal(preview, err)
	}
	if _, err = m.ModbusMap(t.Context(), admin, "map"); !errors.Is(err, ErrNotFound) {
		t.Fatal("preview published", err)
	}
	saved, err := m.ImportModbusMap(t.Context(), admin, request)
	if err != nil || saved.OwnerID != admin.User.ID {
		t.Fatal(saved, err)
	}
	preview, err = m.PreviewModbusMapImport(t.Context(), admin, request)
	if err != nil || preview.CanApply || len(preview.Conflicts) == 0 {
		t.Fatal("missing target conflict", preview, err)
	}
	runner.mu.Lock()
	calls := runner.calls
	runner.mu.Unlock()
	if calls != 0 {
		t.Fatal("map import invoked runner", calls)
	}
}

func TestModbusImportMergeReportsConflictsAndIsAtomic(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	p := modbus.Point{ID: "p", Name: "Original", Area: "holding", Address: 0, Type: "uint16", Width: 1, Order: "normal", Scale: "1", Offset: "0", Access: "read", Labels: map[string]string{}}
	original, err := m.SaveModbusMap(t.Context(), admin, 0, modbus.Map{SchemaVersion: 1, ID: "map", Name: "Map", Points: []modbus.Point{p}})
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Changed"
	doc, err := modbus.ExportMap(modbus.Map{SchemaVersion: 1, ID: "source", Revision: 1, OwnerID: "source", Name: "Imported", Points: []modbus.Point{p}}, "json")
	if err != nil {
		t.Fatal(err)
	}
	input := ModbusMapImport{Format: "json", Mode: "merge", TargetID: "map", ExpectedRevision: 1, Document: string(doc)}
	preview, err := m.PreviewModbusMapImport(t.Context(), admin, input)
	if err != nil || preview.CanApply || len(preview.Conflicts) != 1 || preview.Conflicts[0] != "p" {
		t.Fatal(preview, err)
	}
	if _, err = m.ImportModbusMap(t.Context(), admin, input); !errors.Is(err, ErrConflict) {
		t.Fatal("unaccepted conflict", err)
	}
	stable, err := m.ModbusMap(t.Context(), admin, "map")
	if err != nil || stable.Revision != original.Revision || stable.Points[0].Name != "Original" {
		t.Fatal("partial publication", stable, err)
	}
	input.AcceptConflicts = true
	merged, err := m.ImportModbusMap(t.Context(), admin, input)
	if err != nil || merged.Revision != 2 || merged.Points[0].Name != "Changed" {
		t.Fatal(merged, err)
	}
}
