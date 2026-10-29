# Windows 系统位置 provider 的可实施范围

日期：2026-10-26。对应 H19（真实系统位置组件）与 H32/H33（成熟上游实际能力/许可审查）。这是下一源码批的研究和纯校验实现，不改变 qa.9 发布包；当前仍未接通真实 provider、未读取本机位置、未安装驱动或修改服务/策略。既有固定源码审计见 [GPS_UPSTREAM_REVIEW_2026-10-26.md](GPS_UPSTREAM_REVIEW_2026-10-26.md)。

## 当前接线与真实缺口

`internal/locationadapter` 现有 Apply/Restore 只面向经过核验的 `AtomicBackend`；没有系统后端。`internal/privacyhost/protocol.go` 只接受固定的机器隐私策略、防火墙、维护及 BrowserGuard 方法，没有任何 GPS Set/Read/Clear。`internal/privacy/catalog.go` 中的位置组件仍为 planned。浏览器拒绝定位权限、网页 CDP 位置覆盖和该机器策略协议均不能证明 Windows Location Services 被改变。

正确的免驱动系统路线是第一方常驻 WinRT helper。官方 [`GeolocationProvider`](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider?view=winrt-26100) 面向 Windows geolocation 服务消费者，要求目标用户上下文和持续 provider 对象；文档列出 build 23504 / UniversalApiContract 15，发布时必须检测运行时类型/成员及目标 build，不能承诺所有 Windows 10/11。其他程序或系统可终止 override，必须监控状态并停止，不自动抢回。

这不需要为了调用 API 自写内核驱动，但有不能用代码伪造的外部条件：

- [`SetOverridePosition`](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider.setoverrideposition?view=winrt-26100) 明确要求 `runFullTrust`，并属于 LAF。仅以管理员运行不会获得授权。
- [`LimitedAccessFeatures`](https://learn.microsoft.com/en-us/uwp/api/windows.applicationmodel.limitedaccessfeatures?view=winrt-26100) 要求 Microsoft 明确批准，并提供对应 feature ID 和 use token。仓库当前没有本产品获授的凭据；不得借用其他产品的 token，也不自动申请或把用户坐标上传申请。
- 可评估官方 [external-location identity package](https://learn.microsoft.com/en-us/windows/apps/desktop/modernize/grant-identity-to-nonpackaged-apps)：保留既有 Win32 安装器，另交付已签名、与固定 helper 身份/安装位置一致的 package，注册到目标用户。生产签名须在目标系统受信；身份包本身不代替 LAF 批准。这是待验证的交付方案，不是当前已打包或已获准的组件。
- 接口可使用已审计的 Microsoft `windows-rs` WinRT 绑定（既有报告的固定 SHA 与 MIT/Apache-2.0 文本）；它只是 SDK 绑定，不能代替产品 helper、批准及实际验收。

## 现有事务不能直接套用 Set/Clear

公开 [Set](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider.setoverrideposition?view=winrt-26100) 返回状态，[Clear](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider.clearoverrideposition?view=winrt-26100) 没有 expected-generation 参数。文档没有给出能跨进程重启持久验证的所有权令牌或 compare-and-swap 操作。因此不能据此断言满足现有 `AtomicBackend` 的 durable CAS；本地 mutex、IsOverridden 布尔、重复读回与配置缓存都不能补成 OS 原子合同。

可行的后续最小产品范围是独立的临时会话：普通用户 helper 在同一实例内持有 provider，不接管外部占用，不承诺跨崩溃恢复别人的 override。若要采用这一不同于现有事务的模型，需先明确产品语义，并验证对象销毁/进程异常退出/其他程序 Clear 的实际系统行为；不能悄悄降低现有 Apply/Restore 保证。当前 `CheckAtomic` 继续拒绝不满足合同的后端。

有授权后的 helper 应采用固定安装路径/依赖哈希、父进程/用户验证、匿名管道和有界固定协议；不开放程序路径、任意参数或任意 RPC。provider 运行在目标普通用户会话，不能放到当前 SYSTEM/提升机器策略 helper 中假冒用户位置。管道 EOF/过期租约、权限撤销、外部终止或读回失败应结束本次会话；不能为恢复方便清除未知新会话。坐标与 LAF token 不进入日志、一般报告、诊断或子进程命令行。

## 已完成：独立观测的纯证据校验

`internal/locationadapter/observation.go` 新增 `SystemObservation`、固定 Windows `PositionSource` 数值、受限 `ObservationPolicy` 和 `CheckOverrideObservation`。没有系统调用、UI、网络、进程执行或隐私设置修改。

真实 reader 之后应调用独立 `Geolocator.GetGeopositionAsync(maximumAge, timeout)`，由系统对象读取每个字段，不能回显 Set 参数。官方明确 [maximumAge 仍可能返回更旧的缓存](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.geolocator.getgeopositionasync?view=winrt-26100)，所以新校验同时要求采样时间不早于本次成功 Set/update，并限制年龄及未来偏差。读取权限失败/取消/无数据均停止验收；不自动打开定位设置、不用 IP 查询补坐标。需要用户同意的前台访问须遵循同页 RequestAccessAsync 要求，不能由后台 SYSTEM 代答。

校验还要求真实已读取的来源、remote 标志、有限合法坐标和正精度，拒绝 Unknown/Default/Obfuscated 及未知枚举。[IsRemoteSource](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.geocoordinate.isremotesource?view=winrt-26100) 只说明远程重定向，不说明是哪一个 provider 拥有它；[Timestamp](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.geocoordinate.timestamp?view=winrt-26100) 是系统确定位置的时间。不能从请求或本地缓存推导这些字段。

默认限制是 10 秒最大年龄、1 秒未来时钟偏差、100 米报告精度上限及 5 米独立二维距离；报告精度还不能比本次请求更粗。距离使用保守球面上界，处理反经线/极点；不检查海拔，不是精密测地/物理位置保证。报告精度不会扩大允许距离。

成功只返回 `observation_matches_request` 与 `matchesRequestedPosition=true`，`applied=false`、`providerOwnershipVerified=false`；不复制坐标、时间或距离。`SystemObservation` 默认 JSON 为 `{}`，默认 String/GoString 为固定脱敏值。该纯函数仍信任调用方传入的系统证据，不认证 OS reader，不证明 provider 身份，也不改变现有 Apply 的门禁。

纯测试覆盖旧匹配缓存、年龄/未来边界、缺失来源/remote 标志、来源不符、粗精度/非法浮点、位置不符、反经线/极点/对跖点、独立距离阈值、宽精度不能扩大半径，以及默认序列化/格式化不泄露。`go test ./internal/locationadapter -count=1` 与 `go vet ./internal/locationadapter` 通过；使用合成坐标、fake 后端与测试 TEMP 文件，未调用真实定位。

## 驱动替代与隔离 VM 验收

Microsoft GNSS UMDF 官方样例是真实设备接口/IOCTL 基础，但固定源码仍有示例坐标，需开发动态输入接口、访问控制、WDK 构建、正式驱动签名/分发和受支持 OS 矩阵。样例 MS-PL 与现有 MIT/Apache SDK 路线不同，不能默默加入包或关闭驱动签名/安全设置使其工作；固定源码与许可证据沿用上游审计。Device Portal 只可在已经显式配置的隔离开发 VM 上研究，不应作为终端用户生产依赖。

真实接入验收需要：目标普通用户运行；已核实 LAF/身份/运行时 API；未授权/外部占用时无改动；独立 Windows reader 的新鲜匹配；外部终止与租约关闭；Clear/崩溃后的 override 生命周期；定位服务关闭和权限拒绝；坐标不进入日志。恢复只证明自有 override 释放，不能要求移动中的物理位置回到原坐标。Windows 服务、浏览器位置、GeoIP、Wi-Fi、账号历史与直接硬件 GPS 是不同通道，不能据本模块声称全局位置不可追踪。
