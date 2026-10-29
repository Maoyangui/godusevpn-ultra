package main

import (
	"net/http"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

// Every explicit catalog/download/launch gets an isolated transport bound to
// this profile's proxy. The factory performs no request, never uses the default
// transport/environment proxy, and does not retain credentials in a global map.
func newBrowserEngineManager(proxy browserproxy.Config) (*browserengine.Manager, error) {
	transport, err := browserproxy.NewTransport(proxy)
	if err != nil {
		return nil, err
	}
	return browserengine.New(&http.Client{Transport: transport, Timeout: 10 * time.Minute})
}
