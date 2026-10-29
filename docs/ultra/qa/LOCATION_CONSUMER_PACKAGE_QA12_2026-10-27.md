# QA12：独立 Windows 位置消费者的打包门

本批把只读 Windows 定位消费者纳入 x64 正式包构建，不把本机静态编译、纯测试或门户配置回显当作 GPS 实测成功。ARM64 包不声明或携带此 x64 组件。

`location-consumer/Build.ps1` 仅调用 Windows 自带 C# 编译器和 WinMetadata 编译源码，不运行定位入口。组件固定为 `asInvoker`，运行时最低配置为 .NET Framework 4.6.2；已纠正早期配置要求 4.8、无法覆盖 Win10 1809 默认 4.7.2 的问题。具体 WinRT 接口和位置权限仍须在每个目标 VM 检查。

候选包包含固定两文件 `location-consumer/GodUseVPN.Ultra.LocationConsumer.exe` 及其 `.exe.config`。包描述符声明实现、架构、最低 Framework、未捆绑系统运行时和 `vmValidated:false`。构建审计核验真实 x64 PE 的唯一 manifest、组件身份和普通用户权限；配置必须与固定最低运行时配置逐字节相同，不能增加未知加载配置或 DLL。两文件同时纳入包的 SHA256SUMS 与实际 ZIP 对照。

安装和卸载的只读包验证均拒绝缺文件、额外依赖、错误架构、虚假 VM 通过标记、未声明组件和错误运行时配置。升级或卸载发现固定安装目录的消费者仍在运行，或者不能确认进程路径时会停止，不杀进程。其他目录的同名程序不因此被终止。

本批验证结果：合成 PE/ZIP 变异测试 75 项；安装/卸载 AST 验证与惰性 TEMP 文件测试 42 项；既有包哈希与独立冷 Windows PowerShell `-File -WhatIf` 回归 46 项，均通过。真实源码静态编译后，独立只读 PE 审计读到 `asInvoker`、x64、固定身份与 4.6.2 配置。没有执行消费者定位主入口、安装器主流程、系统设置或本机网络操作。

应用的消费者启动门、门户写入与原值恢复、独立新鲜位置读回，以及 Win10/Win11 多版本行为仍须各自 VM 实测。只有这些实际结果才可以补充 GPS 功能验收状态。
