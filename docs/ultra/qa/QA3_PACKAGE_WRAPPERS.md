# qa.3 固定重建与组包包装脚本

2026-10-19。两份新脚本从已冻结 qa.2 版本复制；不修改旧脚本、旧包或 immutable qa.3 发布源。**本轮只运行 AST、内存元数据、合成 ZIP 和实际 ZIP 的内存校验；没有运行 wrapper main、decoder、EXE、安装器或真实 QA 目录操作。**

| 脚本 | SHA256 |
| --- | --- |
| [Expand-VpsQa3Delta.ps1](Expand-VpsQa3Delta.ps1) | `c8697132b79cbe19c7b592e9d7bcdf6c16a21967b7cbadd90b8146d671ee1c9a` |
| [Assemble-VpsQa3Package.ps1](Assemble-VpsQa3Package.ps1) | `00393b3d79d7d64ecda93ff06d16ea6233a25aeb2ef8b0b879d08315c8153834` |

## 输入、输出与固定数据

`Expand` 参数为 `StageID`、`CandidateLeaf`、`BaseStageID`、`BaseCandidateLeaf`。两个 StageID 必须是不同的 lowercase UUID-v4 无分隔符值；两个 leaf 必须是 `delta-` 加 32 位 lowercase hex。源是既有私有 QA 根下 `<BaseStageID>/<BaseCandidateLeaf>` 的 qa.2 三 EXE，不再使用旧 `base-for-beta-qa2`。目标是 `<StageID>/<CandidateLeaf>`；中间目录固定为 `<StageID>/transfer-beta-qa3`。二者存在即拒绝，不覆盖、不重试、不自动清理。

`Assemble` 参数为 `SupportStageID`、`CandidateStageID`、`CandidateLeaf`。读取 `<SupportStageID>/preview.zip` 和 `<CandidateStageID>/<CandidateLeaf>` 三 EXE，新输出固定为 `<CandidateStageID>/package-beta-qa3`。

| 固定输入 | 长度 | SHA256 |
| --- | ---: | --- |
| transfer ZIP，成员前缀 `vps-beta-qa3-transfer/` | 12,328,692 B | `6439d6ba4d451290290f915e711d28442fa54e8c97b3fe01b791d5dd6779316d` |
| delta manifest | 固定 hash 绑定 | `ee98cd29f3649d40b02ea09501258e4e83920a7af6684998a3b1e73b509ea903` |
| 冻结 decoder | 固定 hash 绑定 | `e4035a5be9d064f3ad51ef79ecce87dc616edd318b87ad32d67fc6862a4a6adf` |
| support ZIP，无顶级前缀或目录项 | 10,459,385 B | `a7540acd84d2ab23354cd20f6f1fcceadb05126650e3b98aa693a869b20c22df` |
| 原 qa.3 SHA256SUMS | 351 个 payload | `bd0bcadae8965f3f782e63146d917f1fac82df8d3d7fd15eee3cd0a2cd5f61dd` |

transfer 精确 5 个文件，support 精确 349 个非 EXE 文件（含 SHA256SUMS）。最终为 **351 个 payload + SHA256SUMS = 352 个文件**。描述符和三 EXE 静态版本必须是 `0.01-beta-qa.3`。

| EXE | 长度 | SHA256 |
| --- | ---: | --- |
| GUI | 16,122,368 B | `2293b29ed415e7ef972208e54f690437f61a6e195f35c9da0f4bdf9a7c7caa15` |
| CLI | 8,723,456 B | `3a17693ee9e80c865b9bee92ee34f403adeb75883a797b821e6123e9edabfae8` |
| service | 43,166,720 B | `2ebd104afd943353f18662510e751acf9421e03fa7cca834d516444b98f5127e` |

## 保留的边界与必要差异

私有 SY/BA owner/DACL、固定本地磁盘、root-first/no-follow、句柄保留、ZIP 文件名/类型/数量与大小上限、CREATE_NEW、写后磁盘 SHA、EXE 静态版本检查保持。Expand 只会启动已绑定哈希的固定 decoder，以 120 秒为上限；失败保留本批目录并报告未知终止，不运行产品。Assemble 没有进程执行入口。两脚本不创建或修复既有 ACL，不触真实产品数据根。

qa.3 原清单包含两个 0 B 的 build/vet 输出记录。新 Assemble 接受普通空文件，但仍必须通过完整 ZIP、清单和逐文件 SHA 认证；空文件 SHA 固定为标准 empty SHA256。没有扩大成员白名单、取消链接/ACL检查或允许无清单文件。增加了独立合成 0 B ZIP fixture，以及实际两文件的 empty SHA 核对。设备保留名中的 superscript 数字改为 ASCII `\u00b9`/`\u00b2`/`\u00b3` 正则表示，避免 PowerShell 5.1 无 BOM 源码解码差异。

这些手动 QA 工具仍以前置核验的私有 QA 祖先、可信管理员、稳定命名空间和操作系统为前提，不声称抵御恶意管理员或所有 DOS 映射/内核竞态。只有实际运行后的固定回执、磁盘读回与后续验收才能证明远端完成，静态测试不能替代。

## 已完成的本地有限检查

[Qa3-PackageWrappers.Tests.ps1](Qa3-PackageWrappers.Tests.ps1) 在 Windows PowerShell 5.1 完成 **79 项**：Parser/函数唯一性/ASCII、旧文件 hash 未变、关键 OS/private/pin/decoder 函数逐字未变、stage/leaf/保留设备名/ACL拒绝、ZIP 额外/重复/路径穿越/链接/EXE拒绝、合成普通空文件；实际 transfer/support 字节 hash、成员 hash、manifest schema/数量、三目标 hash/长度跨包对应及 tamper 拒绝全部通过。

另外只读比较两份 qa.3 Python stager 与原 Stage-VpsPackage.py，差异严格为 header/usage/两固定 hash 共四行，固定 ZIP hash 与上表一致；本次未执行 Python、az 或网络操作。

## 独立只读执行与普通令牌验收脚本

[Vps-Qa3ReadOnly.ps1](Vps-Qa3ReadOnly.ps1) 已从冻结的 `Vps-BetaReadOnly.ps1` 复制，SHA256 为 `9e3bce5d3b0974176211f00014c0a526a83bbe2664ed0f5187e704d58b76f936`。只绑定本批三 hash、qa.3 版本和新 schema `ultra-qa3-readonly-v1`，设备名正则使用等价 ASCII 转义。参数仍是 QA `StageID` 与 `delta-<32hex>` 的 `CandidateLeaf`；不接受安装路径或任意程序。

五个固定命令仍为 CLI `version`、`privacy report`、`privacy detection`、`privacy firewall-list` 和 service `version`，各 15 秒、有界输出、未知不重试。这个执行批次不发 `status` RPC、不启动服务/GUI/helper、不修改策略；SYSTEM 报告也不能代表登录用户。新 Tests 仅运行 AST、元数据和合成报告，验证严格归一化后与冻结源相等、原 qa.2 拒绝、五命令名单和原始字段不输出，均通过；**本轮未运行其主体或产品命令**。

另独立审读 [Vps-Qa3MediumStatus.ps1](Vps-Qa3MediumStatus.ps1)（SHA256 `b651ef9607c2dd6d3124beba88ff7965d336d4adf5d2e340ee20144ce8abd298`）及重跑其 72 项 AST/元数据/HGlobal/fake 生命周期测试，通过且未调用 native/token/child/main。它与 qa.2 冻结源的差异仅为候选说明、程序集命名、候选版本及 CLI/service 固定 hash。真正的 medium 原账户 CLI→SYSTEM 服务认证仍须另外授权并执行，源码/fake 或权限探针不能替代。
