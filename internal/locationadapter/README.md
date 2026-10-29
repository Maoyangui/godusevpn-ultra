# Windows 位置组件适配边界

`internal/locationadapter` 实现组件哈希检查、备份、状态冲突检测和恢复事务。目前没有真实 Windows 位置后端，也没有携带、下载或执行第三方定位程序。纯 fake 后端单测通过，不代表系统 GPS 已接入或真实位置已经改变。

`RuntimeSupportStatus` 现在只读取本机 Windows build，并以官方 `GeolocationProvider` 文档门槛 23504 做前置门禁：低于该 build 明确返回不可用；达到门槛也只表示 WinRT 类型可能存在，仍需 Microsoft LAF、`runFullTrust`、运行时成员检查、会话所有权和原子写入后端。该检查不会请求位置、启用组件或写入坐标。

2026-10-26 实际源码审计发现，原候选 [locsim 的固定提交](https://github.com/itznan/locsim/tree/1bbd1007d127eb1890e14610dc7c798885c12de3) 的 Windows 实现只读写自建注册表/文件状态，未调用 WinRT 设置或独立读取系统位置，恢复也没有原状态备份。因此不将它作为真实 Windows 位置后端。详见 [GPS 上游审计](../../docs/ultra/qa/GPS_UPSTREAM_REVIEW_2026-10-26.md)。

## 事务约定

- 组件须有 HTTPS 来源、版本、许可、绝对本地文件路径和匹配文件的 SHA-256。这些检查只能证明元数据齐全、文件匹配指定哈希，不能代替上游能力/来源审计。
- `Backend.Read` 返回组件 override 的独立状态。`Present` 不能表示“读到了普通物理坐标”；`Raw` 是稳定的恢复/所有权数据，不能包含每次变化的采样时间。
- `Backend` 只有读取接口。修改必须另外实现 `AtomicBackend`，并通过不改配置的 `CheckAtomic` 运行时检查。缺少原子能力时，在创建事务备份和修改 provider 前拒绝；仅声明接口并不证明实现正确，仍需源码审计及真实平台验证。
- `PrepareApply(before, position)` 必须在不改 provider 配置的情况下返回完整目标，`Raw` 中带可跨重启验证的稳定所有权/代次令牌。`CompareAndSwap(expected, replacement)` 必须在 OS 排他事务内原子核对全部配置和所有权再替换，拒绝未知外部所有者/代次变化；先 Read 后无条件写入不是 CAS。
- v3 备份的 `snapshot` 保存首次状态，`expected` 记录当前已确认源状态，`pending` 在每次修改前保存完整目标，包括 `Raw`。重复 Apply 保留最初状态。恢复未完成的 Apply 时，先将实际被接受的源状态记入 `expected`，使恢复操作本身再次中断后仍可恢复。
- Apply、Restore 和失败清理全部使用条件替换。即使外部修改发生在读回之后或清理时，后端也必须原子拒绝不匹配的状态，不能覆盖它。恢复还校验组件 ID、来源、版本、许可证、哈希和管理员要求。
- 清理使用不受原请求取消影响的 10 秒超时，并独立读回核验。恢复成功保留备份；重复恢复为无修改操作。损坏、未知版本和旧 v1/v2 备份保留，拒绝自动迁移覆盖，因为旧格式不能证明完整目标所有权。
- 所有跨后端边界的 `Snapshot.Raw` 都深拷贝，避免后端重用缓冲区或修改传参时破坏初始备份。Inspect 同样校验读回坐标。
- 包级互斥仅用于本进程控制器；跨进程/系统原子性由经过核验的真实后端提供。不能通过仅比较文件缓存或坐标实现上述合同。

文件接入前先词法拒绝两类斜杠的 UNC、设备命名空间、ADS 和常见歧义路径，避免 Inspect 触发远程文件读取。位置后端接入前仍需完成 Windows 目录/文件 DACL、父目录重解析与文件替换防护。`Chmod(0600)` 不是 Windows DACL 的替代。坐标及 `Raw` 只应在本地受保护备份保存，不能放进报告或日志。

## 真实后端的下一步

首选候选是基于 Microsoft 官方 [`windows-rs` 的真实 WinRT 绑定](https://github.com/microsoft/windows-rs/blob/b1fb6408fb333dd3823f57b6fb30e6f113ad3aed/crates/libs/windows/src/Windows/Devices/Geolocation/Provider/mod.rs) 编写第一方常驻 helper。`GeolocationProvider` 需要 `runFullTrust`、Microsoft LAF 授权及运行时 API 检测；单纯管理员权限不能代替这些条件。helper 必须持有同一个 provider 对象管理 Set/Clear 会话。缺少授权/接口、会话被其他组件占用时应清楚报告不可用。

WinRT 的 Set/Clear 绑定本身没有证明满足本包的 OS 原子比较/恢复合同。必须先验证整个会话模型确实能保证该边界；做不到时 `CheckAtomic` 必须返回 `ErrAtomicUnsupported`，不能套上本地锁或读回检查后开放写入。

真实读回应另外通过 `Geolocator` 请求新鲜观测，核对采样时间、来源和经纬度容差。当前 `Position.equal` 是精确配置比较，适用于本包稳定 fake/configuration 快照，不能直接用来比较有舍入/精度变化的实际系统位置观测。正常物理位置在移动，恢复应恢复 override 状态，不能要求物理位置回到旧坐标。

`observation.go` 现在提供独立的纯校验函数 `CheckOverrideObservation`。调用方须从实际 `Geocoordinate` 取得坐标、精度、采样时间、来源及 `IsRemoteSource`，并另外提供本次成功 Set/update 的时间；不能把请求参数或组件缓存填回观测。它拒绝早于本次操作、超龄、明显未来、来源未知、普通非 remote、过粗或距离不符的结果。微软说明即使设置 `maximumAge` 仍可能收到更旧的缓存，所以超时/请求参数不代替采样时间检查。默认限制为 10 秒观测年龄、1 秒时钟偏差、100 米报告精度上限、5 米独立二维匹配距离；粗精度不会扩大匹配半径。该策略不是物理定位精度承诺。

观测匹配仅返回无坐标的 `observation_matches_request`；`applied` 和 `providerOwnershipVerified` 始终为 false。`SystemObservation` 默认 JSON 与格式化均不显示坐标/时间。该函数不认证数据来源、不调用系统、不证明会话所有权，也不开放现有 `AtomicBackend`。实际 helper 运行身份、授权、会话与独立观测方案见 [Windows provider 接入范围](../../docs/ultra/qa/WINDOWS_LOCATION_PROVIDER_SCOPE_2026-10-26.md)。

Windows Device Portal 可作为已配置的隔离测试机开发后端；不得为了它自动启用本机开发模式、Portal 或定位服务。Microsoft GNSS UMDF 样例是真实驱动基础，但仍需动态坐标接口、签名和生产验证，不能直接打包成已完成组件。

任何后端的影响范围都应明确：Windows Location Services、浏览器 CDP、IP/GeoIP、Wi-Fi 扫描、账号历史与应用自建定位是不同通道。真实 GPS 芯片或绕过系统服务的应用也不能由本适配器统一保证。
