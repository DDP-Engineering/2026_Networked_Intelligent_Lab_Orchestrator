package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoveryDescriptionIsReadOnlyPublicMetadata(t *testing.T) {
	handler, err := NewDiscoveryDescriptionHandler(DiscoveryDescription{
		ApplianceID:     "0199a2d0-1234-7abc-8def-0123456789ab",
		SerialName:      "nilo-p2rjmzdkf1",
		FriendlyName:    "bench-west",
		PresentationURL: "https://nilo-p2rjmzdkf1.local:8443/",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/.well-known/nilo-device.xml", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/xml; charset=utf-8" {
		t.Fatalf("response = %d %q", response.Code, response.Header())
	}
	body := response.Body.String()
	for _, want := range []string{
		"<friendlyName>bench-west</friendlyName>",
		"<modelName>NILO Networked Intelligent Lab Orchestrator</modelName>",
		"<serialNumber>nilo-p2rjmzdkf1</serialNumber>",
		"<UDN>uuid:0199a2d0-1234-7abc-8def-0123456789ab</UDN>",
		"<presentationURL>https://nilo-p2rjmzdkf1.local:8443/</presentationURL>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("description omitted %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"<serviceList", "controlURL", "eventSubURL", "admin_password"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("description contains forbidden %q: %s", forbidden, body)
		}
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/.well-known/nilo-device.xml", strings.NewReader("x")))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", post.Code)
	}
}

func TestDiscoveryDescriptionRejectsUnsafeMetadata(t *testing.T) {
	for _, description := range []DiscoveryDescription{
		{},
		{ApplianceID: "not-uuid", SerialName: "nilo-p2rjmzdkf1", PresentationURL: "https://nilo-p2rjmzdkf1.local/"},
		{ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1", FriendlyName: "<script>", PresentationURL: "https://nilo-p2rjmzdkf1.local/"},
		{ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1", PresentationURL: "http://example.test/"},
	} {
		if handler, err := NewDiscoveryDescriptionHandler(description); err == nil || handler != nil {
			t.Fatalf("accepted unsafe description %+v", description)
		}
	}
}
