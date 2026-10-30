# GodUseVPN 线上免费试用 D1 回归（2026-10-30）

本次回归验证的是 Cloudflare Worker `godusevpn-trial-api` 版本 `39193d8e-d3b7-4617-8630-4c7b64b5a72e`，路由为 `godusevpn.org/app/api/trial/*`，绑定 D1 `godusevpn-commerce`。测试没有读取或输出订阅令牌、HMAC 密钥或面板令牌。

## 实测结果

| 请求 | 结果 |
| --- | --- |
| `GET /app/api/trial/status` + 新设备标识 | HTTP 200，`status=available`，`plan=free` |
| `POST /app/api/trial/claim` | HTTP 200，`status=claimed`，用户名格式 `t-xxxxxxxx`，购买链接为 `https://godusevpn.org/subscription/` |
| 返回订阅 URL | HTTPS、`hk.joinvip.vip:2056`，实际请求 HTTP 200，`text/plain`，约 6 KB |
| 同一设备再次 `POST claim` | HTTP 409，`status=already_claimed` |
| 同一设备再次 `GET status` | HTTP 200，`status=already_claimed` |
| `OPTIONS /claim` | HTTP 204 |
| 无效设备标识 | HTTP 400，`status=invalid_device` |
| `Origin: https://evil.example` | 不返回 `Access-Control-Allow-Origin` |

Worker 生产变量为 `TRIAL_ENABLED=true`、`TRIAL_PLAN=free`、`TRIAL_PURCHASE_URL=https://godusevpn.org/subscription/`。D1 记录的备注格式为：`来源：佛跳墙内置免费试用；套餐：free；月份：202610`；最近回读的用户名均为 8 位十六进制短标识。

## 安全边界

- D1 只保存 HMAC 后的设备摘要、月份、短标识、订阅 URL、来源备注和时间；客户端不会收到面板 API 令牌。
- 订阅池通过 Worker secret 保存，不写入仓库；Worker 只接受 HTTPS 订阅 URL。
- 设备限制是 `device_hash + UTC month` 联合主键，D1 `INSERT OR IGNORE` 负责并发竞态。
- Ultra 客户端收到新试用 URL 后更新已有内置试用订阅行，删除重复旧缓存，不追加旧链接。
