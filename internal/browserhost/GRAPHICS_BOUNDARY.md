# GPU 配置与实际生效范围

日期：2026-10-26。本目录的配置校验和 fake 读回测试不等于真实 WebView2、GPU 或 VPS 验收。实际宿主位于 `browser-host`，可信启动与租约位于 `cmd/godusevpn`；只有宿主完成预导航强制与实际读回后才可以返回 ready。

| 层次 | 本批请求 | 必须保留的边界 |
|---|---|---|
| Windows/驱动/原生 GPU | 不提供改名 | 实际适配器、驱动、设备 ID、硬件能力及原生应用观察不因字符串模板改变 |
| 网页 WebGL 元数据 | native 或有限 vendor/renderer 显示字符串；支持成对模板、手填和固定 seed | 仅影响宿主明确覆盖的 WebGL 查询，不代表渲染图像、扩展、limits、性能或硬件已变成模板设备 |
| WebGPU adapter/device | 原生或显式阻断网页 API | 不提供 adapter.info 的 vendor/architecture/device/description 伪造；修改 WebGL 不证明 WebGPU 信息已改变 |

JSON 示例：

```json
{
  "graphics": {
    "mode": "webgl-metadata",
    "presetId": "intel-uhd-620-d3d11",
    "vendor": "Google Inc. (Intel)",
    "renderer": "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)"
  },
  "webgpu": "disabled"
}
```

旧配置省略 `graphics` 时保留原生行为；显式 `{"mode":"native"}` 不接受其余非空元数据。顶层 `webgpu:"runtime-default"` 表示原生，`"disabled"` 表示独立阻断请求。metadata 模式必须请求 disabled，且不能同时关闭 WebGL。当前 metadata 请求只允许 `webview2` 引擎。

`presetId:"manual"` 才允许手填：vendor 为 1–128 字节、renderer 为 1–256 字节，仅 ASCII 可打印字符，不允许前后空白或控制字符。命名模板必须与 `GraphicsPresets()` 的两字段完全匹配。值只能经 JSON 编码传给固定宿主实现，不得作为 JavaScript 源码、命令行、路径或额外 Chromium flags 拼接。未知字段和重复字段由严格 profile 解码器拒绝。

`RandomizedProfilePreview` 保留已选权限与其余主机选项，只有用户已选 metadata 模式才按固定 seed 选择成对 GPU 字符串；默认 native 不会因为随机区域/窗口尺寸而偷偷启用 GPU 覆盖。这些模板只用于显示字符串，不声称是与真实硬件能力匹配的完整设备仿真。

## 宿主强制与读回

`GraphicsRequirements` 是要求，不是完成回执。宿主必须在网页脚本及外部导航前安装固定实现，并在主 frame、同源子 frame、跨源子 frame 中分别读取 WebGL1/2 的真实 context；metadata 模式须核对 `WEBGL_debug_renderer_info` 的两个 unmasked 字符串。支持 OffscreenCanvas 的 Window 也应独立复核同类 WebGL context。

WebGPU disabled 须实际检查每类 frame 的网页 API 不可用；worker、shared worker、service worker 与 OffscreenCanvas 不能被普通页面测试偷偷算作覆盖。当前有限宿主方案是在执行前阻断 worker 构造、注册和未获准目标，防止未设置的 worker 暴露另一组 GPU 数据。未来若允许 worker，必须另做其真实设置和独立读回；不能只删掉阻断条件。

任何安装失败、读回不一致、未知目标或后续覆盖缺失必须保持/关闭导航通道。`ValidateGraphicsReadback` 仅核对可信宿主内部采集数据的完整性、profile/runtime 关联及上述要求；它不认证 IPC，不能接受前端传来的 verified 布尔值来开启保护。预览继续返回 applied=false、canLaunch=false，并明确仍需 runtime readback。

## 官方及上游依据

- [AdsPower 官方 Local API](https://localapi-doc-en.adspower.com/docs/Awy6Dg) 把 WebGL vendor/renderer 配置和 WebGPU 模式分开。这是界面/参数参考，不是对其实现、安全性或本项目保护效果的背书。
- [Khronos 的 WEBGL_debug_renderer_info 扩展](https://registry.khronos.org/webgl/extensions/WEBGL_debug_renderer_info/) 定义两个 unmasked 查询常量，并涉及 Window 与 Worker。普通 WebGL VENDOR/RENDERER 的通用文本不能替代这些查询的读回。
- [Microsoft 的文档创建脚本接口](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2.addscripttoexecuteondocumentcreatedasync) 说明脚本注入与未来文档/子 frame 导航的时序；它并未自动提供 worker GPU 覆盖保证。
- [CDP Emulation 上游协议](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Emulation.pdl) 和 [SystemInfo](https://chromedevtools.github.io/devtools-protocol/tot/SystemInfo/) 是本次核对的协议依据。本次未发现可用来重命名真实硬件的 GPU 字符串 setter，不能将普通 CDP 仿真宣传为真实 GPU 改名。
- [GPUWeb 的 GPUAdapterInfo 定义](https://gpuweb.github.io/types/interfaces/GPUAdapterInfo.html) 将 vendor、architecture、device、description 分为独立只读信息；它们与 WebGL renderer 字符串是不同的观察面。

本批不接收任意 GPU testing flags、不修改驱动或系统设备名，也不把这一个有限能力当成完整反指纹或匿名方案。
