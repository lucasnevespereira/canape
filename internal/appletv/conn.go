package appletv

import (
	"bufio"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// Companion frames: a type byte and a 3-byte big-endian length.
const (
	framePSStart = 3 // pair setup
	framePSNext  = 4
	framePVStart = 5 // pair verify
	framePVNext  = 6
	frameEOPACK  = 8 // an encrypted OPACK message, once verified
)

// Message types in the "_t" field.
const (
	msgEvent    = 1
	msgRequest  = 2
	msgResponse = 3
)

type conn struct {
	c         net.Conn
	r         *bufio.Reader
	out, in   cipher.AEAD // set once pair verify succeeds
	outN, inN uint64
	xid       int64
}

func dial(addr string) (*conn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 2)
	rand.Read(b)
	return &conn{c: c, r: bufio.NewReader(c), xid: int64(binary.BigEndian.Uint16(b))}, nil
}

func (c *conn) Close() error { return c.c.Close() }

// counterNonce is the per-direction message counter, little-endian, as the
// 12-byte nonce.
func counterNonce(n uint64) []byte {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint64(b, n)
	return b
}

// labelNonce pads a pairing label like "PS-Msg05" to 12 bytes on the left.
func labelNonce(label string) []byte {
	return append(make([]byte, 12-len(label)), label...)
}

func (c *conn) writeFrame(typ byte, payload []byte) error {
	n := len(payload)
	if typ == frameEOPACK && c.out != nil {
		n += c.out.Overhead()
	}
	hdr := []byte{typ, byte(n >> 16), byte(n >> 8), byte(n)}
	if typ == frameEOPACK && c.out != nil {
		payload = c.out.Seal(nil, counterNonce(c.outN), payload, hdr)
		c.outN++
	}
	c.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.c.Write(append(hdr, payload...))
	return err
}

func (c *conn) readFrame(timeout time.Duration) (byte, []byte, error) {
	c.c.SetReadDeadline(time.Now().Add(timeout))
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(c.r, hdr); err != nil {
		return 0, nil, err
	}
	n := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
	payload := make([]byte, n)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return 0, nil, err
	}
	if hdr[0] == frameEOPACK && c.in != nil {
		var err error
		if payload, err = c.in.Open(nil, counterNonce(c.inN), payload, hdr); err != nil {
			return 0, nil, fmt.Errorf("decrypt: %w", err)
		}
		c.inN++
	}
	return hdr[0], payload, nil
}

// exchange sends one pairing message and returns the TLV the Apple TV answers.
func (c *conn) exchange(typ byte, body map[string]any, timeout time.Duration) (map[byte][]byte, error) {
	b, err := opackEncode(body)
	if err != nil {
		return nil, err
	}
	if err := c.writeFrame(typ, b); err != nil {
		return nil, err
	}
	_, payload, err := c.readFrame(timeout)
	if err != nil {
		return nil, err
	}
	v, err := opackDecode(payload)
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	pd, _ := m["_pd"].([]byte)
	tlv := tlvDecode(pd)
	if e, ok := tlv[tlvError]; ok {
		return nil, pairingError(e)
	}
	return tlv, nil
}

func pairingError(e []byte) error {
	if len(e) == 1 {
		switch e[0] {
		case 2:
			return errors.New("wrong code")
		case 3:
			return errors.New("too many tries, wait a moment")
		case 5:
			return errors.New("the Apple TV is pairing with something else")
		case 6:
			return errors.New("the Apple TV is busy, try again")
		}
	}
	return fmt.Errorf("the Apple TV refused (error %v)", e)
}

// request sends a command and waits for its answer, skipping the events the
// Apple TV sends in between.
func (c *conn) request(id string, content map[string]any) (map[string]any, error) {
	c.xid++
	x := c.xid
	b, err := opackEncode(map[string]any{"_i": id, "_t": msgRequest, "_c": content, "_x": x})
	if err != nil {
		return nil, err
	}
	if err := c.writeFrame(frameEOPACK, b); err != nil {
		return nil, err
	}
	for {
		typ, payload, err := c.readFrame(5 * time.Second)
		if err != nil {
			return nil, err
		}
		if typ != frameEOPACK {
			continue
		}
		v, err := opackDecode(payload)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		if t, _ := m["_t"].(int64); t != msgResponse {
			continue
		}
		if got, _ := m["_x"].(int64); got != x {
			continue
		}
		if em, ok := m["_em"].(string); ok {
			return nil, fmt.Errorf("the Apple TV said: %s", em)
		}
		resp, _ := m["_c"].(map[string]any)
		return resp, nil
	}
}

func newAEAD(key []byte) cipher.AEAD {
	a, _ := chacha20poly1305.New(key)
	return a
}
