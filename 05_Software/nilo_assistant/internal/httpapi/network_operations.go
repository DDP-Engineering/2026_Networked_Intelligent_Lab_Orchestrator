package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
)

// maxOperationBodyBytes bounds a management request body. These bodies carry an
// action name and a hex revision and nothing else, so anything larger is a
// mistake or an attack rather than a legitimate request.
const maxOperationBodyBytes = 64 << 10

// actionPermissions maps each managed action to the permission that authorizes
// it. Authorization lives here beside the route table, not inside the network
// package, so one file still answers "what may an actor do".
//
// The preview routes declare their permission statically. Acknowledge, confirm
// and rollback cannot, because the action is only known once the operation is
// looked up — so they declare the read permission at the route and check this
// table against the resolved action before doing anything.
var actionPermissions = map[network.Action]auth.Permission{
	network.ActionInterfaceDisable:     auth.PermissionNetworkManage,
	network.ActionInterfaceEnable:      auth.PermissionNetworkManage,
	network.ActionVPNActivate:          auth.PermissionVPNManage,
	network.ActionVPNDeactivate:        auth.PermissionVPNManage,
	network.ActionProfileImport:        auth.PermissionVPNManage,
	network.ActionProfileRemove:        auth.PermissionVPNManage,
	network.ActionAutostartSelect:      auth.PermissionVPNManage,
	network.ActionProfileRename:        auth.PermissionVPNManage,
	network.ActionWireGuardInstall:     auth.PermissionVPNInstall,
	network.ActionInterfaceSettings:    auth.PermissionNetworkManage,
	network.ActionWiFiJoin:             auth.PermissionNetworkManage,
	network.ActionDeviceNetworkCreate:  auth.PermissionNetworkManage,
	network.ActionDeviceNetworkUpdate:  auth.PermissionNetworkManage,
	network.ActionDeviceNetworkEnable:  auth.PermissionNetworkManage,
	network.ActionDeviceNetworkDisable: auth.PermissionNetworkManage,
	network.ActionDeviceNetworkDelete:  auth.PermissionNetworkManage,
}

// interfaceActions are the two actions the interface preview route offers,
// spelled the short way a caller sends them.
var interfaceActions = map[string]network.Action{
	"disable": network.ActionInterfaceDisable,
	"enable":  network.ActionInterfaceEnable,
}

// wireguardActions are the actions the WireGuard preview route offers.
var wireguardActions = map[string]network.Action{
	"activate":   network.ActionVPNActivate,
	"deactivate": network.ActionVPNDeactivate,
	"import":     network.ActionProfileImport,
	"remove":     network.ActionProfileRemove,
	"autostart":  network.ActionAutostartSelect,
	"rename":     network.ActionProfileRename,
}

type operationResponse struct {
	APIVersion string                 `json:"api_version"`
	ID         string                 `json:"id"`
	Action     network.Action         `json:"action"`
	Target     string                 `json:"target"`
	State      network.OperationState `json:"state"`
	Revision   string                 `json:"revision"`
	ActorID    string                 `json:"actor_id"`

	CreatedAt time.Time `json:"created_at"`
	// ConfirmDeadline is omitted for an action that needs no confirmation, so a
	// caller never sees a countdown that does not apply.
	ConfirmDeadline *time.Time `json:"confirm_deadline,omitempty"`
	Failure         string     `json:"failure,omitempty"`
	Progress        string     `json:"progress,omitempty"`
}

func operationPayload(operation network.Operation) operationResponse {
	payload := operationResponse{
		APIVersion: apiVersion, ID: operation.ID, Action: operation.Action,
		Target: operation.Target, State: operation.State,
		Revision: operation.Revision, CreatedAt: operation.CreatedAt,
		Failure: operation.Failure, Progress: operation.Progress, ActorID: operation.ActorID,
	}
	if !operation.ConfirmDeadline.IsZero() {
		deadline := operation.ConfirmDeadline
		payload.ConfirmDeadline = &deadline
	}
	return payload
}

// decodeOperationRequest reads a bounded JSON body, rejecting unknown fields so
// a misspelled parameter is refused rather than silently ignored.
func decodeOperationRequest(request *http.Request, into any) error {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxOperationBodyBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxOperationBodyBytes {
		return errors.New("network request too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("network request must contain one JSON object")
	}
	return nil
}

// handlePreviewInterfaceChange proposes enabling or disabling one interface.
func handlePreviewInterfaceChange(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Action       string `json:"action"`
			ConnectionID string `json:"connection_id"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		action, offered := interfaceActions[body.Action]
		if !offered {
			writeError(response, http.StatusBadRequest, "network_action_unsupported", "")
			return
		}
		previewChange(response, request, manager, network.Change{Action: action, Target: request.PathValue("id"), ConnectionID: body.ConnectionID})
	}
}

// handlePreviewWireGuardChange proposes a tunnel or profile change.
func handlePreviewWireGuardChange(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Action      string `json:"action"`
			Target      string `json:"target"`
			ProfileText string `json:"profile_text"`
			Enabled     *bool  `json:"enabled"`
			Name        string `json:"name"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		action, offered := wireguardActions[body.Action]
		if !offered {
			writeError(response, http.StatusBadRequest, "network_action_unsupported", "")
			return
		}
		if action == network.ActionAutostartSelect && body.Enabled == nil {
			writeError(response, http.StatusBadRequest, "network_enabled_required", "")
			return
		}
		if action == network.ActionProfileRename && body.Name == "" {
			writeError(response, http.StatusBadRequest, "network_name_required", "")
			return
		}
		change := network.Change{Action: action, Target: body.Target, ProfileText: body.ProfileText, Name: body.Name}
		if body.Enabled != nil {
			change.Enabled = *body.Enabled
		}
		if action == network.ActionVPNDeactivate && change.Target == "" {
			change.Target = "active"
		}
		previewChange(response, request, manager, change)
	}
}

// handlePreviewInterfaceSettings proposes a new IPv4 configuration for one
// adapter. The adapter identity is resolved by the provider from the target,
// never taken from the caller.
func handlePreviewInterfaceSettings(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Method              string   `json:"method"`
			Address             string   `json:"address"`
			Gateway             string   `json:"gateway"`
			DNS                 []string `json:"dns"`
			DNSAutomatic        bool     `json:"dns_automatic"`
			ManagementProtected bool     `json:"management_protected"`
			NeverDefault        bool     `json:"never_default"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		settings := network.InterfaceSettings{
			Method: network.AddressMethod(body.Method), DNSAutomatic: body.DNSAutomatic,
			ManagementProtected: body.ManagementProtected, NeverDefault: body.NeverDefault,
		}
		if body.Address != "" {
			prefix, err := netip.ParsePrefix(body.Address)
			if err != nil {
				writeRefusal(response, http.StatusBadRequest, "interface_settings_rejected", "the address must be an IPv4 address with its prefix length, for example 192.168.0.1/24")
				return
			}
			settings.Address = prefix
		}
		if body.Gateway != "" {
			gateway, err := netip.ParseAddr(body.Gateway)
			if err != nil {
				writeRefusal(response, http.StatusBadRequest, "interface_settings_rejected", "the gateway must be an IPv4 address")
				return
			}
			settings.Gateway = gateway
		}
		for _, server := range body.DNS {
			address, err := netip.ParseAddr(server)
			if err != nil {
				writeRefusal(response, http.StatusBadRequest, "interface_settings_rejected", "each DNS server must be an IP address")
				return
			}
			settings.DNS = append(settings.DNS, address)
		}
		previewChange(response, request, manager, network.Change{
			Action: network.ActionInterfaceSettings, Target: request.PathValue("id"), Settings: &settings,
		})
	}
}

// handlePreviewWiFiJoin proposes joining a wireless network. The password is
// staged before the change exists and only its handle travels with it.
func handlePreviewWiFiJoin(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			SSID             string `json:"ssid"`
			Security         string `json:"security"`
			Password         string `json:"password"`
			ConnectAtStartup bool   `json:"connect_at_startup"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		if body.SSID == "" {
			writeRefusal(response, http.StatusBadRequest, "interface_settings_rejected", "a join needs the network name")
			return
		}
		handle, err := manager.StagePassword(request.Context(), body.Password)
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
			return
		}
		previewChange(response, request, manager, network.Change{
			Action: network.ActionWiFiJoin, Target: request.PathValue("id"), SecretHandle: handle,
			WiFi: &network.WiFiJoin{SSID: body.SSID, Security: network.WiFiSecurity(body.Security), ConnectAtStartup: body.ConnectAtStartup},
		})
	}
}

// handlePreviewWireGuardInstall proposes installing the WireGuard tooling.
func handlePreviewWireGuardInstall(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		preview(response, request, manager, network.ActionWireGuardInstall, "wireguard")
	}
}

func preview(
	response http.ResponseWriter, request *http.Request,
	manager *network.OperationManager, action network.Action, target string,
) {
	previewChange(response, request, manager, network.Change{Action: action, Target: target})
}

func previewChange(response http.ResponseWriter, request *http.Request, manager *network.OperationManager, change network.Change) {
	actor, ok := actorFrom(request.Context())
	if !ok {
		writeUnauthorized(response)
		return
	}
	if change.Target == "" {
		writeError(response, http.StatusBadRequest, "network_target_required", "")
		return
	}
	// The client's address is observed from the connection, never taken from a
	// header the client controls, and is what the management-path trace uses.
	change.Origin = clientAddress(request)

	operation, err := manager.PreviewChange(request.Context(), actor.User.ID, change)
	if err != nil {
		if writeAdapterRoleConflict(response, err) {
			return
		}
		if errors.Is(err, network.ErrPreviewStale) {
			writeOperationFailure(response, err)
			return
		}
		if errors.Is(err, network.ErrNotControllable) {
			writeError(response, http.StatusConflict, "network_not_controllable", "")
			return
		}
		for _, failure := range []error{
			network.ErrProfileActive, network.ErrExternalProfile, network.ErrUnsupported, network.ErrConnectionRequired,
			network.ErrProfileNameTaken, network.ErrProfileReferenced, network.ErrProfileNotFound,
		} {
			if errors.Is(err, failure) {
				// The message names what conflicts - the connected tunnel, the
				// device network still using the profile - and nothing secret.
				writeRefusal(response, http.StatusConflict, failure.Error(), err.Error())
				return
			}
		}
		if errors.Is(err, network.ErrProfileRejected) {
			writeRefusal(response, http.StatusBadRequest, "wireguard_profile_rejected", err.Error())
			return
		}
		if errors.Is(err, network.ErrInterfaceSettingsRejected) {
			writeRefusal(response, http.StatusBadRequest, "interface_settings_rejected", err.Error())
			return
		}
		if errors.Is(err, network.ErrDeviceNetworkRejected) {
			writeRefusal(response, http.StatusBadRequest, "device_network_rejected", err.Error())
			return
		}
		if errors.Is(err, network.ErrDeviceNetworkNotFound) {
			writeError(response, http.StatusNotFound, "device_network_not_found", "")
			return
		}
		if errors.Is(err, network.ErrManagementPathRequired) {
			writeRefusal(response, http.StatusConflict, "management_path_required", err.Error())
			return
		}
		writeError(response, http.StatusBadRequest, "network_request_invalid", "")
		return
	}
	writeJSON(response, http.StatusOK, operationPayload(operation))
}

// clientAddress is the connection's remote address without its port.
func clientAddress(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

// authorizeOperation resolves the operation and checks the caller against the
// permission its action needs. It returns false having already written the
// response when the caller may not proceed.
func authorizeOperation(
	response http.ResponseWriter, request *http.Request, manager *network.OperationManager,
) (network.Operation, bool) {
	actor, ok := actorFrom(request.Context())
	if !ok {
		writeUnauthorized(response)
		return network.Operation{}, false
	}
	operation, err := manager.Get(request.PathValue("id"))
	if err != nil {
		writeError(response, http.StatusNotFound, "network_operation_not_found", "")
		return network.Operation{}, false
	}
	permission, known := actionPermissions[operation.Action]
	if !known || !actor.Has(permission) {
		writeForbidden(response)
		return network.Operation{}, false
	}
	return operation, true
}

// maximumListedOperations bounds a reattachment listing. It answers "what is in
// flight, and what just happened", not "show me the history": that is the
// Activity page's question and it is paginated.
const maximumListedOperations = 50

type operationListResponse struct {
	APIVersion string              `json:"api_version"`
	Items      []operationResponse `json:"items"`
}

// handleListOperations answers an interface that has just been reloaded and no
// longer holds the identity of the change it started.
func handleListOperations(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operations, err := manager.Recent(request.Context(), maximumListedOperations)
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
			return
		}
		items := make([]operationResponse, 0, len(operations))
		for _, operation := range operations {
			items = append(items, operationPayload(operation))
		}
		writeJSON(response, http.StatusOK, operationListResponse{APIVersion: apiVersion, Items: items})
	}
}

func handleReadOperation(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operation, err := manager.Get(request.PathValue("id"))
		if err != nil {
			writeError(response, http.StatusNotFound, "network_operation_not_found", "")
			return
		}
		writeJSON(response, http.StatusOK, operationPayload(operation))
	}
}

func handleAcknowledgeOperation(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operation, permitted := authorizeOperation(response, request, manager)
		if !permitted {
			return
		}
		var body struct {
			Revision string `json:"revision"`
		}
		if err := decodeOperationRequest(request, &body); err != nil || body.Revision == "" {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		actor, _ := actorFrom(request.Context())

		acknowledged, err := manager.Acknowledge(
			request.Context(), actor.User.ID, operation.ID, body.Revision)
		if err != nil {
			writeOperationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusAccepted, operationPayload(acknowledged))
	}
}

func handleConfirmOperation(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operation, permitted := authorizeOperation(response, request, manager)
		if !permitted {
			return
		}
		actor, _ := actorFrom(request.Context())
		confirmed, err := manager.Confirm(request.Context(), actor.User.ID, operation.ID)
		if err != nil {
			writeOperationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, operationPayload(confirmed))
	}
}

func handleRollbackOperation(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operation, permitted := authorizeOperation(response, request, manager)
		if !permitted {
			return
		}
		actor, _ := actorFrom(request.Context())
		restored, err := manager.Rollback(request.Context(), actor.User.ID, operation.ID)
		if err != nil {
			writeOperationFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, operationPayload(restored))
	}
}

// writeOperationFailure maps one lifecycle refusal to one documented code. Each
// is a conflict with the operation's current state rather than a bad request:
// the caller asked for something reasonable that is no longer true.
func writeOperationFailure(response http.ResponseWriter, err error) {
	if writeAdapterRoleConflict(response, err) {
		return
	}
	switch {
	case errors.Is(err, network.ErrOperationNotFound):
		writeError(response, http.StatusNotFound, "network_operation_not_found", "")
	case errors.Is(err, network.ErrAlreadyAcknowledged):
		writeError(response, http.StatusConflict, "network_already_acknowledged", "")
	case errors.Is(err, network.ErrRevisionMismatch):
		writeError(response, http.StatusConflict, "network_preview_stale", "")
	case errors.Is(err, network.ErrPreviewStale):
		writeError(response, http.StatusConflict, "network_preview_stale", "")
	case errors.Is(err, network.ErrNotTheActorShown):
		writeForbidden(response)
	case errors.Is(err, network.ErrChangeInProgress):
		writeError(response, http.StatusConflict, "network_change_in_progress", "")
	case errors.Is(err, network.ErrRecoveryUnavailable):
		writeError(response, http.StatusConflict, "network_recovery_unavailable", "")
	case errors.Is(err, network.ErrNotAwaitingConfirm):
		writeError(response, http.StatusConflict, "network_not_awaiting_confirmation", "")
	default:
		writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
	}
}
