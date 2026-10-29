package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

// Identity uncertainty is different from an absent service. Callers must not
// treat this error as permission to reinstall, disconnect, or remove protection.
var ErrUntrustedService = errors.New("控制服务身份无法确认；未发送请求，原有网络保护未改变")
var ErrResponseMismatch = errors.New("控制服务响应不匹配；操作结果未知，请重新核对")

type peerSession interface {
	Validate() error
	Close() error
}

type authenticatedConn struct {
	net.Conn
	peer peerSession
	once sync.Once
}

func (c *authenticatedConn) Write(b []byte) (int, error) {
	// Revalidate immediately before sending even the method name. A failed
	// check sends no bytes and does not silently retry a possibly mutating RPC.
	if err := c.peer.Validate(); err != nil {
		_ = c.Close()
		return 0, authError("发送前的服务身份核对失败")
	}
	return c.Conn.Write(b)
}

func (c *authenticatedConn) Close() error {
	var err error
	c.once.Do(func() {
		err = errors.Join(c.Conn.Close(), c.peer.Close())
	})
	return err
}

func authError(detail string) error { return fmt.Errorf("%w：%s", ErrUntrustedService, detail) }

func dialAuthenticated(ctx context.Context, connect func(context.Context) (net.Conn, error), authenticate func(net.Conn) (peerSession, error)) (net.Conn, error) {
	conn, err := connect(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := authenticate(conn)
	if err != nil || peer == nil {
		_ = conn.Close()
		if peer != nil {
			_ = peer.Close()
		}
		return nil, authError("无法取得可信服务实例；没有回退到未认证连接")
	}
	if err := peer.Validate(); err != nil {
		_ = conn.Close()
		_ = peer.Close()
		return nil, authError("服务实例已变化或无法核对")
	}
	return &authenticatedConn{Conn: conn, peer: peer}, nil
}
