package bench

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// ModbusRecordingFind is a bounded structured event search within one recording.
type ModbusRecordingFind struct {
	After      int64  `json:"after"`
	Kind       string `json:"kind,omitempty"`
	Unit       int    `json:"unit,omitempty"`
	Function   int    `json:"function,omitempty"`
	Address    *int   `json:"address,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	PointID    string `json:"point_id,omitempty"`
	Text       string `json:"text,omitempty"`
	Hex        string `json:"hex,omitempty"`
	UTCFrom    string `json:"utc_from,omitempty"`
	UTCThrough string `json:"utc_through,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// ModbusRecordingEventPage reports what was searched as well as what matched.
type ModbusRecordingEventPage struct {
	Events     []ModbusRecordedEvent `json:"events"`
	NextCursor int64                 `json:"next_cursor"`
	Count      int                   `json:"count"`
	Scanned    int                   `json:"scanned"`
	NoMatch    bool                  `json:"no_match"`
	Truncated  bool                  `json:"truncated"`
}

func (m *Manager) leaseAuthorizedModbusRecording(ctx context.Context, actor identity.Actor, id string) (ModbusRecording, func(), error) {
	m.mu.Lock()
	meta, err := m.ModbusRecordingByID(ctx, actor, id)
	if err != nil {
		m.mu.Unlock()
		return ModbusRecording{}, nil, err
	}
	if m.modbusReads == nil {
		m.modbusReads = map[string]int{}
	}
	m.modbusReads[id]++
	m.mu.Unlock()
	return meta, func() {
		m.mu.Lock()
		m.modbusReads[id]--
		if m.modbusReads[id] == 0 {
			delete(m.modbusReads, id)
		}
		m.mu.Unlock()
	}, nil
}

func (m *Manager) readModbusEventLine(id string, part int, offset int64, length int) (ModbusRecordedEvent, []byte, error) {
	if !validModbusRecordingID(id) || part < 1 || part > 1000000 || offset < 0 || length < 1 || length > 60<<10 {
		return ModbusRecordedEvent{}, nil, ErrUnavailable
	}
	file, err := os.Open(filepath.Join(m.modbusRecordingRoot(id), formatModbusPart(part)))
	if err != nil {
		return ModbusRecordedEvent{}, nil, err
	}
	defer file.Close()
	line := make([]byte, length)
	if _, err = file.ReadAt(line, offset); err != nil && err != io.EOF {
		return ModbusRecordedEvent{}, nil, err
	}
	if len(line) == 0 || line[len(line)-1] != '\n' {
		return ModbusRecordedEvent{}, nil, ErrUnavailable
	}
	var event ModbusRecordedEvent
	if json.Unmarshal(line, &event) != nil || event.Sequence < 1 {
		return ModbusRecordedEvent{}, nil, ErrUnavailable
	}
	return event, line, nil
}

// FindModbusRecordingEvents seeks indexed candidates without loading a whole part.
func (m *Manager) FindModbusRecordingEvents(ctx context.Context, actor identity.Actor, id string, filter ModbusRecordingFind) (ModbusRecordingEventPage, error) {
	if filter.After < 0 || filter.Unit < 0 || filter.Unit > 255 || filter.Function < 0 || filter.Function > 16 || filter.Limit < 0 || filter.Limit > 100 || len(filter.Text) > 256 || len(filter.Hex) > 512 || len(filter.PointID) > 128 || len(filter.Outcome) > 64 || filter.Address != nil && (*filter.Address < 0 || *filter.Address > 65535) {
		return ModbusRecordingEventPage{}, ErrInvalid
	}
	if filter.Kind != "" && filter.Kind != "sample" && filter.Kind != "traffic" && filter.Kind != "write" {
		return ModbusRecordingEventPage{}, ErrInvalid
	}
	from, err := modbusRecordingTimestamp(filter.UTCFrom)
	if err != nil {
		return ModbusRecordingEventPage{}, err
	}
	through, err := modbusRecordingTimestamp(filter.UTCThrough)
	if err != nil {
		return ModbusRecordingEventPage{}, err
	}
	if from != "" && through != "" && from > through {
		return ModbusRecordingEventPage{}, ErrInvalid
	}
	hexNeedle := strings.Join(strings.Fields(filter.Hex), "")
	if len(hexNeedle)%2 != 0 {
		return ModbusRecordingEventPage{}, ErrInvalid
	}
	if _, err := hex.DecodeString(hexNeedle); err != nil {
		return ModbusRecordingEventPage{}, ErrInvalid
	}
	if filter.Limit == 0 {
		filter.Limit = 100
	}
	_, release, err := m.leaseAuthorizedModbusRecording(ctx, actor, id)
	if err != nil {
		return ModbusRecordingEventPage{}, err
	}
	defer release()
	query := `SELECT sequence,part_no,offset_bytes,length_bytes FROM modbus_recording_events WHERE recording_id=? AND sequence>?`
	args := []any{id, filter.After}
	add := func(column string, value any) { query += " AND " + column + "=?"; args = append(args, value) }
	if filter.Kind != "" {
		add("kind", filter.Kind)
	}
	if filter.Unit != 0 {
		add("unit", filter.Unit)
	}
	if filter.Function != 0 {
		add("function", filter.Function)
	}
	if filter.Address != nil {
		add("address", *filter.Address)
	}
	if filter.Outcome != "" {
		add("outcome", filter.Outcome)
	}
	if filter.PointID != "" {
		add("point_id", filter.PointID)
	}
	if from != "" {
		query += ` AND utc_at>=?`
		args = append(args, from)
	}
	if through != "" {
		query += ` AND utc_at<=?`
		args = append(args, through)
	}
	query += ` ORDER BY sequence LIMIT 10001`
	rows, err := m.db().QueryContext(ctx, query, args...)
	if err != nil {
		return ModbusRecordingEventPage{}, err
	}
	defer rows.Close()
	page := ModbusRecordingEventPage{Events: []ModbusRecordedEvent{}, NextCursor: filter.After}
	bytesUsed := 0
	for rows.Next() {
		var sequence, offset int64
		var part, length int
		if err := rows.Scan(&sequence, &part, &offset, &length); err != nil {
			return ModbusRecordingEventPage{}, err
		}
		if page.Scanned >= 10000 {
			page.Truncated = true
			break
		}
		page.Scanned++
		event, line, err := m.readModbusEventLine(id, part, offset, length)
		if err != nil || event.Sequence != sequence {
			return ModbusRecordingEventPage{}, ErrUnavailable
		}
		page.NextCursor = sequence
		if filter.Text != "" && !strings.Contains(strings.ToLower(string(line)), strings.ToLower(filter.Text)) {
			continue
		}
		if hexNeedle != "" {
			frames := event.RequestHex + event.ReplyHex
			for _, frame := range event.Result.Frames {
				frames += frame.DataHex
			}
			if !strings.Contains(strings.ToLower(frames), strings.ToLower(hexNeedle)) {
				continue
			}
		}
		if len(page.Events) >= filter.Limit || bytesUsed+len(line) > 64<<10 {
			page.NextCursor = sequence - 1
			page.Truncated = true
			break
		}
		page.Events = append(page.Events, event)
		page.Count++
		bytesUsed += len(line)
	}
	if err := rows.Err(); err != nil {
		return ModbusRecordingEventPage{}, err
	}
	page.NoMatch = page.Count == 0 && !page.Truncated
	return page, nil
}

// ModbusRecordingPage is a stable metadata page ordered newest first.
type ModbusRecordingPage struct {
	Recordings []ModbusRecording `json:"recordings"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Count      int               `json:"count"`
	NoMatch    bool              `json:"no_match"`
}

// ModbusRecordingSearch selects retained metadata without opening hardware.
type ModbusRecordingSearch struct {
	Before         string `json:"before,omitempty"`
	Name           string `json:"name,omitempty"`
	Tag            string `json:"tag,omitempty"`
	EndpointID     string `json:"endpoint_id,omitempty"`
	StartedFrom    string `json:"started_from,omitempty"`
	StartedThrough string `json:"started_through,omitempty"`
}

func modbusRecordingTimestamp(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	stamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", ErrInvalid
	}
	return stamp.UTC().Format("2006-01-02T15:04:05.000000000Z"), nil
}

// FindModbusRecordings returns a bounded, owner-scoped metadata page.
func (m *Manager) FindModbusRecordings(ctx context.Context, actor identity.Actor, filter ModbusRecordingSearch) (ModbusRecordingPage, error) {
	if !actor.Has(auth.PermissionBenchRead) {
		return ModbusRecordingPage{}, ErrForbidden
	}
	if !validModbusRecordingText(filter.Name, 120) || !validModbusRecordingText(filter.Tag, 48) || len(filter.EndpointID) > 128 || filter.Before != "" && !validModbusRecordingID(filter.Before) {
		return ModbusRecordingPage{}, ErrInvalid
	}
	if filter.Before != "" {
		if _, err := m.ModbusRecordingByID(ctx, actor, filter.Before); err != nil {
			return ModbusRecordingPage{}, err
		}
	}
	from, err := modbusRecordingTimestamp(filter.StartedFrom)
	if err != nil {
		return ModbusRecordingPage{}, err
	}
	through, err := modbusRecordingTimestamp(filter.StartedThrough)
	if err != nil {
		return ModbusRecordingPage{}, err
	}
	if from != "" && through != "" && from > through {
		return ModbusRecordingPage{}, ErrInvalid
	}
	query := `SELECT id FROM modbus_recordings WHERE 1=1`
	args := []any{}
	if !actor.Has(auth.PermissionPluginsManage) {
		query += ` AND owner_id=?`
		args = append(args, actor.User.ID)
	}
	if filter.Before != "" {
		query += ` AND (started_at,id)<(SELECT started_at,id FROM modbus_recordings WHERE id=?)`
		args = append(args, filter.Before)
	}
	if filter.Name != "" {
		query += ` AND instr(lower(name),lower(?))>0`
		args = append(args, filter.Name)
	}
	if filter.Tag != "" {
		query += ` AND EXISTS(SELECT 1 FROM modbus_recording_tags WHERE recording_id=modbus_recordings.id AND tag_key=lower(?))`
		args = append(args, filter.Tag)
	}
	if filter.EndpointID != "" {
		query += ` AND endpoint_id=?`
		args = append(args, filter.EndpointID)
	}
	if from != "" {
		query += ` AND started_at>=?`
		args = append(args, from)
	}
	if through != "" {
		query += ` AND started_at<=?`
		args = append(args, through)
	}
	query += ` ORDER BY started_at DESC,id DESC LIMIT 101`
	rows, err := m.db().QueryContext(ctx, query, args...)
	if err != nil {
		return ModbusRecordingPage{}, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return ModbusRecordingPage{}, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return ModbusRecordingPage{}, err
	}
	page := ModbusRecordingPage{Recordings: []ModbusRecording{}}
	more := len(ids) > 100
	if more {
		ids = ids[:100]
	}
	for _, id := range ids {
		item, err := m.readModbusRecording(ctx, id)
		if err != nil {
			return ModbusRecordingPage{}, fmt.Errorf("read recording metadata: %w", err)
		}
		page.Recordings = append(page.Recordings, item)
	}
	if more {
		page.NextCursor = ids[len(ids)-1]
	}
	page.Count = len(page.Recordings)
	page.NoMatch = page.Count == 0
	return page, nil
}

// ListModbusRecordings preserves the simple metadata cursor API.
func (m *Manager) ListModbusRecordings(ctx context.Context, actor identity.Actor, before string) (ModbusRecordingPage, error) {
	return m.FindModbusRecordings(ctx, actor, ModbusRecordingSearch{Before: before})
}
