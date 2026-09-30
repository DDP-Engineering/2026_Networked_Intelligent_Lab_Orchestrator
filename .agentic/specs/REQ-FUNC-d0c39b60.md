# REQ-FUNC-d0c39b60 — Typed Modbus point codec

## Behavior

`DecodePoint` accepts a canonical point and exactly `width` raw 16-bit words (or
one coil/discrete bit represented as 0 or 1). It retains the raw words and
returns quality `good` with an interpreted JSON value. Malformed word counts,
invalid ASCII and invalid point definitions return `decode_error` without
inventing a numeric zero. It never reads a transport.

`EncodePoint` accepts an explicit typed JSON value, applies the inverse of
`engineering = raw * scale + offset`, verifies write access and inclusive
engineering write limits, and returns exact raw words. A caller submits those
words through a separate policy-controlled write action. Zero scale, nonfinite
input, fractional inverse for integer types, overflow, invalid enum/bit
selection, and read-only points are errors before any command is reserved.
Boolean write limits compare against 0 or 1. ASCII points reject numeric
write limits at definition validation because no numeric engineering value exists.

## Data / interface

Points use the MB-01 `id,name,area,address,type,width,order,unit,scale,offset,
access,write_min,write_max,labels` fields. Optional `description` (at most 512
Unicode code points), `precision` (0..12) and `padding` (`nul` or `space`, ASCII
only) refine display/ASCII behavior. Scale, offset and bounds are finite JSON
decimal numbers parsed without a binary-float intermediate for integer
conversion. Width is 1 for bool/int16/uint16/enum/bits, 2 for 32-bit, 4 for
64-bit and 1..125 for fixed ASCII. Float32/64 use widths 2/4. Boolean uses
coil/discrete areas; all others use holding/input. Input/discrete points are
read-only. Enum labels use decimal unsigned code keys; bits labels use decimal
bit-position keys 0..15. Labels are empty for other types. Writable bitfields
are an encoding preview only: a later live caller must explicitly select the
non-atomic FC06/16 path or a supported mask operation.

Raw words are big-endian bytes in bus order. `normal` means `[0x1234,0x5678]`
represents `0x12345678`; `reverse_words` swaps word positions,
`swap_bytes` swaps the two bytes of each word, and
`reverse_words_swap_bytes` does both. Encoding reverses the same involution.
Signed integers use two's complement; IEEE floats use binary32/64. ASCII uses
fixed `width*2` bytes, with trailing NUL or space padding removed on decode;
unprintable/non-ASCII bytes yield `decode_error` and retain raw words. An
oversize or non-ASCII string cannot be encoded.

Decoded JSON is `{type,value,raw_words,quality,label?,set_bits?}`. Numeric
16/32, enums and bits use JSON numbers when integral; 64-bit integer values
use exact decimal strings, including scaled results. Finite float values use
JSON numbers. IEEE NaN, positive infinity and negative infinity use
`{"non_finite":"nan"}`, `"positive_infinity"` or `"negative_infinity"`
inside `value`, never invalid JSON numeric tokens; quality remains `good`.
Enum `label` and bit `set_bits` are supplemental display metadata. Scale and
offset are the canonical decimal number lexemes represented by the map. For
integer types, rational arithmetic makes inverse-scale and bounds checks exact.

## Examples

- `uint32`, normal order, raw `[0x1234,0x5678]` decodes to `305419896`;
  reverse words expects raw `[0x5678,0x1234]` for the same value.
- `int64` raw `0x7fffffffffffffff`, identity scale, returns
  `"9223372036854775807"`; uint64 max returns
  `"18446744073709551615"`.
- int16 raw 42 with scale 0.1 and offset -1 yields 3.2. Encoding 3.2
  returns raw 42; encoding 3.25 fails because the inverse is fractional.
- Float32 raw NaN returns the tagged value, quality `good`, and the original
  two words. Encoding a NaN tag is refused as a write.

## Acceptance

Golden vectors prove the four orders and every type family, 64-bit exactness,
nonfinite tags, reversible ASCII padding, enum/bit labels, and write boundary
rejections without starting a session or invoking a runner.
