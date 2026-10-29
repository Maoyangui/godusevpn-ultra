//go:build !windows

package main

import "github.com/Maoyangui/godusevpn/internal/browserproxy"

func (a *App) browserEngineDownloadRoute(string) (browserproxy.Config, error) {
	return browserproxy.Config{}, browserproxy.ErrProbe
}
