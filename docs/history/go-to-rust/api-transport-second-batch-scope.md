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
