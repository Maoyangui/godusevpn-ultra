// Package browserproxy provides an explicit, isolated browser-profile proxy
// probe. It never changes the OS proxy, VPN, routing, DNS or browser settings.
package browserproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/idna"
)

var (
	ErrConfig = errors.New("browser_proxy_invalid_configuration")
	ErrProbe  = errors.New("browser_proxy_probe_failed")
	ErrBusy   = errors.New("browser_proxy_probe_busy")
)

// Secret fields are accepted only by the direct UI call. Config must never be
// serialized to logs, diagnostic exports, process arguments or probe results.
type Config struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func Decode(data string) (Config, error) {
	var c Config
	if len(data) == 0 || len(data) > 4096 {
		return c, ErrConfig
	}
	// Explicitly reject duplicate/case-alias keys; encoding/json otherwise
	// silently accepts them and can validate a different configuration.
	d := json.NewDecoder(strings.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return c, ErrConfig
	}
	seen := map[string]bool{}
	for d.More() {
		t, err := d.Token()
		key, ok := t.(string)
		if err != nil || !ok || seen[strings.ToLower(key)] {
			return c, ErrConfig
		}
		seen[strings.ToLower(key)] = true
		switch key {
		case "type", "host", "port", "username", "password":
		default:
			return c, ErrConfig
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return c, ErrConfig
		}
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return c, ErrConfig
	}
	if _, err := d.Token(); err != io.EOF {
		return c, ErrConfig
	}
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return Config{}, ErrConfig
	}
	if err := Validate(c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Validate(c Config) error {
	if c.Type != "socks5" && c.Type != "http" && c.Type != "https" {
		return ErrConfig
	}
	if c.Port < 1 || c.Port > 65535 || c.Host == "" || c.Host != strings.TrimSpace(c.Host) || len(c.Host) > 253 || strings.ContainsAny(c.Host, "/\\@?#% \t\r\n") {
		return ErrConfig
	}
	if ip, err := netip.ParseAddr(c.Host); err == nil {
		if ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return ErrConfig
		}
	} else {
		host, err := idna.Lookup.ToASCII(c.Host)
		if err != nil || len(host) > 253 || strings.HasSuffix(host, ".") || strings.Contains(host, ":") {
			return ErrConfig
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ErrConfig
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
					return ErrConfig
				}
			}
		}
	}
	if len(c.Username) > 255 || len(c.Password) > 255 || (c.Username == "" && c.Password != "") || strings.Contains(c.Username, ":") {
		return ErrConfig
	}
	for _, value := range []string{c.Username, c.Password} {
		for _, ch := range value {
			if unicode.IsControl(ch) {
				return ErrConfig
			}
		}
	}
	return nil
}

func proxyURL(c Config) *url.URL {
	host := c.Host
	if _, err := netip.ParseAddr(host); err != nil {
		host, _ = idna.Lookup.ToASCII(host)
	}
	u := &url.URL{Scheme: c.Type, Host: net.JoinHostPort(host, strconv.Itoa(c.Port))}
	if c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	return u
}

// Result is intentionally separate from Config. IP/location stay in the
// requesting page's memory; they are not a browser-runtime or GPS assertion.
type Result struct {
	Status      string `json:"status"`
	Scope       string `json:"scope"`
	Provider    string `json:"provider"`
	IP          string `json:"ip"`
	Country     string `json:"country"`
	City        string `json:"city,omitempty"`
	Subdivision string `json:"subdivision,omitempty"`
	LatencyMS   int64  `json:"latencyMs"`
	CheckedAt   string `json:"checkedAt"`
}

type Prober struct{ slots chan struct{} }

func NewProber() *Prober { return &Prober{slots: make(chan struct{}, 1)} }

// NewTransport constructs a strict explicit-proxy route, without dialing. The
// caller must close idle connections and keep credentials out of diagnostics.
func NewTransport(config Config) (*http.Transport, error) {
	if err := Validate(config); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	return newTransport(proxyURL(config), dialer.DialContext), nil
}

// Probe is called only by an explicit UI action. The sole destination is the
// documented free, keyless country.is API. No arbitrary URL, redirect, cookie,
// fingerprint, credential or profile is sent to that API.
func (p *Prober) Probe(ctx context.Context, c Config) (Result, error) {
	if err := Validate(c); err != nil {
		return Result{}, err
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		return Result{}, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	u := proxyURL(c)
	dialer := &net.Dialer{Timeout: 6 * time.Second, KeepAlive: -1}
	transport := newTransport(u, dialer.DialContext)
	return runProbe(ctx, transport)
}

type dialFunc func(context.Context, string, string) (net.Conn, error)

func newTransport(u *url.URL, dial dialFunc) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyURL(u), DisableKeepAlives: true, DisableCompression: true,
		ForceAttemptHTTP2: false, TLSHandshakeTimeout: 6 * time.Second, ResponseHeaderTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != u.Host || network != "tcp" {
				return nil, ErrProbe
			}
			return dial(ctx, network, address)
		},
	}
}

func runProbe(ctx context.Context, transport *http.Transport) (Result, error) {
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrProbe }}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.country.is/?fields=city,subdivision", nil)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "GodUseVPN-Ultra-ProxyCheck/1")
	start := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return Result{}, ErrProbe
	} // never leak a URL/auth error
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Result{}, ErrProbe
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(data) > 16384 {
		return Result{}, ErrProbe
	}
	result, err := parseResponse(data)
	if err != nil {
		return Result{}, ErrProbe
	}
	result.Status = "reachable"
	result.Scope = "profile_proxy_probe_only"
	result.Provider = "country.is"
	result.LatencyMS = time.Since(start).Milliseconds()
	result.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	return result, nil
}

func parseResponse(data []byte) (Result, error) {
	var body struct {
		IP          string          `json:"ip"`
		Country     string          `json:"country"`
		City        json.RawMessage `json:"city"`
		Subdivision json.RawMessage `json:"subdivision"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&body) != nil {
		return Result{}, ErrProbe
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Result{}, ErrProbe
	}
	ip, err := netip.ParseAddr(body.IP)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return Result{}, ErrProbe
	}
	if len(body.Country) != 2 || body.Country[0] < 'A' || body.Country[0] > 'Z' || body.Country[1] < 'A' || body.Country[1] > 'Z' {
		return Result{}, ErrProbe
	}
	return Result{IP: ip.String(), Country: body.Country, City: placeName(body.City), Subdivision: placeName(body.Subdivision)}, nil
}

func placeName(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(data, &value) != nil {
		var object struct {
			Name  string            `json:"name"`
			Names map[string]string `json:"names"`
		}
		if json.Unmarshal(data, &object) != nil {
			return ""
		}
		value = object.Name
		if value == "" {
			value = object.Names["en"]
		}
	}
	if len(value) > 160 {
		return ""
	}
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return ""
		}
	}
	return value
}
