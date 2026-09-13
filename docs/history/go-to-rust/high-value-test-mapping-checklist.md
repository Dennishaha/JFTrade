# Go → Rust 高价值测试映射清单

> 本清单是行为映射台账，不把测试数量当作迁移完成度。来源固定为 `go:452dea11`；Rust 路径指当前 production port、store 或 route 的真实测试入口。`[x]` 仅表示已有可复核 Rust 断言，`[~]` 表示已有部分覆盖，`[ ]` 表示仍需补测或修复。

## 使用规则

- 每项必须同时记录 Go 来源、风险、Rust 入口、结论、动作和验证命令。
- `live OpenD`、真实 worker、真实模型 provider 不计入本地通过；对应项保持“未验证”。
- 先补回归测试，再修最小生产实现；修复后将 `[~]`/`[ ]` 更新为 `[x]` 或注明“不适用”。

## P0：API / Transport / Assistant

| 状态 | Go 来源与行为 | 风险 | Rust 对应入口/现状 | 结论与动作 | 验证 |
|---|---|---|---|---|---|
| [x] | `internal/api/assistant/adk_normalize_test.go:15` 空 slice 序列化为数组 | wire shape | `product_adk_read_tests.rs` fixture replay + explicit array assertions | 已确认 runs/sessions 空集合保持 JSON array | `nextest -p jftrade-engine` |
| [x] | `internal/api/assistant/adk_routes_test.go:214` audit 非法分页 | pagination/error mapping | `product_adk_read_api.rs` + `product_adk_read_tests.rs` | 已拒绝 `limit<=0` 与 `offset<0`，并保持端口不可用前置校验 | `nextest -p jftrade-engine` |
| [x] | `internal/api/assistant/adk_ops_test.go:247` task 查询与取消 | cancel/lifecycle | `product_adk_mutation_product_tests.rs` SQLite cutover | operation identity、取消响应与重开数据库后的 `cancelled` 状态持久化均已断言 | `nextest -p jftrade-engine` |
| [x] | `internal/api/assistant/adk_routes_test.go:245` chat stream session/run/final 事件 | stream ordering | `crates/jftrade-engine/tests/adk_chat_stream_compatibility.rs` + product stream tests | Go wire fixture replay、session/run/final 顺序、client disconnect、retained terminal replay 已覆盖 | `nextest -p jftrade-engine` |
| [x] | `internal/api/httpserver/sse_*` 断连、并发、边界 | disconnect/backpressure | `crates/jftrade-engine/tests/ws_live_compatibility.rs`、`adk_chat_stream_timing_challenge.rs` | 已有隔离覆盖；继续核对 production composition | `nextest -p jftrade-engine` |
| [x] | `internal/assistant/assembly/mcp_server_test.go:76,106` loopback 启停与 authenticated stream | lifecycle/auth | `crates/jftrade-engine/src/product_mcp_server_tests.rs` | 现有覆盖；保留 live 未验证边界 | `nextest -p jftrade-engine` |
| [x] | `internal/assistant/assembly/runtime_test.go:14` open 幂等生命周期 | idempotency | `crates/jftrade-assistant/tests/assistant_claims_runtime_contracts.rs` | lease/fence 已有回归 | `nextest -p jftrade-assistant` |
| [x] | `internal/assistant/assembly/*cascade*` session cascade/fence | stale writer/recovery | `crates/jftrade-engine/tests/adk_session_cascade_*` | 已有 adversarial 与 deletion resilience | `nextest -p jftrade-engine` |
| [x] | `internal/api/*routes_payload_pagination_test.go` payload/page 边界 | pagination/shape | `crates/jftrade-engine/tests/*compatibility.rs`、`crates/jftrade-api/tests` | production route fixtures assert page metadata, malformed payload precedence, empty collections and error envelopes; operation-specific gaps remain tracked separately rather than hidden by this aggregate row | `nextest -p jftrade-api -p jftrade-engine` |

## P1：Futu / 行情 / 缓存

| 状态 | Go 来源与行为 | 风险 | Rust 对应入口/现状 | 结论与动作 | 验证 |
|---|---|---|---|---|---|
| [x] | `internal/integration/futu/candle_sessions_test.go:11` session windows | session/timezone | `crates/jftrade-integration-futu/src/session_resolver.rs` | 已补 resolver 边界 | `nextest -p jftrade-integration-futu` |
| [x] | `marketdata_runtime_test.go:267,364` tick 无效价格、quote fallback、缓存继承 | fallback/cache | `basic_quote_tick.rs`、`basic_quote_query.rs`、`session_coordinator.rs`、fake OpenD runtime tests | non-finite/invalid price fallback、quote field inheritance、generation-fenced cache、fallback establishment/recovery and retry/reset lifecycle all have Rust assertions | `nextest -p jftrade-integration-futu --lib` |
| [x] | `notifications_test.go:13,98` neutral notification/status transition | protocol mapping | `tests/futu_notifications_parity.rs` | 已新增 parity 测试 | `nextest -p jftrade-integration-futu` |
| [x] | `internal/marketdata/broker_candles_test.go:12-166` strict/terminal/bounded/bad pagination | pagination | `product_market_data_candle_pagination_tests.rs` | 已补 Rust 分页断言 | `nextest -p jftrade-engine` |
| [x] | `internal/marketdata/cache_test.go:12-185` dedup/promote/freshness/extended session | cache/session | `jftrade-marketdata/tests/cache_boundaries.rs`, `cache_extended_sessions_parity.rs` | 已补跨日和 regular close 语义 | `nextest -p jftrade-marketdata` |
| [x] | `cache_test.go:210` tick candle volume window/limit | aggregation | `market_data_production_compatibility.rs` candle conversion tests | Rust production candle contract already asserts volume/session window, requested limit and extended-session null projection; native tick-candle API is not exposed by Rust | `nextest -p jftrade-engine` |

## P1：Trading / Broker / Reconciliation

| 状态 | Go 来源与行为 | 风险 | Rust 对应入口/现状 | 结论与动作 | 验证 |
|---|---|---|---|---|---|
| [x] | `broker_boundaries_test.go:11` market data unavailable fallback | fallback | `crates/jftrade-trading/tests/order_risk_compatibility.rs` | 已有风险兼容断言 | `nextest -p jftrade-trading` |
| [x] | `broker_conformance_test.go:58` cancel accepted/rejected | broker contract | `crates/jftrade-trading` broker tests | 已覆盖，检查错误分类 | `nextest -p jftrade-trading` |
| [x] | `control_plane_idempotency_test.go:65,99` hard-stop 单次释放/全量阻断 | idempotency/fence | `hard_stop_environment_scope.rs` | 已新增 scope 回归 | `nextest -p jftrade-trading` |
| [x] | `execution_combo_lifecycle_test.go:15,635` preview/place/cancel/cache failures | lifecycle/recovery | engine execution tests | 现有 production route 覆盖，需保持失败分支 | `nextest -p jftrade-engine` |
| [x] | reconciliation late result/order status | late result/idempotency | `order_status_reconciliation_parity.rs` | 已新增 parity 测试 | `nextest -p jftrade-trading` |

## P1：Strategy / Pine / Backtest / Storage / Settings

| 状态 | Go 来源与行为 | 风险 | Rust 对应入口/现状 | 结论与动作 | 验证 |
|---|---|---|---|---|---|
| [x] | `asset_selection_boundaries_test.go:30` missing/empty worker bundle | unavailable | `crates/jftrade-integration-pine/src/asset.rs` | 已补资源选择边界 | `nextest -p jftrade-integration-pine` |
| [~] | `runtime_reconciliation_business_test.go:12-113` catalog state/activity/paging | reconciliation/paging | `strategy_runtime_activity.rs`; `strategy_runtime_owner_tests.rs::production_loop_reopens_at_checkpoint_and_replays_every_unprocessed_bar` | activity 分页/过滤与运行时 checkpoint/replay 已有证据；`ProductionStrategyRuntimePort::restore_running_instances` 的 stale RUNNING/PAUSED 启动收敛仍缺真实 production composition 断言 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_runtime)' --locked` |
| [x] | `runtime_reconciliation_business_test.go:12` 状态转换与活动记录 | lifecycle/audit | `strategy_runtime_owner_tests.rs::production_loop_reopens_at_checkpoint_and_replays_every_unprocessed_bar` | Rust worker 在 checkpoint 后重启式恢复未处理 candle，并写入可观察审计事件；已通过 101 个 strategy_runtime 测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -E 'test(strategy_runtime)' --locked` |
| [x] | `historical_source_test.go:111-341` retry/cancel/empty/broken page | retry/cancel | `product_production_ports_backtest_sync_helpers.rs` | 已补同步 helper 边界 | `nextest -p jftrade-engine` |
| [x] | `recovery_test.go:11-45` nil/panic/blank script | recovery/validation | `product_research_backtest_execution.rs` | 已补错误恢复语义 | `nextest -p jftrade-engine` |
| [x] | `store_failure_test.go:121` canceled maintenance no mutation | rollback | `backtest_run_store_contracts.rs` | 已补 store contract | `nextest -p jftrade-store-sqlite` |
| [x] | `normalization_and_persistence_test.go`、`rollback_test.go` | rollback/malformed input | `settings_file_contracts.rs` | 已补 normalization/rollback | `nextest -p jftrade-store-settings-file` |
| [x] | `store_boundaries_test.go`、snapshot failures | idempotency/schema | `jftrade-calendar`、SQLite audit tests | 已有 calendar/schema 断言 | `nextest -p jftrade-calendar -p jftrade-store-sqlite` |
| [~] | `internal/watchlist/futu/source_test.go:44-418` duplicate/cache/fallback/extended metadata | cache/fallback | `crates/jftrade-watchlist` | generic watchlist identity/cache boundaries are covered; Futu-specific remote group reader and quote projection are not represented in this crate, so duplicate-name and extended-session cases remain an explicit architecture gap | `nextest -p jftrade-watchlist` |

## 尚未映射的高风险集合

审计脚本当前识别 Go 高风险测试 952 个；上表是第一批可复核样本。后续按同一字段扩展剩余条目，优先顺序为：

1. API/Transport 其余 route、middleware、SSE/WS 条目；
2. ADK workflow/approval/session 失败分支；
3. Strategy catalog 与 Pine runtime activity；
4. Futu fallback/stream lifecycle；
5. storage/settings rollback 与 watchlist source；
6. 低风险 tooling/desktop 测试。

## 证据与门禁

- 审计统计：`python3 scripts/compatibility/audit_test_parity.py`（该脚本会刷新统计报告）。
- Rust 局部验证使用仓库 nextest wrapper，不用裸 `cargo test` 替代。
- Rust 变更完成后：`pnpm run check:rust`、`pnpm run check:quick`。
- 最终交付前复查 `git diff --check`、公开契约、schema、worker wire contract 和 live 未验证项。
