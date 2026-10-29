package browserproxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// DialThrough opens one remote TCP connection through the explicit proxy. DNS
// for the destination is performed by that proxy; only the proxy endpoint may
// be resolved locally. It has no direct, PAC, environment or system-proxy path.
func DialThrough(ctx context.Context, config Config, target string) (net.Conn, error) {
	if Validate(config) != nil || !validTarget(target) {
		return nil, ErrConfig
	}
	u := proxyURL(config)
	base := &proxyOnlyDialer{endpoint: u.Host, dialer: net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}}
	return dialThrough(ctx, config, target, base)
}

type upstreamDialer interface {
	proxy.Dialer
	proxy.ContextDialer
}

func dialThrough(ctx context.Context, config Config, target string, base upstreamDialer) (net.Conn, error) {
	if Validate(config) != nil || !validTarget(target) {
		return nil, ErrConfig
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrProbe
	}
	// The public adapter also accepts a background context. Bound the SOCKS
	// and HTTPS-proxy handshakes themselves, before any TLS read can stall.
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	u := proxyURL(config)
	if config.Type == "socks5" {
		var auth *proxy.Auth
		if config.Username != "" {
			auth = &proxy.Auth{User: config.Username, Password: config.Password}
		}
		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, base)
		if err != nil {
			return nil, ErrProbe
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, ErrProbe
		}
		conn, err := contextDialer.DialContext(ctx, "tcp", target)
		if err != nil {
			return nil, ErrProbe
		}
		return conn, nil
	}
	conn, err := base.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, ErrProbe
	}
	if config.Type == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if secure.HandshakeContext(ctx) != nil {
			conn.Close()
			return nil, ErrProbe
		}
		conn = secure
	}
	deadline := time.Now().Add(8 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	var headers http.Header
	if config.Username != "" {
		headers = make(http.Header)
		headers.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(config.Username+":"+config.Password)))
	}
	request := &http.Request{Method: http.MethodConnect, Host: target, Header: headers}
	// Write the fixed CONNECT request explicitly; target has already passed
	// strict host/port validation and cannot inject headers or URL userinfo.
	var text bytes.Buffer
	text.WriteString("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n")
	if value := request.Header.Get("Proxy-Authorization"); value != "" {
		text.WriteString("Proxy-Authorization: " + value + "\r\n")
	}
	text.WriteString("\r\n")
	if _, err = io.Copy(conn, &text); err != nil {
		stop()
		conn.Close()
		return nil, ErrProbe
	}
	reader := bufio.NewReaderSize(conn, 4096)
	var header bytes.Buffer
	for {
		line, err := reader.ReadSlice('\n')
		header.Write(line)
		if err != nil || header.Len() > 16384 {
			stop()
			conn.Close()
			return nil, ErrProbe
		}
		if bytes.Equal(line, []byte("\r\n")) {
			break
		}
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header.Bytes())), request)
	if err != nil || response.StatusCode != http.StatusOK {
		stop()
		conn.Close()
		return nil, ErrProbe
	}
	if !stop() || ctx.Err() != nil {
		conn.Close()
		return nil, ErrProbe
	}
	_ = conn.SetDeadline(time.Time{})
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

type proxyOnlyDialer struct {
	endpoint string
	dialer   net.Dialer
}

func (d *proxyOnlyDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}
func (d *proxyOnlyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || address != d.endpoint {
		return nil, ErrProbe
	}
	return d.dialer.DialContext(ctx, network, address)
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(data []byte) (int, error) { return c.reader.Read(data) }

func validTarget(target string) bool {
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || len(host) > 253 {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast()
	}
	if strings.ContainsAny(host, ":/\\@?#% \t\r\n") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, part := range strings.Split(host, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, ch := range part {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}
