# Ultra 三段对话统一项目基线与执行门槛

这份文件把三段对话中已经确认的目标、边界、问题和验收口径合并为一个执行清单。它只适用于 `godusevpn-ultra`，不授权修改任何其他仓库或原站。

## 归属和边界

- Ultra 客户端、Windows 构建、GitHub Pages 展示站：`godusevpn-ultra`。
- `godusevpn.org`：保留原站、多语言、文章、首页和既有订单/订阅/管理逻辑；Ultra 入口只能通过已约定的桥接入口呈现。
- 现有 `/manage/`：承载 Ultra 试用开关、`free` 套餐、购买链接和跳转链接设置；使用已有面板 API 输入，不增加第二个令牌框。
- Trial Worker / Home Bridge 只能作为独立服务边界；不把 Ultra 代码提交到 `m-ui`、原 `godusevpn`、`liumeiti.vip` 或商业站。
- 不误删、回滚或覆盖 Claude/其他人的提交；不使用失效 remote 发布。

## 三段对话合并后的产品目标

### Ultra 客户端

- Windows 优先，兼容 Win10 1809、Win10 19044/22H2、Win11 x64 和 ARM64；macOS 为目标平台。
- 保留 VPN、连接、订阅、节点、分流、登录、订单和接口行为。
- 浏览器环境支持 Chrome/Firefox 多版本、内核下载与删除、Windows/macOS/Android/iOS 预览、UA/GPU/设备名称手动或随机设置、独立 SOCKS/HTTP/HTTPS 代理和连通性测试。
- 真实打开窗口的参数必须与设置一致；未知、部分支持和宿主不支持必须单独显示。
- 浏览器配置、权限、启动标签页、Cookie、站点存储、指纹策略可导入/导出/恢复；导出档案必须加密，危险字段必须清洗。
- 免费试用调用 `godusevpn.org` 后台 `free` 套餐；服务端保存面板密钥，客户端不得读取面板地址、令牌、D1 或订阅列表；同设备同月复用用户名并获取最新订阅，不重复创建。

### Ultra GitHub Pages 展示站

页面必须是参考 GodUseVPN 原站的产品展示站，而不是单一落地页：

1. 顶部导航、桌面菜单、移动菜单、焦点恢复和 Escape/外部点击关闭；
2. Hero：明确定位、价值句、下载 CTA、进入产品 CTA、工作台视觉示例；
3. 产品路径：范围 → 应用 → 读回 → 恢复，支持 Tab、键盘、aria 和状态反馈；
4. 隐私中心：按作用范围、应用、读回、恢复展示；正文短，详情放问号/展开；
5. 浏览器实验室：环境、内核、UA、GPU、代理、Cookie、标签页、指纹和高级设置的产品化预览；
6. 检测证据中心：可读取性、实际访问、配置状态、运行时读回、恢复状态分开；未知不能冒充保护；
7. 多个可操作章节，学习原站的六层防护、DoH 路径、故障恢复、分流和交互实验室节奏；
8. 开源/发布/排障信任区；文章式 Field Notes；下载、订阅、版本和 SHA256 收束；
9. GodUseVPN 原站标志、暗色分层、渐变、线条、状态面板、进度线、滚动动效和 reduced-motion；
10. 中文/英文母语级文本；所有展示文本短、准确、适合前端；禁止内部 debug、审计编号、后台话术、伪造实时数据。

## 当前已经真实验证的项

- Ultra Pages 已完成结构化展示页重做；`site/build.mjs`、`demo-smoke`、`ultra.js` 语法、`git diff --check` 通过。
- 最新线上页面已核验 GodUseVPN 标志、导航、Hero、产品模块 Tab、浏览器环境 Tab、英文切换、路径和证据筛选。
- 1440、1024、768、390 视口已用 Playwright 检查无横向溢出、无页面错误；交互点击通过。
- 后端 v2 启动信封已严格校验 `schemaVersion/config/meta/extras/engine`，对未实现 extras 返回明确 unsupported 收据。
- 浏览器档案已改为 AES-256-GCM 容器；导入先解密/完整性验证，再清洗 binding、PID、代理凭据、面板/订阅令牌和 extraArgs。
- 独立 `docs/ultra/qa` 已具备 `package.json`/`package-lock.json`，八组 DOM fixture 和档案加密回归通过。
- Go 单元测试、vet、带 `with_clash_api` 全量测试通过；GitHub Pages 构建通过。

## 仍必须完成并逐项验收

- v2 extras 的真实运行时适配：多标签、Cookie 全宿主、Firefox BiDi、WebView2、站点存储、代理出口、WebRTC、GPS、Canvas/Audio/媒体/ClientRects/SpeechVoices 噪音、OOPIF/Worker/WebGPU Worker。
- 高级字段真实能力边界：CPU/RAM/设备名/MAC/端口扫描/TLS/同步策略；宿主不支持必须显示为保存或未知。
- 每个隐私能力具备配置、应用、读回、差异、错误和恢复状态。
- 真实 Azure/VPS 重新测试 Win10 1809、Win10 19044/22H2、Win11 x64、Win11 ARM64、Chrome、Firefox、WebView2、普通用户、管理员助手、UAC、服务身份；旧截图/旧 CI 不计入新证据。
- 生产试用 claim/refresh、每设备每月一次、同用户名复用、URL 替换、并发/限流、CSRF、Secure/SameSite Cookie 和 healthz 脱敏的端到端证据。
- README 中英文简洁详细；构建包全部小写；版本从 0.01 升到 0.02；移除会干扰最新识别的 beta6.2；发布后重新检查 Release、Pages、原站入口和后台订单流程。

## 每一步的硬门槛

1. 先读代码并明确作用范围；
2. 实现后跑针对性测试、全量相关测试和静态检查；
3. 读回设置、失败、未知、冲突和恢复结果；
4. 对所有受影响页面逐尺寸截图并检查层级、对比度、裁切、溢出、焦点和文本；
5. 核对仓库边界、remote、提交作者和未跟踪文件；
6. 任何一项不符合即废弃该步改动，修正后重测；
7. 只有通过本步审计，才能进入下一阶段。
