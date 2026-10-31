# Ultra 展示站最终视觉与交互复核

- 发布提交：`c834223`（页面改动包含 `8f4e191`、`c9f3ecd`）
- GitHub Pages：`main` 构建 `41158062344` 成功并完成部署。
- 视口：1440、1024、768、390。
- 检查：无横向溢出、无页面异常、英文状态无中文正文、Pad/手机导航可用。
- 线上交互：工作台 Privacy/Browser/Evidence Tab、基础连接 Rule/Global/Direct、VPN 路径测试、DNS 四步测试、保护层级 Tab、故障恢复四步、浏览器环境 Tab、证据筛选均已操作并读回状态变化。
- 动效：工作台自动轮换（遵守 reduced-motion）、首屏悬停立体响应、Tab 指示条、VPN 路径逐节点 pulse、DNS 四步状态推进、保护层级切换、恢复步骤切换、滚动 reveal。
- 文案：中文和英文状态分别检查；所有示例标注界面示例/运行时读回/待验证，未把示例数据写成真实连接或出口。
- 页面结构：基础连接 → 产品路径 → 隐私中心 → 加密 DNS → 防护层级 → 故障恢复 → 浏览器实验室 → 检测证据 → Field Notes → 下载订阅。
- 资源：GodUseVPN 标志和 CSS/JS 使用新的缓存参数；线上不再显示旧的 `nav.vpn` 内部键名。桌面导航保留八个产品入口，DNS 与防护层级可直接发现。
- 线上证据：`https://maoyangui.github.io/godusevpn-ultra/?v=c834223`；英文页 `?lang=en&v=c834223` 已在四个视口完成结构、溢出、动态文案和演示回归。

未完成能力仍按项目基线标记：真实 VPS 浏览器矩阵、Firefox/WebView2 Cookie、完整运行时指纹和 v2 extras 宿主适配，不以展示页动画代替实机证据。
