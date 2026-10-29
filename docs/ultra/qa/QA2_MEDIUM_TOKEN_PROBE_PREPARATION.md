# qa.2 真实 medium 令牌能力 probe 准备

日期：2026-10-19。脚本 `Vps-Qa2MediumTokenProbe.ps1` 仅是准备，**没有运行 main、读取真实 token/CIM/服务或调用 native Probe**；首次实际执行需要 ROOT 独立复核及用户确认。必须在独立系统 PowerShell 5.1 x64 进程中运行，不能 dot-source 到长期管理会话。

参数仅一个保守本机用户名，不接受密码、域路径、SID、PID、EXE、命令或目录。真实执行将解析本机账号SID，在至多64个已存在cmd中仅接受唯一的 System32/cmd.exe、同账号候选，再持有真实process/token句柄重新确认实际image和用户。已有medium primary token可用；full token只取其真实linked limited counterpart。必须非SYSTEM、Medium IL、非提升且BA未enabled，再DuplicateTokenEx成SecurityImpersonation token。没有合格token则拒绝；不创建用户、不从SYSTEM构造restricted token、不改UAC/privilege/DACL。

目标固定为ProgramFiles/GodUseVPN Ultra的qa.2 CLI和service SHA。父目录/叶文件root-first no-follow并保留有效读/DELETE共享限制；安装树及OS code根/叶允许普通read/execute，但拒绝非SY/BA/TrustedInstaller写权限。SCM必须单实例running、Own Process、canonical LocalSystem、精确固定quoted image+`service`；持有该PID的process object，并作为SYSTEM独立读取实际image及真正SYSTEM/elevated token。SCM配置不代替这个目标runtime SID观察。

能力探针全部封装在一个Reflection.Emit生成的同步CLR方法：ImpersonateLoggedOnUser → OpenProcess(QUERY_LIMITED_INFORMATION|SYNCHRONIZE) → OpenProcessToken(TOKEN_QUERY)。同方法finally先RevertToSelf并记录明确返回，再关闭自身已取得的probe句柄；不在impersonation中执行PowerShell pipeline。main仅在确认Revert成功及thread无token后才重新核SCM/PID/image/alive；任何不确定仅关闭既有句柄、输出固定状态后退出，无权限操作/retry。

输出仅固定模式、布尔、Win32错误号和错误code，不包含实际账号/SID/PID/路径/token/原始status。`mode=token_query_probe`，`productExecuted=false`、`statusCommandExecuted=false`、`cliAuthenticationPassed=false` 始终保留。这里并未运行CLI status/MGetState，也不获取其输出或开启GUI/服务。

已在实际系统PS5.1运行47项AST/纯facts/native metadata/IL测试：账号与canonical服务拒绝矩阵、medium/system/admin事实、固定bool+错误号转换、代码SDDL、P/Invoke SetLastError与句柄宽度，以及Probe有catch/finally并经JIT PrepareMethod验证。PrepareMethod只编译控制流，不调用Probe或native方法。没有文件夹创建、token读取、注册表/ACL或服务操作。

本probe用于测旧qa.2对SYSTEM token query的实际兼容前提。C01新修订installed分支已不再依赖TOKEN_QUERY；因此即使probe得到AccessDenied，也不能称新版本失败；即使得到成功，也不能称CLI/GUI认证已通过。新候选仍必须通过真实medium进程的pipe/SCM/PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE和完整MGetState验收。管理员/内核/主动DOS命名空间突变仍在可信环境前提之外，输出错误不自动触发修复/授权扩张。
