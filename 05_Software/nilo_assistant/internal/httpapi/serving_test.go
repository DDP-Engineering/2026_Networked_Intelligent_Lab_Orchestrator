package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// A private listener is configured deliberately or it does not exist. Every way
// of getting it slightly wrong is refused rather than served insecurely.
func TestValidatePrivateListenerAcceptsOnlyAConfiguredPrivateTLSListener(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certificate := writeFile(t, dir, "bench.crt", "certificate")
	key := writeFile(t, dir, "bench.key", "key")

	if err := ValidatePrivateListener(PrivateListener{
		Address: "192.168.10.24:8443", Certificate: certificate, Key: key,
	}); err != nil {
		t.Fatalf("a private address with a certificate was refused: %v", err)
	}

	for _, test := range []struct {
		name     string
		listener PrivateListener
	}{
		{
			name:     "no certificate at all",
			listener: PrivateListener{Address: "192.168.10.24:8443"},
		},
		{
			name:     "a key without a certificate",
			listener: PrivateListener{Address: "192.168.10.24:8443", Key: key},
		},
		{
			name: "a certificate that is not there",
			listener: PrivateListener{
				Address: "192.168.10.24:8443", Certificate: filepath.Join(dir, "absent"), Key: key,
			},
		},
		{
			name: "a public address",
			listener: PrivateListener{
				Address: "203.0.113.7:8443", Certificate: certificate, Key: key,
			},
		},
		{
			name: "loopback, which is the development default rather than this",
			listener: PrivateListener{
				Address: "127.0.0.1:8443", Certificate: certificate, Key: key,
			},
		},
		{
			name: "a hostname rather than a literal address",
			listener: PrivateListener{
				Address: "appliance.local:8443", Certificate: certificate, Key: key,
			},
		},
		{
			name:     "no port",
			listener: PrivateListener{Address: "192.168.10.24", Certificate: certificate, Key: key},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidatePrivateListener(test.listener); err == nil {
				t.Fatalf("ValidatePrivateListener accepted %s", test.name)
			}
		})
	}
}

// A private listener without TLS is refused with a reason that says so, rather
// than being served in plaintext.
func TestAPrivateListenerWithoutTLSIsRefusedByName(t *testing.T) {
	t.Parallel()

	err := ValidatePrivateListener(PrivateListener{Address: "10.0.0.5:8443"})
	if !errors.Is(err, ErrPrivateListenerNeedsTLS) {
		t.Fatalf("error = %v, want ErrPrivateListenerNeedsTLS", err)
	}
}

// The development default is unchanged: loopback only, and nothing else binds.
func TestTheDevelopmentBindStillAcceptsOnlyLoopback(t *testing.T) {
	t.Parallel()

	if err := ValidateSimulationBind("127.0.0.1:8090"); err != nil {
		t.Fatalf("loopback bind was refused: %v", err)
	}
	for _, address := range []string{"0.0.0.0:8090", "192.168.10.24:8090", "203.0.113.7:8090"} {
		if err := ValidateSimulationBind(address); err == nil {
			t.Fatalf("ValidateSimulationBind accepted %s", address)
		}
	}
}

// The interface and the API answer on one origin, which is what keeps an
// authenticated request from being preflighted.
func TestStaticAssetsServeTheInterfaceAndTheAPIFromOneOrigin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "index.html", "<!doctype html><title>NILO</title>")
	writeFile(t, root, "app.js", "console.log('nilo')")

	api := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTeapot)
	})
	handler, err := StaticAssets(root, api)
	if err != nil {
		t.Fatalf("StaticAssets: %v", err)
	}

	for _, test := range []struct {
		path string
		want int
		body string
	}{
		{path: "/", want: http.StatusOK, body: "NILO"},
		{path: "/app.js", want: http.StatusOK, body: "nilo"},
		// A route the interface owns falls back to the entry point rather than
		// answering 404 from the server.
		{path: "/network", want: http.StatusOK, body: "NILO"},
		// Anything under /api is the API's, always.
		{path: "/api/v1/network/status", want: http.StatusTeapot},
		{path: "/mcp", want: http.StatusTeapot},
	} {
		test := test
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.want {
				t.Fatalf("%s = %d, want %d", test.path, response.Code, test.want)
			}
			if test.body != "" && !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("%s body = %q, want it to contain %q", test.path, response.Body.String(), test.body)
			}
		})
	}
}

// A static file placed at an API path must never answer in the API's place.
func TestAStaticFileCannotShadowAnAPIRoute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "index.html", "<!doctype html>")
	if err := os.MkdirAll(filepath.Join(root, "api", "v1"), 0o700); err != nil {
		t.Fatalf("create api directory: %v", err)
	}
	writeFile(t, filepath.Join(root, "api", "v1"), "status", `{"status":"forged"}`)

	api := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTeapot)
	})
	handler, err := StaticAssets(root, api)
	if err != nil {
		t.Fatalf("StaticAssets: %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if response.Code != http.StatusTeapot {
		t.Fatalf("a static file answered an API route: %d %s", response.Code, response.Body.String())
	}
}

func TestStaticAssetsRefusesAnUnusableRoot(t *testing.T) {
	t.Parallel()

	api := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, root := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		if _, err := StaticAssets(root, api); err == nil {
			t.Fatalf("StaticAssets accepted the root %q", root)
		}
	}
}

func TestStaticAssetsSupplySelectedRuntimeBeforeAuthentication(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html", "<!doctype html><html><head></head><body>NILO</body></html>")
	handler, err := StaticAssets(root, http.NotFoundHandler(), "appliance")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/index.html", "/network"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if !strings.Contains(r.Body.String(), `<meta name="nilo-runtime-profile" content="appliance">`) {
			t.Fatalf("missing runtime bootstrap on %s: %s", path, r.Body.String())
		}
	}
}
