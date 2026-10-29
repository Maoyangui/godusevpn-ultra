# C02 第二轮独立复核

2026-10-19；仅审查 Ultra 副本源码和既有说明。本轮未运行产品、服务、账户操作、真实 ACL 修改、注册表策略或网络操作；没有修改应用代码。

## 结论

发现一项需在关闭 C02 前处理的源码阻塞：消费目录中的危险可继承 ACE 不能一概忽略。其余本轮指定的初始化顺序、可选隐私目录隔离及失败撤闸路径未发现新的具体阻塞。该结论仅限下列源码和 fake 测试，不能代替真实 Windows 句柄/继承/服务身份验收。

## P1：仅继承的危险权限可进入后续新建核心文件

- `internal/paths/secure_windows.go:280` 对所有 `INHERIT_ONLY_ACE` 直接跳过。对于目录，这种 ACE 虽不授权当前目录，却可能授权以后创建的子项。
- 同文件 `:253` 只收紧根、logs、diag；既有 profiles、rulesets 及其递归子目录只是验证并持有句柄，不移除其中显式的危险继承 ACE。
- 例如一个 Administrators owner、当前仅 SY/BA 可写的既有 profiles 目录还带显式 `(A;OIIO;GW;;;BU)`，可通过当前检查。服务随后在 `internal/profile/profile.go:260` 使用普通 `os.WriteFile` 创建 `.tmp` 并改名，子文件可能获得该目录授予 Users 的继承写权限；规则写入 `internal/ruleset/ruleset.go:274` 存在同类路径。父目录重新设置 DACL 不负责移除子目录自己的显式 ACE。
- 因此“当前目录普通用户不可写”不足以支撑整个服务生命周期中新建核心输入的可信度；它们可在服务运行后变成普通用户可写。此处确认的是源码接受条件和创建路径，未执行真实 Windows ACL 利用。
- 建议在核心消费目录验证时同时审查可继承的危险允许 ACE，拒绝对非受信主体授权子项写入/替换的 OI/CI 组合；Creator Owner 常见继承模板需要单独按实际创建身份判断，不能据此忽略所有主体。应补 fake 描述符/目录用例，后续另行授权验证真实继承。不要为兼容旧目录静默修权并接管其历史内容。

已将此项发送给 root 和实现作者 app_firewall。此记录不代表作者后续修订已经复核。

## 已核对的边界

- `daemon.NewWithOptions` (`internal/daemon/daemon.go:142`) 的首个依赖是准备器；失败立即返回，先于 ruleset、日志、settings/profile/state 内容读取和启动恢复逻辑。`CaptureCrashes` (`internal/daemon/crash.go:40`) 同样先验证，再轮换/打开日志。
- 默认目录来自 `FOLDERID_ProgramData` (`internal/paths/paths_windows.go:20`)。显式 DATA 仍进入相同验证。固定核心文件名探测已落地 (`secure_windows.go:170`)，不再依赖枚举名称筛选；路径组件同时拒绝设备名及 COM/LPT 上标数字变体。
- 原生打开请求 `READ_CONTROL|FILE_READ_ATTRIBUTES`，共享 `FILE_SHARE_READ|FILE_SHARE_WRITE`，不共享 delete (`secure_windows.go:315`)。目录句柄保留到 release，文件核对后关闭。这能表达防改名/删除的 lease 意图，允许服务正常读写与原子替换；它不锁住内容，也不防恶意已提升管理员。真实共享语义尚未在本轮实测。
- 核心输入检查 owner、DACL、reparse、类型；文件要求单一硬链接。祖先仅额外允许固定 TrustedInstaller；可写入已有子项或修改 ACL/owner 的非受信权限被拒绝。消费子树受数量上限限制。
- 未知根项及可选 privacy/data-tool 子目录不会被打开或递归 (`secure_windows.go:109`、`:170`、`:191`)；它们自身异常不会直接 gate VPN。根目录本身不可信仍是核心错误。
- trust failure 仅释放已打开的存储句柄；没有创建 daemon，也没有调用 Close、Unprotect、网卡恢复或 guard 清理。`Daemon.Close` (`daemon.go:255`) 只关闭日志并释放存储 lease。Crash 准备失败同样只返回；成功的 crash lease 保留至进程退出。没有把失败当作自动直连理由。

## 本轮验证

通过：

```text
go test ./internal/paths -count=1
go test ./internal/daemon -run '^(TestStoreTrustFailureStopsBeforeAllInitialization|TestNewInstallsBuiltinRuleSets|TestHealsRedactedProfileURLOnStart)$' -count=1
```

paths 用例使用内存文件树及 SDDL 解析；daemon 定向用例只使用 TEMP 合成内容，不运行 Run/IPC/诊断/网络。

`SERVICE_STORE_TRUST_FIX.md` 对测试与待实机边界的分层总体准确，未把源码或 fake 结果冒充真实 SYSTEM 验收；但其“继承专用 ACE 不被当成当前对象的有效授权”仍需补充上面的未来子项边界，当前不能据此关闭 C02。保留的 logs/diag Users 读取及 C03/A01 不属于本次已关闭范围。

本轮已结束；未扩大为全部服务/IPC/安装器审计。真实共享权限、默认磁盘 ACE 兼容、Windows 继承效果、替换竞态与 persistent guard 实机验收仍按原文列为待授权验证。

## 作者修订后的最终回核

同日，收到作者完成上述 IO 修复后，再次只读核对指定变更并独立运行定向回归。本节更新前述历史结论：**本轮确认的 P1 源码阻塞已关闭；本次指定范围没有剩余已确认的源码阻塞。C02 的真实 Windows / SYSTEM 验收仍未关闭，不能将此结论标为实机通过。**

`checkStoreNode` 现在只对祖先模板、非目录上的 IO ACE、没有 OI/CI 的无效继承模板继续跳过。核心消费目录的 IO+OI/CI 会进入同一危险写 mask 和受信 SID 检查；BU、任意其他非受信 SID、CREATOR OWNER 不属于 SY/BA，危险可继承写授权均拒绝。此规则同样用于递归消费子目录。受信 SY/BA 写模板、普通用户只读模板及受显式私有产品根隔离的祖先 CO 模板保持分开处理，没有通过放宽已有对象 owner/ACL 或隐式迁移解决问题。

新增 `TestCoreDirectoryInheritanceCannotGrantFutureUntrustedWrites` 覆盖 profiles/rulesets/logs/diag 四树的 BU/CO 未来子项写权限拒绝和写入前返回；合成 SDDL 另外覆盖 CO、SY 写模板、BU 只读与祖先模板。源码谓词对其他非受信 SID 使用同一拒绝分支；本轮未另外创建真实账户或 ACL。原 IO-only、无 OI/CI 的 CO 用例属于不会授予未来子项的模板，没有重新开放上述缺口。

独立复跑 `go test ./internal/paths -count=1` 及本记录列出的三项 daemon 定向测试，均通过。`SERVICE_STORE_TRUST_FIX.md` 已同步说明未来子项继承边界，且仍将真实共享/继承效果和服务身份列为待实机项。可选隐私目录隔离、已核对失败路径及本轮非副作用范围保持不变；没有修改产品源码。
