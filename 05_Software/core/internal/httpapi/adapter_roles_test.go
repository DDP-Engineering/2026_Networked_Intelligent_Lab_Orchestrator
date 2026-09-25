package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nilo.local/nilo/internal/network"
)

func TestAdapterRoleConflictResponseIsStructuredAndSecretFree(t *testing.T) {
	response := httptest.NewRecorder()
	writeAdapterRoleConflict(response, &network.AdapterRoleConflictError{
		AdapterID: "aa:bb:cc:dd:ee:ff", Interface: "enx-device",
		ExistingRole: network.AdapterRoleDevice, RequestedRole: network.AdapterRoleManagement,
		ConflictingResourceID: "device-network-1", ConflictingResourceName: "PLC network",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error   string                           `json:"error"`
		Message string                           `json:"message"`
		Details network.AdapterRoleConflictError `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "network_interface_role_conflict" || payload.Details.AdapterID != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Details.ExistingRole != network.AdapterRoleDevice || payload.Details.RequestedRole != network.AdapterRoleManagement {
		t.Fatalf("roles=%+v", payload.Details)
	}
	if payload.Details.ConflictingResourceID != "device-network-1" || payload.Details.ConflictingResourceName != "PLC network" {
		t.Fatalf("resource=%+v", payload.Details)
	}
	for _, secret := range []string{
		"fixture-wifi-password", "fixture-private-key", "fixture-preshared-key",
		"fixture-bearer-token", "fixture-session-token",
	} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("conflict response leaked secret marker %q", secret)
		}
	}
}
