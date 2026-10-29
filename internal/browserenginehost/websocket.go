package browserenginehost

import (
	"context"
	"sync"

	"github.com/coder/websocket"
)

// Firefox uses actual WebDriver BiDi text WebSocket messages. The owning
// Windows connector checks the numeric loopback peer before the HTTP upgrade.
type websocketTransport struct {
	connection *websocket.Conn
	release    func()
	once       sync.Once
}

func (t *websocketTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	kind, data, err := t.connection.Read(ctx)
	if err != nil || kind != websocket.MessageText || len(data) == 0 || len(data) > maxProtocolMessage {
		return nil, ErrRefused
	}
	return data, nil
}
func (t *websocketTransport) WriteMessage(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > maxProtocolMessage || t.connection.Write(ctx, websocket.MessageText, data) != nil {
		return ErrRefused
	}
	return nil
}
func (t *websocketTransport) Close() error {
	t.once.Do(func() {
		t.connection.CloseNow()
		if t.release != nil {
			t.release()
		}
	})
	return nil
}
