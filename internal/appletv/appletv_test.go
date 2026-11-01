package appletv

import (
	"bytes"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func TestOpackRoundTrip(t *testing.T) {
	in := map[string]any{
		"_i": "_hidC", "_t": int64(2), "_x": int64(40000), "big": int64(1 << 40),
		"_c":   map[string]any{"_hBtS": int64(1), "_hidC": int64(6), "on": true, "off": false},
		"long": strings.Repeat("a", 300), "bytes": []byte{1, 2, 3}, "list": []any{int64(1), "two", nil},
		"f": 1.5,
	}
	b, err := opackEncode(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := opackDecode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %v", out)
	}
}

// The Apple TV sends a value it already sent as a reference to its index.
func TestOpackBackReference(t *testing.T) {
	// {"a": "xyz", "b": <ref 1>}: "a" is ref 0, "xyz" ref 1.
	b := []byte{0xe2, 0x41, 'a', 0x43, 'x', 'y', 'z', 0x41, 'b', 0xa1}
	v, err := opackDecode(b)
	if err != nil {
		t.Fatal(err)
	}
	if m := v.(map[string]any); m["b"] != "xyz" {
		t.Fatalf("got %v", m)
	}
}

func TestTLVSplitsLongValues(t *testing.T) {
	long := bytes.Repeat([]byte{7}, 600)
	b := tlvEncode(tlvItem{tlvState, []byte{3}}, tlvItem{tlvPublicKey, long})
	got := tlvDecode(b)
	if !bytes.Equal(got[tlvPublicKey], long) || !bytes.Equal(got[tlvState], []byte{3}) {
		t.Fatal("TLV didn't survive the round trip")
	}
}

// The Apple TV side of SRP, from RFC 5054: with the right code both sides
// get the same key, so our proof matches and theirs comes back as expected.
func TestSRPAgreesWithTheAppleTV(t *testing.T) {
	salt := []byte("0123456789abcdef")
	x := new(big.Int).SetBytes(sha(salt, sha([]byte(srpUser+":1234"))))
	v := new(big.Int).Exp(srpG, x, srpN)
	b := big.NewInt(123456789)
	k := new(big.Int).SetBytes(sha(srpN.Bytes(), pad(srpG)))
	B := new(big.Int).Add(new(big.Int).Mul(k, v), new(big.Int).Exp(srpG, b, srpN))
	B.Mod(B, srpN)

	c := newSRPClient()
	m1, m2, key, ok := c.proof("1234", salt, B.Bytes())
	if !ok {
		t.Fatal("rejected B")
	}
	u := new(big.Int).SetBytes(sha(pad(c.A), pad(B)))
	S := new(big.Int).Exp(new(big.Int).Mul(c.A, new(big.Int).Exp(v, u, srpN)), b, srpN)
	if !bytes.Equal(sha(S.Bytes()), key) {
		t.Fatal("keys differ")
	}
	if want := sha(c.A.Bytes(), m1, key); !bytes.Equal(want, m2) {
		t.Fatal("M2 differs")
	}
	if _, _, wrong, _ := c.proof("0000", salt, B.Bytes()); bytes.Equal(wrong, key) {
		t.Fatal("a wrong code gave the same key")
	}
}
