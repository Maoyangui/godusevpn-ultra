//go:build !windows

package metadatatool

import "os/exec"

type nativeFiles struct{}

func (nativeFiles) openInput(string) (inputLease, error) { return nil, ErrUnavailable }
func (nativeFiles) saveNew(string, string, []byte) error { return ErrUnavailable }
func acquireInstalled() (backendLease, error)            { return backendLease{}, ErrUnavailable }
func backendEnvironment() []string                       { return []string{} }
func configureProcess(*exec.Cmd)                         {}
