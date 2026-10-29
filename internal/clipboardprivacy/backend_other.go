//go:build !windows

package clipboardprivacy

type unsupportedBackend struct{}

func NewNativeBackend() Backend                                   { return unsupportedBackend{} }
func (unsupportedBackend) Supported() bool                        { return false }
func (unsupportedBackend) WriteSecret(string) (Receipt, error)    { return Receipt{}, ErrUnsupported }
func (unsupportedBackend) ClearIfUnchanged(Receipt) (bool, error) { return false, ErrUnsupported }
func (unsupportedBackend) Close() error                           { return nil }
