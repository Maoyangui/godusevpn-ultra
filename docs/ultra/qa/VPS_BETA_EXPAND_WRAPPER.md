# 固定 beta 文件重建 wrapper

日期：2026-10-19。新增 `Expand-VpsBetaDelta.ps1`，仅用于固定 QA 候选的文件重建，不执行产品 EXE。

输入仅 StageID、BaseStageID（32 位小写 UUID4）和 `delta-<32hex>` CandidateLeaf。路径固定从 Windows ProgramData known folder 下 `GodUseVPN-Ultra-QA` 构造。先逐层 no-follow、LIST_DIRECTORY/shareREAD pin，并验证 QA 根和内部目录/文件为 SY/BA owner 且没有其他 SID 的 allow ACE。既有不符合对象拒绝，不修 ACL、不移动数据。

固定输入：`preview.zip`，16,013,722 字节，SHA256 `af5513ea94de4118e2f72bc650da4fee2d18610925a38a3b17dc421b6f885b7f`。仅允许 `vps-beta-qa2-transfer-v2/` 下 manifest、decoder 和三个固定 delta.gz 文件，共五项；拒绝目录项、重复、多项、越界名称、链接属性和长度超限。读取仅在内存中展开；先核 manifest 与 decoder 的固定摘要，再核 manifest schema、固定目标三个 SHA、所有 patch 的长度和 SHA。

在固定旧基线 `base-for-beta-qa2` 校验三个来源文件 hash/长度后，新建 `transfer-beta-qa2`，五个文件均 CreateNew、flush、再读验证，并保持文件读句柄。现有 transfer 或 candidate 拒绝；失败不删除或续传。

只启动已固定的系统 Windows PowerShell 5.1，用固定参数调用已验证 decoder。child 限 120 秒和 4096 字节 stdout，任何 stderr、失败或异常均停止；异常终止状态独立记录，不重试。它不调用任何产品 EXE。decoder 成功 receipt 后，独立重新检查三个新 EXE 的 private ACL、文件属性、长度上限与固定 qa.2 SHA，全部通过才输出 `ready:true`、`files:3`、`hashesMatched:3`、`sha256Match:true`、`productExecuted:false`，总输出限 2000 字符。

实际仅在系统 Windows PowerShell 5.1 运行 `Expand-VpsBetaDelta.Tests.ps1`：30 项通过，包括 Parser、内存 Reflection.Emit 元数据、UUID/路径拒绝、私有 SDDL、整数类型、内存 ZIP 的固定五项/重复/extra/越界/link 拒绝、hash/外层固定包拒绝。测试不加载 main、native 文件调用或 child runner，不执行 decoder，不写文件，不重建真实候选，也不执行 EXE。

随后只读检查实际 `dist/vps-beta-qa2-transfer-v2.zip`：长度和上列固定 SHA 一致，确有五个文件、没有显式目录条目。成员为 decoder（13,611 字节）、CLI delta（2,139,128）、服务 delta（10,411,745）、GUI delta（3,478,519）和 manifest（1,520）。另外在 PS5.1 从 AST 仅加载纯函数和四组固定常量，运行实际 ZIP 的内存 `Read-ExpandBundle`：五成员、三项 manifest 及所有固定/patch SHA 通过；main、decoder 和产品均未运行，也没有落盘展开。

ROOT 在实际云端执行前独立复核。此记录不表示 staging 传输、真实目录/file-sharing 组合、decoder 启动或三 EXE 云端重建已经通过。可信祖先/操作系统及独立批准摘要仍是前提；不防御恶意管理员、内核或主动 DOS 命名空间突变。新目录/文件继承已核私有父目录；失败保留部分产物供人工检查，不自动回滚或覆盖。
