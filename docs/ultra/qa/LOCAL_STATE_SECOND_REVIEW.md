# C04 用户恢复存储：独立二审

日期：2026-10-19。范围仅为 `localstate` 及 `privacy`、`timeprivacy` 用户 store 的 C04 集成；不重新扩大机器策略、VPN 或界面实现范围。

结论：冻结版本符合 root 已接受的最小读写分层，无新增已确认阻塞；**C04 仍为部分修复，不能关闭**。已有/意外远程与重解析路径被拒，严格只读 lease 已验证有效；写模式为保持生产原子保存而允许目录 write，因此下述同账号 in-place reparse 竞态仍保留。

## 已确认并反馈修复的问题

初版目录句柄仅用 `FILE_READ_ATTRIBUTES`，允许 `FILE_SHARE_READ | FILE_SHARE_WRITE`，未允许 DELETE。禁止重命名/删除不能同时证明已验证目录无法变成 junction。

独立受控验证全部在新建的本地 TEMP 根内完成：

`<TEMP>/ultra-c04-review-<test-id>`

步骤：创建两个空目录 `pinned`、`target`；按初版参数持有 `pinned`；另开 `GENERIC_WRITE` 句柄，调用 `FSCTL_SET_REPARSE_POINT`，将它变为指向同一 TEMP 根下 `target` 的 mount point；随后在 `pinned\probe-child` 创建目录。结果如下：

```text
PIN_OPEN=true
REPARSE_SET=True WIN32=0
CHILD_IN_TARGET=True
TEMP_CLEAN=True
```

这证明初版的 pin 仍存在“检查后改变目录类型，再按字符串路径操作”的窗口。前提是另一进程有权写该目录并在检查后、后续文件操作前竞争；这不是任意网站或任意普通账户可触发的远程攻击。仅使用本地目标，没有 UNC、网络、真实用户备份、系统策略、服务或 WFP 操作。先关闭句柄、仅删除 junction 本身，再逐叶删除测试目标；未对 reparse 目标执行递归删除。

问题已即时反馈给 root 与实现负责人。最终只读模式已加强，写模式仍保留已明确接受的窗口；本文不将已确认现象隐藏为纯推测或伪称完全修复。

## 静态核对

- 语法检查发生在磁盘映射查询及任何文件打开之前。UNC、扩展设备命名空间、相对/盘符相对路径、ADS、保留设备名称和歧义尾字符均由纯字符串检查拒绝。
- 驱动器先经 `QueryDosDevice` 映射和固定磁盘类型检查；明确网络映射、SUBST 和无法识别的映射不进入目录打开。此检查本身不是对系统命名空间主动并发篡改的原子认证。
- 从卷根逐级打开并验证目录，再访问下级。缺目录的只读检查立即返回 `os.ErrNotExist`，不创建、不探测后续子目录；写入创建后必须重新打开验证。失败分支释放已持有的祖先句柄，正常释放使用 `sync.Once`。
- 用户隐私 Report 的备份错误只降低备份健康和可写能力；固定策略只读事实仍返回。时区 Report 仍返回已读取的当前时区和夏令时偏好，标记 `backup_error`，不伪造恢复成功。
- `privacy.NewMachine` 使用独立 lock/read/persist；用户 guard 分支被 `!m.machineOnly` 限定。`privacyhost` 创建机器管理器和 WFP 管理器，不依赖用户 store。GUI 的机器项目分类仍可取得固定目录；用户恢复失败后仍尝试机器恢复。VPN/guard 生命周期不导入 `localstate`。

## 验证边界

二审不运行真实隐私策略 Apply/Restore、机器注册表、服务安装、UAC、网络或 WFP 流量测试。fake 的策略读写、真实 TEMP 文件/句柄操作和静态依赖证据分别记录，不将它们等同于 VPS 上的实际策略或防泄漏验收。

本功能防止常见已有/意外的网络重定向和重解析路径；它不是对已能控制同一账户/管理员进程、DOS 设备映射或系统命名空间的攻击者提供的绝对零外联承诺。用户状态目录原本只有 traverse 权限而没有目录读取权限、映射类型无法确认或存在并发 writer 时，可以只禁用本模块的自动改动并提示手动处理，不能因此削弱隧道或关闭其他保护。

## 冻结版本的读写分层

`internal/localstate/directory_windows.go:149` 统一申请 `FILE_LIST_DIRECTORY | FILE_READ_ATTRIBUTES`。`create=false` 的只读模式仅共享 READ；`create=true` 的写模式共享 READ/WRITE。两者都不共享 DELETE。`create` 同时选择租约模式，当前调用点与该约定一致：

- 用户 `privacy/store.go:45`、`timeprivacy/store.go:39` 的加载/报告为严格只读模式，且持有到 journal 文件关闭。
- 两包 Windows `secureDirectory`、`lockStore` 及 `durableWrite` 均为写模式。进程间 lock 持有父级 lease 直到 unlock；短期 `secureDirectory` 和持久写入的嵌套 lease 不替代外层 lock。
- 两包生产替换继续使用既有 `MoveFileEx(REPLACE_EXISTING | WRITE_THROUGH)`；没有留下测试专属的相对 NT rename 来掩盖生产保存失败。

严格只读租约的数据访问权是实际 sharing 检查所需条件；仅从共享标志删 WRITE、却仍仅申请 READ_ATTRIBUTES，不足以据此声称抗写。独立运行的原生 TEMP 测试已确认最终 read lease 既拒绝后来的 writer，也在已有 writer 时拒绝获取；释放后 writer 可重新打开。

写模式仍允许 writer，测试也明确验证这一点，同时验证 DELETE 句柄被 sharing violation 拒绝。因此前述“空目录原地变为 junction”的能力在写模式中没有被关闭；当前改进是先拒绝已观察到的不支持路径与重解析，再阻止目录对象通过删除/改名被替换。并发命名空间突变和写入期间的外联风险不能由该前置检查排除。只读/写入竞争还可能造成安全拒绝、保留待恢复 journal；这不构成丢弃备份或关闭网络保护的理由。

## 独立复验

实现负责人通知冻结后，二审实际执行并全部通过：

```text
go test ./internal/localstate ./internal/privacy ./internal/timeprivacy ./internal/privacyhost ./internal/appfirewall -count=1 -v
go vet ./internal/localstate ./internal/privacy ./internal/timeprivacy
go test ./cmd/godusevpn -run TestPrivacyPlaneHasNoTunnelMutationOrHTTPDependency -count=1
git diff --check
```

验证内容分别为：

- 纯内存 FS：拒绝路径不进入 drive/file/create；映射盘拒绝不打开文件；reparse 不进入下级；只读缺目录不创建；创建成功后第二次检查失败不探下级；成功/失败 lease 均释放，release 幂等。
- TEMP 原生句柄：strict read writer 双向拒绝、释放生效；write 模式允许生产所需 writer，但实际拒绝目录 DELETE；嵌套 lease 下创建与普通替换通过。
- 两包 `TestLocal*StoreProductionLockPersistenceAndRestoreRoundtrip`：真实 TEMP 目录、生产 lock、实际目录 ACL 设置、生产 durable write/MoveFileEx、严格读取、恢复后再次保存全部通过；registry/time backend 始终为 fake。
- 用户备份路径拒绝测试：journal leaf stub 没有调用；Report 保留策略或时区事实并明确备份错误；Apply/Restore/RestoreAll 没有 fake 策略写入、时区设置或持久写入。
- machine recovery/机器 TEMP ACL 及 helper 分派回归通过；appfirewall 为 fake/ABI 回归，未安装 WFP；GUI 隐私平面 AST contract 没有隧道 mutator/HTTP 依赖。静态调用检查与这些回归一起支持模块隔离结论，不将其写成真实流量测试。

`git diff --check` 退出码 0，仅有仓库已有 LF/CRLF 转换提示。没有运行真实当前用户事实采集、VPS、UNC/网络映射、实际策略、服务、UAC 或流量验收。已完成的复验足以确认本批生产保存没有被 pin 改动破坏；它不消除文中保留的 C04 写侧竞态。
