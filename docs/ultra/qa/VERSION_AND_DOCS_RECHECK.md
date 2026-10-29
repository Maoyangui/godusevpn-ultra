# 两段 beta 版本与发布文档复核

日期：2026-10-19。仅审阅 Ultra 副本，未改产品、构建脚本、安装脚本或根 README。新增的测试只处理 AST、临时惰性文件及纯 CLI 函数，不执行安装、服务、UAC、注册表、网络策略或产品 EXE。

## 版本结论

根任务将两份安装脚本中的版本规则改为允许两段/三段后，未发现仍会阻止 `0.01-beta`、`0.01-beta-qa.2` 或 CI 标签的 Windows 发布入口。

| 入口 | 当前检查与边界 |
|---|---|
| `Install-Ultra.ps1`、`Uninstall-Ultra.ps1` 的 `Test-Package` | 同一两段/三段规则；本次实际调用从 AST 提取的验证函数，对完整临时 manifest 和描述文件检查，而非仅重复正则 |
| `build-ultra.ps1` | ValidatePattern 接受两段/三段；数字词转换后补齐四段，限制单词在 0–65535；通过 go-winres 覆盖模板和 ldflags 注入当前标签 |
| `verify-ultra-package.cjs` | 两段/三段映射到 Windows 数字版本，真实三 PE 固定版本及各语言 ProductVersion 必须匹配描述文件；此前 25 项合成 PE/ZIP 回归另有记录 |
| CLI `verifyUninstallManifest` | 只校验产品身份和恢复文件 manifest，没有三段版本门槛；本次纯函数用五个真实标签形状通过 |
| Windows 发布工作流 | 精确 `v0.01-beta` 标签；分支/PR 使用 `0.01-beta-ci.<7sha>`；QA 标签由本地构建显式传入，不触发公开发布 |

剩余的 `internal/update.parse` 仍按旧版三段解析；Windows 桌面的自动检查、手动检查和安装均有 `identity.Ultra` 提前返回，因此它不阻塞本次 Windows ZIP 安装/卸载，也不能被当作已经提供 Ultra 更新支持。未随 Windows 包发布的 Web/Android 旧桥仍保留原上游更新路径，后续发布这些平台前必须独立处理。`winres/*.json` 的开发默认 ProductVersion 与 `internal/buildinfo.Version` 的开发默认值也不是发布标签；应使用审核过的构建脚本，不能将任意 `go build` 结果称为 beta 包。

## 已运行的合成检查

```powershell
./docs/ultra/qa/version-compatibility.ps1
```

54 项通过：

- 解析两份完整 PS 脚本，仅定义白名单中的五个校验函数；从不执行其顶层安装/卸载流程。
- 两份 validator 各接受 `0.01-beta`、`0.01-beta-qa.2`、`0.01-beta-ci.abcdef0`、`0.1.0-ultra.3`、`1.2.3`；拒绝一段/四段、`v` 前缀、空后缀、路径、空格等八个无效标签。EXE fixture 仅是不能运行的文本。
- 从构建脚本 AST 读取实际 ValidatePattern，用同一组标签验证；不执行资源生成或 Go 产品构建。
- 将当前 `internal/cli/uninstall_manifest.go` 原文复制到独立 Temp module，运行五标签的纯函数测试。强制 `GOTOOLCHAIN=local`、`GOWORK=off`，仅标准库，不拉取依赖。结束恢复进程环境并在确认新 Temp 根边界后清理。

该测试不替代 PE 或 ZIP 复验，也没有声称真实安装/卸载、运行时版本注入或 Windows 多版本兼容通过。

## 文档结论

阅读英文 README、中文说明、功能台账、审计台账、beta release notes 和中英恢复说明。七份文档的相对链接目标全部存在。源码构建明确包含 `README.md`，当前没有“包中遗漏英文入口”的源码缺口；最终 ZIP 仍须由构建包审计核对。

现有文档准确保留未签名、C04 部分修复、浏览器/WFP 真实行为待验、旧 v1 证据仅适用于旧包、Web/Android 诊断导出收窄、路线图未交付等边界。未发现把 fake/TEMP/DOM 等同于本轮实机通过的新承诺。

已向根任务回报两个文档可用性建议，未自行修改其拥有的 README：

1. 中文说明的“依赖和上游的最低系统要求见原 README”应改为明确上游链接；本仓库 README 已成为新的英文 Ultra 首页，原措辞不再能定位旧依赖说明。
2. 新报告/图片工具只接受固定本地盘普通子目录，不支持 USB/移动盘、UNC/映射/SUBST/重解析或直接卷根。这已在 `FILE_TOOL_PARENT_GUARD_FIX.md` 写清，建议在中英用户入口各补一句，避免用户把安全拒绝误认为工具无故失效。

以上两项不构成新确认安全绕过；实际 GUI/原生保存框、管理员令牌、SCM、SYSTEM 机器恢复和真实网络行为继续按主台账验收。
