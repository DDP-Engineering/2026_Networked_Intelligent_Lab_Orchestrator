package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// reconcileModbusDeletes resolves a rename/delete crash using the committed
// metadata row as the authority. Only UUID-named staged directories qualify.
func (m *Manager) reconcileModbusDeletes(ctx context.Context) error {
	base := filepath.Join(m.options.DataDir, "modbus-recordings")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".deleting") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".deleting")
		if !validModbusRecordingID(id) {
			continue
		}
		var stored string
		err := m.db().QueryRowContext(ctx, `SELECT id FROM modbus_recordings WHERE id=?`, id).Scan(&stored)
		staged := filepath.Join(base, entry.Name())
		switch {
		case err == nil:
			if _, statErr := os.Stat(filepath.Join(base, id)); statErr == nil {
				return ErrUnavailable
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			if err := os.Rename(staged, filepath.Join(base, id)); err != nil {
				return err
			}
		case errors.Is(err, sql.ErrNoRows):
			if err := os.RemoveAll(staged); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

// recoverModbusRecordings marks interrupted writers incomplete and rebuilds
// only their complete, ordered on-disk prefix before any public read.
func (m *Manager) recoverModbusRecordings(ctx context.Context) error {
	rows, err := m.db().QueryContext(ctx, `SELECT id FROM modbus_recordings WHERE status='active' OR status='incomplete' AND recovery_done=0 ORDER BY started_at,id`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.recoverModbusRecording(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) recoverModbusRecording(ctx context.Context, id string) error {
	if !validModbusRecordingID(id) {
		return ErrUnavailable
	}
	var previousStatus, previousReason, snapshot string
	if err := m.db().QueryRowContext(ctx, `SELECT status,reason,snapshot_json FROM modbus_recordings WHERE id=?`, id).Scan(&previousStatus, &previousReason, &snapshot); err != nil {
		return err
	}
	root := m.modbusRecordingRoot(id)
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		if renameErr := os.Rename(root+".deleting", root); renameErr != nil {
			_, updateErr := m.db().ExecContext(ctx, `UPDATE modbus_recordings SET status='incomplete',reason='missing_parts',ended_at=?,recovery_done=1 WHERE id=?`, m.now().Format(time.RFC3339Nano), id)
			return updateErr
		}
	}
	parts, err := m.modbusExportParts(ctx, id)
	if err != nil {
		return err
	}
	tx, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM modbus_recording_events WHERE recording_id=?`, id); err != nil {
		return err
	}
	lastSequence := int64(0)
	totalSize := int64(0)
	reason := "core_restarted"
	if previousStatus == "incomplete" && previousReason != "" {
		reason = previousReason
	}
	for _, part := range parts {
		path := filepath.Join(root, formatModbusPart(part.Number))
		file, openErr := os.OpenFile(path, os.O_RDWR, 0600)
		if openErr != nil {
			reason = "missing_part"
			break
		}
		if part.Closed {
			hash := sha256.New()
			actual, hashErr := io.Copy(hash, file)
			// The file can be larger than the old index after a synced append
			// whose SQLite transaction failed. Its digest still covers the bytes.
			if hashErr != nil || hex.EncodeToString(hash.Sum(nil)) != part.SHA256 || actual < part.SizeBytes {
				_ = file.Close()
				reason = "digest_mismatch"
				break
			}
			if _, hashErr := file.Seek(0, io.SeekStart); hashErr != nil {
				_ = file.Close()
				return hashErr
			}
		}
		reader := bufio.NewReaderSize(file, 1<<20)
		header, readErr := reader.ReadSlice('\n')
		if readErr != nil || len(header) == 0 || len(header) > 1<<20 {
			_ = file.Close()
			reason = "invalid_header"
			break
		}
		var marker struct {
			Schema      int             `json:"schema"`
			Kind        string          `json:"kind"`
			RecordingID string          `json:"recording_id"`
			PartNo      int             `json:"part_no"`
			Snapshot    json.RawMessage `json:"snapshot"`
		}
		if json.Unmarshal(header, &marker) != nil || marker.Schema != 1 || marker.Kind != "header" || marker.RecordingID != id || marker.PartNo != part.Number || !bytes.Equal(marker.Snapshot, []byte(snapshot)) {
			_ = file.Close()
			reason = "invalid_header"
			break
		}
		offset := int64(len(header))
		first := int64(0)
		partLast := int64(0)
		for {
			line, lineErr := reader.ReadSlice('\n')
			if lineErr == io.EOF && len(line) == 0 {
				break
			}
			if lineErr != nil || len(line) > 60<<10 {
				reason = "torn_tail"
				if err := file.Truncate(offset); err != nil {
					_ = file.Close()
					return err
				}
				break
			}
			var event ModbusRecordedEvent
			if json.Unmarshal(line, &event) != nil || event.Sequence != lastSequence+1 {
				reason = "invalid_event"
				if err := file.Truncate(offset); err != nil {
					_ = file.Close()
					return err
				}
				break
			}
			if first == 0 {
				first = event.Sequence
			}
			lastSequence, partLast = event.Sequence, event.Sequence
			_, err := tx.ExecContext(ctx, `INSERT INTO modbus_recording_events(recording_id,sequence,part_no,offset_bytes,length_bytes,kind,utc_at,elapsed_ns,unit,function,address,outcome,point_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				id, event.Sequence, part.Number, offset, len(line), event.Kind, event.Timestamp, event.ElapsedNS, event.Request.Unit, event.Request.Function, event.Request.Address, event.Result.Outcome, event.PointID)
			if err != nil {
				_ = file.Close()
				return err
			}
			offset += int64(len(line))
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			return err
		}
		hash := sha256.New()
		actual, err := io.Copy(hash, file)
		if err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		if part.Closed && part.SHA256 != digest && (reason == "core_restarted" || reason == previousReason) {
			reason = "digest_mismatch"
		}
		totalSize += actual
		if _, err := tx.ExecContext(ctx, `UPDATE modbus_recording_parts SET size_bytes=?,first_sequence=?,last_sequence=?,sha256=?,closed=1 WHERE recording_id=? AND part_no=?`, actual, first, partLast, digest, id, part.Number); err != nil {
			return err
		}
		if reason == "torn_tail" || reason == "invalid_event" || reason == "digest_mismatch" {
			break
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE modbus_recordings SET status='incomplete',reason=?,ended_at=?,last_sequence=?,size_bytes=?,recovery_done=1 WHERE id=?`, reason, time.Now().UTC().Format(time.RFC3339Nano), lastSequence, totalSize, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
