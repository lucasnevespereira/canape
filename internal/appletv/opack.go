package appletv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// OPACK is Apple's binary format for Companion messages, a compact JSON.
// Only the types the Apple TV sends are handled: nil, bools, ints, floats,
// strings, bytes, arrays and dicts. Decoding also follows back-references,
// which the Apple TV uses for values it already sent in the same message.

func opackEncode(v any) ([]byte, error) {
	var b []byte
	return opackAppend(b, v)
}

func opackAppend(b []byte, v any) ([]byte, error) {
	switch v := v.(type) {
	case nil:
		return append(b, 0x04), nil
	case bool:
		if v {
			return append(b, 0x01), nil
		}
		return append(b, 0x02), nil
	case int:
		return opackInt(b, int64(v)), nil
	case int64:
		return opackInt(b, v), nil
	case uint64:
		return opackInt(b, int64(v)), nil
	case float64:
		b = append(b, 0x36)
		return binary.LittleEndian.AppendUint64(b, math.Float64bits(v)), nil
	case string:
		n := len(v)
		switch {
		case n <= 0x20:
			b = append(b, byte(0x40+n))
		case n <= 0xff:
			b = append(b, 0x61, byte(n))
		case n <= 0xffff:
			b = binary.LittleEndian.AppendUint16(append(b, 0x62), uint16(n))
		default:
			b = binary.LittleEndian.AppendUint32(append(b, 0x64), uint32(n))
		}
		return append(b, v...), nil
	case []byte:
		n := len(v)
		switch {
		case n <= 0x20:
			b = append(b, byte(0x70+n))
		case n <= 0xff:
			b = append(b, 0x91, byte(n))
		case n <= 0xffff:
			b = binary.LittleEndian.AppendUint16(append(b, 0x92), uint16(n))
		default:
			b = binary.LittleEndian.AppendUint32(append(b, 0x93), uint32(n))
		}
		return append(b, v...), nil
	case []any:
		if len(v) < 15 {
			b = append(b, byte(0xd0+len(v)))
		} else {
			b = append(b, 0xdf)
		}
		for _, e := range v {
			var err error
			if b, err = opackAppend(b, e); err != nil {
				return nil, err
			}
		}
		if len(v) >= 15 {
			b = append(b, 0x03)
		}
		return b, nil
	case map[string]any:
		if len(v) < 15 {
			b = append(b, byte(0xe0+len(v)))
		} else {
			b = append(b, 0xef)
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys) // stable bytes, easier to test
		for _, k := range keys {
			var err error
			if b, err = opackAppend(b, k); err != nil {
				return nil, err
			}
			if b, err = opackAppend(b, v[k]); err != nil {
				return nil, err
			}
		}
		if len(v) >= 15 {
			b = append(b, 0x03)
		}
		return b, nil
	}
	return nil, fmt.Errorf("opack: can't encode %T", v)
}

func opackInt(b []byte, v int64) []byte {
	switch {
	case v >= 0 && v <= 0x27:
		return append(b, byte(0x08+v))
	case v >= 0 && v <= 0xff:
		return append(b, 0x30, byte(v))
	case v >= 0 && v <= 0xffff:
		return binary.LittleEndian.AppendUint16(append(b, 0x31), uint16(v))
	case v >= 0 && v <= 0xffffffff:
		return binary.LittleEndian.AppendUint32(append(b, 0x32), uint32(v))
	}
	return binary.LittleEndian.AppendUint64(append(b, 0x33), uint64(v))
}

var errShort = errors.New("opack: message cut short")

// opackDecode returns the first value in b.
func opackDecode(b []byte) (any, error) {
	var refs []any
	v, _, err := opackNext(b, &refs)
	return v, err
}

func opackNext(b []byte, refs *[]any) (v any, rest []byte, err error) {
	if len(b) == 0 {
		return nil, nil, errShort
	}
	t := b[0]
	b = b[1:]
	take := func(n int) ([]byte, error) {
		if n < 0 || len(b) < n {
			return nil, errShort
		}
		out := b[:n]
		b = b[n:]
		return out, nil
	}
	le := func(n int) (uint64, error) {
		raw, err := take(n)
		if err != nil {
			return 0, err
		}
		var x uint64
		for i := len(raw) - 1; i >= 0; i-- {
			x = x<<8 | uint64(raw[i])
		}
		return x, nil
	}
	remember := true
	switch {
	case t == 0x01, t == 0x02:
		v, remember = t == 0x01, false
	case t == 0x04:
		v, remember = nil, false
	case t == 0x05: // UUID
		v, err = take(16)
	case t == 0x06: // date, seconds since 2001 as a float64
		var x uint64
		x, err = le(8)
		v = math.Float64frombits(x)
	case t >= 0x08 && t <= 0x2f:
		v, remember = int64(t-0x08), false
	case t == 0x35:
		var x uint64
		x, err = le(4)
		v = float64(math.Float32frombits(uint32(x)))
	case t == 0x36:
		var x uint64
		x, err = le(8)
		v = math.Float64frombits(x)
	case t >= 0x30 && t <= 0x33:
		var x uint64
		x, err = le(1 << (t & 0x0f))
		v = int64(x)
	case t >= 0x40 && t <= 0x60:
		var s []byte
		s, err = take(int(t - 0x40))
		v = string(s)
	case t >= 0x61 && t <= 0x64:
		var n uint64
		if n, err = le(int(t - 0x60)); err == nil {
			var s []byte
			s, err = take(int(n))
			v = string(s)
		}
	case t >= 0x70 && t <= 0x90:
		v, err = take(int(t - 0x70))
	case t >= 0x91 && t <= 0x94:
		var n uint64
		if n, err = le(1 << (t&0x0f - 1)); err == nil {
			v, err = take(int(n))
		}
	case t >= 0xa0 && t <= 0xc0, t >= 0xc1 && t <= 0xc4:
		idx := uint64(t - 0xa0)
		if t >= 0xc1 {
			idx, err = le(int(t - 0xc0))
		}
		if err == nil && idx >= uint64(len(*refs)) {
			err = errors.New("opack: bad back-reference")
		}
		if err == nil {
			v = (*refs)[idx]
		}
		remember = false
	case t >= 0xd0 && t <= 0xdf:
		list := []any{}
		for n := int(t & 0x0f); n == 0x0f || len(list) < n; {
			if n == 0x0f && len(b) > 0 && b[0] == 0x03 {
				b = b[1:]
				break
			}
			var e any
			if e, b, err = opackNext(b, refs); err != nil {
				return nil, nil, err
			}
			list = append(list, e)
		}
		v, remember = list, false
	case t >= 0xe0:
		dict := map[string]any{}
		for n, i := int(t&0x0f), 0; n == 0x0f || i < n; i++ {
			if n == 0x0f && len(b) > 0 && b[0] == 0x03 {
				b = b[1:]
				break
			}
			var k, e any
			if k, b, err = opackNext(b, refs); err != nil {
				return nil, nil, err
			}
			if e, b, err = opackNext(b, refs); err != nil {
				return nil, nil, err
			}
			ks, _ := k.(string)
			dict[ks] = e
		}
		v, remember = dict, false
	default:
		return nil, nil, fmt.Errorf("opack: unknown type 0x%02x", t)
	}
	if err != nil {
		return nil, nil, err
	}
	// A value already in the list isn't added again, or later indexes shift.
	if remember && !hasRef(*refs, v) {
		*refs = append(*refs, v)
	}
	return v, b, nil
}

func hasRef(refs []any, v any) bool {
	for _, r := range refs {
		switch r := r.(type) {
		case []byte:
			if w, ok := v.([]byte); ok && string(r) == string(w) {
				return true
			}
		default:
			if _, isBytes := v.([]byte); !isBytes && r == v {
				return true
			}
		}
	}
	return false
}
