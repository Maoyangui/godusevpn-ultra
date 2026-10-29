package metadatatool

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Len() int       { return b.buffer.Len() }
func (b *boundedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedBuffer) String() string { return b.buffer.String() }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.exceeded = true
		return remaining, ErrOutputLimit
	}
	return b.buffer.Write(data)
}

type nativeRunner struct{}

func (nativeRunner) run(ctx context.Context, lease backendLease, args []string, input []byte, limit int) ([]byte, error) {
	command := exec.CommandContext(ctx, lease.executable, args...)
	command.Dir = lease.directory
	command.Stdin = bytes.NewReader(input)
	command.Env = backendEnvironment()
	configureProcess(command)
	command.WaitDelay = 2 * time.Second
	output := &boundedBuffer{limit: limit}
	diagnostic := &boundedBuffer{limit: 8192}
	command.Stdout = output
	command.Stderr = diagnostic
	// Raw diagnostics may contain metadata and are intentionally never returned.
	if command.Run() != nil || output.exceeded || diagnostic.Len() != 0 || diagnostic.exceeded || ctx.Err() != nil {
		return nil, ErrBackend
	}
	return append([]byte(nil), output.Bytes()...), nil
}
