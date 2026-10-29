# H32/H33 元数据上游审查与实际适配器（2026-10-26）

状态：已实现 `internal/metadatatool` 的生产进程适配器、固定 Windows 依赖准备、可信文件树门、单文件读取、7 类 EXIF 选择清理与新副本后验。仅通过本机静态、合成和自有 TEMP 测试；**没有在本机执行 ExifTool/Perl，没有完成真实后端、GUI 或隔离 VM 验收**。本模块独立于已有 `privacydata`；没有修改前端、现有清理路径或既有构建。

## 固定上游与下载证据

| 项目 | 固定证据 |
|---|---|
| ExifTool 源码 | [exiftool/exiftool](https://github.com/exiftool/exiftool/tree/2200871d9cef988051d2a99d67df3bda6cbb30a8)，SHA `2200871d9cef988051d2a99d67df3bda6cbb30a8`；脚本及 META 版本 `13.59` |
| 源码 ZIP | [GitHub codeload 固定提交](https://codeload.github.com/exiftool/exiftool/zip/2200871d9cef988051d2a99d67df3bda6cbb30a8)，SHA256 `564e90d3c74cd9062b38e0be0475ed6d4a10553128a648027edb70723bfc3f1c` |
| Windows 供应者 | 固定源码 [html/index.html](https://github.com/exiftool/exiftool/blob/2200871d9cef988051d2a99d67df3bda6cbb30a8/html/index.html) 指明使用 Oliver Betz launcher 并链接其发行页面；本次使用该作者的 [13.59 x64 ZIP](https://oliverbetz.de/cms/files/Artikel/ExifTool-for-Windows/exiftool-13.59_64.zip) |
| Windows ZIP | 13,533,363 字节；SHA256 `577257dd22baebe77157d905792d6ed2c5916cd03aa627f40b2175db12110ac6` |
| 完整包成员 | 1501 个文件、46,706,184 解压字节；嵌入 `bundle.sha256`，其 SHA256 `d1316fddf986e2d17761f85034c753b28375f25bf73235c1084a7c5a38e22faf` |
| 实际入口脚本 | ZIP 内 `exiftool_files/exiftool.pl` 与固定源码 `windows_exiftool` 字节一致，SHA256 `9e8e817fab9fdc2e37f7cd023aa34e76d1ef561834dfd791a48a7e327cc8f4b1` |
| ExifTool 库对应 | 固定提交 `lib/` 的全部 246 个文件逐成员 SHA256 与 Windows ZIP 对应路径一致 |
| Launcher 源码审阅 | 作者 [Portable Perl Applications](https://oliverbetz.de/pages/Artikel/Portable-Perl-Applications) 的 `ppl.zip`，SHA256 `72a112caac70de4cab56971d0bf1ea8050f8228ed036ec88e7657d40cc08ce0e`；`ppl.c` 使用 DLL 的 `RunPerl` 在本进程运行脚本，并设置固定 runtime DLL 目录 |

Windows 包不是一个独立 EXE。必须保留 `ExifTool.exe` 和完整 `exiftool_files/`，包括 `perl532.dll`、Perl 模块、原许可证及 runtime 许可证归档。发行页面说明基于精简 Strawberry Perl 5.32.1.1；本次未重建、未独立证明原生 DLL 与某个可重现构建一致，也未完成该较旧 runtime 的漏洞清单审查。`ExifTool.exe` Authenticode 状态是 **NotSigned**：本次固定 hash 来自上述供应者 HTTPS 获取和逐成员核对，不能写成“有官方签名/已独立认证”。SourceForge 链接本次返回 HTML，已拒绝将其当成 Windows ZIP 使用。

## 实际源码结论与许可边界

- [README](https://github.com/exiftool/exiftool/blob/2200871d9cef988051d2a99d67df3bda6cbb30a8/README) 和 [META.yml](https://github.com/exiftool/exiftool/blob/2200871d9cef988051d2a99d67df3bda6cbb30a8/META.yml) 声明 Perl 同等许可（Artistic 或 GPL）；这个具体提交的 [LICENSE](https://github.com/exiftool/exiftool/blob/2200871d9cef988051d2a99d67df3bda6cbb30a8/LICENSE) 实际为完整 GPLv3 文本。保留两者，不用一个未经解释的 SPDX 标签遮掉该证据差异。
- 包内 `readme_windows.txt` 说明作者自身 launcher/封装内容使用 CC0，ExifTool 与 Strawberry Perl 各保留自己的许可；`Licenses_Strawberry_Perl.zip` 是多组件许可集合。准备脚本复制原包全部成员，保留 `LICENSE`、`README`、`readme_windows.txt` 和该归档。
- GodUseVPN 原仓库自身的 GPLv3 与这些依赖许可分开记录；不是把 ExifTool 和 Perl runtime 改成 GodUseVPN 自有许可。分发时需随实际软件保留通知及适用的对应源码提供安排；仅有 ZIP hash 不替代许可和来源要求。
- ExifTool 的配置文件可执行 Perl。已读 `windows_exiftool` 开头和 `lib/Image/ExifTool.pm` 配置加载逻辑，**首参数固定 `-config ""`**，禁用用户 HOME、EXIFTOOL_HOME 与程序目录配置自动加载。用户不能提供配置、参数文件、脚本、表达式、递归、目录或 stay-open 参数。
- 已读 `windows_exiftool` JSON 输出、stdin buffering 和 stdout 写入分支及 `windows_exiftool.txt` 的 `-o` 文档。输入固定 `-`，写入固定 `-o -`；Perl 仅看到匿名管道，不接收用户输入/输出文件路径。没有 `-overwrite_original`、`-all=`、`-r`、`-execute` 等入口。
- 已对字段核查 `lib/Image/ExifTool/Exif.pm` 的实际名称。例如 EXIF 规范叫 BodySerialNumber 的字段在这个版本名称是 `ExifIFD:SerialNumber`，没有传入不存在的 `BodySerialNumber` 别名。

BleachBit 仅作为交互参照：依据其 [General usage](https://docs.bleachbit.org/doc/general-usage/) 与 [CLI 文档](https://docs.bleachbit.org/doc/command-line-interface/)，先展示范围和选择，再执行当前选择。本模块没有引入其 Python/GTK 程序、系统扫描、浏览器缓存删除或粉碎功能；没有声称实现 BleachBit 清理能力。

## 构建与安装契约

从仓库执行（仅下载、校验、复制，不运行第三方软件、不安装系统 Perl）：

```powershell
& .\internal\metadatatool\Prepare.ps1
# 可使用已下载、但仍需完全匹配固定 SHA256 的归档：
& .\internal\metadatatool\Prepare.ps1 -Archive .\.codex\tmp\exiftool-review-2200871\oliverbetz-exiftool-13.59_64.zip
```

输出 `.codex/tmp/metadata-tool-build/exiftool-13.59/`。脚本验证归档 SHA 和嵌入清单自身 SHA，再对每个 ZIP 成员验证固定 SHA、长度、名称与完整文件数；拒绝额外/重复/穿越/链接/超大成员。既有输出必须完整匹配才复用，绝不合并覆盖；新输出在独立 staging 完成后原子移动。准备脚本已在 PS7.6.5 成功提取并在 Windows PowerShell 5.1.19041.6456 重新校验全部 1501 文件。

Root 打包应将此目录整体复制为：

```text
%ProgramFiles%\GodUseVPN Ultra\metadata-tool\exiftool-13.59\ExifTool.exe
%ProgramFiles%\GodUseVPN Ultra\metadata-tool\exiftool-13.59\exiftool_files\...
```

当前仅 Windows amd64 对应此 x64 bundle，其他平台/架构明确不可用。保持安装目录管理员/SYSTEM 所有者及普通用户只读权限。生产适配器只接受该固定路径，且调用方必须是该安装根中的 `godusevpn.exe`，使用普通 medium token；SYSTEM、服务账户和提升进程被拒绝。它不搜索 PATH，不接受任意解释器或用户自定义工具路径，也不在运行时下载。

每次操作先 pin 祖先目录、完整依赖树和文件；拒绝重解析点、文件 hardlink、不可信 owner/DACL、未知成员及 hash 不匹配，文件句柄拒绝 write/delete sharing 并保持到操作结束。所有 DLL、Perl 模块和配置样例都在完整清单内。缺包/权限/未知平台统一返回不可用，不能降级到系统 Perl 或现有 `privacydata` 而仍标成 ExifTool。

## 给 Go/GUI 的 API

```go
tool := metadatatool.New()
backend := tool.Backend() // 静态可信度；无子进程。VMValidated 当前始终 false。
fields := metadatatool.Fields()
preview, err := tool.Preview(ctx, absoluteInputPath)
values, err := tool.ReadValues(ctx, absoluteInputPath, preview.SourceSHA256, []string{"exif-gps"})
result, err := tool.CreateCopy(ctx, metadatatool.CopyRequest{
    InputPath: absoluteInputPath, OutputPath: absoluteNewPath,
    PreviewSHA256: preview.SourceSHA256, Remove: []string{"artist", "exif-gps"},
})
```

`Preview.Tags` 只含 tag 名 -> `{field, jsonBytes}`；包括 GPS 在内，默认不返回实际值。显式 `ReadValues` 仅揭示选中的字段，并重新验证预览 hash。UI 应将真实值当作不可信文本转义，放在用户当前选择范围内，不写 console/遥测/诊断日志。后端错误只返回固定错误码，子进程原始 stderr、路径和元数据值不外发。

7 个固定字段 ID：`artist`、`copyright`、`description`、`software`、`camera`、`capture-time`、`exif-gps`。`Fields()` 返回每类精确 tag 列表，UI 不应自行扩展。`exif-gps` 清理 EXIF GPS 整组，**不清 XMP 位置**。本轮提供选择删除，不提供任意字段写值或批量目录扫描。

`CopyResult.Selected` 是用户本次选择；`Removed` 只列出清理前实际存在、后验已不存在的字段类，原本不存在的字段不冒充清理成果。

读取源文件和创建副本都只接受明确本地固定磁盘的 JPEG/JPG/PNG 绝对路径。源文件以只读、拒绝 write/delete sharing 的句柄读取，祖先和源句柄持续持有。Windows 的普通 8.3 路径只在已 pin 对象后扩展核对，不授权重定向。清理请求必须匹配当前源内容 SHA256；创建副本前再次通过同一源句柄重算 SHA。

副本先在内存完成 ExifTool 写入和二次读取后验，再使用 `CREATE_NEW` 写到用户指定的新文件：绝不打开原件写句柄，绝不覆盖已存在输出。输出使用仅当前用户/管理员/SYSTEM 的私有 DACL，写入、Sync 后通过同一句柄核对 SHA；失败删除只针对自己创建的文件句柄。

## 边界与尚未验收项

- 仅单帧 JPEG/PNG；最大 32 MiB、16,777,216 像素、单边 16384。APNG、MPF/MPO、多图/尾随数据、损坏 PNG CRC、GIF、PDF、HEIC、RAW、Office 等明确拒绝。上游格式表很大不等于本产品已支持那些格式。
- 每次子进程受总操作 context 的 20 秒 deadline 约束，stdout：JSON 128 KiB/图像 32 MiB，stderr 8 KiB；退出异常、任何 stderr、超限或取消均失败，不返回原始诊断。上游正常 launcher 调用 RunPerl 在自身进程内；当前代码不是 AppContainer/网络隔离沙箱，也未实现进程内存上限，不能把固定参数和超时宣传为可抵御所有解析器漏洞。
- 后验：格式/尺寸一致、Go 解码像素一致、选中的已查询 tag 全部消失、未选中的已查询 tag 值不变；不是对每一种隐藏元数据和外部渲染器的完整等价证明。XMP、IPTC、MakerNotes、缩略图、ICC、可见水印/文字、文件名/文件系统时间等都不在本次全量清除承诺内；存在残留很正常，不能标成“完全匿名”。
- 只读输入与预览 hash 防止普通文件替换/并发写入；同账户已执行恶意代码、内核/管理员、先前已建立的映射等不在完整隔离承诺内。前后 hash 仍会拒绝检测到的内容变化。
- 真实执行的 JSON group 名、Windows launcher 空参数 `-config ""`、Unicode 环境、stdin/stdout JPEG/PNG 写入、字段删除和未选择字段保持、持有句柄时的运行时 DLL 加载，都必须在授权隔离 VM 中实际测试。尚未执行过第三方，所以 `BackendInfo.VMValidated` 明确为 false。

## 本机验证记录

- `go test ./internal/metadatatool -count=1`：14 个顶层测试、19 个子测试通过。包括 JPEG/PNG 合成、严格 JSON/重复 key/版本/类型拒绝、选择白名单、源预览绑定、保留字段/像素变化/超时/未知输出拒绝。
- 生产 `nativeRunner` 使用**自己的 Go 测试 EXE**作惰性 fixture，验证真实进程启动、stdout 限制、stderr 脱敏和超时 kill；不是第三方执行。此项在普通测试 pass 必跑；coverage 运行时因 runner 故意不继承 GOCOVERDIR 而跳过，不用 coverage pass 替代普通 pass。
- TEMP 原生测试实际确认源写/rename 被只读 pin 拒绝、新输出创建成功、已存在输出/同源路径/扩展名不匹配拒绝。
- `go vet ./internal/metadatatool` 通过；Linux 非 Windows 分支仅交叉编译，不运行，明确不可用。
- `Prepare.ps1` 全量准备及两版 PowerShell 复核成功。没有执行真实 ExifTool/Perl，没有系统服务、策略、网络或真实文件清理变更。

隔离 VM 下一步：准备本模块固定依赖并以普通安装 GUI token 调用以上 API；对自行创建、包含 artist/GPS/软件/相机/时间的 JPEG 与 PNG 分别执行预览、显式查看、逐类及组合清理；独立读取输出并比对输入 SHA、像素和保留 tag。还需测试坏包/额外 DLL/可写 DACL/reparse/源替换/锁定/输出重名/超时并读回，之后才将具体版本的真实后端标记为验收通过。
