# GodUseVPN Ultra

GodUseVPN Ultra 是 GodUseVPN 的 Windows 隐私增强版。它保留原有的订阅、节点、TUN、路由和 DNS 能力，并增加隐私中心、检测中心、浏览器环境配置、代理连通性检测、应用规则和可审计的恢复记录。当前构建目标为短版本 `v0.02`；不会自动上传新的 Release。

当前发布为 **Windows x64 未签名预览版**。源码测试和界面测试已经覆盖的范围，会在功能台账中标明；需要真实 Windows、浏览器或网络证据的项目仍会明确显示为“待实测”。请不要把配置存在、策略写入或检测页面显示为完全匿名的证明。

## 快速开始

1. 在 [Releases](https://github.com/Maoyangui/godusevpn-ultra/releases) 下载全小写名称的 `godusevpn-ultra-<version>-windows-amd64.zip` 和同名 `.sha256` 文件，先核对 SHA-256，再完整解压。
2. 安装 Microsoft WebView2 运行时后运行 `godusevpn.exe`。仅查看界面可以使用 `godusevpn.exe --ultra-no-elevate`；需要修改机器策略时，再按提示授权固定权限助手。
3. 安装前在 64 位 PowerShell 运行 `./Install-Ultra.ps1 -WhatIf` 查看范围，确认后以管理员身份运行安装脚本。它只接受新的固定安装目录，不覆盖旧安装，也不会修改本机网络设置之外的其他网络代理配置。
4. 在隐私中心选择策略，应用后到检测中心刷新并查看证据来源、观察者身份和剩余限制。浏览器扩展需要分别安装到实际使用的 Chrome 或 Edge 配置。

完整安装、恢复和卸载步骤见 [详细中文说明](README-ULTRA.zh-CN.md)；功能范围与未完成项见 [功能台账](docs/ultra/FEATURE_STATUS.zh-CN.md)；验收证据见 [最终复核](docs/ultra/qa/FINAL_AUDIT_2026-10-30.zh-CN.md)。

## 已提供的主要能力

- 订阅、节点、连接、TUN、路由、DNS、模式和防直连闸。
- Windows 隐私策略：广告 ID、输入个性化、剪贴板历史和云同步。
- Chrome / Edge 策略：WebRTC 非代理 UDP、预测加载、建议、诊断、定位、相机、麦克风、通知、传感器、第三方 Cookie 和 WebGL。
- 浏览器环境编辑：Windows、macOS、Linux、Android、iOS 的 UA/平台预览，地区、视口、WebGL vendor/renderer 和每浏览器代理配置；系统环境是浏览器配置，不会改变宿主 Windows。
- SOCKS5、HTTP、HTTPS 代理测试和 GeoIP 查询；代理失败时不会静默回退直连。
- 应用联网规则、时区备份/恢复、JPEG/PNG 指定元数据清理、秘密剪贴板定时清理和本地检测报告。
- 官方 Chrome for Testing / Firefox 版本目录、校验和、内核缓存删除入口；未获得运行时证据的内核不会被标记为“已验证”。

## 边界与实测状态

网站仍可能读取账户、Cookie、出口 IP、时区、语言、字体和图形能力；WebRTC 非代理 UDP 限制不等于完整关闭 WebRTC，WebGL 设置也不等于关闭 WebGPU。原生应用、管理员程序和操作系统服务可能读取更多硬件或定位信息。检测中心默认不访问外部检测站、不发 STUN、不扫描局域网，也不读取 Cookie、令牌、真实 MAC 或序列号。

当前台账仍保留真实浏览器原生窗口/实际代理出口、GPS provider 写入与读回、CEF/uBlock、WebRTC/WebGPU/OOPIF/Worker、DPAPI 凭据迁移和多版本 Windows 矩阵等待实测项。没有证据的能力不会在界面或发行说明中宣称完成。

## 构建与验证

```powershell
go install github.com/tc-hib/go-winres@v0.3.3
./build-ultra.ps1 -Version 0.02
node tools/audit-built-package.cjs dist/godusevpn-ultra-0.02-windows-amd64
```

构建脚本执行回归测试、生成 Windows 资源、编译 GUI/CLI/服务、复制审计资料并生成 ZIP 与 SHA-256 sidecar。输出目录和压缩包都使用全小写的包名；产品内部的 Windows 程序标识保持兼容所需的显示名称。构建不会启动客户端、安装服务或修改本机网络设置。

## 许可证

本项目基于 [GodUseVPN](https://github.com/Maoyangui/godusevpn)，遵循 [GPL-3.0](LICENSE)。版权和第三方声明见 [NOTICE](NOTICE)。
