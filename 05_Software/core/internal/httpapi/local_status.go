package httpapi

import (
	"errors"
	"html/template"
	"net"
	"net/http"
	"strings"

	"nilo.local/nilo/internal/appliance"
)

var localStatusPage = template.Must(template.New("local-status").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>NILO appliance status</title></head><body><main>
<h1>{{.SerialName}}</h1>{{if .FriendlyName}}<p>Friendly name: {{.FriendlyName}}</p>{{end}}
<p>State: {{.State}}</p><p>Version: {{.Version}}</p><p>Management URL: <a href="{{.ManagementURL}}">{{.ManagementURL}}</a></p>
<h2>Management addresses</h2><ul>{{range .ManagementAddresses}}<li>{{.}}</li>{{end}}</ul>
<p><a href="/label.svg">Open printable label</a></p></main></body></html>`))

// ValidateLocalStatusBind accepts only a literal loopback address and port.
// Hostnames are refused so DNS or hosts-file changes cannot widen the listener.
func ValidateLocalStatusBind(address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || port == "" {
		return errors.New("local status listener must name a literal loopback address and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("local status listener must use a literal loopback address")
	}
	return nil
}

// NewLocalStatusHandler exposes public recovery facts and the shared printable
// label without authentication. Its caller must bind it only to loopback.
func NewLocalStatusHandler(metadata *ApplianceMetadata) (http.Handler, error) {
	if _, err := validateApplianceMetadata(metadata); err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, errors.New("appliance metadata is required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		snapshot := metadata.snapshot()
		if err := localStatusPage.Execute(response, &snapshot); err != nil {
			return
		}
	})
	mux.HandleFunc("GET /label.svg", func(response http.ResponseWriter, _ *http.Request) {
		snapshot := metadata.snapshot()
		label, err := appliance.RenderLabelSVG(appliance.Label{
			SerialName: snapshot.SerialName, ManagementURL: snapshot.ManagementURL, HardwareAddresses: snapshot.ManagementMACs,
		})
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "appliance_label_unavailable", "")
			return
		}
		response.Header().Set("Content-Type", "image/svg+xml")
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = response.Write(label)
	})
	return mux, nil
}
