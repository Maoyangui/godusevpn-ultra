# 免费试用 UI 与接口验收（2026-10-30）

## 视觉核验

- Chrome headless，420×760 与 900×900，浅色/深色主题，使用本地 `web/dist` 和后端返回的可领取状态；beta.6.2 候选包再次复跑。
- 试用卡片显示标题、说明、状态和两颗按钮：`免费试用`、`购买节点`。
- 页面横向尺寸：`scrollWidth=420`、`clientWidth=420`，无横向溢出。
- 卡片改为白色层级、品牌色左边线和轻量图标，按钮在窄窗口保持同一行；状态文本使用小圆点区分信息层级。
- 视觉快照保存在本轮临时验收目录 `C:\Temp\ultra-trial-ui-qa\trial-card-beta62.png` 与 `final-audit-11`，未写入应用目录；实拍检查确认试用卡片无重复长文案、两按钮同排、窄屏无横向溢出。

执行脚本：

```text
node C:\Temp\ultra-trial-ui-qa\trial-card.cjs
```

结果：`card=true`、`buttons=2`、`status=本月可领取`、无横向溢出；beta.6.2 快照正文为“每台设备每月一次，开通后自动加入订阅”，状态与按钮各只出现一次。

## 功能核验

- `go test -tags with_clash_api ./...`（Ultra）通过。
- `go test ./internal/trial ./internal/uiapi ./cmd/godusevpn` 通过。
- `go test ./web ./database/...`（m-ui）通过。
- 后端并发测试确认同一设备同一 UTC 月份只有一个 200，其余为 409，D1 只创建一个试用领取记录。
- 生产回归通过 `https://godusevpn.org/app/api/trial/*`：Cloudflare Worker + GodUseVPN D1 status 200，claim 200 返回真实 `hk.joinvip.vip:2056` 订阅且 URL 可访问；同设备随后 status 为 `already_claimed`、claim 为 409；OPTIONS 为 204，非法设备为 400，恶意 Origin 不获得允许跨域头。
- 生产后端生成 `t-` 加 8 位十六进制短标识，并在 D1 备注记录“来源：佛跳墙内置免费试用；套餐：free”；客户端只发送设备安装标识，永不接收面板令牌。GodUseVPN 后台截图确认面板 API 已就绪、`free` 套餐存在并完成映射。
- `node docs/ultra/qa/browser-profile-ui.cjs C:\Temp\ultra-trial-ui-qa\node_modules\jsdom` 通过，现有浏览器配置与试用卡片改动无回归。
- `ULTRA_QA_SMOKE=1 node docs/ultra/qa/pixel-qa.cjs C:\Users\Administrator\Desktop\友善\m-ui\node_modules\playwright-core C:\Program Files\Google\Chrome\Application\chrome.exe C:\Temp\ultra-trial-ui-qa\pixel` 通过：8 张截图、0 个页面错误、0 个外连请求；隐私中心和检测中心无横向溢出。

## 设计与安全边界

- Windows/Linux 优先使用系统机器标识的本地单向哈希，无法读取时才保存随机安装实例密钥；不读取 MAC、序列号或 UA，也不上传原始机器标识。
- m-ui 只保存 HMAC 后的设备摘要和月份联合主键，管理员 API token 不进入客户端。
- 用户目录文件删除不会在 Windows/Linux 常见场景下换出新设备标识；macOS/Android/iOS 当前仍使用兼容回退，后续可接入 Keychain/Keystore/Secure Enclave 做更强的不可导出密钥绑定。
- Worker 配置可关闭试用、修改试用套餐名称和购买链接；链接只接受 `http://` 或 `https://`。生产默认值为 `free` 与 `https://godusevpn.org/subscription/`。
- 后端返回的订阅地址在客户端再次做协议校验，失败不会写入订阅列表。
