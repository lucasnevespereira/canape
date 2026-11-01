// Package appletv controls an Apple TV over the network with the Companion
// protocol, the one the iPhone's remote uses. Pairing once with the code the
// Apple TV shows gives keys that every later connection proves itself with.
// The protocol is undocumented: this follows what pyatv worked out.
package appletv

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Credentials are what pairing leaves behind, saved in the home.
type Credentials struct {
	ID     string `json:"id"`      // who canape is to the Apple TV
	Seed   string `json:"seed"`    // canape's Ed25519 key, hex
	ATVID  string `json:"atv_id"`  // the Apple TV's pairing identifier
	ATVKey string `json:"atv_key"` // its Ed25519 public key, hex
}

func hkdf512(secret []byte, salt, info string) []byte {
	k, _ := hkdf.Key(sha512.New, secret, []byte(salt), info, 32)
	return k
}

// Pairing is a pair setup in progress: the Apple TV shows a code between
// StartPairing and Finish.
type Pairing struct {
	c   *conn
	srp *srpClient
	m2  map[byte][]byte
}

// StartPairing makes the Apple TV at addr (ip:port) show a 4-digit code.
func StartPairing(addr string) (*Pairing, error) {
	c, err := dial(addr)
	if err != nil {
		return nil, err
	}
	m2, err := c.exchange(framePSStart, map[string]any{
		"_pd":   tlvEncode(tlvItem{tlvMethod, []byte{0}}, tlvItem{tlvState, []byte{1}}),
		"_pwTy": 1,
	}, 10*time.Second)
	if err != nil {
		c.Close()
		return nil, err
	}
	return &Pairing{c: c, srp: newSRPClient(), m2: m2}, nil
}

func (p *Pairing) Close() { p.c.Close() }

// Finish proves the code and exchanges long-term keys.
func (p *Pairing) Finish(code string) (*Credentials, error) {
	defer p.c.Close()
	m1, m2, key, ok := p.srp.proof(strings.TrimSpace(code), p.m2[tlvSalt], p.m2[tlvPublicKey])
	if !ok {
		return nil, errors.New("the Apple TV sent a bad key")
	}
	m4, err := p.c.exchange(framePSNext, map[string]any{"_pd": tlvEncode(
		tlvItem{tlvState, []byte{3}}, tlvItem{tlvPublicKey, p.srp.A.Bytes()}, tlvItem{tlvProof, m1},
	)}, 10*time.Second)
	if err != nil {
		return nil, err
	}
	if string(m4[tlvProof]) != string(m2) {
		return nil, errors.New("wrong code")
	}

	seed := make([]byte, ed25519.SeedSize)
	rand.Read(seed)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	id := strings.ToUpper(hex.EncodeToString(seed[:8])) // any stable name for canape

	x := hkdf512(key, "Pair-Setup-Controller-Sign-Salt", "Pair-Setup-Controller-Sign-Info")
	sig := ed25519.Sign(priv, append(append(x, id...), pub...))
	inner := tlvEncode(tlvItem{tlvIdentifier, []byte(id)}, tlvItem{tlvPublicKey, pub}, tlvItem{tlvSignature, sig})
	sessionKey := hkdf512(key, "Pair-Setup-Encrypt-Salt", "Pair-Setup-Encrypt-Info")
	aead := newAEAD(sessionKey)
	m6, err := p.c.exchange(framePSNext, map[string]any{"_pd": tlvEncode(
		tlvItem{tlvState, []byte{5}}, tlvItem{tlvEncryptedData, aead.Seal(nil, labelNonce("PS-Msg05"), inner, nil)},
	)}, 10*time.Second)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, labelNonce("PS-Msg06"), m6[tlvEncryptedData], nil)
	if err != nil {
		return nil, errors.New("the Apple TV's answer didn't decrypt")
	}
	atv := tlvDecode(plain)
	return &Credentials{
		ID: id, Seed: hex.EncodeToString(seed),
		ATVID: string(atv[tlvIdentifier]), ATVKey: hex.EncodeToString(atv[tlvPublicKey]),
	}, nil
}

// Client is a verified, encrypted connection to a paired Apple TV.
type Client struct{ c *conn }

// Connect proves canape's pairing to the Apple TV and opens a session.
func Connect(addr string, cr *Credentials) (*Client, error) {
	seed, err := hex.DecodeString(cr.Seed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the saved Apple TV pairing is damaged. Pair again: canape appletv pair")
	}
	c, err := dial(addr)
	if err != nil {
		return nil, err
	}
	cl := &Client{c: c}
	if err := cl.verify(seed, cr); err != nil {
		c.Close()
		return nil, err
	}
	sid := make([]byte, 4)
	rand.Read(sid)
	if _, err := c.request("_systemInfo", map[string]any{
		"_bf": 0, "_cf": 512, "_clFl": 128, "_i": "cafecafecafe", "_idsID": cr.ID,
		"_pubID": "aa:bb:cc:dd:ee:ff", "_sf": 256, "_sv": "170.18", "model": "iPhone14,3", "name": "canape",
	}); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := c.request("_touchStart", map[string]any{"_height": 1000.0, "_tFl": 0, "_width": 1000.0}); err != nil {
		c.Close()
		return nil, err
	}
	if _, err := c.request("_sessionStart", map[string]any{
		"_srvT": "com.apple.tvremoteservices", "_sid": int64(sid[0])<<24 | int64(sid[1])<<16 | int64(sid[2])<<8 | int64(sid[3]),
	}); err != nil {
		c.Close()
		return nil, err
	}
	return cl, nil
}

func (cl *Client) verify(seed []byte, cr *Credentials) error {
	eph, _ := ecdh.X25519().GenerateKey(rand.Reader)
	m2, err := cl.c.exchange(framePVStart, map[string]any{
		"_pd":   tlvEncode(tlvItem{tlvState, []byte{1}}, tlvItem{tlvPublicKey, eph.PublicKey().Bytes()}),
		"_auTy": 4,
	}, 5*time.Second)
	if err != nil {
		return err
	}
	atvPub, err := ecdh.X25519().NewPublicKey(m2[tlvPublicKey])
	if err != nil {
		return errors.New("the Apple TV sent a bad key")
	}
	shared, err := eph.ECDH(atvPub)
	if err != nil {
		return err
	}
	aead := newAEAD(hkdf512(shared, "Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info"))
	plain, err := aead.Open(nil, labelNonce("PV-Msg02"), m2[tlvEncryptedData], nil)
	if err != nil {
		return errors.New("the Apple TV's answer didn't decrypt")
	}
	inner := tlvDecode(plain)
	if key, err := hex.DecodeString(cr.ATVKey); err == nil && len(key) == ed25519.PublicKeySize {
		info := append(append(atvPub.Bytes(), inner[tlvIdentifier]...), eph.PublicKey().Bytes()...)
		if !ed25519.Verify(key, info, inner[tlvSignature]) {
			return errors.New("this isn't the Apple TV canape paired with. Pair again: canape appletv pair")
		}
	}

	priv := ed25519.NewKeyFromSeed(seed)
	info := append(append(eph.PublicKey().Bytes(), cr.ID...), atvPub.Bytes()...)
	enc := tlvEncode(tlvItem{tlvIdentifier, []byte(cr.ID)}, tlvItem{tlvSignature, ed25519.Sign(priv, info)})
	if _, err := cl.c.exchange(framePVNext, map[string]any{"_pd": tlvEncode(
		tlvItem{tlvState, []byte{3}}, tlvItem{tlvEncryptedData, aead.Seal(nil, labelNonce("PV-Msg03"), enc, nil)},
	)}, 5*time.Second); err != nil {
		if strings.Contains(err.Error(), "error [2]") || err.Error() == "wrong code" {
			return errors.New("the Apple TV forgot canape. Pair again: canape appletv pair")
		}
		return err
	}
	cl.c.out = newAEAD(hkdf512(shared, "", "ClientEncrypt-main"))
	cl.c.in = newAEAD(hkdf512(shared, "", "ServerEncrypt-main"))
	return nil
}

func (cl *Client) Close() error { return cl.c.Close() }

// Buttons on the Siri Remote, by the names canape uses.
var buttons = map[string]int{
	"up": 1, "down": 2, "left": 3, "right": 4, "menu": 5, "select": 6, "home": 7,
	"volume-up": 8, "volume-down": 9, "sleep": 12, "wake": 13, "playpause": 14,
}

// ButtonNames lists the buttons Press accepts.
func ButtonNames() []string {
	var names []string
	for n := range buttons {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Press presses and releases a remote button.
func (cl *Client) Press(name string) error {
	code, ok := buttons[name]
	if !ok {
		return fmt.Errorf("unknown button %q", name)
	}
	for _, state := range []int{1, 2} { // down, then up
		if _, err := cl.c.request("_hidC", map[string]any{"_hBtS": state, "_hidC": code}); err != nil {
			return err
		}
	}
	return nil
}

// Skip jumps forward, or back with negative seconds, in what's playing.
func (cl *Client) Skip(seconds float64) error {
	_, err := cl.c.request("_mcc", map[string]any{"_mcc": 7, "_skpS": seconds})
	return err
}
