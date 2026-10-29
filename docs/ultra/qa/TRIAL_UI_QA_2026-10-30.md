# 免费试用 UI 与接口验收（2026-10-30）

## 视觉核验

- Chrome headless，420×760 与 900×900，浅色主题，使用本地 `web/dist` 和后端返回的可领取状态。
- 试用卡片显示标题、说明、状态和两颗按钮：`免费试用`、`购买节点`。
- 页面横向尺寸：`scrollWidth=420`、`clientWidth=420`，无横向溢出。
- 卡片改为白色层级、品牌色左边线和轻量图标，按钮在窄窗口保持同一行；状态文本使用小圆点区分信息层级。
- 视觉快照保存在本轮临时验收目录 `C:\Temp\ultra-trial-ui-qa\trial-card.png`，未写入应用目录。

执行脚本：

```text
node C:\Temp\ultra-trial-ui-qa\trial-card.cjs
```

结果：`card=true`、`buttons=2`、`status=本月可领取`、无横向溢出。

## 功能核验

- `go test -tags with_clash_api ./...`（Ultra）通过。
- `go test ./internal/trial ./internal/uiapi ./cmd/godusevpn` 通过。
- `go test ./web ./database/...`（m-ui）通过。
- 后端并发测试确认同一设备同一 UTC 月份只有一个 200，其余为 409，数据库只创建一个 `free trial` 用户。
- `node docs/ultra/qa/browser-profile-ui.cjs C:\Temp\ultra-trial-ui-qa\node_modules\jsdom` 通过，现有浏览器配置与试用卡片改动无回归。
- `ULTRA_QA_SMOKE=1 node docs/ultra/qa/pixel-qa.cjs C:\Users\Administrator\Desktop\友善\m-ui\node_modules\playwright-core C:\Program Files\Google\Chrome\Application\chrome.exe C:\Temp\ultra-trial-ui-qa\pixel` 通过：8 张截图、0 个页面错误、0 个外连请求；隐私中心和检测中心无横向溢出。

## 设计与安全边界

- Windows/Linux 优先使用系统机器标识的本地单向哈希，无法读取时才保存随机安装实例密钥；不读取 MAC、序列号或 UA，也不上传原始机器标识。
- m-ui 只保存 HMAC 后的设备摘要和月份联合主键，管理员 API token 不进入客户端。
- 用户目录文件删除不会在 Windows/Linux 常见场景下换出新设备标识；macOS/Android/iOS 当前仍使用兼容回退，后续可接入 Keychain/Keystore/Secure Enclave 做更强的不可导出密钥绑定。
- 后台设置页可关闭试用、修改试用套餐名称和购买链接；链接只接受 `http://` 或 `https://`。
- 后端返回的订阅地址在客户端再次做协议校验，失败不会写入订阅列表。
