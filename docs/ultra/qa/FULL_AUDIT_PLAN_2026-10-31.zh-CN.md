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

## 统一编辑器阶段更新（99d73c4）

本轮把旧浏览器配置卡的运行控件合并到唯一的
`#privacy-browser-environment-editor`。旧内核、版本、UA、视口、GPU、WebGPU、CPU、保存、启动和关闭控件现在作为同一编辑器的基础配置与统一操作栏使用；v2 区域只保留元数据、Cookie/标签页、代理策略、指纹计划、高级字段和运行时读回，不再渲染第二套 GPU/CPU/WebGPU 输入。

新增核验包括：

- DOM 只渲染一个浏览器编辑器和一个统一操作栏；核心控件 ID 均唯一；
- 浏览器家族标签与实际内核同时展示，避免把 SunBrowser/FlowerBrowser 标签伪装成真实内核；
- 配置头部展示状态、配置 ID、Revision、保存时间和读回时间；冲突保留本地草稿；
- Cookie 解析/清空按钮、启动 URL 添加/删除/清空、数量和格式状态；
- 每个 v2 字段显示配置已保存、部分支持、运行时待验证或当前宿主不支持；
- 不支持字段启动前阻断，预览不作为运行时证据；
- `browser-saved-profiles-ui.cjs` 覆盖统一编辑器、Cookie、URL 操作、保存/载入/CAS 和无重复核心控件。

仍未宣称完成的项目：真实 Cookie 导入、启动标签页、完整 WebRTC/噪音/字体/地理位置、RAM/MAC/设备名、DNT/端口扫描/TLS/同步、OOPIF/Worker/WebGPU 全上下文读回，以及 ARM64 和真实 Windows VPS 矩阵。这些继续按“运行时待验证”“当前宿主不支持”或“需要原生适配器”分类。

## 最终复核追加（968c618，2026-10-31）

本轮只修改 `godusevpn-ultra`，没有触碰 `m-ui`、`godusevpn` 或 `liumeiti.vip`。当前 `main` 与 `codex/windows-privacy` 均指向 `0ae95e3`，工作树干净。

- 统一编辑器状态源已修正：浏览器家族、实际内核、GPU/WebGL、WebGPU、保存代理策略和实际连接分别显示；保存值与运行时值冲突时启动被阻断。
- 指纹种子必须先生成实际配置预览，未形成预览的种子不能直接启动；运行时读回默认保持 `partial/unknown`，不会冒充完整验证。
- `site/docs.html` 与 `site/demo/index.html` 已改为 Ultra 所有权的 canonical、OG、GitHub、Release 和 Issues 链接，并明确基础客户端演示不等于原生运行时证据。
- 本地测试：`go test -tags with_clash_api ./...`、`go vet`、`node --check`、静态站点构建、演示页资源扫描、8 组浏览器 UI fixture、Edge 视觉回归均通过；视觉 QA 结果为 20/92/12 张截图，错误、阻断请求和布局问题均为 0。
- GitHub Actions：Ultra Pages 与 Windows Beta 在 `968c618` 均通过；Windows Beta 完成包校验、Go 回归、浏览器 UI、Edge 视觉、打包和最终 payload 审计。
- v0.02 Release 包已用 `968c618` 重新构建并替换旧包，当前 ZIP SHA-256 为 `b2d5835fc7dd3cd8e63e412f9394764b0df0e1f47ba1ea73bea5798300156b8f`；`ULTRA_PACKAGE.json` 为 `sourceDirty=false`。
- GitHub Pages、Ultra 文档/演示页和五种 GodUseVPN 首页语言入口均可访问；正式首页在无查询参数的浏览器页面中可见 Ultra 入口，入口链接到 Ultra 展示页和节点订阅页，控制台无错误且无横向溢出。
- Trial Worker 生产 `/healthz` 和 `/status` 正常，计划为 `free`；当前 `panelConfigured=false`、`subscriptionSource=static`。真实面板 API 前缀已固定为用户提供的地址，服务端令牌尚未由管理页写入，因此不能把面板刷新闭环写成完成。

## 原有管理后台集成（Trial Worker 7680be35）

本轮没有新增第二个试用 Worker，也没有修改 `m-ui`、`godusevpn` 或商业站仓库。现有
`godusevpn-trial-api` 继续承载 `/app/api/trial/*`，并接管原有
`https://godusevpn.org/manage/` 的同域路由：管理页保留原页面和登录流程，设置面板追加 Ultra 免费试用卡片。

- 管理页可设置试用开关、面板套餐名（默认 `free`）和“购买节点”跳转地址；关闭只停止新领取，不删除历史记录。
- 管理页可一次性填写面板令牌；令牌不回显、不返回客户端，使用已有 HMAC 密钥派生 AES-GCM 密钥后加密存入现有 D1；留空保存会保留原令牌。
- 设置卡随管理端语言选择提供简体中文、繁體中文、英文和西班牙文文案，令牌输入始终为密码字段。
- 面板地址固定为用户提供的 `https://hk.joinvip.vip:2053/ad/api/v1`，面板请求仍由服务端发出，客户端只收到订阅 URL。
- 生产管理页注入检查：原始页面 35018 字节，集成页 38435 字节，仅增加 Ultra 卡片；未登录接口 `/manage/ultra-settings` 返回 401。
- 生产健康检查：`/app/api/trial/healthz` 返回 200、D1/HMAC 正常、计划 `free`；当前仍为 `panelConfigured=false`，必须由管理员在管理页输入令牌后才能取得真实面板 claim/refresh 证据。
- 本地加密链路回归：管理页保存令牌返回 200；D1 中没有明文令牌；面板模拟调用收到 `Bearer` 令牌，同一设备首次 `claimed`、第二次 `refreshed`，轮换后的订阅 URL 替换成功。
- Worker 本地 `node --check src/index.js` 与 6 项测试全部通过；部署版本为 `7680be35-2c12-4ef7-b685-ac85169d936c`。

以下仍是未完成或未取得真实证据的硬性项目：真实 Cookie 导入、启动标签页、完整 WebRTC/Canvas/Audio/媒体/ClientRects/SpeechVoices 噪音、字体/地理位置/DNT/端口扫描/TLS/同步适配器、OOPIF/Worker 全量读回、真实普通用户 Chrome/Firefox 代理出口、Win10 1809/19044 与 Win11 x64/ARM64 VPS 矩阵、GPS provider 写入/恢复、D03/N08、CEF/uBlock，以及面板 token 注入后的真实 claim/refresh。当前界面会保持“待验证/部分支持/不支持/未知”，不以静态页面或模拟数据替代这些证据。

## Home Bridge 查询路由修复（独立 Worker 64fe0473 → 8b07b778）

独立 `godusevpn-ultra-home-bridge` 增加语言首页通配路由和根路径查询路由，修复 `?lang=zh-Hans` 等查询 URL 未经过 Worker 的问题。Worker 对非首页路径回源到原请求源，避免接管管理端、订阅页、文章、资源和 API。

真实 HTTP 回归：

- 根域与五种语言首页的无参数和语言查询 URL均返回 Ultra 入口；
- `/subscription/`、`/manage/`、文章页保持原响应；
- `/app/api/trial/status` 仍返回 200，`/api/admin/session` 仍按未登录返回 401；
- 中文根域查询 URL在 CUA 移动视口下显示 Ultra 入口，`scrollWidth=625`、`innerWidth=640`，无控制台错误。
