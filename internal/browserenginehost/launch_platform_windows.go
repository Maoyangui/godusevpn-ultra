//go:build windows && (amd64 || arm64)

package browserenginehost

func nativeLaunchPlatformEnabled() bool { return true }
