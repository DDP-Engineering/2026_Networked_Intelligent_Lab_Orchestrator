package modbus

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// DecodedValue is a transport-independent typed interpretation of raw words.
type DecodedValue struct {
	Type     string   `json:"type"`
	Value    any      `json:"value"`
	RawWords []uint16 `json:"raw_words"`
	Quality  string   `json:"quality"`
	Label    string   `json:"label,omitempty"`
	SetBits  []string `json:"set_bits,omitempty"`
}

func orderedBytes(words []uint16, order string) []byte {
	out := make([]byte, len(words)*2)
	for i, w := range words {
		binary.BigEndian.PutUint16(out[i*2:], w)
	}
	if strings.Contains(order, "reverse_words") {
		for i, j := 0, len(out)-2; i < j; i, j = i+2, j-2 {
			out[i], out[i+1], out[j], out[j+1] = out[j], out[j+1], out[i], out[i+1]
		}
	}
	if strings.Contains(order, "swap_bytes") {
		for i := 0; i < len(out); i += 2 {
			out[i], out[i+1] = out[i+1], out[i]
		}
	}
	return out
}

func bytesWords(data []byte, order string) []uint16 {
	b := append([]byte(nil), data...)
	if strings.Contains(order, "swap_bytes") {
		for i := 0; i < len(b); i += 2 {
			b[i], b[i+1] = b[i+1], b[i]
		}
	}
	if strings.Contains(order, "reverse_words") {
		for i, j := 0, len(b)-2; i < j; i, j = i+2, j-2 {
			b[i], b[i+1], b[j], b[j+1] = b[j], b[j+1], b[i], b[i+1]
		}
	}
	out := make([]uint16, len(b)/2)
	for i := range out {
		out[i] = binary.BigEndian.Uint16(b[i*2:])
	}
	return out
}

func nonFinite(f float64) any {
	if math.IsNaN(f) {
		return map[string]string{"non_finite": "nan"}
	}
	if math.IsInf(f, 1) {
		return map[string]string{"non_finite": "positive_infinity"}
	}
	if math.IsInf(f, -1) {
		return map[string]string{"non_finite": "negative_infinity"}
	}
	return f
}

func engineering(p Point, raw *big.Rat) *big.Rat {
	scale, _ := finiteDecimal(p.Scale)
	offset, _ := finiteDecimal(p.Offset)
	return new(big.Rat).Add(new(big.Rat).Mul(raw, scale), offset)
}

func ratValue(r *big.Rat, exactString bool) any {
	if exactString {
		if r.IsInt() {
			return r.Num().String()
		}
		return strings.TrimRight(strings.TrimRight(r.FloatString(256), "0"), ".")
	}
	f, _ := r.Float64()
	return f
}

// DecodePoint interprets exact bus words without consulting any transport.
func DecodePoint(point Point, words []uint16) (DecodedValue, error) {
	p, err := point.Canonical()
	result := DecodedValue{Type: point.Type, RawWords: append([]uint16(nil), words...), Quality: "decode_error"}
	if err != nil || len(words) != p.Width {
		return result, ErrInvalid
	}
	b := orderedBytes(words, p.Order)
	if p.Type == "bool" {
		if words[0] > 1 {
			return result, ErrInvalid
		}
		result.Value = words[0] == 1
		result.Quality = "good"
		return result, nil
	}
	if p.Type == "ascii" {
		pad := byte(0)
		if p.Padding == "space" {
			pad = ' '
		}
		end := len(b)
		for end > 0 && b[end-1] == pad {
			end--
		}
		for _, v := range b[:end] {
			if v < 32 || v > 126 {
				return result, ErrInvalid
			}
		}
		result.Value = string(b[:end])
		result.Quality = "good"
		return result, nil
	}
	if p.Type == "float32" || p.Type == "float64" {
		var f float64
		if p.Type == "float32" {
			f = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
		} else {
			f = math.Float64frombits(binary.BigEndian.Uint64(b))
		}
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			s, _ := p.Scale.Float64()
			o, _ := p.Offset.Float64()
			f = f*s + o
		}
		result.Value = nonFinite(f)
		result.Quality = "good"
		return result, nil
	}
	var u uint64
	for _, v := range b {
		u = (u << 8) | uint64(v)
	}
	raw := new(big.Int).SetUint64(u)
	bits := uint(len(b) * 8)
	if strings.HasPrefix(p.Type, "int") && u&(uint64(1)<<(bits-1)) != 0 {
		raw.Sub(raw, new(big.Int).Lsh(big.NewInt(1), bits))
	}
	v := engineering(p, new(big.Rat).SetInt(raw))
	result.Value = ratValue(v, p.Type == "int64" || p.Type == "uint64")
	if p.Type == "enum" {
		result.Label = p.Labels[strconv.FormatUint(u, 10)]
	}
	if p.Type == "bits" {
		for i := 0; i < 16; i++ {
			if u&(1<<i) != 0 {
				label := p.Labels[strconv.Itoa(i)]
				if label == "" {
					label = strconv.Itoa(i)
				}
				result.SetBits = append(result.SetBits, label)
			}
		}
	}
	result.Quality = "good"
	return result, nil
}

func inputRat(raw json.RawMessage) (*big.Rat, bool) {
	var n json.Number
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, false
		}
		n = json.Number(s)
	} else {
		n = json.Number(raw)
	}
	return finiteDecimal(n)
}

func withinWriteBounds(p Point, value *big.Rat) bool {
	if p.WriteMin != nil {
		minimum, _ := finiteDecimal(*p.WriteMin)
		if value.Cmp(minimum) < 0 {
			return false
		}
	}
	if p.WriteMax != nil {
		maximum, _ := finiteDecimal(*p.WriteMax)
		if value.Cmp(maximum) > 0 {
			return false
		}
	}
	return true
}

// EncodePoint previews exact raw words; sending them requires a separate live policy action.
func EncodePoint(point Point, value json.RawMessage) ([]uint16, error) {
	p, err := point.Canonical()
	if err != nil || p.Access == "read" {
		return nil, ErrInvalid
	}
	if p.Type == "bool" {
		var b bool
		if json.Unmarshal(value, &b) != nil || (string(value) != "true" && string(value) != "false") {
			return nil, ErrInvalid
		}
		value := big.NewRat(0, 1)
		if b {
			value = big.NewRat(1, 1)
		}
		if !withinWriteBounds(p, value) {
			return nil, ErrInvalid
		}
		if b {
			return []uint16{1}, nil
		}
		return []uint16{0}, nil
	}
	if p.Type == "ascii" {
		var s string
		if json.Unmarshal(value, &s) != nil || len(s) > p.Width*2 {
			return nil, ErrInvalid
		}
		out := make([]byte, p.Width*2)
		if p.Padding == "space" {
			for i := range out {
				out[i] = ' '
			}
		}
		for i, v := range []byte(s) {
			if v < 32 || v > 126 {
				return nil, ErrInvalid
			}
			out[i] = v
		}
		return bytesWords(out, p.Order), nil
	}
	eng, ok := inputRat(value)
	if !ok {
		return nil, ErrInvalid
	}
	if !withinWriteBounds(p, eng) {
		return nil, ErrInvalid
	}
	scale, _ := finiteDecimal(p.Scale)
	if scale.Sign() == 0 {
		return nil, ErrInvalid
	}
	offset, _ := finiteDecimal(p.Offset)
	raw := new(big.Rat).Quo(new(big.Rat).Sub(eng, offset), scale)
	data := make([]byte, p.Width*2)
	if p.Type == "float32" || p.Type == "float64" {
		f, _ := raw.Float64()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, ErrInvalid
		}
		if p.Type == "float32" {
			if math.IsInf(float64(float32(f)), 0) {
				return nil, ErrInvalid
			}
			binary.BigEndian.PutUint32(data, math.Float32bits(float32(f)))
		} else {
			binary.BigEndian.PutUint64(data, math.Float64bits(f))
		}
		return bytesWords(data, p.Order), nil
	}
	if !raw.IsInt() {
		return nil, ErrInvalid
	}
	n := raw.Num()
	bits := uint(p.Width * 16)
	if strings.HasPrefix(p.Type, "uint") || p.Type == "enum" || p.Type == "bits" {
		if n.Sign() < 0 || n.BitLen() > int(bits) {
			return nil, ErrInvalid
		}
	} else {
		limit := new(big.Int).Lsh(big.NewInt(1), bits-1)
		if n.Cmp(new(big.Int).Neg(limit)) < 0 || n.Cmp(limit) >= 0 {
			return nil, ErrInvalid
		}
	}
	if p.Type == "enum" && len(p.Labels) > 0 && p.Labels[n.String()] == "" {
		return nil, ErrInvalid
	}
	u := new(big.Int).Set(n)
	if u.Sign() < 0 {
		u.Add(u, new(big.Int).Lsh(big.NewInt(1), bits))
	}
	u.FillBytes(data)
	return bytesWords(data, p.Order), nil
}
