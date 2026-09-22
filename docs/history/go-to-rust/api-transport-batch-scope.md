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

后续（分片五 f）：`internal/api/watchlist/*`（12）、`internal/api/live/*`（15）、`internal/api/marketdata/*`（12）、
`internal/api/research/*`（2）、`internal/api/origin/*`（2）、`internal/api/httpserver` 余量（8）、`internal/api/middleware` 余量（5）；
分片五 g 收尾 `internal/api/assistant/*` 余量（8）与 live SSE 复核。
