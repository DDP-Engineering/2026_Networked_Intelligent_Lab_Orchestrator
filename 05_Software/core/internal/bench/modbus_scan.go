package bench

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusScanPlan selects only finite, explicit read probes at wire addresses.
type ModbusScanPlan struct {
	SessionID       string `json:"session_id"`
	Function        int    `json:"function"`
	UnitFrom        int    `json:"unit_from"`
	UnitTo          int    `json:"unit_to"`
	AddressFrom     int    `json:"address_from"`
	AddressTo       int    `json:"address_to"`
	RatePerSecond   int    `json:"rate_per_second"`
	DeadlineSeconds int    `json:"deadline_seconds"`
}

func validateModbusScanPlan(plan ModbusScanPlan) (int, error) {
	if plan.Function < 1 || plan.Function > 4 ||
		plan.UnitFrom < 0 || plan.UnitTo > 255 || plan.UnitFrom > plan.UnitTo ||
		plan.AddressFrom < 0 || plan.AddressTo > 65535 || plan.AddressFrom > plan.AddressTo ||
		plan.RatePerSecond < 1 || plan.RatePerSecond > 10 ||
		plan.DeadlineSeconds < 1 || plan.DeadlineSeconds > 60 {
		return 0, ErrInvalid
	}
	probes := (plan.UnitTo - plan.UnitFrom + 1) * (plan.AddressTo - plan.AddressFrom + 1)
	if probes > 512 {
		return 0, ErrInvalid
	}
	return probes, nil
}

// ModbusScanProbe records one observed read outcome, without inferring a device type or writable address.
type ModbusScanProbe struct {
	Unit          int                `json:"unit"`
	Address       int                `json:"address"`
	Function      int                `json:"function"`
	Outcome       string             `json:"outcome"`
	Values        []json.RawMessage  `json:"values"`
	ExceptionCode int                `json:"exception_code,omitempty"`
	Frames        []ModbusTraceFrame `json:"frames,omitempty"`
}

// ModbusScanStatus reports finite progress for the current session scan.
type ModbusScanStatus struct {
	ID        string            `json:"scan_id"`
	SessionID string            `json:"session_id"`
	Plan      ModbusScanPlan    `json:"plan"`
	State     string            `json:"state"`
	Total     int               `json:"total"`
	Completed int               `json:"completed"`
	Results   []ModbusScanProbe `json:"results"`
}

type modbusScanState struct {
	status ModbusScanStatus
	cancel context.CancelFunc
}

// ModbusScanPage is a stable bounded slice of settled probes.
type ModbusScanPage struct {
	Results    []ModbusScanProbe `json:"results"`
	NextOffset int               `json:"next_offset"`
	Total      int               `json:"total"`
}

// ModbusScanResults returns at most 100 settled probes after authorization.
func (m *Manager) ModbusScanResults(ctx context.Context, actor identity.Actor, endpointID, sessionID string, offset int) (ModbusScanPage, error) {
	if offset < 0 || offset > 512 {
		return ModbusScanPage{}, ErrInvalid
	}
	status, err := m.ModbusScan(ctx, actor, endpointID, sessionID)
	if err != nil {
		return ModbusScanPage{}, err
	}
	end := offset + 100
	if end > len(status.Results) {
		end = len(status.Results)
	}
	if offset > end {
		offset = end
	}
	return ModbusScanPage{Results: status.Results[offset:end], NextOffset: end, Total: len(status.Results)}, nil
}

// ExportModbusScan renders settled probes; an incomplete scan exports only its observed prefix.
func (m *Manager) ExportModbusScan(ctx context.Context, actor identity.Actor, endpointID, sessionID, format string) ([]byte, error) {
	status, err := m.ModbusScan(ctx, actor, endpointID, sessionID)
	if err != nil {
		return nil, err
	}
	return renderModbusScan(status, format)
}

func renderModbusScan(status ModbusScanStatus, format string) ([]byte, error) {
	var buf bytes.Buffer
	switch format {
	case "csv":
		writer := csv.NewWriter(&buf)
		plan, _ := json.Marshal(status.Plan)
		_ = writer.Write([]string{"kind", "scan_id", "state", "function", "unit", "address", "outcome", "values", "exception_code", "request_hex", "reply_hex", "plan"})
		_ = writer.Write([]string{"plan", status.ID, status.State, "", "", "", "", "", "", "", "", string(plan)})
		for _, probe := range status.Results {
			values := ""
			for i, raw := range probe.Values {
				if i > 0 {
					values += " "
				}
				values += string(raw)
			}
			requestHex, replyHex := "", ""
			for _, frame := range probe.Frames {
				if frame.Direction == "tx" && requestHex == "" {
					requestHex = frame.DataHex
				}
				if frame.Direction == "rx" && replyHex == "" {
					replyHex = frame.DataHex
				}
			}
			_ = writer.Write([]string{"probe", status.ID, "", strconv.Itoa(probe.Function), strconv.Itoa(probe.Unit), strconv.Itoa(probe.Address), probe.Outcome, values, strconv.Itoa(probe.ExceptionCode), requestHex, replyHex, ""})
		}
		writer.Flush()
		return buf.Bytes(), writer.Error()
	case "jsonl":
		metadata, _ := json.Marshal(struct {
			Kind      string         `json:"kind"`
			ScanID    string         `json:"scan_id"`
			State     string         `json:"state"`
			Total     int            `json:"total"`
			Completed int            `json:"completed"`
			Plan      ModbusScanPlan `json:"plan"`
		}{"plan", status.ID, status.State, status.Total, status.Completed, status.Plan})
		buf.Write(metadata)
		buf.WriteByte('\n')
		for _, probe := range status.Results {
			raw, err := json.Marshal(struct {
				Kind   string `json:"kind"`
				ScanID string `json:"scan_id"`
				ModbusScanProbe
			}{"probe", status.ID, probe})
			if err != nil {
				return nil, err
			}
			buf.Write(raw)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), nil
	default:
		return nil, ErrInvalid
	}
}

// StartModbusScan starts only an explicit bounded read plan on an owned live session.
func (m *Manager) StartModbusScan(ctx context.Context, actor identity.Actor, endpointID string, plan ModbusScanPlan) (ModbusScanStatus, error) {
	return m.startModbusScan(ctx, actor, endpointID, plan, func(ctx context.Context, s *modbusSession, request ModbusRequest) (ModbusResult, error) {
		return modbusPrivateCall(ctx, s, request, "")
	})
}

func (m *Manager) startModbusScan(ctx context.Context, actor identity.Actor, endpointID string, plan ModbusScanPlan, dispatch modbusDispatch) (ModbusScanStatus, error) {
	total, err := validateModbusScanPlan(plan)
	if err != nil || !modbusID.MatchString(endpointID) {
		return ModbusScanStatus{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != plan.SessionID {
		return ModbusScanStatus{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusScanStatus{}, err
	}
	if s.scan != nil && s.scan.status.State == "running" {
		return ModbusScanStatus{}, ErrBusy
	}
	e, err := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
	if err != nil {
		return ModbusScanStatus{}, ErrUnavailable
	}
	for unit := plan.UnitFrom; unit <= plan.UnitTo; unit++ {
		if validateModbusTarget(ModbusRequest{Function: plan.Function, Unit: unit, Address: plan.AddressFrom, Count: 1}, e) != nil {
			return ModbusScanStatus{}, ErrInvalid
		}
	}
	workCtx, cancel := context.WithTimeout(context.Background(), time.Duration(plan.DeadlineSeconds)*time.Second)
	status := ModbusScanStatus{ID: uuid.NewString(), SessionID: s.id, Plan: plan, State: "running", Total: total, Results: []ModbusScanProbe{}}
	state := &modbusScanState{status: status, cancel: cancel}
	s.scan = state
	m.modbusWorkers.Add(1)
	go m.runModbusScan(workCtx, actor, s, state, dispatch)
	return status, nil
}

func (m *Manager) runModbusScan(ctx context.Context, actor identity.Actor, s *modbusSession, state *modbusScanState, dispatch modbusDispatch) {
	defer m.modbusWorkers.Done()
	defer state.cancel()
	plan := state.status.Plan
	interval := time.Second / time.Duration(plan.RatePerSecond)
	for unit := plan.UnitFrom; unit <= plan.UnitTo; unit++ {
		for address := plan.AddressFrom; address <= plan.AddressTo; address++ {
			if ctx.Err() != nil {
				m.finishModbusScan(s, state, ctx)
				return
			}
			m.mu.Lock()
			if s.scan != state || s.stopped || state.status.State != "running" {
				m.mu.Unlock()
				return
			}
			if err := m.modbusAuthorized(context.Background(), actor, s, true); err != nil {
				state.status.State = "authorization_expired"
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()
			request := ModbusRequest{Function: plan.Function, Unit: unit, Address: address, Count: 1}
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			result, err := dispatch(probeCtx, s, request)
			cancel()
			probe := ModbusScanProbe{Unit: unit, Address: address, Function: plan.Function, Values: []json.RawMessage{}}
			if err == nil && validModbusResult(request, result) {
				probe.Outcome, probe.Values, probe.ExceptionCode = result.Outcome, result.Values, result.ExceptionCode
				for _, frame := range result.Frames {
					if len(frame.DataHex) <= 1024 && len(probe.Frames) < 2 {
						probe.Frames = append(probe.Frames, frame)
					}
				}
			} else if errors.Is(err, context.DeadlineExceeded) {
				probe.Outcome = "timeout"
			} else if errors.Is(err, context.Canceled) {
				probe.Outcome = "cancelled"
			} else {
				probe.Outcome = "protocol_error"
			}
			m.mu.Lock()
			if s.scan != state || state.status.State != "running" {
				m.mu.Unlock()
				return
			}
			state.status.Results = append(state.status.Results, probe)
			state.status.Completed++
			finished := state.status.Completed == state.status.Total
			m.mu.Unlock()
			if finished {
				break
			}
			select {
			case <-ctx.Done():
				m.finishModbusScan(s, state, ctx)
				return
			case <-time.After(interval):
			}
		}
	}
	m.finishModbusScan(s, state, ctx)
}

func (m *Manager) finishModbusScan(s *modbusSession, state *modbusScanState, ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.scan != state || state.status.State != "running" {
		return
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		state.status.State = "deadline"
	} else if ctx.Err() != nil {
		state.status.State = "cancelled"
	} else {
		state.status.State = "completed"
	}
}

// ModbusScan returns the current bounded scan snapshot to its owner or administrator.
func (m *Manager) ModbusScan(ctx context.Context, actor identity.Actor, endpointID, sessionID string) (ModbusScanStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID || s.scan == nil {
		return ModbusScanStatus{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, false); err != nil {
		return ModbusScanStatus{}, err
	}
	if !actor.Has(auth.PermissionPluginsManage) && (string(actor.Credential) != s.kind || serialCredentialID(actor) != s.credential) {
		return ModbusScanStatus{}, ErrForbidden
	}
	status := s.scan.status
	status.Results = append([]ModbusScanProbe{}, status.Results...)
	return status, nil
}

// CancelModbusScan stops future probes and cancels an in-flight read.
func (m *Manager) CancelModbusScan(ctx context.Context, actor identity.Actor, endpointID, sessionID string) (ModbusScanStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID || s.scan == nil {
		return ModbusScanStatus{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusScanStatus{}, err
	}
	if s.scan.status.State == "running" {
		s.scan.status.State = "cancelled"
		s.scan.cancel()
	}
	status := s.scan.status
	status.Results = append([]ModbusScanProbe{}, status.Results...)
	return status, nil
}
