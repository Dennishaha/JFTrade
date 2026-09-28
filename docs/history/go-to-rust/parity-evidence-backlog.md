# Go → Rust 证据积压清单

本清单只统计缺少函数级 Rust 证据的 `[~]` 项；不代表功能缺失，也不代表已覆盖。每项需要人工对照 Go 断言并补充真实 Rust 测试函数、命令或边界结论。

当前积压：**2955 项**（按当前 `manual-test-mappings.json` 的 `[~]` 条目重算）。

最新审计快照（2026-09-28 Assistant session-context2 strict follow-up）：Go `4451`、Rust `3382`，`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；Parity anchor reconcile 为 `1793/1747/0/0/46`（unique/recorded/unrecorded/stale/unknown）。严格审计仍有 **3696 个 function_exact evidence/receipt gaps**（需 reviewed assertions、Parity anchor、reuse relation 与 passed receipt）；严格审计未通过。

## 按领域

| 领域 | 条目数 |
|---|---:|
| assistant_workflow | 571 |
| api_transport | 517 |
| other | 503 |
| strategy_pine | 454 |
| backtest_calendar | 313 |
| marketdata_quotes | 162 |
| storage_sqlite | 198 |
| futu_opend | 142 |
| trading_broker | 58 |
| settings_watchlist | 39 |

## 按 Go 文件（Top 50）

| Go 文件 | 条目数 |
|---|---:|
| `cmd/jftrade-desktop/main_test.go` | 24 |
| `pkg/backtest/pineworker_command_executor_test.go` | 23 |
| `pkg/bbgo/types/indicator_test.go` | 23 |
| `pkg/strategy/indicatorbinding/parse_test.go` | 22 |
| `internal/strategy/pine_live_executor_test.go` | 20 |
| `pkg/backtest/conservative_bar_executor_test.go` | 19 |
| `cmd/check-go-coverage/changed_lines_analysis_test.go` | 19 |
| `internal/integration/yfinance/conversion_test.go` | 18 |
| `internal/app/apiserver/marketdataapp/runtime_test.go` | 17 |
| `internal/store/sqliteschema/catalog_test.go` | 17 |
| `pkg/strategy/pine/parse_collection_test.go` | 17 |
| `internal/marketdata/instrument_resolver_test.go` | 15 |
| `cmd/check-go-coverage/profile_analysis_test.go` | 15 |
| `internal/app/apiserver/server_test.go` | 14 |
| `internal/backtest/service_test.go` | 14 |
| `pkg/backtest/internal/storage/store_runtime_invariants_test.go` | 14 |
| `pkg/backtest/store_test.go` | 14 |
| `internal/integration/akshare/boundaries_test.go` | 14 |
| `pkg/strategy/pine/parse_test.go` | 14 |
| `internal/app/apiserver/lifecycle/lifecycle_test.go` | 13 |
| `internal/app/apiserver/servercoretest/broker_new_test.go` | 13 |
| `internal/marketdataassets/asset_selection_boundaries_test.go` | 13 |
| `internal/marketdataassets/cache_test.go` | 13 |
| `internal/store/strategy/store_test.go` | 13 |
| `internal/strategy/pineruntime/runtime_test.go` | 13 |
| `pkg/backtest/pineworker_adapter_test.go` | 12 |
| `internal/marketdata/collector_test.go` | 12 |
| `internal/marketdata/subscription_lifecycle_test.go` | 12 |
| `internal/integration/akshare/provider_company_research_test.go` | 12 |
| `internal/integration/yfinance/client_test.go` | 12 |
| `internal/trading/order_updates_test.go` | 12 |
| `internal/app/apiserver/backtestapp/historical_source_test.go` | 11 |
| `internal/app/apiserver/runtimes/handle_lifecycle_test.go` | 11 |
| `pkg/backtest/filter_store_session_queries_test.go` | 11 |
| `internal/integration/futu/marketdata_runtime_test.go` | 11 |
| `pkg/market/market_normalization_test.go` | 11 |
| `pkg/market/market_test.go` | 11 |
| `internal/integration/yfinance/provider_test.go` | 11 |
| `internal/watchlist/futu/source_test.go` | 11 |
| `internal/strategy/liveruntime/manager_boundaries_test.go` | 11 |
| `internal/strategy/pine_live_command_test.go` | 11 |
| `pkg/strategy/pine/parse_object_test.go` | 11 |
| `pkg/strategy/pine/parse_semantic_test.go` | 11 |
| `internal/assistant/workflow_crud_test.go` | 10 |
| `internal/backtest/input_and_readiness_validation_test.go` | 10 |
| `internal/backtest/sync_test.go` | 10 |
| `internal/marketdata/service_facade_test.go` | 10 |
| `pkg/bbgo/types/rbtree_test.go` | 10 |
| `scripts/go-test-quality/main_test.go` | 10 |
| `internal/store/sqliteconn/conn_test.go` | 10 |

## 最近验证

- 2026-09-26：Backtest/Calendar P1 取消边界先红后修；`production_helper_sync_cancel_aborts_in_flight_request` 在修复前 2 秒超时，补 worker cancel signal 后由 `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(production_helper_sync_cancel_aborts_in_flight_request)'` 通过。映射仍为 `[~]`，因为 Rust 尚未直接断言 Go 的 `context.Canceled` 与内存 progress 快照。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked`：20/20 通过（nextest run `ba80ce7b-485a-4c2d-9ea1-08011ea97730`）。
- 该结果仅证明 `jftrade-strategy` 当前测试集合可执行，不会自动提升未建立函数级映射的 `[~]` 条目。

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked`：36 项执行，36 通过，1 项跳过（nextest run `5817aea5-d89d-40ee-bf4b-1eb706127be6`）。

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`：1,066/1,066 通过，0 跳过（日志 `/tmp/jftrade-engine-nextest.log`，汇总耗时 120.745s）。

- `pnpm run check:rust`：workspace nextest 1,862 项中 1,862 通过、2 跳过（摘要耗时 125.259s）；SQLite/backtest/provider/trading-strategy/assistant/API/desktop compatibility replay 全部通过。完整日志：`/tmp/check-rust.log`。
- 2026-09-27 strategy_pine P1：`TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs` 先红后修（未修复实现 hour 预期 180、实际 1170），新增 planner `estimate_security_source_bars_handles_period_and_timeframe_fallbacks` 后 nextest 1/1 通过；修复 `resolve_timeframe_minutes` 的 hour canonical alias，mapping/reuse/anchor 已同步。当前全局审计为 Go 4451、Rust 3370、`function_exact=1493`、`partial=2332`、`boundary=628`、`missing=0`；`parity_anchor_reconcile.py` 为 1783/1737/0/0/46。`audit_test_parity.py --strict` 仍因 evidence/receipt gaps 失败，未伪造 receiptDigest。
- 2026-09-27 API transport P1：`TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 先红后修，新增 quote-read 四路显式 brokerId guard 与 active alias 正向回归。active=yfinance + `brokerId=futu` 现在在 provider/helper/OpenD 读取前返回 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED`，且 helper 请求数为 0；该条因 Rust 无 Go 同形 broker reader registry 仍是 partial。mapping/reuse/anchor 已同步，receiptDigest 保持空。
- 2026-09-27 API transport P1：`TestMarketQueryAndExecutionPayloadFallbacksRemainDeterministic` 补齐可迁移行情断言。新增 pathTail 正常/短路径回归与 `limit=0`/负数→1 的真实路由回归；旧实现先红为 `request.limit=200`，修复后 6/6 精准 nextest 通过。blank limit→默认 200、非法输入拒绝和反向时间窗 fallback 已有证据；execution payload/helper 语义因 Rust 无同形 seam 继续列为 partial，未伪造 receiptDigest。
- 2026-09-27 Backtest/Calendar P1：`TestHistoricalKLineSyncerRejectsBrokenPagination` 新增生产同步回归 `production_helper_sync_rejects_broken_pagination_cursors`，覆盖 missing `nextBefore`、向前 cursor 失败及到达 `since` 边界成功落库，定向 nextest 1/1 通过。映射保持 partial（Rust yfinance helper/durable task 与 Go 可注入 HistoricalKLineSyncer/source、futu provider、内存 progress seam 不同形），mapping/reuse/anchor 已同步，未伪造 receiptDigest。
- 2026-09-27 Broker disconnected/degraded P1：Go `broker-read.json` 与 servercore 断连测试冻结 13 路 200 degraded；Rust 生产 broker-read owner 由 `broker_read_routes_fail_closed_when_snapshot_port_is_unavailable`、`broker_read_fails_closed_without_trade_client`、`broker_klines_valid_request_fails_closed_without_historical_source` 与 securities missing-router 测试固定为 503 `BROKER_READ_UNAVAILABLE` 并保留上游错误。显式 test-cutover fixture 仍回放旧 200 envelope；不改生产 wire。相关 servercore/read-failure mapping 已改为 `boundary`，混合 service fallback 行保留 `partial` 并注明 source-gated 三路边界，receiptDigest 继续为空。
- 2026-09-27 Futu/OpenD runtime P1：`TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts` 已由真实 OpenD mock 覆盖 global-state probe → `SharedTradeReadRuntime` → production broker runtime route，断言 connected startup、`serverVersion=10.9.7000`、markets/health、2 个 discovered accounts 及 REAL 优先排序。`product_runtime_composes_opend_provider_and_fences_shutdown_ownership` 定向 nextest 1/1 通过，映射已由旧 account/order reconciliation 证据行升级为 `function_exact`；真实 receipt 为 `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`（[workspace receipt](verification-receipts/workspace-nextest-2026-09-27T061100Z.json)）。strict audit 当前仍有 4085 个历史 evidence/receipt gaps，未伪造其他 digest。

- 2026-09-27 provider health backoff P1：`TestProviderHealthRetryDelayBacksOffAndCaps` 先红后修。新回归 `helper_restart_policy_defaults_match_go_provider_health_retry_delays` 发现默认 500ms 与 Go 首次 100ms 不一致；修复 `HelperRestartPolicy` 与 managed helper restart policy 为 100ms 初始、1s 上限后，定向 engine nextest 2/2 与 sidecar 回归 3/3 通过，mapping 升为 `function_exact`。同文件 cancellation/error 透传条目仍为 partial；receiptDigest 保持为空。当前 `[~]` 积压按映射重算为 2955 条，严格审计 evidence/receipt gaps 仍未收口。

- 2026-09-27 strict evidence batch：人工逐条复核 17 条单引用 `function_exact`（Assistant/ADK 11 条，Strategy/Pine warmup 与 error normalization 6 条）的 Go 与 Rust 测试体；17 条均已有 `// Parity:` anchor、workspace receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8` 中的 passed test，且 `assertionCoverage.source=reviewed`。未覆盖子断言的 session-negative、空数组与 stream transport 条目未纳入本批。严格审计缺口由 4085 降至 4021，仍不能宣称整体 strict 通过。
- 2026-09-27 全量验证收尾：当前工作树 workspace nextest 3504/3504 passed、0 failed、2 suite skipped；七类 compatibility replay 全部通过。结构化 receipt 为 `sha256:2f7422488ce7addc2b81ad38563b3b7662345283896be959843301f2b609d66f`（[receipt](verification-receipts/workspace-nextest-2026-09-27T071942Z.json)）。当前 `[~]` 积压 2955 条；strict evidence/receipt gaps 为 4019，仍按缺口推进，不宣称严格审计完成。

- 2026-09-27 最新积压快照：API launcher 两条 `[x]` 已有源码锚点与 receipt `sha256:bb5be96baa9877eba7e6d5f9f0f3455fd7d3608f5912999bd6e53cfc8f3bd757`；MarketData/Calendar 三条单引用 `[x]` 使用 receipt `sha256:9d3a85b571117001d7de7cb3a4badbf5f9825c308f243b51763b87426dc22115`。`audit_test_parity.py --strict` 当前真实失败为 3957 个 gap，不能把 workspace nextest 全绿当作 strict parity 完成。
- 2026-09-27 Assistant session-context 最新收口：三条单引用 `[x]` 均已关联 receipt `sha256:917fa4ba6a8484f68cb156733271d11cc5da94a702b60e52d1f91229a1ea5078`（3/3 passed），strict gap 进一步降至 3948；其余历史 function_exact 仍逐条等待 reviewed/reuse/receipt 证据。
- 2026-09-27 Watchlist quote P1 收口：三条单引用 `[x]`（extended session projection、overlapping batch single-flight、provider cache policy）均已关联 receipt `sha256:ae6267da865221f941864dca401b67519a51c0a38b9c0069be36bd4010bad7e3`（3/3 passed）。当前 Go 4451、Rust 3380、`function_exact=1499`、`partial=2318`、`boundary=634`、`missing=0`；anchor `1791/1745/0/0/46`；strict gap 3927。SG.D05 timezone、metadata side effect 与 watchlist port-level fallback/permission isolation 仍是明确 partial，不纳入 exact。
- 2026-09-27 Calendar health strict evidence：三条单引用 `[x]` 已补 `assertionCoverage.source=reviewed` 并关联 receipt `sha256:27d4c070d9bbff94444923e57d51a73b4f62069b70fc6dd5fe357382c0e87bcc`（3/3 passed）；单引用 reuse/Parity anchor 已存在。strict gap 3921，剩余历史 exact rows 继续按 reviewed/reuse/receipt 缺口推进。
- 2026-09-27 Backtest retry P1 收口：`TestHistoricalProviderRetryExhaustionAndTimerCancellation` 的两条 helper owner 测试已通过定向 nextest 2/2；固定退避取消先红后修为取消感知轮询，映射由 partial 升为 `function_exact`。两个 Rust evidence 共用该 Go reference 的 `Parity:` anchor，reuse 为两条单引用，`assertionCoverage.source=reviewed`，receipt `sha256:26dc42121e439058156a76cfe41acd1d0c041eb7104b2f320d99861e119019dc`。当前 exact 1500、partial 2317、boundary 634，anchor `1792/1746/0/0/46`；strict gap 仍为 3921，下一批优先处理已有 anchor/reuse 的 strict evidence rows。
- 2026-09-27 Trading/Broker strict evidence C 批收口：三条已有 Trading exact（derivative single-leg preview、unreadable real-trade control plane、env fallback + US overnight normalization）完成 Go/Rust 断言复核，补 reviewed assertion 与 receipt `sha256:77f918d03712f238ef42d3340f4cc5c73f951a0406e534d74b4c3ad251cfe258`（3/3 passed）；三条 reuse 均为单引用且已由 `parity_anchor_reconcile.py` 记账。无生产变更，strict gap 由 3921 降至 3915，继续处理剩余历史 evidence/receipt 缺口。
- 2026-09-27 Query/MarketData strict evidence：逐条复核 `TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues`、`TestDecodeMarketCandlesQueryParsesRepeatedSessions` 与 `TestCandlesRouteNormalizesRepeatedSessions`。前两条保留 query owner 的空白/非法时间与 repeated-session 去重断言，补 `Parity:` anchor；路由条目先让 provider session 断言转红，修正为分页请求序列 `RTH/RTH/ETH/ETH/ALL/ALL` 并锁定去重后的三条唯一路由。三条定向 nextest 3/3 通过，receipt `sha256:6d383be156db7803a762e52849c2d20b09d245fe42aac931543f27d95ee59496`；共享 query/candle owner 的多引用 reuse 已 reviewed，相关 Go session 聚合差异继续保持 partial。当前 exact 1500、partial 2317、boundary 634；anchor `1793/1747/0/0/46`；strict gap 3901，整体仍未通过。
- 2026-09-27 API transport auth strict evidence：复核 Origin/CSRF 会话写、PATCH 会话写、浏览器禁用页（含 engine 状态）与密码会话读写五条 P1 exact；联合 `jftrade-api`/`jftrade-engine` nextest 5/5 通过，receipt `sha256:c5457db228673e47ae252a104992aa8c261fce3df081f60dfd28cf7c6682cc17`。五条 assertion 均标记 reviewed，保留现有 Parity anchor/reuse；strict gap 降至 3885，整体仍未通过。
- 2026-09-27 Assistant/API strict evidence：复核 `TestADKSessionsCRUDAndFilteringRoutes`、`TestChatStreamTransportHandlesDisconnectedClients`、`TestStreamReconnectAndSkillContracts`、`TestCatalogSessionRunAndObservabilityContracts` 与 `TestSessionTimelineFailureKeepsLegacyErrorCode`；复合映射的 composer、live-stream、disconnect、replay、catalog 与 timeline owner 全部纳入联合 engine nextest，8/8 通过。receipt `sha256:fb75c71c988292130d6e3f017e66f1a2a3de3f3d19b3ec106a6af3ca84da3c7e`；五条 assertion 标记 reviewed，strict gap 降至 3875，整体仍未通过。
- 2026-09-27 Backtest/API strict evidence：复核 `TestBacktestSyncRouteRejectsObsoleteSessionScope`、`TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound`、`TestStatusResultAndDeleteRoutesCoverTerminalAndStoreFailures`、`TestHistoricalPageParsingRejectsMalformedProviderValues` 与 `TestStartScriptRejectsBlankResearchScript`；联合 engine nextest 覆盖 sync scope、progress/cancel、status/result/delete、provider page conversion 与 blank script，10/10 通过。receipt `sha256:fda37329aa034a644056012e122883e27d99a6586ad7bf82003d3af39f110f6a`；strict gap 降至 3870，整体仍未通过。
- 2026-09-27 MarketData boundary strict evidence：复核 candle period 空白/别名、provider generation fence、invalid sessions、subscription canceled mapping 与 logical cleanup 后 release/clear 五条 P1 exact；联合 engine nextest 6/6 通过，receipt `sha256:baa66579a6a1866149f08707d9e80790638ec1fb29f48ed34962a50ae5f17478`。五条 assertion 均完成 reviewed/passed receipt，strict gap 降至 3865，整体仍未通过。
- 2026-09-27 MarketData/API forwarding strict evidence：复核 news/corporate-actions 参数与转发、tick fresh-cache/strict-before、managed-account not-found 与 strategy activity pagination 五条 P1 exact；联合 engine nextest 11/11 通过（activity owner 在 lib 与 bounded integration binary 均通过），receipt `sha256:a583ff41724cfc3bcd259fcf9627a7c8594e6a9aa5c4a79e0082cf332a71c622`。strict gap 降至 3860，整体仍未通过。
- 2026-09-27 Execution/Watchlist strict evidence：复核 activity time/pagination、execution cancel envelope、broker empty arrays 与两条 watchlist business exact；engine/store-sqlite 联合 nextest 10/10 通过，receipt `sha256:53f26ac779a8505c741acb4ea4e9f6316da471fb6e04f0eeeea7b823f63e301b`。strict gap 降至 3855，整体仍未通过。
- 2026-09-27 Runtime/Maintenance strict evidence：复核 installers rollback、SQLite backup/cleanup、Futu quote health/disconnect、OpenD health、provider activation 五组 P1 exact；engine/store-sqlite/integration-futu/marketdata/desktop 联合 nextest 10/10（含 1 个 live OpenD ignored）通过，receipt `sha256:b58fc1c29238f5349c80b07263e7375b665440d25cbe46ae45201a4903967df6`。strict gap 降至 3850，整体仍未通过。
- 2026-09-27 Market HTTP strict evidence：复核 US intraday session labels/unknown-session error 与 snapshot fresh-cache、cache-miss、force-refresh 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:4c4bec4be84d55d241713c361ec13c8feb7011c4863b51821d1b51df8c679a80`。strict gap 降至 3845，整体仍未通过。
- 2026-09-27 Helper/runtime strict evidence：sidecar manager reuse、bounded kill escalation、Node macOS fallback/required aggregation、desktop password session 与 strategy runtime panic recovery 六条 P1 exact 已完成 reviewed assertion 与统一 receipt `sha256:52ef8cc1e2ccf7d57130269388da3d90544630132116f3cc4cf9c431d7de2c51`；marketdata-helper/engine 联合 nextest 7/7 通过。strict gap 降至 3833，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 API portfolio/settings strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:cf733b63c6b03f627d273983d92eff928ecb09de97b1f79be6c6c95d04e3ee76`（api/engine/settings-file 联合 nextest 5/5）；覆盖 browser-only security write、portfolio cash/degraded/removed route 与 broker settings defaults。strict gap 降至 3823，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu marketdata runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:57f5e6283bc3618f2b76411d6374d775dc85c6a8e939d6512d8c0615c06b4899`（integration-futu/marketdata 联合 nextest 5/5，live OpenD ignored）；覆盖 cache inheritance、fallback filtering/projection 与 generation fencing。strict gap 降至 3813，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Assistant boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:7f97c2a3cc83a59db7c1bfa55be2233314f69e89bf29de08c35c2ba56592506`（engine nextest 5/5）；覆盖 MCP auth/catalog、session context、input cancellation、approval resume 与 missing-run continuation。strict gap 降至 3803，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu K-line pagination strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:489957b0b5e2b5d8f04047abee16c357751def89d390ec871a2016be66b24c70`（engine/integration-futu 联合 nextest 5/5，live OpenD ignored）；覆盖 cursor/page normalization、market-time conversion 与 inclusive boundaries。strict gap 降至 3793，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 API/marketdata tail strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:7612368c8b4ebc983e552ba11b03a5ede9cad29575c83c5728c8578411901a69`（engine/marketdata 联合 nextest 5/5）；覆盖 provider retirement、status normalization、cookie-only WS、tick volume delta 与 pagination metadata。strict gap 降至 3783，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Backtest/Calendar/Settings strict evidence：五条 P1 exact 已完成 reviewed assertion 与 receipt `sha256:e2ffadfff2d2d62a4d697f5bf9a65f47f446f2b60316848ed77cac4d6517ae34`（engine/calendar/settings 联合 nextest 6/6）；覆盖 DST range、calendar validation/cache indexing、empty broker defaults 与 watchlist provider failure preservation。strict gap 降至 3774，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Trading/Broker control-plane strict evidence：五条 P1 exact 已完成 reviewed assertion 与 receipt `sha256:6246b9ab3c1d3050579515161c6005f3103a8eb2ac583573a6369db130eeb2bc`（engine nextest 9/9）；覆盖 control-plane availability/audit persistence、option combo lifecycle、forced reconciliation wake 与 capability route table。strict gap 降至 3764，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu subscription reconciler strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c4734a37b0151e32c4e87c9b949f9ae10fa286364e4e337484fd9eb1b2bb1577`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 deferred release、ack-based retention/retry、connection ownership cleanup 与 failed-record pruning。strict gap 降至 3754，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu OpenD boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:b10f6c20a8a268cddbaed2a56114e7e63c4a24caad8e1cd6a856d6c70d17c420`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 timeout/keep-alive、history decode、depth closed-session 与 trading disconnected boundaries。strict gap 降至 3749，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu session/trade strict evidence：五条 Go 映射、七个 Rust owner tests 已完成 reviewed assertion 与 receipt `sha256:9b9427f38df767a2d63f217953fd27bae053fda603622c86d9a7ef33d51b3814`（engine/integration-futu 联合 nextest 7/7，live OpenD ignored）；覆盖 session normalization/defaults、previous-close rules、trade authority 与 CN location fallback。strict gap 降至 3739，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-gate strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c6e22cccbab1c95b191598b13684cc7d6632461fc80d06f8fbed43fb736981b2`（engine nextest 5/5）；覆盖 compaction gate、workflow parent、pending approval tail 与 active-run detection。strict gap 降至 3729，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu subscription/trade strict evidence：六条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:66b32e68a91fcc48f6557dcb436c9ea2d5d16af485d6418fcddc83bb83a25dc1`（integration-futu + engine nextest 6/6）；覆盖 subscription ack/retry、connection generation fencing、terminal close、failed-connect fail-closed 与交易写入断连前置条件。strict gap 降至 3723，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu protocol strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:6478da8aebc98d804488cf76f032ff5ceaf9cc33c3a3fd38aaff652d8355429a`（integration-futu nextest 5/5）；覆盖 prediction catalog identity/pagination、research request injection、history pagination/filter forwarding 与 user-security error preservation。strict gap 降至 3718，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-context strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:0ca3d7f2e3b1e564c6fc25e2a91823516d7de78fb8e56d03ee042f9a272c92da`（engine nextest 5/5）；覆盖 read-pressure projection、model auto-compaction 与 pending-approval protected-tail selection。strict gap 降至 3713，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu K-line boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:f38ccd784ac1bf1afd47e5a14600fc1f016af24883a0787fb35b97b78f55801a`（integration-futu + engine nextest 5/5）；覆盖 period mapping、cursor/time validation、pagination bounds 与 Session_ALL fallback。strict gap 降至 3708，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu snapshot/listener strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:5843770babe2087467db4d181f4c556dcea44dcf03a89385610e9ded8179dad8`（integration-futu + engine nextest 5/5）；覆盖 snapshot batching/cache、sliding budget/rate-limit/cancellation、empty result handling 与 basic quote malformed-row drop。strict gap 降至 3703，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant store-ops strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:66c0e83d636b8f8045e81d60c6cd382808d168af88f8d211cc8739dcdc9df635`（engine nextest 5/5）；覆盖 agent ownership guard、cancel/deny transaction、missing-target classifications 与 listing filters/sort。strict gap 降至 3698，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-context2 strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:83808bfd4b00fe6f1aacfe581f8ee1453badfe7659bff03d5d281583ba62163a`（engine nextest 5/5）；覆盖 provider override/revision compaction、handoff revision filtering、append visibility 与 pending approval preservation。strict gap 降至 3696，仍有历史 evidence/receipt 缺口，整体未通过。
