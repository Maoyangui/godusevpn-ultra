package privacy

// The catalog is the only source of writable registry locations. UI input and
// backup files cannot add arbitrary paths, values, commands, or URLs.
func catalog() []control {
	var out []control
	for _, browser := range []struct{ id, title, path, policyURL string }{
		{"chrome", "Chrome", `SOFTWARE\Policies\Google\Chrome`, "https://chromeenterprise.google/policies/#"},
		{"edge", "Edge", `SOFTWARE\Policies\Microsoft\Edge`, "https://learn.microsoft.com/en-us/deployedge/microsoft-edge-policies/"},
	} {
		add := func(id, title, name, description, impact string, desired value) {
			guide := "完全退出并重新启动浏览器，在 chrome://policy 或 edge://policy 重新加载策略并确认状态为有效；本地注册表检查不等于网页运行时验证。设备/云管理策略、站点例外和浏览器版本可能覆盖此值。"
			if browser.id == "edge" {
				guide += "部分 Edge 策略不适用于已登录 Microsoft 账号的配置文件；请以 edge://policy 和站点权限实际结果为准。"
			}
			out = append(out, control{State: ControlState{ID: browser.id + "-" + id, Title: browser.title + "：" + title, Category: "browser", Scope: "current-user / " + browser.title, Description: description, Impact: impact, Status: "available", Guide: guide, Restart: true, Sources: []string{browser.policyURL + name}}, Changes: []mutation{{location{"HKCU", browser.path, name}, desired}}})
		}
		webrtc := "WebRtcIPHandling"
		if browser.id == "edge" {
			webrtc = "WebRtcLocalhostIpHandling"
		}
		add("webrtc", "限制 WebRTC 非代理 UDP", webrtc, "使用正式策略限制 WebRTC 的接口和 UDP 使用；这不是完全关闭 WebRTC，也不保证公网地址不可见。", "可能影响会议、通话、实时媒体和企业 eCDN。TUN 是虚拟网卡，并不等于浏览器显式代理；隧道由原有网络保护负责。", textValue("disable_non_proxied_udp"))
		add("prediction", "关闭网络预测与预连接", "NetworkPredictionOptions", "减少 DNS 预取、预连接及预测页面访问。", "页面首开可能变慢；不拦截用户实际打开页面产生的连接。", dword(2))
		add("suggestions", "关闭在线搜索建议", "SearchSuggestEnabled", "减少地址栏输入在确认搜索前发送至搜索提供商。", "仍可显示本地历史和收藏建议；实际搜索仍发送查询。", dword(0))
		if browser.id == "chrome" {
			add("metrics", "关闭使用与崩溃指标上报", "MetricsReportingEnabled", "关闭 Chrome 的使用和崩溃报告策略。", "不关闭安全浏览、证书检查、浏览器更新或所有 Google 服务请求。", dword(0))
		} else {
			add("metrics", "仅保留必需诊断数据", "DiagnosticData", "Edge 122+：关闭可选诊断，保留必需数据。", "并非零上报；保留安全、更新和可靠性所需的诊断。旧版浏览器需手动检查。", dword(1))
		}
		add("location", "默认阻止网页定位", "DefaultGeolocationSetting", "将网页定位权限的默认值设为阻止。", "不伪造 GPS，不隐藏 IP 推测位置；已有企业允许列表或特殊例外需另查。", dword(2))
		add("camera", "默认阻止网页摄像头", "VideoCaptureAllowed", "阻止常规网页请求视频输入；正式策略允许列表仍可例外放行。", "会议和扫码可能不可用；不影响原生应用和浏览器外的相机权限。", dword(0))
		add("microphone", "默认阻止网页麦克风", "AudioCaptureAllowed", "阻止常规网页请求音频输入；正式策略允许列表仍可例外放行。", "通话、录音和语音输入可能不可用；不等于禁止全部音频指纹。", dword(0))
		add("notifications", "默认阻止网页通知", "DefaultNotificationsSetting", "将网页通知权限默认值设为阻止。", "网页消息提醒可能停止；站点或企业例外需要分别检查。", dword(2))
		add("sensors", "默认阻止网页传感器", "DefaultSensorsSetting", "将运动和光线等传感器权限默认值设为阻止。", "不覆盖所有硬件查询接口；站点允许列表可能例外。", dword(2))
		add("third-party-cookies", "阻止第三方 Cookie", "BlockThirdPartyCookies", "限制常规浏览会话中的第三方 Cookie。", "跨站登录、嵌入内容可能受影响；不是身份隔离，也不会清除已有第一方数据或登录身份。", dword(1))
		add("webgl", "禁用 WebGL／Pepper 3D", "Disable3DAPIs", "通过官方策略限制 WebGL 和 Pepper 3D API。", "3D 地图和图形网页可能无法使用。此策略不作为 WebGPU 禁用或完整 GPU 匿名化的证据。", dword(1))
	}
	addSystem := func(id, title, path, name, description, impact string, n uint32, min uint32, source, settingsID string) {
		out = append(out, control{State: ControlState{ID: id, Title: title, Category: "windows", Scope: "machine / Windows", Description: description, Impact: impact, Status: "available", RequiresAdmin: true, Guide: "仅在已核实支持该组策略的 Windows 10/11 Pro、Enterprise、Education 等版本提供自动设置；注册表回读只能确认配置，应用范围以系统说明为准。", SettingsID: settingsID, Restart: true, Sources: []string{source}}, Changes: []mutation{{location{"HKLM", path, name}, dword(n)}}, SystemPolicy: true, MinBuild: min})
	}
	privacyURL := "https://learn.microsoft.com/en-us/windows/client-management/mdm/policy-csp-privacy"
	addSystem("windows-advertising-id", "关闭 Windows 广告 ID", `SOFTWARE\Policies\Microsoft\Windows\AdvertisingInfo`, "DisabledByGroupPolicy", "限制支持 Windows 广告 ID 的应用跨应用关联。", "不改变其他硬件标识、账号、Cookie 或第三方自建标识。", 1, 14393, privacyURL+"#disableadvertisingid", "privacy-general")
	addSystem("windows-online-speech", "关闭输入个性化", `SOFTWARE\Policies\Microsoft\InputPersonalization`, "AllowInputPersonalization", "限制 Windows 输入个性化数据和相关云端学习。它不等于关闭所有在线语音识别。", "个性化输入和相关功能可能受影响；第三方应用自行上传语音或文本不受此策略统一限制。", 0, 17763, privacyURL+"#allowinputpersonalization", "privacy-speech")
	addSystem("windows-clipboard-history", "关闭剪贴板历史", `SOFTWARE\Policies\Microsoft\Windows\System`, "AllowClipboardHistory", "禁止 Windows 保存剪贴板历史。", "不清除已上传数据，不阻止桌面应用访问当前剪贴板，也不清理第三方剪贴板管理器。", 0, 17763, "https://learn.microsoft.com/en-us/windows/client-management/mdm/policy-csp-experience#allowclipboardhistory", "clipboard")
	addSystem("windows-clipboard-cloud", "关闭剪贴板跨设备同步", `SOFTWARE\Policies\Microsoft\Windows\System`, "AllowCrossDeviceClipboard", "禁止 Windows 剪贴板同步到其他设备。", "不清除云端已有内容，也不控制第三方同步软件。", 0, 17763, privacyURL+"#allowcrossdeviceclipboard", "clipboard")
	return out
}

func capabilities() []Capability {
	return []Capability{
		{"permissions", "Windows 应用权限", "OS / per-app", "定位、摄像头、麦克风、联系人、日历、账户、消息、后台权限可在系统设置收紧；传统桌面应用并非全部受统一权限开关约束。", "在 Windows 隐私与安全中逐项查看；这版不篡改未公开的 ConsentStore，也不声称管理员可统一拦截一切读取。", "manual", "privacy"},
		{"timezone-language", "时区、语言与地区", "OS + browser", "管理员可修改部分系统设置，但和网页 Accept-Language、Intl、浏览器覆盖值不一定一致；盲目跟随出口会造成不一致和额外指纹。", "在时间和语言设置选择一致且稳定的配置；不自动根据外部 IP 查询或地理定位来修改。", "manual", "dateandtime"},
		{"private-mac", "Wi-Fi 随机硬件地址", "local network", "支持的无线网卡可用 Windows 随机硬件地址；网页通常没有直接读取硬件 MAC 的标准接口。", "在 Wi-Fi 属性中检查随机硬件地址。更改可能断网或影响网络准入，本版不修改网卡、不读取实际 MAC。", "manual", "network-wifi"},
		{"webrtc-off", "完全关闭 WebRTC", "browser", "本版仅支持正式非代理 UDP 限制策略；完全关闭需要支持此功能的浏览器设置或配套扩展。", "对视频通话有影响。不要用封禁几个 STUN 端口当作完全禁用。", "manual", ""},
		{"ping-referrer", "链接审计、跟踪参数与 Referer", "browser extension", "普通注册表策略不足以可靠清理所有链接跟踪和网页请求头；需要浏览器扩展和站点例外。", "使用受信任的隐私扩展并检查权限；勿安装来源不明的全站读取扩展。当前版本未注入网页脚本。", "manual", ""},
		{"fingerprint", "Canvas、Audio、字体、屏幕与计时指纹", "controlled browser", "需要浏览器原生反指纹能力或专用配置；随机伪造互相矛盾的信息可能使设备更独特。", "使用支持统一反指纹的浏览器，保持稳定一致的配置。当前应用不承诺全局加噪或隐藏字体。", "manual", ""},
		{"webgpu", "WebGPU 设备与能力信息", "browser", "WebGL 禁用不等于 WebGPU 禁用；设备名称之外，功能集合、限制和性能仍可用于指纹。", "在支持的浏览器中关闭或限制 WebGPU 并做本地验证。本版不提供未经核实的通用开关。", "manual", ""},
		{"site-isolation", "身份、Cookie 与站点存储隔离", "browser profiles", "第三方 Cookie 策略不能替代独立身份容器；登录账号本身仍可关联身份。", "为不同身份使用独立浏览器配置，控制扩展和同步，并按需清理会话数据。", "manual", ""},
		{"hardware-identifiers", "原生应用硬件信息", "native apps", "Windows 应用层无法统一无痕禁止所有应用读取 CPU/GPU、主板或硬盘信息；管理员权限不会消除这一边界。", "对不可信程序使用受限账号、Windows Sandbox 或虚拟机，检查设备直通和共享目录。体检不采集实际硬件序列号。", "unsupported", ""},
		{"gps-spoof", "位置组件（需核验）", "Windows Location Provider", "候选开源组件可覆盖部分 Windows 位置接口或浏览器测试覆盖；它不能改变 GeoIP、Wi-Fi、账号记录或物理 GPS。当前版本只提供受控适配边界，未随包执行第三方组件。", "安装前必须核对来源、许可证、版本和 SHA-256；应用并独立读回后才能显示已启用。组件未核验时只显示说明，不修改系统、不影响隧道。", "planned", "privacy-location"},
		{"app-network-firewall", "按应用联网与局域网／本机端口隔离", "WFP / network", "已有独立的指定程序 WFP 过滤器安装、删除和本地安装检查；过滤器存在不是所有流量路径均已阻断的证明。", "规则尚未经真实流量验收；回环路径未保护，代理重发及原隧道闸高优先级许可可能改变结果。检查应用规则页面的安装证据与限制，不把它当作完整端口扫描防护；不改变原隧道。", "limited", "windowsdefender"},
		{"file-metadata", "分享前清理文件元数据", "selected files", "已支持用户选定 JPEG/PNG 的指定元数据块检查和新副本清理；不扫描磁盘、不覆盖原文件，也不读取或返回真实 GPS 内容。", "在本地图片工具中检查并另存副本。保留 ICC、其他附加块与原编码像素，不会匿名化画面；删除 EXIF 方向可能影响显示。文档、其他图片格式和通用上传清理尚未实现。", "limited", ""},
		{"access-audit", "实际读取记录与可读取性", "OS + browser", "系统没有提供所有应用、网页信息读取行为的完整统一审计；没有记录不代表无法读取。", "本版只检查本地策略配置，不打开外部检测站，不启动网页，不输出真实 IP、MAC 或硬件序列号。", "limited", ""},
		{"security-boundary", "安全防护与隧道边界", "application", "隐私策略不修改 Safe Browsing、SmartScreen、系统更新、证书信任、DNS、路由、隧道或 TLS 内容。", "隐私设置失败、被覆盖或无管理员权限时继续显示独立状态；不能因此拒绝建隧道、回退直连或关闭其他保护。", "implemented", ""},
	}
}

func findControl(id string) (control, bool) {
	for _, c := range catalog() {
		if c.State.ID == id {
			return c, true
		}
	}
	return control{}, false
}
