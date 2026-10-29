//go:build windows

package browserhost

import (
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	webView2Client = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	edgeClient     = `{56EB18F8-B008-4CBD-B6D2-8C97FE7E9062}`
	chromeClient   = `{8A69D345-D564-463C-AFF1-A69D9E530F96}`
)

type clientRead struct {
	version string
	name    string
}

func readClient(root registry.Key, path, fallback string, view uint32) (clientRead, bool) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|view)
	if err != nil {
		return clientRead{}, false
	}
	defer k.Close()
	version, _, _ := k.GetStringValue("pv")
	name, _, _ := k.GetStringValue("name")
	if strings.TrimSpace(name) == "" {
		name = fallback
	}
	return clientRead{version: strings.TrimSpace(version), name: strings.TrimSpace(name)}, true
}

func detectClient(id, name, resultID, kind string, roots []registry.Key) Engine {
	for _, root := range roots {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			if value, ok := readClient(root, `SOFTWARE\Microsoft\EdgeUpdate\Clients\`+id, name, view); ok {
				detail := "已发现本机运行时；本报告不启动浏览器或修改配置。"
				if value.version == "" {
					detail = "已发现本机运行时，但注册表没有提供版本；请在系统设置中核对。"
				}
				return Engine{ID: resultID, Name: value.name, Kind: kind, Version: value.version, Available: true, Detail: detail}
			}
		}
	}
	return Engine{ID: resultID, Name: name, Kind: kind, Available: false, Detail: "未发现本机运行时；Ultra 不会下载或自动安装它。"}
}

// Read performs a read-only registry inventory. WebView2 and the regular
// browsers are listed separately because the Wails UI WebView is not a
// browser profile and cannot be used as evidence for Chrome/Edge behavior.
func Read() Report {
	roots := []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE}
	webview := detectClient(webView2Client, "Microsoft Edge WebView2", "webview", "webview", roots)
	edge := detectClient(edgeClient, "Microsoft Edge", "edge", "browser", roots)
	chrome := detectClient(chromeClient, "Google Chrome", "chrome", "browser", roots)
	return Report{
		Supported: true,
		Platform:  runtime.GOOS,
		Engines:   []Engine{webview, edge, chrome, {ID: "cef", Name: "CEF / Chromium", Kind: "embedded", Available: false, Detail: "本版本未随包提供 CEF；需独立构建、沙箱、签名和升级验收后才会加入。"}},
		Recommended: func() string {
			if webview.Available {
				return webview.ID
			}
			return ""
		}(),
		Warnings: []string{
			"运行时清单不会修改浏览器设置，也不能证明网页指纹、权限、WebRTC 或代理已受保护。",
			"内嵌 WebView 与 Chrome/Edge 用户配置分开；浏览器策略需在对应配置中重新检查。",
		},
	}
}
