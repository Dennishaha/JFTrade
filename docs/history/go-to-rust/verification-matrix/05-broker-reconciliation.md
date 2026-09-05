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
- **重启推演**: 对账 Worker 扫描到该 `SUBMITTING` 订单。由于 `order.broker_order_id` 为空，代码执行 `product_production_ports_execution_reconciliation.rs:193-198`：
  ```rust
  if broker_id.is_none() && broker_order_id_ex.is_none() {
      let error = failed(502, "EXECUTION_STATE_UNKNOWN", "broker order identity is unavailable for reconciliation");
      self.persist_unknown_if_needed(order, &error, "reconcile_identity_unknown")?;
      return Err("broker order identity is unavailable for reconciliation".to_owned());
  }
  ```
- **破坏性后果**: 引擎直接将该本地订单置为 `UNKNOWN` 并放弃对账！策略或用户在前端看到下单失败或 UNKNOWN，触发重试再次下单，此时交易所已挂单的订单 `987` 与新订单相继成交，造成**仓位翻倍（Double Fill）与重大资金亏损**！

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
- [ ] **TC-D5-01（正常流）**: 模拟 100 股订单的 3 笔连续部分成交（20, 30, 50 股）与佣金，核验状态严格单调推进，均价准确。
- [ ] **TC-D5-02（乱序流）**: 注入乱序成交回执（先 50 股快照，后 20 股细则），核验累计成交恒等于 50 股，无重复超买。
- [ ] **TC-D5-03（崩溃注入 - 阻断门禁）**: 在下单发往交易所返回后立刻注入 `kill -9`，重启后核验对账 Worker 能基于 `client_order_id` 反向识别挂单，严禁将已成交/挂单置为 `UNKNOWN`。
- [ ] **TC-D5-04（断网恢复）**: 阻断 OpenD 端口，验证 Worker 状态由 `ready` 转为 `degraded` 并按指数退避，恢复网络后下周期自动转为 `ready`。
