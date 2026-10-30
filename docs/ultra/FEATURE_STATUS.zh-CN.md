# 逐功能实现与验收台账

## 2026-10-30 当前状态（优先于下方历史记录）

- v0.02 已构建并发布预览包 [godusevpn-ultra-0.02-windows-amd64.zip](https://github.com/Maoyangui/godusevpn-ultra/releases/tag/v0.02)，包名全小写；发布标签 `v0.02` 固定在发布提交 `98249d7`，当前主线源码与文档已推进到 `b8e7cf9`，远程 ZIP SHA-256 为 `5f3d23a7d912429c244da080a734a172c6df1766a9a40198145dbaf55a1d1ff3`。下载包后仍必须以同名 `.sha256` sidecar 校验实际 ZIP。构建验证还修复了 Windows PowerShell BOM 导致的资源构建失败。
- Ultra 介绍页已发布到 [maoyangui.github.io/godusevpn-ultra](https://maoyangui.github.io/godusevpn-ultra/)，GodUseVPN 生产首页由 `godusevpn-ultra-home-bridge` Worker（版本 `7547a7ef-1edd-469c-ad90-38d19b4d0a84`）在下载区域追加 Ultra 入口、问号说明和订阅跳转；生产 HTML 复核为单个入口，按钮与 tooltip 可见。该桥接不修改 liumeiti.vip，也不覆盖 Pages 源站。
- `godusevpn.org/app/api/trial/*` 由 Cloudflare Worker `godusevpn-trial-api` 直接部署在 GodUseVPN 的 Cloudflare 项目中，当前版本 `b497bed8-9017-4ce3-bf34-640850a1fe93`，绑定 `godusevpn-commerce` D1。实测 status 200、claim 200 返回真实 `hk.joinvip.vip:2056` 订阅、同设备 409、再次 status 为 `already_claimed`、OPTIONS 204、非法设备 400、恶意 Origin 无允许跨域头；D1 记录短 `t-xxxxxxxx` 标识和“来源：佛跳墙内置免费试用；套餐：free”备注。公开前缀健康检查 `/app/api/trial/healthz` 返回 `backend=d1`、`backendConfigured=true`、`upstreamConfigured=false`。`TRIAL_ENABLED`、`TRIAL_PLAN`、`TRIAL_PURCHASE_URL` 由 Worker 配置控制，客户端不接触令牌；面板 API 的 `free` 套餐映射已在 GodUseVPN 后台确认通过。Worker 源码不属于 GitHub 仓库。
- Azure Win10 19044/17763 曾有安装、服务、CLI、隧道和出口的有限实机证据；当前 v0.02 预览包又在 19044 VM 完成包校验、固定目录安装、服务 Running/Auto/LocalSystem、CLI 版本/状态和隐私报告回读，详见 [v0.02 VM 复测](qa/QA_V002_VM_RETEST_2026-10-30.zh-CN.md)。普通用户 VM 仍未重新完成 Chrome/Firefox 原生窗口、导航和 GPS 写入读回。代码启动链存在不等于这些运行时能力已验收。
- 仍未关闭：真实 Chrome/Firefox 多版本窗口与代理出口矩阵、WebRTC/WebGPU/OOPIF/Worker 读回、Windows GPS provider 写入/读回、CEF/uBlock 运行集成、跨 Windows 版本完整矩阵，以及 D03/N08 等原台账范围。不得用 DOM、mock、纯 Go/C# 测试或旧包证据替代这些范围。

## 2026-10-27 当前状态（优先于下方历史记录）

> 历史快照：本节记录 10 月 27 日当时的候选状态。当前公开候选、会话清理修复和视觉复核以本页顶部与 [最终审计](qa/FINAL_AUDIT_2026-10-30.zh-CN.md) 为准。

完整对话的用户要求与后续追加目标按 [H01–H37](HANDOVER_REQUIREMENTS_2026-10-26.zh-CN.md) 保留；早先 beta 批次不构成整体项目完成定义。qa.11 已在临时 VM 通过官方 QA9→QA11 升级、完整包与 Running / LocalSystem 读回；普通 Medium 无参数启动、实际取消 UAC 后保留 GUI，第二轮原始 Process handle 取得托盘正常退出码 0，见 [本次证据与边界](qa/QA11_OFFICIAL_UPGRADE_2026-10-27.md)。配置库另已通过 QA11 真实 GUI 的载入、重命名、另存新配置、取消删除与删除的有限往返及独立磁盘核验，见 [本批记录](qa/QA11_PROFILE_GUI_VM_2026-10-27.md)。普通用户代理 GUI 的单节点认证 SOCKS5 显式 probe、错误认证反馈与运行中 UAC 取消保留 GUI 已有 [有限实测](qa/QA11_PROXY_GUI_VM_2026-10-27.md)；元数据仍按其 QA9 有限证据判断。完整 GUI、接受授权路径与独立浏览器仍分别待验。本段是 10 月 27 日历史快照，当时的“私有冻结包尚未公开发布”不适用于当前 v0.02。产品内的 Android / iOS / macOS / Windows 指浏览器环境配置，不能据此宣称四种原生系统内核运行。

| 模块 | 当前实现证据 | 尚待完成 / 验收 |
|---|---|---|
| 下一候选 UI 与 CI | 本轮 [16 项纯 UI / 扩展回归](qa/QA10_PURE_UI_PREPARATION_2026-10-26.md) 全部通过；CI 含新 fixture 和剪贴板门槛。qa.11 含 Client/relay 空指针接口转换修复，已实际完成官方升级及本次取消 UAC 崩溃回归/托盘退出，原始句柄退出 0；64 字节 stderr 已审读，仅 WebView2 初始化成功日志 | 未运行托管 CI；其他提权授权路径、浏览器和整包 GUI 仍待各自验收 |
| 窗口操作 | qa.11 提供工作区/DPI 尺寸适配及 Windows 最大化/还原；本次实际观察 1041×720 工作区最大化、还原 420×704；对应纯几何/生命周期/DOM 检查已通过 | 仅本次桌面尺寸观察；多屏、其他 DPI/缩放、首次显示、重载和最小化完整矩阵仍待验，不外推为全部窗口组合通过 |
| 独立 WebView2 浏览器 | 原用户宿主；独立配置、地区/尺寸、四类环境、GPU 编辑；HTTP/navigator/CH 读回；下一候选加入规范生成 UA、手动/原生 UA 文本保持及网页 CPU 数量；624 项 C#、27 项 GPU JS、9 项 CPU JS 纯检查通过 | 真实运行时 UA/CH/CPU 多 frame 与 OOPIF、WebGPU、独立代理出口及普通用户/UAC组合；仍要求现有 Ultra 严格隧道。新增源码不回写 qa.9 |
| Chromium / Firefox 多版本 | 官方下载与完整树/版本校验；Windows amd64/arm64 普通构建进入受控 native Start，经过普通用户、代码租约、Job/WFP、SOCKS relay、CDP/BiDi 和运行时读回门禁；四类环境及 UA/CH 模型与删除库存已覆盖 | 真实 VM 浏览器矩阵、真实窗口/导航/代理出口、WebRTC/WebGPU/OOPIF/Worker 读回仍待实机；原生 GPU 名称覆盖和模板重用未完成，不能把启动入口存在算成全部隐私能力通过 |
| 网页 CPU 数量 | 下一候选 WebView2/Chrome 可选 1–64 个逻辑处理器，正式 CDP 命令及暂停 target、主文档/iframe 原生属性读回失败即拒绝；省略保留原生，Firefox 自定义请求明确拒绝；纯协议与 UI mock 通过 | 未改变物理 CPU、性能或 RAM；正式命令是否受实际版本支持、后续 OOPIF 与退出/断连仍需 VM。RAM/字体/Canvas/Audio/媒体等后端继续逐项待完成 |
| 浏览器配置交互与保存 | 五类表单、环境 / 内核 / UA 草稿与预览；Windows 10/11 快捷值 10.0.0/13.0.0 可改且保持手动 / 原生 UA；完整 revision CAS，存储不含代理 / 认证 / 启动授权；已有 fake/TEMP / DOM 和 24 身份组合 mock。QA11 普通 Medium 实际载入、仅重命名追加一帧、另存新 ID、取消删除及删除副本均通过 GUI 与独立只读磁盘核验；重命名旧字节前缀、原记录 / ACL / 同级名称哈希保持已实测，见 [证据](qa/QA11_PROFILE_GUI_VM_2026-10-27.md) | 本次只覆盖合成配置的正常路径；跨窗口冲突、并发、损坏 / 崩溃 / 短写 / 同步失败恢复和其他用户 / ACL 矩阵仍待验。完整 GUI receipt 未取得，receiptMatchesDisk=false；克隆全字段归一化等价性未验，canary=0；未启动浏览器或验证浏览数据删除 |
| 原生启动状态与取消 | 下一候选接 BeginLaunch、匹配 token 的 CancelStart 与 Status 轮询；准备中锁定编辑/新启动，取消请求不冒充完成，未知清理保留 owner 和检查/关闭入口；离页取消原 run，旧 DOM 不取消新 run；Go fake 并发及八类 [DOM 场景](qa/browser-start-ui.cjs) 通过，缺少新接口时保留旧同步调用兼容 | 不在冻结 qa.9；真实下载/进程/WFP 清理、取消与 ready 竞争、UI 退出和原生实例残留仍待 VM，不能用 mock 证明已经撤销规则 |
| 浏览器独立代理 | SOCKS5 / HTTP / HTTPS、免费 GeoIP；纯测试及 [生产模块 / 认证 relay VM 网络测试](qa/PRODUCT_PROXY_VM_2026-10-26.md) 通过。QA11 普通 Medium 手填单个认证 SOCKS5 后显式测试，出口与已知代理一致；改密码隐藏旧成功结果，错误认证实际失败，见 [GUI 记录](qa/QA11_PROXY_GUI_VM_2026-10-27.md) | 本批仅 probe，未启动浏览器；没有独立抓包证明无直连回退。恢复认证后旧错误提示残留已列 QA12 修复后重验；浏览器真实出口、HTTP / HTTPS 各代理类型、隧道 / WFP 仍待各自验收 |
| 元数据 | qa.9 普通 Medium 已安装 GUI 实际调用 ExifTool 13.59；两张合成 JPEG/PNG 的七类 EXIF 逐项及全选共 16 副本，独立源哈希/像素/类别选择后验全部通过；默认只显示数量、显式作者查看、取消保留预览和后端拒绝覆盖已实测，见 [真实 GUI 验收](qa/METADATA_REAL_GUI_VM_2026-10-27.md) | 仅两张 4×3 合成图片、七类 EXIF；XMP/IPTC/缩略图、实拍照片及异常矩阵未外推。qa.11 的 1501 个依赖成员与 qa.9 字节一致，仍不代表 qa.11 整包 GUI/UAC 已验收 |
| 秘密剪贴板 | 默认关闭，可选 5–600 秒；只清理本产品复制且序号 / owner / 内容哈希仍一致的当前内容；Go/DOM 回归已覆盖替换保留、取消和服务不可用 | 真实剪贴板竞争、退出、历史 / 云同步边界待 VM 验收；不清除云端或已被其他程序读走的副本 |
| GPS | 上游 locsim 审计、真实能力缺口记录、带 CAS / 独立恢复的事务适配器与纯测试 | 尚无真实 Windows GPS 后端；不能把自建缓存当成系统定位 |
| 安装 / 卸载 | qa.11 官方升级实际通过：固定包接收核对 1966 文件/17 目录/1965 payload；QA9 官方卸载后服务消失、完整旧树保留；冷 WinPS 5.1 默认 Source 安装后 qa.11 全 payload、Running/LocalSystem、oldFilesRetained/defaultSource=true 读回，见 [证据](qa/QA11_OFFICIAL_UPGRADE_2026-10-27.md) | 仅本次隔离 VM 官方升级；浏览器、网络效果及其他系统/权限组合继续分项验收。历史 QA9/QA10/QA11 包均不回写 |

QA9 普通用户保存、列举和载入的有限往返、单条独立磁盘证据及原取消崩溃保留在 [历史 GUI 观察](qa/QA9_ORDINARY_GUI_OBSERVATIONS_2026-10-26.md)。QA11 本次取消回归和正常退出已实际通过，但第一轮 180 秒存活没有退出码，第二轮才由原始句柄取得 0；stderr 64 字节已审读，仅一行 WebView2 环境创建成功日志。随后 QA11 配置库正常操作和原字节前缀通过了 [独立磁盘比较](qa/QA11_PROFILE_GUI_VM_2026-10-27.md)，并未伪造完整后端 receipt；与元数据结果一样，只覆盖各自范围，不代替完整存储异常、授权或浏览器验收。

模块对标、候选开源组件和许可见 [产品对标表](PRODUCT_BENCHMARKS_2026-10-26.zh-CN.md)。以下 10 月 19 日起的记录保留作历史证据，不能覆盖本节或当成新功能的当前完成状态。

基于 `PROJECT_PLAN.zh-CN.md`，日期 2026-10-19。代码实现、自动测试和实机效果分开验收。本台账不删除尚未实现的需求，也不把引导入口算成自动修改功能。

“代码+fake”表示实现及无副作用自动测试完成，仍需对目标 Windows/浏览器版本验收。“引导”表示可以解释范围/打开正式设置，但本版没有自动修改。“后续”表示仍需开发；不是系统不可能做到。

用户确认本次beta范围为修复并验收现有功能；首次联网询问、独立浏览身份及通用凭据加密等继续列路线图，不被偷偷删项或冒充本版交付。当前修复汇总见[A01–A21/C/U整改台账](AUDIT_RECHECK_SUMMARY.zh-CN.md)。VPS已有v1及qa.2有限只读/注册表恢复证据，qa.3固定文件传输/重建/组装也已完成；当前宿主已有新Edge配置的有限MV3设置往返。qa.3 产品只读命令、安装、普通用户、GUI/UAC、恢复和流量仍未验收，且 v1 安装预检在真实机上以 `SCM_unknown` fail-closed，不能继承旧包结果，也不能据这些测试宣称GUI/UAC、普通用户IPC或流量通过，见[候选验收](qa/BETA_CANDIDATE_QA.md)及[安装预检二审](qa/QA3_INSTALL_PREVIEW_SECOND_REVIEW.md)。

## 原有能力与新边界

| 要求 | 当前结果 | 后续核对 |
|---|---|---|
| 原目录不改 | 420 个原 tracked 文件 SHA-256 与基线一致，原 Git 状态干净 | 每版交付前重复检查 |
| 原隧道独立 | 原算法保留；仅 Windows 命名空间和默认端口/地址隔离；原核心回归通过 | 真实订阅和网络环境回归 |
| 附加保护失败 | 隐私桥接没有连接、断开、模式、DNS、路由、撤闸调用；启动默认不请求 UAC，首次机器策略或浏览器隔离操作前由用户显式授权。Win10 22H2 Pro 普通用户启动已在临时 VM 观察到无无关 UAC，见 [QA12.10 记录](qa/QA12_10_WIN10_STANDARD_GUI_2026-10-27.md) | UAC取消/助手崩溃/磁盘错误与真实隧道组合验收；Win10 21H2及其他版本仍待逐项 VM 回归 |
| 默认请求管理员 | 界面保持原用户；固定助手UAC、显式重新授权与single-flight；当前用户策略与机器策略隔离，失败写不重试 | 同账号/替代账号、会话退出与真实管道验证 |
| 恢复 | 写前备份/读回/CAS/损坏拒绝；独立原用户时区CLI；机器写及官方清理/卸载共用maintenance lease | 真多进程/abandoned、崩溃重启、多用户及SCM；门禁为串行化非授权，不进入VPN启动 |
| 主服务身份/数据 | 源码+fake+独立二审：请求前认证/响应ID/输入信任；未知不清闸。安装模式改以fresh SCM当前实例及持有映像认证，不再要求普通UI读SYSTEM token；前台令牌/pin保留。LIST_DIRECTORY有TEMP共享验证 | qa.3普通medium用户的pipe/SCM/进程权限与完整GetState仍待验；SCM账号是配置证据非已读runtime SID；默认ACL/迁移及完整兼容未验收 |
| SYSTEM机器恢复 | 独立机器上下文与SY/BA备份；qa.2四固定HKLM项真实SYSTEM apply/读回/restore及独立后验通过，新store canonical empty、legacyAllowCount=0 | 仅当前Win11 Pro和初始缺失的四值；qa.3/冲突/崩溃/其他令牌待验。旧含具名ACE空store显式保留归档，不等于支持自动迁移或用户HKCU恢复 |
| 原版更新兼容性 | Ultra 拒绝获取和安装原版更新，防止覆盖隔离资源 | 独立签名、清单和回退通道仍需工程实现 |
| 两版本并用 | 支持安装资源隔离 | 不保证两个全机隧道同时运行；不得为并行使用放宽闸 |

## 网络

| ID | 本版实现 | 剩余开发或验收 |
|---|---|---|
| N01 程序联网限制 | 代码+fake：精确本地 EXE 的独立 WFP 安装、检查、删除 | **未进行真实流量验收**；permitSelf的CLEAR_ACTION_RIGHT匹配路径、代理代发、存量连接、其他EXE均有边界，不把全部soft permit称硬许可 |
| N02 程序局域网限制 | 代码+fake：IPv4私网/链路本地、IPv6 ULA/链路本地等固定范围 | 同上；不等于应用无法读取本机网卡配置 |
| N03 回环/端口扫描 | 明确显示 localhost 未保护 | 后续：在原闸、IPv4/IPv6、本地代理及服务代理场景分别设计和验证，不能改个名字就宣称隔离 |
| N04 发现广播 | 代码+fake：所选EXE的组播与有限广播规则，文案与条件范围一致 | 定向广播、蓝牙、单播发现、系统服务代发未覆盖；真实投屏/共享影响待测 |
| N05 跟踪/遥测分类 | 保留原域名规则编辑能力 | 后续：经过来源和更新审核的分类表、例外与效果验收；本版未内置自动跟踪列表 |
| N06 外联用途控制 | 新检测不主动做网络探测；用户备份拒绝远程/不明路径，C04只读保护与写侧剩余边界单列；停止原版自动更新查询；原VPN必要请求保持原行为 | 后续：C04写侧竞态、原订阅/测速/出口查询等用途选择及兼容核对；不把无HTTP说成全进程零外联 |
| N07 规则查看/删除 | 代码+fake：只删自有已识别对象、unknownFilters独立count；未知残留或读回失败单条撤销不报完成，维护窗口内清理后再复核 | 真WFP ACL/持久化/重启、进程竞争与三方仲裁；未知不扩大删除范围 |
| N08 首次联网/目的地白名单 | 明确尚未实现 | 后续独立事件与决策工程；需超时/无人操作默认行为、代理进程身份、目的地解析与缓存规范 |

## Windows 设置

| ID | 本版实现 | 剩余开发或验收 |
|---|---|---|
| S01 时区 | 代码+fake：系统枚举、明确应用、保留夏令时、动态读回与备份；独立timezone-report/restore原用户CLI，maintenance-only助手不碰机器journal | 真实权限/UAC/替代管理员、浏览器缓存/覆盖及重启验收；不自动跟随出口，canApply取门禁就绪资格不等于修改权限 |
| S02 语言/地区 | 当前地区格式经本地NLS校验与规范化，无效值unknown且不导出；与浏览器语言区分，正式设置引导 | 后续：明确地区/显示语言/网页语言三个输入及各自作用范围，正确用户上下文、预览与恢复，验证重登影响 |
| S03 广告ID | 正式策略代码/fake；qa.2 SYSTEM注册表应用/读回/恢复通过 | 不能泛化隐藏硬件ID；Windows/应用实际功能与qa.3待验 |
| S04 输入个性化 | 正式 `AllowInputPersonalization` 策略代码/fake；qa.2 SYSTEM注册表应用/读回/恢复通过 | 该策略不等于关闭在线语音识别；第三方自行上传不受统一覆盖；实际系统功能与qa.3待验 |
| S05 剪贴板历史/云同步 | 两项独立正式策略；qa.2 SYSTEM两项注册表往返及独立后验通过 | 不证明实际剪贴板/同步行为，不删除已有云数据或阻止桌面程序读当前内容；qa.3待验 |
| S06 应用设备权限 | 只读默认定位/摄像头/麦克风记录，逐项引导；浏览器权限另实现 | 尚未实现完整每应用审计/后台权限枚举；不写未公开ConsentStore，不把Deny当全局禁读 |
| S07 随机MAC | 条件、风险和Wi-Fi设置引导，不采集原始MAC | 后续可增加用户主动的网卡支持性检查；不支持硬件不能伪成功 |
| S08 设备名/发现服务 | 说明可读信息和应用发现规则的有限范围 | 后续：明确名称输入、重启/域加入影响、可靠回滚及独立发现策略；当前无自动全局修改 |

## 浏览器（配置单独管理）

| ID | 本版实现 | 剩余开发或验收 |
|---|---|---|
| B01 WebRTC | Chrome/Edge固定策略、扩展正式API限制非代理UDP；新TEMP Edge配置真实设置/控制权/journal/恢复往返通过 | 不是完全关闭或公网IP消失证明；未做STUN/流量、其他配置或来源路径验收 |
| B02 第三方Cookie | 固定策略、扩展控制权/读回；新TEMP Edge配置实际API设置/恢复通过 | 不证明所有Cookie请求被阻止；例外、Storage Access、第一方关联仍存在，不清理旧数据 |
| B03 独立身份/退出清理 | 独立会话使用生成的 `BrowserInstances/<session-id>` 私有树；浏览器配置新增“退出时数据”选择，可保留数据或请求删除本会话拥有的数据树。进程树与防火墙撤销确认后，以 `os.Root` 相对句柄扫描、拒绝重解析点并删除，失败明确保留 `closing_or_unknown`，不会触碰共享内核缓存或 `BrowserProfiles`。源码/Windows 交叉编译与纯文件回归已通过 | 真实浏览器窗口退出、异常进程与跨版本数据目录仍需 VM 验收；选择器已接入配置但不能把 UI 选择或纯测试外推为实际浏览数据删除完成 |
| B04 跟踪参数/Referer/ping | 扩展正式API；用户粘贴链接、预览后复制，敏感模式保守保留 | 无后台自动改写；签名语义无法通用识别；逐站例外管理仍需开发 |
| B05 预测/建议/诊断 | 固定策略；扩展支持的预测等；新TEMP Edge配置的network prediction实际API设置/恢复通过 | 其他设置/版本与企业策略页仍待验；保留安全浏览/更新防护，设置值不证明流量行为 |
| B06 网站设备/内容权限 | 正式API按类型选择allow/ask/block（JS仅两态，摄像头/麦克风allow须HTTPS）；fake/DOM通过，新TEMP Edge仅通知example.invalid的block/ask/clear抽样通过 | 本次仍verified=false，不推断所有站点或规则均撤销；其余API/企业覆盖/隐身待验；改变来源替换本扩展同类规则 |
| B07 USB/蓝牙/串口/HID等 | 边界说明、浏览器正式设置引导 | 未加入不确定版本的通用开关；后续按API逐项验证 |
| B08 WebGL/WebGPU | WebGL正式策略；扩展本地能力自测 | WebGPU无通用自动关闭开关；设备名称与能力集合不同，不把WebGL禁用当WebGPU禁用 |
| B09 Canvas/Audio/字体/硬件等 | 实际浏览器扩展来源的有限可见性自测、原生反指纹引导 | 无通用全系统加噪；浏览器原生统一保护和iframe/Worker一致性需独立工程，不能仅补几个JS属性 |
| B10 环境协调 | Windows时区可选；当前地区只读；浏览器本地自测 | 后续：用户选定浏览器配置的语言/时区/UA/Client Hints一致性方案；不默认随机化或按IP匹配 |

## 数据

| ID | 本版实现 | 剩余开发或验收 |
|---|---|---|
| D01 日志 | 保留原记录/轮转/天数设置；检测仅级别与天数摘要；已验证logs/diag全部历史后代私有SY/BA，诊断默认不读日志，明确选入有界片段 | TEMP DACL/fake通过，真跨账户/日志负载待验；不撤回既有复制或打开句柄，必要故障证据不可无声删除 |
| D02 导出 | 检测后端缓存同一快照、全量/筛选预览、默认排除环境/观察身份，保存同一字节；诊断默认最小摘要，详情逐项确认，同ZIP预览/hash，私有新建文件 | Go/TEMP/DOM通过，真实桌面端到端待验；详情可能保留个人标识，不能称匿名。Web/Android只支持最小摘要，详情/ZIP明确不支持，旧下载410 |
| D03 凭据 | 核心输入/新隐私备份独立ACL；检测明确存储边界且不读取秘密值 | **未新增通用DPAPI凭据加密**，列后续工程；ACL不隔离管理员，C04同账号主动改向残余仍未关闭；迁移须涵盖SYSTEM/用户/备份/恢复/明文残留 |
| D04 复制后清理 | 已实现本应用秘密复制后的可选 5–600 秒清理；Windows 后端在同一剪贴板锁内同时核对序号、所有者和内容摘要，内容被其他程序替换时保留新内容；关闭、取消、立即清理和旧页面失效均有 Go/DOM 回归 | 真实 VM 的剪贴板竞争、退出、历史/云同步边界仍待验；不清除云端或其他程序已经读走的副本，也不声称控制所有应用剪贴板 |
| D05 元数据 | 代码+临时文件测试：JPEG APP1/APP13/COM、PNG eXIf/tEXt/zTXt/iTXt检查/副本清理 | 保留编码像素/ICC/其他块；文档、视频及其他格式未支持；照片方向影响已说明 |

## 检测与 UI 验收

- 检测读取当前策略，与可写资格分开。机器助手授权不等于观察进程提权；网站/应用/网络视角分别表达。缺失、读失败、未覆盖和未实测均不升级为“安全”。
- CLI明确direct_cli与当前进程用户journal，machineJournalChecked=false/not_inspected；直接提升不推断助手授权或机器journal健康。DNS、日志及凭据为独立finding；只传DNS模式/级别/天数，不序列化原始地址、settings/profile或秘密。
- 展示剩余可读范围，并附证据、时间、来源和措施；不声称某个应用实际上读取过。系统默认权限记录不等于每应用权限清单。
- 桌面/扩展失败、回焦和管理变更后使证据失效；桌面报告另有到期限制，扩展没有定时TTL。分来源实际读取完成时间，缺来源留空，不用generatedAt伪装采集。共享MV3后台串行化扩展读写与恢复记录；没有接入桌面自动同步。
- 当前宿主新TEMP Edge配置真实加载MV3 worker，五个同时请求经产品Client/Coordinator全回执，中间active、最终值及journal恢复；不覆盖worker终止/重启、多窗口/配置/扩展竞争。三项privacy和通知单origin设置往返不是网站、STUN、硬件或流量验收，页面四次本地资源请求不证明全浏览器零外联。证据见[真实MV3有限验收](qa/MV3_EDGE_INTEGRATION.md)。
- 真实浏览器自测在扩展本地；客户端WebView结果单独标注。当前未实现扩展与桌面自动同步，防止无意收集浏览指纹。
- 隐私界面、检测界面、时区选择及关键失败路径已有DOM和Edge headless mock测试；深浅色420/900宽视觉证据位于screenshots。模拟测试没有修改真实设置。
- 所有新增功能/子操作/输入/检测信息的`?`需持续纳入新增功能检查；未来未实现项也必须说明原因，不能做空开关。

## 当前 beta 验收与后续路线图

1. 先用可恢复Windows环境验收本版UAC、策略/时区恢复、原有连接与WFP真实行为，未通过的功能保留配置证据或禁用修改。
2. 当前先关闭现有实现缺陷并完成相应验收，人工安装恢复说明保留异常目录/备份，不自动续装、接管或删除；未知过滤器、journal损坏或维护Busy不能假报卸载完成。
3. S02/S08、B03持久化身份选择、D03通用凭据迁移、N03回环、N08首次联网决策继续列路线图，后续逐项设计和验收，不与隧道启动成功绑定，不在首beta临时塞入。
4. N05分类、N06用途选择、B04站点例外、B07更多设备权限按已列状态推进；N06未实现部分不因新检测无HTTP而关闭。不为“功能更多”关闭SmartScreen、安全浏览、证书验证或系统安全更新。
5. 完成受支持Windows/浏览器矩阵、独立签名和更新后，才标正式发行。每批继续在执行日志记录与计划的差异。

本轮新增证据：[诊断](qa/DIAGNOSTIC_MINIMIZATION_FIX.md)、[前端/扩展](qa/FRONTEND_AUDIT_FIX.md)、[维护/恢复](qa/MAINTENANCE_RECOVERY_FIX.md)、[独立二审](qa/MAINTENANCE_SECOND_REVIEW.md)。C04仍以[独立用户存储二审](qa/LOCAL_STATE_SECOND_REVIEW.md)的读写分层限制为准。工具编辑时误调用系统Python launcher导致安装管理器26.4/Python3.15.0自动安装的偏差已告知用户，见[记录](qa/TOOL_RUNTIME_DEVIATION.md)；不是产品外联，未进行全局环境回滚。

## 2026-10-20 qa.4 实机状态

qa.4 Windows 候选已完成私有暂存、三 EXE 哈希核对、CLI/服务五项只读命令和 SCM 只读回读。该证据只覆盖 SYSTEM 观察范围；普通 medium 用户、安装/UAC、策略写入恢复、浏览器实际行为、隧道/WFP/流量仍保持待验收。检测中心中的 `unknown` 与 `manual` 项继续按未证实处理。

## 2026-10-21 修复后候选实际安装状态

- 安装器：在一次性 Server Core VM 上实际安装成功，目标目录、清单哈希和 406 payload 校验通过；首次漏跑 Expand 的流程偏差已补跑并记录。
- 服务：`godusevpn-ultra` 真实注册并运行于 LocalSystem，路径和 CLI/service 版本读回匹配；独立服务卸载后 SCM 计数为 0、状态 `not-installed`。
- 官方卸载：机器清理返回 1，脚本保留数据和恢复工具，按安全失败处理；不能写成完整卸载通过，也不能推断机器策略恢复成功。
- 范围边界：Server Core + SYSTEM Run Command 未验证普通 medium 用户、UAC、GUI、浏览器完整矩阵、真实订阅隧道、WFP/流量；这些仍是待验收，不提升为“已保护”。

## 2026-10-21 前端状态语义复核

隐私中心现在把“不能在应用内完成”的原因直接写在卡片动作区：

| 状态 | 用户看到的含义 | 应用行为 |
|---|---|---|
| `manual` | 需手动设置 | 只提供系统入口和说明，完成后需回到检测中心复查 |
| `unsupported` | 本环境不支持 | 不显示虚假启用开关，只给出降低暴露的替代方案 |
| `unknown` | 无法确认 | 禁止根据旧结果写入或导出，要求重新检查 |
| `policy_conflict` | 被其他策略覆盖 | 不覆盖组织、设备管理或其他扩展的设置 |
| `recovery_pending` / `external_change` | 恢复未完成／检测到外部更改 | 先处理事务或冲突，不能再次覆盖 |
| `configured` 且未由本应用管理 | 策略匹配，待运行验证 | 不提供恢复按钮；只读匹配不等于网页或应用运行时已受限 |

状态徽章使用短句（如“规则已装，流量未测”“读回匹配”“仅覆盖部分范围”），避免把配置证据写成“已保护”。
