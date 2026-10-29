//go:build !windows

package browserengine

func lockStoreFile(string, string, bool) (func(), error) { return nil, ErrUnsupported }
