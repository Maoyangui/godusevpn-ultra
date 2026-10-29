package browserenginehost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

const maxProtocolMessage = 4 << 20

type protocolTransport interface {
	ReadMessage(context.Context) ([]byte, error)
	WriteMessage(context.Context, []byte) error
	Close() error
}
type protocolEvent struct {
	Method    string
	SessionID string
	Params    json.RawMessage
}
type protocolReply struct {
	value json.RawMessage
	err   error
}
type protocolClient struct {
	transport protocolTransport
	mu        sync.Mutex
	next      uint64
	pending   map[uint64]chan protocolReply
	closed    bool
	once      sync.Once
	abort     func()
	events    func(protocolEvent) error
	slots     chan struct{}
	done      chan struct{}
}

func newProtocol(transport protocolTransport, abort func(), events func(protocolEvent) error) *protocolClient {
	client := &protocolClient{transport: transport, pending: map[uint64]chan protocolReply{}, abort: abort, events: events, slots: make(chan struct{}, 32), done: make(chan struct{})}
	go client.read()
	return client
}
func (p *protocolClient) call(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p.mu.Lock()
	if p.closed || p.next >= 1<<52 {
		p.mu.Unlock()
		return nil, ErrRefused
	}
	p.next++
	id := p.next
	reply := make(chan protocolReply, 1)
	p.pending[id] = reply
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	message := map[string]any{"id": id, "method": method, "params": params}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	raw, err := json.Marshal(message)
	if err != nil || len(raw) > maxProtocolMessage {
		p.fail()
		return nil, ErrRefused
	}
	if p.transport.WriteMessage(ctx, raw) != nil {
		p.fail()
		return nil, ErrRefused
	}
	select {
	case result := <-reply:
		return result.value, result.err
	case <-ctx.Done():
		// A timed-out mutating command has unknown completion. Revoke this
		// session rather than accepting later commands against unknown state.
		p.fail()
		return nil, ErrRefused
	case <-p.done:
		return nil, ErrRefused
	}
}
func (p *protocolClient) read() {
	for {
		raw, err := p.transport.ReadMessage(context.Background())
		if err != nil || len(raw) == 0 || len(raw) > maxProtocolMessage {
			p.fail()
			return
		}
		var message struct {
			ID        uint64          `json:"id"`
			Method    string          `json:"method"`
			SessionID string          `json:"sessionId"`
			Params    json.RawMessage `json:"params"`
			Result    json.RawMessage `json:"result"`
			Error     json.RawMessage `json:"error"`
			Type      string          `json:"type"`
		}
		if uniqueProtocolJSON(raw) != nil || json.Unmarshal(raw, &message) != nil {
			p.fail()
			return
		}
		if message.ID != 0 {
			if message.Method != "" || message.Type != "" && message.Type != "success" && message.Type != "error" {
				p.fail()
				return
			}
			p.mu.Lock()
			reply := p.pending[message.ID]
			future := message.ID > p.next
			p.mu.Unlock()
			if future {
				p.fail()
				return
			}
			if reply == nil {
				continue
			}
			result := protocolReply{value: append(json.RawMessage(nil), message.Result...)}
			if len(message.Error) != 0 || message.Type == "error" || len(message.Result) == 0 {
				result.err = ErrRefused
			}
			select {
			case reply <- result:
			default:
				p.fail()
				return
			}
			continue
		}
		if message.Method == "" || len(message.Params) == 0 || message.Type != "" && message.Type != "event" {
			p.fail()
			return
		}
		if p.events == nil {
			continue
		}
		select {
		case p.slots <- struct{}{}:
			event := protocolEvent{message.Method, message.SessionID, append(json.RawMessage(nil), message.Params...)}
			go func() {
				defer func() { <-p.slots }()
				if p.events(event) != nil {
					p.fail()
				}
			}()
		default:
			p.fail()
			return
		}
	}
}
func (p *protocolClient) fail() {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		if p.abort != nil {
			p.abort()
		}
		p.transport.Close()
		close(p.done)
	})
}
func (p *protocolClient) Close() { p.fail() }

// Private Chromium pipe framing is ASCIIZ JSON, not length-prefixed Marionette
// or WebSocket BiDi. No message bytes are returned in diagnostics.
type nulTransport struct {
	reader *bufio.Reader
	writer io.Writer
	closer func() error
	mu     sync.Mutex
}

func (t *nulTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	var result []byte
	for {
		if ctx.Err() != nil {
			return nil, ErrRefused
		}
		chunk, err := t.reader.ReadSlice(0)
		if len(result)+len(chunk) > maxProtocolMessage+1 {
			return nil, ErrRefused
		}
		result = append(result, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil || len(result) < 2 || result[len(result)-1] != 0 {
			return nil, ErrRefused
		}
		return result[:len(result)-1], nil
	}
}
func (t *nulTransport) WriteMessage(ctx context.Context, raw []byte) error {
	if ctx.Err() != nil || len(raw) == 0 || len(raw) > maxProtocolMessage || bytes.IndexByte(raw, 0) >= 0 {
		return ErrRefused
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	data := append(append([]byte(nil), raw...), 0)
	for len(data) > 0 {
		n, err := t.writer.Write(data)
		if err != nil || n <= 0 {
			return ErrRefused
		}
		data = data[n:]
	}
	return nil
}
func (t *nulTransport) Close() error {
	if t.closer != nil {
		return t.closer()
	}
	return nil
}
func uniqueProtocolJSON(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return ErrRefused
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return ErrRefused
		}
		token, err := decoder.Token()
		if err != nil {
			return ErrRefused
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					key, err := decoder.Token()
					name, ok := key.(string)
					if err != nil || !ok || seen[name] {
						return ErrRefused
					}
					if depth == 0 {
						canonical := map[string]string{"id": "id", "method": "method", "sessionid": "sessionId", "params": "params", "result": "result", "error": "error", "type": "type"}
						if want, exists := canonical[strings.ToLower(name)]; exists && name != want {
							return ErrRefused
						}
					}
					seen[name] = true
					if walk(depth+1) != nil {
						return ErrRefused
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim('}') {
					return ErrRefused
				}
			case '[':
				for decoder.More() {
					if walk(depth+1) != nil {
						return ErrRefused
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim(']') {
					return ErrRefused
				}
			default:
				return ErrRefused
			}
		}
		return nil
	}
	if walk(0) != nil {
		return ErrRefused
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrRefused
	}
	return nil
}
