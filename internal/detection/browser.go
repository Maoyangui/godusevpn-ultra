package detection

import (
	"fmt"
	"time"
)

// OwnedBrowser is a bounded in-memory controller observation. It deliberately
// has no session/profile identity, UA, GPU strings, URL, proxy or raw error.
// Ready and graphics records are not a new browser or traffic measurement.
type OwnedBrowser struct {
	Tracked, Ready, CanNavigate bool
	Engine, State               string
	ReadAt                      time.Time
	NativeGraphicsRecord        bool
	GraphicsWindowScopes        int
	GraphicsContexts            int
	OffscreenAbsentScopes       int
}

// AppendOwnedBrowser leaves findings for external browsers intact. It records
// only this UI's controller, including failed and unresolved cleanup states.
func AppendOwnedBrowser(r *Report, in OwnedBrowser) {
	if r == nil {
		return
	}
	summary := "本界面尚未跟踪独立 Chrome / Firefox 会话。"
	evidence := "只读本界面会话管理器的内存状态；没有启动浏览器、请求页面、查询出口或读取网站数据。"
	status := "observed"
	engine := map[string]string{"chrome-for-testing": "Chrome", "firefox": "Firefox"}[in.Engine]
	if in.Tracked {
		switch in.State {
		case "starting":
			summary = "浏览器正在准备，尚未开放导航。"
		case "cancelling":
			summary = "已请求取消启动，仍在等待后台收尾。"
		case "ready":
			if engine != "" && in.Ready && in.CanNavigate {
				summary = engine + " 会话管理器报告已就绪；本次检测没有重测当前页面。"
			} else {
				status, summary = "unknown", "会话状态不完整，当前不能确认浏览器已就绪。"
			}
		case "start_failed":
			summary = "上次启动未完成；请在浏览器页查看失败原因。"
		case "start_cancelled":
			summary = "上次启动已取消；后台报告本次收尾完成。"
		case "closed":
			summary = "会话管理器报告本次会话已关闭。"
		case "closing", "closing_or_unknown", "closed_filters_unknown":
			status, summary = "unknown", "浏览器收尾尚未确认；请先检查会话，暂勿重复启动。"
		default:
			status, summary = "unknown", "会话管理状态未识别，请在浏览器页重新检查。"
		}
		if in.State != "ready" && (in.Ready || in.CanNavigate) {
			status, summary = "unknown", "会话状态相互矛盾，当前不能确认启动或清理结果。"
		}
	}
	if status == "observed" && in.Tracked && in.State == "ready" && in.NativeGraphicsRecord && validGraphicsRecord(in) {
		evidence += fmt.Sprintf(" 本次启动保留了 %d 个 Window 范围、%d 个 WebGL 上下文的原生图形读回记录；其中 %d 个范围未提供 Offscreen API。WebGPU 的原生偏好禁用读回仅限启动检查范围；未重测当前站点，未验证 Worker API。", in.GraphicsWindowScopes, in.GraphicsContexts, in.OffscreenAbsentScopes)
	}
	at := ""
	if !in.ReadAt.IsZero() {
		at = in.ReadAt.UTC().Format(time.RFC3339Nano)
	}
	r.Findings = append(r.Findings, Finding{
		ID: "owned-browser-session", Title: "专用浏览器会话", Category: "专用浏览器", Audience: "app",
		Status: status, Summary: summary, Evidence: evidence,
		Scope:       "仅本界面管理的独立 Chrome / Firefox；不含 WebView2、其他应用或浏览器。无活跃会话不证明系统没有遗留进程或规则；管理状态与启动记录不是当前网站或实际流量实测。",
		ActionLabel: "查看浏览器会话", SettingsID: "browser", Source: "browser-session", DetectedAt: at,
	})
}

func validGraphicsRecord(in OwnedBrowser) bool {
	if in.Engine != "firefox" || in.GraphicsWindowScopes != 4 || in.OffscreenAbsentScopes < 0 || in.OffscreenAbsentScopes > 4 {
		return false
	}
	// Zero contexts is a WebGPU-only record, not a WebGL metadata match.
	if in.GraphicsContexts == 0 {
		return in.OffscreenAbsentScopes == 0
	}
	return in.GraphicsContexts == 16-2*in.OffscreenAbsentScopes
}
