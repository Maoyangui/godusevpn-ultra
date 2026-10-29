# 固定 beta 只读 wrapper 二审

日期：2026-10-19。快速独立只读审阅 `Vps-BetaReadOnly.ps1` 及其 Tests；未运行脚本 main、进程 runner、产品 EXE、传输脚本或任何真实策略/服务操作。

结论：本次未发现确认阻塞。静态确认：

- 目录从卷根到叶逐层 no-follow 打开，实际访问 `0x81` 含 LIST_DIRECTORY，shareREAD，目录/重解析属性通过句柄检查；句柄一直持有。
- QA 根及内部目录、三 EXE 要求 SY/BA owner，DACL 不允许其他 SID；三 EXE 先持有读/no-follow 句柄，再读取路径 ACL、固定 SHA256 和静态 ProductVersion。文件流/句柄保持到全部检查结束。
- 候选受固定 QA 根、UUID4 StageID、固定 delta leaf 和三个哈希约束。APPDATA 检查只针对当前令牌的用户 journal，不据此代表登录 GUI 用户或机器恢复记录；缺目录仅在已持有前缀下终止查找。
- 进程仅从内部固定 role/path 映射和五项计划生成参数，UseShellExecute=false。GUI 不在执行计划。15 秒、1 MiB、stderr、非零退出均中止；异常仅处理当前 child、终止未知不重试，不发起自动修复或保护变更。
- 输出仅固定版本、布尔、计数和允许的错误 code；原始报告、warning/detail、路径、账号/SID、IP/MAC 和环境值不进入结果。machineJournalChecked 保持 false，运行保护效果保持未验收。

仅运行了系统 Windows PowerShell 5.1 下的 `Vps-BetaReadOnly.Tests.ps1`，通过 AST/parser、内存 Reflection.Emit 元数据、固定五命令、合成 schema/空数组/unknown 计数、私有 SD 拒绝和敏感哨兵排除。测试没有加载或调用 main、`Invoke-BetaProcess`、native 文件方法或产品。输出 `productExecuted:false`。

这是脚本及纯合成契约审查，不等于真实 CreateProcess、令牌、ACL/文件共享组合或 VPS 五条命令已经通过。前提仍是可信 QA 祖先和明确批准的固定候选哈希；不声称抵御恶意管理员、内核或主动 DOS 命名空间突变。
