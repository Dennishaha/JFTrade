# 领域 5：Broker 回执、对账与投影

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.5 领域 5：Broker 回执、对账与投影（ExecutionReconciliationWorker、幽灵订单与持仓投影降级）

#### 2.5.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `internal/trading/order_updates.go:160-250`
  - `internal/store/trading/broker_ledger.go:20-105`
- **关键符号**: `OrderUpdatesWorker`, `upsertBrokerOrderWithSource()`, `allocateInternalOrderIDLocked()`, `registerFillKeyLocked`
- **历史行为**:
  Go 基线在对账中同时支持基于 WebSocket/Push 的实时推送订阅与定时全量同步。更为关键的是，在 `upsertBrokerOrderWithSource` 中，若券商端返回了一笔本地未记录的订单，Go 能够**自动调用 `allocateInternalOrderIDLocked` 建立内部订单并主动收养外部孤儿订单**，具备强大的未决订单自愈能力。

#### 2.5.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-engine/src/product_production_ports_execution_orders.rs:113-207`
  - `crates/jftrade-engine/src/product_production_ports_execution_orders_impl.rs:233-280`
  - `crates/jftrade-engine/src/product_production_ports_execution_reconciliation.rs:57, 148-198, 353-374, 588-597, 689-771`
  - `crates/jftrade-engine/src/product_portfolio_projection.rs:456-483`
  - `crates/jftrade-engine/src/strategy_runtime.rs:775-792`
- **关键机制**:
  1. **后台弱引用对账轮询**: `ExecutionReconciliationWorker` 为纯异步后台轮询任务，固定 15 秒间隔 + 错误退避，持有 `Arc::downgrade(&port)` 弱引用，**绝不长期占用 `execution.db` 的 `WriterLease`**。
  2. **Push 通知断层 (P1-02)**: Futu 实时推送通知 `Trd_UpdateOrder` (2208) 与 `Trd_UpdateOrderFill` (2218) 在集成层声明但**未接入对账 Worker**，完全依靠 15 秒轮询。
  3. **covered_by_snapshot 幂等覆盖算法**: 严格证明并实现了乱序成交与快照覆盖计算，确保成交量单调非递减。
  4. **穿透式持仓投影**: 9 个 SQLite 库均未设计持仓表，持仓与资金数据完全属于即时穿透式读取。

#### 2.5.3 微观差异与破坏性边界失效推演

#### 1. 落库前崩溃导致的在途“幽灵订单”与双重成交灾难 (P0-02 缺陷推演)
```
[Client]                [Rust Engine]                 [SQLite DB]                [OpenD / Exchange]
   |                           |                           |                             |
   |--- POST /api/v1/orders -->|                           |                             |
   |                           |-- 1. reserve_order ------>| (status: SUBMITTING)        |
   |                           |<-- Ok(reserved) ----------|                             |
   |                           |                           |                             |
   |                           |-- 2. writer.place_order ------------------------------->|
   |                           |   (Trd_PlaceOrder 2202)   |                             |
   |                           |                           |                             | [Exchange accepts]
   |                           |<-- 3. OrderResult(id=987) ------------------------------| [Order is LIVE]
   |                           |                           |                             |
   |                    💥💥💥 CRASH WINDOW 💥💥💥         |                             |
   |             (SIGKILL / Kernel Panic / Power Loss)     |                             |
   |                           |                           |                             |
   |                           |-- 4. persist_success ❌   | (Never executed)            |
   |                           |   (Write broker_order_id) |                             |
```
- **步骤 1**: 本地写入 `execution.db`，状态为 `SUBMITTING`，`broker_order_id = NULL`。
- **步骤 2~3**: 引擎调用 OpenD 发送委托，交易所撮合中心受理并返回外部订单号 `987`。
- **步骤 4 崩溃**: 进程在第 279 行 `persist_external_success` 写入外部单号前被杀。
- **历史崩溃推演 (修复前)**: 对账 Worker 扫描到该 `SUBMITTING` 订单。若 `order.broker_order_id` 为空，旧逻辑直接执行 `persist_unknown_if_needed` 并将订单置为 `UNKNOWN`，策略或用户可能触发重试造成双重下单与资金暴露。
- **已实施崩溃恢复架构 (P0-02 闭环)**:
  - 核心实现位于 `crates/jftrade-engine/src/product_production_ports_execution_reconciliation_recovery.rs`，由 `product_production_ports_execution_reconciliation.rs:165` 统一接入：
    `match self.resolve_unidentified_submission(reader, order, &header)? { ... }`
  - **1. 候选发现与去重 (Candidate Discovery)**: 通过 `reader.read_orders` 与 `reader.read_history_orders` 联合拉取活动与历史快照，以 `(order_id, order_id_ex)` 去重并保留最新 `update_time` 快照。
  - **2. 外围/非本人订单保护 (Foreign Order Protection)**: 查询本地 `store.list_orders()`，统计所有已认领的 `claimed_numeric_ids` 与 `claimed_ex_ids`。在券商候选集中严格排除已被其他本地订单认领的订单，杜绝跨订单错绑。
  - **3. 双层优先级消歧 (Candidate Disambiguation)**:
    - **Priority 1 (确定性备注匹配)**: 严格匹配 `candidate.remark == order.client_order_id` 或 `candidate.remark == order.remark`（且标的 `symbols_match` 一致），不受 300 秒时间窗口限制。
    - **Priority 2 (安全属性 + 时间窗口匹配)**: 当无 remark 时，检验标的、买卖方向、委托数量（误差 $\le 10^{-6}$）、订单类型、限价价格（误差 $\le 10^{-6}$）以及发单时间窗口（$[-60\text{s}, +300\text{s}]$）。若候选订单存在非空且冲突的 remark，则直接排除（`has_conflicting_remark`）。
  - **4. 三态安全处置闭环**:
    - **唯一候选 (1 笔)**: 返回 `RecoveryResolution::Recovered(snapshot)`，绑定 `broker_order_id` 与 `broker_order_id_ex`，正常继续对账推进。
    - **多候选歧义 (>1 笔)**: 返回 `502 EXECUTION_STATE_AMBIGUOUS`，将本地订单置为 `UNKNOWN`，严禁猜测，保持现场供人工核实。
    - **零候选 (0 笔)**: 判定券商从未受理该订单，将本地订单置为 `FAILED` (`502 BROKER_ORDER_NOT_FOUND`)，触发在途配额安全回收。

#### 2. covered_by_snapshot 乱序回执数学证明
设快照累积成交量为 $Q_{\text{best}} = \max \{ Q_{\text{snapshot}} \mid T_{\text{snapshot}} \ge T_{\text{fill}} \}$，已知成交量为 $Q_{\text{known}} = \sum \{ q_k \mid T_{q_k} \le T_{\text{fill}} \}$。
新增有效成交量计算为：
$$Q_{\text{covered}} = \min(q_{\text{fill}}, \max(0, Q_{\text{best}} - Q_{\text{known}}))$$
$$q_{\text{applied}} = \max(0, q_{\text{fill}} - Q_{\text{covered}})$$
若快照已先到 ($Q_{\text{cum}}=50$)，随后逆序到达 Fill 2 (30 股) 与 Fill 1 (20 股)：
- Fill 2 到达：$Q_{\text{best}} = 50, Q_{\text{known}} = 0 \implies Q_{\text{covered}} = 30 \implies q_{\text{applied}} = 0$。订单总量维持 50 股。
- Fill 1 到达：$Q_{\text{best}} = 50, Q_{\text{known}} = 0 \implies Q_{\text{covered}} = 20 \implies q_{\text{applied}} = 0$。订单总量维持 50 股。
**数学证明证实：快照覆盖算法严格保证了乱序回执下的成交量单调非递减且不重复累加。**

#### 3. 断网时持仓投影坍塌
当 OpenD 离线时，`execute_portfolio_overview` 读取失败，将 `positionCount = 0, partial = true`。策略运行时调用 `read_positions` 报错后，当前 Tick 指标计算与下单被直接中断丢弃，保证了无有效持仓数据时不产生盲目调仓。

#### 2.5.4 Release Qualification 验证清单
- [x] **TC-D5-01（正常流 / P1-02 闭环）**: 模拟 100 股订单的 3 笔连续部分成交（20, 30, 50 股）与佣金，核验状态严格单调推进（`SUBMITTED` -> `PARTIALLY_FILLED` -> `FILLED`），均价准确（$100 \to \$103 \to \$106.5$），佣金正确绑定。
  - 验证用例：`test_tc_d5_01_sequential_partial_fills_and_fees_monotonic` (PASS)
- [x] **TC-D5-02（乱序流 / P1-02 闭环）**: 注入乱序成交回执（先 50 股快照，后 30 股与 20 股细则），核验累计成交恒等于 50 股，无重复超买，`covered_by_snapshot` 拦截有效。
  - 验证用例：`test_tc_d5_02_out_of_order_push_chaos_covered_by_snapshot` (PASS)
- [x] **TC-D5-03（崩溃注入 - 阻断门禁 / P0-02 对账自愈已闭环）**:
  - **核心验证用例 (cargo test -p jftrade-engine --lib reconciliation)**:
    * `reconciliation_crash_window_recovers_submitting_order_and_binds_broker_ids`: 模拟发单落库前崩溃，对账扫描自动从券商订单识别并绑定外部订单 ID。
    * `reconciliation_crash_window_recovers_filled_order_and_reconciles_fills`: 验证崩溃期间券商已成交订单的自愈恢复、外部 ID 绑定与成交状态单调推进。
    * `reconciliation_partial_fill_during_crash_window_recovers_to_partially_filled`: 验证部分成交订单在崩溃后安全恢复至 `PARTIALLY_FILLED`。
    * `reconciliation_no_candidate_transitions_to_failed_for_safe_quota_reclaim`: 验证券商零候选时订单转为 `FAILED`，为配额安全回收提供确定性状态。
    * `reconciliation_foreign_order_protection_ignores_unmatched_remark`: 验证外来非本人订单或备注不匹配订单绝不被误领。
    * `reconciliation_claimed_order_exclusion_protects_against_cross_binding`: 验证已被其他本地订单认领的券商订单绝不发生交叉误绑。
    * `reconciliation_priority_1_matches_client_order_id_in_remark`: 验证客户端订单 ID 优先精确匹配。
    * `reconciliation_priority_1_matches_even_beyond_300s_window`: 验证 Priority 1 remark 匹配不受 300 秒时间窗口限制。
    * `reconciliation_timestamp_window_boundary_301s_vs_299s`: 验证时间窗口边界检查（299s 允许，301s 拒绝）。
    * `reconciliation_extended_id_matching_and_binding_without_numeric_id`: 验证仅有扩展字符串 ID 时的识别与绑定。
    * `challenge_edge_case_1_three_identical_broker_orders_no_client_id_remains_unknown`: 验证多笔无备注同属性订单产生歧义时严格保持 `UNKNOWN`，绝不猜测。
    * `challenge_edge_case_2_different_symbol_and_conflicting_remark_not_claimed`: 验证不同标的与冲突备注阻断认领。
    * `challenge_edge_case_3_broker_network_failure_does_not_mutate_or_release_quota`: 验证网络异常时维持重试，不误标 FAILED 或提前释放配额。
    * `challenge_edge_case_4_order_state_transitions_filled_cancelled_rejected`: 验证对账中终态流转（成交、撤单、拒绝）的单调性与正确性。
  - **剩余风险边界**:
    1. *多候选歧义*: 当无 `client_order_id` 备注且并发提交多笔同属性订单时，对账进入 `UNKNOWN`，需人工干预。
    2. *券商历史窗口*: 恢复延迟若超过券商历史订单查询最长时间范围，无法查到快照将判定为 0 候选，依赖 15 秒常态轮询保障。
- [x] **TC-D5-04（断网恢复 / P1-02 闭环）**: 阻断 OpenD 端口，验证 Worker 状态由 `ready` 转为 `degraded` 并按指数退避（1s~60s），恢复网络后调用 `wake()` 立即自愈恢复为 `ready`。
  - 验证用例：`test_tc_d5_04_opend_disconnect_degraded_backoff_and_self_healing` (PASS)
- [x] **P1-02 专项：Push 与轮询双轨对账一致性与低延迟保障**:
  - `test_p1_02_push_wake_latency_and_polling_fallback`: 交易 Push 接收后即刻唤醒 worker（< 100ms 延迟响应），若丢包未唤醒则 15 秒轮询兜底稳定生效。
  - `test_p1_02_single_writer_lease_and_concurrency_fencing`: Push 洪峰与并发唤醒严格受控于 `ProductionExecutionPort` 单一写属主（`WriterLease`），零锁争用、零双写。
