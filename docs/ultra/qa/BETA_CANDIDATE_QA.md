# beta 候选与冻结回归核对

2026-10-19。仅 Ultra 副本；本记录不是正式发布完成证明。

## 已验证

- 冻结产品代码的 Go 全量测试（internal、CLI、服务、web）及对应 vet 通过；desktop/production GUI 测试和 vet 通过。live 注册表/WFP 标志均关闭。证据为 `beta-final-regression-20261019.txt`、`beta-final-desktop-20261019.txt`；vet 成功无诊断输出。
- 原全量回归首次发现诊断 CSS 的 accent-color 和缺少 flex gap 回退；已去掉不兼容属性并补回退，最终完整回归通过。保留最初失败日志，不将首次运行写成通过。
- 扩展 core、共享队列、桌面 DOM、扩展/launcher DOM、服务身份 UI 和诊断 UI 回归通过。固定 mock 的主品牌六配置与隐私、检测、诊断弹层视觉检查分别保留；它们不是实际策略生效证明。
- 发行校验器 25 项合成 PE/ZIP 用例、安装版本 54 项合成用例通过。这组本地校验没有运行实际安装或产品程序；后续获准的 VPS 命令另列如下。
- 候选 `0.01-beta-qa.1` 成功构建，但包内安装/卸载验证器仍只接受三段版本，`-WhatIf` 拒绝两段标签。已修两份源码脚本并构建新的 `qa.2`；旧产物保留，不在原包内替换脚本或改写校验和。
- `0.01-beta-qa.2` 的实际 ZIP、全部 303 个文件成员/字节、302 项清单、三个 EXE 的真实 PE 版本/权限/产品身份和六个内嵌前端文件均通过检查。包内安装与卸载 `-WhatIf` 通过，未创建安装目录或服务。

| 候选对象 | SHA256 |
|---|---|
| qa.2 ZIP | `d6cd9cf1e5591f4d62491b65e76a3a03b141766e6461c667e2dcbb9e610a07eb` |
| qa.2 GUI | `8aec81db98b683cb5de06fe6e3939f3cd493fc4018a7015d90a72da90f3d61eb` |
| qa.2 CLI | `7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e` |
| qa.2 服务 | `e9105cf5fe9f7e1167455a6654ff7ed3900f24308023663725ebd7f88729b14f` |

产品字符串为 `0.01-beta-qa.2`，Windows 数字版本 `0.1.0.0`。静态 PE 检查不执行 EXE；其 CLI/服务运行时版本随后由获准的 VPS 只读命令核对。

VPS 已完成固定 delta 辅助包的 8 段上传、合并 SHA 校验与三个 EXE 的真实文件重建。包装器随后独立读回三个落盘文件的 SHA 和私有 ACL，返回 `ready:true`、`hashesMatched:3`、`productExecuted:false`。其余 300 个非 EXE 文件经另一固定辅助包的 5 段传输/合并校验后，在新目录组成完整候选包：303 文件的实际字节/原 SHA256SUMS、私有 ACL 和三个静态版本均通过。这一文件组装步骤没有运行候选程序、安装器或服务。脱敏回执见[文件重建与组装](../vps/beta-qa2-files-2026-10-19.json)。辅助包用于传输测试文件，不是正式发行 ZIP。

## qa.2 已取得的真实 VPS 证据

获本批授权后，`qa.2` 的五条 SYSTEM 只读命令真实通过：三个 SHA/静态版本、CLI/服务运行时版本、36 项控制报告、45 项 finding 和零应用规则/零未知过滤器。观察令牌为提升的 SYSTEM、无 Interactive SID，helperAuthorized=false；报告中的用户范围及 `machineJournalChecked:false` 保留。该批不代表登录用户、机器 journal 健康、GUI 或过滤效果，见[五命令回执](../vps/beta-qa2-readonly-2026-10-19.json)。

随后旧空备份归档 v1 返回 `review_required`，立即停止四项策略批次。独立远端只读复核发现源仍为原处的两文件健康空记录、归档目标不存在、四个策略值均不存在。本机新 TEMP 实验复现父级 read-share pin 阻止 `Directory.Move` 的 Win32 32；失败脚本和[失败回执](../vps/beta-qa2-archive-attempt-2026-10-19.json)保留，没有自动重试或删记录。

经单独审阅的新归档 v2 在二次快照/四值检查后释放本批父句柄，只移动一次，再从盘根重新核验。其[真实回执](../vps/beta-qa2-archive-completed-2026-10-19.json)为 completed：两文件字节和 owner/group/DACL 保持，源缺失、目标为空、四值仍缺失；未修 ACL、删除、回滚或执行产品。释放到重验之间仍有窗口，这是受信管理员稳定命名空间中的手动 QA 归档，不是产品原子迁移。

其后 `qa.2` 在 SYSTEM 下依次完成四项固定 HKLM 策略的真实 apply/独立读回/restore，共八条产品命令：广告 ID、在线语音、剪贴板历史、剪贴板云同步。四值初始均不存在；每项观察到 journal/lock、读回匹配、恢复原缺失状态并移除受管条目，见[四项往返](../vps/beta-qa2-policy-roundtrip-2026-10-19.json)。这证明的是该系统版本上的注册表及机器恢复链，不证明相应 Windows 功能、所有应用或网络行为。

另一次[独立只读后验](../vps/beta-qa2-post-restore-readback-2026-10-19.json)确认：新机器 store 为两文件 canonical healthy empty、lock=0、legacyAllowCount=0；归档旧 store 仍为两文件 canonical empty、legacyAllowCount=1；四策略值均不存在。这次检查未执行产品或请求移动，没有为通过新代码校验而重写旧对象。

## 当前宿主 MV3 有限实测

[真实 Edge 结果](MV3_EDGE_INTEGRATION_RESULT.json)使用当前宿主 Edge 155 的独立新 TEMP profile，真实加载 MV3 worker，经产品 panel Client→Coordinator 完成 WebRTC handling、预测和第三方 Cookie 的设置/控制权/active journal/恢复往返。通知仅对 `example.invalid` 做 block→ask→clear 的 API 抽样，仍为 `verified=false`，不推断所有站点；五个同时请求全部回执，中间 active、最终值及 journal 回到初始状态。

这是设置 API 与后台队列的有限验收，不是 VPS GUI、企业策略、所有配置或 worker 重启/竞争验收。没有网站导航、STUN、硬件探测或流量测试；页面只观察到四个本地扩展资源请求，不是整个 Edge 进程零外联证明。范围详见[记录](MV3_EDGE_INTEGRATION.md)。

## qa.3 候选与下一门槛

SCM 认证兼容修复已进入新候选 `0.01-beta-qa.3`。本地实际构建、完整 ZIP 的 352 文件/351 payload 清单、三个 PE 及六个内嵌前端文件、两份安装脚本 `-WhatIf` 已通过；没有实际安装。ZIP 为 36,616,538 字节，SHA256 `50f9b657569da793ef7893cee554c2fc45ba807d8ed9ed040a9941f0419dad27`；证据为 `beta-qa3-build-20261019.txt` 及该候选的 ZIP audit/PE 结果。qa.3 的真实 VPS 命令、安装服务、普通用户 `GetState`、GUI/UAC、恢复和网络批次仍待执行，不能套用 qa.2 通过。一次真实机 v1 preview 已按 fail-closed 结果结束：`SCM_unknown`、`whatIfStarted=false`、`productExecuted=false`，未执行官方 `-WhatIf`，因此不能写成安装预检通过。冻结 v1 保留原样；IntPtr NULL-safe 的 v2 安装预检与 `Vps-Qa3ScmReadbackV2.ps1` 已完成本地脚本检查（安装预检 79 项/11 个虚构场景，SCM readback AST 22 项），尚未取得真实机回执。

qa.3 的固定传输/重建/组装文件批次随后在真实 VPS 私有 QA 目录完成：delta/support 合并哈希匹配，3 个 EXE 重建和 352 文件组装均返回 `ready:true`、清单/静态版本/私有 ACL 复核通过；`productExecuted:false`、`installerExecuted:false`。这只是文件准备与磁盘清单证据，不能替代 qa.3 五条只读命令、安装、普通用户、GUI/UAC 或网络验收，详见[文件批次回执](../vps/beta-qa3-files-2026-10-19.json)。

[C01 修订及本地二审](IPC_SCM_AUTH_SECOND_REVIEW.md)不再要求普通 UI 读取 SYSTEM primary token，而以 fresh SCM 当前实例及持有的实际映像认证；这是 SCM 配置/实例证据，不是观察到 runtime SYSTEM SID。普通 medium 用户的管道/SCM/进程查询权限和完整 `GetState` 是新候选发布前待验门槛。

原目录最近一次[完整性复核](../source-integrity.json)记录 420 个 tracked 文件与 `b51aed4` 一致、Git 状态干净；这只证明该次原目录比对，不代表运行时验收。

R01–R14 的完整状态仍见当前审计台账。C04 残余不关闭；未实现的新工程继续保留路线图；没有因为本地回归或打包成功就创建标签、公开源码或宣称全部验收完成。发行源码及说明将从最终审过的提交重新构建，后续文档变化不回写候选 ZIP。

## qa.4 只读候选实机结果（2026-10-20）

本节记录的是安装器修复前的 qa.4 只读包，后续候选已重新计算清单与 ZIP 哈希；它不作为当前候选的安装预演证据。当前候选见下节。

本地候选 ZIP `dist/godusevpn-ultra-0.01-beta-qa.4-windows-amd64.zip` 大小 37,349,587 字节，SHA256 `c8d87b386180459caefe4372630c5f25ef2c1ffd960b89e7687ce236702c0580`。通过受限分段传输后，VM 私有 QA stage 的 `preview.zip` 独立读回同样的大小和 SHA；Cloud Shell 断线只触发同 stage 恢复，不改变成功判定。

受控解包只提取三个固定 EXE，三项哈希与候选清单一致。qa.4 只读 wrapper 在 SYSTEM 下完成五项固定命令：CLI/service 版本均为 `0.01-beta-qa.4`，36 个控制，检测中心 45 条 finding（observed 2、possible 28、unknown 14、manual 1），firewall-list 0 规则；五项命令均通过，未启动 GUI/服务、未安装、未改设置或网络。

SCM v2 只读回读确认 `godusevpn` 与 `godusevpn-ultra` 服务缺席（1060），CIM 计数均为 0。普通用户、UAC、安装/卸载、策略往返、浏览器、订阅、隧道/WFP 和流量仍是发布前门槛；本结果不能替代这些验收。

脱敏证据：[qa4 只读回执](../vps/qa4-readonly-2026-10-20.json)；[qa4 清理回执](../vps/qa4-cleanup-2026-10-20.json)。

## qa.4 修复后候选复核（2026-10-21）

预期：修复 `Install-Ultra.ps1 -WhatIf` 在 PowerShell `WhatIfPreference` 下无法读取哈希，以及首次加载 CIM 模块产生额外 ShouldProcess 别名提示的问题；重新生成清单、ZIP 和分片后，在临时 Windows VM 独立合并、解包、预演并补做实际安装/服务回读。

结果：本地预演返回 `Verified unsigned Ultra 0.01-beta-qa.4, 406 payload files.`、固定 Program Files 目标、唯一安装动作预演，退出码 0；没有创建目录、服务或运行产品。ZIP 目录审计通过，清单为 406 行，包为 407 个文件成员；`Expand-VpsQa4Package.Tests.ps1` 44 项、`Vps-Qa4InstallV2.Tests.ps1` 114 项、接收器合成测试及既有只读/SCM 测试均通过。Go 全量 test/vet 也通过，公开发布审计仍为 `findings=[]`。

当前候选固定值：ZIP 37,933,559 字节，SHA256 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`；清单 SHA256 `2b42f32810d8f27a0bcd7126f217946a41a5ee95b5f4b3c5c69453163f24ea48`；安装器 SHA256 `487a811743e7f1955633af41e47fcf09353393e1364aa200007b6b1e105a176d`。三个 EXE 哈希保持 GUI `c321a62437521c6c8bfcb5b290c78396dd5fb9b7a2464c2c626d064794b8bafc`、CLI `53107fddbbbea4b11030d1aaede6beae6839b27ea7b4412d0e6fadbd9d4044be`、服务 `b2ede7f86ebe73b4d8e5b3010507357b6f9eb27af4585f2e0f760189cad083f8`。

修复后包重新分为 19 个 2 MiB 上限内的分片。实机已收到新的 `PACKAGE status=merged sha256=match`，Expand、安装器、服务和 CLI 回读均有证据。普通 medium 用户、UAC、策略往返、浏览器完整矩阵、隧道/WFP 和流量继续保持待验；不把旧 qa.4 的实机回执迁移到新包。
### 修复后候选的真实安装批次

同一修复后 ZIP（37,933,559 字节，SHA256 `fed0866073a3f022800e8f70123af57dd6358e477944863c2c1a33b176dc8144`）在一次性 Windows Server 2022 Core VM 上完成了展开、实际安装和服务/CLI 回读。安装器返回 `INSTALL_EXIT=0`，目标为 `C:\Program Files\GodUseVPN Ultra`；SCM 回读为一项 `godusevpn-ultra`、Running、LocalSystem、OwnProcess 路径精确匹配，CLI 与服务版本均为 `0.01-beta-qa.4`，`privacy report` 返回 0。证据中的观察身份是 SYSTEM，报告保留 `machineJournalChecked=false`，不代表普通用户、UAC 或另一用户 HKCU。

首次调用安装器时 QA 流程漏跑 Expand，远端没有安装器文件；该次没有系统写入。补跑固定 Expand 后回执为 `files=407`、`hashesMatched=3`、`sha256Match=true`、`privateChecked=true`，随后安装成功。

官方 `Uninstall-Ultra.ps1` 预检通过，但机器清理返回 1；脚本按设计保留文件、数据和恢复工具，没有递归删除或假报完成。独立执行匹配的 `godusevpn-svc.exe uninstall` 后退出码 0，SCM 计数 0、服务状态 `not-installed`。这是测试资源清理证据，不把官方完整卸载写成通过。临时 Azure 资源组资源列表为空，最终 `az group exists` 已返回 `false`。

本批仍未覆盖普通 medium 用户、桌面 UAC、GUI、浏览器完整矩阵、真实订阅隧道、WFP/流量和应用端口扫描；这些状态继续保持待验收，不能由 SYSTEM 安装批次替代。

## qa.6 开源边界与暂存核对（2026-10-21）

qa.6 在加入浏览器运行时清单和位置适配器边界后重新构建。完整 Go 标签测试、`go vet`、扩展/UI 测试、包目录审计、ZIP/PE/前端资源审计通过；包为 425 个 payload、426 个 ZIP 成员，ZIP SHA256 为 `453440fc904fc86c45c9dad4bbf0bcbf494b598efd4e92815ca02ad94f4b9652`。静态验证没有执行 EXE，runtime version 仍标为未独立验证，unsigned 预览不等于发布签名。

GPS/位置组件只接入受控适配边界：当前没有携带或执行 locsim，未核验来源、版本和哈希时不能应用；位置范围只覆盖后端声明的 Windows 接口，不能改变 GeoIP、Wi-Fi、账号历史或物理 GPS。浏览器运行时清单只读发现 WebView2、Edge、Chrome；CEF、专用 profile、代理绑定、参数随机化仍未交付，不显示伪开关。

qa.5 包在临时 Windows VPS 完成 19 分片下载、逐块 SHA256、合并大小和整包 SHA256 核对。之后 Azure Cloud Shell 输入控件出现旧命令拼接，停止继续发送安装/删除命令；没有把该控制台异常归因于产品，也没有把 qa.4 的安装证据迁移给 qa.6。临时公开传输文件已删除，下一批使用干净 Run Command 或新临时 VM 继续安装、普通用户/UAC、GUI、浏览器、GPS 后端、隧道/WFP/流量和恢复冲突验收。

### qa.5 实际安装批次（2026-10-21）

通过 Azure VM Run Command 在同一临时 Windows 11 VM 上完成 qa.5 安装：旧 qa.4 程序目录先通过精确路径、服务路径、无重解析点检查后清理；发现旧 GUI 仍运行时，安装器按预期返回 `gui_running`，随后仅关闭/停止 2 个路径精确匹配的 Ultra GUI 进程。重新展开 ZIP 顶层目录后官方安装器完成，独立回读为 `godusevpn-ultra=Running`、`StartName=LocalSystem`、路径精确，CLI 与服务版本均为 `0.01-beta-qa.5`，`privacy report` 退出码 0。

三项落盘哈希独立回读：GUI `1308ab26b723a458b56f97aa3cfd14cbde89611c2f2be9803c74c5af5c5207f5`、CLI `70a330c1591b0dcf892314774c8b9cd9e817da50da66f199c3fd4185e4505abd`、服务 `21b33927a69063a7a2c58170bd4820f09741da76e629abaf36e40b0b0b3a8ae2`。这批仍是 SYSTEM Run Command 证据；Bastion 标准用户提交后被 Azure 表单清空并提示 `Value is required`，因此普通用户、UAC 和桌面 GUI 继续标为未知，不把管理员凭据或服务回读当成通过。

本批没有导入订阅、建立隧道、应用 WFP、执行 GPS 后端、改变本机网络或测试流量；这些门槛仍待下一次干净桌面会话/临时 VM。
