# 固定 beta 完整包组装准备

日期：2026-10-19。新增 `Assemble-VpsBetaPackage.ps1` 和 Tests。它只从已验证的三个 EXE 与固定非 EXE 支持 ZIP 创建一个新包；源代码没有进程启动、产品、decoder 或 installer 执行路径。

固定输入支持 ZIP 为 10,313,075 字节、SHA256 `99f81bce6e6a432c7be28267627f7f8e7ab36e3b3de886cc0f7894e2d95d4131`，300 个无 prefix、无目录条目的非 EXE 文件；其中原 `SHA256SUMS` 为 `9f4d7836cefd477fed1b1f16247ae1c389aa7eed818c11f18a9185c8fc8eaf6d`。输入路径固定为 SupportStageID 下的 `preview.zip`、CandidateStageID/CandidateLeaf 下的三个 qa.2 EXE；目标固定为 candidate 的 sibling `package-beta-qa2`。

关键边界：

- 内嵌前批已审的 11 个 native pin/私有 SD/路径/hash 函数，只重命名并将设备别名上标写为 ASCII `\u` regex；不依赖云端额外函数库。
- StageID 为 UUID4 32hex、candidate 为固定 delta leaf，使用 ProgramData known folder。逐层 LIST_DIRECTORY/shareREAD/no-follow pin；QA 根及内部对象须 SY/BA owner 且无其他 SID allow ACE，遇异常拒绝，不修 ACL。
- ZIP 固定字节/摘要先于解包；成员 count、无重复/大小/安全路径/普通文件属性严格检查。只允许固定 manifest 所列的 299 个非 EXE payload 加 manifest；三个 EXE 只能由已验证 candidate 读取，不从 ZIP 引入。
- manifest 必须恰好 302 个 payload，三个 EXE 摘要与独立固定值一致；支持文件逐个 SHA 相等，包 descriptor 为 qa.2/amd64/unsigned Ultra。创建任何输出前，三 EXE 均已核固定 SHA 和静态 ProductVersion。
- 全新目录 native exclusive 创建，文件 CreateNew、flush 后再读，逐目录/文件验证私有属性和权限；不覆盖、删除、迁移或 retry。失败保留不完整新目录。
- 终态独立枚举精确 303 个文件（302 payload+原 manifest）、全部文件实际 SHA，以及三个实际 ProductVersion，全部通过才输出固定计数/布尔/hashMatch。输出限 2000 字符，不含路径/环境/原始文件内容。

已在系统 PS5.1 运行 34 项 AST/内存 fake：Parser、native metadata、UUID/路径/设备名/私有 SD、302 项 manifest、300 项内存 ZIP 及 extra/duplicate/path/link/EXE 注入拒绝。测试没有 native 文件调用或 main。

另对实际固定支持 ZIP 只读载入内存，调用 AST 白名单纯函数验证：300 成员、302 payload manifest、固定 ZIP/manifest 摘要、每个支持文件 SHA、三个预期 EXE SHA 和 descriptor 均通过。没有解压落盘、组装包或运行任何程序。真实私有 ACL/目录共享/303 文件创建组合仍交 ROOT 独立复核和文件测试，不把本次纯函数通过冒充已云端组装完成。

可信 QA 祖先、操作系统及管理员仍为前提，不抵御恶意管理员/内核或主动 DOS 命名空间变化。新对象使用已核私有父目录的继承权限；出现 owner/ACL 不符会停留失败，不为通过测试而接管旧对象。工具没有安装、UAC、服务或网络策略行为。
