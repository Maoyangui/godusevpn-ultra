# C01 / C05 主 IPC 身份边界修复记录

日期：2026-10-19。修改范围为 Ultra 副本的 `internal/ipc`、相关 GUI/托盘/文案、服务只读状态查询及本记录；原版目录未操作。本记录不修改总审计，也不把 fake/本地临时文件测试算成 Windows 服务实机验收。

## 行为

- Windows 控制连接取得真正的管道服务端 PID，打开并持续持有目标进程句柄；发送任何方法或参数前核对进程活性、令牌用户、实际提升状态、启用的管理员组、实际映像和 SCM 实例。
- 已安装服务模式要求：SCM 当前运行的独占服务 PID 与连接匹配、真实 SYSTEM 令牌且已提升、实际进程映像与 SCM 固定 `"..." service` 配置一致。查询 SCM、进程或 TOKEN_QUERY 失败即拒绝，不转入前台兼容路径。服务启动中/停止中、另一 PID 或共享宿主同样拒绝。
- SCM 明确没有该服务，或明确 Stopped/PID=0，才允许管理员显式前台 `godusevpn-svc.exe run` 的兼容路径。它仍必须拥有真实提升的管理员/SYSTEM 令牌，映像来自客户端固定同目录的本产品服务文件；这个受信任程序只有 `run` 分支能在 SCM 外承载主控制管道。
- 前台代码路径必须为本地固定/可移动盘，拒绝 UNC、设备路径、ADS 和重解析。逐级检查并持有卷根、祖先和文件句柄；不向其他账号开放危险写/删除/ACL 权限。可信任主体为 SYSTEM、Administrators、固定 TrustedInstaller 服务 SID，以及实际已经提升的前台管理员本人 SID。拒绝别的账号拥有/替换代码或祖先；仅允许创建不同兄弟目录的权限不等于可替换已存在、已持有的祖先。
- 持有代码文件时禁止写/删除共享，持有祖先时禁止删除共享；这不是依赖可伪造文件名或重新计算一份本地二进制 hash 作为认证。没有全二进制 hash 或签名验证的额外开销。安装模式的信任根是 SCM 服务实例和 SYSTEM 身份，原受保护安装器继续负责代码来源/ACL。
- 开始时认证一次，真正发送前再次核对当前管道 PID、持有的进程活性、账号/映像与当前 SCM 实例。进程对象句柄避免 PID 重用期间误把新进程当旧实例；退出/切换/读不到一律不发请求，不重试写入。
- `ErrUntrustedService` 单独表达“身份无法确认、未发送请求”，不被 `Call` 转成 `ErrNoService` 或 `ErrNoPermission`；不会调用隧道、guard、安装、恢复或任何远程命令。原权限拒绝的登记流程仍与身份未知分开。
- 响应 `ID` 必须等于本次请求，校验先于 OK/error/result 接纳。错误 ID 返回 `ErrResponseMismatch`，提示操作结果未知，不能因响应异常重试已经提交的操作。每次 Call 独立连接的协议结构保留。

## 信任前提和剩余限制

1. 不防御已经掌握管理员/SYSTEM 权限的攻击者，也不宣称能从本地 unsigned 二进制建立发布者签名信任。前台模式仍依赖用户明确以管理员运行受信任产品文件；具名管理员本人对其个人代码目录的控制是该模式明确接受的信任前提。另一普通账号能写代码/祖先时必须拒绝。
2. 普通 GUI 对 SYSTEM 进程的 `PROCESS_QUERY_LIMITED_INFORMATION`、`SYNCHRONIZE`、`TOKEN_QUERY` 以及服务查询的实际可访问性尚未通过 VPS/低权限用户矩阵验证。查询被系统策略拒绝时会明确认证失败，不能猜测通过；没有自动提权整个 GUI 或放宽系统 ACL 的补救。
3. 真实 SCM 服务重启、PID 退出/复用、替代管理员前台运行、低权限管道抢占、旧版本配置及受管机器兼容性仍需隔离 Windows 实测。源码与合成测试只证明分支/拒绝边界，不能冒充这些环境已经通过。
4. 当前开发机的 `C:\Users` 有 Authenticated Users 的有效 Modify 权限，完整前台路径检查因此拒绝该祖先。没有为通过测试修改主机 ACL 或放宽生产判断。文件句柄测试单独注入仅测试的 ACL 检查函数，验证真实临时文件的防写/祖先防重命名与释放；ACL 政策另由合成 SDDL 测试验证。该句柄测试不是完整前台认证成功的证据。
5. GUI 集成已完成下述 mock/contract 层级，普通 GUI 读取 SYSTEM token、真实 SCM 服务和托盘交互仍待 VPS 验证。完整产品验收不能只凭 transport/DOM 测试关闭。

## GUI / 恢复入口集成

- `serviceReadState` 将身份认证失败、响应 ID 不匹配分别记录 identity/response hint，共用 `identity-unknown` 的安全门控。历史设置、连接意愿和 guard 数据保留，但客户端连接状态为 unknown；不会写 daemon 状态、guard、网卡或设置。没有把前次的连接/保护当作当前已验证。
- 仅控制传输连接失败也不能证明服务不存在。刷新时若 SCM 不明确 stopped/not-installed，则同样标未知，隐藏修复/登记。`svc.QueryStatus` 的 OpenService 错误只有 ERROR_SERVICE_DOES_NOT_EXIST 才返回 not-installed；AccessDenied、标记删除等返回 unknown。
- 首页/抽屉/关于/托盘区别“身份未确认”和“响应未确认”。响应不匹配发生在发送后，说明操作结果未知并要求重新核对，绝不使用“请求未发送”的身份失败文案。旧 guard 胶囊隐藏，主按钮显示状态未知；关于页修复按钮和退出说明随动态状态更新。
- 身份或结果未知时，RepairService 在任何 SCM/提权动作前拒绝；既有可读快照也先做新读取，遇到身份/响应/权限异常或 NoService+SCM不明确缺席就拒绝。未对未知结果自动重试写入。
- QuitApp 对未知、无权限或不可达服务只关闭当前界面。已验证服务的 Disconnect 如失败，保留服务，不能继续 StopUser。停止的是本地流量观察，不是原 VPN；未知时保留会话累计值，忽略已取消观察器的迟到更新。
- 托盘“恢复网络”在身份/结果未知时禁用且回调再检查。正常点击也重新读取服务，而非仅信任旧缓存；只有新读取为 ErrNoService **且**新 SCM 查询明确 stopped/not-installed，才进入原本的用户主动离线恢复路径。timeout/busy/SCM unknown/running/starting/stopping 不能据此离线 clear。已验证在线服务仍走原有设置更新，并由认证 IPC 再核对每次操作；没有更改 guard 清理算法。
- `serviceDownError` 保留精确 `SERVICE_DOWN` 前端代码，并用 Unwrap 保留 ErrNoService 供上述判定，避免字符串桥接丢失身份。fake 覆盖该真实桥接形态。仍不承诺恢复入口与所有其他管理员操作之间已有全局维护锁；全局并发维护窗口属于 A07。

## 无副作用验证

- `go test ./internal/ipc -count=1` 通过。
- `go test ./internal/ipc ./internal/cli ./cmd/godusevpn ./cmd/godusevpn-cli ./cmd/godusevpn-svc -count=1` 通过（无测试的命令包完成编译）。并行 paths/privacy 文件未落地期间曾短暂编译失败，文件落地后已重新运行上述命令通过，没有拿瞬态失败结果当通过。
- `go vet ./internal/ipc`、`git diff --check -- internal/ipc` 通过。
- 原协议 roundtrip 改用 `net.Pipe`，不创建产品真实命名管道。验证成功、结构化错误、panic、未知方法和断开路径。
- 新 fake 验证初始身份失败、初次验证失败、发送前身份变化均零 Write，关闭连接/目标句柄一次，且错误不被改为服务缺席；身份失败先于参数序列化，发送前再核对不被省略。
- 正确/错误 response ID 与 success/error 组合验证；错误 ID 不接纳任何结果或错误对象。
- 合成身份矩阵覆盖当前 SCM SYSTEM、其他管理员冒充已安装实例、错 PID、启动/停止不回退、共享宿主拒绝、SCM 缺席/停止时真实提升管理员前台允许、普通/deny-only 管理员拒绝。
- 合成 SDDL 覆盖安全主体、公共只读、他人可写/owner、目录删除子项、仅创建兄弟目录、文件 append、inherit-only 模板与 TrustedInstaller 卷根 owner。
- UNC/设备/相对/ADS 路径在代码文件探测前拒绝；真实文件测试只创建一个非可执行内容的临时 `.exe` 样本，未执行它，未改真实产品数据或主机权限。
- 后续 `go test ./cmd/godusevpn ./internal/svc ./internal/ipc ./web -count=1` 再次通过；GUI fake 覆盖未知历史保留、Repair前置拒绝、退出/Disconnect失败不停止、离线恢复的新读取+SCM矩阵；服务查询 fake 覆盖仅1060等同不存在。
- `node docs/ultra/qa/ipc-identity-ui.cjs <jsdom模块路径>` 通过：down→身份未知→响应未知→恢复的中英 DOM 状态、无修复/登记按钮、主按钮不连/断、旧保护胶囊隐藏、关于页动态文案，以及无自动 Connect/Disconnect/Repair/Register/Save/Quit。脚本只用固定 mock，没有网络或真实 Wails。

没有运行真实服务、UAC、安装/卸载、注册表、WFP、隧道或网络测试，没有读取/发送真实订阅、设备信息或凭据。

接口依据：[GetNamedPipeServerProcessId](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeserverprocessid) 只提供 PID，因此还需本修复的持有进程对象、令牌和 SCM 绑定；PID 或路径字符串本身不能代替认证。
