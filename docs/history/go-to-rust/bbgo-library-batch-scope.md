# bbgo 基础类型域（pkg/bbgo）对齐批次

本域覆盖 `pkg/bbgo` 下随仓库保留的 bbgo 库代码：定点数（`fixedpoint`）、
行情与交易类型（`types`）、浮点序列（`datatype/floats`）。这些类型大多只被
冻结语料引用，Rust 侧没有同名容器，因此批次结论以“Rust 已覆盖什么 + 差异
为何”为主，仅对断言逐项一致的契约升级为 `[x]`。

## 第八十四批：`pkg/bbgo` 全域收尾（145 条，bbgo 库域归零）

### 范围与结果

- 范围：`pkg/bbgo` 剩余 145 条 `missing`（43 个文件），按 P0（定点数/金额安全：
  `fixedpoint` 的 dec/dec_dnum/dec_legacy/convert/expirable/reduce/slice）→ P1（领域类型语义：
  `types` 的 market/interval/kline/position/balance/account/time/duration 与 `datatype/floats` 价格序列）
  → P2（统计与工具：indicator 助手、rbtree、value_map、standardstream、trade_stat、ring buffer、
  connectivity、orderbook 与其余单文件）顺序分 3 片逐条核对。
- 结果：`missing` 960 → **815**（本批结清 145 条）、`partial` 1837 → **1902**、
  `boundary` 448 → **526**、`[x]` 1202 → **1204**。
- **`pkg/bbgo/**` 全域归零**：域内 145 条 = **2 `[x]` + 65 partial + 78 boundary，0 `missing`**。

### 分片执行

- **P0（22 条，`fixedpoint`）**：`dec_test` 10、`expirable_test` 5、`dec_dnum_test` 3、
  `convert_test` 1、`dec_legacy_test` 1、`reduce_test` 1、`slice_test` 1。证据面为
  `crates/jftrade-kernel`（Fixed8/decimal 与 foundation corpus）与
  `crates/jftrade-marketdata`（TTL 缓存/租约）用例；本片新增 2 个 `[x]`（见下）。
- **P1a（43 条，类型语义）**：`types/market_test` 9、`position_test` 6、`time_test` 4、
  `interval_test` 3、`kline_test` 3、`account_test` 3、`balance_test` 2、`duration_test` 2、
  `datatype/floats/slice_test` 6、`funcs_test` 4、`pivot_test` 1。证据面为
  `crates/jftrade-kernel`（定点文本/时间戳）、`crates/jftrade-marketdata`（tick→K 线聚合、
  市场规则 SSOT、价格 tick 对齐）、`crates/jftrade-trading`（虚拟账户/持仓/可卖数量）、
  `crates/jftrade-backtest` 与 `crates/jftrade-broker`（费用规则、流动性告警、市场规则快照）。
- **P1b（80 条，统计与工具）**：`types/indicator_test` 23、`rbtree_test` 10、`value_map_test` 7、
  `standardstream_test` 6、`trade_stat_test` 5、`trade_ring_buffer_test` 4、`connectivity*` 4、
  订单簿（`rbtorderbook`/`orderbook`/`sliceorderbook`）5、其余单文件 12。证据面为
  `crates/jftrade-integration-futu`（深度投影、推送生命周期、受管会话、帧守卫）与
  `crates/jftrade-backtest`/`crates/jftrade-trading`（指标边界、权益回撤、事件台账顺序）。

### 新增证据（本批唯一 `[x]` 升级，均为先跑通再登记）

- 新增 `crates/jftrade-kernel/tests/fixedpoint_parity.rs`，两条用例都把 Go 断言矩阵逐项搬过来，
  并各自带 `// Parity: go:452dea11:...` 锚点：
  1. `json_keeps_eight_digit_text_and_accepts_legacy_payloads` ←
     `pkg/bbgo/fixedpoint/dec_test.go:231 TestJson`：0→`0.00000000`、1.00000003 原样、
     1.000000003/1.000000008→`1.00000000`、0.999999999→`0.99999999`、1.2e-9→`0.00000000`、
     旧式数值 0.00153917575→0.00153917、6e-8 与 0.000062 之差 0.00006194、`"inf"`/`"+Inf"`→POS_INFINITY。
  2. `parsing_normalizes_percent_scientific_empty_and_non_finite_forms` ←
     `pkg/bbgo/fixedpoint/dec_test.go:206 TestFromString`：0.004075/0.03 原样、0.75%→0.0075、
     1.1e-7→0.00000011、.0%→0、空串→0，以及 inf/Inf/INF/iNF 与 ± 前缀归一为 POS/NEG_INFINITY。
- 这两条用例首跑即通过，说明 Fixed8 的 JSON 契约与解析归一化在 Rust 侧已与基线一致，本批
  **没有生产代码修改**（不存在需要 red→fix→probe 的功能缺口）；`-p jftrade-kernel --all-targets`
  13 passed。

### 保留差异（均为 P2/边界，逐条写在清单结论）

- **定点数展示/格式助手**：Go 有 `FormatString(prec)`、`Round(prec, mode)`、`MulExp`、
  `NumFractionalDigits`、`Percentage/FormatPercentage`、YAML 编解码、dnum/legacy 双实现与内部
  系数读取器、`Reduce`、`Slice/Ascending/Descending` 排序包装、`ExpirableValue` 容器；Rust 只有
  8 位固定文本（`fixed_text`，即 prec=8）、去尾零文本（`storage_text`）与步长截断/对齐，
  排序由派生 `Ord` 承担，TTL 语义由行情缓存（Fresh/Stale/Missing）与租约 `expire` 表达。
- **统计与信号助手**：`types/indicator_test` 23 条里 17 条为 Go 库统计/绘图助手（相关系数、
  协方差、偏度、熵/交叉熵、softmax、sigmoid、逻辑回归、OLS、点积、Plot、Filter、Queue、Clone、
  类型开关、数组拷贝、序列外推）；Rust 只在回测指标（SMA/EMA/MACD）与权益极值上覆盖少量序列
  算术，其余按边界保留。`trade_stat`/`sharpe`/`sortino`/`omega`/CAGR/Kelly/OptimalF/年化波动率
  在 Rust 不做本地计算（透传或前端计算）。
- **库内部结构**：红黑树价格索引、`ValueMap` 权重映射运算、环形成交缓冲、多连接聚合状态机、
  本地可增删订单簿在 Rust 均无对应类型——工作集由 `BTreeMap`/有序 Vec、事件台账、
  提供商深度投影（`market_microstructure_query`）与单一受管会话承担。
- **其余**：交易所枚举、可恢复订单错误接口、排序助手、简单时长解析、随机序列生成器按 Rust
  类型系统/平台能力表达，不做逐字段移植。

### 跨批 follow-up 汇总

- P0：无功能缺口（fixedpoint 的 JSON 与解析契约已逐项一致）。
- P1/P2：无新增功能缺口；本域唯一批准项是上表 2 条 `[x]`。
- 补测候选（P2，已写入相应清单结论，不阻塞）：未知缓存键的 Missing 分类断言
  （`expirable_test.go:43`）、`Fixed8` 排序断言（`slice_test.go:10`）、回撤序列的平均/平方平均
  （`trade_stat_test.go:46`）、`ServerStream` 类推送的原始消息模式。

### 仍未结清（下一批）

- 下一批（第八十五批）范围：按域余量排序的下一块 **`internal/integration` 141 条**，先按文件分组
  recon 再按 P0 → P1 → P2 分片；其后：`internal/marketdata` 112、`pkg/futu` 86、`internal/trading` 80、
  `internal/backtest` 63、`pkg/market` 56、`internal/marketdataassets` 36、`internal/watchlist` 31、
  `pkg/broker` 29、`pkg/researchscreen` 24、`internal/settings` 24，直至 4451 条清单全部完成。

验证：`cargo fmt --all -- --check`、`cargo clippy -p jftrade-kernel --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-kernel --all-targets --locked --no-fail-fast`（**13 passed / 0 skipped**，含新增 2 条 fixedpoint parity 用例）、`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2925 Rust** / **1204 `[x]`**；missing 815、partial 1902、boundary 526、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线，7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）、`pnpm run check:compatibility`、`node scripts/check-zero-go.mjs`（2888 tracked files / 0 release artifact）、`pnpm run check:rust:architecture`、`git diff --check`、`pnpm run check:quick`（EXIT=0）、`pnpm run check:ai-context`。
