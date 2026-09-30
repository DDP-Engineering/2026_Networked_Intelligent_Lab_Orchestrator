package modbus

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var decimalLexeme = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// Point is one named, zero-based wire coordinate and its value contract.
type Point struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Area        string            `json:"area"`
	Address     int               `json:"address"`
	Type        string            `json:"type"`
	Width       int               `json:"width"`
	Order       string            `json:"order"`
	Unit        string            `json:"unit"`
	Scale       json.Number       `json:"scale"`
	Offset      json.Number       `json:"offset"`
	Precision   *int              `json:"precision,omitempty"`
	Access      string            `json:"access"`
	WriteMin    *json.Number      `json:"write_min"`
	WriteMax    *json.Number      `json:"write_max"`
	Labels      map[string]string `json:"labels"`
	Padding     string            `json:"padding,omitempty"`
}

func (p *Point) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return ErrInvalid
	}
	for _, name := range []string{"id", "name", "area", "address", "type", "width", "order", "unit", "scale", "offset", "access", "write_min", "write_max", "labels"} {
		raw, ok := fields[name]
		if !ok || (name == "labels" && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return ErrInvalid
		}
	}
	type alias Point
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var value alias
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	*p = Point(value)
	return nil
}

// Map is a shared, creator-owned definition; its points never create I/O.
type Map struct {
	SchemaVersion int     `json:"schema_version"`
	ID            string  `json:"id"`
	Revision      int64   `json:"revision"`
	OwnerID       string  `json:"owner_id"`
	Name          string  `json:"name"`
	Points        []Point `json:"points"`
}

func finiteDecimal(n json.Number) (*big.Rat, bool) {
	if n == "" || len(n) > 128 || !decimalLexeme.MatchString(string(n)) {
		return nil, false
	}
	raw := string(n)
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		exponent, err := strconv.Atoi(raw[index+1:])
		if err != nil || exponent < -128 || exponent > 128 {
			return nil, false
		}
	}
	r, ok := new(big.Rat).SetString(string(n))
	return r, ok
}

func validText(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// WireAddress converts an explicit human notation to a zero-based wire offset.
func WireAddress(area, base string, address int) (int, error) {
	start := 0
	switch area {
	case "coil":
		start = 1
	case "discrete":
		start = 10001
	case "input":
		start = 30001
	case "holding":
		start = 40001
	default:
		return 0, ErrInvalid
	}
	offset := address
	switch base {
	case "zero":
	case "one":
		offset--
	case "reference":
		offset -= start
	default:
		return 0, ErrInvalid
	}
	if offset < 0 || offset > 65535 {
		return 0, ErrInvalid
	}
	return offset, nil
}

func (p Point) Canonical() (Point, error) {
	if !identifier.MatchString(p.ID) || !validName(p.Name) || !validText(p.Description, 512) || !validText(p.Unit, 64) || p.Address < 0 || p.Address > 65535 || p.Width < 1 || p.Width > 125 || p.Address+p.Width-1 > 65535 {
		return Point{}, ErrInvalid
	}
	switch p.Order {
	case "normal", "reverse_words", "swap_bytes", "reverse_words_swap_bytes":
	default:
		return Point{}, ErrInvalid
	}
	switch p.Access {
	case "read", "write", "read_write":
	default:
		return Point{}, ErrInvalid
	}
	switch p.Area {
	case "coil", "discrete":
		if p.Type != "bool" || p.Width != 1 {
			return Point{}, ErrInvalid
		}
	case "holding", "input":
		if p.Type == "bool" {
			return Point{}, ErrInvalid
		}
	default:
		return Point{}, ErrInvalid
	}
	if (p.Area == "discrete" || p.Area == "input") && p.Access != "read" {
		return Point{}, ErrInvalid
	}
	want := 0
	switch p.Type {
	case "bool", "int16", "uint16", "enum", "bits":
		want = 1
	case "int32", "uint32", "float32":
		want = 2
	case "int64", "uint64", "float64":
		want = 4
	case "ascii":
		want = p.Width
	default:
		return Point{}, ErrInvalid
	}
	if p.Width != want {
		return Point{}, ErrInvalid
	}
	if _, ok := finiteDecimal(p.Scale); !ok {
		return Point{}, ErrInvalid
	}
	if _, ok := finiteDecimal(p.Offset); !ok {
		return Point{}, ErrInvalid
	}
	if p.Precision != nil && (*p.Precision < 0 || *p.Precision > 12) {
		return Point{}, ErrInvalid
	}
	for _, bound := range []*json.Number{p.WriteMin, p.WriteMax} {
		if bound != nil {
			if _, ok := finiteDecimal(*bound); !ok {
				return Point{}, ErrInvalid
			}
		}
	}
	if p.WriteMin != nil && p.WriteMax != nil {
		a, _ := finiteDecimal(*p.WriteMin)
		b, _ := finiteDecimal(*p.WriteMax)
		if a.Cmp(b) > 0 {
			return Point{}, ErrInvalid
		}
	}
	if p.Type != "ascii" && p.Padding != "" {
		return Point{}, ErrInvalid
	}
	if p.Type == "ascii" && p.Padding != "" && p.Padding != "nul" && p.Padding != "space" {
		return Point{}, ErrInvalid
	}
	if p.Labels == nil {
		p.Labels = map[string]string{}
	}
	if p.Type != "enum" && p.Type != "bits" && len(p.Labels) > 0 {
		return Point{}, ErrInvalid
	}
	for key, label := range p.Labels {
		n, err := strconv.ParseUint(key, 10, 16)
		if err != nil || strconv.FormatUint(n, 10) != key || !validText(label, 128) {
			return Point{}, ErrInvalid
		}
		if p.Type == "bits" && n > 15 {
			return Point{}, ErrInvalid
		}
	}
	if p.Type == "ascii" || p.Type == "bool" {
		scale, _ := finiteDecimal(p.Scale)
		offset, _ := finiteDecimal(p.Offset)
		if scale.Cmp(big.NewRat(1, 1)) != 0 || offset.Sign() != 0 {
			return Point{}, ErrInvalid
		}
	}
	if p.Type == "ascii" && (p.WriteMin != nil || p.WriteMax != nil) {
		return Point{}, ErrInvalid
	}
	return p, nil
}

func (m Map) Canonical() (Map, error) {
	if m.SchemaVersion != 1 || !identifier.MatchString(m.ID) || !identifier.MatchString(m.OwnerID) || m.Revision < 1 || m.Revision > 2147483647 || !validName(m.Name) || len(m.Points) > 4096 {
		return Map{}, ErrInvalid
	}
	seen := make(map[string]bool, len(m.Points))
	points := make([]Point, 0, len(m.Points))
	for _, p := range m.Points {
		c, err := p.Canonical()
		if err != nil || seen[p.ID] {
			return Map{}, ErrInvalid
		}
		seen[p.ID] = true
		points = append(points, c)
	}
	m.Points = points
	raw, _ := json.Marshal(m)
	if len(raw) > 1<<20 {
		return Map{}, ErrInvalid
	}
	return m, nil
}

// NamedPoint finds one point without exposing a mutable backing slice.
func (m Map) NamedPoint(id string) (Point, bool) {
	for _, p := range m.Points {
		if strings.EqualFold(p.ID, id) {
			return p, true
		}
	}
	return Point{}, false
}
