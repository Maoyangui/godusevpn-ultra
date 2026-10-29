# Windows beta 发布流程二审

日期：2026-10-19。范围：Windows 首发 beta、发布工作流、构建脚本、源代码发布检查和构建包验证。未运行产品 EXE、安装程序、服务、UAC、真实网络策略或诊断采集；未修改原项目目录。

## 已确认并修复的发布门禁缺口

1. 原包检查只核对目录内的 `SHA256SUMS` 和 ZIP 的旁置校验值，未证明 ZIP 中的文件等于被检查的目录。替换 ZIP 后重算旁置 SHA256 仍可能通过。现在直接解析实际 ZIP，核对全部文件成员、解压后字节数、CRC32、SHA256 与审过目录，拒绝缺项、多项、重复/越界路径、链接成员、重叠记录、加密、ZIP64 和超限内容。支持当前构建使用的 stored/deflate 与 data descriptor，不解压到磁盘。
2. 原 PE 检查读清单后返回描述文件的版本，没有读取真实 `RT_VERSION`。现在对 GUI、CLI、服务程序逐个读取 PE `RT_MANIFEST` 和 `RT_VERSION`；核对架构、`asInvoker`、本产品 assembly identity、固定 FileVersion/ProductVersion、各语言表的版本字符串。仅修改 `ULTRA_PACKAGE.json` 版本不能通过。标签 `0.01-beta` 保留在 ProductVersion 字符串，Windows 数字版本为 `0.1.0.0`。
3. 原工作流可从宽泛的 `v*` 标签发布。现在仅 `v0.01-beta` 可进入正式 beta 发布 job；分支/PR 构建使用 `0.01-beta-ci.<7 位提交号>`，并增加验证器合成回归门禁。QA 构建脚本仍可显式构建 `0.01-beta-qa.1`，不会由标签发布 job 自动公开。

验证器 JSON 只包含相对文件名、大小、版本、摘要和检查范围，不包含本机用户名或绝对目录。它明确输出 `runtimeVersionVerified: false`：本次没有执行 EXE，也没有声称已独立验证 Go 运行时注入版本。最终源码构建和 VPS 实测另计。

## 本轮验证

- `node tools/audit-built-package.test.cjs`：25 项通过。包含完整合成 PE/包通过、deflate+data descriptor 通过，以及 ZIP 字节篡改后重算 CRC/旁置 SHA256、缺项、多项、重复成员、路径越界、加密、局部记录不一致、错误 CRC、符号链接属性、ZIP64、膨胀限制、注释、错误旁置摘要、四类真实 PE 版本不一致的拒绝用例。仅使用临时合成文件，退出时校验临时目录边界后清理。
- `node --check tools/audit-built-package.cjs` 和 `node --check tools/verify-ultra-package.cjs`：通过。
- 只读验证现有旧 v3 包的三个 PE：实际固定版本均为 `0.1.0.0`，ProductVersion 为该包原标签 `0.1.0-ultra.3`，清单身份/权限一致。没有改写旧包。
- 只读验证旧 v3 包目录/原 ZIP/旁置摘要：225 个目录文件、224 个 manifest 项、225 个 ZIP 文件成员全部一致；ZIP 为 33,281,429 字节，SHA256 为 `97dbdf1efa7f80a2af8a623fbef4b1c678ab1c2095b2339ce26dbf7b96d66541`。旧 v3 的前端已经过时，因此没有将它描述成当前源代码前端检查通过。
- 原项目 Git 工作树和暂存区均无差异，420 个跟踪文件，基线提交 `b51aed4fd0958a8abf461a74c453d432fca155d4`。检查不写入原目录。

## 发布条件和边界

源代码扫描是启发式发布卫生检查。它扫描 Git 跟踪和未忽略的源文件，检查若干密钥/资源身份/含凭据 URL 模式，只输出文件名和行号。含 NUL 或超过 8 MiB 的内容不做文本模式扫描；无通用明文密码、UTF-16、二进制秘密的完整识别保证。真实发布还需人工检查待发布文件清单及历史，不能把“扫描通过”称为没有任何秘密。本轮没有放宽扫描器或忽略发现项。

初次源扫描发现 12 个 URL 项，已交根任务分类和修正；这份旧扫描结果不是最终发布通过证明。最终源扫描、README/release notes、QA dependency lock、全量回归和新候选包审计由根任务继续完成。构建脚本已加入英文 README，以及资源生成前捕获的 `sourceCommit`/`sourceDirty`，本地 QA 包可能为 dirty；公开 release 应从明确审核的提交构建。

工作流顶层权限为 `contents: read`，只有精确 beta 标签的独立发布 job 请求 `contents: write`；checkout 不持久化凭据，所用 actions 固定到提交。当前流水线为 Windows amd64 首发，不发布旧 Linux/macOS/Android 包。构建、测试及 `-WhatIf` 校验不等于真实管理员安装/卸载、已安装普通用户 GUI 身份查询、WFP 故障恢复或 VPS 验证通过。

目录校验、清单、实际 ZIP 成员和 PE 元数据能发现产物混入、篡改和明显版本错误；它们不构成代码签名、可信发布来源或运行时行为的证明。源目录应由可信构建进程控制；此工具不是对同一账号可并发改写构建工作区的恶意进程提供原子快照认证。
