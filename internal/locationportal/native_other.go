//go:build !windows

package locationportal

func nativeIdentity() (userIdentity, error)                  { return userIdentity{}, ErrUnsupported }
func openNativeStore(userIdentity, storeMode) (store, error) { return nil, ErrUnsupported }
