# 隐私助手授权与 Windows 10 兼容边界

当前普通 GUI 启动默认不请求管理员权限，也不创建隐私助手进程。启动只初始化界面、托盘、隧道状态和只读页面；`PrivacyHelperStatus` 在尚未操作时返回 `not_requested`。

用户在隐私页点击“请求管理员”，或在需要独立浏览器网络隔离的操作中明确点击授权后，才调用 `PrivacyAuthorize`。UAC 取消、助手启动失败、管道失联或超时都保留普通界面和原有隧道，不自动重试，也不把助手状态当成保护已生效。需要管理员助手的机器策略、程序规则和浏览器隔离操作在授权前保持不可用。

Windows 10 的身份边界与 Windows 11 相同：普通 GUI 在提权前获得当前交互用户 SID，通过受 ACL 保护的管道传给提权助手；助手重新查询父 GUI 的 PID、映像、会话和 token SID，必须逐项匹配后才建立会话。助手使用该原始 SID 管理用户范围的恢复记录，不使用提升账户的 HKCU 或 LocalAppData。安装为 `requireAdministrator` 的整个 GUI 不属于此合同；若未来采用该清单，必须保留未提升 launcher 传递原始 SID，不能从提升后的 token 猜测用户。

本说明记录源码行为和纯测试边界。Win10 22H2 Pro 的标准用户普通启动实机记录见 [QA12_10_WIN10_STANDARD_GUI_2026-10-27.md](QA12_10_WIN10_STANDARD_GUI_2026-10-27.md)；该记录覆盖 GUI、WebView2、服务共存和“无无关 UAC”路径。Windows 10 21H2/22H2 的具体补丁、WebView2 缺失、UAC 策略差异、WFP/设备能力和代理连通性仍需在对应环境单独回归。

纯回归 `TestStartupDoesNotAuthorizePrivacyHelper` 只解析 `startup` 与 `PrivacyAuthorize` 的 Go AST，不启动 GUI、UAC、服务、helper、网络或系统 API。
