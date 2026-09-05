# 领域 2：Provider 切换（行情源动态切换与读写屏障）

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

## 2.2.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `internal/settings/market_data.go:109-138`
  - `internal/app/apiserver/marketdataapp/runtime_test.go:33-160`
- **关键符号**: `SaveActiveMarketDataProvider()`, `OnProviderChanged()`, `reconcileSubscriptionsForCleanup()`
- **历史行为**:
  Go 基线在切换行情源时，虽然尝试在 `OnProviderChanged` 失败时调用 `SaveActiveMarketDataProvider` 进行磁盘回滚，但其切换流程**缺乏跨组件全局互斥保护**。内存中的行情缓存没有世代（Generation）区分，且回滚时仅被动尝试清理订阅，无法防止旧网络连接残留的 Tick 污染新行情源的数据流。

---

## 2.2.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-engine/src/product_active_provider_state.rs:20-30, 116-148`
  - `crates/jftrade-marketdata/src/router.rs:34-41, 115-155`
  - `crates/jftrade-engine/src/product_runtime_provider_activation.rs:153-166, 420-427`
  - `crates/jftrade-marketdata/src/cache.rs:13-88`
  - `crates/jftrade-settings/src/market_data_provider.rs:132-153`
- **关键机制**:
  1. **全局切换互斥锁与快照**: `ActiveProviderState` 持有 `transition: Arc<Mutex<()>>` 与 `snapshot: Arc<RwLock<ProviderRuntimeSnapshot>>`。激活新源时必须持有 `transition` 排他锁，并原子递增快照代际：`snapshot.generation = snapshot.generation.saturating_add(1)`。
  2. **活跃策略订阅强阻断**: 切换前调用 `has_managed_consumers()`（`product_runtime_provider_activation.rs:153-166`）。若存在处于活动态的量化策略订阅，直接抛出 `MANAGED_SUBSCRIPTIONS_ACTIVE` 拒绝切换，彻底避免策略指标在算力中途遭受跳变。
  3. **TickCache 代际屏障与全量抹除**: 切换激活时立即获取 `cache` 锁并调用 `clear()`，清空旧行情源所有残留 Tick，并同步自增 `generation`。上游推送调用 `insert()` 时强校验 `tick.provider_generation == active_generation`；上层查询 `lookup_for_generation` 校验不匹配时返回 `CacheLookup::Missing`，杜绝旧缓存伪造为当前行情。
  4. **OpenD 交易与行情角色解耦**: 当从 Futu 切换到 YFinance 或 AKShare 时，代码**明确保持 OpenD 运行时存活**（`product_runtime_provider_activation.rs:420-427`）。OpenD 交易会话（Trade Session）继续为后台对账与撤单提供支持。
  5. **磁盘配置双向事务回滚**: 在 `crates/jftrade-settings/src/market_data_provider.rs:132-153` 中，先写磁盘 `save_active_market_data_provider(next)`，再调用 `runtime.activate(next)`；若运行时激活失败，立即将原配置 `current` 写回磁盘。

---

## 2.2.3 微观差异与破坏性边界失效推演
1. **磁盘先写但回滚前遭遇硬崩溃 (Crash Gap)**:
   - **时序推演**: 磁盘 `settings.json` 已写入新源（如 `futu`），在调用 `runtime.activate` 失败并准备写回 `current` 时，进程被 `kill -9` 终止。
   - **失效后果**: 磁盘配置记录为破损的新源。
   - **自愈机制**: 重启时引擎在 `product_runtime_start.rs:620` 尝试启动配置的数据源，若依赖不可用，Supervisor 会在启动阶段安全报错或降级，不会伪造正常对外服务。
2. **切换期间网络延迟导致旧 Tick 晚到 (Race Condition)**:
   - **时序推演**: 旧行情通道断开瞬间，网络缓冲区中积压的 Tick 晚于 `cache.clear()` 到达。
   - **防御机制**: 旧 Tick 携带旧的 generation，在 `insert()` 阶段直接被拒绝写入。

---

## 2.2.4 Release Qualification 验证清单
- [ ] **RQ-PROV-01（正常流）**: 无策略运行时，调用 `PUT /api/v1/settings/market-data-provider` 将数据源从 `futu` 切至 `yfinance`，验证返回 200，TickCache 清空且代际加 1。
- [ ] **RQ-PROV-02（异常流）**: 启动一个实盘 Pine 策略，发起切换行情源请求，验证被强行拦截并返回 400/409 `MANAGED_SUBSCRIPTIONS_ACTIVE`，配置无污染。
- [ ] **RQ-PROV-03（断网注入）**: 阻断 Python Helper 网络使 `/healthz` 失败，尝试切换至 `yfinance`，验证激活失败并触发磁盘配置自动回滚回 `futu`。
- [ ] **RQ-PROV-04（升级恢复）**: 模拟磁盘落盘新源后进程硬崩溃，验证重启时 Supervisor 能准确识别外部不可用并安全处于 Degraded 状态。
