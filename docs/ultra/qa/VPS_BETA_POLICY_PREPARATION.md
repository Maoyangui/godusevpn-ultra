# qa.2 四项机器策略往返：准备记录

2026-10-19。本记录只说明脚本/fake准备；**未执行脚本主体、产品CLI或任何注册表写入**。运行前由ROOT另行说明具体四项修改、恢复路径并取得用户明确确认。

脚本：[Vps-BetaPolicyRoundtrip.ps1](Vps-BetaPolicyRoundtrip.ps1)。测试：[Vps-BetaPolicyRoundtrip.Tests.ps1](Vps-BetaPolicyRoundtrip.Tests.ps1)。Windows PowerShell5.1 x64、实际SYSTEM上下文、私有QA目录的固定qa.2 CLI是前提，不接受任意程序/策略/路径参数。

CLI SHA256固定为`7eed52a1e1e6f4424db54f4a2b067346f2dc7bd9df734687529422294cd9673e`，同时检查版本资源`0.01-beta-qa.2`；读回只针对固定64位HKLM目录。

| ID | 固定值名 | 本批目标DWORD |
| --- | --- | ---: |
| windows-advertising-id | DisabledByGroupPolicy | 1 |
| windows-online-speech | AllowInputPersonalization | 0 |
| windows-clipboard-history | AllowClipboardHistory | 0 |
| windows-clipboard-cloud | AllowCrossDeviceClipboard | 0 |

目录、值名、类型与`internal/privacy/catalog.go`四个system控制一致。没有浏览器/HKCU/时区/防火墙、网络或服务写入项目。

## 拒绝与恢复规则

1. 既有QA祖先和CLI由root-first、LIST_DIRECTORY/普通文件no-follow句柄保留，固定磁盘映射、SY/BA owner和role-only ACL受检查。既有目录/ACL不修复、不接管；脚本本身不创建或归档产品目录。
2. 所有四个目标值必须原本不存在；机器journal必须不存在，或checksum正确、固定machine owner、canonical当前格式且controls为空。每项开始前再次检查四值缺失和空journal。异常旧v1目录由ROOT独立审核/归档，此脚本直接拒绝。
3. 每项只运行一次固定`privacy apply <ID>`。仅零退出、合法固定ID、configured、changed=true，再独立读到该ID精确active备份（Before不存在、Desired固定值）和私有`privacy.lock`，才记录本批归属。随后独立HKLM读回值/类型。
4. finally只允许本批已确认归属的当前ID进行一次`privacy restore <ID>`。恢复前再次核对精确备份与当前值，发现外部冲突就保留；不存在force、restoreAll或针对其他ID的自动恢复。
5. restore后独立确认目标值不存在、canonical journal为空。任何失败都停止后续项；成功恢复了一个失败用例也只记recovered_after_failure，不升级成往返成功。
6. 每个子进程15秒、输出有界，错误原文不公开；超时/不明输出/非零/异常视为结果未知，不自动重试或猜测归属后恢复。只尝试终止本次创建的Process；终止无法确认则单独标记，保留备份待人工处理。

`lockObserved`只证明命名锁文件存在、普通单链接与私有ACL被独立读取，不表示已经独立观察到进程执行中持有字节锁或全局mutex。活动备份的事后读回同样不冒充逐指令采集到写前持久化时序；源代码和产品回归另提供该顺序的证据。

脚本没有跨整个测试批次拦截外部管理员的写入。每步重检和产品CAS限制已观察到的冲突，不能消除所有其他管理员/同值重写或命名空间竞态。真实系统行为、系统是否接纳这些策略、跨进程锁、崩溃与恢复仍须具体受控测试。

预检与产品apply之间并非一个原子操作。若外部策略在此间出现，产品可能备份该外部原值并写入目标值；脚本随后因备份不符合“原值不存在”的条件而停止，不猜测归属或擅自恢复，须人工核对。最初不存在机器目录/journal/lock时，成功往返也可能留下新建的私有空journal和lock；“恢复”表示目标值及managed记录恢复为空，不表示目录与字节布局完全回到运行前。

## 已执行的有限验证

仅在Windows PowerShell5.1执行Tests文件。它解析产品QA脚本AST，只载入固定纯函数和内存native metadata工厂；不载入主流程、进程runner、目录或注册表读写函数。

- 四个固定ID/目标DWORD、当前Go writer canonical空/active journal样本、SHA、错误owner/阶段/Before/重复字段/超限拒绝。
- native DllImport签名/错误元数据、纯路径字符串拒绝和内存SDDL限制；未调用原生文件API。
- 10个fake批次：四项成功仅八次固定调用；初始值/旧journal拒绝零写；apply超时、无change、坏备份、缺锁不猜测恢复；外部冲突保留；确认归属后读失败只恢复该ID一次；restore未知不重试、后续项停止。
- 成功结果JSON长度界限和不含原始字段，固定调用无restore-all/服务/连接动作。

均通过。该结论不是SYSTEM真机往返通过；固定qa.2源码/哈希也不能让旧v1单项结果自动适用于本批四项。没有实际策略、CLI、GUI、服务、UAC、网络或浏览器操作。
