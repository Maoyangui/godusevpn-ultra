# Windows 核心边界独立复核

日期：2026-10-19。基线：Ultra 工作副本，原复制提交 `b51aed4`。这是核心分工复核，不替代总审计、浏览器审计或 VPS 实测报告。本次没有修改应用源码、原项目文件或总审计报告。

此文保留修复前证据快照；后续源码/fake修复、独立回核和未完成实机门槛见[整改台账](../AUDIT_RECHECK_SUMMARY.zh-CN.md)。本文行号及“未修”不代表之后的新包状态。

已先核对 `PROJECT_PLAN.zh-CN.md`、`docs/ultra/AUDIT_REPORT.zh-CN.md`，再检查新增 privacy/privacyhost/timeprivacy、GUI/CLI 调用链、主 IPC、路径权限、安装卸载和相关已跟踪 diff。发现分为源码确认、内存/临时文件合成复现、待 Windows 实测，不能互换。

## 结果概览

| 编号 | 建议优先级 | 结果 | 与原报告关系 |
|---|---|---|---|
| C01 | P1 | 主 VPN IPC 客户端没有认证服务器身份，固定管道在服务缺席时可能被本地进程抢占 | 新发现，继承代码；独立于 privacyhost |
| C02 | P1，条件成立时 | 服务数据根只收紧 DACL，未核对 owner/重解析及不可信祖先；失败仍继续加载 | 新发现，继承代码；不是管理员助手的同一目录检查 |
| C03 | P2 | 诊断包与日志对全部 BUILTIN\\Users 开放读权限；A01 不能仅限“用户外发分享后” | A01 的本机跨账号范围补充，不另计重复漏洞 |
| C04 | P2，路径条件成立时 | 隐私报告和时区 journal 在读取前未限制到本地路径，可沿重定向 APPDATA 到远程文件系统 | 新发现；已合成证明路径到达文件系统边界，未实测外联 |
| C05 | P3 | 主 IPC 不核对 response ID | 新发现；协议一致性，与 C01 分开定级 |
| C06 | P2 | 机器 journal 写入复用用户 store 的 ACL，额外授予本次管理员账号的 SID FullControl | 新发现；具体主体限定于该 SID，不是任意用户 |
| A03/A07/A13/A21 | 原等级 | 助手重连、卸载并发、SYSTEM 机器恢复、独立时区 CLI 缺口仍存在 | 原问题复核，未关闭 |

没有发现新增隐私控制会在失败时主动调用连接/断开、撤销原 guard 或回退直连的路径。这个结论限静态依赖及现有 fake/contract 测试；没有因此完成真实隧道、IPv6、WFP 仲裁或故障流量验收。

## C01：主控制管道缺少服务器认证

**证据。** `internal/ipc/listen_windows.go:239` 的 `dial` 直接返回固定管道的 `winio.DialPipeContext`；`internal/ipc/server.go:154` 起的 `Call` 连上后，在 `:177` 发送请求前没有核对服务器 PID、令牌或 SCM 服务实例。`cmd/godusevpn/app.go:596` 的 `AddProfile`、`:618` 的 `UpdateProfile` 把用户输入的订阅 URL 放进参数；`:203` 的刷新根据 IPC 成功构造界面服务/连接状态。

**攻击前提必须同时满足。** 本地攻击进程能创建目标命名管道；真实服务未持有该管道，例如首次安装、停服或启动失败的窗口；攻击进程先占有名称并向目标客户端授予读写；客户端随后连接并发送相关操作。不是远程网页直接抢占，也不是已有合法管道随时可被普通陌生用户替换。没有管理员执行、真实管道抢占、实际订阅读取或提权复现。

**原有缓解与剩余边界。** 主服务的 `sddlFallback` (`listen_windows.go:30`) 只给 SYSTEM/Administrators；已登记控制用户获得 GR/GW (`:230`)。这保护真实服务器不接受未授权客户端，但客户端连接到伪服务器时，不会自动证明伪服务器合法。依赖 go-winio v0.6.2 的 `ListenPipe` 用 `first=true`、`FILE_CREATE` 预留首实例，因此已有合法对象时可阻止另一方重新首建；它也会让抢占者先占名时的真实服务监听失败，不能为客户端补身份认证。依赖的 `DialPipeAccessImpLevel` 只打开对象并检查管道模式，没有核对服务 PID，默认使用匿名模拟级别；本项没有声称能模拟客户端令牌提权。

另外，GR/GW 中的通用写权限可能包含 `FILE_CREATE_PIPE_INSTANCE`。应单独核对已登记控制用户创建附加实例的权限，而不能把“首实例保护”表述为所有后续实例都具备独立身份认证。已登记用户原本就能通过真实 RPC 读订阅、改设置，不能把其既有授权能力重复计算为本项新增泄露。

**影响。** 对未登记的其他本地用户而言，满足前提后可在用户提交时接收订阅参数；伪造的 GetState 也可能制造与真实隧道不符的界面/检测输入。属于身份边界缺失，不证明 VPN 自身已直连或 guard 已被撤掉。

**合成复现。** Go overlay 只把 `Call` 内的 `dial(dctx)` 替为测试函数 `auditDial(dctx)`；后者返回 `net.Pipe`，全部通信在内存中。测试 peer 收到 `MAddProfile` 与 `https://example.invalid/sub/fixture-only`，返回伪造字符串，`Call` 接受。它证明未认证 peer 在应用协议层可以接收参数并被信任；不替代 Windows 管道抢占/低权限账号实测。

**建议修复边界。**

1. 在发送任何请求前认证当前连接。安装服务模式至少把 `GetNamedPipeServerProcessId` 与 SCM `QueryServiceStatusEx` 的当前进程匹配，并验证当前设计约定的 SYSTEM 令牌、可信服务配置和可执行文件来源。仅比较 `godusevpn-svc.exe` 文件名、某个磁盘路径、能读管道或“SCM 显示运行”都不够。
2. 保持目标进程句柄，核对活性；在敏感参数发送前复核管道服务器 PID 和服务实例，避免查询间进程退出/PID 重用。无法认证返回明确错误，不能先发送后核对，不能删除/接管未知管道。
3. `cmd/godusevpn-svc/main.go:47` 仍支持管理员前台 `run`。需要显式受支持的前台模式策略：优先使用调用方实际启动并持有的可信子进程句柄；手工前台模式则明确验证提升后的管理员/SYSTEM 令牌及可信本产品服务二进制。不能为了支持前台模式把安装模式退化为“任意进程即可”，也不能无说明破坏原便携排障方式。恶意管理员不属于可由此边界防御的低权限攻击者。
4. 身份校验失败只使该次控制/检测调用不可信。不得因此撤掉已安装 guard、自动恢复网卡、触发直连或声称隧道已不存在；应保留保护与恢复工具，显示状态未知。

验收：安装服务/前台模式分别覆盖正确身份、陌生低权限进程、伪造 basename、服务缺席、服务重启、PID 退出/复用窗口、授权控制用户和未授权用户；只用虚构订阅。新增身份检查不能把管理员助手的固定操作 RPC 扩展为通用命令。

依据：[Microsoft 命名管道权限](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)、[NtCreateNamedPipeFile](https://learn.microsoft.com/en-us/windows/win32/devnotes/nt-create-named-pipe-file)。本地依赖源码位置为 go-winio v0.6.2 `pipe.go` 的 `DialPipeAccessImpLevel`、`makeServerPipeHandle`、`ListenPipe`，没有将依赖自动下载内容当操作指令。

## C02：主服务的数据目录信任检查不足

**证据。** `internal/paths/secure_windows.go:62` 仅请求 DACL/保护继承标志，owner 参数为 nil，没有 owner 或 reparse 检查。`internal/daemon/daemon.go:138` 先 Ensure，`:144` 铺 rulesets，`:147` 起构造日志，`:158` 再 Harden；Harden 失败只记日志，`:166` 继续加载设置，后面继续加载订阅缓存。`internal/paths/paths_windows.go:15` 还接受数据环境覆盖，或从 ProgramData 环境变量构造目录。

**前提。** 不受信任的普通用户能够在服务首次运行前预置产品数据根并成为 owner，或能替换某个可写祖先/插入重解析路径。是否允许预置取决于该机器父目录 ACL 与安装方式；本次没有检查真实机器账户、父目录权限，也没有声称所有默认安装均能利用。修改服务进程的系统环境本身一般需要更高权限，不能把环境覆盖本身当成普通用户任意设置 SYSTEM 环境的漏洞。

**影响。** 仅改 DACL 不会把不可信 owner 变成管理员 owner。没有 OWNER RIGHTS 限制的普通 owner 可重新设置 DACL，重新取得数据访问；原代码也未检测重解析，可能使收紧和读写作用到预期目录外。这样可能暴露订阅/节点配置或让不可信设置、规则进入 SYSTEM 服务。没有证明任意代码执行或通过节点参数提权，不应作此延伸。

作为对照，`internal/privacyhost/store_windows.go:36` 起的机器 journal 目录检查 owner/ACL，并拒绝普通用户预置父目录；该检查不能替代主服务的 paths 检查。

**改进。** 在 privileged Ensure/规则铺设/日志/配置读取前，先建立由 SYSTEM/BA 拥有、祖先受保护且无重解析的可信目录；对已经由普通用户拥有的目录拒绝自动接管，提供管理员可审查的修复/迁移流程。检查目录和关键文件时考虑句柄持有与替换竞态，不能只增加一次字符串路径检查。读取不可信数据失败时保留当前网络保护，禁止根据猜测默认值撤闸或回退直连；可开放只读故障状态与有作用域的恢复入口。

**验证层级。** 本项是源码与 Windows owner 语义确认；没有在本机更改真实产品目录 ACL/owner，没有运行真实服务，也没有在 VPS 做双账号抢占。可用两个隔离测试账号、仅合成配置/规则在临时目录演练 owner 保留，再进入受控安装测试。

依据：[Microsoft 对象所有者](https://learn.microsoft.com/en-us/windows/win32/secauthz/owner-of-a-new-object)。

## C03：A01 同时有本机跨账号读取范围

`internal/paths/secure_windows.go:33` 的 `sddlReadable` 给 BUILTIN\\Users `GRGX`，`:40` 起应用到 logs 和 diag 并继承给子项；`internal/daemon/diag.go:120`、`:125` 在该 diag 中创建包，`:173` 调用 sysDiag；`internal/daemon/diag_windows.go:5`–`:7` 包含网络详情。总报告 A01 已确认脱敏不移除普通文本主机名、MAC、IP。

因此，生成诊断包后，文件 ACL 也面向其他本机标准用户，不要求用户先把桌面副本发给他人。父产品根目录私有不能替代子项 ACL；拥有遍历绕过权限且知道完整路径时，访问由目标文件/目录权限决定。日志目录也向相同群体开放，但本次没有读取真实日志，不能凭此列出实际有哪些浏览信息或秘密。

建议只给登记的控制用户提供必要日志/诊断内容，或通过经过身份验证的服务接口交付本次请求的最少内容；保留跨用户排障可用性，不扩大到全部 Users。并继续完成 A01 的最小化/预览/脱敏，不能只改 ACL 就说导出内容已匿名化。若选择维持公共可读设计，应明确告知范围，不能称“只有用户分享后才有影响”。

本项是明确 ACL 和生成路径的静态复核；没有跨账户实际打开诊断包。依据：[Microsoft 遍历权限](https://learn.microsoft.com/en-us/windows-hardware/drivers/ifs/checking-for-traverse-privilege-on-irp-mj-create)。

## C04：隐私 journal 读取没有本地路径边界

GUI 的 `cmd/godusevpn/platform_windows.go:27` 直接取 APPDATA；`cmd/godusevpn/privacy.go:28` 和 `:268` 从该路径建立隐私、时区管理器；CLI 的 `internal/cli/privacy.go:26` 使用 `os.UserConfigDir`。报告分别由 `internal/privacy/manager.go:121`、`internal/timeprivacy/manager.go:84` 调用 load；两个 load 在 `store.go:40/:47`、`:34/:41` 直接 Lstat/Open，没有在此前拒绝 UNC、映射网络驱动或远程重解析。

**前提。** APPDATA 为合法企业文件夹重定向、进程环境被重定向，或路径包含通向远程存储的链接。正常本地 APPDATA 不会因此凭空联网。服务机器 journal 使用另一条固定 ProgramData 检查链，不能将本项直接扩大为管理员助手可随意选择路径。

**结果。** “没有 HTTP 探测请求”不等于“整个检查无外联”；文件系统可先解析/接触远程目标，涉及实际流量与 Windows 身份验证的结果取决于系统和网络策略。本次未访问任何 UNC 目标、未捕获流量或身份验证，不能宣称已看到凭据泄露。`privacy/manager.go:120` 的无条件“不外联”说明需要与真实文件系统范围一致。

**合成证据。** Go overlay 将 `privacy/store.go` 中的 `os.Lstat(path)` 改为只记录路径并返回 `os.ErrNotExist` 的测试 stub，Manager 指向 `\\example.invalid\redirected-roaming\...`；Report 把合成 UNC 原样交给该 stub，通过。没有实际文件系统或网络访问。写侧 `secureDirectory` 虽检查绝对路径和 reparse，但先 MkdirAll，而且绝对 UNC 不是本地固定盘证明；不应以写侧检查推定报告侧已安全。

**改进。** 隐私缓存/恢复 store 使用正式本地已知目录，或在一切 Lstat/Mkdir/Open 前确认支持的本地盘及路径形式；对远程重定向给出“未读取该恢复存储/无法确认”并保留用户其他隐私、隧道功能。关键目录仍需句柄/祖先可信检查；拒绝 UNC 字符串只能覆盖一种情况。需要兼容企业重定向时，明确让用户选择支持方案，不能默默复制/迁移已有敏感备份。

## C05：response ID 未核对

`internal/ipc/server.go:185` 反序列化 response 后只检查 OK 并解 result，没有比较 `resp.ID` 和 `req.ID`。上述内存 peer 故意返回 `req.ID+1`，测试仍成功。

这是协议关联完整性缺口。当前每次 Call 新建一条连接，不能据此直接推断正常并发请求一定串包，也不是 C01 订阅泄露的必要条件。修复时先校验 ID，再接纳结果/错误；测试应覆盖错 ID、缺失 ID、错误响应和正确响应。此校验不能代替服务器身份认证。

## C06：机器 store 在写入时被授予当前用户 SID 权限

**确认的配置变化。** `privacyhost/store_windows.go:14` 的 privateSDDL 只有 SYSTEM/BA；`privacy/manager.go:28` 的 NewMachine 虽设 machineOnly，却仍调用相同的 `lockStore`/`durableWrite`。`privacy/store_windows.go:38`–`:50` 没有机器模式参数，统一给当前用户 SID 增加 `OICI;FA`。因此机器 Apply/Restore 通过加锁、持久保存，会把本应只向管理员角色/SYSTEM 开放的机器目录再次加入具名用户 SID，子 journal/lock/临时文件也可能继承。

**前提和主体。** 只涉及执行这次 UAC 助手/提升 CLI 的账号 SID，替代管理员授权时为替代管理员，不是原普通 UI 账号，也不是所有 Users。该账号以后的非提升进程、被降出管理员组后的进程，其具名 SID 仍然相同；DACL 层面的授予不随管理员组成员资格自动消失。实际写权限还受 Mandatory Integrity、子文件 ACL、祖先替换权限等限制，本次没有用非提升/降权账号实际读写机器备份，不能把 DACL 放宽直接称为已经完成的提权利用。至少它违背“仅管理员角色/SYSTEM 可访问的机器 store”不变量，不能为让 QA 通过而接受此 ACE。

**持久时间。** GUI helper 的 `runHelper` 在 `platform_windows.go:290` 一次建立机器目录、`:315` 一次创建 dispatcher；其持续会话的 Apply/Restore 和 `protocol.go:379` 起的心跳 Report 复用 manager。心跳不是重新调用 `machineDirectory`，所以不能假设该具名 SID 每 15 秒都会移除。CLI 的 ExecuteElevated 每次建立 dispatcher 可能再收紧目录，但这也不能证明所有历史子文件的权限/所有者都被验证。

**建议。** 将用户和机器 store 的 ACL/owner/可信祖先配置作为显式不可混用的边界：机器目录和 journal 仅允许 SYSTEM/BA，机器锁与 durableWrite 使用相同机器配置；用户 store 保留正确用户 SID。检查已有 child owner/ACL/重解析，而非只检查父目录。修复后用普通管理员 UAC、替代管理员、助手持续会话、CLI、管理员账号后来降权以及崩溃残留分别验证。旧机器 store 中额外 SID 的收紧/迁移需保持原 journal 完整，不能丢失恢复记录或自行恢复全部策略。

本项为调用链和 SDDL 静态确认；未增加实际机器路径写入测试。现有 VPS QA 脚本如在写入后拒绝该 ACL，应保留拒绝并报告产品边界不符合预期。

## 原问题与正向边界复核

| 范围 | 当前复核 |
|---|---|
| A03 管理员助手重连 | `cmd/godusevpn/privacy.go:33` 仍单次启动；`protocol.go:348` 一带失败关闭会话，没有公开重新授权接口；未知结果不自动重试的做法正确，缺少重连/先核对现状流程仍需修复 |
| A07 卸载竞态 | `Uninstall-Ultra.ps1:173` 只确认安装位置 GUI；`:213` 清理与 `:218` 卸载间没有所有机器写入口共享的维护锁；副本/helper/CLI 写仍能交错；保留工具和不删数据是正确的失败后备 |
| A13 SYSTEM 机器恢复 | `privacy/manager.go:340/:418`、`backend_windows.go:140`、`store_windows.go:38` 均依赖交互 SafeUser；只改 Restore 一个入口不足。machineOnly 应单独建立“已提升、仅固定 HKLM、仅受保护机器 journal”的上下文，HKCU 仍拒绝。新增 fake 复现已管理的机器项在 SafeUser=false 时不能恢复、写计数不变且备份保留；没有跑实际 SYSTEM 恢复 |
| A21 时区恢复 | CLI 未接入 timeprivacy；GUI 不可用时的独立恢复缺口仍在；时区恢复不能混入机器策略 RestoreAll 冒充完整清理 |
| 策略 journal | 写前持久保存、固定 catalog 校验、恢复保留外部变更、损坏拒写均有代码与 fake；checksum 是损坏检测，不是抵御可写该 store 的恶意主体认证；机密边界依赖目录信任 |
| 比较后写入 | 当前“CAS”是读-比较-写，收窄外部软件竞争窗口，不是 Windows 注册表/时区的跨进程原子 compare-and-swap；不能承诺覆盖所有外部并发。产品内部实例使用 store lock，卸载另缺全局维护窗口 |
| 机器/用户隔离 | machineBackend 固定 HKLM、dispatcher 拒绝用户浏览器 ID；机器 owner 标识与管理员账号独立，合法另一个管理员可恢复；现有 fake 通过 |
| IPC helper | 固定方法/参数，大小上限、session、递增 ID、双向 PID、父进程句柄、超时关闭且不重试均已实现；这是新 helper 的边界，不替代 C01 主 IPC |
| 安装 | 固定 ProgramFiles、新目录、校验 manifest、保护代码 ACL、不合并覆写、不杀同名进程、不递归删除均保留；真实安装/卸载/多用户/中断续装尚未实测；hash 不等于发布者签名 |
| 时区 | 固定系统目录 tzutil、无 shell/PATH 查找、只允许枚举 ID、保持 DST、原值备份及外部变更冲突处理存在；未运行实际 tzutil 或修改时区 |

## 本次执行的无副作用验证

1. `go test ./internal/privacy ./internal/privacyhost ./internal/timeprivacy ./internal/ipc ./cmd/godusevpn -count=1`：五包通过。策略使用 fake 注册表，时区使用 fake backend，helper 测试不启动 UAC；部分 store 测试只写测试临时目录/ACL。
2. `TestAuditUnauthenticatedPeerReceivesFixtureAndWrongIDAccepted`：Go overlay + net.Pipe，通过并复现 C01 应用协议边界/C05。没有创建真实命名管道。
3. `TestAuditMachineRestorationRejectedForSystemLikeContext`：fake backend + 临时 journal，通过并复现 A13；没有使用实际 SYSTEM 令牌修改 HKLM。
4. `TestAuditRemoteJournalPathReachesFilesystemBoundary`：Lstat stub + 合成 UNC，通过；没有发出 UNC/SMB 请求。
5. 两个安装脚本只调用 PowerShell Parser.ParseFile，语法错误数均为 0；没有执行脚本的安装、ACL 修复、服务或卸载动作。

额外测试通过的是“发现可复现”，不代表产品验收通过。临时 overlay 放在系统 TEMP 中，应用源码没有被写入或替换；测试样本全部虚构，不含服务器账号、订阅、实际路径标识或设备信息。

### 最小复现主体（保留在测试 overlay，禁止替换实际数据路径运行）

主 IPC overlay 仅将 `server.go` 的 `dial(dctx)` 调用替换为下列 `auditDial(dctx)`；测试文件作为 `internal/ipc` 包的新虚拟测试文件加入 overlay：

```go
var auditRequest chan Request

func auditDial(context.Context) (net.Conn, error) {
    client, server := net.Pipe()
    go func() {
        defer server.Close()
        line, _ := bufio.NewReader(server).ReadBytes('\n')
        var req Request
        _ = json.Unmarshal(line, &req)
        auditRequest <- req
        _ = json.NewEncoder(server).Encode(Response{
            ID: req.ID + 1, OK: true,
            Result: json.RawMessage(`"fabricated-state"`),
        })
    }()
    return client, nil
}

func TestAuditUnauthenticatedPeerReceivesFixtureAndWrongIDAccepted(t *testing.T) {
    auditRequest = make(chan Request, 1)
    var out string
    err := Call(context.Background(), MAddProfile,
        map[string]string{"url": "https://example.invalid/sub/fixture-only"}, &out)
    if err != nil || out != "fabricated-state" { t.Fatal(err, out) }
    got := <-auditRequest
    if got.Method != MAddProfile { t.Fatal("unexpected method") }
}
```

本次没有运行 live registry/WFP、真实隐私数据读取、诊断采集、隧道、安装/卸载或 VPS 操作。C01/C02/C03 的跨账号 Windows 实机利用条件应在隔离机器、合成数据和恢复基线下验证，不在用户原项目/当前机器直接演练。
