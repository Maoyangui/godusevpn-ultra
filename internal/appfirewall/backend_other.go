//go:build !windows || (!amd64 && !arm64)

package appfirewall

import "os"

type unsupportedBackend struct{}

func newBackend() backend                                           { return unsupportedBackend{} }
func (unsupportedBackend) supported() bool                          { return false }
func (unsupportedBackend) elevated() bool                           { return false }
func (unsupportedBackend) snapshot() ([]installed, []string, error) { return nil, nil, ErrUnsupported }
func (unsupportedBackend) replace(Rule) error                       { return ErrUnsupported }
func (unsupportedBackend) remove(string) error                      { return ErrUnsupported }
func systemDirectory() string                                       { return "" }
func isReparsePoint(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
