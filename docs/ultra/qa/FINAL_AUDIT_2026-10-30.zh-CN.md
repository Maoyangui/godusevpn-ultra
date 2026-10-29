# GodUseVPN Ultra 全量复核回执（2026-10-30）

本回执重新对照两段接手对话及 H01–H41 要求，并把源码、包、UI、Azure Win10 实机和线上试用接口分开判定。通过纯测试或静态检查的项目不会被写成真实运行时通过。

## 已完成并有证据

| 范围 | 证据 |
| --- | --- |
| 浏览器配置交互 | 四种网页环境预设、OS 版本/视口、UA 生成与手改、GPU/WebGL 字段、代理草稿、内核下载/校验/删除、预览失效和启动前复核；DOM QA 全部通过 |
| 隐私中心/检测中心 | 状态分层、未知/有限/计划中不升级、减少重复帮助标记、紧凑本地时间、页内错误反馈、检测快照筛选/导出、键盘与窄屏布局；最终 Edge smoke 像素回归 8 张，0 error、0 outbound；截图目录 `C:\\Temp\\ultra-trial-ui-qa\\final-audit-9` |
| 开源集成 | sing-box、WFP/WireGuard 规则、ExifTool、WebView2 SDK/宿主、Chrome/Firefox 下载校验链均有源码入口、固定版本/许可证或哈希说明；对应 Go/Node 测试通过 |
| Win10 实机 | Azure 临时资源组 `ultra-browser-qa-20261030` 的 Windows 10 Enterprise LTSC 2021 x64（VM build 19044）；实机运行包 SHA-256 `23001359a412fb7a04ca8a61624114d95725ef8cf353a8bac5e92c336f443991`、服务 Running/Auto、WebView2 156.0.4314.50、CLI status/diag/privacy report 均实测。最新现代 UI beta.6.1 发布包 SHA-256 为 `0f2bc0742459b04eefe9084ff8294adc7b5abf950e50e1d88823724df6bf0019`，其 UI/包审计另行通过，未冒充为本轮 VM 运行证据。 |
| 真实订阅/隧道 | 在该 VM 内设置用户提供的测试订阅并刷新 18 个节点，connect 退出码 0，状态 `connected`、`TUN=true`、`strict=true`；通过 TUN 访问 ipify 和 ipwho.is 得到同一出口 `104.28.222.43`（Singapore），disconnect 后状态恢复 `disconnected` |
| 清理 | 临时资源组 `ultra-browser-qa-20261030` 已在 Azure 通知中确认删除，资源组列表不再显示；其中 VM、Bastion、OS 磁盘、NIC、公共 IP、NSG、VNET 均随组删除，其他原有资源未触碰；临时 gists 与本地分片已删除 |

## 仍不能标记为完成的项目

| 范围 | 当前事实与原因 |
| --- | --- |
| 真实内置浏览器窗口 | 普通构建已进入受控 native Start 严格链路；本轮 VM 的 Azure Run Command SYSTEM 会话得到固定 `start_refused`。Developer Bastion 已部署，但 Bastion 表单在提交后反复回到 `Value is required`，Windows RDP 则停在本地不受信任证书确认页；因此没有把窗口、导航或代理出口写成通过 |
| Chrome/Firefox 多版本原生启动 | Windows amd64/arm64 普通构建已进入受控 native Start 严格链路；下载/完整树校验、CDP/BiDi 和启动门禁通过。真实进程矩阵、窗口、导航、代理出口、WebRTC/WebGPU/OOPIF/Worker 读回仍未在普通用户 VM 完成 |
| 每浏览器 SOCKS/HTTP/HTTPS 浏览器出口 | 代理配置、relay 和 probe 已实现/测试；本轮只实测 Ultra TUN 出口，未取得普通用户浏览器实际导航抓包证据 |
| GPS/Windows Location Provider | Win10 build 19044 低于 GeolocationProvider 23504 门禁，UI 正确显示不可用；Win11 只提供“可能支持”检查。现有代码是只读观察与恢复边界，没有可证明的系统位置写入、独立读回和冲突恢复闭环 |
| CEF/uBlock | 仓库只有候选/审计记录，没有可运行集成和更新/权限流程 |
| Win10 1809/LTSC 2019/22H2 与 Win11 多版本 | 本轮只真实验收 build 19044；其余版本仍需各自 VM/接口矩阵证据 |
| godusevpn.org 线上免费试用 | 已通过 Cloudflare Worker 路由 `godusevpn.org/app/api/trial/*` → 站点服务端生产网关（Vercel `liumeiti.vip`）上线；服务端只保存设备 HMAC 摘要并以 Redis 月份键限额，再由服务端调用面板创建 `free` 用户，客户端不接触令牌。实测 status 200、claim 200 返回真实 `hk.joinvip.vip:2056` 订阅、同设备再次 status 为 `already_claimed`、再次 claim 为 HTTP 409，OPTIONS 为 204，恶意 Origin 不返回允许跨域头；测试用户备注带“来源：佛跳墙内置免费试用”。 |

## 结论

源码、UI、包、订阅隧道、生产试用网关和 Win10 21H2 实测已形成可复核证据；整体 H01–H41 仍未闭环。发布说明、客户端状态和最终答复必须继续保留上述“待实机/未集成”边界，不能用说明卡、mock 或静态载荷充数。

## 视觉标准复核（现代 App UI）

2026-10-30 在 `6ad868e` 与 `80567db` 后重新执行视觉回归：检测中心时间统一为本地短格式，隐私页取消无意义的重复问号提示，移动窄屏标签改为可横向滚动的单行导航，错误提示上移并限制宽度与换行，避免覆盖内容卡片。随后修正浏览器硬件/启动 DOM fixture 的 `canLaunch` 契约并重新执行，浏览器内核、硬件、启动取消竞态和配置 UI 回归均通过；这些仍是隔离 DOM 测试，不升级为原生 VM 证据。

- DOM QA：浏览器环境、UA/GPU、代理草稿、内核管理、错误转义、过期快照与未支持入口全部通过。
- 像素 QA：420px 窄屏与完整审计组合共 8 张截图，`errors=[]`、`outboundRequests=[]`；beta.6.1 发布包重新执行 ZIP/PE/前端资产审计，payload 1997、ZIP 成员 1998，SHA-256 与 GitHub Release sidecar 一致。
- 视觉结论只代表布局、层级、可读性和交互反馈通过；不升级为浏览器宿主、代理出口或 GPS 运行时通过。

