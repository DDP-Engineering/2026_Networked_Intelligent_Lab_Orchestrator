// Package apiclient speaks the authenticated Bench Core HTTP API.
//
// It exists so the command line is an API client like any other caller, even
// when it runs on the appliance itself. A local command that reached past the
// HTTP boundary would be a second authorization path with different rules, and
// a shell user on the appliance would quietly get more than the same person
// gets through the browser.
package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// maxResponseBytes bounds what one answer may be. These are small documents;
// anything larger is a mistake or an attempt to exhaust the caller.
const maxResponseBytes = 4 << 20

// Error is a request the server refused, carrying its stable code and the
// readable message that came with it. The code is what a script branches on;
// the message is what the appliance wants the operator to read, and repeating
// it here is what keeps a rule the appliance owns from being restated by every
// client that reports it refused.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (err *Error) Error() string {
	refusal := fmt.Sprintf("the appliance refused the request (HTTP %d)", err.Status)
	if err.Code != "" {
		refusal = fmt.Sprintf("the appliance refused the request: %s (HTTP %d)", err.Code, err.Status)
	}
	if err.Message == "" {
		return refusal
	}
	return refusal + ": " + err.Message
}

// Options configure one client.
type Options struct {
	// BaseURL is the appliance's API origin.
	BaseURL string
	// TokenFile is the only place a token is ever read from. There is
	// deliberately no Token field: a value here would invite a caller to pass
	// one on a command line, where every other process can read it.
	TokenFile string
	// CAFile optionally pins the certificate authority for a private listener.
	CAFile  string
	Timeout time.Duration
}

// Client calls the network API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New reads the token file and builds a client. A missing or empty token file
// is an error rather than an anonymous request, because an unauthenticated call
// would fail later with a message about permissions instead of about the file.
func New(options Options) (*Client, error) {
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, errors.New("the appliance address is required: pass --server")
	}
	if strings.TrimSpace(options.TokenFile) == "" {
		return nil, errors.New("an API token file is required: pass --token-file")
	}

	raw, err := os.ReadFile(options.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("read the API token file: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return nil, fmt.Errorf("the API token file %s is empty", options.TokenFile)
	}

	transport := &http.Transport{}
	if options.CAFile != "" {
		authority, err := os.ReadFile(options.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read the certificate authority file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(authority) {
			return nil, fmt.Errorf("%s holds no certificate", options.CAFile)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(options.BaseURL, "/"),
		token:   token,
		http:    &http.Client{Transport: transport, Timeout: timeout},
	}, nil
}

// Do performs one request and returns the raw response document.
//
// The token is only ever an Authorization header value. It never reaches the
// URL, where it would land in server logs and shell history.
func (client *Client) Do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, payload)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach the appliance: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	document, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read the response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code, message := refusalOf(document)
		return nil, &Error{Status: response.StatusCode, Code: code, Message: message}
	}
	return document, nil
}

// refusalOf reads the stable code and the readable message from a refusal, if
// it carries them.
func refusalOf(document []byte) (string, string) {
	var refusal struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(document, &refusal); err != nil {
		return "", ""
	}
	return refusal.Error, refusal.Message
}

// Stream performs one GET and hands back the response body unread, for a
// document that is meant to be copied rather than decoded: an export. The
// caller closes it. A refusal is read in full and reported as an Error.
func (client *Client) Stream(ctx context.Context, path string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach the appliance: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer func() { _ = response.Body.Close() }()
		document, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
		code, message := refusalOf(document)
		return nil, &Error{Status: response.StatusCode, Code: code, Message: message}
	}
	return response.Body, nil
}
