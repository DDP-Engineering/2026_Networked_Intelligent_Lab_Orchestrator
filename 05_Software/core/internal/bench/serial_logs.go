package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

type serialLogWriter struct {
	id           string
	file         *os.File
	size, rx, tx int64
	limits       SerialRetention
	started      time.Time
}

const serialLogColumns = `id,device_id,connection_id,owner_id,started_at,ended_at,status,rx_bytes,tx_bytes,size_bytes`

func scanSerialLog(row scanner) (SerialLog, error) {
	var l SerialLog
	err := row.Scan(&l.ID, &l.DeviceID, &l.ConnectionID, &l.OwnerID, &l.StartedAt, &l.EndedAt, &l.Status, &l.RXBytes, &l.TXBytes, &l.SizeBytes)
	return l, normalizedError(err)
}
func (m *Manager) serialLogPath(id string) (string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", ErrInvalid
	}
	return filepath.Join(m.options.DataDir, "serial-logs", id+".jsonl"), nil
}
func (m *Manager) serialRetention(ctx context.Context, id string) (SerialRetentionState, error) {
	_, err := m.db().ExecContext(ctx, `INSERT OR IGNORE INTO serial_retention(device_id) VALUES(?)`, id)
	if err != nil {
		return SerialRetentionState{}, err
	}
	var state SerialRetentionState
	var raw string
	err = m.db().QueryRowContext(ctx, `SELECT revision,limits_json FROM serial_retention WHERE device_id=?`, id).Scan(&state.Revision, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &state.Limits)
	}
	if err == nil {
		err = m.db().QueryRowContext(ctx, `SELECT coalesce(sum(size_bytes),0) FROM serial_logs WHERE device_id=?`, id).Scan(&state.UsageBytes)
	}
	return state, err
}

// SerialRetention returns the shared per-device quota and actual disk usage.
func (m *Manager) SerialRetention(ctx context.Context, actor identity.Actor, id string) (SerialRetentionState, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return SerialRetentionState{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := managedRecord(ctx, m, "serial", id); err != nil {
		return SerialRetentionState{}, err
	}
	return m.serialRetention(ctx, id)
}

// UpdateSerialRetention changes future admission without deleting files immediately.
func (m *Manager) UpdateSerialRetention(ctx context.Context, actor identity.Actor, id string, revision int64, limits SerialRetention) (SerialRetentionState, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return SerialRetentionState{}, err
	}
	if revision < 1 || limits.Files < 1 || limits.Files > 10000 || limits.TotalBytes < 1<<20 || limits.TotalBytes > 1<<40 || limits.FileBytes < 64<<10 || limits.FileBytes > 1<<30 || limits.FileBytes > limits.TotalBytes {
		return SerialRetentionState{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.serialBusy(id) {
		return SerialRetentionState{}, ErrBusy
	}
	if _, err := managedRecord(ctx, m, "serial", id); err != nil {
		return SerialRetentionState{}, err
	}
	raw, _ := json.Marshal(limits)
	result, err := m.db().ExecContext(ctx, `UPDATE serial_retention SET revision=revision+1,limits_json=? WHERE device_id=? AND revision=?`, string(raw), id, revision)
	if err != nil {
		return SerialRetentionState{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return SerialRetentionState{}, ErrConflict
	}
	if err = m.record(ctx, actor, "serial.retention", id, limits); err != nil {
		return SerialRetentionState{}, err
	}
	return m.serialRetention(ctx, id)
}
func (m *Manager) serialLogAuthorized(ctx context.Context, actor identity.Actor, id string) (SerialLog, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return SerialLog{}, err
	}
	l, err := scanSerialLog(m.db().QueryRowContext(ctx, `SELECT `+serialLogColumns+` FROM serial_logs WHERE id=?`, id))
	if err == nil && l.OwnerID != actor.User.ID && !actor.Has(auth.PermissionPluginsManage) {
		return SerialLog{}, ErrForbidden
	}
	return l, err
}

// ListSerialLogs returns up to 100 authorized records in newest-first order.
func (m *Manager) ListSerialLogs(ctx context.Context, actor identity.Actor, id string, offset int) (SerialLogPage, error) {
	p := SerialLogPage{Logs: []SerialLog{}, Offset: offset}
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return p, err
	}
	if offset < 0 {
		return p, ErrInvalid
	}
	filter := `device_id=?`
	args := []any{id}
	if !actor.Has(auth.PermissionPluginsManage) {
		filter += ` AND owner_id=?`
		args = append(args, actor.User.ID)
	}
	if err := m.db().QueryRowContext(ctx, `SELECT count(*) FROM serial_logs WHERE `+filter, args...).Scan(&p.Total); err != nil {
		return p, err
	}
	rows, err := m.db().QueryContext(ctx, `SELECT `+serialLogColumns+` FROM serial_logs WHERE `+filter+` ORDER BY started_at DESC,id DESC LIMIT 100 OFFSET ?`, append(args, offset)...)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		l, err := scanSerialLog(rows)
		if err != nil {
			return p, err
		}
		p.Logs = append(p.Logs, l)
	}
	return p, rows.Err()
}
func (m *Manager) reserveSerialLog(ctx context.Context, s *serialSession, limits SerialRetention) error {
	for {
		var count int
		var used int64
		if err := m.db().QueryRowContext(ctx, `SELECT count(*),coalesce(sum(size_bytes),0) FROM serial_logs WHERE device_id=?`, s.device).Scan(&count, &used); err != nil {
			return err
		}
		free, total, err := m.cameraSpace(m.options.DataDir)
		if err != nil {
			return err
		}
		reserve := int64(2 << 30)
		if total/10 > reserve {
			reserve = total / 10
		}
		var pending int64
		for _, other := range m.serials {
			if other.log != nil {
				pending += other.log.limits.FileBytes - other.log.size
			}
		}
		if count < limits.Files && used+limits.FileBytes <= limits.TotalBytes && free-pending-limits.FileBytes >= reserve {
			return nil
		}
		rows, err := m.db().QueryContext(ctx, `SELECT id FROM serial_logs WHERE device_id=? AND status='completed' ORDER BY started_at,id`, s.device)
		if err != nil {
			return err
		}
		candidate := ""
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			if m.serialReads[id] == 0 {
				candidate = id
				break
			}
		}
		rows.Close()
		if err != nil {
			return err
		}
		if candidate == "" {
			return ErrBusy
		}
		if err = m.removeSerialLog(ctx, candidate); err != nil {
			return err
		}
	}
}
func (m *Manager) startSerialLog(ctx context.Context, s *serialSession) error {
	state, err := m.serialRetention(ctx, s.device)
	if err != nil {
		return err
	}
	if err = m.reserveSerialLog(ctx, s, state.Limits); err != nil {
		return err
	}
	id := uuid.NewString()
	path, _ := m.serialLogPath(id)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	record, err := managedRecord(ctx, m, "serial", s.device)
	if err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	started := m.now()
	header, _ := json.Marshal(map[string]any{"schema_version": 1, "type": "header", "device_id": s.device, "connection_id": s.id, "settings": s.settings, "identity": map[string]string{"vendor": record.Vendor, "product": record.Product, "serial": record.Serial, "port_path": record.PortPath}, "started_at": started.Format(time.RFC3339Nano)})
	header = append(header, '\n')
	n, err := file.Write(header)
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	_, err = m.db().ExecContext(ctx, `INSERT INTO serial_logs(id,device_id,connection_id,owner_id,started_at,status,size_bytes) VALUES(?,?,?,?,?,'active',?)`, id, s.device, s.id, s.owner, started.Format(time.RFC3339Nano), n)
	if err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	s.log = &serialLogWriter{id: id, file: file, size: int64(n), limits: state.Limits, started: started}
	return nil
}
func (m *Manager) appendSerialLog(s *serialSession, event SerialEvent) error {
	observed, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil {
		return ErrInvalid
	}
	if observed.Before(s.log.started) {
		return nil
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if s.log.size+int64(len(raw)) > s.log.limits.FileBytes {
		started := s.log.started
		m.finishSerialLog(s, true)
		if err = m.startSerialLog(context.Background(), s); err != nil {
			return err
		}
		// Rotation must retain already buffered events from this recording.
		s.log.started = started
	}
	l := s.log
	free, total, err := m.cameraSpace(m.options.DataDir)
	if err != nil {
		return err
	}
	reserve := int64(2 << 30)
	if total/10 > reserve {
		reserve = total / 10
	}
	if free-int64(len(raw)) < reserve {
		return ErrBusy
	}
	n, err := l.file.Write(raw)
	l.size += int64(n)
	if err == nil {
		err = l.file.Sync()
	}
	if err != nil {
		return err
	}
	if event.Direction == "rx" {
		l.rx += int64(event.ByteCount)
	}
	if event.Direction == "tx" {
		l.tx += int64(event.ByteCount)
	}
	_, err = m.db().ExecContext(context.Background(), `UPDATE serial_logs SET size_bytes=?,rx_bytes=?,tx_bytes=? WHERE id=?`, l.size, l.rx, l.tx, l.id)
	return err
}
func (m *Manager) finishSerialLog(s *serialSession, completed bool) {
	l := s.log
	if l == nil {
		return
	}
	s.log = nil
	err := l.file.Sync()
	closeErr := l.file.Close()
	if err != nil || closeErr != nil {
		completed = false
		m.report(errors.Join(err, closeErr))
	}
	status := "incomplete"
	if completed {
		status = "completed"
	}
	_, err = m.db().ExecContext(context.Background(), `UPDATE serial_logs SET status=?,ended_at=?,size_bytes=?,rx_bytes=?,tx_bytes=? WHERE id=?`, status, m.now().Format(time.RFC3339Nano), l.size, l.rx, l.tx, l.id)
	m.report(err)
}
func (m *Manager) removeSerialLog(ctx context.Context, id string) error {
	path, err := m.serialLogPath(id)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = m.db().ExecContext(ctx, `DELETE FROM serial_logs WHERE id=?`, id)
	return err
}

// DeleteSerialLog protects active records and download leases from removal.
func (m *Manager) DeleteSerialLog(ctx context.Context, actor identity.Actor, id string) error {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l, err := m.serialLogAuthorized(ctx, actor, id)
	if err != nil {
		return err
	}
	if l.Status == "active" || m.serialReads[id] > 0 {
		return ErrBusy
	}
	if err = m.record(ctx, actor, "serial.log_deleted", id, nil); err != nil {
		return err
	}
	return m.removeSerialLog(ctx, id)
}

type serialFileReader struct {
	io.Reader
	io.Closer
}

type serialLeasedReader struct {
	io.ReadCloser
	release func()
}

func (r *serialLeasedReader) Close() error {
	err := r.ReadCloser.Close()
	if r.release != nil {
		r.release()
		r.release = nil
	}
	return err
}
func (m *Manager) openSerialLog(ctx context.Context, actor identity.Actor, id string) (io.ReadCloser, SerialLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, err := m.serialLogAuthorized(ctx, actor, id)
	if err != nil {
		return nil, l, err
	}
	path, err := m.serialLogPath(id)
	if err != nil {
		return nil, l, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, l, ErrNotFound
	}
	m.serialReads[id]++
	return &serialLeasedReader{ReadCloser: serialFileReader{Reader: io.LimitReader(f, l.SizeBytes), Closer: f}, release: func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.serialReads[id]--
		if m.serialReads[id] == 0 {
			delete(m.serialReads, id)
		}
	}}, l, nil
}

// ReadSerialLog returns a bounded page of saved events with original cursors.
func (m *Manager) ReadSerialLog(ctx context.Context, actor identity.Actor, id string, after int64) (SerialRead, error) {
	if after < 0 {
		return SerialRead{}, ErrInvalid
	}
	r, _, err := m.openSerialLog(ctx, actor, id)
	if err != nil {
		return SerialRead{}, err
	}
	defer r.Close()
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 8192), 16384)
	first := true
	events := []SerialEvent{}
	size := 128
	for scan.Scan() {
		if err = ctx.Err(); err != nil {
			return SerialRead{}, err
		}
		if first {
			first = false
			continue
		}
		var e SerialEvent
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			return SerialRead{}, ErrUnavailable
		}
		if e.Sequence <= after {
			continue
		}
		if size+len(scan.Bytes()) > 64<<10 {
			break
		}
		events = append(events, e)
		size += len(scan.Bytes()) + 1
	}
	if err = scan.Err(); err != nil {
		return SerialRead{}, err
	}
	p := serialPage(events, after)
	p.Gap = false
	return p, nil
}

// ExportSerialLog streams authorized data while retaining a deletion lease.
func (m *Manager) ExportSerialLog(ctx context.Context, actor identity.Actor, id, format string) (io.ReadCloser, string, string, error) {
	if format != "jsonl" && format != "text" && format != "rx" && format != "tx" {
		return nil, "", "", ErrInvalid
	}
	source, _, err := m.openSerialLog(ctx, actor, id)
	if err != nil {
		return nil, "", "", err
	}
	if format == "jsonl" {
		return source, "serial-" + id + ".jsonl", "application/x-ndjson", nil
	}
	reader, writer := io.Pipe()
	exportContext, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer source.Close()
		scan := bufio.NewScanner(source)
		scan.Buffer(make([]byte, 8192), 16384)
		first := true
		for scan.Scan() {
			if exportContext.Err() != nil {
				writer.CloseWithError(exportContext.Err())
				return
			}
			if first {
				first = false
				continue
			}
			var event SerialEvent
			if json.Unmarshal(scan.Bytes(), &event) != nil {
				writer.CloseWithError(ErrUnavailable)
				return
			}
			b, e := event.bytes()
			if e != nil {
				writer.CloseWithError(e)
				return
			}
			if format == "text" {
				_, e = fmt.Fprintf(writer, "%s %s %q\n", event.Timestamp, strings.ToUpper(event.Direction), string(b))
			} else if event.Direction == format {
				_, e = writer.Write(b)
			}
			if e != nil {
				writer.CloseWithError(e)
				return
			}
		}
		writer.CloseWithError(scan.Err())
	}()
	mime, ext := "application/octet-stream", "bin"
	if format == "text" {
		mime, ext = "text/plain; charset=utf-8", "txt"
	}
	return &serialLeasedReader{ReadCloser: reader, release: func() { cancel(); <-done }}, "serial-" + id + "-" + format + "." + ext, mime, nil
}
func (m *Manager) recoverSerialLogs(ctx context.Context) error {
	root := filepath.Join(m.options.DataDir, "serial-workspaces")
	entries, cleanupErr := os.ReadDir(root)
	if cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
		return cleanupErr
	}
	for _, entry := range entries {
		if _, e := uuid.Parse(entry.Name()); e == nil && entry.IsDir() {
			if e = os.RemoveAll(filepath.Join(root, entry.Name())); e != nil {
				return e
			}
		}
	}

	if _, err := m.db().ExecContext(ctx, `UPDATE serial_connections SET state='closed' WHERE state='active'`); err != nil {
		return err
	}
	rows, err := m.db().QueryContext(ctx, `SELECT id FROM serial_logs WHERE status='active'`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		path, e := m.serialLogPath(id)
		if e != nil {
			return e
		}
		size, rx, tx := int64(0), int64(0), int64(0)
		f, e := os.OpenFile(path, os.O_RDWR, 0600)
		if e == nil {
			reader := bufio.NewReaderSize(f, 16384)
			first := true
			for {
				line, e := reader.ReadSlice('\n')
				if e != nil {
					break
				}
				if !json.Valid(line) {
					break
				}
				if !first {
					var event SerialEvent
					if json.Unmarshal(line, &event) != nil {
						break
					}
					b, e := event.bytes()
					if e != nil || len(b) != event.ByteCount || len(b) > 4096 {
						break
					}
					if event.Direction == "rx" {
						rx += int64(len(b))
					}
					if event.Direction == "tx" {
						tx += int64(len(b))
					}
				}
				first = false
				size += int64(len(line))
			}
			e = f.Truncate(size)
			if e == nil {
				e = f.Sync()
			}
			f.Close()
			if e != nil {
				return e
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		_, err = m.db().ExecContext(ctx, `UPDATE serial_logs SET status='incomplete',ended_at=?,size_bytes=?,rx_bytes=?,tx_bytes=? WHERE id=?`, m.now().Format(time.RFC3339Nano), size, rx, tx, id)
		if err != nil {
			return err
		}
	}
	return nil
}
