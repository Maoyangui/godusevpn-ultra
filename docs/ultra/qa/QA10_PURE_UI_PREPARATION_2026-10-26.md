# 下一候选纯 UI 回归与发布准备

日期：2026-10-26。此记录只覆盖当前源码的固定 DOM / API mock 与源文件审查；没有构建、上传或发布 QA10，没有运行真实产品、浏览器、剪贴板、系统策略或 VM。冻结 qa.9 的字节和 [官方升级证据](QA9_OFFICIAL_UPGRADE_2026-10-26.md) 均未改变，不能将本页结果外推至该包或真实运行时。

## 本轮通过证据

使用已固定的 JSDOM 依赖加载实际 HTML、翻译和界面源码。下列 16 个 Node 入口均退出 0：

| 入口 | 已检查的主要交互 |
|---|---|
| [browser-start-ui.cjs](browser-start-ui.cjs) | pending 可见且锁定编辑；匹配 token 的取消；旧 DOM / 重复请求隔离；ready 竞争；未知清理保留检查入口；Status 失败；离页取消原 run；旧同步接口兼容 |
| [window-controls-ui.cjs](window-controls-ui.cjs) | 10 类平台 / 自有桥方法可见性；初始最大化状态、标签 / aria / 图标；翻译事件；重复点击；resize 读回；晚旧结果隔离；失败后重试 |
| [browser-identity-ui.cjs](browser-identity-ui.cjs) | Windows 10 / 11 的 10.0.0 / 13.0.0 快捷值；UA 的 Windows NT 10.0 标记；手动 / 原生文本或未知占位保持；视口草稿；晚到预览隔离 |
| [browser-engine-ui.cjs](browser-engine-ui.cjs) | Chrome / Firefox 的已核验版本选择；四环境 × generated / manual / native 共 24 组合；完整字段转发；手动原文；不编造原生 UA；自定义 GPU 明确拒绝；旧 manager 兼容 |
| [browser-saved-profiles-ui.cjs](browser-saved-profiles-ui.cjs) | 创建、载入、重命名、克隆、删除；中文名称；CAS 冲突保留草稿；不持久化代理；取消和过期结果隔离 |
| [module-form-ui.cjs](module-form-ui.cjs) | 设置脏标记及精确保存；订阅保存成功 / 失败 / 取消 / 已移除旧按钮；连续规则编辑合并；复制仅含可见日志；日志失败不复制旧内容 |
| [browser-hardware-ui.cjs](browser-hardware-ui.cjs) | WebView / Chrome CPU 数量字段；编辑使预览失效；保存 / 载入 / 分页草稿；原生模式省略；Firefox 自定义请求在 API 前拒绝；错误数值与旧控件隔离 |
| [browser-profile-ui.cjs](browser-profile-ui.cjs) | 四环境、系统版本 / 尺寸保留；非 Windows 隐藏具名 D3D 预设；手填 GPU；预览后启动使用已审阅配置；独立代理；目录 / 下载 / 库存显式刷新及拒绝路径 |
| [metadata-ui.cjs](metadata-ui.cjs) | 默认隐藏值；七组显式选择；转义；原生 token；取消 / 错误 / 副本回执与旧页面隔离 |
| [clipboard-ui.cjs](clipboard-ui.cjs) | 默认关闭、时长边界、独立设置；仅传配置 ID；JS 不复制秘密；条件清理 / 取消；服务不可用与旧页面隔离 |
| [ipc-identity-ui.cjs](ipc-identity-ui.cjs) | 服务身份未知 / 响应未知状态；隐藏失效保护；不自动发起变更 |
| [diagnostic-ui.cjs](diagnostic-ui.cjs) | 默认最小化；细节需确认；预览转义；同一快照；旧页面隔离 |
| [原有界面回归](../../../browser-extension/ui-tests.cjs) | 导航、确认、帮助、失败隔离、检测、图片工具、导出、时区与观察来源 |
| [扩展面板回归](../../../browser-extension/panel-tests.cjs) | 逐 API 选择；确认取消；共享设置变化使旧选择失效；帮助覆盖 |
| [扩展核心回归](../../../browser-extension/tests.cjs) | 内存浏览器设置 API、控制权与撤销逻辑 |
| [扩展协调器回归](../../../browser-extension/coordinator-tests.cjs) | 共享队列、崩溃记录、孤立持有者释放及逐 API 权限模式 |

以上均未使用真实 Wails / Chrome / Firefox / Windows API。固定 mock 返回值能证明 UI 请求与状态处理，不证明原生协议、权限或浏览器可见值已在 VM 生效。

## CI 与源码发布边界

- [Windows workflow](../../../.github/workflows/windows-beta.yml) 已包含上述 16 个入口及逐项退出码检查；本轮补入先前遗漏的 clipboard-ui.cjs。新增的 browser-start-ui.cjs、window-controls-ui.cjs 已在同一门槛中。这里只核对 workflow 内容，未声称托管 CI 已运行。
- [publication 扫描](../../../tools/audit-publication.cjs) 覆盖 Git tracked 与未忽略的当前文件；它是启发式检查，不能代替人工审查或新公开历史审计。文档完成后重跑的源清单哈希由本轮最终命令回执提供，避免在被扫描文件中写入其自身哈希形成循环依赖。
- 私有传输脚本、临时 URL / 清单、原始云回包和下载材料位于已忽略目录，不进入源发布文件集；包和依赖缓存也未加入该集合。本轮额外检查固定私有传输标识在公开源集中的意外引用，只记录文件 / 行号，不输出疑似值。

## 尚未满足的用户模块

以下仍属于 [H01–H37](../HANDOVER_REQUIREMENTS_2026-10-26.zh-CN.md) 的未完成项，不以本轮通过缩减要求：

| 模块 | 仍需完成或独立验收 |
|---|---|
| 原生 Chrome / Firefox 和独立 WebView | 真实普通用户启动、权限助手、下载 / 版本验证、HTTP / navigator / CH、四环境 / 视口 / 触摸、CPU、iframe / OOPIF、代理出口及关闭 / 租约 / 取消竞争。原生普通构建的未就绪启动门保持；模板重用和原生 GPU 尚未完成 |
| 配置存储与窗口 | Medium 用户下真实 NT 相对句柄读写、ACL、同步、跨窗口 CAS 与恢复；多屏 / DPI、首次显示 / 最大化 / 还原及重载实际几何行为。DOM 不能代替这些系统行为 |
| 其余浏览器参数 | RAM、字体、Canvas / Audio / 媒体 / ClientRects / Voices、网页设备名等尚需真实后端；MAC 属于系统 / 网卡层，不能由网页字符串覆盖冒充。WebGL 字符串、WebGPU、WebRTC、定位和语言需逐项核对作用范围与真实读回 |
| GPS 与系统设置 | 真实 Windows GPS 后端、坐标 / 轨迹 / reset / 冲突恢复仍未实现；语言 / 地区 / 设备名 / MAC、每应用权限及系统策略实际行为按支持范围继续实现或验收，GeoIP 不作为 GPS |
| 数据工具 | VM 中真实 ExifTool 七组字段、新副本与失败恢复；当前文件、剪贴板历史 / 云同步及用户后续复制竞争的真实边界；诊断最小化与授权导出。纯 mock 不等于这些工具已经运行 |
| 网络 / 恢复 | 原隧道回归、按应用 WFP 实际流量、IPv4 / IPv6 / UDP / 回环、服务 / 助手失联、规则未知与独立恢复。已有代理模块网络证据不外推至所有浏览器流量 |
| 最终交付 | 用户要求的其余开源集成、全量计划核对、审计、独立公开仓库 / 历史、中英文说明、精确 v0.01-beta、发行下载复核及临时资源清理，均未被本页标为完成 |
