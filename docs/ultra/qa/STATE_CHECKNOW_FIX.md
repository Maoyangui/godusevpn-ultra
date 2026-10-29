# CheckNow 丢失唤醒修复

日期：2026-10-19。仅修改 Ultra 副本的 `internal/state`，没有改原目录或 UI。

全量回归原记录 `audit-fix-regression-20261019.txt` 中，`TestCheckNowRunsHealthImmediately` 在 3 秒后超时。确认是继承代码的时序问题：`run` 先发布 Connected，随后 `watch` 才清空旧 kick；两步之间已针对新连接提交的有效 CheckNow 也会被清掉。

新增 fake 在同步 `OnChange(Connected)` 中直接调用 CheckNow，强制这个顺序，不依赖调度概率。旧实现确定失败（等待 1 秒），修复后立即通过；健康定时器设为 1 小时，测试结果只能来自显式唤醒。

修复把旧队列清理、重试计数复位与新 Connected 状态发布放进同一个 `mu` 临界区。CheckNow 同样在该锁内核对 Wanted、关闭状态、有效循环与 Connected/Degraded 状态并入队，因此不是“看快照后再发送”的 TOCTOU。OnChange 仍在解锁后调用；`watch` 不再有发布后的清理。断开/启动期请求直接丢弃，容量 1 的合并语义保留；Degraded 恢复为 Connected 不清空当前会话的有效请求。

另一个确定性 fake 在旧 Health 中等待取消、同时排入第二个旧 kick，再断开并新连接；验证旧排队请求被清理，新 Connected 通知的请求正常执行，且没有多做检查。Preparing/Starting 的请求也在入队前拒绝。这里隔离的是已排队请求；无会话标识的未来 CheckNow 调用仍按调用时的当前有效连接处理，不新增来源推断。

验证：

- CheckNow 与旧会话回归重复 20 轮，通过（13.903 秒）。
- `go test ./internal/state -count=1` 通过。
- `go test ./internal/daemon ./internal/netmode ./internal/netmode/wfp -count=1` 通过。
- `go vet ./internal/state`、针对 state 的 `git diff --check` 通过。

全部使用 fake；没有真实网络、服务、注册表或 WFP 修改，没有改变隐私失败时保留隧道/闸的算法。当前 CGO_ENABLED=0，因此未运行 race detector；确定性时序回归与 race detector 的证据层级不同。全量套件的最终结果仍以主协调者修复后的完整重跑为准。
