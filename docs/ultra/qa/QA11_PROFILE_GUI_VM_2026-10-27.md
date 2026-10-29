# QA11 配置库：真实 GUI 与独立磁盘有限验收

日期：2026-10-27。临时 Windows VM、普通 Medium 用户、已安装的 qa.11 GUI。包与安装身份见 [QA11 官方升级记录](QA11_OFFICIAL_UPGRADE_2026-10-27.md)。本次使用合成配置，实际 GUI 操作与独立只读检查分别保留证据；未用 CLI、直接产品 API 或修改磁盘记录代替 GUI。

## 方法与证据边界

每次操作前后分别运行已审的只读 auditor，取得真实 baseline。auditor 检查固定原用户配置库、记录格式 / 哈希链、文件 ACL 与直接同级项的名称哈希，不写产品数据。重命名、克隆和删除再由冷 WinPS 5.1 子进程比较这两份报告，并结合人工观察的 GUI 动作 / 预期名称。

本批实际使用的补充工具身份为：

- cold launcher：7,037 字节，SHA256 6fdb0a18908f5291614314c067047f9afd39dc30e2ed67863613cb29d4516bb0。
- 冻结的只读 Compare：17,503 字节，SHA256 4b7e2d25713d1a634c13489972641df6041861b351aa126cfa69cf62836f965e。

完整后端 Record / DeleteResult 返回不能从当前远程 GUI 取得，因此没有构造替代 receipt。三个比较报告始终为 receiptMatchesDisk=false；它们证明所观察的磁盘变化符合条件，加上独立 GUI 观察，不能声称完成了完整后端返回值逐字段比对。

所有报告的 secretCanariesChecked=0：本批没有输入真实秘密 canary，不能据此声称未知秘密文本已被逐字扫描。账户、随机记录 ID、revision / 名称哈希、私有路径、下载 URL、完整 JSON 和原始截图均只留私有材料。

## 实际结果

| 案例 | GUI 观察 | 独立后验 |
|---|---|---|
| 升级后原记录 | QA11 能列出并载入原 QA9 记录 | baseline 退出 0；count=1、912 字节、frames=1；ID 与 revision 同 QA9，记录文件 ACL 检查通过 |
| 仅重命名并保存 | 载入唯一原记录，只改名称后保存成功 | 前后 baseline 均退出 0；仍为同一记录、count=1；912→1849 字节、frames 1→2；名称哈希等于独立观察的预期新名称，config / engine / ACL 哈希不变 |
| 重命名旧字节证明 | 不再进行下一次 GUI 写入，先执行冷进程比较 | status=pass、diskTransitionVerified=true、countDelta=0、renameOldBytesPrefixVerified=true、currentFileReadOnlyPrefixRead=true，退出 0。只读打开当前记录，核对完整当前文件等于 after，再证明前 912 字节哈希等于 before revision；其他记录 / ACL / 同级名称哈希不变 |
| 另存为新配置 | 修改草稿名称并点另存为新配置；展开界面确认新 ID | 前后真实 baseline 加冷进程比较通过；count 1→2、countDelta=1、cloneNewID=true；新记录 914 字节、frames=1，预期名称哈希匹配，engine 同源；源记录及其他记录 / ACL / 同级名称哈希不变 |
| 取消删除副本 | 实际进入删除确认后取消 | auditor 退出 0；取消前后完整 baseline JSON 的 Ordinal 字符串比较为 true；未把取消当作删除成功 |
| 确认删除副本 | 重新取得基线，实际确认删除；界面显示记录已删除、当前草稿和浏览数据保留的提示 | after count=1；冷进程 status=pass、countDelta=-1，退出 0；仅所选副本消失，原记录与其他 ACL / 同级名称哈希保持不变 |

克隆报告明确保留 cloneNormalizedConfigEquivalenceVerified=false。新 profileId 会使配置原始字节哈希改变；本批只核对新 ID、单帧、名称、engine 与原有记录不变，没有把不相同的 raw config 哈希伪装成整份克隆内容等价。

删除界面的草稿 / 浏览数据提示只是本次可见 UI 结果。独立 auditor 的范围为配置库，未检查浏览器 Cookie、历史或其他浏览数据；不能据该提示或配置文件消失，推断这些数据已删除或已完整保留。

## 通过范围与仍待验收

本次 QA11 的升级后列举 / 载入、正常重命名追加、另存新配置、取消删除与确认删除，在这个普通用户和合成配置中通过实际 GUI 加独立只读磁盘验证。原始截图已人工审读；公开记录只保留上述必要计数、布尔结果与工具身份。较早 QA9 的保存 / 列举 / 载入证据仍作为 [历史观察](QA9_ORDINARY_GUI_OBSERVATIONS_2026-10-26.md) 保留，不由本次重写。

本批没有启动浏览器、连接代理、改变网络 / 系统设置或验证浏览数据删除。跨窗口 CAS 冲突、并发保存、崩溃 / 部分写入恢复、损坏记录、磁盘满 / 短写 / 同步失败、其他用户 / ACL 组合及完整 GUI 后端 receipt 仍需各自验证。正常追加与删除已经发生，不等于异常或耐久性矩阵全部通过。

本轮只更新脱敏文档；冻结 QA9 / QA10 / QA11 包和产品源码不变，未因记录新证据而重新构建、打包或发布。
