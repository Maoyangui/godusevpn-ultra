//go:build windows

package main

import (
	"errors"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

func (a *App) browserEngineDownloadRoute(proxyJSON string) (browserproxy.Config, error) {
	if proxyJSON != "" {
		return browserproxy.Decode(proxyJSON)
	}
	controller, err := a.browserController()
	if err != nil {
		return browserproxy.Config{}, err
	}
	_, port, err := controller.gateway(a.ctx, 0, 0)
	if err != nil {
		return browserproxy.Config{}, errors.New("请连接 Ultra 严格模式，或填写独立代理后再读取官方版本")
	}
	return browserproxy.Config{Type: "socks5", Host: "127.0.0.1", Port: port}, nil
}
