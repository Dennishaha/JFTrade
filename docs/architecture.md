# 当前系统架构

本文只回答三个问题：

- 系统现在由哪些组件组成
- 请求和实时数据分别走哪条链路
- 后续开发该从哪个边界进入，才不会把 sidecar、bbgo 和前端职责混在一起

协议细节、K 线细枝末节、排障案例不再放在本文，分别下沉到专题文档。

## 一句话概括

JFTrade 当前是一个“双核心”系统：

- 一条是基于 [pkg/jftradeapi](../pkg/jftradeapi) 的前端 API sidecar，负责 `/api/v1/*`、设置持久化、行情查询、实时推送和兼容层。
- 一条是基于 bbgo 的运行时，负责策略执行、交易抽象和 bbgo 原生运行模式。

两条链路都复用 [pkg/futu](../pkg/futu) 作为 Futu 适配层，但入口和职责不同。

## 组件关系

```mermaid
flowchart LR
    Web[apps/web\nVue 3 + Vite] -->|HTTP /api/v1/*| Sidecar[pkg/jftradeapi\nAPI sidecar]
    Web -->|SSE /api/v1/stream/live| Sidecar
    Sidecar -->|direct use| Futu[pkg/futu\nFutu Exchange]
    Sidecar -->|notifications| BBGONotify[bbgo notifier bridge]
    Futu --> OpenD[Futu OpenD\nAPI TCP 11110]

    CLI[cmd/jftrade] -->|api / serve-api| Sidecar
    CLI -->|run| BBGO[bbgo runtime]
    BBGO -->|exchange.Register| Futu
    BBGO -->|optional UI/API| BBGOServer[bbgo /api/*]
```

## 运行模式

[cmd/jftrade/main.go](../cmd/jftrade/main.go) 当前支持两种主路径。

| 模式 | 入口 | 主要用途 | 核心组件 |
| --- | --- | --- | --- |
| API-only | `go run ./cmd/jftrade api` 或 `JFTRADE_API_ONLY=1` | 前端开发、配置调试、行情与通知调试 | `cmd/jftrade` -> `pkg/jftradeapi` -> `pkg/futu` -> OpenD |
| bbgo run | `go run ./cmd/jftrade run --config ./config/jftrade.yaml` | 策略运行、交易运行时、完整 bbgo 集成 | `cmd/jftrade` -> bbgo runtime -> `pkg/futu`，并同时启动 sidecar |

当前默认开发心智应是：

- 前端和控制台功能优先围绕 API-only 模式理解。
- 交易运行时和策略行为优先围绕 bbgo run 模式理解。

## 核心职责边界

### 1. cmd/jftrade

职责：决定进程以哪种模式启动。

- 在 API-only 模式下，只启动 sidecar。
- 在 run 模式下，先尝试启动 sidecar，再进入 bbgo `cmd.Execute()`。
- 统一补默认环境，例如 `DISABLE_MARKETS_CACHE=1`。

它不是业务层，不负责实现行情、设置或协议逻辑。

### 2. pkg/jftradeapi

职责：提供面向前端的控制平面和兼容 API。

它当前承载：

- `/api/v1/settings/*`：Broker 配置持久化和运行时注入
- `/api/v1/system/*`：状态、诊断、OpenD 探针、通知
- `/api/v1/market-data/*`：订阅、快照、K 线查询
- `/api/v1/strategy-definitions/*`：DSL 策略定义、实例化、visualModel 持久化
- `/api/v1/stream/live`：实时心跳、tick、系统通知
- 与 bbgo notifier 的桥接缓存

这里是前端默认依赖的 API 面，不是 bbgo 原生 `/api/*` 的镜像。

### 3. pkg/futu

职责：把 Futu OpenD 包装成可复用的交易所适配层。

它有两种被使用的方式：

- 被 bbgo 通过 `exchange.Register("futu", ...)` 当作交易所插件实例化。
- 被 sidecar 直接创建 `futu.Exchange`，用于行情查询、实时订阅和 OpenD 探针。

因此这里既是 bbgo 集成点，也是 sidecar 的 broker 访问层。

### 4. bbgo runtime

职责：策略运行、交易抽象和 bbgo 生命周期管理。

当前文档要避免的常见误判是：

- 不是所有前端请求都走 bbgo `/api/*`
- JFTrade 前端主要依赖的是 sidecar `/api/v1/*`
- bbgo 自带 server 不是当前控制台的唯一后端

### 5. apps/web

职责：提供控制台 UI，消费 sidecar 的 REST 与 SSE 能力。

前端应优先假设：

- 设置、诊断、实时通知来自 sidecar
- 行情快照和 K 线查询来自 sidecar
- 实时 tick 来自 `/api/v1/stream/live`
- 交易环境、市场、连接状态、订单状态和风控状态等枚举值在前端展示时通过 `apps/web/src/composables/consoleDataFormatting.ts` 统一转成中文标签，避免修改 API 原始值
- 策略设计工作区同时承载 Logic Flow visualModel 与 DSL script，浏览器内代码编辑使用 Monaco，测试环境回退 textarea

## 请求与数据流

### 设置与系统状态

```text
apps/web
  -> /api/v1/settings/* 或 /api/v1/system/*
  -> pkg/jftradeapi
  -> SettingsStore / runtime env / OpenD probe
```

这一层主要是控制平面，不等同于交易执行。

### 策略设计与运行控制

```text
apps/web
  -> /api/v1/strategy-definitions/*
  -> pkg/jftradeapi/strategy_routes.go
  -> strategy_design_store / strategy_catalog_store
  -> DSL strategy definition + optional visualModel persistence
```

这条链路当前有四个重要约束：

- 策略定义同时保存 DSL 源码和可选 `visualModel`，二者都属于控制平面数据。
- 前端支持 Logic Flow 可视化编辑和纯代码编辑，图块不变；可视模型会生成 DSL，DSL 会统一交给后端解析、规划并编译成 Go runtime 可消费的 IR。
- 加载已有定义时优先保留已保存的 `visualModel`；只有显式同步或修改图块参数后，才会重新生成 DSL。
- 设计页的浏览器代码编辑器使用 Monaco，但前端测试仍走 textarea 回退，避免 jsdom 初始化重量级编辑器。

### 实时行情链路

```text
apps/web
  -> SSE /api/v1/stream/live
  -> pkg/jftradeapi/live dispatcher
  -> market subscription manager
  -> futu.Exchange.NewStream() / QueryTickers()
  -> Futu OpenD
```

这条链路有两个关键特征：

- 优先使用 OpenD push 流维持实时性。
- 当样本不够新时，sidecar 会回退到 `QueryTickers()` 做补采样。

因此实时行情不是“纯 SSE 推送”或“纯 HTTP 轮询”，而是由 sidecar 统一调度的混合链路。

### K 线与快照链路

```text
apps/web
  -> /api/v1/market-data/candles/* 与 /api/v1/market-data/snapshots/*
  -> pkg/jftradeapi
  -> futu.Exchange
  -> Futu OpenD 历史/快照查询
  -> sidecar 归一化后返回前端
```

K 线的桶时间归一化、当前未收盘桶补齐、tick 驱动的实时叠加，都属于专题问题，详见 [frontend-kline.md](frontend-kline.md)。

### 通知链路

```text
Futu OpenD protocol 1003 / bbgo.Notify(...)
  -> pkg/jftradeapi notification cache
  -> /api/v1/stream/live
  -> apps/web Notification Center
```

sidecar 当前负责把 Futu 系统通知和 bbgo 通知收束到同一条前端消费链路。

## 当前约束与设计取舍

### 非侵入式 bbgo 接入仍然成立

本项目仍然保持“不改 bbgo 源码、通过公开扩展点接入”的原则：

- `pkg/futu` 在 `init()` 中注册到 bbgo exchange factory。
- `cmd/jftrade` 复用 bbgo CLI 入口。
- 不支持的交易所能力通过 `ErrNotSupported` 明确暴露，而不是伪实现。

但这已经不是系统全貌。当前真实系统还包括 sidecar 这条前端兼容层和控制平面。

### sidecar 与 bbgo server 不等价

维护文档和实现时必须区分：

- bbgo 原生 server 主要是 `/api/*`
- JFTrade 控制台主要使用 `/api/v1/*`
- 两者可以共存，但职责不同

任何需求如果直接假设“前端应改去接 bbgo 原生接口”，都需要先重新审查是否破坏现有控制台契约。

### Futu 适配层是共享依赖，不是单一插件

`pkg/futu` 既服务 bbgo，也服务 sidecar。改这里时必须先判断是：

- 改 bbgo 交易所抽象行为
- 改 sidecar 行情/连接行为
- 还是同时影响两者

## 后续开发的入口建议

如果你准备继续开发，先按下面顺序判断定位：

1. 改启动方式、运行模式、环境变量：先看 [../cmd/jftrade/main.go](../cmd/jftrade/main.go)
2. 改前端 API、系统状态、设置：先看 [../pkg/jftradeapi/server.go](../pkg/jftradeapi/server.go)
3. 改策略定义、模板、DSL/Logic Flow 同步：先看 [../pkg/jftradeapi/strategy_routes.go](../pkg/jftradeapi/strategy_routes.go)、[../pkg/jftradeapi/strategy_design_store.go](../pkg/jftradeapi/strategy_design_store.go)、[../apps/web/src/pages/StrategyPage.vue](../apps/web/src/pages/StrategyPage.vue) 和 [../apps/web/src/features/strategyVisualBuilder.ts](../apps/web/src/features/strategyVisualBuilder.ts)
4. 改行情订阅、实时推送、通知：先看 [../pkg/jftradeapi/market_routes.go](../pkg/jftradeapi/market_routes.go) 和 [../pkg/jftradeapi/market_live.go](../pkg/jftradeapi/market_live.go)
5. 改 Futu 协议、映射、连接：先看 [../pkg/futu/exchange.go](../pkg/futu/exchange.go) 与 reference 层文档
6. 改实时 K 线：先看 [frontend-kline.md](frontend-kline.md)

## 相关文档

- [README.md](README.md)：docs 阅读入口
- [troubleshooting.md](troubleshooting.md)：排障入口
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：前端策略设计专题
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口
- [reference/README.md](reference/README.md)：协议与参考资料入口

## 6. 回测系统（新增）

### 6.1 pkg/futu/backtest

职责：提供基于 SQLite 的历史 K 线存储和回测执行能力。

- `store.go` — 实现 `service.BackTestable` 接口，管理 `local_klines` 表
- `sync.go` — 从 Futu OpenD 增量同步 K 线数据到 SQLite
- `runner.go` — 回测执行编排，创建 bbgo backtest.Exchange，并运行纯 Go DSL 策略

### 6.2 历史数据管线

```
OpenD Qot_RequestHistoryKL (3103)
  → pkg/futu/exchange.go QueryKLines()
    → backtest.SyncKLines()
      → local_klines (SQLite)
        → backtest.FutuKLineStore.QueryKLinesBackward/Forward()
          → bbgo backtest.Exchange
            → SimplePriceMatching (bar close 撮合)
              → DSL Go executor → placeOrder/cancelOrder
```

### 6.3 操作入口

所有回测操作通过前端控制台触发，不提供独立 CLI 命令。

### 6.4 Sidecar API

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/api/v1/backtests` | 发起回测（指定策略定义ID、标的、周期、时段、初始资金，以及是否在回放时包含扩展交易时段数据） |
| GET | `/api/v1/backtests` | 列出历史回测记录 |
| DELETE | `/api/v1/backtests/{id}` | 删除已完成或失败的回测记录 |
| GET | `/api/v1/backtests/{id}/status` | 轮询回测状态 |
| GET | `/api/v1/backtests/{id}` | 获取回测结果详情 |
| POST | `/api/v1/backtests/sync` | 同步历史K线数据 |

回测 run 元数据现在会独立落到 sidecar 本地 SQLite；因此 `/api/v1/backtests` 不再只返回“当前进程内列表”，而会跨 sidecar 重启恢复历史结果。恢复时如果发现上一进程遗留的 `queued/running` run，会在启动期自动改记为 `failed`，并附上“sidecar restarted before backtest completed”错误，避免前端出现永久运行中的僵尸记录。回测页前端仍保留浏览器本地持久化，作为 sidecar 暂不可用时的补充缓存：

- 最近一次使用的回测参数会写入浏览器 `localStorage`，下次进入页面时会用这组参数作为默认填充值。
- 已完成或失败的回测结果会缓存到浏览器本地；如果 sidecar 当前没有返回历史列表，前端仍可先展示这批本地缓存。
- 回测页里的“删除结果”会优先调用 sidecar `DELETE /api/v1/backtests/{id}` 删除当前进程内的终态回测；如果 sidecar 当前删不到该结果或删除失败，前端会退回到本地隐藏，并把对应 run id 记入浏览器删除名单，避免旧缓存回流。
- 结果列表前端已支持搜索、状态筛选、策略筛选和分页，避免历史回测积累后难以定位目标结果。
- backtest run 在启动时会快照当前 `definitionVersion`；回测页会把 run 使用的策略版本和当前定义版本做对比，不一致时提示“旧版本策略回测结果”，避免把历史结果误当成最新策略表现。

当前回测的 session 语义已经同时落到 runner 和 store schema：

- 前端 `/api/v1/backtests` 仍通过 `useExtendedHours` 控制 replay 读取 regular-only 还是 regular+extended 数据版本。
- `/api/v1/backtests/sync` 新增 `sessionScope`，SQLite 会以紧凑的表级 tag 区分 `legacy`、`regular`、`extended` 三类版本；regular-only 与 regular+extended 现在可以在同一 symbol/interval/rehabType 下并存，不再互相覆盖。
- `extended` 版本只会落在真正有扩展时段差异的 US `60m` 及以下基础周期表上；`1d` / `1w` / `1mo` 与 `2h` / `4h` / `6h` / `12h` 的 extended 回测，会优先从这些 sub-daily extended 基础表做 trading-period 或 session-aware synthesis。
- sync 入口会把需要合成的高周期自动规划到基础同步周期：`2h` / `4h` / `6h` / `12h` 映射到 `1h`，US extended 的 `1d` / `1w` / `1mo` 也会落到 `1h` 基础数据，避免把 native regular-only 日线误写进 extended 版本。
- backtest SQLite 的 OHLCV 仍以 `fixedpoint.Value.String()` 形式存为 TEXT；读取侧对这种 plain decimal 存储格式走本地 fast path，只有遇到科学计数、百分号或 `inf` 之类的非常规输入时才回退上游 `fixedpoint.NewFromString`，以控制 `QueryKLinesBackward/Forward` 的解析开销。
- session-aware higher intraday 的 backward 查询按时间窗口分页拉取后，会先批量合并本批结果再一次性 prepend 到最终序列，避免逐条前插造成的重复切片拷贝和额外分配。
- 单标的单周期的回测回放现在会在 `QueryKLinesCh` 走 fast path：direct stored-range 查询会按行直接流到 replay channel，而不再先整段收集和排序；single-series path 的 channel buffer 也已收紧到较小固定值，避免为每次回放预留过大的 channel backing array。regular-only 的 replay wrapper 同样会直接边读边过滤 regular session bar，不再先把底层 channel 全量收集成切片再过滤。`resultCollector` 同时会按预计 bar 数预分配 `candles/pnlCurve`，并在权益计算时直接读取单币种余额，避免每根 bar 复制整张 `BalanceMap`。DSL runtime 同时会缓存 divergence requirement key，并且仅在 `if` 分支内存在 `let` 时才为该分支创建子作用域，避免无意义的字符串构造和 scope clone；表达式层面对 moving-average / MACD / KDJ 这类快照对象新增了序列直读和首选标量快路径，`cross_over(ma, slow)`、`ma.value`、`ma > 10` 之类路径不再反复通过字符串 `FieldValue` 探测。

- `useExtendedHours=true` 不再只影响 replay 数据版本和 higher-period K 线 synthesis。对 US backtest，DSL indicatorruntime 的 moving-average 与 stop-loss `day/week/month` 窗口现在都会切到 extended trading-period 口径，并且回测 warmup 会按 extended trading day 分钟数放大；这样 MA/保护条件/runtime warmup 三者的 trading-window 语义已经对齐。

### 6.5 策略回测感知

DSL 策略通过纯 Go executor 在回测和实盘泵线里执行同一套条件判断与下单逻辑：
```text
strategy BacktestAware
version 0.1.0

on init:
  log "strategy initialized"

on kline_close:
  log "kline closed"
```
