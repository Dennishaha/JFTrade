# 领域 6：Futu/OpenD 与 Python Helper 运行时韧性

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.6 领域 6：Futu/OpenD 与 Python Helper 运行时韧性（OpenD 退避重连、子进程崩溃热拉起鸿沟与角色解耦）

#### 2.6.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `pkg/futu/exchange_client.go:39-137`
  - `pkg/strategy/pineworker/manager.go:281-285, 406-412`
- **关键符号**: `withRetryingClient()`, `restartWorkerLocked()`
- **历史行为**:
  Go 基线在 Futu 连接失败时执行单次即时重连，容易在网络不稳定时诱发连接风暴。但在外部子进程管理上，Go 的 `pineworker/manager.go` 明确实现了 `restartWorkerLocked`，当检测到 Node Worker 探活失败时，能够自动杀掉僵死进程并拉起全新实例。

#### 2.6.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-integration-futu/src/runtime_task.rs:307-310, 451-457`
  - `crates/jftrade-integration-futu/src/session_coordinator.rs:369-414`
  - `crates/jftrade-integration-marketdata-helper/src/process.rs:78-230`
  - `crates/jftrade-engine/src/product_runtime_helper_health.rs:63-158`
  - `crates/jftrade-integration-pine/src/pool.rs:199-210`
  - `crates/jftrade-engine/src/product_runtime_provider_activation.rs:170-427`
- **关键机制**:
  1. **OpenD 二进制指数退避**:
     $$\Delta t = \text{initial} \times 2^{(\text{failures}-1)}, \quad \text{initial} = 250\text{ms}, \quad \text{max} = 5000\text{ms}$$
     退避延迟序列为 $250\text{ms} \to 500\text{ms} \to 1000\text{ms} \to 2000\text{ms} \to 4000\text{ms} \to 5000\text{ms}$。
  2. **会话重建与配额重放**: 重连后自增 `generation`，重新发送 `Qot_Sub` 3001 订阅全部标的，并置位 `quota_refresh_pending = true`，主动调用 `Qot_GetSubInfo` (3003) 向服务端拉平配额。
  3. **行情源切换 OpenD 交易解耦**: 切换至 `yfinance` 时，保持 OpenD 交易会话与对账后台持续运行。

#### 2.6.3 微观差异与破坏性边界失效推演

#### 1. 关键韧性倒退：外部子进程缺乏自动热拉起 (P0-03 缺陷推演)
- **Python Market-Data Helper**:
  运行期间由 `HelperHealthMonitor` 每 5 秒探测一次 `/healthz`。当 Python 进程因 OOM、依赖崩溃或被信号杀死时，探针仅执行 `snapshot.healthy = false; snapshot.last_error = Some(...)`。
  **全系统没有任何代码去重新调用 `HelperProcess::start`！** 新闻、日历、选股与 YFinance 历史数据永久返回 502。
- **Node PineTS Worker**:
  `WorkerPool` 中虽然定义了 `record_restart` 方法，但在整个 `crates/jftrade-engine` 生产运行时中**从未被任何健康监控或任务异常捕获逻辑调用**！Node 进程一旦崩溃，所有实盘 Pine 策略指标计算直接永久瘫痪。
  **对比 Go 基线：Rust 现存明显的子进程自动运维韧性倒退，属于严重 P0 阻断性缺陷。**

#### 2. 冷启动 OpenD 未初始化隐患 (P2-02)
- **原始推演假说**: 若系统以 `market_data_provider = "yfinance"` 冷启动，系统仅启动 Python Helper，不会初始化 OpenD。历史在途的 Futu 订单将无法自动对账，直至用户手动切源或激活。
- **现实验证反证（风险不成立）**:
  在生产运行时组合入口 `crates/jftrade-engine/src/product_runtime_composition.rs:48-78`，Rust 架构在冷启动时已实现券商交易与行情源的明确解耦：
  ```rust
  // Futu's OpenD session is a shared trade owner, not merely a market-data
  // provider. When a broker integration is enabled, compose it even if
  // yfinance/AKShare currently owns market-data reads; reconciliation then
  // keeps account/order/history/fill/fee visibility across provider switches.
  let futu_trade_enabled = store ... .is_some_and(|integration| integration.enabled);
  if active_provider == Some(MarketDataProvider::Futu) || futu_trade_enabled || futu_env_override {
      let provider_config = opend_provider_config(settings_path, Arc::clone(&router))?;
      config.market_data_opend_provider = Some(provider_config);
      config.market_data_router = Some(router);
  }
  ```
  既有生产回归测试（`product_production_ports_execution_reconciliation_provider_tests.rs:136` `helper_market_data_providers_reconcile_futu_account_order_fill_and_fee` 与 `product_production_ports_trade_tests.rs:363` `helper_market_data_provider_keeps_futu_trade_reads_on_the_trade_session`）充分证实：即便冷启动且活跃行情源为 `Yfinance` 或 `Akshare`，只要开启了 Futu broker，交易会话（OpenD Trade Session）独立保持运行，后台对账 Worker（`ExecutionReconciliationWorker`）持续对账 Futu 历史订单、成交与佣金，完全不受行情源影响。在 OpenD 离线时，系统严格 Fail-Closed 并显式暴露错误，绝无虚假数据。

#### 2.6.4 Release Qualification 验证清单
- [ ] **TC-D6-01（正常流）**: 在 OpenD 订阅 5 支标的时用 `iptables` 阻断连接 3 秒后解封，核验退避重试成功，`generation` 自增，5 支标的自动重订阅，配额自动对齐。
- [ ] **TC-D6-02（韧性流 - 阻断门禁）**: `kill -9` 杀掉 Python Helper 进程，核验系统必须在 5 秒内通过 Supervisor 检测到进程死亡并自动重新 spawn 拉起，`/healthz` 恢复 200。
- [ ] **TC-D6-03（韧性流 - 阻断门禁）**: 向 Node Worker 发送 `SIGTERM`，核验 `WorkerPool` 捕获断连后自动重新拉起 Node 进程，`restarts` 加 1，策略会话自愈。
- [x] **TC-D6-04（解耦流 / P2-02 闭环）**: 启动以 `futu` 运行并挂一笔委托，切源至 `yfinance`，核验行情切换成功且后台继续轮询并同步 Futu 订单成交。

#### 2.6.5 闭环验证台账 (P2-02)

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P2-02 / worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 生产代码基线 `ccac83d1` / `crates/jftrade-engine/src/product_runtime_composition.rs:48-78`、`product_production_ports_execution_reconciliation_provider_tests.rs:136-197` |
| 状态 / 确认严重度 | **已关闭 / PASS (事实反证)** (原 P2 级推演在当前架构下不成立，已由角色解耦机制彻底消除) |
| 生产调用链 / 所有者 | `compose_market_data_runtime` -> `futu_trade_enabled` -> `OpenDProviderRuntime` -> `ExecutionReconciliationWorker` |
| 复现或反证 | 原材料推测以 yfinance 冷启动时因未选 Futu 行情而导致 OpenD 未初始化、历史订单无法对账；反证证据证实：架构将 OpenD 交易会话作为独立属主管理，冷启动仅要求 `futu_trade_enabled` 即可初始化交易会话与对账后台；已由 `helper_market_data_providers_reconcile_futu_account_order_fill_and_fee` 等测试全链路验证通过。 |
| 修复 / 回归 | 既有回归测试（`product_production_ports_execution_reconciliation_provider_tests.rs` 及 `product_production_ports_trade_tests.rs`）：<br>• `helper_market_data_providers_reconcile_futu_account_order_fill_and_fee`<br>• `helper_market_data_providers_fail_closed_without_futu_trade_session`<br>• `helper_market_data_providers_project_futu_broker_and_portfolio_routes`<br>• `helper_market_data_provider_keeps_futu_trade_reads_on_the_trade_session`<br>• `provider_switch_does_not_disconnect_an_existing_futu_trade_session`<br>• `helper_market_data_provider_without_trade_session_fails_closed` |
| 门禁 | `cargo test -p jftrade-engine --lib helper_market_data_provider` (全量通过，退出码 0)；`pnpm run check:quick` (退出码 0) |
| 剩余风险 / 依赖 | OpenD 宿主进程必须外部处于运行状态；当 OpenD 挂掉时，交易接口显式报错 Fail-Closed，符合预期。 |
