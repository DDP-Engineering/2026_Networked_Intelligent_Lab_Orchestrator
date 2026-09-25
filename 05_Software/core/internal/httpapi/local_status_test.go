package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalStatusIsLoopbackOnlyReadOnlyAndUsesSharedLabel(t *testing.T) {
	metadata := &ApplianceMetadata{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1", State: "provisioning", Version: "0.8.0",
		ManagementURL: "https://nilo-p2rjmzdkf1.local:8443/", ManagementAddresses: []string{"192.168.247.1"}, ManagementMACs: []string{"02:00:00:00:00:03"},
	}
	handler, err := NewLocalStatusHandler(metadata)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), metadata.SerialName) || strings.Contains(response.Body.String(), "password") {
		t.Fatalf("status page = %d %s", response.Code, response.Body.String())
	}
	metadata.mu.Lock()
	metadata.State = "commissioned"
	metadata.FriendlyName = "bench-west"
	metadata.ManagementAddresses = []string{"10.0.0.42"}
	metadata.mu.Unlock()
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(response.Body.String(), "bench-west") || !strings.Contains(response.Body.String(), "10.0.0.42") || !strings.Contains(response.Body.String(), "commissioned") {
		t.Fatalf("status page retained startup metadata: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/label.svg", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<svg`) {
		t.Fatalf("label = %d %s", response.Code, response.Body.String())
	}
	if err := ValidateLocalStatusBind("127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"0.0.0.0:8080", "192.168.1.2:8080", "localhost:8080"} {
		if err := ValidateLocalStatusBind(unsafe); err == nil {
			t.Fatalf("unsafe local status bind %q accepted", unsafe)
		}
	}
}
