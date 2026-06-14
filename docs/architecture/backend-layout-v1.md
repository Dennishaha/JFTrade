# 后端目录优化 V1

状态：V1 落地完成（兼容门面已删除 + internal servercore 收口）
适用范围：JFTrade Go 后端
目标：在保持现有 API、数据库和运行模式兼容的前提下，把按文件拆分推进为按职责拆包。

## 1. 当前基线

当前后端有两个进程入口：

- `cmd/jftrade-api`：独立 API sidecar。
- `cmd/jftrade`：bbgo CLI 包装器，可同时启动 sidecar。

核心问题不是单个文件过长，而是包边界过宽：

- `pkg/jftradeapi` 曾同时包含 HTTP、应用编排、运行时状态、持久化、Futu 协议转换和后台任务。
- `pkg/jftradeapi` 曾约有 153 个 Go 文件。旧 Server 聚合体已整体迁入
  `internal/app/apiserver/servercore`，迁移期兼容门面已于 2026-06-15 删除。
- API transport 曾直接依赖多个 Futu protobuf 包；当前 `internal/api/*` 已保持
  broker/protobuf free。
- `pkg/adk` 同时包含会话、审批、工具、模型供应商和数据库实现。
- `pkg/backtest` 同时承担同步、存储门面、执行编排和结果收集。
- `pkg/strategy` 已经按解析、IR、指标和运行时拆包，可继续作为其他模块的参考。

V1 不追求一次性采用完整 DDD，也不改变现有部署形态。重点是建立清晰、可执行的依赖方向。

## 2. 设计原则

1. `cmd` 只负责进程启动、信号处理和依赖装配。
2. HTTP handler 不直接访问 SQLite、OpenD client 或 protobuf。
3. 业务模块通过小接口依赖外部能力，接口放在使用方包内。
4. Futu protobuf 只存在于 Futu 集成层内部。
5. 跨模块共享的是稳定业务类型，不共享 `Server`、数据库连接或全局运行时对象。
6. 新包默认放入 `internal`；只有确实需要被其他 Go module 复用的能力才保留在 `pkg`。
7. 每次迁移只移动一个垂直模块，并保留原路由和 JSON 契约。

## 3. V1 目标目录

```text
cmd/
  jftrade/
    main.go
  jftrade-api/
    main.go

internal/
  app/
    apiserver/              # API-only 启动与依赖装配
    bbgo/                   # bbgo 模式下的 sidecar 装配

  api/
    httpserver/             # Gin、路由注册、通用响应与生命周期
    middleware/             # 鉴权、日志、恢复、CORS
    system/                 # /system handlers
    settings/               # /settings handlers
    marketdata/             # /market-data handlers
    trading/                # 账户、资金、持仓、订单、成交 handlers
    strategy/               # 策略定义与运行控制 handlers
    backtest/               # 回测 handlers
    assistant/              # ADK/智能助手 handlers
    live/                   # SSE/WebSocket handlers

  system/
    service.go              # 构建信息、健康检查、运行状态

  settings/
    service.go
    repository.go           # Repository 接口
    model.go

  marketdata/
    service.go
    provider.go             # 行情能力接口
    subscription.go
    cache.go

  trading/
    service.go
    broker.go               # 交易能力接口
    order_updates.go
    model.go

  strategy/
    service.go
    catalog.go
    runtime.go
    repository.go

  backtest/
    service.go
    sync.go
    runner.go
    repository.go

  assistant/
    service.go
    session.go
    approval.go
    tools.go
    provider.go             # 模型供应商接口
    repository.go

  live/
    hub.go                  # 统一事件发布接口
    event.go

  store/
    sqlite/
      settings/
      strategy/
      execution/
      backtest/
      assistant/

  integration/
    futu/
      exchange/             # bbgo exchange 适配
      opend/                # OpenD client
      codec/
      pb/                   # 生成代码
      marketdata.go         # 实现 marketdata.Provider
      trading.go            # 实现 trading.Broker
    bbgo/
      notifier.go
      runtime.go
    llm/
      google/
      openai/

  platform/
    buildinfo/
    frontendassets/
    retry/
    clock/

pkg/
  broker/                   # 暂保留：稳定的通用交易类型
  market/                   # 暂保留：市场、交易时段等稳定类型
  strategy/                 # 保留：Pine -> IR -> runtime 策略引擎
```

目录表达依赖方向，不要求每个目录一开始就有代码。空目录不应提前创建。

## 4. 包依赖规则

允许的主依赖方向：

```text
cmd
  -> internal/app
  -> internal/api
  -> internal/{system,settings,marketdata,trading,strategy,backtest,assistant,live}
  -> pkg/{broker,market,strategy}

internal/app
  -> internal/store
  -> internal/integration
  -> internal/platform

internal/integration
  -> 业务模块中由使用方定义的小接口
  -> 外部 SDK / protobuf
```

禁止的方向：

- 业务模块导入 `internal/api`。
- HTTP handler 导入 Futu protobuf。
- `internal/integration` 导入 Gin handler。
- SQLite repository 返回数据库模型或 `*sql.Rows` 给业务层。
- 模块之间通过一个全局 `Server` 结构体读取彼此内部状态。
- 为复用少量辅助函数建立 `utils`、`common` 或 `helpers` 大包。

## 5. 现有代码迁移映射

| 当前代码 | V1 目标 |
| --- | --- |
| `pkg/jftradeapi/server*.go`, `router.go`, `api_only.go` | `internal/api/httpserver` + `internal/app/apiserver` |
| `pkg/jftradeapi/auth.go` | `internal/api/middleware` |
| `pkg/jftradeapi/settings_*.go` | `internal/api/settings` + `internal/settings` + `internal/store/sqlite/settings` |
| `pkg/jftradeapi/market_*.go` | `internal/api/marketdata` + `internal/marketdata` |
| `pkg/jftradeapi/broker_*.go` | `internal/api/trading` + `internal/trading` |
| `pkg/jftradeapi/strategy_*.go` | `internal/api/strategy` + `internal/strategy` + `internal/store/sqlite/strategy` |
| `pkg/jftradeapi/backtest_*.go`, `pkg/backtest` | `internal/api/backtest` + `internal/backtest` |
| `pkg/jftradeapi/adk_*.go`, `pkg/adk` | `internal/api/assistant` + `internal/assistant` |
| `pkg/jftradeapi/live_*.go`, `sse.go` | `internal/api/live` + `internal/live` |
| `pkg/jftradeapi/futu_runtime.go` | `internal/integration/futu` + 装配代码 |
| `pkg/futu` | `internal/integration/futu`，最后阶段迁移 |
| `internal/buildinfo`, `frontendassets`, `retry` | `internal/platform/*`，仅在确有收益时移动 |

迁移期间曾保留 `pkg/jftradeapi` 兼容门面；两个 `cmd` 入口切换到
`internal/app/apiserver` 后，该门面已删除。

## 6. 分阶段实施

### 阶段 0：建立保护线

- 固化 OpenAPI/Swagger 快照。
- 为关键路由增加 JSON 契约测试。
- 为 SQLite migration 和 repository 增加临时库测试。
- 在 CI 中运行 `go test ./...`、`go vet ./...` 和架构依赖检查。

完成标准：目录移动导致的路由、状态码、JSON 字段或表结构变化能被测试发现。

### 阶段 1：提取 HTTP 基础层

- 提取 `httpserver`、`middleware` 和通用 bind/response 逻辑。
- 把路由注册改为显式的 `RegisterRoutes(router, dependencies)`。
- 保留现有 service 和 store，不改变业务行为。

完成标准：`cmd` 不再直接依赖 `pkg/jftradeapi.Server` 的内部字段。

### 阶段 2：先迁移低耦合模块

按以下顺序提取：

1. `system`
2. `settings`
3. `backtest`
4. `strategy`

每个模块同时提取 handler、service 和 repository 接口；SQLite 实现放入
`internal/store/sqlite/<module>`。

完成标准：模块测试可以单独运行，handler 测试不启动 OpenD。

### 阶段 3：迁移实时行情与交易

- 提取 `marketdata.Provider` 和 `trading.Broker` 接口。
- 在 Futu 集成层完成 protobuf 到业务类型的转换。
- 把订阅、tick cache、订单更新 worker 的生命周期交给各自 service。
- 使用 `internal/live.Publisher` 发布事件，避免模块直接操作 SSE 客户端集合。

完成标准：`internal/api/*` 不再导入任何 `pkg/futu/pb/*`。

### 阶段 4：迁移 Assistant/ADK

- 将会话、审批、工具执行和 provider 分开。
- Google/OpenAI 实现下沉到 `internal/integration/llm`。
- SQLite 会话存储下沉到 `internal/store/sqlite/assistant`。

完成标准：assistant 核心测试使用 fake provider，不发网络请求。

### 阶段 5：收口兼容包

- 删除 `pkg/jftradeapi` 兼容入口，所有进程入口统一使用 `internal/app/apiserver`。
- 评估将 `pkg/adk`、`pkg/backtest`、`pkg/futu` 移入 `internal`。
- `pkg/strategy`、`pkg/market`、`pkg/broker` 是否保留，以外部复用需求为准。

完成标准：不存在从业务模块反向导入 transport 或具体集成实现的路径。

## 7. 首批建议提取项

首个实现批次建议控制在三个 PR：

1. `httpserver + middleware`：只移动基础设施，不碰领域行为。
2. `system`：依赖少，用于验证路由注册和 service 注入模式。
3. `settings`：验证 handler/service/repository 三层以及 SQLite 测试模式。

这三个 PR 完成后再决定 market、trading 和 live 的接口粒度。不要先从实时行情或
订单链路开刀，它们的生命周期和共享状态最多，适合作为边界模式稳定后的第二批。

## 8. 当前落地评估（2026-06-15）

本轮完成的是目录规划的首个可运行落地批次，不代表阶段 0 至阶段 5
全部完成。

本轮实际完成：

- `cmd/jftrade` 与 `cmd/jftrade-api` 已改由 `internal/app/apiserver` 启动；
  API/GUI server、runtime layout、launch defaults 和 shutdown 编排已有独立所有者。
- `internal/app/apiserver/lifecycle` 被 `cmd/jftrade` 与 `cmd/jftrade-api`
  共同复用，避免两套启动流程漂移；旧兼容门面已删除。
- JSON settings 实现已迁入 `internal/store/settingsfile`。该包只负责默认值、
  规范化和文件持久化，不导入 Futu，也不修改进程环境变量。
- integration env 默认值解析和应用集中在 `internal/app/apiserver/runtime`。
  启动时只应用一次最终配置；settings service 保存成功后应用一次。
- `internal/backtest` 已通过 `KLineSyncer` 隔离具体 broker、Futu exchange 和
  protobuf；Futu K 线同步实现、store/exchange 组装和枚举转换已迁入
  `internal/integration/futu`。上一版“下一批优先顺序”中的优先项 1 已完成。
- backtest HTTP 层已区分请求错误、策略不存在和内部错误；sync 的非法时间范围、
  rehab type 等输入返回 4xx，适配器/存储故障保持 5xx。
- backtest service 已拥有后台回测和 K 线同步的关闭生命周期。server 关闭时先
  取消并等待任务，再关闭 SQLite；sync 任务 ID 改为纳秒级，避免同秒覆盖。
- `internal/system`、`internal/settings`、`internal/backtest` 及对应关键 handler
  已有独立测试，覆盖持久化边界、runtime env、错误分类和异步关闭。
- 通知事件、单调 sequence、24 小时/200 条 replay cache 与 source 生命周期已迁入
  仅依赖标准库的 `internal/live.ReplayPublisher`。legacy Server 通过 source adapter
  注册 BBGO sink，关闭时会注销该实例的 sink；多个 Server 之间的生命周期互不影响。
- 阶段 C 已完成 WebSocket transport ownership 与 market subscription ownership 迁移：
  `/api/v1/ws/live` 的协议升级、连接上限、读循环、client subscription registry、
  dispatcher、notification replay、tick 去重和 security/depth push 位于
  `internal/api/live`；中立订阅模型与 registry 位于仅依赖标准库的 `internal/live`。
  legacy Server 只保留一个明确的 `liveWebSocket` 组件，并通过 Backend adapter
  提供旧 market/runtime 能力；Server shutdown 会关闭并等待该组件退出。
- HTTP consumer subscription registry 已迁入 `internal/marketdata.Service`；
  acquire、consumer release/clear、heartbeat、snapshot、active instruments、quota
  与 refCount 均由 service 持有。Provider 不再反向访问 legacy Server 的订阅字段，
  `Server.marketSubscriptions` 和旧 subscription state 文件已删除。
- 阶段 C2 第 1 批行情数据面已完成：broker-neutral Tick/ExtendedQuote/Candle 与
  response mapping、30 分钟/30000 条唯一 Cache、1.5 秒 freshness、继承/去重/
  source promotion、tick candle 均位于 `internal/marketdata`。HTTP snapshot/candles、
  WebSocket tick、legacy push stream 与 snapshot fallback 全部通过
  `marketdata.Service` 读写同一个 Cache；`Server.tickCache` 和旧
  `market_tick_{cache,samples,candles,serialization}.go` 已删除。
- 阶段 C2 第 2 批行情采集与 Futu runtime 生命周期迁移已完成：
  `internal/marketdata` 拥有 HTTP/WS/strategy active-demand 合并、stream generation、
  可取消 Connect、1 秒 fallback polling、900ms query timeout、1.5 秒 freshness、
  5/10/20/30 秒 backoff 以及 health/reset/close；异步 stream/query 结果只有在
  generation 匹配时才能提交。polling fallback 不经过 `PushTickHandler`，策略仍只消费
  push trade。
- `internal/integration/futu.MarketDataRuntime` 拥有 exchange 配置、创建、替换与关闭，
  并实现 NewStream/QueryTickers/QueryTicker/QuerySnapshot 及协议类型转换。它不拥有
  demand、cache、freshness 或 backoff。legacy `Server.liveQuoteState`、
  `Server.liveStreamState`、`live_runtime_state.go`、`market_live.go` 和
  `marketdata_tick_adapters.go` 已删除。
- Server shutdown 先关闭 WebSocket transport，再关闭并等待 marketdata collector，
  最后关闭 Futu runtime/exchange；Close/Reset 通过 generation 封住旧异步结果，
  关闭后不会再 Query/Open 或复活连接。
- 订单更新 worker 的同步编排、1.5 秒节流、60 秒 active cache、诊断快照、
  current/history/cache/push 元数据和订阅生命周期已迁入 `internal/trading`；
  Futu/protobuf push 转换与账户订阅位于 `internal/integration/futu`。`pkg/futu`
  现在提供本地可注销的 order/fill handler registry，每个 OpenD client 只绑定一次
  dispatcher。execution store 与通知文本继续由 `servercore` adapter 复用。
- 旧 `pkg/jftradeapi/broker_order_updates_worker.go` 和
  `Server.brokerOrderUpdates` 已删除；execution route、Futu reset 与 Server shutdown
  均通过 trading service 管理 worker。
- `scripts/check-arch-deps.sh` 已成为阻断式保护线，并检查：
  `internal/api`/`internal/backtest` 不依赖 Futu/protobuf、业务模块不反向依赖
  transport、`cmd` 不导入旧包、apiserver legacy 依赖隔离、settingsfile 不依赖
  broker integration、servercore backtest adapter 不回流 Futu/protobuf，
  且 Futu integration 不反向依赖 `internal/api`、`internal/live` 只依赖标准库，
  `internal/trading` 不依赖 Futu/protobuf/jftradeapi/internal/live。

阶段 D 进度：

- D0 已完成：5 个仅测试 helper 改为真正的 `_test.go`，不再计入生产编译。
- D1 已完成：`/api/v1/adk/*` 的 catalog、session/run、chat/SSE、approval、
  observability、skills 与 optimization transport 已迁入 `internal/api/assistant`；
  router 直接注册 `internal/assistant.Service`，旧 5 个 `adk_routes_*.go` 已删除。
- D1 后 `internal/api/assistant` 有独立 transport 包，catalog、session/run、
  chat/SSE、approval、observability、skills 与 optimization 不再位于旧大包。
- D2 已完成：`pkg/jftradeapi/adk_runtime.go` 和
  `pkg/jftradeapi/adk_strategy_pine_spec.go` 已删除；ADK runtime 初始化、DB/secrets/skills
  路径派生、SQLite backup、runtime limits provider、工具描述符注册，以及
  Pine spec/validate/save draft/save definition/update instance mode 的组合逻辑已迁入
  `internal/app/apiserver/servercore`。
- D3 已完成：旧 `Server` 聚合体、legacy routes/stores/adapters 及同包测试已机械迁入
  `internal/app/apiserver/servercore`；`pkg/jftradeapi` 兼容门面已删除，apiserver
  不再导入 `pkg/jftradeapi`。
- Swagger 生成扫描目录已切到 `internal/app/apiserver/servercore` 和已迁出的
  `internal/api/system`、`internal/api/marketdata`；OpenAPI 产物已重新生成。
- `scripts/check-arch-deps.sh` 现在阻断 70 项依赖/结构规则，包括
  `pkg/jftradeapi` 不得重新出现生产 Go 文件、`internal/app/apiserver` 不得导入
  兼容门面，以及 `go list` 失败也会使检查失败。

V1 后续改进（非本轮完成门槛）：

1. **继续拆分 servercore 内部聚合体**：broker/execution/strategy/plugin 等目前已在
   internal 内部化，但仍位于 `servercore` 聚合包。后续可在不影响兼容门面的前提下，
   继续下沉到 `internal/api/trading`、`internal/store/sqlite/*` 等更细目录。
2. **ADK SQLite ownership 收口**：provider、session、approval 与部分 store
   ownership 仍由 `pkg/adk` 承载。后续可迁入 `internal/store/sqlite/assistant` 与
   `internal/integration/llm`，让 assistant 核心测试完全使用 fake provider。
3. **生命周期接口升级**：shutdown 的 handler 接口仍是无 context 的 `Close()`；当前
   backtest、marketdata、trading worker 已能按 context/close 退出，后续可统一成可受
   shutdown deadline 约束的生命周期接口。
4. **评估 pkg internal 化**：继续评估 `pkg/futu`、`pkg/adk`、`pkg/backtest` 是否应在
   后续版本窗口移入 `internal`，以外部复用需求为准。

## 9. 验收指标

- `pkg/jftradeapi` 已删除，架构检查阻止兼容门面重新出现。
- 任一 API handler 包不超过 25 个生产文件。
- `internal/api` 对 `pkg/futu/pb` 的直接依赖数为 0。
- 每个业务模块都有独立 service 测试；外部集成可用 fake 替换。
- 所有现有路由、JSON 字段、SQLite 表名和命令行入口保持兼容。
- `go test ./...`、`go vet ./...`、两个二进制构建全部通过。
- 包依赖图无环，并通过 CI 规则阻止禁止依赖回流。

## 10. V1 非目标

- 不拆分为微服务。
- 不更换 Gin、GORM/SQLite、bbgo 或 Futu SDK。
- 不重写 Pine 策略引擎。
- 不在目录迁移中顺带修改 API 命名或数据库 schema。
- 不为了减少文件数合并已经职责清晰的小文件。
