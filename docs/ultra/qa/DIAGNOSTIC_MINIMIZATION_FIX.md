# A01 / C03：最小诊断、明确选入与本机私有存储

2026-10-19。只修改 Ultra 副本；原项目不改。本记录覆盖 daemon、诊断 IPC、日志/历史诊断目录 ACL 与旧 Web 桥兼容；桌面预览/私有保存由对应 GUI 改动单独验证。未在当前机器采集真实网络、日志或系统诊断。

## 修复前提与目标

A01 的问题不是自动上传：原导出无条件调用系统网络命令，并加入日志、完整应用状态、节点/配置；普通文本主机名、IP、MAC 等不被现有凭据打码移除。用户导出/分享会把这些关联信息带出。C03 补充本机范围：历史 logs/diag 的 DACL 给全部 BUILTIN\\Users 读权限，因此不需要先外发就可能被另一标准账户读取。

本次采用默认不采集敏感源，而非先生成全量包再过滤。排障能力通过逐项明确选入保留，不能宣称详情已经匿名化。

## 默认与详细内容

`Diagnose` 仅返回白名单摘要；无参数 `ExportDiag` 同样只生成该摘要和范围说明。摘要含产品版本、OS/架构、固定连接状态/错误码、保护开关及“是否有错误记录”等布尔证据。不返回错误原文、精确连接时间、路径、设备/账户/主机名、地址、订阅/节点、原始 settings/state、日志或系统命令输出。

默认路径不调用 `stateView`，不打开 config/log/crash，不执行 route、ipconfig、netsh 或其他网络工具。测试以缺少 core/log/network 对象的 daemon 和会触发失败的 collector 证明这些依赖不被访问。

详细内容分成四个独立选择，零值全部未选：

| 选项 | 内容 | 仍可能保留的信息 |
|---|---|---|
| logs | 服务和内核日志末尾有界片段 | 地址、名称、路径、访问/错误记录 |
| config | 有界有效配置的常见凭据打码副本；必要的错误、DNS、选择和端口状态摘要 | 节点/域名、IP、路径、网络关系；不输出全部原始状态/订阅/设备列表 |
| crashes | 崩溃段原因和栈摘要，保留原有段落提取逻辑 | 本地路径、进程/栈及错误中的标识 |
| network | 原有本地网络详情命令 | IP、MAC、主机/接口名、DNS、网关、路由等 |

任一详情需要 `acknowledgeSensitive=true` 才能开始读取。普通凭据键、URL 用户信息、token 等继续打码，但不能据此把详情称为匿名报告。不可用、过大、截断及省略均在预览/范围说明中显示，不输出原始错误路径作为替代。

Windows 网络命令仅允许固定的 route.exe、ipconfig.exe、netsh.exe，从系统目录解析绝对路径并隐藏窗口；每个命令最多 5 秒、输出有界。默认和自动测试不执行这些命令。

## 预览和导出契约

共享类型位于 `internal/ipc/diagnostic.go`。

1. `PreviewDiag(DiagnosticOptions)` 生成一份可读的文件内容预览、选择、警告、随机 ID、过期时间和 ZIP SHA-256。
2. 预览时 ZIP 已由**相同文件内容**完成。服务只在有界内存保存，最多两份、五分钟；采集过程串行，避免并发构造放大内存。单一定时器清理过期内容，服务关闭时清空缓存；导出取得的是副本。
3. `ExportDiag({previewId})` 返回同一缓存 ZIP 的 `filename`、base64 `data`、`sha256`。过期/未知/被替换的 ID 明确失败，绝不自动重新采集。桌面再验证与用户预览一致的 hash，使用新文件私有创建保存。
4. 服务不再向机器目录写新 ZIP，也不向普通 GUI 返回一个需要公共读权限的服务文件路径。原 `ExportDiag(nil)` 仅为最小内容兼容入口；输出结构已明确变为 bytes，不继续旧 path 契约。

上限：单文件 128 KiB，全部预览文本 2 MiB，ZIP 3 MiB，base64 数据最多 4 MiB。缓存最多两份 ZIP 加两份预览文本，即内容上界约 10 MiB，另有有界构造/序列化副本与元数据。日志只读取末尾有界片段；崩溃提取最多读取末尾 2 MiB，并说明较早原因可能缺失；过大的 JSON 配置不输出原始片段。

## 本机读取边界

`paths.PrepareServiceStore` 继续先验证既有 owner、DACL 写权限、祖先、重解析、普通文件/硬链接边界，全部受信后才做收紧。新 logs/diag 直接使用仅 SYSTEM/Administrators 的私有 DACL；既有 logs/diag **每一个已验证后代**也显式收紧，涵盖受保护、不继承父目录权限的历史文件。仅改父目录不能修复这些文件，测试专门覆盖该情况。

不接管普通用户拥有或可写的异常目录，不扩展扫描到可选 privacy journal，不因用户 journal 失败额外阻断服务。普通 GUI 继续通过已有认证 IPC `GetLogs` 读取排障内容，日志记录/轮转/保留功能不关闭。直接打开服务日志目录需要管理员权限，界面应明确说明；不能为恢复这个目录入口而重新授权全部 Users。

该变更限制今后按文件路径的新读取，不撤回旧版本已经复制/分享的内容或外部进程已有的访问能力，也不试图防止管理员/SYSTEM 读取。TEMP DACL 测试证明允许 ACE 只剩这两个角色、owner 没被更改；没有冒充另一真实账户登录实测。

## 明确的跨平台兼容变化

本次首发对象为 Windows 桌面。Web/Android 共用 `uiapi` 保留最小 `Diagnose`，可调用仅最小选项的 `PreviewDiag`；详细采集及 ZIP 导出明确提示使用 Windows Ultra 客户端，不再静默读取不存在的 `path`。

旧 `/api/diag/*` 返回 HTTP 410，不访问服务文件，也不允许旧历史包绕过新预览下载。`web/dist/api.js` 是通用方法代理，没有另一个 web.js 路径。旧 Android 宿主得到明确不支持错误。**这是一项已说明的兼容收窄，不能称所有平台原有导出不变。** 桌面仍保留用户明确选入的详细排障。

## 实际验证

```text
go test ./internal/daemon -run 'TestDiagnostic|TestRedact|TestExportDiagKeepsSettingsIntact|TestCrashExcerpt' -count=1 -v
go test ./internal/paths ./internal/uiapi ./internal/web ./internal/ipc -count=1 -v
go vet ./internal/daemon ./internal/paths ./internal/uiapi ./internal/web ./internal/ipc
```

均通过。证据区分如下：

- fake：白名单排除合成主机名、IPv4/IPv6、MAC、用户路径、域名及任意错误；敏感来源默认零调用；严格 schema/ack/逐项选择；ZIP 与预览内容逐文件一致、hash 为实际 ZIP；过期/逐出不重采集；缓存/返回值不可被调用者改写；各级大小界限与无命令执行的输出缓冲测试。
- TEMP：显式详细日志/配置/崩溃仅读取合成文件，保留必要原因，移除已覆盖的常见凭据；预览后文件变化不会改变导出；没有生成服务端 ZIP。日志/diag 私有 DACL 在合成临时文件/目录上的实际设置与 owner 保留测试通过。
- fake ACL/桥：既有嵌套历史文件全部收紧，未验证的对象仍拒绝；Web/Android 详情/导出不调用 daemon，最小摘要仍可用；旧下载 410 不分派采集或返回下载头。
- 继承的 `TestExportDiagKeepsSettingsIntact` 保留真实 TEMP 导出与设置不变断言，删除与该目标无关的 `.example` 刷新请求。该用例可直接加入 CI，无需 overlay，也不执行真实 sysDiag。

这些测试不等同于真实 Windows 跨账户、普通 GUI 对 SYSTEM 管道访问、浏览器下载或 VPS 流量验收。UI 交互、用户侧文件私有创建和最终发布包还需对应复核。公开发布不得包含测试凭据、服务器地址、订阅或云资源标识；本记录只使用合成样本。
