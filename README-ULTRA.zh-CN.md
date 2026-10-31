# GodUseVPN Ultra

GodUseVPN Ultra 是 GodUseVPN 的独立隐私工作台，面向 Windows 桌面使用。它把 VPN 连接、浏览器环境、隐私设置和检测证据放进同一条清晰的操作路径。

当前预览版：**v0.02 · Windows x64 · 未签名**。

## 能做什么

- 保留 GodUseVPN 的订阅、节点、TUN、DNS、分流和连接能力。
- 管理支持的 Windows 隐私策略，并在写入前保存原值、写入后独立读回。
- 为每个浏览器保存独立的系统环境、UA、GPU、视口和代理配置。
- 配置 SOCKS5、HTTP 和 HTTPS 代理，并提供主动连通性检查。
- 通过隐私中心和检测中心查看作用范围、来源、读回时间、未知项和恢复记录。
- 检查并清理 JPEG / PNG 的指定元数据，输出新文件，不覆盖原文件。
- 导出加密的浏览器配置档案，导入时校验完整性并清除敏感连接字段。
- 提供 Chrome / Firefox 官方版本目录和已下载内核管理；未就绪的运行时会明确拒绝启动。

浏览器环境可以预览 Windows、macOS、Android 和 iOS 配置。真实浏览器窗口目前以 Windows WebView2 为宿主；其他系统属于配置预览，页面会明确标注运行时状态。

## 下载与安装

1. 从 [Releases](https://github.com/Maoyangui/godusevpn-ultra/releases) 下载 `godusevpn-ultra-0.02-windows-amd64.zip` 和同名 `.sha256`。
2. 校验 SHA256 后完整解压到本地固定磁盘目录。
3. 先用管理员 PowerShell 运行 `./Install-Ultra.ps1 -WhatIf` 查看目标，再运行 `./Install-Ultra.ps1` 安装。
4. 启动 `godusevpn.exe`，导入自己的订阅并按需启用隐私设置。
5. 修改后到检测中心刷新读回；需要浏览器能力时，在对应浏览器配置中打开并再次检查。

应用需要 Microsoft WebView2 运行时，本包不会自动下载或安装它。未签名预览包适合测试，不代表正式发行版。

## 使用原则

- 先看作用范围，再确认写入；保存成功不等于运行时已经生效。
- 检测中心将状态分为已验证、部分支持、待复核和未知；未知不会显示成“已保护”。
- 失败会保留原值、错误原因和恢复入口，不自动改变 VPN 连接状态。
- 浏览器配置、Cookie 和代理凭据按档案隔离；导出档案使用 AES-256-GCM，敏感连接字段不会写入。
- 每台浏览器都有自己的 UA、GPU、视口和代理草稿，打开后再读回实际值。

## 构建

```powershell
go install github.com/tc-hib/go-winres@v0.3.3
./build-ultra.ps1 -Version 0.02
```

构建脚本会运行回归测试、生成 Windows 资源、编译客户端 / 服务 / CLI，并输出全小写包名和 SHA256 文件。已有输出目录不会被覆盖。

## 当前验证

| 项目 | 状态 |
| --- | --- |
| 页面展示、交互动画、中英文切换 | 已在 1440 / 1024 / 768 / 390 宽度验证 |
| 浏览器配置、代理草稿、内核删除、档案加密 | 自动化回归通过 |
| Windows 真实 VPN、代理出口、WebRTC / WebGPU / GPS | 仍需在对应 VPS / 实机复核 |
| Win10 1809、19044 / 22H2、Win11 x64 / ARM64 | 逐版本证据完成后再标记支持 |

未验证的能力会继续显示为未知，不用演示页面替代真实运行证据。

## 恢复与卸载

修改前会保存原值。需要卸载时先运行 `./Uninstall-Ultra.ps1 -WhatIf` 预览，再以管理员权限执行卸载。恢复失败会保留工具和记录，便于继续处理。

详细功能台账见 [项目计划](PROJECT_PLAN.zh-CN.md)、[功能状态](docs/ultra/FEATURE_STATUS.zh-CN.md) 和 [恢复说明](docs/ultra/INSTALL_RECOVERY.zh-CN.md)。

## 相关链接

- [Ultra 展示页](https://maoyangui.github.io/godusevpn-ultra/)
- [节点订阅](https://godusevpn.org/subscription/)
- [浏览器扩展说明](browser-extension/README.md)
- [英文 README](README.md)

本项目遵循 GPL-3.0，第三方声明见 [NOTICE](NOTICE)。
