// Package detection turns bounded local evidence into exposure findings. It
// cannot initiate network tests, change policy, or influence tunnel decisions.
package detection

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

type Finding struct {
	Count       *int   `json:"count,omitempty"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Audience    string `json:"audience"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	Evidence    string `json:"evidence"`
	Scope       string `json:"scope"`
	ActionLabel string `json:"actionLabel,omitempty"`
	SettingsID  string `json:"settingsId,omitempty"`
	ControlID   string `json:"controlId,omitempty"`
	DetectedAt  string `json:"detectedAt"`
	Source      string `json:"source,omitempty"`
}

type Network struct {
	Available                 bool
	Status, Mode, Guard       string
	GuardError                bool
	IPv6, LANBypass, NoDirect bool
	// Only bounded categories from the authenticated settings snapshot belong
	// here. Never pass resolver addresses, profiles, passwords, or raw settings.
	SettingsAvailable                     bool
	LocalDNSMode, RemoteDNSMode, LogLevel string
	LogDays                               *int
}

type Facts struct {
	Platform                     string
	Elevated                     bool
	TimeZone                     string
	Locale                       string
	Location, Camera, Microphone string
	// Notifications and Clipboard are bounded ConsentStore observations. They
	// describe the current account's default record only; they are not a
	// per-app allow/deny decision and never become a global protection claim.
	Notifications, Clipboard string
	Warnings                 []string
}

type Input struct {
	SourceTimes      map[string]string
	ExecutionMode    string
	HelperAuthorized bool
	At               time.Time
	Policies         privacy.Report
	Firewall         appfirewall.Report
	Network          Network
	Facts            Facts
}

type Report struct {
	ExecutionMode    string    `json:"executionMode"`
	GeneratedAt      string    `json:"generatedAt"`
	Platform         string    `json:"platform"`
	Admin            bool      `json:"admin"`
	HelperAuthorized bool      `json:"helperAuthorized"`
	Observer         string    `json:"observer"`
	Warnings         []string  `json:"warnings"`
	Findings         []Finding `json:"findings"`
}

func Build(in Input) Report {
	if in.At.IsZero() {
		in.At = time.Now()
	}
	at := in.At.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339)
	observer := "普通用户客户端；不是对每个应用的实际读取审计"
	if in.Facts.Elevated {
		observer = "管理员观察进程；其可读结果不能代表普通应用也能读取"
	}
	mode := in.ExecutionMode
	if mode != "desktop_ui" && mode != "direct_cli" {
		mode = "unknown"
	}
	if mode == "direct_cli" && in.Facts.Elevated {
		observer = "直接提升的 CLI 观察进程；没有据此确认助手授权或普通应用权限"
	}
	r := Report{ExecutionMode: mode, GeneratedAt: at, Platform: in.Facts.Platform, Admin: in.Facts.Elevated, HelperAuthorized: mode == "desktop_ui" && in.HelperAuthorized, Observer: observer, Findings: []Finding{}, Warnings: []string{
		"本报告默认只读、本地，不进行外网、STUN 或在线指纹检测。生成时间不是旧网络状态的独立核验时间。",
		"策略配置、过滤器存在和接口实测是不同证据；未知不表示已保护，也不代表某个程序已经读取。",
		"未连接用户实际浏览器。桌面 WebView 与扩展自身来源的结果不能证明任意网站、隐身窗口或其他配置受到保护。",
	}}
	allWarnings := append([]string{}, in.Facts.Warnings...)
	allWarnings = append(allWarnings, in.Policies.Warnings...)
	allWarnings = append(allWarnings, in.Firewall.Warnings...)
	for _, w := range allWarnings {
		w = redactLocalPaths(w)
		found := false
		for _, old := range r.Warnings {
			if old == w {
				found = true
				break
			}
		}
		if !found {
			r.Warnings = append(r.Warnings, w)
		}
	}
	add := func(id, title, category, audience, status, summary, evidence, scope, settingsID, controlID string) {
		f := Finding{ID: id, Title: redactLocalPaths(title), Category: category, Audience: audience, Status: status, Summary: redactLocalPaths(summary), Evidence: redactLocalPaths(evidence), Scope: redactLocalPaths(scope), SettingsID: settingsID, ControlID: controlID, DetectedAt: at}
		if controlID != "" {
			f.ActionLabel = "查看可用保护"
		} else if settingsID != "" {
			f.ActionLabel = "打开相关设置"
		}
		r.Findings = append(r.Findings, f)
	}
	for _, p := range in.Policies.Controls {
		// Capability guidance is not an executable system policy. Its limits are
		// described by dedicated findings below; do not attach a fake Apply action.
		if p.Category != "browser" && p.Category != "windows" {
			continue
		}
		audience, category := "app", "系统隐私"
		if p.Category == "browser" {
			audience, category = "website", "浏览器策略"
		}
		status, summary := "possible", "没有确认这项限制已配置；应用或网站仍可能获得相关信息。"
		switch p.Status {
		case "configured":
			status, summary = "unknown", "目标策略值已配置，仍需在实际浏览器或受覆盖应用中确认；不是实际访问阻止证明。"
		case "policy_conflict", "external_change", "recovery_pending":
			status, summary = "unknown", "配置存在冲突或待恢复状态，当前保护范围不能确认。"
		case "unsupported", "unknown", "requires_admin", "error":
			status, summary = "unknown", "本次无法确认该设置的完整状态；查看适用条件或手动检查。"
		}
		if p.ReadStatus == "match" && p.Status == "manual" {
			status, summary = "unknown", "已只读取得目标策略值；当前写入权限或版本适用性需检查，不能据此证明运行时已保护。"
		}
		add("policy-"+p.ID, p.Title, category, audience, status, summary, p.Detail, p.Scope+"；只核对固定策略，不枚举站点例外和实际读取行为", p.SettingsID, p.ID)
	}
	n := in.Network
	status, summary := "unknown", "尚未取得隧道服务状态，无法确认网络路径。"
	if n.Available {
		summary = fmt.Sprintf("服务报告状态：%s，模式：%s。没有发起新的公网出口测试。", n.Status, n.Mode)
		if n.Status != "connected" {
			status = "possible"
			summary += "服务可达不等于隧道已连接；当前快照没有完整连接状态证据。防直连闸也不能替代可用隧道。"
		}
		if n.Mode != "global" || !n.NoDirect || n.LANBypass {
			status = "possible"
			summary += "分流、直连或局域网例外可能允许部分直接访问。"
		}
		if n.GuardError {
			status = "possible"
			summary += "服务报告防直连相关异常，需检查原有网络保护。"
		}
		if n.Guard != "on" {
			status = "possible"
			summary += "本次快照没有防直连闸已开启的证据；仅有全局模式或禁止直连设置不能证明它已安装。"
		}
	}
	add("network-path", "隧道与直连范围", "网络路径", "network", status, summary, "读取现有客户端状态；未查询第三方 IP 服务", "原有服务状态快照；不等于所有应用和转发路径实测", "", "")
	ipv6Summary := "没有可用的服务设置快照，当前 IPv6 配置未知。"
	if n.Available {
		ipv6Summary = map[bool]string{true: "当前设置允许 IPv6；具体出口和本机地址暴露尚未核验。", false: "当前设置限制 IPv6，但没有枚举物理网卡地址或独立核验所有读取路径。"}[n.IPv6]
	}
	add("network-ipv6", "IPv6 信息与连接", "网络路径", "network", "unknown", ipv6Summary, "只使用可取得的服务设置意图；未收集真实地址", "网络包限制与原生应用读取网卡配置是不同问题", "", "")
	firewallSummary := "本次没有取得可列出的应用规则；可能没有规则，也可能未能完整读取，不能据此证明不存在规则或已隔离。"
	if len(in.Firewall.Rules) > 0 {
		firewallSummary = fmt.Sprintf("本次返回 %d 项本产品应用规则；不证明枚举完整，也不能保证回环、代理重发或与严格隧道闸的仲裁结果。", len(in.Firewall.Rules))
	}
	if !in.Firewall.Supported {
		firewallSummary = "当前检查环境未提供支持的 WFP 读取结果；规则和实际流量范围未知。"
	}
	add("network-firewall", "应用规则与代理转发", "应用联网", "network", "unknown", firewallSummary, "只使用本次可取得的独立 WFP 安装证据；未执行真实流量测试", "只限所选可执行文件和实际安装条件；不改变隧道", "firewall", "")
	if in.Firewall.UnknownFilters > 0 {
		add("network-firewall-unknown", "无法识别的本产品过滤器", "应用联网", "network", "unknown", fmt.Sprintf("本次枚举有 %d 个无法识别的本产品过滤器；归属与完整清理未确认。", in.Firewall.UnknownFilters), "后端独立数量，不由警告文字推断；不会自动扩大删除范围", "本次可枚举的本产品 provider 对象；不代表全系统过滤器完整清单", "firewall", "")
		count := in.Firewall.UnknownFilters
		r.Findings[len(r.Findings)-1].Count = &count
	}
	for i, rule := range in.Firewall.Rules {
		// Rule IDs can be user-supplied labels. They are unnecessary for these
		// non-actionable findings, so export a report-local ordinal instead.
		add(fmt.Sprintf("firewall-rule-%d", i+1), "指定程序网络规则", "应用联网", "app", "unknown", "规则状态："+rule.State+"。过滤器安装不等于所有路径已阻断。", rule.Detail, "所选程序；不在报告中导出程序路径或自定义规则标识", "", "")
	}
	if in.Facts.TimeZone != "" {
		add("timezone", "当前系统时区", "环境信息", "app", "observed", "本观察进程可读取时区："+in.Facts.TimeZone, "当前系统时间接口；未按出口修改", "系统进程环境；浏览器可能使用独立覆盖", "time", "")
	} else {
		add("timezone", "当前系统时区", "环境信息", "app", "unknown", "本次未读取时区；不表示其他应用无法读取。", "本地接口不可用", "当前观察进程", "time", "")
	}
	if locale := normalizedLocale(in.Facts.Locale); locale != "" {
		add("locale", "当前用户地区格式", "环境信息", "app", "observed", "可读取地区格式："+locale, "当前用户的有效本地配置；不读取输入内容", "地区格式不等于浏览器语言或全部系统语言", "language", "")
	} else {
		add("locale", "当前用户地区格式", "环境信息", "app", "unknown", "本次未取得有效地区格式；不表示其他应用无法读取。", "本地配置未读取或校验未通过", "当前观察账户；不代表浏览器语言", "language", "")
	}
	for _, item := range []struct{ id, title, value, settingsID string }{
		{"location", "定位默认访问", in.Facts.Location, "location"},
		{"camera", "摄像头默认访问", in.Facts.Camera, "camera"},
		{"microphone", "麦克风默认访问", in.Facts.Microphone, "microphone"},
		{"notifications", "通知默认访问", in.Facts.Notifications, "notifications"},
		{"clipboard", "剪贴板读取默认访问", in.Facts.Clipboard, "clipboard"},
	} {
		status, summary = "unknown", "本次不能确认完整权限；请在 Windows 设置查看应用和桌面应用的不同范围。"
		evidence := "没有读取设备、请求位置、拍摄或录音"
		if item.value != "" {
			evidence = "读取到本机默认访问记录：" + item.value + "；没有枚举每应用例外或验证实际访问"
			if strings.EqualFold(item.value, "Allow") {
				status = "allowed"
				summary = "默认记录允许访问；具体程序仍取决于其他权限、策略和应用类型。"
			}
		}
		add(item.id, item.title, "设备权限", "app", status, summary, evidence, "默认记录不代表每个原生应用；Deny 也不被当成全局阻止证明", item.settingsID, "")
	}
	for _, item := range []struct{ id, title, category, summary, settingsID string }{
		{"browser-environment", "网页语言、时区与屏幕", "网页指纹", "实际浏览器尚未接入；网页通常能够读取多种环境特征，当前返回值未知。", "browser"},
		{"browser-gpu", "WebGL / WebGPU 与设备能力", "网页指纹", "本次未在用户浏览器创建设备或查询 GPU；WebGL 策略不能证明 WebGPU 已关闭。", "browser"},
		{"browser-render", "字体、Canvas 与音频", "网页指纹", "需要实际浏览器的原生保护和受测页面；没有权限提示也不代表网页无法形成指纹。", "browser"},
		{"browser-storage", "账号、Cookie 与站点存储", "身份关联", "没有读取账号、Cookie 值或浏览历史。已登录账号和站点数据仍可能关联身份。", "browser"},
		{"browser-webrtc", "WebRTC 真实连接路径", "网络路径", "没有访问外部 STUN 或建立通话；非代理 UDP 配置不等于完全关闭，也不能代替真实路径核验。", "browser"},
	} {
		add(item.id, item.title, item.category, "website", "unknown", item.summary, "未对用户浏览器和任意站点执行实测", "所选浏览器/配置/来源需单独核验", item.settingsID, "")
	}
	add("native-hardware", "原生程序硬件与环境信息", "原生应用", "app", "possible", "未隔离程序可能通过系统接口获取设备型号、部分硬件参数、字体或进程信息。管理员授权不会自动禁止这些读取。", "能力边界说明；本报告未读取硬件序列号、设备名称或安装程序清单", "取决于目标进程身份、系统版本和隔离范围", "privacy", "")
	add("local-network-identity", "MAC、设备名与发现信息", "网络身份", "network", "unknown", "没有读取实际 MAC 或设备名。是否随机化以及哪些广播仍存在，需要网卡和网络设置单独确认。", "本次不枚举适配器地址或扫描局域网", "本地网络；普通网页通常没有通用硬件 MAC 读取接口", "wifi", "")
	add("selected-files", "分享文件的元数据", "文件数据", "app", "manual", "本报告未包含文件检查，工具检查单独查看。", "不扫描磁盘或已打开文件", "仅用户主动选择的文件", "", "")
	addStorageEvidence(&r, n)
	bindSourceTimes(&r, in.SourceTimes)
	return r
}

// Do not export user names or selected executable paths embedded in OS errors.
var localPathPattern = regexp.MustCompile(`(?i)(?:\\\\\?\\)?\b[a-z]:[\\/][^\r\n"<>；;]+`)
var uncPathPattern = regexp.MustCompile(`\\\\[^\\/\s\r\n"<>]+[\\/][^\r\n"<>；;]+`)
var fileURLPattern = regexp.MustCompile(`(?i)file://[^\r\n"<>；;]+`)
var unixHomePathPattern = regexp.MustCompile(`(^|[\s(])/(?:Users|home|tmp|var|private|mnt|media|Volumes)/[^\r\n"<>；;]+`)

func redactLocalPaths(s string) string {
	s = fileURLPattern.ReplaceAllString(s, "[本地路径]")
	s = localPathPattern.ReplaceAllString(s, "[本地路径]")
	s = uncPathPattern.ReplaceAllString(s, "[本地路径]")
	return unixHomePathPattern.ReplaceAllString(s, "${1}[本地路径]")
}
