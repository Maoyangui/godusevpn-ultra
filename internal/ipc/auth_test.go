package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
)

type fakePeerSession struct{ checks, failAt, closes int }

func (p *fakePeerSession) Validate() error {
	p.checks++
	if p.checks == p.failAt {
		return errors.New("simulated unreadable/changed identity")
	}
	return nil
}
func (p *fakePeerSession) Close() error { p.closes++; return nil }

type countingConn struct {
	net.Conn
	writes, closes int
}

func (c *countingConn) Write(b []byte) (int, error) { c.writes++; return c.Conn.Write(b) }
func (c *countingConn) Close() error                { c.closes++; return c.Conn.Close() }

type observedParams struct{ called *bool }

func (p observedParams) MarshalJSON() ([]byte, error) {
	*p.called = true
	return []byte(`{"fixture":"synthetic-only"}`), nil
}

func TestAuthenticationFailureSendsNothingAndIsNotServiceAbsent(t *testing.T) {
	for _, stage := range []string{"factory", "initial", "before_write"} {
		t.Run(stage, func(t *testing.T) {
			client, host := net.Pipe()
			defer host.Close()
			conn := &countingConn{Conn: client}
			peer := &fakePeerSession{}
			if stage == "initial" {
				peer.failAt = 1
			}
			if stage == "before_write" {
				peer.failAt = 2
			}
			marshaled := false
			connect := func(ctx context.Context) (net.Conn, error) {
				return dialAuthenticated(ctx, func(context.Context) (net.Conn, error) { return conn, nil }, func(net.Conn) (peerSession, error) {
					if stage == "factory" {
						return nil, errors.New("SCM or TOKEN_QUERY denied")
					}
					return peer, nil
				})
			}
			err := callWithDial(context.Background(), MAddProfile, observedParams{&marshaled}, nil, connect)
			if !errors.Is(err, ErrUntrustedService) || errors.Is(err, ErrNoService) || errors.Is(err, ErrNoPermission) {
				t.Fatal("identity uncertainty was hidden", err)
			}
			if conn.writes != 0 || conn.closes != 1 {
				t.Fatalf("request leaked or connection not closed: writes=%d closes=%d", conn.writes, conn.closes)
			}
			if marshaled != (stage == "before_write") {
				t.Fatal("authentication did not precede parameter marshaling")
			}
			if stage != "factory" && peer.closes != 1 {
				t.Fatal("peer handle not closed exactly once")
			}
		})
	}
}

func TestAuthenticatedConnectionRechecksAndClosesPeerOnce(t *testing.T) {
	client, host := net.Pipe()
	defer host.Close()
	peer := &fakePeerSession{}
	conn, err := dialAuthenticated(context.Background(), func(context.Context) (net.Conn, error) { return client, nil }, func(net.Conn) (peerSession, error) { return peer, nil })
	if err != nil {
		t.Fatal(err)
	}
	read := make(chan string, 1)
	go func() { var b [4]byte; n, _ := host.Read(b[:]); read <- string(b[:n]) }()
	if _, err = conn.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	if <-read != "test" || peer.checks != 2 {
		t.Fatal("missing send-time validation")
	}
	_ = conn.Close()
	_ = conn.Close()
	if peer.closes != 1 {
		t.Fatal("peer closed more than once")
	}
}

func TestResponseIdentityCheckedBeforeResultAndRemoteError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wrongID bool
		ok      bool
	}{{"correct", false, true}, {"wrong_success", true, true}, {"wrong_error", true, false}, {"correct_error", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			connect := func(context.Context) (net.Conn, error) {
				client, host := net.Pipe()
				go func() {
					defer host.Close()
					line, _ := bufio.NewReader(host).ReadBytes('\n')
					var req Request
					_ = json.Unmarshal(line, &req)
					id := req.ID
					if tc.wrongID {
						id++
					}
					_ = json.NewEncoder(host).Encode(Response{ID: id, OK: tc.ok, Result: json.RawMessage(`"fixture-result"`), Code: "FIXTURE", Error: "synthetic error"})
				}()
				return client, nil
			}
			out := "unchanged"
			err := callWithDial(context.Background(), "Fixture", nil, &out, connect)
			if tc.wrongID {
				if !errors.Is(err, ErrResponseMismatch) || out != "unchanged" {
					t.Fatal("wrong response was accepted", err, out)
				}
				return
			}
			if tc.ok {
				if err != nil || out != "fixture-result" {
					t.Fatal(err, out)
				}
				return
			}
			var remote *CallError
			if !errors.As(err, &remote) || remote.Code != "FIXTURE" {
				t.Fatal("valid remote error lost", err)
			}
		})
	}
}
