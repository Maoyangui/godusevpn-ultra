package locationportal

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureConfig() Config {
	return Config{58443, strings.Repeat("a", 64), "ultra-location", "not-a-real-password"}
}
func response(status int, text string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(text))}
}
func TestFixedTransportConstruction(t *testing.T) {
	c, e := NewClient(fixtureConfig())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	tr := c.http.Transport.(*http.Transport)
	if tr.Proxy != nil || !tr.DisableCompression || tr.TLSClientConfig.MinVersion < 0x303 || c.http.Timeout > 6*time.Second {
		t.Fatal("transport widened")
	}
	for _, target := range []string{"example.invalid:58443", "127.0.0.2:58443", "127.0.0.1:443", "[::1]:58443"} {
		if _, e = tr.DialContext(context.Background(), "tcp", target); !errors.Is(e, ErrConnection) {
			t.Fatal("non-fixed target allowed")
		}
	}
	if e = c.http.CheckRedirect(nil, nil); !errors.Is(e, ErrConnection) {
		t.Fatal("redirect accepted")
	}
	raw, _ := json.Marshal(c.config)
	if strings.Contains(string(raw), "password") || strings.Contains(string(raw), c.config.Password) {
		t.Fatal("credentials serialized")
	}
}
func TestCertificateExactPinWithoutInventedHostname(t *testing.T) {
	now := time.Unix(1700000000, 0)
	c := &x509.Certificate{Raw: []byte("synthetic-cert"), DNSNames: []string{"test-device"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	h := sha256.Sum256(c.Raw)
	pin := hex.EncodeToString(h[:])
	if pinnedCertificate(c, pin, now) != nil {
		t.Fatal("trusted exact device certificate rejected")
	}
	tests := []struct {
		name   string
		mutate func(*x509.Certificate)
		pin    string
		now    time.Time
	}{{"wrong pin", func(*x509.Certificate) {}, strings.Repeat("0", 64), now}, {"expired", func(*x509.Certificate) {}, pin, now.Add(2 * time.Hour)}, {"not yet", func(*x509.Certificate) {}, pin, now.Add(-2 * time.Hour)}, {"CA", func(c *x509.Certificate) { c.IsCA = true }, pin, now}, {"client only", func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth} }, pin, now}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			copy := *c
			tc.mutate(&copy)
			if pinnedCertificate(&copy, tc.pin, tc.now) == nil {
				t.Fatal("bad certificate accepted")
			}
		})
	}
}
func TestAuthenticationCSRFAndDocumentedURIs(t *testing.T) {
	c, _ := NewClient(fixtureConfig())
	defer c.Close()
	requests := 0
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Scheme != "https" || r.URL.Host != "127.0.0.1:58443" {
			t.Fatal("escaped loopback")
		}
		if requests == 1 {
			if r.Header.Get("Authorization") != "" {
				t.Fatal("sent credentials before challenge")
			}
			v := response(401, "")
			v.Header.Set("WWW-Authenticate", `Basic realm="WDP"`)
			return v, nil
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "ultra-location" || password != "not-a-real-password" {
			t.Fatal("wrong fixed auth")
		}
		v := response(200, `{"Override":false}`)
		v.Header.Add("Set-Cookie", "CSRF-Token=synthetic-nonce; Path=/")
		if r.Method == http.MethodPut {
			if r.URL.Path != overridePath {
				t.Fatal("guessed write URI")
			}
			if r.Header.Get("X-CSRF-Token") != "synthetic-nonce" {
				t.Fatal("missing nonce")
			}
			cookie, e := r.Cookie("CSRF-Token")
			if e != nil || cookie.Value != "synthetic-nonce" {
				t.Fatal("missing csrf cookie")
			}
			raw, _ := io.ReadAll(r.Body)
			if string(raw) == `{"Override":true}` {
				return response(200, string(raw)), nil
			}
			if string(raw) != `{"Latitude":12.5,"Longitude":-23}` {
				t.Fatalf("unexpected fixed body")
			}
			return response(200, string(raw)), nil
		}
		return v, nil
	})
	if on, e := c.Check(context.Background()); e != nil || on {
		t.Fatal("challenge failed", e)
	}
	if e := c.setPosition(context.Background(), Position{12.5, -23}); e != nil {
		t.Fatal(e)
	}
	if e := c.setOverride(context.Background(), true); e != nil {
		t.Fatal(e)
	}
}
func TestFixedClientRefusesUntrustedResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		header http.Header
		want   error
	}{{"auth disabled", 200, `{"Override":false}`, nil, ErrAuthentication}, {"wrong scheme", 401, "", http.Header{"Www-Authenticate": []string{"Digest realm=bad"}}, ErrAuthentication}, {"not installed", 404, "", nil, ErrUnsupported}, {"server failure", 500, "secret raw server details", nil, ErrReadback}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := NewClient(fixtureConfig())
			c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				v := response(tc.status, tc.body)
				if tc.header != nil {
					v.Header = tc.header
				}
				return v, nil
			})
			_, e := c.Check(context.Background())
			if !errors.Is(e, tc.want) {
				t.Fatal(e)
			}
			c.Close()
		})
	}
	for _, body := range []string{`{"Override":false,"Override":true}`, `{"override":false}`, `{"Override":null}`, `{"Override":false,"secret":0}`, `{"Override":false} {}`, strings.Repeat("x", maxResponse+1)} {
		t.Run("body", func(t *testing.T) {
			c, _ := NewClient(fixtureConfig())
			c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(200, body), nil })
			if _, e := c.Override(context.Background()); e == nil {
				t.Fatal("unverified response accepted")
			}
			c.Close()
		})
	}
	c, _ := NewClient(fixtureConfig())
	c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("https://127.0.0.1 secret") })
	if _, e := c.Check(context.Background()); e != ErrConnection {
		t.Fatal("raw connection error escaped")
	}
	c.Close()
	if _, e := c.Override(context.Background()); e != ErrConnection {
		t.Fatal("closed accepted")
	}
}
func TestStrictUserInputs(t *testing.T) {
	for _, raw := range []string{`{"points":[{}],"intervalSeconds":0}`, `{"points":[{"latitude":null,"longitude":0}],"intervalSeconds":0}`, `{"points":[{"latitude":0,"Longitude":0}],"intervalSeconds":0}`, `{"points":[{"latitude":0,"longitude":0}],"points":[],"intervalSeconds":0}`, `{"points":[{"latitude":91,"longitude":0}],"intervalSeconds":0}`, `{"points":[{"latitude":0,"longitude":0}],"intervalSeconds":2}`, `{"points":[{"latitude":0,"longitude":0},{"latitude":1,"longitude":1}],"intervalSeconds":1}`} {
		if _, e := ParseApply(raw); e == nil {
			t.Fatal("malformed accepted")
		}
	}
	if r, e := ParseApply(`{"points":[{"latitude":0,"longitude":0}],"intervalSeconds":0}`); e != nil || len(r.Points) != 1 {
		t.Fatal(e)
	}
	for _, change := range []func(*Config){func(c *Config) { c.Port = 80 }, func(c *Config) { c.Username = "auto-bypass" }, func(c *Config) { c.Username = "user:name" }, func(c *Config) { c.CertificateSHA256 = strings.Repeat("A", 64) }, func(c *Config) { c.Password = "bad\npassword" }} {
		c := fixtureConfig()
		change(&c)
		if validConfig(c) {
			t.Fatal("bad config accepted")
		}
	}
}
