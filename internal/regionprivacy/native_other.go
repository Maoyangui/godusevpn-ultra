//go:build !windows

package regionprivacy

type nativeBackend struct{}

func (nativeBackend) identity() (userIdentity, error)        { return userIdentity{}, ErrUnsupported }
func (nativeBackend) choices() ([]Choice, error)             { return nil, ErrUnsupported }
func (nativeBackend) current() (string, error)               { return "", ErrUnsupported }
func (nativeBackend) set(string) error                       { return ErrUnsupported }
func openNativeStore(userIdentity, storeMode) (store, error) { return nil, ErrUnsupported }
