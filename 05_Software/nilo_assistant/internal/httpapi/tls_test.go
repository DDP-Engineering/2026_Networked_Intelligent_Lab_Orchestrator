package httpapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServeTLSUsesTrustedCertificateForSameOriginGUIAndAPI(t *testing.T) {
	fixture := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := fixture.TLS.Certificates[0]
	fixture.Close()
	dir := t.TempDir()
	cert := writeFile(t, dir, "server.crt", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})))
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	key := writeFile(t, dir, "server.key", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
	guiDir := t.TempDir()
	writeFile(t, guiDir, "index.html", "NILO appliance interface")
	handler, err := StaticAssets(guiDir, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ServeOptions{Address: "127.0.0.1:0", Handler: handler, ShutdownTimeout: time.Second, Certificate: cert, Key: key, OnListening: func(a net.Addr) { ready <- a }})
	}()
	var address string
	select {
	case a := <-ready:
		address = a.String()
	case e := <-done:
		t.Fatalf("server startup: %v", e)
	case <-time.After(5 * time.Second):
		t.Fatal("no listener")
	}
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for path, want := range map[string]int{"/network": 200, "/api/v1/network/status": 401, "/mcp": 401} {
		r, e := client.Get("https://" + address + path)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("%s: %d", path, r.StatusCode)
		}
	}
	plain := &http.Client{Timeout: time.Second}
	r, e := plain.Get("http://" + address + "/network")
	if e == nil {
		r.Body.Close()
		if r.StatusCode == 200 {
			t.Fatal("plaintext served GUI")
		}
	}
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTLSConfigurationFailsBeforeListening(t *testing.T) {
	for _, tc := range []struct{ address, cert, key string }{{"192.168.1.5:8443", "", ""}, {"127.0.0.1:0", "missing", ""}, {"127.0.0.1:0", "missing", "missing"}, {"0.0.0.0:8443", "missing", "missing"}} {
		called := false
		err := Serve(context.Background(), ServeOptions{Address: tc.address, Handler: http.NotFoundHandler(), ShutdownTimeout: time.Second, Certificate: tc.cert, Key: tc.key, OnListening: func(net.Addr) { called = true }})
		if err == nil || called {
			t.Fatalf("invalid configuration listened: %+v %v", tc, err)
		}
	}
}
