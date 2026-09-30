package bench

import (
	"context"
	"encoding/json"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
)

// ModbusEncodedPoint contains exact words from one pinned map point, before any I/O.
type ModbusEncodedPoint struct {
	PointID string   `json:"point_id"`
	Area    string   `json:"area"`
	Address int      `json:"address"`
	Words   []uint16 `json:"words"`
}

func (m *Manager) modbusPinnedPoint(ctx context.Context, actor identity.Actor, mapID string, revision int64, pointID string) (modbus.Point, error) {
	item, err := m.ModbusMapRevision(ctx, actor, mapID, revision)
	if err != nil {
		return modbus.Point{}, err
	}
	for _, point := range item.Points {
		if point.ID == pointID {
			return point, nil
		}
	}
	return modbus.Point{}, ErrNotFound
}

// DecodeModbusPoint applies one immutable map revision to exact raw values without I/O.
func (m *Manager) DecodeModbusPoint(ctx context.Context, actor identity.Actor, mapID string, revision int64, pointID string, words []uint16) (modbus.DecodedValue, error) {
	point, err := m.modbusPinnedPoint(ctx, actor, mapID, revision, pointID)
	if err != nil {
		return modbus.DecodedValue{}, err
	}
	value, err := modbus.DecodePoint(point, words)
	if err != nil {
		return value, ErrInvalid
	}
	return value, nil
}

// EncodeModbusPoint validates one engineering value against a pinned map without I/O.
func (m *Manager) EncodeModbusPoint(ctx context.Context, actor identity.Actor, mapID string, revision int64, pointID string, value json.RawMessage) (ModbusEncodedPoint, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return ModbusEncodedPoint{}, err
	}
	point, err := m.modbusPinnedPoint(ctx, actor, mapID, revision, pointID)
	if err != nil {
		return ModbusEncodedPoint{}, err
	}
	words, err := modbus.EncodePoint(point, value)
	if err != nil {
		return ModbusEncodedPoint{}, ErrInvalid
	}
	return ModbusEncodedPoint{PointID: point.ID, Area: point.Area, Address: point.Address, Words: words}, nil
}
