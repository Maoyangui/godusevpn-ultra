# qa.2 首批安装、身份与卸载验收准备

日期：2026-10-19。**仅计划，未执行。本文件不构成执行授权。** 使用已核对的完整 qa.2 包：303 文件（302 payload+SHA256SUMS），原 manifest SHA256 `9f4d7836cefd477fed1b1f16247ae1c389aa7eed818c11f18a9185c8fc8eaf6d`。不添加订阅、不连接隧道、不装应用限制、不测试断网，不启动 GUI/UAC。

## 批次前提

以下条件全部确认才进入安装；查询错误属于未知，不能当作不存在：

- 系统 PowerShell 5.1 x64、真实 SYSTEM，Azure Run Command 恢复通道可用；现有快照与未演练还原的限制继续保留，不把快照当已验证恢复能力。
- 原版 `godusevpn` 和 Ultra `godusevpn-ultra` SCM 服务均不存在；原版 ProgramFiles/godusevpn 与 Ultra ProgramFiles/GodUseVPN Ultra 均不存在。任何既有对象先停本批，不能自动删除、替换或接管。
- ProgramData/godusevpn-ultra 仅允许本轮新建、已核 SY/BA 的 `privacy-machine` 子目录；其 journal 为当前 canonical empty、managed=0，四个测试值已恢复缺失。legacy 对象必须走独立审核/授权的归档批次，不能在安装中顺便归档。
- 核心 settings/state/profiles/controller 等旧状态均不存在，无服务数据环境覆盖 `GODUSEVPN_ULTRA_DATA`；没有原版/Ultra 残余活动过滤器或待恢复网卡状态。存在或无法读取则停，不用 clear/repair 消除异常。
- 新组装目录与安装脚本仍为批准版本；303 文件均由原 manifest 覆盖，无 extra/reparse/hardlink，三 EXE 为固定 qa.2 SHA 和 ProductVersion。不得以当前源码 README/脚本覆盖该包里的旧文件。

下列变量在私有 QA 会话内赋值，文档仅使用占位符，不记录真实账号、StageID、SID、IP 或云资源标识：

```powershell
$ErrorActionPreference = 'Stop'
$stageId = '<candidate-stage-id>'  # 已验证的32hex UUID4
$programData = [Environment]::GetFolderPath([Environment+SpecialFolder]::CommonApplicationData)
$programFiles = [Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)
$package = Join-Path (Join-Path (Join-Path $programData 'GodUseVPN-Ultra-QA') $stageId) 'package-beta-qa2'
$installed = Join-Path $programFiles 'GodUseVPN Ultra'
$cli = Join-Path $installed 'godusevpn-cli.exe'
$svc = Join-Path $installed 'godusevpn-svc.exe'
```

变量赋值不代替已审 root-first no-follow/private ACL/pin 与 manifest 核对。SCM 只读检查使用固定 Name，不按显示名称或 basename 猜测：

```powershell
$original = @(Get-CimInstance Win32_Service -Filter "Name='godusevpn'")
$ultra = @(Get-CimInstance Win32_Service -Filter "Name='godusevpn-ultra'")
if ($original.Count -ne 0 -or $ultra.Count -ne 0) { throw 'service_baseline_not_empty' }
if (Test-Path -LiteralPath $installed) { throw 'target_exists' }
if (Test-Path -LiteralPath (Join-Path $programFiles 'godusevpn')) { throw 'original_target_exists' }
if ($env:GODUSEVPN_ULTRA_DATA) { throw 'data_override_present' }
```

## 顺序、具体命令与通过条件

| 步骤 | 命令/操作 | 实际副作用与通过条件 |
|---|---|---|
| 1 只读预演 | `& (Join-Path $package 'Install-Ultra.ps1') -WhatIf` | 读取清单、ACL、目录及 SCM；不执行 EXE。必须确认固定 fresh 目标、303 inventory，无修改成功假象。保存固定通过布尔及退出状态。 |
| 2 首次安装 | `& (Join-Path $package 'Install-Ultra.ps1')`，由 SYSTEM 执行 | 会创建 ProgramFiles 目录、复制/保护303文件，首次以 `install` 运行未签名服务程序，写控制者记录、注册自动启动服务并启动真实 SYSTEM 守护进程；还会产生服务数据、规则集及日志。**需要首次服务安装/启动的明确确认。** 成功不能仅看exit0：核SCM running、OwnProcess、固定 `"...godusevpn-svc.exe" service`、StartName LocalSystem、持有PID对应实际SYSTEM令牌和已安装hash；复核303文件。 |
| 3 SYSTEM对照 | `& $cli status` | 只读主IPC。只记版本匹配/退出码/未请求连接；SYSTEM成功不能算普通用户C01通过。若状态Wanted=true、已有节点/订阅、未知身份或非预期活动，停批且不自动disconnect/repair。 |
| 4 真实普通令牌负对照 | 使用下一节方案，在未登记的 `<test-account>` medium令牌运行固定 `"<installed-cli>" status` | fresh SYSTEM安装默认只登记SYSTEM；普通用户应被拒绝。若实际已有该控制者，则不能宣称做过负对照；若无登记却成功，先查明而非继续放宽。 |
| 5 登记指定控制者 | `& $svc register-controller '<test-account>' --restart` | 持久追加该账号SID到控制名单并停止/重新启动Ultra服务；授予的是整套主IPC控制权，包含连接/断开/设置能力，**不只是读取status**。需要明确扩控制权限和重启确认。核注册成功、服务重新running、固定SCM/真实SYSTEM/新PID绑定；未知或非零不重试/repair。 |
| 6 真实普通令牌正对照 | 相同账号、相同验证方法，只运行已安装CLI `status` 一次 | 必须实际medium、非SYSTEM、管理员组未enabled；成功退出才说明该哈希CLI在普通令牌下完成了经过C01认证的GetState。失败标记C01待解决，不改服务token/process DACL，不给用户管理员权限，不把unknown改成服务未安装。 |
| 7 官方卸载预演 | `& (Join-Path $installed 'Uninstall-Ultra.ps1') -WhatIf` | 从已安装目录核清单/私有代码/服务身份/无GUI；不运行清理EXE。不传ValidationPackage进入实际卸载。 |
| 8 官方卸载 | `& (Join-Path $installed 'Uninstall-Ultra.ps1')`，由SYSTEM执行 | 执行固定CLI `privacy uninstall-machine`：同一维护门禁内核完整性、机器RestoreAll/应用规则复核，然后固定服务卸载。原服务卸载流程会停止服务、撤Ultra闸、尝试恢复网卡IPv6/DNS/路由并删除SCM对象。**需要该清理/服务及可能网络恢复范围的确认**，即使本批预期0规则；不是一次断网实验。失败留工具/记录，不扩大删除或恢复范围。 |
| 9 独立终态核对 | 固定SCM查询；`& $cli guard status`；`& $cli privacy firewall-list`；已审机器journal只读snapshot | 原/Ultra服务仍不存在；Ultra活动guard过滤器0、应用规则0及unknownFilters=0、canonical machine managed=0、四测试值保持缺失；ProgramFiles303文件与恢复记录保留。上述CLI读取不依赖已删除服务；不要调用cleanup/restore来获得好看的读数。 |

步骤内命令不是一个可直接粘贴的无条件批处理；每一步核对后才进入下一步。命令输出必须在私有受限捕获中处理，只公开退出码、计数、版本匹配和状态。`status` 会含DNS/节点/订阅摘要，register-controller会打印SID名单，官方脚本会打印路径；不能把原始 stdout/stderr 直接写入公开QA记录。当前已审五命令readonly runner不接受install/register/status等新命令，不能绕其allowlist或假称它已授权这些操作。

## C01 无GUI的可行令牌试验设计

此部分还需一个**小型、独立审过且固定hash的令牌启动器**，本次只给设计，不创建或运行新wrapper；它首次运行/使用现有用户令牌前须取得确认。Native CUA不可用，不能宣称真实GUI/UAC已测。

1. 从已经认证的 `<test-account>` SAC cmd 会话取得明确PID。例如只在该cmd中运行系统PowerShell：`"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -Command "(Get-CimInstance Win32_Process -Filter ('ProcessId=' + $PID)).ParentProcessId"`，获得这个固定child的cmd父PID，仅用于私有会话。SYSTEM启动器再打开并保留原进程句柄、创建时间和token，确认实际cmd image、用户SID与批准账号一致；不能只相信PID数字。不读密码、命令历史、进程环境或其他用户token；PID复用/身份无法核对则停。
2. `GetTokenInformation(TokenElevationType/TokenElevation/TokenIntegrityLevel/TokenUser/TokenGroups/TokenStatistics)`：原token已limited则直接复制；若为full，仅取其真实 `TokenLinkedToken` 中的limited counterpart，再用 `DuplicateTokenEx(..., TokenPrimary)`。确认用户SID/登录AuthenticationId来自原用户、非SYSTEM、TokenElevation=0、Medium完整性、BA未enabled（通常deny-only）。没有真实linked/limited token就保留未测；不用SYSTEM的restricted token冒充普通用户，不修改UAC。
3. 固定应用为已安装CLI全路径，参数仅 `status`。保持代码/目录pin，使用 `CreateEnvironmentBlock` 的该用户环境和明确本地working directory；不载入未知profile或改profile/desktop ACL来凑通过。
4. 优先 `CreateProcessAsUserW`，沿用真实token session，`CREATE_SUSPENDED | CREATE_NO_WINDOW | CREATE_UNICODE_ENVIRONMENT`，不继承任意句柄。恢复线程前独立读取child image/token，确认仍是该真实medium用户；不符则终止尚未运行的child。权限/desktop/session条件不足只报未能测，不自动换SYSTEM执行。
5. 为避免跨session标准句柄继承限制，首版只看受控child退出码，不捕获/导出原始status；已审CLI源码只有GetState成功才返回0。按15秒上限等待，超时终止仅本次child、结果unknown且不重试，释放所有句柄。若要捕获错误分类，须另外设计并审核明确限长的跨session结果通道；不临时把原始报告写到用户可读Temp。
6. 每次保存固定事实：realUserMatched、notSystem、medium、notElevated、adminGroupNotEnabled、sourceSessionMatched、CLIHashMatched、statusExitCode、resultUnknown；不保存原SID/账号/LUID/PID/环境。正对照成功仍只验真实普通CLI→SYSTEM主IPC认证，不覆盖Wails、GUI焦点、UAC取消/替代管理员和窗口交互。

微软接口依据：[TokenLinkedToken](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-token_linked_token)、[DuplicateTokenEx](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-duplicatetokenex)、[CreateProcessAsUserW](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessasuserw)、[CreateEnvironmentBlock](https://learn.microsoft.com/en-us/windows/win32/api/userenv/nf-userenv-createenvironmentblock)。CreateProcessAsUser有特权、session和profile前提，跨session不能直接继承句柄；这些失败必须如实留待验，不修改系统访问控制绕过。CreateProcessWithTokenW会在调用者session运行，不能拿它的默认session行为假装原用户GUI会话。

## 终态与停止规则

本批“无WFP”仅指Ultra的活动过滤器/已识别及未知应用规则均0，不能称全系统没有WFP。若还要求provider/sublayer空壳全清，需固定Ultra GUID的只读查询另给证据，不导出全系统netsh WFP原始dump，也不删未知对象。机器journal终态使用已审snapshot纯函数白名单，绝不运行四策略roundtrip main来“检查”。

卸载保留安装文件、服务数据、controller登记和空journal/lock，故不能称回到字节级原始机器。没有活动服务后名单不提供现役管道，但保留名单会影响将来安装；本批不自动删除它。任何失败/unknown保留工具与记录，停止后续非必要动作；允许的收尾只有已明确授权的官方卸载或人工审查，不自动重装/清闸/还原其他用户配置。没有新增费用资源、断网测试、重启、订阅请求或真实GUI的默认授权。
