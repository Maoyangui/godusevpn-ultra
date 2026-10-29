//go:build !windows

package main

func nativeBrowserEngineDependencies(*App) browserEngineDependencies {
	return browserEngineDependencies{}
}
