package modbus

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

func testPoint(kind string, width int) Point {
	return Point{ID: "speed", Name: "Speed", Area: "holding", Address: 0, Type: kind, Width: width, Order: "normal", Scale: json.Number("1"), Offset: json.Number("0"), Access: "read_write", Labels: map[string]string{}}
}

func TestAddressBasesAndPointBounds(t *testing.T) {
	for _, tc := range []struct {
		area, base    string
		address, want int
	}{{"holding", "zero", 0, 0}, {"holding", "one", 1, 0}, {"holding", "reference", 40001, 0}, {"input", "reference", 30001, 0}, {"input", "reference", 40001, 10000}} {
		got, err := WireAddress(tc.area, tc.base, tc.address)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %d %v", tc, got, err)
		}
	}
	if _, err := WireAddress("holding", "one", 0); err == nil {
		t.Fatal("accepted base-one zero")
	}
	if _, err := WireAddress("input", "reference", 30000); err == nil {
		t.Fatal("accepted below input reference start")
	}
	p := testPoint("uint32", 2)
	p.Address = 65535
	if _, err := p.Canonical(); err == nil {
		t.Fatal("accepted span overflow")
	}
}

func TestCodecGolden(t *testing.T) {
	p := testPoint("uint32", 2)
	for _, tc := range []struct {
		order string
		words []uint16
	}{{"normal", []uint16{0x1234, 0x5678}}, {"reverse_words", []uint16{0x5678, 0x1234}}, {"swap_bytes", []uint16{0x3412, 0x7856}}, {"reverse_words_swap_bytes", []uint16{0x7856, 0x3412}}} {
		p.Order = tc.order
		got, err := DecodePoint(p, tc.words)
		if err != nil || got.Value != float64(0x12345678) {
			t.Fatalf("%s: %+v %v", tc.order, got, err)
		}
		raw, err := EncodePoint(p, json.RawMessage(`305419896`))
		if err != nil || raw[0] != tc.words[0] || raw[1] != tc.words[1] {
			t.Fatalf("%s encode %v %v", tc.order, raw, err)
		}
	}
	p = testPoint("uint64", 4)
	got, err := DecodePoint(p, []uint16{0xffff, 0xffff, 0xffff, 0xffff})
	if err != nil || got.Value != "18446744073709551615" {
		t.Fatalf("uint64: %+v %v", got, err)
	}
	raw, err := EncodePoint(p, json.RawMessage(`"18446744073709551615"`))
	if err != nil || len(raw) != 4 {
		t.Fatalf("uint64 encode: %v %v", raw, err)
	}
	p = testPoint("float32", 2)
	bits := math.Float32bits(float32(math.NaN()))
	got, err = DecodePoint(p, []uint16{uint16(bits >> 16), uint16(bits)})
	if err != nil || got.Quality != "good" {
		t.Fatalf("nan: %+v %v", got, err)
	}
	encoded, _ := json.Marshal(got)
	if !json.Valid(encoded) {
		t.Fatalf("invalid json %s", encoded)
	}
}

func TestCodecScaleAndWriteBounds(t *testing.T) {
	p := testPoint("int16", 1)
	p.Scale = "0.1"
	p.Offset = "-1"
	decoded, err := DecodePoint(p, []uint16{42})
	if err != nil || decoded.Value != 3.2 {
		t.Fatal(decoded, err)
	}
	words, err := EncodePoint(p, json.RawMessage(`3.2`))
	if err != nil || words[0] != 42 {
		t.Fatal(words, err)
	}
	if _, err = EncodePoint(p, json.RawMessage(`3.25`)); err == nil {
		t.Fatal("accepted fractional raw value")
	}
	min, max := json.Number("3"), json.Number("4")
	p.WriteMin = &min
	p.WriteMax = &max
	if _, err = EncodePoint(p, json.RawMessage(`2.9`)); err == nil {
		t.Fatal("accepted write below engineering minimum")
	}
	p.Scale = "0"
	if _, err = EncodePoint(p, json.RawMessage(`3.2`)); err == nil {
		t.Fatal("accepted inverse zero scale")
	}
	p = testPoint("int64", 4)
	value, err := DecodePoint(p, []uint16{0x8000, 0, 0, 0})
	if err != nil || value.Value != "-9223372036854775808" {
		t.Fatal(value, err)
	}
	p.Scale = "0.00000000000000000001"
	value, err = DecodePoint(p, []uint16{0, 0, 0, 1})
	if err != nil || value.Value != "0.00000000000000000001" {
		t.Fatal("exact scaled decimal", value, err)
	}
	p.Scale = "1e-20"
	value, err = DecodePoint(p, []uint16{0, 0, 0, 1})
	if err != nil || value.Value != "0.00000000000000000001" {
		t.Fatal("scientific scale", value, err)
	}
}

func TestCodecASCIIEnumAndBits(t *testing.T) {
	p := testPoint("ascii", 2)
	p.Padding = "space"
	words, err := EncodePoint(p, json.RawMessage(`"AB"`))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePoint(p, words)
	if err != nil || decoded.Value != "AB" {
		t.Fatal(decoded, err)
	}
	if _, err = EncodePoint(p, json.RawMessage(`"É"`)); err == nil {
		t.Fatal("accepted non-ASCII")
	}
	p = testPoint("enum", 1)
	p.Labels = map[string]string{"2": "On"}
	decoded, err = DecodePoint(p, []uint16{2})
	if err != nil || decoded.Label != "On" {
		t.Fatal(decoded, err)
	}
	p = testPoint("bits", 1)
	p.Labels = map[string]string{"0": "Ready", "3": "Alarm"}
	decoded, err = DecodePoint(p, []uint16{9})
	if err != nil || len(decoded.SetBits) != 2 || decoded.SetBits[0] != "Ready" || decoded.SetBits[1] != "Alarm" {
		t.Fatal(decoded, err)
	}
}

func TestCodecRemainingTypeExtrema(t *testing.T) {
	p := testPoint("bool", 1)
	p.Area = "coil"
	decoded, err := DecodePoint(p, []uint16{1})
	if err != nil || decoded.Value != true {
		t.Fatal("bool decode", decoded, err)
	}
	words, err := EncodePoint(p, json.RawMessage(`true`))
	if err != nil || words[0] != 1 {
		t.Fatal("bool encode", words, err)
	}
	for _, tc := range []struct {
		kind  string
		width int
		raw   []uint16
		want  float64
	}{
		{"uint16", 1, []uint16{0xffff}, 65535},
		{"int16", 1, []uint16{0x8000}, -32768},
		{"int32", 2, []uint16{0x8000, 0}, -2147483648},
		{"uint32", 2, []uint16{0xffff, 0xffff}, 4294967295},
	} {
		p = testPoint(tc.kind, tc.width)
		decoded, err = DecodePoint(p, tc.raw)
		if err != nil || decoded.Value != tc.want {
			t.Fatalf("%s: %+v %v", tc.kind, decoded, err)
		}
	}
	p = testPoint("float64", 4)
	pi := math.Float64bits(math.Pi)
	raw := []uint16{uint16(pi >> 48), uint16(pi >> 32), uint16(pi >> 16), uint16(pi)}
	decoded, err = DecodePoint(p, raw)
	if err != nil || decoded.Value != math.Pi {
		t.Fatal("float64", decoded, err)
	}
	infinity := math.Float64bits(math.Inf(1))
	raw = []uint16{uint16(infinity >> 48), uint16(infinity >> 32), uint16(infinity >> 16), uint16(infinity)}
	decoded, err = DecodePoint(p, raw)
	if err != nil || decoded.Quality != "good" {
		t.Fatal("infinity", decoded, err)
	}
	encoded, _ := json.Marshal(decoded)
	if !bytes.Contains(encoded, []byte(`positive_infinity`)) {
		t.Fatal("missing infinity tag", string(encoded))
	}
}

func TestBooleanAndASCIIWriteLimits(t *testing.T) {
	p := testPoint("bool", 1)
	p.Area = "coil"
	one := json.Number("1")
	p.WriteMin = &one
	if _, err := EncodePoint(p, json.RawMessage(`false`)); err == nil {
		t.Fatal("Boolean bypassed write minimum")
	}
	if _, err := EncodePoint(p, json.RawMessage(`true`)); err != nil {
		t.Fatal("bounded true", err)
	}
	p = testPoint("ascii", 2)
	p.WriteMin = &one
	if _, err := p.Canonical(); err == nil {
		t.Fatal("ASCII silently accepts numeric write limit")
	}
	p.WriteMin = nil
	p.Scale = "1.0"
	p.Offset = "0.0"
	if _, err := p.Canonical(); err != nil {
		t.Fatal("numeric identity rejected", err)
	}
}
