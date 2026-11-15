package android

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
)

// ADB wire commands, read as little-endian ASCII.
const (
	cmdCNXN = 0x4e584e43
	cmdAUTH = 0x48545541
	cmdOPEN = 0x4e45504f
	cmdOKAY = 0x59414b4f
	cmdCLSE = 0x45534c43
	cmdWRTE = 0x45545257
)

// AUTH message types.
const (
	authToken     = 1
	authSignature = 2
	authPublicKey = 3
)

// The 2016 protocol version. It still checksums payloads, which every
// device accepts, unlike the newer one that old firmware rejects.
const adbVersion = 0x01000000

type message struct {
	cmd, arg0, arg1 uint32
	data            []byte
}

// adb is one authenticated connection to adbd. Commands run one at a time,
// each in its own shell stream.
type adb struct {
	conn  net.Conn
	local uint32 // last stream id we opened
}

func writeMsg(w io.Writer, m message) error {
	var sum uint32
	for _, b := range m.data {
		sum += uint32(b)
	}
	h := make([]byte, 24, 24+len(m.data))
	binary.LittleEndian.PutUint32(h[0:], m.cmd)
	binary.LittleEndian.PutUint32(h[4:], m.arg0)
	binary.LittleEndian.PutUint32(h[8:], m.arg1)
	binary.LittleEndian.PutUint32(h[12:], uint32(len(m.data)))
	binary.LittleEndian.PutUint32(h[16:], sum)
	binary.LittleEndian.PutUint32(h[20:], m.cmd^0xffffffff)
	_, err := w.Write(append(h, m.data...))
	return err
}

func readMsg(r io.Reader) (message, error) {
	h := make([]byte, 24)
	if _, err := io.ReadFull(r, h); err != nil {
		return message{}, err
	}
	m := message{
		cmd:  binary.LittleEndian.Uint32(h[0:]),
		arg0: binary.LittleEndian.Uint32(h[4:]),
		arg1: binary.LittleEndian.Uint32(h[8:]),
	}
	if binary.LittleEndian.Uint32(h[20:]) != m.cmd^0xffffffff {
		return m, errors.New("not an ADB device")
	}
	n := binary.LittleEndian.Uint32(h[12:])
	if n > 1<<20 {
		return m, fmt.Errorf("ADB message too big (%d bytes)", n)
	}
	m.data = make([]byte, n)
	_, err := io.ReadFull(r, m.data)
	return m, err
}

// handshake connects as a host and authenticates with key. A device that
// doesn't know the key asks the user "Allow debugging?": prompted is called
// and the user gets until the deadline to accept.
func handshake(conn net.Conn, key *rsa.PrivateKey, prompted func(), wait time.Duration) (*adb, error) {
	if err := writeMsg(conn, message{cmdCNXN, adbVersion, 256 << 10, []byte("host::\x00")}); err != nil {
		return nil, err
	}
	signed, asked := false, false
	for {
		m, err := readMsg(conn)
		if err != nil {
			if asked {
				return nil, fmt.Errorf("canape wasn't allowed on the TV: %w", err)
			}
			return nil, err
		}
		switch {
		case m.cmd == cmdCNXN:
			return &adb{conn: conn}, nil
		case m.cmd == cmdAUTH && m.arg0 == authToken && !signed:
			// adbd hands over a 20-byte token and checks it as a SHA-1 digest.
			sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA1, m.data)
			if err != nil {
				return nil, err
			}
			signed = true
			err = writeMsg(conn, message{cmdAUTH, authSignature, 0, sig})
			if err != nil {
				return nil, err
			}
		case m.cmd == cmdAUTH && m.arg0 == authToken:
			// A second token: the device doesn't know this key yet.
			pub, err := publicKey(&key.PublicKey)
			if err != nil {
				return nil, err
			}
			if err := writeMsg(conn, message{cmdAUTH, authPublicKey, 0, append([]byte(pub), 0)}); err != nil {
				return nil, err
			}
			asked = true
			if prompted != nil {
				prompted()
			}
			conn.SetDeadline(time.Now().Add(wait))
		}
	}
}

// Shell runs a command and returns what it printed.
func (a *adb) Shell(command string) (string, error) {
	a.conn.SetDeadline(time.Now().Add(10 * time.Second))
	a.local++
	local := a.local
	if err := writeMsg(a.conn, message{cmdOPEN, local, 0, []byte("shell:" + command + "\x00")}); err != nil {
		return "", err
	}
	var out []byte
	for {
		m, err := a.readFor(local)
		if err != nil {
			return string(out), err
		}
		switch m.cmd {
		case cmdWRTE:
			out = append(out, m.data...)
			if err := writeMsg(a.conn, message{cmdOKAY, local, m.arg0, nil}); err != nil {
				return string(out), err
			}
		case cmdCLSE:
			writeMsg(a.conn, message{cmdCLSE, local, m.arg0, nil})
			return string(out), nil
		}
	}
}

// readFor skips messages left over from earlier streams.
func (a *adb) readFor(local uint32) (message, error) {
	for {
		m, err := readMsg(a.conn)
		if err != nil || m.arg1 == local {
			return m, err
		}
	}
}

func (a *adb) Close() error { return a.conn.Close() }

// publicKey encodes key the way adbd reads it: Android's mincrypt
// RSAPublicKey struct (little-endian words), base64, then a name.
func publicKey(key *rsa.PublicKey) (string, error) {
	const words = 64 // 2048 bits
	if key.N.BitLen() != words*32 {
		return "", errors.New("ADB keys must be 2048-bit RSA")
	}
	b32 := new(big.Int).Lsh(big.NewInt(1), 32)
	// n0inv = -1 / n mod 2^32
	n0inv := new(big.Int).Mod(key.N, b32)
	n0inv.ModInverse(n0inv, b32)
	n0inv.Sub(b32, n0inv)
	// rr = 2^(2*2048) mod n
	rr := new(big.Int).Lsh(big.NewInt(1), 2*words*32)
	rr.Mod(rr, key.N)

	buf := make([]byte, 0, 4*(3+2*words))
	buf = binary.LittleEndian.AppendUint32(buf, words)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(n0inv.Uint64()))
	buf = appendWords(buf, key.N, words)
	buf = appendWords(buf, rr, words)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(key.E))
	return base64.StdEncoding.EncodeToString(buf) + " canape", nil
}

func appendWords(buf []byte, n *big.Int, words int) []byte {
	be := n.FillBytes(make([]byte, words*4))
	for i := len(be) - 4; i >= 0; i -= 4 {
		buf = append(buf, be[i+3], be[i+2], be[i+1], be[i])
	}
	return buf
}

// newKey makes the key canape signs in with, saved as the pairing key.
func newKey() (*rsa.PrivateKey, string, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}
	return k, base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(k)), nil
}

func parseKey(saved string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(saved)
	if err != nil {
		return nil, err
	}
	return x509.ParsePKCS1PrivateKey(der)
}
