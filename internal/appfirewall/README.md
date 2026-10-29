# Windows 应用网络限制模块

此模块与 VPN 连接流程、原全局 WFP guard、BFE 服务启停完全独立。不在 init、构造函数或状态读取时修改机器；只有显式 `Apply` / `Remove` 才开启原生 WFP 写事务。失败只返回本项结果，不断开隧道、不清空其他保护。

## 本期范围

- `block_all`：指定 EXE 路径的直接入站/出站连接，IPv4 和 IPv6。
- `block_lan`：该 EXE 的 RFC1918、169.254/16、IPv6 ULA、IPv6 link-local 直接连接。
- `block_discovery`：该 EXE 的 IP 组播与有限广播；入站按本地目的地址、出站按远端目的地址匹配。
- 每条过滤器同时绑定 App ID 和非 loopback 条件；没有全局默认拒绝，没有放行过滤器。
- 回环明确不保护；这也意味着经 localhost 代理转发后可能绕过。不是完整端口扫描检测，不涵盖系统代发的发现、DNS、蓝牙、子进程或其他 EXE。
- 规则绑定路径，不校验程序签名/二进制内容，不是防管理员绕过的安全边界。

## 最重要的仲裁限制

专用子层权重为 65534，原隧道 guard 不受修改。所审 `permitSelf` 使用 `CLEAR_ACTION_RIGHT` 硬放行，可阻止匹配路径上的低权重普通 block 覆盖；其他 TUN、LAN、回环 soft permit 不能仅因子层权重一并归为硬放行，也不能据此断言所有目标流量一定放过或阻断。其他产品也可影响仲裁。

因此报告只使用 `filters_installed`（规则和条件安装证据）与 `partial`，始终附带 `enforcement: not_live_tested`、`loopback: unprotected`。`Result.verified` 仅指规则安装/删除复核，不能在 UI 转译成“已阻断”“全路径保护”。微软的规则说明见 [Filter Arbitration](https://learn.microsoft.com/en-us/windows/win32/fwp/filter-arbitration)。这一限制应在应用规则区域持续显示，而不只是首次操作 toast。

## 持久化与恢复

提供者、子层和过滤器使用本模块独立 GUID，全部 PERSISTENT；非动态会话关闭后规则仍在，没有动态会话寿命冒充持久化。提供者不绑定服务名，不依赖服务自动启动。该持久化行为有 [FwpmProviderAdd0 文档](https://learn.microsoft.com/en-us/windows/win32/api/fwpmu/nf-fwpmu-fwpmprovideradd0)依据。

WFP 持久存储是唯一状态源，过滤器中只保留版本、归属、规则 ID、EXE 路径及模式，没有连接记录、域名或浏览历史。本模块对象 DACL 只授权 SYSTEM/Administrators。元数据与网络规则在同一 WFP 事务内提交，不存在本地 JSON 已写入但规则未装的双存储窗口。

替换规则在同一事务中撤旧、装新；任意添加或提交失败会尝试中止全部事务。进程崩溃由 WFP 结束未提交事务。提交后复核失败返回 `Applied=true`、`Verified=false`，明确要求刷新/检查，绝不声称已经回滚。退出产品不自动删除持久规则；应在应用内逐条移除后卸载，新版卸载器集成尚需单独验收。

删除只识别同时归属本 provider 和 sublayer、元数据合法、规则 ID 与确定性 GUID 一致的对象。无法识别的对象报告警告而不猜测删除；这是故障诊断边界，不能盲目重置整机防火墙。

`Report.unknownFilters` 独立统计枚举到但无法识别归属/元数据的过滤器，不挤入正常 `Rules`，也不靠警告字符串猜测数量。清理和卸载必须在删除已知规则后复读实际报告；若 `unknownFilters > 0`、仍有规则或读取失败，不能宣称清理完成，应保留恢复工具并要求管理员检查。

## 目标与权限边界

只接受本地绝对 EXE 普通文件路径，拒绝 UNC/设备路径/备用数据流和重解析点链；规范化短路径后再核对受保护目标。拒绝本产品、当前 EXE（含文件别名）、已知代理内核、Windows 系统目录内程序。没有无条件全局目标或自定义脚本入口。已提升管理员令牌仍可能被系统策略拒绝；只报告错误，绝不改动原 guard 或 BFE。

## 验证

`go test ./internal/appfirewall -count=1` 使用内存 fake 后端、fake 事务和伪造原生结构，验证提交/回滚/复核失败、目标校验、条件匹配、身份归属、未知过滤器、JSON 与 64 位 ABI；另通过 `go build -gcflags=-m` 核实挂在原生 union 中的指针对象确实逃逸到稳定堆内存，防止 Go 栈搬迁后悬空。测试不写当前机器防火墙。实际规则安装、退出/重启后的持久性、隧道仲裁、IPv4/IPv6 流量、代理和发现协议必须在隔离 Windows VM 中另行验收；通过之前不能提升运行时保护声明。
