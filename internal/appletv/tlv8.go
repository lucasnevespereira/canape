package appletv

// TLV8 carries the pairing messages (HomeKit's format): a type byte, a
// length byte, then the value. Values over 255 bytes are split into
// consecutive items of the same type.

const (
	tlvMethod        = 0x00
	tlvIdentifier    = 0x01
	tlvSalt          = 0x02
	tlvPublicKey     = 0x03
	tlvProof         = 0x04
	tlvEncryptedData = 0x05
	tlvState         = 0x06
	tlvError         = 0x07
	tlvSignature     = 0x0a
)

type tlvItem struct {
	t byte
	v []byte
}

func tlvEncode(items ...tlvItem) []byte {
	var b []byte
	for _, it := range items {
		v := it.v
		for {
			n := min(len(v), 255)
			b = append(b, it.t, byte(n))
			b = append(b, v[:n]...)
			v = v[n:]
			if len(v) == 0 {
				break
			}
		}
	}
	return b
}

func tlvDecode(b []byte) map[byte][]byte {
	out := map[byte][]byte{}
	last := -1
	for len(b) >= 2 {
		t, n := b[0], int(b[1])
		if len(b) < 2+n {
			break
		}
		if int(t) == last {
			out[t] = append(out[t], b[2:2+n]...)
		} else {
			out[t] = append([]byte(nil), b[2:2+n]...)
		}
		last = int(t)
		b = b[2+n:]
	}
	return out
}
