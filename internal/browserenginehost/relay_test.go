package browserenginehost

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

type memoryHandshake struct {
	input  *bytes.Reader
	output bytes.Buffer
	limit  int
}

func (m *memoryHandshake) Read(p []byte) (int, error) { return m.input.Read(p) }
func (m *memoryHandshake) Write(p []byte) (int, error) {
	if m.limit >= 0 && len(p) > m.limit {
		return m.output.Write(p[:m.limit])
	}
	return m.output.Write(p)
}
func TestFrontendGreetingAndUpstreamAuthAreDifferentProtocols(t *testing.T) {
	for _, raw := range [][]byte{{5, 1, 0}, {5, 2, 2, 0}} {
		stream := &memoryHandshake{input: bytes.NewReader(raw), limit: -1}
		if readFrontendGreeting(stream) != nil || stream.output.Len() != 0 {
			t.Fatal("frontend must await upstream authentication before accepting")
		}
	}
	for _, raw := range [][]byte{{4, 1, 0}, {5, 0}, {5, 17}, {5, 1, 2}, {5, 2, 0}} {
		if readFrontendGreeting(&memoryHandshake{input: bytes.NewReader(raw), limit: -1}) == nil {
			t.Fatal("bad no-auth greeting accepted")
		}
	}
	credentials := browserproxy.RelayCredentials{Username: "ultra-browser", Password: strings.Repeat("a", 64)}
	stream := &memoryHandshake{input: bytes.NewReader([]byte{5, 2, 1, 0}), limit: -1}
	if authenticateBackend(stream, credentials) != nil {
		t.Fatal("RFC1929 failed")
	}
	want := append([]byte{5, 1, 2, 1, 13}, []byte("ultra-browser")...)
	want = append(want, 64)
	want = append(want, []byte(credentials.Password)...)
	if !bytes.Equal(stream.output.Bytes(), want) {
		t.Fatal("wrong method/auth frame")
	}
	for _, response := range [][]byte{{5, 0, 1, 0}, {5, 2, 1, 1}, {5, 2}, {5, 2, 2, 0}} {
		if authenticateBackend(&memoryHandshake{input: bytes.NewReader(response), limit: -1}, credentials) == nil {
			t.Fatal("backend rejection accepted")
		}
	}
	if authenticateBackend(&memoryHandshake{input: bytes.NewReader([]byte{5, 2, 1, 0}), limit: 1}, credentials) == nil {
		t.Fatal("short handshake write accepted")
	}
	for _, password := range []string{strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("a", 63) + "g"} {
		credentials.Password = password
		stream := &memoryHandshake{input: bytes.NewReader(nil), limit: -1}
		if authenticateBackend(stream, credentials) == nil || stream.output.Len() != 0 {
			t.Fatal("invalid credentials entered transport")
		}
	}
	if writeHandshake(io.Discard, []byte{5, 0}) != nil {
		t.Fatal("complete handshake refused")
	}
}
