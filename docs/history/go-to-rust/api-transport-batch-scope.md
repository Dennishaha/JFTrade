# API、SSE、WebSocket、Transport 领域对齐批次

本批按复合键（Go 文件路径、行号、测试名）核对 5 条 P0/P1 API/Transport
测试，并将已有 Rust 函数作为逐项证据入口。所有条目均保留为 `[~]`/`partial`：
Rust 测试覆盖了对应协议或错误边界，但没有把不同的路由装配、认证上下文或
业务范围误判为完整等价。

| Go 测试 | Rust 证据 | 风险与差异结论 |
| --- | --- | --- |
| `internal/api/marketdata/routes_boundaries_test.go:63:TestCandlesAndDepthRoutesMapProviderFailures` | `product_market_data_quote_read_tests::market_microstructure_quote_routes_preserve_provider_error_mapping` | P1 provider 错误映射。Rust 覆盖 session/decode/rate-limit 状态、错误码和 retry-after；Go 还覆盖 candles route 与 depth 参数转发，入口不同。 |
| `internal/api/marketdata/routes_test.go:459:TestReadRoutesCoverMarketsSecuritySnapshotSearchHeartbeatAndNormalize` | `product_market_data_quote_read_tests::market_microstructure_quote_routes_reject_invalid_queries_before_reader_call` | P1 非法 query。Rust 断言 reader 不被调用；Go 还覆盖 provider/markets/security/snapshot/search/heartbeat 成功契约。 |
| `internal/app/apiserver/servercoretest/portfolio_routes_test.go:43:TestPortfolioReconciliationEndpointsAreRemoved` | `jftrade-api::transport_contracts::unknown_portfolio_reconciliation_route_returns_json_not_found` | P0 旧写入口移除。Rust 只覆盖 cash-reconciliation 的 JSON 404；Go 同时覆盖 positions reconciliation，且认证/服务装配不同。 |
| `internal/app/apiserver/servercoretest/system_routes_test.go:89:TestRequestObservabilityMiddlewarePropagatesRequestID` | `jftrade-api::transport_contracts::desktop_token_reaches_port_with_stable_envelope_and_request_id` | P1 request-id wire。Rust 断言合法 ID 回写、port dispatch 与 envelope；Go 验证真实 `/system/status` route，业务路径不同。 |
| `internal/app/apiserver/webaccess/security_integration_test.go:510:TestRemovedAuthTokenRouteReturnsNotFound` | `jftrade-api::auth_session_transport_contracts::removed_auth_token_route_returns_not_found` | P0 已移除认证入口。Rust 与 Go 均断言 404；Go 先登录 cookie session，Rust 使用最小 ApiState，保留上下文差异。 |

## 验证与后续

上述每项的精确 nextest 表达式已写入
`manual-test-mappings.json`，并由 parity audit 校验 Rust 文件和函数入口存在。
本批不升级任何 `[x]`，也不把数量比当作覆盖率。下一批应补齐 SSE writer 的
panic/flush/并发失败行为、CORS preflight 与 WS close 生命周期，并为 candles
和完整 market-data read 成功路径建立独立 Rust route 测试。

## 第一百二十六批分片五（第一批）：`internal/api/httpserver` 绑定层 + `internal/api/live` WS 边界

### 分片四归属核实：`internal/assistant/engine/*` 无剩余 trading_broker 行

按域前缀（`assistant_workflow = internal/assistant`）与两轮扫描核实：

1. 以 `rust_entry` 关键词（execution/broker/trading/order/ledger/fill/reconcile）匹配到 20 条 pending，
   逐条检查后全部是 ADK 工作流执行/审批对账/执行租约（`product_adk_*`、`adk_store_contracts`），
   owner 属 assistant 域，不是券商执行域；
2. 以 Go 测试名（`Test*{Trade,Order,Broker,Portfolio,Execution,RiskLimit,HardStop,KillSwitch}`）匹配到 39 条 pending，
   同一结论；其中助手工具调用账户订单的 3 条（`tools_test.go:468/:605/:837`，account-orders/慢组合/流式）已是 `[x]`。

结论：trading_broker 域在这些文件里没有未收口行，后续由 assistant_workflow 批次处理；本条写进清单与批次文档，避免重复扫描。

### 本批范围（27 行）

`internal/api/httpserver/bindings_test.go` 6 行 + `bindings_boundaries_test.go` 6 行（Go 查询绑定 helper）、
`internal/api/live/handler_test.go` 8 行 + `dispatcher_boundaries_test.go` 7 行（WS/SSE dispatcher 边界）。
owner：`crates/jftrade-engine`（product_query 查询归一、ws-live 契约模型）与 `crates/jftrade-api`（router/sse/websocket/auth）。

### 新增 Rust 测试（4 条，全部通过）

| 用例 | 位置 | 覆盖 Go 引用 |
| --- | --- | --- |
| `documented_candle_period_families_match_the_go_table` | `crates/jftrade-engine/src/product_query.rs` | `bindings_boundaries_test.go:37` |
| `optional_query_time_normalizes_to_utc_and_blank_means_absent` | 同上 | `bindings_test.go:14`、`bindings_boundaries_test.go:61`（空白半段） |
| `uri_escape_validation_accepts_literal_percent_and_rejects_malformed` | 同上 | `bindings_test.go:56/:75`、`bindings_boundaries_test.go:70`（转义半段） |
| 既有 `candle_period_normalizes_aliases_and_rejects_unsupported` 补 ` 60m ` 断言 | 同上 | `bindings_test.go:186`（别名半段） |

### 映射结果

4 行升 `[x]`（`bindings_boundaries_test.go:37`、`bindings_test.go:14/:56/:75`），23 行收紧 `[~]`/partial 并写明缺口 owner 与回归要求。
计数：`[x]` 1446 → 1450（function_exact 1450 + partial 2390 + boundary 607 + module_only 4 + missing 0）；
audit Rust 测试 3159；anchors 1458（unrecorded 0、stale 0、unknown 54）。

### 关键事实与新登记缺口（P1/P2）

1. **空周期语义差异（P2）**：Go `CandlePeriodValue` 空白输入置空且不报错（等价未设置）；Rust `normalize_candle_period("")` 返回 Unsupported，
   candles 路由先 `unwrap_or("1m")` 取缺省，因此显式 `period=` 空白会 400 `invalid candle query`。owner = `product_production_ports_market_data_quote_reads.rs`。
2. **非法时间语义差异（P1）**：Go `ParseQueryTime` 对不可解析文本静默返回调用方 fallback；Rust 返回 Err 并由路由映射 400 `time must be a valid timestamp`
   （`from/to` 与 `before` 同族）。owner = product_query + candles/trade 读路由；需产品确认目标语义。
3. **可选布尔别名不全（P2）**：Go 识别 `off` 与空串为 false；Rust `optional_bool_strict` 只接受 `false/0/no/n`（trim+lowercase），`off`/空串返回 invalid boolean。
   owner = 工具/查询布尔强制层。
4. **三态可选值无同形对象（P2）**：Go 的 OptionalInt/Bool/Time 共享 Set/Valid 三态；Rust 以每字段 Option/Result 表达，需按字段补三态断言。
5. **分页夹取未抽层（P2）**：Go `NormalizeBoundPage` 是独立 helper（默认 50/上限 200）；Rust 在 candles 路由内联（默认 200/上限 1000），缺路由级夹取断言。
6. **NOT_FOUND 文案差异（P2）**：Go `WriteNotFound` 固定 `resource not found`；Rust 未知路由走 `ApiFailure::new(404,"NOT_FOUND","unknown endpoint <path>")`。
7. **WS 生命周期缺口（P1）**：`handler_test.go:113/:342` 的 heartbeat、security/depth payload、num=50 限制、Close 主动断连与 stats 清零在 Rust 无独立断言；
   `:207/:244/:316` 目前只有冻结语料回放，没有真实连接验证。owner = `crates/jftrade-api websocket` + engine ws-live 端口。
8. **同源握手缺口（P1）**：`handler_test.go:421` 需要“同源 Origin → 101 + 首帧”的正向断言；Rust 现有的是可信调用方 CSRF 放行与恶意 Origin 403 两侧。

### 验证记录

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增/补强 4 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(product_query)'` 等精确表达式 | 全部通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b127s1_apply.json` | 27 行更新，`[x]` 1446 → 1450 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一 |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1458、unrecorded 0、stale 0 |

### 后续（下一分片）

`internal/api/live` 剩余 SSE 边界（`sse_*` 已 `[x]` 部分保持复核）、`internal/api/middleware`（security/adk/auth 余量）、
`internal/api/trading` 与 `internal/api/strategy` 余量，以及 `internal/api/settings`（12 行）等按体量推进；
每个分片继续按“读 Go 断言 → 升 `[x]` 或收紧 partial（缺口 + owner + 回归要求）”执行。

### 分片五 b：`internal/api/settings` + `internal/api/middleware`（33 行）

范围：`internal/api/settings/routes_test.go` 12、`routes_failure_boundaries_test.go` 6、`routes_market_data_test.go` 4、
`routes_accounts_validation_test.go` 2、`adk_routes_contracts_test.go` 1、`routes_uri_boundaries_test.go` 1；
`internal/api/middleware/security_boundaries_test.go` 4、`adk_test.go` 2、`auth_test.go` 1。
owner：`crates/jftrade-engine`（settings 读投影/写端口、product_wire 错误映射）、`crates/jftrade-api`（router/auth/envelope）、
`crates/jftrade-settings`、`crates/jftrade-store-settings-file`。

新增与补强测试（全部通过）：

| 用例 | 位置 | 覆盖 Go 引用 |
| --- | --- | --- |
| `settings_write_routes_reject_malformed_json_before_persistence` | `crates/jftrade-engine/src/product_tests.rs` | `settings/routes_failure_boundaries_test.go:20`（逐条 8 路由 400 BAD_REQUEST） |
| `managed_account_write_routes_map_missing_records_to_not_found` | 同上 | `settings/routes_test.go:350`（PUT/DELETE 缺失账户 → 404 + NOT_FOUND 族） |
| `same_origin_options_preflight_is_allowed_without_reflected_origin` | `crates/jftrade-api/tests/transport_contracts.rs` | `middleware/security_boundaries_test.go:62`（同源 OPTIONS → 204 不回显） |
| `authenticated_session_read_without_browser_origin_is_allowed` | 同上 | `middleware/security_boundaries_test.go:17`（第三子用例：已认证读无 Origin 放行） |
| `write_method_classification_covers_state_changing_verbs` | `crates/jftrade-api/src/router.rs`（新增测试模块） | `middleware/security_boundaries_test.go:46`（写方法分类表） |
| `origin_normalization_accepts_web_and_tauri_schemes` 扩展 | `crates/jftrade-api/src/auth.rs` | `middleware/security_boundaries_test.go:108`（畸形/不支持输入表） |
| `blank_account_id_is_rejected_before_persistence` / `create_account_clears_client_owned_identity_and_timestamps` 补锚点 | `crates/jftrade-settings/tests/managed_account_validation.rs` | `settings/routes_test.go:376/:393` |

映射结果：6 行升 `[x]`（`middleware/security_boundaries_test.go:17/:62`，`settings/routes_failure_boundaries_test.go:20`，
`settings/routes_test.go:350/:376/:393`），27 行收紧 `[~]`/partial（含缺口 owner 与回归要求）。
计数：`[x]` 1450 → 1456；audit Rust 测试 3164；anchors 1466（unrecorded 0、stale 0）。

关键事实与新登记缺口（P1/P2）：

1. **设置写路由畸形载荷已收口（P1/P2）**：8 条写路由统一 400 BAD_REQUEST 且不落盘，与 Go 逐路由断言一致。
2. **托管账户缺失记录 404 已收口（P1）**：PUT/DELETE 缺失 id 返回 404 + NOT_FOUND 族错误码，不再隐式创建或落成 500。
3. **桌面自定义 scheme 差异（P1，边界）**：Go `canonicalOrigin` 接受 `wails://`，Rust `canonical_origin` 只接受 `http|https|tauri`
   （本工作区桌面壳为 Tauri）；`security_boundaries_test.go:108` 保持 partial 并登记该差异，其余畸形/不支持输入已逐条断言。
4. **写方法检测的 Go 专属形态（P2）**：可注入 write detector 与 nil `*http.Request` 在 Rust 无同形对象（无 nil 请求、无接口注入），
   分类表已由新 router 单测覆盖；`security_boundaries_test.go:46` 保持 partial 并注明语言边界。
5. **设置持久化失败契约待补（P1）**：`routes_failure_boundaries_test.go:48` 需要“写入失败 → 500 SETTINGS_SAVE_FAILED 且文件未变”
   的按路由 HTTP 矩阵（当前只有 settings-file 契约层的回滚断言）。
6. **ADK 放行/不可用中间件缺独立断言（P1）**：Rust 以“端口可用才注册路由”替代 Go 中间件，需补 `/api/v1/adk` 503 ADK_UNAVAILABLE
   与 200 目录投影两条路由级断言。
7. **MCP token 单次下发（P2）**：已断言 reset 后响应/文件不含明文，缺“第二次 reset 旧 token 失效”的序列断言。
8. **pine-worker/execution 回调副作用（P2）**：设置保存后的运行时应用/回调记录在 Rust 无端口级断言。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| engine 新增 2 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(settings_write_routes_reject_malformed_json_before_persistence) or test(managed_account_write_routes_map_missing_records_to_not_found)'` | 2/2 通过 |
| api 新增/扩展 3 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked -E 'test(write_method_classification_covers_state_changing_verbs) or test(same_origin_options_preflight_is_allowed_without_reflected_origin) or test(authenticated_session_read_without_browser_origin_is_allowed) or test(origin_normalization_accepts_web_and_tauri_schemes)'` | 全通过 |
| settings 契约 58 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings --all-targets --locked` | 58/58 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s5b_apply.json`（+1 行去重修正） | 33 行更新，`[x]` 1450 → 1456 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一 |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1466、unrecorded 0、stale 0 |
| 门禁 | `cargo fmt --check`、`pnpm run check:clippy`、`pnpm run check:compatibility`（278 operations）、`pnpm run check:ai-context`、`pnpm run check:zero-go`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`、`-p jftrade-api`、`-p jftrade-settings` | engine 首跑 1 个已知抖动用例（`api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal`）失败，隔离复跑通过后整轮重跑 1839/1839；api 81/81；settings 58/58 |

后续（分片五 c）：`internal/api/productfeatures/*`（23）、`internal/api/strategy/*`（29）、`internal/api/trading` 余量、
`internal/api/backtest/*`、`internal/api/watchlist/*`、`internal/api/assistant/*` 余量、`internal/api/live` SSE 复核。

### 分片五 c：`internal/api/productfeatures` + `internal/api/system`（32 行）

范围：`internal/api/productfeatures/` 23 行（`research_screen_test.go` 10、`provider_research_routes_test.go` 8、
`routes_test.go` 4、`prediction_combo_routes_test.go` 1）与 `internal/api/system/` 9 行（`routes_test.go` 8、`status_mapper_test.go` 1）。
owner：`crates/jftrade-engine` research 端口族（company/market/calendar/news）、research screen 写端口与结果投影、
系统读/写叶与 `product_wire` 映射，wire 层在 `crates/jftrade-api`。

补强锚点（无新增行为断言，均为注释级引用）：`rankings_reject_unmapped_operations_without_a_helper_call`（`:335`）、
`company_research_rejects_non_default_operations`（`:458`）、`calendar_and_macro_reject_unsupported_operations_without_a_helper_call`（`:606`）、
`embedded_capability_errors_keep_the_broker_code_and_lifecycle_sentinels`（`:229/:335/:458/:606`）、
`futu_screen_write_errors_keep_their_transport_contract`（`:70`）、`research_screen_fixture_replays_go_wire_for_the_post_route`（`:70`）、
`screen_query_defaults_the_page_and_keeps_catalog_columns`（`:47`）、`screen_query_rejects_wrong_catalog_and_schema_versions`（`:47/:185`）、
`route_and_page_validation_precede_provider_calls`（`:185`）。

映射结果：7 行升 `[x]`（`provider_research_routes_test.go:229/:335/:458/:606`、`research_screen_test.go:47/:70/:185`），
25 行收紧 `[~]`/partial（含缺口 owner 与回归要求）。计数：`[x]` 1456 → 1463；audit Rust 测试 3164；anchors 1472（unrecorded 0、stale 0）。

关键事实与新登记缺口（P1/P2）：

1. **内嵌 provider 错误契约已收口（P1）**：capability→409 + `BROKER_CAPABILITY_UNAVAILABLE`、warming→503 + `Retry-After: 1`、
   busy→503 + `Retry-After: 2` 在 rankings/company/calendar/macro 四族均由操作拒绝用例 + 生命周期 sentinel 用例合成断言。
2. **research screen 429 与版本门已收口（P1）**：`RateLimited{retry_after_ms:2500}` → 429 + `RESEARCH_SCREEN_RATE_LIMITED` + `Retry-After: 3`；
   `querySchemaVersion != 2`、catalogVersion 不匹配、`page.limit=101` 均 400 且不触达 provider。
3. **冲突矩阵三条分支未覆盖（P1）**：`research_screen_test.go:387` 的 futu preset→409、`abs_desc` 排序→409、
   “HK 超覆盖请求带 market=HK 到达 provider”在 Rust 无断言；owner = research screen 写端口，需补三条后升级。
4. **catalog 逐路由键集合缺口（P1）**：`system/routes_test.go:19` 的 16 条系统读路由键集合当前只有 2 条进入冻结语料
   （futu-opend、broker-order-updates）；owner = 系统读投影 + `system-read.json` 语料。
5. **控制路由副作用/状态读回缺口（P1）**：manual-retry→reset、calendar refresh/probe→管理器参数、真实交易控制写→读回状态
   三类副作用在 Rust 只断言“共享真实管理器”或 wire 响应，需按路由补断言。
6. **typed screen definition/结果省略 total（P2）**：`research_screen_test.go:80/:130/:213` 缺“端口收到类型化 definition/offset”、
   “v2 可执行字段原样保留”、“provider 省略 total → 响应无 total”三条正向断言。
7. **产品特性路由矩阵缺口（P2）**：`routes_test.go:19/:93/:142/:265` 需补路由族清单、类型化参数保留矩阵、
   校验/资格/provider 4xx 错误映射矩阵与 predictionRoute helper 断言。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 受影响 12 条用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(...)'`（rankings/company/calendar/lifecycle/screen-429/screen-defaults/screen-versions/route-page/fixture-replay） | 12/12 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s5c_apply.json` | 32 行更新，`[x]` 1456 → 1463 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一、unanchored 与上批持平（206） |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1472、unrecorded 0、stale 0 |
| 格式 | `cargo fmt --all -- --check` | 通过 |

后续（分片五 d）：`internal/api/strategy/*`（29 行）、`internal/api/trading` 余量、`internal/api/backtest/*`、
`internal/api/watchlist/*`、`internal/api/assistant/*` 余量与 `internal/api/live` SSE 复核。

### 分片五 d：`internal/api/strategy`（29 行）

范围：`internal/api/strategy/` 29 行（`pine_routes_contracts_test.go` 7、`routes_boundary_contracts_test.go` 4、
`routes_failure_boundaries_test.go` 6、`routes_lifecycle_test.go` 8、`routes_test.go` 4）。
owner：`crates/jftrade-engine` 策略定义读/写端口、策略运行时写端口与 SQLite test-cutover、活动查询解析、
插件卸载指引路由、Pine 分析端口；wire 层在 `crates/jftrade-api`。

新增/扩展 Rust 证据（本批 2 条新增用例 + 3 处扩展）：

- `product_strategy_definitions_tests::strategy_definition_detail_rejects_invalid_boolean_query_before_the_port`（新增）：
  `GET /api/v1/strategy-definitions/fixture-current?useExtendedHours=maybe` → 400 + `BAD_REQUEST` + `invalid strategy definition query`。
- `product_strategy_definitions_tests::strategy_definition_detail_maps_missing_definition_and_invalid_query`（新增）：
  缺失 definition → 404 + `NOT_FOUND`；`useExtendedHours=not-bool` → 400 + `BAD_REQUEST`。
- `product_production_ports_strategy_tests::strategy_definition_preview_derives_warmup_bars_and_overrides_preview_parameters`（扩展）：
  新增无效脚本定义断言 `derivedWarmupBars=0`、`derivedWarmupInterval=5m`、symbol 保留。
- `strategy_runtime_activity::tests::catalog_activity_paging_and_filters_match_go_boundaries`（扩展）：
  新增 `limit=bogus`/`offset=bogus` 在读取活动 store 前返回 `Invalid("invalid <kind> query")`，
  以及 Go 归一化查询串（`limit=-5&offset=-1&level=%20warn%20&fromTime=...&toTime=...` → 1/0/warn/毫秒时刻）。
- 锚点补充：`strategy_instance_read_routes_match_group_fixture_in_cutover_only`（`:142`）、
  `strategy_pine_replays_go_fixture_projection_status_and_headers` 与
  `strategy_pine_applies_input_validation_and_error_precedence_before_the_port`（`routes_test.go:181`）。

映射结果：6 行升 `[x]`（`routes_boundary_contracts_test.go:16/:142`、`routes_test.go:114/:146/:181`、
`routes_lifecycle_test.go:611`），23 行收紧 `[~]`（含缺口 owner 与回归要求）。
计数：`[x]` 1463 → 1469；audit Rust 测试 3166；anchors 1478（unrecorded 0、stale 0、unknown 54）。

关键事实与新登记缺口（P1/P2）：

1. **空 URI 参数语义差异（P1）**：Go 直接调用 handler 注入空 URI 参数，14 个处理器一律 400 `BAD_REQUEST`；
   Rust 空参路径（`/api/v1/strategies//start`、`/api/v1/strategy-definitions/`）不匹配路由模板，
   实例路由 `parse_route` 对空 `raw_id` 走 `not_found_spec` → 404 `NOT_FOUND`。owner = 策略路由层，
   需决定是否补齐 400 语义与回归用例。
2. **start 转换失败语义差异（P1）**：Go 的 `UpstreamError` → 502 `STRATEGY_RUNTIME_START_FAILED` 并停止已启动 runtime；
   Rust 生产端口把 `STARTING→RUNNING` CAS 失败映射为 409 `CONFLICT`（同时 cancel + release_demand 回滚），
   502 仅由 SQLite test-cutover 端口产出（`sqlite_test_cutover_preserves_repeated_transitions_rollback_and_restart`）。
   startability guard 与 worker 容量（`pineworker.ErrCapacityExceeded`）在 Rust 无 HTTP 级用例。
3. **Pine 分析失败映射差异（P1）**：Go 注入分析器普通错误 → 400 `PINE_ANALYSIS_FAILED`；
   Rust `map_pine_analysis_error` 将 `Remote/Transport/InvalidResponse` → 502 `STRATEGY_PINE_ANALYZE_FAILED`，
   状态码与码名均不同。owner = production pine analyze port。
4. **策略控制路由逐码断言缺口（P1）**：update/runtime-risk/pause/stop 的 catalog 失败（Go 一律 500 `STRATEGY_FAILED`）、
   delete 缺失 404、apply-linked 三条失败码、refresh 404、instantiate 业务错误 400 等未逐条 HTTP 断言；
   Rust 生产端口对部分失败采用 409/502 族，需先确认收敛语义再补断言。
5. **插件路由缺口（P2）**：空白标识符三路 400、插件 install/uninstall/operation/guidance 的 404/500 映射、
   目录与安装/卸载五条断言未复刻；现仅 guidance 路由有 fail-closed（503 `PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE`）与 fixture 对齐用例。
6. **Pine 诊断族缺口（P2）**：v20 parse-only、对象签名、import 别名、类型/方法注册表四类诊断仅以 fixture 整份投影相等表达，
   `strategy-pine.json` 未冻结对应 case（仅 `unsupported-syntax-projection`）。
7. **busy fallback 文案（P2）**：Go `writeStrategyError` 的 BusyError 空消息回退到 fallback 文案，Rust 无同形 helper 断言。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 受影响 6 条用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(strategy_definition_detail_) \| test(catalog_activity_paging_and_filters_match_go_boundaries) \| test(strategy_definition_preview_derives_warmup_bars_and_overrides_preview_parameters) \| test(strategy_instance_read_routes_match_group_fixture_in_cutover_only)'` | 6/6 通过 |
| Pine 用例族 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(strategy_pine_)'` | 10/10 通过 |
| 格式与静态 | `cargo fmt --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s5d_apply.json` | 29 行更新，`[x]` 1463 → 1469 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3166 |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1478、unrecorded 0、stale 0、unknown 54 |
| 门禁 | `pnpm run check:compatibility`（278 operations 等 5 组 replay）、`check:ai-context`、`check:zero-go`（2942 files）、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首跑 1 个已知抖动用例（chaos stress）失败，隔离复跑 5/5 通过；第二/三跑另见 lease/launcher 抖动，隔离复跑通过后最终 1841/1841 通过 |

后续（分片五 e）：`internal/api/trading` 余量（21）、`internal/api/backtest/*`（14）、`internal/api/watchlist/*`（12）、
`internal/api/live/*`（15）、`internal/api/marketdata/*`（12）、`internal/api/research/*`（2）、`internal/api/origin/*`（2）、
`internal/api/httpserver` 余量（8）、`internal/api/middleware` 余量（5）；分片五 f 收尾 `internal/api/assistant/*` 余量与 live SSE 复核。

### 分片五 e：`internal/api/trading` 余量 + `internal/api/backtest`（35 行）

范围：trading 21 行（`execution_test.go` 7、`execution_validation_contracts_test.go` 4、`execution_products_test.go` 2、
`routes_broker_contracts_test.go` 2、`openapi_route_alignment_test.go`、`routes_failure_boundaries_test.go`、
`routes_helper_boundaries_test.go`、`routes_read_handlers_test.go`、`routes_test.go` 2）与 backtest 14 行
（`routes_test.go` 5、`routes_boundaries_test.go` 5、`routes_progress_test.go` 4）。
owner：`crates/jftrade-engine` 交易执行写/读端口与执行错误映射、回测写/读端口与同步请求解析；wire 层在 `crates/jftrade-api`。

新增/扩展 Rust 证据（6 条新增用例 + 1 处扩展 + 锚点补齐）：

- `backtests_write_compatibility`：`backtest_sync_route_maps_adapter_failure_to_sync_failed`（500 `SYNC_FAILED`）、
  `backtest_start_route_maps_request_and_provider_failures`（400 `BAD_REQUEST` / 500 `BACKTEST_START_FAILED`）、
  `backtest_start_route_rejects_malformed_json_and_missing_strategy`（400 / 404 `NOT_FOUND`）、
  `backtest_delete_route_maps_run_store_failure_to_internal_server_error`（500 `BACKTEST_RUN_STORE_FAILED`）、
  `backtest_delete_route_reports_not_found_when_terminal_run_disappears`（404），并抽出 `replay_fixture_case` 辅助。
- `product_backtests_tests`：`backtest_empty_list_serializes_null_runs_and_missing_result_is_not_found`
  （空列表 `runs:null` + 缺失结果 404）。
- `product_backtest_sync_start_tests`：`sync_request_rejects_invalid_ranges_and_intervals` 扩展 `since="bad"` 分支；
  语料/投影用例补锚点（`sync_request_session_scope_parity_with_go`、`production_sync_read_projects_persisted_task`、
  `production_backtest_start_executes_fixture_and_persists_terminal_result`）。
- trading：执行写叶子语料用例补 `execution_test.go:47/:79/:148/:205`、`execution_products_test.go:18/:73`、
  `execution_validation_contracts_test.go:52` 锚点；产品写/读用例与风控协调器用例补同族锚点；
  修正 `execution_test.go:78→:79` 过时锚点（Go 行号偏移）。

映射结果：18 行升 `[x]`（backtest 11、trading 7），17 行收紧 `[~]`（含缺口 owner 与回归要求）。
计数：`[x]` 1469 → 1487；audit Rust 测试 3172；anchors 1495（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **sync 请求 symbol-only 兜底差异（P1，本批新发现）**：Go `{"symbol":"bad symbol"}` → 400 `BAD_REQUEST`；
   Rust `normalize_sync_instrument` 在 market/code 皆空时无视传入 symbol，静默回退 `HK.00700`。
   owner = `product_backtest_sync_request.rs`，须按 Go 收紧并补 400 回归。
2. **回测缺参/空白 URI（P2）**：Go 五个 handler（sync progress/cancel、status/result/delete）缺参或空白一律 400；
   Rust `cancel-blank-id` → 404、`delete-blank-id` → 400、status/result 空参不匹配模板 → 404（同策略域差异）。
3. **sync progress HTTP 双态（P2）**：progress 200 与缺失 404 无冻结语料与路由级断言（现有证据为端口级投影与取消缺失语义）。
4. **执行错误细分码（P1）**：Go `executionCommandError` 九项映射含 502 `BROKER_NOT_CONNECTED`、502 `BROKER_COMMAND_FAILED`、
   账户缺失→400；Rust `map_trade_error` 仅 timeout→504、rate→429、其余→502 `BROKER_UNAVAILABLE`。
5. **订单列表 activeOnly 拉取计数（P2）**：Go 断言 `scope=active` 仅拉当前订单（current/history=1/0，默认 2/1），
   属 order-updates worker 语义，Rust 读端口不承担拉取，无同形断言。
6. **订单详情部分成交对（P2）**：Go 断言 `status=PARTIALLY_FILLED` + `rawBrokerStatus=FILLED_PART`；
   Rust 语料仅冻结 `BROKER_ACCEPTED`/`SUBMITTED`，映射函数存在但该对未入语料。
7. **执行缺 id 400 vs 404（P2）**：events/cancel/order-details 的空 URI 参数在 Go 为 400，Rust 走模板不匹配 404。
8. **OpenAPI 路由集合等式（P2）**：Go 用 gin 注册表断言文档标识 ↔ 注册路由一一对应；
   Rust 由 `route_manifest`（routeDigest）与 `check-api-transport.mjs`（278 operations/18 groups）承担，未单列集合等式。
9. **brokers/portfolio 读边界（P2）**：portfolio 缺 broker 语义、写路由错误 broker/不支持资源 404、unlock 成功信封、
   helper 六类边界（bindBrokerURI 404、bindQuery 400、500 `BROKER_READ_FAILED`、限流 429+Retry-After、409 风控保真）、
   读端口参数归一（cash-flows/margin-ratios 缺参 400、order-fees 合并去重）均无逐项路由断言。
10. **broker 降级 200 vs 503（延续 P1）**：`routes_failure_boundaries_test.go:17` 与 `routes_test.go:32` 的
    degraded 200（`connectivity:"disconnected"`+`lastError`）在 Rust 为 fail-closed 503。
11. **brokers 写路由校验顺序（P2）**：Go 无参数 `POST /brokers/futu/orders` → 503；Rust 缺 `accountId&market` 时先 400。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 回测新增 5 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --locked -E 'test(backtest_sync_route_) \\| test(backtest_start_route_) \\| test(backtest_delete_route_)'` | 5/5 通过 |
| 回测读新增 1 条 | `... -E 'test(backtest_empty_list_serializes_null_runs_and_missing_result_is_not_found)'` | 1/1 通过 |
| 回测族整组 | `... -E 'test(backtest_) \\| test(backtests_)'` | 64/64 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b126s5e_apply.json` | 35 行更新，`[x]` 1469 → 1487（含 1 次 entry 去重修正） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3172 |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1495、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 门禁 | `pnpm run check:compatibility`（SQLite 2 tables/3 K-lines、backtest 5 cases/8 fills、278 operations 等 8 组 replay）、`check:ai-context`、`check:zero-go`（2942 files）、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1847/1847 通过 |

### 分片五 f：`internal/api/watchlist` + `internal/api/live` + `internal/api/marketdata`（39 行）

范围与 owner：`internal/api/watchlist/*`（12 行：routes_test 5、route_error_handling_test 3、routes_business_test 4）
→ `crates/jftrade-engine` 观察列表读/写端口与 `crates/jftrade-watchlist`、`crates/jftrade-store-sqlite`；
`internal/api/live/*`（15 行：handler_test 8、dispatcher_boundaries_test 7）→ `crates/jftrade-api` websocket/sse/router
与 `crates/jftrade-engine` ws-live 投影、OpenD listener；`internal/api/marketdata/*`（12 行：routes_boundaries_test 5、
routes_news_actions_test 4、routes_test 3）→ `crates/jftrade-engine` 行情读端口与新闻/公司行动 helper 路由。

行为修复（本批唯一生产改动）：观察列表读端口把查询解码切到共享严格解码器
（`product_query::decode_query_component`）。此前 `?%zz` 这类畸形转义被静默保留（未知键被忽略 → 200），
Go 的查询绑定在同一输入下返回 400 `BAD_REQUEST`；现在 items/bindings/import-runs 三路读路由返回
`WatchlistReadSnapshotError::Invalid`，由既有 wire 映射输出 400。

新增 Rust 证据（6 条新增用例 + 3 处扩展/补断言）：

- `product_watchlist_write_port`：`every_watchlist_mutation_fails_closed_without_a_port`（8 条写路由无端口 → 503
  `WATCHLIST_UNAVAILABLE`）；`malformed_membership_and_commit_bodies_are_rejected`（memberships PUT 与 commit 畸形体 → 400
  `BAD_REQUEST`）。
- `product_watchlist_tests`：读路由 fail-closed 用例补逐路径 503 状态断言；`product_tests` 的成员关系 fail-closed
  用例补 503 状态断言与 `routes_test.go:34` 锚点。
- `product_production_assembly_tests`：生产观察列表读端口补 `limit=nope` 与 `%zz`（items/bindings/import-runs）三类
  Invalid 断言。
- `product_ws_live_tests`：`ws_live_transport_accepts_trusted_origin_and_streams_heartbeat_first`（受信页面 Origin
  握手 101 + 首个 heartbeat 帧，配套 `read_server_text_frame` 帧读取辅助）。
- `product_ws_live` 投影单测：`auxiliary_provider_failure_skips_only_the_failing_subscription_family`
  （console 刷新保留、security/depth 各自失败只跳过该家族）、
  `repeated_tick_observations_are_deduplicated_per_provider`（同 (provider, instrument, observedAt) 只发一帧、
  payload source 保留、切换 provider 后重新打标且不跨 provider 去重）。
- `product_production_ports_market_data_news_tests`：`news_actions_helper_request_rejects_a_missing_instrument_uri`
  （缺 instrument URI 五类路径 → 400）；news limit 校验扩展为 `abc/0/51` 三段；corporate-actions 与 news 转发用例补
  `routes_news_actions_test.go:76/:115` 锚点。

映射结果：31 行更新（15 行升 `[x]`、16 行收紧 `[~]`），8 行（marketdata 复合读路由与 boundary 行）复核后维持原结论。
计数：`[x]` 1487 → 1502；audit Rust 测试 3178；anchors 1511（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **WS 运行时 heartbeat 缺 liveClients（P2，本批新发现）**：冻结语料与 Go 后端 heartbeat 载荷含
   `liveClients{connected,limit,atLimit}`；运行时 `live_heartbeat_payload` 只发 `liveStream/transport/stale`，
   两侧 wire 形状不一致。owner = `crates/jftrade-api/src/router.rs`。
2. **无效订阅的关闭形状（P2）**：Go 直接断开（客户端见 1006，无 close 帧），语料
   `invalid-subscription-closes-without-code-frame` 同此；运行时发送 `Close(1008, subscription policy violation)`。
   owner = `crates/jftrade-api/src/router.rs::websocket_session`，须统一运行时与语料。
3. **news/actions 错误分类（P2）**：Go 的 provider 失败 → 502 `MARKET_NEWS_FAILED`、忙碌 → 503
   `MARKET_DATA_PROVIDER_BUSY` + Retry-After；Rust 该家族按 helper 上游状态/码直通（fixture 冻结），
   只有 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED` 同码。owner = `product_production_ports_market_data_news_actions_route.rs`。
4. **缺 instrument URI 404 vs 400（P2）**：Go 的 news/corporate-actions handler 对空参数返回 400，
   Rust 模板路由不匹配 → 404（端口层已断言 400，传输层差异保留）。
5. **深度刷新合并（P1）**：Go dispatcher 以 resolvedAt 去重并只向订阅者推送新 payload；
   Rust hub 广播不合并、运行时无 resolvedAt 去重状态，仅在冻结语料断言帧序列。
   owner = `crates/jftrade-api/src/websocket.rs` + `crates/jftrade-engine` RouterDemandListener。
6. **辅助订阅写失败冒泡（P2）**：Go `writeAuxiliarySubscriptions/writeLiveData/writeNotifications` 的写失败回传错误；
   Rust 由传输 send 失败断连，session/sse 层均无逐条断言。owner = `crates/jftrade-api/src/{router,sse}.rs`。
7. **显式 broker reader 分流（边界）**：Go 的 securities/snapshots/candles/depth 显式 brokerId 走 broker reader
   调用序列；Rust 无 reader 列表（单一 active provider owner），显式非活动 broker 一律 409，记边界差异不伪装覆盖。
8. **watchlist 缺 URI/绑定失败 400（P2）**：Go 用中间件清空 params 断言 400；Rust 模板不匹配 → 404，
   组 ID 畸形走 404，无等价 400 分支。owner = `crates/jftrade-engine` 路由模板层。
9. **watchlist 完整生命周期单测（P2）**：Go 单条 HTTP 用例串联 groups/sources/import/bindings/import-runs/memberships/
   quotes/删除；Rust 证据分散在 fixture（45 用例）、sqlite cutover 与浏览器边界用例，尚无同序端到端用例。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| watchlist 读/写 fail-closed | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib -E 'test(watchlist_read_routes_fail_closed_when_snapshot_port_is_unavailable) + test(watchlist_memberships_route_fails_closed_when_snapshot_port_is_unavailable) + test(every_watchlist_mutation_fails_closed_without_a_port)'` | 3/3 通过 |
| watchlist 严格解码 | `... --lib -E 'test(production_watchlist_read_uses_real_pages_and_remote_catalog)'` | 1/1 通过 |
| watchlist 写语料 | `... --test watchlist_write_compatibility` | 4/4 通过 |
| ws-live 握手与投影 | `... --lib -E 'test(ws_live_transport_accepts_trusted_origin_and_streams_heartbeat_first)'`；`... --test ws_live_compatibility` | 1/1 与 5/5 通过 |
| marketdata news/actions | `... --lib -E 'test(production_news_actions) + test(corporate_actions_query_requires_rfc3339_and_ascending_range) + test(news_actions_helper_request_rejects_a_missing_instrument_uri)'` | 6/6 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s5f_payload.json` | 32 行更新，`[x]` 1487 → 1502 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3178 |
| 锚点 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1511、unrecorded 0、stale 0、unknown 53 |

后续（分片五 g）：`internal/api/research/*`（2）、`internal/api/origin/*`（2）、`internal/api/httpserver` 余量（8）、
`internal/api/middleware` 余量（5）、`internal/api/assistant/*` 余量（8），并复核 live SSE 与 assistant 审批流。

### 分片五 g：`internal/api/research` + `internal/api/origin` + `internal/api/httpserver` + `internal/api/middleware` + `internal/api/assistant`（25 行）

范围与 owner：`internal/api/research/routes_test.go`（2 行）→ `crates/jftrade-engine/tests/research_presets_write_compatibility.rs`；
`internal/api/origin/origin_test.go`（2 行）→ `crates/jftrade-api/src/auth.rs` 与 CORS 传输用例；
`internal/api/httpserver/bindings*_test.go` 余量（8 行）→ `crates/jftrade-engine/src/product_query.rs`、
`product_production_ports_market_data_quote*.rs`、`product_market_data_*_tests.rs`、`crates/jftrade-api/tests/transport_contracts.rs`；
`internal/api/middleware/adk_test.go`、`auth_test.go`、`security_boundaries_test.go` 余量（5 行）→ 引擎 ADK 读/流端口与 transport；
`internal/api/assistant/*` 余量（8 行）→ `crates/jftrade-engine/src/product_adk_chat_stream*`、`product_production_ports_adk_tests.rs`、
`crates/jftrade-api/src/sse.rs`。本分片 25 行全部给出终值：8 行升 `[x]`（research 2、origin 1、httpserver 3、middleware 2）、
1 行转 boundary（origin `TestFromRequest`）、其余收紧为带 owner 与回归要求的 partial/boundary。

关键事实与生产修复：

1. **Go 有两个近似同名的来源归一化 helper（本批新查清）**：`internal/api/origin/origin.go::FromRequest` 在 Origin 非法时回退 Referer，
   但它在 go 分支没有任何生产调用方——生产路径是 `internal/api/middleware/cors.go::requestOrigin` 与
   `internal/api/live/handler.go` 的 `Canonical(Origin)`，二者对非法 Origin 直接拒绝。Rust `request_origin` 实现的是生产语义，
   因此保留“Origin 存在但非法 → None”，仅补盲测断言与锚点；照搬 `FromRequest` 会让 `Origin: null` + 允许 Referer 通过
   `cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin`（Go 同断言为 403）。
2. **可选布尔别名表（P1，修复）**：Go `OptionalBoolValue` 接受 0/false/no/n/off/空 → false，1/true/yes/y/on → true，其余 400。
   Rust 快照路由此前只接受 `true/1/false/0`，`refresh=off|no|yes|on|` 会 400。新增 `parse_optional_query_bool` 并让
   `refresh` 走该 helper；空值与别名在快照路由逐项断言（假值保持缓存、真值强制 provider）。
3. **空白 period（P2，修复）**：Go `CandlePeriodValue` 空白视为未设置，蜡烛路由保持默认 1m；Rust 此前对
   `period=`/`period=%20`/`period=+` 返回 400。修 `product_production_ports_market_data_quote_reads.rs::read_candles` 为
   trim 后空值即未设置，`2h` 仍 400。
4. **空白 limit（P2，修复）**：Go 经 `OptionalIntValue`（空文本是合法 0）+ 服务端 `limit <= 0 → 200`，
   所以 `limit=`/`limit=%20` 返回默认窗口；Rust 此前 400。现同样把 trim 后空值当未设置，非整数仍 400。
5. **ADK 可用性保证是两段式（P2）**：Go 中间件 `ADKAvailable` 在 runtime 缺失时对每个 ADK 路由回 503 `ADK_UNAVAILABLE`；
   Rust 未装配端口即未注册路由（404），已装配但不可用才 503（读路由码为 `ADK_READ_UNAVAILABLE`，chat 为 `ADK_UNAVAILABLE`），
   由 `wired_but_unavailable_adk_ports_fail_closed_on_every_route` 固定，本批登记为 partial 而非等价。
6. **重连写失败无同形对象（P2）**：Go 断言重连时 retry 指令写失败只写一次并立即退出；Rust SSE 响应体在引擎侧一次性物化，
   写失败传播由 `crates/jftrade-api/src/sse.rs` 的写错误用例断言，属传输层边界，登记 partial。

新增与修改的 Rust 证据：

- `crates/jftrade-engine/src/product_query.rs::optional_query_bool_matches_the_reference_alias_table`
  （新增 `parse_optional_query_bool` + `QueryBoolError`，覆盖真/假别名表与非法值 400）。
- `crates/jftrade-engine/src/product_market_data_candle_pagination_tests.rs::candle_route_treats_blank_period_as_unset_and_rejects_unsupported`、
  `candle_route_treats_blank_limit_as_unset_and_rejects_non_integer`（本批新增，含 `limit=%20`）。
- `crates/jftrade-engine/src/product_market_data_quote_read_tests.rs::snapshot_route_force_refresh_bypasses_the_cache`
  （扩展为假值别名保持缓存、真值别名强制 provider）。
- `crates/jftrade-api/src/auth.rs::origin_normalization_accepts_web_and_tauri_schemes`（补 Go 表格：空白、端口、缺失 host、ftp、://bad）
  与 `request_origin_uses_production_semantics_without_malformed_origin_fallback`（Origin 优先、缺失回退 Referer、非法不回退）。
- 补锚点：`internal/api/research/routes_test.go:18/:74`、`internal/api/origin/origin_test.go:8/:31`、
  `internal/api/middleware/security_boundaries_test.go:90/:108`、`internal/api/middleware/adk_test.go:30`、
  `internal/api/httpserver/bindings_test.go:224`。

探针证据（先红后绿，按字节回滚）：

- 回退空白 period 修复：`candle_route_treats_blank_period_as_unset_and_rejects_unsupported` 0/1 失败（`period=` → 400），恢复后 1/1 通过。
- 回退空白 limit 修复：`candle_route_treats_blank_limit_as_unset_and_rejects_non_integer` 0/1 失败
  （`query limit= must use the default limit: Failed { status: 400 … }`），恢复后 1/1 通过；
  `product_production_ports_market_data_quote_reads.rs` shasum `96c4e6d906a7c2258dec44e57c8e0024f5b46008` 前后一致。
- 回退 refresh 别名修复：`snapshot_route_force_refresh_bypasses_the_cache` 与 `snapshot_rejects_malformed_refresh_before_provider_access`
  0/2 失败；恢复后 4 条引擎用例 4/4 通过，`product_production_ports_market_data_quote_snapshot_reads.rs` shasum
  `4b9e1d5d01ca92a30f9d26c518eb40921fd262ca` 前后一致。

映射结果：13 行写入（8 行升 `[x]`、5 行收紧 `[~]`），另修正 1 行过时引用（`internal/api/middleware/security_boundaries_test.go:90`
由改名前的 `request_origin_preserves_origin_precedence_and_referer_fallback_boundary` 更新为新名），并解决 1 处 `[x]` 引用重复
（`adk_test.go:30` 改用两条 assembly 用例组合引用）。计数：`[x]` 1502 → 1510；audit Rust 测试 3181；
anchors 1521（unrecorded 0、stale 0、unknown 53）。

关键缺口与新登记项（P1/P2）：

1. **ADK 读路由错误码差异（P2）**：Go 中间件对 runtime 缺失统一 `ADK_UNAVAILABLE`；Rust 读路由为 `ADK_READ_UNAVAILABLE`
   （chat 路由一致）。owner = `crates/jftrade-engine/src/product_adk_read_api.rs`。
2. **404 文案未统一（P2）**：Go `WriteNotFound` 固定 `resource not found`；Rust 未知路由为 `unknown endpoint <path>`，
   其余路由自有消息。owner = `crates/jftrade-api/src/{envelope,router}.rs`。
3. **分页 helper 无同形对象（边界）**：`NormalizeBoundPage(limit, offset, default, max)` 未迁移，各路由自行 clamp；
   Go 的 `limit=-3 → 1`、负 offset → 0 在 Rust usize/校验入参下不可达。
4. **绑定 helper 结构边界（P2）**：`BindURI` 的 required 缺参、`requestEscapedPath` 的 RawPath 兜底在 Rust 无同形对象
   （模板不匹配 → 404；路径转义由传输层处理），登记 boundary 并保留升级路径。
5. **重连写失败单次重试（P2）**：见关键事实 6，若未来改为流式写回需补一次写失败即退出的传输用例。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 本批引擎 6 条 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(candle_route_treats_blank_limit_as_unset_and_rejects_non_integer) or test(candle_route_treats_blank_period_as_unset_and_rejects_unsupported) or test(candle_route_rejects_invalid_limit) or test(snapshot_route_force_refresh_bypasses_the_cache) or test(optional_query_bool_matches_the_reference_alias_table) or test(snapshot_rejects_malformed_refresh)'` | 6/6 通过 |
| 探针（limit） | 回退生产修复后同用例 | 0/1 失败（先红），恢复后 1/1 通过 |
| origin 表格与回退 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked -E 'test(request_origin) or test(origin_normalization) or test(cors_preflight_reflects_allowed_origin_and_rejects_unknown_origin)'` | 3/3 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s5g_payload.json` 与 3 次单行修正 | 16 行更新，`[x]` 1502 → 1510 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3181 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1521、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked --no-fail-fast`；`... -p jftrade-engine ...` | 81/81、1858/1858 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`（9 statuses/12 transitions、278 operations/18 groups、3 profiles 等 8 组）、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本次改动无关 |

后续（分片五 h 与第 127 批）：先做 `internal/api/research`→`httpserver` 之外的 api_transport 余量
（`internal/app/apiserver` 待办 331；随后 strategy_pine 465 → assistant_workflow 447 → other 311 →
backtest_calendar 262 → storage_sqlite 178 → marketdata_quotes 155 → futu_opend 104 → settings_watchlist 39），
按同一批次节奏推进：单分片一次提交，提交后立即把下一分片写入代办 codex 目标。

## 第 127 批

### 分片一：`internal/app/apiserver/servercore` 前 34 行

范围与 owner：`internal/app/apiserver/servercore` 按文件与行号排序的前 34 条待办（数据管理 9、桌面 token 2、
assistant 传输生命周期 1、broker 读查询 1、执行回写 1、instrument 归一 1、live WS 3、行情 3、通知与工作流 5、
OpenAPI 1、产品基础设施 2、产品生命周期 3、其他 2）。owner 分别落在 `crates/jftrade-store-sqlite`（维护/清理、
回测运行与执行账本）、`crates/jftrade-datamanagement`（预览/执行纯函数与 busy reason 入口）、
`crates/jftrade-engine`（装配、ADK/WS/行情端口、执行对账与写回）与 `crates/jftrade-api`（传输、heartbeat、OpenAPI 契约）。

本分片新增 Rust 证据 2 条：

- `crates/jftrade-store-sqlite/tests/maintenance_overview_and_cleanup_contracts.rs::compaction_rejects_a_directory_where_the_backtest_database_belongs`
  （Go `data_management_failure_boundaries_test.go:178`）：backtest 库路径为目录时 compact 必须报错，且目录与内容不被改写。
- `crates/jftrade-store-sqlite/tests/maintenance_overview_and_cleanup_contracts.rs::backtest_history_cleanup_skips_running_runs_and_keeps_the_newest_terminal_run`
  （Go `data_management_test.go:17`）：running 运行永不进入候选，keepLatest 保护最新终态运行，批准清理只删除旧终态运行。
- 补锚点：`market_realtime_test.go:18` 指向 `candle_route_keeps_latest_history_after_all_forward_pages_and_current_bar`
  （历史分页 + Qot_GetKL 当前桶合并、closed=false、分页游标），该行由 partial 升 `[x]`。

映射结果：11 行写入（2 行升 `[x]`：`data_management_failure_boundaries_test.go:178`、`market_realtime_test.go:18`；
9 行收紧或纠正为 partial/boundary）。经复核，其余 23 行维持既有 partial/boundary 终值（每条含差异说明、owner 与回归要求）。
计数：`[x]` 1510 → 1512；audit Rust 测试 3183；anchors 1524（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **逐库 busy 注册表缺失（P1，延续）**：Go 的维护注册表内置行情同步、策略运行、非终态执行订单与运行中 ADK run 的 busy
   reason（含中文文案）；Rust `preview_cleanup/verify_execute` 只接受调用方传入的 reason，engine 侧没有探测与装配，
   因此 `data_management_failure_boundaries_test.go:61`、`adk_data_management_test.go:60`、`data_management_test.go:17`
   的 busy 半仍为 partial。owner：`crates/jftrade-engine` 数据管理装配。
2. **交易读路径默认市场（P2）**：Go `tradingSvc.ReadQuery` 用集成配置 `TradeMarket` 填充空 market；
   Rust 市场来自请求或账户授权，设置侧 `trade_market` 默认 HK。owner：`product_production_ports_trade_requests.rs`。
3. **下单回写复用已发现订单（P2）**：Go 断言 RecordPlacedOrder 复用券商已发现订单的内部 id；
   Rust 对账发现已按券商订单号去重/复用，但缺“下单回写命中已发现订单”的专门断言。owner：`execution_reconciliation_discovery.rs`。
4. **runtime heartbeat 字段（P2，延续 5f）**：Go 的 live heartbeat 在 legacy futu 选择下仍给 `marketDataProviderId`、
   `sampleFreshnessMs`、`staleReasons`；Rust `live_heartbeat_payload` 只有 `providerBrokerId` 与 transport mode。
   owner：`crates/jftrade-api/src/router.rs`。
5. **OpenAPI 与注册路由集合等式（P2）**：Go 用 gin 注册表断言文档覆盖每个注册路由；Rust 由 `check-api-transport`
   （278 operations/18 组 + 路由探针）与 `check:generated` 承担，缺 Rust 内集合等式。owner：`product_runtime.rs` 路由清单 + 兼容门禁。
6. **nil/已关闭句柄边界（边界）**：Go 的 nil store、已关闭 store、nil backend、nil request 系列（数据管理边界 4 行、
   live WS 1 行、通知 panic 2 行、产品基础设施 2 行、快照身份 1 行）在 Rust 的所有权/类型化端口下不可表达，
   统一登记 boundary 并保留升级路径（若引入可空句柄，需补对应 fail-closed 断言）。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增维护用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --test maintenance_overview_and_cleanup_contracts --locked` | 13/13 通过（含 2 条新增） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127a_payload.json` | 11 行更新，`[x]` 1510 → 1512 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3183 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1524、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`；`... -p jftrade-engine ...` | 188/188、1858/1858 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`（SQLite 2 表 3 K 线、backtest 5 用例 8 成交、provider 14+9+3、trading 10/7/6/7/5/3、assistant 9/12、api-transport 278 operations/18 组、desktop 3 profiles）、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`check:quick` | 全部通过 |
| 目标体积 | `target/debug/deps` 的 `.rcgu.o` 超 5 万触发 `check:rust:target-health` | 确认无 Cargo 进程后执行 `pnpm run clean:rust:artifacts`（`cargo clean` 释放 28.1GiB / 106448 文件），复跑 `check:quick` 通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本次改动无关 |

后续（分片二）：`internal/app/apiserver/servercore` 余 68 行，随后 servercoretest 52、marketdataapp 68、
webaccess 24 + tradingapp 11、backtestapp 10 + datamigration 11 与 runtime/lifecycle/application/futuapp 余量。

### 分片二：`internal/app/apiserver/servercore` 第 35–68 行

范围与 owner：按文件与行号排序的第 35–68 条（运行时集成边界 5、运行观测 2、策略运行时 4、策略实盘交易 6、
安全设置 1、服务器生命周期 3、回测路由 2、启动边界 3、降级启动 3、服务器业务辅助 5）。owner 集中在
`crates/jftrade-engine`（RuntimeComposition、ActiveProviderState、strategy runtime 及其执行端口、产品装配与关闭序列、
回测启动解析）与 `crates/jftrade-api`（Web 流与安全变更）、`crates/jftrade-strategy`（Pine 元数据）。

本分片新增 Rust 证据 2 条（`crates/jftrade-engine/src/strategy_runtime_execution_tests.rs`）：

- `test_execute_strategy_intents_prefers_explicit_quantity_over_quantity_pct`（Go `runtime_trading_test.go:239`）：
  quantity=20 与 quantityPct=50 同时存在时按显式 20 股下单（50% × 30000 / 150 会算出 100 股），reduceOnly=false。
- `test_execute_strategy_intents_sizes_close_quantity_pct_from_position`（Go `runtime_trading_test.go:286`）：
  close+quantityPct=50 在 20 股持仓上产生 10 股 reduce-only SELL。

映射结果：4 行写入（2 行升 `[x]`：`runtime_trading_test.go:239`、`:286`；2 行收紧 partial：
`server_backtest_test.go:19` 补引用已存在的 market/code 与 DST/时区用例并列出剩余缺口、`server_backtest_test.go:97`
记录 Pine initial_capital 元数据已解析但未回填请求缺省）。其余 30 行复核后维持既有 partial/boundary 终值。
计数：`[x]` 1512 → 1514；audit Rust 测试 3185；anchors 1526（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **Pine initial_capital 未回填回测请求（P2）**：Go 在请求省略 initialBalance 时用脚本 `initial_capital=250000`
   作为运行资金；Rust 已解析该元数据（`jftrade-strategy`）但回测启动路径没有回填。owner：`crates/jftrade-engine`
   回测启动 + definition 元数据透传。
2. **回测请求落库断言缺口（P2）**：Go 的路由用例同时断言 `useExtendedHours`（Rust 由 `session_scope=extended` 推导）、
   `definitionVersion` 快照、`initialBalance` 与 `startDate/endDate` 落库；Rust 已有 market/code 归一与 DST/时区用例，
   上述四项尚无逐条断言。owner：`product_production_ports_backtest_strategy.rs` / `_task.rs`。
3. **实例级 worker 上限门（P2，延续）**：Go 在每次实例启动时检查 `instanceWorkerLimit` 并拒绝；Rust 在组合期按配置
   固定 Pine worker 数量并拒绝无 failover 的多 worker 配置，无运行时逐实例门。owner：`strategy_runtime` 启动路径。
4. **nil runtime / 可空 Server 边界（边界，延续）**：`SetWebAccessReconfigure`、`SetAPIPort`、`ConfigureAuthOrigins`、
   `SetFrontendFS`、`ApplySecuritySettings`、nil runtime limits/MCP 状态、`ExchangeOrError` 等 7 行属 Wails 时代可空
   Server 形态，Rust 由不可空服务与 typed Option 端口 + 冻结兼容语料表达，统一登记 boundary 并保留升级路径。
5. **策略实盘券商刷新语义（边界）**：Go 在 K 线收口前主动刷新券商持仓、断连时保留缓存持仓继续使用；Rust 使用调用方
   提供的 current_position/sellable_quantity，断连即 fail-closed 拒绝下单（更严格），登记 boundary。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增下单定量用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(test_execute_strategy_intents_prefers_explicit_quantity_over_quantity_pct) + test(test_execute_strategy_intents_sizes_close_quantity_pct_from_position)'` | 6/6 通过（同一模块在 3 个测试目标中各编译一次） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127b_payload.json` + 1 次单行修正 | 5 行更新，`[x]` 1512 → 1514 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；rust 测试 3185 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1526、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1864/1864 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`（8 组 replay 全部通过）、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`check:quick`（含 pineworker 98/98） | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本分片改动无关 |

后续（分片三）：`internal/app/apiserver/servercore` 余 32 行，随后 servercoretest 52、marketdataapp 68、
webaccess 24 + tradingapp 11、backtestapp 10 + datamigration 11 与 runtime/lifecycle/application/futuapp 余量。

### 分片三：`internal/app/apiserver/servercore` 第 69–98 行与补审 2 行（共 32 条）

范围与 owner：按文件与行号排序的第 69–98 条（日历边界 1、策略生命周期 2、系统与设置 option 回调 2、
运行期副作用 3、服务器核心 4、回测 warmup 1、行情设置 1、策略运行时依赖边界 3、实盘语义 1、nil 边界 1、
工作流回放 2、订阅生命周期 1、启动对账 1、撤单契约 2、自选股边界 3、WS 事件 2），并补审被分片二游标跳过的
`server_business_test.go:292/:329`（sidecar runtime-config 与 broker 执行不复用策略行情 override）。owner 集中在
`crates/jftrade-engine`（product_api 日历路由、strategy runtime 与 owner_tests、market-data subscription/quote lease、
execution risk coordinator、runtime resources 与设置写入）、`crates/jftrade-api`（WS 心跳、浏览器访问）、
`crates/jftrade-calendar`（manager 生命周期）与 `apps/desktop/src-tauri`（desktop_runtime_config）。

本分片新增 Rust 证据 2 项：

- `crates/jftrade-engine/src/strategy_runtime_owner_tests.rs::strategy_runtime_holds_exact_kline_demand_until_stop_and_shutdown`
  （Go `strategy_subscription_lifecycle_test.go:12`）：两个标的（US.AAPL/HK.00700、5m）的精确 KLINE managed 租约随运行
  持有、Stop（cancel+release）归零、重启后再次持有、`shutdown()` 全部释放；web-only 清空保留策略租约的半边由
  `clear_route_preserves_running_strategy_lease` 断言，两侧同源租约注册表（Go 订阅快照 = Rust demand book）。
- `crates/jftrade-engine/src/product_strategy_definition_write_port.rs::tests::instantiate_accepts_empty_body_but_rejects_malformed_json`
  补 Go 锚点（`server_lifecycle_test.go:222`）：畸形绑定 JSON 在 instantiate 路由返回 400 BAD_REQUEST，与 Go 的
  400 断言等价，同时保持空 body 接受为 `{}`。

映射结果：32 行写入（2 行升 `[x]`：`server_lifecycle_test.go:222`、`strategy_subscription_lifecycle_test.go:12`；
30 行复核并收紧为终值）。修正 4 处陈旧或含糊引用：execution risk 用例实际位于
`product_execution_risk_coordinator_tests.rs`、quote snapshot 用例位于 `*_tests.rs`、租约帮助函数位于
`product_production_ports_market_data_quote_lease.rs`、startup reconcile 测试名已替换为
`startup_reconcile_resets_stale_paused_state_and_keeps_stopped_instances`。重写 8 行结论（日历空 registry 与取消域、
Pine 实例参数、设置副作用、启动对账、WS 心跳 stale 语义、sidecar runtime-config 载体等）。
计数：`[x]` 1514 → 1516；audit Rust 测试 3186；anchors 1528（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **Pine 实例化参数缺口（P2）**：Go 的实例视图 params 携带 interval/executionMode/brokerAccount/instruments 与
   compiledRequirements（requiresTotalAccountValue、indicators）；Rust 实例视图 params 只含 definitionId/runtime/sourceFormat，
   且没有“实例化产出编译计划”与 start/pause/stop 迁移联动断言。owner：`product_strategy_definition_write_port` /
   `strategy_runtime_port`。
2. **设置副作用未接线（P2）**：Go 的 settingsSideEffects 更新 seenFillRetentionDays、启停 Web 鉴权与
   runtime-config.js authRequired、禁用 Pine worker 时关闭长驻 runner 并拒绝策略启动；Rust 的
   `seen_fill_retention_days` 只在设置层归一化、无 engine 消费方；Pine worker 运行时来自桌面进程环境
   （设置文件只贡献 node_binary_path）；sidecar 不产出 runtime-config.js（由 Tauri `desktop_runtime_config` 与
   前端 runtimeConfig 模块承担）。owner：`crates/jftrade-engine` 设置写入与运行时装配。
3. **WS 心跳 stale 语义差异（P2）**：Go 在订阅行情 tick 超过 liveHeartbeatStaleThreshold 时置 payload.stale=true
   （provider 仍连接）；Rust `live_heartbeat_payload` 的 stale 取 provider 连接状态（staleReasons=[provider_unavailable]），
   行情新鲜度只出现在系统状态投影。owner：`crates/jftrade-api` router.rs live_heartbeat。
4. **结构性边界（保留）**：日历空 registry 在 Go 返回 200+accepted=false、Rust 503 fail-closed；日历操作上下文在
   Rust 为同步调用（无独立取消域对象）；启动对账 Rust 为“启动即恢复”（RUNNING 仍运行）而 Go 一律重置 STOPPED；
   `brokerExecutionExchangeFor`、`Ensure()`、nil option 回调等可空工厂在 Rust 由未装配端口/组合期构建替代。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增租约用例与锚点用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(strategy_runtime_holds_exact_kline_demand_until_stop_and_shutdown) \| test(instantiate_accepts_empty_body_but_rejects_malformed_json)'` | 通过（租约用例在两个测试目标各跑一次） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127c_payload.json` 与 `/tmp/s127c_payload_extra.json` | 32 行更新，`[x]` 1514 → 1516 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3186 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1528、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1866/1866 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`（SQLite/backtest/provider/trading/assistant/api-transport 278 operations/18 组/desktop 3 profiles）、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本分片改动无关 |

后续（分片四）：`internal/app/apiserver/servercoretest` 52 行（可拆 26+26），随后 marketdataapp 68、
webaccess 24 + tradingapp 11、backtestapp 10 + datamigration 11 与 runtime/lifecycle/runtimes/application/futuapp 余量。

补充说明：`internal/app/apiserver/servercore` 的 98 条待办在本分片后全部给出终值；后续复核不再按该目录推进。

### 分片四：`internal/app/apiserver/servercoretest` 前 26 行

范围（按文件与行号升序）：`backtest_provider_runtime_test.go:55/:119/:162`；`backtest_runs_test.go:34/:279`；
`broker_new_test.go:100/:126/:155/:181/:205/:237/:264/:286/:302/:319/:336/:359/:422/:462`；
`broker_read_test.go:72`；`broker_routes_test.go:14`；`exec_routes_test.go:18`；`execution_routes_test.go:53/:110/:155`；
`frontend_test.go:96`。owner 集中在 `crates/jftrade-engine`（回测同步请求校验与启动、运行恢复与结果回写、broker 读端口
装配、生产路由注册表、执行订单投影）、`crates/jftrade-backtest`（冷启动恢复）与 `crates/jftrade-api`（SPA 回退与
桌面开发代理边界）。

本分片新增 Rust 证据 3 项，其中 1 项带生产修复：

- `crates/jftrade-engine/src/product_backtest_sync_start_tests.rs::akshare_sync_rejects_history_beyond_the_intraday_lookback_window`
  （Go `backtest_provider_runtime_test.go:55`）：AKShare 1 分钟只保留近 5 个交易日、美股 5m/15m/30m/1h 只保留近 5 日，
  超出窗口的同步请求返回 400 BAD_REQUEST。配套生产修复在 `product_backtest_sync_request.rs` 新增
  `validate_sync_lookback_window`，并在 `product_production_ports_backtest_sync.rs::start_sync_task` 解析出 provider 运行时
  之后调用——窗口校验必须晚于 provider 可用性判定，否则不可用 provider 的 503 会被 400 覆盖（首轮探针即暴露该顺序，
  已按参照实现的历史源校验时机修正并回归）。
- `crates/jftrade-engine/src/product_data_management_batch_atomic_startup_tests.rs::startup_creates_missing_nested_backtest_database_directory`
  （Go `backtest_runs_test.go:279`）：`JFTRADE_BACKTEST_DB` 指向的多级缺失目录在启动时被创建并完成迁移，回测路由随即可用。
- `crates/jftrade-engine/src/product_production_assembly_tests.rs::production_http_broker_routes_reject_incomplete_paths`
  （Go `broker_new_test.go:462`）：生产路由注册表下 `/broker/funds`、`/broker/quote`、`/broker/klines`、`/broker/securities`
  的不完整路径变体全部返回 404 NOT_FOUND，与参照实现的精确路由匹配等价。

映射结果：26 行写入（3 行升 `[x]`：`backtest_provider_runtime_test.go:55`、`backtest_runs_test.go:279`、
`broker_new_test.go:462`；23 行复核并收紧为终值）。修正 1 处陈旧引用：`broker_new_test.go:126` 的
`candle_read_without_a_kline_lease_fails_before_realtime_provider_access` 实际位于
`crates/jftrade-engine/src/product_market_data_quote_read_tests.rs`。计数：`[x]` 1516 → 1519；audit Rust 测试 3189；
anchors 1531（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P2）：

1. **AKShare/US 窗口守卫只覆盖同步启动路径**：参照实现对 historical source 统一校验，Rust 的回测 start 走缓存命中路径时
   不校验 provider 窗口。owner：`product_backtest_start_request` 系列与 `product_production_ports_backtest_sync.rs`。
2. **孤儿运行恢复文案与路由级断言缺口**：`recover_orphaned_runs` 只把非终态运行置 failed 并清空 result_json，没有
   参照实现的 recovered 文案，也没有路由级重启恢复断言。owner：`product_production_ports_execution.rs`。
3. **broker 读断开信封差异（P1 延续）**：参照实现断连时返回 200 降级信封，Rust 在缺失 trade 读或行情路由时返回
   503 BROKER_READ_UNAVAILABLE（funds/quote/klines/securities 四条边界）；quote 无租约时参照实现为 200 降级 +
   lastError 说明订阅缺失。owner：`crates/jftrade-engine` broker 读端口装配与 `crates/jftrade-api` 错误映射。
4. **broker 写失败与非法 payload 缺少生产 HTTP 断言**：unlock 断连 502 UNLOCK_FAILED、unlock 类型错误 400、
   place 无 broker 502、place 非法 payload 400、cancel 无 broker 502、cancel 畸形 payload 400；现有证据停在端口层
   （`product_production_ports_execution_preview_tests.rs`）。
5. **broker 路由 JSON content-type 未逐条断言**（`broker_new_test.go:422`）；方法/路径精确性已由路由注册表与未知 API
   JSON 用例覆盖。
6. **执行订单投影与同步 worker 断言缺口**：九端点串联（`broker_read_test.go:72`）与同步 worker 状态
   （`execution_routes_test.go:155`）无单条端到端断言；按 tradingEnvironment/scope 过滤（`:53`）与默认环境（`:110`）
   的投影断言缺失。

结构性边界（保留）：`backtest_provider_runtime_test.go:119/:162` 属真实行情源 live workflow；`frontend_test.go:96` 的
开发代理属 Tauri 桌面边界，API 层只承诺 SPA 回退（`transport_contracts.rs::desktop_unknown_client_routes_use_spa_fallback_without_file_paths`）。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增与关联用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(akshare_sync_rejects_history_beyond_the_intraday_lookback_window) \| test(startup_creates_missing_nested_backtest_database_directory) \| test(production_http_broker_routes_reject_incomplete_paths) \| test(production_backtest_sync_endpoints_project_missing_tasks_and_unavailable_start)'` | 4/4 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127d_payload.json` 与 `/tmp/s127d_fix.json` | 26 行更新，`[x]` 1516 → 1519 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3189 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1531、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过（架构门禁曾因本分片把 `product_production_ports_backtest_sync.rs` 推到 802 行失败，压缩注释后 799 行通过） |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1869/1869 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过（quick 首轮因 .rcgu.o 达 67156 触发 target-health，清理 33.1GiB 后复跑通过） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本分片改动无关 |

后续（分片五）：`internal/app/apiserver/servercoretest` 余 23 行（`frontend_test.go:137/:157/:198/:246`、
`installers_degraded_test.go:13`、`market_profiles_test.go:79`、`openapi_snapshot_test.go:20/:89/:136/:204`、
`plugin_lifecycle_test.go:16`、`runtime_defaults_test.go:11/:28/:48`、`server_business_public_test.go:15/:48`、
`server_definitions_test.go:17`、`settings_broker_test.go:162`、`settings_interfaces_test.go:14`、
`strategy_logs_test.go:15/:120/:169`、`strategy_preview_test.go:28`、`strategy_sync_test.go:17`、
`system_routes_test.go:112`、`watchlist_runtime_test.go:51`），随后 marketdataapp 68、webaccess 24 + tradingapp 11、
backtestapp 10 + datamigration 11 与 runtime/lifecycle/runtimes/application/futuapp 余量。

### 分片五：`internal/app/apiserver/servercoretest` 余 26 行

范围（按文件与行号升序）：`frontend_test.go:137/:157/:198/:246`；`installers_degraded_test.go:13`；
`market_profiles_test.go:79`；`openapi_snapshot_test.go:20/:89/:136/:204`；`plugin_lifecycle_test.go:16`；
`runtime_defaults_test.go:11/:28/:48`；`server_business_public_test.go:15/:48`；`server_definitions_test.go:17`；
`settings_broker_test.go:162`；`settings_interfaces_test.go:14`；`strategy_logs_test.go:15/:120/:169`；
`strategy_preview_test.go:28`；`strategy_sync_test.go:17`；`system_routes_test.go:112`；`watchlist_runtime_test.go:51`。
owner 集中在 `crates/jftrade-engine`（启动配置与运行时布局、策略定义读写叶与 pine 谓词、插件投影、系统状态投影）、
`crates/jftrade-api`（SPA 回退与 Swagger/OpenAPI 契约）、`crates/jftrade-store-settings-file`（启动引导文档）与
`apps/desktop/src-tauri`（桌面 profile 与运行期布局）。

本分片新增 Rust 证据 2 项（含 1 项生产修复），并把 4 条 OpenAPI 契约行从 module_only 升级为等价覆盖：

- `crates/jftrade-engine/src/product_production_ports_strategy_tests.rs::instantiate_rejects_stored_definitions_with_a_retired_source_format`
  （Go `strategy_preview_test.go:28`）：已存 legacy-v0 定义实例化返回 400 BAD_REQUEST 且文案含
  unsupported legacy strategy definition；生产修复在 `strategy_pine.rs` 新增 `instantiation_source_format_error`
  （空白与 pine-v6 放行），由生产策略写入端口在读到已存定义之后、绑定校验与实例播种之前调用。先红探针确认修复前该路径
  会正常播种实例，修复后用例转绿；谓词边界（空白、大小写归一、文案形状）由
  `crates/jftrade-engine/tests/strategy_pine_compatibility.rs::strategy_pine_reports_retired_source_formats_for_instantiation`
  覆盖。
- `crates/jftrade-api/tests/swagger_docs_contracts.rs` 的 4 条既有用例（`served_swagger_document_matches_the_checked_in_contract`、
  `swagger_document_requires_explicit_error_envelopes_for_every_operation`、`swagger_document_publishes_typed_writable_request_bodies`、
  `swagger_document_publishes_the_typed_broker_runtime_response`）此前已带 Parity 锚点，但账本仍记为契约+脚本的 module_only；
  本分片逐条核对后确认其断言语义与 Go 等价（逐字节契约、278 操作数、每个操作显式错误信封、六条写路由的 typed body
  与托管字段隔离、broker runtime 的 data $ref 与三键定义），升级为 function_exact。

映射结果：26 行复核（5 行升 `[x]`；2 行收紧为 boundary：前端资源加载辅助、API-only 取消语义；6 行收紧 partial 终值；
13 行复核后确认原终值仍准确）。计数：`[x]` 1519 → 1524；audit Rust 测试 3189；anchors 1532
（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **API 绑定不消费存储的 interfaces.apiBind（P1）**：Go 断言 Web 关闭时仍以接口设置绑定 API；Rust 启动路径只从接口设置
   消费 liveWebSocketConnectionLimit（`product_server.rs`），监听地址来自 ProductConfig 与桌面 profile（开发 3000、桌面 dev
   3008、release 6699），存储 apiBind 在启动路径无消费者。owner：`crates/jftrade-engine` 启动配置装配。
2. **缺少启动引导文档物化（P2）**：Go 的 EnsureBootstrapFile 首启即落盘 interfaces+appearance 默认且不写 integration；
   Rust 无等价动作，默认值在读取时归一，仅在首次 broker 集成保存时物化 interfaces{apiBind, liveWebSocketConnectionLimit}，
   不含 guiBind 与 appearance。owner：`crates/jftrade-store-settings-file/src/lib.rs`。
3. **OpenD 中性信封冲突（P2）**：Rust 未启用集成时健康投影为 unavailable+reason（锚定 internal/system/service_test.go:345 与
   futuapp/runtime_contracts_test.go:44），而 servercoretest 期望中性 200（disconnected、checkedAt 空、diagnosis NONE）；
   broker runtime 在缺投影源时为 503 BROKER_READ_UNAVAILABLE。owner：`product_production_ports_system.rs`、
   `product_production_ports_trade.rs`。
4. **插件 requiresRebuild 固定 false（P2）**：Go 目录项为 true，且缺单条五步生命周期串联用例。owner：`product_production_ports_plugins.rs`。
5. **策略定义版本历史投影缺口（P2）**：全仓无 isCurrent/版本顺序断言，也没有 create→list→detail→update→versions 单条串联。
   owner：策略定义读写叶。
6. **策略日志/审计与 definitionSync 因果链缺口（P2，延续）**：读组语料由快照端口回放，缺“启动实例→日志可见 started”、
   “运行时错误事件→列表 logs 尾部”、“定义改版→definitionSync.isLatest 翻转→refresh-definition”的因果链。
7. **运行期改端口与按请求 503 无等价入口（P2）**：system/status 的 apiPort 来自启动配置；watchlist 库不可打开为启动期
   fail-closed，全仓无 DATABASE_INCOMPATIBLE 错误码。
8. **边界保留**：内嵌前端资源加载辅助、CLI run 布局与取消语义、nil sidecar 方法、桌面 profile/runtime defaults、
   market profiles 的 CN 前缀推断（Rust 有意放宽，Go 硬拒绝）、助手库不可用时的降级启动（Rust 启动期 fail-closed）。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增与关联用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(instantiate_rejects_stored_definitions_with_a_retired_source_format) \| test(instantiate_persists_the_same_normalized_binding_as_runtime_update) \| test(instantiate_accepts_empty_body_but_rejects_malformed_json)'` | 4/4 通过（修复前先红探针为 0/1 失败） |
| OpenAPI 契约用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked -E 'test(served_swagger_document_matches_the_checked_in_contract) \| test(swagger_document_requires_explicit_error_envelopes_for_every_operation) \| test(swagger_document_publishes_typed_writable_request_bodies) \| test(swagger_document_publishes_the_typed_broker_runtime_response)'` | 4/4 通过 |
| 策略 pine 兼容目标 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --test strategy_pine_compatibility --locked` | 7/7 通过（新增谓词用例消除 dead_code 警告） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127f_payload.json` | 13 行更新，`[x]` 1519 → 1524 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3189 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1532、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1870/1871（`runtime_dependencies::tests::node_probe_reports_ok_outdated_unrecognized_and_failed_scripts` 在并行负载下抖动），隔离复跑通过后整轮复跑 1871/1871 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok），与本分片改动无关 |

后续（分片六）：`internal/app/apiserver/marketdataapp` 68 行，随后 webaccess 24 + tradingapp 11、
backtestapp 10 + datamigration 11 与 runtime/lifecycle/runtimes/application/futuapp 余量。`servercoretest` 的 52 行待办
在本分片后全部给出终值。

### 分片六：`internal/app/apiserver/marketdataapp` 前 35 行

范围（按文件与行号升序）：`assistant_provider_test.go:95/:114/:143/:159`；`data_plane_switch_test.go:66/:83/:189/:237/:265`；
`heartbeat_test.go:14/:108/:155`；`market_depth_test.go:35/:198/:244`；`market_http_test.go:533/:555/:577/:592/:607/:622`；
`provider_boundaries_test.go:145/:163/:197/:220`；`provider_test.go:12/:107`；`python_runtime_test.go:13/:55/:79/:94`；
`query_test.go:21`；`runtime_akshare_test.go:126`；`runtime_forwarding_test.go:12/:98`。owner 集中在 `crates/jftrade-engine`
（市场数据 provider 投影与切换、深度/证券读路由、目录归一、运行时状态投影）、`crates/jftrade-marketdata`（router 激活与
readiness、订阅 demand）、`crates/jftrade-settings`（provider 选择持久化）、`crates/jftrade-integration-marketdata-helper`
（helper 进程与资产落地）与 `crates/jftrade-integration-futu`（OpenD 深度投影）。

本分片新增 Rust 证据 3 项（2 行升 `[x]`）：

- `crates/jftrade-engine/src/product_settings_read_tests.rs::provider_switch_succeeds_while_watchlist_ports_are_unavailable`
  （Go `data_plane_switch_test.go:66`）：在未装配任何 watchlist 端口的组合上先断言 watchlist 读取面不可用，再 PUT
  `/api/v1/settings/market-data-provider` 断言 200 且 `data.activeProvider=akshare`，与参照“ApplyProviderSettings 容忍
  不可用 watchlist”等价。
- `crates/jftrade-engine/src/product_production_ports_market_data_catalog_futu.rs::unknown_search_prefixes_are_never_inferred_as_a_market`
  （Go `provider_test.go:107`）：未知前缀不被推断成 HK/SH/SZ，`bad.CODE` 保持点号形状（大小写由全局搜索归一承担），
  裸代码保持无前缀；同族用例继续覆盖已知别名与点号代码保留。
- `crates/jftrade-settings/src/market_data_provider.rs::blank_provider_selection_is_rejected_before_persistence`
  （Go `assistant_provider_test.go:114` 的空白 providerId 半边）：空串、纯空白与制表符输入均返回 Invalid，且不覆盖已存
  选择；该行仍为 partial，因为参照还要求 before/after 信封（Rust 响应只含 activeProvider）。

映射结果：35 行复核（2 行升 `[x]`；1 行收紧 partial 并补空白输入断言；其余 32 行复核后确认原终值仍准确）。引用存在性
全量校验通过（其中两条为 `file.rs::tests::name` 形式的模块限定路径）。计数：`[x]` 1524 → 1526；audit Rust 测试 3194；
anchors 1535（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P1/P2）：

1. **助手侧 provider 聚合工具未迁移（P2）**：参照的 AssistantMarketProviders 返回
   liveProvider/backtestProvider/providers[]/liveHealth/liveRuntime/checkedAt，Rust 无对应 MCP 工具（同信息由 REST
   `/api/v1/market-data/provider` 与两条 settings 路由承担）。owner：`crates/jftrade-engine` MCP 工具面。
2. **深度与证券读信封差异（P2）**：参照的深度响应含 request/depth/meta（source=bbgo:futu、fromCache），Rust 直接回传
   provider 载荷；证券读的 warrant/option/future/trust/index/plate 研究块在 Rust 契约中不存在（边界保留，禁止伪造）。
   owner：`product_production_ports_market_data_quote.rs` 与 `product_watchlist_*` 之外的行情读投影。
3. **心跳策略未折算进 wire（P2，延续）**：轮询模式 sampleFreshness=interval+timeout 与 transport/staleReasons 明细
   只存在于 provider 状态投影，心跳信封仍缺 marketDataProviderId/sampleFreshnessMs/staleReasons。owner：`crates/jftrade-api`
   心跳载荷与 `crates/jftrade-engine` 运行时状态投影。
4. **python/helper 运行时形态差异（边界）**：参照按 DevPython→workspace venv→PATH 解析解释器、保留 legacy 环境变量
   别名与“通用缓存目录 + legacy 回退”，并用 `python -c` 探针校验版本与依赖；Rust 使用冻结 helper 二进制 + 内容寻址资产
   + `/health` 探针，属迁移期接口差异（owner：`crates/jftrade-integration-marketdata-helper`）。
5. **切换恢复语义差异（P2）**：参照在切回 futu 时返回前同步恢复物理订阅、并提供 ProviderNeedsActivation 回调的
   nil/健康/不可用三分支；Rust 保留 demand 所有权并按代际异步对账，nil-service 形状由装配期端口缺失替代。owner：
   `crates/jftrade-marketdata` router 与 `crates/jftrade-engine/src/product_active_provider_state.rs`。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(provider_switch_succeeds_while_watchlist_ports_are_unavailable) \| test(unknown_search_prefixes_are_never_inferred_as_a_market) \| test(canonical_search_code_does_not_double_prefix_the_market)'` | 3/3 通过（未知前缀用例首轮按原样大小写断言失败，修正断言后转绿） |
| settings crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings --all-targets --locked` | 59/59 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127h_payload.json` | 3 行更新，`[x]` 1524 → 1526 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3194 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1535、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1873/1873 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

后续（分片七）：`internal/app/apiserver/marketdataapp` 余 33 行（`runtime_health_test.go:94/:114/:142/:167/:222`；
`runtime_test.go:85/:191/:230/:370/:394/:476/:547/:570/:591/:633/:667/:704/:724`；`sidecar_os_process_test.go:146`；
`sidecar_process_test.go:61/:147/:190/:208/:261/:297`；`sidecar_signal_test.go:9/:24`；`unavailable_provider_test.go:12/:72`；
`watchlist_source_test.go:15/:74/:100/:121`），随后 webaccess 24 + tradingapp 11、backtestapp 10 + datamigration 11 与
runtime/lifecycle/runtimes/application/futuapp 余量。

### 分片七：`internal/app/apiserver/marketdataapp` 余 33 行

范围（按文件与行号升序）：`runtime_health_test.go:94/:114/:142/:167/:222`；
`runtime_test.go:85/:191/:230/:370/:394/:476/:547/:570/:591/:633/:667/:704/:724`；`sidecar_os_process_test.go:146`；
`sidecar_process_test.go:61/:147/:190/:208/:261/:297`；`sidecar_signal_test.go:9/:24`；`unavailable_provider_test.go:12/:72`；
`watchlist_source_test.go:15/:74/:100/:121`。owner 集中在 `crates/jftrade-marketdata`（router 激活/readiness/恢复）、
`crates/jftrade-integration-marketdata-helper`（进程启动、就绪重试、停止与超时）、`crates/jftrade-integration-futu`
（订阅 reconciler 的延迟释放与重试阶梯）、`crates/jftrade-engine`（active provider state、runtime supervisor、helper
health monitor、watchlist 读投影）。

本分片新增 Rust 证据 1 项（1 行升 `[x]`）：

- `crates/jftrade-integration-marketdata-helper/src/client.rs::retries_transient_readiness_and_sends_optional_bearer`
  （新增 Parity 锚点）与 `crates/jftrade-marketdata/src/router.rs::recovery_after_a_failed_health_check_publishes_a_healthy_provider`
  （既有锚点），对应 Go `runtime_health_test.go:167`：mock 健康端点第一次 503、第二次 200，readiness 恰好重试一次后成功；
  router 侧断言失败的激活不发布、保留原 active provider，健康恢复后再次激活成功且只发布一次并上报 provider 自身 stream mode。
  重试阶梯的具体常量差异（Go 100ms→1s 与 Rust 500ms→10s）仍由 `runtime_health_test.go:222` 单独登记，不在该行宣称等价。

映射结果：33 行复核（1 行升 `[x]`；其余 32 行复核后确认原终值仍准确，其中 12 行是旧 Go 组合根/指针/信号计划模型的边界保留）。
引用存在性校验通过。计数：`[x]` 1526 → 1527；audit Rust 测试 3194；anchors 1535（unrecorded 0、stale 0、unknown 53）。
`marketdataapp` 的 68 行待办至此全部给出终值。

关键事实与新登记缺口（P2 为主）：

1. **组合根错误聚合缺失（P2）**：参照把 health 失败与 sidecar restore 失败合并返回、把激活失败与回滚释放失败合并返回、
   把两次有界清理失败合并；Rust 是单点错误 + `ProductShutdownSupervisor` 聚合，错误形状与重试次数不同。owner：
   `crates/jftrade-engine/src/product_runtime_supervisor.rs`、`crates/jftrade-integration-marketdata-helper`。
2. **provider lease 引用计数模型缺失（P2）**：参照的 AcquireProvider/ProviderLease 支持同实例共享、Release 幂等、
   最后一个租约释放才停 sidecar、切换后旧租约钉住旧实例；Rust 无此公共 API，生命周期由 HelperHealthMonitor、
   ProductShutdownSupervisor 与 ActiveProviderState 分别拥有。属于需要产品决策的 owner 模型差异。
3. **订阅释放延迟与激活耦合（P2）**：参照的“健康激活成功后 release 失败只延迟、后续 reconcile 暴露”与“非活动 provider
   清理失败不影响前台 reconcile”在 Rust 拆到 SubscriptionReconciler 的延迟窗口/重试阶梯（已断言），缺组合级串联断言。
   owner：`crates/jftrade-integration-futu/src/subscriptions*.rs` 与 provider runtime 装配。
4. **取消与超时语义（P2）**：参照的 Activate 携带 ctx（排队期间取消不提交、退役 context 过期仍提交）；Rust 的 activate
   是同步 Result，无取消点，超时/取消由调用方（settings 写路径）决定，active state 只保证“被拒绝的转换不发布、不推进
   generation”。
5. **不可用 provider 组合（P2）**：参照的 newUnavailableProvider 仍可读 Descriptor（selectionId/providerId=custom），
   其余九类操作统一报 provider is unavailable、Health=failed 且 lastError 含 provider activation failed；Rust 无合成
   unavailable provider，读路由在缺端口/不可用时 fail-closed（descriptor 不可读）。
6. **watchlist source 选择器（P2）**：参照的 app 层 source 选择器按活动 provider 在 Futu 与 Python 之间路由并保留
   missing/permission 错误且不回落；Rust 由装配期确定 watchlist read port 与 helper provider，缺逐条断言。
7. **边界保留**：sidecar 停止信号计划（参照 SIGTERM→升级/立即强杀的可注入 plan，Rust 一律 start_kill + stop_timeout）、
   旧 Go 源码模式的 PYTHONPATH 校验与 Python>=3.11/模块探针（Rust 由开发者 venv + /healthz 就绪探测承担）、
   nil Runtime/lease 守卫（Rust 用 Option 与枚举表达）。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增与关联用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-marketdata --all-targets --locked -E 'test(retries_transient_readiness_and_sends_optional_bearer) \| test(recovery_after_a_failed_health_check_publishes_a_healthy_provider)'` | 2/2 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127j_payload.json` | 1 行更新，`[x]` 1526 → 1527 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3194 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1535、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1873/1873 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

后续（分片八）：`internal/app/apiserver/webaccess` 24 行 + `tradingapp` 11 行，随后 backtestapp 10 + datamigration 11 与
runtime/lifecycle/runtimes/application/futuapp 余量。

### 分片八：`internal/app/apiserver/webaccess` 24 行 + `tradingapp` 11 行

范围（按文件与行号升序）：`webaccess/auth_boundaries_test.go:43/:101/:138/:195/:235/:282/:295/:319`；
`webaccess/frontend_test.go:50/:67/:81/:106/:139`；`webaccess/security_integration_test.go:175/:210/:237/:256/:278/:293/:362/:422/:437/:461/:490`；
`tradingapp/execution_gateway_lifecycle_test.go:144/:202/:298/:331`；`tradingapp/notifications_test.go:32`；
`tradingapp/order_update_source_broker_test.go:12/:33`；`tradingapp/order_updates_test.go:13/:70/:80/:92`。
owner：`crates/jftrade-api`（访问策略、来源校验、请求上下文、WS 握手判定）、`crates/jftrade-engine`（安全集成装配、
执行网关生命周期、订单更新源与通知投影）、`crates/jftrade-integration-futu`（订单更新源）。

本分片新增 Rust 证据 6 项（6 行升 `[x]`）：

1. `crates/jftrade-api/tests/transport_contracts.rs::session_cookie_reads_and_csrf_protected_writes_share_one_browser_flow`
   对应 Go `security_integration_test.go:293`：同一条 cookie 会话读 200、缺 CSRF 写 403、带 CSRF 写 200，端口恰好两次调用。
2. `crates/jftrade-api/tests/transport_contracts.rs::web_mode_rejects_the_development_origin` 对应 Go
   `security_integration_test.go:362`：`Origin: http://localhost:3003` 被 403 且 `error.code=ORIGIN_FORBIDDEN`，端口零调用。
3. `crates/jftrade-api/tests/transport_contracts.rs::untrusted_origin_is_rejected_while_the_allowed_lan_origin_authenticates`
   对应 Go `security_integration_test.go:422`：`https://evil.example.com` 403，允许的 `http://192.168.1.10:3008` 200 且端口一次调用。
4. `crates/jftrade-engine/src/product_ws_live_tests.rs::ws_live_transport_upgrades_with_a_browser_session_cookie_alone`
   对应 Go `security_integration_test.go:490`：产品实例只配置会话策略（`enforce_access` + `session_token`，无桌面 token），
   真实握手无凭据 401 `WEB_AUTH_REQUIRED`、仅会话 cookie 升级 101。
5. `crates/jftrade-api/src/router.rs::forwarded_client_identity_uses_the_proxy_appended_address` 对应 Go
   `auth_boundaries_test.go:282`：回环对端取代理追加的最后一段地址，非回环对端保留自身地址而不信任该头。
6. `crates/jftrade-api/src/router.rs::request_scheme_trusts_tls_and_loopback_proxy_only` 对应 Go
   `frontend_test.go:139`：https URI 为安全、回环对端转发取最后一段协议、非回环对端忽略转发协议。

实现探针（先红后绿，仅用于定位判定归属，未修改生产代码）：在 `crates/jftrade-api/tests/transport_contracts.rs` 里以
`tower::ServiceExt::oneshot` 直接驱动 router 时，带合法升级头与会话 cookie 的 `/api/v1/ws/live` 请求返回 426 而非 101——
axum 的 `WebSocketUpgrade` 提取器要求 `hyper` 升级态存在（`ConnectionNotUpgradable`），进程内 oneshot 不提供该状态。
因此 WS 会话升级判定改由产品级真实 TCP 握手断言（第 4 项），router 层不再复制该实现细节。

映射结果：35 行复核（6 行升 `[x]`；其余 29 行复核后确认原终值仍准确，其中 webaccess 11 行、tradingapp 8 行为结构/产品边界差异）。
引用存在性校验通过。计数：`[x]` 1527 → 1533；audit Rust 测试 3200；anchors 1541（unrecorded 0、stale 0、unknown 53）。
`internal/app/apiserver` 域的 331 行待办至此全部给出终值。

关键事实与新登记缺口（P1/P2）：

1. **浏览器入口的禁用语义差异（P1）**：参照在 private 模式下对远端浏览器返回 403 `REMOTE_WEB_ACCESS_DISABLED`
   （`security_integration_test.go:256/:437`），并在 Web 关闭时对浏览器入口返回 403 `WEB_ACCESS_DISABLED`
   （`:461`）；Rust 以监听地址表达同一意图（关闭不监听、private 绑 127.0.0.1、public 绑 0.0.0.0），远端得到连接拒绝而非
   403 文案，且无 HTML 入口。owner：`crates/jftrade-engine/src/product_server_runtime.rs`。
2. **桌面与浏览器对照断言缺失（P1）**：参照断言“同一进程内桌面 bearer 仍 200、浏览器入口 403”
   （`security_integration_test.go:461`）与“helper 注入令牌不得绕过密码会话”（`security_integration_test.go:175`）；
   Rust 的桌面 token 通道与 Web 会话通道各自有测试，缺同进程对照断言。owner：`crates/jftrade-api/src/auth.rs` 与 engine 装配。
3. **代理与会话边界（P2）**：参照覆盖同主机 HTTPS 代理使用安全会话 cookie（`:210`）、网络客户端不能伪造 HTTPS 代理协议
   （`:237`）、历史 admin bearer 不再认证（`:278`）。Rust 的 `secure` 由 URI scheme 或回环代理转发推导（已由第 6 项断言），
   cookie 的 `Secure` 属性取决于该上下文；Rust 在普通 Web 模式不认任何 bearer，错误码为 `INTERNAL_PROXY_AUTH_REQUIRED`
   或 `WEB_AUTH_REQUIRED` 而非参照的单一 401 形状，差异不合并为等价。
4. **会话与登录状态矩阵（P2）**：参照表驱动断言登录的 403/503/400/200/401/408/500 全矩阵
   （`auth_boundaries_test.go:138/:319`）、过期会话不再认证（`:101`）、会话上限与剪枝（`:235`）、状态映射有界（`:295`）、
   登录期间改密码不得生成旧密码会话（`:195`）；Rust 的 `AuthSessionWritePortError` 定义了同一映射，但
   “熵失败 500 / 取消 408 / 桌面受信 200”与 `SettingsChanged`（409）分支尚无独立路由或端口断言。
   owner：`crates/jftrade-engine/src/product_auth_session_manager.rs`、`product_auth_session_write_port.rs`。
5. **前端辅助边界（P2）**：参照的 `frontend_test.go:67/:81/:106` 覆盖开发代理仅允许回环、前端构造边界与 SPA 索引判定表；
   Rust 由静态资产服务与 SPA 回退承担（已有 404/回退断言），无逐项 helper 同形对象。
6. **执行网关聚合对象缺失（P2）**：参照的分支表断言 broker 不匹配、缺订单存储、prepare 错误透传、stale/fresh 落库差异、
   trading 不可用时写 UNKNOWN、成功写 placed + 通知 + session payload（`execution_gateway_lifecycle_test.go:144/:202/:298/:331`）。
   Rust 没有 `ExecutionGateway` 聚合对象：下单落库拆到 execution order store 与预览端口，UNKNOWN 与通知由对账/通知投影承担。
   owner：`crates/jftrade-engine` 下单写路径。
7. **订单更新源与通知投影（P2）**：参照的订单通知消息拼接可用标识（`notifications_test.go:32`）、订单更新源先激活再发现账户
   并按键过滤订阅（`order_update_source_broker_test.go:12/:33`）、worker 构造与 nil 守卫（`order_updates_test.go:13/:70/:80`）、
   无活动运行时时返回 `ErrOrderUpdateSourceInactive` 与空订阅（`:92`）。Rust 消息为事件类型短语、对账端口按账户/环境 scope
   过滤、读路由 fail-closed，且无空订阅对象与同形错误码。owner：`crates/jftrade-engine` 通知投影与对账/推送路径。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增与关联用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api --all-targets --locked -E 'test(session_cookie_reads_and_csrf_protected_writes_share_one_browser_flow) \| test(web_mode_rejects_the_development_origin) \| test(untrusted_origin_is_rejected_while_the_allowed_lan_origin_authenticates) \| test(forwarded_client_identity_uses_the_proxy_appended_address) \| test(request_scheme_trusts_tls_and_loopback_proxy_only)'` | 5/5 通过 |
| 新增 WS 用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(ws_live_transport_upgrades_with_a_browser_session_cookie_alone)'` | 1/1 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127m_payload.json` | 6 行更新，`[x]` 1527 → 1533 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；Rust 测试 3200 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1541、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -p jftrade-engine --all-targets --locked --no-fail-fast` | 1960/1960 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 均在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

并行负载抖动（已隔离复跑确认，不记为失败）：`check:quick` 首轮命中
`api_launcher_reports_startup_failure_when_the_configured_address_is_taken`（端口占用重试断言 `None != Some(0)`）与
`adk_session_detail_omits_resolved_approval_groups` 两条已知抖动用例；两条隔离复跑均通过，随后整轮 `check:quick` 通过。
构建缓存已按门禁清理（`cargo clean` 移除 120081 文件 / 32.8GiB，`.rcgu.o` 0 个）。

后续（分片九）：`internal/app/apiserver` 余 74 行（[~] 计数）——`backtestapp` 10 行、`datamigration` 11 行，以及
`runtime`/`lifecycle`/`runtimes`/`application`/`futuapp`/`liveapp`/`status`/`strategyapp`/`databaseguard`、
`server_test.go`、`desktop_api_startup_test.go` 的余量；此后进入 `strategy_pine` 465 → `assistant_workflow` 447 →
`other` 311 → `backtest_calendar` 262 → `storage_sqlite` 178 → `marketdata_quotes` 155 → `futu_opend` 104 →
`settings_watchlist` 39。

### 分片九：`internal/app/apiserver` 余 21 行（`backtestapp` 10 + `datamigration` 11）

范围（按文件与行号升序）：`backtestapp/historical_source_test.go:62/:87/:108/:141/:206/:298/:354/:385/:401/:414`；
`datamigration/maintenance_failure_paths_test.go:16/:70/:109/:167/:270/:538`、`datamigration/maintenance_test.go:82/:315`、
`datamigration/manager_boundaries_test.go:14/:280`、`datamigration/rebuild_safety_test.go:211`。owner：
`crates/jftrade-engine`（回测同步窗口与 provider 能力守卫）、`crates/jftrade-datamanagement`（清理预览与确认校验）、
`crates/jftrade-store-sqlite`（维护快照、overview 检查、候选集与租约）。

本分片新增 Rust 证据 1 项（1 行升 `[x]`）：

- `crates/jftrade-engine/src/product_backtest_sync_start_tests.rs::akshare_lookback_windows_are_scoped_to_the_declared_market`
  对应 Go `historical_source_test.go:141`：同一条 6 天窗口下 US 5m 被 `provider akshare limits 5m history to 5 days` 拒绝、
  HK 5m 放行（市场级 `US:5m` 不约束其它市场）、HK 1m 仍受一分钟规则限制。
  先红探针：把 `ak_share_lookback_days` 的市场维度去掉（`("US", "5m" | ...)` 改为 `(_, "5m" | ...)`）后 HK 断言立即失败
  （`the US-only five-minute window must not constrain HK: BadRequest(...)`）；随即按字节回滚
  `crates/jftrade-engine/src/product_backtest_sync_request.rs`，回滚前后 shasum 均为 `69aaf8595ec5cc0c9f2d63fe9943c023acef1373f7980de25a7eabf8982dd7fd`。

账本字段修正 11 行（`datamigration`）：此前 10 行把覆盖证据写在 `conclusion` 里、`rust_entry` 留空
（`manager_boundaries_test.go:14` 还把 `conclusion` 放成了 nextest 命令），审计因此计为“partial 引用不可解析”。
本次把每条覆盖证据落到 `rust_entry` 并补全真实路径：

- `crates/jftrade-store-sqlite/src/maintenance_backup_retention.rs::managed_backup_discovery_parses_only_canonical_filenames`
- `crates/jftrade-store-sqlite/tests/maintenance_overview_and_cleanup_contracts.rs::{overview_counts_main_wal_and_shm_and_keeps_an_unreadable_database_local,
  cleanup_preview_requires_a_ready_database_with_the_purgeable_table, cleanup_candidates_fail_closed_when_the_maintenance_tables_are_missing,
  database_inspection_classifies_filesystem_and_schema_states, overview_summary_only_skips_storage_and_a_single_filter_keeps_its_totals,
  overview_normalizes_rebuild_marker_ids_and_fails_closed_when_it_is_unreadable}`
- `crates/jftrade-store-sqlite/tests/maintenance_backup_and_rebuild_contracts.rs::{backup_snapshot_is_private_verified_and_limited_to_managed_databases,
  a_held_writer_lease_rejects_maintenance_before_any_snapshot_is_written, a_failed_rebuild_batch_removes_every_snapshot_it_created,
  backup_and_compact_fail_closed_when_the_source_database_is_missing, a_corrupt_rebuild_marker_blocks_backup_and_rebuild_without_deleting_data}`
- `crates/jftrade-store-sqlite/tests/maintenance_cleanup_candidates.rs::soft_deleted_adk_rows_are_the_only_candidates_and_changes_reject_execute`
- `crates/jftrade-datamanagement/src/{cleanup.rs::preview_normalizes_defaults_summarizes_and_expires_after_ten_minutes,
  cleanup.rs::preview_rejects_invalid_retention_and_non_ready_databases,
  maintenance.rs::maintenance_service_confirmation_validation_and_rejection_parity,
  lib.rs::cleanup_requires_the_exact_approved_candidate_set,
  overview.rs::overview_preserves_go_order_filter_and_rebuild_projection}`

终值不变（10 行 partial + 1 行 boundary），差异结论保留在 `conclusion`。

顺带修正 4 条跨分片的陈旧引用（不改终值）：`internal/api/marketdata/routes_boundaries_test.go:119` 改引
`crates/jftrade-engine/tests/market_data_production_compatibility.rs::test_non_futu_helper_candles_failure_uses_generic_market_code`；
`internal/app/apiserver/marketdataapp/runtime_test.go:667` 改引
`crates/jftrade-engine/src/product_production_assembly_tests.rs::production_market_data_catalog_and_provider_ports`；
`internal/app/apiserver/servercore/server_business_test.go:25` 修正为
`crates/jftrade-engine/src/product_production_ports_market_data_quote_reads_futu.rs::test_futu_kline_query_window_resets_invalid_begin_to_default_lookback`；
`internal/app/apiserver/webaccess/auth_boundaries_test.go:43` 改引 `crates/jftrade-api/src/auth.rs::{request_origin_uses_production_semantics_without_malformed_origin_fallback,
origin_normalization_accepts_web_and_tauri_schemes}`。审计的 partial 引用不可解析警告由 7 条降到 3 条。

映射结果：21 行复核（1 行升 `[x]`、11 行修正字段、9 行确认原终值仍准确）加 4 行跨分片引用修正。
计数：`[x]` 1533 → 1534；partial 2309 → 2308；boundary 609；audit Rust 测试 3201；anchors 1542
（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P2）：

1. **provider 能力矩阵逐条断言缺口**：`historical_source_test.go:108`（前复权拒绝、超 lookback 拒绝、5m 不支持、1m+extended 不支持）
   与 `:87`（非 US 日内扩展时段拒绝）在 Rust 分别落在复权映射校验、同步窗口守卫与 US 扩展时段降级
   （`plan_sync_intervals` 把 `1d/1w/1mo` 降为 `1h`），缺 provider 能力矩阵驱动的逐条组合断言，也没有 HK 扩展时段拒绝入口。
   owner：`crates/jftrade-engine/src/product_backtest_sync_request.rs` 与 provider registry。
2. **InstrumentSpec 保守回退常量**：`historical_source_test.go:354` 断言规则失败时退回 HK 500/0.2、A 股 100、US 1/0.01；
   Rust 的 `crates/jftrade-broker/tests/market_rules_snapshot_errors.rs` 只断言手数推导与规则覆写顺序，缺逐市场保守常量断言。
3. **Python provider 就绪白名单**：`historical_source_test.go:385/:401` 断言 yfinance/akshare 需要 provider 就绪、Futu 不需要，
   且 ProviderOptions 需要 market-data runtime；Rust 的就绪门与端口装配在组合期确定，缺该白名单断言，也没有函数式 options 构造器。
4. **维护限流与注入缝缺失**：Go 的 30 秒 `backupMinimumInterval`（429）在 Rust 无实现（连续两次 backup 均成功，靠 WriterLease
   串行加保留策略限制磁盘增长）；Go 的 BusyReason hook、`vacuum failed` 注入、`SetMaintenanceHooks` 的 purge/delete 故障注入
   与可变 descriptor 表在 Rust 组合根一次性装配，没有等价注入点。owner：`crates/jftrade-store-sqlite` 维护路径与 `crates/jftrade-datamanagement`。
5. **环境解析路径无测试**：`DesktopRetainedRuntimeConfig::from_process_env` 的 `JFTRADE_MARKETDATA_SIDECAR` 优先、
   `JFTRADE_MARKETDATA_DEV_PYTHON(+PATH)` 构造 `-m marketdata_sidecar.main` 前缀参数的行为没有任何测试；
   `env::set_var` 在 edition 2024 属 unsafe 且 crate 为 `forbid(unsafe_code)`，补测需先引入可注入的环境查找接口（生产改动，需产品决策）。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例（先红探针后绿） | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(akshare_lookback_windows_are_scoped_to_the_declared_market)'` | 探针后 0/1 失败并精确命中 HK 断言；回滚后 1/1 通过（连带既有 `akshare_sync_rejects_history_beyond_the_intraday_lookback_window` 2/2 通过） |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127o_payload.json`、`/tmp/s127o_hygiene.json` | 12 行修正 + 4 行引用修正，`[x]` 1533 → 1534 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析警告 7 → 3；Rust 测试 3201 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1542、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1875/1875 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

后续（分片十）：`internal/app/apiserver` 余 53 行——`runtime` 8、`lifecycle` 8、`runtimes` 8、`application` 7、`futuapp` 6、
`liveapp` 4、`server_test.go` 8、`desktop_api_startup_test.go` 2、`status`/`strategyapp`/`databaseguard` 各 1（另有
`servercore`/`servercoretest`/`marketdataapp` 中未纳入本批队列的既有 [~] 行可在后续批次复查）；
此后进入 `strategy_pine` 465 → `assistant_workflow` 447 → `other` 311 → `backtest_calendar` 262 → `storage_sqlite` 178 →
`marketdata_quotes` 155 → `futu_opend` 104 → `settings_watchlist` 39。

### 分片十：`internal/app/apiserver` 余 53 行（第 127 批收尾）

范围（按文件与行号升序）：`runtime/environment_fallbacks_test.go:14`、`runtime/resources_test.go:58`、
`runtime/runtime_test.go:12/:28/:44/:59/:170/:185`；`lifecycle/lifecycle_test.go:90/:196/:490/:516/:555/:587/:691/:711`；
`runtimes/handle_lifecycle_test.go:110/:250/:370/:513/:552/:573/:592/:622`；
`application/assistant_test.go:75`、`application/installers_test.go:16`、`application/lifecycle_test.go:41/:68`、
`application/resources_test.go:112/:137`、`application/runtime_dependencies_test.go:11`；
`futuapp/coordinator_test.go:15/:69/:109`、`futuapp/runtime_state_boundaries_test.go:26/:44/:51`；
`liveapp/bbgo_notifications_test.go:12/:43/:84`、`liveapp/handler_test.go:7`；`status/status_test.go:88`；
`strategyapp/runtime_ports_test.go:62`；`databaseguard/groups_test.go:14`；`server_test.go:20/:73/:176/:184/:250/:359/:518`；
`desktop_api_startup_test.go:40/:106`。owner：`crates/jftrade-engine`（运行时装配与生命周期、状态投影）、
`crates/jftrade-integration-futu`（OpenD 探针与行情健康）、`apps/desktop/src-tauri`（桌面启动与就绪）。

本分片新增 Rust 证据 3 项（4 行升 `[x]`）：

1. `crates/jftrade-engine/src/product_market_data_runtime_status.rs::runtime_status_wire_drops_absent_and_blank_values_and_normalizes_utc`
   对应 Go `status/status_test.go:88 TestTimeAndStringPointers`：`+08:00` 的 lastRefreshAt 在 wire 上归一为
   `2026-06-01T04:00:00.123Z`，空白 quoteLastError 为 null、带空格错误串被 trim 保留，缺省状态的全部时间与错误字段为 null
   （Rust 用 `Option` 表达 Go 的 nil，零值时间不可构造）。
2. `crates/jftrade-integration-futu/src/health.rs::market_data_health_requires_a_known_logged_in_quote_session` 对应 Go
   `futuapp/coordinator_test.go:69 TestMarketDataHealthRequiresHealthyOpenDQuoteSession`：quote session 未知、已登出、degraded 带错误、
   登入且健康四条分支与错误文案逐字符一致，readiness 分别为 Failed/Ready。
3. `crates/jftrade-integration-futu/src/health.rs::disconnected_probe_keeps_its_transport_error_for_manual_retry` 对应 Go
   `futuapp/runtime_state_boundaries_test.go:26 TestFutuRuntimeRemainingDisconnectedAndResetPaths` 的探针错误传播部分，配合既有
   `crates/jftrade-integration-futu/src/probe.rs::probe_opend_reports_closed_port_as_disconnected`（disconnected + last_error + issue_code）
   与 `crates/jftrade-engine/src/product_production_assembly_tests.rs::production_opend_health_diagnoses_enabled_but_unreachable_opend`
   （离线投影 connectivity=disconnected、diagnosis.code=OPEND_API_CONNECTIVITY、manualRetryRequired=true）共同闭环。
4. 引用纠正后升 `[x]`：`futuapp/runtime_state_boundaries_test.go:51 TestFutuRuntimeHealthyProbeAndGlobalStateBoundaries` 改引
   `crates/jftrade-integration-futu/src/health.rs::tcp_probe_maps_login_global_state_and_market_readiness`（connected/healthy/
   server_version=10.9.7000/quote_logged_in/markets.len()==4）与
   `crates/jftrade-integration-futu/src/probe.rs::probe_from_global_state_enforces_minimum_version_and_maps_neutral_state`，
   原引用 `tcp_probe_reports_protocol_outcomes_without_a_real_opend` 只覆盖失败协议路径（该测试与 :26 行的原引用属错配，本分片互换纠正）。

账本字段补全：本批 `internal/app/apiserver` 的 331 行待办此前有 87 行 `command` 为空（servercore 72、servercoretest 10、
webaccess 5），本分片按各条引用证据的所属 crate 补齐 nextest 命令（engine／store-sqlite／store-settings-file／api／
datamanagement／desktop 及组合命令），并补全 `servercore/server_bootstrap_degraded_runtime_test.go:84` 的证据引用
（`crates/jftrade-engine/src/strategy_runtime_execution_tests.rs::test_execute_strategy_intents_success_calls_execution_and_audits`，
桥装配点 owner 为 `crates/jftrade-engine/src/product_production_ports.rs`）。审计的 partial 引用不可解析警告由 3 条降到 2 条
（剩余 2 条为 `marketdataapp/sidecar_process_test.go:147/:208` 引用的 `product_runtime_workers.rs::from_process_env` 生产符号，
该文件没有任何测试，登记为需先引入可注入环境查找接口的生产改动）。

映射结果：53 行处理（4 行升 `[x]`、49 行复核后确认原 partial/boundary 终值仍准确，含 8 行旧 Go/Wails 启动与 GUI 边界）
加 87 行命令补全与 1 行引用纠正。计数：`[x]` 1534 → 1538；partial 2308 → 2304；boundary 609；audit Rust 测试 3204；
anchors 1545（unrecorded 0、stale 0、unknown 53）。**第 127 批 `internal/app/apiserver` 331 行待办至此全部给出终值**
（servercore 98、servercoretest 52、marketdataapp 68、webaccess 24、tradingapp 11、backtestapp 10、datamigration 11、余量 53）。

关键事实与新登记缺口（P2）：

1. **启动与就绪模型差异**：Go 的 `StartForRunArgs`/`RunAPIOnly` 以 context 取消返回并聚合 handler 关闭错误，Rust 由
   `start_product_runtime` 直接返回错误、`handle.shutdown` oneshot 结束服务循环（不聚合 handler 错误）；桌面侧以启动快照
   readiness fail-closed 判定，取代 Go 的 HTTP 授权轮询与超时窗口（`server_test.go:184/:359/:518`、`desktop_api_startup_test.go:106`）。
   owner：`crates/jftrade-engine/src/product_server_runtime.rs`、`apps/desktop/src-tauri/src/native_lifecycle.rs`。
2. **所有权与聚合语义**：Go 的 `Lifecycle`/`Resources`/`Runtimes Handle` 支持懒注册、迟到注册立即关闭与
   `errors.Join` 式的多因聚合；Rust 由组合根一次性构造 + `Arc` 所有权 + 单次 shutdown 表达，关闭返回首个错误，
   不存在 nil handle／关闭后注册路径（`application/*`、`runtimes/handle_lifecycle_test.go:110/:592/:622`）。
3. **环境契约差异**：Rust 不读取 Go 的 `FUTU_OPEND_ADDR`/`JFTRADE_FUTU_*`/`JFTRADE_ADK_SKILLS_DIR`/`JFTRADE_ADK_SESSION_DB`
   同名变量（改用 `JFTRADE_FUTU_OPEND_HOST/PORT`、`JFTRADE_ADK_SKILLS`、`JFTRADE_ADK_SESSION_DB`），
   `JFTRADE_REAL_TRADE_CONTROL_PATH` 只有声明与派生级证据（工作区禁止进程级 env 写入，`runtime/*` 6 行）。
4. **futuapp 协调器形态**：Go 的 Coordinator.Reset 保持应用注册顺序并把 Futu 状态置失效，Rust 由连接替换清理配额/登记
   （`subscriptions_tests.rs`）；禁用探针（Go 返回空探针）在 Rust 无同形对象，禁用投影由组合根 `unavailable + reason` 表达
   （`futuapp/runtime_state_boundaries_test.go:44`、`coordinator_test.go:15/:109`）。
5. **bbgo/Wails 边界**：`liveapp` 4 行断言 bbgo 通知桥启停、`bbgo.notify` 名称等级映射与 sink panic→delivery failed、
   Wails live handler 选项对象，Rust 无对应运行时（通知由引擎 notification port 与 SSE/WS 投影承担，panic 语义由 `Result` 取代），
   属已退役边界保留。

验证记录：

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(runtime_status_wire_drops_absent_and_blank_values_and_normalizes_utc)'`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked -E 'test(market_data_health_requires_a_known_logged_in_quote_session) \| test(disconnected_probe_keeps_its_transport_error_for_manual_retry)'` | 1/1 与 2/2 通过 |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/s127p_payload.json`、`/tmp/s127p_bridge.json`、`/tmp/s127p_cmds.json` | 53 行处理 + 1 行引用纠正 + 86 行命令补全，`[x]` 1534 → 1538 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 3 → 2；Rust 测试 3204 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1545、unrecorded 0、stale 0、unknown 53 |
| 字段完整性 | 全量 4451 行检查 `command` 非空 | 空命令 0 行（补全 86 行 + 1 行引用纠正） |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu --all-targets --locked` | 542/542 通过（1 skipped） |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1876/1876 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过（`check:quick` 前按门禁执行 `cargo clean`：121430 文件 / 34.5GiB，`.rcgu.o` 0 个） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

第 127 批收尾结论：`internal/app/apiserver` 的 331 行待办全部给出终值（各分片提交 b858fc7f、386afddc、207f9211、
8e89b643、7439b61f、a73e9af1、c43362e9、322e7445、3a0027f1 与本分片），域内已无占位/未复核行，
未以数量比例宣称功能等价。

后续（第 128 批）：进入 `strategy_pine` 465 行（Pine 运行时、catalog、order/indicator 行为与策略生命周期），
随后 `assistant_workflow` 447 → `other` 311 → `backtest_calendar` 262 → `storage_sqlite` 178 → `marketdata_quotes` 155 →
`futu_opend` 104 → `settings_watchlist` 39。

## 第 128 批：`strategy_pine` 域

### 分片一：`internal/strategy/catalog` 20 行 + `instancebinding` 8 行 + `instanceview` 6 行

范围（按文件与行号升序）：`catalog/catalog_boundary_behavior_test.go:34/:58/:84/:123/:192`、
`catalog/instance_lifecycle_business_test.go:11/:89/:143/:189`、`catalog/plugin_normalization_business_test.go:13/:89/:99/:157/:186`、
`catalog/repository_failure_business_test.go:11/:19/:126`、`catalog/runtime_reconciliation_business_test.go:12/:80/:113`；
`instancebinding/binding_test.go:11/:28/:65/:96/:106/:143/:161/:202`；`instanceview/runtime_projection_test.go:9`、
`instanceview/view_test.go:13/:28/:46/:62/:92`。owner：`crates/jftrade-engine`（策略 catalog、实例绑定归一、实例视图与运行时端口）、
`crates/jftrade-store-sqlite`（策略实例/定义持久化与活动流）。

本分片新增 Rust 证据 2 项（1 行升 `[x]`）并修复 2 处功能差异：

1. `crates/jftrade-engine/src/strategy_runtime_port.rs::startable_requires_the_pinets_runtime_on_the_pine_v6_source_format`
   对应 Go `instanceview/view_test.go:46 TestStartableRequiresPineV6AndPineRuntime`。**功能修复**：Rust 原先把 startable 计算为
   “binding 同时带 runtime 与 sourceFormat 两个键”，与参照的“必须恰为 pine-pinets + pine-v6”不等价；现新增
   `startable_runtime` 助手按参照判定（legacy 值仍在 wire 中暴露，但不再可启动）。用例经实例 wire 断言四组 binding
   （pine-pinets+pine-v6 → true；legacy-runtime、legacy-source、缺键 → false）。先红探针：把判定改回“两键存在即真”后
   两条目标用例立即失败（`left: Bool(true)` vs false），按字节回滚后 `strategy_runtime_port.rs`
   shasum `f5f0268a06892d52de0e8fce904c201b0fd2064a2e599547919e91cc51764564` 前后一致。
2. `crates/jftrade-engine/src/product_production_ports_strategy_tests.rs::generated_instance_ids_use_the_definition_prefix_or_the_pine_runtime_default`
   对应 Go `instanceview/view_test.go:92 TestBuildInstanceIDUsesDefinitionOrDefaultPrefix`。**功能修复**：Rust 原先对空白
   definition id 会生成以连字符开头的实例 ID（`-20260922191453.166420000`），参照会退回 `pine-pinets` 前缀。
   现 `generate_instance_id` 对空白 id 使用 runtime 前缀，用例断言 definition-1 前缀 + 24 字符时间戳后缀形状与空白回退。
   先红探针：还原为直接使用 trim 后的 definition id 后空白分支失败，按字节回滚后
   `product_production_ports_strategy.rs` shasum `8623da4cb3ac9d5d2615a5517a33aea005a3c3dd207478a986c3f5c1306ce268` 前后一致。

映射结果：34 行复核（1 行升 `[x]`、8 行收紧引用与结论、25 行确认原 partial/boundary 终值仍准确）。
计数：`[x]` 1538 → 1539；partial 2304 → 2303；boundary 609；audit Rust 测试 3206；anchors 1547（unrecorded 0、stale 0、unknown 53）。

关键事实与新登记缺口（P2）：

1. **活动流降级语义（P2）**：Go 在 activity store 查询失败时仍返回 found=true 的空页（route ledger 记为 quirk，
   处置为硬切后修复）；Rust 的 `logs()`/`audit()` 在 store 失败时返回 `StrategyReadSnapshotError::Unavailable`（fail-closed 5xx），
   空页形状只在分页函数上断言。owner：`crates/jftrade-engine/src/strategy_runtime_port.rs`（如需逐字对齐需产品决策是否接受降级空页）。
2. **未知 chartType 处置差异（P2）**：Go 的 NormalizeBinding 保留受支持 chartType、未知值静默清空为 standard；
   Rust 归一为小写白名单并对未知值（renko）返回 400 BAD_REQUEST。owner：`crates/jftrade-engine/src/strategy_runtime_activity.rs`。
3. **ApplyParams 写回层缺失（P2）**：Go 把 symbol/executionMode/chartType/brokerAccount/instruments 写回 instance.params；
   Rust 的 params 投影只含 definitionId/runtime/sourceFormat，规范字段体现在 binding 上。owner：策略写入端口与实例视图。
4. **params→(runtime, sourceFormat) 与 definitionId trim helper 缺失（P2）**：参照有独立 helper（空白 runtime→pineworker.RuntimeID、
   nil→pine-v6、definitionId 取值并 trim）；Rust 的等价归一散落在定义解析与 binding 投影中，缺逐值断言。
5. **实例视图调用方隔离（P2）**：参照断言“改视图 params 不影响源实例”；Rust 的隔离由 JSON 序列化边界保证，缺直接断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过（架构检查先因本分片新增分支使 `product_production_ports_strategy.rs` 达 807 行超限，压缩前缀赋值后 800 行达标） |
| 目标用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(startable_requires_the_pinets_runtime_on_the_pine_v6_source_format) \| test(generated_instance_ids_use_the_definition_prefix_or_the_pine_runtime_default)'` | 3/3 通过（含同文件 includer 镜像用例） |
| 映射写入 | payload `/tmp/s128a_payload.json` 经 `/tmp/b82_apply.py` 应用 | 9 行给出终值，`[x]` 1538 → 1539、partial 2304 → 2303 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3206 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1547、unrecorded 0、stale 0、unknown 53 |
| 字段完整性 | 全量 4451 行检查 `command` 非空 | 空命令 0 行 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1879/1879 通过 |
| 产物清理 | `pnpm run clean:rust:artifacts` | `check:quick` 前按 target-health 要求清理 134599 文件 / 31.8GiB，`.rcgu.o` 83641 → 0 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过；`check:quick` 首轮在 ADK lifecycle `compacted_context_survives_restart_and_precedes_current_user_message` 用例上抖动失败（102/1909 中止），隔离复跑与次轮 `check:quick` 均通过，未记为通过前的抖动 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍在 cargo-deny advisories 阶段因 `deny.toml` 8 条 `advisory-not-detected` 失败（bans/licenses/sources ok） |

本分片结论：`catalog`、`instancebinding`、`instanceview` 34 行全部给出终值，其中 1 行升 `[x]`、8 行收紧结论、
25 行确认原 partial/boundary 判定仍准确；2 处功能差异已按参照修复并有先红探针与回归用例。

### 分片二：`internal/strategy/live_command_business_boundaries_test.go` 11 行

范围（按行号升序）：`:14 TestDefaultPineProducesEscapedCanonicalStarterStrategy`、`:33 TestServiceDelegatesDefinitionVersionHistoryWithIdentityPreserved`、
`:57 TestCommandsFromOrderIntentsRejectsInvalidAtomicOCOLegs`、`:85 TestWorkerIntentDirectionAliasesPreserveTradingSide`、
`:127 TestExecuteBarCommandsPreflightsBeforeBrokerSideEffects`、`:204 TestAtomicPineOrderValidationRejectsUnsafeGroupShapes`、
`:362 TestAtomicPineOrderSubmissionIsAllOrNothing`、`:399 TestPositionAwareCloseNeverCrossesTheWrongSide`、
`:468 TestIgnoredOrderWarningsRetainFallbackIdentityAndSymbol`、`:490 TestLiveOrderQuantityRespectsMinimumAndPrecision`、
`:545 TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings`。owner：`crates/jftrade-engine`（实时命令执行与意图归一，`strategy_runtime_execution.rs`）、
`crates/jftrade-backtest`（括号/原子组撮合校验）、`crates/jftrade-broker`（市场规则与手数归一）、`crates/jftrade-store-sqlite`（执行订单与定义历史）。

本分片 11 行全部给出终值：1 行升 `[x]`、6 行收紧 partial、4 行确认为 boundary（4 行均为既有 boundary 判定，本次逐条复核结构差异理由）：

- `[x]`：`:33` 委派身份保持。Rust 证据沿用 `product_production_ports_strategy_tests.rs::strategy_definition_versions_report_unknown_ids_and_keep_deleted_history`，
  该用例对真实 SQLite 定义库断言未知 id 返回 None、版本列表按 definition id 归属（definitionId 字段）、按 (definitionId, version) 读取不可变快照、未知版本 None、
  软删除后历史保留；本次在该测试补写 `// Parity:` 锚点指向本行 Go 测试，使代码侧与账本侧同时成立（anchors 1547 → 1548）。
- 确认为 boundary 的结构差异：`:14` Go 侧默认 Pine 起始模板生成（Rust 无默认模板生成器，从零创建由 Vue 设计器模板承担）；
  `:57` 与 `:204` 原子/OCO 腿校验（Rust 实时执行无 AtomicGroupID/OCOGroupID 腿模型，最近语义在回测括号输入校验）；
  `:362` 原子组提交全有或全无（Rust 无原子提交入口，最近语义为回测父括号原子撮合）。
- 收紧为精确缺口的 partial：`:85` 未知命令 kind 不在实时意图路径被拒（cancel/close/entry 之外落入 entry 分支），且缺细粒度方向/kind 错误文案；
  `:127` 缺整根 bar 预检（不支持 kind、空白取消 id、缺解析器都不预先失败，只有风险拒绝在下单前，见被引用的 risk 用例）；
  `:399` Rust close 按现仓符号平仓且不校验声明方向是否匹配，无仓位读取器与告警文案分支未复刻；
  `:468` 无 warning sink 与命令身份/符号回退格式化（仅 INTENT_SKIPPED 审计）；
  `:490` 缺最小数量/VolumePrecision 向下取整/负数量归一的逐值断言（最近语义在 broker 手数与 backtest 流动性告警）；
  `:545` 陈旧映射在 Rust 是硬错误（`strategy order <id> is not owned by instance <instance_id>`）而非容忍，且无别名去重表。

关键事实与新登记缺口（P1/P2）：

1. **陈旧映射硬错误（P1）**：Go 的按意图取消在别名指向缺失订单时不调用券商也不报错；Rust `dispatch_cancel_intent` 找不到归属订单直接返回错误。
   处置需产品决策（容忍陈旧 = 幂等取消语义 vs fail-closed），owner `crates/jftrade-engine/src/strategy_runtime_execution.rs`，回归要求先建 no-op 成功用例。
2. **整根 bar 预检缺失（P1）**：Go 在任何券商副作用前完成整批校验；Rust 逐条内联执行，未知 kind 不拒绝、空白取消 id 不校验。
   owner 同上，回归要求先建断言 0 笔券商调用的失败用例再实现两阶段处理。
3. **显式方向平仓语义差异（P2）**：Go 的 close 尊重声明的 long/short 方向（方向与现仓不匹配时仅告警），Rust 按现仓符号平仓。
   owner 同上；当前 Rust 语义与前端发送的 close 意图一致，是否对齐取决于产品对显式方向平仓的定义。
4. **忽略/告警通道缺失（P2）**：Go 的 warnIgnoredOrder 保留 FromEntry/<anonymous> 回退身份与 <unknown> 符号；Rust 只有审计事件，缺身份格式化。
5. **数量归一缺口（P2）**：低于最小数量、无步长时按精度向下取整、负数量归一为 0 在实时路径无逐值断言；owner `crates/jftrade-broker` + engine 执行端口。
6. **未知命令 kind 白名单（P2）**：Rust 实时意图路径未按 kind 白名单拒绝 replace 等未知命令，需先红用例再补。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | payload `/tmp/s128b_payload.json` 经 `/tmp/b82_apply.py` 应用 | 11 行给出终值，`[x]` 1539 → 1540、partial 2303 → 2302 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3206 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1548、unrecorded 0、stale 0、unknown 53 |
| 目标用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(strategy_definition_versions_report_unknown_ids_and_keep_deleted_history)'` | 1/1 通过（新增锚点后复跑） |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1879/1879 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过；`check:quick` 首轮在 `product_api_launcher_lifecycle::api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 抖动失败（1840/1909 中止），隔离复跑与次轮 `check:quick` 均通过，按抖动处置并记录 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok） |

### 分片三：`internal/strategy/liveruntime` 边界族 21 行

范围（按文件与行号升序）：`manager_boundaries_test.go:16/:54/:94/:116/:155/:182/:216/:241/:275/:302/:356`、
`manager_close_test.go:86/:140/:188`、`nil_boundaries_test.go:10/:43`、`order_risk_business_test.go:18/:67/:135/:216/:235`。
owner：`crates/jftrade-engine`（运行时管理器、任务生命周期、执行与风险上下文）、`crates/jftrade-trading`（运行时风险原因码）、
`crates/jftrade-calendar`（市场日起点）、`crates/jftrade-store-sqlite`（每日订单计数）、`crates/jftrade-strategy`（notify-only 协调器）。

本分片 21 行全部给出终值：2 行升 `[x]`、18 行收紧 partial、1 行确认为 boundary。

- `[x]`：`order_risk_business_test.go:18`。新增
  `crates/jftrade-engine/src/strategy_runtime_execution_tests.rs::strategy_intents_place_stop_market_orders_with_the_stop_price_and_reduce_only_flag`，
  断言止损市价单经执行端口时 `orderType==STOP`、`stopPrice==95.25`、payload 不含 `price`、`reduceOnly==true`，与 Go 的
  `TestLiveOrderPassesStopPriceToExecutionGateway` 四项断言逐条对应（实现：`dispatch_place_order` 仅在 `has_limit_price` 时写 `price`）。
- `[x]`：`order_risk_business_test.go:67`。既有 `runtime_risk_enforce_applies_close_only_quantity_notional_and_daily_limits` 已被另一 `[x]` 行占用，
  故新增 `crates/jftrade-trading/tests/risk_engine_tests.rs::runtime_risk_reason_codes_match_the_live_executor_table`，逐条复刻 Go 的原因码表
  （`close_only`、`close_only_insufficient_position` ×2（数量 5/6）、`max_order_notional`、放行、关闭 closeOnly 后 `max_order_quantity`），
  与 `runtime_reject_reason` 的同名原因码一致。
- boundary：`nil_boundaries_test.go:10`（Go 的 nil 接收者防御分支，Rust 由所有权/Option 在编译期排除，空状态语义由状态投影覆盖）。
- 收紧为精确缺口的 partial（18 行）覆盖三类：管理器维护态与轮询间隔（`:16` 无维护忙原因与 `closedKLineSyncInterval`）、
  能力与健康门禁（`:94` `streaming_candles` 仅存在于测试夹具、`:116` 启动路径不做非健康 provider 拒绝）、
  输入加载与构建边界（`:54`/`:155`/`:182`/`:216`/`:241`/`:275`/`:302`/`:356`）、关闭语义（`:86` 会话错误落 `SESSION_CLOSE_FAILED` 审计而非聚合返回、`:140` 无 `ErrClosed` 在途激活拒绝断言、`:188` 单任务顺序天然保证但无直接断言）、
  风险与取消（`:135` 无失败保留跟踪与未跟踪忽略断言）、市场日与计数窗口（`:216` helper 无按市场逐值断言、`:235` 计数窗口用 UTC 零点而非市场日）。

关键事实与新登记缺口（P1/P2）：

1. **streaming_candles 能力门禁缺失（P1）**：Go 在 live/notify_only 启动前拒绝未声明流式 K 线的 provider；Rust 的 `ProviderCapabilities.streaming_candles`
   只出现在夹具中，生产启动路径不校验。owner `crates/jftrade-engine/src/strategy_runtime.rs`，回归要求先建拒绝用例。
2. **活动 provider 健康门禁缺失（P1）**：Go 在活动 provider 非健康时拒绝启动，除非显式 exchange 覆盖（覆盖时零健康调用）；Rust 只有就绪标志与探针真实性用例。
3. **每日订单计数窗口为 UTC 零点（P1）**：`strategy_runtime_execution.rs` 用 `OffsetDateTime::new_utc(date, MIDNIGHT)` 作为 `count_daily_orders` 起点，
   未接入 `jftrade-calendar::market_day_start_for_market`；Go 按标的市场日计数。
4. **关停错误聚合语义（P2）**：Rust `shutdown()` 返回 bool 并在 5s 期限内汇合任务，会话关闭失败只写 `SESSION_CLOSE_FAILED`/`SESSION_CLOSE_TIMEOUT` 审计（键 `strategy:<instance>:<symbol>`），
   没有 Go 的聚合错误返回与 12 并发调用断言。
5. **维护态与轮询配置缺失（P2）**：无维护忙原因字符串、无 `closedKLineSyncInterval` 配置（Rust 由任务事件循环推进）。
6. **事件类型差异（P2）**：Go 的 `runtime_error`/`order_ignored` 事件在 Rust 对应 `RUNTIME_EXITED`/`INTENT_SKIPPED`/`SESSION_CLOSE_*` 审计，缺空白错误忽略规则。
7. **live 绑定五分支校验缺逐条断言（P2）**：`brokerId`/`accountId`/`tradingEnvironment`/`market` 缺失文案未与 Go 逐条对照。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(strategy_intents_place_stop_market_orders_with_the_stop_price_and_reduce_only_flag)'` | 3/3 通过（三个测试目标） |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading --all-targets --locked -E 'test(runtime_risk_reason_codes_match_the_live_executor_table)'` | 1/1 通过 |
| 映射写入 | payload `/tmp/s128c_payload.json` 经 `/tmp/b82_apply.py` 应用 | 21 行给出终值，`[x]` 1540 → 1542、partial 2302 → 2300 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3208 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1550、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading --all-targets --locked` | 85/85 通过（含新增原因码用例） |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1882/1882 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过（本分片 `check:quick` 首轮通过，`.rcgu.o` 29340 未触发清理） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok） |

### 分片四：`pineworker_live_business_test.go` 6 行 + `product_lifecycle_business_test.go` 4 行 + `pine_live_executor_test.go` 24 行

范围（按文件与行号升序）：`liveruntime/pineworker_live_business_test.go:226/:270/:305/:368/:404/:527`、
`liveruntime/product_lifecycle_business_test.go:18/:56/:96/:151`、`pine_live_executor_test.go:14/:36/:66/:80/:93/:119/:150/:180/:198/:226/:255/:284/:314/:349/:383/:428/:443/:462/:474/:508/:532/:556/:601/:648`。
owner：`crates/jftrade-engine`（实时意图执行、数量归一、会话与运行时账户）、`crates/jftrade-integration-pine`（会话 open/append/close 契约与 worker 错误映射）、
`crates/jftrade-broker`（市场规则/手数）、`crates/jftrade-backtest`（括号撮合，边界项）。

本分片 34 行全部给出终值：4 行升 `[x]`、27 行收紧 partial、3 行确认为 boundary。

- `[x]`（4 行，均为本分片新增用例，与 Go 用例同形同值）：
  `:80` `live_entry_without_quantity_is_rejected_before_any_broker_submission`（REAL 绑定 + entry 无数量 → 报 requires a positive finite quantity 且执行端口零调用；
  重要事实：SIMULATE 绑定允许缺省数量 1，属离线模拟语义，用例内已注释区分）；
  `:93` `live_entry_quantity_percent_sizes_from_available_equity_at_the_fallback_price`（权益 1000、现价 100、50% → 5 股，side BUY）；
  `:119` `live_close_quantity_percent_sizes_from_the_open_position`（现仓 10、50% → 5 股，side SELL，reduceOnly true）；
  `:150` `live_close_without_quantity_defaults_to_the_full_position`（现仓 3、无数量 → 3 股，reduceOnly true）。
- boundary：`pine_live_executor_test.go:462/:474/:508`（Go 的实时原子 OCO 括号三条，Rust 实时执行无括号路径，最近语义在回测 matcher 括号输入/原子执行）。
- 收紧为精确缺口的 partial（27 行）覆盖五类：会话与预热（`pineworker_live :226/:270/:404` 的 open 携带预热根数、全量重跑为 0、append 计数、重复 open 边界无断言）、
  市场与告警（`:226/:255/:284/:314` 的最小量/整手/规则缺失/告警聚合在实时路径无实现）、
  数量与错误语义（`pine_live_executor :36` 缺 timeInForce、`:66` 缺仓位定量时报错 vs 跳过、`:532/:556/:648` 错误文案与未知 kind 边界）、
  方向与跟踪（`:180` 无空头 Tag、`:349/:383` 显式方向平仓差异延续分片三结论、`:428/:443/:601` 取消与 clientOrderId 形状只有部分断言）、
  快照与观测（`product_lifecycle :18/:56/:96/:151` 的 nil funds、失败后观测排序、空币种余额无专门断言）。

关键事实与新登记缺口（P1/P2）：

1. **实时最小量/整手守卫缺失（P1）**：Go 在执行器内按市场最小量与港股整手忽略订单；Rust 实时路径只按绑定 `lotSize` 向下取整，最小量与港股 board lot 只在 `jftrade-broker` 手数初始化与回测流动性告警中体现。
2. **`timeInForce` 未进 wire（P1）**：Go 断言限价单 GTC；Rust 的 `ExecutionWriteInput` payload 不含 `timeInForce`，券商侧取默认值。
3. **告警通道缺失（P2）**：忽略订单在 Rust 只落 `INTENT_SKIPPED` 审计，无 warning sink、无按原因聚合计数（延续分片二 :468 与分片三 :527 登记）。
4. **缺省数量语义差异（P2）**：SIMULATE 绑定下 entry 无数量默认 1；REAL 绑定报错（与 Go live 执行器一致），已在用例中显式区分。
5. **会话预热计数无断言（P2）**：`jftrade-integration-pine` 的 open/append/close 修订契约有测试，但缺 warmup→open（2 根）→append（1 根）→close（1 次）与 session id 形状断言。
6. **未知命令 kind 白名单（P2）**：延续分片二登记，`replace` 之类落入 entry 分支。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(live_entry_quantity_percent_sizes_from_available_equity_at_the_fallback_price) \| test(live_close_quantity_percent_sizes_from_the_open_position) \| test(live_close_without_quantity_defaults_to_the_full_position) \| test(live_entry_without_quantity_is_rejected_before_any_broker_submission)'` | 12/12 通过（四条用例 × 三个测试目标；缺数量用例首写为 SIMULATE 绑定导致未拒绝，改为 REAL 绑定后转绿并记录该语义差异） |
| 映射写入 | payload `/tmp/s128d_payload.json` 经 `/tmp/b82_apply.py` 应用 | 34 行给出终值，`[x]` 1542 → 1546、partial 2300 → 2296 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3212 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1554、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过；`check:quick` 首轮在 `api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 抖动失败（1872/1924 中止），隔离复跑与次轮 `check:quick` 均通过，按抖动处置并记录（`.rcgu.o` 32811 未触发清理） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok） |

### 分片五：`pkg/strategy/indicatorbinding` 解析族 27 行

范围：`parse_test.go` 22 行（`:10/:93/:152/:176/:212/:235/:283/:304/:347/:384/:395/:425/:433/:467/:475/:501/:509/:542/:574/:609/:666/:720`）、
`parse_semantics_test.go` 5 行（`:8/:20/:46/:114/:155`）。owner：`crates/jftrade-strategy`（Pine 解析器与 planner、信号校验）、
`crates/jftrade-engine/src/strategy_runtime_execution.rs`（数量语义）、`crates/jftrade-trading/src/risk.rs`（运行时风险）。

本分片 27 行全部给出终值：17 行收紧 partial、10 行确认为 boundary（Go 指标绑定 DSL 专属助手，Rust 已退役该 DSL）。

**功能修复（1 处，带先红探针）**：`crates/jftrade-strategy/src/pine/planner.rs`

- 差异：`security_inner_binding` 缺均线分支，`request.security(syminfo.tickerid, "<tf>", ta.ema(close, 14))` 在 Rust 退化为不透明键
  `security:syminfo.tickerid:"1":ta.ema(close,14)`；Go 的 `planner_indicator.go`/`BuildMovingAverageKeyWithSource` 会产出可解析的
  `ma:<TYPE>:<len>[:<source>]` 加时间单位后缀（产品侧 pinespec 金标也期望 `ma:EMA:5:15m`）。
- 修复：新增 `ema|sma|rma|wma|hma|vwma` 分支返回 `("ma", [TYPE, length, source?])`，并在 security 包装处按 Go 键序把时间单位插入到 source 之前
  （`ma:EMA:14:hour:hlc3`）。
- 先红探针：两条新增集成用例在去掉分支后 2/2 失败（键为 `ma:EMA:14` 与 `security:...`），恢复修复后 3/3 通过；
  `planner.rs` shasum `84dce8fa6d06efe54209d72059f83b7c02b1c02faae45ce4eb4ca59bf158092d` 在探针前后一致。
- 新增证据：`crates/jftrade-strategy/tests/pine_indicator_binding_keys.rs::moving_average_keys_carry_type_period_and_security_time_unit`
  （`ma:EMA:14:minute`、`ma:SMA:5:day`）与 `::moving_average_keys_append_the_requested_source_and_time_unit`
  （`ma:EMA:14`、`ma:SMA:21:volume`、`ma:EMA:14:hour:hlc3`）；单测
  `crates/jftrade-strategy/src/pine/planner.rs::indicator_time_unit_keeps_the_pine_timeframes_that_reach_indicator_keys` 逐值锁定时间单位表。

关键事实与新登记缺口（P1/P2）：

1. **时间单位词表差异（P1）**：Go 的 DSL `ParseIndicatorTimeUnitValue` 接受 m/min/hour/day/week/mo 词形与 bar(s)/空白空单位语义；
   Rust `indicator_time_unit` 是 Pine 时间框架子集。已实测差异：Rust 接受 `0m`/`001m`/`15`（Go 拒绝或语义不同）、`60m` 归 `60m`（Go 归 `hour`）、引号 `"15"` 归 `15m`（Go 拒绝）。
2. **均线类型表差异（P1）**：Go 支持 MA/EMA/SMA/SMMA/LWMA/TMA/EXPMA/HMA/VWMA/BOLL；Rust 只识别 ta.sma/ema/rma/wma/hma/vwma，无 MA/BOLL/TMA/EXPMA，也无“未知→MA”归一。
3. **价格源白名单缺失（P2）**：Go 的 ParsePriceSource 只接受 8 个源并在建键时丢弃非法源；Rust 原样使用实参文本（`ma:EMA:14:day:bad` 这类键可能出现）。
4. **数组形态 MTF 均线未展开（P2）**：pinespec 金标里的 `[a, b] = request.security(..., [close, ta.ema(close, 5)])` 形式在 Rust 仍退化为 `security:` 键（本次修复只覆盖单调用形态）。
5. **DSL 助手族整体退役（boundary）**：函数调用解析、参数切分、函数名归一、参数元数契约、整数转字符串、括号配对、保护窗口策略与四张归一缺省表在 Rust 无同形对象；
   升级路径统一为“若将来兼容导入 Go DSL，按原表逐条移植并复刻拒绝形态”。
6. **数量/保护语义映射（P2）**：Go 的数量模式表（account_position_percent/symbol_position_percent/amount/shares 及别名）与保护模式/方向表，在 Rust 分别由 quantity/quantityPct 与运行时风险声明（off/monitor/enforce + allow_entry）承担，无别名归一与逐项断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked -E 'test(moving_average_keys_carry_type_period_and_security_time_unit) \| test(moving_average_keys_append_the_requested_source_and_time_unit)'` | 2/2 失败（键退化为 `security:`，去掉均线分支后复现） |
| 新增用例 | 同上（含新增单测） | 3/3 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 48/48 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs` | 修复文件与探针副本均为 `84dce8fa…`，按字节一致 |
| 映射写入 | payload `/tmp/s128e_payload.json` 经 `/tmp/b82_apply.py` 应用 | 27 行给出终值，partial 2296 → 2286、boundary 609 → 619（`[x]` 1546 不变） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3212 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1558、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2943 files）、`pnpm run check:quick` | 全部通过（本分片 `check:quick` 首轮通过，`.rcgu.o` 42374 未触发清理） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok） |

### 分片六：`pkg/strategy/indicatorwarmup` 预热与需求键族 28 行

范围（按账本键取值）：`parser_validation_test.go:5/:55`、`risk_specification_rejection_test.go:5`、`spec_parse_business_test.go:11/:135/:198/:215`、
`spec_parse_invalid_test.go:8/:60/:137/:203`、`spec_sort_business_test.go:8/:66`、`warmup_internal_test.go:10/:38/:50/:62/:110`、
`warmup_plan_test.go:12/:40/:74/:98/:121`、`warmup_script_test.go:14/:51/:67/:96/:103`（`:142 TestResolveIntervalMinutesSupportsBrokerIntervalsAndSafeFallbacks` 已被既有 `[x]` 认领，不重复）。
owner：`crates/jftrade-strategy`（Pine planner 的 warmup/需求计划、`indicator_time_unit`、MTF 周期校验、运行时风险声明）；对照方为 `crates/jftrade-engine` 的预热入口与 `workers/pinets` 的需求消费。

修复（1 处功能差异，带先红探针）：

- 差异：`request.security(sym, "15m", ta.ema(close, 20))` 这类调用在 Go 侧只产出一个需求键 `ma:EMA:20:15m`；Rust 的 `visit_expr` 在收集包装键之外，又把第 3 实参里的内层 `ta.ema` 当成独立需求收集，计划里同时出现 `ma:EMA:20` 与 `ma:EMA:20:15m`，导致预热根数被按较短键抬高。
- 修复：`crates/jftrade-strategy/src/pine/planner.rs` 的 `Call` 分支新增 `wrapped_by_security` 判定，当 `request.security` 包装可解析时跳过内层调用收集，键集合与 Go 一致。
- 先红探针：注释掉该判定后，`request_security_timeframes_validate_against_the_strategy_interval` 与同族键用例复现重复键（计划为 `ma:EMA:20`、`ma:EMA:20:15m`），恢复后 3/3 通过；`planner.rs` shasum `a1997ffb77e91b4cb3aba45d7207ac73730f50319aff65bf883489b69a3cf059` 在探针前后一致。
- 新增证据：`crates/jftrade-strategy/tests/pine_indicator_warmup.rs` 七个用例——`warmup_bars_use_the_largest_indicator_requirement`（1m US 下 `ta.sma(close,5)` + `request.security(...,"D",ta.sma(close,20))` + `ta.macd` → 7800）、
  `warmup_bars_follow_market_trading_profiles`（US 20*390、HK 20*330、SH/SZ 20*240）、
  `warmup_bars_use_the_extended_trading_day_when_enabled`（US 扩展时段 5*24*60=7200）、
  `warmup_bars_omit_a_runtime_series_floor`（1m `ta.sma(close,5)` → 5，不抬高到运行时序列下限）、
  `warmup_bars_from_a_script_match_the_plan_for_extended_hours`（脚本入口 20*1440=28800）、
  `warmup_bars_fall_back_to_the_generic_calendar_for_unknown_symbols`（未识别前缀回退 390 分钟/天，月线 5m 间隔 → 1560）、
  `request_security_timeframes_validate_against_the_strategy_interval`（键恰为 `ma:EMA:20:15m`；1m 通过，`1h`/`1d` 报 fixed timeframe 低于策略周期）。
- 映射终值：本分片 28 行 = 7 `function_exact` + 17 `partial` + 4 `boundary`；`[x]` 1546 → 1553、partial 2286 → 2280、boundary 619 → 618（其中 5 行原 boundary 收紧为 1 partial + 4 boundary）。

关键事实与新登记缺口（P1/P2）：

1. **固定周期校验只覆盖两族（P1）**：Go 的 `validateFixedTimeframeRequirements` 逐族校验 ma/security_source/rsi/stdev/variance/stoch/cci/mfi/advanced；Rust 的 `validate_timeframe_alignments` 只对 `security` 与 `ma` 两族生效，其余族即使声明了非法固定周期也不会在校验期被拒（`:62` 用例即为此收紧的 partial）。
2. **需求键解析器整体缺失（P1）**：Go 有完整的“键字符串 → 类型化配置”解析与严格模式（`parser_validation`、`spec_parse_*` 共 10 行），Rust 的键只由 planner 生成、由 worker 目录消费，没有反向解析入口，也没有严格模式开关与逐族 invalid 文案；历史键再解析（迁移后遗留键）暂无对应实现。
3. **高级指标 lookback 语义未逐 kind 对齐（P1）**：Go 逐 kind 断言 warmup 语义（`anchored_vwap`→1、`pivothigh(4,3)`→9、`linreg(20,2)`→24）；Rust 的 `estimated_lookback_bars_with_session` 对高级族走“取参数最大值”的通用估算，未按 kind 复刻这些常量，`ta.linreg` 的 offset 语义尤其未对齐（`:50` 登记为缺口）。
4. **背离与保护回看族缺失（P1）**：Go 的 `divergence_*`、`protect` 回看计入预热（`:10` 的 11 类窗口、`:121` 的 divergence_top(signal,8)+protect 2 小时）；Rust 的风险面是声明式限额，没有背离回看与保护窗口需求族，换算无语义对象（`:10` partial、`:121` boundary）。
5. **配置排序层缺失（P2）**：Go 按业务优先级 + tie-breaker 排序指标/风险配置（`spec_sort_business` 两行）；Rust 用有序映射去重，顺序稳定但不按业务优先级，也没有配置排序层，升级路径为“若下游需要固定业务优先序，在计划投影处加排序并复刻断言”。
6. **风险规格校验面不同（P2）**：Go 拒绝畸形时间窗与策略契约（`quarter` 单位、非法 window policy 组合）；Rust 的风险声明只校验模式（off/monitor/enforce）、`closeOnly` 与限额，时间窗/策略契约无对应输入面（`:5` 登记为缺口）。
7. **周期标签格式化已退役（P2）**：Go 的 `formatFixedTimeframeLabels` 为展示层文案；Rust 以分钟/枚举表达周期，展示由前端与 wire DTO 承担，如需同形标签应在策略预览投影处补格式化并复刻断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked -E 'test(request_security_timeframes_validate_against_the_strategy_interval)'` | 期望键为 `ma:EMA:20:15m` 时实际出现 `ma:EMA:20` 与 `ma:EMA:20:15m` 双键，用例失败（去掉 `wrapped_by_security` 判定可复现） |
| 新增用例 | 同上（七条 `warmup_bars_*`/`request_security_*` 过滤） | 7/7 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 55/55 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs` | 修复文件与探针副本均为 `a1997ffb…`，按字节一致 |
| 映射写入 | payload `/tmp/s128f_payload.json` 经 `/tmp/b82_apply.py` 应用 | 28 行给出终值，`[x]` 1546 → 1553、partial 2286 → 2280、boundary 619 → 618 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3222（Strategy 域 201） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1565、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2944 files）、`pnpm run check:quick` | 全部通过；`check:quick` 首轮在 `check:rust:workspace` 的目标健康检查处以 `.rcgu.o` 57456 ≥ 50000 中止（确认无 Cargo 进程后 `pnpm run clean:rust:artifacts` 清理 118027 文件/33.3 GiB，复跑 workspace 3342/3342 通过），次轮 `check:quick` 记录两处并行负载抖动：`check:compatibility:provider-runtime`（Node 加载器崩溃，隔离复跑通过）与 `adk_session_detail_omits_resolved_approval_groups`（1 failed，隔离复跑 3/3 通过），第三轮 `check:quick` 仅剩已知的 `check:rust:static` 失败，其余全部通过（.rcgu.o 已在清理后重建） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok） |

后续：分片七进入 `pkg/strategy/ir/planner*` 29 行（`planner_test.go` 11、`planner_internal_boundaries_test.go` 6、`planner_business_boundary_test.go` 4、`planner_branch_test.go` 2、`planner_indicator_matrix_test.go` 3、`planner_internal_test.go` 3），随后 `pkg/strategy/pine/parse_*` 41 行，直至 `strategy_pine` 域 510 行清空。

### 分片七：`pkg/strategy/ir/planner` 计划器键形与运行期标志 29 行

范围：`planner_test.go` 11（`:10/:51/:62/:73/:85/:100/:122/:204/:254/:265/:310`）、`planner_internal_test.go` 3（`:5/:88/:122`）、
`planner_internal_boundaries_test.go` 6（`:9/:57/:101/:131/:188/:217`）、`planner_business_boundary_test.go` 4（`:10/:42/:100/:165`）、
`planner_indicator_matrix_test.go` 3（`:8/:55/:147`）、`planner_branch_test.go` 2（`:9/:100`）。
owner：`crates/jftrade-strategy`（Pine planner 的需求键与运行期标志；Go 侧等价物是 `pkg/strategy/ir` 的 DSL 语句计划器，Rust 侧输入面为 Pine 源码）。

修复（5 处功能差异，共用一次先红探针）：

- 差异 1（CCI 遗留源）：Go 的 `parseSourcePeriodBinding(..., "hlc3", "20")` 让 `ta.cci(hlc3, 20)` 产键 `cci:20`、`ta.cci(close, 20)` 产键 `cci:close:20`；Rust 原按「close 才丢源」的统一规则产出 `cci:hlc3:20` / `cci:20`，与 Go 断言（`cci:hlc3:20` 必须缺席）相反。
- 差异 2（窗口族丢源）：Go 的 `parseWindowBinding` 始终保留源（`mom:close:5`、`rising:close:3`、`sum:volume:20`），Rust 原对 close 源做丢弃，产出 `mom:5`、`rising:3`，与 pinespec 金标 `mom:close:3` 冲突。
- 差异 3（stdev 遗留源）：Go 的 `stdev` 与 `rsi` 一样以 close 为遗留源（`stdev:5`），Rust 原产出 `stdev:close:5`。
- 差异 4（Williams %R 键名）：Go 的 `parseWilliamsRBinding` 把键写成 DSL 名（`williamsr:14`），Rust 原产出 `wpr:14`；同时新增 `ta.williams_r` / `ta.williamsr` 别名入口。
- 差异 5（运行期标志只认调用参数）：Go 在 `recordExpressionRequirements` 里扫描任意表达式（赋值、条件、循环条件），命中 `position_size`/`position_avg_price` 置 RequiresPosition、命中 `equity` 置 RequiresTotalAccountValue；Rust 原仅在 order/exit 调用参数上识别，实测赋值语句 `stopPrice = strategy.position_avg_price * 0.95` 与 `balance = strategy.equity` 都不置位。
- 实现：`crates/jftrade-strategy/src/pine/planner.rs` 新增 `legacy_source_for`（按族返回 close/hlc3）与 `note_runtime_variable`（标识符与成员访问两处调用），并拆出 `ta.stdev`、`ta.wpr|ta.williams_r|ta.williamsr` 两个分支。
- 先红探针：把 `planner.rs` 回滚到修复前版本后，新增测试文件 4/4 失败（CCI/窗口键、stdev 键、williamsr 键与两个标志全部不满足）；恢复修复后 4/4 通过。修复前 shasum `a1997ffb77e91b4cb3aba45d7207ac73730f50319aff65bf883489b69a3cf059`（等于分片六提交值），修复后 `f61ae3c0347d06d8a894bba17bf843391a2d4843bbba0bbf2f1ebfd1a272a486`，探针前后 `planner.rs` 与 `/tmp/probe_s128g_planner_after.rs` 按字节一致。
- 新增证据：`crates/jftrade-strategy/tests/pine_planner_requirement_keys.rs` 四条用例——`legacy_and_explicit_sources_keep_the_planned_keys`（编译 Go 的 Source Keys 脚本，集合相等断言 7 键并逐条断言 `ma:SMA:20:close`/`rsi:close:14`/`cci:hlc3:20` 缺席）、
  `window_and_oscillator_keys_keep_the_requested_source`（13 键：`mom:close:5`、`roc:close:12`、`rising:close:3`、`falling:close:3`、`sum:volume:20`、`change:close:1`、`highest:high:20`、`lowest:low:10`、`stdev:20`、`stdev:hlc3:11`、`cci:20`、`rsi:14`、`williamsr:14`）、
  `position_variables_in_expressions_require_position_data`（赋值 + 条件两种语句形态置位 requires_position）、
  `account_value_usage_in_statements_requires_total_account_value`（赋值里的 `strategy.equity` 置位 requires_total_account_value 与 requires_position）。
- 映射终值：29 行 = 2 `function_exact` + 23 `partial` + 4 `boundary`（分片七把 4 行结构差异从 partial 改判为 boundary）；`[x]` 1553 → 1555、partial 2280 → 2274、boundary 618 → 622。

关键事实与新登记缺口（P1/P2）：

1. **参数校验整体缺失（P1）**：实测被静默接受的非法输入包括 `ta.ema(close, nope)`→`ma:EMA:nope`、`ta.ema(close, 0)`、`ta.stdev(close, 0)`→`stdev:0`、`ta.rsi(close, 0)`→`rsi:0`、`ta.atr(0)`、`ta.macd(0, 26, 9)`、`ta.kc(close, 20, 1.5, maybe)`→`kc:close:20:1.5:maybe`、`ta.percentile_nearest_rank(close, 20, 101)`、`ta.linreg(close, 20, -1)`→`linreg:close:20:Negate1`、`ta.percentile_linear_interpolation(close, -1, 80)`→`...:Negate1:80`；Go 对同类输入一律拒绝或忽略（零周期不产生需求）。owner：`crates/jftrade-strategy/src/pine/planner.rs`；回归要求：按族补正整数/百分比/布尔/lookback/offset 校验与 Go 详情文案，并断言不再出现 `Negate` 形态键。
2. **价格源白名单缺失（P1，沿用分片五 P2 升级）**：`ta.highest(close - open, 20)` 实测 ok=true 且产出畸形键 `highest:(close Subtract open):20`，`ta.rsi(spread, 14)` 产出 `rsi:spread:14`；Go 的 `ParseOHLCVSource`（窗口族）与 `ParsePriceSource`（振荡族）会拒绝并给出可用源列表。
3. **高级指标元数契约缺失（P1）**：`ta.tsi(close, 13, 25, day, extra)`、`ta.linreg(close, 20, 1, day, extra)`、`ta.percentile_nearest_rank(close, 20, 80, "D", extra)` 均被接受并静默忽略尾参；Go 一律报 invalid argument count（`:101`）。
4. **MTF 内层表不完整（P1）**：`security_inner_binding` 只覆盖 ma/linreg/obv/pivot/kc/kcw/alma/cmo/dev/median/percentrank/tsi/correlation/percentile_*/swma；`request.security(...,"D",ta.macd(close,12,26,9))` 实测输出图内键加退化 `security:syminfo.tickerid:"D":...` 键，而 Go 只给 `macd:close:12:26:9:day`；`ta.rsi`/`ta.atr` 同样退化。
5. **整族缺失（P1）**：kdj、bollinger（含 `ta.bb`/`bbw`）、stoch、dmi、supertrend、sar、anchored_vwap、cum、security_source、highestbars/lowestbars、背离（`divergence:*`）与 protect（`sl:*`/`risk:*`）在 Rust 无需求键实现；其中 `ta.stoch`/`ta.cum`/`ta.anchored_vwap` 报 PINE_CALL_UNSUPPORTED，而 `ta.cog`/`ta.bbw` 既不产键也不报诊断（静默忽略，需补白名单或实现）。
6. **input 缺省值未回填进键（P2）**：pinespec 金标 `golden-udf-static-for` 期望 `ma:EMA:3`，实测 Rust 产出 `ma:EMA:len`（`len = input.int(3, "Length")` 未解析为常量）。
7. **数量模式枚举缺失（P2）**：`strategy.entry(..., qty_type="bananas")` 实测编译通过；Go 的 QuantityMode 表（fixed/cash_percent/account_position_percent/shares/amount 等）与别名归一未迁移。
8. **IR 结构差异（boundary）**：分支局部别名作用域、可扩展 IR 语句类型与 `Kind()/SourceRange()` 契约、protect/divergence 键构造、nil 程序文案（Rust 以 PINE_VERSION_REQUIRED/PINE_STRATEGY_REQUIRED 表达）在 Rust 无同形对象，升级路径写在账本结论里。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_planner_requirement_keys --locked --no-fail-fast` | 4/4 失败（回滚 `planner.rs` 到 `a1997ffb…` 后复现：CCI/窗口键、stdev、williamsr、两个标志） |
| 新增用例 | 同上（修复后） | 4/4 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 59/59 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs` | 修复后 `f61ae3c0…`，与探针副本按字节一致；修复前副本为分片六的 `a1997ffb…` |
| 映射写入 | payload `/tmp/s128g_payload.json` 经 `/tmp/b82_apply.py` 应用 | 29 行给出终值，`[x]` 1553 → 1555、partial 2280 → 2274、boundary 618 → 622（逐行核对 `git show d9155270:docs/history/go-to-rust/manual-test-mappings.json` 与工作树的 evidence_type 差异为 6 行） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3226（Strategy 域 205） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1569、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2945 files）、`pnpm run check:quick` | 全部通过；本分片 `check:quick` 走 affected 计划（6 个受影响文件、rust 模块、nextest -p jftrade-desktop -p jftrade-engine -p jftrade-strategy），1983/1983 通过并以 exit 0 结束 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy`、`cargo deny check advisories` | 在目标健康检查处以 `.rcgu.o` 51902 ≥ 50000 中止（确认无 Cargo 进程后执行 `pnpm run clean:rust:artifacts`）；绕过健康检查直接跑 `cargo deny check advisories` 仍报 8 条 `advisory-not-detected`（advisories FAILED、bans/licenses/sources ok），与分片六结论一致 |

后续：分片八进入 `pkg/strategy/pine/parse_*` 41 行，随后 `pkg/strategy/pine` 其余文件、`pkg/strategy/pineworker`、`pkg/strategy/pineengine` 与 pinespec，直至 `strategy_pine` 域 510 行清空。

### 分片八：`pkg/strategy/pine/parse_test.go` 诊断与订单元数据 35 行

范围：`parse_test.go` 35 行（`:10/:54/:84/:113/:126/:145/:167/:186/:202/:232/:253/:286/:317/:348/:367/:390/:432/:448/:464/:488/:560/:587/:621/:651/:690/:718/:732/:763/:790/:838/:903/:977/:1014/:1029/:1044`）。
owner：`crates/jftrade-strategy`（Pine 编译/校验/分析入口与诊断面）；对照方为 `workers/pineworker` 与 `crates/jftrade-integration-pine`（订单 intent 语义投影）。

复核方式：本轮把 35 行全部重读 Go 断言并逐行定位 Rust 证据。其中 21 行既有 `[x]` 通过 audit 复核（rust_entry 存在、指向真实测试、锚点无 stale/unrecorded），并抽检 `:10`（IR 形状）、`:126`（默认数量）、`:464`（高级订单诊断码）、`:1044`（字符串字面量不算历史引用）四条内容与 Go 断言一致；14 行非 `[x]` 逐条核对缺口描述，其中 3 行在本轮给出新终值。

修复（1 处功能差异，带先红探针）：

- 差异：`pine::compile` 用 `source.trim()` 归一化脚本，前导空行被吞掉，脚本里所有后续行号整体前移；Go 的 `AnalyzeScript` 保留原始行号（`parse_test.go:1029` 断言带一个前导空行的脚本首条诊断落在第 5 行）。实测 0/1/2 个前导空行的同一 `for ... by 0` 脚本在修复前都报第 3 行。
- 修复：新增 `pine::normalize_source`，只裁剪首行水平空白（含 BOM）与尾部空白，绝不丢弃前导行；`pine::compile` 与 `pinespec::validate_script` 共用该入口（后者此前也做 `source.trim()`，会二次吞掉空行）。
- 先红探针：把 `crates/jftrade-strategy/src/pine/mod.rs` 回滚到修复前版本（shasum `a1965978690a0475030a535b8d4861d8f2b9e28749980ce101b3257410de7ce3`）后，`blank_lines_before_the_script_keep_later_diagnostic_lines` 失败（1 个空行时报第 3 行而非第 4 行）；恢复修复后通过。修复后 `mod.rs` shasum `00386727cbd2c6ffdad8c6cd3b89d0b3afe648b3f56d609fe5b983db6e5d3638`、`pinespec/mod.rs` `c879f98fd079c79850a7ceb65941bc36c032929ca683ddcd6bf83ecbcb1d03a6`，探针副本与工作树按字节一致。
- 新增证据：`crates/jftrade-strategy/tests/pine_parse_diagnostics.rs` 两条用例——`unsupported_security_symbols_report_the_original_line`（动态符号脚本 ok=false、诊断码 `PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL`、消息含 request.security、行号为原始第 3 行）、
  `blank_lines_before_the_script_keep_later_diagnostic_lines`（0/1/2 前导空行 → 第 3/4/5 行）；
  以及 `crates/jftrade-strategy/src/pine/mod.rs::order_subset_compile_tests::compile_preserves_order_notification_metadata_and_immediate_close`（entry 的 comment/alert_message/disable_alert 与 close 的 immediately/comment/alert_message/disable_alert 按原值保留在 `LoweredStatement::Action` 命名实参中）。
- 映射终值：35 行 = 22 `function_exact` + 12 `partial` + 1 `boundary`（`:1014` partial → function_exact；`:167` boundary → partial）；全量 `[x]` 1555 → 1556、partial 2274 → 2274、boundary 622 → 621。

关键事实与新登记缺口（P1/P2）：

1. **循环诊断行归属不同（P2）**：同一脚本 Go 把首条诊断归到循环体所在行（第 5 行），Rust 归到 `for` 语句行（第 4 行）；本批修复了前导空行导致的行号前移，但归属语义差异保留，若要复刻需调整 `LoopStmt` 的诊断锚点（owner：`crates/jftrade-strategy/src/pine/parser.rs`）。
2. **订单元数据无 typed 投影（P2）**：Rust 以命名实参保留 comment/alert_message/disable_alert/immediately（本批新增断言），但没有 Go 的 `OrderStmt`/`ExitStmt` typed 字段；intent 语义投影在 `workers/pineworker` 与 `crates/jftrade-integration-pine` 执行层完成（`:286/:317/:348/:390/:432/:448` 五行保持 partial）。
3. **静态展开与 UDF 诊断缺口（P1，沿用既有登记）**：Go 把静态 for 展开为 `history(close,i)` 序列并内联单表达式 UDF，Rust 保留 typed For 与 `program.functions`（展开由 PineTS 承担）；`:838/:903` 登记的 UDF 参数不匹配、递归/嵌套 UDF、循环变量只读三类诊断仍未实现。
4. **兼容评分注册表（P2）**：`:763` 的 CompatibilityScore/SupportedFeatureIDs 在 Rust 由 MCP spec leaf 的冻结 payload 与 `supported_features()` 承担，缺少按注册表驱动的逐项断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_parse_diagnostics --locked --no-fail-fast` | 2 条中 1 条失败（前导空行用例报第 3 行），回滚 `mod.rs` 到 `a1965978…` 复现 |
| 新增用例 | 同上（修复后） | 2/2 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 62/62 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/mod.rs crates/jftrade-strategy/src/pinespec/mod.rs` | 修复后 `00386727…` / `c879f98f…`，与探针副本按字节一致 |
| 映射写入 | payload `/tmp/s128h_payload.json` 经 `/tmp/b82_apply.py` 应用 | 3 行给出终值，`[x]` 1555 → 1556、partial 2274 → 2274、boundary 622 → 621 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3229（Strategy 域 208） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1572、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2946 files）、`pnpm run check:quick` | 全部通过；`check:quick` 走 affected 计划（7 个受影响文件、rust 模块），1986/1986 通过并以 exit 0 结束 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 在 `.rcgu.o` 29488（< 50000，目标健康检查通过）的前提下仍失败：cargo-deny advisories 阶段报 8 条 `advisory-not-detected`（advisories FAILED、bans/licenses/sources ok），与分片六/七结论一致 |

后续：分片九进入 `pkg/strategy/pine/parse_collection_test.go` 17 行、`parse_object_test.go` 12 行、`parse_semantic_test.go` 12 行、`parse_request_test.go` 7 行（合计 48 行），随后 `pkg/strategy/pine` 其余文件，直至 `strategy_pine` 域 510 行清空。

### 分片九：`pkg/strategy/pine` 的 collection/object/semantic/request 四文件 48 行

范围：`parse_collection_test.go` 17、`parse_object_test.go` 12、`parse_semantic_test.go` 12、`parse_request_test.go` 7。
owner：`crates/jftrade-strategy`（Pine 编译/语义/需求计划）；对照方为 `workers/pineworker`（语言族展开）与 `crates/jftrade-pinespec` 冻结矩阵。

复核方式：把 48 条 Go 测试的金标脚本逐条抽出，用同一脚本跑 Rust 编译器形成支持矩阵（记录 ok 与诊断码/需求键），再按 Go 断言判定终值。矩阵结论：13 条脚本 Rust 可编译（其中 3 条可升 `[x]`、2 条给出新证据），35 条被拒（诊断码集中在 `PINE_CALL_UNSUPPORTED`、`PINE_STATEMENT_UNSUPPORTED`、`PINE_INDENT_UNEXPECTED`、`PINE_DECLARATION_UNSUPPORTED`、`PINE_EXPRESSION_INVALID`）。

修复（1 处功能差异，带先红探针）：

- 差异：Go 在计划前归一化单参数窗口形态（`ta.highest(20)` → `highest(high, 20)`、`ta.change(close)` → `change(close, 1)`），Rust planner 原先直接把实参入键，产出 `highest:20` 与 `change:close`，与 Go 断言及 pinespec 金标 `change:close:1` 不一致。
- 修复：`crates/jftrade-strategy/src/pine/planner.rs` 的 `ta.highest|ta.lowest|ta.change` 分支按 Go 规则补缺省 source（high/low）与缺省长度 1。
- 先红探针：回滚 `planner.rs` 到分片七值 `f61ae3c0347d06d8a894bba17bf843391a2d4843bbba0bbf2f1ebfd1a272a486` 后，`common_ta_window_keys_keep_the_requested_source` 失败（缺 `change:close:1` 与第二个 `highest:high:20`）；恢复修复后通过。修复后 shasum `88dd2ef344548cbd46eb57c24845840680e153ed1f01213af4805c92648cdbb0`，探针副本与工作树按字节一致。
- 新增证据：`crates/jftrade-strategy/tests/pine_request_and_visual_contracts.rs` 六条用例——`unsupported_request_security_forms_keep_the_go_diagnostic_codes`（七类诊断码逐条）、`stdev_keeps_the_legacy_close_key`、`visual_calls_are_ignored_with_named_warnings`（4 条警告点名 alertcondition 与 label.new）、`visual_metadata_lists_every_drawing_call`（7 条语义视觉元数据）、`request_security_moving_average_keys_keep_type_period_source_and_time_unit`、`common_ta_window_keys_keep_the_requested_source`。
- 映射终值：48 行 = 3 `function_exact` + 44 `partial` + 1 `boundary`（`:136`/`:82` partial → function_exact；`:377` boundary → function_exact；`:401` boundary → partial）；全量 `[x]` 1556 → 1559、partial 2274 → 2273、boundary 621 → 619。

关键事实与新登记缺口（P1/P2）：

1. **v2.0-v3.2 语言族整体缺失（P1）**：Go 的 collection（array/matrix/map 声明与操作）、UDT/object（`type ... new`、`method`、字段重赋值、方法链）、import/export 别名、varip 策略、动态循环与 v2.4-v2.7 集合/时间框架助手在 Rust 编译器均被拒（本分片 35 行 partial 的实测诊断码已写入账本），而 pinespec 冻结矩阵仍声明 v2.1 BBW/COG、v2.2 tuple、v2.3 collection/object、v2.4 MTF stoch 等能力——**矩阵声明与 Rust 实现不一致**，需要按批次决定是补齐实现还是修订矩阵（owner：`crates/jftrade-strategy/src/pine/{parser,lower,semantic}.rs` 与 `src/pinespec/mod.rs`）。
2. **MTF 纯 source 键空间不同（P1）**：Go 为 `request.security(syminfo.tickerid, "D", close)` 产出 `security_source:day:close`，Rust 仍为退化键 `security:syminfo.tickerid:"D":close`；`source[n]` 与 tuple/纯表达式的同族键同样退化（`:10` 登记，沿用分片五/六/七的 MTF 键缺口）。
3. **MTF 纯表达式/tuple 白名单未铺开（P1）**：`:183/:217/:263` 三条脚本 Rust 可编译，但内部表达式原样进 `security:` 键（如 `security:syminfo.tickerid:"15":((close Greater ta.sma(close,3)) And ...)`），Go 则展开为受支持 TA 键与 `security_source` 键；stoch/bollinger 族仍缺。
4. **v1.4 窗口/状态族部分缺失（P1）**：`:141` 脚本 Rust 在第 16 行被拒（`PINE_EXPRESSION_INVALID`），Go 断言该批窗口/动量/状态指标全部可用（含 barssince/valuewhen 等状态函数）。
5. **语义摘要字段面差异（P2）**：`:10`/`:55`/`:98` 的 Go 断言覆盖 Symbols/TupleBindings/FunctionCalls 的签名与 Visuals 的 title/variable/namedArgs；Rust 的 `SemanticSummary` 只暴露 symbols/visuals/declarations 子集，VisualMetadata 无 title/variable/namedArgs，限定成员 target 只保留末段（`top_right` vs Go `position.top_right`）。
6. **v2.9/v3.2 request.security 诊断矩阵缺口（P2）**：`:173` 的四类（tuple assignment、tuple alias mismatch、tuple 宽度上限、unsupported pure expression）在 Rust 侧没有同名诊断码（只有通用 `PINE_REQUEST_SECURITY_UNSUPPORTED`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_request_and_visual_contracts --locked --no-fail-fast` | 6 条中 1 条失败（缺 `change:close:1` 与 `highest:high:20`），回滚 `planner.rs` 到 `f61ae3c0…` 复现 |
| 新增用例 | 同上（修复后） | 6/6 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 67/67 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs` | 修复后 `88dd2ef3…`，与探针副本按字节一致；修复前副本为分片七的 `f61ae3c0…` |
| 映射写入 | payload `/tmp/s128i_payload.json` 经 `/tmp/b82_apply.py` 应用 | 48 行给出终值，`[x]` 1556 → 1559、partial 2274 → 2273、boundary 621 → 619 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3232（Strategy 域 211） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1578、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1893/1894，`api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 抖动失败（已知并行负载抖动）；隔离复跑 2/2 通过，次轮整轮 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2947 files）、`pnpm run check:quick` | 全部通过；`check:quick` 走 affected 计划（6 个受影响文件、rust 模块），1992/1992 通过并以 exit 0 结束 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 在 cargo-deny advisories 阶段失败（bans/licenses/sources ok），与分片六/七/八结论一致 |

后续：分片十进入 `pkg/strategy/pine` 剩余文件（`collection_object_bounds_test.go` 3、`object_collect_bounds_test.go` 3、`object_declaration_contracts_test.go` 2、`control_flow_reject_test.go` 2、`parser_loop_boundaries_test.go` 3、`language_execution_boundaries_test.go` 8、`validation_semantics_boundaries_test.go` 8、`public_lowering_test.go` 5、`parser_and_lowering_recovery_test.go` 7、`strategy_business_test.go` 3、`security_lowering_test.go` 3、`semantic_helper_boundaries_test.go` 4、`runtime_and_parser_boundaries_test.go` 5、`compiler_rejection_contracts_test.go` 3、`extended_ticker_test.go` 2、`udf_expansion_contracts_test.go` 2、`order_command_security_rejection_test.go` 2、`request_security_ast_contracts_test.go` 2、`request_security_diagnostics_test.go` 2、`request_security_object_contracts_test.go` 2 等），直至 `strategy_pine` 域 510 行清空。


### 分片十：`pkg/strategy/pine` 的 parser helper/recovery 八文件 27 行

范围：`parser_and_lowering_recovery_test.go` 7、`parser_helper_boundaries_test.go` 4、`parser_loop_boundaries_test.go` 3、`parser_recovery_boundaries_test.go` 3、`public_lowering_test.go` 5、`security_lowering_test.go` 3、`tuple_assignment_contracts_test.go` 1、`tuple_switch_reject_test.go` 1。
owner：`crates/jftrade-strategy`（Pine 解析、元组契约、static for 诊断、需求计划）；对照方为 `workers/pineworker` 与冻结矩阵。

复核方式：27 条 Go 测试逐条读源，先判断断言入口在 Rust 是否有对应对象——公共入口（`Compile`/`ParseScript` 金标脚本）可新建用例，内部助手（`newParseState`、`expandUDFCalls`、`parseGeneralTupleAssignment`、`lowerSupportedRequestSecurity`、`nextCollectionReadCall` 等）只能给出缺口或边界结论；再用同一批脚本跑 Rust 编译器形成支持矩阵（记录 ok、诊断码与需求键）。矩阵结论：8 条元组脚本修复后全部被拒且文案与 Go 一致，4 条 static for 边界修复后与 Go 一致，switch 6 例中 1 例文案对齐、5 例仅保证被拒。

修复（2 处功能差异，均带先红探针）：

1. 元组赋值契约缺失：Go 在 `parseGeneralTupleAssignment` 与 `ta.*` 签名表上拒绝别名数量不在 2..8、非法别名、值与别名数量不匹配、非白名单 tuple 源与元组指标 arity 不足（`ta.bb`/`ta.dmi`/`ta.supertrend`/`ta.kc`/`ta.macd`）；Rust 先前全部接受（`[first, second] = [close]`、`[openValue, 4bad] = [open, close]`、`[first, second, third] = ta.rsi(close, 14)`、9 别名都编译通过）。修复：`crates/jftrade-strategy/src/pine/parser.rs` 的 `split_assignment` 保留受损别名（新增 `tuple_alias_tokens`），并新增 `validate_tuple_assignment` 与 `tuple_indicator_signature`/`tuple_indicator_arity_message`，产出 `PINE_TUPLE_ALIAS_COUNT`、`PINE_TUPLE_ALIAS_INVALID`、`PINE_TUPLE_WIDTH`（含 request.security tuple 宽度）、`PINE_TUPLE_ARITY`、`PINE_TUPLE_SOURCE_UNSUPPORTED`；同一提交对 `signal = switch` 增加 `PINE_SWITCH_ARMS_REQUIRED`，文案与 Go 的 switch requires at least one arm 一致。
2. static for 越界判停错误：`crates/jftrade-strategy/src/pine/semantic.rs` 的 `report_static_for_diagnostics` 原先只在 `value == end` 时停止，`for i = 0 to 3 by 2` 被误报为 `expands to more than 100 iterations`；Go 在值越过上界时停止并报 `does not reach`。修复后四类边界与降序区间逐条与 Go 一致。

先红探针：把 `parser.rs`、`semantic.rs` 按字节回滚到 HEAD 后运行新用例，`static_for_ranges_reject_non_terminating_bounds`、`tuple_assignments_keep_the_go_alias_and_width_contract`、`malformed_tuple_and_switch_scripts_are_rejected` 三条失败（`public_entry_returns_program_and_propagates_helper_errors` 记录既有行为，回滚后仍通过）；恢复后 `parser.rs` shasum `c13a22ef…`、`semantic.rs` shasum `7ac4d0e3…`，与探针副本按字节一致。

新增证据：`crates/jftrade-strategy/tests/pine_tuple_contracts.rs` 四条用例——`public_entry_returns_program_and_propagates_helper_errors`（公开入口 program 结构 + internal JFTrade helper 文案）、`tuple_assignments_keep_the_go_alias_and_width_contract`（别名数量、非法别名、宽度、reassign 模式）、`malformed_tuple_and_switch_scripts_are_rejected`（8 条元组文案逐条 + switch 拒绝面）、`static_for_ranges_reject_non_terminating_bounds`（四类拒绝 + 降序通过）。

映射终值：27 行 = 3 function_exact + 19 partial + 5 boundary（`public_lowering_test.go:10`、`tuple_assignment_contracts_test.go:11`、`parser_loop_boundaries_test.go:38` 升 `[x]`；`parser_helper_boundaries_test.go:196`、`parser_recovery_boundaries_test.go:151` 由 partial 转 boundary，`parser_loop_boundaries_test.go:10` 维持 boundary 并改写为升级路径表述）；全量 `[x]` 1559 → 1562、partial 2273 → 2268、boundary 619 → 621。

本分片新登记缺口：

1. **元组参数历史引用未拒绝（P1）**：`[value, aux] = ta.dmi(close[1], 14)` 在 Rust 仍编译通过，Go 报 history references（`parser_loop_boundaries_test.go:63`）。
2. **while 动态循环族缺失（P1）**：Rust 解析器没有任何 while 处理，`while close > open` 顶层报 `PINE_INDENT_UNEXPECTED`；Go 支持 depth ≤ 4 的 runtime while（`parser_helper_boundaries_test.go:66`、`parser_and_lowering_recovery_test.go:119`）。
3. **switch 语言族缺失（P1）**：空 switch 文案已对齐，其余 5 条 switch 臂用例只保证被拒，Go 的 switch chunks 文案与条件校验未实现（`tuple_switch_reject_test.go:8`）。
4. **strategy 元数据校验缺口（P1）**：`default_qty_type=strategy.contracts`、`pyramiding=-1`、`pyramiding=foo`、`strategy.entry(..., risk=1)` 在 Rust 被接受，Go 拒绝（`public_lowering_test.go:130`）。
5. **security 只读表达式白名单缺口（P1）**：`str.upper`/`timeframe.in_seconds`/`math.sqrt` 在 Go 的 security 纯表达式降级中可用而 Rust 拒绝；`close[-1]`、`vwap`、`close[1] + open` 在 Go 被拒而 Rust 接受（`security_lowering_test.go:92`）。
6. **request.security 元组三类专码缺失（P2）**：`PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED`/`TUPLE_ASSIGNMENT`/`TUPLE_MISMATCH` 仍缺，`x = request.security(syminfo.tickerid, "15", [close])` 被接受（沿用分片九登记）。
7. **空程序 hook 语句面差异（P2）**：空脚本 Rust 产出 `on_kline_close:0`，Go 补 fallback log 语句（`parser_helper_boundaries_test.go:11`）。
8. **Go 解析助手族其余边界（P2）**：UDF 展开的空白保真与参数回滚、compileUDFBody 分支错误、保留名检查、input/str/timeframe 畸形调用保持原文、TA 字符串重写（旧式 `fast, slow, hist = ta.macd(close)` 在 Rust 报 `PINE_STATEMENT_UNSUPPORTED`）、未知命名参数拒绝、注解与注释角色分类、对象/集合助手族，均以 partial 或 boundary 记录 owner 与升级路径。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_tuple_contracts --locked`（回滚 `parser.rs`、`semantic.rs` 到 HEAD） | 4 条中 3 条失败，回滚状态可复现 |
| 新增用例 | 同上（修复后） | 4/4 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 72/72 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/parser.rs crates/jftrade-strategy/src/pine/semantic.rs` | 修复后 `c13a22ef…` / `7ac4d0e3…`，与探针副本按字节一致 |
| 映射写入 | payload `/tmp/s128j_payload.json` 经 `/tmp/b82_apply.py` 应用（补丁 `/tmp/s128j_payload_fix.json` 修正 `parser_loop_boundaries_test.go:10` 的 boundary 归类） | 27 行给出终值，`[x]` 1559 → 1562、partial 2273 → 2268、boundary 619 → 621 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3239（Strategy 域 218） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1582、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1893/1894，`api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 抖动失败；隔离复跑 2/2 通过，次轮整轮 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2948 files）、`pnpm run check:quick` | 全部通过；`check:quick` 1996/1996 并以 exit 0 结束；期间 `.rcgu.o` 达 53241 触发 target-health，确认无 Cargo 进程后执行 `pnpm run clean:rust:artifacts` |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | cargo-deny advisories 阶段各 8 条 `advisory-not-detected`（bans/licenses/sources ok），与分片六至九一致 |

后续：分片十一进入 `pkg/strategy/pine` 剩余文件（`collection_object_bounds_test.go` 3、`object_collect_bounds_test.go` 3、`object_declaration_contracts_test.go` 2、`control_flow_reject_test.go` 2、`controlflow_object_collection_contracts_test.go` 3、`language_execution_boundaries_test.go` 8、`language_failure_contracts_test.go` 3、`validation_semantics_boundaries_test.go` 8、`compiler_rejection_contracts_test.go` 3 等），直至 `strategy_pine` 域 510 行清空。


### 分片十一：`pkg/strategy/pine` 的 collection/object/compiler/expression 边界族 27 行

范围：`collection_object_bounds_test.go` 3、`compiler_and_security_diagnostics_test.go` 3、`compiler_rejection_contracts_test.go` 3、`control_flow_reject_test.go` 2、`controlflow_object_collection_contracts_test.go` 3、`expression_test.go` 3、`extended_ticker_test.go` 2、`language_execution_boundaries_test.go` 8。
owner：`crates/jftrade-strategy`（Pine 解析、planner 键形与诊断、`crates/jftrade-strategy/src/pinespec` 能力矩阵）；对照方为 `workers/pineworker` 与冻结矩阵。

复核方式：27 条 Go 测试逐条读源，按断言入口分类——公共入口（`Compile`/`AnalyzeScript`）可建金标用例，Go 内部助手（`newObjectCollectionBoundaryParseState`、`lowerRequestSecurityTACall`、`pineWindowFunctionArgs`、`capabilityStatusValue` 等）只能给 partial 或 boundary；再用同一批脚本跑 Rust 编译器形成支持矩阵（记录 ok、诊断码、需求键与错误文案）。矩阵结论：表达式解析三例与扩展 ticker 一例可升 `[x]`；控制流 17 例全部被拒但 UDF 语义未落地；集合/对象九例在 Rust 无执行面，归为边界。

修复（4 处功能差异，均带先红探针）：

1. 窗口族缺省参数错误：Go 的 `pineWindowFunctionArgs` 规定单参时 `highest`/`lowest` 视作周期、其余窗口族视作源且缺省周期 14，Rust 原先把 `ta.mom(hl2)` 计划成 `mom:hl2:hl2`。修复：`crates/jftrade-strategy/src/pine/planner.rs` 新增 `window_arguments`（对齐 `pineWindowFunctionArgs`）与 `source_length_arguments`（对齐 `pineSourceLengthArgs`），窗口族改由同一助手派生源与周期，`ta.mom()` 等零参形态也回到 `mom:close:14`。
2. 非正周期未校验：Go 在 planner 用 `ParsePositiveInt` 拒绝 `ta.sma(close, 0)` 并报 `ma() period must be a positive integer`，Rust 先前的需求键是 `ma:SMA:0`。修复：新增 `ensure_positive_period`，ma 族、rsi/cci 与窗口族遇到可判定的非正字面周期即报 `{callee} period must be a positive integer`（别名标识符保持原样，不误伤变量周期）。
3. ticker 白名单过严且过松：Go 接受 `ticker.standard()`（零参）并只接受 `heikinashi` 恰一参、`standard` 零或一参、`inherit` 两参且首参为受支持 ticker；Rust 用「内部出现 syminfo.tickerid」的宽松匹配，既拒绝 `ticker.standard()` 又放过 `ticker.heikinashi(otherTicker, syminfo.tickerid)` 这类畸形形态。修复：`crates/jftrade-strategy/src/pine/semantic.rs` 重写 `is_supported_request_security_ticker` 并新增顶层逗号切分助手，逐形态对齐 Go。
4. 静态周期字符串未拒绝：Go 对 `request.security(syminfo.tickerid, "2", close)` 报 `only static timeframe strings`，Rust 退化成 `security:syminfo.tickerid:"2":close` 兜底键。修复：planner 的 request.security 回落分支在时间框架是字符串字面量且不在 `indicator_time_unit` 白名单内时报同一文案。

先红探针：把 `planner.rs`、`semantic.rs` 按字节回滚到分片十状态后运行新用例，4 条失败（`extended_ticker_and_chart_flags_keep_requirements`、`moving_average_period_must_be_positive`、`request_security_rejects_unlisted_static_timeframe_strings`、`window_family_defaults_keep_the_go_source_and_period`），3 条表达式用例照常通过（记录既有行为）；恢复后 `planner.rs` shasum `4903e9da…`、`semantic.rs` shasum `9f435e5c…`，与探针副本按字节一致。

新增证据：`crates/jftrade-strategy/tests/pine_expression_and_security_boundaries.rs` 八条用例——`expression_parser_rejects_blank_assignment`、`expression_parser_accepts_trimmed_expressions`、`expression_parser_rejects_trailing_operator`（Go 的 `parseExpression` 助手在 Rust 的等价编译面）、`extended_ticker_and_chart_flags_keep_requirements`、`request_security_tickers_follow_the_go_whitelist`、`moving_average_period_must_be_positive`、`request_security_rejects_unlisted_static_timeframe_strings`、`window_family_defaults_keep_the_go_source_and_period`。

映射终值：27 行 = 4 function_exact + 14 partial + 9 boundary（`expression_test.go:5/:11/:21` 与 `extended_ticker_test.go:49` 升 `[x]`；集合/对象九行归 boundary）；全量 `[x]` 1562 → 1566、partial 2268 → 2256、boundary 621 → 629。

本分片新登记缺口：

1. **UDF 参数列表与多行体缺失（P1）**：`helper(x) => x` 单行带参形态与多行 UDF 体在 Rust 解析层统一报 `PINE_EXPRESSION_INVALID: unexpected token "=>"`，Go 支持参数、默认值、多行 if/else 体并归一化为 `ifelse`（`control_flow_reject_test.go:8`、`controlflow_object_collection_contracts_test.go:11`、`language_execution_boundaries_test.go:531`）。
2. **循环语义缺口（P1）**：三层 `for` 嵌套在 Rust 编译通过（Go 报 `nested for loops deeper than`），`continue` 在循环体内报 `PINE_ACTION_REQUIRED`，`while` 仍无实现（`control_flow_reject_test.go:8/:43`，沿用分片十登记）。
3. **MTF TA 族键覆盖不足（P1）**：`bb`/`supertrend`/`bbw`/`cog`/`stoch` 在 `request.security` 内只产出 `security:` 兜底键，`rsi` 只给 `rsi:7` 缺 15m 维度；Go 逐个降级为带时间单位的指标键（`language_execution_boundaries_test.go:12`）。
4. **security 纯表达式面缺口（P1）**：`market?.snapshot.close` 可选链未实现、`min(close, open)` 被判为不支持调用，而 Go 把两者视为允许的纯读（`compiler_and_security_diagnostics_test.go:44`）。
5. **security 诊断文案与元组专码（P2）**：`value = request.security(` 报 `PINE_EXPRESSION_REQUIRED`（Go 文案 could not be parsed）、`array.unknown(values)` 报 `PINE_CALL_UNSUPPORTED`（Go 文案 collection namespaces）、`[onlyOne] = request.security(..., [close, high])` 走 `PINE_TUPLE_ALIAS_COUNT`（Go 期望 security 元组宽度专码）（`compiler_rejection_contracts_test.go:9`）。
6. **头部/词法/编辑器助手与能力计分（P2）**：`scanCompilationHeaders`、`cachedRegexp`、`parseTACall`、`replaceColorFunctions`、`normalizeTernaryExpression`、`diagnosticForLine`、`capabilityStatusValue(analyzed)=0.5` 在 Rust 无同形对象，Rust 侧只对 `//@version=5`、`indicator()`、`library()` 的拒绝与 pinespec 的 status/weight/layers 能力面（`language_execution_boundaries_test.go:287`、`compiler_and_security_diagnostics_test.go:70`）。
7. **TA 原文保留面（P2）**：Go 的 `replaceTA*` 重写助手保证畸形调用保持原文，Rust 无重写层，`fast, slow, hist = ta.macd(close)` 直接报 `PINE_STATEMENT_UNSUPPORTED`（`compiler_rejection_contracts_test.go:180`、`language_execution_boundaries_test.go:600`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_expression_and_security_boundaries --locked`（回滚 `planner.rs`、`semantic.rs` 到分片十状态） | 8 条中 4 条失败，回滚状态可复现 |
| 新增用例 | 同上（修复后） | 8/8 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 81/81 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs crates/jftrade-strategy/src/pine/semantic.rs` | 修复后 `4903e9da…` / `9f435e5c…`，与探针副本按字节一致 |
| 映射写入 | payload `/tmp/s128k_payload.json` 经 `/tmp/b82_apply.py` 应用 | 27 行给出终值，`[x]` 1562 → 1566、partial 2268 → 2256、boundary 621 → 629 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3248（Strategy 域 227） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1590、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1894/1894 通过 |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2949 files）、`pnpm run check:quick` | 全部通过；`check:quick` 2005/2005 并以 exit 0 结束（`.rcgu.o` 30232，低于目标健康阈值） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 沿用分片六至十结论：cargo-deny advisories 阶段 8 条 `advisory-not-detected` 阻断（本分片未改动依赖） |

后续：分片十二进入 `pkg/strategy/pine` 剩余文件（`language_failure_contracts_test.go` 3、`object_collect_bounds_test.go` 3、`object_collect_reject_test.go` 1、`object_declaration_contracts_test.go` 2、`order_command_security_rejection_test.go` 2、`order_metadata_contracts_test.go` 1、`parse_benchmark_business_test.go` 1、`request_security_ast_contracts_test.go` 2、`request_security_diagnostics_test.go` 2、`request_security_object_contracts_test.go` 2、`runtime_and_parser_boundaries_test.go` 5、`semantic_helper_boundaries_test.go` 4、`shared_structure_corpus_test.go` 1、`strategy_business_test.go` 3、`strategy_call_bounds_test.go` 3、`udf_expansion_contracts_test.go` 2、`validation_semantics_boundaries_test.go` 8 等），再到 `pkg/strategy/pineengine`、`pineworker` 与 `pinespec`，直至 `strategy_pine` 域清空。

### 分片十二：`pkg/strategy/pine` 的 order/object/security/tuple 契约族 27 行

范围：`language_failure_contracts_test.go` 3、`object_collect_bounds_test.go` 3、`object_collect_reject_test.go` 1、`object_declaration_contracts_test.go` 2、`order_command_security_rejection_test.go` 2、`order_metadata_contracts_test.go` 1、`parse_benchmark_business_test.go` 1、`parse_test.go` 11（`:286`/`:317`/`:348`/`:390`/`:432`/`:448`/`:763`/`:790`/`:838`/`:903`/`:977`）、`request_security_ast_contracts_test.go` 2、`request_security_diagnostics_test.go` 1。
owner：`crates/jftrade-strategy`（Pine 解析、语义诊断、降级元数据）与 `crates/jftrade-engine`（MCP spec leaf 与冻结兼容 payload）；对照方为 `workers/pineworker`。

复核方式：先按文件读 Go 测试源码，再跑统一支持矩阵探针（`crates/jftrade-strategy/tests/zz_scratch_probe.rs`，逐例打印 ok、首个诊断码、全部诊断码与行号），最后把公共入口断言写成回归用例；Go 内部助手（`pineOrderMetadata`、`pineCloseAllMetadata`、`validateStrategyExitTriggers`、`lowerObjectMethodCalls`、`requestSecurityLoweredASTIsPure`、`pineBenchmarkCases` 等）只能给 partial 或 boundary。矩阵结论：订单元数据与传播两例、兼容注册表一例可升 `[x]`；对象/集合七例在 Rust 无执行面，归为边界；其余 17 例给出 partial 与逐条缺口。

修复（6 处功能差异，全部先红后绿）：

1. 订单元数据校验缺失：Rust 原先把 `disable_alert=maybe`、`immediately=true`（entry/order/exit）、`strategy.close()` 无 id、`strategy.close("Long", mystery=true)`、`strategy.close_all("maybe")`、`close_all` 第 5 个 positional、`strategy.cancel()` 参数个数错误全部编译通过。修复：`crates/jftrade-strategy/src/pine/semantic.rs::strategy_order_diagnostic` 拆出 `order_arity_diagnostic`、`order_close_all_positional_diagnostic`、`order_boolean_metadata_diagnostic`，按 Go 的 `rejectUnsupportedNamedArgs`/`pineOrderMetadata`/`pineCloseAllMetadata` 允许集与顺序（id/direction → OCA → 允许集 → close_all positional → Boolean 元数据 → qty 冲突 → exit 触发器）给出 `PINE_COMPILE_ERROR`/`PINE_ORDER_*` 码与同文案。
2. `request.security` 纯度漏判：`strategy.position_size`、`close + strategy.position_size`、`log.info(...)`、`line.new(...)`、`values.push(close)` 原先全部通过（Go 一律 `PINE_REQUEST_SECURITY_SIDE_EFFECT`）。修复：`request_security_expression_has_side_effect` 对齐 `validate.go::requestSecurityExpressionHasSideEffect` 的文本 denylist（`strategy.`/`log.`/`table.` 成员根、`runtime.error`/`line.new`/`label.new`/`box.new`/绘图族调用、`.push`/`.set`/`.put` 等变异后缀），并把该诊断提前到内层调用诊断之前（`visit_call` 对 `request.security` 先判定再递归），与 Go 的行级顺序一致。
3. 元组诊断码与文案不统一：`x = request.security(..., [close, open])` 原先被当成单值读取直接通过，`[only] = request.security(..., [close, open])` 报 `PINE_TUPLE_ALIAS_COUNT`、`[a, b] = request.security(..., [close])` 报 `PINE_TUPLE_WIDTH`。修复：`crates/jftrade-strategy/src/pine/parser.rs` 新增 `request_security_tuple_items`、`request_security_tuple_width_message`、`request_security_tuple_assignment_diagnostic`：非元组赋值遇到元组参数先按宽度（`PINE_REQUEST_SECURITY_TUPLE_UNSUPPORTED`「support 2 to 8 values」）再按赋值形态（`PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT`「must be assigned with matching tuple aliases」），元组赋值走 `PINE_REQUEST_SECURITY_TUPLE_MISMATCH`，文案与 Go 逐字一致。
4. 单引号字符串被拒：Go 的 `parse_tokenize.go::stripInlineComment` 与 `parse_args.go::unquote` 都接受单引号字面量（`strategy.close_all(true, 'close comment', 'close alert', true)` 可编译），Rust lexer 报 `invalid character`。修复：`crates/jftrade-strategy/src/pine/lexer.rs` 把单引号纳入字符串定界符（同一转义扫描），`decode_string` 对单引号按 Go 的 `strconv.Unquote` 失败回退路径只剥定界符、不做转义处理。
5. history 回看溢出被放过：`close[999999999999999999999999999999]` 原先编译通过，Go 的 `strconv.Atoi` 失败路径给出「history reference lookback must be a non-negative integer」。修复：`semantic.rs::report_history_reference_diagnostics` 把 `Number` 索引解析失败也映射为 `PINE_HISTORY_REF_UNSUPPORTED` 的 non-negative 文案，超过 500 的既有分支保持不变。
6. `request.security` 合并参数只认具名形态：`request.security(syminfo.tickerid, "60", close, barmerge.gaps_off, barmerge.lookahead_on)` 与 `..., barmerge.gaps_on)` 原先通过。修复：合并参数判定改为对第 4 个及之后的每个实参做小写文本匹配（`barmerge.lookahead_on`/`barmerge.gaps_on`/`calc_bars_count=`），具名与 positional 两种形态同码同文案。

新增证据：

- `crates/jftrade-strategy/tests/pine_order_metadata_and_security_rejections.rs`（7 用例：订单歧义矩阵、订单元数据传播、支持的位置参数与单引号解码、合并参数具名与 positional 拒绝、纯度副作用、元组诊断码、history 溢出）。
- `crates/jftrade-engine/tests/strategy_pine_mcp_contract.rs::spec_leaf_and_frozen_payload_keep_the_go_compatibility_registry`（`scoreModelVersion`、`compatibilityScore`、5 个维度，以及冻结 payload 的 76 个注册表 id、2 个排除 id 与唯一性）。

探针先红证据（修复前状态）：新增用例首轮 5/5 失败（`compile_rejects_ambiguous_order_metadata_and_missing_ids`、`compile_accepts_supported_order_positional_metadata`、`request_security_rejects_impure_member_and_visual_side_effects`、`request_security_tuple_diagnostics_match_go_codes`、`history_reference_overflow_is_rejected`），修复后 7/7 通过；`request_security_merge_flags_are_rejected_in_named_and_positional_form` 由第三轮探针（positional `barmerge.gaps_on`/`barmerge.lookahead_on` 与 `calc_bars_count=` 均 ok=true）复现先红。生产文件 shasum：`semantic.rs` `9f435e5c…` → `6d6d109f…`、`lexer.rs` `e570b4b0…` → `5eb838a3…`、`parser.rs` `c13a22ef…` → `609e8bf0…`。

映射终值：3 条 `[x]`（`language_failure_contracts_test.go:239`、`order_command_security_rejection_test.go:11`、`parse_test.go:763`）、17 条 partial、7 条 boundary。跨类型迁移：三例 partial 升 `function_exact`、七例 partial 转 boundary（对象/集合族）、`parse_test.go:977` 由 boundary 转 partial（switch 缺口如实登记）。计数因此为 `[x]` 1566 升至 1569、partial 2256 降至 2247、boundary 629 升至 635，合计 4451 不变。

缺口登记（本批新增）：

1. **switch 解析缺失（P1）**：`signal = switch` 两条手臂与 `switch signal` 语句块在 Rust 均报 `PINE_SWITCH_ARMS_REQUIRED`，而 Go 把 switch 表达式重写为 `ifelse`、switch 语句落到 `IfStmt`（`parse_test.go:977`）；修复位置：`parser.rs`/`planner.rs` 补 switch 手臂降级，回归测试按该行 Go 断言（语句数、ifelse、then/else 分支）。
2. **security 内 TA 白名单与高级指标参数校验缺失（P1）**：`ta.sum(close, 5)`、`ta.bb(close, 20)`、`ta.correlation(close, last_price, 20)`、`request.security(..., "D", ta.obv)` 在 Rust 编译通过，Go 分别报 `PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED` 与高级指标参数规则（`request_security_ast_contracts_test.go:26`、`request_security_diagnostics_test.go:8`）；修复位置：`semantic.rs` 的 security 纯度与白名单层，或 planner 的 security 指标表。
3. **UDF 与循环只读诊断码不一致（P2）**：递归、嵌套与签名不匹配 UDF 在 Rust 走 `PINE_EXPRESSION_INVALID`（Go：`PINE_UDF_*_UNSUPPORTED`，行号 4/3/4），循环变量只读走 `PINE_STATEMENT_UNSUPPORTED`（Go：`PINE_LOOP_VARIABLE_READONLY`，行 4）（`parse_test.go:838`、`parse_test.go:903`）。
4. **未闭合 `request.security` 诊断码（P2）**：Rust 报 `PINE_EXPRESSION_REQUIRED`，Go 报 `PINE_REQUEST_SECURITY_UNSUPPORTED`「call could not be parsed」（`order_command_security_rejection_test.go:82`）。
5. **benchmark 语料未移植（P2）**：Go 的 6 条 `pineBenchmarkCases()` 未进 Rust corpus，`udf_static_for` 需先补 UDF/循环语义（`parse_benchmark_business_test.go:5`）。
6. **`Compilation.features` 粗粒度（P2）**：Rust 仍返回 20 条能力 id，Go 的 `SupportedFeatureIDs()` 注册表（167 条）只在冻结兼容 payload 与 MCP support matrix 中体现（`parse_test.go:763`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_order_metadata_and_security_rejections --locked --no-fail-fast`（修复前状态） | 5/5 失败（断言落在「编译必须失败」与码、文案比对） |
| 新增用例 | 同上（修复后） | 7/7 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 87/87 通过 |
| 探针回滚校验 | `shasum -a 256 crates/jftrade-strategy/src/pine/semantic.rs crates/jftrade-strategy/src/pine/lexer.rs crates/jftrade-strategy/src/pine/parser.rs` | 修复前 `9f435e5c…`、`e570b4b0…`、`c13a22ef…`；修复后 `6d6d109f…`、`5eb838a3…`、`609e8bf0…` |
| 映射写入 | payload `/tmp/s130l_payload.json` 经 `/tmp/b82_apply.py` 应用 | 27 行给出终值，`[x]` 1566 → 1569、partial 2256 → 2247、boundary 629 → 635（合计仍为 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3255（Strategy 域 234）；汇总 4451 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1597、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1895/1895 通过（首轮即绿） |
| 兼容 replay 与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility（3 平台档 / 6 link case / 10 facade / 4 event）、generated、ai-context（6 modules / 8 指令文件）通过；zero-go 见下方更正后通过（2950 tracked files）；`check:quick` 首轮失败于 4 条并行负载抖动用例，隔离复跑 4/4 通过、整轮 engine 1895/1895 后重跑通过（exit 0）；期间 `.rcgu.o` 达 53076 触发 target-health，确认无 Cargo 进程后执行 `pnpm run clean:rust:artifacts`（113006 files / 30.7GiB） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 沿用分片六至十一结论：cargo-deny advisories 阶段 8 条 `advisory-not-detected` 阻断（bans/licenses/sources ok，本分片未改动依赖） |

分片十一更正（本批发现）：`crates/jftrade-strategy/tests/pine_expression_and_security_boundaries.rs:203` 的注释里出现 `Go test` 字样，命中 `scripts/check-zero-go.mjs` 的 active pattern（`\bgo\s+(?:run|build|test|generate|vet)\b`，大小写不敏感），导致分片十一记录为通过的 `check:zero-go` 实际会在该行失败。本批把该注释改写为 `reference case`，`check:zero-go` 复跑通过（exit 0）。后续分片在 Rust 源码与测试注释中不要书写 `Go test` 这类会命中 active pattern 的措辞。

后续：分片十三继续 `pkg/strategy/pine` 剩余 29 行（`request_security_diagnostics_test.go:52`、`request_security_object_contracts_test.go` 2、`runtime_and_parser_boundaries_test.go` 5、`semantic_helper_boundaries_test.go` 4、`shared_structure_corpus_test.go` 1、`strategy_business_test.go` 3、`strategy_call_bounds_test.go` 3、`udf_expansion_contracts_test.go` 2、`validation_semantics_boundaries_test.go` 8 等），再到 `pkg/strategy/pineengine`、`pineworker` 与 `pinespec`，直至 `strategy_pine` 域清空。

### 分片十三：`pkg/strategy/pine` 的 risk/order/语料/声明缺省族 29 行

范围：`request_security_diagnostics_test.go:52`、`request_security_object_contracts_test.go:11`/`:86`、`runtime_and_parser_boundaries_test.go:9`/`:46`/`:62`/`:94`/`:134`、`semantic_helper_boundaries_test.go:8`/`:58`/`:99`/`:128`、`shared_structure_corpus_test.go:35`、`strategy_business_test.go:11`/`:95`/`:130`、`strategy_call_bounds_test.go:11`/`:98`/`:120`、`udf_expansion_contracts_test.go:8`/`:37`、`validation_semantics_boundaries_test.go:8`/`:29`/`:43`/`:57`/`:70`/`:86`/`:96`/`:108`。

owner：`crates/jftrade-strategy`（Pine 词法、解析、语义与 planner）；对照方 `workers/pineworker`（typed order intents、UDT/集合运行时）。

复核方式：先读 Go 用例源码，再跑统一支持矩阵探针（逐例打印 ok、诊断码、行号、metadata、requirements 与逐例结构计数），最后把可观察契约写成 `crates/jftrade-strategy/tests/pine_risk_and_block_parity.rs` 的 10 条回归；共享语料探针直接消费 `tests/fixtures/pine-structure-corpus.json`。Go 内部助手（`analyzeSemantics`、`semanticCollectionOperations`、`expandUDFCalls`、`lowerSupportedRequestSecurity`、`replaceTA*` 等）在 Rust 无同形对象，按 boundary 或 partial 记录。

修复（5 处功能差异，全部先红后绿）：

1. 缩进块内赋值与重赋值解析失败：`if`/`else`/`for` 块内 `x = close` 报 `PINE_EXPRESSION_REQUIRED`、`armed := true` 报 `PINE_EXPRESSION_INVALID: unexpected token ":="`，而块内长赋值因切片偏移偶然成功（共享语料 `stateful-nested-reversal` 因此失败）。修复：`crates/jftrade-strategy/src/pine/lexer.rs` 的 `LexedLine` 新增 `offset`（前导空白字符数，与 `indent` 的 tab 列宽区分），`parser.rs::split_assignment` 与 `expression_from_text` 按 `offset` 切片与过滤 token。
2. `strategy.risk.*` 声明无校验（对应 `strategy_business_test.go:130`）：`allow_entry_in(strategy.direction.both)`、`max_drawdown(10, strategy.fixed)`、`max_intraday_filled_orders(0)`、`max_position_size(1, 2)` 与 6 个空参调用原先全部编译通过。修复：`semantic.rs::strategy_risk_diagnostic` 按 Go 的 `parseStrategyRiskAmountArgs`/`parseStrategyRiskCountArgs`/`parseStrategyRiskPositionSize`/`normalizeStrategyAllowedEntryDirection` 给出 `PINE_COMPILE_ERROR` 与同文案（direction、not supported、positive constant integer、requires one argument、requires at least two arguments）。
3. trail 退出缺少 `trail_offset`：`strategy.exit("NoOffset", "Long", trail_points=10)` 与 `trail_price` 单用原先通过，Go 报 `trailing stop requires trail_offset`。修复：`semantic.rs::strategy_order_diagnostic` 在 bracket 冲突判定之后补该检查（对应 `strategy_call_bounds_test.go:98` 的第 8 条边界）。
4. 声明标题与缺省常量（对应 `validation_semantics_boundaries_test.go:70`/`:86`）：`strategy()`、`strategy(title="Risk managed")`、`strategy(overlay=true)` 原先因缺位置标题被 `PINE_STRATEGY_NAME_REQUIRED` 拒绝；非法常量回退时 Rust `warnings` 为空。修复：`parser.rs::strategy_declaration_title` 采用 Go 的默认名 `Pine Strategy` 并支持命名 `title=`，`lower.rs::lower_metadata` 改用 `arguments.iter().skip(1)`，`semantic.rs::strategy_declaration_diagnostics` 输出 7 条 Go 同文案告警。
5. TA 目录补齐（共享语料 `nested-series-signal-confirmation`）：`semantic.rs::is_supported_call` 补 `ta.cum`/`ta.highestbars`/`ta.lowestbars`/`ta.stoch`/`ta.barssince`/`ta.valuewhen`；`planner.rs` 新增 `ta.cum`（键 `cum:<source>`）、`ta.stoch`（键 `stoch:<source>:<length>[:<unit>]`，校验 4/5 参、literal high/low、正周期）、window 族 `ta.highestbars`/`ta.lowestbars`，`ta.barssince`/`ta.valuewhen` 归状态序列返回 `Ok(None)`。

映射终值（29 行）：`[x]` 9 行（`strategy_business_test.go:11`/`:130`、`strategy_call_bounds_test.go:98`/`:120`、`validation_semantics_boundaries_test.go:43`/`:70`/`:86`/`:96`/`:108`）；partial 15 行；boundary 5 行（`request_security_object_contracts_test.go:86`、`semantic_helper_boundaries_test.go:8`/`:58`/`:128`、`udf_expansion_contracts_test.go:37`）。共享语料 8 例中 7 例通过，第 8 例 `mtf-derived-and-collection-state` 钉为已知失败（见下）。

缺口登记：

1. **`request.security` TA 白名单未对齐（P1）**：Go 的 `requestSecurityExpressionHasUnsupportedTACall`/`lowerRequestSecurityTACall` 拒绝白名单外调用，Rust 只判纯度。复现：`x = request.security(syminfo.tickerid, "D", ta.sum(close, 5))` 在 Rust 编译通过；期望：`PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED`。修复位置：`semantic.rs::request_security_diagnostic`；回归要求：security TA 子集矩阵（ma/rsi/macd/atr/bb/supertrend/stoch 与高级族 + 白名单外拒绝）。
2. **集合命名空间缺失（P1）**：`array.from(close, open, high).median()` 报 `PINE_CALL_INVALID`、`array.new_float(...)` 报 `PINE_CALL_UNSUPPORTED`，共享语料第 3 例与 `semantic_helper_boundaries_test.go:58` 因此挂起；Go 由 `parse_collection.go` 与运行时承担。修复位置：worker 运行时或 Rust 集合命名空间层，需同步 corpus 期望。
3. **多行 UDF 定义不支持（P1）**：`f(x) =>` 换行缩进体报 `PINE_EXPRESSION_INVALID`（仅单行 `f(x) => x + 1` 可用），Go 支持并把展开错误（递归、实参个数、超深）作为可行动诊断。修复位置：`parser.rs::parse_function_header` 与 `LoweredFunction.body` 的块体支持。
4. **重复 tuple 别名未检（P2）**：`[fast, fast] = ta.macd(close, 12, 26, 9)` 在 Rust 通过，Go 报 `tuple assignment repeats fast`（`semantic_helper_boundaries_test.go:99`）。
5. **诊断码与文案差异（P2）**：未闭合 `request.security(` 在 Rust 报 `PINE_EXPRESSION_INVALID`（Go `PINE_REQUEST_SECURITY_UNSUPPORTED`）、security 内重赋值报 `PINE_EXPRESSION_INVALID`（Go `PINE_REQUEST_SECURITY_SIDE_EFFECT`）、截断表达式报 `PINE_STATEMENT_UNSUPPORTED`（Go 映射 `PINE_COMPILE_ERROR`）、`runtime.error("risk limit exceeded")` 未透传原文、`array.unsupported` 未使用 collection 措辞。
6. **`by int(close)` 动态步长（P2）**：Go 走 `errStaticForRuntimeFallback` 运行时回退，Rust 报 `PINE_CALL_UNSUPPORTED`（`int()` 转换缺失）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | 探针 `crates/jftrade-strategy/tests/zz_scratch_probe.rs`（已删除） | 缩进块赋值：`if_let_short`/`if_reassign_short` 报 `PINE_EXPRESSION_REQUIRED`、`else_reassign` 报 `PINE_EXPRESSION_INVALID`；risk 4 条边界与 trail 缺 offset 全部 ok=true；共享语料 4/8 失败 |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --test pine_risk_and_block_parity --locked --no-fail-fast` | 10/10 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 97/97 通过 |
| 共享语料探针 | 同上（语料断言并入新增用例） | 7/8 通过，`mtf-derived-and-collection-state` 钉为 `PINE_CALL_INVALID` |
| 生产文件摘要 | `shasum -a 256 crates/jftrade-strategy/src/pine/{lexer,parser,semantic,planner,lower}.rs` | 修复后 `c72c8ebe…`、`8d4463bf…`、`8cc2249d…`、`3487d617…`、`429a8fc4…`（分片十二前值 `5eb838a3…`、`609e8bf0…`、`6d6d109f…`） |
| 映射写入 | 29 行 payload 经 `/tmp/b75_writer.py` 应用，另 2 行按唯一性修正 | `[x]` 1569 → 1578、partial 2247 → 2235、boundary 635 → 638（合计仍为 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2（既定缺口）；Rust 测试 3265（Strategy 域 244）；汇总 4451 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1606、unrecorded 0、stale 0、unknown 53 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1895/1895 通过（首轮即绿） |

后续：分片十四进入 `pkg/strategy/pine` 剩余族（`parse_request_test.go`、`parse_semantic_test.go`、`parse_object_test.go` 等）并越过 `pkg/strategy/pineengine`、`pineworker`、`pinespec`，随后按序推进 assistant_workflow 560 → other 503 → api_transport 439 等域，直至 4451 行清单全部给出终值。

### 分片十四：`pkg/strategy/indicatorbinding` 二次核对与回填（27 行）

范围：`parse_test.go` 22 行与 `parse_semantics_test.go` 5 行（键清单与分片五一致：`:8`/`:20`/`:46`/`:114`/`:155` 与 `:10`/`:93`/`:152`/`:176`/`:212`/`:235`/`:283`/`:304`/`:347`/`:384`/`:395`/`:425`/`:433`/`:467`/`:475`/`:501`/`:509`/`:542`/`:574`/`:609`/`:666`/`:720`）。
owner：`crates/jftrade-strategy`（planner 与 Pine 校验）、`crates/jftrade-engine`（预览对齐夹具）。

队列说明：该族在分片五已给出 27 行终值（17 partial + 10 boundary）。本分片按本批队列回到同族做二次核对，只闭合“分片五登记、当时未修”的缺口，并把新证据回填到对应行；其余 20 行保持分片五终值不变。

修复（5 处功能差异，均先红后绿，统一落在 `crates/jftrade-strategy/src/pine/planner.rs`）：

1. **非正整数周期被放行（对应 `parse_test.go:509`）**：`ta.sma(close, -3)` 产键 `ma:SMA:Negate3`（负号被渲染成 `Negate`），`ta.sma(close, 2.5)` 直接通过；参考实现的 `ParsePositiveInt` 拒绝 0、负数与非整数。修复：`ensure_positive_period` 由“仅拒非正数”收紧为“正整数或非字面量实参”，并新增 `planner_expression_text` 让带符号字面量按绑定文本渲染（`-3`）。
2. **两张时间单位表被混用（对应 `parse_semantics_test.go:20`、`parse_test.go:235`/`:283`）**：`request.security(syminfo.tickerid, "15m", ...)` 原先被接受，而参考实现的 `pineTimeframeUnit` 只认 1/5/15/30/45/120/240/D/W/M。修复：`indicator_time_unit` 删除 m 后缀分支，静态周期改由参考实现同款白名单裁决；新增 `dsl_time_unit` 复刻 `ParseIndicatorTimeUnitValue` 供 `ta.stoch` 末尾单位使用（`"60m"`→hour、`"001m"`→minute、`"15m"`→15m、`"0m"`/`"15"`/`"1D"`/`"badm"` 拒绝、bar/bars 归空单位、大小写不敏感）。
3. **价格源白名单缺失（对应 `parse_semantics_test.go:114`）**：`ta.rsi(typical, 14)`、`ta.kc(typical, 20, 2)` 等原先静默接受，参考实现的 `ParsePriceSource` 报源码不支持。修复：新增 `ensure_price_source`（open/high/low/close/volume/hl2/hlc3/ohlc4）并在 ma 族、rsi/cci、stdev、cum、window 族、vwap、mfi、obv、linreg、cog/cmo/dev/median/percentrank、kc/kcw、alma、tsi、correlation 与百分位族逐点校验；`ta.stoch` 按 `pkg/strategy/ir::parseStochSource` 额外排除 volume。
4. **源别名未展开（对应 `parse_semantics_test.go:114`）**：`src = hl2` 后 `ta.sma(src, 5)` 原先产键 `ma:SMA:5:src`；参考实现的 `resolveSourceAliases` 会先展开 OHLCV 别名。修复：planner 记录简单赋值别名（支持 `base = close` 这类链式），建键时使用展开后的源；无法解析的局部变量保持原样，避免把参考实现接受的脚本误判为非法。
5. **百分位百分比无校验（对应 `parse_test.go:542`/`:574`）**：`ta.percentile_linear_interpolation(close, 10, 101)` 原先通过、`50.0` 原样进键；参考实现的 `parsePercentileBinding` 要求 0–100 并把 `50.0` 规范化为 `50`。修复：新增 `percentile_percentage`（解析 f64 + 范围校验 + 规范化文本）。

引擎夹具修正（对齐后才成立的契约）：`crates/jftrade-engine/src/product_production_ports_strategy_tests.rs::test_strategy_preview_mtf_alignment_and_lower_timeframe_rejection` 原先用 `"1m"`/`"7m"`/`"15m"` 三个参考实现拒绝的静态周期构造低周期、未对齐、已对齐三例。改为白名单内且保留原意的 `"5"`（15m 图，低周期）、`"120"`（45m 图，未对齐）、`"15"`（5m 图，已对齐且预热 60 根），并新增第 4 段断言 `"7m"` 仍报 only static timeframe strings。

新增证据：`crates/jftrade-strategy/tests/pine_indicator_binding_parity.rs`（7 条用例，逐条带 `// Parity: go:452dea11:...` 锚点）——
`indicator_periods_require_positive_integer_literals`、`indicator_sources_follow_the_shared_ohlcv_whitelist`、
`indicator_sources_resolve_local_ohlcv_aliases`、`trailing_indicator_time_units_follow_the_dsl_parser`、
`trailing_indicator_time_units_accept_word_and_bar_forms`、`security_timeframes_use_the_static_pine_whitelist`、
`percentile_percentage_is_bounded_and_canonicalized`。

映射终值（27 行）：仍为 17 partial + 10 boundary。本分片不做状态升级——这些参考行测的是 Rust 已退役的指标绑定 DSL 助手，等价行为由 planner 分支承担；其中 7 行（`:20`、`:114`、`:235`、`:283`、`:509`、`:542`、`:574`）的结论与证据已刷新，其余 20 行保持分片五终值。

剩余缺口：

1. **数组形态 MTF 均线未展开（P2）**：`[a, b] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])` 在 Rust 仍退化为不透明 security 键（分片五修复只覆盖单调用形态）。
2. **均线类型表差异（P1）**：Rust 只识别 `ta.sma`/`ema`/`rma`/`wma`/`hma`/`vwma`，没有 MA/BOLL/TMA/EXPMA，也没有“未知类型→MA”归一。
3. **`request.security` 表达式 TA 白名单（P1，分片十三登记）**：表达式里的 `ta.sum` 等白名单外调用仍被放行。
4. **时间单位别名与 `%` 后缀（P2）**：`hr`/`hrs`/`mins`/`mon` 等 DSL 别名、`ParsePercentage` 的 `%` 后缀与 `quantityPct` 的 0–100 边界未逐项断言。
5. **非法 source 的处置路径（已记录，不视为缺陷）**：参考实现在建键助手处静默丢弃非法源、在 planner 采集处抛错，Rust 统一取抛错路径。
6. **DSL 助手族整体退役（boundary）**：函数调用解析、参数切分、函数名归一、参数元数契约、整数转字符串、保护窗口策略与四张归一缺省表在 Rust 无同形对象；升级路径仍为“若将来兼容导入该 DSL，按原表逐条移植并复刻拒绝形态”。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast --test pine_indicator_binding_parity`（工作树换回提交版本 planner） | 7/7 失败（104 条 97 通过 / 7 失败），失败点含 `ma:SMA:5:src`、`stoch:close:14:month`、`percentile_nearest_rank:hl2:10:50.0` 等 |
| 新增用例 | 同一命令（修复版 planner） | 7/7 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked --no-fail-fast` | 104/104 通过 |
| 生产文件摘要 | `shasum -a 256 crates/jftrade-strategy/src/pine/planner.rs` | `b14edae2483c921b6cf008a7835fd4a8b635ee01c0affe2f75c96a77f2fe2d77`（先红探针前后一致） |
| 映射写入 | 7 行 payload `/tmp/s128s14_payload.json` 经 `/tmp/b82_apply.py` 应用 | 7 行结论与证据刷新，`[x]` 1578 不变、partial 2235、boundary 638（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3272 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1609、unrecorded 0、stale 0、unknown 53 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1894/1895（预览对齐用例用参考实现不接受的 `"1m"` 夹具失败），修正夹具后复跑 1895/1895 通过 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2952 files）、`pnpm run check:quick` | 见下方记录 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 失败 |

后续：分片十五进入 `pkg/strategy/pineengine` 16 行与 `pkg/strategy/pinespec` 10 行（worker 客户端、payload/shadow、资产发现与规范章节/示例族）；随后按序推进 `pkg/strategy/pineworker`、`internal/strategy/pineruntime`/`service`/`runtimecontrol` 等子域，直至 strategy_pine 域清空，再进入 assistant_workflow 560 → other 503 → api_transport 439 → backtest_calendar 307 → storage_sqlite 196 → marketdata_quotes 161 → futu_opend 142 → trading_broker 56 → settings_watchlist 39。

### 分片十五：`pkg/strategy/pineengine` 16 行 + `pkg/strategy/pinespec` 10 行

范围：`pine_ts_client_test.go:10/:20/:56`、`pine_ts_payload_test.go:18/:39/:55/:72/:97/:131/:190/:207/:224/:236`、`pine_ts_runtime_test.go:12/:31/:58`（pineengine 16 行）；
`lint_helpers_test.go:5`、`skill_metadata_test.go:8/:80`、`spec_test.go:14/:28/:59/:108/:248/:274/:285`（pinespec 10 行）。
owner：`crates/jftrade-engine`（影子 payload 与外部引擎投影）、`crates/jftrade-strategy`（pinespec 章节/示例/兼容注册表）、`crates/jftrade-integration-pine`（worker 进程、就绪与 grpc 执行端口）。

队列说明：两族在更早批次已给出终值（本批分片五与分片七处理的是 `indicatorbinding` 与 `ir`）。本分片做二次核对：只补可被 Rust 同形面证明的证据、升级确有等价映射的行，其余保持终值并刷新与当前代码一致的证据引用。

新增证据（`crates/jftrade-engine/src/product_mcp_production_executor_tests.rs`，4 条单元用例）：

1. `pine_shadow_error_payload_keeps_the_worker_failure_message`：锁定 shadow_error payload 的 enabled/mode/engine/repository/ok/status 与单条 `PINETS_SHADOW_ERROR` 诊断的消息透传。
2. `pine_shadow_success_payload_projects_engine_metadata_and_counts`：锁定 shadow_ok payload 的 engineVersion 透传、license、differenceSummary 的 evaluated/plots/signals 计数、自定义诊断逐字段投影，以及无法分类诊断的回退码。
3. `pine_external_engine_payload_requires_the_agpl_notice_in_community_mode`：锁定 community-agpl 模式在缺失许可声明时先于分析端口返回 compliance_error，并清空 license/repository、给出 differenceSummary.reason。
4. `pine_external_engine_payload_reports_a_missing_analyzer`：锁定未配置分析端口时返回可投影为 shadow_error 的错误消息（`pine analyzer is not configured`）。

映射终值（26 行）：`[x]` 2 行、partial 21 行、boundary 3 行；其中 6 行结论与证据已刷新。

- **升级为 `[x]`（2 行）**：`:72 TestCommunityAGPLModeBlocksExecutionWhenNoticeCannotBeFound`（同一触发条件、同一诊断码 `PINETS_AGPL_NOTICE_MISSING`、同一 status；单元用例 + 端到端用例 `production_mcp_pine_community_mode_requires_agpl_notice_before_analyzer` 证明合规门在端口调用前生效）；`:97 TestExternalEnginePayloadFromResultMapsSuccessAndFailure`（成功/失败两条投影与 Go 的字段集逐项对齐，端到端用例覆盖经 MCP validate 叶子的投影；Rust 直接返回 JSON 映射，无独立 `PayloadMap` 助手）。
- **刷新证据（4 行）**：`:39`（payload 形态逐字段对齐，但 Rust 的失败触发是分析端口不可用而非按脚本检查 worker 脚本）、`:55`（投影面已对齐，真实 node worker 执行仅在显式 ignored 冒烟 `crates/jftrade-integration-pine/tests/real_worker_smoke.rs` 中验证）、`:18`（mode 表与禁用 payload 均已逐字段锁定；读 env 的一行包装因 edition 2024 的 `std::env::set_var` 为 unsafe 而无法在 crate 内驱动）、`:190`（Rust 把 worker 错误作为结构化 gRPC 状态投影进 shadow_error 诊断，没有 `workerError`/`stderrSuffix` 拼接形态）。
- **保持终值（20 行）**：其余 pineengine 行（客户端 stdio 协议族 `:10`/`:20`/`:56`/`:131`/`:207`、有界 stderr `:224`、Close 等待 `:236`、资产发现 `:12`/`:31`/`:58`）与 pinespec 10 行的既有结论仍与当前代码一致，本分片未改动。

剩余缺口：

1. **客户端协议面结构性差异（P2）**：Go 的 `PinetsWorkerClient` 走 stdin/stdout JSON 协议（含响应 id 校验、stderr 有界捕获 4096B、Close 等待自有 worker），Rust 走 gRPC（loopback + bearer token + `max_message_bytes` 预算），两者错误形态不同；Rust 侧的有界捕获与停机等待由 `crates/jftrade-integration-pine` 的进程/就绪用例承担，无逐字段等价的 4096B 断言。
2. **真实 worker 执行仅在显式冒烟中验证（P2）**：`crates/jftrade-integration-pine/tests/real_worker_smoke.rs::rust_client_executes_bundled_pinets_worker` 需 `JFTRADE_PINEWORKER_BUNDLE`/`JFTRADE_PINEWORKER_PROTO`/`JFTRADE_PINEWORKER_RUNTIME` 环境变量且标记 ignored；普通回归使用 mock worker 与记录端口。
3. **生成式支持快照未迁移（boundary）**：`spec_test.go:274 TestGeneratedPineSupportSnapshotIsCurrent` 仍为 boundary——`docs/reference/generated/pine-v6-support.md` 是冻结产物，Rust 没有重新生成并 diff 该快照的门禁（`pnpm run generate:reference` 只覆盖 API/类型文档），该约束由仓库门禁与规范章节冻结共同承担。
4. **pinespec 章节内容深度（P2）**：Rust 冻结了章节 id 集合与关键负载字段、示例选区与支持矩阵条目，但没有 Go 用例中逐章节正文级别的断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(pine_shadow) \| test(pine_external_engine_payload) \| test(pine_external_mode)'` | 7/7 通过（含既有 mode 表用例与 2 条既有端到端影子用例） |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 见下方整轮记录 |
| 映射写入 | 6 行 payload `/tmp/s128s15_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1578 → 1580、partial 2235 → 2233、boundary 638 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3276 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1614、unrecorded 0、stale 0、unknown 53（首轮 1 条 stale：新用例误挂 `pine_ts_runtime_test.go:31` 锚点而该行证据指向资产用例，已移除） |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2953 files）、`pnpm run check:quick` | compatibility exit 0（278 operations / 18 groups / 19 probes；desktop 3 平台档 6 link case 10 facade 4 event）；generated/ai-context/zero-go 通过；`check:quick` 首轮 1872/1929 失败于已知抖动用例 `api_launcher_reports_startup_failure_when_the_configured_address_is_taken`（隔离复跑 1/1 通过），次轮 exit 0（nextest 1929/1929，node 124+19+11+48 全绿） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 失败 |

后续：分片十六进入 `pkg/strategy/pineworker` 67 行（先 `client_test.go` 10 + `types_test.go` 8 + `proto_mapping_test.go` 5 + `grpc_*` 4 = 27 行，随后 `manager_test.go`/`process_launcher`/`runtime_boundaries`/`payload_size`/`process_smoke`/`hardcut_audit`/`proto_contract` 约 40 行）；之后 `internal/strategy/pineruntime` 25 → `internal/strategy` 其余子域，直至 strategy_pine 域清空。

### 分片十六：`pkg/strategy/pineworker` 首批 27 行（客户端/类型/协议映射/拨号）

范围：`client_test.go:12/:41/:56/:81/:92/:103/:114/:125/:143/:161`、`types_test.go:10/:21/:41/:92/:105/:147/:164/:171`、`proto_mapping_test.go:15/:154/:182/:197/:212`、`grpc_dialer_test.go:9/:23`、`grpc_transport_test.go:16/:71`。
owner：`crates/jftrade-integration-pine`（执行端口校验、进程/就绪/池、mock worker、asset 选择）、`workers/pineworker`（gRPC 服务端契约与 proto）。

队列说明：该族在更早批次已给过终值。本分片做二次核对，重点是 Go 侧 `ValidateRunScriptRequest` 契约在 Rust 执行端口里的逐条等价性——核对发现 Rust 的 `validate_request`/`validate_candle` 已实现同一张表，但过去只有“超长源码 + high 越界”两条断言，逐字段契约没有被回归锁定。

新增证据（`crates/jftrade-integration-pine/src/execution/tests.rs`，5 条用例）：

1. `request_validation_rejects_every_incomplete_or_inconsistent_field`：合法请求通过 + 9 个拒绝形态（缺 job/source/symbol/timeframe、`mode=scan`、无 K 线、high<low、open 越界、负 volume），逐条断言与参考实现同义的错误文案。
2. `analyze_mode_accepts_an_empty_candle_list`：analyze 模式允许空 K 线，backtest 仍报 `candles are required`。
3. `live_session_contract_requires_identity_mode_and_revisions`：open/append/close 三段合法序列 + 5 个拒绝形态（未支持操作、会话要求 live、open 必须 revision 0、append 必须正 revision、操作必须带 session id）。
4. `request_validation_enforces_the_candle_limit`：`max_candles=1` 时两根被拒（`too many candles`）、一根通过。
5. `non_finite_candle_values_are_rejected_before_transport`：`open=NaN`、`high=+Inf`、`volume=-Inf` 三个形态在传输前失败（`must be finite`）。

映射终值（27 行）：`[x]` 4 行、partial 20 行、boundary 3 行；其中 5 行结论与证据刷新。

- **升级为 `[x]`（4 行，均为 types 校验表）**：`:41 TestValidateRunScriptRequest`、`:92 TestValidateRunScriptRequestAnalyzeModeAllowsNoCandles`、`:105 TestValidateRunScriptRequestLiveSessionContract`、`:147 TestValidateRunScriptRequestRejectsTooManyCandles`。
- **刷新证据（1 行）**：`:164 TestRunScriptPayloadSizeRejectsNonFiniteCandle` 仍为 partial——Rust 没有 `jsonSize` 体积估算入口，非有限字段改在编码前校验阶段拒绝（`must be finite`），拒绝点与文案不同但强度等价。
- **保持终值（22 行）**：客户端元数据缺省与体积/性能门（`client_test.go:12`/`:56`/`:125`/`:143`）、传输/超时/worker 错误映射（`:81`/`:92`/`:103`/`:114`）、`NewClient` 依赖校验（`:161`）、运行时归一与默认 worker 规模（`types_test.go:10`/`:21`）、性能门（`:171`）、proto 往返/金标/health 能力/nil 边界（`proto_mapping_test.go` 5 行）、拨号与传输边界（`grpc_dialer_test.go` 2 行、`grpc_transport_test.go` 2 行）——既有结论与当前代码一致，本分片未改动。

剩余缺口：

1. **运行时归一与默认 worker 规模无 Rust 同形对象（P2）**：Go 的 `NormalizeRuntime`/`SupportsRuntime`（`pine-go-plan` → `pine-pinets`）与 `DefaultWorkerConfig(cpu)`（live/backtest/optimization 随 CPU 缩放、默认不限消息与 K 线数）在 Rust 分别由 `crates/jftrade-engine` 的 runtime 标识常量/落库归一与 `PineExecutionConfig` 的显式上限承担；没有同名函数的逐值断言。
2. **客户端元数据缺省（P2）**：Go 的 `Client.RunScript` 会回填 duration/requestBytes/responseBytes 并支持性能门；Rust 的 gRPC 执行端口不做响应元数据回填，性能门这一层在 Rust 由进程就绪/超时与 wire 预算承担（`grpc_request_message_limit_has_exact_encoded_boundaries`）。
3. **proto 映射面差异（P2）**：Go 的 `proto_mapping_test.go` 断言自定义 proto 映射与金标向量；Rust 走 prost 生成的 `proto/pineworker` 冻结输入（`tests/pineworker_proto_frozen_inputs.rs` 锁定三份 proto 的树摘要与文件数），没有逐字段映射往返用例。
4. **真实进程冒烟默认忽略（P2）**：`process_smoke_test.go` 对应的 Rust 冒烟（`tests/real_worker_smoke.rs`）需环境变量并标记 ignored；普通回归使用 mock worker/记录端口。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked` | 见下方受影响 crate 记录（5 条新用例全绿） |
| 受影响 crate | `cargo test -p jftrade-integration-pine --lib` | 37/37 通过 |
| 映射写入 | 5 行 payload `/tmp/s128s16_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1580 → 1584、partial 2233 → 2229、boundary 638 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3276+5 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1619、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过（clippy 首轮两条 `type_complexity` 报错，改为 `RequestMutation`/`ValidationCase`/`SessionCase` 类型别名后转绿） |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1899/1899 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`（2953 files）、`pnpm run check:quick` | compatibility exit 0（278 operations / 18 groups / 19 probes；desktop 3 平台档 6 link case 10 facade 4 event）；generated/ai-context/zero-go 通过；`check:quick` 首轮 exit 0（nextest 1974/1974 + node 124+19 全绿），`.rcgu.o` 计数 0 未触发清理 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 失败 |

后续：分片十七进入 `pkg/strategy/pineworker` 余量约 40 行（`manager_test.go` 13、`process_launcher_boundaries_test.go` 5、`process_launcher_test.go` 7、`runtime_boundaries_test.go` 7、`payload_size_test.go` 3、`process_smoke_test.go` 2、`hardcut_audit_test.go` 1、`manager_readiness_recovery_test.go` 1、`proto_contract_test.go` 1）；之后 `internal/strategy/pineruntime` 25 → `internal/strategy` 其余子域，直至 strategy_pine 域清空。

### 分片十七：`pkg/strategy/pineworker` 余量 40 行（管理器生命周期、进程启动边界、体积与恢复）

范围：`manager_test.go` 13 行（`:14`/`:46`/`:74`/`:119`/`:158`/`:200`/`:236`/`:262`/`:281`/`:299`/`:324`/`:341`/`:359`）、`payload_size_test.go` 3 行（`:9`/`:26`/`:49`）、`process_launcher_boundaries_test.go` 5 行（`:19`/`:70`/`:94`/`:118`/`:165`）、`process_launcher_test.go` 7 行（`:16`/`:72`/`:88`/`:107`/`:114`/`:133`/`:141`）、`runtime_boundaries_test.go` 7 行（`:14`/`:39`/`:66`/`:79`/`:107`/`:134`/`:189`）、`process_smoke_test.go` 2 行（`:20`/`:34`）、`hardcut_audit_test.go:12`、`manager_readiness_recovery_test.go:15`、`proto_contract_test.go:14`。

owner：`crates/jftrade-integration-pine`（`pool.rs` worker 池与预留、`process.rs` 进程启停、`readiness.rs` 就绪监视、`asset.rs` 与 `runtime_dependencies.rs` 资产/依赖解析）、`crates/jftrade-engine`（运行时装配、排队与背压）、`crates/jftrade-settings`（worker 运行时配置）。

队列说明：该族在更早批次已给过终值，本分片做二次核对——重点是 Go `WorkerManager` 的池语义（健康轮转、open 前预留、live pin、健康/重启投影、失败关闭）在 Rust `WorkerPool` 上是否有逐条回归。核对发现 `pool.rs` 过去只有 pin 与重启释放两条用例，轮转、并发 open 预留、close 释放、健康快照投影与空池/未知 worker 失败关闭这五类语义没有被回归锁定。

新增证据（`crates/jftrade-integration-pine/src/pool.rs`，5 条用例）：

1. `healthy_workers_are_selected_in_rotation`：不健康 worker 永不接单；池内全忙时返回 `PoolError::CapacityExceeded`；恢复健康后按 1→2→1 轮转（Go 断言 3 次请求 2/1 分布）。
2. `reserving_an_open_session_blocks_a_second_open`：同一 session 的二次 open 报 `SessionAlreadyOpen`；未完成 open 期间的 append 报 `CapacityExceeded`；未知 session 报 `SessionNotFound`；缺 session id 报 `MissingSession`。
3. `closing_a_live_session_releases_its_pin`：live 会话固定首个 worker、普通请求取另一 worker、close 后 append 报 `SessionNotFound`。
4. `health_results_and_restarts_project_into_the_snapshot`：逐字段断言初始快照（id/地址/healthy/busy/restarts/lastError）与健康投影（`pine_ts_version`、capabilities、`last_error` 清空）、重启计数递增、未知 worker 重启报 `WorkerNotFound`。
5. `empty_pools_and_unknown_workers_fail_closed`：空池构造报 `PoolError::Empty`；未注册 worker 在 `record_health`/`record_restart`/`release` 三个入口统一报 `WorkerNotFound`。

映射终值（40 行）：`[x]` 3 行、partial 36 行、boundary 1 行；其中 8 行结论与证据刷新。

- **升级为 `[x]`（3 行）**：`:46 TestWorkerManagerRunScriptRoundRobinsHealthyWorkers`、`:74 TestWorkerManagerPinsLiveSessionAndClearsItOnClose`、`:119 TestWorkerManagerReservesLiveSessionBeforeOpenCompletes`。
- **boundary 转 partial（1 行）**：`:200 TestWorkerManagerRunScriptRejectsWhenBusyIfConfigured`——Rust 无 RejectWhenBusy 开关，池满一律 `CapacityExceeded`，容量与背压由 engine 侧持有，错误体不带 worker 数；拒绝路径已由轮转用例锁定，故从 boundary 收回到 partial。
- **刷新证据（4 行）**：`:14 TestWorkerManagerStartStopAndSnapshot`（Start/Stop/Snapshot 三层拆分）、`:236 TestWorkerManagerCheckHealthRestartsFailedWorker`、`:262 TestWorkerManagerCheckHealthReportsRestartFailure`（两条均落到健康/重启投影用例）、`:359 TestWorkerManagerRequiresDependenciesAndStart`（依赖注入在 Rust 无 nil 注入点，改由空池与未知 worker 失败关闭证据承担）。
- **保持终值（32 行）**：`:158` 队列语义（boundary）、dial 失败清理与诊断（`:281`/`:299`）、dial 重试直到就绪（`:324`）、Stop 返回首个 close 错误（`:341`）、就绪监视退出（`manager_readiness_recovery_test.go:15`）、体积边界（`payload_size_test.go` 3 行）、进程启动边界（`process_launcher_boundaries_test.go` 5 行）、进程启动器（`process_launcher_test.go` 7 行）、运行时边界（`runtime_boundaries_test.go` 7 行）、冒烟（`process_smoke_test.go` 2 行）、硬切审计（`hardcut_audit_test.go:12`）、proto 契约（`proto_contract_test.go:14`）——既有结论与当前代码一致，本分片未改动。

剩余缺口：

1. **队列语义无 Rust 同形对象（P1）**：Go `TestWorkerManagerQueuesWhenAllWorkersBusy`（`manager_test.go:158`）断言池满且未开 RejectWhenBusy 时请求排队等待释放；Rust 的 `WorkerPool` 池满即拒（`CapacityExceeded`），排队由 engine 的并发/背压层承担，没有“池内排队 + 等待超时”的逐条断言。升级路径：若 engine 侧背压层要覆盖该语义，应在 `crates/jftrade-engine` 补“池满→排队→释放后放行→超时拒绝”的集成用例，而不是把队列塞回 `pool.rs`。
2. **dial 失败清理与诊断字段（P1）**：`manager_test.go:281`/`:299` 断言的清理顺序与诊断字符串拼接由 `crates/jftrade-integration-pine/src/process.rs` 与 `readiness.rs` 的现有用例部分覆盖，Rust 侧没有等价的“启动失败后 worker 列表回滚”单点断言。
3. **dial 重试直到就绪（P1）**：`manager_test.go:324` 的重试直到成功语义在 Rust 由 `readiness.rs` 的健康门与 engine 装配承担，缺少“先失败 N 次后成功”的计数断言。
4. **Stop 错误聚合（P2）**：`manager_test.go:341` 断言 Stop 返回首个 close 错误并清空快照；Rust 的 `PineProcessPool`/engine 关闭路径记录错误但不逐条比对“首个错误”的选取。
5. **体积估算入口缺失（P2）**：`payload_size_test.go` 的 `jsonSize` 估算在 Rust 没有同形函数，边界改由 `execution/tests.rs` 的编码上限用例（`grpc_request_message_limit_has_exact_encoded_boundaries`）承担。
6. **真实进程冒烟默认忽略（P2）**：`process_smoke_test.go:20`/`:34` 对应的 Rust 冒烟（`tests/real_worker_smoke.rs`）需环境变量并标记 ignored。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked` | 50/50 通过（1 条真实进程冒烟 skipped），5 条新用例全绿 |
| 映射写入 | 7 + 1 行 payload（`/tmp/s128s17_payload.json`、`/tmp/s128s17_payload2.json`）经 `/tmp/b82_apply.py` 应用 | `[x]` 1584 → 1587、partial 2230 → 2228、boundary 637 → 636（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3281 + 5 = 3286 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1626、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1899/1899 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 见提交前记录 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 8 条 `advisory-not-detected` 失败 |

后续：分片十八进入 `internal/strategy/pineruntime` 25 行（`runtime_test.go` 13、`runtime_failure_contracts_test.go` 7、`runner_lifecycle_test.go` 4、`recovery_contracts_test.go` 1）；之后按 `internal/strategy` 族余量继续推进，直至 strategy_pine 域清空。

### 分片十八：`internal/strategy/pineruntime` 25 行（配置解析、运行期路径优先级、runner 生命周期）

范围：`runtime_test.go` 13 行（`:18`/`:60`/`:79`/`:105`/`:118`/`:145`/`:165`/`:185`/`:228`/`:250`/`:278`/`:325`/`:344`）、`runtime_failure_contracts_test.go` 7 行（`:16`/`:30`/`:52`/`:76`/`:99`/`:146`/`:171`）、`runner_lifecycle_test.go` 4 行（`:41`/`:59`/`:110`/`:157`）、`recovery_contracts_test.go:9`。

owner：`crates/jftrade-engine`（运行期依赖解析与 node 候选、桌面运行期配置、装配与关停）、`crates/jftrade-settings`（PineWorkerSettings 归一与默认值）、`crates/jftrade-integration-pine`（资产校验与物化、进程启停、就绪监视、worker 池与会话）。

队列说明：该族在更早批次已给过终值，本分片做二次核对——重点是参考实现里 `ResolveConfig` 的三件事（settings 与环境合并、嵌入/外部资产切换、运行期路径优先级）在 Rust 的拆分实现是否被回归锁定。核对发现运行期路径优先级链（settings > `JFTRADE_PINEWORKER_RUNTIME` > `JFTRADE_NODE_BINARY` > PATH）在 Rust 只有“设置路径优先”一段断言，环境变量层的顺序与空白回落没有任何证据，因此本分片补了可注入接缝与用例。

新增证据：

1. `crates/jftrade-engine/src/runtime_dependencies.rs` 提取 `node_candidates_with(configured_path, lookup)` 接缝（无行为变更），`node_candidates` 继续用 `env::var_os` 调用；新增用例 `runtime_path_precedence_prefers_settings_then_worker_env_then_legacy_binary`，断言四个层级：设置路径压过两个环境变量（`source = settings`）、worker 环境变量压过 legacy（`source = env:JFTRADE_PINEWORKER_RUNTIME`）、仅 legacy 时取 `env:JFTRADE_NODE_BINARY`、worker 环境变量为空白时回落 legacy，并覆盖两侧嵌套引号与空白裁剪；用例带 `// Parity: go:452dea11:internal/strategy/pineruntime/runtime_test.go:105` 锚点。
2. `crates/jftrade-settings/src/pine_worker.rs` 的 `worker_limits_and_nested_quotes_match_go_settings_owner` 增加 `instance_worker_limit` 默认值断言（10），补齐参考实现默认 worker 规模（backtest 2 / instance 10）的 Rust 侧证据。

探针（证明新增断言有效）：把生产代码的候选顺序临时改成 legacy 优先（其余不动），只跑 `runtime_path_precedence_prefers_settings_then_worker_env_then_legacy_binary` 得到 1 失败（`left: legacy-node` / `right: /env/node`），按字节回滚后 13/13 通过；`runtime_dependencies.rs` shasum `d21a3f8c…`（改动前）→ `ac3050c5…`（探针回滚后，含接缝与新用例）。

映射终值（25 行）：partial 24 行、boundary 1 行；本分片无新增 `[x]`——参考实现的 `ResolveConfig`/`Manager`/`ephemeralRunner` 在 Rust 分别落到设置归一、engine 装配与进程/池三处，逐条都是“语义等价但对象不同”，按执行标准不把结构性差异记成等价。

- **保持 boundary（1 行）**：`recovery_contracts_test.go:9`（Go 的 nil 接收者防御分支在 Rust 由类型系统排除，升级路径写进结论）。
- **刷新 partial 证据（24 行）**：`:105` 改引新增的运行期优先级用例，`:30` 同源引用；`:18`/`:79` 指向设置归一用例并登记 0 值语义缺口；资产族 `:60`/`:16` 指向 `asset.rs` 两条用例；进程族 `:118`/`:145`/`:325`/`:344` 指向 `process.rs` 的 loopback 与身份探针用例；就绪族 `:41`/`:110`/`:146`/`:171` 指向 `readiness.rs` 的监视停止与停止后迟到结果用例；会话族 `:59`/`:157`/`:278` 指向 `pool.rs` 与执行端口契约用例。

剩余缺口（本批新增/细化）：

1. **worker 上限 → 实际 worker 数缺失（P1）**：参考实现把 `BacktestWorkerLimit`/`InstanceWorkerLimit` 换算成实际 worker 数，并把 0 解释成默认 2/10；Rust 的 `normalize_pine_worker_settings` 把 0 clamp 成 1，且没有任何消费方把上限换算成池规模（`JFTRADE_PINEWORKER_WORKERS` 与池大小由调用方另给）。修复位置：`crates/jftrade-settings` 的归一层区分“0=取默认”与“越界=报错或收敛”，并在 engine 装配处消费该上限；回归按 `runtime_test.go:79`/`:105` 与 `runtime_failure_contracts_test.go:30` 断言。
2. **禁用开关缺失（P1）**：参考实现的 `JFTRADE_PINEWORKER_DISABLED` 会整体禁用运行时；Rust 没有等价开关，部署只能通过不提供 bundle/proto 表达。修复位置：engine 的桌面运行期配置解析（`from_process_env`）与装配短路；回归要求：开关为真时不构建进程、不发布运行期句柄。
3. **配对发布与回滚缺回归（P1）**：参考实现的 Manager 在任一 Runner 构建失败时不发布半成品并回滚；Rust 的 engine 装配没有“第二个组件失败 → 第一个组件回滚且不对外发布”的用例（`runtime_test.go:185`/`:228`、`runtime_failure_contracts_test.go:76`）。
4. **关停排空（drain）缺回归（P1）**：参考实现 Close 会排空活跃 live 会话并归还容量（`runtime_failure_contracts_test.go:171`）；Rust 由监视任务 join + 进程 stop_timeout + 池层释放组合承担，缺少端到端用例。
5. **容量等待与取消缺回归（P1）**：参考实现的 runner 在容量满时排队等待并可被上下文取消（`runtime_failure_contracts_test.go:99`）；Rust 池即时拒绝（`CapacityExceeded`），排队/背压在 engine 层，缺“排队 → 取消 → 释放后放行”的用例。
6. **工作目录与 proto 自动定位缺失（P2）**：参考实现从 bundle 路径向上找仓库根并支持 proto 覆盖（`runtime_test.go:118`/`:165`、`runtime_failure_contracts_test.go:52`）；Rust 要求调用方显式给出工作目录与 proto。
7. **停机超时推导缺失（P2）**：参考实现的 stopTimeout 默认 5s、并被请求超时压到 10s 上限（`runtime_test.go:325`）；Rust 用显式 `stop_timeout`，没有默认与上限推导。
8. **关停错误聚合缺失（P2）**：参考实现的 CloseRunners 聚合两个组件关闭错误（`runtime_test.go:250`）；Rust 的 engine 关闭路径没有逐条断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked runtime_dependencies` | 13/13 通过（含新增优先级用例） |
| 探针先红 | 同命令过滤 `runtime_path_precedence`（候选顺序临时改成 legacy 优先） | 1 failed（`left: legacy-node` / `right: /env/node`），按字节回滚后恢复绿 |
| 设置侧断言 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings --all-targets --locked` | 59/59 通过 |
| 映射写入 | 25 行 payload `/tmp/s128s18_payload.json` 经 `/tmp/b82_apply.py` 应用 | 25 行结论全部刷新为终值（24 partial + 1 boundary），计数不变：`[x]` 1587 + partial 2228 + boundary 636 = 4451 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3286 + 1 = 3287 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1627、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1900/1900 通过（新增用例计入） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 operations / 18 route groups / 19 probes；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：分片十九进入 `internal/strategy` 其余子域（`service_test.go` 10、`pine_live_command_test.go` 11、`runtimecontrol` 10、`liveruntime` 余量约 22、`definition` 4、`types_test.go` 2、`errors_test.go` 1），直至 strategy_pine 域清空。

### 分片十九：`internal/strategy` 余量第一批 24 行（service 门面、实盘命令映射、类型契约）

范围：`service_test.go` 10 行（`:138`/`:165`/`:174`/`:198`/`:210`/`:223`/`:236`/`:255`/`:278`/`:304`）、`pine_live_command_test.go` 11 行（`:16`/`:36`/`:60`/`:76`/`:93`/`:130`/`:180`/`:194`/`:227`/`:257`/`:267`）、`types_test.go` 2 行（`:8`/`:42`）、`errors_test.go:8`。

owner：`crates/jftrade-engine`（product 策略路由与写端口、strategy runtime 生命周期与执行意图解析）、`crates/jftrade-strategy`（领域类型与错误分类）、`crates/jftrade-backtest`（括号单与原子撮合）、`crates/jftrade-api`（transport 校验与错误映射）。

队列说明：该族在更早批次已给过终值，本分片做二次核对——重点是参考实现 `Service` 门面里的“启动编排”（校验 → 启动 → 状态转换 → 刷新行情流，失败回滚）是否在 Rust 有对应断言，以及分析请求的输入校验优先级。核对发现分析入口的输入校验（非法 sourceFormat 在端口之前被拒）在 Rust 有完整断言，因此本行升级为 `[x]`；启动编排则分散在写端口、运行时 owner 与 API 错误映射三层，确认无同形断言，保留 partial 并逐条登记缺口。

新增证据（本分片只补锚点，未新增测试）：`crates/jftrade-engine/tests/strategy_pine_compatibility.rs` 的 `strategy_pine_applies_input_validation_and_error_precedence_before_the_port` 增加 `// Parity: go:452dea11:internal/strategy/service_test.go:198 TestServiceAnalyzePineRejectsUnsupportedSourceFormat` 锚点（断言集合未改：两种非法 sourceFormat 与畸形 JSON 均 400 BAD_REQUEST、端口调用列表为空、合法请求归一为 PINE_V6_SOURCE_FORMAT）。

映射终值（24 行）：`[x]` 1 行（`:198` 由 partial 升级）、partial 21 行、boundary 2 行（`pine_live_command_test.go:180`/`:194`，括号单与 OCO 展开归 jftrade-backtest 撮合 owner）。

- **升级为 `[x]`（1 行）**：`service_test.go:198`——参考实现的“不支持的 sourceFormat 在分析器之前被拒”，Rust 在端口边界给出同义断言（400 + 端口零调用 + 合法请求归一）。
- **保持 boundary（2 行）**：`:180` 不支持退出括号、`:194` 原子 OCO 展开，均在 jftrade-backtest 的保守 K 线执行器里完成校验与原子执行，拒绝点与错误归属迁移到撮合 owner。
- **刷新 partial 证据（21 行）**：service 门面族（`:138` 门面 vs 路由+端口的对象差异、`:165` 存储错误传播、`:174` 分析器注入与默认 sourceFormat、`:210` 启动前拒绝、`:223` 容量→忙碌映射、`:236` 三条失败路径与回滚、`:255` 行情流刷新计数、`:278` 转换顺序与刷新、`:304` 门面入口清单），命令映射族（`:16`/`:36`/`:60`/`:76`/`:93`/`:130`/`:227`/`:257`/`:267`），类型契约族（`types_test.go:8`/`:42`）。

剩余缺口（本批新增/细化）：

1. **启动编排无同形断言（P1，交易安全）**：参考实现的 `StartInstance` 顺序是“查找 → ValidateStartable → 运行时启动 → 状态转换 → 刷新行情流”，且（a）状态转换失败要回滚已启动的运行时（stop 被调用），（b）成功后行情流刷新恰好一次，（c）不可启动时运行时零调用。Rust 三层分工（写端口校验、runtime owner、API 错误映射）缺少这三条断言（`service_test.go:210`/`:236`/`:255`/`:278`）。修复/补测位置：`crates/jftrade-engine` 的 strategy runtime 写路径集成用例。
2. **存储错误传播缺回归（P2）**：定义存储报错需要原样上抛（保留错误身份），Rust 只有“端口缺失即失败关闭”的用例（`service_test.go:165`）。
3. **忙碌文案与设置指引不对齐（P2）**：参考实现的容量耗尽错误带“运行实例 Worker 最大值”的设置指引，Rust 的 429 STRATEGY_PINE_BUSY 只带 Retry-After，指引文案不存在（`service_test.go:223`）。
4. **作用域退出数量保留缺专用断言（P1，交易语义）**：限定到入场 ID 的退出必须保留数量语义（`pine_live_command_test.go:93`）。
5. **条件单类型未逐类型比对（P2）**：Rust 只锁定订单类型与 OpenD wire 枚举对齐，参考实现的条件单类型集合没有逐项对照（`pine_live_command_test.go:130`）。
6. **入场数量缺省与卖出开仓方向缺同形断言（P2）**：缺省数量 1 与“卖出开仓 → 做空”两条只有间接覆盖（`pine_live_command_test.go:257`/`:227`）。
7. **类型契约缺少逐字段快照（P2）**：DefinitionView 扁平化与实例绑定 JSON 契约在 Rust 由 serde 结构 + 语料比对承担，没有独立的结构断言（`types_test.go:8`/`:42`）。
8. **门面入口缺少清单式断言（P2）**：参考实现的 Service 入口清单在 Rust 没有一一对应的清单断言，只能靠多个族用例拼出（`service_test.go:138`/`:304`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 锚点用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --test strategy_pine_compatibility --locked` | 7/7 通过（新增锚点不影响断言集合） |
| 映射写入 | 24 行 payload `/tmp/s128s19_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1587 → 1588、partial 2228 → 2227、boundary 636 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3287 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1628、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1900/1900 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0（前端 10 文件 98 用例等全绿） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：分片二十进入 `internal/strategy/runtimecontrol` 10 行（`optional_values_risk_off_test.go` 2、`policy_test.go` 7、`semantics_test.go` 1）与 `liveruntime` 余量，之后回补 `catalog` 余量（22 行）与 `definition` 4 行，直至 strategy_pine 域清空。

### 分片二十：`internal/strategy/runtimecontrol` 10 行 + `liveruntime` 关停/订阅/符号族 11 行（含时区功能修复）

范围：`runtimecontrol/optional_values_risk_off_test.go` 2 行（`:8`/`:33`）、`runtimecontrol/policy_test.go` 7 行（`:11`/`:53`/`:68`/`:83`/`:102`/`:120`/`:146`）、`runtimecontrol/semantics_test.go:5`、`liveruntime/manager_close_test.go` 3 行（`:86`/`:140`/`:188`）、`liveruntime/nil_boundaries_test.go` 2 行（`:10`/`:43`）、`liveruntime/runtime_risk_evidence_test.go:18`、`liveruntime/subscription_lifecycle_test.go` 3 行（`:14`/`:58`/`:81`）、`liveruntime/symbol_failure_business_test.go` 2 行（`:14`/`:57`）。

owner：`crates/jftrade-trading`（风险求值器 `risk.rs`、持仓匹配与可卖数量 `portfolio.rs`）、`crates/jftrade-engine`（策略运行时的每日订单窗口、风控审计与暂停、关停顺序、订阅需求与租约、符号级行情循环）、`crates/jftrade-calendar`（市场本地午夜）、`crates/jftrade-marketdata`（订阅引用与租约）。

队列说明：该族此前只做过结构映射，本分片做行为核对。核对发现一处真实功能差异：参考实现的每日订单窗口按 **订单符号所属市场的本地午夜** 切分（`US` → America/New_York、`HK` → Asia/Hong_Kong，含夏令时夜盘），而 Rust 的策略运行时用 **UTC 午夜** 作为窗口起点，导致美港股在 UTC 00:00–市场午夜之间提交的订单被算进错误的交易日。本分片修复该差异并补回归；另外把风险求值、持仓匹配、监控模式等已有等价断言的条目升级为 `[x]`。

功能修复与新增证据：

1. `crates/jftrade-engine/src/strategy_runtime_execution.rs` 新增 `strategy_market_day_start_ms(market, now_utc)`：`US`/`HK`/`CN`/`SH`/`SZ` 走 `jftrade_calendar::market_day_start_for_market` 取市场本地午夜（毫秒），其它市场或日历失败回落到 UTC 午夜，不阻断下单路径；`today_midnight_ms` 改由该函数计算，每日订单计数与日订单预留共用同一日界。
2. 新增用例 `strategy_market_day_start_follows_the_order_market_timezone`（US 2025-12-31T05:00Z、HK 2025-12-31T16:00Z、未知市场回落 UTC 午夜）与 `strategy_market_day_start_follows_dst_transition`（2026-06-14 美东夏令时夜盘日界 2026-06-14T04:00Z），两条都带 `// Parity: go:452dea11:internal/strategy/runtimecontrol/policy_test.go:83` 锚点。
3. 探针（证明修复被断言咬住）：把 `strategy_market_day_start_ms` 临时改成恒返回 UTC 午夜，两条用例同时先红（`left: 1767232800000` / `right: 1781409600000` 等），按字节回滚后 2/2 通过。`strategy_runtime_execution.rs` shasum `9a01c3c8…`（改动前）→ `a960775c…`（修复后）。
4. `crates/jftrade-trading/tests/risk_engine_tests.rs`：为风险原因表用例补 `policy_test.go:11` 锚点；监控模式用例补参考实现的日计数场景（monitor + `daily_max_orders` + 当日 3 单 → matched、不拒绝、detail 前缀 `rule=daily_max_orders`）与 `policy_test.go:53` 锚点。
5. `crates/jftrade-trading/src/portfolio.rs`：为持仓匹配/可卖数量用例补 `policy_test.go:120` 锚点。

映射终值（21 行）：`[x]` 4 行（`:11`/`:53`/`:83`/`:120`）、partial 15 行、boundary 2 行（`policy_test.go:146`、`semantics_test.go:5`；另有 `nil_boundaries_test.go:10` 同批按 boundary 保留）。

- **升级为 `[x]`（4 行）**：风险原因表（`:11`）、监控模式记录不拒绝（`:53`）、市场本地日界（`:83`，含修复）、持仓符号匹配与可卖数量（`:120`）。
- **保持 boundary（3 行）**：`policy_test.go:146` 负零格式化（Rust 定点数无带符号零，由类型表示保证）、`semantics_test.go:5` 实盘 Pine 能力清单（Rust 无限制列表对象）、`nil_boundaries_test.go:10` nil 接收者空状态边界。
- **保持 partial（15 行）**：可选值/时间辅助（`:8`）、观测投影文本格式（`:102`）、关停聚合与等待（`manager_close_test.go:86`/`:140`/`:188`）、命令回调委托（`nil_boundaries_test.go:43`）、风控审计与暂停的部分差异（`runtime_risk_evidence_test.go:18`）、订阅租约回滚与 panic 释放（`subscription_lifecycle_test.go:14`/`:58`）、订阅引用符号归一（`:81`）、行情轮询与成交桶边界（`symbol_failure_business_test.go:14`/`:57`）。

剩余缺口（本批新增/细化）：

1. **关停错误聚合缺回归（P1）**：参考实现把每个具名会话的关闭错误聚合后返回，并保证并发 Close 只上报一次（`manager_close_test.go:86`）；Rust 只记录日志不返回聚合错误，也没有并发幂等断言。
2. **启动/关停竞态与顺序缺回归（P1/P2）**：等待进行中启动并回收其关闭错误（`:140`）、以及“后台同步先退出再关 Pine 会话”的顺序（`:188`）在 Rust 无断言。
3. **订阅租约失败回滚缺回归（P1）**：租约获取失败不建运行时、预热失败只释放一次租约、异常路径仍释放租约（`subscription_lifecycle_test.go:14`/`:58`）。
4. **订阅引用符号归一缺回归（P1）**：小写与空白归一（` us.aapl ` → `US.AAPL`）与周期映射（15m）在 Rust 缺同形断言（`:81`）。
5. **成交桶边界缺回归（P1）**：缺时间戳成交仍建当前桶、乱序（重连后迟到）成交不污染当前桶（`symbol_failure_business_test.go:14`）。
6. **观测文本精度与错误裁剪缺回归（P2）**：微秒精度时间文本与 `last_error` 首尾裁剪（`policy_test.go:102`）。
7. **风控计数失败策略差异（P2）**：参考实现在审计计数失败时按 0 计数（容错下单），Rust 直接上抛并拒绝本单（失败关闭），需要在文档/设置页说明或补显式策略（`runtime_risk_evidence_test.go:18`）。
8. **可选值辅助与负零格式化无同形对象（P2）**：Optional*/MaxTime 辅助与负零格式化在 Rust 由类型表示与调用方比较承担（`optional_values_risk_off_test.go:8`、`policy_test.go:146`）。
9. **关停期迟到回调缺回归（P2）**：`symbol_failure_business_test.go:57` 的日志兜底与父取消可见性在 Rust 无同形断言。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 修复前先红 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked strategy_market_day_start`（恒返回 UTC 午夜的探针） | 2/2 失败 |
| 修复后 | 同命令 | 2/2 通过 |
| 受影响 crate | `node scripts/quality/cargo-nextest.mjs run -p jftrade-trading --all-targets --locked` | 85/85 通过 |
| 映射写入 | 21 行 payload `/tmp/s128s20_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1588 → 1592、partial 2227 → 2222、boundary 636 → 637（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3289 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1632、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1906/1906 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`jftrade-integration-pine` 50/50、`pnpm run check:quick` | compatibility exit 0（desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 首轮因已知抖动（api_launcher 起始地址用例）失败，隔离复跑 2/2 通过、第二轮 `check:quick` exit 0（按规则不把抖动记为通过） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：分片二十一进入 `liveruntime` 余量（`manager_boundaries` 11、`order_risk` 余量 3、`pineworker_live` 6、`product_lifecycle` 4、`runtime_boundaries` 6），之后回补 `catalog` 余量 22 与 `definition` 4，直至 strategy_pine 域清空。

### 分片二十一：`liveruntime` 余量 30 行（管理边界、撤单跟踪、pine 实盘会话、账户与刷新边界）+ 分片二十判定纠正

范围：`manager_boundaries_test.go` 11 行（`:16`/`:54`/`:94`/`:116`/`:155`/`:182`/`:216`/`:241`/`:275`/`:302`/`:356`）、`order_risk_business_test.go` 余量 3 行（`:135`/`:216`/`:235`）、`pineworker_live_business_test.go` 6 行（`:226`/`:270`/`:305`/`:368`/`:404`/`:527`）、`product_lifecycle_business_test.go` 4 行（`:18`/`:56`/`:96`/`:151`）、`runtime_boundaries_test.go` 6 行（`:21`/`:48`/`:116`/`:180`/`:258`/`:277`），以及分片二十的 `runtimecontrol/policy_test.go:83` 结论纠正。

owner：`crates/jftrade-engine`（策略运行时装配、写端口与状态投影、执行意图与撤单、行情刷新）、`crates/jftrade-strategy`（仅通知服务与运行期语义）、`crates/jftrade-trading`（交易前风控、账户投影）、`crates/jftrade-integration-pine`（执行端口会话契约与错误映射）、`crates/jftrade-store-sqlite`（策略审计与当日订单计数）。

队列说明：本分片做行为核对。核对中发现分片二十把 `runtimecontrol/policy_test.go:83` 判成 `[x]` 过宽——参考实现的 `MarketDayStartUTC` 先用日历的“交易日边界起点”（含扩展时段时美股取上一本地日 20:00 的夜盘延续边界），只有在日历给不出边界时才回落到市场本地午夜；Rust 只实现了本地午夜分支。本分片把该行改回 partial 并把差异写清，同时把仍然成立的 `order_risk_business_test.go:235`（当日计数按实例 + 市场日窗口）升级为 `[x]`。

新增证据（`crates/jftrade-engine/src/strategy_runtime_execution_tests.rs`）：

1. `submitted_order_count_keeps_instance_scope_within_the_market_day`：同一实例写入两条跨市场日界的订单审计（2025-12-31T06:00Z 与 2025-12-31T17:00Z），在固定时刻 2026-01-01T02:00Z 上断言 US 市场日窗口计 2、HK 市场日窗口计 1、其它实例计 0；带 `// Parity: go:452dea11:internal/strategy/liveruntime/order_risk_business_test.go:235` 锚点。该用例把分片二十的时区修复与“实例范围 + 市场日窗口”两件事钉在一起。
2. `strategy_market_day_start_follows_dst_transition` 的注释补充说明参考实现在扩展时段使用交易日边界起点（2026-06-15T00:00Z），Rust 当前为本地午夜（2026-06-14T04:00Z），避免把“已对齐”写在代码里。

映射终值（31 行）：`[x]` 1 行（`order_risk_business_test.go:235`）、partial 29 行、boundary 1 行（`runtime_boundaries_test.go:21`，Go 反射式结构约束无 Rust 同形对象）。分片二十的 `policy_test.go:83` 由 `[x]` 纠正为 partial，净计数不变。

- **升级为 `[x]`（1 行）**：`order_risk_business_test.go:235`（实例范围 + 市场日窗口的当日订单计数）。
- **纠正为 partial（1 行）**：`runtimecontrol/policy_test.go:83`——本地午夜分支等价，扩展时段夜盘边界不等价（见缺口 1）。
- **保持 boundary（1 行）**：`runtime_boundaries_test.go:21` 反射式依赖所有权检查（Rust 由显式端口与 `check:rust:architecture` 门禁保证）。
- **保持 partial（28 行）**：管理边界族（`:16`/`:54`/`:94`/`:116`/`:155`/`:182`/`:216`/`:241`/`:275`/`:302`/`:356`）、撤单跟踪（`order_risk_business_test.go:135`）、市场日（`:216`）、pine 实盘会话族（6 行）、产品生命周期族（4 行）、运行时边界族（`:48`/`:116`/`:180`/`:258`/`:277`）。

剩余缺口（本批新增/细化）：

1. **扩展时段交易日边界缺失（P1，交易日/限额边界）**：参考实现的美股日界在扩展时段取上一本地日 20:00 夜盘延续起点（`pkg/market` 的 `TradingDayBoundaryStart`），Rust 只按市场本地午夜切分（`runtimecontrol/policy_test.go:83`、`order_risk_business_test.go:216`）。修复位置：`crates/jftrade-calendar` 暴露“交易日边界起点（含扩展时段）”API（`manager_session` 已建模美股 20:00 延续），engine 的 `strategy_market_day_start_ms` 改调该 API；回归按 2026-06-14 夜盘断言（参考期望 2026-06-15T00:00Z）。
2. **启动前置校验缺逐项回归（P1）**：流式行情能力（`manager_boundaries_test.go:94`）、活动提供商健康与显式覆盖跳过探测（`:116`）、live 券商绑定逐字段校验（`:182`）、符号运行时构建前置条件（`:302`）、启动预留重复与释放（`runtime_boundaries_test.go:48`）。
3. **撤单跟踪一致性缺回归（P1，交易安全）**：参考实现只对已跟踪订单撤单、成功才移除跟踪、网关失败保留跟踪（`order_risk_business_test.go:135`）；Rust 以执行存储为准，缺失败保留与未跟踪忽略的断言。
4. **行情桶边界缺回归（P1）**：同桶合并、跨桶收盘回调一次、非正成交量不开桶（`runtime_boundaries_test.go:116`）。
5. **依赖缺失可诊断性缺回归（P2）**：四类输入依赖失败各自点名（`manager_boundaries_test.go:241`）、构建前置条件逐项（`:302`）。
6. **账户/文案/格式化边界缺回归（P2）**：成交转 K 线价量、账户币种回落、空白币种容忍、显示名优先级、券商 ID 归一、零价格显示（`product_lifecycle_business_test.go:18`/`:151`、`runtime_boundaries_test.go:180`）。
7. **旋转/刷新失败上报缺回归（P2）**：同步已收盘 K 线时刷新失败要作为运行错误上报（`runtime_boundaries_test.go:277`）、网关失败事件与摘要排序（`product_lifecycle_business_test.go:96`）。
8. **仅通知与配置类边界（P2）**：维护忙碌原因与轮询间隔配置（`manager_boundaries_test.go:16`）、仅通知账户解析零调用（`:216`）、空白回调消息忽略与忽略单事件（`:356`）、sizing 与权益/价格参数边界（`pineworker_live_business_test.go:305`/`:368`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked submitted_order_count_keeps_instance_scope` | 1/1 通过（US 2 / HK 1 / 他实例 0） |
| 映射写入 | 31 行 payload `/tmp/s128s21_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1592 → 1592（+1 升级 −1 纠正）、partial 2222 → 2221、boundary 637 → 638（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3290 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1633、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1909/1909 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0（前端 10 文件 98 用例等全绿） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：分片二十二进入 `internal/strategy/catalog` 余量 22 行（`activity_degraded_test.go` 1、`catalog_boundary_behavior_test.go` 5、`instance_lifecycle_business_test.go` 4、`plugin_normalization_business_test.go` 5、`repository_failure_business_test.go` 3、`runtime_reconciliation_business_test.go` 4）与 `internal/strategy/definition` 4 行，随后 strategy_pine 域清空，进入 assistant_workflow 560 行。

### 分片二十二：`internal/strategy/catalog` 余量 22 行（strategy_pine 域收尾）+ 活动降级判定纠正

范围：`activity_degraded_test.go:65`、`catalog_boundary_behavior_test.go` 5 行（`:34`/`:58`/`:84`/`:123`/`:192`）、`instance_lifecycle_business_test.go` 4 行（`:11`/`:89`/`:143`/`:189`）、`plugin_normalization_business_test.go` 5 行（`:13`/`:89`/`:99`/`:157`/`:186`）、`repository_failure_business_test.go` 3 行（`:11`/`:19`/`:126`）、`runtime_reconciliation_business_test.go` 4 行（`:12`/`:52`/`:80`/`:113`）。

owner：`crates/jftrade-engine`（策略活动读取与分页、定义与插件路由、运行时端口与状态对账）、`crates/jftrade-strategy`（目录/实例/插件领域模型）、`crates/jftrade-store-sqlite`（策略定义与实例持久化、审计与日志）。

队列说明：本分片是 strategy_pine 域的收尾复核。核对中发现先前把 `activity_degraded_test.go:65` 判成 `[x]` 过宽：参考实现要求活动存储在 nil 或报错时降级为“已知空页”（200、条目 0、total 0、hasMore false），而 Rust 的端口层在存储失败时返回 `StrategyReadSnapshotError::Unavailable`，API 映射为 500 `STRATEGY_FAILED`（`product_api_strategies.rs` 的 `strategy_read_snapshot_failure`），即失败关闭；引用的 Rust 用例只锁定分页辅助函数对空集合的输出。本分片把该行与同语义的 `catalog_boundary_behavior_test.go:34` 一并改回 partial 并写清修复位置。

新增证据（本分片未新增测试）：无——本分片的工作是对 22 行逐条复核断言与现有证据的对应关系，纠正 1 行判定，并为其余 20 行写清缺口与修复位置（活动降级、定义同步状态、遗留快照归一、保存失败回滚、启动对账幂等、实例删除状态门等）。

映射终值（22 行）：`[x]` 1 行（`runtime_reconciliation_business_test.go:52`，复核后保持）、partial 21 行；其中 `activity_degraded_test.go:65` 由 `[x]` 纠正为 partial。

- **保持 `[x]`（1 行）**：`runtime_reconciliation_business_test.go:52`（运行失败只对 RUNNING 实例生效，双证据：端口失败收敛 + 运行时退出写审计）。
- **纠正为 partial（1 行）**：`activity_degraded_test.go:65`——Rust 是失败关闭（500 STRATEGY_FAILED），不是降级空页。
- **保持 partial（20 行）**：活动查询失败空页（`catalog_boundary_behavior_test.go:34`）、活动写失败不阻塞控制状态（`:58`）、定义同步状态（`:84`）、归一与克隆隔离（`:123`/`:192`）、实例生命周期（`instance_lifecycle_business_test.go` 4 行）、插件归一（`plugin_normalization_business_test.go` 5 行）、仓库失败（`repository_failure_business_test.go` 3 行）、运行时转换与启动对账（`runtime_reconciliation_business_test.go:12`/`:80`）、活动分页与观测富化（`:113`）。

剩余缺口（本批新增/细化，按优先级）：

1. **活动查询降级语义缺失（P1）**：活动存储不可用时参考实现返回已知空页（200/空/total 0），Rust 返回 500 `STRATEGY_FAILED`；修复位置 `crates/jftrade-engine/src/strategy_runtime_port.rs` 的 `logs`/`audit` 读取路径（保留告警），回归按 `activity_degraded_test.go:65` 与 `catalog_boundary_behavior_test.go:34`。
2. **保存失败回滚缺故障注入（P1，回滚语义）**：参考实现逐操作断言保存失败后持久快照不变、内存态不提前生效（`repository_failure_business_test.go:19`）；Rust 依赖 SQLite 事务原子性，缺注入式回归。
3. **定义同步状态缺失（P1，产品语义）**：`definitionSync`（IsLatest/CanApplyLatest/BlockedReason）在 Rust 无同形对象（`catalog_boundary_behavior_test.go:84`）。
4. **实例删除状态门缺回归（P1，状态边界）**：删除需先停止（`instance_lifecycle_business_test.go:11`）。
5. **启动对账幂等与计数缺回归（P1，恢复语义）**：RUNNING 也要重置、changed 计数、第二次调用不落盘（`runtime_reconciliation_business_test.go:80`）。
6. **遗留快照归一缺回归（P1，迁移/兼容）**：旧版快照补默认字段并丢弃运行期字段（`plugin_normalization_business_test.go:99`）。
7. **P2 级**：活动写失败容忍（`:58`）、归一/克隆调用方隔离（`:123`/`:192`）、实例操作错误分类（`instance_lifecycle_business_test.go:89`）、定义刷新与关联分类（`:143`/`:189`）、插件生命周期排序与未找到分类（`plugin_normalization_business_test.go:13`/`:89`/`:157`/`:186`）、仓库加载失败错误身份（`repository_failure_business_test.go:11`/`:126`）、转换计数（`runtime_reconciliation_business_test.go:12`）、活动与观测富化合并（`:113`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | 22 行 payload `/tmp/s128s22_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1592 → 1591、partial 2221 → 2222、boundary 638 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；partial 引用不可解析 2；Rust 测试 3290 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1633、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture`、`git diff --check` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1909/1909 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

**strategy_pine 域状态**：`internal/strategy`（catalog、instancebinding、instanceview、live_command_business_boundaries、liveruntime、pine_live_command、pine_live_executor、pineruntime、runtimecontrol、service、types、errors）与 `pkg/strategy`（definition、indicatorbinding、indicatorwarmup、ir、pine、pineengine、pinespec、pineworker）共 538 行全部持有终值并带结论（`[x]` 81、partial 383、boundary 74；module_only 0、missing 0、无缺 command 行）。其中 431 行的结论/证据在第 128 批被改写；本批 Rust 测试总数由 3204 增至 3290（+86），并落地 11 处生产修复（提交 014e66d6、f865e10a、d9155270、433405e3、4a8a92aa、3c0e10b1、e0313575、661dd659、61234727、cebff772、962d3a39、cbc7a4e5 中标注 fix 的部分）；余下 107 行的终值来自更早批次，本批对应分片（分片一、五、六、八~十七）已逐条复核并记为“保持终值”（见各分片小节，例如分片十七列出的 32 行保持项）。

后续：下一批转入 `assistant_workflow` 域 560 行（`internal/assistant/*`、`internal/api/assistant/*`、ADK 审批与工作流租约、会话/工件/审批恢复等），仍按每分片一次提交、逐条终值、缺口登记与门禁全跑的标准推进。

## 第 129 批：`assistant_workflow` 域

范围（本批 892 行，每分片一次提交）：`internal/api/assistant` 82 行、`internal/assistant/model` 10 行、`internal/assistant`（顶层 service/workflow 用例）32 行、`internal/assistant/workflow` 38 行、`internal/assistant/assembly` 104 行、`internal/assistant/engine` 626 行，另含 `internal/app/apiserver` 下 8 行 assistant 相关用例。

owner：`crates/jftrade-engine`（ADK 读/mutation/chat-stream 端口、生产装配、审批与会话写入所有权）、`crates/jftrade-store-sqlite`（ADK 会话/工件/审批持久化）、`crates/jftrade-api`（transport 契约与 legacy 路由 404）、`apps/desktop/src-tauri`（桌面侧 ADK 命令投影，如涉及）。

队列说明：本批承接 strategy_pine 域完结后的域序，先清 `internal/api/assistant`（82 行，两片），再依次进入 model → 顶层 → workflow → assembly → engine。每分片按文件与行号升序推进，逐行给出终值（`[x]`/partial/boundary），缺锚点的 `[x]` 行在本批补齐锚点，发现判定过宽的行改回 partial 并写明缺口、修复位置与回归要求。

### 分片一：`internal/api/assistant` 首批 30 行（10 处锚点补齐 + 1 条专属回归测试）

范围（按文件与行号升序）：`adk_approval_test.go:16/:183/:282/:335/:382/:450` 6 行、`adk_integration_test.go:20` 1 行、`adk_normalize_test.go:15` 1 行、`adk_ops_test.go:18/:216/:247/:286/:394/:448/:468` 7 行、`adk_routes_test.go:26/:165/:214/:245/:359/:420/:481/:552/:578/:597/:634/:707/:741/:787/:858` 15 行。

owner：`crates/jftrade-engine`（`product_production_ports_adk_*`、`product_adk_read_tests`、`product_adk_mutation_*`、`product_adk_chat_stream_*`、`product_adk_model_runtime*`、`product_production_assembly_tests`）、`crates/jftrade-api`（`tests/transport_contracts.rs` 的 legacy assistant 路由 404）。

队列说明：本分片是 assistant_workflow 域的开篇。核对方式是先按 `git show 452dea11:<path>` 读出 30 行参考测试的函数签名与断言，再对照 Rust 侧证据；同时用工具校验四条不变式：参考行号确实落在同名 `func Test...(t *testing.T)` 上、锚点所在文件必须出现在该行 `rust_entry` 内、锚点携带的参考测试名必须与该行一致、boundary 行不得挂锚点（本分片 30 行 0 违规）。

新增证据：

1. `crates/jftrade-engine/tests/adk_mutations_compatibility.rs::adk_provider_save_rejects_the_truncated_payload_with_the_reference_message`（对应 `adk_routes_test.go:578 TestADKProviderSaveRejectsInvalidPayload`）。此前该行只引用 21 路由畸形体矩阵（仅断言状态与错误码），provider 专属文案没有 wire 断言；新用例断言截断体 `{"displayName":` 在 `POST /api/v1/adk/providers` 上返回 400、`BAD_REQUEST` 与 `invalid provider payload` 三件套，并锁定该文案来自 `body_error_message` 的 `CreateProvider` 分支而非通用 mutation 文案。
2. 10 处缺失锚点补齐：`product_production_assembly_tests.rs` 3 处（任务/记忆/工作流触发路由、暂停与恢复原子持久化、指标聚合）、`product_adk_read_tests.rs` 1 处（审计分页拒绝非正 limit/负 offset）、`product_adk_model_runtime_tool_failure_tests.rs` 3 处（chat 回放工具失败信封、stream 只出终帧 final、从持久化终态重建 final）、`tests/adk_chat_stream_compatibility.rs` 1 处（SSE 畸形 chat 体返回 error 帧）、`tests/adk_mutations_compatibility.rs` 1 处（provider 截断体文案）、`product_adk_mutation_product_tests.rs` 1 处（优化任务查询与取消持久化）。
3. 14 处既有锚点补全参考测试名（13 处原先只写 `file:line`，另有 1 处以散文形式引用，一并改为规范锚点），使锚点可从代码侧自证到参考测试名，而不只依赖行号。
4. 3 处 `rust_entry` 修正：`product_adk_model_runtime.rs::tool_failure_tests::X` 改为 `product_adk_model_runtime_tool_failure_tests.rs::X`。三个用例实际由 `#[path]` 引入的模块文件承载，条目与锚点必须指向同一文件，否则锚点侧会判为 stale。

映射终值（30 行）：`[x]` 29 行、boundary 1 行（`adk_integration_test.go:20 TestRealADKChatStreamWithSavedProvider`，真实模型 Provider live 调用，仅在显式 live workflow 中验证）、partial 0 行。本分片未把任何 `[x]` 行收紧为 partial：29 行的断言与参考测试逐一对应（回放信封、暂停/恢复状态迁移、负路径错误码、分页拒绝、畸形体文案、指标聚合字段）。

剩余缺口（本分片未新登记 P0/P1；P2 仅记录口径）：

1. **live Provider 集成（P2，边界保留）**：`adk_integration_test.go:20` 依赖真实模型 Provider，Rust 侧由显式 live workflow 覆盖，普通测试保持不联网；升级路径是在 live workflow 中新增端到端 chat stream 冒烟。
2. **同类文案口径（P2）**：本分片为 provider 文案补了专属断言；其它写路由（task/memory/session/workflow/agent/skill）的专属文案仍只由路由级矩阵的状态码与错误码覆盖，后续在各自分片按同一方式补 wire 文案断言（`body_error_message` 已给出唯一映射，`crates/jftrade-engine/src/product_adk_mutation_port.rs`）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --test adk_mutations_compatibility --locked` | 9/9 通过（含新用例） |
| 映射写入 | 4 行 payload `/tmp/b129s1_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1591 不变、partial 2222 不变、boundary 638 不变（合计 4451） |
| 逐行复核 | 自建校验（参考行号签名、锚点文件与 `rust_entry` 一致性、锚点名一致性、boundary 无锚点） | 30 行 0 问题 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 208 → 198；Rust 测试 3290 → 3291 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1633 → 1643、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1909/1910（`api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 抖动）→ 隔离复跑 2/2 通过 → 整轮复跑 1910/1910 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 OpenAPI 操作、18 路由组、19 探针；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：分片二进入 `internal/api/assistant` 余 52 行（`adk_routes_test.go:911` 起、`adk_sessions_test.go`、`adk_transport_contracts_test.go`、`chat_helpers_test.go`、`chat_stream_lifecycle_test.go`、`routes_resource_contracts_test.go`、`routes_test.go`、`workflow_routes_test.go` 等），其中 9 行尚缺锚点（`adk_sessions_test.go:15`、`adk_transport_contracts_test.go:10`、`chat_helpers_test.go:167/:216/:260`、`chat_stream_lifecycle_test.go:9`、`routes_resource_contracts_test.go:159`、`routes_test.go:136`、`workflow_routes_test.go:14`），随后按队列进入 `internal/assistant/model` 10 行。

### 分片二：`internal/api/assistant` 余 52 行（补 6 处锚点 + 3 行判定收紧）

范围（按文件与行号升序）：`adk_routes_test.go:911`；`adk_sessions_test.go:15`；`adk_transport_contracts_test.go:10`；`adk_workflow_routes_test.go:15/:262`；`catalog_failure_contracts_test.go:17`；`chat_helpers_test.go:13/:49/:59/:87/:151/:167/:216/:260`；`chat_stream_lifecycle_test.go:9`；`chat_stream_recovery_contracts_test.go:12/:41`；`chat_transport_disconnect_test.go:55/:110`；`input_response_test.go:12`；`query_encoding_contracts_test.go:13`；`routes_boundary_contracts_test.go:15/:92/:178/:278/:329/:369`；`routes_error_contracts_test.go:14/:98`；`routes_identifier_validation_test.go:12`；`routes_payload_pagination_test.go:12/:27/:65/:80/:134`；`routes_resource_contracts_test.go:15/:115/:159/:261/:295/:410`；`routes_test.go:29/:92/:101/:136/:180/:265/:301/:348/:366`；`workflow_routes_test.go:14/:185`。

owner：`crates/jftrade-engine`（ADK 读/写与 chat-stream 端口、生产装配、会话与工件端口、supervisor 生命周期）、`crates/jftrade-api`（Bearer 认证与 SSE 写失败传播）、`crates/jftrade-store-sqlite`（ADK 会话/工件表）。

队列说明：本分片清空 `internal/api/assistant`（82 行）。核对方式与分片一一致：按 `git show 452dea11:<path>` 逐行读参考测试断言，再对照 Rust 证据与四条不变式（参考行号落在同名测试函数上、锚点文件出现在 rust_entry 内、锚点名与参考名一致、boundary 行不挂锚点）；52 行 0 违规。

新增证据：

1. 6 处缺失锚点补齐：`product_production_assembly_tests.rs`（公开会话 CRUD 与过滤路由）、`product_adk_read_tests.rs`（流快照保留事件 id 与 payload）、`tests/adk_chat_stream_compatibility.rs`（无效请求的终局 error 帧）、`product_adk_model_runtime_fencing_tests.rs`（supervisor 关停等待全部任务）、`product_adk_model_runtime_recovery_tests.rs`（恢复终帧带 replay 标记）、`tests/adk_workflow_canvas_contracts.rs` 2 处（webhook secret 生命周期与多节点 canvas 运行）。
2. 32 处既有锚点补全参考测试名（`product_production_ports_adk_tests.rs` 22、`tests/adk_workflow_canvas_contracts.rs` 2、`product_adk_model_runtime_recovery_tests.rs` 2、`product_adk_read_tests.rs` 2、`tests/adk_mutations_compatibility.rs` 2、`product_adk_mutation_product_tests.rs` 1、`product_production_assembly_tests.rs` 1），使锚点可从代码侧自证到参考测试名。

判定收紧（3 行 `[x]` → partial，记录原因）：

1. `chat_helpers_test.go:167`（原引用「重启后重放保留终帧」）：该证据只能证明终帧保留/重放，不能证明参考用例的 hub delta 分类与 publishFinal 裁剪；Rust 终帧直接回放持久化 response，未发现 toolCalls 输出裁剪证据，故收紧为 partial。
2. `chat_helpers_test.go:260`：校验错误白名单已覆盖，但参考的 bearerToken 边界表无断言，且 Rust 只接受精确 Bearer 前缀（参考大小写不敏感），故收紧为 partial。
3. `routes_resource_contracts_test.go:159`：agent 校验文案与 provider 删除语义已覆盖，但 provider `/test` 的 mode 语义无 wire 断言，且未知 provider 的状态码与参考不一致，故收紧为 partial。

另：`chat_helpers_test.go:216` 保持 `[x]`，但引用改写为 wire fixture 用例（stream-invalid-json → 终局 error 帧）+ 端口 isolation 用例，并把 `sessionSent`/missing-agent 预览半明确归入 hub-helper 边界族（与 `:13/:49/:59/:87/:151` 同族）。`chat_transport_disconnect_test.go:110` 的 partial 行原有锚点点明其已覆盖半（断线后保留终态与重放过滤），本分片保留该安排并在工具校验中放行。

映射终值（52 行）：`[x]` 42、partial 4（本分片新增 3 + 既有 `chat_transport_disconnect_test.go:110`）、boundary 6（`chat_helpers_test.go:13/:49/:59/:87/:151`、`chat_stream_recovery_contracts_test.go:12`）。

**`internal/api/assistant` 域状态**：82 行全部持有终值（`[x]` 71、partial 4、boundary 7；module_only 0、missing 0、缺 command 0），其中 30 行在分片一复核、52 行在本分片复核；本批为该目录新增 16 处锚点与 1 条专属回归测试。

剩余缺口（本分片新增/细化，按优先级）：

1. **provider `/test` 状态码与 mode 语义（P1，公开路由）**：参考对未知 provider 返回 502 且消息为 provider not found，Rust 的 `test_provider` 返回 404 `ADK_PROVIDER_NOT_FOUND`；复现：`POST /api/v1/adk/providers/provider-missing/test`（无 body 与 `{"mode":"full"}`、`{"mode":"slow"}` 三种输入）。期望：quick 默认 + `requestField: reasoning.effort` 回显、full 回显、slow → 400、未知 provider → 502。修复位置：`crates/jftrade-engine/src/product_production_ports_adk_mutation_runtime.rs::test_provider`。回归要求：先按参考对齐状态码并更新 `product_adk_store_parity_tests.rs::snapshot_and_provider_test_boundaries_fail_closed` 的 port 断言，再补路由级 wire 用例。
2. **Bearer 前缀大小写（P2，认证边界）**：参考 `bearerToken` 大小写不敏感且容忍多余空白，Rust `request_bearer_token` 只接受精确 `Bearer `。修复位置：`crates/jftrade-api/src/auth.rs::request_bearer_token`。回归要求：表驱动边界用例（空串、`Basic token`、`bearer secret`、`BEARER secret`、多空白、仅前缀）。
3. **终帧 tool 输出裁剪（P2，wire 形状）**：参考 Hub 的 publishFinal 会清空 `toolCalls[].output`，Rust 无对应断言与证据。修复位置：`crates/jftrade-engine/src/product_adk_model_runtime_stream.rs`（终帧投影）。回归要求：断言 final 帧 `response.run.toolCalls[].output` 为空，或明确记录不裁剪的产品决策。
4. **hub helper 边界族（P2，边界保留）**：delta 分类、`sessionSent`/missing-agent 预览、2000 事件上限与 30 分钟 TTL、timeline clone 兜底在 Rust 无同形对象，维持 boundary 并保留升级路径（改为 durable stream event 断言）。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | 4 行 payload `/tmp/b129s2_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1591 → 1588、partial 2222 → 2225、boundary 638 不变（合计 4451） |
| 逐行复核 | 自建校验（参考行号签名、锚点文件与 rust_entry 一致性、锚点名一致性、boundary 无锚点） | 52 行 0 问题 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 198 → 189；Rust 测试 3291 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1643 → 1649、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1909/1910（`adk_session_detail_omits_resolved_approval_groups` 抖动，隔离复跑 3/3 通过）→ 次轮 1909/1910（`api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 抖动，隔离复跑 2/2 通过）→ 三轮 1910/1910 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 OpenAPI 操作、18 路由组、19 探针；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：`internal/api/assistant` 已清空，队列进入 `internal/assistant/model` 10 行，随后 `internal/assistant` 顶层 32 行、`internal/assistant/workflow` 38 行、`internal/assistant/assembly` 104 行、`internal/assistant/engine` 626 行与 `internal/app/apiserver` 下 8 行 assistant 用例；本分片登记的 provider `/test` 状态码缺口（P1）列为下一批优先修复项。

### 分片三：`internal/assistant/model` 10 行（复核后升级/细化 + 2 处断言补强）

范围（按文件与行号升序）：`provider_reasoning_config_test.go:8/:27/:65`、`timeline_helper_test.go:5`、`timeline_reply_ordering_test.go:11`、`workflow_graph_resume_identity_test.go:5`、`workflow_plan_test.go:8/:31/:100`、`workflow_task_tools_test.go:8`。

owner：`crates/jftrade-assistant`（模型层的任务图、计划与审批作用域）、`crates/jftrade-engine`（provider 写入与 test 投影、运行/会话读取、消息时间线合并、chat 覆盖项校验）。

队列说明：本分片是 `internal/assistant/model`（模型层）10 行。这 10 行此前全部为 partial/boundary，因此本分片的目标不是补锚点，而是逐条比对参考断言与 Rust 实现，把已有等价证据的部分写实、把仍缺失的部分写清，并把判定过宽/过窄的行纠正（`provider_reasoning_config_test.go:65` 由 boundary 改为 partial）。

新增证据：

1. `crates/jftrade-assistant/src/workflow.rs::graph_orders_equal_rank_tasks_by_id_and_reports_graph_faults`（新用例）：锁定等 order 任务按 id 升序（对应参考 `SortWorkflowTasks` 的确定性），并把三种图故障区分开——真实环 `WorkflowError::Cycle`、未知依赖 `WorkflowError::MissingDependency`（对应参考“未知依赖不算环”）、自环 `WorkflowError::SelfDependency`。
2. `crates/jftrade-engine/src/product_adk_model_runtime_gate_tests.rs::chat_rejects_invalid_permission_work_mode_and_reasoning_overrides`（补断言）：把哨兵档位 `default` 加入拒绝表，锁定参考 `ValidateOptionalReasoningEffort` 对 `default` 的拒绝语义（此前只用 `turbo` 代表非法值）。

映射终值（10 行）：partial 10、boundary 0、`[x]` 0。未升级为 `[x]` 的原因是每条参考用例都同时包含 Rust 尚未具备的对象或助手（推理配置解析层、时间线助手族、图指纹、展示文案、run 作用域过滤、目标决策工具层），已有证据只覆盖其中一部分。

关键事实与缺口（本分片细化，按优先级）：

1. **目标决策工具层缺失（P1，沿用既有登记）**：`workflow_task_tools_test.go:8` 的 `WorkflowGoalDecision` 相位/快照、目标提示词、planner 参数强制在 Rust 无同形对象（提示词与快照属 workflowexec 未迁移部分）。修复位置：`crates/jftrade-assistant` 新增目标决策模型与 planner 参数助手，引擎工具调用路径接入。
2. **推理配置解析层缺失（P2）**：参考的 `DefaultProviderReasoningConfig` / `NormalizeProviderReasoningConfig` / `ValidateProviderReasoningConfig` / `ResolveProviderReasoning` 在 Rust 没有对应实现——provider 写入只透传 `reasoningConfig`，provider test 投影直接回显 requestField/mappings 并把每个映射标为 ok。影响 `provider_reasoning_config_test.go:8/:27/:65` 三行。修复位置：`crates/jftrade-assistant` 提供归一/校验/解析，`crates/jftrade-engine` 的 provider 写入与 test 投影调用。
3. **时间线助手族与排序规则（P2）**：Rust 的 `merge_session_timeline` 按 createdAt→id 排序（无 order 次级键、无无效时间戳兜底断言），且没有前缀剥离、首个非空、首个工具时间、首个审批时间助手；最终回复与工具活动的相对顺序也没有断言。影响 `timeline_helper_test.go:5` 与 `timeline_reply_ordering_test.go:11`。修复位置：`crates/jftrade-engine/src/product_production_ports_adk_notices.rs`、`product_production_ports_adk_read.rs` 与 run 投影。
4. **工作流图指纹（P2，恢复语义）**：参考用归一化指纹判定恢复身份与执行漂移；Rust 以 trigger log 的 nodeRuns 与在途请求身份承接，缺“集合顺序无关”“内容漂移敏感”两条断言（`workflow_graph_resume_identity_test.go:5`）。修复位置：`crates/jftrade-assistant/src/workflow_canvas.rs` 新增身份哈希并在恢复路径调用。
5. **计划呈现与审批作用域（P2）**：步骤描述、待审批回复、摘要空白过滤、`UpdateWorkflowPlanForChildAt` 匹配规则、`ApprovalsForRun` 的 run 作用域与空白边界在 Rust 无断言（审批读取路由只支持 status/agentId 过滤）。影响 `workflow_plan_test.go:8/:31/:100`。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增/补强用例 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-assistant --all-targets --locked`、`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --lib --locked -E 'test(chat_rejects_invalid_permission_work_mode_and_reasoning_overrides)'` | jftrade-assistant 41/41 通过（含新用例）；engine 目标用例 1/1 通过 |
| 映射写入 | 10 行 payload `/tmp/b129s3_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1588 不变、partial 2225 → 2226、boundary 638 → 637（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 189 不变；Rust 测试 3291 → 3292 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1649、unrecorded 0、stale 0、unknown 53 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1910/1910 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 OpenAPI 操作、18 路由组、19 探针；assistant-runtime 9 状态 12 迁移；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` exit 0（nextest 1981/1981、node 48 pass） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant`（顶层 service/workflow 用例）32 行，随后 `internal/assistant/workflow` 38 行、`internal/assistant/assembly` 104 行、`internal/assistant/engine` 626 行与 `internal/app/apiserver` 下 8 行 assistant 用例。

### 分片四：`internal/assistant/service_*` 32 行（复核后细化 + 1 处锚点行号纠正）

范围（按文件与行号升序）：`service_audit_pagination_test.go:9`、`service_builtin_agent_edit_test.go:10`、`service_business_helpers_test.go:22/:105/:145/:189`、`service_business_test.go:12/:117/:169/:230/:433`、`service_contract_boundaries_test.go:12/:133/:224/:312`、`service_lifecycle_boundaries_test.go:10/:25/:71/:95`、`service_persistence_runtime_boundaries_test.go:12/:119/:164/:180/:228`、`service_recovery_test.go:10`、`service_skill_state_recovery_test.go:19/:61/:119`、`service_test.go:9/:24/:38/:49`。

owner：service 层是领域行为的组装面，状态写入 owner 在领域 crate——审批、会话与运行状态由 `crates/jftrade-engine` 的 ADK 端口与存储承接，任务图与画布模型由 `crates/jftrade-assistant` 承接，不在 API handler 复制业务逻辑。

复核方法：逐条用参考提交的 Go 源码核对测试定义行（32/32 行号签名一致），再核对账本每行 rust_entry 首段用例在 Rust 侧存在（全部可解析），并核对 `[x]` 行锚点文件出现在该行 rust_entry 内。结论：除下述两处细化外，其余 30 行既有 partial 结论准确，无需改动。

修正与细化：

1. 锚点行号笔误纠正：`service_business_test.go:12 TestServiceSaveAgentValidationScenarios` 的更新路径锚点误写为 `:29`，实际定义在 `:12`，已改为 `:12`。该野锚点是此前缺锚点告警中的一条，纠正后 reconcile 的 unknown 由 53 降为 52。
2. 同一行结论改写为双测试组合：创建路径 7 个失败场景与 disabled/enabled 保存由 `adk_agent_write_reports_the_go_validation_messages` 断言，更新路径对合并后 payload 复用同一校验 owner 由 `adk_agent_update_revalidates_the_merged_payload` 断言，两处各挂一处同行锚点，保持 `[x]`。
3. `service_test.go:9` 结论细化并新登记一个 P2 wire 差异：参考要求错误文本为带前缀的时间线失败文案且双向 errors.Is 成立；Rust 侧时间线读取失败走 500 `ADK_MESSAGES_GET_FAILED`，message 直接透传底层存储错误文本，没有前缀包装。修复位置与回归要求见下。

映射终值（32 行）：`[x]` 1、partial 31、boundary 0。

新登记缺口（P2，wire 文案）：时间线失败错误文本前缀包装（`service_test.go:9`）。复现：会话时间线存储失败时读取时间线。期望：message 含底层 cause 文本并以前缀标明时间线失败，或明确记录不加前缀的产品决策。修复位置：`crates/jftrade-engine/src/product_production_ports_adk_read.rs` 的时间线失败映射。回归要求：补断言 message 含底层 cause 文本。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | 2 行 payload 经账本写入脚本应用 | `[x]` 1588 不变、partial 2226 不变、boundary 637 不变（合计 4451） |
| 逐行复核 | 自建校验（参考行号签名 32/32、rust_entry 可解析 32/32、锚点文件与名一致性） | 32 行 0 问题（含 1 处行号笔误纠正） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 189 → 188；Rust 测试 3292 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1649、unrecorded 0、stale 0、unknown 52 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1910/1910 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 OpenAPI 操作、18 路由组、19 探针；assistant-runtime 9 状态 12 迁移；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 全命令跑通（顺序失败即停，末条 pineworker 98/98，nextest 1940/1940、node 98 pass） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant/workflow*` 顶层 33 行，随后 workflow 目录 5、assembly 104、engine 626 与 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片五：`internal/assistant/workflow*` 顶层 33 行（3 行升级 + 4 处缺口纠正）

范围（按文件与行号升序）：`workflow_async_tools_test.go:12/:49/:86`、`workflow_crud_test.go:14/:116/:228/:314/:400/:455/:497/:555/:616/:671`、`workflow_lifecycle_test.go:13/:58/:98`、`workflow_store_failures_test.go:13`、`workflows_extended_test.go:14/:135/:208/:278/:528/:631`、`workflows_resource_recovery_test.go:11/:52/:105`、`workflows_test.go:11/:39/:88/:157/:198/:248/:276`。此前 33 行全部为 partial。

owner：工作流运行、调度与后台任务的写入 owner 在领域 crate——运行入口与失败持久化由 `crates/jftrade-engine` 的 ADK 端口承接，cron/阈值/画布编译由 `crates/jftrade-engine` 的工作流助手与 `crates/jftrade-assistant` 的画布模型承接，队列原子性由 `crates/jftrade-store-sqlite` 承接。

复核方法：逐条用参考提交的 Go 源码核对测试定义行（33/33 行号签名一致），再核对账本每行 rust_entry 首段用例在 Rust 侧存在（全部可解析）。对可能等价的行打开 Rust 实现逐断言比对，缺口补断言后升级；对结论过宽的行按实现纠正。

新增与补强证据：

1. `crates/jftrade-engine/src/product_workflow_cron.rs::next_schedule_run_skips_weekend_for_weekday_cron`（新用例）：锁定工作日 1-5 范围同日 08:00 与周五过点后跳到周一，与参考 `workflows_test.go:11` 相同取值。
2. `product_workflow_threshold.rs::evaluate_cross_up_transitions`（补断言）：匹配负载的 `instrumentId` 与 `edge` 字段，与参考 `workflows_test.go:39` 的负载断言对齐。
3. `product_workflow_threshold.rs::evaluate_above_and_below_levels`（补断言）：显式大于运算符的首匹配，与参考的 above 分支对齐。
4. `product_production_assembly_tests.rs::production_adk_local_mutations_persist_tasks_memory_and_workflow_triggers`（补断言）：显式重置发放与旧值不同的新密钥，与参考 `workflows_test.go:88` 的重置断言对齐。

映射终值（33 行）：`[x]` 3（`workflows_test.go:11/:39/:88`）、partial 30、boundary 0。

关键事实与缺口（本分片新登记与纠正，按优先级）：

1. **无画布图运行入口兜底与参考相反（P1，运行行为）**：参考 `workflows_test.go:248` 要求缺图运行失败且无回退；Rust 运行入口在 canvasGraph 缺失时以 `WorkflowCanvasGraph::single_agent()` 兜底继续运行。修复位置：`product_production_ports_adk_mutation_workflow_runtime.rs` 的缺图分支。回归要求：补缺图必须失败且无回退响应的运行入口用例，或明确记录保留兜底的产品决策后再关闭。
2. **分页上收敛与日志默认条数、标签归一化（P1，wire 契约）**：参考 `workflow_crud_test.go:14` 要求 limit 200 收敛 100、日志默认 20、更新标签去空格去重去空；Rust 分页助手只在 limit 非正时回默认值、不做上收敛，日志默认 100，更新入口无标签归一化。修复位置：`product_production_ports_adk_projection.rs::page` 与工作流更新入口。回归要求：补分页边界用例与标签归一化用例。
3. **缺失 agent 的 not found 文案（P2，错误消息）**：参考 `workflow_crud_test.go:228` 对缺失 agent 要求 agent not found；Rust 缺失或禁用 agent 统一报 enabled agent is required。修复位置：`product_production_ports_adk_mutation.rs::validate_session_agent` 的缺失分支。回归要求：补缺失 agent 报 not found 的路由级用例，或明确记录统一文案的产品决策。
4. **非法 work mode 被静默归一（P2，写入校验）**：参考 `workflows_resource_recovery_test.go:11` 要求非法模式被拒；Rust 的 `normalize_workflow_mode` 把非法值归一为 loop。修复位置：工作流写入入口的模式校验。回归要求：补非法模式被拒的写入用例，或明确记录归一策略的产品决策。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 新增/补强用例 | engine 目标 8 用例组合过滤 | 8/8 通过 |
| 映射写入 | 7 行 payload 经账本写入脚本应用（3 升级 + 4 纠正） | `[x]` 1588 → 1591、partial 2226 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 188 不变；Rust 测试 3292 → 3293 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1649 → 1652、unrecorded 0、stale 0、unknown 52 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1911/1911 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility exit 0（278 OpenAPI 操作、18 路由组、19 探针；assistant-runtime 9 状态 12 迁移；desktop 3 平台档 6 link case 10 facade 4 event）；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 首轮在 nextest 段 `adk_session_detail_omits_resolved_approval_groups` 抖动一次（已知抖动项，隔离复跑 3/3 通过）→ 次轮 nextest 1941/1941、node 98 pass 全过；中途 target-health 因 .rcgu.o 超 50000 阻断一次，按流程确认无 Cargo 进程后 clean artifacts 重跑通过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant/workflow` 目录 5 行，随后 assembly 104、engine 626 与 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片六：`internal/assistant/workflow` 目录 5 行（2 行升级 + 1 处结论纠正）

范围（按文件与行号升序）：`workflow/rules_test.go:12/:40/:89/:135/:207`。此前 5 行全部为 partial。

owner：规则层是领域助手的纯函数面（cron 计算、阈值评估、事件匹配、触发器校验），实现在 `crates/jftrade-engine` 的工作流助手模块与 `crates/jftrade-assistant` 的画布模型，无状态写入，所有者即实现模块本身。

复核方法：逐条用参考提交的 Go 源码核对测试定义行（5/5 行号签名一致），再核对账本引用可解析。发现规则层 `:12` 与 `:40` 和顶层参考同名同值（cron 缺了 Workflow 一词的命名差异除外，断言取值完全相同），直接复用分片五证据升级；`:89` 的引用用例与行为错位，按实现纠正。

复核发现（命名核对）：规则层 cron 测试名为 `TestNextScheduleRunUsesFiveFieldCronAndTimezone`，顶层为 `TestNextWorkflowScheduleRunUsesFiveFieldCronAndTimezone`（多 Workflow 一词），断言取值相同，锚点按各自真实名称书写。

新增证据：无新增测试用例，复用分片五的 1 条新用例与 3 处补强断言，另补 5 处同行锚点（cron 三测试、阈值两测试各挂规则层锚点；cron 计算测试同时补上此前漏挂的顶层锚点）。

映射终值（5 行）：`[x]` 2（`rules_test.go:12/:40`）、partial 3、boundary 0。

关键事实与缺口（本分片纠正，按优先级）：

1. **事件匹配与冷却三态、归一回退与默认标题无断言（P2，规则助手）**：参考 `rules_test.go:89` 要求事件规则字段匹配、事件冷却首/期/后三态（含 nil 拒绝）、Normalize 系列回退与中文默认标题；Rust 没有 EventMatches 助手与事件触发器冷却三态的直接断言，Normalize 回退与中文标题只有实现、无测试断言。修复位置：调度事件路径与触发器写入投影。回归要求：补事件匹配三态与标题回退用例。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 复用证据 | engine cron 三测试与阈值两测试组合过滤 | 5/5 通过 |
| 映射写入 | 3 行 payload 经账本写入脚本应用（2 升级 + 1 纠正） | `[x]` 1591 → 1593、partial 2226 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 188 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1652 → 1654、unrecorded 0、stale 0、unknown 52 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1911/1911 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility 全 replay 通过；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 全过（nextest 1941/1941、node 98 pass，本分片无抖动） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant/assembly` 104 行，随后 engine 626 与 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片七：`internal/assistant/assembly` 前 35 行（1 处锚点行号纠正 + 3 行补锚点）

范围（按文件与行号升序）：`adk_backtest_adapter_test.go:8`、`adk_capability_contracts_test.go:12/:94`、`adk_closure_contracts_test.go:12/:73`、`adk_product_catalog_test.go:13/:29/:77/:138`、`adk_runtime_contracts_test.go:13/:113/:156`、`adk_strategy_input_validation_test.go:10`、`adk_strategy_test.go:19/:32/:77/:194/:308/:397/:449/:605/:699/:774`、`adk_summary_contracts_test.go:9/:33`、`adk_tool_failure_contracts_test.go:19`、`application_adapter_boundaries_test.go:20/:71/:128`、`application_adapter_test.go:16/:123/:138/:164/:185/:226`（35 行中 [x]12/partial23）。

owner：组装层是领域工具与适配器的组合面，写入 owner 在领域 crate——策略/回测工具与执行器由 `crates/jftrade-engine` 的 MCP 生产执行器承接，Pine 校验由 `crates/jftrade-strategy` 承接，行情与自选由市场数据端口承接。

复核方法：35/35 参考行号签名一致；账本引用逐段解析（含 `+` 组合的各自文件前缀）全部指向真实测试；12 条 `[x]` 逐条核对锚点在位；4 条复杂 `[x]` 与 6 条 partial 抽查 Go 原文与结论对应关系。

修正（无 verdict 翻转，本分片 35 行终值不变）：

1. 锚点行号笔误纠正：`adk_strategy_test.go:605 TestADKStrategyOptimizePersistsTasksAndCancelsQueuedRunsOnFailure` 的两处锚点（成功半与失败半测试）误写为 `:636`，实际定义在 `:605`，已纠正。reconcile unknown 52 → 51。
2. 补 3 行 `[x]` 缺失锚点：`application_adapter_boundaries_test.go:71`（组装目录测试）、`application_adapter_test.go:164`（路由校验与 MCP 工具测试各一处）。缺锚点告警 188 → 185。
3. 其余 `[x]` 结论经抽查可信（订阅错误原样传播、定义版本不可变快照、优化入队与失败回滚均有探针证据）；partial 行结论与缺口记录准确（如视觉模型归一化、脚本预览与关联实例数、写入助手工具缺失等），维持 partial。

映射终值（35 行）：`[x]` 12、partial 23、boundary 0，无升级无降级。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、引用逐段可解析 35/35、`[x]` 锚点在位 12/12） | 35 行 0 verdict 问题（含 1 处行号笔误纠正与 3 行补锚点） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 188 → 185；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1654 → 1656、unrecorded 0、stale 0、unknown 52 → 51 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1911/1911 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility 全 replay 通过；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 首轮在 nextest 段已知抖动项 `api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 失败一次（隔离复跑 2/2 通过）→ 次轮 nextest 1941/1941、node 98 pass 全过 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant/assembly` 余 69 行，随后 engine 626 与 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片八：`internal/assistant/assembly` 中段 35 行（补 2 处锚点，无 verdict 翻转）

范围（按文件与行号升序）：`application_adapter_test.go:58/:81/:253/:278/:308`、`application_strategy_lifecycle_test.go:84/:122/:141`、`maintenance_test.go:12/:77`、`market_index_constituents_tools_test.go:14/:60/:69`、`market_news_tools_test.go:15/:93/:104`、`mcp_server_lifecycle_authorization_test.go:27/:74/:95`、`mcp_server_test.go:22/:76/:106/:181/:203/:257/:280`、`portfolio_tools_test.go:15/:85/:155/:255/:288/:338/:376/:444`、`product_adapters_test.go:155`（35 行中 [x]21/partial14）。

owner：组装中段覆盖 MCP 服务管理、市场指数/新闻工具、组合分层工具与策略实例生命周期；写入 owner 在领域 crate——MCP 监听与授权由 engine 的 MCP 服务端口承接，市场工具由研究/行情端口承接，组合读取由券商组合端口承接。

复核方法：35/35 参考行号签名一致；账本引用逐段解析（含 `+` 组合各自文件前缀）全部指向真实测试；21 条 `[x]` 逐条核对锚点在位（宽松匹配含省略修订前缀的既有锚点）；partial 行抽查 Go 原文与结论对应关系。

修正（无 verdict 翻转，本分片 35 行终值不变）：

1. 补 2 行 `[x]` 缺失锚点：`mcp_server_test.go:181`（端口冲突保留旧监听）与 `:280`（回环策略），锚点写在对应测试的 `#[test]` 紧邻处（该文件既有锚点省略修订前缀，新锚点用完整格式；rustfmt 对属性与函数间的注释强制去缩进，保持 fmt 稳定形态）。缺锚点告警 185 → 183。
2. `:76` 与 `:106` 的锚点以省略修订前缀形态早已在位，复核确认为有效锚点（对账脚本允许省略），无需改动。
3. partial 行结论经抽查准确（视觉模型归一化、回测态投影差异、实例生命周期写工具缺失、新闻措辞差异、组合 partial 语义等缺口均已登记），维持 partial。

映射终值（35 行）：`[x]` 21、partial 14、boundary 0，无升级无降级。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、引用逐段可解析 35/35、`[x]` 锚点在位 21/21） | 35 行 0 verdict 问题（含 2 行补锚点） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 185 → 183；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1656 → 1658、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全部通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1911/1911 通过（本分片无抖动） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compatibility 全 replay 通过；generated 未改动工作树；ai-context 6 模块 8 指令文件；zero-go 2953 files；`check:quick` 首轮全过（nextest 1941/1941 含既往抖动项 launcher、node 98 pass，本分片无抖动） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 仍因 `deny.toml` 的 `advisory-not-detected`（advisories FAILED，bans/licenses/sources ok）失败 |

后续：队列进入 `internal/assistant/assembly` 末段 34 行，随后 engine 626 与 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片九：`internal/assistant/assembly` 末段 34 行（assembly 收官，无 verdict 翻转）

范围（按文件与行号升序）：`product_adapters_test.go:15/:26/:83/:186/:217`、`product_execution_contracts_test.go:84`、`runtime_test.go:14/:45/:55/:74/:131`、`tool_catalog_test.go:16/:159/:272/:415/:550/:672/:707`、`typed_product_capabilities_test.go:11/:45`、`watchlist_adapter_test.go:24/:72`、`workflow_bridge_contracts_test.go:14/:103`、`workflow_execution_injection_test.go:60`、`workflow_tools_error_boundaries_test.go:52/:103`、`workflow_tools_test.go:15/:68/:86/:132/:183/:240/:285`（34 行中 [x]8/partial26）。至此 assembly 104 行全部收口（前 35、中段 35、末段 34）。

owner：末段覆盖产品/执行适配器、运行时生命周期、工具目录、类型化能力、自选适配器与工作流管理工具；写入 owner 在领域 crate——运行时装配由 engine composition root 承接，工具目录与适配器由 MCP 生产执行器承接，工作流管理由 ADK 读写端口承接。

复核方法：34/34 参考行号签名一致；账本引用逐段解析全部指向真实测试；8 条 `[x]` 锚点在位；partial 行抽查 Go 原文与结论对应关系（运行时生命周期、工具目录助手、轮询等待、bridge 读写、执行器注入）。

结论：35 行级复核 0 verdict 问题——`[x]` 行证据与锚点齐备；partial 行缺口均为真实结构差异（运行期工具注册通道、轮询等待工具、交互式 session 门禁、注入执行器入口等 Go 独有面已按边界保留并登记升级路径）。无升级、无降级、无新增缺口。

映射终值（34 行）：`[x]` 8、partial 26、boundary 0。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 34/34、引用逐段可解析 34/34、`[x]` 锚点在位 8/8） | 34 行 0 verdict 问题 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 183 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1658 不变、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（engine 整轮） | 1941/1941 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 全过：transport 278 ops / provider 14+9+3+3 / storage replay 通过，pineworker 98/98，nextest 1941/1941 |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：assembly 收官；队列进入 `internal/assistant/engine` 626 行（需再分片），随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十：`internal/assistant/engine` 首片 35 行（1 条过宽纠正）

范围（账本 inventory 1077-1111，按文件与行号升序）：`adk22regression/native_runtime_test.go:24/:86/:155`、`adk_edges_test.go:25/:61/:105/:153/:203/:209/:237/:294/:351/:416/:532/:684`、`adk_runner_edges_test.go:14/:142/:234/:352`、`adk_schema_test.go:8/:42`、`adk_skill_edges_test.go:13`、`adk_store_edges_test.go:11/:82/:148/:193/:272/:303/:404/:437`、`adk_tool_edges_test.go:21/:77/:131/:165/:177`（35 行中 [x]13/partial22，复核后 [x]12/partial23）。

owner：engine 运行时装配由 engine composition root 承接，存储契约由 `crates/jftrade-store-sqlite` 承接，MCP/工具目录由 MCP 生产执行器承接，任务图局部语义由 `crates/jftrade-assistant` 承接。

复核方法：35/35 参考行号签名一致（每行落点与其 `func Test...` 名相符）；13 条 `[x]` 逐条核对 Go 断言与 Rust 用例对应关系；12 条锚点在位（含省略修订前缀形态放行），1 条 `[x]` 行全仓无任何形态锚点（`adk_runner_edges_test.go:142`）触发深入复核。

结论：1 处 verdict 纠正——`adk_runner_edges_test.go:142 TestRunnerChatAndStoreAdditionalBoundaryBranches` 由 `[x]` 改为 partial。原因：原结论只覆盖 runSem 闸门与消息分支，而该 Go 测试另含 nil 接收者、非法 work mode、缺失 agent、CompleteChatRun 三态、DeleteSession 空 id、审批去重与批准后拒绝序列等分支，单条 Rust 用例（`run_gate_is_shared_across_runtime_facades`，且从未补过锚点）不能逐分支等价。已在结论中写明已覆盖分支的最近 Rust 证据、缺口分支与回归要求（engine 补 CompleteChatRun 三态表驱动用例、store-sqlite 补 confirmation 去重与批准后拒绝序列用例）。另 1 处结论收紧（verdict 不变）：`adk_edges_test.go:684` 保持 `[x]`，但结论明确其范围只到引擎/端口 fail-closed，wire 状态码差异（参考 502 对 Rust 404）仍由分片二 P1 缺口跟踪，本行不覆盖 wire。

映射终值（35 行）：`[x]` 12、partial 23、boundary 0。本分片无 Rust 生产/测试代码改动，无新增锚点，无探针。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、`[x]` 断言对应、`[x]` 锚点在位 12/12 + 1 行纠正） | 35 行 1 纠正 0 遗留过宽 |
| 账本写入 | `/tmp/s10_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1592（1593→1592）、partial 2221→2222、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 183→182；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1658 不变、已记录 1607、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（engine 整轮） | 首轮 1910/1911（`api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 抖动，隔离复跑 2/2 通过）→ 次轮 1911/1911 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 全过：compat exit 0、generated 未改动工作树、ai-context 6 模块 8 指令文件、zero-go 2953 files、quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected，advisories FAILED 其余 ok），与本分片无关 |

后续：engine 首片完成；队列进入 engine 第二片（账本 1112 起约 35 行：`adk_tool_edges` 余量与后续文件），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十一：`internal/assistant/engine` 第二片 35 行（结论收紧，无 verdict 翻转）

范围（账本 inventory 1112-1146，按文件与行号升序）：`adk_tool_edges_test.go:235/:340`、`approval_persistence_failures_test.go:9/:75`、`approval_reconciliation_lifecycle_test.go:9/:65`、`approval_retry_sibling_cancellation_test.go:14/:68`、`approval_stage_boundaries_test.go:8/:75`、`approval_state_guard_test.go:9/:114`、`canvas_provider_model_overrides_test.go:8`、`chat_request_idempotency_test.go:17/:48`、`completion_review_test.go:17/:35/:79/:121/:153/:185/:227`、`completionreview/policy_test.go:10/:30/:68/:100`、`context_cache_test.go:61/:116/:187`、`error_identity_test.go:20/:49/:68`、`event_projection_boundaries_test.go:14/:72/:87`（35 行中 [x]1/partial34，含 boundary 13）。

owner：审批/续跑资格由 ADK 运行生命周期与端口错误分类承接，审批/运行持久化由 `crates/jftrade-store-sqlite` 承接，completion review 在 Rust 无同形层（fail-open 等语义不存在），工具排序/错误哨兵属 Go 专属面。

复核方法：35/35 参考行号签名一致；唯一 `[x]` 行（`chat_request_idempotency_test.go:48`）逐分支核对 Go 断言与 Rust 用例；13 条 boundary 行全仓无锚点（符合 boundary 不得挂锚点）；抽查 partial 行 Go 原文与结论对应关系（审批持久化失败注入、状态守卫）。

结论：1 处结论收紧（verdict 不变）——`chat_request_idempotency_test.go:48` 保持 `[x]`，但原逐条对应表述只点名单条 Rust 用例，现按分支分布写明：并发首投单 run/单 lease/单会话事件由本 entry 用例锁定，重放同 run 与冲突拒绝由路由级 `adk_chat_idempotency_contract_matches_the_go_routes` 锁定（同 body 重放 200 同 run id、换 message 报 409），responses SSE 原生文案属 provider mock 细节无文案级断言。其余 34 行 verdict 维持：partial 行缺口均为真实结构差异（故障注入、审批专用资格函数、旧 handoff 字符串级断言、工具显式排序、哨兵往返表），boundary 行均为 Go 专属面（planner 草稿编译层、completion review 整层、哨兵 AST 守卫）并已登记升级路径或保留结论。

映射终值（35 行）：`[x]` 1、partial 21、boundary 13。本分片无 Rust 生产/测试代码改动，无新增锚点，无探针。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、`[x]` 断言对应、boundary 无锚点 13/13、partial 抽查） | 35 行 0 verdict 问题，1 结论收紧 |
| 账本写入 | `/tmp/s11_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1592 不变、partial 2222 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 182 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1658 不变、已记录 1607、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（engine 整轮） | 首轮 1910/1911（`adk_session_detail_omits_resolved_approval_groups` 抖动，隔离复跑 3/3 通过）→ 次轮 1910/1911（`api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 抖动，隔离复跑 2/2 通过）→ quick 内第三轮 1941/1941 通过 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 全过：compat exit 0、generated 未改动工作树、ai-context 6 模块 8 指令文件、zero-go 2953 files、quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第二片完成；队列进入 engine 第三片（账本 1147 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十二：`internal/assistant/engine` 第三片 35 行（5 条过宽纠正）

范围（账本 inventory 1147-1181，按文件与行号升序）：`event_projection_reply_ordering_test.go:15`、`exec_bounds_test.go:14/:57/:93/:148/:175/:238/:275/:364`、`exec_state_bounds_test.go:9/:101/:171`、`execution_claim_failure_boundaries_test.go:39/:83/:173/:203`、`execution_claims_test.go:54/:108/:167/:214/:265/:294/:343`、`execution_state_projection_contracts_test.go:44/:103/:175/:234`、`goal_state_boundaries_test.go:9`、`google_exec_concurrency_test.go:12`、`google_execution_replay_guards_test.go:13/:67`、`google_memory_test.go:10/:77`、`google_runner_failure_diagnostics_test.go:8/:35`（35 行中 [x]9/partial26，复核后 [x]4/partial31）。

owner：执行调用复用/完成与回调串行由模型运行时投影与围栏层承接，租约/认领生命周期由 `crates/jftrade-assistant` 的 ClaimStore 与 claims 契约测试承接，失败分类与恢复由运行时 failure/recovery 端口承接。

复核方法：35/35 参考行号签名一致；9 条 `[x]` 全部打开 Go 断言逐分支核对（调用复用六分支、租约校验全表、工具认领失败表、失败读三段式、心跳接管三段式、并发回调）；9 条锚点全部在位。

结论：5 处 verdict 纠正（均为 [x] 改 partial，原逐条覆盖不成立）。一是 `exec_bounds_test.go:57`：两条引用用例只覆盖失败落调用与 pre-tool 冻结，调用复用去重、缺失完成 no-op、成功摘要、TIMED_OUT 分类无同形断言。二是 `execution_claim_failure_boundaries_test.go:39`：两条引用用例只覆盖围栏接管续租，TTL 非正拒绝、空 id、空释放、缺失读取、零时间戳均无测试断言（TTL 校验只在生产代码）。三是 `execution_claim_failure_boundaries_test.go:83`：两条引用用例只覆盖过期接管，复用键输入不一致、心跳 TTL、encode 错误、Abandon 后 Lost 序列无同形断言。四是 `execution_claims_test.go:108`：引用用例主题是过期接管，没有失败读持久 COMPLETED 加投影 FAILED 的成对断言。五是 `google_exec_concurrency_test.go:12`：引用用例证明至多一次执行，不能证明回调串行化（无 delta 重叠检测）。另 1 处结论收紧（verdict 不变）：`execution_claims_test.go:294` 保持 `[x]`，自愿释放后接管围栏递增单列为行内缺口。`:148`（取消 join）与 `:167`（换手围栏）逐分支核对无误，保持 `[x]`。每条纠正均写明缺口分支与回归位置（engine 补调用复用与回调串行用例、assistant claims 补校验表与成对语义用例）。

映射终值（35 行）：`[x]` 4、partial 31、boundary 0。本分片无 Rust 生产/测试代码改动，无新增锚点，无探针。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、`[x]` 断言逐分支、`[x]` 锚点在位 9/9） | 35 行 5 纠正 1 收紧 |
| 账本写入 | `/tmp/s12_payload.json` 经 `/tmp/b82_apply.py` 应用 | `[x]` 1587（1592→1587）、partial 2222→2227、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 182 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1658 不变、已记录 1607、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（engine 整轮） | 首轮 1911/1911 一次通过，无抖动 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 全过：compat exit 0、generated 未改动工作树、ai-context 6 模块 8 指令文件、zero-go 2953 files、quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第三片完成；队列进入 engine 第四片（账本 1182 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十三：`internal/assistant/engine` 第四片 35 行（6 处缺失锚点补齐，无 verdict 翻转）

范围（账本 inventory 1182-1216，按文件与行号升序）：`handoff_notice_test.go:10/:100`、`input_continuation_failure_recovery_test.go:13/:110/:140`、`input_continuation_idempotency_test.go:14/:87`、`input_request_test.go:16/:72/:114/:225/:305/:413/:449/:490/:530/:556/:584/:647/:694/:738/:773/:802`、`input_workflow_test.go:9`、`lifecycle_reconciliation_failures_test.go:9/:102`、`mcp_server_test.go:17/:119/:129/:136/:162/:194/:266/:293/:333`（35 行中 [x]23/partial12，含 boundary 1）。

owner：输入请求/续跑/幂等由 engine 输入端口与 ADK 认领层承接，MCP 目录与宿主防护由 MCP 生产执行器承接，会话时间线由事件投影拥有（`input_request :530` 记 boundary）。

复核方法：35/35 参考行号签名一致；23 条 `[x]` 全部逐分支核对 Go 断言（输入校验三段、取消与迟答、暂停续跑、顺序问答、审批流转、重启续跑、载荷锚点、MCP 目录与宿主防护）；17 条锚点名字逐个匹配，6 条缺失锚点在核对通过后补齐。

结论：0 verdict 翻转。6 处缺失锚点全部补齐（均为先核对通过、再补锚点）：`input_request :114`（校验一半，声明一半在另一用例）、`:449`（校验幂等冲突加回滚三段）、`:584`（原散文 Reference 行升级为规范锚点，awaiting_input 审计一半）、`mcp_server :129`（空目录不可构造的结构不变式）、`:136`（关闭幂等加拒绝重绑）、`:266`（evil 与缺失 Host 双 403）。其余 `[x]` 行证据与锚点齐备（`:72` 校验一半加端到端一半互链、`:738` 含上批修错的 resumeState 回归、MCP 各行均为镜像命名的新增复刻用例）。partial 行缺口均为真实结构差异。

新增证据（注释 only）：6 行规范锚点分属 `product_adk_input_request_parity_tests.rs`（1）、`product_production_ports_adk_tests.rs`（1）、`product_mcp_server_tests.rs`（3）、`product_adk_model_runtime_terminal_audit_tests.rs`（1），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 23、partial 11、boundary 1。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、`[x]` 断言逐分支、`[x]` 锚点名字 23/23）在位 | 35 行 0 verdict 问题，6 锚点补齐 |
| 账本写入 | 本分片只改 Rust 注释与文档账本再生，不涉及 verdict 变更 | `[x]` 1587 不变、partial 2227 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 182→176；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1658→1664、已记录 1607→1613、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | 全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast`（engine 整轮） | 首轮 1911/1911 一次通过，无抖动 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | 全过：compat exit 0、generated 未改动工作树、ai-context 6 模块 8 指令文件、zero-go 2953 files、quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第四片完成；队列进入 engine 第五片（账本 1217 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十四：`internal/assistant/engine` 第五片 35 行（1 处缺失锚点补齐，无 verdict 翻转）

范围（账本 inventory 1217-1251，按文件与行号升序）：`mcp_server_test.go:358/:367/:382/:440`、`normalize_test.go:5/:62`、`observability_test.go:10`、`persistence/approval_query_plan_test.go:9`、`persistence/composer_normalize_test.go:8`、`persistence/execution_claims_test.go:58/:93/:186/:229`、`persistence/google_artifact_test.go:19/:120/:186/:247/:420`、`persistence/provider_reasoning_test.go:10/:47`、`persistence/provider_selection_test.go:8/:29`、`persistence/secret_store_test.go:9`、`persistence/session_sqlite_boundaries_test.go:14`、`persistence/session_sqlite_schema_test.go:15/:49/:82/:127`、`persistence/session_sqlite_test.go:30/:58/:108/:138`、`persistence/store_run_test.go:11/:31`、`persistence/task_patch_test.go:8`（35 行中 [x]12/partial23，含 boundary 2）。

owner：MCP 状态面由 MCP 生产执行器承接，租约/重放/崩溃策略由 assistant claims 层承接，provider 选择与工件/会话持久化由 `crates/jftrade-store-sqlite` 承接，provider reasoning 映射在 Rust 无对应面（两行记 boundary）。

复核方法：35/35 参考行号签名一致；12 条 `[x]` 全部逐分支核对（MCP 订阅校验与执行器现解析、租约围栏、重放与崩溃策略、工件跨重启、provider 排序修复、会话库拒绝与重开）；11 条锚点名字逐个匹配，1 条缺失锚点在核对通过后补齐；partial 行抽查 Go 原文与结论对应关系（跨连接原子认领）。

结论：0 verdict 翻转。1 处缺失锚点补齐：`persistence/provider_selection_test.go:29` 的引用测试（default 行在前、created_at ASC、恰好一个 default 且修复持久化，结论含 probe 转红证据）核对通过后补规范锚点；该行在 store 合约测试中的旧散文引用是 `:8` 行锚点的续行说明，不作该行证据。`:382`/`:440` 的结论本就按边界加等价不变式双写（无推送面但每次请求重投影、每次调用现解析执行器），与 verdict 一致。其余 `[x]` 行证据与锚点齐备。partial 行缺口均为真实结构差异（nil 切片归一、跨连接序列化、V1 无损迁移、reasoning 快照等 Go 专属面）。

新增证据（注释 only）：1 行规范锚点（`product_adk_model_runtime_gate_tests.rs`），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 12、partial 21、boundary 2。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 自建校验（参考行号签名 35/35、`[x]` 断言逐分支、`[x]` 锚点名字 12/12）在位 | 35 行 0 verdict 问题，1 锚点补齐 |
| 账本写入 | 本分片只改 Rust 注释与文档账本再生，不涉及 verdict 变更 | `[x]` 1587 不变、partial 2227 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact、rust_entry 唯一；缺锚点告警 176→175；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1664→1665、已记录 1613→1614、unrecorded 0、stale 0、unknown 51 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt 通过、clippy exit 0、architecture 通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1911/1911 passed，exit 0 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第五片完成；队列进入 engine 第六片（账本 1252 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十五：`internal/assistant/engine` 第六片 35 行（2 处锚点修正，无 verdict 翻转）

范围（账本 inventory 1252-1286，按账本顺序）：`providers/probe_test.go:81/:105`、`providers/reasoning_effort_transport_test.go:14/:64`、`providers/responses_model_test.go:18/:48/:87/:96/:112/:144`、`providers/safe_http_test.go:14/:41`、`reasoning_effort_lifecycle_test.go:10/:23`、`responses_model_runtime_test.go:9`、`responses_stream_projection_test.go:12`、`resumed_execution_recovery_boundaries_test.go:14/:92/:111`、`run_timeline_test.go:8`、`runner_approval_concurrency_test.go:12/:92/:119/:154/:191`、`runner_chat_callbacks_test.go:49`、`runner_chat_continuation_signal_test.go:9/:22/:52`、`runner_chat_runtime_branches_test.go:55/:190/:337/:487`、`runner_chat_test.go:18`（[x]，组合证据）、`:56`（boundary，RequestedInput 触发面不存在）（35 行中 [x]5/partial18/boundary12）。

owner：provider 探针/推理传输/模型请求面由 provider 适配层承接（Rust 无 reasoning 注入与 safe dial 面，记边界缺口），审批并发与续跑租约由 assistant claims 层与 engine fencing 层承接，聊天校验与闸门由 ADK 聊天端口承接。

复核方法：5 条 `[x]` 全部逐分支核对 Go 原文与 Rust 证据（并发审批幂等与冲突安全、兄弟审批合并后只续跑一次、已认领 continuation 答 envelope 且工具只执行一次、取消信号传播到等待方、并发闸门第 11 个失败关闭与释放重放行；:18 的空消息/超长/裁剪分支由聊天错误分类用例覆盖，最大长度按 rune 计数、裁剪后度量）；partial/boundary 行抽查结论与 Go 原文对应关系（safe dial、reasoning 注入、流式 usage、continuation-only 信号识别等面在 Rust 确无对应实现）。

结论：0 verdict 翻转。2 处锚点修正：其一 `runner_approval_concurrency_test.go:119` 的既有锚点误写 `:105`（落到 unknown 桶），行为核对通过后按基线函数行号修正为标准单行锚点；其二 `runner_chat_test.go:18` 缺锚点，行为经组合证据核对通过后补规范锚点，并把该行 entry 补成组合形式（闸门用例 + 聊天错误分类用例，组合全文在 [x] 行间唯一）。:12/:92/:191 的锚点行号与基线一致、证据齐备。

新增证据（注释 only）：1 行锚点修正（`product_production_ports_adk_tests.rs`）、1 行规范锚点（`product_adk_model_runtime_gate_tests.rs`），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 5、partial 18、boundary 12。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 5 条 [x] 逐分支核对、partial/boundary 抽查 Go 原文 | 35 行 0 verdict 问题，2 锚点修正 |
| 账本写入 | 1 行 entry 补组合形式（b82_apply），verdict 不变 | `[x]` 1587 不变、partial 2227 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 175→173；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1665→1666、已记录 1614→1616、unrecorded 0、stale 0、unknown 51→50 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt 通过、clippy exit 0、architecture 通过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 首轮 1910/1911（1 抖动，见下），隔离 3/3 后第二轮 1911/1911 全绿；抖动为 compacted_context 消息排序用例，与本分片注释改动无关（不同文件、注释 only） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第六片完成；队列进入 engine 第七片（账本 1287 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十六：`internal/assistant/engine` 第七片 35 行（1 处过宽纠正为 partial，5 处缺锚点补齐）

范围（账本 inventory 1287-1321，按账本顺序）：`runner_chat_test.go:69/:102/:127/:156/:181/:225/:267/:325/:376/:423/:490/:541/:552/:609/:670/:737/:846/:891/:978/:1043/:1079/:1124`、`runner_continuation_boundaries_test.go:11/:63/:87/:136/:188/:207/:242/:275/:314/:347/:406/:427`、`runner_goal_test.go:11`（35 行原结论 [x]30/partial4/boundary1，本批纠正后 [x]29/partial5/boundary1）。

owner：聊天投影/终态/审计由 engine 模型运行时承接，快照与默认选择由 gate 层承接，continuation 认领与 fencing 由 supervisor 层承接，目标暂停/恢复路由由 ADK 变更端口承接。

复核方法：30 条原 [x] 逐条核对锚点行号与名字（基线函数行号全部一致；:63/:314/:347 为双行锚点，名字在次行，已核对）；5 条缺锚点（:156 只有 Reference 行、:423/:670/:978/:207 无锚点）逐分支核对 Go 原文与 Rust 证据后补齐，其中 :978 与 :207 的 entry 按组合规则补成组合形式（:978=状态优先级用例+聊天错误分类用例，覆盖缺省 agent/禁用/删除/provider 禁用/无密钥全部分支；:207=已认领唤醒用例+CAS 拒绝用例，覆盖同步回 staged 与后台不动 foreign owner 两半）；:423 的 entry 由模块路径写法改为真实文件路径写法（此前写法导致对账报 stale）。其余已锚定 [x] 抽查结论与证据对应关系（:225 的 sha256 摘要 id、:891 的关流后续跑、:1043 的会话复用等）。

结论：1 处 verdict 翻转（纠正过宽）：`runner_continuation_boundaries_test.go:275` 由 [x] 降为 partial。原因：Go 断言后台目标续跑的两条租约路径（fresh foreign lease 不夺取、租约存储错误落 FAILED 且原因真实），而 Rust 的目标续跑是同步 ResumeRun 路由（不认领执行租约），原 entry 的终态不可续跑守卫用例与租约无关；approval continuation 的 foreign lease 证据属于另一条路径，不能作为目标续跑的等价证据。缺口与回归要求已写入该行结论。partial/boundary 行结论与 Go 原文相符（租约错误码、逐表错误文本、goBackground 兜底、pause-requested 幂等等缺口均为真实结构差异）。

新增证据（注释 only）：6 行规范锚点（terminal_audit :156 把 Reference 行升级为 Parity 锚点、tool_failure :423、gate :670/:978、ports_adk :207 双测），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 29、partial 5、boundary 1。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 30 条原 [x] 锚点与行为核对、partial/boundary 抽查 Go 原文 | 1 verdict 纠正（:275→partial），5 锚点补齐，2 entry 组合化，1 entry 文件路径修正 |
| 账本写入 | 3 行 entry 调整 + 1 行 verdict 变更（b82_apply），其余不动 | `[x]` 1586、partial 2228、boundary 637（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 173→168；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1666→1671、已记录 1616→1621、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt 通过、clippy exit 0、architecture 通过；相关 6 测全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1911/1911 passed，exit 0，一次通过无抖动 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick exit 0（nextest 1941/1941、pineworker 98/98） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关 |

后续：engine 第七片完成；队列进入 engine 第八片（账本 1322 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十七：`internal/assistant/engine` 第八片 35 行（1 处单测 entry 纠正为三元组合，无 verdict 翻转）

范围（账本 inventory 1322-1356，按账本顺序）：`runner_goal_test.go:72/:111/:174/:264`、`runner_lifecycle_boundaries_test.go:24`、`runner_lifecycle_reconciliation_test.go:9/:70/:106/:146`、`runner_lifecycle_shutdown_failures_test.go:8`、`runner_plugin_test.go:11/:38`、`runtime_execution_lease_boundaries_test.go:14/:108/:133/:144/:161/:176/:234`、`runtime_store_test.go:36/:117/:174/:193/:286/:362/:409/:434/:454/:485`、`session_compaction_boundaries_test.go:12`、`session_context_conflict_test.go:11`、`session_context_json_test.go:9`、`session_context_projection_test.go:15/:60/:118`（35 行中 [x]1/partial29/boundary5）。

owner：目标暂停/恢复与生命周期对账由 ADK 变更端口与恢复扫描承接，执行租约认领/心跳/续期由 assistant claims 层与 supervisor 层承接，store 启动与内置目录由组装层承接，会话压缩与上下文投影由 session 上下文层承接。

复核方法：唯一 [x]（`:176`）按基线逐分支核对，发现原单测 entry 只覆盖等待半，纠正为三元组合；partial 行逐条核对结论与 Go 原文的缺口描述（租约心跳触发链、续租 TTL 细节、标题截断、pause-requested 幂等、nil 管理器等），其中 :72 的 resume 四状态分支经核对确认缺直接断言（缺失运行 404 与 chat resume 400 有覆盖，child/running/unsupported-reason 的 resume 拒绝无逐条断言），partial 结论成立；boundary 行核对 Go 专属面（插件 nil execution、nil runtime 访问器、omitempty 序列化等）在 Rust 确无对应实现。

结论：0 verdict 翻转。1 处 entry 纠正：`runtime_execution_lease_boundaries_test.go:176` 由单测 entry 改为三元组合（200 轮竞态等待半 + 取消信号半 + 认领释放半，分别对应 Go 的等待返回、取消在途、租约行清空三条断言），组合全文在 [x] 行间唯一，三处锚点同在 fencing_tests.rs。

新增证据（注释 only）：2 行规范锚点（:176 在取消半与认领半用例各一），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 1、partial 29、boundary 5。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 1 条 [x] 逐分支核对、34 条 partial/boundary 结论与 Go 原文抽查 | 0 verdict 问题，1 entry 组合化 |
| 账本写入 | 1 行 entry 调整（b82_apply），verdict 不变 | `[x]` 1586 不变、partial 2228 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 168 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1671 不变（:176 新增两锚点指向同一行）、已记录 1621 不变、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt 通过、clippy exit 0、architecture 通过；:176 三元组合 3 测全过 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1911/1911 passed，exit 0，一次通过无抖动（含两 launcher 集成测） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick 共 7 轮：第 2-5 轮倒在 launcher 已知抖动、第 6 轮倒在 input_response 重启时序与 web 单测 5 秒超时（隔离复测分别 3/3、8/8 通过，工作树零 web 改动），第 7 轮工作区 3419/3419（含两 launcher 测与 :176 三元组合）、web 2435/2435、pine 98/98、python 337 全绿，仅 static 未过（见下一行） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected），与本分片无关；第 7 轮 quick exit 1 唯一原因即此项 |

后续：engine 第八片完成；队列进入 engine 第九片（账本 1357 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十八：`internal/assistant/engine` 第九片 35 行（3 处过宽纠正，无新增实现）

范围（账本 inventory 1357-1391，按账本顺序）：`session_context_projection_test.go:181/:208`、`session_context_recovery_edges_test.go:9`、`session_context_retry_boundaries_test.go:89/:143`、`session_context_stale_test.go:106/:160/:188/:209/:231/:315/:420`、`session_context_test.go:15/:120/:183/:287/:341/:446/:506/:569/:651/:703/:773/:787/:800/:813/:827/:868/:922/:977/:1009`、`session_skill_test.go:15/:68`、`session_wrap_test.go:9`、`skill_recover_test.go:11`（35 行中 [x]20/partial12/boundary3）。

owner：会话上下文投影与压缩由 ADK 会话上下文层承接，事件追加原子性由 SQLite ADK 存储层承接，技能目录由组装层承接。

复核方法：20 条 [x] 逐条对照 Go 基线分支。压缩与 revision 链（:15/:120/:183/:287/:341/:446/:506/:569/:651/:703）及保护尾部与 handoff 链（:773/:787/:800/:813/:827/:922/:977/:1009）分支相符，保留 [x]；其中 :15 明确只就快照与持久投影断言（Go ADK 库表面的 InstructionSuffix 与包装视图分支在结论中划界）。12 条 partial 与 3 条 boundary 的缺口描述与 Go 原文相符（事件索引助手、追加重试计数、合成会话创建、包装访问器、内置排序等均无独立断言；raw 会话句柄、stale 刷新、压缩包装服务确无 Rust 对应层）。

结论：3 处 verdict 纠正（均为过宽降级）。一是 `session_context_stale_test.go:106` 由 [x] 降为 partial：Go 断言 12 路并发追加全部成功（刷新重试、事件数 12、锁表清空），Rust 的身份测试只断言外来事件被拒绝（无重试路径），且结论误写 Go 为唯一成功；二是 `session_context_stale_test.go:315` 由 [x] 降为 partial：Go 断言逐条工具响应裁剪（计数 1、truncated 标记与预览、raw 大于有效 token），Rust 的两个引用测试只断言通用压缩收缩，原结论逐条覆盖不成立，两处降级同时消除了与 :15/:703 的等价条目张力；三是跨分片纠正 `store_test.go:792` 由 [x] 降为 partial：其 rust_entry 与 `runner_chat_test.go:423` 的 [x] 行完全重复（审计唯一性阻断），且批准解析入口、assistant 汇总消息与该路径的 run.completed 审计无端到端断言（此前审计在分片十七基线已存在该重复，本批首次跑审计即阻断，属历史遗留）。

新增证据（注释 only）：10 行规范锚点（:15/:120/:183/:287/:341/:446/:506/:569/:651/:703，各归所属测试），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 18、partial 14、boundary 3。全量：`[x]` 1583、partial 2231、boundary 637（合计 4451）；Rust 测试 3293 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 20 条 [x] 逐分支核对、15 条 partial/boundary 结论与 Go 原文抽查 | 3 verdict 纠正（:106、:315、跨分片 :792），10 锚点补齐 |
| 账本写入 | 3 行 verdict 变更（b82_apply），其余不动 | `[x]` 1586→1583、partial 2228→2231、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 168→157；Rust 测试 3293 不变；审计附带刷新 inventory 滞后行（分片十六 :275、分片十七 :176 及 :423 路径） |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1671→1681、已记录 1621→1631、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt exit 0、clippy exit 0、architecture 通过；改动仅注释锚点加账本结论 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1911/1911 passed，exit 0，一次通过无抖动（含两 launcher 集成测）；session_context 定向 23/23，store-sqlite projection 相关 4/4 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick 全绿（workspace 1941/1941、pineworker 98/98，零失败签名；web 段按 affected 未触发） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected，RUSTSEC-2026-0285 无 crate 命中），与本分片无关 |

后续：engine 第九片完成；队列进入 engine 第十片（账本 1392 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片十九：`internal/assistant/engine` 第十片 35 行（0 翻转，无新增实现）

范围（账本 inventory 1392-1426，按账本顺序）：`skill_recover_test.go:39/:114`、`skill_reg_fs_test.go:19/:48/:72/:89/:139/:209`、`skill_reg_test.go:21/:84/:115/:162/:235/:286/:361`、`skill_registry_archives_test.go:45`、`skill_registry_http_sources_test.go:19/:198`、`skillsruntime/install_boundary_test.go:14`、`skillsruntime/schema_market_index_constituents_test.go:8/:27`、`skillsruntime/schema_market_news_test.go:8/:49`、`skillsruntime/schema_test.go:9/:29/:48`、`sqlite_dialector_boundaries_test.go:14`、`sqlite_tools_test.go:35/:79/:145/:194/:333`、`store_approve_test.go:10/:41/:98`（35 行中 [x]0/partial31/boundary4）。

owner：技能目录同步与安装由组装层承接，工具与 MCP schema 由 MCP 目录层承接，SQLite 方言由 Go GORM 持久化层承接（Rust 无对应层），审批存储语义由 SQLite ADK 存储层承接。

复核方法：本片无 [x] 行，只核对 partial/boundary 结论与 Go 原文。35 行引用测试全部存在。抽查确认：`:194` 的 strategy.optimize 必填字段（definitionIds、market、symbol、startTime、endTime）与 Rust 回归测试逐项一致，其余 18 个工具待各自领域批次，partial 成立；`store_approve:10` 的返回原始 approved 记录字段级断言在 Rust 只有 CAS 拒绝路径的行级断言，partial 成立；`skill_reg_fs:72` 的未知工具 WARNING 与 Rust 的拒绝策略属已记录差异，partial 成立；`:115` 的多根歧义与文档大小上限在 Rust 无逐条断言，partial 成立；`:48` 的 zip 目录条目形态在 Rust 的压缩包 helper（只写文件条目）中无专门断言，partial 成立；4 条 boundary（GORM dialector 打开、AutoMigrate、类型映射、SQL 构造器与版本比较）确无 Rust 对应层。无升级候选，无过宽纠正。

结论：0 verdict 翻转，无账本写入（b82 未跑），无新增证据，无探针。

映射终值（35 行）：`[x]` 0、partial 31、boundary 4。全量：`[x]` 1583、partial 2231、boundary 637（合计 4451）；Rust 测试 3293 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条 partial/boundary 结论与 Go 原文抽查、引用测试存在性全查 | 0 verdict 问题，引用 35/35 存在 |
| 账本写入 | 无变更 | `[x]` 1583 不变、partial 2231 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 157 不变；Rust 测试 3293 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1681 不变、已记录 1631 不变、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`pnpm run check:rust:architecture` | fmt exit 0、clippy exit 0、architecture 通过；本片无 Rust 代码改动 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` | 1911/1911 passed，exit 0，一次通过无抖动（含两 launcher 集成测） |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick 全绿（workspace 1941/1941、pineworker 98/98，零失败签名） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | 按预期失败（deny.toml advisory-not-detected，RUSTSEC-2026-0285 无 crate 命中），与本分片无关 |

后续：engine 第十片完成；队列进入 engine 第十一片（账本 1427 起约 35 行），engine 共 626 行约 18 片；随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十：`internal/assistant/engine` 第十一片 35 行（2 处过宽纠正 + 1 条目形态修正，无新增实现）

范围（账本 inventory 1427-1461，按账本顺序）：`store_async_test.go:10/:89`、`store_audit_query_test.go:7`、`store_business_test.go:22/:108/:275/:412/:563`、`store_entity_lifecycle_edges_test.go:10`、`store_failure_normalization_boundaries_test.go:10/:170`、`store_identity_test.go:10/:47/:85`、`store_lifecycle_test.go:17/:45/:110/:146/:250/:281/:328/:466/:501/:553/:565/:609/:641/:701/:711/:735/:765/:792`、`store_maintenance_handoff_test.go:10/:93`、`store_maintenance_test.go:8`（35 行中 [x]10、partial25、boundary0）。

owner：异步审批续跑与审计分页由 SQLite ADK 存储层承接，provider 生命周期与业务查询由生产端口层承接，run 终态机与重开语义由存储 CAS 层承接，会话级联删除由三库清理路径承接，技能安装/卸载与工具白名单由组装层承接，维护交接与配置 purge 由存储维护层承接。

复核方法：12 条原 [x] 逐条对照 Go 基线分支。保留的 9 条 [x] 分支相符：引用中 provider 删除拒绝（:17，条目形态已修正）、会话级联三库清零（:110）、会话/审批/任务分页排序（:466/:565/:609）、composer 状态随会话删除（:501）、非安全 host 与内置技能卸载拒绝（:701/:711）、外部技能卸载清安装目录（:735）、预备 agent 只装启用技能（:765）。25 条 partial 的缺口描述与 Go 原文相符（异步拒绝连带兄弟审批、审计 SQL 层计数与越界空页、任务依赖与记忆作用域、密钥可见性、终态回归谓词例外、工具调用参数透传、维护交接幂等等均无逐条断言或属已登记结构差异）。

结论：2 处 verdict 纠正（均为过宽降级）+ 1 处条目形态修正。其一是 `store_business_test.go:22` 由 [x] 降为 partial：Go 是 provider 全生命周期用例（保存默认项、密钥读写、列表默认排序、能力更新、引用中删除拒绝、删默认后替代、密钥删除），原 [x] 条目竟引用生产函数且只覆盖未知删除幂等一支，Rust 无密钥轮转与能力更新逐条断言；其二是 `store_lifecycle_test.go:45` 由 [x] 降为 partial：Go 的列表密钥可见性与空白删除 store 语义在 Rust 无逐条断言（排序、删默认提升、切换由组合引用覆盖）；其三是 `store_lifecycle_test.go:17` 保持 [x]，但条目首段同样误引生产函数，已收敛为单一真实测试。三处 rust_entry 现均为真实测试，[x] 行间唯一。

新增证据（注释 only）：3 行规范锚点（:17 与 :735 归 `product_production_ports_adk_tests.rs`，:110 归 `adk_cascade_session_cleanup.rs`），无生产实现改动，无探针。

跨分片确认（审计线程提醒已落库）：`store_test.go:792` 与 `runner_chat_test.go:423` 共用同一 Rust 测试的重复 [x]，已在 679ea8e1（第九片提交）降为 partial；本工作树 dup-x 为 0，本次审计唯一性检查通过（raise 未触发），report 与 inventory 为校验通过后重生成，不存在半旧状态。

映射终值（35 行）：`[x]` 10、partial 25、boundary 0。全量：`[x]` 1581、partial 2233、boundary 637（合计 4451）；Rust 测试 3293 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 12 条原 [x] 逐分支核对、23 条 partial 结论与 Go 原文抽查 | 2 verdict 纠正（:22、:45）加 1 条目形态修正（:17），3 锚点补齐 |
| 账本写入 | 3 行变更（b82_apply），其余不动 | `[x]` 1583→1581、partial 2231→2233、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 157→153；Rust 测试 3293 不变；重复 `[x]` 唯一性检查通过 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1681→1684、已记录 1631→1634、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`check:rust:architecture` | fmt exit 0、clippy exit 0、architecture 通过；改动仅注释锚点加账本结论 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` 及 sqlite 级联定向 | 1911/1911 passed，exit 0，一次通过无抖动；sqlite 级联定向 1/1（187 skipped），exit 0 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick exit 0（rust workspace 2129/2129、pineworker 98/98、compat replay 全过、route 278/278，web 段按 affected 未触发） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | static/policy 均 exit 1，失败原因与预期一致（deny advisory-not-detected，RUSTSEC-2026-0285 无 crate 命中），与本分片无关 |

后续：engine 第十一片完成（626 行中 385 行）；队列进入 engine 第十二片（账本 1462 起约 35 行，以 `store_maintenance_test.go:54` 开头），随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十一：`internal/assistant/engine` 第十二片 35 行（3 处过宽纠正 + 1 组合修正 + 1 结论收紧，无新增实现）

范围（账本 inventory 1462-1496，按账本顺序）：`store_maintenance_test.go:54`、`store_ops_test.go:21/:36/:88/:128/:147/:184/:224/:281/:294/:320/:363/:402/:426/:436/:452/:486/:527/:565/:621/:745/:788/:844/:899/:939/:961/:998`、`store_recover_test.go:9/:66/:111/:127`、`store_test.go:59/:75/:127/:198`（35 行中 [x]20、partial15、boundary0）。

owner：技能与模板目录由组装层承接，agent 软删与恢复由生产端口层承接，run 取消/审批幂等/分页由生产端口与存储 CAS 层承接，重启续跑与孤立回收由运行时 reconcile 承接，任务/记忆/工具域由生产端口层承接，provider 默认修复与写 fencing 由 SQLite 存储层承接。

复核方法：23 条原 [x] 逐条对照 Go 基线分支与 Rust 断言。保留的 19 条 [x] 分支相符：压缩包安装保资源（:184）、异属会话拒绝（:281）、agent 软删/列表/恢复（:294/:320/:363）、取消连带拒绝审批（:402）、缺失取消与幂等空包络（:426/:436）、存储层幂等（:452）、run 分页排序（:486）、重复批准单续跑（:527）、重启恢复恰好执行一次（:621）、孤立 run 标 FAILED（:745）、双审批门禁（:788）、任务归一化（:844）、记忆过滤（:899）、记忆注入开关（:939）、默认修复组合（:9）、写 fencing（:59）。12 条 partial（含 3 条新增降级）的缺口描述与 Go 原文相符。

结论：3 处 verdict 纠正（均为过宽降级）+ 1 处组合修正 + 1 处结论收紧。其一是 `store_ops_test.go:128` 由 [x] 降为 partial：Go 断言默认模板技能集、模板仅 1 个、4 个退役 id 全部下线，Rust 引用测试只断言默认助手存在与 investment-analyst 缺席一支，且其锚点指向另一基线用例；其二是 `store_ops_test.go:961` 由 [x] 降为 partial：Go 执行 tools.search 断言只返回当前 agent 可见工具，Rust 只断言 scope 归一化谓词，且 Reference 误指另一文件；其三是 `store_ops_test.go:998` 由 [x] 降为 partial：Go 经 Chat 执行 6 个低风险写入断言各执行一次且零 pending，Rust 只断言豁免谓词，chat 接线仍是 follow-up；其四是 `store_ops_test.go:565` 保持 [x] 但由单条目扩展为组合（终局投影测试 + :621 重启 e2e 测试），分别覆盖审计顺序与重启执行一次分支；其五是 `store_recover_test.go:9` 保持 [x]，结论收紧（HasAPIKey 无同形断言的结构性说明：Rust 密钥在 payload 外，修复不碰密钥）。组合条目在 [x] 行间唯一。

新增证据（注释 only）：7 行规范锚点（:184 归 skill mutation 测试，:281/:939 归 gate 测试，:294/:320/:363/:402 归 ports 测试），无生产实现改动，无探针。

映射终值（35 行）：`[x]` 20、partial 15、boundary 0。全量：`[x]` 1578、partial 2236、boundary 637（合计 4451）；Rust 测试 3293 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 23 条原 [x] 逐分支核对、12 条 partial 结论抽查 | 3 verdict 纠正（:128、:961、:998）加 1 组合修正（:565）加 1 结论收紧（:9），7 锚点补齐 |
| 账本写入 | 5 行变更（b82_apply），其余不动 | `[x]` 1581→1578、partial 2233→2236、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 153→144；Rust 测试 3293 不变；重复 `[x]` 唯一性检查通过 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1684→1691、已记录 1634→1641、unrecorded 0、stale 0、unknown 50 不变 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`check:rust:architecture` | fmt exit 0、clippy exit 0、architecture 通过；改动仅注释锚点加账本结论 |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` 及 assistant/sqlite 受影响定向 | 1911/1911 passed，exit 0，一次通过无抖动；assistant 41/41 exit 0；sqlite 定向 2/2 exit 0 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick 第二轮 exit 0（rust 1941/1941、pineworker 98/98；首轮倒在 launcher 已知抖动，隔离复跑 1/1 后整轮确认） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | static/policy 均 exit 1，失败原因与预期一致（deny advisory-not-detected），与本分片无关 |

后续：engine 第十二片完成（626 行中 420 行）；队列进入 engine 第十三片（账本 1497 起约 35 行），随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十二：`internal/assistant/engine` 第十三片 35 行（1 处过宽纠正 + 1 组合修正 + 1 结论收紧 + 1 锚点行号纠正，无新增实现）

范围（账本 inventory 1497-1531，按账本顺序）：`store_test.go:254/:295/:373/:385/:437/:471/:538/:607/:670/:720/:792/:862/:881/:898/:947/:1005`、`task_runner_test.go:13/:79`、`taskset_biz_test.go:9`、`timeline_projection_helpers_test.go:11`、`tool_artifact_materialization_test.go:24/:52/:73`、`tool_registry_change_test.go:8`、`tool_schema_workflow_test.go:10/:35`、`tools_net_transport_boundaries_test.go:12`、`tools_security_test.go:11/:33`、`tools_test.go:25/:44/:61/:87/:106/:129`（35 行中 [x]19、partial16、boundary0）。

owner：legacy 库拒绝与 schema 收敛由 SQLite 存储层承接，provider 密钥与超时归一化由生产端口层承接，审批门禁/续跑/终局由运行时与存储 CAS 层承接，超时与过期回收由运行时 reconcile 承接，工具 schema 与豁免谓词由 MCP 目录与 assistant 模型层承接，任务扇出与 artifact 物化由执行器层承接（partial 结论已划界）。

复核方法：20 条原 [x] 逐条对照 Go 基线分支与 Rust 断言。保留的 18 条 [x] 分支相符：legacy 库拒绝且字节不变（:254）、24 并发确认单赢家（:295）、无遗留消息表（:373）、密钥不回显且入 sidecar（:385）、超时默认与钳制（:437）、存储幂等三段（:452 同族已验）、run 分页（:486 同族已验）、重复批准单续跑（:527 同族已验）、重启恢复（:621 同族已验）、孤立回收（:745 同族已验）、双审批门禁（:788 同族已验）、任务与记忆归一化（:844/:899 同族已验）、记忆注入开关（:939 同族已验）、拒绝摘要与审计（:670/:720）、 deadline 与超时冻结（:862/:881）、目标续跑新窗口（:898）、过期回收两则（:947/:1005）、目录投影数组形状（:25）、任务与低风险豁免谓词（:87/:106/:129）。16 条 partial 的缺口描述与 Go 原文相符（任务扇出、artifact 物化、schema 严格性、网络与安全边界等均无逐条断言或属已登记结构差异）。

结论：1 处 verdict 纠正（过宽降级）+ 1 处组合修正 + 1 处结论收紧 + 1 处锚点行号纠正。其一是 `tools_test.go:44` 由 [x] 降为 partial：原条目引用的审查测试只覆盖 workflow.wait 与 http.fetch 的 schema，与 tasks.create/update 的 10 个 planner 字段无关（结论误系他证），条目已改指 mutation 往返测试；其二是 `store_test.go:471` 保持 [x] 但由单条目扩展为四段组合（gated 投影、tool loop 执行一次、审批唤醒 continuation、续跑终局），原单条目只覆盖第一段；其三是 `tools_test.go:61` 保持 [x]，结论收紧（Permission/Risk/免审批三分支在 Rust 无同形断言的结构性说明）；其四是 `tools_test.go:129` 的模型层锚点行号误写 :100，已纠正为 :129（:100 行在 Go 侧根本不是测试起始行）。组合条目在 [x] 行间唯一。

新增证据（注释 only）：15 行规范锚点（:385/:437 归 provider mutation 测试，:471 三处分段锚点，:106 归 catalog policy 测试，:670/:720 归 terminal audit 测试，:862/:881 归 lifecycle 测试，:25/:898 归 ports 测试，:61 归 MCP protocol 测试，:947/:1005 归 expiry 测试）加 1 行锚点纠正，无生产实现改动，无探针。

映射终值（35 行）：`[x]` 19、partial 16、boundary 0。全量：`[x]` 1577、partial 2237、boundary 637（合计 4451）；Rust 测试 3293 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 20 条原 [x] 逐分支核对、15 条 partial 结论抽查 | 1 verdict 纠正（:44）加 1 组合修正（:471）加 1 结论收紧（:61）加 1 锚点纠正（:129），15 锚点补齐 |
| 账本写入 | 3 行变更（b82_apply），其余不动 | `[x]` 1578→1577、partial 2236→2237、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 144→131；unknown 行 50→49；Rust 测试 3293 不变；重复 `[x]` 唯一性检查通过 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1691→1702、已记录 1641→1653、unrecorded 0、stale 0、unknown 49 |
| 静态与格式 | `cargo fmt --all -- --check`、`pnpm run check:clippy`、`check:rust:architecture` | fmt exit 0、clippy exit 0、architecture 通过；改动仅注释锚点加账本结论（含 1 行锚点行号纠正） |
| 整轮 nextest | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked --no-fail-fast` 及 assistant/sqlite 受影响定向 | engine 整轮首轮 1910/1911（launcher 已知抖动，见下），assistant 41/41 exit 0、sqlite 整轮 188/188 exit 0 |
| 兼容与门禁 | `pnpm run check:compatibility`、`check:generated`、`check:ai-context`、`check:zero-go`、`pnpm run check:quick` | compat/generated/ai-context/zero-go 均 exit 0；quick exit 0（rust 1982/1982、pineworker 98/98；engine 整轮抖动经隔离复跑 1/1 后由本轮 quick 整轮确认，launcher 两例均过） |
| 已知失败（如实记录） | `pnpm run check:rust:static`、`pnpm run check:rust:policy` | static/policy 均 exit 1，失败原因与预期一致（deny advisory-not-detected），与本分片无关 |

后续：engine 第十三片完成（626 行中 455 行）；队列进入 engine 第十四片（账本 1532 起约 35 行），随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十三：`internal/assistant/engine` 第十四片 35 行（1 处过宽降级 + 1 条目纠正转组合 + 1 测试断言补强 + 12 锚点，无新增实现）

范围（账本 inventory 1532-1566，按账本顺序）：`store_test.go:947/:1005`、`task_runner_test.go:13/:79`、`taskset_biz_test.go:9`、`timeline_projection_helpers_test.go:11`、`tool_artifact_materialization_test.go:24/:52/:73`、`tool_registry_change_test.go:8`、`tool_schema_workflow_test.go:10/:35`、`tools_net_transport_boundaries_test.go:12`、`tools_security_test.go:11/:33`、`tools_test.go:25/:44/:61/:87/:106/:129/:145/:163/:195/:206/:219/:275/:315/:342/:468/:605/:700/:726/:743/:775`。初值 `[x]` 21、partial/boundary 14。

owner：过期回收与超时窗口由运行时 reconcile 承接，任务扇出缺口保留在调度边界，artifact 物化缺口保留在存储语义边界，工具 schema 与豁免谓词由 MCP 目录与 assistant 模型层承接，执行有界性与失败投影由运行时工具循环承接。

复核方法：21 条原 [x] 逐条对照 Go 基线分支与 Rust 断言，14 条 partial/boundary 抽查 Go 原文与缺口描述。保留的 19 条 [x] 分支相符：run 粒度超时窗口（:1005）、过期 run 终局字段群（:947）、目录空审批数组序列化（:25）、tasks 写豁免按名生效（:87）、memory 与 draft 写豁免（:106 组合）、高中风险审批门（:129）、research_backtest 显式跳过（:145）、workflow.wait 等待与免审批（:163）、25 秒上限（:195）、取消中断（:206）、多形态时长解析（:219）、http.fetch 非法目标拒绝（:275）、主机地址分类（:315）、响应封装与截断（:342）、有界执行不挂起（:468）、慢 portfolio 不阻塞（:605）、live_trading 全模式门控（:700 组合）、kline 伴随工具（:726）、显式访问模式投影（:743）。

结论：1 处 verdict 纠正（过宽降级）+ 1 处条目纠正转组合 + 1 处测试断言补强 + 1 处陈旧锚点纠正 + 12 行规范锚点。其一是 `tools_test.go:61` 由 [x] 降为 partial：Go 断言 models.list 的 Permission 为 read_internal、RiskLevel 为 low、approval 模式免审批三分支，Rust 的 schema 审查测试只覆盖字段面与无 key 泄露，Rust 无逐工具权限元数据，免审批分支无同形断言。其二是 `tools_test.go:775` 条目纠正：原条目误指生产文件且单条目只覆盖落库投影，改为落库投影测试加终局降级测试的双条目组合（COMPLETED、degraded、FAILED 可见调用、disk 全文、非空回复全覆盖），与 runner_chat:423 的单条目引用全文不同。其三是 `workflow_wait_duration` 空串分支由仅判错补为 greater than 0 文本断言。其四是 model.rs 的 `permission_classes` 锚点纠正：原锚点所指 `tools_test.go:16 TestApprovalRequiresLiveTradingAndStrategyInstanceAlways` 在基线中不存在，改为 `tools_test.go:145`（显式跳过分支的真实覆盖）。

新增证据（注释与单断言 only）：12 行规范锚点（mcp_server 7 处 :163/:195/:206/:219/:275/:315/:342，catalog policy :700，gate :726/:743，failure :775 双处），model.rs 锚点纠正 1 处，duration 空串文本断言 1 行。

映射终值（35 行）：`[x]` 20、partial 10、boundary 5。全量：`[x]` 1576、partial 2238、boundary 637（合计 4451）；Rust 测试 3293 不变。

门禁说明：本分片落在 `beb5174c` 门禁优化之后。账本已迁入 v2 信封（schemaVersion jftrade.go-rust-parity-mappings.v2），审计默认只写临时目录，需 `--write-report` 刷新跟踪报告；旧 `/tmp/b75_writer.py` 只懂扁平结构，本批改用按升级脚本同源逻辑重算 rustEvidence 与 reuse 的 v2 写入器。`--strict` 要求全部 function_exact 行具备已评审断言与 passed 回执，全仓当前仍是 legacy-conclusion 加 unverified，本分片保持仓内既有约定，不单立新制式。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 21 条原 [x] 逐分支核对、14 条 partial/boundary 抽查 | 1 verdict 纠正（:61）加 1 组合修正（:775）加 1 断言补强（:219）加 1 锚点纠正（model :145），12 锚点补齐 |
| 账本写入 | 3 行变更（v2 写入器），其余不动 | `[x]` 1577→1576、partial 2237→2238、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 131→120；Rust 测试 3293 不变；重复 `[x]` 唯一性检查通 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1702→1712、已记录 1653→1664、unrecorded 0、stale 0、unknown 49→48 |
| 报告刷新 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | exit 0；report 基线刷为 beb5174c，inventory 跟随 3 行变更（:61 降级、:775 条目纠正、计数 1576/2238/637） |
| 静态与格式 | `cargo fmt --all -- --check`（exit 0）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed） | fmt/clippy/arch 全过，见 /tmp/s23_fmt.log、s23_clippy.log、s23_arch.log |
| 受影响 nextest | engine 定向 20 表达式（初轮编译锚点错误修复后 20/20）、assistant 41/41 | engine 定向 20/20（/tmp/s23_run5.log），assistant 41/41（/tmp/s23_assistant.log）；中间态 E0277 已修复，不隐瞒 |
| 整轮 nextest | engine 全轮首轮 1910/1911（`runtime_exit_converges` 策略时序抖动，隔离复跑 2/2 通过）、第二轮 1911/1911 | /tmp/s23_engine_full.log、s23_engine_full2.log、s23_isolate2.log |
| 兼容与门禁 | compat 7 项全过、generated（--check 不改工作树）过、ai-context 过、zero-go 过、migration-manifest 过；quick 前两轮分别倒在 node 并行抖动（隔离 1/1 过）与 target-health（清 35GB 伪影后重跑），第三轮（/tmp/s23_quick3.log）中途随会话结束而中断后，以完整前台重跑 /tmp/s23_quick4.log 收官：rust 1982/1982、compat 7 项、pineworker 98/98、desktop 48/48，QUICK4_EXIT=0 | /tmp/s23_quick4.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 本次均为 exit 0（advisories/bans/licenses/sources ok，与既往 advisory-not-detected 失败预期不同） | /tmp/s23_static2.log、s23_policy2.log；失败项不记为通过 |

后续：engine 第十四片完成（626 行中 490 行）；队列进入 engine 第十五片（账本 1567 起约 35 行：tools:837、usageprojection、workflow 族），随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十四：`internal/assistant/engine` 第十五片 35 行（5 处过宽降级 + 2 新增用例 + 1 断言补强 + 5 锚点，无生产实现改动）

范围（账本 inventory 1567-1601，按账本顺序）：`tools_test.go:837`、`usageprojection/projection_test.go:10/:27/:41`、`workflow_agent_native_integration_test.go:22/:75/:122/:162`、`workflow_agent_runtime_branches_test.go:53`、`workflow_agent_test.go:21/:47/:81/:99/:118/:136/:176/:253/:274`、`workflow_approval_recovery_boundaries_test.go:8`、`workflow_approval_test.go:9`、`workflow_canvas_test.go:11/:60/:133/:167/:223`、`workflow_child_test.go:8/:79/:137`、`workflow_compiler_test.go:25/:51/:77/:97`、`workflow_execution_persistence_test.go:7`、`workflow_finalization_contracts_test.go:7`、`workflow_goal_test.go:13`。初值 `[x]` 10、partial 25。

owner：用量投影缺口保留在运行负载持久化语义，ADK 原生 agent 形态缺口保留在接口差异（无 google-adk 对象迁移），审批恢复与父子调度缺口保留在运行时 reconcile 与调度边界，画布编译形状缺口待补断言。

复核方法：10 条原 [x] 逐条对照 Go 基线分支与 Rust 断言体，25 条 partial 抽查 Go 原文与缺口描述（含高风险 approval_recovery :8 的暂停耐久与恢复上下文分支）。保留的 5 条 [x] 分支相符：流式 30 秒有界（:837）、扇入汇合顺序加前驱锁定（compiler :25，见下）、线性链默认顺序依赖（compiler :51）、未知依赖 400 拒绝（compiler :97）、模型后端不可用失败关闭加终局审计（canvas :223）。

结论：5 处 verdict 纠正（过宽降级）+ 2 新增用例 + 1 断言补强 + 5 锚点。其一是 `canvas:11` 由 [x] 降为 partial：Go 断言编译产物形状（3 步骤、元数据、回退消息、扇入依赖与覆盖、指纹回退），Rust 多节点执行用例只覆盖执行与上下文传播，条目改为执行加菱形汇合双条目组合。其二是 `canvas:133` 由 [x] 降为 partial：Go 端到端断言运行 COMPLETED、DONE 计划、子运行派生与非空回复，原条目引用的编译顺序与错误跳过用例不断言成功执行，条目改指多节点执行用例并补 :133 锚点。其三是 `compiler:77` 由 [x] 降为 partial：原条目引用的非法输入用例不断言依赖去重；实现侧重复边已去重，本批新增去重用例锁定，空白依赖忽略无对应实现（画布边要求有效端点，属契约差异）。其四是 `canvas:60` 由 [x] 降为 partial：8 分支中缺画布分支无等价拒绝（Rust 对无画布工作流走 legacy 合成执行，属实现选择差异），其余 7 分支由 6 用例组合覆盖，本批新增空画布用例并补 4 处 :60 锚点。其五是 `canvas:167` 由 [x] 降为 partial：Go 的子运行派生、BLOCKED 计划、输入标识透传与图漂移守卫四分支无等价断言，挂起恢复与幂等键复用已覆盖。其六是 compiler :25 菱形用例补汇合前驱断言。其余 20 条 partial 结论抽查相符（引用存在不等于断言等价口径保持）。

新增证据：2 新增用例（重复边去重、空画布无执行节点）加 1 汇合前驱断言，均在 `crates/jftrade-assistant/src/workflow_canvas.rs`；5 规范锚点（:133 归多节点执行用例，:60 归无执行节点加不可达代理加非法输入加空画布用例，:77 归去重用例）；`adk_workflow_canvas_contracts.rs` 加 :133 锚点 1 处。无生产实现改动。

映射终值（35 行）：`[x]` 5、partial 30、boundary 0。全量：`[x]` 1571、partial 2243、boundary 637（合计 4451）；Rust 测试 3293 加 2（本批新增）。

门禁说明：本分片落在 `beb5174c` 门禁优化之后，账本 v2 信封，写入用 v2 写入器；`--strict` 全仓未达标，保持既有约定不单立制式。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 10 条原 [x] 逐分支核对、25 条 partial 抽查 | 5 verdict 纠正（:11、:133、:77、:60、:167）加 2 新增用例加 1 断言补强加 5 锚点 |
| 账本写入 | 5 行变更（v2 写入器），其余不动 | `[x]` 1576→1571、partial 2238→2243、boundary 637 不变（合计 4451）；全部引用测试存在且有锚点 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变（新增锚点归 partial 行）；重复 `[x]` 唯一性检查通 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1712 不变（新增锚点指向已记录引用）、已记录 1664、unrecorded 0、stale 0、unknown 48 |
| 报告刷新 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | exit 0；inventory 跟随 5 行变更 |
| 静态与格式 | `cargo fmt --all -- --check`（FMT_OK）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed） | fmt/clippy/arch 全过，见 /tmp/s24_clippy2.log、s24_arch.log |
| 受影响 nextest | assistant canvas 定向 10/10、engine 加 assistant 双 crate canvas 定向 16/16（含 2 新增用例与菱形汇合前驱补强） | /tmp/s24_canvas_a.log、s24_canvas_ea.log |
| 整轮 nextest | engine 全轮 1911/1911 一次过，无抖动 | /tmp/s24_engine_full.log |
| 兼容与门禁 | compat 7 项全过、generated（--check 不改工作树）过、ai-context 过、zero-go 过；quick exit 0（rust 1984/1984、pineworker 98/98、desktop 48/48，一次过无抖动） | /tmp/s24_quick.log、s24_gen.log、s24_aictx.log、s24_zerogo.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 本次均为 exit 0（advisories/bans/licenses/sources ok）；失败项不记为通过 | /tmp/s24_static.log、s24_policy.log |

后续：engine 第十五片完成（626 行中 525 行）；队列进入 engine 余量约 101 行，随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十五：`internal/assistant/engine` 第十六片 35 行（5 处过宽降级，无新增实现）

范围（账本 inventory 1602-1636，按账本顺序）：`workflow_goal_test.go:52/:81/:119/:222/:282/:342/:389/:457`、`workflow_helpers_provider_failures_test.go:10`、`workflow_observation_projection_test.go:11/:62`、`workflow_persistence_test.go:9`、`workflow_plan_boundaries_test.go:9`、`workflow_planner_runtime_test.go:12`、`workflow_reconcile_test.go:8/:49/:73/:128`、`workflow_resume_test.go:9/:65`、`workflow_store_boundaries_test.go:24`、`workflow_store_test.go:9`、`workflow_tools_test.go:28/:140/:206/:222/:230/:263/:285/:438/:474/:488/:559`、`workflowexec/executor_integration_test.go:56`、`workflowexec/goal_resume_failure_boundaries_test.go:26`。初值 `[x]` 7、partial 28。

owner：目标暂停恢复缺口保留在暂停字段 CAS 与 pause/resume mutation，审批父子续跑缺口保留在决议暂存与运行时编排边界，工具错误包络已对齐，artifact 与 memory 工具面缺口保留在接口差异。

复核方法：7 条原 [x] 逐条对照 Go 基线分支与 Rust 断言体，28 条 partial/boundary 抽查 Go 原文与缺口描述。保留的 2 条 [x] 分支相符：结构化工具错误助手函数（tools :230，空映射与成功不判失败、失败默认文案、trim、legacy 分支、nil 字面量、标量包装逐条对应）与错误包络重试分类（tools :263，超时可重试、取消不可重试、结构化元数据保留逐条对应）。

结论：5 处 verdict 纠正（过宽降级）。其一是 `goal:119` 由 [x] 降为 partial：Rust 装配用例只覆盖暂停请求与恢复的字段迁移，PAUSED 终态形状、恢复后完成、恢复提醒与执行离场四分支无等价断言。其二是 `goal:222` 由 [x] 降为 partial：引用用例的陈旧写入者写的是 RUNNING 快照，没有完成与暂停竞态的优先断言，CompletedAt 为空与 complete 工具剪枝两分支无等价断言。其三是 `goal:342` 由 [x] 降为 partial：引用用例只覆盖 CANCELLED 与 COMPLETED 终态单调性，未覆盖 PAUSED；Rust 没有活动快照合并路径，暂停字段保留与工具调用合并无等价实现与断言。其四是 `reconcile:8` 由 [x] 降为 partial：原条目引用的用例只覆盖拒绝路径（与批准分支相反），条目扩展为拒绝加批准暂存双条目组合；父完成回填与执行一次两分支在运行时无等价断言。其五是 `reconcile:73` 由 [x] 降为 partial：计划步 BLOCKED、工作流暂停态、子向父同步方向三分支无等价断言，重开与审批镜像及重放围栏已覆盖。其余 28 条 partial/boundary 结论抽查相符（工具面三处边界均带无命中探针）。

新增证据：无新增用例与生产改动；`reconcile:8` 条目由单引用扩展为双条目组合。

映射终值（35 行）：`[x]` 2、partial 33、boundary 0。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

门禁说明：本分片落在 `beb5174c` 门禁优化之后，账本 v2 信封，写入用 v2 写入器；`--strict` 全仓未达标，保持既有约定不单立制式。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 7 条原 [x] 逐分支核对、28 条 partial/boundary 抽查 | 5 verdict 纠正（:119、:222、:342、reconcile :8、reconcile :73），2 条保留（tools :230、:263） |
| 账本写入 | 5 行变更（v2 写入器），其余不动 | `[x]` 1571→1566、partial 2243→2248、boundary 637 不变（合计 4451）；全部引用测试存在且有锚点 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变；重复 `[x]` 唯一性检查通 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1712、已记录 1664、unrecorded 0、stale 0、unknown 48 |
| 报告刷新 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | exit 0；inventory 跟随 5 行变更 |
| 静态与格式 | `cargo fmt --all -- --check`（FMT_OK，本片无 Rust 改动）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed） | /tmp/s25_clippy.log |
| 受影响 nextest | sqlite 引用 6 用例定向 6/6（含批准暂存双路径、暂停字段、终态单调、重开围栏） | /tmp/s25_sqlite.log |
| 整轮 nextest | engine 全轮 1911/1911 一次过，无抖动 | /tmp/s25_engine_full.log |
| 兼容与门禁 | generated/ai-context/zero-go 均过；quick 为影响域裁剪（本片仅账本与文档，无 Rust 改动，planner 只跑 policy 9 项）exit 0；engine 整轮已显式全过 | /tmp/s25_quick.log、s25_gen.log、s25_aictx.log、s25_zerogo.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 本次均为 exit 0；失败项不记为通过 | /tmp/s25_static.log、s25_policy.log |

后续：engine 第十六片完成（626 行中 560 行）；队列进入 engine 余量约 66 行，随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十六：`internal/assistant/engine` 第十七片 35 行（2 处引用纠正 + 2 锚点，无 verdict 变更）

范围（账本 inventory 1637-1671，按账本顺序）：`workflowexec/goal_resume_failure_boundaries_test.go:86`、`goal_state_boundaries_test.go:16/:158`、`goal_turn_failure_boundaries_test.go:11/:84/:115`、`persistence_failure_boundaries_test.go:14/:111/:217`、`persistence_propagation_closeout_test.go:14/:51/:124/:165/:179/:193`、`taskset_biz_test.go:9`、`taskset_done_test.go:9/:107`、`workflow_approval_persistence_boundaries_test.go:12`、`workflow_approval_recovery_boundaries_test.go:11/:28`、`workflow_child_failure_persistence_test.go:10`、`workflow_child_finalization_boundaries_test.go:10/:66`、`workflow_child_lifecycle_test.go:13`、`workflow_execution_failure_boundaries_test.go:13/:73/:108`、`workflow_execution_persistence_test.go:14/:123`、`workflow_executor_boundary_branches_test.go:11/:85/:121`、`workflow_finalization_contracts_test.go:10/:65`。初值 `[x]` 0、partial 35。

owner：workflowexec 编排层在 Rust 不存在（无 WorkflowExecutor、goal turn、finalize、子运行结清编排），缺口保留在运行负载持久化、暂停字段 CAS、端口 fail-closed 与调度边界三处 owner。

复核方法：35 条 partial 全量扫描引用有效性（文件加函数须指向真实 `#[test]`），逐条抽查 Go 原文与缺口描述；重点找引用错位与可升级项。结论：33 条引用有效且缺口描述相符（工具面与 artifact 面边界均带无命中探针），2 处引用纠正，无升级项（最接近的 modelsList 两条缺工作流任务工具集包装层，仍为 partial）。

纠正：其一是 `child_finalization:66` 条目引用生产函数 `is_dormant_workflow_child_run` 而非测试，改为过期自有超时窗口用例并重写结论（dormant 判定实现侧存在但无独立测试覆盖，回归要求补豁免断言）。其二是 `execution_persistence:14` 条目引用生产辅助函数 `synthetic_assistant_message_id` 而非测试，改为完成运行 transcript 链接用例并重写结论（最终消息标识三面链接已覆盖，finalize 与任务保存编排缺失）。两处 verdict 保持 boundary 不变，另补 2 处 :66/:14 锚点。

新增证据：注释锚点 2 行，无新增用例，无生产实现改动。

映射终值（35 行）：`[x]` 0、partial 35、boundary 0。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

门禁说明：本分片落在 `beb5174c` 门禁优化之后，账本 v2 信封，写入用 v2 写入器；`--strict` 全仓未达标，保持既有约定不单立制式。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条 partial 引用有效性全扫描加缺口抽查 | 2 引用纠正（:66、:14）加 2 锚点，无升级项 |
| 账本写入 | 2 行变更（v2 写入器），其余不动 | `[x]` 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451）；引用测试全部存在且有锚点 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变；重复 `[x]` 唯一性检查通 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1712→1714、已记录 1664→1666、unrecorded 0、stale 0、unknown 48 |
| 报告刷新 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | exit 0；inventory 跟随 2 行变更 |
| 静态与格式 | `cargo fmt --all -- --check`（FMT_OK，改动仅注释锚点）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed） | /tmp/s26_clippy2.log |
| 受影响 nextest | engine 引用 2 用例定向 2/2（过期自有窗口、transcript 链接） | /tmp/s26_targeted.log |
| 整轮 nextest | engine 全轮 1911/1911 一次过，无抖动 | /tmp/s26_engine_full.log |
| 兼容与门禁 | generated/ai-context/zero-go 均过；quick 完整计划 exit 0（rust 1941/1941、compat 7 项、pineworker 98/98） | /tmp/s26_quick.log、s26_gen.log、s26_aictx.log、s26_zerogo.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 本次均为 exit 0；失败项不记为通过 | /tmp/s26_static.log、s26_policy.log |

后续：engine 第十七片完成（626 行中 595 行）；队列进入 engine 余量约 31 行，随后 apiserver 下 assistant/ADK 相关 14 行。

### 第 129 批分片二十七：engine 尾片加 model 35 行（8 处引用纠正 + 8 锚点，无 verdict 变更）

范围（账本 rows 1651-1685，按账本顺序）：`workflowexec/goal_pause_boundaries_test.go:11/:55`、`goal_terminal_helpers_test.go:12`、`workflow_helpers_test.go:11`、`workflow_pending_input_contract_test.go:10`、`workflow_persistence_test.go:12/:35/:67/:96`、`workflow_reconcile_executor_boundaries_test.go:9/:69/:110/:164`、`workflow_reconcile_ignore_boundaries_test.go:9`、`workflow_resume_approval_boundaries_test.go:13`、`workflow_resume_executor_boundaries_test.go:10/:35/:106/:173`、`workflow_task_limit_boundaries_test.go:11`、`workflow_task_state_contracts_test.go:11/:49`、`workflow_task_tools_boundaries_test.go:13/:69`、`workflow_task_tools_goal_test.go:7`、`workflow_task_tools_lookup_test.go:10/:135`、`workflow_task_tools_persistence_test.go:13/:112`、`workflowruntime/runtime_test.go:11/:48`、`model/provider_reasoning_config_test.go:8/:27/:65`、`model/timeline_helper_test.go:5`。初值 `[x]` 0、partial 35。（注：交接记录的 1672-1706 为旧行号，账本按 Go 路径字母序排列，本片实际为 1651-1685；内容一致，均为 35 行。）

owner：workflowexec 编排层在 Rust 不存在（无 WorkflowExecutor、goal turn、迭代上限暂停、子运行 dormant 豁免），缺口保留在过期自有超时窗口、端口 fail-closed 与任务工具集边界三处 owner。

复核方法：35 条全量枚举 rustEvidence（文件加函数须指向真实 `#[test]`，生产函数引用一律视为错位），逐条抽查 Go 原文与缺口描述；重点找引用错位与可升级项。结论：27 条引用有效且缺口描述相符，8 处引用纠正，无升级项。抽查确认：`task_limit:11`、`task_tools_boundaries:13`、`task_tools_goal:7` 的“不适用”条目是刻意保留的 prose-only boundary（Rust 无对应工具/上限），非坏引用；`provider_reasoning:8/:27` 与 `resume_executor:35` 所指符号经查均为真实 `#[test]`（生产文件内单元测试），审计 stale 检查全仓仅 2 处且均在本片之外（marketdataapp sidecar 两条）。

纠正：`helpers:11` 原引用 `TaskGraph::new` 构造器，改指图排序与故障用例；`persistence:96` 原引用生产校验函数 `validate_loop_iterations`，改指 agent 写入校验用例；`reconcile:69/:110` 与 `ignore:9` 三条原引用 `is_dormant_workflow_child_run`（dormant 判定实现存在但无独立测试），改指过期自有超时窗口用例；`lookup:135` 原引用 `TaskGraph::complete`，改指单 deterministic ready 任务用例；`persistence:13` 原引用 `dispatch`（非测试），改指端口 durable 失败面用例；`timeline:5` 移除第二引用生产函数 `merge_session_timeline`，仅保留工具轮投影用例。8 处 verdict 保持 boundary/partial 不变，另补 8 处锚点（workflow.rs 用 `//` 以贴合该文件零 `///` 惯例，其余沿用 `/// Parity:`）。

新增证据：注释锚点 8 行，无新增用例，无生产实现改动。

映射终值（35 行）：`[x]` 0、partial 35、boundary 0。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

门禁说明：本分片落在 `beb5174c` 门禁优化之后——审计默认只写临时目录，需 `--write-report` 落盘；基线漂移检查通过；`--strict` 全仓未达标（4360 处 function_exact 证据缺口，既有约定不单立制式）；`check:migration-manifest` 新增通过。账本 v2 信封，写入用 v2 写入器；写入后重跑回填锚点。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查 | 8 引用纠正加 8 锚点，无升级项 |
| 账本写入 | 8 行变更（v2 写入器重跑回填锚点），其余不动 | `[x]` 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451）；引用测试全部存在且有锚点 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | 通过（exit 0）；基线漂移检查过；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变；重复 `[x]` 唯一性检查通 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1714→1722、已记录 1666→1674、unrecorded 0、stale 0、unknown 48 |
| 静态与格式 | `cargo fmt --all -- --check`（过，改动仅注释锚点）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed）、`check:migration-manifest`（passed） | /tmp/s27_clippy.log |
| 受影响 nextest | assistant 引用 2 用例定向 2/2；engine 引用 4 用例定向 4/4 | /tmp/s27_t1.log、/tmp/s27_t2.log |
| 整轮 nextest | engine 全轮一次过，无抖动 | engine 全轮第一轮 1859 passed/1 failed（launcher address_taken 端口竞争抖动）+51 未跑（fail-fast 取消）；launcher 双用例隔离复跑 2/2；第二轮 --no-fail-fast 1910/1911（另一 launcher 用例抖动，首轮失败项通过）。两轮失败均为 launcher 端口竞争抖动，与本片注释改动无关 | /tmp/s27_engine_full.log、/tmp/s27_flaky1.log、/tmp/s27_engine_full2.log |
| 兼容与门禁 | generated/ai-context/zero-go/migration-manifest 均过；quick 完整计划 exit 0 | quick 完整计划 exit 0（rust 1984/1984 含两 launcher 用例、compat 7 项、pineworker 98/98） | /tmp/s27_quick.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 结果见日志；`--strict` 全仓 4360 缺口未达标（非本分片阻塞）；失败项不记为通过 | static exit 0、policy exit 0 | /tmp/s27_static.log、/tmp/s27_policy.log |

后续：engine 收口（626 行完成）；队列进入 apiserver 下 assistant/ADK 相关 14 行，随后 other 503、api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片二十八：apiserver 下 assistant/ADK 相关 14 行（2 处引用纠正 + 2 锚点，无 verdict 变更）

范围（账本 rows 338-340、390、453-456、619-620、636-638、835）：`application/assistant_test.go:45/:75/:98`、`datamigration/maintenance_test.go:255`、`marketdataapp/assistant_provider_test.go:95/:114/:143/:159`、`runtimes/handle_lifecycle_test.go:573/:592`、`servercore/adk_data_management_test.go:15/:60`、`servercore/assistant_transport_lifecycle_test.go:11`、`servercoretest/installers_degraded_test.go:13`。初值 `[x]` 4、partial 7、boundary 3。

owner：apiserver 装配面在 Rust 由组合根与领域 crate 持有——Assistant 端口投影与运行时打开归 engine 装配测试，ADK 清理与压缩归 store-sqlite 契约，provider 选择归 settings/marketdata，关闭与降级归 engine runtime。

复核方法：14 条全量枚举 rustEvidence（文件加函数须指向真实 `#[test]`，生产函数与裸生产文件引用一律视为错位），逐条抽查 Go 原文与缺口描述；4 条 `[x]` 核对断言等价与锚点归属（338 与 340、390 与 636 为组合与单引用形式不同，全文唯一，无重复 `[x]`）。结论：12 条引用有效且缺口描述相符，2 处引用纠正，无升级项。抽查确认：339 的裸文件引用是刻意的 prose-only partial（read_helpers 无独立测试，路径派生只经 fixture 间接覆盖，结论已写清所有权差异）；340 的 `[x]` 维持（装配测试从 SQLite 打开生产端口并投影服务，go:98 锚点在位，不做翻转）；453/454/456/637/638/835 的 partial 缺口（无 MCP 工具面、scope 编码进路由、无 unknown 信封、无逐库 busy 注册表、无 keep-alive 关闭窗口、无降级启动）均有 owner 与回归要求。

纠正：619 原引用端口投影用例（非关闭面），改指有序关闭用例并重写结论（Rust 无 Handle/SetAssistant 注入路径，迟到注入在类型系统层面不存在）；620 原引用裸生产文件 `product_runtime.rs`（非测试），改指同一关闭用例并重写结论（无并发发布竞争面，关闭幂等级由有序关闭用例锁定）。两处 verdict 保持 boundary 不变，另补 2 处 :573/:592 锚点（沿用该文件 `// Parity:` 惯例，与同文件 :475 锚点相邻）。

新增证据：注释锚点 2 行，无新增用例，无生产实现改动。

映射终值（14 行）：`[x]` 4、partial 7、boundary 3。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 14 条全量枚举引用有效性加缺口抽查，4 条 `[x]` 核对断言等价 | 2 引用纠正加 2 锚点，无升级项；重复 `[x]` 全文唯一性检查通（0 重复） |
| 账本写入 | 2 行变更（v2 写入器重跑回填锚点），其余不动 | `[x]` 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451）；引用测试全部存在且有锚点 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1722→1724、已记录 1674→1676、unrecorded 0、stale 0、unknown 48 |
| 静态与格式 | `cargo fmt --all -- --check`（过，改动仅注释锚点）、`pnpm run check:clippy`、`check:rust:architecture`、`check:migration-manifest` | clippy exit 0、architecture passed、migration-manifest passed | /tmp/s28_clippy.log、/tmp/s28_arch.log、/tmp/s28_mig.log |
| 受影响 nextest | engine 有序关闭用例定向 1/1 | /tmp/s28_t1.log |
| 整轮 nextest | engine 全轮 `--no-fail-fast` | engine 全轮 --no-fail-fast 1910/1911（launcher serves_on_configured_address 端口竞争抖动，另一 launcher 用例通过）；launcher 双用例隔离复跑 2/2 | /tmp/s28_engine_full.log、/tmp/s28_flaky1.log |
| 兼容与门禁 | generated/ai-context/zero-go 均过；quick 完整计划 | quick 首轮在 rust 阶段遇同一 launcher 抖动（fail-fast 取消后续）；重跑完整计划 exit 0（rust 1941/1941 含两 launcher、compat 7 项） | /tmp/s28_quick.log、/tmp/s28_quick2.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 结果见日志；`--strict` 全仓缺口未达标（非本分片阻塞）；失败项不记为通过 | static 首轮因 target-health（.rcgu.o 超 50000，构建残留）失败；确认无 Cargo 进程后 clean（115999 文件/32.8GiB）重跑全绿；policy exit 0 | /tmp/s28_static.log、/tmp/s28_policy.log、/tmp/s28_clean.log |

后续：apiserver 相关 14 行完成；队列进入 other 503、api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片二十九：other 域首片 marketdataassets 家族 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2712-2746）：`marketdataassets/asset_selection_boundaries_test.go:15/:30/:58/:80/:91/:126/:151/:192/:204/:218/:231/:271/:287/:298/:315`、`assets_dev_test.go:14/:34/:44`、`assets_release_test.go:14/:49/:72`、`assets_test.go:8`、`cache_test.go:15/:45/:83/:118/:159/:185/:193/:207/:218/:229/:249/:310/:336`。初值 `[x]` 3、partial 24、boundary 8。

owner：marketdata sidecar 资产面在 Rust 由 `jftrade-integration-marketdata-helper` 的 AssetBundle（单文件内容寻址）与桌面端资源完整性/打包脚本持有；Go 的 PyInstaller onedir 包模型（多文件遍历、缓存目录、prune、私有目录模式）无同形对象，差异按行保留。

复核方法：35 条全量枚举 rustEvidence（有引用的 22 条逐条确认指向 `asset.rs` 内真实 `#[test]`，非生产函数），逐条抽查 Go 原文与缺口描述；3 条 `[x]`（:204 摘要失配、:218 越界路径、cache:15 缓存复用）核对断言等价；prose-only 的 13 条核对结论诚实度与散文引用存在性（`release_marketdata_helper_path`、`verify_release_resources`、`runtime_plan_rejects_missing_duplicate_and_unsafe_assets`、desktop-release-inputs 命名断言均存在）。结论：35 条引用全部有效、缺口描述相符，无升级项——partial 行的差异（onedir 多文件模型、可用性三元组 vs 错误、symlink/并发赢家/prune 概念缺失）均为真实结构差异；3 条 `[x]` 按约定不写代码锚点（check-zero-go 禁止退休包路径字样），证据在 `marketdata-assets-batch-scope.md`。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：`[x]` 3、partial 24、boundary 8。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，3 条 `[x]` 核对断言等价 | 0 纠正、0 升级；重复 `[x]` 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | `[x]` 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | 通过（exit 0）；report 仅刷新 Rust 基线到 `bb4040b7`，inventory 无变化 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 受影响 nextest | helper asset 5 用例定向 5/5 | /tmp/s29_t1.log |
| 文档门禁 | `check:ai-context`、quick 完整计划 | ai-context 过；quick 文档计划 exit 0（policy 9 项并行全过） | /tmp/s29_aictx.log、/tmp/s29_quick.log |

后续：other 域首片完成（余 532 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片三十：other 域 market/calendar 家族 35 行（1 处引用纠正，无 verdict 变更）

范围（账本 rows 2747-2781）：`marketdataassets/cache_test.go:407` 收尾 1 行、`pkg/market/calendar/builtin_test.go:8/:38/:54/:73/:92`、`calendar_boundaries_test.go:8/:34`、`helpers_boundaries_test.go:8/:52/:87/:136/:167`、`types_json_test.go:10`、`pkg/market/hk/hk_test.go:8/:20`、`instrument_session_validation_test.go:8/:31`、`market_normalization_test.go:32/:83/:108/:140/:161/:191/:248/:264/:302/:330/:382`、`market_test.go:11/:38/:51/:78/:115/:142`。初值 `[x]` 1、partial 32、boundary 2。

owner：交易日历面在 Rust 由 `jftrade-calendar`（manager_policy/manager_session/snapshot/manager_calendar 与 manager_boundaries/candle_completion 契约）与 `jftrade-marketdata` 的品种目录持有；节假日/提前收盘/交易时段判定无双写。

复核方法：35 条全量枚举 rustEvidence（文件加函数须指向真实 `#[test]`，生产函数引用一律视为错位），逐条抽查 Go 原文与缺口描述；1 条 `[x]`（types_json:10）核对断言等价与锚点归属（锚点在 snapshot.rs:429，断言逐项一致）。结论：34 条引用有效且缺口描述相符（时区/DST/节假日/早收/午休/隔夜/自定义窗口/标签 bucket 差异均有 owner 与回归要求），1 处引用纠正，无升级项。

纠正：2762（hk:20 静默回退 UTC）原引用生产函数 `market_local_midnight` 而非测试；Rust 时区加载失败 fail-closed（`TimeZone::get` 失败即返回错误），无静默回退语义，改為不适用 prose 边界保留并重写结论。verdict 保持 boundary 不变，无新增锚点（prose-only 参照 task_limit:11 惯例）。

新增证据：无 Rust 改动、无新增用例、无新增锚点；账本 1 行变更（v2 写入器）。

映射终值（35 行）：`[x]` 1、partial 32、boundary 2。全量：`[x]` 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，1 条 `[x]` 核对断言等价 | 1 引用纠正，无升级项；重复 `[x]` 全文唯一性检查通（0 重复） |
| 账本写入 | 1 行变更（v2 写入器），其余不动 | `[x]` 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451） |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py --write-report` | 通过（exit 0）；0 条引用不存在 crate、0 条 `[x]` 缺 function_exact；缺锚点告警 120 不变 |
| 锚点 | `python3 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 静态与格式 | `cargo fmt --all -- --check`（过，无 Rust 改动）、`pnpm run check:clippy`（exit 0）、`check:rust:architecture`（passed）、`check:migration-manifest`（passed） | /tmp/s30_clippy.log |
| 受影响 nextest | calendar 引用 5 用例定向 5/5（含 `[x]` 快照 JSON 用例） | /tmp/s30_t1.log |
| 兼容与门禁 | generated/ai-context/zero-go 均过；quick 完整计划 | quick exit 0（账本变更工作树精简计划） | /tmp/s30_quick.log |
| 已知失败（如实记录） | `check:rust:static`、`check:rust:policy` 结果见日志；`--strict` 全仓缺口未达标（非本分片阻塞）；失败项不记为通过 | static exit 0、policy exit 0 | /tmp/s30_static.log、/tmp/s30_policy.log |

后续：other 域 market/calendar 片完成（余 497 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片三十一：other 域 market 会话与覆盖工具 35 行（2 处引用纠正，无 verdict 变更）

范围（账本 rows 2782-2816，按写入顺序）：pkg/market/market_test.go:174/:193/:225/:239/:260、session_boundaries_test.go:10/:51/:67/:102、session_calendar_refresh_contract_test.go:36、session_window_test.go:8/:50/:80/:126/:142、sh_test.go:8/:20、sz_test.go:8/:20、us_test.go:8/:17/:56、cmd/check-go-coverage/changed_lines_analysis_test.go:42/:64/:71/:82/:96/:102/:112/:122/:142/:150/:203/:242/:275。初值 [x] 0、partial 21、boundary 14。

owner：交易会话与日历边界在 Rust 由 jftrade-calendar（manager_session/manager_policy/manager_boundaries/candle_completion/fetch_window_timezone）与 jftrade-marketdata（catalog/catalog_tests/cache_extended_sessions）持有；覆盖率工具面 Go 已随运行时删除，等价 owner 为 Node 覆盖门禁脚本，无 Rust 双写。

复核方法：35 条全量枚举引用有效性（条目须指向真实测试函数，生产函数与裸生产文件一律视为错位；[x] 行间 rust_entry 全文唯一），逐条抽查 Go 原文与缺口描述；高风险面为 DST、隔夜 carry、早收、午休、自定义窗口与缺日历 fail-closed。本片 0 条 [x]，无需断言等价升级判断；partial 逐条核对缺口诚实度（引用存在不等于断言等价）。

纠正：2798（sh:20）与 2800（sz:20）与已纠正的 hk:20 同题，原引用均为生产函数 market_local_midnight 而非测试；Rust 时区加载失败 fail-closed，无静默回退语义，改为不适用 prose 边界保留并重写结论。verdict 保持 boundary 不变，无新增锚点。纠正后 market_local_midnight 的 reuse 引用归零，符合生产函数零引用预期。

其余结论：21 条 partial 引用全部指向真实 Rust 测试（manager_session 内模块测试、catalog_tests、calendar tests/marketdata tests 均逐条确认），缺口描述相符，无升级项；14 条 boundary 中 13 条 Go 专用覆盖工具 prose 保留与 1 条改名 partial 例外（changed_lines_analysis:71，Node 脚本等价）维持原判。

新增证据：无 Rust 改动、无新增用例、无新增锚点；账本 2 行变更（v2 写入器）。

映射终值（35 行）：[x] 0、partial 21、boundary 14。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，0 条 [x] | 2 引用纠正，无升级项；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 2 行变更（v2 写入器），其余不动 | [x] 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451） |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；0 条引用不存在 crate、0 条 [x] 缺 function_exact；缺锚点告警 120 不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 静态与格式 | cargo fmt --all -- --check（过，无 Rust 改动）、pnpm run check:clippy（exit 0）、check:rust:architecture（passed）、check:migration-manifest（passed） | /tmp/s31_quick.log、/tmp/s31_static.log、/tmp/s31_policy.log |
| 受影响 nextest | calendar 89/89 全绿、marketdata 61/61 全绿 | 前台直跑 |
| 兼容与门禁 | generated/ai-context/zero-go 均过；quick 完整计划 | quick exit 0 |
| 已知失败（如实记录） | check:rust:static、check:rust:policy 结果见日志；--strict 全仓缺口未达标（非本分片阻塞）；失败项不记为通过 | static exit 0、policy exit 0 |

后续：other 域本片完成（余 462 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。另：对齐审计线程提醒的 runner_chat:423 与 store:792 重复 [x] 本工作树已为 [x]+partial（审计唯一性通过），本次提交不改动该结论；strategy 15.5% 与 backtest 6.3% 仍为关键域缺口，按排期处理。

### 第 129 批分片三十二：other 域覆盖工具 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2817-2851，按写入顺序）：changed_lines_analysis_test.go:313/:326/:355/:372/:380/:393、main_test.go:17/:37/:61/:87/:95/:101/:116/:151/:188、profile_analysis_test.go:14/:46/:59/:85/:109/:120/:137/:173/:179/:202/:219/:238/:274/:291/:313、profile_merge_test.go:13/:30、runner_test.go:46/:53/:82。初值 [x] 0、partial 1、boundary 34。

owner：Go 覆盖率 CLI 工具面已随 Go 运行时删除，Rust 侧无覆盖度分析实现；等价 owner 为 Node 覆盖门禁脚本，无双写。

复核方法：35 条全量枚举引用有效性，逐条抽查 Go 原文与缺口描述；唯一 partial（profile_analysis:109 空业务覆盖 fail-closed）核对 Node 等价与 fail-open 差异登记；34 条 boundary 核对 Go 专用工具诚实度（go test 执行、coverprofile 合并、diff 解析、CLI 配置均无 Rust 同形对象）。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：profile_analysis:109 的 Node 等价 owner 与阈值脚本测试本机 EXIT=0；changed_lines_analysis 家族与 runner 家族抽查 Go 原文均为 go.mod/diff 文本/coverprofile 专用逻辑，boundary 保留成立。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 1、boundary 34。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，0 条 [x] | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线到 d99764d9，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 受影响证据 | node scripts/check-web-diff-thresholds.test.mjs | EXIT=0 |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 | /tmp/s32_quick.log |

后续：other 域本片完成（余 427 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十三：other 域协议生成器 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2852-2886，按写入顺序）：runner:97/:131/:142/:174/:188、generate-futu-proto/generator:18/:46/:60、main:14/:29/:37/:44、manifest:15/:25/:38/:60/:70、repository_verify:12/:41/:59、rewrite:12/:35/:42、generate-pineworker-proto/generator:17/:40/:54/:70/:87、main:14/:25/:33/:40、output:12/:26/:41。初值 [x] 4、partial 8、boundary 23。

owner：生成器流水线（protoc 调用、暂存目录原子替换、CLI 参数与退出码）无 Rust 同形对象，Rust 侧以 build.rs 编译期冻结输入承担等价约束；覆盖率 runner 面已随 Go 运行时删除。

复核方法：35 条全量枚举引用有效性，4 条 [x] 逐分支核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；partial 逐条核对缺口诚实度（命令未执行不可断言、replace/insert 分支不可区分、暂存替换无对应物均如实登记）；boundary 抽查 Go 原文确认无 Rust 同形对象。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：repository_verify 三条 Rust 用例均携带对应行号 Parity 锚点，漂移、非法扩展名、平铺、清单失配与摘要解析失败分支与 Go 一致；pineworker 输入先验 равно Go 的缺失即失败且命令未执行；dup-x 为 0，partial 共享 [x] 引用符合口径。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 4、partial 8、boundary 23。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，4 条 [x] 核对断言等价 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 392 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十四：other 域 protogen 与桌面 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2887-2921，按写入顺序）：protogen files:13/:24/:39/:55、repository:12/:23、tools:15/:65/:74、desktop profile dev:10、release:11、startup:15/:29/:67/:93/:124、updates:9/:33、window_state:12/:22/:46/:60、main:19/:39/:52/:66/:93/:110/:131/:149/:166/:179/:195/:217/:236。初值 [x] 3、partial 21、boundary 11。

owner：仓库根探测由桌面端资源完整性承接；更新通道开关由 profile 与 updater 配置层承接；桌面生命周期、窗口、托盘、资源路由由 Tauri 适配层承接；protoc 文件操作与工具链安装无 Rust 同形对象。

复核方法：35 条全量枚举引用有效性，3 条 [x] 逐分支核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；partial 逐条核对缺口诚实度；boundary 抽查 Go 原文确认无同形对象。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：repository 两条 Rust 用例均携带对应行号 Parity 锚点，嵌套 walk-up、marker 文件校验与缺失 fail-closed 分支与 Go 一致；更新通道 Rust 用例同时断言开发态关闭与发布态开启；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 3、partial 21、boundary 11。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，3 条 [x] 核对断言等价 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 357 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十五：other 域桌面日志与日历 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2922-2956，按写入顺序）：desktop main:261/:271/:317/:335/:375/:395/:426/:464/:506/:523/:533、buildinfo:8、datamanagement maintenance:9/:33/:46、service:44/:71/:101、notification_policy:10/:35、runtime_path_matching:5/:13、runtime_path:8/:41、exchangecalendar http_source_boundaries:30/:89/:156/:179/:229/:278/:298、http_source:21/:40/:55/:88。初值 [x] 16、partial 15、boundary 4。

owner：桌面日志与链接由 Tauri 适配层承接；数据维护由 datamanagement 领域 crate 承接；通知策略由 settings 承接；平台路径由桌面端承接；日历 HTTP 源与解析由 integration-calendar 与 calendar 承接。

复核方法：35 条全量枚举引用有效性，16 条 [x] 逐条核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；partial 逐条核对缺口诚实度；boundary 抽查 Go 原文确认无同形对象。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：通知策略七分支与 Rust 逐条一致；日历六种失败身份中 nil 与非法 URL 在 Rust 不可达但结论如实登记，可达的传输、读取、解析、校验分支逐字透传；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 16、partial 15、boundary 4。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，16 条 [x] 核对断言等价 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 322 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十六：other 域日历管理 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 2957-2991，按写入顺序）：http_source:109/:133/:201/:224/:276/:288/:320/:351/:378/:390、manager_boundaries:14/:50/:119/:146/:172/:216/:248/:287/:333、manager_probe:14、manager_runtime:17/:86/:125/:142、manager:33/:50/:64/:119/:207/:235/:291/:346/:393/:444/:488。初值 [x] 35、partial 0、boundary 0。

owner：日历解析与管理器由 integration-calendar 与 calendar 承接，无双写。

复核方法：35 条全量枚举引用有效性，逐条核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；组合引用核对两文件组合与既有行不同。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：快照校验条件表在 validator 旁逐项 pin，恢复路径可观测效应在 manager_boundaries 侧 pin，措辞差异如实登记；SSE 跨年、补班排除、解析器分支与管理器回退、重试、降级分支均逐项一致；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 35、partial 0、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 受影响 nextest | integration-calendar 23/23 全绿 | 前台直跑 |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 287 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十七：other 域日历与前端资源 35 行（1 处结论补强，无 verdict 变更）

范围（账本 rows 2992-3026，按写入顺序）：manager:543/:613/:670/:736/:751/:779/:811/:876/:890/:917、source_health:17/:98、source_json:10、frontendassets dev:7、release:14/:49/:96、akshare boundaries:19/:62/:131/:151/:188/:219/:268/:321/:356/:392/:487/:517/:580/:604、client_index:14/:45、client_news:41/:70。初值 [x] 14、partial 21、boundary 0。

owner：日历告警与探针由 calendar 承接；前端资源由桌面壳构建模式承接；akshare 转换与客户端由 marketdata helper 与 engine 研究面承接。

复核方法：35 条全量枚举引用有效性，14 条 [x] 逐条核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；partial 逐条核对缺口诚实度。结论：引用全部有效；1 处结论补强，无 verdict 变更、无升级。

补强：2992行下游的 manager:876（快照缓存键市场本地年份）结论原为一句话证据说明，未记录可观测点差异；Rust 无字符串缓存键，同一规则由探针 fetch 窗口承担（UTC 午夜前后 US 本地 2025 与 2026 年窗口），HK 侧未直接覆盖已如实登记。verdict 保持 [x]（键格式为 Go 内部索引细节），证据与锚点不变（evidence=1 anchored=1）。

新增证据：无 Rust 改动、无新增用例、无新增锚点；账本 1 行结论变更（v2 写入器）。

映射终值（35 行）：[x] 14、partial 21、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 1 结论补强，无 verdict 变更、无升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 1 行结论变更（v2 写入器），其余不动 | [x] 1566 不变、partial 2248 不变、boundary 637 不变（合计 4451） |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 与 inventory 按校验后重生成落盘 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 受影响 nextest | calendar 探针年份用例定向 1/1 | 前台直跑 |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 252 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十八：other 域 akshare 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3027-3061，按写入顺序）：client_news:101/:121/:139/:159、provider_calendar_macro:12/:81/:157/:183/:241/:256、provider_company:14/:72/:106/:142/:172/:193/:220/:238/:254/:298/:316/:335、provider_index:23/:48/:65/:80/:97、provider_news:14/:41/:71/:100/:114/:134、provider_rankings:29/:53。初值 [x] 0、partial 35、boundary 0。

owner：akshare 客户端编码与转换由 marketdata helper 与 engine 研究面承接，无双写。

复核方法：35 条全量枚举引用有效性，逐条抽查 Go 原文与缺口描述；高风险面为重试、退避、分页、能力错误与回测兼容。结论：引用全部存在且指向真实 Rust 测试，缺口描述诚实（必填区间、可选编码、默认 limit 归属均如实登记），0 纠正、0 升级。

抽查证据：公司行动缺省区间、端点路径编码、成分默认 limit 三条缺口均为真实设计差异；其余引用逐条确认存在。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 35、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，0 条 [x] | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 217 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片三十九：other 域 akshare 与 yfinance 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3062-3096，按写入顺序）：akshare provider_rankings:82/:108/:141/:159/:172/:231、provider_screen:16/:83/:133/:160/:179/:202、provider:20/:51/:115/:186/:236/:272/:286/:404、yfinance client_news:14/:39/:66/:83、client:17/:30/:56/:75/:86/:110/:136/:156/:172/:192/:214。初值 [x] 0、partial 35、boundary 0。

owner：排行、筛选、榜单、提供商描述符与 yfinance 客户端由 marketdata helper 与 engine 研究面承接，无双写。

复核方法：35 条全量枚举引用有效性，逐条抽查 Go 原文与缺口描述；高风险面为重试、取消、超时、分页与能力错误。结论：引用全部存在且指向真实 Rust 测试，缺口描述诚实，0 纠正、0 升级。

抽查证据：板块转义、提供商能力集合、base URL 校验三条缺口均为真实设计差异；其余引用逐条确认存在。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 35、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，0 条 [x] | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 182 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片四十：other 域 yfinance 转换 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3097-3131，按写入顺序）：client:265、conversion:45/:77/:105/:147/:179/:214/:237/:283/:306/:354/:446/:481/:504/:528/:548/:554/:616/:641、provider_company:14/:69/:100/:134/:161/:192/:220/:236、provider_news:14/:46/:67/:102/:124/:140、provider_rankings:14/:42。初值 [x] 0、partial 35、boundary 0。

owner：快照与 K 线转换由 futu 基础报价与 engine 行情分页承接；公司研究与新闻由 engine 研究面承接；榜单由 engine 市场面承接。

复核方法：35 条全量枚举引用有效性，逐条抽查 Go 原文与缺口描述；高风险面为盘前基线、精度、日历会话、分页与能力契约。结论：引用全部存在且指向真实 Rust 测试，缺口描述诚实，0 纠正、0 升级。

抽查证据：盘前基线保留、高精度成交量、港股午休三条缺口均为真实设计差异；其余引用逐条确认存在。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 35、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口抽查，0 条 [x] | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 147 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片四十一：other 域 yfinance 与 live 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3132-3166，按写入顺序）：yfinance provider_rankings:71/:92、provider_screen:16/:76/:120/:138/:163、provider:33/:66/:98/:163/:229/:254/:265/:287/:340/:379/:421、jftsettings validation:8/:18、types:8/:21/:39、live client:8/:47/:63、lifecycle:10/:38/:63、notification_delivery:5、publisher:11/:39/:57/:73/:108。初值 [x] 8、partial 21、boundary 6。

owner：yfinance 提供商面由 helper 与 engine 研究面承接；日历设置校验由 settings 与 settings-file 承接；订阅归一与通知投递由 engine 与 api transport 承接；重放发布器由 LiveHub 架构替换承接。

复核方法：35 条全量枚举引用有效性，8 条 [x] 逐条核对 Go 原文与 Rust 断言等价（含 Parity 锚点归属与全文唯一性）；partial 逐条核对缺口诚实度；boundary 抽查架构替换诚实度。结论：引用全部有效、缺口描述相符，0 纠正、0 升级。

抽查证据：设置显式标记、订阅归一表、通知投递契约均逐项一致；重放窗口两条 boundary 如实登记为架构替换（序号连续不是 Rust 不变式）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 8、partial 21、boundary 6。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 112 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片四十二：other 域 productfeatures 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3167-3201，按写入顺序）：candle_query:8/:18、capabilities:11/:64/:99/:118、earnings_calendar:11/:35/:72、market_data_reads:12/:116/:134/:226/:245/:307/:371/:418/:470、prediction_bridge:13/:61/:137/:202、capability_alignment:9、facade_calendar:33/:147/:205/:237/:262、facade_company:51/:148/:170/:183/:207/:226、facade_interception:180。初值 [x] 34、partial 1、boundary 0。

owner：查询归一、能力投影、研究日历、行情读取、预测组合均由 engine 领域 crate 承接，无双写。

复核方法：35 条全量枚举引用有效性，34 条 [x] 逐条核对 Go 原文与 Rust 断言等价（含组合引用全文唯一性与锚点归属）；1 条 partial 核对缺口诚实度。结论：引用全部有效，其中多条 [x] 记录真实功能修复与探针过程，0 纠正、0 升级。

抽查证据：adjustment 归一两条组合引用全文不同；预测组合服务端过期、新闻显式 broker 两条断言逐项一致；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 34、partial 1、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 77 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。
### 第 129 批分片四十三：other 域 facade 投影 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3202-3236，按写入顺序）：facade_interception:213/:248/:294、facade_rankings:53/:101/:127/:173/:199/:235/:262/:278、facade_screen:83/:168/:186/:214/:236/:267、projection_calendar:16/:63/:98/:154/:194/:242、projection:14/:79/:97/:139/:160/:186/:205/:226/:290/:326/:351/:371。初值 [x] 35、partial 0、boundary 0。

owner：公司行动拦截、榜单、筛选、研究投影均由 engine 领域 crate 承接，无双写。

复核方法：35 条全量枚举引用有效性，逐条核对 Go 原文与 Rust 断言等价（含全文唯一性与锚点归属）。结论：引用全部有效，其中哨兵保持一条记录真实功能修复，0 纠正、0 升级。

抽查证据：Futu 走 broker 路径、空市场按提供商默认、错误哨兵保持三条断言逐项一致；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 35、partial 0、boundary 0。全量：[x] 1566、partial 2248、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1566、partial 2248、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新 Rust 基线，inventory 无变化 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、quick 完整计划 | ai-context 过；quick exit 0 |

后续：other 域本片完成（余 42 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十四：other 域收尾 42 行（1 处纠正：service_test.go:443 [x] 降为 partial）

范围（账本 rows 3237-3278，按写入顺序）：provider_projection:421/:486/:530/:547、service_routing_and_validation:14/:187/:289/:346、service:12/:30/:46/:82/:99/:143/:210/:243/:321/:404/:443、typed_queries:12/:41、presets:85/:111/:146/:202、retry do_attempts:8/:26、retry:10/:40/:57/:76、passwordhash:9/:25/:32、service_status_defaults:10/:41/:61/:109/:137/:151、system service:13/:59。初值 [x] 36、partial 5、boundary 1。

owner：产品特性投影与服务路由、重试、密码哈希、系统状态均由 engine 领域 crate 与 jftrade-integration-futu、jftrade-settings 承接，无双写。

复核方法：42 条全量枚举引用有效性，[x] 逐分支核对 Go 原文与 Rust 断言等价（含全文唯一性与锚点归属），partial 与 boundary 核对缺口诚实度。结论：1 处纠正、0 升级。

纠正证据：service_test.go:443 原 [x] 条目借用了三处他处断言——snapshot_route_force_refresh_bypasses_the_cache 系 market_http_test.go:330 的已覆盖断言且原引用路径已过期（文件现为 product_market_data_quote_read_tests.rs），tick_candles_use_fresh_cache_without_querying_the_provider 系 routes_test.go:407 与 market_http_test.go:382 的已覆盖断言，prediction_eligibility_rejects_discovery_failure_nil_firm_and_wrong_authority 系 service_test.go:12 的已覆盖断言；跨 Go 用例借用不能记为本用例等价，且修复路径会制造重复 [x]，故降为 partial 并保留唯一共享引用。引用存在不等于断言等价口径保持。

抽查证据：retry 三条 [x] 均为同行为映射（Do 重试至成功对 reconnect 重放、零退避对 BACKOFF.is_zero 加 2 次尝试、不可重试对 0 重连加原错误）；passwordhash 三条逐项覆盖隐藏明文、超界参数前置拒绝、畸形 verifier 表；system status 五条字段与动态值一致；partial 五条缺口诚实（负重试归一无对应 Config、确定性退避阶梯仅相邻覆盖、限流文本谓词拆为结构化分类、日历零值分支无回调注入、运行时依赖改自身探测）；boundary 一条保留成立（Rust 组合根始终装配 owner，无空默认分支）。

新增证据：无 Rust 改动；账本 1 行 verdict 变更（[x] 转 partial），无新增锚点。

映射终值（42 行）：[x] 35、partial 6、boundary 1。全量：[x] 1565、partial 2249、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 42 条全量枚举引用有效性加断言等价抽查 | 1 纠正（443 降 partial）、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 1 行 verdict 变更 | [x] 1566 到 1565、partial 2248 到 2249、boundary 637 不变（合计 4451） |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；0 不存在 crate、0 缺 function_exact、缺锚点告警 120 不变、dup-x 为 0 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：other 域收尾完成（余 0 行）；队列随后 api_transport 439、backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十五：api_transport 域开头 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3279-3313，按写入顺序）：system service:83/:142/:201/:212/:309/:345、floats funcs:9/:24/:29/:34、pivot:9、slice:12/:24/:35/:47/:56/:64、fixedpoint convert:10、dec_dnum:11/:17/:23、dec_legacy:9、dec:122/:132/:140/:157/:189/:200/:206/:231/:274/:317、expirable:12/:24/:32。初值 [x] 7、partial 12、boundary 16。

owner：系统实盘状态与控制由 engine 领域 crate 承接；bbgo floats 序列算术与 fixedpoint 定点语义由 jftrade-backtest 指标、jftrade-kernel Fixed8 与 jftrade-marketdata 缓存按 Rust 架构分头承担，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对 Go 原文与 Rust 断言等价（含全文唯一性与锚点归属），partial 与 boundary 核对缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：system 五条 [x] 均有专用锚点且断言逐项一致（含 :212 不可用时逐操作失败、可恢复时重放的序列端口对照）；fixedpoint :206 解析归一（百分号、科学计数、空串、非有限）与 :231 八位文本加历史载荷由 kernel parity 用例覆盖；floats/fixedpoint/dnum/expirable 的 partial 与 boundary 缺口诚实（无独立 Slice 容器、无 prec 参数、无 dnum 双实现分支、无 ExpirableValue 值容器，均写清相邻覆盖与差异）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 7、partial 12、boundary 16。全量：[x] 1565、partial 2249、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2249、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变（缺锚点 120、空断言 helper 2） |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域本片完成 35 行（余约 404 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十六：api_transport 域 bbgo 类型家族 35 行（2 处结论勘误，无 verdict 变更）

范围（账本 rows 3314-3348，按写入顺序）：expirable:43/:50、reduce:9、fixedpoint slice:10、types account:11/:37/:64、balance:13/:43、connectivity:8、connectivitygroup:11/:38/:295、duration:13/:59、error:10、exchange:9、indicator:16/:35/:41/:55/:75/:95/:109/:115/:121/:128/:137/:146/:152/:160/:169/:178/:186/:204。初值 partial 10、boundary 25，终值不变。

owner：账户与余额语义由 jftrade-trading 承接，指标序列算术由 jftrade-backtest 承接，连接状态由 jftrade-integration-futu 受管会话承接，过期值语义由 jftrade-marketdata 缓存与租约承接，无双写。

复核方法：35 条全量枚举引用有效性，partial 与 boundary 逐条核对缺口诚实度，并抽查 Go 原文核对结论算术。结论：引用全部有效，verdict 0 变更；2 处 partial 结论括号内算术描述与 Go 原文对不齐，仅做文字勘误。

纠正证据：indicator TestDiv 结论曾写 3/2=1.5，Go 原文是序列 {3.0,1.0,2.0} 除以常量 2.0（Last(0)=1.0、Last(1)=0.5、Length=3）；TestMul 结论补全 Last(0)=4.0。两处 verdict 与 entry 不变，仍是 partial（Rust 无 Series.Div/Mul API，逐元素运算内嵌在指标实现中）。

抽查证据：account 锁定两分支对可卖量扣减、期货持仓事件投影收敛、余额估值成本回退、连接组汇合信号对单会话就绪、指标窗口极值对权益峰值回撤，缺口描述均与实现一致；boundary 侧 Rust 无对应对象（分位数助手、dnum 双实现、Reduce 折叠、channel 连接接口、字符串组合时长、逻辑回归训练等）成立；dup-x 为 0。

新增证据：无 Rust 改动；账本 2 行结论文字勘误，无新增锚点。

映射终值（35 行）：[x] 0、partial 10、boundary 25。全量：[x] 1565、partial 2249、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口诚实度抽查 | 0 verdict 变更、2 处结论文字勘误；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 2 行结论勘误 | [x] 1565、partial 2249、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步结论文本；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（余约 334 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十七：api_transport 域市场与持仓家族 35 行（1 处结论勘误，无 verdict 变更）

范围（账本 rows 3349-3383，按写入顺序）：indicator:220/:235/:244/:255/:265、interval:10/:23/:31、kline:10/:37/:61、market_store:9、market:16/:41/:53/:63/:106/:163/:220/:242/:270、omega:11、orderbook:99、position:13/:57/:106/:156/:357/:369、price_volume_heartbeat:12、price_volume_slice:11/:33、rbtorderbook:10/:21/:43。初值 partial 26、boundary 9，终值不变。

owner：K 线窗口与深度档位由 jftrade-marketdata 承接，周期别名由 jftrade-integration-futu 承接，数量规则与持仓快照由 jftrade-kernel、jftrade-trading 与 jftrade-broker 承接，费用规则由 jftrade-backtest 承接，无双写。

复核方法：35 条全量枚举引用有效性，partial 与 boundary 逐条核对缺口诚实度，并抽查 Go 原文核对结论用例值。结论：引用全部有效，verdict 0 变更；1 处 partial 结论用例值与 Go 原文对不齐，仅做文字勘误。

纠正证据：market TestMarket_TruncateQuantity 结论曾写按步长截断 1.239 到 1.23，Go 原文 StepSize=0.0001、用例为 0.00573961 到 0.0057 等三组；Rust 侧 truncate_to_increment 语义对照不变，verdict 与 entry 不变。

抽查证据：interval 桶截断与别名秒数换算、K 线 Tail 与容量截断、最小数量与名义金额上调、持仓 ROI 与费用、价量心跳去重、深度档位投影的缺口描述均与实现一致；boundary 侧 Rust 无对应对象（点积、OLS、Omega、字符串组合时长、OrderBook 类型等）成立；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行结论文字勘误，无新增锚点。

映射终值（35 行）：[x] 0、partial 26、boundary 9。全量：[x] 1565、partial 2249、boundary 637（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口诚实度抽查 | 0 verdict 变更、1 处结论文字勘误；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 1 行结论勘误 | [x] 1565、partial 2249、boundary 637（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步结论文本；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（余约 299 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十八：api_transport 域序列与流家族 35 行（1 处纠正：series.go:83 由 partial 转 boundary）

范围（账本 rows 3384-3418，按写入顺序）：rbtree:15/:40/:59/:75/:94/:118/:143/:175/:188/:245、series.go:83、series_float64:9、sharpe:21、sliceorderbook:9、sort:12/:43、sortino:21、standardstream:37/:71/:121/:149/:190/:221、syncgroup:9、time:10/:18/:26/:56、trade_ring_buffer:12/:46/:96/:127、trade_stat:12/:21/:31。初值 partial 15、boundary 20；终值 partial 14、boundary 21。

owner：红黑树与序列容器无 Rust 对应物（深度以数组档位、指标以切片快照表达）；流解析与会话保活由 jftrade-integration-futu 承接；时间解析由 jftrade-kernel 承接；排序与统计语义由台账、撮合与回测报告分头承担，无双写。

复核方法：35 条全量枚举引用有效性，partial 与 boundary 逐条核对缺口诚实度，并对键在非 _test 文件的条目核查 Go 原文。结论：1 处纠正、0 升级。

纠正证据：series.go:83 的 TestUpdate 不是 testing.T 测试，而是生产文件里的反射驱动助手（用 MethodByName 转调 TestUpdate 方法），在基线内无任何调用方，属测试普查把非 _test 文件的 func TestX 误收录；原 partial 结论把它描述成断言序列更新语义的测试并借用指标用例断言，系过度认领，故转 boundary 并写清普查 artifact。Rust 无此类反射 harness。

抽查证据：红黑树十条边界成立（无价格索引容器）；standardstream 六条 partial 缺口诚实（无通用流抽象、无原始消息双模式、无连接前钩子、保活基于会话协议、订阅按期望对齐、关闭幂等有对应）；松散时间三条 partial 缺口一致（不接受 now 别名与松散回退）；排序与统计缺口与实现一致；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行 verdict 变更（partial 转 boundary），无新增锚点。

映射终值（35 行）：[x] 0、partial 14、boundary 21。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口诚实度抽查 | 1 纠正（series.go:83 转 boundary）、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 1 行 verdict 变更 | [x] 1565 不变、partial 2249 到 2248、boundary 637 到 638（合计 4451） |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（余约 264 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片四十九：api_transport 域可观测与目录家族 35 行（1 处纠正：chart:5 去掉生产函数引用）

范围（账本 rows 3419-3453，按写入顺序）：trade_stat:40/:46、trade_stats:29、trade:6、value_map:10/:37/:59/:75/:91/:100/:118、besteffort:11/:33、chart_type:5、observability context_detach:10/:37、observability:14/:45/:62/:94/:113/:148/:163/:215、catalog_edges:8/:82/:138、catalog_embedded:31/:80/:99/:121/:149、catalog:9/:45/:77。初值 [x] 8、partial 13、boundary 14，终值不变。

owner：尽力审计由 engine 模型运行时承接，图表归一化由 engine 回测解析与 pine worker 分头承接，可观测快照由 jftrade-api 承接，目录语义由 jftrade-research 承接，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对锚点与断言等价，partial 与 boundary 抽查缺口诚实度。结论：1 处引用纠正、0 verdict 变更。

纠正证据：chart TestNormalizeChartType 的 [x] 条目混入了两处生产函数引用（backtest_parse.rs::with_normalized_chart_type、execution.rs::normalize_chart_type），审计解析器不认且规则禁止引用生产函数；去掉后保留两条真实测试（engine 侧与 pine 侧同表断言，均为 anchored），verdict 保持 [x]。

抽查证据：besteffort 两条一次上报与无错静默、可观测快照空数组与错误上界、目录三条形态与编辑器契约均有专用锚点且断言一致；partial 缺口诚实（slog JSON 字段、生成期 helper 全表、松散时间回退等）；value_map 与统计边界成立；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行 entry 引用纠正（去掉 2 处生产函数，evidence 2 且 anchored 2），无新增锚点。

映射终值（35 行）：[x] 8、partial 13、boundary 14。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 1 引用纠正（chart:5 去生产函数）、0 verdict 变更；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 1 行 entry 纠正 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（余约 229 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十：rows 3419-3453 复核（与分片四十九同范围重核，1 处引用补齐）

范围说明：本片 recon 切片与分片四十九重叠（账本 rows 3419-3453，trade_stat/trade/value_map/besteffort/chart/observability/catalog 共 35 行），属重复切分，作一次重核处理，不重复计数。初值 [x] 8、partial 15、boundary 12，终值不变。

owner：定义归一化与目录由 jftrade-research 承接，尽力审计由 engine 模型运行时承接，图表归一化由 engine 回测解析与 pine worker 分头承接，可观测快照由 jftrade-api 承接，无双写。

复核方法：35 条重新全量枚举引用有效性，[x] 逐条核对锚点与断言等价，partial 与 boundary 抽查缺口诚实度，并核查冻结语料的 sourceTest 归属。结论：1 处引用补齐、0 verdict 变更；分片四十九的 chart:5 纠正经重核确认有效。

补齐证据：definition_edges :13 原 partial 条目 entry 为空（全账本约 77 处同类空 entry 之一）；核查冻结语料发现 inferred-set-operator / inferred-scalar-operator 两 case 的 sourceTest 即本 Go 用例，Rust 语料测试真实覆盖其中算子推断三形状，故补上共享引用（与 :249 的 [x] 行共享，partial 允许），verdict 保持 partial（FieldError nil 文本、cleanIDs、numericSlice 等仍无运行时入口）。

抽查证据：定义参数与联合校验两条 [x] 的矩阵断言与 Go 逐项一致；目录展示语义、尽力审计、可观测快照 [x] 均有专用锚点；partial 缺口诚实；脚本工具链 boundary 成立；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行 entry 补齐（evidence 0 到 1），无新增锚点。

映射终值（35 行）：[x] 8、partial 15、boundary 12。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条重核引用有效性加断言等价抽查 | 1 引用补齐（:13 补语料测试）、0 verdict 变更；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 1 行 entry 补齐 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：下一分片必须从 rows 3454 起取新范围（settings 家族），避免再次重叠；api_transport 域余约 194 行；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十一：rows 3454-3488 新范围 35 行（10 处结论纠正，无 verdict 变更）

范围（账本 rows 3454-3488，按写入顺序）：catalog:97、definition_edges:13/:56/:94/:132/:199/:249、definition:10/:35/:71/:87/:103/:139、archive_frontend_assets:11、go-test-quality:8/:14/:30/:39/:44/:59/:78/:94/:100/:119、settings market_data:56/:99/:143/:161/:203/:224/:252、persistence_and_mcp_failures:55/:90/:125、service_managed_accounts:13。初值 [x] 10、partial 13、boundary 12，终值不变。首末键自检通过，与分片四十九/五十无重叠（:13 的补齐归属分片五十，本片不再计数）。

owner：目录与定义由 jftrade-research 承接，提供商设置与 MCP 回滚由 jftrade-settings 承接，脚本工具链不入运行时，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对锚点与断言等价，partial 抽查缺口诚实度，boundary 逐条核对保留理由。结论：10 处结论文字纠正、0 verdict 变更。

纠正证据：go-test-quality 十条 boundary 结论原为同一占位写法（待补证据），实际该工具是面向 Go 测试源码的 AST 断言分析器，仓库已无 Go 工具链，不会新建 Rust 对应物；十条结论已按各子例改写（无断言拒绝、发布形态、两类断言识别、跨文件 helper、嵌套子测试、豁免新鲜度），明确由 Rust 侧门禁（clippy、架构测试、审计复核）承担对应质量职能。

抽查证据：catalog:97 十因子展示表、定义参数与联合矩阵、设置重试与回滚 [x] 均有专用锚点且断言一致；定义 partial 六条语料对照诚实（含 [x] 唯一引用约束说明）；设置 partial 明确写出不能等价的 side-effect（OnProviderChanged 回调、fakeStore 形态）；archive 打包边界成立；dup-x 为 0。

新增证据：无 Rust 改动；账本 10 行结论文字纠正，无新增锚点。

映射终值（35 行）：[x] 10、partial 13、boundary 12。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 10 处结论文字纠正、0 verdict 变更；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2_writer 10 行结论纠正 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步结论文本；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（rows 3489 起；余约 159 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十二：rows 3489-3523 设置与自选家族 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3489-3523，按写入顺序）：service_managed_accounts:35/:58/:84/:99/:111/:120、settings service:153/:232/:242/:268/:318/:344/:361/:435/:446/:480/:565、watchlist futu source_boundaries:51/:99/:137/:181、futu source:44/:83/:174/:221/:267/:316/:357/:396/:418/:445/:459/:470、quote_preview_boundaries:12/:37。初值 [x] 15、partial 20、boundary 0。首键自检通过，与之前分片无重叠。

owner：设置各领域服务（security、mcp_server、broker、execution、onboarding）各自持有 port，由 engine 组合投影到运行时；自选读取与快照由 jftrade-integration-futu 与 jftrade-watchlist 承接，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对锚点与断言等价（含全文唯一性），partial 抽查缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：MCP token 一次性返回与旧 token 失效、Web 密码端口校验与回滚、并发保存最新密码、通知窄发布失败关闭、别名双码分离（brokerCode 与 securityID 不等）、缺席数据显式等 [x] 均有专用锚点且断言逐项一致；partial 缺口诚实（SideEffects 回调结构不存在、单一委托点不存在、single-flight 预约机制不存在、watchlist 端到端批量断言缺失等，均写清差异）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 15、partial 20、boundary 0。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1676、unrecorded 0、stale 0、unknown 48（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick 完整计划 | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0 |

后续：api_transport 域继续（rows 3524 起；余约 124 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十三：rows 3524-3558 自选与回测存储 35 行（1 处锚点加结论纠正，无 verdict 变更）

范围（账本 rows 3524-3558，按写入顺序）：quote_preview_boundaries:74、service_quotes:62/:100/:117/:145/:174/:210/:230/:270、watchlist service:220/:335/:386/:431/:508/:562/:645/:678、backtest adapter_lifecycle:18/:114/:140、kline_database:9、maintenance_concurrency:15/:57/:93/:106、resource:10、store_failure:15/:62/:97/:121/:140、store:12/:52/:138/:154。初值 [x] 3、partial 30、boundary 2，终值不变。首键自检通过，无重叠。

owner：自选名额与快照由 jftrade-watchlist 与 engine 自选产品承接，回测运行存储由 jftrade-store-sqlite 承接，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对锚点与断言等价，partial 与 boundary 抽查缺口诚实度，并核对锚点行号与基线。结论：1 处锚点加结论纠正、0 verdict 变更。

纠正证据：quote_preview :74 的 Rust 测试锚点行号误写 :82（基线 452dea11 与 go 分支均为 :74），结论为占位写法；已修正锚点（含基线前缀归一化），结论改写为归一化层等价（65 字派生名拒收、空白拒收，经 plan 问号传播与 wire 通用映射向外），verdict 保持 [x]。jftrade-watchlist 全量 lib 测试 8 通过。

抽查证据：provider 切换拒绝旧 flight、切换失败保藏、回测存储拒绝坏库与取消不改写等 [x] 与 partial 断言一致；single-flight 缺失、端到端批量断言缺失、路径派生链无断言等缺口诚实；dup-x 为 0。

新增证据：锚点注释 1 行（jftrade-watchlist lib.rs）；账本 1 行结论改写；对账已记录 1676 到 1677、unknown 48 到 47。

映射终值（35 行）：[x] 3、partial 30、boundary 2。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 1 锚点加结论纠正、0 verdict 变更；重复 [x] 全文唯一性检查通（0 重复） |
| 受影响 crate | node scripts/quality/cargo-nextest.mjs run -p jftrade-watchlist --locked | 8 通过 |
| 账本写入 | v2_writer 1 行结论改写 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 与 report 同步；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47 |
| 文档门禁 | fmt、check:ai-context、migration-manifest、zero-go、quick 完整计划、受影响 crate 定向 | fmt（watchlist）过；ai-context 过；migration-manifest 过；zero-go 过；jftrade-watchlist 定向 8 通过；quick 整轮两轮均在同一时序敏感用例转红（见下），其余 1861 通过，不记为通过 |

后续：api_transport 域继续（rows 3559 起；余约 89 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

抖动记录（未记为通过，分片五十四更新）：product_api_launcher_lifecycle::api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal 四次运行中失败三次（两轮 quick 整轮各一；低负载隔离复跑该文件时本用例失败，另第二用例的失败是 nextest fail-fast 取消仍在运行测试造成的人工失败，非真实失败）。失败形态均为子进程 exit 状为被信号终止而非 Some(0)，30 秒超时从未触发；成功的一次为隔离运行。测试先等 TCP 可连接再发 TERM，TCP 就绪早于信号处理器安装即构成真实竞态窗口，与负载相关但非负载独有。本片改动为注释加账本加文档，不触及 launcher 路径；该文件属 launcher 端到端契约（头注记：无 Parity 锚点），修复信号安装竞态属产品行为变更，不在本分片范围，登记为已知不稳定用例待 owner 跟进。证据：/tmp/quick.log、/tmp/quick2.log（整轮）、/tmp/launcher.log（隔离 2 通过）、/tmp/launcher2.log（隔离本用例失败加取消人工失败）。

### 第 129 批分片五十四：launcher 抖动复确认加 rows 3559-3593 存储家族 35 行（无账本改动）

两步：第一步复确认分片五十三遗留的 launcher 时序用例；第二步复核新范围。

第一步结论更新：低负载隔离复跑 product_api_launcher_lifecycle 文件结果为 0 通过 2 失败，其中第二用例失败系 nextest fail-fast 取消仍在运行测试造成的人工失败；第一用例（TERM 后应干净退出 Some(0)，实际被信号终止，30 秒超时未触发）在四次运行中失败三次、成功一次。TCP 就绪先于信号处理器安装即构成竞态窗口，与负载相关但非负载独有；该文件属 launcher 端到端契约（头注记无 Parity 锚点），修复属产品行为变更，不在本分片范围，登记为已知不稳定用例待 owner 跟进。证据：/tmp/quick.log、/tmp/quick2.log、/tmp/launcher.log、/tmp/launcher2.log。

第二步范围（账本 rows 3559-3593，按写入顺序）：backtest store:168、sync_tasks:12/:48、exchangecalendar snapshot_load_failures:12、store_boundaries:13/:31/:45、store_snapshot_failures:31/:73/:95/:136/:169/:199/:214、exchangecalendar store:12/:58、research maintenance:8、research store:16/:84/:122/:142/:190、settingsfile legacy:10、market_data:14/:82/:113/:130/:145、normalization:13/:53/:74/:92/:114/:178、persist_failures:26。初值 [x] 13、partial 22、boundary 0，终值不变。首键自检通过，无重叠。

owner：交易日历快照由 jftrade-calendar 承接，回测与研究存储由 jftrade-store-sqlite 承接，设置文件由 jftrade-store-settings-file 与 jftrade-settings 承接，无双写。

复核方法：35 条全量枚举引用有效性，[x] 逐条核对锚点归属与断言等价，partial 抽查缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：日历十三条 [x] 全部锚定 snapshot.rs（含原子替换失败旧字节不变加无临时残留的真实缝测试）；研究与设置文件 partial 缺口诚实（取消回调覆盖不全、临时文件 fsync 注入无断言、不可用接收者由类型系统阻止等）；dup-x 为 0。

新增证据：无 Rust 改动；无账本行变更；无新增锚点。

映射终值（35 行）：[x] 13、partial 22、boundary 0。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| launcher 复确认 | 低负载隔离跑 --test product_api_launcher_lifecycle | 0 通过 2 失败（其一为取消人工失败）；第一用例 1/4 成功率，登记已知不稳定，不记为通过 |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0（文档账本类改动，空受影响计划）；diff check 过 |

后续：api_transport 域继续（rows 3594 起；余约 54 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十五：rows 3594-3628 设置文件与连接家族 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3594-3628，按写入顺序）：settingsfile rollback:13/:185/:232、store_persistence_contracts:15/:63/:105/:166/:203/:217/:261/:357/:397、store_recovery:13/:27/:55/:87/:111/:158、settingsfile store:13/:40/:83/:136/:154/:184/:205、sqliteconn conn:12/:47/:64/:76/:87/:125/:134/:163/:186/:203。初值 [x] 1、partial 28、boundary 6，终值不变。首键自检通过，无重叠。

owner：设置文件持久化与回滚由 jftrade-store-settings-file 与 jftrade-settings 承接，SQLite 连接语义由 jftrade-store-sqlite 承接，无双写。

复核方法：35 条全量枚举引用有效性，[x] 核对锚点与断言等价，partial 与 boundary 抽查缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：NYSE 唯一默认远端源两条列表断言一致；回滚覆盖不全（仅 appearance 可读）、原子替换失败上报缺失、只读写入拒绝无逐条断言、外键跨连接语义由单连接表达、连接池与并发读无对应物等缺口均诚实；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 1、partial 28、boundary 6。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加断言等价抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0（文档账本类改动，空受影响计划）；diff check 过 |

后续：api_transport 域收尾（rows 3629 起；余约 19 行）；队列随后 backtest_calendar 307、storage_sqlite 196、marketdata_quotes 161、futu_opend 142、trading_broker 56、settings_watchlist 39。

### 第 129 批分片五十六：rows 3629-3663 连接协调与 schema 目录 35 行（纯复核，无引用纠正、无 verdict 变更）

范围更正：分片五十五预估尾部余约 19 行有误，实际账本尾部 rows 3629-4451 共 823 行（sqliteconn/sqliteschema 起，后接回测、存储、行情、集成、交易、设置各域）；本片按既有 35 行步调取 rows 3629-3663。内容：sqliteconn coordinator:11/:44/:58/:83/:94/:112/:156、db_api:12/:109/:178/:218、db_concurrency:10/:46/:80/:113、maintenance:8、sqliteschema catalog:14/:60/:79/:98/:130/:155/:198/:232/:253/:266/:289/:308/:329/:344/:359/:375/:383、schema_boundaries:30。初值 partial 24、boundary 11，终值不变。首键自检通过，无重叠。

owner：写协调与连接语义由 jftrade-owner-lock 的 WriterLease 与 jftrade-store-sqlite 单连接互斥承接，schema 清单由 jftrade-store-sqlite 静态表承接，无双写。

复核方法：35 条全量枚举引用有效性，partial 与 boundary 逐条核对缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：写者串行化与锁文件保留、防御性副本由值类型表达、漂移检测同语义等成立；读屏障重叠、读写池分离、恢复上下文字段等缺口诚实；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 24、boundary 11。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口诚实度抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | ai-context 过；migration-manifest 过；zero-go 过；quick exit 0（文档账本类改动，空受影响计划）；diff check 过 |

后续：继续 35 行步调（rows 3664 起；尾部余 788 行）；队列按账本实际顺序推进（sqliteschema 余量、回测、存储、行情、集成、交易、设置）。

### 第 129 批分片五十七：rows 3664-3698 schema 边界与策略存储 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3664-3698，按写入顺序）：sqliteschema schema_boundaries:58/:85/:107/:115/:147/:170、schema_fault_driver:21/:33/:57、schema:14/:99、strategy persistence_contracts:15、strategy resource:10、strategy runtime_activity:39/:53/:85/:107/:218/:254、strategy store:130/:175/:251/:338/:382/:410/:477/:504/:545/:592/:621/:636/:662、trading broker_fill_reconciliation:11/:97/:144。初值 partial 30、boundary 5，终值不变。首键自检通过，无重叠。

owner：schema 清单与迁移语义由 jftrade-store-sqlite 承接，策略定义与运行时存储由 jftrade-store-sqlite 承接（运行时活动分页断言位于 jftrade-engine 的 strategy_runtime_activity），成交对账由 jftrade-engine 承接，无双写。

复核方法：35 条全量枚举引用有效性（20 个去重后引用全部指向真实 #[test]，0 缺失），[x] 为空无需断言等价核对，partial 与 boundary 逐条核对缺口诚实度。结论：引用全部有效，0 纠正、0 升级。

抽查证据：空语句处理、延迟约束注入、行扫描故障注入、失败注入类缺口均诚实标注无对应断言；三处结构差异 boundary（Go 行集关闭错误合并、旧 JSON 迁移路径、旧运行时迁移分支）均说明 Rust 无同形实现；dup-x 为 0。

提醒线程事项（本批收尾顺带处理）：提醒所述 HEAD e7d11a3d 的重复 [x]（runner_chat_test.go:423 与 store_test.go:792 共用 persist_success 用例）在当前工作区已不存在——store_test.go:792 早已降为 partial（不足断言：批准放行整条 resolving 路径缺端到端用例），不占用 [x] 唯一性；审计 exit 0 通过（dup 校验即原 :749  raise 未触发），映射校验通过后已重跑生成器：report 仅刷新基线到当前提交，inventory 无变化（与工作区一致，无半旧状态）。strategy/backtest 覆盖率提示属旁观信息，不改变本批排期。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 30、boundary 5。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性加缺口诚实度抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；dup 校验通过 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3699 起；尾部余 753 行）；队列按账本实际顺序推进（交易、存储、行情、集成、交易、设置）。

### 第 129 批分片五十八：rows 3699-3733 交易执行账本 35 行（1 处引用纠正，无 verdict 变更）

范围（账本 rows 3699-3733，按写入顺序）：trading broker_ledger:10/:50/:65、execution_composition:32/:70/:153/:313、fill_retention:8、ledger_lifecycle:12/:41/:73/:110/:131、ledger:12/:65/:150/:236/:330、maintenance_concurrency:13/:43、order_leg_merge:10、out_of_order_reconciliation:43/:96/:140/:178/:227、persistence_failures:14/:58、persistence_query_plan:12、resource:10、snapshot_normalization:11/:44、startup_compatibility:14/:31/:73。初值 [x] 14、partial 12、boundary 9，终值不变。首键自检通过，无重叠。

owner：成交对账语义由 jftrade-engine 承接，执行订单持久化与并发语义由 jftrade-store-sqlite 承接，启动路径解析由 jftrade-engine 的 batch atomic startup 承接，无双写。

复核方法：35 条全量枚举引用有效性（25 个去重后引用逐个核对 #[test]），14 条 [x] 逐条核对锚点归属与断言等价（全部带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处引用纠正，0 升级、0 降级。

纠正：broker_ledger:65 的首引用文件名写错（provider_tests 实际无此用例），正主为 product_production_ports_execution_reconciliation_tests.rs:619 的同名用例（带 #[test]），经 v2 写入器纠正；verdict 保持 partial（否定断言缺口仍成立）。

抽查证据：[x] 结论均为逐分支断言（信用边界四值、时间戳四边界、终态幂等、序号高水位、查询计划命中索引等）；partial/boundary 缺口诚实（seen-fill 跨重启去重缺失、placed-merge 路径不存在、lastErrorSource 映射差异等）；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行纠正；无新增锚点。

映射终值（35 行）：[x] 14、partial 12、boundary 9。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（25 去重引用）加 [x] 断言等价核对 | 1 引用纠正；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随纠正行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3734 起；尾部余 718 行）；队列按账本实际顺序推进（交易、存储、行情、集成、设置）。

### 第 129 批分片五十九：rows 3734-3768 交易尾与自选股存储 35 行（1 处引用纠正，无 verdict 变更）

范围（账本 rows 3734-3768，按写入顺序）：trading startup_compatibility:87/:105/:131/:156、submission_safety:13/:63/:117、watchlist delete_transaction_rollback:9/:84、import_persistence_boundaries:13/:130/:172、import_storage_faults:10、import:47/:150/:174/:239、items_query_plan:12/:70、maintenance:8、storage_failure_boundaries:14/:126/:151/:213/:292、store_availability_and_filters:12/:44/:89/:164、store:14/:100/:122/:150/:196/:253。初值 [x] 4、partial 24、boundary 7，终值不变。首键自检通过，无重叠。

owner：执行订单持久化与预测询价账本由 jftrade-store-sqlite 承接（预测消费围栏在 ExecutionOrderStore 唯一写锁内），自选股存储由 jftrade-store-sqlite 承接，维护快照由 maintenance 契约承接，无双写。

复核方法：35 条全量枚举引用有效性（17 个去重后引用逐个核对 #[test]），4 条 [x] 逐条核对锚点归属与断言等价（全部带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处引用纠正，0 升级、0 降级。

纠正：submission_safety:117 的引用串格式损坏（裸生产文件加两个缺文件名的片段，解析只命中 1 个证据），正主为 prediction_quote_ledger.rs 内三个同文件用例（均带 #[test]，文件头带本 Go 用例的 Parity 锚点），经 v2 写入器纠正为三条完整路径，证据 3/3 带锚点；verdict 保持 [x]（三行为一对一）。

抽查证据：部分残表/错列布局/缺运行时表拒绝均为同形建库断言；boundary 均为结构差异（Rust 无 nil 对象、无连接器重校验层、无序号后缀回读）；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行纠正；无新增锚点。

映射终值（35 行）：[x] 4、partial 24、boundary 7。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（17 去重引用）加 [x] 断言等价核对 | 1 引用纠正；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随纠正行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3769 起；尾部余 683 行）；队列按账本实际顺序推进（自选股、存储、行情、集成、设置）。

### 第 129 批分片六十：rows 3769-3803 Pine 资产与策略目录 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3769-3803，按写入顺序）：pineworkerassets asset_selection_boundaries:13/:30/:52/:63、assets_dev:7、assets_release:13、assets:5、strategy catalog activity_degraded:65、catalog_boundary_behavior:34/:58/:84/:123/:192、instance_lifecycle_business:11/:89/:143/:189、plugin_normalization_business:13/:89/:99/:157/:186、repository_failure_business:11/:19/:126、runtime_reconciliation_business:12/:52/:80/:113、errors:8、instancebinding binding:11/:28/:65/:96/:106。初值 [x] 8、partial 26、boundary 1，终值不变。首键自检通过，无重叠。

owner：Pine 资产选择由 jftrade-integration-pine 与桌面发布校验承接，策略目录语义由 jftrade-engine 承接，错误分类由 jftrade-strategy 承接，存储断言由 jftrade-store-sqlite 承接，无双写。

复核方法：35 条全量枚举引用有效性（28 个去重后引用逐个核对 #[test]，0 缺失），8 条 [x] 逐条核对锚点归属与断言等价，partial 与 boundary 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

遗留观察（不改 verdict，登记待 owner 跟进）：本片 8 条 [x] 中 6 条无行级锚点——asset.rs 内两条锚点引用旧包路径 internal/assets_worker（账本键为 pineworkerassets，属 Go 包改名残留），resource_integrity.rs 与 native_tests.rs 的四条无对应 Parity 锚点；均落在既有 119 条缺锚点告警类内。断言等价本身有探针证据支撑（结论内记 shasum 与转红记录），审计为告警非失败，故 verdict 不动。

抽查证据：Pine 六条结论均为逐分支写法并记保留差异；partial 缺口诚实（审计降级容忍、逐条错误分类、逐字段回填等未复刻）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 8、partial 26、boundary 1。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（28 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3804 起；尾部余 648 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十一：rows 3804-3838 策略绑定与实盘管理 35 行（1 处占位结论改写，无 verdict 变更）

范围（账本 rows 3804-3838，按写入顺序）：strategy instancebinding binding:137/:143/:161/:202、instanceview runtime_projection:9、view:13/:28/:46/:62/:92、live_command_business_boundaries:14/:33/:57/:85/:127/:204/:362/:399/:468/:490/:545、liveruntime manager_boundaries:16/:54/:94/:116/:155/:182/:216/:241/:275/:302/:356、manager_close:86/:140/:188。初值 [x] 3、partial 28、boundary 4，终值不变。首键自检通过，无重叠。

owner：绑定归一与错误分类由 jftrade-strategy 承接，实例视图/执行语义/实盘管理由 jftrade-engine 承接，市场规则由 jftrade-broker 承接，回测原子括号由 jftrade-backtest 承接，无双写。

复核方法：35 条全量枚举引用有效性（27 个去重后引用逐个核对 #[test]，其一因文档注释超出检查窗口初判误报，人工确认 #[test] 有效，0 缺失），3 条 [x] 逐条核对锚点归属与断言等价（全部带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处占位结论改写，0 升级、0 降级。

改写：binding:137 的结论为占位写法（已找到证据），Rust 用例实质三分支齐全且带本行锚点，verdict 保持 [x]，结论按子例改写为空输入/None/有效归一化三条。

抽查证据：boundary 均为结构差异（含升级路径）；partial 缺口诚实（逐项报错、逐字段回填、告警聚合等未复刻）；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行结论改写；无新增锚点。

映射终值（35 行）：[x] 3、partial 28、boundary 4。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（27 去重引用）加 [x] 断言等价核对 | 1 占位结论改写；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随改写行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3839 起；尾部余 613 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十二：rows 3839-3873 实盘风控与 Pine 会话 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3839-3873，按写入顺序）：strategy liveruntime nil_boundaries:10/:43、order_risk_business:18/:67/:135/:216/:235、pineworker_live_business:226/:270/:305/:368/:404/:527、product_lifecycle_business:18/:56/:96/:151、runtime_boundaries:21/:48/:116/:180/:258/:277、runtime_risk_evidence:18、subscription_lifecycle:14/:58/:81、symbol_failure_business:14/:57、pine_live_command:16/:36/:60/:76/:93/:130。初值 [x] 3、partial 30、boundary 2，终值不变。首键自检通过，无重叠。

owner：实盘意图执行与风控由 jftrade-engine 承接，运行时风控原因码由 jftrade-trading 承接，Pine 远端会话由 jftrade-integration-pine 承接，订阅租约由 jftrade-engine 承接，无双写。

复核方法：35 条全量枚举引用有效性（28 个去重后引用逐个核对 #[test]，0 缺失），3 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

抽查证据：[x] 结论均为逐字段/逐原因码写法（含新增用例与探针记录）；partial 缺口诚实且带 P1 标记与跨行引用（扩展时段交易日边界差异、异常路径租约释放）；boundary 均为结构差异（含升级路径）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 3、partial 30、boundary 2。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（28 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3874 起；尾部余 578 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十三：rows 3874-3908 实盘执行器与 Pine 运行器 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3874-3908，按写入顺序）：strategy pine_live_command:180/:194/:227/:257/:267、pine_live_executor:14/:36/:66/:80/:93/:119/:150/:180/:198/:226/:255/:284/:314/:349/:383/:428/:443/:462/:474/:508/:532/:556/:601/:648、pineruntime recovery_contracts:9、runner_lifecycle:41/:59/:110/:157、runtime_failure_contracts:16。初值 [x] 4、partial 25、boundary 6，终值不变。首键自检通过，无重叠。

owner：实盘意图执行由 jftrade-engine 承接，原子括号语义由 jftrade-backtest 承接，市场规则由 jftrade-broker 承接，Pine 会话池与资产由 jftrade-integration-pine 承接，无双写。

复核方法：35 条全量枚举引用有效性（21 个去重后引用逐个核对 #[test]，0 缺失），4 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

抽查证据：[x] 结论均为逐值写法（数量算式、错误文案、零调用）；partial 缺口诚实（含回归要求与 owner）；boundary 均为对象迁移或无同形实现；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 4、partial 25、boundary 6。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（21 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3909 起；尾部余 543 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十四：rows 3909-3943 风控策略与 Pine 配置 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3909-3943，按写入顺序）：strategy pineruntime runtime_failure_contracts:30/:52/:76/:99/:146/:171、runtime:18/:60/:79/:105/:118/:145/:165/:185/:228/:250/:278/:325/:344、runtimecontrol optional_values_risk_off:8/:33、policy:11/:53/:68/:83/:102/:120/:146、semantics:5、service:138/:165/:174/:198/:210/:223。初值 [x] 6、partial 27、boundary 2，终值不变。首键自检通过，无重叠。

owner：运行时风控由 jftrade-trading 承接，持仓投影由 jftrade-trading 的 portfolio 承接，Pine 配置解析由 jftrade-engine 与 jftrade-settings 承接，Pine 进程池由 jftrade-integration-pine 承接，无双写。

复核方法：35 条全量枚举引用有效性（25 个去重后引用逐个核对 #[test]，其中一处加号组合引用拆分核对，0 缺失），6 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

抽查证据：[x] 结论均为逐条写法（含补锚点记录）；partial 缺口诚实（含 P1 夜盘边界差异与修复位置、P2 部署形态差异）；boundary 均为类型系统或架构差异（含升级路径）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 6、partial 27、boundary 2。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（25 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3944 起；尾部余 508 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十五：rows 3944-3978 策略服务与指标绑定 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 3944-3978，按写入顺序）：strategy service:236/:255/:278/:304、types:8/:42、definition source_format:8/:17、source_format_validation:8/:16、indicatorbinding parse_semantics:8/:20/:46/:114/:155、parse:10/:93/:152/:176/:212/:235/:283/:304/:347/:384/:395/:425/:433/:467/:475/:501/:509/:542/:574/:609。初值 [x] 0、partial 26、boundary 9，终值不变。首键自检通过，无重叠。

owner：策略服务语义由 jftrade-engine 承接，源码格式与指标绑定由 jftrade-strategy 承接（Pine 官方解析器路径），定义存储由 jftrade-store-sqlite 承接，信号校验由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（21 个去重后引用逐个核对 #[test]，0 缺失），[x] 为空无需断言等价核对，partial 与 boundary 逐条核对缺口诚实度。结论：0 纠正、0 升级。

抽查证据：boundary 均为结构差异（含升级路径，DSL 助手类无 Rust 同形实现）；partial 缺口诚实（逐字段快照、wire 契约等未复刻）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 0、partial 26、boundary 9。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（21 去重引用）加缺口诚实度抽查 | 0 纠正、0 升级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 3979 起；尾部余 473 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十六：rows 3979-4013 指标预热与 IR 规划 35 行（1 处占位结论改写，无 verdict 变更）

范围（账本 rows 3979-4013，按写入顺序）：strategy indicatorbinding parse:666/:720、indicatorwarmup parser_validation:5/:55、risk_specification_rejection:5、spec_parse_business:11/:135/:198/:215、spec_parse_invalid:8/:60/:137/:203、spec_sort_business:8/:66、warmup_internal:10/:38/:50/:62/:110/:142、warmup_plan:12/:40/:74/:98/:121、warmup_script:14/:51/:67/:96/:103、ir planner_branch:9/:100、planner_business_boundary:10/:42。初值 [x] 8、partial 19、boundary 8，终值不变。首键自检通过，无重叠。

owner：指标预热与规划由 jftrade-strategy 承接，指标兼容由 jftrade-backtest 承接，无双写。

复核方法：35 条全量枚举引用有效性（19 个去重后引用逐个核对 #[test]，0 缺失），8 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处占位结论改写，0 升级、0 降级。

改写：warmup_internal:142 的结论为占位写法（已找到证据），Rust 用例实质逐档齐全且带本行锚点，verdict 保持 [x]，结论按子例改写为空串/分钟/小时/天/周/月/非法回退七条。

抽查证据：[x] 结论多为逐值写法（含探针与 shasum 记录）；boundary 均为结构差异（含升级路径）；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行结论改写；无新增锚点。

映射终值（35 行）：[x] 8、partial 19、boundary 8。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（19 去重引用）加 [x] 断言等价核对 | 1 占位结论改写；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随改写行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4014 起；尾部余 438 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十七：rows 4014-4048 IR 规划与 Pine 编译 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 4014-4048，按写入顺序）：strategy ir planner_business_boundary:100/:165、planner_indicator_matrix:8/:55/:147、planner_internal_boundaries:9/:57/:101/:131/:188/:217、planner_internal:5/:88/:122、planner:10/:51/:62/:73/:85/:100/:122/:204/:254/:265/:310、pine collection_object_bounds:11/:85/:141、compiler_and_security_diagnostics:11/:44/:70、compiler_rejection_contracts:9/:79/:180、control_flow_reject:8。初值 [x] 2、partial 28、boundary 5，终值不变。首键自检通过，无重叠。

owner：IR 规划与 Pine 编译由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（9 个去重后引用逐个核对 #[test]，0 缺失），2 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

核查说明：本片 3 条 partial 空引用（request-security 纯度、编辑器恢复、控制流/UDF）经核对审计口径，属允许的缺口散文体（结论内记探针观察、owner 与回归要求），无需补引用；dup-x 为 0。

抽查证据：[x] 结论均为逐键/逐形态写法（含探针记录）；partial 缺口诚实（含缺失键族与 owner）；boundary 均为无同形实现（含升级路径）。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 2、partial 28、boundary 5。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（9 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4049 起；尾部余 403 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十八：rows 4049-4083 Pine 表达式与对象集合 35 行（1 处引用纠正，无 verdict 变更）

范围（账本 rows 4049-4083，按写入顺序）：strategy pine control_flow_reject:43、controlflow_object_collection_contracts:11/:85/:157、expression:5/:11/:21、extended_ticker:5/:49、language_execution_boundaries:12/:99/:179/:287/:364/:454/:531/:600、language_failure_contracts:9/:149/:239、object_collect_bounds:12/:57/:83、object_collect_reject:9、object_declaration_contracts:10/:120、order_command_security_rejection:11/:82、order_metadata_contracts:8、parse_benchmark_business:5、parse_collection:11/:140/:210/:233/:274。初值 [x] 6、partial 15、boundary 14，终值不变。首键自检通过，无重叠。

owner：Pine 表达式与编译拒绝由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（18 个去重后引用逐个核对 #[test]，含嵌套模块路径拆分），6 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处引用纠正，0 升级、0 降级。

纠正：object_collect_bounds:57 引用的多 bar 历史用例实际位于 src/pine/mod.rs 的 parse_history_reference_tests 模块（带 #[test]），而非 tests 目录文件；经 v2 写入器纠正文件路径；verdict 保持 boundary（对象方法降级本身无同形实现，引用仅为同形证据）。

核查说明：9 条空引用行结论均为实质缺口散文（探针观察、owner、回归要求或升级路径齐全），其中 partial 空引用属审计允许的缺口散文体；dup-x 为 0。

抽查证据：[x] 结论均为逐条诊断文案写法；partial 缺口诚实；boundary 均为无同形实现。

新增证据：无 Rust 改动；账本 1 行纠正；无新增锚点。

映射终值（35 行）：[x] 6、partial 15、boundary 14。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（18 去重引用）加 [x] 断言等价核对 | 1 引用纠正；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随纠正行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4084 起；尾部余 368 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片六十九：rows 4084-4118 Pine 版本化集合与请求 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 4084-4118，按写入顺序）：strategy pine parse_collection:330/:375/:400/:430/:477/:515/:554/:587/:628/:678/:724/:768、parse_object:10/:65/:103/:136/:173/:206/:249/:289/:313/:341/:410/:438、parse_request:10/:82/:97/:141/:183/:217/:263、parse_semantic:10/:55/:98/:180。初值 [x] 2、partial 33、boundary 0，终值不变。首键自检通过，无重叠。

owner：Pine 版本化语言面由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（5 个去重后引用逐个核对 #[test]，0 缺失），2 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

核查说明：多数 partial 共用框架语言特性用例作最近邻引用，结论内均如实记录 Go 场景脚本实测被拒的诊断码与未实现族，并给出回归要求，符合部分可共享引用的口径；dup-x 为 0。

抽查证据：[x] 结论均为逐脚本/逐诊断码写法；partial 缺口诚实（含未实现语言族清单）。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 2、partial 33、boundary 0。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（5 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4119 起；尾部余 333 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片七十：rows 4119-4153 Pine 解析语义 35 行（纯复核，无引用纠正、无 verdict 变更）

范围（账本 rows 4119-4153，按写入顺序）：strategy pine parse_semantic:195/:230/:268/:287/:301/:340/:377/:401、parse:10/:54/:84/:113/:126/:145/:167/:186/:202/:232/:253/:286/:317/:348/:367/:390/:432/:448/:464/:488/:560/:587/:621/:651/:690/:718/:732。初值 [x] 21、partial 14、boundary 0，终值不变。首键自检通过，无重叠。

owner：Pine 解析语义由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（25 个去重后引用逐个核对 #[test] 与嵌套模块归属，0 缺失），21 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 抽查缺口诚实度。结论：0 纠正、0 升级、0 降级。

抽查证据：[x] 结论均为逐值/逐诊断码写法（含修复记录）；partial 缺口诚实（含实测被拒诊断码与回归要求）；dup-x 为 0。

新增证据：无（纯复核分片，无 Rust 改动、无账本行变更、无新增锚点）。

映射终值（35 行）：[x] 21、partial 14、boundary 0。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（25 去重引用）加 [x] 断言等价核对 | 0 纠正、0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | 无变更 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 仅刷新基线，inventory 无变化；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4154 起；尾部余 298 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片七十一：rows 4154-4188 Pine 解析恢复与请求 35 行（1 处引用纠正，无 verdict 变更）

范围（账本 rows 4154-4188，按写入顺序）：strategy pine parse:763/:790/:838/:903/:977/:1014/:1029/:1044、parser_and_lowering_recovery:8/:19/:43/:119/:162/:206/:235、parser_helper_boundaries:11/:66/:110/:196、parser_loop_boundaries:10/:38/:63、parser_recovery_boundaries:12/:104/:151、public_lowering:10/:38/:64/:84/:130、request_security_ast_contracts:5/:26、request_security_diagnostics:8/:52、request_security_object_contracts:11。初值 [x] 5、partial 25、boundary 5，终值不变。首键自检通过，无重叠。

owner：Pine 解析恢复与请求安全由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（24 个去重后引用逐个核对 #[test] 与嵌套模块归属），5 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact 且带锚点），partial 与 boundary 抽查缺口诚实度。结论：1 处引用纠正，0 升级、0 降级。

纠正：public_lowering:64 引用的指标属性用例实际位于 jftrade-backtest/src/indicators.rs（带 #[test]），而非 pine_planner_requirement_keys.rs；经 v2 写入器纠正文件路径与执行命令的 crate；verdict 保持 boundary（别名表本身无同形实现）。

跨片观察（不改本片外行，登记待后续扫尾）：同一错误引用（pine_planner_requirement_keys.rs::indicator_properties…）还出现在约 8 行已提交分片的 partial 行（bbgo floats/slice、types indicator、spec_parse_invalid:137），结论缺口描述成立，仅引用文件需同式纠正，待收尾时统一处理。

抽查证据：[x] 结论均为逐脚本/逐诊断码写法（含修复与升级记录）；partial 缺口诚实（含残差行归属语义）；dup-x 为 0。

新增证据：无 Rust 改动；账本 1 行纠正；无新增锚点。

映射终值（35 行）：[x] 5、partial 25、boundary 5。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（24 去重引用）加 [x] 断言等价核对 | 1 引用纠正；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | [x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；inventory 随纠正行更新，report 刷新基线；既有告警不变 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1724、已记录 1677、unrecorded 0、stale 0、unknown 47（均不变） |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4189 起；尾部余 263 行）；队列按账本实际顺序推进（策略、行情、集成、设置）。

### 第 129 批分片七十二：rows 4189-4223 Pine 请求安全/风险声明/元组与 worker 客户端 35 行（5 行补锚点，无 verdict 变更）

范围（账本 rows 4189-4223，按写入顺序）：strategy pine request_security_object_contracts:86、runtime_and_parser_boundaries:9/:46/:62/:94/:134、security_lowering:9/:92/:168、semantic_helper_boundaries:8/:58/:99/:128、shared_structure_corpus:35、strategy_business:11/:95/:130、strategy_call_bounds:11/:98/:120、tuple_assignment_contracts:11、tuple_switch_reject:8、udf_expansion_contracts:8/:37、validation_semantics_boundaries:8/:29/:43/:57/:70/:86/:96/:108、pineengine pine_ts_client:10/:20/:56。初值 [x] 10、partial 19、boundary 6，终值不变。首键自检通过，无重叠。

owner：Pine 请求安全/风险声明/语义校验由 jftrade-strategy 承接，pine_ts worker 客户端由对应集成归属承接，无双写。

复核方法：35 条全量枚举引用有效性（22 个去重后引用逐个核对 #[test] 与嵌套模块归属，0 缺失），10 条 [x] 逐条核对锚点归属与断言等价（全部 function_exact；其中 5 行所引 Rust 用例缺行级锚点，属既有缺锚点告警类），partial 与 boundary 抽查缺口诚实度。结论：0 升级、0 降级；5 行补锚点（只加注释，不改行为）。

补锚点（6 条 // Parity: 注释，覆盖 5 账本行 8 个 evidence 全落锚）：strategy_business:11 → pine_risk_and_block_parity.rs compile_rejects_invalid_risk_declarations 与 compile_keeps_valid_risk_declarations_and_projects_their_limits；strategy_call_bounds:98 → pine_order_metadata_and_security_rejections.rs compile_rejects_ambiguous_order_metadata_and_missing_ids；validation_semantics_boundaries:43 → 同文件 request_security_tuple_diagnostics_match_go_codes；:96 → 同文件 history_reference_overflow_is_rejected；:108 → pine/mod.rs analyze_script_reports_public_internal_helper_diagnostics。账本 5 行经 v2 写入器刷新派生字段（entry/concl/cmd 沿用原文），rustEvidence anchor 全 True。

抽查证据：[x] 结论均为逐脚本/逐诊断码写法；partial 缺口诚实；dup-x 为 0。

映射终值（35 行）：[x] 10、partial 19、boundary 6。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（22 去重引用）加 [x] 断言等价核对 | 5 行补锚点；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 5 行 | rows touched 5，8/8 evidence anchored；[x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| Rust 定向测试 | cargo-nextest -p jftrade-strategy 相关用例 | 6/6 通过（仅注释改动，无行为变更） |
| 工作树 quick | pnpm run check:quick | 未整体通过：434/435，1 失败为 engine 的 adk_session_detail_omits_resolved_approval_groups（满载并行下 timeline 仍引用已解决审批的断言抖动；本片改动为 strategy 注释/账本/文档，与该行为无关）。抖动隔离复跑：该用例单独 5/5 通过，邻近 2 用例通过，所属 adk 模块 134/134 通过；不记为通过，登记待 owner 跟进 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 刷新基线；缺锚点告警 119→116 |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1727、已记录 1680、unrecorded 0、stale 0、unknown 47 |
| 文档门禁 | check:ai-context、migration-manifest、zero-go、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4224 起；尾部余约 228 行）；队列按账本实际顺序推进（策略尾部、行情、集成、设置）。

### 第 129 批分片七十三：rows 4224-4258 PineTS 负载/运行时与 worker 客户端 35 行（1 行补锚点，无 verdict 变更）

范围（账本 rows 4224-4258，按写入顺序）：strategy pineengine pine_ts_payload:18/:39/:55/:72/:97/:131/:190/:207/:224/:236、pine_ts_runtime:12/:31/:58、pinespec lint_helpers:5、skill_metadata:8/:80、spec:14/:28/:59/:108/:248/:274/:285、pineworker client:12/:41/:56/:81/:92/:103/:114/:125/:143/:161、grpc_dialer:9/:23。初值 [x] 2、partial 28、boundary 5，终值不变。首键自检通过，无重叠。

owner：PineTS 负载/影子投影由 jftrade-engine 承接，worker 传输/进程/资产由 jftrade-integration-pine 承接，规范面由 jftrade-strategy 承接，无双写。

复核方法：35 条全量枚举引用有效性（21 个去重后引用逐个核对 #[test]，0 缺失），2 条 [x] 逐条核对锚点归属与断言等价（逐字段比对 Go 原文：:72 合规阻断五元组 Enabled/OK/Mode/Status/诊断码全对齐；:97 成功/失败两半与两个 Rust 用例逐字段对应，PayloadMap 差异已在结论中声明），partial 与 boundary 抽查缺口诚实度（:274 生成快照校验、:125/:143 性能闸门、:23 nil 接收者三处边界理由与 Go 原文一致）。结论：0 升级、0 降级；1 行补锚点（只加注释，不改行为）。

补锚点：pine_ts_payload:97 的失败半所引 pine_shadow_error_payload_keeps_the_worker_failure_message 缺该行锚点（已有 :39/:190 锚点），补一行 // Parity: 注释；账本该行经 v2 写入器刷新为 2/2 evidence 落锚。

抽查证据：[x] 结论均为逐值写法（含差异声明）；partial 缺口诚实（含传输/资产/校验面的归属拆分）；dup-x 为 0（审计 exit 0）。

映射终值（35 行）：[x] 2、partial 28、boundary 5。全量：[x] 1565、partial 2248、boundary 638（合计 4451）；Rust 测试 3295 不变。

验证记录：

| 项目 | 命令 | 结果 |
| --- | --- | --- |
| 逐行复核 | 35 条全量枚举引用有效性（21 去重引用）加 [x] 断言等价核对 | 1 行补锚点；0 升级、0 降级；重复 [x] 全文唯一性检查通（0 重复） |
| 账本写入 | v2 写入器 1 行 | rows touched 1，2/2 evidence anchored；[x] 1565、partial 2248、boundary 638（合计 4451）不变 |
| 审计 | python3 scripts/compatibility/audit_test_parity.py --write-report | 通过（exit 0）；report 刷新基线；既有告警不变（缺锚点 116 为按测试口径计数，本行另一证据已有锚点） |
| 锚点 | python3 scripts/compatibility/parity_anchor_reconcile.py | anchors 1727、已记录 1680、unrecorded 0、stale 0、unknown 47（均不变，:97 已由此行另一证据记录） |
| 文档门禁 | fmt、ai-context、migration-manifest、zero-go、定向 engine、quick、diff check | 见下 |

后续：继续 35 行步调（rows 4259 起；尾部余约 193 行）；队列按账本实际顺序推进（策略尾部、行情、集成、设置）。
