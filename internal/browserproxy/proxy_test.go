package browserproxy

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	for _, data := range []string{
		`{"type":"socks5","host":"127.0.0.1","port":1080}`,
		`{"type":"http","host":"proxy.example","port":8080,"username":"fixture","password":"p:word"}`,
		`{"type":"https","host":"::1","port":443}`,
	} {
		if _, err := Decode(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, data := range []string{
		`null`, `{}`, `{"type":"http","host":"proxy.example","port":0}`,
		`{"type":"http","host":"proxy.example/path","port":8080}`,
		`{"type":"http","host":"proxy.example","port":8080,"Host":"evil.example"}`,
		`{"type":"http","host":"proxy.example","port":8080,"host":"evil.example"}`,
		`{"type":"http","host":"proxy.example","port":8080,"command":"bad"}`,
		`{"type":"http","host":"proxy.example","port":8080,"password":"secret"}`,
		`{"type":"http","host":"proxy.example","port":8080,"username":"a:b"}`,
		`{"type":"http","host":"0.0.0.0","port":8080}`,
		`{"type":"http","host":"224.0.0.1","port":8080}`,
		`{"type":"http","host":"::1%25zone","port":8080}`,
		`{"type":"http","host":"proxy.example","port":8080} {}`,
	} {
		if _, err := Decode(data); !errors.Is(err, ErrConfig) {
			t.Fatalf("configuration was accepted: %s", data)
		}
	}
}

func TestResponseMinimization(t *testing.T) {
	result, err := parseResponse([]byte(`{"ip":"9.9.9.9","country":"US","city":{"name":"Fixture City"},"subdivision":{"names":{"en":"Fixture Region"}},"location":{"latitude":1,"longitude":2}}`))
	if err != nil || result.IP != "9.9.9.9" || result.City != "Fixture City" || result.Subdivision != "Fixture Region" {
		t.Fatal("valid response was not minimized")
	}
	for _, data := range []string{`{}`, `{"ip":"127.0.0.1","country":"US"}`, `{"ip":"10.0.0.1","country":"US"}`, `{"ip":"9.9.9.9","country":"USA"}`, `{"ip":"9.9.9.9","country":"us"}`, `{"ip":"9.9.9.9","country":"US"} {}`} {
		if _, err := parseResponse([]byte(data)); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"api.country.is", "fixture.proxy"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// The real net/http proxy implementation runs against net.Pipe, with TLS and
// an in-memory fixture CA. No listener, DNS, external HTTP or product is used.
func TestProbeThroughAuthenticatedProxy(t *testing.T) {
	cert, pool := testCertificate(t)
	for _, kind := range []string{"http", "https", "socks5"} {
		t.Run(kind, func(t *testing.T) {
			config := Config{Type: kind, Host: "fixture.proxy", Port: 1443, Username: "fixture", Password: "fixture-secret"}
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
			serverDone := make(chan error, 1)
			go func() {
				var proxyConn net.Conn = serverConn
				if kind == "https" {
					proxyConn = tls.Server(serverConn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
				}
				reader := bufio.NewReader(proxyConn)
				if kind == "http" || kind == "https" {
					request, err := http.ReadRequest(reader)
					if err != nil {
						serverDone <- err
						return
					}
					expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("fixture:fixture-secret"))
					if request.Method != "CONNECT" || request.Host != "api.country.is:443" || request.Header.Get("Proxy-Authorization") != expected {
						serverDone <- errors.New("wrong CONNECT or proxy credentials")
						return
					}
					_, _ = io.WriteString(proxyConn, "HTTP/1.1 200 Connection Established\r\n\r\n")
				} else {
					greeting := make([]byte, 2)
					if _, err := io.ReadFull(reader, greeting); err != nil {
						serverDone <- err
						return
					}
					if greeting[0] != 5 {
						serverDone <- errors.New("not socks5")
						return
					}
					methods := make([]byte, int(greeting[1]))
					_, _ = io.ReadFull(reader, methods)
					_, _ = serverConn.Write([]byte{5, 2})
					auth := make([]byte, 2)
					_, _ = io.ReadFull(reader, auth)
					username := make([]byte, int(auth[1]))
					_, _ = io.ReadFull(reader, username)
					length, _ := reader.ReadByte()
					password := make([]byte, int(length))
					_, _ = io.ReadFull(reader, password)
					if auth[0] != 1 || string(username) != "fixture" || string(password) != "fixture-secret" {
						serverDone <- errors.New("wrong socks authentication")
						return
					}
					_, _ = serverConn.Write([]byte{1, 0})
					header := make([]byte, 5)
					_, _ = io.ReadFull(reader, header)
					if header[0] != 5 || header[1] != 1 || header[3] != 3 {
						serverDone <- errors.New("destination must use remote DNS")
						return
					}
					host := make([]byte, int(header[4]))
					_, _ = io.ReadFull(reader, host)
					port := make([]byte, 2)
					_, _ = io.ReadFull(reader, port)
					if string(host) != "api.country.is" || binary.BigEndian.Uint16(port) != 443 {
						serverDone <- errors.New("unexpected socks destination")
						return
					}
					_, _ = serverConn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				}
				secure := tls.Server(proxyConn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
				request, err := http.ReadRequest(bufio.NewReader(secure))
				if err != nil {
					serverDone <- err
					return
				}
				if request.Header.Get("Proxy-Authorization") != "" || request.Header.Get("Authorization") != "" || request.URL.Path != "/" || request.URL.RawQuery != "fields=city,subdivision" {
					serverDone <- errors.New("origin credential disclosure or wrong API")
					return
				}
				body := `{"ip":"9.9.9.9","country":"US","city":"Fixture City"}`
				_, err = fmt.Fprintf(secure, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
				serverDone <- err
			}()
			calls := 0
			transport := newTransport(proxyURL(config), func(_ context.Context, network, address string) (net.Conn, error) {
				calls++
				if network != "tcp" || address != "fixture.proxy:1443" {
					return nil, errors.New("direct destination dial refused")
				}
				return clientConn, nil
			})
			transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := runProbe(ctx, transport)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || result.Scope != "profile_proxy_probe_only" || result.IP != "9.9.9.9" || result.Provider != "country.is" {
				t.Fatal("incomplete route/result evidence")
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProxyFailureCannotFallback(t *testing.T) {
	u := proxyURL(Config{Type: "http", Host: "fixture.proxy", Port: 8080, Username: "fixture", Password: "do-not-return"})
	calls := 0
	transport := newTransport(u, func(context.Context, string, string) (net.Conn, error) {
		calls++
		return nil, errors.New("do-not-return")
	})
	_, err := runProbe(context.Background(), transport)
	if err != ErrProbe || strings.Contains(err.Error(), "do-not-return") || calls != 1 {
		t.Fatal("proxy failure must be redacted, terminal and single-route")
	}
	if _, err := transport.DialContext(context.Background(), "tcp", "api.country.is:443"); err != ErrProbe {
		t.Fatal("direct destination was allowed")
	}
}
