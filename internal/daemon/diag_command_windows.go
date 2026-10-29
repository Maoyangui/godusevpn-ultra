package daemon

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func diagnosticCommand(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	switch name {
	case "route.exe", "ipconfig.exe", "netsh.exe":
	default:
		return nil, errors.New("unsupported diagnostic tool")
	}
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(dir, name), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd, nil
}
