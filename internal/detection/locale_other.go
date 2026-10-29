//go:build !windows

package detection

// This release does not claim Windows regional evidence on another platform.
func normalizedLocale(string) string { return "" }
