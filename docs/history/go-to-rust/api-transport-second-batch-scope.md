# API/Transport 第二批高风险边界清单

本批继续沿用 `Go 文件路径:行号:测试名` 复合键，核对 API/Transport 剩余的
SSE、CORS、WebSocket 和 candles 边界。新增 Rust 测试只验证其实际断言，
不同测试装配、业务范围和 wire 层级明确保留为 `[~]`/`partial`。

| 状态 | Go 测试 | Rust 证据 | 差异结论 | 精确验证 |
| --- | --- | --- | --- | --- |
| [~] | `internal/api/httpserver/sse_test.go:87:TestPrepareSSEWriterAndFrameFormatting` | `jftrade-api::sse::tests::frames_preserve_go_retry_id_data_and_comment_shape` | Rust 覆盖 retry、retry=0、id/data、comment 编码；缺 Go writer 的响应头与完整写入 body。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -E 'test(frames_preserve_go_retry_id_data_and_comment_shape)'` |
| [~] | `internal/api/middleware/auth_test.go:173:TestCORSReflectsAllowedOriginsAndRejectsUnknownPreflight` | `jftrade-api::transport_contracts::cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin` | Rust 覆盖允许 Origin 的 204/header 与恶意 Origin 的 403；Go 另有 expose、Referer/null 和完整 header 集合断言。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -E 'test(cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin)'` |
| [~] | `internal/api/middleware/security_boundaries_test.go:90:TestRequestOriginUsesRefererAndHandlesNil` | `jftrade-api::auth::tests::request_origin_preserves_origin_precedence_and_referer_fallback_boundary` | Rust 覆盖 Origin 优先、Referer fallback、malformed Origin 不回退与 provided 标志；Go 使用 nil/HTTP request，装配不同。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -E 'test(request_origin_preserves_origin_precedence_and_referer_fallback_boundary)'` |
| [~] | `internal/api/live/handler_test.go:397:TestHandlerRejectsUntrustedWebSocketOrigin` | `jftrade-engine::product::tests::ws_live_tests::ws_live_transport_rejects_untrusted_origin_before_upgrade` | Rust 断言 upgrade 前 403、text/plain 和 body；Go 覆盖两个恶意 origin 及 handler 关闭清理，Rust 仅覆盖单值。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(ws_live_transport_rejects_untrusted_origin_before_upgrade)'` |
| [~] | `internal/api/marketdata/routes_test.go:407:TestCandlesRouteTickAndStrictBeforePagination` | `jftrade-engine::product::tests::market_data_quote_read_tests::market_data_quote_read_routes_match_group_fixture_in_cutover_only` | Rust frozen fixture 回放 candles 成功、clamp、before 分页和错误状态；Go 还断言 provider 收到 tick/strict-before 参数，owner 装配不同。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(market_data_quote_read_routes_match_group_fixture_in_cutover_only)'` |
| [~] | `internal/api/marketdata/routes_test.go:439:TestDepthRouteRejectsInvalidNum` | `jftrade-engine::product::tests::market_data_quote_read_tests::market_microstructure_depth_route_rejects_invalid_num_before_reader_call` | Rust 断言 `num=abc` 返回 BAD_REQUEST 且 reader 调用数为零；Go 通过完整 Gin route，Rust 为 port 级验证。 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(market_microstructure_depth_route_rejects_invalid_num_before_reader_call)'` |

## 尚未达到完整等价的项目

SSE writer 的 flush/write panic、并发写串行化、客户端断线后的具体 writer 错误，
以及 WebSocket `handler.Close` 主动终止现有连接，当前仍无独立 Rust wire 测试，
继续保持 `missing`，不能用相近的 stream replay 或连接 permit 测试代替。

## 第三批（SSE writer 生产闭环 + CORS/WS Origin/candles 精确化）

本批把 Go `internal/api/httpserver/sse.go` 的 `SSEWriter` / `PrepareSSEWriter` 语义落成 Rust 生产路径：
`crates/jftrade-api/src/sse.rs` 新增 `SseSink`、`BufferedSseSink`、`SseWriter`、`SseError`，并由
`crates/jftrade-api/src/router.rs::sse_response` 实际使用（`ApiOutput::Sse` 不再是裸字符串拼接）。
panic 由 `catch_unwind` 转成 `sse write failed: ...`，与 Go 的 `defer recover()` 契约一致；
非 flush sink 在写出任何 retry/事件前被 `SseWriter::prepare` 拒绝。

| 状态 | Go 测试 | Rust 入口 | 结论 |
|---|---|---|---|
| [x] | `internal/api/httpserver/sse_test.go:41:TestSSEWriterReturnsFlushPanicAsError` | `sse::tests::write_event_returns_flush_panics_as_errors` | flush panic 转为含 "flush failed" 的错误。 |
| [x] | `internal/api/httpserver/sse_test.go:55:TestSSEWriterReturnsWritePanicAsError` | `sse::tests::write_retry_returns_write_panics_as_errors` | write panic 转为含 "write failed" 的错误。 |
| [x] | `internal/api/httpserver/sse_test.go:87:TestPrepareSSEWriterAndFrameFormatting` | `sse::tests::prepare_writes_retry_then_id_event_and_comment_frames` | prepare 后写入 retry 3000、id+data 事件与 comment，wire body 与 Go 相同。 |
| [x] | `internal/api/httpserver/sse_test.go:129:TestPrepareSSEWriterRejectsWriterWithoutFlusher` | `sse::tests::prepare_rejects_a_sink_without_flush_support` | 非 flush sink 返回 None。 |
| [x] | `internal/api/httpserver/sse_test.go:136:TestRunSSEStreamLoopPropagatesInitialError` | `sse::tests::stream_loop_propagates_initial_error_before_any_tick` | initial 错误先于任何 tick 上抛。 |
| [x] | `internal/api/httpserver/sse_test.go:146:TestRunSSEStreamLoopRunsTriggerAndTickerTicks` | `sse::tests::stream_loop_runs_trigger_and_ticker_ticks` | trigger 两次 tick 后取消；5ms interval 一次 tick 后取消。 |
| [x] | `internal/api/httpserver/sse_test.go:196:TestTickerCHandlesNilTicker` | `sse::tests::stream_loop_handles_nil_ticker_channel` | 无 trigger/interval 时 on_tick 永不触发；Interval 有值但无 OnTick 时不 arm ticker。 |
| [x] | `internal/api/httpserver/sse_boundaries_test.go:30:TestSSEWriterPropagatesSerializationAndWriteFailures` | `sse::tests::write_event_propagates_serialization_and_write_failures` | 序列化失败与 sink I/O 失败原样返回。 |
| [x] | `internal/api/httpserver/sse_boundaries_test.go:46:TestRunSSEStreamLoopHandlesTriggerWithoutCallback` | `sse::tests::stream_loop_ignores_trigger_without_callback` | 无回调时忽略 trigger。 |
| [x] | `internal/api/httpserver/sse_boundaries_test.go:60:TestRunSSEStreamLoopPropagatesTriggerAndTickerFailures` | `sse::tests::stream_loop_propagates_trigger_and_ticker_failures` | trigger/ticker 两条路径的 on_tick 错误上抛。 |
| [x] | `internal/api/httpserver/sse_boundaries_test.go:86:TestFailingSSEWriterIncludesReadableError` | `sse::tests::write_failures_are_reported_with_their_source_message` | retry 写入失败保留源错误文本。 |
| [x] | `internal/api/httpserver/sse_concurrent_test.go:42:TestSSEWriterSerializesConcurrentWrites` | `sse::tests::concurrent_writers_serialize_frames` | 8 个并发写串行化，8 个 frame 与终结符。 |
| [x] | `internal/api/middleware/auth_test.go:173:TestCORSReflectsAllowedOriginsAndRejectsUnknownPreflight` | `transport_contracts::cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin` | 补 expose-headers 与 Origin=null + Referer 仍 403。 |
| [x] | `internal/api/middleware/security_boundaries_test.go:90:TestRequestOriginUsesRefererAndHandlesNil` | `auth::tests::request_origin_preserves_origin_precedence_and_referer_fallback_boundary` | 空 headers 等价 nil request；Origin 优先、malformed 不回退、origin_provided 记录。 |
| [x] | `internal/api/live/handler_test.go:397:TestHandlerRejectsUntrustedWebSocketOrigin` | `product::tests::ws_live_tests::ws_live_transport_rejects_untrusted_origin_before_upgrade` | 两个不可信 Origin（含 null）均在升级前 403。 |
| [x] | `internal/api/marketdata/routes_test.go:407:TestCandlesRouteTickAndStrictBeforePagination` | `product::tests::market_data_quote_read_tests::candle_pagination_tests::candle_route_serves_tick_period_and_forwards_strict_before_window` | tick 成功且 hasMore=false；before 转成 provider 本地窗口 end。 |
| [x] | `internal/api/marketdata/routes_test.go:439:TestDepthRouteRejectsInvalidNum` | `product::tests::market_data_quote_read_tests::market_microstructure_depth_route_rejects_invalid_num_before_reader_call` | 非法 num 在 reader 调用前 400 且调用计数为 0。 |

仍保留 `[~] partial` 的条目：

- `internal/api/marketdata/routes_test.go:459:TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize`：Rust 的 provider/markets/security/snapshot/search/heartbeat/normalize 成功契约分散在多个专属测试文件，尚无以单条路由回放全部子断言的对应测试。
- `internal/api/marketdata/routes_boundaries_test.go:63:TestCandlesAndDepthRoutesMapProviderFailures`：Rust 已覆盖 ticks/depth port 的错误映射与 retry-after，但 candles route 的 provider 失败映射仍走 helper/fixture 路径，未在单条测试中同时覆盖 candles 与 depth。

补充说明：Go 的 `RunSSEStreamLoop` / `tickerC` 在 `go` 分支没有生产调用方（仅测试引用），Rust 侧仍按同语义提供 `run_sse_stream_loop`，以保证行为基线不被丢失。

## 第四批（market-data provider 失败码差异修复）

`internal/api/marketdata/routes_boundaries_test.go:63`（Go）断言 candles/depth 的 provider 失败码由 provider 决定：
显式/激活 Futu 用 `OPEND_*_FAILED`，其余 provider 用 `MARKET_*_FAILED`。

Rust 修复前：非 Futu provider 走 market-data helper 的 candles 路径硬编码
`OPEND_CANDLES_FAILED`，与 Go 的 `MARKET_CANDLES_FAILED` 不一致。

- 复现条件：`activeMarketDataProvider=yfinance`，helper candles 端点返回 502。
- 预期行为：HTTP 502 + `MARKET_CANDLES_FAILED`。
- 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`
  （helper 分支只可能由非 Futu provider 进入，故使用通用失败码）。
- 回归测试：`crates/jftrade-engine/tests/market_data_production_compatibility.rs::test_non_futu_helper_candles_failure_uses_generic_market_code`。

仍保留 `[~] partial`：depth 的通用 `MARKET_DEPTH_FAILED` 没有对应 Rust 实现
（非 Futu depth 在 Rust 返回不支持/Unavailable），且 Go 的 depth 失败码与
depth=25 参数转发未在单条 Rust 测试中同时覆盖；`routes_test.go:459` 的七段
复合成功契约分散在多个 Rust 专属测试中，尚无以单条测试按 Go 顺序完整回放。

## 第五批（auth middleware 全量对齐）

`internal/api/middleware/auth_test.go` 全 11 条已闭环（10 条 function_exact + 1 条接口形态
boundary）。新增 Rust 回归位于 `crates/jftrade-api/tests/transport_contracts.rs`：

| 状态 | Go 测试 | Rust 入口 |
|---|---|---|
| [x] | `auth_test.go:13:TestAuthSkipsPublicPaths` | `auth_skips_public_paths_without_credentials` |
| [x] | `auth_test.go:51:TestAuthProtectsLogout` | `auth_protects_logout` |
| [x] | `auth_test.go:71:TestAuthProtectsSystemStatus` | `auth_protects_system_status_without_credentials` |
| [x] | `auth_test.go:112:TestAuthRejectsNilAuthenticator` | `auth_rejects_requests_without_any_authenticator` |
| [x] | `auth_test.go:119:TestAuthRejectsUntrustedOrigin` | `auth_rejects_untrusted_origin_before_authentication` |
| [x] | `auth_test.go:128:TestAuthRequiresOriginAndCSRFForSessionWrites` | `auth_requires_origin_and_csrf_for_session_writes` |
| [x] | `auth_test.go:153:TestAuthTreatsPatchAsSessionWrite` | `auth_treats_patch_as_session_write_requiring_csrf` |
| [x] | `auth_test.go:173:TestCORSReflectsAllowedOriginsAndRejectsUnknownPreflight` | `cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin` |
| [x] | `auth_test.go:78:TestAuthTrustedHostWithoutBrowserOriginBypassesCSRFChecks` | `auth_trusted_caller_without_origin_bypasses_csrf` |
| [x] | `auth_test.go:86:TestAuthTrustedHostStillRequiresTrustedBrowserOrigin` | `auth_trusted_caller_still_rejects_untrusted_provided_origin` |
| [~] | `auth_test.go:28:TestTrustedAndValidatedRequestContextHelpers` | boundary：Rust 无可变 `*http.Request` 标记 API，等价信息由 `ApiRequest.desktop_trusted/origin_allowed` 与 `current_request_context` task-local 传递；`/health` 在 Go 生产已无 route，仅测试引用。 |

每条 `[x]` 使用唯一 Rust 入口，已通过 audit 的唯一性校验。

## 第六批（candles 路由查询解析与 market 大小写差异修复）

`internal/api/marketdata/routes_test.go` 的 candles 查询族（267/302/321/337/357/387）
全部闭环为 `[x] function_exact`，新增回归位于
`crates/jftrade-engine/src/product_market_data_candle_pagination_tests.rs`。

复现并修复的真实差异：Go 的 candles handler 接受小写 market 路径段（`us/aapl`）
并在 service 内部规范化；Rust 的 `read_candles` 直接把路径段交给大小写敏感的
`quote_market_code`，导致 400 `invalid market: us`。

- 复现条件：`GET /api/v1/market-data/candles/us/aapl?period=k_60m&limit=5`。
- 预期行为：200，provider 收到 `aapl` 与规范化的 `1h` 窗口。
- 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`
  的 `read_candles` 入口（在读取 owner 归一化 market，未改全局 `quote_market_code`
  以免影响交易路径）。
- 回归测试：`candle_route_preserves_legacy_query_parsing` 及同批 5 条 candles 测试。

## 第三批：market-data 读取错误映射与 provider generation fencing（P0/P1）

| 状态 | Go 测试 | Rust 证据 | 差异结论 |
| --- | --- | --- | --- |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:273:TestMarketDataReadErrorsExposeProviderSwitchRetrySignal` | `crates/jftrade-engine/src/product_market_data_quote_read_tests.rs::snapshot_read_fences_provider_generation_switch_during_helper_query` | function_exact：Go 的 `GetSnapshot` 读取前记录 `providerGeneration`、返回后复核并在切换时返回 `ErrProviderChanged`（409 `MARKET_DATA_PROVIDER_CHANGED`）。Rust 原先没有该栅栏，快照会带着已退役 provider 的数据返回。本轮在 `read_snapshots` 的 helper 分支与 Futu fallback 分支补上 generation 复核；回归测试让 helper 在响应前 `activate(Akshare)`，断言 409 + `MARKET_DATA_PROVIDER_CHANGED`。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:286:TestMarketDataReadErrorsExposeProviderWarmupRetrySignal` | `crates/jftrade-engine/src/product_production_ports_market_data_projection.rs::tests::market_data_read_errors_expose_provider_warmup_retry_signal` | function_exact：Go 的 `writeMarketDataReadError` 对 `ErrProviderWarming` 返回 503 + `Retry-After: 1` + `MARKET_DATA_PROVIDER_WARMING`。Rust 原先直接透传 helper 的 `*_RUNTIME_WARMING` 码，本轮在共享 helper 错误投影层按 Go 的 `classifyRuntimeError` 语义归类，覆盖 `YFINANCE_RUNTIME_WARMING`、`AKSHARE_RUNTIME_WARMING`、`PROVIDER_RUNTIME_WARMING`。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:300:TestMarketDataReadErrorsExposeProviderBusyRetrySignal` | `crates/jftrade-engine/src/product_production_ports_market_data_projection.rs::tests::market_data_read_errors_expose_provider_busy_retry_signal` | function_exact：Go 的 akshare 适配器把 `AKSHARE_POOL_BUSY`/`AKSHARE_UPSTREAM_TIMEOUT` 归为 `ErrProviderBusy`，路由返回 503 + `Retry-After: 2` + `MARKET_DATA_PROVIDER_BUSY`。Rust 原先直接透传这两个码，本轮归类到共享投影；同批的 `market_data_read_errors_preserve_unclassified_helper_failures` 保证其他 helper 错误不被改写。 |

复现与修复记录（P0，唯一写入所有权相邻的读一致性）：

- 复现条件：active provider 为 yfinance/akshare，`GET /api/v1/market-data/snapshots/US/AAPL` 进行中，设置写入触发 `ActiveProviderState::activate` 递增 generation。
- 预期行为：409 `MARKET_DATA_PROVIDER_CHANGED`，且该快照不得进入调用方结果。
- 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads.rs`（`read_snapshots` 的 helper 与 Futu fallback 两处），共享错误构造在 `product_production_ports_market_data_projection.rs::provider_changed_error`。
- 回归测试：`snapshot_read_fences_provider_generation_switch_during_helper_query`。

复现与修复记录（P1，Provider retry 语义）：

- 复现条件：helper 在预热期返回 503 `YFINANCE_RUNTIME_WARMING`/`AKSHARE_RUNTIME_WARMING`/`PROVIDER_RUNTIME_WARMING`；akshare 线程池饱和或上游超时返回 503 `AKSHARE_POOL_BUSY`/`AKSHARE_UPSTREAM_TIMEOUT`。
- 预期行为：503 + `Retry-After`（预热 1 秒、繁忙 2 秒）+ `MARKET_DATA_PROVIDER_WARMING`/`MARKET_DATA_PROVIDER_BUSY`，正文为 Go 的中文提示。
- 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_projection.rs::classify_helper_runtime_code`（quote read 路径统一复用）。
- 回归测试：`market_data_read_errors_expose_provider_warmup_retry_signal`、`market_data_read_errors_expose_provider_busy_retry_signal`、`market_data_read_errors_preserve_unclassified_helper_failures`。

## 第八批（订阅写入契约、broker-neutral 轮询与 live 读取租约）

本批闭环 `internal/api/marketdata/routes_test.go` / `routes_boundaries_test.go` 的订阅写入与
live 读取条目，新增回归集中在
`crates/jftrade-engine/src/product_production_ports_market_data_subscription_tests.rs`、
`crates/jftrade-engine/src/product_market_data_quote_read_tests.rs` 与
`crates/jftrade-engine/src/product_production_ports_market_data_catalog_tests.rs`。

| 状态 | Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- | --- |
| [x] | `internal/api/marketdata/routes_test.go:22:TestSubscriptionRoutesUseInstrumentRequestContract` | `product_production_ports_market_data_subscription_tests.rs::subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear` | function_exact：KLINE:HK:00700:1m 的 channel/interval 请求契约保留，单目标 release、consumer-scoped clear、clear-all 每步的 entries/refCount/totalActiveSubscriptions 与 Go 一致。 |
| [x] | `internal/api/marketdata/routes_test.go:96:TestSubscriptionRoutesUseBrokerNeutralPollingWithoutFutuLease` | `product_production_ports_market_data_subscription_tests.rs::broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease` | function_exact：`providerBrokerId=" Alpha "` 归一化为 alpha，acquire/heartbeat/release 的 action、totalActiveSubscriptions=0、quota、transport.mode=snapshot-poll-fallback 全对齐，且 DemandBook 逻辑条目始终为 0。 |
| [x] | `internal/api/marketdata/routes_test.go:205:TestSubscriptionReleaseConsumerOnlyClearsConsumer` | `product_production_ports_market_data_subscription_tests.rs::consumer_only_release_keeps_other_consumer_entry` | function_exact：共享 SNAPSHOT:HK:00700 的 consumer-only release 后仅剩 other，独立条目不受影响。 |
| [x] | `internal/api/marketdata/routes_test.go:239:TestClearSubscriptionRoutePreservesRunningStrategyLease` | `product_production_ports_market_data_subscription_tests.rs::clear_route_preserves_running_strategy_lease` | function_exact：Web clear 不删除 strategy-runtime:one 的托管租约，剩余条目 key/consumers 与 Go 相同。 |
| [~] | `internal/api/marketdata/routes_test.go:563:TestReadRoutesMapProviderAndRequestFailures` | `product_market_data_quote_read_tests.rs::read_routes_map_provider_and_request_failures` | partial：heartbeat/normalize 400 与 provider 502、markets 500、security 502 均已由本回归或同批 fixture/回归冻结；唯一差异是 Go 的 Futu snapshot provider 失败走 502，而 Rust 在“无缓存且无 trade runtime 回退”时返回 503 `MARKET_DATA_QUOTE_READ_UNAVAILABLE`（helper snapshot 失败才是 502 `MARKET_SNAPSHOT_FAILED`）。属 P2 错误分类差异，已记录边界。 |
| [x] | `internal/api/marketdata/routes_test.go:625:TestInstrumentSearchRouteReturnsSubsetResolutionContract` | `product_production_ports_market_data_catalog_tests.rs::instrument_search_route_returns_subset_resolution_contract` | function_exact：候选窗口固定 100、CN 子集过滤、稳定 provider 顺序、securityType 透传、JP 不可选带原因、`SH.600519` 走叶子 lookup 得 resolved/1 条。 |
| [x] | `internal/api/marketdata/routes_test.go:725:TestInstrumentSearchRouteValidatesInputAndMapsProviderFailures` | `product_production_ports_market_data_catalog_tests.rs::instrument_search_route_validates_input_and_maps_provider_failures` | function_exact：missing→not_found、Toyota(JP)→unavailable+原因、五类非法请求 400 `MARKET_INSTRUMENT_INVALID`、Futu runtime 失败 fail closed、非 Futu helper 502 `MARKET_INSTRUMENT_SEARCH_FAILED`。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:387:TestLiveReadRoutesReturnConflictForMissingSubscriptionLease` | `product_market_data_quote_read_tests.rs::live_read_routes_require_a_logical_subscription_lease` | function_exact：snapshots/candles/depth 缺逻辑租约统一 409 `MARKET_DATA_SUBSCRIPTION_REQUIRED`，acquire SNAPSHOT 后恢复 200。本轮在读取 owner 新增 `require_basic_subscription_lease`。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:421:TestPollOnlyReadRoutesPrioritizeCapabilitiesAndPreserveLogicalLeases` | `product_market_data_quote_read_tests.rs::poll_only_read_routes_prioritize_capabilities_and_preserve_leases` | function_exact：poll-only provider 的 tick candles 与 depth 先判 capability，返回 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED` 且不消费逻辑租约。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:502:TestSubscriptionRequestHelpersPreserveOnlyValidTargets` | `product_production_ports_market_data_subscription_tests.rs::subscription_request_helpers_preserve_only_valid_targets` | function_exact：混合 acquire 列表只保留唯一合法目标，全非法 acquire 400，缺一半身份的 release target 400 且已获取 demand 不变。 |
| [x] | `internal/api/marketdata/routes_boundaries_test.go:555:TestReleaseAndClearMapSnapshotCancellationAfterLogicalCleanup` | `product_production_ports_market_data_subscription_tests.rs::release_and_clear_map_snapshot_failure_after_logical_cleanup` | function_exact：物理快照读取在逻辑清理后失败时，release 与 clear 均返回 500 `SUBSCRIPTION_FAILED`，且逻辑 demand 已先归零。 |

### 本批功能修复记录

1. **P1 live 读取逻辑租约（`routes_boundaries_test.go:387`）**
   - 复现条件：active provider=Futu 且 router 已装配，未 acquire SNAPSHOT/KLINE/TICK 时读取
     `GET /api/v1/market-data/snapshots/US/AAPL`、`candles?period=1m`、`depth?num=10`。
   - 预期行为：三条路由均 409 `MARKET_DATA_SUBSCRIPTION_REQUIRED`，acquire 后 snapshot 恢复 200。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote.rs` 新增
     `require_basic_subscription_lease` 与 `subscription_required_error`；
     `product_production_ports_market_data_quote_reads.rs` 在 snapshot 与 candles 入口调用它
     （depth 走既有 `require_order_book_subscription`）。
   - 回归测试：`live_read_routes_require_a_logical_subscription_lease`。

2. **P1 poll-only capability 优先级（`routes_boundaries_test.go:421`）**
   - 复现条件：active provider=yfinance，请求 `candles?period=tick` 与 `depth?num=10`。
   - 预期行为：409 `MARKET_DATA_CAPABILITY_UNSUPPORTED`（含 provider id 与 capability 文案），
     不再泄漏 502/503。
   - 修复位置：`crates/jftrade-engine/src/product_production_ports_market_data_quote.rs::capability_unsupported_error`
     与 `read_depth` 的非 Futu 分支；`product_production_ports_market_data_quote_reads.rs::read_candles`
     的 poll-only tick 分支。
   - 回归测试：`poll_only_read_routes_prioritize_capabilities_and_preserve_leases`；
     `production_market_data_catalog_and_provider_ports` 同步更新未配置 depth 的期望。

3. **P2 Futu snapshot 失败分类（`routes_test.go:563`，仍为 partial）**
   - 复现条件：active provider=Futu、无 router 缓存且无 trade runtime 回退时读取 snapshot。
   - 预期（Go）：502 默认映射；Rust 现状：503 `MARKET_DATA_QUOTE_READ_UNAVAILABLE`。
   - 结论：属于“缓存不可用”语义与“provider 调用失败”语义的分类边界，保留 `[~] partial`
     并记录为后续 P2 决策项，不强行改成 502。
