//go:build !windows || (!amd64 && !arm64)

package locationportalgate

func newNativeGate() (*Gate, error) { return nil, ErrUnavailable }
