package bench

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/identity"
)

type modbusExportPart struct {
	Number        int    `json:"part_no"`
	SizeBytes     int64  `json:"size_bytes"`
	FirstSequence int64  `json:"first_sequence"`
	LastSequence  int64  `json:"last_sequence"`
	SHA256        string `json:"sha256"`
	Closed        bool   `json:"closed"`
}

type modbusExportReader struct {
	*io.PipeReader
	release func()
	once    sync.Once
}

func (r *modbusExportReader) Close() error {
	err := r.PipeReader.Close()
	r.once.Do(r.release)
	return err
}

func (m *Manager) modbusExportParts(ctx context.Context, id string) ([]modbusExportPart, error) {
	rows, err := m.db().QueryContext(ctx, `SELECT part_no,size_bytes,first_sequence,last_sequence,sha256,closed FROM modbus_recording_parts WHERE recording_id=? ORDER BY part_no`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []modbusExportPart
	for rows.Next() {
		var part modbusExportPart
		if err := rows.Scan(&part.Number, &part.SizeBytes, &part.FirstSequence, &part.LastSequence, &part.SHA256, &part.Closed); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return parts, rows.Err()
}

func (m *Manager) verifyModbusExportParts(ctx context.Context, id string, parts []modbusExportPart) error {
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := os.Open(filepath.Join(m.modbusRecordingRoot(id), formatModbusPart(part.Number)))
		if err != nil {
			return ErrUnavailable
		}
		hash := sha256.New()
		count, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || count < part.SizeBytes || part.Closed && (count != part.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != part.SHA256) {
			return ErrUnavailable
		}
	}
	return nil
}

// ExportModbusRecording validates immutable parts and streams one bounded format.
func (m *Manager) ExportModbusRecording(ctx context.Context, actor identity.Actor, id, format string, after, through int64) (io.ReadCloser, string, error) {
	if after < 0 || through < 0 || through != 0 && through <= after {
		return nil, "", ErrInvalid
	}
	media := map[string]string{"jsonl": "application/x-ndjson", "csv": "text/csv; charset=utf-8", "text": "text/plain; charset=utf-8", "bundle": "application/zip"}[format]
	if media == "" {
		return nil, "", ErrInvalid
	}
	meta, release, err := m.leaseAuthorizedModbusRecording(ctx, actor, id)
	if err != nil {
		return nil, "", err
	}
	if format == "bundle" && (after != 0 || through != 0 || meta.Status == "active") {
		release()
		return nil, "", ErrInvalid
	}
	parts, err := m.modbusExportParts(ctx, id)
	if err == nil {
		err = m.verifyModbusExportParts(ctx, id, parts)
	}
	if err != nil {
		release()
		return nil, "", err
	}
	if through == 0 {
		through = meta.LastSequence
	}
	reader, writer := io.Pipe()
	stream := &modbusExportReader{PipeReader: reader, release: release}
	go func() {
		defer writer.Close()
		var writeErr error
		if format == "bundle" {
			writeErr = m.writeModbusBundle(ctx, writer, meta, parts)
		} else {
			writeErr = m.writeModbusEventExport(ctx, writer, meta, format, after, through)
		}
		if writeErr != nil {
			_ = writer.CloseWithError(writeErr)
		}
	}()
	return stream, media, nil
}

func modbusCSVSafe(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

func (m *Manager) writeModbusEventExport(ctx context.Context, w io.Writer, meta ModbusRecording, format string, after, through int64) error {
	var snapshot string
	if format == "jsonl" {
		if err := m.db().QueryRowContext(ctx, `SELECT snapshot_json FROM modbus_recordings WHERE id=?`, meta.ID).Scan(&snapshot); err != nil {
			return err
		}
		if !json.Valid([]byte(snapshot)) {
			return ErrUnavailable
		}
	}
	rows, err := m.db().QueryContext(ctx, `SELECT sequence,part_no,offset_bytes,length_bytes FROM modbus_recording_events WHERE recording_id=? AND sequence>? AND sequence<=? ORDER BY sequence`, meta.ID, after, through)
	if err != nil {
		return err
	}
	defer rows.Close()
	if format == "jsonl" {
		header, _ := json.Marshal(map[string]any{"schema": 1, "kind": "recording_header", "recording": meta, "snapshot": json.RawMessage(snapshot)})
		if _, err := w.Write(append(header, '\n')); err != nil {
			return err
		}
	}
	var csvWriter *csv.Writer
	if format == "csv" {
		csvWriter = csv.NewWriter(w)
		if err := csvWriter.Write([]string{"recording_id", "name", "recording_status", "incomplete_reason", "last_durable_sequence", "sequence", "utc_at", "elapsed_ns", "kind", "group_id", "unit", "function", "address", "point_id", "raw_json", "typed_json", "quality", "outcome", "request_hex", "reply_hex", "command_id"}); err != nil {
			return err
		}
	}
	if format == "text" {
		if _, err := fmt.Fprintf(w, "Recording %s status=%s reason=%s last_durable_sequence=%d\n", meta.ID, meta.Status, meta.Reason, meta.LastSequence); err != nil {
			return err
		}
	}
	var written int64
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var sequence, offset int64
		var part, length int
		if err := rows.Scan(&sequence, &part, &offset, &length); err != nil {
			return err
		}
		event, line, err := m.readModbusEventLine(meta.ID, part, offset, length)
		if err != nil || event.Sequence != sequence {
			return ErrUnavailable
		}
		written += int64(len(line))
		if written > 64<<20 {
			return ErrBusy
		}
		switch format {
		case "jsonl":
			_, err = w.Write(line)
		case "csv":
			raw, _ := json.Marshal(event.Raw)
			typed, _ := json.Marshal(event.Typed)
			err = csvWriter.Write([]string{meta.ID, modbusCSVSafe(meta.Name), meta.Status, modbusCSVSafe(meta.Reason), strconv.FormatInt(meta.LastSequence, 10), strconv.FormatInt(event.Sequence, 10), event.Timestamp,
				strconv.FormatInt(event.ElapsedNS, 10), event.Kind, modbusCSVSafe(event.GroupID), strconv.Itoa(event.Request.Unit), strconv.Itoa(event.Request.Function),
				strconv.Itoa(event.Request.Address), modbusCSVSafe(event.PointID), string(raw), string(typed), modbusCSVSafe(event.Quality), modbusCSVSafe(event.Result.Outcome), event.RequestHex, event.ReplyHex, event.CommandID})
		case "text":
			_, err = fmt.Fprintf(w, "%d %s %s unit=%d function=%d address=%d quality=%s outcome=%s raw=%s\n", event.Sequence, event.Timestamp, event.Kind, event.Request.Unit, event.Request.Function, event.Request.Address, event.Quality, event.Result.Outcome, string(line))
		}
		if err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if csvWriter != nil {
		csvWriter.Flush()
		return csvWriter.Error()
	}
	return nil
}

func (m *Manager) writeModbusBundle(ctx context.Context, w io.Writer, meta ModbusRecording, parts []modbusExportPart) error {
	archive := zip.NewWriter(w)
	var snapshot string
	if err := m.db().QueryRowContext(ctx, `SELECT snapshot_json FROM modbus_recordings WHERE id=?`, meta.ID).Scan(&snapshot); err != nil {
		return err
	}
	metadata, _ := json.Marshal(meta)
	for _, item := range []struct {
		name string
		data []byte
	}{{"metadata.json", metadata}, {"definitions.json", []byte(snapshot)}} {
		entry, err := archive.Create(item.name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(item.data); err != nil {
			return err
		}
	}
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := archive.Create(filepath.ToSlash(filepath.Join("parts", formatModbusPart(part.Number))))
		if err != nil {
			return err
		}
		file, err := os.Open(filepath.Join(m.modbusRecordingRoot(meta.ID), formatModbusPart(part.Number)))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(entry, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	manifest, _ := json.Marshal(map[string]any{"schema": 1, "recording_id": meta.ID, "status": meta.Status, "reason": meta.Reason, "generated_at": time.Now().UTC().Format(time.RFC3339Nano), "parts": parts})
	entry, err := archive.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err = entry.Write(manifest); err != nil {
		return err
	}
	return archive.Close()
}
