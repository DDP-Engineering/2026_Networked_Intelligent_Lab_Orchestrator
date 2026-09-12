package sqlite

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestManagementAccessRepositoryPersistsRecoveryWithoutSecrets(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagementAccessRepository(database)
	if _, present, err := repository.LoadManagementAccess(context.Background()); err != nil || present {
		t.Fatalf("empty repository = present %t, error %v", present, err)
	}
	record := network.ManagementAccessRecord{
		Configuration: network.ManagementAccessConfiguration{
			Interfaces: []string{"wlan0", "nilo0"}, Port: 8443,
			Bindings: []network.ManagementInterfaceBinding{{AdapterID: "wifi-adapter", LastKnownName: "wlan0"}, {LastKnownName: "nilo0"}},
			TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
		},
		LastKnownGood: network.ManagementAccessConfiguration{
			Interfaces: []string{"wlan0"}, Bindings: []network.ManagementInterfaceBinding{{AdapterID: "wifi-adapter", LastKnownName: "wlan0"}},
			Port: 8443, TLSNames: []string{"ddp-remote.local"},
		},
		State:     network.ManagementAccessPending,
		ConfirmBy: time.Date(2026, 9, 8, 10, 2, 0, 0, time.UTC),
		Detail:    "console management reconfiguration requires confirmation",
		Outcome: network.ManagementAccessOutcome{
			Code: network.ManagementOutcomeConsolePending, Interface: "wlan0",
		},
		UpdatedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
	}
	if err := repository.SaveManagementAccess(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, present, err := repository.LoadManagementAccess(context.Background())
	if err != nil || !present || !reflect.DeepEqual(loaded, record) {
		t.Fatalf("loaded = %+v, present=%t, error=%v; want %+v", loaded, present, err, record)
	}
	var raw string
	if err := database.SQL().QueryRow("SELECT interfaces_json || tls_names_json || detail FROM management_access").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"wifi-password", "wireguard-private-key", "preshared-key"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("stored management access leaked %q", secret)
		}
	}
}

func TestManagementAccessRepositoryMigratesLegacyNamesToBindings(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagementAccessRepository(database)
	updated := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	_, err := database.SQL().Exec(`
		INSERT INTO management_access (
			singleton, interfaces_json, port, tls_names_json,
			last_good_interfaces_json, last_good_port, last_good_tls_names_json,
			state, confirm_by, detail, updated_at
		) VALUES (1, ?, 8443, ?, ?, 8443, ?, ?, '', '', ?)
	`, `["wlan0","nilo0"]`, `["ddp-remote.local"]`, `["wlan0"]`,
		`["ddp-remote.local"]`, string(network.ManagementAccessApplied), updated.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	loaded, present, err := repository.LoadManagementAccess(context.Background())
	if err != nil || !present {
		t.Fatalf("load legacy record: present=%t error=%v", present, err)
	}
	want := []network.ManagementInterfaceBinding{
		{LastKnownName: "wlan0"},
		{LastKnownName: "nilo0"},
	}
	if !reflect.DeepEqual(loaded.Configuration.Bindings, want) {
		t.Fatalf("legacy bindings=%+v, want %+v", loaded.Configuration.Bindings, want)
	}
	if err := repository.SaveManagementAccess(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := database.SQL().QueryRow(`SELECT interfaces_json FROM management_access WHERE singleton = 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"last_known_name":"wlan0"`) || strings.Contains(raw, `["wlan0"`) {
		t.Fatalf("legacy names were not rewritten as bindings: %s", raw)
	}
}

func TestManagementAccessRepositoryPersistsStableAdapterBindings(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagementAccessRepository(database)
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"enx-old", "nilo0"},
		Bindings: []network.ManagementInterfaceBinding{
			{AdapterID: "aa:bb:cc:dd:ee:ff", LastKnownName: "enx-old"},
			{LastKnownName: "nilo0"},
		},
		Port: 8443, TLSNames: []string{"ddp-remote.local"},
	}
	record := network.ManagementAccessRecord{
		Configuration: configuration, LastKnownGood: configuration,
		State: network.ManagementAccessApplied, UpdatedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
	}
	if err := repository.SaveManagementAccess(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, present, err := repository.LoadManagementAccess(context.Background())
	if err != nil || !present {
		t.Fatalf("load stable record: present=%t error=%v", present, err)
	}
	if !reflect.DeepEqual(loaded.Configuration.Bindings, configuration.Bindings) {
		t.Fatalf("loaded bindings=%+v, want %+v", loaded.Configuration.Bindings, configuration.Bindings)
	}
}

func TestManagementAccessRepositoryRejectsUnknownOutcomeCodes(t *testing.T) {
	database := openTemporaryDatabase(t)
	repository := NewManagementAccessRepository(database)
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"wlan0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"},
	}
	record := network.ManagementAccessRecord{
		Configuration: configuration, LastKnownGood: configuration,
		State: network.ManagementAccessApplied, Outcome: network.ManagementAccessOutcome{Code: "untrusted_code"},
		UpdatedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
	}
	if err := repository.SaveManagementAccess(context.Background(), record); err == nil || !strings.Contains(err.Error(), "invalid management access outcome") {
		t.Fatalf("save unknown outcome error=%v", err)
	}
}
