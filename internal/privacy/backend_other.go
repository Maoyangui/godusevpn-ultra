//go:build !windows

package privacy

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type unsupportedBackend struct{}

func newBackend() backend { return unsupportedBackend{} }
func (unsupportedBackend) environment() environment {
	return environment{Platform: runtime.GOOS, UserKey: "unsupported"}
}
func (unsupportedBackend) read(location) (value, error) {
	return value{}, errors.New("Windows-only policy")
}
func (unsupportedBackend) write(location, value) error { return errors.New("Windows-only policy") }
func secureDirectory(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("absolute directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}
func atomicReplace(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	f, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Only fake-backend tests can write on non-Windows; the public backend refuses
// all mutations. The Windows implementation uses an OS cross-process lock.
var testStoreLock sync.Mutex

func lockStore(dir string) (func(), error) {
	if err := secureDirectory(dir); err != nil {
		return nil, err
	}
	testStoreLock.Lock()
	return testStoreLock.Unlock, nil
}
