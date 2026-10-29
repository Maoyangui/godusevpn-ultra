# U01/U02：地区证据与 CLI 日志作用域修复

日期：2026-10-19。仅修改 Ultra 副本的检测证据与 CLI 输出边界；未修改 app.js、i18n、品牌、机器 store、现有审计报告或原项目。本次不处理 U06、C04、导出预览交互或其他已登记缺口。

## U01：无效地区格式不能成为 observed 事实

此前 LocaleName 通过长度/UTF-16 读取后，任何非空值都会成为 observed 摘要；空值则整项消失。现在：

- `internal/detection/locale.go` 在调用原生校验前限制为有界 ASCII 名称，不接受路径、空白、控制字符、NUL 或过长内容；同样检查规范名输出。
- `locale_windows.go` 使用本地 `IsValidLocaleName` 检查受支持名称，再用 `GetLocaleInfoEx(LOCALE_SNAME | LOCALE_NOUSEROVERRIDE)` 取得规范名。不使用可能回退成其他地区的 `ResolveLocaleName`，不查询在线语言服务，也不读取默认地区。
- Windows `ReadFacts` 先验证 LocaleName；`Build` 再验证传入 Facts.Locale，避免直接构造/旧事实输入绕过校验。
- 无效、空值或本地校验接口不可用时，仍保留 locale finding，状态为 unknown，文案为“本次未取得有效地区格式；不表示其他应用无法读取。”不在摘要、证据、警告或 JSON 中回显该无效原值。
- 有效名称保留为本地配置证据，使用规范名；继续明确“地区格式不等于浏览器语言或全部系统语言”。没有声称得到了浏览器 Accept-Language 或实际页面返回值。

非 Windows 分支没有 Windows NLS 证据，返回未知，不凭语法猜测 Windows 当前地区。

## U02：CLI 用户日志不能冒充机器备份检查

`privacy report` 仍只读取当前观察账户的用户 Manager；可以只读看到固定机器策略值，但不访问机器 journal、不启动/提权助手、不改变机器策略或网络。`privacy detection` 使用相同作用域限定。

两项 JSON 保留原有顶层字段、嵌套层级、布尔值与现有含义，新增：

```json
{
  "backupScope": "current_process_user",
  "managedScope": "current_process_user_journal",
  "machineJournalChecked": false,
  "machineJournalStatus": "not_inspected"
}
```

`backupHealthy` 仍是本次用户日志检查结果；新增字段明确它不是整机备份健康。单个控件 `managed:false` 仍是当前用户 journal 没有该归属记录，不能解释为独立机器 journal 未管理该项。没有新增 machineBackupHealthy=false 一类会把未检查说成不健康的布尔值。

输出还增加短提示：“管理归属与备份健康仅来自当前观察账户日志；机器日志未检查。”检测输出也携带这条限定。用户日志为空、损坏或读取失败时，scope 与 not_inspected 仍保留；不会因此推断机器日志不存在、健康或可恢复。

字段仅由 CLI 包装输出增加，`privacy.Report`、机器 RPC 和 GUI 合并模型未改，避免错误改变助手协议或 GUI 已合并报告的作用域。旧宽松 JSON 消费者仍可直接解码原 report/detection 类型；封闭白名单消费者需允许这些新增字段，不能把新增说明当作机器检查证据。

`current_process_user` 指 CLI 当前进程账户；SYSTEM/替代管理员运行不代表桌面用户。既有管理员观察/助手标签缺口 A15 不因本修复而关闭。

## 无副作用验证

执行 `go test ./internal/detection ./internal/cli -count=1` 已通过。新增六项测试：

1. fake 校验器：非法字符/长度输入不会进入校验器，非法规范名输出也拒绝。
2. 空/语义无效/带合成哨兵的地区值：保留 unknown finding 和设置引导，序列化中不出现原值或哨兵。
3. Windows NLS：只传入合成的 zh-CN、en-us、de-DE 名称，确认有效性与规范名；未注册/无效合成名称拒绝。没有读取真实 LocaleName、默认地区或其他用户资料。
4. CLI fake report：健康/异常两种用户备份状态都带明确 scope，机器控件的原配置值/managed 布尔值不改变，旧 report 类型仍可解码；原 warnings 切片不被修改。
5. CLI 空报告/失败读取：错误结果原样保留，machineJournalChecked 始终 false、状态 not_inspected，不产生机器健康布尔值。
6. CLI detection：同样保留机器未检查限定与短提示，旧 detection 类型可解码。

这些测试只使用传入字符串、fake report/回调、内存 JSON 与提供名称的本地 NLS 校验；没有运行实际产品 CLI、真实注册表/策略/journal 读取、服务、助手、网络或设备采集。

最终 `go vet ./internal/detection ./internal/cli` 通过。并行编辑 privacy/store_windows.go 期间曾因 unused os import 短暂编译失败，文件完成后已重跑；未将初次失败计为通过。

## 实机限制

尚未在隔离 Windows 修改 LocaleName、切换用户/地区或注册自定义 locale，也没有用真实损坏 journal 运行产品 CLI。NLS 合成名称通过不代表真实注册表损坏场景、Windows 自定义地区或浏览器语言已验收。后续应在有恢复基线的合成配置环境完成这些范围；不得为验证而读取用户真实订阅、机器日志或浏览数据。
