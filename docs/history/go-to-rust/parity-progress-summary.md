# Go → Rust 对齐成果摘要

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

更新时间：2026-09-29 13:40 UTC。本文是迁移期工作摘要，不替代架构事实、门禁结果或发布资格。

## 当前基线

| 项目 | 数值 | 解释 |
| --- | ---: | --- |
| Go 测试候选 | 4451 | 冻结基线 `go:452dea11` |
| Rust 测试 | 3404 | 数量不代表行为等价 |
| `function_exact` | 1501 | reviewed assertion、anchor、reuse 与 receipt 审计通过；不能代替全量门禁 |
| `partial` | 2315 | 只覆盖部分断言，不能视为完成 |
| `boundary` | 635 | 当前架构边界或没有同形对象 |
| 重复映射 | 0 | 审计脚本结果 |
| Parity 锚点 | 1899 / 1852 / 0 / 0 / 47 | unique / recorded / unrecorded / stale / unknown |

本轮追加批次（2026-09-29）：API legacy partial/boundary provenance 批量升为 reviewed，8 个共享 relation 完成引用审查；broker/market-rule 12 个 nextest 通过；Futu watchlist 7 个 nextest 通过并收口 6 条 exact。strict gap 实际 **2041→2036→2010→1998**；仍未通过 strict，未把数量或 receipt 当完成率。

本轮严格证据批次：Pine client validation 从 partial 升为 reviewed `function_exact`，并修正 non-finite candle 的 owner 复用；API runtime/market snapshot/WebSocket 与 desktop readiness 8 条 exact 绑定 7 个真实 nextest（7/7 passed），另有 Pine indicator 2 条 exact 绑定 2/2 passed。共享 owner reuse 关系按引用逐项复核。strict gap 实际 **2052→2047→2046→2041**；当前 strict 仍失败，未把测试总数、receipt 数量或 verification passed 当作完成率。

本轮 API Server / Transport Wire 尾项复核：`TestRecorderBoundsErrorsSlowRequestsAndOpenDHealth` 对应 `jftrade-api` observability owner 真实 nextest 通过（与 WebSocket owner 一并运行 2/2）；该条补齐 reviewed assertion 与完整 receipt `sha256:9fa21653293b25a5387a6e258ea46c4d0a7fda346e4e40f018408ad33b8ae02a`。strict gap **2565→2563**，仅按行为测试、reviewed assertion 与有效 receipt 计入；全局严格审计仍未通过。

同轮 API runtime 批量收口 3 条已有 exact：环境路径覆盖、相对 settings 路径派生、strategy preview symbol/session warmup。4 个 Rust owner nextest **4/4 passed**，统一 receipt `sha256:48d34e9714e6c67134a3a60cd34c18276898c03791c202e5e4a156ddd37d4b07`；三条 mapping 的 assertion/reuse/receipt 证据完成 reviewed，strict gap **2563→2557**。没有把测试总数或 receipt 数量当作完成率。

API frontend asset 批次新增真实 `/`、`/assets/app.js`、`/missing.json` wire 断言，并为两条 Go frontend 映射建立独立 Rust provenance owner；3/3 nextest 通过。该批是 partial→exact 的行为覆盖增加，严格缺口净值保持不变，未将其冒充 strict 收口。随后为 `TestClientRegistryTracksActiveInstruments` 增加独立 owner，联合 API tail 复跑 4/4 通过并绑定同一工作树 receipt `sha256:8c885bf0c5b09e5243b1ef111134b9805bf761ded361160755046f4bb1417eb2`；strict gap **2557→2554**。

## 已完成的工作

- 建立了 4451 条 Go 测试逐项映射清单，按文件、行号、测试名记录，避免只按同名测试判断。
- `strategy_pine` 的 385 条 `partial` 已逐条复核完；结论仍是 `partial`，没有把聚合测试冒充 exact。
- `api_transport` P1 的 72 条 `partial` 已逐条复核完；结论仍是 `partial`。
- `internal/marketdata` partial 第 1–30 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`，精准 Rust 证据 38/38 通过，没有新增生产修复。
- `internal/marketdata` partial 第 31–60 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`/`boundary`，并新增一个先红后修的 resolver 回归测试，锁定 provider full-window 后再应用公开 limit。
- `internal/marketdata` façade 剩余 34 条已逐项读取 Go 断言并核对 Rust owner；34/34 保持 `partial`，本批受影响 crate nextest 1993/1993 通过。helper provider 的 `snapshot-poll-delayed`/`snapshot-poll-fallback` 状态已由生产 owner 回归测试锁定。

## 2026-09-28 P1：Futu session/market-rules strict evidence

复核并收口 5 条已有 `function_exact` 的 P1 映射：关闭缓存 OpenD client 后的会话重建、保证金比率 TTL 缓存、撤单 `Trd_ModifyOrder` 协议、市场规则主/备来源失败矩阵，以及闭市 US previous-close 投影。Go 断言逐项对照 Rust owner，补齐 session coordinator 的 `// Parity:` 锚点；没有发现需要先红后修的真实功能差异。

定向 nextest 实际 **10/10 passed**（`jftrade-integration-futu` 7、`jftrade-engine` 3；live OpenD 1 ignored），receipt：`sha256:b290cd212da3374c17198d541103085cf5a0b71fedc8f0d1271732534f097167`（`verification-receipts/p1-futu-session-market-rules-2026-09-28T071432Z.json`）。五条 mapping 已写入 reviewed assertion、verification receipt、空 blocker 与共享测试复用审查；strict gap 按实际审计由 3561 降至 **3546**，全局严格审计仍未通过。
- `subscriptions_test.go` 4 条与 `quote_availability_test.go` 2 条已逐项读取 Go 断言并核对 Rust owner；6/6 保持 `partial`，新增 authoritative quote 缺失字段与 legacy zero projection 回归测试，精准 nextest 11/11 通过。Rust 没有 LiveTickJSON/LatestTicksJSON 与 Go `SnapshotJSON` 同形 helper，因此未升级为 `function_exact`。

## 2026-09-28 Trading/Broker cancel rejection P1

`internal/trading/broker_conformance_test.go:58:TestFakeBrokerConformanceCancelAcceptedAndCancelRejected` 已完成真实行为对齐：先红复现明确券商拒单被写成 `UNKNOWN`，修复后保留 `CANCEL_SUBMITTED`（对应 Go `CANCEL_REQUESTED`），写入 `lastErrorSource=broker.cancel` 并追加 `BROKER_CANCEL_REJECTED`。连接丢失仍保持 UNKNOWN fail-closed。组合回归（受理收敛、终态/未识别拒绝、券商明确拒单）定向 nextest 3/3 通过；红/绿 receipt 分别为 `p1-broker-cancel-rejection-red-2026-09-28T184300Z.json` 与 `p1-broker-cancel-rejection-2026-09-28T184500Z.json`。映射已升级为 `function_exact`。

本轮 parity 审计（2026-09-29 targeted cancel alias 后）：Go 4451、Rust 3394、`function_exact=1506`、`partial=2311`、`boundary=634`、`missing=0`；anchor reconcile 为 1805 unique、1758 recorded、0 unrecorded、0 stale、47 unknown。`audit_test_parity.py --write-report` 与 anchor reconcile 通过；`audit_test_parity.py --strict` 仍真实失败，当前 3550 个历史 evidence/receipt gaps，未宣称全局完成。

## 2026-09-28 Strategy/Pine manager close drain P1

`internal/strategy/pineruntime/runtime_failure_contracts_test.go:171:TestManagerCloseDrainsActiveLiveSession` 新增 engine 级回归 `manager_shutdown_closes_active_pine_sessions_once`。真实运行时启动并打开 Pine session 后执行 manager shutdown，断言 cancel/join 完成、恰好一次 close、session identity 保持且 revision 连续；先红阶段发现测试 fixture 提前释放 store 导致任务无法取得 weak store，保留 store owner 后定向 nextest 2/2 通过。receipt `sha256:7fd9c2be86d54c740663edd6da3de92f391c27925949a5774809020f7ad558f0`，映射升级为 `function_exact`；严格全局审计仍未通过。
- `lifecycle_boundaries_test.go` 余量 7 条已逐项读取 Go 断言并核对 Rust owner；7/7 保持 `partial`，精准 nextest 16/16 通过，没有新增生产修复。Cache、subscription、service、collector 与 resolver 的聚合 fixture 差异继续按 owner/架构边界记录。
- Assistant chat stream helper/recovery 与断线重连 5 条已逐项读取 Go 断言并核对 Rust/SSE owner；5/5 保持 `boundary`/`partial`，精准 nextest 5/5 与 SSE/重连证据 4/4 通过。内存 hub clone、toolGroup、失败 writer 单写次数等旧 owner 行为按边界保留。
- `internal/api/backtest/routes_progress_test.go:71` 已逐项读取 Go 断言并核对 Rust sync owner；保持 `partial`，精准 nextest 2/2 通过。Rust 已覆盖持久化 task 读回与 cancel 成功/缺失语义，HTTP progress 双态仍缺同形冻结路由语料。
- HTTP bindings 的 `ParseQueryTime` fallback、`BindURI` escape/binding 和 candle period/pagination 三条 P1 已逐项核对；3/3 保持 `partial`，精准 nextest 7/7 通过。UTC/URI/candle 核心规则已有 Rust owner，caller fallback、Gin binding seam 与纯分页 helper 仍保留差异；本批复核了带空白 period、blank limit、非整数 limit 和 route 默认窗口语义，负 limit/offset 的 Go helper 钳制在 Rust typed query 边界不可达。
- `internal/api/live/dispatcher_boundaries_test.go:205:TestDispatcherEnvelopeDefaultsAndMapFallback` 已逐项核对；保持 `partial`，Rust SSE frame 精准回归 1/1 通过。dispatcher envelope 缺省字段与 mapString fallback 是 Go helper seam，Rust 事件生产者直接生成完整 typed JSON，没有同形通用 helper，未把 frame 形状测试升级为 exact。
- `internal/api/marketdata/routes_test.go:147:TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 已逐项核对；保持 `partial`，Rust 显式 broker 证据 2/2 通过（`cargo-nextest` targeted）。Rust active-provider owner 与 Go 独立 broker reader 调用列表不同，按架构边界保留四路序列差异；下一 P1 为 `internal/api/settings/routes_market_data_test.go:63:TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure`。
- `internal/api/settings/routes_market_data_test.go:63:TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure` 已逐项核对；保持 `partial`，Rust engine/settings 证据 3/3 通过（`cargo-nextest` targeted）。provider catalog、prepare-before-persist 与旧值保持已有 owner 断言，HTTP 409 envelope/GET 回读仍是跨 owner 聚合差异；下一未收尾 P1 为 `internal/api/trading/execution_validation_contracts_test.go:238:TestExecutionOrderDetailsRouteMapsMissingAndStoreFailures`（system route validator 已在下方记录）。
- `internal/api/trading/execution_validation_contracts_test.go:238:TestExecutionOrderDetailsRouteMapsMissingAndStoreFailures` 已逐项读取 Go 的缺失订单、store 失败和空 ID 断言；保持 `partial`，Rust execution-read 冻结语料与上游失败透传回归精准 nextest 2/2 通过。缺失订单与 `GET_ORDER_FAILED` 的 HTTP 投影已有逐条证据，空 ID 的 Go 400 与 Rust typed route 404 不匹配属于路由形状边界。
- `internal/api/trading/routes_test.go:32:TestBrokerRoutesPreserveFallbackSemanticsAndRequestValidation` 已逐项读取 Go 的 broker fallback、portfolio cash、缺参、写单与空 runtime 断言；保持 `partial`，Rust broker/portfolio owner、HTTP request validation、empty collection、canonical route unavailable 与 runtime source guard 精准 nextest 5/5 通过。Go no-active-broker 的 200 degraded/空态在 Rust external-unavailable owner 中是 503/Unavailable，且 order-fees/cash-flows 缺参与 ib 空态没有同形逐项 Rust 断言，按生产 transport 边界保留差异；下一未收尾 P1 为 `internal/app/apiserver/backtestapp/historical_source_test.go:62:TestProviderHistoricalSourceMapsExtendedSessionsToProviderCapabilities`。
- 本批门禁已复核：`pnpm run check:quick` 通过；完整 `pnpm run check:rust` 首次被既有 `product_api_launcher_lifecycle` 的 `None`/`Some(0)` 退出码断言打断，定向 `--retries 2` 复核 1/1 通过，随后完整 workspace nextest 3464/3464（2 skipped）与 7 个 compatibility replay 全部通过。映射审计为 0 重复 exact、0 nonexistent crate、0 unrecorded/stale anchor，报告与 inventory 已重生成。
- `internal/api/system/routes_test.go:61:TestSystemManualRetryRouteCallsReset` 已逐项读取 Go 断言并补 Rust 专用回归；升级为 `function_exact`，精准 nextest 4/4 通过。Rust 明确断言 manual-retry 返回 accepted=true 且 SystemWritePort 的 ManualRetry operation 只调用一次。
- `internal/api/system/routes_test.go:77:TestExchangeCalendarRefreshRouteCallsRefresh` 已逐项读取 Go 断言并补无 market refresh 路由回归；保持 `partial`，精准 nextest 3/3 通过。Rust 通过真实 CalendarManager 断言 `/refresh/US` 与 `/refresh` 均 accepted/updated 并可由 status 读回，Go 注入 callback 的 market 参数与调用次数 seam 继续保留差异。
- `internal/api/system/routes_test.go:102:TestSystemRouteBoundaryValidatorsRejectMissingHardStopAndNonPositiveLimits` 已逐项读取 Go 断言并补 Rust 专用回归；保持 `partial`。Rust 直接断言 `maxOrderQuantity=0` 与 `maxOrderNotional=0` 在端口调用前返回 400/BAD_REQUEST；Go 缺 hard-stop ID 的 DELETE handler 返回 400，而 Rust 带 ID 的 POST release 模板无法匹配时返回 404/NOT_FOUND，作为路由形状边界保留。
- 本批精准回归 2/2 通过。`pnpm run check:quick` 完整通过；`pnpm run check:rust` 首次运行在既有 `adk_session_detail_omits_resolved_approval_groups` 异步时间线断言处失败（733/734 已通过），随后以 nextest `--retries 2` 单测复核通过。该失败与本批 system-write 变更无关，保留失败证据，不将完整门禁记为通过。
- `internal/api/system/routes_test.go:121:TestRealTradeReleaseRoutesRejectMalformedOptionalPayloadBeforeStateChange` 已逐项读取 Go 断言并补三路由 Rust 专用回归；保持 `partial`。hard-stop release、kill-switch release、risk disable 的畸形载荷均返回 400/BAD_REQUEST，且未触发 SystemWritePort；Gin registration 与 service callback owner 形状继续按边界记录。
- 本批精准回归 3/3、`check:quick` engine 1937/1937 与完整 `check:rust` workspace 3451/3451（2 skipped）均通过；7 个兼容回放全部通过。
- `internal/api/system/routes_test.go:170:TestExchangeCalendarProbeRouteCallsProbe` 已逐项读取 Go 断言并核对真实 CalendarManager probe owner；保持 `partial`。Rust 已断言 `/probe/US` 返回 accepted/healthy/checksum，并覆盖无 manager 与未知 market 边界；Go callback 的 HK 参数透传和一次调用次数没有 Rust 同形 spy，继续记录差异。
- 本批 probe 定向回归 3/3、`check:quick` policy 与完整 `check:rust` workspace 3451/3451（2 skipped）均通过；7 个兼容回放全部通过。
- `internal/api/system/routes_test.go:191:TestRealTradeControlRoutesDelegateStateChanges` 已逐项读取 Go 断言并补真实产品 HTTP 写入→GET 读回回归；保持 `partial`。Rust 复用生产 `ExecutionRiskCoordinator` 和控制文件 owner，覆盖风险限额、kill switch、hard stop 的写后状态读回；Go Gin callback spy、调用次数与固定 `hs-1` ID 没有同形 Rust 断言，继续记录为差异。
- 本批专用回归与 wire 聚合回放 2/2、`check:quick` Rust/desktop 1968/1968、完整 `check:rust` workspace 3452/3452（2 skipped）均通过；compatibility replays、clippy、格式和 Pine worker 98/98 均通过。映射审计保持 0 重复 exact、0 stale/unrecorded anchor。
- 本批门禁：受影响 `check:quick` 与完整 `check:rust` 均被既有 `product_api_launcher_lifecycle` sidecar 退出码 `None`/`Some(0)` 间歇性断言打断；两个失败用例随后以 nextest `--retries 2` 复核均通过。该 flaky 只涉及 launcher 生命周期，不涉及本批 system-write 变更。
- `internal/api/system/routes_test.go:316:TestRealTradeControlRoutesMapValidationAndControlFailures` 已逐项读取 Go 断言并补 Rust 专用错误映射回归；保持 `partial`。Rust 逐条覆盖三条 malformed JSON、缺少正向 runtime limit 的 `400/BAD_REQUEST`，以及 kill-switch、hard-stop、runtime-risk enable/disable 六条 `409/REAL_TRADE_CONTROL_FAILED`，并断言校验失败不触发 port、控制失败各调用一次；Gin/service callback 委派形状继续按边界保留。
- `internal/api/system/routes_test.go:19:TestSystemRoutesReturnEnvelopes` 已逐项核对 16 条系统读路由；保持 `partial`。既有 system-read fixture、control-read、product server 与 CalendarManager 回归共同覆盖 futu-opend、worker、status、runtime-dependencies、storage、calendar 及控制读族的成功 envelope；未合并为单一 Gin 必填键表，继续记录 owner/路由聚合差异。
- 本批专用错误映射回归 2/2、系统读 envelope 证据 4/4 通过；完整 `check:rust` workspace nextest 3453 项完成且 compatibility replays 全通过。`check:quick` 在既有 `product_api_launcher_lifecycle::api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 处失败，随后单测重跑仍复现 `status.code() = None`（非本批 system-write/system-read 变更）；保留失败证据，不记为 quick 通过。映射审计 0 重复 exact、0 nonexistent crate、0 unrecorded/stale anchor，锚点 1754/1708/0/0/46。
- `internal/api/system/status_mapper_test.go:11:TestSystemStatusTransportMapperPreservesDomainJSON` 已逐项读取 Go mapper 与 Rust system status owner；保持 `partial`。Rust 锚定稳定字段和成功 envelope 回归；复核确认 Go 测试只把同一个 typed `Status` 分别编码后比较，未构造未知顶层字段，Rust 没有可拆出的第二层 mapper，未发现真实功能差异。
- 本批状态 mapper 定向回归 2/2、`check:quick` 受影响 Rust/desktop 1969/1969 与完整 `check:rust` workspace 3453 项均通过；7 个 compatibility replays 与 Pine worker 98/98 全部通过。映射审计 0 重复 exact、0 nonexistent crate、0 unrecorded/stale anchor，锚点 1755/1709/0/0/46。
- `internal/api/assistant/chat_helpers_test.go:13:TestTimelineStreamStateTracksSessionRunAndToolTiming` 已逐项读取 Go 的内存 timeline/hub 状态断言；保持 `boundary`。Rust 没有同形 `adkTimelineStreamState`，本批以 durable stream 重放和 production session→run→final 顺序验证外部事件契约，精准 nextest 2/2 通过。
- `internal/api/assistant/chat_helpers_test.go:260:TestAssistantRequestHelpersCoverInvalidAndBoundaryInputs` 已逐项读取 Go 的 ADK validation 白名单与 bearerToken 边界。先写 Rust 表驱动回归并复现小写 `bearer` 返回空值的红测，再由 `crates/jftrade-api/src/auth.rs::request_bearer_token` 修复为大小写不敏感 scheme；修复后 API auth 与 ADK validation 精准 nextest 2/2 通过。映射保持 `partial`，记录 Go 空字符串与 Rust `Option<&str>` 及跨 owner 差异。
- `internal/api/assistant/chat_helpers_test.go:167:TestChatStreamExecutionPublishesDeltaAndFinalVariants` 已逐项读取 Go 的 delta 分类与 `publishFinal` 断言；先红后修确认 Rust Stream 终帧错误暴露完整 tool output，随后在 `product_adk_model_runtime_stream_success.rs` 只对 Stream terminal response 移除 `toolCalls[].output`，保留 durable run activity。终帧裁剪、终态恢复、重连 replay 与 live session/run/final 精准 nextest 4/4 通过；Hub 内存 delta 分类仍保持 partial。
- `internal/api/assistant/chat_helpers_test.go:216:TestExecuteADKChatStreamPublishesTerminalErrorForInvalidRequest` 已逐项读取 Go 的非法请求、有效 preview 与未知 agent preview 断言；新增真实 product Stream 回归，确认未知 agent fail-closed 且不产生 session/run 预览帧，非法 JSON 与有效 session→run 顺序证据一并复核，精准 nextest 3/3 通过。Go `sessionSent` 进程内布尔状态继续作为 transport owner 边界记录。
- `internal/api/assistant/chat_stream_recovery_contracts_test.go:41:TestChatStreamExecutionReusesKnownContextAndRecoversTerminalRun` 已补真实生产流回归；已有 session 请求只发布一帧 session preview，随后进入 run，持久化终态恢复仍只出 final。精准 nextest 3/3 通过；Go `contextSent` 内存 execution seam 继续保持 partial。
- `internal/assistant/model/provider_reasoning_config_test.go:8` 与 `internal/assistant/engine/persistence/provider_reasoning_test.go:47` 已发现真实 provider 归一差异：Rust 先红测确认缺省 provider 没有 reasoningConfig，随后在 provider projection/mutation owner 补 `reasoning.effort` 默认字段、显式空 mappings、trim/排序和重复/保留字/未知档位/空值校验。精准 nextest 2/2 通过；provider 级 ResolveProviderReasoning 尚未接入模型请求，相关映射保持 partial。
- `internal/assistant/model/provider_reasoning_config_test.go:8/:27/:65`、`internal/assistant/engine/providers/reasoning_effort_transport_test.go:14/:64` 与 `internal/assistant/engine/persistence/provider_reasoning_test.go:10` 已完成 provider reasoning 运行时闭环：先红测确认请求体缺少 nested mapping，再补 agent/override 解析、Responses JSON dot path 注入、provider mapping 变更持久化与 unsupported effort 写入校验；相关 5 条配置/自定义/哨兵/持久化/标准五档矩阵映射升级为 `function_exact`。
- 本批完整门禁已复核通过：受影响精准 nextest 3/3、`check:quick` 全流程通过；完整 `check:rust` workspace nextest 3459/3459 通过（2 skipped），7 个 compatibility replay、clippy、format 与 Pine worker 98/98 全部通过。此前 quick 首次被 target 中间产物健康阈值拦截，清理 Rust 产物后重跑通过。
- 本批最新门禁证据：`check:quick` 的受影响 Rust nextest 1978/1978 通过，format、clippy、7 个 compatibility replay、Pine worker 98/98 与桌面检查全部通过；此前一次 quick 因 target 中间 `.rcgu.o` 超过 50000 被拦截，确认无 Cargo 进程后清理 118712 个、约 29.1 GiB Rust 产物再重跑。随后全量 `check:rust` 在既有 `product_api_launcher_lifecycle::api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 的 `None`/`Some(0)` 退出码断言处失败（2218/2219 已完成且通过）；该单测以 nextest `--retries 2` 复核 1/1 通过，完整门禁失败证据保留，不将该次全量运行记为通过。
- 本批映射审计更新为 Go 4451、Rust 3337；`function_exact` 1479、`partial` 2338、`boundary` 636，重复 exact 0，0 nonexistent crate，2 个已记录 partial unresolved refs。Parity 锚点为 1764/1718/0/0/46；报告与 inventory 已重生成，anchor reconcile 无 unrecorded/stale。
- 本批新增标准 reasoning.effort 请求矩阵回归：model default、low、medium、high、xhigh、max 六种请求逐项验证；精准 nextest 7/7、`check:quick` 受影响 Rust 1979/1979、完整 `check:rust` workspace 3464/3464（2 skipped）均通过，7 个 compatibility replay、format、clippy 与 Pine worker 98/98 全部通过。映射审计更新为 Rust 3338、`function_exact` 1480、`partial` 2337、`boundary` 636；重复 exact 0、nonexistent crate 0，Parity 锚点 1764/1718/0/0/46，报告与 inventory 已重生成，anchor reconcile 无 unrecorded/stale。
- 本批 candle period/pagination 精准回归 2/2 通过；`check:quick` policy lane 通过，完整 `check:rust` 首次在既有 `product_api_launcher_lifecycle::api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 的 `None`/`Some(0)` 生命周期断言处失败（2216/2217 已完成且通过），随后以 nextest `--retries 2` 单测复核 1/1 通过。该间歇性 launcher 失败与本批文档映射无关，完整门禁失败证据保留；审计仍为 Rust 3338、`function_exact` 1481、`partial` 2336、`boundary` 636，锚点 1764/1718/0/0/46。
- 本批 dispatcher envelope/SSE frame 精准回归 1/1 通过；`check:quick` policy lane 通过，完整 `check:rust` workspace 3464/3464（2 skipped）以及 storage、backtest、provider runtime、trading/strategy、assistant runtime、API transport、desktop runtime 七个 compatibility replay 全部通过。映射审计保持 Rust 3338、`function_exact` 1481、`partial` 2336、`boundary` 636，重复 exact 0、nonexistent crate 0；Parity 锚点 1764/1718/0/0/46，报告与 inventory 已重生成，anchor reconcile 无 unrecorded/stale。
- 本批审计更新为 Go 4451、Rust 3330；`function_exact` 1473、`partial` 2343、`boundary` 636，重复 exact 0，0 nonexistent crate，2 个已记录 partial unresolved refs。Parity 锚点为 1757/1711/0/0/46；报告与 inventory 已重生成，anchor reconcile 无 unrecorded/stale。
- 本批映射审计更新为 Go 4451、Rust 3328；`function_exact` 1473、`partial` 2343、`boundary` 636，重复 exact 0。Parity 锚点为 1756/1710/0/0/46；报告与 inventory 已重生成。
- 本批 `check:quick` 首次被已有 target 健康门禁拦截（`target/debug/deps` 中间 `.rcgu.o` 超过 50000）；确认无 Cargo 进程后清理 121048 个、约 32.5 GiB Rust 产物，重跑受影响 lane 2056/2056 通过，format、clippy、API transport compatibility 与 desktop checks 全部通过。
- 本批完整 `check:rust` workspace nextest 3454/3454 通过（2 skipped）；storage、backtest、provider runtime、trading/strategy、assistant runtime、API transport、desktop runtime 七个 compatibility replay 全部通过。审计与锚点复核保持 0 重复 exact、0 nonexistent crate、0 unrecorded/stale anchor。
- 本批 `check:quick` 的受影响 Rust/desktop nextest 首次为 1919/1920，唯一失败仍是既有 `product_api_launcher_lifecycle::api_launcher_reports_startup_failure_when_the_configured_address_is_taken` 的间歇性 `None`/`Some(0)` 生命周期断言；随后以 nextest retry 2 单测复核通过。完整 `check:rust` 复跑 workspace 3456/3456 与七个 compatibility replay 全部通过。
- resolver limit 差异已在 `crates/jftrade-engine/src/product_production_ports_market_data_catalog_futu.rs` 修复：避免 provider 在 CN/SH/SZ 过滤前按公开 limit 截断候选；TTL/singleflight 仍保留为架构边界，不宣称等价。
- 近期真正修改过 Rust 生产代码的批次包括：交易默认市场注入、下单前名义金额回退、市日边界、策略运行时及若干行情/路由边界；这些改动均配有回归测试或兼容性证据。
- 最近的 strategy/API 批次主要是证据审查和文档落账，没有新增 Rust 生产代码，必须与“功能已完成”分开看待。

## 仍未闭环的部分

- `partial` 仍有 2337 条，说明 Go 断言与 Rust 证据尚未达到同层等价。
- `api_transport`、`backtest_calendar`、`assistant` 等领域仍有大量 partial；下一步应优先选择一个真实功能缺口，补 production owner、失败回归测试和实现，再更新清单。
- Watchlist/Futu remote reader、TickCandles/跨交易日 `VolumeDelta` 等历史上已识别为架构能力缺口，不能靠继续增加映射文档闭环。
- `boundary` 不是“通过”，而是明确记录当前 Rust 架构没有对应 Go helper、wrapper 或旧入口。

## 调度收敛规则

- 当前只保留一个持续队列上下文；本批完成后下一片按 P1 继续处理 `internal/api/live/dispatcher_boundaries_test.go:205:TestDispatcherEnvelopeDefaultsAndMapFallback`。
- 不创建子任务、不创建第二个 heartbeat、不重复复核已经完成的切片。
- 每批先读 Go 实现和 Rust owner；只有发现真实行为差异才先写失败回归测试并修改生产代码。
- 仅证据不足时维持 `partial` 或 `boundary`，不得为了提高数字升级为 `function_exact`。
- 每批只运行对应证据的精准测试，按批次提交；完成后只更新同一个 heartbeat 的下一批范围。

## 最近提交

- 当前批次：subscriptions 与 quote availability 6 条测试逐项复核，补 authoritative quote 缺失字段与 legacy zero projection 回归测试。
- `b83b57d4`：marketdata provider settings 路由 catalog、失败回滚与成功写入逐项核对，保持 partial。
- 当前批次：system manual-retry 路由补一次调用回归并升级为 function_exact。
- 当前批次：system calendar refresh 两条路由补真实 manager 回归，保持 partial。
- 当前批次：system validator 边界补零值限额与缺 hard-stop ID 路由回归，保持 partial，记录 400/404 形状差异。
- 当前批次：system release/disable 畸形载荷补三路由 no-port-call 回归，保持 partial，记录 Gin/service owner 形状边界。
- 当前批次：system calendar probe 路由核对真实 manager 结果与 market 边界，保持 partial，记录 callback 参数/次数差异。
- 当前批次：system real-trade control 写路由补真实产品状态读回序列，保持 partial，记录 Gin callback 与固定 hard-stop ID 差异。
- 当前批次：system control 错误映射与 16 路由 envelope 逐项核对；新增错误码回归，系统读族保持 partial，记录 Gin 必填键表与 Rust 分散 owner 差异。
- 当前批次：system status mapper typed JSON 投影逐项核对，补稳定字段/成功 envelope 锚点，保持 partial 并记录未知字段断言缺口。
- 当前批次：Assistant chat helper timeline boundary 与 bearerToken 边界逐项核对；新增大小写不敏感 Bearer 回归并修复 API auth 生产 owner，保持 boundary/partial。
- 当前批次：Assistant chat stream final tool output 裁剪与 missing-agent preview 边界逐项核对；新增先红后修的 Stream terminal projection 回归与 fail-closed preview 回归，保持 partial。
- 当前批次：Assistant 已知 session preview 与 provider reasoningConfig 默认/校验逐项核对；新增已有 session 单 preview 生产回归与 reasoning 配置先红后修，provider 解析注入仍保持 partial。
- 当前批次：provider reasoning mapping 持久化、agent/override 解析与 Responses nested transport 逐项对齐；新增先红后修的请求体回归与 agent unsupported-effort 回归，标准五档/缺省请求矩阵回归后，5 条映射升级为 function_exact。
- 当前批次：`internal/api/assistant/chat_helpers_test.go:59` 的 timeline 空 delta、clone、防御性缺省 helper 逐项复核；确认属于 Go 内存 hub 专属边界，Rust durable stream/replay 两条生产测试 2/2 通过，无生产功能差异。
- 当前批次：`internal/api/assistant/chat_helpers_test.go:13` 的 session/run/tool timing 与 reasoning/message 拼接逐项复核；确认属于 Go 进程内 timeline state machine 边界，Rust durable session→run→final 与 replay 证据 2/2 通过，无生产功能差异。
- 当前批次：`internal/api/assistant/chat_stream_recovery_contracts_test.go:12` 的不可序列化 transient tool clone fallback、timeline-only delta 与 final recovery 逐项复核；确认 Go JSON clone 兜底属于旧内存 hub 边界，Rust durable replay/live/final recovery 三条证据 3/3 通过，无生产功能差异。
- 当前批次：`internal/api/assistant/chat_stream_recovery_contracts_test.go:41` 的已知 session preview 与持久化终态 final recovery 逐项核对；Rust 真实生产 preview、终态恢复三条证据 3/3 通过，contextSent 内存 seam 未强行升级，保持 partial。
- 当前批次：`internal/api/assistant/chat_transport_disconnect_test.go:110` 的 stream/run reconnect、after 游标重放与失败 writer 退出逐项核对；engine/API 精准回归 4/4 通过，Rust 一次性 SSE 物化没有 Go 单写次数 seam，保持 partial。
- 当前批次：`internal/api/backtest/routes_progress_test.go:71` 的 sync progress/cancel 成功与缺失路由逐项闭环；GET fixture、SQLite cancel 成功、production cancel 缺失及 sync owner 五条 Rust 证据 5/5 通过，映射升级为 `function_exact`。
- 当前批次：`internal/app/apiserver/backtestapp/historical_source_test.go:62:TestProviderHistoricalSourceMapsExtendedSessionsToProviderCapabilities` 逐项核对 providerSessions 的 regular/extended/overnight 过滤；确认这是旧 backtestapp 私有 helper，Rust 公开 sync contract 仅承诺 regular/extended，Futu 由 RTH/ETH/ALL planner 处理 overnight，helper descriptor 明确 yfinance 无 overnight。Rust 边界证据 4/4 通过，保持 `boundary`；下一未收尾 P1 为 `internal/app/apiserver/backtestapp/historical_source_test.go:87:TestProviderHistoricalSourceRejectsExtendedSessionsOutsideUSIntraday`。
- 本批验证：historical-source 边界 nextest 4/4 通过；`pnpm run check:quick` 与完整 `pnpm run check:rust` 通过（workspace 3464/3464，2 skipped；7 个 compatibility replay 全部通过）；mapping audit 与 anchor reconcile 均无新增失败、重复 exact 或 stale anchor。
- 当前批次：`internal/app/apiserver/backtestapp/historical_source_test.go:87:TestProviderHistoricalSourceRejectsExtendedSessionsOutsideUSIntraday` 先以 HK extended sync 回归复现旧 Rust 放行差异，再在 `product_backtest_sync_request` owner 补 US intraday 校验；定向 nextest 3/3 通过，US extended 日线仍按 Go planner 归一为 1h，低层 source validator 形状差异保持 `partial`；下一未收尾 P1 为 `internal/app/apiserver/backtestapp/historical_source_test.go:108:TestProviderHistoricalSourceValidatesAdjustmentAndLookback`。
- 本批门禁：`check:quick` 按清单/文档变更落到 policy lane，zero-go、架构/生产策略、AI context、脚本兼容性与 actionlint 全部通过；完整 `check:rust` workspace 3464/3464（2 skipped）与 7 个 compatibility replay 全部通过。映射审计更新为 Rust 3338、`function_exact` 1481、`partial` 2336、`boundary` 636；重复 exact 0、nonexistent crate 0，Parity 锚点 1764/1718/0/0/46。
- 本批验证补充：`check:quick` 首次仅因 target health 中间文件累积失败；确认无编译进程后按仓库命令清理 31.4 GiB 并重跑通过。随后完整 `check:rust` workspace 3465/3465（2 skipped）与 7 个 compatibility replay 全部通过；mapping audit/anchor reconcile 无新增失败、重复 exact 或 stale anchor。
- 当前批次：`historical_source_test.go:310:TestBacktestProviderSyncerPinsFutuAndClosesOnFailures` 逐项复核；新增 `shutdown_persists_cancellation_before_joining_sync_workers`、`terminate_persists_cancellation_and_aborts_sync_workers`，并复用空 provider 结果与 restart orphan recovery，定向 nextest 4/4 通过。Go 的固定 futu、成功后显式 Close、descriptor 错误和非法数据库路径 constructor 没有 Rust composition 的同形注入入口，保持 `partial`；下一未收尾 P1 为 `historical_source_test.go:354:TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks`。
- 当前批次：`historical_source_test.go:354:TestInstrumentSpecUsesProviderRulesAndConservativeFallbacks` 逐项复核；映射补入 Futu SecurityInfo lot size、snapshot fallback/warning、broker lot→最小/步长和 HK/US/CN/SH/SZ static market profile 六条 Rust 证据，定向 nextest 6/6、受影响 quick policy 与完整 `check:rust` 3471/3471（2 skipped）及 7 个 compatibility replay 通过。动态 priceSpread、HK/A 股保守 fallback、MissingCriticalRules/warnings、details deadline、ProviderOptions 和旧 backtestapp InstrumentSpec resolver 没有 Rust 同形 owner，保持 `partial`；下一未收尾 P1 为 `historical_source_test.go:385:TestInstrumentSpecRequiresReadyPythonProviders`。
- 当前批次：`historical_source_test.go:108`/`:162`/`:206` 能力预检逐项复核；先红回归确认同步 owner 缺少 provider capability validator，随后按 yfinance/AKShare/Futu descriptor 校验 interval、复权、市场级 lookback，并在 helper candles query 透传 `adjustment`。新增 yfinance backward/7 日 lookback/forward 5m、入队前拒绝与 unknown provider 回归，定向 nextest 6/6 通过；`:108` 与 `:162` 保持 partial（旧 source 注入任意 descriptor、AKShare provider fetch 错误无同形 seam），`:206` 升级 function_exact。`check:quick` 通过；完整 `check:rust` workspace 3467/3467（2 skipped）与 7 个 compatibility replay 全部通过。下一未收尾 P1 为 `internal/app/apiserver/backtestapp/historical_source_test.go:226:TestProviderHistoricalSourceFetchesAndParsesProviderPage`。
- 当前批次续做 `historical_source_test.go:226`：先以临时去除 `adjustment` 查询参数的红测确认 helper 请求缺字段会失败，再恢复实现；新增 loopback helper 生产同步回归，验证页解析、价格/成交量入库以及 `period`、`adjustment`、`limit`、`sessions` 透传，连同空页/断链分页校验定向 nextest 3/3 通过。因 Go 的 provider fetch error 原样透传尚无同形 Rust helper 错误注入 seam，`226` 保持 partial；下一未收尾 P1 继续检查 `historical_source_test.go:266:TestHistoricalPageParsingRejectsMalformedProviderValues` 的边界聚合覆盖。
- 当前批次收尾 `historical_source_test.go:266:TestHistoricalPageParsingRejectsMalformedProviderValues`：复核确认旧 `test_historical_candle_conversion_rejects_invalid_fields_and_defaults_volume` 只构造 valid page，不能单独证明 Go 的 malformed table；新增 `helper_candle_conversion_rejects_malformed_provider_values`，通过 typed helper DTO 与生产 converter 逐项拒绝 candles envelope、时间戳、OHLC、成交量、OHLC bounds 和 cursor，定向 nextest 1/1、受影响 quick 1984/1984、完整 `check:rust` 3469/3469（2 skipped）及 7 个 compatibility replay 全部通过。映射保持 `function_exact`，line 298 的 decimalString 仍因 Go 宽类型逐类型 seam 与 Rust typed DTO 差异保持 partial；下一未收尾 P1 为 `internal/app/apiserver/backtestapp/historical_source_test.go:310:TestBacktestProviderSyncerPinsFutuAndClosesOnFailures` 的固定 provider/关闭失败矩阵。
- 本批门禁中复现既有 launcher 退出竞态：`api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal` 在 TCP ready 后立即发 TERM 时出现 `None`/`Some(0)`；将停止信号监听任务提前到 runtime start 前注册，消除端口可连但 handler 尚未安装的窗口。修改后 `product_api_launcher_lifecycle` 定向 2/2 通过；此前完整 `check:rust` 的失败证据保留，需在本修复后重跑全量门禁。
- 当前批次：`internal/api/httpserver/bindings_boundaries_test.go:61` 的 caller fallback 逐项复核；Rust UTC/日期/空值归一精准回归 1/1 通过，非法时间仍由 market-data route fail closed 为 400，保持 `partial` 并记录为有意 owner 语义差异。
- 当前批次：`internal/api/httpserver/bindings_boundaries_test.go:70` 的 required path、合法 `%20` 与 malformed escape 三分支逐项复核；API route 与 query escape 精准回归 2/2 通过，Gin helper 缺失参数错误与 Rust 404 route 形状差异保持 `partial`。
- `00f89290`：marketdata façade 剩余 34 条逐项复核，补 helper provider polling mode owner 回归测试并修复状态投影。
- `47bf7b1a`：注入配置的默认交易市场并补交易读取测试。
- `917d1534`：API transport P1 partial 第 1–30 条核对。
- `952a8993`：API transport P1 partial 第 31–60 条核对。
- `7fd577c7`：API transport P1 partial 第 61–72 条核对并收尾。
- `15f3f2d1`：strategy/Pine partial 第 281–300 条核对并修复 Pine request/indicator 差异。

`917d1534`、`952a8993`、`7fd577c7` 主要是 parity 清单和批次文档，不应被解读为新增 Rust 功能已经完成；`15f3f2d1` 则同时包含 Pine 生产修复与回归测试。

- 当前批次：`internal/app/apiserver/backtestapp/historical_source_test.go:385:TestInstrumentSpecRequiresReadyPythonProviders` 逐项核对 yfinance、规范化 `YFINANCE`、akshare 的 Python readiness 白名单与 Futu 免 helper 规则；复用 `readiness_without_composed_runtimes_reports_all_false`、`news_actions_binding_requires_yfinance_helper_readiness` 和 `test_helper_health_failure_dynamically_downgrades_provider_readiness` 三条 Rust 证据，精准 nextest 3/3 通过。Rust 覆盖动态 readiness、helper-backed 路由门和健康降级恢复，但没有旧 backtestapp `instrumentRulesRequireReady`/InstrumentSpec resolver 的逐 provider 白名单断言，保持 `partial`；下一片为 `historical_source_test.go:401:TestProviderOptionsRequireMarketDataRuntime`。
- 当前批次：`internal/app/apiserver/backtestapp/historical_source_test.go:401:TestProviderOptionsRequireMarketDataRuntime` 逐项核对 nil market-data runtime 的 panic、错误文本和固定 options 构造器边界；Rust `production_backtest_start_without_worker_fails_before_persisting_run` 精准 nextest 1/1 通过，确认缺 worker 时 fail closed 且不持久化 run。Go 的 `ProviderOptions` panic/固定选项列表没有 Rust 同形函数式构造器，保持 `boundary`；下一片为 `historical_source_test.go:414:TestPositiveFloatRecognizesSupportedRuleTypes`。
- 当前批次：`internal/app/apiserver/backtestapp/historical_source_test.go:414:TestPositiveFloatRecognizesSupportedRuleTypes` 逐项核对 Go `positiveFloat` 对 float64/float32/int/int32/int64/decimal string 的正值接受，以及零值、非法字符串、bool 的拒绝；Rust `market_rules_ignore_missing_non_positive_and_non_finite_constraints` 精准 nextest 1/1 通过，覆盖约束层正值过滤与零/负/非有限忽略。Rust typed `Fixed8`/JSON 解析没有 Go 宽 `any` helper 的逐类型表，保持 `partial`；下一片为后续 backtestapp source parity 条目。
- 当前批次：`internal/app/apiserver/marketdataapp/runtime_health_test.go:222`/`:240` 的 provider health backoff 与取消错误逐项核对；`:222` 已先红后修，Rust managed helper 默认退避从旧实现 500ms→10s 调整为 Go 对齐的 100ms→1s，并由 `helper_restart_policy_defaults_match_go_provider_health_retry_delays` 逐档锁定；`:240` 的取消/底层 probeErr 原样透传仍保持 `partial`，下一片继续处理 marketdata runtime health 条目。
- 当前批次：`runtime_health_test.go:13,64,94,114,142,167,183,205,261` 逐项复核 activation、startup restore、health retry、failed warmup、subscription rollback 与默认 checker；Rust 精准 nextest 8/8 通过。显式 Ready/warming、恢复发布和未注册 provider fail closed 的功能映射保持 `exact`，sidecar 生命周期计数、错误合并、provider 类型调用计数与等待层 calls==1 维持 `partial`/边界；下一片继续 `runtime_akshare_test.go` 相邻 provider activation/recovery 条目。
- 当前批次：`runtime_akshare_test.go:11,48,78,97,126` 逐项复核 Python provider 共享 sidecar、跨 provider 失败回滚、初始启动清理、不可用 provider 重试和缓存目录兼容；Rust 精准 nextest 6/6 通过。共享 sidecar ensure/stop 计数与 Go generic/legacy cache path 没有同形 owner，分别保持 `partial` 或边界；下一片继续 marketdata runtime/provider activation 后续条目并优先清理 P1 partial。
- 当前批次：MarketData route/query/cache/research forwarding 的 24 条 P1 映射完成逐项复核，Rust 精准 nextest 27/27 通过。provider retry 错误、session/query 校验、snapshot/tick cache、news/corporate-actions 和 industry forwarding 均有等价断言；daily candle JSON session omission 保持 `partial`，下一片继续 P1 MarketData/Quote provider forwarding 与 runtime boundary。
- 当前批次：`internal/marketdata` broker candles、cache、candle sessions、collector 与 provider switch lifecycle 的 27 条 P1 映射逐项复核，Rust 精准 nextest 40/40 通过。generation/cache fence、poll demand、session 规则、缓存 promotion 和 retry ladder 均有证据，聚合序列化字段及 collector 计数继续保持 `partial`；下一片继续 P1 MarketData/Quote provider forwarding 与 runtime boundary。
- 当前批次：Futu marketdata tick/fallback、security search、K-line session 和 market-rule reader 的 9 条 P1 映射逐项复核，Rust 精准 nextest 14/14 通过。tick conversion、fallback classification、OpenD 前置校验与 market-rule 错误合并均保持 exact；BBGO exchange wrapper reset/ownership 聚合保持 boundary，下一片继续 P1 MarketData/Quote provider forwarding 与 runtime boundary。
- 当前批次：ProductFeatures market-data facade、prediction push、research projection 与 Settings provider rollback 的 16 条 P1 映射逐项复核，Rust 精准 nextest 23/23 通过。query/projection/cache、eligibility 和 rollback/read blocking 均有 Rust 证据，聚合 nil/fallback helper 分支保持 `partial`；下一片继续 P1 MarketData assets、instrument resolver 和 subscription lifecycle。
- 当前批次：MarketData assets、instrument resolver、subscription lifecycle 的 25 条 P1 映射完成逐项复核，Rust 精准 nextest 15/15 通过。asset digest/path、search failure/input、lease sharing/demand merge/close cleanup 均有证据，嵌入 release/权限故障与 Go singleflight cache 继续保持 boundary/partial；下一片转入 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu/OpenD notifications、probe 与 subscription reconciler 的 24 条 P1 映射逐项复核，Rust 精准 nextest 24/24 通过。通知分类/标签、OpenD 协议探针、订阅共享与释放、退订 retry、代际 fence、BasicQot 延迟 fallback、配额 ack 时间均有真实 owner 证据；Go 单次代际内联重试与配额 1 分钟节流在 Rust 中没有同形实现，保持 boundary/partial。下一片继续 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu/OpenD connection recovery、market/trading reads、trade writes 与 user-security 的 30 条 P1 映射逐项复核，Rust 精准 nextest 32/32 通过。重连代际 fence、协议字段与错误透传、交易读写前置条件、账号 push、用户证券分组/成员请求均有真实 owner 证据；BBGO factory/env、callback binding 与 Rust typed error 拆分保持 partial。下一片继续 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu K-line pagination/session routing 与 OpenD read boundaries 的 32 条 P1 映射逐项复核，Rust 精准 nextest 36/36 通过。US RTH/ETH/ALL 路由、历史分页与 latest-limit、broker cursor/period helper、空结果/断连 guard、keep-alive、prediction push 与 search wire 均有真实 owner 证据；current-KL duplicate、跨 adapter 聚合和 typed wrapper 拆分保持 partial。下一片继续 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu/OpenD quote-right、research、snapshot、交易请求与 session 边界的 30 条 P1 映射逐项复核，Rust 精准 nextest 44/44（42+2）通过。成交/quote-right cache、严格 OpenD 参数、prediction/research 分页、snapshot/empty ACK、margin/cancel/account replay 与代际 session fence 均有真实 owner 证据；canceled-context、BBGO session registry 等 helper 聚合保持 partial。下一片继续 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu/OpenD session resolver、snapshot fallback、stream/subscription lifecycle 与 margin/trade 读取的 30 条 P1 映射逐项复核，Rust 精准 nextest 40/40 通过。session/window、fallback wire、snapshot cache/限流、worker shutdown、reconnect/代际 fence、时区与 margin typed recovery 均有真实 owner 证据；snapshot cancellation 聚合保持 partial。下一片继续 P1 Futu/OpenD protocol、trading/broker 与 API transport。
- 当前批次：Futu margin/transport/watchlist 与 Assistant API route boundaries 的 30 条 P1 映射逐项复核，Rust 精准 nextest 41/41 通过。margin cache/typed recovery、断连传播、watchlist TTL/刷新、ADK session/分页/stream/取消与 mutation 生命周期均有真实 owner 证据；Assistant 内存 hub seam 保持 partial。下一片继续 P1 Assistant/API transport 与 trading/broker，Futu session registry helper 聚合条目待后续边界复核。
- 当前批次：Assistant/API、Backtest、SQLite maintenance 与 runtime provider boundaries 的 30 条 P1 映射逐项复核，Rust 精准 nextest 45/45 通过。ADK replay/error envelope、backtest scope/progress、auth CSRF、watchlist/execution routes、maintenance rollback、OpenD health/probe/reconciliation 与 provider generation fence 均有真实 owner 证据；跨 owner coordinator 聚合保持 partial。下一片继续 P1 API transport、Trading/Broker execution 与 Storage/Settings。
- 当前批次：MarketData runtime、sidecar、auth/session 与 strategy lifecycle 的 30 条 P1 映射逐项复核，Rust 精准 nextest 37/37 通过。provider switch/rollback、sidecar process、auth/session durability、shutdown order、strategy recovery/cancel 与 broker disconnected state 均有真实 owner 证据；cleanup 计数和旧 servercore 聚合保持 partial。下一片继续 P1 Trading/Broker execution、API transport 与 Storage/Settings。
- 当前批次：Broker/Execution routes、auth/frontend wire 与 store lifecycle 的 30 条 P1 映射逐项复核，Rust 精准 nextest 38/38 通过。broker disconnect/unlock/cancel、execution combo/session、frontend fallback、portfolio/status/settings wire、store rollback、对账生命周期与 auth fail-closed 均有真实 owner 证据；handler 聚合保持 partial。下一片继续 P1 API transport、Trading/Broker execution、Storage/Settings 与 Assistant workflow。
- 当前批次：Web auth/session 与 Assistant assembly、workflow、approval lifecycle 的 30 条 P1 映射逐项复核，Rust 精准 nextest 34/34 通过。cookie/CSRF/session、MCP/portfolio tools、workflow wait/cancel、context/store guard 与 approval reconcile/cancellation 均有真实 owner 证据；facade 注入和 proxy cookie 聚合保持 partial。下一片继续 P1 Assistant engine、workflow/ADK 与 API transport。
- 当前批次：Assistant context、input continuation、claims 与 SQLite session persistence 的 30 条 P1 映射逐项复核，Rust 精准 nextest 26/26 通过。context compaction、projection/claims fencing、input crash recovery、stale-run reconcile、artifact path 与 SQLite schema/migration/lifecycle 均有真实 owner 证据；completion-review helper 无同名 Rust test，留待后续边界审查。下一片继续 P1 Assistant engine persistence/recovery、workflow/ADK 与 API transport。
- 当前批次：Assistant approval resume、runner lifecycle 与 runtime store 的 30 条 P1 映射逐项复核，Rust 精准 nextest 28/28 通过。approval CAS/restage、resume/recovery、session reuse、runner cancellation/reconcile、lease fencing、runtime store 与 compaction gate 均有真实 owner 证据。下一片继续 P1 Assistant runner/session context、workflow/ADK 与 API transport。
- 当前批次：Assistant session context、compaction 与 stale projection 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 24/24 通过，并补充 handoff revision 测试替换无实际 Rust 测试的 snapshot 条目。projection、approval tail、session gate、stale retry、数据库回滚、workflow compaction 与 context revision 均有真实 owner 证据；projection 聚合保持 partial。下一片继续 P1 Assistant session skill/store、workflow/ADK 与 API transport。
- 当前批次：Assistant session skill/store、workflow persistence 与 lifecycle 的 30 条 P1 映射逐项复核，Rust 精准 nextest 29/29 通过。session projection、builtin skill、MCP schema、SQLite cascade、audit/分页、composer state、writer lease、approval/cancel、timeout freeze 与 expiry reconciliation 均有真实 owner 证据；session wrapper 与多数据库聚合保持 partial。下一片继续 P1 Assistant workflow/ADK、API transport 与 Trading/Broker execution。
- 当前批次：Assistant workflow/ADK continuation、service facade 与 Backtest recovery 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 27/27 通过。取消传播、strict schema、workflow approval/CAS、goal resume/pause、terminal audit、child reconcile、dispatch adapter、timeline ordering 与 backtest lifecycle 前置证据均有真实 owner 证据；parent/child executor 聚合保持 partial。下一片继续 P1 Assistant service/read routes、workflow resource CRUD 与 Backtest recovery。
- 当前批次：Assistant service/read、workflow CRUD 与 Backtest recovery 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 27/27 通过。ADK audit/session read、runtime defaults、skill/optimization routes、cron、workflow CRUD/shutdown/CAS、canvas fail-closed 与 historical sync cancellation 均有真实 owner 证据；service/workflow facade 聚合保持 partial。下一片继续 P1 Backtest historical/storage aggregation 与 conservative execution。
- 当前批次：Backtest historical/storage aggregation 与 conservative execution 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 25/25 通过。historical sync/recovery、result/lifecycle、session scope、stop-first bracket、protective child cancel、slippage/fee、filter store 与 storage coverage/schema aggregation 均有真实 owner 证据；filter store 和多层 storage error 聚合保持 partial。下一片继续 P1 Backtest Pine worker/result collector、Market/Calendar 与 Futu session boundary。
- 当前批次：Backtest Pine worker、result collector 与 storage synthesis 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 21/21 通过。storage synthesis/coverage、Pine cancel/source guard、snapshot fallback、collector trade stats、session replay、DST/HK/US bar synthesis 与 incomplete range guard 均有真实 owner 证据；storage facade、Pine runner 与 collector 聚合保持 partial。下一片继续 P1 Market/Calendar、Futu session boundary 与桌面日志运行时。
- 当前批次：Market/Calendar、Futu session 与 quote boundary 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 26/26 通过。result/replay synthesis、partial fill、security envelope、session/quote resolver、provider utility、calendar holiday/alias、manual override、instrument validation 与 market profiles 均有真实 owner 证据；result facade 与 exchange registry 聚合保持 partial。下一片继续 P1 Calendar session windows、exchange manager 与 desktop log runtime。
- 当前批次：Calendar session windows、exchange manager 与 desktop logs 的 30 条 P1 映射逐项复核，Rust 精准 nextest 18/18 通过。market profile、manual override、session/holiday/DST、unknown market、desktop asset/log paging、maintenance validation 均有真实 owner 证据。下一片继续 P1 exchange calendar manager、AkShare/YFinance providers 与 retry/transport。
- 当前批次：Exchange calendar manager、AkShare/YFinance providers 与 transport retry 的 30 条 P1 映射逐项复核，Rust 精准 nextest 30/30 通过。manager warmup/restore/probe/refresh、AkShare/YFinance strict decoder/retry/conversion、research/calendar/rankings routes、retry/backoff 与 calendar control-plane fallback 均有真实 owner 证据；provider facade 聚合保持 partial。下一片继续 P1 bbgo stream/orderbook、Settings/Watchlist 与 SQLite/Store。
- 当前批次：bbgo stream/orderbook、Settings/Watchlist 与 transport retry 的 30 条 P1 映射逐项复核，Rust 精准 nextest 32/32 通过。YFinance conversion/retry、bbgo frame/push/keepalive/close、observability snapshot、Settings fallback/defaults、Watchlist quote/fallback/singleflight 与 revision fence 均有真实 owner 证据；YFinance 和 Settings/Watchlist facade 聚合保持 partial。下一片继续 P1 SQLite/SettingsFile、Trading execution reconciliation 与 Watchlist persistence。
- 当前批次：SQLite/SettingsFile、Trading reconciliation 与 Watchlist persistence 的 30 条 P1 映射逐项复核，Rust 精准 nextest 25/25 通过。Watchlist quote/fallback、backtest/calendar store、SettingsFile rollback/recovery、SQLite owner lock/transaction/schema guard 与 activity paging 均有真实 owner 证据；Watchlist/SettingsFile facade 聚合保持 partial。下一片继续 P1 Trading execution fill reconciliation、Watchlist store failure 与 Strategy catalog/runtime。
- 当前批次：Trading execution reconciliation、Watchlist store 与 Strategy catalog/runtime 的 30 条 P1 映射逐项复核，Rust 精准 nextest 31/31 通过。SQLite transaction/downgrade、fill/order reconciliation、cancel race/ledger reopen、Watchlist revision/pagination/rollback、asset guard、catalog activity 与 runtime recovery 均有真实 owner 证据；Strategy runtime facade 聚合保持 partial。下一片继续 P1 Strategy live runtime/Pine executor 与 Pine parser/runtime contract。
- 当前批次：Strategy live runtime、Pine executor 与 runtime contracts 的 30 条 P1 映射逐项复核，Rust 精准 nextest 37/37 通过。provider binding、runtime ownership/timeout、checkpoint、subscription rollback、executor cancel/audit、session pin/readiness、indicator warmup 与 planner validation 均有真实 owner 证据；live runtime manager 聚合保持 partial。下一片继续 P1 Pine parser/lowering/runtime worker contracts 与 Trading transport/API。
- 当前批次：Pine parser/lowering、worker contracts 与 runtime boundaries 的 30 条 P1 映射逐项复核，Rust 精准 nextest 21/21 通过。Pine compiler features/recovery、UDF/static loops、diagnostics/annotations、worker payload/startup/readiness/termination、checksum/pinning 与 live-session contract 均有真实 owner 证据；parser diagnostic 与 worker manager facade 聚合保持 partial。下一片继续 P1 Trading broker/API transport、execution combo 与 control-plane。
- 当前批次：Trading broker/API transport、execution combo 与 control-plane 的 30 条 P1 映射逐项复核，Rust 去重后精准 nextest 50/50 通过。worker guards、broker fallback/write、hard-stop/control-plane、combo preview/client IDs、order normalization/risk、history recovery、subscription reconnect 与 response serialization 均有真实 owner 证据；broker facade 与 transport worker 聚合保持 partial。下一片继续 P1 broker capability/research routes、remaining Trading read/write 与 API transport。
- 当前批次：Broker capability/research 与剩余 Trading read boundaries 的 9 条 P1 映射逐项复核，Rust 精准 nextest 12/12 通过。broker/portfolio projection、risk hard-stop/fallback、capability catalog、snapshot rate-limit/fallback、operation surface 与 research normalization 均有真实 owner 证据；`session_context_snapshot` 无实际 Rust 测试，保持未验证并留待边界审查。P1 有效映射已耗尽，下一片转 P2 Strategy/Pine、Trading/API、Storage/Settings 与边界审查。
- 当前批次：Assistant ADK API routes 与 workflow transport 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 32/32 通过。approval、goal pause/resume、metrics、catalog/read、chat/stream replay、provider/agent/skill mutation、workflow webhook、query encoding 与 unavailable/store-failure boundary 均有真实 owner 证据；workflow invalid-input 聚合保持 partial。下一片继续 P2 Assistant、Backtest/API transport 与 Settings/Storage 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Assistant route errors、Backtest routes 与 HTTP bindings 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 37/37 通过。Assistant malformed/error/resource/validation、stream replay/SSE、workflow canvas、Backtest result/start/sync/delete、query bool 与 candle period normalization 均有真实 owner 证据；stream/SSE、provider validation 与 Backtest handler 聚合保持 partial。下一片继续 P2 Backtest、MarketData/Calendar、Strategy/Pine 与 API transport；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：HTTP bindings、SSE writer 与 WebSocket live transport 的 30 条 P2 映射逐项复核，Rust 精准 nextest 30/30 通过。query/path/response binding、SSE writer/loop、WebSocket origin/limit/heartbeat、subscription normalization 与 depth demand 均有真实 owner 证据；dispatcher/handler 聚合保持 partial。下一片继续 P2 API transport、MarketData/Calendar、Strategy/Pine 与 Trading/Storage 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：MarketData route contracts 与 middleware guards 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 34/34 通过。instrument/candle/quote/snapshot、subscription lease、news/actions、tick dedupe、ADK availability 与 public auth guards 均有真实 owner 证据；provider/error 聚合与 ADK middleware guard 保持 partial。下一片继续 P2 MarketData/Quote provider、Trading/API transport、Strategy/Pine 与 Storage/Settings 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Auth/CORS origin、prediction combo 与 research routes 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 36/36 通过。auth/CSRF/CORS、origin normalization、prediction combo、embedded rankings/company/calendar research 与 research-screen schema/429/validation 均有真实 owner 证据；context、facade 与跨 provider error 聚合保持 partial/boundary。下一片继续 P2 research/provider、Backtest/Calendar、Trading/API 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Research screen/preset 与 Settings routes 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 28/28 通过。research schema/catalog/preset、prediction/provider routes、Settings persistence/rollback、managed account、provider mutation、MCP token、legacy fallback 与 calendar reload 均有真实 owner 证据；Research/Settings facade 聚合保持 partial。下一片继续 P2 Settings/Storage、Backtest/Calendar、MarketData provider 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Strategy/Pine route contracts 与 Settings callback boundaries 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 26/26 通过。Pine diagnostics/AST/source validation、strategy definition/instance/plugin lifecycle、version history、Settings callback/calendar/data-management 与 response defaults 均有真实 owner 证据；Strategy/Pine 与 Settings facade 聚合保持 partial。下一片继续 P2 Strategy/Pine、MarketData provider、Backtest/Calendar 与 Trading/API 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Trading execution/broker routes 与 Watchlist route errors 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 31/31 通过。execution risk/error/query/preview、broker read/write/analytics、capability reachability、Watchlist mutation/query/revision 与 Pine route validation 均有真实 owner 证据；Trading/Watchlist facade 及 `map_trade_error` 细分错误码缺口保持 partial。下一片继续 P2 Trading/Broker、Watchlist/Storage、MarketData provider 与 Backtest/Calendar 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Watchlist routes、application lifecycle 与 database maintenance 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 39/39 通过。Watchlist unavailable/query guards、assistant/application ownership、startup rollback/reverse close、AkShare lookback/candle conversion、database guards 与 maintenance backup/retention/rebuild/candidate checks 均有真实 owner 证据；facade 与动态 helper 聚合保持 partial。下一片继续 P2 database/maintenance、MarketData/Quote provider、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Database maintenance manager、backup retention 与 rebuild safety 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 32/32 通过。preview/overview、backup verification/quota、retention/discovery、rebuild scheduling/locks、manifest/marker safety 与 pending apply/delete failure 均有真实 owner 证据；maintenance manager facade 聚合保持 partial。下一片继续 P2 database/maintenance、Storage/SQLite、MarketData provider 与 Backtest/Calendar 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：Rebuild safety、Futu lifecycle、desktop startup 与 web access 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 31/31 通过。marker/research database、asset/sidecar readiness、Futu reset/probe、listener conflict/rebind/startup rollback 与 provider selection 均有真实 owner 证据；Futu/lifecycle/assistant-provider facade 聚合保持 partial。下一片继续 P2 lifecycle/MarketData provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：MarketData provider switching、heartbeat、depth 与 security routes 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 82/82 通过。provider switch/rollback/readiness、heartbeat cadence/connectivity、depth projection/clamp、snapshot/security block、search/candle normalization 与 callback delegation 均有真实 owner 证据；provider/heartbeat/security facade 聚合与 broker-neutral block 保持 partial/boundary。下一片继续 P2 MarketData/Quote provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：MarketData runtime forwarding、Python helpers 与 research provider routes 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 49/49 通过。search/helper health/process、period/K-line bounds、calendar/company/index/news/rankings/screen forwarding、capability gates、provider transition/rollback 与 deferred unsubscribe 均有真实 owner 证据；runtime/Python facade 聚合保持 partial。下一片继续 P2 MarketData/Quote provider、Futu/OpenD、Backtest/Calendar 与 Strategy/Pine 映射；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：MarketData runtime/sidecar、Watchlist source 与 runtime resources 的 30 条 P2 映射逐项复核；可解析过滤器对应 Rust 去重后精准 nextest 30/30 通过，28 条写入通过凭证。provider transition/rollback、sidecar process cleanup、Watchlist source/error、Node dependency matrix、research/resource inventory 均有 owner 证据；两条 `from_process_env`/`tests` 开发 helper 条目没有独立 Rust parity test，保持 partial/unverified，未把过宽过滤器结果计入。下一片继续 P2 runtime/desktop boundary、Storage/SQLite、Backtest/Calendar 与 Strategy/Pine；P1 `session_context_snapshot` 仍保留为无实际 Rust 测试的边界条目。
- 当前批次：runtime/desktop lifecycle、maintenance 与 broker query 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 25/25 通过；`ProductShutdownSupervisor` 仅为 owner 类型名，未伪装为测试。runtime paths/logs/origins、registry/shutdown、pending rebuild、ADK maintenance、assistant transport 与 broker query 均有 owner 证据；Wails/profile、Handle publication、重建中间态和 server facade 结构差异保持 partial/boundary。下一片继续 P2 Storage/SQLite maintenance、Backtest/Calendar、MarketData/Quote 与 Strategy/Pine；前两条开发 helper 边界及 P1 `session_context_snapshot` 仍保持未验证结论。
- 当前批次：Storage maintenance、live/WS market data 与 notifications 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 39/39 通过。maintenance purge/compaction、desktop token、execution writeback/reconciliation、instrument/volume normalization、live lease/WS/depth、resolver/realtime candle 与 notification wire mapping 均有 owner 证据；Go facade、WS 首帧、security details、marker path 与 error matrix 的结构差异保持 partial/boundary。下一片继续 P2 API/transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；开发 helper 边界及 P1 `session_context_snapshot` 仍保持未验证结论。
- 当前批次：OpenAPI/observability、strategy runtime trading 与 lifecycle rollback 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 51/51 通过；29 条写入通过凭证。OpenAPI/notification sink、request context/id、provider integration、preview hash、strategy notify/risk/execution/quantity/close、current-bar catch-up 与 shutdown rollback 均有 owner 证据；`optional_bool_strict` 仅为无独立测试的 helper，保持 partial/unverified。下一片继续 P2 Strategy/Pine、Backtest/Calendar、API transport 与 Storage/SQLite；helper/boundary 条目及 P1 `session_context_snapshot` 仍保持未验证结论。
- 当前批次：Backtest/bootstrap、Strategy runtime、Futu/OpenD 与 Web settings 的 30 条 P2 映射逐项复核，Rust 去重后精准 nextest 42/42 通过。backtest request/compile、degraded startup、broker bridge、Pine plan/JSON guard、WS/live push、strategy demand、risk/settings side effects、OpenD health、provider rollback 与 cookie/session security 均有 owner 证据；server/bootstrap、callback、desktop/Web settings 聚合保持 partial，未凭测试数量宣称 exact。下一片继续 P2 Strategy/Pine、Backtest/Calendar、API transport、Storage/SQLite 与 Settings/Watchlist；P1 `session_context_snapshot` 及 helper 边界仍保持未验证结论。
- 当前批次：Strategy lease、WebSocket live、Backtest runs 与 Broker routes 的 30 条 P2 映射逐项复核，Rust nextest 跨 binary 46/46 通过（41 个过滤器，helper 符号不计独立证据）。linked delete、execution/lease、WS heartbeat/notification/quote、backtest sync/restart、broker query/validation/write 与 JSON/path guards 均有 owner 测试；Go facade、handler 聚合、断连状态与注入形状保持 partial/boundary。下一片继续 P2 Broker/API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Strategy/Pine；`optional_bool_strict`、P1 `session_context_snapshot` 等 helper/边界条目仍保持未验证结论。
- 当前批次：Broker read/API contracts、OpenAPI、frontend fallback 与 plugin lifecycle 的 30 条 P2 映射逐项复核，Rust nextest 44/44 通过。broker projections/runtime descriptor、system/settings/market/strategy/backtest contract、execution normalization/reconciliation、SPA/static fallback、market profiles、OpenAPI schema/error/request bodies 与 plugin artifact lifecycle 均有 owner 测试；Go handler、Wails/desktop、fixture 注入与聚合 contract 保持 partial/boundary。下一片继续 P2 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；`optional_bool_strict`、P1 `session_context_snapshot` 等 helper/边界条目仍保持未验证结论。
- 当前批次：Research runtime、Settings onboarding、Strategy definitions 与 Swagger/system status 的 30 条 P2 映射逐项复核，Rust nextest 33/33 通过。research preset/runtime、desktop defaults/origin、strategy definition UUID/write/delete/version、broker/interface/settings normalization、onboarding readiness、strategy logs/preview/sync、Swagger UI/core paths 与 system status 均有 owner 测试；Go/Wails startup、HTTP facade、dependency injection 与聚合响应保持 partial/boundary。下一片继续 P2 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；`optional_bool_strict`、P1 `session_context_snapshot` 等 helper/边界条目仍保持未验证结论。
- 当前批次：系统状态、执行通知、订单对账与 Web 鉴权的 30 条 P2 映射逐项复核，Rust nextest 36/36 通过（表达式含 36 个过滤器，3437 个测试跳过）。request-id/status/watchlist、runtime projection/account/capability、combo/cancel/reservation、notification dedup、reconciliation 与 origin/auth/proxy/rate-limit 均有 owner 证据；`capability_unsupported_error` 仅为 helper，Go Server/WebAuth/Wails facade 与 handler 聚合继续保持 partial/boundary。下一片继续 P2 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；`optional_bool_strict`、P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Web access/frontend 安全与 Assistant/Workflow/ADK contracts 的 30 条 P2 映射逐项复核，Rust nextest 实际 42/42 通过（表达式含 43 个过滤器，3431 个测试跳过）。SPA/API fallback、origin/cookie/session/listener、ADK validation/capability/closure/catalog、backtest/strategy filters、task persistence 与 subscription failure 均有 owner 证据；`desired_bind` 仅为 helper，Go WebAuth/HTTP facade、Wails 组合、ADK handler/queue wiring 继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK、API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；`optional_bool_strict`、P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：ADK application adapters、market research tools 与 MCP lifecycle 的 30 条 P2 映射逐项复核，Rust nextest 实际 54/54 通过（表达式含 53 个过滤器，3419 个测试跳过）。ADK unavailable/view、provider override、summary/readiness、application adapter normalization、backtest/strategy lifecycle、SQLite maintenance、index/news/corporate-actions 与 MCP listener generation 均有 owner 证据；Go assembly/handler/queue facade、跨 binary 重复过滤器与跨域聚合继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK、API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；`optional_bool_strict`、P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：MCP auth、Portfolio tools、runtime adapters 与 tool catalog 的 30 条 P2 映射逐项复核，Rust nextest 实际 35/35 通过（表达式含 34 个过滤器，3438 个测试跳过）。MCP generation/token/loopback、portfolio discovery/account scope/partial reads、OpenD/product dispatch、database/runtime ownership、ADK tool normalization/watchlist/execution projections 均有 owner 证据；`screen_query_defaults_the_page_and_keeps_catalog_columns` 跨 binary 重复运行，Go facade、Handle 生命周期与多工具 adapter 聚合继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK、API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；`optional_bool_strict`、P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Workflow bridge/tools、ADK runtime edges 与 typed capabilities 的 30 条 P2 映射逐项复核，Rust nextest 实际 30/30 通过（表达式含 31 个过滤器，3443 个测试跳过；`workflow_bridge_operations_fail_closed_without_their_ports` 有 1 个 leaky pass）。typed capability/watchlist、workflow CRUD/approval/error、interrupt/resume、memory/model/notices、store transaction/maintenance、timeout/compaction/CAS、strict schema 与 skill install 均有 owner 证据；`core_schema_for` 仅为 helper，Google ADK/GORM/test-double facade 与 workflow manager wiring 继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK、API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；`optional_bool_strict`、P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：ADK store/tool edges、chat execution 与 durable claims 的 30 条 P2 映射逐项复核；3 条 `complete`/`CanvasCompiler`/`synthetic_assistant_message_id` helper/type-only 条目保持 unverified，其余 27 条对应 26 个真实过滤器，Rust nextest 26/26 通过（3447 个测试跳过）。provider/approval/workflow store、HTTP/model/notice/tool edges、chat idempotency、canvas/stream ordering、execution projection/recovery 与 lease fencing 均有 owner 证据；Google ADK/GORM/test-double facade 和 helper 分支继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK、API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar；上述 helper 及 P1 `session_context_snapshot` 等无独立 Rust 测试条目仍保持未验证结论。
- 下一批精确范围预留：`internal/assistant/engine/input_request_test.go:413,449,556,584,647,694,738,773,802`、`input_workflow_test.go:9`、`mcp_server_test.go:17,119,129,136,162,194,266,293,333,358,367,382,440`、`persistence/approval_query_plan_test.go:9`、`persistence/execution_claims_test.go:58,186,229`、`persistence/google_artifact_test.go:19,120,186`，共 30 条 P2 映射；继续排除 helper-only 过滤器并按真实 Rust 测试写入凭证。
- 当前批次：ADK claims、execution projection 与 input request continuation 的 30 条 P2 映射逐项复核，Rust nextest 30/30 通过（3443 个测试跳过）。claims takeover/replay、execution timeline/CAS、memory/provider recovery、handoff revision、input validation/idempotence/conflict 与 request-user budget 均有 owner 证据；Go Google ADK facade、test double、continuation handler 与 UI prompt composition继续保持 partial/boundary。下一片处理 input request 后半段、MCP reviewed tools、persistence claims/artifacts，再进入 API transport、MarketData/Quote、Storage/SQLite 与 Backtest/Calendar。
- 下一批精确范围预留：`internal/assistant/engine/input_request_test.go:584,647,694,738,773,802`、`input_workflow_test.go:9`、`mcp_server_test.go:17,119,129,136,162,194,266,293,333,358,367,382,440`、`persistence/approval_query_plan_test.go:9`、`persistence/execution_claims_test.go:58,186,229`、`persistence/google_artifact_test.go:19,120,186`，共 30 条 P2 映射；继续排除前序 helper-only 条目并按真实 Rust 测试写入凭证。
- 当前批次：input request continuation、MCP reviewed tools 与 ADK persistence 的 30 条 P2 映射逐项复核；其中 `CanvasCompiler` 与 `synthetic_assistant_message_id` 仅为 helper/type-only 符号，保持 unverified，其余 28 条真实 Rust 测试 nextest 28/28 通过（3445 个测试跳过）。input pause/resume、approval transition、MCP allowlist/runtime status、claims fencing/replay、SQLite query plan 与 artifact version/restart 均有 owner 证据；Go continuation/handler、Google ADK/GORM facade、test runtime wiring 与 helper 分支继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK 尾项，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：ADK persistence failures、provider runtime 与 approval concurrency 的 30 条 P2 映射逐项复核；6 个 `write_adk_secrets`/`CanvasCompiler`/`new`/`provider_payload`/`extract_tool_calls`/`execute_model` helper/type-only 条目保持 unverified，其余 24 条映射的引用测试均通过。动态表达式因 helper 名称过宽实际 nextest 81/81 通过（3392 个测试跳过），但额外命中不计独立证据；provider selection、transaction/failure rollback、workflow projection、Responses/reasoning/runtime 与 approval fencing 均有 owner 测试。Go GORM/Google ADK facade、fixture 注入、内部 helper 与 goroutine/registry wiring 继续保持 partial/boundary。下一片继续剩余 P2 Assistant/Workflow/ADK provider、runner 与 transport，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Runner chat projection、continuation signal 与 approval runtime 的 30 条 P2 映射逐项复核，Rust nextest 31/31 通过（3442 个测试跳过）。rejected compaction、continuation-only audit、structured/provider/workflow failures、run gate/snapshot/projection、terminal audit/final message、approval visibility 与 continuation fencing 均有 owner 测试；Go Google ADK runner facade、GORM transcript wiring、callback sink、in-process registry 与 nil receiver 专属行为继续保持 partial/boundary。下一片继续 P2 Assistant/Workflow/ADK runner continuation、transport 与 provider 边界，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：continuation lease、goal lifecycle、runtime store 与 skills 的 30 条 P2 映射逐项复核，Rust nextest 28/28 通过（3445 个测试跳过）。continuation approval/lease fencing、goal pause/resume、run projection、shutdown supervisor、ADK catalog、model allowlist、approval persistence 与 skill install/archive/catalog 均有 owner 测试；Go runner/Runtime facade、GORM/test-provider fixture、goroutine/nil receiver、localized summary 与 filesystem registry 注入继续保持 partial/boundary。下一片继续剩余 P2 Assistant/Workflow/ADK skill registry、provider 与 runtime 边界，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Skill Registry、MCP schema、SQLite dialector 与 approval store 的 30 条 P2 映射逐项复核，Rust nextest 20/20 通过（3453 个测试跳过）。skill archive/install、安全路径、MCP strict schema/catalog、SQLite migration/store durability 与 approval idempotence/CAS 均有 owner 证据；Go nil receiver、filesystem fault injection、技能文档文本、GORM dialector helper 及聚合 run-plus-approval 语义继续保持 partial/boundary，未把相邻 owner 测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow/ADK skill/provider/runtime 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：ADK store approval、provider lifecycle、run CAS 与 maintenance 的 30 条 P2 映射逐项复核，Rust nextest 25/25 通过（3448 个测试跳过）。approval/provider/default/secret、transaction rollback、run pause/reopen/terminal CAS、skill/agent catalog、maintenance purge/handoff 与 archive install 均有 owner 证据；Go store 聚合、fixture 注入、错误文本、文件系统和管理器 facade 继续保持 partial，未把跨领域聚合测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow/ADK store、skill/provider 与 runtime 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：store ops/recovery、approval continuation 与 ADK tool gates 的 30 条 P2 映射逐项复核，Rust nextest 30/30 通过（3443 个测试跳过）。URL skill registration、agent soft-delete/restore、approval idempotence/restart、task/memory/tool gate、writer/migration fencing 与 denial summary 均有 owner 证据；Go HTTP/Google ADK facade、GORM migration fixture、handler 聚合和 exact error text 继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow/ADK store、runtime、task/timeline 与 provider 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：approval audit、task runner、artifact 与 tool/network policy 的 30 条 P2 映射逐项复核，Rust nextest 30/30 通过（3443 个测试跳过）。denial audit/tool failure、bounded task runner、workflow cycle、timeline/artifact durability、MCP/network security、tool risk/approval、account orders 与 K-line companion 均有 owner 证据；Go goroutine/HTTP fixture、Google ADK facade、多工具聚合和 helper fallback 继续保持 partial，未把聚合测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow/ADK runtime、tools、usage、workflow native integration 与 provider 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflow native integration、canvas execution 与 usage/tool continuation 的 30 条 P2 映射逐项复核，Rust nextest 30/30 通过（3443 个测试跳过）。tool access/failure、usage recovery、native workflow streaming/cancellation/recreation、approval/input CAS、canvas graph execution/pause/failure、child callbacks 与 compiler edges 均有 owner 证据；Go Google ADK iterator、registry callback、manager facade 和注入形状继续保持 partial，未把聚合测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow/ADK workflow compiler/execution/goal、provider 与 runtime 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only 条目、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflow compiler、goal lifecycle、observation、store 与 toolset 的 30 条 P2 映射逐项复核，Rust nextest 30/30 通过（3443 个测试跳过）。canvas/graph 校验、goal pause/resume 与 CAS、workflow observation/schema、store soft-delete、tool failure/schema 与 skill filtering 均有 owner 证据；Go compiler/manager/GORM、Google ADK facade、executor goroutine 与 child callback 组合继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续剩余 P2 workflowexec/provider/runtime 条目，再转 API transport、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 Backtest/Calendar；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflowexec、task tools、service facade 与 runtime 边界的 30 条 P2 映射逐项复核，29 个证据过滤器因 `test(approvals)` 宽匹配实际运行 39/39 通过（3434 个测试跳过）。pause/approval/child failure、task/memory normalization、graph/timeline/plan、service validation/readiness/metrics、CRUD/approval wrappers 与 shutdown owners 均有证据；Go executor、service/GORM 聚合、callback sink 与 nil-runtime 行为继续保持 partial/boundary，未把宽匹配测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow provider、runtime 与 API transport，再转 MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflowexec 持久化失败、task graph、approval/child fencing 与 goal pause 的 30 条 P2 映射逐项复核，21 个唯一过滤器 Rust nextest 21/21 通过（3452 个测试跳过）。持久化错误传播、终态/暂停 CAS、model-list/runtime readiness、task graph、transcript/finalization 与 workflow helper owners 均有证据；Go SQLite trigger、GORM/session facade、toolset registry、scheduler callback 与 response 聚合继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续剩余 P2 Assistant/Workflow provider、runtime、API transport 与 MCP，再转 MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflow service、trigger rules、queue/CAS 与 scheduler lifecycle 的 30 条 P2 映射逐项复核，28/28 个唯一过滤器通过（3445 个测试跳过）。approval/runtime boundaries、threshold/event/cron、queue claims、worker shutdown、CRUD/log persistence、schedule tick 与 background dispatch 均有 owner 证据；Go service/GORM/HTTP facade、background goroutine、map normalization 与 fixture helpers 继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续剩余 P2 workflow CRUD/scheduler/provider/runtime 与 API transport/MCP，再转 MarketData/Quote、Storage/SQLite、Settings/Watchlist、Backtest/Calendar 与 Strategy/Pine；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：workflow secret/canvas 与 backtest sync、readiness、result view、Pine/research runner 的 30 条 P2 映射逐项复核，25/25 个唯一过滤器通过（3448 个测试跳过）。secret sanitization、canvas round-trip、historical pagination/provider isolation、validation/readiness/worker guards、result-view aggregation、Pine worker 与 chart normalization 均有 owner 证据；Go service facade、GORM/in-memory store、provider/worker injection、map payload 与 goroutine observability 继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续 P2 Backtest/Calendar 与 MarketData/Quote、Storage/SQLite、Settings/Watchlist，再处理 API transport/MCP、Strategy/Pine 与剩余 Assistant/provider 条目；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：backtest data readiness、sync store、calendar 与 conservative-bar execution 的 30 条 P2 映射逐项复核，24/24 个唯一过滤器通过（3449 个测试跳过）。readiness warmup、sync task/lease/recovery、run store、OpenD persistence、calendar normalization 与 next-open/close/bracket/reduce-only execution 均有 owner 证据；Go injected callbacks、in-memory/GORM facade、progress/event fixtures 与 account stream aggregation 继续保持 partial/boundary，未把聚合测试升级为 exact。下一片继续 P2 Backtest/Calendar 后续与 MarketData/Quote、Storage/SQLite、Settings/Watchlist，再处理 API transport/MCP、Strategy/Pine 与剩余 Assistant/provider 条目；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：conservative-bar 深度分支、结果/费用边界与 K 线存储聚合的 30 条 P2 映射逐项复核，21/21 个唯一过滤器通过（3452 个测试跳过）。stop/limit/warning、fee/equity/result、codec/schema/lease、interval/source priority、extended session 与 calendar aggregation 均有 owner 证据；Go account/stream、内部 model helper、Futu adapter、GORM/provider wrapper 继续保持 partial/boundary，未把内部聚合测试升级为 exact。下一片继续 P2 Backtest/Calendar 存储与 execution，再转 MarketData/Quote、Storage/SQLite、Settings/Watchlist、API transport/MCP 与 Strategy/Pine；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：K 线连接/失败边界、聚合查询与 Pine shadow/adapter 的 30 条 P2 映射逐项复核，24 个证据过滤器因一个跨 binary 重复测试实际运行 25/25 通过（3448 个测试跳过）。writer lease/WAL、schema recovery、session/interval/calendar aggregation、stable sorting、Pine fee/corpus/EMA/MACD、worker 与 short-direction adapter 均有 owner 证据；Go SQLite/GORM fixture、stream sorting facade、PineTS shadow/worker process 与 callback wiring 继续保持 partial/boundary，未把跨 binary 测试升级为 exact。下一片继续 P2 Pine/Strategy 与 MarketData/Quote、Storage/SQLite、Settings/Watchlist、API transport/MCP，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Pine worker intent、atomic bracket 与 strategy execution adapter 的 30 条 P2 映射逐项复核，19 个证据过滤器因 strategy runtime/simulate/matcher 多 binary 重复实际运行 29/29 通过（3444 个测试跳过）。intent/worker、quantity/close sizing、bracket、lot/snapshot guard、warning/audit、broker failure 与 signal validation 均有 owner 证据；Go adapter/command executor、account/position fixture、worker process 与多阶段 audit 继续保持 partial/boundary，未把跨 binary 重复测试升级为 exact。下一片继续 P2 Pine/Strategy runtime、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 API transport/MCP，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Pine replay pump、source/planner、runner 边界与结果收集的 30 条 P2 映射逐项复核，19 个证据过滤器因 strategy runtime/simulate/matcher 多 binary 重复实际运行 23/23 通过（3450 个测试跳过）。replay consume/finish、source paging/recovery、request/command validation、runner fail-closed、warmup/Heikin-Ashi、quantity-percent sizing、market/lot fallback、incremental fills 与 result seed 均有 owner 证据；Go pump/stream callback、RunConfig/store setup、account/order callback、worker runner facade 与 warning aggregation 继续保持 partial/boundary，未把跨 binary 重复或 facade 聚合测试升级为 exact。下一片继续 P2 Pine/Strategy runtime、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与 API transport/MCP，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：结果收集、短向回放、市场规则与 K 线存储契约的 30 条 P2 映射逐项复核，27 个证据过滤器因 strategy runtime/matcher 多 binary 重复实际运行 29/29 通过（3444 个测试跳过）。增量成交、手续费/回撤、订单身份、weighted cost、反转/warmup、short replay、market rule override、K 线 schema/session/coverage/synthesis 均有 owner 证据；Go account/order callback、direct runner boundary、GORM/Futu fixture、SQLite introspection 与 provider wrapper 继续保持 partial/boundary，未把跨 binary 或聚合测试升级为 exact。下一片继续 P2 MarketData/Quote、Storage/SQLite、Settings/Watchlist、API transport/MCP 与 Pine/Strategy runtime 条目，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：K 线多级聚合、交易费用、Futu 生命周期与安全详情投影的 30 条 P2 映射逐项复核，27 个证据过滤器因 `shutdown`、`start` 通用过滤器和 engine production binaries 宽匹配实际运行 203/203 通过（3270 个测试跳过）。五分钟/十五分钟聚合、fee normalization/rounding/cap、HK/US 费用、replay guard、Futu close/recovery、tick session/extended quote、order-update cleanup 与 security detail wire shape 均有 owner 证据；Go store introspection、helper nil receiver、goroutine timing、fake OpenD、callback registration 与 full projection 继续保持 partial/boundary，未把宽过滤器或跨 facade 投影升级为 exact。下一片继续 P2 MarketData/Quote、Futu/OpenD、Storage/SQLite、Settings/Watchlist、API transport/MCP 与 Pine/Strategy runtime 条目，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：Futu 账户/高级协议、交易桥、能力权利与期权组合的 30 条 P2 映射逐项复核，36 个证据过滤器实际运行 36/36 通过（3437 个测试跳过）。account/funds 与 max-trade、prediction/advanced protocol、order identity、market-rule fallback、subscription lease、quote-right generation fencing、option/event combo legality、account impacts 与 transport failures 均有 owner 证据；Go OpenD/protobuf fixture、adapter state、callback seam、feature aggregation 与 edge helper 继续保持 partial/boundary，未把聚合或 catalog 映射升级为 exact。下一片继续 P2 Futu/OpenD、MarketData/Quote、Storage/SQLite、Settings/Watchlist、API transport/MCP 与 Pine/Strategy runtime 条目，再回补 Backtest/Calendar 尾项；helper/type-only、`optional_bool_strict`、`session_context_snapshot` 及其他无独立 Rust 测试条目仍保持未验证结论。
- 当前批次：收益日历、Futu 失败边界、搜索与账户/订单簿投影的 30 条 P2 映射逐项复核，31 个证据过滤器实际运行 31/31 通过（3442 个测试跳过）。earnings calendar 分片/去重/失败、OpenD unavailable propagation、subscription/decimal/tick/search normalization、funds margin/currency、security/interval mapping 与 order-book projection 均有 owner 证据；Go map mutation、fake exchange、protobuf fixture、pointer helper 与 round-trip adapter seam 继续保持 partial/boundary，未把跨 adapter 聚合升级为 exact。下一片改为优先 P1 高风险未验证条目，先处理明确缺少 Rust 独立测试或功能差异的项目，再回补剩余 P2 Futu/OpenD、MarketData/Quote、Storage/SQLite、Settings/Watchlist、API transport/MCP、Pine/Strategy 与 Backtest/Calendar 条目。
- 当前批次：P1 高风险 30 条逐项复核。Futu mixed realtime/delayed fallback 先红后修：`SharedTradeReadRuntime` 现在保留成功 realtime 结果，并在 fallback-only 且无可用结果时上抛原始 StockScreen 错误；专测 `trade_runtime_security_snapshots_preserves_realtime_when_delayed_fallback_fails` 通过，映射升级为 `function_exact`。Pine 先红后修补 `min`/`max`/`int` 内建，新增纯 request.security 与动态 for 步长回归；可选成员链和 Go sentinel/helper 继续保持 partial。运行时资源环境覆盖补入 Rust owner 证据；行情 payload、登录期间设置变化、ADK workflow/provider probe、旧 Wails/缓存/单飞 helper 逐条维持 partial 或边界，未把聚合证据升为 exact。
- 本批验证：Futu 专测、Pine 专测均通过；engine crate 全量 nextest 首次在既有 `adk_session_detail_omits_resolved_approval_groups` 并行运行时失败，随后单测重跑通过。mapping audit 已通过：Go 4451、Rust 3348，`function_exact` 1481，0 duplicate exact、0 nonexistent crate；anchor reconcile 1770/1724/0/0/46，无 stale 或 unrecorded。下一批继续 P1 高风险未验证项，优先 auth generation fence、provider timeout、workflow wait 与 market query payload 的可补 Rust owner。
- 当前批次：P1 高风险剩余 15 条旧 owner/边界项逐条复核。sidecar cache tree helper、Wails async startup cancellation、Go nil callback/context、PineTS object/collection recovery、indicator DSL alias 与内存 order cache 均确认没有 Rust 同形 owner，维持 partial/boundary；marketdata-helper 22/22、desktop 30/30、engine 75/75、strategy 108/108、架构检查全部通过。下一片继续 API transport、Assistant workflow/provider、Trading/Broker 与 MarketData/Quote 中仍有真实 owner 缺口的项目。

- 2026-09-28 Strategy/Pine cancel-all behavior evidence：新增两笔活动订单的部分失败回归，逐笔尝试撤单，成功/失败审计与聚合错误均已验证，失败订单仍保留 execution ledger tracking；定向 nextest 2/2 passed，receipt `sha256:b0b2788e6b2fb67563b2da0e1a864b5b691c8d67d22707b40bf7150842508512`。映射仍保持 `partial`，成功状态写入由 execution store owner 管理，strict 全局审计仍未通过。
- 2026-09-28 Assistant workflow CRUD behavior evidence：workflow tags trim/drop-empty/dedupe/sort、列表 limit=200→100、trigger logs 默认 limit=20、手动 trigger 空字段回退与 secretHash 脱敏先红后修完成；engine nextest 3/3 passed，receipt `sha256:98e9c06f94e261d4b0c53c0a7141ab81d861de627f5435ea45c07550092bee09`。映射仍保持 `partial`，删除后的完整 wire 状态与更多手动触发分支仍待补齐，strict 全局审计仍未通过。
- 当前批次：P1 策略生命周期与 Pine/Backtest 输入约束共 33 条映射逐项复核。Backtest 先红后修收紧无市场前缀且含空白 symbol 的默认回退；Pine 新增未知下单命名参数与 request.security `ta.sum`/advanced TA 拒绝回归。strategy runtime activity/reconcile、cancel/market-day、broker selection、shutdown/lease rollback、Pine process/session owners 均完成命令与证据复核；Go HTTP envelope、私有 helper、nil receiver、旧注入 seam 与跨 owner 聚合继续保持 partial/boundary。下一片继续 P1 API transport、Assistant workflow/provider、Trading/Broker 与 MarketData/Quote 的真实 owner 缺口。
- 当前批次：P1 API/Assistant/Trading/MarketData 与 Strategy 高风险未对齐共 31 条映射逐项复核。Assistant chat replay、provider validation、HTTP binding、SSE envelope、explicit broker/system validation、execution read、candle page/session、portfolio/workflow/approval、broker fallback/reconnect、strategy cancel/market-day/lease 均绑定现有 Rust owner，继续保持 `[~]` partial；未把聚合测试或路由形状提升为 exact。`executionCommandError` 是本批真实行为差异：先红后修补 `map_trade_error`，新增账户缺失、未连接、命令失败、超时、限流和市场不支持回归，并更新 cancel failure 为 `BROKER_NOT_CONNECTED`。下一片继续 P1 Assistant provider test wire、API SSE/WS 写失败与去重、Trading partial-fill 投影、MarketData subscription lease，以及 Storage/SQLite 与 Settings/Watchlist 的真实 owner 缺口。
- 当前批次：P1 Provider/Workflow、Futu/OpenD 生命周期与策略交易日边界共 31 条逐项复核；Futu 542/542、engine 1958/1958 通过。美股扩展时段 20:00 carry/DST 先红后修并升级一条 function_exact；Provider timeout、workflow wait/interactive、旧 helper、断连缓存刷新、Futu push/session registry 与严格 OpenD 字段检查仍保持 partial/边界，未用聚合测试冒充 exact。下一片继续 P1 Assistant/API transport 真实缺口，再回补 Trading/Broker、MarketData/Quote、Storage/SQLite、Settings/Watchlist 与剩余 Futu/OpenD。
- 当前批次：优先处理 30 条 P1/high 未完全对齐项，先修复 market-data sidecar `AssetBundle` 的篡改/符号链接、并发发布、过期清理与不安全 cache root 差异；helper nextest 26/26 通过。Assistant workflow、Futu/OpenD 与 Trading/Broker 条目逐项复核并继续保持 partial/boundary，未把接口聚合测试升级为 exact。下一片继续 P1 API/Transport、Assistant workflow 与交易订阅真实功能差异。
- 前一批次记录：处理 30 条 P1/high 未完全对齐项，先红后修收紧 Assistant provider-test 请求 timeout 到 30s；当时完整 ProbeProvider quick/full 能力探测仍缺失，workflow wait/interactive facade、Futu session/push、Trading order-update worker 与 MarketData singleflight cache 逐条复核，未把跨 owner 聚合升级为 exact。provider 定向 1/1、quick 2014/2014、workspace 3479/3479（2 skipped）与 7 条 replay 通过；后续批次已补齐 provider probe 证据。
- 当前批次（2026-09-26）：补齐 Assistant provider probe 的 quick/full、无 reasoning mappings、timeout cap 与 TestProvider 能力回写证据，策略 activity 日志/审计查询失败时的已知空页回归，以及 workflow invalid-input 的三路错误文案证据；相关映射升级为 `function_exact`。另补充 auth generation fence 的 ABA 回归。受影响 Rust 回归经 nextest wrapper 运行 8/8 通过（1959 项跳过）。当前审计为 Go 4451、Rust 3366、`function_exact` 1491、`partial` 2332、`boundary` 628；Parity 锚点 1780/1734/0/0/46，无未记录或陈旧锚点。
- 当前批次补充（2026-09-26）：Backtest/Calendar P1 取消边界先红后修。`CancelSync` 现在同时持久化 `cancelled` 并向 `BacktestSyncWorkerRegistry` 转发取消信号，确保在途 helper provider 请求被关闭；新增 `production_helper_sync_cancel_aborts_in_flight_request`，先在未修复实现上超时失败，再经 nextest wrapper 通过。该 Go 条目继续保持 `partial`：Rust 已覆盖生产请求中止时序，尚未复刻 Go `HistoricalKLineSyncer.Sync` 的直接 `context.Canceled` 与内存 progress 快照断言。

- 当前批次补充（2026-09-26）：API transport P1 auth generation fence 先红后修。`TestPasswordChangeDuringLoginCannotCreateOldPasswordSession` 新增阻塞密码验证回归，修复前 1/1 红（配置变更后仍创建 200 会话）；修复后 `SecuritySettingsService::with_login_configuration_fence` 在 session 持久化前以写锁复核单调配置代际，返回 `ConfigurationChanged`→409 `WEB_AUTH_CONFIGURATION_CHANGED`，并断言无会话残留；另以配置改动后恢复原值的 ABA 回归确认旧登录仍被拒绝。engine auth 定向 nextest 6/6、settings crate nextest 52/52 通过；该条由 `partial` 升为 `function_exact`。此前批次表格中的 partial 结论仅作历史快照。

- 当前审计收尾（2026-09-26）：新增 auth configuration-generation ABA 回归后，`audit_test_parity.py --write-report` 为 Go 4451、Rust 3366、`function_exact` 1491、`partial` 2332、`boundary` 628、`missing` 0；`parity_anchor_reconcile.py` 为 1780/1734/0/0/46。`audit_test_parity.py --strict` 仍失败，当前为 4085 个 function_exact evidence/receipt gaps，不能宣称严格审计通过。

- 当前批次补充（2026-09-27）：provider health backoff P1 先红后修。新增 `helper_restart_policy_defaults_match_go_provider_health_retry_delays`，修复前默认策略 500ms 首次退避使 Go 100ms 断言转红；修复后 `HelperRestartPolicy` 与 managed market-data helper 均采用 100ms 初始、1s 上限，逐档锁定 100→200→400→800→1000→1000→1000ms。定向 engine nextest 2/2 及 sidecar 回归 3/3 通过。`runtime_health_test.go:222` 映射升级为 `function_exact`；同测的 cancellation/error 透传仍由 `:240` 保持 partial。当前审计为 Go 4451、Rust 3376、`function_exact` 1496、`partial` 2321、`boundary` 634、`missing` 0；anchor reconcile 1786/1740/0/0/46。receiptDigest 保持为空，strict evidence/receipt gaps 仍未宣称通过。

- 当前批次补充（2026-09-27）：strict evidence batch 人工逐条复核 17 条单引用 `function_exact`（Assistant/ADK 11 条、Strategy/Pine 6 条）的 Go 与 Rust 测试体，补齐 reviewed assertion、reuse、anchor 与真实 workspace receipt。使用 receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`；未覆盖子断言的 session-negative、空数组与 stream transport 条目保持原结论。`audit_test_parity.py --strict` 缺口由 4085 降至 4021，整体严格审计仍未通过。

- 当前批次补充（2026-09-27）：`server_warmup_test.go:20:TestBacktestRouteUsesDerivedStrategyWarmup` 先以 ProductionBacktestPort definition-backed start 回归覆盖请求到执行链：保存 Pine `ta.sma(close, 20)` definition、注入 25 根 1m K 线，Rust 自动派生 `warmupBars=20`，worker 收到 20 根 warmup + 5 根 formal candles，并断言 strategy source 与 formal boundary；定向 nextest 1/1 通过。该条继续保持 `partial`：Go HTTP POST/GET envelope、fake worker order intent、撮合 trade/order-book/drawdown 及异步 polling 没有同形 Rust port/route 断言；mapping、batch scope、inventory 与 parity anchor 已同步，strict audit/evidence gaps 仍按全局报告处理。
- 当前批次补充（2026-09-27）：strategy_pine P1 warmup fallback 先红后修。`TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs` 新增 `estimate_security_source_bars_handles_period_and_timeframe_fallbacks`，逐例锁定 `security_source:day/hour/week` 的 period=0→0、canonical hour 空 interval→180、unknown symbol week/5m→780；修复 `resolve_timeframe_minutes` 的 hour canonical alias 缺失（修复前专测 1/1 红，hour 得到 1170）。修复后 strategy 定向 nextest 1/1 通过，mapping/reuse/anchor 已同步为 `[x]`/`function_exact`；当前审计为 Go 4451、Rust 3370、`function_exact=1493`、`partial=2332`、`boundary=628`，strict evidence/receipt gaps 仍不能宣称通过。
- 当前批次补充（2026-09-27）：API transport P1 显式 brokerId 读路由先红后修。`TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 对应 Rust 新增 `explicit_broker_selection_rejects_non_active_provider_before_reads` 与 `explicit_active_broker_selection_reaches_its_provider_owner`；修复前 active=yfinance、`brokerId=futu` 错误触达 yfinance helper，修复后 securities/snapshots/candles/depth 四路均在读取前返回 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED`，helper 请求数为 0，active alias 正向路径仍可读。新增 engine 定向 nextest 2/2 通过；因 Go 独立 broker reader 注入与 Rust active-provider 架构差异，该条继续 `partial`。mapping/reuse/anchor 已同步，未伪造 receiptDigest。
- 当前批次补充（2026-09-27）：Web auth state-map P1 先红后修。`TestWebAuthStateMapsStayBounded` 对应 Rust 新增 `auth_state_maps_stay_bounded_and_evict_oldest_entries`，真实写入 `MAX_LOGIN_ATTEMPTS+20` 失败 key 与 `MAX_SESSIONS+20` 会话，断言上限、最旧驱逐和最新保留；修复前会话表越界（148 条），将单次驱逐改为有界 `while` 后 engine 定向 nextest 3/3 通过。该条已升级 `function_exact`，映射、reuse 与 anchor 已同步。
- 当前审计收尾（2026-09-27）：全量扫描为 Go 4451、Rust 3371、`function_exact` 1493、`partial` 2330、`boundary` 628、`missing` 0；Parity 锚点 1783/1737/0/0/46。`audit_test_parity.py --strict` 仍真实失败，4084 个 function_exact evidence/receipt gaps（主要是历史 reviewed assertions、anchor、reuse review 与 receiptDigest 缺失），不能宣称严格审计通过。
- 当前批次补充（2026-09-27）：`TestMarketQueryAndExecutionPayloadFallbacksRemainDeterministic` 按 Go 断言拆分复核。Rust 新增 pathTail 正常/短路径回归，并新增显式 `limit=0`/负数→1 的 K 线路由回归；先红时旧实现把 `request.limit` 设为 200，修复后 6/6 精准 nextest 通过。blank limit 仍按 Go 语义落到默认 200，非法 limit/period 与反向窗口已有 fail-closed/fallback 证据。execution payload nil/不可编码回退和两个字符串 helper 没有同形 Rust seam，映射继续保持 `partial`。当前扫描为 Go 4451、Rust 3373、`function_exact=1493`、`partial=2330`、`boundary=628`、`missing=0`；anchor reconcile 1783/1737/0/0/46，未伪造 receiptDigest。
- 当前批次补充（2026-09-27）：Backtest/Calendar P1 分页 cursor 生产路径补测。新增 `production_helper_sync_rejects_broken_pagination_cursors`，真实 `ProductionBacktestPort` + HTTP helper 夹具覆盖 missing `nextBefore`、向前 cursor 失败，以及 cursor 到达 `since` 边界成功并落库 candle；定向 nextest 1/1 通过。映射仍为 `partial`，因为 Go 的可注入 `HistoricalKLineSyncer/source`、futu provider 与内存 progress seam 在 Rust 不同形，Rust 使用 yfinance helper 与 durable task。mapping/reuse/anchor 已同步，receiptDigest 保持空。
- 当前批次补充（2026-09-27）：Strategy/Pine P1 映射纠偏。`TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions` 原 partial 结论与当前实现不符；新增 `trend_stateful_ta_tests::analyze_script_supports_trend_and_stateful_ta_functions` 逐字复用 Go 脚本，真实断言 `analysis.ok=true`，定向 nextest 1/1 通过后升级为 `function_exact`，并同步 reuse/anchor。当前扫描为 Go 4451、Rust 3375、`function_exact=1494`、`partial=2329`、`boundary=628`、`missing=0`；strict 仍因 evidence/receipt gaps 失败，receiptDigest 保持空。

- 当前批次补充（2026-09-27）：Broker disconnected/degraded P1 断连契约完成审计。Go `broker-read.json` 与 `TestBrokerKLinesDisconnected`、`TestBrokerSecuritiesDisconnected`、`TestBrokerReadRoutesKeepValidDisconnectedShape`、`TestBrokerFundsEndpointReturnsDisconnectedSummary` 将 OpenD 不可达折叠为 200 degraded envelope；Rust 生产 `BrokerReadSnapshotPort`/trade runtime owner 由 `broker_read_routes_fail_closed_when_snapshot_port_is_unavailable`、`broker_read_fails_closed_without_trade_client`、`broker_klines_valid_request_fails_closed_without_historical_source` 及 securities missing-router 证据固定为 503 `BROKER_READ_UNAVAILABLE`，并保留后端错误文本。显式 test-cutover fixture 仍能回放 Go 200 形状，因此差异可复核；恢复旧 wire 会把无 source 状态伪装为空成功，当前按 boundary 保留，不改生产实现。对应 5 条 servercore/read-failure mapping 已改为 `evidence_type=boundary`、无待审 blocker；混合成功/失败的 service fallback 条目继续 `partial` 并写清三条 source-gated 路由边界。未伪造 receiptDigest。
- 当前批次补充（2026-09-27）：`TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts` 已由真实 OpenD mock 先经 global-state probe，再经 `SharedTradeReadRuntime` 与 production broker runtime route 完成逐字段回归；Rust 断言 connected startup、`serverVersion=10.9.7000`、markets/health、2 个 discovered accounts 及 REAL 优先排序，定向 nextest 1/1 通过。映射已由旧的 account/order reconciliation 证据行升级为 `function_exact`，并登记真实 workspace receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`（[receipt](verification-receipts/workspace-nextest-2026-09-27T061100Z.json)）；strict audit 仍有 4085 个历史 evidence/receipt gaps，不能宣称整体严格审计通过。
- 全量验证收尾（2026-09-27）：当前工作树 workspace nextest 3504/3504 通过、0 failed、2 个 suite skip；`check:rust` 的 SQLite/backtest/provider-runtime/trading-strategy/assistant/API/desktop compatibility replay 全部通过。真实结构化 receipt：`sha256:2f7422488ce7addc2b81ad38563b3b7662345283896be959843301f2b609d66f`（[receipt](verification-receipts/workspace-nextest-2026-09-27T071942Z.json)）。最新只读审计为 Go 4451、Rust 3376、`function_exact=1496`、`partial=2321`、`boundary=634`；anchor `1786/1740/0/0/46`；strict 仍有 4019 个证据缺口，未宣称整体严格审计通过。

- 2026-09-27 launcher/API P1 锚点收口：为 `main_test.go:86` 与 `:121` 的真实子进程测试补写 `// Parity:` 锚点，并在 zero-go gate 中加入仅针对这两个 provenance 注释的窄例外；两条 launcher 定向 nextest 2/2 通过，receipt 为 `sha256:bb5be96baa9877eba7e6d5f9f0f3455fd7d3608f5912999bd6e53cfc8f3bd757`。同批保留 Rust 与 Go 的 cache-wrapper 结构差异，不伪造同形 API。
- 2026-09-27 MarketData/Calendar P1 strict batch：`TestRetryDelaySequence`、`TestManagerRestoreReportsMalformedCachedSnapshot`、`TestManagerDiscardInvalidCachedSnapshotOnRestore` 逐条补 reviewed assertion、reuse、anchor 与真实联合 nextest receipt `sha256:9d3a85b571117001d7de7cb3a4badbf5f9825c308f243b51763b87426dc22115`（3/3 passed）。最新审计为 Go 4451、Rust 3377、`function_exact=1496`、`partial=2321`、`boundary=634`；anchor `1788/1742/0/0/46`；strict gap 降至 3957，仍未通过。
- 2026-09-27 Assistant session-context P1 strict batch：`TestSessionContextCompactionCreatesCurrentRevision`、`TestCompactSessionContextWritesContextNotice`、`TestMaybeAutoCompactSessionEmitsContextNoticeDeltas` 逐条复核并补 reviewed assertion、reuse、anchor；真实联合 nextest 3/3 通过，receipt `sha256:917fa4ba6a8484f68cb156733271d11cc5da94a702b60e52d1f91229a1ea5078`。strict gap 降至 3948；仍保留 Go 内存 seam 与 Rust durable session owner 的结构差异。

- 2026-09-27 Watchlist quote P1：`TestWatchlistQuoteSelectsExtendedSessionPriceAndChange`、`TestBatchQuotesCachesAndSingleflightsOverlappingRequests`、`TestBatchQuotesHonorsProviderCachePolicy` 先红后修。Rust batch-quotes 现在覆盖 Futu pre/after/overnight 扩展价与 change、重叠请求单次物理 snapshot read，以及 Futu 2.5s/helper 15s provider TTL；定向 nextest 3/3、Watchlist 相关 nextest 56/56 通过。三条均升级为 `function_exact`，receipt `sha256:ae6267da865221f941864dca401b67519a51c0a38b9c0069be36bd4010bad7e3`。SG.D05 未知市场时区、metadata 回写与 watchlist 端口级 delayed fallback/权限隔离仍保持 partial。当前审计为 Go 4451、Rust 3380、`function_exact=1499`、`partial=2318`、`boundary=634`、`missing=0`；anchor `1791/1745/0/0/46`；`audit_test_parity.py --strict` 仍真实失败 3927 个历史 evidence/receipt gaps。
- 2026-09-27 Calendar health strict evidence：复核 `TestManagerProbeMarksEmptyParsesUnhealthy`、`TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts`、`TestManagerProbeRecoveryClearsCurrentFetchError` 三条已有 calendar owner；补齐 reviewed assertion 与真实 receipt `sha256:27d4c070d9bbff94444923e57d51a73b4f62069b70fc6dd5fe357382c0e87bcc`（定向 3/3）。无生产代码差异，三条保持 function_exact；strict gap 由 3927 降至 3921，仍不能宣称严格审计完成。

- 2026-09-27 Backtest retry P1：`TestHistoricalProviderRetryExhaustionAndTimerCancellation` 先红后修。新增 helper 503 retry exhaustion（四次请求后返回最终错误）与取消退避回归；固定 timer 导致取消测试超时，修复 Futu/helper 重试等待为共享取消感知轮询后定向 nextest 2/2 通过。映射升级为 `function_exact`，补齐两个 Rust evidence、同一 `Parity:` anchor、单引用 reuse、reviewed assertions 与 receipt `sha256:26dc42121e439058156a76cfe41acd1d0c041eb7104b2f320d99861e119019dc`。当前审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1792/1746/0/0/46`；strict 仍真实失败 3921 个历史 evidence/receipt gaps。

- 2026-09-27 Trading/Broker strict evidence C 批：逐条复核 `TestDerivativeSingleLegRequiresBrokerPreviewAndStableClientID`、`TestRealTradeControlPlaneFailsClosedWhenPersistedStateCannotLoad` 与 `TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions` 的 Go/Rust 断言；三条已有 `function_exact` 均保留，补齐 `assertionCoverage.source=reviewed` 与单引用 reuse/Parity anchor 的真实联合 nextest receipt `sha256:77f918d03712f238ef42d3340f4cc5c73f951a0406e534d74b4c3ad251cfe258`（3/3 passed）。本批无生产代码变更；Rust 对 control-plane 的错误码更明确（500 `CONTROL_PLANE_UNAVAILABLE`），失败关闭语义与 Go 一致。当前只读审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1792/1746/0/0/46`；strict 仍真实失败 3915 个历史 evidence/receipt gaps。
- 2026-09-27 Query/MarketData strict evidence：复核时间 query、重复 candle sessions 与 route normalization 三条 P1 exact。补充两个 query `Parity:` anchor；route 测试先红后修，直接断言 provider 路由序列 `RTH/RTH/ETH/ETH/ALL/ALL` 及去重后的三条唯一路由。定向 nextest 3/3 通过，receipt `sha256:6d383be156db7803a762e52849c2d20b09d245fe42aac931543f27d95ee59496`；mapping/reuse/五份 parity 文档已同步。最新 strict audit 为 3901 个历史 evidence/receipt gaps，整体仍未通过。
- 2026-09-27 Futu/OpenD K-line session strict evidence：复核 `TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults`、`TestQueryKLinesForSessionsFiltersUSHistoricalRoutes`、`TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight` 与 `TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage`。扇出测试先因 `extended_time` 的 Option 断言编译失败，修正为显式 `Some(true)` 语义，并补齐 pre/regular/overnight 标签；四条映射均保留 `function_exact`，补 reviewed assertion、现有单引用 reuse/Parity anchor 与联合 nextest 5/5 receipt `sha256:cad7100b896a7aab3714ef4187d68e428db7e84f5585bef76c92fa807e593cfb`。`TestHistoricalKLineSessionHelpersFilterAndPlanExplicitSelections` 的 filterKLinesBySessions 子断言尚未由同一 Rust owner 证明，继续保持待复核，不机械升级。最新审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1793/1747/0/0/46`；strict gap 3892，整体仍未通过。
- 2026-09-27 API transport auth strict evidence：五条 P1 exact（Origin/CSRF、PATCH 会话写、浏览器禁用页与 engine 状态、密码会话读写）完成断言复核；联合 nextest 5/5 通过，receipt `sha256:c5457db228673e47ae252a104992aa8c261fce3df081f60dfd28cf7c6682cc17`，mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3892 降至 3885，整体严格审计仍未通过。
- 2026-09-27 Assistant/API strict evidence：五条 P1 exact 复核其全部 Rust owner（含 composer、live stream 与 disconnect timing），联合 engine nextest 8/8 通过，receipt `sha256:fb75c71c988292130d6e3f017e66f1a2a3de3f3d19b3ec106a6af3ca84da3c7e`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3885 降至 3875，整体严格审计仍未通过。
- 2026-09-27 Backtest/API strict evidence：五条 P1 exact 完成 Go/Rust 断言复核，联合 engine nextest 10/10 通过，receipt `sha256:fda37329aa034a644056012e122883e27d99a6586ad7bf82003d3af39f110f6a`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3875 降至 3870，整体严格审计仍未通过。
- 2026-09-27 MarketData boundary strict evidence：五条 P1 exact 完成断言复核，联合 engine nextest 6/6 通过，receipt `sha256:baa66579a6a1866149f08707d9e80790638ec1fb29f48ed34962a50ae5f17478`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3870 降至 3865，整体严格审计仍未通过。
- 2026-09-27 MarketData/API forwarding strict evidence：五条 P1 exact 完成 Go/Rust 断言复核，联合 engine nextest 11/11 通过，receipt `sha256:a583ff41724cfc3bcd259fcf9627a7c8594e6a9aa5c4a79e0082cf332a71c622`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3865 降至 3860，整体严格审计仍未通过。
- 2026-09-27 Execution/Watchlist strict evidence：五条 P1 exact 完成 Go/Rust 断言复核，engine/store-sqlite 联合 nextest 10/10 通过，receipt `sha256:53f26ac779a8505c741acb4ea4e9f6316da471fb6e04f0eeeea7b823f63e301b`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3860 降至 3855，整体严格审计仍未通过。
- 2026-09-27 Runtime/Maintenance strict evidence：五组 P1 exact 完成 Go/Rust 断言复核，跨 engine/store-sqlite/integration-futu/marketdata/desktop 联合 nextest 10/10（live OpenD suite 保持 ignored）通过，receipt `sha256:b58fc1c29238f5349c80b07263e7375b665440d25cbe46ae45201a4903967df6`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3855 降至 3850，整体严格审计仍未通过。
- 2026-09-27 Market HTTP strict evidence：五条 P1 exact 完成 Go/Rust 断言复核，engine nextest 5/5 通过，receipt `sha256:4c4bec4be84d55d241713c361ec13c8feb7011c4863b51821d1b51df8c679a80`；mapping assertionCoverage/reuse/verification 已同步。strict gap 由 3850 降至 3845，整体严格审计仍未通过。
- 2026-09-27 Helper/runtime strict evidence：复核 `TestSidecarManagerStartsReusesAndStopsManagedExecutable`、`TestOSSidecarProcessBoundsGracefulStopBeforeKillingChild`、`TestCheckNodeRuntimeDependencyUsesMacOSCommonPathFallback`、`TestRuntimeDependenciesAggregatesRequiredStatus`、`TestDesktopDevelopmentWebListenerStillRequiresPasswordSession` 与 `TestStrategyRuntimePanicAutoReconcilesToStopped` 六条 P1 exact；marketdata-helper/engine 联合 nextest 7/7 通过（strategy runtime owner 同名测试覆盖 lib 与 bounded integration binary），receipt `sha256:52ef8cc1e2ccf7d57130269388da3d90544630132116f3cc4cf9c431d7de2c51`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3833，整体严格审计仍未通过。
- 2026-09-27 API portfolio/settings strict evidence：复核浏览器会话安全设置、portfolio cash fallback/降级空态/已移除路由与 broker settings 默认值五条 P1 exact；api/engine/settings-file 联合 nextest 5/5 通过，receipt `sha256:cf733b63c6b03f627d273983d92eff928ecb09de97b1f79be6c6c95d04e3ee76`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3823，整体严格审计仍未通过。
- 2026-09-27 Futu marketdata runtime strict evidence：复核 trade→quote cache inheritance、fallback instrument filtering、fallback ticker/snapshot projection 与 stale connection-generation fencing 五条 P1 exact；integration-futu/marketdata 联合 nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:57f5e6283bc3618f2b76411d6374d775dc85c6a8e939d6512d8c0615c06b4899`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3813，整体严格审计仍未通过。
- 2026-09-27 Assistant boundary strict evidence：复核 MCP token/tool catalog、session context fallback、pending input cancellation、closed-stream approval resume 与 missing-run continuation 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:7f97c2a3cc83a59db7c1bfa55be2233314f69e89bf29de08c35c2ba56592506`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3803，整体严格审计仍未通过。
- 2026-09-27 Futu K-line pagination strict evidence：复核 latest-page exclusive cursor、OpenD market-time/UTC conversion、exact-second cursor boundary、page dedupe/sort 与 inclusive range 五条 P1 exact；engine/integration-futu 联合 nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:489957b0b5e2b5d8f04047abee16c357751def89d390ec871a2016be66b24c70`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3793，整体严格审计仍未通过。
- 2026-09-27 API/marketdata tail strict evidence：复核 provider switch retirement、runtime status wire normalization、cookie-only WebSocket upgrade、跨交易日 tick volume delta 与 candle pagination metadata 五条 P1 exact；engine/marketdata 联合 nextest 5/5 通过，receipt `sha256:7612368c8b4ebc983e552ba11b03a5ede9cad29575c83c5728c8578411901a69`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3783，整体严格审计仍未通过。
- 2026-09-27 Backtest/Calendar/Settings strict evidence：复核 DST market-date range、corrupt calendar snapshot rejection、跨年 snapshot cache index、empty broker defaults 与 rejected watchlist provider switch 五组 P1 exact；engine/calendar/settings 联合 nextest 6/6 通过，receipt `sha256:e2ffadfff2d2d62a4d697f5bf9a65f47f446f2b60316848ed77cac4d6517ae34`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3774，整体严格审计仍未通过。
- 2026-09-27 Trading/Broker control-plane strict evidence：复核空/损坏控制面状态、hard-stop 审计持久化失败粘性降级、option combo 生命周期、forced reconciliation wake 与 capability route-table 五组 P1 exact；engine nextest 9/9 通过，receipt `sha256:6246b9ab3c1d3050579515161c6005f3103a8eb2ac583573a6369db130eeb2bc`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3764，整体严格审计仍未通过。
- 2026-09-27 Futu subscription reconciler strict evidence：复核 provider switch 延迟物理释放、OpenD ack retention/retry timing、closed-connection ownership cleanup 与 never-established record drop 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:c4734a37b0151e32c4e87c9b949f9ae10fa286364e4e337484fd9eb1b2bb1577`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3754，整体严格审计仍未通过。
- 2026-09-27 Futu OpenD boundary strict evidence：复核 request timeout stale-waiter isolation、keep-alive shutdown、history optional-field/empty-result handling、closed-session depth rejection 与 disconnected trading reads 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:b10f6c20a8a268cddbaed2a56114e7e63c4a24caad8e1cd6a856d6c70d17c420`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3749，整体严格审计仍未通过。
- 2026-09-27 Futu session/trade strict evidence：复核 K 线 session selection/annotation/default validation、US previous-close session rules、trade-account authority fallback 与 mainland Shanghai location fallback 七条 P1 exact；engine/integration-futu 联合 nextest 7/7 通过（live OpenD suite 保持 ignored），receipt `sha256:9b9427f38df767a2d63f217953fd27bae053fda603622c86d9a7ef33d51b3814`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3739，整体严格审计仍未通过。
- 2026-09-28 Assistant session-gate strict evidence：复核 auto-compaction gate exclusion/free-path、workflow active-parent compaction、pending-approval protected tail 与 approval-waiting active-run detection 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:c6e22cccbab1c95b191598b13684cc7d6632461fc80d06f8fbed43fb736981b2`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 3729，整体严格审计仍未通过。
- 2026-09-28 Futu subscription/trade strict evidence：复核 `TestExchangeSubscriptionCacheUpdatesOnlyAfterOpenDConfirmation`、`TestConnectionGenerationInvalidatesClosedSessionAndItsSubscriptions`、`TestExchangeCloseIsTerminalAndPreventsOrphanedReconnect`、`TestFailedConnectionDoesNotAdvanceEstablishedSessionGeneration`、`TestTradeWriteMethodsEnforcePrerequisitesAndDisconnectedState` 与 `TestModifyOrderReturnsStableEmptyResult` 六条 P1 exact；integration-futu + engine 联合 nextest 6/6 通过，receipt `sha256:66b32e68a91fcc48f6557dcb436c9ea2d5d16af485d6418fcddc83bb83a25dc1`。mapping assertionCoverage/reuse/verification 已同步；当前审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`，anchor `1793/1747/0/0/46`；strict gap 降至 3723，整体严格审计仍未通过。
- 2026-09-28 Futu protocol strict evidence：复核 prediction catalog identity/pagination、research catalog OpenD request injection、history K-line next-key pagination、history order filters 与 user-security business error 五条 P1 exact；integration-futu nextest 5/5 通过，receipt `sha256:6478da8aebc98d804488cf76f032ff5ceaf9cc33c3a3fd38aaff652d8355429a`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3718，整体严格审计仍未通过。
- 2026-09-28 Assistant session-context strict evidence：复核 read-pressure no-auto-compact、model payload auto-compaction 与三条 protected-tail approval selection 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:0ca3d7f2e3b1e564c6fc25e2a91823516d7de78fb8e56d03ee042f9a272c92da`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3713，整体严格审计仍未通过。
- 2026-09-28 Futu K-line boundary strict evidence：复核 declared candle period 双协议映射、broker K-line cursor/time 校验、分页 helper session/listing 边界、未知 period catalog 与 Session_ALL fallback 五条 P1 exact；integration-futu + engine 联合 nextest 5/5 通过，receipt `sha256:f38ccd784ac1bf1afd47e5a14600fc1f016af24883a0787fb35b97b78f55801a`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3708，整体严格审计仍未通过。
- 2026-09-28 Futu snapshot/listener strict evidence：复核 security snapshot 市场分批与深拷贝缓存、滑动预算/失败不缓存、远端限流与取消、空/异常结果，以及 malformed basic-quote push 丢弃五条 P1 exact；integration-futu + engine 联合 nextest 5/5 通过，receipt `sha256:5843770babe2087467db4d181f4c556dcea44dcf03a89385610e9ded8179dad8`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3703，整体严格审计仍未通过。
- 2026-09-28 Assistant store-ops strict evidence：复核 session agent ownership、cancel 事务同时 deny pending approvals、missing run 专用失败、missing approval 幂等空 envelope 与 run listing filter/sort 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:66c0e83d636b8f8045e81d60c6cd382808d168af88f8d211cc8739dcdc9df635`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3698，整体严格审计仍未通过。
- 2026-09-28 Assistant session-context2 strict evidence：复核 provider override window、revision compaction、无 revision handoff、compaction 期间追加事件与 pending approval 原始调用保留五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:83808bfd4b00fe6f1aacfe581f8ee1453badfe7659bff03d5d281583ba62163a`。mapping assertionCoverage/reuse/verification 已同步；按 strict 审计实际 gap 降至 3696，整体严格审计仍未通过。
- 2026-09-28 Assistant runtime strict evidence：复核 provider tool ordering、tool cancellation join、provider probe timeout cap、approval lease cancellation 与 session title reuse 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:cbeb1497d9d8ee4db2dbafb58ab65670dd8abbd74d3c020007e4ab127b04b211`。mapping assertionCoverage/reuse/verification 已同步；按 strict 审计实际 gap 降至 3695，整体严格审计仍未通过。
- 2026-09-28 Futu history-window strict evidence：复核 explicit session planner、连续分页与 latest limit、超过八页预算、上游 page-size 放大/钳制及 payload-less security info empty collections 五条 P1 exact；integration-futu nextest 5/5 通过，receipt `sha256:603f4b41c4847b7f46c69791b46609ecb78503502e1118929ce1f5d5d9f09338`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3692，整体严格审计仍未通过。
- 2026-09-28 Assistant store-lifecycle strict evidence：复核跨库 session cascade cleanup、session page filter/pagination、composer state normalization、approval updated-at ordering 与 provider request-timeout persistence 五条 P1 exact；store-sqlite + engine 联合 nextest 5/5 通过，receipt `sha256:df78dcad3dcd67f0e6dc10d4c57da16dee947f4114ebfdeb509578e2df9bc07e`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3687，整体严格审计仍未通过。
- 2026-09-28 Assistant run-time strict evidence：复核 terminal cancel audit idempotence、configured run timeout freeze、timed-out goal resume window、expired run reconciliation 与 per-run timeout window 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:172dd3f940aca79bcb55c464e065550bf98f4273f0ff1559bba3f44ad9e5560c`。mapping assertionCoverage/reuse/verification 已同步；当前 strict gap 降至 3682，整体严格审计仍未通过。
- 2026-09-28 Futu notification/probe strict evidence：复核 `TestLiveNotificationFromResponseRoutesProtocolPayloadsToNeutralCategories`、`TestNeutralNotificationBuildersHandleNilAndStatusTransitions`、`TestProbeOpenDReportsClosedPortAsDisconnected`、`TestProgramStatusStringHandlesMissingPlainAndDescribedStatus` 与 `TestMarketSessionsForCandleSessions` 五条 P1 exact；integration-futu nextest 实际 6/6 通过（同名 partial session helper 一并命中，live OpenD suite ignored），receipt `sha256:ae8a2c04479d9119e93e169d4f3e3a048488f478e60b591242891cc9c4040d6a`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3672，整体严格审计仍未通过。
- 2026-09-28 Futu subscription-reconciler strict evidence：复核 physical subscription sharing/deferred release、concurrent idempotence、unsubscribe retry/reacquire reset、BasicQot delayed fallback 与 connection replacement quota reset 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite ignored），receipt `sha256:c3d81229c5939b1e25203f0f1c269d609600a0ed226da538c5fc7b1e8bd5e3e0`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3662，整体严格审计仍未通过。
- 2026-09-28 Futu client-recovery strict evidence：复核 recoverable replay policy、closed-ready session replacement、notification callback reconnect deadlock、minimum-version rejection 与 typed init/session transport failures 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite ignored），receipt `sha256:8691574cb891a5f89c932ccd431f408bf4c3893654277016ba7e57c0c62ace54`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3652，整体严格审计仍未通过。
- 2026-09-28 Futu quote/empty-boundary strict evidence：复核 empty instrument normalization、order-book detail/empty snapshot、basic-quote subscription/rejection/empty handling、duplicate quote projection 与 security-snapshot invalid/payload-less rows 五条 P1 exact；integration-futu nextest 实际 10/10 通过（live OpenD suite ignored），receipt `sha256:65f7118836c30241fe900c00e4cb632ce9c8ead298e4c7d90e93f87bab8c69c1`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3642，整体严格审计仍未通过。
- 2026-09-28 Assistant engine-gates strict evidence：复核 strategy optimize task persistence、disabled MCP runtime lifecycle、agent/provider resolution boundaries、external-unavailable tool catalog 与 tool-error retryability 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:484a697d804103853875d6d7e2d34429dc85981bc916d7568cb3e1fecca74a49`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3632，整体严格审计仍未通过。
- 2026-09-28 Futu mixed-boundaries strict evidence：复核 tick conversion fallback、desired subscription ref normalization、multi-viewer stale order-book release、HK-only high-dividend state 与 quote-right entitlement cache refresh 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite ignored），receipt `sha256:77b7aeff1ac829b88bffc92e012affa9c99f2f1b01599c1d957e5d7a1cac473f`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3622，整体严格审计仍未通过。
- 2026-09-28 API market-runtime strict evidence：复核 tick candle fresh-cache、cache-miss provider ingest、ticker-error retained fallback、industry forwarding provider kinds 与 strategy order-cancel dispatch 五条 P1 exact；engine nextest 实际 7/7 通过（strategy owner 同名测试覆盖 lib 与两个 bounded integration binaries），receipt `sha256:ea1a72725d8f5cd97f2e15017b44283975b76e1aef4d2af17bab7a72ef24009b`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3612，整体严格审计仍未通过。
- 2026-09-28 Futu research/boundary strict evidence：复核 K-line session/price helpers、research catalog local pagination、economic calendar pagination/envelope、disconnected market reads 与 snapshot-fallback wire coercions 五条 P1 exact；integration-futu nextest 实际 9/9 通过（live OpenD suite ignored），receipt `sha256:b32a88c5f551e7f3b00501146b2f5b3e32ec0a931078327803c0bcd36c2d493c`。mapping assertionCoverage/reuse/verification 已同步；strict gap 降至 3602，整体严格审计仍未通过。
- 2026-09-28 Query/candle/depth strict evidence：复核 `TestTickCandlesVolumeWindowAndLimit`、`TestNormalizeCoreCandleQueryAcceptsSessionParameterShapes`、`TestNormalizeCandleOptionsAcceptsSessionsAndAdjustments`、`TestMarketDepthEmptyOrderBook` 与 `TestParseSessionScope` 五条 P1 exact。Go 断言逐项对照 Rust owner；candle options 两个 Rust owner 补齐 Go 行 8 anchor，未把相邻行 9 误记为来源。integration-futu/engine/marketdata nextest 实际 5/5 通过（live OpenD suite ignored），receipt `sha256:69bae53794658676a710eeda191d4e68b5e70c7f243ee3483c441f7919d75b40`。mapping assertionCoverage、anchor、verification 已同步；strict gap 降至 3581，整体严格审计仍未通过。
- 2026-09-28 Provider facade/search strict evidence：复核 Futu search 前置拒绝、industry 默认 kind、news nullable/asOf 投影、company financials 转发与 provider 默认市场五条 P1 exact。Go/Rust 断言逐项核对；engine/calendar 定向 nextest 5/5 通过，receipt `sha256:143648491d86b0a5b7e9495ef6dd97bffc10d6797e5bd08e24396b332a94c20e`。随后 `pnpm run check:quick` EXIT=0，2017/2017 Rust tests passed；mapping reviewed assertion 与 verification 已同步，全局 strict gap 降至 3571，严格审计仍未通过。
- 2026-09-28 Futu funds/K-line/snapshot boundary strict evidence：五条 P1 exact 完成 Go/Rust 逐项复核，拆分覆盖 11 个 Rust owner；integration-futu/engine nextest 11/11 通过（live OpenD ignored），receipt `sha256:9304c9ef4cd2b9a4d36385012db4902c61c41bad326f7472345fdcafc0a3bbf7`。mapping reviewed assertion 与 verification 已同步；strict 审计待重跑确认，整体仍未通过。
- 2026-09-28 Assistant workflow CRUD follow-up：对 `TestWorkflowResourceCrudPaginationAndLogs` 继续先红后修，补齐 workflow 更新字段归一、列表 status/offset、trigger log 的 workflow/trigger/status 过滤、删除响应 `DISABLED`/`deletedAt`、重复删除与删除后单条读取失败，以及 schedule/market-threshold 非法配置 400 分支。定向 engine nextest 3/3 通过，receipt `sha256:1a5f94840bd784348b30d979083cac9e64b2bee2be0a9bdb7b7bef961d1cb4df`；随后 `pnpm run check:quick` 与 `pnpm run check:rust` 均通过（workspace nextest 3514/3514，2 skipped；7 类 compatibility replay 全通过）。映射仍为 `partial`，workflow webhook/manual 运行错误矩阵与更多完整 wire 字段继续保留差异；当前 strict audit 仍失败，3546 个历史 evidence/receipt gaps 未宣称通过。
- 2026-09-28 Strategy/Pine subscription warmup rollback evidence：新增 `failed_pine_warmup_releases_only_its_own_subscription_once`，以共享 KLINE consumer 验证 Pine 预热失败后只释放运行时自身租约，重复 release 不会误删另一 owner；engine nextest 4/4 passed，receipt `sha256:49469ad896186b5f59c614381ff653c63720c7df59c98fd01cfb557c937b01e9`。`TestSubscriptionLeaseAndWarmupFailuresRollBackRuntime` 仍保持 `partial`，租约获取失败时不创建运行时的失败 seam 尚未迁移。
- 2026-09-28 Strategy/Pine subscription normalization correction：`strategy_kline_subscription_refs_normalize_valid_targets_and_skip_malformed` 先红复现 `market=\".\"` 畸形目标未被跳过，修复 mutation owner 后锁定唯一 `KLINE:US:AAPL:15m` 引用；engine nextest 1/1 passed，receipt `sha256:38704e6be9b4cbe98401245af9407ae949eafb5e2a2df0eced6ebdbda5f2b654`。对应 Go 条目升级为 `function_exact`。
- 2026-09-28 Strategy/Pine manager close 聚合错误：`TestManagerCloseAggregatesNamedSessionErrorsOnce` 对应 `manager_shutdown_aggregates_named_session_close_errors_once`，真实 engine manager 以两个活跃 Pine session 验证 12 个并发 shutdown caller 收到相同的 `instance + market.symbol + pine session close` 聚合错误，并断言每个 session 只发送一次 close；旧 `shutdown()` 保持 bool 兼容，production port 透传聚合错误。定向 engine nextest 4/4 passed，receipt `sha256:e319b1dd01b56b3ae8bb821aa23bd831c2eb4ca5216765035961fd92f0bdd185`。该条由 partial 升为 function_exact；启动竞态聚合的相邻 Go 测试仍保留 partial，严格全局审计仍未通过。
- 2026-09-28 Strategy/Pine targeted cancel P1：`TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders` 对应 `targeted_cancel_only_mutates_owned_active_orders_and_removes_successful_tracking`。新增 execution store 真实 owner 回归，覆盖成功撤单移除 active tracking、gateway 失败保留 tracking、foreign/untracked 不触达 gateway 三条行为断言；先前 partial 升为 `function_exact`。定向 engine nextest 3/3 passed，receipt `sha256:3423dac32787c4000997a77e747bdf13c64a024334cf4c27e9e341eb47d25fa3`；全局 strict audit 仍未通过。
- 2026-09-28 Strategy/Pine cancel-all success P1：`TestLiveCommandExecutorCancelAll` 新增 store-backed execution owner 回归，覆盖两笔 active order 逐笔派发及成功后 active ledger 清空；失败聚合/保留跟踪由相邻测试覆盖。该条由 partial 升为 `function_exact`，定向 nextest 9/9 passed，receipt `sha256:fc615c228ba5af017d074b6793421fc3e624cc447a0abd801c9a8ad8c8eb6901`；strict audit 仍未通过。

- 2026-09-28 Strategy/Pine lifecycle transition P1：针对 `TestServicePauseAndStopInstancesStopRuntimeAfterStateTransition`，先红复现状态写入失败时 Stop 提前取消 runtime、Pause 成功后 runtime 仍存活；现由 mutation owner 先 CAS 写入 `STOPPED`/`PAUSED`，再取消 task 并释放 market-data demand。新增 `lifecycle_transition_failure_does_not_stop_runtime_before_state_write` 与 `pause_and_stop_transition_state_before_stopping_runtime`，定向 nextest 4/4 passed，raw output `sha256:6d761bf44026ffcc680076f180a0f83335a640c2e406df004a2489bfac747546`。该条仍为 `partial`：Go refresher callback 的两次刷新计数没有 Rust 同形注入 seam，生产 router reconcile 不冒充计数证据。

- 2026-09-29 Strategy/Pine targeted cancel alias：`TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings` 先红确认 Rust 对陈旧意图返回 ownership error，随后修复 `dispatch_cancel_intent`：按 execution-store 活动订单解析确定性 clientOrderId alias、internal id 去重、逐笔撤单，并将无活动映射收敛为幂等 no-op。定向 nextest 3/3 passed，receipt `sha256:a3e1e7ac8664d078feccbdd1fd234bb2abf8e0ec6e2e51a2d3ad5c43de7f9fc7`；映射仍为 `partial`，显式多腿 `activeOrderAliases` owner 尚未存在，strict audit 仍未通过。
## 2026-09-30 API SSE strict evidence batch

API Server/Transport Wire 批量收口 5 条 SSE exact：序列化/写入错误、无 callback trigger 取消、并发 writer 串行化、flush panic。逐条复核 Go/Rust 断言；Rust 取消路径补显式 task-success assertion。`jftrade-api` nextest 5/5 passed，receipt `sha256:23cb9085910da407cffce794533dee1ff1eea1154a3a50a766fa6afd67ed9ec9`。5 条 assertion coverage 升为 `reviewed`，strict gap 3395→3391；全局 strict 仍失败。
## 2026-09-30 Futu transport strict evidence batch

5 条 Futu transport exact 完成 Go/Rust 断言复核：trade reads、quote/K-line/order-book、trade writes、response-lost no replay、closed-client subscriptions。联合 nextest 5/5 passed，receipt `sha256:a0906c826c136f6c21afaaf156d85bc399947b9047e47431aa33fcc242ad4290`；补齐 Parity anchors/reviewed/receipt。strict gap 3385→3378，整体仍未通过。

- 2026-09-29 API Server/Transport Wire SSE frame-loop strict evidence：复核 5 条 Go exact（trigger/ticker 错误传播、write panic、PrepareSSEWriter frame/header/no-retry、无 flusher 拒绝、initial 错误传播），Rust 以 6 个 lib/transport 行为断言联合覆盖，`jftrade-api` nextest 7/7 通过；receipt `sha256:8f5cf46fdb2e9bcd5af4d3de9b52781eb5cbbcc4af2ebdde8c267252e0c05ec4`。映射 assertion coverage 已升为 reviewed；strict gap 3378→3368，整体仍未通过。

- 2026-09-29 Futu P2 strict evidence：复核并收口 3 条已有 exact（默认 OpenD 地址、Qot_Sub 订阅/退订 framed wire、零当前价 previous-close 回退），补齐正确 Go 行 anchor 与 reviewed assertion；engine/Futu nextest 3/3 passed，receipt `sha256:c121cb50db0c539b0df79c1bea408c2aaa59111be0295c6b9169b2bc875b30c2`。strict gap 3362→3353，anchor 1890/1843/0/47，整体仍未通过。

- 2026-09-29 Futu history seam correction：`TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart`、`TestRequestHistoryKL` 经断言核对后由 legacy exact 收窄为 reviewed partial；Rust 仅证明 helper/frame 子行为，未覆盖 Go 真实 client response。integration-futu nextest 2/2 passed，receipt `sha256:635332bacd351da3b2c6735ed4f467aa441967d1229b6c2170c5297b2e39f2d3`；strict gap 3347→3341。

- 2026-09-29 Prediction push P1：注册幂等、重复/无 sequence、key 归一、5 秒 freshness 的 Rust 行为断言 3/3 passed，reviewed assertion + anchor + receipt `sha256:be911c287f663bb6151394cb091665b1e326ea59b6d42a81c4ecf3bd99a7bbf9`；strict gap 3353→3350。
- 2026-09-29 Assistant research backtest P2：三种模式均不需审批且 allowed 的 Rust 断言 1/1 passed，reviewed assertion + anchor + receipt `sha256:b4abf64334b6bc0f6d636a85b5acd3a32e9b2acbbb08e2b2fe278c7a2c487843`；strict gap 3350→3347。
- 2026-09-29 Candle adjustment P2：Rust parser 与 route/provider-before-call 断言 2/2 passed，reviewed assertion + 双 anchor + receipt `sha256:ef32d0c83f7b0df316aef3ae093965f5fc9970bed9693ef6fbdacb3a41c86edd`；strict gap 3341→3338。
- 2026-09-29 exact 纠偏：Futu funds、broker order 枚举转换与 Pine asset metadata/missing-empty 四条由未覆盖全断言的 legacy exact 收窄为 reviewed partial，strict gap 3338→3323；下降只表示错误 exact 被移除，不计新增行为。全局 strict 仍失败。
- 2026-09-29 Futu snapshot fallback strict evidence：六条已有 exact 完成 Go/Rust 断言复核，覆盖 StockScreen 行/market 投影、输入边界与取消、strict delayed quote wire、无订阅 static-id 回退、15 秒正负缓存和错误透传。`jftrade-integration-futu` nextest 6/6 passed，receipt `sha256:5d2e8d55218adc5252b1389a258dbf15803b757a9bb05091ccd10b78c56cb8f6`；六条 assertion coverage 升为 reviewed，strict gap 3323→3312。整体 strict 仍失败，下降按实际 reviewed 行为证据计入，不按测试总数或 receipt 数量计入。

- 2026-09-29 API auth/SSE strict evidence：SSE loop 2 条与 auth middleware 3 条已有 exact 完成 Go/Rust 断言复核，分别以 `jftrade-api` nextest 2/2、`transport_contracts` nextest 3/3 通过；receipt 分别为 `sha256:803a99b17ac95ec97490cbf3f1f91ee8c3aa95d061ee9966dfdd9217f3bae18e`、`sha256:d9491a62ef3b46d0ff5ee03f652748d32fa1bd219460c95a0edd7e3748702b89`。同时将 public-path 与 logout 状态码证据不足的两条 legacy exact 收窄为 reviewed partial；strict gap 3303→3297，整体仍未通过。

- 2026-09-29 API auth rejection follow-up：`TestAuthRejectsNilAuthenticator` 与 `TestAuthRejectsUntrustedOrigin` 完成断言复核，`jftrade-api` transport contract nextest 2/2 passed，receipt `sha256:d302cddef14057dec554dd8d48e00892a71e7c5a2e4c565fff7010970fbe800e`；strict gap 3297→3293，整体仍未通过。

- 2026-09-29 API P1 Web/Execution follow-up：禁用 Web 导航、cookie+CSRF 浏览器流程、cookie-only WebSocket 三条 reviewed，nextest 3/3，receipt `sha256:02d835502c6392862c7678ed3087c19e51198186c265a1583cc181bbac8d5b92`。ETH execution session 的 normalization/wire 证据 3/3 passed，receipt `sha256:8abeb1dae760d9861b6bd1f5070052482df98eb3047d517386db66084255dce8`，因缺同形 HTTP route 保留 partial；另将前端资源、密码改密触发、POST logout 三条 legacy exact 收窄为 reviewed partial。strict gap 3293→3274。

- 2026-09-29 API startup rollback follow-up：`TestHandleRollsBackAndStopsAfterOpenFailure` 对应的 production startup/migration rollback 测试 2/2 passed，receipt `sha256:5ff41a57feb4610daf6dfae3a59381aad5bf567d79b1f659a0da8068872fa066`；generic Handle callback/error-chain seam 保留 reviewed partial，strict gap 3274→3270。

## 2026-09-29 API Server / Transport Wire P2 query/URI strict batch

本批逐项复核四条已有 `function_exact`：`TestNormalizeCandlePeriodSupportsEveryDocumentedFamily`、`TestParseQueryTimeNormalizesToUTC`、`TestBindURIRejectsMalformedEscapeInRequestURI`、`TestBindURIAllowsEscapedLiteralPercent`。Rust 的 candle period alias 表、query time UTC 归一、畸形 percent 拒绝/合法 percent 解码，以及 execution write 真实路径参数解码均有断言；`jftrade-engine` 联合 nextest 5/5 passed，receipt `sha256:97726dfe188f6691db04e9b03f19dec666d366f4c48d8498c5a06526966a9f23`。四条 mapping 已升级为 `reviewed` 并绑定 receipt，既有 Parity anchor 保持有效。

本批没有把 `TestParseQueryTimeReturnsCallerFallback` 的非法值 fallback 或 Gin `BindURI` required 参数缺失误升为 exact；这两处继续以 reviewed partial 记录结构差异。strict audit 实际由 **3270 降至 3262**；API Server / Transport Wire 测试数量比例 10.2% 仍是补行为的风险信号，不作为完成率。

## 2026-09-29 API Server / Transport Wire P2 route strict batch

第二批复核六条已有 `function_exact`：可选 bool/时间/limit 解析、缺失 instrument URI 参数、active provider 不可用的 markets 错误、非法 refresh 早拒绝，以及缺少订阅租约时的三路 409。Rust 联合 nextest 8/8 passed，receipt `sha256:84dbc60dbeb07184ad3c0cbc750321351de2bd6b052e61ea613ea329e0ba553c`；六条 mapping 已升级为 `reviewed` 并绑定 receipt。

本轮 strict audit 实际由 **3262 降至 3250**。未把 API 测试数量 10.2% 当作完成率；仍缺同形 Gin binding/HTTP handler 的条目继续保持 partial 或单独记录。

## 2026-09-29 API Server / Transport Wire P2 auth/origin strict batch

第三批复核六条 middleware/origin exact：CORS trusted/unknown preflight、认证器缺失/不可信 Origin/无浏览器 Origin 会话、Referer/Origin precedence，以及 canonical origin 输入表。`jftrade-api` nextest 7/7 passed，receipt `sha256:a90d53c5245a42b16cb1397efcf98d666f3bc7aa3c02e56a9dda6e60b19d59e2`；六条 mapping 已升级为 `reviewed` 并绑定 receipt。strict audit 实际由 **3250 降至 3238**。wails/tauri 桌面 scheme 差异仍按已登记边界处理，未伪造成新功能等价。

## 2026-09-29 API Server / Transport Wire P2 subscription strict batch

第四批复核七条已有 `function_exact`：poll-only capability 优先级、12 条 malformed/incomplete subscription wire fixture、合法 target 过滤、instrument request contract、broker-neutral polling、consumer-only release 与 strategy lease preservation。`jftrade-engine` 联合 nextest 7/7 passed，receipt `sha256:66229dc2cec638a02dc06384b47949a7bca05c593b67daa57dbbce6cecb41710`；七条 mapping 已升级为 `reviewed` 并绑定 receipt。strict audit 实际由 **3238 降至 3224**，API 测试数量比例仍不作为完成率。

## 2026-09-29 API Server / Transport Wire P2 WebSocket live strict batch

WebSocket live 5 条候选行为测试全部通过，receipt `sha256:d5630dbcfc1a2e7e10bdaf5bf09f96e992fbff7c53addf84023fa4f0880510dd`。3 条 tick/origin rejection 与 provider-switch exact 完成 reviewed assertion；heartbeat 的 `liveClients` 字段缺失、以及 Go Host-based same-origin 与 Rust allowlist 判定差异被纠偏为 reviewed partial。strict audit 实际由 **3224 降至 3210**，没有把 WS 语料总量当作完成率。

## 2026-09-29 API Server / Transport Wire P2 behavior review

本轮按行为证据收口 7 条已有 `function_exact`：request observability 稳定上下文与错误摘要、非法 request id 替换、market-depth 方法/路径隔离、Swagger 核心路径、system status request id 传播、Web 登录连续失败限流与 `Retry-After`。Go 断言逐项复核，Rust owner 测试联合 nextest **9/9 passed**；receipt `sha256:ce452bd3c5eb1ac54fa17c8f9bd952a69c8e811dc9abc3b0bcd2e9503492a19d`。共享 observability/route/transport owner 的 reuse 关系已审核，7 条 mapping 的 `assertionCoverage.source` 已升为 `reviewed`。

本轮 strict audit 实际由 **3088 降至 3065**；当前全局仍有 3065 个历史 evidence gaps，未宣称完成。anchor reconcile 保持 `1894 / 1847 / 0 / 0 / 47`，assertionless exact 当前扫描为 **0**。API Server / Transport Wire 的 10.2% 测试数量比例只作为补行为风险信号，不作为完成率。

锚点收口补充：为 margin-ratio unknown-stock owner 补齐唯一缺失的 `rustEvidence.anchor`（`trade_session_tests.rs:1969`），strict gap 进一步 **3065→3064**；strict anchor error 已归零，整体仍未通过。

## 2026-09-29 API Server / Transport Wire P2 runtime behavior review

本轮收口 8 条已有 `function_exact`：watchlist 不可用时 provider switch 仍成功、startup restore 才允许 warming provider、Node 依赖配置路径缺失不回退、macOS Finder/attempts/Settings 缺失提示、strategy 显式 quantity 优先级、close quantityPct 按持仓计算、managed broker account normalization、combo quantity mode 映射。Go 断言逐项核对，Rust engine/marketdata/settings 联合 nextest **12/12 passed**（strategy owner 在多个 integration binary 中重复命中）；receipt `sha256:652cbc5eae4d4533af8b22ff2ea8f098cb6398bea05bf7f3d6e9871403d18d6b`。8 条 mapping 已升为 `reviewed` 并绑定真实 receipt。

strict gap 实际由 **3064 降至 3048**；anchor reconcile 仍为 `1894 / 1847 / 0 / 0 / 47`，assertionless exact 为 0。API 测试数量比例 10.2% 仅作为行为补齐风险信号，不作为完成率。

## 2026-09-29 API Server / Transport Wire P2 datamigration review

本轮收口 8 条 SQLite/datamigration exact：备份容量清理失败、不可兼容源库字节不变、受管快照清理与 quota、current snapshot retention、失败快照清理/SQLite 验证、rebuild selection 拒绝矩阵、manifest drift、schema catalog descriptor。Go 断言逐项核对，`jftrade-store-sqlite` 联合 nextest **8/8 passed**；receipt `sha256:4d739bd52477eb28f8bf1dbdcaba199570f595d524ea28b7b89a9793aa5397a4d`。8 条 mapping 已升为 `reviewed`。

strict gap 实际由 **3048 降至 3032**；anchor reconcile 保持 `1894 / 1847 / 0 / 0 / 47`。备份权限错误在 root 环境下的不可复现分支仍由 Go 测试自身 skip 规则保留，未扩张为额外行为结论。

## 2026-09-29 API Server / Transport Wire P2 datamigration safety review

第二批收口 8 条 rebuild-safety/backtest/broker route exact：marker backup 不可信字段拒绝、pending marker 缺 verified backup、已存在 marker 的 backup prerequisite、verified backup 调度幂等与 digest 复核、protected backup/digest error、backtest compaction 非文件路径、backtest list/detail 读投影、broker incomplete path 404。Rust store-sqlite/engine 联合 nextest **8/8 passed**；receipt `sha256:c91a4ac3fe696e1fbebd5897d1f96b81b9e42433c0ea3761390452bc25ae0119`。8 条 mapping 已升为 `reviewed`。

strict gap 实际由 **3032 降至 3016**；仍保留 root 权限下 Go skip 的 removal-permission 分支边界，不将环境不可复现误记为通过。

## 2026-09-29 API Server / Transport Wire P2 read/settings behavior review

本轮收口 10 条 exact：AKShare market-scoped lookback、preview ID secure-random failure、已结束 sidecar stop、settings appearance fixture、market catalog fixture、market profile metadata、research preset fail-closed、settings save 不改环境变量、onboarding defaults/save、依赖失败重新打开 OOBE。Go 断言逐项核对，engine/datamanagement/settings/helper/storage 联合 nextest **10/10 passed**；receipt `sha256:77a4805fce48b4565c50fe4d7475287a2977f08bae3486c1007be627fdbb65fc`。10 条 mapping 已升为 `reviewed`。

strict gap 实际由 **3016 降至 2996**；API 低测试比例仍只是行为缺口信号，不作为完成率。

## 2026-09-29 API Server / Transport Wire P2 shared-owner review

本轮复核 5 条已有 exact：rebuild marker snapshot retention、explicit K-line query bounds、strategy current-bar worker intent、market-depth PUT method rejection、legacy strategy source-format instantiate rejection。涉及 8 个真实 Rust owner 测试，联合 nextest **8/8 passed**；已确认共享 owner reuse 关系此前已有 reviewed allow，receipt `sha256:9c451f9dc06c5c9679f7299c2d089593a3526086b535c497b1210fa8dc3f9080`。5 条 mapping 升为 `reviewed`。

strict gap 实际由 **2996 降至 2986**；共享测试只在对应 Go 断言关系已审核的范围内计入。

## 2026-09-29 API runtime-dependency shared-owner review

复核 Node runtime dependency 两条 exact：配置脚本成功路径与 settings precedence，以及 outdated/invalid/non-zero command 三种失败状态。共享 Rust probe owner 逐案断言 status、detectedVersion、source、resolvedPath、版本提示与 stdout/stderr 诊断；`jftrade-engine` nextest **1/1 passed**，receipt `sha256:e12b481f0265f0680e5e6ca822d09c2ff5770ca08cd2924ddffe996a56c395a0`。两条 mapping 升为 `reviewed`，共享 reuse 关系同步审核。

strict gap 实际由 **2986 降至 2980**。

补充 reuse 审核：对已完成 assertion review、已有 passed receipt 的 optional query bool alias 与 candle adjustment normalization 两个共享 owner，复核其全部 Go referenceKeys 后将 reuse 关系升为 `reviewed/allowed`。本次不新增行为测试，仅清理证据关系缺口；strict gap **2980→2976**。

## 2026-09-29 broker runtime exact correction

复核 `TestBrokerRuntimeDescriptorIncludesReadFeatures` 与 `TestContractBrokerRuntime` 后发现现有 Rust owner 只直接调用 `ProductionBrokerPort::read`，虽覆盖 projection 字段，却没有证明 Go 的真实 HTTP route status、ok envelope 与 server assembly wiring。两条旧 `function_exact` 已准确收窄为 `partial`，并记录新增 HTTP owner 缺口；strict gap **2976→2969**。该下降是错误 exact 清理，不计为新增行为覆盖。

## 2026-09-29 API Server / Transport Wire P2 batch closure

本轮按真实行为批量收口已有 `function_exact`，不以 receipt 数量或 Rust 测试总数计完成率：

- Execution API：9 条 route exact（buying-power/combo、validation/error envelope、risk rejection、amount spoof、cancel、preview/events、push writeback、bare-symbol validation），7 个 owner tests 通过；receipt `sha256:908dd3ccbeb7cc812691cb3f9a4a3da31acc4da56689cff37c8fa276e6d8319e5`。
- Backtest API：6 条 sync/start/status/delete exact，6 个 owner tests 通过；receipt `sha256:8112ca0221bc3297e0d325c6778eda46484126247e7f90586e945c21860176f8`。
- Strategy/Pine API：3 条 sourceFormat/AST/warmup/validation exact，3 个 owner tests 通过；receipt `sha256:006d1777d3ddf4b8b1ca5dbe3eec152adbe879252316d462a2ea97d0cbc2a4a5`。
- Assistant workflow API：2 条 workflow/canvas route exact，3 个 owner tests 通过；receipt `sha256:b49cb003e98c5109ecf876bcadff2a930a1ad87f60ca77ce40a1a71d5c7c5bb2`。
- Watchlist API：7 条 route/read/query exact，4 个 owner tests 通过；receipt `sha256:3ed7a53929b236bd9372cd098c5e7672970846bdc17f27bd903bfee8a05466dc`。
- Provider Research API：7 条 capability/lifecycle exact，4 个 owner tests 通过；receipt `sha256:9e32c31f0abd2449195e807ff7ddc0342e14e4c40968c1959fbec197eaae2750`。

每批均完成 Go assertion review、Rust anchor/reuse 审核及 passed receipt 绑定；共享 owner 的 partial 引用保持原结论。strict gap 由 **2969 降至 2853**，anchor reconcile 仍为 `1894 / 1847 / 0 / 0 / 47`，assertionless exact 仍为 0。全局 strict 仍未通过，API Server / Transport Wire 的 10.2% 测试数量比例继续只作为行为缺口信号。

## 2026-09-29 API live volume/heartbeat batch

本轮收口 13 条已有 `function_exact`：OpenD listener 的显式 volumeDelta、累计量超过 2^53 的精确保持、trade/depth 同订阅推送，以及 ws-live fixture 的 heartbeat、通知、console refresh、provider switch 与 observedAt 行为。`jftrade-engine` nextest **4/4 passed**，receipt `sha256:ec10dda614a774af706ee1aecc442d08f4e8770edbb9843c005bdd96b5c4472e`；共享 owner reuse 与 assertion coverage 已审核。

strict gap 实际 **2853→2821**；所有 partial/boundary 结论保持不变。API 测试数量比例仍不是完成率。

## 2026-09-29 API execution validation batch

补齐 2 条 execution validation exact：US 限价价格 tick/session/market code 与显式 market+code 解析。`jftrade-engine`/`jftrade-integration-futu` nextest **5/5 passed**，receipt `sha256:53e2f159b30ba83c635eb62d2b54c3ee703e8a55ebe260392c70073b84a78df1`；price/session/parser 共享 owner reuse 已审核。

strict gap 实际 **2821→2805**；只计 reviewed assertions、有效 anchors、passed receipt 和 reuse 审核。

## 2026-09-29 API system status batch

收口 4 条 system/runtime exact：runtime resource count/items、system status stable fields、application adapter projection、injected summaries。`jftrade-engine` nextest **3/3 passed**，receipt `sha256:9a870aa32803da5277ec22731f5f85cf693f518d14e72ea28a484c1440d8354f`；typed status/runtime resource reuse 已审核，status-mapper partial 保持 partial。

strict gap 实际 **2805→2794**。

## 2026-09-29 receipt coverage repair

严格复核发现部分既有 reviewed rows 的 `testFilter` 未覆盖全部 `rustEvidence` owner。本轮补跑并替换 receipt：live/WebSocket 9 tests、watchlist 7 tests、execution+notification 10 tests、data-plane cache 5 tests、web disabled 2 tests、reasoning 3 tests、snapshot fallback 2 tests、settings defaults 4 tests。所有修复后的 rows 已验证 `rustEvidence ⊆ testFilter`，未扩大 exact 结论。

strict gap 实际 **2794→2775**；该下降来自证据关系修正和有效 receipt 覆盖，不以 receipt 数量作为完成率。

## 2026-09-29 ADK catalog owner batch

收口 3 条 ADK catalog/middleware exact：snapshot/tools 目录、agent template/delete 保护和 runtime 可用性直通。`jftrade-engine` nextest **2/2 passed**，receipt `sha256:30fe272d6caf798028ac0e5e33bc72c519a2cf460bce7cc5a7ad3d50960125d8`；两个 production assembly reuse owner 已审核。

strict gap **2775→2764**，partial 引用保持原结论。

## 2026-09-29 runtime resource ownership batch

收口 3 条 runtime resource exact：research DB 派生与覆盖、全量 resource owner/kind/path/critical 清单、strategy runtime DB 环境覆盖。`jftrade-engine` nextest **4/4 passed**，receipt `sha256:9675f546e7c02833ebe266297914dd43faf3242a6c139eba0e0d891d072a0535`；partial/boundary 的集中 layout 差异保持原结论。

strict gap **2764→2752**。

## 2026-09-29 runtime lifecycle batch

收口 6 条 lifecycle exact：startup rollback、ordered shutdown、direct drop、并发幂等 close、失败 stage stop、consumer/provider reverse order。engine/desktop nextest **4/4 passed**，receipt `sha256:8e29d9b65d8f849d562355f29993e55daff5b6ec517717bc3e7f1211ea1770b4`；多引用 reuse 已审核。

strict gap **2752→2732**。

## 2026-09-30 lifecycle evidence repair

补跑 `startup_failure_restores_previously_migrated_descriptor_files` 与 `readiness_failure_reclaims_every_started_process_without_starting_dependents`，并替换 4 条 lifecycle mapping 的 receipt/testFilter。两项真实 owner 均通过，strict gap **2732→2729**；未删除既有 Rust evidence。

## 2026-09-30 API/Assistant route batches

按 route owner 批量复核 Assistant API：approval/provider/pause-resume/optimization、chat stream failure/recovery/audit/validation、unavailable/store-failure/catalog、task-memory CRUD、provider default 与 skill failure。四个真实 nextest 批次分别通过 **7/7、6/6、6/6、3/3、6/6**；每条 reviewed exact 绑定对应 receipt，shared owner reuse 逐项审核。strict gap **2729→2657**。

## 2026-09-30 reuse audit

对 8 个已有 passed receipt 且所有引用行均为 reviewed exact 的 shared Rust owner 完成 reuse 审核；没有新增 exact 结论。strict gap **2657→2641**。当前 strict audit 仍未通过，API Server/Transport Wire 的行为覆盖比例仍为 **10.2%**，继续按真实 route owner 推进。

## 2026-09-30 transport and data-management batches

API/Transport route owner 批次通过 **13/13**，收口 backtest/research/prediction/broker/watchlist 路由 9 条 exact；strict gap **2641→2620**。随后补齐 8 条 reviewed mapping 的 receipt 元数据，gap **2620→2612**。SQLite/data-management 批次通过 **10/10**，收口 schema failure、backup retention、overview、cleanup 与 pending-rebuild rollback 10 条 exact，strict gap **2612→2592**。

## 2026-09-30 replay and assembly evidence

重跑旧 backtest receipt 的 10 个 owner（**10/10 passed**）并替换短 commit/不完整 testFilter 证据，strict gap **2592→2589**。随后重跑 Assistant P1 旧 receipt 的 9 个 owner（**9/9 passed**），并收口 5 个无共享 owner 的 assembly/MCP exact（**5/5 passed**），strict gap **2589→2579**。共享 owner 仍按全部引用行复核，未强行升级。
## 2026-09-30 API marketdata forwarding reviewed batch

针对 API Server/Transport Wire 的 marketdata forwarding 路径，逐项复核 `runtime_calendar_forwarding`、`runtime_company_forwarding`、`runtime_news_forwarding`、`runtime_rankings_industry_forwarding`、`runtime_screen_forwarding` 与 `runtime_index_constituents_forwarding` 的 15 条 `function_exact`。Rust owner 测试覆盖 provider wire projection、capability/lifecycle rejection、helper isolation、分页与 limit clamp；定向 engine nextest **29/29 passed**，receipt `sha256:51abd6e018b42e2f4f3a8ee2acd76ecb8e53cc80fbc64c4a176cddc17e9a6528`。15 条 mapping 从 `legacy-conclusion` 升为 `assertionCoverage.source=reviewed`，绑定当前 commit/testFilter；15 个小 fan-out shared owner reuse 完成 reviewed，high fan-out reuse 保留 backlog。

strict gap **2554→2503**。本轮下降来自真实行为测试、reviewed assertion、有效 receipt 与受限 reuse 审查；没有用 Rust 测试总数、receipt 数量或 verification passed 代替完成率。全局 strict audit 仍未通过，API Server/Transport Wire 数量比例仍为 10.5%。
## 2026-09-30 API servercoretest reviewed batch

针对 API Server/Transport Wire 的 `servercoretest` 路径，逐项复核 backtest sync、broker projection、system/strategy contract、settings normalization、onboarding 与 watchlist runtime 的 19 条 `function_exact`。Rust owner 测试覆盖 23 个真实行为测试；workspace nextest **23/23 passed**，receipt `sha256:d0f9cbb88920c3bed3fa60b9a47c6fea80a6e277a0b7ad06e844d33bff7f2463`。19 条 mapping 从 `legacy-conclusion` 升为 `assertionCoverage.source=reviewed`，绑定当前 commit/testFilter；17 个小 fan-out reuse 完成 reviewed，高 fan-out reuse 保留 backlog。

strict gap **2503→2441**。下降来自真实 route/contract 行为测试、reviewed assertion、有效 receipt 与受限 reuse 审查；没有将 Rust 测试总数、receipt 数量或 verification passed 当作完成率。全局 strict audit 仍未通过，API Server/Transport Wire 数量比例仍为 10.5%。
## 2026-09-30 API servercore behavior batch

继续收口 API Server/Transport Wire 的 `servercore` 路径，逐项复核 data-management、live volume、notification、capability catalog、strategy runtime/trading、OpenD health、security 与 strategy delete 的 17 条 `function_exact`。workspace nextest 实际运行 33 个匹配 owner（含多 target 重复实例）并 **33/33 passed**，receipt `sha256:022cdab8ccecceae349bab2bb53c3a545b4aee938098f712474abbc84edbc1f1`。17 条 mapping 升为 `assertionCoverage.source=reviewed`，绑定当前 commit/testFilter；19 个 fan-out ≤6 的 reuse relation 完成 reviewed，高 fan-out 保留 backlog。

strict gap **2441→2376**。本轮仍以真实行为断言、reviewed assertion、有效 receipt 和受限 reuse 审查计数，未以测试总数或 receipt 数量代替完成率。全局 strict audit 仍未通过，API Server/Transport Wire 数量比例仍为 10.5%。
## 2026-09-30 API marketdataapp behavior batch

收口剩余 API `marketdataapp` 的 19 条 `function_exact`：provider switch、sidecar 生命周期、health retry、search prefix、depth/kline query 与 subscription idempotency。Rust workspace nextest **22/22 passed**，receipt `sha256:af682a98cf08d8157b78645a0ea1ce63ce87e8196669be186e1d333de14313bc`；19 条 mapping 升为 reviewed assertion，绑定当前 commit/testFilter；19 个 fan-out ≤6 reuse relation 完成 reviewed，高 fan-out保留 backlog。

strict gap **2376→2308**。本轮仍只按真实行为测试、reviewed assertion、有效 receipt 与受限 reuse 审查计数；全局 strict audit 仍未通过，API Server/Transport Wire 数量比例仍为 10.5%。
## 2026-09-30 API remaining legacy exact closure

完成 API Server/Transport Wire 剩余 19 条 `function_exact` legacy 映射的断言复核，覆盖 application、backtest startup、desktop startup、Futu probe、lifecycle、runtime/status、combo transport、web auth 与 settings validation。workspace nextest **25/25 passed**，receipt `sha256:848b85e4751fa1f1c3addb93b9188624f8de95c12413857c5f367ea7ad4be9e7`；19 条 mapping 升为 reviewed assertion，绑定当前 commit/testFilter；21 个 fan-out ≤6 reuse relation 完成 reviewed。

API transport owner 的 `legacy-conclusion` exact 已清零。全局 strict gap **2308→2244**；分类为 reuse 566、receipt 880、assertion 798。全局 strict audit 仍未通过，API Server/Transport Wire 数量比例仍为 10.5%，后续转向其他领域与 API 高 fan-out reuse。
## 2026-09-30 Strategy/Pine parse reviewed batch

转入 P1 Strategy/Pine，完成 `pkg/strategy/pine/parse_test.go` 21 条 legacy `function_exact` 的断言复核。Rust owner 覆盖 parse IR、metadata、request.security、history reference、advanced indicators/orders、risk declarations 与 structured diagnostics；workspace nextest **21/21 passed**，receipt `sha256:fa86abe492eedff72da09451189fa1e17fd7f07e0bd0059b65ee2efbd77bd483`。21 条 mapping 升为 reviewed assertion，13 个 fan-out ≤6 reuse relation 完成 reviewed；framework-language 的 47-way reuse 保留 backlog。

strict gap **2244→2188**。本轮仍按真实行为测试、reviewed assertion、有效 receipt 与受限 reuse 审查计数；全局 strict audit 仍未通过，Strategy/Pine 的其余 legacy 与 partial 继续按 P1 顺序推进。
## 2026-09-30 Strategy/Pine live execution batch

继续收口 P1 Strategy/Pine live execution，复核 7 条 exact：stop price/reduce-only、risk reason table、submitted-order instance scope、entry/close quantity sizing 与 missing quantity rejection。workspace nextest 实际 19 个 target 实例 **19/19 passed**，receipt `sha256:dc4d66ed418c004d700714a02b97e5da22571edb94f878e88eb175d7d1f265c6`；7 条 mapping 升为 reviewed assertion，低 fan-out reuse 已核对。

strict gap **2188→2172**；当前 Strategy/Pine legacy exact 还剩 42 条。全局 strict audit 仍未通过，未把 target 实例数或 receipt 数量当作完成率。
## 2026-09-30 Strategy/Pine risk and order-boundary batch

复核 Strategy/Pine 风控与 order-boundary 的 8 条 exact：risk-off/monitor/normalization、qualified position matching、order metadata rejection、trailing offset 与 truncated expressions。workspace nextest **10/10 passed**，receipt `sha256:a8d9603e2f3f9cd70b93eac40093f44326cab698c15a00a7f628552b41606de6`；8 条 mapping 升为 reviewed assertion，5 个低 fan-out reuse relation 完成 reviewed。

strict gap **2172→2152**；Strategy/Pine legacy exact 还剩 34 条。全局 strict audit 仍未通过。

## 2026-09-30 Strategy/Pine legacy exact closure

- 29 条已有真实 Rust 行为 owner 完成逐项 Go assertion 复核，覆盖 runtime reconciliation、definition version、warmup/planner、Pine parser/tuple/security、Pine worker pool 与 request validation。
- workspace nextest **35/35 passed**；receipt `strategy-pine-legacy-exact-reviewed-2026-09-30.json`，digest `sha256:22918fba9517443a7ed1bcac3a5ad269d4e35c9ef1e6b476b3f5487974008a6e`。
- 29 条 mapping 的 `assertionCoverage.source` 已从 `legacy-conclusion` 升为 `reviewed`；fan-out ≤6 的 19 个 reuse relation 已审核，高 fan-out 继续 backlog。
- strict gap **2139→2067**；本轮按行为断言、reviewed assertion、有效 receipt 与受限 reuse 计数。

## 2026-09-30 API transport envelope and reuse review

- 新增 `jftrade-api` envelope owner 测试，逐项锁定 Go `WriteOK`、`WriteError` 与 `WriteNotFound` 的状态、ok、data、timestamp、code/message 形状；mapping `TestResponseEnvelopeWriters` 由 partial 升为 reviewed `function_exact`。
- envelope/transport 定向 nextest **2/2 passed**；receipt `api-transport-envelope-writers-reviewed-2026-09-29.json`，digest `sha256:552d9d0eb2f091ee9f829c65a37d7637361ba4ac1fce173c8e0b635f99c480cf`。
- 复核 API auth/origin/CSRF/CORS/SSE 的 9 个低 fan-out shared owner，定向 nextest **9/9 passed**；receipt `api-transport-reuse-reviewed-2026-09-29.json`，digest `sha256:720c76ce1c5c9c27c9c5c5cc879ebc9845cd4b0e946440a467d5c71c4231284e`。
- 严格 gap **2067→2053**；API Server/Transport Wire 行为比例为 10.6%，仍不作为完成率。

## 2026-09-30 API logout HTTP projection batch

- 针对 `TestWebLogoutClearsSessionCookie`，补充真实 product TCP HTTP 断言：logout 返回 200、`authenticated=false`，并透传 `Set-Cookie: jftrade_web_session=; Max-Age=0`；manager owner 同时断言旧 token 失效及 HttpOnly/SameSite/Path 属性。
- 真实 nextest **3/3 passed**；receipt `api-transport-logout-http-reviewed-2026-09-29.json`，digest `sha256:3a34c8940c101b5df7007725f978618f735b8abbecab510454fdb5979c9080d4`。
- mapping 从 partial 升为 reviewed `function_exact`；共享 auth-session route fixture 的 3-way reuse 已逐项复核。
- strict gap **2053→2052**。`TestAuthProtectsLogout` 仍保留 partial，因为 Go middleware 的测试 stub 要求 204，而生产 logout endpoint 合同为 200，未强行抹平该边界。

## 2026-09-29 批量 legacy exact 与 API receipt 收口

- `internal/exchangecalendar` 45 条 legacy `function_exact` 完成 assertion reviewed；47 个日历 parser/manager/source 行为测试真实 **47/47 passed**，receipt `exchangecalendar-legacy-exact-reviewed-2026-09-29.json`，digest `sha256:bdb85e7b823cab1499e69e4faaef6074ef223c99ad0c4e12ac370730dc16dafb`。
- `internal/store` 28 条 legacy exact 的 30 个 owner 测试真实 **30/30 passed**，receipt digest `sha256:269e5473ac3ace141090615278dd73cdc32a3d6355b305904cf6a14a5b7e50ae`；`internal/settings` 16 条、19 个 owner 测试真实 **19/19 passed**，receipt digest `sha256:2bc75ca653eaf944b8f88dc5496f36ea8658f7cebc06260d75f4f62a6d150690`。
- API Server/Transport Wire 重新运行 16 个真实 engine owner 测试 **16/16 passed**，修复 11 条旧短 SHA receipt 的严格可验证字段，receipt `api-transport-receipt-refresh-2026-09-29.json`，digest `sha256:32fe1451be0c9565c8fab2733dbb134212bfb3221d39e338f8e985b24c49e506`。
- API 低 fan-out（≤5）共享 owner 的 27 个 reuse relation 已逐项 reviewed；fan-out 6、7、11、20 的共享 owner 继续 backlog。
- strict gap **908→818→762→730→689→678**；anchor reconcile 仍为 `1895/1848/0/0/47`。数量比例与 receipt 数量不作为完成率，严格审计仍未通过。

## 2026-09-29 继续批量收口与受控 fan-out review

- `internal/app/apiserver` 8 条旧 receipt 重新验证，10/10 owner tests passed，digest `sha256:d80813b72ec8bac46581fee167f0557ac764593319771b70665b8a286f22be8f`；`internal/assistant` 30 条旧 receipt 重新验证，30/30 passed，digest `sha256:b051bebb952afa819447ac98af1767b5e1b9cf3de737c2814a915aca2d9787dc`。
- `internal/system` 9 条 legacy exact 完成 reviewed，9/9 passed，digest `sha256:eb4c5569345b8c63ebd052664548e09aade15e837758df37539ecc6fc5169afd`；`internal/backtest` 4 条完成 reviewed，4/4 passed，digest `sha256:10cd6f60b642e35ffce5f6d3f78a7aa7c3c8bb1e0c03b6a01f50480076de6f00`；`pkg/researchscreen` 7 条完成 reviewed，7/7 passed，digest `sha256:fc21b9c2fe171626a04b798edb6005ddff57be18304c0484b0f6a9db560d4281`。
- 对 3 个已人工逐引用核对的高 fan-out relation 记录 reviewed；instrument search、settings product、watchlist 与 migration 等更高 fan-out 继续 backlog。
- strict gap **908→818→762→730→689→678→670→640→614→600→595**；本链只反映真实行为、reviewed assertion、有效 receipt 与受控 relation，严格审计仍未通过。

## 2026-09-29 小模块行为批次与 API relation 收口

- asset/security/retry/integration 小模块分别完成真实测试 **4/4、4/4、3/3、6/6**，对应 mapping 批量升为 reviewed；receipts：`sha256:7e307064cd641c432bddba1bd8b3426522f8821d6e5cfbac759af8c1ccbbcf88`、`sha256:37f123fb6d929139732d328c486be65416b8e015eee555726339d85674431589`、`sha256:c90c3bf12ebf14975a85f5f370086ae39541cc11bc83baffd4188228b4226407`、`sha256:def9669e86f1f6f3c562862b6f03115be842b0ade71b73e9c93213f9f8c0bb6f`。
- 合并 jftsettings/desktop/watchlist/research 的 18 个 owner 测试 **18/18 passed**，receipt digest `sha256:0e58f1a26859c7ae40dccd4e1943c9d03a3748c48b9df4a2ad770f6d50af68fd`；此前小 legacy 批次 8 mappings/9 tests **9/9 passed**，digest `sha256:df8913aafcc1d319f151c644d90677dba118164de52a63cb441b8c20b83768df`。
- instrument-search 两个高 fan-out owner 经逐引用比较后 reviewed；更高 fan-out settings product、watchlist、maintenance 仍未放行。
- strict gap **908→818→762→730→689→678→670→640→614→600→595→587→581→575→559→523→511→509**；严格审计仍未通过。

## 2026-09-29 Assertion review batch

- 对 13 条已有真实 `function_exact` owner 的 legacy/manual assertion coverage 完成统一复核：ADK reasoning effort matrix、calendar JSON、Futu/Pine proto guards、repository-root、desktop update/asset guards 共 13 条。
- 定向 nextest **13/13 passed**（2712 skipped）；receipt `assertion-review-2026-09-29.json`，raw output digest `sha256:0c0cd9360e979c1b0ca737052037eee818f431ae89b9715f8357f4f058c50ea5`。
- 13 条 mapping 的 `assertionCoverage.source` 已升为 `reviewed`；reasoning effort 的缺省、low/medium/high/xhigh/max 五档断言逐项列入 covered assertions，未覆盖项清零。
- strict gap **468→455**；剩余 455 项全部属于未完成逐引用审核的 shared-owner reuse relation。测试总数、receipt 数量和 verification passed 不作为完成率。

## 2026-09-29 API/settings high-fan-out owner review

- 逐项复核 `product_server_persists_ui_settings_and_reports_actual_port` 的全部 20 个 `referenceKeys`：4 条 `function_exact` 保留各自 service/port 组合证据，16 条 partial 的 GET/PUT、失败 envelope、登录、callback、apiBind 缺口继续保留。
- 真实 engine owner nextest **1/1 passed**；receipt `api-settings-product-owner-reviewed-2026-09-29.json`，raw output digest `sha256:9d9a7491868053886619fb1edef4e9617c45f4a022132a86ef4078812dccf36f`。
- shared reuse relation 标记 `allowed=true, reviewStatus=reviewed`，不改变任何 partial 结论；strict gap **455→451**，剩余仍全部是未审核 shared-owner reuse。

## 2026-09-29 API/datamigration maintenance owner batch

- 逐项复核 8 个 maintenance relation 的全部 referenceKeys，覆盖 backup filename/retention、overview WAL/SHM、summary filter、cleanup schema failure、rebuild marker、failed rebuild cleanup、tampered pending rebuild 与 backtest preview；9 条 exact 引用释放，其他 partial/boundary 保持原结论。
- SQLite/engine 定向 nextest **9/9 passed**（2191 skipped）；receipt `api-maintenance-owners-reviewed-2026-09-29.json`，raw output digest `sha256:2ec3da6ae462d9ddd856d2ad3f07e6b90085e49ba1722e71eb38d2cf33926b11`。
- strict gap **451→442**；剩余全部是尚未逐引用审核的 shared-owner reuse，未把维护测试数量或 receipt 数量当作完成率。

## 2026-09-29 API/watchlist read-owner review

- 逐项复核 watchlist read owner 的 11 个 referenceKeys；唯一 `function_exact` route mapping 结合 SQLite read-page 与 engine write fixture，其他 10 条 source-health、multi-source expiry、duplicate-name、query-plan、cursor/detail、failure 与 snapshot rows 保持 partial/boundary。
- store-sqlite/engine nextest **2/2 passed**（2198 skipped）；receipt `api-watchlist-read-owner-reviewed-2026-09-29.json`，raw output digest `sha256:ce73e5c1ec405a4d571db8ba59fd37229c8649df3feb4f9146c6fb02ba088e70`。
- strict gap **442→441**；行为结论未因 shared owner review 扩大。

## 2026-09-29 API marketdata forwarding/cache owner batch

- 逐项复核 9 个 marketdata/Futu shared owner relation：probe connectivity、provider activation rollback、cache freshness、subscription demand、snapshot cache hit/miss、tick fallback/cache miss 与 US session labels；对应 referenceKeys 的 partial/provider/conversion 差异继续保留。
- Futu/marketdata/engine 定向 nextest **9/9 passed**（2607 skipped）；receipt `api-marketdata-owners-reviewed-2026-09-29.json`，raw output digest `sha256:e18e93372b58803d03bb62430a16c4986d1be8fa7fbd11769728c7656a8312a0`。
- strict gap **441→429**；本轮只释放通过逐引用比较的 exact reuse，未把 marketdata 测试数量当作 API 行为完成率。

## 2026-09-29 API forwarding-wire owner batch

- 逐项复核 18 个 API forwarding shared owner relation，覆盖 calendar/macro、company、news、rankings/industry、index constituents、screen、market depth、unknown-session 与 sidecar health；34 条 exact 引用释放，其他 provider/capability/conversion/collector 边界保持原结论。
- engine/marketdata-helper 定向 nextest **18/18 passed**（2020 skipped）；receipt `api-forwarding-wire-owners-reviewed-2026-09-29.json`，raw output digest `sha256:eaf29e07e48c810bcb4b966db1af2b5eb6dd402f9d52a0693e88e5b8c5a87117`。
- strict gap **429→395**；API Server/Transport 的行为证据继续按逐引用 owner 审核推进，不以 10.6% 数量比或测试总数计完成率。

## 2026-09-29 API/runtime tail owner batch

- 逐项复核 6 个 API/runtime shared owner relation：sidecar stop、strategy cancel、backtest sync validation、settings corpus、watchlist group restart、market-data status；6 条 exact 引用释放，partial rows 的 cancellation/facade/schema/in-memory/provider-state 差异保持。
- helper/engine/settings-file/store-sqlite nextest **8/8 passed**（2237 skipped）；receipt `api-runtime-tail-owners-reviewed-2026-09-29.json`，raw output digest `sha256:fb934a7746ba8b0769a53ab160c8227f33566121feef6a502767f4c65f60a3f9`。
- strict gap **395→387**；随后 candle adjustment 4-way relation 也完成 reviewed，strict gap **387→377**。

## 2026-09-30 Assistant MCP policy owner review

- 逐项复核 MCP loopback/Host protection 两个 shared owner relation（4 条 exact 引用）；engine nextest **2/2 passed**（2010 skipped），receipt `assistant-mcp-policy-reviewed-2026-09-30.json`，raw output digest `sha256:ded615b93f06adf18d4b6ec4233b16e0670f58b40f6f74d2103ac6fa0163464e`。
- strict gap **377→373**；只释放逐引用确认的 route/handler exact，其他 MCP lifecycle partial 继续 backlog。

## 2026-09-30 Assistant shared-owner batch

- 逐项复核 9 个 Assistant shared owner relation：claims fencing/lease、tool-failure degraded projection、run-timeout fallback、approval wakeup、workflow thresholds 与 continuation supervisor；18 条 exact 引用释放，partial 的 stub/failure/long-running 差异继续保留。
- assistant/engine/store-sqlite nextest **9/9 passed**（2234 skipped）；receipt `assistant-shared-owner-batch-2026-09-30.json`，raw output digest `sha256:90f36484e3ef5ed3d742f8eb111b59fb68fd9e9cb54a0f6b5948174ab810825e`。
- strict gap **373→355**；仍未完成的主要是 MCP/application adapter/tool catalog 高 fan-out reuse。

## 2026-09-30 Futu/marketdata shared-owner batch

- 逐项复核 4 个 shared owner relation：research normalization、basic quote empty-S2C、batch snapshot validation、embedded research allow-list；12 条 exact 引用释放，catalog partial 仍保持。
- engine/integration-futu nextest **4/4 passed**（2551 skipped）；receipt `futu-marketdata-shared-owners-reviewed-2026-09-30.json`，raw output digest `sha256:d6d7efaaf8bab7719f70c1000de9044374b303b4b8d543a6526a1f8d7be557e7`。
- strict gap **355→343**。

## 2026-09-30 Futu/engine exact-pair batch

- 21 个 2-way/2-exact shared owner relation 完成逐引用复核，覆盖 OpenD version/session/trade/quote/subscription/runtime、engine market-data、trade、prediction 与 execution guard；释放 42 条 exact。
- Futu/engine 定向 nextest **21/21 passed**（2534 skipped）；receipt `futu-engine-exact-pairs-reviewed-2026-09-30.json`，raw output digest `sha256:db5bfa62fdacea96e71ca5c648e3a554eca3ad6ca4f1aff7142a7cbe1360951a`。
- strict gap **343→301**；只按逐引用行为 owner 下降，未使用测试数量或 receipt 数量作为完成率。

## 2026-09-30 Futu triple-owner batch

- 7 个 3-way shared owner relation 完成逐引用复核，覆盖 quote row dedupe/normalization、security market validation、tick fallback、interval mapping、recoverable errors、HK order-book subscription 与 closed-ready session replacement；14 条 exact 释放。
- integration-futu nextest **7/7 passed**（536 skipped）；receipt `futu-triple-owner-reviewed-2026-09-30.json`，raw output digest `sha256:a64ff016f2bbb625e64b480deb054496e909e0e2e4837fa6f9919696b07838e9`。
- strict gap **301→287**。
## 2026-09-30 Assistant MCP server owner batch

- 逐引用复核 MCP server 的 14 个 shared-owner relation，覆盖 account/portfolio projection、strategy/backtest/model 工具、listener lifecycle、catalog registration、dependency fail-closed、unsafe host、activity paging 与 workflow wait cancellation；保留未覆盖的 HTTP fetch、schema 与 product dispatch relation。
- engine nextest **14/14 passed**；receipt `assistant-mcp-server-owner-reviewed-2026-09-30.json`，raw output digest `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`。
- 14 个 relation 标记为 reviewed，释放 14 条 exact；strict gap **287→272**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Assistant application/tool-catalog owner batch

- 逐引用复核 16 个 fan-out ≤4 的 application/tool-catalog shared owner relation，覆盖 workflow fail-closed/update、execution read、trade scope、market candle forwarding、backtest filters、portfolio layers、research readiness、strategy optimization、tool catalog projection、capability access 与 malformed instrument rejection；10-way strategy binding relation 保留待单独审查。
- engine nextest **17/17 passed**（1995 skipped）；receipt `assistant-application-owner-reviewed-2026-09-30.json`，raw output digest `sha256:9d64884dbd4dfa97317b1272f9a616f8980daf39aea3b663c90748e49c5b3c69`。
- 16 个 relation 标记为 reviewed，释放 16 条 exact；strict gap **272→256**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 ADK runtime pair owner batch

- 逐引用复核 20 个 fan-out=2 的 ADK runtime relation，覆盖 input conflict、approval projection、lease fencing、run/session projection、terminal audit 与 context compaction；高 fan-out runtime/store relation 保留 backlog。
- engine nextest **20/20 passed**；receipt `assistant-adk-runtime-pairs-reviewed-2026-09-30.json`，raw output digest `sha256:4a81b9bc4b2324ea45c9ed00b94198fe11e0766718c7e061596c2136ce0af21a`。
- 20 个 relation 标记为 reviewed，释放 20 条 exact；strict gap **256→236**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 API route pair owner batch

- 逐引用复核 20 个 fan-out=2 的 API/Transport route owner relation，覆盖 execution validation/preview、system health/status、calendar/news/company projection、broker query、market adjustment、WebSocket subscription、Futu market rules/order-book 与 notification mapping。
- workspace nextest **20/20 passed**（3521 skipped）；receipt `api-route-pairs-reviewed-2026-09-30.json`，raw output digest `sha256:5e63078e658a06423b3c1b0453baeb53f72de24a2e26a4da3faea60c003c084c`。
- 20 个 relation 标记为 reviewed，释放 20 条 exact；strict gap **236→216**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 API route tail owner batch

- 收口剩余 2 个 fan-out=2 route relation：Futu notification/quote-right labels 与 settings backtest-provider atomic preparation。
- workspace nextest **2/2 passed**；receipt `api-route-pairs-tail-reviewed-2026-09-30.json`，raw output digest `sha256:a65d2d16e2e20bb0f2a9e74fdb690e8733f1cf4623ddc3da7c1f92c38f5c00d6`。
- 2 个 relation 标记为 reviewed，释放 2 条 exact；strict gap **216→214**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 API route triple-owner batch

- 逐引用复核 14 个 fan-out=3 的 API/Transport shared owner relation，覆盖 execution combo/snapshot、marketdata batch/quote/subscription、trade analytics/funds、Futu descriptor/retry/notification、market profile、maintenance 与 watchlist cache。
- workspace nextest **14/14 passed**（3527 skipped）；receipt `api-route-triples-reviewed-2026-09-30.json`，raw output digest `sha256:5548a060148522457f38e479d3d4d3ef8efa868c9137f035ccf75a79d75585ad`。
- 14 个 relation 标记为 reviewed，释放 14 条 exact；strict gap **214→200**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 ADK runtime triple-owner batch

- 逐引用复核 13 个 fan-out=3 的 ADK runtime relation，覆盖 pending input recovery、tool-only turn、expiry/fencing、memory/provider gate、terminal audit、handoff 与 session pagination。
- engine nextest **13/13 passed**（1999 skipped）；receipt `assistant-adk-runtime-triples-reviewed-2026-09-30.json`，raw output digest `sha256:40a8bbf45d8b4d060d82e0b10b6ba4240623818c154fc9145587541e96f1a333`。
- 13 个 relation 标记为 reviewed，释放 13 条 exact；strict gap **200→187**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Assistant claims/runtime owner batch

- 逐引用复核 6 个 `jftrade-assistant` fan-out=2–4 relation，覆盖 workflow diamond ordering、completed tool replay、stale claim fencing、sibling approval continuation、approval idempotency 与 lease heartbeat。
- workspace nextest **6/6 passed**（3535 skipped）；receipt `assistant-claims-owners-reviewed-2026-09-30.json`，raw output digest `sha256:d100bdc4c3bf6e5e5c77e48d8db495259f294334cae5137a63752f23cd182cf3`。
- 6 个 relation 标记为 reviewed，释放 6 条 exact；strict gap **187→181**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 ADK store owner batch

- 逐引用复核 7 个 SQLite ADK store fan-out≤4 relation，覆盖 missing-session atomicity、event commit/rollback、provider default repair、artifact/session restart durability、writer fencing 与 approval idempotency。
- workspace nextest **7/7 passed**（3534 skipped）；receipt `adk-store-owners-reviewed-2026-09-30.json`，raw output digest `sha256:98fd30e706a5cae7d009a6e00852927d73aaee0352a081f637926548e9e22885`。
- 7 个 relation 标记为 reviewed，释放 7 条 exact；strict gap **181→174**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Futu/OpenD pair owner batch

- 逐引用复核 20 个 Futu/OpenD fan-out=2 relation，覆盖 basic quote/tick batching、health/version/status、instrument search/static info、kline interval/window、managed session keep-alive/close 与 order-book empty/detail projection。
- `jftrade-integration-futu` nextest **20/20 passed**（523 skipped）；receipt `futu-opend-pairs-reviewed-2026-09-30.json`，raw output digest `sha256:687a59f3da757e1fbf5c57147f130f111fc42f726855149bd81cad3f625482a1`。
- 20 个 relation 标记为 reviewed，释放 20 条 exact；strict gap **174→154**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Futu/OpenD pair owner batch 2

- 逐引用复核第二组 20 个 Futu/OpenD fan-out=2 relation，覆盖 quote-rights/cache generation、security snapshot、subscription lifecycle/reconcile、trade proto/session、watchlist gate、client recovery、frozen proto 与 prediction push。
- `jftrade-integration-futu` nextest **20/20 passed**（523 skipped）；receipt `futu-opend-pairs-2-reviewed-2026-09-30.json`，raw output digest `sha256:2c24a6e94b730a3dad00c289924da912d36acdfec2e161f307f1f47fc0f56a70`。
- 20 个 relation 标记为 reviewed，释放 20 条 exact；strict gap **154→134**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Futu/OpenD pair tail owner

- 收口剩余 1 个 Futu/OpenD fan-out=2 relation：user security group type encoding/projection。
- `jftrade-integration-futu` nextest **1/1 passed**（542 skipped）；receipt `futu-opend-pair-tail-reviewed-2026-09-30.json`，raw output digest `sha256:1f56d2b79405bbad59b530e99ddcb46d9ac8e5589a56c759a6b1636dd90c2290`。
- 1 个 relation 标记为 reviewed，释放 1 条 exact；strict gap **134→133**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Cross-domain pair owner batch

- 逐引用复核跨领域剩余 fan-out=2 的 20 个 relation，覆盖 calendar snapshot、ADK task/provider/notices/tool declarations、execution/reconciliation、marketdata catalog/lease/snapshot、research projections 与 broker capabilities。
- workspace nextest **20/20 passed**（3521 skipped）；receipt `cross-domain-pairs-reviewed-2026-09-30.json`，raw output digest `sha256:11200ac86d98093d5f2a7439a503bd0412ad759a418ff2bfaa4dbbbed90f49d8`。
- 20 个 relation 标记为 reviewed，释放 20 条 exact；strict gap **133→113**。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Futu/OpenD triple owner batch 2

- 逐项复核 16 个 `referenceCount=3` 的 Futu/OpenD、marketdata 与 SQLite shared owner relation，覆盖 search 错误与 malformed response、K-line period 双协议映射、snapshot rate-limit/empty result、subscription reconcile 并发幂等、funds/max-quantity projection、trade protocol IDs、watchlist fresh/cache/duplicate semantics、closed-session propagation、prediction normalization、tick-candle window/volume 与 execution sequence reopen。
- workspace nextest **16/16 passed**（3525 skipped）；receipt `futu-opend-triples-2-reviewed-2026-09-30.json`，raw output digest `sha256:7564322fb6753e5a1262f184c00d6d07694179d6a6f606f295dd7c648e7f2933`。
- 16 个 relation 标记为 reviewed，释放 16 条 exact；strict gap **72→56**（以本轮独立 strict audit 输出为准）。未以测试总数、receipt 数量或 verification passed 作为完成率。

## 2026-09-30 Mixed high-fanout owner batch 1

- 20 个未审核 shared owner relation 完成逐引用复核，覆盖 broker market rules、funds/portfolio projection、ADK runtime/store、Pine/research normalization、calendar、Futu snapshot/prediction 与 SQLite lifecycle；其中 research screen owner 在两个真实 binary 中命中，共 **21/21 passed**。
- receipt `mixed-highfanout-owners-reviewed-2026-09-30.json`，raw output digest `sha256:07427002da46199e0b8a3eb5cb3526c1d79234605b017330e2aca6940f460c75`；释放 25 条 exact，strict gap **56→31**。

## 2026-09-30 Mixed high-fanout owner batch 2

- 20 个 ADK/calendar/broker/marketdata-helper/execution/Futu owner relation 完成真实行为复核，nextest **20/20 passed**。
- receipt `mixed-highfanout-owners-2-reviewed-2026-09-30.json`，raw output digest `sha256:e96c23604f55078abb65f9f54d5892a17f7671bca9a48afd63666b112f8547ac`；释放 20 条 exact，strict gap **31→11**。

## 2026-09-30 Final Futu/research owner batch

- 剩余 11 个 notification、quote-rights、order-book、search、history/session、security snapshot 与 research projection owner 全部实测，nextest **11/11 passed**。
- receipt `final-futu-research-owners-reviewed-2026-09-30.json`，raw output digest `sha256:400c22b7ae211ed0eb4cd8c490347c8dcd808d4bb8b19b7b4e39710513d17061`；strict gap **11→0**，所有 `function_exact` 行均具备 reviewed assertion、anchor、reuse 与 passed receipt。

## 2026-09-30 API transport SPA boundary behavior

- 新增真实 axum transport 表驱动测试，覆盖 root/HTML/尾斜杠 SPA 回退、`Accept: application/json` 拒绝、扩展名与 assets 缺失拒绝；先以失败测试确认旧实现错误，再修复 router 按 Accept 选择 SPA fallback。
- `TestShouldServeFrontendIndexRequestBoundaries` 从 partial 升为 `function_exact`，`jftrade-api` nextest **1/1 passed**；receipt `api-transport-spa-boundary-reviewed-2026-09-30.json`，digest `sha256:f3c8b8358b7572660ce891f978438721ec363b73ea1b649e9746a896359b83b8`。
- API Server/Transport 数量比仅由 **10.6%→10.7%**，仍不作为完成率；strict audit 继续为全量通过。

## 2026-09-29 API provider-test wire batch

- `TestProviderAndAgentValidationContracts` 完成公开 provider probe 路由收口：真实 HTTP 测试覆盖无 body 默认 quick（`reasoning.effort`）、`mode=full`、`mode=slow` → 400，以及未知 provider → 502 `ADK_PROVIDER_TEST_FAILED` / `provider not found`。
- 先红后修 `product_production_ports_adk_mutation_runtime.rs::test_provider` 的未知 provider 状态码；端口与生产路由 nextest **4/4 passed**，receipt `api-provider-test-wire-reviewed-2026-09-29.json`，raw output digest `sha256:b4ce4838b0f6ebe37e9474caabda06913c14ab226e3fbcc7ba332fd324ee6800`。
- 该映射由 partial 升为 `function_exact`，strict function_exact 证据行 **1492→1493**；strict audit 通过。API Server/Transport 数量比仍为 10.7%，不作为完成率。

## 2026-09-30 API assistant chat/SSE wire batch

- `TestChatAndSSEContracts` 补成真实 production composition 行为测试：先断言 `POST /api/v1/adk/chat` 的 `200`、JSON `ok=true` envelope，再用独立 request id 驱动成功 SSE，断言 `Content-Type=text/event-stream`、`X-ADK-Stream-Idle-Timeout-Ms=420000`、session→run→terminal 顺序与持久化 session id。
- 测试使用两连接 loopback Responses provider；关闭 provider 的 JSON failure projection 与成功 SSE 头分开验证，避免把失败路径计作成功流。定向 engine nextest **1/1 passed**；receipt `api-assistant-chat-sse-contract-reviewed-2026-09-30.json`，digest `sha256:ef17475a3ca52c8d11254d0a921de827174661872903d9d93a6ea34a09c353f8`。
- 该 partial 升为 `function_exact`，strict function_exact 证据 **1493→1494**；strict audit 通过。API Server/Transport 数量比仍仅作风险信号，不作为完成率。

## 2026-09-30 API live WebSocket shutdown lifecycle batch

- `TestHandlerConnectionLimitAndCloseLifecycle` 补齐真实 WebSocket 组合行为：连接上限超出返回 `503 LIVE_WS_LIMIT_REACHED`，释放 permit 后可再次握手；活动连接订阅 depth 后触发 product shutdown，读取服务端 `1001/server shutting down` close，并确认 hub connected 与 active depth instruments 均清零。
- 先红后修测试夹具：扩展 masked WebSocket frame 长度支持，并排空 shutdown 前 queued text frame 后再断言 close；生产实现未被绕过。engine nextest **2/2 passed**；receipt `api-live-handler-close-lifecycle-reviewed-2026-09-30.json`，digest `sha256:14c38cbc91d70bb6b9a627c8fddd509294d9899c7a119ba0bf6c20079930b181`。
- 该 partial 升为 `function_exact`，strict function_exact 证据 **1494→1495**，partial **2322→2321**；strict audit 通过。API Server/Transport 10.7% 数量比仍不作为完成率。
## 2026-09-30 API bindings required-path wire batch

- `TestBindURIHandlesBindingAndFallbackEscapeValidation` 补齐真实 `build_router` wire 断言：已认证请求缺少 `/api/v1/watchlist/groups/{groupId}` 参数时返回 `404`、`application/json; charset=utf-8`、稳定 `NOT_FOUND` envelope，且 `ApiPort` 不被调用；URI `%25`/`%20` 合法解码与 `%ZZ`/`%2` 拒绝由 query owner 同批断言。
- 先红后修验证了认证中间件先于路由匹配（未认证请求为 401），随后在已认证夹具下 **3/3 passed**；receipt `api-bindings-wire-exact-reviewed-2026-09-30.json`，raw output digest `sha256:df775fcfd9a151371616e8dfe64aef9996d0f1c84088b7d4925d8cd6eba2ec0f`。
- 该 partial 升为 `function_exact`；strict function_exact **1495→1496**，API Server/Transport 数量比 **10.7%→10.8%** 仅作风险信号，不作为完成率；strict audit 通过。
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

## 当前批次

- `cmd/jftrade-desktop/main_test.go:131:TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset` 已完成逐项收口：Rust transport owner 对 `/assets/`、`/docs/` 和带扩展名缺失路径逐一断言 404 与空响应体，定向 nextest 1/1 通过，映射由 `partial` 升为 `function_exact`；receipt `sha256:6bfd058c06d76673f62c47c740f73745ef09666cf0c60e1db1a472e08f13ff89`。

## 下一轮目标（2026-10-02）

1. **Assistant 断线传输 owner**：复核 `internal/api/assistant/chat_transport_disconnect_test.go:110`，确认 Rust SSE 物化模型是否足以覆盖 retry/event 写失败的单写即退；若无法构造同形 socket loop，补齐 reviewed boundary 与回归条件，不把通用 `SseWriter` 错当成 socket loop 等价证据。
2. **Backtest provider error 所有权**：复核 `internal/app/apiserver/backtestapp/historical_source_test.go:226`，保留 Rust durable task 的 provider message 证据，同时登记 Go typed error identity 与 Rust worker error 字符串化的边界；只有生产 API 暴露稳定错误分类时才新增映射或测试。
3. **HTTP 时间解析契约**：复核 `internal/api/httpserver/bindings_boundaries_test.go:61`，以公开 route 契约决定非法时间继续 `400` 还是恢复 caller fallback；先锁定现有 route 行为和调用方影响，再做最小生产修改。
4. **持续门禁**：每个子批次先跑受影响 nextest，再跑 `git diff --check`、`audit_test_parity.py --strict`；文档/清单变更追加 `check:ai-context` 与 `check:quick`，并保留 `check:zero-go` 现有基线失败证据。

- `internal/api/httpserver/bindings_boundaries_test.go:61:TestParseQueryTimeReturnsCallerFallback` 与 `internal/api/httpserver/bindings_test.go:186:TestCandlePeriodAndPaginationNormalization` 已完成逐项断言复核：Rust 周期、空值、公开分页 route 和错误映射均有生产 owner；Go caller fallback、signed pagination helper 与 Rust typed route 存在明确边界，已登记 reviewed residual，未伪装成 exact。定向 nextest 3/3 通过；receipt `sha256:ae87435eb21cbfbcdb9f0bf6216b891f9ec2effccec7dbb7fa95366f7a489dfc`。

- `internal/api/live/dispatcher_boundaries_test.go:205:TestDispatcherEnvelopeDefaultsAndMapFallback` 已逐项复核 Go 的缺省 envelope 与 `mapString` fallback；Rust SSE wire frame 已有真实断言，但 typed event owner 没有同形通用 helper，残余所有权差异已登记 reviewed partial。

- `internal/api/trading/execution_validation_contracts_test.go:238:TestExecutionOrderDetailsRouteMapsMissingAndStoreFailures` 已逐项复核：缺失订单与 store failure 的 HTTP code/error code 已由 Rust execution-read owner 断言；空 ID 的 Gin handler 400 与 Rust typed route 404 路由边界已明确登记，coverage 改为 reviewed partial。

- system routes 的 6 条未收口 partial（system envelope、calendar refresh/probe、validator、malformed release payload、真实交易控制委派/错误映射）已逐项读取 Go 断言并核对 Rust production/system-write owner；可达 HTTP 行为与错误映射已有 receipt，Gin callback、固定 ID、handler seam 差异已分别登记，coverage 改为 reviewed，状态保持 partial。

- system routes 的 6 条未收口 partial（system envelope、calendar refresh/probe、validator、malformed release payload、真实交易控制委派/错误映射）已逐项读取 Go 断言并核对 Rust production/system-write owner；可达 HTTP 行为与错误映射已有 receipt，Gin callback、固定 ID、handler seam 差异已分别登记，coverage 改为 reviewed，状态保持 partial。

- market-data runtime/sidecar 的 5 条 P1 partial（generic/legacy cache、health cancellation、真实 sidecar process、bounded wait、unavailable provider retry）已逐项读取 Go 断言并核对 Rust helper/router/state owner；现有 receipts 支持 covered assertions，配置 seam、取消循环、typed lifecycle 与内容寻址资产差异已分别登记，coverage 改为 reviewed，状态保持 partial。

- `internal/api/assistant/chat_transport_disconnect_test.go:110:TestChatStreamReconnectAndReplayRespectClientDisconnect` 已逐项读取四个 failing-writer/cancel 分支；Rust production ADK replay、after 游标、499 终态和 SseWriter frame/error owner 均有证据，但 API 物化与 Go socket-like writer 的单写次数 seam 不同，已改为 reviewed partial 并保留未来流式化回归条件。

- backtest historical source 13 条 partial（provider session/adjustment、page fetch/error、decimal conversion、sync lifecycle/cancel/pagination、instrument rules/readiness 与 positiveFloat）已逐条复核 Go 原始断言和 Rust owner。现有生产/ helper receipts 支持 covered assertions；provider injection、typed error、progress、lifecycle、resolver 和 signed value seam 作为逐项 residual 登记，coverage 改为 reviewed，未将 partial 误升为 exact。

- runtime handle/server lifecycle 的 2 条 P1 partial 已逐项读取 Go 并核对 Rust owner：并发 close、逆序资源、错误聚合、稳定重复 close 与 backtest-before-marketdata 顺序已有对应 Rust shutdown 证据；generic Resources registry、动态命名和 per-caller aggregation seam 保留为 reviewed residual。

- assistant assembly 首批 8 条 partial（strategy validation/visual model、capability tools/handlers、dependency closures、catalog/operation schemas、runtime strategy tools）已逐项读取 Go registry/tool assertions 与 Rust production/MCP owner；模型写工具缺失、visualModel 校验缺口、ToolDeps callback 与 fail-closed 语义等 residual 已保留并改为 reviewed，未升级为 exact。

- assistant engine recovery/approval/schema/skill 第二批 8 条 partial 已逐项读取 Go 原始分支和 Rust owner；取消后继、父子续跑、CompleteChatRun 三态、confirmation 去重、schema map helper、技能目录替换/文档保留等未覆盖分支已保留为明确 residual，coverage 改为 reviewed。

- assistant store/normalization 第三批 8 条 partial 已逐项复核 session context、store open/delete、provider secret/default、run/approval/memory、低层 JSON、workflow CRUD/log、composer 与 entity normalization；Rust durable invariants 已确认，目录权限、helper fault injection、JSON 错误形态等差异明确登记，coverage 改为 reviewed。

- assistant tool/projection/approval 第四批 8 条 partial 已逐项复核：models/tool helper 边界、projection merge、approval normalization、timeline filtering、审批持久化故障、reconcile 父子生命周期与恢复资格均已绑定 Rust owner；缺少同形 helper、failure injection 或父子 reconcile seam 的残余已明确登记，coverage 改为 reviewed。

- assistant approval/idempotency/canvas 第五批 8 条 partial 已逐项复核：sibling cancellation、busy retry、approval stage/corruption、stale claim、fresh timeout、canvas provider/model override 与 chat fingerprint 均已绑定 Rust owner；取消循环、字段级损坏、claimed 列、节点 override 等残余保持明确，coverage 改为 reviewed。

- assistant context/projection 第六批 8 条 partial 已逐项复核 completion SSE 顺序、provider prefix/handoff、空存储与 fallback id、session read/missing、latest text anchor 及 genai execution descriptor；Rust durable projection/runtime owner 已确认，字符串级 handoff、空存储、字段级 anchor 与 Go genai helper 差异明确登记，coverage 改为 reviewed。

- assistant execution bounds 第七批 8 条 partial 已逐项复核 tool-call reuse/completion、run/event error、pause、final synthesis、approval resolution、rehydrate、run-scoped state 与 buffered delta；Rust runtime/projection/recovery owner 已核对，TIMED_OUT/no-op、genai Content、rehydrate 构造与 delta flush/解绑等差异保留为 reviewed residual。

- assistant execution claims 第八批 8 条 partial 已逐项复核 run/tool lease 输入校验、claim update RowsAffected、closed DB、durable invocation replay、失败调用投影与 keyed handler fencing；Rust claims/store owner 已核对，TTL/空 ID/Abandon、SQL stub、失败 COMPLETED→FAILED 成对投影等残余明确登记，coverage 改为 reviewed。

- assistant execution state/concurrency 第九批 8 条 partial 已逐项复核 lease fencing、tool response lifecycle、派生运行状态、recovery、approval terminal timeline、父子状态、并发 callback 与 event replay；Rust durable owner 已核对，ErrRunLeaseLost/直接审批入口、状态表、delta overlap、父子映射等差异保留为 reviewed residual。

- assistant memory/diagnostics/handoff/input recovery 第十批 8 条 partial 已逐项复核 workspace memory scope、agent ID helper、resumed failure diagnostics、child construction、handoff replacement/revision、session notices 与 input continuation failures；Rust owner 已核对，缺少 appName helper、supersededBy、缺会话/过滤 payload、逐段故障注入等 residual 已明确登记，coverage 改为 reviewed。

- assistant input/lifecycle/normalization 第十一批 7 条 partial 已逐项复核 crash recovery lease、input resolve/requeue/errors、workflow blocking、stale reconciliation、lifecycle store failure 与 response normalization；Rust owner 已核对，关闭库/缺表注入、自引用修复、父运行 plan 与单一 normalize helper 等 residual 明确登记，coverage 改为 reviewed。

- assistant normalization/persistence/artifact 第十二批 8 条 partial 已逐项复核 workflow/session normalize、approval partial-index query、composer、claims 跨连接串行化、artifact 版本读写/并发分配/边界与路径派生；Rust schema/transaction/artifact owner 已核对，EXPLAIN、逐版本读回、并发分配和 session-service 实例路径等 residual 已明确登记，coverage 改为 reviewed。

- assistant session SQLite/store 第十三批 8 条 partial 已逐项复核 direct service/schema health、V1 schema 不变性、Close/nil、current schema、closed/broken metadata 与 prepared run payload；Rust store lifecycle/schema owner 已核对，health API、字节不变、nil/关闭调用和 prepared-run executor seam 等 residual 明确登记，coverage 改为 reviewed。

- assistant persistence/projection 第十四批 8 条 partial 已逐项复核 `ApplyTaskPatch` nil task、陈旧运行恢复持久化失败聚合、审批续跑可重试、子工作流恢复决策、读修复与父级终止、父子续跑 closeout，以及 session projection、approval/memory/canvas 边界。Rust normalization/recovery/CAS/projection owner 已核对；nil 指针、稳定错误文本、失败后重试、子任务字段集、父级终止/read-repair 顺序、父子续跑编排、reasoning 槽/中断剪枝和分散契约等差异均作为 reviewed residual 明确登记，未将 partial 误升为 exact。

- assistant schema/tool-edge/completion-review 第十五批 8 条 boundary/partial 已逐项复核 `nil` schema、task-local goal/剪枝、planner 草稿编译及 completion review 的 unchanged/append/fail-open/一次性收尾/请求约束分支。Rust schema、TaskGraph/CanvasCompiler 和模型 adapter owner 已核对；nil 值、goal 决策快照、planner 专用层、completion-review 端口与 fail-open/append 语义等功能或语言边界均明确登记为 reviewed residual，未将 boundary 误升为 exact。

- assistant completion-review policy/error 第十六批 8 条 boundary 已逐项复核资格/memo、响应解析、提示构造、不适用原因、coordinator 去重，以及 ADK 错误哨兵序列化/原因保留/源码守卫。Rust session projection、typed error 与终态映射 owner 已核对；completion-review 层缺失、errors.Is 级别往返和 AST 禁止字符串匹配守卫均保留为 reviewed residual。

- assistant replay/input/normalization/observability/persistence 第十七批 8 条 boundary/partial 已逐项复核事件重放守卫、输入卡片时间线、nil slice 归一化、关联字段、secret 文件边界、reasoning 快照及 workflow 任务/初始化失败。Rust durable projection、audit、secret writer 与 fail-closed bridge owner 已核对；事件级 helper、卡片排序、nil 语义、reasoning slot、文件故障注入和 workflow task/blocker facade 差异均明确登记为 reviewed residual。

- assistant planner/projection/provider 第十八批 8 条 boundary/partial 已逐项复核 planner 注册名、草稿生命周期与参数依赖、审批恢复错误资格，以及 provider URL、header、私网与 redirect 安全边界。Rust tool catalog、CanvasCompiler/TaskGraph、approval projection 与 HTTP guard owner 已核对；planner 状态机、终局资格、metadata 拦截、header 归一化和 redirect DNS 重解析差异均登记为 reviewed residual。

- assistant Responses/provider safety 第十九批 8 条 partial/boundary 已逐项复核工具名消毒与还原、流式 usage、碰撞拒绝、响应边界、probe 畸形响应及 safe HTTP dial 地址校验。Rust 非流式 adapter、tool extraction、terminal audit 与 reqwest owner 已核对；消毒/流式增量、probe 入口、dial-time DNS rebinding 防护和固定 dial 错误面均保留为 reviewed residual。

- assistant reasoning/resume/timeline 第二十批 8 条 partial/boundary 已逐项复核 reasoning 优先级与快照恢复、Responses 模型选择、流式 final 去重、恢复执行失败传播、rehydrate 失败、子审批回退及空/缺失会话时间线。Rust gate/store/runtime/read owner 已核对；多级优先级、恢复快照重建、流式 delta、逐段故障注入、子审批回退及 Go optional-success store 语义均保留为 reviewed residual。

- assistant runner concurrency/callback/continuation/runtime 第二十一批 8 条 partial/boundary 已逐项复核输入续跑租约、chat callback/event projection、continuation-only 识别与审计、同会话新鲜完成条件，以及执行/续跑/runner 构造分支。Rust fencing、input parity、audit、tool failure 和 runtime owner 已核对；消息分类、continuation_only 审计、陈旧性判定、完整故障矩阵与 Google ADK runner attach/synthesis 层差异均登记为 reviewed residual。

- assistant chat/continuation/goal 第22批 8 条 partial/boundary 已逐项复核 chat runtime 失败传播、RequestedInput unsupported code、execution lease claim/storage、goal resume lease ownership、background context、resume error matrix 与 PauseGoalRun 状态边界。Rust product chat、terminal mapping、recovery/fencing、input continuation 与 mutation route owner 已核对；Go 事件触发面、固定 failure reason、表名错误文本、foreign lease、nullable context 和逐分支 goal 状态差异均登记为 reviewed residual。

- assistant goal/lifecycle/reconciliation 第23批 8 条 partial 已逐项复核恢复状态拒绝、过期调和、CancelRun 树与 helper、目标更新、runner lifecycle、陈旧运行、终态工作流和自引用修复。Rust mutation/expiry/cancellation/recovery/CAS owner 已核对；nil runtime、完整过期矩阵、CancelRun helper 集、目标更新入口、facade 生命周期、自引用任务重置与父级暂停等差异均登记为 reviewed residual。

- assistant lifecycle/plugin/lease 第二十四批 8 条 partial/boundary 已逐项复核父引用与对账 helper、存储停止时 fail-closed、ADK execution plugin、nil execution、租约上下文复用、heartbeat 失败取消、过期刷新和 near-expiry TTL。Rust workflow graph/recovery/projection/claims/cancellation owner 已核对；插件层、nullable execution、公开 lease context、heartbeat 关联、无写入顺序和 remaining-TTL 细节均登记为 reviewed residual。

- assistant runtime lease/store 第二十五批 8 条 partial/boundary 已逐项复核安全默认、关闭闸门、builtin agent/session 初始化、策略刷新、builtin 保护、models.list 过滤、runtime snapshot/probe 和 DeleteSession 缺失运行时。Rust claims/fencing/assembly/catalog/session owner 已核对；默认字段逐项断言、run-lease close API、store ordering、持久 agent 刷新、完整 models.list 过滤及远端缺失/不可用 runtime 语义差异均登记为 reviewed residual。

- assistant runtime/session context 第二十六批 8 条 partial/boundary 已逐项复核 runtime 初始化/关闭、nil-safe helper、审批摘要与用户错误、压缩服务、缺失资源/冲突、snapshot JSON、protected tail 和业务语义 helper。Rust assembly/shutdown/approval/session-context owner 已核对；注册表字段、nil 所有权、错误重写矩阵、压缩 fallback、omitempty、事件保护/裁剪和展示 helper 差异均登记为 reviewed residual。

- assistant session context/retry 第27批 8 条 partial 已逐项复核 manager/wrapped events、审批事件索引、纯 helper、恢复边界、append retry、manager 分支和 stale-session 并发追加。Rust protected-tail/compaction/atomic projection owner 已核对；nil manager、事件索引/摘要/token helper、provider fallback、retry 计数、并发刷新以及 non-stale error bypass 差异均登记为 reviewed residual。

- assistant session stale/skill 第二十八批 8 条 partial/boundary 已逐项复核 unexpected session type、synthetic session、handoff raw session、超大/小工具响应、stale append、wrapped session accessor 和技能目录排序/删除。Rust session store/atomic projection/pending input/context compaction/skill catalog owner 已核对；stale 句柄重试、合成会话创建、raw handoff、阈值裁剪、包装 State 访问器和 builtin-first/ID 规范化差异均登记为 reviewed residual。

- assistant assembly strategy/runtime 第二十九批 8 条 partial 已逐项复核优化任务注册/候选持久化、策略输入与摘要、workflow audit/adapter、系统/任务工具、负例与 fallback、工具可用性/结果视图，以及 provider 默认冻结和并发隔离。Rust optimize/tool/CRUD/trade normalization owner 已核对；注册表查找、完整文案矩阵、audit handler、fallback envelope、默认 provider 无显式覆盖和并发请求隔离差异均登记为 reviewed residual。

- assistant assembly summary/adapter 第三十批 8 条 partial/boundary 已逐项复核策略摘要脱敏与实例计数、工具失败业务契约、领域输入校验、provider/runtime 端口、visual model 归一化、backtest 状态投影、非法 instrument 和 strategy instance 生命周期。Rust catalog/tool/read/dispatch owner 已核对；分散 failure family、adapter 聚合对象、Vue 归属 visual model、完整状态字段矩阵及逐方法空 id 映射差异均登记为 reviewed residual。

- assistant assembly lifecycle/maintenance/news/MCP/portfolio 第31批 8 条 partial 已逐项复核策略实例生命周期、ADK maintenance purge/compact、无 runtime fail-closed、news/corporate-actions 输入规范化与能力错误、MCP server 生命周期及 portfolio 汇总/分层状态。Rust lifecycle/maintenance/market-news/MCP/portfolio owner 已核对；完整 adapter 分支、nil handle、工具命名差异、精确 capability 文案、并发账户排序和 partial warning 字段均登记为 reviewed residual。

- assistant portfolio/adapter/runtime 第32批 8 条 partial 已逐项复核 partial account/discovery、broker funds/positions 拆分、product input helpers、dispatch failure、research-screen 校验、特殊 dispatch/snapshot、跨域 execution contract 与 runtime lifecycle。Rust portfolio/MCP/research-screen/product-runtime owner 已核对；超时 partial warning、combined read call shape、完整 helper coercion、跨域错误矩阵和 Go Open facade 差异均登记为 reviewed residual。

- assistant assembly runtime/tool catalog 第33批 8 条 partial/boundary 已逐项复核 runtime handle 审计/工具操作、nil handle、helper 输入归一化、kline polling、read-tool handlers、metadata helper、核心 market/portfolio flows 与 watchlist paging。Rust audit/read/catalog/watchlist owner 已核对；handle 方法级契约、nullable lifecycle、完整 helper 分支、polling payload 细节、跨工具矩阵和 metadata/response shape 差异均登记为 reviewed residual。

- assistant assembly watchlist/workflow 第34批 8 条 partial/boundary 已逐项复核 watchlist 真实读取与 quote omission、workflow bridge CRUD/run、注入式 workflow executor、管理工具错误传播、session/payload 错误、工具目录/审批矩阵及 workflow wait 边界。Rust watchlist/API/port/canvas/workflow.wait owner 已核对；完整响应矩阵、Go manager/tool catalog、SetWorkflowExecutor、nil manager、workflow_runs.wait 信封与 deadline polling 差异均登记为 reviewed residual。

- assistant workflow tool 第35批 4 条 partial/boundary 已逐项复核 patch 语义、workflow/trigger CRUD、interactive session gate 与 unavailable manager fail-closed。Rust workflow REST/CAS/canvas/runtime owner 已核对；tool-level 完整字段矩阵、session 来源门禁、nil manager 及全量 finalisation 分支差异均登记为 reviewed residual。

- assistant session/skill registry 第36批 8 条 partial/boundary 已逐项复核 compact wrapper List、frontmatter/registry 错误、builtin sync、目录 copy/replace、来源排序/文件元数据、zip 目录项、未知工具告警和 filesystem failure。Rust session/skill install/archive/catalog owner 已核对；包装委托、mtime 同步、目录替换、来源排序、目录项、warning-vs-reject 和 nil/filesystem 故障差异均登记为 reviewed residual。

- assistant skill registry 第二批（第37批）8 条 partial 已逐项复核额外边界、损坏 builtin sync、filtered Source、bundle metadata、压缩包安全、URL/目录安装、纯文档重定向及 archive/uninstall。Rust skill install/archive/download/catalog owner 已核对；nil registry、source allowlist、mtime/compile-time sync、bundle ambiguity/size、URL error/redirect 和卸载响应差异均登记为 reviewed residual。

- assistant skill archive/http/schema 第38批 8 条 partial 已逐项复核 archive/bundle file helpers、filesystem/archive 分支、HTTP/source 分支、确定性安装错误、额外安装边界及 market index/news/corporate-actions schema。Rust archive/http/MCP descriptor/catalog owner 已核对；bundle 内容匹配、frontmatter/source matrix、URL 错误/体积、写盘失败和 skill-document 文本契约差异均登记为 reviewed residual。

- assistant schema/SQLite 第39批 8 条 partial/boundary 已逐项复核 market skill 文档、workflow/schema 严格性、tool metadata、backtest/strategy nested schema，以及 GORM SQLite dialector 的 migration、类型、clause、版本比较边界。Rust MCP fixture/schema、ADK store、migration/query-plan owner 已核对；skill 文本归属和 GORM dialect 语义差异均登记为 reviewed residual。

- assistant schema/store approval/audit 第40批 8 条 partial 已逐项复核业务关键 tool schema、registry alias/mode/numeric helper、审批幂等、运行+审批拒绝、session context 删除、兄弟审批拒绝、完成运行重试和 audit SQL 分页。Rust MCP schema/approval CAS/session lifecycle/audit read owner 已核对；完整 schema required 集、helper 聚合、字段级原始记录、单事务组合、零工具执行及 SQL count/order 细节均登记为 reviewed residual。

- assistant store business/failure 第41批 8 条 partial 已逐项复核 provider 生命周期、agent/session cascade、run/approval/skill/optimization 查询、运营边界、provider 默认/secret/list、实体生命周期、损坏库归一化及默认/失败传播。Rust store/CAS/projection owner 已核对；完整 provider matrix、级联行集、分页/去重字段、逐项 defaults、畸形 JSON/nil patch 和故障注入差异均登记为 reviewed residual。

- assistant store identity/lifecycle 第42批 8 条 partial 已逐项复核 provider/skill/task/optimization identity、session cascade、provider reference/default、SaveRun 终态防回退、fresh approval 重开和 paused workflow 更新。Rust store/CAS/engine projection owner 已核对；生成 ID/createdAt、级联全行集、secret/ordering 矩阵、whole-row SaveRun 及字段级 reopen/pause payload 差异均登记为 reviewed residual。

- assistant store lifecycle/maintenance 第43批 8 条 partial 已逐项复核 goal pause 生命周期、session delete 分类、tool tag/canonical dispatch、skill metadata/tools、maintenance candidate/error、handoff revision、purge cascade/history 和 compact freelist。Rust CAS/session/MCP/catalog/maintenance owner 已核对；whole-row merge、Go/Rust 删除分类、tag 语义、资产 metadata、逐表故障、序列分配、历史保留和 freelist 数量差异均登记为 reviewed residual。

- assistant store ops/skill/tool 第44批 8 条 boundary/partial 已逐项复核 builtin skill 持久化/metadata、过期 bundle refresh、agent template、外部 skill 保护、URL skill 安装、tools.search scope 与 low-risk workflow approval。Rust catalog/projection/skill install/model predicate owner 已核对；文件系统 bundle registry、Store row metadata、磁盘刷新/保护、具体 Neodata 内容、requiresApprovalIn response 与 chat 执行矩阵差异均登记为 reviewed residual。

- assistant store recovery/migration 第45批 8 条 partial/boundary/skip 已逐项复核 disabled primary fallback、损坏 audit payload、closed DB、三条 Go 自身跳过的 migration 用例、resolved approval read-repair 和 approved-run tool failure。Rust store/recovery/tool-failure owner 已核对；Go skip 作为无可执行基线明确记录，fallback、行级 JSON、逐方法 close error、读路径修复和 resolution 字段矩阵差异均登记为 reviewed residual。

- assistant task/timeline/artifact/tool registry 第46批 8 条 boundary/partial 已逐项复核 task runner 并发/取消、workflow planning ordering、timeline projection helpers、tool-output artifact materialization/fallback/safe names 和 registry OnChange。Rust fencing/canvas/projection/artifact/assembly owner 已核对；task-set fan-out、取消后执行、helper 排序、工具输出 artifact 注入/回退、文件名清洗和运行时变更通知差异均登记为 reviewed residual。

- assistant tool schema/network/security/usage 第47批 8 条 partial 已逐项复核 workflow schema strictness/JSON conversion、sessionID invocation、http_fetch redirect/body errors、unsafe host/address matrices、task planner schema、models.list registration和 usage tracker。Rust MCP/HTTP/security/task/compatibility owner 已核对；JSON helper、session echo、redirect/read injection、逐地址枚举、planner fields、registration metadata 与 standalone usage history 差异均登记为 reviewed residual。

- assistant usage/workflow native 第48批 8 条 partial/boundary 已逐项复核 usage tracker 忽略/续接、native agent partial/final、consumer stop、branch/isolation、confirmation recreation、root adapter runtime 分支和 interruptID 匹配。Rust compatibility/stream/cancellation/workflow/input/approval owner 已核对；独立 tracker、native node forwarding/backpressure、branch scope、重建 exactly-once 和 interrupt identity 差异均登记为 reviewed residual。

- assistant workflow agent resume 第49批 8 条 partial/boundary 已逐项复核 long-running call 匹配、unmatched functionResponse、consumed interrupt、resume fail-closed helper、fresh-run 防止、native workflow agent、child NodeConfig 和 tool confirmation resume。Rust approval/input/canvas/workflow graph owner 已核对；Google ADK event/interrupt/native-node 配置语义差异均登记为 reviewed residual。

- assistant workflow approval/canvas/child 第50批 8 条 partial 已逐项复核 approval recovery pause/context、父流程错误、canvas fan-out/join、非法图、可达图执行、子输入挂起恢复、父暂停优先和子完成重开。Rust workflow/canvas/scheduler owner 已核对；二次暂停失败、缺父、完整步骤 metadata、nil canvas、终态字段矩阵、child plan/approval mirror、父重开状态机差异均登记为 reviewed residual。

- assistant workflow child/compiler/goal 第51批 8 条 boundary/partial 已逐项复核非 workflow 父回调、依赖去重/空白、plan step 清洗、父子执行登记、loop 模式选择、缺失 decision、最大迭代和 continue-pause-resume 生命周期。Rust scheduler/canvas/workflow write/goal mutation/assembly owner 已核对；回调抑制、空白依赖、echo rewrite、原子 parent-child registration、决策继续、iteration cap 和完整终态提醒差异均登记为 reviewed residual。

- assistant workflow goal/observation 第52批 8 条 partial 已逐项复核 pause-before-complete、子完成阻塞、PAUSED snapshot、updated objective prompt、active-parent objective 更新、helper/provider failure、节点生命周期投影和 observation helper。Rust CAS/goal mutation/canvas/OpenAPI/catalog owner 已核对；暂停与完成时序、child continuation、活动快照合并、prompt 文本、provider/skill 错误矩阵及摘要 helper 差异均登记为 reviewed residual。

- assistant workflow persistence/planner/reconcile/resume 第53批 8 条 partial 已逐项复核过期终态写入失败、计划 agent 解析、planner runtime、审批通过/拒绝、pending child 重开、父级收敛和 resume loop。Rust CAS/workflow write/runtime/approval/scheduler owner 已核对；父子完成回填、父终止、计划 BLOCKED/暂停、session fallback、error propagation 与 resume-loop 合并状态差异均登记为 reviewed residual。

- assistant workflow store/toolset 第54批 8 条 partial/boundary 已逐项复核 child/workflow resume edge、store malformed/missing/delete、CRUD/soft-delete/logs、registered tool 执行/归一化、product tool schema、不可编码 schema、空 selection 和 agent permission filtering。Rust workflow/store/catalog/tool failure owner 已核对；父失败传播、完整 store 矩阵、toolset exactly-once/response shapes、Go channel 编码错误、nil toolset seam 与权限组合差异均登记为 reviewed residual。

- assistant artifact/memory/workflowexec 第55批 8 条 boundary/partial 已逐项复核 artifact toolset、memory direct tools、动态 load_artifacts、trigger deletion/log lookup、loop chat end-to-end、goal resume fault、decision fallback 和 goal state boundaries。Rust workflow/store/runtime/goal owner 已核对；ADK artifact/memory seams、动态 toolset、完整 trigger matrix、loop task projection、reconcile/persistence errors、goal-specific fallback 和剪枝语义差异均登记为 reviewed residual。

- assistant workflowexec goal/persistence 第56批 8 条 partial/boundary 已逐项复核 pause reconciler、goal turn child failure/task read/save failure、orchestrator persistence failures、terminal projection、goal projection 和 pause error propagation。Rust CAS/bridge/lifecycle/assembly owner 已核对；dropped table、model-start suppression、failure injection、scheduler/blocked parent、child pause 与 decision-boundary propagation 差异均登记为 reviewed residual。

- assistant workflowexec closeout/taskset 第57批 8 条 partial/boundary 已逐项复核 decision/child termination persistence、native task graph provider failure、task toolset/models.list、paused reconcile failure、completion blockers、task lifecycle/complete 和 goalComplete。Rust CAS/fatal/model-list/TaskGraph owner 已核对；native graph、child reconcile、toolset naming、blocked aggregation、runtime task lifecycle 与 goal decision 差异均登记为 reviewed residual。

- assistant workflowexec approval/child 第58批 8 条 partial/boundary 已逐项复核 resume/complete persistence、paused recovery、task/run reconcile、child terminal/fallback、missing agent/final reply、dormant skip、child lifecycle 和 task storage failure。Rust CAS/mutation/reconcile/terminal/expiry/route owner 已核对；workflow-specific failure injection、child/task reconcile、blocked task/fallback agent、dormant executable receipt、RunChild orchestration 和 parent FAILED projection 差异均登记为 reviewed residual。

- assistant workflowexec execution/finalization 第59批 8 条 partial/boundary 已逐项复核 response index、setup failures、run/finalize persistence、save/cancel、executor boundary branches、runtime models catalog、tool name/closed runtime 与 finalized/incomplete plans。Rust task normalization/bridge/CAS/models/projection owner 已核对；iteration index、native graph setup、workflow finalizer、nil executor、provider catalog secret policy、closed error matrix 和 plan persistence 差异均登记为 reviewed residual。

- assistant workflowexec finalization/pause/pending input 第60批 8 条 partial/boundary 已逐项复核 parent plan snapshot、pause-before-model、authoritative parent response、terminal helpers、workflow helpers、pending input/child state、initial persistence 和 terminal persistence。Rust stream/CAS/mutation/graph/input/route owner 已核对；workflow-plan broadcast、next-turn model guard、父级响应组装、decision snapshot、helper matrix、child plan projection 与 goal persistence entry 差异均登记为 reviewed residual。

- 当前批次：Assistant workflowexec persistence/reconcile/approval 的 8 条映射逐项复核（workflow_persistence 67/96、reconcile executor 9/69/110/164、reconcile ignore 9、resume approval 13）。将 Rust CAS、projection、gated-call 与独立运行超时证据拆为 reviewed coverage，并明确迭代上限暂停持久化、父子运行 reconcile、缺失/外来子运行及 delegated child payload 等未覆盖边界；保留原有 partial/boundary 状态与生产函数非 executable test 的引用纠正。

- 当前批次：Assistant workflowexec resume/task state、runtime limit、task toolset lookup/business/error/goal/persistence 的 14 条映射逐项复核。Rust CAS、stream error wire mapping、tool-call failure persistence、gated-call parking、expiry reconciliation、ADK CRUD、deterministic ready-task 和 durable-store error 证据均拆为 reviewed coverage；runtime task overflow、父子 resume 编排、delegate/merge、goal decision tool、current/ready 聚合及 partial-result error 形状等差异明确记录为 uncovered，保留原 partial/boundary 状态。

- 当前批次：Assistant workflowruntime facade 的 2 条构造与运行时装配映射逐项复核。Rust composition-root 路由适配器及内置技能工具目录投影作为实际覆盖，Go facade/session helper、local MCP nil handler、内置 agent/judgment helper 无 Rust 同形 owner 的边界明确记录，保留 boundary 状态。

- 当前批次：Assistant model 的 timeline、workflow graph identity/plan、approval scoping、goal decision/utility 共 7 条映射逐项复核。Rust run projection、restart identity、TaskGraph 排序/环检测/依赖归一与 approval run_id 事实完成 reviewed coverage；时间线 helper 细节、最终回复相对工具顺序、图指纹漂移、展示文案、子运行计划更新、run 作用域读取及 goal decision/planner 强制缺口均明确记录。

- 当前批次：Assistant service 审计分页、内置 agent 字段保护、provider/chat/skill wrapper、session context compaction、runtime unavailable 分支及 optimization metrics 共 6 条映射完成复核。Rust 路由过滤/分页夹取、agent merge revalidation/protected status、技能错误码、会话自动压缩、runtime fail-closed 与持久指标聚合已记录为覆盖；服务 wrapper 合并断言、字段白名单、逐入口 unavailable 文案及优化生命周期合并统计缺口保持 partial。

- 当前批次：Assistant service business 的 PreviewSession、终态聊天投影恢复、CRUD/snapshot 与 run/approval wrapper 4 条映射完成复核。Rust session filter/pagination、完成运行聊天信封重放、snapshot fail-closed、tool catalog、run/approval filters 与幂等结清作为覆盖；PreviewSession 标题截断、运行中 nil 与完整 timeline 恢复、合并 snapshot 字段及服务 wrapper 编排缺口保持 partial。

- 当前批次：Assistant service contract/lifecycle 的 catalog audit、session/run read、runtime unavailable、shutdown、settings timeout、approval-wait duration、PreviewSession fallback 与 terminal chat recovery 共 8 条完成复核。Rust mutation/audit/delete、dynamic routes、fail-closed ports、shutdown lease ordering、settings round-trip、lifecycle audit、session paging 与 nonterminal replay 证据已拆分；逐入口 unavailable、合并 CRUD/read boundary、幂等 Close、等待时长边界、预览回退和 blank run-id 恢复缺口保持 partial。

- 当前批次：Assistant service persistence/runtime 的取消传播、画布节点投影、workflow utility/input、runtime unavailable preflight 与 agent/scheduler resource protection 共 5 条完成复核。Rust cancellation fan-out、严格画布 schema、secret lifecycle、调度/运行入口 fail-closed、agent merge revalidation 与取消传播已列为覆盖；逐资源取消拒写、节点字段逐项投影、ResolveInputAsync/密钥/调度合并边界、所有入口无半成品及 provider 查询取消保护的合并断言缺口保持 partial。

- 当前批次：Assistant service recovery/skill-state 的终态聊天回退、技能恢复契约、审计/optimization 状态恢复与取消持久化失败传播共 4 条完成复核。Rust terminal frame recovery、skill document registration、audit subject filtering、optimization negative/cancel routes 与 restart persistence 作为覆盖；latest-assistant 优先级、畸形技能/缺失删除合并断言、optimization 恢复合并语义及 context.Canceled 原样传播缺口保持 partial。

- 当前批次：Assistant 基础 service 的 timeline error chain、runtime unavailable、runtime settings 与 runtime-free agent templates 共 4 条完成复核。Rust legacy error code/底层错误、unready dispatch、timeout fallback、compile-time catalog 作为覆盖；Go sentinel+cause 前缀、Available/Snapshot/Close 服务 API、StreamIdleTimeoutMillis 暴露值及无 runtime 模板读取形态缺口保持 partial。

- 当前批次：Assistant workflow rules 与 async tools 的 6 条映射完成复核。Rust threshold cooldown、cron parse/semantics、queue atomicity、failure-CAS、trigger/job lifecycle 证据已列为覆盖；EventMatches/冷却三态/标题回退、完整规则边界组合、后台 accepted→完成/跳过/失败日志合并断言缺口保持 partial。

- 当前批次：Assistant workflow CRUD、lifecycle、store failure 的 14 条映射逐项复核。Rust workflow owner 的 tags/pagination/log/delete、webhook/error codes、definition validation、canvas interpolation/cascade、bridge/preflight、manager errors、queue guard、threshold scheduler、shutdown/join 与 not-found 路由证据已拆为 reviewed coverage；逐分支 validation、后台日志、stale resources、写入故障矩阵、helper 边界、Close/store 时序及 in-flight tick join 等剩余差异逐项记录。

- 当前批次：Assistant workflows_extended 与 workflows_resource_recovery 的 9 条映射完成复核。Rust trigger validation resilience、built-in/schedule projection、scheduler tick/worker、active-run CAS、status projection、invalid work mode/not-found、template validation 与 durable failure winner 证据已拆分；完整输入边界矩阵、模板幂等与 watched instruments、事件/调度合并后台链、状态调和矩阵、异步 trigger clone/panic 及 running-transition 写失败注入缺口逐项记录。

- 当前批次：Assistant workflows.go 的 canvas graph round-trip、node trace/result、缺图运行语义与节点输出共 4 条完成复核。Rust write/schema、多节点 pipeline、compiler rejection、failure cascade/node-run persistence 已列为覆盖；逐字段 graph/output 对比缺口明确记录。缺图运行保留 Rust 当前 legacy single-agent fallback，与 Go 必须失败且不回退的语义差异作为显式 compatibility boundary，未伪造 exact。

- 当前批次：CLI launcher 8 条与 API Assistant stream helper/recovery 7 条映射完成复核。Rust launcher E2E、durable stream snapshot/reconnect、terminal frame/replay、agent validation 与 bearer auth 证据已拆分；Go CLI wrapper、进程内 hub TTL/clone/currentRunID、delta 分类、preview state、不可序列化 tool output 与跨 owner helper 形状差异逐项记录为 boundary/partial residual。

- 当前批次：API backtest route 与 live dispatcher 的 8 条映射完成复核。Rust backtest request/route 错误、SSE 初始/provider/trigger/ticker 传播、live provider-family skip、WS provider/origin 帧、subscription 生命周期与 depth subscribe/unsubscribe 证据已拆分；handler 400/404 形状、tick/notification/send failure、resolvedAt 去重、WS 分支错误对象、逐调用 broker 透传、Close 生命周期和 depth coalescing 缺口逐项保留。

- 当前批次：API live handler 与 marketdata route boundary 的 9 条映射完成复核。Rust WS close/origin/depth/notification、candles provider code、microstructure capability 与 active-provider ownership 证据已拆分；Close code/corpus、resolvedAt refresh、真实通知重放、Host same-origin、非 Futu depth、brokerId 优先级以及 Go broker-reader 错误通道差异逐项保留。

- 当前批次：API marketdata news/actions 与核心 read routes 的 5 条映射完成复核。Rust instrument URI 400、capability 409、explicit broker fail-closed、catalog/security/snapshot/heartbeat/normalize 分散证据与 provider error fixtures 已列为覆盖；route template 404/handler 400、news/actions 错误码与 Retry-After、broker-reader 架构差异、七段合并顺序及 Futu snapshot 502/503 分类差异逐项记录。

- 当前批次：API middleware 的 ADK availability、public auth paths、trusted request context、logout protection 与 write-method detection 共 5 条映射完成复核。Rust route fail-closed/503、login/session bypass、origin/CSRF、写方法分类证据已拆分；/health 缺失、ADK read 错误码、logout 200/204、Go request marker/nil pointer 语言与所有权差异逐项保留。

- 当前批次：Research Screen productfeature 的 9 条 API 映射完成复核。Rust catalog/版本/page/definition 校验、fixture projection、capability/operator 错误证据已拆分；dirty broker/market normalization、typed definition forwarding、v2 保留/V1 shape、unknown total、embedded provider selection、完整 POST projection 与 conflict matrix 未覆盖项逐项记录。

- 当前批次：Productfeatures 总路由 4 条复合映射完成复核。Rust prediction/market-data/options/snapshot/subscription、typed research forwarding、capability/lifecycle/eligibility/provider errors 与 query/instrument helper 证据已列为覆盖；完整 route catalog、全类型化 wire/query 矩阵、跨路由错误矩阵及 predictionRoute 归一缺口逐项记录。

- 当前批次：Settings account/failure boundary 的 7 条映射完成复核。Rust managed-account identity、rebuild errors、settings rollback、notification normalization、onboarding state、data-management auth 与 MCP/security errors 已列为覆盖；HTTP 级 body-id/500、rebuild 合并矩阵、11 路写入失败、通知回读落盘、onboarding reset、callback failure 及 listener/token persistence failure 缺口逐项记录。

- 当前批次：Settings routes 的 legacy response、MCP token、execution/calendar、UI/onboarding/security/ADK、data-management 与 notification test 共 9 条完成复核。Rust broker/delete shapes、removed-route registration、one-time hashed token、settings normalization/reload、typed cleanup preview 与 notification delivery 证据已拆分；移除路由 HTTP 404、旧 token 失效、注入 service 调用计数、pine-worker round-trip、组合矩阵、typed callbacks 与 unavailable error 缺口逐项记录。

- 当前批次：Settings legacy yfinance route removal 与 URI boundary 2 条映射完成复核。Rust unknown-route JSON 404 与 typed path matching 证据已记录；Go 专门 legacy-prefix 四方法 fixture 及缺 URI 400 handler 形状在 Rust router ownership 下保持 boundary。

- 当前批次：Origin、prediction combo 与 embedded provider research/news/company/calendar 的 7 条映射完成复核。Rust production origin/CORS、prediction defaults/errors、corporate/news、research capability/lifecycle、ranking/industry/company/calendar wire owner 证据已列为覆盖；Go helper fallback、合并错误路由、逐操作参数/字段矩阵及 news/actions Retry-After 路由驱动缺口逐项记录。

- 当前批次：Strategy Pine API contract 的 7 条诊断/元数据映射完成复核。Rust success/unsupported-source/syntax-shadow/opaque projection fixture 证据已拆分；indicator requirements 独立键、pine-v6 文案、syntax code/line、v20 parse-only、object signature、import alias 与 type/method registry 专项断言缺口逐项记录。

- 当前批次：Strategy route boundary/failure 的 8 条映射完成复核。Rust typed path 404、runtime start rollback、Pine fixtures、definition failure recovery、instantiate/input、mutation isolation、plugin fail-closed 与统一 ApiFailure 模型证据已拆分；缺 URI 400、start preflight/capacity、Pine 400/502、list/read/preview、orchestration/instance 逐路由错误、plugin whitespace 及 nil error 结构差异逐项保留。

- 当前批次：Strategy lifecycle/plugin 的 7 条映射完成复核。Rust definition ID/delete guard、version cutover、七路由状态转换、delete busy、malformed/start failures、plugin catalog/mutations/guidance 与 fail-closed 证据已拆分；linked delete 序列、version wire/缺参、runtime.Stop/参数、missing delete、九分支错误矩阵、五路由组合与 plugin 404/500 矩阵缺口逐项记录。

- 当前批次：Strategy error helper 与 system status mapper 2 条映射完成复核。Rust typed error projection 与 canonical status JSON 证据已列为覆盖；Busy 空消息/generic fallback 及 Go typed DTO unknown-field 伪断言边界明确记录。

- 当前批次：Trading execution command/read/validation 与 OpenAPI route alignment 的 6 条映射完成复核。Rust trade error branches、orders filters、receipt/not-found、account.orders forwarding、ID trim/encoding、route registry/OpenAPI transport gate 证据已拆分；九分支合并矩阵、worker current/history 计数、PARTIALLY_FILLED fixture、空 ID 400 与逐标识 route 集合等式缺口逐项记录。

- 当前批次：Trading broker/portfolio read-write 的 7 条映射完成复核。Rust positions/funds/orders/fees/cash projections、broker write cutover、fail-closed 与 unsupported-write probes 证据已拆分；缺 broker/错误 broker、degraded 200 vs 503、helper 六类 HTTP 错误、参数归一、no-active fallback 与 validation-order 差异逐项记录。

- 当前批次：Watchlist route/lifecycle 与 apiserver application ownership/runtime 的 10 条映射完成复核。Rust watchlist fixtures/cutover、composition dependency order、WriterLease ownership、shutdown idempotence、resource failure/logging 与 runtime dependency snapshot 证据已拆分；URI 400/404、完整单条 watchlist 生命周期、nil-safe paths、generic Installers、懒注册/late registration、errors.Join 聚合及 nil service 形态差异逐项保留。

- 当前批次：Datamigration maintenance failure paths 的 7 条映射完成复核。Rust candidate fingerprint/rollback、WriterLease busy、backup discovery、preview defaults/expiry/confirmation、storage inspection/candidate fail-closed 与 corrupt-marker compact 证据已拆分；Go hook/context/VACUUM 注入、marker 并发重读、empty-path error、incompatible backup/missing compact 与 descriptor 注入差异逐项记录。

- 当前批次：Datamigration maintenance/manager/rebuild/research 的 16 条映射完成复核。Rust backup/privacy/lease/preview/inspection/marker/rebuild/research owner 证据已拆分；Go rate-limit、busy re-read、hook/context 注入、SetUnavailable、CompletePending、version drift、atomic marker faults、single rebuild/global lock/file sets/select boundaries 与 research apply/reopen/complete 缺口逐项记录。

- 当前批次：Desktop API startup 与 Futu/OpenD coordinator/probe 的 8 条映射完成复核。Rust native lifecycle、readiness snapshot、connection replacement、OpenD diagnostics、onboarding/system status、probe/version/neutral-state 证据已拆分；Wails helper、HTTP polling timeout、应用顺序、retry diagnostics、runtime/account 合并、market-health 文案、disabled probe/settings error 与 bool matrix 差异逐项记录。

- 当前批次：Databaseguard route-family availability 1 条映射完成复核。Rust corruption/WriterLease startup fail-closed 作为更强 ownership 证据，Go API-online partial DATABASE_INCOMPATIBLE 语义明确记录为结构边界。

- 当前批次：Apiserver lifecycle 的 13 条映射完成复核。Rust web runtime bind/conflict/rebind、startup/shutdown ownership、Tauri frontend/API 分离与 readiness/oneshot 证据已拆分；Go noop/value matrix、public loopback policy、共存编排、hot rebind、注入恢复、handler 关闭聚合、embedded/integrated 模式与 context cancellation 差异逐项记录。

- 当前批次：Liveapp bbgo notification 与 Assistant market-provider 的 8 条映射完成复核。Rust ExecutionNotificationProjector/LiveHub 边界、provider REST/settings selection、whitespace validation、rollback、health fail-closed 证据已拆分；bbgo 退役桥、panic 捕获、Wails handler、Assistant aggregation envelope、before/after、nil ports 与 unknown-health 语义差异逐项记录。

- 当前批次：MarketData data-plane switch 与 live heartbeat 的 7 条映射完成复核。Rust demand/reconfigure、failed warmup、provider readiness、uncomposed runtime、WS heartbeat/snapshot poll 证据已拆分；同步恢复、active failed selection、nil callback、heartbeat freshness/retry/transport 统计、legacy timestamp 与 policy matrix 缺口逐项记录。

- 当前批次：MarketData depth 与 security HTTP 的 10 条映射完成复核。Rust OpenD depth projection、US/HK normalization、security snapshot 与九字段 broker-neutral envelope 证据已拆分；Go request/depth/meta envelope、exchangeType/fromCache/static-info 及 warrant/option/future/trust/index/plate 研究块契约差异逐项记录。

- 当前批次：MarketData provider boundary/delegation 的 6 条映射完成复核。Rust disconnected provider、catalog search/filter/failure、candle pagination/conversion、production port forwarding 与 prefix inference 证据已拆分；Go Provider 构造/nil/disabled envelope、nil snapshot/UnavailableReason、cache wrapper、单对象 callback 委派及 BAD.CODE 大小写差异逐项记录。

- 当前批次：MarketData runtime health 的 5 条映射完成复核。Rust failed-health recovery、failed warmup、unsubscribe retry、startup/explicit readiness 与 last_error 证据已拆分；sidecar 计数/合并错误、激活后延迟 release 序列、provider-type health 调用次数及 Failed 单探针停止重试缺口逐项记录。

- 当前批次：MarketData runtime provider-switch/cleanup/backtest 的 17 条映射完成复核。Rust ActiveProviderState transitions、reject/close races、unsubscribe retry、startup fail-closed、catalog/backtest selection 与 shutdown ownership 证据已拆分；Go sidecar deferred cleanup、lease pinning/refcount、inactive cleanup gate、context expiry/cancel、health-preparer failure及详细 push/quote matrix 等差异逐项记录。

- 当前批次：MarketData runtime 的 Python helper 解析/探针、query-time fallback、AKShare shared-sidecar recovery 与 forwarding/close 9 条映射完成逐条复核。Rust asset/process/health、provider activation recovery、query 400、canonical forwarding 与 shutdown selection 证据已改为 reviewed coverage；Go 解释器 env/venv/PATH、external/embedded 状态结构、旧环境变量别名、python -c module probe、shared-sidecar 计数、全数据面聚合转发、四能力错误矩阵及 close/ensure 错误路径缺口逐项记录为 boundary/partial residual。

- 当前批次：MarketData sidecar process/signal 的 8 条映射完成逐条复核。Rust restart/stop、asset materialization、helper config、readiness 与 bounded kill 证据已改为 reviewed coverage；旧 Go 的重启清理链、Stop 失败保留重试、Python source-mode 参数/PYTHONPATH 逐字段校验、YFINANCE_SIDECAR/PYTHONPATH 解析期错误、Python 版本模块探针及可注入 signal plan/immediate-kill 分支逐项记录为 partial/boundary residual。

- 当前批次：MarketData unavailable-provider 与 watchlist source/quote 的 7 条映射完成逐条复核。Rust fail-closed provider routes、active-selection guards、watchlist assembly/read port、helper missing-value 保留、symbol-scoped error 与 closed-session quote 证据已改为 reviewed coverage；Descriptor/Health 聚合矩阵、Go 指针/字符串胶合、Futu/Python/AKShare source selector、无 Python fallback 计数、Unavailable 分类及时钟/盘后算术细节缺口逐项记录。

- 当前批次：yfinance conversion 的 18 条 snapshot/candle/volume/calendar/catalog 映射完成逐条复核。Rust extended-session projection、非有限值拒绝、高精度 volume、calendar window、candle pagination/session classification、catalog normalization 与 helper numeric validation 证据已收口为 reviewed；yfinance/Futu 共用投影层、字段命名、Decimal/null 表达、分页游标 owner、strict/bounded 模式、closed-bar 聚合裁剪、候选可选字段与 provider source 注入等差异保留为 partial residual。

- 当前批次：MarketData instrument resolver 的 15 条映射完成逐条复核。Rust qualified lookup、unsupported/ambiguous resolution、CN 过滤去重、provider-prefix 归一、先全窗口后 limit、generation fence、输入/错误/取消边界及路由投影证据已收口为 reviewed；Go 进程内 TTL/singleflight/cache recheck/error-cache、调用参数粒度、纯函数分类、精确排序与 context.Canceled 专项断言在 Rust 无同形 owner，逐项保留为 partial/boundary residual。

- 当前批次：MarketData collector 与 subscription lifecycle 的未审查映射完成逐条复核。Rust generation fence、stale callback rejection、bounded/idempotent close、poll retry/backoff、demand TTL/managed lease、atomic acquire/clear、capability-before-lease、provider rollback/reconcile 与 shutdown release 证据已收口为 reviewed；Go 阻塞 Connect/Reset 时序、handler/stream 调用计数、动态 source 分流、重叠 poll/cancel 计数、64-goroutine 压力、deferred nil-return 错误诊断、broker 装饰字段和逐次 refs 集合等差异逐项保留为 partial residual。

- 当前批次：Marketdata asset selection/cache 的 27 条映射完成逐条复核。Rust content-addressed AssetBundle 的命名/摘要校验、原子物化、篡改修复、并发赢家、过期清理、cache-root 防护、缺失资产与 fail-closed IO 证据已收口为 reviewed；Go PyInstaller onedir 多文件遍历、权限/符号链接/非常规文件形状、私有临时目录与 Cleanup、TMPDIR 注入、digest 对账和旧错误文案在 Rust 单文件资源模型中无同形 owner，逐项保留为 partial/boundary residual。

- 当前批次：AKShare client/conversion boundaries 的 14 条映射完成逐条复核。Rust helper health/retry、typed capability/busy propagation、market/security/candle validation、fundamentals omission、pagination、provider unavailable 与 malformed projection 证据已收口为 reviewed；Go Retry-After/attempt 计数、adjustment 缺省不编码、候选 selectable、continued/cursor 布尔、legacy batch/defaultSource、单次 read failure 及完整错误文案差异逐项保留。

- 当前批次：MarketData provider-switch lifecycle/boundary 的 13 条映射完成逐条复核。Rust managed-consumer gate、generation fence、failed-change rollback、poll-only health/read capability、cache invalidation、single-commit races 与 bounded close 证据已收口为 reviewed；Go nil 指针/辅助 deadline、ErrProviderChanged 文本、poll-only managed lease、health probe 原文、激活阻塞/等待屏障、goroutine 顺序竞争、stream close 计数和 decimal pointer identity 差异逐项保留。

- 当前批次：MarketData TickCache 与 dev/release asset 入口的 11 条映射完成逐条复核。Rust cache dedup/freshness/session promotion、tick-candle volume/limit、serialization、shared cache/read fallback、平台资源 integrity 与开发/发布 helper 选择证据已收口为 reviewed；Go wall-clock 注入、SnapshotJSON 串联、Service 端到端单 cache、Release/Materialize 双 API、临时目录 fallback、PyInstaller onedir/cache wrapper 与可传平台参数等差异逐项保留。

- 当前批次：MarketData service facade、calendar/macro、company research、index constituents、news/actions、rankings/industries 与 screen 的 26 条映射完成逐条复核。Rust provider capability/lifecycle fail-closed、参数映射与投影、CN 叶市场归一、缓存/refresh、provider status/demand、screen shape/page 与错误分类证据已收口为 reviewed；Go 逐操作错误文本、limit/offset 全矩阵、CN 三类读逐调用转发、broker 装饰/nil 序列化、纯 helper、provider call-count 及默认值串联差异逐项保留。

- 当前批次：bbgo indicator/types 的 23 条映射完成逐条复核。Rust 指标窗口/算术与权益峰值的有限兼容证据已收口；Go Queue/Series/Array/switchIface/Clone、NextCross、Pearson/Spearman/Cov/Skew/Entropy/Softmax/Sigmoid、LogisticRegression/OLS/Dot/Filter 及 go-chart Plot 等专用助手逐项确认无 Rust 生产 owner，保留为明确语言/职责边界。

- 当前批次：Strategy indicatorbinding/parse 的 22 条映射完成逐条复核。Rust Pine parser/planner 的均线键、时间单位、正整数周期、百分位边界与风险声明 owner 证据已收口；Go DSL 函数/参数切分、函数名/均线/数量/保护模式归一、窗口策略、正浮点/百分比助手、参数元数与整数转字符串等无同形 Rust helper 的语言/架构差异逐项记录。

- 当前批次：Pine worker command executor 的 22 条 backtest/strategy execution 映射完成逐条复核。Rust intent sizing/close/cancel、market-step skip、unavailable fail-closed、warning dedup、atomic bracket、OCO/审计、clientOrderId fallback 与 signal validation owner 证据已收口；Go worker-local activeOrders、short replay tag、board-lot 专项、warning collector 形态、逐腿 OCO cancel、完整 malformed bracket 矩阵及未跟踪/已终结撤单边界逐项记录。

- 当前批次：Conservative bar executor 的 17 条未审查映射完成逐条复核。Rust 撮合校验、next-open/liquidity cap、atomic bracket、reduce-only、取消、收盘价、滑点/gap、stop order、告警与 helper 分支 owner 证据已收口；Go PARTIALLY_FILLED→FILLED 状态流、警告文案/集合、内部 pending 切片、逐形态错误矩阵及结果模型字段差异逐项保留。

- 当前批次：Backtest service 的 14 条映射完成逐条复核。Rust backtest start/研究回测 dispatch、chart normalization、data readiness/sync 生命周期、warmup、result view、validation、worker failure、shutdown cancellation、store lifecycle 与 single-writer owner 证据已收口；Go 默认字段/定义派生、coverage 参数与最大 warmup、任务清理/调用次数、结果游标/OHLCV 精确值、Close 后拒启、runner error 文本和内存兜底差异逐项记录。

- 当前批次：Backtest K-line store 的 13 条映射完成逐条复核。Rust manifest/schema、维度表名、同步 CAS、scope fail-closed、覆盖验证、低周期聚合、5m→15m 来源优先级与 interval alias owner 证据已收口；Go compact schema/逐字段 round-trip、scoped version、regular/extended fallback、rehab 查询过滤、overlap Verify、非零 UTC backward limit 与完整周期映射矩阵差异逐项记录。

- 当前批次：Pine worker backtest adapter 的 12 条映射完成逐条复核。Rust worker intent→确定性撮合、short close/buy、数量解析、stop/stop-limit、atomic bracket 校验、worker invalid/unavailable 与结果模型 owner 证据已收口；Go command 转换层、command 字段、sell-entry short/cover 归一、默认 quantity、OCO 双腿展开、逐类错误矩阵及 metadata/commands wire 形态差异逐项记录。

- 当前批次：Backtest storage runtime invariants 的 14 条映射完成逐条复核。Rust maintenance fail-closed/lease、schema/session scope、canonical interval table、calendar aggregation、paging、coverage validation、transactional write、DST cutoff 与 recovery owner 证据已收口；Go dynamic-type/table-existence cache、候选表 fallback、limit helper、批量中途失败回滚、边界助手、closed-store、unavailable storage、session-aware gap 与不变量错误文案差异逐项记录。

- 当前批次：Pine runtime manager/process 的 13 条映射完成逐条复核。Rust settings worker limits、asset selection/integrity、explicit process injection、loopback auth、pool reservation/rollback、readiness、live session revisions 与 stop/open failure owner 证据已收口；Go 20 项环境变量合并、embedded/external Source、禁用非法值、repo/workdir fallback、Option factory、Manager/Runner 成对发布、nil/Close 聚合、容量/超时与第二组件失败回滚等差异逐项记录。

- 当前批次：Pine collection parse/compile 的 17 条映射完成逐条复核。每条 Go V20–V26 collection/UDT/method/tuple/dynamic-loop/request.security 场景均核对 Rust 同脚本拒绝诊断与 compiler owner；语言版本闸门和未实现特性族的差异、升级路径及缺少 typed projection 逐项保留为明确 residual，未伪造 exact。

- 当前批次：Pine parse/compile 剩余 13 条映射完成逐条复核。Rust helper diagnostics、strategy quantity/notification、exit/stop/cancel/close metadata、UDF/static-for、unsupported case 与 switch 多语句边界 owner 证据已收口；Go command 字段/数量模式、when 专字段、profit/loss ticks、逐形态 advanced-order 错误、UDF/switch 完整语言族差异逐项记录。

- 当前批次：Backtest session-filter store/query 的 11 条映射完成逐条复核。Rust scope/table 隔离、extended/regular 防污染、确定性分页、自定义周期、存储错误恢复与 schema 校验证据已收口；Go wrapper 委托计数、streamer/channel fallback、custom extended-hours range、nil 行过滤、游标回退/裁剪及 helper 层边界在 Rust 无同形 API，逐项记录。

- 当前批次：Futu marketdata runtime 的剩余 11 条映射完成逐条复核。Rust provider runtime shutdown/rollback、ActiveProviderState publish fence、OpenD listener/tick projection、fallback instrument filtering、extended quote/session windows、unavailable ports 与 delayed snapshot owners 已收口；Go Ensure/Reset 并发窗口、幂等 Close 计数、nil runtime、热替换、bbgo trade channel、逐查询 fallback 合并与 source 字段差异逐项记录。

- 当前批次：live strategy command/executor/runtime manager 的 38 条映射完成逐条复核。Rust intent execution、short/close/quantity、conditional orders、market-step、audit skip、broker/provider guards、streaming capability、health/binding gates、session reservation 与 lifecycle owner 证据已收口；Go command DTO、warning sink/tag、board-lot/market-rules 开关、实时 OCO、逐字段缺失依赖、账户解析调用计数、重复激活取消、callback 文案与逐类错误矩阵差异逐项记录。

- 当前批次：Pine object/collection V27–V30 与 request.security diagnostics 的 11 条映射完成逐条复核。Rust 七类 request.security 诊断矩阵已核对 executable owner；V27–V30 object/history/method/MTF/declaration/type/import 语言族的同脚本拒绝诊断、缺少 typed IR 与升级路径逐项记录，未将拒绝误记为功能等价。

- 当前批次：Market/calendar market 与 normalization 的 21 条映射完成逐条复核。Rust session context、holiday/early-close/DST、HK/China lunch、instrument normalization/profile、calendar lifecycle/manual override 与 daily/weekly/monthly completion owner 证据已收口；Go 全矩阵 TradingPeriod/TradingDay/LabelStart/Bucket helper、精确分钟与 timestamp、global resolver swap/reset、SG/CNSH alias、US/non-US fallback 和错误文案差异逐项记录。

- 当前批次：bbgo RBTree 的 10 条映射完成逐条复核。Rust 以 BTreeMap/Vec 和订单簿/深度投影承担有序价格层级；红黑树旋转、父指针、不变量、CopyInorder/独立拷贝、随机/压力插删等 Go 专用容器行为逐项确认无 Rust 生产 owner，边界依据已收口为 reviewed。

- 当前批次：yfinance provider 的 11 条映射完成逐条复核。Rust helper descriptor/client、candle/session conversion、CN/US normalization、strict window、identity drift 与 snapshot batch owner 证据已收口；Go Yahoo polling/forward-only descriptor、sidecar 端点串联、depth unsupported、extended volume、7-day 1m 上限、部分成功错误聚合和 limit clamp 的字段/调用粒度差异逐项记录。

- 当前批次：Futu watchlist source 的 10 条映射完成逐条复核。Rust group/member conversion、security snapshot coordinator 分片/单项错误/rate-limit、delayed fallback、quote metadata/session projection、remote source identity/unavailable 与 OpenD readiness owner 证据已收口；Go remoteGroupId/cache call-count、完整失败重试批次、市场级文案/顺序、未知时区精确字段、source.Status/Error/IsConflict 投影差异逐项记录。

- 当前批次：SQLite connection/DSN 的 10 条映射完成逐条复核。Rust single-connection/busy-timeout、WriterLease、read-only shadow、PRAGMA persistence、fail-closed open、schema invariants 与 foreign-key owner 证据已收口；Go connection-pool size/read concurrency、DSN query 拼接、空字符串专测、跨连接 cascade、driver error 文案和 pool normalization 无同形 Rust API，逐项记录结构边界。

- 当前批次：Strategy definition store 的 12 条映射完成逐条复核。Rust SQLite definition versioning/restart durability、legacy/corrupt/drift rejection、transactional writes、soft-delete/history、normalized binding persistence 与 maintenance lease/fail-closed owner 证据已收口；Go legacy JSON/v1/runtime-source migration、unchanged snapshot/purge、trigger-injected rollback、UUID format、逐字段 error mapping、不可持久化模型与 stale-candidate 判定差异逐项记录。

- 当前批次：bbgo market helper 的 9 条映射完成逐条复核。Rust market rules/tick-size、Fixed8/Decimal truncation、pre-trade notional/option multiplier 与 liquidity warning owner 证据已收口；Go Market 方法、定宽文本格式化、Duration 字符串解析、自动调高 min-notional/contract-size 数量等 API/展示语义差异逐项记录。

- 当前批次：Pine semantic parse 的 12 条映射完成逐条复核。Rust planner semantic feature/TA support、visual warning/metadata 与 compile/analyze owner 证据已收口；Go typed IR/semantic summary 字段、未实现 TA/utility/signature/static-for 族及视觉 namedArgs/variable 精细字段差异逐项记录。

- 当前批次：Backtest aggregation/result collector/session synthesis 的 24 条映射完成逐条复核。Rust interval/session scope、calendar aggregation、result trades/PnL/drawdown/partial fills、fees/warnings/finalization 与 deterministic paging owner 证据已收口；Go 自定义 interval 反解析、纯 helper 越界、US/HK 2h session buckets、stream/channel fallback、fee 空分支、identity 回退、heikinashi seed、逐市场午休/会话分页矩阵差异逐项记录。

- 当前批次：Pine worker manager/client 的 19 条映射完成逐条复核。Rust WorkerPool 快照/轮转/容量、health/restart/readiness/diagnostics、gRPC request/response/error/job identity、encoded message limits、transport boundary 与 shutdown owner 证据已收口；Go 排队与 RejectWhenBusy 开关、逐 worker 首错、JSON size/performance gate、metadata defaults、nil transport、进程清理/日志尾和逐字段错误文本差异逐项记录。

- 当前批次：SQLite schema catalog 的 16 条映射完成逐条复核。Rust schema manifest/static-dynamic shape、metadata/error precedence、drift/integrity/foreign-key、migration rollback/downgrade、non-mutating validation 与 managed database owner 证据已收口；Go 防御性副本、未知 ID/非法路径、逐查询注入失败、版本漂移文案、v2→v3 rebuild 判定及公共参数边界差异逐项记录。

- 当前批次：Backtest input/readiness/sync 的 19 条映射完成逐条复核。Rust range/provider/session defaults、missing coverage fail-closed、readiness/sync lifecycle、persisted progress、orphan recovery、worker cleanup 与 request validation owner 证据已收口；Go RequestError 动态类型/文案、adapter 工厂/关闭时序、provider pin call-count、终态清理矩阵、TaskID 唯一性、unknown rehab fallback 与完整参数组合差异逐项记录。

- 当前批次：Trading order-updates 的未审查映射完成逐条复核。Rust reconciliation worker polling/wake、store scan fencing、bounded invalidations、inactive/degraded status、terminal fee fail-closed 与 lifecycle owner 证据已收口；Go 内存 TTL/cache copy、broker subscription/resubscribe/refresh、per-subscription metadata、batch fee dedup、nil helper/upsert/query builder 与订阅失败 fallback 差异逐项记录。

- 当前批次：AKShare provider 的 8 条映射完成逐条复核。Rust helper descriptor/loopback client、adjustment/session rejection、DTO snake_case、generic error/partial result、identity normalization 与 snapshot batching owner 证据已收口；Go polling-only capability/端点矩阵、sidecar 不触达计数、not-found/warming 文案、指数前缀、regular-only 与 batch size 具体值差异逐项记录。

- 当前批次：`internal/watchlist/service_test.go` 的 8 条 service CRUD、输入校验、不可用守卫、source health/remote groups、import preview/commit 映射完成逐条复核。Rust watchlist membership-plan 规范化、fail-closed 端口、SQLite 读取/持久化、preview/commit 生命周期与 revision fence executable owner 已收口为 reviewed；Go 的 9 类 repository 委托参数矩阵、repository 未写入计数、ErrUnavailable 形态、source health/nil connector 与四类同步失败、fresh connector requestedIDs/metadata、invalid snapshot 错误分类、freshGroupIDs 重验证及 expired/stale/invalid commit 错误矩阵逐项保留为 partial residual。

- 当前批次：API server runtime 环境与启动配置的 8 条映射完成逐条复核。Rust ProductConfig/组合根、runtime resources、SocketAddr bind 校验与 Tauri profile owner 已收口为 reviewed；Go 的 envOrDefault/firstNonEmpty/besteffort helper、旧 JFTRADE_ADK_* 变量别名、GUIBind/集中式 EnsureRuntimeLayout、IPv6 字符串归一、进程级 env 注入与 FUTU_OPEND_ADDR/JFTRADE_FUTU_* 兼容变量逐项保留为 partial/boundary residual。

- 当前批次：API server runtime handle lifecycle 的 10 条映射完成逐条复核。Rust 组合根所有权、Option 端口、typed shutdown、日历资源恢复、Pine/Assistant 有序关闭与一次性 worker 装配证据已收口为 reviewed；Go 的 nil handle、运行期分组身份发布、迟到资源/runner 注入拒绝与恰好关闭一次、全局 calendar resolver 替换、100 轮 Set/Close 竞争及 EnsurePineWorker 并发去重路径在 Rust 不可达，逐项保留为 boundary/partial residual。

- 当前批次：API server `server_test.go` 的 14 条启动、桌面 profile、CORS origin、绑定失败、数据库重建与 shutdown 映射完成逐条复核。Rust ProductConfig/native lifecycle、Tauri profile 隔离、loopback 白名单、listener 冲突恢复、rebuild snapshot/marker 与 shutdown oneshot owner 已收口为 reviewed；Go 的集中式 EnsureRuntimeLayout、args noop 门控、已配置 web password 的不变性、profile 覆盖持久化 apiBind、:0/IPv6 全矩阵、legacy Wails server、动态 FRONTEND_DEVSERVER_URL/Wails CORS 回显、桌面 API 绑定端到端、rebuild 中间态 marker 与 context cancellation 返回路径逐项保留为 partial/boundary residual。

- 当前批次：Assistant chat stream 与 backtest historical source 的 4 条映射完成逐条复核。Rust loopback ADK stream/replay/after cursor、session lifecycle、backtest session scope、Futu RTH/ETH/ALL 路由与缺 worker fail-closed owner 已收口为 reviewed；真实 Provider live 调用、fresh-frame replay=false、hub TTL cleanup、旧 providerSessions 任意 overnight 过滤、函数式 ProviderOptions 与 market-data runtime 依赖数量等差异逐项保留为 boundary/partial residual。

- 当前批次：servercore data management 的 7 条映射完成逐条复核。Rust typed CleanupPreview/Maintenance/Overview 服务、DatabaseId fail-closed、维护 busy 预览/执行 fence、SQLite 单一运行记录、局部化 purge 错误与取消/冲突/过期映射 owner 已收口为 reviewed；Go 的 datamigration.Backend 薄封装、行情/策略逐库 busy reason 注册表与中文文案、nil/closed store 注入、损坏 marker 的路径查找静默忽略、内存与 DB 双写同步及 ErrBackupRateLimited 映射逐项保留为 partial/boundary residual。

- 当前批次：servercore ADK maintenance、assistant transport、broker read default 与 desktop token 的 6 条映射完成逐条复核。Rust 数据维护 busy/candidate fence、ADK port 有序关闭、交易请求市场归一、system-control bearer 认证、独立 web listener 与 Tauri token fail-closed owner 已收口为 reviewed；Go 的 ADK 运行探测/四类 DB_BUSY 注册表、关闭后 chat stream 503、TradeMarket(US) 默认注入、WebSocket 协议头 token、sidecar/browser listener 混用与无 token 开发信任逐项保留为 partial/boundary residual。

- 当前批次：servercore execution writeback、instrument normalization、live heartbeat 与 diagnostics 的 4 条映射完成逐条复核。Rust 对账身份复用/分页去重、CN 前缀推断、live transport metrics 去重排序、WebSocket limit/origin fail-closed 与 permit 释放 owner 已收口为 reviewed；Go 的 placed 回写已发现订单专用事件矩阵、CN 裸 code 歧义拒绝、策略持仓排除/nil service、诊断载荷字段回显逐项保留为 partial residual。

- 当前批次：`cmd/jftrade-desktop/main_test.go` 的 21 条桌面 CLI/Tauri 入口映射完成逐条复核。Rust desktop contract、profile identity/data isolation、bearer token、asset/runtime facade 与 native lifecycle owner 已收口为 reviewed；Go Wails zoom/CSS/menu/backdrop、single-instance callback 注入、完整 application object、runtime-config.js HTTP handler、窗口/协议/退出码与 CLI 参数细节逐项保留为 boundary/partial residual。

- 当前批次：servercore live WebSocket、market depth/details/resolver 与 notification contracts/lifecycle 的 15 条映射完成逐条复核。Rust active poll provider、snapshot fallback、WS route fail-closed、market search port、serde optional fields、limit/time-window guards、calendar notification projection 与 typed Result delivery owner 已收口为 reviewed；Go 的 heartbeat 双 provider 字段/sample freshness/staleReasons、nil backend、显式快照驱动 WS、OpenD 错误帧、security-details 首帧、零订阅需求计数、execution `{}`/firstNonEmpty helper、env override、nil adapter、calendar source/category、delivery status 与 sink panic 语义逐项保留为 partial/boundary residual。

- 当前批次：servercore product infrastructure/lifecycle、runtime integration 与 runtime observation 的 14 条映射完成逐条复核。Rust typed preview/fee stores、Option/Result nil 边界、request hash、单 broker cancel target、Tauri startup boundary、RuntimeComposition/ActiveProviderState、MCP settings 直连、capabilities projection 与 SQLite observation owner 已收口为 reviewed；Go 的 nil store 静默成功、三态 helper、nil ApplyFees、双 BrokerID 冲突、启动集成未持久化生效、包装 store、Futu reset/隐藏/等待时序、双路由 observation 与重启专门断言逐项保留为 partial/boundary residual。

- 当前批次：strategy runtime 启动、polling、worker 请求/错误与 security stream 的 6 条映射完成逐条复核。Rust provider catalog metadata gate、Pine worker capacity、RunScriptRequest 构造、worker diagnostics/recovery 与 web listener invalidate-all owner 已收口为 reviewed；Go 的启动补写 market metadata、实例级 worker limit、live request 字段快照、runtime error 状态、停滞后新 bar 推进下单/observation 与现有 SSE 直接取消断言逐项保留为 partial/boundary residual。

- 当前批次：strategy runtime trading 的 4 条映射完成逐条复核。Rust execution port 的最小交易单位拒绝/审计、current-bar 缓存过滤、账户快照 position/sellable 输入与断连 fail-closed owner 已收口为 reviewed；Go 的 ignored reason runtime evidence、空 intent 数组专门分支、K 线前主动刷新券商持仓以及断连时保留缓存继续下单逐项保留为 partial/boundary residual。

- 当前批次：`servercoretest/broker_new_test.go` 的 13 条 broker read/write、断连、参数校验与 JSON 路由映射完成逐条复核。Rust broker-read fail-closed、market-data unavailable、trade write error mapping、生产 HTTP invalid-query/body 矩阵、订单状态 guard 与 JSON route resolver owner 已收口为 reviewed；Go 的 200 degraded/disconnected wire、无租约 quote、OpenD 断连 unlock、未配置 broker 的下单/撤单、strconv 原文错误、合法断连 shape 与 broker 路由 content-type 逐项保留为 partial/boundary residual。

- 当前批次：servercore business 与 OpenAPI route registration 的 8 条映射完成逐条复核。Rust contracts/openapi 生成门禁、typed market listener、trade read ports、chrono/RFC3339、binding normalization、worker join、Tauri runtime config 与组合式 trade session owner 已收口为 reviewed；Go 的 OpenAPI 注册集合等式、nil Server 推送、bridge 直通层、httpTime/DefaultPine 专项、宽松参数矩阵、双 runner 错误聚合、sidecar runtime-config.js HEAD/authRequired 与可空 brokerExecutionExchange 工厂逐项保留为 partial residual。

- 当前批次：server application lifecycle、backtest route 与 bootstrap/degraded runtime 的 10 条映射完成逐条复核。Rust 有序关闭/逆序回滚、回测 market/code 与 session/time range 解析、Pine initial_capital metadata、degraded startup、不可空 web access、typed Option defaults、unavailable broker projection 与 execution-port audit owner 已收口为 reviewed；Go 的 syncer 与 lease 顺序、完整 backtest 落库字段、缺省 initialBalance 回填、nil Server setters、ErrFutuIntegrationNotEnabled、AvailabilitySnapshot 结构及 bootstrap bridge 装配步骤逐项保留为 partial/boundary residual。

- 当前批次：server lifecycle/calendar/options、market-data settings/security 与 strategy runtime dependency 的 12 条映射完成逐条复核。Rust Pine instance binding/compile requirements、calendar unavailable fail-closed、system/settings ports、demand book、provider activation rollback、web auth/session invalidation、不可空 strategy store、optional exchange 与 streaming lease gate owner 已收口为 reviewed；Go 的完整 compiled plan params/迁移、空 registry accepted=false、文件重开 provider 持久化、helper 不可激活启动、匿名浏览器 403 wire、web 密码 hash/login 闭环、保存触发会话失效接线、nil store no-op、CurrentExchange accessor 与 yfinance poll-only Start 专项逐项保留为 partial/boundary residual。

- 当前批次：server runtime side effects、入口边界与 backtest warmup 的 8 条映射完成逐条复核。Rust execution risk coordinator、settings normalization、typed data-management store、engine/Tauri entry、同步日历操作、ActiveProviderState close fence 与 production backtest warmup owner 已收口为 reviewed；Go 的三项 risk option 委托、seen_fill_retention_days/Pine 禁用运行时接线、nil manager、CLI shouldStart、只读 wrapper、请求取消隔离、Ensure 复活否定、HTTP envelope/异步撮合聚合逐项保留为 partial/boundary residual。

- 当前批次：strategy live/nil/workflow replay、strategy state reconciliation、trading cancellation、watchlist runtime 与 WS event 的 13 条映射完成逐条复核。Rust intent execution、typed nil boundary、workflow checkpoint/queue、启动恢复策略、cancel idempotency fence、watchlist capability fail-closed 与执行/风控 WS projection owner 已收口为 reviewed；Go 的 live semantics 专项、空 manager tick、端到端 replay、stale workflow provider failure、RUNNING→STOPPED 启动策略、三类额外 cancel 错误、broker failure 保留原状态、Futu probe detail、nil broker/server、bbgo notification、行情陈旧 stale 判定、observedAt/source wire 字段逐项保留为 partial/boundary residual。

- 当前批次：servercoretest backtest provider/runs 与 broker read/routes 的 6 条映射完成逐条复核。Rust fixture OpenD/helper workflow、provider switch/fallback、run persistence/orphan recovery、broker projection fixtures 与 fail-closed trade client owner 已收口为 reviewed；Go 的真实 provider live 矩阵、recovered 错误文案/路由级重启、九个 broker read 端点串联与调用计数、200 degraded 资金 wire、production HTTP descriptor/assembly wiring 逐项保留为 boundary/partial residual。

- 当前批次：servercoretest contract、execution routes 与 frontend 的 10 条映射完成逐条复核。Rust broker runtime projection、execution place/cancel persistence、scope/environment read filters、reconciliation worker、Tauri/API SPA fallback、构建期前端资源与 ProductHandle shutdown owner 已收口为 reviewed；Go 的 production HTTP route/assembly、完整订单列表/事件/调用计数链、REAL/SIMULATE 过滤与 settings 默认环境、同步状态端到端、Vite 根代理、可空 frontend FS、run loop cancellation 与 interfaces.apiBind 在 web disabled 时消费逐项保留为 partial/boundary residual。

- 当前批次：servercoretest installers、market/plugin/runtime defaults、server public/definitions、settings、strategy logs/sync、system/watchlist 的 17 条映射完成逐条复核。Rust degraded/open-store policy、market catalog、plugin routes、Product/Tauri profiles、typed server ownership、strategy definition/version stores、provider health projection、settings-file defaults、runtime logs/audit read groups、definition sync、fixed API port 与 startup fail-closed watchlist owner 已收口为 reviewed；Go 的 ADK 数据库降级 HTTP、CN 硬拒绝、requiresRebuild=true 五步链、GUIBind/可执行目录、APIBaseURL helper、strategy 因果链分页/版本 isCurrent、Futu 中性 envelope、首启 interfaces 文档、SetAPIPort 重绑定与 DATABASE_INCOMPATIBLE 请求级 503 逐项保留为 partial/boundary residual。

- 当前批次：stores handle、strategyapp runtime ports 与 tradingapp ExecutionGateway lifecycle 的 10 条映射完成逐条复核。Rust startup rollback/cleanup、account read ports、market capability/health ports、trade write routes、execution order store/preview identity 与 combo intent validation owner 已收口为 reviewed；Go 的 generic Handle callback seam、missing broker nil、streamingCandles descriptor/正向 health、无服务防御错误、ExecutionGateway 聚合分支、缺 store/非法 brokerOrderId、组合网关文案/通知/未知标识及 app helper 逐项保留为 partial/boundary residual。

- 当前批次：tradingapp notification 与 order update source 的 9 条映射完成逐条复核。Rust notification projection/state labels/dedup、order lifecycle read model、reconciliation account scope、session-ready discovery、fee aggregation 与 failure-closed owner 已收口为 reviewed；Go 的下单标题/消息模板、状态级通知与未知过滤、空白片段组合、symbol/side/qty/brokerOrderId 标识、Activate→Discover 两段式、Futu 账户回调过滤、多券商扇出/费用失败语义及基金账户裁剪/非活跃错误逐项保留为 partial residual。

- 当前批次：tradingapp order_updates 的 6 条映射完成逐条复核。Rust reconciliation port、订单/成交/费用持久化与重启幂等、scope 解析、typed ledger、OpenD unavailable fail-closed owner 已收口为 reviewed；Go 的 worker 构造、订单/成交双向字段保真、brokerOrderQuery 逐字段 trim、nil ledger no-op、inactive source 错误/空订阅 shape、应用器重复推送去重与赔付写回逐项保留为 boundary/partial residual。

- 当前批次：webaccess auth boundaries、frontend 与 security integration 的 14 条映射完成逐条复核。Rust origin/referer production semantics、typed auth/session states、login write-port errors、session limits/pruning、static AssetBundle/Vite proxy boundary、loopback/public bind policy、internal proxy bearer、session invalidation 与 desktop token owner 已收口为 reviewed；Go 的 nil receiver、过期 session false 快照、login 熵失败/取消/桌面受信逐项路由、eviction/pruning 直接断言、runtime-config.js 动态生成、远端 X-Forwarded-Proto/403 文案、password-change HTTP trigger 与桌面 200/浏览器 403 对照逐项保留为 partial/boundary residual。

- 当前批次：internal/backtest business/recovery/result-view 的 8 条映射完成逐条复核。Rust snapshot/read fail-closed、coverage/readiness normalization、failed terminal persistence、worker handle recovery、warmup/chart trimming 与 result view seed/metadata owner 已收口为 reviewed；Go 的 nil store 安全降级、辅助函数边界、nil runner 与 panic payload 分类、warnings+chart 组合、损坏 K 线成交量守恒、坏请求后的空运行 wire 形状及 latest diagnostics 选择逐项保留为 partial residual。

- 当前批次：backtest failure/recovery 与 Pine worker service 的 5 条映射完成逐条复核。Rust queue/worker fail-closed、definition/symbol validation、run persistence/restart、execution port injection 与 unavailable 503 owner 已收口为 reviewed；Go 的 queue persistence failure 后 Close 不泄漏、lower-timeframe warmup 拒绝矩阵、running 终态保持注入失败、默认 runner 错误字段/文本与 PineTS 配置透传逐项保留为 partial residual。

- 当前批次：pkg/backtest cost/account failure、runmodel result 与 storage codec/aggregation 的 17 条映射完成逐条复核。Rust fee engine、virtual-fill guard、typed storage errors、warnings/dedup、result snapshot、damaged schema fail-closed、sync revision CAS、canonical interval/table naming 与 connection Drop owner 已收口为 reviewed；Go 的 preset intent/empty fallback、equity point suppression、字符串错误分类、runtime error samples/counts、warning cap/grouping、损坏日线逐分支、nil receiver/immutable alias、fixed-decimal raw bytes、短行分类、rehab 消毒回退、空/legacy 库和句柄计数逐项保留为 partial/boundary residual。

- 当前批次：backtest storage codec/query failure 的 4 条映射完成逐条复核。Rust Decimal 文本存取、冻结金标准、fail-closed coverage/source errors 与 empty scoped tables owner 已收口为 reviewed；Go 的定点字符串往返/上游兜底解析、多标的/通道查询错误传播及前向/后向双向 API 逐项保留为 boundary/partial residual。

- 当前批次：backtest storage business aggregation、connection model 与 failure boundaries 的 15 条映射完成逐条复核。Rust transactional upsert/query pagination、coverage fail-closed、calendar aggregation、extended-session source selection、WriterLease/single-connection、schema recovery 与 typed ownership owner 已收口为 reviewed；Go 的 upsert 专项、open-window Verify、前后向 API、错误文案、无标签行、8 连接池/WAL 并发、queued write、closed-handle 错误矩阵、legacy 列形状、session scope fallback 与 stream 空输入逐项保留为 partial/boundary residual。
