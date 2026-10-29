# Windows 本地隐私策略引擎

此包只负责用户主动选择的本地策略。它不导入 daemon、builder、state、隧道、DNS 或路由模块，不发网络请求，不启动 shell，不安装证书，也不读取实际 IP、MAC、硬件序列号、访问历史或剪贴板内容。

## 当前实现

共 26 个独立可执行项：Chrome、Edge 各 11 项 HKCU 浏览器策略，以及 4 项 HKLM Windows 策略。每项默认不写入，用户主动启用后先保存原值，再设置固定目录内的值。普通应用取消 UAC 后仍可管理当前用户浏览器策略；Windows 机器策略需要一次性提权辅助进程。

| 范围 | 已实现策略 | 限制 |
| --- | --- | --- |
| Chrome / 当前用户 | WebRTC 非代理 UDP、网络预测、在线搜索建议、使用/崩溃指标、默认定位、摄像头、麦克风、通知、传感器、第三方 Cookie、WebGL | 注册表回读只能确认配置；需在 chrome://policy 确认接纳，再由对应浏览器环境验证行为。企业或云管理策略、站点例外、旧版本均可能改变结果。 |
| Edge / 当前用户 | 上述同类项；使用 `WebRtcLocalhostIpHandling`，指标项为 `DiagnosticData=1` | 保留必需诊断，关闭可选诊断；122+ 支持单独诊断策略。部分策略不适用于登录 Microsoft 账号的配置文件。 |
| Windows / 整机 | 广告 ID、输入个性化、剪贴板历史、剪贴板跨设备同步 | `AllowInputPersonalization` 只限制 Windows 输入个性化，**不等于关闭在线语音识别**。只在正式文档支持的 Win10/11 Pro、Enterprise、Education、IoTEnterprise 等 SKU/最低版本开放自动写入。Home 和无法核实的版本只引导手动。 |

WebRTC 项没有完全关闭 WebRTC，也不能把 TUN 当作浏览器显式 SOCKS 代理。WebGL 项不证明 WebGPU 被禁用。Cookie 项不隔离登录账号，不清除现有站点数据。默认权限策略可能有已有企业允许列表。剪贴板项不阻止桌面程序读取当前剪贴板、不清理第三方工具。任何一项都不能被界面翻译成“完全匿名”或“所有应用都无法读取”。

无法通用实现的功能列为 manual / unsupported / planned / limited：时区语言一致性、Windows 应用权限、随机 MAC、Canvas/Audio/字体指纹、WebGPU、原生硬件标识隔离、身份容器、链接审计/Referer/跟踪参数、按应用 WFP 与回环/LAN 隔离、文件元数据清理、完整访问审计。全应用 GPS 伪装现在列为 `planned`：`internal/locationadapter` 只提供组件来源、哈希、权限、备份、读回和回滚边界，尚未随包执行第三方组件。每项带作用范围、剩余暴露和操作引导。链接审计只有已核实的浏览器扩展接口，没有伪造企业 registry 策略。

## API 和权限边界

```go
manager := privacy.New(userConfigDir + "/godusevpn-ultra/privacy")
report, err := manager.Report()
result, err := manager.Apply(controlID)
result, err = manager.Restore(controlID)
results, err := manager.RestoreAll()
```

`NewMachine(fixedProgramDataDir)` 仅供固定路径的一次性提权 helper 使用。它只允许 4 项 Windows 机器策略，目录枚举、备份验证、backend 读写均拒绝 HKCU。机器备份使用固定本地作用域标识，不绑定临时 UAC 管理员 SID，从而允许后续授权的管理员恢复。helper 的命令入口还必须固定操作和 ID 白名单，不能允许从 UI 传入任意目录、注册表路径、命令或 URL。正常 UI 的 HKCU 操作不得委托给提权进程，以免 UAC 输入另一管理员账号时修改错用户。

本地普通用户备份仅绑定当前用户 SID 的摘要，Report 不返回这个摘要。SYSTEM、Local Service、Network Service、虚拟服务账号和非交互登录上下文不能自动设置。机器策略 additionally 要求进程具有提升后的管理员令牌；拒绝 UAC 不得改变原隧道的连接行为。

## 事务和恢复

1. 读取固定策略目录；检查平台、用户、SKU、权限和机器级覆盖。浏览器已有机器级策略时拒绝用 HKCU 竞争覆盖。
2. 读取全部原值。目标值已存在则不接管、不创建恢复记录。原值为无法安全保留的类型或超限时拒绝写入。
3. 将 `applying` journal 原子持久保存：私有目录 ACL、随机临时文件、文件 Sync、Windows MoveFileEx 的替换与 write-through。此步骤成功前不触碰注册表。
4. 写入前重读原值，写入后回读，并 RegFlushKey。成功后保存 `active`。失败尝试恢复；恢复失败保留 journal，允许重试。
5. 恢复先持久保存 `restoring`，随后重读每个值。只有当前值仍等于本应用设置值时才恢复原值；已是原值则跳过，第三方改成其他值则保留并报告 conflict。全部恢复并成功保存完成记录后才移除对应备份项。

`applying` 或 `restoring` 在下次报告中标记 recovery_pending。报告不会自动重试系统修改；用户可选恢复。崩溃发生于写入和记录之间时，原始准备日志仍足以恢复。RestoreAll 持有一次跨进程文件锁，逐项处理并继续其他可恢复项。

Windows 注册表没有供本引擎使用的跨所有外部写入者原子 compare-and-swap。这里的“比较后恢复”是紧邻写入前的回读检查，加上本应用进程间锁；它能保留已观察到的外部变更，不能声称排除其他管理员或 GPO 在极短检查/写入间隙内并发改写的全部竞态。写入后复查与后续报告会继续检测偏移，不自动强行重施策略。

journal 带版本、固定策略目录验证、当前作用域绑定、大小上限和 SHA-256 完整性校验；只保存该目录内策略的原值，既不采集设备指纹也不记录应用流量。SHA-256 用于识别损坏，不用于对抗有权重写备份和校验和的管理员。损坏、异常类型、跨用户或非法路径会阻止所有写入和恢复，Report 仍返回可读目录及警告。不得自动删除损坏备份或覆盖原始证据。

状态目录与临时文件不接受相对路径和 Windows reparse-point 目录；仅当前用户、管理员和 SYSTEM 可访问新状态。恢复只操作值，不递归删除注册表键，也不接管原本已经处于目标状态的外部设置。

## 不可破坏的独立性

- 策略缺失、浏览器不接纳、检查失败、无管理员权限、磁盘错误或备份冲突只能改变本模块的状态。
- 不能因此拒绝建立隧道、改成直连、削弱 IPv6/DNS/严格全局保护或跳过其他策略。
- 本包不关闭 Safe Browsing、SmartScreen、Defender、系统更新、安全诊断必需项、浏览器沙箱或 TLS 校验。
- 新隐私功能不预加载外部测试站、不查询出口位置、不安装读取全部网页的扩展，也不上传诊断原值。

## 官方依据

- [Chrome 企业策略目录](https://chromeenterprise.google/policies/)
- [Chromium WebRTC 策略定义](https://raw.githubusercontent.com/chromium/chromium/main/components/policy/resources/templates/policy_definitions/WebRtc/WebRtcIPHandling.yaml)
- [Chromium 使用指标策略定义](https://raw.githubusercontent.com/chromium/chromium/main/components/policy/resources/templates/policy_definitions/Miscellaneous/MetricsReportingEnabled.yaml)
- [Chromium 网络预测策略定义](https://raw.githubusercontent.com/chromium/chromium/main/components/policy/resources/templates/policy_definitions/Miscellaneous/NetworkPredictionOptions.yaml)
- [Chromium WebGL 策略定义](https://raw.githubusercontent.com/chromium/chromium/main/components/policy/resources/templates/policy_definitions/Miscellaneous/Disable3DAPIs.yaml)
- [Edge WebRTC](https://learn.microsoft.com/en-us/deployedge/microsoft-edge-policies/WebRtcLocalhostIpHandling)
- [Edge 诊断策略与最低版本](https://learn.microsoft.com/en-us/deployedge/microsoft-edge-policies/DiagnosticData)
- [Edge 内容策略与覆盖例外](https://learn.microsoft.com/en-us/deployedge/microsoft-edge-policies/AudioCaptureAllowed)
- [Windows Privacy CSP：广告 ID、在线语音、剪贴板同步及 ADMX 映射](https://learn.microsoft.com/en-us/windows/client-management/mdm/policy-csp-privacy)
- [Windows Experience CSP：剪贴板历史及 ADMX 映射](https://learn.microsoft.com/en-us/windows/client-management/mdm/policy-csp-experience#allowclipboardhistory)
- [Chrome 隐私扩展接口：链接审计](https://developer.chrome.com/docs/extensions/reference/api/privacy)

## 验证

`go test ./internal/privacy` 使用 fake backend；所有 registry 写入、写后 flush 故障、回滚、外部变更、崩溃恢复和批量部分失败均在内存中模拟。测试可真实创建临时 journal 文件并测试持久性故障，但不 Apply/Restore 真实系统策略。正式发布还需干净 Windows VM 上逐浏览器版本核实 policy 页面和网页行为；代码回读不能代替这些验收。
