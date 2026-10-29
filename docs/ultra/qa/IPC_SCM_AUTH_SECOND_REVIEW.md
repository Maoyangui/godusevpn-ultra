# C01 已安装服务 SCM 认证修订二审

日期：2026-10-19。只读审阅 `internal/ipc/auth_windows.go` 与 Tests。源码 SHA256 `db8f97c7c4405429dc67dba1d1baff9d2df79e3d32d24b9dcb6b85b8ac8d9726`；测试 `2c05009030fd4c797aab0ab821a7feac673711ebcab658bafc07a801638e202d`。未修改源码、实际服务/管道/peer token 或系统 ACL。

结论：本轮未发现确认阻塞。已安装分支通过 SCM 当前实例和持有的实际进程 image 认证，移除普通用户读取另一个账户 primary token 的前提；这是配置/实例证据，不是已读取 runtime SYSTEM SID 的证据。

静态核对：

- `authenticateWindowsPeer` 使用实际 `windowsPeerReader()`；该 reader 进入生产 `identity`，先读 fresh SCM，再判断模式，再读 held process image。成功的 installed 分支在 `r.token` 调用前直接返回；没有另一条隐含 OpenProcessToken 路径。`Validate` 同样复用该真实分支并在发送前复核。
- SCM 不可读、非 running、PID0/不匹配、非 OwnProcess、非 canonical LocalSystem 或进程 image 不一致均拒绝。只有明确 absent/stopped PID0 可选择原 foreground 模式；已建立 session 的模式变化先于 token 查询被拒绝，不以未知降级。
- LocalSystem 仅允许 ASCII case variants。固定字节长度与 EqualFold 联合排除了 `LocalSyſtem` 等 Unicode fold lookalike，以及机器/域前缀、空白和其他账号；没有将 `NT AUTHORITY\SYSTEM` 或 `.\LocalSystem` 猜成合法别名。
- SCM command 使用真实 Windows command-line parser，必须恰好 image 和大小写精确的 `service` 两参数；额外参数、run、SERVICE 均拒绝。空/相对/UNC/设备/ADS image 不能通过后续本地绝对路径与实际 image 比对。
- pipe PID 与 held process object 仍绑定，alive 在验证两端检查；SCM/PID/image/mode变化拒绝。foreground 保留提升 token、管理员有效组、SID、固定产品 sibling 与原代码 pin，未放宽其检查。

实际运行 `go test ./internal/ipc -count=1` 与 `go vet ./internal/ipc`，均通过。新增 fake 测试调用的正是生产 identity/session reader：installed tokenErr=AccessDenied 而 tokenReads=0，连续重新验证 fresh SCM/image；17 类变更、不可读/模式迁移、foreground token 失败与原代码 Temp pin 回归通过。它们不是读取实际已安装服务或 peer token 的实机证据。

信任边界：SCM 配置与当前 PID 是 installed 分支的权威；安装树 owner/ACL 由原安装流程负责，本修订不声称新加了 installed image 的文件 hash/pin，也不抵御有权更改 SCM/安装树的恶意管理员。服务运行中被管理员改账号配置时，配置不等于进程原始 token；源码已明确这种证据区别，不能称观察到了真实 runtime SID。

仍需新候选验收普通真实 medium 用户的 pipe、SCM_QUERY_CONFIG/STATUS 与 PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE 权限，以及完整 CLI MGetState 成功。旧 qa.2 的 token-query probe 只用于暴露旧兼容前提，不能当作本修订 CLI/GUI 已实测或借此关闭 C01。GUI/UAC 交互、替代管理员和进程生命周期继续单列。
