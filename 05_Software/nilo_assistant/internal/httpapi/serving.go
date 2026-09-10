package httpapi

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PrivateListener describes an explicitly configured non-loopback listener.
//
// Nothing here is inferred. A private listener exists only when an operator
// configured one, and it always carries a certificate: deriving TLS from the
// address would produce a plaintext appliance listener the day somebody typed a
// different address, which is exactly the mistake that must not be one typo away.
type PrivateListener struct {
	Address     string
	Certificate string
	Key         string
}

// ErrPrivateListenerNeedsTLS is returned when a private listener is configured
// without a certificate and key.
var ErrPrivateListenerNeedsTLS = errors.New("a private listener requires a certificate and key")

// ValidateManagementListener permits a wildcard listener only when TLS and a
// non-nil interface gate are both present. This explicit contract replaces the
// literal-private-address contract used by legacy --bind mode.
func ValidateManagementListener(address, certificate, key string, gate ManagementConnectionGate) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || port == "" {
		return errors.New("management listener must be a wildcard host and explicit port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsUnspecified() {
		return errors.New("interface-managed listener must use a wildcard host")
	}
	if gate == nil {
		return errors.New("wildcard management listener requires an interface gate")
	}
	if certificate == "" || key == "" {
		return ErrPrivateListenerNeedsTLS
	}
	for name, path := range map[string]string{"certificate": certificate, "key": key} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("read the %s: %w", name, err)
		}
	}
	return nil
}

// ValidateCertificateNames reports stable DNS or IP identities absent from the
// leaf certificate. It never generates or replaces a production certificate.
func ValidateCertificateNames(path string, required []string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read listener certificate: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("listener certificate is not PEM encoded")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse listener certificate: %w", err)
	}
	missing := make([]string, 0)
	for _, name := range required {
		if err := certificate.VerifyHostname(name); err != nil {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

// ValidatePrivateListener accepts only a private, non-loopback address with a
// readable certificate and key.
//
// A public address is refused outright. This milestone serves an appliance on a
// bench network; a listener reachable from the internet is not a configuration
// this service knows how to defend, so it is rejected rather than served.
func ValidatePrivateListener(listener PrivateListener) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listener.Address))
	if err != nil {
		return fmt.Errorf("private listener address must be host:port: %w", err)
	}
	if port == "" {
		return errors.New("private listener address must name a port")
	}

	address := net.ParseIP(host)
	if address == nil {
		return fmt.Errorf("private listener host %q must be a literal IP address", host)
	}
	if address.IsLoopback() {
		return errors.New("a loopback address is the development default, not a private listener")
	}
	if !address.IsPrivate() && !address.IsLinkLocalUnicast() {
		return fmt.Errorf("private listener host %q is not a private address", host)
	}

	if listener.Certificate == "" || listener.Key == "" {
		return ErrPrivateListenerNeedsTLS
	}
	for name, path := range map[string]string{"certificate": listener.Certificate, "key": listener.Key} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("read the %s: %w", name, err)
		}
	}
	return nil
}

// StaticAssets serves the built browser interface from the same origin as the
// API, falling back to the single-page entry point for a route the interface
// owns rather than the server.
//
// Same-origin is not a convenience: an Authorization header is not
// CORS-safelisted, so a cross-origin interface would have every authenticated
// request preflighted, and Bench Core answers no preflight.
func StaticAssets(root string, api http.Handler, runtimeProfile ...string) (http.Handler, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("a static asset root is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("read the static asset root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	files := http.FileServer(http.Dir(root))
	entry := filepath.Join(root, "index.html")
	if info, err := os.Stat(entry); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("static assets require an index.html file")
	}

	var bootstrap []byte
	if len(runtimeProfile) > 1 {
		return nil, errors.New("only one runtime profile is allowed")
	}
	if len(runtimeProfile) == 1 {
		profile := runtimeProfile[0]
		if profile != "appliance" && profile != "simulation" {
			return nil, errors.New("unsupported GUI runtime profile")
		}
		raw, err := os.ReadFile(entry)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(raw, []byte("</head>")) {
			return nil, errors.New("GUI entry requires a head element for runtime metadata")
		}
		meta := []byte(`<meta name="nilo-runtime-profile" content="` + profile + `"></head>`)
		bootstrap = bytes.Replace(raw, []byte("</head>"), meta, 1)
	}

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		// Anything under /api belongs to the API, always. A static file placed
		// at that path must never be able to answer in its place.
		if strings.HasPrefix(request.URL.Path, "/api/") || request.URL.Path == "/mcp" {
			api.ServeHTTP(response, request)
			return
		}
		candidate := filepath.Join(root, filepath.Clean("/"+request.URL.Path))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && (bootstrap == nil || candidate != entry) {
			files.ServeHTTP(response, request)
			return
		}
		if bootstrap != nil {
			response.Header().Set("Cache-Control", "no-store")
			http.ServeContent(response, request, "index.html", time.Time{}, bytes.NewReader(bootstrap))
			return
		}
		http.ServeFile(response, request, entry)
	}), nil
}

// ValidateListener preserves plaintext loopback development while requiring a
// complete TLS configuration for every explicitly configured private address.
func ValidateListener(address, certificate, key string) error {
	if (certificate == "") != (key == "") {
		return ErrPrivateListenerNeedsTLS
	}
	if err := ValidateSimulationBind(address); err == nil {
		return nil
	}
	return ValidatePrivateListener(PrivateListener{Address: address, Certificate: certificate, Key: key})
}
