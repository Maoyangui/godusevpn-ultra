# A07 维护窗口及 A06/A15/A17/A21：独立二审

2026-10-19。只读审阅 Ultra 副本；未修改产品源、原项目、实际服务、注册表、时区、WFP 或云测试机。实现作者仍在补测试/文档时先核对调用边界；最终冻结复验结果见末尾。

结论：作者完成下述 pin 修正并冻结后，**在文中限定的官方维护/卸载范围内未发现新增已确认阻塞**。A06/A15/A17/A21 的本批源码与 fake 结果符合所述目标；实际多进程竞争、UAC/替代管理员、SCM 卸载和系统设置效果仍待受控 VM，不据此称实机验收完成。

## 已确认并反馈的问题

初版 `internal/cli/uninstall_windows.go` 目录 pin 仅申请 `READ_CONTROL | FILE_READ_ATTRIBUTES`，没有 `FILE_LIST_DIRECTORY`。不共享 DELETE 本身不能证明该访问组合实际拒绝数据共享冲突。

独立原生验证仅在新建的 `ultra-maintenance-review-<随机值>` TEMP 空目录中打开句柄，不实际删除、改名、设置 reparse 或访问安装位置：

```text
ACCESS=20080 DELETE_OPEN=True WIN32=0
ACCESS=20081 DELETE_OPEN=False WIN32=32
TEMP_CLEAN=True
```

`0x20080` 为初版 READ_CONTROL/READ_ATTRIBUTES；`0x20081` 增加 LIST_DIRECTORY。结果证明原 pin 的“保持目录对象不被删除/替换”断言不成立。问题已反馈作者最小补权与 TEMP 回归；同步根修复 C01 `auth_code_windows.go` 和 C02 `paths/secure_windows.go` 的同类断言。

冻结版本已修：A07 `openPinnedCode` 对目录增加 LIST_DIRECTORY；C01 前台代码 pin 同样增加；C02 nativeStore 统一加 FILE_READ_DATA（目录中同一位为 LIST_DIRECTORY），核心普通文件仍仅短暂检查后关闭。二审独立重跑三处生产机制相关 TEMP 回归全部通过：持有时 DELETE 打开拒绝 sharing violation，释放后可打开；服务子文件创建/原子改名仍可用。文件未被实际删除，未操作真实安装路径。

这不等同于任意普通用户可替换安装代码：既有 role-owned、禁止其他主体写入/替换的 ACL 仍是实质前提。前台服务路径另有明确管理员 SID 的信任范围。此次只确认句柄 pin 实效，不把它泛化成已经复现的未授权提权或其他管理员攻击。

## 维护门禁核对

- 命名 mutex 使用固定产品独立 Global 名称；只接受 SYSTEM/Administrators owner、受保护 DACL 和受限的 Interactive 同步权限。非管理员不能将不明对象自动接管；门禁不是机器设置授权，后端仍验证实际提升令牌与固定操作。
- `Acquire` 先 `runtime.LockOSThread`，成功后保留到 `Lease.Close`；零等待的 `WaitForSingleObject` 不让机器维护变成 VPN 启动等待条件。递归线程获取明确拒绝，内部操作必须传已有 lease；借用检查核对当前 OS 线程及 active 状态。abandoned 返回独立错误，不在同一次请求继续写入。
- HKLM Apply/Restore/RestoreAll、写失败回滚，WFP Apply/Remove，以及共享系统时区 Apply/Restore 均在门禁范围。机器管理器借用同一 lease，HKCU 单项不要求机器 gate；RestoreAll 只在 journal 含机器控制或本身为 machine manager 时需要 gate。实际 registry/WFP/time backend 不对包外暴露绕过方法。
- 维护专用助手完成原有管道/父进程认证后只 Prepare 同步对象，`maintenanceOnly` 分支不调用 `machineDirectory`，不创建 machine manager/WFP backend。`MaintenanceReady` 是固定的 synchronization_only 答复，不是管理员代理任意操作。CLI 继续在原账户的 time manager 执行恢复，不让替代管理员读取原用户 journal。
- 门禁不存在/忙碌/身份异常只返回本次机器修改失败；没有调用 VPN connect/disconnect、guard 或网络恢复。真实策略/过滤器/时区效果仍需独立 VM 验收，不能从门禁存在推断保护生效。

## 清理/卸载窗口核对

官方路径为 `Uninstall-Ultra.ps1` → 固定安装 CLI `privacy uninstall-machine`。单个 CLI 父进程持同一 lease，覆盖安装代码预检、机器策略恢复/WFP 清理、最后完整性检查、固定服务子命令及卸载后 SCM 复核。清理未知过滤器、仍有已管理策略、备份异常或任一读错均保留错误，不继续服务卸载；文件/数据/恢复工具保留。

只允许 ProgramFiles KnownFolder 下固定 `GodUseVPN Ultra/godusevpn-cli.exe` 发起此卸载；代码树 owner/DACL、链接、清单全覆盖、哈希及产品命名空间受核对。两次 verify 围住 cleanup，并在卸载前再次检查 lease。固定服务配置需要匹配产品服务名、固定 EXE、唯一 `service` 参数、own-process 类型和 LocalSystem；未知 SCM 结果不按服务不存在处理。

以下保证需要准确限定，不能由本次代码外推：

- SCM 检查核对的是服务配置，不是当前运行 PID 的完整映像/令牌认证；不将它写成 IPC 身份验证等级的证明。
- 此门禁协调本批产品机器隐私写入口和官方卸载桥，不拦截外部管理员的 SCM/注册表工具。旧 `godusevpn-svc.exe uninstall` 仍是单独的管理员服务/网络恢复命令，不经过新 privacy cleanup 窗口；它不删除保留的程序、journal 和恢复工具。不能称全部历史服务管理命令都已合并为机器隐私卸载。
- 角色 ACL 和代码 pin 不是针对恶意管理员的系统命名空间或所有内存篡改防线；mutex 也不是鉴别发布者/代码签名的替代品。

## 相邻问题

- A06：单条 Remove 提交后重新枚举。任何无法识别的本产品对象都会返回 `unknown_remnants`、`Verified=false` 和错误，不再把已解析 ID 不存在等同完整移除；未知对象保留，不扩大删除到其他模块。
- A15：CLI detection 明确 `executionMode=direct_cli`，观察者权限来自 Facts；`helperAuthorized` 只对明确 desktop_ui + 实际助手成功成立。直接提升 CLI 不因此冒充助手或普通应用权限。原当前进程用户 journal scope 仍保留。
- A17：UnknownFilters 独立数量进入专门 finding 的 `count`，状态 unknown，不通过警告文字推断，不据计数宣称全系统枚举完整。
- A21：新增独立 timezone-report/restore，标记 current_process_user。路径来自当前令牌 RoamingAppData 并拒绝与 APPDATA 覆盖不一致；标准服务账号拒绝。普通 CLI 只借助固定门禁 bootstrap，原账户执行恢复并保留冲突/失败；机器清理没有冒充时区恢复。服务账号/替代管理员及真实 UAC 仍需受控 VM 验收。

## 最终冻结复验

作者通知冻结后，二审实际执行：

```text
go test ./internal/maintenance ./internal/privacyhost ./internal/timeprivacy ./internal/appfirewall ./internal/cli ./internal/detection -count=1 -v
go test ./internal/privacy -run TestMachinePolicyGateBusyStopsBeforeStoreOrRegistryMutation -count=1 -v
go test ./internal/ipc ./internal/paths -count=1 -v
go vet ./internal/maintenance ./internal/privacyhost ./internal/privacy ./internal/timeprivacy ./internal/appfirewall ./internal/cli ./internal/detection ./internal/ipc ./internal/paths
```

全部通过。作者最后只收紧正常/直接入口对 MaintenanceReady 的提前拒绝并校正摘要范围后，另独立重跑 privacyhost/detection 全包通过。diff-check 退出 0，仅已有 LF/CRLF 提示。

证明层次分别为：

- fake：lease 线程编号/关闭复用；mutex descriptor owner/DACL；机器/WFP/time Busy 不进入写入；维护专用模式不碰 manager；直接 MaintenanceReady 不调用机器工厂；完整窗口在初次/最终核对或 cleanup/lease 失败后不执行卸载；未知 WFP 对象和回读失败保留未确认结果；当前账户时区闭包在 bootstrap 取消/失联后不调用恢复；身份与未知数量独立。
- native TEMP：A07 `openPinnedCode`、C01 代码 pin、C02 服务目录 pin 的 DELETE sharing 与释放回归，以及新权利下普通创建/原子替换仍兼容；没有实际删除、重解析或 SCM 操作。paths 其他既有 TEMP 私有 DACL 测试也通过。
- 额外临时 Go overlay：在真实 Windows `runtime.LockOSThread` / `GetCurrentThreadId` 上构造**假 finish** lease，另一 goroutine 无法借用，原线程仍可核对和关闭。没有调用产品固定 Global mutex 的 Acquire/Prepare/Ready，没有创建服务或改变机器配置。overlay 文件在新 TEMP 目录中生成并逐叶清理。
- 静态：真正 mutex Acquire 持有 OS 线程至 ReleaseMutex/CloseHandle 后才 Unlock；StartMaintenance 的 machineDirectory 分支排除；所有已列机器写入口的调用与原 VPN 生命周期隔离；实际卸载桥不在本轮执行。

本轮只选跑 privacy 的门禁 fake，未为二审重复执行该包完整机器 store/native gate 用例。真实内核 mutex 多进程竞争/abandoned、原用户与替代管理员 UAC、固定服务卸载以及效果验收仍须 VM；本文不把合成线程检查写成这些实机结果。
