package browserenginehost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryTransport struct {
	reads, writes chan []byte
	done          chan struct{}
	once          sync.Once
}

func newMemoryTransport() *memoryTransport {
	return &memoryTransport{make(chan []byte, 16), make(chan []byte, 16), make(chan struct{}), sync.Once{}}
}
func (m *memoryTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case value := <-m.reads:
		return value, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.done:
		return nil, io.EOF
	}
}
func (m *memoryTransport) WriteMessage(ctx context.Context, value []byte) error {
	select {
	case m.writes <- append([]byte(nil), value...):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return io.ErrClosedPipe
	}
}
func (m *memoryTransport) Close() error { m.once.Do(func() { close(m.done) }); return nil }
func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("signal timeout")
	}
}

func TestProtocolFramingAndStrictEnvelope(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `{"id":1,"id":2}`, `{"id":1,"ID":2}`, `{"sessionID":"x"}`, `{"result":{"x":1,"x":2}}`, `{} {}`} {
		if uniqueProtocolJSON([]byte(raw)) == nil {
			t.Fatalf("accepted malformed envelope %q", raw)
		}
	}
	if uniqueProtocolJSON([]byte(`{"method":"x","params":{"headers":{"A":"x","a":"y"}}}`)) != nil {
		t.Fatal("nested case-sensitive keys were conflated")
	}
	input := strings.Repeat("x", 70000)
	stream := &nulTransport{reader: bufio.NewReaderSize(strings.NewReader(input+"\x00{}\x00"), 31)}
	got, err := stream.ReadMessage(context.Background())
	if err != nil || string(got) != input {
		t.Fatal("fragmented ASCIIZ read failed")
	}
	got, err = stream.ReadMessage(context.Background())
	if err != nil || string(got) != "{}" {
		t.Fatal("next ASCIIZ message lost")
	}
	for _, raw := range []string{"\x00", "{}", strings.Repeat("x", maxProtocolMessage+1) + "\x00"} {
		stream := &nulTransport{reader: bufio.NewReader(strings.NewReader(raw))}
		if _, err := stream.ReadMessage(context.Background()); err == nil {
			t.Fatal("accepted empty/truncated/oversize message")
		}
	}
	var output bytes.Buffer
	writer := &nulTransport{writer: &output}
	if writer.WriteMessage(context.Background(), []byte(`{"id":1}`)) != nil || output.String() != "{\"id\":1}\x00" {
		t.Fatal("wrong ASCIIZ write")
	}
	if writer.WriteMessage(context.Background(), []byte("x\x00y")) == nil {
		t.Fatal("accepted embedded delimiter")
	}
	writer.writer = shortWriter{}
	if writer.WriteMessage(context.Background(), []byte("{}")) == nil {
		t.Fatal("accepted zero-progress writer")
	}
}

type shortWriter struct{}

func (shortWriter) Write([]byte) (int, error) { return 0, nil }

func TestProtocolReplyAndPausedEventDispatch(t *testing.T) {
	m := newMemoryTransport()
	var client *protocolClient
	eventDone := make(chan struct{})
	client = newProtocol(m, nil, func(event protocolEvent) error {
		if event.Method != "Target.attachedToTarget" || event.SessionID != "s" {
			return errors.New("bad event")
		}
		_, err := client.call(context.Background(), "Runtime.runIfWaitingForDebugger", map[string]any{}, "s")
		close(eventDone)
		return err
	})
	defer client.Close()
	m.reads <- []byte(`{"method":"Target.attachedToTarget","sessionId":"s","params":{"waitingForDebugger":true}}`)
	select {
	case raw := <-m.writes:
		var message map[string]any
		if json.Unmarshal(raw, &message) != nil || message["sessionId"] != "s" || message["id"] != float64(1) {
			t.Fatal("bad outgoing command")
		}
	case <-time.After(time.Second):
		t.Fatal("event handler blocked reader")
	}
	m.reads <- []byte(`{"id":1,"result":{}}`)
	awaitSignal(t, eventDone)
}
func TestProtocolFailureRevokesBeforeTransportClose(t *testing.T) {
	for _, raw := range []string{`{"id":99,"result":{}}`, `{"id":1,"ID":2,"result":{}}`, `{"method":"x","type":"success","params":{}}`} {
		m := newMemoryTransport()
		var aborted atomic.Bool
		client := newProtocol(m, func() {
			select {
			case <-m.done:
				t.Error("transport closed before revocation")
			default:
			}
			aborted.Store(true)
		}, nil)
		m.reads <- []byte(raw)
		awaitSignal(t, client.done)
		if !aborted.Load() {
			t.Fatal("missing revoke")
		}
	}
	m := newMemoryTransport()
	var aborted atomic.Bool
	client := newProtocol(m, func() { aborted.Store(true) }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _, _ = client.call(ctx, "mutate", map[string]any{}, "") }()
	<-m.writes
	cancel()
	awaitSignal(t, done)
	awaitSignal(t, client.done)
	if !aborted.Load() {
		t.Fatal("unknown completion kept session live")
	}
}
