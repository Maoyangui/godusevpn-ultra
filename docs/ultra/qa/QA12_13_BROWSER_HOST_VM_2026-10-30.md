# QA12.13：Win10 21H2 GUI、隐私中心与浏览器宿主 VM 实测（2026-10-30）

本批使用一次性 Azure Windows 10 Enterprise LTSC 2021（Windows 10 21H2，build 19044）VM，运行验收构建 `0.01-beta-qa.12.13`。本机没有修改网络设置；VM、Bastion、临时传输凭据和关联资源在验收后删除。

## 构建与传输

| 项目 | 结果 |
|---|---|
| ZIP | `godusevpn-ultra-0.01-beta-qa.12.13-windows-amd64.zip` |
| ZIP 大小 | 55,328,238 bytes |
| ZIP SHA-256 | `63e2acec6f4bfee7985f756f1b1545ec3a2d21de93e7300660d37537b2a584a3` |
| 分片 | 18 个，逐片长度与文本 SHA-256 校验通过 |
| 解包 | `Expand-Archive` 成功，顶层包目录与版本一致 |

临时 GitHub secret gist 仅用于 VM 入站传输，19 个 gist（18 个分片加 1 个 manifest）已删除；本地 `.codex/tmp/qa12.13-transfer` 已删除。

## 真实 VM 结果

| 检查 | 结果 | 证据边界 |
|---|---|---|
| `Install-Ultra.ps1 -WhatIf` | PASS，官方脚本冷启动返回成功 | 由 SYSTEM Run Command 执行 |
| 实际安装 | PASS，目标目录存在 | 由 SYSTEM Run Command 执行 |
| `godusevpn-ultra` SCM | PASS，Running、LocalSystem、路径匹配 `godusevpn-svc.exe service` | 独立 CIM 回读 |
| CLI 版本 | PASS，`0.01-beta-qa.12.13` | 安装目录 CLI 回读 |
| `privacy report` | PASS，退出码 0，JSON 可解析，报告含 4 个 warnings | 只保存汇总，不保存原始报告 |
| 普通用户 GUI 第一次启动 | PASS，Bastion Developer 远程桌面可见 Ultra 主窗口 | Windows 10 普通用户会话 |
| 隐私中心 | PASS，可打开并显示 26 个本次策略项目、0 个待确认项目 | 截图视觉核验 |
| 浏览器保护页 | PASS，浏览器环境、指纹设置、代理连接、内核版本区块可见 | 截图视觉核验 |
| GPU/硬件预览 | PASS，GPU 预设、WebGPU、WebGL 厂商等字段可见，预览显示“原生信息/待独立浏览器读回” | 没有把预览当成实际浏览器回读 |
| GUI 第二次启动 | PASS，可重复打开主窗口和隐私中心 | 第二次 Bastion 视觉核验 |
| GUI 关闭 | PASS，窗口关闭；随后按安装目录精确清理 GUI 进程 | 未停止 `godusevpn-ultra` 服务 |
| WebView2 运行时 | PASS，VM 回读到 `156.0.4314.50` | 运行时存在不代表宿主已启动 |
| browser-host 文件 | PASS，安装目录 `browser-host` 含 7 个文件 | 文件存在不代表用户可启动 |
| 浏览器宿主窗口 | LIMITED，页面显示“需完整安装”，启动按钮保持“启动尚待验收”，未打开浏览器窗口 | 这是当前真实门控结果，不改写为 PASS |
| Bastion | PASS，Developer SKU 成功建立一个桌面会话；并发尝试被 Azure 正确拒绝 | 3 个临时前端标签中只有一个会话可用，符合 Developer SKU 限制 |

浏览器保护页的启动门控保持保守：组件存在、WebView2 运行时存在，但当前配置仍要求完整安装并保持“待验收”，没有在 VM 上伪称 Chrome/Firefox 多版本或真实代理浏览通过。

## 清理回执

- 已删除临时 VM `ultra-qa-win10-browser-20261030`。
- VM 删除动作同时删除了 1 个 OS 磁盘、1 个 NIC 和 1 个临时公共 IP。
- 已删除临时 NSG `ultra-qa-win10-browser-20261030-nsg`。
- 已删除临时 VNET `vnet-eastasia-4`。
- 已删除 Developer Bastion `vnet-eastasia-4-bastion`，其临时公网依赖随资源清理。
- 资源组最终仅保留原有 `ultra-qa-bastion-publicip`、`ultra-qa-desktopNSG`、`ultra-qa-desktopVNET` 与 `ultra-qa-restore-20261026`。
- 本机网络设置：未修改。

## 未关闭的真实运行时范围

本批没有把服务/GUI证据扩展为真实订阅隧道、WFP/BFE、IPv4/IPv6/回环、SOCKS/HTTP实际出口、Chrome/Firefox多版本窗口、CEF、uBlock、GPS写入或 WebRTC/WebGPU 实际浏览器读回。这些项目继续保持“待实机验收/未集成”，见 [开源集成全量审计](OPEN_SOURCE_INTEGRATION_AUDIT_2026-10-29.md)。
