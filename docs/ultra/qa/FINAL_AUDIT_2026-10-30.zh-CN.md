# GodUseVPN Ultra 全量复核回执（2026-10-30）

本回执重新对照两段接手对话及 H01–H41 要求，并把源码、包、UI、Azure Win10 实机和线上试用接口分开判定。通过纯测试或静态检查的项目不会被写成真实运行时通过。

## 已完成并有证据

| 范围 | 证据 |
| --- | --- |
| 浏览器配置交互 | 四种网页环境预设、OS 版本/视口、UA 生成与手改、GPU/WebGL 字段、代理草稿、内核下载/校验/删除、预览失效和启动前复核；DOM QA 全部通过 |
| 隐私中心/检测中心 | 状态分层、未知/有限/计划中不升级、减少重复帮助标记、紧凑本地时间、页内错误反馈、检测快照筛选/导出、键盘与窄屏布局；当前试用卡片/帮助 fixture 修复后再次执行 420px 浅色 smoke，共 8 张截图，`errors=[]`、`outboundRequests=[]`；截图目录 `C:\\Temp\\ultra-trial-ui-qa\\final-audit-after-fixture-fix` |
| 开源集成 | sing-box、WFP/WireGuard 规则、ExifTool、WebView2 SDK/宿主、Chrome/Firefox 下载校验链均有源码入口、固定版本/许可证或哈希说明；对应 Go/Node 测试通过 |
| 独立会话数据生命周期 | 浏览器每次启动使用独立会话目录；进程/防火墙收尾后以 `os.Root` 相对路径扫描并删除本次树，拒绝重解析点、异常对象和共享目录，失败保持 `closing_or_unknown`；Windows 交叉编译与纯文件回归见 [会话清理审计](BROWSER_SESSION_CLEANUP_2026-10-30.md) |
| 秘密剪贴板 | 本应用复制路径已接入序号、所有者和内容摘要的条件清理；默认关闭、时长 5–600 秒，Go 与 DOM 回归覆盖取消、替换内容保留和服务不可用，详见 [审计记录](CLIPBOARD_PRIVACY_QA_2026-10-31.md)。真实 VM 的竞争、退出、历史/云同步边界仍待实测，不扩展为全系统剪贴板控制 |
| Win10 实机 | Win10 21H2 LTSC（build 19044）和 Win10 1809 LTSC（build 17763）均有同一候选包的真实安装、SCM 服务、CLI 与隐私报告回读，分别见 QA12.11/QA12.12，均保持 LIMITED。当前 v0.02 预览包也在 Azure Win10 21H2 build 19044 完成包校验、固定目录安装、服务 Running/Auto/LocalSystem、CLI 版本/状态和隐私报告回读，见 [v0.02 VM 复测](QA_V002_VM_RETEST_2026-10-30.zh-CN.md)。19044 另有 QA12.13/QA12.15 的普通用户 GUI、隐私中心/浏览器保护视觉核验，以及真实订阅、TUN/strict 和出口回读；这些证据不外推为原生浏览器宿主或全版本通过。最新现代 UI beta.6.1 发布包 SHA-256 为 `0f2bc0742459b04eefe9084ff8294adc7b5abf950e50e1d88823724df6bf0019`，其 UI/包审计另行通过，未冒充为旧候选 VM 运行证据。 |
| 真实订阅/隧道 | 在该 VM 内设置用户提供的测试订阅并刷新 18 个节点，connect 退出码 0，状态 `connected`、`TUN=true`、`strict=true`；通过 TUN 访问 ipify 和 ipwho.is 得到同一出口 `104.28.222.43`（Singapore），disconnect 后状态恢复 `disconnected` |
| 清理 | 临时资源组 `ultra-browser-qa-20261030` 已在 Azure 通知中确认删除，资源组列表不再显示；其中 VM、Bastion、OS 磁盘、NIC、公共 IP、NSG、VNET 均随组删除，其他原有资源未触碰；临时 gists 与本地分片已删除 |
| 发布后候选复核 | [beta.6.6 发布包](https://github.com/Maoyangui/godusevpn-ultra/releases/tag/v0.01-beta.6.6) 已上传 GitHub Release，ZIP SHA-256 `977e7e649b2a83a3437cb9995372208a84407973ffdc652738c9dd03e18182ab`；助手失联重新授权、Device Portal 已准备记录安全重连、独立会话数据清理和 iOS UA 边界文案回归通过。当前 Azure 门户对临时 Windows VM 的三个候选区域均返回规格不可用/系列配额不足，未提交新的部署，不产生新资源。 |

## 仍不能标记为完成的项目

| 范围 | 当前事实与原因 |
| --- | --- |
| 真实内置浏览器窗口 | 普通构建已进入受控 native Start 严格链路；本轮 VM 的 Azure Run Command SYSTEM 会话得到固定 `start_refused`。Developer Bastion 已部署，但 Bastion 表单在提交后反复回到 `Value is required`，Windows RDP 仍停在本地不受信任证书确认页；当前临时 Windows 规格还受 Azure 订阅配额限制，因此没有把窗口、导航或代理出口写成通过 |
| Chrome/Firefox 多版本原生启动 | Windows amd64/arm64 普通构建已进入受控 native Start 严格链路；下载/完整树校验、CDP/BiDi 和启动门禁通过。真实进程矩阵、窗口、导航、代理出口、WebRTC/WebGPU/OOPIF/Worker 读回仍未在普通用户 VM 完成 |
| 会话数据删除的真实效果 | 代码已保证只处理本次生成的 `BrowserInstances/<session-id>`，但真实浏览器退出、异常进程、重启以及跨 Win10/Win11 版本的磁盘读回尚未完成；不能以纯文件回归替代 VM 证据 |
| 每浏览器 SOCKS/HTTP/HTTPS 浏览器出口 | 代理配置、relay 和 probe 已实现/测试；本轮只实测 Ultra TUN 出口，未取得普通用户浏览器实际导航抓包证据 |
| GPS/Windows Location Provider | Win10 build 19044 低于 GeolocationProvider 23504 门禁，UI 正确显示不可用；Win11 只提供“可能支持”检查。现有代码是只读观察与恢复边界，没有可证明的系统位置写入、独立读回和冲突恢复闭环 |
| CEF/uBlock | 仓库只有候选/审计记录，没有可运行集成和更新/权限流程 |
| Win10 1809/LTSC 2019、22H2 与 Win11 多版本 | 17763/19044 已有有限安装与服务/CLI/隐私报告证据，19044 另有 GUI/隧道扩展证据；1809 的普通用户 GUI、22H2 完整功能和 Win11 多版本仍需各自 VM/接口矩阵证据 |
| godusevpn.org 线上免费试用 | 已通过 Cloudflare Worker 版本 `608a5434-e3ee-4067-bfce-3fbfa234d9df` + `godusevpn-commerce` D1 上线；服务端只保存设备 HMAC 摘要、月份、短标识、订阅 URL、来源备注和时间，不把面板令牌交给客户端。实测 status 200、claim 200 返回真实 `hk.joinvip.vip:2056` 订阅且订阅 URL 返回 HTTP 200、同设备再次 status 为 `already_claimed`、再次 claim 为 HTTP 409，OPTIONS 为 204，非法设备为 400，恶意 Origin 不返回允许跨域头；D1 备注为“来源：佛跳墙内置免费试用；套餐：free”。公开前缀健康检查 `/app/api/trial/healthz` 返回 `backend=d1`、`backendConfigured=true`、`upstreamConfigured=false`。后台截图确认面板 API 已就绪、`free` 套餐存在并完成映射。详细回执见 [D1 线上回归](TRIAL_PRODUCTION_D1_2026-10-30.md)。 |

## 结论

源码、UI、包、订阅隧道、生产试用网关和 Win10 1809/21H2 的有限实测已形成可复核证据；整体 H01–H41 仍未闭环。发布说明、客户端状态和最终答复必须继续保留上述“待实机/未集成”边界，不能用说明卡、mock 或静态载荷充数。

## 视觉标准复核（现代 App UI）

2026-10-30 在 `6ad868e` 与 `80567db` 后重新执行视觉回归：检测中心时间统一为本地短格式，隐私页取消无意义的重复问号提示，移动窄屏标签改为可横向滚动的单行导航，错误提示上移并限制宽度与换行，避免覆盖内容卡片。随后修正浏览器硬件/启动 DOM fixture 的 `canLaunch` 契约并重新执行，浏览器内核、硬件、启动取消竞态和配置 UI 回归均通过；这些仍是隔离 DOM 测试，不升级为原生 VM 证据。

- DOM QA：浏览器环境、UA/GPU、代理草稿、内核管理、错误转义、过期快照与未支持入口全部通过；v0.02 发布后再次执行七组 UI fixture，结果全部通过。
- 像素 QA：420px 窄屏与 900px 深浅色完整审计共 32 张截图，`errors=[]`、`outboundRequests=[]`；v0.02 发布后以 Chrome headless 在 420×760 浅色视口再次生成 8 张截图，`errors=[]`、`outboundRequests=[]`，输出目录为 `C:\Temp\ultra-v002-final-visual`。beta.6.6 发布包重新执行 ZIP/PE/前端资产审计，payload 1998、ZIP 成员 1999，SHA-256 与 GitHub Release sidecar 一致。试用卡片另以 420×760 快照核对，正文和状态各只出现一次，两按钮同排。
- 视觉结论只代表布局、层级、可读性和交互反馈通过；不升级为浏览器宿主、代理出口或 GPS 运行时通过。

