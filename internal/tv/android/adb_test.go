package android

import (
	"crypto"
	"crypto/rsa"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeDevice plays adbd: it asks for a signature, wants the public key when
// it doesn't trust the signer yet, then runs one shell command.
func fakeDevice(t *testing.T, conn net.Conn, trusted *rsa.PublicKey) {
	defer conn.Close()
	if m, err := readMsg(conn); err != nil || m.cmd != cmdCNXN {
		t.Errorf("want CNXN first, got %x %v", m.cmd, err)
		return
	}
	token := []byte("01234567890123456789")
	writeMsg(conn, message{cmdAUTH, authToken, 0, token})
	m, _ := readMsg(conn)
	if m.cmd != cmdAUTH || m.arg0 != authSignature {
		t.Errorf("want a signature, got %x/%d", m.cmd, m.arg0)
		return
	}
	if trusted == nil || rsa.VerifyPKCS1v15(trusted, crypto.SHA1, token, m.data) != nil {
		writeMsg(conn, message{cmdAUTH, authToken, 0, token})
		m, _ = readMsg(conn)
		if m.cmd != cmdAUTH || m.arg0 != authPublicKey || !strings.HasSuffix(string(m.data), " canape\x00") {
			t.Errorf("want the public key, got %x/%d %q", m.cmd, m.arg0, m.data)
			return
		}
	}
	writeMsg(conn, message{cmdCNXN, adbVersion, 4096, []byte("device::\x00")})

	m, _ = readMsg(conn)
	if m.cmd != cmdOPEN || string(m.data) != "shell:echo hi\x00" {
		t.Errorf("want OPEN shell, got %x %q", m.cmd, m.data)
		return
	}
	local := m.arg0
	writeMsg(conn, message{cmdOKAY, 7, local, nil})
	writeMsg(conn, message{cmdWRTE, 7, local, []byte("h")})
	readMsg(conn) // OKAY
	writeMsg(conn, message{cmdWRTE, 7, local, []byte("i\n")})
	readMsg(conn)
	writeMsg(conn, message{cmdCLSE, 7, local, nil})
	readMsg(conn)
}

func TestPairThenShell(t *testing.T) {
	key, saved, err := newKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publicKey(&key.PublicKey); err != nil {
		t.Fatal(err)
	}
	for _, trusted := range []*rsa.PublicKey{nil, &key.PublicKey} {
		host, dev := net.Pipe()
		go fakeDevice(t, dev, trusted)
		k, err := parseKey(saved)
		if err != nil {
			t.Fatal(err)
		}
		asked := false
		a, err := handshake(host, k, func() { asked = true }, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if asked != (trusted == nil) {
			t.Errorf("trusted=%v: prompted=%v", trusted != nil, asked)
		}
		out, err := a.Shell("echo hi")
		if err != nil || out != "hi\n" {
			t.Errorf("Shell = %q, %v", out, err)
		}
		a.Close()
	}
}

func TestParseMusic(t *testing.T) {
	out := "- STREAM_MUSIC:\n   Muted: true\n   Muted Internally: false\n   Min: 0\n   Max: 100\n   streamVolume:22\n"
	level, top, muted, ok := parseMusic(out)
	if !ok || level != 22 || top != 100 || !muted {
		t.Errorf("got %d/%d muted=%v ok=%v", level, top, muted, ok)
	}
}
