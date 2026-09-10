package httpapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

type addressGate map[netip.Addr]bool

func (gate addressGate) Allows(address netip.Addr) bool { return gate[address.Unmap()] }

func managementCertificate(t *testing.T) (string, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := server.TLS.Certificates[0]
	server.Close()
	directory := t.TempDir()
	cert := writeFile(t, directory, "management.crt", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})))
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key := writeFile(t, directory, "management.key", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
	return cert, key
}

func TestManagementListenerRejectsDisallowedInterfaceBeforeApplicationHandler(t *testing.T) {
	cert, key := managementCertificate(t)
	for _, test := range []struct {
		name    string
		allowed bool
	}{
		{name: "allowed destination", allowed: true},
		{name: "disallowed destination", allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := make(chan net.Addr, 1)
			done := make(chan error, 1)
			var handled atomic.Int32
			gate := addressGate{netip.MustParseAddr("127.0.0.1"): test.allowed}
			go func() {
				done <- Serve(ctx, ServeOptions{
					Address: "0.0.0.0:0", Certificate: cert, Key: key, ManagementGate: gate,
					Handler: http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
						handled.Add(1)
						response.WriteHeader(http.StatusNoContent)
					}),
					ShutdownTimeout: time.Second, OnListening: func(address net.Addr) { ready <- address },
				})
			}()
			var port string
			select {
			case address := <-ready:
				_, port, _ = net.SplitHostPort(address.String())
			case err := <-done:
				t.Fatalf("Serve: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("listener did not start")
			}
			client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}}, Timeout: 500 * time.Millisecond} //nolint:gosec -- test certificate
			response, requestErr := client.Get("https://127.0.0.1:" + port + "/api/v1/status")
			if response != nil {
				response.Body.Close()
			}
			if test.allowed {
				if requestErr != nil || handled.Load() != 1 {
					t.Fatalf("allowed request error=%v handled=%d", requestErr, handled.Load())
				}
			} else if requestErr == nil || handled.Load() != 0 {
				t.Fatalf("disallowed request error=%v handled=%d", requestErr, handled.Load())
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("listener did not stop")
			}
		})
	}
}

func TestManagementRequestGateRechecksAConnectionAfterAllowlistChange(t *testing.T) {
	var handled atomic.Int32
	gate := addressGate{}
	handler := managementRequestGate(gate, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handled.Add(1) }))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), localAddressContextKey{}, &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8443}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || handled.Load() != 0 {
		t.Fatalf("disallowed keep-alive request status=%d handled=%d", response.Code, handled.Load())
	}
}
