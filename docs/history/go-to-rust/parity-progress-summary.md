# Go → Rust 对齐成果摘要

更新时间：2026-09-25。本文是迁移期工作摘要，不替代架构事实、门禁结果或发布资格。

## 当前基线

| 项目 | 数值 | 解释 |
| --- | ---: | --- |
| Go 测试候选 | 4451 | 冻结基线 `go:452dea11` |
| Rust 测试 | 3325 | 数量不代表行为等价 |
| `function_exact` | 1473 | 有真实且唯一的 Rust 测试证据 |
| `partial` | 2343 | 只覆盖部分断言，不能视为完成 |
| `boundary` | 636 | 当前架构边界或没有同形对象 |
| 重复映射 | 0 | 审计脚本结果 |
| Parity 锚点 | 1752 / 1706 / 0 / 0 / 46 | unique / recorded / unrecorded / stale / unknown |

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
- HTTP bindings 的 `ParseQueryTime` fallback、`BindURI` escape/binding 和 candle period/pagination 三条 P1 已逐项核对；3/3 保持 `partial`，精准 nextest 5/5 通过。UTC/URI/candle 核心规则已有 Rust owner，caller fallback、Gin binding seam 与纯分页 helper 仍保留差异。
- `internal/api/live/dispatcher_boundaries_test.go:205:TestDispatcherEnvelopeDefaultsAndMapFallback` 已逐项核对；保持 `partial`，Rust SSE frame 证据 1/1 通过。dispatcher envelope 缺省字段与 mapString fallback 是 Go helper seam，未把 frame 形状聚合测试升级为 exact。
- `internal/api/marketdata/routes_test.go:147:TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 已逐项核对；保持 `partial`，Rust 显式 broker 证据 2/2 通过。Rust active-provider owner 与 Go 独立 broker reader 调用列表不同，按架构边界保留四路序列差异。
- `internal/api/settings/routes_market_data_test.go:63:TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure` 已逐项核对；保持 `partial`，Rust engine/settings 证据 3/3 通过。provider catalog、prepare-before-persist 与旧值保持已有 owner 断言，HTTP 409 envelope/GET 回读仍是跨 owner 聚合差异。
- `internal/api/system/routes_test.go:61:TestSystemManualRetryRouteCallsReset` 已逐项读取 Go 断言并补 Rust 专用回归；升级为 `function_exact`，精准 nextest 4/4 通过。Rust 明确断言 manual-retry 返回 accepted=true 且 SystemWritePort 的 ManualRetry operation 只调用一次。
- `internal/api/system/routes_test.go:77:TestExchangeCalendarRefreshRouteCallsRefresh` 已逐项读取 Go 断言并补无 market refresh 路由回归；保持 `partial`，精准 nextest 3/3 通过。Rust 通过真实 CalendarManager 断言 `/refresh/US` 与 `/refresh` 均 accepted/updated 并可由 status 读回，Go 注入 callback 的 market 参数与调用次数 seam 继续保留差异。
- `internal/api/system/routes_test.go:102:TestSystemRouteBoundaryValidatorsRejectMissingHardStopAndNonPositiveLimits` 已逐项读取 Go 断言并补 Rust 专用回归；保持 `partial`。Rust 直接断言 `maxOrderQuantity=0` 与 `maxOrderNotional=0` 在端口调用前返回 400/BAD_REQUEST；Go 缺 hard-stop ID 的 DELETE handler 返回 400，而 Rust 带 ID 的 POST release 模板无法匹配时返回 404/NOT_FOUND，作为路由形状边界保留。
- 本批精准回归 2/2 通过。`pnpm run check:quick` 完整通过；`pnpm run check:rust` 首次运行在既有 `adk_session_detail_omits_resolved_approval_groups` 异步时间线断言处失败（733/734 已通过），随后以 nextest `--retries 2` 单测复核通过。该失败与本批 system-write 变更无关，保留失败证据，不将完整门禁记为通过。
- `internal/api/system/routes_test.go:121:TestRealTradeReleaseRoutesRejectMalformedOptionalPayloadBeforeStateChange` 已逐项读取 Go 断言并补三路由 Rust 专用回归；保持 `partial`。hard-stop release、kill-switch release、risk disable 的畸形载荷均返回 400/BAD_REQUEST，且未触发 SystemWritePort；Gin registration 与 service callback owner 形状继续按边界记录。
- 本批精准回归 3/3、`check:quick` engine 1937/1937 与完整 `check:rust` workspace 3451/3451（2 skipped）均通过；7 个兼容回放全部通过。
- 本批门禁：受影响 `check:quick` 与完整 `check:rust` 均被既有 `product_api_launcher_lifecycle` sidecar 退出码 `None`/`Some(0)` 间歇性断言打断；两个失败用例随后以 nextest `--retries 2` 复核均通过。该 flaky 只涉及 launcher 生命周期，不涉及本批 system-write 变更。
- resolver limit 差异已在 `crates/jftrade-engine/src/product_production_ports_market_data_catalog_futu.rs` 修复：避免 provider 在 CN/SH/SZ 过滤前按公开 limit 截断候选；TTL/singleflight 仍保留为架构边界，不宣称等价。
- 近期真正修改过 Rust 生产代码的批次包括：交易默认市场注入、下单前名义金额回退、市日边界、策略运行时及若干行情/路由边界；这些改动均配有回归测试或兼容性证据。
- 最近的 strategy/API 批次主要是证据审查和文档落账，没有新增 Rust 生产代码，必须与“功能已完成”分开看待。

## 仍未闭环的部分

- `partial` 仍有 2343 条，说明 Go 断言与 Rust 证据尚未达到同层等价。
- `api_transport`、`backtest_calendar`、`assistant` 等领域仍有大量 partial；下一步应优先选择一个真实功能缺口，补 production owner、失败回归测试和实现，再更新清单。
- Watchlist/Futu remote reader、TickCandles/跨交易日 `VolumeDelta` 等历史上已识别为架构能力缺口，不能靠继续增加映射文档闭环。
- `boundary` 不是“通过”，而是明确记录当前 Rust 架构没有对应 Go helper、wrapper 或旧入口。

## 调度收敛规则

- 当前只保留一个持续队列上下文；本批完成后下一片按 P1 处理 `internal/api/system/routes_test.go:170:TestExchangeCalendarProbeRouteCallsProbe`，继续核对 probe 路由的 market 透传与 owner 调用语义。
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
- `00f89290`：marketdata façade 剩余 34 条逐项复核，补 helper provider polling mode owner 回归测试并修复状态投影。
- `47bf7b1a`：注入配置的默认交易市场并补交易读取测试。
- `917d1534`：API transport P1 partial 第 1–30 条核对。
- `952a8993`：API transport P1 partial 第 31–60 条核对。
- `7fd577c7`：API transport P1 partial 第 61–72 条核对并收尾。
- `15f3f2d1`：strategy/Pine partial 第 281–300 条核对并修复 Pine request/indicator 差异。

`917d1534`、`952a8993`、`7fd577c7` 主要是 parity 清单和批次文档，不应被解读为新增 Rust 功能已经完成；`15f3f2d1` 则同时包含 Pine 生产修复与回归测试。
