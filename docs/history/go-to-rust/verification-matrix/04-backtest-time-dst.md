# 领域 4：回测时间 / DST / Session 语义

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.4 领域 4：回测时间 / DST / Session 语义（美股 09:30 锚点、夏冬令时切换与纯 UTC 取模算法推演）

#### 2.4.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `pkg/market/us/us.go:17-19`
  - `pkg/market/session.go:251-305`
  - `pkg/backtest/internal/storage/store_aggregate.go:712-760`
- **关键符号**: `RegularWindows`, `SessionAwareIntradayBucketBounds()`, `aggregateKLinesFromBase()`
- **历史行为**:
  Go 基线在 `pkg/market/us/us.go` 中明确声明美股常规交易时段为 `9*60 + 30` 至 `16*60`（09:30~16:00）。在 `session.go` 中，Go 引入了 `SessionAwareIntradayBucketBounds`，结合 `America/New_York` 本地时区动态计算当天的开盘时间戳偏移。然而，在标准 K 线聚合函数 `aggregateKLinesFromBase` 中，若开盘首个分桶不足完整整桶（如 30 根 1m 汇入 60m 桶），Go 采取的策略是在 `flush()` 中**静默丢弃**不足整桶的数据，造成开盘半小时数据形成静默“黑洞”。

#### 2.4.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-calendar/src/manager_policy.rs:131-144`
  - `crates/jftrade-store-sqlite/src/backtest_market_data_aggregation.rs:45-138, 197-199`
  - `crates/jftrade-store-sqlite/src/backtest_market_data.rs:686-698`
  - `crates/jftrade-store-sqlite/tests/backtest_market_data_session_scope.rs:92-156`
- **关键机制**:
  1. **纯 UTC 取模计算**: 在 `backtest_market_data_aggregation.rs` 中，聚合算法退化为纯 UTC 欧几里得整除：
     $$\text{first\_bucket} = \left\lfloor \frac{\text{start\_time\_ms}}{\text{target\_ms}} \right\rfloor \times \text{target\_ms}$$
     $$\text{bucket}(T) = \left\lfloor \frac{T}{\text{target\_ms}} \right\rfloor \times \text{target\_ms}$$
  2. **强制整桶覆盖校验**: `aggregate_bucket` 强制要求：
     `if rows.len() != factor { return Err(missing_coverage(...)); }`
     对于 60m 聚合，`factor = 60`。
  3. **分表物理隔离**: 表名通过 `session_scope` 严格隔离：`regular => "r"`, `extended => "x"`。

#### 2.4.3 微观差异与破坏性边界失效推演

#### 1. 美股常规时段 60m 聚合 100% 崩溃推演 (P0-01 缺陷数学证明)
美股常规交易时段（RTH）固定为美东时间 `09:30:00 ET`。
- **夏令时 (EDT, UTC-4)**:
  $$\text{Market Open} = 09:30\text{ EDT} = 13:30:00\text{ UTC} = 13.5 \times 3,600,000\text{ ms}$$
  $$\text{first\_bucket} = \lfloor 13.5 \rfloor \times 3,600,000\text{ ms} = 13 \times 3,600,000\text{ ms} = \mathbf{13:00:00\text{ UTC (09:00:00 EDT)}}$$
- **冬令时 (EST, UTC-5)**:
  $$\text{Market Open} = 09:30\text{ EST} = 14:30:00\text{ UTC} = 14.5 \times 3,600,000\text{ ms}$$
  $$\text{first\_bucket} = \lfloor 14.5 \rfloor \times 3,600,000\text{ ms} = 14 \times 3,600,000\text{ ms} = \mathbf{14:00:00\text{ UTC (09:00:00 EST)}}$$

**边界推演**:
在 `session_scope = "regular"` 下，数据库中仅有 `09:30 ~ 16:00` 的 K 线。在首个分桶（EDT `13:00~14:00 UTC`，或 EST `14:00~15:00 UTC`）中，前半小时（09:00~09:30）不存在数据，数据库内仅有 30 根 1m K 线（09:30~10:00）。
代码执行第 117 行 `if rows.len() != factor` 时，检测到 `rows.len() = 30` 而 `factor = 60`，**直接抛出硬错误**：
`BacktestMarketDataStoreError::Coverage("missing 60m coverage for US.AAPL [13:00, 14:00)")`。
**灾难性后果：在当前 Rust 实现中，任何尝试对美股常规交易时段执行 60m K 线聚合的请求 100% 发生崩溃，无法返回任何数据！**

#### 2. 2024 年夏冬令时转换日跨小时 K 线切片推演明细表

| 日期与时区 | 本地时钟切片 (New York) | UTC 绝对时间区间 | 1m 原始条数 | Go 基线 (`452dea11`) 行为 | Rust 当前 (`main`) 行为 | 失效模式与偏差分析 |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **2024-03-08**<br>(EST, UTC-5) | 09:30 ~ 10:00 | 14:30 ~ 15:00 UTC | 30 根 | 若用会话感知：锚定 09:30~10:30 (60 根)；标准聚合：30 根不足整桶被**静默丢弃** | 归入 `[14:00, 15:00 UTC)`，`factor=60` 但 `rows=30`，**直接抛出 `missing coverage` 崩溃** | **数据截断 vs 算力崩溃**：Go 丢失开盘半小时数据；Rust 查询直接 500 崩溃 |
| 2024-03-08 | 10:00 ~ 11:00 | 15:00 ~ 16:00 UTC | 60 根 | 聚合出标准整点桶 `[15:00, 16:00)` | 归入 `[15:00, 16:00 UTC)`，`rows=60`，合成成功 | 独立查询此段成功，但连续查询因首桶失败而无法执行至此 |
| 2024-03-08 | 15:00 ~ 16:00 | 20:00 ~ 21:00 UTC | 60 根 | 聚合出尾盘整点桶 `[20:00, 21:00)` | 归入 `[20:00, 21:00 UTC)`，`rows=60`，合成成功 | 尾盘正常 |
| **2024-03-10** | **夏令时生效** | **02:00 EST $\to$ 03:00 EDT** | 0 根 | 周末闭市，无数据 | 周末闭市，无数据 | UTC 偏移量由 -5 变为 -4 |
| **2024-03-11**<br>(EDT, UTC-4) | 09:30 ~ 10:00 | 13:30 ~ 14:00 UTC | 30 根 | Go 解析开盘为 13:30 UTC；标准聚合下依然丢弃 13:30~14:00 | 归入 `[13:00, 14:00 UTC)`，`factor=60` 但 `rows=30`，**再次抛出 `missing coverage` 崩溃** | 错误区间由 `[14:00, 15:00)` 偏移至 `[13:00, 14:00)`，时间偏差整整 1 小时 |
| 2024-03-11 | 10:00 ~ 11:00 | 14:00 ~ 15:00 UTC | 60 根 | 合成 `[14:00, 15:00)` | 归入 `[14:00, 15:00 UTC)`，合成成功 | 此时 UTC 14:00 对应 10:00 EDT，而非上周五的 09:00 EST |
| **2024-11-01**<br>(EDT, UTC-4) | 09:30 ~ 10:00 | 13:30 ~ 14:00 UTC | 30 根 | 开盘半小时不足 60 根被丢弃 | 归入 `[13:00, 14:00 UTC)`，报 `missing coverage` 崩溃 | 错误区间在 `[13:00, 14:00)` |
| **2024-11-03** | **冬令时生效** | **02:00 EDT $\to$ 01:00 EST** | 0 根 | 周末闭市，无数据 | 周末闭市，无数据 | UTC 偏移量由 -4 变为 -5 |
| **2024-11-04**<br>(EST, UTC-5) | 09:30 ~ 10:00 | 14:30 ~ 15:00 UTC | 30 根 | 开盘半小时不足 60 根被丢弃 | 归入 `[14:00, 15:00 UTC)`，报 `missing coverage` 崩溃 | 错误区间后移 1 小时至 `[14:00, 15:00)` |

#### 3. Extended 模式 60m 聚合盘前数据污染开盘价推演 (P1-01 隐患)
当 `session_scope = "extended"` 时，美股包含盘前交易（04:00~09:30 ET）。
在 `09:00~09:30` 有 30 根盘前 K 线，`09:30~10:00` 有 30 根常规 K 线。
在 extended 模式下执行 60m 聚合，分桶 `[13:00, 14:00 UTC)`（即 `09:00~10:00 EDT`）刚好凑齐 60 根 1m K 线！
`aggregate_bucket` 校验通过，但聚合生成的 60m K 线：
- `open` 变成了 **09:00:00 的盘前成交价**，而非美股 09:30:00 的正式开盘价！
- `high` / `low` / `volume` 将极度缺乏流动性的盘前挂单与真实 RTH 交易量混合，导致开盘突破（ORB）等量化回测模型产生严重失真的虚假信号。

#### 2.4.4 Release Qualification 验证清单
- [ ] **TC-D4-01（正常流）**: 在 `session_scope = "regular"` 下向 `US.AAPL` 写入 09:30~16:00 连续 1m 数据，验证读取 5m、15m、30m 聚合结果数量准确无误。
- [ ] **TC-D4-02（异常流 - 阻断门禁）**: 查询美股 regular 模式 60m K 线，验证聚合器支持会话锚定（09:30 本地对应 UTC）或将首个半小时作为截断桶，严禁返回 500 或 `missing coverage` 崩溃。
- [ ] **TC-D4-03（DST 边界注入）**: 注入 2024-03-08 (EST) 与 2024-03-11 (EDT) 数据，验证开盘首桶起始时间分别精确等于 `1709908200000` (14:30 UTC) 与 `1710163800000` (13:30 UTC)。
- [ ] **TC-D4-04（隔离流）**: 在 `session_scope = "extended"` 写入 04:00~10:00 数据，验证 60m 聚合输出中盘前数据不得污染 09:30 官方开盘价。
