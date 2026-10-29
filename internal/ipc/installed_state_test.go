package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
)

type installedFixturePeer struct {
	fakePeerSession
	foreground bool
	pidCalls   int
	changePID  bool
}

func (p *installedFixturePeer) installedPID() (uint32, error) {
	p.pidCalls++
	if err := p.Validate(); err != nil {
		return 0, err
	}
	if p.foreground {
		return 0, ErrUntrustedService
	}
	if p.changePID && p.pidCalls > 1 {
		return 43, nil
	}
	return 42, nil
}

func TestInstalledStateRejectsPlainForegroundAndUnknownPeersBeforeWrite(t *testing.T) {
	for _, kind := range []string{"plain", "foreground", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			counted := &countingConn{Conn: client}
			connect := func(context.Context) (net.Conn, error) {
				if kind == "plain" {
					return counted, nil
				}
				var peer peerSession = &fakePeerSession{}
				if kind == "foreground" {
					peer = &installedFixturePeer{foreground: true}
				}
				return &authenticatedConn{Conn: counted, peer: peer}, nil
			}
			_, err := readInstalledStateWithDial(context.Background(), connect)
			if !errors.Is(err, ErrUntrustedService) || counted.writes != 0 {
				t.Fatalf("uninstalled peer reached request: writes=%d err=%v", counted.writes, err)
			}
		})
	}
}

func TestInstalledStateBindsResponseAndFreshPIDOnSameAuthenticatedConnection(t *testing.T) {
	for _, kind := range []string{"valid", "wrong-response", "changed-pid", "postcheck-fails", "null-result"} {
		t.Run(kind, func(t *testing.T) {
			peer := &installedFixturePeer{changePID: kind == "changed-pid"}
			if kind == "postcheck-fails" {
				peer.failAt = 3
			}
			connect := func(context.Context) (net.Conn, error) {
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					line, err := bufio.NewReader(server).ReadBytes('\n')
					if err != nil {
						return
					}
					var req Request
					_ = json.Unmarshal(line, &req)
					if req.Method != MGetState || len(req.Params) != 0 {
						t.Error("installed reader sent a non-read-only method")
					}
					id := req.ID
					if kind == "wrong-response" {
						id++
					}
					result := json.RawMessage(`{"protocol":1,"mode":"global"}`)
					if kind == "null-result" {
						result = json.RawMessage(`null`)
					}
					_ = json.NewEncoder(server).Encode(Response{ID: id, OK: true, Result: result})
				}()
				return &authenticatedConn{Conn: client, peer: peer}, nil
			}
			observed, err := readInstalledStateWithDial(context.Background(), connect)
			if kind == "valid" {
				if err != nil || observed.ServicePID != 42 || observed.View.Protocol != 1 || peer.checks != 3 {
					t.Fatalf("missing same-peer evidence: %+v checks=%d err=%v", observed, peer.checks, err)
				}
			} else if err == nil || observed.ServicePID != 0 {
				t.Fatal("uncorrelated/changed peer returned trusted state")
			}
		})
	}
}

func TestInstalledStateCancellationClosesPendingRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peer := &installedFixturePeer{}
	connect := func(context.Context) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_, _ = bufio.NewReader(server).ReadBytes('\n')
			cancel()
			var b [1]byte
			_, _ = server.Read(b[:])
		}()
		return &authenticatedConn{Conn: client, peer: peer}, nil
	}
	if _, err := readInstalledStateWithDial(ctx, connect); err == nil {
		t.Fatal("cancelled read returned trusted evidence")
	}
}
