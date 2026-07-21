# JFTrade 项目现状分析与改造建议（2026-07-03）

> 范围：基于当前仓库代码、文档与前端页面结构，对 JFTrade 量化交易项目在架构、业务定位、用户交互、交易链路、回测/实盘一致性、风险治理等方面做只读分析。本文只给意见与改造思路，不包含具体代码实现。
>
> 重要提示：本文只讨论软件工程、产品与交易系统风控设计，不构成任何投资建议。

## 0. 总体判断

JFTrade 当前已经不是一个简单的“Futu 下单客户端”，而是一个以 Futu OpenD 为起点的本地量化交易研发控制台。它把行情观察、账户/订单管理、Pine 策略设计、历史数据同步、回测、策略实例运行、ADK/Agent 工作流、系统诊断放进同一个本地 sidecar + Web GUI 中。

一句话概括现状：

> **这是一个工程能力已经较强、模块化方向正确、交易研发闭环初步成型的本地量化工作台；但当前最大瓶颈已经从“有没有功能”转为“能否让架构 ownership、交易风控可信度、用户主路径、产品定位继续收敛”。**

当前最值得优先处理的不是继续横向堆功能，而是：

1. **缩小 `servercore`，让模块真正拥有自己的运行时与资源。**
2. **把交易链路里的风控、审批、kill switch、订单状态机做成真实生产级控制面。**
3. **强化回测与实盘一致性说明、版本化与可验证性。**
4. **降低用户在交易环境、账户、标的、策略版本之间误操作的概率。**
5. **收敛产品叙事：先做强 “Futu-first 的策略研发与半自动执行工作台”。**

---

## 1. 当前产品与业务定位

### 1.1 当前产品定位

从 [README.md](../README.md)、[docs/architecture.md](architecture.md)、前端路由与组件命名看，JFTrade 当前最准确的产品定位是：

> **面向 Futu/OpenD 用户的本地部署量化研究与半自动交易工作台。**

它不是纯交易终端，也不是云端 SaaS 量化平台，更不是机构级 OMS/EMS。当前更像：

- Futu 用户的本地量化研发控制台；
- 个人量化研究者 / 小团队的策略实验台；
- 带 AI/Agent 增强能力的策略研究与执行工具；
- 以本地可控、数据与密钥留在本机为卖点的桌面/sidecar 产品。

### 1.2 已具备的核心能力

当前已有能力比较完整：

| 能力域 | 当前能力 | 成熟度判断 |
| --- | --- | --- |
| 本地 sidecar | `cmd/jftrade-api` + `internal/app/apiserver`，API-only 后端，内嵌前端发布 | 较成熟 |
| Futu/OpenD 接入 | OpenD 连接、账户发现、行情、交易读写、订单更新 | 已可用，但单券商心智重 |
| 行情工作台 | K 线、盘口、快照、自选、扩展时段、实时订阅 | 较成熟，但 UI 状态需统一 |
| 账户/订单 | 资金、持仓、订单、成交、费用、保证金、现金流水 | 能力较宽，需加强订单闭环 |
| 下单 | 限价/市价/止损/止损限价、TIF、US session、最大可交易数量估算 | 可用，但需增强风险确认与状态追踪 |
| 策略设计 | Pine v6 编辑、结构化源码块、Monaco、分析/保存 | 有辨识度，边界需产品化说明 |
| 策略执行 | 定义实例化、绑定账户/标的/周期、运行风控、启停、日志/审计 | 已形成控制面，但 runtime ownership 仍偏集中 |
| 回测 | 历史 K 线同步、成本模型、执行模型版本、结果查看 | 方向正确，是信任基础 |
| ADK/Agent | 会话、Agent、Provider、Workflow Studio、审批/工具 | 能力很宽，但容易变成第二个超级模块 |
| 系统诊断 | 设置页开发者工具、富途接入、运行时依赖、观测摘要 | 工程排障能力不错 |

### 1.3 推荐产品主叙事

建议短中期把主叙事收敛为：

> **Futu-first 的本地量化策略研发与半自动执行工作台。**

不建议当前阶段对外主打：

- “通用多券商量化平台”；
- “完整 TradingView Pine 替代品”；
- “全自动 AI 交易平台”；
- “机构级交易 OMS/EMS”。

这些叙事要么当前能力尚未完全支撑，要么会带来过高的合规和信任成本。

---

## 2. 架构现状与建议

### 2.1 当前后端架构现状

当前后端已经从“大 Gin Server + 直连券商/DB”的传统单体，演进为分层明确的模块化单体。

主要分层如下：

1. **进程入口层**
   - [cmd/jftrade-api/main.go](../cmd/jftrade-api/main.go)
   - 职责较薄：参数校验、信号处理、启动 API-only 模式。

2. **应用装配与生命周期层**
   - [internal/app/apiserver/server.go](../internal/app/apiserver/server.go)
   - [internal/app/apiserver/lifecycle/lifecycle.go](../internal/app/apiserver/lifecycle/lifecycle.go)
   - [internal/app/apiserver/runtime/runtime.go](../internal/app/apiserver/runtime/runtime.go)
   - 负责运行时路径、settings store、handler 构造、启动/关闭流程。

3. **HTTP / SSE / WebSocket 传输层**
   - [internal/api/](../internal/api/)
   - 目前大体保持 thin transport：参数绑定、错误映射、响应封装、路由注册。
   - [internal/api/httpserver/](../internal/api/httpserver/) 和 [internal/api/middleware/](../internal/api/middleware/) 已抽出公共能力。

4. **业务服务层**
   - [internal/settings/](../internal/settings/)
   - [internal/marketdata/](../internal/marketdata/)
   - [internal/live/](../internal/live/)
   - [internal/strategy/](../internal/strategy/)
   - [internal/backtest/](../internal/backtest/)
   - [internal/trading/](../internal/trading/)
   - [internal/assistant/](../internal/assistant/)
   - 多数模块已具备 service + interface + adapter 的雏形。

5. **持久化与基础设施层**
   - [internal/store/settingsfile/](../internal/store/settingsfile/)
   - [internal/store/sqliteconn/](../internal/store/sqliteconn/)
   - [internal/store/sqliteschema/](../internal/store/sqliteschema/)
   - SQLite 连接、schema 初始化/兼容性校验已有统一基础设施。

6. **集成与 legacy 可复用层**
   - [internal/integration/futu/](../internal/integration/futu/)
   - [pkg/futu/](../pkg/futu/)
   - [pkg/backtest/](../pkg/backtest/)
   - [pkg/strategy/](../pkg/strategy/)
   - [pkg/adk/](../pkg/adk/)
   - 这里仍是迁移中的区域：有些能力已经 internal 化，有些仍作为 `pkg/*` 被装配层直接使用。

### 2.2 架构主要优点

#### 优点 1：入口和生命周期层已经比较干净

`cmd/jftrade-api` 很薄，真正启动逻辑下沉到 `internal/app/apiserver`。`lifecycle.Dependencies` 把启动流程与具体实现拆开，对测试、替换实现和后续模块化都有价值。

#### 优点 2：HTTP transport 已明显变薄

[internal/api/](../internal/api/) 下的 handler 大多不直接处理 SQLite、OpenD、Futu protobuf、broker SDK。统一响应、URI 绑定、分页、时间解析、中间件也已经抽到公共层。

这说明“transport thin”目标基本已落地。

#### 优点 3：市场数据链路设计较成熟

[internal/marketdata/service.go](../internal/marketdata/service.go) 使用 broker-neutral `Provider`，collector 处理：

- demand aggregation；
- push + fallback polling；
- freshness/cache；
- backoff / reset / resume / close；
- 订阅者活跃标的聚合。

这比简单 CRUD 行情接口成熟很多。

#### 优点 4：`live` 是较干净的传输无关事件核心

[internal/live/publisher.go](../internal/live/publisher.go) 提供 replay publisher，有利于前端重连、通知补偿和事件一致性。

#### 优点 5：回测服务已有异步任务与生命周期模型

[internal/backtest/service.go](../internal/backtest/service.go) 已经管理回测任务、K 线同步任务、取消、状态、结果视图，并且通过接口注入策略与 K 线同步能力。

#### 优点 6：有架构文档和边界守卫意识

已有：

- [docs/architecture.md](architecture.md)
- [docs/architecture/backend-layout-v1.md](architecture/backend-layout-v1.md)
- [docs/architecture/high-value-optimization-implementation-plan.md](architecture/high-value-optimization-implementation-plan.md)
- `scripts/check-arch-deps.sh`

这说明项目不是“口头架构”，而是开始把目标和约束写进文档与脚本。

### 2.3 架构主要风险

#### 风险 1：`servercore` 仍是最大架构债务

[internal/app/apiserver/servercore/](../internal/app/apiserver/servercore/) 仍是巨大聚合点。它同时持有：

- settings store；
- strategy catalog/runtime/design store；
- backtest run store；
- execution orders；
- live websocket；
- live notifications；
- marketdata runtime；
- broker registry；
- ADK runtime；
- system/settings/backtest/strategy/marketdata/trading/assistant services。

这意味着模块虽然抽了 service，但真正运行期 ownership 仍集中在一个大对象里。

后果：

- 修改一个模块容易牵动其他模块；
- 测试替换成本高；
- 生命周期治理容易越来越复杂；
- 模块自治无法真正完成；
- 多券商、多 provider、多 runtime 的扩展会继续堆进 `servercore`。

#### 风险 2：`pkg/*` legacy 能力仍渗透装配层

文档目标是外部集成收敛到 integration，但真实装配中仍能看到 `servercore` 直接接触：

- `pkg/adk`
- `pkg/backtest`
- `pkg/broker`
- `pkg/futu`

这不一定立刻错误，但会让 internal 模块边界和 legacy 可复用边界长期混杂。

#### 风险 3：`strategy`、`trading`、`assistant` 自治程度不均衡

- `strategy`：service façade 已有，但 runtime/store 实现仍在 servercore 附近。
- `trading`：service 依赖大量闭包注入，port 语义不如 marketdata.Provider 清晰。
- `assistant`：service façade 已抽出，但核心 runtime 仍指向 `pkg/adk`，同时 workflow/scheduler/trigger/webhook 能力继续扩张。

#### 风险 4：settings/system 有“管理面超级模块”倾向

settings 已不只是配置读写，还承担了数据管理、数据库维护、onboarding、broker settings 聚合等能力。system 也聚合了大量运行状态、OpenD 诊断、worker 状态、real-trade 入口。

短期方便，长期会让管理面变成第二个 `servercore`。

#### 风险 5：runtime resource ownership 尚未统一

当前路径派生集中在 [internal/app/apiserver/runtime/runtime.go](../internal/app/apiserver/runtime/runtime.go)，包括：

- settings file；
- backtest DB；
- strategy catalog/runtime DB；
- execution orders DB；
- ADK DB / session DB。

但“哪个模块拥有哪个资源、谁初始化、谁校验 schema、谁关闭、谁暴露 health”还没有完全形成统一契约。

### 2.4 架构改造建议

#### 建议 A1：先拆 `servercore` 的对象 ownership，再拆文件

目标：让 `servercore.Server` 从“持有一切”变为“协调模块 bootstrap 结果”。

落地思路：

1. 定义模块启动结果结构，例如：
   - `SettingsModule`
   - `MarketDataModule`
   - `TradingModule`
   - `StrategyModule`
   - `BacktestModule`
   - `AssistantModule`
2. 每个 module bootstrap 返回：
   - service；
   - route registrar；
   - background workers；
   - closers；
   - health provider；
   - observability labels。
3. `servercore` 只负责：
   - 统一创建模块；
   - 聚合路由；
   - 聚合 shutdown；
   - 提供少量跨模块 wiring。
4. 再逐步把 `servercore/server.go` 压缩为 composition root。

优先级：P0。

#### 建议 A2：把 strategy runtime/store 从 servercore 迁回 strategy / store

目标：让 [internal/strategy/](../internal/strategy/) 不只是 façade，而是真正拥有策略定义、实例、runtime 的核心边界。

落地思路：

1. 将 strategy design store、catalog store、runtime store、runtime manager 迁到：
   - `internal/strategy/...`；或
   - `internal/store/strategy...`。
2. `internal/strategy/service.go` 只依赖本包定义的小接口。
3. app 层只注入具体实现，不再直接暴露 servercore 细节。
4. runtime risk、runtime observation、audit/log 也应向 strategy 模块内聚。

优先级：P0/P1。

#### 建议 A3：重做 trading 的端口模型，减少闭包式注入

目标：把 trading 从“由 Server 拼接的操作集合”升级为“有稳定端口的交易业务模块”。

建议定义更明确的接口：

- `OrderGateway`：place/cancel/modify/preview；
- `OrderStore`：内部订单 ledger；
- `OrderEventStore`：订单事件流；
- `BrokerRuntimeProvider`：券商连接/能力/健康；
- `AccountProvider`：账户发现/选择；
- `RiskGate`：下单前风险检查；
- `ExecutionNotifier`：通知/审计。

落地思路：

1. 保留现有 API JSON 契约不变。
2. 先把 [internal/app/apiserver/servercore/trading_adapters.go](../internal/app/apiserver/servercore/trading_adapters.go) 的逻辑迁为 adapter。
3. `OrderUpdatesWorker` 保留，但 source/execution 接口向 trading 内部收敛。
4. 最后让 servercore 只装配 trading module。

优先级：P0。

#### 建议 A4：建立统一 RuntimeResources 模型

目标：明确每类运行时资源的 owner。

建议引入 `RuntimeResources` / `ResourceRegistry`，记录：

| 资源 | Owner | 初始化 | Schema 校验 | Close | Health |
| --- | --- | --- | --- | --- | --- |
| settings file | settings | settings module | settings store | N/A | settings health |
| backtest run DB | backtest | backtest module | sqliteschema | module close | backtest health |
| strategy runtime DB | strategy | strategy module | sqliteschema | module close | strategy health |
| execution orders DB | trading | trading module | sqliteschema | module close | trading health |
| ADK DB | assistant/runtime | assistant module | adk store | module close | assistant health |

优先级：P1。

#### 建议 A5：assistant 拆成 runtime 与 workflow 两个子边界

当前 assistant 已经覆盖会话、provider、agent、tool、workflow、scheduler、trigger、webhook、market snapshot。建议拆成：

- `internal/assistant/runtime`：session、run、approval、provider、tool；
- `internal/assistant/workflow`：workflow definition、scheduler、trigger、webhook、child run；
- `internal/assistant/admin`：provider/agent/skill 配置。

同时把 `*jfadk.Runtime` 藏到接口后面，不让 service 和 API route 持续扩大对 `pkg/adk` 的直接认知。

优先级：P1。

#### 建议 A6：settings 回归配置模块，数据维护独立出去

建议新增：

- `internal/runtimeadmin`；或
- `internal/datamanagement`。

把 cleanup / compact / rebuild / storage overview 等能力从 settings service 中迁出。settings 只保留：

- 配置读取；
- 配置保存；
- 配置 normalize；
- 配置变更 side effect 触发。

API 可以先保持旧路径兼容，再逐步迁移。

优先级：P1。

#### 建议 A7：强化架构守卫脚本

建议扩展 `scripts/check-arch-deps.sh`：

1. `internal/api/...` 不得依赖 `pkg/futu`、`pkg/adk`、`pkg/backtest` 等具体实现。
2. `internal/{strategy,trading,assistant,backtest,marketdata,settings}` 不得依赖 `servercore`。
3. `servercore` 逐步禁止直接 import `pkg/futu` / `pkg/adk` / `pkg/backtest`，改经 internal integration/adapter。
4. 分阶段执行：
   - 第一阶段 warning；
   - 第二阶段 CI fail。

优先级：P1。

---

## 3. 交易链路现状与风险

### 3.1 当前交易链路推断

当前交易链路可以拆成 6 条子链路。

#### 3.1.1 行情链路

```text
前端 Workspace / Watchlist / Chart / OrderBook
  -> /api/v1/market-data/* 或 /api/v1/stream/live
  -> internal/api/marketdata 或 internal/api/live
  -> internal/marketdata.Service + Collector + Cache
  -> internal/integration/futu.MarketDataRuntime
  -> pkg/futu Exchange / OpenD TCP API
  -> Futu OpenD
```

特点：

- 前端共享全局市场/标的上下文；
- marketdata service 负责 cache、订阅、collector；
- live 负责事件推送和回放；
- Futu integration 负责协议访问与转换；
- collector 支持 push + fallback polling。

#### 3.1.2 手工下单链路

```text
OrderEntryPanel.vue
  -> POST /api/v1/execution/orders
  -> internal/api/trading
  -> internal/trading.Service.normalizeExecutionOrder
  -> servercore.placeExecutionOrder
  -> brokerExecutionExchange().PlaceBrokerOrder
  -> executionOrderStore.recordPlacedOrder
  -> live notification / UI refresh
```

当前能力：

- 支持 `LIMIT`、`MARKET`、`STOP`、`STOP_LIMIT`；
- 默认 broker 为 `futu`；
- 非 futu broker 当前会被拒绝；
- 默认交易环境可回退到 `SIMULATE`；
- US 市场支持 `RTH`、`ETH`、`ALL`、`OVERNIGHT` session；
- `FOK` 被明确拒绝；
- 下单后进入内部 execution order ledger。

#### 3.1.3 订单回报链路

```text
Futu OpenD order/fill push 或主动查询
  -> internal/integration/futu/order_updates.go
  -> internal/trading.OrderUpdatesWorker
  -> tradingExecutionOrderUpdates.ApplyOrder / ApplyFill
  -> executionOrderStore upsert / event timeline
  -> execution_notifications
  -> live publisher / frontend refresh
```

特点：

- 同时支持 push 与 fallback sync；
- 有 active orders cache；
- 有 current orders + history orders 同步；
- 内部 order ledger 合并 command-side 与 broker-side 状态。

#### 3.1.4 策略实盘/模拟运行链路

```text
StrategyRuntimePage 启动实例
  -> internal/strategy.Service.StartInstance
  -> servercore strategyRuntimeManager
  -> Pine worker live closed-kline evaluation
  -> WorkerOrderCommand
  -> runtime risk evaluation
  -> notify_only 或 live order executor
  -> servercore.placeExecutionOrder
  -> broker / execution order store
```

当前策略 runtime 风控包括：

- `off | monitor | enforce`；
- close-only；
- max order quantity；
- max notional；
- daily max orders；
- pause-on-reject。

#### 3.1.5 回测链路

```text
BacktestPage
  -> /api/v1/backtests
  -> internal/backtest.Service
  -> strategy definition lookup
  -> K-line readiness / sync
  -> pkg/backtest RunWithPineWorker
  -> conservative-bar-v1 execution model
  -> result store / result view
  -> frontend BacktestPage / BacktestChart
```

当前已做的关键点：

- `executionModel` 是回测请求和结果的一等字段；
- 默认模型为 `conservative-bar-v1`；
- 未知模型会作为请求错误处理，不静默降级；
- 成本模型与交易费用已进入回测结果；
- extended hours 能进入回测参数。

#### 3.1.6 实盘控制面链路

前端已有 real-trade state 数据模型：

- approvals；
- hard stops；
- kill switch；
- risk limits；
- risk events。

但当前后端 `internal/system/service.go` 相关接口主要返回 disabled / empty / default false 状态。也就是说，UI 与 API 已经预留了控制面，但真实生产级状态机和执行拦截还没有完全落地。

这是交易风险治理中最需要优先补齐的部分。

### 3.2 交易链路优点

#### 优点 1：订单链路不是裸调券商 API

当前已经有内部 execution order ledger、order event timeline、broker sync/push reconciliation，不是直接从 UI 调 Futu 然后结束。

这是未来做审计、风控、重放、问题定位的基础。

#### 优点 2：策略 runtime 已经有本地风控点

`runtimeRiskSettings` 说明项目已经意识到策略下单不能裸奔，需要在策略信号与真实下单之间有 gate。

#### 优点 3：回测执行模型开始版本化

`conservative-bar-v1` 是一个好的方向。交易系统最怕“回测模型变了但结果看不出来”。版本化能让后续结果可解释、可比较。

#### 优点 4：Futu/OpenD 连接排障与文档较完整

[docs/troubleshooting/opend-configuration.md](troubleshooting/opend-configuration.md)、[docs/troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md)、设置页富途接入的 OpenD health 说明运行诊断能力已经较认真。

### 3.3 交易链路主要风险

#### 风险 T1：real-trade 控制面有 UI/接口形态，但后端状态仍偏占位

这是最高优先级风险。

如果用户看到 kill switch、hard stop、approval、risk limit 的 UI 或 API 名称，会自然以为它们已经完整保护实盘。但当前后端真实状态主要是默认 disabled/empty。

这会带来：

- 用户安全感误判；
- 实盘误操作风险；
- 后续商业/合规风险；
- 文档与真实行为不一致。

#### 风险 T2：手工下单和策略下单可能没有统一经过同一个全局风险网关

策略 runtime 有 `runtimeRiskSettings`，但手工下单主要是参数 normalize + broker capability。未来应确保：

- 手工单；
- 策略单；
- ADK/Agent 发起的单；
- workflow 发起的单；
- 未来 API 调用发起的单；

全部经过同一个 pre-trade risk gate。

#### 风险 T3：`brokerId=futu` 硬编码阻碍多券商扩展

当前 `internal/trading/execution.go` 默认 futu 且拒绝非 futu。短期可以接受，但如果文档/产品开始强调多券商，这里会成为核心债务。

#### 风险 T4：订单状态机尚需显式 canonical model

当前有内部订单状态、Futu 状态、事件类型、通知状态。但建议进一步明确：

- canonical order status；
- raw broker status；
- 状态迁移表；
- terminal status；
- partial fill 行为；
- cancel pending / cancel rejected；
- unknown / out-of-order push 处理。

#### 风险 T5：回测与实盘一致性仍需要更强解释

虽然已有 `conservative-bar-v1`，但实盘是实时行情、订单簿、券商撮合、时段、可交易数量、融资融券规则共同作用。用户如果把回测结果理解为实盘保证，会有预期风险。

#### 风险 T6：策略 runtime 声称 `supportsBacktestParity: true` 容易被误解

如果 UI 或 API 将其展示成“回测与实盘完全一致”，会造成误导。更准确的表达应是：

- `supportsBacktestParityMetadata`；
- `usesSharedOrderIntentModel`；
- `parityLevel: closed-bar-signal-compatible`；
- 并展示不一致来源。

#### 风险 T7：全局交易上下文误操作风险

前端全局顶栏控制：

- 市场；
- 标的；
- 账户；
- 模拟/实盘。

这些上下文跨 Workspace、Account、Strategy Runtime、Backtest 复用，但局部页面的持续提醒不足。实盘/模拟盘切换尤其危险。

#### 风险 T8：ADK/Agent 未来接入交易动作时风险会放大

ADK 已有工具、workflow、approval 概念。如果未来允许 Agent 触发交易或策略发布，必须先完成：

- 明确的权限模型；
- human approval；
- dry-run；
- 风控拦截；
- audit log；
- tool scope；
- 外发数据治理。

---

## 4. 回测与实盘一致性现状

### 4.1 当前做得好的地方

1. **执行模型已版本化**
   - `executionModel` 进入请求和结果。
   - 默认 `conservative-bar-v1`。
   - 未知模型报错。

2. **回测成本模型已进入结果**
   - broker fees / market fees / instrument type / quote currency 相关成本有显式处理。

3. **Pine worker 作为共同策略语义基础**
   - 回测和 live runtime 都围绕 Pine worker / Pine intent 工作，有利于保持策略信号语义一致。

4. **策略 runtime 有风控拦截点**
   - 从信号到下单之间有 `runtime risk`。

5. **扩展时段开始进入参数模型**
   - US extended hours 的数据和订单 session 都已有建模。

### 4.2 当前仍不一致或需说明的地方

| 维度 | 回测 | 实盘/模拟运行 | 风险 |
| --- | --- | --- | --- |
| 撮合 | `conservative-bar-v1`，基于 bar 的保守撮合 | 券商真实撮合 / 模拟盘规则 | 成交价、部分成交、滑点不同 |
| 流动性 | 每根 bar 的 volume budget | 真实盘口/券商规则 | 回测成交可行性仍是估计 |
| 订单类型 | worker intent 转换 | Futu 支持能力限制 | 某些 Pine intent live 不支持或 no-op |
| cancel/cancel_all | 回测可建模 | live runtime 中 cancel/cancel_all 当前存在 no-op 风险 | 用户误以为策略可完整撤单 |
| `QuantityPct` | 回测可表达 | live runtime 拒绝 `QuantityPct > 0` | 策略迁移需提示 |
| session | useExtendedHours | RTH/ETH/ALL/OVERNIGHT | 数据口径与下单 session 需对齐 |
| 风控 | 回测结果统计 | runtime risk gate | 回测不一定模拟所有实盘风控拒单 |
| 账户状态 | 初始资金模型 | 真实资金/持仓/保证金 | 买卖能力差异 |

### 4.3 建议的 parity 表达方式

不要简单写“支持回测实盘一致”。建议改成更精确的三层表达：

1. **Signal parity**：回测和实盘使用同一策略源码/同一 Pine worker 语义产生信号。
2. **Order intent parity**：回测和实盘尽量共享订单意图模型，但 live 有 broker capability 限制。
3. **Execution parity**：回测执行模型是 `conservative-bar-v1`，并非真实券商撮合；实盘结果以券商为准。

前端回测结果页、策略运行页都建议展示：

- execution model；
- 支持的 Pine 子集；
- live 不支持的 intent；
- 当前策略是否包含不支持 live 的语义；
- 数据 session 与订单 session 是否一致。

---

## 5. 用户交互与前端体验现状

### 5.1 当前信息架构

前端主结构：

- [apps/web/src/App.vue](../apps/web/src/App.vue)
- [apps/web/src/layout/AppShell.vue](../apps/web/src/layout/AppShell.vue)
- [apps/web/src/router.ts](../apps/web/src/router.ts)

主路由：

- `/workspace`：交易工作台；
- `/account`：账户；
- `/strategy/runtime`：策略执行；
- `/strategy/design`：策略设计；
- `/backtest`：回测；
- `/adk/agents`：智能体；
- `/adk/workflows`：工作流；
- `/system`：系统；
- `/settings/:section?`：设置；
- `/oobe`：首次引导。

全局壳层包括：

- AuthGate；
- OOBE；
- TopBar；
- IconRail；
- RouterView；
- RightDock；
- StatusBar；
- CommandPalette。

这是典型专业工作台结构，适合高频使用，但对新用户压强较高。

### 5.2 主要交互优点

1. **交易工作台布局专业**
   - 图表、盘口、自选、持仓、下单都在一个页面。

2. **全局上下文统一**
   - 顶栏集中管理市场、标的、账户、环境，减少重复选择。

3. **OOBE 存在**
   - 对 OpenD / Node / PineTS worker 这类本地依赖有引导，是正确方向。

4. **设置页能力完整**
   - 富途接入、托管账户、账户发现、Pine Worker、ADK、安全、数据管理都有入口。

5. **前端测试覆盖较强**
   - `apps/web/tests` 覆盖行情、执行、回测、策略、设置、OOBE、ADK 等业务边界。

### 5.3 主要交互风险

#### 风险 UX1：全局交易上下文过强，但局部页面提示不足

用户在页面间切换时可能不知道当前：

- 是实盘还是模拟盘；
- 是哪个账户；
- 是哪个市场；
- 是哪个标的；
- 策略实例绑定是否与当前顶栏一致。

#### 风险 UX2：导航按系统能力组织，而不是按用户任务组织

当前把交易、账户、策略执行、策略设计、回测、智能体、系统、设置并列。对开发者清晰，但普通用户可能困惑：

- 策略设计和策略执行为什么分成两个一级入口？
- 系统和设置区别是什么？
- ADK 页面与设置里的 ADK 是什么关系？
- 账户页和工作台持仓/订单的差异是什么？

#### 风险 UX3：工作台密度高，新用户不知道从哪里开始

[WorkspacePage.vue](../apps/web/src/pages/WorkspacePage.vue) 是专业终端式布局，但对新手缺少任务引导：

- 先连接 OpenD？
- 先选账户？
- 先选标的？
- 先看模拟盘？
- 如何确认当前下单不会进实盘？

#### 风险 UX4：回测页职责过多

回测页同时处理：

- 策略选择；
- 标的/周期/时间；
- 数据同步；
- 成本模型；
- 扩展时段；
- 启动任务；
- 结果浏览。

专业用户喜欢高密度，但普通用户容易不知道当前是在准备数据还是已经启动回测。

#### 风险 UX5：ADK 信息架构过宽

ADK 同时是：

- 聊天/会话；
- 工作流 Studio；
- provider/agent/tool/skill 管理；
- approval/child run/queue 监控。

如果没有清楚区分“使用”和“配置”，它会成为另一个复杂子产品。

### 5.4 前端交互改造建议

#### 建议 U1：给所有交易相关页面增加“交易作用域条”

在 Workspace、Account、Strategy Runtime、Backtest 页面头部固定展示：

- 当前环境：模拟 / 实盘；
- 当前账户；
- 当前市场；
- 当前标的；
- 当前 broker；
- 当前连接健康；
- 如果是实盘，用强视觉提示。

涉及：

- [apps/web/src/layout/TopBar.vue](../apps/web/src/layout/TopBar.vue)
- [apps/web/src/pages/WorkspacePage.vue](../apps/web/src/pages/WorkspacePage.vue)
- [apps/web/src/pages/AccountPage.vue](../apps/web/src/pages/AccountPage.vue)
- [apps/web/src/pages/BacktestPage.vue](../apps/web/src/pages/BacktestPage.vue)
- [apps/web/src/pages/StrategyRuntimePage.vue](../apps/web/src/pages/StrategyRuntimePage.vue)

优先级：P0。

#### 建议 U2：策略收拢为一个一级模块

保留路由兼容，但导航上将：

- 策略执行；
- 策略设计；

收敛成一个“策略”入口，内部再分：

- 定义；
- 设计；
- 实例；
- 运行；
- 日志/审计。

优先级：P1。

#### 建议 U3：回测页改成“向导式提交 + 分析式结果”

不必改后端，先重组 UI：

1. 选择策略；
2. 选择标的与市场；
3. 选择时间与 session；
4. 数据准备状态；
5. 成本与执行模型摘要；
6. 启动回测；
7. 结果详情区。

特别要把“同步数据”和“运行回测”视觉上分开。

优先级：P1。

#### 建议 U4：统一订单反馈闭环

下单后应出现“订单回执卡”：

- 内部单号；
- 券商单号；
- 当前状态；
- 是否已被券商接受；
- 最近事件；
- 可跳转到账户页订单详情；
- 撤单状态。

涉及：

- [apps/web/src/components/workspace/OrderEntryPanel.vue](../apps/web/src/components/workspace/OrderEntryPanel.vue)
- [apps/web/src/components/workspace/PositionsPanel.vue](../apps/web/src/components/workspace/PositionsPanel.vue)
- [apps/web/src/pages/AccountPage.vue](../apps/web/src/pages/AccountPage.vue)
- [apps/web/src/composables/consoleDataExecutionOrders.ts](../apps/web/src/composables/consoleDataExecutionOrders.ts)

优先级：P0/P1。

#### 建议 U5：行情面板统一“连接/新鲜度/时段”状态表达

图表、Watchlist、OrderBook 都展示统一状态：

- 实时连接状态；
- 数据更新时间；
- stale / fresh；
- 当前交易时段；
- regular / extended；
- 是否 fallback polling；
- depth 与 snapshot 时间差。

优先级：P1。

#### 建议 U6：ADK 明确区分使用与配置

建议命名和页面说明改为：

- `/adk/agents`：智能体工作台；
- `/adk/workflows`：工作流 Studio；
- `/settings/adk`：ADK 管理配置。

并增加互链：

- 工作台缺 provider 时跳配置；
- 配置完成后跳工作台；
- workflow 需要工具权限时跳管理页。

优先级：P2。

---

## 6. 风控、合规与运营建议

### 6.1 最高优先级：统一 Pre-trade Risk Gateway

建议新增统一的交易前风控网关，所有下单路径必须经过：

```text
manual order / strategy order / ADK tool / workflow / external API
  -> PreTradeRiskGateway
  -> approval / reject / monitor event
  -> broker gateway
```

风控检查至少包括：

- kill switch；
- real trading enabled；
- account enabled；
- broker connected；
- market session allowed；
- max order quantity；
- max notional；
- daily max orders；
- symbol allowlist / denylist；
- close-only；
- duplicate clientOrderId；
- price deviation from market；
- max slippage；
- max position concentration；
- strategy instance status；
- ADK/tool permission scope。

输出统一 decision：

```text
ALLOW / REJECT / REQUIRE_APPROVAL / MONITOR_ONLY
reason_code
reason_message
risk_snapshot
approval_id(optional)
```

优先级：P0。

### 6.2 把 kill switch / hard stop 做成真实状态机

当前 real-trade API 形态已有，但后端状态偏占位。建议实现持久化控制面：

- `real_trade_controls` 表或 settings + event log；
- kill switch active/inactive；
- hard stop entries；
- blocked operations；
- operator / source；
- reason；
- createdAt / clearedAt；
- audit events。

并确保 trading/strategy/ADK 下单路径在 broker 调用前检查。

优先级：P0。

### 6.3 建立订单状态机与 broker conformance

定义 JFTrade canonical order lifecycle：

```text
CREATED
PRECHECK_REJECTED
SUBMITTING
SUBMITTED
BROKER_ACCEPTED
PARTIALLY_FILLED
FILLED
CANCEL_REQUESTED
CANCELLED
REJECTED
EXPIRED
UNKNOWN
```

同时保存 raw broker status，做映射表与测试。

建议先做 fake broker conformance，不急着接第二个真实券商：

- place accepted；
- place rejected；
- partial fill；
- full fill；
- cancel accepted；
- cancel rejected；
- push arrives before query；
- query arrives before push；
- disconnect/reconnect；
- unsupported capability。

优先级：P0/P1。

### 6.4 完善审批系统

对实盘交易建议分级审批：

| 场景 | 建议策略 |
| --- | --- |
| 模拟盘手工单 | 可直接下单 |
| 实盘小额手工单 | 二次确认 |
| 实盘大额手工单 | require approval |
| 策略实盘启动 | require approval |
| 策略实盘自动下单 | 受 risk gateway 约束 |
| ADK 发起交易 | 默认 require approval |
| workflow 发起批量操作 | require approval + dry-run |

优先级：P1。

### 6.5 AGPL / PineTS 合规治理

项目中 PineTS / pinets 相关能力需要持续关注 AGPL 或类似许可证义务。建议：

- 发布包内包含第三方 notice；
- 关于页展示许可证；
- 文档说明源码获取方式；
- build script 加 release gate；
- 明确哪些组件受 copyleft 影响；
- 不在商业宣传中模糊 license 边界。

优先级：P1。

### 6.6 Agent/LLM 数据治理

ADK 涉及 provider key、账户信息、策略内容、运行记录、工具调用。建议：

- 默认不把账户敏感信息发给外部模型；
- 工具参数脱敏；
- provider 级别数据策略；
- session retention 设置；
- tool call audit；
- “交易相关工具”默认 require approval；
- 明确哪些工具只读、哪些工具可写、哪些工具可交易。

优先级：P1/P2。

---

## 7. 业务路线建议

### 7.1 当前最适合的目标用户

最适配：

1. **Futu 生态进阶用户**
   - 已会使用 OpenD；
   - 想做规则化/半自动交易；
   - 需要本地控制台。

2. **个人量化研究者 / 小团队**
   - 会写或愿意学 Pine；
   - 需要策略设计、回测、运行闭环；
   - 偏好本地部署。

3. **AI 辅助研究用户**
   - 希望用 Agent 辅助回测、解释、生成策略、编排流程；
   - 但前提是底层交易/回测可信。

不建议当前优先面向：

- 完全零基础散户；
- 大型机构交易团队；
- 多券商、多席位、集中权限管理场景；
- 高频/低延迟交易场景。

### 7.2 30 / 60 / 90 天路线

#### 0-30 天：可信主路径

目标：让用户能安全、清楚地完成“连接 -> 模拟盘 -> 回测 -> 策略运行”。

建议做：

1. 统一交易作用域条；
2. 实盘强确认与 UI 风险提示；
3. 真实 kill switch 状态机第一版；
4. 回测 execution model 和限制说明产品化；
5. 策略 live 不支持项在保存/启动前提示；
6. 订单回执卡；
7. OOBE 中明确“模拟盘先行”。

验收：

- 新用户能完成一次模拟盘策略回测和运行；
- 用户清楚当前是否实盘；
- kill switch 能真实阻断下单；
- 回测结果页能解释模型限制。

#### 31-60 天：交易内核与扩展性

目标：让交易链路从“Futu 可用”升级为“可验证、可扩展”。

建议做：

1. PreTradeRiskGateway；
2. canonical order status；
3. fake broker conformance；
4. 去除 `brokerId=futu` 的硬编码心智；
5. marketdata provider descriptor；
6. strategy runtime/store ownership 迁移；
7. ADK 交易工具默认审批。

验收：

- 手工、策略、ADK 下单都走同一风控网关；
- fake broker 能模拟主要订单生命周期；
- broker capability 可以被 UI 展示；
- strategy runtime 不再强依赖 servercore 内部状态。

#### 61-90 天：商业化与治理底座

目标：具备小范围种子用户/私测/商业试点条件。

建议做：

1. License / notice / release gate；
2. 审批与审计导出；
3. 回测报告可分享；
4. 策略运行报告；
5. ADK 数据保留与外发策略；
6. 5-10 个种子用户试点；
7. 决定长期路线：Futu-first 专业桌面工具 vs 多券商平台。

验收：

- 风险、合规、审计不再只是文档；
- 用户能提供真实使用反馈；
- 产品叙事明确；
- 可以评估是否进入商业化。

---

## 8. 优先级清单

### P0：必须优先做

1. `servercore` ownership 拆分设计；
2. trading port model 重构设计；
3. PreTradeRiskGateway；
4. kill switch / hard stop 真实状态机；
5. 交易作用域条；
6. 订单回执与状态闭环；
7. 回测/实盘一致性说明产品化；
8. 策略 live 不支持语义的启动前阻断/提示。

### P1：中期增强

1. strategy runtime/store 迁移出 servercore；
2. broker conformance fake；
3. canonical order status；
4. marketdata provider descriptor；
5. RuntimeResources ownership；
6. settings 数据管理能力独立；
7. assistant runtime/workflow 边界拆分；
8. 架构守卫脚本增强。

### P2：长期能力

1. 第二券商或第二数据源验证；
2. ADK 信息架构重组；
3. 团队/协作/权限管理；
4. 报告分享与导出；
5. License / OSS / commercial release gate；
6. 外部用户试点与商业路线验证。

---

## 9. 结论

JFTrade 当前已经具备一个量化交易研发工作台的核心骨架：

- 行情链路成熟度不错；
- 回测方向正确；
- 策略设计与运行闭环已经形成；
- 前端控制台功能丰富；
- 后端分层和模块化已经明显优于传统大单体；
- 文档、测试、观测意识较强。

但它现在的主要风险也很明确：

- `servercore` 仍是运行时 ownership 黑洞；
- 交易风控控制面尚未完全真实化；
- 回测与实盘一致性需要更克制、更精确地表达；
- 前端全局上下文容易导致实盘误操作；
- ADK 与 settings/system 有继续膨胀成超级模块的趋势；
- 单券商 Futu-first 是现实，应先承认并做强，不宜过早包装成通用多券商平台。

最推荐的路线是：

> **先把 Futu-first 的本地策略研发与半自动执行主路径做到可信、安全、可解释；再用 broker conformance 和 provider descriptor 打开扩展性；最后再考虑商业化、协作化和多券商平台化。**

如果只选三件最有收益的事，建议是：

1. **统一交易风控网关 + kill switch 真实落地。**
2. **收缩 `servercore`，让 trading/strategy 真正拥有自己的 runtime 与 store。**
3. **在 UI 上持续显式展示环境/账户/标的/策略版本/执行模型，降低误操作和误解。**
