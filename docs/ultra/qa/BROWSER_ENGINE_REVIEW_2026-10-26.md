# H34 官方多版本浏览器目录与下载实装核查（2026-10-26）

## 本批实际交付

新增 `internal/browserengine/` 生产模块，提供 `New(explicitHTTPClient)`、
`RefreshCatalog(ctx)`、`DownloadVersion(ctx, engineID, version)`、
`ListInstalled()`、`InspectInstalled(id, version)` 与 `Plan(id, version)`。
真实网络下载逻辑不是 fake；默认测试注入 fake transport，真实成品的静态
核验另行运行，均没有启动浏览器/安装程序。本批没有改动 WebView2 宿主、
系统代理、网卡、服务、策略或隧道，也没有访问任何用户订阅/账号数据。

固定 ID：`chrome-for-testing` 与 `firefox`，实际 Windows x64 成品，平台
`win64`。普通用户目录固定为 LocalAppData 下本产品 BrowserEngines；
外部 API 不接受路径、EXE、任意源站、命令行或脚本。构造不联网、不写文件。
管理员/系统 token 不作为浏览器普通用户目录操作主体。

## 官方来源与实际格式

- Google 官方 [Chrome for Testing 项目/API](https://github.com/GoogleChromeLabs/chrome-for-testing)
  的 `known-good-versions-with-downloads.json` 获取历史版本；选择后读取固定
  `<version>.json`，只接受与版本完整匹配的 Google Storage win64 ZIP 地址。
  [本次固定版本元数据](https://googlechromelabs.github.io/chrome-for-testing/156.0.8078.47.json)
  对应真实 Chrome 156.0.8078.47，不是只修改 UA 或版本显示。
- Mozilla 官方 [当前版本 JSON](https://product-details.mozilla.org/1.0/firefox_versions.json)
  和 [release archive](https://archive.mozilla.org/pub/firefox/releases/)
  提供发行/ESR版本目录。历史目录存在不代表仍受安全维护，UI 不应将旧版
  catalog 等同于安全推荐。当前支持标准发行/ESR数字版本；beta/nightly
  版本串及其他架构不是本批支持范围。
- 实际读取的 [Firefox 158.0.1 Windows en-US 官方目录](https://archive.mozilla.org/pub/firefox/releases/158.0.1/win64/en-US/)
  只列 EXE/MSI，没有冒充官方 ZIP。生产实现下载固定 SFX EXE，以
  [官方 SHA512SUMS](https://archive.mozilla.org/pub/firefox/releases/158.0.1/SHA512SUMS)
  精确匹配 `win64/en-US/Firefox Setup 158.0.1.exe` 后，作为7z数据读取；
  只保留 `core/`，不提取/执行外层 setup，不静默安装。

CfT 是 Google Chrome 可执行成品，使用其
[额外条款](https://www.google.com/chrome/terms/) 及随附组件许可。
Chromium 开源源码及第三方组件的开放许可不能替代 Google 成品条款；本批
不将 CfT 标为独立编译的无品牌 Chromium。没有将两种官方浏览器成品直接
加入 Ultra 发布 payload；用户显式下载时保留其完整运行树及随附许可。
Firefox 核心/组件开放源码许可与 Mozilla 成品品牌/分发政策也分别保留，
不把 Firefox 的可下载性解释为任意重命名授权。

## 固定开源读取器与依赖

生产引入 [bodgit/sevenzip v1.6.5](https://github.com/bodgit/sevenzip/tree/v1.6.5)，
固定提交 `dcfc72a0ee9f527c55521f44ffdf1c31b732e256`，module sum
`h1:7H7BxgmeX0j6UX42lH+KXQ92WgMQJ49DoocFdfHbCng=`，GoMod sum
`h1:GhuB6Lq1xCpP1sps+horjZ8lgiKPJcy2zUX3prla9wc=`。
已实际阅读固定版本的 `reader.go`、`register.go`、`struct.go`、`go.mod`
和 LICENSE：`NewReader(io.ReaderAt, size)` 直接读给定数据；SFX 查找在
1 MiB内扫描7z签名；注册 BCJ2、LZMA、LZMA2 等解码，纯Go，无外部7z/Perl/
installer调用。正式代码使用 `NewReader`，不会使用可根据文件名寻找分卷
或其他文件的 `OpenReader`。

`third_party/` 保留七压读取器 BSD-3-Clause 及新增/升级依赖完整许可证：
plumbing 1.3.0、bodgit/windows 1.0.1、golang-lru/v2 2.0.7 (MPL-2.0)、
afero 1.15.0、ppmd 0.1.1、xz 0.5.15、go4 a5071408f32f、brotli 1.2.2、
lz4/v4 4.1.27。未修改这些库源码；对应公开版本/模块sum在 go.mod/go.sum
固定，发行包需要携带这些 notices 并提供对应源码版本。原有其他Go依赖的
许可审计继续适用，不能把本文件当成全仓最终许可审计。

依赖图实际必需变更：brotli 1.1.0→1.2.2、lz4/v4 4.1.26→4.1.27；
sevenzip 的 go4.org 依赖在 go.mod 引用 cloud/storage 等模块图，并使 Go
MVS将原有 genproto/googleapis/rpc 提升到
`v0.0.0-20251222181119-0a764e51fe1b`。本包实际编译依赖只用
`go4.org/readerutil`，`go list -deps ./internal/browserengine` 无新增 cloud/
grpc执行路径。此图变化仍需纳入最终全仓测试与发布审计。

## 下载/解包/读回保证

下载客户端必须显式提供 transport，由调用方将请求绑定到已验证路由。
模块不会创建 DefaultTransport、继承环境代理或在代理失败时回退直连。
固定 HTTPS源/路径与版本逐字比对，拒绝所有重定向、额外用户信息/端口/
query、内容编码、异常响应长度、超限与超时，错误只返回固定标识。
下载中不附带用户浏览器cookie或上传机器指纹。

使用 Go `os.Root` 将读写限制在本产品目录内；采用原有 localstate
本地固定盘/无reparse祖先检查和租约；打开root后再读取该实际目录句柄的
最终路径，与固定预期目录（正规化8.3名称）对照，拒绝获取期间被改指的
根对象。实际Windows分享/重解析竞争的完整动态验收仍属VM项。ZIP/7z条目在写出前整体检查：
拒绝绝对/上级/设备名/ADS/反斜线/尾点尾空格/非ASCII/链接/reparse、
大小写同名、文件兼作父目录；最多20,000条、单文件512 MiB、整体展开
2 GiB、下载512 MiB、路径深度32。7z外围非core成员也计入解码大小上限。
不继承 archive 的执行/ACL属性；使用新文件独占创建，不落地外层安装代码。
context同时约束HTTP与归档底层读取。归档解析仍是进程内解码；这些限制
不是独立沙箱/无OOM保证，源站可信度及解码器维护仍是依赖边界。

版本目录 `engineID/version` 独占创建，不覆盖已有目录。文件SHA256/大小、
完整目录名单、来源/元数据/归档摘要、版本与时间写入 receipt，最后提交
receipt。失败/崩溃残留目录没有有效receipt则不会视为已安装，不自动复用/
覆盖/递归删除。后续显式修复UI仍需实现，当前可准确报错而不能假装续传成功。
读取已有版本时重算整树，拒绝新增/丢失/改写、链接和非法receipt；检查
真实AMD64 PE、Windows ProductVersion资源，Firefox额外检查 application.ini。
ESR目录后缀与实际 application.ini不含esr的数值版本正确对应。

Chrome 官方feed没有发布归档校验和。代码记录HTTPS来源、下载计算的SHA256/
SHA512和整树receipt，不能称其为上游独立签名/预置哈希认证。Mozilla使用
官方SHA512SUMS验证完整SFX，但摘要与归档经同一HTTPS渠道获取，未实施
独立PGP签名校验。普通用户可写cache/receipt也不是管理员保护的可信代码
清单；控制同一用户token的攻击者可能修改两者。缓存层本身仍将
`CanLaunch` 视为false，不会把缓存自校验提升成安全执行授权；Windows
受控启动仅在完整校验后交给 native host 再次执行代码租约与进程隔离检查。

## 本机实际验证

默认 `go test ./internal/browserengine -count=1`：11个顶层纯fake/TEMP
测试及其分支通过；另两个手工静态案例默认skip。覆盖两种真实生产归档路径
的无害自制ZIP/SFX、精确官方请求、损坏/冒充来源/重复JSON/大小限制/取消/
校验失败、路径遍历/ADS/设备名/链接/大小写冲突、版本不符、已有目录不覆盖、
缺失/改动/额外文件、不完整目录不作为已安装、启动计划继续拒绝执行。
`go vet ./internal/browserengine` 与 `go mod verify` 通过。默认套件无真实
socket、原生浏览器或安装操作。

另已显式下载以下官方成品到ignored临时缓存，运行生产解包和读回的静态
案例。测试仅在TEMP写出成品文件，并读取PE资源；没有执行浏览器/安装器/
归档程序、没有持久安装或系统/网络配置变更。

| 成品 | 归档摘要 | 实际纯数据提取/读回 |
| --- | --- | --- |
| Firefox 158.0.1 win64 en-US SFX | SHA256 `3259a2f8508e58cf2e383b6c99bfc64d6ddc86d1407b7c2f86f660af955b5d10` | 74文件、14目录，core/精确版本/全部SHA256重算通过；63.23秒 |
| CfT 156.0.8078.47 win64 ZIP | SHA256 `a4e0d57245e167e98a9d465af1e3774d2b4ec9f04e483d19bcd97a6f63cf46dd` | 305文件、7目录，AMD64/精确VERSIONINFO/全部SHA256重算通过；12.60秒 |

Firefox完整SHA512：
`4cd87e51e033406ea5bdd4fbb969843b41365570c4e59e95610ed5074326221535279c616fba21ca5c89b0503843aa746f8a3a65a593221e77bef6a18ee38ac8`。
官方SHA512SUMS文件本地SHA256：
`474ce20671dfb7da824fe6af20f89b6860b463cc40a4a634433c220686029cf1`。
CfT固定版本JSON本地SHA256：
`280a3aaf97babf78351b78176ca4fbf3727bf374752f34ea05aa7753bf423c87`。

手工静态案例只接受已经缓存的官方文件，不联网；设置下列环境变量后，
分别运行单个 `TestManualOfficialFirefoxStaticExtraction` 或
`TestManualOfficialChromeStaticExtraction`。缓存内容不进入公开源码或包：
`ULTRA_ENGINE_STATIC_REVIEW_ARCHIVE` + `ULTRA_ENGINE_STATIC_REVIEW_SUMS`，
或 `ULTRA_ENGINE_STATIC_REVIEW_CHROME`。这些案例使用fake HTTP传输把已核实
成品送入同一个生产下载/解包路径，普通fake测试不会因此获得真实成品证据。

## 剩余实际宿主边界

本节由后续源码checkpoint更新：`internal/browserenginehost` 已接生产原生
路径，`internal/browserengineguard` 已实现独立WFP与管理员代码seal，但全部
仍处于真实VM验收阶段。Windows amd64/arm64 普通构建 `Start` 已进入同一
真实生产路径；catalog 仅在完整树/版本校验后开放启动。编译期
`browserengine_acceptance` 仍可用于 QA 包，但不接受前端跳过flag，不绕过原GUI/令牌/
映像/权限/Job/WFP/代理或协议读回。不存在伪造调用者/fake guard的验收旁路。

生产路径为：新随机session目录/独立profile → 从官方归档重新认证的内存
清单与整树pins → CREATE_SUSPENDED+原子JOB_LIST归属 → 已认证helper从GUI
实际token派生路径、核Job/mainPID/creation/image → 逐EXE固定WFP complement
BLOCK安装及独立原生读回 → resume → 真CDP pipe或Firefox BiDi → 内存HTTPS
HTTP UA与主/同源/opaque frame原生环境读回 → 二次WFP核对 → 开relay → ready。
任何中途失败都不会将部分配置当ready；未完成读回不会开放导航。

helper逐对象使用已核验同一句柄设置Administrators owner、受保护精确DACL：
SYSTEM/BA full、原GUI SID仅read/execute。覆盖sessionRoot/engine/version/
所有code/receipt，profile叶保留用户写权限。完整二次枚举和同句柄owner/
DACL/finalpath读回用于排除新增optional DLL及对象置换。GUI与helper的
代码文件pins一直持有到Job独立读零；不是把可改receipt升级为publisher认证。
真实Windows sharing/ACL/seal必须在VM验证，本机没有实际更改任何文件ACL。

浏览器SOCKS前端门在所有配置读回前保持关闭。前端用实际逆TCP tuple、
PID/创建时间/Job/固定EXE/SID核对客户端，再对属于本GUI的后端完成随机
RFC1929认证；CONNECT域名原样传给显式上游，不自行解析网站DNS/直连网站。
Chrome参数与Firefox偏好只是附加配置，不能代替此进程级WFP限制。

失败/GUI失联：先关闭两级relay，TerminateJob并确认ActiveProcesses=0，
再关闭可能同步阻塞的协议pipe，helper自己再查Job为零后撤完整自有filter并
读回缺失，最后释放代码/profile pins。未知Job保留整个对象图防GC释放pins，
只读监视在以后实际观察零时完成清理；未知撤filter结果不重试、不称成功。
独立provider沿用现核心nil-service设计，检测任何disabled/ownership变化即
unknown/终止Job；BFE/重启语义仍需新provider的VM证据，不新增常驻服务。

UA/环境JSON与主表单同形：`identity` native/generated/manual、可选environment
固定四OS preset+数值OSVersion。拒调用者Client Hints；Chrome由实测native
high-entropy CH保留真实引擎品牌/版本，再用正式CDP覆盖UA/platform/CH/
metrics/touch。Firefox用正式BiDi UA/locale/timezone/viewport与fresh profile
原生偏好设置platform/OSCPU/touch/meta-viewport，逐frame实际核对；未提供
Chromium UAData时不伪造。手动UA原文保留，HTTP/header归一化不一致则拒绝。
OS预设不改变实际OS/内核/硬件/像素/性能，也不冒充Safari。native新路径
此段为原验收阶段记录。后续 QA12 源码新增 Firefox 原生 GPU metadata / WebGPU
阻断，准确接口、masked/RFP边界和四类Window/Offscreen证据见
[QA12专项记录](FIREFOX_NATIVE_GRAPHICS_QA12_2026-10-27.md)；Chrome 此项仍明确拒绝，
不能把新增源码算作冻结QA11或真实VM已通过。受控HTTP(S)流量是保护范围，
明确移交到浏览器外的OS应用不宣称共享这个浏览器的代理或环境。

已新增本地纯内存协议/握手、合成TCP表/安全描述符、fake Job/filter生命周期
用例；定向go test/vet通过。它们没有执行真实Job/WFP/浏览器，也不证明网络
分类、窗口、关闭、实际图形/环境或跨进程frame效果。下一步VM需覆盖普通
HTTP(S)、remote DNS、WebRTC/UDP、proxy失败、redirect/外部scheme、worker/
serviceworker、用户关窗、GUI/helper失联、seal新DLL/篡改拒绝，以及同版本
多profile端口隔离。真实受保护模板cache尚未实现，每次验收下载同包的方案
不是最终UX：需helper官方认证下载一次至受保护版本模板，再逐session复制/
核验/seal，避免把“已下载”按钮误当成免重复下载的执行缓存。

Android/iOS/macOS/Windows指网页环境配置；没有宣称提供四个原生OS客户端，
也没有将Firefox/Chromium UA模拟说成真正Safari/iOS内核。跨引擎环境一致性、
GPU/Canvas/Audio/字体和平台能力的真实覆盖须按各宿主继续实现与验证。
H34整体仍未完成，本批完成的是可用官方下载、静态解包和已存版本核验。
