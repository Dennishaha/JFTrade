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
