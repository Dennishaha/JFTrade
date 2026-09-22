# Trading 执行域第四批（第 118 批·分片一）：订单更新 worker 的节流与缓存面

本文件记录 `internal/trading/order_updates_test.go` 全部 15 条 `partial` 的逐条收口。该文件是
Go 服务内自带 `OrderUpdatesWorker` 的测试集合（节流/强制同步、内存订单缓存 TTL、订阅生命周期、
诊断状态、费用同步与辅助边界）。Rust 没有同形 worker：刷新职责由
`crates/jftrade-engine` 的 `ExecutionReconciliationWorker`（轮询节奏 + push wake）与 SQLite 持久层承担，
因此本批以“逐条对照 → 能断言则 function_exact、机制不存在则 boundary 并写清保留差异与升级路径”推进。

分片计划：分片一 `:158`/`:180`/`:228`/`:263`（节流与缓存面）；分片二 `:280`/`:322`/`:343`/`:369`/`:397`
（历史同步 scope、stop 幂等、账户集合重订阅、刷新订阅、并发订阅一次）；分片三 `:414`/`:427`/`:442`/`:469`
（invalidations 上限、非活跃 source 诊断、回退路径、失败标记）；分片四 `:508`/`:531`（费用同步过滤、辅助边界）。

## 第一百一十八批（分片一）：节流与缓存面（4 条）

### 范围与分片

- P1 `internal/trading/order_updates_test.go:158`：`TestOrderUpdatesWorkerThrottleForceAndSubscribeOnce`
  （窗口内重复 sync 被节流、force 强制再拉、订阅只发生一次）。
- P1 `internal/trading/order_updates_test.go:180`：`TestOrderUpdatesWorkerCacheTTLTerminalRemovalAndDefensiveCopy`
  （TTL 命中缓存、防御性拷贝、终态移除、TTL 过期重拉）。
- P2 `internal/trading/order_updates_test.go:228`：`TestOrderUpdatesWorkerCurrentHistoryCacheAndPushMetadata`
  （当前/历史缓存合并后的逐条 metadata 与 fill 投影）。
- P1 `internal/trading/order_updates_test.go:263`：`TestOrderUpdatesWorkerForcedActiveSyncBypassesCache`
  （forced active sync 绕过缓存）。

分类：1 条 `[x]`/`function_exact`（`:263`），3 条改判 `[~]`/`boundary`（`:158`、`:180`、`:228`）；
全仓 `[x]` 1344 → **1345**、`partial` 2528 → **2524**、`boundary` 575 → **578**、`module_only` 4、
`missing` 0；Rust 测试 3084 → **3085**。

### 关键事实（本批 recon 与实测）

- **Rust 无 order-updates worker 面**：Go 的 `OrderUpdatesWorker` 自带内存订单缓存（TTL、终态移除、
  防御性拷贝）、按账户集合的券商订阅与 metadata 投影（`BROKER_SYNC_*`/`BROKER_HISTORY_*`/
  `BROKER_CACHE_*`/`BROKER_PUSH_*`）。Rust 的等价职责拆成三处：`ExecutionReconciliationWorker`
  （`product_production_ports_execution_orders.rs`，起始立即扫描 + 15s 轮询节奏 + `wake()` 强制扫描 +
  失败 backoff）、`ExecutionOrderStore`（SQLite 唯一真相，无内存缓存）、Futu 会话
  （`subscribe_trade_accounts`，交易推送订阅由会话一次性持有）。
- **节流与强制路径可断言**：`ExecutionReconciliationWorker` 的轮询节奏等价于 Go 的
  `Sync(force=false)` 节流，`wake()`（由 `SharedTradeReadRuntime::reconciliation_wake()` 与 OpenD
  事件监听器驱动）等价于 `Sync(force=true)`。现有
  `test_p1_02_push_wake_latency_and_polling_fallback` 只断言 wake 的亚秒延迟，
  `test_p1_02_single_writer_lease_and_concurrency_fencing` 只断言并发 wake 不重复写，
  都没有“窗口内不追加扫描”的节流断言，本轮补齐。
- **缓存面在 Rust 不存在**：全仓 grep `BROKER_CACHE_`/`BROKER_HISTORY_`/`BROKER_PUSH_` 0 命中；
  仅 `execution_reconciliation_discovery.rs:170` 写 `BROKER_SYNC_DISCOVERED`。终态订单不会再次进入
  对账候选（`execution_reconciliation_discovery.rs::reconciliation_candidates` 只取
  `list_reconciliation_candidates` 与缺费用的终态费用候选），由
  `product_production_ports_execution_reconciliation_read_boundaries_tests.rs` 的
  “terminal page retry” 断言（第二次扫描 0 变更）覆盖。

### 新增测试（带 `// Parity:` 锚点）

- `crates/jftrade-engine/src/product_production_ports_execution_reconciliation_push_worker_tests.rs::reconciliation_polling_throttles_scans_until_a_push_wake_forces_one`
  （锚 `:263`）：起始扫描完成后 300ms 窗口内 `worker.status().scans` 保持不变（轮询节流）；
  一次 `worker.wake()` 后恰好追加一次扫描并在 5s 内完成（强制路径）。测试自带
  `scans_reach` 轮询助手（5s 上限）以消除调度抖动。

### 探针记录（破 → 红 → 按字节回滚）

探针针对 `crates/jftrade-engine/src/product_production_ports_execution_orders.rs`，回滚后 sha
`eae79748cbde15eeba184fc09026010a2b65e3971cb35b5f0d5b5d5c75d0f01c` 与探测前一致：

- ① 轮询节奏 `Duration::from_secs(15)` → `from_millis(50)`：300ms 窗口内扫描数 1 → 6，
  节流断言转红。
- ② 删除 worker select 的 `_ = task_wake.notified() => {}` 分支：wake 不再触发扫描，
  强制断言（期望 `first+1`，实际停在 `first`）转红。

### 保留差异

- `:158`（boundary）：节流/强制由新增测试覆盖，但“订阅只发生一次”在 Rust 无同形对象——worker
  不订阅任何券商流，交易推送订阅由 Futu 会话一次性持有（`trade_session.rs::subscribe_trade_accounts`），
  重复 sync 结构上不可能重复订阅。将来若引入 worker 自持订阅，需补“重复 sync 不重复订阅”的计数断言。
- `:180`（boundary）：TTL 缓存不存在。复现 Go 语义的对象（内存缓存）已由 SQLite 唯一真相取代：
  每次扫描都重新读 broker（节流）且每次读都是独立的行结构（所有权拷贝）；终态订单不再作为
  常规对账候选。若重新引入内存缓存，必须补 TTL 命中/防御性拷贝/终态清理三条断言。
- `:228`（boundary）：metadata 事件类型面不存在（Go 的 cache/history/push 三类事件在 Rust 没有
  对应写入点）。推送触发链路本身由 push worker 测试覆盖（wake→扫描→持久化），但事件类型与
  `sourceDetail` 不同。

### 后续待办（本文件剩余 11 条）

- 分片二：`:280`（历史同步按订单 scope）、`:322`（stop 幂等与可重订阅）、`:343`（账户集合变化重订阅）、
  `:369`（sync 刷新既有订阅）、`:397`（并发 sync 只订阅一次）。
- 分片三：`:414`（快照 invalidations 上限）、`:427`（非活跃 source 保留诊断状态）、
  `:442`（订阅+历史回退路径）、`:469`（当前/历史失败标记）。
- 分片四：`:508`（费用同步过滤与失败上报）、`:531`（nil/替换辅助边界）。
- 该文件收口后：第 119 批 `internal/trading/execution_combo_lifecycle_test.go`（12 条），
  随后 `pkg/broker/broker_test.go`(8)、`catalog_test.go`(6) 等 trading_broker 剩余文件；
  再按 api_transport / strategy_pine / assistant_workflow / backtest_calendar / storage_sqlite /
  marketdata_quotes / futu_opend / settings_watchlist 顺序逐域收口。
- 既有独立项：docs/history 尖括号占位符导致 `build:docs:generated` 失败；桌面周期性更新检查
  （`startDesktopUpdateChecks` 24h 节奏）；launcher SIGTERM 竞态抖动。

验证：
- 分片一定向 nextest（1 条新测试）EXIT=0：`reconciliation_polling_throttles_scans_until_a_push_wake_forces_one`。
- 分片一两次探针全部转红并按字节回滚，回滚后 `product_production_ports_execution_orders.rs` sha
  `eae79748cbde15eeba184fc09026010a2b65e3971cb35b5f0d5b5d5c75d0f01c` 与探测前一致。
- 分片一门禁：`cargo fmt --all --check` EXIT=0；`cargo clippy -p jftrade-engine --all-targets --locked` EXIT=0；
  `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`
  **1777 passed / 0 failed / 0 skipped**；`pnpm run test:rust`（workspace）**3182 passed / 2 skipped**，EXIT=0；
  `pnpm run check:compatibility` EXIT=0；`node scripts/check-zero-go.mjs` EXIT=0（2932 tracked files）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- 分片一 `pnpm run check:quick`：首轮因 launcher 家族抖动
  `api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 失败（同轮全量 nextest 该条 PASS），
  隔离复跑 `-E 'test(api_launcher)'` 2/2 PASS（含此前登记的
  `api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal`），
  清理 `.rcgu.o` 超限后复跑 `check:quick` EXIT=0。该条与既有 SIGTERM 抖动同源，建议一并加去抖断言。
- 分片一 `pnpm run check:rust`：`check:rust:policy`（cargo deny advisories）失败（RUSTSEC-2026-0285 与
  陈旧 advisory 告警，既有基线），**不记为通过**；等价的静态与测试面由上面的 clippy、全量 engine nextest、
  workspace `test:rust`、`check:quick`、`check:compatibility` 覆盖。
- 分片一审计：`python3 scripts/compatibility/audit_test_parity.py` 4451 Go / **3085** Rust、
  `[x]` 1344 → **1345**、`partial` 2528 → **2524**、`boundary` 575 → **578**、`missing` 0、0 破坏引用；
  `python3.12 scripts/compatibility/parity_anchor_reconcile.py` **1348** 唯一引用
  （已记账 1291 → **1292**、unrecorded 0、unknown 55、stale 1）。

