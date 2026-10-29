# 最小权限隐私 helper

普通桌面界面及其 WebView 不需要提升权限。Windows UAC 只启动当前产品 EXE 的第二个无界面进程，且该进程在 `main` 最早进入 `Handle`，不初始化 Wails、不打开浏览器、不接入 VPN 生命周期。

## 集成接口

```go
if handled, code := privacyhost.Handle(os.Args[1:]); handled {
    os.Exit(code)
}
```

这段必须位于主函数中任何普通参数处理、服务操作或 Wails 启动之前。

普通 UI 使用长生命周期 context，在工作协程中调用 `Start(ctx, currentExe)`；普通 CLI 可指定同一规范目录内固定名称的 `godusevpn.exe` GUI sibling。身份配对仅允许同一个 EXE，或同一目录的 `godusevpn.exe` 与 `godusevpn-cli.exe`，双端都核对；其他路径或文件名不接受。Windows UAC 由操作系统处理，取消只返回 helper 错误，不阻止普通界面、原服务和隧道使用。退出 UI 时取消 context 或 `Client.Close()`。不要用单次请求的短 context 创建整个 helper 会话。

`Client.Call(ctx, method, params, out)` 参数协议：

| method | params | scope |
| --- | --- | --- |
| Report | nil | 仅四项机器策略 |
| Apply / Restore | 固定字符串 ID | 仅机器策略白名单 |
| RestoreAll | nil | 逐项恢复机器策略，不调用通用 RestoreAll |
| FirewallReport | nil | 独立应用 WFP 规则 |
| FirewallApply | appfirewall.Rule | 固定 EXE 身份目标 |
| FirewallRemove | 字符串 ID | 仅本模块自有规则 |

明确已提权的 CLI / 卸载器可调用 `ExecuteElevated(method, params, out)`，方法和参数白名单完全相同。它实际检查当前进程令牌，未提升立即拒绝，不自行 runas；存储路径仍由 Known Folder API 固定计算。普通权限 CLI 应使用经 GUI EXE 启动的管道 helper。该直接调用也不提供任意路径、URL、进程或脚本执行能力。

`IsElevated()` 只读检查当前实际进程令牌，不获取权限。GUI/CLI 目录配对验证是路径与进程身份约束，不代替发行签名验证；便携开发目录若由用户写入，UAC 确认的就是当时该目录中的 EXE 代码。不要把相邻文件名当成防篡改证明，应从可信发行包运行，正式部署采用管理员保护的安装目录。

机器策略 ID 为 `windows-advertising-id`、`windows-online-speech`（实际对应 `AllowInputPersonalization`，只限制输入个性化，不等于关闭在线语音识别）、`windows-clipboard-history`、`windows-clipboard-cloud`。用户浏览器设置使用普通 UI 的 `privacy.New`，不能通过 helper。helper 使用 `privacy.NewMachine`，在分派、catalog 与 backend 层均不读取或写入 HKCU。UAC 换管理员账号不改变这一边界，也不把该管理员的用户设置冒充登录用户的设置。

`RestoreAll` 首先检查 `BackupHealthy`；备份损坏不能被解释成“没有已管理项”，此时拒绝宣称恢复完成，并要求保留恢复工具。卸载流程还应在清理后复读机器报告与 WFP 报告，任何残留或未知状态都不能当作清理完成。

`Report.admin=true` 代表 helper 的令牌。合并报告时应保留“普通界面权限”和“helper 已授权”两个状态，不能声称 UI/WebView 获得管理员权限。

## 通信与生命周期

- 普通 UI 先创建 256 位随机命名的本地管道；DACL 只给发起用户 SID、管理员组和 SYSTEM。
- `ShellExecuteExW` 返回真实子进程句柄/PID；UI 用内核管道 client PID 验证它。helper 核对实际 server PID、父进程句柄、父进程 token SID，以及相同 EXE 或固定同目录 GUI/CLI 配对，再握手。
- 每次请求检查随机会话、协议版本、递增 ID 和父进程仍存活。严格 JSON、固定方法、帧大小上限；没有 TCP/HTTP、任意路径、命令、URL 或脚本入口。
- go-winio 默认以匿名模拟级别连接，低权限 UI 不能通过管道模拟高权限 helper。
- UI 每 15 秒发只读 Report 心跳；60 秒没有请求、管道关闭或父进程退出时 helper 结束。helper 退出不撤销持久 WFP 规则或恢复系统策略。
- 请求传输/响应超时后关闭该会话，明确“操作结果未知，重新检查后再试”；不会自动重试写操作。父进程退出时已开始的系统事务可能完成，下一次必须读取实际状态。

## 数据边界

数据路径只来自 Windows Known Folder API 的 ProgramData，固定为 `godusevpn-ultra/privacy-machine`；RPC 和环境变量均不能选择它。拒绝重解析点，拒绝由普通用户拥有或可写的产品父目录；不会静默接管用户预植的目录。新目录使用管理员/SYSTEM 私有 ACL。仅收紧本 helper 的 `privacy-machine`，不改日志、VPN 配置、原 guard 或 BFE。

机器备份不绑定临时 UAC 管理员 SID，后续授权管理员可以恢复。普通 UI 无需读取私有备份文件，只通过已认证的管道获取有限报告。

## 验证与剩余验收

`go test ./internal/privacyhost ./internal/appfirewall -count=1`、`go vet` 使用内存 net.Pipe、fake manager、SDDL 描述符和 ABI 检查；不启动真实 helper、不提权、不创建活管道、不写注册表/防火墙。Linux stub 和 Windows arm64 包编译通过。

Windows UAC 同账号/换账号、安装父目录权限、命名管道、父进程退出与系统事务等实际组合仍须在隔离 VM 验收。拒绝授权或身份不能验证时，只保留手动引导和独立错误状态，不弱化现有保护。
