# qa.3 安装预览 runner：准备与缺口

2026-10-19。新脚本 [Vps-Qa3Install.ps1](Vps-Qa3Install.ps1) 的 SHA256：`1e625fae96e677772e07ae74e8f3f851c0bb30985bebdc2bd9425846f83e9a37`。

**第一版只实现预检和官方 `Install-Ultra.ps1 -WhatIf`，没有实际 Install 模式。** 参数仅 `CandidateStageID`，必须是 lowercase UUID-v4 的 32 位无分隔符形式；路径固定为 KnownFolder ProgramData 下 QA 根的该 stage / `package-beta-qa3`。不接受源目录、EXE、命令或注册表路径参数。不调用 UAC、产品 EXE、安装服务、修改 ACL、创建或删除目录；失败不重试、回滚或接管。

## 已实现的入口约束

- Windows PowerShell 5.1 x64，当前**有效**身份为 SYSTEM。字段 `effectiveSystemCaller` 只表示这个检查，不独立证明 primary token 或 session 0；执行环境的来源仍须另行确认。
- 包、所有实际消费的子目录和 352 个普通文件使用 root-first/no-follow、私有 SY/BA owner/DACL 检查及只读句柄保留；拒绝未知文件、未知目录、重解析和 hash 不符。完整 SHA256SUMS 必须为固定 `bd0bcada…`，351 个 payload 加清单共 352 个文件；三 EXE 固定 hash/长度和静态版本、描述符均为 qa.3。
- 本机原版 `godusevpn` 与 Ultra SCM 对象必须明确不存在。通过固定服务名的 OpenService 返回 **1060** 识别缺席；权限拒绝或其他错误是未知，不能当作服务不存在。
- 两个 ProgramFiles KnownFolder 中的原版/Ultra 默认安装目录均须明确不存在；三个固定产品进程名均须为零，不执行 kill。
- 只检查 `GODUSEVPN_ULTRA_DATA` 这个固定环境键的 Process/Machine 作用域，存在非空覆盖即拒绝；不枚举、保存或输出环境。Ultra 原本忽略原版 `GODUSEVPN_DATA`，本脚本不改动它。
- 固定机器数据根只能缺席，或仅有私有 `privacy-machine`，其中精确两个普通文件：canonical machine-policy-store-v1 的 controls 空 journal 和 0 B lock；具名用户 ACE、额外临时文件、controller/settings/profiles/core 状态或其他目录均拒绝。只读取固定空 journal/lock，不加载订阅或旧核心配置。具体“来自本批”的出处应结合此前独立后验记录，本脚本不凭空推断创建历史。
- 固定四项 HKLM64 策略值必须缺席：广告标识、在线语音、剪贴板历史、跨设备剪贴板；只检查值是否存在，不输出内容。

## WhatIf 与结果边界

只有完整预检通过，才会用固定系统 PowerShell 启动包内已由清单认证的 `Install-Ultra.ps1`，参数固定为该包路径和 `-WhatIf`。捕获上限 64 KiB、60 秒，不公开 raw stdout/stderr。零退出仍须具有固定版本/351 payload 摘要、预期 target 行和 ShouldProcess 动作标记；兼容动作描述的控制台换行。出现实际安装完成标记、错误、超时或未知输出均停止，不自动重试。

结束后重复独立预检：服务、进程、ProgramFiles、环境覆盖、四策略值和完整包必须仍符合入口条件，空机器 journal/lock 的内容指纹必须相同。结果里的 `unchanged` 只表示这些检查和内容指纹符合前后基线，**不是整个操作系统或全部 ACL 元数据完全未变的证明**。异常保留现有路径和记录；仅可能终止本次创建且未确认结束的 WhatIf 子进程。

没有 WFP、网卡、IPv6 或持久 guard 的无残留检查；“服务/目录缺席”不能替代这些前置网络保护证据。也不声称抵御恶意管理员、内核或主动 DOS 映射竞态。Windows 本身仍可能记录执行、审计、防护和 WMI 依赖活动。

## 已做与未做的验证

[Vps-Qa3Install.Tests.ps1](Vps-Qa3Install.Tests.ps1) 在本地 PowerShell 5.1 完成 **76 项** AST/元数据/内存/fake 检查：十个复用的 Assemble 只读函数与冻结源逐字一致；唯一 stage 参数；没有创建服务/目录等 native 写入口；四固定策略；canonical empty journal/私有 ACL；WhatIf 摘要及换行、错误版本/count/target/实际安装标记拒绝；11 种预检、超时、未知和后验变化流程均无重试、无 raw 字段输出。

**没有运行脚本主体、native 文件/SCM/注册表 API、官方 WhatIf、产品、安装或远程操作。** 这也不是实际 WhatIf 的系统兼容性验收。

实际 Install 与独立安装后验收尚未实现：显式二阶段授权入口、实际 primary SYSTEM 上下文核对、安装后 352 项磁盘 hash、受保护且 BU 只读的代码树、SCM OwnProcess/canonical LocalSystem/固定路径且 Running、持有实际进程句柄并查询 SYSTEM 身份等，须作为后续单独可审查工作。当前 `actualInstallSupported=false`，不得将预览 completed 当成安装通过。

兼容性补充：冻结 v1 在真实机 preview 中曾以 `SCM_unknown` fail-closed，未启动官方 WhatIf，不能视作预检通过。v1 保留不复用；IntPtr NULL-safe 的 `Vps-Qa3InstallV2.ps1` 与 `Vps-Qa3ScmReadbackV2.ps1` 另行完成本地契约检查，真实机 readback 仍待执行。
