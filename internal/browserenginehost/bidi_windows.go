//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"github.com/coder/websocket"
	"golang.org/x/sys/windows"
)

func parseBiDiServer(data []byte, expectedPort int) error {
	if len(data) == 0 || len(data) > 1024 || uniqueProtocolJSON(data) != nil {
		return ErrRefused
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&fields) != nil || decoder.Decode(new(any)) != io.EOF || len(fields) != 2 {
		return ErrRefused
	}
	var host string
	var port int
	if json.Unmarshal(fields["ws_host"], &host) != nil || json.Unmarshal(fields["ws_port"], &port) != nil || port != expectedPort || expectedPort < 1 || expectedPort > 65535 || host != "127.0.0.1" && host != "localhost" {
		return ErrRefused
	}
	return nil
}
func readBiDiServer(profile string, port int) error {
	file, err := localstate.OpenRegular(filepath.Join(profile, "WebDriverBiDiServer.json"), false)
	if err != nil {
		return ErrRefused
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info) != nil || info.NumberOfLinks != 1 {
		return ErrRefused
	}
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil {
		return ErrRefused
	}
	return parseBiDiServer(data, port)
}
func connectBiDi(ctx context.Context, profile string, port int, owner *processAuthorizer) (protocolTransport, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for readBiDiServer(profile, port) != nil {
		select {
		case <-ctx.Done():
			return nil, ErrRefused
		case <-time.After(20 * time.Millisecond):
		}
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var used atomic.Bool
	var connection net.Conn
	var pin io.Closer
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: func(ctx context.Context, network, target string) (net.Conn, error) {
		if network != "tcp" && network != "tcp4" || target != address || used.Swap(true) {
			return nil, ErrRefused
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp4", address)
		if err != nil {
			return nil, ErrRefused
		}
		lease, err := owner.authorize(conn)
		if err != nil {
			conn.Close()
			return nil, ErrRefused
		}
		connection, pin = conn, lease
		return conn, nil
	}}
	release := func() {
		transport.CloseIdleConnections()
		if connection != nil {
			connection.Close()
		}
		if pin != nil {
			pin.Close()
		}
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ws, response, err := websocket.Dial(ctx, "ws://"+address+"/session", &websocket.DialOptions{HTTPClient: client, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		release()
		return nil, ErrRefused
	}
	check, err := owner.authorize(connection)
	if err != nil {
		ws.CloseNow()
		release()
		return nil, ErrRefused
	}
	check.Close()
	ws.SetReadLimit(maxProtocolMessage)
	return &websocketTransport{connection: ws, release: release}, nil
}
