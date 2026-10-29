# C02：Windows 主服务数据存储信任修复

本记录仅针对 Ultra 副本。原项目未改；本次未启动服务、隧道、诊断采集、网络请求或安装/卸载，也未在真实目录上改 ACL、owner 或账户。

## 修复的行为

原流程先建目录、铺规则、构造日志，再尝试收紧根目录 DACL；收紧失败仍加载设置与订阅缓存。现在 `daemon.NewWithOptions` 的第一个依赖是 `paths.PrepareServiceStore()`。验证/创建/收紧中任一步失败，立即返回“服务数据存储验证失败；现有网络保护保持不变”，不继续使用该存储。

`CaptureCrashes` 比主初始化更早被服务入口调用，现在也先经过同一准备器。失败时不创建、轮换或打开 `crash.log`；随后主初始化返回明确错误。失败路径不调用撤闸、网卡恢复、DNS/路由恢复或直连重试。它不会主动创建新保护；已有 persistent guard 保持原状。

这是拒绝不可信**核心服务输入**的故障，不是把可选隐私策略的应用失败挂到 VPN 初始化上。可选 `privacy-machine`、数据工具存储和未来未知子目录不在主服务输入白名单中，不读取、不遍历其文件或按其 ACL/内容拒绝服务。

## 目录信任规则

- 默认根来自 Windows `FOLDERID_ProgramData`，不再接受同名 `ProgramData` 环境变量；KnownFolder 失败不退回工作目录或猜测路径。
- 显式 `GODUSEVPN_ULTRA_DATA` 保留，但与默认根执行同样验证。只接受本地固定磁盘绝对路径；拒绝 UNC、相对路径、设备命名空间、ADS、点/空格歧义、保留设备名和磁盘根目录。
- 从磁盘根逐段向下打开目录；用 `FILE_FLAG_OPEN_REPARSE_POINT` 检查对象本身。祖先和核心子项的重解析点均拒绝，不访问链接目标。
- 核心数据根、消费的子目录与文件 owner 只允许 SYSTEM 或 BUILTIN\Administrators。Windows 祖先额外允许固定 TrustedInstaller SID。不把“某个个人管理员账号”直接当作受信 owner，因为其普通令牌可能仍控制自己拥有的对象。
- 祖先可以允许创建新的兄弟目录（ProgramData 的常见布局），但不允许非受信主体删除/替换已有子项、删除祖先自身、重写 DACL/owner 或获得通用写权限。核心根与消费子项还拒绝非受信写数据、追加、写扩展属性/属性权限。
- NULL DACL、未支持的有效对象/条件 ACE、既有文件多硬链接、核心文件/目录用途与类型不符均拒绝。祖先的继承专用模板不等于当前对象授权；核心消费目录的 `INHERIT_ONLY` 若带 OI/CI，则仍审查它对未来子项的授权。普通用户与 CREATOR OWNER 的危险可继承写模板均拒绝，不把 CO 默认当作 SYSTEM/BA；受信 SY/BA 写模板和只读模板可保留。拒绝 ACE 不用于掩盖另一条危险允许 ACE。
- 新建目录在 `CreateDirectory` 时直接使用 BA owner 和受保护 DACL，没有“先公开创建、再收紧”的窗口。既有目录不设置 owner，不调用 take-ownership。
- 所有已存在的核心输入都验证后，才进行根目录和既有 logs/diag 兼容 ACL 的收紧。收紧失败不降级成警告。

校验时持有目录句柄，不允许 `FILE_SHARE_DELETE`；已核对的祖先和核心子目录在服务生命周期内被保留，降低校验后路径被换名/替换的竞态。`Daemon.Close`/`Run` 退出释放正常存储句柄；崩溃输出使用的句柄保留至进程退出。文件仅核对元数据，不长期锁住内容文件，避免阻止服务正常原子替换配置。

威胁边界是普通用户不能借目录或既有核心文件让特权服务消费其输入；不是阻止另一个已提权管理员、SYSTEM、内核或恶意磁盘驱动修改系统。

## 主服务消费集合

根目录不依赖枚举名称或 Go 大小写折叠决定是否检查；始终以服务稍后使用的固定 ASCII 路径逐项打开并核对下列集合。这样即使文件系统将某个不同枚举名解析成同一固定输入，也不会被枚举过滤跳过。

| 类别 | 输入 |
|---|---|
| 根文件 | `settings.json`、`profile.json`、`config.json`、`config.last-good.json`、`state.json`、`cache.db`、`ui.json`、`controller.sid`、`nic-ipv6-backup.txt`、`nic-ipv6-lost.txt` |
| 写入/隔离旁路文件 | 上述根文件的 `.tmp`、`.bad`；`cache.db-wal`、`cache.db-shm`、`cache.db-journal` |
| 递归消费子树 | `logs`、`diag`、`profiles`、`rulesets` |
| 不消费 | 未知根文件/目录，包括可选隐私助手与数据工具状态；不检查其内部元数据/内容 |

根本身仍必须可信。未知子目录不属于主服务消费集合，不等于为其他组件背书；其他组件必须独立验证自己的路径。未来主服务新增根输入时，必须同步更新 `serviceRootFiles`/`serviceDirectories` 和测试，不能直接绕开准备器。

递归检查上限为 20,000 个消费子项，超限明确拒绝初始化，不静默跳过剩余文件。

## 便携模式、历史目录与恢复

显式 DATA 可放到管理员保护的本地固定磁盘路径；需要相应权限创建 BA owner/private DACL。普通用户可替换的 Desktop、用户配置目录或 TEMP 祖先不能作为 SYSTEM 服务的可信输入。此限制不影响复制 EXE 的便携分发方式，但不能把“管理员运行 EXE”当作用户可写数据路径安全的证明。

已有普通用户/个人管理员 owner、用户可写 ACL、链接或无法识别的安全描述符时，不自动接管、不自动复制内容再当成可信、不删除原件。管理员应先停止相关进程、离线保留备份，核对目录来源及合成/真实数据边界，建立受信位置，仅迁移经过审查的核心设置/规则/缓存。该流程应另行审核，不在本修复中执行或提供一键绕过。既有 guard 可能继续阻断网络；需要用户明确、受作用域限制的恢复流程，不能为了启动成功静默撤掉。

`NoListen` 只控制 IPC 监听，不降低生产信任规则。daemon 测试的 TEMP 初始化由 `_test.go` 内专用 fake preparation 替换，测试不获取真实 ACL 权限；发行二进制不包含此替换。

本次保留原 logs/diag 面向 Users 的只读兼容边界；不宣称诊断内容已经匿名，不关闭审计 C03/A01。根 ACL 收紧不是完整的跨账号读取修复。IPC 登记写入、隐私 journal 与用户存储路径的其他检查分别由各自修复处理，本记录不将它们标为通过。

## 无副作用验证

以下命令已在本地通过：

```text
go test ./internal/paths -count=1
go vet ./internal/paths
go test ./internal/daemon -run '^(TestStoreTrustFailureStopsBeforeAllInitialization|TestNewInstallsBuiltinRuleSets|TestHealsRedactedProfileURLOnStart)$' -count=1
GOOS=linux go test -c -o <TEMP>/godusevpn-ultra-paths-linux.test ./internal/paths
```

最后一条仅做非 Windows 编译，未运行生成文件。Windows paths 测试仅使用内存 fake 文件树与 SDDL 解析，不用 icacls、真实账户或目录 ACL 操作。daemon 三项定向测试只构造对象/读写 TEMP 中合成内容，不运行 `Run`、控制口、真实网络、诊断采集或策略。

已覆盖：

1. 新目录私有创建顺序、目录句柄保留、幂等释放。
2. 普通用户 owner、祖先可替换权限、祖先/根/关键文件 reparse、文件多硬链接、文件显式普通用户写权、子目录创建权及未支持 ACE 的拒绝。
3. 硬化失败的致命返回与失败句柄释放。
4. UNC/相对/设备/ADS/歧义路径和非固定磁盘的拒绝；多重尾斜杠磁盘根不触发越界。
5. 默认根忽略 ProgramData 环境覆盖；保留显式 Ultra DATA。
6. 可选隐私/数据目录为不可信 owner 或 reparse 时不打开其路径，也不阻止主服务；四个核心消费子树的相同异常仍会拒绝。
7. `NoListen` 下 fake 信任失败立即返回，未创建存储/规则/日志；提前的 crash 捕获也不写文件。
8. 原有内置规则铺设、合成订阅缓存修复仍工作。
9. 核心目录显式 OI/CI+IO 的 BU 或 CREATOR OWNER 写模板会在创建 `.tmp`/缓存之前被拒绝；祖先 CO 模板、核心 SY 写模板和公共只读模板另有内存 SDDL 分支验证。

补充名称复核：仅调用 `CompareStringOrdinal(ignoreCase=true)` 比较内存字符串，在本机 `settıngs.json/settings.json`、`ſtate.json/state.json`、`profıles/profiles` 均为不等；这些候选没有通过该 API 证明别名利用，也未建立真实文件验证 NTFS 名称行为。实现仍改为固定路径探测，并以 fake 文件系统模拟“固定路径解析到另一个枚举名”的情况，确认不可信核心输入会拒绝。保留设备名另补 `COM/LPT` 的 `¹²³` 变体及 `CONIN$`/`CONOUT$`，在任何原生文件打开前拒绝；不宣称此前已被实际利用。

**待隔离实机验收：** 默认 ProgramData/磁盘根实际 ACE 兼容性、SYSTEM 与提权便携运行、双账号预置与替换竞态、真实句柄共享/重解析/硬链接拒绝、服务停止后的句柄释放和既有 guard 保留。上述 fake 通过不代表这些 Windows 系统行为已经实测。应按 [VPS 测试运行手册](../VPS_TEST_RUNBOOK.zh-CN.md) 的恢复前提与批次控制执行，不在用户原项目或本机真实产品目录演练。
