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

