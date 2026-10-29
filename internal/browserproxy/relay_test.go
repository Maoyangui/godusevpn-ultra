package browserproxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryDialer struct {
	conn  net.Conn
	calls int
}

func (d *memoryDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
func (d *memoryDialer) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	d.calls++
	if network != "tcp" || address != "fixture.proxy:1080" {
		return nil, errors.New("unexpected dial")
	}
	return d.conn, nil
}

func TestRawProxyConnectionPreservesBufferedBytes(t *testing.T) {
	for _, kind := range []string{"http", "socks5"} {
		t.Run(kind, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(3 * time.Second))
			finished := make(chan error, 1)
			go func() {
				reader := bufio.NewReader(server)
				if kind == "http" {
					request, err := http.ReadRequest(reader)
					if err != nil {
						finished <- err
						return
					}
					if request.Method != "CONNECT" || request.Host != "example.invalid:443" || request.Header.Get("Proxy-Authorization") == "" {
						finished <- errors.New("wrong raw CONNECT")
						return
					}
					_, err = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\nfixture-upstream")
					finished <- err
				} else {
					header := make([]byte, 2)
					_, _ = io.ReadFull(reader, header)
					methods := make([]byte, int(header[1]))
					_, _ = io.ReadFull(reader, methods)
					_, _ = server.Write([]byte{5, 2})
					_, _ = io.ReadFull(reader, header)
					user := make([]byte, int(header[1]))
					_, _ = io.ReadFull(reader, user)
					length, _ := reader.ReadByte()
					password := make([]byte, int(length))
					_, _ = io.ReadFull(reader, password)
					if header[0] != 1 || string(user) != "fixture" || string(password) != "fixture-secret" {
						finished <- errors.New("wrong raw SOCKS auth")
						return
					}
					_, _ = server.Write([]byte{1, 0})
					request := make([]byte, 5)
					_, _ = io.ReadFull(reader, request)
					if request[3] != 3 {
						finished <- errors.New("SOCKS destination DNS resolved locally")
						return
					}
					host := make([]byte, int(request[4]))
					_, _ = io.ReadFull(reader, host)
					port := make([]byte, 2)
					_, _ = io.ReadFull(reader, port)
					if string(host) != "example.invalid" || binary.BigEndian.Uint16(port) != 443 {
						finished <- errors.New("wrong raw SOCKS target")
						return
					}
					_, err := server.Write(append([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}, []byte("fixture-upstream")...))
					finished <- err
				}
			}()
			dialer := &memoryDialer{conn: client}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := dialThrough(ctx, Config{Type: kind, Host: "fixture.proxy", Port: 1080, Username: "fixture", Password: "fixture-secret"}, "example.invalid:443", dialer)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			data := make([]byte, len("fixture-upstream"))
			if _, err = io.ReadFull(conn, data); err != nil || string(data) != "fixture-upstream" || dialer.calls != 1 {
				t.Fatal("tunneled bytes lost or unexpected dial")
			}
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		})
	}
}

type separateReadWriter struct {
	io.Reader
	io.Writer
}

func TestBrowserFacingSOCKSContract(t *testing.T) {
	domain := "example.invalid"
	credentials := RelayCredentials{Username: "ultra-browser", Password: strings.Repeat("a", 64)}
	prefix := []byte{5, 1, 2, 1, byte(len(credentials.Username))}
	prefix = append(prefix, credentials.Username...)
	prefix = append(prefix, 64)
	prefix = append(prefix, credentials.Password...)
	valid := append(append([]byte{}, prefix...), []byte{5, 1, 0, 3, byte(len(domain))}...)
	valid = append(valid, []byte(domain)...)
	valid = append(valid, 1, 187)
	for _, test := range []struct {
		name    string
		request []byte
		valid   bool
	}{
		{"domain remote DNS", valid, true},
		{"UDP refused", append(append([]byte{}, prefix...), 5, 3, 0, 1, 127, 0, 0, 1, 0, 53), false},
		{"no auth refused", []byte{5, 1, 0}, false},
		{"truncated auth refused", []byte{5, 1, 2}, false},
		{"wrong nonce", bytes.Replace(valid, []byte(credentials.Password), []byte(strings.Repeat("b", 64)), 1), false},
		{"wrong username", bytes.Replace(valid, []byte(credentials.Username), []byte("other-browser"), 1), false},
		{"malformed target", append(append([]byte{}, prefix...), 5, 1, 0, 3, 3, 'a', '\r', 'b', 0, 80), false},
		{"unspecified refused", append(append([]byte{}, prefix...), 5, 1, 0, 1, 0, 0, 0, 0, 0, 80), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reply bytes.Buffer
			target, err := readAuthenticatedSOCKSConnect(separateReadWriter{bytes.NewReader(test.request), &reply}, credentials)
			if (err == nil) != test.valid {
				t.Fatal("unexpected front-proxy result")
			}
			if test.valid && target != "example.invalid:443" {
				t.Fatal("target changed")
			}
		})
	}
}

// All lifecycle fixtures use memory channels and net.Pipe; no listening socket,
// DNS query, system service or product process is opened.
type fixtureListener struct {
	incoming chan net.Conn
	closed   chan struct{}
	once     sync.Once
}

func (l *fixtureListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.incoming:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *fixtureListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func (l *fixtureListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 20888}
}
func TestAuthenticatedRelayLifetime(t *testing.T) {
	for _, reason := range []string{"owner close", "cancel", "expiry"} {
		t.Run(reason, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			if reason == "expiry" {
				parent, cancel = context.WithTimeout(context.Background(), 80*time.Millisecond)
				defer cancel()
			}
			listener := &fixtureListener{incoming: make(chan net.Conn), closed: make(chan struct{})}
			dialed := make(chan string, 1)
			upClient, upServer := net.Pipe()
			defer upServer.Close()
			relay, err := startRelay(parent, Config{Type: "http", Host: "fixture.proxy", Port: 1080}, bytes.NewReader(bytes.Repeat([]byte{1}, 32)), func(network, address string) (net.Listener, error) {
				if network != "tcp4" || address != "127.0.0.1:0" {
					t.Error("nonloopback listener")
				}
				return listener, nil
			}, func(_ context.Context, _ Config, target string) (net.Conn, error) {
				dialed <- target
				return upClient, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer relay.Close()
			credentials := relay.Credentials()
			if credentials.Username != "ultra-browser" || len(credentials.Password) != 64 {
				t.Fatal("bad backend credentials")
			}
			encoded, _ := json.Marshal(credentials)
			if string(encoded) != "{}" || strings.Contains(fmt.Sprintf("%+v %#v", credentials, credentials), credentials.Password) {
				t.Fatal("credentials escaped redaction")
			}
			client, server := net.Pipe()
			defer client.Close()
			client.SetDeadline(time.Now().Add(2 * time.Second))
			listener.incoming <- server
			exchange := func(request []byte, size int) {
				t.Helper()
				if _, e := client.Write(request); e != nil {
					t.Fatal(e)
				}
				reply := make([]byte, size)
				if _, e := io.ReadFull(client, reply); e != nil {
					t.Fatal(e)
				}
				if reply[1] != 0 && !(reply[0] == 5 && reply[1] == 2) {
					t.Fatal("authentication refused")
				}
			}
			exchange([]byte{5, 1, 2}, 2)
			auth := append([]byte{1, 13}, credentials.Username...)
			auth = append(auth, 64)
			auth = append(auth, credentials.Password...)
			exchange(auth, 2)
			target := "example.invalid"
			request := append([]byte{5, 1, 0, 3, byte(len(target))}, target...)
			request = append(request, 1, 187)
			exchange(request, 10)
			if <-dialed != "example.invalid:443" {
				t.Fatal("destination DNS was not passed to proxy")
			}
			if reason == "owner close" {
				relay.Close()
			} else if reason == "cancel" {
				cancel()
			} else {
				<-parent.Done()
			}
			if _, e := client.Read(make([]byte, 1)); e == nil {
				t.Fatal("revoked client remained open")
			}
			upServer.SetReadDeadline(time.Now().Add(time.Second))
			if _, e := upServer.Read(make([]byte, 1)); e == nil {
				t.Fatal("revoked upstream remained open")
			}
			relay.Close()
			if relay.Credentials() != (RelayCredentials{}) {
				t.Fatal("closed relay retained nonce")
			}
		})
	}
}
func TestRelayRejectsExpiredBeforeListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, parent := range []context.Context{nil, ctx} {
		calls := 0
		_, err := startRelay(parent, Config{Type: "http", Host: "fixture.proxy", Port: 1080}, bytes.NewReader(nil), func(string, string) (net.Listener, error) { calls++; return nil, ErrProbe }, nil)
		if err != ErrProbe || calls != 0 {
			t.Fatal("expired owner opened listener")
		}
	}
}

type refusingDialer struct {
	calls    int
	deadline bool
}

func (d *refusingDialer) Dial(n, a string) (net.Conn, error) {
	return d.DialContext(context.Background(), n, a)
}
func (d *refusingDialer) DialContext(ctx context.Context, n, a string) (net.Conn, error) {
	d.calls++
	end, ok := ctx.Deadline()
	d.deadline = ok && time.Until(end) > 0 && time.Until(end) <= 8*time.Second
	if n != "tcp" || a != "fixture.proxy:1080" {
		return nil, errors.New("destination fallback attempted")
	}
	return nil, ErrProbe
}
func TestDialBoundedAndNoFallback(t *testing.T) {
	for _, kind := range []string{"http", "https", "socks5"} {
		t.Run(kind, func(t *testing.T) {
			d := &refusingDialer{}
			_, e := dialThrough(context.Background(), Config{Type: kind, Host: "fixture.proxy", Port: 1080}, "example.invalid:443", d)
			if e != ErrProbe || d.calls != 1 || !d.deadline {
				t.Fatal("handshake missing deadline or changed route")
			}
		})
	}
}
func TestStalledProxyHandshakeCancelled(t *testing.T) {
	for _, kind := range []string{"http", "https", "socks5"} {
		t.Run(kind, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := dialThrough(ctx, Config{Type: kind, Host: "fixture.proxy", Port: 1080}, "example.invalid:443", &memoryDialer{conn: client})
			if err != ErrProbe || time.Since(started) > time.Second {
				t.Fatal("unbounded stalled proxy handshake")
			}
		})
	}
}

func TestHTTPSProxyCredentialsRequireTLS(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	observed := make(chan error, 1)
	go func() {
		header := make([]byte, 5)
		_, err := io.ReadFull(server, header)
		if err == nil && header[0] != 22 {
			err = errors.New("credentials sent before TLS")
		}
		server.Close() // a TLS failure must terminate, never retry clear HTTP/direct.
		observed <- err
	}()
	dialer := &memoryDialer{conn: client}
	_, err := dialThrough(context.Background(), Config{Type: "https", Host: "fixture.proxy", Port: 1080, Username: "fixture", Password: "private-fixture"}, "example.invalid:443", dialer)
	if err != ErrProbe || dialer.calls != 1 {
		t.Fatal("HTTPS failure was not terminal/redacted")
	}
	if err := <-observed; err != nil {
		t.Fatal(err)
	}
}

func TestCONNECTResponseHeaderBound(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := http.ReadRequest(bufio.NewReader(server))
		if err == nil {
			_, _ = io.WriteString(server, "HTTP/1.1 200 OK\r\nX-Fixture: "+strings.Repeat("a", 17000)+"\r\n\r\n")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := dialThrough(ctx, Config{Type: "http", Host: "fixture.proxy", Port: 1080}, "example.invalid:443", &memoryDialer{conn: client})
	if err != ErrProbe {
		t.Fatal("unbounded proxy headers accepted")
	}
	<-finished
}

func TestTargetValidation(t *testing.T) {
	for _, target := range []string{"example.invalid:443", "127.0.0.1:8080", "[::1]:443"} {
		if !validTarget(target) {
			t.Fatal("valid target refused")
		}
	}
	for _, target := range []string{"example.invalid:0", "example.invalid:65536", "example.invalid:0443", "example.invalid/path:443", "user@example.invalid:443", "example.invalid\r\n:443", "0.0.0.0:443", "224.0.0.1:443", "[::1%zone]:443"} {
		if validTarget(target) {
			t.Fatal("unsafe target accepted")
		}
	}
}
