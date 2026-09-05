# JFTrade Go to Rust 架构迁移全景验证矩阵模块目录

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

本目录是 JFTrade 从 Go (`origin/go` 基线版本 `v0.27.0`，commit `452dea11`) 到 Rust (`main` 分支) 迁移全景验证矩阵的细分模块集合。各文档均包含具体的微观源码路径、代码符号、行号范围、破坏性边界推演与 Release Qualification 验证清单。

- **主导航索引总览**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)

---

## 模块分卷索引与直达链接

| 分卷编号 | 文档文件 | 覆盖领域 | 关键发现与阻断等级 |
| :---: | :--- | :--- | :--- |
| **00** | [00-executive-summary.md](./00-executive-summary.md) | **执行总览与验证分类学** | 总体迁移结论、P0/P1/P2 全景风险统计表、架构范式演进 |
| **01** | [01-pine-runtime.md](./01-pine-runtime.md) | **领域 1：Pine Runtime 状态恢复与意图幂等** | 实例重启恢复、Node Worker JS 堆状态重置、`PINE_SESSION_CHECKPOINT`、每日配额 CAS 预留（P2-01） |
| **02** | [02-provider-switching.md](./02-provider-switching.md) | **领域 2：Provider 切换与读写屏障** | `ActiveProviderState` 双锁、`TickCache` 代际递增与全量抹除、OpenD 交易角色解耦、设置回滚 |
| **03** | [03-routes-and-writerlease.md](./03-routes-and-writerlease.md) | **领域 3：278 条路由全景矩阵与 WriterLease** | 18 个 Capability 组 278 条路由完整大表、`ApiFailure` 映射规范、9 库排他文件锁 |
| **04** | [04-backtest-time-dst.md](./04-backtest-time-dst.md) | **领域 4：回测时间 / DST / Session 语义** | 纯 UTC 整除取模导致美股开盘 60m 聚合 **100% 崩溃 (**P0-01**)**、2024 夏冬令时切片对比推演、盘前污染（P1-01） |
| **05** | [05-broker-reconciliation.md](./05-broker-reconciliation.md) | **领域 5：Broker 回执、对账与持仓投影** | 落库前崩溃引发**在途幽灵订单与双重下单 (**P0-02**)**、乱序成交快照覆盖数学证明、持仓穿透降级 |
| **06** | [06-sidecars-resilience.md](./06-sidecars-resilience.md) | **领域 6：Futu/OpenD 与 Python/Node 外部进程韧性** | 外部子进程崩溃**完全无自动热拉起 (**P0-03**)**、OpenD 指数退避重连、冷启动 OpenD 未初始化（P2-02） |
| **07** | [07-adk-leases-approvals.md](./07-adk-leases-approvals.md) | **领域 7：ADK 审批与跨 3 库并发租约** | 会话删除未级联清理孤儿事件/工件（P1-04）、慢推理租约失窃与工具调用幂等（P1-03）、两阶段审批回滚 |
| **08** | [08-sqlite-schemas-migrations.md](./08-sqlite-schemas-migrations.md) | **领域 8：九个 SQLite 数据库演进与兼容** | 9 库版本跳表与备份、历史版本迁移断崖（P1-06）、不可逆降级阻断、`events` 表历史 P0 慢查询（P1-05） |
| **09** | [09-pinets-wire-events.md](./09-pinets-wire-events.md) | **领域 9：PineTS Worker Wire 契约与事件序** | 重复/乱序 Tick 校验前置导致会话泄漏与策略死锁（P1-07）、EMA(200) 预热偏离 13.4%（P1-08）、4MB 报文超限（P1-09） |
| **10** | [10-zero-go-tauri-frontend.md](./10-zero-go-tauri-frontend.md) | **领域 10：零 Go 残留、Tauri 发布与前端一致性** | 2,624 文件零 Go 验证、4 平台 Sidecar 自包含、**前端缺失券商密码解锁导致实盘报单 100% 失败 (**P0-04**)**、13 项路由盲区 |
| **11** | [11-release-qualification-action-plan.md](./11-release-qualification-action-plan.md) | **准入验证与上线工程排期建议** | 跨领域缺陷排期总表、第一阶段 P0 攻坚、第二阶段 P1 质量门禁与性能优化 |
