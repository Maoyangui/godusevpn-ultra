package browserenginehost

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

// Stock Chromium does not implement SOCKS5 username/password authentication.
// This frontend is not a bearer capability: a kernel tuple/Job/image check must
// authorize the real browser client before its upstream credentials are used.
// Only its no-auth greeting is translated; CONNECT bytes remain SOCKS domain
// requests, so this bridge performs no website DNS resolution or direct dial.
type frontendRelay struct {
	listener         net.Listener
	ctx              context.Context
	cancel           context.CancelFunc
	backendPort      int
	credentials      browserproxy.RelayCredentials
	authorize        func(net.Conn) (io.Closer, error)
	authorizeBackend func(net.Conn) error
	mu               sync.Mutex
	connections      map[net.Conn]struct{}
	opened, closed   bool
	slots            chan struct{}
	once             sync.Once
	failed           func()
}

func startFrontend(ctx context.Context, backendPort int, credentials browserproxy.RelayCredentials, authorize func(net.Conn) (io.Closer, error), authorizeBackend func(net.Conn) error, failed func()) (*frontendRelay, error) {
	if backendPort < 1 || backendPort > 65535 || credentials.Username != "ultra-browser" || len(credentials.Password) != 64 || authorize == nil || authorizeBackend == nil {
		return nil, ErrRefused
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, ErrRefused
	}
	ctx, cancel := context.WithCancel(ctx)
	relay := &frontendRelay{listener: listener, ctx: ctx, cancel: cancel, backendPort: backendPort, credentials: credentials, authorize: authorize, authorizeBackend: authorizeBackend, connections: map[net.Conn]struct{}{}, slots: make(chan struct{}, 64), failed: failed}
	go relay.accept()
	go func() { <-ctx.Done(); relay.Close() }()
	return relay, nil
}
func (r *frontendRelay) Port() int { return r.listener.Addr().(*net.TCPAddr).Port }
func (r *frontendRelay) Open() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRefused
	}
	r.opened = true
	return nil
}
func (r *frontendRelay) track(connection net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.opened {
		connection.Close()
		return false
	}
	r.connections[connection] = struct{}{}
	return true
}
func (r *frontendRelay) drop(connection net.Conn) {
	connection.Close()
	r.mu.Lock()
	delete(r.connections, connection)
	r.mu.Unlock()
}
func (r *frontendRelay) Close() {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.opened = false
		for connection := range r.connections {
			connection.Close()
		}
		r.connections = map[net.Conn]struct{}{}
		r.credentials = browserproxy.RelayCredentials{}
		r.mu.Unlock()
		r.cancel()
		r.listener.Close()
	})
}
func (r *frontendRelay) accept() {
	for {
		client, err := r.listener.Accept()
		if err != nil {
			r.mu.Lock()
			closed := r.closed
			r.mu.Unlock()
			if !closed {
				r.abort()
			}
			return
		}
		select {
		case r.slots <- struct{}{}:
			if !r.track(client) {
				<-r.slots
				continue
			}
			go func() { defer func() { <-r.slots }(); defer r.drop(client); r.serve(client) }()
		default:
			client.Close()
		}
	}
}
func (r *frontendRelay) serve(client net.Conn) {
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	owner, err := r.authorize(client)
	if err != nil {
		return
	}
	defer owner.Close()
	if readFrontendGreeting(client) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	dialer := net.Dialer{Timeout: 5 * time.Second}
	upstream, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(r.backendPort)))
	if err != nil {
		r.abort()
		return
	}
	if !r.track(upstream) {
		return
	}
	defer r.drop(upstream)
	upstream.SetDeadline(time.Now().Add(5 * time.Second))
	r.mu.Lock()
	credentials := r.credentials
	r.mu.Unlock()
	if r.authorizeBackend(upstream) != nil || authenticateBackend(upstream, credentials) != nil || r.authorizeBackend(upstream) != nil {
		r.abort()
		return
	}
	// The owner is rechecked after the upstream exchange so an exited/replaced
	// process or Job cannot receive an accepted frontend method afterward.
	check, err := r.authorize(client)
	if err != nil {
		return
	}
	check.Close()
	if writeHandshake(client, []byte{5, 0}) != nil {
		return
	}
	client.SetDeadline(time.Time{})
	upstream.SetDeadline(time.Time{})
	done := make(chan struct{}, 1)
	go func() { io.Copy(upstream, client); upstream.Close(); done <- struct{}{} }()
	io.Copy(client, upstream)
	client.Close()
	<-done
}
func (r *frontendRelay) abort() {
	r.Close()
	if r.failed != nil {
		go r.failed()
	}
}
func readFrontendGreeting(stream io.ReadWriter) error {
	var header [2]byte
	if _, err := io.ReadFull(stream, header[:]); err != nil || header[0] != 5 || header[1] == 0 || header[1] > 16 {
		return ErrRefused
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(stream, methods); err != nil {
		return ErrRefused
	}
	for _, method := range methods {
		if method == 0 {
			return nil
		}
	}
	return ErrRefused
}
func authenticateBackend(stream io.ReadWriter, credentials browserproxy.RelayCredentials) error {
	if credentials.Username != "ultra-browser" || len(credentials.Password) != 64 {
		return ErrRefused
	}
	for _, character := range credentials.Password {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return ErrRefused
		}
	}
	if writeHandshake(stream, []byte{5, 1, 2}) != nil {
		return ErrRefused
	}
	var response [2]byte
	if _, err := io.ReadFull(stream, response[:]); err != nil || response != [2]byte{5, 2} {
		return ErrRefused
	}
	frame := []byte{1, byte(len(credentials.Username))}
	frame = append(frame, credentials.Username...)
	frame = append(frame, byte(len(credentials.Password)))
	frame = append(frame, credentials.Password...)
	n, err := stream.Write(frame)
	for i := range frame {
		frame[i] = 0
	}
	if err != nil || n != 2+len(credentials.Username)+1+len(credentials.Password) {
		return ErrRefused
	}
	if _, err := io.ReadFull(stream, response[:]); err != nil || response != [2]byte{1, 0} {
		return ErrRefused
	}
	return nil
}

func writeHandshake(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err != nil || n != len(data) {
		return ErrRefused
	}
	return nil
}
