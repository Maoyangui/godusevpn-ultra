# Win10 1809 / 21H2 / 22H2 实机验收计划

本文件把可见镜像与实机通过分开。当前订阅在 East Asia 已返回三份可复现的 Win10 x64 镜像：

| 目标 | 精确 URN | 版本 | 版本边界 |
|---|---|---:|---|
| Win10 1809 Enterprise | `MicrosoftWindowsDesktop:Windows-10:rs5-enterprise-g2:17763.9376.261010` | 17763 | 安装后必须重新读取 edition；不能把 Server 2019 当客户端证据 |
| Win10 21H2 Enterprise LTSC | `MicrosoftWindowsDesktop:Windows-10:win10-21h2-ent-ltsc-g2:19044.7807.261013` | 19044 | 记录 LTSC edition、补丁和 WebView2 分发状态 |
| Win10 22H2 Pro | `MicrosoftWindowsDesktop:Windows-10:win10-22h2-pro-g2:19045.6456.251117` | 19045 | 已完成普通用户 GUI 启动的有限实测 |

## 每个 VM 的最小证据

1. 创建前固定精确 URN、区域、架构和 VM 代系；创建后读取 `ProductName`、edition、`CurrentBuild`、`UBR`、架构、交互用户名和 WebView2 版本。SKU 名称不能替代这些读回值。
2. 安装同一个候选 ZIP，保存 ZIP、GUI、服务、CLI 和 `SHA256SUMS` 的哈希；确认 GUI PE 清单仍为 `asInvoker`。
3. 以普通用户启动 GUI：记录窗口是否出现、是否有无关 UAC、会话 ID、GUI 路径、服务会话和是否存在隐私助手进程。启动阶段不点击机器策略或浏览器隔离授权按钮。
4. 只读检查 WebView2、.NET Framework、Windows SDK/WinRT 相关文件。缺少 WebView2 时，记录为“宿主未初始化”，保留 GUI，不把它写成浏览器功能通过。
5. 进入隐私页后只验证显式授权门槛：取消 UAC 或助手失败应保留普通界面；没有真实隧道和 WFP 证据时，不宣称流量或网络保护通过。
6. 运行 CLI/服务只读检查，确认 SCM、IPC、ACL 和当前用户边界；不在旧版 VM 上为了让测试通过而安装驱动、关闭安全策略或修改系统保护。
7. 每个 VM 验收完成后删除 VM、OS 磁盘、NIC、临时公网 IP、Bastion 和快照，并保留删除回执。删除失败或处于异步 `Deleting` 时，状态保持 pending，不改写为已清理。

## 版本特有检查

- **17763**：重点观察 WebView2/Edge 是否预装、`SetInterfaceDnsSettings` 不可用时的 DNS 回退、Schannel 最低版本和旧设置入口；任何新 WinRT provider 均保持不可用，除非运行时类型和成员实际存在。
- **19044**：重点观察 LTSC edition、WebView2 安装状态、普通用户启动和 UAC；不要用 19045 的补丁或组件清单代替 19044。
- **19045**：复用已完成的 GUI/UAC 结果，再补安装/卸载、服务重启、WebView2 缺失和网络功能的独立证据。

17763/19044 已完成同一候选包的 Azure 真实安装、服务/CLI/隐私报告回读；Run Command 运行在 SYSTEM 且没有交互桌面，普通用户 GUI 启动返回 Access denied，因此两版继续保持 `LIMITED`。只有补齐标准用户 GUI、WebView2、UAC、浏览器、服务/IPC 和目标功能读回后才能升级为 `PASS`。记录见 [QA12.12](QA12_12_WIN10_17763_VM_2026-10-29.md) 与 [QA12.11](QA12_11_WIN10_19044_VM_2026-10-29.md)。

## 证据记录模板（每台旧版 VM 一份）

验收记录必须与 VM 名称和 Azure operationId 绑定，建议使用以下字段；缺任一字段时保持 `待`，不能用同一 build 的其他 edition 或另一台 VM 代替：

```text
VM / RG / region:
精确 URN / architecture / generation:
ProductName / EditionID / DisplayVersion:
CurrentBuild / UBR / OS install date:
GUI ZIP SHA256 / GUI SHA256 / service SHA256 / CLI SHA256:
PE subsystem / requestedExecutionLevel:
WebView2 version + install path / .NET Release:
普通用户 SID / session / GUI PID / service PID:
普通启动 UAC: none | expected | unexpected
PrivacyHost before explicit authorization: absent | present
Browser host: initialized | unavailable (reason)
Proxy probe: pass | fail (error and no-retry behavior)
Network/TUN/WFP: pass only with traffic evidence; otherwise not tested
Cleanup: VM / OS disk / NIC / public IP / Bastion / snapshot delete receipts
```

### 结果标记

- `PASS`：在该 VM 上执行并保存了可复查的读回或行为证据；不能只凭编译、镜像名称或源码判断。
- `LIMITED`：只覆盖明确列出的路径，例如普通 GUI 启动；同一版本的其他模块继续保持 `待`。
- `BLOCKED`：资源、运行时或权限导致无法执行，记录原始错误和下一步，不把失败改写成通过。
- `NOT_APPLICABLE`：仅当该功能在该 edition/build 明确没有对应系统接口，并附上版本检测结果。

### 旧版专门回归顺序

1. 先做只读系统与运行时读回，再安装或启动应用；不要为测试关闭 Defender、修改代理、开启全局 TUN 或改变本机网络。
2. 先以标准用户启动 GUI，再单独走显式隐私授权；两条路径的进程会话、UAC 和 helper 状态分别记录。
3. 在 17763 上确认 DNS API 缺失时的既有回退是否返回明确结果；在 19044 上确认 LTSC 的 WebView2 缺失分支和安装提示。回退失败时保留 GUI 并记录错误，不静默降级。
4. 测试完成后按资源清单逐项删除，等待 Azure 异步删除完成，再删除失去关联的公网 IP；删除回执写入 `.codex/tmp`，不删除共享 VNET/NSG 或用户已有快照。

