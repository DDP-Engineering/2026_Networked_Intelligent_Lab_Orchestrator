// Package mcp serves NILO's read-only Model Context Protocol endpoint.
//
// It exposes three reads and nothing else. That property is enforced here, in
// what the handlers can reach, rather than by the read-only annotation the
// protocol offers: an annotation is a label a client may ignore, and the
// guarantee has to hold in the implementation.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/network"
)

// ToolNames is the complete set of tools this endpoint offers, in the order it
// registers them. A test asserts discovery returns exactly this.
var ToolNames = []string{
	"network_get_status",
	"network_get_statistics",
	"network_get_wireguard_status",
}

// reader is the narrow view of the Network service this endpoint is given.
//
// It is an interface rather than the service itself so that the endpoint holds
// no reference to a mutating method. There is nothing here to call that could
// change the appliance.
type reader interface {
	Status(ctx context.Context) (network.Status, error)
	Statistics(ctx context.Context, interfaceID string, period network.Period) (network.Statistics, error)
	WireGuard(ctx context.Context) (network.WireGuardStatus, error)
}

// HandlerOptions carries what the endpoint needs.
type HandlerOptions struct {
	Identity *identity.Service
	Network  reader
	// AllowedOrigins are the Origin header values this endpoint accepts. A
	// request carrying any other Origin is refused: a browser that was told to
	// call this endpoint from a page we do not serve is not a client we answer.
	AllowedOrigins []string
}

type statusInput struct{}

type statisticsInput struct {
	Interface string `json:"interface" jsonschema:"the stable interface identifier"`
	Period    string `json:"period" jsonschema:"one of 15m, 1h or 24h"`
}

type wireguardInput struct{}

// NewHandler builds the authenticated read-only MCP endpoint.
func NewHandler(options HandlerOptions) (http.Handler, error) {
	if options.Identity == nil {
		return nil, errors.New("MCP endpoint requires the identity service")
	}
	if options.Network == nil {
		return nil, errors.New("MCP endpoint requires the network service")
	}

	server := sdk.NewServer(&sdk.Implementation{Name: "nilo", Version: "1"}, nil)

	sdk.AddTool(server, &sdk.Tool{
		Name:        ToolNames[0],
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
		Description: "Read the appliance's network interfaces and their current state.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ statusInput) (*sdk.CallToolResult, network.Status, error) {
		status, err := options.Network.Status(ctx)
		if err != nil {
			return nil, network.Status{}, fmt.Errorf("read network status: %w", err)
		}
		return nil, status, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        ToolNames[1],
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
		Description: "Read one interface's traffic over a 15m, 1h or 24h window.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, input statisticsInput) (*sdk.CallToolResult, network.Statistics, error) {
		period, err := network.ParsePeriod(input.Period)
		if err != nil {
			return nil, network.Statistics{}, err
		}
		statistics, err := options.Network.Statistics(ctx, input.Interface, period)
		if err != nil {
			return nil, network.Statistics{}, fmt.Errorf("read traffic history: %w", err)
		}
		return nil, statistics, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        ToolNames[2],
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
		Description: "Read WireGuard readiness on the appliance.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ wireguardInput) (*sdk.CallToolResult, network.WireGuardStatus, error) {
		status, err := options.Network.WireGuard(ctx)
		return nil, status, err
	})

	streamable := sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server }, nil)
	return authorize(options, streamable), nil
}

// authorize gates every MCP request on a valid, unrevoked bearer token holding
// the network read permission, and on an acceptable Origin.
func authorize(options HandlerOptions, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(options.AllowedOrigins))
	for _, origin := range options.AllowedOrigins {
		allowed[strings.ToLower(strings.TrimSpace(origin))] = true
	}

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		// An Origin is present only on a browser-issued request. When it is
		// there it must be one we serve; a configured token client sends none.
		if origin := request.Header.Get("Origin"); origin != "" {
			if !allowed[strings.ToLower(origin)] {
				refuse(response, http.StatusForbidden, "forbidden")
				return
			}
		}

		token, present := bearerToken(request.Header.Get("Authorization"))
		if !present {
			refuse(response, http.StatusUnauthorized, "unauthorized")
			return
		}
		actor, err := options.Identity.AuthenticateToken(request.Context(), token)
		if err != nil {
			// Missing, unknown, expired, revoked and disabled-owner all answer
			// the same way, so nothing here reports which accounts exist.
			refuse(response, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !actor.Has(auth.PermissionNetworkRead) {
			refuse(response, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func refuse(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/json")
	if status == http.StatusUnauthorized {
		response.Header().Set("WWW-Authenticate", "Bearer")
	}
	response.WriteHeader(status)
	_, _ = response.Write([]byte(`{"error":"` + code + `"}`))
}
