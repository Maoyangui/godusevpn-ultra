package main

import (
	"context"
	"runtime"

	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

var profileProxyProber = browserproxy.NewProber()

// BrowserProxyTest is an explicit user-requested probe. It never saves proxy
// secrets or changes the VPN/OS/browser; the result covers this probe only.
func (a *App) BrowserProxyTest(configJSON string) (browserproxy.Result, error) {
	if runtime.GOOS != "windows" {
		return browserproxy.Result{}, browserproxy.ErrProbe
	}
	config, err := browserproxy.Decode(configJSON)
	if err != nil {
		return browserproxy.Result{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return profileProxyProber.Probe(ctx, config)
}
