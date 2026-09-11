# 2026-09-09 全项目 Go → Rust 行为对齐审计与回归补强

## 结论与证据边界

**当前结论：F01、F02、F03、F04、F05 的越权撤回改动已按授权补回，并完成局部回归验证；它们不再按原确定性缺陷保留。** F01（外部订单发现）、F02（Futu extended 快照投影）、F03（Futu 筛选生产装配及大陆身份）、F04（日/周/月回测聚合）和 F05（Worker drawing 增量）均已接入对应生产调用链并有隔离测试；G01、G02、G03、G04、G05、G06、G07 也已有本轮针对生产装配、边界或故障注入的限定证据。G05 目前只验证了本地 synthetic 九库旧版本升级/回滚；真实旧发布包、四平台安装升级及 G08 的真实外部依赖仍未验证，因此不能宣布“全项目迁移完成”。没有发现已复现的 P0；不将“没有测试”当成“没有实现”，也不将历史 PASS 直接继承为当前结论。

- Go 基线：`452dea115ca75c51361e8876c2aefd7c009839b8`，本地 `go` 与已有 `origin/go` 一致；本轮未 fetch，不声称重新确认了远端最新状态。
- Rust 基线：`8b840d2f87a253a9e7f925b83d3273b25ba2dcf0`，当前 `main`。
- 用户随后明确授权补回此前撤回的越权更改；本次交付因此包含 F01–F05 的生产实现与回归测试，以及本审计报告和导航更新。三个用户既有未跟踪文件 `crates/jftrade-marketdata/tests/cache_extended_sessions_parity.rs`、`docs/history/go-to-rust/test-parity-report.md`、`scripts/compatibility/audit_test_parity.py` 保持原样。
- 环境：macOS 本机，Node `v26.8.1`，pnpm `12.3.4`；使用仓库锁定工具链与 nextest wrapper。
- 本轮未改变公开 HTTP/OpenAPI、SSE/WS、SQLite schema 或 worker wire contract；新增的 protobuf 读取仍由现有生成流程构建，`Cargo.lock` 仅记录已在 workspace 声明的 `jiff` 依赖。未访问用户数据库、真实账户、OpenD、行情源或模型服务，未启动发布流程。四平台安装、签名、升级/回滚仍是外部验收缺口。
- 本轮覆盖的是模块/产品能力级盘点、全部 HTTP 契约静态比较、现有全项目门禁和重点链路审查。**不是逐个重放全部 Go 测试，也不是所有 UI 操作端到端验收。** 能力矩阵中仍有“缺少验证”，因此不能据此宣布“全项目迁移完成”。

判定规则：

| 状态 | 本报告中的含义 |
| --- | --- |
| 已验证对齐 | 仅在表中明确限定的契约/场景范围内有代码比较及本轮检查证据，不外推到整个领域。 |
| 确认缺陷 | 实际函数运行复现，或生产调用链存在不依赖环境的确定性分支/投影错误；分别标明证据层级。 |
| 缺少验证 | 有能力入口或已有测试，但缺少生产链、历史行为差分或相应故障注入证据；不推定功能缺失。 |
| 有意差异 | 当前设计明确允许的改造，附设计依据。 |
| 不适用 | 非当前产品承诺，不要求复刻旧框架或上游全部实现。 |

## 1. 全项目能力矩阵

### 1.1 HTTP 有限契约面：完整枚举

通过 `git show go:tests/fixtures/openapi-baseline.json` 与当前 `contracts/openapi/openapi.json` 解析比较，对象键排序、数组保持原顺序。**278 个 method/path 无增删，全部 paths 和 definitions 深度相同。** 本轮 `check:contracts` 通过。下表所有组的静态契约均为“已验证对齐”；这不代表每个接口的生产行为已验证。

| 路由组 | 操作数 | 行为审计归属 |
| --- | ---: | --- |
| adk | 64 | A01–A05 |
| alerts | 4 | T05 |
| auth | 3 | X01 |
| backtests | 8 | B01–B04 |
| brokers | 16 | T01、T04 |
| execution | 10 | T02–T03 |
| market-data | 46 | M01–M05 |
| plugins | 5 | X04 |
| portfolio | 2 | T01 |
| research | 21 | R01–R03 |
| settings | 35 | X01–X03、B03 |
| strategies | 10 | S02–S03 |
| strategy-definitions | 9 | S01 |
| strategy-pine | 1 | S01、W01 |
| system | 26 | X01、D01–D03 |
| watchlist | 15 | L01–L02 |
| watchlists | 2 | L01 |
| ws | 1 | X05 |

### 1.2 行为与运行时

定位以模块表、Go `internal` / `pkg` 产品入口、当前 router/生产装配/同域测试为依据。下表中的链路名为源码中的 owner 或测试套件；确认问题详见第 2 节，补测范围详见第 3 节。

| ID / 能力 | 调用链、状态 owner 与本轮证据 | 状态及剩余边界 |
| --- | --- | --- |
| M01 Provider 选择、订阅代际和切换拒绝 | Provider 菜单 → provider/actions ports → `ActiveProviderState` / `ProviderRouter` / DemandBook；切换发布与 readiness 测试、provider replay 通过 | 已验证对齐：代际隔离、激活失败不冒充 ready；实际上游重连组合仍属 G08 |
| M02 Futu 快照、盘前/盘后语义 | `basic_quote_ticks_with_resolver` → TickCache → `ProductionMarketDataQuotePort::read_snapshots`；已补入扩展块、regular-close promotion、交易日/时区/session resolver，并以 resolver 贯穿 OpenD 生产装配 | 已验证对齐（隔离投影/cache 场景）：扩展字段和 session/trading date 不再被固定丢弃；`PreAfterMarketData` 没有独立 per-block `quoteTime`，跨页 SSE/WS 收敛仍属 G07，真实 OpenD 属 G08 |
| M03 历史 K 线、会话与分页 | QuotePort → helper/Futu typed reader → wire；`market_data_production_compatibility` 与 operation-readiness tests 通过 | 已验证对齐（已覆盖的 provider/分页/拒绝边界）：readiness 不再由共享 umbrella 能力冒充；尚未把所有上游组合外推为 live 结论 |
| M04 深度、衍生品、期权、预测市场 | broker capability → production typed reader → Futu integration；具体 reader readiness 与不支持分支测试通过 | 已验证对齐（能力目录与生产 reader 绑定）：每个已 advertised operation 需要具体 capability；真实外部权限和全部数据组合仍属 G08/后续补测 |
| M05 Python helper 双 Provider 与研究数据 | Rust HelperClient → 内部 HTTP → yfinance/AKShare；337 项 pytest 通过 | 已验证对齐：当前 fixture 下的双 Provider 行为；真实上游数据/权限属 G08 |
| T01 券商账户、持仓、资金和实时订单读取 | AccountPage → broker/portfolio ports → 独立 trade runtime | 已验证对齐：helper 行情不切断已配置 Futu 交易读；实际券商权限/数据属 G08 |
| T02 下单、拒绝、未知状态与身份对账 | execution port → risk coordinator → broker → execution store；身份、作用域、迟到回执测试通过 | 已验证对齐：UNKNOWN 不猜配/不自动重报等受测边界；不是实盘验收 |
| T03 券商外部订单发现与持久化 | reconciliation worker → `ProductionExecutionPort::reconcile_pending_orders` → account/environment/market scoped active+history orders/fills → execution store → AccountPage | 已验证对齐（隔离 TradeReadPort/store 场景）：补入空账本外单发现、fill-first 建账、身份/作用域隔离、幂等和重启扫描；TradeReadPort 仍没有显式分页参数，OpenD 默认页数和真实账户权限属待验证/G08 |
| T04 组合/保护单与策略模拟账户 | Pine intent → Rust atomic execution port；REAL/SIMULATE 共用券商执行 | 有意差异：不支持原子承诺时整组拒绝；SIMULATE 不降级离线成交，见运行中执行专题 |
| T05 通知、提醒和事件游标 | execution store → notification projector → native/LiveHub；通知过滤、幂等与 operation readiness 测试通过 | 已验证对齐（store/projector/readiness 场景）：跨页断连、重载与全部提醒类别的 UI 收敛仍是 G07 剩余边界 |
| B01 回测任务、取消与结果读取 | BacktestPage → ProductionBacktestPort → execution registry → runs store；取消/恢复测试及回放通过 | 已验证对齐：已有任务状态与冻结回放场景；不覆盖所有撮合组合 |
| B02 分钟聚合至日/周/月 | production backtest task → `BacktestMarketDataStore::read_candles`/query → calendar-period aggregation helpers | 已验证对齐（resolver-backed 临时 SQLite 场景）：日/周/月目标表缺失时按日历桶从低周期 fallback，目标表优先，extended 日/周/月优先 ≤1h 源，缺分钟 fail-closed；生产日历来源统一由 G01 证据覆盖 |
| B03 日历来源、人工覆盖与聚合 | Settings 日历写入 → `CalendarManager` → runtime session/backtest resolvers → quote/session/coverage/aggregation | 已验证对齐（G01）：聚合、session windows、trading date 与 coverage 共享同一 resolver；人工覆盖、US holiday、UTC 中午日期解析已有回归 |
| B04 DST、完整桶与预热 | store session aggregation、backtest warmup；resolver-backed cutoff、缺分钟、DST、人工覆盖通过 | 已验证对齐（G01 限定场景）：跨节假日长预热和真实上游日历权限仍不外推为 live 结论 |
| S01 策略定义、Pine 编辑/预检 | StrategyDesignPage → definition/Pine ports → stores/worker；契约与结构语料测试通过 | 已验证对齐：受测创建/校验与结构生成；复杂编辑器交互属 G07 |
| S02 live append、checkpoint、重开 | StrategyRuntimeManager → market data/Pine → execution port；恢复、拒绝历史重放测试通过 | 已验证对齐：受测 checkpoint/append 生命周期；真实进程故障组合仍需外部/桌面验收 |
| S03 停止、取消与写锁 | strategy owner cancel/join → store lease；stop/ownership 测试通过 | 已验证对齐：受测停止超时保留 owner；不可取消的真实外部阻塞仍按 G03 的安全语义单列 |
| W01 Worker 协议、订单意图、数值输出 | Node executor → adapter → gRPC → Rust integration；98 项 Worker 测试通过 | 已验证对齐：现有脚本与协议 fixture；真实编码阈值属 G04 |
| W02 live drawing 增量 | `resultMarker` structural snapshot → `incrementalResult` → visual outputs → gRPC/UI consumer | 已验证对齐（Worker 结构快照、G04 loopback 编码及 G07 LiveHub backpressure 控制场景）：支持追加、identified drawing 原地修改/定长替换及删除 tombstone，保留旧 marker 兼容；Worker→Rust reducer→Vue 图形合成与浏览器断连恢复仍缺跨层证据 |
| A01 Assistant 会话、流式运行和恢复 | ADKPage → chat/model runtime → session/run stores；持久身份、流和恢复测试通过 | 已验证对齐：受测运行身份、错误分类、恢复扫描；真实模型属 G08 |
| A02 审批、工具租约与副作用隔离 | approval → run continuation → invocation claim/fence | 已验证对齐：重复审批、过期接管和晚结果拒绝等测试；工具目录 readiness 已由 G06 具体 operation 检查，真实外部副作用仍按 provider 验收 |
| A03 Canvas、cron 与定时工作流 | WorkflowScheduler → durable trigger checkpoint → model/节点 → SQLite | 已验证对齐：已有 cron Go 语义、CAS、等待审批和恢复用例 |
| A04 会话删除与跨库恢复 | DeleteSession → artifact/session/adk 三库顺序删除 → process-local deletion fence | 已验证对齐（G02）：三库 cascade 的失败重试、stale writer 拒绝和活动写入 fence 已有临时库/挑战回归；真实旧安装恢复仍属 G05 |
| A05 MCP、provider、skills、tools 目录 | MCP schema/catalog → operation-specific readiness → production dispatch → domain ports | 已验证对齐（G06 限定场景）：research/tool 目录不再从共享 `ResearchRead` umbrella fallback；完整外部工具副作用仍按具体 provider 验收 |
| R01 Futu 股票筛选 | StockScreenerView → POST/GET screens → `ProductionResearchScreenHelperPort` → `SharedTradeReadRuntime` → typed `Qot_StockScreen` reader → `Qot_GetStaticInfo` identity resolver → projection | 已验证对齐（typed reader/production fixture）：402 因子、分页/限流、HK/US/SH/SZ identity 与 CN 合并 SH/SZ 已接入；公共 Futu catalog 当前只开放 HK/US/SH/SZ，CN catalog 明确拒绝是有意限制/待产品确认，不宣称 CN 全部完成；真实 OpenD 属 G08 |
| R02 helper 股票筛选、preset 版本 | screen helper port / ResearchPresetStore；fixture/协议测试通过 | 已验证对齐：现有 helper 子集与 preset 冲突语义；不替代 Futu 402 因子 |
| R03 公司/新闻/技术指标/日历/宏观 | ResearchPage → ProductionResearchPort → 分操作 reader/helper | 已验证对齐（G06）：rankings/industry/calendar/macro/analyst/ownership/corporate-actions 等 operation readiness 按具体 reader 判定；真实上游权限属 G08 |
| L01 自选分组、星标、成员、远端导入 | WatchlistPage → ProductionWatchlistPort / remote port → WatchlistStore | 已验证对齐：现有 mutation、重复/冲突与远端 fixture；新增批量 operation 按 G06 readiness 规则拒绝无 reader 的 provider |
| L02 自选批量行情、缓存与切换 | batch quotes → helper/trade reads → quote projection/cache | 已验证对齐（G06 capability/readiness）：批量入口按具体 helper/Futu reader 选择；其它数据源仍保留各自 regular 投影边界，不机械外推 Futu 语义 |
| X01 认证、设置写入和 Web 暴露 | AccessPolicy / AuthSessionManager / SettingsFileStore → runtime；静态认证与 session 测试通过 | 已验证对齐：受测认证、非法写入、持久化拒绝；浏览器完整流程属 G07 |
| X02 九库 WriterLease、schema/migration | production store assembly → 各库 leased connection；本地 synthetic 九库升级/回滚与 workspace/store replay 通过 | 已验证对齐（G05 本地场景）：9 库、3 条受支持迁移、sentinel 保留和全文件失败恢复均已覆盖；真实旧发布包、四平台安装升级仍属 G08 外部验收 |
| X03 清理预览、执行、backup/rebuild/compact | SettingsDataManagementSection → maintenance service → approved candidates/store owner | 已验证对齐：受测候选指纹和 busy 拒绝；跨库中断/真实旧数据属 G02/G05 |
| X04 插件安装/卸载与 artifact | plugin port → artifact/operation store；原子安装、缺 artifact 拒绝与 restart 测试通过 | 已验证对齐：已有隔离操作场景；真实发布制品属 G08 |
| X05 SSE/WS 与前端 server state | API stream/LiveHub → shared client/Vue Query → 页面；canonical production route 已由 HTTP→helper→HTTP 回归覆盖；LiveHub lagged control event → shared socket resubscribe → console state reload 已有 loopback 证据 | 已验证对齐（G07 代表性生产链）：传输/路由错误映射、provider response、断流/重连及 backpressure resync 控制均已受测；浏览器全流程与行情/图形跨层收敛仍未验收 |
| D01 Tauri IPC、profile 和窗口入口 | native facade → Tauri lifecycle → Rust product | 有意差异：Wails 改 Tauri；当前 facade/config replay 通过，原生行为全验属 G08 |
| D02 helper/Node 启停与崩溃恢复 | runtime supervisor / helper health / Pine readiness monitors | 已验证对齐：现有隔离 restart/ownership 场景；全部阻塞与系统退出属 G03 |
| D03 更新、安装、签名与 runtime 资产 | prepare/check 脚本 → release workflows；桌面脚本通过 | 缺少验证 G08：没有四平台签名安装/升级/回滚 receipt |
| Q01 门禁、affected 与兼容回放 | module map → run-test-layer / run-rust-checks → CI lanes；最新 workspace 1763 tests 与 7 类 compatibility replay 通过 | 已验证对齐（门禁执行层）：当前 quick 83 affected files 走 full preflight，`check:all` 仍 deferred；行为盲区见第 4 节，不用“零 Go”代替迁移证明 |
| Q02 旧 bbgo / Wails 实现细节 | Go `pkg/bbgo`、旧桌面 bindings/build 体系 | 不适用：当前仅承诺 `/api/v1/*`，不复刻上游全部 API 或旧框架生成物 |

有意差异依据：[当前运行中执行与所有权](../../architecture/runtime-execution-ownership.md)、[Pine 原子保护单契约](../../pinets-contract-audit.md)、[Tauri 桌面边界](../../troubleshooting/desktop-release.md)、根 AGENTS 的 API/唯一写入者约束。任何未找到设计依据的缺失均不按“有意差异”免责。

## 2. 修复后证据与剩余边界（按风险排序）

下列五项是本轮授权补回的实现。每项保留 Go/契约依据、生产调用链、触发条件、修复后的预期/实际、用户影响、测试遗漏原因和仍需补验的边界；“已验证对齐”只覆盖列出的隔离场景，不外推到 live 或跨层闭环。

| 修复前分类 | 本轮状态 | 仍需补验 |
| --- | --- | --- |
| 功能缺失：F01、F03 | 已修复并接入生产组合；F03 的公共 CN catalog 仍是有意限制 | OpenD 默认分页、真实权限、Futu 公共 CN 产品范围 |
| 实现错误：F02、F04、F05 | 已修复并有窄回归；G01 日历、G04 gRPC 边界和 G07 server-state reconnect 已有针对性证据，Worker/行情/UI 全链路断连恢复仍缺证据 | G07 的完整浏览器状态收敛与 G08 |
| 生产装配/前后端不一致 | F03 typed reader 已由 `SharedTradeReadRuntime` 注入；F02 projection/cache 已统一；G07 已有 canonical production route | 完整 UI 操作链、SSE/WS 断连重连与真实外部源仍待验收 |

本轮复核还补上了几条容易被同层替身掩盖的 fail-closed 边界：Futu 条件的
`operator` 现在必须属于该因子 catalog 声明的集合，非法 operator 在 provider
调用前返回 400；`Qot_StockScreen` 请求即使用户没有选择 `basic.code`，也会在
协议层隐式取回该字段，仅用于后续权威身份解析，不污染公开 cells。对账只为明确
的股票市场和正数券商账户建立 scope，active/history 任一订单或成交快照失败都会
整体保持可重试失败。日线来源合成周/月桶时要求覆盖桶内每一个交易日；TickCache
遇到墙上时钟回拨则分类为 stale 而不是把未来样本误判为 fresh。上述均有窄层回归，
不改变 HTTP、SQLite 或 worker 契约。

### F01 / P1：券商外部订单发现与持久化（已修复，局部验证）

- Go 依据：`internal/trading/order_updates.go:360` 的 `HandleOrderUpdate`/`ApplyOrder` 以 `BROKER_PUSH_DISCOVERED`、`broker.push` 建立未知外单，fill 分支先建账再落成交。
- Rust 调用链：reconciliation worker → `ProductionExecutionPort::reconcile_pending_orders` → `execution_reconciliation_discovery.rs` 的 account/environment/market scope → Futu `TradeReadPort` active/history orders 与 fills → `ExecutionOrderStore`；原有本地候选仍先处理，避免重复 claim。
- 触发与修复后行为：空本地账本、先到成交、重复 active/history 页或重启后历史单现在都会按精确 broker identity 建立 `broker-sync` 订单/事件，再按单调数量和加权均价落成交；scope 只接受明确股票市场（1/2/3）及正数数字账户身份，跨 account/environment/market 不会绑定，重复扫描不重复建账。active/history 任一侧订单或成交快照失败时整体 fail-closed，UNKNOWN/ambiguous 仍保持可重试。
- 用户影响/严重度：手工单、成交和费用可以进入统一执行账本，降低漏记和重复下单风险；原 P1 缺口已消除。真实账户权限和 OpenD 返回分页仍未验证，不把局部 fake 读端当 live 结论。
- 回归与漏测原因：新增空账本、fill-first、scope/identity、幂等和重启窗口用例，并将既有 reconciliation 计数断言更新为“本地候选 + discovery”语义；过去测试只预置本地订单，因而绕过 discovery owner。
- 本轮已补入 engine 隔离生产链回归：重复 active/history 快照去重、history 单侧失败后的重启重试，以及 bounded shutdown 后迟到订单/成交的完整一次落账；Futu `TradeReadPort`/OpenD 协议没有 cursor/page 字段，因此不擅自增加分页参数。仍需在真实 OpenD 权限和默认返回窗口下验收未知身份、存储失败和上游截断；每例继续使用临时 execution DB，不连接真实账户/OpenD。

### F02 / P1：Futu extended 快照投影（已修复，局部验证）

- Go 依据：`internal/marketdata/cache_test.go:103/138/185` 约束跨交易日继承、after-hours regular-close promotion 和同价 extended 更新保留。
- Rust 调用链：Futu `basic_quote_ticks_with_resolver` → TickCache context merge → `ProductionMarketDataQuotePort::read_snapshots`/`product_production_ports_market_data_quote_snapshot.rs` → HTTP projection；OpenD 生产启动把同一 `RuntimeCalendarSessionResolver` 注入 quote tick 与 task。
- 触发与修复后行为：regular→after→overnight→next-day、DST/半日市和同价更新现在保留 extended block、trading date、session、timezone、previous/last close；HTTP 不再固定输出 `extended=null`/`session=regular`。TickCache 还明确拒绝墙上时钟回拨造成的负 age，将该样本标为 stale。`PreAfterMarketData` 没有独立 per-block `quoteTime`，因此不伪造该字段。
- 用户影响/严重度：盘前盘后看盘、策略读取和收盘价语义回到 Go 约束的范围；原 P1 投影缺口已消除。跨页面 SSE/WS 收敛及真实 OpenD 时钟仍属 G07/G08。
- 回归与漏测原因：新增 resolver、cache inheritance、regular-close promotion 和 production projection 用例；此前 protobuf/cache happy path 没有用同一输入穿过 HTTP 投影。
- 仍需补强（不构成已复现缺陷）：将上述 regular/extended/DST/半日市序列继续穿过 SSE/WS 和 UI 断连重连，断言状态收敛；使用 fake provider 和临时 cache，清理所有连接，不接 OpenD。resolver 与 production API 的局部链路已由 G01/G07 窄测覆盖。

### F03 / P1：Futu 股票筛选生产装配与大陆身份（已修复，局部验证）

- Go 依据：`pkg/futu/adapter_advanced.go` 将 stock_v1/stock_v2 映射到 `Qot_StockFilter`/`Qot_StockScreen`；`stock_screen_normalization.go:264` 对 SH/SZ/CN 用 `Qot_GetStaticInfo` 做权威身份解析。
- Rust 调用链：StockScreenerView → POST/GET screens → `ProductionResearchScreenHelperPort` → `SharedTradeReadRuntime` → typed `Qot_StockScreen` reader（402 因子、分页、限流）→ `Qot_GetStaticInfo` → provider-neutral projection；runtime/provider activation 均安装 reader。
- 触发与修复后行为：Futu 选择合法 factor/page 会进入 OpenD reader；条件 operator 先按 catalog 逐因子校验，非法 operator 在 provider 调用前拒绝；请求缺 `basic.code` 时协议层隐式取回它供身份解析。SH/SZ 逐行按 static-info 解析，CN 查询合并 SH/SZ 并省略不可证明的精确 total，HK/US 返回 total。生产 fixture 已覆盖非法因子/operator、分页、market mismatch、rate-limit 和缺 `basic.code`。
- 用户影响/严重度：Futu 股票筛选不再在 composition 的 helper-only capability 分支提前失败；身份和分页错误会显式拒绝，而不是显示错误 symbol。公共 Futu catalog 目前只开放 HK/US/SH/SZ，CN catalog 明确拒绝是有意限制/待产品确认，不宣称公共 CN 全部完成。
- 回归与漏测原因：新增 typed reader/protobuf、production port 和 factor catalog 测试；此前 Web mock 与 transport FixturePort 都绕过唯一 production composition。
- 仍需补强（不构成已复现缺陷）：继续用 fake OpenD 回放更多 402 因子、每页 static-info、分页边界和限流，并扩展 canonical production route 到浏览器结果展示；测试结束关闭 fake server、删除临时数据，不接真实 OpenD。typed reader、readiness 和代表性 HTTP route 已有 G06/G07 证据。

### F04 / P1：日/周/月回测日历聚合（已修复，局部验证）

- Go 依据：`pkg/backtest/internal/storage/store_aggregate.go` 在目标表缺失或为空时从低周期合成 daily/trading-period bar。
- Rust 调用链：backtest task → `BacktestMarketDataStore::read_candles`/query → `backtest_market_data_aggregation::{period_interval, aggregate_period_range}`；目标表优先，缺失时按 1d/12h…1m（extended 日线优先 ≤1h）源回退。
- 触发与修复后行为：只有 1m/5m/10m/60m 等低周期数据时，`1d/1w/1mo` 现在按市场 session 和日历边界合成 OHLCV；完整桶、DST/早收盘约束和缺分钟 fail-closed 已覆盖，目标周期直接表仍优先。以日线作为周/月来源时，桶内任一应开市交易日缺行都会返回 coverage error，不会生成看似完整的部分桶。
- 用户影响/严重度：已有低周期数据可用于长周期回测，避免错误“无数据”；原 P1 缺口已消除。CalendarManager 现在通过 runtime session/backtest resolver 贯穿聚合、coverage 和 session 消费者；真实上游日历权限与超长历史仍不外推为 live 结论。
- 回归与漏测原因：新增临时 SQLite 的日/周/月、10m fallback、extended source priority、direct target priority 和 gap fail-closed 用例；此前只测分钟聚合，未构造目标表缺失场景。
- 本轮已补入跨年周（含元旦闭市）、跨午夜 extended→trading-date 归属、cutoff 不输出未闭桶和重复读取稳定性回归；仍需补强（不构成已复现缺陷）：在已通过的临时 DB 场景之外增加超长预热组合。人工覆盖、US holiday、DST 与统一 resolver 已由 G01 回归覆盖；测试使用临时 DB，结束后由 fixture 清理。

### F05 / P2：Worker live drawing 增量（已修复，局部验证）

- 契约依据：每 bar 的 visual delta 必须合并为完整结果；固定长度 drawing 对象的修改/替换/删除不能只用数组长度判断。
- Worker 调用链：PineTS executor → `resultMarker` structural snapshot → `incrementalResult`（`workers/pineworker/src/pinetsResult.ts`）→ visual outputs → gRPC/Rust reducer/UI。
- 触发与修复后行为：marker 记录带 `id/name` 的稳定结构快照，增量支持追加、原地修改、定长替换和删除 tombstone；匿名 drawing 保留索引语义，并兼容旧 marker 的 append-only fallback。
- 用户影响/严重度：实时图形不会因对象原地变更而陈旧；原 P2 缺口已消除。LiveHub lagged/resync 控制已有 G07 证据，但真实 Worker→Rust reducer→Vue 图形合成和浏览器断连恢复仍未验收；消息 limit 原子性已有 G04 证据。
- 回归与漏测原因：新增嵌套/多字节 payload、原地修改、定长替换、删除和 legacy marker 测试；此前只断言追加数量，未逐 bar 比较结构结果。
- 本轮已补入 `consoleDataConsoleStream` 的 loopback `MockWebSocket` 回归：首次连接、断开→重连以及 `live.resync` 都会重新拉取/重发 server-owned state，避免 Rust WS 不回放断开期间 `console.refresh` 时造成跨页 stale state；Rust capacity=1 lag 回归验证控制事件、保留事件和后续事件顺序。仍需补强（不构成已复现缺陷）：在已通过的 loopback tonic limit−1/limit/limit+1 边界之外，继续验证 Worker→Rust reducer→图形 UI 合成、行情 regular/extended/DST 序列与 Vue 浏览器级刷新/断连/重连收敛；清理 fake worker/server/临时会话。消息编码原子性与结构化 decode error 已由 G04 窄测覆盖。

## 3. 验证状态与剩余补强方案

G01、G02、G03、G04、G05、G06、G07 已有本轮可复核的窄层或代表性生产链证据，以下“已验证对齐”只对列明场景负责；剩余补强不是新确认缺陷。G08 及 G05 的真实旧发布包/安装升级仍保持“缺少验证”，不可用局部绿灯替代外部验收。

| ID / 优先级 / 状态 | 本轮证据与仍有边界 | 后续补强方案 / 验收 |
| --- | --- | --- |
| G01 / 高 / **已验证对齐（resolver 场景）** | `CalendarScheduleResolver` 已接入 backtest store、session windows、trading date、coverage；运行时 quote/task 由同一 `CalendarManager` 驱动。人工覆盖、US holiday、UTC 中午日期解析、跨年周（含元旦闭市）、跨午夜 extended→trading-date、cutoff 空结果、重复读取稳定性和 9 项日历聚合回归通过。 | 继续增加超长 warmup 组合；仍不连接在线 calendar，失败按明确策略 fail-closed。 |
| G02 / 高 / **已验证对齐（隔离三库）** | cascade 前对三个 ADK 库建立 deletion fence；session/context/handoff/composer/run/approval/task/event/artifact 写路径检查 fence。失败可重试、stale writer 不能复活删除会话；challenge suite 112 passed。 | 仅在独立临时目录继续做进程级 crash/restart receipt；不得操作用户数据库，也不要求跨库伪原子提交。 |
| G03 / 高 / **已验证对齐（bounded shutdown + late-owner fence）** | reconciliation 单次同步扫描移到 `spawn_blocking`；`ExecutionOrderStore` 的 process-local scan fence 覆盖整段 broker 读取、SQLite 投影和通知投影；worker 的 `accept_results` 代际标记在 `shutdown`/`terminate` 前置为 false，迟到 blocking 结果仍可耐久落账，但不会发布通知、更新 retired worker 状态或启动下一轮。测试覆盖 250ms timeout、替换 worker 在旧 owner 释放前不得进入、释放后顺序扫描、迟到成交/事件幂等以及 terminate late-result；engine 对账相关 62/62、package 全量 1032/1032 通过。 | 外部同步 I/O 不可强制取消是有意安全语义；真实 OpenD/子进程退出仍属 G08，不以隔离 fake 代替 live 验收。 |
| G04 / 高 / **已验证对齐（loopback tonic）** | request/response encoded length 覆盖 limit−1/limit/limit+1，多字节 drawing/label/payload 与 order intents 保持原子；limit−1 返回结构化 transport decode-size error。 | 继续把真实 Worker backpressure、Rust reducer 和 Vue 断连恢复串起来；不引入外网或真实 worker。 |
| G05 / 高 / **已验证对齐（本地 synthetic 场景）** | 真实 `initialize_production_databases` 在临时目录覆盖 9 个 descriptor；backtest `v2→v3`、strategy `v1→v2`、ADK `v2→v4`，逐库 sentinel、版本/schema 校验、迁移备份和失败时 main/WAL/SHM/journal 字节恢复均通过。 | 真实 Go 旧发布包/脱敏用户副本、跨进程 crash receipt 和四平台安装升级尚未执行；若无获授权基线保持未验证，不能把 synthetic fixture 当发布验收。 |
| G06 / 中 / **已验证对齐（operation readiness）** | ADK/MCP research operation 按具体 operation 判定 readiness；Futu derivatives/options/alerts/remote-watchlist/stock-screen 要求具体 reader；helper 与 Futu provider readiness 分离。3 项 catalog/readiness 回归通过。 | 对新增 advertised operation 继续做 fixture 级正常/非法/不支持/上游失败断言；不以目录数量或测试数量推定迁移率。 |
| G07 / 中 / **已验证对齐（canonical production route + resync）** | HTTP → production route registry → `ProductionResearchScreenHelperPort` → `HelperClient` → loopback fake helper → HTTP 已贯通；成功响应及第二次 429 的 `RESEARCH_SCREEN_RATE_LIMITED`、原始消息和调用次数均断言。Rust `LiveHubConnection::recv` 在容量 1 的真实 `broadcast::Lagged` 边界发出 `live.resync`；Web `sharedLiveSocket` 收到该控制事件后强制重发完整订阅快照，`consoleDataConsoleStream` 绕过冷却重载 server-owned state；`MockWebSocket` 覆盖首次连接、断开→重连和 backpressure resync，jftrade-api 16/16、Web 366 files/2435 tests 通过。 | 扩展到浏览器全流程、SSE/WS 行情 regular/extended/DST 序列、Worker reducer/backpressure、刷新和完整跨页 UI 收敛；当前未执行真实跨服务 UI E2E。 |
| G08 / 外部验收 / **缺少验证** | live OpenD、真实账户/行情/模型和四平台签名安装/升级/回滚均未执行；Rust live OpenD 与 Pine real-worker 各 1 项按门禁设计跳过。 | 只在显式授权 live workflow 或发布资格流程执行，固定 SHA/版本/权限并留存 receipt；跳过或不可用不得记为通过。 |

## 4. 为什么问题留到手工测试才发现

1. **接口相同不等于实现相同。** 278 个 method/path 全相同，但 api-transport replay 只发 19 个具体 probe；该回放自己的输出明确区分两者。
2. **替身刚好绕过了迁移的断点。** F03 的 Web mock 和 Rust FixturePort 曾分别证明 UI 与 transport，却绕过唯一 production composition；本轮已把 typed `Qot_StockScreen` reader 接入 `SharedTradeReadRuntime`，但仍需用 production API + fake 外部源的 canonical route 证明装配/ready、分页和 UI 展示，不能用同层 fixture 的成功断言替代。
3. **错误的完成度指标。** 计划阶段统计脚本以 crate basename 与带 `crates/` 前缀的值比较，领域归类错误；“待迁移样本”没有检查 Rust 对应行为。即便修正归类，测试数量比仍不是业务迁移率。一条参数化测试与多条函数不可直接相除。
4. **测试标题大于实际证据。** “crash recovery”可能只是手动删行后重试；“desktop compatibility”是 config/facade 回放，不是安装；“trading compatibility”实际 zero dispatches。报告必须记真实入口、输入和断言。
5. **历史关闭项没有覆盖新增回归。** 旧报告已明确列出未闭环项；本轮补回 F01–F05 并完成 G01/G02/G03/G04/G06/G07 的限定验证后，G05/G08 及浏览器跨层边界仍需按生产链和外部验收维护，不能因为局部绿灯把 roadmap 改成无条件完成声明。
6. **覆盖率指标遮不住语义遗漏。** Web 本次 statements 98.02%、lines 98.52%，仍不能证明 Futu 生产筛选的路由 ready、静态身份解析或跨层 drawing 合成；Go/Rust 行为映射、生产副作用与跨层状态才是补强重点。

## 5. 实际执行记录

原始输出保存在本机 `/tmp/jftrade-parity-audit.007yga/`；临时日志不承诺永久保留，本节保留审计必要结果。没有把被前置失败阻断的阶段算通过。

| 检查 | 本轮结果 |
| --- | --- |
| 工作树与 SHA | Go `452dea115ca75c51361e8876c2aefd7c009839b8`、Rust `8b840d2f87a253a9e7f925b83d3273b25ba2dcf0`；当前工作树包含 F01–F05 生产实现/回归测试、报告/导航更新，以及 3 个用户未跟踪文件。用户文件保留且未编辑，tracked 生产差异按授权保留。 |
| 直接比较 Go/current OpenAPI | 278 operations，paths/definitions 深度相同 |
| `cargo fmt --all -- --check` | exit 0 |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --test backtest_market_data_session_dst_aggregation --test adk_crash_recovery_orphan_injection_challenge --locked` | exit 0，12 passed，0 skipped |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --test backtest_market_data_calendar_aggregation --locked` | exit 0，11 passed，0 skipped；覆盖跨年周、元旦闭市、跨午夜 trading-date、cutoff 空结果、重复读取稳定性和日历覆盖 |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-store-sqlite --test backtest_market_data_aggregation --test backtest_market_data_session_dst_aggregation --test backtest_market_data_session_scope --locked` | exit 0，17 passed，0 skipped |
| `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked` | exit 0，1032 passed，0 skipped（含 G03 timeout/terminate/late-owner fence 回归） |
| `pnpm --filter @jftrade/pineworker run test pinetsExecutor.test.ts` | exit 0；脚本实际展开为 `vitest run src pinetsExecutor.test.ts`，执行 9 files / 92 tests；不将文件名参数误报成单文件证据 |
| `pnpm --filter @jftrade/web run test consoleDataConsoleStream.test.ts --run` | exit 0，1 file / 2 tests passed；覆盖首次 WS 连接及断开→重连后的 server-state reconcile |
| `node scripts/quality/check-workspace-architecture.mjs` | 最终 exit 0，Rust workspace architecture policy passed；product 文件 587/723/800 行均在 800 行上限内 |
| `pnpm run check:policy` | 最终 exit 0，含 AI context、policy scripts、actionlint 与架构/生产策略门禁；拆分前超限失败已修复 |
| `pnpm run check:contracts` | exit 0，包含只读 check:generated |
| `pnpm run clean:rust:artifacts` | 在确认无 Cargo/nextest 进程后清理可再生 `target` 中间产物；此前一次 quick 因超过 50,000 个 `.rcgu.o` 被 target-health 阻断，清理后重跑解除该环境阻塞 |
| `pnpm run check:rust` / quick Rust lane | exit 0（target health passed）：workspace nextest 1763 passed、2 skipped；7 类 compatibility replay 全部通过（storage 2 tables/3 K-lines、backtest 5 cases/8 fills、provider-runtime 14+9+3+3、trading-strategy 10+7+6+7+5+3、assistant 9+12+3+2+3+2+3、api 278 operations/19 probes、desktop 3 profiles/10 commands/4 events） |
| `pnpm run check:web` | exit 0；366 files / 2435 tests，coverage 98.02% statements、90.65% branches、98.52% lines，typecheck 通过；shared WebSocket 去重、backpressure resync 及 `consoleDataConsoleStream` 重连 reconcile 的 diff statements/branches 均达到门禁 |
| `pnpm run check:pine` | exit 0；Worker 98 tests、前端结构语料 9 tests、typecheck/发布检查器测试/合规检查通过 |
| `pnpm run check:python` | exit 0；337 passed，2 个依赖弃用 warning；没有真实上游调用 |
| `pnpm run check:desktop` | exit 0；runtime 脚本 11 tests、desktop 脚本集合 48 tests；不是原生安装验收 |
| `pnpm run check:ai-context` | exit 0，6 modules / 8 instruction files |
| `pnpm run check:quick -- --print` | exit 0；识别当前工作树 83 个 affected files、rust/web/pineworker 三个模块，执行 `test:preflight` 并明确将 `check:all` deferred；未缩小 diff 范围 |
| `pnpm run check:quick` | 最终 exit 0；quick planner 执行的 policy、contracts、Rust workspace/compatibility、Web coverage/typecheck、Pine、Python 均通过；新增跨年/跨午夜回测和 Web reconnect 窄测另行通过；Desktop 11 runtime + 48 script tests 另由本轮 `check:desktop` 通过 |
| `pnpm run check:all` | 未运行；不能把 preflight 或局部门禁通过当成完整构建/桌面 smoke 通过 |
| 失败/跳过/未执行汇总 | 早期失败已闭环：`check:policy` 曾因 1474/900/838 行 product 文件超限，早期 `check:rust`/`check:quick` 曾被保留用户文件字段不匹配阻断；另一次 quick 仅因可再生 Rust target 超过 50,000 个 `.rcgu.o` 被 target-health 阻断，确认无编译进程后清理产物并重跑通过。拆分/修复及清理后最终门禁均通过。新增 G05 两项、TradeRead 三项、跨年聚合和 Web reconnect 窄回归均已单独通过；最终仍跳过：Rust `live_opend_provider_runtime.rs` 与 Pine `real_worker_smoke.rs` 各 1 项（门禁设计的 live/外部依赖跳过）。未执行：`check:all`、真实 OpenD/账户/行情/模型、真实 Go 旧发布包/四平台签名安装/升级/回滚、浏览器跨服务 E2E、gRPC/UI drawing 合成 |
| 文档链接、diff 与最终工作树 | 报告内 3 个本地 Markdown 链接逐一存在；最终 `git diff --check` 通过；HEAD/Go SHA 未变化。三份用户文件当前 `git hash-object` 分别为 `cache_extended_sessions_parity.rs=cb70880c030ab80ef41fbf6386228183bf86e7fa`、`test-parity-report.md=866332040985610f03917e392684fd3a4f985ca7`、`audit_test_parity.py=006462a8f28bf4e0afe614332ced51e1d60a8d72`。 |

诊断中一次复用临时数据库再次 `initialize_current` 返回“manifest table already exists”，这是诊断 setup 重复初始化，不列为产品迁移缺陷；最终更换全新临时目录，未清理或改写用户数据。Rust 跳过项为 `jftrade-integration-futu/tests/live_opend_provider_runtime.rs` 与 `jftrade-integration-pine/tests/real_worker_smoke.rs`，不是失败也不是通过。

审计期间曾在临时实验工作树（随后已撤回）运行过七类 compatibility replay；这些结果不计入最终基线门禁结论。实现/拆分完成后已重新运行并通过 `check:rust` 与 `check:quick`；仍不将 skipped live 项、`check:all` 或外部验收解释为通过。

## 6. 后续批次与关闭条件

1. **交易安全与数据一致性（先关）**：G02/G03 的隔离 fence 与 bounded shutdown、G05 本地九库升级/回滚已有证据；下一批聚焦真实旧发布包/脱敏基线、TradeRead 上游截断和进程级 crash receipt；保留唯一 WriterLease，不猜配外单、不进行历史用户数据修补。
2. **核心工作流**：G01 日历全消费者和 G06 operation readiness 已完成限定验证；继续为新增 advertised operation 补 fixture，并确认 Futu 默认分页和真实外部权限，不将目录/fixture 绿灯外推为 live。
3. **增量输出与用户体验**：G04 已验证本地 gRPC 编码边界，G07 已补 server-state 在 WS 重连后的主动 reconcile；下一步把 Worker→Rust→图表 reducer/UI 的 backpressure、行情 regular/extended/DST 序列、断连、重连、刷新流程串成浏览器闭环。
4. **独立 live/发布资格**：G08（真实 OpenD/账户/行情/模型以及四平台签名、安装、升级、回滚）按当前发布专题执行，不由普通修复授权。

关闭单项必须附：基线 SHA、复现/反证、生产链 owner、实际通过的回归命令、剩余 live 限制。需要改变公开 HTTP/SSE/WS、SQLite schema 或 worker wire 的修复必须另行评审；不能在“补测试”名义下改 golden、放宽阈值或修改合同。

本轮交付完成能力清单归类、F01–F05 修复后证据、G01–G07 的限定验证、G05 真实旧安装/G08 的未验证边界和逐项补测方案；仍不能称“全项目迁移完成”。
