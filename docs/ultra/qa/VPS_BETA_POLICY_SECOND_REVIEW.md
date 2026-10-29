# 四项 SYSTEM 策略往返脚本二审

日期：2026-10-19。只读审阅 `Vps-BetaPolicyRoundtrip.ps1` 和 Tests。未运行 main、runner、产品、UAC 或任何真实注册表/恢复存储读写。本文件不是执行授权。

## 结论

在可信测试环境和明确授权的四项范围内，未发现确认的危险写入/恢复绕过。固定 catalog 的 ID、HKLM 64 位路径、值名、DWORD 目标与产品源码一致；CLI 固定为 qa.2 SHA，要求真实 SYSTEM 身份和管理员角色，先 pin 受保护 QA 目录及 CLI，再核 hash/静态版本。私有 store 目录/文件、单链接、no-follow 和 canonical checksum 检查不会为通过验收而修改 ACL、迁移或删除旧记录。

初始四值必须全部缺失、machine journal 必须为合法 empty，且每一项应用前重检。只有 apply 收到本 ID 的 `configured`、布尔 `changed:true`，并独立读到精确 `active`、原值缺失的固定 backup 和 lock，才取得本次 owned 标记。finally 只允许该 ID 一次 restore；必须再次核对相同 backup 和当前值。当前外部值不同、journal 不符、超时/非零/结果未知均保留待审，不 retry、不 restore-all，不继续下一项。已提交且确认的 restore 还要独立确认原值缺失与 journal empty，才计为完成。

机器 journal 的 payload 与当前 Go writer 结构、属性顺序、base64 DWORD、`machine-policy-store-v1` 和 SHA256 核法一致。空/非当前版本、legacy account、重复/未知字段、applying/restoring、原值已存在或错误 checksum 不被误认为本次可恢复记录。读 snapshot 的 native lease 在 CLI 写操作前关闭，不长期占有 journal 的读共享锁阻断原子替换。

输出仅固定 ID、状态、布尔与计数；registry value、payload、原始 stdout/stderr、路径/用户/SID 不输出。SYSTEM 只证明本次令牌与 HKLM 往返，不代表浏览器/剪贴板运行效果；runtimeBehaviorTested 保持 false。

## 已运行的纯验证

在实际系统 Windows PowerShell 5.1 下，AST 白名单函数/内存 Reflection.Emit 元数据、canonical journal/hash、设备路径/SDDL 与 10 个纯 fake 状态机用例通过。覆盖初始值/旧 journal 拒绝、apply timeout/nochange、坏 backup/缺 lock 不恢复、读失败后已拥有项一次恢复、外部冲突保留和 restore unknown 不重试。没有加载 main/runner/真实文件或 registry 方法。

另用纯函数确认零字节 lock 的单对象返回在 PS5.1 中仍是非 null byte[]，不会把存在的空 lock 误判缺失；未打开真实文件。作者已将 UTF-8 无 BOM 中非 ASCII 设备别名字面量改为 ASCII `\u` regex，并补上标字符构造 fake；独立重跑 PS5.1 Tests 通过。最终脚本 SHA256 为 `8493e46f079c9f5fdddd8ae90b9a3f20ee209ecda69269c0ef547ddfa93b8f44`，不把初版问题当作最终未修漏洞。

## 不能外推的边界

- 若初始 machine 目录、lock 或 journal 不存在，首次合法 CLI 操作可能创建它们；恢复完成后会保留 SY/BA 空 journal 和 lock。完成表示四个值恢复缺失、managed 记录为零，不表示文件系统从不存在回到字节级完全不存在。脚本不删除这类恢复工具/记录。
- 预检与 CLI 应用是分离的事务。若其间外部管理员/GPO 写入策略，产品可能先备份该原值再应用目标；wrapper 随后的精确 originally-absent backup 校验不会取得 ownership，也不强行恢复，会停止待人工核查。它不保证阻止所有并发外部写入。测试应在受控、没有其他策略写入者的环境执行，不能为消除冲突自动清理外部值。
- 只读预检可能严格拒绝角色可信但含其他 SID 只读 ACE 的父目录；这是安全失败，不应为了测试成功擅自收紧旧 ACL。异常旧空 store/额外文件会保留并拒绝，不被悄悄接管。
- 纯 fake 通过不等于 SYSTEM 真恢复、锁/ACL/文件共享组合或真实四项往返已通过；真实动作仍由用户单独授权。
