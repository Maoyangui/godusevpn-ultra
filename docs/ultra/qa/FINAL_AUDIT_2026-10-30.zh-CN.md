# GodUseVPN Ultra 全量复核回执（2026-10-30）

本回执重新对照两段接手对话及 H01–H41 要求，并把源码、包、UI、Azure Win10 实机和线上试用接口分开判定。通过纯测试或静态检查的项目不会被写成真实运行时通过。

## 已完成并有证据

| 范围 | 证据 |
| --- | --- |
| 浏览器配置交互 | 四种网页环境预设、OS 版本/视口、UA 生成与手改、GPU/WebGL 字段、代理草稿、内核下载/校验/删除、预览失效和启动前复核；DOM QA 全部通过 |
| 隐私中心/检测中心 | 状态分层、未知/有限/计划中不升级、问号帮助、失败和恢复文案、检测快照筛选/导出、键盘与窄屏布局；Edge smoke 像素回归 8 张，0 error、0 outbound |
| 开源集成 | sing-box、WFP/WireGuard 规则、ExifTool、WebView2 SDK/宿主、Chrome/Firefox 下载校验链均有源码入口、固定版本/许可证或哈希说明；对应 Go/Node 测试通过 |
| Win10 实机 | Azure Win10 Enterprise LTSC 2021 64 位 build 19044；安装包哈希、服务 Running/Auto、WebView2 156.0.4314.50、CLI status/diag/privacy report 均实测 |
| 真实订阅/隧道 | 在 VM 内设置测试订阅、刷新 19 节点、connect 退出码 0，状态 `connected`、`TUN=true`、`strict=true`；通过 TUN 访问 ipify 和 ipwho.is，disconnect 后状态恢复 `disconnected` |
| 清理 | VM、OS 磁盘、NIC、公共 IP、专用 NSG、专用 VNET 已删除；资源组只保留原有 Bastion/快照资源；临时 gists 与本地分片已删除 |

## 仍不能标记为完成的项目

| 范围 | 当前事实与原因 |
| --- | --- |
| 真实内置浏览器窗口 | WebView2/宿主文件存在，但宿主启动需要普通用户父进程、认证管道和严格连接；Run Command SYSTEM 会话得到固定 `start_refused`，不能据此声称窗口、Chrome/Firefox 多版本、真实浏览器代理出口通过 |
| Chrome/Firefox 多版本原生启动 | Windows amd64/arm64 普通构建已进入受控 native Start 严格链路；下载/完整树校验、CDP/BiDi 和启动门禁通过。真实进程矩阵、窗口、导航、代理出口、WebRTC/WebGPU/OOPIF/Worker 读回仍未在普通用户 VM 完成 |
| 每浏览器 SOCKS/HTTP/HTTPS 浏览器出口 | 代理配置、relay 和 probe 已实现/测试；本轮只实测 Ultra TUN 出口，未取得普通用户浏览器实际导航抓包证据 |
| GPS/Windows Location Provider | 现有代码是只读观察与恢复边界；没有可证明的系统位置写入、独立读回和冲突恢复闭环 |
| CEF/uBlock | 仓库只有候选/审计记录，没有可运行集成和更新/权限流程 |
| Win10 1809/LTSC 2019/22H2 与 Win11 多版本 | 本轮只真实验收 build 19044；其余版本仍需各自 VM/接口矩阵证据 |
| godusevpn.org 线上免费试用 | m-ui 后端代码、接口和测试已完成，但线上 `https://godusevpn.org/app/api/trial/status` 当前返回 404；Cloudflare 账户可见站点/Workers，但没有可直接运行该 Go/SQLite 面板的已部署服务或明确生产部署凭据，因此不能声称已上线 |

## 结论

源码、UI、包、订阅隧道和 Win10 21H2 实测已形成可复核证据；整体 H01–H41 仍未闭环。发布说明、客户端状态和最终答复必须继续保留上述“待实机/未集成/未上线”边界，不能用说明卡、mock 或静态载荷充数。

