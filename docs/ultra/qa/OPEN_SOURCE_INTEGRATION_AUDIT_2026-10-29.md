# 开源工具与能力集成全量审计（2026-10-29）

本报告区分“源码实际调用”“只完成缓存/校验”“候选/未集成”和“只能检测或引导”。纯测试、静态包审计和 UI mock 不计作真实运行时集成。

## 已接入并有实际应用入口

| 组件/能力 | 许可证/来源 | 实际入口 | 当前证据 | 结论 |
|---|---|---|---|---|
| sing-box 代理内核 v1.14.0 | GPL-3.0-or-later，SagerNet | `internal/core/core.go`、`mobile/engine.go` | 内嵌 Box 生命周期、出站、TUN、测速和 Android VpnService 平台适配；Go 测试通过 | 已接入；Windows/Android 真实流量仍需独立 VM/设备回归 |
| WireGuard Windows WFP 代码 | MIT，wireguard-windows 衍生代码 | `internal/netmode/wfp` | guard/规则生命周期和纯测试 | 已接入；真实 BFE、IPv4/IPv6、回环、重启和断开恢复仍待实机 |
| ExifTool 13.59 | GPL-3.0 + 随附 Perl 许可 | `internal/metadatatool`、`Prepare.ps1`、GUI 元数据页 | 固定源码提交、归档 SHA、1501 文件清单、运行时 runner | 已接入；下载源为固定 SHA 的 Oliver Betz 镜像，未提供发布签名证明 |
| WebView2 SDK 1.0.4314.35 | Microsoft SDK 许可，不是开源内核 | `browser-host/BrowserForm.cs`、`Build.ps1` | 独立 WinForms host 可编译，SDK/Loader/许可证打包 | 源码集成完成；真实普通用户/UAC/代理/VM 运行仍待验收 |
| sevenzip 及提取依赖 | Go 模块许可证，许可证文件在 `internal/browserengine/third_party` | `internal/browserengine/store.go` | Chrome/Firefox 归档解包、路径/成员/版本/树校验 | 已接入下载校验链；不等于浏览器可启动 |

## 仅完成缓存、校验或测试边界

| 能力 | 现状 | 不应显示的结论 |
|---|---|---|
| Chrome for Testing / Firefox 多版本 | `internal/browserengine` 读取官方目录、下载、解包、SHA/版本/完整树检查、删除库存；Windows 普通构建可进入受控 native Start | 不能把缓存校验当成真实浏览器/代理/流量通过；仍需 VM 证据 |
| Chrome CDP / Firefox BiDi | 代码位于 native host/controller，纯协议和 mock 测试存在；Windows 普通构建可进入同一受控 Start | 不能称为真实浏览器运行、代理出口、WebRTC/WebGPU/Worker/OOPIF 通过 |
| 独立 WebView2 host | 代码与包载荷存在；Start 仍要求完整安装、WebView2 Runtime 与严格连接 | 不能把静态编译当成用户可用浏览器；真实运行仍需 VM 证据 |
| location-consumer | C# WinRT `Geolocator` 消费者读取位置和权限 | 这是位置读取/检测 helper，不是 GPS 写入或模拟后端 |
| locationadapter | 来源、版本、许可证、SHA、备份、CAS、恢复边界和 fake backend | 不能称为 Windows Location Provider 已接入 |

## 明确未集成的候选

- **CEF/Chromium**：只有计划和许可证审计要求；仓库没有 CEF 构建产物、沙箱、签名、升级或运行入口。
- **uBlock Origin**：只有候选文档；没有扩展包、安装流程、权限/规则来源/更新状态入口。
- **locsim**：已审计并排除；Windows 路径无法形成系统位置写入、独立读回和可靠恢复闭环。
- **windows-rs GeolocationProvider**：仅为下一步真实后端候选，尚未写入应用或安装包。
- **iOS/macOS 原生浏览器内核**：用户要求的是浏览器环境配置；当前 Windows 应用的 Android/iOS/macOS/Windows 是网页环境预设，不是四个平台原生内核。

## 供应链与许可证缺口

- `NOTICE` 记录了主要组件，但没有完整 SPDX/SBOM 或全部 Go 间接依赖的逐项许可证/漏洞报告。
- `NOTICE` 所写的 `third-party-notices/browserengine` 是发行包路径；源码中的实际许可证文件在 `internal/browserengine/third_party`，由 `build-ultra.ps1` 构建时复制。
- ExifTool 固定 SHA 能证明文件一致性，不能证明 Oliver Betz 镜像的发布者身份；应在发布审计中保留该限制。
- Chrome 官方 feed 不提供归档 digest；当前实现记录 HTTPS 来源和本地归档/树哈希，不能把它描述成发布者签名验证。

## UI 与逻辑审计结论

- UI 文案多数已明确写出“下载/校验不等于运行”“需组件”“仅说明”“待运行验收”，这符合禁止充数要求。
- 已修正内核管理页的启动门控：缓存未完整校验的版本继续显示“待运行验收”；Windows 普通构建只有在树/版本校验通过并进入受控 native Start 时开放，启动、关闭和代理打开动作仍受严格连接与会话状态控制。对应 `web/dist/privacy.js` 的 DOM 回归已通过。
- 代理测试结果只证明一次出口探测，不证明浏览器实际导航路径；UI 已有该提示，必须保留。
- GPS、CEF、uBlock、真实 Chrome/Firefox 运行链都应继续显示为未完成/需组件，不能改成“已启用”。

结论：sing-box、WFP/WireGuard、ExifTool 和 WebView2 host 的源码接入存在；CEF、uBlock、真实 GPS 模拟仍未接入。Chrome/Firefox 多版本已进入普通 Windows 的受控 native Start，但真实浏览器、代理和流量仍需 VM 证据。启动门控按完整校验与会话状态开放，避免把缓存校验当成运行通过；许可证清单尚未形成完整 SPDX/SBOM。


## 本轮复核回执

- 启动门控修正后，`browser-engine-ui.cjs`、`browser-profile-ui.cjs`、`privacy-centers-ui.cjs`、`browser-extension/ui-tests.cjs` 均通过。
- Edge 156 像素回归覆盖 4 个视口/主题、32 张截图，0 errors、0 outbound；诊断视觉回归覆盖 4 个组合、12 张截图，0 errors、0 blocked。
- `node --check web/dist/privacy.js` 与 `go test ./web` 通过。以上仍是隔离 UI/静态回归，不替代真实浏览器、WFP、代理出口或 GPS 运行时证据。

## 本轮重建与审计产物

- 候选包：`0.01-beta-qa.12.12`，payload 1993 个，ZIP 成员 1994 个，ZIP SHA-256：`7bc00697e43f398fcdde77d45b115776b9052f35d158dc99a7fe9b6be1f8a73a`。
- `go test -count=1 -tags with_quic,with_utls,with_clash_api,with_gvisor ./...` 通过；`node --check web/dist/privacy.js`、包审计和 `git diff --check` 通过。
- 视觉回归：pixel 32 张、diagnostic 12 张、privacy audit 92 张、audit 20 张、time 25 张、brand 6 组；均为 0 errors、0 outbound/blocked、0 layoutIssues。
- 本轮仍没有执行真实浏览器宿主、CEF、uBlock、GPS 写入、WFP/代理出口或真实流量，因此这些项目的状态保持未完成或待实机验收。
- QA12.15 acceptance 构建已重新生成并通过包审计（1995 payload / 1996 ZIP members，SHA-256 `7d0ad7e21d74673757250ec6f5ddfb44d3987bde7c3bd71ab337a623ac0b4f3f`）；该构建已开放受控宿主入口，但尚未在 VM 中安装运行，因此仍不能替代真实浏览器/代理/流量验收。

## 2026-10-30 真实 VM GUI/宿主复核

- `0.01-beta-qa.12.13` 在一次性 Win10 21H2 / build 19044 VM 上完成实际安装、服务/CLI/隐私报告回读；普通用户 Bastion 桌面两次打开 GUI，隐私中心、浏览器保护、GPU/硬件预览完成视觉核验。
- VM 回读到 WebView2 `156.0.4314.50` 与 browser-host 7 个文件，但浏览器设置页仍明确显示“需完整安装”，启动按钮为“启动尚待验收”；本批将真实宿主窗口记为 `LIMITED`，没有把 acceptance 构建或静态文件存在当成 Chrome/Firefox/代理运行通过。
- 临时 VM/Bastion/网络资源/gist 已清理；完整边界和证据见 [QA12.13](QA12_13_BROWSER_HOST_VM_2026-10-30.md)。
- QA12.13 包审计：1993 payload/1994 ZIP members，ZIP SHA-256 63e2acec6f4bfee7985f756f1b1545ec3a2d21de93e7300660d37537b2a584a3；PE、前端和清单校验通过，untimeVersionVerified=false 按审计边界保留。

## 2026-10-30 GPS 运行时门禁补充

- `internal/locationadapter.RuntimeSupportStatus` 现在只读取 Windows build，并用官方 `GeolocationProvider` 文档门槛 23504 做本地门禁；Win10/23H2 明确返回 build 不足，达到门槛也只返回“可能存在”。
- 隐私中心系统位置卡显示该门禁的原因与剩余条件；没有把 build 检查当成 GPS 写入、LAF、runFullTrust、会话所有权或原子恢复能力。
