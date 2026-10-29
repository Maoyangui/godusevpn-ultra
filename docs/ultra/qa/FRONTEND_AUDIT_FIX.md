# 前端、授权会话与扩展审计修复

日期：2026-10-19。范围仅为 Ultra 副本。代码与合成测试结果不代替真实 UAC、浏览器策略和流量矩阵验收；本记录不关闭原完整审计报告中的实机待验项。

## 问题与实现

| 原问题 | 本轮处理 | 证据与剩余边界 |
|---|---|---|
| A02 旧证据失效 | 桌面策略、程序规则、检测与扩展读取失败后标记历史结果/当前未确认，禁止基于旧结果写入或导出；焦点返回、隐私操作及到期使证据失效。网络摘要改为有界的新鲜只读 IPC，不再复用 GUI 缓存。 | DOM 成功→失败→恢复；服务读取 fake；每个来源有完成时间。没有全局监视其他软件，焦点返回后须手动重读；仍不能把配置读回当流量实测。 |
| A03 管理员重新授权 | 独立 `PrivacyAuthorize` 与状态接口；授权单飞；取消/不可用/失联明确区分。失联客户端清除，只允许用户再次授权。写入失败仅追加有界只读探测，绝不重试写入。 | helper fake 包含重复点击、已连接、1223 取消、关闭界面后的迟到结果、普通拒绝与死连接。真实同账号/替代管理员/UAC/崩溃组合待验。 |
| A04 扩展并发恢复记录 | 一个 MV3 本地后台统一串行执行读取、应用、撤销；写浏览器前持久化 `applying`，撤销前记 `restoring`。异常保留记录。缺失记录但浏览器确认由本扩展控制时，可显式清除自己的覆盖层。 | 两面板异项与交错恢复 mock；写盘前失败不修改浏览器；写后记录失败保留可恢复状态；模拟后台重启可恢复。后台中止的实际生命周期仍需浏览器实测。 |
| A05 跨页确认 | 页面/操作世代检查，导航、标签、草稿变化取消待确认。全局隐私写入忙状态独立于页面，已经提交的操作完成后使相关页面证据失效。 | DOM 跨页后旧确认不会调用 Apply；已经开始的结果不被描述成回滚。不调用隧道生命周期。原导航额外关闭对话框由根任务维护。 |
| A08 广播范围 | 改称“限制组播和有限广播”，明确不含定向广播、单播发现、蓝牙和系统代发。 | 文案与后端固定地址条件一致。没有新增范围或声称实测。 |
| A11 检测导出 | 后端为一次 GUI 读取创建十分钟内存快照。选全量/当前筛选，默认排除时区、地区与观察身份。后端从缓存挑选合法 finding，不接收任意前端报告；预览与保存使用同一字节串。 | Go fake 覆盖字段选择、外来 ID、源变化、取消、过期、保存框期间失效、消费 token；真实 TEMP 私有文件/no-overwrite 回归通过。保存目的地必须可确认本地且非重定向，失败不另选路径。 |
| A14 成功误报错误 | 安装/移除成功采用中性结果提示，范围 warning 保留在说明；相同程序路径和模式的重复草稿禁用提交。 | DOM 检查成功 toast 不带错误类型。过滤器存在仍只称安装证据，不能称所有路径受保护。 |
| A18 文件结果范围 | 检测模型由后端作者改为“本报告未包含文件检查，工具检查单独查看”。图片工具结果不会自动混入检测导出。 | 源码核对；未增加图片路径或元数据自动收集。 |
| A19 层次与帮助 | 浏览器策略归入浏览器标签；卡片正文保留用途、状态和动作，范围/证据/恢复放详情；检测先呈现可读信息和未知，再其他结果。帮助名称含具体操作；长状态允许折行。 | 桌面/扩展动态控件帮助 DOM 检查；420/900 深浅主题像素查看。帮助仍按原明确要求保留，每项均可键盘访问。 |
| U03 错误处理入口 | 浏览器 finding 转到隐私中心浏览器标签和实际扩展安装说明，不再打开 Windows 默认应用页。 | DOM 断言不调用 `PrivacyOpenSettings('browser')`。未假称自动打开/连接真实浏览器配置。 |
| U04 扩展帮助缺项 | 标签、动态确认/取消、所有输入输出框、模式选择与 launcher 加具体帮助，支持 hover/focus/click/Esc/outside。 | 独立扩展和 launcher DOM 测试，包含确认区域及输入后的状态。 |
| U06 权限模式选择 | `contentSettings` 按正式 API 提供模式草稿与确认；JavaScript 仅 allow/block，其余当前列出类型为 allow/ask/block。摄像头/麦克风 allow 必须明确 HTTPS 原点。改变来源替换本扩展同类规则，预览中明确说明。 | 非法类型/来源在任何写入前拒绝；逐 API mock 断言；仅正常浏览配置，不扩展成原生应用或所有站点保证。企业、站点例外及实际浏览器版本待验。 |

## 新接口与数据边界

- `PrivacyHelperStatus()` 返回状态；`PrivacyAuthorize()` 明确请求固定独立管理员助手，不提升 WebView。
- `PrivacyDetectionSnapshot()` 返回 `{snapshotId,expiresAt,report}`；标识只在本次 App 内存保存，退出或授权/隐私写操作会失效。
- `PrivacyPreviewDetectionExport({snapshotId,scope,findingIds,includeEnvironment,includeObserver})` 返回 `{previewId,content,expiresAt,count}`。
- `PrivacyExportDetection(previewId)` 保存该预览的确切字节；空返回表示取消。前端不展示/持久化导出路径，成功消费预览，失败保留或明确拒绝失效预览。
- finding 的 `source` 和 `detectedAt` 表示本次固定来源读取完成时间，分 policies/firewall/network/facts；逐规则与 DNS/日志摘要也映射到来源。范围说明、不支持或无完成时间的来源不使用报告生成时间冒充采集时间。
- GUI 服务摘要只映射连接/模式/guard 状态、IPv6/LAN/防直连布尔值及经过模型白名单的 DNS 模式、日志级别/天数；不复制 DNS 地址、订阅、profile 或完整设置。schema 缺失时设置摘要保持未知。
- 快照不能证明外部软件之后没有改变配置。没有自动在线核验、STUN、硬件序列号或真实 IP 收集；GUI 内嵌 WebView 自测仍明确不能替代真实浏览器。
- 扩展后台没有网页内容脚本或主机权限。只持久化设置管理记录与用户主动填写的站点原点，不收集浏览历史、指纹或本地自测结果；来源是设置恢复所需数据。内容 API 不提供统一控制权，故只称样本读回与清除确认。

## 运行的定向检查

均使用当前已有 Node/Go/Edge 运行时；无真实注册表、UAC、VPN、浏览器设置写入或真实扩展加载。

```text
go test ./cmd/godusevpn ./internal/detection -count=1
go vet ./cmd/godusevpn ./internal/detection
node browser-extension/tests.cjs
node browser-extension/coordinator-tests.cjs
node browser-extension/ui-tests.cjs <既有 jsdom 模块>
node browser-extension/panel-tests.cjs <既有 jsdom 模块>
node docs/ultra/qa/privacy-audit-qa.cjs <既有 Playwright 模块> <Edge 路径> [输出目录]
```

像素脚本只以固定 mock 填充 bridge 和扩展 runtime 消息，在路由中提供本地源文件并拒绝其他网页请求。浏览器后台联网禁用参数只是测试减噪措施，不能据此宣称整个浏览器进程绝对零外联。记录中 `blockedRequests=[]` 仅指该浏览器上下文的页面路由。

结果：四个 viewport/theme 配置共 28 张；页面异常为零、被路由拦截的非 mock 请求为零。对 420 宽检测、导出、系统、网络、扩展和权限确认以及 900 深色检测亲眼查看，未见横向溢出或关键输入遮挡。截图为固定合成数据，不是用户真实环境。路径为 `docs/ultra/screenshots/audit-fixed-*`，机器可读结果为该目录中的 `privacy-audit-qa-results.json`。

真实 UAC、断联进程生命周期、扩展加载与持久化、企业策略覆盖和目标站点权限矩阵仍需隔离环境单独确认。本批没有构建或发布产品。

独立源码互审：`app_firewall` 对授权 single-flight/迟到结果、失败写仅一次与只读探针、快照预览保存重验及消费、扩展后台队列与 applying/restoring 恢复记录做只读交叉审阅，未发现新增确认阻塞；明确未把源码/fake 计为真实 MV3 生命周期或 UAC 通过。

API 模式依据：[Chrome contentSettings 官方接口](https://developer.chrome.com/docs/extensions/reference/api/contentSettings)。摄像头/麦克风不允许全站 allow；JavaScript 不支持 ask；其余列出模式按各类型定义提供，运行时仍检查 API 存在性与错误。

测试期间发生一次工具运行时自动安装偏差，已立即报告根任务并由根任务告知用户；详见 [TOOL_RUNTIME_DEVIATION.md](TOOL_RUNTIME_DEVIATION.md)。后续未使用系统 Python launcher，也未擅自卸载全局组件。

## 2026-10-21 能力边界与不可修改项复核

用户反馈“有些功能无法关闭、文案空泛、部分操作不可用”。复核确认这是能力边界被误读，不应通过添加虚假开关解决：

- Windows 机器策略只有在支持的版本、管理员助手已授权、备份健康且没有更高优先级策略时才显示“启用此项”。策略值已经匹配但不是本应用写入时，界面显示“策略匹配，待运行验证”，明确不会提供恢复按钮或覆盖外部设置。
- `manual`、`unsupported`、`unknown`、`policy_conflict`、`recovery_pending`、`external_change` 现在在动作区下方给出短提示，分别说明需要系统设置、当前接口不支持、无法确认、被更高策略覆盖、先恢复未完成事务或先处理外部修改。旧报告和未知读回不能提交写入。
- “能力与引导”卡片增加“应用无法修改／仅提供说明／仅覆盖部分范围”的状态提示；设置入口统一称为“打开系统设置”，避免误以为点击后已经修改。
- 状态徽章改为短句：`策略匹配，待运行验证`、`读回匹配`、`未启用`、`仅覆盖部分范围` 等，不再把配置回读写成“已保护”。

本轮仅修改 Ultra 副本的静态前端文案和提示，不改变原项目、不新增系统写入、不改变隧道生命周期。`browser-extension/ui-tests.cjs`、`docs/ultra/qa/audit-qa.cjs`、`docs/ultra/qa/time-qa.cjs` 均通过；这些仍是 mock/UI 回归，真实 Windows、UAC、浏览器策略和网络效果需按候选验收记录。

同时修正输入个性化的 Windows 设置入口：`privacy-speech` 现在打开 `ms-settings:privacy-speechtyping`；旧 URI 在当前 Windows 10/11 上不是有效页面，已改为可用的官方页面标识。

## 2026-10-21 用户反馈复核：动作名称与能力边界

本次仅改 Ultra 副本：

- 桌面隐私策略按钮由“启用此项”改为“应用并读回”，已管理项目显示“重新应用并读回”，避免把写入注册表策略误说成运行时保护已经完成；页签“系统加固”改为“系统设置”，首屏明确其余项目只提供检查、边界说明和 Windows 入口。
- `available` 状态徽章由“可启用”改为“可应用”，与“读回匹配，待运行验证”区分；它只表示当前目录、权限和备份前提允许提交，不代表浏览器或系统功能已经接受策略。
- 扩展的摄像头/麦克风来源输入改为“允许必须填写；拒绝/询问可留空”，避免把空来源误解为可以全站允许。内容权限详情明确只返回单一来源的抽样值；网页例外、iframe 和 Worker 仍需单独核对。普通隐私策略动作同样改为“应用并读回”。
- 摄像头/麦克风在“允许”模式下会先在界面校验 HTTPS 原点；空值、非 HTTPS、含凭据/路径/通配符时直接禁用应用按钮并提示原因，避免把一个必然失败的提交暴露成可用操作。
- `AllowInputPersonalization` 的标题和说明改为“关闭输入个性化”，明确它不等于关闭 Windows 在线语音识别；第三方应用自行上传仍不受此策略统一限制。兼容 ID `windows-online-speech` 保留，避免旧 journal 和机器 helper 失配。

静态验证：`go test ./internal/privacy ./internal/detection ./internal/privacyhost` 通过；`node --check` 检查桌面隐私页和扩展面板通过；扩展核心回归与队列回归通过。当前宿主没有 `jsdom`，因此 panel/ui DOM 脚本未在本批重复执行；既有固定 mock/UI 结果不能代替真实浏览器、UAC 或网络验收。
