//go:build !windows

package daemon

import (
	"context"
	"os/exec"
)

func diagnosticCommand(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, name, args...), nil
}
