# 机器隐私维护与独立恢复修复记录

2026-10-19。仅 Ultra 副本；不修改原项目。本文说明 A06、A07、A10、A15、A16、A17、A18、A21 的本批实现与有限验证，不代替 Windows 实机验收。

## 结果与入口

| 审计项 | 本批结果 | 实现证据 |
| --- | --- | --- |
| A06 单条撤销误报完成 | WFP 提交后存在任何无法识别的本产品对象，返回错误、`unknown_remnants`、`verified=false`；对象保留 | `internal/appfirewall/manager.go`、`maintenance_test.go` |
| A07 清理与卸载竞态 | 固定全局 maintenance mutex；同一 lease 覆盖机器清理、最终复核及官方卸载桥，不再只依赖最后快照 | `internal/maintenance/`、`internal/cli/machine_cleanup.go`、`uninstall_windows.go` |
| A10 检测缺少 DNS/日志/凭据 | 独立三个 finding；只接收安全的配置类别、级别及天数，实际解析/清理效果仍 unknown | `internal/detection/storage_evidence.go` |
| A15 CLI 冒充助手 | `executionMode=direct_cli`；直接提升 CLI 不据此设置 helperAuthorized | `internal/cli/privacy.go`、`internal/detection/report.go` |
| A16 guard 文案过宽 | 只指定 `permitSelf` 的 `CLEAR_ACTION_RIGHT` 硬放行限制；不把其他 soft permit 一并判定 | `internal/appfirewall/manager.go` |
| A17 未知过滤器数量丢失 | 独立 `network-firewall-unknown` finding，正整数 `count` 来自后端字段，不解析警告文字 | `internal/detection/report.go`、`evidence_test.go` |
| A18 文件选择状态推断 | 改为“本报告未包含文件检查，工具检查单独查看”，不推断用户是否选择过文件 | `internal/detection/report.go` |
| A21 无界面时区恢复 | 新增当前进程用户的时区 report/restore；机器 journal 损坏不要求先加载它 | `internal/cli/timezone*.go`、`internal/privacyhost/` |

新增 CLI 用法（参数不接受自定义路径、账户或命令）：

```text
godusevpn-cli.exe privacy timezone-report
godusevpn-cli.exe privacy timezone-restore
godusevpn-cli.exe privacy cleanup-machine
godusevpn-cli.exe privacy uninstall-machine
```

`timezone-report` 为只读；`timezone-restore` 应在保存备份的原用户终端运行。后两条需显式管理员终端。`uninstall-machine` 仅接受固定 ProgramFiles 安装版 CLI；便携版本只提供独立机器清理，不把任意目录当作安装版卸载。

## 维护门禁的边界

固定命名对象为 `Global\GodUseVPN-Ultra-Privacy-Maintenance-v1`。新对象 owner 为 Administrators，SYSTEM/Administrators 可完整访问，Interactive 仅可读取对象权限、等待及释放 mutex。现有对象必须满足受信 owner、受保护 DACL 与固定允许范围；普通用户不创建缺失对象，不接管不明对象。

门禁用于串行化，并不授予机器策略写入权限。每个后端仍检查实际提升令牌和固定控制范围。本机交互用户可以占用该同步对象，造成可选隐私修改返回 Busy；它不是抗本机拒绝服务机制。VPN 的初始化、连接、重连、DNS、路由和原 guard 均不依赖此门禁。

获取采用零等待，先 `LockOSThread`，成功持有至 `Lease.Close` 才解锁线程。显式 lease 只允许当前持有线程使用，关闭后不可复用；同线程递归 Acquire 被拒绝，内部操作传递已有 lease，避免 Windows mutex 的递归特性掩盖错误。Busy 不自动重试。发现 abandoned 时本次拒绝修改并要求重新核对 journal/规则；这不是持续隔离标记，下一次获取可能正常，不能描述成“故障后永久锁定”。

覆盖的写入口为：机器 HKLM 策略 Apply/Restore/RestoreAll 及其回滚、独立 WFP Apply/Remove、共享系统时区 Apply/Restore，以及官方机器清理/卸载维护窗口。HKCU 单项仍在原普通用户执行，不因机器 gate 缺失而自动提升或改用替代管理员账户。

## 官方卸载的一次完整窗口

`Uninstall-Ultra.ps1` 只调用固定安装 CLI 的 `privacy uninstall-machine`。该 CLI 持有同一 lease，依次：

1. 确认实际提升令牌和固定 ProgramFiles KnownFolder 安装位置。
2. 固定本地磁盘、目录祖先、安装代码树 owner/DACL、非 reparse、普通文件单链接、清单路径/数量/必需恢复文件/产品命名空间以及全部 SHA256。
3. 核对固定 `godusevpn-ultra` SCM 配置：固定 service EXE、唯一 `service` 参数、own-process、LocalSystem；已安装 GUI/助手仍运行则拒绝，不杀进程。
4. 恢复机器策略，逐项撤销已知 WFP 规则，再读 WFP 和机器 journal；任一未知对象、残留、损坏、冲突或读取失败都停止卸载。
5. 再检查 lease、完整代码哈希、SCM 配置及 GUI 状态，再运行固定 service EXE 的唯一 `uninstall` 参数，复核服务确定不存在。

文件和目录句柄贯穿上述窗口。代码文件句柄不共享写入/删除；目录句柄申请 `FILE_LIST_DIRECTORY` 且不共享删除。父目录及子目录可信 ACL 另外限制创建、写入和替换。不能只凭属性句柄宣称目录已被 pin：本次独立二审发现此问题后，已增加 LIST_DIRECTORY，并在 TEMP 空目录验证 DELETE 访问持有期间返回 sharing violation、释放后可打开。测试没有实际删除目录，也没有修改 ACL。

任何失败均保留程序、数据、日志、journal 和恢复工具，不递归删除、接管或自动续装。安装/卸载脚本失败提示指向 [中文人工恢复说明](../INSTALL_RECOVERY.zh-CN.md) / [English recovery guide](../INSTALL_RECOVERY.en.md)。每个用户仍须独立恢复自己的浏览器/HKCU 和时区备份。

此保证只覆盖产品机器隐私写入口和官方 `Uninstall-Ultra.ps1` → CLI 路径。管理员直接使用外部 SCM/注册表工具，或旧 `godusevpn-svc.exe uninstall` 服务/网络恢复命令，不会因此自动进入 privacy cleanup 窗口；旧命令也不删除保留的 journal/恢复工具。SCM 检查针对配置，不能称为运行 PID/令牌认证。哈希证明清单自洽，不代表发布者签名；角色 ACL 不是针对恶意管理员的完整防线。

## 时区独立恢复与最小 bootstrap

`timezone-report` / `timezone-restore` 定位当前令牌的 RoamingAppData，输出 `backupScope=current_process_user`。SYSTEM/LocalService/NetworkService 被拒绝；APPDATA 与当前令牌 KnownFolder 不一致也拒绝，不猜测或访问另一账户目录。恢复沿用已有 timeprivacy journal 及冲突判断，不增加强制覆盖模式、不修改 UTC 时钟，不声称浏览器时区也已恢复。

普通用户 CLI 若找不到受信 maintenance 对象，才请求固定同目录 GUI 的 maintenance-only headless helper；整个 CLI 不提升。新模式复用原管道 ACL、随机会话、父进程/来源配对及握手认证，只允许固定空参数 `MaintenanceReady`，返回 `scope=synchronization_only`。该模式只建立并保持同步对象，不调用 machineDirectory、不初始化机器 policy/WFP backend、不接收任意路径，正常模式和直接提升执行入口也不接受此 bootstrap 方法。原 CLI 在自己的账户下读取/恢复时区 journal。

取消 UAC、连接失败、超时均不自动重试或转向机器 store；连接/父进程结束及 idle timeout 使助手退出。时区 report 的 `canApply=false` 会明确提示尚未建立门禁，不把拿到系统选项视为可写。时区恢复仍可能被实际 Windows 用户权限拒绝，届时保留备份和独立错误。真实替代管理员 UAC 与跨会话行为尚未实机验证。

## 检测证据 schema

- `executionMode` 只接受 `desktop_ui` / `direct_cli`，其余 unknown。观察者 `admin` 与助手 `helperAuthorized` 独立；只有明确 desktop_ui 且助手已成功的输入才标记助手授权。
- 独立未知过滤器 finding 的 `count` 来源于 `UnknownFilters`。即使没有可解析规则或警告文字，数量仍显示；没有数量则不从文本猜测。
- Network 安全摘要仅有 `SettingsAvailable`、`LocalDNSMode=system|doh`、`RemoteDNSMode=doh`、`LogLevel=debug|info|warn|error`、`LogDays`。模型再检查枚举和范围；原始 DNS 地址、profile、订阅、密码、settings 全体均不进入 report。
- DNS finding 只说明配置意图；构建器严格模式可能替换 system，未发查询/抓包，不能证明所有应用无泄漏。日志 finding 不读取内容，也不证明旧记录已经清除或全部日志范围受控。
- 凭据 finding 明确本客户端尚未实现 DPAPI 凭据加密。文件权限不等于加密或管理员隔离，本次不读取秘密，不把服务根信任校验外推为所有应用 ACL 审计。
- 来源时间由 GUI 的各实际只读读取完成时刻提供；缺来源时为空。报告生成时间不是独立采集时间，凭据能力说明与文件工具提示没有伪造的实测时间。

## 已运行的无真实策略写入验证

```text
go test ./internal/maintenance ./internal/appfirewall ./internal/privacyhost ./internal/privacy ./internal/timeprivacy ./internal/cli ./internal/detection -count=1
go vet ./internal/maintenance ./internal/appfirewall ./internal/privacyhost ./internal/privacy ./internal/timeprivacy ./internal/cli ./internal/detection
PowerShell Parser: Install-Ultra.ps1 / Uninstall-Ultra.ps1
```

全部通过。新增回归覆盖：lease 所属线程/关闭复用；synthetic SDDL 的 owner/超额 Interactive/普通主体权限拒绝；Busy 时机器策略/WFP/时区无写入；WFP 未知残留及提交后回读失败不误报完成；bootstrap-only 白名单/无 manager 访问/固定参数；清理失败不卸载、两次完整性检查与 lease 丢失；manifest 缺项/多项/遍历/重复/hash错/产品错；时区原用户闭包、取消和连接失败不进入恢复；直接 CLI 身份、数量独立字段、DNS/日志有配置仍 unknown、非法摘要不导出。

WFP/registry/时区写采用 fake。已有包的 journal 回归会使用临时目录及本地权限检查；新增 pin 回归只打开 TEMP 空目录句柄，没有真实服务、UAC、网络、WFP、注册表或时区修改。未运行实际卸载、安装脚本或浏览器用户数据采集。静态 Parser 不证明脚本实机成功。

## 仍须受控 Windows VM 验收

| 场景 | 验收目标 | 当前结论 |
| --- | --- | --- |
| 两个产品副本/多个 helper/CLI 同时写机器策略、WFP、时区 | 同一对象串行；冲突写返回 Busy，VPN 正常 | fake/源码通过；实机待测 |
| 进程持锁意外退出 | 一次 abandoned 明确失败，journal/规则复查后可恢复 | 代码边界已审；真实进程终止待测 |
| 普通用户、同账户提升、替代管理员提升、UAC取消 | 原用户时区 journal不切账户；bootstrap不触机器store | 协议/fake通过；真实UAC待测 |
| 机器 journal 损坏、GUI不可用 | timezone独立report/restore不被其阻断，冲突仍保留 | 调用边界/fake通过；实机待测 |
| 安装代码/SCM配置异常、未知WFP残留、仍有managed策略 | 统一卸载拒绝，文件与工具保留 | fake/静态通过；实机待测 |
| 原 guard 开启时应用WFP规则及代理/回环路径 | 不把installed当阻断，permitSelf冲突如实反馈 | 未进行流量测试 |

独立二审见 [MAINTENANCE_SECOND_REVIEW.md](MAINTENANCE_SECOND_REVIEW.md)。完整 VM 步骤仍以 [VPS_TEST_RUNBOOK.zh-CN.md](../VPS_TEST_RUNBOOK.zh-CN.md) 为准，不因这些本地验证追加自动网络或系统操作。
