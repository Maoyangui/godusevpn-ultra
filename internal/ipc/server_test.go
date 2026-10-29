package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
)

func TestPipeRoundTrip(t *testing.T) {
	s := NewServer(nil)
	s.Handle("Echo", func(p json.RawMessage) (any, error) {
		in, err := Decode[map[string]string](p)
		if err != nil {
			return nil, err
		}
		return map[string]string{"got": in["msg"]}, nil
	})
	s.Handle("Fail", func(json.RawMessage) (any, error) { return nil, &CallError{Code: "E_X", Msg: "坏了"} })
	s.Handle("Boom", func(json.RawMessage) (any, error) { panic("x") })
	// No real product pipe or service identity is needed for protocol tests.
	closed := false
	connect := func(context.Context) (net.Conn, error) {
		if closed {
			return nil, net.ErrClosed
		}
		client, host := net.Pipe()
		go s.serve(host)
		return client, nil
	}
	call := func(method string, params, out any) error {
		return callWithDial(context.Background(), method, params, out, connect)
	}
	var out map[string]string
	if err := call("Echo", map[string]string{"msg": "你好"}, &out); err != nil {
		t.Fatal(err)
	}
	if out["got"] != "你好" {
		t.Fatalf("回显不对: %v", out)
	}
	err := call("Fail", nil, nil)
	var ce *CallError
	if !errors.As(err, &ce) || ce.Code != "E_X" {
		t.Fatalf("错误码应透传: %v", err)
	}
	if err := call("Boom", nil, nil); err == nil {
		t.Fatal("处理器 panic 应变成错误而不是断连")
	}
	if err := call("Nope", nil, nil); err == nil {
		t.Fatal("未知方法应报错")
	}
	closed = true
	if err := call("Echo", nil, nil); !errors.Is(err, ErrNoService) {
		t.Fatalf("服务关了应报 ErrNoService: %v", err)
	}
}
