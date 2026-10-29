//go:build !windows && !linux

package trial

func platformDeviceID() string { return "" }
