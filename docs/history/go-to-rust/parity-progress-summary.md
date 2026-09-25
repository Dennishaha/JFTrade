# Go → Rust 对齐成果摘要

更新时间：2026-09-25。本文是迁移期工作摘要，不替代架构事实、门禁结果或发布资格。

## 当前基线

| 项目 | 数值 | 解释 |
| --- | ---: | --- |
| Go 测试候选 | 4451 | 冻结基线 `go:452dea11` |
| Rust 测试 | 3338 | 数量不代表行为等价 |
| `function_exact` | 1481 | 有真实且唯一的 Rust 测试证据 |
| `partial` | 2336 | 只覆盖部分断言，不能视为完成 |
| `boundary` | 636 | 当前架构边界或没有同形对象 |
| 重复映射 | 0 | 审计脚本结果 |
| Parity 锚点 | 1764 / 1718 / 0 / 0 / 46 | unique / recorded / unrecorded / stale / unknown |

## 已完成的工作

- 建立了 4451 条 Go 测试逐项映射清单，按文件、行号、测试名记录，避免只按同名测试判断。
- `strategy_pine` 的 385 条 `partial` 已逐条复核完；结论仍是 `partial`，没有把聚合测试冒充 exact。
- `api_transport` P1 的 72 条 `partial` 已逐条复核完；结论仍是 `partial`。
- `internal/marketdata` partial 第 1–30 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`，精准 Rust 证据 38/38 通过，没有新增生产修复。
- `internal/marketdata` partial 第 31–60 条已逐项读取 Go 断言并核对 Rust 证据；30/30 保持 `partial`/`boundary`，并新增一个先红后修的 resolver 回归测试，锁定 provider full-window 后再应用公开 limit。
- `internal/marketdata` façade 剩余 34 条已逐项读取 Go 断言并核对 Rust owner；34/34 保持 `partial`，本批受影响 crate nextest 1993/1993 通过。helper provider 的 `snapshot-poll-delayed`/`snapshot-poll-fallback` 状态已由生产 owner 回归测试锁定。
- `subscriptions_test.go` 4 条与 `quote_availability_test.go` 2 条已逐项读取 Go 断言并核对 Rust owner；6/6 保持 `partial`，新增 authoritative quote 缺失字段与 legacy zero projection 回归测试，精准 nextest 11/11 通过。Rust 没有 LiveTickJSON/LatestTicksJSON 与 Go `SnapshotJSON` 同形 helper，因此未升级为 `function_exact`。
- `lifecycle_boundaries_test.go` 余量 7 条已逐项读取 Go 断言并核对 Rust owner；7/7 保持 `partial`，精准 nextest 16/16 通过，没有新增生产修复。Cache、subscription、service、collector 与 resolver 的聚合 fixture 差异继续按 owner/架构边界记录。
- Assistant chat stream helper/recovery 与断线重连 5 条已逐项读取 Go 断言并核对 Rust/SSE owner；5/5 保持 `boundary`/`partial`，精准 nextest 5/5 与 SSE/重连证据 4/4 通过。内存 hub clone、toolGroup、失败 writer 单写次数等旧 owner 行为按边界保留。
- `internal/api/backtest/routes_progress_test.go:71` 已逐项读取 Go 断言并核对 Rust sync owner；保持 `partial`，精准 nextest 2/2 通过。Rust 已覆盖持久化 task 读回与 cancel 成功/缺失语义，HTTP progress 双态仍缺同形冻结路由语料。
- HTTP bindings 的 `ParseQueryTime` fallback、`BindURI` escape/binding 和 candle period/pagination 三条 P1 已逐项核对；3/3 保持 `partial`，精准 nextest 7/7 通过。UTC/URI/candle 核心规则已有 Rust owner，caller fallback、Gin binding seam 与纯分页 helper 仍保留差异；本批复核了带空白 period、blank limit、非整数 limit 和 route 默认窗口语义，负 limit/offset 的 Go helper 钳制在 Rust typed query 边界不可达。
- `internal/api/live/dispatcher_boundaries_test.go:205:TestDispatcherEnvelopeDefaultsAndMapFallback` 已逐项核对；保持 `partial`，Rust SSE frame 精准回归 1/1 通过。dispatcher envelope 缺省字段与 mapString fallback 是 Go helper seam，Rust 事件生产者直接生成完整 typed JSON，没有同形通用 helper，未把 frame 形状测试升级为 exact。
- `internal/api/marketdata/routes_test.go:147:TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 已逐项核对；保持 `partial`，Rust 显式 broker 证据 2/2 通过（`cargo-nextest` targeted）。Rust active-provider owner 与 Go 独立 broker reader 调用列表不同，按架构边界保留四路序列差异；下一 P1 为 `internal/api/settings/routes_market_data_test.go:63:TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure`。
- `internal/api/settings/routes_market_data_test.go:63:TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure` 已逐项核对；保持 `partial`，Rust engine/settings 证据 3/3 通过（`cargo-nextest` targeted）。provider catalog、prepare-before-persist 与旧值保持已有 owner 断言，HTTP 409 envelope/GET 回读仍是跨 owner 聚合差异；下一未收尾 P1 为 `internal/api/trading/execution_validation_contracts_test.go:238:TestExecutionOrderDetailsRouteMapsMissingAndStoreFailures`（system route validator 已在下方记录）。
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
- 本批门禁：`check:quick` 按清单/文档变更落到 policy lane，zero-go、架构/生产策略、AI context、脚本兼容性与 actionlint 全部通过；完整 `check:rust` workspace 3464/3464（2 skipped）与 7 个 compatibility replay 全部通过。映射审计更新为 Rust 3338、`function_exact` 1481、`partial` 2336、`boundary` 636；重复 exact 0、nonexistent crate 0，Parity 锚点 1764/1718/0/0/46。
- 当前批次：`internal/api/httpserver/bindings_boundaries_test.go:61` 的 caller fallback 逐项复核；Rust UTC/日期/空值归一精准回归 1/1 通过，非法时间仍由 market-data route fail closed 为 400，保持 `partial` 并记录为有意 owner 语义差异。
- 当前批次：`internal/api/httpserver/bindings_boundaries_test.go:70` 的 required path、合法 `%20` 与 malformed escape 三分支逐项复核；API route 与 query escape 精准回归 2/2 通过，Gin helper 缺失参数错误与 Rust 404 route 形状差异保持 `partial`。
- `00f89290`：marketdata façade 剩余 34 条逐项复核，补 helper provider polling mode owner 回归测试并修复状态投影。
- `47bf7b1a`：注入配置的默认交易市场并补交易读取测试。
- `917d1534`：API transport P1 partial 第 1–30 条核对。
- `952a8993`：API transport P1 partial 第 31–60 条核对。
- `7fd577c7`：API transport P1 partial 第 61–72 条核对并收尾。
- `15f3f2d1`：strategy/Pine partial 第 281–300 条核对并修复 Pine request/indicator 差异。

`917d1534`、`952a8993`、`7fd577c7` 主要是 parity 清单和批次文档，不应被解读为新增 Rust 功能已经完成；`15f3f2d1` 则同时包含 Pine 生产修复与回归测试。
