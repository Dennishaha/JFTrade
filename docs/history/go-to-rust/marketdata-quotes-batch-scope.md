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

## 第 137 批：ProviderOptions market-data runtime boundary（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `historical_source_test.go:401:TestProviderOptionsRequireMarketDataRuntime` | `production_backtest_start_without_worker_fails_before_persisting_run` | P2 | boundary：Go 断言 `ProviderOptions(nil, ...)` panic 为 `assemble backtest service: market-data provider runtime is unavailable`，并隐含固定数量 options 构造；Rust 没有同形函数式 options owner，但回测 start 在 worker runtime 缺失时返回 unavailable 且 run_count 保持 0，属于 composition/backtest port 形状差异。 |

本批没有发现需要先红后修的 Rust 生产功能差异；精准 nextest 1/1 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(production_backtest_start_without_worker_fails_before_persisting_run)'`

下一片：`internal/app/apiserver/backtestapp/historical_source_test.go:414:TestPositiveFloatRecognizesSupportedRuleTypes`，核对市场规则正浮点过滤与 Rust typed constraint parser 的差异。

## 第 138 批：positiveFloat 与 typed market-rule constraints（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `historical_source_test.go:414:TestPositiveFloatRecognizesSupportedRuleTypes` | `market_rules_ignore_missing_non_positive_and_non_finite_constraints` | P2 | partial：Go `positiveFloat` 接受 float64/float32/int/int32/int64 与 decimal string 的正值并拒绝 0、非法字符串、bool；Rust broker 规则层覆盖正约束过滤、零/负 lot、非有限 Fixed8 忽略，但 typed `Fixed8`/serde 输入没有同形宽 `any` 逐类型 helper 表。 |

本批没有发现需要先红后修的 Rust 生产功能差异；差异属于 Go 私有宽类型 helper 与 Rust typed DTO/Fixed8 owner 的边界。精准 nextest 1/1 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-broker --all-targets --locked -E 'test(market_rules_ignore_missing_non_positive_and_non_finite_constraints)'`

下一片：继续处理 `historical_source_test.go` 后续 backtest source/provider 条目，保持 P1 优先并在出现真实 owner 差异时先写失败回归。

## 第 139 批：provider health backoff 与 cancellation boundary（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `runtime_health_test.go:222:TestProviderHealthRetryDelayBacksOffAndCaps` | `compute_helper_backoff_doubles_and_caps_at_max` | P1 | partial：Rust 断言 helper restart backoff 倍增并封顶，但默认序列为 500ms→…→10s，Go `waitForProviderHealth` 为 100ms→…→1s；两者 owner/数值都不同。 |
| [x] | `runtime_health_test.go:240:TestWaitForProviderHealthPreservesLastFailureOnCancellation` | `disconnected_provider_reports_its_reason_and_keeps_the_previous_selection` | P1 | partial：Rust 覆盖 disconnected reason 和保持既有 provider selection；Go 的 context cancellation、底层 probeErr 原样返回及单次探测次数没有 Rust 同形 monitor API。 |

本批没有发现可安全直接修复的 Rust 生产差异：backoff 属于 Rust helper restart policy，取消路径属于 Go wait helper 与 Rust router/monitor owner 形状差异。精准 nextest 2/2 通过。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-marketdata --all-targets --locked -E 'test(compute_helper_backoff_doubles_and_caps_at_max) or test(disconnected_provider_reports_its_reason_and_keeps_the_previous_selection)'`

下一片：继续 `internal/app/apiserver/marketdataapp/runtime_health_test.go` 的 provider health/activation 条目。

## 第 140 批：runtime health activation 与 subscription rollback 边界（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `runtime_health_test.go:13:TestRuntimeExplicitYFinanceActivationRequiresHealthBeforePublishing` | `explicit_activation_requires_ready_health_but_startup_restore_allows_warming`; `explicit_activation_fails_closed_and_switch_clears_cache` | P2 | exact：显式激活要求 Ready、失败不发布，健康切换递进 generation 并清理旧运行态；sidecar stub 调用次数属旧 owner 边界。 |
| [x] | `runtime_health_test.go:64:TestRuntimeFailedHealthCheckRestoresSidecarWithoutChangingProvider` | `recovery_after_a_failed_health_check_publishes_a_healthy_provider` | P2 | partial：Rust 覆盖失败保持旧 provider、恢复后发布；没有 Go sidecar ensure/stop 计数与订阅保持断言。 |
| [x] | `runtime_health_test.go:94:TestRuntimeReportsBothHealthAndSidecarRestoreFailures` | `failed_warmup_blocks_startup_restore_and_reports_last_error` | P2 | partial：Rust 覆盖 provider last_error 透出且不发布；sidecar restore 独立错误合并属于 Go lifecycle owner。 |
| [x] | `runtime_health_test.go:114:TestRuntimeDefersSubscriptionReleaseFailureAfterHealthyActivation` | `failed_unsubscribe_is_deferred_until_its_retry_window` | P2 | partial：Rust 覆盖卸载失败进入重试窗口；未串联激活成功后异步 reconcile 暴露原错误的整条流程。 |
| [x] | `runtime_health_test.go:142:TestRuntimeChecksEmbeddedYFinanceOnStartupButNotFutu` | `explicit_activation_requires_ready_health_but_startup_restore_allows_warming` | P2 | partial：Rust 覆盖 StartupRestore 允许 warming、Explicit 要求 Ready；未覆盖按 provider 类型统计 healthCheck 次数。 |
| [x] | `runtime_health_test.go:167:TestWaitForProviderHealthRetriesUntilConnected` | `retries_transient_readiness_and_sends_optional_bearer`; `recovery_after_a_failed_health_check_publishes_a_healthy_provider` | P2 | exact：Rust 覆盖第一次失败、第二次成功、请求重试次数与恢复发布；具体退避常量差异已在第139批单列。 |
| [x] | `runtime_health_test.go:183:TestWaitForProviderHealthAllowsWarmingOnlyDuringStartupRestore` | `warming_provider_is_only_publishable_during_startup_restore` | P2 | exact：同一 warming 健康状态在 StartupRestore 发布，在 Explicit fail closed 且不改变 active。 |
| [x] | `runtime_health_test.go:205:TestWaitForProviderHealthStopsOnFailedWarmup` | `failed_warmup_blocks_startup_restore_and_reports_last_error` | P2 | partial：Rust 覆盖 Failed readiness 的错误透出和不发布；没有 Go calls==1 的等待层单次探测断言。 |
| [x] | `runtime_health_test.go:261:TestWaitForProviderHealthAndRuntimeDefaultCheckerBoundaries` | `unknown_provider_activation_is_rejected_without_touching_the_active_selection` | P2 | exact：未注册 provider fail closed 且保持 active；健康默认检查成功路径由同文件 recovery 用例补证。 |

本批没有发现需要修改 Rust 生产 owner 的真实差异；逐条 nextest 8/8 通过，sidecar 计数、错误合并和 provider 类型调用计数保留为边界/partial。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-marketdata -p jftrade-integration-futu -p jftrade-integration-marketdata-helper --all-targets --locked -E 'test(explicit_activation_requires_ready_health_but_startup_restore_allows_warming) or test(explicit_activation_fails_closed_and_switch_clears_cache) or test(recovery_after_a_failed_health_check_publishes_a_healthy_provider) or test(failed_warmup_blocks_startup_restore_and_reports_last_error) or test(warming_provider_is_only_publishable_during_startup_restore) or test(unknown_provider_activation_is_rejected_without_touching_the_active_selection) or test(failed_unsubscribe_is_deferred_until_its_retry_window) or test(retries_transient_readiness_and_sends_optional_bearer)'`

下一片：继续 `runtime_akshare_test.go` 与 `runtime_health_test.go` 相邻的 provider activation/recovery 条目。

## 第 141 批：AKShare/YFinance sidecar activation 与缓存边界（2026-09-25）

| 复核 | Go 测试 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `runtime_akshare_test.go:11:TestRuntimeReusesSharedSidecarAcrossPythonProviders` | `managed_process_is_reused_until_an_explicit_stop_releases_it`; `helper_provider_switch_preserves_observed_opend_trade_readiness` | P2 | exact：托管 helper 在显式 stop 前复用，Python provider 切换不清理既有 OpenD readiness；旧 runtime 的 ensure 次数由 sidecar owner 负责。 |
| [x] | `runtime_akshare_test.go:48:TestRuntimeKeepsSharedSidecarOnCrossPythonActivationFailure` | `provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update` | P2 | partial：Rust 覆盖健康失败不发布、保留上一代并在恢复后提交；没有 Go ensure=2/stop=0/running 的共享 sidecar 计数。 |
| [x] | `runtime_akshare_test.go:78:TestRuntimeStopsNewSidecarWhenInitialAKShareActivationFails` | `a_failed_launch_never_leaves_a_child_or_a_stale_endpoint` | P2 | exact：初次 helper 启动失败不遗留子进程或 stale endpoint，对应失败激活回收新 sidecar。 |
| [x] | `runtime_akshare_test.go:97:TestRuntimeRetriesAProviderMarkedUnavailable` | `recovery_after_a_failed_health_check_publishes_a_healthy_provider` | P2 | exact：provider 健康失败后恢复即可再次激活并发布。 |
| [x] | `runtime_akshare_test.go:126:TestRuntimeUsesGenericCacheDirectoryWithLegacyFallback` | `verifies_and_materializes_content_addressed_asset` | P1 | partial：Rust 覆盖内容寻址资产校验与 materialize；Go 通用缓存目录优先、legacy 目录回退的路径选择没有 Rust 同形逻辑。 |

本批没有发现需要修改 Rust 生产 owner 的真实差异；精准 nextest 6/6 通过。sidecar ensure/stop 计数与 generic/legacy cache path 保留为边界或 partial。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-engine -p jftrade-marketdata --all-targets --locked -E 'test(managed_process_is_reused_until_an_explicit_stop_releases_it) or test(helper_provider_switch_preserves_observed_opend_trade_readiness) or test(provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update) or test(a_failed_launch_never_leaves_a_child_or_a_stale_endpoint) or test(recovery_after_a_failed_health_check_publishes_a_healthy_provider) or test(verifies_and_materializes_content_addressed_asset)'`

下一片：继续 marketdata runtime/provider activation 后续 Go 测试，并优先清理 P1 partial。

## 第 142 批：MarketData route/query/cache 与 research forwarding（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:229,273,286,300,314,523,555` | `query_time_parses_rfc3339_datetime_and_date`; provider retry error projection tests; candle/search validation; subscription cancellation/cleanup tests | P1 | exact：查询时间、provider changed/warming/busy/unknown 错误、invalid sessions 和 cancellation cleanup 均逐项有 Rust 断言。 |
| [x] | `internal/api/marketdata/routes_news_actions_test.go:76,115` | news/corporate-actions production port and query validation tests | P1 | exact：limit/range 校验、provider 参数转发、helper failure 映射均通过。 |
| [x] | `internal/api/marketdata/routes_test.go:302,321,407` | candle session normalization/rejection, tick cache and strict-before pagination tests | P1 | exact：重复 session 去重排序、非法 session 拒绝、tick cache 与 before 窗口行为逐条覆盖。 |
| [x] | `market_http_test.go:18,137,150,245,306,330,382,418,442` | US session labels, invalid/unknown session errors, snapshot/tick cache hit/miss/refresh/fallback tests | P1 | exact：intraday session label、输入前置校验、snapshot/tick cache 路径和 provider 调用次数均覆盖。 |
| [x] | `market_http_test.go:91:TestMarketCandlesResponseOmitsSessionMetadataForDailyCandles` | `candle_route_skips_session_classification_for_unannotated_requests` | P1 | partial：Rust 覆盖未标注请求跳过 session 分类；Go 额外断言 JSON 中 daily candle/meta 不出现 session 字段，wire omission 仍需补证。 |
| [x] | `query_test.go:45:TestDecodeMarketCandlesQueryParsesRepeatedSessions` | `candle_sessions_parse_dedup_order_and_reject_invalid` | P1 | exact：重复 query session 的去重、稳定排序和非法值拒绝一致。 |
| [x] | `runtime_rankings_industry_forwarding_test.go:92` | industry board/member provider-kind forwarding tests | P1 | exact：Industries/IndustryMembers 参数、调用次数和上游错误透传均覆盖。 |

本批 Go 24 条映射对应 Rust 精准 nextest 27 条测试，全部通过；daily session JSON omission 继续保持 `partial`，未发现需要修复的 Rust 生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(query_time_parses_rfc3339_datetime_and_date) or test(snapshot_read_fences_provider_generation_switch_during_helper_query) or test(market_data_read_errors_expose_provider_warmup_retry_signal) or test(market_data_read_errors_expose_provider_busy_retry_signal) or test(market_data_read_errors_preserve_unclassified_helper_failures) or test(candles_and_search_validation_rules) or test(subscription_mutations_map_canceled_context_to_failed) or test(release_and_clear_map_snapshot_failure_after_logical_cleanup) or test(production_news_actions_port_forwards_yfinance_news_request) or test(production_news_actions_port_maps_helper_failure_and_rejects_bad_limit) or test(production_news_actions_port_forwards_corporate_actions_window) or test(corporate_actions_query_requires_rfc3339_and_ascending_range) or test(candle_route_normalizes_repeated_sessions) or test(candle_route_rejects_invalid_sessions) or test(tick_candles_use_fresh_cache_without_querying_the_provider) or test(candle_route_forwards_strict_before_window) or test(us_intraday_futu_candles_carry_calendar_resolved_session_labels) or test(candle_route_skips_session_classification_for_unannotated_requests) or test(market_http_rejects_invalid_sessions_before_provider_access) or test(candle_route_classifies_unknown_us_session_as_a_data_error) or test(snapshot_route_serves_a_fresh_cache_hit_without_provider_access) or test(snapshot_route_queries_the_provider_once_on_cache_miss) or test(snapshot_route_force_refresh_bypasses_cache) or test(tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample) or test(tick_candles_fall_back_to_retained_cache_on_ticker_error) or test(candle_sessions_parse_dedup_order_and_reject_invalid) or test(industry_board_operations_map_to_provider_kinds_on_the_wire) or test(industry_plate_members_read_the_board_from_the_instrument_id)'`

下一片：继续 P1 的 MarketData/Quote provider forwarding 与 runtime boundary 映射。

## 第 159 批：Assistant session context、compaction 与 stale projection（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/engine/session_context_projection_test.go:15,60,118,181,208` | protected-tail approval filtering, context compaction and session gate tests | P1 | partial：projection、approval tail 与 compaction 分支逐条通过；Go helper 聚合和错误文本由多个 Rust owner 拆分承载。 |
| [x] | `internal/assistant/engine/session_context_recovery_edges_test.go:9`; `session_context_retry_boundaries_test.go:89,143` | chat auto-compaction, rejected compaction notice and gate contention tests | P1 | exact：恢复、失败 notice 与并发 gate 行为均有对应 Rust 断言。 |
| [x] | `internal/assistant/engine/session_context_stale_test.go:106,160,188,209,231,315,420` | stale event/session fencing, duplicate rollback, restart durability, projection size and pressure tests | P1 | partial：stale retry、数据库回滚、handoff/input 恢复和 projection pressure 均通过；Go 的单体 retry 聚合仍由拆分证据覆盖。 |
| [x] | `internal/assistant/engine/session_context_test.go:15,120,183,287,341,446,506,569,651,703,773,787,800,813,827` | context window/revision, manual/auto compaction, pending approval rewind, handoff revision tests | P1 | exact：窗口、revision、notice、gate、workflow compaction 与 approval tail 行为逐项通过。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 24 个去重测试通过，另以 `session_context_ignores_handoff_segments_without_a_revision` 替换无实际 Rust 测试的 `session_context_snapshot` 条目；未发现需要先红后修的生产差异。projection 聚合保持 partial，不把重复或拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(protected_tail_ignores_approvals_closed_by_a_denied_run) or test(context_compaction_shrinks_the_projected_session_view) or test(protected_tail_keeps_only_the_approval_that_is_still_pending) or test(protected_tail_ignores_an_approval_with_a_durable_tool_outcome) or test(protected_tail_starts_at_the_earliest_unresolved_approval) or test(a_chat_turn_autocompacts_the_session_before_the_provider_payload) or test(a_rejected_compaction_records_the_failed_notice) or test(a_second_compaction_is_rejected_while_the_session_gate_is_held) or test(event_must_match_the_run_and_session_before_projection_changes) or test(duplicate_event_key_with_different_content_rolls_back_projection) or test(adk_session_store_lifecycle_and_restart_durability) or test(missing_session_database_is_not_created_and_run_is_unchanged) or test(pending_input_is_persisted_and_answer_resume_is_idempotent) or test(model_context_autocompacts_before_the_provider_payload) or test(session_context_read_reports_pressure_without_compacting) or test(session_context_window_follows_the_composer_provider_override) or test(each_context_compaction_creates_the_next_current_revision) or test(manual_context_compaction_writes_the_done_notice_into_the_timeline) or test(auto_compaction_emits_streaming_then_final_notice_and_context_delta) or test(auto_compaction_skips_while_another_compaction_holds_the_session_gate) or test(model_context_read_compacts_only_when_the_session_gate_is_free) or test(workflow_auto_compaction_proceeds_under_an_active_run_while_chat_waits) or test(protected_tail_rewinds_a_pending_approval_to_its_original_call) or test(session_context_ignores_handoff_segments_without_a_revision)'`

下一片：继续 P1 Assistant session skill/store、workflow/ADK 与 API transport 映射。

## 第 148 批：Futu/OpenD connection recovery、market/trading reads 与 user security（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/futu/client_exchange_recovery_boundaries_test.go:19,56,107,198,249,260,282` | recoverable replay policy、closed session replacement、notification/reconnect fencing、trade push account normalization、minimum-version and typed transport failures | P1 | exact/partial：replay-safe/read-only policy、reconnect fencing、version and typed failure paths are covered; Go factory environment fallback and callback binding remain partial. |
| [x] | `pkg/futu/opend/market_read_boundaries_test.go:21,77,129,192,212,232,284` | quote/history request encoding, business errors, empty ACK projections, disconnected guards and malformed push filtering | P1 | exact/partial：optional wire fields, OpenD error details, empty collection/result normalization, closed-session rejection and push filtering pass; line 212 shares the existing kline query test and stays partial to avoid duplicate exact evidence. |
| [x] | `pkg/futu/opend/trading_error_boundaries_test.go:26,102,138`; `trading_methods_test.go:64,83,182,244,334` | trade read errors/closed guards, history filters, authenticated order writes, account push request/rejection | P1 | exact：trade read/write prerequisites, history filters, account push decoding and typed business errors are asserted. `ModifyOrder` error aggregation remains partial where Rust splits protocol and fill validation. |
| [x] | `pkg/futu/opend/trading_write_boundaries_test.go:17,58,82`; `user_security_test.go:15,51,93,103` | trade write guards/rejection/empty result and user-security group/member protocol tests | P1 | exact：write prerequisites, business rejection, stable empty results, group/member encoding, input validation and empty payloads are covered. |

本批 30 条 Go 映射对应 Rust 精准 nextest 32/32 通过；没有发现需要先红后修的 Rust 生产差异。旧 BBGO factory/env、callback binding、current-KL duplicate evidence 与 Rust typed error 拆分继续保持 partial，不把聚合测试提升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked -E 'test(recoverable_error_policy_gates_replay_safe_reads) or test(recoverable_errors_match_go_is_recoverable_opend_err) or test(closed_ready_session_is_replaced_on_peer_close) or test(reconnect_completes_while_a_notification_listener_reads_coordinator_state) or test(trade_push_subscription_forwards_the_requested_accounts) or test(below_minimum_version_fails_session_initialization) or test(init_response_and_session_transport_failures_stay_typed) or test(trade_push_subscription_failure_surfaces_a_transport_error) or test(quote_subscribe_encodes_advanced_market_data_options_on_the_wire) or test(history_optional_fields_round_trip_and_missing_s2c_is_an_empty_result) or test(market_read_business_errors_keep_opend_return_details) or test(security_info_methods_return_empty_collections_for_a_payload_less_ack) or test(get_kl_missing_s2c_returns_an_empty_result) or test(market_read_methods_reject_a_disconnected_session) or test(stale_or_malformed_push_updates_never_reach_the_lifecycle) or test(trading_reads_propagate_opend_business_errors) or test(trading_reads_reject_a_disconnected_session) or test(history_trading_reads_return_stable_empty_collections) or test(place_order_requires_an_authenticated_conn_id) or test(place_order_encodes_packet_conn_id_and_projects_server_order_identity) or test(modify_order_encodes_packet_conn_id_and_returns_server_identity) or test(fill_validation_rejects_missing_identity_and_bad_time) or test(history_order_call_uses_history_protocol_and_forwards_filters) or test(subscribe_trade_accounts_forwards_every_account_id) or test(subscribe_trade_accounts_propagates_opend_rejection) or test(trade_write_methods_enforce_prerequisites_and_disconnected_state) or test(place_order_propagates_opend_business_rejection) or test(modify_order_returns_stable_identity_for_an_empty_success_payload) or test(groups_encode_group_type_all_and_project_custom_and_system) or test(members_encode_a_trimmed_group_name_and_project_static_info) or test(a_blank_group_name_is_rejected_before_any_rpc) or test(a_rejected_group_query_surfaces_the_opend_message)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 152 批：Futu margin/transport/watchlist 与 Assistant API route boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/futu/trade_margin_ratio_boundaries_test.go:62,86,124,150` | defensive margin cache clones, rate-limit-only fallback, invalid input failures, duplicate/empty quote requests and projection sorting | P1 | exact：cache age/clone、fallback 分类、输入失败、空/重复请求与排序字段逐项通过。 |
| [x] | `pkg/futu/transport_error_propagation_test.go:13,69,125` | trade read, quote/K-line/order-book and trade-write disconnect propagation | P1 | exact：目标协议断连保持 typed transport failure，读写路径不伪造成功结果。 |
| [x] | `pkg/futu/watchlist_reader_boundaries_test.go:14,49`; `watchlist_reader_integration_test.go:11`; `watchlist_reader_test.go:30,113` | group/member TTL, fresh-read replacement, remote ambiguity/rate-limit and group encoding | P1 | exact：缓存过期、绕过并替换、物理读取限流、成员歧义与 watchlist wire 编码均通过。 |
| [x] | `internal/api/assistant/adk_approval_test.go:335`; `adk_normalize_test.go:15`; `adk_ops_test.go:247`; `adk_routes_test.go:26,214,245,597,787` | ADK read filters/empty arrays, optimization cancellation, approval omission, pagination, chat stream, request timeout and negative route envelopes | P1 | exact：读写路由筛选、空集合 JSON、任务取消、分页边界、流终态与错误 envelope 均有生产测试。 |
| [x] | `internal/api/assistant/adk_sessions_test.go:15`; `chat_stream_lifecycle_test.go:9`; `chat_transport_disconnect_test.go:55`; `input_response_test.go:12`; `routes_boundary_contracts_test.go:178,329`; `routes_payload_pagination_test.go:12,27,65,80` | session CRUD/composer validation, supervisor shutdown, disconnect timing, input retry/error mapping, catalog/status contracts and mutation pagination/lifecycle rules | P1 | exact/partial：持久化 session、shutdown join、499 disconnect、输入重试、路由状态矩阵和 mutation 生命周期均通过；Go 进程内 hub seam 继续按 Rust durable owner 拆分。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 41/41 通过；没有发现需要先红后修的 Rust 生产差异。Assistant stream 的内存 hub 细节保持 partial，未将生产 durable/replay 证据冒充为同形 helper 覆盖。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -p jftrade-broker -p jftrade-api --all-targets --locked -E 'test(cache_returns_cloned_snapshots_only_within_requested_age) or test(margin_ratio_cache_returns_defensive_clones_and_ignores_empty_keys) or test(margin_ratios_fall_back_to_recent_cache_only_for_rate_limit_errors) or test(margin_ratios_surface_invalid_symbol_and_missing_account_input_failures) or test(margin_ratios_use_recent_cache_only_for_rate_limit_errors) or test(normalized_instruments_accepts_empty_symbol_list) or test(basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty) or test(basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent) or test(maps_normalized_requested_rows_and_keeps_the_last_duplicate) or test(tick_candles_report_an_empty_page_when_the_ticker_returns_no_sample) or test(margin_ratio_empty_requests_and_duplicate_symbols_match_go) or test(margin_ratio_unknown_stock_code_extraction_matches_go_boundaries) or test(margin_ratio_projection_sorts_by_symbol_and_reports_market_labels) or test(trade_read_methods_propagate_target_protocol_disconnects) or test(quote_kline_and_order_book_propagate_target_disconnects) or test(trade_write_methods_propagate_write_disconnects) or test(watchlist_reader_cache_expires_at_the_ttl_boundary) or test(watchlist_fresh_read_bypasses_and_replaces_group_and_member_caches) or test(watchlist_fresh_member_read_rechecks_remote_ambiguity) or test(watchlist_reader_rate_limit_applies_only_to_physical_reads) or test(groups_encode_group_type_all_and_project_custom_and_system) or test(watchlist_reader_cache_uses_ttl_and_returns_copies) or test(adk_runs_route_filters_by_status_and_agent_id) or test(adk_read_fixture_preserves_empty_collections_as_json_arrays) or test(optimization_task_http_cancellation_persists_through_production_port_restart) or test(adk_session_detail_omits_resolved_approval_groups) or test(adk_read_pagination_rejects_non_positive_limits_and_negative_offsets) or test(production_live_chat_stream_emits_session_run_and_final_events) or test(adk_provider_save_preserves_request_timeout_ms) or test(adk_session_negative_routes_keep_the_go_error_envelopes) or test(production_adk_public_sessions_use_main_store_and_support_crud) or test(adk_composer_state_truncates_trim_and_rejects_invalid_modes) or test(challenge_continuation_supervisor_shutdown_blocks_for_all_tasks) or test(test_client_disconnect_returns_499_within_250ms) or test(adk_respond_to_input_maps_the_go_error_codes_and_retries) or test(adk_catalog_session_and_observability_success_contracts_hold) or test(adk_session_run_boundary_status_codes_match_the_go_matrix) or test(adk_chat_route_reports_the_go_error_classification) or test(adk_mutation_routes_reject_malformed_mutation_payloads) or test(adk_read_routes_clamp_pagination_beyond_available_items) or test(assistant_run_mutation_routes_enforce_goal_lifecycle_rules)'`

下一片：继续 P1 的 Assistant/API transport 与 trading/broker 映射；Futu 的两个 session registry helper 聚合条目保留待后续边界复核。

## 第 153 批：Assistant/API、Backtest、SQLite maintenance 与 runtime provider boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/assistant/routes_payload_pagination_test.go:134`; `routes_resource_contracts_test.go:295,410`; `routes_test.go:29,180` | missing mutation targets, session/optimization route contracts, stream replay markers, catalog success and timeline legacy error code | P1 | exact/partial：路由 envelope、replay marker 与 legacy error code 均通过；Go hub 侧观测 helper 继续由 durable stream owner 拆分。 |
| [x] | `internal/api/backtest/routes_boundaries_test.go:78`; `routes_progress_test.go:97`; `internal/api/httpserver/bindings_boundaries_test.go:27` | session-scope validation, backtest read/write fixture and candle period normalization | P1 | exact：旧 session scope、终态/store failure、写入 fixture 与 period 空值/别名边界逐项通过。 |
| [x] | `internal/api/middleware/auth_test.go:128,153`; `internal/api/settings/routes_test.go:350`; `internal/api/strategy/routes_boundary_contracts_test.go:142`; `routes_lifecycle_test.go:611` | origin/CSRF method matrix, managed-account not-found, activity pagination/time filters | P1 | exact：认证写入方法、资源缺失映射和 activity 分页/时间过滤均有 Rust transport/engine 证据。 |
| [x] | `internal/api/trading/execution_test.go:148`; `routes_read_handlers_test.go:18`; `internal/api/watchlist/routes_business_test.go:51,281` | execution mutation fixture, broker empty-array serialization, watchlist read/write pagination/conflict/body validation | P1 | exact/partial：执行写入、空集合 wire 与 watchlist route contract 通过；Go handler 聚合计数由对应 product/store owner 拆分。 |
| [x] | `internal/app/apiserver/application/installers_test.go:49`; `datamigration/maintenance_failure_paths_test.go:341,552`; `rebuild_safety_test.go:177`; `desktop_api_startup_test.go:106` | startup rollback/readiness reclamation, backup/cleanup metrics, rebuild lease rollback and desktop readiness | P1 | exact：启动回滚、维护路径、lease 回滚与 readiness fail-closed 逐项通过。 |
| [x] | `internal/app/apiserver/futuapp/coordinator_test.go:39,69`; `runtime_contracts_test.go:14,58`; `runtime_probe_contracts_test.go:10`; `runtime_state_boundaries_test.go:26`; `lifecycle/lifecycle_test.go:691`; `marketdataapp/data_plane_switch_test.go:40` | OpenD health diagnostics, account reconciliation, probe/version state, shutdown lock ordering and provider generation/cache recovery | P1 | exact/partial：health/probe、reconciliation、shutdown 与 provider generation fence 有真实 owner；旧 coordinator 聚合 callback 细节保持 partial。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 45/45 通过；没有发现需要先红后修的 Rust 生产差异。Assistant hub/coordinator 聚合 helper 继续保持 partial，未把跨 owner 组合测试升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(adk_missing_mutation_targets_keep_the_go_classification) or test(adk_session_run_and_optimization_route_contracts_match_go) or test(adk_stream_reconnect_routes_carry_replay_markers_and_fail_closed) or test(replayed_stream_payloads_carry_the_go_replay_marker) or test(catalog_session_run_and_observability_routes_answer_ok) or test(session_timeline_failure_keeps_the_legacy_messages_error_code) or test(sync_request_session_scope_parity_with_go) or test(backtests_write_fixture_replays_all_four_go_owned_mutations) or test(backtests_read_routes_match_group_fixture_in_cutover_only) or test(candle_route_treats_blank_period_as_unset_and_rejects_unsupported) or test(candle_period_normalizes_aliases_and_rejects_unsupported) or test(auth_requires_origin_and_csrf_for_session_writes) or test(auth_treats_patch_as_session_write_requiring_csrf) or test(managed_account_write_routes_map_missing_records_to_not_found) or test(broker_settings_writes_replay_frozen_compatibility_cases) or test(catalog_activity_paging_and_filters_match_go_boundaries) or test(strategy_instance_read_routes_match_group_fixture_in_cutover_only) or test(execution_write_fixture_replays_all_seven_go_owned_mutations) or test(execution_write_product_replays_browser_boundary_failure_recovery_and_restart) or test(production_http_broker_reads_serialize_empty_collections_as_arrays) or test(watchlist_read_pages_preserve_filters_groups_sources_and_remote_catalog) or test(watchlist_write_fixture_matches_go_owner_for_all_eight_routes) or test(malformed_membership_and_commit_bodies_are_rejected) or test(watchlist_read_routes_match_group_fixture_in_cutover_only) or test(test_product_runtime_startup_failure_rollback) or test(readiness_failure_reclaims_every_started_process_without_starting_dependents) or test(startup_failure_restores_previously_migrated_descriptor_files) or test(backup_snapshot_rejects_an_unusable_backup_directory_and_an_empty_source_path) or test(cleanup_and_compaction_report_the_reclaimed_bytes_measured_on_disk) or test(a_held_lease_on_one_database_rolls_back_the_batch_rebuild_locks) or test(runtime_readiness_allows_external_degradation_but_rejects_incomplete_startup) or test(production_opend_health_diagnoses_enabled_but_unreachable_opend) or test(production_system_read_reports_unavailable_opend_without_fake_health) or test(market_data_health_requires_a_known_logged_in_quote_session) or test(helper_market_data_providers_reconcile_futu_account_order_fill_and_fee) or test(reconciliation_discovers_external_order_into_empty_ledger) or test(production_opend_health_rejects_old_build_and_guides_upgrade) or test(probe_opend_reports_closed_port_as_disconnected) or test(probe_from_global_state_enforces_minimum_version_and_maps_neutral_state) or test(disconnected_probe_keeps_its_transport_error_for_manual_retry) or test(web_shutdown_does_not_hold_runtime_lock_while_joining_server) or test(provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update) or test(cache_rejects_stale_generation_and_classifies_freshness)'`

下一片：继续 P1 的 API transport、Trading/Broker execution 与 Storage/Settings 映射。

## 第 154 批：MarketData runtime、sidecar、auth/session 与 strategy lifecycle（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/marketdataapp/market_depth_test.go:306`; `runtime_test.go:168,191,476`; `sidecar_os_process_test.go:14,115,146`; `sidecar_process_test.go:15`; `unavailable_provider_test.go:72` | empty depth, provider switch/rollback queue, sidecar process reuse/stop escalation and unknown-provider guards | P1 | exact/partial：runtime/sidecar ownership、停止升级、provider generation 与 fail-closed guard 均通过；Go cleanup retry 计数保持 partial。 |
| [x] | `internal/app/apiserver/runtime/dependencies_test.go:60,141`; `runtimes/handle_lifecycle_test.go:417`; `server_test.go:518` | runtime dependency candidate/status aggregation, concurrent product close and API shutdown lock ordering | P1 | exact：依赖 fallback、required 状态、并发 close 与 shutdown join 逐项通过。 |
| [x] | `internal/app/apiserver/servercore/desktop_token_test.go:101`; `product_lifecycle_closure_test.go:206`; `runtime_integration_boundaries_test.go:106`; `runtime_observation_test.go:176`; `runtime_trading_test.go:443` | desktop auth session route, execution order normalization, Futu trade session switch, strategy panic recovery and tracked cancel | P1 | exact/partial：auth/order/provider/strategy lifecycle 有真实 owner；旧 servercore facade 的 callback 聚合保持 partial。 |
| [x] | `internal/app/apiserver/servercore/security_test.go:43`; `server_application_lifecycle_test.go:37`; `server_bootstrap_boundaries_test.go:15`; `server_test.go:36`; `server_warmup_test.go:20`; `settings_security_test.go:95` | web disable/auth session durability, ordered runtime shutdown, degraded startup, calendar route, strategy warmup and desktop-only settings guard | P1 | exact：安全、启动降级、关闭顺序、calendar/warmup 与 browser settings fence 均通过。 |
| [x] | `internal/app/apiserver/servercore/strategy_runtime_workflow_replay_test.go:55`; `system_reconcile_strategy_states_test.go:11`; `trading_order_cancellation_contracts_test.go:98,134`; `servercoretest/broker_new_test.go:155,181` | stale quote cache, strategy state reconcile/recovery, cancel validation/persistence and broker disconnected empty states | P1 | exact：stale cache、策略重启收敛、取消 fail-closed 与 broker empty/failure shape 均有证据。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 37/37 通过；没有发现需要先红后修的 Rust 生产差异。sidecar cleanup 计数和旧 servercore 聚合回调保持 partial，不把拆分 owner 证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(depth_read_returns_empty_arrays_for_empty_s2c_lists) or test(switch_retires_the_previous_provider_before_publishing_the_next_one) or test(unsubscribe_retry_ladder_escalates_and_reacquire_clears_retry_state) or test(queued_transitions_observe_the_committed_provider_as_their_previous_owner) or test(start_rejects_a_second_launch_of_a_live_process) or test(stop_escalates_to_kill_when_the_child_outlives_the_grace_period) or test(managed_process_is_reused_until_an_explicit_stop_releases_it) or test(unknown_provider_activation_is_rejected_without_touching_the_active_selection) or test(activation_after_shutdown_is_rejected_and_leaves_the_snapshot_committed) or test(node_candidates_prefer_path_then_macos_common_installs) or test(runtime_dependencies_flag_unsatisfied_required_entries) or test(stop_product_is_idempotent_across_concurrent_invocations) or test(web_shutdown_does_not_hold_runtime_lock_while_joining_server) or test(auth_session_route_matches_go_fixture_in_cutover_only) or test(test_normalize_execution_order_preserves_broker_abstraction) or test(provider_switch_does_not_disconnect_an_existing_futu_trade_session) or test(runtime_exit_converges_to_stopped_with_audit_notification_and_error_log) or test(test_execute_strategy_intents_cancel_dispatches_order_cancel) or test(disabled_web_access_does_not_start_a_listener) or test(auth_sessions_are_cookie_bound_hashed_and_restart_durable) or test(test_product_runtime_ordered_shutdown_explicit) or test(production_startup_exposes_degraded_api_when_external_runtimes_are_unavailable) or test(earnings_calendar_route_maps_frontend_keys) or test(strategy_definition_preview_derives_warmup_bars_and_overrides_preview_parameters) or test(browser_authenticated_request_cannot_change_desktop_only_security_settings) or test(cached_projection_does_not_promote_stale_overnight_during_regular_hours) or test(startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances) or test(recovery_failure_converges_the_running_instance_to_stopped) or test(cancel_rejects_missing_terminal_and_unidentified_persisted_orders) or test(accepted_cancel_persists_and_broker_failure_never_advertises_a_cancel) or test(broker_klines_valid_request_fails_closed_without_historical_source) or test(broker_securities_returns_real_empty_result_when_cache_has_no_symbol)'`

下一片：继续 P1 的 Trading/Broker execution、API transport 与 Storage/Settings 映射。

## 第 155 批：Broker/Execution routes、auth/frontend wire 与 store lifecycle（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/servercoretest/broker_new_test.go:237,264,336,359`; `broker_routes_test.go:14` | broker read/write disconnected shapes, unlock forwarding, cancel validation and funds missing-field projection | P1 | exact：断连空集合、unlock 参数、取消 fail-closed 与 funds nullable 字段逐项通过。 |
| [x] | `internal/app/apiserver/servercoretest/exec_routes_test.go:18`; `exec_validate_test.go:88`; `frontend_test.go:25,198` | execution write fixture, US session/price normalization and frontend SPA/API fallback plus shutdown | P1 | exact：执行写入、US session、tick rounding、SPA fallback 与 shutdown port release 均有证据。 |
| [x] | `internal/app/apiserver/servercoretest/installers_degraded_test.go:13`; `portfolio_routes_test.go:12,43,71`; `settings_broker_test.go:120`; `strategy_logs_test.go:169`; `status/status_test.go:88` | degraded startup, portfolio fallback/removed route, settings null integration, strategy read fixture and status wire normalization | P1 | exact/partial：启动、portfolio/status/settings wire 与 strategy route 均通过；Go handler aggregation 保持 partial。 |
| [x] | `internal/app/apiserver/stores/handle_test.go:10,42`; `strategyapp/runtime_ports_test.go:145`; `tradingapp/execution_gateway_lifecycle_test.go:202,250,298` | reverse store close/rollback, execution command recovery, cancel/combo validation and server identity | P1 | exact：store lease order、回滚、执行写入恢复、combo/cancel 边界逐项通过。 |
| [x] | `internal/app/apiserver/tradingapp/order_update_source_test.go:88,172`; `order_updates_test.go:20,114`; `webaccess/auth_boundaries_test.go:235,319`; `webaccess/frontend_test.go:28` | reconciliation provider/fund scope, lifecycle persistence, auth corruption/origin rejection and desktop static fallback | P1 | exact：对账 scope、订单生命周期、auth fail-closed 与 frontend 资源 fallback 均有真实 owner。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 38/38 通过；没有发现需要先红后修的 Rust 生产差异。旧 handler 聚合和 callback 计数继续保持 partial，不把 product/store 拆分测试升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(broker_securities_returns_real_empty_result_when_cache_has_no_symbol) or test(broker_quote_projects_real_futu_tick_cache_for_all_symbols) or test(trade_write_methods_propagate_write_disconnects) or test(broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend) or test(cancel_rejects_missing_terminal_and_unidentified_persisted_orders) or test(funds_projection_keeps_missing_margin_fields_absent) or test(broker_read_fails_closed_without_trade_client) or test(execution_write_fixture_replays_all_seven_go_owned_mutations) or test(accepted_cancel_persists_and_broker_failure_never_advertises_a_cancel) or test(us_limit_route_orders_keep_price_session_and_explicit_market_code) or test(test_normalize_execution_order_supports_extended_us_limit_sessions) or test(place_order_rounds_us_prices_to_the_venue_tick_before_encoding) or test(unknown_api_is_json_but_frontend_uses_spa_fallback) or test(direct_product_shutdown_stops_reconciliation_before_releasing_leases) or test(enabled_web_access_binds_and_shutdown_releases_port) or test(production_startup_fails_closed_when_database_is_corrupted) or test(production_startup_exposes_degraded_api_when_external_runtimes_are_unavailable) or test(portfolio_cash_balances_fall_back_to_summary_currency_when_breakdown_is_empty) or test(unknown_portfolio_reconciliation_route_returns_json_not_found) or test(portfolio_read_routes_match_group_fixture_in_cutover_only) or test(unsaved_broker_settings_expose_null_integration_and_go_defaults) or test(strategy_instance_read_routes_match_group_fixture_in_cutover_only) or test(runtime_status_wire_drops_absent_and_blank_values_and_normalizes_utc) or test(test_product_runtime_ordered_shutdown_explicit) or test(startup_acquires_all_writer_leases_in_stable_order_before_migrating) or test(startup_failure_restores_previously_migrated_descriptor_files) or test(test_product_runtime_startup_failure_rollback) or test(execution_write_product_replays_browser_boundary_failure_recovery_and_restart) or test(combo_place_rejects_invalid_legs_accounts_and_hidden_transport_failures) or test(combo_intent_rejects_missing_kind_legs_and_account) or test(option_combo_preview_place_and_cancel_keep_server_identity) or test(helper_market_data_providers_reconcile_futu_account_order_fill_and_fee) or test(reconciliation_replays_history_fill_and_fee_once_after_restart) or test(reconciliation_scope_accepts_only_stock_trade_markets) or test(execution_orders_lifecycle_events_and_restart_durability) or test(auth_session_store_corruption_fails_closed_without_rewrite) or test(auth_rejects_untrusted_origin_before_authentication) or test(desktop_missing_static_assets_do_not_use_spa_fallback)'`

下一片：继续 P1 的 API transport、Trading/Broker execution、Storage/Settings 与 Assistant workflow 映射。

## 第 156 批：Web auth/session 与 Assistant assembly、workflow、approval lifecycle（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/webaccess/security_integration_test.go:210,293,352,379,490` | auth manager login/logout, cookie/CSRF flow, password/session durability, logout and WebSocket cookie upgrade | P1 | exact/partial：cookie/session/CSRF 与 WebSocket transport 均通过；HTTPS proxy 的旧 handler cookie 属性聚合保持 partial。 |
| [x] | `internal/assistant/assembly/adk_strategy_test.go:194,605`; `application_adapter_test.go:226`; `mcp_server_test.go:76,106` | strategy/backtest tool validation, optimize task persistence, screen catalog/cancel and MCP lifecycle/token catalog | P1 | exact：工具输入边界、任务持久化、screen catalog 与 MCP listener/token contract 逐项通过。 |
| [x] | `internal/assistant/portfolio_tools_test.go:15,155`; `product_adapters_test.go:186`; `assembly/runtime_test.go:14` | portfolio ranking/discovery/partial states, screen schema guard and runtime start/stop idempotency | P1 | exact/partial：portfolio/tool catalog、schema 校验与 runtime 生命周期有真实 owner；Go facade 注入细节保持 partial。 |
| [x] | `internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103`; `workflow_tools_test.go:86`; `engine/adk22regression/native_runtime_test.go:155` | malformed workflow writes, bounded wait/deadline/cancellation and cancellation registry fencing | P1 | exact：workflow payload、wait deadline/cancel 与 cancellation fan-out 均通过。 |
| [x] | `internal/assistant/engine/adk_edges_test.go:416,532`; `adk_runner_edges_test.go:142`; `adk_store_edges_test.go:11,82,404` | session context/provider fallback, missing agent/provider, run gate, pressure read, store corruption/delete and composer validation | P1 | exact：context/provider guard、run gate、store fail-closed 与 composer schema 均有 Rust durable owner。 |
| [x] | `internal/assistant/engine/approval_persistence_failures_test.go:75`; `approval_reconciliation_lifecycle_test.go:9,65`; `approval_retry_sibling_cancellation_test.go:14` | stale embedded approval restaging, orphan startup reconcile, degraded resume and sibling approval cancellation | P1 | exact：approval durable CAS/reconcile、恢复降级与 sibling cancellation 逐项通过。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 34/34 通过；没有发现需要先红后修的 Rust 生产差异。旧 facade 注入与 HTTPS proxy cookie 聚合继续保持 partial。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(auth_manager_login_validate_and_logout_flow) or test(session_cookie_reads_and_csrf_protected_writes_share_one_browser_flow) or test(auth_sessions_are_cookie_bound_hashed_and_restart_durable) or test(auth_session_route_matches_go_fixture_in_cutover_only) or test(ws_live_transport_upgrades_with_a_browser_session_cookie_alone) or test(backtest_and_strategy_tools_reject_missing_identifiers_and_unknown_targets) or test(strategy_optimize_enqueues_every_candidate_and_persists_the_task) or test(screen_catalog_normalizes_padded_lowercase_markets_and_rejects_unsupported_labels) or test(production_backtest_cancel_reports_false_without_a_cancellable_run) or test(disabled_runtime_has_stopped_status_and_releases_listener) or test(token_auth_and_tools_list_use_reviewed_catalog) or test(test_portfolio_funds_overview_and_sorting_and_unsupported_market) or test(portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers) or test(portfolio_layered_tools_report_discovery_failure_and_partial_read_states) or test(portfolio_tools_require_trading_environment_and_fail_closed_without_a_broker_reader) or test(screen_query_rejects_wrong_catalog_and_schema_versions) or test(product_runtime_without_optional_workers_starts_and_stops_cleanly) or test(test_product_runtime_ordered_shutdown_explicit) or test(test_product_runtime_ordered_shutdown_direct_drop) or test(workflow_writes_reject_malformed_canvas_graphs_before_storing) or test(workflow_wait_tool_rejects_too_long_duration) or test(workflow_wait_tool_returns_context_cancellation) or test(cancellation_registry_fans_out_and_unregisters_exact_token) or test(session_context_overrides_fall_back_to_the_agent_provider_window) or test(chat_resolution_reports_missing_agents_and_unusable_providers) or test(run_gate_is_shared_across_runtime_facades) or test(session_context_read_reports_pressure_without_compacting) or test(adk_store_rejects_missing_drifted_and_corrupted_go_databases) or test(adk_composer_state_truncates_trim_and_rejects_invalid_modes) or test(adk_approval_resolution_restages_a_stale_embedded_approval) or test(orphaned_pending_approval_runs_are_failed_on_startup_reconcile) or test(infrastructure_resume_error_degrades_until_a_successful_scan) or test(sibling_approvals_resume_once_after_every_decision)'`

下一片：继续 P1 的 Assistant engine、workflow/ADK 与 API transport 映射。

## 第 157 批：Assistant context、input continuation、claims 与 SQLite session persistence（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/engine/approval_state_guard_test.go:114`; `context_cache_test.go:61,116`; `event_projection_boundaries_test.go:14,72,87` | timeout restart, compacted context prefix/handoff, tool/session projection and missing-session read pressure | P1 | exact：timeout window、context compaction、projection ordering 与 missing-session pressure 均通过。 |
| [x] | `internal/assistant/engine/exec_bounds_test.go:148`; `exec_state_bounds_test.go:9`; `execution_claims_test.go:265`; `execution_state_projection_contracts_test.go:175`; `handoff_notice_test.go:100` | cancellation join, run projection, claim fencing/takeover, recovery degradation and normalized notices | P1 | exact：cancellation、claims lease、recovery readiness 与 notice identity 逐项通过。 |
| [x] | `internal/assistant/engine/input_continuation_failure_recovery_test.go:13,110,140`; `input_request_test.go:490`; `lifecycle_reconciliation_failures_test.go:9,102`; `normalize_test.go:62` | input failure/parent projection/crash recovery, late-answer rejection, stale-run reconcile and ADK task normalization | P1 | exact：input continuation、late answer、run reconcile 和 normalization 均有 durable owner 证据。 |
| [x] | `internal/assistant/engine/persistence/composer_normalize_test.go:8`; `persistence/execution_claims_test.go:93`; `google_artifact_test.go:420`; `session_sqlite_boundaries_test.go:14`; `session_sqlite_schema_test.go:15,49,82,127`; `session_sqlite_test.go:30,58` | composer modes, serialized claims, artifact paths, SQLite session schema/migration/reopen/lifecycle | P1 | exact：SQLite schema health、migration preservation、artifact path、claim serialization 与 composer validation 均通过。 |
| [x] | `internal/assistant/engine/persistence_failure_boundaries_test.go:162` | failed recovery scan degrades readiness until a later successful scan | P1 | exact：持久化恢复失败的可见性与恢复条件有明确测试。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 26/26 通过；没有发现需要先红后修的 Rust 生产差异。Go completion-review tool request helper 没有同名 Rust test，保持未验证并留待后续边界审查，不计入本批 30 条有效映射。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window) or test(compacted_context_survives_restart_and_precedes_current_user_message) or test(context_compaction_shrinks_the_projected_session_view) or test(a_tool_round_projects_the_pre_tool_reply_and_session_timeline) or test(session_context_read_reports_pressure_without_compacting) or test(chat_creates_the_session_once_and_reuses_its_stored_title) or test(a_cancelled_tool_call_reports_the_context_cancellation) or test(the_run_projection_derives_tool_summaries_optimization_task_and_usage_totals) or test(stale_replay_safe_claim_takes_over_with_fencing) or test(expired_tool_invocation_takeover_fences_old_ticket_with_live_run_lease) or test(infrastructure_resume_error_degrades_until_a_successful_scan) or test(context_notices_are_best_effort_and_keep_one_identity_across_updates) or test(an_unrecoverable_input_continuation_fails_the_run_with_the_reference_resume_state) or test(input_response_payload_anchors_the_resumed_run_to_the_original_request) or test(a_restarted_runtime_resumes_a_pending_input_run) or test(cancelling_a_pending_input_run_cancels_the_request_and_rejects_a_late_answer) or test(expired_running_run_is_reconciled_to_timed_out_with_failed_tool_calls) or test(orphaned_pending_approval_runs_are_failed_on_startup_reconcile) or test(adk_task_normalization_and_validation_match_go) or test(adk_composer_state_truncates_trim_and_rejects_invalid_modes) or test(adk_store_fences_a_second_writer_and_serializes_concurrent_access) or test(paths_follow_go_environment_overrides_and_adk_artifact_lifecycle) or test(adk_session_store_rejects_missing_drifted_and_corrupted_go_databases) or test(adk_v2_migration_rebuilds_runs_without_losing_payload) or test(adk_session_store_lifecycle_and_restart_durability) or test(failed_recovery_scan_marks_readiness_degraded_until_next_success)'`

下一片：继续 P1 的 Assistant engine persistence/recovery、workflow/ADK 与 API transport 映射。

## 第 158 批：Assistant approval resume、runner lifecycle 与 runtime store（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/engine/persistence_failure_boundaries_test.go:216,266,311`; `projection_canvas_memory_contracts_test.go:18`; `providers/responses_model_test.go:112` | approval persistence CAS/restage, stale writer cascade deletion, tool/session projection and terminal failure mapping | P1 | exact：approval durable failure、cascade fencing、projection 与 terminal mapping 均通过。 |
| [x] | `internal/assistant/engine/resumed_execution_recovery_boundaries_test.go:14,92,111`; `run_timeline_test.go:8`; `runner_approval_concurrency_test.go:191`; `runner_chat_continuation_signal_test.go:52` | resumed approval recovery, blank session identifier guard, cancellation signal and nonterminal stream replay | P1 | exact：resume/recovery、输入 guard、取消与 replay marker 均有真实 owner。 |
| [x] | `internal/assistant/engine/runner_chat_test.go:891,1043,1124`; `runner_continuation_boundaries_test.go:87,406`; `runner_goal_test.go:111,174` | closed-stream approval resume, session reuse/mismatch, terminal cancel, missing-run no-op, supervisor stopping and expired-run reads | P1 | exact：runner 生命周期、session reuse、终态 cancel 与 expired reconcile 逐项通过。 |
| [x] | `internal/assistant/engine/runner_lifecycle_reconciliation_test.go:9,70,106,146`; `runner_lifecycle_shutdown_failures_test.go:8`; `runtime_execution_lease_boundaries_test.go:108,176` | stale-run reconciliation, child/approval cleanup, self-reference repair, storage shutdown and execution lease fencing | P1 | exact：reconcile/repair、shutdown fail-closed 与 lease cancellation/claims 均通过。 |
| [x] | `internal/assistant/engine/runtime_store_test.go:36,286,409`; `session_compaction_boundaries_test.go:12`; `session_context_conflict_test.go:11` | runtime store agent/session ordering, provider snapshot/delete guards and compaction gate conflict | P1 | exact：runtime store、provider/session fail-closed 与 compaction gate 均有 durable 测试。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 28/28 通过；没有发现需要先红后修的 Rust 生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(adk_approval_resolution_stages_continuation_and_denial_cas) or test(adk_approval_resolution_restages_a_stale_embedded_approval) or test(cascade_deletion_fences_stale_writer_and_session_recovery) or test(a_tool_round_projects_the_pre_tool_reply_and_session_timeline) or test(terminal_failure_mapping_matches_the_reference_table) or test(a_resumed_approval_run_completes_with_the_confirmation_resolved_state) or test(an_approval_resuming_run_is_recovered_after_a_runtime_restart) or test(adk_read_routes_reject_blank_decoded_identifiers) or test(challenge_continuation_supervisor_cancellation_signal_propagates) or test(running_stream_payload_replays_all_nonterminal_events) or test(production_approval_resume_after_the_stream_closed_completes_without_late_frames) or test(chat_creates_the_session_once_and_reuses_its_stored_title) or test(a_cancelled_run_audits_run_cancelled_and_terminates_once) or test(a_continuation_for_a_missing_run_is_a_silent_no_op) or test(challenge_continuation_supervisor_rejects_new_work_once_stopping) or test(the_run_read_routes_reconcile_expired_runs_before_serving) or test(cancellation_registry_fans_out_and_unregisters_exact_token) or test(non_resumable_running_run_does_not_make_runtime_unready) or test(orphaned_pending_approval_runs_are_failed_on_startup_reconcile) or test(user_goal_pause_fields_survive_a_stale_writer_and_clear_on_explicit_resume) or test(graph_exposes_one_deterministic_ready_task) or test(failed_recovery_scan_marks_readiness_degraded_until_next_success) or test(challenge_continuation_supervisor_concurrent_spawn_shutdown_race) or test(challenge_continuation_supervisor_claims_are_exclusive_and_released) or test(production_adk_updates_do_not_create_missing_agents_or_providers) or test(snapshot_and_provider_test_boundaries_fail_closed) or test(adk_session_delete_missing_is_reported_with_the_session_error_code) or test(a_second_compaction_is_rejected_while_the_session_gate_is_held)'`

下一片：继续 P1 的 Assistant runner/session context、workflow/ADK 与 API transport 映射。

## 第 151 批：Futu/OpenD session resolver、snapshot fallback 与 margin/trade 读取（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/futu/kline_session_registry_boundaries_test.go:114`; `market_mapping_order_quantity_test.go:58`; `quote_snapshot_test.go:217,258` | calendar-resolved US session labels, broker lot/buying-power guards and previous-close session conditions | P1 | exact：session/window projection、数量边界与 previous-close 条件矩阵逐项通过。 |
| [x] | `pkg/futu/read_account_test.go:37`; `security_snapshot_coordinator_test.go:17,83,120,189`; `security_snapshot_error_classification_test.go:71` | account/market fallback, snapshot cache batching/sliding budget/rate-limit classification and transport error scoping | P1 | exact/partial：缓存、预算、限流、空结果和 symbol-scoped error 均有断言；Go cancellation 聚合仍由 typed coordinator owner 拆分。 |
| [x] | `pkg/futu/snapshot_fallback_parsing_test.go:14,93,146`; `snapshot_fallback_test.go:18,58,98,153` | delayed quote fields, stock-screen wire coercion/row parsing, static-id fallback, negative cache and screen-error propagation | P1 | exact：fallback 请求字段、行与 market 分组、复制/取消、无订阅静态回退、负缓存与错误报告均通过。 |
| [x] | `pkg/futu/stream_connection_quote_boundaries_test.go:22,115,317`; `subscription_lifecycle_test.go:80,162,194,236` | runtime worker shutdown, malformed quote push filtering, reconnect resync, subscription confirmation, generation fencing and terminal close | P1 | exact：worker join、push fail-closed、reconnect/resync、ack 后 active、代际 replay 与 terminal close 均有真实 owner 证据。 |
| [x] | `pkg/futu/time_normalization_test.go:10,60,79`; `trade_account_test.go:72`; `trade_bounds_test.go:92`; `trade_margin_ratio_boundaries_test.go:13` | market-time normalization, trade-market fallback, max quantity request fields and margin-ratio typed recovery/throttle | P1 | exact：时区/市场 fallback、可选字段与 margin unknown/throttle 结果逐项通过。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 40/40 通过；没有发现需要先红后修的 Rust 生产差异。Go snapshot cancellation 聚合继续保持 partial，未将拆分 owner 证据提升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine -p jftrade-broker --all-targets --locked -E 'test(resolver_projects_us_early_close_extended_window_with_timezone) or test(us_intraday_futu_candles_carry_calendar_resolved_session_labels) or test(broker_lot_size_initializes_minimum_and_step_quantity) or test(buying_power_requires_opend_trade_reader_and_forwards_max_quantity_query) or test(previous_close_condition_switches_on_session_type) or test(previous_close_condition_does_not_rewrite_non_us_unknown_sessions) or test(resolve_account_honors_requested_authority_and_falls_back_like_go) or test(security_snapshot_coordinator_caches_clones_and_uses_market_batches) or test(security_snapshot_coordinator_enforces_sliding_budget_and_does_not_cache_failures) or test(security_snapshot_coordinator_classifies_remote_rate_limit) or test(security_snapshot_coordinator_handles_empty_and_unexpected_results) or test(symbol_scoped_snapshot_errors_are_detectable_through_context) or test(closed_ready_session_is_replaced_on_peer_close) or test(stock_screen_fallback_wire_value_helpers_match_go_coercions) or test(stock_screen_fallback_parses_rows_and_market_groups) or test(stock_screen_fallback_cancellation_and_adapter_entry_point) or test(stock_screen_fallback_coordinates_copies_and_errors) or test(snapshot_fallback_params_use_strict_delayed_quote_fields) or test(futu_stock_screen_snapshot_fallback_uses_static_ids_without_subscription) or test(stock_screen_snapshot_coordinator_caches_rows_and_negative_results) or test(futu_stock_screen_snapshot_fallback_reports_screen_errors) or test(runtime_shutdown_cancels_then_joins_its_worker) or test(basic_quote_pushes_drop_rows_without_a_usable_security_or_price) or test(provider_reconnect_publishes_resync_events_and_wakes_reconciliation) or test(coordinator_close_is_terminal_and_prevents_orphaned_reconnect) or test(failed_or_replayed_subscriptions_are_not_active_until_opend_confirms) or test(closed_session_generation_invalidates_its_subscriptions_and_requires_replay) or test(failed_connection_does_not_advance_the_established_session_generation) or test(hk_lunch_snapshot_keeps_previous_close_and_does_not_mark_extended_hours) or test(history_query_converts_supported_trade_markets_to_local_wall_clock) or test(history_fill_call_uses_history_protocol_and_forwards_time_filter) or test(history_query_converts_hk_rfc3339_to_local_wall_clock) or test(history_fill_read_projects_fill_identity) or test(resolve_trade_market_covers_requested_and_fallback_branches_like_go) or test(broker_read_projects_max_trade_quantity_snapshot) or test(max_trade_quantity_call_preserves_serial_and_projects_optional_fields) or test(margin_ratio_unknown_stock_code_extraction_matches_go_boundaries) or test(margin_ratio_unknown_stock_recovery_keeps_nil_security_rows_and_code_forms) or test(margin_ratio_server_throttling_maps_to_the_typed_rate_limit) or test(margin_ratio_wire_throttling_reaches_the_typed_rate_limit_variant)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 150 批：Futu/OpenD quote rights、research、snapshot 与交易边界（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/futu/account_fill_test.go:71`; `adapter_advanced_test.go:14,80,212`; `adapter_capability_runtime_test.go:202,313` | fill projection ordering, strict advanced request defaults/page limits, HK-only high-dividend state, quote-right product states and reconnect failure cache | P1 | exact：成交排序、OpenD 请求默认值/上限、市场范围校验与 quote-right generation/cache 行为逐项通过。 |
| [x] | `pkg/futu/adapter_failure_boundaries_test.go:115,165`; `adapter_kline_pagination_test.go:342` | history session fallback, kline time adjustment/period catalog, helper price bounds and unavailable OpenD propagation | P1 | exact/partial：底层 session、period、价格 helper 与不可用 provider 错误均有证据；Go canceled-context 保留由 Rust typed transport owner 拆分，保持 partial。 |
| [x] | `pkg/futu/adapter_prediction_boundaries_test.go:78`; `adapter_prediction_stream_test.go:139`; `adapter_research_contract_test.go:12,237,314` | prediction fallback/query validation and pagination, strict research requests, local pagination, economic-calendar hasMore/empty rows | P1 | exact：预测/研究协议字段、分页边界、空行与显式 hasMore 均通过；组合测试中的跨 adapter 聚合仍按已有 owner 拆分。 |
| [x] | `pkg/futu/advanced_product_adapter_contracts_test.go:262`; `exchange_business_boundary_test.go:18`; `exchange_kline_test.go:479`; `exchange_mapping_boundaries_test.go:127` | broker-neutral security identity, currency-row cash precedence, generation-fenced quote polling/reconnect and historical session fallback | P1 | exact/partial：security/cash/session fallback 与 reconnect cache 有真实 owner 断言；Go BBGO session registry 的 helper 级聚合保持 partial。 |
| [x] | `pkg/futu/exchange_quote_request_boundaries_test.go:16,50,110`; `exchange_session_test.go:12`; `exchange_test.go:691,885,974`; `exchange_trade_price_test.go:13`; `kline_session_registry_boundaries_test.go:12,56` | account push/empty symbol guards, basic quote and snapshot normalization, stale extended-session filtering, margin cache, cancel write, account replay, market fallback and kline subscription/session fences | P1 | exact：请求前置校验、空载荷、快照过滤、缓存 TTL、取消写入、账号重放与 session 代际边界逐项通过。 |
| [x] | `pkg/futu/live_opend_test.go:251`; `pkg/futu/quote_snapshot_test.go:49` | delayed stock-screen snapshot fallback without subscription and closed-session previous-close projection | P1 | exact：延迟 snapshot fallback 使用静态信息分页且不建立订阅，闭市 previous-close 投影保持一致。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 44/44 通过（42 条 Futu/engine 过滤测试 + 2 条补充过滤测试）；没有发现需要先红后修的 Rust 生产差异。Go canceled-context、BBGO session registry 等聚合/helper 机制继续保持 `partial`，不把共享或拆分证据提升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked -E 'test(fills_projection_sorts_filled_at_desc_with_id_tiebreak) or test(history_fill_read_projects_fill_identity) or test(advanced_feature_defaults_build_strict_opend_requests) or test(advanced_page_size_respects_protocol_limits) or test(high_dividend_state_is_hk_only) or test(product_states_match_go_quote_right_table) or test(entitlement_failure_is_cached_and_refreshed_after_reconnect) or test(fallback_requires_the_same_route_and_an_unsupported_marker) or test(test_adjust_kline_time_semantics) or test(helper_boundaries_return_the_input_unchanged) or test(broker_adapter_forwards_unavailable_opend_errors_on_every_read_route) or test(candle_period_catalog_rejects_missing_and_unmappable_intervals) or test(combo_rfq_creates_the_quote_once_and_never_replays_after_a_transport_failure) or test(prediction_push_handlers_install_once_and_fail_closed_on_invalid_demand) or test(prediction_catalog_pagination_and_identity_follow_the_go_rules) or test(catalog_operations_build_strict_opend_requests_like_go) or test(local_pagination_never_leaks_into_the_opend_request) or test(local_pagination_walks_plate_and_catalog_windows) or test(economic_calendar_pagination_honors_explicit_has_more_and_empty_rows) or test(payload_envelope_picks_the_first_sorted_entry_list_like_go) or test(futu_securities_route_projects_broker_neutral_envelope_boundary) or test(portfolio_cash_balances_prefer_currency_rows_over_summary_fallback) or test(public_coordinator_polls_basic_quotes_into_a_generation_fenced_cache) or test(subscribe_trade_accounts_propagates_opend_rejection) or test(executor_rejects_invalid_instrument_and_unsupported_interval) or test(market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error) or test(basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty) or test(maps_normalized_requested_rows_and_keeps_the_last_duplicate) or test(basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent) or test(snapshot_reader_rejects_a_present_s2c_with_no_mappable_rows) or test(snapshot_reader_keeps_mappable_rows_and_drops_invalid_ones) or test(snapshot_reader_rejects_invalid_symbols_before_any_opend_call) or test(snapshot_reader_keeps_a_payload_less_ack_as_an_empty_collection) or test(cached_projection_does_not_promote_stale_overnight_during_regular_hours) or test(holiday_snapshot_stays_closed_with_stale_extended_blocks) or test(margin_ratios_reuse_a_recent_success_within_the_ttl) or test(cancel_order_uses_modify_order_cancel_operation) or test(subscribe_trade_accounts_forwards_every_account_id) or test(mainland_trade_market_falls_back_to_the_shanghai_request_location) or test(stale_or_malformed_push_updates_never_reach_the_lifecycle) or test(kline_adds_basic_and_minimum_age_delays_unsubscribe) or test(executor_sends_us_intraday_kline_session_all_on_the_wire)'`

补充：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked -E 'test(delayed_snapshot_fallback_reads_static_info_and_pages_stock_screen_without_subscribing) or test(closed_us_session_reports_regular_close_as_previous_close)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 149 批：Futu K-line pagination、session routing 与 OpenD read boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/futu/exchange_kline_test.go:50,106,131,150,178,192,228,291,326,356` | US RTH/ETH/ALL routing and fallback, session planner, merged-page filtering, history pagination and upstream page-size tests | P1 | exact：路由拆分/合并、session fallback、时间窗口、超过八页分页与 latest-limit 保留逐项通过。 |
| [x] | `pkg/futu/adapter_kline_pagination_test.go:15,66,92,121,152,198,223,284` | broker candle cursor/page normalization, period catalog, OpenD market-time window and helper bounds tests | P1 | exact：exclusive cursor、dedupe/sort、inclusive range、invalid boundaries、period mapping 和 helper projection 均有断言。 |
| [x] | `pkg/futu/adapter_new_methods_test.go:163,475,559`; `pkg/futu/opend/new_methods_test.go:752,768,848`; `orderbook_boundaries_test.go:14` | empty symbol/order-book projections, basic quote/K-line/history empty payloads, depth closed-session guard | P1 | exact：空集合/空结果与 depth 前置错误逐项保持；current-KL 的另一聚合引用继续作为上一批 partial，避免重复 exact。 |
| [~] | `pkg/futu/opend/advanced_test.go:66`; `system_user_info_test.go:14`; `trading_reads_contracts_test.go:268` | strict option/prediction protocol, quote-rights generation state, empty funds/modify-order wrappers | P1 | partial：Rust 覆盖协议字段与 typed projection；Go 的跨 adapter 聚合、closed-client和完整 wrapper 表仍拆分在多个 owner。 |
| [x] | `pkg/futu/opend/client_test.go:206`; `client_transport_boundaries_test.go:166`; `prediction_push_test.go:14`; `search_quote_test.go:14` | request-timeout stale waiter, keep-alive worker, typed prediction push and search request encoding | P1 | exact：超时/关闭回收、推送只分发成功载荷和搜索 keyword/maxCount 透传均通过。 |

本批 32 条 Go 映射对应 Rust 精准 nextest 36/36 通过；没有发现需要先红后修的 Rust 生产差异。current-KL duplicate evidence、跨 adapter 聚合与 Rust typed wrapper 拆分继续保持 partial，不把共享测试提升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-engine --all-targets --locked -E 'test(broker_kline_cursor_preserves_exact_second_window_and_excludes_boundary_locally) or test(normalize_broker_kline_page_deduplicates_sorts_and_keeps_latest) or test(broker_klines_return_latest_page_and_use_exclusive_before_cursor) or test(normalize_broker_kline_range_keeps_inclusive_boundaries) or test(broker_kline_query_rejects_cursor_and_time_boundary_errors) or test(broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates) or test(declared_candle_periods_map_to_historical_and_realtime_types) or test(broker_kline_query_formats_opend_window_in_market_time_and_returns_utc) or test(normalized_instruments_accepts_empty_symbol_list) or test(order_book_levels_omit_detail_list_when_absent) or test(depth_read_returns_empty_arrays_for_empty_s2c_lists) or test(us_regular_only_history_uses_a_single_rth_route) or test(session_planner_selects_explicit_routes_and_keep_sets) or test(us_regular_only_request_still_queries_the_current_bucket) or test(us_regular_only_bounded_window_re_filters_the_merged_page) or test(routed_sessions_override_the_clock_classification) or test(us_history_falls_back_to_session_all_when_a_route_is_rejected) or test(chinese_supported_session_message_triggers_the_all_fallback) or test(history_window_follows_forward_pages_and_keeps_the_latest_limit) or test(history_window_allows_more_than_eight_pages) or test(history_window_uses_a_larger_upstream_page_size_than_the_limit) or test(us_intraday_history_fans_out_across_opend_session_routes) or test(request_uses_all_strategy_filters) or test(prediction_category_read_encodes_protocol_and_projects_entries) or test(response_after_request_timeout_is_not_delivered_to_a_stale_waiter) or test(keep_alive_worker_sends_frames_and_stops_on_close) or test(basic_quote_query_returns_an_empty_list_when_the_success_s2c_is_absent) or test(get_kl_missing_s2c_returns_an_empty_result) or test(history_pagination_round_trips_the_next_req_key_across_pages) or test(depth_read_rejects_a_closed_session_before_any_projection) or test(prediction_subscribers_dispatch_only_successful_typed_updates) or test(search_request_preserves_chinese_name_and_requests_full_candidate_window) or test(fetch_failure_outcomes_follow_generation_and_notification_state) or test(entitlement_query_runs_once_per_connection) or test(funds_missing_s2c_normalizes_to_an_empty_snapshot) or test(modify_order_returns_stable_identity_for_an_empty_success_payload)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 145 批：ProductFeatures market-data facade 与 Settings provider rollback（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/productfeatures/candle_query_options_test.go:8`; `market_data_reads_test.go:134,226,307,418,470` | candle query/adjustment/session parsing, workspace pagination, provider failure and snapshot extended-session projections | P1 | exact：query 归一化、分页元数据、provider failure 优先级、fallback projection、active extended session 与 fallback 字段均逐条覆盖。 |
| [x] | `internal/productfeatures/prediction_quote_candle_bridge_test.go:137` | prediction push dedupe/expiry/key folding | P1 | exact：新鲜预测 push、重复 sequence 保留首样本、五秒过期与 broker/instrument/data-type key folding 一致。 |
| [x] | `internal/productfeatures/provider_facade_company_test.go:148`; `provider_facade_rankings_test.go:278`; `provider_projection_test.go:14` | company financial forwarding, market default and nullable news projection | P1 | exact：参数转发、provider default market 和 nullable/as-of projection 均通过。 |
| [x] | `internal/productfeatures/service_test.go:30,143` | explicit broker capability rejection, optional research facade, cache and batch snapshot routes | P1 | exact：显式 broker 不 fallback、允许 feature 集合、短缓存与 batch dedupe 均覆盖。 |
| [~] | `internal/productfeatures/service_test.go:443:TestProductFeatureDirectAdapterCacheAndEligibilityBranches` | `prediction_eligibility_rejects_discovery_failure_nil_firm_and_wrong_authority` | P1 | partial：Rust 覆盖 eligibility 三类失败；Go 同测还包含多 adapter nil/fallback/cache helper 分支，属于聚合测试拆分。 |
| [x] | `internal/settings/market_data_test.go:203,224,252` | active/backtest rollback, persistence/rollback error composition and read blocking window | P1 | exact/partial：Rust 三条测试覆盖 runtime rollback、持久化/回滚错误及读等待；Go 的 fake store 细节由 settings owner 内部实现。 |

本批 16 条 Go 映射对应 Rust 精准 nextest 23/23 通过；未发现需先红后修的 Rust 生产差异，聚合 helper 分支保持 `partial`。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-settings --all-targets --locked -E 'test(candle_adjustment_normalizes_and_rejects_unsupported_labels) or test(candle_sessions_parse_dedup_order_and_reject_invalid) or test(workspace_candle_pagination_rejects_the_go_metadata_table) or test(candle_sessions_accept_padded_and_csv_query_shapes) or test(workspace_reads_surface_provider_failures_before_normalizing) or test(fallback_projection_preserves_observation_fallbacks) or test(workspace_snapshot_uses_active_extended_session_fields) or test(workspace_snapshot_extended_session_fallbacks_remain_stable) or test(fresh_prediction_push_serves_reads_and_duplicate_sequences_keep_the_first_sample) or test(prediction_push_samples_expire_after_the_five_second_window) or test(prediction_push_key_folds_broker_instrument_and_data_type) or test(company_financials_forwards_market_symbol_and_statement) or test(market_research_defaults_an_absent_market_per_provider) or test(projection_maps_nullable_fields_and_as_of) or test(explicit_broker_that_is_not_the_active_provider_is_rejected_without_fallback) or test(embedded_research_facade_serves_exactly_the_allowed_feature_set) or test(tick_candles_use_fresh_cache_without_querying_the_provider) or test(snapshot_route_serves_a_fresh_cache_hit_without_provider_access) or test(batch_snapshots_normalize_deduplicate_and_serve_the_short_lived_cache) or test(prediction_eligibility_rejects_discovery_failure_nil_firm_and_wrong_authority) or test(active_failure_rolls_back_but_backtest_failure_never_persists) or test(persistence_and_rollback_failures_are_reported) or test(reads_wait_for_the_runtime_rollback_window)'`

下一片：继续 P1 的 MarketData assets、instrument resolver 和 subscription lifecycle 映射。

## 第 146 批：MarketData assets、instrument resolver 与 subscription lifecycle（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/marketdataassets/asset_selection_boundaries_test.go:58,192` | `rejects_checksum_mismatch_before_writing` | P1 | partial：Rust 覆盖 asset 校验失败前不写入；Go missing/empty FS 的 unavailable 选择与同一 helper 聚合，保持 partial。 |
| [x] | `internal/marketdataassets/cache_test.go:15,45,83,118,249,310,336` | asset reuse, digest mismatch and path traversal tests | P1 | partial/exact：Rust 覆盖已发布 bundle 复用、digest tamper 和 unsafe path；并发 winner、socket/permission/cleanup 细节由 Go/sidecar owner 拆分。 |
| [~] | `internal/marketdataassets/assets_dev_test.go:44`; `assets_release_test.go:72`; `cache_test.go:159,185,193,207,218,229,407` | no same-shape Rust test | P1 | boundary/partial：Go 依赖嵌入 release asset、开发空资源和本地 FS 权限/清理故障；Rust helper 使用内容寻址 bundle，保留 owner 边界。 |
| [~] | `internal/marketdata/instrument_resolver_test.go:248,365` | no same-shape Rust test | P1 | boundary：Go peer cache/singleflight 与 cache recheck 是旧 resolver owner 专属，Rust provider search 不持有同形缓存。 |
| [x] | `internal/marketdata/instrument_resolver_test.go:397,437` | search failure/input mapping and generation-fenced read tests | P1 | partial：Rust 覆盖 malformed/provider error 不变为空成功、输入校验和 generation fence；Go 两次不缓存错误及 context cancellation 聚合断言拆分在不同 owner。 |
| [x] | `internal/marketdata/subscription_lifecycle_test.go:149,469,498` | subscription lease sharing, demand merge, close/deactivation cleanup | P1 | partial：Rust 覆盖 web/managed lease 隔离、并发 reconcile、exact request contract 与 close cleanup；Go fake reconciler snapshot 序列仍是聚合层差异。 |

本批 25 条 Go 映射对应 Rust 精准 nextest 15/15 通过；资产嵌入/权限和旧 resolver singleflight 保持 boundary/partial，未发现需先红后修的 Rust 生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-engine -p jftrade-marketdata -p jftrade-integration-futu --all-targets --locked -E 'test(rejects_checksum_mismatch_before_writing) or test(reuses_a_published_asset_without_rewriting_it) or test(rejects_a_bundle_whose_bytes_no_longer_match_the_digest) or test(rejects_escaping_asset_names_before_writing) or test(search_failures_and_malformed_responses_are_not_empty_successes) or test(instrument_search_route_validates_input_and_maps_provider_failures) or test(snapshot_read_fences_provider_generation_switch_during_helper_query) or test(broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease) or test(clear_route_preserves_running_strategy_lease) or test(exact_physical_subscriptions_are_shared_and_final_release_is_deferred) or test(concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release) or test(subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear) or test(deactivation_fences_cache_and_marks_router_inactive) or test(release_and_deactivate_clears_bridge_owned_router_state) or test(lifecycle_rejects_stale_callbacks_and_closes_recorder_once)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 147 批：Futu/OpenD notifications、probe 与 subscription reconciler（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/integration/futu/notifications_test.go:13,98,135,207` | `futu_notifications_parity.rs` 的 response 路由、空载荷/状态转换、程序与网关标签、权限标签测试 | P1 | exact：通知分类、nil 边界、12 个程序状态、网关/行情权限文案逐项断言。 |
| [x] | `internal/integration/futu/probe_test.go:22,96,114,137,173` | `health.rs` 协议失败/健康 fixture、`probe.rs` closed-port/version/global-state、`program_status` 测试 | P1 | 4 条 exact；协议错误承载从 Go 字符串变为 Rust typed error 的 1 条保持 partial。 |
| [x] | `internal/integration/futu/subscription_reconciler_test.go:88,160,191,287,346,394,459,499,587,623,676,697,713` | `subscriptions_tests.rs` 的物理订阅共享、并发幂等、代际快照、退订 retry、fallback、provider switch、ack 时间、清理、规范化与 viewer 能力测试；`fake_framed_opend_runtime_tests.rs` 覆盖配额拒绝 fallback | P1 | exact：订阅 ownership、最小保留期、退订阶梯、代际 fence、延迟 fallback 与请求规范化逐项通过。 |
| [~] | `subscription_reconciler_test.go:236` | `closed_session_generation_invalidates_its_subscriptions_and_requires_replay` | P1 | boundary：Rust 以新连接代际 replay/fence 取代 Go 单次 reconcile 内联重试，不复现 Go 异常文本和重试次数。 |
| [~] | `subscription_reconciler_test.go:548` | `session_coordinator.rs::refresh_quota_uses_the_authenticated_opend_protocol_and_preserves_last_success` | P1 | partial：认证配额协议、成功快照保留有 Rust 证据；Go 的 ack+1 分钟节流与固定间隔刷新在 Rust 中不存在。 |

本批 24 条 Go 映射对应 Rust 精准 nextest 24/24 通过；未发现需要先红后修的生产差异。连接代际与配额刷新两处机制差异继续保持 partial/boundary，不把聚合证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked -E 'test(test_live_notification_from_response_routes_protocol_payloads_to_neutral_categories) or test(test_neutral_notification_builders_handle_nil_and_status_transitions) or test(test_notification_labels_cover_every_supported_program_and_gateway_state) or test(test_notification_and_quote_right_labels_remain_stable) or test(tcp_probe_reports_protocol_outcomes_without_a_real_opend) or test(tcp_probe_maps_login_global_state_and_market_readiness) or test(probe_opend_reports_closed_port_as_disconnected) or test(probe_from_global_state_enforces_minimum_version_and_maps_neutral_state) or test(program_status_handles_missing_plain_and_described_status) or test(exact_physical_subscriptions_are_shared_and_final_release_is_deferred) or test(concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release) or test(stale_observed_generation_reports_pending_reconnect_instead_of_active) or test(closed_session_generation_invalidates_its_subscriptions_and_requires_replay) or test(unsubscribe_retry_ladder_escalates_and_reacquire_clears_retry_state) or test(test_basic_quote_availability_rejection_enters_delayed_fallback_and_reconcile_succeeds) or test(provider_switch_defers_physical_release_until_opend_eligible) or test(retention_is_measured_from_the_opend_acknowledgement) or test(retry_is_measured_from_the_opend_failure_acknowledgement) or test(refresh_quota_uses_the_authenticated_opend_protocol_and_preserves_last_success) or test(pending_provider_cleanup_drops_closed_connection_ownership) or test(connection_replacement_clears_quota_ownership_and_reset_is_idempotent) or test(released_never_established_records_are_dropped_without_fallback_leakage) or test(desired_physical_subscriptions_reject_incomplete_refs_and_normalize_symbols) or test(viewers_share_capabilities_and_only_the_stale_order_book_is_released)'`

下一片：继续 P1 的 Futu/OpenD protocol、trading/broker 与 API transport 映射。

## 第 144 批：Futu marketdata tick、fallback 与 market-rule reader 边界（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/integration/futu/marketdata_runtime_test.go:267,364,613,824,869` | tick conversion/trade cache inheritance, fallback push filtering, ticker map and snapshot classification | P1 | exact：不可用价格、quote fallback、trade 字段继承、fallback instrument 过滤、usable snapshot 与 session/value 分类逐条通过。 |
| [x] | `pkg/futu/adapter_marketdata_search_test.go:113` | `futu_search_rejects_invalid_queries_before_reaching_opend` | P1 | exact：空 keyword、负 limit、超上限 limit 在触达 OpenD 前拒绝。 |
| [x] | `pkg/futu/marketdata_reader_boundaries_test.go:108` | session selection and broker K-line snapshot projection tests | P1 | exact：US extended-hours 默认集合、alias 归一化、非 US/unknown session 拒绝与 response label 一致。 |
| [x] | `pkg/futu/marketdata_reader_boundaries_test.go:153` | market-rule primary/fallback/error composition tests | P1 | exact：静态规则失败时 snapshot fallback、空规则和 primary/fallback 错误合并、空 symbol 前置拒绝均覆盖。 |
| [~] | `internal/integration/futu/marketdata_runtime_test.go:452:TestMarketDataRuntimeExchangeResetAndStreamLifecycle` | `product_runtime_supervisor.rs`; `runtime_task.rs` | P1 | boundary：Go 依赖 BBGO/OpenD exchange wrapper 的 Ensure/替换、Broker ownership、通知 session 和 disabled runtime 聚合；Rust owner 分拆为 product supervisor 与显式 OpenD task，没有同形单一 runtime API，保留边界。 |

本批 9 条 Go 映射对应 Rust 精准 nextest 14/14 通过；exchange wrapper 行为保持边界，未发现需先红后修的 Rust 生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-marketdata -p jftrade-engine --all-targets --locked -E 'test(test_tick_conversion_rejects_unusable_prices_and_uses_quote_fallbacks) or test(test_tick_from_trade_inherits_latest_quote_fields_through_cache) or test(fallback_instruments_are_filtered_from_the_push_stream) or test(test_fallback_ticker_map_projects_only_requested_usable_snapshots) or test(test_fallback_snapshot_conversion_rejects_invalid_values_and_uses_classification) or test(futu_search_rejects_invalid_queries_before_reaching_opend) or test(session_selection_normalizes_aliases_and_rejects_unsupported_ones) or test(broker_kline_snapshot_session_fields_follow_go_classification_helpers) or test(sessions_default_and_validation_follow_go_extended_hours_rules) or test(market_rules_fallback_empty_keeps_the_primary_error_in_the_message) or test(market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error) or test(market_rules_report_no_rules_when_both_sources_are_empty) or test(market_rules_combine_primary_and_fallback_failures) or test(market_rules_reject_empty_symbol_queries_before_touching_opend)'`

下一片：继续 P1 的 MarketData/Quote provider forwarding 与 runtime boundary 映射。

## 第 143 批：MarketData candle/cache/collector/provider-switch 细节（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/marketdata/broker_candles_test.go:12,51,109,137` | strict page projection, terminal/bounded pages, session/number helpers, invalid pagination metadata | P1 | partial：页面、游标和输入校验由 Rust 覆盖；Go 聚合 helper 的部分错误文本与 JSON 字段仍由拆分测试分别承载。 |
| [x] | `internal/marketdata/cache_test.go:12,76,103,138,185` | cache dedupe/freshness and extended-session inheritance/promotion tests | P1 | partial：Rust 覆盖去重、保鲜窗口、跨交易日隔离、regular close promotion 和 unchanged-price extended quote；Go 的全部字段聚合断言拆分在 projection 测试。 |
| [x] | `internal/marketdata/cache_test.go:210,241,261,301,337` | tick candle volume, quote snapshot projection, shared cache and retained fallback tests | P1 | partial：行为路径与缓存调用次数均覆盖；序列化 null/extended 字段和服务聚合仍是跨 owner 拆分证据。 |
| [x] | `internal/marketdata/candle_sessions_test.go:8,18,26,44,71` | session parsing/filtering, CN instrument normalization, calendar session labels | P1 | partial：session 解析、过滤和 normalize 规则有 Rust 证据；Go helper 的 unknown→regular 兼容细节分布在多个 Rust owner。 |
| [x] | `internal/marketdata/collector_test.go:15,142,180,249,355` | collector lifecycle, polling fallback, demand cancellation, generation fencing, retry ladder | P1 | partial：Rust 覆盖 stale generation、poll demand、lease 优先级和 retry ladder；Go close/handler 计数与 collector 聚合状态没有一一同形断言。 |
| [x] | `internal/marketdata/provider_switch_lifecycle_test.go:184,268,312` | provider activation/deactivation cache fencing, in-flight read fencing, failed switch preservation | P1 | partial：Rust 覆盖 generation/cache fence 和失败保留旧 provider；Go collector close 顺序与缓存可见性的组合时序由多个测试拆分。 |

本批 27 条 Go 映射对应 Rust 精准 nextest 40/40 通过。未发现需要先红后修的真实 Rust 生产差异；聚合状态/序列化字段/collector 计数差异保持 `partial`，不提升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-marketdata -p jftrade-integration-futu --all-targets --locked -E 'test(broker_kline_pagination_helpers_cover_sessions_bounds_and_listing_dates) or test(cached_projection_uses_active_after_quote_and_separates_closes) or test(cached_security_snapshot_reader_batches_hk_and_serves_repeats_from_cache) or test(candle_sessions_parse_dedup_order_and_reject_invalid) or test(candles_and_search_validation_rules) or test(closed_session_rejects_connect_like_rpcs_before_touching_the_transport) or test(completion_after_generation_change_cannot_mutate_cache_or_failure_state) or test(deactivation_fences_cache_and_marks_router_inactive) or test(disconnected_provider_reports_its_reason_and_keeps_the_previous_selection) or test(explicit_activation_fails_closed_and_switch_clears_cache) or test(futu_snapshot_route_projects_cached_extended_quote_contract) or test(infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes) or test(latest_demand_replaces_stale_replay_while_reconnect_is_pending) or test(lifecycle_rejects_stale_executor_before_qot_sub_io) or test(normalize_instrument_rejects_unsupported_market_and_market_mismatch) or test(poll_normalizes_demand_writes_only_requested_ticks_and_skips_fresh_cache) or test(poll_only_read_routes_prioritize_capabilities_and_preserve_leases) or test(provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update) or test(recorder_matches_generation_retry_recovery_and_close_rules) or test(retry_delay_is_capped_after_four_failures) or test(snapshot_read_fences_provider_generation_switch_during_helper_query) or test(snapshot_route_serves_a_fresh_cache_hit_without_provider_access) or test(stale_caller_is_rejected_even_when_the_active_generation_cache_is_fresh) or test(test_broker_k_line_candles_response_handles_terminal_and_bounded_pages) or test(test_broker_k_line_candles_response_projects_strict_page) or test(test_broker_k_line_helpers_classify_sessions_and_numbers) or test(test_broker_k_line_pagination_rejects_invalid_bounded_and_paged_metadata) or test(test_cache_deduplicates_promotes_and_inherits) or test(test_cache_does_not_inherit_extended_sessions_across_trading_days) or test(test_cache_freshness_retention_and_maximum) or test(test_cache_promotes_us_regular_close_when_after_hours_trade_arrives) or test(test_cache_retains_extended_quote_when_price_is_unchanged) or test(tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume) or test(tick_candles_fall_back_to_retained_cache_on_ticker_error) or test(tick_candles_filter_sessions_before_applying_the_limit) or test(tick_candles_query_the_provider_once_on_cache_miss_and_ingest_the_sample) or test(tick_candles_use_explicit_volume_delta_across_trading_days) or test(tick_candles_use_fresh_cache_without_querying_the_provider) or test(unknown_provider_activation_is_rejected_without_touching_the_active_selection) or test(us_intraday_futu_candles_carry_calendar_resolved_session_labels)'`

下一片：继续 P1 的 MarketData/Quote provider forwarding 与 runtime boundary 映射。

## 第 160 批：Assistant session skill/store、workflow persistence 与 lifecycle（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/engine/session_context_test.go:922,977,1009`; `session_skill_test.go:15,68`; `session_wrap_test.go:9` | post-compaction event tracking, pending approval execution, builtin skill catalog and session wrapper tests | P1 | partial：session projection、approval hold 与内建 skill 保护均有 Rust 断言；Go wrapper 委托聚合由 Rust session service 拆分覆盖。 |
| [x] | `internal/assistant/engine/skillsruntime/schema_test.go:48`; `store_approve_test.go:98`; `store_async_test.go:89`; `store_audit_query_test.go:7` | MCP schema closure, session deletion, approval retry and audit pagination tests | P1 | exact：schema、删除幂等、approval reopen guard 与 audit filters 逐项通过。 |
| [x] | `internal/assistant/engine/store_business_test.go:108`; `store_failure_normalization_boundaries_test.go:170`; `store_identity_test.go:85`; `store_lifecycle_test.go:110,466,501,553,565`; `store_maintenance_test.go:54` | cascade cleanup, restart durability, session page/composer/approval queries and owned-database maintenance tests | P1 | partial：SQLite cascade、分页、composer state、missing/blank errors 与 writer lease 均覆盖；Go 多数据库 facade 聚合保持 partial。 |
| [x] | `internal/assistant/engine/store_ops_test.go:281,402,426,436,486`; `store_test.go:437,607,881,898,947,1005` | agent ownership, cancellation, idempotent approval, run listing, provider timeout, resume and expiry reconciliation tests | P1 | exact：owner guard、cancel/approval semantics、timeout freeze、resume 与 expiry reconciliation 均有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 29/29 通过；未发现需要先红后修的生产差异。session wrapper 与多数据库聚合保持 partial，不把聚合证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(session_context_tracks_events_appended_after_a_compaction) or test(a_run_waiting_for_approval_does_not_block_chat_auto_compaction) or test(compaction_preserves_the_call_of_a_pending_approval) or test(builtin_skill_catalog_stays_registered_alongside_external_installs) or test(session_context_window_follows_the_composer_provider_override) or test(reviewed_mcp_schemas_match_canonical_go_fixture_deeply) or test(session_delete_missing_is_idempotent_and_blank_ids_are_rejected) or test(completed_run_reopens_only_for_a_fresh_durable_approval) or test(adk_audit_route_filters_by_kind_and_subject_id) or test(test_shared_artifacts_preserved_and_user_session_deletion_rejected) or test(adk_store_lifecycle_and_restart_durability) or test(test_idempotent_retry_and_partial_failure_crash_recovery) or test(test_adk_cascade_cleanup_removes_all_entities_across_three_databases) or test(adk_session_page_filters_by_agent_and_title_and_paginates) or test(adk_composer_state_truncates_trim_and_rejects_invalid_modes) or test(adk_session_delete_missing_is_reported_with_the_session_error_code) or test(adk_approval_page_orders_by_latest_update_and_counts_filtered_rows) or test(adk_maintenance_compacts_every_owned_database_and_requires_the_writer_lease) or test(chat_rejects_a_session_owned_by_a_different_agent) or test(cancelling_a_run_denies_its_pending_approvals) or test(adk_cancel_run_missing_is_the_dedicated_cancel_failure) or test(adk_resolve_approval_missing_returns_the_idempotent_empty_envelope) or test(adk_run_listing_filters_and_sorts_newest_first) or test(saved_provider_normalizes_the_request_timeout_on_write) or test(an_approval_resuming_run_is_recovered_after_a_runtime_restart) or test(run_start_freezes_the_configured_run_timeout_from_settings) or test(resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window) or test(expired_running_run_is_reconciled_to_timed_out_with_failed_tool_calls) or test(expired_runs_use_each_runs_own_timeout_window)'`

下一片：继续 P1 Assistant workflow/ADK、API transport 与 Trading/Broker execution 映射。

## 第 161 批：Assistant workflow/ADK continuation、service facade 与 Backtest recovery（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/engine/task_runner_test.go:79`; `tool_schema_workflow_test.go:35`; `tools_test.go:25,206` | cancellation registry, strict tool schema, unavailable tool catalog and workflow wait cancellation tests | P1 | exact：取消传播、schema 校验、不可用工具标记与 wait cancellation 均逐项通过。 |
| [x] | `internal/assistant/engine/workflow_approval_recovery_boundaries_test.go:8`; `workflow_child_test.go:8`; `workflow_persistence_test.go:9`; `workflow_reconcile_test.go:8,49,73,128`; `workflow_tools_test.go:263` | fail-closed workflow invocation, disabled schedule, CAS approval continuation/denial, child reopen/restage and tool error envelope tests | P1 | partial：workflow approval/CAS 与错误 envelope 有真实 owner 证据；Go parent/child facade 聚合仍由拆分测试承载。 |
| [x] | `internal/assistant/engine/workflowexec/goal_resume_failure_boundaries_test.go:26,86`; `goal_state_boundaries_test.go:158`; `workflow_approval_recovery_boundaries_test.go:11,28`; `workflow_child_failure_persistence_test.go:10`; `workflow_execution_persistence_test.go:123` | goal resume, terminal audit, pause CAS, startup orphan recovery, child failure and save/cancel persistence tests | P1 | exact：goal pause/resume、terminal audit、orphan approval、child failure 与 stale writer 规则逐项通过。 |
| [x] | `internal/assistant/engine/workflow_goal_terminal_helpers_test.go:12`; `workflow_pending_input_contract_test.go:10`; `workflow_persistence_test.go:67`; `workflow_reconcile_executor_boundaries_test.go:9,69,110,164`; `workflow_reconcile_ignore_boundaries_test.go:9`; `workflow_task_tools_lookup_test.go:135`; `internal/assistant/engine/workflowruntime/runtime_test.go:11`; `internal/assistant/model/timeline_reply_ordering_test.go:11` | completed projection, input response, pause persistence, pending/running/terminal child reconciliation, scheduler graph, route dispatch and timeline ordering tests | P1 | partial：child reconciliation、dispatch adapter 与 timeline ordering 有 Rust 证据；Go executor facade 的跨层组合保持 partial。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 27/27 通过；未发现需要先红后修的生产差异。workflow parent/child 与 executor facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(cancellation_registry_fans_out_and_unregisters_exact_token) or test(strict_tool_schemas_reject_invalid_arguments_before_the_executor_runs) or test(tool_catalog_marks_external_unavailable_tools_non_callable) or test(workflow_wait_tool_returns_context_cancellation) or test(workflow_run_without_a_model_runtime_fails_closed_and_finalises_the_invocation) or test(test_schedule_trigger_disabled_parent_workflow_safely_advances) or test(run_terminal_state_cannot_be_regressed_by_a_stale_running_snapshot) or test(adk_approval_resolution_stages_continuation_and_denial_cas) or test(adk_approval_resolution_missing_and_non_pending_rows_are_idempotent) or test(a_gated_call_parks_the_run_and_audits_awaiting_approval) or test(completed_run_reopens_only_for_a_fresh_durable_approval) or test(adk_approval_resolution_restages_a_stale_embedded_approval) or test(tool_error_envelopes_classify_timeout_cancellation_and_structured_metadata) or test(resume_goal_run_restarts_a_timed_out_goal_with_a_fresh_settings_window) or test(a_failed_run_persists_its_terminal_state_and_audit_row) or test(user_goal_pause_fields_survive_a_stale_writer_and_clear_on_explicit_resume) or test(assistant_run_mutation_routes_enforce_goal_lifecycle_rules) or test(orphaned_pending_approval_runs_are_failed_on_startup_reconcile) or test(a_failed_run_persists_the_provider_error_and_audit_row) or test(a_completed_run_persists_the_reply_and_audits_run_completed) or test(input_response_payload_anchors_the_resumed_run_to_the_original_request) or test(paused_workflow_run_keeps_accepting_progress_and_terminal_updates) or test(expired_runs_use_each_runs_own_timeout_window) or test(a_completed_projection_hides_resolved_approvals) or test(graph_exposes_one_deterministic_ready_task) or test(every_canonical_production_route_has_a_dispatch_adapter) or test(staging_a_tool_round_freezes_the_pre_tool_assistant_text)'`

下一片：继续 P1 Assistant service/read routes、workflow resource CRUD 与 Backtest recovery 映射。

## 第 162 批：Assistant service/read、workflow CRUD 与 Backtest recovery（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/assistant/service_audit_pagination_test.go:9`; `service_business_helpers_test.go:105`; `service_business_test.go:117`; `service_contract_boundaries_test.go:133`; `service_lifecycle_boundaries_test.go:10,71`; `service_persistence_runtime_boundaries_test.go:12`; `service_recovery_test.go:10` | ADK audit/session read, compaction wrapper, dynamic route validation, runtime defaults, cancellation and terminal frame recovery tests | P1 | partial：service read/compaction/recovery 分支有 Rust 证据；Go facade 的错误包装和多资源聚合保持 partial。 |
| [x] | `internal/assistant/service_skill_state_recovery_test.go:19,61,119`; `service_test.go:9` | skill install, optimization negative/cancel routes and legacy timeline error code tests | P1 | exact：skill、optimization route 与 error code 链路逐项通过。 |
| [x] | `internal/assistant/workflow/rules_test.go:12,207`; `workflow_crud_test.go:14,400`; `workflow_lifecycle_test.go:13,98`; `workflow_store_failures_test.go:13` | cron timezone/weekday semantics, workflow/trigger CRUD, bridge preflight, shutdown/join and closed-store routes | P1 | exact：cron、资源删除可见性、unknown target、停止 join 与 not-found code 均有真实 owner 证据。 |
| [x] | `internal/assistant/workflows_extended_test.go:528`; `workflows_resource_recovery_test.go:11,52,105`; `workflows_test.go:11,198,248,276` | workflow CAS loser/recovery, canvas interpolation/join, no-node fail-closed and downstream error skip tests | P1 | partial：workflow recovery、canvas pipeline 与 fail-closed 路径通过；Go 运行时 facade 聚合保持 partial。 |
| [x] | `internal/backtest/historical_source_test.go:111,147,181` | historical sync cancellation and empty provider preflight tests | P1 | exact：取消不污染恢复状态、空 provider 结果与 preflight 拒绝逐项通过。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 27/27 通过；未发现需要先红后修的生产差异。service/workflow facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(adk_audit_route_filters_by_kind_and_subject_id) or test(a_chat_turn_autocompacts_the_session_before_the_provider_payload) or test(adk_session_page_filters_by_agent_and_title_and_paginates) or test(adk_read_dynamic_routes_validate_suffixes_and_identifiers) or test(assistant_runtime_defaults_bounds_and_round_trip_match_go) or test(cancellation_registry_fans_out_and_unregisters_exact_token) or test(a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame) or test(skill_document_install_registers_the_parsed_document) or test(optimization_task_negative_routes_match_the_reference_matrix) or test(optimization_task_cancel_route_preserves_go_operation_identity) or test(session_timeline_failure_keeps_the_legacy_messages_error_code) or test(next_schedule_run_calculation_with_timezone) or test(next_schedule_run_skips_weekend_for_weekday_cron) or test(parse_invalid_cron_expressions) or test(numeric_step_and_wildcard_day_rules_match_go) or test(workflow_and_trigger_lists_hide_deleted_rows_after_create_and_delete) or test(workflow_bridge_pages_lists_and_rejects_unknown_run_targets) or test(stop_blocks_new_dispatch_and_join_reports_active_owners) or test(test_workflow_scheduler_worker_start_stop_and_status) or test(workflow_mutation_routes_keep_the_go_not_found_codes) or test(failure_cas_loser_distinguishes_missing_durable_winner) or test(failure_cas_loser_returns_durable_failed_winner_without_overwriting_it) or test(test_context_interpolation_multi_node_pipeline_and_diamond_join) or test(test_canvas_no_executable_nodes) or test(test_canvas_workflow_error_skips_downstream_nodes) or test(cancellation_does_not_poison_a_recovered_run) or test(test_historical_k_line_syncer_rejects_empty_provider_result)'`

下一片：继续 P1 Backtest historical/storage aggregation 与 conservative execution 映射。

## 第 163 批：Backtest historical/storage aggregation 与 conservative execution（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/backtest/historical_source_test.go:268,341`; `recovery_test.go:11,27,45`; `result_view_test.go:223`; `run_failure_recovery_test.go:26,53,81`; `service_test.go:652`; `sync_test.go:17,147,412`; `time_test.go:8` | historical pagination/provider sync, worker failure/panic, result view, lifecycle persistence, shutdown/cancel, sync task and session scope/DST tests | P1 | partial：backtest start/sync/recovery/expiry 行为均覆盖；Go worker facade 的 panic 聚合与错误文本保持 partial。 |
| [x] | `pkg/backtest/conservative_bar_executor_test.go:147,245,276,336,412,771`; `cost_account_failure_boundaries_test.go:13` | parent bracket atomicity, protective child cancellation, reduce-only guard, cancel targeting, slippage and fee fallback tests | P1 | exact：stop-first、protective children、reduce-only、cancel matching、slippage 与 empty fee rules 逐项通过。 |
| [x] | `pkg/backtest/filter_store_session_queries_test.go:137,160,246,276,310,346,387,413,459,540,627` | session-filtered read/stream/query, extended-hours ranges, cursor ordering, empty/error propagation and schema helper tests | P1 | partial：session scope、custom ranges、paging 与 empty/error guard 有 Rust 证据；Go filter store 聚合保持 partial。 |
| [x] | `internal/backtest/internal/runmodel/result_test.go:126`; `internal/backtest/internal/storage/aggregate_corruption_errors_test.go:15`; `codec_progress_boundaries_test.go:130`; `query_failure_empty_recovery_test.go:11,120`; `store_aggregation_boundaries_test.go:71,194,234`; `store_business_aggregation_test.go:168`; `store_failure_boundaries_test.go:188,207`; `store_runtime_invariants_test.go:47,354,441,500`; `store_session_aggregation_contracts_test.go:11,58,105,132`; `stream_query_failure_sorting_test.go:11` | deterministic result snapshot, corruption/coverage failures, canonical table aliases, session normalization, intraday/daily aggregation and stream/query failure tests | P1 | partial：storage aggregation、coverage gap、canonical schema 与 session-aware buckets 均覆盖；Go 多层 store error aggregation 保持 partial。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 25/25 通过；未发现需要先红后修的生产差异。filter store 与多层 storage error 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(test_historical_k_line_syncer_rejects_broken_pagination) or test(production_futu_sync_uses_opend_reader_and_persists_candles) or test(reap_finished_marks_nonterminal_worker_failed) or test(test_start_script_rejects_blank_research_script) or test(test_result_view_contract_alignment_and_edge_cases) or test(production_backtest_start_without_worker_fails_before_persisting_run) or test(definition_resolution_rejects_missing_definition_and_version_conflict) or test(backtest_runs_lifecycle_and_restart_durability) or test(shutdown_persists_cancelled_state_before_joining_workers) or test(production_sync_read_projects_persisted_task) or test(reap_finished_removes_completed_join_handles) or test(sync_request_session_scope_parity_with_go) or test(backtest_start_resolves_market_dates_with_dst) or test(parent_bracket_runs_atomically_with_stop_first_protection) or test(canceling_parent_order_cancels_all_protective_children) or test(reduce_only_order_without_position_is_canceled) or test(explicit_cancel_orders_and_unmatched_target_handling) or test(sell_market_order_applies_downward_slippage) or test(cancel_skips_unmatched_pending_orders_without_side_effects) or test(provided_fee_rules_apply_and_empty_rules_charge_nothing) or test(aggregate_reads_use_only_the_requested_session_scope) or test(regular_and_extended_session_tables_keep_direct_reads_isolated) or test(test_tc_d4_04_extended_session_pre_market_does_not_pollute_regular_open) or test(direct_interval_rows_win_and_paging_is_deterministic) or test(missing_symbol_table_reports_storage_error_then_recovers_after_insert)'`

下一片：继续 P1 Backtest Pine worker/result collector、Market/Calendar 与 Futu session boundary 映射。

## 第 164 批：Backtest Pine worker、result collector 与 storage synthesis（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/backtest/filter_store_session_queries_test.go:540,627`; `internal/backtest/internal/runmodel/result_test.go:126`; `internal/backtest/internal/storage/aggregate_corruption_errors_test.go:15`; `codec_progress_boundaries_test.go:130`; `query_failure_empty_recovery_test.go:11,120`; `store_aggregation_boundaries_test.go:71,194,234`; `store_business_aggregation_test.go:168`; `store_failure_boundaries_test.go:188,207`; `store_runtime_invariants_test.go:47,354,441,500`; `store_session_aggregation_contracts_test.go:11,58,105,132`; `stream_query_failure_sorting_test.go:11` | session stream/helper, deterministic result, corruption/coverage, schema alias, aggregation, paging and stream failure tests | P1 | partial：storage synthesis、coverage gap 和 stream/query failures 均有 Rust 证据；Go 多层 storage facade 聚合保持 partial。 |
| [x] | `pkg/backtest/pine_costs_test.go:40,64`; `pineworker_command_executor_test.go:406,421,579,626`; `pineworker_runner_boundaries_test.go:93`; `pineworker_runner_test.go:358` | slippage/cancel dispatch, cancel-all, source/history guard and snapshot fallback eligibility tests | P1 | partial：Pine worker cancel、source guard 与 fallback eligibility 通过；Go runner 文件清理与 command executor 聚合保持 partial。 |
| [x] | `pkg/backtest/result_collector_test.go:66`; `result_collector_trade_stats_test.go:81`; `session_filter_store_boundaries_test.go:11,78,141`; `session_filter_store_test.go:9`; `short_replay_bounds_test.go:88`; `short_replay_test.go:74`; `store_session_synth_test.go:11,75,138,184,234,302,376,468`; `store_test.go:292` | trade/result stats, session-filter replay, synthetic price/cancel, DST/HK/US bar synthesis and incomplete-range fail-closed tests | P1 | partial：collector trade stats、session replay、synthetic bars 与 incomplete coverage 均覆盖；Go aggregate result shape 拆分在多个 Rust owner。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 21/21 通过；未发现需要先红后修的生产差异。storage facade、Pine runner 与 collector 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(test_tc_d4_04_extended_session_pre_market_does_not_pollute_regular_open) or test(schema_kline_table_name_and_session_scope_validation_contracts) or test(corpus_output_is_byte_deterministic_across_replays) or test(missing_empty_partial_and_corrupt_lower_coverage_fail_closed) or test(interval_aliases_share_a_canonical_storage_table) or test(empty_regular_and_extended_tables_return_empty_results) or test(test_tc_d4_01_regular_session_intraday_sub_hourly_aggregation) or test(missing_daily_target_uses_ten_minute_source_before_failing) or test(unknown_session_scope_is_rejected_without_creating_a_table) or test(missing_regular_minute_fails_closed_for_synthesized_daily_bar) or test(direct_interval_rows_win_and_paging_is_deterministic) or test(slippage_price_uses_market_tick_size_and_adjusts_sides) or test(explicit_cancel_orders_by_generated_order_id) or test(test_execute_strategy_intents_cancel_dispatches_order_cancel) or test(test_execute_strategy_intents_cancel_all_queries_and_cancels_active_orders) or test(pine_adapter_requires_source_and_history) or test(snapshot_availability_kinds_control_fallback_eligibility)'`

下一片：继续 P1 Market/Calendar、Futu session boundary 与桌面日志运行时映射。

## 第 165 批：Market/Calendar、Futu session 与 quote boundary（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/backtest/result_collector_test.go:66`; `result_collector_trade_stats_test.go:81`; `session_filter_store_boundaries_test.go:11,78,141`; `session_filter_store_test.go:9`; `short_replay_bounds_test.go:88`; `short_replay_test.go:74`; `store_session_synth_test.go:11,75,138,184,234,302,376,468` | result stats, session-filter replay, synthetic pricing/cancel and DST/HK/US synthesis tests | P1 | partial：collector、replay、synthetic bars 与 calendar buckets 有 Rust 证据；Go result facade 聚合保持 partial。 |
| [x] | `pkg/backtest/store_test.go:292`; `internal/integration/futu/candle_sessions_test.go:11`; `order_updates_test.go:59`; `security_details_test.go:126` | incomplete range fail-closed, candle session labels, fill mapping and neutral security envelope tests | P1 | exact：range coverage、session conversion、partial fill guard 与 null optional blocks 逐项通过。 |
| [x] | `pkg/futu/exchange_business_boundary_test.go:433,461`; `internal/marketdata/provider_switch_boundaries_test.go:123`; `rankings_facade_test.go:184` | market/session resolver samples, quote window fallback, provider utility and board-kind default tests | P1 | partial：session resolver、quote fallback、provider utility 与 rankings default 有 Rust 证据；Go exchange registry 聚合保持 partial。 |
| [x] | `pkg/market/calendar/builtin_test.go:38,54,73,92`; `helpers_boundaries_test.go:52,136,167`; `hk/hk_test.go:8`; `instrument_session_validation_test.go:8,31`; `market_normalization_test.go:83,161,264,330` | builtin calendar holiday/alias fallback, override/session bounds, market profiles, instrument validation and session normalization tests | P1 | exact：HK/China aliases、holiday fallback、manual override、instrument/session validation 与 session labels 均有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 26/26 通过；未发现需要先红后修的生产差异。result facade 与 exchange registry 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(short_covers_and_reversals_count_each_closing_order_once) or test(partial_closing_cancellation_finalizes_the_filled_segment) or test(regular_and_extended_session_tables_keep_direct_reads_isolated) or test(missing_empty_partial_and_corrupt_lower_coverage_fail_closed) or test(extended_daily_aggregation_uses_intraday_source_before_stored_daily_fallback) or test(matching_and_pricing_helper_branches_behavior) or test(explicit_cancel_orders_by_generated_order_id) or test(test_tc_d4_02_and_03_dst_boundary_and_60m_session_anchored_aggregation) or test(test_tc_d4_01_regular_session_intraday_sub_hourly_aggregation) or test(direct_interval_rows_win_and_paging_is_deterministic) or test(missing_daily_target_synthesizes_us_regular_day_from_minutes) or test(weekly_and_monthly_targets_synthesize_from_daily_rows_with_calendar_boundaries) or test(weekly_daily_source_requires_every_open_trading_date) or test(test_market_sessions_for_candle_sessions) or test(mapper_rejects_partial_or_malformed_fill_fields) or test(test_tc_d5_01_sequential_partial_fills_and_fees_monotonic) or test(futu_securities_route_projects_broker_neutral_envelope_boundary) or test(resolve_market_session) or test(QuoteSessionResolver) or test(quote_session_label) or test(resolve_quote_session) or test(tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume) or test(infer_cn_prefix_supports_various_formats_and_preserves_explicit_prefixes) or test(tick_candles_use_fresh_cache_without_querying_the_provider) or test(test_board_kind_defaults_empty_to_industry) or test(builtin_hk_and_mainland_weekday_weekend_and_alias_boundaries_match_go) or test(validate_snapshot_accepts_mainland_alias_markets) or test(manual_override_status_and_session_window_boundaries_match_go) or test(cached_mainland_snapshot_fallback_and_freshness_boundaries_match_go)'`

下一片：继续 P1 Calendar session windows, exchange manager 与 desktop log runtime 映射。

## 第 166 批：Calendar session windows、exchange manager 与 desktop logs（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/market/helpers_boundaries_test.go:167`; `hk/hk_test.go:8`; `instrument_session_validation_test.go:8,31`; `market_normalization_test.go:83,161,264,330`; `market_test.go:11,260`; `session_boundaries_test.go:10,51,67,102`; `session_calendar_refresh_contract_test.go:36`; `session_window_test.go:8,50,80,126,142`; `sh/sh_test.go:8`; `sz/sz_test.go:8`; `us/us_test.go:8` | calendar/market normalization, manual override, session windows, DST/holiday, profile and instrument validation tests | P1 | exact：市场 profile、manual override、session bounds、overnight/holiday、DST 与 unknown market 边界均有真实 owner 证据。 |
| [x] | `cmd/jftrade-desktop/main_test.go:131,395,426,464,506,523`; `internal/datamanagement/service_test.go:44` | desktop static asset fallback, log day/page/tail filtering and maintenance confirmation tests | P1 | exact：静态资源、日志过滤分页/尾部偏移、缺失目录与 maintenance validation 逐项通过。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 18/18 通过；未发现需要先红后修的生产差异。Calendar/session 与 desktop log 行为均有真实 owner 证据。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(builtin_hk_and_mainland_weekday_weekend_and_alias_boundaries_match_go) or test(market_rules_ssot_contains_all_core_markets_with_decimal_tick_sizes) or test(normalize_instrument_rejects_missing_inputs) or test(manager_lifecycle_and_market_lookup_boundaries_match_go) or test(session_context_distinguishes_hk_lunch_and_weekend) or test(manual_override_is_authoritative_for_closed_and_custom_windows) or test(manual_override_status_and_session_window_boundaries_match_go) or test(test_go_compatible_candle_conversion_and_session_classification) or test(authoritative_half_day_closes_daily_and_partial_intraday_bars) or test(session_context_handles_us_holidays_early_close_and_sunday_overnight) or test(unknown_market_probe_and_refresh_are_accepted_noops) or test(cached_mainland_snapshot_fallback_and_freshness_boundaries_match_go) or test(refresh_uses_market_local_boundaries_and_not_current_dst_offset) or test(desktop_missing_static_assets_do_not_use_spa_fallback) or test(log_reader_matches_go_filter_paging_and_day_order) or test(log_reader_caps_page_limit_and_paginates_all_lines) or test(test_list_desktop_log_days_missing_dir_returns_empty) or test(maintenance_service_confirmation_validation_and_rejection_parity)'`

下一片：继续 P1 exchange calendar manager、AkShare/YFinance providers 与 retry/transport 映射。

## 第 167 批：Exchange calendar manager、AkShare/YFinance providers 与 transport retry（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/exchangecalendar/http_source_test.go:40`; `manager_boundaries_test.go:50,172,216,248`; `manager_runtime_test.go:125`; `manager_test.go:50,346,444,488,613,670,876,890` | calendar fetch timeout, snapshot validation/fallback, restore/discard, probe/refresh failures, alert dedupe and cross-year cache tests | P1 | exact：manager warmup/restore/probe/refresh、snapshot validator、fallback freshness 与 year indexing 均逐项通过。 |
| [x] | `internal/integration/akshare/boundaries_test.go:62,131,392`; `client_index_constituents_test.go:14`; `client_news_actions_test.go:41,139,159`; `provider_calendar_macro_test.go:81`; `provider_company_research_test.go:14,172`; `provider_rankings_industries_test.go:82`; `provider_test.go:272` | AkShare retry/backpressure, pagination, index/news/corporate/calendar/research/rankings and session validation tests | P1 | partial：provider route and transport behavior covered；Go upstream fixtures and helper composition remain split across Rust production ports. |
| [x] | `internal/integration/yfinance/client_news_actions_test.go:83`; `client_test.go:86,136,156,172,192`; `conversion_test.go:147,237,354,446,528,548`; `provider_company_research_test.go:14,69,100`; `provider_test.go:254` | YFinance structured failures, decoder strictness, retry/cancellation, snapshot/candle conversion and company/statement projections | P1 | partial：strict decoder、retry budget、session conversion 与 provider projection 有真实 owner 证据；Go client facade 聚合保持 partial。 |
| [x] | `internal/retry/do_attempts_test.go:8,26`; `retry_test.go:10,40,57,76`; `internal/system/service_status_defaults_test.go:61` | retry defaults/backoff/non-retryable/rate-limit and calendar control-plane fallback tests | P1 | exact：retry/backoff、rate-limit classification 与 calendar manager fallback 逐项通过。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 30/30 通过；未发现需要先红后修的生产差异。AkShare/YFinance client facade 与 fixture 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(default_registry_client_uses_the_calendar_fetch_timeout) or test(manual_override_status_and_session_window_boundaries_match_go) or test(validate_snapshot_rejects_the_go_boundary_table) or test(corrupt_cached_snapshots_are_rejected_with_the_go_conditions) or test(registered_provider_snapshots_pass_domain_validation_before_caching) or test(cached_mainland_snapshot_fallback_and_freshness_boundaries_match_go) or test(restore_reports_malformed_cached_snapshot_with_its_path) or test(probe_budget_bounds_one_provider_call_without_hanging_the_manager) or test(invalid_cached_snapshot_is_discarded_deleted_and_replaced_by_builtin_rules) or test(probe_treats_an_empty_parse_as_structure_changed_unhealthy) or test(refresh_treats_an_empty_parse_as_structure_changed_failure) or test(network_timeout_variants_share_one_alert_fingerprint) or test(successful_probe_recovery_clears_the_recorded_fetch_failure) or test(probe_uses_market_local_year_when_us_crosses_utc_new_year) or test(cross_year_snapshot_is_cached_for_every_covered_year_and_summarised_once) or test(retries_transient_readiness_and_sends_optional_bearer) or test(test_research_screen_route_uses_production_helper_and_preserves_upstream_rate_limit) or test(candle_route_excludes_before_boundary_and_can_continue_loading_older_pages) or test(index_constituents_read_forwards_the_normalized_leaf_and_limit) or test(market_data_news_search_read_route_matches_group_fixture_in_cutover_only) or test(calendar_and_macro_propagate_capability_and_busy_errors) or test(economic_calendar_route_derives_date_and_time) or test(company_financials_forwards_market_symbol_and_statement) or test(research_screen_definition_rejects_unsupported_market_and_stable_keys) or test(rankings_limit_falls_back_to_legacy_limit_and_default) or test(candle_route_classifies_unknown_us_session_as_a_data_error) or test(research_screen_leaf_maps_invalid_provider_rows_to_bad_gateway) or test(decoder_rejects_unknown_and_trailing_json_before_the_port)'`

下一片：继续 P1 bbgo stream/orderbook, Settings/Watchlist 与 SQLite/Store boundaries。

## 第 168 批：bbgo stream/orderbook、Settings/Watchlist 与 transport retry（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/integration/yfinance/client_test.go:172,192`; `conversion_test.go:147,237,354,446,528,548`; `provider_company_research_test.go:14,69,100`; `provider_test.go:254` | YFinance warming/timeout, snapshot/candle conversion, strict pages, session filtering and company/statement projection tests | P1 | partial：YFinance retry/conversion/decoder 与 research projection 有真实 owner 证据；client facade 聚合保持 partial。 |
| [x] | `internal/retry/do_attempts_test.go:8,26`; `retry_test.go:10,40,57,76`; `internal/system/service_status_defaults_test.go:61` | retry/backoff/rate-limit and calendar control-plane fallback tests | P1 | exact：retry policy、non-retryable/rate-limit mapping 与 calendar fallback 逐项通过。 |
| [x] | `pkg/bbgo/types/rbtorderbook_test.go:10`; `standardstream_test.go:37,71,121,149,190,221`; `observability/observability_test.go:45` | orderbook projection, frame size, push lifecycle, closed session, keepalive, reader close and observability snapshot tests | P1 | exact：stream/orderbook lifecycle、frame guard、keepalive/close 与 empty collection serialization 均有真实 owner 证据。 |
| [x] | `internal/settings/persistence_and_mcp_failures_test.go:125`; `service_test.go:435`; `internal/watchlist/futu/source_test.go:44,174,221,396,418`; `quote_preview_boundaries_test.go:12,37`; `service_quotes_test.go:62,117,210,230,270`; `service_test.go:678` | settings security/MCP fallback, onboarding defaults, watchlist duplicate/fallback/extended quote, quote cache/singleflight/provider switch and revision fence tests | P1 | partial：Settings/Watchlist cache、fallback、singleflight 与 revision fence 有 Rust 证据；Go source facade 聚合保持 partial。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 32/32 通过；未发现需要先红后修的生产差异。YFinance 与 Settings/Watchlist facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(calendar_and_macro_propagate_capability_and_busy_errors) or test(retries_transient_readiness_and_sends_optional_bearer) or test(snapshot_route_queries_the_provider_once_on_cache_miss) or test(resolver_projects_us_early_close_extended_window_with_timezone) or test(candle_route_excludes_before_boundary_and_can_continue_loading_older_pages) or test(candle_route_forwards_strict_before_window) or test(candle_route_skips_session_classification_for_unannotated_requests) or test(candle_route_classifies_unknown_us_session_as_a_data_error) or test(company_financials_forwards_market_symbol_and_statement) or test(provider_company_profile_projection_maps_frontend_keys) or test(provider_financial_statements_projection_maps_frontend_keys) or test(recoverable_errors_match_go_is_recoverable_opend_err) or test(runtime_task_backoff_replays_after_a_failed_reconnect_attempt) or test(test_exponential_backoff_progression_and_upper_bound_capping) or test(basic_quote_query_replays_once_after_recoverable_session_timeout) or test(basic_quote_query_returns_non_recoverable_rejection_without_replaying) or test(security_snapshot_coordinator_classifies_remote_rate_limit) or test(margin_ratio_server_throttling_maps_to_the_typed_rate_limit) or test(calendar_control_plane_routes_share_the_real_manager_in_cutover_only) or test(calendar_control_plane_routes_fail_closed_without_a_manager) or test(order_book_levels_project_price_volume_count_and_details) or test(frame_size_guards_cover_encode_and_decode) or test(lifecycle_accepts_active_push_preserves_recorder_failures_and_rejects_stale_push) or test(closed_session_rejects_connect_like_rpcs_before_touching_the_transport) or test(start_keep_alive_ignores_non_positive_intervals) or test(close_is_idempotent_and_joins_the_single_reader) or test(request_observability_snapshot_serializes_empty_collections_as_arrays) or test(rollback_and_disabled_fallbacks_preserve_the_stored_projection) or test(writes_validate_password_port_and_public_access_like_go) or test(default_onboarding_inputs_and_settings_return_empty_values) or test(empty_store_projects_an_empty_broker_surface) or test(watchlist_group_conversion_marks_every_normalized_duplicate_ambiguous)'`

下一片：继续 P1 SQLite/SettingsFile, Trading execution reconciliation 与 Watchlist persistence 映射。

## 第 169 批：SQLite/SettingsFile、Trading reconciliation 与 Watchlist persistence（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/watchlist/futu/source_test.go:174,221,396,418`; `quote_preview_boundaries_test.go:12,37`; `service_quotes_test.go:62,117,210,230,270`; `service_test.go:678` | watchlist rate-limit/fallback, quote cache/singleflight, extended snapshot and revision-fenced import tests | P1 | partial：Watchlist/quote cache 与 provider switch 有真实 owner 证据；Go source/service facade 聚合保持 partial。 |
| [x] | `internal/store/backtest/store_failure_test.go:121`; `store_test.go:168`; `sync_tasks_test.go:12`; `store/exchangecalendar/store_boundaries_test.go:45`; `store_snapshot_failures_test.go:95` | backtest maintenance/cancel, run lifecycle, sync task, calendar store idempotence and snapshot validation tests | P1 | exact：store cancel/lifecycle、sync snapshot、calendar empty/delete 与 year fallback 逐项通过。 |
| [x] | `internal/store/settingsfile/normalization_and_persistence_test.go:13`; `rollback_test.go:13,185,232`; `store_recovery_test.go:13,27,55,87,111,158` | settings normalization, save/bootstrap/account rollback, malformed input, unknown fields and security/MCP replacement tests | P1 | partial：SettingsFile persistence/rollback/recovery 有 Rust 证据；Go managed account facade 聚合保持 partial。 |
| [x] | `internal/store/sqliteconn/conn_test.go:12`; `coordinator_test.go:44,58,83`; `db_api_test.go:109`; `db_concurrency_test.go:113`; `sqliteschema/schema_boundaries_test.go:30`; `store/strategy/runtime_activity_test.go:254` | sqlite open/lock/read barrier, transaction rollback, cancellation, downgrade guard and catalog activity paging tests | P1 | exact：SQLite owner lock、transaction atomicity、cancellation、schema downgrade 与 activity page 均有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 25/25 通过；未发现需要先红后修的生产差异。Watchlist/SettingsFile facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(security_snapshot_coordinator_classifies_remote_rate_limit) or test(test_basic_quote_availability_rejection_enters_delayed_fallback_and_reconcile_succeeds) or test(quote_cache_serves_repeat_reads_within_one_provider_generation) or test(futu_snapshot_route_projects_cached_extended_quote_contract) or test(malformed_identity_and_unbounded_pages_fail_closed) or test(test_quote_cache_and_import_helpers_keep_absent_data_explicit) or test(rejected_provider_switch_preserves_the_current_quote_cache) or test(provider_switch_drops_quotes_cached_under_the_previous_provider) or test(concurrent_group_updates_commit_with_revision_fence) or test(test_store_canceled_maintenance_does_not_mutate_runs) or test(test_in_memory_store_implements_run_lifecycle_and_cancellation) or test(sync_task_cancel_distinguishes_missing_active_and_terminal) or test(test_calendar_store_empty_load_and_delete_are_idempotent) or test(test_save_snapshot_validates_inputs_and_resolves_year_fallbacks) or test(test_settings_normalization_handles_fallbacks_and_boundaries) or test(test_failed_setting_saves_rollback_all_runtime_state) or test(test_failed_bootstrap_and_migration_rollback_runtime_state) or test(test_failed_managed_account_crud_rolls_back_backing_array) or test(missing_empty_and_corrupted_documents_have_distinct_behavior) or test(product_corpus_replays_frozen_compatibility_and_preserves_unknown_fields) or test(appearance_round_trip_preserves_unknown_go_owned_fields) or test(security_settings_writes_match_frozen_compatibility_expectations) or test(listener_failure_rolls_back_the_persisted_settings) or test(opening_database_with_damaged_dynamic_schema_fails_closed_without_replacing_file) or test(exclusive_lock_conflicts_and_file_survives_release)'`

下一片：继续 P1 Trading execution fill reconciliation、Watchlist store failure 与 Strategy catalog/runtime 映射。

## 第 170 批：Trading execution reconciliation、Watchlist store 与 Strategy catalog/runtime（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/store/sqliteconn/coordinator_test.go:83`; `db_api_test.go:109`; `sqliteschema/schema_boundaries_test.go:30`; `store/strategy/runtime_activity_test.go:254` | read barrier cancellation, transaction commit/rollback, schema downgrade and catalog activity paging tests | P1 | exact：SQLite read barrier/transaction、downgrade guard 与 catalog activity 逐项通过。 |
| [x] | `internal/store/trading/broker_fill_reconciliation_test.go:11,97,144`; `execution_composition_test.go:313`; `ledger_test.go:236`; `out_of_order_reconciliation_test.go:43,96,140,178,227` | external order discovery, snapshot fill coverage, stale regression, cancel race, persistence queue and terminal idempotence tests | P1 | exact：fill/order reconciliation、monotonic progress、cancel race 与 ledger reopen 均有真实 owner 证据。 |
| [x] | `internal/store/watchlist/delete_transaction_rollback_test.go:9,84`; `import_test.go:47`; `items_query_plan_test.go:12`; `storage_failure_boundaries_test.go:292`; `store_availability_and_filters_test.go:44` | revision-fenced delete/replacement/import, grouped read pages, rollback and corrupted database guards | P1 | exact：Watchlist mutation/preview/revision、pagination 与 storage failure rollback 逐项通过。 |
| [x] | `internal/pineworkerassets/asset_selection_boundaries_test.go:30`; `internal/strategy/catalog/activity_degraded_test.go:65`; `catalog_boundary_behavior_test.go:34,192`; `runtime_reconciliation_business_test.go:12,52,80,113`; `errors_test.go:8`; `instancebinding/binding_test.go:137` | asset availability, empty activity, binding normalization, runtime transition/reconcile and sentinel error tests | P1 | partial：Strategy catalog/runtime recovery 和 asset guard 有 Rust 证据；Go runtime facade 的跨层聚合保持 partial。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 31/31 通过；未发现需要先红后修的生产差异。Strategy runtime facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(exclusive_lock_conflicts_and_file_survives_release) or test(payload_and_session_event_commit_or_rollback_together) or test(test_p1_06_downgrade_strictly_rejected_at_all_layers) or test(catalog_activity_paging_and_filters_match_go_boundaries) or test(reconciliation_discovers_external_order_into_empty_ledger) or test(conformance_partial_full_fill_average_and_out_of_order_updates_hold) or test(reconciliation_snapshot_coverage_prevents_duplicate_fill_quantity) or test(reconciliation_snapshot_coverage_caps_partial_and_exhausted_credit) or test(reconciliation_rejects_terminal_and_partial_status_regressions) or test(reconciliation_cancel_submitted_resolves_fill_and_cancel_confirmation) or test(execution_order_concurrent_writes_and_reads_survive_reopen) or test(reconciliation_older_snapshot_fill_progress_is_accepted_with_monotonic_updated_at) or test(reconciliation_duplicate_terminal_snapshot_is_idempotent) or test(watchlist_group_mutations_are_revision_fenced_and_survive_restart) or test(watchlist_membership_mutations_and_preview_commit_lifecycle) or test(watchlist_read_pages_preserve_filters_groups_sources_and_remote_catalog) or test(watchlist_store_rejects_missing_drifted_and_corrupted_go_databases) or test(test_select_from_fs_treats_missing_and_empty_bundles_as_unavailable) or test(test_catalog_activity_returns_empty_pages_when_activity_store_is_unavailable) or test(binding_normalization_validates_execution_enums_and_nested_account) or test(sqlite_test_cutover_preserves_repeated_transitions_rollback_and_restart) or test(recovery_failure_converges_the_running_instance_to_stopped) or test(runtime_exit_converges_to_stopped_with_audit_notification_and_error_log) or test(startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances) or test(test_classified_strategy_errors_match_sentinel_kinds)'`

下一片：继续 P1 Strategy live runtime/Pine executor 与 Pine parser/runtime contract 映射。

## 第 171 批：Strategy live runtime、Pine executor 与 runtime contracts（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/strategy/instancebinding/binding_test.go:137`; `live_command_business_boundaries_test.go:468,545`; `liveruntime/manager_boundaries_test.go:155`; `manager_close_test.go:86,188`; `order_risk_business_test.go:135,216`; `pineworker_live_business_test.go:226,404`; `runtime_boundaries_test.go:258`; `subscription_lifecycle_test.go:14`; `symbol_failure_business_test.go:57` | broker binding, order cancel aliases, exact provider selection, runtime close/timeout, market timezone, stateful checkpoint and subscription rollback tests | P1 | partial：live runtime ownership、cancel/timeout、provider binding 与 checkpoint 有 Rust 证据；Go manager facade 聚合保持 partial。 |
| [x] | `internal/strategy/pine_live_executor_test.go:428,443,601,648`; `pineruntime/runner_lifecycle_test.go:41,59,110,157`; `runtime_failure_contracts_test.go:30,146,171`; `runtime_test.go:165,278,344`; `runtimecontrol/policy_test.go:83`; `service_test.go:278` | live command cancel/success audit, session watcher/pinning, runtime fallback/readiness, loopback validation and service stop lifecycle tests | P1 | exact：executor cancel/audit、session pin/close、fallback/readiness、loopback guard 与 service stop 均逐项通过。 |
| [x] | `pkg/strategy/indicatorwarmup/spec_parse_business_test.go:198`; `warmup_internal_test.go:38,142`; `ir/planner_business_boundary_test.go:165` | indicator requirement validation, interval fallback and strategy requirement planning tests | P1 | exact：requirements、interval fallback 与 planner nil/unsupported handling 均有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 37/37 通过；未发现需要先红后修的生产差异。live runtime manager 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(test_normalize_broker_account_drops_empty_input) or test(market_rules_ignore_missing_non_positive_and_non_finite_constraints) or test(test_execute_strategy_intents_cancel_all_queries_and_cancels_active_orders) or test(explicit_broker_that_is_not_the_active_provider_is_rejected_without_fallback) or test(test_strategy_runtime_idempotent_lifecycle_operations) or test(test_strategy_stop_bounded_timeout_on_synchronously_blocking_task) or test(strategy_market_day_start_follows_the_order_market_timezone) or test(pine_runtime_checkpoint_restores_latest_bar_and_intent_keys) or test(close_rejects_a_different_session_or_a_stale_explicit_revision) or test(start_request_accepts_explicit_market_and_code_without_symbol) or test(strategy_runtime_holds_exact_kline_demand_until_stop_and_shutdown) or test(timed_out_stop_keeps_owner_and_releases_store_during_blocking_quote_io) or test(test_execute_strategy_intents_cancel_dispatches_order_cancel) or test(test_execute_strategy_intents_success_calls_execution_and_audits) or test(monitor_shutdown_marks_state_and_joins_task) or test(live_sessions_are_pinned_and_open_failure_rolls_back) or test(closing_a_live_session_releases_its_pin) or test(runtime_path_precedence_prefers_settings_then_worker_env_then_legacy_binary) or test(stopped_state_ignores_late_health_success) or test(test_select_from_fs_returns_embedded_bundle_metadata) or test(live_session_contract_requires_identity_mode_and_revisions) or test(validates_loopback_worker_boundary_before_spawn) or test(strategy_market_day_start_follows_dst_transition) or test(validation_payload_matches_go_owner_field_set_and_defaults_requirements)'`

下一片：继续 P1 Pine parser/lowering/runtime worker contracts 与 Trading transport/API boundaries。

## 第 172 批：Pine parser/lowering、worker contracts 与 runtime boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/strategy/indicatorwarmup/warmup_internal_test.go:38,142`; `ir/planner_business_boundary_test.go:165`; `pine/parse_collection_test.go:233,678`; `parse_object_test.go:289`; `parse_request_test.go:141`; `parse_semantic_test.go:287,301` | indicator/planner, collection/object methods, v14 fixture, stateful TA/session and framework feature compiler tests | P1 | partial：Pine parser/compiler feature coverage 有 Rust 证据；Go parser diagnostic 聚合保持 partial。 |
| [x] | `pkg/strategy/pine/parse_test.go:390,790,838`; `parser_and_lowering_recovery_test.go:8,19,43,119,162,206,235`; `parser_helper_boundaries_test.go:11`; `parser_recovery_boundaries_test.go:12`; `validation_semantics_boundaries_test.go:70` | order subset/UDF/static loop, malformed recovery, control-flow diagnostics, tuple/named arg validation, annotation handling and constant fallback tests | P1 | exact：parser recovery、UDF/static loop、validation diagnostics、annotations 与 fallback warnings 逐项通过。 |
| [x] | `pkg/strategy/pineengine/pine_ts_payload_test.go:190,207,224`; `pinespec/skill_metadata_test.go:80`; `pineworker/client_test.go:92`; `manager_readiness_recovery_test.go:15`; `manager_test.go:74,119,359`; `payload_size_test.go:26`; `process_launcher_boundaries_test.go:70,118`; `types_test.go:105` | worker error payload/startup, Pine spec metadata, endpoint/token, readiness/pinning, pool/start guards, payload size, checksum/termination and live session contract tests | P1 | partial：worker wire/startup/readiness/termination 有真实 owner 证据；Go worker manager facade 聚合保持 partial。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 21/21 通过；未发现需要先红后修的生产差异。Pine parser diagnostics 与 worker manager facade 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(pine_indicators_reject_zero_periods_without_partial_results) or test(test_resolve_interval_minutes_supports_broker_intervals_and_safe_fallbacks) or test(native_pipeline_parses_lowers_and_plans_strategy_requirements) or test(compile_supports_framework_language_features) or test(compile_v14_window_momentum_fixture_keeps_go_requirement_keys) or test(compile_accepts_strategy_order_subset_scripts) or test(compile_accepts_expression_udf_and_static_for_unroll) or test(validate_script_reports_supported_udf_and_static_for_boundaries) or test(parser_handles_strings_history_and_nested_calls_without_regex) or test(semantic_checker_rejects_non_boolean_conditions_and_unsupported_declarations) or test(validation_payload_matches_save_hint_and_rejection_contract) or test(history_references_ignore_string_literals) or test(compile_warns_when_strategy_constants_fall_back_to_defaults) or test(pine_shadow_error_payload_keeps_the_worker_failure_message) or test(validates_loopback_worker_boundary_before_spawn) or test(command_error_summary_keeps_output_tail_within_wire_budget) or test(pine_spec_freezes_go_owner_sections_and_key_payload_fields) or test(endpoint_and_token_boundaries_fail_closed) or test(monitor_shutdown_marks_state_and_joins_task) or test(closing_a_live_session_releases_its_pin) or test(reserving_an_open_session_blocks_a_second_open)'`

下一片：继续 P1 Trading broker/API transport、execution combo 与 control-plane boundaries。

## 第 173 批：Trading broker/API transport、execution combo 与 control-plane（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `pkg/strategy/pineworker/manager_test.go:359`; `payload_size_test.go:26`; `process_launcher_boundaries_test.go:70,118`; `types_test.go:105` | worker dependency/start, message size, checksum/termination and live-session contract tests | P1 | exact：worker dependency guard、payload boundary、checksum、termination 与 identity/revision contract 均通过。 |
| [x] | `internal/trading/broker_boundaries_test.go:11`; `broker_conformance_test.go:58`; `broker_test.go:453,533` | broker read fallback, cancel conformance, portfolio projections, place/cancel/unlock and inactive writer guards | P1 | partial：broker read/write and portfolio fallback 有 Rust 证据；Go service facade 聚合保持 partial。 |
| [x] | `internal/trading/control_plane_idempotency_test.go:65,99`; `control_plane_state_audit_test.go:99,250`; `execution_combo_lifecycle_test.go:15,635`; `execution_products_test.go:12,193` | hard-stop idempotence/scope, state audit failure, combo lifecycle/details, derivative preview and client-id hash tests | P1 | exact：hard-stop、state audit、combo preview/place/cancel、derivative lock 与 stable client IDs 逐项通过。 |
| [x] | `internal/trading/execution_test.go:47,173,687,738,939,965`; `order_status_test.go:33`; `order_update_recovery_test.go:11,68`; `order_updates_reconnect_test.go:12,68`; `order_updates_test.go:263,442`; `responses_test.go:141`; `risk_status_broker_boundaries_test.go:13,187` | order normalization/facade/risk, kill-switch persistence, canonical status, history recovery, subscription reconnect, response serialization and hard-stop scope tests | P1 | partial：execution/risk/reconnect/history recovery 均有 Rust 证据；Go transport worker 聚合保持 partial。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 50/50 通过；未发现需要先红后修的生产差异。broker facade 与 transport worker 聚合保持 partial，不把拆分证据升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(empty_pools_and_unknown_workers_fail_closed) or test(grpc_request_message_limit_has_exact_encoded_boundaries) or test(checksum_is_verified_before_worker_asset_is_written) or test(readiness_policy_preserves_go_delay_and_supports_capped_backoff) or test(live_session_contract_requires_identity_mode_and_revisions) or test(test_service_broker_read_operations_return_fallback_when_market_data_unavailable) or test(portfolio_views_return_fallback_keys_when_market_data_runtime_is_absent) or test(reconciliation_cancel_submitted_resolves_fill_and_cancel_confirmation) or test(cancel_rejects_missing_terminal_and_unidentified_persisted_orders) or test(portfolio_cash_balances_prefer_currency_rows_over_summary_fallback) or test(portfolio_cash_balances_fall_back_to_market_currency_when_summary_currency_is_absent) or test(position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback) or test(portfolio_cash_balances_carry_created_at_alongside_updated_at) or test(broker_adapter_place_and_cancel_keep_server_order_identity_and_submitted_status) or test(broker_unlock_route_forwards_password_md5_and_unlock_flag_to_opend) or test(broker_write_routes_reject_an_inactive_broker_before_the_trade_writer) or test(control_plane_hard_stop_release_is_single_shot) or test(control_plane_hard_stops_block_until_every_entry_released) or test(blank_persisted_state_loads_as_available_fresh_plane) or test(directory_state_path_reports_unavailable_plane_with_load_error) or test(unavailable_plane_mutations_report_control_plane_unavailable) or test(port_update_risk_validates_limits_before_mutating) or test(port_release_missing_hard_stop_reports_not_found) or test(hard_stop_rejection_audit_failure_degrades_the_snapshot_and_blocks_later_mutations) or test(option_combo_lifecycle_preview_place_cancel_and_buying_power_keep_go_contract) or test(execution_details_read_boundaries_keep_go_failure_propagation) or test(derivative_single_leg_preview_requires_client_order_id_and_locks_the_place) or test(single_preview_hash_binds_client_order_id) or test(combo_preview_hash_binds_client_order_id) or test(test_normalize_execution_order_supports_extended_us_limit_sessions) or test(execution_read_port_normalizes_filters_and_routes_order_identifiers) or test(pre_trade_risk_snapshot_uses_empty_vectors_not_null) or test(kill_switch_and_hard_stop_survive_a_restart_with_rejection_audit) or test(unreadable_persisted_state_fails_closed_and_blocks_mutations) or test(test_normalize_execution_order_uses_env_fallback_and_supports_non_limit_us_sessions) or test(test_reconcile_canonical_order_status_prevents_broker_regressions) or test(reconciliation_replays_history_fill_and_fee_once_after_restart) or test(reconciliation_keeps_external_unavailable_as_retryable_without_unknown_write) or test(reconciliation_no_candidate_preserves_unknown_submission) or test(reconciliation_worker_projects_inactive_source_connectivity_and_go_shaped_invalidations) or test(subscribe_trade_accounts) or test(security_snapshot_coordinator_honors_cancellation_of_a_coalesced_wait) or test(trade_push_subscription_failure_surfaces_a_transport_error) or test(failed_unsubscribe_is_deferred_until_its_retry_window) or test(reconciliation_polling_throttles_scans_until_a_push_wake_forces_one) or test(reconciliation_worker_bounds_recent_invalidations_to_twenty_entries)'`

下一片：继续 P1 broker capability/research routes、remaining Trading read/write and API transport mappings。

## 第 174 批：Broker capability/research 与剩余 Trading read boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/trading/responses_test.go:141`; `risk_status_broker_boundaries_test.go:13,187` | broker/portfolio projection, risk hard-stop scope, fallback display and terminal classification tests | P1 | partial：Trading response/risk helper 逐项通过；Go service aggregation 保持 partial。 |
| [x] | `pkg/broker/catalog_test.go:98,233`; `market_rules_snapshot_errors_test.go:63,85`; `product_capability_contracts_test.go:26`; `research_screen_test.go:11` | capability catalog filters/fallback, provider generation recovery, snapshot rate-limit/fallback eligibility, operation surface and research normalization tests | P1 | exact：broker capability/research identity、snapshot errors 与 fallback eligibility 均有真实 owner 证据。 |

本批 9 条 Go 映射对应 Rust 精准 nextest 12/12 通过；P1 中当前仍有一个无实际 Rust 测试的 `session_context_snapshot` 条目，保持未验证并留待边界审查。未发现需要先红后修的生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(helper_market_data_providers_project_futu_broker_and_portfolio_routes) or test(pre_trade_risk_hard_stop_scope_matrix_matches_market_symbol_account_and_broker) or test(hard_stop_environment_scope_trims_case_and_supports_wildcards) or test(preserves_broker_error_display_and_snapshot_fallback_rules) or test(position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback) or test(stored_status_and_terminal_classification_match_go_contract) or test(capabilities_filters_by_broker_market_and_feature_id) or test(provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update) or test(snapshot_rate_limits_preserve_retry_delay_and_context) or test(snapshot_availability_kinds_control_fallback_eligibility) or test(ui_surface_ids_and_operation_overrides_follow_the_catalog_route_table) or test(normalization_and_field_errors_match_the_go_owner_corpus)'`

下一片：P1 有效映射已耗尽；继续处理 P2 Strategy/Pine、Trading/API、Storage/Settings 与边界审查。

## 第 175 批：Assistant ADK API routes 与 workflow transport（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/assistant/adk_approval_test.go:16,183,282,382,450`; `adk_ops_test.go:18,216,286,394` | approval approve/deny、provider 引用删除、goal pause/resume、metrics、task/memory workflow 与 optimization negative route | P2 | exact：审批 envelope、持久化 pause/resume、指标聚合、引用保护与错误码逐项通过。 |
| [x] | `internal/api/assistant/adk_ops_test.go:448,468`; `adk_routes_test.go:165,359,420,481,552,578,634,707,741,858,911` | legacy route 404、snapshot/tools catalog、audit/read、chat/stream terminal recovery、invalid payload、provider/agent/skill mutation 与 run/approval negative routes | P2 | exact：API envelope、stream replay、工具失败、输入校验、skill 绑定与幂等审批均有真实 owner 断言。 |
| [x] | `internal/api/assistant/adk_transport_contracts_test.go:10`; `adk_workflow_routes_test.go:15,262`; `catalog_failure_contracts_test.go:17`; `query_encoding_contracts_test.go:13` | stream event identity、workflow run/trigger/webhook secret、workflow not-found、catalog read fault 与 query encoding | P2 | partial：workflow invalid-input 聚合中 Rust 按 route fixture 拆分断言，未将跨 handler 聚合升为 exact；其余 wire/error contract exact。 |
| [x] | `internal/api/assistant/routes_boundary_contracts_test.go:15,92,278` | unavailable ADK ports、store close failure 与 catalog boundary status | P2 | exact：runtime 缺失、持久化故障与 boundary status 均 fail-closed 并保持 Go 错误分类。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 32/32 通过；未发现需要先红后修的生产差异。`TestADKWorkflowRoutesRejectInvalidInputs` 保持 `partial`，因为 Rust 将 read-resource miss 与 workflow mutation not-found 拆为两个 owner 测试，聚合入口没有同形单一测试。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(adk_approval_approve_returns_the_running_resolution_envelope) or test(adk_approval_deny_returns_the_resolution_envelope_without_a_sync_message) or test(adk_provider_delete_reports_the_in_use_agent_and_keeps_the_go_projection) or test(production_adk_goal_pause_and_resume_are_persisted_atomically) or test(goal_pause_rejects_child_runs_and_resume_reports_missing_runs) or test(production_adk_metrics_are_aggregated_from_persisted_records) or test(adk_metrics_route_ignores_unexpected_query_params) or test(production_adk_local_mutations_persist_tasks_memory_and_workflow_triggers) or test(optimization_task_negative_routes_match_the_reference_matrix) or test(legacy_assistant_chat_route_is_strictly_not_found) or test(adk_snapshot_and_tools_routes_return_the_composed_catalog) or test(adk_audit_route_filters_by_kind_and_subject_id) or test(a_completed_run_with_a_failed_tool_replays_as_the_chat_envelope) or test(a_completed_run_with_a_failed_tool_replays_as_a_stream_final_frame) or test(a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame) or test(adk_chat_stream_replays_go_wire_fixture_for_leaf_owned_cases) or test(adk_provider_save_rejects_the_truncated_payload_with_the_reference_message) or test(adk_agent_write_reports_the_go_validation_messages) or test(adk_skill_install_and_uninstall_failures_keep_the_go_codes) or test(adk_agent_write_accepts_preinstalled_external_and_builtin_skills) or test(adk_cancel_run_missing_uses_the_go_cancel_error_code) or test(adk_approval_negative_and_idempotent_routes_match_the_go_envelopes) or test(adk_read_streams_preserve_event_ids_and_payloads) or test(workflow_run_and_trigger_routes_keep_the_go_error_codes) or test(workflow_webhook_secret_lifecycle_stays_sanitized_and_authenticated) or test(adk_read_resource_misses_keep_the_go_route_error_codes) or test(workflow_mutation_routes_keep_the_go_not_found_codes) or test(catalog_read_faults_expose_the_go_resource_error_codes) or test(adk_read_routes_reject_malformed_query_encoding) or test(wired_but_unavailable_adk_ports_fail_closed_on_every_route) or test(adk_routes_surface_durable_store_failures_instead_of_empty_success) or test(adk_catalog_boundary_status_codes_match_the_go_matrix)'`

下一片：继续 P2 Assistant、Backtest/API transport 与 Settings/Storage 映射；P1 仍保留 `session_context_snapshot` 这一无实际 Rust 测试的边界条目，不能以零测试通过替代结论。

## 第 176 批：Assistant route errors、Backtest routes 与 HTTP bindings（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/assistant/routes_boundary_contracts_test.go:369`; `routes_error_contracts_test.go:14,98`; `routes_identifier_validation_test.go:12`; `routes_resource_contracts_test.go:15,115,159,261`; `routes_test.go:92,101` | malformed JSON、query/resource errors、business validation、blank identifiers、task/memory CRUD、catalog templates、provider/agent validation/default 与 storage classification | P2 | partial：provider/agent validation 聚合保留 snapshot/provider boundary 的拆分差异；其余 route error/resource contracts 逐项通过。 |
| [x] | `internal/api/assistant/routes_test.go:136,265,301,348,366`; `workflow_routes_test.go:14,185` | retained stream replay/terminal recovery、live chat final frame、idempotency/message-field/approval wire 与 workflow canvas execution/error routes | P2 | partial：stream hub/Chat SSE 聚合由 Rust replay/production stream 两个 owner 测试覆盖，保持 partial；其余 idempotency、approval 与 workflow contracts exact。 |
| [x] | `internal/api/backtest/routes_boundaries_test.go:19,32,53,98`; `routes_progress_test.go:143,160`; `routes_test.go:21,35,52,58,85` | empty/missing result、start/sync validation、task persistence、cancel URI branch、delete/store failures、sync adapter errors、trailing JSON 与 queued/terminal result | P2 | partial：Go sync request 的跨字段聚合和 cancel handler branch 仍由拆分 fixture 证据支撑；其余 backtest route 状态、错误优先级与结果持久化 exact。 |
| [x] | `internal/api/httpserver/bindings_boundaries_test.go:13,37` | optional bool alias table、force-refresh query 与 documented candle period families | P2 | exact：HTTP query binding 的别名、布尔语义与 period normalization 逐项通过。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 37/37 通过；未发现需要先红后修的生产差异。Assistant stream/SSE、provider validation 与 Backtest handler 聚合保持 `partial`，没有把拆分测试升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(adk_go_boundary_malformed_json_subset_rejects_at_the_wire) or test(adk_read_and_mutation_routes_keep_the_go_error_classification) or test(adk_session_and_run_update_routes_keep_the_go_business_error_codes) or test(adk_mutation_routes_reject_blank_decoded_identifiers) or test(adk_task_and_memory_crud_contracts_match_go) or test(production_adk_catalog_templates_and_delete_agent_contracts_hold) or test(adk_agent_write_reports_the_go_validation_messages) or test(snapshot_and_provider_test_boundaries_fail_closed) or test(provider_default_contract_orders_the_default_first_and_keeps_the_route_code) or test(agent_save_storage_failure_is_not_client_classified) or test(adk_run_input_response_route_accepts_then_conflicts) or test(adk_chat_stream_replays_retained_terminal_events_through_adk_read_after_restart) or test(recovered_terminal_stream_frame_carries_the_replay_marker) or test(production_live_chat_stream_emits_session_before_run_and_terminal_frame) or test(adk_chat_idempotency_contract_matches_the_go_routes) or test(adk_chat_request_uses_only_the_declared_message_field) or test(adk_chat_approval_is_listed_as_pending_and_denied_with_ok_envelope) or test(workflow_webhook_secret_lifecycle_stays_sanitized_and_authenticated) or test(test_canvas_workflow_multi_node_execution_and_context_propagation) or test(disabled_workflow_and_webhook_routes_keep_the_go_error_codes) or test(workflow_mutation_routes_keep_the_go_not_found_codes) or test(backtest_empty_list_serializes_null_runs_and_missing_result_is_not_found) or test(backtest_start_route_rejects_malformed_json_and_missing_strategy) or test(backtests_write_fixture_replays_all_four_go_owned_mutations) or test(production_sync_read_projects_persisted_task) or test(backtests_write_cancel_blank_id_preserves_go_route_branch) or test(backtest_delete_route_maps_run_store_failure_to_internal_server_error) or test(backtests_read_routes_match_group_fixture_in_cutover_only) or test(backtest_delete_route_reports_not_found_when_terminal_run_disappears) or test(sync_request_rejects_invalid_ranges_and_intervals) or test(backtest_sync_route_maps_adapter_failure_to_sync_failed) or test(backtests_write_leaf_preserves_trailing_json_and_error_precedence) or test(backtest_start_route_maps_request_and_provider_failures) or test(production_backtest_start_executes_fixture_and_persists_terminal_result) or test(optional_query_bool_matches_the_reference_alias_table) or test(snapshot_route_force_refresh_bypasses_the_cache) or test(documented_candle_period_families_match_the_go_table)'`

下一片：继续 P2 Backtest、MarketData/Calendar、Strategy/Pine 与 API transport；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 177 批：HTTP bindings、SSE writer 与 WebSocket live transport（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/httpserver/bindings_boundaries_test.go:109`; `bindings_test.go:14,56,75,100,224` | query map/path fallback、UTC query time、URI percent escape、optional values 与 response envelope request id | P2 | partial：raw-path fallback 与 response envelope 聚合在 Rust 由拆分 transport/query owner 覆盖，保持 partial；其余 binding matrix exact。 |
| [x] | `internal/api/httpserver/sse_boundaries_test.go:30,46,60,86`; `sse_concurrent_test.go:42`; `sse_test.go:41,55,87,129,136,146,196` | SSE serialization/write/flush panic、trigger/ticker loop、concurrent frame serialization、retry/id/comment formatting 与 nil ticker | P2 | exact：SSE writer 错误传播、并发序列化、frame 格式与 loop 生命周期逐项通过。 |
| [x] | `internal/api/live/dispatcher_boundaries_test.go:20,59,120,219,257,295` | initial/live failures、auxiliary subscriptions、trigger/ticker errors、broker selection、nil/closed lifecycle 与 depth demand union | P2 | partial：Go dispatcher facade 的多订阅/后端聚合由 websocket compatibility 与 subscription owner 拆分覆盖，未升为 exact。 |
| [x] | `internal/api/live/handler_test.go:113,207,244,316,342,397` | heartbeat/subscribe normalization、provider/origin/limit guards、depth payload 与 notification replay | P2 | partial：provider-missing、depth/notification payload 聚合保持 partial；origin、limit、heartbeat 和 subscription normalization 有 exact owner 证据。 |

本批 30 条 Go 映射对应 Rust 精准 nextest 30/30 通过；未发现需要先红后修的生产差异。HTTP path/response、live dispatcher 与 handler 聚合保持 `partial`，没有把 WebSocket corpus replay 误升为全部 handler exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(query_map_handles_percent_plus_and_multi_values) or test(optional_query_time_normalizes_to_utc_and_blank_means_absent) or test(query_time_parses_rfc3339_datetime_and_date) or test(uri_escape_validation_accepts_literal_percent_and_rejects_malformed) or test(execution_write_leaf_preserves_null_trailing_json_and_percent_id_trim) or test(optional_query_bool_matches_the_reference_alias_table) or test(candle_route_treats_blank_limit_as_unset_and_rejects_non_integer) or test(desktop_token_reaches_port_with_stable_envelope_and_request_id) or test(write_event_propagates_serialization_and_write_failures) or test(stream_loop_ignores_trigger_without_callback) or test(stream_loop_propagates_trigger_and_ticker_failures) or test(write_failures_are_reported_with_their_source_message) or test(concurrent_writers_serialize_frames) or test(write_event_returns_flush_panics_as_errors) or test(write_retry_returns_write_panics_as_errors) or test(prepare_writes_retry_then_id_event_and_comment_frames) or test(prepare_rejects_a_sink_without_flush_support) or test(stream_loop_propagates_initial_error_before_any_tick) or test(stream_loop_runs_trigger_and_ticker_ticks) or test(stream_loop_handles_nil_ticker_channel) or test(ws_live_replays_complete_go_corpus) or test(auxiliary_provider_failure_skips_only_the_failing_subscription_family) or test(live_connection_limit_is_never_zero) or test(active_instruments_are_normalized_unioned_replaced_and_released) or test(ws_live_transport_accepts_trusted_origin_and_streams_heartbeat_first) or test(subscription_normalization_matches_the_go_table) or test(subscription_message_matches_go_ignore_update_and_close_rules) or test(order_book_pushes_publish_depth_for_the_subscribed_instrument) or test(ws_live_transport_rejects_origin_and_limit_without_leaking_permits) or test(ws_live_transport_rejects_untrusted_origin_before_upgrade)'`

下一片：继续 P2 API transport、MarketData/Calendar、Strategy/Pine 与 Trading/Storage 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 178 批：MarketData route contracts 与 middleware guards（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/live/handler_test.go:421,441,480`; `internal/api/marketdata/routes_boundaries_test.go:35,63,97,119,208,387,421,462,502` | same-origin WebSocket、tick dedupe/provider generation、instrument/candles/markets/snapshot、logical lease、poll capability、subscription mutation 与 malformed target guards | P2 | partial：provider failure code 聚合按 Futu/helper owner 拆分，保持 partial；其余 route/lease/query boundary exact。 |
| [x] | `internal/api/marketdata/routes_news_actions_test.go:55,160`; `routes_test.go:22,96,205,239,267,337,357,387,439,459,563,625,725` | news/corporate-action path/capability、instrument subscription/poll/release/clear、legacy candle query/session/period/limit、depth/quote validation、provider failure 与 subset search | P2 | partial：news/actions 与 read-route 聚合跨 provider/transport owner，保持 partial；subscription、candle validation 与 instrument search 有逐项 exact 证据。 |
| [x] | `internal/api/middleware/adk_test.go:12,30`; `internal/api/middleware/auth_test.go:13` | ADK unavailable/available guards、catalog runtime path 与 public auth bypass | P2 | partial：ADK available guard 同时由 isolated-port 与 catalog owner 覆盖，保持 partial；public auth path exact。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 34/34 通过；未发现需要先红后修的生产差异。MarketData provider 聚合与 ADK middleware guard 保持 `partial`，没有将跨 provider 的错误码拆分测试升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(ws_live_transport_accepts_trusted_origin_and_streams_heartbeat_first) or test(repeated_tick_observations_are_deduplicated_per_provider) or test(ws_live_replays_complete_go_corpus) or test(instrument_read_routes_reject_missing_uri_parameters) or test(market_microstructure_quote_routes_preserve_provider_error_mapping) or test(test_non_futu_helper_candles_failure_uses_generic_market_code) or test(markets_route_fails_with_market_data_failed_when_active_provider_is_unavailable) or test(read_candles) or test(snapshot_rejects_malformed_refresh_before_provider_access) or test(live_read_routes_require_a_logical_subscription_lease) or test(poll_only_read_routes_prioritize_capabilities_and_preserve_leases) or test(subscription_mutation_replay_matches_go_fixture_data_errors_and_retry_metadata) or test(subscription_request_helpers_preserve_only_valid_targets) or test(news_actions_helper_request_rejects_a_missing_instrument_uri) or test(research_helper_request_rejects_unsupported_or_malformed_paths) or test(futu_news_queries_stay_on_the_broker_path) or test(production_news_actions_port_maps_helper_failure_and_rejects_bad_limit) or test(subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear) or test(broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease) or test(consumer_only_release_keeps_other_consumer_entry) or test(clear_route_preserves_running_strategy_lease) or test(candle_route_preserves_legacy_query_parsing) or test(candle_route_rejects_unsupported_period) or test(candle_route_forwards_exclusive_before_and_rejects_invalid_combinations) or test(candle_route_rejects_invalid_limit) or test(market_microstructure_depth_route_rejects_invalid_num_before_reader_call) or test(market_microstructure_quote_routes_reject_invalid_queries_before_reader_call) or test(read_routes_map_provider_and_request_failures) or test(instrument_search_route_returns_subset_resolution_contract) or test(instrument_search_route_validates_input_and_maps_provider_failures) or test(wired_but_unavailable_adk_ports_fail_closed_on_every_route) or test(adk_chat_stream_routes_are_isolated_without_port) or test(adk_snapshot_and_tools_routes_return_the_composed_catalog) or test(production_adk_catalog_templates_and_delete_agent_contracts_hold) or test(auth_skips_public_paths_without_credentials)'`

下一片：继续 P2 MarketData/Quote provider、Trading/API transport、Strategy/Pine 与 Storage/Settings 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 179 批：Auth/CORS origin、prediction combo 与 research routes（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/middleware/auth_test.go:28,51,71,78,86,112,119,173`; `security_boundaries_test.go:17,46,62,90,108`; `internal/api/origin/origin_test.go:8,31` | request context/authenticator guards、logout/system-status protection、trusted caller/CSRF/origin/CORS、write method classification 与 canonical/request origin | P2 | partial：context helper 与 write-method 聚合保留 boundary/拆分 owner 结论；认证、CSRF、CORS 与 origin normalization 逐项通过。 |
| [x] | `internal/api/productfeatures/prediction_combo_routes_test.go:15,64` | prediction combo invalid/unpersistable mapping 与 route default query normalization | P2 | partial：Go post-query 上游错误聚合由 prediction read/write ports 拆分覆盖，保持 partial；combo rejection exact。 |
| [x] | `internal/api/productfeatures/provider_research_routes_test.go:179,229,264,335,370,458,498,606` | embedded news/actions、capability/lifecycle errors、rankings/industry、company research、calendar/macro operation mapping and rejection | P2 | partial：embedded provider facade 与错误聚合跨 research/calendar owners，保持 partial；unsupported operation 与 wire kind tests exact。 |
| [x] | `internal/api/productfeatures/research_screen_test.go:19,47,70,80,130` | research catalog/query schema、429 wire、typed definition/page validation 与 executable V2 definition | P2 | partial：catalog/post facade 和 typed definition 聚合保持 partial；schema/version、429 transport 与 validation precedence 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 36/36 通过；未发现需要先红后修的生产差异。Auth context、prediction/research facade 与跨 provider error 聚合继续保持 `partial`/boundary，不以同名 helper 替代完整行为结论。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(current_request_context) or test(auth_protects_logout) or test(auth_protects_system_status_without_credentials) or test(auth_trusted_caller_without_origin_bypasses_csrf) or test(auth_trusted_caller_still_rejects_untrusted_provided_origin) or test(auth_rejects_requests_without_any_authenticator) or test(auth_rejects_untrusted_origin_before_authentication) or test(cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin) or test(authenticated_session_read_without_browser_origin_is_allowed) or test(write_method_classification_covers_state_changing_verbs) or test(auth_treats_patch_as_session_write_requiring_csrf) or test(same_origin_options_preflight_is_allowed_without_reflected_origin) or test(request_origin_uses_production_semantics_without_malformed_origin_fallback) or test(origin_normalization_accepts_web_and_tauri_schemes) or test(prediction_combo_quote_rejects_invalid_and_unpersistable_requests) or test(prediction_read_query_accepts_go_route_defaults) or test(corporate_action_projection_formats_statements) or test(embedded_capability_errors_keep_the_broker_code_and_lifecycle_sentinels) or test(explicit_broker_that_is_not_the_active_provider_is_rejected_without_fallback) or test(rankings_operations_map_to_provider_kinds_on_the_wire) or test(rankings_reject_unmapped_operations_without_a_helper_call) or test(company_research_default_operations_project_on_the_wire) or test(company_research_rejects_non_default_operations) or test(calendar_operations_map_to_provider_reads_on_the_wire) or test(calendar_and_macro_reject_unsupported_operations_without_a_helper_call) or test(embedded_research_facade_serves_exactly_the_allowed_feature_set) or test(research_screen_catalog_route_matches_go_fixture_for_all_variants) or test(screen_query_defaults_the_page_and_keeps_catalog_columns) or test(screen_query_rejects_wrong_catalog_and_schema_versions) or test(futu_screen_write_errors_keep_their_transport_contract) or test(research_screen_fixture_replays_go_wire_for_the_post_route) or test(route_and_page_validation_precede_provider_calls) or test(research_screen_definition_rejects_unsupported_market_and_stable_keys)'`

下一片：继续 P2 research/provider、Backtest/Calendar、Trading/API 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 180 批：Research screen/preset 与 Settings routes（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/productfeatures/research_screen_test.go:185,213,272,355,387`; `productfeatures/routes_test.go:19,93,142,265` | V1/V2 schema rejection、typed result projection、catalog/embedded provider post conflict、prediction/read route、research query/wire and capability errors | P2 | partial：Research screen/product-feature facade 聚合由 schema、provider 与 prediction owners 拆分，保持 partial；schema rejection、projection、conflict and route helper assertions 均有真实证据。 |
| [x] | `internal/api/research/routes_test.go:18,74` | preset CRUD fixture and unavailable-store fail-closed route | P2 | exact：preset create/list/get/delete wire 与缺失 test port 的 fail-closed 行为逐项通过。 |
| [x] | `internal/api/settings/adk_routes_contracts_test.go:18`; `routes_accounts_validation_test.go:18,73`; `routes_failure_boundaries_test.go:20,48,84,125,147,164` | ADK/UI settings persistence, managed account identity/timestamps, malformed JSON, rollback, notification/onboarding/data-management failures | P2 | partial：Settings service facade 与多 route error 聚合保持 partial；malformed JSON、account validation、rollback 与 authenticated data-management owner 有真实断言。 |
| [x] | `internal/api/settings/routes_market_data_test.go:18,115,128`; `routes_test.go:247,304,376,393,419,468,589` | provider read/save/apply, removed legacy YFinance route, validation/runtime errors, legacy response shapes, MCP token one-time, account creation and execution/calendar/broker routes | P2 | partial：Settings route facade/legacy response 聚合保持 partial；provider mutation atomicity、unknown API fallback、account field guards 与 calendar reload 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 28/28 通过；未发现需要先红后修的生产差异。Research/Settings facade 与跨 route 聚合继续保持 `partial`，preset CRUD 与字段/错误边界保持 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(screen_query_rejects_wrong_catalog_and_schema_versions) or test(route_and_page_validation_precede_provider_calls) or test(research_screen_helper_projects_rows_and_cells_without_fixture_defaults) or test(research_screen_catalog_route_matches_go_fixture_for_all_variants) or test(futu_screen_capability_errors_use_the_broker_code) or test(research_screen_fixture_replays_go_wire_for_the_post_route) or test(futu_interval_factor_rejects_non_catalog_operator_before_provider_call) or test(market_data_prediction_read_routes_match_group_fixture_in_cutover_only) or test(company_financials_forwards_market_symbol_and_statement) or test(embedded_capability_errors_keep_the_broker_code_and_lifecycle_sentinels) or test(research_helper_request_parses_canonical_instrument_ids) or test(research_preset_write_fixture_matches_go_owner_for_all_three_routes) or test(research_preset_write_leaf_fails_closed_without_a_test_port) or test(product_server_persists_ui_settings_and_reports_actual_port) or test(adk_snapshot_and_tools_routes_return_the_composed_catalog) or test(update_account_preserves_immutable_identity_and_clears_timestamps) or test(cleanup_preview_route_returns_candidates_and_rejects_bad_payloads) or test(settings_write_routes_reject_malformed_json_before_persistence) or test(test_failed_setting_saves_rollback_all_runtime_state) or test(data_management_overview_is_authenticated_and_does_not_create_missing_databases) or test(test_active_provider_mutation_atomic_single_state_source) or test(unknown_api_is_json_but_frontend_uses_spa_fallback) or test(blank_account_id_is_rejected_before_persistence) or test(create_account_clears_client_owned_identity_and_timestamps) or test(production_calendar_settings_write_reloads_running_manager)'`

下一片：继续 P2 Settings/Storage、Backtest/Calendar、MarketData provider 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 181 批：Strategy/Pine route contracts 与 Settings callback boundaries（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/settings/routes_test.go:709,754,802,935`; `routes_uri_boundaries_test.go:12` | calendar/data-management callback injection、system notification settings port 与 resource URI fallback | P2 | partial：Settings callback facade 聚合保持 partial；missing URI 与 calendar reload/fallback 有真实 owner 证据。 |
| [x] | `internal/api/strategy/pine_routes_contracts_test.go:72,148,182,217,253,398,453,502` | Pine diagnostics/requirements、AST omission、source/syntax validation、V2 parse metadata、object/import/type diagnostics 与 worker shadow projection | P2 | partial：Go diagnostics 聚合与 Rust fixture/worker projection 拆分覆盖，保持 partial；输入校验、opaque projection 与 AST omission 有逐项证据。 |
| [x] | `internal/api/strategy/routes_boundary_contracts_test.go:16,75`; `routes_failure_boundaries_test.go:16,71,138,184,230`; `routes_lifecycle_test.go:203,252,319,401,462,642,689`; `routes_test.go:41,114,146` | definition query/start transitions、Pine error precedence、definition/instance/plugin CRUD failure guards、version history, lifecycle and response defaults | P2 | partial：strategy route facade 与 browser/restart fixtures 保持 partial；query/ID guards、runtime state transitions、version rollback and response defaults 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 26/26 通过；未发现需要先红后修的生产差异。Strategy/Pine 与 Settings callback 聚合继续保持 `partial`，不把 fixture/replay 直接提升为整条 facade exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(production_calendar_settings_write_reloads_running_manager) or test(cleanup_preview_route_returns_candidates_and_rejects_bad_payloads) or test(product_server_persists_ui_settings_and_reports_actual_port) or test(unknown_api_is_json_but_frontend_uses_spa_fallback) or test(strategy_pine_replays_go_fixture_projection_status_and_headers) or test(strategy_pine_applies_input_validation_and_error_precedence_before_the_port) or test(strategy_pine_preserves_worker_shadow_errors_as_successful_projections) or test(strategy_pine_accepts_go_zero_values_and_opaque_empty_or_abnormal_projections) or test(strategy_definition_detail_rejects_invalid_boolean_query_before_the_port) or test(sqlite_test_cutover_preserves_repeated_transitions_rollback_and_restart) or test(strategy_definition_write_product_replays_browser_failure_recovery_and_restart) or test(strategy_definition_routes_match_group_fixture_in_cutover_only) or test(instantiate_accepts_empty_body_but_rejects_malformed_json) or test(malformed_input_precedes_missing_port_and_non_mutations_are_isolated) or test(plugin_uninstall_guidance_route_fails_closed_when_snapshot_port_is_unavailable) or test(create_clears_client_id_and_update_overrides_body_id) or test(sqlite_test_cutover_preserves_versions_rollback_linked_delete_and_restart) or test(strategy_runtime_write_product_replays_browser_failure_recovery_and_restart) or test(strategies_runtime_write_fixture_matches_go_owner_for_all_seven_routes) or test(plugin_uninstall_guidance_route_matches_go_fixture_in_cutover_only) or test(strategy_definition_preview_derives_warmup_bars_and_overrides_preview_parameters) or test(strategy_definition_detail_maps_missing_definition_and_invalid_query)'`

下一片：继续 P2 Strategy/Pine、MarketData provider、Backtest/Calendar 与 Trading/API 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 182 批：Trading execution/broker routes 与 Watchlist route errors（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/strategy/routes_test.go:181`; `internal/api/trading/execution_products_test.go:18,73`; `execution_test.go:19,47,79,110,175,205,249`; `execution_validation_contracts_test.go:52,161,205` | Pine analysis validation, execution buying-power/combo lifecycle, risk/error envelopes, query normalization, preview/events, order details and active/history sync | P2 | partial：execution facade、order detail/active-history 聚合保持 partial；risk shape, query normalization, preview and fixture lifecycle 有真实 owner 证据。 |
| [x] | `internal/api/trading/openapi_route_alignment_test.go:14`; `routes_broker_contracts_test.go:143,214`; `routes_failure_boundaries_test.go:17,58`; `routes_helper_boundaries_test.go:17,119,131`; `routes_read_handlers_test.go:71`; `routes_test.go:95,121,145` | capability adapter reachability, portfolio/broker writes, degraded backend/missing input guards, malformed query, analytics projection and broker read/write route fixtures | P2 | partial：Trading broker facade/read handler 聚合保持 partial；URI/query guards、read route projection与 production failure contracts 有真实 owner 证据。 |
| [x] | `internal/api/watchlist/route_error_handling_test.go:16,50,77`; `routes_business_test.go:126,147` | watchlist write error matrix, binding/delete guards, malformed query, membership conflict and import/group lifecycle | P2 | partial：Watchlist route error 与 lifecycle 聚合由 fixture/SQLite owner 拆分覆盖，保持 partial；fail-closed mutation、query validation 与 revision fencing 有真实证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 31/31 通过；未发现需要先红后修的生产差异。Trading/Watchlist facade 聚合与 `map_trade_error` 的细分错误码缺口保持 `partial`，没有将已有 fixture 断言升为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(strategy_pine_replays_go_fixture_projection_status_and_headers) or test(strategy_pine_applies_input_validation_and_error_precedence_before_the_port) or test(execution_sqlite_test_cutover_replays_transport_and_restart) or test(execution_write_fixture_replays_all_seven_go_owned_mutations) or test(execution_error_envelopes_keep_request_errors_distinct_from_upstream_failures) or test(map_trade_error) or test(real_order_rejects_when_kill_switch_active) or test(single_equity_rejects_event_only_fields) or test(execution_read_routes_match_group_fixture_in_cutover_only) or test(trade_query_normalizes_scope_merges_aliases_and_treats_blank_optionals_as_absent) or test(execution_read_routes_are_not_registered_without_snapshot_port) or test(every_declared_capability_resolves_to_an_executable_production_adapter) or test(position_projection_prefers_diluted_cost_and_account_pnl_with_legacy_fallback) or test(brokers_write_sqlite_test_cutover_replays_transport_and_restart) or test(broker_read_routes_fail_closed_when_snapshot_port_is_unavailable) or test(production_http_broker_reads_reject_missing_and_invalid_optional_inputs) or test(broker_read_fails_closed_without_trade_client) or test(production_http_broker_reads_reject_missing_uri_broker_and_unknown_resource) or test(production_http_broker_reads_reject_malformed_query_encoding_before_dispatch) or test(broker_account_analytics_project_fees_margin_cash_flow_and_buying_power) or test(brokers_write_routes_register_only_with_explicit_test_port) or test(production_http_broker_reads_serve_orders_fills_quotes_klines_and_securities) or test(production_http_broker_reads_reject_invalid_scope_and_numeric_queries) or test(watchlist_write_fixture_matches_go_owner_for_all_eight_routes) or test(watchlist_write_fixture_covers_concurrency_and_every_route) or test(every_watchlist_mutation_fails_closed_without_a_port) or test(query_map_handles_percent_plus_and_multi_values) or test(production_watchlist_read_uses_real_pages_and_remote_catalog) or test(watchlist_memberships_route_matches_go_fixture_in_cutover_only) or test(sqlite_test_cutover_preserves_revision_fencing_import_commit_and_restart) or test(watchlist_write_product_replays_browser_boundary_failure_recovery_and_restart)'`

下一片：继续 P2 Trading/Broker、Watchlist/Storage、MarketData provider 与 Backtest/Calendar 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 183 批：Watchlist routes、application lifecycle 与 database maintenance（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/api/watchlist/routes_test.go:15,34,72,84,107` | unavailable/read/write route envelopes, invalid limits/query encoding and missing URI guards | P2 | partial：missing URI 与跨 read/write route facade 聚合保持 partial；fail-closed port, paging and query guards 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/application/assistant_test.go:45,98`; `installers_test.go:16`; `lifecycle_test.go:11,41`; `resources_test.go:12,34,78,137`; `runtime_dependencies_test.go:11` | typed dependency order, assistant port projection, startup rollback, reverse close/idempotence, late resource ownership and normalized runtime candidate | P2 | partial：installer ownership/runtime dependency facade 保持 partial；lifecycle rollback/order/concurrency 与 assistant projection 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/backtestapp/historical_source_test.go:141,298`; `databaseguard/groups_test.go:14,35` | market-scoped AkShare lookback, typed candle conversion and database availability/healthy route groups | P2 | partial：decimalString 动态类型聚合保持 partial；lookback, strict conversion and database guard states 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:16,70,109,142,167,225,245,270,358,394,455` | maintenance concurrency/lease, backup/compact/rebuild, inspection/retention, candidate schema guards, preview expiry and byte-identical source protection | P2 | partial：maintenance service 多阶段聚合保持 partial；candidate fail-closed、retention/rebuild marker、lease and source immutability 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 39/39 通过；未发现需要先红后修的生产差异。Watchlist/application/maintenance facade 聚合与动态 helper 继续保持 `partial`，不将分层 owner 测试误记为单一 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(watchlist_read_routes_fail_closed_when_snapshot_port_is_unavailable) or test(every_watchlist_mutation_fails_closed_without_a_port) or test(watchlist_read_routes_match_group_fixture_in_cutover_only) or test(production_watchlist_read_uses_real_pages_and_remote_catalog) or test(watchlist_write_fixture_matches_go_owner_for_all_eight_routes) or test(query_map_handles_percent_plus_and_multi_values) or test(malformed_identity_and_unbounded_pages_fail_closed) or test(watchlist_memberships_route_fails_closed_when_snapshot_port_is_unavailable) or test(production_adk_and_plugin_and_alert_ports) or test(production_opend_health_diagnoses_enabled_but_unreachable_opend) or test(test_product_runtime_ordered_shutdown_explicit) or test(test_product_runtime_startup_failure_rollback) or test(test_product_runtime_ordered_shutdown_direct_drop) or test(startup_failure_restores_previously_migrated_descriptor_files) or test(stop_product_is_idempotent_across_concurrent_invocations) or test(runtime_dependencies_use_the_normalized_settings_node_candidate) or test(akshare_lookback_windows_are_scoped_to_the_declared_market) or test(test_historical_candle_conversion_rejects_invalid_fields_and_defaults_volume) or test(production_startup_fails_closed_when_database_is_corrupted) or test(production_startup_fails_closed_on_writer_lease_conflict) or test(production_system_status_reports_real_database_lease_and_schema_state) or test(cleanup_requires_the_exact_approved_candidate_set) or test(soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute) or test(a_held_writer_lease_rejects_maintenance_before_any_snapshot_is_written) or test(backup_and_compact_fail_closed_when_the_source_database_is_missing) or test(managed_backup_discovery_parses_only_canonical_filenames) or test(overview_counts_main_wal_and_shm_and_keeps_an_unreadable_database_local) or test(cleanup_candidates_fail_closed_when_the_maintenance_tables_are_missing) or test(preview_normalizes_defaults_summarizes_and_expires_after_ten_minutes) or test(cleanup_preview_requires_a_ready_database_with_the_purgeable_table) or test(maintenance_service_confirmation_validation_and_rejection_parity) or test(a_corrupt_rebuild_marker_blocks_backup_and_rebuild_without_deleting_data) or test(backup_capacity_surfaces_a_removal_failure_without_reporting_success) or test(database_inspection_classifies_filesystem_and_schema_states) or test(backup_retention_keeps_three_snapshots_and_reports_an_over_quota_snapshot) or test(rebuild_marker_snapshots_survive_later_retention_pressure) or test(backup_retention_never_evicts_snapshots_a_rebuild_marker_references) or test(backup_snapshot_leaves_an_incompatible_source_byte_identical)'`

下一片：继续 P2 database/maintenance、MarketData/Quote provider、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 184 批：Database maintenance manager、backup retention 与 rebuild safety（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/datamigration/maintenance_failure_paths_test.go:502,529,538`; `maintenance_test.go:17,49,82,147,185,227,255,315` | compaction/rebuild-marker failure, secure preview ID, overview/summary, verified backup/quota, backtest cleanup and conflict/invalid operation guards | P2 | partial：maintenance service facade 的多数据库聚合保持 partial；preview expiry, source immutability, backup/quota and candidate guards 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/datamigration/managed_backup_retention_test.go:12,61,105`; `manager_boundaries_test.go:14,50,74,108,163,187,280` | managed snapshot retention/discovery/verification, status/rebuild scheduling, pending marker inspection, manifest drift and filesystem/stat failure handling | P2 | partial：manager status/marker facade 聚合保持 partial；retention, marker normalization, manifest and inspection boundaries 逐项通过。 |
| [x] | `internal/app/apiserver/datamigration/manager_test.go:14,49,75,100,145,190`; `rebuild_safety_test.go:12,77,106` | single/batch rebuild scheduling, maintenance locks, schema descriptors, selected database apply, tampered/delete failure and backup marker safety | P2 | partial：manager orchestration 聚合保持 partial；catalog, lock, tamper/delete safety and cleanup-after-failure 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 32/32 通过；未发现需要先红后修的生产差异。Database maintenance manager 聚合保持 `partial`，各 backup/retention/rebuild owner 的断言逐项记录。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(compaction_fails_closed_when_the_rebuild_marker_is_corrupt) or test(preview_surfaces_a_preview_id_failure_without_storing_the_preview) or test(cleanup_preview_requires_a_ready_database_with_the_purgeable_table) or test(overview_summary_only_skips_storage_and_a_single_filter_keeps_its_totals) or test(overview_counts_main_wal_and_shm_and_keeps_an_unreadable_database_local) or test(backup_snapshot_is_private_verified_and_limited_to_managed_databases) or test(a_held_writer_lease_rejects_maintenance_before_any_snapshot_is_written) or test(a_failed_rebuild_batch_removes_every_snapshot_it_created) or test(maintenance_service_confirmation_validation_and_rejection_parity) or test(backup_capacity_prunes_only_managed_snapshots_and_enforces_quota) or test(backtest_history_preview_prefers_age_and_latest_protection) or test(preview_normalizes_defaults_summarizes_and_expires_after_ten_minutes) or test(overview_lists_cleanable_categories_for_soft_deleted_and_history_rows) or test(preview_rejects_invalid_retention_and_non_ready_databases) or test(cleanup_candidates_fail_closed_when_the_maintenance_tables_are_missing) or test(backup_and_compact_fail_closed_when_the_source_database_is_missing) or test(backup_retention_keeps_three_snapshots_and_prunes_the_oldest_of_one_database) or test(managed_backup_discovery_parses_only_canonical_filenames) or test(snapshot_verification_rejects_invalid_sqlite_and_failed_backups_leave_no_partial_file) or test(overview_preserves_go_order_filter_and_rebuild_projection) or test(overview_normalizes_rebuild_marker_ids_and_fails_closed_when_it_is_unreadable) or test(rebuild_selection_rejects_ambiguous_ids_and_an_empty_incompatible_batch) or test(startup_reports_pending_markers_that_are_corrupt_or_unknown) or test(database_inspection_classifies_filesystem_and_schema_states) or test(database_inspection_rejects_manifest_drift) or test(batch_rebuild_schedules_every_incompatible_database) or test(managed_descriptors_match_the_pinned_schema_catalog) or test(startup_applies_a_pending_rebuild_only_to_the_selected_databases) or test(startup_rejects_a_tampered_pending_rebuild_without_deleting_any_source) or test(startup_keeps_the_marker_when_a_selected_database_cannot_be_deleted) or test(marker_snapshots_must_live_in_the_managed_directory_with_a_managed_name) or test(startup_reports_pending_markers_without_a_verified_backup)'`

下一片：继续 P2 database/maintenance、Storage/SQLite、MarketData provider 与 Backtest/Calendar 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 185 批：Rebuild safety、Futu lifecycle、desktop startup 与 web access（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/datamigration/rebuild_safety_test.go:128,150,211,221`; `research_lifecycle_test.go:13` | marker backup scheduling/idempotence, unreadable/digest safety and research database status/backup/rebuild | P2 | partial：rebuild manager facade 与 research database aggregation 保持 partial；marker/digest, idempotence and source safety 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/desktop_api_startup_test.go:87,148`; `futuapp/coordinator_test.go:15,109`; `runtime_contracts_test.go:44`; `runtime_state_boundaries_test.go:44,51` | embedded asset unavailable semantics, sidecar readiness rollback, Futu reset/order-book registry, onboarding/readiness and OpenD probe/global state | P2 | partial：Futu coordinator/onboarding 聚合保持 partial；asset, sidecar cleanup, probe version/state and reset ownership 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/lifecycle/lifecycle_test.go:90,110,196,222,243,275,331,378,424,459,490,516,555,587,616,711` | disabled/API-only startup, loopback/public bind guards, listener rebind/conflict, origin allowlist, staged startup rollback, rebuild finalize failure and shutdown idempotence | P2 | partial：Go lifecycle facade 的多 listener/embedded frontend 聚合保持 partial；port conflict, startup order/rollback, shutdown and public security guards 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/marketdataapp/assistant_provider_test.go:95,114` | provider status corpus and active provider mutation rollback/selection | P2 | partial：assistant provider facade 与 market-data runtime projection 拆分覆盖，保持 partial。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 31/31 通过；未发现需要先红后修的生产差异。Futu/lifecycle/assistant-provider facade 聚合保持 `partial`，边界条目明确记录 owner 证据。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(a_marker_with_an_unbacked_database_id_blocks_scheduling) or test(rebuild_scheduling_is_idempotent_and_reverifies_marker_snapshots) or test(a_corrupt_rebuild_marker_blocks_backup_and_rebuild_without_deleting_data) or test(unreadable_marker_and_digest_errors_surface_without_evicting_snapshots) or test(research_database_participates_in_status_backup_and_rebuild) or test(required_asset_does_not_fallback_for_missing_static_asset) or test(readiness_failure_reclaims_every_started_process_without_starting_dependents) or test(connection_replacement_clears_quota_ownership_and_reset_is_idempotent) or test(order_book_registry_reset_clears_marks_for_a_replacement_connection) or test(onboarding_settings_writes_replay_frozen_compatibility_cases) or test(production_system_read_reports_unavailable_opend_without_fake_health) or test(probe_from_global_state_enforces_minimum_version_and_maps_neutral_state) or test(tcp_probe_maps_login_global_state_and_market_readiness) or test(disabled_web_access_does_not_start_a_listener) or test(lifecycle_starts_in_dependency_order_and_shuts_down_in_reverse) or test(access_fails_closed_without_a_configured_password) or test(enabled_web_access_binds_and_shutdown_releases_port) or test(port_conflict_keeps_the_previous_listener_running) or test(dynamic_origin_allowlist_tracks_the_current_web_port) or test(product_runtime_without_optional_workers_starts_and_stops_cleanly) or test(startup_keeps_the_marker_when_a_selected_database_cannot_be_deleted) or test(startup_applies_a_pending_rebuild_only_to_the_selected_databases) or test(build_profiles_preserve_tauri_identity_and_data_isolation) or test(test_product_runtime_startup_failure_rollback) or test(readiness_failure_reclaims_every_started_process_without_starting_dependents) or test(test_product_runtime_ordered_shutdown_explicit) or test(stop_product_is_idempotent_across_concurrent_invocations) or test(market_data_provider_read_routes_match_group_fixture_in_cutover_only) or test(market_data_runtime_projection_matches_go_status_corpus) or test(test_active_provider_mutation_atomic_single_state_source) or test(blank_provider_selection_is_rejected_before_persistence) or test(active_failure_rolls_back_but_backtest_failure_never_persists)'`

下一片：继续 P2 lifecycle/MarketData provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 186 批：MarketData provider switching、heartbeat、depth 与 security routes（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/marketdataapp/assistant_provider_test.go:143,159`; `data_plane_switch_test.go:16,66,83,140,189,237,265` | unavailable provider ports, provider switch atomicity/watchlist availability, demand restore/rollback, Python helper warmup, activation readiness and startup restore | P2 | partial：assistant/data-plane facade 聚合保持 partial；atomic switch, demand fencing, readiness and rollback 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/marketdataapp/heartbeat_test.go:14,108,155`; `market_depth_test.go:35,143,198,244` | live heartbeat/poll cadence, provider connectivity, depth projection/level cap/num clamp, symbol casing and HK validation | P2 | partial：heartbeat policy 与 depth provider facade 聚合保持 partial；cadence, connectivity, depth projection and bounds 有真实 owner 证据。 |
| [x] | `internal/app/apiserver/marketdataapp/market_http_test.go:368,478,533,555,577,592,607,622` | snapshot refresh validation, security snapshot query and broker-neutral warrant/option/future/trust/index/plate envelope boundaries | P2 | partial：security detail block projections明确为 broker-neutral boundary，snapshot/query owner exact；未把 Go block facade 聚合升为 exact。 |
| [x] | `internal/app/apiserver/marketdataapp/provider_boundaries_test.go:145,163,197,220,271`; `provider_test.go:12` | provider closure/optional capabilities, search filtering/failure/normalization, candle parsing and dotted code prefixes, callback delegation | P2 | partial：provider closure/search/candle callback 聚合保持 partial；search prefix/normalization and strict candle converter 有真实 owner 证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 82/82 通过；未发现需要先红后修的生产差异。MarketData provider/heartbeat/depth/security facade 与 broker-neutral block 明确保持 partial/boundary。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(market_data_provider_read_routes_are_not_registered_without_snapshot_port) or test(active_failure_rolls_back_but_backtest_failure_never_persists) or test(failed_warmup_blocks_startup_restore_and_reports_last_error) or test(test_active_provider_mutation_atomic_single_state_source) or test(explicit_activation_fails_closed_and_switch_clears_cache) or test(provider_switch_succeeds_while_watchlist_ports_are_unavailable) or test(managed_streaming_demand_blocks_provider_switch_until_released) or test(reconfigure_preserves_demand_and_clears_previous_runtime_state) or test(failed_provider_change_restores_the_previous_subscription_owner) or test(partial_release_and_clear_operations) or test(market_data_runtime_projection_matches_go_status_corpus) or test(explicit_activation_requires_ready_health_but_startup_restore_allows_warming) or test(same_provider_activation_is_idempotent_after_a_rejected_switch) or test(readiness_without_composed_runtimes_reports_all_false) or test(ws_live_replays_complete_go_corpus) or test(live_connection_snapshot_tracks_concurrency_and_limit) or test(missing_cache_poll_respects_one_second_cadence) or test(non_positive_policy_values_retain_collector_defaults) or test(heartbeat_reports_live_provider_connectivity_without_a_fixture_projection) or test(depth_read_projects_name_times_and_levels_from_opend_s2c) or test(market_microstructure_depth_forwards_maximum_supported_level) or test(market_microstructure_depth_route_clamps_num_to_the_go_window) or test(normalize_instrument_handles_us_and_hk) or test(market_microstructure_depth_route_rejects_invalid_num_before_reader_call) or test(snapshot_route_rejects_invalid_refresh_before_provider_access) or test(securities_route_queries_the_security_snapshot_once) or test(futu_securities_route_projects_broker_neutral_envelope_boundary) or test(disconnected_provider_reports_its_reason_and_keeps_the_previous_selection) or test(instrument_search_route_validates_input_and_maps_provider_failures) or test(futu_search_filters_and_deduplicates_before_limiting_without_hiding_ambiguity) or test(normalize_instrument_rejects_missing_inputs) or test(futu_search_distinguishes_no_match_unsupported_market_and_runtime_failure) or test(candle_pagination_tests) or test(test_candle_converter_strict_rejection_boundaries) or test(canonical_search_code_does_not_double_prefix_the_market) or test(search_quote_market_prefixes_follow_the_go_aliases) or test(test_market_data_canonical_fixture_subscription_matches_production) or test(batch_snapshots_forward_each_instrument_through_the_snapshot_reader_once)'`

下一片：继续 P2 MarketData/Quote provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 187 批：MarketData runtime forwarding、Python helpers 与 research provider routes（2026-09-25）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/marketdataapp/provider_test.go:86,107`; `python_runtime_test.go:55,94`; `query_test.go:11,28,60` | search prefix inference, helper asset/process/health, period aliases and Futu K-line window bounds | P2 | partial：Python runtime facade/unknown prefix aggregation 保持 partial；helper health/process, alias and explicit window tests 有真实证据。 |
| [x] | `runtime_calendar_forwarding_test.go:74,119,131`; `runtime_company_forwarding_test.go:57,95,108`; `runtime_forwarding_test.go:12,98,166,191` | calendar/macro and company research forwarding/errors/capability, data-plane forwarding, provider error preservation, activation/close lifecycle | P2 | partial：runtime forwarding facade 聚合保持 partial；wire mapping, capability gates, provider identity and activation fences 有真实 owner 证据。 |
| [x] | `runtime_index_constituents_forwarding_test.go:35,61`; `runtime_news_forwarding_test.go:52,89`; `runtime_rankings_industry_forwarding_test.go:66,125`; `runtime_screen_forwarding_test.go:39,63,77` | index/news/actions/rankings/industry/screen forwarding and unsupported-provider errors | P2 | exact：各 provider capability、helper path、wire operation 与 rejection owner 测试逐项通过。 |
| [x] | `runtime_test.go:14,113,143,230` | provider transition serialization, invalid activation/rollback, rejected snapshot publication and deferred unsubscribe | P2 | partial：runtime facade 与 sidecar cleanup 聚合保持 partial；transition snapshot/rollback/deferred retry 有真实证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest 49/49 通过；未发现需要先红后修的生产差异。runtime/Python facade 聚合保持 partial，provider forwarding 与 capability gates 逐项记录。

验证：`node scripts/quality/cargo-nextest.mjs run --all-targets --locked`，筛选本批过滤器，49/49 通过。

下一片：继续 P2 MarketData/Quote provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。

## 第 188 批：MarketData runtime/sidecar、Watchlist source 与 runtime resources（2026-09-26）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/marketdataapp/runtime_test.go:276,337,370,414,437,504,667,704` | provider transition serialization/rollback, close fencing, subscription restoration, production provider catalog and backtest preparation | P2 | partial：runtime facade 与 backtest provider 聚合保持 partial；transition ordering、rollback、shutdown fencing 与 catalog/preparation owner 测试有真实证据。 |
| [x] | `internal/app/apiserver/marketdataapp/sidecar_os_process_test.go:78,161`; `sidecar_process_test.go:61,92,128` | natural child exit/finished stop, failed launch cleanup, materialization cleanup and bounded/idempotent stop | P2 | partial：sidecar manager restart/重试状态聚合保持 partial；OS process stop 与 launch cleanup owner 测试逐项通过。 |
| [~] | `internal/app/apiserver/marketdataapp/sidecar_process_test.go:147,208` | `from_process_env`/helper test-name references only; no standalone Rust parity test | P2 | boundary：开发覆盖与 Python source command 属于 helper/开发运行时边界，当前没有可解析的独立 Rust 测试，保持 `unverified`/partial。 |
| [x] | `internal/app/apiserver/marketdataapp/unavailable_provider_test.go:12`; `watchlist_source_test.go:15,74,100,121,175` | unavailable provider route failures, remote watchlist routing, explicit missing quote fields, Futu permission propagation and closed-session prior-close projection | P2 | partial：Go watchlist/source facade 与 provider error 聚合保持 partial；fail-closed、source routing、field availability、permission error 与 prior-close owner 证据已通过。 |
| [x] | `internal/app/apiserver/runtime/dependencies_test.go:13,39,94,115,156` | Node probe status matrix, configured-path precedence, missing Finder attempts, compatibility version parser and bounded command-error summary | P2 | exact：Node runtime dependency 状态、消息、版本解析和输出边界逐项有 Rust owner 测试。 |
| [x] | `internal/app/apiserver/runtime/research_runtime_test.go:9`; `resources_test.go:9,78`; `runtime_test.go:12` | research database derivation/override, runtime resource owners/count, development launch profile defaults | P2 | partial：resource inventory 与 research path/count 为 exact；launch profile 仍保留 product/desktop profile 聚合差异。 |

本批 30 条 Go 映射中，28 条具有可解析 Rust 测试过滤器并通过；Rust 去重后精准 nextest **30/30** 通过。两条开发 helper 命令条目只引用 `from_process_env`/`tests`，过滤器会扩大为名称子串且没有独立 Rust parity test，因此保留 `[~]`/`unverified`，未把过宽运行结果计入证据；未发现需要先红后修的生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(queued_transitions_observe_the_committed_provider_as_their_previous_owner) or test(failed_provider_change_restores_the_previous_subscription_owner) or test(provider_transitions_are_serialized_and_publish_one_snapshot) or test(activation_after_shutdown_is_rejected_and_leaves_the_snapshot_committed) or test(concurrent_close_and_activation_commit_exactly_one_outcome) or test(provider_change_cannot_restore_subscriptions_after_shutdown) or test(production_market_data_catalog_and_provider_ports) or test(active_failure_rolls_back_but_backtest_failure_never_persists) or test(a_naturally_exited_child_reports_stopped_and_can_be_closed_again) or test(already_finished_children_are_stopped_successfully_and_repeatable) or test(a_failed_launch_never_leaves_a_child_or_a_stale_endpoint) or test(stop_is_idempotent_and_bounds_graceful_shutdown_before_killing_the_child) or test(markets_route_fails_with_market_data_failed_when_active_provider_is_unavailable) or test(market_data_provider_read_routes_fail_closed_without_snapshot_port) or test(production_watchlist_read_uses_real_pages_and_remote_catalog) or test(test_quote_cache_and_import_helpers_keep_absent_data_explicit) or test(symbol_scoped_snapshot_errors_are_detectable_through_context) or test(remote_watchlist_read_route_fails_closed_when_unavailable) or test(closed_us_session_reports_regular_close_as_previous_close) or test(node_probe_reports_ok_outdated_unrecognized_and_failed_scripts) or test(configured_node_candidate_has_go_settings_precedence_and_wire_source) or test(configured_node_path_is_reported_missing_without_falling_back) or test(missing_node_message_names_finder_attempts_and_settings_guidance) or test(command_error_summary_keeps_output_tail_within_wire_budget) or test(node_version_parser_matches_go_compatibility_rules) or test(runtime_resources_declare_go_owners_and_derived_paths) or test(paths_follow_go_environment_overrides_and_adk_artifact_lifecycle) or test(runtime_resource_summary_count_matches_projected_items) or test(system_status_matches_go_stable_fields_without_claiming_migration_ownership) or test(build_profiles_preserve_tauri_identity_and_data_isolation)'`，30/30 通过。

下一片：继续 P2 runtime/desktop boundary、Storage/SQLite、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 及本批两条开发 helper 边界仍保持未验证结论。

## 第 189 批：runtime/desktop lifecycle、maintenance 与 broker query（2026-09-26）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/runtime/runtime_test.go:28,44,82,116,155,170`; `server_test.go:85,150,224,250,280,310,331,359` | desktop profile identity/origins, runtime env/path derivation, daily log projection, loopback validation, lifecycle order, listener conflict and persisted settings stability | P2 | partial：profile/Wails helper 与 desktop facade 聚合保持 partial；环境路径、日志、origin allowlist、loopback、listener conflict 与 startup ordering 有 owner 测试。 |
| [x] | `internal/app/apiserver/runtimes/handle_lifecycle_test.go:161,250,331,370,475,573,592,622` | production registry bindings, calendar restore, explicit ordered shutdown, late publication rejection and Pine worker cardinality | P2 | partial/boundary：Rust 类型化所有权和组合根关闭语义覆盖 registry/shutdown；Go 的并发 publication、late resource injection 与 resolver 全局替换保持结构边界。 |
| [x] | `internal/app/apiserver/server_test.go:472` | pending rebuild selection and tamper rejection without source deletion | P2 | partial：启动重建校验快照、marker 清除与篡改 fail-closed 有证据；Go 中间态删除/迁移阶段的逐步契约仍保持 partial。 |
| [x] | `internal/app/apiserver/servercore/adk_data_management_test.go:15,60`; `data_management_failure_boundaries_test.go:19,61,93` | ADK candidate overview/compaction, exact candidate-set execution, busy owner fencing and ready-database cleanup boundaries | P2 | partial：ADK/server facade、nil store 与业务 busy reason 的结构差异保留 partial；candidate set、reclaimed bytes、busy no-mutation 与 ready-table guards 有 owner 证据。 |
| [x] | `internal/app/apiserver/servercore/assistant_transport_lifecycle_test.go:11`; `broker_read_query_default_test.go:11` | websocket/assistant transport shutdown and broker read query market/account normalization | P2 | partial：Assistant HTTP transport close 聚合保持 partial；broker query normalization owner 测试已通过，但 Go Server facade 与 Rust route/port ownership 不完全同形。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest **25/25** 通过；其中 `ProductShutdownSupervisor` 是 Rust owner 类型而非独立测试名，未被计入测试数量。runtime/desktop profile、Handle publication、重建中间态和 ADK/server facade 的结构差异继续保留 `partial`/`boundary`，未发现需要先红后修的生产差异。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(build_profiles_preserve_tauri_identity_and_data_isolation) or test(desktop_trusted_origins_cover_development_and_packaged_wails_hosts) or test(paths_follow_go_environment_overrides_and_adk_artifact_lifecycle) or test(runtime_resources_keep_the_settings_directory_for_relative_paths) or test(native_runtime_events_append_to_the_existing_daily_log_contract) or test(log_reader_matches_go_filter_paging_and_day_order) or test(product_config_rejects_public_bind_and_missing_path) or test(production_registry_is_built_from_non_optional_adapters) or test(production_registry_rejects_a_missing_binding_instead_of_registering_it) or test(production_calendar_manager_restores_settings_and_snapshots) or test(test_product_runtime_ordered_shutdown_explicit) or test(ProductShutdownSupervisor) or test(production_runtime_rejects_multiple_pine_workers_without_failover) or test(system_status_matches_go_stable_fields_without_claiming_migration_ownership) or test(lifecycle_starts_in_dependency_order_and_shuts_down_in_reverse) or test(port_conflict_keeps_the_previous_listener_running) or test(startup_applies_a_pending_rebuild_only_to_the_selected_databases) or test(startup_rejects_a_tampered_pending_rebuild_without_deleting_any_source) or test(overview_lists_cleanable_categories_for_soft_deleted_and_history_rows) or test(cleanup_and_compaction_report_the_reclaimed_bytes_measured_on_disk) or test(soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute) or test(cleanup_requires_the_exact_approved_candidate_set) or test(websocket_live_port_stays_enabled_until_shutdown) or test(broker_read_query_normalization_and_account_priority_match_go) or test(busy_owner_blocks_preview_and_execute_without_mutation) or test(cleanup_preview_requires_a_ready_database_with_the_purgeable_table)'`，25/25 通过。

下一片：继续 P2 Storage/SQLite maintenance、Backtest/Calendar、MarketData/Quote 与 Strategy/Pine 映射；前两条开发 helper 边界和 P1 `session_context_snapshot` 仍保持未验证结论。

## 第 190 批：Storage maintenance、live/WS market data 与 notifications（2026-09-26）

| 复核 | Go 测试范围 | Rust 证据 | P | 结论 |
| :---: | --- | --- | :---: | --- |
| [x] | `internal/app/apiserver/servercore/data_management_failure_boundaries_test.go:141,178,191`; `data_management_test.go:17,59,124,191` | maintenance purge/compaction path errors, rebuild marker overview, backtest history retention, candidate-set/busy guards and error translation | P2 | partial/boundary：nil store、marker path lookup 与 Go error matrix 的结构差异保留 partial；directory rejection、retention, candidate fencing、measured bytes 与 ready-table guards 有真实 owner 测试。 |
| [x] | `internal/app/apiserver/servercore/desktop_token_test.go:14,46,127`; `exec_writeback_test.go:11,127` | desktop token/auth fail-closed, independent browser listener, execution writeback/replay and reconciliation deduplication | P2 | partial：token middleware 与 Server facade 聚合保持 partial；认证 fail-closed、listener separation、writeback recovery/restart 和 broker-discovered order dedupe 有 owner 证据。 |
| [x] | `internal/app/apiserver/servercore/instrument_ref_test.go:8`; `live_adapter_volume_test.go:11,29,49` | instrument normalization matrix and explicit/decimal/invalid volume-delta trade projection | P2 | partial：Go table facade 与 Rust market-data action/OpenD listener 分层不同；qualified/market aliases、decimal precision、zero/negative delta guards 均有 owner 测试。 |
| [x] | `internal/app/apiserver/servercore/live_heartbeat_boundaries_test.go:27`; `live_runtime_test.go:13`; `live_ws_boundaries_test.go:36,114` | active instrument/status projection, configured WS diagnostics, broker-neutral polling lease and WS backend/nil boundaries | P2 | partial/boundary：heartbeat/WS backend facade 与 typed port ownership 不完全同形；共享 status metrics、limit/origin guard、lease non-consumption、nil/backend fail-closed 有真实证据。 |
| [x] | `internal/app/apiserver/servercore/market_depth_test.go:33,117`; `market_details_ws_test.go:11` | initial depth push, no-price/error handling and broker-neutral security snapshot envelope | P2 | partial：Go WebSocket 首帧与 OpenD error 聚合保持 partial；order-book publication/drop、REST snapshot single-read 与 broker-neutral envelope owner 测试通过，未把 REST 证据升级为 WS exact。 |
| [x] | `internal/app/apiserver/servercore/market_instrument_resolver_test.go:13,65`; `market_realtime_test.go:18` | search-only instrument resolver, qualified leaf routing and realtime candle bucket after history pages | P2 | partial：resolver facade 不持有 Go demand/lease 句柄，保持 partial；search validation/leaf normalization 与 candle pagination/current bucket 有 owner 证据。 |
| [x] | `internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:18,140`; `notification_sources_test.go:13`; `notifications_lifecycle_test.go:58` | notification text/optional security fields, explicit real-trade-control path, calendar alert setting and live event wire mapping | P2 | partial：notification source/format facade 聚合保持 partial；wire event mapping、path precedence、calendar manager settings 与 bounded observability owner 测试有证据。 |

本批 30 条 Go 映射对应 Rust 去重后精准 nextest **39/39** 通过；未发现需要先红后修的生产差异。Storage maintenance、live/WS、security details、resolver 与 notification 条目的 facade/协议结构差异均按 partial/boundary 保留，未凭聚合测试升级为 exact。

验证：
`node scripts/quality/cargo-nextest.mjs run --all-targets --locked -E 'test(overview_counts_main_wal_and_shm_and_keeps_an_unreadable_database_local) or test(compaction_rejects_a_directory_where_the_backtest_database_belongs) or test(overview_normalizes_rebuild_marker_ids_and_fails_closed_when_it_is_unreadable) or test(backtest_history_cleanup_skips_running_runs_and_keeps_the_newest_terminal_run) or test(backtest_history_preview_prefers_age_and_latest_protection) or test(cleanup_and_compaction_report_the_reclaimed_bytes_measured_on_disk) or test(cleanup_preview_requires_a_ready_database_with_the_purgeable_table) or test(busy_owner_blocks_preview_and_execute_without_mutation) or test(cleanup_preview_route_returns_candidates_and_rejects_bad_payloads) or test(system_control_reads_are_authenticated_and_do_not_create_control_state) or test(enabled_web_access_binds_and_shutdown_releases_port) or test(auth_session_route_is_not_registered_without_snapshot_port) or test(execution_write_product_replays_browser_boundary_failure_recovery_and_restart) or test(test_notification_projector_advances_cursors_and_is_idempotent) or test(reconciliation_discovery_deduplicates_repeated_pages_and_keeps_newest_snapshot) or test(normalize_instrument_error_cases_match_go_fixture_semantics) or test(normalize_instrument_handles_us_and_hk) or test(normalize_instrument_handles_cn_market_and_prefixes) or test(basic_quote_pushes_publish_delta_and_cumulative_volume) or test(one_live_subscription_publishes_both_trade_and_depth_pushes) or test(basic_quote_pushes_keep_exact_volume_beyond_fixedpoint_range) or test(basic_quote_pushes_without_a_volume_counter_report_zero_delta) or test(negative_cumulative_volume_publishes_no_trade_event) or test(system_status_live_projection_uses_shared_transport_metrics) or test(ws_live_transport_rejects_origin_and_limit_without_leaking_permits) or test(broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease) or test(ws_live_replays_complete_go_corpus) or test(order_book_pushes_publish_depth_for_the_subscribed_instrument) or test(order_book_pushes_without_any_price_are_dropped) or test(securities_route_queries_the_security_snapshot_once) or test(futu_securities_route_projects_broker_neutral_envelope_boundary) or test(instrument_search_route_returns_subset_resolution_contract) or test(instrument_search_route_validates_input_and_maps_provider_failures) or test(normalize_instrument_resolves_qualified_sh_and_sz_prefixes) or test(candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar) or test(request_observability_matches_go_shape_and_bounded_order) or test(path_is_sibling_of_settings_file) or test(production_calendar_manager_restores_settings_and_snapshots)'`，39/39 通过。

下一片：继续 P2 API/transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar 映射；开发 helper 边界和 P1 `session_context_snapshot` 仍保持未验证结论。
