// Package locationportal controls only the documented local Windows Device
// Portal location endpoints. It never enables the portal, changes Developer
// Mode, modifies its authentication, or accepts an arbitrary URL or command.
package locationportal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInput          = errors.New("LOCATION_PORTAL_INPUT_REFUSED")
	ErrConnection     = errors.New("LOCATION_PORTAL_CONNECTION_UNVERIFIED")
	ErrAuthentication = errors.New("LOCATION_PORTAL_AUTHENTICATION_REQUIRED")
	ErrUnsupported    = errors.New("LOCATION_PORTAL_COMPONENT_UNAVAILABLE")
	ErrReadback       = errors.New("LOCATION_PORTAL_READBACK_UNCONFIRMED")
	ErrConflict       = errors.New("LOCATION_PORTAL_EXTERNAL_CHANGE_PRESERVED")
	ErrStore          = errors.New("LOCATION_PORTAL_PRIVATE_BACKUP_UNAVAILABLE")
	ErrBusy           = errors.New("LOCATION_PORTAL_OPERATION_BUSY")
)

// Config is an explicit, in-memory credential input. Never persist or export it.
// Only a numeric HTTPS port on literal 127.0.0.1 can be selected.
type Config struct {
	Port              int    `json:"port"`
	CertificateSHA256 string `json:"certificateSHA256"`
	Username          string `json:"username"`
	Password          string `json:"password"`
}

func (Config) String() string   { return "<private-local-location-portal-connection>" }
func (Config) GoString() string { return "<private-local-location-portal-connection>" }
func (Config) MarshalJSON() ([]byte, error) {
	return []byte(`"<private-local-location-portal-connection>"`), nil
}

type Position struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (Position) String() string   { return "<private-location-position>" }
func (Position) GoString() string { return "<private-location-position>" }
func (p *Position) UnmarshalJSON(raw []byte) error {
	var v struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if exactJSON(raw, []string{"latitude", "longitude"}, &v) != nil || v.Latitude == nil || v.Longitude == nil {
		return ErrInput
	}
	value := Position{*v.Latitude, *v.Longitude}
	if !validPosition(value) {
		return ErrInput
	}
	*p = value
	return nil
}
func validPosition(p Position) bool {
	return !math.IsNaN(p.Latitude) && !math.IsNaN(p.Longitude) && !math.IsInf(p.Latitude, 0) && !math.IsInf(p.Longitude, 0) && p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
}

const overridePath = "/ext/location/override"
const positionPath = "/ext/location/position"

// The Microsoft core API reference currently documents this exact write URI,
// despite the separate GET /position URI. Do not try guessed alternatives.
// A successful write still requires independent GET configuration readback.
const positionWritePath = overridePath
const maxResponse = 16 << 10

type Client struct {
	mu     sync.Mutex
	config Config
	http   *http.Client
	csrf   string
	closed bool
}

func validConfig(c Config) bool {
	if c.Port < 1024 || c.Port > 65535 || len(c.CertificateSHA256) != 64 || strings.ToLower(c.CertificateSHA256) != c.CertificateSHA256 || len(c.Username) == 0 || len(c.Username) > 128 || strings.HasPrefix(c.Username, "auto-") || len(c.Password) == 0 || len(c.Password) > 1024 || !utf8.ValidString(c.Password) {
		return false
	}
	if b, e := hex.DecodeString(c.CertificateSHA256); e != nil || len(b) != 32 {
		return false
	}
	for _, r := range c.Username {
		if r < 33 || r > 126 || r == ':' {
			return false
		}
	}
	for _, r := range c.Password {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

func pinnedCertificate(certificate *x509.Certificate, expected string, now time.Time) error {
	// The literal loopback transport has no DNS name. The exact DER pin from
	// the trusted local HTTP.sys binding (or explicitly reviewed input) is the
	// server identity; the official generated certificate may name the device.
	if certificate == nil || now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
		return ErrConnection
	}
	if certificate.IsCA {
		return ErrConnection
	}
	if len(certificate.ExtKeyUsage) > 0 {
		server := false
		for _, usage := range certificate.ExtKeyUsage {
			if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
				server = true
			}
		}
		if !server {
			return ErrConnection
		}
	}
	digest := sha256.Sum256(certificate.Raw)
	want, err := hex.DecodeString(expected)
	if err != nil || len(want) != 32 || subtle.ConstantTimeCompare(want, digest[:]) != 1 {
		return ErrConnection
	}
	return nil
}

// New constructs a transport only; no socket, native API, or file is opened.
func NewClient(c Config) (*Client, error) {
	if !validConfig(c) {
		return nil, ErrInput
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port))
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 8192,
		DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
			if network != "tcp" || target != address {
				return nil, ErrConnection
			}
			return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "tcp4", address)
		},
		TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 4 * time.Second,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, // Custom exact certificate trust, not an unverified TLS fallback.
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return ErrConnection
				}
				return pinnedCertificate(state.PeerCertificates[0], c.CertificateSHA256, time.Now())
			}},
	}
	return &Client{config: c, http: &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrConnection }}}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.csrf = ""
	c.config.Password = ""
	c.config.Username = ""
	c.http.CloseIdleConnections()
	return nil
}

func basicChallenge(headers http.Header) bool {
	for _, value := range headers.Values("WWW-Authenticate") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "basic ") {
			return true
		}
	}
	return false
}
func validCSRF(value string) bool {
	if len(value) < 1 || len(value) > 2048 {
		return false
	}
	for _, r := range value {
		if r < 33 || r > 126 || strings.ContainsRune(";,\"\\", r) {
			return false
		}
	}
	return true
}

// exchange is deliberately not exported: callers cannot select another URI.
func (c *Client) exchange(ctx context.Context, method, path string, body []byte, authenticated bool) (int, []byte, error) {
	if c.closed || ctx == nil || ctx.Err() != nil || (path != overridePath && path != positionPath) || (method != http.MethodGet && method != http.MethodPut) {
		return 0, nil, ErrConnection
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://127.0.0.1:"+strconv.Itoa(c.config.Port)+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, ErrInput
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "GodUseVPN-Ultra-Location/1")
	if method == http.MethodGet {
		request.Header.Set("CSRF-Token", "Fetch")
	} else {
		if !validCSRF(c.csrf) {
			return 0, nil, ErrAuthentication
		}
		request.Header.Set("X-CSRF-Token", c.csrf)
		request.AddCookie(&http.Cookie{Name: "CSRF-Token", Value: c.csrf})
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.SetBasicAuth(c.config.Username, c.config.Password)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, ErrConnection
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized && !authenticated && basicChallenge(response.Header) {
		return response.StatusCode, nil, nil
	}
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return response.StatusCode, nil, ErrAuthentication
	}
	if response.StatusCode == 404 {
		return response.StatusCode, nil, ErrUnsupported
	}
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, nil, ErrReadback
	}
	if !authenticated {
		return response.StatusCode, nil, ErrAuthentication
	} // Never operate an authentication-disabled portal.
	if response.Uncompressed || response.Header.Get("Content-Encoding") != "" {
		return response.StatusCode, nil, ErrReadback
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return response.StatusCode, nil, ErrReadback
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "CSRF-Token" && validCSRF(cookie.Value) {
			c.csrf = cookie.Value
		}
	}
	return response.StatusCode, raw, nil
}

func exactJSON(raw []byte, keys []string, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ErrReadback
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		text, ok := key.(string)
		if err != nil || !ok || seen[text] {
			return ErrReadback
		}
		allowed := false
		for _, want := range keys {
			if text == want {
				allowed = true
			}
		}
		if !allowed {
			return ErrReadback
		}
		seen[text] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return ErrReadback
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != len(keys) {
		return ErrReadback
	}
	if _, err = decoder.Token(); err != io.EOF {
		return ErrReadback
	}
	if json.Unmarshal(raw, out) != nil {
		return ErrReadback
	}
	return nil
}
func decodeOverride(raw []byte) (bool, error) {
	var value struct{ Override *bool }
	if exactJSON(raw, []string{"Override"}, &value) != nil || value.Override == nil {
		return false, ErrReadback
	}
	return *value.Override, nil
}
func decodePosition(raw []byte) (Position, error) {
	var value struct{ Latitude, Longitude *float64 }
	if exactJSON(raw, []string{"Latitude", "Longitude"}, &value) != nil || value.Latitude == nil || value.Longitude == nil {
		return Position{}, ErrReadback
	}
	p := Position{*value.Latitude, *value.Longitude}
	if !validPosition(p) {
		return Position{}, ErrReadback
	}
	return p, nil
}

// Check authenticates only the fixed location endpoint. No override is written.
func (c *Client) Check(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, _, err := c.exchange(ctx, http.MethodGet, overridePath, nil, false)
	if err != nil {
		return false, err
	}
	if status != 401 {
		return false, ErrAuthentication
	}
	_, raw, err := c.exchange(ctx, http.MethodGet, overridePath, nil, true)
	if err != nil {
		return false, err
	}
	return decodeOverride(raw)
}
func (c *Client) Override(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, raw, err := c.exchange(ctx, http.MethodGet, overridePath, nil, true)
	if err != nil {
		return false, err
	}
	return decodeOverride(raw)
}
func (c *Client) Injected(ctx context.Context) (Position, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, raw, err := c.exchange(ctx, http.MethodGet, positionPath, nil, true)
	if err != nil {
		return Position{}, err
	}
	return decodePosition(raw)
}
func (c *Client) setOverride(ctx context.Context, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	body, _ := json.Marshal(struct{ Override bool }{enabled})
	_, raw, err := c.exchange(ctx, http.MethodPut, overridePath, body, true)
	if err != nil {
		return err
	}
	got, err := decodeOverride(raw)
	if err != nil || got != enabled {
		return ErrReadback
	}
	return nil
}
func (c *Client) setPosition(ctx context.Context, p Position) error {
	if !validPosition(p) {
		return ErrInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	body, _ := json.Marshal(struct{ Latitude, Longitude float64 }{p.Latitude, p.Longitude})
	_, raw, err := c.exchange(ctx, http.MethodPut, positionWritePath, body, true)
	if err != nil {
		return err
	}
	got, err := decodePosition(raw)
	if err != nil || got != p {
		return ErrReadback
	}
	return nil
}
