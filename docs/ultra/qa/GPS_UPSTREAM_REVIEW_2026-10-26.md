# GPS 上游源码核查与适配事务修复

审计日期：2026-10-26（Asia/Shanghai）。真实 Windows GPS 后端尚未完成；locsim 不能作为该能力的已验证实现。本次只读取官方上游、下载源码到 Codex 临时目录并运行本包纯 fake/TEMP 测试，没有执行第三方程序、驱动安装、定位操作或系统/网络/服务修改。

## 1. locsim：固定版本与实际能力

固定提交：[`1bbd1007d127eb1890e14610dc7c798885c12de3`](https://github.com/itznan/locsim/tree/1bbd1007d127eb1890e14610dc7c798885c12de3)，日期 2026-09-25；[Cargo.toml](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/Cargo.toml) 的版本为 `0.1.0`。[LICENSE](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/LICENSE) 实际提供 MIT 文本；README/Cargo 声明 MIT OR Apache-2.0，但仓库没有单独 Apache 许可证文件，不能把元数据当作完整许可证证据。

逐项读取 [Windows 实现](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/src/platform/windows.rs) 后确认：

- `set_location`（47 行起）生成 JSON、GPX、CDP JSON、PowerShell 脚本，写自己的 HKCU 路径并尝试写 HKLM Sensor Overrides 字符串。没有 WinRT 调用、传感器驱动或系统位置注入接口；不能由这些键的存在推导系统使用其坐标。
- `get_location`（130 行起）仅读自建 HKCU 或自己的 JSON。它不是独立的 Windows 系统位置读回。
- 注册表写入错误被忽略；`clear_location`（167 行起）忽略删除错误并删除相应整棵子树，无原状态备份，无法证明可靠恢复。
- 导出的 PowerShell 可展开字符串仅转义双引号，未处理表达式。若不可信位置名称/地址写入脚本并被执行，存在命令执行风险。

[CLI 测试](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/tests/cli_tests.rs) 只验证自身行为，未证明某个具体 Windows build 的系统定位往返。手工坐标也在 [main.rs](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/src/main.rs#L293) 先请求反向地理编码；[reset 的网络实现](https://github.com/itznan/locsim/blob/1bbd1007d127eb1890e14610dc7c798885c12de3/src/real_location.rs#L25) 使用明文 HTTP GeoIP 查询。这不能证明真实物理位置或恢复成功。

上述结论来自固定源码，未运行该工具。README 的 Windows Location Provider、WinRT 或恢复声明不能作为本项目验收证据。

## 2. 可用官方接口与真实开源基础

### 建议路线：第一方常驻 WinRT helper

Microsoft 官方 [`windows-rs`](https://github.com/microsoft/windows-rs) 提供真实 WinRT ABI 绑定。固定提交为 [`b1fb6408fb333dd3823f57b6fb30e6f113ad3aed`](https://github.com/microsoft/windows-rs/tree/b1fb6408fb333dd3823f57b6fb30e6f113ad3aed)。[Provider 源码](https://github.com/microsoft/windows-rs/blob/b1fb6408fb333dd3823f57b6fb30e6f113ad3aed/crates/libs/windows/src/Windows/Devices/Geolocation/Provider/mod.rs) 实际通过系统对象 vtable 调用 IsOverridden、SetOverridePosition、ClearOverridePosition；[MIT](https://github.com/microsoft/windows-rs/blob/b1fb6408fb333dd3823f57b6fb30e6f113ad3aed/license-mit) 和 [Apache-2.0](https://github.com/microsoft/windows-rs/blob/b1fb6408fb333dd3823f57b6fb30e6f113ad3aed/license-apache-2.0) 均有实际许可文本。这是可靠的接口库，不是已完成的 GPS 产品。

[微软 GeolocationProvider 文档](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider?view=winrt-26100) 的 Windows requirements 标注 Windows 11 Insider Preview、引入 build `10.0.23504.0`、UniversalApiContract v15，并要求 `runFullTrust` 与 Limited Access Feature 解锁。页面还显示较广 SDK Applies to 列表，不能据此承诺 Windows 10 或任意 Windows 11 可用。发布范围必须依据运行时 API 检测、官方 LAF 授权和各目标 build 实测。

| 适配动作 | 真实 helper 行为 | 验证条件 |
| --- | --- | --- |
| 能力检查 | 检测 API、应用身份/runFullTrust、LAF 解锁 | 缺失时报不可用；提权不代替授权 |
| Apply | 常驻对象调用 SetOverridePosition | 检查所有 LocationOverrideStatus；保存自有会话身份 |
| Read | 独立读取自有 override 的稳定状态 | 普通物理坐标不能当作已应用；自建缓存不是系统证据 |
| 独立 Observe | 另一个 Geolocator 请求新鲜系统位置 | 检查采样时间、IsRemoteSource、距离/精度容差和超时 |
| Restore | 同一对象清除自有 override 或恢复自有配置 | 不接管其他程序会话，独立核对恢复结果 |

微软将覆盖会话与 Set/Clear 及目标用户上下文关联，所以必须保留 provider 对象；一次 set 后退出的 CLI 不足以形成可靠会话。真实位置另用 [Geolocator](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.geolocator?view=winrt-26100) 观测。应用能识别 IsRemoteSource，不能宣称 override 不可检测。

### Microsoft GNSS UMDF：真实驱动基础，尚不能直接投产

固定 [`Windows-driver-samples` 提交 `bb68ed5e770bce04dc5395ec9258dafab034184d`](https://github.com/microsoft/Windows-driver-samples/tree/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss)。[Device.cpp](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss/gnssUmdf/Device.cpp#L98) 注册真实 GNSS 设备接口，[Queue.cpp](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss/gnssUmdf/Queue.cpp#L151) 处理 GNSS IOCTL，[FixSession.cpp](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss/gnssUmdf/FixSession.cpp#L455) 回传 GNSS 数据结构。坐标仍是硬编码示例，动态设点需自行实现。

[README](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss/ReadMe.md) 明确不是生产驱动，集成者负责签名、安装与验证。固定版本 [INF](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/gnss/gnssUmdf/gnssUmdf.inf) 最低平台段为 build 22000，不能用旧文档中的 Windows 10 设计说明替代包兼容性证据。[许可为 MS-PL](https://github.com/microsoft/Windows-driver-samples/blob/bb68ed5e770bce04dc5395ec9258dafab034184d/LICENSE)，不在当前适配器允许列表。未选择/安装该驱动，不应直接扩充许可列表或关闭系统安全能力来使其可用。

### Device Portal：隔离测试机候选

[官方 REST API](https://learn.microsoft.com/en-us/windows/uwp/debug-test-perf/device-portal-api-core#location) 为 Windows Desktop 提供 override 开关和注入位置读回，要求开发模式。只能在已显式配置的隔离测试机开发验证，不能自动启用本机开发模式或 Portal。文档中设置位置与读取位置的路径不对称，需在目标系统确认，不能补猜 endpoint 后宣称成功。

已读 [`WindowsDevicePortalWrapper` 固定提交 `81fd28186c89a7661efe4ca305cf73920da0a1f7`](https://github.com/microsoft/WindowsDevicePortalWrapper/tree/81fd28186c89a7661efe4ca305cf73920da0a1f7)，[许可为 MIT](https://github.com/microsoft/WindowsDevicePortalWrapper/blob/81fd28186c89a7661efe4ca305cf73920da0a1f7/License.txt)，但没有 location endpoint 封装，不能当作现成 GPS 后端。

本次未发现可直接装入当前发布包、同时满足真实 Windows 实现/独立读回/可靠恢复/许可和签名/版本验证的即插即用开源工具。可行下一步是第一方 WinRT helper，而不是继续套用 locsim 的能力声明。

## 3. 已完成的事务修复与验证

本包备份最终升级为 v3：`snapshot` 保留首次应用前状态，`expected` 保存最近独立确认的源状态，`pending` 在操作前记录包含稳定所有权 `Raw` 的完整目标。重复 Apply 不覆盖初始状态。恢复未完成 Apply 时先把当前被接受的完整源状态保存为 expected，再保存恢复目标，因此恢复本身再次中断也能继续。Restore 核对组件 ID、来源、版本、许可、哈希和权限要求；损坏或旧 v1/v2 备份保留，不自动迁移覆盖。

二次审查指出，先读后无条件恢复仍会覆盖操作期间的外部修改；只预写坐标还会丢失目标 Raw，导致自有崩溃事务不能恢复。已明确调整接口：Backend 只读，修改必须实现 AtomicBackend，其 CheckAtomic 在创建事务/修改配置前检查能力；PrepareApply 无配置修改地返回带完整所有权令牌的精确目标；CompareAndSwap 必须由 OS 排他事务完成状态/所有权核对与替换。Apply、Restore、失败清理均只调用该条件替换，不能用先 Read 后写入模拟 CAS。平台不能提供该保证时须返回 ErrAtomicUnsupported，在修改前拒绝。

失败清理不受原请求取消影响，有 10 秒超时并独立读回核验。未知状态或所有权冲突只保留恢复线索，不覆盖外部状态。所有跨后端边界的 Snapshot.Raw 都深拷贝，防止后端重用切片破坏最初备份。Inspect 拒绝非法读回。文件检查前词法拒绝正/反斜杠 UNC、设备命名空间、ADS 与常见歧义路径，避免只读检查引发远程文件访问。包级互斥只保护本进程回执；临时文件重命名前同步写入。

验证命令：`go test -count=1 ./internal/locationadapter`、`go vet ./internal/locationadapter` 均通过。测试仅使用 fake 后端、测试组件文件及 TEMP 目录，覆盖初始状态保留、身份不匹配、存储失败、中断恢复和组件替换；还覆盖 Apply 报错时/成功后/清理 CAS 内/Restore 读取与 CAS 之间发生的外部修改、带非空 Raw 的崩溃重启恢复、恢复再次中断、读回/Prepare/CAS 缓冲区别名、无原子合同/运行时不支持的后端拒绝，以及非法读回/路径拒绝。它们不是 GPS 系统验收。额外尝试 `go test -race` 因当前环境未启用 cgo 而未运行，未为此修改工具链或系统设置。

## 4. 未完成边界

- 尚无真实 WinRT helper、LAF 授权、MSIX/runFullTrust 交付或 Windows VPS 定位往返证据。
- 快照精确相等用于稳定配置；系统采样必须另建含时间、来源和误差的观测模型。恢复普通定位是解除 override，不能要求移动中的物理位置回到旧坐标。
- 目前没有经过实际 Windows 验证的 AtomicBackend。WinRT Set/Clear 绑定本身并不证明 OS 条件替换/崩溃恢复保证；真实 helper 若不能满足该合同，就必须报告原子事务不可用。接口声明和 fake 测试不能证明真实平台原子性。
- Windows DACL、父目录重解析与组件哈希检查到执行之间的替换防护待完成。Go 的 Chmod(0600) 不证明 Windows ACL 收紧；备份元数据匹配也不是密码学防篡改认证。
- 没有已构建、审计并固定 SHA-256 的定位 helper 二进制，固定源码不能冒充二进制签名/哈希。
- Windows 系统定位、浏览器 CDP、IP/GeoIP、Wi-Fi/BSSID、账号历史、自建定位及硬件直连 GPS 的覆盖范围不同，UI 不能宣称它们统一受本适配器保护。
