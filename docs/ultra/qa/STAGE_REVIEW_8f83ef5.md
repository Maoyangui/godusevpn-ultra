# 阶段 1–2 复核：v2 契约与档案安全

## 代码变更

- `internal/browserprofiles/launch_contract.go`：新增 v2 启动信封校验，严格检查 `schemaVersion/config/meta/extras/engine`、未知字段、危险绑定和引擎一致性。
- `cmd/godusevpn/browser_v2_contract.go`：统一返回 capability receipt。
- Windows / 非 Windows `BrowserProfileV2Preview`、`BrowserProfileV2Launch`、`BrowserProfileV2LaunchProxy`：合法但未实现 extras 返回 `status=unsupported`，不启动。
- `web/dist/privacy.js`：前端优先调用 v2；旧运行时没有 v2 方法时明确显示兼容路径提示。档案导出改为 AES-256-GCM；导入先解密、完整性校验，再清洗 binding、服务 PID、代理凭据、面板/订阅令牌、extraArgs，最后进入草稿。
- `docs/ultra/qa/package.json`、`package-lock.json`：QA 依赖可在独立目录执行 `npm ci`。

## 本阶段验证

- `go test ./internal/browserprofiles ./cmd/godusevpn`：通过。
- `go vet ./internal/browserprofiles ./cmd/godusevpn`：通过。
- `go test -tags with_clash_api ./...`：通过。
- `node --check web/dist/privacy.js`：通过。
- `node site/build.mjs`：通过。
- `node site/tools/demo-smoke.mjs`：通过。
- `git diff --check`：通过。
- `Set-Location docs/ultra/qa; npm ci`：通过。
- 八组浏览器 UI fixture：全部通过。
- `browser-archive-crypto.cjs`：AES-GCM 字段、危险字段清洗、篡改拒绝全部通过。

## 仍然明确未完成

- extras 的真实运行时能力仍按 receipt 标记 unsupported；没有伪称启动成功。
- Firefox/WebView2 Cookie、全量站点存储、多标签启动、完整指纹噪音和真实 VPS 矩阵仍未验证。
- 兼容旧运行时的 fallback 只用于旧宿主；新 v2 运行时不会静默丢弃 extras。

## 阶段门槛复核

代码测试、失败状态、未知状态和恢复路径均已复核通过；新增前端文案保持短且说明真实范围。下一阶段只能在此状态上继续，不得把本阶段的 unsupported 当作已完成能力。
