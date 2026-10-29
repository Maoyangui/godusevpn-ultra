# MV3 真实浏览器设置往返：有限验收

2026-10-19。在当前开发宿主的系统 Edge **155.0.4283.62**、Playwright **1.62.1** 中完成，**不是 VPS GUI、用户原配置或所有浏览器验收**。

脚本：[mv3-edge-live-qa.cjs](mv3-edge-live-qa.cjs)。本次结果：[MV3_EDGE_INTEGRATION_RESULT.json](MV3_EDGE_INTEGRATION_RESULT.json)。脚本只使用已经存在的绝对 Playwright 模块目录和系统 `msedge.exe`，没有安装依赖。结果文件以 `CREATE_NEW` 写入；存在时不再启动测试。没有修改产品扩展源码。

## 实际通过的范围

真实 Manifest V3 service worker 已载入。本次从产品 `panel.html` 内调用原 `Client`，经 `runtime.sendMessage` 到原后台 `Coordinator` 和 `Engine`；没有替换、mock 或拦截设置 API。

| 项目 | 独立临时配置中的读回 | 恢复证据 |
| --- | --- | --- |
| WebRTC IP handling | `default` → `disable_non_proxied_udp`，控制权为本扩展 | 清除后 `default`、可请求控制，own journal 记录消失 |
| network prediction | `true` → `false`，控制权为本扩展 | 清除后 `true`、可请求控制，记录消失 |
| third-party cookies | `true` → `false`，控制权为本扩展 | 清除后 `true`、可请求控制，记录消失 |
| 通知权限 | 对 `https://example.invalid` 的 API 抽样值 `ask` → `block` → `ask` | 清除获得回执，抽样回到初值、记录消失；产品仍返回 `verified=false`，不宣称所有内容权限规则均已验证撤销 |
| 共享后台队列 | 同时提交 `apply → restore → apply → report → restore` 五个请求，全部获得成功回执；中间 report 读到 active | 最后值回到初值，无本扩展控制、最终全部 managed 记录为空 |

每次 privacy 应用均核对实际值、`controlled_by_this_extension`、active 记录及记录中的原始值；恢复使用产品原 `clear` 流程。已安装、set 返回或记录存在均没有单独作为通过依据。本次没有触发企业策略/不支持分支，其拒绝与 unknown 行为不能据此宣称实测通过。

## 隔离与边界

- 只建立一个随机命名的全新 TEMP profile；从未提供或读取现有用户浏览器 profile。profile 保留，没有递归删除、ACL 修改或未知目录清理。
- 仅打开扩展页面及浏览器空白页；`example.invalid` 只作设置 API 参数，没有导航、请求网站、创建 WebRTC 连接、发送 STUN、读取设备或运行本地指纹自测。
- 启动参数关闭背景联网、组件更新、同步等开发测试能力，并在 Playwright 页面路由拒绝非扩展/空白请求。本次监听后观察到 4 个本地扩展资源请求，其他被拦截请求为 0。**这不是整个 Edge 进程的抓包或零外联证明**，也不覆盖监听建立前或浏览器自身后台请求。
- 没有修改 Windows/HKCU/HKLM 策略、安装软件、启动 VPN 服务或控制已有浏览器。设置变化仅位于这个新建且保留的临时 profile。
- 证明的是设置 API 和后台协调器的真实往返；没有证明 WebRTC 所有实际流量不泄露、Cookie 的所有例外消失、所有网站权限生效、MV3 重启/崩溃恢复或其他扩展竞争行为。

[Playwright 官方当前说明](https://playwright.dev/docs/chrome-extensions)警告 Chrome/Edge 的侧载命令行能力可能不可用。本次实际系统 Edge 正常加载了扩展，未修改全局 Developer Mode、使用绕过安全限制的机制或下载替代浏览器。其他版本若拒绝，脚本记 `not_tested`，不会自动改环境。浏览器设置仍需核验控制权；[privacy 官方文档](https://developer.chrome.com/docs/extensions/reference/api/privacy)说明企业策略或其他扩展可能覆盖成功的 set 调用。

脚本 SHA256：`942d0f7529695acc8fc63b37b719a3435549cfe703785545c26db06ec375d230`。结果保存了本次扩展 manifest/core/background/panel 文件的 SHA256，未包含原 profile、账号、设备标识或真实网络地址。
