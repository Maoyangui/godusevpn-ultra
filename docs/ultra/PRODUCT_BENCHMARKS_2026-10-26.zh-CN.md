# 功能对标与开源实现映射

本表落实 H31–H33。对标的是具体操作和结果；选用组件须形成实际后端、可编辑 UI、独立读回和恢复。用户已授权自行集成开源工具，无须为常规组件选择重复确认。当前只完成下列明确记录的工作，其余保持未完成。

| 模块 | 对标与官方依据 | 要补齐的操作 | 实现选择与当前证据 |
|---|---|---|---|
| 专用浏览器 | [AdsPower 参数说明](https://help.adspower.com/docs/browser_fingerprint)、[官方 API](https://localapi-doc-en.adspower.com/docs/Awy6Dg) | 配置名、真实内核选择、成组环境、GPU 厂商/设备名称、手填/预设/种子、打开前预览、窗口导航与本地读回 | WebView2 的 WebGL1/2 字符串与 WebGPU 阻断、配置库 API 已接源码；Google Chrome for Testing / Firefox 多版本下载、专用进程、协议配置与读回已接源码，真实 VM 运行仍待验。晚于 qa.9 的下一候选加入规范 UA 与 WebView2 / Chrome 网页 CPU 数量；普通构建的原生启动门槛在验收前保留 |
| 浏览器原生反指纹 | [Brave 的 WebGL/WebGPU 保护说明](https://brave.com/privacy-updates/38-webgl-webgpu-fingerprinting-protections/) | 显示配置覆盖范围，区分名称、能力、渲染输出与性能；分来源验证 | 当前有限 WebGL 字符串层不能当作完整内核保护。后续选择可审计引擎/补丁，继续验证 iframe、Worker、Offscreen 和更新兼容 |
| 程序联网与规则编辑 | [simplewall](https://github.com/henrypp/simplewall)、[Portmaster](https://github.com/safing/portmaster/blob/development/README.md) | 程序列表、方向/协议/地址/端口编辑、回环/LAN/互联网范围、规则预览、命中记录、撤销；首次联网决策单独实现 | 现有独立 WFP 后端只覆盖有限模式，优先扩展它并核对两者的实际源码。GPL-3.0 项目是候选来源，尚未复制其模块或加入第二个常驻防火墙 |
| Windows 隐私 | [privacy.sexy](https://github.com/undergroundwires/privacy.sexy) | 分类、搜索、建议级别、逐项说明、所选更改清单、原值与恢复差异 | 参考透明、可逆的交互。当前上游 AGPL-3.0，不能照搬后省略许可证义务；优先用现有正式 API/固定策略事务实现审查过的动作，不整批执行未知脚本 |
| 文件元数据 | [ExifTool 官方项目](https://github.com/exiftool/exiftool) | 支持格式清单、选择字段、检查/编辑/清理预览、新副本、处理前后比较 | 固定 13.59 / 源码 `2200871d9cef988051d2a99d67df3bda6cbb30a8` 的完整 Windows 配套已审查并集成；1501 文件 / 原许可随包、七组 EXIF 可选。qa.9 普通 Medium 真实 GUI 对两张合成 JPEG / PNG 逐项及全选生成 16 副本，独立源哈希、像素、类别选择与既有目标哨兵核验有限 PASS，见 [实测范围](qa/METADATA_REAL_GUI_VM_2026-10-27.md)；其他格式、XMP / IPTC、实拍与异常矩阵及字段编辑仍待完成，不外推为全格式验收 |
| 浏览数据与日志清理 | [BleachBit](https://github.com/bleachbit/bleachbit) | 先选择类型和时间，再预览具体范围；只处理获准的应用数据，显示结果与未处理原因 | 参考其预览交互。现有日志设置、诊断脱敏与独立浏览器数据边界继续保留；未加入全盘扫描/清理后端，不能把文案当作已清理 |
| GPS / 位置模拟 | [Microsoft 正式位置提供接口](https://learn.microsoft.com/en-us/uwp/api/windows.devices.geolocation.provider.geolocationprovider)、上游实际源码见 GPS 审查 | 坐标/地点/轨迹编辑、精度、范围、独立消费者读回、停止模拟与冲突恢复 | locsim 的 Windows 实现已审查并排除。事务适配器及独立新鲜位置观测校验已有纯测试；真实 provider 尚缺本产品的 Microsoft LAF 授权与受信身份交付，公开 Set/Clear 也不满足现有持久 CAS 合同，见[接入范围](qa/WINDOWS_LOCATION_PROVIDER_SCOPE_2026-10-26.md)。尚不能显示系统位置已改 |
| 恢复与诊断 | 上述工具的可逆配置/预览模式，结合当前 Ultra 事务实现 | 原值→待应用→当前读回、备份健康、外部冲突、逐项恢复、失败保留输入；诊断默认最少信息 | 现有维护租约、备份/CAS、快照预览导出已实现有限范围；真实多用户、崩溃与恢复矩阵继续在临时 VM 验收 |

## 共同 UI 验收

每个已支持操作必须有可见输入或选择、有效值限制、轻量问号、待应用状态、错误后保留草稿、应用后真实来源读回，以及适用的恢复操作。预设与随机配置必须可审阅、可重现，不在编辑时自动改系统。运行时不支持、来源不可信或读回失败要说明具体原因，不能静默丢弃参数。

## 集成与验证次序

1. 关闭当前候选真实安装/卸载缺陷，完成专用浏览器和 GPU 的代码及隔离 VM 读回。
2. 按功能台账扩展系统/网络编辑器和实际后端，补齐缺失的范围及恢复；同步审查选定上游。
3. 集成受控元数据组件及更完整的浏览器引擎/位置后端；每个组件固定版本、依赖、许可证和散列。
4. 对照全部 H01–H37 与逐功能台账验收、审计、发布，再审计下载结果；阶段成果不关闭整体目标。

AdsPower 官方称其指纹修改在定制 Chromium C++ 内核中实现（[技术说明](https://www.adspower.com/blog/how-adspower-builds-browser-fingerprints)）。当前 WebView2 脚本层实现必须如实说明层次，不能宣称具有该闭源内核的等价保护。
