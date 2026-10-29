//go:build !windows || !(amd64 || arm64)

package locationportalprepare

func NewNativeManager(_ string) (*Manager, error) { return nil, ErrUnsupported }
