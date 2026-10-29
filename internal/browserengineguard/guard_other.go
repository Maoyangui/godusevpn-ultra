//go:build !windows || (!amd64 && !arm64)

package browserengineguard

func New(uintptr) (*Manager, error) { return nil, ErrUnsupported }
