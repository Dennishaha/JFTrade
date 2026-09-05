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
2. **预留成功后崩溃导致的配额泄漏 (Leak Path - P2-01)**:
   - **时序推演**: `reserve_daily_order` 写入 `ORDER_RESERVED` 后立即崩溃，未执行后续下单与释放。
   - **失效后果**: 该孤儿预留事件将一直占用 1 个配额槽位直到 UTC 次日零点，导致当天策略可下单次数被动减少 1 次。
3. **PAUSED 实例抢先启动并发竞争 (Race Condition)**:
   - **时序推演**: 引擎刚启动、行情源尚未完成握手时，用户通过 API 触发 `POST /strategies/{id}/start`。
   - **失效后果**: 触发 `dependency_error()` 发现行情未就绪，直接将实例状态置为 `FAILED`，导致原本处于 `PAUSED` 状态的策略无法恢复。

---

## 2.1.4 Release Qualification 验证清单
- [ ] **RQ-PINE-01（正常流）**: 实盘产生订单意图，核验 `strategy_audit_events` 严格按序产生 `ORDER_RESERVED` $\to$ `ORDER_SUBMITTED`，`submittedIntentKeys` 准确记录。
- [ ] **RQ-PINE-02（异常流）**: 配置 `daily_max_orders = 3`，连续产生 4 笔信号，核验第 4 笔被 `reserve_daily_order` 拦截返回 409 `DAILY_LIMIT_REACHED`，策略平滑暂停。
- [ ] **RQ-PINE-03（断网注入）**: 运行中 `kill -9` 杀掉 Node Worker 进程，核验 Rust 记录 `SESSION_APPEND_RETRY`，Node 重启后 Rust 降级为 `open` 并注入 200 根 K 线重建指标。
- [ ] **RQ-PINE-04（升级恢复）**: 策略运行中强杀 Rust 引擎，重启后 `restore_running_instances` 自动接管，核验最新已闭合 K 线未产生重复报单。
