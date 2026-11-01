package appletv

import (
	"crypto/rand"
	"crypto/sha512"
	"math/big"
	"strings"
)

// SRP-6a as HomeKit pairing uses it: the 3072-bit group from RFC 5054,
// SHA-512, user "Pair-Setup" and the code on screen as the password.

var srpN, _ = new(big.Int).SetString(strings.ReplaceAll(`
FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74
020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F1437
4FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED
EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF05
98DA48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB
9ED529077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3B
E39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF695581718
3995497CEA956AE515D2261898FA051015728E5A8AAAC42DAD33170D04507A33
A85521ABDF1CBA64ECFB850458DBEF0A8AEA71575D060C7DB3970F85A6E1E4C7
ABF5AE8CDB0933D71E8C94E04A25619DCEE3D2261AD2EE6BF12FFA06D98A0864
D87602733EC86A64521F2B18177B200CBBE117577A615D6C770988C0BAD946E2
08E24FA074E5AB3143DB5BFCE0FD108E4B82D120A93AD2CAFFFFFFFFFFFFFFFF`, "\n", ""), 16)

var srpG = big.NewInt(5)

const srpUser = "Pair-Setup"

func sha(parts ...[]byte) []byte {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

// pad left-pads to the size of N, as k and u require.
func pad(x *big.Int) []byte { return x.FillBytes(make([]byte, len(srpN.Bytes()))) }

type srpClient struct {
	a, A *big.Int
}

func newSRPClient() *srpClient {
	b := make([]byte, 32)
	rand.Read(b)
	a := new(big.Int).SetBytes(b)
	return &srpClient{a: a, A: new(big.Int).Exp(srpG, a, srpN)}
}

// proof answers the Apple TV's salt and public key: it returns our proof
// M1, the proof M2 to expect back, and the session key K.
func (c *srpClient) proof(code string, salt, bBytes []byte) (m1, m2, key []byte, ok bool) {
	B := new(big.Int).SetBytes(bBytes)
	if new(big.Int).Mod(B, srpN).Sign() == 0 {
		return nil, nil, nil, false
	}
	k := new(big.Int).SetBytes(sha(srpN.Bytes(), pad(srpG)))
	u := new(big.Int).SetBytes(sha(pad(c.A), pad(B)))
	x := new(big.Int).SetBytes(sha(salt, sha([]byte(srpUser+":"+code))))

	// S = (B - k*g^x) ^ (a + u*x) mod N
	base := new(big.Int).Sub(B, new(big.Int).Mul(k, new(big.Int).Exp(srpG, x, srpN)))
	base.Mod(base, srpN)
	S := new(big.Int).Exp(base, new(big.Int).Add(c.a, new(big.Int).Mul(u, x)), srpN)
	key = sha(S.Bytes())

	hn, hg := sha(srpN.Bytes()), sha(srpG.Bytes())
	for i := range hn {
		hn[i] ^= hg[i]
	}
	m1 = sha(hn, sha([]byte(srpUser)), salt, c.A.Bytes(), B.Bytes(), key)
	m2 = sha(c.A.Bytes(), m1, key)
	return m1, m2, key, true
}
