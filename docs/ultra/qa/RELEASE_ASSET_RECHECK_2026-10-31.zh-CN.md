# v0.02 发布资产复核（2026-10-31）

这份记录只对应当前发布资产，不把历史 QA 包当成当前版本。

- 源码提交：以包内 `ULTRA_PACKAGE.json` 的 `sourceCommit` 为准
- 发布标签：`v0.02`
- 资产：`godusevpn-ultra-0.02-windows-amd64.zip`
- ZIP SHA-256：以 Release 资产 digest 和同名 `.zip.sha256` sidecar 为准
- 发布页：<https://github.com/Maoyangui/godusevpn-ultra/releases/tag/v0.02>
- 展示页：<https://maoyangui.github.io/godusevpn-ultra/>

## 已复核

- ZIP sidecar 与实际 ZIP 字节一致。
- `ULTRA_PACKAGE.json` 为 `0.02`、`amd64`、`sourceDirty=false`。
- 三个 PE 为 amd64，文件版本为 `0.2.0.0`，产品版本为 `0.02`。
- ZIP 成员、PE 资源、前端资产和清单通过 `audit-built-package.cjs`、`verify-ultra-package.cjs`、75 项包测试、ZIP CRC 和公开发布扫描。
- `npm audit --package-lock-only --ignore-scripts` 在两个 QA 依赖目录均为 0 漏洞。
- 发布说明中的旧提交、占位哈希和错误的 `free` 转义已移除。

## 仍需单独验收

该包是未签名的 amd64 预览包。清单中的 `runtimeVersionVerified=false`、浏览器宿主、位置消费者和元数据工具的 `vmValidated=false` 保持真实状态。真实 Chrome/Firefox 窗口、代理出口、GPS provider、Cookie 读写、ARM64 和完整 Windows 版本矩阵不能由包审计或展示页动画替代。

`docs/ultra/qa/` 中带有 `0.01-beta-*`、`0.1.0-ultra.1` 或旧 SHA 的文件属于历史测试记录或固定测试脚本；它们不代表当前 v0.02 资产。当前包以本页、`ULTRA_PACKAGE.json`、`.zip.sha256` 和 Release 资产为准。
