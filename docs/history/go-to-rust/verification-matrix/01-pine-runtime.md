# 领域 1：Pine Runtime（Pine 运行时恢复与意图幂等）

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

## 2.1.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `internal/strategy/service.go:102, 385`
  - `internal/strategy/catalog/lifecycle.go:92-123`
  - `internal/app/apiserver/servercore/server.go:408`
- **关键符号**: `ReconcileOnStartup()`, `s.data.Strategies`, `strategies.json`
- **历史行为**:
  在 Go 基线版本（commit `452dea11`）中，系统在启动时调用 `ReconcileOnStartup()` 遍历全部策略。若发现策略状态处于 `RUNNING` 或 `PAUSED`，**无条件强制重置为 `STOPPED`**，记录一条日志并覆写 `strategies.json`。Go 架构中不存在策略运行时的进程重启热恢复机制，用户每次重启引擎后必须手动逐一重新启动各个策略。

---

## 2.1.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-engine/src/product_production_ports.rs:590-594`
  - `crates/jftrade-engine/src/strategy_runtime_port.rs:48-90`
  - `crates/jftrade-store-sqlite/src/strategy_runtime.rs:343-390`
  - `crates/jftrade-engine/src/strategy_runtime.rs:347-353, 640-724`
  - `crates/jftrade-engine/src/strategy_runtime_execution.rs:240-284, 722-738`
  - `crates/jftrade-store-sqlite/src/strategy_runtime_observation.rs:43-92`
  - `workers/pineworker/src/pinetsExecutor.ts:70-106`
- **关键机制**:
  1. **RUNNING 自动恢复**: `restore_running_instances()` 扫描 SQLite `strategy_catalog_operations` 中 `runtime_active == true || status == "RUNNING"` 的实例。若外部行情、报价端口及 PineTS Worker 均健康，自动重新建立物理订阅，调用 `spawn_task` 启动后台 Tokio 协程并写入 `RECOVERED` 审计事件。
  2. **PAUSED 冷态保留**: `PAUSED` 实例在库中的 `runtime_active` 为 `false`（`strategy_runtime.rs:374`），启动时不唤醒后台任务，避免不必要的系统开销。
  3. **Node Worker JS 堆状态补偿**: Node Worker 内部状态仅保存在 V8 堆内存中的 `NativeLiveSession`（`pinetsExecutor.ts:70`）。引擎重启后，Rust 端强制重置 `revision = 0`，首周期触发 `session_operation = "open"` 并注入 200 根全量预热 K 线；Node Worker 在处理 open 时显式执行 `result.orderIntents = []`（`pinetsExecutor.ts:104`），防止将历史预热信号伪造为实盘订单。
  4. **PINE_SESSION_CHECKPOINT 审计流恢复**: 每个计算周期结束，调用 `persist_pine_runtime_checkpoint`（`strategy_runtime.rs:696-715`）向 SQLite 写入检查点。通过 `SHA256(binding + definitionRevision)` 计算 Scope，确保用户修改策略配置后旧检查点自动失效；重启后读取 `lastClosedOpenTime`，凡早于该时间的 Bar 直接跳过（`strategy_runtime.rs:410`），杜绝跨重启重复计算。
  5. **ClientOrderId 确定性幂等**: 生成格式为 `strategy-{instance_id}-{symbol}-{intent_id}-{bar_index}-{candle_time}`（`strategy_runtime_execution.rs:722-738`），实现订单在券商端的强幂等拦截。
  6. **每日配额原子预留 (CAS)**: 当风控为 enforce 模式且配置了 `daily_max_orders` 时，在向券商发单前开启 SQLite `Immediate` 事务执行 `reserve_daily_order`。只有配额未满并成功写入 `ORDER_RESERVED` 审计事件后才发单；若券商返回失败，立即写入 `ORDER_RESERVATION_RELEASED` 释放配额；成功则写入 `ORDER_SUBMITTED`。

---

## 2.1.3 微观差异与破坏性边界失效推演
1. **发单成功但检查点未刷盘崩溃窗口 (Crash Gap)**:
   - **时序推演**: `dispatch_place_order` 发单成功，但在执行 `persist_pine_runtime_checkpoint`（`strategy_runtime.rs:545`）之前操作系统硬杀。
   - **防御机制**: 重启后该 Bar 重新触发 Intent，但由于 `clientOrderId` 完全一致，券商底层或 `ExecutionOrderStore` 的唯一索引 `(broker_id, trading_environment, account_id, client_order_id)` 触发拦截拒绝，防止资金双重暴露。
2. **预留成功后崩溃导致的配额泄漏与子串碰撞修复 (P2-01 已闭环)**:
   - **历史缺陷推演 (修复前)**:
     1. `reserve_daily_order` 写入 `ORDER_RESERVED` 后若发生崩溃，旧逻辑未与对账恢复形成闭环，孤儿预留占用配额槽位。
     2. `crates/jftrade-store-sqlite/src/strategy_runtime_observation.rs:74` 原使用无锚定子串查询 `(r.kind = 'ORDER_SUBMITTED' AND a.detail != '' AND (r.detail = a.detail OR r.detail LIKE '%' || a.detail || '%'))`。当 Order 1 预留键为 `:1:1`，Order 2 预留键为 `:1:10` 并提交时，Order 2 的 detail 包含 `... reservation: inst:US.AAPL:1:10)`，导致 `'...:1:10)' LIKE '%:1:1%'` 成立，从而错误将 Order 1 的在途预留从配额统计中提前剔除，在限额边界下允许非法第 3 笔订单侵入。
     3. 存在配额统计在途预留与已提交订单计数的 2x 重叠计算风险。
   - **已实施修复方案**:
     1. **消除 2x 双重计数**: 在 `strategy_runtime_observation.rs` 中重构配额生命周期统计，分离有效预留（`ORDER_RESERVED` 且未 `RELEASED` 且未 `SUBMITTED`）与实际已提交委托（`ORDER_SUBMITTED`），确保各阶段计数严格单一。
     2. **严格锚定 SQL 子串匹配**: 将子串模式严格锚定为 `(r.detail = a.detail OR r.detail LIKE '%reservation: ' || a.detail || ')%')`，杜绝 `:1:1` 与 `:1:10` 等前缀/子串碰撞。
     3. **对账联动安全回收**: 结合 P0-02 对账发现，当确认券商端零候选判定为 `FAILED` 时，执行安全回收，不再永久泄露配额。
   - **核心验证用例 (cargo test -p jftrade-store-sqlite --lib strategy_runtime_observation)**:
     * `test_reserve_daily_order_no_double_counting_and_safe_reclaim`: 验证配额统计无 2x 双重计数，并验证崩溃/未成交订单的安全配额回收。
     * `test_stress_quota_reservation_substring_collision_counter_evidence`: 验证 `:1:1` 在途与 `:1:10` 已提交时，在限额为 2 条件下第 3 笔预留被严格拦截为 `Err(StrategyRuntimeStoreError::Conflict)`，反证并闭环子串碰撞。
     * `test_stress_quota_lifecycle_exact_counts`: 验证配额从预留、提交到释放各阶段计数的严格精确单调性。
     * `test_stress_concurrent_quota_reservations`: 验证多并发预留下的 CAS 事务隔离与配额上限刚性约束。
3. **PAUSED 实例抢先启动并发竞争 (Race Condition)**:
   - **时序推演**: 引擎刚启动、行情源尚未完成握手时，用户通过 API 触发 `POST /strategies/{id}/start`。
   - **失效后果**: 触发 `dependency_error()` 发现行情未就绪，直接将实例状态置为 `FAILED`，导致原本处于 `PAUSED` 状态的策略无法恢复。

---

## 2.1.4 Release Qualification 验证清单
- [ ] **RQ-PINE-01（正常流）**: 实盘产生订单意图，核验 `strategy_audit_events` 严格按序产生 `ORDER_RESERVED` $\to$ `ORDER_SUBMITTED`，`submittedIntentKeys` 准确记录。
- [x] **RQ-PINE-02（异常流 / P2-01 配额生命周期与并发拦截已闭环）**:
  - 配置 `daily_max_orders`，验证并发、在途与已提交订单总额不超过配额上限。
  - 消除 2x 双重计数与 SQL 子串碰撞，验证 `test_reserve_daily_order_no_double_counting_and_safe_reclaim`、`test_stress_quota_reservation_substring_collision_counter_evidence`、`test_stress_quota_lifecycle_exact_counts`、`test_stress_concurrent_quota_reservations` 全部 PASS。
- [ ] **RQ-PINE-03（断网注入）**: 运行中 `kill -9` 杀掉 Node Worker 进程，核验 Rust 记录 `SESSION_APPEND_RETRY`，Node 重启后 Rust 降级为 `open` 并注入 200 根 K 线重建指标。
- [ ] **RQ-PINE-04（升级恢复）**: 策略运行中强杀 Rust 引擎，重启后 `restore_running_instances` 自动接管，核验最新已闭合 K 线未产生重复报单。
