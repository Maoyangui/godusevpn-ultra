// Package centerguide supplies compact, local help for the privacy and detection
// centers. This catalog neither observes the machine nor authorizes an action.
// Live state, timestamps and permission checks always come from the real backend.
package centerguide

type Catalog struct {
	SchemaVersion int      `json:"schemaVersion"`
	Kind          string   `json:"kind"`
	Modules       []Module `json:"modules"`
}

type Module struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Capability   string   `json:"capability"`
	Scope        string   `json:"scope"`
	Verification string   `json:"verification"`
	Recovery     string   `json:"recovery"`
	Limitations  []string `json:"limitations"`
	Pending      []string `json:"pending"`
	ControlIDs   []string `json:"controlIds"`
	SelectionIDs []string `json:"selectionIds"`
	Backend      Backend  `json:"backend"`
	Actions      []Action `json:"actions"`
}

// Backend is a traceable capability mapping, not a dynamic dispatcher. The UI
// must never invoke a method from catalog text or bypass its existing workflow.
type Backend struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

// Action targets are closed navigation/settings identifiers, never URLs, paths,
// registry locations, command lines, or caller-provided executable names.
type Action struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// Guides creates fresh nested slices for every caller. Its capability labels
// describe implementation scope, never current availability or protection.
func Guides() Catalog {
	return Catalog{SchemaVersion: 1, Kind: "help_only_not_live_evidence", Modules: []Module{
		{
			ID: "system-policies", Title: "Windows 隐私策略", Summary: "按需收紧广告 ID、输入个性化与剪贴板历史、同步。", Capability: "limited",
			Scope:        "四项固定机器策略；需管理员助手与受支持的 Windows 版本。",
			Verification: "重新读取固定策略值、支持范围与备份状态；策略匹配不等于所有应用均已受限。",
			Recovery:     "逐项或统一恢复本应用记录的系统策略；外部变更保留并提示冲突。",
			Limitations:  []string{"不关闭第三方应用自己的标识、上传或同步。", "系统策略可能持续生效；退出本应用不等于恢复。"},
			Pending:      []string{"更多受支持系统隐私项及真实应用行为、权限和重启矩阵。"},
			ControlIDs:   []string{"windows-advertising-id", "windows-online-speech", "windows-clipboard-history", "windows-clipboard-cloud"},
			Backend:      Backend{Read: []string{"PrivacyReport", "PrivacyHelperStatus"}, Write: []string{"PrivacyApply", "PrivacyRestore", "PrivacyRestoreAll"}},
			Actions:      []Action{{"privacy-tab", "system", "查看系统策略"}},
		},
		{
			ID: "system-timezone", Title: "系统时区", Summary: "从 Windows 实际列表选择时区，确认后应用并读回。", Capability: "backend",
			Scope:        "仅系统时区，保留夏令时偏好；不修改 UTC 时钟、语言或隧道。",
			Verification: "从系统读取真实时区选项和当前时区；修改后重新读取。",
			Recovery:     "恢复本应用保存的原时区和夏令时偏好；外部修改不强行覆盖。",
			Limitations:  []string{"不根据代理出口自动改时区；浏览器可以使用独立覆盖值。"},
			Pending:      []string{"真实普通用户、替代管理员、重启及外部冲突组合。"},
			Backend:      Backend{Read: []string{"PrivacyTimeReport"}, Write: []string{"PrivacyTimeApply", "PrivacyTimeRestore"}},
			Actions:      []Action{{"privacy-tab", "system", "选择系统时区"}, {"settings", "dateandtime", "Windows 时间设置"}},
		},
		{
			ID: "system-language-region", Title: "语言与地区", Summary: "选择并恢复当前用户的国家或地区；显示语言和键盘另行设置。", Capability: "limited",
			Scope:        "仅当前普通 Windows 用户的国家或地区，不改 GPS、IP、显示语言、键盘、时区或浏览器环境。",
			Verification: "从系统清单选择，确认后保存原值并重新读取当前地区；其他应用可能缓存旧值。",
			Recovery:     "在国家或地区卡片恢复本应用保存的最初值；发现外部变更或备份不明时停止。",
			Limitations:  []string{"系统接口没有与其他程序的原子比较替换，同时修改仍有竞争窗口。", "更改显示语言可能涉及语言包和重新登录；日期、数字格式、键盘及浏览器请求头分别检查。"},
			Pending:      []string{"国家或地区的多版本普通用户真实修改、恢复与外部冲突验证。", "显示语言、日期与数字格式和键盘的受控编辑尚未接入。"},
			Backend:      Backend{Read: []string{"PrivacyRegionReport", "PrivacyDetectionSnapshot"}, Write: []string{"PrivacyRegionApply", "PrivacyRegionRestore"}},
			Actions:      []Action{{"privacy-tab", "system", "选择国家或地区"}, {"settings", "language", "Windows 语言与地区"}},
		},
		{
			ID: "app-permissions", Title: "应用权限", Summary: "逐项核对定位、摄像头、麦克风等权限，按应用收紧。", Capability: "settings",
			Scope:        "系统权限入口与有限状态证据；传统桌面应用并非全部受统一开关控制。",
			Verification: "检测快照保留读取来源和未知状态，不把总开关推断为某个进程已无法读取。",
			Recovery:     "在对应 Windows 设置页恢复；当前未自动写入此类应用授权。",
			Limitations:  []string{"不篡改未公开的权限存储；网页权限与系统权限需分别检查。"},
			Pending:      []string{"为有正式可用接口的权限增加逐应用编辑与独立消费者验证。"},
			Backend:      Backend{Read: []string{"PrivacyDetectionSnapshot"}},
			Actions:      []Action{{"settings", "privacy-location", "定位权限"}, {"settings", "camera", "摄像头权限"}, {"settings", "microphone", "麦克风权限"}, {"settings", "privacy", "其他系统权限"}},
		},
		{
			ID: "program-network", Title: "程序联网", Summary: "选择程序与限制类型，安装、检查或移除独立规则。", Capability: "limited",
			Scope:        "指定 EXE 路径的新建直接连接；回环、代理代发、子进程及已有连接另有边界。",
			Verification: "重新检查本应用规则与数量；规则存在不是实际流量已阻断的证明。",
			Recovery:     "只移除所选本应用规则并检查结果，不撤销隧道或其他应用规则。",
			Limitations:  []string{"原隧道高优先级许可可能改变匹配结果；改路径或其他程序代发不在原规则范围。"},
			Pending:      []string{"方向、协议、地址与端口的真实规则编辑；命中记录、首次联网决策及流量验收。"},
			SelectionIDs: []string{"block_all", "block_lan", "block_discovery"},
			Backend:      Backend{Read: []string{"PrivacyFirewallReport", "PrivacyChooseProgram"}, Write: []string{"PrivacyFirewallApply", "PrivacyFirewallRemove"}},
			Actions:      []Action{{"privacy-tab", "network", "管理程序规则"}, {"settings", "windowsdefender", "Windows 安全设置"}},
		},
		{
			ID: "wifi-mac", Title: "Wi-Fi 随机地址", Summary: "在支持的网卡与网络上检查 Windows 随机硬件地址。", Capability: "settings",
			Scope:        "本地网络可见的无线地址；本应用不读取或修改实际 MAC。",
			Verification: "当前只打开固定 Wi-Fi 设置入口，没有伪造已随机化的状态。",
			Recovery:     "由用户在该网络属性恢复；当前没有网卡写入或本应用备份。",
			Limitations:  []string{"取决于驱动与网络配置；更改可能中断连接或影响网络准入。", "MAC 与代理出口 IP、网页身份不同。"},
			Pending:      []string{"受支持网卡的能力检测、明确选择、断网预告与可恢复事务。"},
			Actions:      []Action{{"settings", "network-wifi", "Windows Wi-Fi 设置"}},
		},
		{
			ID: "native-hardware", Title: "设备与硬件标识", Summary: "应用隔离可降低暴露；当前不提供全局硬件隐藏开关。", Capability: "pending",
			Scope:        "原生应用可见信息与网页可见值不同；管理员能力不等于可统一隐藏硬件。",
			Verification: "本应用不收集硬件序列号，不用网页属性覆盖证明操作系统设备已改变。",
			Recovery:     "当前未写入设备名称或硬件标识，无此类恢复记录。",
			Limitations:  []string{"CPU、GPU、主板、磁盘等来源不同；账号、共享目录和设备直通仍可能关联身份。"},
			Pending:      []string{"系统设备名称的正式接口编辑、重启提示和备份；受限运行环境的实际隔离工作流。"},
		},
		{
			ID: "location-provider", Title: "系统位置模拟", Summary: "真实 Windows 位置提供器尚未接通，不显示已模拟。", Capability: "pending",
			Scope:        "系统定位、浏览器位置、GeoIP 与物理 GPS 是不同来源。",
			Verification: "当前仅有事务适配器与纯测试，没有可用于启用状态的真实提供器和独立观测。",
			Recovery:     "未提供真实 Set / Clear 操作；后续必须保留原 override 所有权并拒绝外部冲突。",
			Limitations:  []string{"尚无可用的 Windows 系统定位提供器，不能显示为已模拟。", "代理归属地不会改变系统位置；管理员授权本身也不代表模拟可用。"},
			Pending:      []string{"接通正式系统位置提供器，独立读取新鲜位置，并验证停止后的恢复与外部冲突处理。"},
			Actions:      []Action{{"settings", "privacy-location", "Windows 定位设置"}},
		},
		{
			ID: "image-metadata", Title: "图片元数据", Summary: "选择 JPEG / PNG 与 EXIF 类别，检查后另存清理副本。", Capability: "limited",
			Scope:        "固定 ExifTool 配套的七组 EXIF；只处理显式选中的图片，不覆盖原图。",
			Verification: "校验工具配套、预览源摘要与副本字段读回，并核对源未变及编码像素。",
			Recovery:     "原图保留；取消或失败不显示副本成功，已有目标不会被覆盖。",
			Limitations:  []string{"默认只显示类别与数量，真实字段值需另行显式选择。", "不匿名化画面；XMP、IPTC、缩略图、文件时间等可能保留，方向标签移除可能影响显示。"},
			Pending:      []string{"可编辑字段值、XMP / IPTC 与更多格式；实拍图片及异常组合验收。"},
			SelectionIDs: []string{"artist", "copyright", "description", "software", "camera", "capture-time", "exif-gps"},
			Backend:      Backend{Read: []string{"MetadataToolStatus", "MetadataPreviewImage", "MetadataReadValues"}, Write: []string{"MetadataCreateCopy"}},
			Actions:      []Action{{"privacy-tab", "data", "检查图片并另存"}},
		},
		{
			ID: "secret-clipboard", Title: "秘密复制后清理", Summary: "默认关闭；开启后只定时清理由本应用刚复制的秘密。", Capability: "limited",
			Scope:        "5–600 秒；序号、所有者与内容摘要仍匹配才清理当前剪贴板。",
			Verification: "只显示本应用的清理任务状态，不读取或显示任意剪贴板内容。",
			Recovery:     "可以取消计时或立即清理仍属本模块的当前内容；退出时作同样匹配检查。",
			Limitations:  []string{"用户后来复制的内容保留；取消计时不撤回已复制文本。", "不清除历史、云端或已被其他程序读走的副本。"},
			Pending:      []string{"真实剪贴板竞争、繁忙重试、退出及历史 / 云同步组合验收。"},
			Backend:      Backend{Read: []string{"ClipboardPrivacyReport"}, Write: []string{"ClipboardPrivacySave", "ClipboardPrivacyCopyProfile", "ClipboardPrivacyCancel", "ClipboardPrivacyClearNow"}},
			Actions:      []Action{{"app-page", "settings", "设置清理时长"}, {"app-page", "profiles", "查看秘密复制入口"}},
		},
		{
			ID: "diagnostics-logs", Title: "日志与诊断", Summary: "调整日志保留；先审阅所选诊断内容，再保存同一份预览。", Capability: "limited",
			Scope:        "本产品设置、当前可见日志与有界诊断；不扫描或清理其他应用。",
			Verification: "日志读取与诊断预览来自实际接口；导出核对已审预览 ID、字节摘要和内容。",
			Recovery:     "取消保存不导出；日志保留设置可再改，已删除数据不能靠恢复设置找回。",
			Limitations:  []string{"默认脱敏不等于绝无可关联上下文；分享前仍需审阅。", "保留配置不是旧日志已清除或无法恢复的证明。"},
			Pending:      []string{"按产品数据类别和时间预览后的显式清理与独立删除结果；真实保留周期验收。"},
			Backend:      Backend{Read: []string{"GetSettings", "GetLogs", "PreviewDiag"}, Write: []string{"SaveSettings", "ExportDiagPreview"}},
			Actions:      []Action{{"app-page", "settings", "日志保留设置"}, {"app-page", "logs", "查看日志与诊断"}},
		},
		{
			ID: "credential-storage", Title: "订阅与凭据存储", Summary: "当前未接通凭据加密，不把文件权限称为已加密。", Capability: "pending",
			Scope:        "本客户端的订阅、认证及缓存；不包括浏览器或其他应用的密码库。",
			Verification: "此项是实现缺口说明，不读取真实订阅、密码或缓存内容来生成帮助。",
			Recovery:     "尚无加密迁移、回退或密钥恢复功能，不显示虚构开关。",
			Limitations:  []string{"同用户进程与管理员仍有边界；认证不应进入日志、诊断或普通导出。"},
			Pending:      []string{"原用户 DPAPI 加密、版本化迁移与回退、崩溃恢复及真实秘密 canary 检查。"},
		},
		{
			ID: "detection-observations", Title: "本地暴露检测", Summary: "区分实际观察、配置证据与未知，再选择可处理项。", Capability: "limited",
			Scope:        "本地策略、程序规则、已认证服务状态和有限环境事实；不自动外联检测站。",
			Verification: "每项保留来源、观察身份和读取完成时间；来源缺失不补造时间或保护结论。",
			Recovery:     "更改或窗口回到前台后旧结果失效；重新检测，重新审阅所选快照再导出。",
			Limitations:  []string{"管理员可读不代表普通应用可读；无记录不代表无法读取。", "配置值不证明网站、所有进程或网络路径的实际行为。"},
			Pending:      []string{"目标应用 / 浏览器的独立消费者观测与流量验证；更完整且默认最少的来源矩阵。"},
			Backend:      Backend{Read: []string{"PrivacyDetectionSnapshot", "PrivacyPreviewDetectionExport", "PrivacyDetectionTextPreview", "PrivacyDetectionTextForCopy"}, Write: []string{"PrivacyExportDetection"}},
			Actions:      []Action{{"app-page", "detection", "重新本地检测"}},
		},
		{
			ID: "recovery", Title: "恢复原值", Summary: "分开恢复系统策略、时区与本应用程序规则。", Capability: "limited",
			Scope:        "仅本应用已记录且能确认所有权的修改，不提供清空所有保护的总开关。",
			Verification: "检查备份、当前状态与外部冲突；恢复后读回，未知或失败保留线索。",
			Recovery:     "系统策略、时区与程序规则各走自己的受控恢复入口。",
			Limitations:  []string{"退出应用不等于恢复；不强行覆盖其他软件后来的修改。"},
			Pending:      []string{"跨用户、并发、崩溃、损坏、短写及同步失败的完整真实恢复矩阵。"},
			Backend:      Backend{Read: []string{"PrivacyReport", "PrivacyTimeReport", "PrivacyFirewallReport"}, Write: []string{"PrivacyRestore", "PrivacyRestoreAll", "PrivacyTimeRestore", "PrivacyFirewallRemove"}},
			Actions:      []Action{{"privacy-tab", "system", "检查系统恢复记录"}, {"privacy-tab", "network", "检查程序规则"}},
		},
	}}
}
