# Windows 时区选择与恢复

此包只对用户明确选择的系统时区执行设置。使用 Windows 正式枚举/读取接口取得当前 ID 和候选项；设置只调用 `GetSystemDirectory()` 返回目录内的 `tzutil.exe /s`，参数是本次操作重新枚举后核实的单个时区 ID。没有 shell、PATH 查找、任意命令/参数、地理查询、外部 IP 请求或 UTC 时钟修改。

`New(dir)` 提供 `Report()`、`Apply(id)`、`Restore()`。报告包含 `supported`、`currentId`、`choices`、`managed`、`status`、`detail`；还显示 `daylightSavingDisabled`。用户原有夏令时偏好会随选择保留，恢复时还原首次修改前的 ID 和夏令时开关。语言、地区、键盘和输入法不在此功能中修改。

普通用户若有系统授予的更改时区权限即可由系统工具完成；没有权限只返回错误和手动设置说明，不触发 UAC、不影响 VPN、不修改其他保护。系统时区影响整机，Windows 自动时区或其他软件仍可能改动；本工具不擅自关闭它们。浏览器可以单独覆盖时区，系统回读不是网页指纹验证。

写入前持久保存原始值及 `applying` 事务，经过私有目录 ACL、临时文件 Sync 和 Windows write-through 原子替换。写前再次检查当前值。命令失败、超时、回读未知或完成记录保存失败均保留备份，不凭失败返回值假定系统没有变化。再次选择时保留第一次的原始值，并记录上一次已知状态，以恢复写前崩溃的中间事务。

恢复先比较当前 ID 与夏令时开关：若已为原值，只完成恢复记录；若仍为本应用的已知事务状态，则先保存 `restoring` 后恢复；外部变更则保留并报告冲突。完成回读之前不清除备份。损坏、超限、未知版本、非法字段/ID 或校验失败一律阻止 Apply/Restore，报告仍尽可能显示当前 ID 和可选列表。SHA-256 用于发现意外损坏，不能对抗拥有同等文件权限的恶意程序重新计算校验和。

Windows 没有与其他系统写入者共享的原子时区 CAS；本工具使用进程内互斥、跨本应用进程的文件锁、写前重读和写后校验缩小竞态窗口，并保留冲突。极短间隙中的外部并发写入无法获得绝对排除保证。原值按 Windows 时区 ID 和夏令时偏好恢复；非标准自定义时区、手工修改的转移规则不在本功能的恢复承诺内。时区数据库被系统更新后，使用更新后的系统规则。

测试全部注入 fake backend，没有调用真实 `tzutil /s` 或改变当前系统。测试覆盖写前备份、重复选择保留首次原值、权限失败、命令返回失败但已实际改变、回读未知、崩溃恢复、外部 ID/DST 变更冲突、持久化失败、损坏备份和非枚举 ID 拒绝。

官方依据：[tzutil 参数](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/tzutil)、[EnumDynamicTimeZoneInformation](https://learn.microsoft.com/en-us/windows/win32/api/timezoneapi/nf-timezoneapi-enumdynamictimezoneinformation)、[DYNAMIC_TIME_ZONE_INFORMATION](https://learn.microsoft.com/en-us/windows/win32/api/timezoneapi/ns-timezoneapi-dynamic_time_zone_information)。
