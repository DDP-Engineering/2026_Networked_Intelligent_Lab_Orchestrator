package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusPollGroup schedules raw reads. Missed intervals are dropped, never caught up.
type ModbusPollGroup struct {
	ID         string          `json:"id"`
	IntervalMS int             `json:"interval_ms"`
	Requests   []ModbusRequest `json:"requests"`
}

// ModbusEvent is a bounded live observation, independent of retained recordings.
type ModbusEvent struct {
	Sequence  int64         `json:"sequence"`
	Timestamp string        `json:"timestamp"`
	GroupID   string        `json:"group_id"`
	Request   ModbusRequest `json:"request"`
	Result    ModbusResult  `json:"result"`
}

// ModbusEventPage identifies gaps when a consumer falls behind the live ring.
type ModbusEventPage struct {
	Events     []ModbusEvent `json:"events"`
	NextCursor int64         `json:"next_cursor"`
	Gap        bool          `json:"gap"`
}

func validateModbusPollGroups(groups []ModbusPollGroup) error {
	if len(groups) > 32 {
		return ErrInvalid
	}
	ids := make(map[string]bool, len(groups))
	total := 0
	for _, group := range groups {
		if !modbusID.MatchString(group.ID) || ids[group.ID] || group.IntervalMS < 100 || group.IntervalMS > 60000 || len(group.Requests) == 0 || len(group.Requests) > 128 {
			return ErrInvalid
		}
		ids[group.ID] = true
		total += len(group.Requests)
		for _, request := range group.Requests {
			if request.Function < 1 || request.Function > 4 {
				return ErrInvalid
			}
			if _, err := canonicalModbusRequest(request); err != nil {
				return err
			}
		}
	}
	if total > 4096 {
		return ErrInvalid
	}
	return nil
}

func canonicalModbusPollGroups(groups []ModbusPollGroup) ([]ModbusPollGroup, error) {
	if err := validateModbusPollGroups(groups); err != nil {
		return nil, err
	}
	canonical := make([]ModbusPollGroup, len(groups))
	for i, group := range groups {
		canonical[i] = group
		canonical[i].Requests = make([]ModbusRequest, len(group.Requests))
		for j, request := range group.Requests {
			canonical[i].Requests[j], _ = canonicalModbusRequest(request)
		}
	}
	return canonical, nil
}

func modbusRequestInPlan(groups []ModbusPollGroup, request ModbusRequest) bool {
	for _, group := range groups {
		for _, approved := range group.Requests {
			if reflect.DeepEqual(approved, request) {
				return true
			}
		}
	}
	return false
}

// SetModbusPolling updates pause state only when the immutable approved plan matches.
func (m *Manager) SetModbusPolling(ctx context.Context, actor identity.Actor, endpointID, sessionID string, groups []ModbusPollGroup, paused bool) (ModbusStatus, error) {
	groups, err := canonicalModbusPollGroups(groups)
	if err != nil {
		return ModbusStatus{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ModbusStatus{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusStatus{}, err
	}
	e, err := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
	if err != nil {
		return ModbusStatus{}, err
	}
	for _, group := range groups {
		for _, request := range group.Requests {
			if err := validateModbusTarget(request, e); err != nil {
				return ModbusStatus{}, err
			}
		}
	}
	if !reflect.DeepEqual(groups, s.groups) {
		return ModbusStatus{}, ErrConflict
	}
	if err := m.record(ctx, actor, "modbus.poll_configured", s.id, map[string]any{"groups": len(groups), "paused": paused}); err != nil {
		return ModbusStatus{}, err
	}
	s.pollPaused = paused
	return m.modbusStatus(s, actor), nil
}

// SetModbusDisplayPaused changes only the caller's display control state; polling continues.
func (m *Manager) SetModbusDisplayPaused(ctx context.Context, actor identity.Actor, endpointID, sessionID string, paused bool) (ModbusStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ModbusStatus{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusStatus{}, err
	}
	s.displayPaused = paused
	return m.modbusStatus(s, actor), nil
}

func (m *Manager) pollModbus(s *modbusSession) {
	defer m.modbusWorkers.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	next := map[string]time.Time{}
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
		}
		m.mu.Lock()
		if s.stopped || m.modbusSessions[s.endpointID] != s {
			m.mu.Unlock()
			return
		}
		if s.pollPaused {
			m.mu.Unlock()
			continue
		}
		var due []ModbusPollGroup
		for _, group := range s.groups {
			if time.Now().Before(next[group.ID]) {
				continue
			}
			due = append(due, group)
			next[group.ID] = time.Now().Add(time.Duration(group.IntervalMS) * time.Millisecond)
		}
		m.mu.Unlock()
		for _, group := range due {
			for index, request := range group.Requests {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				key := sha256.Sum256([]byte(group.ID))
				result, err := modbusPrivateCall(ctx, s, request, fmt.Sprintf("poll-%x-%d", key[:8], index))
				cancel()
				if err != nil {
					result = ModbusResult{Outcome: "disconnected", Values: []json.RawMessage{}, Attempts: 0}
				}
				m.mu.Lock()
				if s.stopped {
					m.mu.Unlock()
					return
				}
				s.cursor++
				s.events = append(s.events, ModbusEvent{Sequence: s.cursor, Timestamp: m.now().Format(time.RFC3339Nano), GroupID: group.ID, Request: request, Result: result})
				if len(s.events) > 256 {
					s.events = append([]ModbusEvent(nil), s.events[len(s.events)-256:]...)
				}
				m.mu.Unlock()
			}
		}
	}
}

// ReadModbusEvents returns at most 64 KiB of live events after a cursor.
func (m *Manager) ReadModbusEvents(ctx context.Context, actor identity.Actor, endpointID, sessionID string, after int64) (ModbusEventPage, error) {
	if after < 0 {
		return ModbusEventPage{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ModbusEventPage{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, false); err != nil {
		return ModbusEventPage{}, err
	}
	page := ModbusEventPage{Events: []ModbusEvent{}, NextCursor: after}
	if len(s.events) > 0 && after < s.events[0].Sequence-1 {
		page.Gap = true
	}
	size := 128
	for _, event := range s.events {
		if event.Sequence <= after {
			continue
		}
		raw, _ := json.Marshal(event)
		if size+len(raw) > 64<<10 {
			break
		}
		page.Events = append(page.Events, event)
		page.NextCursor = event.Sequence
		size += len(raw)
	}
	return page, nil
}
