//go:build !windows

package browserprofiles

func platformStorage() (storage, error) { return nil, ErrUnsupported }
