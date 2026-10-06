# Go → Rust 证据积压清单

## 2026-10-07 API/Transport strategy lifecycle 因果链收口

- `TestStrategiesEndpointReturnsList` 已由真实 production Product HTTP owner `production_http_strategy_lifecycle_projects_started_activity` 收口为 `function_exact`：HTTP 创建 definition、instantiate 实例并 start，随后从同一 durable SQLite projection 读取列表、logs、audit，分别断言实例可见、`started` 日志和 `STARTED` 审计。
- SQLite RUNNING 转换在事务内同时落 `STARTED` 审计和 `started` info 日志，避免测试只验证合成快照；旧 `strategy_instance_read_routes_match_group_fixture_in_cutover_only` 仅保留 malformed pagination 引用。
- receipt：`verification-receipts/strategy-lifecycle-http-2026-10-07.json`，**1/1 passed**，SHA-256 `2afb5eb15f1837b14e7c4ef6697f20193c0855a7090310f70f9f6ef18e9c6200`；验证提交 `5098c4fe`。
- 本批不刷新无行为变化的 report/mapping；下一批优先寻找 API/Transport、Strategy/Pine 或 Assistant/Workflow 中仍缺 production causal owner 的高风险 partial。

## 2026-10-06 API/Transport persisted strategy log tail 收口

- `TestStrategiesEndpointIncludesPersistedRuntimeLogTail` 已由真实 Product HTTP owner `production_http_strategies_list_includes_persisted_runtime_log_tail` 收口为 `function_exact`：production SQLite 写入 runtime error 后，经 `/api/v1/strategies` 列表读取并验证日志尾部完整文本。
- receipt：`verification-receipts/strategy-log-tail-http-2026-10-06.json`，**1/1 passed**，SHA-256 `c38bee2f3ec1b56d04d1ddd49e897904a749967243cc778cfce81f85c2292f21`；mapping 已绑定提交 `2e648eb1`，旧 fixture owner 仅保留其他引用。
- 当前扫描 **4451 Go / 3515 Rust / 1586 function_exact / 2225 partial / 640 boundary**；`check:quick` 与完整 `check:rust` 均已通过，workspace **3671 passed、0 failed、2 skipped**，7 类 compatibility replay 全部通过；focused receipt 只证明本条 owner，strict audit 与 anchor reconcile 另行通过。

## 2026-10-06 API/Transport strategy activity filter 收口

- `TestStrategyLogsAndAuditEndpointsSupportPaginationAndFilters` 已由真实 Product HTTP owner `production_http_strategy_activity_filters_level_and_time_window` 收口为 `function_exact`：同一 production SQLite store 写入 info/warning 日志与 kind 审计后，经 HTTP 路由验证 level/time、kind/time 过滤，logs 的 limit/offset 结果与 `total/returned/hasMore`，以及 audit 分页元数据。
- receipt：`verification-receipts/strategy-activity-filter-http-2026-10-06.json`，**1/1 passed**，SHA-256 `ff1619e330f6536315770138c1be41d81022800859aa6cbcc14e30fed7f018d6`；mapping 已绑定提交 `036c2205`，旧 fixture owner 仅保留其余引用。
- focused receipt 只证明本条 owner；完整门禁与 strict 审计另行现场确认，不将 focused receipt 单独视为 workspace 通过。
- 现场收口已完成：workspace **3670 passed、0 failed、2 skipped**，7 类 compatibility replay 通过；当前严格扫描为 **4451 Go / 3514 Rust / 1585 function_exact / 2226 partial / 640 boundary**，数量覆盖约 **78.9%**。

## 2026-10-06 Strategy/Pine runtime lifecycle 收口

- `TestCatalogRuntimeTransitionsPersistStateAndActivity` 已由 durable SQLite Product HTTP owner `strategy_runtime_sqlite_test_cutover_replays_transport_and_restart` 收口为 `function_exact`：真实路由顺序执行 `STOPPED → RUNNING → PAUSED → STOPPED`，每次转换复读持久状态，重启后继续 stop，并断言 `STARTED`、`PAUSED`、`STOPPED` 审计各一次且顺序稳定。
- 绿色 receipt：`verification-receipts/strategy-runtime-lifecycle-2026-10-06-passed.json`，**1/1 passed**，SHA-256 `af4fadeb2cd3f14ca813c52b620931682375fe71fa6cbabf00474e82309092c6`；首次断言顺序失败的红 receipt `strategy-runtime-lifecycle-2026-10-06.json` 保留，不作为当前成功证据。
- strict parity audit 与 anchor reconcile 已通过（2028 unique、1980 recorded、0 unrecorded、0 stale、48 unknown）。`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` 仍保持 partial，因为 Rust 对 RUNNING 采用 resume 语义而 Go 是 blanket reset。
- 现场完整 `pnpm run check:rust` 首次在 workspace nextest 因 Node probe “ok” case 并发超时失败（2069 passed、1 failed、2 skipped，fail-fast）；隔离重跑该测试通过。随后完整重跑已通过：workspace **3669 passed、0 failed、2 skipped**，7 类 compatibility replay 全部通过；首轮失败保留为诊断证据，不计入当前门禁状态。

## 2026-10-06 Strategy/Pine 活动组合根批次

- `TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment` 已由真实生产 HTTP owner 收口为 `function_exact`：`production_http_strategy_activity_pages_filters_and_merges_runtime_observation` 使用 durable strategy SQLite store，验证日志分页（`limit=1&offset=1`）、页面 `total/returned/hasMore`、audit `kind` 过滤、runtime observation 富化和最近日志顺序。
- receipt：`verification-receipts/strategy-activity-http-2026-10-06.json`，**1/1 passed**，SHA-256 `de1921e652d5be0080532c62712e5feddd37f2da5d85f211d768b5c3f7b52d7c`；strict audit、anchor reconcile 均通过。
- 当前 Strategy/Pine 高风险残余：`TestCatalogRuntimeTransitionsPersistStateAndActivity` 仍缺真实 start→pause→stop 每次恰好一次保存/审计的 production route 闭环；`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` 保留 Rust resume 与 Go blanket-reset 的架构差异。下一目标继续优先真实 runtime lifecycle，不刷新无行为变化的报告或 mapping。

## 2026-10-06 API HTTP / WebSocket 行为批次

- `TestSyncRouteClassifiesRequestErrorsAsBadRequest` 从 partial 升为 `function_exact`：真实 Product HTTP 注入生产 `ProductionBacktestPort`，逐项发送 Go 的非法 symbol、非法 since 和反向时间范围，均断言 HTTP 400、`ok=false`、`BAD_REQUEST`、JSON content-type、无 success data，并确认未创建活动 sync task。
- 新增真实 WebSocket depth 订阅测试：由生产 OpenD listener 投影两次 book push，连线客户端收到更新后的价格、`meta.resolvedAt` 和 depth 字段，断线后统计与订阅释放。该条保持 partial：缺少订阅触发的初始 snapshot，push 的 `request.num` 使用实际档数而非请求档数，`entityId` 没有 Go 的 `|50` 后缀。Go 原始 `TestHandlerDepthUpdatePublishesFreshPayload` 没有“相同 resolvedAt 不重发”的断言，不再把该额外要求当作此函数的唯一残余。
- 限定 receipt `verification-receipts/api-sync-http-ws-depth-2026-10-06.json`：**5/5 passed**，SHA-256 `b31ef3a7a788aed0d740e0641ada3e1450243c2d719530c42be6c3506f2c679b`；在 helper 日期修复后的源码上复跑，覆盖两条 mapping 的全部新旧 Rust owner。首轮测试编译缺少 import，修复后的复跑通过；编译失败记录保留在本地 `/tmp/jftrade-api-sync-http-ws-depth-2026-10-06-compile-failed.json`，不作为行为红测。
- 本批只修改两条 mapping 及两条 single-owner reuse 关系；Rust 测试 **3510→3512**，`function_exact` **1581→1582**，partial **2230→2229**。数量覆盖仍为 **78.9%**，严格行为等价仍约 **35.5%**，均不代表整体目标完成。
- `check:quick` 首轮失败：五个 helper sync 用例固定使用 9 月 29 日的 1m 数据，现场日期已超出生产 yfinance 七天历史窗口。请求、mock candle 与分页 cursor 改为同一次捕获的近期 UTC 窗口，保留原行为断言与生产拒绝阈值；另修正正向 capability 用例的相同日期依赖。红 receipt `verification-receipts/backtest-helper-sync-window-red-2026-10-06.json`（0 passed、5 failed）由绿色 `verification-receipts/backtest-helper-sync-window-passed-2026-10-06.json`（6/6 passed）替代为当前定向状态，不删除原失败证据。
- strict parity audit、AI context、anchor reconcile（0 unrecorded / 0 stale）均通过；`check:quick` 重跑已通过，受影响 nextest **2132/2132 passed**。现场完整 `check:rust` 首轮在 target-health 停止（至少 50,000 个 `.rcgu.o`），确认无 Cargo/rustc 进程后按仓库提示运行 `clean:rust:artifacts`，清理可重建编译产物；首次日志保留在本地 `/tmp/jftrade-api-transport-check-rust-2026-10-06.log`。完整重跑明确退出码 **0**：workspace **3668 passed、0 failed、2 skipped**，七类 compatibility replay 全部通过；实际日志为本地 `/tmp/jftrade-api-transport-check-rust-2026-10-06-rerun.log`，未伪造新的 workspace JSON receipt。
- 两个历史失败 receipt 的红绿/current rerun 替代关系已在下一节明确保留，不删除或改写原失败证据。

## 2026-10-04 严格复核补证与下一轮门槛

- ### 历史失败 receipt 的替代关系

  下表中的红 receipt 保留为先红证据，不能删除或改写；当前审计以对应的绿色复跑 receipt 为有效状态。绿色 receipt 的 mapping digest 已分别记录在 `manual-test-mappings.json`，因此历史失败不会被误计为当前门禁失败。

  | 历史红 receipt | 替代绿色 receipt | 当前解释 |
  | --- | --- | --- |
  | `verification-receipts/p1-broker-cancel-rejection-red-2026-09-28T184300Z.json` | `verification-receipts/p1-broker-cancel-rejection-2026-09-28T184500Z.json`；当前工作树复跑为 `verification-receipts/broker-cancel-rejection-2026-10-03.json` | 先红阶段复现旧的 `UNKNOWN` 写入；绿色阶段已验证明确券商拒单保留 `CANCEL_SUBMITTED` 并记录拒单事件。 |
  | `verification-receipts/backtest-aggregation-2026-09-30.json` | `verification-receipts/backtest-aggregation-passed-2026-09-30.json`；当前工作树复跑为 `verification-receipts/backtest-aggregation-2026-10-03.json` | 首轮聚合批次失败后已隔离复跑通过；后续分页、cursor 和聚合顺序 receipt 继续覆盖同一 owner，未把残余语义升级为 exact。 |

- 本轮现场全量 workspace receipt `verification-receipts/workspace-nextest-2026-10-04-final.json` 为 **3664 passed、0 failed、2 skipped suites**（文件 SHA-256 `47f5a5cbba3d5cfd8214c8ca515f45ec04e77855475fa8be6436841db891b210`）；manual evidence verifier 已对 4451 条 mapping 全部通过。
- 2026-10-05 `pnpm run check:rust` 现场复跑为 **3665 passed、0 failed、2 skipped**，7 组 compatibility replay 全部通过；未另生成 workspace receipt，归档证据仍以 2026-10-04 receipt digest 为准。
- 2026-10-06 在当前工作树再次执行 `pnpm run check:rust`，明确退出码 **0**：workspace **3665 passed、0 failed、2 skipped**，7 组 compatibility replay 全部通过；本次仍未生成新的 workspace receipt，因此不改写既有 receipt digest。
- 2026-10-05 新增 `verification-receipts/backtest-aggregation-stream-cursor-2026-10-05.json`，**13/13 passed**：`stream_candles` 对直接区间使用固定页大小和递进 cursor，分钟聚合按完整目标 bucket 分段读取；新增 600 根 1m 跨页回放与 10,000 根 1m→5m 跨交易时段顺序校验。日/周/月聚合仍保留完整读取路径，QueryKLinesCh 多区间 channel 与 EnsureCoverage 仍是 residual，相关 mapping 不升级。
- 新增 `verification-receipts/backtest-aggregation-pages-stream-2026-10-04.json`，**12/12 passed**，覆盖 10,000 根 1m 数据的分页边界、顺序和 callback stream；旧的 2026-10-03 小批 receipt 保留作历史证据。
- 本 receipt 已写回两条 Backtest mapping；当前聚合证据仍只证明结果一致性，不证明有界内存或 Go `QueryKLinesCh` 多区间 channel，因此仍保持 `partial`。
- API/Transport 新增 `verification-receipts/api-sse-write-failure-2026-10-04.json`（2/2 passed）：SSE 通知与 heartbeat 的真实 writer 写失败均在单次回调后终止循环，不重试。WS 会话 send 失败仍只证明 close/断开，不具备 Go dispatcher 的错误返回对象，因此保留 partial。
- API/Transport 新增 `verification-receipts/api-ws-close-lifecycle-2026-10-04.json`（1/1 passed）：LiveHub shutdown 幂等、拒绝新连接、最后连接释放后清空 connected/active instruments 统计，并在 stopped 状态继续拒绝连接；nil handler 的 HTTP 404 仍未有 Rust 同形 owner 断言。
- API/Transport 新增 `verification-receipts/api-execution-order-detail-envelope-2026-10-04.json`（3/3 passed）：execution order detail 的缺失订单与 store failure 在 engine owner 和真实 API router 上分别保持 `404 ORDER_NOT_FOUND`、`500 GET_ORDER_FAILED`，并验证 JSON envelope 字段；缺失 ID 的 handler-level 400 与 typed route 404 差异继续保持 partial。
- Assistant/Workflow 新增 `verification-receipts/assistant-stream-replay-marker-2026-10-04.json`（3/3 passed）：live POST stream frame 不带 `replay:true`，retained/recovered stream frame 继续带 replay marker；Go hub TTL 与内存事件上限属于 durable 模型差异，继续保持 partial。
- Assistant/Workflow 的 chat reconnect 条目已复用 `api-sse-write-failure-2026-10-04.json` 作为真实 transport 证据；当前 residual 是 ADK route 物化 body 与 Go socket-like writer 的 owner 形状差异。
- 下一轮门槛：API/Transport、Strategy/Pine、Assistant/Workflow 三个 P1 域各完成至少一个真实行为闭环，并分别具备 reviewed assertion、Parity anchor、绿色 receipt；随后复跑 `check:quick`、`check:rust` 与 strict parity audit。

## 最新状态：2026-10-03 高风险 partial 行为测试批次

- 本批修改后的现场 `pnpm run check:rust` 已退出码 0：workspace nextest **3659 passed、0 failed，原始输出含 2 个 ignored suite**，7 类 compatibility replay 全部通过；最终工作树 receipt 为 `verification-receipts/workspace-nextest-2026-10-03-final-round.json`（`sha256:bad1550dc8be2d9fbd8ade5b97b61cf9234053ba35aa70fba8c3927ff589a3c0`）。历史 mapping 继续绑定原 canonical receipt，避免批量改写既有证据。
- broker cancel rejection 旧失败记录已由当前工作树 **3/3 passed** receipt `verification-receipts/broker-cancel-rejection-2026-10-03.json`（`sha256:e9fc3b9310f9cefbe81e13e61e7c3d37f0fa9aad09667e3e02406b10cb730001`）取代为当前证据；backtest aggregation 旧失败记录已由 **9/9 passed** receipt `verification-receipts/backtest-aggregation-2026-10-03.json`（`sha256:36db334705ee7d95b9ec5cce5b1b14d85046536bfaa4fe443e493d69589c0023`）取代为当前证据。旧失败 receipt 保留在历史目录供引用，当前 mapping 与新 receipt 均指向绿色复跑证据。
- 新增两个真实 owner receipt：`verification-receipts/backtest-aggregation-pages-stream-2026-10-03.json`（1/1，`sha256:32c3bd7d119e0ec7dbbff0efbd3da47185dba099da9348797328c5fb27b3c123`）和 `verification-receipts/broker-capability-rejection-2026-10-03.json`（1/1，`sha256:70a342b816ad2b08ffe9d7c74dd9731482234d4d8ee01353cfb9a26e83fda429`）。Backtest callback stream、forward/backward limit 和 capability unsupported 的一次性失败/UNKNOWN 账本语义已可执行复现；多区间 channel、非零 UTC wire 投影、请求级 capability 4xx 与 REJECTED/rawBrokerStatus 仍是明确 residual。
- strict parity audit 与 manual evidence verifier 均通过；本批不升级 partial 为 exact。
- `pnpm run check:quick` 已通过：zero-go、policy/contracts、受影响 Rust nextest/clippy、7 类 compatibility replay、Pine worker 与 desktop checks 全部通过。

## 最新状态：2026-09-30 provider 写路由错误矩阵批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:128:TestMarketDataSettingsRoutesMapValidationPersistenceAndRuntimeErrors` 已由真实 Product HTTP 测试 `live_provider_http_route_maps_validation_persistence_and_runtime_failures` 覆盖四类行为：malformed JSON→400 `BAD_REQUEST`、非法 provider→400 `MARKET_DATA_PROVIDER_INVALID`、runtime activation failure→409 `MARKET_DATA_PROVIDER_UPDATE_FAILED` 且旧值保持、settings persistence failure→500 `SETTINGS_SAVE_FAILED`。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-marketdata-provider-errors-reviewed-2026-09-30.json`，文件 SHA-256：`b5cfb9b3a9b816624477a39a9913528b2801a26c31b1df60eaa4276f67cd2c53`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2312→2311**，function_exact **1504→1505**。本轮以真实错误行为、reviewed assertion 与 strict gap 下降收口，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 Web 密码保护 API 行为批次

- 冻结 Go `internal/app/apiserver/webaccess/security_integration_test.go:175:TestWebPasswordIsRequiredForProtectedAPI` 已由真实 Product HTTP 测试 `protected_system_status_requires_web_password_over_product_http` 覆盖：启用 Web 密码保护后，无 session 访问 `/api/v1/system/status` 返回 `401`，错误码为 `WEB_AUTH_REQUIRED`。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-web-auth-required-product-http-reviewed-2026-09-30.json`，文件 SHA-256：`26640e64417f4739d20bb1d629adbb242bc60d4ffa8743f05f398ba3676d69ce`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2313→2312**，function_exact **1503→1504**。本轮以真实行为测试、reviewed assertion 与 strict gap 下降收口，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 live provider HTTP callback 批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:18:TestMarketDataSettingsRoutesReadSaveAndApplyProvider` 已由真实 Product HTTP GET/PUT fixture 覆盖：seed `yfinance`，切换 `futu` 与 `yfinance` 均返回规范化 `activeProvider`，注入 `ActiveProviderState` 的 activation callback 对两次实际 selection 恰好调用两次。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-live-provider-http-reviewed-2026-09-30.json`，文件 SHA-256：`052d983ca4fc111bb019ea843dd3f3644eb84c9dc8aa5ffac63a9f94ce9bd004`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2314→2313**，function_exact **1502→1503**。本轮以真实行为测试、reviewed assertion 与 strict gap 下降作为收口条件，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 backtest provider HTTP route 批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:63` 已由真实 Product HTTP fixture 覆盖：provider catalog 与 yfinance capabilities、prepare failure 的 409 `MARKET_DATA_PROVIDER_UPDATE_FAILED` envelope、旧值保持、成功切换及 GET 回读。
- 为测试组合增加可控 prepare failure seam；生产默认行为不变。定向 nextest **1/1 passed**，receipt `verification-receipts/api-backtest-provider-http-reviewed-2026-09-30.json`，文件 SHA-256：`bfa7c9803beaa15ab798a1ea7a0dd3f5f14766b6bd44e9997590b0817fbd9fbc`。
- mapping 从 partial 升 reviewed `function_exact`；partial **2315→2314**，function_exact **1501→1502**。不以测试数量或 receipt 数量作为完成率。

## 历史批次记录（以下阶段数值不代表最新状态）

## 最新状态：2026-09-30 current-KL 缺失 S2C 行为批次

- 冻结 Go `pkg/futu/opend/market_read_boundaries_test.go:212` 的断言已逐项核对。已有独立 Rust framed-socket reader 测试直接发送 `GET_KL`，模拟 `retType=0` 且缺失 S2C，断言成功空 klines、空 name 和正确协议号；它与另一条纯 decoder Go 测试使用不同 Rust owner，不再合并计数。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2316→2315**，function_exact **1500→1501**。这是行为缺口下降，不以测试数量或 receipt 数量作为完成率。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/futu-current-kl-empty-s2c-reviewed-2026-09-30.json`，文件 SHA-256：`eb09e7f11ce37faa435c2f6072739f742e49cb024efcdf1bbe0f9e35b52badbf`。

## 历史批次记录（以下阶段数值不代表最新状态）

## 最新状态：2026-09-30 HTTPS 代理登录行为批次

- 冻结 Go `security_integration_test.go:210` 已复核。新增真实 Product HTTP + `ProductionAuthSessionManager` 回归，使用临时 settings/session 文件；断言 loopback + `X-Forwarded-Proto=https` 登录为 200，cookie 带 Secure/HttpOnly/SameSite=Strict、session 有效、响应 no-store。无转发头的对照请求不带 Secure。新增行为首次即绿，没有生产修复；编译阶段错误不作为功能红测。
- 该条从 legacy partial 升为 reviewed function_exact。实际映射：function_exact **1499→1500**、partial **2317→2316**、boundary 635；均不是完成率。strict evidence gap **0→0**，不将本批计为 strict gap 净下降；实际减少的是一条 HTTP 行为缺口。
- strict audit 通过；旧 function_exact anchor 缺口和 assertionless 引用复核均为 **0**。anchor reconcile：1899 unique / 1852 recorded / 0 unrecorded / 0 stale / 47 unknown Go line。2 条无可解析 Rust 测试的 acknowledged partial 仍保留。
- 定向 nextest 实际通过；receipt：`verification-receipts/api-webaccess-secure-cookie-reviewed-2026-09-30.json`，文件 SHA-256：`4c614ca808796890f14ef8e95b941b44c630dcf03733c8947769852279de644a`。映射中的 argv、toolchain、commit、timestamp 和 digest 取自该 receipt。
- 本轮全量门禁实际失败：`check:rust` 在 target-health（至少 50000 rcgu.o）停止；`check:quick` 在既有 zero-go provenance 规则停止。未清理 target、删除 provenance 或放宽门禁；这两项不记为通过。全局目标尚未完成。

## 历史批次记录（以下阶段数值不代表最新状态）

本轮模块批量复核：Futu 234 条 legacy exact 绑定 265/265 passed；Assistant/Workflow 164 条绑定 186/186 passed；均写入 reviewed assertion 与同批 receipt，高 fan-out reuse 仍保持未审。strict gap 实际 **1966→1498→1170**，当前仍未通过。

Futu/OpenD 追加批次：watchlist 7/7、trade-account/helper 8/8、subscription/session 10/10 nextest 均通过；26 条已有 exact 写入 reviewed assertion 与 receipt。strict gap 实际 **1998→1982→1966**，高 fan-out reuse 仍保持未审，strict 仍失败。

本清单只统计缺少函数级 Rust 证据的 `[~]` 项；不代表功能缺失，也不代表已覆盖。每项需要人工对照 Go 断言并补充真实 Rust 测试函数、命令或边界结论。

当前积压：**2960 项**（按当前 `manual-test-mappings.json` 的 `[~]` 条目重算）。

最新审计快照（2026-09-29 13:40 UTC）：Go `4451`、Rust `3398`；`function_exact=1491`、`partial=2325`、`boundary=635`；Parity anchor reconcile 为 `1895/1848/0/0/47`（unique/recorded/unrecorded/stale/unknown）。最新批次：API partial/boundary provenance 复核后，8 个 reuse relation 具备 reviewed 引用；broker/market-rule 12/12、Futu watchlist 7/7 nextest 通过，6 条 watchlist exact 与 13 条 broker exact 写入有效 receipt。strict gap 实际 **2041→2036→2010→1998**，剩余 1998 条仍需按行为证据收口。

本轮严格收口记录：Pine client validation、API runtime/market snapshot/WebSocket、desktop readiness 与 Pine indicator owner 均经过真实 nextest；strict gap 由 2052 降至 2041，仍有 2041 条 function_exact evidence gaps。

API observability 与 runtime 两批已收口；随后 frontend asset 行为测试 3/3 通过并将两条 partial 升为 exact（strict 净值不变），再为 WebSocket client registry 建立独立 owner。联合 API tail 复跑 4/4 通过，receipt `sha256:8c885bf0c5b09e5243b1ef111134b9805bf761ded361160755046f4bb1417eb2`；marketdata forwarding 29/29、servercoretest 23/23、servercore behavior 33/33、marketdataapp 22/22、remaining API 25/25、Strategy/Pine parse 21/21、live execution 19/19、risk/order 10/10 通过，先后收口 15、19、17、19、19、21、7、8 条 reviewed exact，strict gap 实际 **2554→2152**；API transport owner 的 legacy exact 已清零。严格审计仍未通过。

随后批量收口 API runtime 的 3 条 legacy exact（路径环境覆盖、相对 settings 资源派生、strategy preview warmup）。真实 Rust owner **4/4 passed**，receipt `sha256:48d34e9714e6c67134a3a60cd34c18276898c03791c202e5e4a156ddd37d4b07`；strict gap 实际 **2563→2557**，仍保留其余历史 reuse/receipt 缺口。

第二批 API transport route 复核再收口六条已有 exact（optional query、缺失 URI、markets provider failure、非法 refresh、订阅租约冲突）；8/8 行为测试通过并绑定 receipt `sha256:84dbc60dbeb07184ad3c0cbc750321351de2bd6b052e61ea613ea329e0ba553c`。strict gap 实际 **3262→3250**；其余 `[~]` 与结构不等价项继续保留，未用测试数量或 receipt 数量表示完成率。

第三批 API auth/origin 复核六条已有 exact；7/7 行为测试通过并绑定 receipt `sha256:a90d53c5245a42b16cb1397efcf98d666f3bc7aa3c02e56a9dda6e60b19d59e2`。strict gap 实际 **3250→3238**；桌面 scheme 的 wails/tauri 差异继续作为已登记 boundary，不计作未记录功能。

第四批 API subscription 复核七条已有 exact；7/7 行为测试通过并绑定 receipt `sha256:66229dc2cec638a02dc06384b47949a7bca05c593b67daa57dbbce6cecb41710`。strict gap 实际 **3238→3224**；malformed wire fixture 与 subscription owner 的组合仍只按逐条 assertion 计入。

WebSocket live 批次 5/5 行为测试通过并绑定 receipt `sha256:d5630dbcfc1a2e7e10bdaf5bf09f96e992fbff7c53addf84023fa4f0880510dd`；3 条 exact 升为 reviewed，heartbeat `liveClients` 缺失与 Host/allowlist same-origin 差异改记 reviewed partial。strict gap 实际 **3224→3210**。

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

- 2026-09-28 Strategy/Pine manager close P1：`TestManagerCloseAggregatesNamedSessionErrorsOnce` 先由单 session 错误 fixture 暴露聚合上下文缺口，随后新增两个真实活跃 Pine session 的并发 shutdown 回归；`shutdown_with_error` 现在串行化并发关停、稳定复用聚合错误、按 instance/market.symbol/session close 命名错误，并保证每个 session 只 close 一次。定向 engine nextest 4/4、完整 `check:rust` 3522/3522（2 skipped）、quick 2571/2571（1 skipped）均通过；receipt `sha256:e319b1dd01b56b3ae8bb821aa23bd831c2eb4ca5216765035961fd92f0bdd185`。对应 Go 条目已升级为 `function_exact`；启动竞态聚合的相邻条目仍保留 partial。

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
- 2026-09-28 Assistant runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:cbeb1497d9d8ee4db2dbafb58ab65670dd8abbd74d3c020007e4ab127b04b211`（engine nextest 5/5）；覆盖 tool ordering、cancellation join、probe timeout cap、approval lease cancellation 与 session title reuse。strict gap 降至 3695，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu history-window strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:603f4b41c4847b7f46c69791b46609ecb78503502e1118929ce1f5d5d9f09338`（integration-futu nextest 5/5）；覆盖 session planning、multi-page pagination/page limits、upstream page sizing 与 payload-less success normalization。strict gap 降至 3692，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant store-lifecycle strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:df78dcad3dcd67f0e6dc10d4c57da16dee947f4114ebfdeb509578e2df9bc07e`（store-sqlite + engine nextest 5/5）；覆盖 cascade cleanup、session/composer/approval store semantics 与 provider timeout normalization。strict gap 降至 3687，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant run-time strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:172dd3f940aca79bcb55c464e065550bf98f4273f0ff1559bba3f44ad9e5560c`（engine nextest 5/5）；覆盖 terminal cancellation audit、configured/per-run timeout windows、resume reset 与 expiry cleanup。strict gap 降至 3682，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu notification/probe strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:ae8a2c04479d9119e93e169d4f3e3a048488f478e60b591242891cc9c4040d6a`（integration-futu nextest 6/6，live OpenD ignored）；覆盖 notification payload/status routing、closed-port disconnected probe、program-status formatting 与 candle-session mapping。strict gap 降至 3672，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu subscription-reconciler strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c3d81229c5939b1e25203f0f1c269d609600a0ed226da538c5fc7b1e8bd5e3e0`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 subscription sharing/deferred release、concurrent idempotence、retry ladder/reacquire、delayed fallback 与 connection quota reset。strict gap 降至 3662，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu client-recovery strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:8691574cb891a5f89c932ccd431f408bf4c3893654277016ba7e57c0c62ace54`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 replay-safe recoverable errors、session replacement、callback lock release、minimum version 与 typed transport failures。strict gap 降至 3652，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu quote/empty-boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:65f7118836c30241fe900c00e4cb632ce9c8ead298e4c7d90e93f87bab8c69c1`（integration-futu nextest 10/10，live OpenD ignored）；覆盖 empty normalization、order-book/basic-quote empty/rejection paths、duplicate quote projection 与 invalid/payload-less snapshot rows。strict gap 降至 3642，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant engine-gates strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:484a697d804103853875d6d7e2d34429dc85981bc916d7568cb3e1fecca74a49`（engine nextest 5/5）；覆盖 strategy optimization persistence、MCP runtime lifecycle、agent/provider resolution、tool catalog availability 与 retryability envelope。strict gap 降至 3632，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu mixed-boundaries strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:77b7aeff1ac829b88bffc92e012affa9c99f2f1b01599c1d957e5d7a1cac473f`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 tick fallback、subscription normalization/release、HK-only research state 与 quote-right cache refresh。strict gap 降至 3622，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 API market-runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:ea1a72725d8f5cd97f2e15017b44283975b76e1aef4d2af17bab7a72ef24009b`（engine nextest 7/7）；覆盖 candle cache/provider fallback、research provider forwarding 与 strategy cancel dispatch。strict gap 降至 3612，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu research/boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:b32a88c5f551e7f3b00501146b2f5b3e32ec0a931078327803c0bcd36c2d493c`（integration-futu nextest 9/9，live OpenD ignored）；覆盖 K-line/price helper、research/calendar pagination、disconnected reads 与 fallback wire coercion。strict gap 降至 3602，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Strategy/Pine targeted cancel P1：`TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders` 先补真实 execution store owner 断言，覆盖成功撤单移除 tracking、失败保留 tracking 与 foreign/untracked gateway 隔离；定向 engine nextest 3/3 passed，receipt `sha256:3423dac32787c4000997a77e747bdf13c64a024334cf4c27e9e341eb47d25fa3`。对应映射升为 `function_exact`；cancel-all 的 execution-store 成功终态仍保持 partial，不把 strategy owner 证据重复计入。
- 2026-09-28 Strategy/Pine cancel-all success P1：`TestLiveCommandExecutorCancelAll` 先补真实 store-backed gateway 终态断言，覆盖逐笔派发和成功清空 active ledger；定向 engine nextest 9/9 passed，receipt `sha256:fc615c228ba5af017d074b6793421fc3e624cc447a0abd801c9a8ad8c8eb6901`。对应映射升为 `function_exact`，失败撤单保留 tracking 仍由相邻回归覆盖。

- 2026-09-28 P1 Strategy/Pine lifecycle：已补 `Pause/Stop` 状态先写后停 runtime 与转换失败保留 owner 的真实回归，定向 nextest 4/4；映射仍为 `partial`。剩余证据缺口是 Go `WithLiveMarketStreamRefresher` 每次操作刷新两次的同形计数 seam，不将 router demand reconcile 过度宣称为 exact。

- 2026-09-29 P1 Strategy/Pine targeted cancel alias：先红复现陈旧意图 ownership error，随后在 `dispatch_cancel_intent` 增加确定性 clientOrderId alias 解析、internal id 去重和 stale no-op；定向 nextest 3/3 passed，receipt `sha256:a3e1e7ac8664d078feccbdd1fd234bb2abf8e0ec6e2e51a2d3ad5c43de7f9fc7`。映射保持 `partial`，剩余缺口是 Go 显式 `activeOrderAliases` 多腿持久化 owner 与复杂 OCO alias 回归。
## 2026-09-30 API SSE reviewed batch

- 已完成 5 条 API transport SSE 行为条目的 Go/Rust 断言复核与联合 nextest 5/5：`write_event_propagates_serialization_and_write_failures`、`stream_loop_ignores_trigger_without_callback`、`write_failures_are_reported_with_their_source_message`、`concurrent_writers_serialize_frames`、`write_event_returns_flush_panics_as_errors`。
- 共享 receipt：`verification-receipts/api-sse-reviewed-2026-09-30.json`，digest `sha256:23cb9085910da407cffce794533dee1ff1eea1154a3a50a766fa6afd67ed9ec9`；单引用 reuse 与 Parity anchor 均保留。
- strict gap 由 3395 降至 3391；剩余 API transport exact 继续按 reviewed assertion、anchor、reuse 与 passed receipt 收口，不能用 receipt 数量替代行为完成率。
## 2026-09-30 Futu transport reviewed batch

- 5 条 Futu transport exact 已补真实 Parity anchor、reviewed assertions 和联合 nextest receipt：`TestTradeReadMethodsPropagateTargetProtocolDisconnects`、`TestQuoteKLineAndOrderBookPropagateTargetDisconnects`、`TestTradeWriteMethodsPropagateAccountAndWriteDisconnects`、`TestTradeWritesAreNotReplayedWhenResponseIsLost`、`TestDirectSubscriptionCallsPropagateClosedClientErrors`。
- receipt：`verification-receipts/futu-transport-reviewed-2026-09-30.json`，digest `sha256:a0906c826c136f6c21afaaf156d85bc399947b9047e47431aa33fcc242ad4290`；strict gap 3385→3378。

## 2026-09-29 evidence closure checkpoint

本轮只计入有真实行为断言、reviewed assertion coverage、有效 Parity anchor 和 passed receipt 的 5 条 API SSE exact。联合测试覆盖 7 个 Rust 测试（含 router headers 与 retry=0 断言），receipt `sha256:8f5cf46fdb2e9bcd5af4d3de9b52781eb5cbbcc4af2ebdde8c267252e0c05ec4`。严格审计缺口由 3378 降至 **3368**；其余历史 exact 缺口继续列为 backlog，不以 Rust 测试总数、receipt 数量或单次 verification passed 代替完成率。

## 2026-09-29 Futu P2 evidence closure

本轮只把 3 条同形度足够的 Futu 行计入 reviewed：默认端口、订阅 frame、零价 previous-close。7 个 Rust 行为断言在联合 nextest 中通过（本轮单独执行 3/3），receipt `sha256:c121cb50db0c539b0df79c1bea408c2aaa59111be0295c6b9169b2bc875b30c2`；strict gap 实际下降到 **3353**。Pine asset 选择行保留其 `(Asset,false,nil)` 与 Rust typed error 的 seam 差异，未机械升 exact。

## 2026-09-29 partial correction

本轮明确记录两项剩余功能差异：日线 QueryKLines 的真实 OpenD client 返回断言、RequestHistoryKL 的 KLine/next cursor response 投影尚未由 Rust 同形测试证明。条目保留 `partial`，即使窄 helper/frame 测试通过也不计为 exact；strict gap 实际下降到 **3341**。

## 2026-09-29 remaining behavior gaps after review

- Futu funds：补 availableFunds 优先级、locked/maxWithdrawal、CN/MY 兜底和负 locked 夹零的同路径行为断言。
- Futu order mapping：补 Go 目标订单类型折叠、FOK/nil/DAY TimeInForce 与 margin account type 断言。
- Pine asset：Go 的 embedded FS 选择、missing/empty 返回空 Asset + false + nil 与 Rust typed error 仍为不同 seam；保持 partial。
- Futu history：真实 QueryKLines/RequestHistoryKL client response 投影仍为 partial。

本轮 reviewed exact 中 prediction push、Assistant research backtest、candle adjustment 都有真实 Rust 行为断言与 passed receipt；strict gap 当前 **3323**，整体未通过。
## 2026-09-29 Futu snapshot fallback reviewed batch

- 六条 Futu snapshot fallback exact 已完成断言复核并升级 `reviewed`：行与 market 分组、canonical/取消/错误与 clone、strict delayed quote fields、static-id 无订阅回退、TTL 正负缓存、StockScreen 错误传播。
- 定向 nextest 6/6 passed；receipt：`verification-receipts/futu-snapshot-fallback-reviewed-2026-09-29.json`，digest `sha256:5d2e8d55218adc5252b1389a258dbf15803b757a9bb05091ccd10b78c56cb8f6`。
- strict gap 由 **3323** 降至 **3312**。剩余 exact 仍需逐项补 reviewed assertion、有效 anchor 与 passed receipt；本批不以测试总数或 receipt 数量代表完成率。

## 2026-09-29 API auth/SSE reviewed batch

- SSE loop 两条与 auth middleware 三条已完成 reviewed assertion、anchor 与 passed receipt；receipt 分别为 `api-sse-loop-reviewed-2026-09-29.json`（`sha256:803a99b17ac95ec97490cbf3f1f91ee8c3aa95d061ee9966dfdd9217f3bae18e`）和 `api-auth-boundaries-reviewed-2026-09-29.json`（`sha256:d9491a62ef3b46d0ff5ee03f652748d32fa1bd219460c95a0edd7e3748702b89`）。
- `TestAuthSkipsPublicPaths` 与 `TestAuthProtectsLogout` 因 `/health`/204 断言和 logout 200/204 投影差异收窄为 reviewed partial。
- strict gap 由 **3303** 降至 **3297**；API 低测试比例仍是后续行为补齐重点。

`TestAuthRejectsNilAuthenticator` 与 `TestAuthRejectsUntrustedOrigin` 随后完成 reviewed 收口；2/2 nextest receipt `api-auth-rejections-reviewed-2026-09-29.json`，digest `sha256:d302cddef14057dec554dd8d48e00892a71e7c5a2e4c565fff7010970fbe800e`，strict gap 3297→3293。

## 2026-09-29 API P1 Web/Execution review

- 三条 Web 行为已 reviewed：disabled navigation page、cookie+CSRF browser flow、cookie-only WebSocket；receipt `api-web-p1-reviewed-2026-09-29.json`，digest `sha256:02d835502c6392862c7678ed3087c19e51198186c265a1583cc181bbac8d5b92`。
- ETH execution session 的 Rust normalization/wire 测试 3/3 passed，receipt `api-execution-session-reviewed-2026-09-29.json`，digest `sha256:8abeb1dae760d9861b6bd1f5070052482df98eb3047d517386db66084255dce8`；由于缺少 Go 同形 HTTP route，映射保持 reviewed partial。
- 前端资源完整矩阵、密码变化触发 session invalidation、POST logout route 均已纠正为 reviewed partial；strict gap 3293→3274。

启动 rollback 条目随后完成证据复核：engine nextest 2/2 passed，receipt `api-startup-rollback-reviewed-2026-09-29.json`，digest `sha256:5ff41a57feb4610daf6dfae3a59381aad5bf567d79b1f659a0da8068872fa066`。Rust 覆盖 production migration/resource rollback，但 Go generic Handle callback 与 error-chain seam 未同形迁移，映射保持 reviewed partial；strict gap 3274→3270。

## 2026-09-29 API transport P2 behavior review checkpoint

- 已复核 7 条既有 `function_exact` 的真实行为断言：request observability、非法 request id、route method/path isolation、Swagger core paths、system request id propagation、web login rate limit。
- 联合 nextest **9/9 passed**；receipt `api-transport-p2-behavior-reviewed-2026-09-29.json`，digest `sha256:ce452bd3c5eb1ac54fa17c8f9bd952a69c8e811dc9abc3b0bcd2e9503492a19d`。
- strict gap **3088→3065**，仅计 reviewed assertion +有效 anchor + passed receipt +审核 reuse 的实际收口；Rust 测试总数、receipt 数量和 verification passed 不作为完成率。
- assertionless exact 复核结果为 **0**；anchor reconcile `1894/1847/0/0/47`。后续优先清理剩余 legacy-conclusion exact 与 API transport wire owner 行为缺口。

锚点补齐后 strict gap 由 **3065 降至 3064**；当前 strict error 分类为 receipt 1146、reviewed assertion 1044、reuse 869、test filter 5，anchor 缺口为 0。

## 2026-09-29 API transport P2 runtime behavior checkpoint

- 8 条 runtime/strategy/settings exact 完成 Go 断言复核并升为 `reviewed`：provider switch、warming health、Node dependency diagnostics、strategy quantity sizing、broker normalization、combo quantity mode。
- 联合 nextest **12/12 passed**；receipt `api-transport-p2-runtime-reviewed-2026-09-29.json`，digest `sha256:652cbc5eae4d4533af8b22ff2ea8f098cb6398bea05bf7f3d6e9871403d18d6b`。
- strict gap **3064→3048**，只计真实行为测试、reviewed assertion、有效 anchor、passed receipt；测试总数与 receipt 数量不作为完成率。

## 2026-09-29 API datamigration P2 behavior checkpoint

- 8 条 SQLite 维护 exact 完成 Go 断言复核并升为 `reviewed`：backup retention/quota、incompatible snapshot、failed backup cleanup、rebuild selection、manifest drift、schema catalog。
- `jftrade-store-sqlite` 定向 nextest **8/8 passed**；receipt `api-transport-p2-datamigration-reviewed-2026-09-29.json`，digest `sha256:4d739bd52477eb28f8bf1dbdcaba199570f595d524ea28b7b89a9793aa5397a4d`。
- strict gap **3048→3032**；本轮仅按真实行为、reviewed assertion、anchor、receipt 收口计入。

## 2026-09-29 API datamigration safety checkpoint

- 8 条 rebuild-safety/backtest/broker route exact 完成逐项断言复核并升为 `reviewed`。
- `jftrade-store-sqlite`/`jftrade-engine` nextest **8/8 passed**；receipt `api-transport-p2-datamigration-safety-reviewed-2026-09-29.json`，digest `sha256:c91a4ac3fe696e1fbebd5897d1f96b81b9e42433c0ea3761390452bc25ae0119`。
- strict gap **3032→3016**；root-only skip 分支仍保留环境边界说明。

## 2026-09-29 API read/settings P2 behavior checkpoint

- 10 条 read/settings/runtime exact 完成断言复核并升为 `reviewed`：lookback、preview failure、sidecar stop、appearance/market fixtures、market profile、research preset、settings environment isolation、onboarding/readiness。
- 联合 nextest **10/10 passed**；receipt `api-transport-p2-readsettings-reviewed-2026-09-29.json`，digest `sha256:77a4805fce48b4565c50fe4d7475287a2977f08bae3486c1007be627fdbb65fc`。
- strict gap **3016→2996**，只计真实行为证据。

## 2026-09-29 API shared-owner P2 checkpoint

- 5 条 shared-owner exact 完成断言复核：backup marker retention、K-line explicit bounds、current-bar intent、depth method rejection、legacy source-format rejection。
- 8 个 owner tests nextest **8/8 passed**；receipt `api-transport-p2-shared-owners-reviewed-2026-09-29.json`，digest `sha256:9c451f9dc06c5c9679f7299c2d089593a3526086b535c497b1210fa8dc3f9080`。
- strict gap **2996→2986**；没有把共享测试命中次数当作完成率。

## 2026-09-29 runtime dependency shared-owner checkpoint

- Node probe OK/outdated/invalid/command-error 两条 exact 完成断言复核，shared owner reuse 已审核。
- `jftrade-engine` nextest **1/1 passed**；receipt `api-transport-p2-runtime-dependencies-shared-reviewed-2026-09-29.json`，digest `sha256:e12b481f0265f0680e5e6ca822d09c2ff5770ca08cd2924ddffe996a56c395a0`。
- strict gap **2986→2980**。

本轮补充审核两个已 reviewed owner 的 reuse 关系（optional query bool alias、candle adjustment normalization）；无新增行为测试，strict gap **2980→2976**，该下降仅表示 reuse 证据闭合，不计为新增功能行为。

## 2026-09-29 broker runtime correction

- 两条 broker runtime 旧 exact 因缺少同形 production HTTP route owner，降为 reviewed `partial`。
- Rust projection 字段仍有证据；真实 HTTP 200/ok envelope/assembly wiring 留在 backlog。
- strict gap **2976→2969**；该下降不计为新增行为。

## 2026-09-29 strict batch evidence update

Execution、Backtest、Strategy/Pine、Assistant workflow、Watchlist、Provider Research 六个 API 行为批次已逐项核对 Go assertions，并以真实 nextest owner 测试和 receipt 收口；本轮没有把文档行数、receipt 数量或 verification passed 当作完成率。6 个 batch receipts 已写入 `verification-receipts/`，对应 mapping 的 `assertionCoverage.source` 和多引用 reuse 已升为 `reviewed`。

strict gap 实际 **2969→2853**。剩余缺口仍主要是历史 `legacy-conclusion` assertion、未绑定 receipt 和未审核 reuse；API transport 低比例的行为补齐继续按 route owner 推进。边界/partial 引用未因共享测试而升级为 exact。

## 2026-09-29 live volume/heartbeat evidence update

OpenD live listener 与 ws-live fixture 的 13 条 exact 已完成逐项 assertion review、anchor/reuse 审核和真实 passed receipt。覆盖 volumeDelta/cumulativeVolume、超大累计量、trade/depth 同订阅投影及 heartbeat/通知 wire。strict gap **2853→2821**；没有把 fixture case 数或 receipt 数量作为完成率。

## 2026-09-29 execution validation evidence update

US price tick/session/market-code 两条 API exact 已以 engine 与 Futu wire owner 测试重新验证并绑定 receipt；严格审计 **2821→2805**。其他非 API 引用保持原结论，不因共享 owner 自动升级。

## 2026-09-29 system status evidence update

System status/runtime resource 的 4 条 exact 已完成真实 owner 测试、assertion review、anchor/reuse 审核和 receipt 绑定；status mapper 的非同形 DTO 边界继续保留 partial。strict gap **2805→2794**。

## 2026-09-29 receipt coverage repair

补跑缺失 Rust owner 并替换相关 receipts，修复 reviewed rows 的 testFilter 覆盖缺口；当前 reviewed exact 不再存在 `rustEvidence` 未包含于 `testFilter` 的 mismatch。strict gap **2794→2775**。

## 2026-09-29 ADK catalog evidence update

ADK catalog/middleware 的 3 条 exact 已完成 production assembly owner 测试、assertion review、reuse 审核和 receipt 绑定；strict gap **2775→2764**。当前 reviewed exact 的 `rustEvidence ⊆ testFilter` mismatch count 为 0。

## 2026-09-29 runtime resources/lifecycle evidence update

runtime resource ownership 与 lifecycle 两批已完成真实 owner 测试、assertion review、anchor/reuse 审核及 receipt 绑定；集中布局与 Go callback 形态差异保持 partial/boundary。strict gap **2764→2732**。

## 2026-09-30 API/Assistant evidence update

补跑 runtime lifecycle 遗漏 owner 后，按 API/Assistant route owner 批量收口 approval、workflow、chat stream、catalog、task/memory 与 provider 边界。所有本轮 exact 都有行为测试、reviewed assertion、有效 receipt 和 reuse 审核；严格 gap **2732→2657**。随后对 8 个共享 owner 做一致性 reuse 审计，gap **2657→2641**。没有把 receipt 数量或 Rust 测试总数当作完成率，partial/boundary 结论保持不变。

## 2026-09-30 transport/data-management evidence update

新增 API/Transport route owner receipt（13/13）并收口 9 条 exact；补齐 8 条 reviewed receipt 元数据后，strict gap **2641→2612**。新增 SQLite/data-management owner receipt（10/10），覆盖 schema 缺失、损坏 marker、备份配额、overview/cleanup 与 pending rebuild rollback，strict gap **2612→2592**。所有变化均以实际行为测试和 strict gap 下降为准。

## 2026-09-30 replay/assembly evidence update

重跑 backtest P1 旧 receipt 的 10 个 owner 并绑定完整当前 commit，strict gap **2592→2589**；重跑 Assistant P1 9 个 owner，并收口 5 个无共享 owner 的 assembly/MCP exact，strict gap **2589→2579**。仍有共享 owner 的 legacy 引用时，保留其原结论并继续列入 backlog。
## 2026-09-30 API marketdata forwarding batch

- 15 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 calendar/company/news/rankings/screen/index-constituents forwarding 及 capability、helper isolation、limit/page 边界。
- 定向 engine nextest **29/29 passed**；receipt：`api-transport-marketdata-forwarding-reviewed-2026-09-30.json`，digest `sha256:51abd6e018b42e2f4f3a8ee2acd76ecb8e53cc80fbc64c4a176cddc17e9a6528`。
- 15 条 mapping 已从 `legacy-conclusion` 升为 reviewed assertion，并绑定当前 commit/testFilter；15 个小 fan-out reuse relation 已审核。高 fan-out shared owner 继续保留待审，不因共享测试自动扩大 exact。
- strict gap **2554→2503**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API servercoretest batch

- 19 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 backtest sync、broker projection、system/strategy contract、settings、onboarding 与 watchlist runtime。
- workspace nextest **23/23 passed**；receipt：`api-transport-servercoretest-reviewed-2026-09-30.json`，digest `sha256:d0f9cbb88920c3bed3fa60b9a47c6fea80a6e277a0b7ad06e844d33bff7f2463`。
- 19 条 mapping 已升为 reviewed assertion 并绑定当前 commit/testFilter；17 个小 fan-out reuse relation 已审核。高 fan-out shared owner 继续保留待审，不因共享测试自动扩大 exact。
- strict gap **2503→2441**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API servercore behavior batch

- 17 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 data-management、live volume、notification、capability catalog、strategy runtime/trading、OpenD health、security 与 strategy delete。
- workspace nextest **33/33 passed**（多 target 的同名 owner 均纳入 receipt）；receipt：`api-transport-servercore-reviewed-2026-09-30.json`，digest `sha256:022cdab8ccecceae349bab2bb53c3a545b4aee938098f712474abbc84edbc1f1`。
- 17 条 mapping 已升为 reviewed assertion 并绑定当前 commit/testFilter；19 个 fan-out ≤6 reuse relation 已审核。高 fan-out shared owner 继续保留待审。
- strict gap **2441→2376**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API marketdataapp behavior batch

- 19 条 API Server/Transport Wire `function_exact` 完成 provider switch、sidecar、health、search、depth/kline 与 subscription 行为复核。
- workspace nextest **22/22 passed**；receipt：`api-transport-marketdataapp-reviewed-2026-09-30.json`，digest `sha256:af682a98cf08d8157b78645a0ea1ce63ce87e8196669be186e1d333de14313bc`。
- 19 条 mapping 升为 reviewed assertion 并绑定当前 commit/testFilter；19 个 fan-out ≤6 reuse relation 已审核，高 fan-out shared owner 继续保留待审。
- strict gap **2376→2308**；全局 strict 仍失败。
## 2026-09-30 API remaining legacy exact closure

- 收口 API Server/Transport Wire 最后 19 条 legacy `function_exact`，覆盖 application、startup、Futu probe、lifecycle、status、combo、web auth 与 settings。
- workspace nextest **25/25 passed**；receipt：`api-transport-remaining-reviewed-2026-09-30.json`，digest `sha256:848b85e4751fa1f1c3addb93b9188624f8de95c12413857c5f367ea7ad4be9e7`。
- API transport owner 的 `legacy-conclusion` exact 已清零；21 个 fan-out ≤6 reuse relation 已审核，高 fan-out 与其他领域 legacy 继续保留。
- strict gap **2308→2244**（reuse 566、receipt 880、assertion 798）；全局 strict 仍失败。
## 2026-09-30 Strategy/Pine parse batch

- P1 `pkg/strategy/pine/parse_test.go` 的 21 条 `function_exact` 完成 Go 断言到 Rust owner 复核，覆盖 parse/analyze/validate、metadata、history、request.security、advanced indicator/order 与 risk declarations。
- workspace nextest **21/21 passed**；receipt：`strategy-pine-parse-reviewed-2026-09-30.json`，digest `sha256:fa86abe492eedff72da09451189fa1e17fd7f07e0bd0059b65ee2efbd77bd483`。
- 21 条 mapping 升为 reviewed assertion；13 个低 fan-out reuse relation 已审核，framework-language 47-way high fan-out 继续 backlog。
- strict gap **2244→2188**；全局 strict 仍失败。
## 2026-09-30 Strategy/Pine live execution batch

- 7 条 P1 Strategy/Pine live execution `function_exact` 完成 Go 断言复核，覆盖 stop/reduce-only、risk reason、instance scope、entry/close sizing 与缺失 quantity 拒绝。
- workspace nextest **19/19 passed**（多 target 实例）；receipt：`strategy-pine-live-execution-reviewed-2026-09-30.json`，digest `sha256:dc4d66ed418c004d700714a02b97e5da22571edb94f878e88eb175d7d1f265c6`。
- strict gap **2188→2172**；Strategy/Pine 仍有 42 条 legacy exact，高 fan-out reuse 继续单独审查。
## 2026-09-30 Strategy/Pine risk and order-boundary batch

- 8 条 Strategy/Pine `function_exact` 完成 risk mode、qualified position、order metadata/trailing boundary 与 truncation 断言复核。
- workspace nextest **10/10 passed**；receipt：`strategy-pine-risk-order-reviewed-2026-09-30.json`，digest `sha256:a8d9603e2f3f9cd70b93eac40093f44326cab698c15a00a7f628552b41606de6`。
- 8 条 mapping 升为 reviewed assertion；5 个低 fan-out reuse relation 已审核；strict gap **2172→2152**。

## 2026-09-30 Strategy/Pine legacy exact closure

- 29 条已有真实 owner 的 Strategy/Pine `function_exact` 已完成 reviewed assertion 与通过 receipt；对应 strict gap **2139→2067**。
- 仍未自动放行高 fan-out shared owner；需要后续把共享引用按 Go assertion 分组复核，再绑定同一批完整 receipt。

## 2026-09-30 API transport envelope/reuse review

- `TestResponseEnvelopeWriters` 已补齐 Rust envelope owner 的 404 `NOT_FOUND/resource not found` 行为断言并升级 exact。
- auth/origin/CSRF/CORS/SSE 的 9 个低 fan-out reuse relation 已完成 reviewed；高 fan-out relation 保留 backlog。
- 本轮严格 gap **2067→2053**；API transport 仍有大量 partial 与高 fan-out reuse，不能以 10.6% 数量比例视为完成。

## 2026-09-30 API logout HTTP projection

- `TestWebLogoutClearsSessionCookie` 已补真实 product HTTP response 的 Set-Cookie 断言，并结合 manager token invalidation 证据升级为 exact。
- auth-session route fixture 的 3-way shared owner reuse 已 reviewed；strict gap **2053→2052**。
- `TestAuthProtectsLogout` 保持 partial：Go 中间件 stub 的 204 与生产 logout endpoint 的 200 是明确边界，不以同名测试强行升级。

## 2026-09-29 批量证据收口与 API fan-out backlog

- 日历 45 条、存储 28 条、设置 16 条 legacy exact 已由真实 owner 测试与通过 receipt 批量升为 reviewed；对应测试分别为 47/47、30/30、19/19。
- API receipt refresh 重新验证 16 个 engine owner 测试并修正 11 条旧短 commit receipt；所有更新 mapping 均包含可执行 testFilter、40 位 verifiedCommit 与 raw NDJSON digest。
- 低 fan-out API relation 已 reviewed 27 条；剩余严格缺口集中在高 fan-out shared owner、历史 receipt 与 assertion review，不能用 relation 数量替代行为审查。
- strict gap 链：**908→818→762→730→689→678**；下一批优先逐组复核高 fan-out owner 的 Go assertion 分组，并补 API Server/Transport 的真实行为测试。

## 2026-09-29 受控 fan-out 与历史 receipt 继续收口

- API runtime 8 条旧 receipt（10 tests）、Assistant 30 条旧 receipt（30 tests）均以当前 40 位 commit 重新验证；未改变任何 partial/boundary 结论。
- system 9 条、backtest 4 条、researchscreen 7 条 legacy exact 已批量升为 reviewed，并分别保存可追溯 NDJSON receipt。
- 仅 3 个经过人工逐引用核对的高 fan-out owner relation 设为 reviewed；instrument search、settings product、watchlist、maintenance 等高 fan-out 仍明确列在 backlog，避免批量证据替代行为审查。
- strict gap 链更新为 **908→818→762→730→689→678→670→640→614→600→595**。

## 2026-09-29 小模块 reviewed 与 API catalog relation

- asset/security/retry/Futu integration 的 16 个 owner 测试全部通过；settings/watchlist/research/desktop 的 18 个 owner 测试全部通过；对应 receipt 已写入 mapping。
- 另有 8 个小模块 mapping 由真实 9-test 批次升为 reviewed；不是 receipt 数量完成率，而是每条 mapping 都绑定自己的行为 testFilter。
- API catalog instrument-search 两个 shared owner relation 经人工逐引用 reviewed；settings product、watchlist 与 maintenance 的高 fan-out relation 保持 unreviewed backlog。
- strict gap 最新为 **509**，剩余以 reuse（高 fan-out）、receipt 和 assertion review 分类处理。

## 2026-09-29 Assertion review closure batch

- 已关闭 13 条 `function_exact` assertion review 缺口：对应真实 Rust owner 均在当前工作树定向 nextest 通过，receipt `assertion-review-2026-09-29.json`。
- strict gap 实际从 **468 降至 455**；当前剩余项分类为 **455 条 shared-owner reuse relation**，需要按 `referenceKeys` 逐引用核对后才能将 `reviewStatus` 升为 `reviewed`。
- 不以测试数量、receipt 数量或 verification passed 代替行为证据；未逐引用核对的高 fan-out owner 保持 backlog。

## 2026-09-29 API/settings shared owner review

- `product_server_persists_ui_settings_and_reports_actual_port` 的 20-way relation 已完成逐 `referenceKeys` 审核并记录 reviewNote；4 条 exact gap 释放，16 条 partial 继续 backlog。
- strict gap **455→451**。下一优先级转向 watchlist read 与 maintenance/API transport owners，继续按全量引用比较后放行。

## 2026-09-29 API/datamigration maintenance owner batch

- 8 个维护 shared-owner relation 已逐 referenceKeys 审核并记录 reviewNote，释放 9 条 exact 引用；partial/boundary 仍保留各自缺口。
- strict gap **451→442**。下一批继续处理 API marketdata forwarding/cache 与 watchlist shared owners。

## 2026-09-29 API/watchlist read owner

- watchlist read owner 的 11-way relation 已逐引用审核，释放 1 条 exact；其余 10 条 partial/boundary 仍需实际行为补齐。
- strict gap **442→441**，继续处理 API marketdata forwarding/cache shared owners。

## 2026-09-29 API marketdata forwarding/cache owners

- 9 个 API marketdata shared owner relation 已逐项审核，释放其 exact references；剩余差异集中在 provider facade 组合、collector/push 分离、TTL/时间语义和 conversion 聚合边界。
- strict gap **441→429**，继续处理 calendar/company/news/index forwarding 与 transport wire owner。

## 2026-09-29 API forwarding-wire owners

- 18 个 API forwarding shared owner relation 已完成逐 referenceKeys 审核，释放 34 条 exact；保留 provider capability、conversion aggregation、collector/push separation 与 sidecar facade partial 差异。
- strict gap **429→395**。剩余重点转向 strategy/runtime shared owner、Assistant shared owner 与 transport lifecycle。

## 2026-09-29 API/runtime tail and candle validation

- API/runtime tail 的 6 个 relation 与 candle adjustment 4-way relation 均逐引用审核；对应 owner tests 8/8 与 1/1 通过，所有 partial/boundary 差异保留。
- strict gap **395→377**。当前剩余以 Assistant/MCP/strategy shared owner 为主，继续按 fan-out 逐组收口。

## 2026-09-30 Assistant MCP policy owner

- MCP loopback peer 与 Host rebinding 的 2 个 shared relation 已 reviewed，释放 4 条 exact；MCP lifecycle manager 的 disabled/port-conflict/transport 分支仍待 owner review。
- strict gap **377→373**。

## 2026-09-30 Assistant shared owners

- claims/lease, tool failure, timeout, approval, workflow threshold 与 continuation supervisor 的 9 个 relation 已 reviewed，释放 18 条 exact；partial failure/stub/long-running 结论未改变。
- strict gap **373→355**，下一批优先处理 MCP/application adapter/tool-catalog shared owners。

## 2026-09-30 Futu/marketdata shared owners

- research/basic quote/batch snapshot/embedded research 的 4 个 relation 已 reviewed，释放 12 条 exact；research catalog 的非法 market 组合仍为 partial。
- strict gap **355→343**。

## 2026-09-30 Futu/engine exact-pair owners

- 21 个 2-way/2-exact relation 已 reviewed，释放 42 条 exact；下一批继续处理高 fan-out mixed partial owner 与 Assistant application/MCP。
- strict gap **343→301**。

## 2026-09-30 Futu triple owners

- 7 个 Futu 3-way relation 已 reviewed，释放 14 条 exact；剩余主要是 Assistant application/MCP 和 mixed partial owner。
- strict gap **301→287**。
## 2026-09-30 Assistant MCP server owner batch

- MCP server owner 的 14 个 shared relation 已逐项对照全部 `referenceKeys`，并以同名 Rust owner 行为测试复核 account/portfolio、strategy/backtest/model、listener、catalog、dependency、unsafe-host 与 workflow wait 结论。
- 定向 engine nextest **14/14 passed**；receipt `assistant-mcp-server-owner-reviewed-2026-09-30.json`，digest `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`。
- strict gap **287→272**。HTTP fetch、schema、product dispatch 及其他高 fan-out owner 仍保留 backlog；未用 receipt 数量替代行为覆盖判断。

## 2026-09-30 Assistant application/tool-catalog owner batch

- 16 个 fan-out ≤4 的 application/tool-catalog relation 已逐项对照 `referenceKeys` 并运行对应 owner：workflow、execution/trade、market candle/backtest、portfolio/research、optimization、catalog、capability 与 instrument boundary。
- 定向 engine nextest **17/17 passed**；receipt `assistant-application-owner-reviewed-2026-09-30.json`，digest `sha256:9d64884dbd4dfa97317b1272f9a616f8980daf39aea3b663c90748e49c5b3c69`。
- strict gap **272→256**。10-way strategy binding、ADK runtime 高 fan-out 与 mixed partial 继续 backlog；未用 receipt 数量替代行为覆盖判断。

## 2026-09-30 ADK runtime pair owner batch

- 20 个 fan-out=2 的 ADK runtime relation 已逐项对照 `referenceKeys` 并运行 input/approval/lease/projection/terminal/session owner；高 fan-out runtime/store relation 保留 backlog。
- 定向 engine nextest **20/20 passed**；receipt `assistant-adk-runtime-pairs-reviewed-2026-09-30.json`，digest `sha256:4a81b9bc4b2324ea45c9ed00b94198fe11e0766718c7e061596c2136ce0af21a`。
- strict gap **256→236**，仍只按实际行为、reviewed relation、有效 receipt 计数。

## 2026-09-30 API route pair owner batch

- 20 个 fan-out=2 的 API/Transport relation 已逐项对照 `referenceKeys` 并运行 execution/system/marketdata/Futu route owner；partial/provider 边界保持原结论。
- 定向 workspace nextest **20/20 passed**；receipt `api-route-pairs-reviewed-2026-09-30.json`，digest `sha256:5e63078e658a06423b3c1b0453baeb53f72de24a2e26a4da3faea60c003c084c`。
- strict gap **236→216**，未用测试数量或 receipt 数量替代行为覆盖判断。

## 2026-09-30 API route tail owner batch

- Futu notification/quote-right labels 与 settings backtest-provider atomic preparation 两个 fan-out=2 relation 已逐项复核。
- 定向 workspace nextest **2/2 passed**；receipt `api-route-pairs-tail-reviewed-2026-09-30.json`，digest `sha256:a65d2d16e2e20bb0f2a9e74fdb690e8733f1cf4623ddc3da7c1f92c38f5c00d6`。
- strict gap **216→214**，其他 API 高 fan-out 与 mixed partial 保留 backlog。

## 2026-09-30 API route triple-owner batch

- 14 个 fan-out=3 的 API/Transport relation 已逐项对照 `referenceKeys` 并运行 execution/marketdata/trade/Futu/maintenance/watchlist owner。
- 定向 workspace nextest **14/14 passed**；receipt `api-route-triples-reviewed-2026-09-30.json`，digest `sha256:5548a060148522457f38e479d3d4d3ef8efa868c9137f035ccf75a79d75585ad`。
- strict gap **214→200**，higher fan-out 与 mixed partial 仍保留 backlog。

## 2026-09-30 ADK runtime triple-owner batch

- 13 个 fan-out=3 的 ADK runtime relation 已逐项对照 `referenceKeys` 并运行 input/turn/expiry/fencing/gate/terminal/handoff/session owner。
- 定向 engine nextest **13/13 passed**；receipt `assistant-adk-runtime-triples-reviewed-2026-09-30.json`，digest `sha256:40a8bbf45d8b4d060d82e0b10b6ba4240623818c154fc9145587541e96f1a333`。
- strict gap **200→187**，higher fan-out runtime/store relation继续 backlog。

## 2026-09-30 Assistant claims/runtime owner batch

- 6 个 `jftrade-assistant` fan-out=2–4 relation 已逐项对照 `referenceKeys` 并运行 workflow/claims/runtime owner。
- 定向 workspace nextest **6/6 passed**；receipt `assistant-claims-owners-reviewed-2026-09-30.json`，digest `sha256:d100bdc4c3bf6e5e5c77e48d8db495259f294334cae5137a63752f23cd182cf3`。
- strict gap **187→181**，高 fan-out ADK/store relation继续 backlog。

## 2026-09-30 ADK store owner batch

- 7 个 SQLite ADK store fan-out≤4 relation 已逐项对照 `referenceKeys` 并运行 atomic projection/provider/artifact/session/writer/approval owner。
- 定向 workspace nextest **7/7 passed**；receipt `adk-store-owners-reviewed-2026-09-30.json`，digest `sha256:98fd30e706a5cae7d009a6e00852927d73aaee0352a081f637926548e9e22885`。
- strict gap **181→174**，高 fan-out store relation继续 backlog。

## 2026-09-30 Futu/OpenD pair owner batch

- 20 个 Futu/OpenD fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 quote/health/search/kline/session/order-book owner。
- 定向 `jftrade-integration-futu` nextest **20/20 passed**；receipt `futu-opend-pairs-reviewed-2026-09-30.json`，digest `sha256:687a59f3da757e1fbf5c57147f130f111fc42f726855149bd81cad3f625482a1`。
- strict gap **174→154**，其余 Futu 高 fan-out与mixed partial保留 backlog。

## 2026-09-30 Futu/OpenD pair owner batch 2

- 第二组 20 个 Futu/OpenD fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 quote-rights/snapshot/subscription/trade/watchlist/recovery/prediction owner。
- 定向 `jftrade-integration-futu` nextest **20/20 passed**；receipt `futu-opend-pairs-2-reviewed-2026-09-30.json`，digest `sha256:2c24a6e94b730a3dad00c289924da912d36acdfec2e161f307f1f47fc0f56a70`。
- strict gap **154→134**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Futu/OpenD pair tail owner

- user security group type encoding/projection 的最后一个 Futu/OpenD fan-out=2 relation 已逐项复核。
- 定向 `jftrade-integration-futu` nextest **1/1 passed**；receipt `futu-opend-pair-tail-reviewed-2026-09-30.json`，digest `sha256:1f56d2b79405bbad59b530e99ddcb46d9ac8e5589a56c759a6b1636dd90c2290`。
- strict gap **134→133**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Cross-domain pair owner batch

- 跨领域 20 个 fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 calendar/ADK/execution/marketdata/research/capability owner。
- 定向 workspace nextest **20/20 passed**；receipt `cross-domain-pairs-reviewed-2026-09-30.json`，digest `sha256:11200ac86d98093d5f2a7439a503bd0412ad759a418ff2bfaa4dbbbed90f49d8`。
- strict gap **133→113**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Futu/OpenD triple owner batch 2

- `referenceCount=3` 的 16 个 Futu/OpenD、marketdata、SQLite relation 已逐引用复核并升为 `reviewed`；只释放对应 16 条 `function_exact` exact reuse，未改变已有 partial/boundary 结论。
- 定向 workspace nextest **16/16 passed**；receipt `futu-opend-triples-2-reviewed-2026-09-30.json`；raw output digest `sha256:7564322fb6753e5a1262f184c00d6d07694179d6a6f606f295dd7c648e7f2933`。
- strict audit **72→56**；剩余 56 条为未审核的高 fan-out/shared owner relation，继续按 referenceCount 分批处理。测试数量比例、receipt 数量和 verification passed 不计为完成率。

## 2026-09-30 Mixed high-fanout owner batches

- 两批共 40 个 shared owner relation 完成真实 owner 复核：第一批 nextest **21/21 passed**、释放 25 条 exact（strict **56→31**）；第二批 **20/20 passed**、释放 20 条 exact（strict **31→11**）。receipt 分别为 `mixed-highfanout-owners-reviewed-2026-09-30.json` 与 `mixed-highfanout-owners-2-reviewed-2026-09-30.json`。
- 最后一批 Futu/research 11 个 owner **11/11 passed**，receipt `final-futu-research-owners-reviewed-2026-09-30.json`，strict **11→0**。此处的完成条件是逐项行为证据收口，不是测试数量或 receipt 数量。

## 2026-09-30 API transport SPA boundary

- `frontend_spa_fallback_respects_path_and_accept_boundaries` 先红后修：旧 router 对 `Accept: application/json` 仍返回 SPA，修复后按路径与 Accept 矩阵返回 HTML/404；`TestShouldServeFrontendIndexRequestBoundaries` 已升级为 reviewed `function_exact`。
- receipt `api-transport-spa-boundary-reviewed-2026-09-30.json`，nextest **1/1 passed**，strict 保持全量通过；API 数量比 10.7% 仍只是风险信号。

## 2026-09-29 API provider-test wire batch

- `TestProviderAndAgentValidationContracts` 的 provider probe 路由已补真实 HTTP 行为证据：默认 quick、full、invalid slow 与 missing-provider 502 envelope 均逐项断言。
- 先红后修未知 provider 状态码，定向 nextest **4/4 passed**；receipt `api-provider-test-wire-reviewed-2026-09-29.json`，digest `sha256:b4ce4838b0f6ebe37e9474caabda06913c14ab226e3fbcc7ba332fd324ee6800`。
- 该 partial 已升为 `function_exact`；strict function_exact 证据从 **1492→1493**，strict audit 通过。剩余 API transport backlog 继续按真实 wire 差异推进。

## 2026-09-30 API assistant chat/SSE wire batch

- `TestChatAndSSEContracts` 已补齐 chat JSON envelope 与成功 SSE wire 行为：真实 production composition 断言 `200 + ok=true`、`text/event-stream`、配置化 `X-ADK-Stream-Idle-Timeout-Ms=420000`，以及 session→run→terminal 顺序与 durable session id。
- provider fixture 使用两条 loopback Responses 连接；provider failure 的 JSON 投影不与成功流头部混用。定向 engine nextest **1/1 passed**；receipt `api-assistant-chat-sse-contract-reviewed-2026-09-30.json`，digest `sha256:ef17475a3ca52c8d11254d0a921de827174661872903d9d93a6ea34a09c353f8`。
- partial→`function_exact`，strict function_exact 证据 **1493→1494**；strict audit 通过。剩余 dispatcher/WS/backtest boundary partial 继续保留并按行为缺口推进。

## 2026-09-30 API live WebSocket shutdown lifecycle batch

- `TestHandlerConnectionLimitAndCloseLifecycle` 已由两个真实 owner 测试覆盖：`ws_live_transport_rejects_origin_and_limit_without_leaking_permits` 锁定 503 limit、permit release 与再次握手，`ws_live_shutdown_closes_active_connection_and_releases_depth_subscription` 锁定活动连接 close、`1001` 原因、连接清零和 depth demand 释放。
- 红测先发现测试夹具不支持 extended-length masked frame，随后补齐标准长度编码；第二次红测发现 shutdown 前可能有 queued text frame，测试改为排空后检查 close。生产路径最终 **2/2 passed**；receipt `api-live-handler-close-lifecycle-reviewed-2026-09-30.json`，digest `sha256:14c38cbc91d70bb6b9a627c8fddd509294d9899c7a119ba0bf6c20079930b181`。
- partial→`function_exact`；strict function_exact **1494→1495**，partial **2322→2321**。同一 limit owner 与 servercore partial 的多引用 reuse 已显式标记 reviewed，未扩大 servercore 的诊断字段结论。
## 2026-09-30 API bindings required-path wire batch

| BindURI required path + escape matrix | 1 mapping（由 partial 升 function_exact） | `product_query::uri_escape_validation_accepts_literal_percent_and_rejects_malformed` + `transport_contracts::missing_path_parameter_returns_not_found_json_without_dispatch` | API nextest 3/3 passed；receipt `api-bindings-wire-exact-reviewed-2026-09-30.json`；digest `sha256:df775fcfd9a151371616e8dfe64aef9996d0f1c84088b7d4925d8cd6eba2ec0f` | 先红后修确认认证先于路由；已认证缺参请求断言 404 JSON envelope 与 port 不调用，strict function_exact **1495→1496**，数量比例不作为完成率 |
## 2026-09-30 Daily candle assertion review

- 冻结 Go `market_http_test.go:91` 的断言逐项核对：成功返回一根 daily candle，candle 与 meta 均省略 session，meta.extendedHours=false。生产 read owner 测试补齐 array length、meta.session 缺省和 extendedHours=false；首次即绿，无生产差异修复。
- 定向 nextest 1/1 通过；mapping 从 partial 升为 reviewed function_exact，partial 2321→2320，exact 1496→1497；strict audit 必须以本轮实际输出确认。receipt `api-daily-candle-reviewed-2026-09-30.json` 文件 digest `sha256:81066e73e3ad596e37e434d16a68862032bf1e46f5b7f1ac386fede7d975b832`。
- 修正上一轮 bindings mapping 的 receiptDigest：日志 rawOutputSha256 仅描述日志，receiptDigest 应为 receipt 文件 SHA-256（`b0b57dcbc9ccdaa5869ff5c59955dc4cc97f97a109dfc0bb59f83dbe6b9bfad0`）。
- 全量 check:rust 本轮仍在 target-health 失败（至少 50000 rcgu.o）；check:quick 实际执行后在 check:zero-go 失败。两项均未记为通过。
## 2026-09-30 System hard-stop handler boundary review

- 冻结 Go `routes_test.go:102` 直接调用 handler；DELETE 请求 URL 不代表注册路由。Go 与 Rust 公开 release 路由均为 POST `/{hardStopId}/release`。修正原映射把 handler seam 差异当作公开 route 差异的结论。
- Rust dispatch 增加 `%20/%20%20/%09` 空白 id 回归：400、BAD_REQUEST、hard stop id is required，零 port 调用；已有 quantity/notional=0 拒绝保留。新断言首次通过，无生产差异。
- 定向 nextest 1/1 通过，receipt `api-system-blank-hard-stop-reviewed-2026-09-30.json`（文件 digest `aabb091fc34e9a5dc1fff7211d4291cb6aca91d450bdd2fa4dcfe28d6ff21ac0`）。assertionCoverage 从 legacy-conclusion 升 reviewed；独立无 param handler seam 保留 partial，不宣称 exact 或整体完成。
- 本批 strict gap 没有下降；实质变化是空白 id 防御行为被断言、错误差异结论被纠正。后续需继续模块批量补 wire 行为，而非以 receipt 或通过数量计完成率。
## 2026-09-30 Execution ETH session route batch

- 对照 Go `exec_validate_test.go:88` 的 HTTP status、ETH session、fillOutsideRTH 三项断言，新增真实 Product POST 测试锁定 200 与 raw session；既有 parser/to_trade_request/Futu wire 测试锁定 ETH→session=2、fillOutsideRTH=true 及协议编码。
- 先红后修核对 owner：HTTP write port 接收 raw payload，不能错误要求其携带下游计算字段 `fillOutsideRTH`；移除错误断言后组合测试 **3/3 passed**。receipt `api-execution-eth-session-wire-reviewed-2026-09-30.json`，文件 digest `sha256:ef814b566fa23db1756adfc112360616db9f75b2193b496ee280ff98aad9086c`。
- mapping 从 partial 升 reviewed function_exact，strict function_exact **1497→1498**，partial **2320→2319**（随后 daily candle 批次已将 partial 更新为 **2318**）；数量比例不作为完成率。
## 2026-09-30 Assistant catalog HTTP composition batch

- `TestCatalogSessionRunAndObservabilityContracts` 由 port-level partial 补为真实 Product HTTP composition：同一 ProductionAdkPort seed 下逐一请求 12 个 catalog/session/run/observability GET 路由，断言 HTTP 200 + `ok=true`，并断言 DELETE provider 200 + `ok=true`。
- 定向 nextest 1/1 passed；receipt `api-adk-catalog-http-reviewed-2026-09-30.json`，文件 digest `sha256:8249f1afebb9345f3c0a4bb8fefb4eab02ac313e3706f75b39fdc26e5283e383`。
- mapping 从 partial 升 `function_exact`，strict function_exact **1498→1499**，partial 实际 **2318→2317**；数量比例不作为完成率。
