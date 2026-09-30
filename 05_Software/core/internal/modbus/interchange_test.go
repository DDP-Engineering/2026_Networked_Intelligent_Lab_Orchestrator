package modbus

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMapInterchangeRoundTripAndRows(t *testing.T) {
	p := testPoint("uint16", 1)
	p.Name = "=SUM(1,2)"
	p.Description = "quoted, description"
	m := Map{SchemaVersion: 1, ID: "map", Revision: 1, OwnerID: "alice", Name: "=Map", Points: []Point{p}}
	for _, format := range []string{"json", "csv"} {
		data, err := ExportMap(m, format)
		if err != nil {
			t.Fatal(format, err)
		}
		if format == "csv" && !strings.Contains(string(data), "~=Map") {
			t.Fatal("CSV formula not escaped")
		}
		got, issues := ParseMapDocument(format, data)
		if len(issues) != 0 || got.Name != m.Name || len(got.Points) != 1 || got.Points[0].Name != p.Name || got.Points[0].Description != p.Description {
			t.Fatalf("%s: %+v %+v", format, got, issues)
		}
	}
	data, _ := json.Marshal(m)
	invalid := append(data, []byte(" trailing")...)
	if _, issues := ParseMapDocument("json", invalid); len(issues) == 0 {
		t.Fatal("accepted trailing data")
	}
}

func TestMapCSVAllFieldsAndMultiRowErrors(t *testing.T) {
	p := testPoint("enum", 1)
	p.Labels = map[string]string{"1": "=On"}
	p.Unit = "+rpm"
	p.Description = "line, with comma"
	p.Precision = new(int)
	*p.Precision = 2
	low, high := json.Number("0.5"), json.Number("100.5")
	p.WriteMin = &low
	p.WriteMax = &high
	m := Map{SchemaVersion: 1, ID: "map", Revision: 4, OwnerID: "owner", Name: "Map", Points: []Point{p}}
	data, err := ExportMap(m, "csv")
	if err != nil {
		t.Fatal(err)
	}
	got, issues := ParseMapDocument("csv", data)
	if len(issues) != 0 || !reflect.DeepEqual(got, m) {
		t.Fatalf("round trip %+v %+v", got, issues)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	badRow := strings.Replace(lines[1], ",zero,0,", ",one,0,", 1)
	invalid := lines[0] + "\n" + badRow + "\n" + badRow + "\n"
	_, issues = ParseMapDocument("csv", []byte(invalid))
	if len(issues) != 2 {
		t.Fatalf("want both row errors: %+v", issues)
	}
	for _, issue := range issues {
		if issue.Field != "address" {
			t.Fatalf("wrong error field: %+v", issues)
		}
	}
}

func TestEmptyMapCSVMetadataRoundTrip(t *testing.T) {
	item := Map{SchemaVersion: 1, ID: "empty", Revision: 3, OwnerID: "owner", Name: "Empty", Points: []Point{}}
	data, err := ExportMap(item, "csv")
	if err != nil {
		t.Fatal(err)
	}
	got, issues := ParseMapDocument("csv", data)
	if len(issues) != 0 || got.ID != item.ID || got.Revision != item.Revision || got.OwnerID != item.OwnerID || got.Name != item.Name || len(got.Points) != 0 {
		t.Fatalf("empty map: %+v %+v", got, issues)
	}
}

func TestMapJSONRequiresCompletePointContract(t *testing.T) {
	p := testPoint("uint16", 1)
	item := Map{SchemaVersion: 1, ID: "map", Revision: 1, OwnerID: "owner", Name: "Map", Points: []Point{p}}
	raw, err := ExportMap(item, "json")
	if err != nil {
		t.Fatal(err)
	}
	missing := strings.Replace(string(raw), `"write_min":null,`, "", 1)
	if _, issues := ParseMapDocument("json", []byte(missing)); len(issues) == 0 {
		t.Fatal("accepted missing required write_min")
	}
	invalid := strings.Replace(string(raw), `"labels":{}`, `"labels":null`, 1)
	if _, issues := ParseMapDocument("json", []byte(invalid)); len(issues) == 0 {
		t.Fatal("accepted null labels")
	}
}
