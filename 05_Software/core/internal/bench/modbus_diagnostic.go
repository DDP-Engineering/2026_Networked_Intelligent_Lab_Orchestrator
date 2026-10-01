package bench

import (
	"context"
	"encoding/hex"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusDiagnosticRequest selects one supported read-only diagnostic, never a raw PDU.
type ModbusDiagnosticRequest struct {
	SessionID string `json:"session_id"`
	Kind      string `json:"kind"`
	Unit      int    `json:"unit"`
}

// ModbusIdentityObject carries one bounded basic device-identification object.
type ModbusIdentityObject struct {
	ID       int    `json:"id"`
	ValueHex string `json:"value_hex"`
	Text     string `json:"text"`
}

// ModbusDiagnosticData is a typed projection of a validated diagnostic reply.
type ModbusDiagnosticData struct {
	Kind          string                 `json:"kind"`
	Objects       []ModbusIdentityObject `json:"objects,omitempty"`
	MoreFollows   bool                   `json:"more_follows,omitempty"`
	Status        bool                   `json:"status,omitempty"`
	Count         int                    `json:"count,omitempty"`
	IdentifierHex string                 `json:"identifier_hex,omitempty"`
}

func validModbusDiagnosticData(function int, data *ModbusDiagnosticData) bool {
	if data == nil {
		return false
	}
	switch function {
	case 43:
		if data.Kind != "device_information" || len(data.Objects) > 3 || data.Count != 0 || data.IdentifierHex != "" || data.Status {
			return false
		}
		seen := map[int]bool{}
		for _, object := range data.Objects {
			value, err := hex.DecodeString(object.ValueHex)
			if err != nil || len(value) > 64 || object.ID < 0 || object.ID > 2 || seen[object.ID] || len(object.Text) > 192 {
				return false
			}
			seen[object.ID] = true
		}
		return true
	case 11:
		return data.Kind == "comm_event_counter" && len(data.Objects) == 0 && data.Count >= 0 && data.Count <= 65535 && data.IdentifierHex == "" && !data.MoreFollows
	case 17:
		value, err := hex.DecodeString(data.IdentifierHex)
		return data.Kind == "report_server_id" && err == nil && len(value) <= 64 && len(data.Objects) == 0 && data.Count == 0 && !data.MoreFollows
	default:
		return false
	}
}

// ReadModbusDiagnostic sends a typed read-only diagnostic on the current owned lease.
func (m *Manager) ReadModbusDiagnostic(ctx context.Context, actor identity.Actor, endpointID string, input ModbusDiagnosticRequest) (ModbusResult, error) {
	return m.readModbusDiagnostic(ctx, actor, endpointID, input, func(ctx context.Context, s *modbusSession, request ModbusRequest) (ModbusResult, error) {
		return modbusPrivateCall(ctx, s, request, "")
	})
}

func (m *Manager) readModbusDiagnostic(ctx context.Context, actor identity.Actor, endpointID string, input ModbusDiagnosticRequest, dispatch modbusDispatch) (ModbusResult, error) {
	function := map[string]int{"device_information": 43, "comm_event_counter": 11, "report_server_id": 17}[input.Kind]
	if function == 0 || input.Unit < 1 || input.Unit > 247 || !modbusID.MatchString(endpointID) {
		return ModbusResult{}, ErrInvalid
	}
	request := ModbusRequest{Function: function, Unit: input.Unit, Address: 0, Count: 1}
	m.mu.Lock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != input.SessionID {
		m.mu.Unlock()
		return ModbusResult{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		m.mu.Unlock()
		return ModbusResult{}, err
	}
	e, err := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
	if err != nil || validateModbusTarget(request, e) != nil {
		m.mu.Unlock()
		return ModbusResult{}, ErrInvalid
	}
	if e.Transport == "tcp" && function != 43 {
		m.mu.Unlock()
		return ModbusResult{}, ErrInvalid
	}
	m.mu.Unlock()
	result, err := dispatch(ctx, s, request)
	if err != nil {
		return ModbusResult{}, err
	}
	if !validModbusResult(request, result) {
		return ModbusResult{}, ErrUnavailable
	}
	return result, nil
}
