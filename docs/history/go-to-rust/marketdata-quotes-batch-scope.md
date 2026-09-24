# MarketData、Quote、Provider 领域对齐批次

本批范围固定为 Go 基线中的 `internal/marketdata/cache_test.go` 与
`internal/marketdata/broker_candles_test.go` 两个文件，共 10 条复合键
（Go 路径、起始行号、测试名）。逐条阅读 Go 断言并对照 Rust 实际测试函数后，
没有任何条目满足全断言等价，因此统一保留为 `[~]`/`partial`，不把“存在同名
Parity 函数”当作覆盖完成。

| Go 测试范围 | Rust 证据 | 结论 |
| --- | --- | --- |
| `cache_test.go:12` dedupe/promotion/inherit | `cache_boundaries::test_cache_deduplicates_promotes_and_inherits` | 同价 promotion 有证据；trade 字段继承、observedAt 保留等断言缺失 |
| `cache_test.go:76` freshness/retention/max | `cache_boundaries::test_cache_freshness_retention_and_maximum` | fresh/stale 有证据；retention 截断、最大容量和 AllFresh 缺失 |
| `cache_test.go:103` 跨交易日 extended session | `cache_extended_sessions_parity::test_cache_does_not_inherit_extended_sessions_across_trading_days` | trading_date/session/pre/after 有证据；ExtendedHours、overnight、book/volume 缺失 |
| `cache_test.go:138` after-hours regular-close promotion | `cache_extended_sessions_parity::test_cache_promotes_us_regular_close_when_after_hours_trade_arrives` | close promotion 有证据；JSON 序列化与后续 tick 保留缺失 |
| `cache_test.go:185` 同价新 extended quote | `cache_extended_sessions_parity::test_cache_retains_extended_quote_when_price_is_unchanged` | after-market quote 有证据；样本计数、时间及 close 上下文缺失 |
| `broker_candles_test.go:12` strict page projection | `product_market_data_candle_pagination_tests::test_broker_k_line_candles_response_projects_strict_page` | 页大小/分页标记有证据；具体 wire 值未锁定 |
| `broker_candles_test.go:51` terminal/bounded pages | `...::test_broker_k_line_candles_response_handles_terminal_and_bounded_pages` | 仅 terminal 分支，bounded 未覆盖 |
| `broker_candles_test.go:77` invalid provider rows | `...::test_broker_k_line_candles_response_rejects_invalid_provider_rows` | 仅 close 数值校验，Go 的其他错误矩阵缺失 |
| `broker_candles_test.go:109` helper classification | `...::test_broker_k_line_helpers_classify_sessions_and_numbers` | 时间/数字解析有证据；regular/after session 分类缺失 |
| `broker_candles_test.go:137` pagination metadata | `...::test_broker_k_line_pagination_rejects_invalid_bounded_and_paged_metadata` | 两个错误场景有证据；bounded cursor 等边界缺失 |

验证命令按单函数记录在 `manual-test-mappings.json`；本批使用
`jftrade-marketdata` 与 `jftrade-engine` 的 nextest wrapper。后续补测应先补齐
缺失断言，再将对应条目从 `partial` 升级为 `function_exact`/`[x]`。

## 第二批：runtime / sidecar / health + Futu candle session 标注

批次范围：`internal/app/apiserver/marketdataapp/` 的 `runtime_test.go`（22）、
`runtime_health_test.go`（11）、`sidecar_process_test.go`（9）、`market_http_test.go`（18）、
`provider_boundaries_test.go`（5）、`provider_test.go`（3）、`query_test.go` 等边界文件。
逐条对照后：18 条找到全断言等价的 Rust 测试（`[x]`/`function_exact`），14 条为
`[~]`/`partial`（有部分证据但断言集合不同），10 条为 `[~]`/`boundary`
（provider lease 引用计数旧实例、sidecar 进程清理重试、nil-receiver、deferred cleanup
等只在 Go/Wails 进程模型内成立的边界）。

### 真实功能差异与修复（P0：会话标注正确性）

- **复现条件**：Futu provider、US 市场、`period=1d`（或任意非 US 市场）请求
  `GET /api/v1/market-data/candles/US/AAPL?period=1d`；此前 Rust 会对每根 K 线
  固定写入 `"session": "regular"`。
- **Go 预期行为**：`ShouldAnnotateHistoricalKLineSession` 只在 US 且 intraday
  period 时标注 session；daily 与非 US 市场完全不输出 `session` 字段，且
  `meta.session` 仅在发生逐根标注时出现。
- **修复位置**：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`
  （`read_candles` Futu 分支）新增 `annotate_session` 判定与
  `futu_candle_session` 辅助函数：用 `jiff` 解析 `at`，再经
  `CalendarManager::classify_session` 映射 `pre`/`regular`/`after`/`overnight`；
  未配置日历时返回 `None`（fail-closed，不臆造 session）。
- **回归测试**：
  `crates/jftrade-engine/src/product_market_data_candle_pagination_tests.rs::candle_route_only_annotates_sessions_for_us_intraday_history`
  与 `...::us_intraday_futu_candles_carry_calendar_resolved_session_labels`
  （断言 pre@13:00Z / regular@15:00Z / after@22:00Z 标签，daily 不输出 session）。

### 结构整理

`product_production_ports_market_data_quote_reads.rs` 因新增逻辑超过 800 行生产文件
上限，按既有 `#[path = ...] mod` 模式把 Futu `KLineQueryWindow` / `effectivePeriodSeconds`
/ `parseFutuTimeToTS` 辅助函数与其两个 `query_test.go` 回归测试拆分到
`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads_futu.rs`
（155 行），主文件回到 710 行；映射条目 `rust_entry` 同步更新到新路径。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`
（1110 passed）、`jftrade-marketdata`（51 passed）、
`jftrade-integration-marketdata-helper --all-targets`（19 passed）、
`pnpm run check:rust:architecture`、`python3 scripts/compatibility/audit_test_parity.py`。

## 批次 C：tick candle 服务路径对齐（go:452dea11）

`internal/app/apiserver/marketdataapp/market_http_test.go` 的三条 tick candle 测试
（`TestMarketCandlesTickResponseUsesFreshCache`、`...QueriesTickerOnCacheMiss`、
`...FallsBackToCachedCandlesOnTickerError`）此前在 Rust 中没有对应实现：Rust 把
`period=tick` 直接交给 `historical_klines_window`，即用 historical K-line
读代替 Go 的 tick cache 读。Go 的实际行为是：

1. `requireProviderCapability(ctx, "tick candles")` 与 session 解析（US 默认
   `regular,extended,overnight`）。
2. `requireBasicSubscriptionDemand(market, symbol, "TICK")`——注意 Go 给 tick 读
   的是 `TICK` channel，historical 读才是 `KLINE`。
3. `fromCache := s.cache.Latest(instrumentID, TickFreshness) != nil`；命中即直接
   投影。
4. 未命中时调用一次 `s.provider.QueryTicker`，成功后 `s.Ingest(*sample)`；失败时
   仍从 `s.cache.Snapshot(instrumentID)` 投影，只有在无 candle 可产出时才返回错误。
5. `TickCandles` 把每个 tick 映射为 open=high=low=close=price、
   `volume = VolumeDelta`（负数截断为 0）、`session = sample.Session`，再按请求
   session 过滤、最后 `limitCandleMaps` 取最新 N 条。
6. `tickCandlesResponse` 的 `ExtendedHours`/`IncludeSession` 都等于
   `market == "US"`，因此 US tick 读固定 `extendedHours: true` + `meta.session: "all"`，
   非 US 完全不输出 `meta.session`。

### Rust 实现

- `crates/jftrade-marketdata/src/tick_candles.rs`（新）：provider-neutral
  `tick_candles()` 投影，含 15 分钟默认窗口、负 delta 截断、显式 bounds 与
  `limit` 保留最新；`TickCandle::to_value` 用 `DecimalText` 规范化价格，去掉
  provider 的尾随零以匹配 Go 的 `decimal.String()`。
- `crates/jftrade-marketdata/src/cache.rs`：新增 `TickCache::history(instrumentID)`，
  对应 Go 的 `Cache.Snapshot`（此前只有 `lookup`，无法投影保留窗口）。
- `crates/jftrade-marketdata/src/model.rs`：`Tick` 新增 `volume_delta`
  （对应 Go `Tick.VolumeDelta`）；candle volume 用 delta，而不是累计 volume。
- `crates/jftrade-integration-futu/src/ticker_query.rs`（新）：
  `TickerQuoteReadPort` / `OpenDTickerQuoteReader`，用同一条 OpenD session 执行
  一次受 lifecycle 代次与 BASIC 订阅约束的 BasicQot 读；`Ok(None)` 表示 provider
  返回了 `(nil, nil)`，与错误区分。
- `crates/jftrade-engine/src/product_production_ports_market_data_quote_tick_candles.rs`（新）：
  `read_tick_candles` 承担 cache-first → 单次 ticker → ingest → fallback 的完整
  owner 逻辑，并生成 `tickCandlesResponse` 信封；`read_candles` 在 tick 分支
  提前返回，不再落入 historical reader。
- `crates/jftrade-engine/src/product_runtime_provider_activation.rs`：Futu 激活时
  把 `OpenDTickerQuoteReader` 装进 `SharedTradeReadRuntime`。

### 修复的功能差异（P1，cache/fallback/cancellation 边界）

| 差异 | 复现条件 | Go 预期 | Rust 修复位置 | 回归测试 |
| --- | --- | --- | --- | --- |
| tick 请求被转发到 historical K-line 读 | `GET /api/v1/market-data/candles/US/AAPL?period=tick` | 由 tick cache 提供，非分页，`pagination.hasMore=false` | `product_production_ports_market_data_quote_reads.rs::read_candles` | `tick_candles_use_fresh_cache_without_querying_the_provider` |
| tick 读没有 provider ticker 回退 | 空缓存 + `period=tick` | 调用一次 `QueryTicker` 并 ingest | `..._quote_tick_candles.rs::read_tick_candles` | `tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample` |
| ticker 失败时丢失保留样本 | ticker 失败 + 缓存有旧样本 | 返回旧样本且 `fromCache=true` | 同上 | `tick_candles_fall_back_to_retained_cache_on_ticker_error` / `..._surface_the_ticker_error_when_no_candle_is_retained` |
| tick 读误用 `KLINE` lease | Futu + router + 无 KLINE lease | 409 `MARKET_DATA_SUBSCRIPTION_REQUIRED`（channel `TICK`） | `..._quote_reads.rs` tick 分支 lease channel | `tick_candles_use_fresh_cache_without_querying_the_provider`（fixture 先取 TICK lease） |
| candle volume 用累计 volume | `period=tick` | 用每事件 `VolumeDelta`，负数截断 | `jftrade-marketdata/src/tick_candles.rs` | `tick_candles_use_explicit_volume_delta_across_trading_days` |
| `meta.session` 语义 | 非 US tick 读 | 完全不输出 `meta.session` | `..._quote_tick_candles.rs::tick_candles_response` | `tick_candles_only_annotate_us_session_metadata` |

映射（`manual-test-mappings.json`）：

- `market_http_test.go:382` → `...::tick_candles_use_fresh_cache_without_querying_the_provider` `[x]/function_exact`
- `market_http_test.go:418` → `...::tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample` `[x]/function_exact`
- `market_http_test.go:442` → `...::tick_candles_fall_back_to_retained_cache_on_ticker_error` `[x]/function_exact`

原 `candle_route_serves_tick_period_and_forwards_strict_before_window` 把“tick 走
historical 分页读”当成基线，属于对 Go 行为的误读，已改名为
`candle_route_forwards_strict_before_window` 并只保留 `before` 断言。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`
（1120 passed）、`-p jftrade-marketdata --all-targets --locked`（54 passed）、
`-p jftrade-integration-futu --all-targets --locked`（245 passed）、
`python3 scripts/compatibility/audit_test_parity.py`。

## 批次 D：snapshot cache-first 与 refresh 绕过（go:452dea11）

`market_http_test.go` 的 snapshot 三条测试（`...UsesFreshCache`、
`...QueriesQuoteSnapshotOnCacheMiss`、`...ForceRefreshBypassesCache`）逐条对照后，《Go
行为：`GetSnapshot(ctx, market, symbol, refresh)` 在 `!refresh` 时读
`cache.Latest(instrumentID, TickFreshness)`，命中即以 `fromCache=true` 返回；否则
`provider.QuerySnapshot` + `generation` fence + `Ingest`；`refresh=true` 时**完全
跳过缓存读**。

### 修复的功能差异（P1，cache/refresh 边界）

| 差异 | 复现条件 | Go 预期 | Rust 修复位置 | 回归测试 |
| --- | --- | --- | --- | --- |
| `refresh=true` 仍命中缓存 | Futu + 缓存有 fresh 样本 + `?refresh=true` | 跳过缓存并调用 provider，`fromCache=false` | `product_production_ports_market_data_quote_reads.rs::read_snapshots` | `snapshot_route_force_refresh_bypasses_the_cache` |
| 缓存命中路径缺少断言证据 | Futu + 缓存有 fresh 样本 | `fromCache=true`，字段全部来自缓存样本 | 同上（已有实现，补证据） | `snapshot_route_serves_a_fresh_cache_hit_without_provider_access` |

映射（`manual-test-mappings.json`）：

- `market_http_test.go:245` → `...::snapshot_route_serves_a_fresh_cache_hit_without_provider_access` `[x]/function_exact`
- `market_http_test.go:330` → `...::snapshot_route_force_refresh_bypasses_the_cache` `[x]/function_exact`

`market_http_test.go:306`（`...QueriesQuoteSnapshotOnCacheMiss`）仍为 `[~]`：若
要断言“强制刷新恰好一次 GetBasicQot 调用计数”，需要把 provider 调用计数暴露给
Rust 读端口；当前 `ProductionMarketDataQuotePort` 通过 `trade_runtime` 间接持有
OpenD 读客户端，无计数 seam，属 `boundary`（计数断言只能在集成/fixture 层完成）。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`、
`python3 scripts/compatibility/audit_test_parity.py`。

## 批次 E：candle session 数据错误分类（go:452dea11）

`market_http_test.go:150 TestMarketCandlesResponseClassifiesUnknownUSSessionAsDataError`
与 `:91 TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles` 逐条对照后：

- Go `brokerKLineSession`（`internal/marketdata/broker_candles.go:120`）先调
  `market.ClassifySession`。当分类结果是 `SessionClosed`（周末/节假日/未知日程）时
  返回 `unable to classify K-line session at <RFC3339>`，路由把它当成数据错误；
  daily 与非 US 请求根本不进入该分支。
- Rust 之前只在 `futu_candle_session` 返回 `None` 时**静默省略** `session`，因此一个
  US intraday 页面可能看起来完整却缺 session 标签，且不会像 Go 那样失败。

### 修复的功能差异（P1，时区/session 边界）

| 差异 | 复现条件 | Go 预期 | Rust 修复位置 | 回归测试 |
| --- | --- | --- | --- | --- |
| 无法分类的 US intraday bar 被静默省略 session | US + intraday + 日历判定落在所有 session 之外（例：2026-05-24 周日 12:00Z） | 数据错误 `unable to classify K-line session at <ts>` | `product_production_ports_market_data_quote_reads.rs::futu_candle_session_checked` | `candle_route_classifies_unknown_us_session_as_a_data_error` |
| daily 请求不应触发分类 | `period=1d` | candle 与 meta 都不含 session，extendedHours=false | 同上（`annotate_session` 谓词） | `candle_route_skips_session_classification_for_unannotated_requests` |

未配置日历时仍保持既有 fail-closed 边界：不臆造 session，而是留空（该分支由
`annotate_session` 谓词与 `futu_securities...` 等既有测试固定）。

映射（`manual-test-mappings.json`）：

- `market_http_test.go:91` → `...::candle_route_skips_session_classification_for_unannotated_requests` `[x]/function_exact`
- `market_http_test.go:150` → `...::candle_route_classifies_unknown_us_session_as_a_data_error` `[x]/function_exact`

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`
（1125 passed）、`python3 scripts/compatibility/audit_test_parity.py`。

## 批次：market_http_test.go 剩余 18 项收口

本批把 `internal/app/apiserver/marketdataapp/market_http_test.go` 的 18 条
Go 测试全部处理完：12 条 `[x]`/`function_exact`，6 条研究块冻结为
`[~]`/boundary。此前 7 条已闭环，本批新增 5 条：

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `:18 TestMarketCandlesResponseUsesExchangeResolvedSessionsForUSIntraday` | `candle_pagination_tests::us_intraday_futu_candles_carry_calendar_resolved_session_labels` | `[x]`：无 `sessions` 参数时按 Go 的 US 日内默认 `regular+extended+overnight` 解析；逐根标注 pre/regular/after/overnight，`meta.session=all`。修复读取 owner 的默认会话集合。 |
| `:137 TestMarketCandlesResponseRejectsInvalidSessionsBeforeFutuAccess` | `candle_pagination_tests::market_http_rejects_invalid_sessions_before_provider_access` | `[x]`：`sessions=regular,unknown` 返回 400，且历史/当前 K 线调用计数均为 0。 |
| `:306 TestMarketSnapshotResponseQueriesQuoteSnapshotOnCacheMiss` | `quote_read_tests::snapshot_route_queries_the_provider_once_on_cache_miss` | `[x]`：cache miss 恰好一次 provider 快照，`fromCache=false`。 |
| `:368 TestMarketSnapshotResponseRejectsInvalidRefreshQuery` | `quote_read_tests::snapshot_route_rejects_invalid_refresh_before_provider_access` | `[x]`：`refresh=sometimes` 返回 400 且 provider 未被访问。 |
| `:478 TestMarketSecurityDetailsResponseQueriesSecuritySnapshot` | `quote_read_tests::securities_route_queries_the_security_snapshot_once` | `[x]`：恰好一次 `GetSecuritySnapshot`，name/currentPrice/equity.peRate 来自实时快照。 |

### 本批发现并修复的真实功能差异

1. **快照缓存新鲜度**
   - 复现：缓存中存在 1.5s~30s 的旧样本，请求快照。
   - 修复前：Rust 用 30s 窗口且接受 `Stale`，直接返回旧价，cache miss 不再查询 provider。
   - 预期（Go `cache.Latest(id, TickFreshness)`）：只有 1.5s 内的 `Fresh` 可命中，否则必须查询 provider。
   - 修复：`product_production_ports_market_data_quote_reads.rs` 改用共享的
     `TICK_FRESHNESS_MS` 并只接受 `CacheLookup::Fresh`。
   - 回归：`snapshot_route_queries_the_provider_once_on_cache_miss`、
     `snapshot_route_serves_a_fresh_cache_hit_without_provider_access`。
2. **securities 价格字段类型**
   - 复现：实时快照携带数字型价格，请求 `/api/v1/market-data/securities/HK/00700`。
   - 修复前：Rust 输出 JSON 数字（`321.4`），Go `decimalJSON` 输出字符串（`"321.4"`）。
   - 修复：`enrich_security_from_snapshot` 对价格/比率字段统一字符串化。
   - 回归：`securities_route_queries_the_security_snapshot_once`。
3. **US 日内默认会话集合**
   - 复现：US 日内 K 线请求不带 `sessions`。
   - 修复前：默认 `regular+extended`，缺 `overnight`，`meta.sessions` 与 Go 不一致。
   - 修复：默认解析为 `regular+extended+overnight`。
   - 回归：`us_intraday_futu_candles_carry_calendar_resolved_session_labels`。

### 冻结边界

`:533/:555/:577/:592/:607/:622` 六条研究块（warrant/option/future/trust/
index/plate）停留在 `[~]`/boundary：Go 把这些块放在完整
`pkg/futu.SecurityDetails` 模型里，Rust `/api/v1/market-data/securities`
的权威契约只有九字段 envelope 并允许 provider 追加研究字段。Rust 侧
`futu_securities_route_projects_broker_neutral_envelope_boundary` 断言这些
块不存在且不得伪造。

## 批次 F：internal/marketdata 剩余 112 条收口（go:452dea11）

批次范围：`internal/marketdata` 在清单中剩余的全部 112 条 `missing`，覆盖 17 个文件
（instrument_resolver 15、collector 13、subscription_lifecycle 12、service_facade 10、
lifecycle_boundaries 8、provider_switch_lifecycle 7、provider_switch_boundaries 6、
rankings_facade 6、cache 剩余 5、candle_sessions 5、company_research_facade 5、
calendar_macro_facade 4、news_facade 4、subscriptions 4、index_constituents_facade 3、
screen_facade 3、quote_availability 2）。

分片执行：P0 生命周期与订阅所有权 46 条（subscription_lifecycle 12、provider_switch_lifecycle 7、
provider_switch_boundaries 6、lifecycle_boundaries 8、collector 13）→ P1 数据语义与缓存 37 条
（cache 剩余 5、candle_sessions 5、quote_availability 2、instrument_resolver 15、service_facade 10）
→ P2 façade 只读投影 29 条（rankings 6、company_research 5、calendar_macro 4、news 4、
index_constituents 3、screen 3、subscriptions 4）。每片先用 Go 断言摘要逐测试比对 Rust 入口池
（`jftrade-marketdata`、`jftrade-engine` market_data 读/订阅端口、`jftrade-integration-futu`
订阅执行器、`jftrade-integration-marketdata-helper`、`jftrade-watchlist`），再写回清单。

结果：本批 112 条 = **5 `[x]`/function_exact + 104 partial + 3 boundary**，`internal/marketdata/**`
全域归零（122 条 = 5 `[x]` + 114 partial + 3 boundary，0 missing）。全局 4451 条 =
function_exact 1209 + partial 2147 + boundary 529 + module_only 4 + **missing 562**。

本批新增 `[x]` 与锚点（锚点行号一律对齐清单键，去掉历史偏行）：

| Go 测试 | Rust 入口 | 锚点动作 |
| --- | --- | --- |
| `collector_test.go:355` TestRetryDelaySequence | `jftrade-marketdata/src/runtime.rs::retry_delay_is_capped_after_four_failures` | 新增 `// Parity:`（退避 5/10/20/30/30 逐项一致） |
| `cache_test.go:210` TestTickCandlesVolumeWindowAndLimit | `jftrade-marketdata/src/tick_candles.rs::tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume` | 既有锚点行号 :207 → :210 |
| `cache_test.go:241` TestTickCandlesUsesExplicitVolumeDeltaAcrossTradingDays | `jftrade-marketdata/src/tick_candles.rs::tick_candles_use_explicit_volume_delta_across_trading_days` | 既有锚点补 `go:452dea11` 前缀 |
| `lifecycle_boundaries_test.go:95` TestNormalizeInstrumentIDRejectsIncompleteValues | `jftrade-watchlist/src/lib.rs::test_normalize_instrument_id_rejects_incomplete_values` | 既有锚点行号 :87 → :95 |
| `rankings_facade_test.go:184` TestServiceIndustriesDefaultsEmptyKindToIndustry | `jftrade-engine/src/product_production_ports_research_market_tests.rs::test_board_kind_defaults_empty_to_industry` | 既有锚点行号 :193 → :184 |

另有两条被 Go 侧同行为路径先占用唯一 Rust 证据的行，按“`[x]` rust_entry 全局唯一”的审计规则
保留 partial 并写明理由：`cache_test.go:337`（Rust 回落缓存测试已作为
`internal/app/apiserver/marketdataapp/market_http_test.go:442` 的证据）与
`candle_sessions_test.go:8`（会话解析测试已作为
`internal/app/apiserver/marketdataapp/query_test.go:45` 的证据）。

保留差异与补测候选（登记为后续批次输入）：

1. **provider 状态 streamMode**：Go 的 poll-only 健康模式（degraded → `snapshot-poll-delayed`、
   有需求 → `snapshot-poll-fallback`）在 Rust `/api/v1/market-data/provider` 对 helper provider
   固定返回 `idle`（`product_production_ports_provider.rs`），`snapshot-poll-fallback` 只出现在
   live WS heartbeat 的 `transport.mode`；涉及 `provider_switch_boundaries_test.go:81`、
   `service_facade_test.go:335`、`subscriptions_test.go:100` 三条。
2. **helper 解析器缓存与单飞**：Go `MarketSubsetInstrumentResolver` 的关键词归一缓存、TTL 失效、
   单飞内二次查缓存、错误不缓存与 Reset 代际隔离在 Rust 无同形实现（provider 搜索由 Python
   sidecar 承担）；涉及 `instrument_resolver_test.go:248/314/365/397`。
3. **引用计数/并发压力断言缺口**：64 消费者并发 acquire/heartbeat/release、managed 并发 release
   幂等、激活与关闭 exactly-one 之外的顺序竞争在 Rust 由类型与租约结构保证，缺少同形压力断言。
4. **报价可用性**：Go 用 `Availability.Authoritative` 在运行时决定零值输出 `null`；Rust 由
   `Option`/`PriceValue` 类型表达（缺失即 null、零值输出 `"0"`），无同形 wire 断言
   （`quote_availability_test.go` 两条）。
5. **不适用边界**：nil 接收者分支（`provider_switch_boundaries_test.go:37`、
   `service_facade_test.go:320` 的 nil 序列化）在 Rust 类型系统下不存在，按边界保留。
6. **CN 聚合→叶市场读路由**：snapshot/details/candles/depth/news/index 的 CN 聚合改写在 Rust
   没有逐入口断言（前缀推断有测试，路由改写无）。

验证：`cargo fmt --all -- --check`；`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-watchlist -p jftrade-engine --all-targets --locked --no-fail-fast`（**1782 passed / 0 skipped**）；`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2927 Rust** / **1209 `[x]`**；missing 562、partial 2147、boundary 529、module_only 4；0 破坏引用、0 重复 rust_entry、未锚定告警 193 = 前批基线、7 条 partial 无解析引用与 2 条无断言为前批已登记缺口）；`pnpm run check:compatibility`（EXIT=0）；`node scripts/check-zero-go.mjs`（2891 tracked files / 0 release artifact）；`pnpm run check:rust:architecture`；`git diff --check`；`pnpm run check:quick`（EXIT=0）；`pnpm run check:ai-context`。

## 第 130 批 12 切片三十九：`internal/marketdata` partial 第 1–30 条逐项复核（2026-09-25）

本批以 Go `internal/marketdata` 的清单顺序处理 30 条（broker candle 5、cache 8、
calendar/macro 4、candle session 5、collector 8）。逐一读取 Go 测试函数体和 Rust
证据函数；本批没有发现需要先写失败回归测试的新增生产行为差异。所有条目继续保持
`[~]`/`partial`，因为 Rust 证据只覆盖断言子集、测试结构不同，或其行为由类型/租约
边界表达；不因存在 `Parity` 引用而升级为 exact。

| 复核 | Go 测试（清单键） | Rust 证据 | P | 本批结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `broker_candles_test.go:12:TestBrokerKLineCandlesResponseProjectsStrictPage` | `product_market_data_candle_pagination_tests::test_broker_k_line_candles_response_projects_strict_page` | P1 | partial：页大小、分页标记和基本字段有证据；未锁定 Go 的时间、收盘值、source、extendedHours、session wire 值。 |
| [x] | `broker_candles_test.go:51:TestBrokerKLineCandlesResponseHandlesTerminalAndBoundedPages` | `...::test_broker_k_line_candles_response_handles_terminal_and_bounded_pages` | P1 | partial：Rust 实际只走 terminal 分支；Go 的 bounded 分支与 pagination 对象字段数未同形断言。 |
| [x] | `broker_candles_test.go:77:TestBrokerKLineCandlesResponseRejectsInvalidProviderRows` | `...::test_broker_k_line_candles_response_rejects_invalid_provider_rows` | P2 | partial：仅 close 正数/非法值；nil snapshot、cursor、乱序、时间、缺字段、非有限值和 session 矩阵未覆盖。 |
| [x] | `broker_candles_test.go:109:TestBrokerKLineHelpersClassifySessionsAndNumbers` | `...::test_broker_k_line_helpers_classify_sessions_and_numbers` | P1 | partial：时间/数字解析有证据；默认 regular、显式 after 及 CandleSession 分组未断言。 |
| [x] | `broker_candles_test.go:137:TestBrokerKLinePaginationRejectsInvalidBoundedAndPagedMetadata` | `...::test_broker_k_line_pagination_rejects_invalid_bounded_and_paged_metadata` | P1 | partial：bounded hasMore 与 page 超限有证据；bounded cursor、空/非法/不匹配 nextBefore 未全覆盖。 |
| [x] | `cache_test.go:12:TestCacheDeduplicatesPromotesAndInherits` | `jftrade-marketdata/tests/cache_boundaries.rs::test_cache_deduplicates_promotes_and_inherits` | P1 | partial：同价 promotion/计数有证据；observedAt、trade bid/ask/volumeDelta、close/pre-market/turnover/session 继承未等价断言。 |
| [x] | `cache_test.go:76:TestCacheFreshnessRetentionAndMaximum` | `.../cache_boundaries.rs::test_cache_freshness_retention_and_maximum` | P1 | partial：fresh/stale 有证据；retention 清理、max=3 顺序和 AllFresh 未覆盖。 |
| [x] | `cache_test.go:103:TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays` | `.../cache_extended_sessions_parity.rs::test_cache_does_not_inherit_extended_sessions_across_trading_days` | P1 | partial：交易日变化阻断 session/pre/after 继承；ExtendedHours、overnight、bid/ask/volume 未全投影。 |
| [x] | `cache_test.go:138:TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives` | `.../cache_extended_sessions_parity.rs::test_cache_promotes_us_regular_close_when_after_hours_trade_arrives` | P1 | partial：previous/last close 和价格 promotion 有证据；SnapshotJSON 及后续 after-hours 上下文保留未覆盖。 |
| [x] | `cache_test.go:185:TestCacheRetainsNewExtendedQuoteWhenPriceIsUnchanged` | `.../cache_extended_sessions_parity.rs::test_cache_retains_extended_quote_when_price_is_unchanged` | P1 | partial：同价 after quote 有证据；计数、quote 时间和 close 字段完整保留未断言。 |
| [x] | `cache_test.go:261:TestSerializationPreservesNullExtendedAndStringPrices` | `product_production_ports_market_data_quote_snapshot.rs::cached_projection_uses_active_after_quote_and_separates_closes` | P1 | partial：字符串价格、after quote、session 窗口和 observedAt 有证据；Availability.Authoritative→null 无同形 wire 断言。 |
| [x] | `cache_test.go:301:TestServiceUsesSingleCacheForSnapshotCandlesAndLatest` | snapshot/tick/security cache tests | P1 | partial：各入口分别证明 cache hit；Go 的同一 Service 串联 snapshot→tick→latest 单一缓存断言未复现。 |
| [x] | `cache_test.go:337:TestServiceTickCandleFallsBackToRetainedCache` | `product_market_data_candle_pagination_tests::tick_candles_fall_back_to_retained_cache_on_ticker_error` | P1 | partial：语义证据已被 market HTTP 行唯一占用；为保持 rust_entry 全局唯一，本行不升 exact。 |
| [x] | `calendar_macro_facade_test.go:66:TestServiceCalendarMacroRejectsProvidersWithoutCapability` | `product_production_ports_research_calendar_tests::{calendar_and_macro_routes_fail_closed_for_other_providers,calendar_and_macro_propagate_capability_and_busy_errors}` | P2 | partial：fail-closed/能力错误有证据；Go 六个操作逐项 unsupported 文本矩阵未同形覆盖。 |
| [x] | `calendar_macro_facade_test.go:109:TestServiceCalendarValidatesDateFormats` | `...::{calendar_operations_map_to_provider_reads_on_the_wire,economic_calendar_route_derives_date_and_time}` | P2 | partial：参数转发和日期派生有证据；RFC3339/日期非法输入矩阵未逐项断言。 |
| [x] | `calendar_macro_facade_test.go:144:TestServiceMacroIndicatorHistoryValidatesIDAndLimit` | `...::{macro_history_limit_is_bounded_and_defaults,macro_history_uses_page_size_before_legacy_limit,macro_indicator_history_route_rejects_identity_and_type_drift}` | P2 | partial：limit/default/pageSize 与身份漂移有证据；空 ID、超界 limit 的 Go 错误矩阵未完全对齐。 |
| [x] | `calendar_macro_facade_test.go:163:TestServiceCalendarMacroPassesProviderErrorsThrough` | `...::{calendar_and_macro_propagate_capability_and_busy_errors,macro_projection_rejects_missing_or_malformed_typed_fields}` | P2 | partial：错误分类/畸形字段有证据；Go provider 原文透传未逐操作锁定。 |
| [x] | `candle_sessions_test.go:8:TestParseCandleSessionsNormalizesCSVAndRepeatedValues` | `product_query::tests::candle_sessions_parse_dedup_order_and_reject_invalid` | P1 | partial：同一输入的去重排序有证据，但该 Rust 函数已被 query 行占用，不能重复升 exact。 |
| [x] | `candle_sessions_test.go:18:TestParseCandleSessionsRejectsEmptyAndUnknownValues` | `product_query::tests::candle_sessions_parse_dedup_order_and_reject_invalid` + assembly validation | P1 | partial：空/未知 token 及 400 映射有证据；`regular,invalid` 组合和 Go 统一错误变体未同形断言。 |
| [x] | `candle_sessions_test.go:26:TestResolveCandleSessionsDefaultsAndRejectsUnsupportedValues` | `...::us_intraday_futu_candles_carry_calendar_resolved_session_labels` + assembly validation | P1 | partial：默认会话和不支持值有证据；available 集合顺序/overnight 缺失的函数级语义未单独钉住。 |
| [x] | `candle_sessions_test.go:44:TestFilterCandlesBySessionsPreservesUnknownAsRegular` | `...::{tick_candles_filter_sessions_before_applying_the_limit,broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates}` | P1 | partial：过滤及 limit 前顺序有证据；未知标签降 regular、nil filter 和映射表未同形覆盖。 |
| [x] | `candle_sessions_test.go:71:TestNormalizeInstrumentFallsBackForUnqualifiedInput` | `jftrade-marketdata::catalog_tests::{infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes,normalize_instrument_rejects_unsupported_market_and_market_mismatch}` | P1 | partial：合法推断和 fail-closed 有证据；Go 对 `??` 仍宽松生成 `??.CODE`，属于冻结的 go-behavior quirk。 |
| [x] | `collector_test.go:15:TestCollectorCloseCancelsBlockingConnectAndPreventsRevival` | Futu stale executor/closed session + marketdata runtime recorder | P1 | partial：关闭 fence、幂等和不触 transport 有证据；阻塞 Connect 取消及 Wake 不新建 stream 的计数未同形断言。 |
| [x] | `collector_test.go:47:TestCollectorOldGenerationCannotCommitPushOrConnect` | `quote_push_tests::lifecycle_accepts_active_push_preserves_recorder_failures_and_rejects_stale_push` + snapshot generation fence | P2 | partial：旧 generation push/在途结果拒绝有证据；旧 handler tick 计数与新流 connected 结构不同。 |
| [x] | `collector_test.go:79:TestCollectorResetBoundsStreamCloseAndRejectsLateTick` | stale executor + router deactivation + `runtime::reconfigure_preserves_demand_and_clears_previous_runtime_state` | P2 | partial：generation/reset/late callback fence 有证据；Go 的 500ms 与 closeRelease 时序边界未同形验证。 |
| [x] | `collector_test.go:114:TestCollectorCloseBoundsUncooperativeConnectAndKeepsError` | `managed_session_tests::close_is_idempotent_and_joins_the_single_reader` + helper process kill escalation | P2 | partial：关闭有界/幂等有证据；首次超时错误文本保留与重复 Close 文本未锁定。 |
| [x] | `collector_test.go:142:TestCollectorPollingFallbackDoesNotCallPushHandler` | `snapshot_poll::poll_normalizes_demand_writes_only_requested_ticks_and_skips_fresh_cache` | P1 | partial：poll 只写 cache 的类型边界有证据；没有 Go 式 push handler 计数 seam。 |
| [x] | `collector_test.go:161:TestCollectorSkipsDynamicallyUnavailablePushSource` | `snapshot_poll::empty_invalid_closed_and_inactive_demand_never_calls_provider` + router provider state | P2 | partial：无需求/不健康时 fail-closed 有证据；动态 push source NewStream 调用计数未暴露。 |
| [x] | `collector_test.go:180:TestCollectorPollsFallbackInstrumentsWithoutAddingThemToPushStream` | snapshot poll demand normalization + `poll_only_read_routes_prioritize_capabilities_and_preserve_leases` | P1 | partial：需求集合与轮询写入有证据；实时/回退分流的 push stream 只含可用标的未同形断言。 |
| [x] | `collector_test.go:223:TestCollectorUsesDynamicPollingPolicyAndPreventsOverlap` | `snapshot_poll::{missing_cache_poll_respects_one_second_cadence,non_positive_policy_values_retain_collector_defaults}` | P2 | partial：cadence/default policy 有证据；Go 的阻塞期间多次 poll 计数=1 未直接验证，Rust 由串行 executor 表达。 |

本批勾选结果：30/30 已读取 Go 断言并核对 Rust 证据；精准 Rust 测试共 38 条（engine 18、
marketdata 14、integration-futu 3、marketdata-helper 1、catalog 2），全部通过。没有新增
`function_exact`，没有新增生产修复；P1 缺口保留在对应映射的 `nextAction`，后续若要升级必须
先补同形回归测试并重新核对唯一 `rust_entry`。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（本批精准 18/18）；`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata --all-targets --locked --no-fail-fast`（14/14）；`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked --no-fail-fast`（3/3）；`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper --all-targets --locked --no-fail-fast`（1/1）；`python3 scripts/compatibility/audit_test_parity.py`；`python3 scripts/compatibility/parity_anchor_reconcile.py`；`git diff --check`。

下一片：`internal/marketdata` partial 第 31–60 条（collector 余量、company/index/research façade）；若该片出现真实 P1 行为差异，先补失败回归测试再修复所属 owner。
