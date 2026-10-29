//go:build !windows

package timeprivacy

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type unsupportedBackend struct{}

func newBackend() backend                             { return unsupportedBackend{} }
func (unsupportedBackend) supported() bool            { return false }
func (unsupportedBackend) choices() ([]Choice, error) { return nil, errors.New("Windows only") }
func (unsupportedBackend) current() (zoneState, error) {
	return zoneState{}, errors.New("Windows only")
}
func (unsupportedBackend) set(zoneState) error { return errors.New("Windows only") }
func secureDirectory(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("absolute directory required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}
func replace(from, to string) error {
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

var testStoreLock sync.Mutex

func lockStore(dir string) (func(), error) {
	if err := secureDirectory(dir); err != nil {
		return nil, err
	}
	testStoreLock.Lock()
	return testStoreLock.Unlock, nil
}
