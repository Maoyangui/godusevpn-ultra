# 独立浏览器配置预览边界

本目录管理独立浏览器的配置与读回合同；实际启动、租约和设置执行分别由 `cmd/godusevpn` 与 `browser-host` 负责。本目录自身不启动浏览器、不创建目录或修改系统。现有 `Read()` / `Report` 返回保持原样，真实主机效果仍需 VPS 验收。

- `DefaultProfile`、`ValidateProfile`、`EncodeProfile`、`DecodeProfile` 处理版本化配置。存储中只有安全 profile ID，不接受可执行文件/用户目录路径、任意启动参数、远程调试字段；JSON 拒绝未知字段、重复字段、超限及尾随内容。
- `RegionPresets` 返回语言、地区、时区和 Accept-Language 的成组选择。`SeededPreview` 按固定种子和 profile ID 生成可重复预览，只选择这些区域和普通桌面内容窗口尺寸，不推断出口地区，不制造屏幕、GPU、Canvas、字体或硬件身份。
- UA 支持 `native`、`generated`、`manual` 和兼容的 `observed-exact`。仅 generated 调用共用 `GeneratedChromiumUserAgent`，版本一律为真实 Chromium 主版本 `.0.0.0`；Windows/Mac/Android 使用 Chromium 官方固定简化令牌，未选环境使用 Windows 模板。iOS 保留合成 iPhone + 真实 Chrome 主版本，明确不是零售 CriOS、原生 Safari 或 WebKit 605。manual 的 `userAgent` 接受非空白、至多 1024 字节的可打印 ASCII，逐字保留；native 即使选择 environment 也保留独立观察的原始 UA。所选环境仍独立控制 platform/CH/viewport/touch，native+environment 会提示可能差异。前端不能提供 generated/manual 的 Client Hints；CH 中的完整内核版本不归零，仍由真实运行时读取。
- `PreviewIdentity` / `ProfilePreview.identityPreview` 是计划：只有库存产品版本时提供真实主版本的 reduced UA（`Chrome/<major>.0.0.0`），`exactChromiumVersionKnown=false`；有独立 native CH 才能填写 `chromiumVersion` 完整值，但 generated UA 仍使用简化主版本。主版本未知只返回 `{ChromiumMajor}.0.0.0` 模板；native 未观察时只返回 `{NativeUserAgent}`。`applied` 始终为 false，启动前必须核对 HTTP/navigator UA 与 CH。手工 UA 中版本/OS 令牌不一致会给出具体警告，不偷偷改写。`RuntimeContext.Identity` 不能由用户 JSON 冒充；当前 `Read()` 不提供这种实测快照。

Windows `environment.osVersion` 保持既有 UA-CH platformVersion API 合约原值，不自动把手填或已保存的 11 改成 13。Windows 10 / 11 界面预设应分别显式选择 10.0.0 / 13.0.0；generated UA 均固定 NT 10.0。依据：[Chromium 官方简化格式](https://www.chromium.org/updates/ua-reduction/)、[Chromium 当前实现](https://github.com/chromium/chromium/blob/main/components/embedder_support/user_agent_utils.cc)、[Microsoft 平台合约说明](https://learn.microsoft.com/en-us/microsoft-edge/web-platform/how-to-detect-win11)。Go/C# 共用 `testdata/chromium-ua-golden.json` 的完整字符串向量，避免预览与宿主各写一套格式。
- WebRTC、WebGL、权限、Cookie、退出清理和隧道/本地代理参数在本模型中是待执行意图。WebGPU adapter 身份伪造、全局权限允许、不受支持模式一律拒绝；显式 WebGPU 阻断和有限 WebGL 字符串请求必须满足[独立 GPU 强制与读回条件](GRAPHICS_BOUNDARY.md)。代理只接受不带凭据/路径的 `http` 或 `socks5` 字面量回环端点，不接受直连回退。
- `CapabilitiesFor` 区分可预览、仅运行时默认值和不支持。所有引擎的 `hostImplemented`、`canLaunch`、`runtimeVerified`、`enforced` 均为 false；安装了运行时不等于功能已执行或验收。

后续主机必须在自有根目录下安全创建 profile、处理重解析点及所有权、绑定受控隧道/代理，并独立读回身份和设置。绑定失效时禁止导航；不能仅凭此模型的验证成功开放联网。退出清理还需独立验证仅删除该产品创建的数据。任何这些步骤均未由本批模型代替。

包内新增测试全部使用内存数据，不启动浏览器、不写策略、不改网络、不创建 profile。运行 `go test ./internal/browserhost` 和 `go vet ./internal/browserhost`。
# Logical processor count

Optional profile field (schema version remains 1):

```json
"hardware": { "mode": "logical-processors", "logicalProcessors": 8 }
```

Omission keeps the native value and preserves old profiles. Explicit null,
empty/native objects, unknown/case-alias/duplicate fields, other hardware,
non-integer JSON numbers and counts outside 1–64 are rejected. This changes only
the website-visible `navigator.hardwareConcurrency` through the real engine
protocol; it does not change the CPU, performance, core affinity or RAM.

`hardwareRequirements` in `ProfilePreview` describes the requested method,
count, contexts and failure behavior. It is never an applied readback receipt;
`applied` remains false and `needsRuntimeReadback` remains true. The saved draft
store accepts this same non-secret field; bindings and runtime trust remain
excluded.

The official [CDP Emulation domain source](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Emulation.pdl)
defines `Emulation.setHardwareConcurrencyOverride` with the integer parameter
`hardwareConcurrency` and marks it experimental. Chromium's
[InspectorEmulationAgent implementation](https://github.com/chromium/chromium/blob/main/third_party/blink/renderer/core/inspector/inspector_emulation_agent.cc)
validates a positive count, stores/restores its override state and applies that
state to the getter. These upstream sources justify the command, not a claim
that every installed WebView2 version supports it. The host must execute it,
read back the native property and refuse navigation on any unsupported command,
missing context or mismatch. No JavaScript property override is used for CPU.

