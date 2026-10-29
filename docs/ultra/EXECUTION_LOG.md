# Ultra 分阶段执行核对

## 2026-10-27 qa.11 代理真实 GUI 与运行中取消 UAC 有限验收

- 普通 Medium 在已安装 QA11 GUI 手填用户提供的单个认证 SOCKS5 节点并显式点击测试；country.is 实际返回出口与已知代理出口一致，归属地 US / CA / Los Angeles，当次 1103 ms，checkedAt=2026-10-26T17:04:39Z。公开文档不含真实 IP、节点地址、端口或认证。
- 密码追加合成错误后缀后旧成功结果被隐藏；再次显式测试失败，界面提示没有改用直连。本批没有 socket / 流量捕获，无 fallback 的证据仅为界面、源码和此前生产模块测试，不把该提示算作本轮独立网络证明。
- 恢复正确认证后没有自动测试，但旧错误 DOM 残留已实际发现并纳入 QA12 源码修复；修复后真实 GUI 重验另记，不回写冻结 QA11。
- 运行中点击 PrivacyAuthorize、通过产品确认后实际 UAC No，GUI 保留，显示用户取消及“授权未完成，请手动重试”。本次没有连接隧道，不宣称已验取消授权不影响运行中隧道；未覆盖接受授权、替代管理员或助手连接成功。
- 详见 [脱敏代理 GUI 记录](qa/QA11_PROXY_GUI_VM_2026-10-27.md)；本次仅 probe，不是浏览器实际出口、HTTP / HTTPS 代理、Hysteria2 或 WFP / Job 验收。原始四张截图仅留私有材料。本轮只更新指定文档和元数据对标表，没有运行开发机产品、重新构建 / 上传或改冻结包；QA12 源码正在独立变动，未在此时重跑全源审计。

## 2026-10-27 qa.11 配置真实 GUI 与独立磁盘有限验收

- 升级后的实际普通 Medium 会话中，原 QA9 配置 baseline 退出 0，count=1、912 字节、frames=1，ID / revision 同原记录。实际 GUI 载入后只改名称并保存；after 仍 count=1，记录 1849 字节、frames=2，名称哈希匹配人工观察的预期新名称，config / engine / ACL 哈希不变。
- fresh 冷 WinPS 5.1 实际运行已审 7037 字节 launcher 与 17503 字节冻结 Compare：rename status=pass、diskTransitionVerified=true、countDelta=0、renameOldBytesPrefixVerified=true、currentFileReadOnlyPrefixRead=true、退出 0；当前文件完整 SHA / 长度先匹配 after，再验证前 912 字节等于 before revision。其他记录、ACL 和直接同级名称哈希不变。没有在取得前缀证明前继续下一次 GUI 写入。
- GUI 修改草稿名称并另存为新配置，实际展开确认新 ID；真实 before/after baseline 加 clone 比较通过，count 1→2、countDelta=1、cloneNewID=true；新记录 914 字节、单帧，预期名称 / engine 匹配，源记录及其他 ACL / 同级项不变。cloneNormalizedConfigEquivalenceVerified=false 保持原值，不把新 profileId 导致不同的 raw config 哈希当成整份配置等价证明。
- 实际删除副本先点取消，前后整个独立 baseline JSON 用 Ordinal 比较为 true，auditor 退出 0。重新取基线后由 GUI 确认删除副本，界面提示记录删除、当前草稿和浏览数据保留；after count=1，cold delete 比较 status=pass、countDelta=-1、退出 0，仅目标记录移除，其余记录 / ACL / 同级项不变。界面提示不等于已独立核验浏览数据。
- 详见 [脱敏配置验收记录](qa/QA11_PROFILE_GUI_VM_2026-10-27.md)。三个比较均 receiptMatchesDisk=false、secretCanariesChecked=0：完整 GUI 后端 receipt 未取得且没有伪造；本批没有真实秘密 canary、浏览器启动 / 代理 / 浏览数据删除证据。跨窗口 CAS、并发、故障恢复、损坏和其他用户 / ACL 矩阵仍未由正常路径覆盖。
- 原始截图、账户、随机 ID、revision / 名称哈希、私有路径和完整 JSON 仅留私有材料。本轮只核对证据并更新文档，未改产品源码、GPS / 服务 / UI 测试或冻结 QA9 / QA10 / QA11 包，未重新构建 / 上传 / 发布。

## 2026-10-27 qa.11 官方升级与取消 UAC 崩溃回归实测

- 固定 qa.11 ZIP 54,868,922 字节，SHA256 `688fe67c2f4a8c771db6ba4de9f1deb78b525a0b9c7b6650e91116e9ed558ce9`。VM 实际接收返回 `qa11_staged_verified`：1983 个 ZIP 项（1966 文件、17 目录）、1965 payload，`rootUnchanged=true`、`executed=false`。接收前后已检查根/阶段目录安全描述符一致，接收器不执行 payload；后续官方升级另行执行。
- 已审固定升级脚本在 VM 完成全部阶段：QA9 版本/全 payload/Running LocalSystem 验证 → 已安装官方卸载 → 服务不存在 → 完整旧 QA9 树移动保留且文件/目录快照一致 → 冷 WinPS 5.1 官方 QA11 安装不传 Source → `official_upgrade_verified`。最终版本 `0.01-beta-qa.11`、全部 1965 payload 匹配、Running/LocalSystem、`oldFilesRetained=true`、`defaultSource=true`。详见 [脱敏证据表](qa/QA11_OFFICIAL_UPGRADE_2026-10-27.md)；原始日志、路径、账户、资源标识及截图仅留私有材料。
- GUI 第一轮：普通 Medium、无参数、实际 UAC No，原始 Process handle 在 10/30/60/120 秒等待后仍运行，180 秒诊断结束时 `pending_process_alive`、`exitCodeAvailable=false`，未强杀。后续实际最大化/还原与托盘退出可观察到，同名 GUI 查询 0；这一轮不能写退出码 0。
- GUI 第二轮：fresh 冷 PowerShell `-NoProfile -File` 固定诊断器，同一 GUI 映像 SHA256 `d93205d060ecd86d651d72a4ac63aedbb9572d83bc875c3edafa201bc5afae05`，Medium 8192、无参数、UAC No 后 GUI 保留，实际托盘 Quit 后原始 Process handle 读回 signed `0` / hex `0x00000000`、`processKilled=false`、capture `complete`，GUI 查询剩余 0。stdout 0 字节；实际确认 stderr 64 字节后审读，仅一行 WebView2 环境创建成功日志，未见 panic/error，不据此排除所有潜在问题。本条件下取消提权崩溃回归与正常退出为实际 PASS，不覆盖其他接受/替代管理员授权路径。
- 窗口截图记录本次 1041×720 工作区最大化、还原 420×704；其他 DPI/多屏、首次显示、重载和最小化组合仍待验。浏览器、流量、WFP/Job、完整 GUI 和其他模块不由安装或进程退出自动判通过。
- 本轮只更新脱敏文档；开发机未再次测试、构建或运行产品。冻结 QA9/QA10/QA11 包保持不变，新文档不回写其 ZIP；下方 QA9 元数据记录仍限定原版本、两张合成图片与七类 EXIF。

## 2026-10-27 qa.9 元数据真实 GUI 有限验收（优先于下方历史待验记录）

- 普通 Medium 用户在已安装 qa.9 GUI 实际调用固定 ExifTool 13.59，读取自行生成的 JPEG（4×3，1340 字节）和 PNG（4×3，771 字节）。默认只显示字段类别/数量；显式查看作者后仅显示合成文本 `QA Synthetic Artist`，公开记录不包含实际路径、账户、序列号或经纬度。
- GUI 分别对 `artist`、`copyright`、`description`、`software`、`camera`、`capture-time`、`exif-gps` 和同时选择七类执行新副本保存，共 16 个副本，界面 `removed` 后验与选择相符。真实取消保存保留原预览且无成功提示；PNG 选择既有目标并在系统替换提示确认后，产品后端仍以红字拒绝覆盖。
- 独立只读文件检查器实际在 VM 运行，`VERIFY_EXIT=0`、`allExpectedCopiesPassed=true`、`existingDestinationUnchanged=true`；16 项 `exists/sourceUnchanged/pixelsUnchanged/selectionVerified` 全 true。其 `adapterExecutedByVerifier=false`、`uiAcceptanceRequired=true` 保持原值：它不调用 ExifTool/产品桥，也不制造 GUI 成功。字段后验仅证明这些夹具的选中类别消失、未选类别仍在，不扩大为每个保留值或所有元数据均相同。
- 详细证据与边界见 [元数据真实 GUI 记录](qa/METADATA_REAL_GUI_VM_2026-10-27.md)。原始截图、VM/账户标识、绝对路径和 UTF-8/CreateNew 保存的完整 JSON 仅留私有材料。未在开发机运行产品、ExifTool 或 VM 检查器；本次文档复核只读截图、源码和包内依赖字节。
- qa.9/qa.11 固定 ExifTool 清单及全部 1501 成员已逐字节比较一致，清单 SHA256 `d1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf`。qa.11 私有 ZIP `688fe67c2f4a8c771db6ba4de9f1deb78b525a0b9c7b6650e91116e9ed558ce9` 保持冻结；相同依赖不等于整个 GUI 相同，默认 UAC 取消修复与 qa.11 整包 GUI 仍待该版本实际验证。
- 范围仅七类 EXIF、两张合成图片；XMP/IPTC、缩略图、实拍照片和异常/并发矩阵继续待验。本次新文档晚于冻结包，不回写历史 ZIP；publication 源审计在文档完成后另存当前指纹，与 qa.11 构建前 `63e1554dc890a8fe7e7203e2a93577eec4945ed613f07678eb2190343536bb5e` / 构建后 `c356405288fdada240ef54c2d22fe1287815a28fcf5135eb11265e6b48671e97` 区分。

## 2026-10-26 下一候选纯 UI 与发布准备检查点（未构建 / 未实测）

- 当前源码的 16 个固定 mock / DOM / 扩展入口均退出 0，覆盖 browser-start、window-controls、identity、native-engine、saved-profiles、module-form、hardware、profile、metadata、clipboard、IPC identity、diagnostic、旧 UI 与扩展核心 / 协调器 / 面板，具体证据和未满足模块见 [本轮记录](qa/QA10_PURE_UI_PREPARATION_2026-10-26.md)。
- 新增启动 / 窗口 fixture 已在 Windows CI；补入先前遗漏的 clipboard fixture 和退出码检查后，上述入口全部纳入同一门槛。这里只核对 workflow 源码，没有调用托管 CI。
- 当前源 publication 启发式扫描返回 findings=[]；本轮文档更新后再次运行并在最终命令回执记录精确源清单哈希。固定私有传输 URL / 标识另做只读源集排查，私有接收器与云回包仍被 Git 忽略；没有构建、上传 QA10 或变更冻结 qa.9。
- 本机只运行 Node 纯 mock 和源审查，未运行实际产品 / 浏览器 / 剪贴板 / 服务 / 系统设置，也未操作 VM。qa.9 官方安装 PASS 仍以其独立记录为准；真实 GUI / 存储 / 身份 / CPU / 浏览器 / WFP 与用户其余模块没有因此被标为完成。

## 2026-10-26 qa.9 官方升级与下一候选源码检查点（GUI/浏览器待验）

- qa.9 已冻结构建、包审计并在临时 VM 完成官方升级：54,799,224 字节，1958 payload / 1959 ZIP 成员，ZIP SHA256 `d8dc35c504e1a5c754ca7a3080f793769bfb69443c4151f1558ceff37f4b9095`。本次通过证据来自 qa.9 自身的已安装文件与服务读回，不继承 qa.8；旧包字节不回写。
- 官方升级首轮曾返回 `preflight_refused`，在卸载、目录移动和新安装之前停止。原因是 QA 父目录继承权限不满足私有目录检查；固定根目录权限准备脚本经 AST / 独立审查后转入 VM，其原始云回包丢失，但独立读回确认父目录与 stage 私有、owner/group 保持、原 SDDL 备份存在、qa.8 服务仍 Running。随后原样重试升级：qa.8 官方卸载退出成功且服务消失，整棵旧安装树移动后文件及空目录核对匹配；冷 WinPS 5.1 官方 qa.9 `-File` 不传 Source，实际返回 `official_upgrade_verified`、版本 `0.01-beta-qa.9`、全部 1958 payload 匹配、Running / LocalSystem、旧树保留且 `defaultSource=true`。详见 [脱敏升级证据](qa/QA9_OFFICIAL_UPGRADE_2026-10-26.md)。两份私有准备脚本本机均未执行；原始日志、云标识与连接资料不进入公开文档。
- 下一候选源码已接网页 CPU 数量：WebView2 / Chrome 的 `hardware.logicalProcessors` 为 1–64，省略保留原生；正式 CDP 命令后独立读取原生属性，主文档、同源/opaque frame 及后续暂停 target 不匹配即拒绝，Firefox 自定义请求在协议修改前拒绝。这不改变物理核心、性能或 RAM。WebView 静态编译与 624 项 C#、27 项 GPU Node vm、9 项 CPU Node vm 纯检查通过；原生宿主 151 项 Go 检查、普通/验收 tag test/vet 通过。这里的 Node vm 是内存脚本测试，不是 Windows VM 验收。
- 下一候选规范 UA 生成已采用现代冻结的 OS / 版本格式，实际 Client Hints 完整引擎版本仍来自可信内核读回；手动/原生 UA 文本保持，所选环境独立设置明确字段并提示可能不同。Windows 10/11 的平台版本快捷值为 `10.0.0` / `13.0.0`，UA 继续保留 `Windows NT 10.0`；输入仍可编辑。新增 [身份 DOM 回归](qa/browser-identity-ui.cjs) 验证快捷值、视口不丢失、手动/原生文本或未知占位保持、重绘无需重复请求及晚到预览隔离。独立 [原生内核 DOM](qa/browser-engine-ui.cjs) 的两内核×四环境×generated/manual/native 共 24 组合通过；全为固定 mock，不是实际浏览器身份读回。
- 配置保存/载入/重命名/克隆/删除已接实际 Go 存储与原用户目录，完整 revision CAS、追加日志、短写/同步失败、损坏/中断恢复及错误边界已有 fake/TEMP 测试；生产 NT 路径只读交叉审查未发现确定调用缺陷，真实 medium 用户文件/ACL/相对句柄行为仍待 VM。身份、保存配置、CPU 和通用表单四项纯 DOM fixture 通过。通用表单补验已移除的旧订阅保存按钮不能调用 API、日志读取失败不能复制旧隐藏内容且恢复时重新绘制；均未调用真实剪贴板、产品、系统或网络。
- 下一候选原生启动已接 BeginLaunch / token / Status / CancelStart，准备中禁编辑与新启动；只有关闭成功、严格 closed 且 Done 已闭才释放失败启动 owner，规则或进程未知时继续保留。Go controller / host 普通与验收 tag test/vet 及 25 次 controller 并发回归通过。[启动 DOM](qa/browser-start-ui.cjs) 八类固定 mock 场景通过：pending / 取消确认、重复与旧 DOM、ready 竞争、未知清理、Status 错误及错 token、等待轮询或 Begin 回包时离页、旧接口兼容。测试先复现离页期间下一次 Status=ready 时漏取消，以及 unknown 状态丢失检查入口，两处修复后重跑通过；全程未启动实际产品或网络。身份、保存、通用表单、CPU、原配置和原生内核六项相关 DOM 回归也通过。
- qa.9 官方升级证据只覆盖本次安装、服务身份与文件保留；上述下一候选 CPU / 现代 UA、后续取消启动和错误路径源码均未进入冻结 qa.9。普通用户会话已确认 Medium，但真实 GUI 操作、UAC 取消/授权、浏览器 UA/CH/CPU HTTP 与多 frame / OOPIF、WFP/Job/代理出口、NT 配置存储、租约断连和其余功能矩阵仍待逐项实测；GUI 启动命令返回或托盘出现不代替这些证据。公开仓库、正式 tag / beta Release 与最终临时资源清理尚未完成。

## 2026-10-26 较早浏览器表单与 UA 检查点（历史）

- 要求完整保留至 H37，包括最新 UA / 填写逻辑与四张 AdsPower 参考图。当前源码重排环境/指纹/代理/内核/现有浏览器五类表单，保留草稿；内核与已下载版本进入同一配置，UA 生成/手改、晚到结果隔离、检查并打开已接入。80 张固定 mock 界面截图检查无脚本错误/页面外联/横向溢出；这不是实际浏览器验证。
- WebView2 UA 后端新增真实 HTTP/navigator/CH 读回；495 项 C# 纯检查与 27 项 JS 检查通过。Chrome CDP / Firefox BiDi 与四系统环境/UA、封闭代码树、Job/WFP/生命周期已有实现，普通/验收 tag test/vet 通过；真实 VM 未验收，普通构建仍保留未就绪启动门。原生 GPU 参数明确拒绝，其他指纹需求继续保留未完成。
- qa.8 已构建并独立审核：54,541,353 字节，1953 payload / 1954 ZIP 成员，ZIP SHA256 `1cdaf5c7365208dbb6e0530aa1cb20330e3761e2d2469965fa01db8ce7e94d66`。VM 官方冷 WinPS 5.1 使用显式 Source 安装成功，LocalSystem 服务运行；默认 Source 冷入口缺陷已修并补 PS5.1 / PS7 TEMP 回归。qa.8 字节冻结，最新 UA/UI 需要新候选。
- 当前 VM 客体 RDP 服务运行、3389 监听，Bastion Developer 请求桌面会话前返回服务端错误；未把错误当作凭据失败或完成 GUI 验收。继续诊断管理入口及产品代理模块真实测试；没有在本机启动产品/浏览器/ExifTool、修改网络或服务。
- 随后产品 browserproxy / 认证 relay 的 VM 真实测试通过：远端 DNS、TLS、同一美国出口、错误认证拒绝、关闭后凭据清空/监听关闭；固定程序与源树摘要见 [验收记录](qa/PRODUCT_PROXY_VM_2026-10-26.md)。进程由 RunCommand 启动，不外推普通用户 GUI 或浏览器实际出口。
- 尚需完成配置保存/载入、其余真实后端和 VM 运行矩阵、全量审计、公开仓库/精确发布/下载复核和临时资源清理；整体未完成。

## 2026-10-26 较早新增实现与有限 VM 验收检查点

- 完整要求已按 H01–H35 保留。全文读取原会话的 63 条用户侧记录与 206 条助手文本；没有把重复二进制 / 截图工具输出当成新的项目要求。原项目 420 个 tracked 文件复核无变化。
- 浏览器：独立 WebView2 宿主、四 OS 环境模板与手动系统版本、GPU vendor / renderer、显式预览失效、独立认证代理桥；Go、205 项 C# 纯检查及 JS / DOM 测试通过。未在本机运行实际浏览器或改服务 / 网络设置；VM 运行时仍待验收。
- 多版本：官方 Chrome for Testing / Firefox 目录、固定来源下载 / 解包 / 完整树校验与版本读回；真实官方归档静态验证通过。原生 Job、挂起进程与独立 WFP 框架已有源码，但协议、代码封闭与生命周期未齐，启动明确拒绝。不会用缓存或框架存在冒充可用浏览器。
- 代理：SOCKS5 / HTTP / HTTPS 的认证、远端 DNS、证书校验、无直连回退、双端关闭、取消和总超时通过纯内存测试；凭据不进普通 JSON / fmt / 前端结果。新测试订阅含 12 个 Hysteria2 和 1 个 SOCKS。
- VM 有限证据：重建临时 Windows 11 测试机；官方 qa.7 卸载返回 completed=true，Ultra 服务数 0，安装文件保留。独立 PowerShell SOCKS5 + TLS 脚本在 VM 成功认证并通过免费 API 观察到美国出口；未修改系统代理。此证据不代表产品代理、浏览器出口、隧道或 WFP 已验收。详见 [受控 VM 记录](qa/QA7_UNINSTALL_AND_SOCKS_PROBE_2026-10-26.md)。
- 元数据：固定 ExifTool 13.59 的完整 1501 文件清单、执行边界、七组选定 EXIF、来源哈希绑定、只生成新副本；字段值须显式显示，UI取消 / 失败 / 旧页面结果隔离已验证。没有在本机执行 ExifTool，VM 验收待做。
- 秘密剪贴板：默认关、5–600 秒、内容哈希 / 序号 / 所有者同时匹配才清当前项；纯测试通过，实际剪贴板待 VM。GPS：原 locsim 不构成真后端；CAS 适配器已有纯测试，真实 Windows 定位仍未实现。
- 包装：修复 WinPS 5.1 冷启动 -WhatIf 的 Get-FileHash 空结果；新宿主与 ExifTool 整树进入 qa.8 包校验，最终 ZIP / 外层清单 / 嵌入前端独立审核。qa.7 保持原字节，不回写历史结果。
- 下一步：qa.8 构建、包审计、VM 安装和真实功能验收；同时继续原生多内核与其余未完成要求。临时测试资源在完整测试结束后清理；公开仓库 / v0.01-beta 尚未发布。

## 2026-10-26 全量对话接手核对（早期检查点）

- 预期：完整核对用户整体要求，保全 Ultra 现有改动，继续实际开发与验收；原目录和本机网络不改。
- 需求来源：新增 `HANDOVER_REQUIREMENTS_2026-10-26.zh-CN.md`，现按 H01–H35 保留初始要求与后续开源/GPS/内置浏览器追加目标。首 beta 的发行批次不作为整体任务的完成定义。
- 实际：原目录重新比对 420 个 tracked 文件，差异 0，Git 状态干净；qa.7 独立只读验证 PE/版本/实际 ZIP 与清单通过，425 payload / 426 ZIP 成员，ZIP SHA256 `327acea1749747d24b2d945739219bb3ce35b09ef46cd937f126756355626bb6`，`runtimeVersionVerified=false`。
- 实际：把卸载 SCM 读取与固定安装身份判定分离，补 26 个合成身份用例（路径、参数、账户、服务类型），定向 CLI 测试与 vet 通过；未运行产品或打开/修改真实 SCM。原 mgr 查询与拒绝条件保持，完整卸载仍需 VPS。
- 偏差：文档当前候选落后于实际 qa.7；本次新增代码需要新候选，历史包不回写。GPS 只有适配器、浏览器只有运行时清单，不能计为用户功能已交付。
- 下一步：并行核实 GPS 上游真实实现、修复适配事务并开发浏览器配置/能力模型；读取 Azure 当前资源，随后在临时 Windows VM 做当前候选实装/官方卸载和其余验收。

日期：2026-10-19（Asia/Shanghai）。依据根目录 `PROJECT_PLAN.zh-CN.md`。

## P0：基线与计划

- 预期：独立复制原项目，保存可执行和可验收的详细计划，原目录不修改。
- 实际：从本机干净的 `godusevpn` 仓库使用 `git clone --no-hardlinks` 复制至用户指定目录；副本分支 `codex/windows-privacy`；移除指向原本机仓库的写入 remote，保留只读用途的 GitHub upstream。
- 基线：提交 `b51aed4`；420 个 tracked 文件 SHA-256 已保存于新目录旁的 `godusevpn ultra-source-baseline.json`（不含用户凭据）。
- 计划：R01–R14、P0–P9、功能级别、检测证据、UI问号和输入规范、失败与恢复边界均已列出。
- 尚待：交付前再次比对原目录指纹；核对阶段实际实现和未完成清单。
- 结论：计划和复制已完成，原目录最终复核待 P8。

## P1：命名空间与权限边界

- 预期：新旧产品不会相互控制；默认请求管理员，但取消不会影响原隧道。
- 实际：独立 identity 统一服务、ProgramData/APPDATA、IPC、URL协议、自启、单实例、TUN名/地址、端口和原闸GUID。保留原导入链接解析，但仅注册Ultra scheme。界面保持原用户，无界面助手异步请求UAC；只接受固定方法/策略ID，机器助手不进入HKCU。
- 复核：GUI helper分支位于Wails/普通启动之前；固定本地管道ACL、随机会话、双向PID/SID/路径校验、超时不重试写操作。辅助模块与服务连接流程分离。
- 证据：identity/IPC/guard已有及新增测试、privacyhost内存通信与参数测试、GUI架构contract测试均通过。`go list -deps ... ./internal/daemon`确认没有新隐私模块依赖。
- 偏差：Ultra暂停获取/安装上游原版更新，防止原安装包覆盖隔离资源。尚未配置独立签名更新；未宣称保留可用的自动升级通道。
- 结论：开发边界通过；真实UAC同账号/替代账号和父进程退出仍待隔离Windows验收。两个整机隧道不承诺同时运行。

## P2：固定策略、时区与恢复

- 预期：只有明确支持的项目可写；写前备份，错误和外部冲突不破坏原值。
- 实际：Chrome/Edge各11项及Windows4项固定策略；HKLM/HKCU、系统适用性、读取证据与写入资格分开。没有UAC仍尽可能只读实际HKLM值。配置匹配不标运行时已保护。时区枚举选择、保留夏令时偏好、独立备份与恢复。
- 证据：fake测试覆盖持久写失败、部分失败、备份损坏、非白名单/错误用户、机器级覆盖、读失败、外部修改、重复操作、恢复、时区列表与冲突。privacy/timeprivacy单测及vet通过；跨平台stub和ARM64包编译由模块验收通过。
- 复核修复：已管理策略读失败不能误判为外部修改；只读匹配不能解除修改权限。机器RestoreAll遇损坏备份必须失败，不能因Managed列表为空误报恢复完成。
- 偏差：系统语言、设备名、完整每应用权限、凭据迁移等未纳入本版可写catalog，保留明确引导或后续任务。
- 结论：已实现范围的自动验收通过；真实策略接纳、时区修改/重启恢复未执行。

## P3：应用网络规则

- 预期：独立WFP规则，不放宽原闸，不误伤本产品/核心系统，安装与实际效果分开。
- 实际：独立持久provider/sublayer、固定程序身份、IPv4/IPv6规则、事务提交、实际对象检查与ID限定删除。拒绝远程/重解析/系统核心目标；只管理自有可识别对象。未知持久过滤器单独计数并阻止清理假成功。
- 证据：事务失败、无管理员、读取失败不写、重复/部分/非法所有权、删除范围、路径、ABI、JSON与编译器逃逸回归通过。`go vet`和checkptr模块验收通过。
- 复核修复：WFP union中的Go指针需要保证堆分配，不能仅依赖KeepAlive；增加编译逃逸检查。
- 偏差：原闸高优先级hard permit可能覆盖较低层阻断；代理重发、存量连接和localhost未覆盖。UI只写“过滤器已安装，流量未实测”，不写“已禁止全部联网/端口扫描”。
- 结论：规则管理代码自动验收通过；保护效果仍待真实流量、严格闸和三方仲裁验收。未新增内核驱动、回环隔离或首次联网决策。

## P4：界面、说明与输入

- 预期：无订阅可进入，所有新增功能/子操作/输入/检测项有轻问号，用户选择后才应用，失败不碰隧道。
- 实际：隐私/检测独立导航，系统/网络/浏览器/数据/边界分类，程序与文件选择器，网络模式与真实时区选择框，搜索/筛选，独立策略/时区恢复。问号支持悬停、点击、键盘、Esc、外部关闭和避边。
- 证据：`browser-extension/ui-tests.cjs`通过，检查新增button/input/select/summary/h1/h2说明覆盖、确认、失败保留输入、时区读回/夏令时、只读不可写、未知状态和观察者身份。`web`兼容测试及JS语法检查通过。
- 视觉：Edge headless、纯mock、420×760与900×900、深浅色；现有73张PNG及三份QA结果。最新局部audit通过，零页面异常和页面外部请求。复用源码位于`docs/ultra/qa`；模拟截图不是机器实际保护证明。
- 复核修复：窄窗口布局、滚动后问号定位、手动说明卡跳转、无订阅入口/逐项WebView自测问号、旧时区信息不能标“当前”、未知未来状态降为“无法确认”。
- 结论：已实现界面的自动/像素验收通过；真实Wails原生文件对话框、辅助技术和Windows多缩放组合需实机验收。

## P5：本地检测中心

- 预期：基于真实设置，列出剩余可读取信息；不能混淆可写、可读、实际读取和管理员观察结果。
- 实际：只读策略、既有服务快照、WFP返回证据、动态系统时区、地区和默认设备权限记录；按网站/应用/网络显示证据、时间、范围、未知、措施及筛选。报告不触发网络/设备访问，不输出凭据、MAC、IP、序列号或选定程序路径。
- 证据：detection单测、vet通过；unknown不升级安全、配置不冒充运行时、默认Deny不冒充全局阻止、助手与观察者不同身份、缺失网络/规则及导出脱敏有回归。
- 复核修复：WFP查询失败/空返回不能声称“零规则”；服务可达不代表连接/闸已就绪；手动能力说明不生成伪执行入口。全字段路径脱敏，自定义规则ID改为报告内序号；事实注册表读取先限制大小。
- 导出：复用普通本地文件的严格路径与句柄验证，拒设备/UNC/ADS/重解析和覆盖；写/Sync失败尝试仅删除本次创建文件，不按可能被替换的路径盲删。
- 偏差：没有完整每应用行为审计、全部站点/浏览器配置或硬件访问审计。扩展本地结果不自动传到桌面；WebView只表示客户端来源。报告仍含用户主动导出的时区/地区和设置状态。
- 结论：只读模型自动验收通过，未独立实测的暴露继续保留未知。

## P6：浏览器配套

- 预期：正式API、明确作用配置、读回和控制权，不注入不一致指纹，不收集浏览信息。
- 实际：MV3本地选项页和本地后台协调器，privacy/contentSettings控制、撤销自己的覆盖层、版本不支持降级；手动追踪参数清理及敏感链接保留；扩展来源本地环境/接口自测。没有 host permissions、内容脚本、远程服务或遥测；后台协调器只串行化本地设置/恢复记录，CSP禁止网络。
- 证据：`node browser-extension/tests.cjs`内存Chrome API回归通过；权限冲突/读回失败/记录和撤销/参数清理有检查。扩展说明单独记录来源、站点例外与隐身限制。
- 偏差：未自动安装到用户浏览器，未验证所有实际浏览器版本；未验证后台 worker 终止/重启、多窗口和多配置竞争；无通用WebRTC关闭、完整WebGPU/字体/Canvas/Audio加噪或身份容器。
- 结论：扩展代码与mock验收通过；真实安装、策略优先级与网站行为待用户选定浏览器的隔离验收。

## P7：文件数据与手动引导

- 预期：只处理用户选定数据，优先新副本；未支持项目不能出现假开关。
- 实际：JPEG/PNG指定块检查和去除，校验结构/CRC/像素/大小上限；不返回元数据真实内容，保留编码像素和ICC。系统权限、MAC、深层反指纹和原生硬件范围有正式入口或说明。
- 证据：临时生成图片/错误结构/写失败/重解析路径/不覆盖/只删自身输出等测试通过。新增报告保存同样拒绝设备名（含DOS上标端口别名）、ADS与含糊路径。
- 偏差：其他文件格式、文档修订/作者、原有诊断深层预览、DPAPI迁移和复制后清理等保留在功能台账，不算本版完成。
- 结论：支持的本地工具自动验收通过；其他项为引导或后续工程。

## P8：综合回归与原目录复核

- 预期：原连接流程不受新增控制失败影响，不改原目录；新增权限与导出错误不能放宽保护。
- 最终自动回归：
  - `go test -count=1 -tags with_quic,with_utls,with_clash_api,with_gvisor ./internal/... ./cmd/godusevpn-svc ./cmd/godusevpn-cli ./web`通过，见`go-regression.txt`。
  - 同范围`go vet`通过，见`go-vet.txt`（无诊断）。
  - `go test -count=1 -tags desktop,production ./cmd/godusevpn`和同范围vet通过，见`desktop-regression.txt`/`desktop-vet.txt`。
  - 扩展、DOM、JS语法和最终`web`回归通过，见`browser-regression.txt`/`ui-dom-regression.txt`。
  - 架构contract和daemon依赖检查通过，隧道不依赖新隐私模块。
- 所有live开关关闭：`GODUSEVPN_WFP_LIVE=0`、`GODUSEVPN_WFP_KEEP=0`、`GODUSEVPN_ULTRA_REGISTRY_LIVE=0`。没有用自动测试更改真实策略/WFP/时区、安装服务或弹UAC。
- 原目录：420个tracked文件SHA-256全部一致，原Git状态干净，见`source-integrity.json`。只恢复新副本中76个没有语义变化的换行改动，减少无关diff。
- 结论：开发自动门槛通过。无真实订阅/隔离VM的Windows实机组合验收仍未完成，不能由单测替代。

## P9：Windows 构建交付

- 预期：x64 GUI/CLI/服务、独立资源、可复用打包脚本和可审阅的安装/恢复范围。
- 已实现：`build-ultra.ps1`，3个独立manifest/版本资源；便携ZIP、SHA256SUMS、描述符、配套扩展/计划/说明；目标已有时不混入旧产物。普通GUI manifest保持asInvoker，助手单独UAC。
- 安装脚本：固定ProgramFiles下的`GodUseVPN Ultra`新安装；全payload哈希、路径/重解析/全树ACL和复制后校验。包括子EXE可写性复核，避免仅根目录安全却在提权执行前被替换。卸载先机器策略/自有WFP清理，再本版服务卸载；错误保留恢复工具。只读WhatIf、无全局进程名kill、不递归删文件、不恢复替代账号HKCU；首版保留全部文件与数据。
- 上游Inno脚本在副本中明确禁止用于Ultra，防止其AppID、原服务和进程名清理碰到原版。原项目该脚本保持不变。
- 实际构建：`./build-ultra.ps1 -SkipTests`成功；先前独立最终回归已记录在P8。产物为`dist/godusevpn-ultra-0.1.0-ultra.1-windows-amd64.zip`及同名目录，包含GUI、CLI、服务、扩展、说明、计划、状态台账和校验清单。
- PE复核：三个二进制均为Windows AMD64，ProductName=`GodUseVPN Ultra`，ProductVersion=`0.1.0-ultra.1`；实际RT_MANIFEST的assembly identity均独立，executionLevel=`asInvoker`。通过PE资源目录读取而不是在内嵌DLL的XML字符串中猜测。工具已保存为`tools/verify-ultra-package.cjs`并纳入后续构建。
- 前端一致性：二进制中确实内嵌当前`privacy.js`/`privacy.css`/`app.js`/`index.html`完整内容，不是旧页面。文件大小、SHA-256和验证结果见`build-results.json`。
- 只读运行：CLI与服务`version`通过；CLI `privacy report`返回26项固定策略，`privacy detection`生成42条有范围说明的结果。只在进程内解析数量，不保存本机完整设置/指纹报告；见`smoke-results.json`。
- 安装器验证：PowerShell 7及Windows PowerShell 5.1 Parser通过；假包WhatIf、额外payload拒绝和内存ACL拒绝测试通过。真实生成包的安装与卸载WhatIf通过，见`install-preview.txt`/`uninstall-preview.txt`；没有执行安装、cleanup、服务启停、UAC或网络连接。
- 文档最终补齐后同步到包中并重新计算SHA256SUMS/ZIP；ZIP校验放在同目录的`.zip.sha256`侧文件，避免包内引用自身校验造成循环。
- 偏差：本次交付仅x64未签名开发预览；ARM64模块编译已有检查，整套ARM64发行包未构建。实际安装/卸载与真实隧道、WFP、浏览器生效未冒充通过。
- 结论：Windows开发预览构建与只读交付门槛通过；正式发行验收及台账中后续开发项仍未关闭。

## 尚未关闭的需求

完整逐功能状态见`FEATURE_STATUS.zh-CN.md`。语言/设备名自动修改、独立身份配置、凭据迁移、复制后清理、回环隔离/首次联网等仍需开发；真实系统策略、浏览器与WFP效果仍需实机验收。整体项目没有被标为“全部完成”。

## 后续全量审计与 VPS 准备（2026-10-19）

- 预期：重新按 R01–R14 及全部功能台账核对实现、恢复、UI、简洁文案、遗漏与可增加功能，明确确认问题和待实测结果。
- 结果：新增 `AUDIT_REPORT.zh-CN.md`，记录21项问题/缺口（P1 1项、P2 12项、P3 8项）、逐需求核对、文案候选、功能扩展及57条 Windows 11 Pro 实测用例。应用代码在本次审计中未修复，问题仍待关闭。
- 已确认的首要分享风险：原诊断文本脱敏仍保留机器/网络标识，而界面鼓励直接分享。只用虚构网卡文本的 Go overlay 复现，没有采集真实网卡配置。
- 其他确认项包括旧证据失效、助手重连、扩展跨页并发恢复、单规则移除结果、卸载维护窗口、广播范围、检测遗漏及导出快照等。证据类型与精确触发条件见各 A01–A21 条，不把静态推理统一称为真实复现。
- 对 P3 早期记录中“原闸 hard permit”的概括作精度修正：仅所审 `permitSelf` 明确带 CLEAR_ACTION_RIGHT，不能把 Tun/LAN/回环全部归为硬放行；实际仲裁仍需实测。本轮没有因此改动旧 guard。
- 应急恢复新增缺口：机器策略仍沿用交互用户约束；有已管理 HKLM 项时，默认 SYSTEM 云命令不能完成策略恢复。不能仅删除 SafeUser 条件或把 WFP 可移除当作全部机器设置均可恢复。
- 远端只做明确管理端口的协议预检：RDP/CredSSP 返回公告版本 `10.0.26100`；未输出服务器挑战中的主机标识，未发送账号密码或完成登录。SSH/WinRM 尚未建立可用命令通道，未导入测试订阅或更改远端设置。
- 用户确认拥有 Azure 管理后台，已打开官方登录页面交由用户登录。快照、运行命令与独立恢复能力尚未核实；可能切断 RDP 的测试尚未执行。
- 新增 `VPS_TEST_RUNBOOK.zh-CN.md` 记录分批步骤和阻塞条件；新增 `qa/Inspect-TestEnvironment.ps1`，仅输出最少只读环境 JSON。PS5.1 Parser 零错误、7项纯函数验证通过；未因此执行完整远端检查。32位宿主漏查浏览器时保留 unknown，不误报不存在。
- 官方接口复核补充：符合版本/配置条件的本地字体授权、网页访问本机/内网、WebHID 等可以分别通过正式浏览器策略继续实现；只列为候选，不算本版已实现，也不扩大为所有原生应用禁读。
- 原目录再次复核：420个 tracked 文件哈希一致，Git状态干净。此次新增文档/检查脚本位于 Ultra 副本；现有预览 ZIP 和程序未重新构建，不包含本轮新增审计文件。
- 结论：候选代码/需求/UI审计形成可审阅文件；修复和真实 Windows 组合验收仍未完成。下一步先核实恢复入口和环境，再关闭已确认问题并逐批实测，每批回到计划复核。这里的“审计”不是发布终审。

## Azure 隔离测试：环境与文件暂存（2026-10-19）

- 已通过用户登录的 Azure 后台运行只读环境检查，结果保存在 `vps/preflight-2026-10-19.json`。Windows 11 Pro 25H2 / x64，BFE、Edge 与 WebView2 可用；云命令是 SYSTEM，不能代表桌面用户的 HKCU/APPDATA。
- 串行控制台已认证，重连后短命令 `ver` 成功，并得到 `QA_ADMIN=TRUE`。早期令牌检查显示 Interactive SID 存在且非 SYSTEM；早期完整 JSON 有串行显示缺字，不用它单独证明提升。
- 用户批准 1 美元快照预算，已创建 1 份匹配系统盘的 Standard_LRS 完整快照，DenyAll、127 GiB、Succeeded、完成百分比 100。尚未实际恢复磁盘；快照保留期间持续计费，预算不等于 Azure 自动费用上限。
- ZIP 在本地、Cloud Shell、远端分块及最终合并各环节核对固定 SHA-256；13块均通过，远端合并成功。只在私有 QA 暂存目录写文件，没有安装服务或运行产品。
- 新增 `qa/Stage-VpsPackage.py`、`qa/Expand-VpsCliSmoke.ps1` 和 `qa/Vps-ReadOnlySmoke.ps1`，分别限制传输、全包校验后仅提取3文件、固定 CLI 两条只读命令；独立审阅与本地静态/纯函数验证通过。各步骤不可混称为只读，SYSTEM 报告也不是 GUI 用户验收。
- 第一次临时调度文本在 Python 解析阶段因引号错误失败，未调用 Azure 或解压；后续改用独立语法检查的调度脚本。用户已批准首次运行未签名测试 CLI 的 `version` 与 `privacy report`，实际运行结果尚待记录。
- 带外会话曾无回显，重连恢复。串行命令可用与快照存在仍不等于产品恢复已演练；目前不执行可能切断远程通道的防直连/WFP测试。
- 证据与范围：`vps/session-2026-10-19.json`；程序哈希保持原预览包版本，测试订阅尚未导入，原项目未被本轮工具修改。

## Ultra 品牌标识调整（2026-10-19）

- 最新明确要求：整体图片正方形、主体居中；右上角小写 `ultra` 加粗、更换草写字体、颜色贴合；可轻微重叠，无外圈。抽屉已获认可，保持现有小写细体字标。
- 最终实现：400×400 SVG在界面显示为80×80；原主图几何中心与方形中心重合。主字标由Gabriola字形排版后转曲，加粗为紫蓝渐变，不分发字体文件、不依赖设备安装字体。没有外框、底色或阴影。抽屉保持300字重、9.5px、0.8px字距及6px间距。
- 核对：`qa/brand-square-qa.cjs` 在320/420宽、深浅色、1×/2×六个组合检查正方形、居中、转曲边界、无框、抽屉样式，均通过；mock页面无JS错误/外部请求。实际图片为`screenshots/ultra-brand-square-*`；旧refined/vector/type/signature截图仅为历史方案。
- 单独方形矢量图：`assets/ultra-logo-square.svg`。生成工具与几何来源为`qa/build-signature-wordmark.py`；无需在应用运行时加载字体或执行生成器。
- 边界：品牌本身不表示任何隐私保护已启用。固定v1 VPS包及旧v2构建均未覆盖；后续使用独立v3输出，v1测试不能替代新包验收。

## Azure 隔离测试：首批 CLI 实测完成（2026-10-19）

- 三份小脚本改由终端传输，Cloud Shell 对完整源集合及每份文件 SHA-256 校验通过；未再依赖原生文件选择窗。调度脚本 Python 语法、两份 PowerShell 5.1 wrapper Parser、正确回执及3种错误回执拒绝检查通过，未用本地验证冒充远端执行。
- 远端实际 `expand`：Succeeded；固定ZIP匹配，32个条目、30个payload全部校验，仅提取CLI/校验清单/包描述3文件，CLI哈希匹配。
- 用户批准后实际 `smoke`：CLI `version` 与 `privacy report` 成功，版本 `0.1.0-ultra.1`；26项策略均存在，无缺项，managed=0、canApply=0。执行者为提升的SYSTEM、Interactive SID=false，26项均为manual/readStatus=not_checked；没有把管理员标志、用户journal健康或报告成功当成实际保护。
- 没有安装服务、启动GUI、应用策略、导入订阅、建立隧道或改动网络。证据见 `vps/readonly-smoke-2026-10-19.json`，只记录固定计数与范围，不保存原始策略/错误、账号、地址、云资源标识或凭据。
- 下一门槛仍为交互用户令牌、实际策略写入/读回/恢复及冲突，再验证浏览器接纳与有恢复保障的网络测试；A13等审计问题没有被这两条只读命令关闭。
- 原项目再次核对：420个tracked文件SHA-256全部一致，原Git状态干净。

## Azure 隔离测试：交互上下文与单项策略往返（2026-10-19）

- 预期：在已认证的交互管理员上下文核对固定v1只读命令；逐项获准后，测试一项机器策略的写前备份、应用、独立读回和恢复，不改变网络。
- 交互只读结果：26项策略均available/canApply，未发生读取失败或冲突，用户journal为空。SYSTEM报告和交互用户报告分别记录，未把任一报告当成机器journal已检查或功能运行时证明。
- 用户明确同意后，`windows-clipboard-history`真实往返完成：初始注册表值不存在、无目标备份；应用后独立读回匹配并观察到备份；恢复后原值仍不存在、目标journal条目移除。另一次SYSTEM云命令独立复核原值与journal计数。
- 证据：`vps/interactive-readonly-2026-10-19.json`、`vps/policy-roundtrip-2026-10-19.json`。固定脚本`qa/Vps-PolicyRoundtrip.ps1`及v1CLI哈希均核对；未保存凭据、账号、云资源ID、订阅或原始策略报告。
- 偏差与限制：首次交互路径格式错误被执行前检查拒绝，修正为固定本地路径后成功；串行长JSON显示缺字，改为私有结果文件并由独立云命令解析固定字段。未测试剪贴板实际运行行为、全部26项、GUI/UAC、服务、浏览器、订阅、隧道或流量。
- v1机器对象包含当时管理员的具名ACE，本批仅接受预先固定核对的该账号用于读取证据。新代码不接受这类旧对象；没有清理、重写或收紧VPS旧journal/lock来绕过新校验。
- 结论：一项真实策略恢复成功，未演练磁盘快照恢复；仍不足以开启全部断网测试。快照继续保留计费，1美元为已同意预算而非云端硬限额。

## 第二轮整改与逐步复核（2026-10-19）

- 预期：先修复会把本地输入或未知身份错误扩大为泄漏/保护副作用的核心边界，再修正检测作用域；保持隧道、guard及可选隐私模块独立。完整21项A与12项C/U台账保留，见`AUDIT_RECHECK_SUMMARY.zh-CN.md`，不把全部缺口叫作漏洞或宣称全部完成。
- C01/C05：主IPC发送前验证管道实际服务端、SCM/令牌/代码并持有句柄，发送前复查；响应ID不一致为结果未知。GUI/托盘未知分支不修复、登记、清闸或继续停止服务；确认缺席须新读取及SCM明确结果。独立二审发现并修复四处调用方分支回归。普通GUI→SYSTEM真实令牌兼容仍待VPS。
- C02：消费核心数据前验证owner/ACL/重解析/祖先及句柄，验证固定文件与四个消费目录；可选机器隐私journal不参与VPN启动判定。二审发现继承专用ACE可能赋予未来普通用户写权限，补齐模板检查后独立回归通过。没有接管、修ACL或迁移旧对象。
- A13/C06：机器上下文与交互HKCU分开，固定机器操作入口验证真实提升；新增机器对象只允许SY/BA，恢复旧受管值不被当前SKU变化挡住。旧具名账号权限或异常对象保留并拒绝，真实SYSTEM恢复与迁移仍待测。
- U01/A09：地区值经本地NLS验证和规范化；空/无效始终有unknown检测项，不回显或导出无效原串。U02：CLI说明当前进程用户备份范围、machineJournalChecked=false/not_inspected，不据用户journal推断机器健康。相关测试及独立回核通过；A15执行身份标签问题未关闭。
- U05/U07及A01提示：敏感链接帮助收窄识别范围；中英订阅提示说明更新会联系提供方；诊断提示改为先审阅。诊断内容、预览和本机Users读取范围仍待改，A01不因文案修改关闭。
- C04部分修复：用户journal在目录访问前拒绝UNC/设备/远程映射/已有重解析路径。严格只读lease拒绝数据写句柄；安全策略/当前时区事实仍可返回。独立Temp实验发现原lease可被原地改向；直接禁止写共享又会破坏原子替换，未隐藏这一回归。最终为只读严格、写入兼容分层，并用生产lock/ACL/durableWrite/read/restore的临时文件往返验证；写侧同账号并发原地重解析和主动DOS映射变化仍未关闭。不用假pin或全进程“零外联”宣传替代整改。
- 原核心回归发现CheckNow丢失唤醒：旧队列清理晚于Connected发布。新增同步OnChange确定性复现，先证实旧代码失败，再将清队列、发布和请求入队纳入同锁。20轮定向及整包通过；没有改原目录、隧道失败或guard算法。
- 最终Go回归通过：`qa/audit-final-regression-20261019.txt`；同范围vet无诊断。范围为全部internal、CLI/服务与web，原诊断设置副本用例单独用`qa/diagnostic-fixture-check.ps1`的合成系统文本及拒绝联网transport验证，未采集本机诊断。所有live开关为0。
- 桌面Go/vet、扩展内存回归、原DOM与新增身份DOM通过；界面headless四个深浅/宽窄组合20截图无错误/页面外联；品牌六组合方形/居中/字形边界通过。真实浏览器控制权、GUI/UAC、安装卸载、订阅与WFP/隧道流量仍未据此通过。
- 交付使用独立`0.1.0-ultra.3`输出；v1固定测试包与v2历史构建不覆盖。构建、PE资源及内嵌前端核对以新包的`docs/ultra/build-results.json`为准；包内保留审计相对链接对应的QA证据、最小VPS结果和演示截图，不自动执行QA脚本。
- 原目录再次复核420个tracked文件哈希一致、Git状态干净，见`source-integrity.json`。本批源码/fake与v1真实单项策略恢复分别记账；整个项目仍为未签名开发预览。

## 现有功能审计修复与 beta 范围同步（2026-10-19）

- 预期：用户确认先修复并验收现有功能后发beta，首次联网询问、独立浏览身份和通用凭据加密等继续列路线图。完整原A01–A21和第二轮C/U审计保留，不用删需求或免责声明绕过现有实现问题；根README、版本、发布与实机由独立步骤处理。
- A01/C03实际代码修复：诊断默认最小白名单，不依赖日志/配置/崩溃/系统网络命令；详情逐项选入并确认敏感性，同一ZIP的预览/缓存/导出及hash契约，GUI新建私有文件。验证后logs/diag及全部历史后代收紧SY/BA，不接管不可信目录。fake/合成TEMP/桥测试通过，详情仍可能识别个人；真实跨账户与桌面端到端待验。Web/Android详情/ZIP改为明确不支持，保留最小摘要，旧下载410，兼容收窄已记录。
- 前端/授权/扩展实际代码修复：桌面/扩展失败、回焦和管理操作后使证据失效，桌面报告另有到期限制（扩展没有定时TTL）；fresh认证IPC只映射安全摘要，分来源完成时间；显式授权single-flight、迟到结果拒绝、失败写不重试；页面世代防跨页旧确认、全局忙状态；导出缓存同一快照/选择/预览/相同字节保存。正式contentSettings按类型提供模式及明确HTTPS来源，帮助和错误引导补齐，成功/范围提醒分开。扩展现新增本地MV3后台队列与applying/restoring记录；早期P6“无后台脚本”描述为旧阶段，当前为无远程服务/内容脚本/host权限，不应再说没有后台worker。
- 后台A06/A07/A10/A15/A16/A17/A18/A21实际修复：unknown残留或提交后回读失败不误报移除；独立Global mutex与线程lease覆盖HKLM/WFP/时区及官方cleanup→复核→固定卸载桥，Busy不等待/不影响VPN。独立原用户时区CLI及仅建同步门禁helper不加载机器journal；CLI执行身份不冒充helper。DNS/日志/凭据finding只传安全摘要，unknown数量独立count，文件结果单独查看；guard说明仅permitSelf/CLEAR_ACTION_RIGHT，不笼统外推soft permit。两installer失败指向中英人工恢复说明，不自动续装/接管/删除。
- 维护边界：Interactive可占锁造成Busy，mutex不是授权/发布者认证。abandoned本次拒绝后要求核对，不是持久隔离标记。官方Uninstall脚本→CLI完整窗口不拦外部管理员SCM或旧独立svc uninstall命令；SCM配置核对不是运行PID认证。真实多进程、abandoned、UAC/替代管理员、SCM及WFP/时区效果待验。
- Windows pin新证据：独立TEMP实验确认原metadata-only句柄不能证明DELETE共享限制；C01/C02/A07补FILE_LIST_DIRECTORY并真实验证持有时DELETE open拒绝、释放后可open，普通创建/原子替换仍通过。C04严格只读拒writer已验；其写模式仍共享WRITE，同账号原地reparse/主动DOS映射残余继续部分修复未关闭。未以新pin说法掩盖该限制。
- 本轮验证：新增专用fake、TEMP句柄/私有文件/合成DACL、GUI隐私测试、七包test/vet、诊断/paths/桥定向回归及两installer Parser均通过；维护独立二审没有新增确认阻塞。扩展mock与四viewport/theme的28张合成截图通过，无页面异常；这些不是实际扩展加载、真实浏览器策略、UAC、SCM或流量验收。源码及fake互审通过不覆盖生产目录/真实用户数据。
- 当前证据：`qa/DIAGNOSTIC_MINIMIZATION_FIX.md`、`qa/FRONTEND_AUDIT_FIX.md`、`qa/MAINTENANCE_RECOVERY_FIX.md`及`qa/MAINTENANCE_SECOND_REVIEW.md`。`AUDIT_RECHECK_SUMMARY.zh-CN.md`、`FEATURE_STATUS.zh-CN.md`和原报告附记已同步；原报告的旧发现/行号/当时未执行状态仍保留，不拿历史v1/v3包证明新源码或新发行包通过。N06尚未落地的用途控制保持未实现，不把新增检测无HTTP扩大为全进程无外联。
- 工具偏差：一次未限定路径的Windows `python` launcher在本地编辑时自动更新Python安装管理器至26.4、下载并安装Python3.15.0；bootstrap确有网络请求，超出预定离线编辑/mock范围，已及时告知用户。它不是产品检测或产品默认外联，未有意提交项目/用户载荷，也不作网络零发生承诺。其后未卸载/回滚/改变全局环境，改用apply_patch和既有Node/Go，详见`qa/TOOL_RUNTIME_DEVIATION.md`。
- 结论：本批现有功能代码修复与有限本地验收完成，C04残余保留；真实Windows组合、受支持浏览器及新发行包仍需逐批核对。仅增加文档同步和内部WFP模块README精准文案，没有执行新的策略、WFP、UAC、SCM或网络操作。

## beta 候选文件与实机准备（2026-10-19）

- 用户再次明确发布范围：现有功能修复并验收后发 beta，首次联网询问、独立浏览身份、通用凭据加密等保留路线图。发布计划和中英说明已同步，不把修改文案当成关闭真实验收门槛。
- 冻结产品代码的最终全量 Go/desktop 回归与 vet 通过；安装版本兼容 54 项、真实 ZIP/PE/内嵌前端检查通过。`qa.1` 两段版本安装验证失败保留，修复后重建 `0.01-beta-qa.2`，未回写旧包。详见 `qa/BETA_CANDIDATE_QA.md`。
- VPS 新候选 delta 辅助包共 8 段，全部及合并 SHA 匹配。独立重建 wrapper 在真实私有 QA 路径完成三个文件重建，并独立检查落盘 SHA/ACL，`ready:true`、`productExecuted:false`。仅执行文件解码器，没有执行新候选程序、安装器、GUI、服务或网络动作；脱敏回执 `vps/beta-qa2-files-2026-10-19.json`。
- 新五命令只读 wrapper、四项 SYSTEM 策略往返 wrapper、完整原包组装 wrapper 均已准备并进行独立静态复核及 PS5.1 纯合成检查。真实策略/安装/身份/浏览器/流量验收继续单列；旧 v1 的单项结果不适用于新候选。
- 为完整包验收准备原 `qa.2` 的 300 个非 EXE 文件辅助 ZIP，固定原清单及全部字节，不修改原候选包。辅助传输/组包只处理文件，不自动安装或执行产品。旧 v1 空机器存储仍保留，独立归档操作需先严格核对，不自动改 ACL 或接管旧记录。

## qa.2 真实往返、MV3有限实测与qa.3门槛（2026-10-19）

- 本节更新上一阶段的“准备”状态，不覆盖历史失败。qa.2完整文件组装后，获准执行的五条SYSTEM只读命令真实通过：三个SHA/静态版本、CLI/服务运行时版本、36项控制、45项finding、零规则/未知过滤器；无GUI/服务启动和网络变更。观察者SYSTEM无Interactive SID、helperAuthorized=false，machineJournalChecked=false仍只说明当前用户报告范围，不能推断机器journal健康。证据：`vps/beta-qa2-readonly-2026-10-19.json`。
- 旧空store归档v1真实返回review_required，四项策略批次立即停止。独立远端读回源仍两文件canonical empty、legacyAllowCount=1、target不存在、四值缺失；新TEMP复现父级LIST/shareREAD pin阻止Directory.Move，Win32 32。失败脚本与`vps/beta-qa2-archive-attempt-2026-10-19.json`保留，未将moveAttempted冒充已经移动。
- 单独审查后，v2在第二次preflight/源快照/四值缺失后释放本批父pin，单次Move，再从盘根重新pin并核目标字节/owner/group/DACL与源缺失。真实completed，未修ACL、删除、回滚或执行产品；见`vps/beta-qa2-archive-completed-2026-10-19.json`。这段释放窗口依赖受信管理员和稳定命名空间，仅手动QA归档，不作产品原子迁移保证。
- 其后qa.2在SYSTEM下依次真实往返广告ID、在线语音、剪贴板历史、剪贴板云同步四个固定HKLM值，共八条apply/restore命令。四值初始缺失，各项写前备份/lock可见、独立读回匹配、restore回到缺失、受管记录移除。见`vps/beta-qa2-policy-roundtrip-2026-10-19.json`；这不是实际Windows功能或应用行为验收。
- ROOT另做独立恢复后只读检查并保存`vps/beta-qa2-post-restore-readback-2026-10-19.json`：新source两文件healthy canonical empty、lock0、legacyAllowCount0；保留旧target同样empty但legacyAllowCount1；四值均缺失，无产品/移动。本节据实际文件记录，不由先前report的machineJournalChecked=false反推。
- 当前开发宿主Edge155的新TEMP profile真实加载MV3 worker，产品panel Client→Coordinator完成WebRTC handling/预测/第三方Cookie设置与控制权/journal/恢复；通知仅example.invalid的block→ask→clear样本，仍verified=false。五个同时请求全回执、中间active、最终初值及空journal；详见`qa/MV3_EDGE_INTEGRATION_RESULT.json`/`.md`。不是VPS或原profile，不覆盖企业策略/其他配置/worker重启/扩展竞争；未导航网站、STUN、硬件或流量测试。仅页面四个本地资源请求不证明整个Edge零外联。
- C01安装模式改为fresh SCM当前PID/OwnProcess/规范LocalSystem配置及持有进程映像认证，移除普通UI查询SYSTEM primary token的前提；前台模式提升令牌/代码pin保留，SCM不明不降级。IPC/desktop测试与vet、独立二审通过；见`qa/IPC_SCM_COMPATIBILITY_FIX.md`及`qa/IPC_SCM_AUTH_SECOND_REVIEW.md`。配置账号不是已观察runtime SID，普通medium用户pipe/SCM/进程查询与完整GetState仍是发布前待验。
- 因此新建immutable `0.01-beta-qa.3`，本地build、352文件/351payload的完整ZIP/PE/六内嵌前端、包内安装/卸载WhatIf通过；其固定文件传输、三EXE重建和352文件组装也在真实VPS私有目录完成，但未执行产品或安装器。后续qa.3产品只读命令、普通用户IPC、GUI/UAC、安装/恢复、订阅/隧道/WFP流量均未被qa.2或上述Edge测试替代。候选摘要见`qa/BETA_CANDIDATE_QA.md`，未回写qa.2/qa.3原包。
- 最近`source-integrity.json`记录原目录420个tracked文件与b51aed4一致、Git干净，只证明该次基线比对；本轮文档整理未执行产品、真实设置修改、浏览器或VPS命令。C04写侧残余、磁盘快照实际还原及未实现路线图继续保留。

## qa.3 预检兼容性复核（2026-10-20）

- 冻结的 `Vps-Qa3Install.ps1` v1 在真实机上的一次 preview 以 `SCM_unknown` fail-closed；回执为 `whatIfStarted=false`、`productExecuted=false`、未启动官方安装器 `-WhatIf`，因此不能计作预检或安装通过。该结果保留在独立执行记录中，v1 不回写、不复用。
- 纯 PowerShell 5.1 binder 复核确认 v1 的 `[string]` P/Invoke 参数不能可靠表达 `OpenSCManagerW` 所需的 NULL database 指针。已准备 IntPtr NULL-safe 的 v2 安装预检及 `Vps-Qa3ScmReadbackV2.ps1` 只读交叉核对脚本；在取得真实机 readback 前，SCM 缺失仍保持 `unknown`，不能猜测为不存在。v2 安装预检的 79 项/11 个虚构场景和 SCM readback 的 22 项 AST 检查仅是本地脚本契约。
- `Vps-Qa3SessionFacts.ps1`、`Vps-Qa3MediumStatus.ps1`、`Vps-Qa3ReadOnly.ps1` 及 v2 安装预检的本地纯函数/AST 测试只证明脚本契约，不证明真实令牌、SCM、安装、GUI/UAC、服务、浏览器或流量。测试 VM 关闭后，以上项目继续保持待验，不能用旧 qa.2 证据继承。
- 文档核对未发现测试 VM 地址、登录凭据、订阅链接、Azure 资源标识、令牌或真实指纹写入 Ultra 文档/证据；Bastion Developer 会话只记录布尔状态和失败边界，未记录连接秘密。公开仓库审计仍必须在提交前再次扫描。

## qa.4 候选只读验收（2026-10-20）

- 预期：验证冻结 qa.4 包与三个 PE，执行五条固定只读命令和 SCM 只读回读；不安装、不启动 GUI/服务、不改设置或网络。
- 传输：18 个分段、`stage.json`、`preview.zip` 均在私有 QA stage；远端 `preview.zip` 为 37,349,587 字节，SHA256 `c8d87b386180459caefe4372630c5f25ef2c1ffd960b89e7687ce236702c0580`，与本地一致。Cloud Shell 两次断线后使用同一 stage-id 恢复，未把中途失败误记为完成。
- 解包：受控脚本只提取三个固定 EXE 到 SY/BA 私有 candidate，回执 `status=prepared`、`files=3`、`sha256Match=true`。
- 只读命令：`Vps-Qa4ReadOnly.ps1` 真实回执 `status=completed`、`hashesMatched=3`、`staticVersionsMatched=3`、`commandsStarted=5`、`commandsPassed=5`、`terminationUnknown=false`；CLI 与服务版本为 `0.01-beta-qa.4`。36 个控制、45 条 detection finding（observed 2、possible 28、unknown 14、manual 1）、firewall 规则 0；GUI/服务/设置/网络/运行时保护均未启动或请求。
- SCM：qa.4 v2 只读回读为 `status=completed`、SYSTEM、SCM 打开成功；`godusevpn` 与 `godusevpn-ultra` 均缺席且错误码 1060，CIM 查询成功、服务计数均为 0，未执行产品或请求变更。
- 与预期差异：第 1 阶段包传输、固定五条只读命令和 SCM 回读已通过；普通 medium 用户、v2 安装预检、官方安装器 WhatIf、安装/卸载、GUI/UAC、策略写入/恢复、浏览器/隧道/WFP/流量仍待验收。

证据文件：[`qa4-readonly-2026-10-20.json`](vps/qa4-readonly-2026-10-20.json)、[`qa4-cleanup-2026-10-20.json`](vps/qa4-cleanup-2026-10-20.json)。文件仅保留固定包摘要、固定计数、作用域和清理布尔值，不保存云资源标识、订阅、账号、地址或原始策略内容。

## 历史本地审计复核（尚未达到发布终审，2026-10-20）

- Go：完整构建标签 `with_quic,with_utls,with_clash_api,with_gvisor` 的 `go test ./...` 与 `go vet ./...` 均通过。
- PowerShell：qa.4 只读 wrapper AST/合成测试通过；SCM v2 AST 测试 22 项通过。错误地调用不存在的 `Assemble-VpsQa4Package.Tests.ps1` 未计入产品失败；该脚本并无对应测试文件。
- 前端/浏览器 mock：brand 6 profiles、privacy 4 profiles/28 screenshots、diagnostic UI、IPC identity UI、time QA 均通过；无页面错误或外联。依赖使用仓库现有 `tools/qa/node_modules` 与 Edge 可执行文件。
- 包与发布扫描：PE/ZIP synthetic audit 25 checks 通过；`audit-publication.cjs` 最终 `findings=[]`，源清单 SHA256 `82155a01494eef79e19a6f2813f0aebf8f13f89ccc2057757929b8337c6ff31b`；运行时敏感标识扫描为空；原项目 420 tracked 文件完整性记录仍 `pass=true`。
- 清理：临时 gist、qa-transfer、本次 Azure 资源组均已删除；最终 `az group exists` 为 `false`。未创建公开仓库、标签或 Release，因为安装/普通用户/UAC/恢复/浏览器全面矩阵/隧道与真实流量门槛仍未通过。

## 修复后候选与下一步（2026-10-21）

- 计划核对：发现 `Install-Ultra.ps1 -WhatIf` 中 `Get-FileHash` 会被 WhatIf 偏好短路，随后发现首次加载 CIM 模块产生的 `Set-Alias` ShouldProcess 提示会使严格预演回执拒绝；两项均已修复，源脚本和 qa.4 目录脚本保持一致。
- 本地核对：修复后的真实候选目录 `-WhatIf` 退出码为 0，输出固定版本/406 payload/目标/警告/唯一 ShouldProcess 动作；没有写入安装目录或 SCM。ZIP 37,933,559 字节、SHA256 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`，清单 SHA256 `2b42f32810d8f27a0bcd7126f217946a41a5ee95b5f4b3c5c69453163f24ea48`。
- 自动核对：ZIP/恶意成员夹具 44 项、安装预检 AST/内存回执 114 项、接收器合成、只读/SCM、Go test/vet 和公开发布扫描均通过；公开扫描 `findings=[]`。此前 qa.4 固定包哈希证据因脚本修复失效，已在候选记录中明确标为历史证据。
- 实机核对（当时状态）：最新 19 分片任务已启动，目标是同一临时 Windows VM 的新 stage；该段随后由“2026-10-21 实际安装与清理批次”更新，最终取得合并 SHA、Expand、实际安装和服务/CLI 回读。中途漏跑 Expand 的偏差已保留并记录。
- 当前偏差与恢复：包体积跨过 18 片上限，传输协议和接收器末片校验已同步为 19 片；旧 stage/gist 仅作临时传输，最终均删除。普通用户、UAC、实际安装/卸载、恢复冲突、浏览器全面矩阵、网络/WFP/隧道仍未通过，不能发布 beta。

## 2026-10-21 实际安装与清理批次

- **范围/预期**：在用户授权的一次性 Windows VM 上使用修复后 `0.01-beta-qa.4`，先快照，再合并/展开，运行真实安装器，独立读回 SCM/CLI，执行官方卸载，最后删除资源组。只记录实际证据，不把 SYSTEM Run Command 当作普通用户或 UAC。
- **环境证据**：一次性 Windows Server 2022 Core VM；临时快照创建成功（`DISK_FOUND=yes`、`SNAPSHOT_READY`）。云资源名称不写入公开树。
- **包准备**：私有 stage 的 19 个分片合并；补跑 Expand 后回执 `ready=true`、`files=407`、`hashesMatched=3`、`sha256Match=true`、`privateChecked=true`。首次直接安装时漏跑 Expand，安装器不存在；该偏差没有系统写入，补跑后重试。
- **实际安装**：安装器输出 `Verified unsigned Ultra 0.01-beta-qa.4, 406 payload files`、`Installed Ultra at C:\Program Files\GodUseVPN Ultra`、`INSTALL_EXIT=0`。普通用户/UAC 未覆盖。
- **服务/CLI 回读**：`SERVICE_COUNT=1`、`SERVICE_STATE=Running`、`SERVICE_STARTNAME=LocalSystem`、服务路径为 `"C:\Program Files\GodUseVPN Ultra\godusevpn-svc.exe" service`、PID=1572；目标目录和 CLI 存在；CLI/service 版本均为 `0.01-beta-qa.4`；`privacy report` 退出码 0，报告保留 `machineJournalChecked=false/not_inspected`。
- **卸载边界**：官方 `Uninstall-Ultra.ps1` 预检通过，`privacy uninstall-machine` 返回 1；脚本保留文件/数据/恢复工具并明确警告，没有删除或假报完成。独立匹配服务卸载随后返回 `SVC_UNINSTALL_EXIT=0`、`SERVICE_COUNT_AFTER_SVC=0`、`SVC_STATUS_AFTER=not-installed`。机器清理未通过，不能写成官方完整卸载通过。
- **云清理**：已提交 `az group delete --yes --no-wait`；资源列表为空，轮询期间短暂返回 `true`，最终 `az group exists` 返回 `false`。临时资源已清理完成。
- **边界与下一步**：订阅链接只保留在当前操作上下文，未写入项目、gist、日志或 JSON；本批未启动 GUI、未导入订阅、未做 WFP/流量/浏览器测试。实际安装、服务启动、CLI 只读和独立服务清理在 SYSTEM 作用域通过；官方完整卸载为安全失败；普通用户/UAC、策略往返、浏览器、隧道/WFP/流量仍待验收。

## 2026-10-21 候选本地全量审计（尚未达到发布终审）

- Go 完整标签 `go test -count=1 -tags "with_quic with_utls with_clash_api with_gvisor" ./...` 通过；同标签 `go vet ./...` 无诊断。
- PowerShell QA：Expand hostile fixture 44 项、安装预检 114 项、接收器合成、qa.4 只读 AST、SCM v2 AST 均通过；均明确未启动产品或官方 WhatIf 主流程。
- 前端/UI：brand `profiles=6, errors=[], outbound=[], square=true, centered=true`；privacy `screenshots=28, profiles=4, errors=[], blockedRequests=[]`；time `screenshots=25, profiles=4, errors=[], outboundRequests=[]`；audit `screenshots=20, profiles=4, errors=[], outboundRequests=[]`；diagnostic 和 IPC identity DOM mock 均通过。
- 包审计：`audit-built-package`/`verify-ultra-package` 对 406 payload、407 ZIP 成员、三项 amd64 PE、前端资源和 SHA256 通过；ZIP SHA256 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`。`audit-publication.cjs` 返回 `findings=[]`；其 source manifest 以最终命令输出为准。
- 来源与清理：原项目 `git status --short` 为空；Ultra `git diff --check` 仅有换行符提示、无空白错误；临时安装传输目录和 helper 不存在；临时 gist 列表为空；敏感资源/凭据扫描为空。
- 发布结论：当前本地实现、包一致性、UI 和实际 SYSTEM 安装证据可复核；普通用户/UAC、官方完整卸载成功、真实订阅隧道、WFP/流量、浏览器完整矩阵仍未通过，故不创建公开仓库、tag 或 beta Release。

## 2026-10-21 开源组件边界、浏览器运行时清单与 qa.6 候选

- 新增 `internal/locationadapter`：只接受 HTTPS 上游、MIT/Apache-2.0 许可、固定版本和 SHA-256 的本地组件；写入前独立读回和备份，应用后再次读回，失败或不一致立即尝试恢复并保留备份。没有后端、管理员资格、固定哈希或独立回读时拒绝写入；不执行任意 shell，不影响隧道、路由、DNS 或本机网络。当前没有随包携带或执行 locsim 二进制，因此 UI 不显示虚假的“全局 GPS 已开启”。
- 新增 `internal/browserhost` 和 `App.BrowserHostReport()`：Windows 只读发现 WebView2、Edge、Chrome 的注册信息和版本，CEF 明确列为未提供；不读取路径、用户目录或命令行，不启动、下载或修改浏览器。隐私中心显示运行时清单和范围说明；专用 profile、代理绑定、参数编辑和随机配置仍是 P3 门槛。
- 重新构建 `0.01-beta-qa.6`：完整 Go 测试/`go vet`、扩展和 UI 测试通过；包目录审计与 ZIP/PE/前端资源审计通过。ZIP 为 425 个 payload、426 个成员，SHA-256 `453440fc904fc86c45c9dad4bbf0bcbf494b598efd4e92815ca02ad94f4b9652`，三项 amd64 PE 哈希与清单匹配，仍为 unsigned candidate，runtime version 尚未由审计工具执行。
- qa.5 候选包曾在临时 Windows VPS 完成 19 个分片的逐块校验、合并大小和 SHA-256 校验；Cloud Shell 后续输入控件把命令拼接到旧缓冲，未继续发送安装/删除命令，避免误操作。该控制台故障不计为产品失败，也不把 qa.4 安装结果迁移到 qa.6。临时公开传输文件已删除；VPS 上的暂存包将在下一次干净 Run Command 或新临时 VM 中继续安装验收。
- 本阶段仍只称“候选静态与有限暂存审计”，不称发布终审；普通用户/UAC、qa.6 实际安装、GUI、浏览器完整矩阵、GPS 后端往返、隧道/WFP/流量和恢复冲突仍未关闭。

## 2026-10-21 qa.5 实际安装与独立回读

- Azure VM Run Command 在临时 Windows 11 VM 上先按精确服务/目录/重解析点边界清理旧 qa.4；旧 GUI 运行时安装器返回 `gui_running`，随后只关闭/停止 2 个安装目录内的 Ultra GUI 进程。
- qa.5 ZIP 重新按顶层版本目录展开后，官方安装器成功；服务独立回读为 Running、LocalSystem、路径精确，CLI 与服务版本均为 `0.01-beta-qa.5`，`privacy report` 退出码 0。
- 三项独立落盘哈希：GUI `1308ab26b723a458b56f97aa3cfd14cbde89611c2f2be9803c74c5af5c5207f5`、CLI `70a330c1591b0dcf892314774c8b9cd9e817da50da66f199c3fd4185e4505abd`、服务 `21b33927a69063a7a2c58170bd4820f09741da76e629abaf36e40b0b0b3a8ae2`。
- Bastion 标准用户提交后由 Azure 表单清空并提示 `Value is required`，没有取得普通用户、UAC 或桌面 GUI 证据；本批没有订阅、隧道、WFP、GPS 后端或流量测试。qa.6 仍是当前本地候选，不能继承 qa.5 的安装回执。
## 2026-10-21 需求回顾与开源集成计划重列（当前仍未达到发布终审）

- 预期：重新核对全部对话要求，把可实现保护、受控开源组件、只能检测/引导的项目分开，并让 UI 文案保持简短易懂。
- 实际：更新 `COMPLETE_EXECUTION_PLAN_2026-10-21.zh-CN.md`，新增功能交付表、locsim/CEF/uBlock/系统接口准入矩阵、WebRTC/WebGPU/GPS/MAC/硬件噪音/端口规则边界、UI 短文案和 P0–P10 阶段门槛；同步更新 `OPEN_SOURCE_INTEGRATION_PLAN.zh-CN.md`。
- 偏差/风险：locsim 尚未在 disposable Windows VM 完成 apply/readback/reset 往返；CEF 专用浏览器尚未构建；uBlock 仍是用户主动安装的候选扩展。普通用户/UAC、qa.6 实装、完整浏览器/隧道/WFP/GPS 实测仍未关闭。
- 范围核对：按用户此前确认，`v0.01-beta` 只发布已实现并实测的现有功能；GPS、CEF、uBlock、独立浏览身份和首次联网询问等未完成工程保留路线图，不用静态边界冒充 beta 已交付。
- 下一步：继续在临时 Windows VPS 完成 qa.6、管理员/普通用户 GUI、浏览器矩阵、GPS 适配器和网络恢复测试；每批回到计划核对，现有 beta 门槛全部通过后再构建 `v0.01-beta` 并发布。

## 2026-10-29 QA12.13/QA12.14 旧版真实候选安装回归

- Win10 1809 build 17763 与 Win10 21H2 LTSC build 19044 均使用同一 `0.01-beta-qa.12.11` 候选包；18 个传输分片按文本 SHA-256 校验，合并 ZIP 为 55,147,100 bytes，包 SHA-256 与本地清单一致。
- 两台 VM 的 `Install-Ultra.ps1 -WhatIf` 和实际安装均 exit 0；GUI、服务、CLI 文件存在；`godusevpn-ultra` SCM 服务为 Running、LocalSystem；CLI 版本和隐私报告读取成功（36 controls、14 capabilities、4 warnings）。
- 通过 SYSTEM Run Command 以临时标准账户启动 GUI 时，Azure 环境没有交互用户桌面，`Start-Process -Credential` 返回 Access denied。这是旧版交互式 GUI 的环境限制，记录为 LIMITED；没有改动本机网络设置，也没有把服务证据扩展为 GUI、浏览器、代理或隧道 PASS。
- 本批临时 VM、OS 磁盘、NIC、公共 IP、NSG、VNET 和临时 GitHub gist 已删除；资源组最终只保留既有 Bastion 公共 IP、共享 NSG、共享 VNET 与 restore 快照。详细记录见 [QA12.11](qa/QA12_11_WIN10_19044_VM_2026-10-29.md) 与 [QA12.12](qa/QA12_12_WIN10_17763_VM_2026-10-29.md)。

## 2026-10-29 开源工具与 UI 全量审计

- 对照项目计划、交接需求、功能台账和源码完成开源集成矩阵：确认 sing-box、WFP/WireGuard、ExifTool、WebView2 host、sevenzip 校验链有源码入口；Chrome/Firefox 目前是官方下载与完整性校验链，CEF、uBlock、locsim、真实 GPS 写入后端未集成，未把它们显示为已启用。
- 修复浏览器启动门控：普通构建必须同时满足 WebView2 `hostAvailable`/`canLaunch` 或 Chrome/Firefox 本地版本树校验、版本校验和 `canLaunch=true`；否则按钮禁用并显示待运行验收。
- 全量静态、Go、DOM、扩展、像素、诊断、隐私审计、时区和品牌视觉回归通过；新候选包 `0.01-beta-qa.12.12` 已重建并完成包审计。详细矩阵见 `qa/OPEN_SOURCE_INTEGRATION_AUDIT_2026-10-29.md`。
- 供应链剩余限制：尚无完整 SPDX/SBOM；ExifTool 使用固定 SHA 的镜像来源且无发布签名证明；Chrome CfT 归档没有发布者 digest。真实浏览器宿主、GPS 写入、CEF/uBlock、WFP/代理出口/真实流量继续保持未完成或待 VM 验收。

## 2026-10-30 QA12.13 Win10 21H2 GUI 与浏览器宿主 VM 实测

- 在一次性 Azure Windows 10 Enterprise LTSC 2021 / 21H2 / build 19044 VM 上使用验收包 `0.01-beta-qa.12.13`；18 个分片逐片校验，合并 ZIP 55,328,238 bytes，SHA-256 `63e2acec6f4bfee7985f756f1b1545ec3a2d21de93e7300660d37537b2a584a3`。
- 官方 WhatIf、实际安装、`godusevpn-ultra` Running/LocalSystem、CLI 版本和 `privacy report` 独立回读通过；普通用户 Bastion 桌面两次启动 Ultra GUI，隐私中心、浏览器保护、GPU/硬件预览页面完成截图视觉核验，GUI 关闭后按安装路径精确清理。
- VM 回读 WebView2 `156.0.4314.50` 与 browser-host 7 个文件；浏览器设置页仍显示“需完整安装”，启动按钮保持“启动尚待验收”，没有把宿主存在误报为真实浏览器窗口或代理出口通过。Bastion Developer 成功提供一个普通用户桌面，并按 SKU 限制拒绝并发会话。
- 本轮临时 VM、Developer Bastion、OS 磁盘、NIC、临时公共 IP、NSG、VNET、19 个临时 gist 和本地传输目录均已删除；资源组最终仅保留既有四项资源；本机网络设置未修改。详细记录见 [QA12.13](qa/QA12_13_BROWSER_HOST_VM_2026-10-30.md)。
