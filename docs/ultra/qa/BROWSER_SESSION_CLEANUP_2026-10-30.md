# 独立浏览器会话数据清理审计（2026-10-30）

## 本批改动

独立浏览器启动时使用 `BrowserInstances/<64 位会话 ID>` 私有目录。会话关闭现在按以下顺序处理：

1. 确认浏览器进程树已退出；
2. 撤销并读回本会话防火墙限制；
3. 关闭代码与配置句柄；
4. 以 `os.Root` 相对路径预扫描会话树，拒绝符号链接/重解析点、非普通文件和超过 100000 个对象的异常树；
5. 只删除当前会话目录，并读回目录已不存在。

共享内核缓存、已保存的 `BrowserProfiles` 配置和其他会话目录不在删除路径内。预扫描或删除失败会保留 `closing_or_unknown` 状态，不显示为干净退出。

## 本地证据

- `go test ./internal/browserenginehost`：通过。
- `GOOS=windows GOARCH=amd64 go test -c ./internal/browserenginehost`：通过（Windows amd64 测试二进制成功生成）。
- Windows 纯文件回归覆盖：正常嵌套目录删除、同级哨兵保留、非生成会话 ID 拒绝。

## 尚未宣称的范围

本批没有把本机网络设置或云资源当作清理测试目标，也没有声称真实 VM 的 Chrome/Firefox 窗口、异常进程、重启和跨 Win10/Win11 版本已全部通过。必须在解除 Windows VM 远程桌面证书确认后，再取得真实窗口退出与独立磁盘读回证据。
