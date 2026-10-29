# VPS QA 增量传输工具二审

日期：2026-10-19。工具仅重建待测三个 EXE 的字节，不运行产品、不调用 Azure、不修改安装/服务/策略。此次审查和回归仅在新建 Temp 合成目录进行；没有读取 VPS 凭据或执行 EXE。

## 范围与确认结论

检查 `Build-VpsDelta.cjs`、`Apply-VpsDelta.ps1`。它们是 QA 辅助，不是产品更新通道或发布认证机制。

- 输出文件名固定为 GUI、CLI、服务三个 EXE。manifest 必须恰好含三项、名称无重复、patchFile 与固定 EXE 名对应；不接受任意目的文件名。
- manifest 独立预期 SHA256 在解析前核对；每项进一步绑定 base/patch/target 的长度及 SHA256。manifest 与旁置 patch 自洽不能替代通过独立渠道批准的 manifest hash。
- 所有长度限制为整数并受上限控制；u32 解码值转入 long，copy/literal 用剩余长度检查，防止越界、零长度、截断、未知操作、尾部指令和命令数超限。解压后的 encodedLength、目标长度及目标 SHA 分别校验。
- 全部三个目标先在内存通过验证，才原子创建新的 `delta-<32hex>` 目录。存在目录拒绝；文件 `CreateNew`，无覆盖/续传/删除/ACL 接管；I/O 中断的部分结果保留供人工检查。
- 仅接受已核对的固定本地盘路径，逐级拒绝已有重解析目录。工具没有声称可以抵御可信 staging 祖先被同账号/管理员主动并发改向。

## 二审发现与修订

1. 初版 `blockBytes -ne 256` 会进行 PowerShell 隐式比较，JSON 字符串 `"256"` 也可能通过。作者已改为 `Assert-Integer ... 256 256`，并加入字符串拒绝用例。
2. 实际 Windows PowerShell 5.1 中，GZipStream 会拒绝完整 trailer 的错误 CRC/ISIZE，却可能接受缺失 trailer 或末尾无关数据。二审使用惰性 Temp fixture 修改 gzip、重算 patch hash 和独立预期 manifest hash，隔离解压器行为：CRC/ISIZE 改错拒绝；删末 1 字节、删整个 8 字节 trailer、追加 3 字节垃圾在初版中仍返回 ready。目标 SHA 仍正确，故它是 gzip 格式完整性门槛缺口，不是 manifest 认证或任意目标替换绕过。作者已补固定基础头、footer ISIZE 和解压结果 CRC32 显式校验；二审独立复验五项均拒绝，且均未创建输出目录。工具明确不提供通用 deflate/串联 gzip 边界认证，批准的压缩 SHA 与最终 EXE SHA 继续作为内容信任依据。
3. `Assert-Local` 初版将卷根 `C:\` 去尾斜线后返回 `C:`。实际 PS5.1 的 `Path.Combine` 会产生 `C:leaf` 驱动器相对路径，PS7 则表现不同。用纯字符串证实后，作者最小改为在 DriveInfo/属性查询前拒绝卷根输入。二审从 AST 只加载两个函数，在 PS5.1 中注入 DriveInfo 构造拦截，确认卷根返回 unsafe_path，未调用该探测。没有对真实卷根进行读写。

## 必须由远程执行 wrapper 保证的前提

- 创建/核对固定 staging 父目录的可信 owner 与私有 DACL，不能把工具的 local/reparse 检查当作 ACL 验证。输出 ACL 继承父级；工具不自动修改既有 ACL。
- 固定本次已批准 manifest hash，使用可信的 base，且工具/传输块本身来源可信。独立 hash 不能来自同一未认证输入。
- 工具的 `sha256Match` 首先表示重建的内存字节通过核对；在执行前，wrapper 应重新读取三个落盘 EXE 的 SHA256，与已批准的 newSha256 比较，并维持受控目录边界。
- 建议独立启动系统 PowerShell `-NoProfile -NonInteractive -File`，检查退出码和结果 JSON，再由父 wrapper 做落盘复核。脚本含 `exit` 和固定 Add-Type 名称，不能仅假定 dot-source 后续步骤一定执行。
- 重建通过不是执行许可，也不是运行时、服务、隧道或真实保护效果验收。本轮未执行任何重建目标。

## 已执行的检查

- `node --check docs/ultra/qa/Build-VpsDelta.cjs`：通过。
- PowerShell Parser 解析解码脚本：通过，没有执行其顶层逻辑。
- 作者原自测的初次独立重跑：5 个算法往返、5 个坏指令流、8 个 PS5.1 用例通过；修订后的最终独立重跑为 5 个算法往返、5 个坏指令流、17 个 PS5.1 用例通过，含既有输出哨兵不变、base hash、解压上限、整数类型及 gzip 损坏用例。
- 二审独立 PS5.1 CRC/trailer 五探针在最终修订中均返回失败、ready=false、输出目录不存在；根目录合成拦截通过。所有输入/输出都是临时惰性数据，按明确记录的文件列表清理，未碰产品/用户目录。

在上述可信 staging 和独立 hash 前提下，本次没有剩余确认阻塞。此前发现项均保留触发与修复记录，不能把二审发现的旧分支问题重复算成最终未修漏洞。

另按根任务要求，Windows workflow 的合成验证步骤已加入 `version-compatibility.ps1`。该步骤先检查 Node 验证器退出码，再运行 54 项版本 fixture，不执行实际安装。
