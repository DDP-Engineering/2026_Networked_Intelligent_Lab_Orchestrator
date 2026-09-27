package bench

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// SerialSettings is the complete hardware configuration bound to a connection approval.
type SerialSettings struct {
	BaudRate    int     `json:"baud_rate"`
	DataBits    int     `json:"data_bits"`
	Parity      string  `json:"parity"`
	StopBits    float64 `json:"stop_bits"`
	FlowControl string  `json:"flow_control"`
	RTS         bool    `json:"rts"`
	DTR         bool    `json:"dtr"`
}

// UnmarshalJSON resolves omitted choices before the immutable approval request
// is prepared. Explicit false line states remain false.
func (s *SerialSettings) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrInvalid
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	for _, member := range members {
		if bytes.Equal(bytes.TrimSpace(member), []byte("null")) {
			return ErrInvalid
		}
	}
	type fields SerialSettings
	value := fields{BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: 1, FlowControl: "none", RTS: true, DTR: true}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*s = SerialSettings(value)
	return nil
}

func validSerialSettings(s SerialSettings) bool {
	return s.BaudRate >= 50 && s.BaudRate <= 4000000 && s.DataBits >= 5 && s.DataBits <= 8 &&
		(s.Parity == "none" || s.Parity == "odd" || s.Parity == "even" || s.Parity == "mark" || s.Parity == "space") &&
		(s.StopBits == 1 || s.StopBits == 1.5 || s.StopBits == 2) && (s.FlowControl == "none" || s.FlowControl == "rtscts" || s.FlowControl == "xonxoff")
}

// SerialConnect requests a finite Bench job which can establish a sixty-minute lease.
type SerialConnect struct {
	Revision       int64          `json:"revision"`
	Settings       SerialSettings `json:"settings"`
	IdempotencyKey string         `json:"idempotency_key"`
	TimeoutSeconds int            `json:"timeout_seconds,omitempty"`
}
type serialConnectParameters struct {
	Revision int64          `json:"revision"`
	Settings SerialSettings `json:"settings"`
}

// SerialStatus is an authorized view of a managed port's current connection.
type SerialStatus struct {
	DeviceID     string           `json:"device_id"`
	Connected    bool             `json:"connected"`
	ConnectionID string           `json:"connection_id"`
	OwnerID      string           `json:"owner_id"`
	ExpiresAt    string           `json:"expires_at"`
	Settings     SerialSettings   `json:"settings"`
	Logging      bool             `json:"logging"`
	LogID        string           `json:"log_id"`
	RXBytes      int64            `json:"rx_bytes"`
	TXBytes      int64            `json:"tx_bytes"`
	Warning      string           `json:"warning"`
	Signals      map[string]*bool `json:"signals"`
	CanControl   bool             `json:"can_control"`
}

// SerialEvent retains original bytes and host observation timing.
type SerialEvent struct {
	Sequence   int64   `json:"sequence"`
	Timestamp  string  `json:"timestamp"`
	ElapsedMS  float64 `json:"elapsed_ms"`
	Direction  string  `json:"direction"`
	DataBase64 string  `json:"data_base64"`
	ByteCount  int     `json:"byte_count"`
	Message    string  `json:"message,omitempty"`
}

func (e SerialEvent) bytes() ([]byte, error) { return base64.StdEncoding.DecodeString(e.DataBase64) }

// SerialRead is a bounded cursor page; Gap explicitly reports evicted events.
type SerialRead struct {
	Events     []SerialEvent `json:"events"`
	NextCursor int64         `json:"next_cursor"`
	Gap        bool          `json:"gap"`
}

// SerialSend is a complete, idempotent transmission request.
type SerialSend struct {
	ConnectionID   string `json:"connection_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Encoding       string `json:"encoding"`
	Data           string `json:"data"`
	Ending         string `json:"ending"`
}

// SerialSendResult reports OS acceptance, never acknowledgement by the target.
type SerialSendResult struct {
	IdempotencyKey string `json:"idempotency_key"`
	Status         string `json:"status"`
	BytesWritten   int    `json:"bytes_written"`
	Error          string `json:"error,omitempty"`
}

func serialPayload(input SerialSend) ([]byte, error) {
	if !keyPattern.MatchString(input.IdempotencyKey) || len(input.Data) > 16384 {
		return nil, ErrInvalid
	}
	var b []byte
	switch input.Encoding {
	case "ascii":
		for _, r := range input.Data {
			if r > 127 {
				return nil, ErrInvalid
			}
		}
		b = []byte(input.Data)
	case "utf8":
		if !utf8.ValidString(input.Data) {
			return nil, ErrInvalid
		}
		b = []byte(input.Data)
	case "hex":
		parts := strings.Fields(input.Data)
		for _, part := range parts {
			if len(part)%2 != 0 {
				return nil, ErrInvalid
			}
		}
		compact := strings.Join(parts, "")
		var err error
		b, err = hex.DecodeString(compact)
		if err != nil {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	switch input.Ending {
	case "none":
	case "cr":
		b = append(b, 13)
	case "lf":
		b = append(b, 10)
	case "crlf":
		b = append(b, 13, 10)
	default:
		return nil, ErrInvalid
	}
	if len(b) == 0 || len(b) > 4096 {
		return nil, ErrInvalid
	}
	return b, nil
}

// SerialAction starts or stops logging within an authorized connection.
type SerialAction struct {
	ConnectionID string `json:"connection_id"`
	Action       string `json:"action"`
}

// SerialSignals requests supported manually controlled line changes.
type SerialSignals struct {
	ConnectionID string `json:"connection_id"`
	RTS          *bool  `json:"rts,omitempty"`
	DTR          *bool  `json:"dtr,omitempty"`
}

// SerialRetention bounds retained files and their actual encoded disk sizes.
type SerialRetention struct {
	Files      int   `json:"files"`
	TotalBytes int64 `json:"total_bytes"`
	FileBytes  int64 `json:"file_bytes"`
}

// SerialRetentionState includes revision and usage for optimistic configuration.
type SerialRetentionState struct {
	Revision   int64           `json:"revision"`
	Limits     SerialRetention `json:"limits"`
	UsageBytes int64           `json:"usage_bytes"`
}

func defaultSerialRetention() SerialRetention { return SerialRetention{100, 2 << 30, 100 << 20} }

// SerialLog describes one independently retained segment of a connection.
type SerialLog struct {
	ID           string `json:"id"`
	DeviceID     string `json:"device_id"`
	ConnectionID string `json:"connection_id"`
	OwnerID      string `json:"owner_id"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	Status       string `json:"status"`
	RXBytes      int64  `json:"rx_bytes"`
	TXBytes      int64  `json:"tx_bytes"`
	SizeBytes    int64  `json:"size_bytes"`
}

// SerialLogPage is an owner-filtered page of retained segments.
type SerialLogPage struct {
	Logs   []SerialLog `json:"logs"`
	Total  int         `json:"total"`
	Offset int         `json:"offset"`
}
