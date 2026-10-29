//go:build !windows

package browserhost

import "runtime"

// Read returns a truthful unsupported report on non-Windows hosts. The
// Windows client is the only target for this MVP; no platform is inferred from
// the embedded WebView used by another target.
func Read() Report {
	return Report{
		Supported: false,
		Platform:  runtime.GOOS,
		Engines: []Engine{
			{ID: "webview2", Name: "Microsoft Edge WebView2", Kind: "webview", Available: false, Detail: "此构建只在 Windows 检查 WebView2；当前平台未提供内嵌浏览器主机。"},
			{ID: "cef", Name: "CEF / Chromium", Kind: "embedded", Available: false, Detail: "本版本未随包提供 CEF；不会显示虚假的可用开关。"},
		},
		Warnings: []string{"这只是运行时清单，不代表网页指纹、权限或网络已被隐藏。"},
	}
}
