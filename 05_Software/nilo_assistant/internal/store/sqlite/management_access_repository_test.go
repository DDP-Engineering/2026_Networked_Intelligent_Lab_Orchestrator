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
			TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
		},
		LastKnownGood: network.ManagementAccessConfiguration{
			Interfaces: []string{"wlan0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"},
		},
		State:     network.ManagementAccessPending,
		ConfirmBy: time.Date(2026, 9, 8, 10, 2, 0, 0, time.UTC),
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
