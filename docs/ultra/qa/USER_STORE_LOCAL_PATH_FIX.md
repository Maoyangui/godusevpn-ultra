# C04：用户恢复存储本地路径边界（部分修复，未关闭）

2026-10-19；只改 Ultra 副本。原项目、机器 store ACL、UI 品牌和隧道逻辑未改。此记录分开说明已修范围、确认兼容性和剩余窗口；不能把“不发送 HTTP 探测”写成全进程零外联。

## 前提与已修范围

C04 的前提是用户 APPDATA/调用者提供的恢复路径实际被配置为 UNC、映射网络盘、设备路径，或包含通向其他位置的重解析目录。普通本地 APPDATA 不会因此自行联网。先前两个用户 journal 在 Lstat/Open 前没有本地边界；写侧 MkdirAll 又早于 reparse 检查，文件系统可能先接触远程位置。没有真实凭据外传的观察证据。

新增 `internal/localstate`，仅用于 privacy/timeprivacy 的**用户恢复存储**。GUI 和 CLI 继续使用既有路径来源和接口；不忽略原 APPDATA 静默切换位置，不复制/迁移备份，不自动改用户系统目录配置。

1. 在任何 journal/lock 的文件打开、目录创建、持久写入之前，先纯语法拒绝 UNC、扩展/设备命名空间、相对/驱动器相对路径、ADS、保留设备名与歧义名称。
2. 先查本地 DOS 盘符设备映射，只接受可识别的 HarddiskVolume 数字设备，再确认 `DRIVE_FIXED`。网络提供者、SUBST/其他别名、不明或不可用磁盘均保守拒绝；不先对网络盘调用文件打开。
3. 本地目录从盘根逐级以 `OPEN_REPARSE_POINT` 打开，检查目录本体；已有 reparse、非目录或未知结果立即拒绝，不继续访问它的下级文件。只读路径缺失时返回“无本地记录”，不创建或探测更深路径；写入只在已验证并持有的本地父级下逐个创建，创建后重新打开验证。
4. 目录句柄请求 `FILE_LIST_DIRECTORY|FILE_READ_ATTRIBUTES`。单独 READ_ATTRIBUTES 不足以使数据共享约束真实生效，新增本地测试确认了这一差异。所有模式都不共享目录 delete，限制被核对目录的改名/替换。叶 journal/lock 使用 no-follow 打开并核对普通磁盘文件，拒绝叶重解析和设备。

privacy `Report` 不能读取恢复存储时保持 `BackupHealthy=false`、所有相关 `CanApply=false`，同时继续给出本来安全可读的固定 HKCU/HKLM 策略值证据，说明备份未读取。不能因无法读取 journal 就说未管理、安全或不存在旧备份。timeprivacy `Report` 同样保留当前时区、夏令时偏好及本机枚举选项，状态为 `backup_error`；自动应用和恢复仍会拒绝。错误不包含原始路径或服务器名。

机器 `NewMachine` 的独立 ProgramData 上下文、ACL、lock、persist、owner/备份验证不改；用户 store 故障不会阻断独立机器恢复，也不会调用 VPN connect/disconnect、guard 或网络恢复。

## 读写分层与明确剩余窗口

**只读报告使用严格 read lease**，不共享目录 write。原生 TEMP 测试确认：持有该 lease 后，修改 junction 所需的数据写句柄被 sharing violation 拒绝；已有写句柄时不能取得该 lease，保守报备份不可用。未知/竞争结果不能转成“安全”。

**写入使用 write-compatible lease**，共享目录 read/write、不共享 delete。原因是 Windows 普通原子文件替换会打开父目录进行写操作；对父目录持严格 read lease 时，生产 MoveFileEx/os.Rename 无法保存 journal。曾在本地 TEMP 验证相对 RootDirectory 的 NtSetInformationFile，同样返回共享冲突，没有据此引入另一套手写 NT 持久化。最终保留现有生产 atomic persistence 和恢复语义，不为构建掩盖“备份无法保存”的回归。

写模式仍在操作前拒绝直接远程命名空间、网络/不明盘及检查时已有的 reparse，并持有真实防目录 delete/rename 的 lease；**但同账号并发进程仍可能原地修改目录 reparse 数据，在检查后改变路径解析。此窗口本批未完全消除，C04 因此保持“部分修复，未关闭”。** 消除此写侧窗口需要单独设计所有创建/打开/替换/清理都基于已验证目录对象的方案，不能只把换名函数改成相对名字就宣称解决。

两种模式都不承诺抵抗同账号/管理员主动改 DOS 设备映射、恶意内核/驱动，或其他进程自身的网络活动。支持的是所列应用恢复路径边界，不是整个 Windows 零网络证明。企业重定向、遍历可达但没有 LIST_DIRECTORY 的特殊目录及非标准磁盘映射可能被拒绝；应通过明确的受支持本地配置方案处理，不自动搬运历史备份。只读与写入竞争也可能导致安全拒绝/保留待恢复 journal。

## 验证与范围

```text
go test ./internal/localstate ./internal/privacy ./internal/privacyhost ./internal/timeprivacy -count=1
go vet ./internal/localstate ./internal/privacy ./internal/timeprivacy
```

纯内存 stub FS 证明：UNC/设备路径在 drive/file/create 调用前拒绝；映射网络/未知盘不进入文件打开；reparse 不进入下级；只读缺目录不创建；写创建前父级已持有、创建后验证失败不继续下级、错误和成功均释放 lease。用户 report/load 的叶打开 stub 证明被拒路径不进入 journal I/O；fake registry/time backend 证明安全只读事实保留、无真实策略写入。

原生验证只使用新建 TEMP 合成目录：严格 read lease 拒绝 writer/预持 writer，write lease 实际拒绝目录 delete，嵌套 lease 下创建和普通文件替换可工作。privacy/timeprivacy 新增生产 lock、实际目录 ACL 设置、生产 durable write、读取和恢复轮回测试；注册表与时区 backend 始终为 fake，没有改变真实设置。没有真实 UNC、网络盘、凭据、服务、UAC 或隧道操作。

这些 TEMP 结果不能代替企业 APPDATA、真实 Windows SKU/用户权限、SYSTEM/替代管理员、映射变化或生产读写负载兼容验收。原有数据目录 ACL 迁移和诊断泄露另有审计范围，本记录不将其关闭。
