//go:build !windows

package localstate

import (
	"errors"
	"os"
	"path/filepath"
)

// Non-Windows policy backends remain unsupported. Preserve existing temporary
// fake-store behavior without importing Windows namespace assumptions.
func AcquireDirectory(dir string, _ bool) (func(), error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("absolute recovery directory required")
	}
	return func() {}, nil
}

func OpenRegular(path string, createLock bool) (*os.File, error) {
	if createLock {
		return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("recovery file is not regular")
	}
	return os.Open(path)
}
