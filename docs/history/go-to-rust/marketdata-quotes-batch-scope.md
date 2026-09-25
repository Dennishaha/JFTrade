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

## 第 133 批：backtest provider malformed page（2026-09-25）

本批收尾 `internal/app/apiserver/backtestapp/historical_source_test.go:266:TestHistoricalPageParsingRejectsMalformedProviderValues`。旧 Rust helper 测试只构造成功页，不能证明 Go 表驱动的非法 `candles`、时间戳、OHLC、成交量与 cursor 均被拒绝；新增 `jftrade-engine::product::tests::market_data_quote_read_tests::candle_pagination_tests::helper_candle_conversion_rejects_malformed_provider_values`，直接调用生产 `convert_helper_candles_response`，并先通过 typed `HelperCandlesResponse` 拒绝非数组 envelope。

| Go 测试 | Rust 证据 | 结论 |
| --- | --- | --- |
| `historical_source_test.go:266:TestHistoricalPageParsingRejectsMalformedProviderValues` | `product_market_data_candle_pagination_tests::helper_candle_conversion_rejects_malformed_provider_values` | `function_exact`：Go 的各非法字段均有逐项失败断言；Rust 还锁定 impossible OHLC bounds。 |
| `historical_source_test.go:298:TestDecimalStringAcceptsProviderNumericRepresentations` | `product_production_ports_backtest_sync_helpers::test_historical_candle_conversion_rejects_invalid_fields_and_defaults_volume` 与 helper DTO 解析测试 | `partial`：Rust typed JSON 支持字符串/JSON 数字并拒绝非法值，但没有 Go `decimalString` 对 `decimal.Decimal`、`float32`、`int64` 等动态类型的同形逐类型入口。 |

定向验证：`helper_candle_conversion_rejects_malformed_provider_values` 1/1 通过；完整 `check:quick` 与 `check:rust` 必须在本批提交后重跑。下一批继续 `historical_source_test.go:310`，核对固定 Futu、成功后 Close 与四种构造失败，不把空页测试聚合升级为 exact。

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

## 第 130 批 12 切片四十：`internal/marketdata` partial 第 31–60 条逐项复核（2026-09-25）

本片继续按清单键逐项读取 Go 测试实现，并核对 Rust owner、接口边界和现有断言。范围固定为
collector 4 条、company research façade 5 条、index constituents façade 3 条、instrument
resolver 15 条、lifecycle boundaries 3 条，共 30 条。每行 `[x]` 只表示“已完成核对”，不表示
已经达到 `function_exact`；断言集合不等价的条目仍保留 `[~]`/`partial` 或 `boundary`。

| 复核 | Go 测试（清单键） | Rust 证据 | P | 结论与下一步 |
| :---: | --- | --- | :---: | --- |
| [x] | `collector_test.go:249:TestCollectorDemandChangeCancelsPreviousProviderPoll` | `snapshot_poll::stale_caller_is_rejected_even_when_the_active_generation_cache_is_fresh`；`completion_after_generation_change_cannot_mutate_cache_or_failure_state`；`provider_runtime_recovery::latest_demand_replaces_stale_replay_while_reconnect_is_pending` | P1 | partial：generation 变化、在途结果丢弃和最新需求回放有证据；缺 Go 式逐 instrument 取消/调用计数。 |
| [x] | `collector_test.go:279:TestCollectorResetInvalidatesBlockingQueryResult` | `runtime::reconfigure_preserves_demand_and_clears_previous_runtime_state`；`snapshot_poll::completion_after_generation_change_cannot_mutate_cache_or_failure_state` | P2 | partial：reset fence 与阻塞结果不提交有证据；cache 计数/测试构造不完全同形。 |
| [x] | `collector_test.go:300:TestCollectorStreamFailureBacksOff` | `runtime::recorder_matches_generation_retry_recovery_and_close_rules`；`runtime::retry_delay_is_capped_after_four_failures` | P2 | partial：retry_at、失败计数和恢复有证据；缺 NewStream 调用计数及错误文本保留断言。 |
| [x] | `collector_test.go:325:TestCollectorQuoteFailureBacksOff` | `runtime::recorder_matches_generation_retry_recovery_and_close_rules`；`snapshot_poll::failure_preserves_cache_and_enforces_capped_recorder_retry_window` | P2 | partial：失败窗口抑制、到期恢复和缓存保留有证据；Rust 用 outcome 表达而非 provider 调用计数。 |
| [x] | `company_research_facade_test.go:59:TestServiceCompanyResearchRejectsProvidersWithoutCapability` | `product_production_ports_research_company_tests::company_research_propagates_capability_and_lifecycle_errors`；`company_research_stays_off_the_helper_for_futu` | P2 | partial：能力/生命周期 fail-closed 有证据；未逐操作锁定 profile/analyst/ownership/financials unsupported 矩阵。 |
| [x] | `company_research_facade_test.go:94:TestServiceCompanyResearchRequiresMarketAndSymbol` | `embedded_research_instrument_derives_market_and_symbol`；`company_research_rejects_provider_identity_drift` | P2 | partial：instrument 派生与身份漂移拒绝有证据；空 market/symbol/instrument 组合未逐项对齐。 |
| [x] | `company_research_facade_test.go:109:TestServiceFinancialStatementsValidatesStatementAndDefaultsToIncome` | `company_financials_forwards_market_symbol_and_statement`；`provider_financial_statements_projection_maps_frontend_keys` | P2 | partial：默认 income、statement 转发和报表投影有证据；非法 statement 错误分支缺同形断言。 |
| [x] | `company_research_facade_test.go:143:TestServiceCompanyResearchResolvesChinaAggregateToExchangeLeaf` | `embedded_research_instrument_derives_market_and_symbol`；`company_research_rejects_provider_identity_drift` | P2 | partial：叶市场身份校验有证据；CN 聚合到 SH/SZ 的正向 provider 转发未单独覆盖。 |
| [x] | `company_research_facade_test.go:156:TestServiceCompanyResearchPassesProviderErrorsThrough` | `company_research_propagates_capability_and_lifecycle_errors`；`provider_analyst_consensus_projection_maps_frontend_keys` | P2 | partial：错误分类向上传播有证据；analyst/ownership provider 原文错误未逐操作锁定。 |
| [x] | `index_constituents_facade_test.go:31:TestServiceIndexConstituentsRejectsProvidersWithoutCapability` | `index_constituents_read_requires_akshare_and_a_ready_helper`；`index_constituents_read_rejects_non_cn_indices_without_touching_the_helper` | P2 | partial：provider/helper/市场准入有证据；未逐条复现 unsupported provider 文本。 |
| [x] | `index_constituents_facade_test.go:41:TestServiceIndexConstituentsValidatesLimitAndForwardsArguments` | `index_constituents_projection_rejects_identity_drift_and_blank_codes` | P2 | partial：身份漂移/空 code 有证据；limit 边界和 market/symbol/limit 转发仍缺同形断言。 |
| [x] | `index_constituents_facade_test.go:73:TestServiceIndexConstituentsResolvesChinaAggregateToExchangeLeaf` | `index_constituents_read_rejects_non_cn_indices_without_touching_the_helper` | P2 | partial：非 CN fail-closed 有证据；CN 聚合叶市场改写与 provider 参数转发未覆盖。 |
| [x] | `instrument_resolver_test.go:45:TestMarketSubsetInstrumentResolverKeepsQualifiedExactLookup` | `instrument_search_route_returns_subset_resolution_contract`；`qualified_lookup_encodes_exact_security_without_market_catalog_or_subscription` | P2 | partial：限定市场精确查询有证据；未锁定“不触发跨市场 search”的调用计数粒度。 |
| [x] | `instrument_resolver_test.go:83:TestMarketSubsetInstrumentResolverMarksUnsupportedQualifiedMarketUnavailable` | `futu_search_distinguishes_no_match_unsupported_market_and_runtime_failure`；`search_response_maps_chinese_stock_and_preserves_unavailable_markets` | P2 | partial：三类结果及不可选候选有证据；Go 的 resolvedMarket/unavailable envelope 不完全同形。 |
| [x] | `instrument_resolver_test.go:101:TestMarketSubsetInstrumentResolverSearchesNamesAndPreservesRelevance` | `instrument_search_route_returns_subset_resolution_contract`；`search_request_preserves_chinese_name_and_requests_full_candidate_window` | P2 | partial：中文名、完整候选窗口和保序有证据；相关度排序及 disabled 候选未同形锁定。 |
| [x] | `instrument_resolver_test.go:136:TestMarketSubsetInstrumentResolverExactCodeWinsAndCrossMarketCodeStaysAmbiguous` | `instrument_search_route_returns_subset_resolution_contract`；`futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity` | P2 | partial：跨市场多候选保持 ambiguous 有证据；精确优先排序分支未单独断言。 |
| [x] | `instrument_resolver_test.go:158:TestMarketSubsetInstrumentResolverFiltersCNAndDeduplicates` | `instrument_search_route_returns_subset_resolution_contract`；`futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity` | P2 | partial：CN 叶市场过滤、去重和 full-window 有证据；具体候选顺序仍较粗。 |
| [x] | `instrument_resolver_test.go:183:TestMarketSubsetInstrumentResolverNormalizesProviderPrefixedCodes` | `futu_search_canonicalizes_open_d_prefixed_symbols_like_go`；`search_symbols_normalize_open_d_prefixed_codes_like_go` | P2 | partial：OpenD/市场前缀规范化有证据；缺 Resolve 返回 instrumentId 的单入口断言。 |
| [x] | `instrument_resolver_test.go:211:TestMarketSubsetInstrumentResolverUnavailableWhenAllMatchesAreUnsupported` | `instrument_search_route_returns_subset_resolution_contract`；`futu_search_distinguishes_no_match_unsupported_market_and_runtime_failure` | P2 | partial：不支持候选保留且不可选有证据；unavailable 枚举值仅由路由间接覆盖。 |
| [x] | `instrument_resolver_test.go:229:TestMarketSubsetInstrumentResolverLimitsAfterRankingWithoutAutoResolvingHiddenMatches` | `futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity`；`instrument_search_route_returns_subset_resolution_contract`；`instrument_search_requests_full_provider_window_before_applying_limit` | P1 | partial：本片先红后修，锁定 provider 先取最大 100 条再过滤/limit，修复 limit=1 过早截断造成的假 resolved；仍缺隐藏候选不自动解析和 TTL/singleflight。 |
| [x] | `instrument_resolver_test.go:248:TestMarketSubsetInstrumentResolverCachesAndCoalescesNormalizedKeyword` | 无同形 Rust resolver cache/singleflight（sidecar 负责搜索） | P1 | boundary：保留 Go 进程内 TTL、singleflight 及并发调用计数差异；不得把 sidecar 能力当 Rust resolver 等价。 |
| [x] | `instrument_resolver_test.go:314:TestMarketSubsetInstrumentResolverResetSeparatesProviderGenerations` | `qualified_lookup_encodes_exact_security_without_market_catalog_or_subscription`；`router::deactivation_fences_cache_and_marks_router_inactive` | P2 | partial：provider generation/失活 fence 有证据；Rust 无跨请求 resolver cache，缺 Reset 前后调用计数矩阵。 |
| [x] | `instrument_resolver_test.go:365:TestMarketSubsetInstrumentResolverRechecksCacheInsideSingleflightWork` | 无同形 Rust singleflight cache | P1 | boundary：Go 的 singleflight 内二次 cache 检查在 Rust 搜索路径不存在；登记为架构缺口而非通过。 |
| [x] | `instrument_resolver_test.go:397:TestMarketSubsetInstrumentResolverDoesNotCacheSearchErrors` | `search_failures_and_malformed_responses_are_not_empty_successes`；`instrument_search_route_validates_input_and_maps_provider_failures` | P1 | partial：错误不静默且向上暴露有证据；因无缓存层，未覆盖“重复错误仍调用 provider”。 |
| [x] | `instrument_resolver_test.go:417:TestMarketSubsetInstrumentResolverValidatesInput` | `instrument_search_route_validates_input_and_maps_provider_failures`；`futu_search_rejects_invalid_queries_before_reaching_opend` | P2 | partial：非法输入在 provider 前拒绝并映射失败有证据；Go 参数矩阵分散在两处，未单表同形。 |
| [x] | `instrument_resolver_test.go:437:TestMarketSubsetInstrumentResolverPropagatesContextCancellation` | `snapshot_read_fences_provider_generation_switch_during_helper_query`；`search_failures_and_malformed_responses_are_not_empty_successes` | P1 | partial：generation 失效/在途拒绝有证据；无搜索专用 context.Canceled 与缓存不污染断言。 |
| [x] | `instrument_resolver_test.go:466:TestClassifyInstrumentResolutionKeepsMultiplePartialCandidatesAmbiguous` | `instrument_search_route_returns_subset_resolution_contract`；`futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity` | P2 | partial：多候选不自动解析有路由证据；缺纯函数级 classify 的单候选/多候选表。 |
| [x] | `lifecycle_boundaries_test.go:13:TestCacheRemainingLifecycleBoundaries` | `cache_rejects_stale_generation_and_classifies_freshness`；`cache_rejects_empty_generation_and_backwards_timestamp_inputs`；`trade_volume::a_new_trading_day_or_session_resets_the_cumulative_baseline` | P1 | partial：generation、时间戳、stale 和交易日基线有证据；Latest(maxAge=0)、Clear 保留 provider 样本、0 时间戳等未同形。 |
| [x] | `lifecycle_boundaries_test.go:60:TestSubscriptionRegistryRemainingLifecycleBoundaries` | `demand_is_deduplicated_and_managed_leases_do_not_expire`；`partial_release_and_clear_operations`；`active_instruments_are_normalized_unioned_replaced_and_released` | P2 | partial：去重、规范化、过期 web 条目有证据；TTL=0 禁用过期和 blank consumer 分支表达不同。 |
| [x] | `lifecycle_boundaries_test.go:101:TestServiceRemainingLifecycleBoundaries` | `market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable`；`snapshot_rejects_malformed_refresh_before_provider_access`；`empty_invalid_closed_and_inactive_demand_never_calls_provider` | P2 | partial：取消/畸形输入前置失败和无需求不访问 provider 有证据；Go 的 nil Resolve/reconcile、degraded/canceled、默认周期聚合矩阵未同形。 |

本片勾选结果：30/30 已完成 Go 断言→Rust 证据核对；精准测试覆盖本片引用的现有 Rust 用例以及新增
`instrument_search_requests_full_provider_window_before_applying_limit`，均通过。新增用例暴露并修复了
一个真实 P1 差异：Rust 不再把公开 `limit` 直接传给 OpenD，而是先请求最大候选窗口，再在 market
过滤、去重和排序后应用公开 limit。其余 TTL/singleflight、调用计数和 façade 聚合差异继续保持
`partial`/`boundary`，没有为了提高 exact 数量而升格。

验证记录：`cargo fmt --all -- --check`；`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-marketdata -p jftrade-integration-futu -p jftrade-integration-marketdata-helper --all-targets --locked`（2556 passed，1 skipped）；审计与锚点复核须在 mapping 校验通过后执行。

## 第 130 批 12 切片四十一：`internal/marketdata` partial 第 61–90 条逐项复核（2026-09-25）

本片范围固定为 `subscription_lifecycle_test.go` 12 条、`provider_switch_lifecycle_test.go` 7 条、
`provider_switch_boundaries_test.go` 6 条，以及 `service_facade_test.go` 前 5 条，共 30 条。先读取
Go 的并发、lease、回滚和 provider 切换断言，再核对 Rust 的 DemandBook、ProviderRouter、
ActiveProviderState、quote read port 与 subscription owner；引用聚合测试只记为 `partial`，不自动升格。

| 复核 | Go 测试（清单键） | Rust 证据 | P | 结论与下一步 |
| :---: | --- | --- | :---: | --- |
| [x] | `subscription_lifecycle_test.go:51:TestSubscriptionRegistryExpiresOnlyWebConsumersAndPreservesManagedLeases` | `demand::demand_is_deduplicated_and_managed_leases_do_not_expire`；`partial_release_and_clear_operations`；`heartbeat_updates_consumer_and_entry_timestamps` | P2 | partial：managed lease 不过期、web heartbeat/clear 有证据；Go 的 TTL 具体时序和最终物理回收未同形。 |
| [x] | `subscription_lifecycle_test.go:83:TestSubscriptionRegistryConcurrentAcquireHeartbeatAndRelease` | `demand::demand_is_deduplicated_and_managed_leases_do_not_expire`；`subscriptions_tests::concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release` | P2 | partial：去重和 reconcile 幂等有证据；64 consumer 并发最终快照的压力矩阵未同形。 |
| [x] | `subscription_lifecycle_test.go:128:TestManagedSubscriptionConcurrentReleaseIsIdempotent` | `subscription_executor::order_book_lifecycle_leases_hk_detail_and_releases_idempotently`；`prediction_subscription_uses_reference_counted_leases` | P2 | partial：引用计数和幂等 release 有证据；Go 的 64 线程 exactly-once 回调计数未直接锁定。 |
| [x] | `subscription_lifecycle_test.go:149:TestServiceReconcilesWebAndManagedSubscriptionLifecycles` | `broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease`；`clear_route_preserves_running_strategy_lease`；`subscriptions_tests::exact_physical_subscriptions_are_shared_and_final_release_is_deferred` | P1 | partial：web/managed owner 分离、clear 保留和最终 release 有证据；Go 的 decorated snapshot 字段与 reconcile 长度序列未全同形。 |
| [x] | `subscription_lifecycle_test.go:199:TestServiceRollsBackFailedAcquireAndHandlesDeferredReleaseFailures` | `provider_runtime::provider_configuration_rolls_back_activation_when_demand_validation_fails`；`product_active_provider_state::failed_provider_change_restores_the_previous_subscription_owner`；`subscriptions_tests::unsubscribe_retry_ladder_escalates_and_reacquire_clears_retry_state` | P2 | partial：失败回滚、旧 owner 恢复和 unsubscribe retry 有证据；Go 的 heartbeat/clear 诊断错误序列未逐项覆盖。 |
| [x] | `subscription_lifecycle_test.go:237:TestSubscriptionValidationAndSnapshotDecorationBoundaries` | `demand::channel_and_interval_validation_rules`；`active_instruments_are_normalized_unioned_replaced_and_released`；`subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear` | P2 | partial：channel/interval 校验与 route envelope 有证据；nil lease、装饰字段和全部非法组合未同形。 |
| [x] | `subscription_lifecycle_test.go:303:TestFailedAcquireRestoresOnlyTheAttemptedConsumerState` | `demand::partial_release_and_clear_operations`；`product_active_provider_state::failed_provider_change_restores_the_previous_subscription_owner` | P2 | partial：失败扩展不破坏既有 owner 有证据；同一 consumer 的 retained/new refs 两次失败矩阵未逐项复现。 |
| [x] | `subscription_lifecycle_test.go:338:TestManagedLeaseReleaseRestoresPreexistingConsumerOwnership` | `consumer_only_release_keeps_other_consumer_entry`；`prediction_subscription_uses_reference_counted_leases` | P2 | partial：managed release 恢复既有 consumer 有证据；Go 的 shared instrument 与 managed-only instrument 快照未同形。 |
| [x] | `subscription_lifecycle_test.go:357:TestSubscriptionRequiredErrorsAndManagedReadDemandBoundaries` | `subscription_required_error_formats_channel_instrument_and_interval`；`live_read_routes_require_a_logical_subscription_lease`；`candle_read_without_a_kline_lease_fails_before_realtime_provider_access` | P2 | partial：租约门禁、规范化错误和读路径先验失败有证据；nil receiver、全部错误文本变体及 poll-only 组合未全覆盖。 |
| [x] | `subscription_lifecycle_test.go:446:TestPollOnlyProviderPreservesLogicalLeaseAndRejectsUnsupportedReadsFirst` | `poll_only_read_routes_prioritize_capabilities_and_preserve_leases` | P2 | partial：poll-only 能力优先和 logical lease 保留有证据；Go 对 snapshot/tick/depth 三路具体错误顺序未同形。 |
| [x] | `subscription_lifecycle_test.go:469:TestServiceMergesAdditionalDemandIntoExactReconciliation` | `subscriptions_tests::concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release`；`subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear` | P1 | partial：额外 demand 合并与 exact reconcile 有证据；DemandSource 的非法标的过滤和最终 refs 顺序未直接断言。 |
| [x] | `subscription_lifecycle_test.go:498:TestServiceCloseLeavesPhysicalSubscriptionsEmpty` | `router::deactivation_fences_cache_and_marks_router_inactive`；`provider_runtime::release_and_deactivate_clears_bridge_owned_router_state`；`subscriptions_tests::lifecycle_rejects_stale_callbacks_and_closes_recorder_once` | P1 | partial：close 清理、旧 callback fence 和 recorder 关闭有证据；Go 的 close 后 Wake 不再 reconcile 的时序计数未同形。 |
| [x] | `provider_switch_lifecycle_test.go:63:TestProviderChangeAndManagedLeaseAreMutuallyExclusive` | `router::managed_demand_blocks_provider_switch`；`provider_activation_recovery::managed_streaming_demand_blocks_provider_switch_until_released` | P2 | partial：managed lease 阻断切换、release 后可切换有证据；Go 的 push capability 变化和 poll-only lease 错误未同形。 |
| [x] | `provider_switch_lifecycle_test.go:98:TestConcurrentManagedLeaseWinsBeforeProviderChange` | `product_active_provider_state::concurrent_close_and_activation_commit_exactly_one_outcome`；`provider_activation_recovery::managed_streaming_demand_blocks_provider_switch_until_released` | P2 | partial：切换/lease gate 竞争有证据；Go 的 blocking reconciler 释放顺序和具体错误返回未直接复现。 |
| [x] | `provider_switch_lifecycle_test.go:143:TestConcurrentProviderChangeWinsBeforeManagedLease` | `product_active_provider_state::concurrent_close_and_activation_commit_exactly_one_outcome`；`provider_activation_recovery::provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update` | P2 | partial：provider transition 串行化及失败恢复有证据；旧 Go callback gate 的等待窗口未同形。 |
| [x] | `provider_switch_lifecycle_test.go:184:TestProviderChangeInvalidatesCacheBeforeUnblockingReaders` | `router::explicit_activation_fails_closed_and_switch_clears_cache`；`deactivation_fences_cache_and_marks_router_inactive`；`provider_activation_recovery::provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update` | P1 | partial：切换清 cache、generation fence 和失败保持旧状态有证据；阻塞 stream Close release 与 callback 执行顺序未同形。 |
| [x] | `provider_switch_lifecycle_test.go:224:TestProviderChangeBlocksReadsDuringActivation` | `snapshot_read_fences_provider_generation_switch_during_helper_query`；`snapshot_poll::stale_caller_is_rejected_even_when_the_active_generation_cache_is_fresh` | P2 | partial：activation barrier 与 stale read 拒绝有证据；Go 的 ChangeProvider 阻塞期间 provider read 未启动计数未直接锁定。 |
| [x] | `provider_switch_lifecycle_test.go:268:TestProviderChangeWaitsForInflightReadThenClearsItsCache` | `snapshot_read_fences_provider_generation_switch_during_helper_query`；`snapshot_poll::completion_after_generation_change_cannot_mutate_cache_or_failure_state` | P1 | partial：在途读取完成后清理旧 cache 有证据；Go 的 changeStarted 等待和具体 cache count 时序未同形。 |
| [x] | `provider_switch_lifecycle_test.go:312:TestFailedProviderChangeKeepsOldCollectorAndCache` | `router::disconnected_provider_reports_its_reason_and_keeps_the_previous_selection`；`unknown_provider_activation_is_rejected_without_touching_the_active_selection`；`provider_activation_recovery::provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update` | P1 | partial：失败切换保留旧 owner/cache 有证据；旧 collector 未关闭的 blocking stream 计数未同形。 |
| [x] | `provider_switch_boundaries_test.go:37:TestProviderSwitchHelpersHandleNilAndResetBoundaries` | 不适用：Rust 非空类型没有 Go nil receiver 的 `Service`/resolver 分支 | P2 | boundary：保留 nil receiver 不可表达边界；其余 reset/失活由 typed owner fence 覆盖，不伪造同形测试。 |
| [x] | `provider_switch_boundaries_test.go:55:TestProviderSwitchRetainsOnlyCurrentGenerationTickCandles` | `snapshot_poll::completion_after_generation_change_cannot_mutate_cache_or_failure_state`；`snapshot_read_fences_provider_generation_switch_during_helper_query`；`basic_quote_query_feeds_snapshot_poll_cache_with_generation_fencing` | P2 | partial：旧 generation ticker 不入 cache 有证据；Go 的 TickCandles 错误类型和 cache count 组合未同形。 |
| [x] | `provider_switch_boundaries_test.go:81:TestPollOnlyProviderHealthUsesPollingModes` | `product_production_ports_provider::provider_status_projects_helper_polling_modes_and_demand_count`；`heartbeat_reports_live_provider_connectivity_without_a_fixture_projection`；`market_data_runtime_projection_matches_go_status_corpus` | P2 | partial：先红后修，provider route 现输出 degraded 的 `snapshot-poll-delayed`、有 demand 的 `snapshot-poll-fallback` 并投影 activeCount；helper health 原文错误透传和完整 readiness 矩阵仍缺。 |
| [x] | `provider_switch_boundaries_test.go:106:TestPollOnlyProviderReadsDoNotRequireLogicalLease` | `poll_only_read_routes_prioritize_capabilities_and_preserve_leases` | P2 | partial：poll-only snapshot 无 logical lease 可读有证据；Go 只验证 snapshot 的具体 provider stub 路径，其他 read 能力矩阵未同形。 |
| [x] | `provider_switch_boundaries_test.go:123:TestProviderSwitchCacheUtilitiesCoverConcreteValuesAndCNRejection` | `tick_candles::tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume`；`catalog_tests::infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes`；`tick_candles_use_fresh_cache_without_querying_the_provider` | P1 | partial：Decimal equality、CN reject 和 tick cache 投影有证据；Go helper 组合在 Rust 分散到三个 owner 测试。 |
| [x] | `provider_switch_boundaries_test.go:141:TestCollectorCloseHelpersRejectElapsedDeadlines` | `router::deactivation_fences_cache_and_marks_router_inactive`；`managed_session_tests::close_is_idempotent_and_joins_the_single_reader` | P2 | partial：关闭 fence 与 session join 有证据；closeStreamUntil/waitGroupUntil 的 elapsed deadline helper 无同形公开 owner。 |
| [x] | `service_facade_test.go:11:TestServiceDelegatesProviderFacadeAndRefreshBoundaries` | `snapshot_route_force_refresh_bypasses_the_cache`；`snapshot_route_queries_the_provider_once_on_cache_miss`；`instrument_search_route_returns_subset_resolution_contract` | P2 | partial：snapshot refresh/cache miss、catalog route 有证据；Go markets/details/candles/depth/normalize 的同一 facade 串联转发未一条测试覆盖。 |
| [x] | `service_facade_test.go:69:TestServiceSnapshotErrorsAreBusinessVisible` | `market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable`；`snapshot_rejects_malformed_refresh_before_provider_access` | P2 | partial：provider 错误与无 snapshot fail-closed 有证据；Go 原始错误文本和缺失 snapshot 文案未同形。 |
| [x] | `service_facade_test.go:83:TestServiceSnapshotResolvesChinaAggregateToExchangeLeaf` | `workspace_reads_resolve_cn_aggregate_to_exchange_leaf`；`normalize_instrument_handles_cn_market_and_prefixes` | P2 | partial：CN 聚合到 SH/SZ 叶市场的 snapshot 读有证据；Go provider snapshotID/request 全字段转发未逐字段复现。 |
| [x] | `service_facade_test.go:105:TestServiceProviderReadsResolveChinaAggregateToExchangeLeaf` | `infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes`；`workspace_reads_resolve_cn_aggregate_to_exchange_leaf` | P2 | partial：details/candles/depth 叶市场归一由 owner 测试覆盖；缺同一 facade 对三个 provider 调用的连续参数断言。 |
| [x] | `service_facade_test.go:136:TestServiceRejectsSnapshotCompletedAfterProviderChange` | `snapshot_read_fences_provider_generation_switch_during_helper_query`；`snapshot_poll::completion_after_generation_change_cannot_mutate_cache_or_failure_state` | P2 | partial：provider generation fence 与旧结果不写 cache 有证据；Go 的 ErrProviderChanged 类型和 blocking read 时序未完全同形。 |

本片勾选结果：30/30 已完成 Go 断言→Rust 证据核对；新增 helper provider 状态回归测试 1 条，先红后修，
修复 `/api/v1/market-data/provider` 在 helper provider 上固定返回 `idle` 且 `activeCount=0` 的差异。
其余并发压力、调用计数、nil receiver、错误原文和 facade 聚合均按证据不足保持 `partial`/`boundary`，
没有将“存在引用”误判为 `function_exact`。

验证记录：`cargo fmt --all -- --check`；精准回归
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(provider_status_projects_helper_polling_modes_and_demand_count)'`
（先红后修，修复后 1/1 通过）；其余片内证据待统一 nextest 批次复跑后落账；审计与锚点复核须在 mapping
校验通过后执行。

## 第 130 批 12 切片四十二：marketdata façade 剩余 34 条逐项复核（2026-09-25）

本片覆盖 service façade 余量 10 条、news 4 条、rankings 5 条、screen 3 条、calendar/macro 4 条、company research 5 条、index constituents 3 条，共 34 条。读取了冻结 Go 测试实现及 Rust owner 测试；Rust 证据均已实际运行，断言粒度或 provider seam 不同的条目保持 `[~]`/`partial`，未把聚合测试冒充 `function_exact`。

| 复核 | Go 测试（清单键） | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/marketdata/calendar_macro_facade_test.go:66:TestServiceCalendarMacroRejectsProvidersWithoutCapability` | `calendar_and_macro_propagate_capability_and_busy_errors`；`calendar_and_macro_routes_fail_closed_for_other_providers` | P2 | partial：Rust 覆盖日历/宏观操作在 provider 缺能力或非归属 provider 时 fail-closed（busy/capability 错误分类）；差异：Go 逐操作断言 unsupported，Rust 以路由 fail-closed 与会话/能力错误断言。 |
| [x] | `internal/marketdata/calendar_macro_facade_test.go:109:TestServiceCalendarValidatesDateFormats` | `calendar_operations_map_to_provider_reads_on_the_wire`；`economic_calendar_route_derives_date_and_time` | P2 | partial：Rust 覆盖日历操作参数→provider 读取映射与 economic 日历日期/时间派生；差异：Go 断言非法日期格式（RFC3339/日期）报错矩阵，Rust 的输入校验在路由投影中，未逐格式断言。 |
| [x] | `internal/marketdata/calendar_macro_facade_test.go:144:TestServiceMacroIndicatorHistoryValidatesIDAndLimit` | `macro_history_limit_is_bounded_and_defaults`；`macro_history_uses_page_size_before_legacy_limit` | P2 | partial：Rust 覆盖宏观指标历史 limit 的边界/默认与 pageSize 优先于 legacy limit；差异：Go 断言空 indicator id 与超界 limit 的错误，Rust 的指标身份/类型漂移拒绝在 macro_indicator_history_route_rejects_identity_and_type_drift 中，未逐项对齐。 |
| [x] | `internal/marketdata/calendar_macro_facade_test.go:163:TestServiceCalendarMacroPassesProviderErrorsThrough` | `calendar_and_macro_propagate_capability_and_busy_errors`；`macro_projection_rejects_missing_or_malformed_typed_fields` | P2 | partial：Rust 断言 provider 能力/忙错误与畸形字段错误向上传播（不伪造投影）；差异：Go 断言 provider 错误文本原样返回，Rust 以错误分类断言。 |
| [x] | `internal/marketdata/company_research_facade_test.go:59:TestServiceCompanyResearchRejectsProvidersWithoutCapability` | `company_research_propagates_capability_and_lifecycle_errors`；`company_research_stays_off_the_helper_for_futu` | P2 | partial：Rust 覆盖公司研究操作按 provider 能力失败与 Futu 不走 helper；差异：Go 逐操作（profile/analyst/ownership/financials）断言 unsupported，Rust 以能力/生命周期错误分类断言，未逐操作断言。 |
| [x] | `internal/marketdata/company_research_facade_test.go:94:TestServiceCompanyResearchRequiresMarketAndSymbol` | `embedded_research_instrument_derives_market_and_symbol`；`company_research_rejects_provider_identity_drift` | P2 | partial：Rust 覆盖缺少 market/symbol 时从 instrumentId 派生或拒绝，并对 provider 身份漂移 fail-closed；差异：Go 断言空 market/空 symbol/空 instrument 三种输入均报错，Rust 的输入矩阵在路由校验中，未逐项断言空值组合。 |
| [x] | `internal/marketdata/company_research_facade_test.go:109:TestServiceFinancialStatementsValidatesStatementAndDefaultsToIncome` | `company_financials_forwards_market_symbol_and_statement`；`provider_financial_statements_projection_maps_frontend_keys` | P2 | partial：Rust 覆盖 statement 参数转发（含默认 income）与三张报表投影；差异：Go 断言非法 statement 报错且 balance 请求转发 statementKind="balance"，Rust 断言转发与投影但未断言非法 statement 错误分支。 |
| [x] | `internal/marketdata/company_research_facade_test.go:143:TestServiceCompanyResearchResolvesChinaAggregateToExchangeLeaf` | `embedded_research_instrument_derives_market_and_symbol`；`company_research_rejects_provider_identity_drift` | P2 | partial：Rust 覆盖 instrumentId→market/symbol 派生与叶市场身份校验；差异：Go 断言 GetCompanyProfile("CN","600519") 转发到 SH.600519 叶市场，Rust 无 CN 聚合入参的专门断言。 |
| [x] | `internal/marketdata/company_research_facade_test.go:156:TestServiceCompanyResearchPassesProviderErrorsThrough` | `company_research_propagates_capability_and_lifecycle_errors`；`provider_analyst_consensus_projection_maps_frontend_keys` | P2 | partial：Rust 断言能力/生命周期错误向上传播（不吞错、不伪造数据）；差异：Go 断言 analyst 与 ownership 两个具体 provider 错误原样透传，Rust 的错误透传为分类断言，未逐操作对齐错误文本。 |
| [x] | `internal/marketdata/index_constituents_facade_test.go:31:TestServiceIndexConstituentsRejectsProvidersWithoutCapability` | `index_constituents_read_requires_akshare_and_a_ready_helper`；`index_constituents_read_rejects_non_cn_indices_without_touching_the_helper` | P2 | partial：Rust 断言指数成分读取要求 akshare provider 且 helper ready、非 CN 指数在触达 helper 前拒绝；差异：Go 断言缺能力 provider 返回 unsupported，Rust 以 provider/helper/市场三重准入断言，覆盖等价但不逐条同形。 |
| [x] | `internal/marketdata/index_constituents_facade_test.go:41:TestServiceIndexConstituentsValidatesLimitAndForwardsArguments` | `index_constituents_projection_rejects_identity_drift_and_blank_codes` | P2 | partial：Rust 断言成分投影拒绝身份漂移与空代码；差异：Go 断言 limit 边界与 market/symbol/limit 参数转发，Rust 的 limit 校验在路由请求契约中，无逐边界的转发断言。 |
| [x] | `internal/marketdata/index_constituents_facade_test.go:73:TestServiceIndexConstituentsResolvesChinaAggregateToExchangeLeaf` | `index_constituents_read_rejects_non_cn_indices_without_touching_the_helper` | P2 | partial：Rust 断言非 CN 指数拒绝且不触达 helper；差异：Go 断言 CN 聚合输入改写为叶市场并把 market/symbol/limit 转发给 provider，Rust 无 CN 聚合改写的正向断言。 |
| [x] | `internal/marketdata/news_facade_test.go:42:TestServiceNewsAndCorporateActionsRejectProvidersWithoutCapability` | `futu_news_queries_stay_on_the_broker_path`；`explicit_broker_that_is_not_the_active_provider_is_rejected_without_fallback` | P2 | partial：Rust 覆盖新闻查询的 provider 归属（Futu 走 broker 路径、显式非活跃 provider 拒绝且不回落）；差异：Go 断言 GetNews/GetCorporateActions 对缺能力 provider 返回 unsupported，Rust 对应能力错误分类在新闻/公司行动路由测试中，未逐条断言。 |
| [x] | `internal/marketdata/news_facade_test.go:57:TestServiceNewsValidatesLimitAndForwardsNormalizedArguments` | `market_data_news_search_read_route_matches_group_fixture_in_cutover_only`；`market_data_news_actions_read_routes_match_group_fixture_in_cutover_only` | P2 | partial：Rust 以 wire fixture 锁定新闻搜索/公司行动路由的参数与响应契约（limit 归一在路由请求契约中）；差异：Go 断言 limit 边界（负值/超界）报错与规范化参数转发，Rust 无逐 limit 边界的错误断言。 |
| [x] | `internal/marketdata/news_facade_test.go:84:TestServiceNewsResolvesChinaAggregateToExchangeLeaf` | `infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes`；`market_data_news_search_read_route_matches_group_fixture_in_cutover_only` | P2 | partial：Rust 覆盖 CN 前缀推断与会生成叶市场 instrumentId；差异：Go 断言 GetNews("CN","600519") 转发到叶市场（newsMarket/newsSymbol），Rust 新闻路由的 CN 聚合改写没有专门断言。 |
| [x] | `internal/marketdata/news_facade_test.go:97:TestServiceCorporateActionsValidatesRangeAndForwardsArguments` | `corporate_actions_query_requires_rfc3339_and_ascending_range`；`corporate_actions_projection_rejects_missing_events` | P2 | partial：Rust 断言公司行动查询必须 RFC3339 且区间升序（反转区间拒绝）与缺失事件拒绝；差异：Go 断言 provider 收到 actions 参数与错误透传，Rust 侧参数转发由投影测试覆盖，无单条串联断言。 |
| [x] | `internal/marketdata/rankings_facade_test.go:54:TestServiceRankingsRejectsProvidersWithoutCapability` | `rankings_propagate_capability_and_lifecycle_errors`；`market_research_stays_off_the_helper_for_futu` | P2 | partial：Rust 覆盖 rankings/industry 操作在 provider 缺能力时按 capability 错误失败（不发 helper 调用）、Futu 走自有目录；差异：Go 断言 GetRankings/GetIndustries/GetIndustryMembers 三条各自返回 unsupported，Rust 以能力错误分类与路由投影断言，未逐条区分三个入口。 |
| [x] | `internal/marketdata/rankings_facade_test.go:72:TestServiceRankingsValidatesKindAndLimit` | `rankings_limit_prefers_positive_page_size`；`rankings_limit_clamps_to_provider_bounds`；`industry_rejects_unsupported_operations_and_plate_types` | P2 | partial：Rust 覆盖 limit 归一（pageSize 优先、上限钳制、legacy limit 回退）与不支持的 kind/plate 类型拒绝；差异：Go 断言非法 kind 与超界 limit 的 error 值，Rust 断言投影函数返回值/错误分支，粒度不同。 |
| [x] | `internal/marketdata/rankings_facade_test.go:91:TestServiceRankingsForwardsNormalizedKindAndDefaultLimit` | `rankings_operations_map_to_provider_kinds_on_the_wire`；`rankings_limit_falls_back_to_legacy_limit_and_default` | P2 | partial：Rust 覆盖 kind 归一为 provider 查询类别（gainers/losers 等 wire 映射）与默认 limit；差异：Go 断言 provider 收到的 kind/limit 转发值（含默认 20 一类）与错误透传，Rust 分别断言路由投影与 provider 参数，未在同一用例串联。 |
| [x] | `internal/marketdata/rankings_facade_test.go:121:TestServiceIndustriesRejectsNonCNMarkets` | `market_research_stays_off_the_helper_for_futu`；`industry_plate_members_read_the_board_from_the_instrument_id` | P2 | partial：Rust 对行业板块读实施 CN-only 与 provider 归属校验（非 CN/非 akshare 不落到 helper）；差异：Go 断言 GetIndustries/GetIndustryMembers 对非 CN 市场报错且不触达 provider（boardsKind 未变），Rust 无逐市场参数化断言。 |
| [x] | `internal/marketdata/rankings_facade_test.go:139:TestServiceIndustriesForwardsKindAndMembersArguments` | `industry_board_operations_map_to_provider_kinds_on_the_wire`；`provider_industry_members_projection_uses_ranking_keys` | P2 | partial：Rust 覆盖行业板块 kind→provider 查询映射与成员投影键；差异：Go 断言 GetIndustryMembers 转发 market/kind/limit 参数与空 board/invalid kind 错误，Rust 参数化断言分散在投影测试中。 |
| [x] | `internal/marketdata/screen_facade_test.go:23:TestServiceScreenRejectsProvidersWithoutCapability` | `embedded_screen_maps_capability_and_lifecycle_errors`；`embedded_screen_rejects_a_futu_catalog_definition` | P2 | partial：Rust 断言屏幕能力/生命周期错误映射与 Futu 目录定义拒绝；差异：Go 断言缺能力 provider 直接 unsupported，Rust 的能力错误分类覆盖等价场景但用例构造不同。 |
| [x] | `internal/marketdata/screen_facade_test.go:31:TestServiceScreenValidatesRequestAndForwards` | `embedded_screen_rejects_shapes_the_helper_cannot_execute`；`embedded_screen_accepts_a_map_definition_and_defaults_paging`；`embedded_screen_projects_rows_and_forwards_the_definition` | P2 | partial：Rust 覆盖可执行形状校验（无法执行的形状拒绝）、map 定义与分页默认、行投影与定义转发；差异：Go 逐项断言空 market、无量程条件、abs 排序方向、负 offset、超界 limit 的错误，Rust 的形状校验粒度不同，无逐项错误矩阵。 |
| [x] | `internal/marketdata/screen_facade_test.go:78:TestServiceScreenPassesProviderErrorsThrough` | `embedded_screen_maps_capability_and_lifecycle_errors`；`embedded_screen_serves_a_us_screen_via_akshare` | P2 | partial：Rust 断言 provider 能力/生命周期错误映射（透传错误类别）与 US 屏幕经 akshare 服务的正向路径；差异：Go 断言 provider 错误文本原样返回，Rust 以错误类别断言。 |
| [x] | `internal/marketdata/service_facade_test.go:11:TestServiceDelegatesProviderFacadeAndRefreshBoundaries` | `snapshot_route_force_refresh_bypasses_the_cache`；`snapshot_route_queries_the_provider_once_on_cache_miss`；`instrument_search_route_returns_subset_resolution_contract` | P2 | partial：Rust 覆盖 refresh=true 绕过缓存、缓存未命中只查一次、市场/证券/搜索等读路由的参数委派；差异：Go 单条测试串联 GetMarkets/GetSecurityDetails/GetCandles/GetDepth/NormalizeInstrument/GetSnapshot(refresh/cached) 并断言 provider 收到参数，Rust 拆成多条路由测试，没有同一条聚合断言。 |
| [x] | `internal/marketdata/service_facade_test.go:69:TestServiceSnapshotErrorsAreBusinessVisible` | `market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable`；`snapshot_rejects_malformed_refresh_before_provider_access` | P2 | partial：Rust 断言 provider 不可用时读路由 fail-closed 返回业务错误；差异：Go 断言 provider 错误与“missing snapshot”两类错误原样可见（不被包装为空响应），Rust 的错误码映射在路由层断言，缺少同形的 provider 错误透传用例。 |
| [x] | `internal/marketdata/service_facade_test.go:83:TestServiceSnapshotResolvesChinaAggregateToExchangeLeaf` | `normalize_instrument_handles_cn_market_and_prefixes`；`futu_snapshot_route_projects_cached_extended_quote_contract` | P2 | partial：Rust 覆盖 CN 市场代码前缀推断（600519→SH.600519 一类叶市场归一）与快照读；差异：Go 断言 GetSnapshot("CN","600519") 时 provider 收到的 snapshotID 是叶市场，Rust 没有针对 CN 聚合参数落到叶市场的快照路由断言。 |
| [x] | `internal/marketdata/service_facade_test.go:105:TestServiceProviderReadsResolveChinaAggregateToExchangeLeaf` | `infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes`；`instrument_search_route_returns_subset_resolution_contract` | P2 | partial：Rust 覆盖 CN 前缀推断与 CN 子集搜索解析为叶市场；差异：Go 断言 security details/candles/depth 三条读在 CN 聚合入参下都改写为叶市场请求，Rust 没有这三条读的 CN 聚合改写断言。 |
| [x] | `internal/marketdata/service_facade_test.go:136:TestServiceRejectsSnapshotCompletedAfterProviderChange` | `snapshot_read_fences_provider_generation_switch_during_helper_query`；`completion_after_generation_change_cannot_mutate_cache_or_failure_state` | P2 | partial：Rust 断言 provider 切换期间完成的旧快照读被 generation fence 拒绝且不写入缓存；差异：Go 断言旧 provider 结果错误可见且不重新填充缓存，Rust 以读端口错误与缓存计数断言表达，语义等价。 |
| [x] | `internal/marketdata/service_facade_test.go:162:TestServiceTickCandlesProviderAndFallbackBoundaries` | `tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample`；`tick_candles_use_fresh_cache_without_querying_the_provider`；`tick_candles_fall_back_to_retained_cache_on_ticker_error` | P1 | partial：Rust 覆盖 tick candles 的 provider 查询/缓存命中/回退三分支；差异：Go 还断言不支持 tick session 时报错、无缓存且 ticker 失败时报错，Rust 的会话拒绝与无缓存失败分支没有对应断言。 |
| [x] | `internal/marketdata/service_facade_test.go:213:TestLimitCandleMapsKeepsLatestEntriesOnly` | `tick_candles_filter_sessions_before_applying_the_limit`；`broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates` | P2 | partial：Rust 断言 limit 只保留最新条目（过滤后截断）与 K 线分页助手；差异：Go 的 limitCandleMaps(limit<=0 返回全集) 纯辅助断言在 Rust 由 limit 归一（<=0 → 200 上限 1000）承担，无同形纯函数用例。 |
| [x] | `internal/marketdata/service_facade_test.go:237:TestServiceSubscriptionFacadeCacheHelpersAndLifecycle` | `partial_release_and_clear_operations`；`subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear` | P1 | partial：Rust 覆盖缓存命中计数/最新样本、订阅 acquire/heartbeat/active instruments/release/clear 生命周期与定点释放；差异：Go 单条测试串联 cache helpers（CachedCount/Latest/LatestMany/AllFresh/LiveTick）与订阅路由并断言 nil RuntimeState 为零值，Rust 拆到 demand 与订阅路由两处，无聚合断言。 |
| [x] | `internal/marketdata/service_facade_test.go:320:TestServiceHealthAndSerializationNilBoundaries` | `market_data_runtime_projection_matches_go_status_corpus`；`recorder_matches_generation_retry_recovery_and_close_rules` | P2 | partial：Rust 以 runtime 状态投影（connected/closed/degraded/activeCount 等 wire 契约）覆盖健康字段与关闭语义；差异：Go 断言 nil 序列化（SnapshotJSON(nil)==nil、LatestTicksJSON(nil)）与 nil RuntimeState 归零，Rust 无 nil 输入概念（类型非空），该分支为不适用边界。 |
| [x] | `internal/marketdata/service_facade_test.go:335:TestServiceProviderStatusCombinesDescriptorHealthAndDemand` | `market_data_provider_read_routes_match_group_fixture_in_cutover_only`；`ProductionMarketDataProviderPort`；`market_data_runtime_projection_matches_go_status_corpus` | P2 | partial：Rust /api/v1/market-data/provider 组合 provider 描述、健康与需求（activeCount）投影，并已修复 helper provider 的 snapshot-poll-delayed/fallback streamMode；差异：Go 还逐项断言 descriptor 错误透传、完整 readiness/streamMode 矩阵和 subscriptions envelope，Rust 仍以 cutover/owner 测试分散覆盖。 |

本片结果：34/34 完成 Go 断言→Rust 证据核对；没有发现新的、需要先红后修的生产功能差异。CN 聚合→叶市场、错误分类、日期/limit 校验、provider capability 和 tick fallback 均有可运行 Rust owner 证据，但仍因测试结构/调用 seam 不同保留 partial。现有 provider helper `streamMode` 差异已由上一片的回归测试与 owner 修复覆盖。

下一片：`internal/marketdata/subscriptions_test.go` 4 条与 `internal/marketdata/quote_availability_test.go` 2 条，再进入 `lifecycle_boundaries_test.go` 余量；如发现真实 P1 差异，先写失败回归测试并修复所属 owner。

验证：`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-engine --all-targets --locked --no-fail-fast`（1993 passed，0 skipped，receipt `sha256:84c5b407dddaa20033c7dbea771fda2abd895f0871d9a6fb43291700162b9566`）；随后执行 mapping 审计、锚点复核、`pnpm run check:quick`、`pnpm run check:rust` 与 `git diff --check`。

## 第 131 批：subscriptions 与 quote availability（2026-09-25）

本片覆盖 `internal/marketdata/subscriptions_test.go` 的 4 条测试和
`internal/marketdata/quote_availability_test.go` 的 2 条测试，共 6 条 Go
测试。逐条读取 Go 断言并核对 DemandBook、订阅 mutation port、live hub、runtime
status 与 quote snapshot projection owner；6/6 保持 `[~]`/`partial`，没有把跨 owner
聚合测试升级为 `function_exact`。

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `subscriptions_test.go:10:TestSubscriptionRegistryContract` | `demand_is_deduplicated_and_managed_leases_do_not_expire`；`heartbeat_updates_consumer_and_entry_timestamps`；`partial_release_and_clear_operations` | P2 | partial：覆盖规范化、managed lease、心跳、部分释放和最终清理；Go 的 RFC3339 时间、quota envelope 与 refCount 递增未由同一 wire 测试锁定。 |
| [x] | `subscriptions_test.go:51:TestSubscriptionRegistrySeparatesChannelAndInterval` | `channel_and_interval_validation_rules`；`demand_is_deduplicated_and_managed_leases_do_not_expire` | P2 | partial：覆盖 channel/interval 分键和 interval 规范化；Go 的 active instrument 去重、计数和定点 release 聚合断言分散在不同 owner。 |
| [x] | `subscriptions_test.go:82:TestSubscriptionRegistryClearAllAndActiveInstruments` | `partial_release_and_clear_operations`；`active_instruments_are_normalized_unioned_replaced_and_released` | P2 | partial：覆盖 unmanaged clear 与 WS active instrument 归一/并集/替换/释放；没有同一 registry 快照的完整串联。 |
| [x] | `subscriptions_test.go:100:TestServiceOwnsSubscriptionsAndHealthMode` | `subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear`；`live_hub_demand_listener_tracks_subscriptions_and_disconnect`；`market_data_runtime_projection_matches_go_status_corpus` | P2 | partial：订阅 ownership、disconnect 和 runtime wire 有证据；Go 的 idle/fallback/push/delayed 四态 Health 矩阵未在同一 Service fixture 重现。 |
| [x] | `quote_availability_test.go:9:TestSnapshotSerializationPreservesAuthoritativeMissingQuoteFields` | `authoritative_snapshot_keeps_missing_quote_fields_null`；`futu_snapshot_route_projects_cached_extended_quote_contract` | P2 | partial：authoritative quote 的 snapshot 缺失字段已断言为 null；Go 的 LiveTickJSON/LatestTicksJSON 可空字段和 brokerId 断言没有 Rust 同形入口。 |
| [x] | `quote_availability_test.go:35:TestSnapshotSerializationKeepsLegacyZeroValuesAvailable` | `legacy_tick_without_snapshot_keeps_zero_quote_fields_available`；`futu_snapshot_route_projects_cached_extended_quote_contract` | P2 | partial：legacy Tick 的 bid/ask/volume/turnover 均保持字符串 0；Go 针对 SnapshotJSON helper 的公共 wire 断言尚无同名 Rust 测试。 |

本片发现并保留了工作树中已有的 quote availability owner 修复：typed Futu quote
标记 authoritative，缺失 volume 不再被 tick 累积值填充；projection 对 authoritative
缺失 quote 输出 null，同时继续保持无 snapshot legacy tick 的零值兼容。新增回归测试
已精准运行并通过，未发现需要继续修复的 P1 生产差异；Live/Latest 两个旧 helper
入口属于 Rust 架构边界，记录为 partial。

下一片：`internal/marketdata/lifecycle_boundaries_test.go` 的余量，优先处理
`TestCacheRemainingLifecycleBoundaries`、`TestSubscriptionRegistryRemainingLifecycleBoundaries`
和 `TestServiceRemainingLifecycleBoundaries`，若发现真实差异先补失败回归测试再修改 owner。

验证：`cargo fmt --all -- --check`；精准回归
`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-engine -p jftrade-api -p jftrade-integration-futu --all-targets --locked -E 'test(demand_is_deduplicated_and_managed_leases_do_not_expire) or test(partial_release_and_clear_operations) or test(heartbeat_updates_consumer_and_entry_timestamps) or test(channel_and_interval_validation_rules) or test(subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear) or test(active_instruments_are_normalized_unioned_replaced_and_released) or test(live_hub_demand_listener_tracks_subscriptions_and_disconnect) or test(market_data_runtime_projection_matches_go_status_corpus) or test(authoritative_snapshot_keeps_missing_quote_fields_null) or test(legacy_tick_without_snapshot_keeps_zero_quote_fields_available) or test(futu_snapshot_route_projects_cached_extended_quote_contract)'`（11/11 通过）。随后执行 mapping 审计、锚点复核、`check:quick`、`check:rust` 与 `git diff --check`。

## 第 132 批：lifecycle boundaries 余量（2026-09-25）

本片覆盖 `internal/marketdata/lifecycle_boundaries_test.go` 中尚未逐项复核的 7 条测试；`TestNormalizeInstrumentIDRejectsIncompleteValues` 已有 exact 证据，未重复修改。7/7 继续保持 `[~]`/`partial`，没有把拆分到多个 owner 的聚合测试升级为 `function_exact`。

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `lifecycle_boundaries_test.go:13:TestCacheRemainingLifecycleBoundaries` | `cache_rejects_stale_generation_and_classifies_freshness`；`cache_rejects_empty_generation_and_backwards_timestamp_inputs`；`a_new_trading_day_or_session_resets_the_cumulative_baseline` | P1 | partial：无效 tick、stale freshness 与交易日基线已验证；Go 的 zero-age/空集合/clear/clone nil/零时间戳和周末 fallback 未同形断言。 |
| [x] | `lifecycle_boundaries_test.go:60:TestSubscriptionRegistryRemainingLifecycleBoundaries` | `demand_is_deduplicated_and_managed_leases_do_not_expire`；`partial_release_and_clear_operations`；`active_instruments_are_normalized_unioned_replaced_and_released` | P2 | partial：demand 与 active instrument 生命周期已验证；TTL=0、blank consumer 归一和同一 registry 快照串联保留差异。 |
| [x] | `lifecycle_boundaries_test.go:101:TestServiceRemainingLifecycleBoundaries` | `market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable`；`snapshot_rejects_malformed_refresh_before_provider_access`；`empty_invalid_closed_and_inactive_demand_never_calls_provider` | P2 | partial：取消/畸形 refresh 与无需求/关闭 fail-closed 已验证；nil Service、ProviderStatus、默认周期和 subscription context 仍跨 owner。 |
| [x] | `lifecycle_boundaries_test.go:233:TestServiceFinalSubscriptionCleanupAlwaysHasDeadline` | `close_shuts_down_the_transport_before_joining_the_reader`；`direct_product_shutdown_stops_reconciliation_before_releasing_leases` | P2 | partial：Rust 关闭顺序与 bounded shutdown 已验证；Go 的最终 ReconcileSubscriptions deadline 参数没有同形断言。 |
| [x] | `lifecycle_boundaries_test.go:271:TestCollectorAdvancesInactiveSubscriptionCleanupAfterActiveDemand` | `router_drives_runtime_recorder_from_provider_and_demand_state`；`reconcile_preserves_demand_and_clears_previous_runtime_state` | P2 | partial：需求同步和 runtime 清理已验证；inactive cleanup 计数/重试及 Close fence 没有同形字段。 |
| [x] | `lifecycle_boundaries_test.go:302:TestCollectorRemainingLifecycleBoundaries` | `empty_invalid_closed_and_inactive_demand_never_calls_provider`；`recorder_matches_generation_retry_recovery_and_close_rules`；`basic_quote_query_uses_the_collector_900ms_deadline_boundary` | P2 | partial：collector provider gate、generation/retry/close 和 900ms deadline 已验证；nil source、detached stream、负 retry clamp 等分支仍是结构差异。 |
| [x] | `lifecycle_boundaries_test.go:416:TestInstrumentResolverRemainingLifecycleBoundaries` | `futu_search_rejects_invalid_queries_before_reaching_opend`；`provider_aware_methods_prefix_providers_and_validate_provider_names` | P2 | partial：Futu 输入拒绝和 provider 前缀校验已验证；MarketSubset resolver 的 peer cache、单飞取消、alias 纯函数由 Python/helper 边界承担。 |

本片没有发现需要先红后修的 Rust 生产差异；所有引用 owner 均为现有实现，差异集中在 Go 聚合 fixture、nil/function helper 与 Rust typed/sidecar 架构边界。精准 nextest 16/16 通过。

下一片：按 P1 优先处理 `internal/api/assistant/chat_helpers_test.go` 的
`TestTimelineStreamStateTracksSessionRunAndToolTiming`、
`TestTimelineStreamStateEmptyAndCloneBoundaries`，以及
`internal/api/assistant/chat_stream_recovery_contracts_test.go` 的前 3 条；若出现真实
ADK/Assistant 功能差异，先补失败回归测试再修改领域 owner。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-engine -p jftrade-api -p jftrade-integration-futu -p jftrade-integration-marketdata-helper --all-targets --locked -E 'test(cache_rejects_stale_generation_and_classifies_freshness) or test(cache_rejects_empty_generation_and_backwards_timestamp_inputs) or test(a_new_trading_day_or_session_resets_the_cumulative_baseline) or test(demand_is_deduplicated_and_managed_leases_do_not_expire) or test(partial_release_and_clear_operations) or test(active_instruments_are_normalized_unioned_replaced_and_released) or test(market_data_quote_read_routes_fail_closed_when_snapshot_is_unavailable) or test(snapshot_rejects_malformed_refresh_before_provider_access) or test(empty_invalid_closed_and_inactive_demand_never_calls_provider) or test(close_shuts_down_the_transport_before_joining_the_reader) or test(direct_product_shutdown_stops_reconciliation_before_releasing_leases) or test(router_drives_runtime_recorder_from_provider_and_demand_state) or test(reconcile_preserves_demand_and_clears_previous_runtime_state) or test(recorder_matches_generation_retry_recovery_and_close_rules) or test(basic_quote_query_uses_the_collector_900ms_deadline_boundary) or test(futu_search_rejects_invalid_queries_before_reaching_opend) or test(provider_aware_methods_prefix_providers_and_validate_provider_names)'`（16/16 通过）。

## 第 134 批：backtest sync worker lifecycle（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `historical_source_test.go:310:TestBacktestProviderSyncerPinsFutuAndClosesOnFailures` | `test_historical_k_line_syncer_rejects_empty_provider_result`；`shutdown_persists_cancellation_before_joining_sync_workers`；`terminate_persists_cancellation_and_aborts_sync_workers`；`production_sync_restart_recovery_marks_orphaned_task_failed` | P1 | partial：空 provider 结果、shutdown/terminate 在 join 或 abort 前持久化 cancelled、重启 orphaned task 标记 failed 已有 Rust 回归；固定 futu、成功后 Close、descriptor 错误和非法数据库路径 constructor 属于旧 owner 或 composition 注入边界。 |

本批先红后修没有发现新的 Rust 生产功能差异；新增 worker registry 生命周期回归由 `jftrade-engine` owner 持有。精准 nextest 4/4 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(test_historical_k_line_syncer_rejects_empty_provider_result) or test(shutdown_persists_cancellation_before_joining_sync_workers) or test(terminate_persists_cancellation_and_aborts_sync_workers) or test(production_sync_restart_recovery_marks_orphaned_task_failed)'`

下一片：`internal/app/apiserver/backtestapp/historical_source_test.go:354:TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks`，优先核对 provider 规则与保守 fallback 的 owner 及逐市场断言。

## 第 135 批：InstrumentSpec provider rules 与 conservative fallback（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `historical_source_test.go:354:TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks` | `broker_lot_size_initializes_minimum_and_step_quantity`；`market_rules_match_trimmed_symbols_and_apply_overrides_in_order`；`market_rules_use_security_info_lot_size_without_warnings`；`market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error`；`market_rules_ssot_contains_all_core_markets_with_decimal_tick_sizes`；`inferred_market_profiles_match_go_market_rules` | P1 | partial：Rust 覆盖 Futu lot size 成功与 snapshot fallback/warning、lot→最小/步长、规则覆写及 HK/US/CN/SH/SZ quote/tick static profiles；旧 backtestapp InstrumentSpec 的动态 priceSpread、HK 500/0.2、A 股 100 conservative fallback、缺失规则告警、details deadline 与 ProviderOptions 没有同形 resolver。 |

本批未发现需要先红后修的 Rust 生产差异；缺口属于旧 backtestapp 私有 resolver 与 Rust composition/backtest payload owner 的架构边界。精准 nextest 6/6 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-broker -p jftrade-integration-futu -p jftrade-marketdata --all-targets --locked -E 'test(broker_lot_size_initializes_minimum_and_step_quantity) or test(market_rules_match_trimmed_symbols_and_apply_overrides_in_order) or test(market_rules_use_security_info_lot_size_without_warnings) or test(market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error) or test(market_rules_ssot_contains_all_core_markets_with_decimal_tick_sizes) or test(inferred_market_profiles_match_go_market_rules)'`

下一片：`internal/app/apiserver/backtestapp/historical_source_test.go:385:TestInstrumentSpecRequiresReadyPythonProviders`，核对 Python provider readiness 白名单与 Rust provider activation owner 的边界。

## 第 136 批：InstrumentSpec Python provider readiness（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `historical_source_test.go:385:TestInstrumentSpecRequiresReadyPythonProviders` | `readiness_without_composed_runtimes_reports_all_false`；`news_actions_binding_requires_yfinance_helper_readiness`；`test_helper_health_failure_dynamically_downgrades_provider_readiness` | P2 | partial：Rust 覆盖未装配 runtime 时三路 readiness 均为 false、yfinance/akshare helper-backed 路由在 helper 未 ready 时不可用，以及 helper 健康失败后的动态降级/恢复；旧 backtestapp 对 yfinance、规范化 `YFINANCE`、akshare 与 Futu 的 `instrumentRulesRequireReady` 白名单和 InstrumentSpec resolver 没有同形逐 provider 断言。 |

本批没有发现需要先红后修的 Rust 生产功能差异；三条 readiness 证据均为既有 engine owner。精准 nextest 3/3 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(readiness_without_composed_runtimes_reports_all_false) or test(news_actions_binding_requires_yfinance_helper_readiness) or test(test_helper_health_failure_dynamically_downgrades_provider_readiness)'`

下一片：`internal/app/apiserver/backtestapp/historical_source_test.go:401:TestProviderOptionsRequireMarketDataRuntime`，核对旧 ProviderOptions 构造器与 Rust composition/backtest startup 边界。
