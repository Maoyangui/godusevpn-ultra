# 2026-10-26 临时 Windows VM 有限验收

本记录仅含可公开摘要；不附测试订阅、节点主机 / 端口 / 认证、云账号和资源标识。完整私有回执与截图留在本机忽略目录。测试发生在用户授权的可丢弃 Windows VM；没有在开发电脑安装 / 启动 Ultra、执行 WFP、修改网络或运行真实浏览器宿主。

## qa.7 官方卸载

- 固定归档 SHA256：`327acea1749747d24b2d945739219bb3ce35b09ef46cd937f126756355626bb6`。
- VM 原磁盘已安装 qa.7，服务为 Running / LocalSystem，Windows build 26100，无 Ultra GUI 进程。
- 独立校验安装包与卸载脚本后，官方卸载成功，回执 schema `ultra-qa7-official-uninstall-v1`，`completed=true`、`serviceCount=0`、`filesRetained=true`。
- 当时没有待恢复策略，不能用这次成功外推所有策略 / 冲突 / 崩溃恢复；文件保留也不等于清除了用户数据。
- 最初 WinPS 5.1 冷启动 `-File … -WhatIf` 暴露 Get-FileHash 的 ProviderPath 在 WhatIf 下被抑制的问题。源码已改用直接 .NET SHA256；冷启动 WinPS 5.1 / PS7 TEMP fixture 通过。修复未回写原 qa.7 包，新包需重新验收。

## SOCKS 节点独立探测

- 新订阅 `format=json` 含 12 个 Hysteria2、1 个 SOCKS，另有选择器 / 自动测试 / direct 类型；不把这些内部出站算成额外代理节点。
- VM 独立脚本从该订阅读取 SOCKS 配置，完成 RFC1929 认证、SOCKS 域名形式的远端 DNS、默认验证证书的 SslStream TLS，以及 country.is 固定 HTTPS API 请求。
- 回执 schema `ultra-socks-vps-probe-v1`，`authenticated=true`、`remoteDNS=true`、`tlsVerified=true`、`country=US`、出口 IP 有有效值。请求 / 响应有长度和超时上限，没有关闭证书校验。
- `settingsChanged=false`、`secretsPrinted=false`；测试不更改系统代理或 DNS。结果只证明该节点在此时可用，不证明产品 `BrowserProxyTest`、实际浏览器、VPN 隧道、WebRTC / DNS 防漏或 WFP 的行为。

qa.8 的真实安装、普通用户 / UAC、产品代理测试及浏览器仍分别待验收，不能继承本记录结论。
