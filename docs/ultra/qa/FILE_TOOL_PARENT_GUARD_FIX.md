# 文件工具父目录验证与诊断导出复核

日期：2026-10-19。仅 Ultra 副本；没有运行产品、UAC、实际策略或真实网络路径。

## 已确认问题与最小修复

独立检查新诊断 GUI/export 时，发现公共 `privacydata.WriteNewLocal` 进入原 `validatePath` 后，会先对完整叶路径做 `Lstat`，之后才向上检查祖先重解析点。已存在的父级 junction 因而可能先被解析。诊断返回内容虽已严格绑定预览字节，这种路径检查顺序仍不满足“处理前拒绝已有重定向”的边界。根任务先在公共报告保存入口加了父目录 pin，随后授权将图像工具的同类路径统一处理。

最终实现：

- `internal/privacydata/paths.go` 的 `pinValidatedPath` 先做无 I/O 的路径语法检查，再借用 `localstate.AcquireDirectory(parent,false)` 从卷根向下打开并验证父级，取得只读 lease 后才检查文件叶；普通 `validatePath` 也走此入口。失败不创建父目录。
- Windows 源文件 opener 在父级 lease 内执行路径检查、`CreateFile(OPEN_REPARSE_POINT)` 与最终句柄路径验证；之后读取始终使用已核对的句柄。
- Windows 目的文件 opener 在同样的父级 lease 内校验和 `CREATE_NEW`，验证实际句柄；不覆盖已有文件。
- 图片清理及报告写入持有外层父级 lease 直到输出完成和关闭；内部 opener 仍独立验证，以覆盖直接使用 opener 的调用者。公共 `WriteNewLocal` 原额外 wrapper 已折叠进共享写入实现。GUI 检测报告现有外层只读 lease 暂保留，双 lease 在生产 TEMP 保存测试中兼容。
- Windows 文件工具现在明确只接受可确认的本地固定盘目录；UNC、设备路径、映射/不明盘及已有重解析点拒绝。不会自动换位置或移动用户数据。输出错误使用文件工具说明，不把恢复 journal 术语当用户保存失败原因。

用户保存提示：请选择固定本地磁盘中的普通子目录，例如自行建立的报告文件夹。本批暂不支持 USB/移动盘、映射盘、SUBST、网络目录或重解析目录，也不支持直接选择卷根（例如 `C:\report.json`）；这种情况请重新选择本地子目录，不会自动改选或移动文件。

## 运行的定向证据

```text
go test ./internal/privacydata ./internal/localstate ./cmd/godusevpn -count=1
```

三包通过。新增 Windows TEMP 测试仅在一个新建临时根中创建 sibling junction 与 target：

1. 报告保存拒绝 junction 父级，target 没有新文件、原哨兵内容保持一致，随后普通本地保存仍通过。
2. 图像 Inspect、Clean 的来源和目的、直接目的 opener 均拒绝已有 junction；目标 PNG 与本地源图未改、没有新输出。随后普通本地图片清理仍通过。
3. 注入输出创建回调，在创建前实际尝试取得父目录 `GENERIC_WRITE` 句柄，得到 sharing violation，证明外层只读 lease 在生产创建回调期间仍被持有。正常图片与报告输出均完成。
4. 原 JPEG/PNG 元数据块、像素、ICC、损坏输入、已有文件和失败清理回归继续通过；原报告私有 ACL/no-overwrite 及 GUI 预览字节保存回归继续通过。

`localstate` 既有纯内存测试另证明 UNC/设备/不明映射不会进入文件打开，且遇到 reparse 后不探测后代。TEMP 集成仅证明本地重解析目录拒绝和目标未写入，没有用真实远程目标检测查询或网络包；不要把本地测试写成全进程零外联或抵御恶意管理员/内核/DOS 映射主动突变的保证。原 C04 写 journal 的剩余并发命名空间窗口不由本修复关闭。

## 诊断 UI/export 的独立只读结论

`cmd/godusevpn/diagnostics.go` 的大小/文件名/hash 校验先于保存框；期望 hash 绑定当前预览，保存使用返回 ZIP 的同一字节，不重新采集。取消不写，已有文件不覆盖，失败显示错误。`app.js` 的选项默认关闭、详情需明确确认，预览文本经过转义，跨页前后有操作世代检查。原 `diagnostic-ui.cjs` 与 Go 绑定 fake 通过；没有新增确认的内容一致性或失败重试阻塞。路径顺序问题已按上文修复。

另向根任务报告 P3 覆盖建议：新诊断 modal 的保存/取消按钮和文件展开项没有各自轻问号；若严格按每个新增子操作旁均有帮助的要求，应在根任务 UI 中补齐。没有自行修改诊断产品代码。

后续根任务已补 `diagnosticDialogHelp`、选择/预览/取消与文件展开帮助、dialog role/label。`diagnostic-pixel-qa.cjs` 仅加载固定 mock 检查诊断弹层，覆盖 420/900 深浅四配置：选择页 7 个帮助、敏感项未确认不可提交、预览内容内部滚动、窄屏全部文件展开后整体滚动、footer 可达与提交帮助的键盘访问。12 张选择/审阅/底部截图位于 `docs/ultra/screenshots/diagnostic-modals/`，没有保存 ZIP、打开原生文件选择或读取实际诊断。实际读屏、原生窗口和其他缩放仍待验。

## 旧 QA fixtures 同步

- `browser-extension/README.md` 已准确说明本地 MV3 队列、主动 origin 存储、正式逐 API 模式与真实生命周期待验；根 README 未改。
- `audit-qa.cjs`、`pixel-qa.cjs` 与桌面 DOM fixture 更新为冻结的 Snapshot/Preview API、明确助手状态与浏览器标签分类；旧无参导出不再被当作成功 fixture。
- 两个像素脚本支持 `ULTRA_QA_SMOKE=1`，只跑 420 浅色；默认仍保留 420/900 深浅四配置。此次分别生成 5 张、8 张，零页面异常和外部页面路由请求；未重复前批 28 张。
- 截图/结果在 `docs/ultra/screenshots/followup-fixtures/`；桌面 DOM 与诊断 DOM mock 均通过。浏览器后台参数不代表整个进程绝对零外联。
