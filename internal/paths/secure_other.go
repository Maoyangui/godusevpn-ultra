//go:build !windows

package paths

// PrepareServiceStore preserves the existing non-Windows 0700 layout.
func PrepareServiceStore() (func(), error) {
	if err := ensureDirectories(); err != nil {
		return nil, err
	}
	return func() {}, nil
}
