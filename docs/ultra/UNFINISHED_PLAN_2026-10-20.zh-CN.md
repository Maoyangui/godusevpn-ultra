# GodUseVPN Ultra 未完成计划（2026-10-20）

这份文件是对全部对话要求、原项目计划、第二轮审计和当前证据的合并清单。它只列仍未完成或必须重新验收的事项；已经通过的本地测试、qa.2 有限实测和 qa.3 文件组装不重复当作新的通过证据。

## 当前边界

- 工作目录是 `C:\Users\Administrator\Desktop\友善\godusevpn ultra`，原项目 `C:\Users\Administrator\Desktop\友善\godusevpn` 保持不改。
- 目标平台是 Windows x64。当前源码分支仍是开发中的 `codex/windows-privacy`，尚未创建公开仓库、正式标签或 GitHub Release。
- 当前候选 `0.01-beta-qa.4` 已重新构建；它是未签名、未安装的测试包。PE、ZIP、前端资源、源码敏感信息和本地自动化测试已通过。
- 临时 Azure 测试机 `ultra-qa-win` 已创建并确认 Windows Server 2022 与 Run Command 可用；qa.4 只读验收期间临时启动，完成清理后必须停止并删除整个临时资源组。
- 原测试机 `test` 已经被 Azure 删除，旧 qa.2 结果不能替代新候选的实机验收。

## 必须完成后才可以发布 v0.01 beta

### 1. 新候选实机只读批次

1. 启动临时 Windows 测试机，确认运行状态、Windows PowerShell 5.1 x64、SYSTEM Run Command、磁盘和 ProgramData 路径边界。
2. 通过受限的分块传输把 `0.01-beta-qa.4` 固定包放入私有 QA 目录；重新核对 ZIP、三个 EXE、`SHA256SUMS`、`ULTRA_PACKAGE.json` 和传输脚本 SHA。
3. 先运行 qa.4 版本的 `Vps-Qa4ScmReadbackV2.ps1` 临时只读副本。它使用 `IntPtr.Zero` 传递 `OpenSCManagerW` 的真实 NULL，并用 CIM 交叉核对 `godusevpn` 与 `godusevpn-ultra` 服务；失败、超时或未知都保持 `rejected/unknown`。
4. 再运行 qa.4 只读命令、服务版本、隐私报告、检测中心和 firewall-list；记录 SYSTEM 令牌、机器 journal 是否检查、命令数量、未知数量和所有退出状态。
5. 运行 `Vps-Qa3SessionFacts.ps1`，只读记录普通交互会话、medium token、EnableLUA、用户 profile 和临时 profile。没有真实 medium 会话时写 `unknown`，不猜测。
6. 运行 v2 安装预检和官方 `Install-Ultra.ps1 -WhatIf`。预检必须证明环境空、服务不存在、包完整、脚本没有改变状态；任一 SCM、ACL、路径或终止状态未知就停止。

### 2. 实际安装、服务和普通用户验收

这一步会运行未签名候选程序、创建 LocalSystem 服务并改变机器级 IPC/隐私设置，开始前要在动作时确认具体命令和范围。

1. 在快照或可恢复 VM 上安装固定 hash 的候选；安装器只允许全新安装，任何部分目录或外部服务冲突都停止并保留恢复工具。
2. 验证服务 SCM 配置、当前 PID、`OwnProcess`、LocalSystem 身份、持有的映像路径和 SHA；服务身份未知时不启动隧道、不清理 guard、不自动替换服务。
3. 用普通 medium 用户执行完整 `GetState`、IPC 连接、错误响应 ID、服务缺席和服务重启矩阵；验证访问被拒绝时结果是未知/不可用，不重试写入。
4. 验证 GUI 请求固定管理员助手时的 UAC 同意、取消、替代管理员、助手退出、父窗口关闭、迟到回执和重新授权；不把 WebView 自身提升为管理员。
5. 仅在成功读回和写入证据存在时，验证一个固定机器策略和一个固定用户策略的 apply/readback/restore；外部冲突保留并提示。

### 3. 安装中断、恢复和卸载

1. 在预检、复制、服务注册、启动和 cleanup 各阶段制造一次可恢复失败，检查脚本是否停止、保留工具/日志、不给出“已完成”假象。
2. 重新启动 VM 后检查服务、机器策略、用户策略、journal、WFP 对象和安装目录；验证原子恢复和冲突检测。
3. 在普通用户和 SYSTEM 两个作用域分别核对时区、HKCU/HKLM 备份边界；SYSTEM 不得假称已经恢复另一用户的 HKCU。
4. 执行官方卸载并逐项读回：服务、程序目录、隐私策略、WFP 自有对象、维护锁、journal 和恢复文件。任何不确定对象都保留，不递归删除。

### 4. 真实网络、隧道和 WFP 验收

1. 先建立空白基线，再导入测试订阅；确认订阅刷新、节点必要请求和原有隧道能建立。
2. 对固定 EXE 分别验证全阻断、局域网限制、组播/有限广播、代理代发、IPv4/IPv6 和已存在连接；读回过滤器不能代替流量结果。
3. 单独验证回环 `localhost`、本地代理端口、IPv4/IPv6 端口扫描路径、DNS、直连回退和 TUN/WFP 仲裁；任何未覆盖路径显示 `unknown`。
4. 验证可选隐私失败不会断开 VPN、清除原 guard、删除隧道或静默切换直连；隧道失败仍按原项目规则处理。
5. 记录对投屏、共享、语音、视频会议、局域网发现和现有连接的影响；不把端口、WFP 或 TUN 的单项结果扩写成“所有应用都不能联网”。

### 5. 真实浏览器与扩展验收

1. 在新的 Edge 和 Chrome 测试 profile 中加载 MV3 扩展，逐 profile 验证 WebRTC、网络预测、预连接、Cookie、通知、定位、摄像头和麦克风的 block/ask/allow/clear。
2. 在浏览器策略页和实际页面分别读回；设置值、扩展回执和网站实际行为分开记录。
3. 验证 worker 重启、多个窗口、并发请求、恢复 journal、企业策略接管、隐身 profile 和外部修改；不能确认时保留 `verified=false`。
4. 保留浏览器安全浏览、证书验证、自动更新和 SmartScreen；隐私增强不能靠关闭这些安全机制实现。

### 5A. UI 和文案最终验收

1. 每个隐私功能、检测结果和不可用能力都必须有轻量的 `?` 帮助入口；悬浮或点击只显示一两句直白说明，不能使用需要猜测的术语。
2. 可由用户选择的功能必须提供明确的开关、输入框或选择框；系统无法修改或应用层无法保证的项目必须显示为“引导/仅检测”，不能伪装成已生效的开关。
3. 检测中心必须按当前设置逐项列出：应用或网站可能读取的内容、观察者、作用域、最近读回时间、证据来源、当前状态（已阻止/可能仍可读/未知）和建议动作。
4. 主 logo 必须保持正方形画布、主体居中；`ultra` 使用小写、加粗、轻量的艺术草写字形，允许轻微遮挡主体，不添加外包围圆圈。抽屉中的 `ultra` 使用当前抽屉字体的小写样式，避免与主 logo 混用。
5. 在 100%、125% 和 150% Windows 缩放，以及窄窗口、键盘导航、高对比度和中英文文本下复核溢出、遮挡、焦点顺序、帮助提示位置和状态颜色；任何“已保护”文字都必须对应可追溯证据。

### 6. 最终全量审计

1. 在最终树上运行 Go 测试（默认测试和完整构建标签分别记录原因）、`go vet`、桌面/扩展/前端/PowerShell 5.1 测试、真实 ZIP wrapper、PE/资源校验和构建脚本。
2. 重新运行源码敏感信息扫描、包成员扫描、README/Markdown 链接检查、原项目 420 个 tracked 文件完整性核对和最终构建 hash。
3. 复核检测中心每条结果都有来源、时间、作用域、观察者身份、未知状态和建议动作；不能把设置存在、规则存在或策略读回写成“网站/应用一定无法读取”。
4. 复核所有失败分支：隐私模块失败不影响隧道；服务/助手身份未知不清闸；恢复冲突不覆盖外部修改；路径/重解析/远程映射不扩大读取范围；C04 同账号主动改向残余继续标为部分修复。
5. 将每个阶段写入 `docs/ultra/EXECUTION_LOG.md`，同时更新 `FEATURE_STATUS.zh-CN.md`、`AUDIT_RECHECK_SUMMARY.zh-CN.md` 和 `BETA_CANDIDATE_QA.md`。没有证据的项目保持待验收。

### 7. 创建公开仓库并发布

1. 在最终审计通过后，从最终树建立新的公开仓库 `Maoyangui/godusevpn-ultra`；使用新的根提交，不携带原项目历史、测试机信息、密码、订阅链接或 Azure 资源标识。
2. 推送源码、中文计划、英文 README、恢复说明、浏览器扩展说明和已核对的 Windows x64 beta ZIP；README 必须写清未签名、支持范围、安装前备份、失败恢复和未覆盖场景。
3. 创建精确标签 `v0.01-beta`，发布说明只写实际通过的 Windows 范围、包 SHA256、已知限制和路线图；不能把 qa.2、mock、静态审计当作真实安装/流量通过。
4. 发布后再次从 GitHub 下载包做独立 hash、PE、ZIP 成员、README、敏感信息和版本号核对；发现差异就撤回候选，不修改测试证据来匹配包。

## qa.4 候选只读验收（2026-10-20）

- 预期：在临时 Windows VM 上只读核对 qa.4 固定包、三个 PE、CLI/服务版本、隐私报告、检测中心和 firewall-list；不安装、不启动 GUI/服务、不导入订阅、不改设置或网络。
- 实际：通过不依赖文件选择窗口的短期隐藏传输，将本地 `0.01-beta-qa.4` 包分块写入 VM 私有 `ProgramData\GodUseVPN-Ultra-QA\<stage-id>`；18 个分段、`stage.json`、`preview.zip` 均存在，远端 `preview.zip` 大小 `37349587`，SHA256 `c8d87b386180459caefe4372630c5f25ef2c1ffd960b89e7687ce236702c0580`，与本地一致。传输期间 Cloud Shell 两次断线，后台/同 stage 恢复后由独立查询确认，未把中途失败误记为完成。
- 受控准备脚本只提取三个固定 EXE 到新建 SY/BA 私有 candidate 目录，并逐个核 SHA；结果 `status=prepared`、`files=3`、`sha256Match=true`。没有解压安装器、脚本、扩展或用户文件。
- `Vps-Qa4ReadOnly.ps1` 真实回执：schema `ultra-qa4-readonly-v1`、SYSTEM、`hashesMatched=3`、`staticVersionsMatched=3`、`commandsStarted=5`、`commandsPassed=5`、`terminationUnknown=false`；CLI 与服务版本均为 `0.01-beta-qa.4`。报告含 36 个控制、检测中心 45 条 finding（observed 2、possible 28、unknown 14、manual 1）、firewall 规则 0；`guiStarted=false`、`serviceStarted=false`、`settingsOrNetworkChangesRequested=false`、`runtimeProtectionTested=false`。
- `Vps-Qa4ScmReadbackV2.ps1` 真实回执：SYSTEM、SCM 打开成功，`godusevpn` 与 `godusevpn-ultra` 均不存在且 Win32 错误码 1060；CIM 查询成功、两个服务计数均为 0；未执行产品、未请求变更。
- 与计划核对：第 1 阶段中的包传输、固定五条只读命令和 SCM 只读回读已通过；普通 medium 用户、真实安装/卸载、GUI/UAC、策略写入/恢复、浏览器/隧道/WFP/流量仍未验收，不能由本批结果替代。临时 VM、私有 QA candidate、Cloud Shell 和短期隐藏传输内容在最终清理前仍需处理。

## Beta 后路线图

以下需求在所有对话中被保留为独立工程，不作为首个 beta 的假完成项：

- 首次联网询问、目的地白名单、用途级外联控制和明确的无人操作默认行为。
- 独立浏览身份、可验证的设备/语言/时区一致性策略和更完整的浏览器站点例外。
- 通用凭据迁移与 DPAPI/硬件保护；管理员可读边界、迁移残留和回滚必须先设计。
- 全面的回环/端口扫描隔离、N05 跟踪分类、N06 外联用途选择、更多 Windows 权限和企业策略覆盖。
- WebGPU、字体、硬件噪声、GPS、MAC/设备名等指纹面只能按浏览器/应用/沙箱能力逐项处理；不能承诺一个管理员权限开关阻止所有应用。
- macOS 版本、签名/公证、跨平台发布和新的测试矩阵。

## 每一步的固定核对格式

每个阶段都必须记录：预期动作、实际动作、证据文件和 SHA、是否改变系统、与预期的差异、失败后的恢复状态、结论（通过/部分通过/未通过）以及下一步。任何 `unknown`、超时、身份不明、读回失败或外部冲突都不能自动升级为成功。

## 2026-10-21 实际安装 VM 批次复核

本批目标是补齐修复后 `0.01-beta-qa.4` 的真实安装、服务和 CLI 证据，并在测试完成后删除一次性 Azure 资源。使用的是临时 Windows Server 2022 Core VM；这是 SYSTEM Run Command 环境，不是普通 medium 用户或桌面 UAC 会话。云资源名称与 stage ID 不写入公开树。

| 阶段 | 预期 | 实际与证据 | 差异/恢复 | 结论与下一步 |
|---|---|---|---|---|
| 快照 | 安装前保留可恢复系统盘 | 临时快照创建成功；Azure CLI 返回 `DISK_FOUND=yes`、`SNAPSHOT_READY` | 未演练还原；只保留快照直到批次结束 | 通过；后续只在需要时做独立还原演练 |
| 分片/合并/展开 | 19 段合并后清单、三项哈希和私有目录全匹配 | 私有 stage（ID 不写入公开树）；Expand 回执 `ready=true, files=407, hashesMatched=3, sha256Match=true, privateChecked=true` | 第一次直接调用安装器时漏跑 Expand，安装器不存在；未写入系统，随后补跑 Expand 后重试 | 通过；将“先 Expand 再安装”作为门禁 |
| 实际安装 | 固定包安装到 Program Files，注册匹配的 LocalSystem 服务 | 官方安装器输出 `Verified unsigned Ultra 0.01-beta-qa.4, 406 payload files`、`Installed Ultra at C:\Program Files\GodUseVPN Ultra`、`INSTALL_EXIT=0` | Server Core 无桌面 UAC；不能证明普通用户安装体验 | 部分通过；继续服务/CLI与卸载验收 |
| 服务/CLI | SCM、服务身份、目标目录和版本读回 | `SERVICE_COUNT=1`、`SERVICE_STATE=Running`、`SERVICE_STARTNAME=LocalSystem`、路径精确为 `godusevpn-svc.exe service`、PID=1572；目标和 CLI 存在；CLI/service 版本 `0.01-beta-qa.4`；`privacy report` 退出码 0 | 报告明确 `machineJournalChecked=false/not_inspected`；未把 SYSTEM 观察范围扩写为所有用户 | 通过当前作用域；普通用户 IPC/GetState 仍待验 |
| 官方卸载 | 机器清理成功后只移除匹配服务并保留恢复数据 | `Uninstall-Ultra.ps1` 预检通过，但 `privacy uninstall-machine` 返回 1；官方脚本按设计保留文件/数据/恢复工具，服务仍在 | 失败原因是本批没有可供机器清理恢复的机器策略状态；没有递归删除或假报完成 | 部分通过/安全失败；记录为产品边界问题，不改证据 |
| 独立服务清理 | 在机器清理失败后，验证匹配服务可独立移除 | `godusevpn-svc.exe uninstall` 退出码 0；`SERVICE_COUNT_AFTER_SVC=0`、`SVC_STATUS_AFTER=not-installed` | 这是清理测试环境，不替代官方完整卸载通过 | 通过服务清理；删除临时资源组 |
| 资源清理 | 删除 VM、磁盘、网卡、快照等一次性资源 | 已提交 `az group delete --yes --no-wait`；资源列表为空，最终 `az group exists` 返回 `false` | 删除为异步，轮询期间曾短暂返回 `true`；最终已确认不存在 | 通过；本地暂存与临时 gist 已清理 |

本批没有运行 GUI、浏览器页面、真实订阅隧道、WFP/流量或普通用户/UAC；这些门槛仍按第 4、5 阶段保持待验收。收到的订阅链接仅用于可能的临时网络测试，不写入仓库、日志或证据文件。
