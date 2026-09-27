package bench

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

type serialRunnerStatus struct {
	Connected    bool             `json:"connected"`
	Signals      map[string]*bool `json:"signals"`
	Warning      string           `json:"warning"`
	LastSequence int64            `json:"last_sequence"`
}

func (m *Manager) waitSerialReady(ctx context.Context, s *serialSession) error {
	ready := time.NewTicker(50 * time.Millisecond)
	defer ready.Stop()
	lastCheck := time.Time{}
	for {
		if time.Since(lastCheck) >= time.Second {
			lastCheck = time.Now()
			if err := m.serialLeaseValid(ctx, s); err != nil {
				return err
			}
		}
		statusCtx, finish := context.WithTimeout(ctx, 500*time.Millisecond)
		raw, err := cameraCall(statusCtx, s.transport, "GET", "/status", nil)
		finish()
		if err == nil {
			var status serialRunnerStatus
			if json.Unmarshal(raw, &status) == nil && status.Connected {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.runnerDone:
			return ErrUnavailable
		case <-ready.C:
		}
	}
}

// ingestSerial accepts overlapping polls, validates byte records and preserves
// discontinuities for downstream readers. Manager.mu is held by the caller.
func (m *Manager) ingestSerial(s *serialSession, raw []byte) error {
	var page SerialRead
	if strict(raw, &page) != nil || len(page.Events) > 512 {
		return ErrInvalid
	}
	if page.Gap {
		s.warning = "receive_gap"
		m.finishSerialLog(s, false)
	}
	for _, event := range page.Events {
		if event.Sequence <= s.cursor {
			continue
		}
		b, err := event.bytes()
		_, timeErr := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil || timeErr != nil || len(b) > 4096 || len(b) != event.ByteCount || len(event.Message) > 1024 || event.ElapsedMS < 0 {
			return ErrInvalid
		}
		switch event.Direction {
		case "rx", "tx", "gap", "state", "error":
		default:
			return ErrInvalid
		}
		if event.Sequence > s.cursor+1 || event.Direction == "gap" || event.Direction == "error" {
			s.warning = "receive_gap"
			m.finishSerialLog(s, false)
		}
		s.cursor = event.Sequence
		s.events = append(s.events, event)
		s.ringBytes += len(b) + len(event.Message) + 128
		if event.Direction == "rx" {
			s.rx += int64(len(b))
		}
		if event.Direction == "tx" {
			s.tx += int64(len(b))
		}
		for s.ringBytes > 1<<20 || len(s.events) > 4096 {
			old := s.events[0]
			s.ringBytes -= old.ByteCount + len(old.Message) + 128
			s.events[0] = SerialEvent{}
			s.events = s.events[1:]
		}
		if s.log != nil {
			if err = m.appendSerialLog(s, event); err != nil {
				s.warning = "logging_stopped"
				m.finishSerialLog(s, false)
				m.report(err)
			}
		}
	}
	return nil
}

// drainSerial snapshots the runner tail so closing a recording includes bytes
// already observed there. A bounded failed drain produces an incomplete log.
func (m *Manager) drainSerial(s *serialSession) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if s.transport == nil || s.transport.client == nil {
		return ErrUnavailable
	}
	raw, err := cameraCall(ctx, s.transport, "GET", "/status", nil)
	if err != nil {
		return err
	}
	var status serialRunnerStatus
	if json.Unmarshal(raw, &status) != nil {
		return ErrUnavailable
	}
	target := status.LastSequence
	for s.cursor < target {
		raw, err = cameraCall(ctx, s.transport, "GET", "/read?after="+strconv.FormatInt(s.cursor, 10)+"&wait_ms=0", nil)
		if err != nil {
			return err
		}
		previous := s.cursor
		if err = m.ingestSerial(s, raw); err != nil {
			return err
		}
		if s.cursor == previous {
			return ErrUnavailable
		}
	}
	return nil
}
