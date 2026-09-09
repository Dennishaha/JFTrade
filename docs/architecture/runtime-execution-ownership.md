# 运行中执行、恢复与停止

本文说明策略和工作流的生产写入与恢复边界。入口分别为 `strategy_runtime_task.rs`、`product_workflow_scheduler.rs`，组装由 `jftrade-engine` 持有。

## 策略与成交

- `REAL` 和 `SIMULATE` 都是券商账户的交易环境：使用绑定的 broker/account/environment 查询资金和持仓，经同一个执行订单 port 提交。SIMULATE 不是离线纸交易开关，也不因 OpenD 不可用而改成本地成交。
- 券商返回提交成功并不意味着成交。成交与订单投影仍由 execution store/reconciliation owner 写入；策略不为已提交订单额外记本地成交。虚拟账户模型仅保留在隔离测试中，不由生产运行器初始化或恢复。
- 日历模块统一确定 bar 完成时刻。日/周/月线使用交易所当地日期及日历 session，覆盖半日市和休市；尚无权威日历的市场保守等待周期边界或下一根 bar。内部 `closed` 标志不增加公开 HTTP 字段。
- 首次启动可预热历史数据；恢复启动只预热到已持久化的 checkpoint，再按时间顺序逐根补齐之后的已收盘 bar。append 失败立即终止当前补 bar 批次，复开 session 后重试，不能把待处理数据当预热跳过。带时间的 intent 按时间匹配，bar index 不替代时间身份。
- 每个实例的生命周期 mutation 串行；停止取消异步行情/Pine 等待。同步阻塞超过期限时保留 task owner、返回停止未完成，并阻止重启重叠。行情 I/O 期间仅持有 store 的弱引用；已在执行的券商命令仍保留真实写锁，不能强制释放后允许第二写入者。

## 工作流

- Canvas 与旧的单 prompt workflow 共用节点执行器。模型返回 RUNNING、PENDING_APPROVAL 或 PENDING_INPUT 时挂起图；只有前置节点终态成功，后续节点才可执行。未知状态不映射为成功。
- 调用图、输入、调用 UUID 和节点请求身份保存在现有 trigger log 的私有 checkpoint 中；每个节点跨外部调用前后使用日志 revision CAS 保存进度。私有 checkpoint 不返回到 HTTP 日志响应。
- 重启、审批或输入继续后，scheduler 重新读取非终态日志；已成功节点跳过，未完成节点复用原 clientRequestId，与模型运行的持久化执行租约及工具幂等共同防止重复副作用。
- 调度器按 Go 分支所用五字段 cron 的步进/日星期语义计算时间，以真实时间线处理 DST 重复及不存在时刻。下一次触发时间与调用入队在 SQLite 同一事务完成；并发 trigger 更新通过 revision CAS 决定唯一胜者。
- Scheduler 在全部业务 ports 连接完成后启动；后台 invocation 数量有界、受 owner 管理。停止先关闭派发，再取消模型运行并 join invocation；未完成时明确报错，不把轮询任务被取消等同于所有任务结束。

## 验证

关键用例直接驱动生产 StrategyRuntimeManager、ProductionAdkPort 和 SQLite：收盘/补 bar、append 故障与重开、待审批节点恢复、提交但未成交、停止超时写锁、事务入队失败回滚以及 Cron/DST 兼容。测试中的外部模型、Pine 和行情使用明确替身，不访问真实账户。
