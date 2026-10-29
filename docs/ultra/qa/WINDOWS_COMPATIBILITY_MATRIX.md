# Windows x64 兼容性候选与实测矩阵

核对日期：2026-10-30。本文区分官方支持范围、源码中实际的检测/回退路径和真实 VM 证据。源码审查、编译、假后端测试均不等于对应 Windows 版本实测通过。本批新增 QA12.13 Win10 21H2 LTSC 2021 普通用户 GUI、隐私中心和浏览器保护视觉验收；浏览器宿主、网络链路和策略写入仍按边界记录。

## 目标系统

| 优先候选 | 基础 build | 必须记录的版本差异 | 本项目该版本实测状态 |
|---|---:|---|---|
| Windows 10 1809 / Enterprise LTSC 2019 | 17763 | LTSC 与普通 1809 的 edition、补丁、WebView2/Edge 是否预装不同 | **LIMITED**：同一候选包已真实安装，服务/CLI/隐私报告通过；交互式普通用户 GUI 受 Azure Run Command 无桌面会话限制 |
| Windows 10 21H2 / Enterprise LTSC 2021 | 19044 | LTSC 与普通 21H2 分别记载，不以共同内核替代 edition 检查 | **LIMITED**：QA12.13 真实安装、服务/CLI/隐私报告通过；Bastion 普通用户两次启动 GUI，隐私中心/浏览器保护/GPU 预览完成视觉核验；真实浏览器宿主、WFP、代理出口和流量仍待验收 |
| Windows 10 22H2 | 19045 | 优先 Pro；Home 的机器隐私策略须明确不支持，不能写入后假成功 | 有限实测：Pro 22H2 普通用户 GUI 启动；其余功能待逐项 VM 回归 |
| Windows 11 23H2 | 22631 | 优先 Enterprise/Pro；不能因“Windows 11”标签放行新 WinRT provider | 待 VM |
| Windows 11 24H2 | 26100 | 普通用户、UAC、安装与浏览器需各自记录包 hash 与版本 | 已有下述有限 QA.9/QA.11 桌面证据；整个版本矩阵未完成 |
| Windows 11 25H2 | 26200 | 按实际镜像 build/UBR 确认，不能从 24H2 结果推断 | 待 VM |

版本与 build 映射依据微软 [Windows 10 release information](https://learn.microsoft.com/en-us/windows/release-health/release-information) 和 [Windows 11 release information](https://learn.microsoft.com/en-us/windows/release-health/windows11-release-information)。补丁版本必须从目标 VM 重新读取，不把镜像 SKU/版本串当作实机 build。不同 edition 的支持策略、镜像可见性和 OS 生命周期另行确认；运行时支持与 Windows 安全更新支持不是同一个结论。

## 官方门槛与实际源码路径

| 组件 / 功能 | 已核对依据与源码 | 对候选矩阵的结论 |
|---|---|---|
| Go 主程序、服务、CLI | `go.mod` 为 Go 1.27.0；[Go 官方最低要求](https://go.dev/wiki/MinimumRequirements) 的 Go 1.21+ 为 Windows 10 / Server 2016+；本地对应 runtime 源码对高精度计时器、完成包和长路径有可选检测 | 六个候选均在官方 OS 家族范围内；仍需实际加载、进程生命周期、文件与网络测试 |
| Wails GUI / WebView2 | Wails v2.15.0；[WebView2 官方平台范围](https://learn.microsoft.com/en-us/microsoft-edge/webview2/) 包括 1709+、LTSC 2019/2021、Windows 11；[分发文档](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution) 要求处理 runtime 缺失 | 不能假定 LTSC/干净 Win10 已装 WebView2。未安装、旧 runtime、安装位置/ACL不满足的分支均须 VM 测试，不能假 ready |
| 独立 WebView2 host | `browser-host/Build.ps1` 固定 SDK 1.0.4314.35、net462；`host.exe.config` 要求 .NET Framework 4.6.2。微软 [Framework 对照表](https://learn.microsoft.com/en-us/dotnet/framework/install/versions-and-dependencies) 列 1809 自带 4.7.2，21H2/22H2 自带 4.8，目标 Win11 自带 4.8.1 | CLR 家族门槛满足不代表 SDK 所用 COM/CDP 能力满足。每次必须实际初始化、配置/读回成功才导航；旧 runtime 拒绝须保留 GUI 与 VPN |
| Chrome for Testing x64 | [Google 官方要求](https://support.google.com/chrome/a/answer/7100626) 为 Windows 10+/Server 2016+、支持 SSE3 的 x86 CPU；仓库固定官方目录/下载与真实版本核验 | OS 范围覆盖候选；CDP 命令、Job、WFP、proxy、HTTP/JS/iframe 读回仍按具体 Chrome 版本实测 |
| Firefox x64 | [Mozilla 官方支持平台入口](https://support.mozilla.org/en-US/products/firefox/installation-and-updates) 与其所链接的版本系统要求；仓库官方版本目录、BiDi 与实际 capability/readback | 本次网页抓取无法取到 158.0.1 的版本要求正文，故不补造精确版本声明。必须对实际下载版本核对官方要求；旧 BiDi 不支持的方法应清楚拒绝，不伪 CDP、不静默丢字段 |
| DPI / 窗口 | `cmd/godusevpn/window_screen_windows.go`；[GetDpiForWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdpiforwindow) 最低 1607。代码调用前 `Find`，失败回落至 OS 最大化/安全尺寸 | 未发现六个候选缺该入口的确定问题；高 DPI、窄桌面、跨屏、最大化/恢复仍需逐版 GUI 验证 |
| IPC / UAC / 进程与 ACL | `internal/ipc`、`privacyhost`、`paths`、`localstate`：SCM、token、KnownFolder、named-pipe peer PID、相对 NtCreateFile、文件安全描述符、共享与锁 | 本次没有发现六个候选所需 Win32 入口晚于 17763 的确定缺口。NT ABI、权限、替代管理员与 Medium 原用户语义仍必须逐版实测；不因编译成功认证安全 |
| TUN / DNS / 严格路由 | sing-tun v0.9.0-beta.4 的 `internal/winipcfg/zwinipcfg_windows.go` 三种 SetInterfaceDnsSettings ABI 均先 Find；`LUID.SetDNS/DisableDNSRegistration` 仅在 ERROR_PROC_NOT_FOUND 时走既有固定 netsh 回退 | [函数文档](https://learn.microsoft.com/en-us/windows/win32/api/netioapi/nf-netioapi-setinterfacednssettings) 写最低 19041，结构文档另写 18362，不能凭依赖注释“1809+”认证。17763 尤须验证回退路径的实际 DNS 与恢复；本批未改或运行隧道算法 |
| Windows Schannel | sing-box v1.14.0 的 `common/schannel/schannel_windows.go` 显式检查最低 17763，失败返回错误 | 1809 恰为该依赖声明门槛；并不表示所有 TLS/证书/代理场景已测 |
| WFP / 独立浏览器保护 | 原 VPN `netmode/wfp` 与独立 `browserengineguard` 使用 Fwpm*0、Job、精确 EXE/relay 规则、读回/撤销 | 可查到的入口家族不是新 Win11 专属。所有候选的 BFE、重启、规则读回、子进程、取消与失败保留保护均待真实 VM；不能为兼容降低 guard |
| 四项机器隐私策略 | `internal/privacy/catalog.go`：广告 ID 14393；输入个性化/剪贴板历史/云同步 17763；已有 build 与 edition gate | 目标 Pro/Enterprise 可进入检测，Home/未知 edition 不应放行。还须验证固定键读回、外部冲突与恢复，不能把所有隐私读取能力当被禁止 |
| 时区 / 设置入口 | `internal/timeprivacy/backend_windows.go` 对 Enum/GetDynamicTimeZoneInformation 先 Find；固定 System32/tzutil；`privacy_settings_windows.go` 固定 URI 清单 | 不自动改变 UTC/显示语言/键盘。各版本设置 URI、权限、恢复与刷新需真实验证 |
| 当前用户国家/地区 | `internal/regionprivacy` 使用 [GetUserDefaultGeoName](https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-getuserdefaultgeoname)/SetUserGeoName/EnumSystemGeoNames，API 最低 1709且先 Find；Medium 原用户、pending/备份/外部冲突检查 | 六个候选满足公布 API 门槛；本批只有纯 fake/ABI 测试，所有版本真实 apply/restore 待 VM。setter 无原子 CAS 的边界仍存在 |
| GPS / 系统位置 | [GeolocationProvider](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider?view=winrt-26100) 标 23504、UniversalApiContract 15，另需 runFullTrust/LAF；页面 SDK AppliesTo 不能当 runtime 门槛 | `locationadapter.RuntimeSupportStatus` 已加入本地 build 门禁：Win10/23H2 明确不开放，达到 23504 仅表示“可能存在”，仍需运行时类型/成员、LAF、runFullTrust、会话和原子写入后端。独立 Windows Device Portal 后端另行集成，未以说明代替实现，也未给任一版本标 PASS |
| 元数据 / 配置库 / 剪贴板 / 检测导出 | `privacydata`/`metadatatool`、`browserprofiles`、`clipboardprivacy`、`detection`：固定工具、相对文件/ACL、原生 clipboard owner/sequence/content、有界筛选预览 | Windows 基础 API 家族可覆盖候选；权限、文件对话框、TEMP 与用户目录、剪贴板竞争/取消等必须逐版验证。元数据有限 PASS 不涵盖全部格式/编辑 |

这是按仓库自有 Windows 源、C# P/Invoke、installer、固定 Go 依赖实际相关路径的审查结果，不是逐一调用所有 API 的测试报告。没有发现确定缺陷时没有新增未使用的兼容包、宽泛版本阻断或镜像实现的字符串测试。

## 逐功能 VM 验收表

`待` 表示没有该目标、该候选包对应的真实证据；`有限历史` 只能查阅所链接记录，不能算当前 next 源码或整模块 PASS。六列均需记录实际 edition、完整 build/UBR、包 ZIP/GUI hash、runtime/内核版本与普通/管理员身份。

| 必验功能 | 17763 | 19044 | 19045 | 22631 | 26100 | 26200 |
|---|---|---|---|---|---|---|
| 官方安装/卸载/升级、服务、重启 | 有限实测：安装/服务/CLI 通过，交互式 GUI 待 | 有限实测：安装/服务/CLI 通过，交互式 GUI 待 | 待 | 待 | 有限历史 | 待 |
| 普通用户 GUI、默认启动、UAC No/Yes、窗口/DPI | LIMITED：无交互桌面，GUI 路径受阻 | LIMITED：无交互桌面，GUI 路径受阻 | 有限实测：GUI 正常显示、默认启动未触发 UAC | 待 | 有限历史 | 待 |
| TUN、IPv4/IPv6、DNS及旧版回退、strict/no-direct、恢复 | 待 | 待 | 待 | 待 | 待完整场景 | 待 |
| 系统策略 / 时区 / 国家地区的保存、读回、外部冲突、恢复 | 待 | 待 | 待 | 待 | 待本批新地区能力 | 待 |
| 当前用户配置保存/载入/改名/克隆/取消删除/删除 | 待 | 待 | 待 | 待 | QA.11 有限历史 | 待 |
| JPEG/PNG 七字段及全部删除、像素/原件/目标不覆盖 | 待 | 待 | 待 | 待 | QA.9 限定16副本 | 待 |
| App 秘密复制后定时清理与后来用户复制保护 | 待 | 待 | 待 | 待 | 待 | 待 |
| SOLO proxy 测试与错误认证 / 无自动重试 | 待 | 待 | 待 | 待 | QA.11 限定 SOCKS probe | 待 |
| WebView2 host 初始化/UA/CH/CPU/GPU/权限/多frame读回与lease | 待 | 待 | 待 | 待 | 待实际完整链 | 待 |
| Chrome/Firefox 独立版本：WFP/Job/relay、页面和网络、启动取消/清理未知 | 待 | 待 | 待 | 待 | 待实际完整链 | 待 |
| 内核版本下载/库存/删除（在用/跨进程busy/partial） | 待 | 待 | 待 | 待 | 待新能力 | 待 |
| GPS provider / Device Portal 各自可用性、坐标读回、清除与异常退出 | 待 | 待 | 待 | 待 | 待新能力 | 待 |

已有有限记录：[QA.9 官方升级](QA9_OFFICIAL_UPGRADE_2026-10-26.md)、[QA.9 普通 GUI](QA9_ORDINARY_GUI_OBSERVATIONS_2026-10-26.md)、[QA.11 官方升级](QA11_OFFICIAL_UPGRADE_2026-10-27.md)、[QA.11 配置库](QA11_PROFILE_GUI_VM_2026-10-27.md)、[QA.11 代理](QA11_PROXY_GUI_VM_2026-10-27.md)。build 26100 的既有观察见 [QA.7 记录](QA7_UNINSTALL_AND_SOCKS_PROBE_2026-10-26.md)；后续同 VM 不能省略下一轮的实际 build 回读。旧 Server 2022 Core 与源码纯测试不填充客户端矩阵。

## 镜像与后续操作

先按 [Azure 只读查询](AZURE_WINDOWS_IMAGES_READONLY.md) 获得当前订阅、目标 region 的实际 x64 SKU/精确版本，再按 [Win10 旧版实机验收计划](WIN10_LEGACY_VM_ACCEPTANCE_PLAN_2026-10-27.md) 固定证据。未返回的 LTSC/旧版镜像写“未找到可用镜像”，不能猜 URN，不能用 Server 或当前 22H2 代替。Win10 22H2 的本次有限证据与限制见 [QA12_10_WIN10_STANDARD_GUI_2026-10-27.md](QA12_10_WIN10_STANDARD_GUI_2026-10-27.md)；17763/19044 已分别完成同一候选包的 Azure 真实安装、服务/CLI/隐私报告回读；普通用户 GUI、WebView2、UAC、浏览器和网络链路仍受交互式桌面/功能实测门槛限制，继续保持 LIMITED。

## 旧版实测记录入口

17763 和 19044 的实测结果必须分别链接独立记录文件，不能把同一份 22H2 记录复制到旧版行。本轮记录为 [QA12.12](QA12_12_WIN10_17763_VM_2026-10-29.md) 与 [QA12.11](QA12_11_WIN10_19044_VM_2026-10-29.md)。两版已有安装、服务/IPC、CLI 和隐私报告的真实回读；由于 Run Command 不能提供交互式普通用户桌面，矩阵仍保持 `LIMITED`，不能把服务证据扩展为 GUI、浏览器或网络链路 PASS。



