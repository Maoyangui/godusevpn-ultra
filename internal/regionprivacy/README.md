# 当前用户国家或地区

此模块实际接入 Windows 的用户国家或地区设置。它不更改 GPS、IP、显示语言、键盘、时区、浏览器环境或隧道；不会根据出口地址自动选择地区。其他应用可能缓存旧值，需重新打开后核对。

## 正式 API 与身份

- [GetUserDefaultGeoName](https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-getuserdefaultgeoname) 独立读取当前用户的 ISO 两字母或 UN M.49 三位代码。
- [EnumSystemGeoNames](https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-enumsystemgeonames) 使用 `GEOCLASS_ALL` 获取系统实际支持的代码；[GetGeoInfoEx](https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-getgeoinfoex) 只取名称，不取坐标。不会提供猜测的国家清单。
- [SetUserGeoName](https://learn.microsoft.com/en-us/windows/win32/api/winnls/nf-winnls-setusergeoname) 只在明确提交 Apply 或 Restore 后调用，最低要求 Windows 10 1709。政策禁止或缺少接口时拒绝，不通过注册表或命令行绕过。

每次读取/设置均核对当前进程普通用户身份，拒绝线程模拟、服务账号、提升或非 Medium 完整性。用户目录由进程 token 的 KnownFolder 获取，不接收前端目录、SID、用户名或管理员身份。此功能直接运行于 GUI 普通用户进程，**不通过 UAC helper 写替代账户的 HKCU**。`New()` 不访问系统或创建目录。

## 写前备份与恢复

固定目录为当前普通用户 LocalAppData 下 `GodUseVPN Ultra/PrivacyRegion`。目录及记录使用用户、SYSTEM、Administrators 三者专用的受保护 DACL；拒绝既有不符合 ACL 的对象，不自动修复权限。盘符/路径通过 `localstate` 先验证为本地固定盘，持有无删除共享的祖先/目录 handle，文件通过相对 `NtCreateFile` 创建/读取，拒绝重解析点和多链接文件。

本应用内互斥和固定空文件上的跨进程独占字节锁串行化操作。Report 不创建文件或目录，Restore 不创建缺失目录。读入的备份校验版本、大小、SHA、字段及所属用户；SHA 用于损坏检测，不能抵御有权限写该用户文件的同账户程序。

1. 读取系统当前值；已有记录必须与已知状态一致，否则保留外部变化并拒绝。
2. 保留第一次修改前的原值，先将 `applying/restoring` 记录持久写入。
3. 临时文件使用 CreateNew、同 handle 有界写入/短写检查、Sync，再相对持有目录 handle 原子重命名到固定记录；完成后重新读取文件核对。使用写穿透与文件 flush，不宣称已验证任意断电情况下的磁盘/目录元数据保证。
4. pending 落盘后再次读取当前值并核对用户身份，才调用正式 setter。setter 后 fresh 读取，再写 `active/restored`，最后再 fresh 读取一次才报告 `verified: true`。
5. 任一不确定结果保留恢复线索，不自动回滚。恢复只接受记录中的写前值或计划目标值，不覆盖不同的外部值。重复 Apply 不丢失第一次原值；恢复过程崩溃后可从 pending 继续。

**正式 setter 不提供原子 CAS 或所有权 token。** 与其他程序在检查后同时修改仍有竞争窗口，也无法辨别“先改走再改回相同值”的 ABA。上述防护能拒绝实际观察到的不同状态，不能承诺与所有外部程序原子互斥。此限制通过 Report 的 `atomicExternalCAS: false` 和 warnings 明示；本模块没有声称实现系统级事务。

## GUI 桥

- `PrivacyRegionReport()`：`supported/currentCode/choices/status/managed/canApply/canRestore/originalCode?/observedAt?/scope/detail/atomicExternalCAS/warnings`。
- `PrivacyRegionApply(code)`、`PrivacyRegionRestore()`：`status/currentCode/changed/verified/detail`，失败另返回固定错误码。

前端只能提交系统清单中的完整代码，不能输入脚本/路径/注册表键或改变身份。Apply/Restore 前后使现有隐私报告快照失效。UI 接线、用户确认和真实普通用户读回验收另行完成；仅有后端/API 不表示 H14 全部完成，显示语言、格式和键盘编辑仍未实现。

## 本批验证范围

`go test ./internal/regionprivacy` 全部使用内存假后端/假存储；Windows 测试仅检查合成 token、UTF-16 结果、回调缓冲区、ACL 策略、相对重命名 ABI、短写和 flush 失败，**不调用真实国家地区 getter/setter、token/KnownFolder/文件 ACL/原生存储**。`cmd` 桥用注入的假 manager 验证固定方法、逐字代码转发、隐私 epoch 失效及未请求 helper。

真实 Medium 用户的 API 可用性、系统列表/名称、本地 NT 创建/重命名/锁/ACL、修改与原值恢复、策略拒绝、两个产品进程互斥及外部 Windows 设置冲突仍须仅在获授权 QA VM 执行，尚未取得 VM PASS。
