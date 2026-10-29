package browserproxy

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// Relay exposes SOCKS5 CONNECT on one ephemeral numeric loopback endpoint. It
// uses the same explicit upstream proxy as Probe; no credentials enter a child
// process command line. Closing the owner closes every accepted/upstream socket.
type Relay struct {
	listener    net.Listener
	ctx         context.Context
	cancel      context.CancelFunc
	config      Config
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	closed      bool
	once        sync.Once
	slots       chan struct{}
	credentials RelayCredentials
	dial        func(context.Context, Config, string) (net.Conn, error)
}

// RelayCredentials is passed only through the authenticated host's private
// stdin pipe. It must never be returned to frontend, logs or process args.
type RelayCredentials struct {
	Username string `json:"-"`
	Password string `json:"-"`
}

func (RelayCredentials) String() string   { return "[private relay credentials]" }
func (RelayCredentials) GoString() string { return "browserproxy.RelayCredentials{redacted}" }

func StartRelay(parent context.Context, config Config) (*Relay, error) {
	return startRelay(parent, config, rand.Reader, net.Listen, DialThrough)
}
func startRelay(parent context.Context, config Config, entropy io.Reader, listen func(string, string) (net.Listener, error), dial func(context.Context, Config, string) (net.Conn, error)) (*Relay, error) {
	if Validate(config) != nil {
		return nil, ErrConfig
	}
	if parent == nil || parent.Err() != nil {
		return nil, ErrProbe
	}
	var secret [32]byte
	if _, err := io.ReadFull(entropy, secret[:]); err != nil {
		return nil, ErrProbe
	}
	credentials := RelayCredentials{Username: "ultra-browser", Password: hex.EncodeToString(secret[:])}
	listener, err := listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, ErrProbe
	}
	ctx, cancel := context.WithCancel(parent)
	r := &Relay{listener: listener, ctx: ctx, cancel: cancel, config: config, connections: map[net.Conn]struct{}{}, slots: make(chan struct{}, 64), credentials: credentials, dial: dial}
	go r.accept()
	go func() { <-ctx.Done(); r.Close() }()
	return r, nil
}

func (r *Relay) Port() int { return r.listener.Addr().(*net.TCPAddr).Port }
func (r *Relay) Credentials() RelayCredentials {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.credentials
}
func (r *Relay) Close() {
	r.once.Do(func() {
		r.cancel()
		r.listener.Close()
		r.mu.Lock()
		r.closed = true
		for conn := range r.connections {
			conn.Close()
		}
		r.connections = map[net.Conn]struct{}{}
		r.config.Password = ""
		r.config.Username = ""
		r.credentials = RelayCredentials{}
		r.mu.Unlock()
	})
}
func (r *Relay) track(conn net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		conn.Close()
		return false
	}
	r.connections[conn] = struct{}{}
	return true
}
func (r *Relay) drop(conn net.Conn) {
	conn.Close()
	r.mu.Lock()
	delete(r.connections, conn)
	r.mu.Unlock()
}
func (r *Relay) accept() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.Close()
			return
		}
		select {
		case r.slots <- struct{}{}:
			if !r.track(conn) {
				<-r.slots
				continue
			}
			go func() { defer func() { <-r.slots }(); defer r.drop(conn); r.serve(conn) }()
		default:
			conn.Close()
		}
	}
}

func (r *Relay) serve(client net.Conn) {
	if r.ctx.Err() != nil {
		return
	}
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	target, err := readAuthenticatedSOCKSConnect(client, r.Credentials())
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	r.mu.Lock()
	config := r.config
	r.mu.Unlock()
	upstream, err := r.dial(ctx, config, target)
	cancel()
	if err != nil {
		_, _ = client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if !r.track(upstream) {
		return
	}
	defer r.drop(upstream)
	if _, err = client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	done := make(chan struct{}, 1)
	go func() { _, _ = io.Copy(upstream, client); upstream.Close(); done <- struct{}{} }()
	_, _ = io.Copy(client, upstream)
	client.Close()
	<-done
}

func readAuthenticatedSOCKSConnect(conn io.ReadWriter, credentials RelayCredentials) (string, error) {
	if credentials.Username != "ultra-browser" || len(credentials.Password) != 64 {
		return "", ErrProbe
	}
	for _, ch := range credentials.Password {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return "", ErrProbe
		}
	}
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 || header[1] == 0 {
		return "", ErrProbe
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", ErrProbe
	}
	const methodExpected byte = 2
	matched := false
	for _, method := range methods {
		if method == methodExpected {
			matched = true
		}
	}
	if !matched {
		_, _ = conn.Write([]byte{5, 255})
		return "", ErrProbe
	}
	if _, err := conn.Write([]byte{5, methodExpected}); err != nil {
		return "", ErrProbe
	}
	if methodExpected == 2 {
		auth := make([]byte, 2)
		if _, err := io.ReadFull(conn, auth); err != nil || auth[0] != 1 || auth[1] == 0 {
			return "", ErrProbe
		}
		user := make([]byte, int(auth[1]))
		if _, err := io.ReadFull(conn, user); err != nil {
			return "", ErrProbe
		}
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil || length[0] == 0 {
			return "", ErrProbe
		}
		password := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, password); err != nil {
			return "", ErrProbe
		}
		ok := subtle.ConstantTimeCompare(user, []byte(credentials.Username)) & subtle.ConstantTimeCompare(password, []byte(credentials.Password))
		if ok != 1 {
			_, _ = conn.Write([]byte{1, 1})
			return "", ErrProbe
		}
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return "", ErrProbe
		}
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil || request[0] != 5 || request[1] != 1 || request[2] != 0 {
		return "", ErrProbe
	}
	var host string
	switch request[3] {
	case 1:
		data := make([]byte, 4)
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", ErrProbe
		}
		host = net.IP(data).String()
	case 4:
		data := make([]byte, 16)
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", ErrProbe
		}
		host = net.IP(data).String()
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil || length[0] == 0 {
			return "", ErrProbe
		}
		data := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", ErrProbe
		}
		host = string(data)
	default:
		return "", ErrProbe
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return "", ErrProbe
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port))))
	if !validTarget(target) {
		return "", ErrProbe
	}
	return target, nil
}
