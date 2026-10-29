# Win10 22H2 标准用户 GUI 验收记录（2026-10-27）

## 环境

- 镜像：`MicrosoftWindowsDesktop:Windows-10:win10-22h2-pro-g2:19045.6456.251117`
- 系统：Windows 10 Pro 22H2，build `19045.6456`
- 运行用户：`ultraqa`，通过 Bastion 交互会话登录
- WebView2：`156.0.4314.50`
- 包：`0.01-beta-qa.12.10` GUI，SHA-256 `7546e267d0c05c7b34df095edfb78e848a2965c2c89c9a9ce7b5b1fef5f6eb17`
- 后续候选：`0.01-beta-qa.12.11` 已重新打包，源码行为相同并包含本次启动回归测试与兼容性文档；ZIP SHA-256 `664a11b412800cf4188a23a1a52089127757eaf091dacb79b8527229d657be3f`

## 结果

1. 通过 Azure Run Command 以 SYSTEM 身份校验并安装 GUI，安装后目标文件哈希与分片解码后的预期值一致，`SHA256SUMS` 中的 GUI 条目同步更新。
2. 通过受限计划任务以 `ultraqa` 的交互会话、`RunLevel Limited` 启动 GUI。远程桌面显示正常的“先填一条订阅”首页，窗口可见且没有 UAC 凭据弹窗。
3. 进程校验显示 GUI 位于 `C:\Program Files\GodUseVPN Ultra\godusevpn.exe`，运行在交互会话 `5`；服务仍在会话 `0`。未发现 `GodUseVPN.PrivacyHost` 进程。
4. 这证明普通启动路径不会自动调用隐私助手；需要机器策略或浏览器隔离时，仍由隐私页的显式授权入口触发提权。

## 兼容性边界

这次真实验收覆盖 Win10 22H2 Pro 的标准用户、WebView2 已安装、GUI 普通启动和服务共存路径。Windows 10 21H2/22H2 的具体补丁、WebView2 缺失、UAC 策略差异、WFP/设备能力和代理连通性仍需在对应环境单独回归；纯测试不能替代这些系统能力验证。

## 可复现检查

- 启动任务：`GodUseVPN-QA1210-GUI`，用户 `ultraqa`，`Interactive/Limited`。
- 进程检查：`Get-Process godusevpn, godusevpn-svc, GodUseVPN.PrivacyHost`。
- 文件检查：对 `C:\Program Files\GodUseVPN Ultra\godusevpn.exe` 执行 SHA-256，并与 `SHA256SUMS` 的 `godusevpn.exe` 条目比较。
