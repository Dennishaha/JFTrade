# productfeatures 域对齐批次（第 99 批）

本文件记录 Go 基线 `internal/productfeatures` 的 earnings-calendar 查询校验与 workspace
行情读取（snapshot/security/candles/depth 信封、分页元数据、sessions 形状、CN 聚合、
session 快照投影）迁移到 Rust 的逐测试核对结论，涉及 `crates/jftrade-engine`
（`research_earnings_calendar_query*.rs`、`product_production_ports_research*.rs`、
`product_production_ports_market_data_*`、`product_query.rs`、`product_candle_converter.rs`）。

## 第九十九批：productfeatures 收口（12 条）

### 范围与分片

`internal/productfeatures` 共 12 条 `missing`，两组：

- P1 `earnings_calendar_query_test.go` 3：接受业务参数表（:11）、拒绝 12 行非法参数表（:35）、
  非 earnings operation 跳过校验（:72）。
- P1 `market_data_reads_test.go` 9：显式 provider 与响应形状（:12）、非法 instrument（:116）、
  candle 分页元数据矩阵（:134）、sessions 参数形状（:226）、CN 聚合→交易所叶子（:245）、
  provider 失败与快照回退（:307）、常规会话收盘对比（:371）、活动扩展时段字段（:418）、
  扩展时段回退稳定性（:470）。

### 关键事实（本批 recon 实测）

- Go 的 `validateResearchCalendarQuery` 是单一 validator（sort/stockScope/日期/区间四类规则），
  Rust 拆成两个 owner：`translate_earnings_calendar_params`（sort 1–6、stockScope 0–3、
  五个区间 rule 的 optionOnly/percentage/min>max）与 `earnings_calendar_date_chunks`
  （`YYYY-MM-DD`、end≥begin、42 天上限、≤7 天分片）。`operation` 门控位于
  `ProductionResearchPort::read`：空或 `earnings` 才进入 OpenD earnings reader，
  其余 operation 落到 helper/calendar 路径。
- workspace 行情读取的 Rust owner 是 `ProductionMarketDataQuotePort`
  （active-provider 网关门 + helper/trade-runtime），路径解析与 instrument 归一化在
  `parse_market_symbol_path`；candle 分页在 `product_candle_converter::validate_pagination`
  与 `validate_timestamp_order`；快照会话/收盘投影在
  `quote_snapshot::project_cached_snapshot` / `project_fallback_snapshot`。
- 冻结 wire fixture `stage9.market-data-quote-read.v1` 只包含
  `securities-cn-qualified-normalizes-to-leaf`（`CN/SH.600519` → 200）与
  `depth|ticks|broker-queue|capital-flow` 的 `brokerId` 形态，没有裸 CN 用例；
  Go 的 broker-registry 分流（`brokerId` 存在且不是 active non-broker provider 才走
  workspace reader）在 Rust 已是登记边界（见 `routes_boundaries_test.go:239` 行）。
- Go 的 `workspaceSnapshot` 与 Rust 的 typed 投影语义一致：活动扩展时段块以“正价格”为门控
  覆盖 price/high/low/volume/turnover，US 非 regular 会话把最近常规收盘价写进
  `previousClosePrice`，`lastClosePrice` 保留 provider 原值，`extendedHours` 由会话判定。

### 结果

- 12 条全部给出结论：**11 条 `[x]`/`function_exact` + 1 条 `partial`**，
  `internal/productfeatures` 域内 `missing` 归零。
- 全局：4451 = function_exact **1287** + partial **2528** + boundary 567 + module_only 4 +
  missing **65**（前批 1276 / 2527 / 567 / 4 / 77），Rust 测试 2983 → **2996**。
- 锚点对账：anchors 1278 → **1290**（本批 12 个新锚点全部已记账），unrecorded 保持 **0**。

### 本批新增测试（13 条）

`[x]` 行证据（11 条）：

- `earnings_calendar_accepts_the_go_business_parameter_table`（:11）：同一张业务参数表同时
  通过 translate 与 chunker，断言 sort=6、6 个 filter、scope filter（4/[1]）与 39 天窗口。
- `earnings_calendar_rejects_the_go_invalid_parameter_table`（:35）：12 行非法表逐行在
  translate 或 chunker 报错。
- `futu_calendar_route_skips_earnings_validation_for_other_operations`（:72）：
  `operation=dividends` 不含 earnings 校验错误，`operation=earnings` 仍拒绝 option-only sort。
- `workspace_reads_reject_missing_market_and_symbol`（:116）：8 条空段路径 400 BAD_REQUEST。
- `workspace_candle_pagination_rejects_the_go_metadata_table`（:134）：7 行非法分页 +
  合法页 + `has_more` 缺失的反序列化拒绝。
- `candle_sessions_accept_padded_and_csv_query_shapes`（:226）：`" regular "`/`extended`
  与 `"regular, overnight"` 两种形状归一后两项互异。
- `workspace_reads_resolve_cn_aggregate_to_exchange_leaf`（:245）：见下节功能修复。
- `workspace_reads_surface_provider_failures_before_normalizing`（:307）与
  `fallback_projection_preserves_observation_fallbacks`（:307）：provider 502 上抛 +
  nil/非正价格返回 None + `updateTime`/fallback 两种 observedAt 取值。
- `workspace_snapshot_restores_regular_close_comparison_semantics`（:371）：US regular /
  US after / SZ unknown 三行收盘对比。
- `workspace_snapshot_uses_active_extended_session_fields`（:418）：pre/after/overnight
  三行 7 字段覆盖。
- `workspace_snapshot_extended_session_fallbacks_remain_stable`（:470）：regular/closed
  忽略陈旧块、overnight 缺块/零价/部分块三类回退。

`partial` 行证据（1 条 + 1 条辅助）：

- `workspace_read_envelopes_preserve_provider_and_response_shape`（:12）：helper 侧
  snapshot/security/candles 信封（meta/request/period/at/before/pagination）与叶路径。

### 功能修复（含先红后改）

- **CN 聚合→交易所叶子**（`market_data_reads_test.go:245`）：Rust 原先把 `CN` 原样透传，
  `CN/SH.600519` 会以 `CN.SH.600519` 直达 provider。新增
  `product_production_ports_market_data_projection.rs::resolve_market_aggregate_leaf`
  并被 `parse_market_symbol_path` 调用：`CN` + `SH.`/`SZ.` 前缀解析为叶市场，裸 CN 在
  provider 之前以 400 BAD_REQUEST 失败；其余市场（含 legacy 小写路径）保持路由原值。
  复现测试 `workspace_reads_resolve_cn_aggregate_to_exchange_leaf` 先转红
  （`request.market = "CN"` vs `"SH"`），修复后转绿。
- **既有与 Go 冲突的夹具修正**：非锚定集成测试
  `crates/jftrade-engine/tests/market_data_production_compatibility.rs`
  的 akshare K 线用例原先请求裸 `CN/600519`（Go legacy `svc.GetCandles` 透传路径）；
  该输入在 Go 的 workspace reader 基线与冻结 wire fixture 下都无依据，改为叶子市场
  `SH/600519`（mock 响应同步改 `market/instrumentId`），断言（session=null、
  extendedHours=false）不变。
- **机械抽取**：`product_production_ports_market_data_quote_snapshot.rs` 新增测试后超过
  800 行生产文件上限，测试模块抽到同目录 `product_production_ports_market_data_quote_snapshot_tests.rs`
  （`#[path]` 引用，测试名与锚点不变）。

### 探针（改坏 → 转红 → 按字节回滚）

1. CN 缺口：先写复现测试 → `left: String("CN")` / `right: "SH"` 转红 → 实现叶解析 → 转绿。
2. `uses_regular_close_as_previous_close` 去掉 `is_us_market(market)` 判定 →
   `workspace_snapshot_restores_regular_close_comparison_semantics` 转红
   （`SZ/unknown previousClosePrice` 74.10 ≠ 72.76）→ 按字节回滚，
   `product_production_ports_market_data_quote_snapshot.rs` 恢复
   `c26a6cab5a30153798fdfce91294a8ff99d7a81a` 且测试转绿。

### 保留差异（记录在 partial 结论里）

- `market_data_reads_test.go:12` 的 depth 半：Rust workspace 读不产出 `request/depth/meta`
  信封——Futu depth 走 OrderBook 微结构 reader，非 Futu depth 以 409
  `MARKET_DATA_CAPABILITY_UNSUPPORTED` 拒绝（既有锚点行
  `market_depth_test.go:143`）；Go 的 broker `FeatureMarketDepth` 投影在 Rust 无对应 owner。
- Go workspace reader 对 market/symbol 的整体大写归一未在 Rust 全量套用：legacy
  `/api/v1/market-data/candles` 契约要求小写透传
  （`candle_route_preserves_legacy_query_parsing` 仍绿），因此本批只落地 CN 聚合解析与
  裸 CN fail-closed 两条规则，其余大小写语义保持现状。
- Go 的 `missing hasMore` 行在 Rust 由 typed `HelperCandlesResponse`（非可选 `has_more`）
  承接，缺字段即反序列化失败；`brokerId` 分流本身仍是登记边界。

### 后续待办

- 下一批：`pkg/observability`（10）→ `cmd/internal`（9）→ `internal/pineworkerassets`（7，锚点例外）
  → `internal/desktop`（6）→ `internal/retry`（6）→ `internal/jftsettings`（5）→
  `internal/frontendassets`（4，锚点例外）→ `internal/research`（4）→ `internal/security`（3）→
  其余小片，直到 4451 条清单全部完成。
- 若后续需要让非 Futu provider 也走 workspace instrument 归一化，必须在 transport 层先补
  `brokerId` 分流 owner，再回填 `market_data_reads_test.go:12` 的 depth 与大小写差异，
  不要在 API handler 里复制归一化逻辑。

`pnpm run check:quick` **EXIT=0**：target-health、policy/contracts、`cargo-nextest`
（**1770 tests run / 1770 passed / 0 skipped**）、fmt、clippy、compatibility replay
（market-data/Pine/OpenD/SQLite/trading 五组）、前端与桌面 Node 套件（48 tests pass）全部通过。

`pnpm run check:rust` **EXIT=1**，唯一失败项与本批 diff 无关且是既有基线失败，保留原始证据不记为通过：
target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 全部通过后，
`check:rust:policy` 的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，
修复 >=0.23.45），另有 **8 条** `warning[advisory-not-detected]`（陈旧 ignore）；run 在 policy 处中止，
因此该次运行的 workspace 测试阶段未执行（本批的 crate 级证据以上面的 1770 条 nextest 为准）。
本批未改动 `Cargo.toml`/`Cargo.lock`。

验证：`cargo fmt --all`（EXIT=0）；`cargo clippy -p jftrade-engine --all-targets --locked`（EXIT=0，无告警）；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（**1750 passed / 0 skipped**，EXIT=0）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2996 Rust** / **1287 `[x]`**；missing 65、partial 2528、boundary 567、module_only 4；`OK: 1287 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors **1290**；already recorded 1201、**unrecorded 0**、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`（EXIT=0）；`pnpm run check:compatibility`（EXIT=0）；
`node scripts/check-zero-go.mjs`（2903 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；
`git diff --check`（EXIT=0）。
