# Go → Rust 证据积压清单

本清单只统计缺少函数级 Rust 证据的 `[~]` 项；不代表功能缺失，也不代表已覆盖。每项需要人工对照 Go 断言并补充真实 Rust 测试函数、命令或边界结论。

当前积压：**4401 项**。

## 按领域

| 领域 | 条目数 |
|---|---:|
| api_transport | 951 |
| assistant_workflow | 810 |
| other | 674 |
| strategy_pine | 541 |
| futu_opend | 517 |
| backtest_calendar | 300 |
| storage_sqlite | 221 |
| marketdata_quotes | 206 |
| trading_broker | 120 |
| settings_watchlist | 61 |

## 按 Go 文件（Top 50）

| Go 文件 | 条目数 |
|---|---:|
| `pkg/strategy/pine/parse_test.go` | 35 |
| `pkg/futu/exchange_test.go` | 27 |
| `internal/trading/execution_test.go` | 27 |
| `internal/assistant/engine/store_ops_test.go` | 26 |
| `internal/assistant/engine/runner_chat_test.go` | 24 |
| `pkg/futu/opend/new_methods_test.go` | 24 |
| `cmd/jftrade-desktop/main_test.go` | 24 |
| `internal/strategy/pine_live_executor_test.go` | 24 |
| `pkg/backtest/pineworker_command_executor_test.go` | 23 |
| `pkg/bbgo/types/indicator_test.go` | 23 |
| `internal/app/apiserver/marketdataapp/runtime_test.go` | 22 |
| `pkg/futu/adapter_new_methods_test.go` | 22 |
| `pkg/strategy/indicatorbinding/parse_test.go` | 22 |
| `internal/assistant/engine/tools_test.go` | 21 |
| `internal/exchangecalendar/manager_test.go` | 21 |
| `internal/assistant/engine/store_test.go` | 20 |
| `internal/api/marketdata/routes_boundaries_test.go` | 19 |
| `internal/assistant/engine/session_context_test.go` | 19 |
| `pkg/backtest/conservative_bar_executor_test.go` | 19 |
| `cmd/check-go-coverage/changed_lines_analysis_test.go` | 19 |
| `internal/app/apiserver/marketdataapp/market_http_test.go` | 18 |
| `internal/app/apiserver/servercoretest/broker_new_test.go` | 18 |
| `internal/assistant/engine/store_lifecycle_test.go` | 18 |
| `internal/integration/yfinance/conversion_test.go` | 18 |
| `internal/api/marketdata/routes_test.go` | 17 |
| `internal/app/apiserver/lifecycle/lifecycle_test.go` | 17 |
| `internal/app/apiserver/webaccess/security_integration_test.go` | 17 |
| `internal/store/sqliteschema/catalog_test.go` | 17 |
| `pkg/strategy/pine/parse_collection_test.go` | 17 |
| `internal/api/assistant/adk_routes_test.go` | 16 |
| `internal/app/apiserver/datamigration/maintenance_failure_paths_test.go` | 16 |
| `internal/assistant/engine/input_request_test.go` | 16 |
| `pkg/futu/exchange_kline_test.go` | 16 |
| `internal/productfeatures/provider_projection_test.go` | 16 |
| `internal/backtest/service_test.go` | 15 |
| `internal/integration/futu/subscription_reconciler_test.go` | 15 |
| `internal/marketdata/instrument_resolver_test.go` | 15 |
| `internal/marketdataassets/asset_selection_boundaries_test.go` | 15 |
| `cmd/check-go-coverage/profile_analysis_test.go` | 15 |
| `internal/trading/order_updates_test.go` | 15 |
| `internal/app/apiserver/backtestapp/historical_source_test.go` | 14 |
| `internal/app/apiserver/server_test.go` | 14 |
| `pkg/backtest/internal/storage/store_runtime_invariants_test.go` | 14 |
| `pkg/backtest/store_test.go` | 14 |
| `internal/integration/futu/marketdata_runtime_test.go` | 14 |
| `internal/marketdataassets/cache_test.go` | 14 |
| `internal/exchangecalendar/http_source_test.go` | 14 |
| `internal/integration/akshare/boundaries_test.go` | 14 |
| `internal/app/apiserver/servercore/runtime_trading_test.go` | 13 |
| `internal/assistant/engine/mcp_server_test.go` | 13 |

## 最近验证

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked`：20/20 通过（nextest run `ba80ce7b-485a-4c2d-9ea1-08011ea97730`）。
- 该结果仅证明 `jftrade-strategy` 当前测试集合可执行，不会自动提升未建立函数级映射的 `[~]` 条目。
