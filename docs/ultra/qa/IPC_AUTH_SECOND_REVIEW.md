# C01/C05：IPC 认证与未知状态独立二审

日期：2026-10-19。只读审阅 Ultra 的主 IPC 认证、服务查询、GUI/托盘集成与现有 fake/DOM 测试；本二审没有修改这些应用源码。C02 路径修复是另一项已授权任务，记录见 [SERVICE_STORE_TRUST_FIX.md](SERVICE_STORE_TRUST_FIX.md)。本记录不替代 [IPC_AUTH_FIX.md](IPC_AUTH_FIX.md) 或实机验收。

## 结论

本轮有限审阅未发现新的、已确认的传输认证绕过。发现并反馈的四处调用方问题已由实现者修正，再读代码与定向 fake/DOM 回归均通过。普通非提权 GUI 是否能在目标 Windows 上读取 SYSTEM 服务进程/令牌仍未实测；不能把这些合成结果称为正式客户端验收通过。

没有运行真实服务、UAC、产品命名管道、网络、账户切换、ACL 修改或用户信息采集。本轮未运行包含当前账户读取/真实临时代码句柄的 `TestForegroundImagePinsOnlyTemporaryCodeAndAncestors`，也未将他人的该测试结果记为本轮执行。

## 已核对边界

| 边界 | 代码位置 | 结果与证据范围 |
|---|---|---|
| 认证先于请求 | `internal/ipc/auth.go:47`、`server.go:160` | 先 authenticated dial，再序列化参数；初始失败不进入参数 Marshal。fake 确认零 Write、连接与 peer 关闭一次。 |
| 真正发送前再次核对 | `internal/ipc/auth.go:27`、`auth_windows.go:94` | 发送方法/参数前校验当前管道 PID、持有进程活性、令牌/映像与 SCM；失败不发请求，不自动重试。发送前变化的 fake 通过。 |
| 安装服务分支 | `internal/ipc/auth_windows.go:163`、`:215` | 要求运行中独占服务 PID 相同、SYSTEM 且实际提升、实际映像与固定 SCM 命令匹配；SCM 读不到、启动/停止中、错误 PID/共享宿主均拒绝。源码与合成身份矩阵确认。 |
| 前台分支 | `internal/ipc/auth_windows.go:45`、`:163` | 仅 SCM 明确不存在或 Stopped/PID=0 才进入；仍要求真实提升的管理员/SYSTEM、客户端固定同目录服务文件。普通/deny-only 管理员不能进入。 |
| 前台代码来源与替换 | `internal/ipc/auth_code_windows.go:23`、`:86` | 拒绝 UNC/设备/ADS，逐级检查实际句柄属性、owner/写 ACL；祖先禁止删除共享，代码禁止写/删除共享。合成 ACL/路径规则通过；没有在本轮实际改目录或尝试真实替换。 |
| PID 与句柄 | `internal/ipc/auth_windows.go:45`、`:94` | 持有进程对象并在 Write 前再取管道 PID，降低 PID 重用/退出后误认风险；前台还持有祖先与代码句柄。真实进程退出、重启与查询间竞争仍待隔离测试，未声明完全消除所有竞态。 |
| 响应 ID | `internal/ipc/server.go:197` | ID 检查先于 OK/error/result 接纳；错误 ID 不修改结果，返回“操作结果未知”，没有自动重发。四种成功/失败与正确/错误 ID 组合 fake 通过。 |
| 身份未知与响应未知 | `cmd/godusevpn/service_identity.go:20` | 两类均为 `identity-unknown`/状态未知并保留历史配置，但分别用 identity/response hint；后者不说“未发送”。未知不被当作已断线或已撤保护。 |
| 退出与修复 | `service_identity.go:75`、`app.go:451`、`:758` | 未确认/无权限/不可达时只关闭 UI；断开失败后不继续 StopUser。修复前 fresh read；未知身份、响应不匹配、权限拒绝均阻止修复，NoService 还要 SCM 确认停止/不存在。 |
| 显式恢复网络 | `service_identity.go:55`、`tray_windows.go:123` | 不再只依赖旧 UI 快照。fresh read 成功才走在线设置；只有 NoService 加 fresh SCM stopped/not-installed 才走离线恢复。认证、响应、权限、其他错误及 SCM 运行/启动/停止中/未知都不清闸。 |
| UI 展示 | `web/dist/app.js:149`、`:343`、`:591`、`:1463`、`:1587` | 未知时隐藏修复/登记入口、旧 guard 保护胶囊，主按钮显示状态未知；关于页说明退出仅关 UI，托盘禁用连接/模式/恢复入口。中英身份/响应提示与无自动写操作已由 DOM mock 核对。 |

前台模式的明确前提仍是：用户已以管理员运行受信产品文件，允许该**实际提权管理员本人 SID**拥有/修改其个人代码路径；不防该管理员自身恶意，也没有发布者签名验证。安装分支以 SCM 的 SYSTEM 当前实例为信任根，安装目录来源由独立安装器负责。不能将本地 basename、相同路径或可读管道单独作为身份凭据。

## 二审发现并关闭的调用方问题

1. **SCM 查询拒绝被误报为未安装。** 原 `QueryStatus` 将所有 OpenService 错误返回 not-installed，可能让无权限/不可查询状态露出修复入口。现在 `internal/svc/service_windows.go:346` 只将 ERROR_SERVICE_DOES_NOT_EXIST 判缺席，其余 unknown；合成 access denied/invalid handle/marked-for-delete 等分支通过。
2. **C05 响应未知漏接 GUI。** 最初只识别 ErrUntrustedService，响应 ID 不符仍会走 disconnected/down。现在 `service_identity.go:20`、`app.go:305` 同时识别 ErrResponseMismatch，使用“操作结果未知、勿重复提交”文案，禁止未知状态的服务副作用；fake 与中英 DOM 验证通过。
3. **恢复网络只看旧快照/宽泛 NoService。** 最初旧 `service=false` 直接选择离线 clear；新增 fresh read 后，仍须处理 NoService 可能来自 timeout/busy，而非确认服务不存在。现在 `service_identity.go:55` 增加 fresh SCM 门槛，未知/运行/转换中不清闸；对应分支与 SCM 查询次数的 fake 通过。UI 的 NoService+SCM 不明确情况也映为未知，RepairService fresh read 同样门控。
4. **Wails 错误桥接丢掉 sentinel，离线恢复反而失效。** 初次集成的 `a.call` 把 ErrNoService 改成普通 `errors.New("SERVICE_DOWN")`，导致恢复路由的 errors.Is 永远不命中；helper fake 直接给 sentinel 没覆盖此桥接。现在 `serviceDownError` 保持精确前端文本并以 Unwrap 保留 ErrNoService，新增 `wails_absent_bridge` fake，通过后复读实际 tray 调用链确认。

这些条目是本轮开发中发现并已修正的分支/回归，不是新增四个未关闭漏洞，也没有真实恢复/停止服务的演练证据。

## 本轮实际执行

- IPC 八项定向测试：认证工厂/初验/发送前失败、重检与关闭、响应 ID、安装/前台身份策略、合成 ACL、路径拒绝、内存 roundtrip、SDDL 控制用户名单；全部通过。
- `go vet ./internal/ipc` 通过。
- GUI 四项定向测试：未知历史状态与修复阻止、退出动作、恢复路由及 SERVICE_DOWN 桥接、服务缺席无法确认时禁止修复；全部通过。测试动作是注入计数回调，不调用 SCM 或 Wails 系统接口。
- `go test ./internal/svc -run '^TestServiceLookupDoesNotMistakeAccessDeniedForAbsent$' -count=1` 通过，只有合成错误分类。
- `ipc-identity-ui.cjs` 在 jsdom 中通过；使用 `outside-only` 执行本地脚本，Wails 是 mock，无网络/真实浏览器/系统服务操作。回执为 identityUnknown、responseResultUnknown、staleProtectionHidden、noAutomaticMutations。

没有把整个包所有测试的结果外推为本轮执行，也没有使用真实订阅或机器标识。

## 仍需隔离 Windows 验收

1. **普通 GUI 访问 SYSTEM 身份的兼容性是发布前必测项。** `readPeerProcess` 申请 PROCESS_QUERY_LIMITED_INFORMATION、SYNCHRONIZE、TOKEN_QUERY，读取用户/提升状态/组和映像。目标进程/令牌 DACL、受管系统策略或安全软件可能拒绝；当前策略会安全失败并显示未知，但客户端可能因此无法控制 VPN。本轮没有证明标准用户或 UAC filtered 管理员都能通过。不能通过提权整个 GUI 或无依据放宽进程/令牌 ACL 来“修复”。
2. 正确 SCM/前台实例、SCM 不可查/转换中、进程退出/重启/PID 复用、代码文件与祖先替换/重解析、管道抢占及授权/未授权控制用户，需要真实 Windows 低权限矩阵；仅使用合成订阅。
3. 真实托盘/窗口的旧快照与点击竞态、UAC 取消、SCM fresh query 后状态变化，以及不确定结果时现有 guard/网卡状态保留，需要已有恢复基线后验证。源码和计数回调不会证明真实 WFP/隧道流量已通过。

按 [VPS 测试运行手册](../VPS_TEST_RUNBOOK.zh-CN.md) 控制后续范围。本轮没有因此扩大授权或开始实机操作。
