# Ultra 当前基线审计

> 类型：当前证据基线
> 目的：记录 Ultra 仓库在进入下一阶段前的代码、构建、页面和边界状态。
> 规则：旧计划、旧截图、旧 CI 只能作为历史参考，不能替代本次证据。

## 仓库

- 路径：`C:\Users\Administrator\Desktop\友善\godusevpn ultra`
- 分支：`codex/windows-privacy`
- HEAD：`78f401284d941d2f6e54593e104cb7ca1e26f511`
- origin：`https://github.com/Maoyangui/godusevpn-ultra.git`
- 允许修改：Ultra 仓库、独立 Trial Worker、独立 Home Bridge。
- 禁止修改：`m-ui`、`godusevpn`、`liumeiti.vip-main`、`liumeiti.vip`。
- 工作树：建立本基线时干净。

## 已验证（本次）

- `node site/build.mjs`：通过，产物 146 个文件。
- `node site/tools/demo-smoke.mjs`：通过。
- `node --check site/ultra.js`：通过。
- `git diff --check`：通过。
- GitHub Pages：提交 `7c29fbb` 页面构建通过；提交 `78f4012` 的 CSS 缓存更新正在等待/复核。
- 线上页面：已通过 CUA 打开，桌面首屏可见；导航、产品模块 Tab、浏览器环境 Tab、英文切换均已操作并读回。
- 线上资源：页面已包含 `assets/godusevpn-logo.svg`，CSS 与 JS 使用 `0.02-ultra-rebuild` 缓存参数。

## 页面视觉证据

- CUA 线上桌面截图：本次会话采集，展示 GodUseVPN 深色分层、工作台窗口、渐变 CTA、产品路径和品牌标志。
- 历史多尺寸截图：`C:\Users\Administrator\AppData\Local\Temp\ultra-online-1440.png`、`C:\Users\Administrator\AppData\Local\Temp\ultra-mobile-top.png`；仅作为历史参考，不能替代当前多尺寸复验。
- 下一步必须重新采集 1440、1024、768、390 宽度，并记录无溢出、裁切、低对比、焦点丢失。

## 当前未完成，禁止宣布完成

- 后端 v2 extras capability 校验。
- 档案导入立即清洗 binding、PID、凭据和令牌。
- 加密浏览器档案（DPAPI 或 AES-GCM）及完整性/差异确认。
- Firefox BiDi 与 WebView2 Cookie 读写；全量站点存储。
- 多标签启动和逐项启动/读回结果。
- 完整 WebRTC、Canvas、AudioContext、媒体设备、ClientRects、SpeechVoices、OOPIF、Worker、WebGPU Worker 等运行时闭环。
- 高级字段真实能力边界（RAM、设备名、MAC、物理 CPU/GPU、端口扫描、TLS、同步策略）。
- 独立 QA 依赖清单和 `npm ci` 入口。
- Win10 1809、Win10 19044/22H2、Win11 x64、Win11 ARM64 的当前 Azure/VPS 证据。
- 真实 Chrome/Firefox 代理出口、GPS、生产 claim/refresh 的新证据。

## 下一步门槛

每个阶段完成后必须同时满足：

1. 代码级校验和针对性测试通过；
2. 页面或客户端交互读回与设置一致；
3. 失败、未知、不支持和恢复路径有明确结果；
4. 视觉复核通过，所有新增前端文案短、准确、适合展示；
5. 仓库边界检查通过；
6. 再进入下一阶段。
