# Ultra 全量审计与合并计划

本计划对应接手的两段对话：隐私保护评估、浏览器环境与代理能力、隐私/检测中心、GodUseVPN 试用后台、页面入口、视觉验收、Azure/VPS 实测、Win10/Win11/ARM 兼容、发布与清理。

## 仓库边界

| 目标 | 允许的代码位置 | 当前状态 |
|---|---|---|
| Ultra 客户端、Windows 构建、GitHub Pages | `godusevpn-ultra` | 保留，主线持续开发 |
| `godusevpn.org` 免费试用后台 | Cloudflare Pages/Worker + GodUseVPN D1 | 线上 Worker `b497bed8-9017-4ce3-bf34-640850a1fe93` |
| GodUseVPN 原客户端/面板仓库 | `godusevpn`、`m-ui` | 已撤回误放的 Ultra 网站桥接/Worker 提交；更早既有改动保留 |
| `liumeiti.vip` | 禁止改动 | 已把线上 Vercel 部署切回 `87ec525`；`/app/api/trial/status` 返回 404；原站既有 1 元测试商品保留 |

## 误提交全量核对（2026-10-31）

| 仓库 | 核对结果 | 处理与证据 |
|---|---|---|
| `m-ui` | 本轮误写入的 Ultra 试用提交为 `8f73d34`、`caadacd`、`c3d7103` | 远端 `main=4a1b46d`；三条提交及其 Actions 运行已移除；`4a1b46d` 及更早历史保留 |
| `godusevpn` | 本轮误写入的 Ultra 桥接/Worker 提交不在远端主分支 | 远端 `master=f89bbc2`；仅保留原有客户端与站点历史 |
| `liumeiti.vip` | 本轮误写入的试用网关提交不在远端主分支，线上旧部署曾残留 | 远端 `main=87ec525`；Vercel production deployment `dpl_8YLVgFFXRCSBmoChMNecxSRKMV1h` 已验证；试用 API 404 |

## Ultra 客户端服务器调用审计

客户端试用调用固定在 `internal/trial/client.go` 的
`https://godusevpn.org/app/api/trial`，只发送不可逆设备摘要到
`/status` 与 `/claim`；面板令牌、D1 凭据和节点面板地址不进入客户端。购买地址由后端返回，缺省回落到
`https://godusevpn.org/subscription/`。当前没有客户端可编辑服务器地址的入口，这是为了避免把后台密钥或可替换的管理端点交给用户配置；若将来需要切换服务，必须由发行版构建配置或受控更新通道改变，并补齐签名与回滚测试。

## 逐项判定

| 模块 | 当前判定 | 直接证据 | 合并前条件 |
|---|---|---|---|
| 试用 Worker | 已上线 | status/claim/409/CORS/healthz/D1 真实 HTTP 回归 | 每次 Worker 修改后重跑同一矩阵 |
| 客户端试用替换 | 应用层完成 | `TrialClaim` + `UpsertTrialProfile`，短用户名与来源备注 | 两条轮换 URL 真实端到端替换 |
| 浏览器环境/UA | 应用层完成，运行时待验 | DOM/UI fixture、Win10/11 平台值与手动 UA 测试 | Chrome/Firefox 真实窗口、HTTP、navigator、CH 读回 |
| 内核目录/删除 | 应用层完成，运行时待验 | 下载、树校验、删除、库存重读 fixture | x64/ARM VM 实际安装/删除/磁盘读回 |
| 浏览器代理 | probe/relay 完成，出口待验 | SOCKS/HTTP/HTTPS 表单、认证、probe、relay fixture | 普通用户窗口真实出口、断开无直连回退 |
| 隐私中心 | 应用层与视觉通过，系统写入待验 | Go/UI/Edge 像素 QA | Win10/Win11 逐项写入、读回、恢复、冲突 |
| 检测中心 | 应用层与视觉通过，真实快照待验 | 筛选、快照、导出、帮助覆盖、像素 QA | VM 真实报告和脱敏导出核对 |
| 元数据/剪贴板 | 有限实测 | ExifTool GUI、条件清理、Go/DOM QA | 实拍图片、竞争剪贴板、退出/同步边界 |
| GPS | 未完成真实写入 | Win10 provider 门禁保持不可用 | Win11 x64/ARM provider、独立消费者、恢复读回 |
| D03 DPAPI | 未完成 | 当前只有 ACL/边界说明 | 用户/SYSTEM 加密、迁移、回滚、明文残留测试 |
| N08 首次联网 | 未完成 | 当前没有完整事件/目的地决策模型 | 只读事件、目标身份、超时和默认拒绝测试 |
| CEF/uBlock | 未集成 | 只有审计/候选记录 | 许可证、更新、权限和可运行集成测试 |
| Windows 版本 | 17763/19044 有限证据 | 安装/服务/CLI/部分 GUI 证据 | 1809 GUI、22H2、Win11、ARM64 同一矩阵 |
| 视觉 | 当前发现生产首页桥接对比度不足；Ultra Pages 需重新逐帧审查 | 截图、DOM、像素 QA | 首页桥接和 Pages 每个断点截图无裁切/低对比/假状态 |

## 可合并批次

1. **视觉修正批次**：修正 Cloudflare 首页桥接的深色背景/标题/正文对比度；重截桌面、窄屏、问号 tooltip；生产只部署 Worker，不向网站仓库提交。
2. **Pages 真实性批次**：删除或标记静态演示百分比，校正浏览器能力文案，补充明确的“示例/待实测”标识；桌面、平板、420px、深浅色逐帧截图。
3. **会话清理契约批次**：让 WebView2 与 Chrome/Firefox native 的 `exitCleanup` JSON、宿主处理、状态和目录读回一致；删除失败保持 `closing_or_unknown`。
4. **ARM64 VM 批次**：使用 Windows 11 Arm64 24H2 镜像、标准安全类型和可用 ARM 规格；完成安装、服务、CLI、隐私、浏览器和代理矩阵。
5. **真实浏览器批次**：x64 Win10 19044、Win10 17763、Win11 x64、Win11 ARM64；Chrome/Firefox 各一版本，记录窗口、导航、UA/CH、代理出口、WebRTC/WebGPU/OOPIF/Worker、退出清理。
6. **GPS 批次**：Win10 只验证门禁/无写入/说明；Win11 x64/ARM 实测 provider、独立消费者、恢复和外部冲突。
7. **剩余工程批次**：D03 DPAPI、N08 目标白名单、CEF/uBlock 逐项独立设计、实现、测试和文档，不以入口或说明代替集成。
8. **最终发布批次**：Go test/vet、全部 UI fixture、Edge 像素 QA、构建包 SHA、README、中英文发布页、Cloudflare 线上回归、Azure 资源清单与删除核对。

任何批次只有在代码、测试、真实运行（适用时）和视觉证据都对应后才合并；DOM、mock、静态页面和旧包证据不会替代真实浏览器、代理或 GPS 证据。
