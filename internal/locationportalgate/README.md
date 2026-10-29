# Device Portal 固定入站前置限制

本包只负责 Windows Device Portal 的 TCP 58080 / 58443 入站限制，不安装 FoD、设置凭据、启动服务或修改 VPN。`locationportalprepare` 负责在持久备份和端口/原状态核对后调用本包；原 Portal 已运行、固定端口被他人占用或缺少可确认的准备记录时，不应借本包接管。

`NewGate()` 仅构造；`Ensure(ctx)` 事务安装并读回后才返回 `filters_verified` / `verified:true` / `loopbackGateVerified:true`。`Inspect` 与 `Remove` 的确定无规则终态是 `absent_verified` / `verified:true` / `installed:false`，**不代表允许启动 Portal**。任何未知都是 `unknown` / `verified:false`；禁止只看 `installed:false` 推断已撤干净。

## 实际 native 行为

- 独立 provider/sublayer 与四个固定 key，和 VPN guard、应用规则、浏览器 guard 不共用命名空间。
- 每个 IPv4/IPv6 ALE_AUTH_RECV_ACCEPT layer 各阻断两个固定端口的 TCP 非回环流量。条件只含 IP_PROTOCOL=TCP、IP_LOCAL_PORT=固定值、FLAGS_NONE_SET(IS_LOOPBACK)。不按程序 PID，因此不漏过 HTTP.sys 代理监听。
- 持久 provider/sublayer/filter，私有 SYSTEM/Administrators DACL。规则为硬 block，没有 permit 或清除 action right 的软化 flag；sublayer 权重 65535。读回只允许 BFE 的 INDEXED 附加标志，不允许 disabled/boot-time/其他未知标志。
- provider 精确关联既有 `godusevpn-ultra` 服务，不新增服务。每次 native 打开先读取固定 ProgramFiles 安装位置，锁定祖先和服务 EXE；拒绝重解析点、多硬链、普通主体可改的代码或安装目录，并要求安装根受保护 DACL。SCM 必须为 AutoStart、独立进程、canonical LocalSystem，命令只能是固定服务 EXE 加 `service`。每次规则快照重新核对服务配置；无需服务当前 Running，绝不为满足 gate 改它的启动类型。未正式安装、外部禁用或身份未知时拒绝本次 Portal 操作。
- 每次验证都精确核 provider、sublayer、metadata、key、layer、action、权重、三条件与类型。枚举所有 layer，仅读本模块 namespace并拒绝额外/重复/损坏对象；枚举有界 65,536 条，超过时明确未知，不继续启动。快照在显式 WFP 事务内，不能拿过时 enum 当 live 查询。
- `Ensure` 未齐、取消、提交或 fresh 读回失败不会自动撤掉可能已存在的保护，也不会启动服务。
- `Remove` 不接受“服务已停”输入。它自身 fresh 查询固定 `WebManagement` 的 SCM 状态：必须 absent，或同时为 Disabled、Stopped、PID=0；事务内撤前/撤后及提交后再次检查。只删自己的完整规则及空的自有 base，外来/部分/未知对象保留。若已验证保护可能被不确定提交撤走，会尽力重新安装同一固定集合并返回错误，绝不把它当恢复成功或重启服务的依据。

普通进程不能通过这个包自由输入端口、provider、filter、flags、地址或服务名。固定 helper 才能调用生产 opener；它要求实际提升权限并对所有动态入口先 Find。SCM 只读，本包不更改服务启动类型。全局 HTTP.sys IPListen、系统防火墙开关和其他模块规则完全不在本包接口中。

Portal 恢复及 gate 撤销必须在移除 Ultra 服务之前完成。服务代码核对证明固定安装命令和持有文件的本地身份/权限，不是发行者签名验证；有权限同时修改受保护安装树或 SCM 的其他管理员不在互斥保证中。

## 依据与边界

微软定义 [IS_LOOPBACK 可用于 ALE_AUTH_RECV_ACCEPT](https://learn.microsoft.com/en-us/windows/win32/fwp/filtering-condition-flags-)，[FWP_MATCH_FLAGS_NONE_SET](https://learn.microsoft.com/en-us/windows/win32/api/fwptypes/ne-fwptypes-fwp_match_type) 用于无对应标志的流量。[Filter arbitration](https://learn.microsoft.com/en-us/windows/win32/fwp/filter-arbitration) 区分硬/软动作；本包不设置可软化固定 block 的标志。[FwpmFilterCreateEnumHandle0](https://learn.microsoft.com/en-us/windows/win32/api/fwpmu/nf-fwpmu-fwpmfiltercreateenumhandle0) 明确 nil template 枚举全部，并要求在同一显式事务内才能确认当前快照。

微软 [FWPM_FILTER0](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_filter0) 对 disabled 标志说明：“A provider's filters are disabled when the BFE starts if the provider has no associated Windows service name, or if the associated service is not set to auto-start.” 因此不能只靠 persistent 标志或空 serviceName 推断重启后有效。本包以固定服务关联、实际 AutoStart 配置和无 disabled 的 fresh 规则读回作为配置前提；它仍不能替代 BFE 重启或系统重启后的实际 VM 网络验证。

这不是让服务只绑定回环地址；服务仍可能监听 wildcard，本包限制新非回环 TCP 接入。不能阻止其他本机进程用 loopback 访问，所以 HTTPS、真实证书 pin、认证和 CSRF 仍是 consumer 的独立必要条件。没有声称隐藏 DNS-SD 广播或控制其他机器上的代理代发。管理员/BFE故障、重启及与其他高优先策略组合仍须 VM 验证；有能力重新配置 SCM/WFP 的其他管理员不在本模块原子互斥范围。

本批仅 Go 内存事务 fake 与合成 WFP/SCM/ABI 测试；没有调用 WFP/SCM、开启端口、运行 Portal/FoD或修改本机。真实 API 接口、规则有效性、LAN拒绝与loopback可用、重启/助手失联持久性、错误后的停止/恢复组合均待授权 VM；`loopbackGateVerified` 只说明已读回配置，不是本批真实流量 PASS。
