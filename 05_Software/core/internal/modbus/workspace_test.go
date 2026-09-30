package modbus

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWorkspaceCanonicalRejectsUnsafePlan(t *testing.T) {
	w := Workspace{SchemaVersion: 1, ID: "desk", Revision: 1, OwnerID: "operator", Name: "Desk", EndpointID: "peer", ProfileRef: RevisionRef{ID: "profile", Revision: 1}, MapRefs: []RevisionRef{{ID: "map", Revision: 1}}, PollGroups: []PollGroup{{ID: "group", Name: "Read", Enabled: true, UnitID: 1, Function: 3, Address: 65535, Quantity: 2, IntervalMS: 1000, MapRef: RevisionRef{ID: "map", Revision: 1}}}, Layout: Layout{AddressBase: "zero", Columns: []string{"name"}, Series: []string{}, DisplayPaused: false}}
	if _, err := w.Canonical(); err == nil {
		t.Fatal("accepted register span overflow")
	}
	w.PollGroups[0].Quantity = 1
	if _, err := w.Canonical(); err != nil {
		t.Fatalf("valid workspace: %v", err)
	}
	w.PollGroups[0].Function = 6
	if _, err := w.Canonical(); err == nil {
		t.Fatal("accepted write function")
	}
	w.PollGroups = []PollGroup{}
	w.MapRefs = []RevisionRef{}
	w.Layout.Series = []string{}
	canonical, err := w.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{[]byte(`"map_refs":null`), []byte(`"poll_groups":null`), []byte(`"series":null`)} {
		if bytes.Contains(raw, field) {
			t.Fatalf("null array in workspace: %s", raw)
		}
	}
}
